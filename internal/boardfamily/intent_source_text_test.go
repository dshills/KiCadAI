package boardfamily

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type boundaryCompiledFact struct {
	Kind, Detail, State string
	Evidence            []string
}

func compileBoundaryFacts(t testing.TB, prompt string, fixture map[string]any) []boundaryCompiledFact {
	t.Helper()
	raw := addressedJSON(t, fixture)
	before := bytes.Clone(raw)
	compiled, err := CompileSemanticBoundaryEvidence(prompt, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("caller evidence changed")
	}
	var result struct {
		Version string
		Facts   []boundaryCompiledFact
	}
	if err := json.Unmarshal(compiled, &result); err != nil {
		t.Fatal(err)
	}
	if result.Version != ConnectionEvidenceVersion {
		t.Fatal("unexpected internal target", result.Version)
	}
	return result.Facts
}

func TestSemanticBoundaryOtherPreservesFullEitherScope(t *testing.T) {
	prompt := "Please make a BMP280 controller powered directly from 5 V USB. I also need wireless telemetry, with no external adapter or change to either requirement."
	input, f := boundaryFixture(t, prompt)
	requestAllBoundaryMentions(t, input, f)
	f["quantities"] = map[string]any{"q0": []any{groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")}}
	var owner SourceResidual
	for _, residual := range input.Residuals {
		if residual.ClauseID == 1 {
			owner = residual
		}
	}
	entry := map[string]any{"kind": "other", "state": "requested", "context": []string{"c0"}, "span": owner.ID}
	f["additional"].(map[string]any)["c1"] = []any{entry}
	// Export mutation cannot supply a narrower source than fresh preparation.
	for i := range input.Residuals {
		input.Residuals[i].Text = "No adapter for wireless only"
	}
	found := false
	for _, fact := range compileBoundaryFacts(t, prompt, f) {
		if fact.Kind == "other" {
			found = true
			if fact.Detail != owner.Text || !member("c0", fact.Evidence...) || !member("c1", fact.Evidence...) {
				t.Fatal("lost source text or context", fact)
			}
		}
	}
	if !found {
		t.Fatal("source requirement disappeared")
	}
	d := checkBoundary(t, prompt, f, "unsupported")
	if !strings.Contains(d.Message, "USB") || !strings.Contains(d.Message, "Wireless") || !strings.Contains(d.Message, "either requirement") {
		t.Fatal("unsupported demands or joint scope missing", d.Message)
	}
	for _, replacement := range []any{"No adapter for wireless only", "No peripheral is permitted", "", nil, 42} {
		entry["detail"] = replacement
		raw := addressedJSON(t, f)
		schema, _ := SemanticBoundarySchema(prompt)
		if checkGroundedSchema(t, schema, raw) == nil {
			t.Fatal("schema permits model-authored other text")
		}
		if result, err := CompileSemanticBoundaryEvidence(prompt, raw); err == nil || result != nil {
			t.Fatal("replacement text accepted or partially compiled", err)
		}
	}
}

func TestSemanticBoundaryOtherPreservesLiteralUnicodeText(t *testing.T) {
	prompt := "Use SHT31. I need galvanic isolation for réseau Δ, not merely a new label."
	input, f := boundaryFixture(t, prompt)
	requestAllBoundaryMentions(t, input, f)
	residual := input.Residuals[1]
	f["additional"].(map[string]any)["c1"] = []any{map[string]any{
		"kind": "other", "state": "requested", "context": []string{}, "span": residual.ID,
	}}
	found := false
	for _, fact := range compileBoundaryFacts(t, prompt, f) {
		if fact.Kind == "other" {
			found = true
			if fact.Detail != residual.Text {
				t.Fatal("source text normalized or paraphrased", fact.Detail)
			}
		}
	}
	if !found {
		t.Fatal("missing literal source text")
	}
}

func TestSemanticBoundaryQuantityOtherPreservesScopeAndIdentity(t *testing.T) {
	for _, prompt := range []string{
		"Use SHT31. Guarantee ambient accuracy within 0.1 C without calibration or bench characterization.",
		"Use SHT31. Guarantee accuracy within 0.1 C and repeatability within 0.1 C without calibration.",
	} {
		t.Run(prompt, func(t *testing.T) {
			input, f := boundaryFixture(t, prompt)
			requestAllBoundaryMentions(t, input, f)
			for id := range input.QuantityRoles {
				f["quantities"].(map[string]any)[id] = []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}}}
			}
			found := map[string]bool{}
			for _, fact := range compileBoundaryFacts(t, prompt, f) {
				if fact.Kind != "other" {
					continue
				}
				for id := range input.QuantityRoles {
					if strings.HasPrefix(fact.Detail, "Source quantity "+id+" (") {
						found[id] = true
						if !strings.HasSuffix(fact.Detail, input.Source.Clauses[1].Text) || !member(id, fact.Evidence...) {
							t.Fatal("numeric qualifier scope or occurrence lost", fact)
						}
					}
				}
			}
			if len(found) != len(input.Source.Quantities) {
				t.Fatal("numeric occurrences collapsed", found)
			}
			checkBoundary(t, prompt, f, "unsupported")
			entry := f["quantities"].(map[string]any)["q0"].([]any)[0].(map[string]any)
			for _, detail := range []any{"Ambient operating minimum is 0.1 C", "Accuracy only needs a sensor data sheet", nil} {
				entry["detail"] = detail
				raw := addressedJSON(t, f)
				schema, _ := SemanticBoundarySchema(prompt)
				if checkGroundedSchema(t, schema, raw) == nil {
					t.Fatal("schema allowed fabricated quantity detail")
				}
				if result, err := CompileSemanticBoundaryEvidence(prompt, raw); err == nil || result != nil {
					t.Fatal("quantity detail accepted or partially compiled", err)
				}
			}
		})
	}
}

func TestSemanticBoundaryUnclearKeepsTargetedQuestions(t *testing.T) {
	for _, quantity := range []bool{false, true} {
		prompt, question := "Use SHT31. I need some isolation but have not chosen the kind.", "Do you require galvanic or optical isolation?"
		if quantity {
			prompt, question = "Use SHT31 with a 0.1 C specification.", "Is 0.1 C an accuracy requirement or an operating-temperature bound?"
		}
		input, f := boundaryFixture(t, prompt)
		requestAllBoundaryMentions(t, input, f)
		entry := map[string]any{"kind": "unclear", "state": "unresolved_choice", "context": []string{}, "detail": question}
		if quantity {
			f["quantities"].(map[string]any)["q0"] = []any{entry}
		} else {
			entry["span"] = input.Residuals[1].ID
			f["additional"].(map[string]any)["c1"] = []any{entry}
		}
		d := checkBoundary(t, prompt, f, "clarify")
		if !strings.Contains(d.Message, question) {
			t.Fatal("targeted question lost", d.Message)
		}
		delete(entry, "detail")
		if result, err := CompileSemanticBoundaryEvidence(prompt, addressedJSON(t, f)); err == nil || result != nil {
			t.Fatal("unclear question silently filled", err)
		}
	}
}

func TestSemanticBoundaryOtherDoesNotChangeHistoricalSchema(t *testing.T) {
	prompt := "Use SHT31 with 0.1 C accuracy. I require galvanic isolation."
	before, err := SourceAddressedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	_, f := addressedFixture(t, prompt)
	f["quantities"] = map[string]any{"q0": []any{map[string]any{"kind": "other", "detail": "0.1 C accuracy", "state": "requested", "context": []string{}}}}
	f["additional"].(map[string]any)["c1"] = []any{map[string]any{"kind": "other", "detail": "galvanic isolation", "state": "requested", "context": []string{}}}
	if _, err := SemanticBoundarySchema(prompt); err != nil {
		t.Fatal(err)
	}
	after, _ := SourceAddressedEvidenceSchema(prompt)
	if !bytes.Equal(addressedJSON(t, before), addressedJSON(t, after)) || checkGroundedSchema(t, after, addressedJSON(t, f)) != nil {
		t.Fatal("v9 schema mutated")
	}
	if _, err := CompileSourceAddressedEvidence(prompt, addressedJSON(t, f)); err != nil {
		t.Fatal("v9 compilation changed", err)
	}
}

func FuzzSemanticBoundaryOtherCompiler(f *testing.F) {
	prompt := "Use SHT31 with 0.1 C accuracy. I also need galvanic isolation."
	input, fixture := boundaryFixture(f, prompt)
	requestAllBoundaryMentions(f, input, fixture)
	fixture["quantities"].(map[string]any)["q0"] = []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}}}
	fixture["additional"].(map[string]any)["c1"] = []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}, "span": input.Residuals[1].ID}}
	f.Add(addressedJSON(f, fixture))
	f.Add([]byte(`{"version":"10-semantic-boundaries-offline","mentions":{},"additional":{},"quantities":{"q0":[null]}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		compiled, err := CompileSemanticBoundaryEvidence(prompt, raw)
		if !bytes.Equal(before, raw) {
			t.Fatal("compiler mutated caller evidence")
		}
		if err != nil {
			if compiled != nil {
				t.Fatal("partial compiled evidence on error")
			}
			return
		}
		schema, schemaErr := SemanticBoundarySchema(prompt)
		if schemaErr != nil || checkGroundedSchema(t, schema, raw) != nil {
			t.Fatal("compiler accepted bytes outside the declared contract", schemaErr)
		}
	})
}
