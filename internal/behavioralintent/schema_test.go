package behavioralintent

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
)

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
