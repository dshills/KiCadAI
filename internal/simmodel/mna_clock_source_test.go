package simmodel

import (
	"math"
	"reflect"
	"testing"
)

func TestClockSourcePrimitivesDriveDeterministicBufferedWaveforms(t *testing.T) {
	tests := []struct {
		name              string
		sourceModel       string
		sourceParameters  []NamedValue
		frequencyHz       float64
		loadF             float64
		transientStepS    float64
		transientDuration float64
		startupStepS      float64
		startupDurationS  float64
		timingResistance  float64
	}{
		{
			name: "fixed", sourceModel: PrimitiveFixedClockSourceV1, frequencyHz: 8e6, loadF: 20e-12,
			transientStepS: 1e-9, transientDuration: 500e-9, startupStepS: 5e-6, startupDurationS: 5.2e-3,
			sourceParameters: clockTestSourceParameters(8e6, 5e-3),
		},
		{
			name: "resistor_programmed", sourceModel: PrimitiveResistorProgrammedClockSourceV1, frequencyHz: 100e3, loadF: 50e-12,
			transientStepS: 20e-9, transientDuration: 25e-6, startupStepS: 5e-6, startupDurationS: 1e-3,
			timingResistance: 1e6,
			sourceParameters: append(clockTestSourceParameters(0, 0),
				NamedValue{Name: "frequency_scale_hz_ohm", Value: 1e11},
				NamedValue{Name: "divider_ratio", Value: 1},
				NamedValue{Name: "timing_resistance_min_ohm", Value: 100e3},
				NamedValue{Name: "timing_resistance_max_ohm", Value: 1e6},
				NamedValue{Name: "startup_cycles", Value: 64},
				NamedValue{Name: "startup_fixed_s", Value: 100e-6},
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			intent := Intent{
				ModelID: ModelTransientCircuitV1,
				Analyses: []Analysis{
					{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}}},
					{ID: "waveform", Kind: AnalysisTransient, DurationS: tc.transientDuration, TimeStepS: tc.transientStepS, Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}}},
					{ID: "startup", Kind: AnalysisStartup, DurationS: tc.startupDurationS, TimeStepS: tc.startupStepS, Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}}},
				},
				Assertions: []Assertion{
					{AnalysisID: "dc", Node: "CLOCK", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31},
					{AnalysisID: "waveform", Node: "CLOCK", Quantity: QuantityVoltageV, TimeS: clockTestGridTime(.25/tc.frequencyHz, tc.transientStepS), Min: 2.69, Max: 3.31},
					{AnalysisID: "waveform", Node: "CLOCK", Quantity: QuantityVoltageV, TimeS: clockTestGridTime(.75/tc.frequencyHz, tc.transientStepS), Min: 0, Max: .61},
					{AnalysisID: "waveform", Node: "CLOCK", Quantity: QuantityRiseTimeS, Min: 0, Max: 200e-9},
					{AnalysisID: "waveform", Node: "CLOCK", Quantity: QuantityFallTimeS, Min: 0, Max: 200e-9},
					{AnalysisID: "startup", Node: "CLOCK", Quantity: QuantityPeakAbsVoltageV, Min: 0, Max: 3.31},
				},
			}
			components := clockTestComponents(tc.sourceModel, tc.sourceParameters, tc.loadF, tc.timingResistance)
			nodes := []NodeEvidence{
				{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"},
				{Name: "RAW_CLOCK", Role: "clock"}, {Name: "CLOCK", Role: "clock"},
			}
			if tc.timingResistance > 0 {
				nodes = append(nodes, NodeEvidence{Name: "SET", Role: "timing"})
			}
			plan, diagnostics := ResolveWithTopology(intent, "clock-test", "catalog-hash", components, nodes)
			if len(diagnostics) != 0 {
				t.Fatalf("resolve diagnostics = %#v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report = %#v, diagnostics = %#v", report, diagnostics)
			}
			var waveform, startup AnalysisResult
			for _, analysis := range report.Analyses {
				if analysis.ID == "waveform" {
					waveform = analysis
				} else if analysis.ID == "startup" {
					startup = analysis
				}
			}
			if got := measuredClockFrequency(waveform, "CLOCK"); math.Abs(got-tc.frequencyHz) > tc.frequencyHz*.02 {
				t.Fatalf("measured clock frequency = %.12g Hz, want %.12g Hz; CLOCK crossings=%v RAW_CLOCK crossings=%v",
					got, tc.frequencyHz, clockRisingEdges(waveform, "CLOCK"), clockRisingEdges(waveform, "RAW_CLOCK"))
			}
			if got := firstClockRisingEdge(startup, "CLOCK"); got <= 0 || got > tc.startupDurationS {
				t.Fatalf("first startup edge = %.12g s, want within (0, %.12g]", got, tc.startupDurationS)
			}
			replay, replayDiagnostics := Evaluate(ClonePlan(plan))
			if len(replayDiagnostics) != 0 || !reflect.DeepEqual(replay, report) {
				t.Fatalf("clock replay differs: diagnostics=%#v", replayDiagnostics)
			}
		})
	}
}

func TestClockSourceDCStateIsExplicit(t *testing.T) {
	parameters := clockTestSourceParameters(1e6, 1e-3)
	for index := range parameters {
		if parameters[index].Name == "dc_output_high" {
			parameters[index].Value = 0
		}
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{
			{
				ID: "dc_low", Kind: AnalysisDCOperatingPoint,
				Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}},
			},
			{
				ID: "waveform", Kind: AnalysisTransient, DurationS: 2e-6, TimeStepS: 10e-9,
				Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}},
			},
		},
		Assertions: []Assertion{
			{AnalysisID: "dc_low", Node: "RAW_CLOCK", Quantity: QuantityVoltageV, Min: 0, Max: 1e-9},
			{AnalysisID: "waveform", Node: "RAW_CLOCK", Quantity: QuantityVoltageV, TimeS: 250e-9, Min: 3.29, Max: 3.31},
			{AnalysisID: "waveform", Node: "RAW_CLOCK", Quantity: QuantityVoltageV, TimeS: 750e-9, Min: 0, Max: 1e-9},
		},
	}
	plan, diagnostics := ResolveWithTopology(
		intent, "clock-dc-state", "catalog-hash",
		clockTestComponents(PrimitiveFixedClockSourceV1, parameters, 20e-12, 0),
		[]NodeEvidence{
			{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"},
			{Name: "RAW_CLOCK", Role: "clock"}, {Name: "CLOCK", Role: "clock"},
		},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("explicit low-state report = %#v diagnostics=%#v", report, diagnostics)
	}
	for _, analysis := range report.Analyses {
		if analysis.ID != "dc_low" || len(analysis.Points) != 1 {
			continue
		}
		for _, device := range analysis.Points[0].Devices {
			if device.Component == "clock_source" && math.Abs(device.CurrentA) > 1e-12 {
				t.Fatalf("low-state clock current = %.12g A, want zero", device.CurrentA)
			}
		}
	}
	for _, analysis := range report.Analyses {
		if analysis.ID != "waveform" {
			continue
		}
		for _, targetTime := range []float64{250e-9, 750e-9} {
			found := false
			for _, point := range analysis.Points {
				if math.Abs(point.TimeS-targetTime) > 1e-15 {
					continue
				}
				found = true
				for _, device := range point.Devices {
					if (device.Component == "clock_source" || device.Component == "buffer") &&
						math.Abs(device.CurrentA) > 1e-6 {
						t.Fatalf("%s current at %.12g s = %.12g A, want settled state near zero",
							device.Component, targetTime, device.CurrentA)
					}
				}
			}
			if !found {
				t.Fatalf("missing waveform observation at %.12g s", targetTime)
			}
		}
	}
}

func clockTestGridTime(timeS, stepS float64) float64 {
	return math.Round(timeS/stepS) * stepS
}

func TestProgrammedClockFrequencyRejectsAmbiguousOrInvalidEvidence(t *testing.T) {
	timingResistance := 1e6
	decoyResistance := 10e3
	source := ResolvedDevice{
		Component: "clock", PrimitiveModel: PrimitiveResistorProgrammedClockSourceV1,
		ModelParameters: []NamedValue{
			{Name: "frequency_scale_hz_ohm", Value: 1e11},
			{Name: "divider_ratio", Value: 1},
		},
		Terminals: []TerminalBinding{{Terminal: "SET", Net: "SET"}, {Terminal: "GND", Net: "GND"}},
	}
	timing := ResolvedDevice{
		Component: "timing", Usage: "timing_resistor", PrimitiveModel: PrimitiveResistorV1, ValueSI: &timingResistance,
		Terminals: []TerminalBinding{{Terminal: "A", Net: "SET"}, {Terminal: "B", Net: "GND"}},
	}
	decoy := ResolvedDevice{
		Component: "decoy", PrimitiveModel: PrimitiveResistorV1, ValueSI: &decoyResistance,
		Terminals: []TerminalBinding{{Terminal: "A", Net: "SET"}, {Terminal: "B", Net: "AUX"}},
	}
	plan := Plan{Devices: []ResolvedDevice{source, decoy, timing}}
	if got := programmedClockFrequency(plan, source); got != 100e3 {
		t.Fatalf("programmed frequency = %g, want 100 kHz", got)
	}
	duplicate := timing
	duplicate.Component = "duplicate"
	plan.Devices = append(plan.Devices, duplicate)
	if got := programmedClockFrequency(plan, source); got != 0 {
		t.Fatalf("ambiguous programmed frequency = %g, want fail-closed zero", got)
	}
	source.ModelParameters[1].Value = 0
	if got := programmedClockFrequency(Plan{Devices: []ResolvedDevice{source, timing}}, source); got != 0 {
		t.Fatalf("zero-divider programmed frequency = %g, want fail-closed zero", got)
	}
}

func TestClockSourceHighRejectsNonFinitePeriod(t *testing.T) {
	device := ResolvedDevice{ModelParameters: []NamedValue{
		{Name: "frequency_hz", Value: math.SmallestNonzeroFloat64},
		{Name: "duty_cycle_fraction", Value: .5},
	}}
	if clockSourceHigh(device, nil, Analysis{Kind: AnalysisTransient}, 1, nil) {
		t.Fatal("clock source accepted a frequency whose period is non-finite")
	}
}

func TestFixedClockEnableLowSuppressesTransientOutput(t *testing.T) {
	parameters := clockTestSourceParameters(1e6, 1e-9)
	components := clockTestComponents(PrimitiveFixedClockSourceV1, parameters, 20e-12, 0)
	for componentIndex := range components {
		if components[componentIndex].InstanceID != "clock_source" {
			continue
		}
		for connectionIndex := range components[componentIndex].Connections {
			if components[componentIndex].Connections[connectionIndex].Function == "ENABLE" {
				components[componentIndex].Connections[connectionIndex].Net = "GND"
			}
		}
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{
			{
				ID: "disabled_dc", Kind: AnalysisDCOperatingPoint,
				Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}},
			},
			{
				ID: "disabled", Kind: AnalysisTransient, DurationS: 1e-6, TimeStepS: 10e-9,
				Excitations: []SourceExcitation{{Component: "supply", DCValue: 3.3}},
			},
		},
		Assertions: []Assertion{
			{
				AnalysisID: "disabled_dc", Node: "RAW_CLOCK", Quantity: QuantityVoltageV,
				Min: 0, Max: 1e-9,
			},
			{
				AnalysisID: "disabled", Node: "RAW_CLOCK", Quantity: QuantityVoltageV,
				TimeS: 250e-9, Min: 0, Max: 1e-9,
			},
		},
	}
	plan, diagnostics := ResolveWithTopology(
		intent, "clock-enable-low", "catalog-hash", components,
		[]NodeEvidence{
			{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"},
			{Name: "RAW_CLOCK", Role: "clock"}, {Name: "CLOCK", Role: "clock"},
		},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("disabled clock report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestClockPhaseResolutionRejectsUnboundedElapsedCycles(t *testing.T) {
	device := ResolvedDevice{
		Component: "clock", PrimitiveModel: PrimitiveFixedClockSourceV1,
		ModelParameters: []NamedValue{{Name: "frequency_hz", Value: 1e9}},
	}
	analysis := Analysis{ID: "long_clock", Kind: AnalysisTransient, DurationS: 2 * maxClockPhaseCycles / 1e9}
	diagnostics := validateClockPhaseResolution(Plan{Devices: []ResolvedDevice{device}}, analysis)
	if len(diagnostics) != 1 || diagnostics[0].Path != "analyses.long_clock.duration_s" {
		t.Fatalf("clock phase diagnostics = %#v, want one bounded-work finding", diagnostics)
	}
}

func clockTestSourceParameters(frequency, startup float64) []NamedValue {
	parameters := []NamedValue{
		{Name: "frequency_accuracy_fraction", Value: .00002},
		{Name: "duty_cycle_fraction", Value: .5},
		{Name: "rms_jitter_s", Value: 3e-12},
		{Name: "dc_output_high", Value: 1},
		{Name: "output_high_ratio", Value: .9},
		{Name: "output_low_ratio", Value: .1},
		{Name: "output_resistance_ohm", Value: 75},
		{Name: "max_output_current_a", Value: .004},
		{Name: "max_load_capacitance_f", Value: 60e-12},
		{Name: "supply_current_a", Value: .0045},
	}
	if frequency > 0 {
		parameters = append(parameters, NamedValue{Name: "frequency_hz", Value: frequency})
	}
	if startup > 0 {
		parameters = append(parameters, NamedValue{Name: "startup_time_s", Value: startup})
	}
	return parameters
}

func clockTestComponents(sourceModel string, sourceParameters []NamedValue, loadF, timingResistance float64) []ComponentEvidence {
	sourceParameters = append([]NamedValue(nil), sourceParameters...)
	sourceConnections := []ConnectionEvidence{
		{Function: "VDD", Net: "VCC"},
		{Function: "GND", Net: "GND"},
		{Function: "OUT", Net: "RAW_CLOCK"},
	}
	if sourceModel == PrimitiveFixedClockSourceV1 {
		sourceParameters = append(sourceParameters, NamedValue{Name: "enable_high_ratio", Value: .7})
		sourceConnections = append(sourceConnections, ConnectionEvidence{Function: "ENABLE", Net: "VCC"})
	} else {
		sourceConnections = append(sourceConnections,
			ConnectionEvidence{Function: "SET", Net: "SET"},
			ConnectionEvidence{Function: "DIV", Net: "GND"},
		)
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "supply", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VCC"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "clock_source", CatalogID: "clock", Family: "clock_source", ModelClaims: []CatalogEvidence{{ModelID: sourceModel, Parameters: sourceParameters}}, Connections: sourceConnections},
		{InstanceID: "buffer", CatalogID: "buffer", Family: "logic_buffer", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCMOSBufferV1, Parameters: []NamedValue{
			{Name: "input_low_max_v", Value: .89}, {Name: "input_high_min_v", Value: 1.92},
			{Name: "output_high_drop_v_at_rated_current", Value: .6}, {Name: "output_low_rise_v_at_rated_current", Value: .4},
			{Name: "rated_output_current_a", Value: .016}, {Name: "output_resistance_ohm", Value: 37.5},
			{Name: "propagation_delay_s", Value: 6.5e-9}, {Name: "max_load_capacitance_f", Value: 50e-12},
			{Name: "supply_current_a", Value: 10e-6},
		}}}, Connections: []ConnectionEvidence{{Function: "IN", Net: "RAW_CLOCK"}, {Function: "OUT", Net: "CLOCK"}, {Function: "VCC", Net: "VCC"}, {Function: "GND", Net: "GND"}}},
		{InstanceID: "load", CatalogID: "load", Family: "capacitor", HasValueSI: true, ValueSI: loadF, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 6.3}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "CLOCK"}, {Function: "B", Net: "GND"}}},
	}
	if timingResistance > 0 {
		components = append(components, ComponentEvidence{InstanceID: "timing", CatalogID: "timing", Family: "resistor", Usage: "timing_resistor", HasValueSI: true, ValueSI: timingResistance, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "SET"}, {Function: "B", Net: "GND"}}})
	}
	return components
}

func measuredClockFrequency(result AnalysisResult, node string) float64 {
	crossings := clockRisingEdges(result, node)
	if len(crossings) < 2 {
		return 0
	}
	return float64(len(crossings)-1) / (crossings[len(crossings)-1] - crossings[0])
}

func clockRisingEdges(result AnalysisResult, node string) []float64 {
	var crossings []float64
	for index := 1; index < len(result.Points); index++ {
		before, after := result.Points[index-1], result.Points[index]
		beforeV, afterV := testNodeVoltage(before, node), testNodeVoltage(after, node)
		if beforeV < 1.65 && afterV >= 1.65 {
			crossings = append(crossings, interpolateCrossing(before.TimeS, after.TimeS, beforeV, afterV, 1.65))
		}
	}
	return crossings
}

func firstClockRisingEdge(result AnalysisResult, node string) float64 {
	for index := 1; index < len(result.Points); index++ {
		before, after := result.Points[index-1], result.Points[index]
		beforeV, afterV := testNodeVoltage(before, node), testNodeVoltage(after, node)
		if beforeV < 1.65 && afterV >= 1.65 {
			return interpolateCrossing(before.TimeS, after.TimeS, beforeV, afterV, 1.65)
		}
	}
	return 0
}

func testNodeVoltage(point AnalysisPoint, node string) float64 {
	for _, value := range point.Nodes {
		if value.Node == node {
			return value.Real
		}
	}
	return 0
}
