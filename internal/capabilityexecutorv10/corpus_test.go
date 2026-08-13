package capabilityexecutorv10

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/corpuspublication"
)

func TestLoadPublicDiscoveryBindsCanonicalCohort(t *testing.T) {
	root := t.TempDir()
	manifest := corpuspublication.ManifestV10{
		Schema: corpuspublication.ManifestSchemaV10, Version: corpuspublication.ManifestVersionV10,
		DiscoveryCaseCount: 24, HeldOutCaseCount: 24,
	}
	source := []byte("{}")
	for index := 1; index <= 24; index++ {
		id := fmt.Sprintf("v10_case_%03d", index)
		stablePath := filepath.ToSlash(filepath.Join("discovery", id+".json"))
		manifest.Entries = append(manifest.Entries, corpuspublication.EntryV10{
			ID: id, Role: "discovery", Domain: testDomains[(index-1)%len(testDomains)], CircuitRole: testRoles[(index-1)%len(testRoles)],
			SafetyImpact: "review_required", StablePath: stablePath, RequirementSHA256: testRawDigest(source),
		})
		path := filepath.Join(root, filepath.FromSlash(stablePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestSource, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := testRawDigest(manifestSource)
	obligations := corpuspublication.DiscoveryObligationsV10{
		Schema: "kicadai.closed-loop-open-set-discovery-obligations.v10", Version: 10, CorpusManifestSHA256: manifestHash,
	}
	for index := 1; index <= 24; index++ {
		id := fmt.Sprintf("v10_case_%03d", index)
		obligations.Obligations = append(obligations.Obligations, corpuspublication.ObligationV10{
			Anchor: testDigest("anchor-" + id), Role: "discovery", CaseID: id,
			OperatingCaseID: "nominal", AssertionID: "assertion", ObservationKind: "port", ObservationID: "output", OutputID: "output",
		})
	}
	writeTestJSON(t, filepath.Join(root, corpuspublication.ManifestFileV10), manifest)
	writeTestJSON(t, filepath.Join(root, corpuspublication.DiscoveryObligationsFileV10), obligations)
	loaded, err := loadPublicDiscovery(root, manifestHash)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ManifestSHA256 != manifestHash || len(loaded.Cases) != 24 || loaded.Cases[23].Entry.ID != "v10_case_024" {
		t.Fatalf("loaded corpus = %#v", loaded)
	}
}

func TestReadRegularFileRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularFile(link); err == nil {
		t.Fatal("symlinked corpus file was accepted")
	}
}

func TestReadContainedRegularFileRejectsParentTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.json")
	if err := os.WriteFile(outside, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	if _, err := readContainedRegularFile(root, "../outside.json"); err == nil {
		t.Fatal("parent traversal was accepted")
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
