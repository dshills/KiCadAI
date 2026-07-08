package routing

import (
	"context"
	"testing"

	"kicadai/internal/reports"
)

func TestRouteTwoLayerPathRequiresOneVia(t *testing.T) {
	request := twoLayerViaRequest()
	path, issues := routeTwoLayerFirstPair(t, request)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	vias := BuildViasFromPath(path, request.Rules)
	if len(vias) != 1 {
		t.Fatalf("vias = %#v, want one via", vias)
	}
	if vias[0].DiameterMM != request.Rules.ViaDiameterMM || vias[0].DrillMM != request.Rules.ViaDrillMM {
		t.Fatalf("via geometry = %#v", vias[0])
	}
}

func TestRouteTwoLayerPathViaForbidden(t *testing.T) {
	request := twoLayerViaRequest()
	disabled := false
	request.Rules.AllowVias = &disabled
	_, issues := routeTwoLayerFirstPair(t, request)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one via-forbidden issue", issues)
	}
}

func TestRouteTwoLayerPathBackLayerDisabled(t *testing.T) {
	request := twoLayerViaRequest()
	disabled := false
	request.Rules.AllowBackLayer = &disabled
	_, issues := routeTwoLayerFirstPair(t, request)
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one back-layer-disabled issue", issues)
	}
}

func TestRouteTwoLayerPathBackLayerDisabledAndPreferredLayerDisallowed(t *testing.T) {
	request := twoLayerViaRequest()
	disabled := false
	request.Rules.AllowBackLayer = &disabled
	request.Rules.PreferLayer = "B.Cu"
	request.Rules.AllowedLayers = []string{"F.Cu"}

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if len(result.Routes) == 0 || len(result.Routes[0].Issues) == 0 {
		t.Fatalf("expected route issue: %#v", result)
	}
	if got := result.Routes[0].Issues[0].Message; got != "no valid routing layer is available with back layer disabled" {
		t.Fatalf("message = %q", got)
	}
}

func TestBuildViasFromPathDeduplicatesLayerTransitions(t *testing.T) {
	path := GridPath{
		Net:        "SIG",
		Layer:      "F.CU",
		LayerNames: map[int]string{0: "F.CU", 1: "B.CU"},
		Coordinates: []GridCoord{
			{X: 1, Y: 1, Layer: 0},
			{X: 1, Y: 1, Layer: 1},
			{X: 1, Y: 1, Layer: 0},
		},
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 1, YMM: 1},
			{XMM: 1, YMM: 1},
		},
	}

	vias := BuildViasFromPath(path, DefaultRules())
	if len(vias) != 1 {
		t.Fatalf("vias = %#v, want deduplicated via", vias)
	}
	if got := vias[0].Layers; len(got) != 2 || got[0] != "B.CU" || got[1] != "F.CU" {
		t.Fatalf("layers = %#v", got)
	}
}

func TestBuildViasFromPathKeepsFallbackDrillInsideDiameter(t *testing.T) {
	path := GridPath{
		Net:        "SIG",
		Layer:      "F.CU",
		LayerNames: map[int]string{0: "F.CU", 1: "B.CU"},
		Coordinates: []GridCoord{
			{X: 1, Y: 1, Layer: 0},
			{X: 1, Y: 1, Layer: 1},
		},
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 1, YMM: 1},
		},
	}
	rules := DefaultRules()
	rules.ViaDiameterMM = 0.2
	rules.ViaDrillMM = 0.25

	vias := BuildViasFromPath(path, rules)
	if len(vias) != 1 {
		t.Fatalf("vias = %#v, want one via", vias)
	}
	if vias[0].DrillMM <= 0 || vias[0].DrillMM >= vias[0].DiameterMM {
		t.Fatalf("via geometry = %#v, drill must fit diameter", vias[0])
	}
}

func TestBuildViasFromPathSkipsUnresolvedLayerNames(t *testing.T) {
	path := GridPath{
		Net:        "SIG",
		Layer:      "F.CU",
		LayerNames: map[int]string{0: "F.CU"},
		Coordinates: []GridCoord{
			{X: 1, Y: 1, Layer: 0},
			{X: 1, Y: 1, Layer: 1},
		},
		Points: []Point{
			{XMM: 1, YMM: 1},
			{XMM: 1, YMM: 1},
		},
	}

	vias := BuildViasFromPath(path, DefaultRules())
	if len(vias) != 0 {
		t.Fatalf("vias = %#v, want unresolved transition skipped", vias)
	}
}

func routeTwoLayerFirstPair(t *testing.T, request Request) (GridPath, []reports.Issue) {
	t.Helper()
	access := BuildPadAccess(request)
	if len(access.Issues) != 0 {
		t.Fatalf("access issues = %#v", access.Issues)
	}
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildOccupancy error: %v", err)
	}
	viaOccupancy, err := BuildViaOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildViaOccupancy error: %v", err)
	}
	plans, issues := PlanRoutes(request, access)
	if len(issues) != 0 {
		t.Fatalf("plan issues = %#v", issues)
	}
	if len(plans) != 1 || len(plans[0].Pairs) != 1 {
		t.Fatalf("plans = %#v, want one pair", plans)
	}
	return routeTwoLayerPath(context.Background(), request, access, occupancy, viaOccupancy, "SIG", plans[0].Pairs[0])
}

func twoLayerViaRequest() Request {
	request := minimalRequest()
	request.Rules.GridMM = 1
	request.Rules.TraceWidthMM = 0.1
	request.Rules.ClearanceMM = 0.01
	request.Rules.EdgeClearanceMM = 0.01
	request.Rules.MaxSearchNodes = 10000
	request.Rules.MaxViasPerNet = 2
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ViaDrillMM = 0.3
	request.Strategy.Mode = ModeTwoLayer
	request.Components[0].Pads[0].Type = PadSMD
	request.Components[0].Pads[0].Drill = nil
	request.Components[0].Pads[0].Layers = []string{"F.Cu"}
	request.Components[1].Pads[0].Type = PadSMD
	request.Components[1].Pads[0].Drill = nil
	request.Components[1].Pads[0].Layers = []string{"B.Cu"}
	return request
}
