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
	maximumHarnessResistanceOhm    = 1e12
	maximumDynamicTimeSteps        = 1_000_000
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
			operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
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
			attempt, diagnoses := evaluateAssertionCorner(
				requirement,
				work.assertion,
				work.operatingCase,
				work.corner,
				graph,
				inventory,
				environment,
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

// electrothermalPeriodicBehavior carries a declared periodic operating signal
// into a circuit-level electrothermal assertion. Thermal and SOA assertions
// intentionally describe the circuit rather than an input/output transfer, so
// they commonly omit an excitation of their own. Reusing a behavior-only
// periodic transfer at the same observation preserves the declared stimulus
// across thermal corners without inventing a waveform or frequency.
func electrothermalPeriodicBehavior(requirement Requirement, assertion BehavioralAssertion) BehavioralAssertion {
	if assertion.Analysis != simmodel.AnalysisElectrothermal ||
		(assertion.Excitation != nil && assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0) {
		return assertion
	}
	type periodicBehavior struct {
		id         string
		excitation Observation
		frequency  float64
	}
	candidates := []periodicBehavior{}
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Excitation == nil || candidate.FrequencyHz == nil || *candidate.FrequencyHz <= 0 ||
			(assertion.Observation.Kind != "circuit" && candidate.Observation != assertion.Observation) {
			continue
		}
		switch candidate.Analysis {
		case simmodel.AnalysisTransient, simmodel.AnalysisDistortion, simmodel.AnalysisElectrothermal:
			candidates = append(candidates, periodicBehavior{
				id:         candidate.ID,
				excitation: *candidate.Excitation,
				frequency:  *candidate.FrequencyHz,
			})
		}
	}
	slices.SortFunc(candidates, func(left, right periodicBehavior) int {
		return cmp.Or(
			cmp.Compare(left.excitation.Kind, right.excitation.Kind),
			cmp.Compare(left.excitation.ID, right.excitation.ID),
			cmp.Compare(left.frequency, right.frequency),
			cmp.Compare(left.id, right.id),
		)
	})
	unique := []periodicBehavior{}
	for _, candidate := range candidates {
		if len(unique) != 0 &&
			unique[len(unique)-1].excitation == candidate.excitation &&
			unique[len(unique)-1].frequency == candidate.frequency {
			continue
		}
		unique = append(unique, candidate)
	}
	if len(unique) != 1 {
		return assertion
	}
	result := assertion
	result.Excitation = &unique[0].excitation
	result.FrequencyHz = &unique[0].frequency
	return result
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
	return directSimulationQuantityForRequirement(Requirement{}, assertion)
}

func directSimulationQuantityForRequirement(requirement Requirement, assertion BehavioralAssertion) (string, float64, bool) {
	switch assertion.Metric {
	case "output_voltage", "output_high_voltage", "output_low_voltage", "on_state_voltage":
		return simmodel.QuantityVoltageV, 1, true
	case "voltage_gain", "voltage_gain_at_frequency":
		if assertion.Analysis == "dc_sweep" {
			return simmodel.QuantityDCSweepVoltageSlopeVPerV, 1, true
		}
		return simmodel.QuantityVoltageGainRatio, 1, true
	case "bandwidth":
		return simmodel.QuantityBandwidthHz, 1, true
	case "cutoff_frequency":
		return simmodel.QuantityCutoffFrequencyHz, 1, true
	case "phase_margin":
		return simmodel.QuantityPhaseMarginDeg, 1, true
	case "settling_time":
		return simmodel.QuantitySettlingTimeS, 1, true
	case "propagation_delay":
		return simmodel.QuantityResponseTimeS, 1, true
	case "rise_time":
		return simmodel.QuantityRiseTimeS, 1, true
	case "fall_time":
		return simmodel.QuantityFallTimeS, 1, true
	case "output_noise_rms":
		return simmodel.QuantityIntegratedNoiseVRMS, 1, true
	case "thd":
		return simmodel.QuantityTHDPercent, 100, true
	case "total_harmonic_distortion":
		return simmodel.QuantityTHDPercent, 1, true
	case "rising_threshold":
		if envelope, required := topologyWindowBehaviorEnvelope(requirement); required && envelope.signed &&
			assertion.Excitation != nil && assertion.Excitation.ID == envelope.input &&
			assertion.Observation.ID == envelope.output {
			return simmodel.QuantityUpperThresholdVoltageV, 1, true
		}
		return simmodel.QuantityRisingThresholdVoltageV, 1, true
	case "falling_threshold":
		if envelope, required := topologyWindowBehaviorEnvelope(requirement); required && envelope.signed &&
			assertion.Excitation != nil && assertion.Excitation.ID == envelope.input &&
			assertion.Observation.ID == envelope.output {
			return simmodel.QuantityLowerThresholdVoltageV, 1, true
		}
		return simmodel.QuantityFallingThresholdVoltageV, 1, true
	case "lower_threshold":
		return simmodel.QuantityLowerThresholdVoltageV, 1, true
	case "upper_threshold":
		return simmodel.QuantityUpperThresholdVoltageV, 1, true
	case "hysteresis":
		return simmodel.QuantityHysteresisVoltageV, 1, true
	case "startup_output_voltage", "peak_voltage":
		return simmodel.QuantityPeakAbsVoltageV, 1, true
	case "output_swing":
		return simmodel.QuantityOutputSwingVPP, 1, true
	case "oscillation_frequency":
		return simmodel.QuantityOscillationFrequencyHz, 1, true
	case "duty_cycle":
		return simmodel.QuantityDutyCyclePct, 1, true
	case "output_ripple":
		return simmodel.QuantityOutputRippleVPP, 1, true
	case "conversion_efficiency":
		return simmodel.QuantityConversionEfficiencyPct, 1, true
	case "output_power":
		return simmodel.QuantityOutputPowerW, 1, true
	case "startup_overshoot":
		return simmodel.QuantityOvershootVoltageV, 1, true
	case "output_current":
		if assertion.Analysis == simmodel.AnalysisTransient ||
			assertion.Analysis == simmodel.AnalysisStartup ||
			assertion.Analysis == simmodel.AnalysisElectrothermal {
			return simmodel.QuantityFinalAbsDeviceCurrentA, 1, true
		}
		return simmodel.QuantityDeviceCurrentA, 1, true
	case "peak_current":
		return simmodel.QuantityPeakAbsDeviceCurrentA, 1, true
	case "off_state_current":
		if assertion.Analysis == simmodel.AnalysisTransient ||
			assertion.Analysis == simmodel.AnalysisStartup ||
			assertion.Analysis == simmodel.AnalysisElectrothermal {
			return simmodel.QuantityPeakAbsDeviceCurrentA, 1, true
		}
		if assertion.Observation.Kind == "port" {
			return simmodel.QuantityDeviceCurrentA, 1, true
		}
		return simmodel.QuantityTotalSupplyCurrentA, 1, true
	case "startup_current":
		return simmodel.QuantityPeakAbsDeviceCurrentA, 1, true
	case "quiescent_current":
		return simmodel.QuantityTotalSupplyCurrentA, 1, true
	case "transconductance":
		return simmodel.QuantityDCSweepDeviceSlopeAperV, 1, true
	case "transimpedance":
		return simmodel.QuantityTransimpedanceOhm, 1, true
	case "input_impedance":
		return simmodel.QuantityInputImpedanceOhm, 1, true
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
			if !reviewedPrimitiveModelSupportsCircuitAnalysis(model, modelAnalysis) {
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
	conditions := simulationHarnessConditions(requirement, assertion, operatingCase)
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" || node.Role == "output" {
			continue
		}
		instanceID := canonicalIdentifier("source_" + node.ID)
		family, modelID := "voltage_source", simmodel.PrimitiveVoltageSourceV1
		firstTerminal, firstNode := "POSITIVE", node.ID
		secondTerminal, secondNode := "NEGATIVE", reference
		for _, port := range requirement.Requirements.Ports {
			if port.ID != node.SemanticID || port.Kind != "analog_current" {
				continue
			}
			family, modelID = "current_source", simmodel.PrimitiveCurrentSourceV1
			if port.Direction == "sink" {
				firstNode, secondNode = reference, node.ID
			}
			break
		}
		record, provenanceHashes, ok := selectHarnessRecord(
			environment,
			family,
			modelID,
			trustedModelAnalysisKind(assertion.Analysis),
		)
		if !ok {
			diagnostics = append(diagnostics, SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.harness." + instanceID, Message: "reviewed " + family + " harness primitive is unavailable"})
			continue
		}
		result = append(result, simmodel.ComponentEvidence{
			InstanceID:  instanceID,
			CatalogID:   record.ID,
			Family:      record.Family,
			ModelClaims: cloneCatalogClaims(record.SimulationModels),
			Connections: []simmodel.ConnectionEvidence{
				{Function: firstTerminal, Net: firstNode},
				{Function: secondTerminal, Net: secondNode},
			},
		})
		hashes = append(hashes, provenanceHashes...)
	}
	for _, condition := range conditions {
		if simulationExcludesLoadCurrent(assertion) && condition.Axis == "load_current" {
			continue
		}
		target, found := externalNodeForSemanticTarget(graph, condition.Target)
		if !found {
			continue
		}
		value := simulationConditionValue(corner, condition)
		var family, modelID, firstTerminal, secondTerminal string
		resistiveCurrentLoad := false
		switch condition.Axis {
		case "load_resistance":
			if _, paired := operatingCaseInductiveSeriesResistance(conditions, corner, condition.Target); paired {
				continue
			}
			family, modelID, firstTerminal, secondTerminal = "resistor", simmodel.PrimitiveResistorV1, "A", "B"
		case "load_capacitance":
			if value <= 0 {
				continue
			}
			family, modelID, firstTerminal, secondTerminal = "capacitor", capacitorHarnessModel(assertion.Analysis), "A", "B"
		case "load_inductance":
			if value <= 0 {
				continue
			}
			family, modelID, firstTerminal, secondTerminal = "inductor", simmodel.PrimitiveInductorTransientV1, "A", "B"
		case "load_current":
			if resistance, found := dynamicVoltageOutputLoadResistance(requirement, assertion, condition, value); found {
				family, modelID, firstTerminal, secondTerminal = "resistor", simmodel.PrimitiveResistorV1, "A", "B"
				value, resistiveCurrentLoad = resistance, true
			} else {
				family, modelID, firstTerminal, secondTerminal = "current_source", simmodel.PrimitiveCurrentSourceV1, "POSITIVE", "NEGATIVE"
			}
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
			seriesResistance, found := operatingCaseInductiveSeriesResistance(conditions, corner, condition.Target)
			if !found {
				seriesResistance, found = derivedInductiveLoadSeriesResistance(requirement, condition.Target)
			}
			if found {
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
		if condition.Axis != "load_current" || resistiveCurrentLoad {
			component.ValueSI = value
			component.HasValueSI = true
		}
		result = append(result, component)
		hashes = append(hashes, provenanceHashes...)
	}
	shortTargets := map[string]bool{}
	for _, event := range operatingCase.Events {
		if event.Kind != "short_circuit" || shortTargets[event.Target] {
			continue
		}
		target, found := externalNodeForSemanticTarget(graph, event.Target)
		_, resistanceFound := protectedShortResistance(requirement, event)
		if !found || !resistanceFound {
			continue
		}
		record, provenanceHashes, ok := selectHarnessRecord(
			environment, "resistor", simmodel.PrimitiveResistorV1, trustedModelAnalysisKind(assertion.Analysis),
		)
		if !ok {
			diagnostics = append(diagnostics, SimulationDiagnostic{
				Code: diagnosisModelUnavailable, Path: "simulation.harness." + shortLoadInstanceID(event.Target),
				Message: "reviewed resistor harness primitive is unavailable for short-circuit testing",
			})
			continue
		}
		firstNode, secondNode := loadHarnessNodes(requirement, graph, target, reference)
		result = append(result, simmodel.ComponentEvidence{
			InstanceID: shortLoadInstanceID(event.Target), CatalogID: record.ID, Family: record.Family,
			ValueSI: 1e12, HasValueSI: true, ModelClaims: cloneCatalogClaims(record.SimulationModels),
			Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: firstNode}, {Function: "B", Net: secondNode}},
		})
		hashes = append(hashes, provenanceHashes...)
		shortTargets[event.Target] = true
	}
	slices.SortFunc(result, func(left, right simmodel.ComponentEvidence) int {
		return cmp.Compare(left.InstanceID, right.InstanceID)
	})
	return result, hashes, diagnostics
}

func operatingCaseInductiveSeriesResistance(
	conditions []OperatingCondition,
	corner operatingCorner,
	target string,
) (float64, bool) {
	hasInductance := false
	resistance := 0.0
	for _, condition := range conditions {
		if condition.Target != target {
			continue
		}
		value := simulationConditionValue(corner, condition)
		switch condition.Axis {
		case "load_inductance":
			hasInductance = value > 0
		case "load_resistance":
			if value > 0 {
				resistance = value
			}
		}
	}
	return resistance, hasInductance && resistance > 0 && finite(resistance)
}

func simulationHarnessConditions(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
) []OperatingCondition {
	result := append([]OperatingCondition(nil), operatingCase.Conditions...)
	result = signedWindowStaticHarnessConditions(requirement, assertion, result)
	seen := map[string]bool{}
	for _, condition := range result {
		seen[conditionKey(condition)] = true
	}
	loadTargets := map[string]bool{}
	controlTargets := map[string]bool{}
	excitationTargets := map[string]bool{}
	if assertion.Observation.Kind == "port" {
		loadTargets[assertion.Observation.ID] = true
	} else if assertion.Observation.Kind == "circuit" {
		for _, candidateCase := range requirement.Requirements.OperatingCases {
			for _, condition := range candidateCase.Conditions {
				if slices.Contains([]string{"load_capacitance", "load_current", "load_inductance", "load_resistance"}, condition.Axis) {
					loadTargets[condition.Target] = true
				}
			}
		}
	}
	if len(loadTargets) == 0 {
		loadTargets = nil
	}
	if assertion.Excitation != nil && assertion.Excitation.Kind == "port" {
		excitationTargets[assertion.Excitation.ID] = true
	}
	if simulationAssertionRequiresActiveControl(requirement, assertion) {
		for _, port := range requirement.Requirements.Ports {
			if port.Direction == "sink" && (port.Kind == "digital" || port.Kind == "control") {
				controlTargets[port.ID] = true
			}
		}
	}
	if len(loadTargets) == 0 && len(controlTargets) == 0 && len(excitationTargets) == 0 {
		return result
	}
	loadMagnitudeAxis := map[string]string{}
	ambiguousLoadMagnitude := map[string]bool{}
	recordLoadMagnitude := func(condition OperatingCondition) {
		if condition.Axis != "load_current" && condition.Axis != "load_resistance" {
			return
		}
		if previous := loadMagnitudeAxis[condition.Target]; previous != "" && previous != condition.Axis {
			ambiguousLoadMagnitude[condition.Target] = true
			return
		}
		loadMagnitudeAxis[condition.Target] = condition.Axis
	}
	for _, condition := range result {
		recordLoadMagnitude(condition)
	}
	for target := range loadTargets {
		if loadMagnitudeAxis[target] != "" || ambiguousLoadMagnitude[target] {
			continue
		}
		for _, candidateCase := range requirement.Requirements.OperatingCases {
			for _, condition := range candidateCase.Conditions {
				if condition.Target == target {
					recordLoadMagnitude(condition)
				}
			}
		}
	}
	for _, candidateCase := range requirement.Requirements.OperatingCases {
		for _, condition := range candidateCase.Conditions {
			loadCondition := loadTargets[condition.Target] &&
				slices.Contains([]string{"load_capacitance", "load_current", "load_inductance", "load_resistance"}, condition.Axis)
			if loadCondition && (condition.Axis == "load_current" || condition.Axis == "load_resistance") &&
				(ambiguousLoadMagnitude[condition.Target] || loadMagnitudeAxis[condition.Target] != condition.Axis) {
				continue
			}
			controlCondition := controlTargets[condition.Target] &&
				slices.Contains([]string{"control_voltage", "input_voltage"}, condition.Axis)
			excitationCondition := excitationTargets[condition.Target] &&
				slices.Contains([]string{"control_voltage", "input_voltage", "supply_voltage"}, condition.Axis)
			if (!loadCondition && !controlCondition && !excitationCondition) ||
				seen[conditionKey(condition)] {
				continue
			}
			result = append(result, condition)
			seen[conditionKey(condition)] = true
		}
	}
	if forced, found := onStateCurrentHarnessCondition(requirement, assertion); found {
		filtered := result[:0]
		for _, condition := range result {
			if condition.Target == forced.Target &&
				slices.Contains([]string{"load_capacitance", "load_current", "load_inductance", "load_resistance"}, condition.Axis) {
				continue
			}
			filtered = append(filtered, condition)
		}
		result = append(filtered, forced)
	}
	slices.SortFunc(result, func(left, right OperatingCondition) int {
		return cmp.Or(cmp.Compare(left.Axis, right.Axis), cmp.Compare(left.Target, right.Target))
	})
	return result
}

// signedWindowStaticHarnessConditions separates the two static output states
// encoded by a default-low bipolar window. The high-voltage assertion is
// evaluated beyond the positive boundary, while the low-voltage assertion is
// evaluated between the two boundaries. The paired threshold sweeps retain
// independent evidence for both signed transitions.
func signedWindowStaticHarnessConditions(
	requirement Requirement,
	assertion BehavioralAssertion,
	conditions []OperatingCondition,
) []OperatingCondition {
	envelope, required := topologyWindowBehaviorEnvelope(requirement)
	if !required || !envelope.signed || assertion.Observation.Kind != "port" ||
		assertion.Observation.ID != envelope.output ||
		(assertion.Metric != "output_high_voltage" && assertion.Metric != "output_low_voltage") {
		return conditions
	}
	result := append([]OperatingCondition(nil), conditions...)
	for index := range result {
		condition := &result[index]
		if condition.Axis != "input_voltage" || condition.Target != envelope.input ||
			condition.Min >= condition.Max {
			continue
		}
		value := (envelope.lowerV + envelope.upperV) / 2
		if assertion.Metric == "output_high_voltage" {
			value = condition.Max
		}
		condition.Min, condition.Max = value, value
	}
	return result
}

// onStateCurrentHarnessCondition separates a switch's conductive-state drop
// from its switching regulator's average-current behavior. For a controlled
// current port, the midpoint of the declared bounded current requirement is a
// deterministic DC test current that remains below the controller's upper
// regulation threshold while still exercising the power path.
func onStateCurrentHarnessCondition(
	requirement Requirement,
	assertion BehavioralAssertion,
) (OperatingCondition, bool) {
	if assertion.Metric != "on_state_voltage" || assertion.Analysis != simmodel.AnalysisDCOperatingPoint ||
		assertion.Observation.Kind != "port" {
		return OperatingCondition{}, false
	}
	controlled := false
	for _, port := range requirement.Requirements.Ports {
		if port.ID == assertion.Observation.ID && port.Kind == "controlled_current" {
			controlled = true
			break
		}
	}
	if !controlled {
		return OperatingCondition{}, false
	}
	minimum, maximum := math.Inf(-1), math.Inf(1)
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Observation != assertion.Observation ||
			!slices.Contains([]string{"output_current", "peak_current"}, candidate.Metric) {
			continue
		}
		if candidate.Min != nil && finite(*candidate.Min) {
			minimum = math.Max(minimum, *candidate.Min)
		}
		if candidate.Max != nil && finite(*candidate.Max) {
			maximum = math.Min(maximum, *candidate.Max)
		}
	}
	if !finite(minimum) || !finite(maximum) || minimum <= 0 || maximum < minimum {
		return OperatingCondition{}, false
	}
	testCurrent := (minimum + maximum) / 2
	return OperatingCondition{
		Axis: "load_current", Target: assertion.Observation.ID,
		Min: testCurrent, Max: testCurrent, Unit: "A",
	}, true
}

func simulationAssertionRequiresActiveControl(requirement Requirement, assertion BehavioralAssertion) bool {
	if assertion.Analysis == simmodel.AnalysisStartup {
		return false
	}
	switch assertion.Metric {
	case "off_state_current", "output_low_voltage", "quiescent_current", "startup_current":
		return false
	}
	if assertion.Observation.Kind == "circuit" {
		return true
	}
	if assertion.Observation.Kind != "port" {
		return false
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == assertion.Observation.ID {
			return port.Direction == "source"
		}
	}
	return false
}

// requirementActiveControlValue derives the asserted state of a control from
// behavior rather than its name. An off/on-state assertion or a startup
// assertion identifies the controlling excitation; a rising event on that
// excitation then supplies the active state for related steady-state output
// and circuit assertions.
func requirementActiveControlValue(requirement Requirement, controlID string) (float64, bool) {
	activationControl := false
	active := math.Inf(-1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Excitation == nil || assertion.Excitation.Kind != "port" ||
			assertion.Excitation.ID != controlID {
			continue
		}
		switch {
		case assertion.Analysis == simmodel.AnalysisStartup:
			activationControl = true
		case assertion.Metric == "off_state_current" || assertion.Metric == "on_state_voltage":
			activationControl = true
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, event := range operatingCase.Events {
			if event.Target == controlID && event.Kind == "input_step" && event.Applied > event.Initial {
				activationControl = true
				active = math.Max(active, event.Applied)
			}
		}
	}
	if !activationControl {
		return 0, false
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Target == controlID &&
				(condition.Axis == "input_voltage" || condition.Axis == "control_voltage") {
				active = math.Max(active, condition.Max)
			}
		}
	}
	return active, finite(active)
}

func simulationConditionValue(corner operatingCorner, condition OperatingCondition) float64 {
	if value, found := corner.Values[conditionKey(condition)]; found {
		return value
	}
	return (condition.Min + condition.Max) / 2
}

func capacitorHarnessModel(analysis string) string {
	switch trustedModelAnalysisKind(analysis) {
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisDistortion, simmodel.AnalysisElectrothermal:
		return simmodel.PrimitiveCapacitorTransientV1
	default:
		return simmodel.PrimitiveCapacitorV1
	}
}

func dynamicVoltageOutputLoadResistance(
	requirement Requirement,
	assertion BehavioralAssertion,
	condition OperatingCondition,
	currentA float64,
) (float64, bool) {
	if condition.Axis != "load_current" || currentA == 0 ||
		(assertion.Analysis == "dc_sweep" && assertion.Metric == "load_regulation") {
		return 0, false
	}
	switch assertion.Analysis {
	case "dc_sweep", simmodel.AnalysisDCOperatingPoint:
	case simmodel.AnalysisTransient, simmodel.AnalysisStartup, simmodel.AnalysisElectrothermal,
		simmodel.AnalysisThermal, simmodel.AnalysisStability, simmodel.AnalysisACSweep, simmodel.AnalysisNoise:
	default:
		return 0, false
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID != condition.Target || port.Kind != "power" || port.Direction != "source" {
			continue
		}
		voltageV := 0.0
		if port.Electrical.NominalVoltageV != nil {
			voltageV = math.Abs(*port.Electrical.NominalVoltageV)
		} else if port.Electrical.MinVoltageV != nil && port.Electrical.MaxVoltageV != nil {
			voltageV = math.Abs((*port.Electrical.MinVoltageV + *port.Electrical.MaxVoltageV) / 2)
		}
		resistance := math.Min(voltageV/math.Abs(currentA), maximumHarnessResistanceOhm)
		return resistance, resistance > 0 && finite(resistance)
	}
	return 0, false
}

// simulationThermalBoundary closes reviewed junction-to-case device models
// with a catalog-backed non-electrical assembly. The boundary temperature is
// derived conservatively from the requested sine-output operating point,
// standing current, worst active rail magnitude, and the selected natural-
// convection case-to-ambient path. No implicit ambient-equals-case shortcut is
// permitted.
func simulationThermalBoundary(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	evidence []simmodel.ComponentEvidence,
	catalog *components.Catalog,
) ([]simmodel.NamedValue, []string, []SimulationDiagnostic) {
	if assertion.Analysis != simmodel.AnalysisThermal &&
		assertion.Analysis != simmodel.AnalysisElectrothermal {
		return nil, nil, nil
	}
	thermalCase := operatingCase
	thermalCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
	ambient := cornerAmbientTemperature(thermalCase, corner)
	conditions := []simmodel.NamedValue{{Name: "ambient_temperature_c", Value: ambient}}
	junctionToCaseInstances := map[string]bool{}
	for _, component := range evidence {
		for _, claim := range component.ModelClaims {
			if claim.ThermalModel != nil && claim.ThermalModel.Reference == "junction_to_case" {
				junctionToCaseInstances[component.InstanceID] = true
				break
			}
		}
	}
	if len(junctionToCaseInstances) == 0 {
		return conditions, nil, nil
	}
	if catalog == nil {
		return nil, nil, []SimulationDiagnostic{{
			Code:       diagnosisThermalUnavailable,
			Path:       "simulation.thermal_boundary",
			Message:    "junction-to-case device evidence requires a reviewed external thermal-path catalog",
			Suggestion: "bind the catalog used to construct the primitive inventory",
		}}
	}
	packageTypes := map[string]bool{}
	for _, instance := range graph.Instances {
		if !junctionToCaseInstances[instance.ID] {
			continue
		}
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found || strings.TrimSpace(primitive.PackageType) == "" {
			return nil, nil, []SimulationDiagnostic{{
				Code:       diagnosisThermalUnavailable,
				Path:       "simulation.thermal_boundary." + instance.ID,
				Message:    "thermally modeled device lacks a physical package for thermal-path compatibility",
				Suggestion: "onboard a reviewed package variant",
			}}
		}
		packageTypes[strings.ToLower(strings.TrimSpace(primitive.PackageType))] = true
	}
	type thermalPathSelection struct {
		Path            components.ThermalPathRecord `json:"path"`
		AssemblyCount   int                          `json:"assembly_count"`
		DevicesPerBatch int                          `json:"devices_per_assembly"`
	}
	paths := []thermalPathSelection{}
	for _, path := range catalog.ThermalPaths {
		if path.ReviewStatus != "reviewed" || strings.ToLower(path.Lifecycle) != "active" ||
			!acceptedConfidence(path.Verification.Confidence) ||
			path.MaximumSharedDevices <= 0 ||
			path.CaseToSinkCPerW < 0 || path.NaturalSinkToAmbientCPerW <= 0 {
			continue
		}
		compatible := true
		for packageType := range packageTypes {
			matched := false
			for _, candidate := range path.CompatiblePackages {
				matched = matched || strings.EqualFold(strings.TrimSpace(candidate), packageType)
			}
			compatible = compatible && matched
		}
		if compatible {
			paths = append(paths, thermalPathSelection{
				Path:            path,
				AssemblyCount:   (len(junctionToCaseInstances) + path.MaximumSharedDevices - 1) / path.MaximumSharedDevices,
				DevicesPerBatch: path.MaximumSharedDevices,
			})
		}
	}
	slices.SortFunc(paths, func(left, right thermalPathSelection) int {
		leftResistance := left.Path.CaseToSinkCPerW + left.Path.NaturalSinkToAmbientCPerW
		rightResistance := right.Path.CaseToSinkCPerW + right.Path.NaturalSinkToAmbientCPerW
		return cmp.Or(
			cmp.Compare(leftResistance, rightResistance),
			cmp.Compare(left.AssemblyCount, right.AssemblyCount),
			cmp.Compare(left.Path.ID, right.Path.ID),
		)
	})
	if len(paths) == 0 {
		return nil, nil, []SimulationDiagnostic{{
			Code:       diagnosisThermalUnavailable,
			Path:       "simulation.thermal_boundary",
			Message:    "no reviewed natural-convection thermal path covers every junction-to-case device package and shared-device count",
			Suggestion: "select fewer shared devices or onboard a compatible thermal assembly",
		}}
	}
	selection := paths[0]
	path := selection.Path
	targets := derivePowerTransferSizingTargets(requirement)
	railMagnitude := 0.0
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "supply" {
			railMagnitude = math.Max(
				railMagnitude,
				math.Abs(sourceValueForNode(requirement, operatingCase, corner, node)),
			)
		}
	}
	loadResistance := targets.loadResistance
	for _, condition := range thermalCase.Conditions {
		if condition.Axis != "load_resistance" {
			continue
		}
		value := corner.Values[conditionKey(condition)]
		if value <= 0 {
			value = positiveMidpoint(condition.Min, condition.Max)
		}
		if value > 0 && (loadResistance <= 0 || value < loadResistance) {
			loadResistance = value
		}
	}
	totalDissipation := railMagnitude * targets.quiescentCurrent
	if outputDissipation, bounded := thermalOutputStageDissipation(
		railMagnitude,
		targets.outputPeakVoltage,
		loadResistance,
	); bounded {
		totalDissipation += outputDissipation
	}
	if transferDissipation, bounded := thermalBehavioralTransferDissipation(
		requirement,
		thermalCase,
		corner,
		railMagnitude,
	); bounded {
		totalDissipation = math.Max(totalDissipation, transferDissipation)
	}
	if transferDissipation, bounded := thermalBehavioralCurrentTransferDissipation(
		requirement,
		thermalCase,
		corner,
		railMagnitude,
	); bounded {
		totalDissipation = math.Max(totalDissipation, transferDissipation)
	}
	if totalDissipation <= 0 || !finite(totalDissipation) {
		return nil, nil, []SimulationDiagnostic{{
			Code:       diagnosisThermalUnavailable,
			Path:       "simulation.thermal_boundary",
			Message:    "thermal boundary cannot be derived without bounded rail, load, output, and standing-current requirements",
			Suggestion: "declare the operating envelope needed to size the cooling assembly",
		}}
	}
	// Repeating a reviewed assembly partitions the symmetric stage's aggregate
	// dissipation. The device evaluator still adds each component's own
	// junction-to-case trajectory, so this division affects only the shared
	// sink boundary and does not hide local device stress.
	perAssemblyDissipation := totalDissipation / float64(selection.AssemblyCount)
	caseTemperature := ambient + perAssemblyDissipation*(path.CaseToSinkCPerW+path.NaturalSinkToAmbientCPerW)
	if !finite(caseTemperature) || caseTemperature < -100 || caseTemperature > 300 {
		return nil, nil, []SimulationDiagnostic{{
			Code:    diagnosisThermalUnavailable,
			Path:    "simulation.thermal_boundary.case_temperature_c",
			Message: "derived case boundary temperature is outside the trusted simulation range",
		}}
	}
	conditions = append(conditions, simmodel.NamedValue{Name: "case_temperature_c", Value: caseTemperature})
	return conditions, []string{hashJSON(selection)}, nil
}

// thermalBehavioralTransferDissipation derives a conservative linear-transfer
// envelope directly from declared rail, output-voltage, and load-current
// bounds. It is intentionally architecture-neutral: switching or more
// efficient implementations may run cooler, but the thermal boundary must not
// assume an efficiency that the behavioral contract does not establish.
func thermalBehavioralTransferDissipation(
	requirement Requirement,
	operatingCase OperatingCase,
	corner operatingCorner,
	railMagnitude float64,
) (float64, bool) {
	if railMagnitude <= 0 || !finite(railMagnitude) {
		return 0, false
	}
	ports := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	maximum := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_voltage" || assertion.Observation.Kind != "port" {
			continue
		}
		port, found := ports[assertion.Observation.ID]
		if !found || port.Direction != "source" || port.Kind != "power" {
			continue
		}
		outputVoltage := assertionTarget(assertion)
		if assertion.Min != nil && *assertion.Min > 0 {
			outputVoltage = *assertion.Min
		}
		if outputVoltage <= 0 || outputVoltage >= railMagnitude {
			continue
		}
		loadCurrent := 0.0
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "load_current" || condition.Target != port.ID {
				continue
			}
			loadCurrent = corner.Values[conditionKey(condition)]
			if loadCurrent <= 0 {
				loadCurrent = condition.Max
			}
		}
		if loadCurrent <= 0 && port.Electrical.MaxCurrentA != nil {
			loadCurrent = *port.Electrical.MaxCurrentA
		}
		dissipation := (railMagnitude - outputVoltage) * loadCurrent
		if dissipation > maximum && finite(dissipation) {
			maximum = dissipation
		}
	}
	return maximum, maximum > 0
}

// thermalBehavioralCurrentTransferDissipation derives a conservative linear
// current-source or current-sink loss envelope from behavioral declarations.
// The available current comes from output-current and transconductance bounds,
// capped by the port rating; the declared load converts that current into the
// minimum pass-device voltage burden. Sense and ballast drops are deliberately
// omitted, which overestimates rather than hides pass-device dissipation.
func thermalBehavioralCurrentTransferDissipation(
	requirement Requirement,
	operatingCase OperatingCase,
	corner operatingCorner,
	railMagnitude float64,
) (float64, bool) {
	if railMagnitude <= 0 || !finite(railMagnitude) {
		return 0, false
	}
	caseApplies := func(assertion BehavioralAssertion) bool {
		return len(assertion.OperatingCases) == 0 || slices.Contains(assertion.OperatingCases, operatingCase.ID)
	}
	maximum := 0.0
	for _, port := range requirement.Requirements.Ports {
		if (port.Direction != "source" && port.Direction != "sink") ||
			(port.Kind != "analog_current" && port.Kind != "controlled_current") {
			continue
		}
		current := 0.0
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if !caseApplies(assertion) || assertion.Observation.Kind != "port" ||
				assertion.Observation.ID != port.ID {
				continue
			}
			switch assertion.Metric {
			case "output_current":
				candidate := assertionTarget(assertion)
				if assertion.Max != nil {
					candidate = *assertion.Max
				}
				current = math.Max(current, candidate)
			case "transconductance":
				if assertion.Excitation == nil || assertion.Excitation.Kind != "port" {
					continue
				}
				gain := assertionTarget(assertion)
				if assertion.Max != nil {
					gain = *assertion.Max
				}
				command := 0.0
				for _, condition := range operatingCase.Conditions {
					if condition.Target != assertion.Excitation.ID ||
						(condition.Axis != "input_voltage" && condition.Axis != "control_voltage") {
						continue
					}
					command = corner.Values[conditionKey(condition)]
					if command <= 0 {
						command = condition.Max
					}
				}
				if gain > 0 && command > 0 {
					current = math.Max(current, gain*command)
				}
			}
		}
		if port.Electrical.MaxCurrentA != nil && *port.Electrical.MaxCurrentA > 0 {
			if current <= 0 {
				current = *port.Electrical.MaxCurrentA
			} else {
				current = math.Min(current, *port.Electrical.MaxCurrentA)
			}
		}
		loadResistance := 0.0
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "load_resistance" || condition.Target != port.ID {
				continue
			}
			// Linear pass-device loss is greatest at the minimum declared load
			// resistance. Use that bounded worst case even when the enclosing
			// electrical corner is evaluating a lighter load.
			candidate := condition.Min
			if candidate > 0 && (loadResistance <= 0 || candidate < loadResistance) {
				loadResistance = candidate
			}
		}
		if current <= 0 || loadResistance <= 0 {
			continue
		}
		loadVoltage := current * loadResistance
		dissipation := math.Max(0, railMagnitude-loadVoltage) * current
		if dissipation > maximum && finite(dissipation) {
			maximum = dissipation
		}
	}
	return maximum, maximum > 0
}

// thermalOutputStageDissipation derives the bounded class-B output-stage loss
// term used by the conservative thermal boundary. Invalid or open load
// conditions contribute no inferred dynamic loss; the caller still requires a
// finite positive total before accepting the boundary.
func thermalOutputStageDissipation(railMagnitude, outputPeakVoltage, loadResistance float64) (float64, bool) {
	if !finite(railMagnitude) || !finite(outputPeakVoltage) || !finite(loadResistance) ||
		railMagnitude <= 0 || outputPeakVoltage <= 0 || loadResistance <= 0 {
		return 0, false
	}
	peakCurrent := outputPeakVoltage / loadResistance
	dcInputPower := 2 * railMagnitude * peakCurrent / math.Pi
	loadPower := outputPeakVoltage * outputPeakVoltage / (2 * loadResistance)
	dissipation := math.Max(0, dcInputPower-loadPower)
	return dissipation, finite(dissipation)
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
	thermalConditions []simmodel.NamedValue,
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
		analysis.DCSweep = &simmodel.DCSweep{
			Component:     source,
			DeviceValue:   dcSweepUsesDeviceValue(requirement, assertion, source),
			StartValue:    start,
			StopValue:     stop,
			Points:        simulationDCSweepPoints(assertion, start, stop),
			Bidirectional: true,
		}
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
		if assertion.Analysis == simmodel.AnalysisStartup {
			// Startup is a behavioral/provenance contract executed by the
			// trusted transient engine so bounded source events are legal.
			analysis.Kind = simmodel.AnalysisTransient
		}
		duration := dynamicDurationForRequirement(requirement, assertion, operatingCase)
		analysis.DurationS = duration
		analysis.TimeStepS = dynamicTimeStep(duration, operatingCase)
		if assertion.Analysis == simmodel.AnalysisElectrothermal {
			analysis.Conditions = append([]simmodel.NamedValue(nil), thermalConditions...)
		}
		if assertion.Analysis != simmodel.AnalysisStartup {
			addSimulationEvents(&analysis, requirement, operatingCase, graph)
		}
		addAutonomousStartupEvents(&analysis, requirement, assertion, graph)
		simmodel.NormalizeDynamicGrid(&analysis)
	case simmodel.AnalysisDistortion:
		frequency := assertionFrequencyScale(requirement, assertion)
		analysis.DurationS = 4 / frequency
		analysis.TimeStepS = 1 / (frequency * 64)
	case simmodel.AnalysisThermal:
		analysis.Conditions = append([]simmodel.NamedValue(nil), thermalConditions...)
	}
	node := observationNodeID(graph, requirement, assertion.Observation)
	if quantity == simmodel.QuantityPhaseMarginDeg {
		loopNode, found := simulationStabilityObservationNode(graph, node)
		if !found {
			return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{
				Code:       diagnosisSimulationInvalid,
				Path:       "simulation.assertion.observation",
				Message:    "stability observation does not resolve to one catalog-modeled negative-feedback loop",
				Suggestion: "retain a unique passive feedback path from the observed output to an active inverting input",
			}}
		}
		node = loopNode
	}
	if node == "" && simulationQuantityNeedsNode(quantity) {
		return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{Code: diagnosisSimulationInvalid, Path: "simulation.assertion.observation", Message: "behavioral observation does not resolve to a candidate graph node"}}
	}
	minimum, maximum := scaledAssertionBounds(assertion, scale)
	if quantity == simmodel.QuantityInputImpedanceOhm && assertion.Max == nil {
		maximum = 1e15
	}
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
	if assertion.Metric == "on_state_voltage" {
		if positive, negative, found := simulationOnStateVoltageNodes(graph, node); found {
			simulationAssertion.Node = positive
			simulationAssertion.ReferenceNode = negative
		}
	}
	component, components, scopeDiagnostic := simulationMeasurementScope(
		requirement,
		assertion,
		operatingCase,
		corner,
		graph,
		evidence,
		quantity,
	)
	if scopeDiagnostic != nil {
		return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{*scopeDiagnostic}
	}
	simulationAssertion.Component = component
	simulationAssertion.Components = components
	frequencyPointAssertion :=
		(assertion.Analysis == simmodel.AnalysisACSweep &&
			(quantity == simmodel.QuantityVoltageMagnitudeV ||
				quantity == simmodel.QuantityVoltagePhaseDeg ||
				quantity == simmodel.QuantityVoltageDBV ||
				quantity == simmodel.QuantityVoltageGainRatio ||
				quantity == simmodel.QuantityTransimpedanceOhm ||
				quantity == simmodel.QuantityInputImpedanceOhm)) ||
			(assertion.Analysis == simmodel.AnalysisDistortion && quantity == simmodel.QuantityTHDPercent)
	if assertion.FrequencyHz != nil && frequencyPointAssertion {
		simulationAssertion.FrequencyHz = *assertion.FrequencyHz
	} else if assertion.FrequencyHz != nil && quantity == simmodel.QuantityDutyCyclePct {
		// Measure transfer duty over its declared excitation period. Faster
		// regulator subcycles on the observed switching node are not the
		// transfer frequency.
		simulationAssertion.FrequencyHz = *assertion.FrequencyHz
	} else if quantity == simmodel.QuantityVoltageGainRatio {
		simulationAssertion.FrequencyHz = assertionFrequencyScale(requirement, assertion)
	}
	if assertion.Excitation != nil &&
		(quantity == simmodel.QuantityVoltageGainRatio ||
			quantity == simmodel.QuantityCutoffFrequencyHz ||
			quantity == simmodel.QuantityBandwidthHz ||
			quantity == simmodel.QuantityInputImpedanceOhm) {
		if simulationAssertion.Component == "" {
			simulationAssertion.ReferenceNode = observationNodeID(graph, requirement, *assertion.Excitation)
		}
		if quantity == simmodel.QuantityInputImpedanceOhm {
			simulationAssertion.ReferenceNode = referenceNodeForDomain(graph, *assertion.Excitation)
		}
	}
	if assertion.Metric == "settling_time" {
		effectiveExcitation := simulationEffectiveExcitation(assertion, graph)
		if effectiveExcitation != nil {
			component := sourceInstanceForObservation(graph, *effectiveExcitation)
			for _, excitation := range analysis.Excitations {
				if excitation.Component != component || excitation.PulseWidthS <= 0 {
					continue
				}
				simulationAssertion.WindowStartS = excitation.PulseDelayS
				simulationAssertion.WindowEndS = analysis.DurationS
				break
			}
		}
	}
	if len(analysis.SourceValueEvents) != 0 {
		if assertion.Analysis == simmodel.AnalysisStartup {
			simulationAssertion.WindowStartS = 0
			simulationAssertion.WindowEndS = analysis.DurationS
			for _, event := range operatingCase.Events {
				if event.TriggerTimeS > 0 && event.TriggerTimeS < simulationAssertion.WindowEndS {
					simulationAssertion.WindowEndS = event.TriggerTimeS
				}
			}
		} else {
			for _, event := range operatingCase.Events {
				simulationAssertion.WindowStartS = event.TriggerTimeS
				simulationAssertion.WindowEndS = analysis.DurationS
				break
			}
		}
	}
	_ = evidence
	return analysis, simulationAssertion, nil
}

func simulationDCSweepPoints(assertion BehavioralAssertion, start, stop float64) int {
	const (
		defaultPoints        = 101
		maximumPoints        = 256
		stepsAcrossTolerance = 20
	)
	if assertion.Min == nil || assertion.Max == nil || *assertion.Max <= *assertion.Min {
		return defaultPoints
	}
	tolerance := *assertion.Max - *assertion.Min
	rangeWidth := math.Abs(stop - start)
	if rangeWidth == 0 || tolerance == 0 {
		return defaultPoints
	}
	points := math.Ceil(rangeWidth/(tolerance/stepsAcrossTolerance)) + 1
	if points < defaultPoints {
		return defaultPoints
	}
	if points > maximumPoints {
		return maximumPoints
	}
	return int(points)
}

func simulationOnStateVoltageNodes(
	graph CandidateGraph,
	observationNode string,
) (string, string, bool) {
	type voltagePair struct{ positive, negative string }
	pairs := []voltagePair{}
	for _, instance := range graph.Instances {
		// topologyTerminalNodes exposes the canonical primitive function names
		// validated when inventory terminals are bound; these are not raw symbol
		// pin labels or model-specific aliases.
		terminals := topologyTerminalNodes(instance)
		switch instance.Kind {
		case "n_channel_mosfet":
			if terminals["DRAIN"] == observationNode {
				pairs = append(pairs, voltagePair{terminals["DRAIN"], terminals["SOURCE"]})
			}
		case "p_channel_mosfet":
			if terminals["DRAIN"] == observationNode {
				pairs = append(pairs, voltagePair{terminals["SOURCE"], terminals["DRAIN"]})
			}
		case "npn_bjt":
			if terminals["COLLECTOR"] == observationNode {
				pairs = append(pairs, voltagePair{terminals["COLLECTOR"], terminals["EMITTER"]})
			}
		case "pnp_bjt":
			if terminals["COLLECTOR"] == observationNode {
				pairs = append(pairs, voltagePair{terminals["EMITTER"], terminals["COLLECTOR"]})
			}
		}
	}
	slices.SortFunc(pairs, func(left, right voltagePair) int {
		return cmp.Or(
			cmp.Compare(left.positive, right.positive),
			cmp.Compare(left.negative, right.negative),
		)
	})
	pairs = slices.Compact(pairs)
	if len(pairs) != 1 || pairs[0].positive == "" || pairs[0].negative == "" {
		return "", "", false
	}
	return pairs[0].positive, pairs[0].negative, true
}

func simulationStabilityObservationNode(
	graph CandidateGraph,
	observedNode string,
) (string, bool) {
	if observedNode == "" {
		return "", false
	}
	dcAdjacency := topologyPassiveNodeAdjacency(graph, true)
	supplies := topologyNodesByRole(graph, "supply")
	candidates := []string{}
	for _, instance := range graph.Instances {
		if instance.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["OUT"] == "" {
			continue
		}
		negativeFeedback := terminals["IN_MINUS"] != "" &&
			topologyNodePathExists(dcAdjacency, observedNode, terminals["IN_MINUS"])
		if !negativeFeedback && terminals["IN_PLUS"] != "" &&
			topologyNodePathExists(dcAdjacency, observedNode, terminals["IN_PLUS"]) {
			for _, driver := range graph.Instances {
				if driver.Kind != "pnp_bjt" {
					continue
				}
				driverTerminals := topologyTerminalNodes(driver)
				if !slices.Contains(supplies, driverTerminals["EMITTER"]) ||
					!topologyNodePathExists(dcAdjacency, terminals["OUT"], driverTerminals["BASE"]) {
					continue
				}
				for _, pass := range graph.Instances {
					if pass.Kind != "npn_bjt" {
						continue
					}
					passTerminals := topologyTerminalNodes(pass)
					if slices.Contains(supplies, passTerminals["COLLECTOR"]) &&
						topologyNodePathExists(dcAdjacency, driverTerminals["COLLECTOR"], passTerminals["BASE"]) &&
						topologyNodePathExists(dcAdjacency, passTerminals["EMITTER"], observedNode) {
						negativeFeedback = true
						break
					}
				}
				if negativeFeedback {
					break
				}
			}
		}
		if !negativeFeedback {
			continue
		}
		candidates = append(candidates, terminals["OUT"])
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
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
		simmodel.QuantityConversionEfficiencyPct,
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
	corner operatingCorner,
	graph CandidateGraph,
	evidence []simmodel.ComponentEvidence,
	quantity string,
) (string, []string, *SimulationDiagnostic) {
	switch quantity {
	case simmodel.QuantityCutoffFrequencyHz, simmodel.QuantityBandwidthHz:
		if assertion.Excitation == nil || !observationIsAnalogCurrentPort(requirement, *assertion.Excitation) {
			return "", nil, nil
		}
		component := sourceInstanceForObservation(graph, *assertion.Excitation)
		if component == "" {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "current-referenced frequency response requires a resolved excitation source"}
		}
		return component, nil, nil
	case simmodel.QuantityTransimpedanceOhm:
		if assertion.Excitation == nil {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "transimpedance measurement requires a resolved excitation source"}
		}
		component := sourceInstanceForObservation(graph, *assertion.Excitation)
		if component == "" {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "transimpedance measurement requires a resolved excitation source"}
		}
		return component, nil, nil
	case simmodel.QuantityInputImpedanceOhm:
		target := assertion.Observation
		if assertion.Excitation != nil {
			target = *assertion.Excitation
		}
		component := sourceInstanceForObservation(graph, target)
		if component == "" {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "input-impedance measurement requires a resolved excitation source"}
		}
		return component, nil, nil
	case simmodel.QuantityDeviceCurrentA, simmodel.QuantityDCSweepDeviceCurrentSpanA, simmodel.QuantityDCSweepDeviceSlopeAperV:
		if component, found := observedCurrentComponent(requirement, assertion, operatingCase, corner, graph); found {
			return component, nil, nil
		}
		return "", nil, &SimulationDiagnostic{
			Code:       diagnosisModelUnavailable,
			Path:       "simulation.assertion.component",
			Message:    "current measurement requires a unique catalog-backed load or active current path at the observed port",
			Suggestion: "declare a bounded load condition or remove ambiguous parallel active paths",
		}
	case simmodel.QuantityTotalSupplyCurrentA:
		if assertion.Metric == "quiescent_current" {
			components := supplySourceComponents(graph)
			if len(components) == 0 {
				return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.components", Message: "quiescent-current measurement requires at least one external supply source"}
			}
			return "", components, nil
		}
		component, found := dominantSupplySource(requirement, graph)
		if !found {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.components", Message: "supply-current measurement requires an external supply source"}
		}
		return "", []string{component}, nil
	case simmodel.QuantityPeakAbsDeviceCurrentA, simmodel.QuantityFinalAbsDeviceCurrentA:
		if assertion.Observation.Kind == "port" {
			if assertion.Metric == "peak_current" || assertion.Metric == "off_state_current" {
				if component, found := protectedVoltageOutputCurrentComponent(
					requirement, assertion, operatingCase, graph,
				); found {
					return component, nil, nil
				}
			}
			if component, found := observedCurrentComponent(requirement, assertion, operatingCase, corner, graph); found {
				return component, nil, nil
			}
		}
		component, found := dominantSupplySource(requirement, graph)
		if !found {
			return "", nil, &SimulationDiagnostic{Code: diagnosisModelUnavailable, Path: "simulation.assertion.component", Message: "startup-current measurement requires an external supply source"}
		}
		return component, nil, nil
	case simmodel.QuantityOutputPowerW:
		component, found := loadMeasurementComponent(
			requirement, assertion, operatingCase, corner, graph,
		)
		if !found {
			return "", nil, &SimulationDiagnostic{
				Code:       diagnosisModelUnavailable,
				Path:       "simulation.assertion.component",
				Message:    "output-power measurement requires one bounded load at the observed port",
				Suggestion: "declare a load-resistance operating condition for the observed output",
			}
		}
		return component, nil, nil
	case simmodel.QuantityConversionEfficiencyPct:
		component, found := loadMeasurementComponent(
			requirement, assertion, operatingCase, corner, graph,
		)
		if !found {
			return "", nil, &SimulationDiagnostic{
				Code:       diagnosisModelUnavailable,
				Path:       "simulation.assertion.component",
				Message:    "conversion-efficiency measurement requires one bounded load at the observed port",
				Suggestion: "declare a bounded load condition for the observed output",
			}
		}
		components := supplySourceComponents(graph)
		if len(components) == 0 {
			return "", nil, &SimulationDiagnostic{
				Code:    diagnosisModelUnavailable,
				Path:    "simulation.assertion.components",
				Message: "conversion-efficiency measurement requires at least one external supply source",
			}
		}
		return component, components, nil
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

func observationIsCurrentPort(requirement Requirement, observation Observation) bool {
	if observation.Kind != "port" {
		return false
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == observation.ID {
			return port.Kind == "analog_current" || port.Kind == "controlled_current"
		}
	}
	return false
}

func observationIsAnalogCurrentPort(requirement Requirement, observation Observation) bool {
	if observation.Kind != "port" {
		return false
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == observation.ID {
			return port.Kind == "analog_current"
		}
	}
	return false
}

func supplySourceComponents(graph CandidateGraph) []string {
	components := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.Role == "supply" {
			components = append(components, sourceInstanceForNode(node))
		}
	}
	slices.Sort(components)
	return slices.Compact(components)
}

func observedCurrentComponent(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) (string, bool) {
	observationNode := observationNodeID(graph, requirement, assertion.Observation)
	controlledPathComponents := []string{}
	activeComponents := []string{}
	for _, instance := range graph.Instances {
		if !topologyActiveKind(instance.Kind) {
			continue
		}
		for _, terminal := range instance.Terminals {
			if terminal.Node == observationNode {
				activeComponents = append(activeComponents, instance.ID)
				if topologyCurrentPathKind(instance.Kind) {
					controlledPathComponents = append(controlledPathComponents, instance.ID)
				}
				break
			}
		}
	}
	slices.Sort(controlledPathComponents)
	controlledPathComponents = slices.Compact(controlledPathComponents)
	if len(controlledPathComponents) == 1 {
		return controlledPathComponents[0], true
	}
	if len(controlledPathComponents) > 1 {
		return "", false
	}
	slices.Sort(activeComponents)
	activeComponents = slices.Compact(activeComponents)
	if len(activeComponents) == 1 {
		return activeComponents[0], true
	}
	if len(activeComponents) > 1 {
		return "", false
	}
	return loadMeasurementComponent(
		requirement,
		assertion,
		operatingCase,
		corner,
		graph,
	)
}

func topologyCurrentPathKind(kind string) bool {
	switch kind {
	case "n_channel_mosfet", "p_channel_mosfet", "npn_bjt", "pnp_bjt",
		"fixed_voltage_regulator", "adjustable_voltage_regulator", "synchronous_buck_regulator":
		return true
	default:
		return false
	}
}

func loadMeasurementComponent(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) (string, bool) {
	observationNode := observationNodeID(graph, requirement, assertion.Observation)
	conditions := simulationHarnessConditions(requirement, assertion, operatingCase)
	for _, condition := range conditions {
		if condition.Axis != "load_inductance" || simulationConditionValue(corner, condition) <= 0 {
			continue
		}
		target, found := externalNodeForSemanticTarget(graph, condition.Target)
		if found && target.ID == observationNode {
			return loadInstanceID(condition.Target, condition.Axis), true
		}
	}
	components := []string{}
	for _, condition := range conditions {
		if condition.Axis != "load_resistance" && condition.Axis != "load_current" {
			continue
		}
		target, found := externalNodeForSemanticTarget(graph, condition.Target)
		if found && target.ID == observationNode {
			components = append(components, loadInstanceID(condition.Target, condition.Axis))
		}
	}
	slices.Sort(components)
	components = slices.Compact(components)
	if len(components) != 1 {
		return "", false
	}
	return components[0], true
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
	if topologyGraphHasLowSideCurrentRegulation(graph, target.ID, reference) {
		if supply, found := dominantSupplyNode(requirement, graph); found {
			return supply.ID, target.ID
		}
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "n_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["DRAIN"] != target.ID ||
			!topologyLowSideSwitchReturnsToReference(graph, terminals["SOURCE"], reference) {
			continue
		}
		if rail, found := topologySwitchedLoadRail(graph, target.ID, reference); found {
			return rail, target.ID
		}
		if supply, found := dominantSupplyNode(requirement, graph); found {
			return supply.ID, target.ID
		}
	}
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
			if (!requireSOA && (claim.ThermalModel != nil || namedThermalParametersComplete(claim.Parameters))) ||
				(requireSOA && len(claim.TransientSOA) != 0) {
				result = append(result, component.InstanceID)
				break
			}
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func namedThermalParametersComplete(parameters []simmodel.NamedValue) bool {
	maximumTemperature, thermalResistance := false, false
	for _, parameter := range parameters {
		switch parameter.Name {
		case "max_temperature_c":
			maximumTemperature = finite(parameter.Value)
		case "thermal_resistance_c_per_w", "junction_to_ambient_c_per_w":
			thermalResistance = finite(parameter.Value) && parameter.Value > 0
		}
	}
	return maximumTemperature && thermalResistance
}

func simulationExcitations(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) []simmodel.SourceExcitation {
	result := []simmodel.SourceExcitation{}
	effectiveExcitation := simulationEffectiveExcitation(assertion, graph)
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role == "reference" || node.Role == "output" {
			continue
		}
		excitation := simmodel.SourceExcitation{
			Component: sourceInstanceForNode(node),
			DCValue: assertionSourceValue(
				requirement,
				assertion,
				operatingCase,
				corner,
				node,
			),
		}
		periodicPulse := assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 &&
			(transientPeriodicPulseMetric(assertion.Metric) ||
				periodicControlPulseRequired(requirement, assertion, effectiveExcitation))
		if (assertion.Analysis == simmodel.AnalysisTransient ||
			assertion.Analysis == simmodel.AnalysisDistortion ||
			assertion.Analysis == simmodel.AnalysisElectrothermal) &&
			effectiveExcitation != nil &&
			observationMatchesNode(node, *effectiveExcitation) &&
			(!operatingCaseHasSourceEvent(operatingCase, graph, node) || periodicPulse ||
				assertion.Analysis == simmodel.AnalysisDistortion) {
			if assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 && !periodicPulse {
				// Frequency metadata defines the observation window for many
				// transient output metrics; it does not turn a typed digital
				// control into an analog sine. Start a digital active-state check
				// from its stable inactive state, then hold the requested active
				// value for the rest of the window. This also avoids requiring a
				// switching regulator to have an impossible static DC initial
				// condition at its hysteretic current threshold.
				if node.Role == "control" || node.SemanticKind == "digital" || node.SemanticKind == "control" {
					active := excitation.DCValue
					inactive, maximum, bounded := periodicControlRange(requirement, node.SemanticID)
					if bounded {
						if active <= inactive {
							active = maximum
						}
						duration := dynamicDurationForRequirement(requirement, assertion, operatingCase)
						excitation.DCValue = 0
						excitation.PulseInitialValue = inactive
						excitation.PulseValue = active
						excitation.PulseDelayS = duration / 100
						excitation.PulseWidthS = duration
						excitation.PulsePeriodS = duration * 2
					}
					result = append(result, excitation)
					continue
				}
				excitation.DCValue = periodicExcitationBias(requirement, operatingCase, node)
				excitation.SineAmplitude = excitationAmplitude(requirement, *effectiveExcitation)
				excitation.SineFrequencyHz = *assertion.FrequencyHz
				result = append(result, excitation)
				continue
			}
			configuredPulse := false
			configurePulse := func(initial, applied float64) {
				duration := dynamicDurationForRequirement(requirement, assertion, operatingCase)
				excitation.DCValue = 0
				excitation.PulseInitialValue = initial
				excitation.PulseValue = applied
				excitation.PulseDelayS = duration / 5
				excitation.PulseWidthS = duration * 3 / 5
				if periodicPulse {
					excitation.PulseDelayS = periodicPulseDelay(operatingCase, node, graph, 1/(*assertion.FrequencyHz))
					excitation.PulseWidthS = .5 / *assertion.FrequencyHz
					excitation.PulsePeriodS = 1 / *assertion.FrequencyHz
				}
				if assertion.Metric == "settling_time" {
					excitation.PulseWidthS = duration
				}
				if !periodicPulse {
					excitation.PulsePeriodS = duration * 2
				}
				configuredPulse = true
			}
			for _, condition := range operatingCase.Conditions {
				target, found := externalNodeForSemanticTarget(graph, condition.Target)
				if !found || target.ID != node.ID ||
					(condition.Axis != "input_voltage" &&
						condition.Axis != "control_voltage") ||
					condition.Min == condition.Max {
					continue
				}
				initial, applied := condition.Min, condition.Max
				if windowInitial, windowApplied, found := windowDynamicExcitationRange(
					requirement, assertion, condition,
				); found {
					initial, applied = windowInitial, windowApplied
				}
				configurePulse(initial, applied)
				break
			}
			if periodicPulse && !configuredPulse {
				if initial, applied, found := periodicControlRange(requirement, node.SemanticID); found {
					configurePulse(initial, applied)
				}
			}
		}
		if assertion.Analysis == simmodel.AnalysisACSweep && effectiveExcitation != nil &&
			observationMatchesNode(node, *effectiveExcitation) {
			excitation.ACMagnitude = 1
		}
		result = append(result, excitation)
	}
	for _, condition := range simulationHarnessConditions(requirement, assertion, operatingCase) {
		if condition.Axis != "load_current" && condition.Axis != "load_resistance" {
			continue
		}
		// Quiescent current is the supply current with the declared external
		// load removed. Keep its harness and excitation sets identical: a load
		// source omitted from the circuit must not survive as an analysis
		// excitation referencing a nonexistent component.
		if condition.Axis == "load_current" && simulationExcludesLoadCurrent(assertion) {
			continue
		}
		if condition.Axis == "load_current" {
			if _, found := dynamicVoltageOutputLoadResistance(
				requirement, assertion, condition, corner.Values[conditionKey(condition)],
			); found {
				continue
			}
		}
		if condition.Axis == "load_resistance" {
			continue
		}
		value := corner.Values[conditionKey(condition)]
		if value == 0 {
			value = simulationConditionValue(corner, condition)
		}
		result = append(result, simmodel.SourceExcitation{
			Component: loadInstanceID(condition.Target, condition.Axis),
			DCValue:   value,
		})
	}
	slices.SortFunc(result, func(left, right simmodel.SourceExcitation) int {
		return cmp.Compare(left.Component, right.Component)
	})
	return result
}

func periodicControlPulseRequired(
	requirement Requirement,
	assertion BehavioralAssertion,
	effectiveExcitation *Observation,
) bool {
	if assertion.Analysis != simmodel.AnalysisElectrothermal || effectiveExcitation == nil ||
		assertion.FrequencyHz == nil || *assertion.FrequencyHz <= 0 {
		return false
	}
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric != "duty_cycle" || candidate.Excitation == nil || candidate.FrequencyHz == nil {
			continue
		}
		if *candidate.Excitation == *effectiveExcitation && *candidate.FrequencyHz == *assertion.FrequencyHz {
			return true
		}
	}
	return false
}

func periodicControlRange(requirement Requirement, semanticID string) (float64, float64, bool) {
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Target == semanticID &&
				(condition.Axis == "input_voltage" || condition.Axis == "control_voltage") && condition.Min < condition.Max {
				minimum = math.Min(minimum, condition.Min)
				maximum = math.Max(maximum, condition.Max)
			}
		}
	}
	if finite(minimum) && finite(maximum) && minimum < maximum {
		return minimum, maximum, true
	}
	for _, port := range requirement.Requirements.Ports {
		if port.ID != semanticID || port.Electrical.MinVoltageV == nil || port.Electrical.MaxVoltageV == nil ||
			*port.Electrical.MinVoltageV >= *port.Electrical.MaxVoltageV {
			continue
		}
		return *port.Electrical.MinVoltageV, *port.Electrical.MaxVoltageV, true
	}
	return 0, 0, false
}

func transientPeriodicPulseMetric(metric string) bool {
	switch metric {
	case "duty_cycle", "oscillation_frequency", "rise_time", "fall_time":
		return true
	default:
		return false
	}
}

func periodicPulseDelay(operatingCase OperatingCase, node GraphNode, graph CandidateGraph, periodS float64) float64 {
	for _, event := range operatingCase.Events {
		target, found := externalNodeForSemanticTarget(graph, event.Target)
		if found && target.ID == node.ID {
			return event.TriggerTimeS
		}
	}
	return periodS
}

func simulationExcludesLoadCurrent(assertion BehavioralAssertion) bool {
	return assertion.Metric == "quiescent_current" ||
		assertion.Metric == "off_state_current" ||
		assertion.Metric == "startup_output_voltage"
}

func operatingCaseHasSourceEvent(operatingCase OperatingCase, graph CandidateGraph, node GraphNode) bool {
	for _, event := range operatingCase.Events {
		target, found := externalNodeForSemanticTarget(graph, event.Target)
		if found && target.ID == node.ID {
			return true
		}
	}
	return false
}

func windowDynamicExcitationRange(
	requirement Requirement,
	assertion BehavioralAssertion,
	condition OperatingCondition,
) (float64, float64, bool) {
	if assertion.Metric != "propagation_delay" || assertion.Excitation == nil ||
		assertion.Excitation.Kind != "port" || condition.Target != assertion.Excitation.ID {
		return 0, 0, false
	}
	envelope, required := topologyWindowBehaviorEnvelope(requirement)
	if !required || assertion.Excitation.ID != envelope.input ||
		condition.Min >= envelope.lowerV || condition.Max <= envelope.lowerV {
		return 0, 0, false
	}
	span := envelope.upperV - envelope.lowerV
	initial := math.Max(condition.Min, envelope.lowerV-span/4)
	applied := math.Min(condition.Max, envelope.lowerV+span/4)
	if initial >= envelope.lowerV || applied <= envelope.lowerV || applied >= envelope.upperV {
		return 0, 0, false
	}
	return initial, applied, true
}

func loadInstanceID(target, axis string) string {
	return canonicalIdentifier("load_" + target + "_" + axis)
}

func dcSweepUsesDeviceValue(requirement Requirement, assertion BehavioralAssertion, source string) bool {
	if assertion.Metric != "load_regulation" || source == "" {
		return false
	}
	for _, caseID := range assertion.OperatingCases {
		for _, operatingCase := range requirement.Requirements.OperatingCases {
			if operatingCase.ID != caseID {
				continue
			}
			for _, condition := range operatingCase.Conditions {
				if condition.Axis == "load_resistance" && source == loadInstanceID(condition.Target, condition.Axis) {
					return true
				}
			}
		}
	}
	return false
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
				!reviewedCatalogModelSupportsCircuitAnalysis(modelID, provenance.Provenance.AllowedAnalyses, analysis) {
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
		if found && reviewedCatalogModelSupportsCircuitAnalysis(claim.ModelID, provenance.Provenance.AllowedAnalyses, analysis) {
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
		if condition.Axis != "supply_voltage" && condition.Axis != "input_voltage" &&
			condition.Axis != "input_current" {
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

// assertionSourceValue resolves a DC source at the operating point implied by
// the assertion when that point is more specific than the enclosing corner.
// A current target paired with a voltage-to-current transfer defines its
// command unambiguously as Vcmd=Iout/gm. Using the midpoint of a wider command
// sweep would evaluate the current assertion at a different requested current
// and can turn two mutually consistent behavioral assertions into a false
// conflict.
func assertionSourceValue(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	node GraphNode,
) float64 {
	fallback := sourceValueForNode(requirement, operatingCase, corner, node)
	if assertion.Analysis == simmodel.AnalysisStartup {
		for _, event := range operatingCase.Events {
			if event.Target == node.SemanticID && event.Kind != "short_circuit" {
				return event.Initial
			}
		}
	}
	if node.Role == "control" || requirementPortDrivesDecision(requirement, node.SemanticID) {
		switch assertion.Metric {
		case "off_state_current":
			inactive := math.Inf(1)
			for _, candidateCase := range requirement.Requirements.OperatingCases {
				for _, condition := range candidateCase.Conditions {
					if condition.Target == node.SemanticID &&
						(condition.Axis == "input_voltage" || condition.Axis == "control_voltage") {
						inactive = math.Min(inactive, condition.Min)
					}
				}
			}
			if finite(inactive) {
				return inactive
			}
		case "on_state_voltage":
			active := 0.0
			for _, candidateCase := range requirement.Requirements.OperatingCases {
				for _, condition := range candidateCase.Conditions {
					if condition.Target == node.SemanticID &&
						(condition.Axis == "input_voltage" || condition.Axis == "control_voltage") {
						active = math.Max(active, condition.Max)
					}
				}
			}
			if active > 0 {
				return active
			}
		}
		if simulationAssertionRequiresActiveControl(requirement, assertion) {
			eventDrivenAnalysis := assertion.Analysis == simmodel.AnalysisTransient ||
				assertion.Analysis == simmodel.AnalysisElectrothermal
			hasExecutedTransition := false
			if eventDrivenAnalysis {
				for _, event := range operatingCase.Events {
					if event.Target == node.SemanticID && event.Kind == "input_step" {
						hasExecutedTransition = true
						break
					}
				}
			}
			if !hasExecutedTransition {
				if active, found := requirementActiveControlValue(requirement, node.SemanticID); found {
					return active
				}
			}
		}
	}
	if assertion.Metric != "output_current" || node.Scope != "external" {
		return fallback
	}
	targetCurrent := assertionTarget(assertion)
	if targetCurrent <= 0 {
		return fallback
	}
	for _, transfer := range requirement.Requirements.BehavioralRequirements {
		if transfer.Metric != "transconductance" || transfer.Excitation == nil ||
			transfer.Excitation.Kind != "port" ||
			transfer.Observation != assertion.Observation ||
			transfer.Excitation.ID != node.SemanticID {
			continue
		}
		transconductance := assertionTarget(transfer)
		if transconductance <= 0 {
			continue
		}
		command := targetCurrent / transconductance
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "input_voltage" ||
				condition.Target != transfer.Excitation.ID ||
				command < condition.Min || command > condition.Max {
				continue
			}
			return command
		}
	}
	return fallback
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

func simulationEffectiveExcitation(
	assertion BehavioralAssertion,
	graph CandidateGraph,
) *Observation {
	if assertion.Excitation != nil {
		result := *assertion.Excitation
		return &result
	}
	if assertion.Analysis == "dc_sweep" {
		switch assertion.Metric {
		case "hysteresis", "rising_threshold", "falling_threshold",
			"threshold_voltage", "threshold_current", "lower_threshold", "upper_threshold":
		default:
			return nil
		}
	} else if assertion.Analysis != simmodel.AnalysisTransient &&
		assertion.Analysis != simmodel.AnalysisDistortion &&
		assertion.Analysis != simmodel.AnalysisElectrothermal {
		return nil
	}
	candidates := []Observation{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || (node.Role != "input" && node.Role != "control") ||
			node.SemanticID == "" || node.SemanticKind == "" {
			continue
		}
		candidates = append(candidates, Observation{Kind: node.SemanticKind, ID: node.SemanticID})
	}
	slices.SortFunc(candidates, func(left, right Observation) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	})
	candidates = slices.Compact(candidates)
	if len(candidates) != 1 {
		return nil
	}
	return &candidates[0]
}

func excitationAmplitude(requirement Requirement, observation Observation) float64 {
	portLimit := 0.0
	if observation.Kind == "port" {
		for _, port := range requirement.Requirements.Ports {
			if port.ID != observation.ID {
				continue
			}
			if port.Electrical.MaxVoltageV != nil && port.Electrical.MinVoltageV != nil {
				amplitude := (*port.Electrical.MaxVoltageV - *port.Electrical.MinVoltageV) / 2
				if amplitude > 0 {
					portLimit = amplitude
				}
			}
		}
	}
	targets := derivePowerTransferSizingTargets(requirement)
	drivePeakVoltage := targets.drivePeakVoltage
	if drivePeakVoltage <= 0 {
		drivePeakVoltage = targets.outputPeakVoltage
	}
	if drivePeakVoltage > 0 && targets.gain > 0 {
		target := drivePeakVoltage / targets.gain
		if target > 0 && (portLimit == 0 || target < portLimit) {
			return target
		}
	}
	if portLimit > 0 {
		return portLimit
	}
	return 1
}

// periodicExcitationBias separates a periodic signal's quiescent operating
// point from the operating-corner endpoints that bound its waveform. A fixed
// condition remains an explicit bias. For a ranged signal, the declared port
// nominal is authoritative when it lies inside the range; otherwise the range
// midpoint is the only deterministic bias implied by the requirement.
func periodicExcitationBias(
	requirement Requirement,
	operatingCase OperatingCase,
	node GraphNode,
) float64 {
	for _, condition := range operatingCase.Conditions {
		if (condition.Target != node.SemanticID && condition.Target != node.Domain) ||
			(condition.Axis != "input_voltage" &&
				condition.Axis != "control_voltage" &&
				condition.Axis != "input_current") {
			continue
		}
		if condition.Min == condition.Max {
			return condition.Min
		}
		if condition.Axis != "input_current" {
			if nominal, found := portNominalVoltage(requirement, node.SemanticID); found &&
				nominal >= condition.Min && nominal <= condition.Max {
				return nominal
			}
		}
		return (condition.Min + condition.Max) / 2
	}
	if nominal, found := portNominalVoltage(requirement, node.SemanticID); found {
		return nominal
	}
	return 0
}

func portNominalVoltage(requirement Requirement, portID string) (float64, bool) {
	for _, port := range requirement.Requirements.Ports {
		if port.ID == portID && port.Electrical.NominalVoltageV != nil {
			return *port.Electrical.NominalVoltageV, true
		}
	}
	return 0, false
}

func dynamicDuration(assertion BehavioralAssertion, operatingCase OperatingCase) float64 {
	frequency := 0.0
	if assertion.FrequencyHz != nil {
		frequency = *assertion.FrequencyHz
	} else if assertion.Unit == "Hz" {
		frequency = assertionTarget(assertion)
	}
	return dynamicDurationAtFrequency(assertion, operatingCase, frequency)
}

func dynamicDurationForRequirement(requirement Requirement, assertion BehavioralAssertion, operatingCase OperatingCase) float64 {
	return dynamicDurationAtFrequency(assertion, operatingCase, assertionFrequencyScale(requirement, assertion))
}

func dynamicDurationAtFrequency(assertion BehavioralAssertion, operatingCase OperatingCase, frequencyHz float64) float64 {
	duration := 0.0
	switch assertion.Metric {
	case "settling_time", "propagation_delay", "rise_time", "fall_time":
		duration = assertionTarget(assertion) * 10
	}
	periodicDuration := 0.0
	if frequencyHz > 0 && finite(frequencyHz) {
		periodicDuration = 10 / frequencyHz
		duration = math.Max(duration, periodicDuration)
	}
	for _, event := range operatingCase.Events {
		// Preserve a deterministic post-event observation interval. Merely
		// doubling a short trigger delay can end a startup analysis while a
		// bounded feedback loop is still approaching its final value, which
		// turns ordinary settling into false overshoot.
		if periodicDuration > 0 {
			duration = math.Max(duration, event.TriggerTimeS+periodicDuration)
		} else {
			duration = math.Max(duration, math.Max(0.01, event.TriggerTimeS*10))
		}
	}
	if duration <= 0 {
		duration = 0.01
	}
	return math.Min(duration, 10)
}

func dynamicTimeStep(duration float64, operatingCase OperatingCase) float64 {
	const ticksPerSecond int64 = 1_000_000_000_000
	if duration <= 0 || !finite(duration) {
		return 0
	}
	durationTicks := int64(math.Round(duration * float64(ticksPerSecond)))
	gridTicks := durationTicks
	for _, event := range operatingCase.Events {
		triggerTicks := int64(math.Round(event.TriggerTimeS * float64(ticksPerSecond)))
		if triggerTicks > 0 && triggerTicks < durationTicks {
			gridTicks = greatestCommonDivisor(gridTicks, triggerTicks)
		}
	}
	targetTicks := max(int64(1), durationTicks/1000)
	divisor := max(int64(1), (gridTicks+targetTicks-1)/targetTicks)
	alignedStep := float64(gridTicks) / float64(divisor) / float64(ticksPerSecond)
	// Exact event alignment can require a pathologically fine common grid for
	// arbitrary decimal trigger times. Bound the work while retaining exact
	// alignment whenever that grid fits within the deterministic step budget.
	return math.Max(alignedStep, duration/maximumDynamicTimeSteps)
}

func greatestCommonDivisor(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	if left < 0 {
		return -left
	}
	return left
}

func addSimulationEvents(analysis *simmodel.Analysis, requirement Requirement, operatingCase OperatingCase, graph CandidateGraph) {
	for _, event := range operatingCase.Events {
		target, found := externalNodeForSemanticTarget(graph, event.Target)
		if !found {
			continue
		}
		if event.Kind == "short_circuit" {
			resistance, resistanceFound := protectedShortResistance(requirement, event)
			if !resistanceFound {
				continue
			}
			analysis.DeviceValueEvents = append(analysis.DeviceValueEvents, simmodel.DeviceValueEvent{
				ID: canonicalIdentifier(event.ID), Component: shortLoadInstanceID(event.Target),
				TriggerTimeS: event.TriggerTimeS,
				DurationS:    math.Max(analysis.DurationS-event.TriggerTimeS, analysis.TimeStepS),
				InitialSI:    1e12,
				AppliedSI:    resistance,
			})
			continue
		}
		component := sourceInstanceForNode(target)
		periodicallyDriven := false
		for _, excitation := range analysis.Excitations {
			if excitation.Component == component && excitation.PulsePeriodS > 0 {
				periodicallyDriven = true
				break
			}
		}
		if periodicallyDriven {
			continue
		}
		analysis.SourceValueEvents = append(analysis.SourceValueEvents, simmodel.SourceValueEvent{
			ID:           canonicalIdentifier(event.ID),
			Component:    component,
			TriggerTimeS: event.TriggerTimeS,
			DurationS:    math.Max(analysis.DurationS-event.TriggerTimeS, analysis.TimeStepS),
			Initial:      event.Initial,
			Applied:      event.Applied,
		})
	}
}

func addAutonomousStartupEvents(
	analysis *simmodel.Analysis,
	requirement Requirement,
	assertion BehavioralAssertion,
	graph CandidateGraph,
) {
	if analysis == nil || analysis.Kind != simmodel.AnalysisTransient || assertion.Observation.Kind != "port" {
		return
	}
	autonomousOutput := false
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "oscillation_frequency" && candidate.Analysis == simmodel.AnalysisTransient &&
			candidate.Excitation == nil && candidate.Observation == assertion.Observation {
			autonomousOutput = true
			break
		}
	}
	if !autonomousOutput {
		return
	}
	excitationByComponent := map[string]float64{}
	for _, excitation := range analysis.Excitations {
		excitationByComponent[excitation.Component] = excitation.DCValue
	}
	existing := map[string]bool{}
	for _, event := range analysis.SourceValueEvents {
		existing[event.Component] = true
	}
	for _, node := range graph.Nodes {
		if node.Scope != "external" || node.Role != "supply" {
			continue
		}
		component := sourceInstanceForNode(node)
		applied := excitationByComponent[component]
		if existing[component] || applied == 0 || !finite(applied) {
			continue
		}
		analysis.SourceValueEvents = append(analysis.SourceValueEvents, simmodel.SourceValueEvent{
			ID:           canonicalIdentifier("autonomous_startup_" + node.SemanticID),
			Component:    component,
			TriggerTimeS: 0,
			DurationS:    analysis.DurationS,
			Initial:      0,
			Applied:      applied,
		})
	}
	slices.SortFunc(analysis.SourceValueEvents, func(left, right simmodel.SourceValueEvent) int {
		return simmodel.CompareValueEventOrder(
			left.Component, left.TriggerTimeS, left.ID,
			right.Component, right.TriggerTimeS, right.ID,
		)
	})
}

func shortLoadInstanceID(target string) string {
	return canonicalIdentifier("load_" + target + "_short_circuit")
}

func protectedShortResistance(requirement Requirement, event OperatingEvent) (float64, bool) {
	faultCurrentA := math.Abs(event.Applied)
	if event.Kind != "short_circuit" || faultCurrentA <= 0 || !finite(faultCurrentA) {
		return 0, false
	}
	outputVoltageV := 0.0
	for _, port := range requirement.Requirements.Ports {
		if port.ID != event.Target {
			continue
		}
		for _, value := range []*float64{
			port.Electrical.MaxVoltageV,
			port.Electrical.NominalVoltageV,
			port.Electrical.MinVoltageV,
		} {
			if value != nil {
				outputVoltageV = math.Max(outputVoltageV, math.Abs(*value))
			}
		}
		break
	}
	if outputVoltageV <= 0 || !finite(outputVoltageV) {
		return 0, false
	}
	resistance := outputVoltageV / faultCurrentA
	return resistance, resistance > 0 && finite(resistance)
}

func protectedVoltageOutputCurrentComponent(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	graph CandidateGraph,
) (string, bool) {
	observationNode := observationNodeID(graph, requirement, assertion.Observation)
	if observationNode == "" {
		return "", false
	}
	direction := 0
	for _, event := range operatingCase.Events {
		if event.Kind != "short_circuit" || event.Target != assertion.Observation.ID {
			continue
		}
		if event.Applied > 0 {
			direction = 1
		} else if event.Applied < 0 {
			direction = -1
		}
	}
	if direction == 0 {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "load_current" || condition.Target != assertion.Observation.ID {
				continue
			}
			if condition.Min >= 0 && condition.Max > 0 {
				direction = 1
			} else if condition.Min < 0 && condition.Max <= 0 {
				direction = -1
			}
		}
	}
	supplies := topologyNodesByRole(graph, "supply")
	references := topologyNodesByRole(graph, "reference")
	candidates := []string{}
	paths := [][]string{}
	for _, instance := range graph.Instances {
		if direction > 0 && instance.Kind != "npn_bjt" {
			continue
		}
		if direction < 0 && instance.Kind != "pnp_bjt" {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		railConnected := direction > 0 && slices.Contains(supplies, nodes["COLLECTOR"])
		railConnected = railConnected || direction < 0 && slices.Contains(references, nodes["COLLECTOR"])
		path := topologyResistorPath(graph, nodes["EMITTER"], observationNode)
		if !railConnected || len(path) == 0 {
			continue
		}
		candidates = append(candidates, instance.ID)
		paths = append(paths, path)
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	if len(candidates) == 0 {
		return "", false
	}
	shared := map[string]bool{}
	for _, id := range paths[0] {
		shared[id] = true
	}
	for _, path := range paths[1:] {
		present := map[string]bool{}
		for _, id := range path {
			present[id] = true
		}
		for id := range shared {
			if !present[id] {
				delete(shared, id)
			}
		}
	}
	commonSeries := make([]string, 0, len(shared))
	for id := range shared {
		commonSeries = append(commonSeries, id)
	}
	slices.Sort(commonSeries)
	if len(commonSeries) == 0 {
		return "", false
	}
	return commonSeries[0], true
}

func sweepSourceAndRange(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
) (string, float64, float64, bool) {
	if excitation := simulationEffectiveExcitation(assertion, graph); excitation != nil {
		source := sourceInstanceForObservation(graph, *excitation)
		excitationTarget := ""
		excitationAxis := ""
		for _, condition := range simulationHarnessConditions(requirement, assertion, operatingCase) {
			target, found := externalNodeForSemanticTarget(graph, condition.Target)
			if found && observationMatchesNode(target, *excitation) &&
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
		if excitation.Kind == "port" {
			for _, port := range requirement.Requirements.Ports {
				if port.ID != excitation.ID ||
					port.Electrical.MinVoltageV == nil ||
					port.Electrical.MaxVoltageV == nil ||
					*port.Electrical.MaxVoltageV <= *port.Electrical.MinVoltageV {
					continue
				}
				return source, *port.Electrical.MinVoltageV, *port.Electrical.MaxVoltageV, true
			}
		}
	}
	if assertion.Metric == "line_regulation" {
		source := ""
		minimum, maximum := math.Inf(1), math.Inf(-1)
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "supply_voltage" || condition.Max <= condition.Min {
				continue
			}
			target, found := externalNodeForSemanticTarget(graph, condition.Target)
			if !found || target.Role != "supply" {
				continue
			}
			candidate := sourceInstanceForNode(target)
			if source != "" && source != candidate {
				return "", 0, 0, false
			}
			source = candidate
			minimum = math.Min(minimum, condition.Min)
			maximum = math.Max(maximum, condition.Max)
		}
		if source != "" && maximum > minimum {
			return source, minimum, maximum, true
		}
	}
	if assertion.Metric == "load_regulation" {
		minimum, maximum := math.Inf(1), math.Inf(-1)
		targetID := ""
		loadAxis := ""
		for _, caseID := range assertion.OperatingCases {
			for _, candidateCase := range requirement.Requirements.OperatingCases {
				if candidateCase.ID != caseID {
					continue
				}
				for _, condition := range candidateCase.Conditions {
					if condition.Axis != "load_current" && condition.Axis != "load_resistance" {
						continue
					}
					target, found := externalNodeForSemanticTarget(graph, condition.Target)
					if !found || target.ID != observationNodeID(graph, requirement, assertion.Observation) {
						continue
					}
					if (targetID != "" && targetID != condition.Target) ||
						(loadAxis != "" && loadAxis != condition.Axis) {
						return "", 0, 0, false
					}
					targetID = condition.Target
					loadAxis = condition.Axis
					minimum = math.Min(minimum, condition.Min)
					maximum = math.Max(maximum, condition.Max)
				}
			}
		}
		if targetID != "" && loadAxis != "" && maximum > minimum {
			return loadInstanceID(targetID, loadAxis), minimum, maximum, true
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
