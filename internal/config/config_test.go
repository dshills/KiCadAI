package config

import (
	"strings"
	"testing"
)

func TestResolveUsesExplicitValues(t *testing.T) {
	cfg, err := Resolve(Explicit{
		SocketPath:  " ipc:///custom.sock ",
		Token:       " token ",
		ClientName:  " client ",
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

func TestResolveNormalizesRawUnixSocketPath(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		want       string
	}{
		{name: "absolute", socketPath: "/tmp/kicad/api.sock", want: "ipc:///tmp/kicad/api.sock"},
		{name: "relative", socketPath: "api.sock", want: "ipc://api.sock"},
		{name: "relative with dot", socketPath: "./api.sock", want: "ipc://./api.sock"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := Resolve(Explicit{
				SocketPath:  test.socketPath,
				Environment: []string{},
				RuntimeOS:   "darwin",
			})
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}

			if cfg.SocketPath != test.want {
				t.Fatalf("SocketPath = %q, want %q", cfg.SocketPath, test.want)
			}
		})
	}
}

func TestResolvePreservesNonIPCSocketValues(t *testing.T) {
	cfg, err := Resolve(Explicit{
		SocketPath:  `\\.\pipe\kicad-api`,
		Environment: []string{},
		RuntimeOS:   "windows",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.SocketPath != `\\.\pipe\kicad-api` {
		t.Fatalf("SocketPath = %q", cfg.SocketPath)
	}
}

func TestResolveRejectsMissingWindowsSocketPath(t *testing.T) {
	_, err := Resolve(Explicit{
		Environment: []string{},
		RuntimeOS:   "windows",
	})
	if err == nil {
		t.Fatal("expected error")
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

func TestResolveRejectsNonPositiveEnvironmentTimeout(t *testing.T) {
	tests := []string{"0", "-1"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			_, err := Resolve(Explicit{
				Environment: []string{"KICAD_TIMEOUT_MS=" + value},
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestResolveRejectsNegativeExplicitTimeout(t *testing.T) {
	_, err := Resolve(Explicit{
		TimeoutMS:   -1,
		Environment: []string{},
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

func TestNormalizeSocketPathUsesCurrentPlatform(t *testing.T) {
	if NormalizeSocketPath("ipc:///tmp/kicad/api.sock") != "ipc:///tmp/kicad/api.sock" {
		t.Fatal("expected already-normalized socket to be preserved")
	}

	if NormalizeSocketPath("api.sock", "darwin") != "ipc://api.sock" {
		t.Fatal("expected target OS override to normalize relative Unix sockets")
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
