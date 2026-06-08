//go:build integration

package kiapi

import (
	"context"
	"os"
	"testing"
	"time"

	"kicadai/internal/config"
)

func TestIntegrationPingVersionAndDocuments(t *testing.T) {
	socket := os.Getenv(config.EnvSocketPath)
	if socket == "" {
		t.Skipf("%s is required for integration tests", config.EnvSocketPath)
	}

	cfg, err := config.Resolve(config.Explicit{Environment: os.Environ()})
	if err != nil {
		t.Fatalf("resolving config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("connecting to KiCad: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if version, err := client.GetVersion(ctx); err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	} else if version == nil {
		t.Fatalf("GetVersion returned nil version")
	}
	if _, err := client.GetOpenDocuments(ctx, DocumentTypeUnknown); err != nil {
		t.Fatalf("GetOpenDocuments returned error: %v", err)
	}
}
