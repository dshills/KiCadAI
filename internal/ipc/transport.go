package ipc

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAlreadyConnected = errors.New("transport is already connected")
	ErrNoResponses      = errors.New("fake transport has no queued responses")
	ErrNotConnected     = errors.New("transport is not connected")
	ErrClosed           = errors.New("transport is closed")
)

type Transport interface {
	Dial(context.Context, string) error
	Request(context.Context, []byte) ([]byte, error)
	Close() error
}

type ConnectionError struct {
	Endpoint string
	Op       string
	Cause    error
}

func (e *ConnectionError) Error() string {
	if e.Endpoint == "" {
		return fmt.Sprintf("%s transport: %v", e.Op, e.Cause)
	}

	return fmt.Sprintf("%s %s: %v", e.Op, e.Endpoint, e.Cause)
}

func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

type TimeoutConfig struct {
	SendTimeout time.Duration
	RecvTimeout time.Duration
}
