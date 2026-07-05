package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestClassABOutputPairInstantiatesHeadphonePair(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "class_ab_output_pair",
		InstanceID: "pair",
		Params: map[string]any{
			"supply_voltage":             "9V",
			"load_impedance":             "32Ω",
			"upper_output_component_id":  "bjt.onsemi.mmbt3904.sot23",
			"lower_output_component_id":  "bjt.onsemi.mmbt3906.sot23",
			"emitter_resistor_value":     "0.47Ω",
			"emitter_resistor_footprint": "Resistor_SMD:R_1206_3216Metric",
			"output_footprint":           "Package_TO_SOT_SMD:SOT-23",
			"application":                "headphone",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 5 {
		t.Fatalf("refs = %#v, want output devices, emitter resistors, and load reference", output.Instance.Refs)
	}
	for _, net := range []string{"pair_bias_p", "pair_bias_n", "pair_amp_out", "pair_vcc", "pair_vee", "pair_load_ref"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	if peak, ok := output.Instance.Params["estimated_peak_current_a"].(float64); !ok || peak <= 0 {
		t.Fatalf("estimated peak current = %#v", output.Instance.Params["estimated_peak_current_a"])
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestClassABOutputPairBlocksSpeakerAndSOAOverflow(t *testing.T) {
	_, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "class_ab_output_pair",
		InstanceID: "bad_pair",
		Params: map[string]any{
			"supply_voltage":         "24V",
			"load_impedance":         "4Ω",
			"emitter_resistor_value": "0.22Ω",
			"application":            "speaker",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want speaker/SOA blockers", issues)
	}
}
