package verification

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/transactions"
)

func TestRunCaseLEDIndicatorPassesSemantics(t *testing.T) {
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "led_indicator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusPass {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Stages) != 3 || result.Stages[2].Name != "semantic_assertions" {
		t.Fatalf("stages = %#v", result.Stages)
	}
}

func TestRunCaseBlocksWrongExpectedSymbol(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Components[0].SymbolID = "Device:C"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected symbol Device:C") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCaseBlocksMissingNetPinMembership(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	manifest.Expected.Nets[0].Pins[0].Pin = "1"
	manifest.Expected.Nets[0].Pins[1].Pin = "2"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry()})
	if result.Status != StatusBlocked || !hasIssue(result.Issues, "expected role") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAssertSemanticsMatchesRepeatedComponentRoles(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "led", RefPrefix: "D", SymbolID: "Device:LED"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsDoesNotDoubleMatchExplicitRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "missing expected component resistor") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsMatchesExplicitRefsBeforeRoles(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R", SymbolID: "Device:R"},
		{Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsAcceptsAnyMatchingRolePin(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsReportsMissingRolePin(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "C1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected role resistor pin 2") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestRunCaseStrictReportsUnexpectedNetWarning(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Name = "status_led_series"
	result := RunCase(context.Background(), manifest, RunOptions{Registry: blocks.NewBuiltinRegistry(), Strict: true})
	if result.Status != StatusWarning || !hasIssue(result.Issues, "unexpected generated net") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAssertSemanticsChecksRoleForExplicitRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components[0].Ref = "R1"
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "capacitor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected role resistor, got capacitor") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertStrictSemanticsReportsUnexpectedComponentAndPort(t *testing.T) {
	manifest := validManifest()
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
			"C1": {Role: "capacitor", Ref: "C1", SymbolID: "Device:C"},
		},
		Nets:  map[string][]actualPin{"LED_A": {{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}, "OUT": {Name: "OUT", Direction: blocks.PortOutput}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	for _, want := range []string{"unexpected generated component C1", "unexpected generated port OUT"} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
	}
}

func TestAssertStrictSemanticsReportsExtraSharedRoleComponent(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
			"D1": {Role: "led", Ref: "D1", SymbolID: "Device:LED", FootprintID: "LED_SMD:LED_0805_2012Metric"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if !hasIssue(issues, "unexpected generated component R2") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsChecksExplicitRefPrefix(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", Ref: "R1", RefPrefix: "C", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{})
	if !hasIssue(issues, "expected ref prefix C, got R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsUsesRefPrefixForRoleMatching(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R2", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 1 || !hasIssue(issues, "unexpected generated component R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssertSemanticsDoesNotSkipRoleCandidatesAfterPrefixMiss(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components = []ExpectedComponent{
		{Role: "resistor", RefPrefix: "R2", SymbolID: "Device:R"},
		{Role: "resistor", SymbolID: "Device:R"},
	}
	manifest.Expected.Nets = nil
	summary := semanticSummary{
		Components: map[string]actualComponent{
			"R1": {Role: "resistor", Ref: "R1", SymbolID: "Device:R"},
			"R2": {Role: "resistor", Ref: "R2", SymbolID: "Device:R"},
		},
		Ports: map[string]blocks.BlockPort{"IN": {Name: "IN", Direction: blocks.PortInput}, "GND": {Name: "GND", Direction: blocks.PortPower}},
	}
	issues := assertSemantics(manifest, summary, RunOptions{Strict: true})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestComparePinNamesSortsNumericPinsNaturally(t *testing.T) {
	pins := []actualPin{{Ref: "U1", Pin: "A10"}, {Ref: "U1", Pin: "10"}, {Ref: "U1", Pin: "2"}, {Ref: "U1", Pin: "A2"}, {Ref: "U1", Pin: "01"}, {Ref: "U1", Pin: "1"}}
	got := uniquePins(pins)
	want := []string{"01", "1", "2", "10", "A2", "A10"}
	for index, pin := range got {
		if pin.Pin != want[index] {
			t.Fatalf("pins = %#v", got)
		}
	}
}

func TestSummarizeOutputKeepsAnonymousNetsSeparate(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawConnect(t, "R1", "1", "R2", "1", ""),
			rawConnect(t, "R3", "1", "R4", "1", ""),
		},
	}
	summary, issues := summarizeOutput(output)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if got := len(summary.Nets); got != 2 {
		t.Fatalf("net count = %d, nets = %#v", got, summary.Nets)
	}
	for _, netName := range []string{"__anonymous_net_0", "__anonymous_net_1"} {
		if _, ok := summary.Nets[netName]; !ok {
			t.Fatalf("missing %s in %#v", netName, summary.Nets)
		}
	}
}

func TestSummarizeOutputMergesAnonymousConnectionChains(t *testing.T) {
	output := blocks.BlockOutput{
		Operations: []transactions.Operation{
			rawConnect(t, "R1", "1", "R2", "1", ""),
			rawConnect(t, "R2", "1", "R3", "1", ""),
		},
	}
	summary, issues := summarizeOutput(output)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	pins := summary.Nets["__anonymous_net_0"]
	if len(summary.Nets) != 1 || len(pins) != 3 {
		t.Fatalf("nets = %#v", summary.Nets)
	}
	for _, pin := range []actualPin{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}, {Ref: "R3", Pin: "1"}} {
		if _, ok := pinSet(pins)[pin]; !ok {
			t.Fatalf("missing pin %#v in %#v", pin, pins)
		}
	}
}

func rawConnect(t *testing.T, fromRef string, fromPin string, toRef string, toPin string, netName string) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(transactions.ConnectOperation{
		Op:      transactions.OpConnect,
		From:    transactions.Endpoint{Ref: fromRef, Pin: fromPin},
		To:      transactions.Endpoint{Ref: toRef, Pin: toPin},
		NetName: netName,
	})
	if err != nil {
		t.Fatalf("marshal connect: %v", err)
	}
	return transactions.NewOperation(transactions.OpConnect, raw)
}
