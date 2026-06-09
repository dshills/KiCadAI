package design

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
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

func TestWriteProjectDirectoryRequiresTargetNameMatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "other")

	_, err := WriteProjectDirectory(root, validLEDDesign(t), WriteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "target directory name") {
		t.Fatalf("error = %v", err)
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
