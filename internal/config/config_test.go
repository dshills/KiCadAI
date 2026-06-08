package config

import (
	"strings"
	"testing"
)

func TestResolveUsesExplicitValues(t *testing.T) {
	cfg, err := Resolve(Explicit{
		SocketPath:  "ipc:///custom.sock",
		Token:       "token",
		ClientName:  "client",
		TimeoutMS:   5000,
		Environment: []string{},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.SocketPath != "ipc:///custom.sock" {
		t.Fatalf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.Token != "token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
	if cfg.ClientName != "client" {
		t.Fatalf("ClientName = %q", cfg.ClientName)
	}
	if cfg.TimeoutMS != 5000 {
		t.Fatalf("TimeoutMS = %d", cfg.TimeoutMS)
	}
}

func TestResolveUsesEnvironment(t *testing.T) {
	cfg, err := Resolve(Explicit{
		Environment: []string{
			"KICAD_API_SOCKET=ipc:///env.sock",
			"KICAD_API_TOKEN=env-token",
			"KICAD_CLIENT_NAME=env-client",
			"KICAD_TIMEOUT_MS=7000",
		},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.SocketPath != "ipc:///env.sock" {
		t.Fatalf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
	if cfg.ClientName != "env-client" {
		t.Fatalf("ClientName = %q", cfg.ClientName)
	}
	if cfg.TimeoutMS != 7000 {
		t.Fatalf("TimeoutMS = %d", cfg.TimeoutMS)
	}
}

func TestResolveDefaults(t *testing.T) {
	cfg, err := Resolve(Explicit{Environment: []string{}})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.TimeoutMS != DefaultTimeoutMS {
		t.Fatalf("TimeoutMS = %d", cfg.TimeoutMS)
	}
	if !strings.HasPrefix(cfg.ClientName, "kicadai-go-") {
		t.Fatalf("ClientName = %q", cfg.ClientName)
	}
}

func TestResolveRejectsInvalidTimeout(t *testing.T) {
	_, err := Resolve(Explicit{
		Environment: []string{"KICAD_TIMEOUT_MS=abc"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveRejectsBlankSocketPath(t *testing.T) {
	_, err := Resolve(Explicit{
		SocketPath:  "   ",
		Environment: []string{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRedactedHidesToken(t *testing.T) {
	cfg := Config{Token: "secret"}
	redacted := cfg.Redacted()

	if redacted.Token != "<redacted>" {
		t.Fatalf("Token = %q", redacted.Token)
	}
	if cfg.Token != "secret" {
		t.Fatalf("original token changed: %q", cfg.Token)
	}
}
