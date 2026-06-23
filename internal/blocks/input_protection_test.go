package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestReversePolarityInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("reverse_polarity_protection")
	if !ok {
		t.Fatal("missing reverse_polarity_protection")
	}
	if len(definition.Components) != 1 || definition.PCBRealization == nil {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "reverse_polarity_protection" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "series_diode") || !slices.Contains(family.ExportedPorts, "VIN_RAW") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestReversePolarityInstantiateAndRealize(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, _ := registry.GetBlock("reverse_polarity_protection")
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "reverse_polarity_protection", InstanceID: "prot1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 1 || !slices.Contains(output.Instance.Nets, "prot1_vin_protected") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 1 || countOperations(output.Operations, transactions.OpConnect) != 3 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 4, OriginYMM: 5})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 1 || realized.Components[0].Placement.XMM != 4 {
		t.Fatalf("realized = %#v", realized)
	}
}

func TestReversePolarityRejectsInvalidRatings(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "reverse_polarity_protection",
		InstanceID: "prot1",
		Params: map[string]any{
			"input_voltage":   "-5V",
			"input_current":   "2A",
			"diode_footprint": "Diode_SMD:D_SOD-323",
		},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.input_voltage") || !hasBlockIssuePath(issues, "params.input_current") || !hasBlockIssuePath(issues, "params.diode_footprint") {
		t.Fatalf("issues = %#v, want voltage/current/footprint blockers", issues)
	}
}
