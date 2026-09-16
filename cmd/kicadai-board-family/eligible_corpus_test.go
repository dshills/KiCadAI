package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/boardfamily"
)

func TestSourceEligibleSyntheticCorpusPreservesAdmission(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(data, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("corpus", err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			// Representation test only. These are pre-existing hand-authored
			// synthetic fixtures, never modified captured provider responses.
			old := groundedCorpusFixture(t, c.ID, c.Prompt)
			var raw map[string]any
			if err := json.Unmarshal(old, &raw); err != nil {
				t.Fatal(err)
			}
			for _, item := range raw["requirements"].([]any) {
				fact := item.(map[string]any)
				if fact["kind"] == "feature" {
					fact["anchor"] = fact["evidence"].([]any)[0]
				}
			}
			raw["version"] = boardfamily.SourceEligibleVersion
			current, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(current)
			got, err := boardfamily.CompileSourceEligibleEvidence(c.Prompt, current)
			if err != nil {
				t.Fatal("synthetic representation rejected", err)
			}
			want, err := boardfamily.CompileGroundedEvidence(c.Prompt, old)
			if err != nil || !bytes.Equal(want, got) {
				t.Fatal("changed internal evidence", err)
			}
			decision, err := boardfamily.DecodeSourceEligibleEvidenceIntent(c.Prompt, current)
			legacy, legacyErr := boardfamily.DecodeGroundedEvidenceIntent(c.Prompt, old)
			if err != nil || legacyErr != nil || !reflect.DeepEqual(decision, legacy) {
				t.Fatal("changed engineering admission", err, legacyErr)
			}
			if !bytes.Equal(before, current) {
				t.Fatal("synthetic raw bytes changed")
			}
		})
	}
}
