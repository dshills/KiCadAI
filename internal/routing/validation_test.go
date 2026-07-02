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
	assertValidationIssue(t, report, "segment clearance violation with net B")
}

func TestValidateResultDetectsCrossedTraceClearanceViolation(t *testing.T) {
	request := singleLayerSearchRequest()
	result := Result{Routes: []Route{
		{Net: "A", Status: RouteStatusRouted, Segments: []Segment{{Net: "A", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 5, YMM: 5}, WidthMM: 0.1}}},
		{Net: "B", Status: RouteStatusRouted, Segments: []Segment{{Net: "B", Layer: "F.CU", Start: Point{XMM: 1, YMM: 5}, End: Point{XMM: 5, YMM: 1}, WidthMM: 0.1}}},
	}}

	report := ValidateResult(request, result)
	assertValidationIssue(t, report, "segment clearance violation with net B")
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

func assertValidationIssue(t *testing.T, report ValidationReport, message string) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Message == message {
			return
		}
	}
	t.Fatalf("missing validation issue %q in %#v", message, report.Issues)
}
