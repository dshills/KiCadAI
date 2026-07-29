package repair

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestFableH10ReproductionFailedReplacementLeavesMixedOutputWithoutMarker(t *testing.T) {
	stage := t.TempDir()
	output := t.TempDir()
	stageFirst := filepath.Join(stage, "first.txt")
	outputFirst := filepath.Join(output, "first.txt")
	outputSecond := filepath.Join(output, "second.txt")
	if err := os.WriteFile(stageFirst, []byte("new-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputFirst, []byte("old-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputSecond, []byte("old-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := replaceGeneratedOutput(stage, output, []reports.Artifact{
		{Kind: reports.ArtifactDiagnosticsReport, Path: "first.txt"},
		{Kind: reports.ArtifactDiagnosticsReport, Path: "missing.txt"},
	})
	if err == nil {
		t.Fatal("expected the second replacement to fail")
	}
	first, readErr := os.ReadFile(outputFirst)
	if readErr != nil {
		t.Fatal(readErr)
	}
	second, readErr := os.ReadFile(outputSecond)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(first) != "new-first" || string(second) != "old-second" {
		t.Fatalf("mixed output reproduction changed: first=%q second=%q", first, second)
	}
	markerDir := filepath.Join(output, ".kicadai")
	if info, statErr := os.Stat(markerDir); statErr != nil || !info.IsDir() {
		t.Fatalf("repair marker directory was not initialized: info=%v err=%v", info, statErr)
	}
	marker := filepath.Join(output, ".kicadai", "repair-in-progress")
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failure marker unexpectedly remains: %v", statErr)
	}
}
