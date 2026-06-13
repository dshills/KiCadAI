package blocks

import (
	"context"
	"math"
	"strings"
	"testing"

	"kicadai/internal/transactions"
)

func TestLEDIndicatorInstantiatesDeterministicOperations(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "led_indicator",
		InstanceID: "status",
		Params: map[string]any{
			"supply_voltage":      "3.3V",
			"led_forward_voltage": "2.0V",
			"led_current":         "5mA",
			"color":               "green",
			"active_high":         true,
			"resistor_footprint":  "Resistor_SMD:R_0805_2012Metric",
			"led_footprint":       "LED_SMD:LED_0805_2012Metric",
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if output.Instance.Params["resistor_value"] != "260" {
		t.Fatalf("resistor = %#v", output.Instance.Params["resistor_value"])
	}
	if len(output.Operations) != 9 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	if got := output.Instance.Refs; len(got) != 2 || !strings.HasPrefix(got[0], "R") || !strings.HasPrefix(got[1], "D") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 3 || got[0] != "status_in" || got[1] != "status_led_series" || got[2] != "status_gnd" {
		t.Fatalf("nets = %#v", got)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestLEDIndicatorActiveLowNetOrder(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "led_indicator",
		InstanceID: "status",
		Params: map[string]any{
			"active_high": false,
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 3 || got[0] != "status_vcc" || got[2] != "status_in" {
		t.Fatalf("nets = %#v", got)
	}
}

func TestLEDIndicatorResistorOverride(t *testing.T) {
	registry := NewBuiltinRegistry()
	for _, value := range []string{"100", "1kΩ", "1k", "10k", "4k7", "4R7"} {
		output, issues := registry.Instantiate(context.Background(), BlockRequest{
			BlockID:    "led_indicator",
			InstanceID: "status",
			Params: map[string]any{
				"resistor_value": value,
			},
		})
		if len(issues) != 0 {
			t.Fatalf("value %s issues = %#v", value, issues)
		}
		if output.Instance.Params["resistor_value"] != value {
			t.Fatalf("value %s params = %#v", value, output.Instance.Params)
		}
	}
}

func TestLEDIndicatorRejectsInvalidCurrentAndVoltage(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "led_indicator",
		InstanceID: "status",
		Params: map[string]any{
			"led_current":         "0mA",
			"supply_voltage":      "1.8V",
			"led_forward_voltage": "2.0V",
		},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	messages := issues[0].Message + " " + issues[1].Message
	if !strings.Contains(messages, "led_current must be positive") || !strings.Contains(messages, "below supply_voltage") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestLEDIndicatorRejectsEmptyFootprint(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "led_indicator",
		InstanceID: "status",
		Params: map[string]any{
			"led_footprint": "",
		},
	})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "led_footprint is required") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestParseResistanceUnits(t *testing.T) {
	for value, want := range map[string]float64{
		"10":   10,
		"10Ω":  10,
		"10uΩ": 0.00001,
		"10kΩ": 10000,
		"4k7":  4700,
		"4k7Ω": 4700,
		"4u7":  0.0000047,
		"4µ7":  0.0000047,
		"4R7":  4.7,
	} {
		got, ok := parseUnit(value, "Ω", resistanceMultipliers())
		if !ok || math.Abs(got-want) > 1e-12 {
			t.Fatalf("parseUnit(%q) = %v, %v; want %v, true", value, got, ok, want)
		}
	}
}

func TestParseBaseUnits(t *testing.T) {
	cases := []struct {
		value       string
		suffix      string
		multipliers []unitMultiplier
		want        float64
	}{
		{value: "5V", suffix: "V", multipliers: voltageMultipliers(), want: 5},
		{value: "250mA", suffix: "A", multipliers: currentMultipliers(), want: 0.25},
		{value: "10uF", suffix: "F", multipliers: capacitanceMultipliers(), want: 0.00001},
	}
	for _, tc := range cases {
		got, ok := parseUnit(tc.value, tc.suffix, tc.multipliers)
		if !ok || math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("parseUnit(%q) = %v, %v; want %v, true", tc.value, got, ok, tc.want)
		}
	}
}
