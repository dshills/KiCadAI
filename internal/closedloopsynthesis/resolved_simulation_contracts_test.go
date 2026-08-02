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

func TestBehavioralVoltageAssertionUsesObservedDomainReference(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "host_5v", Kind: "supply"},
			{ID: "host_ground", Kind: "reference"},
			{ID: "remote_1v8", Kind: "supply"},
			{ID: "remote_ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{{ID: "remote_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "remote_1v8"}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "rise", Metric: "rise_time", Observation: architecturesearch.Observation{Kind: "port", ID: "remote_bus"},
		}},
	}}
	binding, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "rise", Metric: "rise_time", Target: "REMOTE_BUS"},
		"", nil, nil,
		simmodel.Plan{Nodes: []string{"REMOTE_BUS", "REMOTE_GROUND"}},
		requirement,
		[]SemanticBinding{
			{Kind: "domain", ID: "host_ground", Target: "HOST_GROUND"},
			{Kind: "domain", ID: "remote_ground", Target: "REMOTE_GROUND"},
		},
	)
	if diagnostic != nil {
		t.Fatal(diagnostic.Message)
	}
	if len(binding.Prototypes) != 1 || binding.Prototypes[0].ReferenceNode != "REMOTE_GROUND" {
		t.Fatalf("resolved assertion binding = %#v", binding)
	}
}

func TestEventVoltageAssertionUsesDeclaredTargetDomainReference(t *testing.T) {
	requirement := closedLoopTestRequirement()
	requirement.Schema = architecturesearch.SchemaIDV5
	requirement.Version = architecturesearch.VersionV5
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, architecturesearch.Domain{ID: "ground", Kind: "reference"})
	requirement.Requirements.Ports[0].Domain = "ground"
	requirement.Requirements.BehavioralRequirements[0].Observation = architecturesearch.Observation{Kind: "event", ID: "output_step"}
	requirement.Requirements.OperatingCases[0].Events = []architecturesearch.OperatingEvent{{
		ID: "output_step", Kind: "load_step", Target: architecturesearch.Observation{Kind: "port", ID: requirement.Requirements.Ports[0].ID},
	}}
	reference, required := behavioralObservationReferenceNode(
		requirement,
		requirement.Requirements.BehavioralRequirements[0].ID,
		requirement.Requirements.OperatingCases[0].ID,
		[]SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}},
	)
	if !required || reference != "GND" {
		t.Fatalf("event reference = %q, %t", reference, required)
	}
}

func TestBehavioralReferenceUsesProducingObjectiveAcrossIsolationBoundary(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "primary_power", Kind: "supply"},
			{ID: "secondary_power", Kind: "supply"},
			{ID: "primary_return", Kind: "reference"},
			{ID: "secondary_return", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{
			{ID: "input", Kind: "power", Domain: "primary_power"},
			{ID: "secondary_return", Kind: "reference", Domain: "secondary_return"},
		},
		Signals: []architecturesearch.Signal{{ID: "regulated", Kind: "power", Domain: "secondary_power"}},
		Objectives: []architecturesearch.Objective{{
			Capability: "voltage_regulation",
			Bindings: []architecturesearch.Binding{
				{Role: "input", Port: "input"},
				{Role: "output", Signal: "regulated", Direction: "source"},
				{Role: "reference", Port: "secondary_return"},
			},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "rail", Observation: architecturesearch.Observation{Kind: "signal", ID: "regulated"},
		}},
	}}
	reference, required := behavioralObservationReferenceNode(requirement, "rail", "", []SemanticBinding{
		{Kind: "domain", ID: "primary_return", Target: "PRIMARY_GROUND"},
		{Kind: "domain", ID: "secondary_return", Target: "SECONDARY_GROUND"},
	})
	if !required || reference != "SECONDARY_GROUND" {
		t.Fatalf("isolated behavioral reference = %q, %t; want secondary return", reference, required)
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
		{Component: "output_disconnect", PrimitiveModel: simmodel.PrimitivePMOSSwitchV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SOURCE", Net: "AMP_OUT"}, {Terminal: "DRAIN", Net: "PROTECTED"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "PROTECTED"}, {Terminal: "B", Net: "OUTPUT"}}},
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

func TestStabilityObservationResolvesSynchronousBuckThroughProtection(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "controller", PrimitiveModel: simmodel.PrimitiveSynchronousBuckRegulatorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SW", Net: "SW"}}},
		{Component: "inductor", PrimitiveModel: simmodel.PrimitiveInductorTransientV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SW"}, {Terminal: "B", Net: "BUCK_OUT"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "BUCK_OUT"}, {Terminal: "B", Net: "OUTPUT"}}},
	}}

	node, ok := stabilityObservationNode(plan, "OUTPUT")
	if !ok || node != "BUCK_OUT" {
		t.Fatalf("buck stability node = %q, %v; want BUCK_OUT, true", node, ok)
	}
}

func TestStabilityObservationDoesNotTraverseSeriesMOSFETGate(t *testing.T) {
	plan := simmodel.Plan{GroundNode: "GND", Devices: []simmodel.ResolvedDevice{
		{Component: "controller", PrimitiveModel: simmodel.PrimitiveSynchronousBuckRegulatorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SW", Net: "SW"}}},
		{Component: "inductor", PrimitiveModel: simmodel.PrimitiveInductorTransientV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SW"}, {Terminal: "B", Net: "BUCK_OUT"}}},
		{Component: "disconnect", PrimitiveModel: simmodel.PrimitivePMOSSwitchV1, Terminals: []simmodel.TerminalBinding{{Terminal: "SOURCE", Net: "BUCK_OUT"}, {Terminal: "DRAIN", Net: "PROTECTED"}, {Terminal: "GATE", Net: "GATE"}}},
		{Component: "output_fuse", PrimitiveModel: simmodel.PrimitiveFuseClosedStateV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "PROTECTED"}, {Terminal: "B", Net: "OUTPUT"}}},
		{Component: "gate_drive", PrimitiveModel: simmodel.PrimitiveResistorV1, Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "GATE"}, {Terminal: "B", Net: "SERVO_OUT"}}},
		{Component: "servo", PrimitiveModel: simmodel.PrimitiveOpAmpV1, Terminals: []simmodel.TerminalBinding{{Terminal: "OUT", Net: "SERVO_OUT"}}},
	}}

	node, ok := stabilityObservationNode(plan, "OUTPUT")
	if !ok || node != "BUCK_OUT" {
		t.Fatalf("stability node = %q, %v; want BUCK_OUT without traversing the series switch gate", node, ok)
	}
}

func TestSequenceResponseTargetUsesDependentPublicRail(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Objectives: []architecturesearch.Objective{
			{
				Capability: "rail_sequencing",
				Bindings:   []architecturesearch.Binding{{Role: "rail_a", Signal: "rail_a"}, {Role: "rail_b", Signal: "rail_b"}, {Role: "state", Signal: "sequence_state"}},
			},
			{
				Capability: "output_protection",
				Bindings:   []architecturesearch.Binding{{Role: "input", Signal: "rail_b"}, {Role: "output", Port: "output_b"}},
			},
		},
	}}
	target, ok := sequenceResponseTarget(requirement, []SemanticBinding{
		{Kind: "signal", ID: "rail_a", Target: "RAIL_A"},
		{Kind: "signal", ID: "rail_b", Target: "RAIL_B"},
		{Kind: "signal", ID: "sequence_state", Target: "SEQUENCE_STATE"},
		{Kind: "port", ID: "output_b", Target: "OUTPUT_B"},
	})
	if !ok || target != "OUTPUT_B" {
		t.Fatalf("sequence response target = %q, %v; want OUTPUT_B, true", target, ok)
	}
}

func TestGeneratedDomainControlBindingResolvesExternalControlSource(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "generated_logic", Kind: "supply", Source: "internal_rail"}},
		Ports: []architecturesearch.Port{{
			ID: "enable", Kind: "digital_logic", Direction: "sink", Domain: "generated_logic",
		}},
	}}
	plans := map[string]simmodel.Plan{simmodel.AnalysisTransient: {
		Devices: []simmodel.ResolvedDevice{{
			Component: "enable_source", PrimitiveModel: simmodel.PrimitiveConnectorVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "PIN_1", Net: "ENABLE"}, {Terminal: "PIN_2", Net: "GND"}},
		}},
	}}
	bindings := appendGeneratedDomainControlBindings(nil, requirement, []SemanticBinding{{Kind: "port", ID: "enable", Target: "ENABLE"}}, plans)
	if len(bindings) != 1 || bindings[0] != (SimulationOperatingBinding{
		Axis: "generated_domain_control", Target: "ENABLE", Kind: OperatingGeneratedControl, Component: "enable_source",
	}) {
		t.Fatalf("generated-domain control bindings = %#v", bindings)
	}

	requirement.Requirements.Domains[0].Source = "external"
	if got := appendGeneratedDomainControlBindings(nil, requirement, []SemanticBinding{{Kind: "port", ID: "enable", Target: "ENABLE"}}, plans); len(got) != 0 {
		t.Fatalf("external-domain control unexpectedly coupled to generated power: %#v", got)
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

func TestThermalComponentsTraversePassiveOutputNetworkToNearestActiveDevice(t *testing.T) {
	resistance := 10.0
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{
			Component: "output_program", Family: "resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &resistance,
			Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "BUFFER"}, {Terminal: "B", Net: "LOAD"}},
		},
		{
			Component: "current_regulator", Family: "current_regulator", PrimitiveModel: simmodel.PrimitiveProgrammableCurrentSourceV1,
			Terminals:       []simmodel.TerminalBinding{{Terminal: "OUT", Net: "BUFFER"}, {Terminal: "IN", Net: "SWITCHED"}},
			ModelParameters: []simmodel.NamedValue{{Name: "junction_to_ambient_c_per_w", Value: 24}},
		},
		{
			Component: "input_switch", Family: "mosfet", PrimitiveModel: simmodel.PrimitivePMOSSwitchV1,
			Terminals:       []simmodel.TerminalBinding{{Terminal: "DRAIN", Net: "SWITCHED"}, {Terminal: "SOURCE", Net: "SUPPLY"}},
			ModelParameters: []simmodel.NamedValue{{Name: "junction_to_ambient_c_per_w", Value: 125}},
		},
	}}

	if got := thermalComponentsForTarget(plan, "LOAD"); !slices.Equal(got, []string{"current_regulator"}) {
		t.Fatalf("thermal components = %#v, want nearest active current regulator", got)
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

func TestDynamicStressAndEfficiencyMetricsResolveToStructuredEvidence(t *testing.T) {
	load := OperatingHarnessComponentID("load_current", "OUT")
	plan := simmodel.Plan{Devices: []simmodel.ResolvedDevice{
		{
			Component: "supply", PrimitiveModel: simmodel.PrimitiveVoltageSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "VIN"}, {Terminal: "NEGATIVE", Net: "GND"}},
		},
		{
			Component: load, PrimitiveModel: simmodel.PrimitiveCurrentSourceV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "POSITIVE", Net: "OUT"}, {Terminal: "NEGATIVE", Net: "GND"}},
		},
	}}
	operating := []SimulationOperatingBinding{{Axis: "load_current", Target: "OUT", Kind: OperatingLoadCurrent, Component: load}}

	efficiency, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "efficiency", Metric: "conversion_efficiency", Target: "circuit"},
		"", []string{"VIN"}, operating, plan, architecturesearch.Requirement{}, nil,
	)
	if diagnostic != nil || len(efficiency.Prototypes) != 1 {
		t.Fatalf("conversion-efficiency binding = %#v diagnostic=%#v", efficiency, diagnostic)
	}
	efficiencyPrototype := efficiency.Prototypes[0]
	if efficiencyPrototype.Quantity != simmodel.QuantityConversionEfficiencyPct ||
		efficiencyPrototype.Component != load ||
		!slices.Equal(efficiencyPrototype.Components, []string{"supply"}) {
		t.Fatalf("conversion-efficiency prototype = %#v", efficiencyPrototype)
	}

	current, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "current", Metric: "peak_device_current", Target: "OUT"},
		"", nil, operating, plan, architecturesearch.Requirement{}, nil,
	)
	if diagnostic != nil || len(current.Prototypes) != 1 ||
		current.Prototypes[0].Quantity != simmodel.QuantityPeakAbsDeviceCurrentA ||
		current.Prototypes[0].Component != load {
		t.Fatalf("peak-current binding = %#v diagnostic=%#v", current, diagnostic)
	}
}

func TestProtectionResponseObservesResolvedInterlockControl(t *testing.T) {
	maximum := 1e-3
	analysisPlan := AnalysisPlan{
		Bindings: []SemanticBinding{
			{Kind: "port", ID: "load", Target: "SWITCHED_LOAD"},
			{Kind: "port", ID: "fault", Target: "FAULT_STATE"},
			{Kind: "signal", ID: "limited_drive", Target: "PROTECTION_CONTROL"},
		},
		Analyses: []PlannedAnalysis{{ID: "fault_transient", Kind: simmodel.AnalysisTransient, OperatingCase: "faulted"}},
		Events:   []PlannedEvent{{ID: "overload", OperatingCase: "faulted", Target: "SWITCHED_LOAD"}},
		Assertions: []PlannedAssertion{{
			RequirementID: "fault_response", AnalysisID: "fault_transient", OperatingCase: "faulted",
			Metric: "protection_response_time", Target: "event:overload", Max: &maximum, Unit: "s",
		}},
	}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisTransient: {
			Nodes:    []string{"SWITCHED_LOAD", "FAULT_STATE", "PROTECTION_CONTROL"},
			Analyses: []simmodel.Analysis{{ID: "transient", Kind: simmodel.AnalysisTransient, TimeStepS: 1e-5, DurationS: .01}},
		},
	}
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{Objectives: []architecturesearch.Objective{{
		ID: "limit", Capability: "safety_interlock", Bindings: []architecturesearch.Binding{
			{Role: "sense", Signal: "current_feedback", Direction: "sink"},
			{Role: "control", Signal: "limited_drive", Direction: "source"},
			{Role: "fault", Port: "fault", Direction: "source"},
		},
	}}}}

	_, assertions, _, diagnostics := BuildResolvedSimulationContracts(requirement, analysisPlan, plans)
	if len(diagnostics) != 0 || len(assertions) != 1 || len(assertions[0].Prototypes) != 1 {
		t.Fatalf("protection-response contracts = %#v diagnostics=%#v", assertions, diagnostics)
	}
	if prototype := assertions[0].Prototypes[0]; prototype.Node != "PROTECTION_CONTROL" || prototype.Quantity != simmodel.QuantityResponseTimeS {
		t.Fatalf("protection-response prototype = %#v", prototype)
	}
}

func TestProtectionResponseObservesAffectedProtectedPathWhenControlIsInternal(t *testing.T) {
	maximum := 1e-3
	analysisPlan := AnalysisPlan{
		Bindings: []SemanticBinding{
			{Kind: "signal", ID: "protected_drive", Target: "PROTECTED_DRIVE"},
			{Kind: "port", ID: "load", Target: "LOAD"},
		},
		Analyses: []PlannedAnalysis{{ID: "fault_transient", Kind: simmodel.AnalysisTransient, OperatingCase: "faulted"}},
		Events:   []PlannedEvent{{ID: "overload", OperatingCase: "faulted", Target: "LOAD"}},
		Assertions: []PlannedAssertion{{
			RequirementID: "fault_response", AnalysisID: "fault_transient", OperatingCase: "faulted",
			Metric: "protection_response_time", Target: "event:overload", Max: &maximum, Unit: "s",
		}},
	}
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{Objectives: []architecturesearch.Objective{{
		ID: "protect", Capability: "output_protection", Bindings: []architecturesearch.Binding{
			{Role: "input", Signal: "protected_drive", Direction: "sink"},
			{Role: "output", Port: "load", Direction: "source"},
		},
	}}}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisTransient: {
			Nodes:    []string{"PROTECTED_DRIVE", "LOAD"},
			Analyses: []simmodel.Analysis{{ID: "transient", Kind: simmodel.AnalysisTransient, TimeStepS: 1e-5, DurationS: .01}},
		},
	}

	_, assertions, _, diagnostics := BuildResolvedSimulationContracts(requirement, analysisPlan, plans)
	if len(diagnostics) != 0 || len(assertions) != 1 || len(assertions[0].Prototypes) != 1 {
		t.Fatalf("protection-response contracts = %#v diagnostics=%#v", assertions, diagnostics)
	}
	if prototype := assertions[0].Prototypes[0]; prototype.Node != "PROTECTED_DRIVE" || prototype.Quantity != simmodel.QuantityResponseTimeS {
		t.Fatalf("protection-response prototype = %#v", prototype)
	}
}

func TestProtectionResponseRejectsAmbiguousInterlockControls(t *testing.T) {
	maximum := 1e-3
	analysisPlan := AnalysisPlan{
		Bindings: []SemanticBinding{
			{Kind: "port", ID: "load", Target: "SWITCHED_LOAD"},
			{Kind: "signal", ID: "drive_a", Target: "CONTROL_A"},
			{Kind: "signal", ID: "drive_b", Target: "CONTROL_B"},
		},
		Analyses: []PlannedAnalysis{{ID: "fault_transient", Kind: simmodel.AnalysisTransient, OperatingCase: "faulted"}},
		Events:   []PlannedEvent{{ID: "overload", OperatingCase: "faulted", Target: "SWITCHED_LOAD"}},
		Assertions: []PlannedAssertion{{
			RequirementID: "fault_response", AnalysisID: "fault_transient", OperatingCase: "faulted",
			Metric: "protection_response_time", Target: "event:overload", Max: &maximum, Unit: "s",
		}},
	}
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{Objectives: []architecturesearch.Objective{
		{ID: "limit_a", Capability: "safety_interlock", Bindings: []architecturesearch.Binding{{Role: "control", Signal: "drive_a", Direction: "source"}}},
		{ID: "limit_b", Capability: "safety_interlock", Bindings: []architecturesearch.Binding{{Role: "control", Signal: "drive_b", Direction: "source"}}},
	}}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisTransient: {
			Nodes:    []string{"SWITCHED_LOAD", "CONTROL_A", "CONTROL_B"},
			Analyses: []simmodel.Analysis{{ID: "transient", Kind: simmodel.AnalysisTransient, TimeStepS: 1e-5, DurationS: .01}},
		},
	}

	_, _, _, diagnostics := BuildResolvedSimulationContracts(requirement, analysisPlan, plans)
	if len(diagnostics) != 1 || diagnostics[0].Message != "protection-response event requires exactly one resolved protection control or affected protected path" {
		t.Fatalf("ambiguous protection-response diagnostics = %#v", diagnostics)
	}
}

func TestProtectionResponseRejectsAmbiguousAffectedProtectedPaths(t *testing.T) {
	maximum := 1e-3
	analysisPlan := AnalysisPlan{
		Bindings: []SemanticBinding{
			{Kind: "signal", ID: "drive_a", Target: "DRIVE_A"},
			{Kind: "signal", ID: "drive_b", Target: "DRIVE_B"},
			{Kind: "port", ID: "load", Target: "LOAD"},
		},
		Analyses: []PlannedAnalysis{{ID: "fault_transient", Kind: simmodel.AnalysisTransient, OperatingCase: "faulted"}},
		Events:   []PlannedEvent{{ID: "overload", OperatingCase: "faulted", Target: "LOAD"}},
		Assertions: []PlannedAssertion{{
			RequirementID: "fault_response", AnalysisID: "fault_transient", OperatingCase: "faulted",
			Metric: "protection_response_time", Target: "event:overload", Max: &maximum, Unit: "s",
		}},
	}
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{Objectives: []architecturesearch.Objective{
		{ID: "protect_a", Capability: "output_protection", Bindings: []architecturesearch.Binding{
			{Role: "input", Signal: "drive_a", Direction: "sink"},
			{Role: "output", Port: "load", Direction: "source"},
		}},
		{ID: "protect_b", Capability: "output_protection", Bindings: []architecturesearch.Binding{
			{Role: "input", Signal: "drive_b", Direction: "sink"},
			{Role: "output", Port: "load", Direction: "source"},
		}},
	}}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisTransient: {
			Nodes:    []string{"DRIVE_A", "DRIVE_B", "LOAD"},
			Analyses: []simmodel.Analysis{{ID: "transient", Kind: simmodel.AnalysisTransient, TimeStepS: 1e-5, DurationS: .01}},
		},
	}

	_, _, _, diagnostics := BuildResolvedSimulationContracts(requirement, analysisPlan, plans)
	if len(diagnostics) != 1 || diagnostics[0].Message != "protection-response event requires exactly one resolved protection control or affected protected path" {
		t.Fatalf("ambiguous protection-response diagnostics = %#v", diagnostics)
	}
}

func TestEventDrivenMuteDoesNotRequireSeparateControlOverride(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{
			{ID: "audio", Kind: "supply"},
			{ID: "ground", Kind: "reference"},
		},
		Ports: []architecturesearch.Port{{ID: "output", Domain: "audio"}},
		OperatingCases: []architecturesearch.OperatingCase{{
			ID: "startup", Events: []architecturesearch.OperatingEvent{{
				ID: "startup", Target: architecturesearch.Observation{Kind: "port", ID: "output"},
			}},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "startup_mute", Observation: architecturesearch.Observation{Kind: "event", ID: "startup"},
			OperatingCases: []string{"startup"},
		}},
	}}
	binding, diagnostic := resolvedAssertionBinding(
		PlannedAssertion{RequirementID: "startup_mute", OperatingCase: "startup", Metric: "muted_output_voltage", Target: "OUT"},
		"", nil, nil, simmodel.Plan{Nodes: []string{"OUT", "GND"}}, requirement,
		[]SemanticBinding{{Kind: "domain", ID: "ground", Target: "GND"}},
	)
	if diagnostic != nil || len(binding.Prototypes) != 1 || len(binding.ExcitationOverrides) != 0 {
		t.Fatalf("event-driven mute binding = %#v diagnostic=%#v", binding, diagnostic)
	}
}

func TestEventDrivenMuteMeasuresSemanticProtectedOutput(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains: []architecturesearch.Domain{{ID: "ground", Kind: "reference"}, {ID: "audio", Kind: "supply"}},
		Ports:   []architecturesearch.Port{{ID: "output", Domain: "audio"}},
		Objectives: []architecturesearch.Objective{{
			Capability: "mute_control",
			Bindings:   []architecturesearch.Binding{{Role: "protected", Port: "output"}},
		}},
		OperatingCases: []architecturesearch.OperatingCase{{
			ID: "startup", Events: []architecturesearch.OperatingEvent{{
				ID: "startup", Target: architecturesearch.Observation{Kind: "circuit", ID: "circuit"},
			}},
		}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "mute", Observation: architecturesearch.Observation{Kind: "event", ID: "startup"},
			OperatingCases: []string{"startup"},
		}},
	}}
	maximum := .1
	analysisPlan := AnalysisPlan{
		Bindings: []SemanticBinding{
			{Kind: "port", ID: "output", Target: "AUDIO_OUT"},
			{Kind: "domain", ID: "ground", Target: "GND"},
		},
		Analyses: []PlannedAnalysis{{ID: "startup_transient", Kind: simmodel.AnalysisTransient, OperatingCase: "startup"}},
		Events:   []PlannedEvent{{ID: "startup", OperatingCase: "startup", Target: "circuit"}},
		Assertions: []PlannedAssertion{{
			RequirementID: "mute", AnalysisID: "startup_transient", OperatingCase: "startup",
			Metric: "muted_output_voltage", Target: "event:startup", Max: &maximum, Unit: "V",
		}},
	}
	plans := map[string]simmodel.Plan{simmodel.AnalysisTransient: {
		Nodes:    []string{"AUDIO_OUT", "GND"},
		Analyses: []simmodel.Analysis{{ID: "transient", Kind: simmodel.AnalysisTransient, TimeStepS: 1e-3, DurationS: 1}},
	}}

	_, assertions, _, diagnostics := BuildResolvedSimulationContracts(requirement, analysisPlan, plans)
	if len(diagnostics) != 0 || len(assertions) != 1 || len(assertions[0].Prototypes) != 1 ||
		assertions[0].Prototypes[0].Node != "AUDIO_OUT" {
		t.Fatalf("event-driven mute contracts = %#v diagnostics=%#v", assertions, diagnostics)
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

func TestCoolingModeUsesRegisteredWorstCaseExpansion(t *testing.T) {
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{{ID: "cooling", Assignments: []CornerAssignment{{Axis: "cooling_mode", Target: "circuit", Selection: "blocked_airflow"}}}}}
	diagnostics := []Diagnostic{}
	bindings := resolvedOperatingBindings(analysisPlan, map[string]simmodel.Plan{"thermal": {}}, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 || bindings[0].Kind != OperatingWorstCase {
		t.Fatalf("cooling-mode bindings = %#v diagnostics=%#v", bindings, diagnostics)
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
		simmodel.AnalysisDCOperatingPoint: {
			Devices:  []simmodel.ResolvedDevice{{Component: component, Family: "current_source", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1, Terminals: []simmodel.TerminalBinding{{Terminal: "NEGATIVE", Net: "LOAD"}}}},
			Analyses: []simmodel.Analysis{{Excitations: []simmodel.SourceExcitation{{Component: component, DCValue: maximum}, {Component: "supply", DCValue: 12}}}},
		},
		simmodel.AnalysisStartup: {
			Devices:  []simmodel.ResolvedDevice{{Component: component, Family: "resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &resistance, Terminals: []simmodel.TerminalBinding{{Terminal: "B", Net: "LOAD"}}}},
			Analyses: []simmodel.Analysis{{Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 12}}}},
		},
	}
	var diagnostics []Diagnostic
	bindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 || bindings[0].Kind != OperatingLoadCurrent || bindings[0].Component != component ||
		bindings[0].Scale != 12 || bindings[0].ReferenceComponent != "supply" {
		t.Fatalf("bindings = %#v diagnostics=%#v", bindings, diagnostics)
	}
}

func TestResolvedLoadCurrentBindingPreservesCatalogBackedParallelSupportLoad(t *testing.T) {
	semanticMaximum, physicalMaximum := 0.25, 0.24994
	resistance := 12 / physicalMaximum
	component := OperatingHarnessComponentID("load_current", "LOAD")
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{{ID: "maximum", Assignments: []CornerAssignment{{
		Axis: "load_current", Target: "LOAD", Value: &semanticMaximum,
	}}}}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisDCOperatingPoint: {
			Devices: []simmodel.ResolvedDevice{{
				Component: component, Family: "current_source", PrimitiveModel: simmodel.PrimitiveCurrentSourceV1,
			}},
			Analyses: []simmodel.Analysis{{Excitations: []simmodel.SourceExcitation{
				{Component: component, DCValue: physicalMaximum},
				{Component: "supply", DCValue: 12},
			}}},
		},
		simmodel.AnalysisStartup: {
			Devices: []simmodel.ResolvedDevice{{
				Component: component, Family: "resistor", PrimitiveModel: simmodel.PrimitiveResistorV1, ValueSI: &resistance,
			}},
			Analyses: []simmodel.Analysis{{Excitations: []simmodel.SourceExcitation{{Component: "supply", DCValue: 12}}}},
		},
	}
	var diagnostics []Diagnostic
	bindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 {
		t.Fatalf("bindings = %#v diagnostics=%#v", bindings, diagnostics)
	}
	if math.Abs(bindings[0].Offset-(physicalMaximum-semanticMaximum)) > 1e-15 ||
		math.Abs(bindings[0].Scale-12) > 1e-12 || bindings[0].ReferenceComponent != "supply" {
		t.Fatalf("catalog-backed load transfer = %#v", bindings[0])
	}
}

func TestResolvedLoadInductanceBindingUsesCatalogBackedHarness(t *testing.T) {
	value := 80e-3
	component := OperatingHarnessComponentID("load_inductance", "LOAD")
	analysisPlan := AnalysisPlan{Corners: []PlannedCorner{{ID: "inductive", Assignments: []CornerAssignment{{
		Axis: "load_inductance", Target: "LOAD", Value: &value,
	}}}}}
	plans := map[string]simmodel.Plan{
		simmodel.AnalysisElectrothermal: {Devices: []simmodel.ResolvedDevice{{
			Component: component, Family: "inductor", PrimitiveModel: simmodel.PrimitiveInductorTransientV1,
			Terminals: []simmodel.TerminalBinding{{Terminal: "A", Net: "SERIES_LOAD"}, {Terminal: "B", Net: "GND"}},
		}}},
	}
	var diagnostics []Diagnostic

	bindings := resolvedOperatingBindings(analysisPlan, plans, &diagnostics)
	if len(diagnostics) != 0 || len(bindings) != 1 ||
		bindings[0].Kind != OperatingDeviceValueSI || bindings[0].Component != component {
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

func TestSemanticSupplyNodesIncludeOnlyExternallySourcedRails(t *testing.T) {
	requirement := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{Domains: []architecturesearch.Domain{
		{ID: "input", Kind: "supply", Source: "external"},
		{ID: "regulated", Kind: "supply", Source: "input"},
		{ID: "ground", Kind: "reference", Source: "external"},
	}}}
	bindings := []SemanticBinding{
		{Kind: "domain", ID: "input", Target: "VIN"},
		{Kind: "domain", ID: "regulated", Target: "VOUT"},
		{Kind: "domain", ID: "ground", Target: "GND"},
	}

	if nodes := semanticSupplyNodes(requirement, bindings); !slices.Equal(nodes, []string{"VIN"}) {
		t.Fatalf("semantic supply nodes = %#v", nodes)
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
