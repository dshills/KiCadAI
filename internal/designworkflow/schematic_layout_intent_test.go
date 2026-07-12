package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

func TestBMP280LayoutIntentIsNameIndependentAndDeterministic(t *testing.T) {
	file, err := os.Open("../../examples/design/kicad-backed/sensor_bmp280_breakout.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode BMP280 fixture: %#v", issues)
	}
	if request.SchematicLayout != nil {
		t.Fatal("BMP280 fixture must not carry hand-authored schematic_layout intent")
	}
	if !request.AutoSchematicLayout {
		t.Fatal("BMP280 fixture must opt into automatic schematic layout")
	}
	request.Name = "renamed_layout_acceptance"
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan BMP280 fixture: %#v", plan.Stage.Issues)
	}
	if plan.Request.SchematicLayout == nil {
		t.Fatal("block planning did not synthesize schematic layout intent")
	}
	first, firstPaper, err := layoutSchematicOperations(plan.Output, *plan.Request.SchematicLayout)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := request
	secondRequest.Name = "another_name"
	secondPlan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), secondRequest)
	if reports.HasBlockingIssue(secondPlan.Stage.Issues) {
		t.Fatalf("repeat plan BMP280 fixture: %#v", secondPlan.Stage.Issues)
	}
	if !reflect.DeepEqual(*plan.Request.SchematicLayout, *secondPlan.Request.SchematicLayout) {
		t.Fatal("inferred schematic layout changed with project name or repeated planning")
	}
	second, secondPaper, err := layoutSchematicOperations(secondPlan.Output, *secondPlan.Request.SchematicLayout)
	if err != nil {
		t.Fatal(err)
	}
	if firstPaper != "A4" || secondPaper != firstPaper {
		t.Fatalf("layout papers = %q and %q, want deterministic A4", firstPaper, secondPaper)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("schematic layout operations changed with project name or repeated generation")
	}

	positions := map[string]transactions.Point{}
	for index, operation := range first {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode add_symbol %d: %v", index, err)
		}
		positions[payload.Role] = payload.At
		standard := map[string]transactions.SymbolProperty{}
		for _, property := range payload.Properties {
			if property.Name == "Reference" || property.Name == "Value" {
				standard[property.Name] = property
			}
		}
		for _, name := range []string{"Reference", "Value"} {
			property, ok := standard[name]
			if !ok || property.At == nil || *property.At == payload.At {
				t.Fatalf("%s %s property is not explicitly separated from its body: %#v", payload.Ref, name, property)
			}
		}
	}
	for _, pair := range [][2]string{{"usb_c_receptacle", "regulator"}, {"regulator", "sensor"}, {"sensor", "connector"}} {
		if positions[pair[0]].XMM >= positions[pair[1]].XMM {
			t.Fatalf("flow %s -> %s is not left-to-right: %#v", pair[0], pair[1], positions)
		}
	}
	for _, role := range []string{"decoupling_capacitor", "sda_pullup", "scl_pullup"} {
		if positions[role].YMM >= positions["sensor"].YMM {
			t.Fatalf("%s is not above sensor: %#v", role, positions)
		}
	}
	minX, maxX := positions["usb_c_receptacle"].XMM, positions["usb_c_receptacle"].XMM
	minY, maxY := positions["usb_c_receptacle"].YMM, positions["usb_c_receptacle"].YMM
	for _, point := range positions {
		if point.XMM < minX {
			minX = point.XMM
		}
		if point.XMM > maxX {
			maxX = point.XMM
		}
		if point.YMM < minY {
			minY = point.YMM
		}
		if point.YMM > maxY {
			maxY = point.YMM
		}
	}
	if center := (minX + maxX) / 2; center < 138.5 || center > 158.5 {
		t.Fatalf("horizontal layout center = %.2f mm, want near A4 center", center)
	}
	if center := (minY + maxY) / 2; center < 95 || center > 115 {
		t.Fatalf("vertical layout center = %.2f mm, want near A4 center", center)
	}
}

func TestProtectedUSBCLEDLayoutInferenceIsDeterministicAndReadable(t *testing.T) {
	file, err := os.Open("../../examples/design/kicad-backed/usb_c_led_indicator_protected.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode protected USB-C LED fixture: %#v", issues)
	}
	if request.SchematicLayout != nil {
		t.Fatal("protected USB-C LED fixture must not carry hand-authored schematic_layout intent")
	}
	if !request.AutoSchematicLayout {
		t.Fatal("protected USB-C LED fixture must opt into automatic schematic layout")
	}

	request.Name = "renamed_protected_indicator"
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan protected USB-C LED fixture: %#v", plan.Stage.Issues)
	}
	if plan.Request.SchematicLayout == nil {
		t.Fatal("block planning did not synthesize protected USB-C LED layout intent")
	}
	placements := map[string]schematicir.Placement{}
	for _, placement := range plan.Request.SchematicLayout.Placements {
		placements[placement.Target] = placement
	}
	for target, relation := range map[string]string{
		"usb_power__vbus_fuse": "usb_power__usb_c_receptacle",
		"indicator__resistor":  "usb_power__vbus_fuse",
		"indicator__led":       "indicator__resistor",
	} {
		if !containsLayoutTarget(placements[target].RightOf, relation) {
			t.Fatalf("%s right_of = %#v, want %s", target, placements[target].RightOf, relation)
		}
	}
	for _, target := range []string{"usb_power__cc1_rd", "usb_power__cc2_rd"} {
		if !containsLayoutTarget(placements["usb_power__usb_c_receptacle"].Above, target) {
			t.Fatalf("USB-C receptacle above = %#v, want %s below", placements["usb_power__usb_c_receptacle"].Above, target)
		}
	}
	for _, target := range []string{"usb_power__vbus_tvs", "usb_power__bulk_capacitor"} {
		if !containsLayoutTarget(placements["usb_power__vbus_fuse"].Above, target) {
			t.Fatalf("fuse above = %#v, want %s below", placements["usb_power__vbus_fuse"].Above, target)
		}
		if !containsLayoutTarget(placements[target].Near, "usb_power__vbus_fuse") {
			t.Fatalf("%s near = %#v, want fuse", target, placements[target].Near)
		}
	}

	secondRequest := request
	secondRequest.Name = "another_protected_indicator_name"
	secondPlan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), secondRequest)
	if reports.HasBlockingIssue(secondPlan.Stage.Issues) {
		t.Fatalf("repeat plan protected USB-C LED fixture: %#v", secondPlan.Stage.Issues)
	}
	if !reflect.DeepEqual(*plan.Request.SchematicLayout, *secondPlan.Request.SchematicLayout) {
		t.Fatal("protected USB-C LED inferred layout changed with project name or repeated planning")
	}

	operations, paper, err := layoutSchematicOperations(plan.Output, *plan.Request.SchematicLayout)
	if err != nil {
		t.Fatal(err)
	}
	if paper != "A4" {
		t.Fatalf("selected paper = %q, want A4", paper)
	}
	secondOperations, secondPaper, err := layoutSchematicOperations(secondPlan.Output, *secondPlan.Request.SchematicLayout)
	if err != nil {
		t.Fatal(err)
	}
	if secondPaper != paper || !reflect.DeepEqual(operations, secondOperations) {
		t.Fatal("protected USB-C LED schematic transactions changed with project name or repeated planning")
	}
	positions := map[string]transactions.Point{}
	for index, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode add_symbol %d: %v", index, err)
		}
		positions[payload.Role] = payload.At
		for _, property := range payload.Properties {
			if property.Name != "Reference" && property.Name != "Value" {
				continue
			}
			if property.At == nil || *property.At == payload.At {
				t.Fatalf("%s %s property is not explicitly separated from its body", payload.Ref, property.Name)
			}
		}
	}
	for _, pair := range [][2]string{
		{"usb_c_receptacle", "vbus_fuse"},
		{"vbus_fuse", "resistor"},
		{"resistor", "led"},
	} {
		if positions[pair[0]].XMM >= positions[pair[1]].XMM {
			t.Fatalf("flow %s -> %s is not left-to-right: %#v", pair[0], pair[1], positions)
		}
	}
	for _, role := range []string{"cc1_rd", "cc2_rd"} {
		if positions[role].YMM <= positions["usb_c_receptacle"].YMM {
			t.Fatalf("%s is not below USB-C receptacle: %#v", role, positions)
		}
	}
	for _, role := range []string{"vbus_tvs", "bulk_capacitor"} {
		if positions[role].YMM <= positions["vbus_fuse"].YMM {
			t.Fatalf("%s is not below fuse: %#v", role, positions)
		}
	}
}

func containsLayoutTarget(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestSchematicLayoutComponentRoleRecognizesLEDToken(t *testing.T) {
	for _, role := range []string{"led", "power_led", "status-led", "led.red", "indicator_led"} {
		if got := schematicLayoutComponentRole(role); got != schematicir.ComponentRoleIndicatorLED {
			t.Errorf("schematicLayoutComponentRole(%q) = %q, want %q", role, got, schematicir.ComponentRoleIndicatorLED)
		}
	}
	if got := schematicLayoutComponentRole("led_driver"); got == schematicir.ComponentRoleIndicatorLED {
		t.Fatalf("schematicLayoutComponentRole(led_driver) = %q, want a non-LED role", got)
	}
}

func TestSchematicLayoutComponentValueNormalizesOnlyPlainDecimalResistance(t *testing.T) {
	for value, want := range map[string]string{
		"600":      "600R",
		" 0.5 ":    "0.5R",
		"1e3":      "1e3",
		"NaN":      "NaN",
		"Infinity": "Infinity",
		"-10":      "-10",
	} {
		if got := schematicLayoutComponentValue(schematicir.ComponentRoleResistor, value); got != want {
			t.Errorf("schematicLayoutComponentValue(resistor, %q) = %q, want %q", value, got, want)
		}
	}
}

func TestAutomaticSchematicLayoutFailsClosedOnDisconnectedStageTopology(t *testing.T) {
	file, err := os.Open("../../examples/design/kicad-backed/sensor_bmp280_breakout.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode BMP280 fixture: %#v", issues)
	}
	filtered := request.Connections[:0]
	for _, connection := range request.Connections {
		if connection.From == "usb_power.VBUS_OUT" && connection.To == "regulator.VIN" {
			continue
		}
		filtered = append(filtered, connection)
	}
	request.Connections = filtered
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	for _, issue := range plan.Stage.Issues {
		if issue.Path == "schematic_layout.inference.topology" && issue.Severity == reports.SeverityBlocked {
			return
		}
	}
	t.Fatalf("planning issues = %#v, want blocked topology inference diagnostic", plan.Stage.Issues)
}

func TestAutomaticSchematicLayoutFailsClosedOnAmbiguousParallelStages(t *testing.T) {
	_, issues := inferredSchematicInstances([]schematicir.Component{
		{ID: "sensor_a__sensor", Role: schematicir.ComponentRoleSensor},
		{ID: "sensor_b__sensor", Role: schematicir.ComponentRoleSensor},
	})
	for _, issue := range issues {
		if issue.Path == "schematic_layout.inference.instances.sensor_b" && issue.Severity == reports.SeverityBlocked {
			return
		}
	}
	t.Fatalf("inference issues = %#v, want ambiguous parallel-stage diagnostic", issues)
}

func TestValidateRequestRejectsUnknownSchematicLayoutRelationTarget(t *testing.T) {
	layout := schematicir.NewDocument().Layout
	layout.Groups = []schematicir.Group{{ID: "input", Members: []string{"source__connector"}}}
	layout.Placements = []schematicir.Placement{{Target: "source__connector", Group: "input", RightOf: []string{"missing__sensor"}}}
	request := validRequest()
	request.Blocks = []BlockInstanceSpec{{ID: "source", BlockID: "connector_breakout"}}
	request.SchematicLayout = &layout
	issues := ValidateRequest(request)
	found := false
	for _, issue := range issues {
		if issue.Path == "schematic_layout.placements[0].right_of[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want unknown right_of target", issues)
	}
}

func TestValidateRequestRejectsSchematicLayoutRelationCycle(t *testing.T) {
	layout := schematicir.NewDocument().Layout
	layout.Groups = []schematicir.Group{{ID: "signal", Members: []string{"source__connector", "sink__connector"}}}
	layout.Placements = []schematicir.Placement{
		{Target: "source__connector", Group: "signal", RightOf: []string{"sink__connector"}},
		{Target: "sink__connector", Group: "signal", RightOf: []string{"source__connector"}},
	}
	request := validRequest()
	request.Blocks = []BlockInstanceSpec{{ID: "source", BlockID: "connector_breakout"}, {ID: "sink", BlockID: "connector_breakout"}}
	request.SchematicLayout = &layout
	issues := ValidateRequest(request)
	found := false
	for _, issue := range issues {
		if issue.Path == "schematic_layout.placements" && strings.Contains(issue.Message, "right_of relation contains a cycle") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want right_of cycle", issues)
	}
}

func TestValidateRequestRejectsReservedSchematicLayoutDelimiter(t *testing.T) {
	layout := schematicir.NewDocument().Layout
	layout.Groups = []schematicir.Group{{ID: "signal", Members: []string{"source__nested__connector"}}}
	layout.Placements = []schematicir.Placement{{Target: "source__nested__connector", Group: "signal"}}
	request := validRequest()
	request.Blocks = []BlockInstanceSpec{{ID: "source__nested", BlockID: "connector_breakout"}}
	request.SchematicLayout = &layout
	issues := ValidateRequest(request)
	found := false
	for _, issue := range issues {
		if issue.Path == "blocks[0].id" && strings.Contains(issue.Message, "reserved schematic layout delimiter") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want reserved delimiter error", issues)
	}
}

func TestSchematicLayoutPointConversionUsesKiCadIU(t *testing.T) {
	point := schematiclayoutPointToTransaction(kicadfiles.MM(12.7), kicadfiles.MM(25.4))
	if point != (transactions.Point{XMM: 12.7, YMM: 25.4}) {
		t.Fatalf("point = %#v", point)
	}
}
