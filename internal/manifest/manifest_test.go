package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestWriteReadManifestAndDetectStale(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "demo.kicad_pro")
	if err := os.WriteFile(projectPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := Write(root, Manifest{
		ProjectName: "demo",
		Artifacts:   []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: projectPath}},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if artifact.Path == "" {
		t.Fatalf("manifest artifact missing path")
	}
	_, status, err := Read(root)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if !status.Present || status.Stale {
		t.Fatalf("unexpected status: %#v", status)
	}
	if err := os.WriteFile(projectPath, []byte(`{"changed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, status, err = Read(root)
	if err != nil {
		t.Fatalf("Read stale returned error: %v", err)
	}
	if !status.Stale {
		t.Fatalf("expected stale status")
	}
}

func TestWriteRejectsArtifactOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.kicad_pro")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, Manifest{ProjectName: "demo", Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: outside}}}); err == nil {
		t.Fatal("expected outside artifact error")
	}
}
