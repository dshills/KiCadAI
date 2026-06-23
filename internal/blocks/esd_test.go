package blocks

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestESDProtectionInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("esd_protection")
	if !ok {
		t.Fatal("missing esd_protection")
	}
	if len(definition.Components) != 1 || definition.PCBRealization == nil {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "esd_protection" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "tvs") || !slices.Contains(family.ExportedPorts, "SIGNAL") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestESDProtectionInstantiateAndRealize(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, _ := registry.GetBlock("esd_protection")
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "esd_protection", InstanceID: "esd1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 1 || !slices.Contains(output.Instance.Nets, "esd1_signal") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 1 || countOperations(output.Operations, transactions.OpConnect) != 2 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 2, OriginYMM: 3})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 1 || realized.Components[0].Placement.XMM != 2 {
		t.Fatalf("realized = %#v", realized)
	}
}

func TestESDProtectionRejectsUnsupportedVoltageAndFootprint(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "esd_protection",
		InstanceID: "esd1",
		Params: map[string]any{
			"working_voltage": "12V",
			"tvs_footprint":   "Diode_SMD:D_SMA",
		},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.working_voltage") || !hasBlockIssuePath(issues, "params.tvs_footprint") {
		t.Fatalf("issues = %#v, want voltage and footprint blockers", issues)
	}
}

func TestESDProtectionLabelsVerifiedWorkingVoltage(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "esd_protection",
		InstanceID: "esd1",
		Params:     map[string]any{"working_voltage": "5V"},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	symbols := addSymbolOperations(t, output.Operations)
	if symbols[output.Instance.Refs[0]].Value != "5V TVS" {
		t.Fatalf("symbol = %#v", symbols[output.Instance.Refs[0]])
	}
}

func addSymbolOperations(t *testing.T, operations []transactions.Operation) map[string]transactions.AddSymbolOperation {
	t.Helper()
	symbols := map[string]transactions.AddSymbolOperation{}
	for _, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var symbol transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &symbol); err != nil {
			t.Fatalf("decode add symbol operation: %v", err)
		}
		symbols[symbol.Ref] = symbol
	}
	return symbols
}
