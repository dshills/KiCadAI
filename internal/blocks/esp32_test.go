package blocks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestESP32WROOM32EMinimalInstantiatesReviewedSystem(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "esp32_wroom_32e_minimal", InstanceID: "controller"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 12 {
		t.Fatalf("refs = %#v", output.Instance.Refs)
	}
	if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
	encoded, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{
		`"library_id":"RF_Module:ESP32-WROOM-32E"`,
		`"footprint_id":"RF_Module:ESP32-WROOM-32E"`,
		`"net_name":"controller_enable"`,
		`"net_name":"controller_boot"`,
		`"net_name":"controller_uart_tx"`,
		`"net_name":"controller_uart_rx"`,
		`"library_id":"Connector_Generic:Conn_01x06"`,
		`"library_id":"power:PWR_FLAG"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("operations missing %q", want)
		}
	}
}

func TestESP32WROOM32EMinimalRejectsSupplyOutsideReviewedRange(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID: "esp32_wroom_32e_minimal", InstanceID: "controller", Params: map[string]any{"supply_voltage": "5V"},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.supply_voltage" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestESP32WROOM32EMinimalCatalogAndPCBRealization(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("esp32_wroom_32e_minimal")
	if !ok {
		t.Fatal("missing ESP32 block")
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	selection := SelectDefinitionComponents(context.Background(), definition, catalog, components.AcceptanceERCDRC)
	if reports.HasBlockingIssue(selection.Issues) {
		t.Fatalf("selection issues = %#v", selection.Issues)
	}
	var selected BlockComponentSelection
	for _, candidate := range selection.Selections {
		if candidate.Role == "module" {
			selected = candidate
		}
	}
	if selected.Selection.Component.ID != esp32WROOM32EComponentID || selected.Selection.Variant.FootprintID != esp32WROOM32EFootprint {
		t.Fatalf("module selection = %#v", selected)
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: definition.ID, InstanceID: "controller"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{OriginXMM: 50, OriginYMM: 35})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realization issues = %#v", realized.Issues)
	}
	if len(realized.LocalRoutes) != len(definition.PCBRealization.Validation.RequiredRoutes) {
		t.Fatalf("local routes = %d, required = %d", len(realized.LocalRoutes), len(definition.PCBRealization.Validation.RequiredRoutes))
	}
	if len(definition.PCBRealization.Keepouts) != 1 || definition.PCBRealization.Keepouts[0].ID != "esp32_pcb_antenna_exclusion" || definition.PCBRealization.Keepouts[0].BlocksRoute == nil || !*definition.PCBRealization.Keepouts[0].BlocksRoute {
		t.Fatalf("antenna keepout = %#v", definition.PCBRealization.Keepouts)
	}
}
