package routing

import (
	"context"
	"math"
	"reflect"
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

func TestRouteRequestReusesDuplicatePadAccessAcrossNetBranches(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Components = []Component{
		{
			Ref:      "J1",
			Position: Placement{Layer: "F.Cu"},
			Pads: []Pad{
				duplicateAccessTestPad("J1", "SH", Point{XMM: 15, YMM: 5}),
				duplicateAccessTestPad("J1", "SH", Point{XMM: 15, YMM: 15}),
			},
		},
		{
			Ref:      "J2",
			Position: Placement{Layer: "F.Cu"},
			Pads:     []Pad{duplicateAccessTestPad("J2", "1", Point{XMM: 25, YMM: 5})},
		},
		{
			Ref:      "J3",
			Position: Placement{Layer: "F.Cu"},
			Pads:     []Pad{duplicateAccessTestPad("J3", "1", Point{XMM: 5, YMM: 15})},
		},
	}
	request.Nets = []Net{{
		Name: "SIG",
		Endpoints: []Endpoint{
			{Ref: "J1", Pin: "SH"},
			{Ref: "J2", Pin: "1"},
			{Ref: "J3", Pin: "1"},
		},
	}}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s, issues = %#v, routes = %#v", result.Status, result.Issues, result.Routes)
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 {
		t.Fatalf("routes = %#v, want one routed tree", result.Routes)
	}
	graph := newRouteConnectivity(result.Routes[0])
	root, rootOK := graph.nearestKey(Point{XMM: 15, YMM: 5}, "F.Cu")
	second, secondOK := graph.nearestKey(Point{XMM: 25, YMM: 5}, "F.Cu")
	third, thirdOK := graph.nearestKey(Point{XMM: 5, YMM: 15}, "F.Cu")
	if !rootOK || !secondOK || !thirdOK || graph.find(root) != graph.find(second) || graph.find(root) != graph.find(third) {
		t.Fatalf("duplicate-pad route tree is disconnected: root=%v second=%v third=%v route=%#v", rootOK, secondOK, thirdOK, result.Routes[0])
	}
}

func duplicateAccessTestPad(ref, name string, point Point) Pad {
	return Pad{
		Ref:      ref,
		Name:     name,
		Net:      "SIG",
		Position: point,
		Shape:    PadRect,
		Type:     PadSMD,
		Size:     Size{WidthMM: 1, HeightMM: 1},
		Layers:   []string{"F.Cu"},
	}
}

func TestEndpointNeckdownTrunkIssueIdentifiesPair(t *testing.T) {
	issue := endpointNeckdownTrunkIssue("GND", 3, EndpointPair{
		From: Endpoint{Ref: "U2", Pin: "7"},
		To:   Endpoint{Ref: "R5", Pin: "2"},
	})
	if issue.Path != `nets["GND"].pairs[3]` || issue.Message != "endpoint neckdown path between U2.7 and R5.2 does not leave a clearance-safe full-width trunk" {
		t.Fatalf("issue = %#v", issue)
	}
	if !reflect.DeepEqual(issue.Refs, []string{"U2", "R5"}) || !reflect.DeepEqual(issue.Nets, []string{"GND"}) {
		t.Fatalf("issue identity = %#v", issue)
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

func TestRouteRequestFailedNetDoesNotOccupyCopperForLaterNets(t *testing.T) {
	request := failedBranchOccupancyRequest()

	result := RouteRequest(request)
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial; issues = %#v", result.Status, result.Issues)
	}
	var failedRoute, laterRoute Route
	for _, route := range result.Routes {
		switch route.Net {
		case "A_FAIL":
			failedRoute = route
		case "B_LATER":
			laterRoute = route
		}
	}
	if failedRoute.Status != RouteStatusFailed || len(failedRoute.Segments) == 0 {
		t.Fatalf("failed route = %#v, want a discarded successful branch", failedRoute)
	}
	if laterRoute.Status != RouteStatusRouted || len(laterRoute.Segments) == 0 {
		t.Fatalf("later route = %#v, want routed after failed branch rollback", laterRoute)
	}
	if result.Metrics.SegmentCount != len(laterRoute.Segments) || result.Metrics.ViaCount != len(laterRoute.Vias) {
		t.Fatalf("committed metrics = %#v, want only later route geometry", result.Metrics)
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

func TestNominalSegmentsClearOccupancyRejectsThickenedCollision(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.TraceWidthMM = 0.8
	request.Obstacles = append(request.Obstacles, Obstacle{
		Layer:    "F.Cu",
		Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9, YMM: 9}, Max: Point{XMM: 11, YMM: 11}}},
	})
	occupancy, err := BuildOccupancy(request, "SIG")
	if err != nil {
		t.Fatalf("build occupancy: %v", err)
	}
	segments := []Segment{
		{Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 2, YMM: 10}, End: Point{XMM: 5, YMM: 10}, WidthMM: 0.2},
		{Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 5, YMM: 10}, End: Point{XMM: 15, YMM: 10}, WidthMM: 0.8},
	}
	if nominalSegmentsClearOccupancy(segments, 0.8, occupancy, request.Board.Layers) {
		t.Fatal("thickened segment crossing an obstacle was accepted")
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

func failedBranchOccupancyRequest() Request {
	request := singleLayerSearchRequest()
	request.Board.WidthMM = 10
	request.Board.HeightMM = 10
	request.Board.MarginMM = 1
	request.Strategy.AllowPartial = true
	request.Components = []Component{
		testComponent("A1", "1", "A_FAIL", 1.5, 5),
		testComponent("A2", "1", "A_FAIL", 8.5, 5),
		testComponent("A3", "1", "A_FAIL", 8, 8),
		testComponent("B1", "1", "B_LATER", 5, 2),
		testComponent("B2", "1", "B_LATER", 5, 8),
	}
	request.Nets = []Net{
		{Name: "A_FAIL", Priority: 10, Endpoints: []Endpoint{{Ref: "A1", Pin: "1"}, {Ref: "A2", Pin: "1"}, {Ref: "A3", Pin: "1"}}},
		{Name: "B_LATER", Priority: 1, Endpoints: []Endpoint{{Ref: "B1", Pin: "1"}, {Ref: "B2", Pin: "1"}}},
	}
	request.Obstacles = []Obstacle{
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 6.5}, Max: Point{XMM: 9.5, YMM: 7}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 9}, Max: Point{XMM: 9.5, YMM: 9.5}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 6.5, YMM: 7}, Max: Point{XMM: 7, YMM: 9}}}},
		{Kind: ObstacleKeepout, Layer: "F.Cu", Geometry: Shape{Rect: &Rect{Min: Point{XMM: 9, YMM: 7}, Max: Point{XMM: 9.5, YMM: 9}}}},
	}
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
