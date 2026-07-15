package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestCircuitPreflightReadyAndDeterministic(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	args := []string{"--request", graph, "--output", output, "circuit", "preflight"}
	first := runCircuitPreflightCLI(t, args)
	second := runCircuitPreflightCLI(t, args)
	if !reflect.DeepEqual(first, second) || !first.OK {
		t.Fatalf("preflight is not deterministic or ready: first=%#v second=%#v", first, second)
	}
	data := preflightResultData(t, first)
	if !data.ReadyForWrite || data.CapabilityProfile != "generic-circuit-v1" || data.InputContract == "" || data.Routing == nil || len(data.Gates) == 0 {
		t.Fatalf("preflight data = %#v", data)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func TestCircuitPreflightAcceptsDocumentedArgumentOrder(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	result := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", graph, "--json"})
	if !result.OK || !preflightResultData(t, result).ReadyForWrite {
		t.Fatalf("documented argument order result = %#v", result)
	}
}

func TestCircuitPreflightFailsClosedBeforeWrite(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "unsupported_unknown_component.json")
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	result := runCircuitPreflightCLI(t, []string{"--request", graph, "--output", output, "circuit", "preflight"})
	data := preflightResultData(t, result)
	if result.OK || data.ReadyForWrite || len(result.Issues) == 0 || result.Issues[0].RetryScope == "" {
		t.Fatalf("unsupported preflight = %#v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func runCircuitPreflightCLI(t *testing.T, args []string) reports.Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	_ = run(args, &stdout, &stderr)
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode preflight: %v\n%s", err, stdout.String())
	}
	return result
}

func preflightResultData(t *testing.T, result reports.Result) circuitPreflightData {
	t.Helper()
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var preflight circuitPreflightData
	if err := json.Unmarshal(data, &preflight); err != nil {
		t.Fatal(err)
	}
	return preflight
}
