package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"kicadai/internal/boardfamily"
)

// Converts HANDCRAFTED TEST FIXTURES, never captured model responses. This is
// test-only: the command and provider transport have no coverage protocol route.
func offlineCoverageCorpusFixture(t testing.TB, prompt string, synthetic []byte) []byte {
	t.Helper()
	input, err := boardfamily.PrepareRequirementCoverageRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]json.RawMessage
	if err := json.Unmarshal(synthetic, &f); err != nil {
		t.Fatal(err)
	}
	var additional map[string][]map[string]any
	if err := json.Unmarshal(f["additional"], &additional); err != nil {
		t.Fatal(err)
	}
	coverage := map[string]any{}
	for _, r := range input.Residuals {
		refs := input.CoverageReferences[r.ID]
		if len(refs) > 0 {
			coverage[r.ID] = map[string]any{"kind": "represented", "references": refs}
		} else {
			coverage[r.ID] = map[string]any{"kind": "non_requirement", "reason": "task_framing"}
		}
	}
	constraints := map[string][]map[string]any{}
	for _, facts := range additional {
		for _, fact := range facts {
			span, ok := fact["span"].(string)
			if !ok {
				t.Fatal("synthetic additional fact needs an explicit span")
			}
			delete(fact, "span")
			constraints[span] = append(constraints[span], fact)
		}
	}
	for span, facts := range constraints {
		coverage[span] = map[string]any{"kind": "constraints", "references": input.CoverageReferences[span], "facts": facts}
	}
	delete(f, "additional")
	f["version"], err = json.Marshal(boardfamily.RequirementCoverageVersion)
	if err != nil {
		t.Fatal(err)
	}
	f["coverage"], err = json.Marshal(coverage)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOfflineCoverageFrozenCorpusPreservesExactSyntheticFacts(t *testing.T) {
	data, err := os.ReadFile("../../specs/board-family-v2/typed-evaluation-02/cases-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID, Prompt  string
			Disposition string `json:"expected_disposition"`
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus changed", err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			old := boundaryCorpusFixture(t, c.ID, c.Prompt)
			current := offlineCoverageCorpusFixture(t, c.Prompt, old)
			before := bytes.Clone(current)
			got, err := boardfamily.CompileRequirementCoverage(c.Prompt, current)
			want, wantErr := boardfamily.CompileSemanticBoundaryEvidence(c.Prompt, old)
			if err != nil || wantErr != nil || !reflect.DeepEqual(canonicalAddressedFacts(t, got), canonicalAddressedFacts(t, want)) {
				t.Fatal("synthetic fact or provenance changed", err, wantErr)
			}
			d, err := boardfamily.DecodeRequirementCoverageIntent(c.Prompt, current)
			legacy, legacyErr := boardfamily.DecodeSemanticBoundaryIntent(c.Prompt, old)
			if err != nil || legacyErr != nil || d.Disposition != c.Disposition || !reflect.DeepEqual(d, legacy) {
				t.Fatal("admission changed", d, legacy, err, legacyErr)
			}
			if !bytes.Equal(before, current) {
				t.Fatal("raw synthetic response mutated")
			}
			t.Log("SYNTHETIC ONLY: exact compiler/admission parity; no provider or native board execution")
		})
	}
}
