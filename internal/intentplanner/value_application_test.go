package intentplanner

import (
	"testing"

	"kicadai/internal/blocks"
)

func TestValueApplicationRuleForSupportedBlocks(t *testing.T) {
	rule, ok := valueApplicationRuleFor("led_indicator", "led_resistor")
	if !ok {
		t.Fatalf("missing LED resistor application rule")
	}
	if rule.Param != "resistor_value" || rule.ResultKey != "resistance_ohms" {
		t.Fatalf("unexpected rule: %#v", rule)
	}
	if _, ok := valueApplicationRuleFor("opamp_gain_stage", "opamp_gain"); ok {
		t.Fatalf("opamp gain should not have a direct mutation rule")
	}
}

func TestBlockDefinitionSupportsParam(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	definition, ok := registry.GetBlock("crystal_oscillator")
	if !ok {
		t.Fatal("missing crystal block")
	}
	if !blockDefinitionSupportsParam(definition, "load_capacitor_value") {
		t.Fatalf("expected crystal load capacitor support")
	}
	if blockDefinitionSupportsParam(definition, "feedback_resistor_value") {
		t.Fatalf("unexpected unsupported param")
	}
}

func TestAppliedBlockValueHandlesMissingInstanceID(t *testing.T) {
	value := appliedBlockValue("", "resistor_value", "300", "ohm", "calculated")
	if value.Path != "blocks.params.resistor_value" {
		t.Fatalf("path = %q", value.Path)
	}
}

func TestValueLiteralFormatting(t *testing.T) {
	for _, tc := range []struct {
		ohms float64
		want string
	}{
		{ohms: 300, want: "300"},
		{ohms: 999.999, want: "1k"},
		{ohms: 4700, want: "4.7k"},
		{ohms: 999_999.9, want: "1M"},
		{ohms: 2_200_000, want: "2.2M"},
		{ohms: 260.5, want: "260.5"},
		{ohms: 0.001, want: "0.001"},
		{ohms: 0, want: "0"},
		{ohms: -1, want: "INVALID"},
	} {
		if got := formatResistanceLiteral(tc.ohms); got != tc.want {
			t.Errorf("formatResistanceLiteral(%v) = %q, want %q", tc.ohms, got, tc.want)
		}
	}
	if got := formatCapacitancePFLiteral(32); got != "32pF" {
		t.Fatalf("formatCapacitancePFLiteral = %q", got)
	}
	if got := formatCapacitancePFLiteral(0); got != "0pF" {
		t.Fatalf("formatCapacitancePFLiteral = %q", got)
	}
	if got := formatCapacitancePFLiteral(1000); got != "1nF" {
		t.Fatalf("formatCapacitancePFLiteral = %q", got)
	}
	if got := formatCapacitancePFLiteral(1_000_000); got != "1uF" {
		t.Fatalf("formatCapacitancePFLiteral = %q", got)
	}
	if got := formatCapacitancePFLiteral(-1); got != "INVALID" {
		t.Fatalf("formatCapacitancePFLiteral = %q", got)
	}
}
