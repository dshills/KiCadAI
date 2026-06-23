package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestResetProgrammingInventoryAndDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("reset_programming_header")
	if !ok {
		t.Fatal("missing reset_programming_header")
	}
	if len(definition.Components) != 4 || definition.PCBRealization == nil {
		t.Fatalf("definition = %#v", definition)
	}
	inventory := registry.Inventory()
	var family BlockFamilyInventory
	for _, candidate := range inventory.Families {
		if candidate.ID == "reset_programming_header" {
			family = candidate
			break
		}
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("inventory family = %#v", family)
	}
	if !slices.Contains(family.RequiredRoles, "reset_pullup") || !slices.Contains(family.ExportedPorts, "RESET") {
		t.Fatalf("inventory family = %#v", family)
	}
}

func TestResetProgrammingInstantiateISPAndRealize(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, _ := registry.GetBlock("reset_programming_header")
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "reset_programming_header", InstanceID: "prog1"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 3 || !slices.Contains(output.Instance.Nets, "prog1_mosi") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 3 || countOperations(output.Operations, transactions.OpConnect) != 10 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 5, OriginYMM: 6})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	if len(realized.Components) != 1 || len(realized.LocalRoutes) != 0 {
		t.Fatalf("realized = %#v", realized)
	}
}

func TestResetProgrammingInstantiateUARTWithoutSwitch(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "reset_programming_header",
		InstanceID: "prog1",
		Params: map[string]any{
			"programming_interface": "uart",
			"include_reset_switch":  false,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 2 || !slices.Contains(output.Instance.Nets, "prog1_uart_rx") {
		t.Fatalf("output instance = %#v", output.Instance)
	}
	if countOperations(output.Operations, transactions.OpAddSymbol) != 2 || countOperations(output.Operations, transactions.OpConnect) != 6 {
		t.Fatalf("operations = %#v", output.Operations)
	}
}

func TestResetProgrammingRejectsUnsupportedInterface(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "reset_programming_header",
		InstanceID: "prog1",
		Params:     map[string]any{"programming_interface": "swd"},
	})
	if !reports.HasBlockingIssue(issues) || !hasBlockIssuePath(issues, "params.programming_interface") {
		t.Fatalf("issues = %#v, want unsupported programming interface blocker", issues)
	}
}
