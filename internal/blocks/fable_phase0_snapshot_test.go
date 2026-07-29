package blocks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"kicadai/internal/reports"
)

func TestFablePhase0NormalizedBlockOperationSnapshots(t *testing.T) {
	// These digests are audit receipts, not compatibility goldens. Any
	// serialized operation change must be reviewed and update the Phase 0
	// transaction ledger deliberately.
	cases := []struct {
		name     string
		request  BlockRequest
		expected string
	}{
		{
			name: "class_ab_output_stage",
			request: BlockRequest{
				BlockID:    "class_ab_output_stage",
				InstanceID: "hp_out",
				Params: map[string]any{
					"supply_voltage":            "9V",
					"load_impedance":            "32Ω",
					"upper_output_component_id": "bjt.onsemi.mmbt3904.sot23",
					"lower_output_component_id": "bjt.onsemi.mmbt3906.sot23",
				},
			},
			expected: "d824d0412999b11a6934ecaf604a27c5e84b6baf8be99265890d7296fb57c95a",
		},
		{
			name: "opamp_gain_stage_ac_coupled",
			request: BlockRequest{
				BlockID:    "opamp_gain_stage",
				InstanceID: "amp",
				Params: map[string]any{
					"gain":           2.0,
					"input_coupling": "ac",
				},
			},
			expected: "4fe32b2af1268bf9974e48bbccc3a7fdb790d83217c96f3f6fe83ab4b16d85e2",
		},
		{
			name: "amplifier_bias_network",
			request: BlockRequest{
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
			},
			expected: "67976ddbbbc1a15a8f1819f1f914c6bbe1e1c174fb3be4e08f0c4370c4111acf",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := NewBuiltinRegistry()
			output, issues := registry.Instantiate(t.Context(), testCase.request)
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("instantiate issues = %#v", issues)
			}
			payload, err := json.Marshal(output.Operations)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(payload)
			got := hex.EncodeToString(sum[:])
			if got != testCase.expected {
				t.Fatalf("normalized operation digest = %s, want %s", got, testCase.expected)
			}
		})
	}
}
