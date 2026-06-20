package routing

import (
	"context"
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
