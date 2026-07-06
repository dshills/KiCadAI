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
