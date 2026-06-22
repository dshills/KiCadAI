package repair

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/inspect"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

func TestExportBundleDryRunReturnsDefaultPath(t *testing.T) {
	root := t.TempDir()
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: exportStageIssues(),
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if !result.DryRun || result.BundlePath != filepath.ToSlash(filepath.Join(root, ".kicadai", "repair-bundle.json")) {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.FromSlash(result.BundlePath)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote bundle or unexpected stat error: %v", err)
	}
	if result.Summary.StageCount != 1 || result.Summary.IssueCount != 2 || result.Summary.BlockingCount != 1 || !result.Summary.Generated {
		t.Fatalf("summary = %#v", result.Summary)
	}
}

func TestExportBundleExecuteWritesParseableBundle(t *testing.T) {
	root := t.TempDir()
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: exportStageIssues(),
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	loaded, err := LoadBundle(filepath.FromSlash(result.BundlePath))
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
	if loaded.ProjectRoot != filepath.ToSlash(root) || loaded.ProjectName != "demo" || !loaded.Generated || len(loaded.StageIssues) != 1 {
		t.Fatalf("bundle = %#v", loaded)
	}
	if !loaded.RepairOptions.Enabled || !loaded.RepairOptions.Apply {
		t.Fatalf("repair options = %#v", loaded.RepairOptions)
	}
}

func TestExportBundleBlocksExistingBundleWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".kicadai", "repair-bundle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: exportStageIssues(),
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	assertIssueCode(t, result.Issues, reports.CodeInvalidArgument)
}

func TestExportBundleOverwriteReplacesExistingBundle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".kicadai", "repair-bundle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: exportStageIssues(),
		Execute:     true,
		Overwrite:   true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if _, err := LoadBundle(path); err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
}

func TestExportBundleBlocksMissingTarget(t *testing.T) {
	result := ExportBundle(ExportOptions{
		TargetPath:  filepath.Join(t.TempDir(), "missing"),
		StageIssues: exportStageIssues(),
		Execute:     true,
	})
	assertIssueCode(t, result.Issues, reports.CodeMissingFile)
}

func TestExportBundleBlocksTargetWithoutGeneratedManifest(t *testing.T) {
	root := t.TempDir()
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: exportStageIssues(),
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root: root,
			Name: "demo",
		}),
	})
	assertIssueCode(t, result.Issues, reports.CodePreservationConflict)
}

func TestExportBundleBlocksEmptyStageIssues(t *testing.T) {
	root := t.TempDir()
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		StageIssues: []StageIssues{{Stage: "writer_correctness"}},
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	assertIssueCode(t, result.Issues, reports.CodeInvalidArgument)
}

func TestExportBundleBlocksOutputOutsideTargetRoot(t *testing.T) {
	root := t.TempDir()
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		OutputPath:  filepath.Join(t.TempDir(), "repair-bundle.json"),
		StageIssues: exportStageIssues(),
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	assertIssueCode(t, result.Issues, reports.CodeInvalidArgument)
}

func TestExportBundleCreatesSafeNestedParentDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".kicadai", "exports", "repair-bundle.json")
	result := ExportBundle(ExportOptions{
		TargetPath:  root,
		OutputPath:  path,
		StageIssues: exportStageIssues(),
		Execute:     true,
		InspectProject: stubInspect(root, inspect.ProjectSummary{
			Root:     root,
			Name:     "demo",
			Manifest: manifest.Status{Present: true},
		}),
	})
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected bundle at %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("expected regular bundle file at %s, got mode %s", path, info.Mode())
	}
}

func exportStageIssues() []StageIssues {
	return []StageIssues{{Stage: "writer_correctness", Issues: []reports.Issue{
		{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: "pcb.pad", Message: "missing net"},
		{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityWarning, Path: "kicad_drc", Message: "missing KiCad CLI"},
	}}}
}
