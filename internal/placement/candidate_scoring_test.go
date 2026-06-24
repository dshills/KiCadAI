package placement

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"kicadai/internal/reports"
)

func TestNormalizeCandidateScoringRulesDefaults(t *testing.T) {
	got := normalizeCandidateScoringRules(DefaultRules().CandidateScoring)
	if got.Enabled || got.Policy != defaultCandidateScoringPolicy {
		t.Fatalf("rules = %#v", got)
	}
	if got.MaxAlternativesPerComponent != defaultCandidateAlternativesPerComponent {
		t.Fatalf("max alternatives = %d", got.MaxAlternativesPerComponent)
	}
	if got.MaxEvidencePerDimension != defaultCandidateScoreEvidencePerDimension {
		t.Fatalf("max evidence = %d", got.MaxEvidencePerDimension)
	}
	if got.Weights.HardConstraints != 1 || got.Weights.Fanout != 1 || got.Weights.Mobility != 1 {
		t.Fatalf("weights not defaulted: %#v", got.Weights)
	}
}

func TestNormalizeCandidateScoringRulesPreservesExplicitZero(t *testing.T) {
	got := normalizeCandidateScoringRules(CandidateScoringRules{
		Enabled:                     true,
		MaxAlternativesPerComponent: 0,
		MaxEvidencePerDimension:     0,
		Weights: CandidateScoreWeights{
			HardConstraints: -1,
			SemanticRole:    math.NaN(),
			GroupCohesion:   math.Inf(1),
			Fanout:          0,
		},
	})
	if got.MaxAlternativesPerComponent != 0 || got.MaxEvidencePerDimension != 0 {
		t.Fatalf("zero limits should be preserved: %#v", got)
	}
	if got.Weights.HardConstraints != 0 || got.Weights.Fanout != 0 {
		t.Fatalf("zero/negative weights should normalize to disabled: %#v", got.Weights)
	}
	if got.Weights.SemanticRole != 0 || got.Weights.GroupCohesion != 0 {
		t.Fatalf("non-finite weights should normalize to disabled: %#v", got.Weights)
	}
}

func TestNormalizeCandidateScoringRulesCapsLimits(t *testing.T) {
	got := normalizeCandidateScoringRules(CandidateScoringRules{
		MaxAlternativesPerComponent: maxCandidateAlternativesPerComponent + 1,
		MaxEvidencePerDimension:     maxCandidateScoreEvidencePerDimension + 1,
	})
	if got.MaxAlternativesPerComponent != maxCandidateAlternativesPerComponent {
		t.Fatalf("max alternatives = %d", got.MaxAlternativesPerComponent)
	}
	if got.MaxEvidencePerDimension != maxCandidateScoreEvidencePerDimension {
		t.Fatalf("max evidence = %d", got.MaxEvidencePerDimension)
	}
}

func TestPlaceCandidateScoringReportIsOptIn(t *testing.T) {
	req := minimalRequest()
	if result := Place(req); result.CandidateScoring != nil {
		t.Fatalf("candidate scoring report should be omitted by default: %#v", result.CandidateScoring)
	}
	req.Rules.CandidateScoring.Enabled = true
	result := Place(req)
	if result.CandidateScoring == nil || !result.CandidateScoring.Enabled || result.CandidateScoring.ScoreVersion == "" {
		t.Fatalf("candidate scoring report missing when enabled: %#v", result.CandidateScoring)
	}
}

func TestPlaceCandidateScoringReportsOutsideBoardRejections(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 2
	req.Board.HeightMM = 2
	req.Board.MarginMM = 0
	req.Rules.BoardEdgeClearanceMM = 0.1
	req.Rules.CandidateScoring.Enabled = true
	req.Rules.CandidateScoring.MaxAlternativesPerComponent = 1

	result := Place(req)
	if result.CandidateScoring == nil {
		t.Fatal("candidate scoring report missing")
	}
	if result.CandidateScoring.RejectedByReason[string(CandidateRejectOutsideBoard)] == 0 {
		t.Fatalf("outside-board rejections missing: %#v", result.CandidateScoring.RejectedByReason)
	}
	if len(result.CandidateScoring.AlternativeCandidates) != 1 || !result.CandidateScoring.AlternativeCandidates[0].Rejected {
		t.Fatalf("bounded rejected alternative missing: %#v", result.CandidateScoring.AlternativeCandidates)
	}
}

func TestPlaceCandidateScoringReportsCollisionRejections(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.CandidateScoring == nil {
		t.Fatal("candidate scoring report missing")
	}
	if result.CandidateScoring.RejectedByReason[string(CandidateRejectCollision)] == 0 {
		t.Fatalf("collision rejections missing: %#v placements=%#v", result.CandidateScoring.RejectedByReason, result.Placements)
	}
	if issues := ValidateGeometry(req, result.Placements); len(issues) != 0 {
		t.Fatalf("collision rejection allowed invalid winner: %#v", issues)
	}
}

func TestPlaceCandidateScoringReportsFixedKeepoutRejection(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 4, YMM: 4}, Max: Point{XMM: 6, YMM: 6}},
		Layers: []string{"F.Cu"},
	}}
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.CandidateScoring == nil {
		t.Fatal("candidate scoring report missing")
	}
	if result.CandidateScoring.RejectedByReason[string(CandidateRejectKeepout)] != 1 {
		t.Fatalf("keepout rejection count = %#v", result.CandidateScoring.RejectedByReason)
	}
}

func TestNormalizeCandidateScoringReportSortsAndBoundsEvidence(t *testing.T) {
	longEvidence := strings.Repeat("a", candidateScoreMaxEvidenceLength+20)
	report := CandidateScoringReport{
		Policy:              " custom ",
		AverageWinningScore: 1.23456,
		LowestWinningScore:  0.33333,
		RejectedByReason: map[string]int{
			"":          2,
			" keepout ": 3,
			"keepout":   2,
		},
		AlternativeCandidates: []CandidateScore{
			{
				Ref:   " U2 ",
				Index: 2,
				Total: 1,
				Dimensions: []CandidateScoreDimension{
					{Name: CandidateScoreFanout, Score: 0.33333, Weight: 1, Evidence: []string{"b", "a", "a"}},
				},
			},
			{
				Ref:   " U1 ",
				Index: 3,
				Total: 1,
			},
			{
				Ref:   " U1 ",
				Index: 1,
				Total: 2.22229,
				Reasons: []CandidateRejectionReason{{
					Name:     CandidateRejectCollision,
					Severity: reports.SeverityError,
					Message:  " collision ",
					Refs:     []string{"R2", "R1", "R1"},
				}},
				Evidence: []string{longEvidence, "second", "third"},
			},
		},
	}
	got := NormalizeCandidateScoringReport(report, CandidateScoringRules{MaxAlternativesPerComponent: 1, MaxEvidencePerDimension: 2})
	if got.Policy != "custom" || got.ScoreVersion != defaultCandidateScoringVersion {
		t.Fatalf("metadata = %#v", got)
	}
	if got.AverageWinningScore != 1.235 || got.LowestWinningScore != 0.333 {
		t.Fatalf("scores not rounded: %#v", got)
	}
	if got.RejectedByReason["keepout"] != 5 || len(got.RejectedByReason) != 1 {
		t.Fatalf("rejected counts = %#v", got.RejectedByReason)
	}
	if len(got.AlternativeCandidates) != 2 || got.AlternativeCandidates[0].Ref != "U1" || got.AlternativeCandidates[1].Ref != "U2" {
		t.Fatalf("candidate ordering/bounding = %#v", got.AlternativeCandidates)
	}
	candidate := got.AlternativeCandidates[0]
	if len(candidate.Evidence) != 2 || len(candidate.Evidence[0]) != candidateScoreMaxEvidenceLength {
		t.Fatalf("evidence not bounded: %#v", candidate.Evidence)
	}
	if len(candidate.Reasons) != 1 || len(candidate.Reasons[0].Refs) != 2 || candidate.Reasons[0].Refs[0] != "R1" {
		t.Fatalf("reasons not normalized: %#v", candidate.Reasons)
	}
}

func TestNormalizeCandidateEvidencePreservesUTF8(t *testing.T) {
	evidence := normalizeCandidateEvidence([]string{strings.Repeat("µ", candidateScoreMaxEvidenceLength+1)}, 1)
	if len(evidence) != 1 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if !utf8.ValidString(evidence[0]) {
		t.Fatalf("evidence is not valid UTF-8: %q", evidence[0])
	}
	if got := len([]rune(evidence[0])); got != candidateScoreMaxEvidenceLength {
		t.Fatalf("rune length = %d", got)
	}
}

func TestNormalizeCandidateScoreDimensionsDeterministic(t *testing.T) {
	dimensions := normalizeCandidateScoreDimensions([]CandidateScoreDimension{
		{Name: CandidateScoreRouteLength, Score: 1, Weight: 1},
		{Name: CandidateScoreFanout, Score: 2, Weight: 1},
		{Name: CandidateScoreFanout, Score: 1, Weight: 1},
	}, 1)
	if len(dimensions) != 3 {
		t.Fatalf("dimensions = %#v", dimensions)
	}
	if dimensions[0].Name != CandidateScoreFanout || dimensions[0].Score != 2 {
		t.Fatalf("dimension order = %#v", dimensions)
	}
	if dimensions[2].Name != CandidateScoreRouteLength {
		t.Fatalf("dimension order = %#v", dimensions)
	}
}
