package evaluate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestProjectStructureReportsMissingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "demo.kicad_pro"), "{}")

	report, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	if report.FabricationReady {
		t.Fatalf("missing files should not be fabrication ready: %#v", report)
	}
	if len(report.Issues) != 2 {
		t.Fatalf("issues = %#v, want missing schematic and PCB", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Code != reports.CodeMissingFile || issue.Severity != reports.SeverityError {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
}

func TestProjectPreservesNonMissingInspectionIssues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "demo.kicad_pro"), "{}")
	writeFile(t, filepath.Join(root, "demo.kicad_sch"), `(kicad_sch (version 20260306) (generator "kicadai"))`)
	writeFile(t, filepath.Join(root, "demo.kicad_pcb"), `(kicad_pcb (unsupported_widget))`)

	report, err := Project(root)
	if err != nil {
		t.Fatalf("Project returned error: %v", err)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == reports.CodeMissingBoardOutline {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing board outline issue to be preserved, got %#v", report.Issues)
	}
}

func TestPCBEvaluationReportsCorpusHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeFile(t, path, `(kicad_pcb
  (gr_rect (layer "Edge.Cuts"))
  (footprint "Test:One" (pad "1" smd rect (layers "F.Cu")))
)`)

	report, err := PCB(path)
	if err != nil {
		t.Fatalf("PCB returned error: %v", err)
	}
	if len(report.Checks) != 2 {
		t.Fatalf("checks = %#v", report.Checks)
	}
	if report.Checks[0].Name != "pcb_corpus_scan" || report.Checks[0].Status != CheckPassed {
		t.Fatalf("unexpected corpus check: %#v", report.Checks[0])
	}
	if !report.FabricationReady {
		t.Fatalf("PCB should be fabrication ready when required checks pass: %#v", report)
	}
}

func TestPCBEvaluationReportsMissingOutline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	writeFile(t, path, `(kicad_pcb (footprint "Test:One" (pad "1" smd rect (layers "F.Cu"))))`)

	report, err := PCB(path)
	if err != nil {
		t.Fatalf("PCB returned error: %v", err)
	}
	if len(report.Issues) == 0 || report.Issues[0].Code != reports.CodeMissingBoardOutline || report.Issues[0].Severity != reports.SeverityError {
		t.Fatalf("expected missing outline issue, got %#v", report.Issues)
	}
	if report.FabricationReady {
		t.Fatalf("missing outline should not be fabrication ready: %#v", report)
	}
}

func TestSchematicEvaluationSkipsReaderGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	writeFile(t, path, `(kicad_sch (version 20260306) (generator "kicadai"))`)

	report, err := Schematic(path)
	if err != nil {
		t.Fatalf("Schematic returned error: %v", err)
	}
	if len(report.Checks) != 2 {
		t.Fatalf("checks = %#v", report.Checks)
	}
	if report.Checks[1].Status != CheckSkipped || report.Checks[1].Issues[0].Code != reports.CodeUnsupportedOperation {
		t.Fatalf("unexpected reader gap check: %#v", report.Checks[1])
	}
}

func TestFinishTreatsFailedChecksAsNotReady(t *testing.T) {
	report := newReport("board")
	report.addCheck(CheckResult{Name: "failed_check", Status: CheckFailed})
	report.finish()

	if report.FabricationReady {
		t.Fatalf("failed check should not be fabrication ready: %#v", report)
	}
	if report.FabricationReadyReason == "" {
		t.Fatalf("expected fabrication ready reason")
	}
}

func TestIssueFromErrorMapsKnownValidationMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code reports.Code
	}{
		{name: "duplicate uuid", err: WithCode(errors.New("duplicate UUID abc"), reports.CodeDuplicateUUID), code: reports.CodeDuplicateUUID},
		{name: "duplicate reference", err: WithCode(errors.New("duplicate schematic reference R1"), reports.CodeDuplicateReference), code: reports.CodeDuplicateReference},
		{name: "missing footprint", err: WithCode(errors.New("missing PCB footprint for schematic reference R1"), reports.CodeMissingFootprint), code: reports.CodeMissingFootprint},
		{name: "outline", err: WithCode(errors.New("no Edge.Cuts board outline detected"), reports.CodeMissingBoardOutline), code: reports.CodeMissingBoardOutline},
		{name: "disconnected", err: WithCode(errors.New("pad is disconnected from net GND"), reports.CodeDisconnectedPad), code: reports.CodeDisconnectedPad},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue := IssueFromError(test.err, "design")
			if issue.Code != test.code {
				t.Fatalf("code = %s, want %s", issue.Code, test.code)
			}
		})
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
