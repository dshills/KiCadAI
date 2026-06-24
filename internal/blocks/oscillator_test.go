package blocks

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestCrystalOscillatorInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("crystal_oscillator")
	if !ok {
		t.Fatal("missing crystal_oscillator")
	}
	if len(definition.Components) != 3 || definition.PCBRealization == nil {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "crystal_oscillator" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "crystal") || !slices.Contains(family.ExportedPorts, "XTAL1") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestCrystalOscillatorInstantiateAndRealize(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, _ := registry.GetBlock("crystal_oscillator")
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "crystal_oscillator", InstanceID: "clk1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 3 || !slices.Contains(output.Instance.Nets, "clk1_xtal1") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 3 || countOperations(output.Operations, transactions.OpConnect) != 6 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	placements := placeFootprintOperations(t, output.Operations)
	if placements[output.Instance.Refs[1]].At.XMM != -4 || placements[output.Instance.Refs[2]].At.XMM != 4 {
		t.Fatalf("placements = %#v", placements)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 20, OriginYMM: 10})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 3 || len(realized.LocalRoutes) != 3 {
		t.Fatalf("realized = %#v", realized)
	}
	if len(realized.Timing) != 1 {
		t.Fatalf("timing evidence = %#v, want one fixture", realized.Timing)
	}
	timing := realized.Timing[0]
	if timing.ID != "crystal_loop" || !timing.Satisfied || !timing.GroundReturnPresent {
		t.Fatalf("timing evidence = %#v", timing)
	}
	if timing.LoadCapacitorAsymmetryMM == nil || *timing.LoadCapacitorAsymmetryMM != 0 {
		t.Fatalf("load capacitor asymmetry = %#v", timing.LoadCapacitorAsymmetryMM)
	}
	if len(timing.ClockRouteLengthsMM) != 2 || timing.ClockRouteLengthsMM["xtal1_load"] <= 0 {
		t.Fatalf("clock route lengths = %#v", timing.ClockRouteLengthsMM)
	}
	if realized.Components[0].Placement.XMM != 20 {
		t.Fatalf("realized placement = %#v", realized.Components[0])
	}
}

func TestCrystalOscillatorRejectsUnsupportedFrequency(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "crystal_oscillator",
		InstanceID: "clk1",
		Params:     map[string]any{"frequency": "8MHz"},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.frequency") {
		t.Fatalf("issues = %#v, want unsupported frequency blocker", issues)
	}
}

func TestCrystalOscillatorRejectsUnsupportedCrystalFootprint(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "crystal_oscillator",
		InstanceID: "clk1",
		Params:     map[string]any{"crystal_footprint": "Crystal:Unverified"},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.crystal_footprint") {
		t.Fatalf("issues = %#v, want unsupported crystal footprint blocker", issues)
	}
}

func TestCrystalOscillatorNormalizesCapacitorQuery(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "crystal_oscillator",
		InstanceID: "clk1",
		Params: map[string]any{
			"load_capacitor_value": " 22 pF ",
			"capacitor_footprint":  "Capacitor_SMD:C_0402_1005Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	query := oscillatorCapacitorQuery("Capacitor_SMD:C_0402_1005Metric", normalizeCapacitanceQueryValue(" 22 PF "))
	if query.Value != "22p" || query.Package != "0402" {
		t.Fatalf("query = %#v", query)
	}
}

func hasBlockIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}

func placeFootprintOperations(t *testing.T, operations []transactions.Operation) map[string]transactions.PlaceFootprintOperation {
	t.Helper()
	placements := map[string]transactions.PlaceFootprintOperation{}
	for _, operation := range operations {
		if operation.Op != transactions.OpPlaceFootprint {
			continue
		}
		var place transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &place); err != nil {
			t.Fatalf("decode place footprint operation: %v", err)
		}
		placements[place.Ref] = place
	}
	return placements
}
