package boardfamily

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ownedRaw(t testing.TB, facts ...map[string]any) []byte {
	t.Helper()
	if facts == nil {
		facts = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"version": OwnedEvidenceVersion, "facts": facts})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ownedFact(kind, value, state string, evidence ...string) map[string]any {
	return map[string]any{"kind": kind, "value": value, "state": state, "evidence": evidence}
}

func ownedNumber(choice, state string, context ...string) map[string]any {
	if context == nil {
		context = []string{}
	}
	return map[string]any{"kind": "number", "choice": choice, "state": state, "context": context}
}

func ownedAccepts(t testing.TB, prompt string, facts ...map[string]any) ReferencedIntent {
	t.Helper()
	raw := ownedRaw(t, facts...)
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkReferencedSchemaJSON(schema, raw); err != nil {
		t.Fatal("new synthetic shape does not fit its schema:", err)
	}
	compiled, err := CompileOwnedEvidenceIntent(prompt, raw)
	if err != nil {
		t.Fatal("schema-valid synthetic structural input failed:", err)
	}
	return compiled
}

func TestOwnedEvidenceAtomicNumericReferences(t *testing.T) {
	prompt := "Use BMP280 with 2.2k pull-ups, 400 kHz I2C and 100 pF total bus capacitance. Keep those values unchanged."
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.NumericChoices) != 3 {
		t.Fatalf("wrong choices: %+v", request.NumericChoices)
	}
	for _, choice := range request.NumericChoices {
		for _, state := range []string{"required", "not_required", "forbidden", "uncertain"} {
			compiled := ownedAccepts(t, prompt, ownedNumber(choice.ID, state, "c1"))
			f := compiled.Facts[0]
			if f.Value != choice.Field || f.State != state || !reflect.DeepEqual(f.Sources, []int{0, 1}) || !reflect.DeepEqual(f.Quantities, []int{choice.QuantityID}) {
				t.Fatalf("atomic relation lost: %+v", f)
			}
			if value := request.Quantities[f.Quantities[0]].Fields[f.Value]; value != choice.Value {
				t.Fatalf("converted value changed: %v != %v", value, choice.Value)
			}
		}
	}
}

func TestOwnedEvidenceQuantityReferencesOwnTheirSources(t *testing.T) {
	prompt := "Use BMP280 powered from 5 V USB. I also require wireless telemetry."
	compiled := ownedAccepts(t, prompt, ownedFact("feature", "usb", "required", "q0", "c1"))
	if !reflect.DeepEqual(compiled.Facts[0].Sources, []int{0, 1}) || !reflect.DeepEqual(compiled.Facts[0].Quantities, []int{0}) {
		t.Fatalf("quantity did not establish its own provenance: %+v", compiled)
	}
	// The compiler does not pretend the extra wireless-clause citation proves
	// USB meaning. Semantic source scope still requires actual evaluation.
}

func TestOwnedEvidenceDoesNotTrustCallerTables(t *testing.T) {
	prompt := "Use BMP280 with 2.2k pull-ups. Keep that value unchanged."
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	request.Quantities[0].Fields["pullup_ohms"] = 4700
	request.Quantities[0].ClauseID = 1
	request.NumericChoices[0].Field = "clock_hz"
	request.NumericChoices[0].Value = 400000
	compiled := ownedAccepts(t, prompt, ownedNumber("q0/pullup_ohms", "required"))
	fresh, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil || fresh.Quantities[0].Fields["pullup_ohms"] != 2200 || !reflect.DeepEqual(compiled.Facts[0].Sources, []int{0}) {
		t.Fatalf("caller-modified tables affected authoritative compilation: %+v %v", compiled, err)
	}
}

func TestOwnedEvidenceReferenceSetsPreserveRawAndContext(t *testing.T) {
	prompt := "Use BMP280. Keep that sensor. Do not choose a different sensor."
	raw := ownedRaw(t, ownedFact("sensor", "BMP280", "required", "c1", "c0", "c1"))
	before := append([]byte(nil), raw...)
	compiled, err := CompileOwnedEvidenceIntent(prompt, raw)
	if err != nil || !reflect.DeepEqual(compiled.Facts[0].Sources, []int{0, 1}) {
		t.Fatalf("set-union reference semantics changed: %+v %v", compiled, err)
	}
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("raw model bytes were rewritten")
	}
	ownedAccepts(t, prompt, ownedFact("sensor", "BMP280", "required", "c1", "c0", "c1"))
}

func TestOwnedEvidenceMultipleQuantitiesAndRepeatedOccurrences(t *testing.T) {
	prompt := "Use BMP280 with 5 V input and 1 A supply capacity. Also declare 100 pF total bus capacitance and repeat 100 pF."
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil || len(request.Quantities) != 4 {
		t.Fatalf("occurrences missing: %+v %v", request, err)
	}
	compiled := ownedAccepts(t, prompt, ownedFact("feature", "usb", "required", "q0", "q1"),
		ownedNumber("q2/total_bus_capacitance_pf", "required"), ownedNumber("q3/total_bus_capacitance_pf", "not_required"))
	if !reflect.DeepEqual(compiled.Facts[0].Quantities, []int{0, 1}) || !reflect.DeepEqual(compiled.Facts[1].Quantities, []int{2}) || !reflect.DeepEqual(compiled.Facts[2].Quantities, []int{3}) {
		t.Fatalf("distinct occurrences or multi-quantity fact lost: %+v", compiled)
	}
}

func TestOwnedEvidenceUnmappedQuantityRetained(t *testing.T) {
	prompt := "Use SHT31 with a 5 mm board clearance."
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil || len(request.Quantities) != 1 || len(request.NumericChoices) != 0 {
		t.Fatalf("unsupported dimension must remain a non-configurable occurrence: %+v %v", request, err)
	}
	fact := map[string]any{"kind": "other", "detail": "A 5 mm board clearance is required.", "state": "required", "evidence": []string{"q0"}}
	compiled := ownedAccepts(t, prompt, fact)
	if !reflect.DeepEqual(compiled.Facts[0].Quantities, []int{0}) {
		t.Fatal("unsupported quantity was dropped")
	}
	d, err := DecodeOwnedEvidenceIntent(prompt, ownedRaw(t, fact))
	if err != nil || d.Disposition != "unsupported" || d.Configuration != nil {
		t.Fatalf("unsupported requirement did not reach admission: %+v %v", d, err)
	}
}

func TestOwnedEvidenceEmptyAndOmittedQuantityBehavior(t *testing.T) {
	d, err := DecodeOwnedEvidenceIntent("Hello, can you make a sensor controller?", ownedRaw(t))
	if err != nil || d.Disposition != "clarify" || d.Configuration != nil {
		t.Fatalf("empty greeting must ask a targeted measurement question: %+v %v", d, err)
	}
	prompt := "Use SHT31 with the fast profile and 100 pF total bus capacitance."
	d, err = DecodeOwnedEvidenceIntent(prompt, ownedRaw(t, ownedFact("sensor", "SHT31", "required", "c0"), ownedFact("profile", "fast", "required", "c0")))
	if err != nil || d.Disposition != "clarify" || d.Configuration != nil || !strings.Contains(d.Message, "100 pF") {
		t.Fatalf("omitted quantity must not silently default: %+v %v", d, err)
	}
}

func TestOwnedEvidenceCompileFailureReturnsNoPartialFacts(t *testing.T) {
	prompt := "Use BMP280 with 2.2k pull-ups."
	raw := ownedRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedNumber("q0/clock_hz", "required"))
	compiled, err := CompileOwnedEvidenceIntent(prompt, raw)
	if err == nil || len(compiled.Facts) != 0 {
		t.Fatalf("invalid second fact exposed a partial result: %+v %v", compiled, err)
	}
	d, err := DecodeOwnedEvidenceIntent(prompt, raw)
	if err == nil || d.Disposition != "clarify" || d.Configuration != nil {
		t.Fatalf("invalid extraction escaped fail-closed admission: %+v %v", d, err)
	}
}

func TestOwnedEvidenceTemporalAndNegativeRequirements(t *testing.T) {
	prompt := "Use SHT31. At startup keep the heater off. Later energize its heater."
	raw := ownedRaw(t, ownedFact("sensor", "SHT31", "required", "c0"),
		ownedFact("feature", "heater_operation", "forbidden", "c1"), ownedFact("feature", "heater_operation", "required", "c2"))
	d, err := DecodeOwnedEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" || !strings.Contains(d.Message, "heater") {
		t.Fatalf("startup prohibition erased later demand: %+v %v", d, err)
	}
}

func TestOwnedEvidenceRejectsInvalidShapesAndRelations(t *testing.T) {
	prompt := "Use BMP280 with 2.2k pull-ups. Keep that value unchanged."
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	for name, fact := range map[string]map[string]any{
		"wrong-dimensional-field": ownedNumber("q0/clock_hz", "required"),
		"unknown-quantity":        ownedNumber("q1/pullup_ohms", "required"),
		"fabricated-choice":       ownedNumber("q0/pullup_ohms=4700", "required"),
		"foreign-context":         ownedNumber("q0/pullup_ohms", "required", "c99"),
		"quantity-as-context":     ownedNumber("q0/pullup_ohms", "required", "q0"),
		"quantity-on-identity":    ownedFact("sensor", "BMP280", "required", "q0"),
		"foreign-evidence":        ownedFact("sensor", "BMP280", "required", "c99"),
		"invented-sensor":         ownedFact("sensor", "SHT31", "required", "c0"),
		"invalid-state":           ownedFact("sensor", "BMP280", "preferred", "c0"),
		"invalid-kind":            ownedFact("none", "BMP280", "required", "c0"),
		"no-evidence":             ownedFact("sensor", "BMP280", "required"),
		"unclear-definite":        {"kind": "unclear", "detail": "Which?", "state": "required", "evidence": []string{"c0"}},
		"null-detail":             {"kind": "other", "detail": nil, "state": "required", "evidence": []string{"c0"}},
		"numeric-value-invented":  {"kind": "number", "choice": "q0/pullup_ohms", "state": "required", "context": []string{}, "number": 4700},
		"owner-override":          {"kind": "number", "choice": "q0/pullup_ohms", "state": "required", "context": []string{}, "sources": []int{1}},
		"old-quantity-array":      {"kind": "feature", "value": "usb", "state": "required", "evidence": []string{"c0"}, "quantities": []int{0}},
	} {
		t.Run(name, func(t *testing.T) {
			raw := ownedRaw(t, fact)
			if err := checkReferencedSchemaJSON(schema, raw); err == nil {
				t.Fatal("invalid input fits schema")
			}
			if _, err := CompileOwnedEvidenceIntent(prompt, raw); err == nil {
				t.Fatal("invalid input compiled")
			}
		})
	}
	for _, raw := range []string{
		`{"version":"4-owned-evidence-experimental","facts":null}`,
		`{"version":"4-owned-evidence-experimental","version":"4-owned-evidence-experimental","facts":[]}`,
		`{"version":"3-indexed-quantities-experimental","facts":[]}`,
		`{"version":"4-owned-evidence-experimental","facts":[],"source_table":[]}`,
		`{"version":"4-owned-evidence-experimental","facts":[{"kind":"number","choice":"q0/pullup_ohms","state":"required","context":null}]}`,
	} {
		if _, err := CompileOwnedEvidenceIntent(prompt, []byte(raw)); err == nil {
			t.Fatalf("invalid envelope accepted: %s", raw)
		}
	}
}

func TestOwnedEvidenceSchemaAndCompilerDimensionMatrix(t *testing.T) {
	prompt := "Use BMP280. 2.2k pull-ups, 400 kHz I2C, 100 pF capacitance, 3.2 to 3.4 V supply, 1 A capacity and 10 to 35 C ambient. Preserve them."
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, q := range request.Quantities {
		for _, field := range intentNumericFields {
			for _, state := range []string{"required", "not_required", "forbidden", "uncertain"} {
				count++
				raw := ownedRaw(t, ownedNumber(fmt.Sprintf("q%d/%s", q.ID, field), state, "c2"))
				_, eligible := q.Fields[field]
				schemaErr := checkReferencedSchemaJSON(schema, raw)
				compiled, compileErr := CompileOwnedEvidenceIntent(prompt, raw)
				if (schemaErr == nil) != eligible || (compileErr == nil) != eligible {
					t.Fatalf("dimension/schema/decoder disagreement for q%d/%s: %v %v", q.ID, field, schemaErr, compileErr)
				}
				if eligible && (!reflect.DeepEqual(compiled.Facts[0].Quantities, []int{q.ID}) || !reflect.DeepEqual(compiled.Facts[0].Sources, []int{q.ClauseID, 2})) {
					t.Fatalf("missing immutable owner: %+v", compiled)
				}
			}
		}
	}
	t.Logf("checked %d quantity/field/state combinations", count)
}

func TestOwnedEvidenceDoesNotCertifySemanticMeaning(t *testing.T) {
	prompt := "Please make a wired pressure monitor."
	raw := ownedRaw(t, ownedFact("measurement", "pressure", "required", "c0"), ownedFact("feature", "wireless_operation", "required", "c0"))
	ownedAccepts(t, prompt, ownedFact("feature", "wireless_operation", "required", "c0"))
	d, err := DecodeOwnedEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" {
		t.Fatalf("semantic counterexample unexpectedly disappeared: %+v %v", d, err)
	}
	// This is an intentionally WRONG extraction, not a passing language case.
	// New structural references cannot establish the positive wireless meaning.
}

func TestOwnedEvidenceRejectsHistoricalWireBytes(t *testing.T) {
	for _, id := range []string{"useful-03", "refuse-05"} {
		file := filepath.Join("..", "..", "specs", "board-family-v2", "indexed-evaluation-04", "batch", id, "journal", "selection", "selection.json")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var s struct {
			Prompt string          `json:"original_request"`
			Raw    json.RawMessage `json:"raw_intent"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		var original ReferencedIntent
		if err := json.Unmarshal(s.Raw, &original); err != nil || original.Version != ReferenceIntentVersion || len(original.Facts) == 0 {
			t.Fatalf("test did not read a real historical v3 extraction: %v", err)
		}
		if _, err := CompileOwnedEvidenceIntent(s.Prompt, s.Raw); err == nil {
			t.Fatal("historical v3 response silently migrated into v4")
		}
	}
}
