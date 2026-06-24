package placement

import (
	"math"
	"slices"
	"strings"
)

const (
	defaultCandidateScoringPolicy                 = "semantic_v1"
	defaultCandidateScoringVersion                = "candidate-scoring.v1"
	defaultCandidateAlternativesPerComponent      = 3
	defaultCandidateScoreEvidencePerDimension     = 4
	maxCandidateAlternativesPerComponent          = 32
	maxCandidateScoreEvidencePerDimension         = 16
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
