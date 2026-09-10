package opentopologysynthesis

import (
	"context"
	"fmt"
	"slices"

	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
)

const ElectricalEvaluationSchemaV23 = "kicadai.electrical-evaluation.v23"

// SolverAttemptV23 binds numerical execution to one requested assertion/corner.
// Reports already live in Evaluation; no duplicate waveforms or plans are kept.
type SolverAttemptV23 struct {
	RequirementID string                      `json:"requirement_id"`
	Analysis      string                      `json:"analysis"`
	OperatingCase string                      `json:"operating_case"`
	CornerID      string                      `json:"corner_id"`
	Execution     simmodel.SolverExecutionV23 `json:"execution"`
	Diagnostics   []simmodel.Diagnostic       `json:"diagnostics"`
}

// ElectricalEvaluationV23 is an explicit opt-in execution contract. Historical
// admission decisions in Evaluation remain prerequisite model/backend proofs,
// not claims that the historical solver executed the corrected sweep.
type ElectricalEvaluationV23 struct {
	Schema           string               `json:"schema"`
	SolverPolicyID   string               `json:"solver_policy_id"`
	SolverPolicyHash string               `json:"solver_policy_sha256"`
	Evaluation       SimulationEvaluation `json:"evaluation"`
	SolverAttempts   []SolverAttemptV23   `json:"solver_attempts"`
	Hash             string               `json:"hash"`
}

func EvaluateElectricalCandidateV23(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy) ElectricalEvaluationV23 {
	return evaluateElectricalPreparedV23(ctx, requirement, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), policy)
}

func evaluateElectricalPreparedV23(ctx context.Context, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, policy Policy) ElectricalEvaluationV23 {
	result := ElectricalEvaluationV23{Schema: ElectricalEvaluationSchemaV23, SolverPolicyID: simmodel.SolverIDV23, SolverPolicyHash: simmodel.SolverSHA256V23(), SolverAttempts: []SolverAttemptV23{}}
	result.Evaluation = evaluateElectricalNumericalPreparedV23(ctx, requirement, graph, nil, inventory, environment, admission, policy, &result.SolverAttempts)
	return finalizeElectricalEvaluationV23(result)
}

func finalizeElectricalEvaluationV23(result ElectricalEvaluationV23) ElectricalEvaluationV23 {
	// Bind the independently authenticated numerical identity without hashing
	// the same potentially large waveform payload a second time.
	result.Hash = causalCrossStageHash(struct {
		Schema           string
		SolverPolicyID   string
		SolverPolicyHash string
		EvaluationHash   string
		SolverAttempts   []SolverAttemptV23
	}{result.Schema, result.SolverPolicyID, result.SolverPolicyHash, result.Evaluation.Hash, result.SolverAttempts})
	return result
}

// VerifyElectricalEvaluationV23 verifies exact provenance binding, not a solver
// rerun. Admission and reconstructed plans are checked independently. A passing
// repair still requires the full electrical certificate and physical promotion.
func VerifyElectricalEvaluationV23(ctx context.Context, result ElectricalEvaluationV23, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment) error {
	return verifyElectricalEvaluationPreparedV23(ctx, result, requirement, graph, inventory, environment, simulationadmission.PrepareEnvironment(admission))
}

func verifyElectricalEvaluationPreparedV23(ctx context.Context, result ElectricalEvaluationV23, requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.Schema != ElectricalEvaluationSchemaV23 || result.SolverPolicyID != simmodel.SolverIDV23 || result.SolverPolicyHash != simmodel.SolverSHA256V23() || !validDigestV22(result.Hash) || finalizeElectricalEvaluationV23(result).Hash != result.Hash {
		return fmt.Errorf("V23 evaluation solver policy or content differs")
	}
	requirement = Normalize(requirement)
	if len(Validate(requirement)) != 0 || len(validateSimulationEnvironment(inventory, environment)) != 0 {
		return fmt.Errorf("V23 evaluation requirement or environment is invalid")
	}
	inventoryHash, err := primitiveInventoryHash(inventory)
	if err != nil || inventoryHash != inventory.Hash {
		return fmt.Errorf("V23 evaluation inventory differs")
	}
	graph, err = NormalizeGraph(graph)
	if err != nil {
		return fmt.Errorf("V23 evaluation graph is invalid: %w", err)
	}
	requirementHash, _ := CanonicalHash(requirement)
	graphHash, err := GraphHash(graph)
	evaluation := result.Evaluation
	if err != nil || evaluation.Schema != SimulationEvaluationSchema || evaluation.Version != SimulationEvaluationVersion || evaluation.PolicyVersion != PolicyVersion || !validDigestV22(evaluation.Hash) || finalizeSimulationEvaluation(evaluation).Hash != evaluation.Hash || evaluation.RequirementHash != requirementHash || evaluation.GraphHash != graphHash || evaluation.InventoryHash != inventory.Hash || evaluation.ValueTrialHash != "" {
		return fmt.Errorf("V23 evaluation source identities differ")
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	required := map[string]bool{}
	if evaluation.Status == SimulationEvaluationPassed {
		if len(evaluation.Attempts) == 0 || len(evaluation.Diagnoses) != 0 || len(evaluation.Issues) != 0 {
			return fmt.Errorf("V23 passing evaluation is empty or refused")
		}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			for _, caseID := range assertion.OperatingCases {
				operatingCase := cases[caseID]
				operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
				for _, corner := range operatingCaseCornersForAssertion(assertion, operatingCase) {
					required[assertion.ID+"\x00"+caseID+"\x00"+corner.ID] = true
				}
			}
		}
	}
	recordIndex := 0
	cornerCount := len(evaluation.Attempts)
	seen := map[string]bool{}
	for index, attempt := range evaluation.Attempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		assertion, found := assertions[attempt.RequirementID]
		key := electricalAttemptKeyV22(attempt)
		if !found || seen[key] || attempt.Number != index+1 || attempt.Analysis != assertion.Analysis || attempt.Metric != assertion.Metric || !sameBoundV22(attempt.RequiredMin, assertion.Min) || !sameBoundV22(attempt.RequiredMax, assertion.Max) || !slices.Contains(assertion.OperatingCases, attempt.OperatingCase) {
			return fmt.Errorf("V23 attempt assignment or ordering differs")
		}
		seen[key] = true
		delete(required, key)
		if attempt.Report == nil {
			if attempt.ReportHash != "" || attempt.Status == SimulationEvaluationPassed || attempt.AssertionPass {
				return fmt.Errorf("V23 attempt lacks required numerical evidence")
			}
			continue
		}
		if recordIndex >= len(result.SolverAttempts) {
			return fmt.Errorf("V23 numerical execution evidence is missing")
		}
		record := result.SolverAttempts[recordIndex]
		recordIndex++
		if record.RequirementID != attempt.RequirementID || record.Analysis != attempt.Analysis || record.OperatingCase != attempt.OperatingCase || record.CornerID != attempt.CornerID {
			return fmt.Errorf("V23 numerical execution belongs to another attempt")
		}
		operatingCase := cases[attempt.OperatingCase]
		operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
		var selected operatingCorner
		for _, corner := range operatingCaseCornersForAssertion(assertion, operatingCase) {
			if corner.ID == attempt.CornerID {
				selected = corner
				break
			}
		}
		if selected.ID == "" {
			return fmt.Errorf("V23 numerical execution has an unrequested corner")
		}
		plan, scale, err := electricalNumericalPlanV22(requirement, assertion, operatingCase, selected, graph, inventory, environment)
		if err != nil || causalCrossStageHash(plan) != attempt.PlanHash || causalCrossStageHash(*attempt.Report) != attempt.ReportHash || !electricalReportPlanIdentityV22(*attempt.Report, plan) {
			return fmt.Errorf("V23 reconstructed numerical plan or report differs: %v", err)
		}
		if err := simmodel.VerifySolverExecutionV23(plan, *attempt.Report, record.Diagnostics, record.Execution); err != nil {
			return fmt.Errorf("V23 numerical execution binding differs: %w", err)
		}
		if causalCrossStageHash(normalizeSimModelDiagnostics(record.Diagnostics)) != causalCrossStageHash(attempt.Diagnostics) {
			return fmt.Errorf("V23 numerical diagnostics differ")
		}
		cornerCount += len(attempt.Report.Corners)
		if evaluation.Status == SimulationEvaluationPassed && (attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass || len(record.Diagnostics) != 0 || attempt.Actual == nil || !finite(*attempt.Actual) || !assertionValuePasses(assertion, *attempt.Actual) || attempt.Report.Status != "pass" || len(attempt.Report.Assertions) != 1 || !attempt.Report.Assertions[0].Pass || attempt.Report.Assertions[0].Actual/scale != *attempt.Actual) {
			return fmt.Errorf("V23 passing evaluation lacks a required electrical pass")
		}
		if evaluation.Status == SimulationEvaluationPassed {
			if err := simmodel.VerifyPassingCornersV23(plan, *attempt.Report); err != nil {
				return err
			}
		}
		components, _, diagnostics := simulationComponentEvidence(graph, inventory, assertion.Analysis)
		if len(diagnostics) != 0 {
			return fmt.Errorf("V23 component admission evidence unavailable")
		}
		harness, _, diagnostics := simulationHarness(requirement, assertion, operatingCase, selected, graph, environment)
		if len(diagnostics) != 0 {
			return fmt.Errorf("V23 harness admission evidence unavailable")
		}
		decision := simulationadmission.AdmitPrepared(simulationAdmissionRequest(requirement, []BehavioralAssertion{assertion}, append(components, harness...)), admission)
		if decision.Status != simulationadmission.StatusAdmitted || attempt.WorkflowModel != admittedWorkflowV20(decision, assertion.Analysis) || attempt.Report.ModelID != attempt.WorkflowModel || !slices.Contains(attempt.ModelEvidenceSHA256s, decision.Hash) {
			return fmt.Errorf("V23 prerequisite model/backend admission differs")
		}
		for _, model := range decision.Models {
			for _, digest := range []string{model.ParametersSHA256, model.ModelClaimSHA256, model.RegistrySourceSHA256, model.RegistryRecordSHA256} {
				if !validDigestV22(digest) || !slices.Contains(attempt.ModelEvidenceSHA256s, digest) {
					return fmt.Errorf("V23 model admission provenance differs")
				}
			}
		}
	}
	if recordIndex != len(result.SolverAttempts) {
		return fmt.Errorf("V23 has unassociated numerical execution evidence")
	}
	if evaluation.Status == SimulationEvaluationPassed && (len(required) != 0 || evaluation.Consumption.CandidateSimulations != len(evaluation.Attempts) || evaluation.Consumption.CornerEvaluations != cornerCount || evaluation.Consumption.CandidateSimulations > evaluation.Policy.MaxCandidateSimulations || cornerCount > evaluation.Policy.MaxCornerEvaluations || evaluation.Consumption.BudgetExhausted) {
		return fmt.Errorf("V23 passing evaluation has incomplete corner coverage or invalid accounting")
	}
	return nil
}
