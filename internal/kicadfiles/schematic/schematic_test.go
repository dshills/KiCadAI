package schematic

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
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
		"  (sheet_instances",
		"    (path",
		"      \"/\"",
		"      (page \"1\")",
		"    )",
		"  )",
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
	schematic.NoConnects = []NoConnect{{
		UUID:     kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		Position: kicadfiles.Point{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
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
		"(no_connect",
		"(wire",
		"(label",
		"(global_label",
		"(hierarchical_label",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	)
}

func TestWriteRendersLabelShapesAndNoConnects(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Labels = []Label{{
		UUID:             kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:             "OUT",
		Kind:             LabelGlobal,
		Shape:            LabelShapeOutput,
		Position:         kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		Locked:           true,
		FieldsAutoplaced: true,
		Fields:           []Field{{Name: "Netclass", Value: "Fast", Visible: true}},
	}}
	schematic.NoConnects = []NoConnect{{
		UUID:     kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Position: kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(20)},
	}}
	schematic.Junctions = []Junction{{
		UUID:     kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
		Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
		Diameter: kicadfiles.MM(1),
		Color:    Color{R: 1, G: 2, B: 3, A: 4},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(junction",
		"(diameter 1.0)",
		"(color 1 2 3 4)",
		"(no_connect",
		"(global_label",
		"(shape output)",
		"(locked yes)",
		"(fields_autoplaced yes)",
		"\"Netclass\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestWriteRendersBusesPolylinesBusEntriesAndText(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Buses = []Bus{{
		UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.Polylines = []Polyline{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(25)},
		},
	}}
	schematic.BusEntries = []BusEntry{{
		UUID:     kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
		Position: kicadfiles.Point{X: kicadfiles.MM(15), Y: kicadfiles.MM(10)},
		Size:     kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(2.54)},
	}}
	schematic.Texts = []Text{{
		UUID:     kicadfiles.UUID("55555555-5555-4555-8555-555555555555"),
		Value:    "Bus note",
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(30)},
		Locked:   true,
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{"(bus_entry", "(size 2.54 2.54)", "(bus", "(polyline", "(text", "\"Bus note\"", "(locked yes)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestWritePreservesRawSchematicItemsInKiCadOrder(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:      kicadfiles.UUID("55555555-5555-4555-8555-555555555555"),
		LibraryID: "Device:R",
		Reference: "R1",
		Value:     "1k",
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Body: sexpr.Raw(`
			(rule_area (name "Preserved") (uuid "33333333-3333-4333-8333-333333333333"))`),
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	assertInOrder(t, output,
		"(lib_symbols)",
		"(rule_area",
		"\"Preserved\"",
		"(symbol",
	)
}

func TestWritePreservesUnknownRawSchematicItems(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:      kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		LibraryID: "Device:R",
		Reference: "R1",
		Value:     "1k",
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("77777777-7777-4777-8777-777777777777"),
		Body: sexpr.Raw(`(future_widget (uuid "77777777-7777-4777-8777-777777777777") (value "preserve me"))`),
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	assertInOrder(t, buf.String(),
		"(symbol",
		"(future_widget",
		"\"preserve me\"",
		"(sheet_instances",
	)
}

func TestWritePreservesRawSchematicItemsWithExplicitOrder(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:      kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		LibraryID: "Device:R",
		Reference: "R1",
		Value:     "1k",
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID:  kicadfiles.UUID("77777777-7777-4777-8777-777777777777"),
		Order: int64(schematicItemSymbol)*schematicItemOrderStride - 1,
		Body:  sexpr.Raw(`(future_widget (uuid "77777777-7777-4777-8777-777777777777") (value "before symbol"))`),
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	assertInOrder(t, buf.String(),
		"(future_widget",
		"\"before symbol\"",
		"(symbol",
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

func TestWriteRendersRequiredSymbolPropertiesBeforeExtras(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:      kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		LibraryID: "Device:R",
		Reference: "R1",
		Value:     "1k",
		Properties: []Property{
			{Name: "Tolerance", Value: "1%"},
			{Name: "Description", Value: "Precision resistor"},
			{Name: "Footprint", Value: "Resistor_SMD:R_0603"},
		},
		Fields: []Field{
			{Name: "Datasheet", Value: "https://example.test/r.pdf", Visible: true},
			{Name: "Manufacturer", Value: "ExampleCo", Visible: true},
		},
	}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	assertInOrder(t, buf.String(),
		"\"Reference\"",
		"\"Value\"",
		"\"Footprint\"",
		"\"Datasheet\"",
		"\"Description\"",
		"\"Tolerance\"",
		"\"Manufacturer\"",
	)
}

func TestNewSymbolUsesWriterDerivedProperties(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
	)

	if symbol.Reference != "R1" || symbol.Value != "1k" {
		t.Fatalf("NewSymbol compatibility fields = %q/%q", symbol.Reference, symbol.Value)
	}
	if len(symbol.Properties) != 0 {
		t.Fatalf("NewSymbol properties length = %d, want 0", len(symbol.Properties))
	}

	properties := symbolProperties(symbol)
	if len(properties) != 5 {
		t.Fatalf("derived properties length = %d, want 5", len(properties))
	}
	if properties[0].Name != "Reference" || properties[0].Value != "R1" {
		t.Fatalf("derived reference property = %#v", properties[0])
	}
	if properties[1].Name != "Value" || properties[1].Value != "1k" {
		t.Fatalf("derived value property = %#v", properties[1])
	}
	for index, want := range []string{"Footprint", "Datasheet", "Description"} {
		property := properties[index+2]
		if property.Name != want || property.Value != "" {
			t.Fatalf("derived %s property = %#v", want, property)
		}
	}
}

func TestWriterDerivedSymbolPropertiesIncludeRoundTripDefaults(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{},
	)

	properties := symbolProperties(symbol)
	names := make([]string, 0, len(properties))
	for _, property := range properties {
		names = append(names, property.Name)
	}
	for _, want := range []string{"Reference", "Value"} {
		if !slices.Contains(names, want) {
			t.Errorf("derived properties missing %q: %#v", want, names)
		}
	}
	for _, want := range []string{"Footprint", "Datasheet", "Description"} {
		if !slices.Contains(names, want) {
			t.Errorf("derived properties missing round-trip default %q: %#v", want, names)
		}
	}
}

func TestWriterDerivedSymbolPropertiesPreserveExplicitDefaults(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{},
	)
	symbol.Properties = []Property{
		{Name: "Datasheet", Value: "https://example.test/r.pdf", Hidden: true},
		{Name: "Description", Value: "Precision resistor"},
	}

	properties := symbolProperties(symbol)
	if properties[3].Name != "Datasheet" || properties[3].Value != "https://example.test/r.pdf" || !properties[3].Hidden {
		t.Fatalf("Datasheet property not preserved: %#v", properties[3])
	}
	if properties[4].Name != "Description" || properties[4].Value != "Precision resistor" {
		t.Fatalf("Description property not preserved: %#v", properties[4])
	}
}

func TestWriterDerivedSymbolPropertiesUseLastExplicitDuplicate(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{},
	)
	symbol.Properties = []Property{
		{Name: "Description", Value: "old"},
		{Name: "Description", Value: "new"},
		{Name: "Tolerance", Value: "1%"},
		{Name: "Tolerance", Value: "5%"},
	}

	properties := symbolProperties(symbol)
	if properties[4].Name != "Description" || properties[4].Value != "new" {
		t.Fatalf("Description property = %#v, want last duplicate", properties[4])
	}
	if len(properties) != 6 || properties[5].Name != "Tolerance" || properties[5].Value != "5%" {
		t.Fatalf("extra properties = %#v, want last duplicate Tolerance only", properties)
	}
}

func TestWriterDerivedSymbolPropertiesUseLastLegacyFieldDuplicate(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{},
	)
	symbol.Fields = []Field{
		{Name: "Description", Value: "old"},
		{Name: "Description", Value: "new"},
		{Name: "Tolerance", Value: "1%"},
		{Name: "Tolerance", Value: "5%"},
	}

	properties := symbolProperties(symbol)
	if properties[4].Name != "Description" || properties[4].Value != "new" {
		t.Fatalf("Description property = %#v, want last legacy duplicate", properties[4])
	}
	if len(properties) != 6 || properties[5].Name != "Tolerance" || properties[5].Value != "5%" {
		t.Fatalf("extra properties = %#v, want last legacy duplicate Tolerance only", properties)
	}
}

func TestWriterDerivedSymbolPropertiesKeepStructReferenceAndValue(t *testing.T) {
	symbol := NewSymbol(
		kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		"Device:R",
		"R1",
		"1k",
		kicadfiles.Point{},
	)
	symbol.Fields = []Field{
		{Name: "Reference", Value: "R_BAD"},
		{Name: "Value", Value: "bad"},
	}

	properties := symbolProperties(symbol)
	if properties[0].Name != "Reference" || properties[0].Value != "R1" {
		t.Fatalf("Reference property = %#v, want struct-derived reference", properties[0])
	}
	if properties[1].Name != "Value" || properties[1].Value != "1k" {
		t.Fatalf("Value property = %#v, want struct-derived value", properties[1])
	}
}

func TestRenderAtCanonicalizesZero(t *testing.T) {
	output, err := sexpr.Format(renderAt(kicadfiles.Point{}, 0))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if strings.TrimSpace(output) != "(at 0 0 0)" {
		t.Fatalf("expected schematic zero at-node to render canonically:\n%s", output)
	}
}

func TestSchematicFixedPreservesNonZeroPrecision(t *testing.T) {
	output, err := sexpr.Format(sexpr.L(sexpr.A("xy"), schematicFixed(kicadfiles.IU(1_234_567)), schematicFixed(kicadfiles.MM(-0.5))))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if strings.TrimSpace(output) != "(xy 1.234567 -0.5)" {
		t.Fatalf("formatted non-zero points = %q", strings.TrimSpace(output))
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
	schematic.NoConnects = []NoConnect{{UUID: ""}}
	schematic.Junctions = []Junction{{
		UUID:     "",
		Diameter: -1,
		Color:    Color{R: 300},
	}}
	schematic.Buses = []Bus{{UUID: "", Points: []kicadfiles.Point{{}}}}
	schematic.Polylines = []Polyline{{UUID: "", Points: []kicadfiles.Point{{}, {}}}}
	schematic.BusEntries = []BusEntry{{UUID: "", Size: kicadfiles.Point{}}}
	schematic.Texts = []Text{{UUID: "", Value: ""}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: "",
		Kind: RawItemBitmap,
		Body: sexpr.Raw(`(rule_area (name "wrong")`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"symbols[0].library_id", "wires[0].points", "labels[0].text", "labels[0].kind", "no_connects[0].uuid", "junctions[0].uuid", "junctions[0].diameter", "junctions[0].color", "buses[0].uuid", "buses[0].points", "polylines[0].uuid", "polylines[0].points", "bus_entries[0].uuid", "bus_entries[0].size", "texts[0].uuid", "texts[0].value", "raw_items[0].uuid", "raw_items[0].body"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestValidateRejectsRawItemKindMismatch(t *testing.T) {
	schematic := minimalSchematic()
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Kind: RawItemBitmap,
		Body: sexpr.Raw(`(rule_area (name "Preserved") (uuid "33333333-3333-4333-8333-333333333333"))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].kind") {
		t.Fatalf("error missing raw item kind mismatch: %v", err)
	}
}

func TestValidateRejectsNegativeRawItemOrder(t *testing.T) {
	schematic := minimalSchematic()
	schematic.RawItems = []RawSchematicItem{{
		UUID:  kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Order: -1,
		Body:  sexpr.Raw(`(rule_area (name "Preserved") (uuid "33333333-3333-4333-8333-333333333333"))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].order") {
		t.Fatalf("error missing raw item order validation: %v", err)
	}
}

func TestValidateRejectsRawItemUUIDMismatch(t *testing.T) {
	schematic := minimalSchematic()
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Body: sexpr.Raw(`(rule_area (name "33333333-3333-4333-8333-333333333333") (uuid "44444444-4444-4444-8444-444444444444"))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].body") {
		t.Fatalf("error missing raw item UUID mismatch: %v", err)
	}
}

func TestValidateRejectsMalformedRawItemBody(t *testing.T) {
	schematic := minimalSchematic()
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Body: sexpr.Raw(`(rule_area (uuid "33333333-3333-4333-8333-333333333333")`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].body") {
		t.Fatalf("error missing raw body validation: %v", err)
	}
}

func TestValidateRejectsMultipleTopLevelRawItemBodies(t *testing.T) {
	schematic := minimalSchematic()
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Body: sexpr.Raw(`(rule_area (uuid "33333333-3333-4333-8333-333333333333")) (future_widget (uuid "44444444-4444-4444-8444-444444444444"))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].body") {
		t.Fatalf("error missing multiple top-level raw body validation: %v", err)
	}
}

func TestValidateRejectsDuplicateRawItemUUID(t *testing.T) {
	schematic := minimalSchematic()
	duplicateUUID := kicadfiles.UUID("33333333-3333-4333-8333-333333333333")
	schematic.Wires = []Wire{{
		UUID: duplicateUUID,
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: duplicateUUID,
		Body: sexpr.Raw(`(rule_area (name "Preserved") (uuid "33333333-3333-4333-8333-333333333333"))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].uuid") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error missing duplicate raw item UUID: %v", err)
	}
}

func TestRawSchematicItemUUIDsRequiresUUIDListShape(t *testing.T) {
	uuids, err := rawSchematicItemUUIDs(`(future_widget (uuid "33333333-3333-4333-8333-333333333333") (property uuid "44444444-4444-4444-8444-444444444444") (uuid "55555555-5555-4555-8555-555555555555" extra))`)
	if err != nil {
		t.Fatal(err)
	}
	if len(uuids) != 1 || uuids[0] != kicadfiles.UUID("33333333-3333-4333-8333-333333333333") {
		t.Fatalf("uuids = %#v, want only direct uuid list", uuids)
	}
}

func TestValidateRejectsDuplicateNestedRawItemUUID(t *testing.T) {
	schematic := minimalSchematic()
	nestedDuplicateUUID := kicadfiles.UUID("33333333-3333-4333-8333-333333333333")
	schematic.Wires = []Wire{{
		UUID: nestedDuplicateUUID,
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
		Body: sexpr.Raw(`(future_widget (uuid "44444444-4444-4444-8444-444444444444") (child (uuid "33333333-3333-4333-8333-333333333333")))`),
	}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "raw_items[0].body") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error missing duplicate nested raw item UUID: %v", err)
	}
}

func TestValidateIgnoresRawUUIDPatternInsideString(t *testing.T) {
	schematic := minimalSchematic()
	mentionedUUID := kicadfiles.UUID("33333333-3333-4333-8333-333333333333")
	schematic.Wires = []Wire{{
		UUID: mentionedUUID,
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.RawItems = []RawSchematicItem{{
		UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
		Body: sexpr.Raw(`(future_widget (uuid "44444444-4444-4444-8444-444444444444") (name "(uuid \"33333333-3333-4333-8333-333333333333\")"))`),
	}}

	if err := Validate(schematic); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsInvalidLabelShapeAndNoConnect(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Labels = []Label{{
		UUID:   kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Text:   "OUT",
		Kind:   LabelGlobal,
		Shape:  "sideways",
		Fields: []Field{{Name: ""}},
	}}
	schematic.NoConnects = []NoConnect{{UUID: ""}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"labels[0].shape", "labels[0].fields[0].name", "no_connects[0].uuid"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}

func TestValidateGeneratedConnectivityAcceptsKnownAnchors(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Wires = []Wire{{
		UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.Labels = []Label{{
		UUID:     kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Text:     "IN",
		Kind:     LabelLocal,
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
	}}
	schematic.Junctions = []Junction{{
		UUID:     kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
		Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}

	if err := ValidateGeneratedConnectivity(schematic); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityAcceptsSymbolPinAnchors(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Symbols = []SchematicSymbol{{
		UUID:       kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
		LibraryID:  "Device:R",
		Reference:  "R1",
		Value:      "1k",
		PinAnchors: []kicadfiles.Point{{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}},
	}}
	schematic.Wires = []Wire{{
		UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.Labels = []Label{{
		UUID:     kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Text:     "OUT",
		Kind:     LabelLocal,
		Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}

	if err := ValidateGeneratedConnectivity(schematic); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestValidateGeneratedConnectivityRejectsOpenEndpointAndNearMiss(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Wires = []Wire{{
		UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"),
		Points: []kicadfiles.Point{
			{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)},
			{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}}
	schematic.Labels = []Label{{
		UUID:     kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
		Text:     "IN",
		Kind:     LabelLocal,
		Position: kicadfiles.Point{X: kicadfiles.MM(10.1), Y: kicadfiles.MM(10)},
	}}

	err := ValidateGeneratedConnectivity(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"near but not on anchor", "endpoint is not connected"} {
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
		"(size 30.0 15.0)",
		"(exclude_from_sim no)",
		"(in_bom yes)",
		"(on_board yes)",
		"(dnp no)",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestWriteRendersSheetPinsAndInstances(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		Size:     kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(15)},
		Locked:   true,
		Pins: []SheetPin{{
			UUID:     kicadfiles.UUID("22345678-1234-5678-9234-123456789abc"),
			Text:     "VIN",
			Kind:     SheetPinInput,
			Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(25)},
			Rotation: 180,
		}},
		Instances: []SheetInstance{{Project: "demo", Path: "/12345678-1234-5678-9234-123456789abc", Page: "2"}},
	}}
	schematic.SheetInstances = []SheetInstance{{Path: "/", Page: "1"}}

	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"(locked yes)",
		"(pin",
		"\"VIN\"",
		"input",
		"(at 10.0 25.0 180)",
		"(instances",
		"\"demo\"",
		"(page \"2\")",
		"(sheet_instances",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
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

func TestValidateRejectsInvalidSheetPinAndInstance(t *testing.T) {
	schematic := minimalSchematic()
	schematic.Sheets = []Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)},
		Size:     kicadfiles.Point{X: kicadfiles.MM(30), Y: kicadfiles.MM(15)},
		Pins: []SheetPin{{
			UUID:     "",
			Text:     "",
			Kind:     "sideways",
			Position: kicadfiles.Point{X: kicadfiles.MM(12), Y: kicadfiles.MM(22)},
		}},
		Instances: []SheetInstance{{Path: "relative", Page: ""}},
	}}
	schematic.SheetInstances = []SheetInstance{{Path: "relative", Page: ""}}

	err := Validate(schematic)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"sheets[0].pins[0].uuid",
		"sheets[0].pins[0].text",
		"sheets[0].pins[0].kind",
		"sheets[0].pins[0].position",
		"sheets[0].instances[0].path",
		"sheets[0].instances[0].page",
		"sheet_instances[0].path",
		"sheet_instances[0].page",
	} {
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
