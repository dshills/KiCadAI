package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestExportPreviewDryRunWritesNoFiles(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPreview(context.Background(), root, Options{})
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if _, err := os.Stat(filepath.Join(root, "fabrication", "readiness.json")); !os.IsNotExist(err) {
		t.Fatalf("readiness file exists after dry-run or stat failed: %v", err)
	}
}

func TestExportPackageExecuteWritesArtifacts(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPackage(context.Background(), root, Options{Execute: true})
	if result.DryRun {
		t.Fatalf("DryRun = true, want false")
	}
	for _, rel := range []string{"readiness.json", "package-manifest.json", "bom.csv", "cpl.csv"} {
		if _, err := os.Stat(filepath.Join(root, "fabrication", rel)); err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
	}
	if result.ManifestPath == "" {
		t.Fatalf("ManifestPath is empty")
	}
}

func TestExportBlocksExistingFileWithoutOverwrite(t *testing.T) {
	root := testFabricationProject(t)
	fabricationDir := filepath.Join(root, "fabrication")
	if err := os.MkdirAll(fabricationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fabricationDir, "readiness.json"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ExportPreview(context.Background(), root, Options{Execute: true})
	if !hasIssuePath(result.Issues, "fabrication/readiness.json") {
		t.Fatalf("issues = %#v, want existing file issue", result.Issues)
	}
	if status := artifactStatus(result.Artifacts, ArtifactReadinessReport); status != ArtifactBlocked {
		t.Fatalf("readiness artifact status = %s, want blocked", status)
	}
}

func TestExportRejectsOutputOutsideProject(t *testing.T) {
	root := testFabricationProject(t)
	result := ExportPreview(context.Background(), root, Options{Output: filepath.Join(filepath.Dir(root), "outside")})
	if !hasIssueCode(result.Issues, reports.CodeInvalidArgument) {
		t.Fatalf("issues = %#v, want invalid output issue", result.Issues)
	}
}

func testFabricationProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func artifactStatus(artifacts []Artifact, kind ArtifactKind) ArtifactStatus {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact.Status
		}
	}
	return ""
}
