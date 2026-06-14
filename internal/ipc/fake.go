package ipc

import (
	"context"
	"sync"
)

type FakeTransport struct {
	mu             sync.Mutex
	dialedEndpoint string
	sent           [][]byte
	responses      [][]byte

	dialError  error
	sendError  error
	recvError  error
	closeError error
	closed     bool
	afterSend  func()
}

func (t *FakeTransport) QueueResponse(response []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.responses = append(t.responses, append([]byte(nil), response...))
}

func (t *FakeTransport) SetDialError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.dialError = err
}

func (t *FakeTransport) SetSendError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sendError = err
}

func (t *FakeTransport) SetRecvError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.recvError = err
}

func (t *FakeTransport) SetCloseError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closeError = err
}

func (t *FakeTransport) DialedEndpoint() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.dialedEndpoint
}

func (t *FakeTransport) SentMessages() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	messages := make([][]byte, len(t.sent))
	for i, message := range t.sent {
		messages[i] = append([]byte(nil), message...)
	}
	return messages
}

func (t *FakeTransport) Dial(ctx context.Context, endpoint string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}
	if t.dialedEndpoint != "" {
		return ErrAlreadyConnected
	}
	if t.dialError != nil {
		return t.dialError
	}

	t.dialedEndpoint = endpoint
	return nil
}

func (t *FakeTransport) Send(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}
	if t.dialedEndpoint == "" {
		return ErrNotConnected
	}
	if t.sendError != nil {
		return t.sendError
	}

	copied := append([]byte(nil), payload...)
	t.sent = append(t.sent, copied)
	return nil
}

func (t *FakeTransport) Recv(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrClosed
	}
	if t.dialedEndpoint == "" {
		return nil, ErrNotConnected
	}
	if t.recvError != nil {
		return nil, t.recvError
	}
	if len(t.responses) == 0 {
		return nil, ErrNoResponses
	}

	response := append([]byte(nil), t.responses[0]...)
	t.responses[0] = nil
	t.responses = t.responses[1:]
	return response, nil
}

func (t *FakeTransport) Request(ctx context.Context, payload []byte) ([]byte, error) {
	if err := t.Send(ctx, payload); err != nil {
		return nil, err
	}
	if t.afterSend != nil {
		t.afterSend()
	}

	return t.recvAfterSend()
}

func (t *FakeTransport) recvAfterSend() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrClosed
	}
	if t.dialedEndpoint == "" {
		return nil, ErrNotConnected
	}
	if t.recvError != nil {
		return nil, t.recvError
	}
	if len(t.responses) == 0 {
		return nil, ErrNoResponses
	}

	response := append([]byte(nil), t.responses[0]...)
	t.responses[0] = nil
	t.responses = t.responses[1:]
	return response, nil
}

func (t *FakeTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	return t.closeError
}
