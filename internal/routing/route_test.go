package routing

import (
	"context"
	"math"
	"testing"

	"kicadai/internal/reports"
)

func TestRouteRequestRoutesSimpleBoard(t *testing.T) {
	request := singleLayerSearchRequest()

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.RoutedNetCount != 1 || result.Metrics.SegmentCount == 0 {
		t.Fatalf("metrics = %#v", result.Metrics)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 {
		t.Fatalf("routes = %#v", result.Routes)
	}
}

func TestRouteRequestContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := RouteRequestContext(ctx, singleLayerSearchRequest())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeOperationCanceled {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestRouteRequestLaterNetAvoidsEarlierCopper(t *testing.T) {
	request := crossingNetsRequest()

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if len(result.Routes) != 2 {
		t.Fatalf("routes = %#v, want two", result.Routes)
	}
	var vertical Route
	for _, route := range result.Routes {
		if route.Net == "B_VERTICAL" {
			vertical = route
		}
	}
	for _, segment := range vertical.Segments {
		if segmentContainsPoint(segment, Point{XMM: 12, YMM: 10}) {
			t.Fatalf("vertical net crosses first route at %#v in segments %#v", Point{XMM: 12, YMM: 10}, vertical.Segments)
		}
	}
}

func TestRouteRequestBlockedWhenPartialDisallowed(t *testing.T) {
	request := partialRoutingRequest()
	request.Strategy.AllowPartial = false

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked; issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.FailedNetCount != 1 {
		t.Fatalf("metrics = %#v, want one failed net", result.Metrics)
	}
}

func TestRouteRequestPartialWhenAllowed(t *testing.T) {
	request := partialRoutingRequest()
	request.Strategy.AllowPartial = true

	result := RouteRequest(request)
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial; issues = %#v", result.Status, result.Issues)
	}
	if result.Metrics.RoutedNetCount != 1 || result.Metrics.FailedNetCount != 1 {
		t.Fatalf("metrics = %#v, want one routed and one failed", result.Metrics)
	}
}

func TestRouteRequestAppliesNetClassTraceAndViaRules(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"WIDE": {
			TraceWidthMM:  0.45,
			ViaDiameterMM: 0.8,
			ViaDrillMM:    0.35,
			MaxViasPerNet: 1,
		},
	}
	request.Nets[0].Class = "WIDE"

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v", result.Status, result.Issues)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 || len(result.Routes[0].Vias) == 0 {
		t.Fatalf("expected routed segments and via: %#v", result.Routes)
	}
	if result.Routes[0].Segments[0].WidthMM != 0.45 {
		t.Fatalf("segment width = %v, want net class width", result.Routes[0].Segments[0].WidthMM)
	}
	if result.Routes[0].Vias[0].DiameterMM != 0.8 || result.Routes[0].Vias[0].DrillMM != 0.35 {
		t.Fatalf("via geometry = %#v", result.Routes[0].Vias[0])
	}
}

func TestRouteRequestKeepsViasClearOfNoNetPads(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.GridMM = 0.25
	request.Rules.TraceWidthMM = 0.25
	request.Rules.ClearanceMM = 0.2
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ViaDrillMM = 0.3
	request.Components = append(request.Components, Component{
		Ref:      "U1",
		Position: Placement{XMM: 15, YMM: 10, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref:      "U1",
			Name:     "NC",
			Position: Point{},
			Shape:    PadRect,
			Type:     PadSMD,
			Size:     Size{WidthMM: 0.7, HeightMM: 0.9},
			Layers:   []string{"F.Cu"},
		}},
	})

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v routes=%#v", result.Status, result.Issues, result.Routes)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Vias) == 0 {
		t.Fatalf("routes = %#v, want a routed via", result.Routes)
	}
	noNetPad := Rect{
		Min: Point{XMM: 15 - 0.35, YMM: 10 - 0.45},
		Max: Point{XMM: 15 + 0.35, YMM: 10 + 0.45},
	}
	requiredClearance := request.Rules.ViaDiameterMM/2 + request.Rules.ClearanceMM
	for _, via := range result.Routes[0].Vias {
		if distancePointToRect(via.At, noNetPad) < requiredClearance-1e-9 {
			t.Fatalf("via at %#v is too close to no-net pad %#v; want clearance >= %.3f", via.At, noNetPad, requiredClearance)
		}
	}
}

func TestRoutableViaSpanChecksIntermediateLayers(t *testing.T) {
	request := twoLayerViaRequest()
	request.Board.Layers = []Layer{
		{Name: "F.Cu", Kind: LayerCopper, Routable: true},
		{Name: "In1.Cu", Kind: LayerCopper},
		{Name: "B.Cu", Kind: LayerCopper, Routable: true},
	}
	request.Rules.GridMM = 0.25
	request.Rules.ViaDiameterMM = 0.6
	request.Rules.ClearanceMM = 0.2
	request.Obstacles = []Obstacle{{
		Kind:     ObstacleKeepout,
		Layer:    "In1.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9.75, YMM: 9.75}, Max: Point{XMM: 10.25, YMM: 10.25}}},
	}}
	viaOccupancy, err := BuildViaOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("BuildViaOccupancy error: %v", err)
	}
	layerIndexes, err := LayerIndexes(request.Board.Layers)
	if err != nil {
		t.Fatalf("LayerIndexes error: %v", err)
	}
	coord := viaOccupancy.Grid.ToGrid(Point{XMM: 10, YMM: 10}, layerIndexes[normalizeLayer("F.Cu")])
	target := coord
	target.Layer = layerIndexes[normalizeLayer("B.Cu")]

	if routableViaSpan(viaOccupancy, coord, target) {
		t.Fatal("via span should be blocked by intermediate-layer keepout")
	}
}

func TestRouteRequestAllowedLayersCanBlockRoute(t *testing.T) {
	request := twoLayerViaRequest()
	request.Rules.NetClasses = map[string]NetClass{
		"TOP_ONLY": {AllowedLayers: []string{"F.Cu"}},
	}
	request.Nets[0].Class = "TOP_ONLY"

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if result.Quality == nil || result.Quality.NetReports[0].Status != RouteStatusFailed {
		t.Fatalf("expected failed quality report: %#v", result.Quality)
	}
}

func distancePointToRect(point Point, rect Rect) float64 {
	dx := max(max(rect.Min.XMM-point.XMM, 0), point.XMM-rect.Max.XMM)
	dy := max(max(rect.Min.YMM-point.YMM, 0), point.YMM-rect.Max.YMM)
	return math.Hypot(dx, dy)
}

func TestRouteQualityReportsSameNetMergeCandidates(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Existing = []ExistingCopper{
		{
			Kind:     CopperSegment,
			Net:      "SIG",
			Layer:    "F.Cu",
			Geometry: Shape{Rect: &Rect{Min: Point{XMM: 10, YMM: 9}, Max: Point{XMM: 11, YMM: 10}}},
		},
		{
			Kind:     CopperSegment,
			Net:      "OTHER",
			Layer:    "F.Cu",
			Geometry: Shape{Rect: &Rect{Min: Point{XMM: 18, YMM: 1}, Max: Point{XMM: 19, YMM: 2}}},
		},
	}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v", result.Status, result.Issues)
	}
	if result.Quality == nil || len(result.Quality.NetReports) != 1 {
		t.Fatalf("quality = %#v", result.Quality)
	}
	report := result.Quality.NetReports[0]
	if report.SameNetPads != 2 || report.SameNetCopper != 1 {
		t.Fatalf("same-net evidence = pads %d copper %d, want 2/1", report.SameNetPads, report.SameNetCopper)
	}
}

func TestRouteRequestLengthWarningDoesNotFailRoute(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.NetOverrides = map[string]NetRule{
		"SIG": {WarningLengthMM: 1},
	}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues=%#v", result.Status, result.Issues)
	}
	if len(result.Routes[0].Issues) == 0 {
		t.Fatalf("expected length warning")
	}
	if result.Routes[0].Status != RouteStatusRouted {
		t.Fatalf("route status = %s", result.Routes[0].Status)
	}
}

func TestRouteRequestMaxLengthFailsRoute(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.NetOverrides = map[string]NetRule{
		"SIG": {MaxLengthMM: 1},
	}

	result := RouteRequest(request)
	if result.Status != StatusBlocked {
		t.Fatalf("status = %s, want blocked", result.Status)
	}
	if result.Routes[0].Status != RouteStatusFailed {
		t.Fatalf("route status = %s", result.Routes[0].Status)
	}
}

func TestExistingCopperForSegmentsIncludesTraceWidth(t *testing.T) {
	existing := existingCopperForSegments([]Segment{{
		Net:     "SIG",
		Layer:   "F.CU",
		Start:   Point{XMM: 1, YMM: 2},
		End:     Point{XMM: 5, YMM: 2},
		WidthMM: 0.4,
	}})
	if len(existing) != 1 || existing[0].Geometry.Rect == nil {
		t.Fatalf("existing = %#v", existing)
	}
	rect := *existing[0].Geometry.Rect
	if rect.Min != (Point{XMM: 0.8, YMM: 1.8}) || rect.Max != (Point{XMM: 5.2, YMM: 2.2}) {
		t.Fatalf("rect = %#v, want trace-width-expanded bounds", rect)
	}
	if len(existing[0].Geometry.Polygon) != 4 {
		t.Fatalf("polygon = %#v, want oriented trace body", existing[0].Geometry.Polygon)
	}
}

func crossingNetsRequest() Request {
	request := singleLayerSearchRequest()
	request.Nets[0].Name = "A_HORIZONTAL"
	request.Nets[0].Priority = 10
	request.Components[0].Pads[0].Net = "A_HORIZONTAL"
	request.Components[1].Pads[0].Net = "A_HORIZONTAL"
	request.Components = append(request.Components,
		testComponent("J3", "1", "B_VERTICAL", 12, 5),
		testComponent("J4", "1", "B_VERTICAL", 12, 15),
	)
	request.Nets = append(request.Nets, Net{
		Name:     "B_VERTICAL",
		Priority: 1,
		Endpoints: []Endpoint{
			{Ref: "J3", Pin: "1"},
			{Ref: "J4", Pin: "1"},
		},
	})
	return request
}

func partialRoutingRequest() Request {
	request := singleLayerSearchRequest()
	request.Nets[0].Name = "A_OK"
	request.Components[0].Pads[0].Net = "A_OK"
	request.Components[1].Pads[0].Net = "A_OK"
	request.Components[0].Position.XMM = 2
	request.Components[0].Position.YMM = 5
	request.Components[1].Position.XMM = 8
	request.Components[1].Position.YMM = 5
	request.Components = append(request.Components,
		testComponent("J3", "1", "Z_FAIL", 5, 15),
		testComponent("J4", "1", "Z_FAIL", 25, 15),
	)
	request.Nets = append(request.Nets, Net{
		Name: "Z_FAIL",
		Endpoints: []Endpoint{
			{Ref: "J3", Pin: "1"},
			{Ref: "J4", Pin: "1"},
		},
	})
	request.Obstacles = []Obstacle{{
		Kind:  ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: Shape{Rect: &Rect{
			Min: Point{XMM: 10, YMM: 0},
			Max: Point{XMM: 15, YMM: 20},
		}},
	}}
	return request
}

func testComponent(ref string, pin string, net string, x float64, y float64) Component {
	return Component{
		Ref:      ref,
		Position: Placement{XMM: x, YMM: y, Layer: "F.Cu"},
		Pads: []Pad{{
			Ref:      ref,
			Name:     pin,
			Net:      net,
			Position: Point{},
			Shape:    PadCircle,
			Type:     PadSMD,
			Size:     Size{WidthMM: 1, HeightMM: 1},
			Layers:   []string{"F.Cu"},
		}},
	}
}

func segmentContainsPoint(segment Segment, point Point) bool {
	minX := min(segment.Start.XMM, segment.End.XMM)
	maxX := max(segment.Start.XMM, segment.End.XMM)
	minY := min(segment.Start.YMM, segment.End.YMM)
	maxY := max(segment.Start.YMM, segment.End.YMM)
	return point.XMM >= minX && point.XMM <= maxX && point.YMM >= minY && point.YMM <= maxY
}
