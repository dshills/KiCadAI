package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestAmplifierInputBufferInstantiatesPassiveCoupledBiasNetwork(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_input_buffer",
		InstanceID: "input",
		Params: map[string]any{
			"input_impedance":      "47k",
			"coupling_capacitance": "2.2uF",
			"input_stopper_value":  "100",
			"resistor_footprint":   "Resistor_SMD:R_0805_2012Metric",
			"capacitor_footprint":  "Capacitor_SMD:C_0805_2012Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 4 {
		t.Fatalf("refs = %#v, want stopper, coupling cap, and bias divider", output.Instance.Refs)
	}
	for _, net := range []string{"input_in", "input_pre_coupling", "input_out", "input_vcc", "input_gnd"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	if cutoff, ok := output.Instance.Params["high_pass_cutoff_hz"].(float64); !ok || cutoff <= 0 {
		t.Fatalf("high pass cutoff = %#v, want positive calculation", output.Instance.Params["high_pass_cutoff_hz"])
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestAmplifierInputBufferRejectsInvalidValues(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_input_buffer",
		InstanceID: "bad",
		Params: map[string]any{
			"input_impedance":      "0",
			"coupling_capacitance": "not-a-cap",
			"input_stopper_value":  "-1Ω",
		},
	})
	if len(issues) < 3 {
		t.Fatalf("issues = %#v, want invalid impedance, capacitance, and stopper", issues)
	}
}

func TestAmplifierInputBufferInventoryIsImplementedButNotFabricationReady(t *testing.T) {
	family, ok := inventoryFamily(NewBuiltinRegistry().Inventory(), "amplifier_input_buffer")
	if !ok {
		t.Fatal("missing amplifier_input_buffer inventory")
	}
	if !family.Implemented || family.Readiness != BlockReadinessPartial {
		t.Fatalf("family = %#v, want implemented partial readiness", family)
	}
	if !slices.Contains(family.ExportedPorts, "IN") || !slices.Contains(family.ExportedPorts, "OUT") {
		t.Fatalf("ports = %#v", family.ExportedPorts)
	}
	if family.VerificationLevel.AllowsFabricationReadinessClaim() {
		t.Fatalf("verification level = %s, did not expect fabrication readiness", family.VerificationLevel)
	}
}
