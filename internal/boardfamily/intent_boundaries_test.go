package boardfamily

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// These are synthetic compiler/grammar tests, not model accuracy measurements.
func boundaryFixture(t testing.TB, prompt string) (SemanticBoundaryRequest, map[string]any) {
	t.Helper()
	_, f := addressedFixture(t, prompt)
	f["version"] = SemanticBoundaryVersion
	input, err := PrepareSemanticBoundaryRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	for id, state := range input.RequiredState {
		f["mentions"].(map[string]any)[id] = []any{map[string]any{"state": state, "context": []string{}}}
	}
	return input, f
}

func checkBoundary(t testing.TB, prompt string, f any, disposition string) Decision {
	t.Helper()
	raw := addressedJSON(t, f)
	before := bytes.Clone(raw)
	schema, err := SemanticBoundarySchema(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkGroundedSchema(t, schema, raw); err != nil {
		t.Fatal("schema", err)
	}
	d, err := DecodeSemanticBoundaryIntent(prompt, raw)
	if err != nil || d.Disposition != disposition {
		t.Fatalf("decision=%+v error=%v", d, err)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw mutated")
	}
	return d
}

func TestSemanticBoundaryRecognizesOnlyCompleteControls(t *testing.T) {
	for _, clause := range []string{
		"Use the normal reviewed operating limits.",
		"Use the reviewed default power and ambient limits.",
		"The reviewed default supply and ambient limits are acceptable.",
		"Please retain reviewed electrical defaults.",
		"Apply the reviewed ambient limits!",
		"Keep all stated requirements unchanged.",
		"Please keep the profile and all explicit electrical values unchanged.",
		"Keep that profile and all those values unchanged.",
		"Profile and all stated electrical values are to remain unchanged.",
	} {
		t.Run(clause, func(t *testing.T) {
			prompt := "Use SHT31 fast. " + clause
			input, f := boundaryFixture(t, prompt)
			if len(input.Controls) != 1 || input.Controls[0].ClauseID != 1 {
				t.Fatal("missing control", input.Controls)
			}
			setAddressedState(t, input.SourceAddressedRequest, f, 0, "sensor", "SHT31", "requested")
			setAddressedState(t, input.SourceAddressedRequest, f, 0, "profile", "fast", "requested")
			d := checkBoundary(t, prompt, f, "supported")
			if d.Configuration.Family != FamilySHT31 || d.Configuration.Profile != "fast" {
				t.Fatal("control changed selection", d)
			}
		})
	}
}

func TestSemanticBoundaryDoesNotSwallowCompoundOrUnsupportedWording(t *testing.T) {
	for _, clause := range []string{
		"Do not use the reviewed electrical defaults.",
		"Never use reviewed operating limits.",
		"If possible, use the reviewed operating limits.",
		"Use the reviewed defaults unless they prevent USB power.",
		"Use the reviewed default power and ambient limits and add wireless telemetry.",
		"Use the reviewed default electrical limits with reverse-polarity protection.",
		"Use reviewed operating limits of 5 V.",
		"Use reviewed operating limits with 0.1 C guaranteed accuracy.",
		"Use the reviewed defaults, but never the standard profile.",
		"Use reviewed default GPIO load limits.",
		"Use reviewed heater defaults.",
		"Use manufacturer electrical defaults.",
		"Use the reviewed default layout.",
		"Use factory operating limits.",
		"For context, use the reviewed electrical defaults.",
		"The label must say 'Use the reviewed operating limits'.",
		"\"Use the reviewed operating limits\".",
		"Keep all stated values unchanged except the supply voltage.",
		"Keep all stated values unchanged and guarantee certification.",
		"Don't keep all stated values unchanged.",
	} {
		t.Run(clause, func(t *testing.T) {
			input, err := PrepareSemanticBoundaryRequest(clause)
			if err != nil || len(input.Residuals) == 0 {
				t.Fatal("compound clause swallowed", input.Controls, err)
			}
			_, f := boundaryFixture(t, clause)
			f["additional"].(map[string]any)["c0"] = []any{map[string]any{
				"kind": "other", "state": "requested", "context": []string{}, "span": input.Residuals[len(input.Residuals)-1].ID,
			}}
			for _, q := range input.Source.Quantities {
				f["quantities"].(map[string]any)["q"+strconv.Itoa(q.ID)] = []any{map[string]any{
					"kind": "other", "state": "requested", "context": []string{},
				}}
			}
			// A remaining requirement still traverses the unknown-requirement gate.
			checkBoundary(t, clause, f, "unsupported")
		})
	}
}

func TestSemanticBoundaryDefaultsNeverOverwriteExplicitValues(t *testing.T) {
	for _, tc := range []struct {
		name, prompt, want, reason string
		pf                         float64
	}{
		{"valid", "Use SHT31 fast with 90 pF bus capacitance. Use the reviewed default electrical and ambient limits.", "supported", "", 90},
		{"too-large", "Use SHT31 standard with 100 pF bus capacitance. Profile and all stated electrical values are to remain unchanged.", "unsupported", "70", 0},
		{"wrong-supply", "Use SHT31 standard with a 5 V supply. Use the reviewed electrical defaults.", "unsupported", "3.2 and 3.4 v", 0},
		{"unknown", "Use SHT31 fast. Use the reviewed electrical defaults. I require galvanic isolation.", "unsupported", "galvanic isolation", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, f := boundaryFixture(t, tc.prompt)
			for _, m := range input.Mentions {
				setAddressedState(t, input.SourceAddressedRequest, f, m.ClauseID, m.Kind, m.Value, "requested")
			}
			switch tc.name {
			case "valid", "too-large":
				f["quantities"] = map[string]any{"q0": []any{groundedNumber("total_bus_capacitance_pf", "requested")}}
			case "wrong-supply":
				f["quantities"] = map[string]any{"q0": []any{groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")}}
			case "unknown":
				f["additional"].(map[string]any)["c2"] = []any{map[string]any{"kind": "other", "state": "requested", "context": []string{}, "span": input.Residuals[len(input.Residuals)-1].ID}}
			}
			d := checkBoundary(t, tc.prompt, f, tc.want)
			if tc.reason != "" && !strings.Contains(strings.ToLower(d.Message), tc.reason) {
				t.Fatal("actual constraint explanation missing", d.Message)
			}
			if strings.Contains(d.Message, "additional requirement: Profile") || strings.Contains(d.Message, "additional requirement: Use the reviewed") {
				t.Fatal("control masks engineering constraint", d.Message)
			}
			if tc.want == "supported" && d.Configuration.TotalBusCapacitancePF != tc.pf {
				t.Fatal("explicit value replaced by a default", d)
			}
		})
	}
}

func TestSemanticBoundaryKnownCorpusControlsSyntheticOnly(t *testing.T) {
	// Prompts are the unchanged tracked corpus; responses here are handcrafted.
	// This tests representability/admission, NEVER revised live case scores.
	raw, err := os.ReadFile("../../specs/board-family-v2/typed-evaluation-02/cases-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct{ ID, Prompt string }
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	tested := 0
	for _, tc := range corpus.Cases {
		if !member(tc.ID, "useful-03", "useful-05", "refuse-02") {
			continue
		}
		tested++
		t.Run(tc.ID, func(t *testing.T) {
			input, f := boundaryFixture(t, tc.Prompt)
			if len(input.Controls) != 1 {
				t.Fatal("expected one complete control clause", input.Controls)
			}
			for _, m := range input.Mentions {
				state := "requested"
				if fixed, ok := input.RequiredState[m.ID]; ok {
					state = fixed
				}
				setAddressedState(t, input.SourceAddressedRequest, f, m.ClauseID, m.Kind, m.Value, state)
			}
			fields := []string{"pullup_ohms", "clock_hz", "total_bus_capacitance_pf"}
			if tc.ID == "useful-05" {
				fields = []string{"total_bus_capacitance_pf"}
			}
			for i, field := range fields {
				f["quantities"].(map[string]any)["q"+strconv.Itoa(i)] = []any{groundedNumber(field, "requested")}
			}
			want := "supported"
			if tc.ID == "refuse-02" {
				want = "unsupported"
			}
			d := checkBoundary(t, tc.Prompt, f, want)
			if tc.ID == "refuse-02" && !strings.Contains(d.Message, "70") {
				t.Fatal("engineering limit masked by no-substitution control", d)
			}
			if tc.ID == "useful-03" && (d.Configuration.Family != Family || d.Configuration.Profile != "low_current") {
				t.Fatal("wrong low-current configuration", d)
			}
			if tc.ID == "useful-05" && (d.Configuration.Family != FamilySHT31 || d.Configuration.Profile != "fast") {
				t.Fatal("wrong SHT31 fast configuration", d)
			}
		})
	}
	if tested != 3 {
		t.Fatal("missing known corpus cases", tested)
	}
}

func TestSemanticBoundaryNegationContrasts(t *testing.T) {
	for _, tc := range []struct {
		clause, state string
		count         int
	}{
		{"I don't need pressure measurement.", "unnecessary_but_allowed", 1},
		{"I don’t need wireless telemetry.", "unnecessary_but_allowed", 1},
		{"Humidity and wireless telemetry are not needed.", "unnecessary_but_allowed", 2},
		{"I do not need heater operation.", "unnecessary_but_allowed", 1},
		{"Do not run the heater at any time.", "must_not_occur", 1},
		{"Please never enable its heater.", "must_not_occur", 1},
		{"At startup the heater should stay off.", "must_not_occur", 1},
		{"The heater must remain off.", "must_not_occur", 1},
		{"I only mean reducing pull-up current, not whole-board power or battery operation.", "unnecessary_but_allowed", 2},
		{"I only mean pull up current, not whole board current or battery operation.", "unnecessary_but_allowed", 2},
	} {
		t.Run(tc.clause, func(t *testing.T) {
			prompt := "Use SHT31 fast. " + tc.clause
			input, f := boundaryFixture(t, prompt)
			if len(input.RequiredState) != tc.count {
				t.Fatal("wrong constrained inventory", input.RequiredState)
			}
			for _, state := range input.RequiredState {
				if state != tc.state {
					t.Fatal("wrong state", state)
				}
			}
			setAddressedState(t, input.SourceAddressedRequest, f, 0, "sensor", "SHT31", "requested")
			setAddressedState(t, input.SourceAddressedRequest, f, 0, "profile", "fast", "requested")
			checkBoundary(t, prompt, f, "supported")
			for id := range input.RequiredState {
				for _, wrong := range []string{"requested", "context_only", "unresolved_choice", "must_not_occur", "unnecessary_but_allowed"} {
					if wrong == tc.state {
						continue
					}
					f["mentions"].(map[string]any)[id] = []any{map[string]any{"state": wrong, "context": []string{}}}
					raw := addressedJSON(t, f)
					schema, _ := SemanticBoundarySchema(prompt)
					if checkGroundedSchema(t, schema, raw) == nil {
						t.Fatal("schema permits wrong state", id, wrong)
					}
					before := bytes.Clone(raw)
					if d, err := DecodeSemanticBoundaryIntent(prompt, raw); err == nil || d.Configuration != nil || d.Disposition != "clarify" {
						t.Fatal("wrong state accepted or repaired", id, wrong, d, err)
					}
					if !bytes.Equal(before, raw) {
						t.Fatal("failed evidence repaired")
					}
				}
				f["mentions"].(map[string]any)[id] = []any{map[string]any{"state": tc.state, "context": []string{}}}
			}
		})
	}
}

func TestSemanticBoundaryDoesNotExtrapolateAmbiguousNegation(t *testing.T) {
	for _, clause := range []string{
		"I don't need pressure measurement unless temperature is unavailable.",
		"I don't need to forbid pressure measurement.",
		"I don't need pressure measurement to stop.",
		"I do not need heater operation disabled.",
		"I don't need wireless telemetry but do need Bluetooth.",
		"Do not run the heater until startup completes.",
		"Do not run the heater, then enable it later.",
		"The heater should stay off unless condensation occurs.",
		"At startup the heater should stay off, then turn on.",
		"The heater must not remain off.",
		"The manual says 'the heater must remain off'.",
		"Pressure is forbidden.",
	} {
		t.Run(clause, func(t *testing.T) {
			input, err := PrepareSemanticBoundaryRequest(clause)
			if err != nil || len(input.RequiredState) != 0 {
				t.Fatal("unjustified polarity inference", input.RequiredState, err)
			}
		})
	}
}

func TestSemanticBoundaryTemporalOffDoesNotCancelLaterOn(t *testing.T) {
	prompt := "Use SHT31. At startup the heater should stay off. Once normal readings begin, energize its heater between the humidity samples."
	input, f := boundaryFixture(t, prompt)
	setAddressedState(t, input.SourceAddressedRequest, f, 0, "sensor", "SHT31", "requested")
	setAddressedState(t, input.SourceAddressedRequest, f, 2, "feature", "heater_operation", "requested")
	d := checkBoundary(t, prompt, f, "unsupported")
	if !strings.Contains(strings.ToLower(d.Message), "heater") {
		t.Fatal("later heater demand lost", d.Message)
	}
	for id, state := range input.RequiredState {
		f["mentions"].(map[string]any)[id] = []any{map[string]any{"state": state, "context": []string{"c2"}}}
		if d, err := DecodeSemanticBoundaryIntent(prompt, addressedJSON(t, f)); err == nil || d.Configuration != nil {
			t.Fatal("local prohibition gained later-clause scope", d, err)
		}
	}
}

func TestSemanticBoundaryCrossClauseScopeCuesFallBack(t *testing.T) {
	for _, prompt := range []string{
		"Here is an example. Use the reviewed operating limits. I don't need pressure measurement.",
		"The following is prohibited. Use the reviewed operating limits.",
		"The next sentence is hypothetical. Do not run the heater.",
		"Here is what I do not want. Use the reviewed operating limits.",
		"If condensation is detected; the heater must remain off.",
		"The heater must remain off. Except when de-icing is required.",
		"I rejected this: 'Use the reviewed operating limits.'",
		"Use SHT31 (the heater must remain off).",
		"I don't need pressure measurement. Instead I require barometric sensing.",
	} {
		t.Run(prompt, func(t *testing.T) {
			input, err := PrepareSemanticBoundaryRequest(prompt)
			if err != nil || len(input.Controls) != 0 || len(input.RequiredState) != 0 {
				t.Fatal("cross-clause scope forced local semantics", input, err)
			}
		})
	}
}

func TestSemanticBoundaryControlAndEnvelopeTampering(t *testing.T) {
	prompt := "Use SHT31 fast. Use the reviewed electrical defaults."
	for _, change := range []string{"invented-other", "missing-control", "null-control", "extra-top-field", "wrong-version", "missing-mentions", "extra-mention", "missing-quantities", "extra-entry-field"} {
		t.Run(change, func(t *testing.T) {
			input, f := boundaryFixture(t, prompt)
			setAddressedState(t, input.SourceAddressedRequest, f, 0, "sensor", "SHT31", "requested")
			switch change {
			case "invented-other":
				f["additional"].(map[string]any)["c1"] = []any{map[string]any{"kind": "other", "detail": "Use defaults", "state": "requested", "context": []string{}}}
			case "missing-control":
				delete(f["additional"].(map[string]any), "c1")
			case "null-control":
				f["additional"].(map[string]any)["c1"] = nil
			case "extra-top-field":
				f["controls"] = []any{}
			case "wrong-version":
				f["version"] = SourceAddressedVersion
			case "missing-mentions":
				delete(f, "mentions")
			case "extra-mention":
				f["mentions"].(map[string]any)["m99"] = []any{}
			case "missing-quantities":
				delete(f, "quantities")
			case "extra-entry-field":
				f["mentions"].(map[string]any)["m0"].([]any)[0].(map[string]any)["new"] = true
			}
			raw := addressedJSON(t, f)
			schema, _ := SemanticBoundarySchema(prompt)
			if checkGroundedSchema(t, schema, raw) == nil {
				t.Fatal("schema accepted tampering")
			}
			if d, err := DecodeSemanticBoundaryIntent(prompt, raw); err == nil || d.Configuration != nil {
				t.Fatal("decoder accepted tampering", d, err)
			}
		})
	}
}

func TestSemanticBoundarySourceAndHistoricalIsolation(t *testing.T) {
	prompt := "Use SHT31 fast. Use the reviewed electrical defaults. I don’t need pressure measurement."
	input, f := boundaryFixture(t, prompt)
	v9, err := PrepareSourceAddressedRequest(prompt)
	if err != nil || !reflect.DeepEqual(input.SourceAddressedRequest, v9) {
		t.Fatal("source table changed", err)
	}
	var joined strings.Builder
	for _, c := range input.Source.Clauses {
		joined.WriteString(c.Text)
	}
	if joined.String() != prompt {
		t.Fatal("source bytes changed")
	}
	original, _ := json.Marshal(input)
	input.Controls[0].Kind = "grant-wireless"
	for id := range input.RequiredState {
		input.RequiredState[id] = "requested"
	}
	again, _ := PrepareSemanticBoundaryRequest(prompt)
	encoded, _ := json.Marshal(again)
	if !bytes.Equal(original, encoded) {
		t.Fatal("mutable export poisoned internal state")
	}
	setAddressedState(t, input.SourceAddressedRequest, f, 0, "sensor", "SHT31", "requested")
	checkBoundary(t, prompt, f, "supported")
	if _, err := DecodeSourceAddressedEvidenceIntent(prompt, addressedJSON(t, f)); err == nil {
		t.Fatal("historical decoder accepted new identity")
	}
	f["version"] = SourceAddressedVersion
	if _, err := DecodeSemanticBoundaryIntent(prompt, addressedJSON(t, f)); err == nil {
		t.Fatal("new decoder reinterpreted historical evidence")
	}
	// v9 still permits the known wrong-state counterexample; v10 must not alter it.
	for id := range again.RequiredState {
		f["mentions"].(map[string]any)[id] = []any{map[string]any{"state": "must_not_occur", "context": []string{}}}
	}
	if _, err := CompileSourceAddressedEvidence(prompt, addressedJSON(t, f)); err != nil {
		t.Fatal("historical compiler changed", err)
	}
	if !strings.Contains(semanticBoundaryContext(), SemanticBoundaryRequestRevision) {
		t.Fatal("request identity missing")
	}
}

func FuzzSemanticBoundaryDecoder(f *testing.F) {
	prompt := "Use SHT31 fast. Use the reviewed electrical defaults. I don't need pressure measurement."
	input, seed := boundaryFixture(f, prompt)
	setAddressedState(f, input.SourceAddressedRequest, seed, 0, "sensor", "SHT31", "requested")
	setAddressedState(f, input.SourceAddressedRequest, seed, 0, "profile", "fast", "requested")
	f.Add(addressedJSON(f, seed))
	f.Add([]byte(`{"version":"10-semantic-boundaries-offline","mentions":null,"additional":{},"quantities":{}}`))
	f.Add([]byte(`{"version":"10-semantic-boundaries-offline","version":"9-source-addressed-evidence-experimental"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		decision, err := DecodeSemanticBoundaryIntent(prompt, raw)
		if !bytes.Equal(before, raw) {
			t.Fatal("raw mutation")
		}
		if err != nil && (decision.Disposition != "clarify" || decision.Configuration != nil) {
			t.Fatal("invalid evidence produced a configuration", decision, err)
		}
		if err == nil {
			schema, schemaErr := SemanticBoundarySchema(prompt)
			if schemaErr != nil || checkGroundedSchema(t, schema, raw) != nil {
				t.Fatal("decoder accepted bytes outside the schema", schemaErr)
			}
		}
	})
}
