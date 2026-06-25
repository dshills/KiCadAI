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
			Kind:   "sensor",
			Params: map[string]any{"address": "0x48", "nested": map[string]any{"mode": "fast"}},
		}},
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

func hasIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
