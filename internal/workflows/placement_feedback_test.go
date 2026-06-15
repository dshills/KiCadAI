package workflows

import (
	"strings"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func TestEvaluatePlacementReadyForRouting(t *testing.T) {
	req := workflowPlacementRequest()
	result := placement.Place(req)

	feedback := EvaluatePlacement(req, result)
	if feedback.Status != FeedbackStatusReady {
		t.Fatalf("feedback = %#v, want ready", feedback)
	}
	if feedback.Placement == nil || feedback.Placement.Metrics.PlacedCount != 1 {
		t.Fatalf("placement feedback = %#v, want placed metrics", feedback.Placement)
	}
	if !strings.Contains(feedback.Summary, "ready for routing") {
		t.Fatalf("summary = %q", feedback.Summary)
	}
	if len(feedback.Diagnostics) != 1 || feedback.Diagnostics[0].Category != placement.PlacementDiagnosticRoutingReadiness {
		t.Fatalf("diagnostics = %#v, want readiness diagnostic", feedback.Diagnostics)
	}
}

func TestEvaluatePlacementNeedsRevisionForEstimatedBounds(t *testing.T) {
	req := workflowPlacementRequest()
	req.Components[0].Bounds.Source = placement.BoundsEstimated
	result := placement.Place(req)

	feedback := EvaluatePlacement(req, result)
	if feedback.Status != FeedbackStatusNeedsRevision {
		t.Fatalf("feedback = %#v, want needs revision", feedback)
	}
	if len(feedback.Placement.Quality.EstimatedBoundsRefs) != 1 || feedback.Placement.Quality.EstimatedBoundsRefs[0] != "R1" {
		t.Fatalf("estimated refs = %#v", feedback.Placement.Quality.EstimatedBoundsRefs)
	}
	if !hasRecommendationContaining(feedback.Recommendations, "footprints with valid courtyard") {
		t.Fatalf("recommendations = %#v", feedback.Recommendations)
	}
}

func TestEvaluatePlacementBlockedForUnplacedComponents(t *testing.T) {
	req := workflowPlacementRequest()
	req.Board.WidthMM = 2
	req.Board.HeightMM = 2
	result := placement.Place(req)

	feedback := EvaluatePlacement(req, result)
	if feedback.Status != FeedbackStatusBlocked {
		t.Fatalf("feedback = %#v, want blocked", feedback)
	}
	if !strings.Contains(feedback.Summary, "unplaced components") {
		t.Fatalf("summary = %q", feedback.Summary)
	}
	if len(feedback.Placement.Quality.UnplacedRefs) != 1 || feedback.Placement.Quality.UnplacedRefs[0] != "R1" {
		t.Fatalf("unplaced refs = %#v", feedback.Placement.Quality.UnplacedRefs)
	}
}

func TestPlacementFeedbackSummaryLimitsUnplacedRefs(t *testing.T) {
	summary := placementFeedbackSummary(FeedbackStatusBlocked, placement.QualityReport{
		UnplacedRefs: []string{"R1", "R2", "R3", "R4", "R5", "R6"},
	}, nil)
	if !strings.Contains(summary, "and 1 more") {
		t.Fatalf("summary = %q, want truncated ref list", summary)
	}
}

func TestPlacementFeedbackSummaryHandlesNonPositiveLimit(t *testing.T) {
	if got := summarizedRefs([]string{"R1", "R2"}, 0); got != "2 components" {
		t.Fatalf("summarizedRefs = %q, want component count", got)
	}
}

func TestPlacementFeedbackSummaryUsesBlockingDiagnostic(t *testing.T) {
	summary := placementFeedbackSummary(FeedbackStatusBlocked, placement.QualityReport{}, []placement.PlacementDiagnostic{{
		Severity: reports.SeverityError,
		Message:  "fixed placement conflicts with R2",
	}})
	if !strings.Contains(summary, "fixed placement conflicts") {
		t.Fatalf("summary = %q, want diagnostic message", summary)
	}
}

func workflowPlacementRequest() placement.Request {
	return placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 40, HeightMM: 25, MarginMM: 1},
		Components: []placement.Component{{
			Ref:         "R1",
			FootprintID: "Resistor_SMD:R_0805_2012Metric",
			Bounds:      placement.Bounds{WidthMM: 2, HeightMM: 1.25, Source: placement.BoundsLibraryCourtyard},
			Pads:        []placement.PadSummary{{Name: "1"}, {Name: "2"}},
		}},
		Nets: []placement.Net{{Name: "N1", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "1"}}}},
	}
}

func hasRecommendationContaining(recommendations []string, want string) bool {
	for _, recommendation := range recommendations {
		if strings.Contains(recommendation, want) {
			return true
		}
	}
	return false
}
