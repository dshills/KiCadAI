package blocks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
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
	if countHeadphoneProtectionOperations(output.Operations, transactions.OpAddSymbol) != 3 {
		t.Fatalf("operations = %#v, want capacitor, bleed resistor, and load-return anchor", output.Operations)
	}
	if countHeadphoneProtectionOperations(output.Operations, transactions.OpConnect) != 5 {
		t.Fatalf("operations = %#v, want five default connects", output.Operations)
	}
	if !headphoneProtectionHasLabel(t, output.Operations, "hp_protect_amp_out_dc_biased") ||
		!headphoneProtectionHasLabel(t, output.Operations, "hp_protect_hp_out") ||
		!headphoneProtectionHasLabel(t, output.Operations, "hp_protect_load_ref") ||
		!headphoneProtectionHasLabel(t, output.Operations, "hp_protect_load_ret") {
		t.Fatalf("missing expected labels in operations: %#v", output.Operations)
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

func TestHeadphoneOutputProtectionEmitsOptionalSeriesResistor(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    headphoneOutputProtectionID,
		InstanceID: "hp_protect",
		Params: map[string]any{
			"series_resistor_ohms": "22Ω",
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 4 {
		t.Fatalf("refs = %#v, want capacitor, bleed, series, and load return anchor", output.Instance.Refs)
	}
	if countHeadphoneProtectionOperations(output.Operations, transactions.OpAddSymbol) != 4 {
		t.Fatalf("operations = %#v, want four add-symbol operations", output.Operations)
	}
	if countHeadphoneProtectionOperations(output.Operations, transactions.OpConnect) != 6 {
		t.Fatalf("operations = %#v, want six connect operations with series resistor", output.Operations)
	}
	if !slicesContainString(output.Instance.Nets, "hp_protect_coupled_output") {
		t.Fatalf("nets = %#v, want coupled output net when series resistor is present", output.Instance.Nets)
	}
}

func TestHeadphoneOutputProtectionCanSkipBleedPolicy(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    headphoneOutputProtectionID,
		InstanceID: "hp_protect",
		Params: map[string]any{
			"bleed_required": false,
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if countHeadphoneProtectionOperations(output.Operations, transactions.OpAddSymbol) != 2 {
		t.Fatalf("operations = %#v, want capacitor and load-return anchor only", output.Operations)
	}
	if slicesContainString(output.Instance.Refs, "R1") {
		t.Fatalf("refs = %#v, did not expect bleed resistor ref", output.Instance.Refs)
	}
}

func TestHeadphoneOutputProtectionDoesNotConnectMissingComponents(t *testing.T) {
	definition := headphoneOutputProtectionDefinition()
	definition.Components = nil
	operations, refs, nets, issues := headphoneOutputProtectionOperations(definition, BlockRequest{
		BlockID:    headphoneOutputProtectionID,
		InstanceID: "hp_protect",
	}, ApplyParameterDefaults(definition, nil))
	if len(issues) == 0 {
		t.Fatal("expected missing component issues")
	}
	if len(operations) != 0 || len(refs) != 0 || len(nets) != 0 {
		t.Fatalf("operations=%#v refs=%#v nets=%#v, want no malformed output", operations, refs, nets)
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

func countHeadphoneProtectionOperations(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func headphoneProtectionHasLabel(t *testing.T, operations []transactions.Operation, label string) bool {
	t.Helper()
	for _, operation := range operations {
		if operation.Op != transactions.OpAddLabel {
			continue
		}
		var payload transactions.AddLabelOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("unmarshal label: %v", err)
		}
		if payload.Text == label {
			return true
		}
	}
	return false
}

func slicesContainString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
