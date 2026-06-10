package schematic

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestWriteMinimalSchematic(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, minimalSchematic())
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	want := strings.Join([]string{
		"(kicad_sch",
		"  (version 20260306)",
		"  (generator \"eeschema\")",
		"  (generator_version \"10.0\")",
		"  (uuid \"6ba7b810-9dad-11d1-80b4-00c04fd430c8\")",
		"  (paper \"A4\")",
		"  (lib_symbols)",
		")",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("Write =\n%s\nwant =\n%s", got, want)
	}
}

func TestWriteTitleBlockEscapesStrings(t *testing.T) {
	schematic := minimalSchematic()
	schematic.TitleBlock = kicadfiles.TitleBlock{
		Title:    "LED \"Demo\"",
		Date:     "2026-06-09",
		Revision: "A",
		Company:  "KiCadAI",
		Comments: []string{"line\nbreak"},
	}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	for _, want := range []string{
		"(title \"LED \\\"Demo\\\"\")",
		"(comment 1 \"line\\nbreak\")",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestValidateRejectsMissingUUID(t *testing.T) {
	schematic := minimalSchematic()
	schematic.UUID = ""

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.uuid") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsMissingGenerator(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Generator = " "

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.generator") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsMissingGeneratorVersionForModernSchematic(t *testing.T) {
	schematic := minimalSchematic()
	schematic.GeneratorVersion = " "

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.generator_version") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteValidatesBeforeRendering(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, SchematicFile{})
	var validationErrors kicadfiles.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Write emitted output despite validation error: %q", buf.String())
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	var first bytes.Buffer
	var second bytes.Buffer
	schematic := minimalSchematic()

	if err := Write(&first, schematic); err != nil {
		t.Fatalf("first Write returned error: %v", err)
	}
	if err := Write(&second, schematic); err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("outputs differ:\n%s\n---\n%s", first.String(), second.String())
	}
}

func TestWriteOrdersItemsByKiCadKindThenUUID(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{
		{
			UUID:      kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
			LibraryID: "Device:R",
			Reference: "R2",
			Value:     "2k",
		},
		{
			UUID:      kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
			LibraryID: "Device:R",
			Reference: "R1",
			Value:     "1k",
		},
	}
	schematic.Labels = []Label{
		{
			UUID: kicadfiles.UUID("88888888-8888-4888-8888-888888888888"),
			Text: "HIER",
			Kind: LabelHierarchical,
		},
		{
			UUID: kicadfiles.UUID("77777777-7777-4777-8777-777777777777"),
			Text: "GLOBAL",
			Kind: LabelGlobal,
		},
		{
			UUID: kicadfiles.UUID("66666666-6666-4666-8666-666666666666"),
			Text: "LOCAL",
			Kind: LabelLocal,
		},
	}
	schematic.Wires = []Wire{{
		UUID: kicadfiles.UUID("55555555-5555-4555-8555-555555555555"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.Junctions = []Junction{{
		UUID:     kicadfiles.UUID("99999999-9999-4999-8999-999999999999"),
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	assertInOrder(t, output,
		"(lib_symbols)",
		"(junction",
		"(wire",
		"(label",
		"(global_label",
		"(hierarchical_label",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	)
}

func TestWriteRendersKiCadStyleSymbolDetails(t *testing.T) {
	showName := true
	doNotAutoplace := true
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:             kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		LibraryID:        "Device:R",
		Reference:        "R1",
		Value:            "1k",
		Position:         kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		Rotation:         90,
		Mirror:           SymbolMirrorX,
		Passthrough:      SymbolPassthroughYes,
		Locked:           true,
		FieldsAutoplaced: true,
		Properties: []Property{
			{
				Name:           "Reference",
				Value:          "R1",
				Private:        true,
				Hidden:         true,
				ShowName:       &showName,
				DoNotAutoplace: &doNotAutoplace,
				Position:       kicadfiles.Point{X: kicadfiles.MM(11), Y: kicadfiles.MM(21)},
			},
			{Name: "Value", Value: "1k", Position: kicadfiles.Point{X: kicadfiles.MM(12), Y: kicadfiles.MM(22)}},
		},
		Fields: []Field{{Name: "Footprint", Value: "Resistor_SMD:R_0603", Visible: true}},
		Pins: []SymbolPin{
			{Number: "2", UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333")},
			{Number: "1", UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"), Alternate: "ALT"},
		},
		Instances: []SymbolInstance{
			{Project: "demo", Path: "/22222222-2222-4222-8222-222222222222", Reference: "R1", Unit: 1},
		},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(mirror x)",
		"(passthrough yes)",
		"(locked yes)",
		"(fields_autoplaced yes)",
		"private",
		"\"Reference\"",
		"\"Footprint\"",
		"\"Resistor_SMD:R_0603\"",
		"(hide yes)",
		"(show_name yes)",
		"(do_not_autoplace yes)",
		"\"1\"",
		"(alternate \"ALT\")",
		"(instances",
		"\"demo\"",
		"\"/22222222-2222-4222-8222-222222222222\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
	if strings.Contains(output, "symbol_instances") {
		t.Fatalf("output contains legacy top-level symbol_instances:\n%s", output)
	}
}

func TestLEDIndicatorSchematicIsDeterministic(t *testing.T) {
	input := LEDIndicatorInput{
		Name:     "led_indicator",
		DesignID: kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		Seed:     "fixture",
	}
	first, err := LEDIndicatorSchematic(input)
	if err != nil {
		t.Fatalf("LEDIndicatorSchematic returned error: %v", err)
	}
	second, err := LEDIndicatorSchematic(input)
	if err != nil {
		t.Fatalf("LEDIndicatorSchematic returned error: %v", err)
	}

	var firstOutput bytes.Buffer
	var secondOutput bytes.Buffer
	if err := Write(&firstOutput, first); err != nil {
		t.Fatalf("first Write returned error: %v", err)
	}
	if err := Write(&secondOutput, second); err != nil {
		t.Fatalf("second Write returned error: %v", err)
	}
	if firstOutput.String() != secondOutput.String() {
		t.Fatalf("LED fixture is not deterministic")
	}
	for _, want := range []string{"(lib_symbols", "\"Device:R\"", "\"Device:LED\"", "\"LED_OUT\"", "(wire"} {
		if !strings.Contains(firstOutput.String(), want) {
			t.Fatalf("LED output missing %s:\n%s", want, firstOutput.String())
		}
	}
}

func assertInOrder(t *testing.T, output string, needles ...string) {
	t.Helper()
	remainder := output
	for _, needle := range needles {
		index := strings.Index(remainder, needle)
		if index == -1 {
			t.Fatalf("output missing %s:\n%s", needle, output)
		}
		remainder = remainder[index+len(needle):]
	}
}

func TestValidateRejectsInvalidElements(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{UUID: schematic.UUID, LibraryID: "", Reference: "R1", Value: "1k"}}
	schematic.Wires = []Wire{{UUID: schematic.UUID, Points: []kicadfiles.Point{{}, {}}}}
	schematic.Labels = []Label{{UUID: schematic.UUID, Text: "", Kind: "bad"}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"symbols[0].library_id", "wires[0].points", "labels[0].text", "labels[0].kind"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestValidateRejectsInvalidSymbolDetails(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:        kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		LibraryID:   "Device:R",
		Reference:   "R1",
		Value:       "1k",
		Mirror:      "diagonal",
		Passthrough: "maybe",
		Properties: []Property{
			{Name: "Reference", Value: "R1"},
			{Name: "Reference", Value: "R2"},
		},
		Pins: []SymbolPin{
			{Number: "1", UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333")},
			{Number: "1", UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333")},
		},
		Instances: []SymbolInstance{{Path: "relative", Reference: "", Unit: -1}},
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"symbols[0].mirror",
		"symbols[0].passthrough",
		"symbols[0].properties[1].name",
		"symbols[0].pins[1].number",
		"symbols[0].pins[1].uuid",
		"symbols[0].instances[0].path",
		"symbols[0].instances[0].reference",
		"symbols[0].instances[0].unit",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestWriteRendersSheet(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		Size:     kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(15)},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	for _, want := range []string{
		"(sheet",
		"\"Sheetname\"\n      \"Power\"",
		"\"Sheetfile\"\n      \"power.kicad_sch\"",
		"(id 0)",
		"(id 1)",
		"(size 30.0 15.0)",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestValidateRejectsInvalidSheet(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     "",
		Name:     "",
		Filename: "../outside.kicad_sch",
		Size:     kicadfiles.Point{},
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"sheets[0].uuid", "sheets[0].name", "sheets[0].filename", "sheets[0].size"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestValidateSheetFilenameAllowsDoubleDotInsideComponent(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Name:     "OddName",
		Filename: "sub_sheet..kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
	}}

	if err := Validate(schematic); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsWindowsAbsoluteSheetFilename(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Name:     "Bad",
		Filename: "C:/tmp/bad.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sheets[0].filename") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSheetName(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{
		{
			UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
			Name:     "Power",
			Filename: "power.kicad_sch",
			Size:     kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		},
		{
			UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
			Name:     "Power",
			Filename: "power2.kicad_sch",
			Size:     kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
		},
	}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate Power") {
		t.Fatalf("error = %v", err)
	}
}

func minimalSchematic() SchematicFile {
	return SchematicFile{
		Version:          kicadfiles.KiCadSchematicFormatV20260306,
		Generator:        "eeschema",
		GeneratorVersion: "10.0",
		UUID:             kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		Paper:            kicadfiles.Paper{Name: "A4"},
	}
}
