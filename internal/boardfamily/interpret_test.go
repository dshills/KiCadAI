package boardfamily

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionRequirementPreservation(t *testing.T) {
	prompt := "A pressure board with low I2C pull-up current."
	c := testConfig()
	c.Profile = "low_current"
	c.TotalBusCapacitancePF = 100
	d := Decision{"supported", "Using the low-current I2C profile and its declared family envelope.", []Clause{{prompt, "supported", "Selects low_current bus profile."}}, &c}
	b, _ := json.Marshal(d)
	if _, e := DecodeDecision(prompt, b); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Decision){func(d *Decision) { d.Clauses[0].Text = "A pressure board." }, func(d *Decision) { d.Clauses[0].Disposition = "unsupported" }, func(d *Decision) { d.Configuration.SupplyMaxV = 5 }, func(d *Decision) { d.Configuration = nil }} {
		local := d
		local.Clauses = append([]Clause(nil), d.Clauses...)
		copyC := c
		local.Configuration = &copyC
		change(&local)
		b, _ = json.Marshal(local)
		if _, e := DecodeDecision(prompt, b); e == nil {
			t.Fatal("accepted dropped/unsupported requirement")
		}
	}
}
func TestNonSupportedNeverHasConfiguration(t *testing.T) {
	for _, status := range []string{"unsupported", "clarify"} {
		d := Decision{status, "Specific unsupported requirement or question.", []Clause{{"prompt", status, "Reason."}}, nil}
		b, _ := json.Marshal(d)
		if _, e := DecodeDecision("prompt", b); e != nil {
			t.Fatal(e)
		}
		c := testConfig()
		d.Configuration = &c
		b, _ = json.Marshal(d)
		if _, e := DecodeDecision("prompt", b); e == nil {
			t.Fatal("non-supported board escaped")
		}
	}
}

func TestDecisionRestoresOnlyBoundaryWhitespace(t *testing.T) {
	c := testConfig()
	for _, tc := range []struct {
		name, prompt string
		clauses      []string
		valid        bool
	}{
		{"sentence space", "First request. Second request.", []string{"First request.", "Second request."}, true},
		{"unicode boundary", "\tFirst.\n\u2003Second. \n", []string{"First.", "Second."}, true},
		{"already exact", "First. Second.", []string{"First. ", "Second."}, true},
		{"dropped negation", "First. Not battery powered.", []string{"First.", "battery powered."}, false},
		{"dropped punctuation", "3.3 V; 100 kHz", []string{"3.3 V", "100 kHz"}, false},
		{"changed internal spacing", "100 kHz", []string{"100kHz"}, false},
		{"trailing requirement", "First. Add relay.", []string{"First."}, false},
		{"reordered", "First. Second.", []string{"Second.", "First."}, false},
		{"duplicated", "First. Second.", []string{"First.", "First.", "Second."}, false},
		{"empty clause", "First. ", []string{"First.", " "}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Decision{Disposition: "supported", Message: "Supported family requirements.", Configuration: &c}
			for _, s := range tc.clauses {
				d.Clauses = append(d.Clauses, Clause{s, "supported", "Supported."})
			}
			b, _ := json.Marshal(d)
			got, err := DecodeDecision(tc.prompt, b)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if err == nil {
				var joined string
				for _, clause := range got.Clauses {
					joined += clause.Text
				}
				if joined != tc.prompt {
					t.Fatal("normalization failed exact original text preservation")
				}
			}
		})
	}
}

func TestLowCurrentRequiresExplicitUserScope(t *testing.T) {
	c := testConfig()
	c.Profile, c.TotalBusCapacitancePF = "low_current", 100
	for _, tc := range []struct {
		prompt    string
		supported bool
	}{
		{"A low-power controller, please.", false},
		{"Save energy in this pressure logger.", false},
		{"Improve battery life.", false},
		{"I need low current in the whole board.", false},
		{"Create a low-current pressure node.", false},
		{"Choose low_current.", true},
		{"Use the low-current bus profile.", true},
		{"Reduce I2C pull-up current, not whole-board power.", true},
		{"Fit 10-kilohm SDA and SCL pull-ups.", true},
		{"Use 10k pullups.", true},
		{"Use 10 kOhm pull-up resistors.", true},
		{"Use 110k pullups to save power.", false},
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			d := Decision{"supported", "Model chose low_current.", []Clause{{tc.prompt, "supported", "Model interpretation."}}, &c}
			b, _ := json.Marshal(d)
			got, err := DecodeDecision(tc.prompt, b)
			if err != nil {
				t.Fatal(err)
			}
			if tc.supported {
				if got.Disposition != "supported" || got.Configuration == nil {
					t.Fatal("explicit scope rejected")
				}
			} else {
				if got.Disposition != "clarify" || got.Configuration != nil || got.Clauses[0].Disposition != "clarify" || got.Clauses[0].Text != tc.prompt {
					t.Fatal("unspecified energy goal escaped clarification")
				}
			}
		})
	}
}

func TestRecordedLanguageResponsesAfterLocalCorrections(t *testing.T) {
	// These records are historical failures, not new first-shot successes.
	root := "../../specs/board-family-v1/evidence/live"
	for _, tc := range []struct{ run, id, disposition string }{
		{"acceptance-language-02", "nl-05", "supported"},
		{"acceptance-language-03", "ambiguous-02", "clarify"},
		{"acceptance-language-02", "nl-03", "unsupported"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			p := filepath.Join(root, tc.run, tc.id)
			prompt, err := os.ReadFile(p + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(p, "selection.json"))
			if err != nil {
				t.Fatal(err)
			}
			var s Selection
			if err = json.Unmarshal(b, &s); err != nil {
				t.Fatal(err)
			}
			raw := s.RawDecision
			if len(raw) == 0 {
				raw, err = json.Marshal(s.Decision)
				if err != nil {
					t.Fatal(err)
				}
			}
			d, err := DecodeDecision(string(prompt), raw)
			if err != nil {
				t.Fatal(err)
			}
			if d.Disposition != tc.disposition {
				t.Fatalf("got %s want %s", d.Disposition, tc.disposition)
			}
			if tc.disposition != "supported" && d.Configuration != nil {
				t.Fatal("non-supported replay retained configuration")
			}
			if tc.id == "nl-05" && (d.Configuration == nil || d.Configuration.Profile != "fast") {
				t.Fatal("correct cached selection lost")
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportEndpointAccountingAndNoRetries(t *testing.T) {
	calls := 0
	tr := &reservedTransport{Path: filepath.Join(t.TempDir(), "ledger.json"), Base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	bad, _ := http.NewRequest("POST", "https://example.org/", strings.NewReader("{}"))
	if _, e := tr.RoundTrip(bad); e == nil || calls != 0 {
		t.Fatal("sent outside approved endpoint")
	}
	good, _ := http.NewRequest("POST", "https://api.openai.com/v1/responses", strings.NewReader("{}"))
	r, e := tr.RoundTrip(good)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if tr.Index != 1 || calls != 1 {
		t.Fatal("unaccounted request")
	}
	if _, e = tr.RoundTrip(good); e == nil || calls != 1 {
		t.Fatal("retried request")
	}
}
