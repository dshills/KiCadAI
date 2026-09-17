package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"kicadai/internal/boardfamily"
)

// Test-only translation of hand-authored synthetic fixtures, NEVER a repair
// of provider output. Source-owned text replaces the old fixture paraphrases.
func boundaryCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	input, err := boardfamily.PrepareSemanticBoundaryRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Version    string                      `json:"version"`
		Mentions   map[string][]map[string]any `json:"mentions"`
		Additional map[string][]map[string]any `json:"additional"`
		Quantities map[string][]map[string]any `json:"quantities"`
	}
	if err := json.Unmarshal(addressedCorpusFixture(t, id, prompt), &f); err != nil {
		t.Fatal(err)
	}
	f.Version = boardfamily.SemanticBoundaryVersion
	for id, state := range input.RequiredState {
		f.Mentions[id] = []map[string]any{{"state": state, "context": []string{}}}
	}
	for owner, facts := range f.Additional {
		for _, fact := range facts {
			spans := []string{}
			for _, r := range input.Residuals {
				if owner == fmt.Sprint("c", r.ClauseID) {
					spans = append(spans, r.ID)
				}
			}
			if len(spans) != 1 {
				t.Fatal("synthetic fixture needs explicit span selection", id, owner)
			}
			fact["span"] = spans[0]
			if fact["kind"] == "other" {
				delete(fact, "detail")
			}
		}
	}
	for _, facts := range f.Quantities {
		for _, fact := range facts {
			if fact["kind"] == "other" {
				delete(fact, "detail")
			}
		}
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSemanticBoundaryCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "semantic-boundaries-v10")
}

func TestSemanticBoundaryCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "semantic-boundaries-v10")
}

func TestSemanticBoundaryFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "semantic-boundaries-v10")
}

func TestSemanticBoundaryCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "semantic-boundaries-v10")
}
