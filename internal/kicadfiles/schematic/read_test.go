package schematic

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestReadSchematicWrittenByWriter(t *testing.T) {
	design, err := LEDIndicatorSchematic(LEDIndicatorInput{
		Name:     "reader_demo",
		DesignID: "11111111-1111-5111-8111-111111111111",
		Seed:     "reader_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, design); err != nil {
		t.Fatal(err)
	}
	read, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if read.Version == "" || read.Generator == "" || len(read.Symbols) == 0 || len(read.Wires) == 0 {
		t.Fatalf("unexpected read schematic: %#v", read)
	}
	if read.Symbols[0].UUID == "" || read.Symbols[0].LibraryID == "" {
		t.Fatalf("symbol metadata not read: %#v", read.Symbols[0])
	}
	if read.Symbols[0].Raw == "" {
		t.Fatal("symbol raw node not preserved")
	}
}

func TestReadSchematicPreservesUnsupportedRawNode(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (rule_area (name "Keep") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.RawItems) != 1 {
		t.Fatalf("raw items = %d, want 1", len(read.RawItems))
	}
	if read.RawItems[0].UUID != kicadfiles.UUID("22222222-2222-5222-8222-222222222222") ||
		!strings.Contains(string(read.RawItems[0].Body), "rule_area") {
		t.Fatalf("unexpected raw item: %#v", read.RawItems[0])
	}
}

func TestReadSchematicReadsSymbolMirror(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (symbol`,
		`    (lib_id "Device:R")`,
		`    (at 20 20 0)`,
		`    (mirror x)`,
		`    (uuid "33333333-3333-5333-8333-333333333333")`,
		`    (property "Reference" "R1")`,
		`    (property "Value" "1k")`,
		`  )`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Symbols) != 1 || read.Symbols[0].Mirror != SymbolMirrorX {
		t.Fatalf("symbol mirror not read: %#v", read.Symbols)
	}
}

func TestReadSchematicReadsEmbeddedLibSymbols(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (lib_symbols (symbol "Device:R" (property "Reference" "R")))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.LibSymbols) != 1 || read.LibSymbols[0].LibraryID != "Device:R" || len(read.LibSymbols[0].Body) == 0 {
		t.Fatalf("unexpected lib symbols: %#v", read.LibSymbols)
	}
}

func TestReadWriteSchematicPreservesEmbeddedLibSymbolNumerics(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (lib_symbols`,
		`    (symbol "Device:R"`,
		`      (pin passive line (at 0 1.27 0) (length 2.54) (name "~") (number "1"))`,
		`    )`,
		`  )`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "(at 0 1.27 0)") || !strings.Contains(output, "(length 2.54)") {
		t.Fatalf("embedded symbol numerics not preserved:\n%s", output)
	}
}

func TestReadWriteSchematicReadsSheetSizePinsAndInstances(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (lib_symbols)`,
		`  (sheet`,
		`    (at 10 20 0)`,
		`    (size 30 20)`,
		`    (uuid "22222222-2222-5222-8222-222222222222")`,
		`    (property "Sheetname" "Child" (at 10 17.46 0))`,
		`    (property "Sheetfile" "child.kicad_sch" (at 10 42.54 0))`,
		`    (pin "IN" input (at 10 25 0) (uuid "33333333-3333-5333-8333-333333333333"))`,
		`    (instances`,
		`      (project "project"`,
		`        (path "/child" (page "2"))`,
		`      )`,
		`    )`,
		`  )`,
		`  (sheet_instances (path "/" (page "1")))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		"(size 30.0 20.0)",
		"(pin",
		"\"IN\"",
		"input",
		"\"/child\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %s:\n%s", want, output)
		}
	}
}

func TestReadWriteSchematicKeepsRawTextInItemOrder(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_sch`,
		`  (version 20260306)`,
		`  (generator "eeschema")`,
		`  (generator_version "10.0.0")`,
		`  (uuid "11111111-1111-5111-8111-111111111111")`,
		`  (paper A4)`,
		`  (lib_symbols)`,
		`  (text "Imported note" (at 10 20 0) (uuid "44444444-4444-5444-8444-444444444444"))`,
		`  (wire (pts (xy 5 5) (xy 15 5)) (uuid "22222222-2222-5222-8222-222222222222"))`,
		`  (symbol`,
		`    (lib_id "Device:R")`,
		`    (at 20 20 0)`,
		`    (uuid "33333333-3333-5333-8333-333333333333")`,
		`    (property "Reference" "R1")`,
		`    (property "Value" "1k")`,
		`  )`,
		`  (sheet_instances (path "/" (page "1")))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, read); err != nil {
		t.Fatal(err)
	}
	assertInOrder(t, buf.String(),
		"(lib_symbols)",
		"(wire",
		"(text",
		"\"Imported note\"",
		"(symbol",
	)
}
