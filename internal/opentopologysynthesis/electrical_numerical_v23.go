package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"slices"
	"strings"
)

// Version-isolated adapter preserves frozen work ordering, admission prerequisites,
// fail-closed diagnostics, and atomic corner budgets. Only numerical dispatch and
// its explicit execution sidecar differ from the historical evaluator.
func evaluateCandidateNumericalV23(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
	executions *[]SolverAttemptV23,
) SimulationEvaluation {
	result := SimulationEvaluation{
		Schema:        SimulationEvaluationSchema,
		Version:       SimulationEvaluationVersion,
		PolicyVersion: PolicyVersion,
		InventoryHash: inventory.Hash,
		Policy:        effectiveTopologyPolicy(policy),
		Status:        SimulationEvaluationFailed,
		Attempts:      []SimulationAttempt{},
		Diagnoses:     []Diagnosis{},
		Issues:        []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash open-topology requirement: "+err.Error(), "")}
		return finalizeSimulationEvaluation(result)
	}
	result.RequirementHash = requirementHash
	if issues := Validate(requirement); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluation(result)
	}
	if issues := validateSimulationEnvironment(inventory, environment); len(issues) != 0 {
		result.Status = SimulationEvaluationUnsupported
		result.Issues = issues
		return finalizeSimulationEvaluation(result)
	}
	if trial != nil {
		graph, err = ApplyValueTrial(graph, *trial, inventory)
		if err != nil {
			result.Issues = []reports.Issue{graphIssue(CodeValueExhausted, "value_trial", "apply value trial: "+err.Error(), "")}
			return finalizeSimulationEvaluation(result)
		}
		result.ValueTrialHash = trial.Hash
	}
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize simulation graph: "+err.Error(), "")}
		return finalizeSimulationEvaluation(result)
	}
	result.GraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash simulation graph: "+err.Error(), "")}
		return finalizeSimulationEvaluation(result)
	}
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluation(result)
	}
	if issues := validateGraphRequirementBinding(graph, requirement); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluation(result)
	}

	cases := make(map[string]OperatingCase, len(requirement.Requirements.OperatingCases))
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	type simulationWorkItem struct {
		assertion     BehavioralAssertion
		operatingCase OperatingCase
		corner        operatingCorner
	}
	nominalWork := []simulationWorkItem{}
	cornerWork := []simulationWorkItem{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		for _, caseID := range assertion.OperatingCases {
			operatingCase := cases[caseID]
			operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
			for _, corner := range operatingCaseCornersForAssertion(assertion, operatingCase) {
				work := simulationWorkItem{
					assertion:     assertion,
					operatingCase: operatingCase,
					corner:        corner,
				}
				if corner.ID == "nominal" {
					nominalWork = append(nominalWork, work)
				} else {
					cornerWork = append(cornerWork, work)
				}
			}
		}
	}
	slices.SortStableFunc(nominalWork, func(left, right simulationWorkItem) int {
		return cmp.Compare(
			simulationAnalysisCostRank(left.assertion.Analysis),
			simulationAnalysisCostRank(right.assertion.Analysis),
		)
	})
	nominalRejected := false
	cornerRejected := false
	for phaseIndex, workItems := range [][]simulationWorkItem{nominalWork, cornerWork} {
		for _, work := range workItems {
			if nominalRejected && !work.assertion.Critical {
				continue
			}
			if err := ctx.Err(); err != nil {
				result.Status = SimulationEvaluationCanceled
				result.Issues = []reports.Issue{graphIssue(CodeCanceled, "simulation", "open-topology simulation canceled", "retry with an active context")}
				return finalizeSimulationEvaluation(result)
			}
			if result.Consumption.CandidateSimulations >= result.Policy.MaxCandidateSimulations ||
				result.Consumption.CornerEvaluations >= result.Policy.MaxCornerEvaluations {
				result.Status = SimulationEvaluationExhausted
				result.Consumption.BudgetExhausted = true
				result.Issues = []reports.Issue{graphIssue(CodeSearchExhausted, "simulation.policy", "candidate-simulation or operating-corner budget exhausted", "increase the explicit count budget or narrow the operating envelope")}
				return finalizeSimulationEvaluation(result)
			}
			attempt, diagnoses := evaluateAssertionCornerV23(
				requirement,
				work.assertion,
				work.operatingCase,
				work.corner,
				graph,
				inventory,
				environment,
				executions,
				result.Policy.MaxCornerEvaluations-result.Consumption.CornerEvaluations,
			)
			if attempt.Status == SimulationEvaluationExhausted {
				result.Status = SimulationEvaluationExhausted
				result.Consumption.BudgetExhausted = true
				result.Issues = []reports.Issue{graphIssue(CodeSearchExhausted, "simulation.policy", "remaining operating-corner budget cannot contain the next atomic worst-case proof", "increase the explicit count budget or narrow the operating envelope")}
				return finalizeSimulationEvaluation(result)
			}
			attempt.Number = len(result.Attempts) + 1
			result.Attempts = append(result.Attempts, attempt)
			result.Diagnoses = append(result.Diagnoses, diagnoses...)
			// The atomic proof was preflighted above. Charge its outer attempt and
			// every returned worst-case corner before the next iteration derives
			// the remaining budget; an exhausted preflight performs no simulation
			// and returns before this accounting boundary.
			result.Consumption.CandidateSimulations++
			result.Consumption.CornerEvaluations++
			if attempt.Report != nil {
				result.Consumption.CornerEvaluations += len(attempt.Report.Corners)
			}
			if phaseIndex == 0 && attempt.Status != SimulationEvaluationPassed {
				nominalRejected = true
			} else if phaseIndex == 1 && attempt.Status != SimulationEvaluationPassed {
				cornerRejected = true
				break
			}
		}
		if nominalRejected || cornerRejected {
			// A rejected candidate cannot become passing by collecting more
			// stress-corner failures. Complete the nominal phase so critical
			// evidence survives noncritical failures, then preserve the first
			// deterministic non-nominal diagnosis for repair. Passing candidates
			// still traverse every declared operating corner.
			break
		}
	}
	slices.SortFunc(result.Diagnoses, compareDiagnoses)
	if len(result.Attempts) == 0 {
		result.Status = SimulationEvaluationUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeNoPassingGraph, "requirements.behavioral_requirements", "no simulation attempts were generated", "declare at least one bounded operating case")}
		return finalizeSimulationEvaluation(result)
	}
	result.Status = SimulationEvaluationPassed
	unsupported := false
	for _, attempt := range result.Attempts {
		if attempt.Status == SimulationEvaluationFailed {
			result.Status = SimulationEvaluationFailed
			break
		}
		if attempt.Status == SimulationEvaluationUnsupported {
			unsupported = true
		}
	}
	if result.Status == SimulationEvaluationPassed && unsupported {
		result.Status = SimulationEvaluationUnsupported
	}
	return finalizeSimulationEvaluation(result)
}
func evaluateAssertionCornerV23(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	executions *[]SolverAttemptV23,
	cornerBudget ...int,
) (SimulationAttempt, []Diagnosis) {
	attempt := SimulationAttempt{
		RequirementID: assertion.ID,
		OperatingCase: operatingCase.ID,
		CornerID:      corner.ID,
		Analysis:      assertion.Analysis,
		Metric:        assertion.Metric,
		Status:        SimulationEvaluationFailed,
		RequiredMin:   cloneInventoryFloat(assertion.Min),
		RequiredMax:   cloneInventoryFloat(assertion.Max),
		Diagnostics:   []SimulationDiagnostic{},
	}
	quantity, scale, supported := directSimulationQuantityForRequirement(requirement, assertion)
	if !supported {
		diagnosis := simulationDiagnosis(
			diagnosisMetricUnsupported,
			assertion,
			operatingCase.ID+"/"+corner.ID,
			nil,
			graphConeHash(graph, observationNodeID(graph, requirement, assertion.Observation)),
			"",
			"behavioral metric has no trusted direct simulation measurement",
		)
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = []SimulationDiagnostic{{
			Code:    diagnosisMetricUnsupported,
			Path:    "behavioral_requirements." + assertion.ID + ".metric",
			Message: diagnosis.Message,
		}}
		return attempt, []Diagnosis{diagnosis}
	}
	if (assertion.Metric == "line_regulation" || assertion.Metric == "load_regulation") &&
		observationIsCurrentPort(requirement, assertion.Observation) {
		quantity = simmodel.QuantityDCSweepDeviceCurrentSpanA
	}
	evidence, evidenceHashes, diagnostics := simulationComponentEvidence(
		graph,
		inventory,
		assertion.Analysis,
	)
	if len(diagnostics) != 0 {
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = diagnostics
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, diagnostics)}
	}
	nodes := simulationNodeEvidence(requirement, graph)
	harness, harnessHashes, harnessDiagnostics := simulationHarness(
		requirement,
		assertion,
		operatingCase,
		corner,
		graph,
		environment,
	)
	if len(harnessDiagnostics) != 0 {
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = harnessDiagnostics
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, harnessDiagnostics)}
	}
	evidence = append(evidence, harness...)
	evidenceHashes = append(evidenceHashes, harnessHashes...)
	thermalConditions, thermalHashes, thermalDiagnostics := simulationThermalBoundary(
		requirement, assertion, operatingCase, corner, graph, inventory, evidence, environment.Catalog,
	)
	if len(thermalDiagnostics) != 0 {
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = thermalDiagnostics
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, thermalDiagnostics)}
	}
	evidenceHashes = append(evidenceHashes, thermalHashes...)
	slices.Sort(evidenceHashes)
	attempt.ModelEvidenceSHA256s = slices.Compact(evidenceHashes)

	simulationBehavior := electrothermalPeriodicBehavior(requirement, assertion)
	analysis, simulationAssertion, analysisDiagnostics := simulationIntentParts(
		requirement,
		simulationBehavior,
		operatingCase,
		corner,
		graph,
		evidence,
		quantity,
		scale,
		thermalConditions,
	)
	if len(analysisDiagnostics) != 0 {
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = analysisDiagnostics
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, analysisDiagnostics)}
	}
	modelID, ok, reason := simmodel.ApplicableGraphModelForAnalysis(evidence, trustedModelAnalysisKind(assertion.Analysis))
	if !ok {
		diagnostic := SimulationDiagnostic{
			Code:       diagnosisModelUnavailable,
			Path:       "simulation.model",
			Message:    reason,
			Suggestion: "onboard a reviewed primitive model covering the required analysis",
		}
		attempt.Status = SimulationEvaluationUnsupported
		attempt.Diagnostics = []SimulationDiagnostic{diagnostic}
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, []SimulationDiagnostic{diagnostic})}
	}
	attempt.WorkflowModel = modelID
	intent := simmodel.Intent{
		ModelID:    modelID,
		Analyses:   []simmodel.Analysis{analysis},
		Assertions: []simmodel.Assertion{simulationAssertion},
		WorstCase:  requirement.Acceptance.RequireAllCorners,
	}
	plan, resolveDiagnostics := simmodel.ResolveWithTopology(
		intent,
		"open-topology:"+requirement.Project.Name,
		environment.CatalogHash,
		evidence,
		nodes,
	)
	attempt.PlanHash = hashJSON(plan)
	if len(resolveDiagnostics) != 0 {
		attempt.Diagnostics = normalizeSimModelDiagnostics(resolveDiagnostics)
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, attempt.Diagnostics)}
	}
	if len(cornerBudget) != 0 &&
		1+simmodel.CornerEvaluationUpperBound(plan) > cornerBudget[0] {
		attempt.Status = SimulationEvaluationExhausted
		return attempt, nil
	}
	report, evaluateDiagnostics, execution := simmodel.EvaluateWithSolverV23(plan)
	*executions = append(*executions, SolverAttemptV23{
		RequirementID: assertion.ID, Analysis: assertion.Analysis, OperatingCase: operatingCase.ID, CornerID: corner.ID,
		Execution: execution, Diagnostics: slices.Clone(evaluateDiagnostics),
	})
	attempt.ReportHash = hashJSON(report)
	attempt.Report = &report
	if len(evaluateDiagnostics) != 0 {
		attempt.Diagnostics = normalizeSimModelDiagnostics(evaluateDiagnostics)
		if actual, found := failedSimulationActual(report, evaluateDiagnostics, scale); found {
			attempt.Actual = &actual
			code := diagnosisAssertionBelowMinimum
			direction := "below_minimum"
			if assertion.Max != nil && actual > *assertion.Max {
				code = diagnosisAssertionAboveMaximum
				direction = "above_maximum"
			}
			diagnosis := simulationDiagnosis(
				code,
				assertion,
				operatingCase.ID+"/"+corner.ID,
				&actual,
				graphConeHash(graph, observationNodeID(graph, requirement, assertion.Observation)),
				attempt.ReportHash,
				attempt.Diagnostics[0].Message,
			)
			diagnosis.Direction = direction
			return attempt, []Diagnosis{diagnosis}
		}
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, attempt.Diagnostics)}
	}
	if len(report.Assertions) != 1 {
		diagnostic := SimulationDiagnostic{Code: diagnosisSimulationInvalid, Path: "simulation.report.assertions", Message: "trusted evaluator returned an unexpected assertion count"}
		attempt.Diagnostics = []SimulationDiagnostic{diagnostic}
		return attempt, []Diagnosis{diagnosisFromSimulationDiagnostics(assertion, operatingCase.ID+"/"+corner.ID, graph, attempt.Diagnostics)}
	}
	actual := report.Assertions[0].Actual / scale
	attempt.Actual = &actual
	attempt.AssertionPass = assertionValuePasses(assertion, actual)
	if attempt.AssertionPass && report.Status == "pass" {
		attempt.Status = SimulationEvaluationPassed
		return attempt, nil
	}
	code := diagnosisAssertionBelowMinimum
	direction := "below_minimum"
	if assertion.Max != nil && actual > *assertion.Max {
		code = diagnosisAssertionAboveMaximum
		direction = "above_maximum"
	}
	diagnosis := simulationDiagnosis(
		code,
		assertion,
		operatingCase.ID+"/"+corner.ID,
		&actual,
		graphConeHash(graph, observationNodeID(graph, requirement, assertion.Observation)),
		attempt.ReportHash,
		fmt.Sprintf("observed %.12g %s is %s", actual, assertion.Unit, strings.ReplaceAll(direction, "_", " ")),
	)
	diagnosis.Direction = direction
	return attempt, []Diagnosis{diagnosis}
}
