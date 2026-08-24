package closedloopsynthesis

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/runtimebudget"
	"kicadai/internal/simmodel"
)

type scheduledPlan struct {
	index    int
	hash     string
	stage    EvaluationStage
	analyses []string
}

type indexedSimulationReport struct {
	Index  int             `json:"index"`
	Report simmodel.Report `json:"report"`
}

type scheduledPlanResult struct {
	plan        scheduledPlan
	report      simmodel.Report
	diagnostics []simmodel.Diagnostic
	cacheHit    bool
}

func (evaluator SimModelEvaluator) evaluateScheduledResolution(
	ctx context.Context,
	resolution SimulationResolution,
	plans []simmodel.Plan,
	modelDecisions []ModelDecision,
	limits EvaluationLimits,
) (Evaluation, error) {
	result := Evaluation{ModelDecisions: cloneModelDecisions(modelDecisions)}
	result.Schedule = append(result.Schedule, EvaluationStageEvidence{
		Stage: EvaluationStageStructural, Status: "pass",
		Reason: "resolved plans, assertion links, and model provenance validated",
	})
	if limits.MaxPlans > 0 && len(plans) > limits.MaxPlans {
		result.Partial, result.BudgetExhausted = true, true
		result.Schedule = append(result.Schedule,
			EvaluationStageEvidence{Stage: EvaluationStageExhaustivePromotion, Status: "skipped", Reason: "plan budget exhausted before trusted execution"},
		)
		result.EvidenceHash = hashJSON(result.Schedule)
		return result, nil
	}

	ordered := make([]scheduledPlan, 0, len(plans))
	for index, plan := range plans {
		hash, ok := simulationEvaluationCacheKey(plan)
		if !ok {
			return Evaluation{}, fmt.Errorf("hash trusted simulation plan %d", index)
		}
		analyses := planAnalysisKinds(plan)
		stage, ok := planEvaluationStage(analyses)
		if !ok {
			return Evaluation{}, fmt.Errorf("trusted simulation plan %d cannot be scheduled: analyses must be registered and partitioned into single-stage batches", index)
		}
		ordered = append(ordered, scheduledPlan{index: index, hash: hash, stage: stage, analyses: analyses})
	}
	slices.SortStableFunc(ordered, compareScheduledPlans)

	reports := make([]simmodel.Report, len(plans))
	evaluated := make([]bool, len(plans))
	executions := 0
	byAnalysis := map[string]int{}
	stopped := false
	for start := 0; start < len(ordered) && !stopped; {
		stage := ordered[start].stage
		end := start + 1
		for end < len(ordered) && ordered[end].stage == stage {
			end++
		}
		stageEvidence := EvaluationStageEvidence{Stage: stage, Status: "pass"}
		stageFailed := false
		for _, scheduled := range ordered[start:end] {
			stageEvidence.PlanHashes = append(stageEvidence.PlanHashes, scheduled.hash)
			stageEvidence.Analyses = append(stageEvidence.Analyses, scheduled.analyses...)
		}
		stageResults, stageExecutions, budgetBlocked, err := evaluator.evaluateScheduledStage(ctx, plans, ordered[start:end], limits, executions, byAnalysis)
		if err != nil {
			return Evaluation{}, err
		}
		executions += stageExecutions
		if budgetBlocked {
			result.Partial, result.BudgetExhausted = true, true
			stageEvidence.Status = "blocked"
			stageEvidence.Reason = "trusted analysis execution budget exhausted"
			stopped = true
		}
		var fatalErr error
		for _, stageResult := range stageResults {
			scheduled := stageResult.plan
			if len(stageResult.diagnostics) != 0 && !onlyAssertionFailures(stageResult.report, stageResult.diagnostics) {
				if fatalErr == nil {
					fatalErr = fmt.Errorf("trusted simulation plan %d (%s) failed: %s", scheduled.index, strings.Join(scheduled.analyses, ","), joinSimModelDiagnostics(stageResult.diagnostics))
				}
			}
			reports[scheduled.index] = simmodel.CloneReport(stageResult.report)
			evaluated[scheduled.index] = true
			result.Work = append(result.Work, AnalysisExecution{
				PlanHash: scheduled.hash, Stage: scheduled.stage,
				Analyses: append([]string(nil), scheduled.analyses...), CacheHit: stageResult.cacheHit,
			})
			if simulationReportFailed(stageResult.report) {
				stageFailed = true
			}
		}
		slices.Sort(stageEvidence.Analyses)
		stageEvidence.Analyses = slices.Compact(stageEvidence.Analyses)
		if fatalErr != nil {
			result.Partial = true
			stageEvidence.Status = "blocked"
			stageEvidence.Reason = "trusted simulation execution failed"
			result.Schedule = append(result.Schedule, stageEvidence)
			if end < len(ordered) {
				for _, remaining := range remainingEvaluationStages(ordered[end:]) {
					result.Schedule = append(result.Schedule, EvaluationStageEvidence{
						Stage: remaining, Status: "skipped", Reason: "earlier electrical stage failed",
					})
				}
			}
			result.Schedule = append(result.Schedule, EvaluationStageEvidence{
				Stage: EvaluationStageExhaustivePromotion, Status: "skipped", Reason: "candidate evaluation is partial",
			})
			result.EvidenceHash = scheduledPartialEvidenceHash(resolution, result.Schedule, result.Work, reports, evaluated)
			return result, fatalErr
		}
		if stageFailed && stageEvidence.Status == "pass" {
			stageEvidence.Status = "fail"
			stageEvidence.Reason = "one or more trusted assertions failed"
		}
		result.Schedule = append(result.Schedule, stageEvidence)
		start = end
		if stageFailed && start < len(ordered) {
			result.Partial = true
			for _, remaining := range remainingEvaluationStages(ordered[start:]) {
				result.Schedule = append(result.Schedule, EvaluationStageEvidence{
					Stage: remaining, Status: "skipped", Reason: "earlier electrical stage failed",
				})
			}
			stopped = true
		}
	}

	complete := true
	for _, done := range evaluated {
		complete = complete && done
	}
	result.Partial = result.Partial || !complete
	if complete {
		result.Schedule = append(result.Schedule, EvaluationStageEvidence{
			Stage: EvaluationStageExhaustivePromotion, Status: "pass",
			Reason: "all required plans, analyses, assertions, and registered worst-case corners completed",
		})
	} else {
		result.Schedule = append(result.Schedule, EvaluationStageEvidence{
			Stage: EvaluationStageExhaustivePromotion, Status: "skipped", Reason: "candidate evaluation is partial",
		})
	}

	measurements, err := scheduledMeasurements(plans, reports, resolution.Measurements, evaluated)
	if err != nil {
		result.Partial = true
		result.Simulation = nil
		result.EvidenceHash = scheduledPartialEvidenceHash(resolution, result.Schedule, result.Work, reports, evaluated)
		return result, err
	}
	result.Measurements = measurements
	if complete {
		canonicalReports := cloneSimulationReports(reports)
		result.Simulation = &SimulationEvidence{Resolution: cloneSimulationResolution(resolution), Reports: canonicalReports}
		result.EvidenceHash, err = simulationEvidenceHash(resolution, canonicalReports)
		if err != nil {
			result.Partial = true
			result.Simulation = nil
			result.EvidenceHash = scheduledPartialEvidenceHash(resolution, result.Schedule, result.Work, reports, evaluated)
			return result, fmt.Errorf("hash simulation evidence: %w", err)
		}
		recentTrustedSimulationTranscripts.remember(result.EvidenceHash)
		return result, nil
	}

	result.EvidenceHash = scheduledPartialEvidenceHash(resolution, result.Schedule, result.Work, reports, evaluated)
	return result, nil
}

func scheduledPartialEvidenceHash(
	resolution SimulationResolution,
	schedule []EvaluationStageEvidence,
	work []AnalysisExecution,
	reports []simmodel.Report,
	evaluated []bool,
) string {
	partialReports := make([]indexedSimulationReport, 0, len(work))
	for index, done := range evaluated {
		if done {
			partialReports = append(partialReports, indexedSimulationReport{Index: index, Report: simmodel.CloneReport(reports[index])})
		}
	}
	return hashJSON(struct {
		ResolutionHash string                    `json:"resolution_hash"`
		Schedule       []EvaluationStageEvidence `json:"schedule"`
		Work           []AnalysisExecution       `json:"work"`
		Reports        []indexedSimulationReport `json:"reports"`
	}{hashJSON(resolution), schedule, work, partialReports})
}

func (evaluator SimModelEvaluator) evaluateScheduledStage(
	ctx context.Context,
	plans []simmodel.Plan,
	scheduled []scheduledPlan,
	limits EvaluationLimits,
	used int,
	byAnalysis map[string]int,
) ([]scheduledPlanResult, int, bool, error) {
	results := make([]scheduledPlanResult, 0, len(scheduled))
	resultByHash := map[string]int{}
	var jobs []int
	executions := 0
	budgetBlocked := false
	prospectiveByAnalysis := make(map[string]int, len(byAnalysis))
	for analysis, count := range byAnalysis {
		prospectiveByAnalysis[analysis] = count
	}
	for _, plan := range scheduled {
		if err := ctx.Err(); err != nil {
			return nil, 0, false, err
		}
		if _, exists := resultByHash[plan.hash]; exists {
			results = append(results, scheduledPlanResult{plan: plan, cacheHit: true})
			continue
		}
		if evaluator.Cache != nil {
			if report, diagnostics, found := evaluator.Cache.get(plan.hash); found {
				results = append(results, scheduledPlanResult{plan: plan, report: report, diagnostics: diagnostics, cacheHit: true})
				resultByHash[plan.hash] = len(results) - 1
				continue
			}
		}
		if !scheduledWorkFits(limits, used+executions, prospectiveByAnalysis, plan.analyses) {
			budgetBlocked = true
			break
		}
		results = append(results, scheduledPlanResult{plan: plan})
		resultByHash[plan.hash] = len(results) - 1
		jobs = append(jobs, len(results)-1)
		executions += len(plan.analyses)
		for _, analysis := range plan.analyses {
			prospectiveByAnalysis[analysis]++
		}
	}

	maximumAnalysisFanout := 1
	for _, resultIndex := range jobs {
		maximumAnalysisFanout = max(maximumAnalysisFanout, min(4, len(results[resultIndex].plan.analyses)))
	}
	workerCount := runtimebudget.NestedLimit(len(jobs), limits.MaxConcurrentPlans, maximumAnalysisFanout)
	if workerCount == 1 {
		for _, resultIndex := range jobs {
			plan := results[resultIndex].plan
			results[resultIndex].report, results[resultIndex].diagnostics = simmodel.Evaluate(simmodel.ClonePlan(plans[plan.index]))
		}
	} else if workerCount > 1 {
		work := make(chan int)
		var workers sync.WaitGroup
		workers.Add(workerCount)
		for range workerCount {
			go func() {
				defer workers.Done()
				for resultIndex := range work {
					plan := results[resultIndex].plan
					results[resultIndex].report, results[resultIndex].diagnostics = simmodel.Evaluate(simmodel.ClonePlan(plans[plan.index]))
				}
			}()
		}
		for _, resultIndex := range jobs {
			work <- resultIndex
		}
		close(work)
		workers.Wait()
	}
	for analysis, count := range prospectiveByAnalysis {
		byAnalysis[analysis] = count
	}
	// Insert and resolve exact duplicates in canonical order so cache capacity,
	// cache-hit evidence, and returned bytes do not depend on worker completion.
	canonical := map[string]scheduledPlanResult{}
	for index := range results {
		entry := &results[index]
		if previous, exists := canonical[entry.plan.hash]; exists {
			entry.report = simmodel.CloneReport(previous.report)
			entry.diagnostics = append([]simmodel.Diagnostic(nil), previous.diagnostics...)
			entry.cacheHit = true
			continue
		}
		canonical[entry.plan.hash] = *entry
		if !entry.cacheHit && evaluator.Cache != nil {
			evaluator.Cache.put(entry.plan.hash, entry.report, entry.diagnostics)
		}
	}
	return results, executions, budgetBlocked, nil
}

func planAnalysisKinds(plan simmodel.Plan) []string {
	kinds := make([]string, 0, len(plan.Analyses))
	for _, analysis := range plan.Analyses {
		kinds = append(kinds, analysis.Kind)
	}
	slices.Sort(kinds)
	if len(kinds) == 0 {
		// Legacy closed-form registered models have no explicit Analysis array;
		// they are deterministic operating-point work and belong in the first
		// electrical stage.
		return []string{simmodel.AnalysisDCOperatingPoint}
	}
	return kinds
}

func planEvaluationStage(analyses []string) (EvaluationStage, bool) {
	stage := EvaluationStage("")
	rank := 0
	for _, analysis := range analyses {
		candidate, candidateRank, ok := analysisEvaluationStage(analysis)
		if !ok {
			return "", false
		}
		if rank != 0 && candidateRank != rank {
			// A plan crossing scheduler stages would execute expensive work before
			// the earlier stage can reject it. Resolvers must emit independently
			// schedulable plan batches instead of weakening ordering here.
			return "", false
		}
		if candidateRank > rank {
			stage, rank = candidate, candidateRank
		}
	}
	return stage, rank != 0
}

func analysisEvaluationStage(analysis string) (EvaluationStage, int, bool) {
	switch analysis {
	case simmodel.AnalysisDCOperatingPoint:
		return EvaluationStageDC, 1, true
	case simmodel.AnalysisACSweep, simmodel.AnalysisNoise, simmodel.AnalysisStability:
		return EvaluationStageAC, 2, true
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisDistortion:
		return EvaluationStageTransient, 3, true
	case simmodel.AnalysisThermal, simmodel.AnalysisElectrothermal:
		return EvaluationStageThermalSOA, 4, true
	default:
		return "", 0, false
	}
}

func evaluationStageRank(stage EvaluationStage) int {
	switch stage {
	case EvaluationStageStructural:
		return 0
	case EvaluationStageDC:
		return 1
	case EvaluationStageAC:
		return 2
	case EvaluationStageTransient:
		return 3
	case EvaluationStageThermalSOA:
		return 4
	case EvaluationStageExhaustivePromotion:
		return 5
	default:
		return 99
	}
}

func compareScheduledPlans(left, right scheduledPlan) int {
	if order := evaluationStageRank(left.stage) - evaluationStageRank(right.stage); order != 0 {
		return order
	}
	if order := strings.Compare(left.hash, right.hash); order != 0 {
		return order
	}
	return left.index - right.index
}

func scheduledWorkFits(limits EvaluationLimits, used int, byAnalysis map[string]int, analyses []string) bool {
	if limits.MaxAnalysisExecutions > 0 && used+len(analyses) > limits.MaxAnalysisExecutions {
		return false
	}
	requested := map[string]int{}
	for _, analysis := range analyses {
		requested[analysis]++
	}
	for analysis, count := range requested {
		if remaining, bounded := limits.AnalysisRemaining[analysis]; bounded && byAnalysis[analysis]+count > remaining {
			return false
		}
	}
	return true
}

func simulationReportFailed(report simmodel.Report) bool {
	for _, assertion := range report.Assertions {
		if !assertion.Pass {
			return true
		}
	}
	return false
}

func remainingEvaluationStages(plans []scheduledPlan) []EvaluationStage {
	var result []EvaluationStage
	for _, plan := range plans {
		if len(result) == 0 || result[len(result)-1] != plan.stage {
			result = append(result, plan.stage)
		}
	}
	return result
}

func scheduledMeasurements(plans []simmodel.Plan, reports []simmodel.Report, links []SimulationMeasurementLink, evaluated []bool) ([]Measurement, error) {
	measurements := make([]Measurement, 0, len(links))
	for _, link := range links {
		ready := true
		for _, set := range measurementAssertionSets(link) {
			if set.Plan < 0 || set.Plan >= len(evaluated) || !evaluated[set.Plan] {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}
		assertion, err := worstLinkedMeasurement(plans, reports, link)
		if err != nil {
			return nil, err
		}
		measurements = append(measurements, Measurement{
			RequirementID: link.RequirementID, OperatingCase: link.OperatingCase, Actual: assertion.Actual,
		})
	}
	slices.SortStableFunc(measurements, compareMeasurements)
	return measurements, nil
}
