package blocks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestVoltageRegulatorInstantiatesDefaultOperations(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 3 || !strings.HasPrefix(got[0], "U") || !strings.HasPrefix(got[1], "C") || !strings.HasPrefix(got[2], "C") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 3 || got[0] != "rail3v3_vin" || got[1] != "rail3v3_vout" || got[2] != "rail3v3_gnd" {
		t.Fatalf("nets = %#v", got)
	}
	if len(output.Operations) != 16 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestVoltageRegulatorDropoutWarningDoesNotBlock(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"input_voltage_min": "3.8V",
			"output_voltage":    "3.3V",
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityWarning || !strings.Contains(issues[0].Message, "dropout") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestVoltageRegulatorRejectsInvalidEnableMode(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"enable_mode": "export",
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityBlocked || issues[0].Path != "params.enable_mode" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestVoltageRegulatorRejectsNominalInputBelowOutput(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"input_voltage":  "3.0V",
			"output_voltage": "3.3V",
		},
	})
	if len(issues) != 1 || issues[0].Path != "params.input_voltage" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestVoltageRegulatorRejectsUnsupportedSymbolPinout(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail5v",
		Params: map[string]any{
			"regulator_symbol": "Regulator_Linear:L7805",
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityBlocked || issues[0].Path != "params.regulator_symbol" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestVoltageRegulatorPowerLEDComposesDeterministically(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"include_power_led": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 5 || !strings.HasPrefix(got[3], "R") || !strings.HasPrefix(got[4], "D") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) != 4 || got[3] != "rail3v3_power_led_series" {
		t.Fatalf("nets = %#v", got)
	}
	if output.Instance.Params["output_voltage"] != "3.3V" {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
	if len(output.Operations) != 25 {
		t.Fatalf("operations = %#v", output.Operations)
	}
}

func TestVoltageRegulatorProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("rail3v3", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "rail3v3")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"rail3v3.kicad_pro", "rail3v3.kicad_sch", "rail3v3.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
