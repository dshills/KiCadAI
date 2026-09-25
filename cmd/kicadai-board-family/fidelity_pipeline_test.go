package main

import (
	"encoding/json"
	"testing"

	"kicadai/internal/boardfamily"
)

// Synthetic fixture construction only. Never translate a recorded model output.
func fidelityCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	raw := offlineCoverageCorpusFixture(t, prompt, boundaryCorpusFixture(t, id, prompt))
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture["version"] = boardfamily.FidelityEvidenceVersion
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFidelityCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "requirement-fidelity-v12")
}

func TestFidelityCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "requirement-fidelity-v12")
}

func TestFidelityFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "requirement-fidelity-v12")
}

func TestFidelityCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "requirement-fidelity-v12")
}
