package designworkflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestRunKiCadChecksSkipsWhenNotRequired(t *testing.T) {
	result := RunKiCadChecks(context.Background(), &Request{}, &ProjectWriteResult{}, KiCadCheckOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRunKiCadChecksBlocksWhenRequiredCLIMissing(t *testing.T) {
	request := Request{Validation: ValidationSpec{RequireDRC: true}}
	write := ProjectWriteResult{}

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: filepath.Join(t.TempDir(), "missing-kicad-cli"), EnforceRequirements: true})
	if result.Stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRunKiCadChecksLeavesRequestEvidencePendingWhenCLIUnavailable(t *testing.T) {
	request := Request{Validation: ValidationSpec{RequireDRC: true}}
	write := ProjectWriteResult{}

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: filepath.Join(t.TempDir(), "missing-kicad-cli")})
	if result.Stage.Status != StageStatusSkipped || len(result.Stage.Issues) != 1 || result.Stage.Issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRunKiCadChecksWithFakeCLIPasses(t *testing.T) {
	request, write := writeValidationFixture(t)
	request.Validation.RequireERC = true
	request.Validation.RequireDRC = true
	cli := fakeKiCadCheckCLI(t)

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("stage = %#v", result.Stage)
	}
	if result.ERC.Status == "" || result.DRC.Status == "" {
		t.Fatalf("checks did not run: %#v", result)
	}
	if _, ok := result.Stage.Summary[promotionKiCadERCSummaryKey]; !ok {
		t.Fatalf("ERC result missing from stage summary: %#v", result.Stage.Summary)
	}
	if _, ok := result.Stage.Summary[promotionKiCadDRCSummaryKey]; !ok {
		t.Fatalf("DRC result missing from stage summary: %#v", result.Stage.Summary)
	}
	if len(result.Stage.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v", result.Stage.Artifacts)
	}
	if result.ERC.ProjectContext != "full" || result.DRC.ProjectContext != "full" {
		t.Fatalf("checks did not use project context: ERC=%q DRC=%q", result.ERC.ProjectContext, result.DRC.ProjectContext)
	}
}

func TestRunKiCadChecksSerializesNonReentrantKiCadCLI(t *testing.T) {
	request, write := writeValidationFixture(t)
	request.Validation.RequireERC = true
	request.Validation.RequireDRC = true
	path := filepath.Join(t.TempDir(), "non-reentrant-kicad-cli")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "10.0.0"
  exit 0
fi
lock="$0.lock"
if ! mkdir "$lock" 2>/dev/null; then
  exit 9
fi
trap 'rmdir "$lock"' EXIT
sleep 1
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    shift
    out="$1"
  fi
  shift
done
printf '{"violations":[],"sheets":[],"coordinate_units":"mm"}' > "$out"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result := RunKiCadChecks(context.Background(), &request, &write, KiCadCheckOptions{KiCadCLI: path, KeepArtifacts: true, ArtifactDir: t.TempDir()})
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("serial non-reentrant checks did not pass: %#v", result.Stage)
	}
}

func TestKiCadCheckTargetPrefersProjectRoot(t *testing.T) {
	write := ProjectWriteResult{Inspection: inspect.ProjectSummary{
		Root:      "/tmp/demo",
		Schematic: &inspect.SchematicSummary{Path: "/tmp/demo/demo.kicad_sch"},
		PCB:       &inspect.PCBSummary{Path: "/tmp/demo/demo.kicad_pcb"},
	}}

	if got := kicadCheckTargetFromWrite(&write, checks.CheckKindERC); got != "/tmp/demo" {
		t.Fatalf("ERC target = %q, want project root", got)
	}
	if got := kicadCheckTargetFromWrite(&write, checks.CheckKindDRC); got != "/tmp/demo" {
		t.Fatalf("DRC target = %q, want project root", got)
	}
}

func TestWorkflowCheckResultWithIssuesDowngradesZeroFindingToolError(t *testing.T) {
	result := checks.CheckResult{
		Kind:          checks.CheckKindDRC,
		Status:        checks.CheckStatusError,
		TargetPath:    "demo.kicad_pcb",
		ExitCode:      -1,
		ToolErrorKind: checks.ToolErrorNoOutputCrash,
	}

	_, issues, _ := workflowCheckResultWithIssues(result, errors.New("drc check failed with exit code -1"))

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one tool issue", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed || issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("issue = %#v, want warning KiCad CLI failure", issues[0])
	}
	if !strings.Contains(issues[0].Suggestion, "exited before writing a report") {
		t.Fatalf("suggestion = %q, want no-output crash guidance", issues[0].Suggestion)
	}
}

func TestWorkflowCheckResultWithIssuesSkipsMissingReportArtifact(t *testing.T) {
	result := checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		Status:     checks.CheckStatusError,
		TargetPath: "demo.kicad_pcb",
		ExitCode:   -1,
		ReportPath: filepath.Join(t.TempDir(), "missing-drc.json"),
	}

	_, _, artifacts := workflowCheckResultWithIssues(result, errors.New("drc check failed with exit code -1"))

	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for missing DRC report", artifacts)
	}
}

func TestWorkflowCheckResultWithIssuesKeepsGenericToolErrorBlocking(t *testing.T) {
	result := checks.CheckResult{
		Kind:          checks.CheckKindDRC,
		Status:        checks.CheckStatusError,
		TargetPath:    "demo.kicad_pcb",
		ExitCode:      2,
		ToolErrorKind: checks.ToolErrorCommandFailed,
	}

	_, issues, _ := workflowCheckResultWithIssues(result, errors.New("drc check failed with exit code 2"))

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one tool issue", issues)
	}
	if issues[0].Code != reports.CodeKiCadCLIFailed || issues[0].Severity != reports.SeverityError {
		t.Fatalf("issue = %#v, want blocking KiCad CLI failure", issues[0])
	}
}

func TestWorkflowCheckResultWithIssuesDoesNotAddToolErrorForFindings(t *testing.T) {
	result := checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		Status:     checks.CheckStatusFail,
		TargetPath: "demo.kicad_pcb",
		Findings: []checks.CheckFinding{{
			Kind:     checks.CheckKindDRC,
			Severity: "error",
			Message:  "unconnected pad",
		}},
	}

	_, issues, _ := workflowCheckResultWithIssues(result, errors.New("drc check failed with exit code 5"))

	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one validation issue", issues)
	}
	if issues[0].Code != reports.CodeValidationFailed {
		t.Fatalf("issue = %#v, want validation failure only", issues[0])
	}
}

func TestWorkflowCheckResultWithIssuesKeepsParserToolErrorBlocking(t *testing.T) {
	result := checks.CheckResult{
		Kind:          checks.CheckKindDRC,
		Status:        checks.CheckStatusError,
		TargetPath:    "demo.kicad_pcb",
		ToolErrorKind: checks.ToolErrorReportParse,
		ParserIssues:  []checks.ParserIssue{{Message: "malformed DRC report"}},
	}

	_, issues, _ := workflowCheckResultWithIssues(result, errors.New("drc check failed with exit code 2"))

	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want parser issue and tool issue", issues)
	}
	if issues[1].Code != reports.CodeKiCadCLIFailed || issues[1].Severity != reports.SeverityError {
		t.Fatalf("tool issue = %#v, want blocking KiCad CLI failure", issues[1])
	}
}

func TestWorkflowCheckResultWithIssuesReportsParserToolErrorWithoutCommandError(t *testing.T) {
	result := checks.CheckResult{
		Kind:          checks.CheckKindDRC,
		Status:        checks.CheckStatusError,
		TargetPath:    "demo.kicad_pcb",
		ToolErrorKind: checks.ToolErrorReportParse,
		ParserIssues:  []checks.ParserIssue{{Message: "malformed DRC report"}},
	}

	_, issues, _ := workflowCheckResultWithIssues(result, nil)

	if len(issues) != 2 {
		t.Fatalf("issues = %#v, want parser issue and tool issue", issues)
	}
	if issues[1].Code != reports.CodeKiCadCLIFailed || issues[1].Message == "" {
		t.Fatalf("tool issue = %#v, want parser tool failure message", issues[1])
	}
	if !strings.Contains(issues[1].Suggestion, "could not parse") {
		t.Fatalf("suggestion = %q, want parser guidance", issues[1].Suggestion)
	}
}

func fakeKiCadCheckCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fake kicad-cli is not portable to Windows")
	}
	path := filepath.Join(t.TempDir(), "kicad-cli")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "10.0.0"
  exit 0
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    shift
    out="$1"
  fi
  shift
done
if [ -n "$out" ]; then
  printf '{"violations":[],"sheets":[],"coordinate_units":"mm"}' > "$out"
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
