package placement

import (
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestPlacementOperationPayload(t *testing.T) {
	component := Component{
		Ref:         "R1",
		Value:       "10k",
		FootprintID: "Resistor:R_0603",
		Pads:        []PadSummary{{Name: "1", XMM: -0.5, WidthMM: 0.4, HeightMM: 0.6}},
	}
	placement := PlacementResult{
		Ref:      "R1",
		Position: Placement{XMM: 12, YMM: 7, RotationDeg: 90, Layer: "F.Cu"},
	}

	operation, err := PlacementOperation(component, placement)
	if err != nil {
		t.Fatalf("PlacementOperation returned error: %v", err)
	}
	var payload transactions.PlaceFootprintOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatalf("unmarshal operation payload: %v", err)
	}
	if payload.Op != transactions.OpPlaceFootprint || payload.Ref != "R1" || payload.FootprintID != "Resistor:R_0603" {
		t.Fatalf("payload identity = %#v", payload)
	}
	if payload.At.XMM != 12 || payload.At.YMM != 7 || payload.Rotation != 90 || payload.Layer != "F.Cu" {
		t.Fatalf("payload placement = %#v", payload)
	}
	if len(payload.Pads) != 1 || payload.Pads[0].Name != "1" || payload.Pads[0].WidthMM != 0.4 {
		t.Fatalf("payload pads = %#v", payload.Pads)
	}
}

func TestPlacementOperationAuditPadNetEvidenceDropped(t *testing.T) {
	// This documents the Phase 1 blocker from
	// specs/generated-design-net-assignment/PLAN.md: placement pads carry net
	// evidence, but the transaction payload currently drops it.
	component := Component{
		Ref:         "D1",
		Value:       "LED",
		FootprintID: "LED_SMD:LED_0805_2012Metric",
		Pads: []PadSummary{
			{Name: "1", Net: "LED_K", XMM: -0.6, WidthMM: 0.7, HeightMM: 0.8},
			{Name: "2", Net: "LED_A", XMM: 0.6, WidthMM: 0.7, HeightMM: 0.8},
		},
	}
	placement := PlacementResult{
		Ref:      "D1",
		Position: Placement{XMM: 12, YMM: 7, Layer: "F.Cu"},
	}

	operation, err := PlacementOperation(component, placement)
	if err != nil {
		t.Fatalf("PlacementOperation returned error: %v", err)
	}
	var payload transactions.PlaceFootprintOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatalf("unmarshal operation payload: %v", err)
	}
	if payload.Ref != component.Ref || payload.At.XMM != placement.Position.XMM || payload.At.YMM != placement.Position.YMM || payload.Layer != placement.Position.Layer {
		t.Fatalf("payload identity/placement = %#v", payload)
	}
	if len(payload.Pads) != len(component.Pads) {
		t.Fatalf("payload pads = %d, want %d", len(payload.Pads), len(component.Pads))
	}
	for index, pad := range payload.Pads {
		if pad.Net != nil {
			t.Errorf("pad %d %q expected current placement operation to drop pad net evidence, got %q", index, pad.Name, *pad.Net)
		}
	}
}

func TestPlaceEmitsOperationsForSuccessfulPlacements(t *testing.T) {
	req := minimalRequest()

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if len(result.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(result.Operations))
	}
}

func TestPlacementOperationsSkipsUnplacedResults(t *testing.T) {
	req := minimalRequest()
	operations, issues := PlacementOperations(req, []PlacementResult{{Ref: "R1", Reason: "blocked"}})
	if len(issues) != 0 {
		t.Fatalf("PlacementOperations returned issues: %#v", issues)
	}
	if len(operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(operations))
	}
}

func TestPlacementOperationsRejectsUnknownPlacementRef(t *testing.T) {
	req := minimalRequest()

	operations, issues := PlacementOperations(req, []PlacementResult{{Ref: "R404", FootprintID: "Device:R"}})
	if len(operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(operations))
	}
	assertIssueContains(t, issues, "placement component not found in request")
}

func TestPlacementOperationsRejectsMissingFootprintID(t *testing.T) {
	req := minimalRequest()
	req.Components[0].FootprintID = ""

	operations, issues := PlacementOperations(req, []PlacementResult{{Ref: "R1"}})
	if len(operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(operations))
	}
	assertIssueContains(t, issues, "placement component missing footprint ID")
}

func TestPlacementOperationsRejectsDuplicateComponentRefs(t *testing.T) {
	req := minimalRequest()
	duplicate := req.Components[0]
	duplicate.Ref = " r1 "
	req.Components = append(req.Components, duplicate)

	operations, issues := PlacementOperations(req, []PlacementResult{{Ref: "R1", FootprintID: "Device:R"}})
	if len(operations) != 1 {
		t.Fatalf("operations = %d, want operation for first unique component", len(operations))
	}
	assertIssueContains(t, issues, "duplicate component reference")
}

func TestPlacementOperationUsesCanonicalComponentRef(t *testing.T) {
	component := Component{Ref: "R1", FootprintID: "Device:R"}
	placement := PlacementResult{Ref: " r1 ", Position: Placement{XMM: 1, YMM: 2}}

	operation, err := PlacementOperation(component, placement)
	if err != nil {
		t.Fatalf("PlacementOperation returned error: %v", err)
	}
	var payload transactions.PlaceFootprintOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		t.Fatalf("unmarshal operation payload: %v", err)
	}
	if payload.Ref != "R1" {
		t.Fatalf("payload ref = %q, want canonical component ref", payload.Ref)
	}
}

func TestPlacementOperationsRejectsDuplicatePlacementRefs(t *testing.T) {
	req := minimalRequest()
	placement := PlacementResult{Ref: "R1", FootprintID: "Device:R"}

	operations, issues := PlacementOperations(req, []PlacementResult{placement, placement})
	if len(operations) != 1 {
		t.Fatalf("operations = %d, want first operation only", len(operations))
	}
	assertIssueContains(t, issues, "duplicate placement reference")
}
