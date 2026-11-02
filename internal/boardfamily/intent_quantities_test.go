package boardfamily

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSourceQuantitiesNormalizeWithoutModelNumbers(t *testing.T) {
	for _, tc := range []struct {
		literal string
		fields  map[string]float64
	}{
		{"400 kilohertz", map[string]float64{"clock_hz": 400000}},
		{"100 Hz", map[string]float64{"clock_hz": 100}},
		{"2.2 kilohm", map[string]float64{"pullup_ohms": 2200}},
		{"4.7 kohms", map[string]float64{"pullup_ohms": 4700}},
		{"10k", map[string]float64{"pullup_ohms": 10000}},
		{"2200 ohms", map[string]float64{"pullup_ohms": 2200}},
		{"3.2–3.4 V", map[string]float64{"supply_min_v": 3.2, "supply_max_v": 3.4}},
		{"between 3.2 and 3.4 V", map[string]float64{"supply_min_v": 3.2, "supply_max_v": 3.4}},
		{"between 3200 mV and 3.4 V", map[string]float64{"supply_min_v": 3.2, "supply_max_v": 3.4}},
		{"3200 millivolts to 3.4 V", map[string]float64{"supply_min_v": 3.2, "supply_max_v": 3.4}},
		{"3.3 volts", map[string]float64{"supply_min_v": 3.3, "supply_max_v": 3.3}},
		{"3.25 V - 3.35 volts", map[string]float64{"supply_min_v": 3.25, "supply_max_v": 3.35}},
		{"2 A", map[string]float64{"supply_capacity_ma": 2000}},
		{"1e3 milliamperes", map[string]float64{"supply_capacity_ma": 1000}},
		{"+1.5 amps", map[string]float64{"supply_capacity_ma": 1500}},
		{"15–30 C", map[string]float64{"ambient_min_c": 15, "ambient_max_c": 30}},
		{"-15 to -5 °C", map[string]float64{"ambient_min_c": -15, "ambient_max_c": -5}},
		{"25 degrees Celsius", map[string]float64{"ambient_min_c": 25, "ambient_max_c": 25}},
		{".1 degrees C", map[string]float64{"ambient_min_c": .1, "ambient_max_c": .1}},
		{"70 picofarads", map[string]float64{"total_bus_capacitance_pf": 70}},
		{".1 nF", map[string]float64{"total_bus_capacitance_pf": 100}},
		{"1e-4 uF", map[string]float64{"total_bus_capacitance_pf": 100}},
		{"0.0001 µF", map[string]float64{"total_bus_capacitance_pf": 100}},
		{"0.0001 μF", map[string]float64{"total_bus_capacitance_pf": 100}},
		{"0.0001 microfarads", map[string]float64{"total_bus_capacitance_pf": 100}},
		{"3.4–3.2 V", map[string]float64{}}, // no endpoint sorting
		{"3.2 V to 3.4 C", map[string]float64{}},
		{"1e309 V", map[string]float64{}},
		{"1e-999 V", map[string]float64{}},
		{"1e-323 mV", map[string]float64{}}, // underflow after unit scaling
		{"0e-999 V", map[string]float64{"supply_min_v": 0, "supply_max_v": 0}},
		{"1e308 A", map[string]float64{}},     // overflow after unit scaling
		{"120 mm", map[string]float64{}},      // fixed geometry is not a config field
		{"100–400 kHz", map[string]float64{}}, // no invented scalar from a range
		{"50 to 100 pF", map[string]float64{}},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			prompt := "Please use BMP280 with " + tc.literal + ". Thanks!"
			r, err := PrepareReferencedRequest(prompt)
			if err != nil || len(r.Quantities) != 1 {
				t.Fatalf("quantity inventory: %+v %v", r, err)
			}
			q := r.Quantities[0]
			if q.ID != 0 || q.ClauseID != 0 || q.Text != tc.literal || prompt[q.Start:q.End] != tc.literal || !reflect.DeepEqual(q.Fields, tc.fields) {
				t.Fatalf("incorrect application-owned quantity: %+v; want %+v", q, tc.fields)
			}
		})
	}
}

func TestSourceQuantitiesIdentityAndExactBytes(t *testing.T) {
	for _, prompt := range []string{"I2C, SHT31-DIS-B2.5KS, ESP32 and 0x44.", "α3.3V, 3.3Vβ, _100pF."} {
		if r, err := PrepareReferencedRequest(prompt); err != nil || len(r.Quantities) != 0 {
			t.Fatalf("identifier was treated as a quantity: %+v %v", r, err)
		}
	}
	prompt := "\nBonjour — use SHT31-DIS-B2.5KS on I2C, at address 0x44. A .1 nF bus is fine; the alternative .1 nF still refers to a separate occurrence.\n  "
	r, err := PrepareReferencedRequest(prompt)
	if err != nil || len(r.Quantities) != 2 {
		t.Fatalf("invented or lost quantity: %+v %v", r, err)
	}
	joined := ""
	for i, c := range r.Clauses {
		if c.ID != i {
			t.Fatal("unstable source clause identity")
		}
		joined += c.Text
	}
	if joined != prompt || r.Request != prompt {
		t.Fatal("original bytes changed")
	}
	for i, q := range r.Quantities {
		if q.ID != i || q.Text != ".1 nF" || prompt[q.Start:q.End] != q.Text || !strings.Contains(r.Clauses[q.ClauseID].Text, q.Text) {
			t.Fatalf("wrong source occurrence: %+v", q)
		}
	}
	if r.Quantities[0].Start == r.Quantities[1].Start || r.Quantities[0].ClauseID == r.Quantities[1].ClauseID {
		t.Fatal("repeated quantities were coalesced")
	}
	r2, _ := PrepareReferencedRequest(prompt)
	if !reflect.DeepEqual(r, r2) {
		t.Fatal("source table is nondeterministic")
	}
	for _, id := range retainedTypedCaseIDs {
		s := retainedTypedSelection(t, id)
		r, err := PrepareReferencedRequest(s.OriginalRequest)
		if err != nil || !reflect.DeepEqual(r.Clauses, s.RequestClauses) {
			t.Fatalf("known original clauses changed for %s: %+v %v", id, r, err)
		}
	}
}

func TestIndexedQuantityContractRejectsInventedEvidence(t *testing.T) {
	prompt := "Use BMP280 with 400 kHz I2C clock and 100 pF bus capacitance. Keep the defaults."
	valid := string(syntheticReferencedIntent(t, referencedNumber("clock_hz", 0, 0)))
	for _, raw := range []string{
		strings.Replace(valid, `"quantities":[0]`, `"quantities":null`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[]`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[-1]`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[2]`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[0,0]`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[0,1]`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[1]`, 1), // capacitance cannot become clock
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[0],"number":100000`, 1),
		strings.Replace(valid, `"quantities":[0]`, `"quantities":[0],"number":400000`, 1), // even a correct copied magnitude is not this protocol
		strings.Replace(valid, `"sources":[0]`, `"sources":[1]`, 1),
		strings.Replace(valid, `"clock_hz"`, `"invented_field"`, 1),
		strings.Replace(valid, `"required"`, `"maybe"`, 1),
		string(syntheticReferencedIntent(t, referencedWithQuantities(referencedChoice("sensor", "BMP280", "required", 0), 0))),
		string(syntheticReferencedIntent(t, referencedWithQuantities(referencedChoice("feature", "wireless_operation", "not_required", 0), 0, 0))),
	} {
		if d, err := DecodeReferencedIntent(prompt, []byte(raw)); err == nil || d.Configuration != nil {
			t.Fatalf("invalid quantity evidence admitted: %s; %+v %v", raw, d, err)
		}
	}
	if d, err := DecodeReferencedIntent("Please use BMP280 with the reviewed default supply limits.", syntheticReferencedIntent(t, referencedNumber("supply_min_v", 0, 0))); err == nil || d.Configuration != nil {
		t.Fatal("invented default quantity admitted")
	}
}

func TestIndexedQuantityCoverageIsPerOccurrence(t *testing.T) {
	sensor := referencedChoice("sensor", "BMP280", "required", 0)
	for _, tc := range []struct {
		prompt string
		facts  []ReferencedFact
	}{
		{"Please use BMP280 with 100 pF total bus capacitance and 100 pF additional capacitance.", []ReferencedFact{sensor, referencedNumber("total_bus_capacitance_pf", 0, 0)}},
		{"Please use BMP280 with 100 pF total bus capacitance; wireless is not required.", []ReferencedFact{sensor, referencedChoice("feature", "wireless_operation", "not_required", 0, 1)}},
		{"Please use BMP280 with 100 pF total bus capacitance and a 3.3 V supply; wireless is not required.", []ReferencedFact{sensor, referencedNumber("total_bus_capacitance_pf", 0, 0), referencedChoice("feature", "wireless_operation", "not_required", 0, 1)}},
	} {
		d, err := DecodeReferencedIntent(tc.prompt, syntheticReferencedIntent(t, tc.facts...))
		if err != nil || d.Disposition != "clarify" || d.Configuration != nil || !strings.Contains(d.Message, "source quantity") {
			t.Fatalf("numeric omission disappeared: %+v %v", d, err)
		}
	}
}

func TestIndexedQuantityRequirementStates(t *testing.T) {
	sensor := referencedChoice("sensor", "BMP280", "required", 0)
	for _, tc := range []struct {
		prompt, state, field, disposition, profile string
	}{
		{"Please use BMP280; use a 400 kHz I2C clock.", "required", "clock_hz", "supported", "fast"},
		{"Please use BMP280; a 400 kHz I2C clock is not required.", "not_required", "clock_hz", "supported", "standard"},
		{"Please use BMP280; a 100 kHz I2C clock is forbidden.", "forbidden", "clock_hz", "supported", "fast"},
		{"Please use BMP280; 4.7k pull-ups are forbidden.", "forbidden", "pullup_ohms", "supported", "fast"},
		{"Please use BMP280; I have not decided whether I want a 400 kHz I2C clock.", "uncertain", "clock_hz", "clarify", ""},
		{"Please use BMP280; a 3.3 V supply is forbidden.", "forbidden", "supply_min_v", "clarify", ""},
	} {
		t.Run(tc.state+"/"+tc.field, func(t *testing.T) {
			fact := referencedNumber(tc.field, 0, 1)
			fact.State = tc.state
			d, err := DecodeReferencedIntent(tc.prompt, syntheticReferencedIntent(t, sensor, fact))
			if err != nil || d.Disposition != tc.disposition {
				t.Fatalf("state collapsed or inferred positive value: %+v %v", d, err)
			}
			if tc.disposition == "supported" {
				if d.Configuration == nil || d.Configuration.Profile != tc.profile {
					t.Fatalf("wrong profile: %+v", d.Configuration)
				}
			} else if d.Configuration != nil || !strings.Contains(d.Message, tc.field) {
				t.Fatal("numeric clarification is not targeted")
			}
		})
	}
	// A non-configurable numeric requirement is explicitly preserved, including
	// whether it is demanded, excluded, prohibited, or genuinely unresolved.
	for _, state := range []string{"required", "not_required", "forbidden", "uncertain"} {
		prompt := "Please use BMP280; a 100 Hz reporting rate is " + state + "."
		f := ReferencedFact{Kind: "other", State: state, Sources: []int{1}, Quantities: []int{0}, Detail: "A delivered 100 Hz reporting rate is outside this hardware-only generator."}
		want := "unsupported"
		if state == "not_required" {
			want = "supported"
		} else if state == "uncertain" {
			want = "clarify"
		}
		d, err := DecodeReferencedIntent(prompt, syntheticReferencedIntent(t, sensor, f))
		if err != nil || d.Disposition != want {
			t.Fatalf("non-configurable numeric state %s: %+v %v", state, d, err)
		}
	}
}

func TestIndexedQuantityConversionsAndTableOwnership(t *testing.T) {
	prompt := "Please use BMP280 with a 400 kilohertz I2C clock, 2.2 kilohm pull-ups, .1 nF total bus capacitance and a 3.2–3.4 V supply."
	facts := []ReferencedFact{referencedChoice("sensor", "BMP280", "required", 0), referencedNumber("clock_hz", 0, 0), referencedNumber("pullup_ohms", 1, 0), referencedNumber("total_bus_capacitance_pf", 2, 0), referencedNumber("supply_min_v", 3, 0), referencedNumber("supply_max_v", 3, 0)}
	r, err := PrepareReferencedRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	r.Quantities[2].Fields["total_bus_capacitance_pf"] = 1000 // Mutating a returned table grants no authority.
	d, err := DecodeReferencedIntent(prompt, syntheticReferencedIntent(t, facts...))
	want := Config{"1", Family, "fast", 3.2, 3.4, 1000, 10, 35, 100}
	if err != nil || d.Configuration == nil || *d.Configuration != want {
		t.Fatalf("source conversion changed or external table was trusted: %+v %v", d, err)
	}
	for _, prompt := range []string{"", " \n", strings.Repeat("a", 2001), strings.Repeat("a;", 33), strings.Repeat("1 V ", 129), string([]byte{0xff})} {
		if _, err := PrepareReferencedRequest(prompt); err == nil {
			t.Fatalf("unbounded/invalid source inventory accepted: %q", prompt)
		}
	}
}

func FuzzReferencedQuantitySourceIntegrity(f *testing.F) {
	for _, prompt := range []string{"SHT31; .1 nF, 3.2–3.4 V, 2 A.", "BMP280 on I2C at 0x76.", "こんにちは .1 nF. .1 nF."} {
		f.Add(prompt)
	}
	f.Fuzz(func(t *testing.T, prompt string) {
		r, err := PrepareReferencedRequest(prompt)
		if err != nil {
			return
		}
		joined := ""
		for _, c := range r.Clauses {
			joined += c.Text
		}
		if joined != prompt || r.Request != prompt {
			t.Fatal("source text changed")
		}
		last := 0
		for i, q := range r.Quantities {
			if q.ID != i || q.Start < last || q.End <= q.Start || q.End > len(prompt) || prompt[q.Start:q.End] != q.Text || q.ClauseID < 0 || q.ClauseID >= len(r.Clauses) {
				t.Fatalf("invalid source index: %+v", q)
			}
			last = q.End
			for field, value := range q.Fields {
				if !member(field, intentNumericFields...) || math.IsNaN(value) || math.IsInf(value, 0) {
					t.Fatal("invalid application-normalized field")
				}
			}
		}
		if _, err := json.Marshal(r); err != nil {
			t.Fatal(err)
		}
	})
}
