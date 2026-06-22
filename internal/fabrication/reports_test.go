package fabrication

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	schematicfiles "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

func TestBuildBOMRowsSortsAndGroupsDeterministically(t *testing.T) {
	rows, issues := BuildBOMRows(schematicfiles.SchematicFile{Symbols: []schematicfiles.SchematicSymbol{
		testBOMSymbol("R2", "10k", "Device:R", "Resistor_SMD:R_0603", "Yageo", "RC0603FR-0710KL"),
		testBOMSymbol("R10", "10k", "Device:R", "Resistor_SMD:R_0603", "Yageo", "RC0603FR-0710KL"),
		testBOMSymbol("R1", "10k", "Device:R", "Resistor_SMD:R_0603", "Yageo", "RC0603FR-0710KL"),
		testBOMSymbol("C1", "100n", "Device:C", "Capacitor_SMD:C_0603", "Murata", "GRM188R71C104KA01"),
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want grouped resistor and capacitor rows", rows)
	}
	if got := strings.Join(rows[0].References, " "); got != "C1" {
		t.Fatalf("first references = %s, want C1", got)
	}
	if got := strings.Join(rows[1].References, " "); got != "R1 R2 R10" || rows[1].Quantity != 3 {
		t.Fatalf("resistor row = %#v, want natural-sorted R1/R2/R10 quantity 3", rows[1])
	}
}

func TestBuildBOMRowsReportsMissingReadinessData(t *testing.T) {
	rows, issues := BuildBOMRows(schematicfiles.SchematicFile{Symbols: []schematicfiles.SchematicSymbol{
		{Reference: "U1", Value: "MCU", LibraryID: "MCU:Part"},
		{
			Reference: "U2",
			Value:     "MCU",
			LibraryID: "MCU:Part",
			Properties: []schematicfiles.Property{
				{Name: "Manufacturer", Value: "Example"},
				{Name: "MPN", Value: "EX-1"},
			},
		},
	}})
	if len(rows) != 2 || rows[0].Confidence != "partial" ||
		!strings.Contains(rows[0].ReadinessNote, "missing manufacturer or MPN") ||
		!strings.Contains(rows[0].ReadinessNote, "missing footprint") {
		t.Fatalf("rows = %#v, want combined partial readiness note", rows)
	}
	if rows[1].Confidence != "partial" || !strings.Contains(rows[1].ReadinessNote, "missing footprint") {
		t.Fatalf("rows = %#v, want missing footprint to keep confidence partial", rows)
	}
	if !hasIssueCode(issues, reports.CodeMissingFootprint) || !hasIssueCode(issues, reports.CodeValidationFailed) {
		t.Fatalf("issues = %#v, want footprint and manufacturer/MPN issues", issues)
	}
}

func TestBuildCPLRowsSortsAndFormatsPlacement(t *testing.T) {
	rows := BuildCPLRows(pcbfiles.PCBFile{Footprints: []pcbfiles.Footprint{
		{Reference: "U1", LibraryID: "Package:QFN", Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(5)}, Rotation: 90, Layer: kicadfiles.LayerBCu, Locked: true},
		{Reference: "C1", LibraryID: "Capacitor:C_0603", Position: kicadfiles.Point{X: kicadfiles.MM(1.5), Y: kicadfiles.MM(2.25)}, Layer: kicadfiles.LayerFCu},
		{Reference: "R1", LibraryID: "Resistor:R_0603", Attributes: []string{"dnp"}, Position: kicadfiles.Point{X: kicadfiles.MM(3), Y: kicadfiles.MM(4)}, Layer: kicadfiles.LayerFCu},
	}})
	if len(rows) != 2 || rows[0].Reference != "C1" || rows[1].Reference != "U1" {
		t.Fatalf("rows = %#v, want reference-sorted CPL", rows)
	}
	if rows[0].XMM != "1.5" || rows[0].YMM != "2.25" || rows[1].Layer != "bottom" || !rows[1].Fixed {
		t.Fatalf("rows = %#v, want formatted coordinates/layer/fixed state", rows)
	}
}

func TestReportCSVSerializationEscapesFields(t *testing.T) {
	bom, err := MarshalBOMCSV([]BOMRow{{
		References:    []string{"R1"},
		Quantity:      1,
		Value:         "10k, 1%",
		SymbolID:      "Device:R",
		FootprintID:   "Resistor:R_0603",
		Manufacturer:  "Example",
		MPN:           "ABC-1",
		Confidence:    "high",
		ReadinessNote: "quoted \"note\"",
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(bom)
	if !strings.Contains(text, `"10k, 1%"`) || !strings.Contains(text, `"quoted ""note"""`) {
		t.Fatalf("BOM CSV not escaped correctly:\n%s", text)
	}
	cpl, err := MarshalCPLCSV([]CPLRow{{Reference: "U1", Footprint: "Package:QFN", XMM: "1", YMM: "2", RotationDegrees: "90.000", Layer: "top", PlacementSource: "pcb"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cpl), "Reference,Footprint") || !strings.Contains(string(cpl), "U1,Package:QFN") {
		t.Fatalf("CPL CSV unexpected:\n%s", cpl)
	}
}

func TestSummaryFilePathResolvesRelativePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	got := summaryFilePath(root, []inspect.FileSummary{{Kind: "schematic", Path: "nested/demo.kicad_sch"}}, "schematic")
	want := filepath.Join(root, "nested/demo.kicad_sch")
	if got != want {
		t.Fatalf("summaryFilePath = %q, want %q", got, want)
	}
}

func TestReadSchematicsRecursiveIncludesChildSheets(t *testing.T) {
	root := t.TempDir()
	child := testSchematic()
	child.Symbols = []schematicfiles.SchematicSymbol{
		testBOMSymbol("R1", "10k", "Device:R", "Resistor_SMD:R_0603", "Yageo", "RC0603FR-0710KL"),
	}
	child.Symbols[0].UUID = kicadfiles.UUID("11111111-1111-4111-8111-111111111111")
	writeSchematicFile(t, filepath.Join(root, "child.kicad_sch"), child)
	parent := testSchematic()
	parent.Sheets = []schematicfiles.Sheet{{UUID: kicadfiles.UUID("22222222-2222-4222-8222-222222222222"), Name: "child", Filename: "child.kicad_sch", Size: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}}}
	parentPath := filepath.Join(root, "parent.kicad_sch")
	writeSchematicFile(t, parentPath, parent)

	merged, err := readSchematicsRecursive(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	rows, issues := BuildBOMRows(merged)
	if len(issues) != 0 || len(rows) != 1 || strings.Join(rows[0].References, " ") != "R1" {
		t.Fatalf("rows=%#v issues=%#v, want child sheet symbol in BOM", rows, issues)
	}
}

func TestReadSchematicsRecursiveRejectsRepeatedSheetFiles(t *testing.T) {
	root := t.TempDir()
	child := testSchematic()
	child.Symbols = []schematicfiles.SchematicSymbol{
		testBOMSymbol("R1", "10k", "Device:R", "Resistor_SMD:R_0603", "Yageo", "RC0603FR-0710KL"),
	}
	child.Symbols[0].UUID = kicadfiles.UUID("33333333-3333-4333-8333-333333333333")
	writeSchematicFile(t, filepath.Join(root, "child.kicad_sch"), child)
	parent := testSchematic()
	parent.Sheets = []schematicfiles.Sheet{
		{UUID: kicadfiles.UUID("44444444-4444-4444-8444-444444444444"), Name: "child_a", Filename: "child.kicad_sch", Size: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}},
		{UUID: kicadfiles.UUID("55555555-5555-4555-8555-555555555555"), Name: "child_b", Filename: "child.kicad_sch", Size: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}},
	}
	parentPath := filepath.Join(root, "parent.kicad_sch")
	writeSchematicFile(t, parentPath, parent)

	if _, err := readSchematicsRecursive(parentPath); err == nil || !strings.Contains(err.Error(), "reused hierarchical sheet") {
		t.Fatalf("err = %v, want reused hierarchical sheet error", err)
	}
}

func TestReadSchematicsRecursiveReportsCycles(t *testing.T) {
	root := t.TempDir()
	cyclic := testSchematic()
	cyclic.Sheets = []schematicfiles.Sheet{{UUID: kicadfiles.UUID("66666666-6666-4666-8666-666666666666"), Name: "self", Filename: "self.kicad_sch", Size: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}}}
	path := filepath.Join(root, "self.kicad_sch")
	writeSchematicFile(t, path, cyclic)
	if _, err := readSchematicsRecursive(path); err == nil || !strings.Contains(err.Error(), "circular sheet reference") {
		t.Fatalf("err = %v, want circular reference error", err)
	}
}

func testBOMSymbol(reference, value, libraryID, footprint, manufacturer, mpn string) schematicfiles.SchematicSymbol {
	return schematicfiles.SchematicSymbol{
		Reference: reference,
		Value:     value,
		LibraryID: libraryID,
		Properties: []schematicfiles.Property{
			{Name: "Footprint", Value: footprint},
			{Name: "Manufacturer", Value: manufacturer},
			{Name: "MPN", Value: mpn},
		},
	}
}

func testSchematic() schematicfiles.SchematicFile {
	return schematicfiles.SchematicFile{
		Version:          kicadfiles.KiCadSchematicFormatV20260306,
		Generator:        "kicadai-test",
		GeneratorVersion: "test",
		UUID:             kicadfiles.UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		Paper:            kicadfiles.Paper{Name: "A4"},
	}
}

func writeSchematicFile(t *testing.T, path string, schematic schematicfiles.SchematicFile) {
	t.Helper()
	var buf bytes.Buffer
	if err := schematicfiles.Write(&buf, schematic); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
