package blocks

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/transactions"
)

func TestConnectorBreakoutInstantiatesTwoPinConnector(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "power",
		Params: map[string]any{
			"pin_names": []any{"VCC", "GND"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 1 || !strings.HasPrefix(got[0], "J") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 2 || got[0] != "power_VCC" || got[1] != "power_GND" {
		t.Fatalf("nets = %#v", got)
	}
	if len(output.Instance.Ports) != 2 || output.Instance.Ports[0].Name != "VCC" || output.Instance.Ports[1].Name != "GND" {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
	if len(output.Operations) != 5 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("validation issues = %#v", validation.Issues)
	}
}

func TestConnectorBreakoutInstantiatesFourPinConnector(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "i2c",
		Params: map[string]any{
			"pin_names": []string{"VCC", "GND", "SDA", "SCL"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if output.Instance.Params["connector_symbol"] != "Connector_Generic:Conn_01x04" {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
	if output.Instance.Params["connector_footprint"] != "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical" {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
	if len(output.Instance.Ports) != 4 || output.Instance.Ports[3].Name != "SCL" {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
}

func TestConnectorBreakoutPreservesExplicitDefaultSymbol(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "i2c",
		Params: map[string]any{
			"pin_names":           []string{"VCC", "GND", "SDA", "SCL"},
			"connector_symbol":    defaultConnectorSymbol,
			"connector_footprint": defaultConnectorFootprint,
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if output.Instance.Params["connector_symbol"] != defaultConnectorSymbol {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
	if output.Instance.Params["connector_footprint"] != defaultConnectorFootprint {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
}

func TestConnectorBreakoutRejectsPinCountMismatch(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "bad",
		Params: map[string]any{
			"pin_names": []string{"A", "B"},
			"pin_count": 3,
		},
	})
	if len(issues) != 1 || issues[0].Path != "params.pin_count" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestConnectorBreakoutRejectsDuplicatePinNames(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "bad",
		Params: map[string]any{
			"pin_names": []string{"A", "A"},
		},
	})
	if len(issues) != 1 || issues[0].Path != "params.pin_names" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestConnectorBreakoutRejectsMountingHolesUntilPCBSupport(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "bad",
		Params: map[string]any{
			"pin_names":              []string{"A", "B"},
			"include_mounting_holes": true,
		},
	})
	if len(issues) != 1 || issues[0].Path != "params.include_mounting_holes" {
		t.Fatalf("issues = %#v", issues)
	}
}
