package blocks

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
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
