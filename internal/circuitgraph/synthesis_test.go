package circuitgraph

import (
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

func TestDeriveCompanionRecipeValueUsesRequestedOutputAndE96Series(t *testing.T) {
	output := "3.3V"
	recipe := components.CompanionPartRecipe{
		ValueKind: "resistance",
		ValueFormula: &components.CompanionValueFormula{
			Kind: "divider_upper_from_output_v1", Parameter: "output_voltage_v",
			ReferenceVoltageV: .8, LowerResistanceOhm: 10_000, PreferredSeries: "E96",
		},
	}
	value, issue := deriveCompanionRecipeValue(recipe, []Parameter{{Name: "output_voltage_v", Value: ParameterValue{String: &output}}})
	if issue != nil || value != "31.6k" {
		t.Fatalf("derived value=%q issue=%+v", value, issue)
	}

	reference := ".8"
	if value, issue := deriveCompanionRecipeValue(recipe, []Parameter{{Name: "output_voltage_v", Value: ParameterValue{String: &reference}}}); issue != nil || value != "0" {
		t.Fatalf("reference-voltage divider = %q issue=%#v, want 0-ohm upper link", value, issue)
	}
	unsupported := ".7"
	if _, issue := deriveCompanionRecipeValue(recipe, []Parameter{{Name: "output_voltage_v", Value: ParameterValue{String: &unsupported}}}); issue == nil {
		t.Fatal("expected output below the reference voltage to fail closed")
	}
}

func TestDeriveCompanionRecipeValueRejectsUnsupportedFormula(t *testing.T) {
	output := "3.3"
	recipe := components.CompanionPartRecipe{ValueFormula: &components.CompanionValueFormula{
		Kind: "unknown", Parameter: "output_voltage_v", ReferenceVoltageV: 0.8, LowerResistanceOhm: 10_000, PreferredSeries: "E96",
	}}
	if _, issue := deriveCompanionRecipeValue(recipe, []Parameter{{Name: "output_voltage_v", Value: ParameterValue{String: &output}}}); issue == nil {
		t.Fatal("expected unsupported formula to fail closed")
	}
}

func TestNearestE96ValueUsesAlternatingHalfEvenTieBreak(t *testing.T) {
	if got := nearestE96Value(101); got != 100 {
		t.Fatalf("first midpoint = %g, want even-ordinal 100", got)
	}
	if got := nearestE96Value(103.5); got != 105 {
		t.Fatalf("second midpoint = %g, want even-ordinal 105", got)
	}
}

func TestFormatResistanceValueUsesEngineeringMilliohms(t *testing.T) {
	if got := formatResistanceValue(.5); got != "500m" {
		t.Fatalf("sub-ohm resistance = %q, want 500m", got)
	}
}

func TestApplyRegulatorParameterConstraintsMergesOutputCurrent(t *testing.T) {
	maximum := "30mA"
	instance := Component{
		Role: RoleRegulator, Query: &ComponentQuery{Family: "regulator"},
		Parameters:      []Parameter{{Name: "maximum_output_current_ma", Value: ParameterValue{String: &maximum}}},
		RequiredRatings: []RequiredRating{{Kind: "output_current", Value: "0.02", Unit: "A"}},
	}
	applyRegulatorParameterConstraints(&instance)
	if len(instance.RequiredRatings) != 1 || instance.RequiredRatings[0].Value != "30mA" || instance.RequiredRatings[0].Unit != "mA" {
		t.Fatalf("merged output-current ratings = %#v", instance.RequiredRatings)
	}
	instance.RequiredRatings[0] = RequiredRating{Kind: "output_current", Value: "50", Unit: "mA"}
	applyRegulatorParameterConstraints(&instance)
	if len(instance.RequiredRatings) != 1 || instance.RequiredRatings[0].Value != "50" {
		t.Fatalf("more restrictive explicit rating was not preserved: %#v", instance.RequiredRatings)
	}
}

func TestConfiguredSimulationModelsOverlaysMatchingInstanceParameters(t *testing.T) {
	output := "3.3V"
	direction := -1.0
	selection := ResolvedComponent{
		Instance: Component{Parameters: []Parameter{
			{Name: "output_voltage_v", Value: ParameterValue{String: &output}},
			{Name: "direction", Value: ParameterValue{Number: &direction}},
		}},
		Record: components.ComponentRecord{SimulationModels: []simmodel.CatalogEvidence{{
			ModelID: "test_model", Parameters: []simmodel.NamedValue{
				{Name: "output_voltage_v", Value: .8},
				{Name: "direction", Value: 1},
				{Name: "unchanged", Value: 2},
			},
		}}},
	}
	models := configuredSimulationModels(selection)
	values := map[string]float64{}
	if len(models) == 1 {
		for _, parameter := range models[0].Parameters {
			values[parameter.Name] = parameter.Value
		}
	}
	if len(models) != 1 ||
		values["output_voltage_v"] != 3.3 ||
		values["direction"] != -1 ||
		values["unchanged"] != 2 {
		t.Fatalf("configured models = %#v", models)
	}
}

func TestConfiguredSimulationModelsMakesTypedOutputAndBidirectionalConnectorsPassive(t *testing.T) {
	record := components.ComponentRecord{
		Family: "connector",
		SimulationModels: []simmodel.CatalogEvidence{{
			ModelID: simmodel.PrimitiveConnectorVoltageSourceV1,
		}},
	}
	for _, role := range []ComponentRole{RoleOutputConnector, RoleConnector} {
		models := configuredSimulationModels(ResolvedComponent{
			Instance: Component{Role: role},
			Record:   record,
		})
		if len(models) != 0 {
			t.Fatalf("%s connector models = %#v, want passive boundary", role, models)
		}
	}
	models := configuredSimulationModels(ResolvedComponent{
		Instance: Component{Role: RoleInputConnector},
		Record:   record,
	})
	if len(models) != 1 || models[0].ModelID != simmodel.PrimitiveConnectorVoltageSourceV1 {
		t.Fatalf("input connector models = %#v, want catalog-backed source", models)
	}
}

func TestUniqueResolvedFunctionCollapsesRepeatedPhysicalPinsWithinOneUnit(t *testing.T) {
	functions := []ResolvedFunction{
		{Function: "GND", Unit: 1, UnitID: "A", SymbolPin: "A12", Pad: "A12"},
		{Function: "GND", Unit: 1, UnitID: "A", SymbolPin: "B12", Pad: "B12"},
	}
	function, ok := uniqueResolvedFunction(functions, "gnd")
	if !ok || function.Function != "GND" {
		t.Fatalf("function=%+v ok=%t", function, ok)
	}

	functions[1].UnitID = "B"
	functions[1].Unit = 2
	if _, ok := uniqueResolvedFunction(functions, "GND"); ok {
		t.Fatal("expected a repeated logical function across units to remain ambiguous")
	}
}

func TestConnectionHasInternalPowerOutputSuppressesRedundantExternalFlag(t *testing.T) {
	connection := FunctionConnection{Endpoints: []FunctionalEndpoint{
		{Interface: "external_return", Signal: "RETURN"},
		{Function: "converter", Port: "VOUT_MINUS"},
	}}
	selected := map[string]ResolvedComponent{
		"converter": {Functions: []ResolvedFunction{{Function: "VOUT_MINUS", Electrical: "power_out"}}},
	}
	if !connectionHasInternalPowerOutput(connection, selected) {
		t.Fatal("internal converter power output was not detected")
	}
	selected["converter"] = ResolvedComponent{Functions: []ResolvedFunction{{Function: "VIN_MINUS", Electrical: "power_in"}}}
	if connectionHasInternalPowerOutput(connection, selected) {
		t.Fatal("power input must not suppress an external-source flag")
	}
}

func TestPropagatePowerFlagsAcrossSeriesPowerLimiters(t *testing.T) {
	document := Document{
		Nets: []Net{
			{Name: "VIN", Role: NetRolePower, Endpoints: []Endpoint{
				{Component: "limiter", SelectorKind: SelectorFunction, Selector: "A"},
			}},
			{Name: "VIN_LIMITED", Role: NetRolePower, Endpoints: []Endpoint{
				{Component: "limiter", SelectorKind: SelectorFunction, Selector: "B"},
				{Component: "regulator", SelectorKind: SelectorFunction, Selector: "VIN"},
			}},
		},
		PowerFlags: []PowerFlag{{Net: "VIN"}},
	}
	selected := map[string]ResolvedComponent{
		"limiter": {
			Instance:  Component{Usage: "current_limit"},
			Functions: []ResolvedFunction{{Function: "A"}, {Function: "B"}},
		},
		"regulator": {
			Functions: []ResolvedFunction{{Function: "VIN", Electrical: "power_in"}},
		},
	}

	propagatePowerFlagsAcrossSeriesPowerLimiters(&document, selected, map[string]string{"limiter": "current_limit"})

	if len(document.PowerFlags) != 2 || document.PowerFlags[1].Net != "VIN_LIMITED" {
		t.Fatalf("power flags = %#v, want VIN followed by VIN_LIMITED", document.PowerFlags)
	}
}

func TestExternalPowerDomainFlagSearchSkipsEarlierInternalBranch(t *testing.T) {
	connections := []FunctionConnection{
		{Name: "LOGIC_INTERNAL", Role: NetRolePower, VoltageDomain: "logic", Endpoints: []FunctionalEndpoint{{Function: "bias"}, {Function: "load"}}},
		{Name: "LOGIC_IN", Role: NetRolePower, VoltageDomain: "logic", Endpoints: []FunctionalEndpoint{{Function: "connector"}, {Function: "load"}}},
	}
	net, connectionFound, sourceFound := externalPowerDomainFlagNet("logic", connections, nil, map[string]ComponentRole{"connector": RoleInputConnector})
	if net != "LOGIC_IN" || !connectionFound || !sourceFound {
		t.Fatalf("external domain flag selection = %q connection=%t source=%t", net, connectionFound, sourceFound)
	}
}

func TestEnsureExternalConnectorPowerFlagsCoversEveryPowerEntryNet(t *testing.T) {
	document := Document{
		Components: []Component{{ID: "input_a", Role: RoleInputConnector}, {ID: "input_b", Role: RoleInputConnector}, {ID: "load", Role: RoleIC}},
		Nets: []Net{
			{Name: "POWER_A", Role: NetRolePower, Endpoints: []Endpoint{{Component: "input_a"}, {Component: "load"}}},
			{Name: "POWER_B", Role: NetRolePower, Endpoints: []Endpoint{{Component: "input_b"}, {Component: "load"}}},
			{Name: "SIGNAL", Role: NetRoleSignal, Endpoints: []Endpoint{{Component: "input_b"}, {Component: "load"}}},
		},
		PowerFlags: []PowerFlag{{Net: "POWER_A"}},
	}
	ensureExternalConnectorPowerFlags(&document, nil)
	if len(document.PowerFlags) != 2 || document.PowerFlags[0].Net != "POWER_A" || document.PowerFlags[1].Net != "POWER_B" {
		t.Fatalf("external connector power flags = %#v", document.PowerFlags)
	}
}

func TestPropagatePowerFlagsFromInternalOutputAcrossTwoTerminalSeriesPath(t *testing.T) {
	document := Document{Nets: []Net{
		{Name: "SW", Role: NetRoleSignal, Endpoints: []Endpoint{
			{Component: "converter", SelectorKind: SelectorFunction, Selector: "SW"},
			{Component: "inductor", SelectorKind: SelectorFunction, Selector: "A"},
		}},
		{Name: "VOUT", Role: NetRolePower, Endpoints: []Endpoint{
			{Component: "inductor", SelectorKind: SelectorFunction, Selector: "B"},
			{Component: "shunt", SelectorKind: SelectorFunction, Selector: "A"},
			{Component: "regulator_bias", SelectorKind: SelectorFunction, Selector: "VIN"},
		}},
		{Name: "VOUT_SENSED", Role: NetRolePower, Endpoints: []Endpoint{
			{Component: "shunt", SelectorKind: SelectorFunction, Selector: "B"},
			{Component: "monitor", SelectorKind: SelectorFunction, Selector: "VCC"},
		}},
	}}
	selected := map[string]ResolvedComponent{
		"converter":      {Functions: []ResolvedFunction{{Function: "SW", Electrical: "power_out"}}},
		"inductor":       {Functions: []ResolvedFunction{{Function: "A"}, {Function: "B"}}},
		"shunt":          {Functions: []ResolvedFunction{{Function: "A"}, {Function: "B"}}},
		"regulator_bias": {Functions: []ResolvedFunction{{Function: "VIN", Electrical: "power_in"}}},
		"monitor":        {Functions: []ResolvedFunction{{Function: "VCC", Electrical: "power_in"}}},
	}

	propagatePowerFlagsAcrossSeriesPowerLimiters(&document, selected, nil)

	if len(document.PowerFlags) != 2 || document.PowerFlags[0].Net != "VOUT" || document.PowerFlags[1].Net != "VOUT_SENSED" {
		t.Fatalf("power flags = %#v, want internally driven VOUT path in deterministic order", document.PowerFlags)
	}
}

func TestSeriesPowerLimiterPropagationRejectsSignalAndMultiPowerNetPaths(t *testing.T) {
	document := Document{
		Nets: []Net{
			{Name: "VIN", Role: NetRolePower, Endpoints: []Endpoint{
				{Component: "signal_limiter", SelectorKind: SelectorFunction, Selector: "A"},
				{Component: "multi_limiter", SelectorKind: SelectorFunction, Selector: "A"},
			}},
			{Name: "SIGNAL", Role: NetRoleSignal, Endpoints: []Endpoint{
				{Component: "signal_limiter", SelectorKind: SelectorFunction, Selector: "B"},
			}},
			{Name: "RAIL_A", Role: NetRolePower, Endpoints: []Endpoint{
				{Component: "multi_limiter", SelectorKind: SelectorFunction, Selector: "B"},
			}},
			{Name: "RAIL_B", Role: NetRolePower, Endpoints: []Endpoint{
				{Component: "multi_limiter", SelectorKind: SelectorFunction, Selector: "C"},
			}},
		},
		PowerFlags: []PowerFlag{{Net: "VIN"}},
	}
	selected := map[string]ResolvedComponent{
		"signal_limiter": {Instance: Component{Usage: "current_limit"}},
		"multi_limiter":  {Instance: Component{Usage: "current_limit"}},
	}

	propagatePowerFlagsAcrossSeriesPowerLimiters(&document, selected, map[string]string{
		"signal_limiter": "current_limit",
		"multi_limiter":  "current_limit",
	})

	if len(document.PowerFlags) != 1 {
		t.Fatalf("power flags = %#v, want no propagation", document.PowerFlags)
	}
}
