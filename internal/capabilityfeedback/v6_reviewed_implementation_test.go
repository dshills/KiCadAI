package capabilityfeedback

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	closedLoopV6ImplementationSealSchema    = "kicadai.closed-loop-open-set-reviewed-implementation.v6"
	closedLoopV6ImplementationSealUpdateEnv = "UPDATE_CLOSED_LOOP_V6_IMPLEMENTATION_SEAL"
	closedLoopV6ImplementationCommit        = "0a2449f9d2249e98c9ea5e8778862280223b02df"
	closedLoopV6ImplementationReview        = "prism_reviewed_no_high_or_medium_findings"
)

type closedLoopV6ImplementationSeal struct {
	Schema               string                       `json:"schema"`
	Version              int                          `json:"version"`
	SelectionSHA256      string                       `json:"selection_sha256"`
	SelectedBundleKey    string                       `json:"selected_bundle_key"`
	ImplementationCommit string                       `json:"implementation_commit"`
	Review               string                       `json:"review"`
	Artifacts            []closedLoopArtifactEvidence `json:"artifacts"`
	Hash                 string                       `json:"hash"`
}

func TestClosedLoopV6ReviewedImplementationSealIsFrozen(t *testing.T) {
	path := filepath.Join(closedLoopSpecDirectory(t), "V6_REVIEWED_IMPLEMENTATION.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("V6 reviewed implementation has not been sealed")
	} else if err != nil {
		t.Fatal(err)
	}
	loadClosedLoopV6CurrentImplementationSeal(t)
}

func TestUpdateClosedLoopV6ReviewedImplementationSeal(t *testing.T) {
	if os.Getenv(closedLoopV6ImplementationSealUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V6_IMPLEMENTATION_SEAL=1 to seal the reviewed V6 implementation")
	}
	specRoot := closedLoopSpecDirectory(t)
	jsonPath := filepath.Join(specRoot, "V6_REVIEWED_IMPLEMENTATION.json")
	checksumPath := filepath.Join(specRoot, "V6_REVIEWED_IMPLEMENTATION.sha256")
	for _, path := range []string{jsonPath, checksumPath} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("V6 reviewed implementation seal already exists at %s; refusing overwrite", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat V6 reviewed implementation seal %s: %v", path, err)
		}
	}
	selection := loadClosedLoopV6FrozenSelection(t)
	paths := closedLoopV6ImplementationArtifactPaths()
	moduleRoot := closedLoopModuleRoot(t)
	artifacts := make([]closedLoopArtifactEvidence, 0, len(paths))
	for _, path := range paths {
		artifacts = append(artifacts, closedLoopArtifactEvidence{
			Path:   path,
			SHA256: corpusHash(mustCorpusRead(t, filepath.Join(moduleRoot, filepath.FromSlash(path)))),
		})
	}
	seal := closedLoopV6ImplementationSeal{
		Schema:               closedLoopV6ImplementationSealSchema,
		Version:              closedLoopV6BaselineVersion,
		SelectionSHA256:      selection.Hash,
		SelectedBundleKey:    selection.Selected.Key,
		ImplementationCommit: closedLoopV6ImplementationCommit,
		Review:               closedLoopV6ImplementationReview,
		Artifacts:            artifacts,
	}
	var err error
	seal.Hash, err = hashClosedLoopV6ImplementationSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	data := corpusJSON(t, seal)
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := []byte(fmt.Sprintf("%s  %s\n", corpusHash(data), filepath.Base(jsonPath)))
	if err := os.WriteFile(checksumPath, checksum, 0o644); err != nil {
		t.Fatal(err)
	}
	loadClosedLoopV6CurrentImplementationSeal(t)
}

func loadClosedLoopV6HistoricalImplementationSeal(t *testing.T) closedLoopV6ImplementationSeal {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V6_REVIEWED_IMPLEMENTATION.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V6_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV6ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	selection := loadClosedLoopV6FrozenSelection(t)
	paths := closedLoopV6ImplementationArtifactPaths()
	if seal.Schema != closedLoopV6ImplementationSealSchema {
		t.Fatalf("V6 reviewed implementation schema = %q", seal.Schema)
	}
	if seal.Version != closedLoopV6BaselineVersion {
		t.Fatalf("V6 reviewed implementation version = %d", seal.Version)
	}
	if seal.SelectionSHA256 != selection.Hash || seal.SelectedBundleKey != selection.Selected.Key {
		t.Fatal("V6 reviewed implementation selection binding is invalid")
	}
	if seal.ImplementationCommit != closedLoopV6ImplementationCommit {
		t.Fatalf("V6 reviewed implementation commit = %q", seal.ImplementationCommit)
	}
	if seal.Review != closedLoopV6ImplementationReview {
		t.Fatalf("V6 reviewed implementation review = %q", seal.Review)
	}
	if len(seal.Artifacts) != len(paths) {
		t.Fatalf("V6 reviewed implementation artifact count = %d, want %d", len(seal.Artifacts), len(paths))
	}
	for index, artifact := range seal.Artifacts {
		if artifact.Path != paths[index] || !closedLoopV6ValidHash(artifact.SHA256) {
			t.Fatal("V6 reviewed implementation seal artifact set is invalid")
		}
	}
	if want, err := hashClosedLoopV6ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V6 reviewed implementation seal hash is invalid")
	}
	return seal
}

func loadClosedLoopV6CurrentImplementationSeal(t *testing.T) closedLoopV6ImplementationSeal {
	t.Helper()
	seal := loadClosedLoopV6HistoricalImplementationSeal(t)
	moduleRoot := closedLoopModuleRoot(t)
	for _, artifact := range seal.Artifacts {
		path := filepath.Join(moduleRoot, filepath.FromSlash(artifact.Path))
		if corpusHash(mustCorpusRead(t, path)) != artifact.SHA256 {
			t.Fatalf("V6 reviewed implementation artifact drifted: %s", artifact.Path)
		}
	}
	return seal
}

func closedLoopV6ImplementationArtifactPaths() []string {
	return []string{
		"data/components/audio_power.json",
		"data/model-provenance/registry.json",
		"internal/components/catalog_test.go",
		"internal/components/testdata/golden/coverage_checked_in.json",
		"internal/opentopologysynthesis/graph.go",
		"internal/opentopologysynthesis/graph_test.go",
		"internal/opentopologysynthesis/graph_validate.go",
		"internal/opentopologysynthesis/multi_output_composition.go",
		"internal/opentopologysynthesis/multi_output_composition_test.go",
		"internal/opentopologysynthesis/nonlinear_switching_relationship.go",
		"internal/opentopologysynthesis/nonlinear_switching_simulation_test.go",
		"internal/opentopologysynthesis/realizability.go",
		"internal/opentopologysynthesis/realizability_test.go",
		"internal/opentopologysynthesis/search.go",
		"internal/opentopologysynthesis/search_test.go",
		"internal/opentopologysynthesis/validate.go",
		"internal/opentopologysynthesis/value_domains.go",
		"internal/opentopologysynthesis/value_domains_test.go",
	}
}

func hashClosedLoopV6ImplementationSeal(value closedLoopV6ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}
