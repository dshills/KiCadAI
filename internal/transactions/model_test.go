package transactions

import (
	"encoding/json"
	"testing"
)

func TestOperationRefMetadataFromUnmarshal(t *testing.T) {
	var operation Operation
	if err := json.Unmarshal([]byte(`{"op":"place_footprint","ref":"R1","footprint_id":"Resistor:R_0805","at":{"x_mm":1,"y_mm":2}}`), &operation); err != nil {
		t.Fatal(err)
	}
	if operation.Op != OpPlaceFootprint || operation.Ref != "R1" {
		t.Fatalf("operation metadata = %#v", operation)
	}
}

func TestNewOperationWrapsRawPayload(t *testing.T) {
	raw := json.RawMessage(`{"op":"assign_footprint","ref":"U1","footprint_id":"Package:QFN"}`)
	operation := NewOperation(OpAssignFootprint, raw)
	if operation.Ref != "" {
		t.Fatalf("operation ref = %q, want empty metadata", operation.Ref)
	}
	var payload map[string]any
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["op"] != string(OpAssignFootprint) {
		t.Fatalf("raw operation op = %#v", payload["op"])
	}
	raw[0] = '['
	if len(operation.Raw) == 0 || operation.Raw[0] != '{' {
		t.Fatalf("operation raw was not copied: %q", string(operation.Raw))
	}
}

func TestNewOperationWithRefPopulatesRefMetadata(t *testing.T) {
	raw := json.RawMessage(`{"op":"assign_footprint","ref":"U1","footprint_id":"Package:QFN"}`)
	operation := NewOperationWithRef(OpAssignFootprint, raw, "U1")
	if operation.Ref != "U1" {
		t.Fatalf("operation ref = %q, want U1", operation.Ref)
	}
	var payload map[string]any
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["op"] != string(OpAssignFootprint) {
		t.Fatalf("raw operation op = %#v", payload["op"])
	}
	raw[0] = '['
	if len(operation.Raw) == 0 || operation.Raw[0] != '{' {
		t.Fatalf("operation raw was not copied: %q", string(operation.Raw))
	}
}
