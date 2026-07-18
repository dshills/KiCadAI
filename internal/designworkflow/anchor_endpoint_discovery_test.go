package designworkflow

import (
	"math"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/transactions"
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
	// Pad 1 is at local X=-1. KiCad +90 maps local -X to board +Y, so its
	// absolute Y coordinate is 20+1=21 (pad 2 at local +X maps to Y=19).
	if first.Point == nil || first.Point.XMM != 10 || first.Point.YMM != 21 {
		t.Fatalf("endpoint point = %#v, want local-X=-1 KiCad-rotated absolute point 10,21", first.Point)
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

func TestDiscoverPhysicalEndpointsIncludesExplicitExternalEndpoints(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: []ExternalEndpointSpec{
			{
				ID:         " Edge SIG ",
				Kind:       PhysicalEndpointBoardEdgePoint,
				NetName:    "SIG",
				Roles:      []string{" external ", "signal"},
				Layers:     []string{" f.cu "},
				Point:      &transactions.Point{XMM: 0, YMM: 4},
				Confidence: PhysicalEndpointConfidenceHigh,
			},
			{
				ID:      "mech_vin",
				Kind:    PhysicalEndpointImportedMechanicalPoint,
				NetName: "VIN",
				Roles:   []string{"power_entry"},
				Point:   &transactions.Point{XMM: 2, YMM: 4},
			},
		},
	})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 2 {
		t.Fatalf("endpoints = %#v, want explicit endpoints", endpoints)
	}
	if endpoints[0].ID != "edge_sig" || endpoints[0].Kind != PhysicalEndpointBoardEdgePoint || endpoints[0].Source != physicalEndpointSourceExternalRequest {
		t.Fatalf("board edge endpoint = %#v", endpoints[0])
	}
	if endpoints[0].Point == nil || endpoints[0].Point.XMM != 0 || endpoints[0].Point.YMM != 4 {
		t.Fatalf("board edge point = %#v", endpoints[0].Point)
	}
	if len(endpoints[0].Layers) != 1 || endpoints[0].Layers[0] != "F.Cu" {
		t.Fatalf("board edge layers = %#v", endpoints[0].Layers)
	}
	if endpoints[1].ID != "mech_vin" || endpoints[1].Kind != PhysicalEndpointImportedMechanicalPoint || endpoints[1].Confidence != PhysicalEndpointConfidenceMedium {
		t.Fatalf("imported mechanical endpoint = %#v", endpoints[1])
	}
}

func TestDiscoverPhysicalEndpointsKeepsOptionalExternalEndpointWithoutPoint(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: []ExternalEndpointSpec{{
			ID:    "advisory",
			Kind:  PhysicalEndpointImportedMechanicalPoint,
			Roles: []string{"mechanical_interface"},
		}},
	})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 1 || endpoints[0].Point != nil {
		t.Fatalf("optional endpoint without point was not preserved: %#v", endpoints)
	}
}

func TestDiscoverPhysicalEndpointsKeepsExplicitEndpointsWhenPlacementBlocked(t *testing.T) {
	placed := PlacementStageResult{
		Stage: StageResult{Name: StagePlacement, Status: StageStatusBlocked},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: []ExternalEndpointSpec{{
			ID:     "edge_sig",
			Kind:   PhysicalEndpointBoardEdgePoint,
			Point:  &transactions.Point{XMM: 0, YMM: 3},
			Layers: []string{"F.Cu"},
		}},
	})

	if len(endpoints) != 1 || endpoints[0].ID != "edge_sig" {
		t.Fatalf("blocked placement explicit endpoints = %#v", endpoints)
	}
	if len(issues) != 1 {
		t.Fatalf("blocked placement issues = %#v", issues)
	}
}

func TestDiscoverPhysicalEndpointsSnapsExplicitEndpointToBoardFrame(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		Board: BoardSpec{WidthMM: 10, HeightMM: 5},
		ExternalEndpoints: []ExternalEndpointSpec{{
			ID:    "edge_sig",
			Kind:  PhysicalEndpointBoardEdgePoint,
			Point: &transactions.Point{XMM: -0.0005, YMM: 5.0005},
		}},
	})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 1 || endpoints[0].Point == nil || endpoints[0].Point.XMM != 0 || endpoints[0].Point.YMM != 5 {
		t.Fatalf("snapped endpoint = %#v", endpoints)
	}
}

func TestDiscoverPhysicalEndpointsSkipsDuplicateExplicitEndpointIDs(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{
		ExternalEndpoints: []ExternalEndpointSpec{
			{ID: "edge_sig", Kind: PhysicalEndpointBoardEdgePoint},
			{ID: " edge sig ", Kind: PhysicalEndpointImportedMechanicalPoint},
		},
	})

	if len(endpoints) != 1 || endpoints[0].ID != "edge_sig" {
		t.Fatalf("deduped endpoints = %#v", endpoints)
	}
	if len(issues) != 1 {
		t.Fatalf("duplicate explicit issues = %#v", issues)
	}
}

func TestDiscoverPhysicalEndpointsDerivesBoardEdgeEndpoint(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10},
			Components: []placement.Component{{
				Ref:  "J1",
				Role: "connector",
				Edge: placement.EdgeLeft,
				Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}},
			}},
			Nets: []placement.Net{{Name: "SIG", Role: placement.NetSignal}},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 0.4, YMM: 5, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(endpoints) != 2 {
		t.Fatalf("endpoints = %#v, want pad and derived board edge", endpoints)
	}
	edge := endpoints[0]
	if edge.Kind != PhysicalEndpointBoardEdgePoint {
		edge = endpoints[1]
	}
	if edge.Kind != PhysicalEndpointBoardEdgePoint || edge.Source != physicalEndpointSourceEdgePad || edge.Ref != "J1" || edge.Pad != "1" {
		t.Fatalf("derived edge endpoint = %#v", edge)
	}
	if edge.Point == nil || edge.Point.XMM != 0 || edge.Point.YMM != 5 {
		t.Fatalf("derived edge projected point = %#v", edge.Point)
	}
	if !containsString(edge.Roles, "edge") || !containsString(edge.Roles, "left") || !containsString(edge.Roles, "signal") {
		t.Fatalf("derived edge roles = %#v", edge.Roles)
	}
}

func TestDiscoverPhysicalEndpointsDoesNotDeriveForNonEdgeOrFarPad(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10},
			Components: []placement.Component{
				{Ref: "U1", Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}}},
				{Ref: "J1", Edge: placement.EdgeLeft, Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}}},
			},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{
			{Ref: "U1", Position: placement.Placement{XMM: 0.2, YMM: 5, Layer: "F.Cu"}},
			{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	for _, endpoint := range endpoints {
		if endpoint.Kind == PhysicalEndpointBoardEdgePoint {
			t.Fatalf("unexpected board edge endpoint = %#v", endpoint)
		}
	}
}

func TestDiscoverPhysicalEndpointsDerivedBoardEdgePreservesBottomLayer(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10},
			Components: []placement.Component{{
				Ref:  "J1",
				Edge: placement.EdgeRight,
				Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}},
			}},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 19.5, YMM: 5, Layer: "B.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	var edge PhysicalEndpoint
	for _, endpoint := range endpoints {
		if endpoint.Kind == PhysicalEndpointBoardEdgePoint {
			edge = endpoint
		}
	}
	if edge.ID == "" || len(edge.Layers) != 1 || edge.Layers[0] != "B.Cu" || edge.Point == nil || edge.Point.XMM != 20 {
		t.Fatalf("derived bottom edge endpoint = %#v", edge)
	}
}

func TestDiscoverPhysicalEndpointsDisambiguatesDuplicatePadNamesForDerivedEdges(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10},
			Components: []placement.Component{{
				Ref:  "J1",
				Edge: placement.EdgeLeft,
				Pads: []placement.PadSummary{
					{Name: "GND", Net: "GND", XMM: 0, YMM: -1},
					{Name: "GND", Net: "GND", XMM: 0, YMM: 1},
				},
			}},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 0.2, YMM: 5, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	padEndpointIDs := map[string]struct{}{}
	edgeEndpointIDs := map[string]struct{}{}
	for _, endpoint := range endpoints {
		if endpoint.Kind == PhysicalEndpointBoardEdgePoint {
			edgeEndpointIDs[endpoint.ID] = struct{}{}
		}
		if endpoint.Kind == PhysicalEndpointFootprintPad {
			padEndpointIDs[endpoint.ID] = struct{}{}
		}
	}
	if len(padEndpointIDs) != 2 {
		t.Fatalf("duplicate same-net physical pad endpoints = %#v, want two durable IDs", endpoints)
	}
	if len(edgeEndpointIDs) != 2 {
		t.Fatalf("duplicate same-net derived edge endpoints = %#v, want two durable IDs", endpoints)
	}
	if len(issues) != 0 {
		t.Fatalf("duplicate pad issues = %#v, want none", issues)
	}
}

func TestDiscoverPhysicalEndpointsKeepsDuplicateSameNetPads(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{Components: []placement.Component{{
			Ref: "U1",
			Pads: []placement.PadSummary{
				{Name: "2", Net: "VOUT", XMM: 0, YMM: 2.4},
				{Name: "2", Net: "VOUT", XMM: 0, YMM: -2.1},
			},
		}}},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "U1",
			Position: placement.Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpoints(placed)
	if len(issues) != 0 {
		t.Fatalf("duplicate same-net pad issues = %#v", issues)
	}
	if len(endpoints) != 2 || endpoints[0].ID == endpoints[1].ID || endpoints[0].Pad != "2" || endpoints[1].Pad != "2" {
		t.Fatalf("duplicate same-net endpoints = %#v, want two physical pad IDs for logical pad 2", endpoints)
	}
}

func TestDiscoverPhysicalEndpointsUsesBoardOriginFrame(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10, Origin: placement.Point{XMM: 100, YMM: 50}},
			Components: []placement.Component{{
				Ref:  "J1",
				Edge: placement.EdgeLeft,
				Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}},
			}},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 100.4, YMM: 55, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	var edge PhysicalEndpoint
	for _, endpoint := range endpoints {
		if endpoint.Kind == PhysicalEndpointBoardEdgePoint {
			edge = endpoint
		}
	}
	if edge.Point == nil || math.Abs(edge.Point.XMM-100) > 1e-9 || edge.Point.YMM != 55 {
		t.Fatalf("origin-relative edge point = %#v", edge.Point)
	}
}

func TestDiscoverPhysicalEndpointsWarnsWhenEdgeDerivationHasNoBoardFrame(t *testing.T) {
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{{
				Ref:  "J1",
				Edge: placement.EdgeLeft,
				Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0}},
			}},
		},
		Result: placement.Result{Placements: []placement.PlacementResult{{
			Ref:      "J1",
			Position: placement.Placement{XMM: 0, YMM: 0, Layer: "F.Cu"},
		}}},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}

	endpoints, issues := DiscoverPhysicalEndpointsWithOptions(placed, PhysicalEndpointDiscoveryOptions{})

	if len(endpoints) != 1 || endpoints[0].Kind != PhysicalEndpointFootprintPad {
		t.Fatalf("endpoints = %#v", endpoints)
	}
	if len(issues) != 1 || issues[0].Path != "anchor_bindings.endpoints.J1.edge" {
		t.Fatalf("issues = %#v", issues)
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
