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

func TestProviderGraphSchemaConstrainsSchematicTransforms(t *testing.T) {
	schema := ProviderGraphSchema()
	placements := schema["properties"].(map[string]any)["schematic"].(map[string]any)["properties"].(map[string]any)["placements"].(map[string]any)
	properties := placements["items"].(map[string]any)["properties"].(map[string]any)
	if got := properties["orientation"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"normal", "rotated_90", "rotated_180", "rotated_270"}) {
		t.Fatalf("orientation enum = %#v", got)
	}
	if got := properties["mirror"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"", "none", "x", "y"}) {
		t.Fatalf("mirror enum = %#v", got)
	}
}

func TestProviderGraphSchemaConstrainsRoutingNetClasses(t *testing.T) {
	schema := ProviderGraphSchema()
	nets := schema["properties"].(map[string]any)["nets"].(map[string]any)
	properties := nets["items"].(map[string]any)["properties"].(map[string]any)
	if got := properties["net_class"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"", "signal", "clock", "power", "ground"}) {
		t.Fatalf("net class enum = %#v", got)
	}
}

func TestProviderGraphSchemaConstrainsPowerFlags(t *testing.T) {
	schema := ProviderGraphSchema()
	flags := schema["properties"].(map[string]any)["power_flags"].(map[string]any)
	if got := flags["maxItems"]; got != MaxPowerFlags {
		t.Fatalf("power flag maxItems = %#v, want %d", got, MaxPowerFlags)
	}
	properties := flags["items"].(map[string]any)["properties"].(map[string]any)
	if _, exists := properties["net"]; !exists || len(properties) != 1 {
		t.Fatalf("power flag properties = %#v", properties)
	}
}

func TestProviderGraphSchemaRequiresUsableBoardAndPCBLayout(t *testing.T) {
	properties := ProviderGraphSchema()["properties"].(map[string]any)
	project := properties["project"].(map[string]any)["properties"].(map[string]any)
	board := project["board"].(map[string]any)["properties"].(map[string]any)
	for _, field := range []string{"width_mm", "height_mm"} {
		constraint := board[field].(map[string]any)
		if constraint["exclusiveMinimum"] != 0 || constraint["maximum"] != MaxBoardDimensionMM {
			t.Fatalf("board %s constraint = %#v", field, constraint)
		}
	}
	if constraint := board["edge_clearance_mm"].(map[string]any); constraint["minimum"] != 0 || constraint["maximum"] != MaxBoardDimensionMM {
		t.Fatalf("board edge clearance constraint = %#v", constraint)
	}
	pcb := properties["pcb"].(map[string]any)["properties"].(map[string]any)
	regionBounds := pcb["regions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["bounds"].(map[string]any)["properties"].(map[string]any)
	if regionBounds["width_mm"].(map[string]any)["exclusiveMinimum"] != 0 || regionBounds["x_mm"].(map[string]any)["minimum"] != 0 {
		t.Fatalf("region bounds constraints = %#v", regionBounds)
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
