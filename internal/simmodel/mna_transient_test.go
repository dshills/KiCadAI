package simmodel

import (
	"encoding/json"
	"strings"
	"testing"
)

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
	if lastEvidence == nil || lastEvidence.Method != "backward_euler_bounded_newton_v1" || lastEvidence.TimeSteps != 300 || lastEvidence.TotalIterations <= 0 || lastEvidence.MaxIterationsPerStep != nonlinearMaxIterations || lastEvidence.MaxTotalIterations != maxTransientWork {
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
