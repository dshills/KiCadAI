package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
)

type referencedFailingReader struct{}

func (referencedFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic read failure")
}

type referencedCloseFailure struct{ io.Reader }

func (referencedCloseFailure) Close() error { return errors.New("synthetic close failure") }

func TestReferencedEvidencePreservesFailures(t *testing.T) {
	testReferencedEvidencePreservesFailures(t, false)
}

func TestReferencedJournalPreservesFailures(t *testing.T) {
	testReferencedEvidencePreservesFailures(t, true)
}

func testReferencedEvidencePreservesFailures(t *testing.T, journaled bool) {
	for _, mode := range []string{"complete", "malformed-output", "refusal", "incomplete", "failed", "partial-read", "close-error", "oversized", "http-error", "transport-error", "missing-model", "missing-id", "missing-usage", "missing-total", "bad-usage", "duplicate-model", "duplicate-event-type", "trailing-terminal", "status-mismatch", "invalid-utf8", "json-response"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
			prompt := "Please use BMP280."
			raw := syntheticReferencedIntent(t, referencedChoice("sensor", "BMP280", "required", 0))
			if mode == "malformed-output" {
				raw = []byte(`{"broken":`)
			}
			status := "completed"
			if mode == "incomplete" || mode == "failed" {
				status = mode
			}
			content := []any{map[string]any{"type": "output_text", "text": string(raw)}}
			if mode == "refusal" {
				content = []any{map[string]any{"type": "refusal", "refusal": "Synthetic retained refusal."}}
			}
			usage := map[string]any{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}
			response := map[string]any{"id": "offline-evidence", "model": SelectionModel, "status": status, "error": nil, "incomplete_details": nil,
				"output": []any{map[string]any{"type": "message", "status": status, "content": content}}, "usage": usage}
			switch mode {
			case "missing-model":
				delete(response, "model")
			case "missing-id":
				delete(response, "id")
			case "missing-usage":
				delete(response, "usage")
			case "missing-total":
				delete(usage, "total_tokens")
			case "bad-usage":
				usage["total_tokens"] = 301
			case "failed":
				response["error"] = map[string]any{"code": "server_error"}
			}
			terminal, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "duplicate-model" {
				terminal = bytes.Replace(terminal, []byte(`"model":`), []byte(`"model":"other","model":`), 1)
			}
			event := "response." + status
			payload := `{"type":"` + event + `","response":` + string(terminal) + `}`
			if mode == "duplicate-event-type" {
				payload = strings.Replace(payload, `"type":`, `"type":"response.failed","type":`, 1)
			}
			wire := []byte("event: " + event + "\ndata: " + payload + "\n\n")
			contentType, httpStatus := "text/event-stream", 200
			switch mode {
			case "json-response":
				wire, contentType = terminal, "application/json"
			case "oversized":
				wire = append(wire, bytes.Repeat([]byte(" "), aiprovider.MaxResponseBytes)...)
			case "trailing-terminal":
				wire = append(wire, wire...)
			case "status-mismatch":
				wire = bytes.Replace(wire, []byte(`"status":"completed"`), []byte(`"status":"incomplete"`), -1)
			case "http-error":
				httpStatus, wire = 429, []byte(`{"error":{"code":"rate_limit"}}`)
			case "invalid-utf8":
				wire = bytes.Replace(wire, []byte("offline-evidence"), []byte{'x', 0xff, 'y'}, 1)
			case "partial-read":
				wire = wire[:len(wire)-20]
			}
			ledger := filepath.Join(t.TempDir(), "ledger.json")
			calls := 0
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkReferencedProviderRequest(t, r, prompt)
				if mode == "transport-error" {
					return nil, errors.New("synthetic transport failure")
				}
				var reader io.Reader = bytes.NewReader(wire)
				if mode == "partial-read" {
					reader = io.MultiReader(reader, referencedFailingReader{})
				}
				var body io.ReadCloser = io.NopCloser(reader)
				if mode == "close-error" {
					body = referencedCloseFailure{reader}
				}
				return &http.Response{StatusCode: httpStatus, Header: http.Header{"Content-Type": []string{contentType}, "Set-Cookie": []string{"must-not-be-recorded"}}, Body: body}, nil
			})
			var s ReferencedSelection
			if journaled {
				s, err = InterpretReferencedWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-evidence", 1, 50000}, transport, ledger+".evidence")
			} else {
				s, err = interpretReferencedWithTransport(context.Background(), prompt, ledger, LedgerPolicy{"offline-evidence", 1, 50000}, transport)
			}
			wantSuccess := member(mode, "complete", "json-response")
			if calls != 1 || (err == nil) != wantSuccess || (s.Decision.Configuration != nil) != wantSuccess {
				t.Fatalf("unexpected outcome %s, %v, calls %d", s.Outcome, err, calls)
			}
			e := s.ProviderEvidence
			if e == nil || !json.Valid(e.RequestBody) || referencedDigest(e.RequestBody) != e.RequestSHA256 || referencedDigest(e.Body) != e.BodySHA256 {
				t.Fatal("recorded bodies or hashes are missing")
			}
			wantBody := wire
			if mode == "transport-error" {
				wantBody = nil
			} else if mode == "oversized" {
				wantBody = wire[:aiprovider.MaxResponseBytes]
			}
			if !bytes.Equal(e.Body, wantBody) {
				t.Fatal("response bytes changed or disappeared on a failure path")
			}
			if journaled {
				root := ledger + ".evidence"
				saved := readJournalSelection(t, root)
				b, readErr := os.ReadFile(filepath.Join(root, "response.bin"))
				if !journalSelectionsEqual(saved, s) || readErr != nil || !bytes.Equal(b, wantBody) || e.PersistenceError {
					t.Fatal("durable evidence differs from recorded response")
				}
				checkJournalFile(t, root, "request/receipt.json", true)
				checkJournalFile(t, root, "response/receipt.json", true)
				audit, auditErr := InspectReferencedJournal(root)
				wantAudit := member(mode, "complete", "json-response", "malformed-output", "refusal")
				if (auditErr == nil) != wantAudit || (wantAudit && (audit.Outcome != s.Outcome || audit.Selection.ResponseID != s.ResponseID)) {
					t.Fatalf("wrong journal audit for %s: %v", mode, auditErr)
				}
			}
			if e.Truncated != (mode == "oversized") || e.ReadError != (mode == "partial-read") || e.CloseError != (mode == "close-error") || e.TransportError != (mode == "transport-error") || e.EOFObserved == member(mode, "partial-read", "oversized", "transport-error") {
				t.Fatal("partial/failed response was described as complete")
			}
			encoded, marshalErr := json.Marshal(s)
			if marshalErr != nil || bytes.Contains(encoded, []byte("offline-referenced-placeholder")) || bytes.Contains(encoded, []byte("must-not-be-recorded")) {
				t.Fatal("selection cannot be saved safely")
			}
			var restored ReferencedSelection
			if json.Unmarshal(encoded, &restored) != nil || !bytes.Equal(restored.ProviderEvidence.Body, wantBody) {
				t.Fatal("JSON persistence lost non-JSON response bytes")
			}
			wantOutcome, wantStatus := "invalid_response_evidence", "failed_or_unknown"
			switch mode {
			case "complete", "json-response":
				wantOutcome, wantStatus = "decision", "completed"
			case "malformed-output":
				wantOutcome, wantStatus = "invalid_extraction", "completed"
			case "refusal":
				wantOutcome, wantStatus = "provider_refusal", "completed"
			case "incomplete":
				wantOutcome = "provider_incomplete"
			case "failed", "http-error", "transport-error":
				wantOutcome = "provider_failed_or_unknown"
			}
			l := readReferencedTestLedger(t, ledger)
			if s.Outcome != wantOutcome || len(l.Entries) != 1 || l.Entries[0].Status != wantStatus || l.Entries[0].ReserveMicroUSD != 50000 {
				t.Fatalf("incorrect outcome/accounting: %s %+v", s.Outcome, l)
			}
			if member(mode, "complete", "json-response", "malformed-output", "refusal", "incomplete", "failed") && (s.ResponseID != "offline-evidence" || s.Model != SelectionModel || s.Usage.TotalTokens != 300 || l.Entries[0].EstimatedMicroUSD != 360) {
				t.Fatal("verified terminal metadata was lost")
			}
		})
	}
}

func TestReferencedEvidenceRejectsPostCaptureTampering(t *testing.T) {
	raw := syntheticReferencedIntent(t, referencedChoice("sensor", "BMP280", "required", 0))
	r := referencedProviderResponse(t, raw, "complete", "offline-tamper")
	body, err := io.ReadAll(r.Body)
	if err = errors.Join(err, r.Body.Close()); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"request", "response", "length", "eof", "close"} {
		t.Run(mode, func(t *testing.T) {
			e := &ReferencedProviderEvidence{RequestBody: []byte("{}"), RequestSHA256: referencedDigest([]byte("{}")), StatusCode: 200,
				ContentType: "text/event-stream", Body: append([]byte(nil), body...), BodySHA256: referencedDigest(body), BytesRead: len(body), EOFObserved: true}
			switch mode {
			case "request":
				e.RequestBody[0] = '['
			case "response":
				e.Body[0] = 'x'
			case "length":
				e.BytesRead++
			case "eof":
				e.EOFObserved = false
			case "close":
				e.CloseError = true
			}
			if _, err := inspectReferencedEvidence(e); err == nil {
				t.Fatal("tampered or incomplete evidence accepted")
			}
		})
	}
}
