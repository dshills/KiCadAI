package main

import (
	"encoding/json"
	"testing"

	"kicadai/internal/boardfamily"
)

// Synthetic fixture construction only. Never translate a recorded model output.
func coverageCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	raw := offlineCoverageCorpusFixture(t, prompt, boundaryCorpusFixture(t, id, prompt))
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture["version"] = boardfamily.CoverageEvidenceVersion
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCoverageCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "requirement-coverage-v11")
}

func TestCoverageCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "requirement-coverage-v11")
}

func TestCoverageFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "requirement-coverage-v11")
}

func TestCoverageCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "requirement-coverage-v11")
}
