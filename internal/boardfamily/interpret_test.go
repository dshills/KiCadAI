package boardfamily

import (
	"encoding/json"
	"io"
	"net/http"
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
