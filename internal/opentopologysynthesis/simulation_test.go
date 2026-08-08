package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestActiveControlValueUsesRisingStartupExcitation(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(multiStageOODCorpusRoot(), "low_voltage_power_with_soft_start.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	active, found := requirementActiveControlValue(requirement, "enable")
	if !found || active != 5 {
		t.Fatalf("active enable = %.12g, %t; want 5 V inferred from the rising startup excitation", active, found)
	}
}

func TestPowerTransferDistortionExcitationUsesFeasiblePowerLoadIntersection(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(multiStageOODCorpusRoot(), "bounded_audio_power_transfer.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	targets := derivePowerTransferSizingTargets(requirement)
	amplitude := excitationAmplitude(requirement, Observation{Kind: "port", ID: "audio_input"})
	peak := amplitude * targets.gain
	minimumPeak := math.Sqrt(2 * 8 * 8)
	maximumPeak := math.Sqrt(2 * 20 * 4)
	if peak < minimumPeak || peak > maximumPeak {
		t.Fatalf("drive peak %.12g V from amplitude %.12g is outside feasible power/load interval %.12g..%.12g V", peak, amplitude, minimumPeak, maximumPeak)
	}
	if amplitude >= .8 {
		t.Fatalf("bounded power stimulus %.12g V reused the port ceiling instead of an interior feasible target", amplitude)
	}
}

func TestDistortionExcitationUsesNominalBiasAcrossSignalCorners(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(multiStageOODCorpusRoot(), "bounded_audio_power_transfer.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "harmonic_limit" {
			assertion = candidate
			break
		}
	}
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "nominal_audio" {
			operatingCase = candidate
			break
		}
	}
	if assertion.ID == "" || operatingCase.ID == "" {
		t.Fatal("bounded audio requirement lacks its distortion assertion or nominal case")
	}
	graph := CandidateGraph{Nodes: []GraphNode{
		{ID: "port_audio_input", Scope: "external", Role: "input", SemanticKind: "port", SemanticID: "audio_input", Domain: "ground"},
		{ID: "port_audio_output", Scope: "external", Role: "output", SemanticKind: "port", SemanticID: "audio_output", Domain: "ground"},
		{ID: "port_ground", Scope: "external", Role: "reference", SemanticKind: "port", SemanticID: "ground", Domain: "ground"},
	}}
	var corner operatingCorner
	for _, candidate := range operatingCaseCorners(operatingCase) {
		if candidate.ID == "bounds_00" {
			corner = candidate
			break
		}
	}
	if corner.ID == "" {
		t.Fatal("nominal audio case lacks its lower signal corner")
	}
	for _, excitation := range simulationExcitations(requirement, assertion, operatingCase, corner, graph) {
		if excitation.Component != "source_port_audio_input" {
			continue
		}
		wantAmplitude := excitationAmplitude(requirement, Observation{Kind: "port", ID: "audio_input"})
		if excitation.DCValue != 0 || excitation.SineFrequencyHz != 1000 ||
			excitation.SineAmplitude != wantAmplitude || excitation.PulseWidthS != 0 {
			t.Fatalf("distortion input excitation = %#v, want zero-bias declared sine", excitation)
		}
		return
	}
	t.Fatal("distortion input excitation is missing")
}

func TestOutputCurrentAssertionUsesImpliedTransconductanceCommand(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "protected_programmable_current_output.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	var settlingAssertion BehavioralAssertion
	var operatingCase OperatingCase
	var commandNode GraphNode
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "rated_current" {
			assertion = candidate
		}
		if candidate.ID == "settling" {
			settlingAssertion = candidate
		}
	}
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "programmed_load" {
			operatingCase = candidate
		}
	}
	for _, candidate := range graph.Nodes {
		if candidate.SemanticID == "current_command" {
			commandNode = candidate
		}
	}
	if assertion.ID == "" || operatingCase.ID == "" || commandNode.ID == "" {
		t.Fatal("protected-current requirement lacks expected semantic evidence")
	}
	corner := operatingCaseCorners(operatingCase)[0]
	if fallback := sourceValueForNode(requirement, operatingCase, corner, commandNode); fallback != 1.125 {
		t.Fatalf("ordinary nominal command = %.12g, want midpoint 1.125", fallback)
	}
	if got := assertionSourceValue(requirement, assertion, operatingCase, corner, commandNode); got != 1 {
		t.Fatalf("assertion-specific command = %.12g, want I/gm = 1", got)
	}

	outside := assertion
	minimum, maximum := 0.25, 0.25
	outside.Min, outside.Max = &minimum, &maximum
	if got := assertionSourceValue(requirement, outside, operatingCase, corner, commandNode); got != 1.125 {
		t.Fatalf("out-of-envelope implied command = %.12g, want fallback midpoint", got)
	}
	excitations := simulationExcitations(requirement, settlingAssertion, operatingCase, corner, graph)
	duration := dynamicDuration(settlingAssertion, operatingCase)
	foundSingleStep := false
	for _, excitation := range excitations {
		if excitation.Component == sourceInstanceForNode(commandNode) &&
			excitation.PulseDelayS > 0 && excitation.PulseWidthS >= duration {
			foundSingleStep = true
		}
	}
	if !foundSingleStep {
		t.Fatalf("settling excitation does not retain one applied step: %#v", excitations)
	}
}

func TestThermalCurrentTransferCoversControlledSourceAndSink(t *testing.T) {
	tests := []struct {
		file          string
		operatingCase string
	}{
		{filepath.Join(architectureGeneralizationCorpusRoot(), "protected_programmable_current_output.json"), "programmed_load"},
		{filepath.Join(protectedCurrentOutputCorpusRoot(), "fault_protected_low_side_current_sink.json"), "enabled_regulation"},
		{filepath.Join(protectedCurrentOutputCorpusRoot(), "startup_safe_high_side_current_source.json"), "permitted_regulation"},
	}
	for _, test := range tests {
		t.Run(filepath.Base(test.file), func(t *testing.T) {
			requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, test.file)))
			if len(issues) != 0 {
				t.Fatalf("requirement decode issues: %#v", issues)
			}
			var operatingCase OperatingCase
			for _, candidate := range requirement.Requirements.OperatingCases {
				if candidate.ID == test.operatingCase {
					operatingCase = candidate
				}
			}
			if operatingCase.ID == "" {
				t.Fatalf("operating case %q is missing", test.operatingCase)
			}
			corner := operatingCaseCorners(operatingCase)[0]
			if dissipation, bounded := thermalBehavioralCurrentTransferDissipation(
				requirement, operatingCase, corner, 12,
			); !bounded || dissipation <= 0 {
				t.Fatalf("controlled-current dissipation = %.12g, bounded=%t", dissipation, bounded)
			}
		})
	}
}

func TestThermalOutputStageDissipationRejectsUnboundedLoads(t *testing.T) {
	for _, load := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if dissipation, bounded := thermalOutputStageDissipation(24, 10, load); bounded || dissipation != 0 {
			t.Fatalf("load %v produced dissipation %v, bounded=%v", load, dissipation, bounded)
		}
	}
	if dissipation, bounded := thermalOutputStageDissipation(24, 10, 8); !bounded || !finite(dissipation) || dissipation <= 0 {
		t.Fatalf("bounded load produced dissipation %v, bounded=%v", dissipation, bounded)
	}
}

func TestBehavioralThermalTransferAndEventDurationAreBounded(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(architectureGeneralizationCorpusRoot(), "regulated_low_voltage_output.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	var lineCase, startupCase OperatingCase
	var startupAssertion BehavioralAssertion
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		switch operatingCase.ID {
		case "line_and_load":
			lineCase = operatingCase
		case "startup":
			startupCase = operatingCase
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "startup_overshoot" {
			startupAssertion = assertion
		}
	}
	if duration := dynamicDuration(startupAssertion, startupCase); duration < 0.01 {
		t.Fatalf("event-driven startup duration=%g, want at least 0.01 s", duration)
	}
	maximum := 0.0
	for _, corner := range operatingCaseCorners(lineCase) {
		if dissipation, bounded := thermalBehavioralTransferDissipation(
			requirement,
			lineCase,
			corner,
			12,
		); bounded {
			maximum = math.Max(maximum, dissipation)
		}
	}
	if maximum < 0.7 || maximum > 0.71 {
		t.Fatalf("behavioral transfer dissipation=%g, want conservative 0.7 W", maximum)
	}
}

func TestBehavioralCurrentTransferThermalEnvelopeUsesDeclaredLoadAndCurrent(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "adjustable_current_output.json")
	var highCommand OperatingCase
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if operatingCase.ID == "high_command" {
			highCommand = operatingCase
			break
		}
	}
	maximum := 0.0
	for _, corner := range operatingCaseCorners(highCommand) {
		rail := 0.0
		for _, condition := range highCommand.Conditions {
			if condition.Axis != "supply_voltage" {
				continue
			}
			rail = corner.Values[conditionKey(condition)]
			if rail <= 0 {
				rail = positiveMidpoint(condition.Min, condition.Max)
			}
		}
		dissipation, bounded := thermalBehavioralCurrentTransferDissipation(
			requirement, highCommand, corner, rail,
		)
		if !bounded {
			t.Fatalf("current-transfer corner %s was not thermally bounded", corner.ID)
		}
		maximum = math.Max(maximum, dissipation)
	}
	if math.Abs(maximum-2.331) > 1e-12 {
		t.Fatalf("current-transfer dissipation=%g, want conservative 2.331 W", maximum)
	}
}

func TestTrustedSimulationEvaluationPassesCornersAndReplaysDeterministically(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	policy := DefaultPolicy()

	first := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	if first.Status != SimulationEvaluationPassed || len(first.Attempts) < 3 || len(first.Diagnoses) != 0 ||
		first.Consumption.CandidateSimulations != len(first.Attempts) ||
		first.Consumption.CornerEvaluations < len(first.Attempts) ||
		len(first.Hash) != 64 {
		t.Fatalf("simulation evaluation = status=%s attempts=%d diagnoses=%#v consumption=%#v issues=%#v", first.Status, len(first.Attempts), first.Diagnoses, first.Consumption, first.Issues)
	}
	for index, attempt := range first.Attempts {
		if attempt.Number != index+1 || attempt.Status != SimulationEvaluationPassed ||
			!attempt.AssertionPass || attempt.Actual == nil ||
			len(attempt.PlanHash) != 64 || len(attempt.ReportHash) != 64 ||
			len(attempt.ModelEvidenceSHA256s) == 0 || attempt.Report == nil {
			t.Fatalf("attempt lacks trusted evidence: %#v", attempt)
		}
	}

	second := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("simulation replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestTrustedSimulationEvaluationReportsStableFailureAndBudgets(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(2000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(2200)
	failed := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	if failed.Status != SimulationEvaluationFailed || len(failed.Diagnoses) == 0 {
		t.Fatalf("failed evaluation = status=%s diagnoses=%#v issues=%#v", failed.Status, failed.Diagnoses, failed.Issues)
	}
	for _, diagnosis := range failed.Diagnoses {
		if diagnosis.Code != diagnosisAssertionBelowMinimum ||
			diagnosis.Actual == nil || diagnosis.EvidenceHash == "" || diagnosis.AffectedConeHash == "" ||
			diagnosis.RequirementID == "" || diagnosis.OperatingCase == "" || diagnosis.Analysis == "" {
			t.Fatalf("incomplete normalized diagnosis: %#v", diagnosis)
		}
	}
	for _, attempt := range failed.Attempts {
		if attempt.CornerID != "nominal" {
			t.Fatalf("nominally failing candidate evaluated tolerance corner %q", attempt.CornerID)
		}
	}

	budgetRequirement, budgetGraph, budgetInventory, budgetEnvironment := testSimulationFixture(t)
	tiny := DefaultPolicy()
	tiny.MaxCandidateSimulations = 1
	tiny.MaxCornerEvaluations = 1
	exhausted := EvaluateCandidate(
		context.Background(),
		budgetRequirement,
		budgetGraph,
		nil,
		budgetInventory,
		budgetEnvironment,
		tiny,
	)
	if exhausted.Status != SimulationEvaluationExhausted || !exhausted.Consumption.BudgetExhausted ||
		len(exhausted.Issues) != 1 || exhausted.Issues[0].Code != CodeSearchExhausted {
		t.Fatalf("exhausted evaluation = status=%s consumption=%#v issues=%#v", exhausted.Status, exhausted.Consumption, exhausted.Issues)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := EvaluateCandidate(ctx, requirement, graph, nil, inventory, environment, DefaultPolicy())
	if canceled.Status != SimulationEvaluationCanceled ||
		len(canceled.Issues) != 1 || canceled.Issues[0].Code != CodeCanceled {
		t.Fatalf("canceled evaluation = status=%s issues=%#v", canceled.Status, canceled.Issues)
	}
}

func TestTransientBehaviorUsesBoundedSemanticExcitation(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "hysteretic_detector.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "propagation_delay" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	excitations := simulationExcitations(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
	)
	foundInputPulse := false
	for _, excitation := range excitations {
		if excitation.Component != "source_port_input" {
			continue
		}
		foundInputPulse = excitation.DCValue == 0 &&
			excitation.PulseInitialValue == 0 &&
			excitation.PulseValue == 5 &&
			excitation.PulseDelayS > 0 &&
			excitation.PulseWidthS > 0 &&
			excitation.PulsePeriodS > excitation.PulseWidthS
	}
	if !foundInputPulse {
		t.Fatalf("transient semantic excitations = %#v", excitations)
	}
}

func TestDynamicGridNormalizesBeforeSemanticValueEvents(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "peak_voltage" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	analysis, _, diagnostics := simulationIntentParts(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
		nil,
		"peak_abs_voltage_v",
		1,
		nil,
	)
	if len(diagnostics) != 0 || len(analysis.SourceValueEvents) != 1 {
		t.Fatalf("dynamic event plan: diagnostics=%#v analysis=%#v", diagnostics, analysis)
	}
	event := analysis.SourceValueEvents[0]
	triggerSteps := event.TriggerTimeS / analysis.TimeStepS
	durationSteps := event.DurationS / analysis.TimeStepS
	if math.Abs(triggerSteps-math.Round(triggerSteps)) > 1e-9 ||
		math.Abs(durationSteps-math.Round(durationSteps)) > 1e-9 ||
		event.TriggerTimeS+event.DurationS > analysis.DurationS+1e-12 {
		t.Fatalf(
			"event is not on the normalized grid: dt=%g duration=%g event=%#v",
			analysis.TimeStepS,
			analysis.DurationS,
			event,
		)
	}
}

func TestDCSweepResolutionTracksAssertionTolerance(t *testing.T) {
	minimum := 10.2
	maximum := 10.6
	assertion := BehavioralAssertion{Min: &minimum, Max: &maximum}
	if points := simulationDCSweepPoints(assertion, 8, 15); points != 256 {
		t.Fatalf("narrow assertion sweep points = %d, want cap 256", points)
	}
	maximum = 20
	if points := simulationDCSweepPoints(assertion, 8, 15); points != 101 {
		t.Fatalf("broad assertion sweep points = %d, want default 101", points)
	}
	maximum = 10.200001
	if points := simulationDCSweepPoints(assertion, 8, 15); points != 256 {
		t.Fatalf("very narrow assertion sweep points = %d, want cap 256", points)
	}
	maximum = math.Nextafter(minimum, math.Inf(1))
	if points := simulationDCSweepPoints(assertion, 8, 15); points != 256 {
		t.Fatalf("machine-resolution assertion sweep points = %d, want cap 256", points)
	}
	assertion.Max = nil
	if points := simulationDCSweepPoints(assertion, 8, 15); points != 101 {
		t.Fatalf("one-sided assertion sweep points = %d, want default 101", points)
	}
}

func TestPeriodicAssertionsOverrideOneShotSourceEvents(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("controlled pulse requirement issues: %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("controlled pulse graph issues: %#v", graphIssues)
	}
	operatingCase := requirement.Requirements.OperatingCases[0]
	corner := operatingCaseCorners(operatingCase)[0]
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	dutyExcitations := simulationExcitations(requirement, assertions["pulse_duty_transfer"], operatingCase, corner, graph)
	dutyAnalysis := simmodel.Analysis{Kind: simmodel.AnalysisTransient, DurationS: .0005, TimeStepS: .5e-6, Excitations: dutyExcitations}
	addSimulationEvents(&dutyAnalysis, requirement, operatingCase, graph)
	periodic := false
	for _, excitation := range dutyAnalysis.Excitations {
		if excitation.Component == "source_port_pulse_command" && excitation.PulsePeriodS == 1.0/20_000 {
			periodic = true
		}
	}
	if !periodic || len(dutyAnalysis.SourceValueEvents) != 0 {
		t.Fatalf("duty-cycle analysis did not own its periodic source: %#v", dutyAnalysis)
	}
	for _, assertionID := range []string{"rising_transition", "falling_transition"} {
		excitations := simulationExcitations(requirement, assertions[assertionID], operatingCase, corner, graph)
		analysis := simmodel.Analysis{Kind: simmodel.AnalysisTransient, DurationS: .0005, TimeStepS: .5e-6, Excitations: excitations}
		addSimulationEvents(&analysis, requirement, operatingCase, graph)
		periodic = false
		for _, excitation := range analysis.Excitations {
			if excitation.Component == "source_port_pulse_command" && excitation.PulsePeriodS == 1.0/20_000 {
				periodic = true
			}
		}
		if !periodic || len(analysis.SourceValueEvents) != 0 {
			t.Fatalf("%s analysis did not own its periodic source: %#v", assertionID, analysis)
		}
	}
}

func TestFrequencyBoundedActiveMetricUsesHeldDigitalActiveStep(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "inductive_load_current_control.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("inductive-control requirement issues: %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("inductive-control graph issues: %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "active_current" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "stress_corners" {
			operatingCase = candidate
			break
		}
	}
	excitations := simulationExcitations(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
	)
	for _, excitation := range excitations {
		if excitation.Component != "source_port_pulse_command" {
			continue
		}
		if excitation.DCValue != 0 || excitation.PulseInitialValue != 0 ||
			excitation.PulseValue != 3.3 || excitation.PulseDelayS <= 0 ||
			excitation.PulseWidthS <= 0 || excitation.PulsePeriodS <= excitation.PulseWidthS ||
			excitation.SineAmplitude != 0 || excitation.SineFrequencyHz != 0 {
			t.Fatalf("frequency-bounded digital active-state excitation = %#v", excitation)
		}
		return
	}
	t.Fatalf("digital active-state excitation is missing: %#v", excitations)
}

func TestTransientOffStateCurrentUsesPostEventPeakWithoutSyntheticPulse(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(protectedCurrentOutputCorpusRoot(), "fault_protected_low_side_current_sink.json"),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "off_state_current" {
			assertion = candidate
			break
		}
	}
	for _, candidate := range requirement.Requirements.OperatingCases {
		if len(assertion.OperatingCases) != 0 && candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	quantity, _, supported := directSimulationQuantity(assertion)
	if !supported || quantity != simmodel.QuantityPeakAbsDeviceCurrentA {
		t.Fatalf("transient off-state quantity = %q supported=%t", quantity, supported)
	}
	corner := operatingCaseCorners(operatingCase)[0]
	for _, excitation := range simulationExcitations(requirement, assertion, operatingCase, corner, graph) {
		if excitation.Component == "source_port_fault" &&
			(excitation.PulseWidthS != 0 || excitation.PulsePeriodS != 0) {
			t.Fatalf("explicit fault event also received a synthetic pulse: %#v", excitation)
		}
	}
	analysis, measurement, diagnostics := simulationIntentParts(
		requirement, assertion, operatingCase, corner, graph, nil, quantity, 1, nil,
	)
	if len(diagnostics) != 0 || len(analysis.SourceValueEvents) != 1 ||
		measurement.WindowStartS != analysis.SourceValueEvents[0].TriggerTimeS ||
		measurement.WindowEndS != analysis.DurationS {
		t.Fatalf("post-event off-state plan: diagnostics=%#v analysis=%#v measurement=%#v",
			diagnostics, analysis, measurement)
	}
}

func TestHeldOutMetricsHaveGenericTrustedMeasurementContracts(t *testing.T) {
	metrics := []string{
		"bandwidth",
		"cutoff_frequency",
		"duty_cycle",
		"fall_time",
		"falling_threshold",
		"hysteresis",
		"junction_temperature",
		"line_regulation",
		"load_regulation",
		"lower_threshold",
		"off_state_current",
		"on_state_voltage",
		"oscillation_frequency",
		"output_current",
		"output_high_voltage",
		"output_low_voltage",
		"output_noise_rms",
		"output_power",
		"output_ripple",
		"output_swing",
		"output_voltage",
		"peak_voltage",
		"phase_margin",
		"propagation_delay",
		"quiescent_current",
		"rising_threshold",
		"rise_time",
		"settling_time",
		"soa_margin",
		"startup_current",
		"startup_output_voltage",
		"startup_overshoot",
		"thd",
		"total_harmonic_distortion",
		"transconductance",
		"transimpedance",
		"upper_threshold",
		"voltage_gain",
		"voltage_gain_at_frequency",
		"conversion_efficiency",
	}
	for _, metric := range metrics {
		if quantity, _, supported := directSimulationQuantity(BehavioralAssertion{Metric: metric}); !supported || quantity == "" {
			t.Errorf("held-out metric %q has no generic trusted measurement contract", metric)
		}
	}
}

func TestOnStateVoltageMeasuresSwitchDropForBothOrientations(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		terminals    []TerminalConnection
		wantPositive string
		wantNegative string
	}{
		{
			name: "low side nmos", kind: "n_channel_mosfet",
			terminals: []TerminalConnection{
				{Terminal: "DRAIN", Node: "OUTPUT"},
				{Terminal: "GATE", Node: "GATE"},
				{Terminal: "SOURCE", Node: "GROUND"},
			},
			wantPositive: "OUTPUT", wantNegative: "GROUND",
		},
		{
			name: "high side pmos", kind: "p_channel_mosfet",
			terminals: []TerminalConnection{
				{Terminal: "DRAIN", Node: "OUTPUT"},
				{Terminal: "GATE", Node: "GATE"},
				{Terminal: "SOURCE", Node: "SUPPLY"},
			},
			wantPositive: "SUPPLY", wantNegative: "OUTPUT",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := CandidateGraph{Instances: []GraphInstance{{
				ID: "switch", Kind: test.kind, Terminals: test.terminals,
			}}}
			positive, negative, found := simulationOnStateVoltageNodes(graph, "OUTPUT")
			if !found || positive != test.wantPositive || negative != test.wantNegative {
				t.Fatalf("on-state voltage nodes = %q - %q found=%t", positive, negative, found)
			}
		})
	}
}

func TestTHDRatioBoundsConvertToSolverPercent(t *testing.T) {
	maximum := 0.005
	assertion := BehavioralAssertion{Metric: "thd", Max: &maximum}
	quantity, scale, supported := directSimulationQuantity(assertion)
	if !supported ||
		quantity != "thd_percent" ||
		scale != 100 {
		t.Fatalf(
			"THD measurement contract = quantity:%q scale:%g supported:%t",
			quantity,
			scale,
			supported,
		)
	}
	minimumPercent, maximumPercent := scaledAssertionBounds(assertion, scale)
	if minimumPercent != 0 || maximumPercent != 0.5 {
		t.Fatalf(
			"THD percent bounds = %g..%g; want 0..0.5",
			minimumPercent,
			maximumPercent,
		)
	}
}

func TestTransientOutputCurrentUsesFinalSolvedCurrent(t *testing.T) {
	quantity, scale, supported := directSimulationQuantity(BehavioralAssertion{
		Metric: "output_current", Analysis: simmodel.AnalysisTransient,
	})
	if !supported || quantity != simmodel.QuantityFinalAbsDeviceCurrentA || scale != 1 {
		t.Fatalf(
			"transient output-current contract = quantity:%q scale:%g supported:%t",
			quantity, scale, supported,
		)
	}
}

func TestPortScopedOffAndStartupCurrentUseObservedActivePath(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(
		context.Background(),
		requirement,
		inventory,
		DefaultPolicy(),
	)
	if len(search.Candidates) == 0 {
		t.Fatalf("controlled-current search produced no candidate: %#v", search)
	}
	graph := search.Candidates[0].Graph
	for _, metric := range []string{"off_state_current", "startup_current"} {
		var assertion BehavioralAssertion
		for _, candidate := range requirement.Requirements.BehavioralRequirements {
			if candidate.Metric == metric {
				assertion = candidate
				break
			}
		}
		if assertion.ID == "" {
			t.Fatalf("%s assertion is missing", metric)
		}
		var operatingCase OperatingCase
		for _, candidate := range requirement.Requirements.OperatingCases {
			if candidate.ID == assertion.OperatingCases[0] {
				operatingCase = candidate
				break
			}
		}
		quantity, _, supported := directSimulationQuantity(assertion)
		if !supported {
			t.Fatalf("%s has no measurement quantity", metric)
		}
		if metric == "off_state_current" &&
			quantity != "device_current_a" {
			t.Fatalf("port-scoped off current quantity = %q", quantity)
		}
		component, _, diagnostic := simulationMeasurementScope(
			requirement,
			assertion,
			operatingCase,
			operatingCaseCorners(operatingCase)[0],
			graph,
			nil,
			quantity,
		)
		if diagnostic != nil {
			t.Fatalf("%s measurement scope: %#v", metric, diagnostic)
		}
		found := false
		for _, instance := range graph.Instances {
			if instance.ID == component &&
				instance.Kind == "n_channel_mosfet" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s current component = %q; graph=%s", metric, component, testGraphTopologySummary(graph))
		}
	}
}

func TestCriticalSimulationEvidenceContinuesAfterNoncriticalNominalFailure(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	for index := range requirement.Requirements.BehavioralRequirements {
		assertion := &requirement.Requirements.BehavioralRequirements[index]
		if assertion.ID == "disabled_leakage" {
			impossible := 1e-12
			assertion.Max = &impossible
		}
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	if len(search.Candidates) == 0 {
		t.Fatalf("controlled-current search produced no candidate: %#v", search)
	}
	plan := BuildValueSearchPlan(requirement, search.Candidates[0].Graph, inventory, DefaultPolicy())
	trials := EnumerateValueTrials(plan, 1).Trials
	if len(trials) != 1 {
		t.Fatalf("value trials = %d, plan=%#v", len(trials), plan)
	}
	graph, err := ApplyValueTrial(search.Candidates[0].Graph, trials[0], inventory)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, DefaultPolicy())
	foundNoncriticalFailure := false
	foundCriticalEvidence := false
	for _, attempt := range evaluation.Attempts {
		if attempt.CornerID != "nominal" {
			t.Fatalf("nominally rejected candidate continued into stress corners: %#v", evaluation.Attempts)
		}
		if attempt.RequirementID == "disabled_leakage" && attempt.Status == SimulationEvaluationFailed {
			foundNoncriticalFailure = true
		}
		if attempt.RequirementID == "startup_off" {
			foundCriticalEvidence = true
		}
	}
	if !foundNoncriticalFailure || !foundCriticalEvidence {
		t.Fatalf("critical evidence did not survive a noncritical nominal failure: %#v", evaluation.Attempts)
	}
}

func TestLoadCurrentHarnessAndCrossCaseSweepAreCatalogBacked(t *testing.T) {
	_, _, _, environment := testSimulationFixture(t)
	requirement := testOpenTopologyRequirement(t, "adjustable_voltage_regulation.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "load_regulation" {
			assertion = candidate
			break
		}
	}
	if assertion.ID == "" {
		t.Fatal("load-regulation assertion is missing")
	}
	operatingCase := requirement.Requirements.OperatingCases[0]
	corner := operatingCaseCorners(operatingCase)[0]
	harness, hashes, diagnostics := simulationHarness(requirement, assertion, operatingCase, corner, graph, environment)
	if len(diagnostics) != 0 || len(hashes) == 0 {
		t.Fatalf("load-current harness diagnostics=%#v hashes=%#v", diagnostics, hashes)
	}
	loadID := loadInstanceID("output_power", "load_current")
	found := false
	for _, component := range harness {
		if component.InstanceID == loadID && component.Family == "current_source" && !component.HasValueSI {
			found = true
		}
	}
	if !found {
		t.Fatalf("load-current harness omitted reviewed current source %q: %#v", loadID, harness)
	}
	source, start, stop, ok := sweepSourceAndRange(requirement, assertion, operatingCase, corner, graph)
	if !ok || source != loadID || start != .005 || stop != .1 {
		t.Fatalf("load-regulation sweep = %q %.12g..%.12g ok=%t", source, start, stop, ok)
	}
	excitations := simulationExcitations(requirement, assertion, operatingCase, corner, graph)
	found = false
	for _, excitation := range excitations {
		if excitation.Component == loadID && excitation.DCValue == .005 {
			found = true
		}
	}
	if !found {
		t.Fatalf("load-current excitation is missing: %#v", excitations)
	}
}

func TestLoadResistanceHarnessProvidesLoadRegulationSweep(t *testing.T) {
	_, _, _, environment := testSimulationFixture(t)
	requirement := testOpenTopologyRequirement(t, "adjustable_voltage_regulation.json")
	for caseIndex := range requirement.Requirements.OperatingCases {
		for conditionIndex := range requirement.Requirements.OperatingCases[caseIndex].Conditions {
			condition := &requirement.Requirements.OperatingCases[caseIndex].Conditions[conditionIndex]
			if condition.Axis == "load_current" {
				condition.Axis = "load_resistance"
				condition.Min = 5
				condition.Max = 10
				condition.Unit = "ohm"
			}
		}
	}
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "load_regulation" {
			assertion = candidate
			break
		}
	}
	operatingCase := requirement.Requirements.OperatingCases[0]
	corner := operatingCaseCorners(operatingCase)[0]
	harness, hashes, diagnostics := simulationHarness(requirement, assertion, operatingCase, corner, graph, environment)
	if len(diagnostics) != 0 || len(hashes) == 0 {
		t.Fatalf("load-resistance harness diagnostics=%#v hashes=%#v", diagnostics, hashes)
	}
	loadID := loadInstanceID("output_power", "load_resistance")
	found := false
	for _, component := range harness {
		if component.InstanceID == loadID && component.Family == "resistor" && component.HasValueSI {
			found = true
		}
	}
	if !found {
		t.Fatalf("load-resistance harness omitted reviewed resistor %q: %#v", loadID, harness)
	}
	source, start, stop, ok := sweepSourceAndRange(requirement, assertion, operatingCase, corner, graph)
	if !ok || source != loadID || start != 5 || stop != 10 {
		t.Fatalf("load-regulation resistance sweep = %q %.12g..%.12g ok=%t", source, start, stop, ok)
	}
	if !dcSweepUsesDeviceValue(requirement, assertion, source) {
		t.Fatal("load-resistance sweep was not classified as a device-value sweep")
	}
	excitations := simulationExcitations(requirement, assertion, operatingCase, corner, graph)
	for _, excitation := range excitations {
		if excitation.Component == loadID {
			t.Fatalf("load resistor was incorrectly emitted as an independent source: %#v", excitations)
		}
	}
}

func TestCurrentPortClassificationIncludesControlledOutputs(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(multiStageOODCorpusRoot(), "enabled_current_regulation.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	if !observationIsCurrentPort(requirement, Observation{Kind: "port", ID: "regulated_current"}) {
		t.Fatal("controlled-current output was not classified as a current observation")
	}
	if observationIsCurrentPort(requirement, Observation{Kind: "port", ID: "current_command"}) {
		t.Fatal("analog-voltage input was incorrectly classified as a current observation")
	}
}

func TestDCSweepInheritsUniqueCrossCaseExcitationRange(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t, filepath.Join(multiStageOODCorpusRoot(), "enabled_current_regulation.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "transconductance" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "regulation_corners" {
			operatingCase = candidate
			break
		}
	}
	corner := operatingCaseCorners(operatingCase)[0]
	source, start, stop, ok := sweepSourceAndRange(requirement, assertion, operatingCase, corner, graph)
	if !ok || source != "source_port_current_command" || start != .2 || stop != 1.8 {
		t.Fatalf("cross-case command sweep = %q %.12g..%.12g ok=%t", source, start, stop, ok)
	}
}

func TestLineRegulationSweepUsesUniqueDeclaredSupplyRange(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues = %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "line_regulation" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	source, start, stop, ok := sweepSourceAndRange(
		requirement, assertion, operatingCase, operatingCaseCorners(operatingCase)[0], graph,
	)
	if !ok || source != "source_port_input_power" || start != 10 || stop != 14 {
		t.Fatalf("line-regulation sweep = %q %.12g..%.12g ok=%t", source, start, stop, ok)
	}
}

func TestActiveOutputAssertionsInheritEnableEnvelopeWithoutChangingStartup(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "efficient_step_down_power.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	conditions := simulationHarnessConditions(requirement, assertions["line_regulation"], cases["line_load_corners"])
	foundEnable := false
	for _, condition := range conditions {
		foundEnable = foundEnable || condition.Axis == "input_voltage" && condition.Target == "enable" && condition.Min == 3 && condition.Max == 5
	}
	if !foundEnable {
		t.Fatalf("active line-regulation conditions omit enable envelope: %#v", conditions)
	}
	startupConditions := simulationHarnessConditions(requirement, assertions["startup_time"], cases["nominal_load"])
	enableConditions := 0
	for _, condition := range startupConditions {
		if condition.Target == "enable" && condition.Axis == "input_voltage" {
			enableConditions++
		}
	}
	if enableConditions != 1 {
		t.Fatalf("startup enable conditions = %#v", startupConditions)
	}
}

func TestActiveOutputAssertionsInheritCrossCaseActivationEvent(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		multiStageOODCorpusRoot(), "enabled_current_regulation.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	cases := map[string]OperatingCase{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		cases[operatingCase.ID] = operatingCase
	}
	enable := GraphNode{ID: "port_enable", SemanticID: "enable", Scope: "external", Role: "control"}
	regulationCorner := operatingCaseCorners(cases["regulation_corners"])[0]
	if got := assertionSourceValue(
		requirement, assertions["command_transfer"], cases["regulation_corners"], regulationCorner, enable,
	); got != 5 {
		t.Fatalf("active steady-state enable = %.12g V, want cross-case applied state 5 V", got)
	}
	commandCorner := operatingCaseCorners(cases["command_range"])[0]
	if got := assertionSourceValue(
		requirement, assertions["command_transfer"], cases["command_range"], commandCorner, enable,
	); got != 5 {
		t.Fatalf("same-case DC enable = %.12g V, want applied state 5 V", got)
	}
	if got := assertionSourceValue(
		requirement, assertions["disabled_current"], cases["command_range"], commandCorner, enable,
	); got != 0 {
		t.Fatalf("off-state enable = %.12g V, want inactive 0 V", got)
	}
	if got := assertionSourceValue(
		requirement, assertions["enable_settling"], cases["command_range"], commandCorner, enable,
	); got != 0 {
		t.Fatalf("transient initial enable = %.12g V, want local event initial state 0 V", got)
	}

	thermalCase := cases["regulation_corners"]
	thermalCase.Conditions = simulationHarnessConditions(
		requirement, assertions["safe_temperature"], thermalCase,
	)
	regulationCorner.Values[conditionKey(OperatingCondition{
		Axis: "load_resistance", Target: "regulated_current",
	})] = 20
	if dissipation, bounded := thermalBehavioralCurrentTransferDissipation(
		requirement, thermalCase, regulationCorner, 9,
	); !bounded || dissipation <= 0 {
		t.Fatalf("cross-case current-transfer dissipation = %.12g, bounded=%t", dissipation, bounded)
	}
}

func TestDecisionInputStateAssertionsUseSemanticExtremes(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		multiStageOODCorpusRoot(), "undervoltage_load_permission.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues = %#v", graphIssues)
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "battery_sweep" {
			operatingCase = candidate
			break
		}
	}
	var input GraphNode
	for _, node := range graph.Nodes {
		if node.SemanticID == "battery_level" {
			input = node
			break
		}
	}
	if input.ID == "" {
		t.Fatal("initial graph lacks battery-level decision input")
	}
	corner := operatingCaseCorners(operatingCase)[0]
	if got := assertionSourceValue(requirement, assertions["active_loss"], operatingCase, corner, input); got != 15 {
		t.Fatalf("on-state decision input = %.12g V, want active maximum 15 V", got)
	}
	if got := assertionSourceValue(requirement, assertions["disconnected_leakage"], operatingCase, corner, input); got != 8 {
		t.Fatalf("off-state decision input = %.12g V, want inactive minimum 8 V", got)
	}
	if got := assertionSourceValue(requirement, assertions["disconnected_start"], operatingCase, corner, input); got != 9 {
		t.Fatalf("startup decision input = %.12g V, want event initial value 9 V", got)
	}
}

func TestQuiescentCurrentRemovesLoadHarnessAndExcitationTogether(t *testing.T) {
	_, _, _, environment := testSimulationFixture(t)
	requirement := testProtectedVoltageOutputRequirement(t, "protected_high_power_voltage_output.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "quiescent_current" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	corner := operatingCaseCorners(operatingCase)[0]
	harness, _, diagnostics := simulationHarness(
		requirement, assertion, operatingCase, corner, graph, environment,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("quiescent-current harness diagnostics=%#v", diagnostics)
	}
	loadID := loadInstanceID("power_output", "load_current")
	declared := map[string]bool{}
	for _, component := range harness {
		declared[component.InstanceID] = true
	}
	if declared[loadID] {
		t.Fatalf("quiescent-current harness retained load %q", loadID)
	}
	for _, excitation := range simulationExcitations(requirement, assertion, operatingCase, corner, graph) {
		if excitation.Component == loadID {
			t.Fatalf("quiescent-current excitation retained omitted load %q", loadID)
		}
		if !declared[excitation.Component] {
			t.Fatalf("quiescent-current excitation %q is absent from harness %#v", excitation.Component, harness)
		}
	}
}

func TestDynamicTimeStepAlignsEventAndDurationGrid(t *testing.T) {
	duration := .3
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: .001}}}
	step := dynamicTimeStep(duration, operatingCase)
	if step <= 0 || step > duration/1000 {
		t.Fatalf("dynamic time step = %.12g", step)
	}
	for name, value := range map[string]float64{"duration": duration, "event": operatingCase.Events[0].TriggerTimeS} {
		steps := value / step
		if math.Abs(steps-math.Round(steps)) > 1e-9 {
			t.Fatalf("%s %.12g is not aligned to step %.12g", name, value, step)
		}
	}
}

func TestDynamicTimeStepBoundsPathologicalEventGrid(t *testing.T) {
	duration := .3
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: .001000000001}}}
	step := dynamicTimeStep(duration, operatingCase)
	minimum := duration / maximumDynamicTimeSteps
	if step < minimum || duration/step > maximumDynamicTimeSteps*(1+1e-12) {
		t.Fatalf("dynamic time step = %.12g, minimum %.12g", step, minimum)
	}
}

func TestAnalogCurrentInputHarnessUsesCatalogCurrentSource(t *testing.T) {
	data := mustRead(t, filepath.Join(architectureGeneralizationCorpusRoot(), "low_current_voltage_converter.json"))
	requirement, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	if len(search.Candidates) == 0 {
		t.Fatalf("current-input search produced no candidate: %#v", search)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "phase_margin" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	corner := operatingCaseCorners(operatingCase)[0]
	harness, _, diagnostics := simulationHarness(
		requirement, assertion, operatingCase, corner,
		search.Candidates[0].Graph, environment,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("current-input harness diagnostics: %#v", diagnostics)
	}
	sourceID := sourceInstanceForObservation(search.Candidates[0].Graph, Observation{Kind: "port", ID: "signal_current"})
	found := false
	for _, component := range harness {
		if component.InstanceID != sourceID {
			continue
		}
		found = component.Family == "current_source"
		connections := map[string]string{}
		for _, connection := range component.Connections {
			connections[connection.Function] = connection.Net
		}
		if connections["POSITIVE"] != "port_ground" || connections["NEGATIVE"] != "port_signal_current" {
			t.Fatalf("current-input source orientation = %#v", connections)
		}
	}
	if !found {
		t.Fatalf("current-input harness lacks catalog current source %q: %#v", sourceID, harness)
	}
}

func TestFixedExcitationUsesBoundedLocalSweep(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "adjustable_current_output.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "transconductance" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "low_command" {
			operatingCase = candidate
			break
		}
	}
	source, start, stop, ok := sweepSourceAndRange(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
	)
	if !ok || source != "source_port_setpoint" ||
		math.Abs(start-.5) > 1e-12 || math.Abs(stop-.65) > 1e-12 {
		t.Fatalf(
			"local excitation sweep = %q %.12g..%.12g ok=%t",
			source,
			start,
			stop,
			ok,
		)
	}
}

func TestControlledCurrentSinkLoadHarnessUsesDominantExternalSupply(t *testing.T) {
	_, _, _, environment := testSimulationFixture(t)
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "on_state_voltage" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	harness, _, diagnostics := simulationHarness(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
		environment,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("controlled-current harness diagnostics: %#v", diagnostics)
	}
	loadID := loadInstanceID("load_return", "load_current")
	for _, component := range harness {
		if component.InstanceID != loadID {
			continue
		}
		connections := map[string]string{}
		for _, connection := range component.Connections {
			connections[connection.Function] = connection.Net
		}
		if connections["POSITIVE"] != "port_load_power" ||
			connections["NEGATIVE"] != "port_load_return" {
			t.Fatalf("controlled-current load connections = %#v", connections)
		}
		return
	}
	t.Fatalf("controlled-current load harness %q is missing: %#v", loadID, harness)
}

func TestLoadHarnessRecognizesLowSideSwitchThroughCurrentShunt(t *testing.T) {
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	targetID, found := ExternalNodeForObservation(
		graph,
		requirement,
		Observation{Kind: "port", ID: "load_return"},
	)
	if !found {
		t.Fatal("load-return graph node is missing")
	}
	reference := referenceNodeForDomain(graph, Observation{Kind: "port", ID: "load_return"})
	if reference == "" {
		t.Fatal("reference graph node is missing")
	}
	var target GraphNode
	for _, node := range graph.Nodes {
		if node.ID == targetID {
			target = node
			break
		}
	}
	// Suppress the controlled-current-port fallback so this regression proves
	// that the topology itself identifies the load-side harness orientation.
	target.SemanticID = ""
	graph.Nodes = append(graph.Nodes, GraphNode{ID: "internal_current_sense", Scope: "internal", Role: "signal"})
	graph.Instances = append(graph.Instances,
		GraphInstance{
			ID:   "switch",
			Kind: "n_channel_mosfet",
			Terminals: []TerminalConnection{
				{Terminal: "DRAIN", Node: targetID},
				{Terminal: "GATE", Node: "internal_gate"},
				{Terminal: "SOURCE", Node: "internal_current_sense"},
			},
		},
		GraphInstance{
			ID:   "current_shunt",
			Kind: "resistor",
			Terminals: []TerminalConnection{
				{Terminal: "1", Node: "internal_current_sense"},
				{Terminal: "2", Node: reference},
			},
		},
	)
	positive, negative := loadHarnessNodes(requirement, graph, target, reference)
	if positive != "port_load_power" || negative != targetID {
		t.Fatalf("shunt-sensed low-side harness = %q -> %q", positive, negative)
	}
}

func TestInductiveHarnessUsesBoundedExternalLoadAndBehavioralWindingResistance(t *testing.T) {
	_, _, _, environment := testSimulationFixture(t)
	requirement := testOpenTopologyRequirement(t, "ground_referenced_load_control.json")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.Metric == "peak_voltage" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == assertion.OperatingCases[0] {
			operatingCase = candidate
			break
		}
	}
	harness, _, diagnostics := simulationHarness(
		requirement,
		assertion,
		operatingCase,
		operatingCaseCorners(operatingCase)[0],
		graph,
		environment,
	)
	if len(diagnostics) != 0 {
		t.Fatalf("inductive harness diagnostics: %#v", diagnostics)
	}
	loadID := loadInstanceID("load_return", "load_inductance")
	for _, component := range harness {
		if component.InstanceID != loadID {
			continue
		}
		if component.CatalogID != "load.inductive.external.1x02" {
			t.Fatalf("inductive harness catalog = %q", component.CatalogID)
		}
		for _, claim := range component.ModelClaims {
			for _, parameter := range claim.Parameters {
				if parameter.Name == "series_resistance_ohm" {
					if math.Abs(parameter.Value-12) > 1e-12 {
						t.Fatalf("derived winding resistance = %g; want 12", parameter.Value)
					}
					return
				}
			}
		}
		t.Fatalf("inductive harness lacks series-resistance evidence: %#v", component)
	}
	t.Fatalf("inductive load harness %q is missing: %#v", loadID, harness)
}

func TestCircuitStressAssertionsInheritLoadEnvelopeAndExpandItsCorners(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "safe_temperature" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "electrical_corners" {
			operatingCase = candidate
			break
		}
	}
	conditions := simulationHarnessConditions(requirement, assertion, operatingCase)
	haveResistance, haveInductance := false, false
	for _, condition := range conditions {
		haveResistance = haveResistance || condition.Axis == "load_resistance" && condition.Target == "load_output"
		haveInductance = haveInductance || condition.Axis == "load_inductance" && condition.Target == "load_output"
	}
	if !haveResistance || !haveInductance {
		t.Fatalf("circuit stress conditions = %#v", conditions)
	}
	operatingCase.Conditions = conditions
	seen := map[string]bool{}
	for _, corner := range operatingCaseCorners(operatingCase) {
		resistance := corner.Values[conditionKey(OperatingCondition{Axis: "load_resistance", Target: "load_output"})]
		inductance := corner.Values[conditionKey(OperatingCondition{Axis: "load_inductance", Target: "load_output"})]
		seen[fmt.Sprintf("%.0f/%.3f", resistance, inductance)] = true
	}
	for _, key := range []string{"16/0.000", "16/0.002", "24/0.000", "24/0.002"} {
		if !seen[key] {
			t.Fatalf("inherited load corner %s missing from %v", key, seen)
		}
	}
}

func TestZeroInductanceCornerMeasuresPresentResistiveLoad(t *testing.T) {
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(t, filepath.Join(
		nonlinearSwitchingCorpusRoot(), "controlled_pulse_power_stage.json",
	))))
	if len(issues) != 0 {
		t.Fatalf("requirement issues = %#v", issues)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues = %#v", graphIssues)
	}
	var assertion BehavioralAssertion
	for _, candidate := range requirement.Requirements.BehavioralRequirements {
		if candidate.ID == "off_state_leakage" {
			assertion = candidate
			break
		}
	}
	var operatingCase OperatingCase
	for _, candidate := range requirement.Requirements.OperatingCases {
		if candidate.ID == "nominal_pulses" {
			operatingCase = candidate
			break
		}
	}
	var corner operatingCorner
	for _, candidate := range operatingCaseCorners(operatingCase) {
		if candidate.Values[conditionKey(OperatingCondition{Axis: "load_inductance", Target: "load_output"})] == 0 {
			corner = candidate
			break
		}
	}
	component, found := loadMeasurementComponent(requirement, assertion, operatingCase, corner, graph)
	if !found || component != loadInstanceID("load_output", "load_resistance") {
		t.Fatalf("zero-inductance current component = %q found=%t", component, found)
	}
}

func testSimulationFixture(t *testing.T) (Requirement, CandidateGraph, PrimitiveInventory, SimulationEnvironment) {
	t.Helper()
	requirement := testOpenTopologyRequirement(t, "powered_lowpass.json")
	var originalAssertion BehavioralAssertion
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "cutoff_frequency" {
			originalAssertion = assertion
			break
		}
	}
	if originalAssertion.ID == "" {
		t.Fatal("powered-lowpass cutoff assertion is missing")
	}
	requirement.Requirements.BehavioralRequirements = []BehavioralAssertion{originalAssertion}
	operatingCase := requirement.Requirements.OperatingCases[0]
	operatingCase.Conditions = operatingCase.Conditions[:1]
	requirement.Requirements.OperatingCases = []OperatingCase{operatingCase}

	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model-provenance diagnostics: %#v", diagnostics)
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := BuildPrimitiveInventory(catalog, catalogHash, registry)
	if len(issues) != 0 {
		t.Fatalf("primitive inventory issues: %#v", issues)
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	resistor, resistorFound := valuePrimitiveAt(inventory, requirement, requiredAnalyses, "resistor", 10_000)
	capacitor, capacitorFound := valuePrimitiveAt(inventory, requirement, requiredAnalyses, "capacitor", 15e-9)
	if !resistorFound || !capacitorFound {
		t.Fatalf("default inventory lacks simulation passives: resistor=%t capacitor=%t", resistorFound, capacitorFound)
	}
	graph, graphIssues := InitialGraph(requirement)
	if len(graphIssues) != 0 {
		t.Fatalf("initial graph issues: %#v", graphIssues)
	}
	graph = AddPrimitive(graph, resistor, graphFloat(10_000), []TerminalConnection{
		{Terminal: "A", Node: "port_input"},
		{Terminal: "B", Node: "port_output"},
	})
	graph = AddPrimitive(graph, capacitor, graphFloat(15e-9), []TerminalConnection{
		{Terminal: "A", Node: "port_output"},
		{Terminal: "B", Node: "port_ground"},
	})
	graph = AddPrimitive(graph, resistor, graphFloat(10_000), []TerminalConnection{
		{Terminal: "A", Node: "port_power"},
		{Terminal: "B", Node: "port_ground"},
	})
	return requirement, graph, inventory, SimulationEnvironment{
		Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: registry,
	}
}

func valuePrimitiveAt(
	inventory PrimitiveInventory,
	requirement Requirement,
	requiredAnalyses map[string]bool,
	kind string,
	value float64,
) (PrimitiveCandidate, bool) {
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) ||
			!valueWithinPrimitiveDomain(value, *primitive.ValueDomain) {
			continue
		}
		if _, proven := primitiveTolerancePercent(primitive, primitive.ValueDomain.Kind); !proven {
			continue
		}
		tolerance, _ := primitiveTolerancePercent(primitive, primitive.ValueDomain.Kind)
		if tolerance > 1 {
			continue
		}
		return primitive, true
	}
	return PrimitiveCandidate{}, false
}
