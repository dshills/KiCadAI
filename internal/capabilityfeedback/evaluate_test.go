package capabilityfeedback

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityexpansion"
)

func TestEvaluateRanksByCaseThenElectricalDiversityAndReplays(t *testing.T) {
	shared := Gap{
		Stage: "simulation", Scope: ScopeModel, Capability: "trusted_simulation_model", Code: "MODEL_UNAVAILABLE",
		RequiredEvidence: []string{"reviewed model"}, EvidenceHashes: []string{feedbackHash("shared")},
	}
	narrow := Gap{
		Stage: "routing", Scope: ScopeRouting, Capability: "route_completion", Code: "ROUTE_COMPLETION_PARTIAL",
		RequiredEvidence: []string{"complete routing"}, EvidenceHashes: []string{feedbackHash("narrow")},
	}
	cases := []CaseEvidence{
		feedbackSealedCase(t, "case-a", RoleDiscovery, capabilityevaluation.DomainAnalog, capabilityevaluation.SafetyReviewRequired, []string{"dc_sweep"}, shared),
		feedbackSealedCase(t, "case-b", RoleDiscovery, capabilityevaluation.DomainPower, capabilityevaluation.SafetyCritical, []string{"transient"}, shared),
		feedbackSealedCase(t, "case-c", RoleDiscovery, capabilityevaluation.DomainAnalog, capabilityevaluation.SafetyNonSafety, []string{"dc_sweep"}, narrow),
		feedbackSealedCase(t, "case-d", RoleDiscovery, capabilityevaluation.DomainAnalog, capabilityevaluation.SafetyNonSafety, []string{"dc_sweep"}, narrow),
	}
	registry := capabilityevaluation.ImpactRegistry{Version: "test-v1", Records: []capabilityevaluation.ImpactRecord{{
		Capability: "trusted_simulation_model", Consumers: []string{"closed_loop_control", "fault_analysis"},
	}}}
	first, err := Evaluate(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	reordered := slices.Clone(cases)
	slices.Reverse(reordered)
	second, err := Evaluate(RoleDiscovery, reordered, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Clusters) != 2 ||
		first.Clusters[0].Capability != "trusted_simulation_model" || first.Clusters[0].CaseCount != 2 ||
		first.Clusters[0].DomainCount != 2 || first.Clusters[0].AnalysisCount != 2 || first.Clusters[0].SafetyScore != 6 ||
		first.Clusters[0].ReuseScore != 2 {
		t.Fatalf("ranked report = %#v", first)
	}
	plan, err := BuildRankOneExpansionPlan(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Needs) != 1 || plan.Needs[0].Kind != capabilityexpansion.NeedModel ||
		plan.Needs[0].CapabilityID != "trusted_simulation_model" ||
		!reflect.DeepEqual(plan.Domains, []string{"analog", "power"}) {
		t.Fatalf("rank-1 expansion plan = %#v", plan)
	}
}

func TestHeldOutEvaluationIsSealedAndCannotInfluenceDiscoveryRank(t *testing.T) {
	heldOut := []CaseEvidence{feedbackSealedCase(
		t, "held-a", RoleHeldOut, capabilityevaluation.DomainSensor, capabilityevaluation.SafetyCritical,
		[]string{"electrothermal"}, Gap{
			Stage: "simulation", Scope: ScopeModel, Capability: "held_out_only_model", Code: "MODEL_UNAVAILABLE",
			RequiredEvidence: []string{"sealed"}, EvidenceHashes: []string{feedbackHash("held")},
		},
	)}
	registry := capabilityevaluation.ImpactRegistry{Version: "test-v1"}
	report, err := Evaluate(RoleHeldOut, heldOut, registry)
	if err != nil {
		t.Fatal(err)
	}
	if report.CorpusRole != RoleHeldOut || len(report.Cases) != 1 || len(report.Clusters) != 0 || report.Hash == "" {
		t.Fatalf("held-out report leaked rankable clusters: %#v", report)
	}
	if _, err := BuildRankOneExpansionPlan(report); err == nil {
		t.Fatal("held-out evidence produced an expansion plan")
	}
}

func TestEvaluateRejectsImpactCyclesAndEvidenceDrift(t *testing.T) {
	current := feedbackSealedCase(
		t, "case-a", RoleDiscovery, capabilityevaluation.DomainDigital, capabilityevaluation.SafetyNonSafety,
		[]string{"transient"}, Gap{
			Stage: "routing", Scope: ScopeRouting, Capability: "route_completion", Code: "ROUTE_COMPLETION_PARTIAL",
			RequiredEvidence: []string{"complete routing"}, EvidenceHashes: []string{feedbackHash("route")},
		},
	)
	cyclic := capabilityevaluation.ImpactRegistry{Version: "test-v1", Records: []capabilityevaluation.ImpactRecord{
		{Capability: "route_completion", Consumers: []string{"physical_realization"}},
		{Capability: "physical_realization", Consumers: []string{"route_completion"}},
	}}
	if _, err := Evaluate(RoleDiscovery, []CaseEvidence{current}, cyclic); err == nil {
		t.Fatal("cyclic impact registry was accepted")
	}
	current.StopReason = "mutated"
	if _, err := Evaluate(RoleDiscovery, []CaseEvidence{current}, capabilityevaluation.ImpactRegistry{Version: "test-v1"}); err == nil {
		t.Fatal("mutated case evidence was accepted")
	}
}

func TestEvaluateRealizabilityAwareSeparatesPreviouslyConflatedTopologyGaps(t *testing.T) {
	energy := feedbackSealedCaseForPolicy(
		t, RealizabilityPolicyVersion, "case-energy", RoleDiscovery,
		capabilityevaluation.DomainAnalog, capabilityevaluation.SafetyRelevant,
		[]string{"dc_operating_point"},
		Gap{
			Stage: "requirement_realizability", Scope: ScopeTopology,
			Capability: "energy_domain_creation", Code: "OPEN_TOPOLOGY_ENERGY_DOMAIN_CREATION_REQUIRED",
			RequiredEvidence: requiredEvidence(ScopeTopology, "energy_domain_creation"),
			EvidenceHashes:   []string{feedbackHash("energy")},
		},
	)
	composition := feedbackSealedCaseForPolicy(
		t, RealizabilityPolicyVersion, "case-composition", RoleDiscovery,
		capabilityevaluation.DomainMixedSignal, capabilityevaluation.SafetyReviewRequired,
		[]string{"dc_sweep", "transient"},
		Gap{
			Stage: "requirement_realizability", Scope: ScopeTopology,
			Capability: "multi_obligation_composition", Code: "OPEN_TOPOLOGY_MULTI_CONTROL_COMPOSITION_REQUIRED",
			RequiredEvidence: requiredEvidence(ScopeTopology, "multi_obligation_composition"),
			EvidenceHashes:   []string{feedbackHash("composition")},
		},
	)
	registry := capabilityevaluation.ImpactRegistry{Version: "realizability-test-v1", Records: []capabilityevaluation.ImpactRecord{
		{Capability: "energy_domain_creation", Consumers: []string{"passing_behavioral_evidence"}},
		{Capability: "multi_obligation_composition", Consumers: []string{"passing_behavioral_evidence"}},
	}}
	report, err := EvaluateRealizabilityAware(RoleDiscovery, []CaseEvidence{composition, energy}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if report.PolicyVersion != RealizabilityPolicyVersion || len(report.Clusters) != 2 ||
		report.Clusters[0].Capability == report.Clusters[1].Capability {
		t.Fatalf("realizability clusters = %#v", report.Clusters)
	}
	if _, err := Evaluate(RoleDiscovery, []CaseEvidence{energy}, registry); err == nil {
		t.Fatal("legacy evaluator accepted realizability-policy evidence")
	}
	if err := ValidateAggregateReport(report, registry); err != nil {
		t.Fatalf("realizability aggregate did not reproduce: %v", err)
	}
}

func feedbackSealedCase(
	t *testing.T,
	id string,
	role CorpusRole,
	domain capabilityevaluation.Domain,
	safety capabilityevaluation.SafetyImpact,
	analyses []string,
	gap Gap,
) CaseEvidence {
	return feedbackSealedCaseForPolicy(t, PolicyVersion, id, role, domain, safety, analyses, gap)
}

func feedbackSealedCaseForPolicy(
	t *testing.T,
	policyVersion string,
	id string,
	role CorpusRole,
	domain capabilityevaluation.Domain,
	safety capabilityevaluation.SafetyImpact,
	analyses []string,
	gap Gap,
) CaseEvidence {
	t.Helper()
	current := CaseEvidence{
		Schema: CaseEvidenceSchema, PolicyVersion: policyVersion,
		Case:    CaseMeta{ID: id, Role: role, Domain: domain, SafetyImpact: safety},
		Outcome: OutcomeUnsupported, StopReason: "model_unavailable",
		RequirementHash: feedbackHash(id + "-requirement"), InventoryHash: feedbackHash("inventory"),
		CatalogHash: feedbackHash("catalog"), ModelRegistryHash: feedbackHash("models"),
		SynthesisPolicyHash: feedbackHash("policy"), SynthesisHash: feedbackHash(id + "-synthesis"),
		AnalysisKinds: normalizedStrings(analyses), Gaps: normalizeGaps([]Gap{gap}),
	}
	hash, err := caseEvidenceHash(current)
	if err != nil {
		t.Fatal(err)
	}
	current.Hash = hash
	return current
}
