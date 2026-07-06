package reportartifacts

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestExistingReportArtifactSkipsMissingPath(t *testing.T) {
	artifacts := ExistingReportArtifact(reports.ArtifactDRCReport, filepath.Join(t.TempDir(), "missing.json"), "DRC JSON report")
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for missing report", artifacts)
	}
}

func TestExistingReportArtifactSkipsDirectory(t *testing.T) {
	artifacts := ExistingReportArtifact(reports.ArtifactDRCReport, t.TempDir(), "DRC JSON report")
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for directory report path", artifacts)
	}
}

func TestExistingReportArtifactIncludesExistingFile(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "drc.json")
	if err := os.WriteFile(reportPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	artifacts := ExistingReportArtifact(reports.ArtifactDRCReport, reportPath, "DRC JSON report")
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	if artifacts[0].Kind != reports.ArtifactDRCReport {
		t.Fatalf("kind = %q, want %q", artifacts[0].Kind, reports.ArtifactDRCReport)
	}
	if artifacts[0].Path != filepath.ToSlash(reportPath) {
		t.Fatalf("path = %q, want %q", artifacts[0].Path, filepath.ToSlash(reportPath))
	}
	if artifacts[0].Description != "DRC JSON report" {
		t.Fatalf("description = %q, want DRC JSON report", artifacts[0].Description)
	}
}
