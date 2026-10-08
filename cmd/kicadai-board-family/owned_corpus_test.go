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
			dir := t.TempDir()
			output, ledger, budget, journal := filepath.Join(dir, "output"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "budget.json"), filepath.Join(dir, "evidence")
			if err := save(budget, boardfamily.LedgerPolicy{Goal: "offline-owned-" + c.ID, MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			t.Setenv("OPENAI_API_KEY", "offline-command-placeholder")
			pipeline, counts := ownedCommandPipeline(t, raw, "", "")
			if _, err := runIndexedCommand(t, pipeline, "--intent-protocol", "owned-v4", "--prompt", c.Prompt, "--live-budget", budget, "--ledger", ledger, "--evidence-journal", journal, "--output", output); err != nil {
				t.Fatal("owned command orchestration failed", err)
			}
			wantNativeCalls := 0
			if c.Disposition == "supported" {
				wantNativeCalls = 1
				compareOwnedGeneratedFiles(t, output, c.Family, c.Profile)
			}
			if counts.requests != 1 || counts.generate != wantNativeCalls || counts.validate != wantNativeCalls {
				t.Fatalf("wrong command boundaries: %+v", counts)
			}
			audit, err := boardfamily.InspectOwnedJournal(journal)
			if err != nil || !reflect.DeepEqual(audit.Selection.Decision, after) || audit.Selection.AdmissionVersion != boardfamily.OwnedEvidenceVersion || len(audit.FilesSHA256) != 8 || len(audit.Ledger.Entries) != 1 {
				t.Fatalf("owned journal replay lost the full decision/accounting: %+v %v", audit, err)
			}
			if _, err := boardfamily.InspectReferencedJournal(journal); err == nil {
				t.Fatal("legacy inspector accepted an owned-v4 journal")
			}
			t.Logf("synthetic parity only: old response=%d bytes, owned response=%d bytes", len(old), len(raw))
		})
	}
}

func compareOwnedGeneratedFiles(t testing.TB, output, family, profile string) {
	t.Helper()
	name := "bmp280"
	if family == boardfamily.FamilySHT31 {
		name = "sht31"
	}
	published := filepath.Join("..", "..", "examples", "board-family-v2", name+"-"+profile)
	compared := 0
	err := filepath.WalkDir(output, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(output, path)
		if err != nil || rel == "selection.json" {
			return err
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(filepath.Join(published, rel))
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			return fmt.Errorf("owned command changed reviewed generated file %s/%s", name+"-"+profile, rel)
		}
		compared++
		return nil
	})
	if err != nil || compared == 0 {
		t.Fatalf("generated file comparison failed: compared=%d err=%v", compared, err)
	}
	t.Logf("all %d newly generated files match reviewed %s-%s; validation stub is not a new native qualification", compared, name, profile)
}
