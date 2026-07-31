package opentopologysynthesis

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	diagnosisAssertionBelowMinimum = "assertion_below_minimum"
	diagnosisAssertionAboveMaximum = "assertion_above_maximum"
	diagnosisMetricUnsupported     = "metric_unsupported"
	diagnosisModelUnavailable      = "model_unavailable"
	diagnosisOperatingPointInvalid = "operating_point_invalid"
	diagnosisNonconvergent         = "nonconvergent"
	diagnosisUnstable              = "unstable"
	diagnosisThermalUnavailable    = "thermal_model_unavailable"
	diagnosisSimulationInvalid     = "simulation_invalid"
)

type SimulationEnvironment struct {
	Catalog       *components.Catalog
	CatalogHash   string
	ModelRegistry modelprovenance.Registry
}

type operatingCorner struct {
	ID     string
	Values map[string]float64
}

func EvaluateCandidate(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	trial *ValueTrial,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
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
			for _, corner := range operatingCaseCorners(operatingCase) {
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
	for phaseIndex, workItems := range [][]simulationWorkItem{nominalWork, cornerWork} {
		if phaseIndex == 1 && nominalRejected {
			break
		}
		for _, work := range workItems {
			if phaseIndex == 0 && nominalRejected {
				break
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
			attempt, diagnoses := evaluateAssertionCorner(
				requirement,
				work.assertion,
				work.operatingCase,
				work.corner,
				graph,
				inventory,
				environment,
			)
			attempt.Number = len(result.Attempts) + 1
			result.Attempts = append(result.Attempts, attempt)
			result.Diagnoses = append(result.Diagnoses, diagnoses...)
			result.Consumption.CandidateSimulations++
			result.Consumption.CornerEvaluations++
			if attempt.Report != nil {
				result.Consumption.CornerEvaluations += len(attempt.Report.Corners)
			}
			if phaseIndex == 0 && attempt.Status != SimulationEvaluationPassed {
				nominalRejected = true
			}
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

func validateSimulationEnvironment(
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
) []reports.Issue {
	issues := []reports.Issue{}
	if environment.Catalog == nil || len(environment.CatalogHash) != 64 ||
		environment.CatalogHash != inventory.CatalogHash {
		issues = append(issues, graphIssue(CodePrimitiveUnavailable, "simulation.catalog", "simulation catalog does not match the primitive inventory", "bind the immutable catalog used to build the inventory"))
	}
	registryHash, err := modelprovenance.Hash(environment.ModelRegistry)
	if err != nil || registryHash != inventory.ModelRegistryHash {
		issues = append(issues, graphIssue(CodeModelUnavailable, "simulation.model_registry", "simulation model-provenance registry does not match the primitive inventory", "bind the reviewed registry used to build the inventory"))
	}
	return reports.SortedIssues(issues)
}

func evaluateAssertionCorner(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
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
	quantity, scale, supported := directSimulationQuantity(assertion)
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
	nodes := simulationNodeEvidence(graph)
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
	slices.Sort(evidenceHashes)
	attempt.ModelEvidenceSHA256s = slices.Compact(evidenceHashes)

	analysis, simulationAssertion, analysisDiagnostics := simulationIntentParts(
		requirement,
		assertion,
		operatingCase,
		corner,
		graph,
		evidence,
		quantity,
		scale,
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
	report, evaluateDiagnostics := simmodel.Evaluate(plan)
	attempt.ReportHash = hashJSON(report)
	attempt.Report = &report
	if len(evaluateDiagnostics) != 0 {
		attempt.Diagnostics = normalizeSimModelDiagnostics(evaluateDiagnostics)
		if actual, found := failedSimulationActual(report, scale); found {
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

func simulationAnalysisCostRank(analysis string) int {
	switch trustedModelAnalysisKind(analysis) {
	case simmodel.AnalysisDCOperatingPoint,
		simmodel.AnalysisACSweep,
		simmodel.AnalysisNoise,
		simmodel.AnalysisStability:
		return 0
	case simmodel.AnalysisTransient,
		simmodel.AnalysisStartup,
		simmodel.AnalysisDistortion:
		return 1
	case simmodel.AnalysisThermal,
		simmodel.AnalysisElectrothermal:
		return 2
	default:
		return 3
	}
}

func failedSimulationActual(report simmodel.Report, scale float64) (float64, bool) {
	for _, corner := range report.Corners {
		for _, assertion := range corner.Assertions {
			if !assertion.Pass && finite(assertion.Actual) {
				return assertion.Actual / scale, true
			}
		}
	}
	for _, assertion := range report.Assertions {
		if !assertion.Pass && finite(assertion.Actual) {
			return assertion.Actual / scale, true
		}
	}
	return 0, false
}

func directSimulationQuantity(assertion BehavioralAssertion) (string, float64, bool) {
	switch assertion.Metric {
	case "output_voltage", "output_high_voltage", "output_low_voltage", "on_state_voltage":
		return simmodel.QuantityVoltageV, 1, true
	case "voltage_gain", "voltage_gain_at_frequency":
		return simmodel.QuantityVoltageGainRatio, 1, true
	case "cutoff_frequency":
		return simmodel.QuantityCutoffFrequencyHz, 1, true
	case "phase_margin":
		return simmodel.QuantityPhaseMarginDeg, 1, true
	case "settling_time":
		return simmodel.QuantitySettlingTimeS, 1, true
	case "propagation_delay":
		return simmodel.QuantityResponseTimeS, 1, true
	case "output_noise_rms":
		return simmodel.QuantityIntegratedNoiseVRMS, 1, true
	case "thd":
		return simmodel.QuantityTHDPercent, 100, true
	case "rising_threshold":
		return simmodel.QuantityRisingThresholdVoltageV, 1, true
	case "falling_threshold":
		return simmodel.QuantityFallingThresholdVoltageV, 1, true
	case "lower_threshold":
		return simmodel.QuantityLowerThresholdVoltageV, 1, true
	case "upper_threshold":
		return simmodel.QuantityUpperThresholdVoltageV, 1, true
	case "hysteresis":
		return simmodel.QuantityHysteresisVoltageV, 1, true
	case "startup_output_voltage", "peak_voltage":
		return simmodel.QuantityPeakAbsVoltageV, 1, true
	case "startup_overshoot":
		return simmodel.QuantityOvershootVoltageV, 1, true
	case "output_current":
		return simmodel.QuantityDeviceCurrentA, 1, true
	case "off_state_current":
		if assertion.Observation.Kind == "port" {
			return simmodel.QuantityDeviceCurrentA, 1, true
		}
		return simmodel.QuantityTotalSupplyCurrentA, 1, true
	case "startup_current":
		return simmodel.QuantityPeakAbsDeviceCurrentA, 1, true
	case "transconductance":
		return simmodel.QuantityDCSweepDeviceSlopeAperV, 1, true
	case "line_regulation", "load_regulation":
		return simmodel.QuantityDCSweepVoltageSpanV, 1, true
	case "junction_temperature":
		return simmodel.QuantityMaximumJunctionTemperatureC, 1, true
	case "soa_margin":
		return simmodel.QuantityMinimumTransientSOAMargin, 1, true
	default:
		return "", 1, false
	}
}

func trustedModelAnalysisKind(analysis string) string {
	if analysis == "dc_sweep" {
		return simmodel.AnalysisDCOperatingPoint
	}
	return analysis
}

func simulationComponentEvidence(
	graph CandidateGraph,
	inventory PrimitiveInventory,
	analysis string,
) ([]simmodel.ComponentEvidence, []string, []SimulationDiagnostic) {
	result := []simmodel.ComponentEvidence{}
	hashes := []string{}
	diagnostics := []SimulationDiagnostic{}
	modelAnalysis := trustedModelAnalysisKind(analysis)
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			diagnostics = append(diagnostics, SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "graph.instances." + instance.ID, Message: "instance primitive is absent from the reviewed inventory"})
			continue
		}
		claims := []simmodel.CatalogEvidence{}
		for _, model := range primitive.Models {
			if !reviewedModelSupportsCircuitAnalysis(model.AllowedAnalyses, modelAnalysis) {
				continue
			}
			claims = append(claims, simmodel.CatalogEvidence{
				ModelID:       model.ModelID,
				Parameters:    append([]simmodel.NamedValue(nil), model.Parameters...),
				Uncertainties: append([]simmodel.Uncertainty(nil), model.Uncertainties...),
				ThermalModel:  cloneThermalModel(model.ThermalModel),
				TransientSOA:  cloneTransientSOAEnvelopes(model.TransientSOA),
			})
			hashes = append(hashes, model.ProvenanceSHA256)
		}
		if len(claims) == 0 {
			diagnostics = append(diagnostics, SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "graph.instances." + instance.ID, Message: "instance has no reviewed primitive claim for " + analysis})
			continue
		}
		connections := make([]simmodel.ConnectionEvidence, 0, len(instance.Terminals))
		for _, terminal := range instance.Terminals {
			connections = append(connections, simmodel.ConnectionEvidence{Function: terminal.Terminal, UnitID: primitive.UnitID, Net: terminal.Node})
		}
		uncertainties := valueUncertainty(instance, primitive)
		for _, claim := range claims {
			uncertainties = append(uncertainties, claim.Uncertainties...)
		}
		uncertainties = normalizeSimulationUncertainties(uncertainties)
		component := simmodel.ComponentEvidence{
			InstanceID:    instance.ID,
			CatalogID:     primitive.CatalogID,
			Family:        primitive.Family,
			ModelClaims:   claims,
			Connections:   connections,
			Uncertainties: uncertainties,
		}
		if instance.ValueSI != nil {
			component.HasValueSI = true
			component.ValueSI = *instance.ValueSI
		}
		result = append(result, component)
	}
	slices.SortFunc(result, func(left, right simmodel.ComponentEvidence) int {
		return cmp.Compare(left.InstanceID, right.InstanceID)
	})
	slices.Sort(hashes)
	return result, slices.Compact(hashes), diagnostics
}

func simulationNodeEvidence(graph CandidateGraph) []simmodel.NodeEvidence {
	result := make([]simmodel.NodeEvidence, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		role := "signal"
		switch node.Role {
		case "reference":
			role = "ground"
		case "supply":
			role = "power_pos"
		case "control":
			role = "control"
		}
		result = append(result, simmodel.NodeEvidence{Name: node.ID, Role: role, VoltageDomain: node.Domain})
	}
	slices.SortFunc(result, func(left, right simmodel.NodeEvidence) int {
		return cmp.Compare(left.Name, right.Name)
	})
	return result
}

func simulationHarness(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	environment SimulationEnvironment,
) ([]simmodel.ComponentEvidence, []string, []SimulationDiagnostic) {
	reference := referenceNodeForDomain(graph, assertion.Observation)
	if reference == "" {
		return nil, nil, []SimulationDiagnostic{{Code: diagnosisSimulationInvalid, Path: "simulation.reference", Message: "candidate graph has no semantic reference node"}}
	}
	result := []simmodel.ComponentEvidence{}
	hashes := []string{}
	diagnostics := []SimulationDiagnostic{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" || node.Role == "output" {
			continue
		}
		value := sourceValueForNode(requirement, operatingCase, corner, node)
		instanceID := canonicalIdentifier("source_" + node.ID)
		record, provenanceHashes, ok := selectHarnessRecord(
			environment,
			"voltage_source",
			simmodel.PrimitiveVoltageSourceV1,
			trustedModelAnalysisKind(assertion.Analysis),
		)
		if !ok {
			diagnostics = append(diagnostics, SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.harness." + instanceID, Message: "reviewed voltage-source harness primitive is unavailable"})
			continue
		}
		result = append(result, simmodel.ComponentEvidence{
			InstanceID:  instanceID,
			CatalogID:   record.ID,
			Family:      record.Family,
			ModelClaims: cloneCatalogClaims(record.SimulationModels),
			Connections: []simmodel.ConnectionEvidence{
				{Function: "POSITIVE", Net: node.ID},
				{Function: "NEGATIVE", Net: reference},
			},
		})
		_ = value
		hashes = append(hashes, provenanceHashes...)
	}
	for _, condition := range operatingCase.Conditions {
		target, found := externalNodeForSemanticTarget(graph, condition.Target)
		if !found {
			continue
		}
		value := corner.Values[conditionKey(condition)]
		var family, modelID, firstTerminal, secondTerminal string
		switch condition.Axis {
		case "load_resistance":
			family, modelID, firstTerminal, secondTerminal = "resistor", simmodel.PrimitiveResistorV1, "A", "B"
		case "load_capacitance":
			if value <= 0 {
				continue
			}
			family, modelID, firstTerminal, secondTerminal = "capacitor", capacitorHarnessModel(assertion.Analysis), "A", "B"
		case "load_inductance":
			family, modelID, firstTerminal, secondTerminal = "inductor", simmodel.PrimitiveInductorTransientV1, "A", "B"
		case "load_current":
			family, modelID, firstTerminal, secondTerminal = "current_source", simmodel.PrimitiveCurrentSourceV1, "POSITIVE", "NEGATIVE"
		default:
			continue
		}
		record, provenanceHashes, ok := selectHarnessRecord(environment, family, modelID, trustedModelAnalysisKind(assertion.Analysis))
		instanceID := loadInstanceID(condition.Target, condition.Axis)
		if !ok {
			diagnostics = append(diagnostics, SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.harness." + instanceID, Message: "reviewed " + family + " harness primitive is unavailable"})
			continue
		}
		component := simmodel.ComponentEvidence{
			InstanceID:  instanceID,
			CatalogID:   record.ID,
			Family:      record.Family,
			ModelClaims: cloneCatalogClaims(record.SimulationModels),
		}
		if condition.Axis == "load_inductance" {
			if seriesResistance, found := derivedInductiveLoadSeriesResistance(
				requirement,
				condition.Target,
			); found {
				for claimIndex := range component.ModelClaims {
					for parameterIndex := range component.ModelClaims[claimIndex].Parameters {
						if component.ModelClaims[claimIndex].Parameters[parameterIndex].Name ==
							"series_resistance_ohm" {
							component.ModelClaims[claimIndex].Parameters[parameterIndex].Value =
								seriesResistance
						}
					}
				}
			}
		}
		firstNode, secondNode := loadHarnessNodes(
			requirement,
			graph,
			target,
			reference,
		)
		component.Connections = []simmodel.ConnectionEvidence{
			{Function: firstTerminal, Net: firstNode},
			{Function: secondTerminal, Net: secondNode},
		}
		if condition.Axis != "load_current" {
			component.ValueSI = value
			component.HasValueSI = true
		}
		result = append(result, component)
		hashes = append(hashes, provenanceHashes...)
	}
	slices.SortFunc(result, func(left, right simmodel.ComponentEvidence) int {
		return cmp.Compare(left.InstanceID, right.InstanceID)
	})
	return result, hashes, diagnostics
}

func capacitorHarnessModel(analysis string) string {
	switch trustedModelAnalysisKind(analysis) {
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisDistortion, simmodel.AnalysisElectrothermal:
		return simmodel.PrimitiveCapacitorTransientV1
	default:
		return simmodel.PrimitiveCapacitorV1
	}
}

func simulationIntentParts(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	evidence []simmodel.ComponentEvidence,
	quantity string,
	scale float64,
) (simmodel.Analysis, simmodel.Assertion, []SimulationDiagnostic) {
	analysis := simmodel.Analysis{
		ID:          canonicalIdentifier(operatingCase.ID + "_" + assertion.ID),
		Kind:        assertion.Analysis,
		Excitations: simulationExcitations(requirement, assertion, operatingCase, corner, graph),
	}
	switch assertion.Analysis {
	case "dc_sweep":
		analysis.Kind = simmodel.AnalysisDCOperatingPoint
		source, start, stop, found := sweepSourceAndRange(requirement, assertion, operatingCase, corner, graph)
		if !found {
			return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{Code: diagnosisSimulationInvalid, Path: "simulation.analysis.dc_sweep", Message: "DC sweep requires one bounded semantic excitation range"}}
		}
		analysis.DCSweep = &simmodel.DCSweep{Component: source, StartValue: start, StopValue: stop, Points: 101, Bidirectional: true}
	case simmodel.AnalysisACSweep, simmodel.AnalysisNoise:
		center := assertionFrequencyScale(requirement, assertion)
		analysis.StartFrequencyHz = math.Max(center/100, 1e-3)
		analysis.StopFrequencyHz = math.Max(center*100, analysis.StartFrequencyHz*10)
		analysis.Points = 61
	case simmodel.AnalysisStability:
		center := assertionFrequencyScale(requirement, assertion)
		characteristic := simulationCharacteristicFrequency(evidence)
		analysis.StartFrequencyHz = math.Max(center/100, 1e-3)
		analysis.StopFrequencyHz = math.Max(center*100, analysis.StartFrequencyHz*10)
		if characteristic > 0 {
			analysis.StartFrequencyHz = math.Max(
				math.Min(analysis.StartFrequencyHz, characteristic/1e6),
				1e-3,
			)
			analysis.StopFrequencyHz = math.Min(
				math.Max(analysis.StopFrequencyHz, characteristic*100),
				1e12,
			)
		}
		analysis.Points = 64
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisElectrothermal:
		duration := dynamicDuration(assertion, operatingCase)
		analysis.DurationS = duration
		analysis.TimeStepS = duration / 1000
		if assertion.Analysis == simmodel.AnalysisElectrothermal {
			analysis.Conditions = []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: cornerAmbientTemperature(operatingCase, corner)}}
		}
		simmodel.NormalizeDynamicGrid(&analysis)
		addSimulationEvents(&analysis, operatingCase, graph)
	case simmodel.AnalysisDistortion:
		frequency := assertionFrequencyScale(requirement, assertion)
		analysis.DurationS = 4 / frequency
		analysis.TimeStepS = 1 / (frequency * 64)
		for index := range analysis.Excitations {
			if assertion.Excitation != nil &&
				analysis.Excitations[index].Component == sourceInstanceForObservation(graph, *assertion.Excitation) {
				analysis.Excitations[index].SineAmplitude = excitationAmplitude(requirement, *assertion.Excitation)
				analysis.Excitations[index].SineFrequencyHz = frequency
			}
		}
	case simmodel.AnalysisThermal:
		analysis.Conditions = []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: cornerAmbientTemperature(operatingCase, corner)}}
	}
	node := observationNodeID(graph, requirement, assertion.Observation)
	if node == "" && simulationQuantityNeedsNode(quantity) {
		return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{Code: diagnosisSimulationInvalid, Path: "simulation.assertion.observation", Message: "behavioral observation does not resolve to a candidate graph node"}}
	}
	minimum, maximum := scaledAssertionBounds(assertion, scale)
	if quantity == simmodel.QuantityPeakAbsVoltageV {
		minimum = 0
		maximum = 0
		if assertion.Min != nil {
			maximum = math.Max(maximum, math.Abs(*assertion.Min)*scale)
		}
		if assertion.Max != nil {
			maximum = math.Max(maximum, math.Abs(*assertion.Max)*scale)
		}
		if maximum == 0 {
			maximum = 1e12
		}
	}
	simulationAssertion := simmodel.Assertion{
		AnalysisID: analysis.ID,
		Node:       node,
		Quantity:   quantity,
		Min:        minimum,
		Max:        maximum,
	}
	component, components, scopeDiagnostic := simulationMeasurementScope(
		requirement,
		assertion,
		operatingCase,
		graph,
		evidence,
		quantity,
	)
	if scopeDiagnostic != nil {
		return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{*scopeDiagnostic}
	}
	simulationAssertion.Component = component
	simulationAssertion.Components = components
	if assertion.FrequencyHz != nil {
		simulationAssertion.FrequencyHz = *assertion.FrequencyHz
	} else if quantity == simmodel.QuantityVoltageGainRatio {
		simulationAssertion.FrequencyHz = assertionFrequencyScale(requirement, assertion)
	}
	if assertion.Excitation != nil &&
		(quantity == simmodel.QuantityVoltageGainRatio || quantity == simmodel.QuantityCutoffFrequencyHz) {
		simulationAssertion.ReferenceNode = observationNodeID(graph, requirement, *assertion.Excitation)
	}
	for _, event := range operatingCase.Events {
		simulationAssertion.WindowStartS = event.TriggerTimeS
		simulationAssertion.WindowEndS = analysis.DurationS
		break
	}
	_ = evidence
	return analysis, simulationAssertion, nil
}

func simulationCharacteristicFrequency(
	evidence []simmodel.ComponentEvidence,
) float64 {
	result := 0.0
	for _, component := range evidence {
		for _, claim := range component.ModelClaims {
			for _, parameter := range claim.Parameters {
				switch parameter.Name {
				case "gain_bandwidth_hz", "bandwidth_hz", "transition_frequency_hz":
					result = math.Max(result, parameter.Value)
				}
			}
		}
	}
	return result
}

func simulationQuantityNeedsNode(quantity string) bool {
	switch quantity {
	case simmodel.QuantityDeviceCurrentA,
		simmodel.QuantityTotalSupplyCurrentA,
		simmodel.QuantityPeakAbsDeviceCurrentA,
		simmodel.QuantityMaximumJunctionTemperatureC,
		simmodel.QuantityMinimumTransientSOAMargin:
		return false
	default:
		return true
	}
}

func simulationMeasurementScope(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	graph CandidateGraph,
	evidence []simmodel.ComponentEvidence,
	quantity string,
) (string, []string, *SimulationDiagnostic) {
	switch quantity {
	case simmodel.QuantityDeviceCurrentA, simmodel.QuantityDCSweepDeviceSlopeAperV:
		if component, found := observedCurrentComponent(requirement, assertion, operatingCase, graph); found {
			return component, nil, nil
		}
		return "", nil, &SimulationDiagnostic{
			Code:       diagnosisModelUnavailable,
			Path:       "simulation.assertion.component",
			Message:    "current measurement requires a unique catalog-backed load or active current path at the observed port",
			Suggestion: "declare a bounded load condition or remove ambiguous parallel active paths",
		}
	case simmodel.QuantityTotalSupplyCurrentA:
		component, found := dominantSupplySource(requirement, graph)
		if !found {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.components", Message: "supply-current measurement requires an external supply source"}
		}
		return "", []string{component}, nil
	case simmodel.QuantityPeakAbsDeviceCurrentA:
		if assertion.Observation.Kind == "port" {
			if component, found := observedCurrentComponent(requirement, assertion, operatingCase, graph); found {
				return component, nil, nil
			}
		}
		component, found := dominantSupplySource(requirement, graph)
		if !found {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "startup-current measurement requires an external supply source"}
		}
		return component, nil, nil
	case simmodel.QuantityMaximumJunctionTemperatureC:
		components := evidenceComponentsWithThermal(evidence, false)
		if len(components) == 0 {
			return "", nil, &SimulationDiagnostic{
				Code:       diagnosisThermalUnavailable,
				Path:       "simulation.assertion.components",
				Message:    "maximum junction temperature requires at least one complete reviewed thermal path",
				Suggestion: "select active primitives with reviewed thermal RC evidence",
			}
		}
		return "", components, nil
	case simmodel.QuantityMinimumTransientSOAMargin:
		components := evidenceComponentsWithThermal(evidence, true)
		if len(components) == 0 {
			return "", nil, &SimulationDiagnostic{
				Code:       diagnosisThermalUnavailable,
				Path:       "simulation.assertion.components",
				Message:    "minimum SOA margin requires at least one reviewed transient SOA envelope",
				Suggestion: "select active primitives with reviewed transient SOA evidence",
			}
		}
		return "", components, nil
	default:
		return "", nil, nil
	}
}

func observedCurrentComponent(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	graph CandidateGraph,
) (string, bool) {
	if component, found := loadMeasurementComponent(
		requirement,
		assertion,
		operatingCase,
		graph,
	); found {
		return component, true
	}
	observationNode := observationNodeID(graph, requirement, assertion.Observation)
	components := []string{}
	for _, instance := range graph.Instances {
		if !topologyActiveKind(instance.Kind) {
			continue
		}
		for _, terminal := range instance.Terminals {
			if terminal.Node == observationNode {
				components = append(components, instance.ID)
				break
			}
		}
	}
	slices.Sort(components)
	components = slices.Compact(components)
	if len(components) != 1 {
		return "", false
	}
	return components[0], true
}

func loadMeasurementComponent(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	graph CandidateGraph,
) (string, bool) {
	observationNode := observationNodeID(graph, requirement, assertion.Observation)
	for _, condition := range operatingCase.Conditions {
		if condition.Axis != "load_resistance" && condition.Axis != "load_current" && condition.Axis != "load_inductance" {
			continue
		}
		target, found := externalNodeForSemanticTarget(graph, condition.Target)
		if found && target.ID == observationNode {
			return loadInstanceID(condition.Target, condition.Axis), true
		}
	}
	return "", false
}

func dominantSupplySource(requirement Requirement, graph CandidateGraph) (string, bool) {
	node, found := dominantSupplyNode(requirement, graph)
	if !found {
		return "", false
	}
	return sourceInstanceForNode(node), true
}

func dominantSupplyNode(requirement Requirement, graph CandidateGraph) (GraphNode, bool) {
	type supplyCandidate struct {
		domain     string
		maxCurrent float64
		maxVoltage float64
	}
	candidates := []supplyCandidate{}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		maxVoltage := 0.0
		if domain.NominalVoltageV != nil {
			maxVoltage = *domain.NominalVoltageV
		}
		if domain.MaxVoltageV != nil {
			maxVoltage = *domain.MaxVoltageV
		}
		maxCurrent := 0.0
		if domain.MaxCurrentA != nil {
			maxCurrent = *domain.MaxCurrentA
		}
		candidates = append(candidates, supplyCandidate{domain: domain.ID, maxCurrent: maxCurrent, maxVoltage: maxVoltage})
	}
	slices.SortFunc(candidates, func(left, right supplyCandidate) int {
		if order := cmp.Compare(right.maxCurrent, left.maxCurrent); order != 0 {
			return order
		}
		if order := cmp.Compare(right.maxVoltage, left.maxVoltage); order != 0 {
			return order
		}
		return cmp.Compare(left.domain, right.domain)
	})
	for _, candidate := range candidates {
		for _, node := range graph.Nodes {
			if node.Scope == "external" && node.Role == "supply" && node.Domain == candidate.domain {
				return node, true
			}
		}
	}
	return GraphNode{}, false
}

func loadHarnessNodes(
	requirement Requirement,
	graph CandidateGraph,
	target GraphNode,
	reference string,
) (string, string) {
	for _, port := range requirement.Requirements.Ports {
		if port.ID != target.SemanticID ||
			port.Kind != "controlled_current" ||
			port.Direction != "sink" {
			continue
		}
		if supply, found := dominantSupplyNode(requirement, graph); found {
			return supply.ID, target.ID
		}
	}
	return target.ID, reference
}

func derivedInductiveLoadSeriesResistance(
	requirement Requirement,
	target string,
) (float64, bool) {
	maximumCurrent := 0.0
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Target == target &&
				condition.Axis == "load_current" &&
				condition.Max > maximumCurrent {
				maximumCurrent = condition.Max
			}
		}
	}
	if maximumCurrent <= 0 {
		return 0, false
	}
	maximumSupplyCurrent := -1.0
	supplyVoltage := 0.0
	supplyID := ""
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		current := 0.0
		if domain.MaxCurrentA != nil {
			current = *domain.MaxCurrentA
		}
		voltage := 0.0
		switch {
		case domain.NominalVoltageV != nil:
			voltage = *domain.NominalVoltageV
		case domain.MinVoltageV != nil && domain.MaxVoltageV != nil:
			voltage = (*domain.MinVoltageV + *domain.MaxVoltageV) / 2
		case domain.MaxVoltageV != nil:
			voltage = *domain.MaxVoltageV
		}
		if current > maximumSupplyCurrent ||
			(current == maximumSupplyCurrent &&
				(voltage > supplyVoltage ||
					(voltage == supplyVoltage && domain.ID < supplyID))) {
			maximumSupplyCurrent = current
			supplyVoltage = voltage
			supplyID = domain.ID
		}
	}
	if supplyVoltage <= 0 {
		return 0, false
	}
	return supplyVoltage / maximumCurrent, true
}

func evidenceComponentsWithThermal(evidence []simmodel.ComponentEvidence, requireSOA bool) []string {
	result := []string{}
	for _, component := range evidence {
		for _, claim := range component.ModelClaims {
			if (!requireSOA && claim.ThermalModel != nil) || (requireSOA && len(claim.TransientSOA) != 0) {
				result = append(result, component.InstanceID)
				break
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func simulationExcitations(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) []simmodel.SourceExcitation {
	result := []simmodel.SourceExcitation{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" || node.Role == "output" {
			continue
		}
		excitation := simmodel.SourceExcitation{
			Component: sourceInstanceForNode(node),
			DCValue:   sourceValueForNode(requirement, operatingCase, corner, node),
		}
		if assertion.Analysis == simmodel.AnalysisTransient &&
			assertion.Excitation != nil &&
			observationMatchesNode(node, *assertion.Excitation) {
			for _, condition := range operatingCase.Conditions {
				target, found := externalNodeForSemanticTarget(graph, condition.Target)
				if !found || target.ID != node.ID ||
					(condition.Axis != "input_voltage" &&
						condition.Axis != "control_voltage") ||
					condition.Min == condition.Max {
					continue
				}
				duration := dynamicDuration(assertion, operatingCase)
				excitation.DCValue = condition.Min
				excitation.PulseInitialValue = condition.Min
				excitation.PulseValue = condition.Max
				excitation.PulseDelayS = duration / 5
				excitation.PulseWidthS = duration * 3 / 5
				excitation.PulsePeriodS = duration * 2
				break
			}
		}
		if assertion.Analysis == simmodel.AnalysisACSweep && assertion.Excitation != nil &&
			observationMatchesNode(node, *assertion.Excitation) {
			excitation.ACMagnitude = 1
		}
		result = append(result, excitation)
	}
	for _, condition := range operatingCase.Conditions {
		if condition.Axis != "load_current" {
			continue
		}
		result = append(result, simmodel.SourceExcitation{
			Component: loadInstanceID(condition.Target, condition.Axis),
			DCValue:   corner.Values[conditionKey(condition)],
		})
	}
	slices.SortFunc(result, func(left, right simmodel.SourceExcitation) int {
		return cmp.Compare(left.Component, right.Component)
	})
	return result
}

func loadInstanceID(target, axis string) string {
	return canonicalIdentifier("load_" + target + "_" + axis)
}

func selectHarnessRecord(
	environment SimulationEnvironment,
	family string,
	modelID string,
	analysis string,
) (components.ComponentRecord, []string, bool) {
	candidates := []components.ComponentRecord{}
	for _, record := range environment.Catalog.Records {
		if record.Family != family || !acceptedConfidence(record.Verification.Confidence) {
			continue
		}
		for _, claim := range record.SimulationModels {
			if claim.ModelID != modelID {
				continue
			}
			provenance, found := modelprovenance.Lookup(environment.ModelRegistry, record.ID, modelID)
			if !found || provenance.Provenance.ReviewStatus != "reviewed" ||
				!reviewedModelSupportsCircuitAnalysis(provenance.Provenance.AllowedAnalyses, analysis) {
				continue
			}
			candidates = append(candidates, record)
			break
		}
	}
	slices.SortFunc(candidates, func(left, right components.ComponentRecord) int {
		return cmp.Or(
			cmp.Compare(harnessRecordRank(left), harnessRecordRank(right)),
			cmp.Compare(left.ID, right.ID),
		)
	})
	if len(candidates) == 0 {
		return components.ComponentRecord{}, nil, false
	}
	record := candidates[0]
	hashes := []string{}
	for _, claim := range record.SimulationModels {
		provenance, found := modelprovenance.Lookup(environment.ModelRegistry, record.ID, claim.ModelID)
		if found && reviewedModelSupportsCircuitAnalysis(provenance.Provenance.AllowedAnalyses, analysis) {
			hashes = append(hashes, provenance.Provenance.SHA256)
		}
	}
	slices.Sort(hashes)
	return record, slices.Compact(hashes), true
}

func harnessRecordRank(record components.ComponentRecord) int {
	if slices.Contains(record.Tags, "simulation_load") ||
		slices.Contains(record.Tags, "external_load") ||
		slices.Contains(record.Tags, "testbench") {
		return 0
	}
	if record.Generic {
		return 1
	}
	return 2
}

func cloneCatalogClaims(source []simmodel.CatalogEvidence) []simmodel.CatalogEvidence {
	result := make([]simmodel.CatalogEvidence, 0, len(source))
	for _, claim := range source {
		result = append(result, simmodel.CloneCatalogEvidence(claim))
	}
	return result
}

func valueUncertainty(instance GraphInstance, primitive PrimitiveCandidate) []simmodel.Uncertainty {
	if instance.ValueSI == nil || primitive.ValueDomain == nil {
		return nil
	}
	tolerance, proven := primitiveTolerancePercent(primitive, primitive.ValueDomain.Kind)
	if !proven || tolerance <= 0 {
		return nil
	}
	fraction := tolerance / 100
	return []simmodel.Uncertainty{{
		Target:  "value_si",
		Source:  "catalog:" + primitive.CatalogID + ":" + primitive.ValueDomain.Kind + "_tolerance",
		Nominal: *instance.ValueSI,
		Minimum: *instance.ValueSI * (1 - fraction),
		Maximum: *instance.ValueSI * (1 + fraction),
	}}
}

func operatingCaseCorners(operatingCase OperatingCase) []operatingCorner {
	if len(operatingCase.Conditions) == 0 {
		return []operatingCorner{{ID: "nominal", Values: map[string]float64{}}}
	}
	nominal := operatingCorner{ID: "nominal", Values: map[string]float64{}}
	for _, condition := range operatingCase.Conditions {
		nominal.Values[conditionKey(condition)] = (condition.Min + condition.Max) / 2
	}
	result := []operatingCorner{nominal}
	var expand func(int, map[string]float64, string)
	expand = func(index int, values map[string]float64, suffix string) {
		if index == len(operatingCase.Conditions) {
			copyValues := make(map[string]float64, len(values))
			for key, value := range values {
				copyValues[key] = value
			}
			result = append(result, operatingCorner{ID: "bounds_" + suffix, Values: copyValues})
			return
		}
		condition := operatingCase.Conditions[index]
		key := conditionKey(condition)
		values[key] = condition.Min
		expand(index+1, values, suffix+"0")
		if condition.Max != condition.Min {
			values[key] = condition.Max
			expand(index+1, values, suffix+"1")
		}
		delete(values, key)
	}
	expand(0, map[string]float64{}, "")
	slices.SortFunc(result[1:], func(left, right operatingCorner) int {
		return cmp.Compare(left.ID, right.ID)
	})
	result = compactOperatingCorners(result)
	return result
}

func compactOperatingCorners(corners []operatingCorner) []operatingCorner {
	result := make([]operatingCorner, 0, len(corners))
	seen := map[string]bool{}
	for _, corner := range corners {
		data, _ := json.Marshal(corner.Values)
		key := string(data)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, corner)
	}
	return result
}

func conditionKey(condition OperatingCondition) string {
	return condition.Axis + "\x00" + condition.Target
}

func sourceValueForNode(
	requirement Requirement,
	operatingCase OperatingCase,
	corner operatingCorner,
	node GraphNode,
) float64 {
	for _, condition := range operatingCase.Conditions {
		if condition.Axis != "supply_voltage" && condition.Axis != "input_voltage" {
			continue
		}
		if condition.Target == node.SemanticID || condition.Target == node.Domain {
			return corner.Values[conditionKey(condition)]
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID != node.SemanticID {
			continue
		}
		if port.Electrical.NominalVoltageV != nil {
			return *port.Electrical.NominalVoltageV
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == node.Domain && domain.NominalVoltageV != nil {
			return *domain.NominalVoltageV
		}
	}
	return 0
}

func sourceInstanceForNode(node GraphNode) string {
	return canonicalIdentifier("source_" + node.ID)
}

func sourceInstanceForObservation(graph CandidateGraph, observation Observation) string {
	for _, node := range graph.Nodes {
		if observationMatchesNode(node, observation) {
			return sourceInstanceForNode(node)
		}
	}
	return ""
}

func observationMatchesNode(node GraphNode, observation Observation) bool {
	if node.Scope != "external" {
		return false
	}
	if observation.Kind == "port" {
		return node.SemanticID == observation.ID
	}
	return observation.Kind == "domain" && node.Domain == observation.ID && node.Role == "supply"
}

func observationNodeID(graph CandidateGraph, requirement Requirement, observation Observation) string {
	node, found := ExternalNodeForObservation(graph, requirement, observation)
	if found {
		return node
	}
	return ""
}

func referenceNodeForDomain(graph CandidateGraph, observation Observation) string {
	candidates := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "reference" {
			candidates = append(candidates, node.ID)
		}
	}
	slices.Sort(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func externalNodeForSemanticTarget(graph CandidateGraph, target string) (GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.Scope == "external" && (node.SemanticID == target || node.Domain == target) {
			return node, true
		}
	}
	return GraphNode{}, false
}

func assertionFrequencyScale(
	requirement Requirement,
	assertion BehavioralAssertion,
) float64 {
	if assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 {
		return *assertion.FrequencyHz
	}
	value := assertionTarget(assertion)
	if value > 0 && assertion.Unit == "Hz" {
		return value
	}
	if assertion.Metric == "voltage_gain" ||
		assertion.Metric == "phase_margin" ||
		assertion.Metric == "gain_margin" {
		behaviorFrequency := math.Inf(1)
		for _, candidate := range requirement.Requirements.BehavioralRequirements {
			if candidate.Metric != "cutoff_frequency" &&
				candidate.Metric != "bandwidth" {
				continue
			}
			target := assertionTarget(candidate)
			if target > 0 {
				behaviorFrequency = math.Min(behaviorFrequency, target)
			}
		}
		if finite(behaviorFrequency) {
			return math.Max(behaviorFrequency/100, 1e-3)
		}
	}
	return 1000
}

func excitationAmplitude(requirement Requirement, observation Observation) float64 {
	if observation.Kind == "port" {
		for _, port := range requirement.Requirements.Ports {
			if port.ID != observation.ID {
				continue
			}
			if port.Electrical.MaxVoltageV != nil && port.Electrical.MinVoltageV != nil {
				amplitude := (*port.Electrical.MaxVoltageV - *port.Electrical.MinVoltageV) / 4
				if amplitude > 0 {
					return amplitude
				}
			}
		}
	}
	return 1
}

func dynamicDuration(assertion BehavioralAssertion, operatingCase OperatingCase) float64 {
	duration := 0.0
	switch assertion.Metric {
	case "settling_time", "propagation_delay", "rise_time", "fall_time":
		duration = assertionTarget(assertion) * 10
	}
	for _, event := range operatingCase.Events {
		duration = math.Max(duration, event.TriggerTimeS*2)
	}
	if duration <= 0 {
		duration = 0.01
	}
	return math.Min(duration, 10)
}

func addSimulationEvents(analysis *simmodel.Analysis, operatingCase OperatingCase, graph CandidateGraph) {
	for _, event := range operatingCase.Events {
		target, found := externalNodeForSemanticTarget(graph, event.Target)
		if !found {
			continue
		}
		analysis.SourceValueEvents = append(analysis.SourceValueEvents, simmodel.SourceValueEvent{
			ID:           canonicalIdentifier(event.ID),
			Component:    sourceInstanceForNode(target),
			TriggerTimeS: event.TriggerTimeS,
			DurationS:    math.Max(analysis.DurationS-event.TriggerTimeS, analysis.TimeStepS),
			Initial:      event.Initial,
			Applied:      event.Applied,
		})
	}
}

func sweepSourceAndRange(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) (string, float64, float64, bool) {
	if assertion.Excitation != nil {
		source := sourceInstanceForObservation(graph, *assertion.Excitation)
		excitationTarget := ""
		excitationAxis := ""
		for _, condition := range operatingCase.Conditions {
			target, found := externalNodeForSemanticTarget(graph, condition.Target)
			if found && observationMatchesNode(target, *assertion.Excitation) &&
				(condition.Axis == "input_voltage" || condition.Axis == "supply_voltage") {
				if condition.Max > condition.Min {
					return source, condition.Min, condition.Max, true
				}
				excitationTarget = condition.Target
				excitationAxis = condition.Axis
				break
			}
		}
		if excitationTarget != "" {
			for _, port := range requirement.Requirements.Ports {
				if port.ID != excitationTarget ||
					port.Electrical.MinVoltageV == nil ||
					port.Electrical.MaxVoltageV == nil ||
					*port.Electrical.MaxVoltageV <= *port.Electrical.MinVoltageV {
					continue
				}
				const localSweepFraction = .1
				envelopeMinimum := *port.Electrical.MinVoltageV
				envelopeMaximum := *port.Electrical.MaxVoltageV
				width := (envelopeMaximum - envelopeMinimum) * localSweepFraction
				center := corner.Values[conditionKey(OperatingCondition{
					Axis:   excitationAxis,
					Target: excitationTarget,
				})]
				start := center - width/2
				stop := center + width/2
				if start < envelopeMinimum {
					stop += envelopeMinimum - start
					start = envelopeMinimum
				}
				if stop > envelopeMaximum {
					start -= stop - envelopeMaximum
					stop = envelopeMaximum
				}
				start = math.Max(start, envelopeMinimum)
				if stop > start {
					return source, start, stop, true
				}
			}
		}
	}
	if assertion.Metric == "load_regulation" {
		minimum, maximum := math.Inf(1), math.Inf(-1)
		targetID := ""
		for _, caseID := range assertion.OperatingCases {
			for _, candidateCase := range requirement.Requirements.OperatingCases {
				if candidateCase.ID != caseID {
					continue
				}
				for _, condition := range candidateCase.Conditions {
					if condition.Axis != "load_current" {
						continue
					}
					target, found := externalNodeForSemanticTarget(graph, condition.Target)
					if !found || target.ID != observationNodeID(graph, requirement, assertion.Observation) {
						continue
					}
					if targetID != "" && targetID != condition.Target {
						return "", 0, 0, false
					}
					targetID = condition.Target
					minimum = math.Min(minimum, condition.Min)
					maximum = math.Max(maximum, condition.Max)
				}
			}
		}
		if targetID != "" && maximum > minimum {
			return loadInstanceID(targetID, "load_current"), minimum, maximum, true
		}
	}
	return "", 0, 0, false
}

func cornerAmbientTemperature(operatingCase OperatingCase, corner operatingCorner) float64 {
	for _, condition := range operatingCase.Conditions {
		if condition.Axis == "ambient_temperature" || condition.Axis == "temperature" {
			return corner.Values[conditionKey(condition)]
		}
	}
	return 25
}

func scaledAssertionBounds(assertion BehavioralAssertion, scale float64) (float64, float64) {
	minimum := -1e12
	maximum := 1e12
	if assertion.Min != nil {
		minimum = *assertion.Min * scale
	}
	if assertion.Max != nil {
		maximum = *assertion.Max * scale
	}
	if scale == 100 && assertion.Min == nil {
		minimum = 0
	}
	return minimum, maximum
}

func assertionValuePasses(assertion BehavioralAssertion, actual float64) bool {
	if assertion.Min != nil && actual < *assertion.Min {
		return false
	}
	if assertion.Max != nil && actual > *assertion.Max {
		return false
	}
	return finite(actual)
}

func simulationDiagnosis(
	code string,
	assertion BehavioralAssertion,
	operatingCase string,
	actual *float64,
	coneHash string,
	evidenceHash string,
	message string,
) Diagnosis {
	return Diagnosis{
		Code:             code,
		RequirementID:    assertion.ID,
		OperatingCase:    operatingCase,
		Analysis:         assertion.Analysis,
		Metric:           assertion.Metric,
		Actual:           cloneInventoryFloat(actual),
		RequiredMin:      cloneInventoryFloat(assertion.Min),
		RequiredMax:      cloneInventoryFloat(assertion.Max),
		AffectedConeHash: coneHash,
		EvidenceHash:     evidenceHash,
		Message:          message,
	}
}

func diagnosisFromSimulationDiagnostics(
	assertion BehavioralAssertion,
	operatingCase string,
	graph CandidateGraph,
	diagnostics []SimulationDiagnostic,
) Diagnosis {
	code := diagnosisSimulationInvalid
	message := "trusted simulation failed"
	if len(diagnostics) != 0 {
		code = diagnostics[0].Code
		message = diagnostics[0].Message
	}
	return simulationDiagnosis(
		code,
		assertion,
		operatingCase,
		nil,
		graphConeHash(graph, ""),
		hashJSON(diagnostics),
		message,
	)
}

func normalizeSimModelDiagnostics(source []simmodel.Diagnostic) []SimulationDiagnostic {
	result := make([]SimulationDiagnostic, 0, len(source))
	for _, diagnostic := range source {
		result = append(result, SimulationDiagnostic{
			Code:       classifySimulationDiagnostic(diagnostic.Message),
			Path:       diagnostic.Path,
			Message:    diagnostic.Message,
			Suggestion: diagnostic.Suggestion,
		})
	}
	slices.SortFunc(result, func(left, right SimulationDiagnostic) int {
		return cmp.Or(
			cmp.Compare(left.Code, right.Code),
			cmp.Compare(left.Path, right.Path),
			cmp.Compare(left.Message, right.Message),
		)
	})
	return result
}

func classifySimulationDiagnostic(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "thermal") && (strings.Contains(lower, "missing") || strings.Contains(lower, "requires")):
		return diagnosisThermalUnavailable
	case strings.Contains(lower, "no trusted") || strings.Contains(lower, "no reviewed") || strings.Contains(lower, "primitive claim"):
		return diagnosisModelUnavailable
	case strings.Contains(lower, "singular") || strings.Contains(lower, "floating"):
		return diagnosisOperatingPointInvalid
	case strings.Contains(lower, "converg"):
		return diagnosisNonconvergent
	case strings.Contains(lower, "unstable"):
		return diagnosisUnstable
	default:
		return diagnosisSimulationInvalid
	}
}

func compareDiagnoses(left, right Diagnosis) int {
	return cmp.Or(
		cmp.Compare(left.RequirementID, right.RequirementID),
		cmp.Compare(left.OperatingCase, right.OperatingCase),
		cmp.Compare(left.Code, right.Code),
		cmp.Compare(left.Message, right.Message),
	)
}

func graphConeHash(graph CandidateGraph, nodeID string) string {
	if nodeID == "" {
		hash, _ := GraphHash(graph)
		return hash
	}
	adjacency := map[string][]string{}
	for _, instance := range graph.Instances {
		instanceVertex := "instance:" + instance.ID
		for _, terminal := range instance.Terminals {
			nodeVertex := "node:" + terminal.Node
			adjacency[instanceVertex] = append(adjacency[instanceVertex], nodeVertex)
			adjacency[nodeVertex] = append(adjacency[nodeVertex], instanceVertex)
		}
	}
	reached := map[string]bool{}
	markReachableVertices(adjacency, "node:"+nodeID, reached)
	cone := CandidateGraph{Schema: CandidateGraphSchema, Version: CandidateGraphVersion}
	for _, node := range graph.Nodes {
		if reached["node:"+node.ID] {
			cone.Nodes = append(cone.Nodes, node)
		}
	}
	for _, instance := range graph.Instances {
		if reached["instance:"+instance.ID] {
			cone.Instances = append(cone.Instances, instance)
		}
	}
	hash, _ := GraphHash(cone)
	return hash
}

func normalizeSimulationUncertainties(source []simmodel.Uncertainty) []simmodel.Uncertainty {
	slices.SortFunc(source, func(left, right simmodel.Uncertainty) int {
		return cmp.Or(
			cmp.Compare(left.Target, right.Target),
			cmp.Compare(left.Source, right.Source),
			cmp.Compare(left.Nominal, right.Nominal),
		)
	})
	return slices.CompactFunc(source, func(left, right simmodel.Uncertainty) bool {
		return left.Target == right.Target && left.Source == right.Source &&
			left.Nominal == right.Nominal && left.Minimum == right.Minimum && left.Maximum == right.Maximum
	})
}

func cloneThermalModel(source *simmodel.ThermalRCNetwork) *simmodel.ThermalRCNetwork {
	if source == nil {
		return nil
	}
	claim := simmodel.CloneCatalogEvidence(simmodel.CatalogEvidence{ThermalModel: source})
	return claim.ThermalModel
}

func cloneTransientSOAEnvelopes(source []simmodel.TransientSOAEnvelope) []simmodel.TransientSOAEnvelope {
	claim := simmodel.CloneCatalogEvidence(simmodel.CatalogEvidence{TransientSOA: source})
	return claim.TransientSOA
}

func finalizeSimulationEvaluation(result SimulationEvaluation) SimulationEvaluation {
	result.Hash = ""
	data, _ := json.Marshal(result)
	sum := sha256.Sum256(data)
	result.Hash = hex.EncodeToString(sum[:])
	return result
}

func hashJSON(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
