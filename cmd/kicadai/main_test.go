package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/config"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
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

func TestRunPingJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"reachable": true`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunVersionJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"version",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"reachable": true`,
		`"full_version": "9.1.0"`,
		`"major": 9`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPingJSONFailureReturnsError(t *testing.T) {
	want := errors.New("not reachable")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{pingErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"reachable": false`) {
		t.Fatalf("expected failure JSON, got %s", stdout.String())
	}
}

func TestRunDocumentsJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			documents: []kiapi.Document{{
				Type:       kiapi.DocumentTypeSchematic,
				Identifier: "/",
				SheetPath:  "/",
			}},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--document-type", "schematic",
		"--json",
		"documents",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"documents": [`,
		`"type": "schematic"`,
		`"sheet_path": "/"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunCapabilitiesJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"capabilities",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	var capabilities kiapi.Capabilities
	if err := json.Unmarshal(stdout.Bytes(), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities JSON: %v", err)
	}
	if capabilities.KiCadVersion != "9.1.0" {
		t.Fatalf("KiCadVersion = %q", capabilities.KiCadVersion)
	}
	if !capabilities.Supports(kiapi.CapabilitySchematicRead) {
		t.Fatalf("expected schematic read in supported capabilities")
	}
	for _, missing := range []kiapi.Capability{
		kiapi.CapabilitySchematicWrite,
		kiapi.CapabilitySymbolPlace,
	} {
		if capabilities.Supports(missing) {
			t.Fatalf("expected %s to be missing, got supported", missing)
		}
	}
}

func TestRunPlanLEDDemoJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--origin-x", "1000",
		"--origin-y", "2000",
		"--prefix", "STATUS",
		"--lib-resistor", "Custom:R_US",
		"--json",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"operations": [`,
		`"kind": "add_symbol"`,
		`Custom:R_US`,
		`STATUS_OUT`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

type fakeAPIClient struct {
	pingErr   error
	version   *commontypes.KiCadVersion
	documents []kiapi.Document
}

func (c *fakeAPIClient) Ping(context.Context) error {
	return c.pingErr
}

func (c *fakeAPIClient) GetVersion(context.Context) (*commontypes.KiCadVersion, error) {
	return c.version, nil
}

func (c *fakeAPIClient) GetOpenDocuments(context.Context, kiapi.DocumentType) ([]kiapi.Document, error) {
	return c.documents, nil
}

func (c *fakeAPIClient) Close() error {
	return nil
}

func appWithClientFactory(factory func(context.Context, config.Config) (apiClient, error)) app {
	return app{newClient: factory}
}
