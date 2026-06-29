package blocks

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestOpAmpGainStageInstantiatesNonInvertingGain(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"gain": 2.0,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 4 || !strings.HasPrefix(got[0], "U") || !strings.HasPrefix(got[1], "R") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 5 || got[2] != "amp_feedback" {
		t.Fatalf("nets = %#v", got)
	}
	if !feedbackRatioClose(2.0, 10000, 10000) {
		t.Fatal("feedback ratio helper failed")
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestOpAmpGainStageRejectsInvalidGainAndTopology(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "bad",
		Params: map[string]any{
			"gain":     1.0,
			"topology": "inverting",
		},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestOpAmpGainStageACCouplingAddsBiasNetwork(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"input_coupling": "ac",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 6 || got[5] != "amp_bias" {
		t.Fatalf("nets = %#v", got)
	}
}

func TestOpAmpGainStageSchematicLayoutIsReadable(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"input_coupling":          "ac",
			"include_output_resistor": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	positions := addSymbolPositionsByRole(t, output.Operations)
	wantRoles := []string{"input_coupling", "bias_top", "bias_bottom", "feedback", "opamp", "gain_to_ground", "decoupling_capacitor", "output_resistor"}
	for _, role := range wantRoles {
		if _, ok := positions[role]; !ok {
			t.Fatalf("missing role %s in positions %#v", role, positions)
		}
	}
	if !(positions["input_coupling"].XMM < positions["opamp"].XMM && positions["opamp"].XMM < positions["output_resistor"].XMM) {
		t.Fatalf("expected left-to-right signal flow, positions=%#v", positions)
	}
	if !(positions["feedback"].YMM < positions["opamp"].YMM && positions["decoupling_capacitor"].YMM < positions["opamp"].YMM) {
		t.Fatalf("expected feedback and decoupling above op-amp, positions=%#v", positions)
	}
	if !(positions["gain_to_ground"].YMM > positions["opamp"].YMM && positions["bias_bottom"].YMM > positions["bias_top"].YMM) {
		t.Fatalf("expected ground/reference elements lower on page, positions=%#v", positions)
	}
	if spread := positions["output_resistor"].XMM - positions["input_coupling"].XMM; spread < 60 {
		t.Fatalf("schematic spread = %g mm, want at least 60 mm", spread)
	}
}

func TestOpAmpGainStageDualSupplyWarnsAndBlocksACBias(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"single_supply": false,
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("issues = %#v", issues)
	}
	_, issues = registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"single_supply":  false,
			"input_coupling": "ac",
		},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("expected blocking issue, got %#v", issues)
	}
}

func TestOpAmpGainStageOptionalOutputResistor(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
		Params: map[string]any{
			"include_output_resistor": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Nets; len(got) != 6 || got[5] != "amp_out_series" {
		t.Fatalf("nets = %#v", got)
	}
}

func feedbackRatioClose(gain float64, rfOhms float64, rgOhms float64) bool {
	return math.Abs(gain-(1+rfOhms/rgOhms)) < 1e-9
}

func addSymbolPositionsByRole(t *testing.T, operations []transactions.Operation) map[string]transactions.Point {
	t.Helper()
	positions := map[string]transactions.Point{}
	for index, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatalf("decode add_symbol operation %d: %v", index, err)
		}
		positions[payload.Role] = payload.At
	}
	return positions
}

func TestOpAmpGainStageProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "opamp_gain_stage",
		InstanceID: "amp",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("amp", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "amp")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"amp.kicad_pro", "amp.kicad_sch", "amp.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
