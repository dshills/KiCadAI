package manifest

import (
	"encoding/json"
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

func TestReadOldManifestWithoutProvenance(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".kicadai"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"project_name":"demo","generator_version":"test","operations":[],"artifacts":[],"file_hashes":{}}`)
	if err := os.WriteFile(filepath.Join(root, RelativePath), data, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, status, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Provenance != nil {
		t.Fatalf("old manifest provenance = %#v, want nil", manifest.Provenance)
	}
	if !status.Present || status.Stale {
		t.Fatalf("status = %#v", status)
	}
}

func TestWriteHashesTransactionProvenance(t *testing.T) {
	root := t.TempDir()
	txPath := filepath.Join(root, ".kicadai", "transaction.json")
	if err := os.MkdirAll(filepath.Dir(txPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txPath, []byte(`{"schema":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, Manifest{
		ProjectName: "demo",
		Provenance:  &ProvenanceRef{TransactionPath: ".kicadai/transaction.json", Schema: "test", OperationCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	manifest, status, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Stale {
		t.Fatalf("status = %#v, want fresh", status)
	}
	if manifest.Provenance == nil || manifest.Provenance.Hash == "" {
		t.Fatalf("provenance ref = %#v", manifest.Provenance)
	}
	if got := manifest.FileHashes[".kicadai/transaction.json"]; got == "" || got != manifest.Provenance.Hash {
		t.Fatalf("file hash = %q provenance = %#v", got, manifest.Provenance)
	}
}

func TestManifestProvenanceMissingOrChangedIsStale(t *testing.T) {
	root := t.TempDir()
	txPath := filepath.Join(root, ".kicadai", "transaction.json")
	if err := os.MkdirAll(filepath.Dir(txPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txPath, []byte(`{"schema":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, Manifest{
		ProjectName: "demo",
		Provenance:  &ProvenanceRef{TransactionPath: ".kicadai/transaction.json"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txPath, []byte(`{"schema":"changed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, status, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stale {
		t.Fatalf("expected stale after provenance change")
	}
	if err := os.Remove(txPath); err != nil {
		t.Fatal(err)
	}
	_, status, err = Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stale {
		t.Fatalf("expected stale after provenance removal")
	}
}

func TestWriteRejectsUnsafeProvenancePath(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../transaction.json", filepath.Join(root, ".kicadai", "transaction.json")} {
		_, err := Write(root, Manifest{ProjectName: "demo", Provenance: &ProvenanceRef{TransactionPath: rel}})
		if err == nil {
			t.Fatalf("expected unsafe provenance path error for %q", rel)
		}
	}
}

func TestManifestProvenanceJSONShape(t *testing.T) {
	root := t.TempDir()
	txPath := filepath.Join(root, ".kicadai", "transaction.json")
	if err := os.MkdirAll(filepath.Dir(txPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txPath, []byte(`{"schema":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, Manifest{ProjectName: "demo", Provenance: &ProvenanceRef{TransactionPath: ".kicadai/transaction.json"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["provenance"].(map[string]any); !ok {
		t.Fatalf("manifest JSON missing provenance object: %s", data)
	}
}
