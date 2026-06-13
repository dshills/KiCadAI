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
