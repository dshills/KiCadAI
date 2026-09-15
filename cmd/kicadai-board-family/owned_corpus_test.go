package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/boardfamily"
)

// Convert only the pre-existing hand-authored TEST fixture. This does not read,
// convert, fix or rescore a captured provider response, or select a CLI route.
func ownedCorpusFixture(t testing.TB, id, prompt string) ([]byte, []byte) {
	t.Helper()
	synthetic := indexedCorpusFixture(t, id, prompt)
	var original boardfamily.ReferencedIntent
	if err := json.Unmarshal(synthetic, &original); err != nil {
		t.Fatal(err)
	}
	source, err := boardfamily.PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	facts := []map[string]any{}
	for _, f := range original.Facts {
		next := map[string]any{"kind": f.Kind, "state": f.State}
		refs := []string{}
		if f.Kind == "number" {
			if len(f.Quantities) != 1 {
				t.Fatal("synthetic number must bind one quantity")
			}
			q := source.Quantities[f.Quantities[0]]
			next["choice"] = fmt.Sprintf("q%d/%s", q.ID, f.Value)
			for _, c := range f.Sources {
				if c != q.ClauseID {
					refs = append(refs, fmt.Sprintf("c%d", c))
				}
			}
			next["context"] = refs
		} else {
			key, value := "value", f.Value
			if f.Kind == "other" || f.Kind == "unclear" {
				key, value = "detail", f.Detail
			}
			next[key] = value
			owners := []int{}
			for _, q := range f.Quantities {
				refs = append(refs, fmt.Sprintf("q%d", q))
				owners = append(owners, source.Quantities[q].ClauseID)
			}
			for _, c := range f.Sources {
				if !slices.Contains(owners, c) {
					refs = append(refs, fmt.Sprintf("c%d", c))
				}
			}
			next["evidence"] = refs
		}
		facts = append(facts, next)
	}
	encoded, err := json.Marshal(map[string]any{"version": boardfamily.OwnedEvidenceVersion, "facts": facts})
	if err != nil {
		t.Fatal(err)
	}
	return synthetic, encoded
}

func TestOwnedCorpusSyntheticAdmissionParity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Cases []struct {
			ID          string `json:"id"`
			Prompt      string `json:"prompt"`
			Disposition string `json:"expected_disposition"`
			Family      string `json:"family"`
			Profile     string `json:"profile"`
		}
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Cases) != 14 {
		t.Fatal("expected unchanged 14-case synthetic regression corpus")
	}
	for _, c := range spec.Cases {
		t.Run(c.ID, func(t *testing.T) {
			old, raw := ownedCorpusFixture(t, c.ID, c.Prompt)
			before, err := boardfamily.DecodeReferencedIntent(c.Prompt, old)
			if err != nil {
				t.Fatal(err)
			}
			after, err := boardfamily.DecodeOwnedEvidenceIntent(c.Prompt, raw)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("full synthetic decision/configuration parity lost: before=%+v after=%+v err=%v", before, after, err)
			}
			if after.Disposition != c.Disposition {
				t.Fatalf("wrong disposition: %+v", after)
			}
			if c.Disposition == "supported" && (after.Configuration == nil || after.Configuration.Family != c.Family || after.Configuration.Profile != c.Profile) {
				t.Fatalf("wrong synthetic configuration: %+v", after.Configuration)
			}
			if c.Disposition != "supported" && after.Configuration != nil {
				t.Fatal("non-supported request produced a configuration")
			}
			t.Logf("synthetic parity only: old response=%d bytes, owned response=%d bytes", len(old), len(raw))
		})
	}
}
