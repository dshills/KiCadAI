package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func capturedIndexedEnvelope(t *testing.T) (*ReferencedProviderEvidence, string) {
	t.Helper()
	root := filepath.Join("..", "..", "specs", "board-family-v2", "indexed-evaluation-03", "batch", "useful-01")
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	request, body := read("journal/request/body.bin"), read("journal/response.bin")
	if referencedDigest(request) != "b2dfef3a75d7b166856e2646c8b48d8df8b9be8586a3f44afc9984cdfd5701f1" || referencedDigest(body) != "9a8c732de3864645b1e8f20893998076cd4da10e1b2a85f3ab80e02ccb925017" {
		t.Fatal("historical captured request/response changed")
	}
	return &ReferencedProviderEvidence{RequestBody: request, RequestSHA256: referencedDigest(request), StatusCode: 200,
		ContentType: "text/event-stream; charset=utf-8", Body: body, BodySHA256: referencedDigest(body), BytesRead: len(body), EOFObserved: true, TransportStarted: true}, string(read("prompt.txt"))
}

func TestReferencedEnvelopeCapturedSchema(t *testing.T) {
	e, _ := capturedIndexedEnvelope(t)
	terminal, err := inspectReferencedEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Model != SelectionModel || terminal.Status != "completed" || terminal.ID != "resp_0af9d97ec5c58378016aa913babff887d186a396f729a83fc4" || terminal.Usage == nil || *terminal.Usage.Input != 2596 || *terminal.Usage.Output != 100 || *terminal.Usage.Total != 2696 {
		t.Fatalf("captured terminal metadata changed: %+v", terminal)
	}
}

func nestedEnvelopeJSON(depth int) []byte {
	return []byte(strings.Repeat(`{"x":`, depth) + "0" + strings.Repeat("}", depth))
}

func TestIntentJSONDepthRemainsTwelve(t *testing.T) {
	if err := validateIntentJSON(nestedEnvelopeJSON(12)); err != nil {
		t.Fatal(err)
	}
	if err := validateIntentJSON(nestedEnvelopeJSON(13)); err == nil {
		t.Fatal("intent-output nesting bound was relaxed")
	}
}

func TestReferencedEnvelopeJSONLimits(t *testing.T) {
	for _, depth := range []int{12, 13, 64} {
		if err := validateReferencedEnvelopeJSON(nestedEnvelopeJSON(depth)); err != nil {
			t.Fatalf("valid bounded depth %d: %v", depth, err)
		}
	}
	for name, raw := range map[string][]byte{
		"depth-65":         nestedEnvelopeJSON(65),
		"duplicate-root":   []byte(`{"model":"one","model":"two"}`),
		"duplicate-nested": []byte(`{"response":{"usage":{"total_tokens":1,"total_tokens":2}}}`),
		"duplicate-deep":   []byte(strings.Repeat(`{"x":`, 20) + `{"model":1,"model":2}` + strings.Repeat("}", 20)),
		"invalid-utf8":     {'"', 0xff, '"'},
		"trailing-json":    []byte(`{} {}`),
		"truncated":        []byte(`{"response":`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReferencedEnvelopeJSON(raw); err == nil {
				t.Fatal("malformed, ambiguous or over-depth envelope accepted")
			}
		})
	}
}

func TestReferencedEnvelopeCapturedReplay(t *testing.T) {
	// Feed the unchanged saved response to a newly generated offline request.
	// The request contract may evolve; this is fault injection, not a rerun or
	// faithful request/response pair from a new model trial. The separate schema
	// regression proves the captured invented sensor is no longer permitted.
	t.Setenv("OPENAI_API_KEY", "offline-captured-envelope-placeholder")
	e, prompt := capturedIndexedEnvelope(t)
	ledger := filepath.Join(t.TempDir(), "ledger.json")
	calls := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		checkReferencedProviderRequest(t, r, prompt)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{e.ContentType}}, Body: io.NopCloser(bytes.NewReader(e.Body))}, nil
	})
	s, err := InterpretReferencedWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-captured-envelope", 1, 50000}, transport, ledger+".journal")
	if s.Outcome != "invalid_extraction" || calls != 1 || err == nil || !strings.Contains(err.Error(), "sensor identity is not grounded") {
		t.Fatalf("captured envelope was rejected: outcome=%s calls=%d err=%v", s.Outcome, calls, err)
	}
	if s.Decision.Configuration != nil || s.Usage.TotalTokens != 2696 || !bytes.Equal(s.ProviderEvidence.Body, e.Body) {
		t.Fatal("semantic failure, terminal metadata, or original bytes were lost")
	}
	var raw ReferencedIntent
	if err := json.Unmarshal(s.RawIntent, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Facts) != 4 || raw.Facts[0].Value != "BMP280" || raw.Facts[3].Value != "wireless_operation" || raw.Facts[3].State != "required" {
		t.Fatal("unfaithful facts were silently repaired or dropped")
	}
	audit, auditErr := InspectReferencedJournal(ledger + ".journal")
	if auditErr != nil || audit.Outcome != s.Outcome {
		t.Fatalf("fresh offline journal cannot be audited: %v", auditErr)
	}
	l := readReferencedTestLedger(t, ledger)
	if len(l.Entries) != 1 || l.Entries[0].Status != "completed" || l.Entries[0].InputTokens != 2596 || l.Entries[0].OutputTokens != 100 {
		t.Fatalf("fresh offline accounting mismatch: %+v", l)
	}
	t.Logf("offline-only saved-response injection into request revision %s: outcome=%s disposition=%s error=%v; historical batch is unchanged", ReferenceIntentRequestRevision, s.Outcome, s.Decision.Disposition, err)
}
