package ipc

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testTransportTimeout = 100 * time.Millisecond

func TestNewMangosTransportWithTimeout(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()
}

func TestMangosTransportRequiresDialBeforeRequest(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
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
	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()

	if err := transport.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestMangosTransportDialFailure(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()

	err = transport.Dial(testContext(t), "unsupported://endpoint")
	if err == nil {
		t.Fatalf("Dial returned nil error, want failure")
	}
	var connectionErr *ConnectionError
	if !errors.As(err, &connectionErr) {
		t.Fatalf("Dial error = %v, want ConnectionError", err)
	}
	if connectionErr.Op != "dial" {
		t.Fatalf("Dial op = %q, want dial", connectionErr.Op)
	}
	if connectionErr.Endpoint != "unsupported://endpoint" {
		t.Fatalf("Dial endpoint = %q, want unsupported://endpoint", connectionErr.Endpoint)
	}
}

func TestMangosTransportRequestAfterClose(t *testing.T) {
	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()
	if err := transport.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if _, err := transport.Request(testContext(t), []byte("request")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Request error = %v, want %v", err, ErrClosed)
	}
}

func TestMangosTransportHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport, err := NewMangosTransportWithTimeout(testTransportTimeout)
	if err != nil {
		t.Fatalf("NewMangosTransportWithTimeout returned error: %v", err)
	}
	defer transport.Close()

	if err := transport.Dial(ctx, testIPCEndpoint(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial error = %v, want %v", err, context.Canceled)
	}
	if _, err := transport.Request(ctx, []byte("request")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Request error = %v, want %v", err, context.Canceled)
	}
}

func TestConnectionErrorFormattingAndUnwrap(t *testing.T) {
	want := errors.New("boom")
	endpoint := testIPCEndpoint(t)
	err := &ConnectionError{Endpoint: endpoint, Op: "dial", Cause: want}

	if !strings.Contains(err.Error(), "dial "+endpoint) {
		t.Fatalf("error text = %q", err.Error())
	}
	if !errors.Is(err, want) {
		t.Fatalf("ConnectionError did not unwrap %v", want)
	}

	err = &ConnectionError{Op: "close", Cause: want}
	if !strings.Contains(err.Error(), "close transport") {
		t.Fatalf("error text = %q", err.Error())
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testIPCEndpoint(t *testing.T) string {
	t.Helper()

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(t.Name()))
	name := fmt.Sprintf("kicadai-%08x", hash.Sum32())
	if runtime.GOOS == "windows" {
		return `\\.\pipe\` + name
	}

	base := os.TempDir()
	if len(filepath.Join(base, name+".sock")) > 100 {
		base = "/tmp"
	}
	return "ipc://" + filepath.Join(base, name+".sock")
}

func TestEffectiveDeadline(t *testing.T) {
	if got, err := effectiveDeadline(context.Background(), 2*time.Second); err != nil || got != 2*time.Second {
		t.Fatalf("effectiveDeadline without context deadline = %s, %v", got, err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()
	got, err := effectiveDeadline(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("effectiveDeadline returned error: %v", err)
	}
	if got != 2*time.Second {
		t.Fatalf("effectiveDeadline = %s, want %s", got, 2*time.Second)
	}

	shorterCtx, cancelShorter := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancelShorter()
	got, err = effectiveDeadline(shorterCtx, 10*time.Second)
	if err != nil {
		t.Fatalf("effectiveDeadline shorter context returned error: %v", err)
	}
	if got <= 0 || got > 5*time.Second {
		t.Fatalf("effectiveDeadline shorter context = %s, want positive deadline below context deadline", got)
	}

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancelExpired()
	if _, err := effectiveDeadline(expired, 2*time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("effectiveDeadline expired error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestSocketDeadlineHelpers(t *testing.T) {
	if got := socketDeadline(0); got != blockingDeadline {
		t.Fatalf("socketDeadline(0) = %s", got)
	}
	if got := socketDeadline(time.Second); got != time.Second {
		t.Fatalf("socketDeadline(1s) = %s", got)
	}
	deadlines := socketDeadlines(TimeoutConfig{SendTimeout: 0, RecvTimeout: time.Second})
	if deadlines.SendTimeout != blockingDeadline || deadlines.RecvTimeout != time.Second {
		t.Fatalf("socketDeadlines = %+v", deadlines)
	}
}

func TestDeadlineChanged(t *testing.T) {
	tests := []struct {
		name    string
		next    time.Duration
		current time.Duration
		want    bool
	}{
		{name: "same", next: time.Second, current: time.Second, want: false},
		{name: "shorter", next: time.Second, current: 2 * time.Second, want: true},
		{name: "small increase", next: time.Second + time.Millisecond, current: time.Second, want: false},
		{name: "large increase", next: time.Second + deadlineUpdateThreshold + time.Millisecond, current: time.Second, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deadlineChanged(test.next, test.current); got != test.want {
				t.Fatalf("deadlineChanged(%s, %s) = %v, want %v", test.next, test.current, got, test.want)
			}
		})
	}
}
