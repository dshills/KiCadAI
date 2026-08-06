package simmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestAdvanceActiveDeviceStateSettlesSensorBeforeController(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "controller", PrimitiveModel: PrimitiveOpAmpV1},
		{Component: "sensor", PrimitiveModel: PrimitiveCurrentSenseAmplifierV1},
	}}
	next := advanceActiveDeviceState(plan, nil, map[string]float64{
		"controller": 5,
		"sensor":     0,
	})
	if len(next) != 1 || next["sensor"] != 0 {
		t.Fatalf("next active state = %#v; want only upstream sensor clamp", next)
	}
}

func TestNonlinearOperatingRangeAcceptsSolverNoiseButRejectsMaterialViolation(t *testing.T) {
	tolerance := nonlinearOperatingVoltageTolerance(36)
	if outsideNonlinearOperatingRange(36+tolerance/2, 4.5, 36) {
		t.Fatal("solver-scale noise at the reviewed maximum was rejected")
	}
	if !outsideNonlinearOperatingRange(36+tolerance*2, 4.5, 36) {
		t.Fatal("material reviewed-range violation was accepted")
	}
}

func TestCoupledLinearAmplifierReleaseRecoversCascadedNegativeFeedback(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 120000},
		{Name: "gain_bandwidth_hz", Value: 10_000_000},
		{Name: "output_high_margin_v", Value: .05},
		{Name: "output_low_margin_v", Value: .05},
		{Name: "supply_max_v", Value: 36},
		{Name: "supply_min_v", Value: 4.5},
	}
	opAmp := func(id, plus, minus, output string) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: id, CatalogID: "opamp.reviewed", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: plus}, {Function: "IN_MINUS", Net: minus},
				{Function: "OUT", Net: output}, {Function: "V_PLUS", Net: "VP"},
				{Function: "V_MINUS", Net: "GND"},
			},
		}
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VP", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("diode_bias", 1000, "VP", "DIODE"),
		{
			InstanceID: "diode", CatalogID: "diode.reviewed", Family: "diode",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}},
			Connections: []ConnectionEvidence{{Function: "ANODE", Net: "DIODE"}, {Function: "CATHODE", Net: "GND"}},
		},
		opAmp("first", "IN", "MID", "MID"),
		opAmp("second", "MID", "OUT", "OUT"),
	}
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 5}, {Component: "signal", DCValue: 1},
		}}},
		Assertions: []Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: .99, Max: 1.01}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "VP"}, {Name: "IN"}, {Name: "DIODE"}, {Name: "MID"}, {Name: "OUT"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	system, solution, evidence, states, ok := solveNonlinearDCByLinearAmplifierRelease(
		plan, plan.Analyses[0], map[string]float64{"first": .05, "second": 4.95}, nil, false,
	)
	if !ok || len(states) != 0 || evidence.Method != "bounded_coupled_linear_amplifier_release_v1" {
		t.Fatalf("coupled release ok=%t states=%#v evidence=%+v", ok, states, evidence)
	}
	if output := nonlinearNodeVoltage(&system, solution, "OUT"); math.Abs(output-1) > 1e-4 {
		t.Fatalf("coupled release output=%.12g, want 1 V", output)
	}
}

func TestNonlinearDCDiodeOperatingPointIsDeterministic(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode.onsemi.1n4148w.sod_123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan := resolveNonlinearTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}, []Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: .55, Max: .9}})
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || report.Analyses[0].Points[0].Solver == nil {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	first, _ := json.Marshal(report)
	replayed, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics=%+v", replayDiagnostics)
	}
	second, _ := json.Marshal(replayed)
	if string(first) != string(second) {
		t.Fatalf("nonlinear replay differs\n%s\n%s", first, second)
	}
}

func TestNonlinearDCPrecisionRectifierFindsCenteredOpAmpOperatingPoint(t *testing.T) {
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 120000},
		{Name: "gain_bandwidth_hz", Value: 10_000_000},
		{Name: "output_high_margin_v", Value: .05},
		{Name: "output_low_margin_v", Value: .05},
		{Name: "supply_max_v", Value: 36},
		{Name: "supply_min_v", Value: 4.5},
	}
	opAmp := func(id, plus, minus, output string) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: id, CatalogID: "opamp.reviewed", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: plus}, {Function: "IN_MINUS", Net: minus}, {Function: "OUT", Net: output},
				{Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "VN"},
			},
		}
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VP", "GND"),
		voltageSourceEvidence("negative_supply", "VN", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("input", 680_000, "IN", "SUM"),
		resistorEvidence("feedback", 680_000, "OUT", "SUM"),
		resistorEvidence("steering_input", 680_000, "IN", "STEER"),
		resistorEvidence("steering_damping", 47, "STEERING_OUT", "DIODE_DRIVE"),
		opAmp("magnitude_amplifier", "STEER", "SUM", "OUT"),
		opAmp("steering_amplifier", "GND", "STEER", "STEERING_OUT"),
		{
			InstanceID: "steering_diode", CatalogID: "diode.reviewed", Family: "diode",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}},
			Connections: []ConnectionEvidence{{Function: "ANODE", Net: "DIODE_DRIVE"}, {Function: "CATHODE", Net: "STEER"}},
		},
	}
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 5}, {Component: "negative_supply", DCValue: -.232}, {Component: "signal", DCValue: -1},
		}}},
		Assertions: []Assertion{{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: .95, Max: 1.05}},
	}
	nodes := []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "VP"}, {Name: "VN"}, {Name: "IN"}, {Name: "SUM"},
		{Name: "STEER"}, {Name: "OUT"}, {Name: "STEERING_OUT"}, {Name: "DIODE_DRIVE"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCCenteredSeedSupportsMoreThanFourOpAmps(t *testing.T) {
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 120000},
		{Name: "gain_bandwidth_hz", Value: 10_000_000},
		{Name: "output_high_margin_v", Value: .05},
		{Name: "output_low_margin_v", Value: .05},
		{Name: "supply_max_v", Value: 36},
		{Name: "supply_min_v", Value: 4.5},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("positive_supply", "VP", "GND"),
		voltageSourceEvidence("negative_supply", "VN", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("diode_bias", 1000, "VP", "DIODE"),
		{
			InstanceID: "diode", CatalogID: "diode.reviewed", Family: "diode",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}},
			Connections: []ConnectionEvidence{{Function: "ANODE", Net: "DIODE"}, {Function: "CATHODE", Net: "GND"}},
		},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VP"}, {Name: "VN"}, {Name: "IN"}, {Name: "DIODE"}}
	for index := 1; index <= 5; index++ {
		id := fmt.Sprintf("buffer_%d", index)
		output := fmt.Sprintf("OUT_%d", index)
		components = append(components, ComponentEvidence{
			InstanceID: id, CatalogID: "opamp.reviewed", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: output}, {Function: "OUT", Net: output},
				{Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "VN"},
			},
		})
		nodes = append(nodes, NodeEvidence{Name: output})
	}
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "positive_supply", DCValue: 5},
			{Component: "negative_supply", DCValue: -5},
			{Component: "signal", DCValue: 1},
		}}},
		Assertions: []Assertion{{AnalysisID: "bias", Node: "OUT_5", Quantity: QuantityVoltageV, Min: .99, Max: 1.01}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, _, evidence, _, ok := solveNonlinearDCByCenteredOpAmpSeed(plan, plan.Analyses[0], map[string]float64{})
	if !ok || evidence.Method != "bounded_newton_centered_opamp_seed_v1" {
		t.Fatalf("five-op-amp centered seed failed: ok=%v evidence=%+v", ok, evidence)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestLinearColdSeedRequiresOpAmpControlledEmitterFollower(t *testing.T) {
	opamp := ResolvedDevice{
		Component: "U1", PrimitiveModel: PrimitiveOpAmpV1,
		Terminals: []TerminalBinding{{Terminal: "OUT", Net: "DRIVE"}, {Terminal: "IN_MINUS", Net: "SENSE"}},
	}
	follower := ResolvedDevice{
		Component: "Q1", PrimitiveModel: PrimitiveBJTNPNV1,
		Terminals: []TerminalBinding{{Terminal: "BASE", Net: "DRIVE"}, {Terminal: "EMITTER", Net: "SENSE"}},
	}
	if !hasOpAmpControlledEmitterFollower(Plan{Devices: []ResolvedDevice{opamp, follower}}) {
		t.Fatal("op-amp-controlled emitter follower was not recognized")
	}
	driver := ResolvedDevice{
		Component: "RDRIVE", PrimitiveModel: PrimitiveResistorV1,
		Terminals: []TerminalBinding{{Terminal: "A", Net: "DRIVE"}, {Terminal: "B", Net: "BASE"}},
	}
	sense := ResolvedDevice{
		Component: "RSENSE", PrimitiveModel: PrimitiveResistorV1,
		Terminals: []TerminalBinding{{Terminal: "A", Net: "EMITTER"}, {Terminal: "B", Net: "OUTPUT"}},
	}
	feedback := ResolvedDevice{
		Component: "RFEEDBACK", PrimitiveModel: PrimitiveResistorV1,
		Terminals: []TerminalBinding{{Terminal: "A", Net: "OUTPUT"}, {Terminal: "B", Net: "SENSE"}},
	}
	follower.Terminals = []TerminalBinding{{Terminal: "BASE", Net: "BASE"}, {Terminal: "EMITTER", Net: "EMITTER"}}
	if !hasOpAmpControlledEmitterFollower(Plan{Devices: []ResolvedDevice{opamp, follower, driver, sense, feedback}}) {
		t.Fatal("resistor-coupled op-amp-controlled emitter follower was not recognized")
	}
	follower.Terminals = append(follower.Terminals, TerminalBinding{Terminal: "COLLECTOR", Net: "SUPPLY"})
	opamp.Terminals = append(opamp.Terminals, TerminalBinding{Terminal: "V_PLUS", Net: "SUPPLY"})
	if !hasOpAmpControlledEmitterFollower(Plan{Devices: []ResolvedDevice{opamp, follower, driver}}) {
		t.Fatal("supply-referenced high-side emitter follower was not recognized")
	}
	follower.Terminals[1].Net = "UNRELATED"
	follower.Terminals[2].Net = "OTHER_SUPPLY"
	if hasOpAmpControlledEmitterFollower(Plan{Devices: []ResolvedDevice{opamp, follower, driver, sense, feedback}}) {
		t.Fatal("unrelated op-amp and BJT incorrectly enabled the linear cold seed")
	}
}

func TestNonlinearDCProtectedPassRegulatorConvergesFromRailSeed(t *testing.T) {
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100_000},
		{Name: "gain_bandwidth_hz", Value: 1_000_000},
		{Name: "output_high_margin_v", Value: 1.5},
		{Name: "output_low_margin_v", Value: .02},
		{Name: "supply_max_v", Value: 32},
		{Name: "supply_min_v", Value: 3},
	}
	bjt := func(id, base, collector, emitter string, saturationCurrent, beta, maxCurrent float64) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: id, CatalogID: id, Family: "bjt",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: []NamedValue{
				{Name: "saturation_current_a", Value: saturationCurrent}, {Name: "forward_beta", Value: beta},
				{Name: "reverse_beta", Value: 1}, {Name: "emission_coefficient", Value: 1},
				{Name: "junction_temperature_k", Value: 300.15}, {Name: "max_collector_current_a", Value: maxCurrent},
				{Name: "max_collector_emitter_voltage_v", Value: 80},
			}}},
			Connections: []ConnectionEvidence{
				{Function: "BASE", Net: base}, {Function: "COLLECTOR", Net: collector}, {Function: "EMITTER", Net: emitter},
			},
		}
	}
	capacitor := func(id string, value float64, a, b string) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: id, CatalogID: id, Family: "capacitor", ValueSI: value, HasValueSI: true,
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}},
			Connections: []ConnectionEvidence{{Function: "A", Net: a}, {Function: "B", Net: b}},
		}
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VP", "GND"),
		voltageSourceEvidence("command", "COMMAND", "GND"),
		resistorEvidence("command_access", 1000, "COMMAND", "SETPOINT"),
		capacitor("command_filter", 4.7e-6, "SETPOINT", "GND"),
		{
			InstanceID: "controller", CatalogID: "opamp.reviewed", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "SETPOINT"}, {Function: "IN_MINUS", Net: "FEEDBACK"}, {Function: "OUT", Net: "DRIVE"},
				{Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"},
			},
		},
		bjt("pass", "BASE", "VP", "SENSE_HIGH", 1e-12, 40, 10),
		bjt("limit", "SENSE_HIGH", "BASE", "OUTPUT", 1e-14, 100, .2),
		resistorEvidence("drive", 47, "DRIVE", "BASE"),
		resistorEvidence("base_bleeder", 10_000, "BASE", "SENSE_HIGH"),
		resistorEvidence("sense_a", 1.6, "SENSE_HIGH", "SENSE_MID"),
		resistorEvidence("sense_b", 1.6, "SENSE_MID", "OUTPUT"),
		resistorEvidence("feedback_upper", 47_000, "OUTPUT", "FEEDBACK"),
		resistorEvidence("feedback_lower", 15_000, "FEEDBACK", "GND"),
		capacitor("output_capacitor", 4.7e-6, "OUTPUT", "GND"),
		capacitor("compensation", 15e-9, "FEEDBACK", "DRIVE"),
		{
			InstanceID: "load", CatalogID: "source.current", Family: "current_source",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}},
			Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "OUTPUT"}, {Function: "NEGATIVE", Net: "GND"}},
		},
	}
	nodes := []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "VP"}, {Name: "COMMAND"}, {Name: "SETPOINT"},
		{Name: "FEEDBACK"}, {Name: "DRIVE"}, {Name: "BASE"}, {Name: "SENSE_HIGH"},
		{Name: "SENSE_MID"}, {Name: "OUTPUT"},
	}
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "supply", DCValue: 9.5},
			{Component: "command", DCValue: 1.2},
			{Component: "load", DCValue: .0775},
		}}},
		Assertions: []Assertion{{AnalysisID: "bias", Node: "OUTPUT", Quantity: QuantityVoltageV, Min: 4.9, Max: 5.1}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	plan.Uncertainties = []Uncertainty{
		{Target: "devices.command_access.value_si", Source: "test", Nominal: 1000, Minimum: 950, Maximum: 1050},
		{Target: "devices.drive.value_si", Source: "test", Nominal: 47, Minimum: 46.53, Maximum: 47.47},
		{Target: "devices.base_bleeder.value_si", Source: "test", Nominal: 10_000, Minimum: 9900, Maximum: 10_100},
		{Target: "devices.feedback_upper.value_si", Source: "test", Nominal: 47_000, Minimum: 46_953, Maximum: 47_047},
		{Target: "devices.feedback_lower.value_si", Source: "test", Nominal: 15_000, Minimum: 14_985, Maximum: 15_015},
		{Target: "devices.sense_a.value_si", Source: "test", Nominal: 1.6, Minimum: 1.52, Maximum: 1.68},
		{Target: "devices.sense_b.value_si", Source: "test", Nominal: 1.6, Minimum: 1.52, Maximum: 1.68},
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCManufacturerZenerModelClampsInReverse(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "12V", "GND"),
		resistorEvidence("limit", 330, "12V", "OUT"),
		{InstanceID: "zener", CatalogID: "diode.diodes.ddz5v1b_7.sod123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveUnidirectionalZenerV1, Parameters: zenerParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "GND"}, {Function: "CATHODE", Net: "OUT"}}},
	}
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.8, Max: 5.5}})
	intent.Analyses[0].Excitations[0].DCValue = 12
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "12V"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCGuaranteedNMOSSwitchUsesCatalogGateAndResistance(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "off", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "gate", DCValue: 0}}},
			{ID: "on", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "gate", DCValue: 2.5}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "off", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 4.99, Max: 5.01},
			{AnalysisID: "on", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 0, Max: .001},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"), voltageSourceEvidence("gate", "GATE", "GND"), resistorEvidence("load", 1000, "5V", "DRAIN"),
		{InstanceID: "switch", CatalogID: "mosfet.aos.ao3400a.sot23", Family: "mosfet", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveNMOSSwitchV1, Parameters: nmosSwitchParameters()}}, Connections: []ConnectionEvidence{{Function: "GATE", Net: "GATE"}, {Function: "DRAIN", Net: "DRAIN"}, {Function: "SOURCE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "GATE"}, {Name: "DRAIN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCGuaranteedPMOSSwitchUsesSourceReferencedGate(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "off", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}, {Component: "gate", DCValue: 12}}},
			{ID: "on", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}, {Component: "gate", DCValue: 2}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "off", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 0, Max: 1e-6},
			{AnalysisID: "on", Node: "DRAIN", Quantity: QuantityVoltageV, Min: 11.98, Max: 12.01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "12V", "GND"), voltageSourceEvidence("gate", "GATE", "GND"), resistorEvidence("load", 1000, "DRAIN", "GND"),
		{InstanceID: "switch", CatalogID: "mosfet.vishay.irfp9240.to247", Family: "mosfet", ModelClaims: []CatalogEvidence{{ModelID: PrimitivePMOSSwitchV1, Parameters: pmosSwitchParameters()}}, Connections: []ConnectionEvidence{{Function: "GATE", Net: "GATE"}, {Function: "DRAIN", Net: "DRAIN"}, {Function: "SOURCE", Net: "12V"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "12V"}, {Name: "GATE"}, {Name: "DRAIN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestReverseBlockingLoadSwitchConductsForwardAndLimitsBackfeed(t *testing.T) {
	t.Run("forward", func(t *testing.T) {
		intent := nonlinearTestIntent([]Assertion{
			{AnalysisID: "bias", Node: "VOUT", Quantity: QuantityVoltageV, Min: 4.97, Max: 5},
		})
		components := []ComponentEvidence{
			voltageSourceEvidence("supply", "VIN", "GND"),
			resistorEvidence("load", 100, "VOUT", "GND"),
			{
				InstanceID: "switch", CatalogID: "protection.test.reverse_blocking", Family: "protection",
				ModelClaims: []CatalogEvidence{{ModelID: PrimitiveReverseBlockingLoadSwitchV1, Parameters: reverseBlockingLoadSwitchParameters()}},
				Connections: []ConnectionEvidence{
					{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"},
					{Function: "GND", Net: "GND"}, {Function: "ON", Net: "VIN"},
				},
			},
		}
		plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "VOUT"}})
		if len(diagnostics) != 0 {
			t.Fatalf("resolve diagnostics=%+v", diagnostics)
		}
		report, diagnostics := Evaluate(plan)
		if len(diagnostics) != 0 || report.Status != "pass" {
			t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
		}
	})

	t.Run("reverse", func(t *testing.T) {
		intent := Intent{
			ModelID: ModelNonlinearCircuitDCV1,
			Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
				{Component: "input", DCValue: 0}, {Component: "output", DCValue: 5}, {Component: "enable", DCValue: 5},
			}}},
			Assertions: []Assertion{{AnalysisID: "bias", Component: "switch", Quantity: QuantityDeviceCurrentA, Min: 0, Max: 1e-6}},
		}
		components := []ComponentEvidence{
			voltageSourceEvidence("input", "VIN", "GND"),
			voltageSourceEvidence("output", "VOUT", "GND"),
			voltageSourceEvidence("enable", "ON", "GND"),
			{
				InstanceID: "switch", CatalogID: "protection.test.reverse_blocking", Family: "protection",
				ModelClaims: []CatalogEvidence{{ModelID: PrimitiveReverseBlockingLoadSwitchV1, Parameters: reverseBlockingLoadSwitchParameters()}},
				Connections: []ConnectionEvidence{
					{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"},
					{Function: "GND", Net: "GND"}, {Function: "ON", Net: "ON"},
				},
			},
		}
		plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "VOUT"}, {Name: "ON"}})
		if len(diagnostics) != 0 {
			t.Fatalf("resolve diagnostics=%+v", diagnostics)
		}
		report, diagnostics := Evaluate(plan)
		if len(diagnostics) != 0 || report.Status != "pass" {
			t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
		}
	})
}

func TestCurrentLimitingEFuseRegulatesOverloadAtProgrammedLimit(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{
			ID:          "bias",
			Kind:        AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 24}},
		}},
		Assertions: []Assertion{
			{AnalysisID: "bias", Node: "VOUT", Quantity: QuantityVoltageV, Min: 14.39, Max: 14.41},
			{AnalysisID: "bias", Component: "efuse", Quantity: QuantityDeviceCurrentA, Min: .2999, Max: .3001},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		resistorEvidence("load", 48, "VOUT", "GND"),
		{
			InstanceID: "efuse", CatalogID: "protection.test.current_limiting_efuse", Family: "protection",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentLimitingEFuseV1, Parameters: currentLimitingEFuseParameters()}},
			Connections: []ConnectionEvidence{
				{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"},
				{Function: "RTN", Net: "GND"}, {Function: "SHDN", Net: "VIN"},
			},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "VOUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestCurrentLimitingEFuseUsesOnResistanceBelowLimit(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{
			ID:          "bias",
			Kind:        AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 24}},
		}},
		Assertions: []Assertion{
			{AnalysisID: "bias", Node: "VOUT", Quantity: QuantityVoltageV, Min: 23.96, Max: 23.97},
			{AnalysisID: "bias", Component: "efuse", Quantity: QuantityDeviceCurrentA, Min: .2395, Max: .2397},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		resistorEvidence("load", 100, "VOUT", "GND"),
		{
			InstanceID: "efuse", CatalogID: "protection.test.current_limiting_efuse", Family: "protection",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentLimitingEFuseV1, Parameters: currentLimitingEFuseParameters()}},
			Connections: []ConnectionEvidence{
				{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"},
				{Function: "RTN", Net: "GND"}, {Function: "SHDN", Net: "VIN"},
			},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "VOUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCNPNAndPNPBias(t *testing.T) {
	for _, test := range []struct {
		name       string
		primitive  string
		components []ComponentEvidence
		nodes      []NodeEvidence
		assertions []Assertion
	}{
		{
			name: "npn", primitive: PrimitiveBJTNPNV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "5V", "BASE"), resistorEvidence("collector_load", 1000, "5V", "COLLECTOR")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: .5, Max: .9}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: 3.5, Max: 4.8}},
		},
		{
			name: "pnp", primitive: PrimitiveBJTPNPV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "BASE", "GND"), resistorEvidence("collector_load", 1000, "COLLECTOR", "GND")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: 4.1, Max: 4.5}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: .2, Max: 1.5}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			emitterNet := "GND"
			if test.primitive == PrimitiveBJTPNPV1 {
				emitterNet = "5V"
			}
			test.components = append(test.components, ComponentEvidence{InstanceID: "q1", CatalogID: "reviewed-bjt", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: test.primitive, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "COLLECTOR"}, {Function: "EMITTER", Net: emitterNet}}})
			plan := resolveNonlinearTestPlan(t, test.components, test.nodes, test.assertions)
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
			}
		})
	}
}

func TestNonlinearDCComplementaryEmitterFollowerIsPolaritySymmetric(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "positive", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "drive", DCValue: 1}}},
			{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "drive", DCValue: -1}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "positive", Node: "OUT", Quantity: QuantityVoltageV, Min: .1, Max: .8},
			{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: -.8, Max: -.1},
		},
	}
	parameters := bjtParameters(.2, 40)
	components := []ComponentEvidence{
		voltageSourceEvidence("positive_supply", "VCC", "GND"),
		voltageSourceEvidence("negative_supply", "VEE", "GND"),
		voltageSourceEvidence("drive", "DRIVE", "GND"),
		resistorEvidence("npn_emitter", .22, "NPN_EMITTER", "OUT"),
		resistorEvidence("pnp_emitter", .22, "PNP_EMITTER", "OUT"),
		resistorEvidence("load", 8, "OUT", "GND"),
		{InstanceID: "npn", CatalogID: "reviewed-npn", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "DRIVE"}, {Function: "COLLECTOR", Net: "VCC"}, {Function: "EMITTER", Net: "NPN_EMITTER"}}},
		{InstanceID: "pnp", CatalogID: "reviewed-pnp", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTPNPV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "DRIVE"}, {Function: "COLLECTOR", Net: "VEE"}, {Function: "EMITTER", Net: "PNP_EMITTER"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}, {Name: "VEE"}, {Name: "DRIVE"}, {Name: "NPN_EMITTER"}, {Name: "PNP_EMITTER"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestNonlinearDCRejectsACAmbiguousClaimsAndOperatingLimit(t *testing.T) {
	base := []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("limit", 1000, "5V", "OUT"), {InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}}}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}
	ac := Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "ac", Kind: AnalysisACSweep, StartFrequencyHz: 1, StopFrequencyHz: 10, Points: 2, Excitations: []SourceExcitation{{Component: "supply", ACMagnitude: 1}}}}, Assertions: []Assertion{{AnalysisID: "ac", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1, Min: 0, Max: 10}}}
	if _, diagnostics := ResolveWithTopology(ac, "test", "hash", base, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "DC operating points only") {
		t.Fatalf("AC diagnostics=%+v", diagnostics)
	}
	ambiguous := append([]ComponentEvidence(nil), base...)
	ambiguous[2].ModelClaims = append(ambiguous[2].ModelClaims, ambiguous[2].ModelClaims[0])
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", ambiguous, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "ambiguous") {
		t.Fatalf("ambiguous diagnostics=%+v", diagnostics)
	}
	missing := append([]ComponentEvidence(nil), base...)
	missing[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)[1:]}}
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", missing, nodes); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "missing required parameter saturation_current_a") {
		t.Fatalf("missing-parameter diagnostics=%+v", diagnostics)
	}
	limited := append([]ComponentEvidence(nil), base...)
	limited[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(1e-6, 100)}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", limited, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "forward current") || diagnostics[0].Suggestion == "" {
		t.Fatalf("limit diagnostics=%+v", diagnostics)
	}
}

func diagnosticsContain(diagnostics []Diagnostic, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, fragment) {
			return true
		}
	}
	return false
}

func TestNonlinearDCReportsActionableBoundedSolveFailure(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		voltageSourceEvidence("conflict", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	intent.Analyses[0].Excitations = append(intent.Analyses[0].Excitations, SourceExcitation{Component: "conflict", DCValue: 4})
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "continuation stage") || !strings.Contains(diagnostics[0].Suggestion, "bias path") {
		t.Fatalf("solve diagnostics=%+v", diagnostics)
	}
}

func TestBidirectionalOpenDrainTranslatorPreservesHighRailsAndPropagatesLow(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bus", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "low_supply", DCValue: 3.3}, {Component: "high_supply", DCValue: 5}, {Component: "driver", DCValue: 0},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "bus", Node: "A1", Quantity: QuantityVoltageV, Min: 0, Max: .01},
			{AnalysisID: "bus", Node: "B1", Quantity: QuantityVoltageV, Min: 0, Max: .4},
			{AnalysisID: "bus", Node: "A2", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31},
			{AnalysisID: "bus", Node: "B2", Quantity: QuantityVoltageV, Min: 4.99, Max: 5},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("low_supply", "VCCA", "GND"),
		voltageSourceEvidence("high_supply", "VCCB", "GND"),
		voltageSourceEvidence("driver", "A1", "GND"),
		resistorEvidence("a1_pullup", 4700, "VCCA", "A1"),
		resistorEvidence("b1_pullup", 4700, "VCCB", "B1"),
		resistorEvidence("a2_pullup", 4700, "VCCA", "A2"),
		resistorEvidence("b2_pullup", 4700, "VCCB", "B2"),
		{InstanceID: "translator", CatalogID: "translator", Family: "level_translator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalOpenDrainTranslatorV1, Parameters: openDrainTranslatorParameters()}}, Connections: []ConnectionEvidence{{Function: "A1", Net: "A1"}, {Function: "A2", Net: "A2"}, {Function: "B1", Net: "B1"}, {Function: "B2", Net: "B2"}, {Function: "VCCA", Net: "VCCA"}, {Function: "VCCB", Net: "VCCB"}, {Function: "GND", Net: "GND"}, {Function: "OE", Net: "VCCA"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "A1"}, {Name: "A2"}, {Name: "B1"}, {Name: "B2"}})
	if len(diagnostics) != 0 {
		t.Fatalf("translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestPushPullTranslatorPropagatesLogicWithoutJoiningSupplyDomains(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "logic", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "low_supply", DCValue: 1.8}, {Component: "high_supply", DCValue: 3.3},
			{Component: "high_input_1", DCValue: 1.8}, {Component: "low_input_2", DCValue: 0},
			{Component: "high_input_3", DCValue: 1.8}, {Component: "low_input_4", DCValue: 0},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "logic", Node: "B1", Quantity: QuantityVoltageV, Min: 3.2, Max: 3.31},
			{AnalysisID: "logic", Node: "B2", Quantity: QuantityVoltageV, Min: 0, Max: .4},
			{AnalysisID: "logic", Node: "VCCA", Quantity: QuantityVoltageV, Min: 1.79, Max: 1.81},
			{AnalysisID: "logic", Node: "VCCB", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("low_supply", "VCCA", "GND"),
		voltageSourceEvidence("high_supply", "VCCB", "GND"),
		voltageSourceEvidence("high_input_1", "A1", "GND"),
		voltageSourceEvidence("low_input_2", "A2", "GND"),
		voltageSourceEvidence("high_input_3", "A3", "GND"),
		voltageSourceEvidence("low_input_4", "A4", "GND"),
		resistorEvidence("b1_load", 1e6, "B1", "GND"),
		resistorEvidence("b2_load", 10000, "VCCB", "B2"),
		resistorEvidence("b3_load", 1e6, "B3", "GND"),
		resistorEvidence("b4_load", 10000, "VCCB", "B4"),
		pushPullTranslatorEvidence("translator", "VCCA", 1),
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "A1"}, {Name: "A2"}, {Name: "A3"}, {Name: "A4"}, {Name: "B1"}, {Name: "B2"}, {Name: "B3"}, {Name: "B4"}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("push-pull translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("push-pull translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestPushPullTranslatorDisabledStateIsHighImpedance(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "disabled", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "low_supply", DCValue: 1.8}, {Component: "high_supply", DCValue: 3.3},
			{Component: "input_1", DCValue: 1.8}, {Component: "input_2", DCValue: 0},
			{Component: "input_3", DCValue: 1.8}, {Component: "input_4", DCValue: 0},
			{Component: "disabled", DCValue: 0},
		}}},
		Assertions: []Assertion{{AnalysisID: "disabled", Node: "B1", Quantity: QuantityVoltageV, Min: 1.08, Max: 1.12}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("low_supply", "VCCA", "GND"),
		voltageSourceEvidence("high_supply", "VCCB", "GND"),
		voltageSourceEvidence("input_1", "A1", "GND"),
		voltageSourceEvidence("input_2", "A2", "GND"),
		voltageSourceEvidence("input_3", "A3", "GND"),
		voltageSourceEvidence("input_4", "A4", "GND"),
		voltageSourceEvidence("disabled", "OE", "GND"),
		resistorEvidence("b1_upper", 10000, "VCCB", "B1"),
		resistorEvidence("b1_lower", 5000, "B1", "GND"),
		resistorEvidence("b2_load", 10000, "B2", "GND"),
		resistorEvidence("b3_load", 10000, "B3", "GND"),
		resistorEvidence("b4_load", 10000, "B4", "GND"),
		pushPullTranslatorEvidence("translator", "OE", 1),
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "OE"}, {Name: "A1"}, {Name: "A2"}, {Name: "A3"}, {Name: "A4"}, {Name: "B1"}, {Name: "B2"}, {Name: "B3"}, {Name: "B4"}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("disabled translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("disabled translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestDirectionControlledTranslatorSupportsBothDirectionGroups(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "logic", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "vcca_supply", DCValue: 1.8}, {Component: "vccb_supply", DCValue: 3.3},
			{Component: "enabled", DCValue: 0}, {Component: "dir1_high", DCValue: 1.8}, {Component: "dir2_low", DCValue: 0},
			{Component: "a1_high", DCValue: 1.8}, {Component: "a2_low", DCValue: 0}, {Component: "a3_low", DCValue: 0}, {Component: "a4_low", DCValue: 0},
			{Component: "b5_high", DCValue: 3.3}, {Component: "b6_low", DCValue: 0}, {Component: "b7_low", DCValue: 0}, {Component: "b8_low", DCValue: 0},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "logic", Node: "B1", Quantity: QuantityVoltageV, Min: 3.28, Max: 3.31},
			{AnalysisID: "logic", Node: "B2", Quantity: QuantityVoltageV, Min: 0, Max: .01},
			{AnalysisID: "logic", Node: "A5", Quantity: QuantityVoltageV, Min: 1.79, Max: 1.81},
			{AnalysisID: "logic", Node: "A6", Quantity: QuantityVoltageV, Min: 0, Max: .01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("vcca_supply", "VCCA", "GND"), voltageSourceEvidence("vccb_supply", "VCCB", "GND"),
		voltageSourceEvidence("enabled", "OE", "GND"), voltageSourceEvidence("dir1_high", "DIR1", "GND"), voltageSourceEvidence("dir2_low", "DIR2", "GND"),
		voltageSourceEvidence("a1_high", "A1", "GND"), voltageSourceEvidence("a2_low", "A2", "GND"),
		voltageSourceEvidence("a3_low", "A3", "GND"), voltageSourceEvidence("a4_low", "A4", "GND"),
		voltageSourceEvidence("b5_high", "B5", "GND"), voltageSourceEvidence("b6_low", "B6", "GND"),
		voltageSourceEvidence("b7_low", "B7", "GND"), voltageSourceEvidence("b8_low", "B8", "GND"),
		directionControlledTranslatorEvidence("transceiver", "OE"),
	}
	for channel := 1; channel <= 4; channel++ {
		components = append(components, resistorEvidence(fmt.Sprintf("b%d_load", channel), 1e6, fmt.Sprintf("B%d", channel), "GND"))
	}
	for channel := 5; channel <= 8; channel++ {
		components = append(components, resistorEvidence(fmt.Sprintf("a%d_load", channel), 1e6, fmt.Sprintf("A%d", channel), "GND"))
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "OE"}, {Name: "DIR1"}, {Name: "DIR2"}}
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		nodes = append(nodes, NodeEvidence{Name: fmt.Sprintf("A%d", channel)}, NodeEvidence{Name: fmt.Sprintf("B%d", channel)})
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("direction-controlled translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("direction-controlled translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestDirectionControlledTranslatorDisabledStateIsHighImpedance(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "disabled", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "vcca_supply", DCValue: 1.8}, {Component: "vccb_supply", DCValue: 3.3},
			{Component: "disabled", DCValue: 1.8}, {Component: "dir1_high", DCValue: 1.8}, {Component: "dir2_low", DCValue: 0},
		}}},
		Assertions: []Assertion{{AnalysisID: "disabled", Node: "B1", Quantity: QuantityVoltageV, Min: 1.08, Max: 1.12}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("vcca_supply", "VCCA", "GND"), voltageSourceEvidence("vccb_supply", "VCCB", "GND"),
		voltageSourceEvidence("disabled", "OE", "GND"), voltageSourceEvidence("dir1_high", "DIR1", "GND"), voltageSourceEvidence("dir2_low", "DIR2", "GND"),
		resistorEvidence("b1_upper", 10000, "VCCB", "B1"), resistorEvidence("b1_lower", 5000, "B1", "GND"),
		directionControlledTranslatorEvidence("transceiver", "OE"),
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCCA"}, {Name: "VCCB"}, {Name: "OE"}, {Name: "DIR1"}, {Name: "DIR2"}}
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		nodes = append(nodes, NodeEvidence{Name: fmt.Sprintf("A%d", channel)}, NodeEvidence{Name: fmt.Sprintf("B%d", channel)})
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("disabled direction-controlled translator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("disabled direction-controlled translator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestPushPullDigitalIsolatorPropagatesBothDirectionsAndDisablesHighImpedance(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "enabled", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
				{Component: "supply_1", DCValue: 3.3}, {Component: "supply_2", DCValue: 3.3},
				{Component: "enable_1", DCValue: 3.3}, {Component: "enable_2", DCValue: 3.3},
				{Component: "ina1", DCValue: 3.3}, {Component: "ina2", DCValue: 0}, {Component: "ina3", DCValue: 0}, {Component: "inb4", DCValue: 3.3},
			}},
			{ID: "disabled", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
				{Component: "supply_1", DCValue: 3.3}, {Component: "supply_2", DCValue: 3.3},
				{Component: "enable_1", DCValue: 0}, {Component: "enable_2", DCValue: 0},
				{Component: "ina1", DCValue: 3.3}, {Component: "ina2", DCValue: 0}, {Component: "ina3", DCValue: 0}, {Component: "inb4", DCValue: 3.3},
			}},
		},
		Assertions: []Assertion{
			{AnalysisID: "enabled", Node: "OUTB1", ReferenceNode: "GND2", Quantity: QuantityVoltageV, Min: 3.28, Max: 3.31},
			{AnalysisID: "enabled", Node: "OUTA4", ReferenceNode: "GND1", Quantity: QuantityVoltageV, Min: 3.28, Max: 3.31},
			{AnalysisID: "disabled", Node: "OUTB1", ReferenceNode: "GND2", Quantity: QuantityVoltageV, Min: 3.28, Max: 3.31},
			{AnalysisID: "disabled", Node: "OUTA4", ReferenceNode: "GND1", Quantity: QuantityVoltageV, Min: 3.28, Max: 3.31},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply_1", "VDD1", "GND1"), voltageSourceEvidence("supply_2", "VDD2", "GND2"),
		voltageSourceEvidence("enable_1", "EN1", "GND1"), voltageSourceEvidence("enable_2", "EN2", "GND2"),
		voltageSourceEvidence("ina1", "INA1", "GND1"), voltageSourceEvidence("ina2", "INA2", "GND1"),
		voltageSourceEvidence("ina3", "INA3", "GND1"), voltageSourceEvidence("inb4", "INB4", "GND2"),
		resistorEvidence("outb1_load", 1e6, "OUTB1", "VDD2"), resistorEvidence("outb2_load", 1e6, "OUTB2", "GND2"),
		resistorEvidence("outb3_load", 1e6, "OUTB3", "GND2"), resistorEvidence("outa4_load", 1e6, "OUTA4", "VDD1"),
		pushPullIsolatorEvidence("isolator"),
	}
	nodes := []NodeEvidence{
		{Name: "GND1", Role: "ground", VoltageDomain: "side_1"}, {Name: "GND2", Role: "ground", VoltageDomain: "side_2"},
		{Name: "VDD1", VoltageDomain: "side_1"}, {Name: "VDD2", VoltageDomain: "side_2"},
		{Name: "EN1", VoltageDomain: "side_1"}, {Name: "EN2", VoltageDomain: "side_2"},
		{Name: "INA1", VoltageDomain: "side_1"}, {Name: "INA2", VoltageDomain: "side_1"}, {Name: "INA3", VoltageDomain: "side_1"},
		{Name: "INB4", VoltageDomain: "side_2"}, {Name: "OUTB1", VoltageDomain: "side_2"}, {Name: "OUTB2", VoltageDomain: "side_2"},
		{Name: "OUTB3", VoltageDomain: "side_2"}, {Name: "OUTA4", VoltageDomain: "side_1"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("push-pull isolator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("push-pull isolator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalOpenDrainIsolatorPropagatesLowWithoutJoiningGroundDomains(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "bus", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "side_a_supply", DCValue: 3.3}, {Component: "side_b_supply", DCValue: 5}, {Component: "driver", DCValue: 0},
		}}},
		Assertions: []Assertion{
			{AnalysisID: "bus", Node: "SDA1", Quantity: QuantityVoltageV, Min: 0, Max: .01},
			{AnalysisID: "bus", Node: "SDA2", Quantity: QuantityVoltageV, Min: 0, Max: .4},
			{AnalysisID: "bus", Node: "SCL1", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31},
			{AnalysisID: "bus", Node: "SCL2", Quantity: QuantityVoltageV, Min: 4.98, Max: 5.01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("side_a_supply", "VDD1", "GND1"),
		voltageSourceEvidence("side_b_supply", "VDD2", "GND2"),
		voltageSourceEvidence("driver", "SDA1", "GND1"),
		resistorEvidence("sda1_pullup", 4700, "VDD1", "SDA1"),
		resistorEvidence("sda2_pullup", 4700, "VDD2", "SDA2"),
		resistorEvidence("scl1_pullup", 4700, "VDD1", "SCL1"),
		resistorEvidence("scl2_pullup", 4700, "VDD2", "SCL2"),
		{
			InstanceID: "isolator", CatalogID: "isolator", Family: "isolator",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalOpenDrainIsolatorV1, Parameters: openDrainIsolatorParameters()}},
			Connections: []ConnectionEvidence{
				{Function: "SDA1", Net: "SDA1"}, {Function: "SCL1", Net: "SCL1"},
				{Function: "SDA2", Net: "SDA2"}, {Function: "SCL2", Net: "SCL2"},
				{Function: "VDD1", Net: "VDD1"}, {Function: "GND1", Net: "GND1"},
				{Function: "VDD2", Net: "VDD2"}, {Function: "GND2", Net: "GND2"},
			},
		},
	}
	nodes := []NodeEvidence{
		{Name: "GND1", Role: "ground"}, {Name: "GND2"}, {Name: "VDD1"}, {Name: "VDD2"},
		{Name: "SDA1"}, {Name: "SDA2"}, {Name: "SCL1"}, {Name: "SCL2"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("isolator resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("isolator report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalOpenDrainIsolatorReleasesRemotelyDrivenLow(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "release", Kind: AnalysisTransient, DurationS: 2e-6, TimeStepS: 50e-9,
			Excitations: []SourceExcitation{
				{Component: "side_a_supply", DCValue: 3.3},
				{Component: "side_b_supply", DCValue: 5},
				{
					Component: "driver", PulseInitialValue: 0, PulseValue: 3.3,
					PulseDelayS: 500e-9, PulseWidthS: 2e-6, PulsePeriodS: 4e-6,
				},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "release", Node: "SDA2", Quantity: QuantityVoltageV, TimeS: 2e-6, Min: 4.98, Max: 5.01},
			{AnalysisID: "release", Node: "SDA2", Quantity: QuantityRiseTimeS, Min: 0, Max: 500e-9},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("side_a_supply", "VDD1", "GND1"),
		voltageSourceEvidence("side_b_supply", "VDD2", "GND2"),
		voltageSourceEvidence("driver", "HOST_SDA", "GND1"),
		resistorEvidence("host_series", 22, "HOST_SDA", "SDA1"),
		resistorEvidence("sda1_pullup", 4700, "VDD1", "SDA1"),
		resistorEvidence("sda2_pullup", 4700, "VDD2", "SDA2"),
		resistorEvidence("scl1_pullup", 4700, "VDD1", "SCL1"),
		resistorEvidence("scl2_pullup", 4700, "VDD2", "SCL2"),
		{
			InstanceID: "isolator", CatalogID: "isolator", Family: "isolator",
			ModelClaims: []CatalogEvidence{{
				ModelID:    PrimitiveBidirectionalOpenDrainIsolatorV1,
				Parameters: openDrainIsolatorParameters(),
			}},
			Connections: []ConnectionEvidence{
				{Function: "SDA1", Net: "SDA1"}, {Function: "SCL1", Net: "SCL1"},
				{Function: "SDA2", Net: "SDA2"}, {Function: "SCL2", Net: "SCL2"},
				{Function: "VDD1", Net: "VDD1"}, {Function: "GND1", Net: "GND1"},
				{Function: "VDD2", Net: "VDD2"}, {Function: "GND2", Net: "GND2"},
			},
		},
	}
	for index := range components {
		if components[index].InstanceID == "host_series" {
			components[index].Usage = "transient_series_impedance"
		}
	}
	nodes := []NodeEvidence{
		{Name: "GND1", Role: "ground"}, {Name: "GND2"}, {Name: "VDD1"}, {Name: "VDD2"},
		{Name: "HOST_SDA"}, {Name: "SDA1"}, {Name: "SDA2"}, {Name: "SCL1"}, {Name: "SCL2"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("isolator release resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("isolator release report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalTVSClampsBothPolarities(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: -12}}},
			{ID: "positive", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 12}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: -9.52, Max: -9.50},
			{AnalysisID: "positive", Node: "OUT", Quantity: QuantityVoltageV, Min: 9.50, Max: 9.52},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("series", 100, "SOURCE", "OUT"),
		{InstanceID: "clamp", CatalogID: "tvs", Family: "protection", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalTVSV1, Parameters: tvsParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "SOURCE"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestBidirectionalTVSSmallSignalIncludesCatalogJunctionCapacitance(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "response", Kind: AnalysisACSweep, StartFrequencyHz: 1e7, StopFrequencyHz: 1e7, Points: 2,
			Excitations: []SourceExcitation{{Component: "source", ACMagnitude: 1}},
		}},
		Assertions: []Assertion{{AnalysisID: "response", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1e7, Min: .034, Max: .037}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("series", 10_000, "SOURCE", "OUT"),
		{InstanceID: "clamp", CatalogID: "tvs", Family: "protection", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalTVSV1, Parameters: tvsParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "SOURCE"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestPiecewiseLinearRegionStableRejectsBoundaryCrossing(t *testing.T) {
	system := mnaSystem{nodeIndex: map[string]int{"OUT": 0}}
	device := compiledNonlinearDevice{
		primitive:  PrimitiveBidirectionalTVSV1,
		terminals:  map[string]string{"ANODE": "OUT", "CATHODE": "GND"},
		parameters: map[string]float64{"breakdown_voltage_v": 5},
	}
	if !piecewiseLinearRegionStable([]compiledNonlinearDevice{device}, &system, []complex128{1}, []complex128{4}) {
		t.Fatal("same off-region TVS step was not recognized as exact")
	}
	if piecewiseLinearRegionStable([]compiledNonlinearDevice{device}, &system, []complex128{1}, []complex128{6}) {
		t.Fatal("TVS breakdown-region crossing was accepted as exact")
	}
}

func TestPiecewiseLinearRegionStableAcceptsForcedMOSFETActiveSetStep(t *testing.T) {
	system := mnaSystem{nodeIndex: map[string]int{"GATE": 0, "SOURCE": 1}}
	device := compiledNonlinearDevice{
		primitive: PrimitivePMOSSwitchV1,
		terminals: map[string]string{"GATE": "GATE", "SOURCE": "SOURCE"},
		parameters: map[string]float64{
			"gate_on_voltage_v":        4.5,
			parameterForcedMOSFETState: 1,
		},
		polarity: -1,
	}
	if !piecewiseLinearRegionStable([]compiledNonlinearDevice{device}, &system, []complex128{0, 0}, []complex128{0, 5}) {
		t.Fatal("forced MOSFET active-set step was incorrectly damped at its physical gate boundary")
	}
}

func TestNonlinearResidualIsNormalizedByResolvedEquationScale(t *testing.T) {
	base := mnaSystem{
		matrix:        [][]complex128{{1e6}},
		rhs:           []complex128{1},
		unknownLabels: []string{"branch_current:high_gain"},
	}
	residual, label := nonlinearResidual(base, nil, []complex128{complex(1.000000000001e-6, 0)})
	if label != "branch_current:high_gain" || residual >= nonlinearResidualTolerance {
		t.Fatalf("normalized residual = %.12g label=%q", residual, label)
	}
}

func TestNonlinearIterationConvergenceAcceptsOnlyResidualBoundedNumericalFloor(t *testing.T) {
	if !nonlinearIterationConverged(5e-8, 5e-11, nonlinearResidualTolerance/2) {
		t.Fatal("residual-bounded nanovolt update should be accepted at the numerical floor")
	}
	if nonlinearIterationConverged(1e-6, 5e-11, 0) {
		t.Fatal("meaningful voltage update must not be hidden by a zero residual")
	}
	if nonlinearIterationConverged(nonlinearResidualFloorUpdateV*1.01, 5e-11, 0) {
		t.Fatal("voltage update above the numerical floor must still block convergence")
	}
	if nonlinearIterationConverged(5e-8, 5e-9, 0) {
		t.Fatal("meaningful branch-current update must still block convergence")
	}
	if nonlinearIterationConverged(5e-8, 5e-11, nonlinearResidualTolerance*2) {
		t.Fatal("nonzero normalized residual above tolerance must still block convergence")
	}
}

func TestIntrinsicSourceContinuationScalesRegisteredInternalSources(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{
			PrimitiveModel: PrimitiveProgrammableCurrentSourceV1,
			ModelParameters: []NamedValue{
				{Name: "reference_current_a", Value: 10e-6},
				{Name: "offset_voltage_v", Value: .004},
				{Name: "min_headroom_v", Value: 1.65},
			},
		},
		{
			PrimitiveModel:  PrimitiveShuntVoltageReferenceV1,
			ModelParameters: []NamedValue{{Name: "output_voltage_v", Value: 1.25}, {Name: "min_bias_current_a", Value: 10e-6}},
		},
		{
			PrimitiveModel: PrimitiveFixedBuckModuleV1,
			ModelParameters: []NamedValue{
				{Name: "output_voltage_v", Value: 12},
				{Name: "input_current_reference_voltage_v", Value: 24},
			},
		},
		{
			PrimitiveModel: PrimitiveProtectedIsolatedConverterV1,
			ModelParameters: []NamedValue{
				{Name: "output_voltage_v", Value: 12},
				{Name: "input_min_v", Value: 9},
			},
		},
		{
			PrimitiveModel: PrimitiveCurrentLimitingEFuseV1,
			ModelParameters: []NamedValue{
				{Name: "input_min_v", Value: 4.2},
				{Name: "input_max_v", Value: 60},
				{Name: "enable_high_voltage_v", Value: .94},
			},
		},
	}}
	plan = indexMNAPlanDevices(plan)
	scaled := planWithIntrinsicSourceContinuationScale(plan, .2)
	currentParameters := deviceParameterMap(scaled.Devices[0])
	referenceParameters := deviceParameterMap(scaled.Devices[1])
	buckParameters := deviceParameterMap(scaled.Devices[2])
	converterParameters := deviceParameterMap(scaled.Devices[3])
	efuseParameters := deviceParameterMap(scaled.Devices[4])
	if math.Abs(currentParameters["reference_current_a"]-2e-6) > 1e-15 ||
		math.Abs(currentParameters["offset_voltage_v"]-.0008) > 1e-15 ||
		currentParameters["min_headroom_v"] != 1.65 ||
		math.Abs(referenceParameters["output_voltage_v"]-.25) > 1e-15 ||
		referenceParameters["min_bias_current_a"] != 10e-6 ||
		math.Abs(buckParameters["output_voltage_v"]-2.4) > 1e-15 ||
		buckParameters["input_current_reference_voltage_v"] != 24 ||
		math.Abs(converterParameters["output_voltage_v"]-2.4) > 1e-15 ||
		converterParameters["input_min_v"] != 9 ||
		math.Abs(efuseParameters["input_min_v"]-.84) > 1e-15 ||
		math.Abs(efuseParameters["enable_high_voltage_v"]-.188) > 1e-15 ||
		efuseParameters["input_max_v"] != 60 {
		t.Fatalf("scaled source parameters = %#v %#v %#v %#v %#v", currentParameters, referenceParameters, buckParameters, converterParameters, efuseParameters)
	}
	if deviceParameterMap(plan.Devices[0])["reference_current_a"] != 10e-6 ||
		deviceParameterMap(plan.Devices[2])["output_voltage_v"] != 12 ||
		deviceParameterMap(plan.Devices[3])["output_voltage_v"] != 12 ||
		deviceParameterMap(plan.Devices[4])["input_min_v"] != 4.2 {
		t.Fatal("source continuation mutated the original plan")
	}
}

func TestOpAmpGainContinuationUpdatesIndexedParameters(t *testing.T) {
	plan := indexMNAPlanDevices(Plan{Devices: []ResolvedDevice{{
		PrimitiveModel:  PrimitiveOpAmpV1,
		ModelParameters: []NamedValue{{Name: "dc_open_loop_gain", Value: 100000}, {Name: "supply_min_v", Value: 2.7}},
	}}})
	scaled := planWithOpAmpGainScale(plan, .01)
	if gain := deviceParameterMap(scaled.Devices[0])["dc_open_loop_gain"]; gain != 1000 {
		t.Fatalf("scaled indexed op-amp gain = %.12g", gain)
	}
	if supply := deviceParameterMap(scaled.Devices[0])["supply_min_v"]; supply != 2.7 {
		t.Fatalf("unrelated indexed parameter changed to %.12g", supply)
	}
	if gain := deviceParameterMap(plan.Devices[0])["dc_open_loop_gain"]; gain != 100000 {
		t.Fatalf("source plan gain mutated to %.12g", gain)
	}
}

func TestSourceContinuationScalesOnlyVoltageValuedActiveDeviceStates(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "controller", PrimitiveModel: PrimitiveOpAmpV1},
		{Component: "sensor", PrimitiveModel: PrimitiveCurrentSenseAmplifierV1},
		{Component: "decision", PrimitiveModel: PrimitiveComparatorOpenCollectorV1},
		{Component: "pass", PrimitiveModel: PrimitiveBJTNPNV1},
	}}
	states := map[string]float64{
		"controller": 23.7,
		"sensor":     4.8,
		"decision":   1,
		"pass":       1,
	}
	scaled := activeDeviceStatesWithSourceContinuationScale(plan, states, .05)
	if math.Abs(scaled["controller"]-1.185) > 1e-12 || math.Abs(scaled["sensor"]-.24) > 1e-12 {
		t.Fatalf("voltage-valued states were not source-scaled: %#v", scaled)
	}
	if scaled["decision"] != 1 || scaled["pass"] != 1 {
		t.Fatalf("discrete active-device states changed: %#v", scaled)
	}
	if states["controller"] != 23.7 || states["sensor"] != 4.8 {
		t.Fatalf("source states mutated: %#v", states)
	}
	if identity := activeDeviceStatesWithSourceContinuationScale(plan, states, 1); identity["controller"] != 23.7 {
		t.Fatalf("full-scale states changed: %#v", identity)
	}
}

func TestGroundReferencedSourceProjectionBalancesAcceptedDampedState(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{{
		Component: "supply", PrimitiveModel: PrimitiveVoltageSourceV1,
		Terminals: []TerminalBinding{{Terminal: "POSITIVE", Net: "VP"}, {Terminal: "NEGATIVE", Net: "GND"}},
	}}}
	base := mnaSystem{
		matrix:        [][]complex128{{1, 1}, {1, 0}},
		rhs:           []complex128{0, 1.2},
		unknownLabels: []string{"node:VP", "branch_current:supply"},
		nodeIndex:     map[string]int{"VP": 0},
		branchIndex:   map[string]int{"supply": 1},
	}
	candidate := []complex128{1.2, -1.2}
	guess := []complex128{.2, -.01}
	voltageUpdate, currentUpdate := projectGroundReferencedVoltageSources(plan, &base, nil, candidate, guess)
	if math.Abs(real(guess[0])-1.2) > 1e-12 || math.Abs(real(guess[1])+1.2) > 1e-12 {
		t.Fatalf("projected source state = %#v", guess)
	}
	if math.Abs(voltageUpdate-1) > 1e-12 || math.Abs(currentUpdate-1.19) > 1e-12 {
		t.Fatalf("projection updates = voltage %.12g current %.12g", voltageUpdate, currentUpdate)
	}
	if residual, _ := nonlinearResidual(base, nil, guess); residual != 0 {
		t.Fatalf("projected residual = %.12g", residual)
	}
}

func TestHighSidePNPDriveChainUsesJunctionAwareContinuation(t *testing.T) {
	device := func(component, primitive string, terminals ...TerminalBinding) ResolvedDevice {
		return ResolvedDevice{Component: component, PrimitiveModel: primitive, Terminals: terminals}
	}
	plan := Plan{Devices: []ResolvedDevice{
		device("controller", PrimitiveOpAmpV1,
			TerminalBinding{Terminal: "OUT", Net: "DRIVE"}, TerminalBinding{Terminal: "V_PLUS", Net: "VP"}),
		device("drive_resistor", PrimitiveResistorV1,
			TerminalBinding{Terminal: "A", Net: "DRIVE"}, TerminalBinding{Terminal: "B", Net: "DRIVER_BASE"}),
		device("driver", PrimitiveBJTPNPV1,
			TerminalBinding{Terminal: "BASE", Net: "DRIVER_BASE"}, TerminalBinding{Terminal: "COLLECTOR", Net: "PASS_BASE"}, TerminalBinding{Terminal: "EMITTER", Net: "VP"}),
		device("pass", PrimitiveBJTNPNV1,
			TerminalBinding{Terminal: "BASE", Net: "PASS_BASE"}, TerminalBinding{Terminal: "COLLECTOR", Net: "VP"}, TerminalBinding{Terminal: "EMITTER", Net: "OUT"}),
	}}
	stages := nonlinearContinuationForPlan(plan)
	if !hasSupplyReferencedPNPDriveChain(plan) || len(stages) == 0 || stages[0].sourceScale != .75 || stages[0].gainScale != 1e-8 {
		t.Fatalf("high-side drive continuation = %#v", stages)
	}
	plan.Devices[3].Terminals[1].Net = "OTHER_SUPPLY"
	if hasSupplyReferencedPNPDriveChain(plan) || nonlinearContinuationForPlan(plan)[0].sourceScale != .05 {
		t.Fatal("unrelated PNP stage selected the high-side drive continuation")
	}
}

func TestDirectHighSideNPNPassUsesJunctionAwareClampedContinuation(t *testing.T) {
	device := func(component, primitive string, terminals ...TerminalBinding) ResolvedDevice {
		return ResolvedDevice{Component: component, PrimitiveModel: primitive, Terminals: terminals}
	}
	plan := Plan{Devices: []ResolvedDevice{
		device("controller", PrimitiveOpAmpV1,
			TerminalBinding{Terminal: "OUT", Net: "DRIVE"}, TerminalBinding{Terminal: "IN_MINUS", Net: "SENSE"}, TerminalBinding{Terminal: "V_PLUS", Net: "VP"}),
		device("drive_resistor", PrimitiveResistorV1,
			TerminalBinding{Terminal: "A", Net: "DRIVE"}, TerminalBinding{Terminal: "B", Net: "PASS_BASE"}),
		device("pass", PrimitiveBJTNPNV1,
			TerminalBinding{Terminal: "BASE", Net: "PASS_BASE"}, TerminalBinding{Terminal: "COLLECTOR", Net: "VP"}, TerminalBinding{Terminal: "EMITTER", Net: "OUT"}),
		device("feedback", PrimitiveResistorV1,
			TerminalBinding{Terminal: "A", Net: "OUT"}, TerminalBinding{Terminal: "B", Net: "SENSE"}),
	}}
	if !hasDirectSupplyReferencedNPNPass(plan) || nonlinearClampedContinuationForPlan(plan)[0].sourceScale != .75 || nonlinearContinuationForPlan(plan)[0].sourceScale != .05 {
		t.Fatal("direct high-side clamped continuation was not isolated from the ordinary schedule")
	}
	plan.Devices[2].Terminals[1].Net = "OTHER_SUPPLY"
	if hasDirectSupplyReferencedNPNPass(plan) || nonlinearClampedContinuationForPlan(plan)[0].sourceScale != .05 {
		t.Fatal("unrelated NPN selected direct high-side clamped continuation")
	}
}

func TestVoltageClampContinuationSpendsStagesOnGminInsteadOfInactiveGain(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{{Component: "controller", PrimitiveModel: PrimitiveOpAmpV1}}}
	if !hasSingleAffineProjectionFeedbackDevice(plan) ||
		!hasSingleVoltageValuedActiveDeviceClamp(plan, map[string]float64{"controller": 2.5}) ||
		hasSingleVoltageValuedActiveDeviceClamp(plan, map[string]float64{"decision": 1}) {
		t.Fatal("voltage-valued active-device clamp classification failed")
	}
	multiple := Plan{Devices: append(append([]ResolvedDevice(nil), plan.Devices...), ResolvedDevice{
		Component: "second_controller", PrimitiveModel: PrimitiveOpAmpV1,
	})}
	if hasSingleAffineProjectionFeedbackDevice(multiple) ||
		hasSingleVoltageValuedActiveDeviceClamp(multiple, map[string]float64{"controller": 2.5, "second_controller": 2.5}) {
		t.Fatal("multi-controller loop incorrectly selected the single-loop continuation")
	}
	stages := nonlinearClampedContinuationForPlan(plan)
	if len(stages) > nonlinearMaxContinuationStages || stages[len(stages)-1].gmin != nonlinearFinalGmin {
		t.Fatalf("clamped continuation bounds = %#v", stages)
	}
	for _, stage := range stages {
		if stage.gainScale != 1 {
			t.Fatalf("clamped continuation changed inactive gain: %#v", stage)
		}
	}
}

func resolveNonlinearTestPlan(t *testing.T, components []ComponentEvidence, nodes []NodeEvidence, assertions []Assertion) Plan {
	t.Helper()
	plan, diagnostics := ResolveWithTopology(nonlinearTestIntent(assertions), "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func nonlinearTestIntent(assertions []Assertion) Intent {
	return Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}}, Assertions: assertions}
}

func voltageSourceEvidence(id, positive, negative string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "source.voltage", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: positive}, {Function: "NEGATIVE", Net: negative}}}
}

func resistorEvidence(id string, value float64, a, b string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "resistor", Family: "resistor", ValueSI: value, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: a}, {Function: "B", Net: b}}}
}

func diodeParameters(maxCurrent, maxReverse float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 4e-9}, {Name: "emission_coefficient", Value: 1.9}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_forward_current_a", Value: maxCurrent}, {Name: "max_reverse_voltage_v", Value: maxReverse}}
}

func tvsParameters() []NamedValue {
	return []NamedValue{
		{Name: "breakdown_voltage_v", Value: 9.5},
		{Name: "dynamic_resistance_ohm", Value: .5},
		{Name: "junction_capacitance_f", Value: 45e-12},
		{Name: "max_pulse_current_a", Value: 12},
		{Name: "off_resistance_ohm", Value: 50e6},
	}
}

func openDrainTranslatorParameters() []NamedValue {
	return []NamedValue{
		{Name: "vcca_min_v", Value: 1.65}, {Name: "vcca_max_v", Value: 3.6},
		{Name: "vccb_min_v", Value: 2.3}, {Name: "vccb_max_v", Value: 5.5},
		{Name: "low_level_threshold_v", Value: .15}, {Name: "enable_high_ratio", Value: .65},
		{Name: "channel_on_resistance_ohm", Value: 400}, {Name: "channel_off_resistance_ohm", Value: 2.75e6},
		{Name: "max_channel_current_a", Value: .05},
		{Name: "vcca_quiescent_current_a", Value: 2.4e-6}, {Name: "vccb_quiescent_current_a", Value: 12e-6},
		{Name: "max_temperature_c", Value: 150}, {Name: "junction_to_ambient_c_per_w", Value: 239.8},
	}
}

func pushPullTranslatorParameters(direction float64) []NamedValue {
	return []NamedValue{
		{Name: "vcca_min_v", Value: 1.65}, {Name: "vcca_max_v", Value: 3.6},
		{Name: "vccb_min_v", Value: 2.3}, {Name: "vccb_max_v", Value: 5.5},
		{Name: "input_low_max_v", Value: .15}, {Name: "input_high_headroom_v", Value: .2},
		{Name: "input_high_headroom_a_v", Value: .2}, {Name: "input_high_headroom_b_v", Value: .4},
		{Name: "enable_high_ratio", Value: .65}, {Name: "direction", Value: direction},
		{Name: "output_low_resistance_ohm", Value: 400}, {Name: "output_high_resistance_ohm", Value: 16500},
		{Name: "output_off_resistance_ohm", Value: 2.75e6},
		{Name: "max_sink_current_a", Value: .001}, {Name: "max_source_current_a", Value: .00002},
		{Name: "max_transient_sink_current_a", Value: .05}, {Name: "max_transient_source_current_a", Value: .05},
		{Name: "vcca_quiescent_current_a", Value: 2.4e-6}, {Name: "vccb_quiescent_current_a", Value: 12e-6},
		{Name: "max_temperature_c", Value: 150}, {Name: "junction_to_ambient_c_per_w", Value: 120.1},
	}
}

func TestPushPullTranslatorUsesPortSpecificHighThresholds(t *testing.T) {
	parameters := map[string]float64{
		"input_high_headroom_v":   .3,
		"input_high_headroom_a_v": .2,
		"input_high_headroom_b_v": .4,
	}
	if got := pushPullTranslatorInputHeadroom(parameters, "A"); got != .2 {
		t.Fatalf("A-port high headroom = %v, want 0.2", got)
	}
	if got := pushPullTranslatorInputHeadroom(parameters, "B"); got != .4 {
		t.Fatalf("B-port high headroom = %v, want 0.4", got)
	}
}

func pushPullTranslatorEvidence(id, enableNet string, direction float64) ComponentEvidence {
	connections := []ConnectionEvidence{
		{Function: "VCCA", Net: "VCCA"}, {Function: "VCCB", Net: "VCCB"},
		{Function: "GND", Net: "GND"}, {Function: "OE", Net: enableNet},
	}
	for channel := 1; channel <= pushPullTranslatorChannels; channel++ {
		connections = append(connections,
			ConnectionEvidence{Function: fmt.Sprintf("A%d", channel), Net: fmt.Sprintf("A%d", channel)},
			ConnectionEvidence{Function: fmt.Sprintf("B%d", channel), Net: fmt.Sprintf("B%d", channel)},
		)
	}
	return ComponentEvidence{
		InstanceID: id, CatalogID: "translator", Family: "level_translator",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitivePushPullTranslatorV1, Parameters: pushPullTranslatorParameters(direction)}},
		Connections: connections,
	}
}

func directionControlledTranslatorParameters() []NamedValue {
	return []NamedValue{
		{Name: "vcca_min_v", Value: .65}, {Name: "vcca_max_v", Value: 3.6},
		{Name: "vccb_min_v", Value: .65}, {Name: "vccb_max_v", Value: 3.6},
		{Name: "input_low_ratio", Value: .2}, {Name: "input_high_ratio", Value: .7},
		{Name: "control_low_ratio", Value: .2}, {Name: "control_high_ratio", Value: .7},
		{Name: "output_low_resistance_ohm", Value: 2000}, {Name: "output_high_resistance_ohm", Value: 2000},
		{Name: "output_off_resistance_ohm", Value: 450000},
		{Name: "max_sink_current_a", Value: .00005}, {Name: "max_source_current_a", Value: .00005},
		{Name: "vcca_quiescent_current_a", Value: .00004}, {Name: "vccb_quiescent_current_a", Value: .000038},
		{Name: "max_temperature_c", Value: 150}, {Name: "junction_to_ambient_c_per_w", Value: 92},
	}
}

func directionControlledTranslatorEvidence(id, enableNet string) ComponentEvidence {
	connections := []ConnectionEvidence{
		{Function: "VCCA", Net: "VCCA"}, {Function: "VCCB", Net: "VCCB"}, {Function: "GND", Net: "GND"},
		{Function: "OE", Net: enableNet}, {Function: "DIR1", Net: "DIR1"}, {Function: "DIR2", Net: "DIR2"},
	}
	for channel := 1; channel <= directionControlledTranslatorChannels; channel++ {
		connections = append(connections,
			ConnectionEvidence{Function: fmt.Sprintf("A%d", channel), Net: fmt.Sprintf("A%d", channel)},
			ConnectionEvidence{Function: fmt.Sprintf("B%d", channel), Net: fmt.Sprintf("B%d", channel)},
		)
	}
	return ComponentEvidence{
		InstanceID: id, CatalogID: "transceiver", Family: "level_translator",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDirectionControlledTranslatorV1, Parameters: directionControlledTranslatorParameters()}},
		Connections: connections,
	}
}

func pushPullIsolatorEvidence(id string) ComponentEvidence {
	parameters := []NamedValue{
		{Name: "supply_min_v", Value: 2.25}, {Name: "supply_max_v", Value: 5.5},
		{Name: "input_low_ratio", Value: .3}, {Name: "input_high_ratio", Value: .7}, {Name: "enable_high_ratio", Value: .7},
		{Name: "output_low_resistance_ohm", Value: 100}, {Name: "output_high_resistance_ohm", Value: 100},
		{Name: "output_off_resistance_ohm", Value: 1e9}, {Name: "max_output_current_a", Value: .004},
		{Name: "side_1_quiescent_current_a", Value: .0015}, {Name: "side_2_quiescent_current_a", Value: .0015},
		{Name: "isolation_resistance_ohm", Value: 1e12}, {Name: "isolation_working_voltage_v", Value: 1000},
		{Name: "max_temperature_c", Value: 150}, {Name: "junction_to_ambient_c_per_w", Value: 80},
	}
	connections := []ConnectionEvidence{}
	for _, function := range []string{"INA1", "INA2", "INA3", "INB4", "OUTB1", "OUTB2", "OUTB3", "OUTA4", "VDD1", "GND1", "VDD2", "GND2", "EN1", "EN2"} {
		connections = append(connections, ConnectionEvidence{Function: function, Net: function})
	}
	return ComponentEvidence{
		InstanceID: id, CatalogID: "isolator", Family: "isolator",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitivePushPullDigitalIsolatorV1, Parameters: parameters}},
		Connections: connections,
	}
}

func openDrainIsolatorParameters() []NamedValue {
	return []NamedValue{
		{Name: "side_a_min_v", Value: 1.71}, {Name: "side_a_max_v", Value: 5.5},
		{Name: "side_b_min_v", Value: 1.71}, {Name: "side_b_max_v", Value: 5.5},
		{Name: "low_level_threshold_v", Value: .7},
		{Name: "output_on_resistance_ohm", Value: 300}, {Name: "output_off_resistance_ohm", Value: 5e8},
		{Name: "isolation_resistance_ohm", Value: 1e9}, {Name: "max_output_current_a", Value: .003},
		{Name: "side_a_quiescent_current_a", Value: .00025}, {Name: "side_b_quiescent_current_a", Value: .00025},
		{Name: "max_temperature_c", Value: 125}, {Name: "junction_to_ambient_c_per_w", Value: 120},
	}
}

func bjtParameters(maxCurrent, maxVoltage float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 1e-14}, {Name: "forward_beta", Value: 100}, {Name: "reverse_beta", Value: 1}, {Name: "emission_coefficient", Value: 1}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_collector_current_a", Value: maxCurrent}, {Name: "max_collector_emitter_voltage_v", Value: maxVoltage}}
}

func TestSiliconSaturationCurrentTracksTemperatureFromNominalReference(t *testing.T) {
	nominal := 1e-14
	if actual := siliconSaturationCurrentAtTemperature(nominal, 1, nonlinearNominalTemperatureK); math.Abs(actual-nominal) > nominal*1e-12 {
		t.Fatalf("nominal saturation current = %.12g, want %.12g", actual, nominal)
	}
	cold := siliconSaturationCurrentAtTemperature(nominal, 1, 273.15)
	hot := siliconSaturationCurrentAtTemperature(nominal, 1, 323.15)
	if !(cold > 0 && cold < nominal && hot > nominal) {
		t.Fatalf("temperature-adjusted saturation currents = cold %.12g nominal %.12g hot %.12g", cold, nominal, hot)
	}
}

func zenerParameters() []NamedValue {
	return []NamedValue{
		{Name: "forward_saturation_current_a", Value: 4.04e-11}, {Name: "forward_series_resistance_ohm", Value: 34.9}, {Name: "forward_emission_coefficient", Value: 1.1},
		{Name: "reverse_saturation_current_a", Value: 8.08e-15}, {Name: "reverse_series_resistance_ohm", Value: 13.1}, {Name: "reverse_emission_coefficient", Value: 3},
		{Name: "zener_offset_voltage_v", Value: 2.62}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_current_a", Value: .0980392156863},
	}
}

func nmosSwitchParameters() []NamedValue {
	return []NamedValue{
		{Name: "gate_on_voltage_v", Value: 2.5}, {Name: "on_resistance_ohm", Value: .048},
		{Name: "max_drain_current_a", Value: 5.7}, {Name: "max_drain_source_voltage_v", Value: 30}, {Name: "max_gate_source_voltage_v", Value: 12},
	}
}

func pmosSwitchParameters() []NamedValue {
	return []NamedValue{
		{Name: "gate_on_voltage_v", Value: 10}, {Name: "on_resistance_ohm", Value: .5},
		{Name: "max_drain_current_a", Value: 12}, {Name: "max_drain_source_voltage_v", Value: 200},
		{Name: "max_gate_source_voltage_v", Value: 20}, {Name: "max_temperature_c", Value: 150},
		{Name: "junction_to_ambient_c_per_w", Value: 40},
	}
}

func reverseBlockingLoadSwitchParameters() []NamedValue {
	return []NamedValue{
		{Name: "input_min_v", Value: 1}, {Name: "input_max_v", Value: 5.5},
		{Name: "enable_high_voltage_v", Value: 1}, {Name: "on_resistance_ohm", Value: .39},
		{Name: "reverse_blocking_release_voltage_v", Value: .025}, {Name: "reverse_leakage_current_a", Value: 1e-6},
		{Name: "max_output_current_a", Value: 2}, {Name: "max_output_voltage_v", Value: 5.5},
		{Name: "quiescent_current_a", Value: 1.2e-6}, {Name: "max_temperature_c", Value: 125},
		{Name: "junction_to_ambient_c_per_w", Value: 183},
	}
}

func currentLimitingEFuseParameters() []NamedValue {
	return []NamedValue{
		{Name: "input_min_v", Value: 4.2}, {Name: "input_max_v", Value: 60},
		{Name: "enable_high_voltage_v", Value: .94}, {Name: "on_resistance_ohm", Value: .15},
		{Name: "programmed_current_limit_a", Value: .3}, {Name: "minimum_current_limit_a", Value: .285},
		{Name: "maximum_current_limit_a", Value: .315}, {Name: "reverse_leakage_current_a", Value: 66e-6},
		{Name: "max_output_voltage_v", Value: 60}, {Name: "maximum_output_slew_v_per_s", Value: 1000},
		{Name: "quiescent_current_a", Value: 390e-6}, {Name: "max_temperature_c", Value: 125},
		{Name: "junction_to_ambient_c_per_w", Value: 38.6},
	}
}

func TestBoundedExponentialIsFinite(t *testing.T) {
	value, derivative := boundedExponential(1e9)
	if math.IsInf(value, 0) || math.IsInf(derivative, 0) || value <= 0 || derivative <= 0 {
		t.Fatalf("value=%g derivative=%g", value, derivative)
	}
}

func TestAdvanceActiveDeviceStateChangesOneDeviceInPlanOrder(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "first", PrimitiveModel: PrimitiveOpAmpV1},
		{Component: "second", PrimitiveModel: PrimitiveComparatorOpenCollectorV1},
	}}
	resolved := map[string]float64{"first": 4.9, "second": 1}

	first := advanceActiveDeviceState(plan, nil, resolved)
	if len(first) != 1 || first["first"] != 4.9 {
		t.Fatalf("first transition = %#v", first)
	}
	second := advanceActiveDeviceState(plan, first, resolved)
	if len(second) != 2 || second["first"] != 4.9 || second["second"] != 1 {
		t.Fatalf("second transition = %#v", second)
	}
}

func TestAdvanceActiveDeviceStateReleasesOpAmpBeforeOppositeRail(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{{Component: "amplifier", PrimitiveModel: PrimitiveOpAmpV1}}}
	next := advanceActiveDeviceState(plan, map[string]float64{"amplifier": 0.1}, map[string]float64{"amplifier": 4.9})
	if _, clamped := next["amplifier"]; clamped {
		t.Fatalf("op-amp jumped directly between rail clamps: %#v", next)
	}
}

func TestAdvanceActiveDeviceStateMovesInteriorSeedDirectlyToResolvedRail(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{{
		Component: "amplifier", PrimitiveModel: PrimitiveOpAmpV1,
		Terminals:       []TerminalBinding{{Terminal: "V_MINUS", Net: "GND"}, {Terminal: "V_PLUS", Net: "VCC"}},
		ModelParameters: []NamedValue{{Name: "output_low_margin_v", Value: .1}, {Name: "output_high_margin_v", Value: .1}},
	}}}
	system := mnaSystem{nodeIndex: map[string]int{"GND": 0, "VCC": 1}}
	solution := []complex128{0, 5}
	next := advanceActiveDeviceStateAtOperatingPoint(
		plan, &system, solution,
		map[string]float64{"amplifier": 2.5},
		map[string]float64{"amplifier": .1},
	)
	if got := next["amplifier"]; got != .1 {
		t.Fatalf("interior seed transitioned to %g; want resolved low rail", got)
	}
}

func TestActiveDeviceClampAtOutputRailRejectsMissingSupplyTerminal(t *testing.T) {
	device := ResolvedDevice{
		Component: "amplifier", PrimitiveModel: PrimitiveOpAmpV1,
		Terminals:       []TerminalBinding{{Terminal: "V_MINUS", Net: "GND"}},
		ModelParameters: []NamedValue{{Name: "output_low_margin_v", Value: .1}, {Name: "output_high_margin_v", Value: .1}},
	}
	system := mnaSystem{nodeIndex: map[string]int{"GND": 0}}
	if activeDeviceClampAtOutputRail(device, .1, &system, []complex128{0}) {
		t.Fatal("incomplete op-amp terminal binding was accepted as an output-rail clamp")
	}
}

func TestAdvanceActiveDeviceStatePrioritizesLinearOutputBeforeComparatorOrder(t *testing.T) {
	plan := Plan{Devices: []ResolvedDevice{
		{Component: "decision", PrimitiveModel: PrimitiveComparatorOpenCollectorV1},
		{Component: "filter", PrimitiveModel: PrimitiveOpAmpV1},
	}}
	current := map[string]float64{"decision": 1, "filter": .065}
	resolved := map[string]float64{"decision": 0, "filter": 4.38}
	next := advanceActiveDeviceState(plan, current, resolved)
	if next["decision"] != 1 {
		t.Fatalf("comparator changed before upstream linear output: %#v", next)
	}
	if _, clamped := next["filter"]; clamped {
		t.Fatalf("upstream op-amp clamp was not released first: %#v", next)
	}
}
