package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kicadai/internal/aiprovider"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func checkOwnedProviderRequest(t testing.TB, r *http.Request, prompt string) int {
	t.Helper()
	if r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" || r.ContentLength <= 0 || r.ContentLength > OwnedEvidenceMaxRequestBytes {
		t.Fatal("endpoint/method/byte bounds changed")
	}
	body, err := io.ReadAll(r.Body)
	if closeErr := r.Body.Close(); err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr))
	}
	if bytes.Contains(body, []byte("offline-owned-placeholder")) {
		t.Fatal("credential leaked into model input")
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
	if payload.Model != SelectionModel || payload.MaxOutputTokens != 1600 || payload.Background || payload.Store || !payload.Stream || !payload.Text.Format.Strict || payload.Text.Format.Type != "json_schema" || payload.Text.Format.Name != OwnedEvidenceSchemaName {
		t.Fatal("provider model/streaming/schema/token restrictions changed")
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
	want, _, err := prepareOwnedGenerateRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if input.Prompt != want.Prompt || input.CapabilityContext != want.CapabilityContext || input.Attempt != 1 || len(input.Diagnostics) != 0 {
		t.Fatal("source/context changed or a recovery attempt was requested")
	}
	a, _ := json.Marshal(payload.Text.Format.Schema)
	b, _ := json.Marshal(want.OutputSchema)
	if !bytes.Equal(a, b) {
		t.Fatal("request-specific provider schema differs from its source contract")
	}
	return len(body)
}

func TestOwnedProviderRequestBoundsAndAccounting(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-owned-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override-pinned-model")
	prompts := map[string]string{
		"max-inventory": "Use BMP280 or SHT31 with " + strings.Repeat("0 C, 0 C, 0 C, 0 C. ", 32),
		"max-text":      strings.Repeat("x", 2000),
		"escaped-text":  strings.Repeat("\x00", 2000),
		"quotes":        strings.Repeat(`"\`, 1000),
		"unicode":       strings.Repeat("界", 666),
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID     string
			Prompt string
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("unexpected corpus")
	}
	for _, c := range corpus.Cases {
		prompts[c.ID] = c.Prompt
	}
	inventory := prompts["max-inventory"]
	prompts["inventory-escaped-padding"] = inventory[:len(inventory)-2] + strings.Repeat("\x00", 2000-len(inventory)) + ". "
	minimum, maximum := OwnedEvidenceMaxRequestBytes, 0
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			ledger, journal := filepath.Join(root, "ledger.json"), filepath.Join(root, "journal")
			calls := 0
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				size := checkOwnedProviderRequest(t, r, prompt)
				minimum, maximum = min(minimum, size), max(maximum, size)
				t.Logf("actual encoded request: %d bytes; synthetic provider refusal", size)
				return referencedProviderResponse(t, ownedRaw(t), "refusal", "offline-owned-"+name), nil
			})
			s, err := InterpretOwnedWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-owned-bound", 1, 50000}, transport, journal)
			if err == nil || s.Outcome != "provider_refusal" || calls != 1 || s.Decision.Configuration != nil || s.AdmissionVersion != OwnedEvidenceVersion {
				t.Fatalf("wrong synthetic refusal: outcome=%s calls=%d error=%v", s.Outcome, calls, err)
			}
			if _, err := InspectOwnedJournal(journal); err != nil {
				t.Fatal("full request replay failed", err)
			}
			l := readReferencedTestLedger(t, ledger)
			if len(l.Entries) != 1 || l.Entries[0].Status != "completed" || l.Entries[0].ReserveMicroUSD != 50000 {
				t.Fatal("policy or accounting changed")
			}
		})
	}
	t.Logf("all %d request fixtures: %d–%d encoded bytes (not token/cost/latency observations)", len(prompts), minimum, maximum)
	// Recorded accounting arithmetic, not a current external price quote.
	if (int64(OwnedEvidenceMaxRequestBytes)*4+1600*16+9)/10 >= RequestReserveMicroUSD {
		t.Fatal("byte bound exceeds reservation")
	}
}

func TestOwnedProviderFailuresRemainRecorded(t *testing.T) {
	for _, mode := range []string{"complete", "invalid-extraction", "malformed-json", "refusal", "incomplete", "wrong-model", "transport-failure", "rate-limit", "close-error"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-owned-placeholder")
			prompt := "Please use BMP280 with standard profile."
			raw := ownedRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"))
			if mode == "invalid-extraction" {
				raw = []byte(`{"version":"wrong","facts":[]}`)
			}
			if mode == "malformed-json" {
				raw = []byte(`{"broken":`)
			}
			dir := t.TempDir()
			ledger, journal := filepath.Join(dir, "ledger.json"), filepath.Join(dir, "journal")
			calls := 0
			s, err := InterpretOwnedWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-owned-failure", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkOwnedProviderRequest(t, r, prompt)
				if mode == "transport-failure" {
					return nil, errors.New("synthetic transport failure")
				}
				response := referencedProviderResponse(t, raw, mode, "offline-owned-"+mode)
				if mode == "rate-limit" {
					response.StatusCode = 429
				}
				if mode == "close-error" {
					response.Body = &ownedCloseFailure{ReadCloser: response.Body}
				}
				return response, nil
			}), journal)
			if calls != 1 || (err == nil) != (mode == "complete") || s.AdmissionVersion != OwnedEvidenceVersion {
				t.Fatal("wrong failure/count/version", mode, s.Outcome, err, calls)
			}
			if mode != "complete" && s.Decision.Configuration != nil {
				t.Fatal("failed extraction authorizes generation")
			}
			if !journalSelectionsEqual(s, readJournalSelection(t, journal)) {
				t.Fatal("returned and durable selections differ")
			}
			l := readReferencedTestLedger(t, ledger)
			status := "completed"
			if mode == "transport-failure" || mode == "incomplete" || mode == "rate-limit" || mode == "close-error" {
				status = "failed_or_unknown"
			}
			if len(l.Entries) != 1 || l.Entries[0].Status != status {
				t.Fatal("failure lost accounting", l)
			}
			_, auditErr := InspectOwnedJournal(journal)
			auditable := mode == "complete" || mode == "invalid-extraction" || mode == "malformed-json" || mode == "refusal"
			if (auditErr == nil) != auditable {
				t.Fatal("wrong byte audit outcome", mode, auditErr)
			}
			// Existing evidence is not reusable authority: no second physical call.
			_, retryErr := InterpretOwnedWithJournal(context.Background(), prompt, ledger, LedgerPolicy{"offline-owned-failure", 1, 50000}, roundTripperFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not run") }), journal)
			if retryErr == nil || calls != 1 {
				t.Fatal("existing journal reused")
			}
		})
	}
}

type ownedCloseFailure struct{ io.ReadCloser }

func (r *ownedCloseFailure) Close() error {
	return errors.Join(r.ReadCloser.Close(), errors.New("synthetic close failure"))
}

func TestOwnedProviderDoesNotRewriteIndexedRequestContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID string } }
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 14 {
		t.Fatal("unexpected history")
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "indexed-evaluation-04", "batch", c.ID, "journal", "selection", "selection.json"))
			if err != nil {
				t.Fatal(err)
			}
			var s ReferencedSelection
			if err := json.Unmarshal(b, &s); err != nil {
				t.Fatal(err)
			}
			request, _, err := prepareReferencedGenerateRequest(s.OriginalRequest)
			if err != nil {
				t.Fatal(err)
			}
			replay := &referencedAuditReplay{evidence: s.ProviderEvidence}
			provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-replay", Model: SelectionModel, HTTPClient: &http.Client{Transport: replay}, MaxOutputTokens: 1600})
			if err != nil {
				t.Fatal(err)
			}
			response, err := provider.GenerateJSON(context.Background(), request)
			if err != nil || !replay.matched || !sameReferencedJSON(response.IntentJSON, s.RawIntent) {
				t.Fatal("archived indexed HTTP contract changed", err)
			}
			// This is byte-contract compatibility only, never new accuracy or a repair.
		})
	}
}

func TestOwnedProtocolLegacyGuardsStaySeparate(t *testing.T) {
	for _, p := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, 99} {
		t.Run(fmt.Sprint(p), func(t *testing.T) {
			limit := p.requestLimit()
			if (p == indexedProtocol && limit != 24000) || (p == ownedProtocol && limit != 65536) || (p == connectionProtocol && limit != 65536) || (p == 99 && limit != 0) {
				t.Fatal("wrong fixed limit")
			}
			calls := 0
			transport := reservedTransport{Path: filepath.Join(t.TempDir(), "ledger.json"), Policy: LedgerPolicy{"offline-owned-guard", 1, 50000}, protocol: p, Base: roundTripperFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not run") })}
			r, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", strings.NewReader(strings.Repeat("x", int(limit+1))))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Body.Close()
			if _, err := transport.RoundTrip(r); err == nil || calls != 0 || transport.Index != 0 {
				t.Fatal("oversize request reserved or reached transport")
			}
			if _, err := os.Stat(transport.Path); !os.IsNotExist(err) {
				t.Fatal("oversize request wrote a ledger")
			}
			replay := &referencedAuditReplay{protocol: p, evidence: &ReferencedProviderEvidence{RequestBody: []byte(strings.Repeat("x", int(limit+1)))}}
			if _, err := replay.RoundTrip(r); err == nil || replay.matched {
				t.Fatal("byte replay accepted a request forbidden by the live guard")
			}
		})
	}
	if _, _, err := extractionProtocol(99).prepare("Hello"); err == nil {
		t.Fatal("unknown protocol accepted")
	}
	if _, err := extractionProtocol(99).decode("Hello", []byte("{}")); err == nil {
		t.Fatal("unknown decoder accepted")
	}
	request, source, err := prepareOwnedGenerateRequest("Use BMP280 with 2.2k pull-ups.")
	if err != nil {
		t.Fatal(err)
	}
	var sent ReferencedRequest
	if err := json.Unmarshal([]byte(request.Prompt), &sent); err != nil || !reflect.DeepEqual(sent, source) {
		t.Fatal("original source table lost")
	}
	if strings.Contains(request.CapabilityContext, string(mustOwnedCatalog(t))) {
		t.Fatal("catalog entered requirement extraction")
	}
}
func mustOwnedCatalog(t testing.TB) []byte {
	t.Helper()
	b, err := json.Marshal(Catalog())
	if err != nil {
		t.Fatal(err)
	}
	return b
}
