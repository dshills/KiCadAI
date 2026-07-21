package simmodel

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestPeriodicTransientMeasurementsUseSettledTwoCycleWindow(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisTransient, FundamentalFrequencyHz: 1}
	for index := 0; index <= 12; index++ {
		voltage, powerVoltage := .02, 2.0
		if index < 4 {
			voltage, powerVoltage = 100, 100
		}
		result.Points = append(result.Points, AnalysisPoint{
			TimeS:   float64(index) * .25,
			Nodes:   []NodeResult{{Node: "OUT", Real: voltage}},
			Devices: []DeviceResult{{Component: "LOAD", VoltageV: powerVoltage, CurrentA: 2}},
		})
	}
	peak, diagnostic := peakAbsVoltage(result, Assertion{AnalysisID: "transient", Node: "OUT"})
	if diagnostic != nil || peak != .02 {
		t.Fatalf("peakAbsVoltage() = %.12g, %#v; want settled 0.02 V", peak, diagnostic)
	}
	power, diagnostic := transientDerivedValue(result, Assertion{AnalysisID: "transient", Component: "LOAD", Quantity: QuantityOutputPowerW})
	if diagnostic != nil || power != 4 {
		t.Fatalf("output power = %.12g, %#v; want settled 4 W", power, diagnostic)
	}
}

func TestTransientAnalysisWorkersPreserveOrderAndReplay(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	second := cloneAnalyses(plan.Analyses)[0]
	second.ID = "z_switch"
	plan.Analyses = append(plan.Analyses, second)
	secondAssertions := append([]Assertion(nil), plan.Assertions...)
	for index := range secondAssertions {
		secondAssertions[index].AnalysisID = second.ID
	}
	plan.Assertions = append(plan.Assertions, secondAssertions...)
	first, firstDiagnostics := Evaluate(plan)
	secondReport, secondDiagnostics := Evaluate(ClonePlan(plan))
	if len(firstDiagnostics) != 0 || len(secondDiagnostics) != 0 {
		t.Fatalf("parallel transient diagnostics: first=%#v second=%#v", firstDiagnostics, secondDiagnostics)
	}
	if len(first.Analyses) != 2 || first.Analyses[0].ID != "switch" || first.Analyses[1].ID != "z_switch" {
		t.Fatalf("parallel transient result order = %#v", first.Analyses)
	}
	if !reflect.DeepEqual(first, secondReport) {
		t.Fatal("parallel transient replay is not deterministic")
	}
}

func TestNonlinearControlUpdateIgnoresCommonModeMovement(t *testing.T) {
	system := mnaSystem{nodeIndex: map[string]int{"B": 0, "C": 1, "E": 2}}
	devices := []compiledNonlinearDevice{{primitive: PrimitiveBJTNPNV1, terminals: map[string]string{"BASE": "B", "COLLECTOR": "C", "EMITTER": "E"}}}
	before := []complex128{.6, 0, 0}
	commonMode := []complex128{10.6, 10, 10}
	if update := maxNonlinearControlVoltageUpdate(devices, &system, before, commonMode); update > 1e-12 {
		t.Fatalf("common-mode nonlinear control update = %.12g", update)
	}
	changedJunction := []complex128{10.8, 10, 10}
	if update := maxNonlinearControlVoltageUpdate(devices, &system, before, changedJunction); math.Abs(update-.2) > 1e-12 {
		t.Fatalf("junction nonlinear control update = %.12g", update)
	}
}

func TestTransientPeriodicZeroCrossingUsesOperatingPointSeed(t *testing.T) {
	analysis := Analysis{Excitations: []SourceExcitation{{Component: "input", SineAmplitude: 1, SineFrequencyHz: 1000, SinePhaseDeg: 180}}}
	previous := []complex128{2, -3}
	history := [][]complex128{{.1, -.2}, previous}
	guess := transientInitialGuess(analysis, .0005, previous, history)
	if guess[0] != .1 || guess[1] != -.2 {
		t.Fatalf("zero-crossing guess=%#v", guess)
	}
	guess[0] = 99
	if history[0][0] != .1 {
		t.Fatal("initial guess aliases the accepted operating point")
	}
	ordinary := transientInitialGuess(analysis, .00025, previous, history)
	if ordinary[0] != 2 || ordinary[1] != -3 {
		t.Fatalf("ordinary guess=%#v", ordinary)
	}
}

func TestTransientOutputLimitGuessInterpolatesBracketSolutions(t *testing.T) {
	base := mnaSystem{nodeIndex: map[string]int{"OUT": 0, "OTHER": 1}}
	devices := []ResolvedDevice{{Component: "amp", PrimitiveModel: PrimitiveOpAmpV1, Terminals: []TerminalBinding{{Terminal: "OUT", Net: "OUT"}}}}
	guess := make([]complex128, 2)
	next := map[string]transientOutputLimitState{"amp": {
		value: 2.5, lower: 0, upper: 10,
		lowerSolution: []complex128{0, 4}, upperSolution: []complex128{10, 8},
	}}
	seedTransientOutputLimitGuess(base, devices, guess, nil, next)
	if guess[0] != 2.5 || guess[1] != 5 {
		t.Fatalf("interpolated transient output-limit guess = %#v", guess)
	}
}

func TestNormallyOpenRelayIsolatesStartupAndClosesForNormalAnalyses(t *testing.T) {
	parameters := []NamedValue{
		{Name: "coil_resistance_ohm", Value: 720}, {Name: "contact_off_resistance_ohm", Value: 1e12},
		{Name: "contact_on_resistance_ohm", Value: .05}, {Name: "max_contact_current_a", Value: 5},
		{Name: "max_contact_voltage_v", Value: 30}, {Name: "operate_current_a", Value: .005}, {Name: "operate_delay_s", Value: .01},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{
			{ID: "muted", Kind: AnalysisTransient, DurationS: 10e-6, TimeStepS: 1e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 0}, {Component: "source", DCValue: 5}}},
			{ID: "normal", Kind: AnalysisTransient, DurationS: 10e-6, TimeStepS: 1e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 5}, {Component: "source", DCValue: 5}}},
			{ID: "startup", Kind: AnalysisStartup, DurationS: 100e-6, TimeStepS: 10e-6, Excitations: []SourceExcitation{{Component: "control", DCValue: 5}, {Component: "source", DCValue: 5}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "muted", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 0, Max: 1e-6},
			{AnalysisID: "normal", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 4.99, Max: 5.01},
			{AnalysisID: "startup", Node: "OUT", Quantity: QuantityPeakAbsVoltageV, Min: 0, Max: 1e-6},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "control", CatalogID: "source", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "CONTROL"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "relay", CatalogID: "relay", Family: "relay", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveRelayNormallyOpenV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "COIL_A", Net: "CONTROL"}, {Function: "COIL_B", Net: "GND"}, {Function: "CONTACT_IN", Net: "IN"}, {Function: "CONTACT_OUT", Net: "OUT"}}},
		{InstanceID: "load", CatalogID: "resistor", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "CONTROL"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientNPNSwitchWaveformIsDeterministic(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 301 {
		t.Fatalf("transient points=%d", len(report.Analyses[0].Points))
	}
	lastEvidence := report.Analyses[0].Points[len(report.Analyses[0].Points)-1].Solver
	if lastEvidence == nil || lastEvidence.Method != "backward_euler_bounded_newton_v1" || lastEvidence.TimeSteps != 300 || lastEvidence.TotalIterations <= 0 || lastEvidence.MaxIterationsPerStep != transientMaxNewtonIterations*transientMaxNewtonAttemptsPerObservation || lastEvidence.MaxTotalIterations != maxTransientWork {
		t.Fatalf("transient evidence=%+v", lastEvidence)
	}
	first, _ := json.Marshal(report)
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics=%+v", replayDiagnostics)
	}
	second, _ := json.Marshal(replay)
	if string(first) != string(second) {
		t.Fatal("transient replay is not byte-identical")
	}
}

func TestTransientDiodeAndPNPSwitching(t *testing.T) {
	for _, test := range []struct {
		name       string
		intent     Intent
		components []ComponentEvidence
		nodes      []NodeEvidence
	}{
		{name: "diode", intent: transientDiodeIntent(), components: transientDiodeComponents(), nodes: []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "SOURCE"}, {Name: "OUT"}}},
		{name: "pnp", intent: transientPNPIntent(), components: transientPNPComponents(), nodes: transientSwitchNodes()},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, diagnostics := ResolveWithTopology(test.intent, "test", "catalog-hash", test.components, test.nodes)
			if len(diagnostics) != 0 {
				t.Fatalf("resolve diagnostics=%+v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
			}
		})
	}
}

func TestTransientOpenCollectorComparatorAppliesCatalogDelay(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisTransient, DurationS: 9e-6, TimeStepS: 1e-6,
			Excitations: []SourceExcitation{
				{Component: "signal", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 5e-6, PulseWidthS: 3e-6, PulsePeriodS: 20e-6},
				{Component: "supply", DCValue: 5},
				{Component: "threshold", DCValue: 2.5},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 7e-6, Min: .10, Max: .12},
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 8e-6, Min: 4.99, Max: 5},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "threshold", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "THRESH"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "pullup", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 10000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VP"}, {Function: "B", Net: "OUT"}}},
		{InstanceID: "comparator", CatalogID: "comparator", Family: "comparator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveComparatorOpenCollectorV1, Parameters: comparatorParameters(2e-6)}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "THRESH"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "THRESH"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientPNPResistiveSwitchDoesNotRequireCapacitiveState(t *testing.T) {
	components := transientPNPComponents()
	for index, component := range components {
		if component.InstanceID == "load" {
			components = append(components[:index], components[index+1:]...)
			break
		}
	}
	intent := transientPNPIntent()
	intent.Assertions = []Assertion{
		{AnalysisID: "switch", Node: "VCC", Quantity: QuantityVoltageV, TimeS: 0, Min: 4.99, Max: 5.01},
		{AnalysisID: "switch", Node: "DRIVE", Quantity: QuantityVoltageV, TimeS: .0002, Min: -.01, Max: .01},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestTransientBoundsClaimsAndOperatingLimitsFailClosed(t *testing.T) {
	plan := resolveTransientSwitchPlan(t, 25)
	badGrid := transientSwitchIntent()
	badGrid.Analyses[0].DurationS = .003001
	if _, diagnostics := ResolveWithTopology(badGrid, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exact integer grid") {
		t.Fatalf("grid diagnostics=%+v", diagnostics)
	}
	tooMuchWork := transientSwitchIntent()
	tooMuchWork.Analyses[0].DurationS = .02049
	tooMuchWork.Analyses[0].Excitations[1].PulsePeriodS = .03
	if _, diagnostics := ResolveWithTopology(tooMuchWork, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "at most 2048 steps") {
		t.Fatalf("work-limit diagnostics=%+v", diagnostics)
	}
	badPulse := transientSwitchIntent()
	badPulse.Analyses[0].Excitations[1].PulseWidthS = .001505
	if _, diagnostics := ResolveWithTopology(badPulse, "test", "hash", transientSwitchComponents(25), transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exactly on the observation grid") {
		t.Fatalf("pulse diagnostics=%+v", diagnostics)
	}
	for index := range plan.Devices {
		if plan.Devices[index].PrimitiveModel == PrimitiveCapacitorTransientV1 {
			plan.Devices[index].ModelParameters[0].Value = 1
		}
	}
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	plan.RegistryHash = RegistryHash()
	if _, diagnostics := Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "capacitor voltage") {
		t.Fatalf("operating-limit diagnostics=%+v", diagnostics)
	}
	components := transientSwitchComponents(25)
	for index := range components {
		if components[index].Family == "capacitor" {
			components[index].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}
		}
	}
	if _, diagnostics := ResolveWithTopology(transientSwitchIntent(), "test", "hash", components, transientSwitchNodes()); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "transient") {
		t.Fatalf("claim diagnostics=%+v", diagnostics)
	}
}

func TestTransientRejectsProviderSolverAndTopologyFieldsByContract(t *testing.T) {
	intent := transientSwitchIntent()
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"equation", "matrix", "solver", "integration_method", "initial_conditions", "topology", "model_file"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("provider intent unexpectedly contains %q: %s", forbidden, data)
		}
	}
}

func TestTransientReportsPointSpecificFailureAndRejectsTampering(t *testing.T) {
	components := transientSwitchComponents(25)
	components = append(components, voltageSourceEvidence("conflict", "VCC", "GND"))
	intent := transientSwitchIntent()
	intent.Analyses[0].Excitations = append(intent.Analyses[0].Excitations, SourceExcitation{Component: "conflict", DCValue: 4})
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "initial condition failed") || diagnostics[0].Suggestion == "" {
		t.Fatalf("convergence diagnostics=%+v", diagnostics)
	}

	plan = resolveTransientSwitchPlan(t, 25)
	plan.Analyses[0].TimeStepS = .000011
	if _, diagnostics = Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "exact integer grid") {
		t.Fatalf("grid tamper diagnostics=%+v", diagnostics)
	}
	plan = resolveTransientSwitchPlan(t, 25)
	for index := range plan.Devices {
		if plan.Devices[index].PrimitiveModel == PrimitiveCapacitorTransientV1 {
			plan.Devices[index].PrimitiveModel = PrimitiveCapacitorV1
			plan.Devices[index].ModelParameters = nil
		}
	}
	plan.TopologyHash = topologyHash(plan.GroundNode, plan.Nodes, plan.Devices)
	if _, diagnostics = Evaluate(plan); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "transient capacitor") {
		t.Fatalf("primitive tamper diagnostics=%+v", diagnostics)
	}
}

func resolveTransientSwitchPlan(t *testing.T, capacitorLimit float64) Plan {
	t.Helper()
	plan, diagnostics := ResolveWithTopology(transientSwitchIntent(), "test", "catalog-hash", transientSwitchComponents(capacitorLimit), transientSwitchNodes())
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func transientSwitchIntent() Intent {
	return Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{ID: "switch", Kind: AnalysisTransient, DurationS: .003, TimeStepS: .00001, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 5},
			{Component: "drive", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: .0005, PulseWidthS: .0015, PulsePeriodS: .004},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 0, Min: 4.9, Max: 5.01},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .001, Min: 0, Max: 1},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0028, Min: 4.5, Max: 5.01},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityFallTimeS, Min: 0, Max: .0005},
			{AnalysisID: "switch", Node: "OUT", Quantity: QuantityRiseTimeS, Min: .0001, Max: .0004},
		},
	}
}

func transientSwitchComponents(capacitorLimit float64) []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("base", 10000, "DRIVE", "BASE"),
		resistorEvidence("collector", 1000, "VCC", "OUT"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 100e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: capacitorLimit}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "q1", CatalogID: "bjt.onsemi.mmbt3904.sot23", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "OUT"}, {Function: "EMITTER", Net: "GND"}}},
	}
}

func transientSwitchNodes() []NodeEvidence {
	return []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "DRIVE"}, {Name: "BASE"}, {Name: "OUT"}}
}

func transientDiodeIntent() Intent {
	return Intent{ModelID: ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "clamp", Kind: AnalysisTransient, DurationS: .0008, TimeStepS: .00001, Excitations: []SourceExcitation{{Component: "source", PulseValue: 5, PulseDelayS: .0001, PulseWidthS: .0005, PulsePeriodS: .001}}}},
		Assertions: []Assertion{{AnalysisID: "clamp", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0003, Min: .5, Max: .9}, {AnalysisID: "clamp", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .00075, Min: 0, Max: .1}}}
}

func transientDiodeComponents() []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("limit", 1000, "SOURCE", "OUT"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 10e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 25}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "diode", CatalogID: "diode.onsemi.1n4148w.sod_123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
}

func transientPNPIntent() Intent {
	return Intent{ModelID: ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "switch", Kind: AnalysisTransient, DurationS: .0015, TimeStepS: .00001, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "drive", PulseInitialValue: 5, PulseValue: 0, PulseDelayS: .0002, PulseWidthS: .0007, PulsePeriodS: .002}}}},
		Assertions: []Assertion{{AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0001, Min: 0, Max: .1}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .0006, Min: 4, Max: 5.1}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityFallTimeS, Min: 0, Max: .0005}, {AnalysisID: "switch", Node: "OUT", Quantity: QuantityRiseTimeS, Min: 0, Max: .0005}}}
}

func transientPNPComponents() []ComponentEvidence {
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"), voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("base", 10000, "DRIVE", "BASE"), resistorEvidence("collector", 1000, "OUT", "GND"),
		{InstanceID: "load", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 100e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 25}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "q1", CatalogID: "reviewed-pnp", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTPNPV1, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "OUT"}, {Function: "EMITTER", Net: "VCC"}}},
	}
}
