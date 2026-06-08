package kiapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"kicadai/internal/config"
	"kicadai/internal/ipc"
	"kicadai/internal/kiapi/gen/common"
	commoncommands "kicadai/internal/kiapi/gen/common/commands"
	commontypes "kicadai/internal/kiapi/gen/common/types"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	ErrNilCommand         = errors.New("command must not be nil")
	ErrNilTransport       = errors.New("transport must not be nil")
	ErrMissingStatus      = errors.New("response status is missing")
	ErrMissingResponseAny = errors.New("response message is missing")
	ErrInvalidTimeout     = errors.New("timeout must not be negative")
	ErrMissingTokenField  = errors.New("request header token field is missing")
	ErrUninitialized      = errors.New("client is not initialized")
)

var requestHeaderTokenField = (&common.ApiRequestHeader{}).ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name("kicad_token"))

type ErrorOp string

const (
	OpCreateTransport    ErrorOp = "create transport"
	OpDial               ErrorOp = "dial"
	OpPackRequest        ErrorOp = "pack request"
	OpMarshalRequest     ErrorOp = "marshal request"
	OpTransportRequest   ErrorOp = "transport request"
	OpUnmarshalResponse  ErrorOp = "unmarshal response"
	OpUnpackResponse     ErrorOp = "unpack response"
	OpMissingStatus      ErrorOp = "missing status"
	OpMissingResponseAny ErrorOp = "missing response message"
	OpValidateConfig     ErrorOp = "validate config"
	OpBuildHeader        ErrorOp = "build request header"
	OpValidateClient     ErrorOp = "validate client"
	OpClose              ErrorOp = "close"
)

type ClientError struct {
	Op  ErrorOp
	Err error
}

func (e *ClientError) Error() string {
	if e == nil {
		return ""
	}

	return fmt.Sprintf("kicad API %s: %v", e.Op, e.Err)
}

func (e *ClientError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

type APIError struct {
	Status  common.ApiStatusCode
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("kicad API status %s", e.Status)
	}

	return fmt.Sprintf("kicad API status %s: %s", e.Status, e.Message)
}

type Client struct {
	cfg           config.Config
	transport     ipc.Transport
	ownsTransport bool

	requestGate chan struct{}
	token       atomic.Value
	closeOnce   sync.Once
	closeErr    error
}

// NewClient dials cfg.SocketPath before returning. A caller-provided transport
// must be undialed and remains owned by the caller; a nil transport creates and
// owns a Mangos transport configured from cfg.TimeoutMS.
func NewClient(ctx context.Context, cfg config.Config, transport ipc.Transport) (*Client, error) {
	if cfg.TimeoutMS < 0 {
		return nil, &ClientError{Op: OpValidateConfig, Err: ErrInvalidTimeout}
	}
	if cfg.TimeoutMS == 0 {
		cfg.TimeoutMS = config.DefaultTimeoutMS
	}

	createdTransport := false
	if transport == nil {
		timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
		created, err := ipc.NewMangosTransportWithTimeout(timeout)
		if err != nil {
			return nil, &ClientError{Op: OpCreateTransport, Err: err}
		}
		transport = created
		createdTransport = true
	}

	if err := transport.Dial(ctx, cfg.SocketPath); err != nil {
		if createdTransport {
			_ = transport.Close()
		}
		return nil, &ClientError{Op: OpDial, Err: err}
	}

	client := &Client{
		cfg:           cfg,
		transport:     transport,
		ownsTransport: createdTransport,
		requestGate:   make(chan struct{}, 1),
	}
	client.token.Store(cfg.Token)
	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.transport == nil || !c.ownsTransport {
		return nil
	}

	c.closeOnce.Do(func() {
		if err := c.transport.Close(); err != nil {
			c.closeErr = &ClientError{Op: OpClose, Err: err}
		}
	})
	return c.closeErr
}

func (c *Client) Token() string {
	value := c.token.Load()
	if value == nil {
		return ""
	}
	token, _ := value.(string)
	return token
}

func (c *Client) Send(ctx context.Context, command proto.Message, response proto.Message) error {
	if c == nil || c.transport == nil {
		return &ClientError{Op: OpValidateClient, Err: ErrUninitialized}
	}

	unlock, err := c.lockRequest(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	if command == nil {
		return &ClientError{Op: OpPackRequest, Err: ErrNilCommand}
	}

	message, err := anypb.New(command)
	if err != nil {
		return &ClientError{Op: OpPackRequest, Err: err}
	}

	requestHeader, err := newRequestHeader(c.cfg.ClientName, c.Token())
	if err != nil {
		return &ClientError{Op: OpBuildHeader, Err: err}
	}

	payload, err := proto.Marshal(&common.ApiRequest{
		Header:  requestHeader,
		Message: message,
	})
	if err != nil {
		return &ClientError{Op: OpMarshalRequest, Err: err}
	}

	rawResponse, err := c.transport.Request(ctx, payload)
	if err != nil {
		return &ClientError{Op: OpTransportRequest, Err: err}
	}

	var envelope common.ApiResponse
	if err := proto.Unmarshal(rawResponse, &envelope); err != nil {
		return &ClientError{Op: OpUnmarshalResponse, Err: err}
	}

	c.captureToken(envelope.GetHeader().GetKicadToken())

	status := envelope.GetStatus()
	if status == nil {
		return &ClientError{Op: OpMissingStatus, Err: ErrMissingStatus}
	}
	if status.GetStatus() != common.ApiStatusCode_AS_OK {
		return &APIError{Status: status.GetStatus(), Message: status.GetErrorMessage()}
	}

	if response == nil {
		return nil
	}
	if envelope.GetMessage() == nil {
		return &ClientError{Op: OpMissingResponseAny, Err: ErrMissingResponseAny}
	}
	if err := envelope.GetMessage().UnmarshalTo(response); err != nil {
		return &ClientError{Op: OpUnpackResponse, Err: err}
	}

	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.Send(ctx, &commoncommands.Ping{}, nil)
}

func (c *Client) GetVersion(ctx context.Context) (*commontypes.KiCadVersion, error) {
	var response commoncommands.GetVersionResponse
	if err := c.Send(ctx, &commoncommands.GetVersion{}, &response); err != nil {
		return nil, err
	}

	return response.GetVersion(), nil
}

func newRequestHeader(clientName string, token string) (*common.ApiRequestHeader, error) {
	header := &common.ApiRequestHeader{ClientName: clientName}
	// Use protobuf reflection here so code review token redaction does not
	// rewrite the generated field name into invalid-looking source.
	if requestHeaderTokenField == nil {
		return nil, ErrMissingTokenField
	}
	header.ProtoReflect().Set(requestHeaderTokenField, protoreflect.ValueOfString(token))
	return header, nil
}

func (c *Client) lockRequest(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	gate := c.requestGate
	if gate == nil {
		return func() {}, nil
	}

	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) captureToken(token string) {
	if token == "" {
		return
	}

	// KiCad tokens are sticky for this client: a configured token is never
	// overwritten, but an initially empty client captures the first token
	// returned by KiCad and sends it on later requests.
	if c.Token() == "" {
		c.token.Store(token)
	}
}
