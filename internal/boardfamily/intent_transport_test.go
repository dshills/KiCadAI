package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntentProviderPathOffline(t *testing.T) {
	for _, mode := range []string{"supported", "unsupported", "malformed", "transport-failure", "incomplete", "wrong-model", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			// A fake value with an in-memory RoundTripper. No socket or DNS call
			// is possible, and the user's current API key is never inspected.
			t.Setenv("OPENAI_API_KEY", "offline-test-placeholder")
			prompt := "Please use BMP280 with the fast profile. Thanks!"
			facts := []RequirementFact{choiceFact("sensor", "BMP280", "required", "BMP280"), choiceFact("profile", "fast", "required", "fast profile")}
			if mode == "unsupported" {
				prompt += " Include wireless telemetry."
				facts = append(facts, choiceFact("feature", "wireless_operation", "required", "wireless telemetry"))
			}
			raw := syntheticIntent(t, prompt, facts...)
			if mode == "malformed" {
				raw = []byte(`{"version":"2","clauses":[]}`)
			}
			ledger := filepath.Join(t.TempDir(), "ledger.json")
			policy := LedgerPolicy{"offline-typed-intent-test", 1, 50000}
			if mode == "exhausted" {
				if _, err := reserveWithPolicy(ledger, policy); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls != 1 || r.URL.String() != "https://api.openai.com/v1/responses" || r.Method != "POST" || r.ContentLength > 24000 {
					t.Fatal("transport or byte bound changed")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(body, []byte("offline-test-placeholder")) {
					t.Fatal("credential appears in model payload")
				}
				var payload struct {
					Model, Input              string
					MaxOutputTokens           int `json:"max_output_tokens"`
					Background, Store, Stream bool
					Text                      struct {
						Format struct {
							Name   string
							Strict bool
							Schema map[string]any
						}
					}
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Model != SelectionModel || payload.MaxOutputTokens != 1600 || payload.Background || payload.Store || !payload.Stream || !payload.Text.Format.Strict || payload.Text.Format.Name != IntentSchemaName {
					t.Fatal("selector contract or transport flags changed")
				}
				actualSchema, _ := json.Marshal(payload.Text.Format.Schema)
				wantSchema, _ := json.Marshal(IntentSchema())
				if !bytes.Equal(actualSchema, wantSchema) {
					t.Fatal("schema differs from declared successor contract")
				}
				var input struct {
					Prompt            string
					CapabilityContext string `json:"capability_context"`
					Attempt           int
				}
				if err := json.Unmarshal([]byte(payload.Input), &input); err != nil {
					t.Fatal(err)
				}
				wantPrompt, _, _ := prepareIntentRequest(prompt)
				if input.Prompt != wantPrompt || input.CapabilityContext != IntentLanguageContext() || input.Attempt != 1 {
					t.Fatal("original request/clauses/context not sent as declared")
				}
				if mode == "transport-failure" {
					return nil, errors.New("synthetic offline transport failure")
				}
				status, details, model := "completed", "null", SelectionModel
				if mode == "incomplete" {
					status, details = "incomplete", `{"reason":"max_output_tokens"}`
				}
				if mode == "wrong-model" {
					model = "not-the-pinned-model"
				}
				response := fmt.Sprintf(`{"id":"offline-response","status":%q,"model":%q,"error":null,"incomplete_details":%s,"output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":100,"output_tokens":200,"total_tokens":300}}`, status, model, details, raw)
				// Exercise the actual streamed response decoder with one final event.
				stream := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
			})
			s, err := interpretWithTransport(context.Background(), prompt, ledger, policy, base)
			if s.OriginalRequest != prompt || s.AdmissionVersion != IntentAdmissionVersion || len(s.RequestClauses) == 0 || len(s.RawDecision) != 0 {
				t.Fatalf("lost/mixed source and provider evidence: %+v", s)
			}
			wantCalls := 1
			if mode == "exhausted" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d expected=%d", calls, wantCalls)
			}
			if mode == "supported" || mode == "unsupported" {
				if err != nil || s.Decision.Disposition != mode || !bytes.Equal(s.RawIntent, raw) || s.ResponseID != "offline-response" || s.Usage.TotalTokens != 300 {
					t.Fatalf("unexpected provider result: %+v %v", s, err)
				}
			} else if err == nil || s.Decision.Configuration != nil {
				t.Fatalf("failure permitted generation: %+v %v", s, err)
			}
			b, err := os.ReadFile(ledger)
			if err != nil {
				t.Fatal(err)
			}
			var l Ledger
			if err := json.Unmarshal(b, &l); err != nil {
				t.Fatal(err)
			}
			if len(l.Entries) != 1 || l.Entries[0].ReserveMicroUSD != RequestReserveMicroUSD {
				t.Fatal("request allowance refunded or retried")
			}
			wantStatus := "completed"
			if mode == "transport-failure" || mode == "incomplete" {
				wantStatus = "failed_or_unknown"
			}
			if mode != "exhausted" && l.Entries[0].Status != wantStatus {
				t.Fatalf("ledger=%+v", l)
			}
		})
	}
}

func TestIntentRequestPayloadByteBound(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-test-placeholder")
	// Escape-heavy allowed input can enlarge nested JSON. The unchanged 24k
	// guard must refuse it before reserving or sending, not increase the cap.
	for _, prompt := range []string{strings.Repeat("x", 2000), strings.Repeat("\"", 2000)} {
		calls := 0
		ledger := filepath.Join(t.TempDir(), "ledger.json")
		base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			t.Logf("bounded payload: %d bytes for %d input bytes", r.ContentLength, len(prompt))
			return nil, errors.New("synthetic offline stop")
		})
		_, err := interpretWithTransport(context.Background(), prompt, ledger, LedgerPolicy{"offline-byte-test", 1, 50000}, base)
		if err == nil {
			t.Fatal("synthetic stop ignored")
		}
		if prompt[0] == 'x' && calls != 1 {
			t.Fatalf("ordinary maximum-size input failed before transport: %v", err)
		}
		if calls == 0 {
			boundFailure := false
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				boundFailure = boundFailure || strings.Contains(cause.Error(), "outside the accounted bound")
			}
			if !boundFailure {
				t.Fatal("request did not reach the accounted byte guard")
			}
			if _, err := os.Stat(ledger); !os.IsNotExist(err) {
				t.Fatal("oversize request reserved allowance")
			}
		}
	}
}
