package behavioralintent

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
)

var rawMessageType = reflect.TypeOf(json.RawMessage{})

// ProposalSchema returns a fresh strict JSON schema for untrusted provider
// output. Semantic vocabulary and engineering limits remain authoritative in
// Compile and architecturesearch.Validate rather than being trusted to JSON
// schema validation alone.
func ProposalSchema() map[string]any {
	schema := schemaForType(reflect.TypeOf(Proposal{}))
	properties := schema["properties"].(map[string]any)
	properties["version"] = map[string]any{"type": "integer", "const": ProposalVersion}
	requirement := properties["requirement"].(map[string]any)
	branches := requirement["anyOf"].([]any)
	requirementObject := branches[0].(map[string]any)
	requirementProperties := requirementObject["properties"].(map[string]any)
	requirementProperties["schema"] = map[string]any{"type": "string", "const": architecturesearch.SchemaIDV3}
	requirementProperties["version"] = map[string]any{"type": "integer", "const": architecturesearch.VersionV3}
	return schema
}

func schemaForType(value reflect.Type) map[string]any {
	return schemaForTypeActive(value, map[reflect.Type]bool{})
}

func schemaForTypeActive(value reflect.Type, active map[reflect.Type]bool) map[string]any {
	if value == rawMessageType {
		scalar := []any{
			map[string]any{"type": "string"}, map[string]any{"type": "number"}, map[string]any{"type": "boolean"},
		}
		return map[string]any{"anyOf": append(slices.Clone(scalar), map[string]any{"type": "array", "items": map[string]any{"anyOf": scalar}})}
	}
	if value.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{schemaForTypeActive(value.Elem(), active), map[string]any{"type": "null"}}}
	}
	if active[value] {
		// A recursive provider type cannot be represented by the bounded strict
		// schema. Reject that branch instead of recursing without limit.
		return map[string]any{"not": map[string]any{}}
	}
	track := value.Kind() == reflect.Struct || value.Kind() == reflect.Slice || value.Kind() == reflect.Array
	if track {
		active[value] = true
		defer delete(active, value)
	}
	switch value.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			properties[name] = schemaForTypeActive(field.Type, active)
		}
		return strictSchemaObject(properties)
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaForTypeActive(value.Elem(), active)}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{}
	}
}

func strictSchemaObject(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	slices.Sort(required)
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
