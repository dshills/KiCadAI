package main

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func TestNativeGatesFailClosed(t *testing.T) {
	good := func() designworkflow.WorkflowResult {
		var r designworkflow.WorkflowResult
		for _, name := range requiredStages {
			r.Stages = append(r.Stages, designworkflow.StageResult{Name: name, Status: designworkflow.StageStatusOK})
		}
		return r
	}
	if gate := firstFailedStage(good()); gate != "" {
		t.Fatal(gate)
	}
	for i, name := range requiredStages {
		for _, state := range []string{"absent", "duplicate", "warning", "blocked", "skipped", "error"} {
			r := good()
			switch state {
			case "absent":
				r.Stages = append(r.Stages[:i], r.Stages[i+1:]...)
			case "duplicate":
				r.Stages = append(r.Stages, r.Stages[i])
			case "error":
				r.Stages[i].Issues = []reports.Issue{{Severity: reports.SeverityError}}
			default:
				r.Stages[i].Status = designworkflow.StageStatus(state)
			}
			if gate := firstFailedStage(r); gate != string(name) {
				t.Fatalf("%s %s: %q", name, state, gate)
			}
		}
	}
	r := good()
	r.Feedback.Summary.BlockingCount = 1
	if firstFailedStage(r) != "workflow_feedback" {
		t.Fatal("blocking feedback admitted")
	}
}

func TestProjectIdentityIncludesChildSheetsAndRejectsLinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.kicad_sch", "child/second.kicad_sch", "root.kicad_pro", "root.kicad_pcb", "rules.kicad_dru"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := projectIdentity(dir)
	if err != nil || len(before) != 5 {
		t.Fatal(before, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child/second.kicad_sch"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := projectIdentity(dir)
	if err != nil || before["child/second.kicad_sch"] == after["child/second.kicad_sch"] {
		t.Fatal("child sheet change lost", err)
	}
	if err := os.Symlink(filepath.Join(dir, "root.kicad_sch"), filepath.Join(dir, "alias.kicad_sch")); err != nil {
		t.Fatal(err)
	}
	if _, err := projectIdentity(dir); err == nil {
		t.Fatal("symbolic link accepted")
	}
}
