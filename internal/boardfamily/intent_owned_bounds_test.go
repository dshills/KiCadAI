package boardfamily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// These are offline schema-size checks against the documented Structured
// Outputs limits, not a claim that the provider has accepted this new schema.
// https://developers.openai.com/api/docs/guides/structured-outputs
func ownedSchemaSize(t testing.TB, prompt string) (schemaBytes, requestBytes, enums int) {
	t.Helper()
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	schemaBytes = len(encoded)
	var root map[string]any
	if err := json.Unmarshal(encoded, &root); err != nil {
		t.Fatal(err)
	}
	properties, characters := 0, 0
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		if values, ok := node["enum"].([]any); ok {
			enums += len(values)
			length := 0
			for _, value := range values {
				length += len(value.(string)) // This protocol uses ASCII string enums only.
			}
			characters += length
			if len(values) > 250 && length > 15000 {
				t.Fatal("single large enum exceeds documented string limit")
			}
		}
		if props, ok := node["properties"].(map[string]any); ok {
			properties += len(props)
			required, ok := node["required"].([]any)
			if !ok || len(required) != len(props) || node["additionalProperties"] != false {
				t.Fatal("object is not closed with every property required")
			}
			for key, value := range props {
				if !slices.Contains(required, any(key)) {
					t.Fatal("missing required property", key)
				}
				characters += len(key)
				walk(value.(map[string]any))
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			walk(items)
		}
		if branches, ok := node["anyOf"].([]any); ok {
			for _, branch := range branches {
				walk(branch.(map[string]any))
			}
		}
	}
	walk(root)
	if enums > 1000 || properties > 5000 || characters > 120000 {
		t.Fatalf("schema exceeds documented limits: enums=%d properties=%d strings=%d", enums, properties, characters)
	}
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return schemaBytes, len(encoded), enums
}

func TestOwnedEvidenceSchemaSizeKnownCorpus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("unexpected regression case inventory")
	}
	for _, c := range corpus.Cases {
		schema, request, enums := ownedSchemaSize(t, c.Prompt)
		t.Logf("%s: schema=%d bytes, prepared source table=%d bytes, total enum values=%d (not a full API request)", c.ID, schema, request, enums)
	}
}

func TestOwnedEvidenceOriginalInputBounds(t *testing.T) {
	// Simultaneous maximum clause and quantity counts, with two dimensional
	// choices per quantity. No original limit is narrowed to shrink the schema.
	prompt := "Use BMP280 or SHT31 with " + strings.Repeat("0 C, 0 C, 0 C, 0 C. ", 32)
	request, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil || len(request.Clauses) != 32 || len(request.Quantities) != 128 || len(request.NumericChoices) != 256 {
		t.Fatalf("full input scope was reduced: clauses=%d quantities=%d choices=%d err=%v", len(request.Clauses), len(request.Quantities), len(request.NumericChoices), err)
	}
	schema, table, enums := ownedSchemaSize(t, prompt)
	t.Logf("simultaneous 32-clause/128-quantity bound: schema=%d bytes, prepared source table=%d bytes, enum values=%d", schema, table, enums)
	for _, p := range []string{strings.Repeat("x", 2000), "  Use BMP280 with .1 nF capacitance.\n保留这些要求。  "} {
		r, err := PrepareOwnedEvidenceRequest(p)
		if err != nil || r.Request != p {
			t.Fatalf("exact request bytes lost: %v", err)
		}
		for _, q := range r.Quantities {
			if p[q.Start:q.End] != q.Text {
				t.Fatal("quantity lost its original UTF-8 byte span")
			}
		}
		ownedSchemaSize(t, p)
	}
	for _, p := range []string{"", " \n ", strings.Repeat("x", 2001), strings.Repeat("A. ", 33), strings.Repeat("0 C, ", 129), string([]byte{0xff})} {
		if _, err := PrepareOwnedEvidenceRequest(p); err == nil {
			t.Fatal("invalid original input bound accepted")
		}
		if _, err := OwnedEvidenceSchema(p); err == nil {
			t.Fatal("schema accepted invalid original input bound")
		}
		if compiled, err := CompileOwnedEvidenceIntent(p, ownedRaw(t)); err == nil || len(compiled.Facts) != 0 {
			t.Fatal("compiler accepted invalid original input bound")
		}
	}
}

func TestOwnedEvidenceOutputBounds(t *testing.T) {
	prompt := "Use BMP280 with 2.2k pull-ups."
	facts := make([]map[string]any, 64)
	for i := range facts {
		facts[i] = ownedFact("sensor", "BMP280", "required", "c0")
	}
	ownedAccepts(t, prompt, facts...)
	schema, err := OwnedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	raw := ownedRaw(t, append(facts, facts[0])...)
	if checkReferencedSchemaJSON(schema, raw) == nil {
		t.Fatal("schema accepted 65 facts")
	}
	for _, raw := range [][]byte{raw, bytes.Repeat([]byte(" "), 65537),
		ownedRaw(t, ownedFact("sensor", "BMP280", "required", "c0", "c0")),
		ownedRaw(t, ownedNumber("q0/pullup_ohms", "required", "c0", "c0")),
		ownedRaw(t, map[string]any{"kind": "other", "detail": "x", "state": "required", "evidence": []string{"q0", "q0", "q0"}}),
	} {
		if compiled, err := CompileOwnedEvidenceIntent(prompt, raw); err == nil || len(compiled.Facts) != 0 {
			t.Fatal("output bound did not fail closed")
		}
	}
}

func FuzzOwnedEvidenceCompiler(f *testing.F) {
	prompt := "Use BMP280 with 2.2k pull-ups. Keep the value unchanged."
	f.Add(prompt, ownedRaw(f, ownedFact("sensor", "BMP280", "required", "c0"), ownedNumber("q0/pullup_ohms", "required", "c1")))
	f.Add(prompt, ownedRaw(f, ownedFact("feature", "usb", "not_required", "q0", "c1")))
	f.Add(prompt, ownedRaw(f, map[string]any{"kind": "other", "detail": "", "state": "required", "evidence": []string{"c0"}}))
	f.Add("Hello", ownedRaw(f))
	f.Add("", []byte("null"))
	f.Fuzz(func(t *testing.T, p string, raw []byte) {
		before := append([]byte(nil), raw...)
		compiled, err := CompileOwnedEvidenceIntent(p, raw)
		if !bytes.Equal(raw, before) {
			t.Fatal("raw input mutated")
		}
		if err != nil {
			if len(compiled.Facts) != 0 {
				t.Fatal("partial facts returned on failure")
			}
			return
		}
		second, err := CompileOwnedEvidenceIntent(p, raw)
		if err != nil || !reflect.DeepEqual(compiled, second) {
			t.Fatal("compiler is not deterministic")
		}
		schema, err := OwnedEvidenceSchema(p)
		if err != nil || checkReferencedSchemaJSON(schema, raw) != nil {
			t.Fatal("compiler accepted schema-invalid bytes", err)
		}
		request, err := PrepareOwnedEvidenceRequest(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range compiled.Facts {
			for _, q := range fact.Quantities {
				if q < 0 || q >= len(request.Quantities) || !slices.Contains(fact.Sources, request.Quantities[q].ClauseID) {
					t.Fatal("compiled quantity lost immutable source ownership")
				}
			}
			if fact.Kind == "number" {
				if len(fact.Quantities) != 1 {
					t.Fatal("number did not bind one occurrence")
				}
				id := fmt.Sprintf("q%d/%s", fact.Quantities[0], fact.Value)
				if !slices.ContainsFunc(request.NumericChoices, func(choice OwnedNumericChoice) bool { return choice.ID == id }) {
					t.Fatal("number invented a dimensional choice")
				}
			}
		}
	})
}
