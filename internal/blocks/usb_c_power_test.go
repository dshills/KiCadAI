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

func TestUSBCPowerInstantiatesDefaultSink(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if warningCount(issues) != 1 {
		t.Fatalf("expected no-connect warning, got %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 6 || !strings.HasPrefix(got[0], "J") || !strings.HasPrefix(got[1], "R") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) < 6 || got[0] != "usb_vbus_out" {
		t.Fatalf("nets = %#v", got)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestUSBCPowerSymbolPinsFollowPowerOnlyRoleMap(t *testing.T) {
	pins := usbCSymbolPins(usbCPowerPins)
	got := map[string]bool{}
	for _, pin := range pins {
		got[pin.Number] = true
	}
	for _, want := range []string{"A5", "A9", "A12", "B5", "B9", "B12", "SH"} {
		if !got[want] {
			t.Fatalf("missing power-only USB-C pin %s in %#v", want, pins)
		}
	}
	for _, forbidden := range []string{"A1", "A4", "A6", "A7", "A8", "B1", "B4", "B6", "B7", "B8"} {
		if got[forbidden] {
			t.Fatalf("unexpected 16-pin USB-C pin %s in %#v", forbidden, pins)
		}
	}
}

func TestUSBCPowerCCPullDownsArePresent(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "usb_c_power", InstanceID: "usb"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"value":"5.1k"`, `"net_name":"usb_cc1"`, `"net_name":"usb_cc2"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("operations missing %q: %s", want, text)
		}
	}
}

func TestUSBCPowerPowerLEDIsForwardBiased(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_power_led": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	rolesByRef := addSymbolRolesByRef(t, output.Operations)
	var resistorRef, ledRef string
	for ref, role := range rolesByRef {
		switch role {
		case "power_led_resistor":
			resistorRef = ref
		case "power_led":
			ledRef = ref
		}
	}
	if resistorRef == "" || ledRef == "" {
		t.Fatalf("roles by ref = %#v", rolesByRef)
	}
	if !hasPinOnNet(output.Operations, resistorRef, "1", "usb_vbus_out") {
		t.Fatalf("expected VBUS_OUT net to feed power LED resistor pin 1")
	}
	if !hasConnect(output.Operations, resistorRef, "2", ledRef, "2", "usb_power_led_series") {
		t.Fatalf("expected power LED resistor pin 2 to feed LED anode pin 2")
	}
	if !hasPinOnNet(output.Operations, ledRef, "1", "usb_gnd") {
		t.Fatalf("expected power LED cathode pin 1 to return to GND")
	}
}

func TestUSBCPowerFuseCanBeDisabled(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_fuse": false,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	for _, ref := range output.Instance.Refs {
		if strings.HasPrefix(ref, "F") {
			t.Fatalf("fuse ref should be absent: %#v", output.Instance.Refs)
		}
	}
}

func TestUSBCPowerFloatingShieldIsNotExported(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"shield_policy": "floating",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if warningCount(issues) != 2 {
		t.Fatalf("expected no-connect and floating shield warnings, got %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"pin":"SH"`) || strings.Contains(string(data), `"SHIELD"`) {
		t.Fatalf("floating shield should not be connected: %s", string(data))
	}
}

func TestUSBCPowerExportsVBUSOut(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "usb_c_power", InstanceID: "usb"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	found := false
	for _, port := range output.Instance.Ports {
		found = found || port.Name == "VBUS_OUT"
	}
	if !found {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
}

func TestUSBCPowerRejectsUnsupportedDataMode(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"data_mode": "usb2",
		},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.data_mode" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestUSBCPowerProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("usb_power", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "usb_power")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"usb_power.kicad_pro", "usb_power.kicad_sch", "usb_power.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func warningCount(issues []reports.Issue) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == reports.SeverityWarning {
			count++
		}
	}
	return count
}
