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
