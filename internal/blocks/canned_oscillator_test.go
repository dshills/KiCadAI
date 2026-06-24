package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestCannedOscillatorInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("canned_oscillator")
	if !ok {
		t.Fatal("missing canned_oscillator")
	}
	if len(definition.Components) != 3 || len(definition.Ports) != 4 {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "canned_oscillator" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "oscillator") || !slices.Contains(family.ExportedPorts, "CLK_OUT") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestCannedOscillatorInstantiate(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "canned_oscillator", InstanceID: "clk1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 3 || !slices.Contains(output.Instance.Nets, "clk1_clk_out") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 3 || countOperations(output.Operations, transactions.OpConnect) != 8 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	placements := placeFootprintOperations(t, output.Operations)
	if placements[output.Instance.Refs[1]].At.XMM != 4 || placements[output.Instance.Refs[1]].At.YMM != -2 || placements[output.Instance.Refs[2]].At.XMM != 4 {
		t.Fatalf("placements = %#v", placements)
	}
}

func TestOscillatorFourPinPinsMatch7050Pitch(t *testing.T) {
	pins := oscillatorFourPinPins()
	if pins[0].XMM != -2.54 || pins[0].YMM != -1.8 || pins[2].XMM != 2.54 || pins[2].YMM != 1.8 {
		t.Fatalf("pins = %#v", pins)
	}
}

func TestCannedOscillatorRejectsUnsupportedFrequency(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "canned_oscillator",
		InstanceID: "clk1",
		Params:     map[string]any{"frequency": "8MHz"},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.frequency") {
		t.Fatalf("issues = %#v, want unsupported frequency blocker", issues)
	}
}

func TestCannedOscillatorRejectsUnsupportedPinMapOverride(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "canned_oscillator",
		InstanceID: "clk1",
		Params: map[string]any{
			"oscillator_symbol":    "Oscillator:Other",
			"oscillator_footprint": "Oscillator:Other_Footprint",
		},
	})
	if !reports.HasBlockingIssue(issues) ||
		!hasBlockIssuePath(issues, "params.oscillator_symbol") ||
		!hasBlockIssuePath(issues, "params.oscillator_footprint") {
		t.Fatalf("issues = %#v, want unsupported symbol and footprint blockers", issues)
	}
}
