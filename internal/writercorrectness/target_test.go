package writercorrectness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestValidateProjectStructureDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)

	result := ValidateProjectStructure(dir, Options{})
	if !result.OK {
		t.Fatalf("OK = false, issues = %#v", result.Issues)
	}
	if result.Target.ProjectPath == "" || result.Target.SchematicPath == "" {
		t.Fatalf("target did not resolve project and schematic: %#v", result.Target)
	}
	if len(result.Target.SchematicFiles) != 1 {
		t.Fatalf("schematic files = %d, want 1", len(result.Target.SchematicFiles))
	}
}

func TestValidateProjectStructureSchematicOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	writeSchematic(t, path, nil)

	result := ValidateProjectStructure(path, Options{})
	if !result.OK {
		t.Fatalf("OK = false, issues = %#v", result.Issues)
	}
	if result.Target.ProjectPath != "" {
		t.Fatalf("project path = %q, want empty", result.Target.ProjectPath)
	}
	if result.Target.SchematicPath == "" {
		t.Fatalf("schematic path not resolved")
	}
}

func TestValidateProjectStructureDiscoversHierarchicalSheets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "child.kicad_sch"), nil)
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), []string{"child.kicad_sch"})

	result := ValidateProjectStructure(dir, Options{})
	if !result.OK {
		t.Fatalf("OK = false, issues = %#v", result.Issues)
	}
	if len(result.Target.SchematicFiles) != 2 {
		t.Fatalf("schematic files = %#v, want root and child", result.Target.SchematicFiles)
	}
}

func TestValidateProjectStructureReportsMissingChildSheet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), []string{"missing.kicad_sch"})

	result := ValidateProjectStructure(dir, Options{})
	if result.OK {
		t.Fatalf("OK = true, want false")
	}
	assertIssueContains(t, result, "hierarchical child sheet file not found")
}

func TestValidateProjectStructureBlocksChildSheetOutsideProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), []string{"../outside.kicad_sch"})

	result := ValidateProjectStructure(dir, Options{})
	if result.OK {
		t.Fatalf("OK = true, want false")
	}
	assertIssueContains(t, result, "resolves outside project directory")
}

func TestValidateProjectStructureReportsMismatchedBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "other.kicad_sch"), nil)

	result := ValidateProjectStructure(filepath.Join(dir, "other.kicad_sch"), Options{})
	if !result.OK {
		t.Fatalf("schematic-only target should pass without project mismatch: %#v", result.Issues)
	}

	writeFile(t, filepath.Join(dir, "other.kicad_pro"), "{}")
	result = ValidateProjectStructure(filepath.Join(dir, "other.kicad_sch"), Options{})
	if !result.OK {
		t.Fatalf("same-base project/schematic should pass: %#v", result.Issues)
	}

	writeFile(t, filepath.Join(dir, "other.kicad_pcb"), minimalPCB())
	result = ValidateProjectStructure(filepath.Join(dir, "demo.kicad_pro"), Options{})
	if result.OK {
		t.Fatalf("OK = true, want false for project/schematic basename mismatch")
	}
	assertIssueContains(t, result, "matching root schematic file not found")
}

func TestValidateProjectStructureValidatesLocalLibraryTablePaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)
	writeFile(t, filepath.Join(dir, "fp-lib-table"), `(fp_lib_table
  (version 7)
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/missing.pretty") (options "") (descr ""))
)`)

	result := ValidateProjectStructure(dir, Options{})
	if result.OK {
		t.Fatalf("OK = true, want false")
	}
	assertIssueContains(t, result, "library table URI does not resolve")
}

func TestValidateProjectStructureAcceptsFootprintLibraryDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)
	if err := os.Mkdir(filepath.Join(dir, "Local.pretty"), 0o755); err != nil {
		t.Fatalf("mkdir local footprint library: %v", err)
	}
	writeFile(t, filepath.Join(dir, "fp-lib-table"), `(fp_lib_table
  (version 7)
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/Local.pretty") (options "") (descr ""))
)`)

	result := ValidateProjectStructure(dir, Options{})
	for _, issue := range result.Issues {
		if issue.Code == reports.CodeMissingFile && strings.Contains(issue.Message, "library table URI") {
			t.Fatalf("unexpected local footprint library issue: %#v", issue)
		}
	}
}

func TestValidateProjectStructureRejectsSymbolLibraryDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)
	if err := os.Mkdir(filepath.Join(dir, "LocalSymbols"), 0o755); err != nil {
		t.Fatalf("mkdir local symbol library dir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "sym-lib-table"), `(sym_lib_table
  (version 7)
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/LocalSymbols") (options "") (descr ""))
)`)

	result := ValidateProjectStructure(dir, Options{})
	if result.OK {
		t.Fatalf("OK = true, want false")
	}
	assertIssueContains(t, result, "library table URI does not resolve")
}

func TestValidateProjectStructureWarnsOnUntrustedLibraryPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)
	writeFile(t, filepath.Join(dir, "fp-lib-table"), `(fp_lib_table
  (version 7)
  (lib (name "Outside") (type "KiCad") (uri "/etc") (options "") (descr ""))
)`)

	result := ValidateProjectStructure(dir, Options{})
	if !result.OK {
		t.Fatalf("untrusted path should warn, not block: %#v", result.Issues)
	}
	assertIssueContains(t, result, "outside allowed roots")
}

func TestValidateProjectStructureReportsUnresolvedLibraryVariable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeSchematic(t, filepath.Join(dir, "demo.kicad_sch"), nil)
	writeFile(t, filepath.Join(dir, "fp-lib-table"), `(fp_lib_table
  (version 7)
  (lib (name "MissingVar") (type "KiCad") (uri "${KICADAI_MISSING_TEST_VAR}/foo.pretty") (options "") (descr ""))
)`)

	result := ValidateProjectStructure(dir, Options{})
	if !result.OK {
		t.Fatalf("unresolved external variable should warn, not block: %#v", result.Issues)
	}
	assertIssueContains(t, result, "contains unresolved variables")
}

func TestExpandKiCadVariablesDoesNotRecurseEnvironmentValues(t *testing.T) {
	got, ok := expandKiCadVariables("${LOOP}/foo.pretty", "/project", map[string]string{"LOOP": "${LOOP}"})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got != "${LOOP}/foo.pretty" {
		t.Fatalf("expanded = %q", got)
	}
}

func TestExpandKiCadVariablesProjectVariablesOverrideEnvironment(t *testing.T) {
	got, ok := expandKiCadVariables("${KIPRJMOD}/foo.pretty", "/project", variableReplacements("/project", map[string]string{"KIPRJMOD": "/wrong"}))
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got != "/project/foo.pretty" {
		t.Fatalf("expanded = %q", got)
	}
}

func assertIssueContains(t *testing.T, result Result, want string) {
	t.Helper()
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, want) {
			return
		}
	}
	t.Fatalf("missing issue containing %q in %#v", want, result.Issues)
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeSchematic(t *testing.T, path string, sheets []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`(kicad_sch
  (version 20260306)
  (generator "kicadai-test")
  (uuid "00000000-0000-0000-0000-000000000001")
  (paper "A4")
`)
	for i, sheet := range sheets {
		b.WriteString(`  (sheet (at 10 10) (size 20 20)
    (property "Sheetname" "Sheet`)
		b.WriteString(string(rune('A' + i)))
		b.WriteString(`" (at 10 10 0))
    (property "Sheetfile" "`)
		b.WriteString(sheet)
		b.WriteString(`" (at 10 15 0))
  )
`)
	}
	b.WriteString(")\n")
	writeFile(t, path, b.String())
}

func minimalPCB() string {
	return `(kicad_pcb
  (version 20260206)
  (generator "kicadai-test")
  (general (thickness 1.6))
  (paper "A4")
  (layers
    (0 "F.Cu" signal)
    (31 "B.Cu" signal)
    (32 "B.Adhes" user)
    (33 "F.Adhes" user)
    (34 "B.Paste" user)
    (35 "F.Paste" user)
    (36 "B.SilkS" user)
    (37 "F.SilkS" user)
    (38 "B.Mask" user)
    (39 "F.Mask" user)
    (44 "Edge.Cuts" user)
  )
  (net 0 "")
)`
}
