package circuitgraph

import (
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

func TestDerivedSynthesisACExcitesEachInputIndependently(t *testing.T) {
	sourceRecord := components.ComponentRecord{Family: "connector", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveConnectorVoltageSourceV1}}}
	resistorRecord := components.ComponentRecord{Family: "resistor", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}}
	selected := map[string]ResolvedComponent{
		"iface_power":   {Instance: Component{ID: "iface_power"}, ComponentID: "power", Record: sourceRecord},
		"iface_input_a": {Instance: Component{ID: "iface_input_a"}, ComponentID: "input_a", Record: sourceRecord},
		"iface_input_b": {Instance: Component{ID: "iface_input_b"}, ComponentID: "input_b", Record: sourceRecord},
		"load":          {Instance: Component{ID: "load", Value: "1k"}, ComponentID: "load", Record: resistorRecord},
	}
	document := Document{Nets: []Net{
		{Name: "GND", Role: NetRoleGround, Endpoints: []Endpoint{{Component: "iface_power", Selector: "PIN_1"}, {Component: "iface_input_a", Selector: "PIN_1"}, {Component: "iface_input_b", Selector: "PIN_1"}, {Component: "load", Selector: "B"}}},
		{Name: "VCC", Role: NetRolePower, Endpoints: []Endpoint{{Component: "iface_power", Selector: "PIN_2"}, {Component: "load", Selector: "A"}}},
		{Name: "INPUT_A", Role: NetRoleSignal, Endpoints: []Endpoint{{Component: "iface_input_a", Selector: "PIN_2"}}},
		{Name: "INPUT_B", Role: NetRoleSignal, Endpoints: []Endpoint{{Component: "iface_input_b", Selector: "PIN_2"}}},
	}}
	intent := FunctionIntent{
		Interfaces: []InterfaceRequirement{
			{ID: "power", Role: InterfacePowerInput, Signals: []InterfaceSignal{{Name: "GND", Role: NetRoleGround}, {Name: "VCC", Role: NetRolePower}}},
			{ID: "input_a", Role: InterfaceAnalogInput, Signals: []InterfaceSignal{{Name: "GND", Role: NetRoleGround}, {Name: "SIGNAL", Role: NetRoleSignal}}},
			{ID: "input_b", Role: InterfaceAnalogInput, Signals: []InterfaceSignal{{Name: "GND", Role: NetRoleGround}, {Name: "SIGNAL", Role: NetRoleSignal}}},
		},
		PowerDomains: []PowerDomainIntent{{Name: "VCC", Role: NetRolePower, VoltageV: 3.3}},
		Connections: []FunctionConnection{
			{Name: "VCC", VoltageDomain: "VCC", Endpoints: []FunctionalEndpoint{{Interface: "power", Signal: "VCC"}}},
			{Name: "INPUT_A", Endpoints: []FunctionalEndpoint{{Interface: "input_a", Signal: "SIGNAL"}}},
			{Name: "INPUT_B", Endpoints: []FunctionalEndpoint{{Interface: "input_b", Signal: "SIGNAL"}}},
		},
	}
	simulation, evidence := deriveSynthesisSimulation(document, intent, selected)
	if simulation == nil || evidence.Status != "derived" || len(simulation.Analyses) != 3 {
		t.Fatalf("derived simulation = %#v evidence=%#v", simulation, evidence)
	}
	for _, analysis := range simulation.Analyses {
		if analysis.Kind != simmodel.AnalysisACSweep {
			continue
		}
		active := 0
		for _, excitation := range analysis.Excitations {
			if excitation.ACMagnitude != 0 {
				active++
			}
		}
		if active != 1 {
			t.Fatalf("analysis %s has %d active AC inputs: %#v", analysis.ID, active, analysis.Excitations)
		}
	}
}

func TestSynthesisSourceInterfaceSupportsNegativePowerRail(t *testing.T) {
	signal, polarity, ok := synthesisSourceInterfaceSignal(InterfaceRequirement{
		Role:    InterfacePowerInput,
		Signals: []InterfaceSignal{{Name: "GND", Role: NetRoleGround}, {Name: "VEE", Role: NetRolePowerNeg}},
	})
	if !ok || signal != "VEE" || polarity != -1 {
		t.Fatalf("negative rail source = %q polarity=%v ok=%t", signal, polarity, ok)
	}
}

func TestSynthesisOperatingRailSupportsSignedSupplies(t *testing.T) {
	tests := []struct {
		name    string
		domains []PowerDomainIntent
		want    float64
	}{
		{name: "negative only", domains: []PowerDomainIntent{{Name: "VEE", Role: NetRolePowerNeg, VoltageV: -5}}, want: -5},
		{name: "largest excursion", domains: []PowerDomainIntent{{Name: "VDD", Role: NetRolePowerPos, VoltageV: 3.3}, {Name: "VEE", Role: NetRolePowerNeg, VoltageV: -12}}, want: -12},
		{name: "positive tie", domains: []PowerDomainIntent{{Name: "VEE", Role: NetRolePowerNeg, VoltageV: -5}, {Name: "VCC", Role: NetRolePowerPos, VoltageV: 5}}, want: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := synthesisOperatingRail(test.domains); got != test.want {
				t.Fatalf("operating rail = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSynthesisInterfaceOperatingRailUsesInterfaceDomain(t *testing.T) {
	domains := map[string]PowerDomainIntent{
		"negative_15v": {Name: "negative_15v", Role: NetRolePowerNeg, VoltageV: -15},
		"logic_3v3":    {Name: "logic_3v3", Role: NetRolePower, VoltageV: 3.3},
		"ground":       {Name: "ground", Role: NetRoleGround},
	}
	if got := synthesisInterfaceOperatingRail(domains, "logic_3v3", -15); got != 3.3 {
		t.Fatalf("logic-domain operating rail = %v, want 3.3", got)
	}
	if got := synthesisInterfaceOperatingRail(domains, "ground", -15); got != -15 {
		t.Fatalf("zero-volt domain fallback = %v, want -15", got)
	}
	if got := synthesisInterfaceOperatingRail(domains, "", -15); got != -15 {
		t.Fatalf("unbound-domain fallback = %v, want -15", got)
	}
}

func TestCatalogModelUncertaintiesRequireExplicitVariabilityEvidence(t *testing.T) {
	temperature := simmodel.Uncertainty{
		Target:  "model_parameters.junction_temperature_k",
		Source:  "reviewed:temperature",
		Nominal: 298.15,
		Minimum: 273.15,
		Maximum: 323.15,
	}
	models := []simmodel.CatalogEvidence{{
		ModelID:       simmodel.PrimitiveBJTNPNV1,
		Parameters:    []simmodel.NamedValue{{Name: "junction_temperature_k", Value: 298.15}},
		Uncertainties: []simmodel.Uncertainty{temperature},
	}}
	got := catalogModelUncertainties(models)
	if len(got) != 1 || got[0] != temperature {
		t.Fatalf("explicit model uncertainty = %#v", got)
	}

	models[0].Uncertainties = nil
	if got := catalogModelUncertainties(models); len(got) != 0 {
		t.Fatalf("catalog qualification parameters became stochastic uncertainties: %#v", got)
	}
}

func TestDerivedSynthesisTransientUsesCompleteBoundedOperatingCase(t *testing.T) {
	sourceRecord := components.ComponentRecord{Family: "connector", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveConnectorVoltageSourceV1}}}
	selected := map[string]ResolvedComponent{
		"iface_power": {Instance: Component{ID: "iface_power"}, ComponentID: "power", Record: sourceRecord},
		"iface_input": {Instance: Component{ID: "iface_input"}, ComponentID: "input", Record: sourceRecord},
		"resistor": {Instance: Component{ID: "resistor", Value: "1k"}, ComponentID: "resistor", Record: components.ComponentRecord{
			Family: "resistor", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}},
		}},
		"capacitor": {Instance: Component{ID: "capacitor", Value: "10n"}, ComponentID: "capacitor", Record: components.ComponentRecord{
			Family: "capacitor", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveCapacitorV1}, {ModelID: simmodel.PrimitiveCapacitorTransientV1}},
		}},
	}
	document := Document{Nets: []Net{
		{Name: "GND", Role: NetRoleGround, Endpoints: []Endpoint{{Component: "iface_power", Selector: "PIN_2"}, {Component: "iface_input", Selector: "PIN_2"}, {Component: "capacitor", Selector: "B"}}},
		{Name: "VCC", Role: NetRolePower, Endpoints: []Endpoint{{Component: "iface_power", Selector: "PIN_1"}}},
		{Name: "INPUT", Role: NetRoleSignal, Endpoints: []Endpoint{{Component: "iface_input", Selector: "PIN_1"}, {Component: "resistor", Selector: "A"}}},
		{Name: "OUTPUT", Role: NetRoleSignal, Endpoints: []Endpoint{{Component: "resistor", Selector: "B"}, {Component: "capacitor", Selector: "A"}}},
	}}
	parameters := []Parameter{
		synthesisNumberParameter("pulse_initial_value_v", 0), synthesisNumberParameter("pulse_value_v", 5),
		synthesisNumberParameter("pulse_delay_s", .0001), synthesisNumberParameter("pulse_width_s", .0005),
		synthesisNumberParameter("pulse_period_s", .001), synthesisNumberParameter("analysis_duration_s", .0008),
		synthesisNumberParameter("analysis_time_step_s", .00001),
	}
	intent := FunctionIntent{
		Functions: []FunctionRequirement{{ID: "filter", Parameters: parameters}},
		Interfaces: []InterfaceRequirement{
			{ID: "power", Role: InterfacePowerInput, Signals: []InterfaceSignal{{Name: "VCC", Role: NetRolePower}, {Name: "GND", Role: NetRoleGround}}},
			{ID: "input", Role: InterfaceDigitalIn, Signals: []InterfaceSignal{{Name: "SIGNAL", Role: NetRoleSignal}, {Name: "GND", Role: NetRoleGround}}},
		},
		PowerDomains: []PowerDomainIntent{{Name: "VCC", Role: NetRolePower, VoltageV: 5}},
		Connections: []FunctionConnection{
			{Name: "VCC", VoltageDomain: "VCC", Endpoints: []FunctionalEndpoint{{Interface: "power", Signal: "VCC"}}},
			{Name: "INPUT", Endpoints: []FunctionalEndpoint{{Interface: "input", Signal: "SIGNAL"}}},
		},
	}
	simulation, evidence := deriveSynthesisSimulation(document, intent, selected)
	if simulation == nil || evidence.Status != "derived" || simulation.ModelID != simmodel.ModelTransientCircuitV1 || len(simulation.Analyses) != 1 {
		t.Fatalf("derived transient = %#v evidence=%#v", simulation, evidence)
	}
	analysis := simulation.Analyses[0]
	if analysis.Kind != simmodel.AnalysisTransient || analysis.DurationS != .0008 || analysis.TimeStepS != .00001 {
		t.Fatalf("transient grid = %#v", analysis)
	}
	for _, excitation := range analysis.Excitations {
		if excitation.Component == "iface_input" && (excitation.PulseValue != 5 || excitation.PulsePeriodS != .001) {
			t.Fatalf("pulse excitation = %#v", excitation)
		}
	}

	intent.Functions[0].Parameters = parameters[:len(parameters)-1]
	if incomplete, incompleteEvidence := deriveSynthesisSimulation(document, intent, selected); incomplete != nil || incompleteEvidence.Reason != "incomplete_bounded_transient_operating_case" {
		t.Fatalf("incomplete transient = %#v evidence=%#v", incomplete, incompleteEvidence)
	}
}

func TestDerivedSynthesisTransientPolarityChangesSourceEquationNotNodeAssertion(t *testing.T) {
	condition := synthesisTransientCondition{pulseValueV: 5, widthS: .0005, periodS: .001, durationS: .0008, timeStepS: .00001}
	simulation, evidence := deriveSynthesisTransient(simmodel.ModelTransientCircuitV1, []synthesisSourceCondition{{
		component: "reversed_input", node: "INPUT", sourcePolarity: -1, pulseInput: true,
	}}, condition)
	if simulation == nil || evidence.Status != "derived" {
		t.Fatalf("derived reversed-source transient = %#v evidence=%#v", simulation, evidence)
	}
	if got := simulation.Analyses[0].Excitations[0].PulseValue; got != -5 {
		t.Fatalf("trusted source equation pulse = %v, want -5", got)
	}
	assertion := simulation.Assertions[0]
	if assertion.Node != "INPUT" || assertion.Min >= 5 || assertion.Max <= 5 {
		t.Fatalf("physical node assertion = %#v, want bounds around +5 V", assertion)
	}
}

func synthesisNumberParameter(name string, value float64) Parameter {
	return Parameter{Name: name, Value: ParameterValue{Number: &value}}
}
