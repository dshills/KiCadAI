package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func TestCapturedWorkflowRequest(t *testing.T) {
	requestPath := strings.TrimSpace(os.Getenv("KICADAI_CAPTURED_WORKFLOW_REQUEST"))
	if requestPath == "" {
		t.Skip("set KICADAI_CAPTURED_WORKFLOW_REQUEST to replay a captured workflow request")
	}
	requestData, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := json.Unmarshal(requestData, &request); err != nil {
		t.Fatalf("decode captured workflow request: %v", err)
	}

	var index libraryresolver.LibraryIndex
	if indexPath := strings.TrimSpace(os.Getenv("KICADAI_CAPTURED_LIBRARY_INDEX")); indexPath != "" {
		indexData, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(indexData, &index); err != nil {
			t.Fatalf("decode captured library index: %v", err)
		}
	}
	output := t.TempDir()
	opts := CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
		Writer: writercorrectness.Options{
			LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true,
		},
	}
	if cli := strings.TrimSpace(os.Getenv("KICADAI_KICAD_CLI")); cli != "" {
		opts.Validation = ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "validation")}
		opts.KiCadChecks = KiCadCheckOptions{KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "checks")}
		opts.Writer = writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true}
	}
	result := Create(context.Background(), request, opts)
	for _, stage := range result.Stages {
		t.Logf("stage=%s status=%s summary=%#v issues=%#v", stage.Name, stage.Status, stage.Summary, stage.Issues)
	}
	if report, ok := AutonomousCorrectionReportFromWorkflow(result); ok {
		data, err := json.Marshal(report)
		if err != nil {
			t.Fatalf("encode autonomous correction report: %v", err)
		}
		t.Logf("autonomous_correction=%s", data)
	}
	if issues := WorkflowIssues(result); reports.HasBlockingIssue(issues) {
		t.Fatalf("captured workflow has blocking issues: %#v", issues)
	}
}
