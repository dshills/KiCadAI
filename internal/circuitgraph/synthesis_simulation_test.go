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
