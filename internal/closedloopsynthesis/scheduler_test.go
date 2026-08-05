package closedloopsynthesis

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

type scheduledEvaluatorFunc func(context.Context, CandidateState, EvaluationLimits) (Evaluation, error)

func (function scheduledEvaluatorFunc) Evaluate(ctx context.Context, state CandidateState) (Evaluation, error) {
	return function(ctx, state, EvaluationLimits{})
}

func (function scheduledEvaluatorFunc) EvaluateScheduled(ctx context.Context, state CandidateState, limits EvaluationLimits) (Evaluation, error) {
	return function(ctx, state, limits)
}

func TestScheduledRunnerRetainsPartialFailureAndRequiresExhaustiveFinalist(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("scheduled"), Variables: []Variable{{
		ID: "gain", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2},
		Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}},
	}}}
	evaluator := scheduledEvaluatorFunc(func(_ context.Context, state CandidateState, limits EvaluationLimits) (Evaluation, error) {
		if limits.MaxPlans != DefaultPolicy().MaxPlansPerEvaluation || limits.MaxAnalysisExecutions <= 0 {
			t.Fatalf("scheduler limits = %#v", limits)
		}
		base, _ := closedLoopTestEvaluator(false).Evaluate(context.Background(), state)
		value := state.Variables[0].Value
		if value == 1 {
			base.Measurements = base.Measurements[:1]
			base.Partial = true
			base.Simulation = nil
			base.Schedule = []EvaluationStageEvidence{
				{Stage: EvaluationStageStructural, Status: "pass"},
				{Stage: EvaluationStageAC, Status: "fail"},
				{Stage: EvaluationStageThermalSOA, Status: "skipped", Reason: "earlier electrical stage failed"},
				{Stage: EvaluationStageExhaustivePromotion, Status: "skipped", Reason: "candidate evaluation is partial"},
			}
			base.Work = []AnalysisExecution{{PlanHash: testHash("ac"), Stage: EvaluationStageAC, Analyses: []string{simmodel.AnalysisACSweep}}}
			return base, nil
		}
		base.Schedule = []EvaluationStageEvidence{
			{Stage: EvaluationStageStructural, Status: "pass"},
			{Stage: EvaluationStageAC, Status: "pass"},
			{Stage: EvaluationStageThermalSOA, Status: "pass"},
			{Stage: EvaluationStageExhaustivePromotion, Status: "pass"},
		}
		base.Work = []AnalysisExecution{
			{PlanHash: testHash("ac-final"), Stage: EvaluationStageAC, Analyses: []string{simmodel.AnalysisACSweep}},
			{PlanHash: testHash("thermal-final"), Stage: EvaluationStageThermalSOA, Analyses: []string{simmodel.AnalysisThermal}},
		}
		return base, nil
	})
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{candidate},
	}
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || len(report.Candidates[0].Attempts) != 2 {
		t.Fatalf("scheduled report = %#v", report)
	}
	first, final := report.Candidates[0].Attempts[0], report.Candidates[0].Attempts[1]
	if !first.Partial || first.Status != "fail" || first.Simulation != nil || final.Partial || final.Status != "pass" || final.Simulation == nil {
		t.Fatalf("partial/final attempts = %#v / %#v", first, final)
	}
	if report.Consumption.AnalysisExecutions != 3 || report.Consumption.CacheMisses != 3 {
		t.Fatalf("analysis accounting = %#v", report.Consumption)
	}
}

func TestCandidateBlockedBeforeEvaluationUsesOneBasedAttemptNumber(t *testing.T) {
	requirement := closedLoopTestRequirement()
	policy := DefaultPolicy()
	policy.MaxAnalysisExecutions = 1
	policy.MaxAnalysisExecutionsPerCandidate = 1
	consumption := Consumption{AnalysisExecutions: 1}
	result := evaluateCandidate(context.Background(), requirement, Candidate{Fingerprint: testHash("budget-blocked")}, closedLoopTestEvaluator(false), policy, &consumption)
	if len(result.Attempts) != 1 || result.Attempts[0].Number != 1 || result.StopReason != StopBudgetExhausted {
		t.Fatalf("pre-evaluation budget result = %#v", result)
	}
}

func TestEvaluationErrorStillConsumesCompletedAnalysisWork(t *testing.T) {
	requirement := closedLoopTestRequirement()
	evaluator := scheduledEvaluatorFunc(func(_ context.Context, _ CandidateState, _ EvaluationLimits) (Evaluation, error) {
		return Evaluation{
			EvidenceHash: testHash("partial-error"),
			Partial:      true,
			Schedule: []EvaluationStageEvidence{
				{Stage: EvaluationStageStructural, Status: "pass"},
				{Stage: EvaluationStageAC, Status: "blocked", Reason: "trusted simulation execution failed"},
				{Stage: EvaluationStageExhaustivePromotion, Status: "skipped", Reason: "candidate evaluation is partial"},
			},
			Work: []AnalysisExecution{{
				PlanHash: testHash("failed-plan"), Stage: EvaluationStageAC,
				Analyses: []string{simmodel.AnalysisACSweep},
			}},
		}, errors.New("solver failed after execution")
	})
	report := Run(context.Background(), Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{Fingerprint: testHash("error-work")}},
	}, evaluator, DefaultPolicy())
	if report.Consumption.AnalysisExecutions != 1 || report.Consumption.CacheMisses != 1 || len(report.Candidates) != 1 || len(report.Candidates[0].Attempts) != 1 || len(report.Candidates[0].Attempts[0].Work) != 1 {
		t.Fatalf("error work accounting = %#v candidates=%#v", report.Consumption, report.Candidates)
	}
}

func TestSchedulerAnalysisBudgetsArePerKindAndCacheHitsAreFree(t *testing.T) {
	limits := EvaluationLimits{
		MaxAnalysisExecutions: 2,
		AnalysisRemaining:     map[string]int{simmodel.AnalysisACSweep: 1, simmodel.AnalysisTransient: 2},
	}
	used := map[string]int{}
	if !scheduledWorkFits(limits, 0, used, []string{simmodel.AnalysisACSweep}) {
		t.Fatal("first AC execution should fit")
	}
	used[simmodel.AnalysisACSweep] = 1
	if scheduledWorkFits(limits, 1, used, []string{simmodel.AnalysisACSweep}) {
		t.Fatal("second AC execution exceeded its per-analysis budget")
	}
	if !scheduledWorkFits(limits, 1, used, []string{simmodel.AnalysisTransient}) {
		t.Fatal("independent transient execution should fit")
	}

	total, candidate := Consumption{}, candidateEvaluationBudget{}
	consumeAnalysisWork(&total, &candidate, []AnalysisExecution{
		{PlanHash: testHash("miss"), Stage: EvaluationStageAC, Analyses: []string{simmodel.AnalysisACSweep}},
		{PlanHash: testHash("hit"), Stage: EvaluationStageAC, Analyses: []string{simmodel.AnalysisACSweep}, CacheHit: true},
	})
	if total.AnalysisExecutions != 1 || candidate.AnalysisExecutions != 1 || total.CacheHits != 1 || total.CacheMisses != 1 || len(total.ByAnalysis) != 1 || total.ByAnalysis[0].CacheHits != 1 {
		t.Fatalf("cache/budget accounting = %#v candidate=%#v", total, candidate)
	}
}

func TestSchedulerPlanOrderIsIndependentOfInputOrder(t *testing.T) {
	plans := []scheduledPlan{
		{index: 0, hash: "c", stage: EvaluationStageThermalSOA},
		{index: 1, hash: "b", stage: EvaluationStageDC},
		{index: 2, hash: "a", stage: EvaluationStageDC},
		{index: 3, hash: "d", stage: EvaluationStageTransient},
	}
	slices.SortStableFunc(plans, compareScheduledPlans)
	got := []string{plans[0].hash, plans[1].hash, plans[2].hash, plans[3].hash}
	want := []string{"a", "b", "d", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("scheduled order = %#v, want %#v", got, want)
	}
}

func TestSchedulerRejectsPlanThatCrossesCostStages(t *testing.T) {
	if _, ok := planEvaluationStage([]string{simmodel.AnalysisDCOperatingPoint, simmodel.AnalysisTransient}); ok {
		t.Fatal("mixed DC/transient plan bypassed deterministic stage ordering")
	}
	if stage, ok := planEvaluationStage([]string{simmodel.AnalysisACSweep, simmodel.AnalysisNoise}); !ok || stage != EvaluationStageAC {
		t.Fatalf("same-stage AC/noise batch = %q, %t", stage, ok)
	}
}

func TestSchedulerCountsEachAnalysisExecutionOfTheSameKind(t *testing.T) {
	plan := simmodel.Plan{Analyses: []simmodel.Analysis{
		{ID: "nominal", Kind: simmodel.AnalysisACSweep},
		{ID: "loaded", Kind: simmodel.AnalysisACSweep},
	}}
	kinds := planAnalysisKinds(plan)
	if !slices.Equal(kinds, []string{simmodel.AnalysisACSweep, simmodel.AnalysisACSweep}) {
		t.Fatalf("analysis execution kinds = %#v", kinds)
	}
	limits := EvaluationLimits{
		MaxAnalysisExecutions: 8,
		AnalysisRemaining:     map[string]int{simmodel.AnalysisACSweep: 1},
	}
	if scheduledWorkFits(limits, 0, nil, kinds) {
		t.Fatal("two same-kind analyses consumed only one per-kind budget unit")
	}
}

func TestScheduledPlanConcurrencyDoesNotChangeResultsOrCacheDecisions(t *testing.T) {
	resolution, err := (dividerSimulationResolver{}).ResolveSimulation(context.Background(), CandidateState{Fingerprint: testHash("parallel")})
	if err != nil {
		t.Fatal(err)
	}
	plans := []simmodel.Plan{simmodel.ClonePlan(resolution.Plan), simmodel.ClonePlan(resolution.Plan)}
	hash, ok := simulationEvaluationCacheKey(plans[0])
	if !ok {
		t.Fatal("plan hash failed")
	}
	scheduled := []scheduledPlan{
		{index: 0, hash: hash, stage: EvaluationStageDC, analyses: []string{simmodel.AnalysisDCOperatingPoint}},
		{index: 1, hash: hash, stage: EvaluationStageDC, analyses: []string{simmodel.AnalysisDCOperatingPoint}},
	}
	var baseline []scheduledPlanResult
	for _, concurrency := range []int{1, 4} {
		evaluator := SimModelEvaluator{Cache: NewSimulationEvaluationCache()}
		results, executions, blocked, err := evaluator.evaluateScheduledStage(context.Background(), plans, scheduled, EvaluationLimits{
			MaxAnalysisExecutions: 8, MaxConcurrentPlans: concurrency,
			AnalysisRemaining: map[string]int{simmodel.AnalysisDCOperatingPoint: 8},
		}, 0, map[string]int{})
		if err != nil || blocked || executions != 1 || len(results) != 2 || results[0].cacheHit || !results[1].cacheHit {
			t.Fatalf("concurrency %d results=%#v executions=%d blocked=%t err=%v", concurrency, results, executions, blocked, err)
		}
		if concurrency == 1 {
			baseline = results
			continue
		}
		if !reflect.DeepEqual(results, baseline) {
			t.Fatalf("concurrency changed canonical stage results\none=%#v\nfour=%#v", baseline, results)
		}
	}
}

func TestClosedLoopEliminatesOnlyParetoDominatedCompleteFinalists(t *testing.T) {
	requirement := closedLoopTestRequirement()
	better := Candidate{Fingerprint: testHash("better"), Score: architecturesearch.CandidateScore{EvidenceRank: 2, ComponentCount: 2}}
	worse := Candidate{Fingerprint: testHash("worse"), Score: architecturesearch.CandidateScore{EvidenceRank: 1, ComponentCount: 3}}
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{worse, better},
	}
	report := Run(context.Background(), input, closedLoopTestEvaluator(false), DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.Fingerprint != better.Fingerprint || report.Consumption.DominatedCandidates != 1 {
		t.Fatalf("dominance report = %#v", report)
	}
	var dominated *CandidateReport
	for index := range report.Candidates {
		if report.Candidates[index].Fingerprint == worse.Fingerprint {
			dominated = &report.Candidates[index]
		}
	}
	if dominated == nil || dominated.Status != "dominated" || dominated.StopReason != StopDominated || dominated.Dominance == nil || dominated.Dominance.DominatingFingerprint != better.Fingerprint || len(dominated.Attempts) == 0 {
		t.Fatalf("auditable dominance evidence = %#v", dominated)
	}
}
