package pinmap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

func TestValidateProjectVerifiedMappingPassesAndSurfacesNotes(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (at 10 10 0)
    (property "Reference" "R1")
    (property "Value" "10k")
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "2" (uuid "22222222-2222-5222-8222-222222222222"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	report, err := ValidateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.FabricationReady || len(report.Issues) != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(report.Mappings) != 1 || report.Mappings[0].Status != "verified" || report.Mappings[0].Notes == "" {
		t.Fatalf("mapping missing verified status/notes: %#v", report.Mappings)
	}
}

func TestValidateProjectMissingMappingBlocksFabricationReadiness(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:U")
    (property "Reference" "U1")
    (property "Footprint" "Package_SO:SOIC-8")
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	report, err := ValidateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Issues) != 1 || report.Issues[0].Code != reports.CodePinmapUnverified {
		t.Fatalf("expected blocking pinmap issue: %#v", report)
	}
}

func TestValidateProjectPinCountMismatchBlocks(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (property "Reference" "R1")
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	report, err := ValidateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Issues) != 1 || report.Mappings[0].Status != "mismatch" {
		t.Fatalf("expected pin count mismatch: %#v", report)
	}
}

func TestValidateProjectPinIdentifierMismatchBlocks(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (property "Reference" "R1")
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "3" (uuid "22222222-2222-5222-8222-222222222223"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	report, err := ValidateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Issues) != 1 || report.Mappings[0].Status != "mismatch" {
		t.Fatalf("expected pin identifier mismatch: %#v", report)
	}
}

func TestValidateProjectHierarchyBlocksReadiness(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "child")
    (property "Sheetfile" "child.kicad_sch")
    (uuid "22222222-2222-5222-8222-222222222222"))
)`)
	if err := os.WriteFile(filepath.Join(dir, "child.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "33333333-3333-5333-8333-333333333333")
  (paper A4)
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := ValidateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Issues) != 1 || report.Issues[0].Code != reports.CodeUnsupportedOperation {
		t.Fatalf("expected hierarchy blocker: %#v", report)
	}
}

func TestBuiltinsIncludesHumanVerifiedEntries(t *testing.T) {
	entries := Builtins()
	if len(entries) == 0 {
		t.Fatal("expected built-in pinmap entries")
	}
	for _, entry := range entries {
		if entry.Source != "human_verified" || entry.Notes == "" {
			t.Fatalf("entry missing source/notes: %#v", entry)
		}
	}
}

func TestBuiltinsIncludeExpandedI2CSensorPinmaps(t *testing.T) {
	want := map[string]int{
		mappingKey("Sensor_Pressure:BMP280", "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering"):       8,
		mappingKey("Sensor_Humidity:SHT31-DIS", "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm"): 9,
	}
	for _, entry := range Builtins() {
		key := mappingKey(entry.Symbol, entry.Footprint)
		count, ok := want[key]
		if !ok {
			continue
		}
		if len(entry.Pins) != count {
			t.Fatalf("%s pin count = %d, want %d", key, len(entry.Pins), count)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing expanded sensor pinmaps: %#v", want)
	}
}

func TestBuiltinsReturnsDeepCopy(t *testing.T) {
	entries := Builtins()
	entries[0].Pins[0].SymbolPin = "mutated"
	again := Builtins()
	if again[0].Pins[0].SymbolPin == "mutated" {
		t.Fatalf("Builtins returned shared pin slice")
	}
}

func TestValidateProjectWithResolverVerifiesFootprintPadsAndEvidence(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (property "Reference" "R1")
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "2" (uuid "22222222-2222-5222-8222-222222222222"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	index := writePinmapResolverIndex(t)
	report, err := ValidateProjectWithOptions(dir, ValidateOptions{LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if !report.FabricationReady || len(report.Issues) != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(report.Mappings) != 1 || report.Mappings[0].Status != "verified" || len(report.Mappings[0].ResolverEvidence) == 0 {
		t.Fatalf("mapping missing resolver evidence: %#v", report.Mappings)
	}
}

func TestValidateProjectWithResolverMissingFootprintBlocks(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (property "Reference" "R1")
    (property "Footprint" "Missing:Nope")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "2" (uuid "22222222-2222-5222-8222-222222222222"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	index := writePinmapResolverIndex(t)
	report, err := ValidateProjectWithOptions(dir, ValidateOptions{LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Issues) == 0 || report.Mappings[0].Status != "missing" {
		t.Fatalf("expected missing resolver footprint blocker: %#v", report)
	}
}

func TestValidateProjectWithResolverKeepsBuiltinStatusWhenLibraryIncomplete(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:C")
    (property "Reference" "C1")
    (property "Footprint" "Capacitor_SMD:C_0805_2012Metric")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "2" (uuid "22222222-2222-5222-8222-222222222222"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	index := writePinmapResolverIndex(t)
	report, err := ValidateProjectWithOptions(dir, ValidateOptions{LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Mappings) != 1 || report.Mappings[0].Status != "verified" || len(report.Issues) == 0 {
		t.Fatalf("expected verified builtin mapping with resolver missing-file issues: %#v", report)
	}
}

func TestValidateProjectWithResolverCandidateForUnverifiedMapping(t *testing.T) {
	dir := writePinmapProject(t, `(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:X")
    (property "Reference" "X1")
    (property "Footprint" "Test:TwoPad")
    (pin "1" (uuid "22222222-2222-5222-8222-222222222221"))
    (pin "2" (uuid "22222222-2222-5222-8222-222222222222"))
    (uuid "33333333-3333-5333-8333-333333333333"))
)`)
	index := writePinmapResolverIndex(t)
	report, err := ValidateProjectWithOptions(dir, ValidateOptions{LibraryIndex: &index})
	if err != nil {
		t.Fatal(err)
	}
	if report.FabricationReady || len(report.Mappings) != 1 || report.Mappings[0].Status != "candidate" || len(report.Mappings[0].CandidatePinmap) != 2 {
		t.Fatalf("expected resolver candidate: %#v", report)
	}
	if len(report.Issues) == 0 || report.Issues[0].Code != reports.CodePinmapUnverified {
		t.Fatalf("expected unverified candidate issue: %#v", report.Issues)
	}
}

func writePinmapProject(t *testing.T, schematicContents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_sch"), []byte(schematicContents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writePinmapResolverIndex(t *testing.T) libraryresolver.LibraryIndex {
	t.Helper()
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	if err := os.MkdirAll(symbols, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(footprints, "Resistor_SMD.pretty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(footprints, "Test.pretty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(symbols, "Device.kicad_sym"), []byte(pinmapSymbolLibrary()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), []byte(pinmapResistorFootprint()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(footprints, "Test.pretty", "TwoPad.kicad_mod"), []byte(pinmapTwoPadFootprint()), 0o644); err != nil {
		t.Fatal(err)
	}
	index, issues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints}, libraryresolver.LoadOptions{})
	for _, issue := range issues {
		if issue.Blocking() {
			t.Fatalf("resolver issues = %#v", issues)
		}
	}
	return index
}

func pinmapSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (ki_keywords "resistor R")
    (ki_description "Resistor")
    (ki_fp_filters "R_*" "Resistor_*")
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
  (symbol "X"
    (property "Reference" "X" (at 0 0 0))
    (property "Value" "X" (at 0 -2.54 0))
    (ki_keywords "test")
    (ki_description "Test two pin part")
    (symbol "X_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "B") (number "2"))
    )
  )
)`
}

func pinmapResistorFootprint() string {
	return `
(footprint "R_0805_2012Metric"
  (descr "resistor")
  (tags "resistor")
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "2" smd rect (at 2 0) (size 1 1) (layers "F.Cu"))
)`
}

func pinmapTwoPadFootprint() string {
	return `
(footprint "TwoPad"
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "2" smd rect (at 2 0) (size 1 1) (layers "F.Cu"))
)`
}
