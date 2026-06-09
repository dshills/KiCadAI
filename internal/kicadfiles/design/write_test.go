package design

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/schematic"
)

func TestWriteProjectDirectoryCreatesFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	result, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	if len(result.WrittenFiles) != 3 {
		t.Fatalf("written files = %v", result.WrittenFiles)
	}
	for _, name := range []string{"led_indicator.kicad_pro", "led_indicator.kicad_sch", "led_indicator.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestWriteProjectDirectoryRefusesExistingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "target exists") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryAllowsOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{Overwrite: true})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	if result.BackupDir != "" || result.JournalPath != "" {
		t.Fatalf("cleanup paths should be cleared after successful overwrite: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old file still exists or unexpected stat error: %v", err)
	}
}

func TestWriteProjectDirectoryOmitsPCBWhenNil(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	result, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("written files = %v", result.WrittenFiles)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pcb")); !os.IsNotExist(err) {
		t.Fatalf("PCB should be omitted, stat error: %v", err)
	}
}

func TestWriteProjectDirectoryWritesLibraryTables(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	design := validLEDDesign(t)
	design.SymbolTables = []library.TableEntry{{
		Name: "local_symbols",
		Type: "KiCad",
		URI:  "${KIPRJMOD}/lib/local_symbols.kicad_sym",
	}}
	design.FootprintTables = []library.TableEntry{{
		Name: "local_footprints",
		Type: "KiCad",
		URI:  "${KIPRJMOD}/footprints.pretty",
	}}

	result, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	for _, name := range []string{"sym-lib-table", "fp-lib-table"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	for _, want := range []string{filepath.Join(root, "sym-lib-table"), filepath.Join(root, "fp-lib-table")} {
		if !containsString(result.WrittenFiles, want) {
			t.Fatalf("written files missing %s: %v", want, result.WrittenFiles)
		}
	}
}

func TestWriteProjectDirectoryWritesChildSheetFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hierarchical")
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Power",
		Filename: "sch/power.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("sch/power.kicad_sch")
	design.SheetFiles = []*schematic.SchematicFile{&child}

	result, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	childPath := filepath.Join(root, "sch", "power.kicad_sch")
	if _, err := os.Stat(childPath); err != nil {
		t.Fatalf("child sheet missing: %v", err)
	}
	if !containsString(result.WrittenFiles, childPath) {
		t.Fatalf("written files missing child: %v", result.WrittenFiles)
	}
}

func TestWriteProjectDirectoryWritesOptionalArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	design := validLEDDesign(t)
	design.RuleFiles = []TextArtifact{{Path: "rules/demo.kicad_dru", Contents: []byte("(version 1)\n")}}
	design.WorksheetFiles = []TextArtifact{{Path: "layout/page.kicad_wks", Contents: []byte("(page_layout)\n")}}
	design.AssetFiles = []TextArtifact{{Path: "models/readme.txt", Contents: []byte("asset\n")}}

	if _, err := WriteProjectDirectory(root, design, WriteOptions{}); err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	for _, name := range []string{"rules/demo.kicad_dru", "layout/page.kicad_wks", "models/readme.txt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestWriteProjectDirectoryDemoLikeStructure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo-output")
	design := validLEDDesign(t)
	design.Name = "demo_project"
	design.Project.Name = "demo_project"
	design.Schematic.Filename = "demo_project.kicad_sch"
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Power",
		Filename: "sch/power.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("sch/power.kicad_sch")
	design.SheetFiles = []*schematic.SchematicFile{&child}
	design.SymbolTables = []library.TableEntry{{Name: "local_symbols", Type: "KiCad", URI: "${KIPRJMOD}/lib/local_symbols.kicad_sym"}}
	design.FootprintTables = []library.TableEntry{{Name: "local_footprints", Type: "KiCad", URI: "${KIPRJMOD}/footprints.pretty"}}
	design.RuleFiles = []TextArtifact{{Path: "rules/demo.kicad_dru", Contents: []byte("(version 1)\n")}}
	design.WorksheetFiles = []TextArtifact{{Path: "layout/demo.kicad_wks", Contents: []byte("(page_layout)\n")}}

	if _, err := WriteProjectDirectory(root, design, WriteOptions{}); err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	want := []string{
		"demo_project.kicad_pro",
		"demo_project.kicad_sch",
		"fp-lib-table",
		"layout/demo.kicad_wks",
		"rules/demo.kicad_dru",
		"sch/power.kicad_sch",
		"sym-lib-table",
	}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Fatalf("files = %v, want %v", files, want)
	}
	projectBytes, err := os.ReadFile(filepath.Join(root, "demo_project.kicad_pro"))
	if err != nil {
		t.Fatal(err)
	}
	var projectJSON map[string]any
	if err := json.Unmarshal(projectBytes, &projectJSON); err != nil {
		t.Fatalf("invalid project JSON: %v", err)
	}
	for _, key := range []string{"board", "boards", "erc", "libraries", "net_settings", "pcbnew", "schematic", "sheets", "text_variables", "time_domain_parameters"} {
		if _, ok := projectJSON[key]; !ok {
			t.Fatalf("project JSON missing key %s", key)
		}
	}
	rootSchematic, err := os.ReadFile(filepath.Join(root, "demo_project.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootSchematic), "\"sch/power.kicad_sch\"") {
		t.Fatalf("root schematic missing child sheet reference:\n%s", rootSchematic)
	}
}

func TestWriteProjectDirectoryRejectsInvalidArtifactExtension(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	design := validLEDDesign(t)
	design.RuleFiles = []TextArtifact{{Path: "rules/demo.txt", Contents: []byte("(version 1)\n")}}

	_, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must use .kicad_dru extension") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryRefusesExistingJournal(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "led_indicator")
	journal := filepath.Join(dir, ".led_indicator.kicadai-journal")
	if err := os.WriteFile(journal, []byte("phase=move-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "recovery journal exists") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryFailsInvalidDesignBeforeFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bad")
	design := validLEDDesign(t)
	design.Project.DesignID = kicadfiles.UUID("")

	_, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("target should not exist after validation failure: %v", statErr)
	}
}

func TestWriteProjectDirectoryRejectsPathLikeDesignName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	design := validLEDDesign(t)
	design.Name = "../led_indicator"
	design.Project.Name = "../led_indicator"
	design.Schematic.Filename = "../led_indicator.kicad_sch"

	_, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "path separators") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryRejectsCurrentDirectoryTarget(t *testing.T) {
	design := validLEDDesign(t)

	_, err := WriteProjectDirectory(".", design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryAllowsTargetNameMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "other")

	result, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err != nil {
		t.Fatalf("WriteProjectDirectory returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pro")); err != nil {
		t.Fatalf("project file missing: %v", err)
	}
	if result.ProjectDir != root {
		t.Fatalf("ProjectDir = %q, want %q", result.ProjectDir, root)
	}
}

func TestWriteProjectDirectoryRejectsReservedWindowsName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CON")
	design := validLEDDesign(t)
	design.Name = "CON"
	design.Project.Name = "CON"
	design.Schematic.Filename = "CON.kicad_sch"

	_, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "reserved Windows") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryRejectsReservedTargetDirectoryName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CON")

	_, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "target directory name") || !strings.Contains(err.Error(), "reserved Windows") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteProjectDirectoryRejectsTrailingWindowsCharacters(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator.")
	design := validLEDDesign(t)
	design.Name = "led_indicator."
	design.Project.Name = "led_indicator."
	design.Schematic.Filename = "led_indicator..kicad_sch"

	_, err := WriteProjectDirectory(root, design, WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "space or period") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsTraversal(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{testGeneratedFile("../outside.kicad_sch")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsBackslashTraversal(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{testGeneratedFile("..\\outside.kicad_sch")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsDuplicatePath(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{
		testGeneratedFile("sch/child.kicad_sch"),
		testGeneratedFile("./sch/child.kicad_sch"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate generated path") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsCaseInsensitiveCollision(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{
		testGeneratedFile("Project.kicad_sch"),
		testGeneratedFile("project.kicad_sch"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "case-insensitive") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsFileDirectoryCollision(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{
		testGeneratedFile("lib"),
		testGeneratedFile("lib/symbol.kicad_sym"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "conflicts with directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGeneratedFilesRejectsReservedCharacters(t *testing.T) {
	_, err := validateGeneratedFiles([]generatedFile{testGeneratedFile("bad:name.kicad_sch")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "generated path component") {
		t.Fatalf("error = %v", err)
	}
}

func testGeneratedFile(path string) generatedFile {
	return generatedFile{
		Path: path,
		Write: func(w io.Writer) error {
			_, err := io.WriteString(w, "test\n")
			return err
		},
	}
}

func minimalChildSheet(filename string) schematic.SchematicFile {
	return schematic.SchematicFile{
		Filename:  filename,
		Version:   kicadfiles.KiCadFormatV20230121,
		Generator: "kicadai",
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789abe"),
		Paper:     kicadfiles.Paper{Name: "A4"},
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
