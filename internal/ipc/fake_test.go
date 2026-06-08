package ipc

import (
	"context"
	"errors"
	"testing"
)

func TestFakeTransportRoundTrip(t *testing.T) {
	transport := &FakeTransport{}
	transport.QueueResponse([]byte("response"))

	ctx := context.Background()

	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	if err := transport.Send(ctx, []byte("request")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	response, err := transport.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv returned error: %v", err)
	}

	if transport.DialedEndpoint() != "ipc:///tmp/kicad/api.sock" {
		t.Fatalf("DialedEndpoint = %q", transport.DialedEndpoint())
	}
	sent := transport.SentMessages()
	if string(sent[0]) != "request" {
		t.Fatalf("Sent[0] = %q", string(sent[0]))
	}
	if string(response) != "response" {
		t.Fatalf("response = %q", string(response))
	}
}

func TestFakeTransportRequest(t *testing.T) {
	transport := &FakeTransport{}
	transport.QueueResponse([]byte("response"))
	ctx := context.Background()

	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	response, err := transport.Request(ctx, []byte("request"))
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}

	if string(response) != "response" {
		t.Fatalf("response = %q", string(response))
	}
	sent := transport.SentMessages()
	if string(sent[0]) != "request" {
		t.Fatalf("Sent[0] = %q", string(sent[0]))
	}
}

func TestFakeTransportCopiesPayloads(t *testing.T) {
	request := []byte("request")
	response := []byte("response")
	transport := &FakeTransport{}
	transport.QueueResponse(response)
	if err := transport.Dial(context.Background(), "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	ctx := context.Background()

	if err := transport.Send(ctx, request); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	request[0] = 'R'

	got, err := transport.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv returned error: %v", err)
	}
	response[0] = 'R'

	sent := transport.SentMessages()
	if string(sent[0]) != "request" {
		t.Fatalf("stored request was mutated: %q", string(sent[0]))
	}
	if string(got) != "response" {
		t.Fatalf("returned response was mutated: %q", string(got))
	}
}

func TestFakeTransportErrors(t *testing.T) {
	want := errors.New("boom")
	transport := &FakeTransport{}
	transport.SetSendError(want)
	ctx := context.Background()
	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	if err := transport.Send(ctx, []byte("request")); !errors.Is(err, want) {
		t.Fatalf("Send error = %v, want %v", err, want)
	}

	transport = &FakeTransport{}
	transport.SetRecvError(want)
	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	if _, err := transport.Recv(ctx); !errors.Is(err, want) {
		t.Fatalf("Recv error = %v, want %v", err, want)
	}
}

func TestFakeTransportStateErrors(t *testing.T) {
	transport := &FakeTransport{}
	ctx := context.Background()

	if err := transport.Send(ctx, []byte("request")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Send error = %v, want %v", err, ErrNotConnected)
	}
	if _, err := transport.Recv(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Recv error = %v, want %v", err, ErrNotConnected)
	}

	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	if err := transport.Dial(ctx, "ipc:///tmp/kicad/other.sock"); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("Dial error = %v, want %v", err, ErrAlreadyConnected)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Dial error = %v, want %v", err, ErrClosed)
	}
}

func TestFakeTransportNoResponsesError(t *testing.T) {
	transport := &FakeTransport{}
	if err := transport.Dial(context.Background(), "ipc:///tmp/kicad/api.sock"); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	if _, err := transport.Recv(context.Background()); !errors.Is(err, ErrNoResponses) {
		t.Fatalf("Recv error = %v, want %v", err, ErrNoResponses)
	}
}

func TestFakeTransportHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport := &FakeTransport{}

	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial error = %v, want %v", err, context.Canceled)
	}
	if err := transport.Send(ctx, []byte("request")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want %v", err, context.Canceled)
	}
	if _, err := transport.Recv(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv error = %v, want %v", err, context.Canceled)
	}
}
