package boardfamily

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

func choiceFact(kind, value, state, quote string) RequirementFact {
	return RequirementFact{Kind: kind, Value: value, State: state, Quote: quote}
}

func numberFact(field string, value float64, quote string) RequirementFact {
	return RequirementFact{Kind: "number", Value: field, Number: &value, Quote: quote}
}

// These are synthetic extractions authored for decision-logic tests, NOT
// responses from a model or evidence of unseen natural-language reliability.
func syntheticIntent(t testing.TB, prompt string, facts ...RequirementFact) []byte {
	t.Helper()
	clauses, err := requestClauses(prompt)
	if err != nil {
		t.Fatal(err)
	}
	intent := RequirementIntent{Version: "2"}
	used := make([]bool, len(facts))
	for _, clause := range clauses {
		c := IntentClause{ID: clause.ID}
		for i, f := range facts {
			if !used[i] && strings.Contains(clause.Text, f.Quote) {
				c.Facts = append(c.Facts, f)
				used[i] = true
			}
		}
		if len(c.Facts) == 0 {
			c.Facts = []RequirementFact{{Kind: "none"}}
		}
		intent.Clauses = append(intent.Clauses, c)
	}
	for i, ok := range used {
		if !ok {
			t.Fatalf("fact %d has no source clause: %+v", i, facts[i])
		}
	}
	b, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkIntent(t testing.TB, prompt string, raw []byte, want string) Decision {
	t.Helper()
	d, err := DecodeIntent(prompt, raw)
	if err != nil || d.Disposition != want {
		t.Fatalf("want %s; decision=%+v error=%v", want, d, err)
	}
	if len(d.Clauses) != 1 || d.Clauses[0].Text != prompt || d.Message == "" {
		t.Fatalf("application lost original request or explanation: %+v", d)
	}
	if want == "supported" {
		if d.Configuration == nil {
			t.Fatal("supported decision lacks configuration")
		}
		if _, err := Check(*d.Configuration); err != nil {
			t.Fatal(err)
		}
	} else if d.Configuration != nil {
		t.Fatal("non-design decision exposes a configuration")
	}
	return d
}

func TestIntentRequestSegmentation(t *testing.T) {
	for _, prompt := range []string{
		"\n  Please use SHT31-DIS-B2.5KS. Thanks!\n  ",
		"Use BMP280; supply 3.2–3.4 V, 10.0 to 35.0 C?",
		"\n\nBonjour — BMP280!\r\nPlease.\t",
		strings.Repeat("x", 2000),
		strings.Repeat("a;", 32),
	} {
		request, clauses, err := prepareIntentRequest(prompt)
		if err != nil {
			t.Fatal(err)
		}
		var joined string
		for i, c := range clauses {
			if i != c.ID {
				t.Fatal("unstable clause ID")
			}
			joined += c.Text
		}
		if joined != prompt {
			t.Fatalf("lost original bytes: %q != %q", joined, prompt)
		}
		var payload struct {
			Request string `json:"request"`
		}
		if err := json.Unmarshal([]byte(request), &payload); err != nil || payload.Request != prompt {
			t.Fatalf("request wrapper: %+v %v", payload, err)
		}
	}
	clauses, _ := requestClauses("SHT31-DIS-B2.5KS at 3.3 V. Thanks!")
	if len(clauses) != 2 {
		t.Fatalf("split decimal or part number: %+v", clauses)
	}
	for _, prompt := range []string{"", " \n", strings.Repeat("a", 2001), strings.Repeat("a;", 33), string([]byte{0xff})} {
		if _, _, err := prepareIntentRequest(prompt); err == nil {
			t.Fatalf("accepted invalid prompt %q", prompt)
		}
	}
}

func TestIntentOrdinaryWordingMetamorphic(t *testing.T) {
	// Same 25 wording/configuration combinations as usability-01, where the
	// earlier closed-vocabulary gate admitted only the five canonical inputs.
	for _, family := range Catalog() {
		sensor := "BMP280"
		if family.ID == FamilySHT31 {
			sensor = "SHT31"
		}
		for _, p := range family.Profiles {
			base := sensor + " with the " + p.ID + " profile"
			for i, prompt := range []string{"Use " + base + ".", "Please use " + base + ".", "Use " + base + ". Thank you.", "I would like " + base + ".", "Could you use " + base + "?"} {
				t.Run(fmt.Sprintf("%s/%s/%d", sensor, p.ID, i), func(t *testing.T) {
					raw := syntheticIntent(t, prompt, choiceFact("sensor", sensor, "required", sensor), choiceFact("profile", p.ID, "required", p.ID+" profile"))
					d := checkIntent(t, prompt, raw, "supported")
					want := Config{"1", family.ID, p.ID, 3.2, 3.4, 1000, 10, 35, p.MaxBusPF}
					if *d.Configuration != want {
						t.Fatalf("changed configuration: %+v", d.Configuration)
					}
				})
			}
		}
	}
}

func TestIntentRequirementDecisions(t *testing.T) {
	sensor := func(s string) RequirementFact { return choiceFact("sensor", s, "required", s) }
	profile := func(s string) RequirementFact { return choiceFact("profile", s, "required", s) }
	feature := func(s, state, quote string) RequirementFact { return choiceFact("feature", s, state, quote) }
	for _, tc := range []struct {
		name, prompt, want, profile string
		facts                       []RequirementFact
	}{
		{"pressure paraphrase", "Hello! A barometric monitor for my desk would be great, using your normal supported setup. Much appreciated.", "supported", "standard", []RequirementFact{choiceFact("measurement", "pressure", "required", "barometric monitor")}},
		{"humidity paraphrase", "Could you put together a humidity-sensing controller for indoor use? The reviewed default setup is fine.", "supported", "standard", []RequirementFact{choiceFact("measurement", "humidity", "required", "humidity-sensing")}},
		{"temperature paraphrase", "I'd like a temperature monitor, please. Start with the reviewed electrical defaults.", "supported", "standard", []RequirementFact{choiceFact("measurement", "temperature", "required", "temperature monitor")}},
		{"generic", "Please make a sensor board for my desk.", "clarify", "", nil},
		{"sensor choice", "Either BMP280 or SHT31 would work, but I have not chosen.", "clarify", "", []RequirementFact{choiceFact("sensor", "BMP280", "uncertain", "Either BMP280"), choiceFact("sensor", "SHT31", "uncertain", "SHT31 would work, but I have not chosen")}},
		{"profile choice", "Please use SHT31. Standard or fast? I haven't decided.", "clarify", "", []RequirementFact{sensor("SHT31"), choiceFact("profile", "standard", "uncertain", "Standard"), choiceFact("profile", "fast", "uncertain", "fast")}},
		{"both required", "Please use BMP280 plus SHT31 on the same board.", "unsupported", "", []RequirementFact{sensor("BMP280"), sensor("SHT31")}},
		{"excluded not forbidden", "Please use BMP280; humidity is not required.", "supported", "standard", []RequirementFact{sensor("BMP280"), choiceFact("measurement", "humidity", "not_required", "humidity is not required")}},
		{"forbidden sensor", "Please use SHT31, but SHT31 must not be present.", "unsupported", "", []RequirementFact{sensor("SHT31"), choiceFact("sensor", "SHT31", "forbidden", "SHT31 must not be present")}},
		{"forbidden measurement", "Please use SHT31; humidity measurement must not be present.", "unsupported", "", []RequirementFact{sensor("SHT31"), choiceFact("measurement", "humidity", "forbidden", "humidity measurement must not be present")}},
		{"two required profiles", "Please use BMP280 with standard and fast profiles simultaneously.", "unsupported", "", []RequirementFact{sensor("BMP280"), profile("standard"), profile("fast")}},
		{"same profile forbidden", "Please use BMP280 with fast profile; fast is forbidden.", "unsupported", "", []RequirementFact{sensor("BMP280"), profile("fast"), choiceFact("profile", "fast", "forbidden", "fast is forbidden")}},
		{"SHT low current", "Please use SHT31 with low_current.", "unsupported", "", []RequirementFact{sensor("SHT31"), profile("low_current")}},
		{"clock inferred", "Could you arrange BMP280 with a 400 kilohertz I2C bus and 2.2 kilohm pull-ups?", "supported", "fast", []RequirementFact{sensor("BMP280"), numberFact("clock_hz", 400000, "400 kilohertz I2C bus"), numberFact("pullup_ohms", 2200, "2.2 kilohm pull-ups")}},
		{"SHT capacitance", "Please use SHT31 with standard profile and 100 pF bus capacitance.", "unsupported", "", []RequirementFact{sensor("SHT31"), profile("standard"), numberFact("total_bus_capacitance_pf", 100, "100 pF bus capacitance")}},
		{"off then on", "Please use SHT31; keep heater off at startup but run the heater during measurements.", "unsupported", "", []RequirementFact{sensor("SHT31"), feature("heater_operation", "not_required", "heater off at startup"), feature("heater_operation", "required", "run the heater during measurements")}},
		{"heater forbidden", "Please use SHT31. Do not run the heater.", "supported", "standard", []RequirementFact{sensor("SHT31"), feature("heater_operation", "forbidden", "Do not run the heater")}},
		{"no heater need", "Please use SHT31; there is no need to run the heater.", "supported", "standard", []RequirementFact{sensor("SHT31"), feature("heater_operation", "not_required", "no need to run the heater")}},
		{"other", "Please use BMP280 with an LCD screen.", "unsupported", "", []RequirementFact{sensor("BMP280"), {Kind: "other", Quote: "LCD screen", Detail: "An LCD screen is required."}}},
		{"targeted ambiguity", "Please use BMP280 optimized for low power.", "clarify", "", []RequirementFact{sensor("BMP280"), {Kind: "unclear", Quote: "low power", Detail: "Do you mean I2C pull-up current or whole-board power?"}}},
		{"one bound", "Please use BMP280 with minimum supply 3.2 V.", "clarify", "", []RequirementFact{sensor("BMP280"), numberFact("supply_min_v", 3.2, "minimum supply 3.2 V")}},
		{"duplicate number", "Please use BMP280 with 100 pF and 200 pF bus capacitance simultaneously.", "unsupported", "", []RequirementFact{sensor("BMP280"), numberFact("total_bus_capacitance_pf", 100, "100 pF"), numberFact("total_bus_capacitance_pf", 200, "200 pF")}},
		{"unsupported clock", "Please use BMP280 with a 200 kHz I2C clock.", "unsupported", "", []RequirementFact{sensor("BMP280"), numberFact("clock_hz", 200000, "200 kHz I2C clock")}},
		{"negative capacitance", "Please use BMP280 with -50 pF bus capacitance.", "unsupported", "", []RequirementFact{sensor("BMP280"), numberFact("total_bus_capacitance_pf", -50, "-50 pF bus capacitance")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := checkIntent(t, tc.prompt, syntheticIntent(t, tc.prompt, tc.facts...), tc.want)
			if d.Configuration != nil && d.Configuration.Profile != tc.profile {
				t.Fatalf("wrong profile: %+v", d.Configuration)
			}
		})
	}
}

func TestIntentFeatureStates(t *testing.T) {
	for name := range intentFeatureReasons {
		for _, state := range []string{"required", "forbidden", "not_required", "uncertain"} {
			t.Run(name+"/"+state, func(t *testing.T) {
				prompt := "Please use BMP280. The extra feature is " + name + "."
				want := "supported"
				if state == "required" {
					want = "unsupported"
				} else if state == "uncertain" {
					want = "clarify"
				}
				checkIntent(t, prompt, syntheticIntent(t, prompt, choiceFact("sensor", "BMP280", "required", "BMP280"), choiceFact("feature", name, state, name)), want)
			})
		}
	}
}

func TestIntentExplicitOperatingLimits(t *testing.T) {
	prompt := "Could you use BMP280 at a 3.25–3.35 V supply with 2 A source capacity, 15–30 C ambient, and 80 pF bus capacitance?"
	raw := syntheticIntent(t, prompt, choiceFact("sensor", "BMP280", "required", "BMP280"),
		numberFact("supply_min_v", 3.25, "3.25–3.35 V supply"), numberFact("supply_max_v", 3.35, "3.25–3.35 V supply"),
		numberFact("supply_capacity_ma", 2000, "2 A source capacity"), numberFact("ambient_min_c", 15, "15–30 C ambient"), numberFact("ambient_max_c", 30, "15–30 C ambient"), numberFact("total_bus_capacitance_pf", 80, "80 pF bus capacitance"))
	d := checkIntent(t, prompt, raw, "supported")
	want := Config{"1", Family, "standard", 3.25, 3.35, 2000, 15, 30, 80}
	if *d.Configuration != want {
		t.Fatalf("explicit limits changed: %+v", d.Configuration)
	}
}

func TestIntentIndependentOmissionChecks(t *testing.T) {
	for _, prompt := range []string{
		"Please use SHT31, and run the heater continuously.",
		"Please use SHT31 powered directly by 5 V USB.",
		"Use SHT31 with 100 pF total bus capacitance.",
		"Use SHT31 with low_current.",
		"Use SHT31 on a 60 x 40 mm board.",
		"Use SHT31 on a four-layer board.",
	} {
		t.Run(prompt, func(t *testing.T) {
			// Deliberately omit the contradiction, retaining just the sensor.
			checkIntent(t, prompt, syntheticIntent(t, prompt, choiceFact("sensor", "SHT31", "required", "SHT31")), "unsupported")
		})
	}
	// Fully recognized independent source limits cannot silently be defaulted.
	prompt := "Use BMP280 with the fast profile."
	checkIntent(t, prompt, syntheticIntent(t, prompt, choiceFact("sensor", "BMP280", "required", "BMP280")), "clarify")
	for _, prompt := range []string{
		"Please use SHT31 with 100 pF bus capacitance.",
		"I'd like SHT31 with a 3.3 V supply.",
		"Could you use SHT31 sampling at 100 Hz?",
	} {
		checkIntent(t, prompt, syntheticIntent(t, prompt, choiceFact("sensor", "SHT31", "required", "SHT31")), "clarify")
	}
}

func TestIntentNumericGrounding(t *testing.T) {
	for _, tc := range []struct {
		field, quote string
		value        float64
		want         bool
	}{
		{"clock_hz", "400 kHz I2C clock", 400000, true},
		{"clock_hz", "100000 hertz bus", 100000, true},
		{"clock_hz", "400 kHz I2C clock", 400, false},
		{"clock_hz", "sampling at 100 Hz", 100, false},
		{"clock_hz", "200 kHz with 100 pF", 100000, false},
		{"pullup_ohms", "4.7 kohms", 4700, true},
		{"pullup_ohms", "4700 ohms", 4700, true},
		{"pullup_ohms", "4.7k", 4700, true},
		{"pullup_ohms", "2.2k and 4.7 V", 4700, false},
		{"supply_capacity_ma", "source 1 ampere", 1000, true},
		{"supply_capacity_ma", "2000 milliamperes source capacity", 2000, true},
		{"supply_capacity_ma", "GPIO sink current 1000 mA", 1000, false},
		{"supply_min_v", "3.2–3.4 V", 3.2, true},
		{"supply_max_v", "3.2–3.4 V", 3.4, true},
		{"supply_max_v", "3.2–3.4 V", 3.2, false},
		{"supply_min_v", "3.2 to 3.4 V", 3.4, false},
		{"supply_max_v", "3.2V to 3.4V", 3.4, true},
		{"supply_max_v", "3.3 volts", 3.3, true},
		{"ambient_min_c", "-10–35 °C", -10, true},
		{"ambient_max_c", "10-35 degrees C", 35, true},
		{"ambient_max_c", "10–35 C", 10, false},
		{"total_bus_capacitance_pf", "100 picofarads", 100, true},
		{"total_bus_capacitance_pf", "70 pF and 100 kHz", 100, false},
		{"total_bus_capacitance_pf", "100 nF", 100, false},
		{"nonsense", "100 pF", 100, false},
	} {
		t.Run(tc.field+"/"+tc.quote+fmt.Sprint(tc.value), func(t *testing.T) {
			if got := numberGrounded(tc.field, tc.value, tc.quote); got != tc.want {
				t.Fatalf("grounding=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestIntentStrictShape(t *testing.T) {
	prompt := "Please use BMP280. Thank you."
	valid := string(syntheticIntent(t, prompt, choiceFact("sensor", "BMP280", "required", "BMP280")))
	bad := []string{
		"", "null", "[]", valid + "{}", strings.Repeat(" ", 65537),
		strings.Replace(valid, `"version":"2"`, `"version":"1"`, 1),
		strings.Replace(valid, `"version":"2"`, `"version":"1","version":"2"`, 1),
		strings.Replace(valid, `"version":"2"`, `"version":null`, 1),
		strings.Replace(valid, `"version":"2"`, `"version":"2","configuration":null`, 1),
		strings.Replace(valid, `"id":0`, `"id":-1`, 1),
		strings.Replace(valid, `"id":0`, `"id":1`, 1),
		strings.Replace(valid, `"id":0`, `"id":3`, 1),
		strings.Replace(valid, `"id":0`, `"id":0.5`, 1),
		strings.Replace(valid, `"state":"required"`, `"state":"safe"`, 1),
		strings.Replace(valid, `"state":"required",`, ``, 1),
		strings.Replace(valid, `"quote":"BMP280"`, `"quote":"SHT31"`, 1),
		strings.Replace(valid, `"value":"BMP280"`, `"value":"SHT31"`, 1),
		strings.Replace(valid, `"kind":"sensor"`, `"kind":"sensor","number":1`, 1),
		strings.Replace(valid, `"kind":"none"`, `"kind":"none","quote":"Thank you"`, 1),
		strings.Replace(valid, `"kind":"sensor"`, `"kind":"unknown"`, 1),
		`{"version":"2","clauses":[]}`,
		`{"version":"2","clauses":[{"id":0,"facts":[]},{"id":1,"facts":[{"kind":"none"}]}]}`,
		`{"version":"2","clauses":[{"id":0,"facts":null},{"id":1,"facts":[{"kind":"none"}]}]}`,
		strings.Replace(valid, `"kind":"sensor"`, `"kind":"none","kind":"sensor"`, 1),
		string([]byte{0xff}),
		strings.Repeat("[", 14) + "null" + strings.Repeat("]", 14),
	}
	for i, raw := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			d, err := DecodeIntent(prompt, []byte(raw))
			if err == nil || d.Configuration != nil || d.Disposition != "clarify" || d.Clauses[0].Text != prompt {
				t.Fatalf("invalid intent escaped: %+v %v", d, err)
			}
		})
	}
	var reordered RequirementIntent
	if err := json.Unmarshal([]byte(valid), &reordered); err != nil {
		t.Fatal(err)
	}
	reordered.Clauses[0], reordered.Clauses[1] = reordered.Clauses[1], reordered.Clauses[0]
	b, _ := json.Marshal(reordered)
	checkIntent(t, prompt, b, "supported")
	for _, count := range []int{25, 65} {
		facts := make([]RequirementFact, count)
		for i := range facts {
			facts[i] = RequirementFact{Kind: "none"}
		}
		b, _ := json.Marshal(RequirementIntent{"2", []IntentClause{{0, facts}, {1, []RequirementFact{{Kind: "none"}}}}})
		if d, err := DecodeIntent(prompt, b); err == nil || d.Configuration != nil {
			t.Fatal("accepted excess facts")
		}
	}
}

func TestIntentFalseGrounding(t *testing.T) {
	for _, f := range []RequirementFact{
		choiceFact("measurement", "pressure", "required", "sensor board"),
		choiceFact("profile", "fast", "required", "sensor board"),
		choiceFact("sensor", "BMP280", "required", "sensor board"),
		numberFact("total_bus_capacitance_pf", 100, "sensor board"),
		{Kind: "unclear", Quote: "sensor board"},
		{Kind: "other", Quote: "sensor board"},
	} {
		prompt := "Please make a sensor board."
		raw := syntheticIntent(t, prompt, f)
		if d, err := DecodeIntent(prompt, raw); err == nil || d.Configuration != nil {
			t.Fatalf("accepted ungrounded fact %+v", f)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateFact(numberFact("clock_hz", value, "100 kHz"), []byte(`{"kind":"number","value":"clock_hz","number":1,"quote":"100 kHz"}`), "100 kHz"); err == nil {
			t.Fatal("accepted nonfinite number")
		}
	}
}

func TestIntentFrozenPromptsSyntheticFacts(t *testing.T) {
	var fixture struct {
		Cases []struct {
			ID, Prompt          string
			ExpectedDisposition string `json:"expected_disposition"`
			Family, Profile     string
		}
	}
	b, err := os.ReadFile("../../specs/board-family-v2/evaluation/cases-01.json")
	if err != nil || json.Unmarshal(b, &fixture) != nil {
		t.Fatalf("read frozen prompts: %v", err)
	}
	sensor := func(s string) RequirementFact { return choiceFact("sensor", s, "required", s) }
	profile := func(s string) RequirementFact { return choiceFact("profile", s, "required", s) }
	facts := map[string][]RequirementFact{
		"supported-01":   {sensor("BMP280"), profile("standard"), numberFact("clock_hz", 100000, "100 kHz I2C bus"), numberFact("pullup_ohms", 4700, "4.7k pull-ups"), numberFact("total_bus_capacitance_pf", 200, "200 pF total bus capacitance")},
		"supported-02":   {sensor("BMP280"), numberFact("pullup_ohms", 2200, "2.2k pull-ups"), numberFact("clock_hz", 400000, "400 kHz bus"), numberFact("total_bus_capacitance_pf", 100, "100 pF total loading"), choiceFact("measurement", "humidity", "not_required", "Humidity sensing and wireless are not required"), choiceFact("feature", "wireless_operation", "not_required", "Humidity sensing and wireless are not required")},
		"supported-03":   {sensor("BMP280"), profile("low_current"), numberFact("clock_hz", 100000, "profile at 100 kHz"), numberFact("pullup_ohms", 10000, "10k low_current profile"), numberFact("total_bus_capacitance_pf", 100, "100 pF total capacitance"), choiceFact("feature", "whole_board_low_power", "not_required", "not asking for reduced whole-board power or battery operation"), choiceFact("feature", "battery_operation", "not_required", "not asking for reduced whole-board power or battery operation")},
		"supported-04":   {sensor("SHT31"), profile("standard"), numberFact("clock_hz", 100000, "pull-ups at 100 kHz"), numberFact("pullup_ohms", 4700, "4.7k pull-ups"), numberFact("total_bus_capacitance_pf", 70, "70 pF total bus capacitance"), choiceFact("measurement", "pressure", "not_required", "do not need pressure measurement or an ambient accuracy guarantee"), choiceFact("feature", "accuracy_guarantee", "not_required", "do not need pressure measurement or an ambient accuracy guarantee")},
		"supported-05":   {sensor("SHT31"), numberFact("pullup_ohms", 2200, "2.2k pull-ups"), numberFact("clock_hz", 400000, "400 kHz bus"), numberFact("total_bus_capacitance_pf", 100, "100 pF total loading"), choiceFact("feature", "heater_operation", "forbidden", "Keep the heater off")},
		"clarify-01":     {},
		"clarify-02":     {choiceFact("sensor", "BMP280", "uncertain", "either a BMP280 pressure board"), choiceFact("sensor", "SHT31", "uncertain", "or an SHT31 humidity board")},
		"clarify-03":     {sensor("SHT31"), choiceFact("profile", "standard", "uncertain", "Either the standard or fast bus profile might work"), choiceFact("profile", "fast", "uncertain", "Either the standard or fast bus profile might work")},
		"unsupported-01": {sensor("BMP280"), sensor("SHT31")},
		"unsupported-02": {sensor("SHT31"), profile("standard"), numberFact("pullup_ohms", 4700, "4.7k pull-ups"), numberFact("clock_hz", 100000, "pull-ups at 100 kHz"), numberFact("total_bus_capacitance_pf", 100, "100 pF total bus capacitance")},
		"unsupported-03": {sensor("SHT31"), profile("low_current"), numberFact("pullup_ohms", 10000, "10k low_current profile"), numberFact("total_bus_capacitance_pf", 100, "100 pF total capacitance")},
		"unsupported-04": {sensor("SHT31"), choiceFact("feature", "heater_operation", "required", "run the heater continuously while taking normal humidity measurements")},
		"unsupported-05": {sensor("BMP280"), choiceFact("feature", "usb", "required", "powered directly by 5 V USB"), choiceFact("feature", "wireless_operation", "required", "wireless telemetry")},
		"unsupported-06": {sensor("SHT31"), choiceFact("feature", "accuracy_guarantee", "required", "guarantee assembled-board ambient temperature accuracy of plus or minus 0.1 degrees C without calibration or bench characterization")},
	}
	if len(facts) != len(fixture.Cases) {
		t.Fatal("frozen prompt coverage changed")
	}
	for _, c := range fixture.Cases {
		t.Run(c.ID, func(t *testing.T) {
			d := checkIntent(t, c.Prompt, syntheticIntent(t, c.Prompt, facts[c.ID]...), c.ExpectedDisposition)
			if d.Configuration != nil && (d.Configuration.Family != c.Family || d.Configuration.Profile != c.Profile) {
				t.Fatalf("wrong family/profile: %+v", d.Configuration)
			}
		})
	}
}

func FuzzDecodeIntentNeverExposesInvalidConfig(f *testing.F) {
	f.Add("Please use BMP280.", `{"version":"2","clauses":[{"id":0,"facts":[{"kind":"sensor","value":"BMP280","state":"required","quote":"BMP280"}]}]}`)
	f.Add("Please use SHT31; run the heater.", `{"version":"2","clauses":[]}`)
	f.Fuzz(func(t *testing.T, prompt, raw string) {
		d, err := DecodeIntent(prompt, []byte(raw))
		if d.Configuration != nil {
			if err != nil || d.Disposition != "supported" {
				t.Fatalf("invalid design exposure: %+v %v", d, err)
			}
			if _, err := Check(*d.Configuration); err != nil {
				t.Fatal(err)
			}
		}
		if len(d.Clauses) != 1 || d.Clauses[0].Text != prompt {
			t.Fatal("lost original request")
		}
	})
}
