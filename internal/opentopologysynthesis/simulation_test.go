package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
)

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

func TestHeldOutMetricsHaveGenericTrustedMeasurementContracts(t *testing.T) {
	metrics := []string{
		"bandwidth",
		"cutoff_frequency",
		"falling_threshold",
		"hysteresis",
		"junction_temperature",
		"line_regulation",
		"load_regulation",
		"lower_threshold",
		"off_state_current",
		"on_state_voltage",
		"output_current",
		"output_high_voltage",
		"output_low_voltage",
		"output_noise_rms",
		"output_power",
		"output_swing",
		"output_voltage",
		"peak_voltage",
		"phase_margin",
		"propagation_delay",
		"quiescent_current",
		"rising_threshold",
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
	}
	for _, metric := range metrics {
		if quantity, _, supported := directSimulationQuantity(BehavioralAssertion{Metric: metric}); !supported || quantity == "" {
			t.Errorf("held-out metric %q has no generic trusted measurement contract", metric)
		}
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
