package blocks

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestDCBlockingCapacitorInstantiatesWithDefaults(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "dc_blocking_capacitor",
		InstanceID: "coup",
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if got := output.Instance.Params["capacitance"]; got != "220uF" {
		t.Fatalf("capacitance param = %#v, want default 220uF", got)
	}
	if len(output.Instance.Refs) != 1 {
		t.Fatalf("refs = %#v, want one capacitor ref", output.Instance.Refs)
	}
	if countDCBlockOperations(output.Operations, transactions.OpAddSymbol) != 1 {
		t.Fatalf("operations = %#v, want one add symbol", output.Operations)
	}
	if countDCBlockOperations(output.Operations, transactions.OpConnect) != 2 {
		t.Fatalf("operations = %#v, want two external port connects", output.Operations)
	}
	if len(output.Instance.Nets) != 2 || output.Instance.Nets[0] != "coup_in" || output.Instance.Nets[1] != "coup_out" {
		t.Fatalf("nets = %#v, want coup_in/coup_out", output.Instance.Nets)
	}
}

func TestDCBlockingCapacitorPropagatesCustomParameters(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "dc_blocking_capacitor",
		InstanceID: "coup",
		Params: map[string]any{
			"capacitance":         "100nF",
			"polarized":           false,
			"capacitor_footprint": "Capacitor_SMD:C_0603_1608Metric",
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	addSymbol := requireDCBlockOperation[transactions.AddSymbolOperation](t, output.Operations, transactions.OpAddSymbol)
	if addSymbol.Value != "100nF" || addSymbol.LibraryID != "Device:C" {
		t.Fatalf("add symbol = %#v, want non-polarized 100nF capacitor", addSymbol)
	}
	assignFootprint := requireDCBlockOperation[transactions.AssignFootprintOperation](t, output.Operations, transactions.OpAssignFootprint)
	if assignFootprint.FootprintID != "Capacitor_SMD:C_0603_1608Metric" {
		t.Fatalf("assign footprint = %#v", assignFootprint)
	}
}

func TestDCBlockingCapacitorRejectsResistanceValue(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "dc_blocking_capacitor",
		InstanceID: "bad_coup",
		Params: map[string]any{
			"capacitance": "32Ω",
		},
	})
	if len(issues) == 0 {
		t.Fatal("expected invalid capacitance issue")
	}
	if len(output.Operations) != 0 {
		t.Fatalf("invalid capacitance emitted operations: %#v", output.Operations)
	}
}

func TestDCBlockingCapacitorReversePolaritySwapsPins(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "dc_blocking_capacitor",
		InstanceID: "input_coup",
		Params: map[string]any{
			"reverse_polarity": true,
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	connects := dcBlockConnects(t, output.Operations)
	if len(connects) != 2 {
		t.Fatalf("connects = %#v, want two connect operations", connects)
	}
	if connects[0].From.Ref != "input_coup" || connects[0].From.Pin != "IN" || connects[0].To.Pin != "2" {
		t.Fatalf("input connect = %#v, want IN to capacitor pin 2", connects[0])
	}
	if connects[1].From.Pin != "1" || connects[1].To.Ref != "input_coup" || connects[1].To.Pin != "OUT" {
		t.Fatalf("output connect = %#v, want capacitor pin 1 to OUT", connects[1])
	}
}

func requireDCBlockOperation[T any](t *testing.T, operations []transactions.Operation, kind transactions.OperationKind) T {
	t.Helper()
	for _, operation := range operations {
		if operation.Op != kind {
			continue
		}
		var payload T
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", kind, err)
		}
		return payload
	}
	t.Fatalf("missing operation %s in %#v", kind, operations)
	var zero T
	return zero
}

func dcBlockConnects(t *testing.T, operations []transactions.Operation) []transactions.ConnectOperation {
	t.Helper()
	var connects []transactions.ConnectOperation
	for _, operation := range operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("unmarshal connect: %v", err)
		}
		connects = append(connects, payload)
	}
	return connects
}

func countDCBlockOperations(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}
