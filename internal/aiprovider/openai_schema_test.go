package aiprovider

import (
	"reflect"
	"sort"
	"testing"

	"kicadai/internal/intentplanner"
)

func TestOpenAIIntentSchemaObjectsAreStrictAndFullyRequired(t *testing.T) {
	assertStrictSchemaNode(t, "schema", BMP280ReferenceIntentEnvelopeSchema())
}

func assertStrictSchemaNode(t *testing.T, path string, node any) {
	t.Helper()
	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	if object["type"] == "object" {
		if object["additionalProperties"] != false {
			t.Fatalf("%s additionalProperties = %#v, want false", path, object["additionalProperties"])
		}
		properties, ok := object["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s properties = %#v", path, object["properties"])
		}
		var want []string
		for name := range properties {
			want = append(want, name)
		}
		sort.Strings(want)
		got, ok := object["required"].([]string)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s required = %#v, want %#v", path, object["required"], want)
		}
		for name, property := range properties {
			assertStrictSchemaNode(t, path+"."+name, property)
		}
	}
	if items, ok := object["items"]; ok {
		assertStrictSchemaNode(t, path+"[]", items)
	}
}

func TestOpenAIIntentTopLevelSchemaIsSubsetOfGoIntentModel(t *testing.T) {
	schema := BMP280ReferenceIntentEnvelopeSchema()
	properties := schema["properties"].(map[string]any)
	intent := properties["intent"].(map[string]any)
	intentProperties := intent["properties"].(map[string]any)
	goFields := jsonFieldNames(reflect.TypeOf(intentplanner.Request{}))
	for name := range intentProperties {
		if !goFields[name] {
			t.Fatalf("provider intent property %q has no intentplanner.Request field", name)
		}
	}
}

func jsonFieldNames(value reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for index := 0; index < value.NumField(); index++ {
		tag := value.Field(index).Tag.Get("json")
		for offset, char := range tag {
			if char == ',' {
				tag = tag[:offset]
				break
			}
		}
		if tag != "" && tag != "-" {
			fields[tag] = true
		}
	}
	return fields
}
