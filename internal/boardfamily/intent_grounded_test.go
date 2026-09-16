package boardfamily

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func groundedRaw(t testing.TB, requirements []map[string]any, quantities map[string][]map[string]any) []byte {
	t.Helper()
	if requirements == nil {
		requirements = []map[string]any{}
	}
	if quantities == nil {
		quantities = map[string][]map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"version": GroundedEvidenceVersion, "requirements": requirements, "quantities": quantities})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func groundedNumber(field, state string) map[string]any {
	return map[string]any{"kind": "number", "field": field, "state": state, "context": []string{}}
}

// Expand only local, acyclic definitions for the existing test-only checker.
// This is not a provider schema validator or server-acceptance claim.
func checkGroundedSchema(t testing.TB, schema map[string]any, raw []byte) error {
	t.Helper()
	defs := schema["$defs"].(map[string]any)
	var expand func(any, int) any
	expand = func(value any, depth int) any {
		if depth > 24 {
			t.Fatal("cyclic or excessive schema reference depth")
		}
		switch node := value.(type) {
		case map[string]any:
			if r, ok := node["$ref"].(string); ok {
				if len(node) != 1 || !strings.HasPrefix(r, "#/$defs/") {
					t.Fatal("unexpected schema reference")
				}
				target, ok := defs[strings.TrimPrefix(r, "#/$defs/")]
				if !ok {
					t.Fatal("missing schema definition")
				}
				return expand(target, depth+1)
			}
			copy := map[string]any{}
			for key, child := range node {
				if key != "$defs" {
					copy[key] = expand(child, depth+1)
				}
			}
			return copy
		case []any:
			copy := make([]any, len(node))
			for i, child := range node {
				copy[i] = expand(child, depth+1)
			}
			return copy
		default:
			return value
		}
	}
	return checkReferencedSchemaJSON(expand(schema, 0).(map[string]any), raw)
}

func checkGrounded(t testing.TB, prompt string, raw []byte, disposition string) Decision {
	t.Helper()
	schema, err := GroundedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkGroundedSchema(t, schema, raw); err != nil {
		t.Fatal("synthetic input violates schema:", err)
	}
	before := bytes.Clone(raw)
	d, err := DecodeGroundedEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != disposition {
		t.Fatalf("got %s, want %s: %v (%s)", d.Disposition, disposition, err, d.Message)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("raw extraction mutated")
	}
	return d
}

func TestGroundedFiveConfigurationsAndOrdinaryWording(t *testing.T) {
	// Hand-authored synthetic responses test representation and admission only.
	for _, pair := range [][2]string{{"BMP280", "standard"}, {"BMP280", "fast"}, {"BMP280", "low_current"}, {"SHT31", "standard"}, {"SHT31", "fast"}} {
		for _, wording := range []string{"Use ", "Please use ", "Could you use ", "I'd like ", "For this project, use "} {
			prompt := wording + pair[0] + " with the " + pair[1] + " profile and a wired connection."
			t.Run(prompt, func(t *testing.T) {
				requirements := []map[string]any{ownedFact("sensor", pair[0], "requested", "c0"), ownedFact("profile", pair[1], "requested", "c0"), ownedFact("connection", "wired", "requested", "c0")}
				d := checkGrounded(t, prompt, groundedRaw(t, requirements, nil), "supported")
				baseline, err := DecodeConnectionEvidenceIntent(prompt, connectionRaw(t, ownedFact("sensor", pair[0], "required", "c0"), ownedFact("profile", pair[1], "required", "c0"), ownedFact("connection", "wired", "required", "c0")))
				if err != nil || !reflect.DeepEqual(d, baseline) {
					t.Fatal("engineering admission changed", err)
				}
			})
		}
	}
}

func TestGroundedExclusionAndProhibitionRemainDistinct(t *testing.T) {
	for _, tc := range []struct{ phrase, state, want string }{
		{"Humidity is not needed.", "unnecessary_but_allowed", "not_required"},
		{"Humidity sensing is prohibited.", "must_not_occur", "forbidden"},
	} {
		prompt := "Use BMP280 standard. " + tc.phrase
		raw := groundedRaw(t, []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0"), ownedFact("measurement", "humidity", tc.state, "c1")}, nil)
		checkGrounded(t, prompt, raw, "supported")
		compiled, err := CompileGroundedEvidence(prompt, raw)
		if err != nil {
			t.Fatal(err)
		}
		var out struct{ Facts []struct{ State string } }
		if err := json.Unmarshal(compiled, &out); err != nil || len(out.Facts) != 3 || out.Facts[2].State != tc.want {
			t.Fatal("state distinction lost", err)
		}
	}
}

func TestGroundedQuantityInventoryRejectsMissingAndFeatureConsumption(t *testing.T) {
	prompt := "Use SHT31 low_current with 10k pull-ups and 100 pF total bus capacitance."
	reqs := []map[string]any{ownedFact("sensor", "SHT31", "requested", "c0"), ownedFact("profile", "low_current", "requested", "c0")}
	correct := map[string][]map[string]any{"q0": {groundedNumber("pullup_ohms", "requested")}, "q1": {groundedNumber("total_bus_capacitance_pf", "requested")}}
	checkGrounded(t, prompt, groundedRaw(t, reqs, correct), "unsupported")
	for name, quantities := range map[string]map[string][]map[string]any{
		"omitted-capacitance": {"q0": correct["q0"]},
		"empty-capacitance":   {"q0": correct["q0"], "q1": {}},
		"wrong-quantity-id":   {"q0": correct["q0"], "q2": correct["q1"]},
		"invented-crc":        {"q0": {{"kind": "feature", "value": "skip_crc", "state": "requested", "context": []string{}}}, "q1": correct["q1"]},
		"wrong-dimension":     {"q0": {groundedNumber("clock_hz", "requested")}, "q1": correct["q1"]},
	} {
		t.Run(name, func(t *testing.T) {
			raw := groundedRaw(t, reqs, quantities)
			schema, err := GroundedEvidenceSchema(prompt)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkGroundedSchema(t, schema, raw); err == nil {
				t.Fatal("schema admitted invalid quantity inventory")
			}
			if d, err := DecodeGroundedEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
				t.Fatal("decoder admitted invalid quantity inventory")
			}
		})
	}
	// Unlike a quantity slot, an ordinary feature cannot consume qN at all.
	bad := append(append([]map[string]any{}, reqs...), ownedFact("feature", "skip_crc", "requested", "q0"))
	if _, err := DecodeGroundedEvidenceIntent(prompt, groundedRaw(t, bad, correct)); err == nil {
		t.Fatal("feature consumed quantity")
	}
}

func TestGroundedQuantityOwnerRangesAndRepeatedOccurrences(t *testing.T) {
	prompt := "Use BMP280 standard with 3.2 to 3.4 V supply. Use 100 pF total bus loading. Repeat 100 pF total bus loading."
	reqs := []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0")}
	quantities := map[string][]map[string]any{
		"q0": {groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")},
		"q1": {groundedNumber("total_bus_capacitance_pf", "requested")},
		"q2": {groundedNumber("total_bus_capacitance_pf", "requested")},
	}
	raw := groundedRaw(t, reqs, quantities)
	checkGrounded(t, prompt, raw, "supported")
	compiled, err := CompileGroundedEvidence(prompt, raw)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Facts []json.RawMessage }
	if err := json.Unmarshal(compiled, &out); err != nil {
		t.Fatal(err)
	}
	owned, err := json.Marshal(map[string]any{"version": OwnedEvidenceVersion, "facts": out.Facts})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := CompileOwnedEvidenceIntent(prompt, owned)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs.Facts) != 6 || !reflect.DeepEqual(refs.Facts[4].Sources, []int{1}) || !reflect.DeepEqual(refs.Facts[5].Quantities, []int{2}) {
		t.Fatal("range/owner/occurrence loss", refs)
	}
}

func TestGroundedOtherQuantitiesAndTemporalConstraints(t *testing.T) {
	for _, tc := range []struct{ prompt, detail, state, kind, disposition string }{
		{"Use SHT31 standard and guarantee 0.1 degrees C accuracy.", "guarantee 0.1 degrees C accuracy", "requested", "other", "unsupported"},
		{"Use SHT31 standard; a 50 mA external load is required.", "50 mA external load", "requested", "other", "unsupported"},
		{"Use SHT31 standard; I do not need a 50 mA external load.", "50 mA external load", "unnecessary_but_allowed", "other", "supported"},
		{"Use SHT31 standard; 50 mA could describe the supply or a GPIO load; I have not chosen.", "Does 50 mA describe supply capacity or a GPIO load?", "unresolved_choice", "unclear", "clarify"},
	} {
		reqs := []map[string]any{ownedFact("sensor", "SHT31", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0")}
		q := map[string][]map[string]any{"q0": {{"kind": tc.kind, "detail": tc.detail, "state": tc.state, "context": []string{}}}}
		checkGrounded(t, tc.prompt, groundedRaw(t, reqs, q), tc.disposition)
	}
	prompt := "Use SHT31 standard. Keep the heater off at startup. Run it afterward."
	reqs := []map[string]any{ownedFact("sensor", "SHT31", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0"), ownedFact("feature", "heater_operation", "must_not_occur", "c1"), ownedFact("feature", "heater_operation", "requested", "c1", "c2")}
	checkGrounded(t, prompt, groundedRaw(t, reqs, nil), "unsupported")
}

func TestGroundedFrozenCorpusSourceAndRequiredQuantityProperties(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("corpus changed")
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			prior, priorSource, err := prepareDirectGenerateRequest(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(prior)
			if err != nil {
				t.Fatal(err)
			}
			request, source, err := prepareGroundedGenerateRequest(c.Prompt)
			if err != nil || request.Prompt != prior.Prompt || !reflect.DeepEqual(source, priorSource) || !request.DirectSourceJSON || request.MaxOutputTokens != 1600 || request.Attempt != 1 {
				t.Fatal("source or bounds changed", err)
			}
			qSchema := request.OutputSchema["properties"].(map[string]any)["quantities"].(map[string]any)
			properties := qSchema["properties"].(map[string]any)
			if len(properties) != len(source.Quantities) {
				t.Fatal("quantity inventory mismatch")
			}
			for _, q := range source.Quantities {
				if _, ok := properties["q"+strconv.Itoa(q.ID)]; !ok {
					t.Fatal("missing quantity property")
				}
			}
			again, _, err := prepareDirectGenerateRequest(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(again)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("prior contract changed", err)
			}
		})
	}
}

func TestGroundedRejectsHistoricalBytesAndInvalidEnvelopes(t *testing.T) {
	prompt := "Use BMP280 standard."
	valid := groundedRaw(t, nil, nil)
	for _, raw := range [][]byte{
		directRaw(t), connectionRaw(t), ownedRaw(t),
		bytes.Replace(valid, []byte(`"requirements":[]`), []byte(`"requirements":null`), 1),
		bytes.Replace(valid, []byte(`"quantities":{}`), []byte(`"quantities":null`), 1),
		[]byte(strings.Repeat("x", 65537)),
		[]byte(`{"version":"7-partitioned-evidence-experimental","version":"7-partitioned-evidence-experimental","requirements":[],"quantities":{}}`),
	} {
		if d, err := DecodeGroundedEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
			t.Fatal("invalid/historical bytes admitted", string(raw[:min(100, len(raw))]))
		}
	}
}

func TestGroundedDoesNotClaimSemanticCorrectness(t *testing.T) {
	// Deliberately false, schema-valid SYNTHETIC assertions remain a known gap.
	// The new inventory prevents omission, not wrong meaning or false refusals.
	prompt := "Use BMP280 standard with wired data."
	raw := groundedRaw(t, []map[string]any{ownedFact("sensor", "BMP280", "requested", "c0"), ownedFact("profile", "standard", "requested", "c0"), ownedFact("connection", "wireless", "requested", "c0")}, nil)
	checkGrounded(t, prompt, raw, "unsupported")
}

func FuzzGroundedEvidenceDoesNotMutateOrPanic(f *testing.F) {
	f.Add("Use BMP280 standard.", string(groundedRaw(f, nil, nil)))
	f.Add("Use SHT31 standard with 100 pF.", string(groundedRaw(f, nil, map[string][]map[string]any{"q0": {groundedNumber("total_bus_capacitance_pf", "requested")}})))
	f.Fuzz(func(t *testing.T, prompt, raw string) {
		if len(prompt) > 2500 || len(raw) > 70000 {
			t.Skip()
		}
		b := []byte(raw)
		before := bytes.Clone(b)
		_, _ = DecodeGroundedEvidenceIntent(prompt, b)
		if !bytes.Equal(before, b) {
			t.Fatal("raw mutated")
		}
	})
}
