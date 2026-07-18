package blocks

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"kicadai/internal/transactions"
)

func TestClassABOutputStageDefinitionContract(t *testing.T) {
	registry, issues := NewBuiltinRegistryChecked()
	if len(issues) != 0 {
		t.Fatalf("builtin registry issues = %#v", issues)
	}
	definition, ok := registry.GetBlock(classABOutputStageID)
	if !ok {
		t.Fatalf("missing block %s", classABOutputStageID)
	}
	if definition.Verification.Level != VerificationStructural {
		t.Fatalf("verification level = %q, want %q", definition.Verification.Level, VerificationStructural)
	}
	for _, port := range []string{"DRIVER_OUT", "VCC", "VEE", "AMP_OUT", "LOAD_REF"} {
		if !classABHasPort(definition, port) {
			t.Fatalf("missing port %s", port)
		}
	}
	for _, role := range []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed", "upper_emitter_resistor", "lower_emitter_resistor", "upper_output", "lower_output", "load_reference"} {
		if _, ok := blockComponentByRole(definition.Components)[role]; !ok {
			t.Fatalf("missing component role %s", role)
		}
	}
	if !slices.Contains(definition.Verification.Evidence, "component:bjt.onsemi.mmbt3904.sot23") {
		t.Fatalf("missing NPN output-device evidence: %#v", definition.Verification.Evidence)
	}
	if definition.PCBRealization == nil || len(definition.PCBRealization.EntryAnchors) != 5 {
		t.Fatalf("PCB entry anchors = %#v, want all five ports", definition.PCBRealization)
	}
	if len(definition.PCBRealization.PlacementGroups) == 0 || !definition.PCBRealization.PlacementGroups[0].TranslateAsUnit {
		t.Fatal("expected Class AB bias network to preserve and legalize its routed placement as a unit")
	}
	for _, routeID := range []string{"upper_emitter", "lower_emitter", "amp_out_join", "vcc_output", "vee_output", "load_reference"} {
		found := false
		for _, route := range definition.PCBRealization.LocalRoutes {
			found = found || route.ID == routeID
		}
		if !found {
			t.Fatalf("missing required local route %s", routeID)
		}
	}
	for _, route := range definition.PCBRealization.LocalRoutes {
		if route.From.Port == "DRIVER_OUT" || route.To.Port == "DRIVER_OUT" {
			t.Fatalf("driver net should enter through routed physical pads, not a redundant virtual-port stub: %#v", route)
		}
	}
}

func TestClassABOutputStageInstantiation(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classABOutputStageID,
		InstanceID: "hp_out",
		Params: map[string]any{
			"supply_voltage": "9V",
			"load_impedance": "32Ω",
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 9 {
		t.Fatalf("refs = %#v, want nine block component refs", output.Instance.Refs)
	}
	if len(output.Operations) == 0 {
		t.Fatal("expected schematic operations")
	}
	if got := classABOperationCount(output.Operations, transactions.OpAddSymbol); got != 9 {
		t.Fatalf("add symbol operations = %d, want 9", got)
	}
	if got := classABOperationCount(output.Operations, transactions.OpConnect); got < 15 {
		t.Fatalf("connect operations = %d, want at least 15", got)
	}
	if labels := classABLabels(t, output.Operations); len(labels) != 0 {
		t.Fatalf("class AB output stage should not emit decorative standalone labels: %#v", labels)
	}
}

func TestClassABOutputStageBlocksSpeakerLoad(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classABOutputStageID,
		InstanceID: "speaker_out",
		Params: map[string]any{
			"supply_voltage": "12V",
			"load_impedance": "8Ω",
		},
	})
	if len(issues) == 0 {
		t.Fatal("expected blocking issue for speaker-class load")
	}
	if len(output.Operations) != 0 {
		t.Fatalf("blocked output emitted operations: %#v", output.Operations)
	}
}

func TestClassABOutputStageCalculatesVBEMultiplier(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    classABOutputStageID,
		InstanceID: "trimmed",
		Params: map[string]any{
			"topology":                             "vbe_multiplier",
			"application":                          "headphone",
			"supply_voltage":                       "9V",
			"load_impedance":                       "32Ω",
			"target_quiescent_current":             "5mA",
			"output_vbe_voltage":                   "0.65V",
			"bias_multiplier_vbe_voltage":          "0.60V",
			"emitter_resistor_value":               "0.47Ω",
			"bias_multiplier_lower_resistor_value": "1kΩ",
			"bias_multiplier_component_id":         "bjt.onsemi.mmbt3904.sot23",
			"bias_multiplier_footprint":            "Package_TO_SOT_SMD:SOT-23",
		},
	})
	if len(issues) != 0 {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	if len(output.Instance.Refs) != 10 {
		t.Fatalf("refs = %#v, want ten VBE-multiplier components", output.Instance.Refs)
	}
	if got := output.Instance.Params["calculated_bias_voltage_v"].(float64); got <= 1.3 || got >= 1.32 {
		t.Fatalf("calculated bias voltage = %g, want about 1.305 V", got)
	}
	if got := output.Instance.Params["calculated_bias_multiplier_upper_ohms"].(float64); got <= 1160 || got >= 1180 {
		t.Fatalf("calculated multiplier resistance = %g, want about 1170 ohm using the multiplier transistor's independent VBE", got)
	}
	if got := classABOperationCount(output.Operations, transactions.OpConnect); got < 18 {
		t.Fatalf("connect operations = %d, want complete multiplier network", got)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func classABHasPort(definition BlockDefinition, name string) bool {
	for _, port := range definition.Ports {
		if port.Name == name {
			return true
		}
	}
	return false
}

func classABOperationCount(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func classABLabels(t *testing.T, operations []transactions.Operation) []string {
	t.Helper()
	var labels []string
	for _, operation := range operations {
		if operation.Op != transactions.OpAddLabel {
			continue
		}
		var payload transactions.AddLabelOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("unmarshal label: %v", err)
		}
		labels = append(labels, payload.Text)
	}
	return labels
}
