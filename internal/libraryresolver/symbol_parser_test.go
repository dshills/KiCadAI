package libraryresolver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestIndexSymbolsParsesSimpleSymbol(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record, ok := records["Device:R"]
	if !ok {
		t.Fatalf("missing Device:R in %#v", records)
	}
	if record.Description != "Resistor" {
		t.Fatalf("description = %q", record.Description)
	}
	if got := strings.Join(record.Keywords, " "); got != "resistor R" {
		t.Fatalf("keywords = %#v", record.Keywords)
	}
	if len(record.FootprintFilter) != 2 || record.FootprintFilter[0] != "R_*" {
		t.Fatalf("footprint filters = %#v", record.FootprintFilter)
	}
	if record.Properties["Reference"] != "R" || record.Properties["Value"] != "R" {
		t.Fatalf("properties = %#v", record.Properties)
	}
	if len(record.Pins) != 2 {
		t.Fatalf("pins = %#v", record.Pins)
	}
	if record.Pins[0].Number != "1" || record.Pins[0].Electrical != "passive" || record.Pins[0].Length != kicadfiles.MM(2.54) {
		t.Fatalf("pin 1 = %#v", record.Pins[0])
	}
}

func TestResolveSymbol(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{
		"Device:R": {LibraryID: "Device:R", Name: "R"},
	}}
	record, ok := ResolveSymbol(index, "Device:R")
	if !ok || record.Name != "R" {
		t.Fatalf("ResolveSymbol = %#v/%v", record, ok)
	}
}

func TestIndexSymbolsParsesMultiUnitAndHiddenPins(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Amplifier.kicad_sym"), amplifierSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Amplifier:Dual_OpAmp"]
	if len(record.Units) != 2 {
		t.Fatalf("units = %#v", record.Units)
	}
	if len(record.Pins) != 4 {
		t.Fatalf("pins = %#v", record.Pins)
	}
	if !record.Pins[3].Hidden || record.Pins[3].Number != "8" || record.Pins[3].Unit != 2 {
		t.Fatalf("hidden pin = %#v", record.Pins[3])
	}
}

func TestIndexSymbolsMalformedFileDiagnostic(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Bad.kicad_sym"), "(kicad_symbol_lib")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unterminated") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsInvalidRootDiagnostic(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Bad.kicad_sym"), "(not_symbols)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	_, issues := IndexSymbols(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "expected kicad_symbol_lib root") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsOversizedFileDiagnostic(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	path := filepath.Join(symbols, "Huge.kicad_sym")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxSymbolLibraryBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	_, issues := IndexSymbols(inventory)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "exceeds 64 MiB parser limit") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsDuplicateIDDiagnostic(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), resistorSymbolLibrary())
	mustWrite(t, filepath.Join(symbols, "Device.kicad_symdir", "Alternate.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	_, issues := IndexSymbols(inventory)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "duplicate symbol ID Device:R") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate symbol ID diagnostic: %#v", issues)
	}
}

func TestIndexSymbolsGoldenRecordJSON(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.MarshalIndent(records["Device:R"], "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"library_id": "Device:R"`, `"description": "Resistor"`, `"number": "1"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("golden JSON missing %s:\n%s", want, text)
		}
	}
}

func resistorSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (property "Datasheet" "~" (at 0 0 0))
    (ki_keywords "resistor R")
    (ki_description "Resistor")
    (ki_fp_filters "R_*" "Resistor_*")
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
)`
}

func amplifierSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Dual_OpAmp"
    (property "Reference" "U" (at 0 0 0))
    (property "Value" "Dual_OpAmp" (at 0 -2.54 0))
    (symbol "Dual_OpAmp_1_1"
      (pin input line (at -5.08 2.54 0) (length 2.54) (name "+") (number "3"))
      (pin output line (at 5.08 0 180) (length 2.54) (name "OUT") (number "1"))
    )
    (symbol "Dual_OpAmp_2_1"
      (pin input line (at -5.08 -2.54 0) (length 2.54) (name "-") (number "6"))
      (pin power_in line (at 0 5.08 270) (length 2.54) hide (name "V+") (number "8"))
    )
  )
)`
}
