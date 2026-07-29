package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestAmplifierBiasNetworkInstantiatesDiodeString(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_bias_network",
		InstanceID: "bias",
		Params: map[string]any{
			"topology":                     "diode_string",
			"application":                  "headphone",
			"diode_count":                  2.0,
			"emitter_resistor_value":       "0.47Ω",
			"bias_feed_resistor_value":     "10kΩ",
			"target_quiescent_current":     "review_required",
			"thermal_coupling_policy":      "adjacent_to_output_pair",
			"bias_diode_footprint":         "Diode_SMD:D_SOD-123",
			"bias_feed_resistor_footprint": "Resistor_SMD:R_0805_2012Metric",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 5 {
		t.Fatalf("refs = %#v, want bias feeds, two diodes, and output anchor", output.Instance.Refs)
	}
	for _, net := range []string{"bias_bias_p", "bias_driver_out", "bias_bias_n", "bias_vcc", "bias_vee", "bias_amp_out"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
	definition, ok := registry.GetBlock("amplifier_bias_network")
	if !ok {
		t.Fatal("missing amplifier_bias_network definition")
	}
	topology := projectBlockTopology(t, definition, "bias", output.Instance.Params, output.Operations)
	upper := InstanceNetName("bias", "bias_p")
	driver := InstanceNetName("bias", "driver_out")
	lower := InstanceNetName("bias", "bias_n")
	topology.requirePortNet(t, "BIAS_P", upper)
	topology.requirePortNet(t, "DRIVER_OUT", driver)
	topology.requirePortNet(t, "BIAS_N", lower)
	topology.requirePortNet(t, "VCC", InstanceNetName("bias", "vcc"))
	topology.requirePortNet(t, "VEE", InstanceNetName("bias", "vee"))
	topology.requirePortNet(t, "AMP_OUT", InstanceNetName("bias", "amp_out"))
	topology.requireFunctionNet(t, "bias_upper", "ANODE", upper)
	topology.requireFunctionNet(t, "bias_upper", "CATHODE", driver)
	topology.requireFunctionNet(t, "bias_lower", "ANODE", driver)
	topology.requireFunctionNet(t, "bias_lower", "CATHODE", lower)
}

func TestAmplifierBiasNetworkBlocksUnsupportedVariants(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "amplifier_bias_network",
		InstanceID: "bad_bias",
		Params: map[string]any{
			"topology":                 "vbe_multiplier",
			"application":              "speaker",
			"diode_count":              3.0,
			"target_quiescent_current": "25mA",
			"thermal_coupling_policy":  "unconstrained",
		},
	})
	if len(issues) < 5 {
		t.Fatalf("issues = %#v, want unsupported topology, application, diode count, quiescent current, and thermal policy", issues)
	}
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want at least one blocking issue", issues)
	}
}
