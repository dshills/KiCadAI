package designworkflow

import (
	"testing"

	"kicadai/internal/placement"
)

func TestDiscoverPhysicalEndpointsFromPlacedPads(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{{
				Ref:         "J1",
				Role:        "connector",
				FootprintID: "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical",
				Pads: []placement.PadSummary{
					{Name: "1", Net: "VIN", XMM: -1, YMM: 0, WidthMM: 1, HeightMM: 1},
					{Name: "2", Net: "GND", XMM: 1, YMM: 0, WidthMM: 1, HeightMM: 1},
				},
			}},
			Nets: []placement.Net{
				{Name: "VIN", Role: placement.NetPower},
				{Name: "GND", Role: placement.NetGround},
			},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 10, YMM: 20, RotationDeg: 90, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpoints(placed)

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 2 {
		t.Fatalf("endpoints = %#v, want two", endpoints)
	}
	first := endpoints[0]
	if first.ID == "" || first.Kind != PhysicalEndpointFootprintPad || first.Ref != "J1" || first.Pad != "1" || first.NetName != "VIN" {
		t.Fatalf("endpoint identity = %#v", first)
	}
	if len(first.Layers) != 1 || first.Layers[0] != "F.Cu" {
		t.Fatalf("endpoint layers = %#v", first.Layers)
	}
	if first.Point == nil || first.Point.XMM != 10 || first.Point.YMM != 19 {
		t.Fatalf("endpoint point = %#v, want rotated absolute point 10,19", first.Point)
	}
	if first.Confidence != PhysicalEndpointConfidenceHigh {
		t.Fatalf("confidence = %q", first.Confidence)
	}
	if !containsString(first.Roles, "connector") || !containsString(first.Roles, "power") {
		t.Fatalf("roles = %#v", first.Roles)
	}
}

func TestDiscoverPhysicalEndpointsReportsMissingPlacementAndUnnamedPads(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{Components: []placement.Component{
			{Ref: "J1", Pads: []placement.PadSummary{{Name: "1"}}},
			{Ref: "J2", Position: &placement.Placement{XMM: 1, YMM: 2}, Pads: []placement.PadSummary{{Name: ""}}},
		}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpoints(placed)

	if len(endpoints) != 0 {
		t.Fatalf("endpoints = %#v, want none", endpoints)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want missing placement and unnamed pad", issues)
	}
}

func TestDiscoverPhysicalEndpointsMirrorsBottomLayerPads(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{Components: []placement.Component{{
			Ref:      "J1",
			Pads:     []placement.PadSummary{{Name: "1", Net: "SIG", XMM: -1, YMM: 0, WidthMM: 1, HeightMM: 1}},
			Position: &placement.Placement{XMM: 10, YMM: 20, RotationDeg: 0, Layer: "B.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpoints(placed)

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %#v, want one", endpoints)
	}
	if endpoints[0].Point == nil || endpoints[0].Point.XMM != 11 || endpoints[0].Point.YMM != 20 {
		t.Fatalf("bottom pad point = %#v, want mirrored absolute point 11,20", endpoints[0].Point)
	}
	if len(endpoints[0].Layers) != 1 || endpoints[0].Layers[0] != "B.Cu" {
		t.Fatalf("bottom endpoint layers = %#v", endpoints[0].Layers)
	}
}

func TestPhysicalEndpointIDStableAcrossCoordinateAndNetChanges(t *testing.T) {
	id1 := physicalEndpointID(PhysicalEndpointFootprintPad, "J1", "A6")
	id2 := physicalEndpointID(PhysicalEndpointFootprintPad, "J1", "A6")
	id3 := physicalEndpointID(PhysicalEndpointFootprintPad, "J1", "A7")

	if id1 == "" || id1 != id2 {
		t.Fatalf("stable IDs = %q %q", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("different pad reused endpoint ID %q", id1)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
