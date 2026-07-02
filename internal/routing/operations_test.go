package routing

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/reports"
)

func TestOperationsFromResultEmitsSegmentsAndViasInOrder(t *testing.T) {
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Segments: []Segment{{
			Net: "SIG", Layer: "F.CU", Start: Point{XMM: 1, YMM: 2}, End: Point{XMM: 3, YMM: 2}, WidthMM: 0.2,
		}},
		Vias: []Via{{
			Net: "SIG", At: Point{XMM: 3, YMM: 2}, DiameterMM: 0.6, DrillMM: 0.3, Layers: []string{"B.CU", "F.CU"},
		}},
	}}}

	operations := OperationsFromResult(result)
	if len(operations) != 2 {
		t.Fatalf("operations = %#v, want segment and via op", operations)
	}
	var segment RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &segment); err != nil {
		t.Fatal(err)
	}
	if segment.NetName != "SIG" || segment.Layer != "F.CU" || len(segment.Points) != 2 {
		t.Fatalf("segment op = %#v", segment)
	}
	var via RouteOperation
	if err := json.Unmarshal(operations[1].Raw, &via); err != nil {
		t.Fatal(err)
	}
	if len(via.Vias) != 1 || via.Vias[0].DrillMM != 0.3 {
		t.Fatalf("via op = %#v", via)
	}
}

func TestOperationsFromResultSkipsInvalidFloatPayload(t *testing.T) {
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Segments: []Segment{{
			Net: "SIG", Layer: "F.CU", Start: Point{XMM: math.NaN(), YMM: 2}, End: Point{XMM: 3, YMM: 2}, WidthMM: 0.2,
		}},
	}}}

	operations, issues := OperationsFromResultWithIssues(result)
	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want invalid operation skipped", operations)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want conversion issue", issues)
	}
}

func TestOperationsFromResultSkipsFailedRoutes(t *testing.T) {
	result := Result{Routes: []Route{
		{
			Net:    "OK",
			Status: RouteStatusRouted,
			Segments: []Segment{{
				Net: "OK", Layer: "F.CU", Start: Point{XMM: 1, YMM: 2}, End: Point{XMM: 3, YMM: 2}, WidthMM: 0.2,
			}},
		},
		{
			Net:    "FAIL",
			Status: RouteStatusFailed,
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityBlocked,
				Message:  "route failed",
			}},
			Segments: []Segment{{
				Net: "FAIL", Layer: "F.CU", Start: Point{XMM: 4, YMM: 2}, End: Point{XMM: 6, YMM: 2}, WidthMM: 0.2,
			}},
		},
	}}

	operations := OperationsFromResult(result)
	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want only routed route operation", operations)
	}
	var payload RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.NetName != "OK" {
		t.Fatalf("payload = %#v, want OK route only", payload)
	}
}

func TestOperationsFromResultGroupsContiguousSegments(t *testing.T) {
	result := Result{Routes: []Route{{
		Net:    "SIG",
		Status: RouteStatusRouted,
		Segments: []Segment{
			{Net: "SIG", Layer: "F.CU", Start: Point{XMM: 1, YMM: 1}, End: Point{XMM: 2, YMM: 1}, WidthMM: 0.2},
			{Net: "SIG", Layer: "F.CU", Start: Point{XMM: 2, YMM: 1}, End: Point{XMM: 3, YMM: 1}, WidthMM: 0.2},
		},
	}}}

	operations := OperationsFromResult(result)
	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want grouped operation", operations)
	}
	var payload RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Points) != 3 {
		t.Fatalf("points = %#v, want polyline", payload.Points)
	}
}

func TestRouteRequestIncludesOperations(t *testing.T) {
	result := RouteRequest(singleLayerSearchRequest())
	if result.Status != StatusRouted {
		t.Fatalf("status = %s issues = %#v", result.Status, result.Issues)
	}
	if len(result.Operations) == 0 {
		t.Fatalf("operations = %#v, want route operations", result.Operations)
	}
}
