package placement

import (
	"math"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"kicadai/internal/reports"
)

const candidateScoringHPWLToleranceMM = 0.001

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
	if got.Weights.HardConstraints != 1 || got.Weights.Fanout != 1 || got.Weights.Mobility != 1 || got.Weights.Thermal != 1 || got.Weights.HighCurrent != 1 {
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

func TestPlaceCandidateScoringReportsSemanticRoleAndGroupDimensions(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Role = string(IntentDecoupling)
	req.Components[0].GroupID = "power"
	req.Groups = []Group{{
		ID:         "power",
		Components: []string{"R1"},
		Anchor:     GroupAnchor{At: &Point{XMM: 30, YMM: 20}},
	}}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.CandidateScoring == nil || len(result.CandidateScoring.WinningCandidates) != 1 {
		t.Fatalf("winning candidate scoring missing: %#v", result.CandidateScoring)
	}
	winner := result.CandidateScoring.WinningCandidates[0]
	if !candidateScoreHasDimension(winner, CandidateScoreSemanticRole) {
		t.Fatalf("semantic role dimension missing: %#v", winner.Dimensions)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreGroupCohesion) {
		t.Fatalf("group cohesion dimension missing: %#v", winner.Dimensions)
	}
}

func TestPlaceCandidateScoringOmitsGroupDimensionWithoutGroupMetadata(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Role = string(IntentConnector)
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.CandidateScoring == nil || len(result.CandidateScoring.WinningCandidates) != 1 {
		t.Fatalf("winning candidate scoring missing: %#v", result.CandidateScoring)
	}
	winner := result.CandidateScoring.WinningCandidates[0]
	if !candidateScoreHasDimension(winner, CandidateScoreSemanticRole) {
		t.Fatalf("semantic role dimension missing: %#v", winner.Dimensions)
	}
	if candidateScoreHasDimension(winner, CandidateScoreGroupCohesion) {
		t.Fatalf("unexpected group cohesion dimension: %#v", winner.Dimensions)
	}
}

func TestPlaceCandidateScoringReportsElectricalDimensions(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	winner, ok := candidateScoreForRef(result.CandidateScoring.WinningCandidates, "R2")
	if !ok {
		t.Fatalf("R2 winning candidate missing: %#v", result.CandidateScoring.WinningCandidates)
	}
	if winner.Total <= 0 || result.CandidateScoring.AverageWinningScore <= 0 {
		t.Fatalf("winning score summary missing: winner=%#v report=%#v", winner, result.CandidateScoring)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreElectricalProximity) {
		t.Fatalf("electrical proximity dimension missing: %#v", winner.Dimensions)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreRouteLength) {
		t.Fatalf("route length dimension missing: %#v", winner.Dimensions)
	}
}

func TestPlaceCandidateScoringReportsCongestionAndFanoutDimensions(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[1].Pads = []PadSummary{{Name: "1"}, {Name: "2"}, {Name: "3"}, {Name: "4"}}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	winner, ok := candidateScoreForRef(result.CandidateScoring.WinningCandidates, "R2")
	if !ok {
		t.Fatalf("R2 winning candidate missing: %#v", result.CandidateScoring.WinningCandidates)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreCongestion) {
		t.Fatalf("congestion dimension missing: %#v", winner.Dimensions)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreFanout) {
		t.Fatalf("fanout dimension missing: %#v", winner.Dimensions)
	}
}

func TestPlaceCandidateScoringReportsThermalDimension(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Role = "regulator"
	req.AdvancedRules.Thermal = []ThermalPlacementRule{{
		ID:            "thermal-edge",
		Roles:         []string{"regulator"},
		ThermalRole:   ThermalRoleRegulator,
		PreferredEdge: EdgeRight,
	}}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true
	req.Rules.CandidateScoring.Weights = CandidateScoreWeights{Thermal: 1}

	result := Place(req)
	winner, ok := candidateScoreForRef(result.CandidateScoring.WinningCandidates, "R1")
	if !ok {
		t.Fatalf("R1 winning candidate missing: %#v", result.CandidateScoring.WinningCandidates)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreThermal) {
		t.Fatalf("thermal dimension missing: %#v", winner.Dimensions)
	}
	if winner.Placement.XMM < req.Board.WidthMM/2 {
		t.Fatalf("thermal preferred-edge scoring did not move toward right edge: %#v", winner.Placement)
	}
}

func TestPlaceCandidateScoringRejectsHardThermalSpacing(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 20, YMM: 12, Layer: "F.Cu"}
	req.AdvancedRules.Thermal = []ThermalPlacementRule{{
		ID:            "hot-away",
		Refs:          []string{"R2"},
		KeepAwayRefs:  []string{"R1"},
		MinDistanceMM: 100,
		Enforcement:   AdvancedRuleHard,
	}}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	result := Place(req)
	if result.CandidateScoring == nil {
		t.Fatal("candidate scoring report missing")
	}
	if result.CandidateScoring.RejectedByReason[string(CandidateRejectAdvancedRule)] == 0 {
		t.Fatalf("advanced-rule rejections missing: %#v", result.CandidateScoring.RejectedByReason)
	}
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want partial because all R2 candidates violate hard thermal spacing", result.Status)
	}
}

func TestPlaceCandidateScoringReportsHighCurrentDimension(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 30, YMM: 12, Layer: "F.Cu"}
	req.AdvancedRules.HighCurrent = []HighCurrentPlacementRule{{
		ID:         "power-path",
		Nets:       []string{"N1"},
		SourceRefs: []string{"R1"},
		SinkRefs:   []string{"R2"},
	}}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true
	req.Rules.CandidateScoring.Weights = CandidateScoreWeights{HighCurrent: 1}

	result := Place(req)
	winner, ok := candidateScoreForRef(result.CandidateScoring.WinningCandidates, "R2")
	if !ok {
		t.Fatalf("R2 winning candidate missing: %#v", result.CandidateScoring.WinningCandidates)
	}
	if !candidateScoreHasDimension(winner, CandidateScoreHighCurrent) {
		t.Fatalf("high-current dimension missing: %#v", winner.Dimensions)
	}
	if winner.Placement.XMM < 20 {
		t.Fatalf("high-current scoring did not move sink toward source: %#v", winner.Placement)
	}
}

func TestCandidateScoringDeterministicAndGeometrySafe(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Rules = DefaultRules()
	req.Rules.CandidateScoring.Enabled = true

	first := Place(CloneRequest(req))
	second := Place(CloneRequest(req))
	if first.CandidateScoring == nil || second.CandidateScoring == nil {
		t.Fatalf("candidate scoring missing: first=%#v second=%#v", first.CandidateScoring, second.CandidateScoring)
	}
	if len(first.Placements) == 0 || len(second.Placements) == 0 {
		t.Fatalf("placements missing: first=%d second=%d", len(first.Placements), len(second.Placements))
	}
	if math.IsNaN(first.CandidateScoring.AverageWinningScore) || math.IsNaN(second.CandidateScoring.AverageWinningScore) ||
		math.IsNaN(first.CandidateScoring.LowestWinningScore) || math.IsNaN(second.CandidateScoring.LowestWinningScore) {
		t.Fatalf("candidate scoring contains NaN: first=%#v second=%#v", first.CandidateScoring, second.CandidateScoring)
	}
	if math.IsInf(first.CandidateScoring.AverageWinningScore, 0) || math.IsInf(second.CandidateScoring.AverageWinningScore, 0) ||
		math.IsInf(first.CandidateScoring.LowestWinningScore, 0) || math.IsInf(second.CandidateScoring.LowestWinningScore, 0) {
		t.Fatalf("candidate scoring contains infinity: first=%#v second=%#v", first.CandidateScoring, second.CandidateScoring)
	}
	if math.Abs(first.CandidateScoring.AverageWinningScore-second.CandidateScoring.AverageWinningScore) > 1e-9 ||
		math.Abs(first.CandidateScoring.LowestWinningScore-second.CandidateScoring.LowestWinningScore) > 1e-9 {
		t.Fatalf("candidate scoring not deterministic: first=%#v second=%#v", first.CandidateScoring, second.CandidateScoring)
	}
	if !placementsNearlyEqual(first.Placements, second.Placements) {
		t.Fatalf("placements not deterministic: first=%#v second=%#v", first.Placements, second.Placements)
	}
	geometryIssues := ValidateGeometry(req, first.Placements)
	if len(geometryIssues) != 0 {
		t.Fatalf("scored placement failed geometry validation: %#v", geometryIssues)
	}
}

func TestCandidateScoringPreservesOrImprovesHPWL(t *testing.T) {
	baseline := twoComponentRequest()
	baseline.Components[0].Fixed = true
	baseline.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	baseline.Rules = DefaultRules()
	baseline.Rules.CandidateScoring.Enabled = false
	scored := CloneRequest(baseline)
	scored.Rules.CandidateScoring.Enabled = true

	baselineResult := Place(baseline)
	scoredResult := Place(scored)
	if scoredResult.Metrics.PlacedCount != baselineResult.Metrics.PlacedCount {
		t.Fatalf("placed count changed: scored=%d baseline=%d", scoredResult.Metrics.PlacedCount, baselineResult.Metrics.PlacedCount)
	}
	if baselineResult.Metrics.PlacedCount < 2 {
		t.Fatalf("baseline placement did not exercise movable component: %#v", baselineResult.Metrics)
	}
	if scoredResult.Metrics.HPWLMM > baselineResult.Metrics.HPWLMM+candidateScoringHPWLToleranceMM {
		t.Fatalf("scored HPWL regressed: scored=%f baseline=%f placed=%d", scoredResult.Metrics.HPWLMM, baselineResult.Metrics.HPWLMM, scoredResult.Metrics.PlacedCount)
	}
}

func placementsNearlyEqual(first []PlacementResult, second []PlacementResult) bool {
	if len(first) != len(second) {
		return false
	}
	first = append([]PlacementResult(nil), first...)
	second = append([]PlacementResult(nil), second...)
	sort.Slice(first, func(i, j int) bool {
		return first[i].Ref < first[j].Ref
	})
	sort.Slice(second, func(i, j int) bool {
		return second[i].Ref < second[j].Ref
	})
	for index := range first {
		if first[index].Ref != second[index].Ref || first[index].FootprintID != second[index].FootprintID {
			return false
		}
		if !positionNearlyEqual(first[index].Position, second[index].Position) {
			return false
		}
	}
	return true
}

func positionNearlyEqual(first Placement, second Placement) bool {
	return math.Abs(first.XMM-second.XMM) <= 1e-9 &&
		math.Abs(first.YMM-second.YMM) <= 1e-9 &&
		math.Abs(first.RotationDeg-second.RotationDeg) <= 1e-9 &&
		first.Layer == second.Layer
}

func candidateScoreForRef(scores []CandidateScore, ref string) (CandidateScore, bool) {
	for _, score := range scores {
		if score.Ref == ref {
			return score, true
		}
	}
	return CandidateScore{}, false
}

func candidateScoreHasDimension(score CandidateScore, name CandidateScoreDimensionName) bool {
	for _, dimension := range score.Dimensions {
		if dimension.Name == name {
			return true
		}
	}
	return false
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
