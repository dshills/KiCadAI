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

func checkSourceAddressedProviderRequest(t testing.TB, r *http.Request, prompt string) int {
	t.Helper()
	if r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" || r.ContentLength <= 0 || r.ContentLength > SourceAddressedMaxRequestBytes {
		t.Fatal("source-addressed endpoint or size bound differs")
	}
	body, err := io.ReadAll(r.Body)
	if err = errors.Join(err, r.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) != r.ContentLength || bytes.Contains(body, []byte("offline-addressed-placeholder")) {
		t.Fatal("body length or credential separation differs")
	}
	var payload struct {
		Model, Input, Instructions string
		MaxOutputTokens            int `json:"max_output_tokens"`
		Store, Stream, Background  bool
		Text                       struct {
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
	want, source, err := prepareSourceAddressedGenerateRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != GroundedFullModel || payload.MaxOutputTokens != 1600 || payload.Store || payload.Background || !payload.Stream ||
		!payload.Text.Format.Strict || payload.Text.Format.Type != "json_schema" || payload.Text.Format.Name != SourceAddressedSchemaName ||
		payload.Input != want.Prompt || !strings.HasSuffix(payload.Instructions, "\n\n"+want.CapabilityContext) || !sameReferencedJSON(payload.Text.Format.Schema, want.OutputSchema) {
		t.Fatal("source-addressed wire contract differs")
	}
	var input SourceAddressedRequest
	if err := decodeReferencedAuditJSON([]byte(payload.Input), &input); err != nil || !sameReferencedJSON(input.Source, source) {
		t.Fatal("source lost its original inventory", err)
	}
	contract, err := SourceAddressedEvidenceContract(prompt)
	if err != nil || !sameReferencedJSON(contract["input"], input) ||
		!sameReferencedJSON(contract["source"], source) || !sameReferencedJSON(contract["schema"], payload.Text.Format.Schema) ||
		contract["capability_context"] != want.CapabilityContext || contract["live_authorization_granted"] != false {
		t.Fatal("export does not describe actual request", err)
	}
	return len(body)
}

func TestSourceAddressedProviderCorpusWireAndJournal(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-addressed-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override")
	b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(b, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus differs", err)
	}
	minimum, maximum := SourceAddressedMaxRequestBytes, 0
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			calls := 0
			s, err := InterpretSourceAddressedWithJournal(context.Background(), c.Prompt, filepath.Join(root, "ledger.json"), LedgerPolicy{"offline-addressed-wire", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				n := checkSourceAddressedProviderRequest(t, r, c.Prompt)
				minimum, maximum = min(minimum, n), max(maximum, n)
				return referencedProviderResponseForModel(t, []byte(`{}`), "refusal", "offline-addressed-"+c.ID, GroundedFullModel), nil
			}), journal)
			if err == nil || calls != 1 || s.Outcome != "provider_refusal" || s.Model != GroundedFullModel || s.AdmissionVersion != SourceAddressedVersion {
				t.Fatal("incorrect synthetic provider refusal", err, calls, s.Outcome)
			}
			a, err := InspectSourceAddressedJournal(journal)
			if err != nil || a.Version != "source-addressed-journal-audit-1" || a.Ledger.Version != 3 || a.Ledger.AccountingProfile != SourceAddressedAccountingProfile || a.Ledger.Entries[0].EstimatedMicroUSD != 1800 {
				t.Fatal("v9 accounting/audit differs", err)
			}
			for _, p := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, directProtocol, groundedProtocol, groundedFullProtocol, sourceEligibleProtocol} {
				if _, err := inspectProtocolJournal(journal, p); err == nil {
					t.Fatal("historical inspector accepted v9 journal")
				}
			}
		})
	}
	t.Logf("14 synthetic refusal transports: %d-%d request bytes; not a live semantic evaluation", minimum, maximum)
}

func TestSourceAddressedProviderPreflightAndUnknownOutcome(t *testing.T) {
	for _, mode := range []string{"no-budget", "no-key", "nil-transport", "no-ledger", "no-journal", "existing-journal", "oversize", "wrong-model", "transport-error", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-addressed-placeholder")
			root := t.TempDir()
			journal, ledger := filepath.Join(root, "journal"), filepath.Join(root, "ledger.json")
			prompt := "Use BMP280 standard."
			policy := LedgerPolicy{"offline-addressed-gates", 2, 100000}
			calls, want := 0, 0
			var base http.RoundTripper = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkSourceAddressedProviderRequest(t, r, prompt)
				if mode == "transport-error" {
					return nil, errors.New("synthetic transport error")
				}
				return referencedProviderResponseForModel(t, []byte(`{}`), mode, "offline-addressed-gate", GroundedFullModel), nil
			})
			switch mode {
			case "no-budget":
				policy = LedgerPolicy{}
			case "no-key":
				t.Setenv("OPENAI_API_KEY", "")
			case "nil-transport":
				base = nil
			case "no-ledger":
				ledger = ""
			case "no-journal":
				journal = ""
			case "existing-journal":
				if err := os.Mkdir(journal, 0700); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				prompt = strings.Repeat("\x00", 2000)
			default:
				want = 1
			}
			s, err := InterpretSourceAddressedWithJournal(context.Background(), prompt, ledger, policy, base, journal)
			if err == nil || calls != want || s.Decision.Configuration != nil {
				t.Fatal("unsafe preflight/result", mode, err, calls, s.Outcome)
			}
			if _, err := InspectSourceAddressedJournal(journal); err == nil {
				t.Fatal("failed request audited as complete")
			}
			if want == 0 {
				if _, err := os.Stat(ledger); !os.IsNotExist(err) {
					t.Fatal("preflight reserved funds", err)
				}
				return
			}
			before, err := os.ReadFile(ledger)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "wrong-model" {
				l := readReferencedTestLedger(t, ledger)
				if s.Outcome != "model_mismatch" || l.HaltReason == "" || l.Entries[0].EstimatedMicroUSD != 0 {
					t.Fatal("disputed model priced or not halted")
				}
			}
			if _, err := InterpretSourceAddressedWithJournal(context.Background(), prompt, ledger, policy, base, filepath.Join(root, "second-journal")); err == nil || calls != 1 {
				t.Fatal("unresolved/disputed history allowed another invocation", err)
			}
			after, err := os.ReadFile(ledger)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected history mutated", err)
			}
		})
	}
}

func TestSourceAddressedLedgerIsolationAndCaps(t *testing.T) {
	policy := LedgerPolicy{"offline-addressed-accounting", 2, 100000}
	if sourceAddressedProtocol.estimatedMicroUSD(100, 200) != 1800 || sourceAddressedProtocol.estimatedMicroUSD(SourceAddressedMaxRequestBytes, 1600) != 44800 {
		t.Fatal("wrong model pricing or reserve bound")
	}
	for _, old := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, directProtocol, groundedProtocol, groundedFullProtocol, sourceEligibleProtocol} {
		for _, pair := range [][2]extractionProtocol{{old, sourceAddressedProtocol}, {sourceAddressedProtocol, old}} {
			file := filepath.Join(t.TempDir(), "ledger.json")
			if _, err := reserveForProtocol(file, policy, pair[0]); err != nil {
				t.Fatal(err)
			}
			if err := finishReservationForProtocol(file, policy, pair[0], 1, "completed", "synthetic-isolation", 100, 200); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reserveForProtocol(file, policy, pair[1]); err == nil {
				t.Fatal("cross-protocol ledger reused")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected ledger changed", err)
			}
		}
	}
	for _, mode := range []string{"request-cap", "cost-cap", "overrun", "duplicate-id", "wrong-profile", "missing-profile"} {
		t.Run(mode, func(t *testing.T) {
			p := policy
			if mode == "request-cap" {
				p.MaxRequests = 1
			}
			if mode == "cost-cap" {
				p.MaxMicroUSD = 50000
			}
			if mode == "duplicate-id" {
				p.MaxRequests, p.MaxMicroUSD = 3, 150000
			}
			file := filepath.Join(t.TempDir(), "ledger.json")
			if _, err := reserveForProtocol(file, p, sourceAddressedProtocol); err != nil {
				t.Fatal(err)
			}
			input := 100
			if mode == "overrun" {
				input = 200000
			}
			err := finishReservationForProtocol(file, p, sourceAddressedProtocol, 1, "completed", "synthetic-accounting", input, 200)
			if (err != nil) != (mode == "overrun") {
				t.Fatal("wrong settlement result", err)
			}
			if mode == "duplicate-id" {
				if _, err := reserveForProtocol(file, p, sourceAddressedProtocol); err != nil {
					t.Fatal(err)
				}
				if err := finishReservationForProtocol(file, p, sourceAddressedProtocol, 2, "completed", "synthetic-accounting", 100, 200); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "wrong-profile" || mode == "missing-profile" {
				l := readReferencedTestLedger(t, file)
				l.AccountingProfile = GroundedFullAccountingProfile
				if mode == "missing-profile" {
					l.AccountingProfile = ""
				}
				if err := writeReferencedJournalJSON(file, l); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := reserveForProtocol(file, p, sourceAddressedProtocol); err == nil {
				t.Fatal("unsafe ledger accepted", mode)
			}
		})
	}
}

func TestSourceAddressedJournalTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, sourceAddressedProtocol)
}

func TestSourceAddressedHistoricalV8JournalIsolation(t *testing.T) {
	baseline := os.Getenv("KICADAI_ADDRESSED_V8_BASELINE")
	if baseline == "" {
		t.Skip("optional read-only historical v8 evidence path not supplied")
	}
	if !filepath.IsAbs(baseline) {
		t.Fatal("baseline path must be absolute")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(data, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("corpus differs", err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			root := filepath.Join(baseline, c.ID, "journal")
			a, err := InspectSourceEligibleJournal(root)
			if err != nil || a.Selection.OriginalRequest != c.Prompt || a.Selection.AdmissionVersion != SourceEligibleVersion || a.Ledger.AccountingProfile != SourceEligibleAccountingProfile {
				t.Fatal("historical v8 evidence no longer replays", err)
			}
			if _, err := InspectSourceAddressedJournal(root); err == nil {
				t.Fatal("historical v8 evidence accepted as new v9 output")
			}
		})
	}
}
