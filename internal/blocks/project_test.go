package blocks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestProjectTransactionForLEDKeepsInternalConnectionsOnly(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}

	tx, err := ProjectTransactionForBlockOutput("status_led", output, false)
	if err != nil {
		t.Fatal(err)
	}

	if tx.Name != "status_led" || tx.Project != "status_led" {
		t.Fatalf("unexpected transaction identity: %#v", tx)
	}
	if len(tx.Operations) != 9 {
		t.Fatalf("expected create + 6 component ops + 1 internal connect + write, got %d", len(tx.Operations))
	}
	if tx.Operations[0].Op != transactions.OpCreateProject || tx.Operations[len(tx.Operations)-1].Op != transactions.OpWriteProject {
		t.Fatalf("transaction must be bracketed by create/write: %#v", tx.Operations)
	}
	connects := 0
	for _, operation := range tx.Operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		connects++
		var payload transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.From.Ref == output.Instance.InstanceID || payload.To.Ref == output.Instance.InstanceID {
			t.Fatalf("project transaction leaked pseudo-port connect: %#v", payload)
		}
	}
	if connects != 1 {
		t.Fatalf("expected one internal connect, got %d", connects)
	}
	if validation := transactions.Validate(tx); len(validation.Issues) != 0 {
		t.Fatalf("unexpected validation issues: %#v", validation.Issues)
	}
}

func TestProjectTransactionForConnectorFiltersPseudoPortConnections(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "header",
		Params:     map[string]any{"pin_names": []string{"VCC", "GND", "SCL", "SDA"}},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}

	tx, err := ProjectTransactionForBlockOutput("header", output, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpConnect {
			t.Fatalf("connector project transaction should not include pseudo-port connects: %#v", operation)
		}
	}
	if validation := transactions.Validate(tx); len(validation.Issues) != 0 {
		t.Fatalf("unexpected validation issues: %#v", validation.Issues)
	}
}

func TestProjectTransactionReportsMalformedConnect(t *testing.T) {
	_, err := ProjectTransactionForBlockOutput("bad", BlockOutput{
		Instance: BlockInstance{Refs: []string{"R1", "D1"}},
		Operations: []transactions.Operation{
			{Op: transactions.OpConnect},
		},
	}, false)
	if err == nil {
		t.Fatal("expected malformed connect error")
	}
}

func TestProjectTransactionRejectsDuplicateBlockRefs(t *testing.T) {
	_, err := ProjectTransactionForBlockOutput("bad", BlockOutput{
		Instance: BlockInstance{InstanceID: "status", Refs: []string{"R1", "R1"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate generated reference R1") {
		t.Fatalf("expected duplicate ref error, got %v", err)
	}
}

func TestProjectTransactionRejectsDuplicateCompositionRefs(t *testing.T) {
	_, err := ProjectTransactionForCompositionOutput("bad", CompositionOutput{
		Instances: []BlockInstance{
			{InstanceID: "left", Refs: []string{"R1"}},
			{InstanceID: "right", Refs: []string{"R1"}},
		},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "left and right") {
		t.Fatalf("expected duplicate composition ref error, got %v", err)
	}
}

func TestProjectTransactionAppliesBlockOutput(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("status_led", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "status_led")

	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected apply issues: %#v", result.Issues)
	}
	for _, name := range []string{"status_led.kicad_pro", "status_led.kicad_sch", "status_led.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestProjectTransactionAppliesCompositionOutput(t *testing.T) {
	registry := NewBuiltinRegistry()
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		ProjectName: "composed",
		Instances: []CompositionInstance{
			{ID: "status", BlockID: "led_indicator"},
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"SIG", "GND"}}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "header", Port: "SIG"}, To: PortRef{InstanceID: "status", Port: "IN"}, NetAlias: "LED_EN"},
		},
	})
	issues := output.Issues
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("compose issues: %#v", issues)
	}
	tx, err := ProjectTransactionForCompositionOutput("composed", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "composed")

	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected apply issues: %#v", result.Issues)
	}
	for _, name := range []string{"composed.kicad_pro", "composed.kicad_sch", "composed.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
