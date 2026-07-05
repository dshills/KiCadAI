package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestAmplifierSupplyDecouplingInstantiatesSingleSupply(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "decouple",
		Params: map[string]any{
			"rail_mode":                "single_supply",
			"rail_voltage":             "9V",
			"ceramic_capacitance":      "100nF",
			"bulk_capacitance":         "10uF",
			"include_bulk":             true,
			"capacitor_voltage_rating": "16V",
			"ceramic_footprint":        "Capacitor_SMD:C_0805_2012Metric",
			"bulk_footprint":           "Capacitor_SMD:C_1210_3225Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 2 {
		t.Fatalf("refs = %#v, want ceramic and bulk VCC decoupling", output.Instance.Refs)
	}
	for _, net := range []string{"decouple_vcc", "decouple_gnd"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestAmplifierSupplyDecouplingInstantiatesDualSupply(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "dual",
		Params: map[string]any{
			"rail_mode":                "dual_supply",
			"rail_voltage":             "12V",
			"capacitor_voltage_rating": "25V",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if !slices.Contains(output.Instance.Nets, "dual_vee") {
		t.Fatalf("nets = %#v, want VEE decoupling", output.Instance.Nets)
	}
}

func TestAmplifierSupplyDecouplingBlocksUnderratedCapacitors(t *testing.T) {
	_, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_supply_decoupling",
		InstanceID: "bad_decouple",
		Params: map[string]any{
			"rail_voltage":             "12V",
			"capacitor_voltage_rating": "16V",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want capacitor derating blocker", issues)
	}
}
