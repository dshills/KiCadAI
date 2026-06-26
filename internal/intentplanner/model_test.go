package intentplanner

import (
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func TestDecodeRequestStrictValidMinimal(t *testing.T) {
	request, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "USB Sensor Breakout!",
	  "kind": "breakout",
	  "board": {"width_mm": 50, "height_mm": 30},
	  "power": {"inputs": [{"kind": "usb_c", "voltage": "5V"}]},
	  "functions": [{"kind": "sensor", "family": "i2c_sensor"}]
	}`))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	normalized := NormalizeRequest(request)
	if normalized.Name != "USB_Sensor_Breakout" {
		t.Fatalf("name = %q", normalized.Name)
	}
	if normalized.Board.Layers != 2 {
		t.Fatalf("layers = %d, want default 2", normalized.Board.Layers)
	}
	if normalized.Acceptance != designworkflow.AcceptanceStructural {
		t.Fatalf("acceptance = %q", normalized.Acceptance)
	}
	if normalized.Power.Inputs[0].Strength != StrengthRequired {
		t.Fatalf("power input strength = %q", normalized.Power.Inputs[0].Strength)
	}
}

func TestDecodeRequestStrictRejectsUnknownField(t *testing.T) {
	_, issues := DecodeRequestStrict(strings.NewReader(`{"version":"0.1.0","name":"bad","unexpected":true}`))
	if !hasIssuePath(issues, "request") {
		t.Fatalf("issues = %#v, want request decode issue", issues)
	}
}

func TestDecodeRequestStrictRejectsTrailingJSON(t *testing.T) {
	_, issues := DecodeRequestStrict(strings.NewReader(`{"version":"0.1.0","name":"bad"} {}`))
	if !hasIssuePath(issues, "request") {
		t.Fatalf("issues = %#v, want request issue", issues)
	}
}

func TestValidateRequestReportsInvalidFields(t *testing.T) {
	issues := ValidateRequest(Request{
		Version:    "0.1.0",
		Name:       "bad",
		Kind:       IntentKind("unsupported"),
		Acceptance: designworkflow.AcceptanceLevel("magic"),
		Board:      BoardIntent{WidthMM: -1, HeightMM: 10, Layers: 4, MountingHoles: Strength("maybe")},
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "mains", Voltage: "five"}},
			Rails:  []PowerRailIntent{{Voltage: ""}},
		},
		Interfaces: []InterfaceIntent{{Kind: "canbus", Quantity: -1}},
		Functions:  []FunctionIntent{{Kind: "radio", Quantity: -1}},
		Protection: ProtectionIntent{ESD: Strength("yes")},
	})
	for _, path := range []string{
		"acceptance",
		"board.layers",
		"board.mounting_holes",
		"board.width_mm",
		"functions[0].kind",
		"functions[0].quantity",
		"interfaces[0].kind",
		"interfaces[0].quantity",
		"kind",
		"power.inputs[0].kind",
		"power.inputs[0].voltage",
		"power.rails[0].name",
		"power.rails[0].voltage",
		"protection.esd",
	} {
		if !hasIssuePath(issues, path) {
			t.Fatalf("missing issue path %s in %#v", path, issues)
		}
	}
}

func TestNormalizeRequestCopiesMutableFields(t *testing.T) {
	request := Request{
		Version: "0.1.0",
		Name:    "copy",
		Functions: []FunctionIntent{{
			Kind:      "sensor",
			Target:    TargetRef{ID: " MCU_1 ", Role: " MCU "},
			Interface: " I2C ",
			Bus:       " I2C_MAIN ",
			Supply:    " 3V3 ",
			Params:    map[string]any{"address": "0x48", "nested": map[string]any{"mode": "fast"}},
		}},
		Interfaces:  []InterfaceIntent{{Kind: " I2C ", Target: TargetRef{Role: " MCU "}, Bus: " I2C_MAIN "}},
		Power:       PowerIntent{Rails: []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", Alias: " 3V3 ", Supplies: []TargetRef{{ID: " SENSOR_1 "}}}}},
		Constraints: ConstraintIntent{PackagePreferences: map[string]string{" sensor ": " qfn "}},
	}
	normalized := NormalizeRequest(request)
	normalized.Functions[0].Params["address"] = "0x49"
	normalized.Functions[0].Params["nested"].(map[string]any)["mode"] = "slow"
	normalized.Constraints.PackagePreferences[" sensor "] = "changed"
	if request.Functions[0].Params["address"] != "0x48" {
		t.Fatalf("NormalizeRequest mutated params: %#v", request.Functions[0].Params)
	}
	if request.Functions[0].Params["nested"].(map[string]any)["mode"] != "fast" {
		t.Fatalf("NormalizeRequest mutated nested params: %#v", request.Functions[0].Params)
	}
	if request.Constraints.PackagePreferences[" sensor "] != " qfn " {
		t.Fatalf("NormalizeRequest mutated package preferences: %#v", request.Constraints.PackagePreferences)
	}
	if normalized.Constraints.PackagePreferences["sensor"] != "qfn" {
		t.Fatalf("package preferences not normalized: %#v", normalized.Constraints.PackagePreferences)
	}
	if normalized.Functions[0].Target.ID != "mcu_1" || normalized.Functions[0].Target.Role != "mcu" {
		t.Fatalf("function target not normalized: %#v", normalized.Functions[0].Target)
	}
	if normalized.Functions[0].Interface != "i2c" || normalized.Functions[0].Bus != "i2c_main" || normalized.Functions[0].Supply != "3v3" {
		t.Fatalf("function semantic fields not normalized: %#v", normalized.Functions[0])
	}
	if normalized.Interfaces[0].Target.Role != "mcu" || normalized.Interfaces[0].Bus != "i2c_main" {
		t.Fatalf("interface semantic fields not normalized: %#v", normalized.Interfaces[0])
	}
	if normalized.Power.Rails[0].Alias != "3v3" || normalized.Power.Rails[0].Supplies[0].ID != "sensor_1" {
		t.Fatalf("power rail semantic fields not normalized: %#v", normalized.Power.Rails[0])
	}
}

func TestValidateRequestRequiresInterfaceVoltageWhenRequired(t *testing.T) {
	issues := ValidateRequest(Request{
		Version:    "0.1.0",
		Name:       "iface",
		Kind:       IntentBreakout,
		Acceptance: designworkflow.AcceptanceStructural,
		Board:      BoardIntent{WidthMM: 20, HeightMM: 10, Layers: 2},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Strength: StrengthRequired}},
	})
	if !hasIssuePath(issues, "interfaces[0].voltage") {
		t.Fatalf("issues = %#v, want required interface voltage issue", issues)
	}
}

func TestDecodeRequestStrictAcceptsSemanticFields(t *testing.T) {
	request, issues := DecodeRequestStrict(strings.NewReader(`{
	  "version": "0.1.0",
	  "name": "semantic",
	  "kind": "sensor_node",
	  "power": {
	    "inputs": [{"kind": "external", "voltage": "5V"}],
	    "rails": [{"name": "VCC", "voltage": "3.3V", "alias": "3v3", "supplies": [{"role": "sensor"}]}]
	  },
	  "interfaces": [{"kind": "i2c", "voltage": "3.3V", "bus": "i2c1", "target": {"role": "mcu"}}],
	  "functions": [{"kind": "sensor", "family": "i2c_sensor", "interface": "i2c", "bus": "i2c1", "supply": "3v3", "target": {"id": "mcu_1"}}]
	}`))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	normalized := NormalizeRequest(request)
	if normalized.Power.Rails[0].Supplies[0].Role != "sensor" {
		t.Fatalf("supplies = %#v", normalized.Power.Rails[0].Supplies)
	}
	if normalized.Interfaces[0].Target.Role != "mcu" {
		t.Fatalf("interface target = %#v", normalized.Interfaces[0].Target)
	}
	if normalized.Functions[0].Target.ID != "mcu_1" || normalized.Functions[0].Interface != "i2c" {
		t.Fatalf("function = %#v", normalized.Functions[0])
	}
}

func TestValidateRequestReportsInvalidSemanticFields(t *testing.T) {
	issues := ValidateRequest(Request{
		Version: "0.1.0",
		Name:    "semantic_bad",
		Kind:    IntentSensorNode,
		Power: PowerIntent{
			Inputs: []PowerInputIntent{{Kind: "external", Voltage: "5V"}},
			Rails:  []PowerRailIntent{{Name: "VCC", Voltage: "3.3V", Alias: "bad alias", Supplies: []TargetRef{{ID: "sensor one"}}}},
		},
		Interfaces: []InterfaceIntent{{Kind: "i2c", Voltage: "3.3V", Bus: "bad bus", Target: TargetRef{Role: "mcu main"}}},
		Functions: []FunctionIntent{{
			Kind:      "sensor",
			Interface: "can",
			Bus:       "bad bus",
			Supply:    "bad supply",
			Target:    TargetRef{ID: "mcu one"},
		}},
	})
	for _, path := range []string{
		"functions[0].bus",
		"functions[0].interface",
		"functions[0].supply",
		"functions[0].target.id",
		"interfaces[0].bus",
		"interfaces[0].target.role",
		"power.rails[0].alias",
		"power.rails[0].supplies[0].id",
	} {
		if !hasIssuePath(issues, path) {
			t.Fatalf("missing issue path %s in %#v", path, issues)
		}
	}
}

func hasIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
