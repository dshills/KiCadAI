package libraryresolver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
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
	if record.Pins[0].ElectricalType != "passive" {
		t.Fatalf("electrical type = %q", record.Pins[0].ElectricalType)
	}
	if len(record.Units) != 1 || len(record.Units[0].PinIndexes) != 2 {
		t.Fatalf("units = %#v", record.Units)
	}
}

func TestIndexSymbolsResolvesInheritanceAcrossSymdirFiles(t *testing.T) {
	root := t.TempDir()
	symdir := filepath.Join(root, "Test.kicad_symdir")
	mustWrite(t, filepath.Join(symdir, "Base.kicad_sym"), `(kicad_symbol_lib
	(version 20251024)
	(generator "test")
	(symbol "Base"
		(symbol "Base_1_1"
			(rectangle (start -2.54 -2.54) (end 2.54 2.54) (stroke (width 0.254) (type default)) (fill (type background)))
			(pin passive line (at -5.08 0 0) (length 2.54) (name "IN") (number "1"))
		)
	)
)`)
	mustWrite(t, filepath.Join(symdir, "Child.kicad_sym"), `(kicad_symbol_lib
	(version 20251024)
	(generator "test")
	(symbol "Child"
		(extends "Base")
		(property "Value" "Child")
	)
)`)

	records, issues := IndexSymbols(Discover(LibraryRoots{SymbolsRoot: root}))
	if len(issues) != 0 {
		t.Fatalf("cross-file inheritance issues = %#v", issues)
	}
	child, ok := records["Test:Child"]
	if !ok || !child.Inherited || len(child.Pins) != 1 || len(child.Graphics) != 1 || len(child.Diagnostics) != 0 {
		t.Fatalf("cross-file child = %#v", child)
	}
	if strings.Contains(child.Raw, `(extends "Base")`) || !strings.Contains(child.Raw, `"Child"`) || !strings.Contains(child.Raw, "(rectangle") || !strings.Contains(child.Raw, "(pin") {
		t.Fatalf("cross-file child raw body was not materialized: %s", child.Raw)
	}
}

func TestIndexSymbolsParsesConnectorSymbol(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Connector.kicad_sym"), connectorParserSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if hasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	record, ok := records["Connector:Conn_01x02"]
	if !ok {
		t.Fatalf("missing Connector:Conn_01x02 in %#v", records)
	}
	if len(record.Pins) != 2 {
		t.Fatalf("pins = %#v", record.Pins)
	}
	if record.Pins[0].Number != "1" || record.Pins[1].Number != "2" {
		t.Fatalf("pin order = %#v", record.Pins)
	}
	if record.Pins[0].Name != "Pin_1" || record.Pins[1].Name != "Pin_2" {
		t.Fatalf("pin names = %#v", record.Pins)
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
	if hasBlockingIssue(issues) {
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

func TestCollectSymbolUnitsAppliesCommonPinsToEveryUnit(t *testing.T) {
	units := collectSymbolUnits([]SymbolPin{
		{Number: "1", Unit: 1, BodyStyle: 1},
		{Number: "2", Unit: 2, BodyStyle: 1},
		{Number: "3", Unit: 0, BodyStyle: 0},
	})
	if len(units) != 2 {
		t.Fatalf("units = %#v", units)
	}
	for _, unit := range units {
		if len(unit.CommonPinIndexes) != 1 || unit.CommonPinIndexes[0] != 2 {
			t.Fatalf("common pins not applied to unit %#v", unit)
		}
	}
}

func TestIndexSymbolsParsesCommonUnitPins(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Logic.kicad_sym"), commonPinSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Logic:Dual_Buffer"]
	if len(record.Units) != 2 {
		t.Fatalf("units = %#v", record.Units)
	}
	if !record.Pins[2].Common || record.Pins[2].Unit != 0 {
		t.Fatalf("common pin = %#v", record.Pins[2])
	}
	for _, unit := range record.Units {
		if len(unit.CommonPinIndexes) != 1 || unit.CommonPinIndexes[0] != 2 {
			t.Fatalf("common pin indexes = %#v", record.Units)
		}
	}
}

func TestIndexSymbolsReportsUnknownElectricalType(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Bad.kicad_sym"), unknownElectricalSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if _, ok := records["Bad:Odd"]; !ok {
		t.Fatalf("records = %#v", records)
	}
	if !hasSymbolIssue(issues, "unknown electrical type") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsReportsDuplicatePinInSameUnit(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Bad.kicad_sym"), duplicatePinSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if _, ok := records["Bad:Dup"]; !ok {
		t.Fatalf("records = %#v", records)
	}
	if !hasSymbolIssue(issues, "duplicate pin number 1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsAllowsDuplicatePinsAcrossSeparateUnits(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Logic.kicad_sym"), duplicateAcrossUnitsSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if _, ok := records["Logic:Dual_Gate"]; !ok {
		t.Fatalf("records = %#v", records)
	}
	if hasSymbolIssue(issues, "duplicate pin number") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsDetectsPowerSymbolAndHiddenPowerPolicy(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "power.kicad_sym"), powerSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if !hasSymbolIssue(issues, "hidden power pin") {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["power:VCC"]
	if !record.PowerSymbol {
		t.Fatalf("expected power symbol: %#v", record)
	}
	acceptanceIssues := SymbolConnectivityAcceptanceIssues(record)
	if !hasSymbolIssue(acceptanceIssues, "hidden power pin") {
		t.Fatalf("acceptance issues = %#v", acceptanceIssues)
	}
}

func TestIndexSymbolsResolvesInheritedSymbolMetadata(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), inheritedSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Device:R_Small"]
	if !record.Inherited || record.Extends != "R_Base" {
		t.Fatalf("record = %#v", record)
	}
	if record.Properties["Reference"] != "R" || record.Properties["Value"] != "R_Small" {
		t.Fatalf("properties = %#v", record.Properties)
	}
	if len(record.Pins) != 2 || record.Pins[0].Number != "1" || record.Pins[1].Number != "2" {
		t.Fatalf("pins = %#v", record.Pins)
	}
	if len(record.Graphics) == 0 {
		t.Fatalf("graphics = %#v", record.Graphics)
	}
}

func TestIndexSymbolsParsesPropertyFormMetadata(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), propertyFormMetadataSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Device:Property_Metadata"]
	if record.Description != "Property form description" {
		t.Fatalf("description = %q", record.Description)
	}
	if got := strings.Join(record.Keywords, " "); got != "property form metadata" {
		t.Fatalf("keywords = %#v", record.Keywords)
	}
	if len(record.FootprintFilter) != 2 || record.FootprintFilter[0] != "R_*" || record.FootprintFilter[1] != "Resistor_*" {
		t.Fatalf("footprint filters = %#v", record.FootprintFilter)
	}
	if record.Datasheet != "https://example.test/ds.pdf" {
		t.Fatalf("datasheet = %q", record.Datasheet)
	}
	kiDescriptionRecord := records["Device:Ki_Description_Property"]
	if kiDescriptionRecord.Description != "ki_description property value" {
		t.Fatalf("ki_description property description = %q", kiDescriptionRecord.Description)
	}
	if got := strings.Join(kiDescriptionRecord.Keywords, " "); got != "standard keywords" {
		t.Fatalf("standard Keywords property = %#v", kiDescriptionRecord.Keywords)
	}
}

func TestIndexSymbolsReportsCyclicInheritanceOnAllAffectedSymbols(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), cyclicInheritanceSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if _, ok := records["Device:Cycle_A"]; !ok {
		t.Fatalf("records = %#v", records)
	}
	if !hasSymbolIssueForPath(issues, "library.symbol.Device:Cycle_A", "cyclic symbol inheritance") {
		t.Fatalf("missing Cycle_A cyclic diagnostic: %#v", issues)
	}
	if !hasSymbolIssueForPath(issues, "library.symbol.Device:Cycle_B", "cyclic symbol inheritance") {
		t.Fatalf("missing Cycle_B cyclic diagnostic: %#v", issues)
	}
}

func TestIndexSymbolsMissingInheritedBaseBlocks(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), missingBaseSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if _, ok := records["Device:Derived"]; !ok {
		t.Fatalf("records = %#v", records)
	}
	if !hasSymbolIssue(issues, "unresolved base symbol") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsMissingInheritedAncestorBlocks(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), `(kicad_symbol_lib
	(version 20251024)
	(generator "test")
	(symbol "Base"
		(extends "Missing")
	)
	(symbol "Derived"
		(extends "Base")
	)
)`)

	records, issues := IndexSymbols(Discover(LibraryRoots{SymbolsRoot: symbols}))
	derived, ok := records["Device:Derived"]
	if !ok || derived.Inherited {
		t.Fatalf("derived record = %#v", derived)
	}
	if !hasSymbolIssue(issues, "unresolved inherited base symbol") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestIndexSymbolsParsesGraphicsBoundsAndBodyStyle(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), graphicsSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	record := records["Device:Graphic"]
	if len(record.Graphics) != 3 {
		t.Fatalf("graphics = %#v", record.Graphics)
	}
	if record.Graphics[0].BodyStyle != 2 {
		t.Fatalf("body style = %#v", record.Graphics[0])
	}
	box := record.Graphics[0].Bounds
	if box.Min.X != kicadfiles.MM(-1.27) || box.Max.X != kicadfiles.MM(1.27) {
		t.Fatalf("bounds = %#v", box)
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

func TestIndexSymbolsInvalidSymbolIDDiagnostic(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Bad.kicad_sym"), invalidIDSymbolLibrary())

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	records, issues := IndexSymbols(inventory)
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "invalid symbol ID") {
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
	for _, want := range []string{`"library_id": "Device:R"`, `"description": "Resistor"`, `"number": "1"`, `"electrical_type": "passive"`} {
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
      (pin input line (at 0 5.08 270) (length 2.54) hide (name "V+") (number "8"))
    )
  )
)`
}

func connectorParserSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Conn_01x02"
    (property "Reference" "J" (at 0 0 0))
    (property "Value" "Conn_01x02" (at 0 -2.54 0))
    (ki_description "Generic connector, single row, 01x02")
    (ki_keywords "connector")
    (symbol "Conn_01x02_1_1"
      (pin passive line (at 0 2.54 270) (length 2.54) (name "Pin_1") (number "1"))
      (pin passive line (at 0 -2.54 90) (length 2.54) (name "Pin_2") (number "2"))
    )
  )
)`
}

func invalidIDSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Bad:Name"
    (property "Reference" "U" (at 0 0 0))
  )
)`
}

func commonPinSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Dual_Buffer"
    (property "Reference" "U" (at 0 0 0))
    (property "Value" "Dual_Buffer" (at 0 -2.54 0))
    (symbol "Dual_Buffer_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
    )
    (symbol "Dual_Buffer_2_1"
      (pin output line (at 5.08 0 180) (length 2.54) (name "Y") (number "2"))
    )
    (symbol "Dual_Buffer_0_1"
      (pin power_in line (at 0 5.08 270) (length 2.54) (name "VCC") (number "14"))
    )
  )
)`
}

func unknownElectricalSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Odd"
    (property "Reference" "U" (at 0 0 0))
    (symbol "Odd_1_1"
      (pin strange line (at 0 0 0) (length 2.54) (name "X") (number "1"))
    )
  )
)`
}

func duplicatePinSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Dup"
    (property "Reference" "U" (at 0 0 0))
    (symbol "Dup_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
      (pin output line (at 5.08 0 180) (length 2.54) (name "B") (number "1"))
    )
  )
)`
}

func duplicateAcrossUnitsSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Dual_Gate"
    (property "Reference" "U" (at 0 0 0))
    (symbol "Dual_Gate_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
    )
    (symbol "Dual_Gate_2_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
    )
  )
)`
}

func powerSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "VCC"
    (property "Reference" "#PWR" (at 0 0 0))
    (property "Value" "VCC" (at 0 -2.54 0))
    (symbol "VCC_1_1"
      (pin power_in line (at 0 0 90) (length 0) hide (name "VCC") (number "1"))
    )
  )
)`
}

func inheritedSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R_Base"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R_Base" (at 0 -2.54 0))
    (ki_description "Base resistor")
    (symbol "R_Base_1_1"
      (rectangle (start -1.27 -1.27) (end 1.27 1.27))
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
  (symbol "R_Small"
    (extends "R_Base")
    (property "Value" "R_Small" (at 0 -2.54 0))
  )
	)`
}

func propertyFormMetadataSymbolLibrary() string {
	return `
	(kicad_symbol_lib
	  (version 20220914)
	  (generator "kicadai-test")
	  (symbol "Property_Metadata"
	    (property "Reference" "R" (at 0 0 0))
	    (property "Value" "Property_Metadata" (at 0 -2.54 0))
	    (property "Description" "Property form description" (at 0 0 0))
	    (property "ki_keywords" "property form metadata" (at 0 0 0))
	    (property "ki_fp_filters" "R_* Resistor_*" (at 0 0 0))
	    (property "ki_datasheet" "https://example.test/ds.pdf" (at 0 0 0))
	    (symbol "Property_Metadata_1_1"
	      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
	      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
	    )
	  )
	  (symbol "Ki_Description_Property"
	    (property "Reference" "R" (at 0 0 0))
	    (property "Value" "Ki_Description_Property" (at 0 -2.54 0))
	    (property "ki_description" "ki_description property value" (at 0 0 0))
	    (property "Keywords" "standard keywords" (at 0 0 0))
	    (symbol "Ki_Description_Property_1_1"
	      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
	      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
	    )
	  )
	)`
}

func cyclicInheritanceSymbolLibrary() string {
	return `
	(kicad_symbol_lib
	  (version 20220914)
	  (generator "kicadai-test")
	  (symbol "Cycle_A"
	    (extends "Cycle_B")
	    (property "Reference" "U" (at 0 0 0))
	  )
	  (symbol "Cycle_B"
	    (extends "Cycle_A")
	    (property "Reference" "U" (at 0 0 0))
	  )
	)`
}

func missingBaseSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Derived"
    (extends "Missing_Base")
    (property "Reference" "U" (at 0 0 0))
  )
)`
}

func graphicsSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Graphic"
    (property "Reference" "U" (at 0 0 0))
    (symbol "Graphic_1_2"
      (rectangle (start -1.27 -2.54) (end 1.27 2.54))
      (circle (center 5.08 0) (radius 1.27))
      (polyline (pts (xy -2.54 -2.54) (xy -1.27 -1.27) (xy -2.54 0)))
    )
  )
)`
}

func hasSymbolIssue(issues []reports.Issue, text string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}

func hasSymbolIssueForPath(issues []reports.Issue, path string, text string) bool {
	for _, issue := range issues {
		if issue.Path == path && strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}

func hasBlockingIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}
