package simmodel

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestMNASynchronousBuckExecutesDCStabilityAndStartupDeterministically(t *testing.T) {
	components, nodes := synchronousBuckEvidence()

	dcIntent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "operating_point", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.97, Max: 4.98}},
	}
	dcPlan, diagnostics := ResolveWithTopology(dcIntent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck DC: %#v", diagnostics)
	}
	dcReport, diagnostics := Evaluate(dcPlan)
	if len(diagnostics) != 0 || dcReport.Status != "pass" {
		t.Fatalf("synchronous buck DC report=%#v diagnostics=%#v", dcReport, diagnostics)
	}

	stabilityIntent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "loop", Kind: AnalysisStability, StartFrequencyHz: 10, StopFrequencyHz: 10e6, Points: 64,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "loop", Node: "OUT", Quantity: QuantityPhaseMarginDeg, Min: 0, Max: 180}},
	}
	stabilityPlan, diagnostics := ResolveWithTopology(stabilityIntent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck stability: %#v", diagnostics)
	}
	stabilityReport, diagnostics := Evaluate(stabilityPlan)
	if len(diagnostics) != 0 || stabilityReport.Status != "pass" || len(stabilityReport.Analyses) != 1 || len(stabilityReport.Analyses[0].ControlLoops) != 1 {
		t.Fatalf("synchronous buck stability report=%#v diagnostics=%#v", stabilityReport, diagnostics)
	}
	loop := stabilityReport.Analyses[0].ControlLoops[0]
	if loop.PrimitiveModel != PrimitiveSynchronousBuckRegulatorV1 ||
		loop.InjectionNet != "SW" || loop.ObservationNet != "OUT" || loop.FeedbackNet != "FB" ||
		!finite(loop.CrossoverFrequencyHz) || loop.CrossoverFrequencyHz <= 0 || loop.PhaseMarginDeg <= 0 ||
		loop.ReturnRatioSamplesSHA256 == "" {
		t.Fatalf("synchronous buck loop evidence = %#v", loop)
	}

	startupIntent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "startup", Kind: AnalysisStartup, DurationS: .012, TimeStepS: .0001,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "startup", Node: "OUT", Quantity: QuantityPeakAbsVoltageV, Min: 4.8, Max: 5.1}},
	}
	startupPlan, diagnostics := ResolveWithTopology(startupIntent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck startup: %#v", diagnostics)
	}
	startupReport, diagnostics := Evaluate(startupPlan)
	if len(diagnostics) != 0 || startupReport.Status != "pass" {
		t.Fatalf("synchronous buck startup report=%#v diagnostics=%#v", startupReport, diagnostics)
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(startupPlan))
	if len(replayDiagnostics) != 0 || !reflect.DeepEqual(startupReport, replay) {
		t.Fatalf("synchronous buck startup replay differs: diagnostics=%#v", replayDiagnostics)
	}
}

func TestMNASynchronousBuckFailsClosedForInvalidLimitsAndAmbiguousOutputInductor(t *testing.T) {
	components, nodes := synchronousBuckEvidence()
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "loop", Kind: AnalysisStability, StartFrequencyHz: 10, StopFrequencyHz: 10e6, Points: 32,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "loop", Node: "OUT", Quantity: QuantityPhaseMarginDeg, Min: 0, Max: 180}},
	}

	invalid := append([]ComponentEvidence(nil), components...)
	invalid[2] = cloneComponentEvidence(components[2])
	for index := range invalid[2].ModelClaims[0].Parameters {
		if invalid[2].ModelClaims[0].Parameters[index].Name == "peak_current_limit_a" {
			invalid[2].ModelClaims[0].Parameters[index].Value = 1
		}
	}
	if _, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", invalid, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "peak_current_limit") {
		t.Fatalf("invalid synchronous-buck current limit diagnostics = %#v", diagnostics)
	}

	ambiguous := append([]ComponentEvidence(nil), components...)
	ambiguous = append(ambiguous, ComponentEvidence{
		InstanceID: "second_inductor", CatalogID: "inductor.second", Family: "inductor",
		ValueSI: 22e-6, HasValueSI: true,
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveInductorTransientV1, Parameters: []NamedValue{
			{Name: "rated_current_a", Value: 3}, {Name: "saturation_current_a", Value: 4}, {Name: "series_resistance_ohm", Value: .04},
		}}},
		Connections: []ConnectionEvidence{{Function: "A", Net: "SW"}, {Function: "B", Net: "OTHER_OUT"}},
	})
	ambiguousNodes := append(append([]NodeEvidence(nil), nodes...), NodeEvidence{Name: "OTHER_OUT"})
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", ambiguous, ambiguousNodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve ambiguous synchronous buck: %#v", diagnostics)
	}
	if _, diagnostics = Evaluate(plan); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "unique catalog-modeled output inductor") {
		t.Fatalf("ambiguous synchronous-buck output diagnostics = %#v", diagnostics)
	}
}

func TestMNASynchronousBuckAppliesSoftStartAfterExplicitInputEvent(t *testing.T) {
	components, nodes := synchronousBuckEvidence()
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "input_event", Kind: AnalysisTransient, DurationS: 100e-6, TimeStepS: 10e-6,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
			SourceValueEvents: []SourceValueEvent{{
				ID: "power_on", Component: "input_supply", TriggerTimeS: 0, DurationS: 100e-6,
				Initial: 0, Applied: 24,
			}},
		}},
		Assertions: []Assertion{{AnalysisID: "input_event", Node: "OUT", Quantity: QuantityPeakAbsVoltageV, Min: 0, Max: 5.1}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck input event: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("synchronous buck input-event report=%#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMNASynchronousBuckTransientEfficiencyUsesResolvedOutputBoundary(t *testing.T) {
	components, nodes := synchronousBuckEvidence()
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "efficiency", Kind: AnalysisTransient, DurationS: .002, TimeStepS: .0001,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "efficiency", Component: "load", Components: []string{"input_supply"},
			Quantity: QuantityConversionEfficiencyPct, Min: 0, Max: 100,
		}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck efficiency: %#v", diagnostics)
	}
	result, diagnostics := solveTransientAnalysis(plan, plan.Analyses[0])
	if len(diagnostics) != 0 {
		t.Fatalf("evaluate synchronous buck efficiency: %#v", diagnostics)
	}
	actual, diagnostic := transientDerivedValue(result, Assertion{
		AnalysisID: "efficiency", Component: "load", Components: []string{"input_supply"},
		Quantity: QuantityConversionEfficiencyPct,
	})
	if diagnostic != nil || actual < 81.9 || actual > 82.5 || math.IsNaN(actual) {
		t.Fatalf("resolved-output conversion efficiency = %.12g, want the reviewed 82%% boundary after explicit feedback consumption and without duplicate quiescent loss; diagnostic=%#v", actual, diagnostic)
	}
	ripple, rippleDiagnostic := transientDerivedValue(result, Assertion{
		AnalysisID: "efficiency", Node: "OUT", Quantity: QuantityOutputRippleVPP,
	})
	if rippleDiagnostic != nil || ripple < .029 || ripple > .031 {
		t.Fatalf("model-derived LC-plus-ESR ripple = %.12g, want about .030 Vpp; diagnostic=%#v periodic=%#v", ripple, rippleDiagnostic, result.PeriodicNodes)
	}
	if len(result.PeriodicNodes) != 1 || result.PeriodicNodes[0].Method != "averaged_buck_lc_parallel_esr_ripple_bound_v3" {
		t.Fatalf("periodic node evidence = %#v", result.PeriodicNodes)
	}
	parallel := ClonePlan(plan)
	for _, device := range parallel.Devices {
		if device.PrimitiveModel != PrimitiveCapacitorTransientV1 {
			continue
		}
		duplicate := device
		duplicate.Component += "_parallel"
		parallel.Devices = append(parallel.Devices, duplicate)
		break
	}
	parallel = indexMNAPlanDevices(parallel)
	parallelPeriodic := synchronousBuckPeriodicNodeResults(parallel, result)
	if len(parallelPeriodic) != 1 || math.Abs(parallelPeriodic[0].VoltageRippleVPP-ripple/2) > 1e-12 {
		t.Fatalf("two identical parallel output capacitors produced periodic evidence %#v, want half the single-capacitor ripple %.12g", parallelPeriodic, ripple/2)
	}
	withUnprovenLoad := ClonePlan(plan)
	for _, device := range withUnprovenLoad.Devices {
		if device.PrimitiveModel != PrimitiveCapacitorTransientV1 {
			continue
		}
		load := device
		load.Component += "_unproven_load"
		parameters := []NamedValue{}
		for _, parameter := range load.ModelParameters {
			if parameter.Name != "series_resistance_ohm" {
				parameters = append(parameters, parameter)
			}
		}
		load.ModelParameters = parameters
		load.parameterIndex = nil
		withUnprovenLoad.Devices = append(withUnprovenLoad.Devices, load)
		break
	}
	withUnprovenLoad = indexMNAPlanDevices(withUnprovenLoad)
	unprovenLoadPeriodic := synchronousBuckPeriodicNodeResults(withUnprovenLoad, result)
	if len(unprovenLoadPeriodic) != 1 || math.Abs(unprovenLoadPeriodic[0].VoltageRippleVPP-ripple) > 1e-12 {
		t.Fatalf("unproven parallel load capacitance changed ESR-proven output ripple evidence: %#v, want %.12g", unprovenLoadPeriodic, ripple)
	}
	withoutESR := ClonePlan(plan)
	for deviceIndex := range withoutESR.Devices {
		device := &withoutESR.Devices[deviceIndex]
		if device.PrimitiveModel != PrimitiveCapacitorTransientV1 {
			continue
		}
		parameters := device.ModelParameters[:0]
		for _, parameter := range device.ModelParameters {
			if parameter.Name != "series_resistance_ohm" {
				parameters = append(parameters, parameter)
			}
		}
		device.ModelParameters = parameters
		device.parameterIndex = nil
	}
	withoutESR = indexMNAPlanDevices(withoutESR)
	if periodic := synchronousBuckPeriodicNodeResults(withoutESR, result); len(periodic) != 0 {
		t.Fatalf("unproven output-capacitor ESR produced trusted ripple evidence: %#v", periodic)
	}
}

func TestMNASynchronousBuckTransientEfficiencyWithCurrentLoad(t *testing.T) {
	components, nodes := synchronousBuckEvidence()
	components[len(components)-1] = ComponentEvidence{
		InstanceID: "load", CatalogID: "current_load", Family: "current_source",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}},
		Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "OUT"}, {Function: "NEGATIVE", Net: "GND"}},
	}
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "efficiency", Kind: AnalysisTransient, DurationS: .02, TimeStepS: .00002,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 18}, {Component: "enable", DCValue: 5}, {Component: "load", DCValue: .45}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "efficiency", Component: "load", Components: []string{"input_supply"},
			Quantity: QuantityConversionEfficiencyPct, Min: 0, Max: 100,
		}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck current-load efficiency: %#v", diagnostics)
	}
	result, diagnostics := solveTransientAnalysis(plan, plan.Analyses[0])
	if len(diagnostics) != 0 {
		t.Fatalf("evaluate synchronous buck current-load efficiency: %#v", diagnostics)
	}
	actual, diagnostic := transientDerivedValue(result, intent.Assertions[0])
	if diagnostic != nil || actual < 81.9 || actual > 82.5 || math.IsNaN(actual) {
		t.Fatalf("current-load conversion efficiency = %.12g, want the reviewed 82%% boundary; diagnostic=%#v", actual, diagnostic)
	}
}

func TestSettledPeriodicNodeAverageDoesNotUseFinalSampleAsSteadyVoltage(t *testing.T) {
	result := AnalysisResult{Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 4}}},
		{TimeS: 1e-6, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{TimeS: 2e-6, Nodes: []NodeResult{{Node: "OUT", Real: 6}}},
	}}
	average, found := settledPeriodicNodeAverage(result, "OUT", "", 500_000)
	if !found || average != 5 {
		t.Fatalf("settled-period average = %g found=%t, want 5 V across all samples rather than final-sample 6 V", average, found)
	}
}

func TestMNASynchronousBuckEfficiencyExcludesSettledStartupEnergy(t *testing.T) {
	components, nodes := synchronousBuckEvidence()
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "event_efficiency", Kind: AnalysisTransient, DurationS: .012, TimeStepS: .0001,
			Excitations: []SourceExcitation{{Component: "input_supply", DCValue: 24}, {Component: "enable", DCValue: 5}},
			SourceValueEvents: []SourceValueEvent{{
				ID: "enable_start", Component: "enable", TriggerTimeS: .001, DurationS: .011,
				Initial: 0, Applied: 5,
			}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "event_efficiency", Component: "load", Components: []string{"input_supply"},
			Quantity: QuantityConversionEfficiencyPct, Min: 0, Max: 100,
		}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "buck-catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve synchronous buck event efficiency: %#v", diagnostics)
	}
	result, diagnostics := solveTransientAnalysis(plan, plan.Analyses[0])
	if len(diagnostics) != 0 {
		t.Fatalf("evaluate synchronous buck event efficiency: %#v", diagnostics)
	}
	actual, diagnostic := transientDerivedValue(result, intent.Assertions[0])
	if diagnostic != nil || actual < 81.9 || actual > 82.5 || math.IsNaN(actual) {
		t.Fatalf("settled conversion efficiency = %.12g, want the reviewed 82%% boundary after startup; diagnostic=%#v", actual, diagnostic)
	}
}

func synchronousBuckEvidence() ([]ComponentEvidence, []NodeEvidence) {
	buckParameters := []NamedValue{
		{Name: "reference_voltage_v", Value: 1},
		{Name: "control_transconductance_s", Value: 200},
		{Name: "control_pole_hz", Value: 150_000},
		{Name: "nominal_input_voltage_v", Value: 24},
		{Name: "nominal_output_voltage_v", Value: 5},
		{Name: "min_input_voltage_v", Value: 3.5},
		{Name: "max_input_voltage_v", Value: 60},
		{Name: "max_output_current_a", Value: 2.5},
		{Name: "peak_current_limit_a", Value: 3.4},
		{Name: "conversion_efficiency_fraction", Value: .82},
		{Name: "quiescent_current_a", Value: .0007},
		{Name: "soft_start_time_s", Value: .0063},
		{Name: "switching_frequency_hz", Value: 500_000},
		{Name: "high_side_on_resistance_ohm", Value: .17},
		{Name: "low_side_on_resistance_ohm", Value: .08},
		{Name: "switch_transition_time_s", Value: 20e-9},
		{Name: "enable_threshold_v", Value: 1.2},
		{Name: "max_temperature_c", Value: 150},
		{Name: "junction_to_ambient_c_per_w", Value: 31.7},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("input_supply", "PVIN", "GND"),
		voltageSourceEvidence("enable", "EN", "GND"),
		{
			InstanceID: "buck", CatalogID: "regulator.ti.lm76002rnp.wqfn30", Family: "regulator",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveSynchronousBuckRegulatorV1, Parameters: buckParameters}},
			Connections: []ConnectionEvidence{
				{Function: "PVIN", Net: "PVIN"}, {Function: "SW", Net: "SW"}, {Function: "FB", Net: "FB"},
				{Function: "AGND", Net: "GND"}, {Function: "PGND", Net: "GND"}, {Function: "EN", Net: "EN"},
			},
		},
		{
			InstanceID: "inductor", CatalogID: "inductor.sunlord.mwsa1206s_150mt", Family: "inductor",
			ValueSI: 15e-6, HasValueSI: true,
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveInductorTransientV1, Parameters: []NamedValue{
				{Name: "rated_current_a", Value: 7.5}, {Name: "saturation_current_a", Value: 7.2}, {Name: "series_resistance_ohm", Value: .026},
			}}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "SW"}, {Function: "B", Net: "OUT"}},
		},
		{
			InstanceID: "output_capacitor", CatalogID: "capacitor.panasonic.eeufr1v221.radial", Family: "capacitor",
			ValueSI: 220e-6, HasValueSI: true,
			ModelClaims: []CatalogEvidence{
				{ModelID: PrimitiveCapacitorV1},
				{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 35}, {Name: "series_resistance_ohm", Value: .056}}},
			},
			Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}},
		},
		resistorEvidence("feedback_top", 40_000, "OUT", "FB"),
		resistorEvidence("feedback_bottom", 10_000, "FB", "GND"),
		resistorEvidence("load", 5, "OUT", "GND"),
	}
	nodes := []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "PVIN", Role: "power_pos"}, {Name: "EN"},
		{Name: "SW", Role: "switching"}, {Name: "OUT", Role: "power_pos"}, {Name: "FB"},
	}
	return components, nodes
}

func cloneComponentEvidence(source ComponentEvidence) ComponentEvidence {
	clone := source
	clone.ModelClaims = make([]CatalogEvidence, len(source.ModelClaims))
	for index := range source.ModelClaims {
		clone.ModelClaims[index] = CloneCatalogEvidence(source.ModelClaims[index])
	}
	clone.Connections = append([]ConnectionEvidence(nil), source.Connections...)
	return clone
}
