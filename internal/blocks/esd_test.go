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
	if len(realized.EntryAnchors) != 2 {
		t.Fatalf("entry anchors = %#v, want signal and ground anchors", realized.EntryAnchors)
	}
	if len(realized.LocalRoutes) != 2 {
		t.Fatalf("local routes = %#v, want signal and ground route evidence", realized.LocalRoutes)
	}
	routeIDs := map[string]bool{}
	for _, route := range realized.LocalRoutes {
		routeIDs[route.ID] = true
		if route.LengthMM <= 0 {
			t.Fatalf("realized route %s length = %f, want calculated positive length", route.ID, route.LengthMM)
		}
	}
	if !routeIDs["esd_signal_entry_to_tvs"] || !routeIDs["esd_tvs_to_ground"] {
		t.Fatalf("route IDs = %#v", routeIDs)
	}
	for _, unsupported := range definition.PCBRealization.UnsupportedBehaviors {
		if unsupported == "route-through connector ordering is advisory until ordered net segments are modeled" ||
			unsupported == "external connector entry-point proximity is advisory until entry anchors are modeled" {
			t.Fatalf("unexpected stale unsupported behavior: %q", unsupported)
		}
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
