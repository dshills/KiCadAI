package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	v17MaximumDynamicTimeSteps = 100_000
	v17RetainedReportPoints    = 256
)

func EvaluateCandidateV17(
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
		return finalizeSimulationEvaluationV17(result)
	}
	result.RequirementHash = requirementHash
	if issues := Validate(requirement); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluationV17(result)
	}
	if issues := validateSimulationEnvironment(inventory, environment); len(issues) != 0 {
		result.Status = SimulationEvaluationUnsupported
		result.Issues = issues
		return finalizeSimulationEvaluationV17(result)
	}
	if trial != nil {
		graph, err = ApplyValueTrial(graph, *trial, inventory)
		if err != nil {
			result.Issues = []reports.Issue{graphIssue(CodeValueExhausted, "value_trial", "apply value trial: "+err.Error(), "")}
			return finalizeSimulationEvaluationV17(result)
		}
		result.ValueTrialHash = trial.Hash
	}
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize simulation graph: "+err.Error(), "")}
		return finalizeSimulationEvaluationV17(result)
	}
	result.GraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash simulation graph: "+err.Error(), "")}
		return finalizeSimulationEvaluationV17(result)
	}
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	if issues := ValidateCompleteGraph(graph, inventory, limits); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluationV17(result)
	}
	if issues := validateGraphRequirementBinding(graph, requirement); len(issues) != 0 {
		result.Issues = issues
		return finalizeSimulationEvaluationV17(result)
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
				return finalizeSimulationEvaluationV17(result)
			}
			if result.Consumption.CandidateSimulations >= result.Policy.MaxCandidateSimulations ||
				result.Consumption.CornerEvaluations >= result.Policy.MaxCornerEvaluations {
				result.Status = SimulationEvaluationExhausted
				result.Consumption.BudgetExhausted = true
				result.Issues = []reports.Issue{graphIssue(CodeSearchExhausted, "simulation.policy", "candidate-simulation or operating-corner budget exhausted", "increase the explicit count budget or narrow the operating envelope")}
				return finalizeSimulationEvaluationV17(result)
			}
			attempt, diagnoses := evaluateAssertionCornerV17(
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
				return finalizeSimulationEvaluationV17(result)
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
		return finalizeSimulationEvaluationV17(result)
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
	return finalizeSimulationEvaluationV17(result)
}
func evaluateAssertionCornerV17(
	requirement Requirement,
	assertion BehavioralAssertion,
	operatingCase OperatingCase,
	corner operatingCorner,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	cornerBudget ...int,
) (attempt SimulationAttempt, diagnoses []Diagnosis) {
	attempt = SimulationAttempt{
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
	analysis, simulationAssertion, analysisDiagnostics := simulationIntentPartsV17(
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
	attempt.PlanHash = hashJSONV17(plan)
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
	attempt.ReportHash = hashJSONV17(report)
	attempt.Report = &report
	defer func() {
		compact := simmodel.CloneReportWithAnalysisPointLimit(report, v17RetainedReportPoints)
		attempt.Report = &compact
	}()
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
func simulationIntentPartsV17(
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
		analysis.TimeStepS = dynamicTimeStepV17(duration, operatingCase, dynamicResolutionForAssertion(assertion))
		if analysis.DurationS <= 0 || analysis.TimeStepS <= 0 {
			return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{
				Code: diagnosisSimulationInvalid, Path: "simulation.analysis.dynamic_grid",
				Message: "dynamic analysis duration or time step is outside the deterministic supported grid",
			}}
		}
		stepCount := analysis.DurationS / analysis.TimeStepS
		if !finite(stepCount) || stepCount > float64(v17MaximumDynamicTimeSteps)*(1+1e-12) {
			return simmodel.Analysis{}, simmodel.Assertion{}, []SimulationDiagnostic{{
				Code: diagnosisSimulationInvalid, Path: "simulation.analysis.dynamic_grid",
				Message: "dynamic analysis duration or time step is outside the deterministic supported grid",
			}}
		}
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
			simulationAssertion.ReferenceNode = referenceNodeForDomain(requirement, graph, *assertion.Excitation)
		}
	}
	if assertion.Metric == "settling_time" || quantity == simmodel.QuantityResponseTimeS {
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
func dynamicTimeStepV17(duration float64, operatingCase OperatingCase, resolutionS float64) float64 {
	const ticksPerSecond int64 = 1_000_000_000_000
	if duration <= 0 || !finite(duration) {
		return 0
	}
	durationTicks := int64(math.Round(duration * float64(ticksPerSecond)))
	if durationTicks <= 0 {
		return 0
	}
	gridTicks := durationTicks
	for _, event := range operatingCase.Events {
		triggerTicks := int64(math.Round(event.TriggerTimeS * float64(ticksPerSecond)))
		if triggerTicks > 0 && triggerTicks < durationTicks {
			gridTicks = greatestCommonDivisor(gridTicks, triggerTicks)
		}
	}
	targetTicks := max(int64(1), durationTicks/1000)
	if resolutionS > 0 && finite(resolutionS) {
		resolutionTicks := max(int64(1), int64(math.Round(resolutionS*float64(ticksPerSecond))))
		targetTicks = min(targetTicks, resolutionTicks)
	}
	divisor := max(int64(1), (gridTicks+targetTicks-1)/targetTicks)
	alignedStep := float64(gridTicks) / float64(divisor) / float64(ticksPerSecond)
	// Exact event alignment can require a pathologically fine common grid for
	// arbitrary decimal trigger times. Bound the work while retaining exact
	// alignment whenever that grid fits within the deterministic step budget.
	return math.Max(alignedStep, duration/v17MaximumDynamicTimeSteps)
}

func finalizeSimulationEvaluationV17(result SimulationEvaluation) SimulationEvaluation {
	result.Hash = ""
	result.Hash = hashJSONV17(result)
	return result
}
