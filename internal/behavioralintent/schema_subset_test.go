package behavioralintent

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"testing"
	"unicode/utf8"
)

// Deliberately test only the emitted subset; fail the test on unknown schema
// keywords rather than claiming a general-purpose JSON Schema implementation.
func providerSchemaAccepts(t *testing.T, schema map[string]any, value any) bool {
	t.Helper()
	for key := range schema {
		switch key {
		case "type", "properties", "required", "additionalProperties", "anyOf", "const", "enum", "pattern", "minimum", "maximum", "exclusiveMinimum", "minItems", "maxItems", "minLength", "maxLength", "items", "description":
		default:
			t.Fatalf("untested schema keyword %s", key)
		}
	}
	equal := func(a, b any) bool { aa, _ := json.Marshal(a); bb, _ := json.Marshal(b); return bytes.Equal(aa, bb) }
	if expected, ok := schema["const"]; ok && !equal(value, expected) {
		return false
	}
	if choices, ok := schema["enum"].([]string); ok {
		found := false
		for _, choice := range choices {
			found = found || equal(value, choice)
		}
		if !found {
			return false
		}
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		for _, branch := range branches {
			if providerSchemaAccepts(t, branch.(map[string]any), value) {
				return true
			}
		}
		return false
	}
	switch schema["type"] {
	case "null":
		return value == nil
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		properties := schema["properties"].(map[string]any)
		for _, name := range schema["required"].([]string) {
			if _, ok := object[name]; !ok {
				return false
			}
		}
		for name, v := range object {
			property, ok := properties[name]
			if !ok {
				return false
			}
			if !providerSchemaAccepts(t, property.(map[string]any), v) {
				return false
			}
		}
		return schema["additionalProperties"] == false
	case "array":
		array, ok := value.([]any)
		if !ok {
			return false
		}
		if n, ok := schema["minItems"].(int); ok && len(array) < n {
			return false
		}
		if n, ok := schema["maxItems"].(int); ok && len(array) > n {
			return false
		}
		for _, v := range array {
			if !providerSchemaAccepts(t, schema["items"].(map[string]any), v) {
				return false
			}
		}
		return true
	case "string":
		text, ok := value.(string)
		if !ok {
			return false
		}
		if pattern, ok := schema["pattern"].(string); ok && !regexp.MustCompile(pattern).MatchString(text) {
			return false
		}
		if n, ok := schema["minLength"].(int); ok && utf8.RuneCountInString(text) < n {
			return false
		}
		if n, ok := schema["maxLength"].(int); ok && utf8.RuneCountInString(text) > n {
			return false
		}
		return true
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number", "integer":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
		if schema["type"] == "integer" && math.Trunc(n) != n {
			return false
		}
		for _, key := range []string{"minimum", "maximum", "exclusiveMinimum"} {
			if bound, ok := schema[key]; ok {
				b := 0.0
				switch v := bound.(type) {
				case int:
					b = float64(v)
				case float64:
					b = v
				default:
					t.Fatalf("unsupported bound %T", bound)
				}
				if key == "minimum" && n < b || key == "maximum" && n > b || key == "exclusiveMinimum" && n <= b {
					return false
				}
			}
		}
		return true
	default:
		t.Fatalf("untested schema type %#v", schema["type"])
		return false
	}
}
