package ipc

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestFakeTransportRoundTrip(t *testing.T) {
	transport := &FakeTransport{}
	transport.QueueResponse([]byte("response"))

	ctx := testContext(t)
	endpoint := testIPCEndpoint(t)

	if err := transport.Dial(ctx, endpoint); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	if err := transport.Send(ctx, []byte("request")); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	response, err := transport.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv returned error: %v", err)
	}

	if transport.DialedEndpoint() != endpoint {
		t.Fatalf("DialedEndpoint = %q, want %q", transport.DialedEndpoint(), endpoint)
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
	ctx := testContext(t)

	if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
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
	ctx := testContext(t)
	if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

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
	if !bytes.Equal(sent[0], []byte("request")) {
		t.Fatalf("stored request was mutated or incorrect: %q", string(sent[0]))
	}
	if !bytes.Equal(got, []byte("response")) {
		t.Fatalf("returned response was mutated or incorrect: %q", string(got))
	}
}

func TestFakeTransportErrors(t *testing.T) {
	want := errors.New("boom")

	t.Run("dial", func(t *testing.T) {
		transport := &FakeTransport{}
		transport.SetDialError(want)
		if err := transport.Dial(testContext(t), testIPCEndpoint(t)); !errors.Is(err, want) {
			t.Fatalf("Dial error = %v, want %v", err, want)
		}
	})

	t.Run("send", func(t *testing.T) {
		transport := &FakeTransport{}
		transport.SetSendError(want)
		ctx := testContext(t)
		if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
			t.Fatalf("Dial returned error: %v", err)
		}

		if err := transport.Send(ctx, []byte("request")); !errors.Is(err, want) {
			t.Fatalf("Send error = %v, want %v", err, want)
		}
	})

	t.Run("recv", func(t *testing.T) {
		transport := &FakeTransport{}
		transport.SetRecvError(want)
		ctx := testContext(t)
		if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
			t.Fatalf("Dial returned error: %v", err)
		}
		if _, err := transport.Recv(ctx); !errors.Is(err, want) {
			t.Fatalf("Recv error = %v, want %v", err, want)
		}
	})

	t.Run("close", func(t *testing.T) {
		transport := &FakeTransport{}
		transport.SetCloseError(want)
		if err := transport.Close(); !errors.Is(err, want) {
			t.Fatalf("Close error = %v, want %v", err, want)
		}
	})
}

func TestFakeTransportRequestPropagatesSendAndRecvErrors(t *testing.T) {
	want := errors.New("boom")

	t.Run("send", func(t *testing.T) {
		ctx := testContext(t)
		transport := &FakeTransport{}
		transport.SetSendError(want)
		if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
			t.Fatalf("Dial returned error: %v", err)
		}
		if _, err := transport.Request(ctx, []byte("request")); !errors.Is(err, want) {
			t.Fatalf("Request send error = %v, want %v", err, want)
		}
	})

	t.Run("recv", func(t *testing.T) {
		ctx := testContext(t)
		transport := &FakeTransport{}
		transport.SetRecvError(want)
		if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
			t.Fatalf("Dial returned error: %v", err)
		}
		if _, err := transport.Request(ctx, []byte("request")); !errors.Is(err, want) {
			t.Fatalf("Request recv error = %v, want %v", err, want)
		}
	})
}

func TestFakeTransportStateErrors(t *testing.T) {
	transport := &FakeTransport{}
	ctx := testContext(t)
	endpoint := testIPCEndpoint(t)

	if err := transport.Send(ctx, []byte("request")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Send error = %v, want %v", err, ErrNotConnected)
	}
	if _, err := transport.Recv(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Recv error = %v, want %v", err, ErrNotConnected)
	}

	if err := transport.Dial(ctx, endpoint); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	if err := transport.Dial(ctx, testIPCEndpoint(t)); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("Dial error = %v, want %v", err, ErrAlreadyConnected)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := transport.Dial(ctx, endpoint); !errors.Is(err, ErrClosed) {
		t.Fatalf("Dial error = %v, want %v", err, ErrClosed)
	}
}

func TestFakeTransportNoResponsesError(t *testing.T) {
	transport := &FakeTransport{}
	ctx := testContext(t)
	if err := transport.Dial(ctx, testIPCEndpoint(t)); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}

	if _, err := transport.Recv(ctx); !errors.Is(err, ErrNoResponses) {
		t.Fatalf("Recv error = %v, want %v", err, ErrNoResponses)
	}
}

func TestFakeTransportHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport := &FakeTransport{}

	if err := transport.Dial(ctx, testIPCEndpoint(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial error = %v, want %v", err, context.Canceled)
	}
	if err := transport.Send(ctx, []byte("request")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want %v", err, context.Canceled)
	}
	if _, err := transport.Recv(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv error = %v, want %v", err, context.Canceled)
	}
}
