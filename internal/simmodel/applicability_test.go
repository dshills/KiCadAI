package simmodel

import "testing"

func TestApplicableGraphModelRequiresCompleteTrustedTopology(t *testing.T) {
	linear := []ComponentEvidence{
		{InstanceID: "supply", Family: "connector", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveConnectorVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "PIN_1", Net: "GND"}, {Function: "PIN_2", Net: "VCC"}}},
		{InstanceID: "resistor", Family: "resistor", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}, {ModelID: ModelRCLowpassACV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}}},
	}
	if model, ok, _ := ApplicableGraphModel(linear); !ok || model != ModelLinearCircuitMNAV1 {
		t.Fatalf("linear applicability = %q, %t", model, ok)
	}

	nonlinear := append([]ComponentEvidence(nil), linear...)
	nonlinear = append(nonlinear, ComponentEvidence{InstanceID: "indicator", Family: "led", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "VCC"}}})
	if model, ok, _ := ApplicableGraphModel(nonlinear); !ok || model != ModelNonlinearCircuitDCV1 {
		t.Fatalf("nonlinear applicability = %q, %t", model, ok)
	}
	mixed := append([]ComponentEvidence(nil), nonlinear...)
	mixed = append(mixed, ComponentEvidence{InstanceID: "amplifier", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1}}, Connections: []ConnectionEvidence{{Function: "OUT", Net: "VCC"}}})
	if model, ok, _ := ApplicableGraphModel(mixed); !ok || model != ModelNonlinearCircuitDCV1 {
		t.Fatalf("mixed op-amp/nonlinear applicability = %q, %t", model, ok)
	}

	incomplete := append([]ComponentEvidence(nil), linear...)
	incomplete = append(incomplete, ComponentEvidence{InstanceID: "controller", Family: "mcu", Connections: []ConnectionEvidence{{Function: "VCC", Net: "VCC"}}})
	if model, ok, _ := ApplicableGraphModel(incomplete); ok || model != "" {
		t.Fatalf("incomplete applicability = %q, %t", model, ok)
	}
}

func TestApplicableGraphModelForTransientRequiresTransientCapacitorEvidence(t *testing.T) {
	base := []ComponentEvidence{
		{InstanceID: "supply", Family: "connector", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveConnectorVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "PIN_1", Net: "GND"}, {Function: "PIN_2", Net: "VCC"}}},
		{InstanceID: "resistor", Family: "resistor", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}}},
	}
	transient := append([]ComponentEvidence(nil), base...)
	transient = append(transient, ComponentEvidence{InstanceID: "capacitor", Family: "capacitor", ModelClaims: []CatalogEvidence{
		{ModelID: PrimitiveCapacitorV1}, {ModelID: PrimitiveCapacitorTransientV1},
	}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}}})
	if model, ok, _ := ApplicableGraphModelForAnalysis(transient, AnalysisTransient); !ok || model != ModelTransientCircuitV1 {
		t.Fatalf("transient applicability = %q, %t", model, ok)
	}

	dcOnly := append([]ComponentEvidence(nil), base...)
	dcOnly = append(dcOnly, ComponentEvidence{InstanceID: "capacitor", Family: "capacitor", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}}})
	if model, ok, _ := ApplicableGraphModelForAnalysis(dcOnly, AnalysisTransient); ok || model != "" {
		t.Fatalf("DC-only capacitor transient applicability = %q, %t", model, ok)
	}
}

func TestBJTPolarityPrimitivesShareCatalogFamily(t *testing.T) {
	for _, primitive := range []string{PrimitiveBJTNPNV1, PrimitiveBJTPNPV1} {
		if diagnostics := ValidateCatalogEvidence("bjt", []CatalogEvidence{{ModelID: primitive, Parameters: bjtParameters(.2, 40)}}); len(diagnostics) != 0 {
			t.Fatalf("%s bjt-family diagnostics = %#v", primitive, diagnostics)
		}
	}
}

func TestNonlinearDCCombinesOpAmpAndDiodeStamps(t *testing.T) {
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 5.5}, {Name: "supply_min_v", Value: 2.7},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
		{InstanceID: "amplifier", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VCC"}, {Function: "V_MINUS", Net: "GND"}}},
		resistorEvidence("limit", 1000, "OUT", "LOAD"),
		{InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.02, 5)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "LOAD"}, {Function: "CATHODE", Net: "GND"}}},
	}
	intent := Intent{
		ModelID:    ModelNonlinearCircuitDCV1,
		Analyses:   []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "signal", DCValue: 1}, {Component: "supply", DCValue: 5}}}},
		Assertions: []Assertion{{AnalysisID: "dc", Node: "OUT", Quantity: QuantityVoltageV, Min: .99, Max: 1.01}},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "LOAD"}, {Name: "OUT"}, {Name: "VCC", Role: "power"}}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("mixed nonlinear resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("mixed nonlinear evaluation = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestConnectorVoltageSourceUsesCanonicalNormalizedPinMapping(t *testing.T) {
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "connector.pinheader.1x02.2_54mm", Family: "connector", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveConnectorVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "PIN_1", Net: "GND"}, {Function: "PIN_2", Net: "VCC"}}},
		{InstanceID: "load", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 1000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}, {Function: "B", Net: "GND"}}},
	}
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: -5}}}},
		Assertions: []Assertion{{AnalysisID: "dc", Node: "VCC", Quantity: QuantityVoltageV, Min: 4.999, Max: 5.001}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"}})
	if len(diagnostics) != 0 {
		t.Fatalf("connector source resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("connector source evaluation = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestLEDMayUseTrustedDiodePrimitive(t *testing.T) {
	evidence := []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.02, 5)}}
	if diagnostics := ValidateCatalogEvidence("led", evidence); len(diagnostics) != 0 {
		t.Fatalf("LED diode evidence diagnostics = %#v", diagnostics)
	}
}
