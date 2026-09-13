package behavioralintent

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
)

func TestProposalSchemaFitsStrictProviderStructureBudget(t *testing.T) {
	schema := ProposalSchema()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	properties, enums, characters, maximumDepth := 0, 0, 0, 0
	var visit func(map[string]any, int)
	visit = func(node map[string]any, depth int) {
		if node["type"] == "object" || node["type"] == "array" {
			depth++
		}
		maximumDepth = max(maximumDepth, depth)
		for _, banned := range []string{"not", "oneOf", "allOf", "if", "then", "else", "dependentRequired", "dependentSchemas"} {
			if _, ok := node[banned]; ok {
				t.Fatalf("unsupported provider keyword %s", banned)
			}
		}
		if values, ok := node["enum"].([]string); ok {
			enums += len(values)
			for _, value := range values {
				characters += len(value)
			}
		}
		if value, ok := node["const"]; ok {
			if reflect.TypeOf(value).Kind() == reflect.String {
				characters += reflect.ValueOf(value).Len()
			}
		}
		if fields, ok := node["properties"].(map[string]any); ok {
			properties += len(fields)
			for name, child := range fields {
				characters += len(name)
				visit(child.(map[string]any), depth)
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			visit(items, depth)
		}
		if branches, ok := node["anyOf"].([]any); ok {
			for _, branch := range branches {
				visit(branch.(map[string]any), depth)
			}
		}
	}
	visit(schema, 1) // Reserve one containing object for the AI intent envelope.
	if properties > 4998 || enums > 1000 || characters > 119000 || maximumDepth > 10 {
		t.Fatalf("schema exceeds provider structure budget: properties=%d enums=%d characters=%d depth=%d", properties, enums, characters, maximumDepth)
	}
	// Leave space in the former 128-KiB request ceiling for source/context.
	if len(encoded) > 112*1024 {
		t.Fatalf("schema leaves insufficient request space: %d bytes", len(encoded))
	}
	t.Logf("schema bytes=%d properties=%d enum_values=%d string_characters=%d envelope_depth=%d", len(encoded), properties, enums, characters, maximumDepth)
}

func TestProposalSchemaIsStrictFullyRequiredAndV3Only(t *testing.T) {
	schema := ProposalSchema()
	assertStrictProviderSchema(t, "proposal", schema)
	properties := schema["properties"].(map[string]any)
	if properties["version"].(map[string]any)["const"] != ProposalVersion {
		t.Fatalf("proposal version schema = %#v", properties["version"])
	}
	requirementBranches := properties["requirement"].(map[string]any)["anyOf"].([]any)
	requirementProperties := requirementBranches[0].(map[string]any)["properties"].(map[string]any)
	if requirementProperties["schema"].(map[string]any)["const"] != architecturesearch.SchemaIDV3 || requirementProperties["version"].(map[string]any)["const"] != architecturesearch.VersionV3 {
		t.Fatalf("requirement schema/version = %#v / %#v", requirementProperties["schema"], requirementProperties["version"])
	}
	first := ProposalSchema()
	first["mutated"] = true
	if _, exists := ProposalSchema()["mutated"]; exists {
		t.Fatal("proposal schema reused mutable state")
	}
}

func TestSchemaForTypeRejectsRecursiveBranchesWithoutRecursing(t *testing.T) {
	type recursive struct {
		Child *recursive `json:"child"`
	}
	schema := schemaForType(reflect.TypeOf(recursive{}))
	child := schema["properties"].(map[string]any)["child"].(map[string]any)
	branches := child["anyOf"].([]any)
	if _, ok := branches[0].(map[string]any)["not"]; !ok {
		t.Fatalf("recursive branch schema = %#v, want fail-closed not schema", branches[0])
	}
}

func assertStrictProviderSchema(t *testing.T, path string, node any) {
	t.Helper()
	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	if object["type"] == "object" {
		if object["additionalProperties"] != false {
			t.Fatalf("%s is not strict", path)
		}
		properties := object["properties"].(map[string]any)
		want := make([]string, 0, len(properties))
		for name, property := range properties {
			want = append(want, name)
			assertStrictProviderSchema(t, path+"."+name, property)
		}
		slices.Sort(want)
		if got, ok := object["required"].([]string); !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s required = %#v, want %#v", path, object["required"], want)
		}
	}
	if items, exists := object["items"]; exists {
		assertStrictProviderSchema(t, path+"[]", items)
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if branches, ok := object[keyword].([]any); ok {
			for index, branch := range branches {
				assertStrictProviderSchema(t, path+"."+keyword+string(rune('0'+index)), branch)
			}
		}
	}
}
