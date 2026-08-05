package closedloopsynthesis

import (
	"math"
	"slices"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

type candidateEvaluationBudget struct {
	Evaluations        int
	AnalysisExecutions int
}

func effectivePolicy(policy Policy) Policy {
	defaults := DefaultPolicy()
	if policy.MaxEvaluationsPerCandidate == 0 {
		policy.MaxEvaluationsPerCandidate = defaults.MaxEvaluationsPerCandidate
	}
	if policy.MaxAnalysisExecutions == 0 {
		policy.MaxAnalysisExecutions = defaults.MaxAnalysisExecutions
	}
	if policy.MaxAnalysisExecutionsPerCandidate == 0 {
		policy.MaxAnalysisExecutionsPerCandidate = defaults.MaxAnalysisExecutionsPerCandidate
	}
	if policy.MaxAnalysisExecutionsPerKind == 0 {
		policy.MaxAnalysisExecutionsPerKind = defaults.MaxAnalysisExecutionsPerKind
	}
	if policy.MaxPlansPerEvaluation == 0 {
		policy.MaxPlansPerEvaluation = defaults.MaxPlansPerEvaluation
	}
	if policy.MaxEvaluations > 0 && policy.MaxEvaluationsPerCandidate > policy.MaxEvaluations {
		policy.MaxEvaluationsPerCandidate = policy.MaxEvaluations
	}
	if policy.MaxAnalysisExecutions > 0 && policy.MaxAnalysisExecutionsPerCandidate > policy.MaxAnalysisExecutions {
		policy.MaxAnalysisExecutionsPerCandidate = policy.MaxAnalysisExecutions
	}
	if policy.MaxAnalysisExecutions > 0 && policy.MaxAnalysisExecutionsPerKind > policy.MaxAnalysisExecutions {
		policy.MaxAnalysisExecutionsPerKind = policy.MaxAnalysisExecutions
	}
	return policy
}

func candidateEvaluationFits(policy Policy, total *Consumption, candidate *candidateEvaluationBudget) bool {
	return total.Evaluations < policy.MaxEvaluations &&
		candidate.Evaluations < policy.MaxEvaluationsPerCandidate &&
		total.AnalysisExecutions < policy.MaxAnalysisExecutions &&
		candidate.AnalysisExecutions < policy.MaxAnalysisExecutionsPerCandidate
}

func evaluationLimits(policy Policy, total *Consumption, candidate *candidateEvaluationBudget) EvaluationLimits {
	remaining := min(
		policy.MaxAnalysisExecutions-total.AnalysisExecutions,
		policy.MaxAnalysisExecutionsPerCandidate-candidate.AnalysisExecutions,
	)
	limits := EvaluationLimits{
		MaxPlans: policy.MaxPlansPerEvaluation, MaxAnalysisExecutions: max(0, remaining),
		MaxConcurrentPlans: 1,
		AnalysisRemaining:  map[string]int{},
	}
	executed := map[string]int{}
	for _, analysis := range total.ByAnalysis {
		executed[analysis.Analysis] = analysis.Executions
	}
	for _, analysis := range []string{
		simmodel.AnalysisDCOperatingPoint, simmodel.AnalysisACSweep, simmodel.AnalysisNoise,
		simmodel.AnalysisStability, simmodel.AnalysisTransient, simmodel.AnalysisStartup,
		simmodel.AnalysisDistortion, simmodel.AnalysisThermal, simmodel.AnalysisElectrothermal,
	} {
		limits.AnalysisRemaining[analysis] = max(0, policy.MaxAnalysisExecutionsPerKind-executed[analysis])
	}
	return limits
}

func cloneAnalysisExecutions(source []AnalysisExecution) []AnalysisExecution {
	clone := append([]AnalysisExecution(nil), source...)
	for index := range clone {
		clone[index].Analyses = append([]string(nil), source[index].Analyses...)
	}
	return clone
}

func consumeAnalysisWork(total *Consumption, candidate *candidateEvaluationBudget, work []AnalysisExecution) {
	byAnalysis := make(map[string]AnalysisConsumption, len(total.ByAnalysis))
	for _, entry := range total.ByAnalysis {
		byAnalysis[entry.Analysis] = entry
	}
	for _, execution := range work {
		if execution.CacheHit {
			total.CacheHits++
		} else {
			total.CacheMisses++
			total.AnalysisExecutions += len(execution.Analyses)
			candidate.AnalysisExecutions += len(execution.Analyses)
		}
		for _, analysis := range execution.Analyses {
			entry := byAnalysis[analysis]
			entry.Analysis = analysis
			if execution.CacheHit {
				entry.CacheHits++
			} else {
				entry.Executions++
			}
			byAnalysis[analysis] = entry
		}
	}
	keys := make([]string, 0, len(byAnalysis))
	for analysis := range byAnalysis {
		keys = append(keys, analysis)
	}
	slices.Sort(keys)
	total.ByAnalysis = total.ByAnalysis[:0]
	for _, analysis := range keys {
		total.ByAnalysis = append(total.ByAnalysis, byAnalysis[analysis])
	}
}

func eliminateDominatedCandidates(report *Report, passing []int) []int {
	retained := make([]int, 0, len(passing))
	for _, dominatedIndex := range passing {
		var evidence *DominanceEvidence
		for _, dominatingIndex := range passing {
			if dominatedIndex == dominatingIndex {
				continue
			}
			candidateEvidence, dominates := candidateDominance(report.Candidates[dominatingIndex], report.Candidates[dominatedIndex])
			if !dominates {
				continue
			}
			if evidence == nil || candidateEvidence.DominatingFingerprint < evidence.DominatingFingerprint {
				copy := candidateEvidence
				evidence = &copy
			}
		}
		if evidence == nil {
			retained = append(retained, dominatedIndex)
			continue
		}
		report.Candidates[dominatedIndex].Dominance = evidence
		report.Candidates[dominatedIndex].Status = "dominated"
		report.Candidates[dominatedIndex].StopReason = StopDominated
		report.Consumption.DominatedCandidates++
		if report.Backtracking != nil {
			for index := range report.Backtracking.Candidates {
				if report.Backtracking.Candidates[index].Fingerprint == report.Candidates[dominatedIndex].Fingerprint {
					report.Backtracking.Candidates[index].Status = "dominated"
					report.Backtracking.Candidates[index].StopReason = StopDominated
				}
			}
		}
	}
	return retained
}

func candidateDominance(dominating, dominated CandidateReport) (DominanceEvidence, bool) {
	dominatingAttempt, dominatingMatches := attemptForState(dominating.Attempts, dominating.FinalState)
	dominatedAttempt, dominatedMatches := attemptForState(dominated.Attempts, dominated.FinalState)
	if dominating.Status != "pass" || dominated.Status != "pass" ||
		dominatingMatches != 1 || dominatedMatches != 1 ||
		dominatingAttempt.Status != "pass" || dominatedAttempt.Status != "pass" ||
		dominatingAttempt.Partial || dominatedAttempt.Partial ||
		dominatingAttempt.BudgetExhausted || dominatedAttempt.BudgetExhausted {
		return DominanceEvidence{}, false
	}
	evidence := DominanceEvidence{
		Schema: "kicadai.synthesis-candidate-dominance.v1", DominatingFingerprint: dominating.Fingerprint,
		Reason: "fully evaluated candidate is no worse in every persisted ranking dimension and strictly better in at least one",
	}
	strict := false
	addLower := func(name string, left, right float64) bool {
		evidence.Dimensions = append(evidence.Dimensions, DominanceDimension{Name: name, Dominating: left, Dominated: right, Preference: "lower"})
		strict = strict || left < right
		return left <= right
	}
	addHigher := func(name string, left, right float64) bool {
		evidence.Dimensions = append(evidence.Dimensions, DominanceDimension{Name: name, Dominating: left, Dominated: right, Preference: "higher"})
		strict = strict || left > right
		return left >= right
	}
	if !addLower("critical_failures", float64(dominating.FinalScore.CriticalFailures), float64(dominated.FinalScore.CriticalFailures)) ||
		!addLower("failures", float64(dominating.FinalScore.Failures), float64(dominated.FinalScore.Failures)) ||
		!addHigher("worst_normalized_margin", dominating.FinalScore.WorstMargin, dominated.FinalScore.WorstMargin) ||
		!addHigher("reviewed_model_uses", float64(dominating.FinalScore.ModelUses), float64(dominated.FinalScore.ModelUses)) ||
		!addLower("repairs", float64(len(dominating.Repairs)), float64(len(dominated.Repairs))) ||
		!staticScoreDominates(dominating.StaticScore, dominated.StaticScore, &evidence, &strict) {
		return DominanceEvidence{}, false
	}
	return evidence, strict
}

func staticScoreDominates(left, right architecturesearch.CandidateScore, evidence *DominanceEvidence, strict *bool) bool {
	lower := func(name string, a, b float64) bool {
		evidence.Dimensions = append(evidence.Dimensions, DominanceDimension{Name: name, Dominating: a, Dominated: b, Preference: "lower"})
		*strict = *strict || a < b
		return a <= b
	}
	higher := func(name string, a, b float64) bool {
		evidence.Dimensions = append(evidence.Dimensions, DominanceDimension{Name: name, Dominating: a, Dominated: b, Preference: "higher"})
		*strict = *strict || a > b
		return a >= b
	}
	if !lower("unproven_non_safety", float64(left.UnprovenNonSafety), float64(right.UnprovenNonSafety)) ||
		!lower("catalog_substitutions", float64(left.CatalogSubstitutions), float64(right.CatalogSubstitutions)) ||
		!higher("evidence_rank", float64(left.EvidenceRank), float64(right.EvidenceRank)) ||
		!lower("component_count", float64(left.ComponentCount), float64(right.ComponentCount)) ||
		!lower("fragment_count", float64(left.FragmentCount), float64(right.FragmentCount)) ||
		!lower("quiescent_power_fragments", float64(left.QuiescentPowerParts), float64(right.QuiescentPowerParts)) ||
		!lower("area_fragments", float64(left.AreaParts), float64(right.AreaParts)) {
		return false
	}
	for _, optional := range []struct {
		name        string
		left, right *float64
		higher      bool
	}{
		{"architecture_worst_margin", left.WorstMargin, right.WorstMargin, true},
		{"quiescent_power_w", left.QuiescentPowerW, right.QuiescentPowerW, false},
		{"area_mm2", left.AreaMM2, right.AreaMM2, false},
	} {
		if (optional.left == nil) != (optional.right == nil) {
			return false
		}
		if optional.left == nil {
			continue
		}
		if !isFinite(*optional.left) || !isFinite(*optional.right) {
			return false
		}
		if optional.higher {
			if !higher(optional.name, *optional.left, *optional.right) {
				return false
			}
		} else if !lower(optional.name, *optional.left, *optional.right) {
			return false
		}
	}
	return true
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
