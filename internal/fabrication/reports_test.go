package fabrication

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/components"
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

func TestBuildBOMRowsHydratesIdentityEvidence(t *testing.T) {
	rows, issues := BuildBOMRows(schematicfiles.SchematicFile{Symbols: []schematicfiles.SchematicSymbol{{
		Reference: "U1",
		Value:     "MCU",
		LibraryID: "MCU:ATmega328P",
		Properties: []schematicfiles.Property{
			{Name: "Footprint", Value: "Package_QFP:TQFP-32"},
			{Name: "Component ID", Value: "mcu.atmega328p-au"},
			{Name: "Manufacturer Part Number", Value: "ATMEGA328P-AU"},
			{Name: "Manufacturer", Value: "Microchip"},
			{Name: "Package", Value: "TQFP-32"},
			{Name: "Component Class", Value: "Active"},
			{Name: "Lifecycle", Value: "active"},
			{Name: "Confidence", Value: "verified"},
		},
	}}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	row := rows[0]
	if row.ComponentID != "mcu.atmega328p-au" || row.Package != "TQFP-32" || row.ComponentClass != "active" || row.Lifecycle != "active" {
		t.Fatalf("identity row = %#v", row)
	}
	if row.IdentityStatus != IdentityPass || row.IdentitySource != IdentitySourceSchematicProperty {
		t.Fatalf("identity evidence = %#v", row)
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

func TestBuildCPLRowsWithBOMHydratesIdentityAndNormalizesPlacement(t *testing.T) {
	rows, issues := BuildCPLRowsWithBOM(
		pcbfiles.PCBFile{Footprints: []pcbfiles.Footprint{
			{Reference: "U1", LibraryID: "Package_QFP:TQFP-32", Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(5)}, Rotation: 450, Layer: kicadfiles.LayerBCu},
			{Reference: "R1", LibraryID: "Resistor_SMD:R_0603", Position: kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(3)}, Rotation: -90, Layer: kicadfiles.LayerFCu},
		}},
		[]BOMRow{
			{
				References:   []string{"U1"},
				Value:        "MCU",
				SymbolID:     "MCU:ATmega328P",
				FootprintID:  "Package_QFP:TQFP-32",
				ComponentID:  "mcu.atmega328p-au",
				Manufacturer: "Microchip",
				MPN:          "ATMEGA328P-AU",
			},
			{
				References:  []string{"R1"},
				Value:       "10k",
				SymbolID:    "Device:R",
				FootprintID: "Resistor_SMD:R_0603",
			},
		},
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].Reference != "R1" || rows[0].Layer != "top" || rows[0].NormalizedRotationDegrees != "270.000" {
		t.Fatalf("R1 CPL row = %#v, want top side and normalized -90 rotation", rows[0])
	}
	u1 := rows[1]
	if u1.ComponentID != "mcu.atmega328p-au" || u1.Manufacturer != "Microchip" || u1.MPN != "ATMEGA328P-AU" {
		t.Fatalf("U1 CPL identity = %#v", u1)
	}
	if u1.BOMLinkageStatus != "linked" || u1.IdentityKey != "mcu.atmega328p-au" || u1.Layer != "bottom" || u1.RawRotationDegrees != "450.000" || u1.NormalizedRotationDegrees != "90.000" {
		t.Fatalf("U1 CPL placement = %#v", u1)
	}
}

func TestBuildCPLRowsWithBOMReportsUnknownSide(t *testing.T) {
	rows, issues := BuildCPLRowsWithBOM(pcbfiles.PCBFile{Footprints: []pcbfiles.Footprint{{
		Reference: "U1",
		LibraryID: "Package:QFN",
		Layer:     kicadfiles.LayerFSilkS,
	}}}, nil)
	if len(rows) != 1 || rows[0].Layer != "unknown" || !strings.Contains(rows[0].ReadinessNote, "unknown placement side") {
		t.Fatalf("rows = %#v, want unknown side readiness note", rows)
	}
	if !hasIssueCode(issues, reports.CodeValidationFailed) || !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want blocking unknown side issue", issues)
	}
}

func TestValidateBOMCPLConsistencyPassesMatchingAssemblySet(t *testing.T) {
	summary, issues := ValidateBOMCPLConsistency(
		[]BOMRow{{References: []string{"R1", "R2"}, FootprintID: "Resistor_SMD:R_0603"}},
		[]CPLRow{
			{Reference: "R1", Footprint: "Resistor_SMD:R_0603", XMM: "1", YMM: "2", Layer: "top", NormalizedSide: "top"},
			{Reference: "R2", Footprint: "Resistor_SMD:R_0603", XMM: "3", YMM: "4", Layer: "bottom", NormalizedSide: "bottom"},
		},
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
	if summary.CheckedReferences != 2 || summary.MatchedReferences != 2 || summary.BlockingCount != 0 {
		t.Fatalf("summary = %#v, want two matched references", summary)
	}
}

func TestValidateBOMCPLConsistencyBlocksMissingAndExtraReferences(t *testing.T) {
	summary, issues := ValidateBOMCPLConsistency(
		[]BOMRow{{References: []string{"R1"}, FootprintID: "Resistor:R_0603"}},
		[]CPLRow{{Reference: "C1", Footprint: "Capacitor:C_0603", XMM: "1", YMM: "2", Layer: "top", NormalizedSide: "top"}},
	)
	if summary.CheckedReferences != 2 || summary.MatchedReferences != 0 || summary.BlockingCount != 2 {
		t.Fatalf("summary = %#v, want one missing and one extra blocking issue", summary)
	}
	if !reports.HasBlockingIssue(issues) || !hasIssuePath(issues, "cpl.R1") || !hasIssuePath(issues, "bom.C1") {
		t.Fatalf("issues = %#v, want missing CPL and extra CPL issues", issues)
	}
}

func TestValidateBOMCPLConsistencyBlocksDuplicatesMismatchAndPlacementEvidence(t *testing.T) {
	_, issues := ValidateBOMCPLConsistency(
		[]BOMRow{
			{References: []string{"U1"}, FootprintID: "Package:QFN"},
			{References: []string{"U1"}, FootprintID: "Package:QFN"},
		},
		[]CPLRow{
			{Reference: "U1", Footprint: "Package:TQFP", Layer: "unknown", NormalizedSide: "unknown"},
			{Reference: "U1", Footprint: "Package:TQFP", XMM: "1", YMM: "2", Layer: "top", NormalizedSide: "top"},
		},
	)
	if !hasIssueCode(issues, reports.CodeDuplicateReference) {
		t.Fatalf("issues = %#v, want duplicate reference issue", issues)
	}
	for _, path := range []string{"cpl.U1.footprint", "cpl.U1.position", "cpl.U1.layer"} {
		if !hasIssuePath(issues, path) {
			t.Fatalf("issues = %#v, want %s", issues, path)
		}
	}
}

func TestReportCSVSerializationEscapesFields(t *testing.T) {
	fresh := true
	bom, err := MarshalBOMCSV([]BOMRow{{
		References:             []string{"R1"},
		Quantity:               1,
		Value:                  "10k, 1%",
		SymbolID:               "Device:R",
		FootprintID:            "Resistor:R_0603",
		Manufacturer:           "Example",
		MPN:                    "ABC-1",
		ProcurementSourceID:    "curated",
		LifecycleSourceDate:    "2026-06-26",
		LifecycleFresh:         &fresh,
		AvailabilityStatus:     "not_checked",
		AvailabilitySourceDate: "2026-06-26",
		AvailabilityFresh:      &fresh,
		ProcurementOutcome:     "snapshot",
		Confidence:             "high",
		ReadinessNote:          "quoted \"note\"",
	}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(bom)
	if !strings.Contains(text, `"10k, 1%"`) || !strings.Contains(text, `"quoted ""note"""`) {
		t.Fatalf("BOM CSV not escaped correctly:\n%s", text)
	}
	if !strings.Contains(text, "Package,ComponentClass,Lifecycle,Confidence,IdentityStatus,IdentitySource,IdentityIssueCount,IdentityBlockingCount,ReadinessNote") {
		t.Fatalf("BOM CSV missing identity columns:\n%s", text)
	}
	if !strings.Contains(text, "ReadinessNote,ProcurementSourceID,LifecycleSourceDate,LifecycleFresh,AvailabilityStatus,AvailabilitySourceDate,AvailabilityFresh,ProcurementOutcome") {
		t.Fatalf("BOM CSV missing appended procurement columns:\n%s", text)
	}
	cpl, err := MarshalCPLCSV([]CPLRow{{Reference: "U1", Footprint: "Package:QFN", ComponentID: "ic.example", XMM: "1", YMM: "2", RotationDegrees: "90.000", Layer: "top", NormalizedSide: "top", RawLayer: "F.Cu", RawRotationDegrees: "450.000", NormalizedRotationDegrees: "90.000", BOMLinkageStatus: "linked", PlacementSource: "pcb"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cpl), "Reference,Footprint,ComponentID") || !strings.Contains(string(cpl), "U1,Package:QFN,ic.example") || !strings.Contains(string(cpl), "NormalizedRotation,BOMLinkageStatus") {
		t.Fatalf("CPL CSV unexpected:\n%s", cpl)
	}
}

func TestEnrichBOMRowsWithProcurementSnapshots(t *testing.T) {
	sources := loadFabricationSourceFixture(t, "valid")
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	rows, issues := EnrichBOMRowsWithProcurement([]BOMRow{{
		References:   []string{"U1"},
		Manufacturer: "Diodes Incorporated",
		MPN:          "AP2112K-3.3",
	}, {
		References:   []string{"R1"},
		ComponentID:  "resistor.yageo.rc0805fr_0710kl.0805",
		Manufacturer: "Yageo",
		MPN:          "RC0805FR-0710KL",
	}}, sources, components.ProcurementPolicy{Now: &now})
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	for _, row := range rows {
		if row.ProcurementSourceID != "curated_seed_procurement" || row.Lifecycle != "active" || row.AvailabilityStatus != "not_checked" || row.ProcurementOutcome != "snapshot" {
			t.Fatalf("row = %#v", row)
		}
		if row.LifecycleFresh == nil || !*row.LifecycleFresh {
			t.Fatalf("lifecycle freshness missing: %#v", row)
		}
	}
	if !hasIssuePath(issues, "bom.U1.availability") || !hasIssuePath(issues, "bom.R1.availability") || reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want advisory availability warning only", issues)
	}
}

func TestEnrichBOMRowsWithProcurementRequiredAvailabilityBlocks(t *testing.T) {
	sources := loadFabricationSourceFixture(t, "valid")
	now := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	_, issues := EnrichBOMRowsWithProcurement([]BOMRow{{
		References:   []string{"U1"},
		Manufacturer: "Diodes Incorporated",
		MPN:          "AP2112K-3.3",
	}}, sources, components.ProcurementPolicy{RequireAvailability: true, Now: &now})
	if !reports.HasBlockingIssue(issues) || !hasIssuePath(issues, "bom.U1.availability") {
		t.Fatalf("issues = %#v, want required availability blocker", issues)
	}
}

func loadFabricationSourceFixture(t *testing.T, name string) *components.SourceCollection {
	t.Helper()
	dir := filepath.Join("..", "components", "testdata", "sources", name)
	sources, err := components.LoadSources(context.Background(), components.SourceLoadOptions{SourceDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return sources
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
