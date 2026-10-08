package boardfamily

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

// This deliberately small, test-only checker covers every keyword emitted by
// referencedIntentSchema. It is not a general JSON Schema implementation or
// proof of server acceptance. Unknown keywords fail rather than being ignored.
func checkReferencedSchemaValue(schema map[string]any, value any) error {
	for key := range schema {
		if !member(key, "type", "enum", "properties", "required", "additionalProperties", "items", "minItems", "maxItems", "minimum", "maximum", "anyOf", "description") {
			return fmt.Errorf("unhandled schema keyword %q", key)
		}
	}
	if description, ok := schema["description"]; ok {
		if _, ok := description.(string); !ok {
			return fmt.Errorf("schema description is not a string")
		}
	}
	if union, ok := schema["anyOf"].([]any); ok {
		for _, branch := range union {
			if checkReferencedSchemaValue(branch.(map[string]any), value) == nil {
				return nil
			}
		}
		return fmt.Errorf("no anyOf branch accepts the value")
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("not an object")
		}
		properties := schema["properties"].(map[string]any)
		for _, key := range schema["required"].([]string) {
			if _, ok := object[key]; !ok {
				return fmt.Errorf("missing field %q", key)
			}
		}
		for key, child := range object {
			property, exists := properties[key]
			if !exists {
				return fmt.Errorf("extra field %q", key)
			}
			if err := checkReferencedSchemaValue(property.(map[string]any), child); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("not an array")
		}
		if lower, ok := schema["minItems"].(int); ok && len(array) < lower {
			return fmt.Errorf("too few items")
		}
		if upper, ok := schema["maxItems"].(int); ok && len(array) > upper {
			return fmt.Errorf("too many items")
		}
		for i, item := range array {
			if err := checkReferencedSchemaValue(schema["items"].(map[string]any), item); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("not a string")
		}
		if values, ok := schema["enum"].([]string); ok && !member(s, values...) {
			return fmt.Errorf("not an enum member")
		}
	case "integer":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return fmt.Errorf("not an integer")
		}
		if lower, ok := schema["minimum"].(int); ok && n < float64(lower) {
			return fmt.Errorf("below minimum")
		}
		if upper, ok := schema["maximum"].(int); ok && n > float64(upper) {
			return fmt.Errorf("above maximum")
		}
	default:
		return fmt.Errorf("unhandled schema type %v", schema["type"])
	}
	return nil
}

func checkReferencedSchemaJSON(schema map[string]any, raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return checkReferencedSchemaValue(schema, value)
}

func TestReferencedProviderSchemaFixtures(t *testing.T) {
	for _, tc := range referencedSyntheticCases() {
		t.Run(tc.id, func(t *testing.T) {
			request, _, err := prepareReferencedGenerateRequest(retainedTypedSelection(t, tc.id).OriginalRequest)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkReferencedSchemaJSON(request.OutputSchema, syntheticReferencedIntent(t, tc.facts...)); err != nil {
				t.Fatal("synthetic extraction cannot be emitted under the request schema:", err)
			}
		})
	}
	base := `{"kind":"sensor","value":"BMP280","state":"required","sources":[0],"quantities":[]}`
	for name, fact := range map[string]string{
		"missing-required":     strings.Replace(base, `,"quantities":[]`, "", 1),
		"invented-magnitude":   strings.Replace(base, `"kind":`, `"number":100,"kind":`, 1),
		"copied-quote":         strings.Replace(base, `"kind":`, `"quote":"BMP280","kind":`, 1),
		"invented-sensor":      strings.Replace(base, "BMP280", "SHT99", 1),
		"unknown-state":        strings.Replace(base, "required", "preferred", 1),
		"no-source":            strings.Replace(base, `"sources":[0]`, `"sources":[]`, 1),
		"foreign-source":       strings.Replace(base, `"sources":[0]`, `"sources":[1]`, 1),
		"fractional-source":    strings.Replace(base, `"sources":[0]`, `"sources":[0.5]`, 1),
		"negative-source":      strings.Replace(base, `"sources":[0]`, `"sources":[-1]`, 1),
		"string-source":        strings.Replace(base, `"sources":[0]`, `"sources":["0"]`, 1),
		"null-quantities":      strings.Replace(base, `"quantities":[]`, `"quantities":null`, 1),
		"quantity-on-identity": strings.Replace(base, `"quantities":[]`, `"quantities":[0]`, 1),
		"invented-default":     `{"kind":"number","value":"clock_hz","state":"required","sources":[0],"quantities":[0]}`,
		"definite-unclear":     `{"kind":"unclear","detail":"Which sensor?","state":"required","sources":[0],"quantities":[]}`,
		"missing-detail":       `{"kind":"other","state":"required","sources":[0],"quantities":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := []byte(`{"version":"` + ReferenceIntentVersion + `","facts":[` + fact + `]}`)
			if err := checkReferencedSchemaJSON(referencedIntentSchema(1, 0), raw); err == nil {
				t.Fatal("schema allowed forbidden extraction")
			}
		})
	}
	for _, count := range []int{0, 1, 64, 65} {
		facts := make([]ReferencedFact, count)
		for i := range facts {
			facts[i] = referencedChoice("sensor", "BMP280", "required", 0)
		}
		err := checkReferencedSchemaJSON(referencedIntentSchema(1, 0), syntheticReferencedIntent(t, facts...))
		if (err == nil) != (count <= 64) {
			t.Fatalf("incorrect fact limit at %d: %v", count, err)
		}
	}
}
