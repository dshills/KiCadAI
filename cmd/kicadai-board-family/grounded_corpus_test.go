package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

// This adapts only pre-existing hand-authored SYNTHETIC test fixtures. No
// captured provider output is read, corrected or relabelled as new success.
func groundedCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	source, err := boardfamily.PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	var old struct{ Facts []map[string]any }
	if err := json.Unmarshal(connectionCorpusFixture(t, id, prompt), &old); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{"required": "requested", "not_required": "unnecessary_but_allowed", "forbidden": "must_not_occur", "uncertain": "unresolved_choice"}
	requirements := []map[string]any{}
	quantities := map[string][]map[string]any{}
	for _, fact := range old.Facts {
		state, ok := states[fact["state"].(string)]
		if !ok {
			t.Fatal("unknown fixture state")
		}
		fact["state"] = state
		if fact["kind"] == "number" {
			id, field, found := strings.Cut(fact["choice"].(string), "/")
			if !found {
				t.Fatal("invalid fixture quantity choice")
			}
			quantities[id] = append(quantities[id], map[string]any{"kind": "number", "field": field, "state": state, "context": fact["context"]})
			continue
		}
		refs := []string{}
		for _, ref := range fact["evidence"].([]any) {
			s := ref.(string)
			if strings.HasPrefix(s, "c") {
				refs = append(refs, s)
				continue
			}
			var qid int
			if _, err := fmt.Sscanf(s, "q%d", &qid); err != nil || qid < 0 || qid >= len(source.Quantities) {
				t.Fatal("invalid fixture quantity", err)
			}
			q := source.Quantities[qid]
			owner := fmt.Sprintf("c%d", q.ClauseID)
			if !slices.Contains(refs, owner) {
				refs = append(refs, owner)
			}
			switch fact["value"] {
			case "usb":
				// The interface demand and its explicit input voltage are
				// separate requirements; do not lose either in partitioning.
				for _, field := range []string{"supply_min_v", "supply_max_v"} {
					if q.Fields[field] != 5 {
						t.Fatal("USB fixture source changed")
					}
					quantities[s] = append(quantities[s], map[string]any{"kind": "number", "field": field, "state": state, "context": []string{}})
				}
			case "accuracy_guarantee":
				quantities[s] = []map[string]any{{"kind": "other", "detail": "The requested assembled-board ambient temperature accuracy tolerance is within " + q.Text + ".", "state": state, "context": []string{}}}
			default:
				t.Fatal("new numeric-feature fixture requires source review")
			}
		}
		fact["evidence"] = refs
		requirements = append(requirements, fact)
	}
	// These additional synthetic assertions preserve explicit source meaning
	// not needed by the older decision-only fixtures. No production parser
	// depends on corpus IDs, wording fragments, gold answers or these fixtures.
	add := func(kind, value, state string, refs ...string) {
		requirements = append(requirements, map[string]any{"kind": kind, "value": value, "state": state, "evidence": refs})
	}
	switch id {
	case "choice-02":
		add("measurement", "pressure", "unresolved_choice", "c0", "c1")
		add("measurement", "humidity", "unresolved_choice", "c0", "c1")
	case "refuse-01":
		add("measurement", "pressure", "requested", "c1")
		add("measurement", "humidity", "requested", "c1")
	case "refuse-05":
		requirements = append(requirements, map[string]any{"kind": "other", "detail": "No external adapter may be added or either stated requirement changed.", "state": "requested", "evidence": []string{"c1"}})
	case "refuse-06":
		add("measurement", "temperature", "requested", "c0")
		add("measurement", "humidity", "requested", "c0")
		requirements = append(requirements, map[string]any{"kind": "other", "detail": "The accuracy guarantee must not depend on calibration or bench characterization.", "state": "requested", "evidence": []string{"c1"}})
	}
	raw, err := json.Marshal(map[string]any{"version": boardfamily.GroundedEvidenceVersion, "requirements": requirements, "quantities": quantities})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestGroundedSyntheticCorpusRetainsEveryGoldAssertion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID, Prompt, Family, Profile string
			Disposition                 string  `json:"expected_disposition"`
			Capacitance                 float64 `json:"total_bus_capacitance_pf"`
			Requirements                []struct {
				ID    string
				AnyOf []struct {
					Kind, Value, State string
					Number             *float64
				} `json:"any_of"`
			}
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("acceptance inventory changed")
	}
	goldCount, quantityCount := 0, 0
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw := groundedCorpusFixture(t, c.ID, c.Prompt)
			before := bytes.Clone(raw)
			d, err := boardfamily.DecodeGroundedEvidenceIntent(c.Prompt, raw)
			if err != nil || d.Disposition != c.Disposition {
				t.Fatal("synthetic admission", d, err)
			}
			if c.Disposition == "supported" && (d.Configuration == nil || d.Configuration.Family != c.Family || d.Configuration.Profile != c.Profile || d.Configuration.TotalBusCapacitancePF != c.Capacitance) {
				t.Fatal("changed useful configuration", d.Configuration)
			}
			if (c.ID == "useful-03" || c.ID == "refuse-06") && strings.Contains(d.Message, "heater operation") {
				t.Fatal("invented heater requirement")
			}
			if c.ID == "useful-03" && strings.Contains(d.Message, "CRC") {
				t.Fatal("invented CRC requirement")
			}
			if c.ID == "refuse-05" && (!strings.Contains(d.Message, "USB") || !strings.Contains(d.Message, "Wireless")) {
				t.Fatal("refusal omitted USB or wireless")
			}
			if c.ID == "refuse-06" && !strings.Contains(d.Message, "bench characterization") {
				t.Fatal("accuracy refusal lost measurement limitation")
			}
			compiled, err := boardfamily.CompileGroundedEvidence(c.Prompt, raw)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Facts []struct {
					Kind, Value, State, Choice, Detail string
					Evidence                           []string
				}
			}
			if err := json.Unmarshal(compiled, &wire); err != nil {
				t.Fatal(err)
			}
			source, err := boardfamily.PrepareOwnedEvidenceRequest(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			quantityCount += len(source.Quantities)
			for _, req := range c.Requirements {
				found := false
				for _, wanted := range req.AnyOf {
					state := wanted.State
					if state == "" {
						state = "required"
					}
					for _, f := range wire.Facts {
						kind, value := f.Kind, f.Value
						if kind == "connection" && value == "wireless" {
							kind, value = "feature", "wireless_operation"
						}
						if f.State != state {
							continue
						}
						if kind == "number" {
							for _, q := range source.NumericChoices {
								if f.Choice == q.ID && wanted.Kind == "number" && wanted.Value == q.Field && wanted.Number != nil && *wanted.Number == q.Value {
									found = true
								}
							}
						} else if kind == wanted.Kind && value == wanted.Value {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("lost original gold assertion %s", req.ID)
				}
				goldCount++
			}
			if !bytes.Equal(before, raw) {
				t.Fatal("synthetic extraction bytes modified")
			}
			t.Logf("SYNTHETIC: %d original assertions and %d quantity occurrences represented; not an AI accuracy result", len(c.Requirements), len(source.Quantities))
		})
	}
	if goldCount != 47 || quantityCount != 15 {
		t.Fatal("original acceptance scope changed", goldCount, quantityCount)
	}
}
