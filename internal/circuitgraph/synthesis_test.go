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
	selection := ResolvedComponent{
		Instance: Component{Parameters: []Parameter{{Name: "output_voltage_v", Value: ParameterValue{String: &output}}}},
		Record: components.ComponentRecord{SimulationModels: []simmodel.CatalogEvidence{{
			ModelID: "test_model", Parameters: []simmodel.NamedValue{{Name: "output_voltage_v", Value: .8}, {Name: "unchanged", Value: 2}},
		}}},
	}
	models := configuredSimulationModels(selection)
	if len(models) != 1 || models[0].Parameters[0].Value != 3.3 || models[0].Parameters[1].Value != 2 {
		t.Fatalf("configured models = %#v", models)
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
