package placement

import (
	"encoding/json"
	"testing"
)

func TestQualityReportScoresProximityWithPadEvidence(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
		Pads:        []PadSummary{{Name: "1", XMM: 0, YMM: 0}},
	})
	req.Components[0].Pads[0].XMM = 0
	req.Components[0].Pads[0].YMM = 0
	req.ProximityRules = []ProximityRule{{
		ID:            "decoupling",
		Role:          IntentDecoupling,
		AnchorRef:     "R1",
		TargetRefs:    []string{"C1"},
		AnchorPins:    []string{"1"},
		TargetPins:    []string{"1"},
		MaxDistanceMM: 5,
		Required:      true,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 8, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 {
		t.Fatalf("proximity reports = %#v", quality.ProximityReports)
	}
	report := quality.ProximityReports[0]
	if !report.Satisfied || report.Evidence != "pad" || report.ActualMM == nil {
		t.Fatalf("unexpected proximity report: %#v", report)
	}
	if len(quality.Score.Dimensions) == 0 || quality.Score.Dimensions[0].Status != "pass" {
		t.Fatalf("unexpected score: %#v", quality.Score)
	}
}

func TestQualityReportScoresFailedProximity(t *testing.T) {
	req := minimalRequest()
	req.Components = append(req.Components, Component{
		Ref:         "C1",
		FootprintID: "Capacitor_SMD:C_0805_2012Metric",
		Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsExplicit},
	})
	req.ProximityRules = []ProximityRule{{
		ID:            "far",
		AnchorRef:     "R1",
		TargetRefs:    []string{"C1"},
		MaxDistanceMM: 2,
		Required:      true,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 30, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 || quality.ProximityReports[0].Satisfied {
		t.Fatalf("unexpected proximity reports: %#v", quality.ProximityReports)
	}
	if len(quality.Score.Dimensions) == 0 || quality.Score.Dimensions[0].Status != "fail" {
		t.Fatalf("unexpected score: %#v", quality.Score)
	}
}

func TestQualityReportScoresGroupCohesion(t *testing.T) {
	req := twoComponentRequest()
	req.Groups = []Group{{
		ID:           "analog",
		Components:   []string{"R1", "R2"},
		KeepTogether: true,
		MaxSpreadMM:  2,
	}}
	result := Result{
		Status: StatusPlaced,
		Placements: []PlacementResult{
			mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5}),
			mustPlacementResultForTest(t, req.Components[1], Placement{XMM: 30, YMM: 5}),
		},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.GroupReports) != 1 || quality.GroupReports[0].SpreadSatisfied {
		t.Fatalf("unexpected group reports: %#v", quality.GroupReports)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "group_cohesion" {
			found = true
			if dimension.Status != "fail" {
				t.Fatalf("group cohesion status = %q, want fail", dimension.Status)
			}
		}
	}
	if !found {
		t.Fatalf("missing group cohesion score: %#v", quality.Score)
	}
}

func TestQualityReportScoresEdgeConstraints(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Edge = EdgeRight
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if quality.EdgeConstraintCount != 1 || quality.EdgeConstraintSatisfied != 0 {
		t.Fatalf("unexpected edge counts: %#v", quality)
	}
	var found bool
	for _, dimension := range quality.Score.Dimensions {
		if dimension.Name == "edge_constraints" {
			found = true
			if dimension.Status != "fail" || dimension.Score != 0 {
				t.Fatalf("edge dimension = %#v, want failing zero score", dimension)
			}
		}
	}
	if !found {
		t.Fatalf("missing edge score: %#v", quality.Score)
	}
}

func TestQualityReportMissingProximityTargetMarshalsJSON(t *testing.T) {
	req := minimalRequest()
	req.ProximityRules = []ProximityRule{{
		ID:         "missing",
		AnchorRef:  "R1",
		TargetRefs: []string{"C1"},
		Required:   true,
	}}
	result := Result{
		Status:     StatusPlaced,
		Placements: []PlacementResult{mustPlacementResultForTest(t, req.Components[0], Placement{XMM: 5, YMM: 5})},
	}

	quality := BuildQualityReport(req, result)
	if len(quality.ProximityReports) != 1 || quality.ProximityReports[0].ActualMM != nil {
		t.Fatalf("unexpected proximity report: %#v", quality.ProximityReports)
	}
	if _, err := json.Marshal(quality); err != nil {
		t.Fatalf("quality report should marshal without infinities: %v", err)
	}
}

func mustPlacementResultForTest(t *testing.T, component Component, position Placement) PlacementResult {
	t.Helper()
	result, ok := NewPlacementResult(component, position, DefaultRules())
	if !ok {
		t.Fatalf("failed to create placement result for %#v", component)
	}
	return result
}
