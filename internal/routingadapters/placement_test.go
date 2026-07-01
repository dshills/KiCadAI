package routingadapters

import (
	"math"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
)

const floatTolerance = 1e-6

func TestRequestFromPlacementBuildsRoutingRequest(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", FootprintID: "Connector:J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", FootprintID: "Connector:J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(request.Components) != 2 || len(request.Nets) != 1 {
		t.Fatalf("request = %#v", request)
	}
	if request.Components[0].Pads[0].Layers[0] != "F.Cu" {
		t.Fatalf("pad layers = %#v", request.Components[0].Pads[0].Layers)
	}
}

func TestRequestFromPlacementUsesPlacementLayerForPads(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "B.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "B.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if request.Components[0].Pads[0].Layers[0] != "B.Cu" {
		t.Fatalf("pad layers = %#v, want B.Cu", request.Components[0].Pads[0].Layers)
	}
}

func TestRequestFromPlacementPreservesThroughHolePadLayerAccess(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementRequest.Components[0].Pads[0].Type = "thru_hole"
	placementRequest.Components[0].Pads[0].DrillMM = 0.8
	placementRequest.Components[0].Pads[0].Layers = []string{"*.Cu", "*.Mask"}
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	pad := request.Components[0].Pads[0]
	if pad.Type != routing.PadThroughHole || pad.Drill == nil || pad.Drill.DiameterMM != 0.8 {
		t.Fatalf("pad = %#v, want through-hole drill evidence", pad)
	}
	access := routing.BuildPadAccess(request)
	points, ok := routing.AccessPointsForEndpoint(access, routing.Endpoint{Ref: "J1", Pin: "1"})
	if !ok || len(points) != 2 {
		t.Fatalf("access points = %#v, ok=%v; issues=%#v", points, ok, access.Issues)
	}
}

func TestRequestFromPlacementKeepsSMDPadLayerConstrained(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementRequest.Components[0].Pads[0].Type = "smd"
	placementRequest.Components[0].Pads[0].Layers = []string{"B.Cu", "B.Mask", "B.Paste"}
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	pad := request.Components[0].Pads[0]
	if pad.Type != routing.PadSMD || len(pad.Layers) != 1 || pad.Layers[0] != "B.Cu" {
		t.Fatalf("pad = %#v, want SMD constrained to B.Cu", pad)
	}
}

func TestRequestFromPlacementPreservesExplicitSMDCopperLayers(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementRequest.Components[0].Pads[0].Type = "smd"
	placementRequest.Components[0].Pads[0].Layers = []string{"F.Cu", "B.Cu", "F.Mask"}
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	pad := request.Components[0].Pads[0]
	if len(pad.Layers) != 2 || pad.Layers[0] != "F.Cu" || pad.Layers[1] != "B.Cu" {
		t.Fatalf("pad layers = %#v, want explicit F.Cu/B.Cu SMD copper layers", pad.Layers)
	}
}

func TestRequestFromPlacementPreservesLocalPadGeometryAndNet(t *testing.T) {
	placementRequest := placementAdapterRequest()
	if len(placementRequest.Components) < 2 {
		t.Fatal("placement adapter fixture missing components")
	}
	placementRequest.Components[0].Pads = []placement.PadSummary{{Name: "A", Net: "SIG_A", XMM: -0.65, YMM: 0.25, WidthMM: 0.4, HeightMM: 0.8}}
	placementResult := placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, RotationDeg: 90, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}}

	request, issues := RequestFromPlacement(placementRequest, placementResult)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(request.Components) == 0 || len(request.Components[0].Pads) == 0 {
		t.Fatalf("missing converted pad: %#v", request.Components)
	}
	pad := request.Components[0].Pads[0]
	if pad.Name != "A" || pad.Net != "SIG_A" {
		t.Fatalf("pad identity/net = %#v", pad)
	}
	assertCloseFloat(t, pad.Position.XMM, -0.65, "pad x")
	assertCloseFloat(t, pad.Position.YMM, 0.25, "pad y")
	assertCloseFloat(t, pad.Size.WidthMM, 0.4, "pad width")
	assertCloseFloat(t, pad.Size.HeightMM, 0.8, "pad height")
}

func TestRequestFromPlacementConvertsKeepouts(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementRequest.Keepouts = []placement.Keepout{{
		ID:     "mounting",
		Bounds: placement.Rect{Min: placement.Point{XMM: 1, YMM: 2}, Max: placement.Point{XMM: 3, YMM: 4}},
		Layers: []string{"F.Cu"},
	}}

	request, issues := RequestFromPlacement(placementRequest, placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(request.Obstacles) != 1 || request.Obstacles[0].Kind != routing.ObstacleKeepout {
		t.Fatalf("obstacles = %#v", request.Obstacles)
	}
}

func TestRequestFromPlacementReportsMissingPadData(t *testing.T) {
	placementRequest := placementAdapterRequest()
	placementRequest.Components[0].Pads = nil
	_, issues := RequestFromPlacement(placementRequest, placement.Result{Placements: []placement.PlacementResult{
		{Ref: "J1", Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "J2", Position: placement.Placement{XMM: 15, YMM: 5, Layer: "F.Cu"}},
	}})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want missing pad issue", issues)
	}
}

func placementAdapterRequest() placement.Request {
	return placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 20, HeightMM: 10, MarginMM: 1},
		Components: []placement.Component{
			{Ref: "J1", FootprintID: "Connector:J1", Pads: []placement.PadSummary{{Name: "1", Net: "SIG", WidthMM: 1, HeightMM: 1}}},
			{Ref: "J2", FootprintID: "Connector:J2", Pads: []placement.PadSummary{{Name: "1", Net: "SIG", WidthMM: 1, HeightMM: 1}}},
		},
		Nets: []placement.Net{{
			Name: "SIG",
			Role: placement.NetSignal,
			Endpoints: []placement.Endpoint{
				{Ref: "J1", Pin: "1"},
				{Ref: "J2", Pin: "1"},
			},
		}},
		Rules: placement.Rules{AllowBackLayer: true},
	}
}

func assertCloseFloat(t testing.TB, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) >= floatTolerance {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
