package capabilityevaluation

import (
	"slices"
	"testing"
)

func TestVerifyImprovementRequiresPhysicalGatesAndCapabilityBenefit(t *testing.T) {
	baseline, final := improvementReports(t)
	promotions := []PromotionEvidence{{CaseID: "case_002", Gates: slices.Clone(requiredPromotionGates)}}
	improvement, err := VerifyImprovement(baseline, final, promotions, []string{"clock_fanout_loading"})
	if err != nil {
		t.Fatal(err)
	}
	if improvement.ReadyIncrease != 1 ||
		!slices.Equal(improvement.PromotedCases, []string{"case_002"}) ||
		!slices.Equal(improvement.ImprovedCapabilities, []string{"clock_fanout_loading"}) {
		t.Fatalf("improvement = %#v", improvement)
	}

	promotions[0].Gates = []string{"simulation"}
	if _, err := VerifyImprovement(baseline, final, promotions, nil); err == nil {
		t.Fatal("expected missing physical gate error")
	}
	if _, err := VerifyImprovement(baseline, final, []PromotionEvidence{{CaseID: "case_002", Gates: slices.Clone(requiredPromotionGates)}}, []string{"bus_buffering_level_translation"}); err == nil {
		t.Fatal("expected missing capability improvement")
	}
}

func TestVerifyImprovementRejectsReadyRegressionAndSafetyEvidenceLoss(t *testing.T) {
	baseline, final := improvementReports(t)
	gates := []PromotionEvidence{{CaseID: "case_002", Gates: slices.Clone(requiredPromotionGates)}}
	final.Cases[0].Outcome = OutcomeUnsupported
	final.Cases[0].Observations = slices.Clone(baseline.Cases[1].Observations)
	if _, err := VerifyImprovement(baseline, final, gates, nil); err == nil {
		t.Fatal("expected ready regression")
	}

	baseline, final = improvementReports(t)
	final.Cases[1].Outcome = OutcomeUnsupported
	final.Cases[1].Observations = []Observation{{
		Capability: "other_capability", Outcome: OutcomeUnsupported,
		Stage: "architecture_search", Code: CodeCapabilityUnsupported,
		Path: "requirements.clock", Reason: "different blocker",
		RequiredEvidence: []string{"different evidence"},
	}}
	if _, err := VerifyImprovement(baseline, final, nil, nil); err == nil {
		t.Fatal("expected safety evidence loss")
	}
}

func improvementReports(t *testing.T) (Report, Report) {
	t.Helper()
	cases := []CaseResult{
		{ID: "case_001", Domain: DomainPower, SafetyImpact: SafetyRelevant, Outcome: OutcomeReady, Observations: []Observation{}},
		{
			ID: "case_002", Domain: DomainDigital, SafetyImpact: SafetyRelevant, Outcome: OutcomeUnsupported,
			Observations: []Observation{{
				Capability: "clock_fanout_loading", Outcome: OutcomeUnsupported,
				Stage: "architecture_search", Code: CodeCapabilityUnsupported,
				Path: "requirements.clock", Reason: "fanout unavailable",
				RequiredEvidence: []string{"reviewed fanout evidence"},
			}},
		},
	}
	registry := ImpactRegistry{Version: "registry_v1"}
	baseline, err := Evaluate(cases, registry, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	baseline.CorpusRole, baseline.CorpusSHA256 = CorpusHeldOut, sourceHash("corpus")
	finalCases := slices.Clone(cases)
	finalCases[1] = CaseResult{
		ID: "case_002", Domain: DomainDigital, SafetyImpact: SafetyRelevant,
		Outcome: OutcomeReady, Observations: []Observation{},
	}
	final, err := Evaluate(finalCases, registry, DefaultRankingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	final.CorpusRole, final.CorpusSHA256 = baseline.CorpusRole, baseline.CorpusSHA256
	return baseline, final
}
