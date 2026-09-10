package practicalboardeval

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/aiprovider"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReservationAndReconciliation(t *testing.T) {
	root := t.TempDir()
	first, err := Reserve(root, "baseline", "synthetic", []byte("{}"))
	if err != nil || first.Number != 1 {
		t.Fatalf("reserve: %#v %v", first, err)
	}
	recorder := RecordingTransport{Journal: root, Last: &first}
	if err := recorder.Reconcile(aiprovider.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}); err != nil {
		t.Fatal(err)
	}
	second, err := Reserve(root, "final", "synthetic", []byte("{}"))
	if err != nil || second.Number != 2 {
		t.Fatalf("reserve: %#v %v", second, err)
	}
	if _, err := Reserve(root, "baseline", "synthetic", make([]byte, MaxRequestBytes+1)); err == nil {
		t.Fatal("oversize request allowed")
	}
	if _, err := Reserve(root, "paired", "synthetic", []byte("{}")); err == nil {
		t.Fatal("paired replay allowed API call")
	}
}

func TestRequestCapStopsBeforeDispatch(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxCampaignRequests; i++ {
		if _, err := Reserve(root, "baseline", "synthetic", []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Reserve(root, "baseline", "synthetic", []byte("{}")); err == nil {
		t.Fatal("campaign cap exceeded")
	}
}

func TestSpendCapAndMissingUsageStayConservative(t *testing.T) {
	root := t.TempDir()
	first, err := Reserve(root, "baseline", "synthetic", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := RecordingTransport{Journal: root, Last: &first}
	if err := recorder.Reconcile(aiprovider.Usage{}); err != nil {
		t.Fatal(err)
	}
	var reconciled Reconciliation
	if err := ReadJSON(filepath.Join(root, "001.usage.json"), &reconciled); err != nil {
		t.Fatal(err)
	}
	if reconciled.UsageAvailable || reconciled.ChargedOrReservedUSD != first.ReservedUSD {
		t.Fatal("missing usage released the reservation")
	}
	if err := WriteJSON(filepath.Join(root, "002.reservation.json"), Reservation{Number: 2, Campaign: "baseline", ReservedUSD: MaxSpendUSD}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reserve(root, "final", "synthetic", []byte("{}")); err == nil {
		t.Fatal("spend cap exceeded")
	}
}

func TestUnsafeEndpointCannotDispatch(t *testing.T) {
	called := false
	r := RecordingTransport{Base: transportFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, nil })}
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/responses", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RoundTrip(req); err == nil || called {
		t.Fatal("unapproved endpoint reached transport")
	}
}

func TestTransportCapturesOnlySafeEvidence(t *testing.T) {
	output, journal := t.TempDir(), t.TempDir()
	secret := "sk-synthetic-private-key"
	called := false
	recorder := RecordingTransport{Journal: journal, Output: output, Campaign: "baseline", CaseID: "synthetic", Secret: secret, Base: transportFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		if req.Header.Get("Authorization") != "Bearer "+secret {
			t.Fatal("authentication not delivered")
		}
		return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"error":"` + secret + `"}`))}, nil
	})}
	body := `{"model":"gpt-5.6-sol","max_output_tokens":16384,"store":false,"stream":true,"background":false}`
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	response, err := recorder.RoundTrip(req)
	if err != nil || !called {
		t.Fatalf("dispatch: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := Inventory(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(output, file.Path))
		if err != nil || bytes.Contains(data, []byte(secret)) || bytes.Contains(data, []byte("Authorization")) {
			t.Fatalf("unsafe capture %s: %v", file.Path, err)
		}
	}
}
