// Offline verification of current decision handling against retained real
// provider responses. This never calls a provider or generates a board.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"kicadai/internal/boardfamily"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func load(p string, v any) []byte {
	b, e := os.ReadFile(p)
	must(e)
	must(json.Unmarshal(b, v))
	return b
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func save(p string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	must(e)
	_, e = f.Write(append(b, '\n'))
	must(e)
	must(f.Close())
}
func main() {
	out := flag.String("output", "", "new offline evidence directory")
	flag.Parse()
	if *out == "" || flag.NArg() != 0 {
		panic("--output NEW_DIRECTORY required")
	}
	must(os.Mkdir(*out, 0755))
	var spec struct {
		Configuration boardfamily.Config `json:"configuration_expectations"`
		Capacitance   map[string]float64 `json:"capacitance_overrides_pf"`
		Cases         []struct {
			ID          string `json:"id"`
			Prompt      string `json:"prompt"`
			Disposition string `json:"expected_disposition"`
			Profile     string `json:"expected_profile"`
		} `json:"cases"`
	}
	specBytes := load("specs/board-family-v1/evaluation/language-holdout-proposed.json", &spec)
	seen := map[string]bool{}
	var records []map[string]any
	for _, run := range []string{"acceptance-holdout-01", "acceptance-holdout-02", "acceptance-holdout-03"} {
		base := filepath.Join(".cache/board-family-v1", run)
		var summary struct {
			SpecSHA string `json:"spec_sha256"`
			Cases   []struct {
				ID     string `json:"id"`
				Exit   int    `json:"exit_code"`
				Passed bool   `json:"passed"`
			} `json:"cases"`
		}
		load(filepath.Join(base, "summary.json"), &summary)
		if summary.SpecSHA != hash(specBytes) {
			panic("evaluation changed")
		}
		for _, c := range summary.Cases {
			if seen[c.ID] {
				panic("duplicate response")
			}
			seen[c.ID] = true
			var selection boardfamily.Selection
			bytes := load(filepath.Join(base, c.ID, "selection.json"), &selection)
			prompt, e := os.ReadFile(filepath.Join(base, c.ID+".txt"))
			must(e)
			if len(selection.RawDecision) == 0 {
				panic("raw provider decision missing")
			}
			d, e := boardfamily.DecodeDecision(string(prompt), selection.RawDecision)
			must(e)
			found := false
			for _, want := range spec.Cases {
				if want.ID != c.ID {
					continue
				}
				found = true
				if string(prompt) != want.Prompt || d.Disposition != want.Disposition {
					panic("wrong disposition or altered prompt")
				}
				if want.Disposition == "supported" {
					cfg := spec.Configuration
					cfg.Profile = want.Profile
					for _, p := range boardfamily.Profiles() {
						if p.ID == want.Profile {
							cfg.TotalBusCapacitancePF = p.MaxBusPF
						}
					}
					if pf, ok := spec.Capacitance[c.ID]; ok {
						cfg.TotalBusCapacitancePF = pf
					}
					if d.Configuration == nil || !reflect.DeepEqual(*d.Configuration, cfg) {
						panic("wrong configuration")
					}
				} else if d.Configuration != nil {
					panic("refusal/clarification retained a design")
				}
			}
			if !found {
				panic("unknown case")
			}
			r := map[string]any{"id": c.ID, "run": run, "source_selection_sha256": hash(bytes), "source_response_id": selection.ResponseID, "ledger_index": selection.LedgerIndex, "original_live_exit_code": c.Exit, "original_live_passed": c.Passed, "current_decision": d, "passed": true, "api_requests": 0, "native_generation_attempted": false}
			records = append(records, r)
			save(filepath.Join(*out, c.ID+".json"), r)
		}
	}
	if len(records) != 16 {
		panic("incomplete holdout responses")
	}
	result := map[string]any{"kind": "offline replay of retained live responses, not a live retry or new first attempt", "passed": true, "cases": records, "case_count": 16, "api_requests": 0, "spec_sha256": hash(specBytes)}
	save(filepath.Join(*out, "summary.json"), result)
	fmt.Println(`{"offline_decision_replays":16,"passed":true,"api_requests":0}`)
}
