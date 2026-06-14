package routing

import (
	"testing"

	"kicadai/internal/placement"
)

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
	if len(request.Obstacles) != 1 || request.Obstacles[0].Kind != ObstacleKeepout {
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
