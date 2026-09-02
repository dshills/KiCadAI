package opentopologysynthesis

import (
	"context"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

// EvaluateCandidateV20 admits every required analysis against the exact graph
// component models before invoking the established numerical evaluator. The
// historical evaluator remains byte-frozen. V20 binds every attempt to the
// admission decision and exact selected model records through immutable hashes;
// the full decision is returned by the V20 production sidecar.
func EvaluateCandidateV20(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admissionEnvironment simulationadmission.Environment,
	policy Policy,
) SimulationEvaluation {
	return evaluateCandidatePreparedV20(
		ctx, requirement, graph, trial, inventory, environment,
		simulationadmission.PrepareEnvironment(admissionEnvironment), policy,
	)
}

func evaluateCandidatePreparedV20(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admissionEnvironment simulationadmission.PreparedEnvironment,
	policy Policy,
) SimulationEvaluation {
	original := Normalize(requirement)
	graphForAdmission := graph
	if trial != nil {
		var err error
		graphForAdmission, err = ApplyValueTrial(graphForAdmission, *trial, inventory)
		if err != nil {
			return simulationPreparationFailureV20(original, graph, trial, inventory, policy, CodeValueExhausted, "value_trial", "apply value trial before admission: "+err.Error())
		}
	}
	graphForAdmission, err := NormalizeGraph(graphForAdmission)
	if err != nil {
		return simulationPreparationFailureV20(original, graph, trial, inventory, policy, CodeNoCompleteGraph, "graph", "normalize graph before admission: "+err.Error())
	}

	decisions := map[string]simulationadmission.Decision{}
	cases := make(map[string]OperatingCase, len(original.Requirements.OperatingCases))
	for _, operatingCase := range original.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	for _, assertion := range original.Requirements.BehavioralRequirements {
		for _, caseID := range assertion.OperatingCases {
			operatingCase := cases[caseID]
			operatingCase.Conditions = simulationHarnessConditions(original, assertion, operatingCase)
			for _, corner := range operatingCaseCornersForAssertion(assertion, operatingCase) {
				components, _, componentDiagnostics := simulationComponentEvidence(graphForAdmission, inventory, assertion.Analysis)
				if len(componentDiagnostics) != 0 {
					return simulationEvidenceFailureV20(
						original, graphForAdmission, trial, inventory, policy,
						assertion, operatingCase.ID, corner.ID, componentDiagnostics,
					)
				}
				harness, _, harnessDiagnostics := simulationHarness(
					original, assertion, operatingCase, corner, graphForAdmission, environment,
				)
				if len(harnessDiagnostics) != 0 {
					return simulationEvidenceFailureV20(
						original, graphForAdmission, trial, inventory, policy,
						assertion, operatingCase.ID, corner.ID, harnessDiagnostics,
					)
				}
				components = append(components, harness...)
				decision := simulationadmission.AdmitPrepared(
					simulationAdmissionRequest(original, []BehavioralAssertion{assertion}, components),
					admissionEnvironment,
				)
				decisions[admissionAttemptKeyV20(assertion.Analysis, operatingCase.ID, corner.ID)] = decision
				if decision.Status != simulationadmission.StatusAdmitted {
					return admissionRefusalEvaluationV20(original, graphForAdmission, trial, inventory, policy, decision)
				}
			}
		}
	}

	translated := cloneRequirement(original)
	metricByRequirement := map[string]string{}
	for index := range translated.Requirements.BehavioralRequirements {
		assertion := &translated.Requirements.BehavioralRequirements[index]
		metricByRequirement[assertion.ID] = assertion.Metric
		switch assertion.Metric {
		case "dc_voltage":
			assertion.Metric = "output_voltage"
		case "dc_current":
			assertion.Metric = "output_current"
		}
	}
	translated = Normalize(translated)
	result := EvaluateCandidate(ctx, translated, graph, trial, inventory, environment, policy)
	originalHash, _ := CanonicalHash(original)
	result.RequirementHash = originalHash
	workflowMismatch := false
	for index := range result.Attempts {
		attempt := &result.Attempts[index]
		if metric := metricByRequirement[attempt.RequirementID]; metric != "" {
			attempt.Metric = metric
		}
		decision, found := decisions[admissionAttemptKeyV20(attempt.Analysis, attempt.OperatingCase, attempt.CornerID)]
		if !found {
			continue
		}
		workflow := admittedWorkflowV20(decision, attempt.Analysis)
		if attempt.WorkflowModel != "" && workflow != "" && attempt.WorkflowModel != workflow {
			workflowMismatch = true
			attempt.Status = SimulationEvaluationUnsupported
			attempt.Diagnostics = []SimulationDiagnostic{{
				Code:    string(simulationadmission.CodeSolverModelIncompatible),
				Path:    "simulation.workflow_model",
				Message: "numerical evaluator selected a workflow model different from the admitted exact model",
			}}
		}
		attempt.ModelEvidenceSHA256s = append(attempt.ModelEvidenceSHA256s, decision.Hash)
		for _, model := range decision.Models {
			attempt.ModelEvidenceSHA256s = append(attempt.ModelEvidenceSHA256s, model.ParametersSHA256, model.ModelClaimSHA256, model.RegistrySourceSHA256)
			attempt.ModelEvidenceSHA256s = append(attempt.ModelEvidenceSHA256s, model.RegistryRecordSHA256)
		}
		slices.Sort(attempt.ModelEvidenceSHA256s)
		attempt.ModelEvidenceSHA256s = slices.Compact(attempt.ModelEvidenceSHA256s)
	}
	for index := range result.Diagnoses {
		if metric := metricByRequirement[result.Diagnoses[index].RequirementID]; metric != "" {
			result.Diagnoses[index].Metric = metric
		}
	}
	if workflowMismatch {
		result.Status = SimulationEvaluationUnsupported
		result.Diagnoses = append(result.Diagnoses, Diagnosis{
			Code:    string(simulationadmission.CodeSolverModelIncompatible),
			Message: "numerical evaluator selected a workflow model different from the admitted exact model",
		})
		slices.SortFunc(result.Diagnoses, compareDiagnoses)
	}
	return finalizeSimulationEvaluation(result)
}

func simulationEvidenceFailureV20(
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	policy Policy,
	assertion BehavioralAssertion,
	operatingCase string,
	corner string,
	diagnostics []SimulationDiagnostic,
) SimulationEvaluation {
	result := SimulationEvaluation{
		Schema: SimulationEvaluationSchema, Version: SimulationEvaluationVersion,
		PolicyVersion: PolicyVersion, InventoryHash: inventory.Hash,
		Status: SimulationEvaluationUnsupported, Policy: effectiveTopologyPolicy(policy),
		Attempts: []SimulationAttempt{{
			Number: 1, RequirementID: assertion.ID, OperatingCase: operatingCase, CornerID: corner,
			Analysis: assertion.Analysis, Metric: assertion.Metric,
			Status: SimulationEvaluationUnsupported, Diagnostics: append([]SimulationDiagnostic(nil), diagnostics...),
			ModelEvidenceSHA256s: []string{},
		}},
		Diagnoses: []Diagnosis{}, Issues: []reports.Issue{},
	}
	result.RequirementHash, _ = CanonicalHash(requirement)
	result.GraphHash, _ = GraphHash(graph)
	if trial != nil {
		result.ValueTrialHash = trial.Hash
	}
	for _, diagnostic := range diagnostics {
		result.Diagnoses = append(result.Diagnoses, simulationDiagnosis(
			diagnostic.Code, assertion, operatingCase+"/"+corner, nil,
			graphConeHash(graph, ""), result.GraphHash, diagnostic.Message,
		))
	}
	if len(diagnostics) != 0 {
		result.Issues = []reports.Issue{graphIssue(
			CodeModelUnavailable, diagnostics[0].Path, diagnostics[0].Message,
			"correct the component or harness evidence before simulation admission",
		)}
	}
	slices.SortFunc(result.Diagnoses, compareDiagnoses)
	return finalizeSimulationEvaluation(result)
}

func simulationPreparationFailureV20(
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	policy Policy,
	code reports.Code,
	path string,
	message string,
) SimulationEvaluation {
	result := SimulationEvaluation{
		Schema: SimulationEvaluationSchema, Version: SimulationEvaluationVersion,
		PolicyVersion: PolicyVersion, InventoryHash: inventory.Hash,
		Status: SimulationEvaluationUnsupported, Policy: effectiveTopologyPolicy(policy),
		Attempts: []SimulationAttempt{}, Diagnoses: []Diagnosis{},
		Issues: []reports.Issue{graphIssue(code, path, message, "correct the candidate before simulation admission")},
	}
	result.RequirementHash, _ = CanonicalHash(requirement)
	result.GraphHash, _ = GraphHash(graph)
	if trial != nil {
		result.ValueTrialHash = trial.Hash
	}
	return finalizeSimulationEvaluation(result)
}

func admissionAttemptKeyV20(analysis, operatingCase, corner string) string {
	return strings.Join([]string{analysis, operatingCase, corner}, "\x00")
}

func directSimulationQuantityForRequirementV20(requirement Requirement, assertion BehavioralAssertion) (string, float64, bool) {
	translated := assertion
	switch strings.TrimSpace(translated.Metric) {
	case "dc_voltage":
		translated.Metric = "output_voltage"
	case "dc_current":
		translated.Metric = "output_current"
	}
	return directSimulationQuantityForRequirement(requirement, translated)
}

func admissionRefusalEvaluationV20(
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	policy Policy,
	decision simulationadmission.Decision,
) SimulationEvaluation {
	result := SimulationEvaluation{
		Schema: SimulationEvaluationSchema, Version: SimulationEvaluationVersion,
		PolicyVersion: PolicyVersion, InventoryHash: inventory.Hash,
		Status: SimulationEvaluationUnsupported, Policy: effectiveTopologyPolicy(policy),
		Attempts: []SimulationAttempt{}, Diagnoses: []Diagnosis{}, Issues: []reports.Issue{},
	}
	result.RequirementHash, _ = CanonicalHash(requirement)
	result.GraphHash, _ = GraphHash(graph)
	if trial != nil {
		result.ValueTrialHash = trial.Hash
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.Analysis] = assertion
	}
	for _, diagnostic := range decision.Diagnostics {
		assertion := assertions[diagnostic.Analysis]
		if assertion.ID == "" {
			for _, candidate := range requirement.Requirements.BehavioralRequirements {
				assertion = candidate
				break
			}
		}
		operatingCase := ""
		if len(assertion.OperatingCases) != 0 {
			operatingCase = assertion.OperatingCases[0]
		}
		simulationDiagnostic := SimulationDiagnostic{
			Code: string(diagnostic.Code), Path: diagnostic.Path,
			Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
		}
		result.Attempts = append(result.Attempts, SimulationAttempt{
			Number: len(result.Attempts) + 1, RequirementID: assertion.ID,
			OperatingCase: operatingCase, CornerID: "admission", Analysis: assertion.Analysis,
			Metric: assertion.Metric, Status: SimulationEvaluationUnsupported,
			Diagnostics:          []SimulationDiagnostic{simulationDiagnostic},
			ModelEvidenceSHA256s: []string{decision.Hash},
		})
		result.Diagnoses = append(result.Diagnoses, simulationDiagnosis(
			string(diagnostic.Code), assertion, operatingCase+"/admission", nil,
			graphConeHash(graph, ""), decision.Hash, diagnostic.Message,
		))
	}
	slices.SortFunc(result.Diagnoses, compareDiagnoses)
	return finalizeSimulationEvaluation(result)
}

func admittedWorkflowV20(decision simulationadmission.Decision, analysis string) string {
	for _, item := range decision.Analyses {
		if item.CanonicalKind == analysis || item.AuthoredKind == analysis {
			return item.WorkflowModelID
		}
	}
	return ""
}
