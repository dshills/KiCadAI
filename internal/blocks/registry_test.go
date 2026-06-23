package blocks

import (
	"context"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestBuiltinRegistryListsInitialBlocksSorted(t *testing.T) {
	registry, issues := NewBuiltinRegistryChecked()
	if len(issues) != 0 {
		t.Fatalf("registry issues = %#v", issues)
	}
	summaries := registry.ListBlocks()
	got := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		got = append(got, summary.ID)
	}
	want := []string{
		"connector_breakout",
		"crystal_oscillator",
		"esd_protection",
		"i2c_sensor",
		"led_indicator",
		"mcu_minimal",
		"opamp_gain_stage",
		"reset_programming_header",
		"usb_c_power",
		"voltage_regulator",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("IDs = %#v, want %#v", got, want)
	}
	summaries[0].ID = "mutated"
	if registry.ListBlocks()[0].ID != "connector_breakout" {
		t.Fatalf("ListBlocks returned mutable backing slice")
	}
}

func TestBuiltinRegistryGetBlockReturnsDefinition(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("led_indicator")
	if !ok {
		t.Fatalf("led_indicator not found")
	}
	if definition.Name != "LED Indicator" || definition.Verification.Level != VerificationStructural {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestGetBlockReturnsDefensiveCopy(t *testing.T) {
	definition := minimalDefinition()
	definition.Components = []BlockComponent{{Role: "led", Properties: map[string]string{"a": "A"}, Alternatives: []string{"Device:LED"}}}
	definition.Nets = []BlockNet{{NameTemplate: "N", Pins: []NetPin{{ComponentRole: "led", Pin: "1"}}, Constraints: []string{"short"}}}
	definition.Verification.Evidence = []string{"test"}
	registry := NewRegistry([]BlockDefinition{definition})
	got, ok := registry.GetBlock(definition.ID)
	if !ok {
		t.Fatalf("definition not found")
	}
	got.Parameters[0].Allowed[0] = "mutated"
	got.Components[0].Properties["a"] = "mutated"
	got.Components[0].Alternatives[0] = "mutated"
	got.Nets[0].Pins[0].Pin = "mutated"
	got.Nets[0].Constraints[0] = "mutated"
	got.Verification.Evidence[0] = "mutated"

	again, _ := registry.GetBlock(definition.ID)
	if again.Parameters[0].Allowed[0] != "red" ||
		again.Components[0].Properties["a"] != "A" ||
		again.Components[0].Alternatives[0] != "Device:LED" ||
		again.Nets[0].Pins[0].Pin != "1" ||
		again.Nets[0].Constraints[0] != "short" ||
		again.Verification.Evidence[0] != "test" {
		t.Fatalf("registry definition was mutated: %#v", again)
	}
}

func TestRegistryDetectsDuplicateBlockIDs(t *testing.T) {
	definition := minimalDefinition()
	registry := NewRegistry([]BlockDefinition{definition, definition})
	issues := registry.Issues()
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "duplicate block ID") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestBuiltinPlaceholdersHaveMetadata(t *testing.T) {
	for _, definition := range BuiltinDefinitions() {
		if definition.ID == "" || definition.Name == "" || definition.Version == "" {
			t.Fatalf("missing identity metadata: %#v", definition)
		}
		if len(definition.Parameters) == 0 || len(definition.Ports) == 0 {
			t.Fatalf("%s missing parameters or ports: %#v", definition.ID, definition)
		}
		if len(definition.Components) == 0 || definition.PCBRealization == nil {
			t.Fatalf("%s missing component or PCB realization metadata: %#v", definition.ID, definition)
		}
		structuralBlocks := map[string]bool{
			"connector_breakout":       true,
			"crystal_oscillator":       true,
			"esd_protection":           true,
			"i2c_sensor":               true,
			"led_indicator":            true,
			"mcu_minimal":              true,
			"opamp_gain_stage":         true,
			"reset_programming_header": true,
			"usb_c_power":              true,
			"voltage_regulator":        true,
		}
		if structuralBlocks[definition.ID] {
			if definition.Verification.Level != VerificationStructural {
				t.Fatalf("%s verification = %q", definition.ID, definition.Verification.Level)
			}
		} else if definition.Verification.Level != VerificationExperimental {
			t.Fatalf("%s verification = %q", definition.ID, definition.Verification.Level)
		}
		if definition.Verification.Level.AllowsFabricationReadinessClaim() {
			t.Errorf("%s unexpectedly allows fabrication readiness claims at level %q", definition.ID, definition.Verification.Level)
		}
		if issues := NewRegistry(nil).ValidateDefinition(definition); len(issues) != 0 {
			t.Fatalf("%s validation issues = %#v", definition.ID, issues)
		}
	}
}

func TestValidateDefinitionChecksPortVoltageReferences(t *testing.T) {
	registry := NewRegistry(nil)
	definition := minimalDefinition()
	definition.Ports[0].Voltage = "missing_voltage"
	issues := registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unknown parameter missing_voltage") {
		t.Fatalf("issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "color"
	issues = registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "must use voltage type") {
		t.Fatalf("issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "5V"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("literal voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = ".5V"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("decimal literal voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "500mv"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("lowercase prefixed voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "5 V"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("spaced voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "1GV"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("giga voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "1\u00B5V"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("micro voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "1e-3V"
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("scientific voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "5"
	if issues = registry.ValidateDefinition(definition); len(issues) != 1 || !strings.Contains(issues[0].Message, "unknown parameter 5") {
		t.Fatalf("unitless voltage issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "rail1_voltage"
	if issues = registry.ValidateDefinition(definition); len(issues) != 1 || !strings.Contains(issues[0].Message, "unknown parameter rail1_voltage") {
		t.Fatalf("digit-bearing parameter issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Ports[0].Voltage = "5_volt_rail"
	if issues = registry.ValidateDefinition(definition); len(issues) != 1 || !strings.Contains(issues[0].Message, "unknown parameter 5_volt_rail") {
		t.Fatalf("numeric-prefix parameter issues = %#v", issues)
	}
}

func TestValidateDefinitionChecksEnumDefaults(t *testing.T) {
	registry := NewRegistry(nil)
	definition := minimalDefinition()
	definition.Parameters[0].Default = "blue"
	issues := registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "enum default for color is not allowed") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateDefinitionChecksTypedDefaults(t *testing.T) {
	registry := NewRegistry(nil)
	definition := minimalDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "supply_voltage", Type: ParameterVoltage, Default: true})
	issues := registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "invalid type") {
		t.Fatalf("typed default issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "supply_voltage", Type: ParameterVoltage, Default: "ABC"})
	issues = registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "must include a voltage unit") {
		t.Fatalf("voltage default issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "led_current", Type: ParameterCurrent, Default: "ABC"})
	issues = registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "must include a current unit") {
		t.Fatalf("current default issues = %#v", issues)
	}

	definition = minimalDefinition()
	definition.Parameters = append(definition.Parameters, BlockParameter{Name: "led_current", Type: ParameterCurrent, Default: "2.5E-6A"})
	if issues = registry.ValidateDefinition(definition); len(issues) != 0 {
		t.Fatalf("scientific current default issues = %#v", issues)
	}
}

func TestValidateDefinitionRejectsDuplicateParameterNames(t *testing.T) {
	registry := NewRegistry(nil)
	definition := minimalDefinition()
	definition.Parameters = append(definition.Parameters, definition.Parameters[0])
	issues := registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "duplicate parameter name color") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateDefinitionRejectsDuplicatePortNames(t *testing.T) {
	registry := NewRegistry(nil)
	definition := minimalDefinition()
	definition.Ports = append(definition.Ports, definition.Ports[0])
	issues := registry.ValidateDefinition(definition)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "duplicate port name IN") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateRequestReportsMissingAndInvalidParams(t *testing.T) {
	registry := NewBuiltinRegistry()
	missing := registry.ValidateRequest(BlockRequest{BlockID: "missing_block"})
	if len(missing) != 1 || missing[0].Code != reports.CodeMissingFile {
		t.Fatalf("missing issues = %#v", missing)
	}
	invalid := registry.ValidateRequest(BlockRequest{BlockID: "led_indicator", Params: map[string]any{"active_high": "yes"}})
	if len(invalid) != 1 || !strings.Contains(invalid[0].Message, "active_high must be a bool") {
		t.Fatalf("invalid issues = %#v", invalid)
	}
}

func TestInstantiateNormalizesDefaults(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if output.Instance.BlockID != "led_indicator" || output.Instance.InstanceID != "status" {
		t.Fatalf("instance = %#v", output.Instance)
	}
	if output.Instance.Params["active_high"] != true || output.Instance.Params["color"] != "green" {
		t.Fatalf("params = %#v", output.Instance.Params)
	}
	if len(output.Instance.Ports) != 3 {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
}

func TestInstantiateRequiresInstanceID(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator"})
	if len(issues) != 1 || issues[0].Severity != reports.SeverityError || issues[0].Path != "instance_id" {
		t.Fatalf("issues = %#v", issues)
	}
	if output.Instance.InstanceID != "" {
		t.Fatalf("instance = %#v", output.Instance)
	}
}

func TestInstantiateReportsCanceledContext(t *testing.T) {
	registry := NewBuiltinRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, issues := registry.Instantiate(ctx, BlockRequest{BlockID: "led_indicator"})
	if len(issues) != 1 || issues[0].Code != reports.CodeValidationFailed {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestInstantiateReportsNilContext(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(nil, BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if len(issues) != 1 || issues[0].Code != reports.CodeValidationFailed || issues[0].Message != "context is required" {
		t.Fatalf("issues = %#v", issues)
	}
}
