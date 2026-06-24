package placement

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	defaultCandidateScoringPolicy                 = "semantic_v1"
	defaultCandidateScoringVersion                = "candidate-scoring.v1"
	defaultCandidateAlternativesPerComponent      = 3
	defaultCandidateScoreEvidencePerDimension     = 4
	maxCandidateAlternativesPerComponent          = 32
	maxCandidateScoreEvidencePerDimension         = 16
	maxCandidateRejectedSamplesPerComponent       = 64
	candidateScoreFloatPrecision              int = 1000
	candidateScoreMaxEvidenceLength           int = 160
	candidatePlacementCompareEpsilon              = 1e-9
)

func normalizeCandidateScoringRules(rules CandidateScoringRules) CandidateScoringRules {
	rules.Policy = strings.TrimSpace(rules.Policy)
	if rules.Policy == "" {
		rules.Policy = defaultCandidateScoringPolicy
	}
	if rules.MaxAlternativesPerComponent < 0 {
		rules.MaxAlternativesPerComponent = defaultCandidateAlternativesPerComponent
	}
	if rules.MaxAlternativesPerComponent > maxCandidateAlternativesPerComponent {
		rules.MaxAlternativesPerComponent = maxCandidateAlternativesPerComponent
	}
	if rules.MaxEvidencePerDimension < 0 {
		rules.MaxEvidencePerDimension = defaultCandidateScoreEvidencePerDimension
	}
	if rules.MaxEvidencePerDimension > maxCandidateScoreEvidencePerDimension {
		rules.MaxEvidencePerDimension = maxCandidateScoreEvidencePerDimension
	}
	rules.Weights = normalizeCandidateScoreWeights(rules.Weights)
	return rules
}

func normalizeCandidateScoreWeights(weights CandidateScoreWeights) CandidateScoreWeights {
	weights.HardConstraints = normalizeCandidateScoreWeight(weights.HardConstraints)
	weights.SemanticRole = normalizeCandidateScoreWeight(weights.SemanticRole)
	weights.GroupCohesion = normalizeCandidateScoreWeight(weights.GroupCohesion)
	weights.ElectricalProximity = normalizeCandidateScoreWeight(weights.ElectricalProximity)
	weights.RouteLength = normalizeCandidateScoreWeight(weights.RouteLength)
	weights.Congestion = normalizeCandidateScoreWeight(weights.Congestion)
	weights.Fanout = normalizeCandidateScoreWeight(weights.Fanout)
	weights.Edge = normalizeCandidateScoreWeight(weights.Edge)
	weights.Region = normalizeCandidateScoreWeight(weights.Region)
	weights.Mobility = normalizeCandidateScoreWeight(weights.Mobility)
	return weights
}

func normalizeCandidateScoreWeight(weight float64) float64 {
	if weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
		return 0
	}
	return weight
}

func NewCandidateScoringReport(rules CandidateScoringRules) *CandidateScoringReport {
	rules = normalizeCandidateScoringRules(rules)
	if !rules.Enabled {
		return nil
	}
	return &CandidateScoringReport{
		Enabled:      true,
		Policy:       rules.Policy,
		ScoreVersion: defaultCandidateScoringVersion,
	}
}

func NormalizeCandidateScoringReport(report CandidateScoringReport, rules CandidateScoringRules) CandidateScoringReport {
	rules = normalizeCandidateScoringRules(rules)
	report.Policy = strings.TrimSpace(report.Policy)
	if report.Policy == "" {
		report.Policy = rules.Policy
	}
	report.ScoreVersion = strings.TrimSpace(report.ScoreVersion)
	if report.ScoreVersion == "" {
		report.ScoreVersion = defaultCandidateScoringVersion
	}
	report.AverageWinningScore = roundCandidateScore(report.AverageWinningScore)
	report.LowestWinningScore = roundCandidateScore(report.LowestWinningScore)
	report.AggregateDimensions = normalizeCandidateScoreDimensions(report.AggregateDimensions, rules.MaxEvidencePerDimension)
	report.TopPenalties = normalizeCandidateScoreDimensions(report.TopPenalties, rules.MaxEvidencePerDimension)
	report.WinningCandidates = normalizeCandidateScores(report.WinningCandidates, rules.MaxEvidencePerDimension)
	report.AlternativeCandidates = normalizeCandidateScores(report.AlternativeCandidates, rules.MaxEvidencePerDimension)
	report.AlternativeCandidates = limitCandidateScoresPerRef(report.AlternativeCandidates, rules.MaxAlternativesPerComponent)
	report.RejectedByReason = normalizeCandidateReasonCounts(report.RejectedByReason)
	return report
}

func recordCandidateRejection(report *CandidateScoringReport, component Component, refKey string, placement Placement, index int, reason CandidateRejectionReasonName, message string, refs ...string) {
	if report == nil {
		return
	}
	if report.RejectedByReason == nil {
		report.RejectedByReason = map[string]int{}
	}
	if report.rejectedSamplesByRef == nil {
		report.rejectedSamplesByRef = map[string]int{}
	}
	reasonKey := string(reason)
	if reasonKey == "" {
		reasonKey = string(CandidateRejectUnsupportedPolicy)
	}
	report.RejectedByReason[reasonKey]++
	if refKey == "" {
		refKey = component.Ref
	}
	if report.rejectedSamplesByRef[refKey] >= maxCandidateRejectedSamplesPerComponent {
		return
	}
	report.rejectedSamplesByRef[refKey]++
	report.AlternativeCandidates = append(report.AlternativeCandidates, CandidateScore{
		Ref:       component.Ref,
		Role:      component.Role,
		Index:     index,
		Placement: normalizePlacementLayer(placement),
		Rejected:  true,
		Total:     0,
		Reasons: []CandidateRejectionReason{{
			Name:     reason,
			Severity: reports.SeverityError,
			Message:  message,
			Refs:     refs,
		}},
		Evidence: []string{message},
	})
}

func recordCandidateWinner(report *CandidateScoringReport, component Component, placement PlacementResult, candidate placementCandidate) {
	if report == nil {
		return
	}
	report.WinningCandidates = append(report.WinningCandidates, CandidateScore{
		Ref:        component.Ref,
		Role:       component.Role,
		Index:      candidate.Index,
		Placement:  placement.Position,
		Total:      candidate.Total,
		Dimensions: candidate.Dimensions,
		Evidence:   []string{"selected placement candidate"},
	})
}

func weightedCandidateDimensionTotal(dimensions []CandidateScoreDimension) float64 {
	if len(dimensions) == 0 {
		return 0
	}
	totalWeight := 0.0
	total := 0.0
	for _, dimension := range dimensions {
		if dimension.Weight <= 0 {
			continue
		}
		totalWeight += dimension.Weight
		total += dimension.Score * dimension.Weight
	}
	if totalWeight == 0 {
		return 0
	}
	return total / totalWeight
}

func semanticCandidateDimensions(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, groupTarget Point, hasGroupTarget bool) []CandidateScoreDimension {
	weights := request.Rules.CandidateScoring.Weights
	dimensions := make([]CandidateScoreDimension, 0, 2)
	if component.Role != "" && weights.SemanticRole > 0 {
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:     CandidateScoreSemanticRole,
			Score:    1,
			Weight:   weights.SemanticRole,
			Evidence: []string{"role=" + component.Role},
		})
	}
	if weights.GroupCohesion > 0 {
		groupDimension, ok := groupCohesionCandidateDimension(component, placement, request, anchor, hasAnchor, groupTarget, hasGroupTarget, weights.GroupCohesion)
		if ok {
			dimensions = append(dimensions, groupDimension)
		}
	}
	return dimensions
}

type electricalCandidateScoringContext struct {
	Targets             []netScoreTarget
	Weights             CandidateScoreWeights
	ProximityNormalizer float64
	RouteNormalizer     float64
}

func newElectricalCandidateScoringContext(request Request, netTargets []netScoreTarget) electricalCandidateScoringContext {
	weightSum := netTargetWeightSum(netTargets)
	if weightSum <= 0 {
		weightSum = 1
	}
	proximityNormalizer := boardDistance(request.Board.WidthMM, request.Board.HeightMM) * weightSum
	if proximityNormalizer <= 0 {
		proximityNormalizer = 1
	}
	routeNormalizer := (request.Board.WidthMM + request.Board.HeightMM) * weightSum
	if routeNormalizer <= 0 {
		routeNormalizer = 1
	}
	return electricalCandidateScoringContext{
		Targets:             netTargets,
		Weights:             request.Rules.CandidateScoring.Weights,
		ProximityNormalizer: proximityNormalizer,
		RouteNormalizer:     routeNormalizer,
	}
}

func appendElectricalCandidateDimensions(dimensions []CandidateScoreDimension, placement Placement, context electricalCandidateScoringContext, rotatedPadsByName map[string]Point) []CandidateScoreDimension {
	if len(context.Targets) == 0 {
		return dimensions
	}
	if context.Weights.ElectricalProximity > 0 {
		proximityDistance := netEuclideanDistanceScore(placement, context.Targets, rotatedPadsByName)
		proximityScore := clampCandidateUnitScore(1 - proximityDistance/context.ProximityNormalizer)
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreElectricalProximity,
			Score:  proximityScore,
			Weight: context.Weights.ElectricalProximity,
		})
	}
	if context.Weights.RouteLength > 0 {
		routeDistance := netManhattanDistanceScore(placement, context.Targets, rotatedPadsByName)
		routeScore := clampCandidateUnitScore(1 - routeDistance/context.RouteNormalizer)
		dimensions = append(dimensions, CandidateScoreDimension{
			Name:   CandidateScoreRouteLength,
			Score:  routeScore,
			Weight: context.Weights.RouteLength,
		})
	}
	return dimensions
}

func clampCandidateUnitScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func netEuclideanDistanceScore(placement Placement, targets []netScoreTarget, rotatedPadsByName map[string]Point) float64 {
	total := 0.0
	for _, target := range targets {
		point := absoluteComponentPadPoint(rotatedPadsByName, target.CurrentPin, placement)
		dx := point.XMM - target.Target.XMM
		dy := point.YMM - target.Target.YMM
		total += netTargetEffectiveWeight(target) * boardDistance(dx, dy)
	}
	return total
}

func netTargetWeightSum(targets []netScoreTarget) float64 {
	total := 0.0
	for _, target := range targets {
		total += netTargetEffectiveWeight(target)
	}
	return total
}

func netTargetEffectiveWeight(target netScoreTarget) float64 {
	if target.Weight <= 0 {
		return 1
	}
	return float64(target.Weight)
}

func netManhattanDistanceScore(placement Placement, targets []netScoreTarget, rotatedPadsByName map[string]Point) float64 {
	total := 0.0
	for _, target := range targets {
		point := absoluteComponentPadPoint(rotatedPadsByName, target.CurrentPin, placement)
		total += netTargetEffectiveWeight(target) * (math.Abs(point.XMM-target.Target.XMM) + math.Abs(point.YMM-target.Target.YMM))
	}
	return total
}

func groupCohesionCandidateDimension(component Component, placement Placement, request Request, anchor Point, hasAnchor bool, groupTarget Point, hasGroupTarget bool, weight float64) (CandidateScoreDimension, bool) {
	if component.GroupID == "" || (!hasAnchor && !hasGroupTarget) {
		return CandidateScoreDimension{}, false
	}
	boardDiagonal := boardDistance(request.Board.WidthMM, request.Board.HeightMM)
	if boardDiagonal <= 0 {
		boardDiagonal = 1
	}
	score := 0.0
	evidence := []string{"group=" + component.GroupID}
	count := 0.0
	if hasAnchor {
		distance := boardDistance(placement.XMM-anchor.XMM, placement.YMM-anchor.YMM)
		score += 1 - min(1, distance/boardDiagonal)
		count++
		evidence = append(evidence, fmt.Sprintf("anchor_distance_mm=%.3f", distance))
	}
	if hasGroupTarget {
		distance := boardDistance(placement.XMM-groupTarget.XMM, placement.YMM-groupTarget.YMM)
		score += 1 - min(1, distance/boardDiagonal)
		count++
		evidence = append(evidence, fmt.Sprintf("peer_distance_mm=%.3f", distance))
	}
	if count == 0 {
		return CandidateScoreDimension{}, false
	}
	return CandidateScoreDimension{
		Name:     CandidateScoreGroupCohesion,
		Score:    score / count,
		Weight:   weight,
		Evidence: evidence,
	}, true
}

func candidateRejectionReasonForConflict(conflict occupancyConflict) CandidateRejectionReasonName {
	if conflict.Kind == occupancyConflictKeepout {
		return CandidateRejectKeepout
	}
	return CandidateRejectCollision
}

func limitCandidateScoresPerRef(scores []CandidateScore, limit int) []CandidateScore {
	if limit < 0 {
		return scores
	}
	counts := make(map[string]int, len(scores))
	out := make([]CandidateScore, 0, len(scores))
	for _, score := range scores {
		ref := score.Ref
		if ref == "" {
			ref = "<unknown>"
		}
		if counts[ref] >= limit {
			continue
		}
		counts[ref]++
		out = append(out, score)
	}
	return out
}

func normalizeCandidateScores(scores []CandidateScore, evidenceLimit int) []CandidateScore {
	out := slices.Clone(scores)
	for index := range out {
		out[index].Ref = strings.TrimSpace(out[index].Ref)
		out[index].Role = strings.TrimSpace(out[index].Role)
		out[index].Total = roundCandidateScore(out[index].Total)
		out[index].Evidence = normalizeCandidateEvidence(out[index].Evidence, evidenceLimit)
		out[index].Dimensions = normalizeCandidateScoreDimensions(out[index].Dimensions, evidenceLimit)
		out[index].Reasons = normalizeCandidateRejectionReasons(out[index].Reasons)
	}
	slices.SortStableFunc(out, func(left, right CandidateScore) int {
		if candidateScoreLess(left, right) {
			return -1
		}
		if candidateScoreLess(right, left) {
			return 1
		}
		return 0
	})
	return out
}

func normalizeCandidateScoreDimensions(dimensions []CandidateScoreDimension, evidenceLimit int) []CandidateScoreDimension {
	out := slices.Clone(dimensions)
	for index := range out {
		out[index].Name = CandidateScoreDimensionName(strings.TrimSpace(string(out[index].Name)))
		out[index].Score = roundCandidateScore(out[index].Score)
		out[index].Weight = roundCandidateScore(out[index].Weight)
		out[index].Evidence = normalizeCandidateEvidence(out[index].Evidence, evidenceLimit)
	}
	slices.SortStableFunc(out, func(left, right CandidateScoreDimension) int {
		if left.Name != right.Name {
			return compareOrdered(left.Name, right.Name)
		}
		if left.Score != right.Score {
			return compareOrdered(right.Score, left.Score)
		}
		return compareOrdered(right.Weight, left.Weight)
	})
	return out
}

func normalizeCandidateRejectionReasons(reasons []CandidateRejectionReason) []CandidateRejectionReason {
	out := slices.Clone(reasons)
	for index := range out {
		out[index].Name = CandidateRejectionReasonName(strings.TrimSpace(string(out[index].Name)))
		out[index].Message = strings.TrimSpace(out[index].Message)
		out[index].Refs = normalizeCandidateReasonRefs(out[index].Refs)
	}
	slices.SortStableFunc(out, func(left, right CandidateRejectionReason) int {
		if left.Name != right.Name {
			return compareOrdered(left.Name, right.Name)
		}
		if left.Severity != right.Severity {
			return compareOrdered(left.Severity, right.Severity)
		}
		return compareOrdered(left.Message, right.Message)
	})
	return out
}

func normalizeCandidateReasonCounts(counts map[string]int) map[string]int {
	if len(counts) == 0 {
		return nil
	}
	out := make(map[string]int, len(counts))
	for key, value := range counts {
		key = strings.TrimSpace(key)
		if key == "" || value <= 0 {
			continue
		}
		out[key] += value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeCandidateReasonRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	slices.Sort(out)
	return out
}

func normalizeCandidateEvidence(evidence []string, limit int) []string {
	if limit <= 0 || len(evidence) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(evidence))
	out := make([]string, 0, len(evidence))
	for _, item := range evidence {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		item = truncateRunes(item, candidateScoreMaxEvidenceLength)
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	slices.Sort(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range value {
		if count == limit {
			return value[:index]
		}
		count++
	}
	return value
}

func candidateScoreLess(left CandidateScore, right CandidateScore) bool {
	if left.Ref != right.Ref {
		return left.Ref < right.Ref
	}
	if left.Rejected != right.Rejected {
		return !left.Rejected
	}
	if left.Total != right.Total {
		return left.Total > right.Total
	}
	if left.Index != right.Index {
		return left.Index < right.Index
	}
	return candidatePlacementLess(left.Placement, right.Placement)
}

func candidatePlacementLess(left Placement, right Placement) bool {
	if math.Abs(left.YMM-right.YMM) > candidatePlacementCompareEpsilon {
		return left.YMM < right.YMM
	}
	if math.Abs(left.XMM-right.XMM) > candidatePlacementCompareEpsilon {
		return left.XMM < right.XMM
	}
	if math.Abs(left.RotationDeg-right.RotationDeg) > candidatePlacementCompareEpsilon {
		return left.RotationDeg < right.RotationDeg
	}
	return left.Layer < right.Layer
}

func compareOrdered[T ~string | ~int | ~float64](left, right T) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func roundCandidateScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*float64(candidateScoreFloatPrecision)) / float64(candidateScoreFloatPrecision)
}
