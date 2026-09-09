package opentopologysynthesis

import (
	"context"
	"kicadai/internal/simulationadmission"
	"slices"
)

// EvaluateElectricalCandidateV22 preserves numerical evaluation while binding
// each assertion/case/corner to its own admission decision. The historical V20
// key omitted the assertion ID, allowing same-analysis assertions to overwrite
// one another's provenance. V20 is intentionally left byte-for-byte unchanged.
func EvaluateElectricalCandidateV22(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy) SimulationEvaluation {
	return evaluateElectricalPreparedV22(ctx, requirement, graph, nil, inventory, environment, simulationadmission.PrepareEnvironment(admission), policy)
}

func admissionAttemptKeyV22(assertion, analysis, operatingCase, corner string) string {
	return assertion + "\x00" + admissionAttemptKeyV20(analysis, operatingCase, corner)
}

func evaluateElectricalPreparedV22(
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
				decisions[admissionAttemptKeyV22(assertion.ID, assertion.Analysis, operatingCase.ID, corner.ID)] = decision
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
		decision, found := decisions[admissionAttemptKeyV22(attempt.RequirementID, attempt.Analysis, attempt.OperatingCase, attempt.CornerID)]
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
