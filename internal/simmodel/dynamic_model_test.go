package simmodel

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestInductorPrimitiveExecutesACAndBackwardEulerTransient(t *testing.T) {
	parameters := []NamedValue{
		{Name: "rated_current_a", Value: 2},
		{Name: "saturation_current_a", Value: 3},
		{Name: "series_resistance_ohm", Value: .1},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "VIN", "GND"),
		{
			InstanceID: "inductor", CatalogID: "inductor.reviewed", Family: "inductor",
			ValueSI: 1e-3, HasValueSI: true,
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveInductorTransientV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "OUT"}},
		},
		resistorEvidence("load", 9.9, "OUT", "GND"),
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "OUT"}}

	transientIntent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "step", Kind: AnalysisTransient, DurationS: 200e-6, TimeStepS: 10e-6,
			Excitations: []SourceExcitation{{
				Component: "source", PulseInitialValue: 0, PulseValue: 1,
				PulseDelayS: 100e-6, PulseWidthS: 1e-3, PulsePeriodS: 2e-3,
			}},
		}},
		Assertions: []Assertion{{AnalysisID: "step", Node: "OUT", Quantity: QuantityVoltageV, TimeS: 200e-6, Min: .55, Max: .70}},
	}
	plan, diagnostics := ResolveWithTopology(transientIntent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve transient inductor: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("transient inductor report=%#v diagnostics=%#v", report, diagnostics)
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 || !reflect.DeepEqual(report, replay) {
		t.Fatalf("transient inductor replay differs: diagnostics=%#v", replayDiagnostics)
	}

	acIntent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "ac", Kind: AnalysisACSweep, StartFrequencyHz: 15915.494309189535,
			StopFrequencyHz: 15915.494309189535, Points: 2,
			Excitations: []SourceExcitation{{Component: "source", ACMagnitude: 1}},
		}},
		Assertions: []Assertion{{AnalysisID: "ac", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 15915.494309189535, Min: .098, Max: .099}},
	}
	acPlan, diagnostics := ResolveWithTopology(acIntent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve AC inductor: %#v", diagnostics)
	}
	acReport, diagnostics := Evaluate(acPlan)
	if len(diagnostics) != 0 || acReport.Status != "pass" {
		t.Fatalf("AC inductor report=%#v diagnostics=%#v", acReport, diagnostics)
	}
}

func TestDynamicCatalogEvidenceIsBoundedCanonicalAndDeepCloned(t *testing.T) {
	pulse := 1e-3
	claim := CatalogEvidence{
		ModelID: PrimitiveNMOSSwitchV1,
		Parameters: append(nmosSwitchParameters(),
			NamedValue{Name: "junction_to_case_c_per_w", Value: 1},
			NamedValue{Name: "max_temperature_c", Value: 150},
		),
		ThermalModel: &ThermalRCNetwork{
			Reference: "junction_to_case",
			Stages: []ThermalRCStage{
				{ThermalResistanceCPerW: .2, ThermalCapacitanceJPerC: .005},
				{ThermalResistanceCPerW: .8, ThermalCapacitanceJPerC: .125},
			},
			BoundaryAssumption: "Case held at the declared operating-case temperature.",
		},
		TransientSOA: []TransientSOAEnvelope{{
			PulseDurationS: &pulse, CaseTemperatureC: 25,
			Points: []TransientSOAPoint{{VoltageV: 5, CurrentA: 5}, {VoltageV: 30, CurrentA: 1}},
		}, {
			DC: true, CaseTemperatureC: 25,
			Points: []TransientSOAPoint{{VoltageV: 5, CurrentA: 2}, {VoltageV: 30, CurrentA: .25}},
		}},
	}
	if diagnostics := ValidateCatalogEvidence("mosfet", []CatalogEvidence{claim}); len(diagnostics) != 0 {
		t.Fatalf("valid dynamic catalog evidence rejected: %#v", diagnostics)
	}
	clone := CloneCatalogEvidence(claim)
	clone.ThermalModel.Stages[0].ThermalResistanceCPerW = 99
	clone.TransientSOA[0].Points[0].CurrentA = 99
	*clone.TransientSOA[0].PulseDurationS = .5
	if claim.ThermalModel.Stages[0].ThermalResistanceCPerW != .2 ||
		claim.TransientSOA[0].Points[0].CurrentA != 5 ||
		math.Abs(*claim.TransientSOA[0].PulseDurationS-1e-3) > 1e-15 {
		t.Fatal("dynamic catalog evidence clone aliases source storage")
	}

	invalid := CloneCatalogEvidence(claim)
	invalid.ThermalModel.Stages[1], invalid.ThermalModel.Stages[0] = invalid.ThermalModel.Stages[0], invalid.ThermalModel.Stages[1]
	invalid.TransientSOA[0].Points[1].CurrentA = 6
	diagnostics := ValidateCatalogEvidence("mosfet", []CatalogEvidence{invalid})
	if len(diagnostics) < 2 {
		t.Fatalf("noncanonical dynamic evidence accepted: %#v", diagnostics)
	}
	joined := ""
	for _, diagnostic := range diagnostics {
		joined += diagnostic.Message + "\n"
	}
	if !strings.Contains(joined, "time constant") || !strings.Contains(joined, "must not increase") {
		t.Fatalf("dynamic evidence diagnostics are unstable or incomplete: %s", joined)
	}
}

func TestMOSFETDynamicCapacitancesDelayCatalogSwitching(t *testing.T) {
	parameters := append(nmosSwitchParameters(),
		NamedValue{Name: "gate_threshold_max_v", Value: 1.5},
		NamedValue{Name: "input_capacitance_f", Value: 1e-9},
		NamedValue{Name: "output_capacitance_f", Value: 100e-12},
		NamedValue{Name: "reverse_transfer_capacitance_f", Value: 20e-12},
	)
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "gate_step", Kind: AnalysisTransient, DurationS: 120e-6, TimeStepS: 1e-6,
			Excitations: []SourceExcitation{
				{Component: "gate_drive", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 1e-6, PulseWidthS: 200e-6, PulsePeriodS: 400e-6},
				{Component: "supply", DCValue: 5},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "gate_step", Node: "DRAIN", Quantity: QuantityVoltageV, TimeS: 20e-6, Min: 4.9, Max: 5.01},
			{AnalysisID: "gate_step", Node: "DRAIN", Quantity: QuantityVoltageV, TimeS: 100e-6, Min: 0, Max: .01},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("gate_drive", "DRIVE", "GND"),
		voltageSourceEvidence("supply", "VCC", "GND"),
		resistorEvidence("gate_resistor", 100e3, "DRIVE", "GATE"),
		resistorEvidence("load", 1e3, "VCC", "DRAIN"),
		{
			InstanceID: "switch", CatalogID: "mosfet.reviewed", Family: "mosfet",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveNMOSSwitchV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "GATE", Net: "GATE"},
				{Function: "DRAIN", Net: "DRAIN"},
				{Function: "SOURCE", Net: "GND"},
			},
		},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "DRIVE"}, {Name: "GATE"}, {Name: "DRAIN"}, {Name: "VCC"}}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve dynamic MOSFET: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("dynamic MOSFET report=%#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMOSFETDynamicCapacitancesAreScopedToWaveformAnalyses(t *testing.T) {
	device := ResolvedDevice{
		PrimitiveModel: PrimitiveNMOSSwitchV1,
		ModelParameters: []NamedValue{
			{Name: "input_capacitance_f", Value: 1e-9},
			{Name: "output_capacitance_f", Value: 100e-12},
			{Name: "reverse_transfer_capacitance_f", Value: 20e-12},
		},
		Terminals: []TerminalBinding{
			{Terminal: "GATE", Net: "GATE"},
			{Terminal: "DRAIN", Net: "DRAIN"},
			{Terminal: "SOURCE", Net: "SOURCE"},
		},
	}
	if capacitors := mosfetDynamicCapacitors(device, AnalysisStartup); len(capacitors) != 0 {
		t.Fatalf("startup source ramp stamped waveform parasitics: %#v", capacitors)
	}
	for _, kind := range []string{AnalysisACSweep, AnalysisTransient, AnalysisElectrothermal} {
		if capacitors := mosfetDynamicCapacitors(device, kind); len(capacitors) != 3 {
			t.Fatalf("%s dynamic capacitances = %#v", kind, capacitors)
		}
	}
}

func TestElectrothermalTransientIntegratesReviewedThermalRCAndSOA(t *testing.T) {
	pulse := 1e-3
	longPulse := 10e-3
	parameters := append(nmosSwitchParameters(),
		NamedValue{Name: "max_temperature_c", Value: 150},
		NamedValue{Name: "junction_to_ambient_c_per_w", Value: 10},
	)
	claim := CatalogEvidence{
		ModelID:    PrimitiveNMOSSwitchV1,
		Parameters: parameters,
		ThermalModel: &ThermalRCNetwork{
			Reference: "junction_to_ambient",
			Stages: []ThermalRCStage{{
				ThermalResistanceCPerW:  10,
				ThermalCapacitanceJPerC: .001,
			}},
			BoundaryAssumption: "Reviewed board boundary held at the declared ambient temperature.",
		},
		TransientSOA: []TransientSOAEnvelope{
			{PulseDurationS: &pulse, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: 2}, {VoltageV: 30, CurrentA: .1}}},
			{PulseDurationS: &longPulse, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: 1}, {VoltageV: 30, CurrentA: .05}}},
			{DC: true, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: .5}, {VoltageV: 30, CurrentA: .025}}},
		},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "load_event", Kind: AnalysisElectrothermal, DurationS: 2e-3, TimeStepS: 100e-6,
			Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 25}},
			Excitations: []SourceExcitation{
				{Component: "gate_drive", DCValue: 5},
				{Component: "supply", PulseInitialValue: 0, PulseValue: 10, PulseDelayS: 100e-6, PulseWidthS: 5e-3, PulsePeriodS: 10e-3},
			},
		}},
		Assertions: []Assertion{
			{AnalysisID: "load_event", Component: "switch", Quantity: QuantityJunctionTemperatureC, Min: 25, Max: 26},
			{AnalysisID: "load_event", Component: "switch", Quantity: QuantityTransientSOAMargin, Min: 5, Max: maxMNASolutionValue},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("gate_drive", "GATE", "GND"),
		voltageSourceEvidence("supply", "VCC", "GND"),
		resistorEvidence("load", 100, "VCC", "DRAIN"),
		{
			InstanceID: "switch", CatalogID: "mosfet.reviewed", Family: "mosfet",
			ModelClaims: []CatalogEvidence{claim},
			Connections: []ConnectionEvidence{
				{Function: "GATE", Net: "GATE"},
				{Function: "DRAIN", Net: "DRAIN"},
				{Function: "SOURCE", Net: "GND"},
			},
		},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "DRAIN"}, {Name: "GATE"}, {Name: "VCC"}}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve electrothermal plan: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("electrothermal report=%#v diagnostics=%#v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || report.Analyses[0].Kind != AnalysisElectrothermal || len(report.Analyses[0].Points) != 21 {
		t.Fatalf("electrothermal trajectory = %#v", report.Analyses)
	}
	last := report.Analyses[0].Points[len(report.Analyses[0].Points)-1]
	var switchResult DeviceResult
	for _, device := range last.Devices {
		if device.Component == "switch" {
			switchResult = device
			break
		}
	}
	if switchResult.JunctionTemperatureC == nil || *switchResult.JunctionTemperatureC <= 25 || switchResult.TransientSOAMargin < 5 {
		t.Fatalf("electrothermal switch result = %#v", switchResult)
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 || !reflect.DeepEqual(report, replay) {
		t.Fatalf("electrothermal replay differs: diagnostics=%#v", replayDiagnostics)
	}
}

func TestElectrothermalTransientFailsClosedOnSOAViolation(t *testing.T) {
	pulse := 10e-3
	claim := CatalogEvidence{
		ModelID: PrimitiveNMOSSwitchV1,
		Parameters: append(nmosSwitchParameters(),
			NamedValue{Name: "max_temperature_c", Value: 150},
			NamedValue{Name: "junction_to_ambient_c_per_w", Value: 10},
		),
		ThermalModel: &ThermalRCNetwork{
			Reference:          "junction_to_ambient",
			Stages:             []ThermalRCStage{{ThermalResistanceCPerW: 10, ThermalCapacitanceJPerC: .001}},
			BoundaryAssumption: "Reviewed board boundary held at the declared ambient temperature.",
		},
		TransientSOA: []TransientSOAEnvelope{
			{PulseDurationS: &pulse, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: .001}, {VoltageV: 30, CurrentA: .0001}}},
			{DC: true, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: .001}, {VoltageV: 30, CurrentA: .0001}}},
		},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "unsafe", Kind: AnalysisElectrothermal, DurationS: 1e-3, TimeStepS: 100e-6,
			Conditions:  []NamedValue{{Name: "ambient_temperature_c", Value: 25}},
			Excitations: []SourceExcitation{{Component: "gate_drive", DCValue: 5}, {Component: "supply", DCValue: 10}},
		}},
		Assertions: []Assertion{{AnalysisID: "unsafe", Component: "switch", Quantity: QuantityTransientSOAMargin, Min: 1, Max: maxMNASolutionValue}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("gate_drive", "GATE", "GND"),
		voltageSourceEvidence("supply", "VCC", "GND"),
		resistorEvidence("load", 100, "VCC", "DRAIN"),
		{
			InstanceID: "switch", CatalogID: "mosfet.reviewed", Family: "mosfet",
			ModelClaims: []CatalogEvidence{claim},
			Connections: []ConnectionEvidence{{Function: "GATE", Net: "GATE"}, {Function: "DRAIN", Net: "DRAIN"}, {Function: "SOURCE", Net: "GND"}},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "DRAIN"}, {Name: "GATE"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve unsafe electrothermal plan: %#v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "SOA margin") {
		t.Fatalf("SOA violation diagnostics = %#v", diagnostics)
	}
}

func TestTransientSOAPulseClockTracksOnlyDCUnsafeExcursion(t *testing.T) {
	pulse := 1e-3
	device := ResolvedDevice{
		Component: "switch",
		ModelParameters: []NamedValue{
			{Name: "max_temperature_c", Value: 150},
		},
		TransientSOA: []TransientSOAEnvelope{
			{PulseDurationS: &pulse, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: 2}, {VoltageV: 30, CurrentA: 2}}},
			{DC: true, CaseTemperatureC: 25, Points: []TransientSOAPoint{{VoltageV: 1, CurrentA: .5}, {VoltageV: 30, CurrentA: .5}}},
		},
	}

	margin, excursion, diagnostic := transientSOAObservationMargin(device, transientSOAExcursion{}, 0, 25, 10, 1)
	if diagnostic != nil || margin != .5 || excursion.durationS != 0 || !excursion.active {
		t.Fatalf("pre-existing stress = margin %.12g excursion %#v diagnostic %#v", margin, excursion, diagnostic)
	}

	excursion = transientSOAExcursion{}
	for step := 1; step <= 11; step++ {
		margin, excursion, diagnostic = transientSOAObservationMargin(device, excursion, 100e-6, 25, 10, 1)
		wantDuration := float64(step-1) * 100e-6
		if diagnostic != nil || math.Abs(margin-2) > 1e-12 || math.Abs(excursion.durationS-wantDuration) > 1e-15 || !excursion.active {
			t.Fatalf("pulse step %d = margin %.12g excursion %#v diagnostic %#v", step, margin, excursion, diagnostic)
		}
	}
	margin, excursion, diagnostic = transientSOAObservationMargin(device, excursion, 100e-6, 25, 10, 1)
	if diagnostic != nil || margin != .5 || math.Abs(excursion.durationS-1.1e-3) > 1e-15 || !excursion.active {
		t.Fatalf("expired pulse = margin %.12g excursion %#v diagnostic %#v", margin, excursion, diagnostic)
	}

	margin, excursion, diagnostic = transientSOAObservationMargin(device, excursion, 100e-6, 25, 10, .25)
	if diagnostic != nil || margin != 2 || excursion != (transientSOAExcursion{}) {
		t.Fatalf("DC-safe recovery = margin %.12g excursion %#v diagnostic %#v", margin, excursion, diagnostic)
	}
}

func TestSteadyStateThermalPathUsesConfiguredRCNetwork(t *testing.T) {
	thermal := &ThermalRCNetwork{
		Reference:          "junction_to_ambient",
		BoundaryAssumption: "reviewed package and applied heatsink path",
		Stages: []ThermalRCStage{
			{ThermalResistanceCPerW: 1.8, ThermalCapacitanceJPerC: .01},
			{ThermalResistanceCPerW: 4.2, ThermalCapacitanceJPerC: 10},
		},
	}
	parameters := map[string]float64{"junction_to_ambient_c_per_w": 62.5}
	theta, reference, ok := resolvedThermalPath(thermal, parameters, nil, 50)
	if !ok || math.Abs(theta-6) > 1e-12 || reference != 50 {
		t.Fatalf("configured steady-state thermal path = %.12g C/W at %.12g C ok=%t", theta, reference, ok)
	}
}
