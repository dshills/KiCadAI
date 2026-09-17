package boardfamily

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func checkBoundaryPartition(t testing.TB, input SemanticBoundaryRequest) {
	t.Helper()
	prompt := input.Source.Request
	coverage := make([]int, len(prompt))
	check := func(clauseID, start, end int) {
		t.Helper()
		if start < 0 || start >= end || end > len(prompt) || !utf8.ValidString(prompt[start:end]) {
			t.Fatal("invalid byte span", clauseID, start, end)
		}
		offset, inClause := 0, false
		for _, c := range input.Source.Clauses {
			if c.ID == clauseID {
				inClause = start >= offset && end <= offset+len(c.Text)
			}
			offset += len(c.Text)
		}
		if !inClause {
			t.Fatal("span outside owner clause")
		}
		for i := start; i < end; i++ {
			coverage[i]++
		}
	}
	for _, control := range input.Controls {
		check(control.ClauseID, control.Start, control.End)
		for _, q := range input.Source.Quantities {
			if control.Start < q.End && q.Start < control.End {
				t.Fatal("control consumed a source quantity", control, q)
			}
		}
	}
	for i, residual := range input.Residuals {
		check(residual.ClauseID, residual.Start, residual.End)
		if residual.ID != "r"+strconv.Itoa(i) || residual.Text != prompt[residual.Start:residual.End] {
			t.Fatal("residual rewritten or renamed", residual)
		}
	}
	for i, n := range coverage {
		if n != 1 {
			t.Fatal("source byte lost or duplicated", i, n)
		}
	}
}

func requestAllBoundaryMentions(t testing.TB, input SemanticBoundaryRequest, f map[string]any) {
	t.Helper()
	for _, m := range input.Mentions {
		state := "requested"
		if fixed, ok := input.RequiredState[m.ID]; ok {
			state = fixed
		}
		setAddressedState(t, input.SourceAddressedRequest, f, m.ClauseID, m.Kind, m.Value, state)
	}
}

func TestSemanticBoundaryCompoundDefaultsAndProfileMeaning(t *testing.T) {
	for _, tc := range []struct {
		prompt, family, profile string
		controls                int
		standardContext         bool
		pf                      float64
	}{
		{"Use SHT31. Please use your standard profile, 70 pF total bus capacitance, and the reviewed default electrical and ambient limits.", FamilySHT31, "standard", 1, false, 70},
		{"Use BMP280. Your standard profile and reviewed default operating limits are fine.", Family, "standard", 1, false, 0},
		{"I'd like the SHT31 controller with the reviewed electrical defaults.", FamilySHT31, "standard", 1, false, 0},
		{"I’d like the SHT31 controller with the reviewed electrical defaults.", FamilySHT31, "standard", 1, false, 0},
		{"Please build an SHT31 temperature and humidity board using the standard electrical defaults.", FamilySHT31, "standard", 1, true, 0},
		{"Use SHT31 fast with standard electrical defaults.", FamilySHT31, "fast", 1, true, 0},
		{"Use SHT31 with the standard profile and standard electrical defaults.", FamilySHT31, "standard", 1, false, 0},
		{"Use BMP280 with reviewed ambient limits and reviewed electrical defaults.", Family, "standard", 2, false, 0},
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			input, f := boundaryFixture(t, tc.prompt)
			checkBoundaryPartition(t, input)
			if len(input.Controls) != tc.controls {
				t.Fatal("wrong control inventory", input.Controls)
			}
			requestAllBoundaryMentions(t, input, f)
			fixedStandard := false
			for _, m := range input.Mentions {
				if m.Kind == "profile" && m.Value == "standard" && input.RequiredState[m.ID] == "context_only" {
					fixedStandard = true
				}
			}
			if fixedStandard != tc.standardContext {
				t.Fatal("default adjective confused with named profile", input.RequiredState)
			}
			if tc.pf != 0 {
				f["quantities"] = map[string]any{"q0": []any{groundedNumber("total_bus_capacitance_pf", "requested")}}
			}
			d := checkBoundary(t, tc.prompt, f, "supported")
			if d.Configuration.Family != tc.family || d.Configuration.Profile != tc.profile || tc.pf != 0 && d.Configuration.TotalBusCapacitancePF != tc.pf {
				t.Fatal("wrong configuration", d)
			}
		})
	}
}

func TestSemanticBoundaryResidualHardwareBeforeAndAfterDefaults(t *testing.T) {
	for _, prompt := range []string{
		"Use SHT31 with galvanic isolation and reviewed electrical defaults.",
		"Use SHT31 with reviewed electrical defaults and galvanic isolation.",
		"Use SHT31 with reviewed ambient limits, galvanic isolation and reviewed electrical defaults.",
	} {
		t.Run(prompt, func(t *testing.T) {
			input, f := boundaryFixture(t, prompt)
			checkBoundaryPartition(t, input)
			requestAllBoundaryMentions(t, input, f)
			found := false
			for _, r := range input.Residuals {
				if strings.Contains(r.Text, "galvanic isolation") {
					found = true
					f["additional"].(map[string]any)["c"+strconv.Itoa(r.ClauseID)] = []any{map[string]any{
						"kind": "other", "state": "requested", "context": []string{}, "span": r.ID,
					}}
				}
			}
			if !found {
				t.Fatal("unknown hardware requirement disappeared")
			}
			d := checkBoundary(t, prompt, f, "unsupported")
			if !strings.Contains(d.Message, "galvanic isolation") {
				t.Fatal("remaining hardware fact not admitted", d)
			}
		})
	}
}

func TestSemanticBoundaryPartialControlsDoNotWeakenNumericLimits(t *testing.T) {
	prompt := "Use SHT31 standard with reviewed electrical defaults and 100 pF bus capacitance."
	input, f := boundaryFixture(t, prompt)
	checkBoundaryPartition(t, input)
	requestAllBoundaryMentions(t, input, f)
	f["quantities"] = map[string]any{"q0": []any{groundedNumber("total_bus_capacitance_pf", "requested")}}
	d := checkBoundary(t, prompt, f, "unsupported")
	if !strings.Contains(d.Message, "70") {
		t.Fatal("capacitance replaced by defaults", d)
	}
}

func TestSemanticBoundaryRejectsResidualAnchorTampering(t *testing.T) {
	prompt := "Use SHT31 with reviewed electrical defaults and galvanic isolation. Keep all stated requirements unchanged."
	for _, span := range []any{"c0", "r99", "", nil, 0, []string{"r0"}} {
		t.Run(strconv.FormatInt(int64(len(addressedJSON(t, span))), 10)+"-"+string(addressedJSON(t, span)), func(t *testing.T) {
			_, f := boundaryFixture(t, prompt)
			f["additional"].(map[string]any)["c0"] = []any{map[string]any{
				"kind": "other", "state": "requested", "context": []string{}, "span": span,
			}}
			raw := addressedJSON(t, f)
			schema, _ := SemanticBoundarySchema(prompt)
			if checkGroundedSchema(t, schema, raw) == nil {
				t.Fatal("schema accepted invalid residual anchor")
			}
			if d, err := DecodeSemanticBoundaryIntent(prompt, raw); err == nil || d.Configuration != nil {
				t.Fatal("decoder accepted invalid residual anchor", d, err)
			}
		})
	}
	// A valid residual ID cannot be borrowed by a different, complete-control clause.
	_, f := boundaryFixture(t, prompt)
	f["additional"].(map[string]any)["c1"] = []any{map[string]any{
		"kind": "other", "state": "requested", "context": []string{}, "span": "r0",
	}}
	if d, err := DecodeSemanticBoundaryIntent(prompt, addressedJSON(t, f)); err == nil || d.Configuration != nil {
		t.Fatal("cross-clause residual accepted", d, err)
	}
}

func TestSemanticBoundaryRejectsDefaultAdjectiveAsProfile(t *testing.T) {
	prompt := "Use SHT31 fast with standard electrical defaults."
	input, f := boundaryFixture(t, prompt)
	requestAllBoundaryMentions(t, input, f)
	for _, wrong := range []string{"requested", "unnecessary_but_allowed", "must_not_occur", "unresolved_choice"} {
		setAddressedState(t, input.SourceAddressedRequest, f, 0, "profile", "standard", wrong)
		if d, err := DecodeSemanticBoundaryIntent(prompt, addressedJSON(t, f)); err == nil || d.Configuration != nil {
			t.Fatal("default adjective became a profile requirement", wrong, d, err)
		}
	}
}

func TestSemanticBoundaryCompoundScopeCuesRemainModelOwned(t *testing.T) {
	for _, prompt := range []string{
		"Use SHT31 but do not use reviewed electrical defaults.",
		"Use SHT31 without standard electrical defaults.",
		"Use a label saying reviewed electrical defaults.",
		"Your request mentions standard electrical defaults and those are fine.",
		"I need you to avoid reviewed electrical defaults.",
		"If possible, use SHT31 with reviewed electrical defaults.",
		"Use SHT31 with reviewed electrical defaults unless the regulator changes.",
		"Useé SHT31 with reviewed electrical defaults.",
		"Youré standard profile and reviewed electrical defaults are fine.",
		"Use SHT31 with éstandard electrical defaults.",
		"Use SHT31 with standard electrical defaultsλ.",
		"Use SHT31 with reviewed electrical limitsα.",
		"Use SHT31 with reviewed electrical defaults\u0301.",
	} {
		t.Run(prompt, func(t *testing.T) {
			input, err := PrepareSemanticBoundaryRequest(prompt)
			if err != nil || len(input.Controls) != 0 {
				t.Fatal("non-affirmative control invented", input.Controls, err)
			}
			checkBoundaryPartition(t, input)
		})
	}
}

func TestSemanticBoundaryFrozenCorpusRepresentability(t *testing.T) {
	// Deliberately handcrafted extraction, never recorded provider output or a
	// model test. All 14 unchanged prompts must remain expressible after adding
	// the deterministic boundary constraints. Expected answers stay local.
	raw, err := os.ReadFile("../../specs/board-family-v2/typed-evaluation-02/cases-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID, Prompt, Family, Profile string
			Disposition                 string  `json:"expected_disposition"`
			Capacitance                 float64 `json:"total_bus_capacitance_pf"`
		}
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("frozen inventory changed")
	}
	fields := map[string][]string{
		"useful-02": {"pullup_ohms", "clock_hz", "total_bus_capacitance_pf"},
		"useful-03": {"pullup_ohms", "clock_hz", "total_bus_capacitance_pf"},
		"useful-04": {"total_bus_capacitance_pf"},
		"useful-05": {"total_bus_capacitance_pf"},
		"refuse-02": {"pullup_ohms", "clock_hz", "total_bus_capacitance_pf"},
		"refuse-03": {"pullup_ohms", "total_bus_capacitance_pf"},
	}
	for _, tc := range corpus.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			input, f := boundaryFixture(t, tc.Prompt)
			checkBoundaryPartition(t, input)
			requestAllBoundaryMentions(t, input, f)
			for _, m := range input.Mentions {
				if tc.ID == "choice-02" || tc.ID == "choice-03" && m.Kind == "profile" {
					setAddressedState(t, input.SourceAddressedRequest, f, m.ClauseID, m.Kind, m.Value, "unresolved_choice")
				}
				if tc.ID == "refuse-04" && m.ClauseID == 2 && m.Kind == "measurement" {
					setAddressedState(t, input.SourceAddressedRequest, f, m.ClauseID, m.Kind, m.Value, "context_only")
				}
			}
			for i, field := range fields[tc.ID] {
				f["quantities"].(map[string]any)["q"+strconv.Itoa(i)] = []any{groundedNumber(field, "requested")}
			}
			if tc.ID == "refuse-05" {
				f["quantities"].(map[string]any)["q0"] = []any{groundedNumber("supply_min_v", "requested"), groundedNumber("supply_max_v", "requested")}
				for _, r := range input.Residuals {
					if r.ClauseID == 1 {
						f["additional"].(map[string]any)["c1"] = []any{map[string]any{
							"kind": "other", "state": "requested", "context": []string{"c0"}, "span": r.ID,
						}}
					}
				}
			}
			if tc.ID == "refuse-06" {
				f["quantities"].(map[string]any)["q0"] = []any{map[string]any{
					"kind": "other", "state": "requested", "context": []string{},
				}}
			}
			d := checkBoundary(t, tc.Prompt, f, tc.Disposition)
			if tc.Disposition == "supported" && (d.Configuration.Family != tc.Family || d.Configuration.Profile != tc.Profile || d.Configuration.TotalBusCapacitancePF != tc.Capacitance) {
				t.Fatal("wrong frozen expected configuration", d)
			}
			if tc.ID == "refuse-02" && !strings.Contains(d.Message, "70") || tc.ID == "refuse-04" && !strings.Contains(strings.ToLower(d.Message), "heater") || tc.ID == "refuse-05" && (!strings.Contains(d.Message, "USB") || !strings.Contains(d.Message, "Wireless")) {
				t.Fatal("substantive refusal reason missing", d)
			}
		})
	}
}

func FuzzSemanticBoundarySourcePartition(f *testing.F) {
	for _, prompt := range []string{
		"Use SHT31 with reviewed electrical defaults and galvanic isolation.",
		"I’d like SHT31 with standard electrical defaults. Use the reviewed ambient limits.",
		"Use SHT31 with 70 pF bus capacitance and reviewed default electrical and ambient limits.",
		"Your standard profile and reviewed default operating limits are fine.",
	} {
		f.Add(prompt)
	}
	f.Fuzz(func(t *testing.T, prompt string) {
		input, err := PrepareSemanticBoundaryRequest(prompt)
		if err != nil {
			return
		}
		checkBoundaryPartition(t, input)
	})
}
