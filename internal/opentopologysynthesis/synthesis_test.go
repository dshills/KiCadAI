package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"
)

func TestSynthesizeRetainsDeterministicBoundedEvidenceAcrossTopologies(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 1_000
	policy.MaxGeneratedGraphs = 5_000
	policy.MaxRetainedCandidates = 4
	policy.MaxValueTrials = 4
	policy.MaxTopologyRepairs = 2
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1_024

	first := Synthesize(context.Background(), requirement, inventory, environment, policy)
	if len(first.Hash) != 64 || first.Report.PolicyHash == "" ||
		first.Report.RequirementHash == "" || len(first.Search.Candidates) < 2 ||
		len(first.Candidates) != len(first.Search.Candidates) {
		t.Fatalf("incomplete synthesis evidence: status=%s search=%d evidence=%d report=%#v",
			first.Report.Status, len(first.Search.Candidates), len(first.Candidates), first.Report)
	}
	evaluatedCandidates := 0
	for index, candidate := range first.Candidates {
		if candidate.Fingerprint != first.Search.Candidates[index].Fingerprint ||
			candidate.TopologyHash != first.Search.Candidates[index].TopologyHash ||
			candidate.ValuePlan.GraphHash != first.Search.Candidates[index].Fingerprint {
			t.Fatalf("candidate evidence is not search-bound at %d: %#v", index, candidate)
		}
		if len(candidate.Evaluations) != 0 {
			evaluatedCandidates++
		}
	}
	if evaluatedCandidates < 2 {
		t.Fatalf("value evaluation did not cover distinct retained topologies: %d", evaluatedCandidates)
	}
	if first.Report.Consumption.ValueTrials > policy.MaxValueTrials ||
		first.Report.Consumption.TopologyRepairs > policy.MaxTopologyRepairs ||
		first.Report.Consumption.CandidateSimulations > policy.MaxCandidateSimulations ||
		first.Report.Consumption.CornerEvaluations > policy.MaxCornerEvaluations {
		t.Fatalf("synthesis exceeded explicit budgets: %#v", first.Report.Consumption)
	}
	assertSynthesisConsumptionMatchesEvidence(t, first)

	second := Synthesize(context.Background(), requirement, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("synthesis replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestPostPassRankingWindowUsesRetainedCandidateBudget(t *testing.T) {
	policy := DefaultPolicy()
	policy.MaxRetainedCandidates = 7
	if got := synthesisPostPassEvaluationBudget(policy); got != 7 {
		t.Fatalf("post-pass ranking budget = %d, want 7", got)
	}
	policy.MaxRetainedCandidates = 0
	if got := synthesisPostPassEvaluationBudget(policy); got != 1 {
		t.Fatalf("minimum post-pass ranking budget = %d, want 1", got)
	}
}

func TestInitialEvaluationPolicyReservesBudgetsForAuthorizedRepairs(t *testing.T) {
	policy := DefaultPolicy()
	policy.MaxRetainedCandidates = 16
	policy.MaxValueTrials = 64
	policy.MaxTopologyRepairs = 16
	policy.MaxCandidateSimulations = 4_096
	policy.MaxCornerEvaluations = 16_384

	initial := synthesisInitialEvaluationPolicy(policy, 16)
	if initial.MaxValueTrials != 32 || initial.MaxCandidateSimulations != 2_048 ||
		initial.MaxCornerEvaluations != 8_192 ||
		initial.MaxTopologyRepairs != policy.MaxTopologyRepairs {
		t.Fatalf("initial evaluation policy did not reserve a proportional repair share: %#v", initial)
	}

	policy.MaxTopologyRepairs = 0
	if got := synthesisInitialEvaluationPolicy(policy, 16); got != policy {
		t.Fatalf("repair-disabled policy changed: got %#v want %#v", got, policy)
	}
}

func TestSynthesisMarginNormalizesSubUnitQuantitiesRelatively(t *testing.T) {
	minimum := 0.9e-6
	actual := 1e-6
	margin := synthesisWorstNormalizedMargin(SimulationEvaluation{
		Attempts: []SimulationAttempt{{Actual: &actual, RequiredMin: &minimum}},
	})
	if math.Abs(margin-0.1) > 1e-12 {
		t.Fatalf("sub-unit normalized margin = %.12g, want 0.1", margin)
	}
}

func TestRankedSynthesisSelectionPrefersRequirementMarginAndExplainsAlternatives(t *testing.T) {
	_, base, _, _ := testSimulationFixture(t)
	moreComplex := CloneGraph(base)
	extra := moreComplex.Instances[0]
	extra.ID = "ranking_extra"
	moreComplex.Instances = append(moreComplex.Instances, extra)
	baseActive, err := ActiveStructureHash(base)
	if err != nil {
		t.Fatal(err)
	}
	complexActive, err := ActiveStructureHash(moreComplex)
	if err != nil {
		t.Fatal(err)
	}

	minimum := 1.0
	narrowActual := 1.1
	wideActual := 2.0
	narrow := SimulationEvaluation{
		Status: SimulationEvaluationPassed,
		Hash:   "eval-narrow",
		Attempts: []SimulationAttempt{{
			Actual: &narrowActual, RequiredMin: &minimum, AssertionPass: true,
		}},
	}
	wide := SimulationEvaluation{
		Status: SimulationEvaluationPassed,
		Hash:   "eval-wide",
		Attempts: []SimulationAttempt{{
			Actual: &wideActual, RequiredMin: &minimum, AssertionPass: true,
		}},
	}
	rejected := SimulationEvaluation{
		Status: SimulationEvaluationFailed,
		Hash:   "eval-rejected",
		Diagnoses: []Diagnosis{{
			Code: "assertion_below_minimum", EvidenceHash: "rejection-evidence",
		}},
	}
	run := SynthesisRun{
		Schema: SynthesisRunSchema, Version: SynthesisRunVersion,
		Report: Report{
			Schema: ReportSchema, Version: ReportVersion,
			Candidates: []CandidateReport{
				{Fingerprint: "simple", ActiveStructureHash: baseActive},
				{Fingerprint: "wide-margin", ActiveStructureHash: complexActive},
				{Fingerprint: "rejected", TopologyHash: "topology-rejected", ActiveStructureHash: "active-rejected"},
			},
		},
		Candidates: []SynthesisCandidateEvidence{
			{Fingerprint: "simple", ActiveStructureHash: baseActive, ValuePlan: ValueSearchPlan{Status: ValuePlanReady}},
			{Fingerprint: "wide-margin", ActiveStructureHash: complexActive, ValuePlan: ValueSearchPlan{Status: ValuePlanReady}},
			{
				Fingerprint: "rejected", TopologyHash: "topology-rejected", ActiveStructureHash: "active-rejected",
				ValuePlan: ValueSearchPlan{Status: ValuePlanReady}, Evaluations: []SimulationEvaluation{rejected},
			},
		},
	}
	selected := selectRankedSynthesisResult(run, []synthesisPassingCandidate{
		{
			candidateIndex: 0, graph: base, evaluation: narrow,
			physical: PhysicalLoweringResult{Status: PhysicalLoweringReady, Hash: "physical-simple"},
			margin:   synthesisWorstNormalizedMargin(narrow),
		},
		{
			candidateIndex: 1, graph: moreComplex, evaluation: wide,
			physical: PhysicalLoweringResult{Status: PhysicalLoweringReady, Hash: "physical-wide"},
			margin:   synthesisWorstNormalizedMargin(wide),
		},
	})
	if selected.Report.Selected == nil || selected.Report.Selected.Fingerprint != "wide-margin" {
		t.Fatalf("ranked selection = %#v", selected.Report.Selected)
	}
	ranking := selected.Report.Selected.Ranking
	if ranking.Policy != synthesisSelectionRankingPolicy || len(ranking.Alternatives) != 2 ||
		!ranking.Alternatives[0].Selected || ranking.Alternatives[0].Fingerprint != "wide-margin" ||
		ranking.Alternatives[0].WorstNormalizedMargin <= ranking.Alternatives[1].WorstNormalizedMargin ||
		ranking.Alternatives[0].Disposition != "selected" || ranking.Alternatives[1].Disposition != "not_selected" ||
		ranking.Alternatives[0].Reason == "" || ranking.Alternatives[1].Reason == "" ||
		ranking.Alternatives[0].PhysicalHash == "" || ranking.Alternatives[1].PhysicalHash == "" {
		t.Fatalf("selection ranking = %#v", ranking)
	}
	if len(ranking.Rejections) != 1 || ranking.Rejections[0].Stage != "simulation" ||
		!slices.Contains(ranking.Rejections[0].Codes, "assertion_below_minimum") ||
		!slices.Contains(ranking.Rejections[0].EvidenceHashes, "rejection-evidence") {
		t.Fatalf("selection rejection evidence = %#v", ranking.Rejections)
	}
	if selected.Report.Selected.SelectionSummary == "" || selected.SelectedGraph == nil || selected.Physical == nil {
		t.Fatalf("selection explanation or bound artifacts missing: %#v", selected)
	}
}

func assertSynthesisConsumptionMatchesEvidence(t *testing.T, run SynthesisRun) {
	t.Helper()
	expected := run.Search.Consumption
	for _, candidate := range run.Candidates {
		expected.ValueTrials += len(candidate.Evaluations)
		for _, evaluation := range candidate.Evaluations {
			expected.CandidateSimulations += evaluation.Consumption.CandidateSimulations
			expected.CornerEvaluations += evaluation.Consumption.CornerEvaluations
		}
		if candidate.Repair != nil {
			expected.CandidateSimulations += candidate.Repair.Consumption.CandidateSimulations
			expected.CornerEvaluations += candidate.Repair.Consumption.CornerEvaluations
			expected.ValueTrials += candidate.Repair.Consumption.ValueTrials
			expected.TopologyRepairs += candidate.Repair.Consumption.TopologyRepairs
			expected.MaximumFrontier = max(
				expected.MaximumFrontier,
				candidate.Repair.Consumption.MaximumFrontier,
			)
			expected.BudgetExhausted =
				expected.BudgetExhausted || candidate.Repair.Consumption.BudgetExhausted
		}
	}
	// Exhaustion can be raised by the synthesis scheduler when it observes that
	// the next unit of work cannot start; that scheduler decision is not a
	// retained child-consumption field.
	expected.BudgetExhausted = run.Report.Consumption.BudgetExhausted
	if run.Report.Consumption != expected {
		t.Fatalf(
			"synthesis consumption does not match retained evidence:\nreport=%#v\nexpected=%#v",
			run.Report.Consumption,
			expected,
		)
	}
}

func TestSynthesisValueCapabilityUnavailableRequiresEveryPlanToLackCoverage(t *testing.T) {
	if synthesisValueCapabilityUnavailable(nil) {
		t.Fatal("empty candidate set is not a value capability decision")
	}
	unsupported := []SynthesisCandidateEvidence{
		{ValuePlan: ValueSearchPlan{Status: ValuePlanExhausted}},
		{ValuePlan: ValueSearchPlan{Status: ValuePlanUnsupported}},
	}
	if !synthesisValueCapabilityUnavailable(unsupported) {
		t.Fatal("uniform exhausted/unsupported value plans did not form a capability gap")
	}
	unsupported[1].ValuePlan.Status = ValuePlanReady
	if synthesisValueCapabilityUnavailable(unsupported) {
		t.Fatal("a ready value plan was misclassified as a catalog capability gap")
	}
}

func TestSynthesizeFailsClosedBeforeSearchAndHonorsCancellation(t *testing.T) {
	requirement, _, inventory, environment := testSimulationFixture(t)
	invalid := requirement
	invalid.Schema = "not-a-schema"
	result := Synthesize(context.Background(), invalid, inventory, environment, DefaultPolicy())
	if result.Report.Status != StatusInvalid ||
		result.Report.StopReason != StopRequirementInvalid ||
		len(result.Report.Diagnostics) == 0 || result.Search.Schema != "" ||
		result.SelectedGraph != nil || result.Physical != nil {
		t.Fatalf("invalid synthesis result = %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = Synthesize(ctx, requirement, inventory, environment, DefaultPolicy())
	if result.Report.Status != StatusCanceled ||
		result.Report.StopReason != StopCanceled ||
		len(result.Report.Diagnostics) != 1 ||
		result.Report.Diagnostics[0].Code != CodeCanceled ||
		result.SelectedGraph != nil || result.Physical != nil {
		t.Fatalf("canceled synthesis result = %#v", result)
	}
}

func TestFinalizeSynthesisRunRejectsIncompletePassingSelection(t *testing.T) {
	result := finalizeSynthesisRun(SynthesisRun{
		Schema:  SynthesisRunSchema,
		Version: SynthesisRunVersion,
		Report: Report{
			Schema: ReportSchema, Version: ReportVersion,
			Status: StatusPassed, StopReason: StopPassed,
		},
	})
	if result.Report.Status != StatusFailed ||
		result.Report.StopReason != StopNoPassingGraph ||
		len(result.Report.Diagnostics) != 1 ||
		result.Report.Diagnostics[0].Code != CodeNoPassingGraph ||
		result.SelectedGraph != nil || result.Physical != nil ||
		result.Hash == "" {
		t.Fatalf("incomplete passing selection was not rejected: %#v", result)
	}
}
