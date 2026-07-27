package capabilityevaluation

import (
	"fmt"
	"slices"
)

var requiredPromotionGates = []string{
	"connectivity",
	"erc",
	"routing",
	"simulation",
	"strict_drc",
	"writer_correctness",
	"zero_diff_replay",
}

type PromotionEvidence struct {
	CaseID string   `json:"case_id"`
	Gates  []string `json:"gates"`
}

type OutcomeTransition struct {
	CaseID string  `json:"case_id"`
	Before Outcome `json:"before"`
	After  Outcome `json:"after"`
}

type Improvement struct {
	BaselineReady        int                 `json:"baseline_ready"`
	FinalReady           int                 `json:"final_ready"`
	ReadyIncrease        int                 `json:"ready_increase"`
	Transitions          []OutcomeTransition `json:"transitions"`
	PromotedCases        []string            `json:"promoted_cases"`
	ImprovedCapabilities []string            `json:"improved_capabilities"`
}

// VerifyImprovement enforces the frozen-corpus acceptance contract. A case can
// become ready only when every local physical promotion gate is recorded.
func VerifyImprovement(
	baseline, final Report,
	promotions []PromotionEvidence,
	requiredCapabilities []string,
) (Improvement, error) {
	if err := compareReportIdentity(baseline, final); err != nil {
		return Improvement{}, err
	}
	baselineCases := caseMap(baseline.Cases)
	finalCases := caseMap(final.Cases)
	promotionGates, err := normalizePromotions(promotions)
	if err != nil {
		return Improvement{}, err
	}
	improvement := Improvement{}
	improvedCapabilities := map[string]bool{}
	for _, before := range baseline.Cases {
		after, exists := finalCases[before.ID]
		if !exists {
			return Improvement{}, fmt.Errorf("final report is missing case %q", before.ID)
		}
		if before.Domain != after.Domain || before.SafetyImpact != after.SafetyImpact {
			return Improvement{}, fmt.Errorf("case %q metadata changed between reports", before.ID)
		}
		if before.Outcome == OutcomeReady {
			improvement.BaselineReady++
			if after.Outcome != OutcomeReady {
				return Improvement{}, fmt.Errorf("previously ready case %q regressed to %q", before.ID, after.Outcome)
			}
		}
		if after.Outcome == OutcomeReady {
			improvement.FinalReady++
		}
		if before.Outcome != after.Outcome {
			improvement.Transitions = append(improvement.Transitions, OutcomeTransition{
				CaseID: before.ID, Before: before.Outcome, After: after.Outcome,
			})
		}
		if before.Outcome != OutcomeReady && after.Outcome == OutcomeReady {
			if err := verifyPromotionGates(before.ID, promotionGates[before.ID]); err != nil {
				return Improvement{}, err
			}
			improvement.PromotedCases = append(improvement.PromotedCases, before.ID)
			for _, observation := range before.Observations {
				improvedCapabilities[observation.Capability] = true
			}
		} else if safetyWeight(before.SafetyImpact) >= safetyWeight(SafetyRelevant) {
			if err := verifySafetyEvidencePreserved(before, after); err != nil {
				return Improvement{}, err
			}
		}
	}
	if len(finalCases) != len(baselineCases) {
		return Improvement{}, fmt.Errorf("final report case membership changed")
	}
	improvement.ReadyIncrease = improvement.FinalReady - improvement.BaselineReady
	if improvement.ReadyIncrease <= 0 {
		return Improvement{}, fmt.Errorf("ready cases did not strictly increase: %d -> %d", improvement.BaselineReady, improvement.FinalReady)
	}
	improvement.ImprovedCapabilities = sortedStringKeys(improvedCapabilities)
	for _, capability := range normalizeNonEmptyStrings(requiredCapabilities) {
		if !improvedCapabilities[capability] {
			return Improvement{}, fmt.Errorf("required capability %q improved no held-out case", capability)
		}
	}
	return improvement, nil
}

func compareReportIdentity(baseline, final Report) error {
	if baseline.Schema != ReportSchema || final.Schema != ReportSchema {
		return fmt.Errorf("report schema mismatch")
	}
	if baseline.PolicyVersion != final.PolicyVersion ||
		baseline.CorpusRole != final.CorpusRole ||
		baseline.CorpusSHA256 != final.CorpusSHA256 ||
		baseline.RegistryVersion != final.RegistryVersion ||
		baseline.RegistrySHA256 != final.RegistrySHA256 ||
		baseline.CaseCount != final.CaseCount {
		return fmt.Errorf("baseline and final report identities differ")
	}
	return nil
}

func caseMap(cases []CaseResult) map[string]CaseResult {
	result := make(map[string]CaseResult, len(cases))
	for _, current := range cases {
		result[current.ID] = current
	}
	return result
}

func normalizePromotions(promotions []PromotionEvidence) (map[string][]string, error) {
	result := make(map[string][]string, len(promotions))
	for _, promotion := range promotions {
		if !semanticIDPattern.MatchString(promotion.CaseID) {
			return nil, fmt.Errorf("promotion case id %q is invalid", promotion.CaseID)
		}
		if _, exists := result[promotion.CaseID]; exists {
			return nil, fmt.Errorf("promotion case %q is duplicated", promotion.CaseID)
		}
		result[promotion.CaseID] = normalizeNonEmptyStrings(promotion.Gates)
	}
	return result, nil
}

func verifyPromotionGates(caseID string, gates []string) error {
	for _, required := range requiredPromotionGates {
		if !slices.Contains(gates, required) {
			return fmt.Errorf("promoted case %q is missing gate %q", caseID, required)
		}
	}
	return nil
}

func verifySafetyEvidencePreserved(before, after CaseResult) error {
	afterObservations := map[string]Observation{}
	for _, observation := range after.Observations {
		afterObservations[clusterKey(observation)] = observation
	}
	for _, observation := range before.Observations {
		current, exists := afterObservations[clusterKey(observation)]
		if !exists {
			return fmt.Errorf("safety-relevant case %q lost blocking evidence %q", before.ID, clusterKey(observation))
		}
		for _, evidence := range observation.RequiredEvidence {
			if !slices.Contains(current.RequiredEvidence, evidence) {
				return fmt.Errorf("safety-relevant case %q lost required evidence %q", before.ID, evidence)
			}
		}
	}
	return nil
}
