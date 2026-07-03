package blocks

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestHeadphoneOutputProtectionInstantiatesWithDefaults(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    headphoneOutputProtectionID,
		InstanceID: "hp_protect",
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if output.Definition.ID != headphoneOutputProtectionID {
		t.Fatalf("definition ID = %q", output.Definition.ID)
	}
	if got := output.Instance.Params["load_kind"]; got != "headphone" {
		t.Fatalf("load_kind = %#v", got)
	}
	if got := output.Instance.Params["nominal_load_ohms"]; got != "32Ω" {
		t.Fatalf("nominal_load_ohms = %#v", got)
	}
	if len(output.Operations) != 0 {
		t.Fatalf("phase 1 model should not emit operations yet: %#v", output.Operations)
	}
}

func TestHeadphoneOutputProtectionAcceptsSupportedLoadClasses(t *testing.T) {
	registry := NewBuiltinRegistry()
	for _, load := range []string{"16Ω", "32Ω", "64Ω"} {
		t.Run(load, func(t *testing.T) {
			_, issues := registry.Instantiate(context.Background(), BlockRequest{
				BlockID:    headphoneOutputProtectionID,
				InstanceID: "hp_protect",
				Params: map[string]any{
					"nominal_load_ohms": load,
				},
			})
			if len(issues) != 0 {
				t.Fatalf("instantiate %s issues = %#v", load, issues)
			}
		})
	}
}

func TestHeadphoneOutputProtectionBlocksUnsupportedLoads(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]any
		message string
	}{
		{
			name:    "speaker",
			params:  map[string]any{"load_kind": "speaker"},
			message: "only headphone loads are supported",
		},
		{
			name:    "bridge",
			params:  map[string]any{"load_kind": "bridge"},
			message: "only headphone loads are supported",
		},
		{
			name:    "unsupported impedance",
			params:  map[string]any{"nominal_load_ohms": "8Ω"},
			message: "supported headphone load classes",
		},
		{
			name:    "dual rail review",
			params:  map[string]any{"coupling": "dual_rail_direct_review_required"},
			message: "require AC coupling",
		},
		{
			name:    "missing return policy",
			params:  map[string]any{"connector_return_policy": "unknown"},
			message: "connector_return_policy",
		},
		{
			name:    "unverified fault protection",
			params:  map[string]any{"fault_protection_status": "connectivity"},
			message: "fault protection connectivity is not verified",
		},
	}
	registry := NewBuiltinRegistry()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, issues := registry.Instantiate(context.Background(), BlockRequest{
				BlockID:    headphoneOutputProtectionID,
				InstanceID: "hp_protect",
				Params:     tc.params,
			})
			if len(issues) == 0 {
				t.Fatal("expected blocking issue")
			}
			if len(output.Operations) != 0 {
				t.Fatalf("blocked request emitted operations: %#v", output.Operations)
			}
			if !headphoneProtectionIssuesContain(issues, tc.message) {
				t.Fatalf("issues = %#v, want message containing %q", issues, tc.message)
			}
		})
	}
}

func TestHeadphoneOutputProtectionRequiresBleedValueWhenPolicyRequired(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    headphoneOutputProtectionID,
		InstanceID: "hp_protect",
		Params: map[string]any{
			"bleed_required":      true,
			"bleed_resistor_ohms": "0Ω",
		},
	})
	if !headphoneProtectionIssuesContain(issues, "bleed_resistor_ohms must be positive") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestHeadphoneOutputProtectionAppliesParamsToComponentMetadata(t *testing.T) {
	definition := headphoneOutputProtectionDefinitionForParams(headphoneOutputProtectionDefinition(), map[string]any{
		"dc_blocking_capacitance": "470uF",
		"bleed_resistor_ohms":     "220kΩ",
		"series_resistor_ohms":    "22Ω",
	})
	components := blockComponentByRole(definition.Components)
	if components["dc_blocking_capacitor"].Value != "470uF" {
		t.Fatalf("capacitor value = %q", components["dc_blocking_capacitor"].Value)
	}
	if components["bleed_resistor"].Value != "220kΩ" {
		t.Fatalf("bleed value = %q", components["bleed_resistor"].Value)
	}
	if components["series_resistor"].Value != "22Ω" {
		t.Fatalf("series value = %q", components["series_resistor"].Value)
	}
}

func headphoneProtectionIssuesContain(issues []reports.Issue, want string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, want) {
			return true
		}
	}
	return false
}
