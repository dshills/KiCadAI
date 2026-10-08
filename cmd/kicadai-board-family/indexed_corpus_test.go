package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/boardfamily"
)

// Hand-authored synthetic responses for the known regression prompts. This is
// a test-only transport fixture, not an AI result, parser or production route.
// Gold requirements never enter the real provider request; native generation,
// journal, accounting and audit are unchanged.
func indexedCorpusFixture(t testing.TB, id, prompt string) []byte {
	t.Helper()
	source, err := boardfamily.PrepareReferencedRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	facts := []boardfamily.ReferencedFact{}
	add := func(kind, value, state string, fragments ...string) {
		f := indexedCommandFact(kind, value, state)
		for _, fragment := range fragments {
			found := false
			for _, c := range source.Clauses {
				if strings.Contains(c.Text, fragment) {
					found = true
					if !slices.Contains(f.Sources, c.ID) {
						f.Sources = append(f.Sources, c.ID)
					}
				}
			}
			if !found {
				t.Fatalf("fixture %s has no source fragment %q", id, fragment)
			}
		}
		facts = append(facts, f)
	}
	number := func(field string, value float64) {
		for _, q := range source.Quantities {
			if n, ok := q.Fields[field]; ok && n == value {
				f := indexedCommandFact("number", field, "required", q.ClauseID)
				f.Quantities = []int{q.ID}
				facts = append(facts, f)
				return
			}
		}
		t.Fatalf("fixture %s has no quantity %s=%v", id, field, value)
	}
	switch id {
	case "useful-01":
		add("measurement", "pressure", "required", "wired pressure")
		add("profile", "standard", "required", "standard profile")
	case "useful-02":
		add("sensor", "BMP280", "required", "BMP280 controller")
		number("pullup_ohms", 2200)
		number("clock_hz", 400000)
		number("total_bus_capacitance_pf", 100)
		add("measurement", "humidity", "not_required", "Humidity and wireless")
		add("feature", "wireless_operation", "not_required", "Humidity and wireless")
	case "useful-03":
		add("sensor", "BMP280", "required", "BMP280 board")
		add("profile", "low_current", "required", "low_current profile")
		number("pullup_ohms", 10000)
		number("clock_hz", 100000)
		number("total_bus_capacitance_pf", 100)
		add("feature", "whole_board_low_power", "not_required", "not whole-board power")
		add("feature", "battery_operation", "not_required", "not whole-board power")
	case "useful-04":
		add("measurement", "temperature", "required", "temperature and humidity")
		add("measurement", "humidity", "required", "temperature and humidity")
		add("profile", "standard", "required", "standard profile")
		number("total_bus_capacitance_pf", 70)
		add("measurement", "pressure", "not_required", "don't need pressure")
	case "useful-05":
		add("sensor", "SHT31", "required", "SHT31 with the fast")
		add("profile", "fast", "required", "SHT31 with the fast")
		number("total_bus_capacitance_pf", 100)
		add("feature", "heater_operation", "forbidden", "Do not run the heater")
	case "choice-01": // Deliberately no invented sensor or measurement.
	case "choice-02":
		add("sensor", "BMP280", "uncertain", "Either BMP280", "haven't chosen")
		add("sensor", "SHT31", "uncertain", "Either BMP280", "haven't chosen")
	case "choice-03":
		add("sensor", "SHT31", "required", "Please use SHT31")
		add("profile", "standard", "uncertain", "deciding between")
		add("profile", "fast", "uncertain", "deciding between")
	case "refuse-01":
		add("sensor", "BMP280", "required", "BMP280 and an SHT31", "omitting either")
		add("sensor", "SHT31", "required", "BMP280 and an SHT31", "omitting either")
	case "refuse-02":
		add("sensor", "SHT31", "required", "SHT31 with the standard")
		add("profile", "standard", "required", "SHT31 with the standard", "values unchanged")
		number("pullup_ohms", 4700)
		number("clock_hz", 100000)
		number("total_bus_capacitance_pf", 100)
	case "refuse-03":
		add("sensor", "SHT31", "required", "SHT31 board")
		add("profile", "low_current", "required", "low_current profile")
		number("pullup_ohms", 10000)
		number("total_bus_capacitance_pf", 100)
	case "refuse-04":
		add("sensor", "SHT31", "required", "SHT31 controller")
		add("feature", "heater_operation", "forbidden", "At startup")
		add("feature", "heater_operation", "required", "energize its heater")
	case "refuse-05":
		add("sensor", "BMP280", "required", "BMP280 controller")
		add("feature", "usb", "required", "5 V USB")
		facts[len(facts)-1].Quantities = []int{0}
		add("feature", "wireless_operation", "required", "wireless telemetry")
	case "refuse-06":
		add("sensor", "SHT31", "required", "SHT31 temperature")
		add("feature", "accuracy_guarantee", "required", "guaranteed assembled-board")
		facts[len(facts)-1].Quantities = []int{0}
	default:
		t.Fatalf("unrecognized synthetic corpus case %q", id)
	}
	return indexedCommandRaw(t, facts...)
}

func TestIndexedCorpusSyntheticAdmissions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Cases []struct {
			ID          string `json:"id"`
			Prompt      string `json:"prompt"`
			Disposition string `json:"expected_disposition"`
			Family      string `json:"family"`
			Profile     string `json:"profile"`
		}
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Cases) != 14 {
		t.Fatal("expected unchanged 14-case corpus")
	}
	for _, c := range spec.Cases {
		t.Run(c.ID, func(t *testing.T) {
			d, err := boardfamily.DecodeReferencedIntent(c.Prompt, indexedCorpusFixture(t, c.ID, c.Prompt))
			if err != nil || d.Disposition != c.Disposition {
				t.Fatalf("synthetic fixture does not reach expected admission: %+v; %v", d, err)
			}
			if c.Disposition == "supported" && (d.Configuration == nil || d.Configuration.Family != c.Family || d.Configuration.Profile != c.Profile) {
				t.Fatalf("wrong synthetic configuration: %+v", d.Configuration)
			}
		})
	}
}
