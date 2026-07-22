package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"kicadai/internal/generationcapability"
	"kicadai/internal/reports"
)

func TestCapabilityGenerationEmitsSharedMachineReadableContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"capability", "generation", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("run capability generation: %v; stderr=%s", err, stderr.String())
	}
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.OK || result.Command != "capability" {
		t.Fatalf("result = %#v", result)
	}
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var document generationcapability.Document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := generationcapability.Lookup(generationcapability.ProfileGenericCircuit); !ok || !json.Valid(document.GenericGraphContract) {
		t.Fatalf("document = %#v", document)
	}
	if document.FunctionLevelContract.Schema != "kicadai.function-level-capabilities.v1" || len(document.FunctionLevelContract.Operations) == 0 {
		t.Fatalf("missing function-level contract: %#v", document.FunctionLevelContract)
	}
}

func TestCapabilityGenerationFailsClosedForUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"capability", "unknown"}, &stdout, &stderr); err == nil {
		t.Fatal("expected failure")
	}
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.OK || len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("result = %#v", result)
	}
}
