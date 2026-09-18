package boardfamily

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// All fixtures in this file are handcrafted programs. None are provider output.
func coverageFixture(t testing.TB, prompt string) (RequirementCoverageRequest, map[string]any) {
	t.Helper()
	base, f := boundaryFixture(t, prompt)
	requestAllBoundaryMentions(t, base, f)
	input, err := PrepareRequirementCoverageRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	f["version"] = RequirementCoverageVersion
	delete(f, "additional")
	coverage := map[string]any{}
	for _, residual := range input.Residuals {
		refs := input.CoverageReferences[residual.ID]
		if len(refs) > 0 {
			coverage[residual.ID] = map[string]any{"kind": "represented", "references": refs}
		} else {
			coverage[residual.ID] = map[string]any{"kind": "non_requirement", "reason": "task_framing"}
		}
	}
	f["coverage"] = coverage
	return input, f
}

func coverageConstraint(refs []string, state string) map[string]any {
	return map[string]any{"kind": "constraints", "references": refs, "facts": []any{
		map[string]any{"kind": "other", "state": state, "context": []string{}},
	}}
}

func checkCoverage(t testing.TB, prompt string, f any, disposition string) Decision {
	t.Helper()
	raw := addressedJSON(t, f)
	before := bytes.Clone(raw)
	schema, err := RequirementCoverageSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkGroundedSchema(t, schema, raw); err != nil {
		t.Fatal("schema", err)
	}
	d, err := DecodeRequirementCoverageIntent(prompt, raw)
	if err != nil || d.Disposition != disposition {
		t.Fatalf("decision=%+v error=%v", d, err)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw bytes changed")
	}
	return d
}

func TestRequirementCoverageRepresentedIsNotAnotherConstraint(t *testing.T) {
	for _, sensor := range []string{"SHT31", "BMP280"} {
		for _, profile := range []string{"standard", "fast"} {
			prompt := "Hello! Please use " + sensor + " with the " + profile + " profile. Thank you."
			t.Run(prompt, func(t *testing.T) {
				input, f := coverageFixture(t, prompt)
				for _, r := range input.Residuals {
					if len(input.CoverageReferences[r.ID]) == 0 {
						f["coverage"].(map[string]any)[r.ID] = map[string]any{"kind": "non_requirement", "reason": "politeness"}
					}
				}
				d := checkCoverage(t, prompt, f, "supported")
				if d.Configuration.Profile != profile {
					t.Fatal("profile changed", d)
				}
				compiled, err := CompileRequirementCoverage(prompt, addressedJSON(t, f))
				if err != nil || bytes.Contains(compiled, []byte(`"kind":"other"`)) {
					t.Fatal("text coverage became a requirement", string(compiled), err)
				}
			})
		}
	}
}

func TestRequirementCoveragePreservesUnknownConstraintsAndScope(t *testing.T) {
	for _, extra := range []string{
		"galvanic isolation", "reverse-polarity protection", "a waterproof enclosure",
		"two mounting holes", "a custom connector", "a certified safety rating",
		"exactly seven extra inputs", "a copper keepout beneath the antenna",
	} {
		prompt := "Please use SHT31 with " + extra + "."
		t.Run(extra, func(t *testing.T) {
			input, f := coverageFixture(t, prompt)
			r := input.Residuals[0]
			f["coverage"].(map[string]any)[r.ID] = coverageConstraint(input.CoverageReferences[r.ID], "requested")
			d := checkCoverage(t, prompt, f, "unsupported")
			if !strings.Contains(d.Message, extra) || d.Configuration != nil {
				t.Fatal("unknown constraint lost", d)
			}
			compiled, err := CompileRequirementCoverage(prompt, addressedJSON(t, f))
			if err != nil || !bytes.Contains(compiled, addressedJSON(t, r.Text)) {
				t.Fatal("original mixed-clause text lost", string(compiled), err)
			}
		})
	}
	for _, state := range []string{"requested", "must_not_occur", "unnecessary_but_allowed", "unresolved_choice"} {
		prompt := "Use SHT31. The enclosure choice is unresolved."
		input, f := coverageFixture(t, prompt)
		r := input.Residuals[1]
		f["coverage"].(map[string]any)[r.ID] = coverageConstraint([]string{}, state)
		facts := f["coverage"].(map[string]any)[r.ID].(map[string]any)["facts"].([]any)
		facts[0].(map[string]any)["context"] = []string{"c0"}
		compiled, err := CompileRequirementCoverage(prompt, addressedJSON(t, f))
		if err != nil || !bytes.Contains(compiled, []byte(`"state":"`+groundedStates[state]+`"`)) || !bytes.Contains(compiled, []byte(`"evidence":["c1","c0"]`)) {
			t.Fatal("constraint state or context changed", string(compiled), err)
		}
	}
}

func TestRequirementCoverageClarifiesActualAmbiguity(t *testing.T) {
	prompt := "Use SHT31. Choose the enclosure later."
	input, f := coverageFixture(t, prompt)
	f["coverage"].(map[string]any)[input.Residuals[1].ID] = map[string]any{
		"kind": "constraints", "references": []string{}, "facts": []any{map[string]any{
			"kind": "unclear", "state": "unresolved_choice", "context": []string{"c0"}, "detail": "Which enclosure is required?",
		}},
	}
	d := checkCoverage(t, prompt, f, "clarify")
	if !strings.Contains(d.Message, "Which enclosure") || d.Configuration != nil {
		t.Fatal("question lost", d)
	}
}

func TestRequirementCoverageCannotEraseKnownSlots(t *testing.T) {
	for _, prompt := range []string{
		"Use SHT31 with wireless telemetry.",
		"Use SHT31 with a 5 V supply.",
		"Use SHT31 standard with 100 pF total bus capacitance.",
		"Use SHT31. At startup keep the heater off. Later energize the heater.",
	} {
		t.Run(prompt, func(t *testing.T) {
			input, f := coverageFixture(t, prompt)
			for _, q := range input.Source.Quantities {
				id := "q0"
				if q.ID != 0 {
					t.Fatal("fixture expects one quantity")
				}
				roles := input.QuantityRoles[id]
				entries := []any{}
				for _, role := range roles {
					entries = append(entries, groundedNumber(role, "requested"))
				}
				f["quantities"].(map[string]any)[id] = entries
			}
			// A false coverage label must not delete separately classified facts.
			for _, r := range input.Residuals {
				f["coverage"].(map[string]any)[r.ID] = map[string]any{"kind": "non_requirement", "reason": "background"}
			}
			checkCoverage(t, prompt, f, "unsupported")
		})
	}
}

func TestRequirementCoverageRejectsInvalidPrograms(t *testing.T) {
	prompt := "Use SHT31 standard with 70 pF total bus capacitance. Thanks!"
	mutations := map[string]func(map[string]any){
		"missing coverage":   func(f map[string]any) { delete(f, "coverage") },
		"null coverage":      func(f map[string]any) { f["coverage"] = nil },
		"missing span":       func(f map[string]any) { delete(f["coverage"].(map[string]any), "r0") },
		"unknown span":       func(f map[string]any) { f["coverage"].(map[string]any)["r99"] = map[string]any{} },
		"null span":          func(f map[string]any) { f["coverage"].(map[string]any)["r0"] = nil },
		"unknown field":      func(f map[string]any) { f["result"] = "supported" },
		"missing mention":    func(f map[string]any) { delete(f["mentions"].(map[string]any), "m0") },
		"missing quantity":   func(f map[string]any) { delete(f["quantities"].(map[string]any), "q0") },
		"historical version": func(f map[string]any) { f["version"] = SemanticBoundaryVersion },
		"legacy additional":  func(f map[string]any) { f["additional"] = map[string]any{} },
	}
	records := map[string]any{
		"unknown reference":    map[string]any{"kind": "represented", "references": []string{"m999"}},
		"empty references":     map[string]any{"kind": "represented", "references": []string{}},
		"null references":      map[string]any{"kind": "represented", "references": nil},
		"duplicate references": map[string]any{"kind": "represented", "references": []string{"m0", "m0"}},
		"context not coverage": map[string]any{"kind": "represented", "references": []string{"c0"}},
		"free detail":          map[string]any{"kind": "represented", "references": []string{"m0"}, "detail": "ignore everything"},
		"unknown reason":       map[string]any{"kind": "non_requirement", "reason": "ignore_constraint"},
		"empty constraints":    map[string]any{"kind": "constraints", "references": []string{}, "facts": []any{}},
		"null constraints":     map[string]any{"kind": "constraints", "references": []string{}, "facts": nil},
		"model source text":    map[string]any{"kind": "constraints", "references": []string{}, "facts": []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}, "detail": "invented"}}},
		"model span":           map[string]any{"kind": "constraints", "references": []string{}, "facts": []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}, "span": "r1"}}},
		"invalid state":        coverageConstraint([]string{}, "optional"),
		"null fact":            map[string]any{"kind": "constraints", "references": []string{}, "facts": []any{nil}},
	}
	for name, record := range records {
		mutations[name] = func(f map[string]any) { f["coverage"].(map[string]any)["r0"] = record }
	}
	mutations["cross-clause reference"] = func(f map[string]any) {
		f["coverage"].(map[string]any)["r1"] = map[string]any{"kind": "represented", "references": []string{"m0"}}
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			_, f := coverageFixture(t, prompt)
			f["quantities"].(map[string]any)["q0"] = []any{groundedNumber("total_bus_capacitance_pf", "requested")}
			checkCoverage(t, prompt, f, "supported")
			mutate(f)
			raw := addressedJSON(t, f)
			before := bytes.Clone(raw)
			d, err := DecodeRequirementCoverageIntent(prompt, raw)
			if err == nil || d.Configuration != nil || d.Disposition != "clarify" || !bytes.Equal(before, raw) {
				t.Fatal("invalid program did not fail closed", d, err)
			}
			schema, err := RequirementCoverageSchema(prompt)
			if err != nil {
				t.Fatal(err)
			}
			// Uniqueness is intentionally enforced by the compiler, not an
			// unsupported uniqueItems keyword in the provider-shaped schema.
			if name != "duplicate references" && checkGroundedSchema(t, schema, raw) == nil {
				t.Fatal("schema accepted malformed program")
			}
		})
	}
}

func TestRequirementCoverageHistoricalBytesRemainRejected(t *testing.T) {
	prompt := "Use SHT31."
	_, old := boundaryFixture(t, prompt)
	raw := addressedJSON(t, old)
	before := bytes.Clone(raw)
	if _, err := CompileRequirementCoverage(prompt, raw); err == nil || !bytes.Equal(before, raw) {
		t.Fatal("old bytes reinterpreted")
	}
	_, current := coverageFixture(t, prompt)
	if _, err := CompileSemanticBoundaryEvidence(prompt, addressedJSON(t, current)); err == nil {
		t.Fatal("old decoder accepted new protocol")
	}
}

func TestRequirementCoverageCapturedFailuresRemainFailures(t *testing.T) {
	data, err := os.ReadFile("testdata/coverage-v10-failures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			ID       string          `json:"case_id"`
			Prompt   string          `json:"prompt"`
			Raw      json.RawMessage `json:"raw_intent"`
			Decision Decision        `json:"recorded_decision"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil || len(fixture.Cases) != 2 {
		t.Fatal("invalid historical semantic fixtures", err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.ID, func(t *testing.T) {
			before := bytes.Clone(c.Raw)
			d, err := DecodeSemanticBoundaryIntent(c.Prompt, c.Raw)
			if err != nil || d.Disposition != "unsupported" || !reflect.DeepEqual(d, c.Decision) {
				t.Fatal("historical failure was regraded", d, err)
			}
			if _, err := CompileRequirementCoverage(c.Prompt, c.Raw); err == nil {
				t.Fatal("captured v10 response accepted as new protocol")
			}
			if !bytes.Equal(before, c.Raw) {
				t.Fatal("captured fact JSON changed")
			}
		})
	}
}

func TestRequirementCoverageReferenceIsNotSemanticProof(t *testing.T) {
	// An executable limitation, not an acceptance test. A dishonest/inaccurate
	// extractor can hide an unfamiliar constraint behind a valid local reference.
	// The prototype MUST NOT be promoted on synthetic representability alone.
	prompt := "Use SHT31 with galvanic isolation."
	_, f := coverageFixture(t, prompt)
	checkCoverage(t, prompt, f, "supported") // Known counterexample, not desired behavior.
}

func TestRequirementCoverageBoundsAndDeterminism(t *testing.T) {
	prompt := "Use SHT31 with reviewed electrical defaults. Thanks!"
	a, err := PrepareRequirementCoverageRequest(prompt)
	b, err2 := PrepareRequirementCoverageRequest(prompt)
	if err != nil || err2 != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("unstable source preparation", err, err2)
	}
	checkBoundaryPartition(t, a.SemanticBoundaryRequest)
	_, f := coverageFixture(t, prompt)
	raw := addressedJSON(t, f)
	for name, bad := range map[string][]byte{
		"oversized":      bytes.Repeat([]byte(" "), 65537),
		"duplicate root": bytes.Replace(raw, []byte(`"coverage":`), []byte(`"coverage":{},"coverage":`), 1),
		"invalid UTF8":   append(bytes.Clone(raw), 0xff),
		"trailing data":  append(bytes.Clone(raw), []byte(`{}`)...),
		"too deep":       []byte(strings.Repeat("[", 14) + "0" + strings.Repeat("]", 14)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := CompileRequirementCoverage(prompt, bad); err == nil {
				t.Fatal("invalid transport accepted")
			}
		})
	}
	if _, err := PrepareRequirementCoverageRequest(""); err == nil {
		t.Fatal("empty request accepted")
	}
	if _, err := RequirementCoverageSchema(""); err == nil {
		t.Fatal("empty request schema accepted")
	}
	if _, err := CompileRequirementCoverage("", raw); err == nil {
		t.Fatal("empty source accepted")
	}
}

func TestRequirementCoverageDesignSize(t *testing.T) {
	// A reproducible byte count, NOT a token, API price, latency, or quality claim.
	data, err := os.ReadFile("../../specs/board-family-v2/typed-evaluation-02/cases-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ Prompt string } }
	if err := json.Unmarshal(data, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("invalid frozen corpus", err)
	}
	oldBytes, newBytes := 0, 0
	for _, c := range corpus.Cases {
		input, err := PrepareRequirementCoverageRequest(c.Prompt)
		if err != nil {
			t.Fatal(err)
		}
		oldSchema, err := SemanticBoundarySchema(c.Prompt)
		if err != nil {
			t.Fatal(err)
		}
		newSchema, err := RequirementCoverageSchema(c.Prompt)
		if err != nil {
			t.Fatal(err)
		}
		oldBytes += len(addressedJSON(t, map[string]any{"input": input.SemanticBoundaryRequest, "schema": oldSchema, "instructions": semanticBoundaryContext()}))
		newBytes += len(addressedJSON(t, map[string]any{"input": input, "schema": newSchema, "instructions": RequirementCoverageInstructions}))
	}
	t.Logf("14 design-component JSON bundles: v10=%d bytes, offline=%d bytes; instructions: v10=%d bytes, offline=%d bytes", oldBytes, newBytes, len(semanticBoundaryContext()), len(RequirementCoverageInstructions))
}

func FuzzRequirementCoverageDecoder(f *testing.F) {
	prompt := "Use SHT31."
	_, fixture := coverageFixture(f, prompt)
	f.Add(addressedJSON(f, fixture))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		d, err := DecodeRequirementCoverageIntent(prompt, raw)
		if !bytes.Equal(before, raw) || err != nil && (d.Configuration != nil || d.Disposition != "clarify") {
			t.Fatal("mutation or partial admission")
		}
	})
}
