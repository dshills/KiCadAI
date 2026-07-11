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
	if request.SchematicLayout == nil {
		t.Fatal("BMP280 fixture is missing schematic_layout intent")
	}
	request.Name = "renamed_layout_acceptance"
	plan := PlanBlocks(context.Background(), blocks.NewBuiltinRegistry(), request)
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		t.Fatalf("plan BMP280 fixture: %#v", plan.Stage.Issues)
	}
	first, firstPaper, err := layoutSchematicOperations(plan.Output, *request.SchematicLayout)
	if err != nil {
		t.Fatal(err)
	}
	plan.Output.ProjectName = "another_name"
	second, secondPaper, err := layoutSchematicOperations(plan.Output, *request.SchematicLayout)
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
