package design

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestReadProjectDirectoryReadsGeneratedFiles(t *testing.T) {
	generated, err := LEDIndicatorDesign(LEDIndicatorInput{
		Name:       "reader_demo",
		DesignID:   kicadfiles.UUID("11111111-1111-5111-8111-111111111111"),
		Seed:       "reader_demo",
		IncludePCB: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "reader_demo")
	if _, err := WriteProjectDirectory(root, generated, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	read, err := ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if read.Project.Name != "reader_demo" || read.Schematic == nil || read.PCB == nil {
		t.Fatalf("unexpected read design: %#v", read)
	}
	if len(read.Schematic.Symbols) == 0 || len(read.PCB.Footprints) == 0 {
		t.Fatalf("missing read child content: %#v", read)
	}
}

func TestReadProjectDirectoryRejectsMultipleProjects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProjectDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected multiple project error, got %v", err)
	}
}

func TestReadProjectDirectoryReadsChildSheetFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Child")
    (property "Sheetfile" "child.kicad_sch")
    (uuid "22222222-2222-5222-8222-222222222222"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "33333333-3333-5333-8333-333333333333")
  (paper A4)
  (symbol
    (lib_id "Device:R")
    (at 10 10 0)
    (property "Reference" "R1")
    (property "Value" "10k")
    (uuid "44444444-4444-5444-8444-444444444444"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.SheetFiles) != 1 || read.SheetFiles[0].Filename != "child.kicad_sch" {
		t.Fatalf("sheet files = %#v", read.SheetFiles)
	}
	if got := read.SheetFiles[0].Symbols[0].Reference; got != "R1" {
		t.Fatalf("child symbol = %q", got)
	}
	if len(read.Schematic.Sheets) != 1 || read.Schematic.Sheets[0].Filename != "child.kicad_sch" {
		t.Fatalf("root sheets = %#v", read.Schematic.Sheets)
	}
}

func TestReadProjectDirectorySkipsSheetCycles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Child")
    (property "Sheetfile" "child.kicad_sch")
    (uuid "22222222-2222-5222-8222-222222222222"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "33333333-3333-5333-8333-333333333333")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Root")
    (property "Sheetfile" "demo.kicad_sch")
    (uuid "44444444-4444-5444-8444-444444444444"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.SheetFiles) != 1 || read.SheetFiles[0].Filename != "child.kicad_sch" {
		t.Fatalf("sheet files = %#v", read.SheetFiles)
	}
}

func TestReadProjectDirectoryResolvesNestedSheetPathsFromParent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(filepath.Join(root, "sheets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Nested")
    (property "Sheetfile" "sheets/parent.kicad_sch")
    (uuid "22222222-2222-5222-8222-222222222222"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sheets", "parent.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "33333333-3333-5333-8333-333333333333")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Child")
    (property "Sheetfile" "child.kicad_sch")
    (uuid "44444444-4444-5444-8444-444444444444"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sheets", "child.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "55555555-5555-5555-8555-555555555555")
  (paper A4)
)`), 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := ReadProjectDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.SheetFiles) != 2 {
		t.Fatalf("sheet files = %#v", read.SheetFiles)
	}
	if read.SheetFiles[0].Filename != "sheets/parent.kicad_sch" || read.SheetFiles[1].Filename != "sheets/child.kicad_sch" {
		t.Fatalf("sheet filenames = %q, %q", read.SheetFiles[0].Filename, read.SheetFiles[1].Filename)
	}
}

func TestReadProjectDirectoryReportsMissingSheetFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (sheet
    (at 10 10)
    (size 20 20)
    (property "Sheetname" "Missing")
    (property "Sheetfile" "missing.kicad_sch")
    (uuid "22222222-2222-5222-8222-222222222222"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadProjectDirectory(root)
	if err == nil || !strings.Contains(err.Error(), "sheet file not found: missing.kicad_sch") {
		t.Fatalf("expected missing sheet error, got %v", err)
	}
}
