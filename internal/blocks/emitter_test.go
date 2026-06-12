package blocks

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/transactions"
)

func TestReferenceAllocatorIsDeterministicAndPrefixScoped(t *testing.T) {
	allocator := NewReferenceAllocator()
	if got := []string{allocator.Next("R"), allocator.Next("c"), allocator.Next("R"), allocator.Next("12-?")}; got[0] != "R1" || got[1] != "C1" || got[2] != "R2" || got[3] != "U1" {
		t.Fatalf("refs = %#v", got)
	}
}

func TestInstanceNetNameIsDeterministic(t *testing.T) {
	if got := InstanceNetName("led 1", "signal-out"); got != "led_1_signal_out" {
		t.Fatalf("net = %q", got)
	}
	if got := InstanceNetName("", "VCC"); got != "VCC" {
		t.Fatalf("net = %q", got)
	}
}

func TestComponentOperationsEmitStableTransactionJSON(t *testing.T) {
	component := BlockComponent{
		Role:        "resistor",
		RefPrefix:   "R",
		Value:       "1k",
		SymbolID:    "Device:R",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
	}
	operations, issues := ComponentOperations(component, "R1", transactions.Point{XMM: 10, YMM: 20})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	tx := transactions.Transaction{Operations: operations}
	validation := transactions.Validate(tx)
	if len(validation.Issues) != 0 {
		t.Fatalf("validation issues = %#v", validation.Issues)
	}
	data, err := json.Marshal(operations)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	got := string(data)
	for _, want := range []string{`"op":"add_symbol"`, `"ref":"R1"`, `"library_id":"Device:R"`, `"op":"assign_footprint"`, `"op":"place_footprint"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("operation JSON missing %q: %s", want, got)
		}
	}
}

func TestComponentOperationsReportUnsupportedNeeds(t *testing.T) {
	_, issues := ComponentOperations(BlockComponent{Role: "missing"}, "", transactions.Point{})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestConnectOperationValidatesThroughTransactions(t *testing.T) {
	operation, issues := ConnectOperation("R1", "1", "D1", "2", "LED_A")
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	tx := transactions.Transaction{Operations: []transactions.Operation{operation}}
	validation := transactions.Validate(tx)
	if len(validation.Issues) != 0 {
		t.Fatalf("validation issues = %#v", validation.Issues)
	}
}

func TestDryRunBlockOutputCarriesPortsAndOperations(t *testing.T) {
	definition := minimalDefinition()
	operation, issues := ConnectOperation("R1", "1", "D1", "2", "STATUS")
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	output := dryRunBlockOutput(definition, BlockRequest{BlockID: definition.ID, InstanceID: "status"}, []transactions.Operation{operation}, nil)
	if output.Instance.InstanceID != "status" || len(output.Instance.Ports) != len(definition.Ports) || len(output.Operations) != 1 {
		t.Fatalf("output = %#v", output)
	}
}
