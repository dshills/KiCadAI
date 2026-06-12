package pinmap

import (
	"os"
	"path/filepath"
	"testing"

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

func TestBuiltinsReturnsDeepCopy(t *testing.T) {
	entries := Builtins()
	entries[0].Pins[0].SymbolPin = "mutated"
	again := Builtins()
	if again[0].Pins[0].SymbolPin == "mutated" {
		t.Fatalf("Builtins returned shared pin slice")
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
