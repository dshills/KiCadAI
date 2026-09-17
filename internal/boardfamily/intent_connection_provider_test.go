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
)

func checkConnectionProviderRequest(t testing.TB, r *http.Request, prompt string) int {
	t.Helper()
	if r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" || r.ContentLength <= 0 || r.ContentLength > ConnectionEvidenceMaxRequestBytes {
		t.Fatal("endpoint or request bounds changed")
	}
	body, err := io.ReadAll(r.Body)
	if err = errors.Join(err, r.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("offline-connection-placeholder")) {
		t.Fatal("credential entered model input")
	}
	var payload struct {
		Model, Input              string
		MaxOutputTokens           int `json:"max_output_tokens"`
		Background, Store, Stream bool
		Text                      struct {
			Format struct {
				Type, Name string
				Strict     bool
				Schema     map[string]any
			}
		}
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != SelectionModel || payload.MaxOutputTokens != 1600 || payload.Background || payload.Store || !payload.Stream || !payload.Text.Format.Strict || payload.Text.Format.Type != "json_schema" || payload.Text.Format.Name != ConnectionEvidenceSchemaName {
		t.Fatal("pinned request contract changed")
	}
	var input struct {
		Prompt            string
		CapabilityContext string `json:"capability_context"`
		Attempt           int
		Diagnostics       []any
	}
	if err := json.Unmarshal([]byte(payload.Input), &input); err != nil {
		t.Fatal(err)
	}
	want, _, err := prepareConnectionGenerateRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if input.Prompt != want.Prompt {
		t.Fatal("source prompt differs from the prepared request")
	}
	if input.CapabilityContext != want.CapabilityContext {
		t.Fatal("extraction instructions differ from the prepared request")
	}
	if input.Attempt != 1 || len(input.Diagnostics) != 0 {
		t.Fatal("retry or diagnostics entered the first-attempt request")
	}
	if !sameReferencedJSON(want.OutputSchema, payload.Text.Format.Schema) {
		t.Fatal("schema differs from the prepared request")
	}
	return len(body)
}

func TestConnectionProviderBoundsAndJournal(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-connection-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override-model")
	prompts := map[string]string{"max-text": strings.Repeat("x", 2000), "escaped": strings.Repeat("\x00", 2000), "quotes": strings.Repeat(`"\`, 1000), "unicode": strings.Repeat("界", 666), "inventory": "Use BMP280 or SHT31 with " + strings.Repeat("0 C, 0 C, 0 C, 0 C. ", 32)}
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("original corpus changed")
	}
	for _, c := range corpus.Cases {
		prompts[c.ID] = c.Prompt
	}
	inventory := prompts["inventory"]
	prompts["inventory-escaped-padding"] = inventory[:len(inventory)-2] + strings.Repeat("\x00", 2000-len(inventory)) + ". "
	minimum, maximum := ConnectionEvidenceMaxRequestBytes, 0
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			ledger, journal := filepath.Join(root, "ledger.json"), filepath.Join(root, "journal")
			calls := 0
			s, err := InterpretConnectionWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-connection-bounds", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				size := checkConnectionProviderRequest(t, r, prompt)
				minimum, maximum = min(minimum, size), max(maximum, size)
				return referencedProviderResponse(t, connectionRaw(t), "refusal", "offline-connection-"+name), nil
			}), journal)
			if err == nil || s.Outcome != "provider_refusal" || calls != 1 || s.Decision.Configuration != nil || s.AdmissionVersion != ConnectionEvidenceVersion {
				t.Fatal("wrong synthetic refusal", s.Outcome, err, calls)
			}
			a, err := InspectConnectionJournal(journal)
			if err != nil || a.Version != "connection-journal-audit-1" || len(a.FilesSHA256) != 8 {
				t.Fatal("incomplete journal", err)
			}
			for _, inspect := range []func(string) (ReferencedJournalAudit, error){InspectOwnedJournal, InspectReferencedJournal} {
				if _, err := inspect(journal); err == nil {
					t.Fatal("legacy inspector accepted new journal")
				}
			}
		})
	}
	t.Logf("%d synthetic request fixtures: %d-%d HTTP bytes; no network", len(prompts), minimum, maximum)
	if (int64(ConnectionEvidenceMaxRequestBytes)*4+1600*16+9)/10 >= RequestReserveMicroUSD {
		t.Fatal("bound exceeds reservation arithmetic")
	}
}

func TestConnectionJournalAuditRejectsTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, connectionProtocol)
}

func TestConnectionProviderFailuresStayTerminal(t *testing.T) {
	for _, mode := range []string{"complete", "invalid", "old-prototype", "refusal", "server-error", "incomplete", "wrong-model", "transport-failure", "rate-limit", "close-error"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-connection-placeholder")
			prompt := "Use BMP280 standard with a wired connection."
			raw := connectionRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"), ownedFact("connection", "wired", "required", "c0"))
			if mode == "invalid" {
				raw = []byte(`{"version":"wrong","facts":[]}`)
			}
			if mode == "old-prototype" {
				raw = []byte(`{"version":"5-connection-evidence-offline","facts":[]}`)
			}
			root := t.TempDir()
			ledger, journal := filepath.Join(root, "ledger.json"), filepath.Join(root, "journal")
			policy := LedgerPolicy{"offline-connection-failure", 1, 50000}
			calls := 0
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkConnectionProviderRequest(t, r, prompt)
				if mode == "transport-failure" {
					return nil, errors.New("synthetic transport failure")
				}
				response := referencedProviderResponse(t, raw, mode, "offline-connection-"+mode)
				if mode == "server-error" {
					response.Body = io.NopCloser(strings.NewReader("event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_error\",\"message\":\"Synthetic failure\"}}\n\n"))
				}
				if mode == "rate-limit" {
					response.StatusCode = 429
				}
				if mode == "close-error" {
					response.Body = &ownedCloseFailure{ReadCloser: response.Body}
				}
				return response, nil
			})
			s, err := InterpretConnectionWithJournal(context.Background(), prompt, ledger, policy, transport, journal)
			if calls != 1 || (err == nil) != (mode == "complete") || s.AdmissionVersion != ConnectionEvidenceVersion {
				t.Fatal("wrong failure behavior", s.Outcome, err, calls)
			}
			if mode != "complete" && s.Decision.Configuration != nil {
				t.Fatal("failure permits generation")
			}
			if !journalSelectionsEqual(s, readJournalSelection(t, journal)) {
				t.Fatal("returned/durable selection mismatch")
			}
			auditable := mode == "complete" || mode == "invalid" || mode == "old-prototype" || mode == "refusal"
			_, auditErr := InspectConnectionJournal(journal)
			if (auditErr == nil) != auditable {
				t.Fatal("wrong audit result", auditErr)
			}
			l := readReferencedTestLedger(t, ledger)
			status := "failed_or_unknown"
			if auditable || mode == "wrong-model" {
				status = "completed"
			}
			if len(l.Entries) != 1 || l.Entries[0].Status != status {
				t.Fatal("failure accounting lost", l)
			}
			_, err = InterpretConnectionWithJournal(context.Background(), prompt, ledger, policy, transport, journal)
			if err == nil || calls != 1 {
				t.Fatal("journal reused or automatic retry")
			}
		})
	}
}
