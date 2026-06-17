package writercorrectness

import (
	"testing"

	"kicadai/internal/reports"
)

func TestResultFinishEmptyPasses(t *testing.T) {
	result := NewResult("examples/demo")
	result.Finish()

	if !result.OK {
		t.Fatalf("OK = false, want true")
	}
	if result.Target.Input != "examples/demo" {
		t.Fatalf("Input = %q", result.Target.Input)
	}
	if result.OverallSummary.CheckCount != 0 || result.OverallSummary.IssueCount != 0 {
		t.Fatalf("unexpected summary: %#v", result.OverallSummary)
	}
}

func TestBlockingIssueFailsResult(t *testing.T) {
	result := NewResult("demo")
	result.AddCheck(CheckResult{
		Name:     CheckPCBNetTable,
		Required: true,
		Issues: []reports.Issue{{
			Code:     reports.CodeInvalidNetAssignment,
			Severity: reports.SeverityBlocked,
			Path:     "demo.kicad_pcb",
			Message:  "pad references missing net",
			Nets:     []string{"SDA"},
		}},
	})
	result.Finish()

	if result.OK {
		t.Fatalf("OK = true, want false")
	}
	if result.Checks[0].Status != CheckFail {
		t.Fatalf("check status = %q, want %q", result.Checks[0].Status, CheckFail)
	}
	if result.OverallSummary.FailCount != 1 || result.OverallSummary.BlockingCount != 1 {
		t.Fatalf("unexpected summary: %#v", result.OverallSummary)
	}
	if got := result.OverallSummary.ByNet["SDA"]; got != 1 {
		t.Fatalf("ByNet[SDA] = %d, want 1", got)
	}
}

func TestWarningOnlyResultPasses(t *testing.T) {
	result := NewResult("demo")
	result.AddCheck(CheckResult{
		Name: CheckSchematicPCBTransfer,
		Issues: []reports.Issue{{
			Code:     reports.CodePinmapUnverified,
			Severity: reports.SeverityWarning,
			Path:     "symbols.U1",
			Message:  "pinmap inferred",
		}},
	})
	result.Finish()

	if !result.OK {
		t.Fatalf("OK = false, want true")
	}
	if result.Checks[0].Status != CheckWarning {
		t.Fatalf("check status = %q, want %q", result.Checks[0].Status, CheckWarning)
	}
	if result.OverallSummary.WarningCount != 1 || result.OverallSummary.BlockingCount != 0 {
		t.Fatalf("unexpected summary: %#v", result.OverallSummary)
	}
}

func TestSortsChecksIssuesAndArtifactsDeterministically(t *testing.T) {
	result := NewResult("demo")
	result.AddCheck(CheckResult{
		Name: CheckPCBParse,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "z",
			Message:  "z",
		}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactPCB, Path: "z.kicad_pcb"}},
	})
	result.AddCheck(CheckResult{
		Name: CheckProjectStructure,
		Issues: []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "a",
			Message:  "a",
		}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: "a.kicad_pro"}},
	})
	result.Finish()

	if result.Checks[0].Name != CheckProjectStructure || result.Checks[1].Name != CheckPCBParse {
		t.Fatalf("checks not sorted by priority: %#v", result.Checks)
	}
	if result.Issues[0].Path != "a" || result.Issues[1].Path != "z" {
		t.Fatalf("issues not sorted by path: %#v", result.Issues)
	}
	if result.Artifacts[0].Kind != reports.ArtifactKiCadProject || result.Artifacts[1].Kind != reports.ArtifactPCB {
		t.Fatalf("artifacts not sorted: %#v", result.Artifacts)
	}
}

func TestReportResultWrapsIssuesAndArtifacts(t *testing.T) {
	result := NewResult("demo")
	result.AddCheck(CheckResult{
		Name: CheckProjectStructure,
		Issues: []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "demo.kicad_pro",
			Message:  "missing project",
		}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactValidationReport, Path: "writer.json"}},
	})
	result.Finish()

	report := result.ReportResult("writer check")
	if report.OK {
		t.Fatalf("report OK = true, want false")
	}
	if report.Command != "writer check" {
		t.Fatalf("command = %q", report.Command)
	}
	if len(report.Issues) != 1 || len(report.Artifacts) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if _, ok := report.Data.(Result); !ok {
		t.Fatalf("report data type = %T, want Result", report.Data)
	}
}

func TestHelpersNormalizePaths(t *testing.T) {
	issue := BlockingIssue(reports.CodeMissingFile, `foo\bar.kicad_pcb`, "missing")
	result := NewResult(`foo\bar`)
	result.AddIssue(CheckProjectStructure, issue)
	result.Finish()

	if result.Target.Input != "foo/bar" {
		t.Fatalf("input path = %q", result.Target.Input)
	}
	if result.Issues[0].Path != "foo/bar.kicad_pcb" {
		t.Fatalf("issue path = %q", result.Issues[0].Path)
	}
}
