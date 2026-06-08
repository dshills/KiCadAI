package ipc

import (
	"context"
	"sync"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/req"
	_ "go.nanomsg.org/mangos/v3/transport/ipc"
)

const (
	deadlineUpdateThreshold = 10 * time.Millisecond
	blockingDeadline        = 24 * time.Hour
)

type MangosTransport struct {
	stateMu  sync.Mutex
	opMu     sync.Mutex
	socket   mangos.Socket
	endpoint string
	timeout  TimeoutConfig
	current  TimeoutConfig
}

func NewMangosTransport(timeout TimeoutConfig) (*MangosTransport, error) {
	socket, err := newMangosSocket(timeout)
	if err != nil {
		return nil, err
	}

	return &MangosTransport{socket: socket, timeout: timeout, current: socketDeadlines(timeout)}, nil
}

func newMangosSocket(timeout TimeoutConfig) (mangos.Socket, error) {
	socket, err := req.NewSocket()
	if err != nil {
		return nil, &ConnectionError{Op: "create request socket", Cause: err}
	}

	deadlines := socketDeadlines(timeout)
	if err := socket.SetOption(mangos.OptionSendDeadline, deadlines.SendTimeout); err != nil {
		_ = socket.Close()
		return nil, &ConnectionError{Op: "set send timeout", Cause: err}
	}

	if err := socket.SetOption(mangos.OptionRecvDeadline, deadlines.RecvTimeout); err != nil {
		_ = socket.Close()
		return nil, &ConnectionError{Op: "set receive timeout", Cause: err}
	}

	return socket, nil
}

func NewMangosTransportWithTimeout(timeout time.Duration) (*MangosTransport, error) {
	return NewMangosTransport(TimeoutConfig{
		SendTimeout: timeout,
		RecvTimeout: timeout,
	})
}

func (t *MangosTransport) Dial(ctx context.Context, endpoint string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	t.opMu.Lock()
	defer t.opMu.Unlock()

	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	if t.socket == nil {
		socket, err := newMangosSocket(t.timeout)
		if err != nil {
			return err
		}
		t.socket = socket
		t.current = socketDeadlines(t.timeout)
	}
	if t.endpoint != "" {
		return ErrAlreadyConnected
	}

	if err := t.socket.Dial(endpoint); err != nil {
		return &ConnectionError{Endpoint: endpoint, Op: "dial", Cause: err}
	}

	t.endpoint = endpoint
	return nil
}

func (t *MangosTransport) Request(ctx context.Context, payload []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	t.opMu.Lock()
	defer t.opMu.Unlock()

	t.stateMu.Lock()
	if t.socket == nil {
		t.stateMu.Unlock()
		return nil, ErrClosed
	}
	if t.endpoint == "" {
		t.stateMu.Unlock()
		return nil, ErrNotConnected
	}
	socket := t.socket
	endpoint := t.endpoint
	t.stateMu.Unlock()

	sendDeadline, err := effectiveDeadline(ctx, t.timeout.SendTimeout)
	if err != nil {
		return nil, err
	}
	sendDeadline = socketDeadline(sendDeadline)

	t.stateMu.Lock()
	currentSendDeadline := t.current.SendTimeout
	t.stateMu.Unlock()

	if deadlineChanged(sendDeadline, currentSendDeadline) {
		if err := socket.SetOption(mangos.OptionSendDeadline, sendDeadline); err != nil {
			return nil, &ConnectionError{Endpoint: endpoint, Op: "set send deadline", Cause: err}
		}
		t.stateMu.Lock()
		t.current.SendTimeout = sendDeadline
		t.stateMu.Unlock()
	}

	if err := socket.Send(payload); err != nil {
		t.closeSocketAfterInterruptedRequest(socket)
		return nil, &ConnectionError{Endpoint: endpoint, Op: "send", Cause: err}
	}

	if err := ctx.Err(); err != nil {
		t.closeSocketAfterInterruptedRequest(socket)
		return nil, err
	}

	recvDeadline, err := effectiveDeadline(ctx, t.timeout.RecvTimeout)
	if err != nil {
		t.closeSocketAfterInterruptedRequest(socket)
		return nil, err
	}
	recvDeadline = socketDeadline(recvDeadline)

	t.stateMu.Lock()
	currentRecvDeadline := t.current.RecvTimeout
	t.stateMu.Unlock()

	if deadlineChanged(recvDeadline, currentRecvDeadline) {
		if err := socket.SetOption(mangos.OptionRecvDeadline, recvDeadline); err != nil {
			t.closeSocketAfterInterruptedRequest(socket)
			return nil, &ConnectionError{Endpoint: endpoint, Op: "set receive deadline", Cause: err}
		}
		t.stateMu.Lock()
		t.current.RecvTimeout = recvDeadline
		t.stateMu.Unlock()
	}

	response, err := socket.Recv()
	if err != nil {
		t.closeSocketAfterInterruptedRequest(socket)
		return nil, &ConnectionError{Endpoint: endpoint, Op: "receive", Cause: err}
	}

	return response, nil
}

func (t *MangosTransport) closeSocketAfterInterruptedRequest(socket mangos.Socket) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	if t.socket == socket {
		_ = socket.Close()
		t.socket = nil
		t.endpoint = ""
		t.current = TimeoutConfig{}
	}
}

func effectiveDeadline(ctx context.Context, configured time.Duration) (time.Duration, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return configured, nil
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, context.DeadlineExceeded
	}
	if configured <= 0 || remaining < configured {
		return remaining, nil
	}

	return configured, nil
}

func socketDeadlines(timeout TimeoutConfig) TimeoutConfig {
	return TimeoutConfig{
		SendTimeout: socketDeadline(timeout.SendTimeout),
		RecvTimeout: socketDeadline(timeout.RecvTimeout),
	}
}

func socketDeadline(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return blockingDeadline
	}

	return timeout
}

func deadlineChanged(next time.Duration, current time.Duration) bool {
	if next == current {
		return false
	}
	if next < current {
		return true
	}

	diff := next - current
	return diff > deadlineUpdateThreshold
}

func (t *MangosTransport) Close() error {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	if t.socket == nil {
		return nil
	}

	err := t.socket.Close()
	t.socket = nil
	t.endpoint = ""
	if err != nil {
		return &ConnectionError{Op: "close", Cause: err}
	}

	return nil
}
