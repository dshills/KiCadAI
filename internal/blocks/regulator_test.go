package blocks

import (
	"context"
	"encoding/json"
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

func TestVoltageRegulatorAMS1117PinsMatchKiCadSymbol(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	pins := addSymbolPinsForRole(t, output.Operations, "regulator")
	assertPinOffset(t, pins, "1", 0, 0)
	assertPinOffset(t, pins, "2", 7.62, 0)
	assertPinOffset(t, pins, "3", -7.62, 0)
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

func TestVoltageRegulatorAP2112KConnectsEnableAndNoConnect(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"input_voltage_min":   "3.8V",
			"input_voltage_max":   "6V",
			"input_voltage":       "5V",
			"output_current":      "100mA",
			"enable_mode":         "tied_input",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if len(output.Operations) != 18 {
		t.Fatalf("operations = %#v", output.Operations)
	}
	regulatorRef := output.Instance.Refs[0]
	if !hasAddSymbolPins(output.Operations, regulatorRef, "1", "2", "3", "4", "5") {
		t.Fatalf("expected AP2112K five-pin add_symbol operation for %s", regulatorRef)
	}
	if !hasConnect(output.Operations, regulatorRef, "3", regulatorRef, "1", "rail3v3_vin") {
		t.Fatalf("expected AP2112K EN pin tied to VIN")
	}
	if !hasNoConnect(output.Operations, regulatorRef, "4") {
		t.Fatalf("expected AP2112K NC pin marker")
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestVoltageRegulatorAP2112KRejectsExternalEnableExport(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"enable_mode":         "export",
		},
	})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityBlocked || issues[0].Path != "params.enable_mode" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestVoltageRegulatorAP2112KRejectsUnhandledEnablePin(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"enable_mode":         "none",
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
	resistorRef := output.Instance.Refs[3]
	ledRef := output.Instance.Refs[4]
	if !hasPinOnNet(output.Operations, resistorRef, "1", "rail3v3_vout") {
		t.Fatalf("expected VOUT net to feed power LED resistor pin 1")
	}
	if !hasConnect(output.Operations, resistorRef, "2", ledRef, "2", "rail3v3_power_led_series") {
		t.Fatalf("expected power LED resistor pin 2 to feed LED anode pin 2")
	}
	if !hasConnect(output.Operations, ledRef, "1", "rail3v3", "GND", "rail3v3_gnd") {
		t.Fatalf("expected power LED cathode pin 1 to return to GND")
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

func TestVoltageRegulatorAP2112KProjectTransactionWritesNoConnect(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "voltage_regulator",
		InstanceID: "rail3v3",
		Params: map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"input_voltage_min":   "3.8V",
			"input_voltage_max":   "6V",
			"enable_mode":         "tied_input",
		},
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
	data, err := os.ReadFile(filepath.Join(outputDir, "rail3v3.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "(no_connect") {
		t.Fatalf("expected no_connect marker in generated schematic")
	}
}

func hasAddSymbolPins(operations []transactions.Operation, ref string, pins ...string) bool {
	for _, op := range operations {
		if op.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil || payload.Ref != ref {
			continue
		}
		found := map[string]bool{}
		for _, pin := range payload.Pins {
			found[pin.Number] = true
		}
		for _, pin := range pins {
			if !found[pin] {
				return false
			}
		}
		return true
	}
	return false
}

func hasConnect(operations []transactions.Operation, fromRef string, fromPin string, toRef string, toPin string, netName string) bool {
	for _, op := range operations {
		if op.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil {
			continue
		}
		if payload.NetName == netName &&
			payload.From.Ref == fromRef && payload.From.Pin == fromPin &&
			payload.To.Ref == toRef && payload.To.Pin == toPin {
			return true
		}
	}
	return false
}

func hasPinOnNet(operations []transactions.Operation, ref string, pin string, netName string) bool {
	for _, op := range operations {
		if op.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil {
			continue
		}
		if payload.NetName == netName &&
			((payload.From.Ref == ref && payload.From.Pin == pin) ||
				(payload.To.Ref == ref && payload.To.Pin == pin)) {
			return true
		}
	}
	return false
}

func addSymbolPins(t *testing.T, operations []transactions.Operation, ref string) map[string]transactions.PinSpec {
	t.Helper()
	pins, ok := addSymbolPinsMatching(t, operations, func(payload transactions.AddSymbolOperation) bool {
		return payload.Ref == ref
	})
	if !ok {
		t.Fatalf("missing add_symbol operation for %s", ref)
	}
	return pins
}

func addSymbolPinsForRole(t *testing.T, operations []transactions.Operation, role string) map[string]transactions.PinSpec {
	t.Helper()
	pins, ok := addSymbolPinsMatching(t, operations, func(payload transactions.AddSymbolOperation) bool {
		return payload.Role == role
	})
	if !ok {
		t.Fatalf("missing add_symbol operation for role %s", role)
	}
	return pins
}

func addSymbolPinsMatching(t *testing.T, operations []transactions.Operation, match func(transactions.AddSymbolOperation) bool) (map[string]transactions.PinSpec, bool) {
	t.Helper()
	for _, op := range operations {
		if op.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil || !match(payload) {
			continue
		}
		pins := make(map[string]transactions.PinSpec, len(payload.Pins))
		for _, pin := range payload.Pins {
			pins[pin.Number] = pin
		}
		return pins, true
	}
	return nil, false
}

func assertPinOffset(t *testing.T, pins map[string]transactions.PinSpec, pin string, wantX float64, wantY float64) {
	t.Helper()
	got, ok := pins[pin]
	if !ok {
		t.Fatalf("missing pin %s in %#v", pin, pins)
	}
	if got.XMM != wantX || got.YMM != wantY {
		t.Fatalf("pin %s offset = (%g, %g), want (%g, %g)", pin, got.XMM, got.YMM, wantX, wantY)
	}
}

func hasNoConnect(operations []transactions.Operation, ref string, pin string) bool {
	for _, op := range operations {
		if op.Op != transactions.OpAddNoConnect {
			continue
		}
		var payload transactions.AddNoConnectOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil {
			continue
		}
		if payload.Endpoint.Ref == ref && payload.Endpoint.Pin == pin {
			return true
		}
	}
	return false
}
