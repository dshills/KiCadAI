package circuitgraph

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/simmodel"
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
	fields := jsonFieldNames(reflect.TypeOf(Document{}))
	for branchName, branch := range providerGraphSchemaBranches(t) {
		properties := branch["properties"].(map[string]any)
		for name := range properties {
			if !fields[name] {
				t.Fatalf("%s schema property %q has no Document field", branchName, name)
			}
		}
	}
}

func TestProviderGraphSchemaTransientTrustBoundary(t *testing.T) {
	properties := providerGraphSchemaBranches(t)["explicit"]["properties"].(map[string]any)
	nullableSimulation := properties["simulation"].(map[string]any)
	var simulation map[string]any
	for _, option := range nullableSimulation["anyOf"].([]any) {
		candidate := option.(map[string]any)
		if _, exists := candidate["oneOf"]; exists {
			simulation = candidate
			break
		}
	}
	if simulation == nil {
		t.Fatal("nullable simulation schema lacks its non-null oneOf branch")
	}
	branches := simulation["oneOf"].([]any)
	var transient map[string]any
	for _, branch := range branches {
		candidate := branch.(map[string]any)
		model := candidate["properties"].(map[string]any)["model_id"].(map[string]any)
		if model["const"] == simmodel.ModelTransientCircuitV1 {
			transient = candidate
			break
		}
	}
	if transient == nil {
		t.Fatal("transient simulation schema branch is missing")
	}
	data, err := json.Marshal(transient)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, required := range []string{"duration_s", "time_step_s", "pulse_initial_value", "pulse_value", "rise_time_s", "fall_time_s"} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("transient schema lacks %q", required)
		}
	}
	for _, forbidden := range []string{"equation", "matrix", "integration_method", "initial_conditions", "topology", "model_file", "max_iterations"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("transient provider schema exposes forbidden field %q", forbidden)
		}
	}
}

func TestProviderGraphSchemaConstrainsSchematicTransforms(t *testing.T) {
	schema := providerGraphSchemaBranches(t)["explicit"]
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
	schema := providerGraphSchemaBranches(t)["explicit"]
	nets := schema["properties"].(map[string]any)["nets"].(map[string]any)
	properties := nets["items"].(map[string]any)["properties"].(map[string]any)
	if got := properties["net_class"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"", "signal", "clock", "power", "ground"}) {
		t.Fatalf("net class enum = %#v", got)
	}
}

func TestProviderGraphSchemaConstrainsPowerFlags(t *testing.T) {
	schema := providerGraphSchemaBranches(t)["explicit"]
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
	properties := providerGraphSchemaBranches(t)["explicit"]["properties"].(map[string]any)
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

func TestProviderGraphSchemaFunctionFormExcludesImplementationDetails(t *testing.T) {
	properties := providerGraphSchemaBranches(t)["function"]["properties"].(map[string]any)
	if _, exists := properties["synthesis"]; !exists {
		t.Fatal("function schema lacks synthesis intent")
	}
	synthesis := properties["synthesis"].(map[string]any)["properties"].(map[string]any)
	interfaceItem := synthesis["interfaces"].(map[string]any)["items"].(map[string]any)
	signals := interfaceItem["properties"].(map[string]any)["signals"].(map[string]any)
	if signals["maxItems"] != MaxFunctionInterfaceSignals {
		t.Fatalf("function interface signal limit = %#v", signals["maxItems"])
	}
	for _, forbidden := range []string{"components", "nets", "no_connects", "power_flags", "buses", "schematic", "pcb", "simulation"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("function schema exposes explicit graph field %q", forbidden)
		}
	}
	encoded, err := json.Marshal(properties["synthesis"])
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"symbol_pin", "footprint", "pad", "x_mm", "y_mm", "layers", "routes", "block_id"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("function schema exposes implementation detail %q", forbidden)
		}
	}
}

func providerGraphSchemaBranches(t *testing.T) map[string]map[string]any {
	t.Helper()
	result := map[string]map[string]any{}
	branches, ok := ProviderGraphSchema()["oneOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("provider graph schema branches = %#v", ProviderGraphSchema()["oneOf"])
	}
	for _, raw := range branches {
		branch := raw.(map[string]any)
		properties := branch["properties"].(map[string]any)
		name := "explicit"
		if _, exists := properties["synthesis"]; exists {
			name = "function"
		}
		result[name] = branch
	}
	if result["explicit"] == nil || result["function"] == nil {
		t.Fatalf("provider graph schema branches are incomplete: %#v", result)
	}
	return result
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
