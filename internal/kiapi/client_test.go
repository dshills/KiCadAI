package kiapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/config"
	"kicadai/internal/ipc"
	"kicadai/internal/kiapi/gen/common"
	commoncommands "kicadai/internal/kiapi/gen/common/commands"
	commontypes "kicadai/internal/kiapi/gen/common/types"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestClientSendsEnvelope(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "returned-token", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, &commoncommands.GetVersionResponse{
		Version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
	}))

	client, err := NewClient(context.Background(), testConfig("initial-token"), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	var response commoncommands.GetVersionResponse
	if err := client.Send(context.Background(), &commoncommands.GetVersion{}, &response); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if got := transport.DialedEndpoint(); got != "ipc:///tmp/kicad/api.sock" {
		t.Fatalf("DialedEndpoint = %q", got)
	}
	request := sentRequest(t, transport)
	if request.GetHeader().GetClientName() != "test-client" {
		t.Fatalf("client name = %q", request.GetHeader().GetClientName())
	}
	if request.GetHeader().GetKicadToken() != "initial-token" {
		t.Fatalf("token = %q", request.GetHeader().GetKicadToken())
	}
	var command commoncommands.GetVersion
	if err := request.GetMessage().UnmarshalTo(&command); err != nil {
		t.Fatalf("request command did not unpack as GetVersion: %v", err)
	}
	if response.GetVersion().GetFullVersion() != "9.1.0" {
		t.Fatalf("version response = %q", response.GetVersion().GetFullVersion())
	}
	if client.Token() != "initial-token" {
		t.Fatalf("client token changed unexpectedly to %q", client.Token())
	}
}

func TestClientCapturesTokenWhenUnset(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "returned-token", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, nil))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if err := client.Send(context.Background(), &commoncommands.Ping{}, nil); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if client.Token() != "returned-token" {
		t.Fatalf("client token = %q", client.Token())
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", &common.ApiResponseStatus{
		Status:       common.ApiStatusCode_AS_TOKEN_MISMATCH,
		ErrorMessage: "token mismatch",
	}, nil))

	client, err := NewClient(context.Background(), testConfig("wrong-token"), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.Ping{}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Send error = %v, want APIError", err)
	}
	if apiErr.Status != common.ApiStatusCode_AS_TOKEN_MISMATCH {
		t.Fatalf("status = %s", apiErr.Status)
	}
	if !strings.Contains(apiErr.Error(), "token mismatch") {
		t.Fatalf("error text = %q", apiErr.Error())
	}
}

func TestAPIErrorWithoutMessage(t *testing.T) {
	err := (&APIError{Status: common.ApiStatusCode_AS_BUSY}).Error()
	if !strings.Contains(err, "AS_BUSY") {
		t.Fatalf("error text = %q", err)
	}
	if got := (*APIError)(nil).Error(); got != "" {
		t.Fatalf("nil APIError text = %q", got)
	}
}

func TestClientErrorFormattingAndUnwrap(t *testing.T) {
	want := errors.New("boom")
	err := &ClientError{Op: OpDial, Err: want}
	if !strings.Contains(err.Error(), "kicad API dial") {
		t.Fatalf("error text = %q", err.Error())
	}
	if !errors.Is(err, want) {
		t.Fatalf("ClientError did not unwrap %v", want)
	}
	if got := (*ClientError)(nil).Error(); got != "" {
		t.Fatalf("nil ClientError text = %q", got)
	}
	if got := (*ClientError)(nil).Unwrap(); got != nil {
		t.Fatalf("nil ClientError unwrap = %v", got)
	}
}

func TestClientWrongResponseTypeFailsCleanly(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, &commoncommands.Ping{}))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.GetVersion{}, &commoncommands.GetVersionResponse{})
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("Send error = %v, want ClientError", err)
	}
	if clientErr.Op != OpUnpackResponse {
		t.Fatalf("op = %s, want %s", clientErr.Op, OpUnpackResponse)
	}
}

func TestClientMissingResponseMessageFailsCleanly(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, nil))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.GetVersion{}, &commoncommands.GetVersionResponse{})
	if !errors.Is(err, ErrMissingResponseAny) {
		t.Fatalf("Send error = %v, want ErrMissingResponseAny", err)
	}
}

func TestClientMissingStatusFailsCleanly(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", nil, nil))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.Ping{}, nil)
	if !errors.Is(err, ErrMissingStatus) {
		t.Fatalf("Send error = %v, want ErrMissingStatus", err)
	}
}

func TestClientMalformedResponseFailsCleanly(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse([]byte("not protobuf"))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.Ping{}, nil)
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("Send error = %v, want ClientError", err)
	}
	if clientErr.Op != OpUnmarshalResponse {
		t.Fatalf("op = %s, want %s", clientErr.Op, OpUnmarshalResponse)
	}
}

func TestClientRejectsNilCommand(t *testing.T) {
	transport := &ipc.FakeTransport{}
	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), nil, nil)
	if !errors.Is(err, ErrNilCommand) {
		t.Fatalf("Send error = %v, want ErrNilCommand", err)
	}
}

func TestClientRejectsUninitializedSend(t *testing.T) {
	err := (*Client)(nil).Send(context.Background(), &commoncommands.Ping{}, nil)
	if !errors.Is(err, ErrUninitialized) {
		t.Fatalf("Send error = %v, want ErrUninitialized", err)
	}

	err = (&Client{}).Send(context.Background(), &commoncommands.Ping{}, nil)
	if !errors.Is(err, ErrUninitialized) {
		t.Fatalf("Send error = %v, want ErrUninitialized", err)
	}
}

func TestClientRespectsCanceledContextBeforeRequest(t *testing.T) {
	transport := &ipc.FakeTransport{}
	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = client.Send(ctx, &commoncommands.Ping{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
}

func TestClientPingWrapper(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, nil))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	request := sentRequest(t, transport)
	var command commoncommands.Ping
	if err := request.GetMessage().UnmarshalTo(&command); err != nil {
		t.Fatalf("request command did not unpack as Ping: %v", err)
	}
}

func TestClientGetVersionWrapper(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(responsePayload(t, "", &common.ApiResponseStatus{
		Status: common.ApiStatusCode_AS_OK,
	}, &commoncommands.GetVersionResponse{
		Version: &commontypes.KiCadVersion{Major: 9, Minor: 2, Patch: 1, FullVersion: "9.2.1"},
	}))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	version, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if version.GetFullVersion() != "9.2.1" {
		t.Fatalf("version = %q", version.GetFullVersion())
	}
}

func TestClientGetVersionWrapperReturnsError(t *testing.T) {
	want := errors.New("request failed")
	transport := &ipc.FakeTransport{}
	transport.SetSendError(want)

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	version, err := client.GetVersion(context.Background())
	if version != nil {
		t.Fatalf("version = %+v, want nil", version)
	}
	if !errors.Is(err, want) {
		t.Fatalf("GetVersion error = %v, want %v", err, want)
	}
}

func TestClientCloseOwnedTransport(t *testing.T) {
	transport := &ipc.FakeTransport{}
	client := &Client{transport: transport, ownsTransport: true}

	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestClientCloseOwnedTransportError(t *testing.T) {
	want := errors.New("close failed")
	transport := &ipc.FakeTransport{}
	transport.SetCloseError(want)
	client := &Client{transport: transport, ownsTransport: true}

	err := client.Close()
	if !errors.Is(err, want) {
		t.Fatalf("Close error = %v, want %v", err, want)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.Op != OpClose {
		t.Fatalf("Close error = %v, want OpClose ClientError", err)
	}
}

func TestClientCloseUnownedTransportIsNoop(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.SetCloseError(errors.New("should not close"))
	client := &Client{transport: transport}

	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := (*Client)(nil).Close(); err != nil {
		t.Fatalf("nil Close returned error: %v", err)
	}
}

func TestClientTransportErrorsAreWrapped(t *testing.T) {
	want := errors.New("boom")
	transport := &ipc.FakeTransport{}
	transport.SetSendError(want)

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	err = client.Send(context.Background(), &commoncommands.Ping{}, nil)
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("Send error = %v, want ClientError", err)
	}
	if clientErr.Op != OpTransportRequest {
		t.Fatalf("op = %s, want %s", clientErr.Op, OpTransportRequest)
	}
	if !errors.Is(err, want) {
		t.Fatalf("Send error = %v, want wrapped %v", err, want)
	}
}

func testConfig(token string) config.Config {
	return config.Config{
		SocketPath: "ipc:///tmp/kicad/api.sock",
		Token:      token,
		ClientName: "test-client",
		TimeoutMS:  2000,
	}
}

func responsePayload(t *testing.T, token string, status *common.ApiResponseStatus, message proto.Message) []byte {
	t.Helper()

	var packed *anypb.Any
	if message != nil {
		var err error
		packed, err = anypb.New(message)
		if err != nil {
			t.Fatalf("packing response: %v", err)
		}
	}

	payload, err := proto.Marshal(&common.ApiResponse{
		Header:  &common.ApiResponseHeader{KicadToken: token},
		Status:  status,
		Message: packed,
	})
	if err != nil {
		t.Fatalf("marshaling response: %v", err)
	}
	return payload
}

func sentRequest(t *testing.T, transport *ipc.FakeTransport) *common.ApiRequest {
	t.Helper()

	messages := transport.SentMessages()
	if len(messages) != 1 {
		t.Fatalf("sent message count = %d, want 1", len(messages))
	}

	var request common.ApiRequest
	if err := proto.Unmarshal(messages[0], &request); err != nil {
		t.Fatalf("unmarshaling request: %v", err)
	}
	return &request
}
