package boardfamily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func addressedFixture(t testing.TB, prompt string) (SourceAddressedRequest, map[string]any) {
	t.Helper()
	input, err := PrepareSourceAddressedRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	mentions, additional := map[string]any{}, map[string]any{}
	for _, m := range input.Mentions {
		mentions[m.ID] = []any{map[string]any{"state": "context_only", "context": []string{}}}
	}
	for _, c := range input.Source.Clauses {
		additional[fmt.Sprint("c", c.ID)] = []any{}
	}
	return input, map[string]any{"version": SourceAddressedVersion, "mentions": mentions, "additional": additional, "quantities": map[string]any{}}
}

func setAddressedState(t testing.TB, input SourceAddressedRequest, fixture map[string]any, clause int, kind, value string, states ...string) {
	t.Helper()
	for _, m := range input.Mentions {
		if m.ClauseID != clause || m.Kind != kind || m.Value != value {
			continue
		}
		entries := []any{}
		for _, state := range states {
			entries = append(entries, map[string]any{"state": state, "context": []string{}})
		}
		fixture["mentions"].(map[string]any)[m.ID] = entries
		return
	}
	t.Fatalf("missing source slot: c%d %s/%s", clause, kind, value)
}

func addressedJSON(t testing.TB, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func checkAddressed(t testing.TB, prompt string, fixture any, disposition string) Decision {
	t.Helper()
	raw := addressedJSON(t, fixture)
	before := bytes.Clone(raw)
	schema, err := SourceAddressedEvidenceSchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkGroundedSchema(t, schema, raw); err != nil {
		t.Fatal("schema", err)
	}
	d, err := DecodeSourceAddressedEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != disposition {
		t.Fatalf("decision=%+v error=%v", d, err)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw data mutated")
	}
	return d
}

func TestSourceAddressedInventoryIsSourceOwnedAndStable(t *testing.T) {
	prompt := "Use BMP280 powered directly by 5 V USB. I need wireless telemetry without an adapter."
	first, err := PrepareSourceAddressedRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceAddressedMention{{"m0", "sensor", "BMP280", 0}, {"m1", "feature", "usb", 0}, {"m2", "connection", "wireless", 1}}
	if !reflect.DeepEqual(first.Mentions, want) {
		t.Fatal("invented or misplaced label", first.Mentions)
	}
	for range 20 {
		again, err := PrepareSourceAddressedRequest(prompt)
		if err != nil || !reflect.DeepEqual(first, again) {
			t.Fatal("nondeterministic source table", err)
		}
	}
	old, err := PrepareReferencedRequest(prompt)
	if err != nil || !reflect.DeepEqual(old, first.Source) || first.Source.Request != prompt {
		t.Fatal("original source changed", err)
	}
	// Public mutable export values do not influence regeneration by the decoder.
	first.Mentions[0].Value = "SHT31"
	first.QuantityRoles["q0"] = []string{"ambient_min_c"}
	again, _ := PrepareSourceAddressedRequest(prompt)
	if !reflect.DeepEqual(again.Mentions, want) || member("ambient_min_c", again.QuantityRoles["q0"]...) {
		t.Fatal("caller mutated source truth")
	}
}

func TestSourceAddressedPreservesLowCurrentAndHeaterNegation(t *testing.T) {
	for _, tc := range []struct {
		name, prompt, family, profile string
	}{
		{"low-current", "Please use BMP280 with the low_current profile and 100 pF. I only mean pull-up current, not whole-board power or battery operation.", "esp32_bmp280_v1", "low_current"},
		{"heater-off", "Please use the wired SHT31 fast profile with 100 pF. Never run the heater.", "esp32_sht31_v1", "fast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, f := addressedFixture(t, tc.prompt)
			for _, m := range input.Mentions {
				state := "requested"
				if m.Kind == "feature" {
					state = "unnecessary_but_allowed"
					if m.Value == "heater_operation" {
						state = "must_not_occur"
					}
				}
				setAddressedState(t, input, f, m.ClauseID, m.Kind, m.Value, state)
				if m.Kind == "profile" && m.ClauseID != 0 {
					t.Fatal("invented profile slot for exclusion clause")
				}
			}
			f["quantities"] = map[string]any{"q0": []any{groundedNumber("total_bus_capacitance_pf", "requested")}}
			d := checkAddressed(t, tc.prompt, f, "supported")
			if d.Configuration.Family != tc.family || d.Configuration.Profile != tc.profile {
				t.Fatal("incorrect configuration", d)
			}
		})
	}
}

func TestSourceAddressedCannotInventLabelsOrAnchors(t *testing.T) {
	prompt := "Use SHT31 fast. Never run the heater."
	for _, mutation := range []string{"new-slot", "new-label", "new-anchor", "missing-slot", "renamed-slot", "missing-clause"} {
		t.Run(mutation, func(t *testing.T) {
			_, f := addressedFixture(t, prompt)
			mentions := f["mentions"].(map[string]any)
			entry := mentions["m0"].([]any)[0].(map[string]any)
			switch mutation {
			case "new-slot":
				mentions["m99"] = []any{entry}
			case "new-label":
				entry["value"] = "standard"
			case "new-anchor":
				entry["anchor"] = "c1"
			case "missing-slot":
				delete(mentions, "m0")
			case "renamed-slot":
				mentions["m99"] = mentions["m0"]
				delete(mentions, "m0")
			case "missing-clause":
				delete(f["additional"].(map[string]any), "c1")
			}
			raw := addressedJSON(t, f)
			schema, err := SourceAddressedEvidenceSchema(prompt)
			if err != nil || checkGroundedSchema(t, schema, raw) == nil {
				t.Fatal("schema accepted altered source shape", err)
			}
			if d, err := DecodeSourceAddressedEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
				t.Fatal("decoder accepted altered source shape", d, err)
			}
		})
	}
}

func TestSourceAddressedPreservesAdditionalRequirements(t *testing.T) {
	prompt := "Use BMP280 with USB power and wireless telemetry. No external adapter is allowed."
	input, f := addressedFixture(t, prompt)
	for _, m := range input.Mentions {
		setAddressedState(t, input, f, m.ClauseID, m.Kind, m.Value, "requested")
	}
	f["additional"].(map[string]any)["c1"] = []any{map[string]any{
		"kind": "other", "detail": "No external adapter is allowed", "state": "must_not_occur", "context": []string{"c0"},
	}}
	d := checkAddressed(t, prompt, f, "unsupported")
	if !strings.Contains(d.Message, "USB") || !strings.Contains(d.Message, "Wireless") {
		t.Fatal("lost substantive refusal reasons", d.Message)
	}
	compiled, err := CompileSourceAddressedEvidence(prompt, addressedJSON(t, f))
	if err != nil || !bytes.Contains(compiled, []byte("No external adapter is allowed")) {
		t.Fatal("unknown constraint was lost", err)
	}
}

func TestSourceAddressedPreservesTemporalAndUnresolvedStates(t *testing.T) {
	for _, prompt := range []string{
		"Use SHT31. Keep the heater off at startup then enable the heater later.",
		"Use SHT31. Keep the heater off at startup. Enable the heater later.",
	} {
		input, f := addressedFixture(t, prompt)
		setAddressedState(t, input, f, 0, "sensor", "SHT31", "requested")
		if len(input.Source.Clauses) == 2 {
			setAddressedState(t, input, f, 1, "feature", "heater_operation", "must_not_occur", "requested")
		} else {
			setAddressedState(t, input, f, 1, "feature", "heater_operation", "must_not_occur")
			setAddressedState(t, input, f, 2, "feature", "heater_operation", "requested")
		}
		checkAddressed(t, prompt, f, "unsupported")
	}
	prompt := "Either BMP280 or SHT31 would work; I have not chosen."
	input, f := addressedFixture(t, prompt)
	for _, m := range input.Mentions {
		setAddressedState(t, input, f, m.ClauseID, m.Kind, m.Value, "unresolved_choice")
		f["mentions"].(map[string]any)[m.ID].([]any)[0].(map[string]any)["context"] = []string{"c1"}
	}
	d := checkAddressed(t, prompt, f, "clarify")
	if !strings.Contains(d.Message, "Which measurement") {
		t.Fatal("not a targeted family question", d)
	}
}

func TestSourceAddressedDoesNotConfusePrecisionAndOperatingRange(t *testing.T) {
	prompt := "Use SHT31 operating at 15 to 30 C with accuracy within 0.2 C."
	input, f := addressedFixture(t, prompt)
	setAddressedState(t, input, f, 0, "sensor", "SHT31", "requested")
	setAddressedState(t, input, f, 0, "feature", "accuracy_guarantee", "requested")
	f["quantities"] = map[string]any{
		"q0": []any{groundedNumber("ambient_min_c", "requested"), groundedNumber("ambient_max_c", "requested")},
		"q1": []any{map[string]any{"kind": "other", "detail": "assembled accuracy within 0.2 C", "state": "requested", "context": []string{}}},
	}
	checkAddressed(t, prompt, f, "unsupported")
	f["quantities"].(map[string]any)["q1"] = []any{groundedNumber("ambient_max_c", "requested")}
	schema, _ := SourceAddressedEvidenceSchema(prompt)
	if checkGroundedSchema(t, schema, addressedJSON(t, f)) == nil {
		t.Fatal("schema permits tolerance as ambient bound")
	}
	if _, err := CompileSourceAddressedEvidence(prompt, addressedJSON(t, f)); err == nil {
		t.Fatal("decoder permits tolerance as ambient bound")
	}
}

func TestSourceAddressedRejectsInvalidStateContextAndEnvelope(t *testing.T) {
	prompt := "Use BMP280."
	for _, mutation := range []string{"null-mentions", "null-additional", "null-quantities", "null-clause", "empty-state-list", "duplicate-state", "mixed-context-only", "invalid-state", "null-context", "bad-context", "duplicate-context", "old-version", "extra-root", "numeric-additional", "unclear-requested"} {
		t.Run(mutation, func(t *testing.T) {
			_, f := addressedFixture(t, prompt)
			mentions := f["mentions"].(map[string]any)
			entry := mentions["m0"].([]any)[0].(map[string]any)
			switch mutation {
			case "null-mentions":
				f["mentions"] = nil
			case "null-additional":
				f["additional"] = nil
			case "null-quantities":
				f["quantities"] = nil
			case "null-clause":
				f["additional"].(map[string]any)["c0"] = nil
			case "empty-state-list":
				mentions["m0"] = []any{}
			case "duplicate-state":
				mentions["m0"] = []any{entry, entry}
			case "mixed-context-only":
				mentions["m0"] = []any{entry, map[string]any{"state": "requested", "context": []string{}}}
			case "invalid-state":
				entry["state"] = "required"
			case "null-context":
				entry["context"] = nil
			case "bad-context":
				entry["context"] = []string{"q0"}
			case "duplicate-context":
				entry["context"] = []string{"c0", "c0"}
			case "old-version":
				f["version"] = SourceEligibleVersion
			case "extra-root":
				f["requirements"] = []any{}
			case "numeric-additional":
				f["additional"].(map[string]any)["c0"] = []any{map[string]any{"kind": "number", "detail": "default 3.3 V", "state": "requested", "context": []string{}}}
			case "unclear-requested":
				f["additional"].(map[string]any)["c0"] = []any{map[string]any{"kind": "unclear", "detail": "Which output?", "state": "requested", "context": []string{}}}
			}
			raw := addressedJSON(t, f)
			if d, err := DecodeSourceAddressedEvidenceIntent(prompt, raw); err == nil || d.Configuration != nil {
				t.Fatal("accepted malformed extraction", d, err)
			}
		})
	}
	for _, raw := range [][]byte{[]byte(`{"version":"x","version":"y"}`), []byte(`[]`), []byte(`null`), []byte(`{}`), bytes.Repeat([]byte(" "), 65537)} {
		if _, err := CompileSourceAddressedEvidence(prompt, raw); err == nil {
			t.Fatal("accepted invalid JSON or bounds")
		}
	}
}

func TestSourceAddressedRetainsSemanticCounterexamples(t *testing.T) {
	// These are counterexamples, not passing semantic evaluations. A shape-safe
	// model can still misclassify a prohibition or omit an unfamiliar requirement.
	prompt := "Use SHT31. Never enable its heater."
	input, f := addressedFixture(t, prompt)
	setAddressedState(t, input, f, 0, "sensor", "SHT31", "requested")
	setAddressedState(t, input, f, 1, "feature", "heater_operation", "requested") // Deliberately wrong polarity.
	checkAddressed(t, prompt, f, "unsupported")
	prompt = "Use BMP280. Make the finished board waterproof."
	input, f = addressedFixture(t, prompt)
	setAddressedState(t, input, f, 0, "sensor", "BMP280", "requested")
	checkAddressed(t, prompt, f, "supported") // Deliberately omitted waterproofing.
	f["additional"].(map[string]any)["c1"] = []any{map[string]any{"kind": "other", "detail": "finished board must be waterproof", "state": "requested", "context": []string{}}}
	checkAddressed(t, prompt, f, "unsupported")
	// Binding the profile identity to c0 cannot prove that its state was not
	// incorrectly borrowed from the heater prohibition in c1.
	prompt = "Use SHT31 with the fast profile and 100 pF. Never run the heater."
	input, f = addressedFixture(t, prompt)
	setAddressedState(t, input, f, 0, "sensor", "SHT31", "requested")
	setAddressedState(t, input, f, 0, "profile", "fast", "requested", "must_not_occur")
	setAddressedState(t, input, f, 1, "feature", "heater_operation", "must_not_occur")
	for _, m := range input.Mentions {
		if m.Kind == "profile" {
			f["mentions"].(map[string]any)[m.ID].([]any)[1].(map[string]any)["context"] = []string{"c1"}
		}
	}
	f["quantities"] = map[string]any{"q0": []any{groundedNumber("total_bus_capacitance_pf", "requested")}}
	checkAddressed(t, prompt, f, "unsupported") // Deliberate cross-clause state error.
}

func TestSourceAddressedPreservesExistingPromptLevelHeaterGuard(t *testing.T) {
	// The existing engineering admission guard catches this exact affirmative
	// wording even when the synthetic extraction incorrectly says context_only.
	// That guard is preserved, not a claim of general omission detection.
	prompt := "Use SHT31. Enable the heater."
	input, f := addressedFixture(t, prompt)
	setAddressedState(t, input, f, 0, "sensor", "SHT31", "requested")
	d := checkAddressed(t, prompt, f, "unsupported")
	if !strings.Contains(d.Message, "heater must remain off") {
		t.Fatal("lost existing prompt-level guard", d)
	}
	setAddressedState(t, input, f, 1, "feature", "heater_operation", "requested")
	checkAddressed(t, prompt, f, "unsupported")
}

func TestSourceAddressedDenseQuantitiesAndOfflineOnlyContract(t *testing.T) {
	prompt := strings.Repeat("3.3V ", 128)
	input, f := addressedFixture(t, prompt)
	if len(input.Source.Quantities) != 128 {
		t.Fatal("quantity occurrences lost")
	}
	for _, q := range input.Source.Quantities {
		f["quantities"].(map[string]any)[fmt.Sprint("q", q.ID)] = []any{groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")}
	}
	checkAddressed(t, prompt, f, "clarify")
	for _, invalid := range []string{"", " \n\t", strings.Repeat("x", 2001), string([]byte{0xff}), strings.Repeat("3.3V ", 129), strings.Repeat("BMP280. ", 33)} {
		if _, err := SourceAddressedEvidenceContract(invalid); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	contract, err := SourceAddressedEvidenceContract("Use BMP280.")
	if err != nil || contract["export_dispatches_request"] != false || contract["live_authorization_granted"] != false || contract["stage"] != "experimental-integration-no-live-acceptance" {
		t.Fatal("offline boundary", contract, err)
	}
}

func TestSourceAddressedRetainsAggregateAssertionLimit(t *testing.T) {
	// Three distinct concepts in each of 22 clauses are 66 real assertions.
	// Do not silently keep only the first 64 to make an extraction admissible.
	prompt := strings.Repeat("BMP280 SHT31 pressure. ", 22)
	input, f := addressedFixture(t, prompt)
	if len(input.Mentions) != 66 {
		t.Fatal("source inventory unexpectedly truncated", len(input.Mentions))
	}
	for _, m := range input.Mentions {
		setAddressedState(t, input, f, m.ClauseID, m.Kind, m.Value, "requested")
	}
	before := addressedJSON(t, f)
	if _, err := CompileSourceAddressedEvidence(prompt, before); err == nil || !strings.Contains(err.Error(), "64 nonnumeric assertions") {
		t.Fatal("aggregate assertion bound bypassed", err)
	}
	if !bytes.Equal(before, addressedJSON(t, f)) {
		t.Fatal("over-bound fixture modified")
	}
}

func FuzzSourceAddressedNeverMutatesOrPanics(f *testing.F) {
	input, fixture := addressedFixture(f, "Use BMP280.")
	setAddressedState(f, input, fixture, 0, "sensor", "BMP280", "requested")
	f.Add("Use BMP280.", addressedJSON(f, fixture))
	f.Add("", []byte("{}"))
	f.Fuzz(func(t *testing.T, prompt string, raw []byte) {
		if len(prompt) > 2100 || len(raw) > 66000 {
			t.Skip()
		}
		before := bytes.Clone(raw)
		_, _ = DecodeSourceAddressedEvidenceIntent(prompt, raw)
		if !bytes.Equal(raw, before) {
			t.Fatal("raw bytes changed")
		}
	})
}
