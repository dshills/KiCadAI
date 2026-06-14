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
