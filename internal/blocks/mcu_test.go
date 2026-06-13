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

func TestMCUMinimalInstantiatesFixture(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "mcu_minimal",
		InstanceID: "mcu",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if warningCount(issues) != 1 {
		t.Fatalf("expected alternate-function warning, got %#v", issues)
	}
	const expectedDefaultRefs = 1 + 3 + 1 + 1 + 1 // MCU, decoupling caps, AREF cap, reset pull-up, ISP header.
	if len(output.Instance.Refs) != expectedDefaultRefs {
		t.Fatalf("refs = %#v", output.Instance.Refs)
	}
	if validation := transactions.Validate(transactions.Transaction{Operations: output.Operations}); len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestMCUMinimalBlocksUnknownSymbolRoleMap(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "mcu_minimal",
		InstanceID: "mcu",
		Params: map[string]any{
			"mcu_symbol": "MCU_ST_STM32F1:STM32F103C8Tx",
		},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.mcu_symbol" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestMCUMinimalValidatesDecouplingCount(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "mcu_minimal",
		InstanceID: "mcu",
		Params: map[string]any{
			"decoupling_count": 0,
		},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.decoupling_count" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestMCUMinimalValidatesResetMode(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "mcu_minimal",
		InstanceID: "mcu",
		Params: map[string]any{
			"reset_mode": "floating",
		},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.reset_mode" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestMCUMinimalNoProgrammingHeaderOmitsProgrammingNetsAndPorts(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "mcu_minimal",
		InstanceID: "mcu",
		Params: map[string]any{
			"programming_header": "none",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	for _, net := range output.Instance.Nets {
		if net == "mcu_mosi" || net == "mcu_miso" || net == "mcu_sck" || net == "mcu_uart_tx" || net == "mcu_uart_rx" {
			t.Fatalf("unexpected programming net %q in %#v", net, output.Instance.Nets)
		}
	}
	for _, port := range output.Instance.Ports {
		if port.Name == "MOSI" || port.Name == "MISO" || port.Name == "SCK" || port.Name == "UART_TX" || port.Name == "UART_RX" {
			t.Fatalf("unexpected programming port %#v", port)
		}
	}
}

func TestMCUMinimalISPHeaderPinsConnectToExpectedPorts(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "mcu_minimal", InstanceID: "mcu"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"library_id":"Connector_Generic:Conn_02x03_Odd_Even"`, `"net_name":"mcu_mosi"`, `"net_name":"mcu_miso"`, `"net_name":"mcu_sck"`, `"net_name":"mcu_reset"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("operations missing %q: %s", want, text)
		}
	}
}

func TestMCUMinimalProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "mcu_minimal", InstanceID: "mcu"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("mcu_minimal", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "mcu_minimal")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"mcu_minimal.kicad_pro", "mcu_minimal.kicad_sch", "mcu_minimal.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
