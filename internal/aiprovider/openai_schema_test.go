package aiprovider

import (
	"reflect"
	"sort"
	"testing"

	"kicadai/internal/intentplanner"
)

func TestOpenAIIntentSchemaObjectsAreStrictAndFullyRequired(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"bmp280":        BMP280ReferenceIntentEnvelopeSchema(),
		"protected_led": ProtectedLEDReferenceIntentEnvelopeSchema(),
	} {
		t.Run(name, func(t *testing.T) {
			assertStrictSchemaNode(t, "schema", schema)
		})
	}
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

func TestProtectedLEDReferenceSchemaPinsProvenDesign(t *testing.T) {
	schema := ProtectedLEDReferenceIntentEnvelopeSchema()
	board := schemaProperties(t, schemaNodeAt(t, schema, "intent", "board"))
	if got := schemaConst(t, schemaProperty(t, board, "width_mm")); got != 50.0 {
		t.Fatalf("board width = %#v, want 50", got)
	}
	if got := schemaConst(t, schemaProperty(t, board, "height_mm")); got != 30.0 {
		t.Fatalf("board height = %#v, want 30", got)
	}
	function := schemaProperties(t, schemaItems(t, schemaProperty(t, schemaProperties(t, schemaNodeAt(t, schema, "intent")), "functions")))
	params := schemaProperties(t, schemaProperty(t, function, "params"))
	for name, want := range map[string]any{
		"active_high": true, "supply_voltage": "5V", "led_forward_voltage": "2.0V", "led_current_ma": 5,
	} {
		if got := schemaConst(t, schemaProperty(t, params, name)); got != want {
			t.Fatalf("function params.%s = %#v, want %#v", name, got, want)
		}
	}
	protection := schemaProperties(t, schemaNodeAt(t, schema, "intent", "protection"))
	for _, name := range []string{"overcurrent", "transient", "bulk_capacitance"} {
		if got := schemaConst(t, schemaProperty(t, protection, name)); got != "required" {
			t.Fatalf("protection.%s = %#v, want required", name, got)
		}
	}
}

func TestOpenAIIntentTopLevelSchemaIsSubsetOfGoIntentModel(t *testing.T) {
	for profile, schema := range map[string]map[string]any{
		"bmp280": BMP280ReferenceIntentEnvelopeSchema(),
		"led":    ProtectedLEDReferenceIntentEnvelopeSchema(),
	} {
		t.Run(profile, func(t *testing.T) {
			properties := schema["properties"].(map[string]any)
			intent := properties["intent"].(map[string]any)
			intentProperties := intent["properties"].(map[string]any)
			goFields := jsonFieldNames(reflect.TypeOf(intentplanner.Request{}))
			for name := range intentProperties {
				if !goFields[name] {
					t.Fatalf("provider intent property %q has no intentplanner.Request field", name)
				}
			}
		})
	}
}

func TestOpenAIReferenceSchemaPinsSupportedProtectionPolicy(t *testing.T) {
	schema := BMP280ReferenceIntentEnvelopeSchema()
	protection := schemaProperties(t, schemaNodeAt(t, schema, "intent", "protection"))
	want := map[string]string{
		"esd":              "optional",
		"reverse_polarity": "optional",
		"overcurrent":      "required",
		"transient":        "required",
		"bulk_capacitance": "required",
	}
	for name, value := range want {
		if got := schemaConst(t, schemaProperty(t, protection, name)); got != value {
			t.Fatalf("protection.%s const = %#v, want %q", name, got, value)
		}
	}
}

func TestOpenAIReferenceSchemaPinsProvenBoardGeometry(t *testing.T) {
	schema := BMP280ReferenceIntentEnvelopeSchema()
	board := schemaProperties(t, schemaNodeAt(t, schema, "intent", "board"))
	want := map[string]any{
		"width_mm":          100.0,
		"height_mm":         75.0,
		"edge_clearance_mm": 0.25,
		"layers":            2,
		"mounting_holes":    "optional",
	}
	for name, value := range want {
		if got := schemaConst(t, schemaProperty(t, board, name)); got != value {
			t.Fatalf("board.%s const = %#v, want %#v", name, got, value)
		}
	}
}

func TestOpenAIReferenceSchemaPinsProvenCurrentRequirements(t *testing.T) {
	schema := BMP280ReferenceIntentEnvelopeSchema()
	power := schemaProperties(t, schemaNodeAt(t, schema, "intent", "power"))
	input := schemaProperties(t, schemaItems(t, schemaProperty(t, power, "inputs")))
	rail := schemaProperties(t, schemaItems(t, schemaProperty(t, power, "rails")))
	if got := schemaConst(t, schemaProperty(t, input, "current_ma")); got != 500 {
		t.Fatalf("input current const = %#v, want 500", got)
	}
	if got := schemaConst(t, schemaProperty(t, rail, "current_ma")); got != 100 {
		t.Fatalf("rail current const = %#v, want 100", got)
	}
}

func schemaNodeAt(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	node := root
	for _, name := range path {
		node = schemaProperty(t, schemaProperties(t, node), name)
	}
	return node
}

func schemaProperties(t *testing.T, node map[string]any) map[string]any {
	t.Helper()
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema node properties = %#v", node["properties"])
	}
	return properties
}

func schemaProperty(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()
	node, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %#v", name, properties[name])
	}
	return node
}

func schemaItems(t *testing.T, node map[string]any) map[string]any {
	t.Helper()
	items, ok := node["items"].(map[string]any)
	if !ok {
		t.Fatalf("schema items = %#v", node["items"])
	}
	return items
}

func schemaConst(t *testing.T, node map[string]any) any {
	t.Helper()
	value, ok := node["const"]
	if !ok {
		t.Fatalf("schema const missing from %#v", node)
	}
	return value
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
