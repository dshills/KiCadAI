package closedloopsynthesis

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

func TestLoadCurrentCornerFollowsPulsedSupplyWindow(t *testing.T) {
	analysis := simmodel.Analysis{Kind: simmodel.AnalysisTransient, Excitations: []simmodel.SourceExcitation{
		{Component: "supply", PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 1e-6, PulseWidthS: 8e-6, PulsePeriodS: 10e-6},
		{Component: "load", DCValue: 0},
	}}
	value := 0.15
	diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{Axis: "load_current", Kind: OperatingSourceDCValue, Component: "load"}, CornerAssignment{Value: &value})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	load := analysis.Excitations[1]
	if load.DCValue != 0 || load.PulseInitialValue != 0 || load.PulseValue != value || load.PulseDelayS != 1e-6 || load.PulseWidthS != 8e-6 || load.PulsePeriodS != 10e-6 {
		t.Fatalf("dynamic load excitation = %#v", load)
	}
}

func TestLoadCurrentCornerUsesEquivalentStartupResistance(t *testing.T) {
	baseResistance := 4.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "load", Family: "resistor", ValueSI: &baseResistance}}}
	for current, expected := range map[float64]float64{3: 4, 1.5: 8, 0: maxCompiledAssertionBound} {
		analysis := simmodel.Analysis{Kind: simmodel.AnalysisStartup}
		value := current
		diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_current", Kind: OperatingLoadCurrent, Component: "load", Scale: 12}, CornerAssignment{Value: &value})
		if diagnostic != nil {
			t.Fatalf("current %.12g: %#v", current, diagnostic)
		}
		if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil || *analysis.DeviceOverrides[0].ValueSI != expected {
			t.Fatalf("current %.12g override = %#v, want %.12g ohm", current, analysis.DeviceOverrides, expected)
		}
	}
}

func TestLoadCurrentCornerAppliesCatalogBackedParallelSupportOffset(t *testing.T) {
	semanticMaximum, supportCurrent := 0.25, 60e-6
	physicalMaximum := semanticMaximum - supportCurrent
	binding := SimulationOperatingBinding{
		Axis: "load_current", Kind: OperatingLoadCurrent, Component: "load",
		Scale: 12, Offset: -supportCurrent,
	}
	t.Run("current_source", func(t *testing.T) {
		analysis := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "load"}}}
		if diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, binding, CornerAssignment{Value: &semanticMaximum}); diagnostic != nil {
			t.Fatal(diagnostic)
		}
		if math.Abs(analysis.Excitations[0].DCValue-physicalMaximum) > 1e-15 {
			t.Fatalf("physical source current = %.12g, want %.12g", analysis.Excitations[0].DCValue, physicalMaximum)
		}
	})
	t.Run("startup_resistance", func(t *testing.T) {
		baseResistance := 12 / physicalMaximum
		plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "load", Family: "resistor", ValueSI: &baseResistance}}}
		analysis := simmodel.Analysis{Kind: simmodel.AnalysisStartup}
		if diagnostic := applyOperatingAssignment(&analysis, &plan, binding, CornerAssignment{Value: &semanticMaximum}); diagnostic != nil {
			t.Fatal(diagnostic)
		}
		if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil ||
			math.Abs(*analysis.DeviceOverrides[0].ValueSI-baseResistance) > 1e-12 {
			t.Fatalf("startup load override = %#v, want %.12g ohm", analysis.DeviceOverrides, baseResistance)
		}
	})
}

func TestLoadCurrentCornerTracksResolvedSupplyCorner(t *testing.T) {
	baseResistance := 8.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "load", Family: "resistor", ValueSI: &baseResistance}}}
	supply, current := 30.0, 3.0
	analysis := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 24}}}
	if diagnostic := applyOperatingAssignment(
		&analysis, &plan,
		SimulationOperatingBinding{Axis: "supply_voltage", Kind: OperatingSourceDCValue, Component: "supply"},
		CornerAssignment{Value: &supply},
	); diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if diagnostic := applyOperatingAssignment(
		&analysis, &plan,
		SimulationOperatingBinding{
			Axis: "load_current", Kind: OperatingLoadCurrent, Component: "load",
			ReferenceComponent: "supply", Scale: 24,
		},
		CornerAssignment{Value: &current},
	); diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil || *analysis.DeviceOverrides[0].ValueSI != 10 {
		t.Fatalf("load override = %#v, want 10 ohm at the 30 V corner", analysis.DeviceOverrides)
	}
}

func TestAggregateCurrentEventDistributesWithinDeclaredLoadCapacity(t *testing.T) {
	initial, applied, recovered := 0.04, 1.6, 0.04
	max3V3, max5V := 0.6, 1.0
	resistance3V3, resistance5V := 3.3/max3V3, 5.0/max5V
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "load_3v3", Family: "resistor", ValueSI: &resistance3V3},
		{Component: "load_5v", Family: "resistor", ValueSI: &resistance5V},
	}}
	analysis := simmodel.Analysis{ID: "step", Kind: simmodel.AnalysisTransient, DurationS: 0.04, TimeStepS: 0.0001}
	event := PlannedEvent{
		ID: "dual_load", Kind: "load_step", OperatingCase: "rated", Target: "circuit",
		TriggerTimeS: 0.02, DurationS: 0.01, Initial: &initial, Applied: applied, Recovered: &recovered, Unit: "A",
	}
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{
		{ID: "rated_3v3", OperatingCase: "rated", Assignments: []CornerAssignment{{Axis: "load_current", Target: "out_3v3", Value: &max3V3}}},
		{ID: "rated_5v", OperatingCase: "rated", Assignments: []CornerAssignment{{Axis: "load_current", Target: "out_5v", Value: &max5V}}},
	}}
	bindings := []SimulationOperatingBinding{
		{Axis: "load_current", Target: "out_3v3", Kind: OperatingLoadCurrent, Component: "load_3v3", Scale: 3.3},
		{Axis: "load_current", Target: "out_5v", Kind: OperatingLoadCurrent, Component: "load_5v", Scale: 5},
	}
	ok, diagnostic := applyAggregateCurrentEvent(&analysis, plan, event, analysisPlan, bindings)
	if diagnostic != nil || !ok {
		t.Fatalf("aggregate event = %v, %#v", ok, diagnostic)
	}
	if len(analysis.DeviceValueEvents) != 2 {
		t.Fatalf("device events = %#v", analysis.DeviceValueEvents)
	}
	got := map[string]simmodel.DeviceValueEvent{}
	for _, member := range analysis.DeviceValueEvents {
		got[member.Component] = member
	}
	if math.Abs(got["load_3v3"].InitialSI-165) > 1e-9 || math.Abs(got["load_3v3"].AppliedSI-resistance3V3) > 1e-9 ||
		math.Abs(got["load_5v"].InitialSI-250) > 1e-9 || math.Abs(got["load_5v"].AppliedSI-resistance5V) > 1e-9 {
		t.Fatalf("distributed event values = %#v", got)
	}
}

func TestApplyPlannedEventsDeduplicatesRepeatedPlannedIdentity(t *testing.T) {
	initial, recovered := 8.0, 8.0
	value := ShortCircuitHarnessOpenResistanceOhm
	component := OperatingHarnessComponentID("short_circuit", "LOAD")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}}}
	analysis := simmodel.Analysis{ID: "fault", Kind: simmodel.AnalysisTransient, DurationS: .08, TimeStepS: 5e-8}
	planned := PlannedAnalysis{OperatingCase: "faulted", Events: []string{"short", "short"}}
	analysisPlan := AnalysisPlan{Events: []PlannedEvent{{
		ID: "short", Kind: "short_circuit", OperatingCase: "faulted", Target: "LOAD",
		TriggerTimeS: .07, DurationS: .01, Initial: &initial, Applied: .01, Recovered: &recovered, Unit: "Ohm",
	}}}
	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, nil)
	if len(diagnostics) != 0 || len(applied) != 1 || len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("applied=%#v events=%#v diagnostics=%#v", applied, analysis.DeviceValueEvents, diagnostics)
	}
}

func TestResistanceEventReusesEquivalentPhysicalCurrentLoad(t *testing.T) {
	initial, recovered := 24.0, 24.0
	value := 8.0
	cornerValue := 12.0
	component := OperatingHarnessComponentID("load_current", "LOAD")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}}}
	analysis := simmodel.Analysis{
		ID: "fault", Kind: simmodel.AnalysisTransient, DurationS: .08, TimeStepS: 50e-6,
		DeviceOverrides: []simmodel.DeviceOverride{{Component: component, ValueSI: &cornerValue}},
	}
	planned := PlannedAnalysis{OperatingCase: "faulted", Events: []string{"short"}}
	analysisPlan := AnalysisPlan{Events: []PlannedEvent{{
		ID: "short", Kind: "resistance_step", OperatingCase: "faulted", Target: "LOAD",
		TriggerTimeS: .07, DurationS: .01, Initial: &initial, Applied: .01, Recovered: &recovered, Unit: "Ohm",
	}}}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_current", Target: "LOAD", Kind: OperatingLoadCurrent, Component: component, Scale: 24,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"short"}) || len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("applied=%#v events=%#v diagnostics=%#v", applied, analysis.DeviceValueEvents, diagnostics)
	}
	if event := analysis.DeviceValueEvents[0]; event.Component != component || event.InitialSI != cornerValue || event.AppliedSI != .01 ||
		event.RecoveredSI == nil || *event.RecoveredSI != cornerValue {
		t.Fatalf("reused current-load event = %#v", event)
	}
}

func TestResistanceEventScalesNominalLoadToResolvedSupplyCorner(t *testing.T) {
	initial, recovered := 8.0, 8.0
	value := initial
	component := OperatingHarnessComponentID("load_current", "LOAD")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}}}
	analysis := simmodel.Analysis{
		ID: "fault", Kind: simmodel.AnalysisElectrothermal, DurationS: .08, TimeStepS: 100e-6,
		Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 30}},
	}
	planned := PlannedAnalysis{ID: "fault_analysis", Kind: simmodel.AnalysisElectrothermal, OperatingCase: "faulted", Events: []string{"short"}}
	analysisPlan := AnalysisPlan{Events: []PlannedEvent{{
		ID: "short", Kind: "resistance_step", OperatingCase: "faulted", Target: "LOAD",
		TriggerTimeS: .07, DurationS: .01, Initial: &initial, Applied: .01, Recovered: &recovered, Unit: "Ohm",
	}}}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_current", Target: "LOAD", Kind: OperatingLoadCurrent, Component: component,
		ReferenceComponent: "supply", Scale: 24,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"short"}) || len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("applied=%#v events=%#v diagnostics=%#v", applied, analysis.DeviceValueEvents, diagnostics)
	}
	event := analysis.DeviceValueEvents[0]
	if event.InitialSI != 10 || event.RecoveredSI == nil || *event.RecoveredSI != 10 {
		t.Fatalf("supply-scaled resistance event = %#v, want 10 ohm pre-event and recovery", event)
	}
}

func TestCurrentEventScalesEquivalentLoadToResolvedSupplyCorner(t *testing.T) {
	initialCurrent := 3.0
	value := 8.0
	component := OperatingHarnessComponentID("load_current", "LOAD")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}}}
	analysis := simmodel.Analysis{
		ID: "turn_off", Kind: simmodel.AnalysisTransient, DurationS: .011, TimeStepS: 50e-6,
		Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 30}},
	}
	planned := PlannedAnalysis{ID: "turn_off_analysis", Kind: simmodel.AnalysisTransient, OperatingCase: "rated", Events: []string{"turn_off"}}
	analysisPlan := AnalysisPlan{Events: []PlannedEvent{{
		ID: "turn_off", Kind: "inductive_turn_off", OperatingCase: "rated", Target: "LOAD",
		TriggerTimeS: .001, DurationS: .01, Initial: &initialCurrent, Applied: 0, Unit: "A",
	}}}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_current", Target: "LOAD", Kind: OperatingLoadCurrent, Component: component,
		ReferenceComponent: "supply", Scale: 24,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"turn_off"}) || len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("applied=%#v events=%#v diagnostics=%#v", applied, analysis.DeviceValueEvents, diagnostics)
	}
	event := analysis.DeviceValueEvents[0]
	if event.InitialSI != 10 || event.AppliedSI != maxCompiledAssertionBound || event.RecoveredSI != nil {
		t.Fatalf("supply-scaled current event = %#v, want 10 ohm pre-event and open-circuit event", event)
	}
}

func TestPhysicalLoadEventTruncatesOperatingPreludeWithoutOverlap(t *testing.T) {
	component := OperatingHarnessComponentID("load_current", "LOAD")
	value := 10.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}}}
	analysis := simmodel.Analysis{
		ID: "load_step", Kind: simmodel.AnalysisTransient, DurationS: .04, TimeStepS: .0001,
		DeviceOverrides: []simmodel.DeviceOverride{{Component: component, ValueSI: float64PointerForTest(maxCompiledAssertionBound)}},
		DeviceValueEvents: []simmodel.DeviceValueEvent{{
			ID: compiledEventID("operating_load_current", component), Component: component,
			TriggerTimeS: .0001, DurationS: .0399, InitialSI: maxCompiledAssertionBound, AppliedSI: value,
		}},
	}
	initial, recovered := .5, .5
	event := PlannedEvent{
		ID: "load_step", Kind: "load_step", Target: "LOAD", TriggerTimeS: .01, DurationS: .02,
		Initial: &initial, Applied: 1, Recovered: &recovered, Unit: "A",
	}
	binding := SimulationOperatingBinding{
		Axis: "load_current", Target: "LOAD", Kind: OperatingLoadCurrent, Component: component, Scale: 5,
	}

	applied, diagnostic := applyCurrentEventBinding(&analysis, plan, event, binding)
	if diagnostic != nil || !applied || len(analysis.DeviceValueEvents) != 2 {
		t.Fatalf("applied=%t diagnostic=%#v events=%#v", applied, diagnostic, analysis.DeviceValueEvents)
	}
	compactPlannedEventWindow(&analysis, []string{"load_step"})
	truncateOperatingLoadPreludes(&analysis)
	prelude, declared := analysis.DeviceValueEvents[0], analysis.DeviceValueEvents[1]
	if math.Abs(prelude.DurationS-.0019) > 1e-12 || prelude.TriggerTimeS+prelude.DurationS > declared.TriggerTimeS+1e-12 {
		t.Fatalf("prelude=%#v overlaps declared event=%#v", prelude, declared)
	}
	if declared.InitialSI != 10 || declared.AppliedSI != 5 || declared.RecoveredSI == nil || *declared.RecoveredSI != 10 {
		t.Fatalf("declared physical-load event = %#v", declared)
	}
}

func TestEventOnlyAnalysisStagesFallbackPulseFromStableBoundary(t *testing.T) {
	initial, recovered := 8.0, 8.0
	value := initial
	component := OperatingHarnessComponentID("load_resistance", "LOAD")
	analysis := simmodel.Analysis{
		ID: "fault", Kind: simmodel.AnalysisElectrothermal, DurationS: .02, TimeStepS: 50e-6,
		Excitations: []simmodel.SourceExcitation{{
			Component:         "control",
			DCValue:           -5,
			PulseInitialValue: -5, PulseValue: 0, PulseDelayS: .001, PulseWidthS: .019, PulsePeriodS: .02,
		}},
	}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component, Family: "resistor", ValueSI: &value,
	}, {
		Component: OperatingHarnessComponentID("short_circuit", "LOAD"),
		Family:    "resistor",
		ValueSI:   float64PointerForTest(ShortCircuitHarnessOpenResistanceOhm),
	}}}
	planned := PlannedAnalysis{ID: "fault_analysis", Kind: simmodel.AnalysisElectrothermal, OperatingCase: "faulted", Events: []string{"short"}}
	analysisPlan := AnalysisPlan{
		Events: []PlannedEvent{{
			ID: "short", Kind: "short_circuit", OperatingCase: "faulted", Target: "LOAD",
			TriggerTimeS: .07, DurationS: .01, Initial: &initial, Applied: .01, Recovered: &recovered, Unit: "Ohm",
		}},
		Assertions: []PlannedAssertion{{
			RequirementID: "soa", AnalysisID: planned.ID, OperatingCase: "faulted",
			Metric: "transient_soa_margin", Target: "event:short", Min: float64PointerForTest(1.25), Unit: "ratio",
		}},
	}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_resistance", Target: "LOAD", Kind: OperatingDeviceValueSI, Component: component,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"short"}) {
		t.Fatalf("applied=%#v diagnostics=%#v", applied, diagnostics)
	}
	excitation := analysis.Excitations[0]
	wantDelay := 2 * analysis.TimeStepS
	if excitation.DCValue != 0 || excitation.PulseInitialValue != 0 || excitation.PulseValue != -5 ||
		excitation.PulseDelayS != wantDelay || excitation.PulseWidthS != analysis.DurationS ||
		excitation.PulsePeriodS != excitation.PulseDelayS+analysis.DurationS+analysis.TimeStepS {
		t.Fatalf("staged fallback pulse = %#v analysis=%#v", excitation, analysis)
	}
}

func TestExcitationOverrideReplacesDynamicSourceWithCanonicalDC(t *testing.T) {
	analysis := simmodel.Analysis{
		Excitations: []simmodel.SourceExcitation{{
			Component: "control", DCValue: 0, ACMagnitude: 1, ACPhaseDeg: 90,
			PulseInitialValue: 0, PulseValue: 5, PulseDelayS: 1e-3, PulseWidthS: 2e-3, PulsePeriodS: 4e-3,
			SineAmplitude: 2, SineFrequencyHz: 1e3, SinePhaseDeg: 45,
		}},
	}
	if diagnostic := applyExcitationOverrides(&analysis, []SimulationExcitationOverride{{Component: "control", DCValue: 3.3}}); diagnostic != nil {
		t.Fatal(diagnostic)
	}
	excitation := analysis.Excitations[0]
	if excitation != (simmodel.SourceExcitation{Component: "control", DCValue: 3.3}) {
		t.Fatalf("DC override retained conflicting source fields: %#v", excitation)
	}
}

func TestEventResponsePreservesPeriodicDriveAndCompactsDeclaredPrehistory(t *testing.T) {
	loadValue := 4.0
	analysis := simmodel.Analysis{
		ID: "fault", Kind: simmodel.AnalysisTransient, DurationS: .02, TimeStepS: 50e-6,
		Excitations: []simmodel.SourceExcitation{
			{Component: "input", SineAmplitude: 1, SineFrequencyHz: 1000},
			{
				Component: "control", PulseInitialValue: 0, PulseValue: 5,
				PulseDelayS: .001, PulseWidthS: .019, PulsePeriodS: .02,
			},
		},
	}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: "load", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &loadValue,
	}, {
		Component: OperatingHarnessComponentID("short_circuit", "output"),
		Family:    "resistor",
		ValueSI:   float64PointerForTest(ShortCircuitHarnessOpenResistanceOhm),
	}}}
	planned := PlannedAnalysis{
		ID: "transient_fault", Kind: simmodel.AnalysisTransient, OperatingCase: "faulted",
		Requirements: []string{"response"}, Events: []string{"short"},
	}
	analysisPlan := AnalysisPlan{
		Events: []PlannedEvent{{
			ID: "short", Kind: "short_circuit", OperatingCase: "faulted", Target: "output",
			TriggerTimeS: 1, DurationS: .05, Initial: float64PointerForTest(4), Applied: .05, Recovered: float64PointerForTest(4), Unit: "Ohm",
		}},
		Assertions: []PlannedAssertion{{
			RequirementID: "response", AnalysisID: planned.ID, OperatingCase: "faulted",
			Metric: "protection_response_time", Target: "event:short", Max: float64PointerForTest(.001), Unit: "s",
		}},
	}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_resistance", Target: "output", Kind: OperatingDeviceValueSI, Component: "load", Scale: 1,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"short"}) {
		t.Fatalf("applied=%#v diagnostics=%#v", applied, diagnostics)
	}
	if analysis.Excitations[0].SineAmplitude != 1 || analysis.Excitations[0].SineFrequencyHz != 1000 {
		t.Fatalf("periodic response drive was neutralized: %#v", analysis.Excitations[0])
	}
	control := analysis.Excitations[1]
	if control.DCValue != 0 || control.PulseInitialValue != 0 || control.PulseValue != 5 ||
		control.PulseDelayS != 2*analysis.TimeStepS || control.PulseWidthS != analysis.DurationS ||
		control.PulsePeriodS != control.PulseDelayS+analysis.DurationS+analysis.TimeStepS {
		t.Fatalf("fallback control pulse was not staged from a stable boundary: %#v", control)
	}
	if len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("device events = %#v", analysis.DeviceValueEvents)
	}
	event := analysis.DeviceValueEvents[0]
	if event.OriginalTriggerTimeS != 1 || math.Abs(event.TriggerTimeS-.004) > 1e-12 ||
		math.Abs(analysis.DurationS-.054) > 1e-12 || analysis.DurationS/analysis.TimeStepS > 2048 {
		t.Fatalf("compacted event=%#v duration=%.12g step=%.12g", event, analysis.DurationS, analysis.TimeStepS)
	}
}

func TestCompactPlannedEventWindowPreservesUnrelatedOperatingPrelude(t *testing.T) {
	analysis := simmodel.Analysis{
		ID: "response", Kind: simmodel.AnalysisTransient, DurationS: 1.05, TimeStepS: .0001,
		SourceValueEvents: []simmodel.SourceValueEvent{{
			ID: "shutdown_supply", Component: "supply", TriggerTimeS: 1, DurationS: .05,
			Initial: 12, Applied: 0,
		}},
		DeviceValueEvents: []simmodel.DeviceValueEvent{{
			ID: "operating_load_current", Component: "load", TriggerTimeS: .0001, DurationS: .0399,
			InitialSI: 1e12, AppliedSI: 10,
		}},
	}

	compactPlannedEventWindow(&analysis, []string{"shutdown"})

	planned := analysis.SourceValueEvents[0]
	prelude := analysis.DeviceValueEvents[0]
	if math.Abs(planned.TriggerTimeS-.002) > 1e-12 || planned.OriginalTriggerTimeS != 1 {
		t.Fatalf("compacted planned event = %#v", planned)
	}
	if prelude.TriggerTimeS != .0001 || prelude.OriginalTriggerTimeS != 0 {
		t.Fatalf("unrelated operating prelude was rebased: %#v", prelude)
	}
	if math.Abs(analysis.DurationS-.052) > 1e-12 {
		t.Fatalf("compacted duration = %.12g", analysis.DurationS)
	}
}

func TestEventBearingPowerAnalysisPreservesPeriodicDrive(t *testing.T) {
	loadValue := 8.0
	analysis := simmodel.Analysis{
		ID: "power", Kind: simmodel.AnalysisTransient, DurationS: .02, TimeStepS: 50e-6,
		Excitations: []simmodel.SourceExcitation{{Component: "input", SineAmplitude: 2, SineFrequencyHz: 1000}},
	}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: "load", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &loadValue,
	}}}
	planned := PlannedAnalysis{
		ID: "transient_power", Kind: simmodel.AnalysisTransient, OperatingCase: "loaded",
		Requirements: []string{"power"}, Events: []string{"load_step"},
	}
	analysisPlan := AnalysisPlan{
		Events: []PlannedEvent{{
			ID: "load_step", Kind: "load_step", OperatingCase: "loaded", Target: "output",
			TriggerTimeS: .01, DurationS: .005, Initial: float64PointerForTest(8), Applied: 4, Recovered: float64PointerForTest(8), Unit: "Ohm",
		}},
		Assertions: []PlannedAssertion{{
			RequirementID: "power", AnalysisID: planned.ID, OperatingCase: "loaded",
			Metric: "output_power", Target: "output", Min: float64PointerForTest(1), Unit: "W",
		}},
	}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_resistance", Target: "output", Kind: OperatingDeviceValueSI, Component: "load", Scale: 1,
	}}

	applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, analysisPlan, bindings)
	if len(diagnostics) != 0 || !slices.Equal(applied, []string{"load_step"}) {
		t.Fatalf("applied=%#v diagnostics=%#v", applied, diagnostics)
	}
	if excitation := analysis.Excitations[0]; excitation.SineAmplitude != 2 || excitation.SineFrequencyHz != 1000 {
		t.Fatalf("periodic power drive was neutralized: %#v", excitation)
	}
}

func TestOperatingDeviceValueAllowsZeroOnlyForCapacitors(t *testing.T) {
	zero := 0.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: "capacitive_load", Family: "capacitor"}, {Component: "resistive_load", Family: "resistor"}}}
	analysis := simmodel.Analysis{}
	if diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_capacitance", Kind: OperatingDeviceValueSI, Component: "capacitive_load"}, CornerAssignment{Value: &zero}); diagnostic != nil {
		t.Fatalf("zero-capacitance corner = %#v", diagnostic)
	}
	if len(analysis.DeviceOverrides) != 1 || analysis.DeviceOverrides[0].ValueSI == nil || *analysis.DeviceOverrides[0].ValueSI != 0 {
		t.Fatalf("zero-capacitance override = %#v", analysis.DeviceOverrides)
	}
	if diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "load_resistance", Kind: OperatingDeviceValueSI, Component: "resistive_load"}, CornerAssignment{Value: &zero}); diagnostic == nil {
		t.Fatal("zero-resistance corner was accepted")
	}
}

func TestOperatingModelParameterMergesExistingDeviceOverride(t *testing.T) {
	valueSI := 1_000.0
	analysis := simmodel.Analysis{DeviceOverrides: []simmodel.DeviceOverride{{
		Component: "gain_device",
		ValueSI:   &valueSI,
		ModelParameters: []simmodel.NamedValue{
			{Name: "forward_beta", Value: 80},
		},
	}}}
	earlyVoltage := 120.0
	diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{
		Axis: "model_corner", Kind: OperatingModelParameter, Component: "gain_device", Parameter: "early_voltage",
	}, CornerAssignment{Value: &earlyVoltage})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if len(analysis.DeviceOverrides) != 1 {
		t.Fatalf("device overrides = %#v, want one merged component override", analysis.DeviceOverrides)
	}
	override := analysis.DeviceOverrides[0]
	if override.ValueSI == nil || *override.ValueSI != valueSI {
		t.Fatalf("merged override lost device value: %#v", override)
	}
	if len(override.ModelParameters) != 2 || override.ModelParameters[0] != (simmodel.NamedValue{Name: "early_voltage", Value: 120}) || override.ModelParameters[1] != (simmodel.NamedValue{Name: "forward_beta", Value: 80}) {
		t.Fatalf("merged model parameters = %#v", override.ModelParameters)
	}
}

func TestAmbientTemperatureCornerOverridesTemperatureSensitiveDevices(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "output_npn", ModelParameters: []simmodel.NamedValue{{Name: "junction_temperature_k", Value: 298.15}}},
		{Component: "output_pnp", ModelParameters: []simmodel.NamedValue{{Name: "junction_temperature_k", Value: 298.15}}},
		{Component: "load", ModelParameters: []simmodel.NamedValue{{Name: "resistance_temperature_coefficient", Value: 1e-6}}},
	}}
	analysis := simmodel.Analysis{DeviceOverrides: []simmodel.DeviceOverride{{Component: "output_npn", ModelParameters: []simmodel.NamedValue{{Name: "forward_beta", Value: 80}}}}}
	ambientC := 50.0
	diagnostic := applyOperatingAssignment(&analysis, &plan, SimulationOperatingBinding{Axis: "ambient_temperature", Kind: OperatingAnalysisCondition, Parameter: "ambient_temperature_c"}, CornerAssignment{Value: &ambientC})
	if diagnostic != nil {
		t.Fatal(diagnostic)
	}
	if len(analysis.Conditions) != 1 || analysis.Conditions[0].Name != "ambient_temperature_c" || analysis.Conditions[0].Value != 50 {
		t.Fatalf("ambient condition = %#v", analysis.Conditions)
	}
	if len(analysis.DeviceOverrides) != 2 {
		t.Fatalf("temperature overrides = %#v", analysis.DeviceOverrides)
	}
	for _, override := range analysis.DeviceOverrides {
		if override.Component == "load" {
			t.Fatalf("temperature-insensitive device received override: %#v", override)
		}
		foundTemperature := false
		for _, parameter := range override.ModelParameters {
			if parameter.Name == "junction_temperature_k" {
				foundTemperature = true
				if parameter.Value != 323.15 {
					t.Fatalf("%s junction temperature = %.12g K", override.Component, parameter.Value)
				}
			}
		}
		if !foundTemperature {
			t.Fatalf("%s missing junction-temperature override: %#v", override.Component, override)
		}
	}
}

func TestAmbientTemperatureCornerRejectsAbsoluteZero(t *testing.T) {
	analysis := simmodel.Analysis{}
	ambientC := -273.15
	if diagnostic := applyOperatingAssignment(&analysis, &simmodel.Plan{}, SimulationOperatingBinding{Axis: "ambient_temperature", Kind: OperatingAnalysisCondition, Parameter: "ambient_temperature_c"}, CornerAssignment{Value: &ambientC}); diagnostic == nil {
		t.Fatal("absolute-zero ambient corner was accepted")
	}
}

func TestInputFrequencyCornerUpdatesResolvedSource(t *testing.T) {
	frequency := 20_000.0
	analysis := simmodel.Analysis{
		Kind: simmodel.AnalysisDistortion, TimeStepS: 1.0 / (20 * 64), DurationS: 4.0 / 20,
		Excitations: []simmodel.SourceExcitation{{
			Component: "input", SineAmplitude: 1, SineFrequencyHz: 20,
		}},
	}
	diagnostic := applyOperatingAssignment(
		&analysis,
		&simmodel.Plan{},
		SimulationOperatingBinding{Axis: "input_frequency", Kind: OperatingSourceFrequencyHz, Component: "input"},
		CornerAssignment{Value: &frequency},
	)
	if diagnostic != nil || analysis.Excitations[0].SineFrequencyHz != frequency ||
		math.Abs(analysis.TimeStepS-1/(frequency*64)) > 1e-15 ||
		math.Abs(analysis.DurationS-4/frequency) > 1e-15 {
		t.Fatalf("source-frequency assignment = %#v diagnostic=%#v", analysis.Excitations[0], diagnostic)
	}
}

func TestInputFrequencyCornerRetimesTransientPeriodicGrid(t *testing.T) {
	frequency := 20_000.0
	analysis := simmodel.Analysis{
		Kind: simmodel.AnalysisTransient, TimeStepS: 1.0 / (1_000 * 32), DurationS: 20.0 / 1_000,
		Excitations: []simmodel.SourceExcitation{{
			Component: "input", SineAmplitude: 1, SineFrequencyHz: 1_000,
		}},
	}
	diagnostic := applyOperatingAssignment(
		&analysis,
		&simmodel.Plan{},
		SimulationOperatingBinding{Axis: "input_frequency", Kind: OperatingSourceFrequencyHz, Component: "input"},
		CornerAssignment{Value: &frequency},
	)
	if diagnostic != nil || analysis.Excitations[0].SineFrequencyHz != frequency ||
		math.Abs(analysis.TimeStepS-1/(frequency*32)) > 1e-15 ||
		math.Abs(analysis.DurationS-20/frequency) > 1e-15 {
		t.Fatalf("transient source-frequency assignment = %#v diagnostic=%#v", analysis, diagnostic)
	}
}

func TestEventSupplyComponentsIncludeUnsweptExternalRails(t *testing.T) {
	bindings := []SimulationOperatingBinding{
		{Axis: eventSupplyAxis, Target: "VP", Kind: OperatingSourceDCValue, Component: "positive_supply"},
		{Axis: eventSupplyAxis, Target: "VN", Kind: OperatingSourceDCValue, Component: "negative_supply"},
		{Axis: "input_frequency", Target: "IN", Kind: OperatingSourceFrequencyHz, Component: "signal"},
	}
	if got := eventSupplyComponents(bindings); !slices.Equal(got, []string{"negative_supply", "positive_supply"}) {
		t.Fatalf("event supply components = %#v", got)
	}
}

func TestPlannedPowerEventsCoupleGeneratedDomainControls(t *testing.T) {
	for _, test := range []struct {
		name             string
		kind             string
		initial, applied float64
		wantInitial      float64
		wantApplied      float64
	}{
		{name: "startup", kind: "startup", initial: 0, applied: 12, wantInitial: 0, wantApplied: -3.3},
		{name: "shutdown", kind: "shutdown", initial: 12, applied: 0, wantInitial: -3.3, wantApplied: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := test.initial
			analysis := simmodel.Analysis{
				ID: "power_event", Kind: simmodel.AnalysisTransient, DurationS: .1, TimeStepS: .001,
				Excitations: []simmodel.SourceExcitation{
					{Component: "supply", DCValue: 12},
					{Component: "enable", DCValue: -3.3},
					{Component: "secondary_enable", DCValue: -1.8},
				},
			}
			plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
				Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
				Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VIN"}, {Terminal: "NEGATIVE", Net: "GND"}},
			}}}
			event := PlannedEvent{
				ID: test.name, Kind: test.kind, OperatingCase: "normal", Target: "VIN",
				TriggerTimeS: .01, DurationS: .02, Initial: &initial, Applied: test.applied, Unit: "V",
			}
			planned := PlannedAnalysis{ID: "power_event", Kind: simmodel.AnalysisTransient, OperatingCase: "normal", Events: []string{test.name}}
			bindings := []SimulationOperatingBinding{
				{Axis: eventSupplyAxis, Target: "VIN", Kind: OperatingSourceDCValue, Component: "supply"},
				{Axis: "generated_domain_control", Target: "ENABLE", Kind: OperatingGeneratedControl, Component: "enable"},
				{Axis: "generated_domain_control", Target: "SECONDARY_ENABLE", Kind: OperatingGeneratedControl, Component: "secondary_enable"},
			}
			applied, diagnostics := applyPlannedEvents(&analysis, plan, planned, AnalysisPlan{Events: []PlannedEvent{event}}, bindings)
			if len(diagnostics) != 0 || !slices.Equal(applied, []string{test.name}) {
				t.Fatalf("applied=%#v diagnostics=%#v", applied, diagnostics)
			}
			var control *simmodel.SourceValueEvent
			controlCount := 0
			seenEventIDs := map[string]bool{}
			for index := range analysis.SourceValueEvents {
				event := &analysis.SourceValueEvents[index]
				if seenEventIDs[event.ID] {
					t.Fatalf("duplicate generated event ID %q", event.ID)
				}
				seenEventIDs[event.ID] = true
				if event.Component == "enable" || event.Component == "secondary_enable" {
					controlCount++
				}
				if analysis.SourceValueEvents[index].Component == "enable" {
					control = &analysis.SourceValueEvents[index]
				}
			}
			if controlCount != 2 || control == nil || control.Initial != test.wantInitial || control.Applied != test.wantApplied {
				t.Fatalf("generated-domain control event = %#v", control)
			}
		})
	}
}

func TestEdgeTimeAssertionsRequireDynamicExcitation(t *testing.T) {
	static := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "load", PulseInitialValue: 0, PulseValue: 0, PulseWidthS: 1e-3, PulsePeriodS: 2e-3}}}
	dynamicPulse := static
	dynamicPulse.Excitations = append([]simmodel.SourceExcitation(nil), static.Excitations...)
	dynamicPulse.Excitations[0].PulseValue = 3
	dynamicSine := simmodel.Analysis{Excitations: []simmodel.SourceExcitation{{Component: "input", SineAmplitude: 1, SineFrequencyHz: 1000}}}
	if analysisHasDynamicExcitation(static) {
		t.Fatal("constant pulse endpoints were treated as a dynamic excitation")
	}
	if !analysisHasDynamicExcitation(dynamicPulse) || !analysisHasDynamicExcitation(dynamicSine) {
		t.Fatal("bounded changing excitation was not recognized")
	}
	for _, quantity := range []string{simmodel.QuantityRiseTimeS, simmodel.QuantityFallTimeS, simmodel.QuantitySettlingTimeS, simmodel.QuantityResponseTimeS} {
		if !edgeTimeQuantity(quantity) {
			t.Fatalf("%s is not recognized as an edge-time quantity", quantity)
		}
	}
	if edgeTimeQuantity(simmodel.QuantityOutputPowerW) {
		t.Fatal("non-edge measurement was classified as edge time")
	}
}

func TestDynamicAnalysesPartitionAtTrustedPlanWorkBound(t *testing.T) {
	var analyses []simmodel.Analysis
	for index := 0; index < 128 && (len(analyses) == 0 || simmodel.FitsPlanDynamicWork(analyses)); index++ {
		analyses = append(analyses, simmodel.Analysis{
			ID: fmt.Sprintf("startup_%03d", index), Kind: simmodel.AnalysisStartup,
			DurationS: 100e-6, TimeStepS: 100e-6 / 256,
		})
	}
	if simmodel.FitsPlanDynamicWork(analyses) {
		t.Fatal("could not construct an analysis set beyond the trusted plan work bound")
	}
	batches := partitionAnalysesByDynamicWork(analyses)
	if len(batches) < 2 {
		t.Fatalf("dynamic batches = %d, want at least 2", len(batches))
	}
	covered := 0
	for _, batch := range batches {
		if !simmodel.FitsPlanDynamicWork(batch) {
			t.Fatalf("partition exceeds trusted work bound: %d analyses", len(batch))
		}
		covered += len(batch)
	}
	if covered != len(analyses) {
		t.Fatalf("partition covered %d/%d analyses", covered, len(analyses))
	}
}

func TestVoltageEventHarnessIsAbsentFromUnrelatedDynamicBatch(t *testing.T) {
	harness := OperatingHarnessComponentID("voltage_event", "OUT")
	analyses := []simmodel.Analysis{
		{
			ID: "event", Kind: simmodel.AnalysisTransient,
			Excitations: []simmodel.SourceExcitation{{Component: harness, DCValue: 5}},
			SourceValueEvents: []simmodel.SourceValueEvent{{
				ID: "short", Component: harness, Initial: 5, Applied: 0,
			}},
		},
		{
			ID: "nominal", Kind: simmodel.AnalysisTransient,
			Excitations: []simmodel.SourceExcitation{{Component: harness, DCValue: 5}},
		},
	}
	harnesses := map[string]bool{harness: true}
	batches := partitionAnalysesByDynamicWorkAndVoltageEventHarness(analyses, harnesses)
	if len(batches) != 2 {
		t.Fatalf("dynamic batches = %d, want separate event and nominal batches", len(batches))
	}

	value := 5.0
	plan := simmodel.Plan{
		Devices: []simmodel.ResolvedDevice{{
			Component: harness, PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, ValueSI: &value,
		}},
		Analyses: append([]simmodel.Analysis(nil), batches[1]...),
	}
	pruneInactiveVoltageEventHarnesses(&plan, activeVoltageEventHarnesses(batches[1][0], harnesses), harnesses)
	if len(plan.Devices) != 0 || len(plan.Analyses[0].Excitations) != 0 {
		t.Fatalf("inactive voltage harness survived nominal batch: devices=%#v analysis=%#v", plan.Devices, plan.Analyses[0])
	}

	plan.Devices = []simmodel.ResolvedDevice{{
		Component: harness, PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, ValueSI: &value,
	}}
	plan.Analyses = append([]simmodel.Analysis(nil), batches[0]...)
	pruneInactiveVoltageEventHarnesses(&plan, activeVoltageEventHarnesses(batches[0][0], harnesses), harnesses)
	if len(plan.Devices) != 1 || len(plan.Analyses[0].SourceValueEvents) != 1 {
		t.Fatalf("active voltage harness was pruned from event batch: devices=%#v analysis=%#v", plan.Devices, plan.Analyses[0])
	}
}

func TestCompileSimulationResolutionBindsDeviceCornersAndSharedAggregateLinks(t *testing.T) {
	baseIntent := simmodel.Intent{
		ModelID:    simmodel.ModelLinearCircuitMNAV1,
		Analyses:   []simmodel.Analysis{{ID: "placeholder", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []simmodel.Assertion{{AnalysisID: "placeholder", Node: "OUT", Quantity: simmodel.QuantityVoltageV, Min: 0, Max: 3}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source.current.generic", Family: "current_source", ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveCurrentSourceV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}},
	}
	base, baseDiagnostics := simmodel.ResolveWithTopology(baseIntent, "catalog", testHash("catalog"), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(baseDiagnostics) != 0 {
		t.Fatalf("base diagnostics = %#v", baseDiagnostics)
	}
	minimum, maximum := 0.9, 2.1
	minimumCurrent, maximumCurrent := .0009, .0011
	analysisPlan := AnalysisPlan{
		Schema: AnalysisPlanSchema, PlanHash: testHash("analysis-plan"),
		Analyses: []PlannedAnalysis{{ID: "dc_operating_point:load_range", Kind: simmodel.AnalysisDCOperatingPoint, OperatingCase: "load_range", Requirements: []string{"output", "output_duplicate", "load_current"}}},
		Corners: []PlannedCorner{
			{ID: "load_range:low", OperatingCase: "load_range", Assignments: []CornerAssignment{{Axis: "load_resistance", Target: "OUT", Value: float64PointerForTest(1000), Unit: "ohm"}}},
			{ID: "load_range:high", OperatingCase: "load_range", Assignments: []CornerAssignment{{Axis: "load_resistance", Target: "OUT", Value: float64PointerForTest(2000), Unit: "ohm"}}},
			{ID: "load_range:equivalent_high", OperatingCase: "load_range", Assignments: []CornerAssignment{{Axis: "load_resistance", Target: "OUT", Value: float64PointerForTest(2000), Unit: "ohm"}}},
		},
		Assertions: []PlannedAssertion{
			{RequirementID: "output", AnalysisID: "dc_operating_point:load_range", OperatingCase: "load_range", Metric: "dc_voltage", Target: "OUT", Min: &minimum, Max: &maximum, Unit: "V"},
			{RequirementID: "output", AnalysisID: "dc_operating_point:load_range", OperatingCase: "load_range", Metric: "dc_voltage", Target: "OUT", Min: &minimum, Max: &maximum, Unit: "V"},
			{RequirementID: "output_duplicate", AnalysisID: "dc_operating_point:load_range", OperatingCase: "load_range", Metric: "dc_voltage", Target: "OUT", Min: &minimum, Max: &maximum, Unit: "V"},
			{RequirementID: "load_current", AnalysisID: "dc_operating_point:load_range", OperatingCase: "load_range", Metric: "dc_current", Target: "OUT", Min: &minimumCurrent, Max: &maximumCurrent, Unit: "A"},
		},
	}
	template := base.Analyses[0]
	resolution, diagnostics := CompileSimulationResolution(
		analysisPlan,
		map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: base},
		[]SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: template}},
		[]SimulationAssertionBinding{
			{Metric: "dc_voltage", Target: "OUT", BoundsMode: AssertionBoundsDirect, Prototypes: []simmodel.Assertion{{Node: "OUT", Quantity: simmodel.QuantityVoltageV}}},
			{Metric: "dc_current", Target: "OUT", BoundsMode: AssertionBoundsDirect, Prototypes: []simmodel.Assertion{{Component: "load", Quantity: simmodel.QuantityDeviceCurrentA}}},
		},
		[]SimulationOperatingBinding{{Axis: "load_resistance", Target: "OUT", Kind: OperatingDeviceValueSI, Component: "load"}},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics = %#v", diagnostics)
	}
	if len(resolution.Plans) != 1 || len(resolution.Plans[0].Analyses) != 2 || len(resolution.Plans[0].Assertions) != 4 || len(resolution.Measurements) != 3 {
		t.Fatalf("resolution = %#v", resolution)
	}
	for _, measurement := range resolution.Measurements {
		wantQuantity := simmodel.QuantityVoltageV
		if measurement.RequirementID == "load_current" {
			wantQuantity = simmodel.QuantityDeviceCurrentA
		}
		if len(measurement.Assertions) != 2 {
			t.Fatalf("measurement %s links = %#v", measurement.RequirementID, measurement)
		}
		for _, assertionIndex := range measurement.Assertions {
			if got := resolution.Plans[0].Assertions[assertionIndex].Quantity; got != wantQuantity {
				t.Fatalf("measurement %s linked quantity = %s, want %s", measurement.RequirementID, got, wantQuantity)
			}
		}
	}
	report, evaluationDiagnostics := simmodel.Evaluate(resolution.Plans[0])
	if len(evaluationDiagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("compiled evaluation = %#v diagnostics=%#v", report, evaluationDiagnostics)
	}
	var voltageActuals []float64
	for index, assertion := range resolution.Plans[0].Assertions {
		if assertion.Quantity == simmodel.QuantityVoltageV {
			voltageActuals = append(voltageActuals, report.Assertions[index].Actual)
		}
	}
	slices.Sort(voltageActuals)
	if !slices.Equal(voltageActuals, []float64{1, 2}) {
		t.Fatalf("compiled voltage corner assertions = %#v", voltageActuals)
	}
}

func TestCompileSimulationResolutionExecutesBoundedSourceAndLoadEvents(t *testing.T) {
	baseIntent := simmodel.Intent{
		ModelID: simmodel.ModelTransientCircuitV1,
		Analyses: []simmodel.Analysis{{
			ID: "placeholder", Kind: simmodel.AnalysisTransient, DurationS: .2, TimeStepS: .1,
			Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: 1}},
		}},
		Assertions: []simmodel.Assertion{{AnalysisID: "placeholder", Node: "OUT", Quantity: simmodel.QuantityOutputSwingVPP, Min: 0, Max: 3}},
	}
	components := []simmodel.ComponentEvidence{
		{
			InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000,
			ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}},
			Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}},
		},
		{
			InstanceID: "source", CatalogID: "source.voltage.generic", Family: "voltage_source",
			ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveVoltageSourceV1}},
			Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "OUT"}, {Function: "NEGATIVE", Net: "GND"}},
		},
	}
	base, baseDiagnostics := simmodel.ResolveWithTopology(baseIntent, "catalog", testHash("catalog"), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(baseDiagnostics) != 0 {
		t.Fatalf("event base diagnostics = %#v", baseDiagnostics)
	}
	initialSource, appliedSource := 1.0, 2.0
	initialLoad, appliedLoad := 1000.0, 500.0
	maximum := 3.0
	analysisPlan := AnalysisPlan{
		Schema: AnalysisPlanSchema, PlanHash: testHash("event-analysis-plan"),
		Analyses: []PlannedAnalysis{{
			ID: "transient:event_case", Kind: simmodel.AnalysisTransient, OperatingCase: "event_case",
			Requirements: []string{"swing"}, Events: []string{"load_step", "source_step"},
		}},
		Corners: []PlannedCorner{{ID: "event_case:nominal", OperatingCase: "event_case"}},
		Events: []PlannedEvent{
			{ID: "load_step", Kind: "load_step", OperatingCase: "event_case", Target: "OUT", TriggerTimeS: .1, DurationS: .1, Initial: &initialLoad, Applied: appliedLoad, Unit: "Ohm"},
			{ID: "source_step", Kind: "input_step", OperatingCase: "event_case", Target: "OUT", TriggerTimeS: .1, DurationS: .1, Initial: &initialSource, Applied: appliedSource, Unit: "V"},
		},
		Assertions: []PlannedAssertion{{
			RequirementID: "swing", AnalysisID: "transient:event_case", OperatingCase: "event_case",
			Metric: "peak_to_peak_ripple", Target: "OUT", Max: &maximum, Unit: "V",
		}},
	}
	resolution, diagnostics := CompileSimulationResolution(
		analysisPlan,
		map[string]simmodel.Plan{simmodel.AnalysisTransient: base},
		[]SimulationAnalysisTemplate{{Kind: simmodel.AnalysisTransient, Analysis: base.Analyses[0]}},
		[]SimulationAssertionBinding{{
			Metric: "peak_to_peak_ripple", Target: "OUT", BoundsMode: AssertionBoundsDirect,
			Prototypes: []simmodel.Assertion{{Node: "OUT", Quantity: simmodel.QuantityOutputSwingVPP}},
		}},
		[]SimulationOperatingBinding{{Axis: "load_resistance", Target: "OUT", Kind: OperatingDeviceValueSI, Component: "load"}},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("event compile diagnostics = %#v", diagnostics)
	}
	if len(resolution.Plans) != 1 || len(resolution.Plans[0].Analyses) != 1 ||
		len(resolution.Plans[0].Analyses[0].SourceValueEvents) != 1 ||
		len(resolution.Plans[0].Analyses[0].DeviceValueEvents) != 1 {
		t.Fatalf("compiled event resolution = %#v", resolution)
	}
	report, evaluationDiagnostics := simmodel.Evaluate(resolution.Plans[0])
	if len(evaluationDiagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("compiled event evaluation = %#v diagnostics=%#v", report, evaluationDiagnostics)
	}
}

func TestApplyPlannedShortCircuitUsesResistanceEvent(t *testing.T) {
	value := ShortCircuitHarnessOpenResistanceOhm
	component := OperatingHarnessComponentID("short_circuit", "OUT")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component: component,
		Family:    "resistor",
		ValueSI:   &value,
	}}}
	analysis := simmodel.Analysis{
		ID: "short", Kind: simmodel.AnalysisElectrothermal,
		DurationS: 20e-3, TimeStepS: 1e-3,
	}
	initial, recovered := 5.0, 5.0
	event := PlannedEvent{
		ID: "output_short", Kind: "short_circuit", OperatingCase: "fault", Target: "OUT",
		TriggerTimeS: 5e-3, DurationS: 10e-3,
		Initial: &initial, Applied: 0, Recovered: &recovered, Unit: "V",
	}
	applied, diagnostic := applyPlannedEvent(&analysis, plan, event, AnalysisPlan{}, nil)
	if diagnostic != nil || !applied || len(analysis.SourceValueEvents) != 0 || len(analysis.DeviceValueEvents) != 1 {
		t.Fatalf("short-circuit event = %#v, applied=%t diagnostic=%#v", analysis, applied, diagnostic)
	}
	got := analysis.DeviceValueEvents[0]
	if got.Component != component ||
		got.InitialSI != ShortCircuitHarnessOpenResistanceOhm ||
		got.AppliedSI != ShortCircuitHarnessClosedResistanceOhm ||
		got.RecoveredSI == nil || *got.RecoveredSI != ShortCircuitHarnessOpenResistanceOhm {
		t.Fatalf("short-circuit resistance transition = %#v", got)
	}
}

func TestLoadCurrentFollowsExplicitPowerTransitionOnly(t *testing.T) {
	recovered := 0.0
	analysis := simmodel.Analysis{
		Excitations: []simmodel.SourceExcitation{
			{Component: "load", DCValue: 2},
			{Component: "supply", DCValue: 24},
		},
		SourceValueEvents: []simmodel.SourceValueEvent{
			{ID: "startup", Component: "supply", TriggerTimeS: 1e-3, DurationS: 2e-3, Initial: 0, Applied: 24},
			{ID: "input_step", Component: "supply", TriggerTimeS: 5e-3, DurationS: 1e-3, Initial: 18, Applied: 30, Recovered: &recovered},
		},
	}
	bindings := []SimulationOperatingBinding{
		{Axis: "load_current", Target: "OUT", Kind: OperatingLoadCurrent, Component: "load"},
		{Axis: "supply_voltage", Target: "VIN", Kind: OperatingSourceDCValue, Component: "supply"},
	}

	coupleLoadCurrentsToPowerEvents(&analysis, bindings)
	var loadEvents []simmodel.SourceValueEvent
	for _, event := range analysis.SourceValueEvents {
		if event.Component == "load" {
			loadEvents = append(loadEvents, event)
		}
	}
	if len(loadEvents) != 2 {
		t.Fatalf("coupled load events = %#v", loadEvents)
	}
	if loadEvents[0].Initial != 0 || loadEvents[0].Applied != 2 || loadEvents[0].Recovered != nil {
		t.Fatalf("startup load transition = %#v", loadEvents[0])
	}
	if loadEvents[1].Initial != 2 || loadEvents[1].Applied != 2 ||
		loadEvents[1].Recovered == nil || *loadEvents[1].Recovered != 0 {
		t.Fatalf("shutdown recovery load transition = %#v", loadEvents[1])
	}
}

func TestEventOnlyAnalysisAssertionUsesKindAppropriateScope(t *testing.T) {
	plan := simmodel.Plan{
		GroundNode: "GND",
		Nodes:      []string{"GND", "OUT"},
		Devices: []simmodel.ResolvedDevice{
			{Component: "z_switch", ThermalModel: &simmodel.ThermalRCNetwork{}},
			{Component: "a_switch", ThermalModel: &simmodel.ThermalRCNetwork{}},
		},
	}

	transient, ok := eventOnlyAnalysisAssertion(plan, simmodel.Analysis{
		ID: "transient_event", Kind: simmodel.AnalysisTransient, DurationS: 2e-3,
	})
	if !ok || transient.AnalysisID != "transient_event" || transient.Node != "OUT" ||
		transient.Component != "" || transient.Quantity != simmodel.QuantityVoltageV ||
		transient.TimeS != 2e-3 {
		t.Fatalf("transient event-only assertion = %#v, ok=%t", transient, ok)
	}

	electrothermal, ok := eventOnlyAnalysisAssertion(plan, simmodel.Analysis{
		ID: "electrothermal_event", Kind: simmodel.AnalysisElectrothermal, DurationS: 2e-3,
	})
	if !ok || electrothermal.AnalysisID != "electrothermal_event" || electrothermal.Node != "" ||
		electrothermal.Component != "a_switch" ||
		electrothermal.Quantity != simmodel.QuantityJunctionTemperatureC ||
		electrothermal.TimeS != 0 {
		t.Fatalf("electrothermal event-only assertion = %#v, ok=%t", electrothermal, ok)
	}

	plan.Devices = nil
	if assertion, ok := eventOnlyAnalysisAssertion(plan, simmodel.Analysis{
		ID: "electrothermal_event", Kind: simmodel.AnalysisElectrothermal,
	}); ok {
		t.Fatalf("electrothermal event-only assertion without thermal scope = %#v", assertion)
	}
}

func TestEventSourceForTargetRejectsCurrentSource(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{
			Component: "load_current", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "OUT"}, {Terminal: "NEGATIVE", Net: "GND"}},
		},
	}}
	bindings := []SimulationOperatingBinding{{
		Axis: "load_current", Target: "OUT", Kind: OperatingSourceDCValue, Component: "load_current",
	}}
	if component, ok := eventSourceForTarget(plan, bindings, "OUT"); ok {
		t.Fatalf("current source selected for voltage event: %q", component)
	}

	plan.Devices = append(plan.Devices, simmodel.ResolvedDevice{
		Component: "rail_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
		Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "OUT"}, {Terminal: "NEGATIVE", Net: "GND"}},
	})
	if component, ok := eventSourceForTarget(plan, bindings, "OUT"); !ok || component != "rail_source" {
		t.Fatalf("voltage event source = %q, %t", component, ok)
	}
}

func float64PointerForTest(value float64) *float64 { return &value }

type repairableFreshMNAResolver struct{}

func (repairableFreshMNAResolver) ResolveSimulationPlans(_ context.Context, state CandidateState) (map[string]simmodel.Plan, error) {
	load := 500.0
	for _, variable := range state.Variables {
		if variable.ID == "load_resistance" {
			load = variable.Value
		}
	}
	intent := simmodel.Intent{
		ModelID:    simmodel.ModelLinearCircuitMNAV1,
		Analyses:   []simmodel.Analysis{{ID: "placeholder", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: 5}}}},
		Assertions: []simmodel.Assertion{{AnalysisID: "placeholder", Node: "OUT", Quantity: simmodel.QuantityVoltageV, Min: 0, Max: 6}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "upper", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "OUT"}}},
		{InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", HasValueSI: true, ValueSI: load, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "source.voltage.connector.1x02", Family: "voltage_source", ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveVoltageSourceV1}}, Connections: []simmodel.ConnectionEvidence{{Function: "POSITIVE", Net: "VIN"}, {Function: "NEGATIVE", Net: "GND"}}},
	}
	plan, diagnostics := simmodel.ResolveWithTopology(intent, "catalog", testHash("catalog"), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "VIN"}})
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("fresh MNA resolve diagnostics: %#v", diagnostics)
	}
	return map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: plan}, nil
}

func TestPlannedSimulationResolverRepairsAndReevaluatesAllSupplyCorners(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimum, maximum := 2.2, 2.8
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{{
		ID: "output", Metric: "dc_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{"rated"}, Critical: true,
	}}
	modelDecision := ModelDecision{
		Component: "load", Family: "resistor", Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1}, Status: "used", Reason: "trusted DC model",
		RequiredAnalyses: []string{simmodel.AnalysisDCOperatingPoint},
		Provenance:       &simmodel.ModelProvenance{Source: "manufacturer:test", Revision: "a", SHA256: testHash("model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisDCOperatingPoint}},
	}
	analysisPlan, diagnostics := BuildAnalysisPlan(requirement, []SemanticBinding{{Kind: "port", ID: "output", Target: "OUT"}, {Kind: "domain", ID: "supply", Target: "SUPPLY"}}, []ModelDecision{modelDecision})
	if len(diagnostics) != 0 {
		t.Fatalf("analysis plan diagnostics = %#v", diagnostics)
	}
	resolver := PlannedSimulationResolver{
		Plan: analysisPlan, Base: repairableFreshMNAResolver{},
		Templates:         []SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: simmodel.Analysis{ID: "template", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "source", DCValue: 5}}}}},
		Assertions:        []SimulationAssertionBinding{{Metric: "dc_voltage", Target: "OUT", BoundsMode: AssertionBoundsDirect, Prototypes: []simmodel.Assertion{{Node: "OUT", Quantity: simmodel.QuantityVoltageV}}}},
		OperatingBindings: []SimulationOperatingBinding{{Axis: "supply_voltage", Target: "SUPPLY", Kind: OperatingSourceDCValue, Component: "source"}},
	}
	registry, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics = %#v", provenanceDiagnostics)
	}
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{
			Fingerprint: testHash("candidate"),
			Variables: []Variable{{
				ID: "load_resistance", Kind: "passive_value", Value: 500, AllowedValues: []float64{500, 1000},
				Effects: []RepairEffect{{Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "dc_voltage", Direction: RepairMetricIncreases}},
			}},
		}},
	}
	report := Run(context.Background(), input, SimModelEvaluator{Resolver: resolver, ProvenanceRegistry: registry}, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.State.Variables[0].Value != 1000 {
		t.Fatalf("planned closed-loop report = %#v", report)
	}
	if report.Consumption.Evaluations != 2 || report.Consumption.RepairsApplied != 1 || len(report.Candidates[0].Attempts[1].Assertions) != 1 {
		t.Fatalf("planned repair consumption/evidence = %#v", report)
	}
	resolution, err := resolver.ResolveSimulation(context.Background(), report.Selected.State)
	if err != nil || len(resolution.Plans) != 1 || len(resolution.Plans[0].Analyses) != 2 || len(resolution.Measurements[0].Assertions) != 2 {
		t.Fatalf("fresh selected resolution = %#v err=%v", resolution, err)
	}
}
