package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDefaultsToHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRunConfigJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--token", "secret-token",
		"--client-name", "test-client",
		"--timeout-ms", "3000",
		"--json",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()

	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"timeout_ms": 3000`,
		`"token": "<redacted>"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}

	if strings.Contains(output, "secret-token") {
		t.Fatalf("token leaked in output: %s", output)
	}
}
