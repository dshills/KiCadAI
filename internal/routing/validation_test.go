package routing

import "testing"

func TestValidateResultDetectsDisconnectedEndpoint(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Segments: []Segment{{
			Net: "SIG", Layer: "F.CU", Start: Point{XMM: 5, YMM: 10}, End: Point{XMM: 8, YMM: 10}, WidthMM: 0.1,
		}},
	}}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "route does not connect all intended endpoints")
}

func TestValidateResultDetectsOutsideBoardSegment(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Segments: []Segment{{
			Net: "SIG", Layer: "F.CU", Start: Point{XMM: -1, YMM: 10}, End: Point{XMM: 20, YMM: 10}, WidthMM: 0.1,
		}},
	}}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "segment endpoint is outside board")
}

func TestValidateResultDetectsClearanceViolation(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Segments: []Segment{{Net: "A", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.1}}},
		{Net: "B", Status: RouteStatusRouted, Segments: []Segment{{Net: "B", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1.005}, End: Point{XMM: 5, YMM: 1.005}, WidthMM: 0.1}}},
	}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "segment clearance violation with net B: (1,1) to (5,1) crosses (1,1.005) to (5,1.005)")
}

func TestValidateResultDetectsCrossedTraceClearanceViolation(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Segments: []Segment{{Net: "A", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 5}, WidthMM: 0.1}}},
		{Net: "B", Status: RouteStatusRouted, Segments: []Segment{{Net: "B", Layer: "F.CU", Start: Point{XMM: 1, YMM: 5}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.1}}},
	}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "segment clearance violation with net B: (1,1) to (5,5) crosses (1,5) to (5,1)")
}

func TestClearanceIssuesDetectsLongShallowDiagonalViolation(t *testing.T) {
	routes := []Route{
		{Net: "composition_net_008", Segments: []Segment{{
			Net: "composition_net_008", Layer: "B.Cu", Start: Point{XMM: 10.25, YMM: 8}, End: Point{XMM: 29.75, YMM: 8.5}, WidthMM: 0.2,
		}}},
		{Net: "composition_net_017", Segments: []Segment{{
			Net: "composition_net_017", Layer: "B.Cu", Start: Point{XMM: 15.25, YMM: 7.75}, End: Point{XMM: 50.25, YMM: 7.75}, WidthMM: 0.2,
		}}},
	}
	if issues := clearanceIssues(routes, 0.2); len(issues) == 0 {
		t.Fatal("long shallow diagonal clearance violation was not detected")
	}
}

func TestValidatePhysicalClearanceDetectsCrossPhasePadCollision(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.ClearanceMM = 0.2
	request.Components = append(request.Components, Component{
		Ref: "J7", Position: Placement{XMM: 22, YMM: 11}, Pads: []Pad{{
			Name: "1", Net: "OTHER", Type: PadThroughHole, Shape: PadCircle,
			Size: Size{WidthMM: 1.7, HeightMM: 1.7}, Layers: []string{"*.Cu"},
		}},
	})
	routes := []Route{{Net: "SIG", Segments: []Segment{{
		Net: "SIG", Layer: "F.Cu", Start: Point{XMM: 22.75, YMM: 13.5}, End: Point{XMM: 23.25, YMM: 8}, WidthMM: 0.3,
	}}}}
	if issues := ValidatePhysicalClearance(request, routes); len(issues) == 0 {
		t.Fatal("cross-phase segment-to-pad violation was not detected")
	}
}

func TestValidateResultDetectsSegmentToViaClearanceViolation(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.ClearanceMM = 0.2
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Segments: []Segment{{Net: "A", Layer: "F.Cu", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.5}}},
		{Net: "B", Status: RouteStatusRouted, Vias: []Via{{Net: "B", At: Point{XMM: 3, YMM: 1.74}, DiameterMM: 0.7, DrillMM: 0.35, Layers: []string{"F.Cu", "B.Cu"}}}},
	}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "segment clearance violation with via on net B")
}

func TestValidateResultAcceptsSegmentAtExactViaClearance(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.ClearanceMM = 0.2
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Segments: []Segment{{Net: "A", Layer: "F.Cu", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.5}}},
		{Net: "B", Status: RouteStatusRouted, Vias: []Via{{Net: "B", At: Point{XMM: 3, YMM: 1.8}, DiameterMM: 0.7, DrillMM: 0.35, Layers: []string{"F.Cu", "B.Cu"}}}},
	}}

	if report := ValidateResult(request, result); len(report.Issues) != 0 {
		t.Fatalf("exact segment-to-via clearance produced issues: %#v", report.Issues)
	}
}

func TestValidateResultDetectsViaToViaClearanceViolation(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.ClearanceMM = 0.2
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Vias: []Via{{Net: "A", At: Point{XMM: 3, YMM: 3}, DiameterMM: 0.7, DrillMM: 0.35, Layers: []string{"F.Cu", "B.Cu"}}}},
		{Net: "B", Status: RouteStatusRouted, Vias: []Via{{Net: "B", At: Point{XMM: 3.8, YMM: 3}, DiameterMM: 0.7, DrillMM: 0.35, Layers: []string{"F.Cu", "B.Cu"}}}},
	}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "via clearance violation with net B")
}

func TestClearanceIssuesAcceptsExactDeclaredGap(t *testing.T) {
	routes := []Route{
		{Net: "A", Segments: []Segment{{Net: "A", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.3}}},
		{Net: "B", Segments: []Segment{{Net: "B", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1.5}, End: Point{XMM: 5, YMM: 1.5}, WidthMM: 0.3}}},
	}
	if issues := clearanceIssues(routes, 0.2); len(issues) != 0 {
		t.Fatalf("exact declared clearance produced issues: %#v", issues)
	}
}

func TestRouteRequestEmitsTailSegmentsToOffGridPadCenters(t *testing.T) {
	request := singleLayerSearchRequest()
	request.Rules.GridMM = 0.25
	request.Components[0].Position = Placement{XMM: 1.95, YMM: 10, Layer: "F.Cu"}
	request.Components[1].Position = Placement{XMM: 8.05, YMM: 10, Layer: "F.Cu"}

	result := RouteRequest(request)
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues = %#v routes = %#v", result.Status, result.Issues, result.Routes)
	}
	for _, issue := range result.Issues {
		if issue.Code == "DISCONNECTED_PAD" || issue.Message == "route does not connect all intended endpoints" {
			t.Fatalf("unexpected disconnected endpoint issue: %#v", result.Issues)
		}
	}
	if len(result.Routes) != 1 || len(result.Routes[0].Segments) == 0 {
		t.Fatalf("routes = %#v", result.Routes)
	}
	first := result.Routes[0].Segments[0].Start
	last := result.Routes[0].Segments[len(result.Routes[0].Segments)-1].End
	if first != (Point{XMM: 1.95, YMM: 10}) || last != (Point{XMM: 8.05, YMM: 10}) {
		t.Fatalf("route endpoints = %#v -> %#v, want exact off-grid pad centers", first, last)
	}
}

func TestValidateResultDetectsInvalidVia(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Vias:   []Via{{Net: "SIG", At: Point{XMM: 5, YMM: 5}, DiameterMM: 0.6, DrillMM: 0.3, Layers: []string{"F.CU"}}},
	}}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "via must span at least two layers")
}

func TestRouteConnectivityJoinsSameLayerTJunction(t *testing.T) {
	route := Route{Segments: []Segment{
		{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
		{Layer: "F.Cu", Start: Point{XMM: 5, YMM: -5}, End: Point{XMM: 5, YMM: 0}},
	}}
	assertRoutePointsConnected(t, route, layerPoint{Point: Point{XMM: 0, YMM: 0}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 5, YMM: -5}, Layer: "F.Cu"})
}

func TestRouteConnectivityJoinsSameLayerCrossing(t *testing.T) {
	route := Route{Segments: []Segment{
		{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
		{Layer: "F.Cu", Start: Point{XMM: 5, YMM: -5}, End: Point{XMM: 5, YMM: 5}},
	}}
	assertRoutePointsConnected(t, route, layerPoint{Point: Point{XMM: 0, YMM: 0}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 5, YMM: 5}, Layer: "F.Cu"})
}

func TestRouteConnectivityJoinsViaOnSegmentInterior(t *testing.T) {
	route := Route{
		Segments: []Segment{
			{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
			{Layer: "B.Cu", Start: Point{XMM: 5, YMM: 0}, End: Point{XMM: 5, YMM: 10}},
		},
		Vias: []Via{{At: Point{XMM: 5, YMM: 0}, Layers: []string{"F.Cu", "B.Cu"}}},
	}
	assertRoutePointsConnected(t, route, layerPoint{Point: Point{XMM: 0, YMM: 0}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 5, YMM: 10}, Layer: "B.Cu"})
}

func TestRouteConnectivityDoesNotJoinDifferentLayerCrossing(t *testing.T) {
	route := Route{Segments: []Segment{
		{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
		{Layer: "B.Cu", Start: Point{XMM: 5, YMM: -5}, End: Point{XMM: 5, YMM: 5}},
	}}
	assertRoutePointsDisconnected(t, route, layerPoint{Point: Point{XMM: 0, YMM: 0}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 5, YMM: 5}, Layer: "B.Cu"})
}

func TestRouteConnectivityDoesNotJoinNearMiss(t *testing.T) {
	route := Route{Segments: []Segment{
		{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
		{Layer: "F.Cu", Start: Point{XMM: 5, YMM: 0.001}, End: Point{XMM: 5, YMM: 5}},
	}}
	assertRoutePointsDisconnected(t, route, layerPoint{Point: Point{XMM: 0, YMM: 0}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 5, YMM: 5}, Layer: "F.Cu"})
}

func TestRouteConnectivityJunctionsAreSegmentOrderIndependent(t *testing.T) {
	segments := []Segment{
		{Layer: "F.Cu", Start: Point{XMM: 0, YMM: 0}, End: Point{XMM: 10, YMM: 0}},
		{Layer: "F.Cu", Start: Point{XMM: 5, YMM: -5}, End: Point{XMM: 5, YMM: 0}},
		{Layer: "F.Cu", Start: Point{XMM: 10, YMM: 0}, End: Point{XMM: 15, YMM: 5}},
	}
	assertRoutePointsConnected(t, Route{Segments: segments}, layerPoint{Point: Point{XMM: 5, YMM: -5}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 15, YMM: 5}, Layer: "F.Cu"})
	for left, right := 0, len(segments)-1; left < right; left, right = left+1, right-1 {
		segments[left], segments[right] = segments[right], segments[left]
	}
	assertRoutePointsConnected(t, Route{Segments: segments}, layerPoint{Point: Point{XMM: 5, YMM: -5}, Layer: "F.Cu"}, layerPoint{Point: Point{XMM: 15, YMM: 5}, Layer: "F.Cu"})
}

func assertRoutePointsConnected(t *testing.T, route Route, left, right layerPoint) {
	t.Helper()
	graph := newRouteConnectivity(route)
	leftKey, leftOK := graph.nearestKey(left.Point, left.Layer)
	rightKey, rightOK := graph.nearestKey(right.Point, right.Layer)
	if !leftOK || !rightOK || graph.find(leftKey) != graph.find(rightKey) {
		t.Fatalf("route points are disconnected: left=%#v/%v right=%#v/%v", left, leftOK, right, rightOK)
	}
}

func assertRoutePointsDisconnected(t *testing.T, route Route, left, right layerPoint) {
	t.Helper()
	graph := newRouteConnectivity(route)
	leftKey, leftOK := graph.nearestKey(left.Point, left.Layer)
	rightKey, rightOK := graph.nearestKey(right.Point, right.Layer)
	if !leftOK || !rightOK {
		t.Fatalf("route points are missing: left=%#v/%v right=%#v/%v", left, leftOK, right, rightOK)
	}
	if graph.find(leftKey) == graph.find(rightKey) {
		t.Fatalf("route points are unexpectedly connected: left=%#v right=%#v", left, right)
	}
}

func assertValidationIssue(t *testing.T, report ValidationReport, message string) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Message == message {
			return
		}
	}
	t.Fatalf("missing validation issue %q in %#v", message, report.Issues)
}
