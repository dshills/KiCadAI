package blocks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBlockDefinitionJSONShape(t *testing.T) {
	definition := minimalDefinition()
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	output := string(data)
	for _, want := range []string{`"id":"led_indicator"`, `"verification"`, `"level":"experimental"`, `"parameters"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestApplyParameterDefaultsDeterministic(t *testing.T) {
	definition := minimalDefinition()
	params := ApplyParameterDefaults(definition, map[string]any{"color": "green"})
	if params["color"] != "green" || params["active_high"] != true {
		t.Fatalf("params = %#v", params)
	}
	keys := SortedParamKeys(params)
	if len(keys) != 2 || keys[0] != "active_high" || keys[1] != "color" {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestApplyParameterDefaultsCopiesReferenceValues(t *testing.T) {
	label := "A"
	definition := BlockDefinition{Parameters: []BlockParameter{
		{Name: "pins", Type: ParameterStringList, Default: []string{"VCC", "GND"}},
		{Name: "nested", Type: ParameterString, Default: []any{map[string]any{"name": "A"}}},
		{Name: "numbers", Type: ParameterNumber, Default: []int{1, 2}},
		{Name: "floats", Type: ParameterNumber, Default: []float64{1.0, 2.0}},
		{Name: "flags", Type: ParameterBool, Default: []bool{true, false}},
		{Name: "labels", Type: ParameterString, Default: map[string]string{"a": "A"}},
		{Name: "label_ptr", Type: ParameterString, Default: &label},
	}}
	first := ApplyParameterDefaults(definition, nil)
	second := ApplyParameterDefaults(definition, nil)
	first["pins"].([]string)[0] = "CHANGED"
	first["nested"].([]any)[0].(map[string]any)["name"] = "CHANGED"
	first["numbers"].([]int)[0] = 99
	first["floats"].([]float64)[0] = 99
	first["flags"].([]bool)[0] = false
	first["labels"].(map[string]string)["a"] = "CHANGED"
	*first["label_ptr"].(*string) = "CHANGED"
	if second["pins"].([]string)[0] != "VCC" {
		t.Fatalf("defaults share backing storage: first=%#v second=%#v", first, second)
	}
	if second["nested"].([]any)[0].(map[string]any)["name"] != "A" || second["numbers"].([]int)[0] != 1 || second["floats"].([]float64)[0] != 1.0 || second["flags"].([]bool)[0] != true || second["labels"].(map[string]string)["a"] != "A" || *second["label_ptr"].(*string) != "A" {
		t.Fatalf("nested defaults share backing storage: first=%#v second=%#v", first, second)
	}
}

func TestValidateBlockID(t *testing.T) {
	if issues := ValidateBlockID("led_indicator"); len(issues) != 0 {
		t.Fatalf("valid ID issues = %#v", issues)
	}
	if issues := ValidateBlockID("LED Indicator"); len(issues) != 1 || !issues[0].Blocking() {
		t.Fatalf("invalid ID issues = %#v", issues)
	}
	if issues := ValidateBlockID(" led_indicator "); len(issues) != 1 || !strings.Contains(issues[0].Message, "whitespace") {
		t.Fatalf("whitespace ID issues = %#v", issues)
	}
}

func TestValidateParametersRejectsInvalidTypes(t *testing.T) {
	definition := minimalDefinition()
	params := ApplyParameterDefaults(definition, map[string]any{"color": "red", "active_high": "yes"})
	issues := ValidateParameters(definition, params)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "active_high must be a bool") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateParametersRejectsMissingRequiredAndUnknown(t *testing.T) {
	definition := minimalDefinition()
	issues := ValidateParameters(definition, map[string]any{"extra": true})
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateParametersComparesEnumScalarsByType(t *testing.T) {
	definition := BlockDefinition{Parameters: []BlockParameter{{Name: "mode", Type: ParameterEnum, Allowed: []any{"1"}}}}
	if issues := ValidateParameters(definition, map[string]any{"mode": 1}); len(issues) != 1 {
		t.Fatalf("string enum should not accept numeric value: %#v", issues)
	}
	definition.Parameters[0].Allowed = []any{int32(1)}
	if issues := ValidateParameters(definition, map[string]any{"mode": uint8(1)}); len(issues) != 0 {
		t.Fatalf("numeric enum should accept numeric value: %#v", issues)
	}
}

func TestScalarEqualHandlesNilAndTinyNumbers(t *testing.T) {
	if !scalarEqual(nil, nil) {
		t.Fatalf("nil values should compare equal")
	}
	if scalarEqual(1e-10, 1.01e-10) {
		t.Fatalf("distinct small values should not compare equal")
	}
}

func TestValidateParametersAcceptsStandardNumericTypes(t *testing.T) {
	definition := BlockDefinition{Parameters: []BlockParameter{{Name: "count", Type: ParameterNumber}}}
	for _, value := range []any{int8(1), int16(1), int32(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1)} {
		if issues := ValidateParameters(definition, map[string]any{"count": value}); len(issues) != 0 {
			t.Fatalf("value %T rejected: %#v", value, issues)
		}
	}
}

func TestVerificationLevelStableString(t *testing.T) {
	if string(VerificationRoundTripVerified) != "roundtrip_verified" {
		t.Fatalf("verification level changed: %q", VerificationRoundTripVerified)
	}
}

func minimalDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          "led_indicator",
		Name:        "LED Indicator",
		Version:     "0.1.0",
		Category:    "indicator",
		Description: "Status LED",
		Parameters: []BlockParameter{
			{Name: "color", Type: ParameterEnum, Required: true, Allowed: []any{"red", "green"}},
			{Name: "active_high", Type: ParameterBool, Default: true},
		},
		Ports: []BlockPort{{Name: "IN", Direction: PortInput}, {Name: "GND", Direction: PortPower}},
		Verification: VerificationRecord{
			Level: VerificationExperimental,
		},
	}
}
