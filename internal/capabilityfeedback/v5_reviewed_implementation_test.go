package capabilityfeedback

import (
	"path/filepath"
	"testing"
)

const closedLoopV5ImplementationSealSchema = "kicadai.closed-loop-open-set-reviewed-implementation.v5"

type closedLoopV5ImplementationSeal struct {
	Schema             string                       `json:"schema"`
	Version            int                          `json:"version"`
	SelectedCapability string                       `json:"selected_capability"`
	Review             string                       `json:"review"`
	Artifacts          []closedLoopArtifactEvidence `json:"artifacts"`
	Hash               string                       `json:"hash"`
}

func TestClosedLoopV5ReviewedImplementationSealIsFrozen(t *testing.T) {
	loadClosedLoopV5CurrentImplementationSeal(t)
}

func loadClosedLoopV5HistoricalImplementationSeal(t *testing.T) closedLoopV5ImplementationSeal {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V5_REVIEWED_IMPLEMENTATION.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V5_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV5ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	if seal.Schema != closedLoopV5ImplementationSealSchema || seal.Version != closedLoopV5BaselineVersion ||
		seal.SelectedCapability != "electrothermal_solver" ||
		seal.Review != "prism_reviewed_no_actionable_findings" || len(seal.Artifacts) == 0 {
		t.Fatal("V5 reviewed implementation seal metadata is invalid")
	}
	if want, err := hashClosedLoopV5ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V5 reviewed implementation seal hash is invalid")
	}
	return seal
}

func loadClosedLoopV5CurrentImplementationSeal(t *testing.T) closedLoopV5ImplementationSeal {
	t.Helper()
	seal := loadClosedLoopV5HistoricalImplementationSeal(t)
	allowedDrift := map[string]bool{
		"internal/opentopologysynthesis/simulation.go":      false,
		"internal/opentopologysynthesis/simulation_test.go": false,
		"internal/simmodel/mna_registry.go":                 false,
	}
	for _, artifact := range seal.Artifacts {
		path := filepath.Join(closedLoopModuleRoot(t), filepath.FromSlash(artifact.Path))
		if corpusHash(mustCorpusRead(t, path)) == artifact.SHA256 {
			continue
		}
		if _, admitted := allowedDrift[artifact.Path]; !admitted {
			t.Fatalf("V5 reviewed implementation artifact drifted outside a later selected boundary: %s", artifact.Path)
		}
		allowedDrift[artifact.Path] = true
	}
	for path, drifted := range allowedDrift {
		if !drifted {
			t.Fatalf("declared later-round V5 artifact did not drift: %s", path)
		}
	}
	return seal
}

func hashClosedLoopV5ImplementationSeal(value closedLoopV5ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}
