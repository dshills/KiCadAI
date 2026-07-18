package blocks

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestSelectDefinitionComponentsForLED(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition := ledIndicatorDefinition()
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if len(report.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", report.Issues)
	}
	if len(report.Selections) != 2 {
		t.Fatalf("expected two selections, got %+v", report.Selections)
	}
	roles := map[string]bool{}
	for _, selection := range report.Selections {
		roles[selection.Role] = true
	}
	if !roles["resistor"] || !roles["led"] {
		t.Fatalf("missing roles: %+v", roles)
	}
}

func TestSelectDefinitionComponentsForConnectorBreakout(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition, ok := NewBuiltinRegistry().GetBlock("connector_breakout")
	if !ok {
		t.Fatal("missing connector_breakout definition")
	}
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if len(report.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", report.Issues)
	}
	if len(report.Selections) != 1 {
		t.Fatalf("expected one connector selection, got %+v", report.Selections)
	}
	selection := report.Selections[0]
	if selection.Selection.Component.ID == "" {
		t.Fatalf("selection = %+v, want resolved connector catalog component", selection)
	}
	if selection.Role != "connector" || selection.Selection.Component.Family != "connector" {
		t.Fatalf("selection = %+v, want connector catalog selection", selection)
	}
}

func TestConnectorBreakoutSelectionRequestUsesPinCountParam(t *testing.T) {
	definition, ok := NewBuiltinRegistry().GetBlock("connector_breakout")
	if !ok {
		t.Fatal("missing connector_breakout definition")
	}
	component := definition.Components[0]
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{"pin_count": 4})
	if !ok {
		t.Fatal("expected connector selection request")
	}
	if request.Query.ValueKind != "pin_count" || request.Query.Value != "4" || request.Query.Package != "1x04" {
		t.Fatalf("selection query = %+v, want four-pin connector query", request.Query)
	}
}

func TestConnectorBreakoutSelectionRequestInfersPinCountFromPinNames(t *testing.T) {
	definition, ok := NewBuiltinRegistry().GetBlock("connector_breakout")
	if !ok {
		t.Fatal("missing connector_breakout definition")
	}
	component := definition.Components[0]
	params := map[string]any{"pin_names": []string{"VCC", "GND", "SDA", "SCL"}}
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, params)
	if !ok {
		t.Fatal("expected connector selection request")
	}
	if request.Query.ValueKind != "pin_count" || request.Query.Value != "4" || request.Query.Package != "1x04" {
		t.Fatalf("selection query = %+v, want inferred four-pin connector query", request.Query)
	}
	if _, ok := params["pin_count"]; ok {
		t.Fatalf("input params mutated: %+v", params)
	}
}

func TestConnectorBreakoutSelectionRequestPreservesPackageParam(t *testing.T) {
	definition, ok := NewBuiltinRegistry().GetBlock("connector_breakout")
	if !ok {
		t.Fatal("missing connector_breakout definition")
	}
	component := definition.Components[0]
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{"pin_count": 4, "connector_footprint": "Connector_PinHeader_2.54mm:PinHeader_2x02_P2.54mm_Vertical"})
	if !ok {
		t.Fatal("expected connector selection request")
	}
	if request.Query.Package != "pinheader_2x02_p2.54mm_vertical" {
		t.Fatalf("selection query = %+v, want explicit package query to win", request.Query)
	}
}

func TestI2CSensorSelectionRequestUsesConcreteComponentAndSupply(t *testing.T) {
	definition := i2cSensorDefinition()
	component := definition.Components[0]
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{
		"sensor_component_id": "sensor.sensirion.sht31_dis.dfn8",
		"supply_voltage":      "3.3V",
	})
	if !ok {
		t.Fatal("expected concrete sensor selection request")
	}
	if request.Query.Text != "sensor.sensirion.sht31_dis.dfn8" || !request.RequireConcrete || !request.RequireCompanions {
		t.Fatalf("request = %#v", request)
	}
	if got := request.RequiredFunctions; len(got) != 2 || got[0] != "SDA" || got[1] != "SCL" {
		t.Fatalf("required functions = %#v", got)
	}
	if len(request.RequiredRatings) != 1 || request.RequiredRatings[0].Kind != "supply_voltage" || request.RequiredRatings[0].Value != "3.3" {
		t.Fatalf("ratings = %#v", request.RequiredRatings)
	}
	selection, result := components.Select(context.Background(), loadBlockTestCatalog(t), request)
	if !result.OK || selection.Component.ID != "sensor.sensirion.sht31_dis.dfn8" {
		t.Fatalf("selection = %#v, issues = %#v", selection, result.Issues)
	}
}

func TestSelectionRequestUsesParamDrivenTolerance(t *testing.T) {
	component := BlockComponent{
		FootprintID:             "Resistor_SMD:R_0805_2012Metric",
		ComponentQuery:          &components.Query{Family: "resistor", ValueKind: "resistance", ToleranceKind: "resistance", ToleranceUnit: "%"},
		ComponentValueParam:     "resistance",
		ComponentToleranceParam: "tolerance_percent",
	}
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{
		"resistance":        "47kΩ",
		"tolerance_percent": 0.1,
	})
	if !ok || request.Query.Value != "47kΩ" || request.Query.MaximumTolerance != 0.1 || request.Query.Package != "0805" {
		t.Fatalf("request = %#v, ok = %v", request, ok)
	}
}

func TestExplicitActiveDeviceSelectionDoesNotRequireSensorBusFunctions(t *testing.T) {
	component := BlockComponent{ComponentIDParam: "transistor_component_id"}
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{
		"transistor_component_id": "transistor.bjt.onsemi.mmbt3904.sot23",
	})
	if !ok {
		t.Fatal("expected concrete active-device selection request")
	}
	if len(request.RequiredFunctions) != 0 {
		t.Fatalf("required functions = %#v, want none", request.RequiredFunctions)
	}
}

func TestI2CSensorSelectionRequestIsOptionalForGenericTemplate(t *testing.T) {
	definition := i2cSensorDefinition()
	component := definition.Components[0]
	if _, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, map[string]any{"supply_voltage": "3.3V"}); ok {
		t.Fatal("generic template should not claim concrete component evidence")
	}
}

func TestSelectDefinitionComponentsForVoltageRegulator(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition := voltageRegulatorDefinition()
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if reports.HasBlockingIssue(report.Issues) {
		t.Fatalf("unexpected blocking issues: %+v", report.Issues)
	}
	if len(report.Issues) == 0 {
		t.Fatal("expected regulator stability review warnings")
	}
	want := map[string]string{
		"regulator":        "regulator.linear.ams1117_3v3.sot223",
		"input_capacitor":  "capacitor.murata.grm21br61a106ke19l.0805",
		"output_capacitor": "capacitor.murata.grm21br61a106ke19l.0805",
	}
	if len(report.Selections) != len(want) {
		t.Fatalf("selections = %+v", report.Selections)
	}
	seen := map[string]bool{}
	for _, selection := range report.Selections {
		if wantID, ok := want[selection.Role]; !ok {
			t.Fatalf("unexpected role %s", selection.Role)
		} else if gotID := selection.Selection.Component.ID; gotID != wantID {
			t.Fatalf("role %s selected %s, want %s", selection.Role, gotID, wantID)
		}
		if seen[selection.Role] {
			t.Fatalf("duplicate role %s in selections: %+v", selection.Role, report.Selections)
		}
		seen[selection.Role] = true
	}
	for role := range want {
		if !seen[role] {
			t.Fatalf("missing role %s in selections: %+v", role, report.Selections)
		}
	}
}

func TestSelectionRequestForVoltageRegulatorUsesInstanceParams(t *testing.T) {
	definition := voltageRegulatorDefinition()
	params := ApplyParameterDefaults(definition, map[string]any{
		"output_voltage":      "5V",
		"input_capacitance":   "22uF",
		"output_capacitance":  "47uF",
		"capacitor_footprint": "Capacitor_SMD:C_0603_1608Metric",
		"include_power_led":   true,
	})
	componentsByRole := map[string]BlockComponent{}
	for _, component := range definition.Components {
		componentsByRole[component.Role] = component
	}

	regulatorRequest, ok := SelectionRequestForComponentWithParams(componentsByRole["regulator"], components.AcceptanceConnectivity, params)
	if !ok {
		t.Fatal("missing regulator selection request")
	}
	if regulatorRequest.Query.Value != "5V" || regulatorRequest.Query.Package != "sot223" {
		t.Fatalf("regulator query = %+v", regulatorRequest.Query)
	}
	inputRequest, ok := SelectionRequestForComponentWithParams(componentsByRole["input_capacitor"], components.AcceptanceConnectivity, params)
	if !ok {
		t.Fatal("missing input capacitor selection request")
	}
	if inputRequest.Query.Value != "22uF" || inputRequest.Query.Package != "0603" {
		t.Fatalf("input capacitor query = %+v", inputRequest.Query)
	}
	outputRequest, ok := SelectionRequestForComponentWithParams(componentsByRole["output_capacitor"], components.AcceptanceConnectivity, params)
	if !ok {
		t.Fatal("missing output capacitor selection request")
	}
	if outputRequest.Query.Value != "47uF" || outputRequest.Query.Package != "0603" {
		t.Fatalf("output capacitor query = %+v", outputRequest.Query)
	}
	if !ComponentActiveForParams(componentsByRole["power_led"], params) || !ComponentActiveForParams(componentsByRole["power_led_resistor"], params) {
		t.Fatalf("power LED roles should be active with include_power_led=true")
	}
}

func TestVoltageRegulatorComponentPinsFollowSelectedProfile(t *testing.T) {
	definition := voltageRegulatorDefinition()
	componentsByRole := map[string]BlockComponent{}
	for _, component := range definition.Components {
		componentsByRole[component.Role] = component
	}
	params := ApplyParameterDefaults(definition, map[string]any{
		"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
		"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
	})

	component := componentWithParamDrivenPins(componentsByRole["regulator"], params)
	if component.SymbolID != "Regulator_Linear:AP2112K-3.3" || component.FootprintID != "Package_TO_SOT_SMD:SOT-23-5" {
		t.Fatalf("component = %+v, want AP2112K profile metadata", component)
	}
	if len(component.Pins) != 5 {
		t.Fatalf("pins = %+v, want AP2112K five-pin profile", component.Pins)
	}
	assertPinOffset(t, pinSpecsByNumber(component.Pins), "5", 2.54, -2.54)
}

func TestVoltageRegulatorComponentPinsClearForUnsupportedProfile(t *testing.T) {
	definition := voltageRegulatorDefinition()
	componentsByRole := map[string]BlockComponent{}
	for _, component := range definition.Components {
		componentsByRole[component.Role] = component
	}
	params := ApplyParameterDefaults(definition, map[string]any{
		"regulator_symbol":    "Regulator_Linear:Unsupported",
		"regulator_footprint": "Package_TO_SOT_SMD:Unsupported",
	})

	component := componentWithParamDrivenPins(componentsByRole["regulator"], params)
	if len(component.Pins) != 0 {
		t.Fatalf("pins = %+v, want unsupported profile to clear default pins", component.Pins)
	}
}

func TestSelectionRequestUsesVoltageParamWithoutMutatingComponentQuery(t *testing.T) {
	definition := amplifierSupplyDecouplingDefinition()
	params := ApplyParameterDefaults(definition, map[string]any{
		"ceramic_capacitance":      "220nF",
		"capacitor_voltage_rating": "25V",
		"ceramic_footprint":        "Capacitor_SMD:C_0603_1608Metric",
	})
	componentsByRole := map[string]BlockComponent{}
	for _, component := range definition.Components {
		componentsByRole[component.Role] = component
	}
	component := componentsByRole["vcc_ceramic"]
	request, ok := SelectionRequestForComponentWithParams(component, components.AcceptanceConnectivity, params)
	if !ok {
		t.Fatal("missing capacitor selection request")
	}
	if request.Query.Value != "220nF" || request.Query.MinVoltageV != 25 || request.Query.Package != "0603" {
		t.Fatalf("query = %+v", request.Query)
	}
	if component.ComponentQuery.Value != "" || component.ComponentQuery.MinVoltageV != 0 {
		t.Fatalf("component query was mutated: %+v", component.ComponentQuery)
	}
}

func TestSelectDefinitionComponentsBlocksPlaceholderAtConnectivity(t *testing.T) {
	catalog := loadBlockTestCatalog(t)
	definition := opampGainStageDefinition()
	definition.Components[0].ComponentQuery = &components.Query{Family: "opamp"}
	definition.Components[0].Acceptance = components.AcceptanceConnectivity
	report := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceConnectivity)
	if len(report.Issues) == 0 {
		t.Fatal("expected placeholder opamp issue")
	}
}

func loadBlockTestCatalog(t *testing.T) *components.Catalog {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	catalogDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "data", "components")
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func pinSpecsByNumber(pins []transactions.PinSpec) map[string]transactions.PinSpec {
	out := make(map[string]transactions.PinSpec, len(pins))
	for _, pin := range pins {
		out[pin.Number] = pin
	}
	return out
}
