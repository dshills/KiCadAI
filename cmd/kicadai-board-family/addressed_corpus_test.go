package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/boardfamily"
)

// Adapt only existing hand-authored SYNTHETIC fixtures. This helper never reads
// captured provider output and is not part of a provider or command path.
func addressedCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	input, err := boardfamily.PrepareSourceAddressedRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Requirements []map[string]any
		Quantities   map[string]any
	}
	if err := json.Unmarshal(groundedCorpusFixture(t, id, prompt), &old); err != nil {
		t.Fatal(err)
	}
	mentions, additional := map[string][]map[string]any{}, map[string][]map[string]any{}
	for _, m := range input.Mentions {
		mentions[m.ID] = []map[string]any{{"state": "context_only", "context": []string{}}}
	}
	for _, c := range input.Source.Clauses {
		additional[fmt.Sprint("c", c.ID)] = []map[string]any{}
	}
	for _, req := range old.Requirements {
		refs := []string{}
		for _, ref := range req["evidence"].([]any) {
			refs = append(refs, ref.(string))
		}
		if len(refs) == 0 {
			t.Fatal("synthetic requirement has no source")
		}
		if req["kind"] == "other" || req["kind"] == "unclear" {
			additional[refs[0]] = append(additional[refs[0]], map[string]any{
				"kind": req["kind"], "detail": req["detail"], "state": req["state"], "context": refs[1:],
			})
			continue
		}
		matched := false
		for _, m := range input.Mentions {
			owner := fmt.Sprint("c", m.ClauseID)
			if m.Kind != req["kind"] || m.Value != req["value"] || !slices.Contains(refs, owner) {
				continue
			}
			context := []string{}
			for _, ref := range refs {
				if ref != owner {
					context = append(context, ref)
				}
			}
			if mentions[m.ID][0]["state"] == "context_only" {
				mentions[m.ID] = nil
			}
			mentions[m.ID] = append(mentions[m.ID], map[string]any{"state": req["state"], "context": context})
			matched = true
			break
		}
		if !matched {
			t.Fatalf("no source slot for original synthetic assertion: %+v", req)
		}
	}
	raw, err := json.Marshal(map[string]any{
		"version": boardfamily.SourceAddressedVersion, "mentions": mentions, "additional": additional, "quantities": old.Quantities,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Compare multisets, not sets: no original assertion or repeated assertion may
// disappear. Only fact ordering and citation ordering are irrelevant here.
func canonicalAddressedFacts(t testing.TB, raw []byte) []string {
	t.Helper()
	var wire struct{ Facts []map[string]any }
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	result := []string{}
	for _, fact := range wire.Facts {
		for _, field := range []string{"evidence", "context"} {
			if value, ok := fact[field]; ok {
				refs := []string{}
				for _, ref := range value.([]any) {
					refs = append(refs, ref.(string))
				}
				slices.Sort(refs)
				fact[field] = refs
			}
		}
		encoded, err := json.Marshal(fact)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, string(encoded))
	}
	slices.Sort(result)
	return result
}

func TestSourceAddressedSyntheticCorpusPreservesAllAssertionsAndAdmission(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID, Prompt, Family, Profile string
			Disposition                 string  `json:"expected_disposition"`
			Capacitance                 float64 `json:"total_bus_capacitance_pf"`
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen case inventory changed", err)
	}
	oldInputBytes, newInputBytes, oldOutputBytes, newOutputBytes := 0, 0, 0, 0
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			old := groundedCorpusFixture(t, c.ID, c.Prompt)
			current := addressedCorpusFixture(t, c.ID, c.Prompt)
			before := bytes.Clone(current)
			got, err := boardfamily.CompileSourceAddressedEvidence(c.Prompt, current)
			if err != nil {
				t.Fatal("synthetic representation rejected", err)
			}
			want, err := boardfamily.CompileGroundedEvidence(c.Prompt, old)
			if err != nil || !reflect.DeepEqual(canonicalAddressedFacts(t, got), canonicalAddressedFacts(t, want)) {
				t.Fatalf("changed synthetic fact or source provenance: %v\ngot: %s\nwant: %s", err, got, want)
			}
			decision, err := boardfamily.DecodeSourceAddressedEvidenceIntent(c.Prompt, current)
			legacy, legacyErr := boardfamily.DecodeGroundedEvidenceIntent(c.Prompt, old)
			if err != nil || legacyErr != nil || decision.Disposition != c.Disposition || !reflect.DeepEqual(decision.Configuration, legacy.Configuration) || (decision.Disposition == "clarify" && decision.Message != legacy.Message) {
				t.Fatal("changed engineering admission", decision, legacy, err, legacyErr)
			}
			if c.Disposition == "supported" && (decision.Configuration == nil || decision.Configuration.Family != c.Family || decision.Configuration.Profile != c.Profile || decision.Configuration.TotalBusCapacitancePF != c.Capacitance) {
				t.Fatal("changed useful configuration", decision.Configuration)
			}
			if !bytes.Equal(before, current) {
				t.Fatal("synthetic raw bytes changed")
			}
			oldContract, err := boardfamily.SourceEligibleEvidenceContract(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			newContract, err := boardfamily.SourceAddressedEvidenceContract(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			componentBytes := func(contract map[string]any) int {
				// Serialized design components, not tokens or an HTTP request.
				data, err := json.Marshal(map[string]any{"input": contract["input"], "schema": contract["schema"], "instructions": contract["capability_context"]})
				if err != nil {
					t.Fatal(err)
				}
				return len(data)
			}
			oldInputBytes += componentBytes(oldContract)
			newInputBytes += componentBytes(newContract)
			oldOutputBytes += len(eligibleCorpusFixture(t, c.ID, c.Prompt))
			newOutputBytes += len(current)
			t.Log("SYNTHETIC representation/admission only; not a new AI or native-generation result")
		})
	}
	t.Logf("OFFLINE JSON byte totals for all 14 cases: v8 input components=%d v9 input components=%d; v8 synthetic output=%d v9 synthetic output=%d. Not token, cost, latency, or live-accuracy estimates.", oldInputBytes, newInputBytes, oldOutputBytes, newOutputBytes)
}
