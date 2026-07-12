package circuitgraph

import (
	"reflect"
	"sort"
	"testing"
)

func TestProviderGraphSchemaIsStrictAndFullyRequired(t *testing.T) {
	assertStrictGraphSchema(t, "graph", ProviderGraphSchema())
}

func assertStrictGraphSchema(t *testing.T, path string, node any) {
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
		want := make([]string, 0, len(properties))
		for name := range properties {
			want = append(want, name)
		}
		sort.Strings(want)
		got, ok := object["required"].([]string)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s required = %#v, want %#v", path, object["required"], want)
		}
		for name, property := range properties {
			assertStrictGraphSchema(t, path+"."+name, property)
		}
	}
	if items, exists := object["items"]; exists {
		assertStrictGraphSchema(t, path+"[]", items)
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		if alternatives, ok := object[keyword].([]any); ok {
			for index, alternative := range alternatives {
				assertStrictGraphSchema(t, path+"."+keyword, alternative)
				_ = index
			}
		}
	}
}

func TestProviderGraphSchemaTopLevelMatchesDocument(t *testing.T) {
	properties := ProviderGraphSchema()["properties"].(map[string]any)
	fields := jsonFieldNames(reflect.TypeOf(Document{}))
	for name := range properties {
		if !fields[name] {
			t.Fatalf("schema property %q has no Document field", name)
		}
	}
}

func jsonFieldNames(typ reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for index := 0; index < typ.NumField(); index++ {
		tag := typ.Field(index).Tag.Get("json")
		for end, char := range tag {
			if char == ',' {
				tag = tag[:end]
				break
			}
		}
		if tag != "" && tag != "-" {
			fields[tag] = true
		}
	}
	return fields
}

func TestProviderGraphSchemaReturnsFreshValue(t *testing.T) {
	first := ProviderGraphSchema()
	second := ProviderGraphSchema()
	first["mutated"] = true
	if _, exists := second["mutated"]; exists {
		t.Fatal("ProviderGraphSchema reused mutable map")
	}
}
