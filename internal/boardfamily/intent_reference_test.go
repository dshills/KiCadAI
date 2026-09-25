package boardfamily

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func referencedChoice(kind, value, state string, sources ...int) ReferencedFact {
	return ReferencedFact{Kind: kind, Value: value, State: state, Sources: sources, Quantities: []int{}}
}

func referencedNumber(field string, quantity int, sources ...int) ReferencedFact {
	return ReferencedFact{Kind: "number", Value: field, State: "required", Quantities: []int{quantity}, Sources: sources}
}

func referencedWithQuantities(f ReferencedFact, ids ...int) ReferencedFact {
	f.Quantities = ids
	return f
}

func syntheticReferencedIntent(t testing.TB, facts ...ReferencedFact) []byte {
	t.Helper()
	if facts == nil {
		facts = []ReferencedFact{}
	}
	b, err := json.Marshal(ReferencedIntent{Version: ReferenceIntentVersion, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func retainedTypedSelection(t testing.TB, id string) Selection {
	t.Helper()
	path := filepath.Join("../../specs/board-family-v2/typed-evaluation-02/evidence-02", id, "selection.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s Selection
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

var retainedTypedCaseIDs = []string{
	"useful-01", "useful-02", "useful-03", "useful-04", "useful-05",
	"choice-01", "choice-02", "choice-03",
	"refuse-01", "refuse-02", "refuse-03", "refuse-04", "refuse-05", "refuse-06",
}

func TestReferencedIntentPreservesActualHistoricalFailures(t *testing.T) {
	errorsByID := map[string]bool{"useful-02": true, "useful-03": true, "useful-04": true, "choice-02": true, "refuse-01": true, "refuse-02": true}
	for _, id := range retainedTypedCaseIDs {
		t.Run(id, func(t *testing.T) {
			s := retainedTypedSelection(t, id)
			d, err := DecodeIntent(s.OriginalRequest, s.RawIntent)
			if (err != nil) != errorsByID[id] || !reflect.DeepEqual(d, s.Decision) {
				t.Fatalf("historical result changed: decision=%+v error=%v", d, err)
			}
			if d, err := DecodeReferencedIntent(s.OriginalRequest, s.RawIntent); err == nil || d.Configuration != nil {
				t.Fatal("historical provider bytes silently accepted as a successor response")
			}
		})
	}
}

// All facts below are NEW, implementing-agent-authored synthetic inputs to a
// local decoder. Only prompts come from the preserved known-failure corpus.
// These are not corrected provider outputs, live results, or an unseen holdout.
type referencedSyntheticCase struct {
	id, disposition, family, profile, reason string
	capacitance                              float64
	facts                                    []ReferencedFact
}

func referencedSyntheticCases() []referencedSyntheticCase {
	choice, number := referencedChoice, referencedNumber
	return []referencedSyntheticCase{
		{"useful-01", "supported", Family, "standard", "", 200, []ReferencedFact{
			choice("measurement", "pressure", "required", 1), choice("profile", "standard", "required", 2),
		}},
		{"useful-02", "supported", Family, "fast", "", 100, []ReferencedFact{
			choice("sensor", "BMP280", "required", 0), number("pullup_ohms", 0, 0), number("clock_hz", 1, 0), number("total_bus_capacitance_pf", 2, 0),
			choice("measurement", "humidity", "not_required", 1), choice("feature", "wireless_operation", "not_required", 1),
		}},
		{"useful-03", "supported", Family, "low_current", "", 100, []ReferencedFact{
			choice("sensor", "BMP280", "required", 0), choice("profile", "low_current", "required", 0, 1), number("pullup_ohms", 0, 0), number("clock_hz", 1, 0), number("total_bus_capacitance_pf", 2, 0),
			choice("feature", "whole_board_low_power", "not_required", 1), choice("feature", "battery_operation", "not_required", 1),
		}},
		{"useful-04", "supported", FamilySHT31, "standard", "", 70, []ReferencedFact{
			choice("measurement", "temperature", "required", 0), choice("measurement", "humidity", "required", 0), choice("profile", "standard", "required", 1), number("total_bus_capacitance_pf", 0, 1),
			choice("measurement", "pressure", "not_required", 2),
		}},
		{"useful-05", "supported", FamilySHT31, "fast", "", 100, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("profile", "fast", "required", 0), number("total_bus_capacitance_pf", 0, 0), choice("feature", "heater_operation", "forbidden", 1),
		}},
		{"choice-01", "clarify", "", "", "Which measurement", 0, nil},
		{"choice-02", "clarify", "", "", "Which measurement", 0, []ReferencedFact{
			choice("sensor", "BMP280", "uncertain", 0, 1), choice("measurement", "pressure", "uncertain", 0, 1),
			choice("sensor", "SHT31", "uncertain", 0, 1), choice("measurement", "humidity", "uncertain", 0, 1),
		}},
		{"choice-03", "clarify", "", "", "Which I2C profile", 0, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("profile", "standard", "uncertain", 1), choice("profile", "fast", "uncertain", 1),
		}},
		{"refuse-01", "unsupported", "", "", "Neither family combines", 0, []ReferencedFact{
			choice("sensor", "BMP280", "required", 0, 2), choice("sensor", "SHT31", "required", 0, 2),
			choice("measurement", "pressure", "required", 1, 2), choice("measurement", "humidity", "required", 1, 2),
		}},
		{"refuse-02", "unsupported", "", "", "capacitance", 0, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("profile", "standard", "required", 0, 2), number("pullup_ohms", 0, 0, 2), number("clock_hz", 1, 0, 2), number("total_bus_capacitance_pf", 2, 1, 2),
		}},
		{"refuse-03", "unsupported", "", "", "do not match a reviewed profile", 0, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("profile", "low_current", "required", 0), number("pullup_ohms", 0, 0), number("total_bus_capacitance_pf", 1, 1),
		}},
		{"refuse-04", "unsupported", "", "", "heater must remain off", 0, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("feature", "heater_operation", "forbidden", 1), choice("feature", "heater_operation", "required", 2),
		}},
		{"refuse-05", "unsupported", "", "", "USB", 0, []ReferencedFact{
			choice("sensor", "BMP280", "required", 0), referencedWithQuantities(choice("feature", "usb", "required", 0), 0), choice("feature", "wireless_operation", "required", 1),
		}},
		{"refuse-06", "unsupported", "", "", "accuracy requires bench", 0, []ReferencedFact{
			choice("sensor", "SHT31", "required", 0), choice("measurement", "temperature", "required", 0), choice("measurement", "humidity", "required", 0), choice("profile", "standard", "required", 0), referencedWithQuantities(choice("feature", "accuracy_guarantee", "required", 1), 0),
		}},
	}
}

func TestReferencedIntentKnownPromptsWithSyntheticFacts(t *testing.T) {
	for _, tc := range referencedSyntheticCases() {
		t.Run(tc.id, func(t *testing.T) {
			s := retainedTypedSelection(t, tc.id)
			d, err := DecodeReferencedIntent(s.OriginalRequest, syntheticReferencedIntent(t, tc.facts...))
			if err != nil || d.Disposition != tc.disposition || !strings.Contains(d.Message, tc.reason) {
				t.Fatalf("decision=%+v error=%v", d, err)
			}
			if len(d.Clauses) != 1 || d.Clauses[0].Text != s.OriginalRequest {
				t.Fatal("original request was lost")
			}
			if tc.disposition == "supported" {
				want := Config{"1", tc.family, tc.profile, 3.2, 3.4, 1000, 10, 35, tc.capacitance}
				if d.Configuration == nil || *d.Configuration != want {
					t.Fatalf("wrong configuration: %+v", d.Configuration)
				}
				if _, err := Check(*d.Configuration); err != nil {
					t.Fatal(err)
				}
			} else if d.Configuration != nil {
				t.Fatal("non-design decision exposes a configuration")
			}
		})
	}
}

func TestReferencedIntentStrictShapeAndGrounding(t *testing.T) {
	valid := string(syntheticReferencedIntent(t, referencedChoice("sensor", "BMP280", "required", 0)))
	for _, raw := range []string{
		"", "null", valid + valid, strings.Replace(valid, `"facts":`, `"facts":[],"facts":`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":null`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[-1]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[2]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[0,0]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[0.5]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":["0"]`, 1),
		strings.Replace(valid, `"sources":[0]`, `"sources":[1]`, 1), // pronoun without antecedent
		strings.Replace(valid, `"sources":[0]`, `"sources":[0],"quote":"BMP280"`, 1),
		strings.Replace(valid, `"required"`, `"probably"`, 1),
		strings.Replace(valid, `"sensor"`, `"none"`, 1),
		strings.Replace(valid, `"BMP280"`, `"SHT31"`, 1),
		strings.Replace(valid, `"BMP280"`, `""`, 1),
		strings.Replace(valid, ReferenceIntentVersion, "2", 1),
		string(syntheticReferencedIntent(t, referencedNumber("supply_min_v", 0, 1))), // default is not a source quantity
		string(syntheticReferencedIntent(t, ReferencedFact{Kind: "other", Sources: []int{0}, Detail: ""})),
		strings.Repeat(" ", 65537), string([]byte{0xff}),
	} {
		d, err := DecodeReferencedIntent("Use BMP280. Keep that sensor and the default limits.", []byte(raw))
		if err == nil || d.Configuration != nil || d.Disposition != "clarify" {
			t.Fatalf("invalid extraction was admitted: %s; %+v %v", raw, d, err)
		}
	}
	for _, prompt := range []string{"", " \n", strings.Repeat("a", 2001), strings.Repeat("a;", 33), string([]byte{0xff})} {
		if d, err := DecodeReferencedIntent(prompt, []byte(valid)); err == nil || d.Configuration != nil {
			t.Fatal("invalid prompt accepted")
		}
	}
	facts := make([]ReferencedFact, 65)
	for i := range facts {
		facts[i] = referencedChoice("sensor", "BMP280", "required", 0)
	}
	if _, err := DecodeReferencedIntent("Use BMP280.", syntheticReferencedIntent(t, facts...)); err == nil {
		t.Fatal("unbounded fact inventory accepted")
	}
}

func TestReferencedIntentQuantitiesCannotHideInBroadReferences(t *testing.T) {
	sensor := referencedChoice("sensor", "BMP280", "required", 0)
	for _, tc := range []struct {
		prompt, missing string
		facts           []ReferencedFact
	}{
		{"BMP280 with 400 kHz I2C clock and 80 pF total bus capacitance.", "80 pF", []ReferencedFact{sensor, referencedNumber("clock_hz", 0, 0)}},
		{"BMP280 with 100 kHz I2C clock and 100 pF total bus capacitance.", "100 pF", []ReferencedFact{sensor, referencedNumber("clock_hz", 0, 0)}},
		{"BMP280 with 100 pF total bus capacitance. Use a 400 kHz I2C clock.", "400 kHz", []ReferencedFact{sensor, referencedNumber("total_bus_capacitance_pf", 0, 0)}},
		{"BMP280 with 3.2–3.4 V supply.", "supply_min_v", []ReferencedFact{sensor, referencedNumber("supply_max_v", 0, 0)}},
	} {
		d, err := DecodeReferencedIntent(tc.prompt, syntheticReferencedIntent(t, tc.facts...))
		if err != nil || d.Disposition != "clarify" || d.Configuration != nil || !strings.Contains(d.Message, tc.missing) {
			t.Fatalf("omitted numeric requirement: %+v %v", d, err)
		}
	}
	for _, tc := range []struct {
		prompt string
		fact   ReferencedFact
	}{
		{"BMP280 with 400 kHz I2C clock.", referencedNumber("clock_hz", 1, 0)},
		{"BMP280 with 3.4–3.2 V supply.", referencedNumber("supply_max_v", 0, 0)},
		{"BMP280 with 100 Hz sampling.", referencedNumber("clock_hz", 0, 0)},
		{"BMP280 with a 2 A GPIO load.", referencedNumber("supply_capacity_ma", 0, 0)},
		{"BMP280 with 100 pF total capacitance. Keep the default limits.", referencedNumber("total_bus_capacitance_pf", 0, 1)},
	} {
		if d, err := DecodeReferencedIntent(tc.prompt, syntheticReferencedIntent(t, sensor, tc.fact)); err == nil || d.Configuration != nil {
			t.Fatalf("invented value, role, endpoint or source admitted: %+v %v", d, err)
		}
	}
	prompt := "BMP280 with a 3.25–3.35 V supply, 2 A source capacity, 15–30 C ambient, and 80 pF bus capacitance."
	raw := syntheticReferencedIntent(t, sensor, referencedNumber("supply_min_v", 0, 0), referencedNumber("supply_max_v", 0, 0),
		referencedNumber("supply_capacity_ma", 1, 0), referencedNumber("ambient_min_c", 2, 0), referencedNumber("ambient_max_c", 2, 0), referencedNumber("total_bus_capacitance_pf", 3, 0))
	d, err := DecodeReferencedIntent(prompt, raw)
	want := Config{"1", Family, "standard", 3.25, 3.35, 2000, 15, 30, 80}
	if err != nil || d.Configuration == nil || *d.Configuration != want {
		t.Fatalf("explicit units/ranges changed: %+v %v", d, err)
	}
}

func TestReferencedIntentReferenceOrderingAndStates(t *testing.T) {
	prompt := "Please use SHT31. I have chosen that sensor."
	a, err := DecodeReferencedIntent(prompt, syntheticReferencedIntent(t, referencedChoice("sensor", "SHT31", "required", 0, 1)))
	b, reverseErr := DecodeReferencedIntent(prompt, syntheticReferencedIntent(t, referencedChoice("sensor", "SHT31", "required", 1, 0)))
	if err != nil || reverseErr != nil || !reflect.DeepEqual(a, b) || a.Disposition != "supported" {
		t.Fatal("source reference order changed the decision")
	}
	for _, state := range []string{"not_required", "forbidden", "uncertain", "required"} {
		want := "supported"
		if state == "forbidden" {
			want = "unsupported"
		} else if state == "uncertain" {
			want = "clarify"
		}
		prompt := "Use SHT31. Humidity is " + state + "."
		raw := syntheticReferencedIntent(t, referencedChoice("sensor", "SHT31", "required", 0), referencedChoice("measurement", "humidity", state, 1))
		if d, err := DecodeReferencedIntent(prompt, raw); err != nil || d.Disposition != want {
			t.Fatalf("state %s collapsed: %+v %v", state, d, err)
		}
	}
}

func TestReferencedIntentKnownConflictPrecedesMissingQuantity(t *testing.T) {
	prompt := "Use SHT31 with 100 pF total bus capacitance. Enable the heater."
	raw := syntheticReferencedIntent(t, referencedChoice("sensor", "SHT31", "required", 0))
	d, err := DecodeReferencedIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" || d.Configuration != nil || !strings.Contains(d.Message, "heater must remain off") {
		t.Fatalf("missing quantity hid the known heater conflict: %+v %v", d, err)
	}
}

func TestReferencedIntentReferencesAreNotSemanticCertification(t *testing.T) {
	// Known semantic counterexample, intentionally NOT counted as a successful
	// user outcome: valid IDs still cannot prove that "indoor project" means
	// custom geometry. Do not promote this candidate on structural tests alone.
	s := retainedTypedSelection(t, "choice-01")
	raw := syntheticReferencedIntent(t, referencedChoice("feature", "custom_geometry", "required", 0))
	d, err := DecodeReferencedIntent(s.OriginalRequest, raw)
	if err != nil || d.Disposition != "unsupported" || d.Configuration != nil {
		t.Fatal("update the documented semantic limitation and its assessment")
	}
}

func FuzzDecodeReferencedIntentNoInvalidConfiguration(f *testing.F) {
	f.Add("Please use BMP280.", string(syntheticReferencedIntent(f, referencedChoice("sensor", "BMP280", "required", 0))))
	f.Add("Please make a sensor board.", string(syntheticReferencedIntent(f)))
	f.Fuzz(func(t *testing.T, prompt, raw string) {
		d, err := DecodeReferencedIntent(prompt, []byte(raw))
		if d.Configuration != nil {
			if err != nil || d.Disposition != "supported" {
				t.Fatal("unvalidated decision exposes configuration")
			}
			if _, err := Check(*d.Configuration); err != nil {
				t.Fatal(err)
			}
		}
		if len(d.Clauses) != 1 || d.Clauses[0].Text != prompt {
			t.Fatal("source request was lost")
		}
	})
}
