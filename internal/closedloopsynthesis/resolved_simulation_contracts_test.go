package closedloopsynthesis

import (
	"math"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

func TestPrimaryInputReferencePrefersSignalIngressOverControl(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Ports: []architecturesearch.Port{
			{ID: "input", Kind: "analog_voltage", Direction: "sink"},
			{ID: "mute", Kind: "digital_logic", Direction: "sink"},
		},
		Signals: []architecturesearch.Signal{{ID: "muted", Kind: "analog_voltage"}},
		Objectives: []architecturesearch.Objective{{Bindings: []architecturesearch.Binding{
			{Role: "signal", Port: "input"}, {Role: "control", Port: "mute"}, {Role: "output", Signal: "muted", Direction: "source"},
		}}},
	}}
	node, ok := primaryInputReference(requirement, []SemanticBinding{{Kind: "port", ID: "input", Target: "IN"}, {Kind: "port", ID: "mute", Target: "MUTE"}})
	if !ok || node != "IN" {
		t.Fatalf("primary input = %q, %v; want IN, true", node, ok)
	}
}

func TestSourceSweepExcitationScalePreservesSemanticConnectorPolarity(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
		Component:      "input",
		PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1,
		Terminals:      []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "GND"}, {Terminal: "PIN_2", Net: "SIGNAL"}},
	}}}

	scale, ok := sourceSweepExcitationScale(plan, "input", "SIGNAL", "threshold_voltage")
	if !ok || scale != -1 {
		t.Fatalf("scale = %g, %v; want -1, true", scale, ok)
	}
}

func TestVoltageSourcePolarityDrivesSemanticNodeRelativeToOppositeTerminal(t *testing.T) {
	tests := []struct {
		name      string
		primitive string
		positive  string
		negative  string
	}{
		{name: "voltage_source", primitive: simmodel.PrimitiveVoltageSourceV1, positive: "POSITIVE", negative: "NEGATIVE"},
		{name: "connector_source", primitive: simmodel.PrimitiveConnectorVoltageSourceV1, positive: "PIN_1", negative: "PIN_2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, semanticOnPositive := range []bool{true, false} {
				positiveNet, negativeNet := "SEMANTIC", "REFERENCE"
				want := 1.0
				if !semanticOnPositive {
					positiveNet, negativeNet = negativeNet, positiveNet
					want = -1
				}
				plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{
					Component:      "source",
					PrimitiveModel: test.primitive,
					Terminals: []simmodel.TerminalBinding{
						{Terminal: test.positive, Net: positiveNet},
						{Terminal: test.negative, Net: negativeNet},
					},
				}}}
				if got, ok := sourceSweepExcitationScale(plan, "source", "SEMANTIC", "threshold_voltage"); !ok || got != want {
					t.Fatalf("sweep scale = %g, %v; want %g, true", got, ok, want)
				}
				if got, ok := resolvedVoltageSourcePolarity(plan, "source", "SEMANTIC"); !ok || got != want {
					t.Fatalf("resolved polarity = %g, %v; want %g, true", got, ok, want)
				}
			}
		})
	}
}

func TestSeriesRelayMuteUsesEnergizedNormalAndFailSafeDeenergizedMute(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "logic", NominalVoltageV: 3.3}},
		Ports:   []architecturesearch.Port{{ID: "input"}, {ID: "mute", Domain: "logic"}},
		Signals: []architecturesearch.Signal{{ID: "muted"}},
		Objectives: []architecturesearch.Objective{{Capability: "mute_control", Bindings: []architecturesearch.Binding{
			{Role: "signal", Port: "input"}, {Role: "control", Port: "mute"}, {Role: "output", Signal: "muted"},
		}}},
	}}
	bindings := []SemanticBinding{{Kind: "port", ID: "input", Target: "IN"}, {Kind: "port", ID: "mute", Target: "MUTE"}, {Kind: "signal", ID: "muted", Target: "OUT"}}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "mute_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "MUTE"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		{Component: "series_switch", PrimitiveModel: simmodel.PrimitiveRelayNormallyOpenV1, Terminals: []simmodel.TerminalBinding{{Terminal: "CONTACT_IN", Net: "IN"}, {Terminal: "CONTACT_OUT", Net: "OUT"}}},
	}}
	normal, muted, ok := ResolveMuteExcitationStates(requirement, bindings, plan)
	if !ok || normal.Component != "mute_source" || normal.DCValue != 3.3 || muted.Component != "mute_source" || muted.DCValue != 0 {
		t.Fatalf("series relay mute states = normal %#v muted %#v ok=%v", normal, muted, ok)
	}
}

func TestStabilityObservationResolvesUniqueUpstreamOpAmpThroughProtection(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "amplifier", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "AMP_OUT"}}},
		{Component: "other", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "OTHER_OUT"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "AMP_OUT"}, {Terminal: "B", Net: "OUTPUT"}}},
		{Component: "pulldown", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "OUTPUT"}, {Terminal: "B", Net: "GND"}}},
	}}

	node, ok := stabilityObservationNode(plan, "OUTPUT")
	if !ok || node != "AMP_OUT" {
		t.Fatalf("stability node = %q, %v; want AMP_OUT, true", node, ok)
	}
}

func TestStabilityObservationFailsClosedForAmbiguousPassiveFanIn(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "left", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "LEFT"}}},
		{Component: "right", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "RIGHT"}}},
		{Component: "left_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "LEFT"}, {Terminal: "B", Net: "OUTPUT"}}},
		{Component: "right_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "RIGHT"}, {Terminal: "B", Net: "OUTPUT"}}},
	}}

	if node, ok := stabilityObservationNode(plan, "OUTPUT"); ok || node != "" {
		t.Fatalf("stability node = %q, %v; want ambiguous failure", node, ok)
	}
}

func TestStabilityObservationResolvesEmitterDegeneratedBJTCollector(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "transistor", PrimitiveModel: simmodel.PrimitiveBJTNPNV1, ModelParameters: []simmodel.NamedValue{{Name: "transition_frequency_hz", Value: 40e6}}, Terminals: []simmodel.TerminalBinding{{Terminal: "BASE", Net: "BASE"}, {Terminal: "COLLECTOR", Net: "COLLECTOR"}, {Terminal: "EMITTER", Net: "EMITTER"}}},
		{Component: "emitter_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "EMITTER"}, {Terminal: "B", Net: "GND"}}},
		{Component: "output_capacitor", PrimitiveModel: simmodel.PrimitiveCapacitorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "COLLECTOR"}, {Terminal: "B", Net: "PROTECTED"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "PROTECTED"}, {Terminal: "B", Net: "OUTPUT"}}},
	}}

	node, ok := stabilityObservationNode(plan, "OUTPUT")
	if !ok || node != "COLLECTOR" {
		t.Fatalf("BJT stability node = %q, %v; want COLLECTOR, true", node, ok)
	}
}

func TestStabilityObservationTraversesComplementaryEmitterFollowerControlPath(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "driver", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "DRIVE"}}},
		{Component: "bias_diode", PrimitiveModel: simmodel.PrimitiveDiodeShockleyV1, Terminals: []simmodel.TerminalBinding{{Terminal: "ANODE", Net: "BASE_BIAS"}, {Terminal: "CATHODE", Net: "DRIVE"}}},
		{Component: "base_stop", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "BASE_BIAS"}, {Terminal: "B", Net: "BASE"}}},
		{Component: "output_npn", PrimitiveModel: simmodel.PrimitiveBJTNPNV1, ModelParameters: []simmodel.NamedValue{{Name: "transition_frequency_hz", Value: 30e6}}, Terminals: []simmodel.TerminalBinding{{Terminal: "BASE", Net: "BASE"}, {Terminal: "COLLECTOR", Net: "VP"}, {Terminal: "EMITTER", Net: "EMITTER"}}},
		{Component: "emitter_resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "EMITTER"}, {Terminal: "B", Net: "AMP_OUT"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "AMP_OUT"}, {Terminal: "B", Net: "OUTPUT"}}},
	}}

	node, ok := stabilityObservationNode(plan, "OUTPUT")
	if !ok || node != "DRIVE" {
		t.Fatalf("buffered stability node = %q, %v; want DRIVE, true", node, ok)
	}
}

func TestUniqueLoadComponentPrefersSemanticOperatingHarness(t *testing.T) {
	target := "OUTPUT"
	harness := OperatingHarnessComponentID("load_resistance", target)
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: harness, Family: "resistor", Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: target}, {Terminal: "B", Net: "GND"}}},
		{Component: "pulldown", Family: "resistor", Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: target}, {Terminal: "B", Net: "GND"}}},
	}}
	component, ok := uniqueLoadComponent(plan, target)
	if !ok || component != harness {
		t.Fatalf("load component = %q, %v; want harness", component, ok)
	}
}

func TestTransimpedanceUsesResolvedLoadCurrentExcitation(t *testing.T) {
	load := OperatingHarnessComponentID("load_current", "LOAD")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1},
		{Component: load, PrimitiveModel: simmodel.PrimitiveCurrentSourceV1},
	}}
	binding, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "sense", Metric: "transimpedance", Target: "SENSE"},
		"", nil,
		[]SimulationOperatingBinding{{Axis: "load_current", Target: "LOAD", Kind: OperatingSourceDCValue, Component: load}},
		plan, architecturesearch.Requirement{}, nil,
	)
	if diagnostic != nil {
		t.Fatalf("resolve transimpedance: %#v", diagnostic)
	}
	if len(binding.Prototypes) != 1 || binding.Prototypes[0].Component != load || binding.Prototypes[0].Quantity != simmodel.QuantityTransimpedanceOhm {
		t.Fatalf("transimpedance binding = %#v", binding)
	}
}

func TestTransimpedanceFailsClosedForAmbiguousLoadCurrentExcitation(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "left", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1},
		{Component: "right", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1},
	}}
	_, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "sense", Metric: "transimpedance", Target: "SENSE"},
		"", nil,
		[]SimulationOperatingBinding{
			{Axis: "load_current", Target: "LEFT", Kind: OperatingSourceDCValue, Component: "left"},
			{Axis: "load_current", Target: "RIGHT", Kind: OperatingSourceDCValue, Component: "right"},
		},
		plan, architecturesearch.Requirement{}, nil,
	)
	if diagnostic == nil {
		t.Fatal("ambiguous load-current excitations satisfied transimpedance binding")
	}
}

func TestModelParameterAllUsesRegisteredWorstCaseExpansion(t *testing.T) {
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{{ID: "model", Assignments: []CornerAssignment{{Axis: "model_parameter", Target: "circuit", Selection: "all"}}}}}
	diagnostics := []Diagnostic{}
	bindings := resolvedOperatingBindings(analysisPlan, map[string]simmodel.Plan{"dc": {}}, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 || bindings[0].Kind != OperatingWorstCase {
		t.Fatalf("model-parameter bindings = %#v diagnostics=%#v", bindings, diagnostics)
	}
}

func TestResolvedLoadCurrentBindingSpansDrivenAndPhysicalStartupLoads(t *testing.T) {
	zero, maximum, resistance := 0.0, 3.0, 4.0
	component := OperatingHarnessComponentID("load_current", "LOAD")
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{
		{ID: "zero", Assignments: []CornerAssignment{{Axis: "load_current", Target: "LOAD", Value: &zero}}},
		{ID: "maximum", Assignments: []CornerAssignment{{Axis: "load_current", Target: "LOAD", Value: &maximum}}},
	}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisDCOperatingPoint: {Devices: []simmodel.ResolvedDevice{{Component: component, Family: "current_source", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "NEGATIVE", Net: "LOAD"}}}}},
		simmodel.AnalysisStartup:          {Devices: []simmodel.ResolvedDevice{{Component: component, Family: "resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &resistance, Terminals: []simmodel.TerminalBinding{{Terminal: "B", Net: "LOAD"}}}}},
	}
	var diagnostics []Diagnostic
	bindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 || bindings[0].Kind != OperatingLoadCurrent || bindings[0].Component != component || bindings[0].Scale != 12 {
		t.Fatalf("bindings = %#v diagnostics=%#v", bindings, diagnostics)
	}
}

func TestSupplySourceComponentsResolveEverySemanticRailAndExcludeSignalSources(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "positive_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VP"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		{Component: "negative_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "GND"}, {Terminal: "NEGATIVE", Net: "VN"}}},
		{Component: "signal", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}}},
	}}

	components, ok := supplySourceComponents(plan, []string{"VN", "VP"})
	if !ok || len(components) != 2 || components[0] != "negative_supply" || components[1] != "positive_supply" {
		t.Fatalf("supply source components = %#v, %v", components, ok)
	}
}

func TestCircuitQuiescentCurrentBindsOneSummedSupplyMeasurement(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "positive_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VP"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		{Component: "negative_supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "GND"}, {Terminal: "NEGATIVE", Net: "VN"}}},
	}}
	binding, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "bias", Metric: "quiescent_current", Target: "circuit"},
		"", []string{"VN", "VP"}, nil, plan, architecturesearch.Requirement{}, nil,
	)
	if diagnostic != nil {
		t.Fatalf("resolve quiescent current: %#v", diagnostic)
	}
	if len(binding.Prototypes) != 1 || binding.Prototypes[0].Quantity != simmodel.QuantityTotalSupplyCurrentA || !slices.Equal(binding.Prototypes[0].Components, []string{"negative_supply", "positive_supply"}) {
		t.Fatalf("quiescent-current binding = %#v", binding)
	}
}

func TestVoltageOperatingSourceExcludesCurrentLoadSharingSupplyNode(t *testing.T) {
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "GND"}}},
		{Component: "load", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VCC"}, {Terminal: "NEGATIVE", Net: "SWITCHED"}}},
	}}
	component, ok := uniqueVoltageSourceAcrossPlans(map[string]simmodel.Plan{"dc": plan}, "VCC")
	if !ok || component != "supply" {
		t.Fatalf("voltage source = %q, %v; want supply", component, ok)
	}
}

func TestThresholdCurrentSweepIsBoundedByDeclaredOperatingRange(t *testing.T) {
	zero, three, lower, upper := 0.0, 3.0, 1.9, 2.1
	component := OperatingHarnessComponentID("load_current", "LOAD")
	analysisPlan := AnalysisPlan{
		Assertions: []PlannedAssertion{{RequirementID: "trip", OperatingCase: "rated", Metric: "threshold_current", Min: &lower, Max: &upper}},
		Corners: []PlannedCorner{
			{OperatingCase: "rated", Assignments: []CornerAssignment{{Axis: "load_current", Target: "LOAD", Value: &zero}}},
			{OperatingCase: "rated", Assignments: []CornerAssignment{{Axis: "load_current", Target: "LOAD", Value: &three}}},
		},
	}
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{{Component: component, Family: "current_source", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1}}}
	templates := []SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: simmodel.Analysis{Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: component}}}}}
	if diagnostic := configureThresholdSweep(analysisPlan, map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: plan}, "", templates); diagnostic != nil {
		t.Fatalf("configure threshold sweep: %#v", diagnostic)
	}
	sweep := templates[0].Analysis.DCSweep
	if sweep == nil || sweep.StartValue != 0 || sweep.StopValue != 3 {
		t.Fatalf("threshold sweep = %#v, want 0..3 A", sweep)
	}
}

func TestThresholdVoltageSweepUsesLocalRequirementWindow(t *testing.T) {
	lower, upper := 2.4, 2.6
	analysisPlan := AnalysisPlan{Assertions: []PlannedAssertion{{RequirementID: "trip", OperatingCase: "rated", Metric: "threshold_voltage", Min: &lower, Max: &upper}}}
	plan := simmodel.Plan{
		GroundNode: "GND",
		Devices: []simmodel.ResolvedDevice{{
			Component: "signal", Family: "voltage_source", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "IN"}, {Terminal: "NEGATIVE", Net: "GND"}},
		}},
	}
	templates := []SimulationAnalysisTemplate{{Kind: simmodel.AnalysisDCOperatingPoint, Analysis: simmodel.Analysis{Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{{Component: "signal"}}}}}
	if diagnostic := configureThresholdSweep(analysisPlan, map[string]simmodel.Plan{simmodel.AnalysisDCOperatingPoint: plan}, "IN", templates); diagnostic != nil {
		t.Fatalf("configure threshold sweep: %#v", diagnostic)
	}
	sweep := templates[0].Analysis.DCSweep
	if sweep == nil || math.Abs(sweep.StartValue-2.0) > 1e-12 || math.Abs(sweep.StopValue-3.0) > 1e-12 || sweep.Points != 201 {
		t.Fatalf("threshold sweep = %#v, want local 2.0..3.0 V window", sweep)
	}
}
