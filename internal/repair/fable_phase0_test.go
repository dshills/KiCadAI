package repair

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/atomicfile"
	"kicadai/internal/reports"
)

func TestFableH10InterruptedReplacementRecoversAllOldWithJournal(t *testing.T) {
	stage := t.TempDir()
	output := t.TempDir()
	stageFirst := filepath.Join(stage, "first.txt")
	stageSecond := filepath.Join(stage, "second.txt")
	outputFirst := filepath.Join(output, "first.txt")
	outputSecond := filepath.Join(output, "second.txt")
	if err := os.WriteFile(stageFirst, []byte("new-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stageSecond, []byte("new-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputFirst, []byte("old-first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputSecond, []byte("old-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	replacements := 0
	_, _, err := replaceGeneratedOutputWithOptions(stage, output, []reports.Artifact{
		{Kind: reports.ArtifactDiagnosticsReport, Path: "first.txt"},
		{Kind: reports.ArtifactDiagnosticsReport, Path: "second.txt"},
	}, atomicfile.GroupOptions{
		PreserveOnError: true,
		Fault: func(transition atomicfile.Transition) error {
			if transition != atomicfile.TransitionReplacement {
				return nil
			}
			replacements++
			if replacements == 1 {
				return errors.New("simulated interruption")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected replacement interruption")
	}
	journal := filepath.Join(output, filepath.FromSlash(atomicfile.JournalRelativePath))
	if _, statErr := os.Stat(journal); statErr != nil {
		t.Fatalf("interrupted commit did not retain its journal: %v", statErr)
	}
	if err := atomicfile.RecoverGroup(output); err != nil {
		t.Fatal(err)
	}
	first, readErr := os.ReadFile(outputFirst)
	if readErr != nil {
		t.Fatal(readErr)
	}
	second, readErr := os.ReadFile(outputSecond)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(first) != "old-first" || string(second) != "old-second" {
		t.Fatalf("recovery did not restore all-old output: first=%q second=%q", first, second)
	}
	if _, statErr := os.Stat(journal); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("resolved recovery retained journal: %v", statErr)
	}
}
