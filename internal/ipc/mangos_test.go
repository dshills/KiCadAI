package ipc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewMangosTransportWithTimeout(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(time.Millisecond)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()
}

func TestMangosTransportRequiresDialBeforeRequest(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(time.Millisecond)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()

	ctx := context.Background()

	if _, err := transport.Request(ctx, []byte("request")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Request error = %v, want %v", err, ErrNotConnected)
	}
}

func TestMangosTransportCloseIsIdempotent(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(time.Millisecond)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestMangosTransportHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport, err := NewMangosTransportWithTimeout(time.Millisecond)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()

	if err := transport.Dial(ctx, "ipc:///tmp/kicad/api.sock"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial error = %v, want %v", err, context.Canceled)
	}
	if _, err := transport.Request(ctx, []byte("request")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Request error = %v, want %v", err, context.Canceled)
	}
}
