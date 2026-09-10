package opentopologysynthesis

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"

	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
)

// ElectricalCertificateV22 binds an exact sequence of control edits, its structural
// feedback explanation, and every required admitted numerical assertion/corner.
// It makes no KiCad or physical-promotion claim.
type ElectricalCertificateV22 struct {
	Schema              string               `json:"schema"`
	RequirementHash     string               `json:"requirement_sha256"`
	InventoryHash       string               `json:"inventory_sha256"`
	BeforeGraphHash     string               `json:"before_graph_sha256"`
	GraphHash           string               `json:"graph_sha256"`
	EvaluationHash      string               `json:"evaluation_sha256"`
	StructureHash       string               `json:"structure_sha256"`
	Changes             []GraphChange        `json:"changes"`
	StepGraphHashes     []string             `json:"step_graph_sha256s"`
	Feedback            []FeedbackBindingV22 `json:"feedback"`
	AdmissionHashes     []string             `json:"admission_sha256s"`
	ModelEvidenceHashes []string             `json:"model_evidence_sha256s"`
	AttemptCount        int                  `json:"attempt_count"`
	Hash                string               `json:"hash"`
}

func certifyElectricalRebindingV22(ctx context.Context, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, change GraphChange, evaluation SimulationEvaluation) (ElectricalCertificateV22, error) {
	return certifyElectricalPathV22(ctx, requirement, before, after, inventory, environment, admission, []GraphChange{change}, evaluation)
}

func certifyElectricalPathV22(ctx context.Context, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, changes []GraphChange, evaluation SimulationEvaluation) (ElectricalCertificateV22, error) {
	result := ElectricalCertificateV22{}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	requirement = Normalize(requirement)
	if len(Validate(requirement)) != 0 {
		return result, fmt.Errorf("electrical certificate requirement is invalid")
	}
	inventoryHash, inventoryErr := primitiveInventoryHash(inventory)
	if inventoryErr != nil || inventoryHash != inventory.Hash {
		return result, fmt.Errorf("electrical certificate inventory content hash differs")
	}
	if len(validateSimulationEnvironment(inventory, environment)) != 0 {
		return result, fmt.Errorf("electrical certificate simulation environment differs")
	}
	initial := AnalyzeTopologyV21(requirement, before, inventory)
	if !initial.Complete || initial.Contradictory {
		return result, fmt.Errorf("electrical certificate source lacks a complete structural binding")
	}
	steps, err := validateControlPathV22(ctx, requirement, before, after, inventory, changes)
	if err != nil {
		return result, err
	}
	bindings := deriveFeedbackBindingsV22(requirement, after, inventory)
	structure := analyzeElectricalTopologyV22(requirement, after, inventory, bindings)
	if !structure.Complete || structure.Contradictory {
		return result, fmt.Errorf("electrical certificate structure is incomplete")
	}
	unsigned := evaluation
	unsigned.Hash = ""
	if evaluation.Schema != SimulationEvaluationSchema || evaluation.Version != SimulationEvaluationVersion || evaluation.Hash == "" || causalCrossStageHash(unsigned) != evaluation.Hash || evaluation.Status != SimulationEvaluationPassed || len(evaluation.Attempts) == 0 || len(evaluation.Issues) != 0 || len(evaluation.Diagnoses) != 0 || evaluation.ValueTrialHash != "" || evaluation.RequirementHash != structure.RequirementHash || evaluation.InventoryHash != inventory.Hash || evaluation.GraphHash != structure.GraphHash {
		return result, fmt.Errorf("electrical certificate requires the exact complete passing evaluation")
	}
	result = ElectricalCertificateV22{Schema: "kicadai.electrical-rebinding-certificate.v22", RequirementHash: structure.RequirementHash, InventoryHash: inventory.Hash, BeforeGraphHash: initial.GraphHash, GraphHash: structure.GraphHash, EvaluationHash: evaluation.Hash, StructureHash: structure.Hash, Changes: slices.Clone(changes), StepGraphHashes: steps, Feedback: bindings, AdmissionHashes: []string{}, ModelEvidenceHashes: []string{}, AttemptCount: len(evaluation.Attempts)}
	key := func(assertion, caseID, corner string) string { return assertion + "\x00" + caseID + "\x00" + corner }
	attempts := map[string]SimulationAttempt{}
	previous := 0
	for _, attempt := range evaluation.Attempts {
		k := key(attempt.RequirementID, attempt.OperatingCase, attempt.CornerID)
		if _, duplicate := attempts[k]; duplicate || attempt.Number <= previous {
			return ElectricalCertificateV22{}, fmt.Errorf("duplicate or unordered electrical attempt")
		}
		previous = attempt.Number
		if attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass || len(attempt.Diagnostics) != 0 || attempt.Actual == nil || !finite(*attempt.Actual) || attempt.Report == nil || attempt.Report.Status != "pass" || attempt.ReportHash == "" || causalCrossStageHash(*attempt.Report) != attempt.ReportHash || !validDigestV22(attempt.PlanHash) {
			return ElectricalCertificateV22{}, fmt.Errorf("incomplete numerical report in electrical certificate")
		}
		if len(attempt.Report.Assertions) != 1 || !attempt.Report.Assertions[0].Pass || !finite(attempt.Report.Assertions[0].Actual) {
			return ElectricalCertificateV22{}, fmt.Errorf("numerical report lacks one passing assertion")
		}
		attempts[k] = attempt
	}
	cases := map[string]OperatingCase{}
	for _, c := range requirement.Requirements.OperatingCases {
		cases[c.ID] = c
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		for _, caseID := range assertion.OperatingCases {
			operatingCase := cases[caseID]
			operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
			for _, corner := range operatingCaseCornersForAssertion(assertion, operatingCase) {
				if err := ctx.Err(); err != nil {
					return ElectricalCertificateV22{}, err
				}
				k := key(assertion.ID, caseID, corner.ID)
				attempt, found := attempts[k]
				if !found || attempt.Analysis != assertion.Analysis || attempt.Metric != assertion.Metric || !sameBoundV22(attempt.RequiredMin, assertion.Min) || !sameBoundV22(attempt.RequiredMax, assertion.Max) || (assertion.Min != nil && *attempt.Actual < *assertion.Min) || (assertion.Max != nil && *attempt.Actual > *assertion.Max) {
					return ElectricalCertificateV22{}, fmt.Errorf("required assertion/corner evidence differs")
				}
				plan, scale, planErr := electricalNumericalPlanV22(requirement, assertion, operatingCase, corner, after, inventory, environment)
				if planErr != nil || causalCrossStageHash(plan) != attempt.PlanHash || attempt.Report.Assertions[0].Actual/scale != *attempt.Actual {
					return ElectricalCertificateV22{}, fmt.Errorf("electrical plan or scaled numerical actual differs: %v", planErr)
				}
				if !electricalReportPlanIdentityV22(*attempt.Report, plan) {
					return ElectricalCertificateV22{}, fmt.Errorf("numerical report provenance differs from the exact plan")
				}
				components, _, diagnostics := simulationComponentEvidence(after, inventory, assertion.Analysis)
				if len(diagnostics) != 0 {
					return ElectricalCertificateV22{}, fmt.Errorf("electrical certificate component evidence is unavailable")
				}
				harness, _, diagnostics := simulationHarness(requirement, assertion, operatingCase, corner, after, environment)
				if len(diagnostics) != 0 {
					return ElectricalCertificateV22{}, fmt.Errorf("electrical certificate harness evidence is unavailable")
				}
				decision := simulationadmission.AdmitPrepared(simulationAdmissionRequest(requirement, []BehavioralAssertion{assertion}, append(components, harness...)), admission)
				if decision.Status != simulationadmission.StatusAdmitted || attempt.WorkflowModel != admittedWorkflowV20(decision, assertion.Analysis) || attempt.Report.ModelID != attempt.WorkflowModel || !slices.Contains(attempt.ModelEvidenceSHA256s, decision.Hash) {
					return ElectricalCertificateV22{}, fmt.Errorf("electrical certificate exact admission differs")
				}
				for _, model := range decision.Models {
					for _, digest := range []string{model.ParametersSHA256, model.ModelClaimSHA256, model.RegistrySourceSHA256, model.RegistryRecordSHA256} {
						if !validDigestV22(digest) || !slices.Contains(attempt.ModelEvidenceSHA256s, digest) {
							return ElectricalCertificateV22{}, fmt.Errorf("electrical certificate model provenance differs")
						}
					}
				}
				for _, digest := range attempt.ModelEvidenceSHA256s {
					if !validDigestV22(digest) {
						return ElectricalCertificateV22{}, fmt.Errorf("invalid electrical model evidence digest")
					}
				}
				result.AdmissionHashes = append(result.AdmissionHashes, decision.Hash)
				result.ModelEvidenceHashes = append(result.ModelEvidenceHashes, attempt.ModelEvidenceSHA256s...)
				delete(attempts, k)
			}
		}
	}
	if len(attempts) != 0 {
		return ElectricalCertificateV22{}, fmt.Errorf("unrequested electrical attempt in certificate")
	}
	slices.Sort(result.AdmissionHashes)
	result.AdmissionHashes = slices.Compact(result.AdmissionHashes)
	slices.Sort(result.ModelEvidenceHashes)
	result.ModelEvidenceHashes = slices.Compact(result.ModelEvidenceHashes)
	result.Hash = causalCrossStageHash(result)
	if result.Hash == "" {
		return ElectricalCertificateV22{}, fmt.Errorf("electrical certificate hashing failed")
	}
	return result, nil
}

// Rebuild the admitted evaluator's plan identity without executing a solver.
// This prevents a rehashed evaluation from claiming unrelated plan provenance.
func electricalNumericalPlanV22(requirement Requirement, assertion BehavioralAssertion, operatingCase OperatingCase, corner operatingCorner, graph CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment) (simmodel.Plan, float64, error) {
	translated := cloneRequirement(requirement)
	for i := range translated.Requirements.BehavioralRequirements {
		a := &translated.Requirements.BehavioralRequirements[i]
		if a.Metric == "dc_voltage" {
			a.Metric = "output_voltage"
		}
		if a.Metric == "dc_current" {
			a.Metric = "output_current"
		}
		if a.ID == assertion.ID {
			assertion = *a
		}
	}
	translated = Normalize(translated)
	quantity, scale, ok := directSimulationQuantityForRequirement(translated, assertion)
	if !ok || !finite(scale) || scale == 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable direct measurement")
	}
	if (assertion.Metric == "line_regulation" || assertion.Metric == "load_regulation") && observationIsCurrentPort(translated, assertion.Observation) {
		quantity = simmodel.QuantityDCSweepDeviceCurrentSpanA
	}
	evidence, _, diagnostics := simulationComponentEvidence(graph, inventory, assertion.Analysis)
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable component evidence")
	}
	harness, _, diagnostics := simulationHarness(translated, assertion, operatingCase, corner, graph, environment)
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable harness evidence")
	}
	evidence = append(evidence, harness...)
	thermal, _, diagnostics := simulationThermalBoundary(translated, assertion, operatingCase, corner, graph, inventory, evidence, environment.Catalog)
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable thermal evidence")
	}
	analysis, claim, diagnostics := simulationIntentParts(translated, electrothermalPeriodicBehavior(translated, assertion), operatingCase, corner, graph, evidence, quantity, scale, thermal)
	if len(diagnostics) != 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable analysis definition")
	}
	modelID, ok, _ := simmodel.ApplicableGraphModelForAnalysis(evidence, trustedModelAnalysisKind(assertion.Analysis))
	if !ok {
		return simmodel.Plan{}, 0, fmt.Errorf("unavailable workflow model")
	}
	plan, resolveDiagnostics := simmodel.ResolveWithTopology(simmodel.Intent{ModelID: modelID, Analyses: []simmodel.Analysis{analysis}, Assertions: []simmodel.Assertion{claim}, WorstCase: translated.Acceptance.RequireAllCorners}, "open-topology:"+translated.Project.Name, environment.CatalogHash, evidence, simulationNodeEvidence(translated, graph))
	if len(resolveDiagnostics) != 0 {
		return simmodel.Plan{}, 0, fmt.Errorf("unresolved numerical plan")
	}
	return plan, scale, nil
}

func electricalReportPlanIdentityV22(report simmodel.Report, plan simmodel.Plan) bool {
	if report.RegistryVersion != plan.RegistryVersion || report.RegistryHash != plan.RegistryHash || report.CatalogID != plan.CatalogID || report.CatalogHash != plan.CatalogHash || report.ModelID != plan.ModelID || report.GroundNode != plan.GroundNode || report.TopologyHash != plan.TopologyHash {
		return false
	}
	return causalCrossStageHash(report.Bindings) == causalCrossStageHash(plan.Bindings) && causalCrossStageHash(report.Inputs) == causalCrossStageHash(plan.Inputs) && causalCrossStageHash(report.Nodes) == causalCrossStageHash(plan.Nodes) && causalCrossStageHash(report.Devices) == causalCrossStageHash(plan.Devices)
}

func verifyElectricalCertificateV22(ctx context.Context, certificate ElectricalCertificateV22, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, evaluation SimulationEvaluation) error {
	expected, err := certifyElectricalPathV22(ctx, requirement, before, after, inventory, environment, admission, certificate.Changes, evaluation)
	if err != nil {
		return err
	}
	if certificate.Hash == "" || causalCrossStageHash(certificate) != causalCrossStageHash(expected) {
		return fmt.Errorf("electrical certificate content differs")
	}
	return nil
}

func validDigestV22(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func sameBoundV22(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
