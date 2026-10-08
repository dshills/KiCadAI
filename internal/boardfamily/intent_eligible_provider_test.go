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

func checkSourceEligibleProviderRequest(t testing.TB, r *http.Request, prompt string) int {
	t.Helper()
	if r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" || r.ContentLength <= 0 || r.ContentLength > SourceEligibleMaxRequestBytes {
		t.Fatal("source-eligible endpoint or size bound differs")
	}
	body, err := io.ReadAll(r.Body)
	if err = errors.Join(err, r.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) != r.ContentLength || bytes.Contains(body, []byte("offline-eligible-placeholder")) {
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
	want, source, err := prepareSourceEligibleGenerateRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != GroundedFullModel || payload.MaxOutputTokens != 1600 || payload.Store || payload.Background || !payload.Stream ||
		!payload.Text.Format.Strict || payload.Text.Format.Type != "json_schema" || payload.Text.Format.Name != SourceEligibleSchemaName ||
		payload.Input != want.Prompt || !strings.HasSuffix(payload.Instructions, "\n\n"+want.CapabilityContext) || !sameReferencedJSON(payload.Text.Format.Schema, want.OutputSchema) {
		t.Fatal("source-eligible wire contract differs")
	}
	var input sourceEligibleInput
	if err := decodeReferencedAuditJSON([]byte(payload.Input), &input); err != nil || !sameReferencedJSON(input.Source, source) {
		t.Fatal("source lost its original inventory", err)
	}
	contract, err := SourceEligibleEvidenceContract(prompt)
	if err != nil || !sameReferencedJSON(contract["input"], input) || !sameReferencedJSON(contract["eligibility"], input.Eligibility) ||
		!sameReferencedJSON(contract["source"], source) || !sameReferencedJSON(contract["schema"], payload.Text.Format.Schema) ||
		contract["capability_context"] != want.CapabilityContext || contract["live_authorization_granted"] != false {
		t.Fatal("export does not describe actual request", err)
	}
	return len(body)
}

func TestSourceEligibleProviderCorpusWireAndJournal(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-eligible-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override")
	b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(b, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus differs", err)
	}
	minimum, maximum := SourceEligibleMaxRequestBytes, 0
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			calls := 0
			s, err := InterpretSourceEligibleWithJournal(context.Background(), c.Prompt, filepath.Join(root, "ledger.json"), LedgerPolicy{"offline-eligible-wire", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				n := checkSourceEligibleProviderRequest(t, r, c.Prompt)
				minimum, maximum = min(minimum, n), max(maximum, n)
				return referencedProviderResponseForModel(t, eligibleRaw(t, nil, nil), "refusal", "offline-eligible-"+c.ID, GroundedFullModel), nil
			}), journal)
			if err == nil || calls != 1 || s.Outcome != "provider_refusal" || s.Model != GroundedFullModel || s.AdmissionVersion != SourceEligibleVersion {
				t.Fatal("incorrect synthetic provider refusal", err, calls, s.Outcome)
			}
			a, err := InspectSourceEligibleJournal(journal)
			if err != nil || a.Version != "source-eligible-journal-audit-1" || a.Ledger.Version != 3 || a.Ledger.AccountingProfile != SourceEligibleAccountingProfile || a.Ledger.Entries[0].EstimatedMicroUSD != 1800 {
				t.Fatal("v8 accounting/audit differs", err)
			}
			for _, p := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, directProtocol, groundedProtocol, groundedFullProtocol} {
				if _, err := inspectProtocolJournal(journal, p); err == nil {
					t.Fatal("historical inspector accepted v8 journal")
				}
			}
		})
	}
	t.Logf("14 synthetic refusal transports: %d-%d request bytes; not a live semantic evaluation", minimum, maximum)
}

func TestSourceEligibleProviderPreflightAndUnknownOutcome(t *testing.T) {
	for _, mode := range []string{"no-budget", "no-key", "nil-transport", "no-ledger", "no-journal", "existing-journal", "oversize", "wrong-model", "transport-error", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-eligible-placeholder")
			root := t.TempDir()
			journal, ledger := filepath.Join(root, "journal"), filepath.Join(root, "ledger.json")
			prompt := "Use BMP280 standard."
			policy := LedgerPolicy{"offline-eligible-gates", 2, 100000}
			calls, want := 0, 0
			var base http.RoundTripper = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkSourceEligibleProviderRequest(t, r, prompt)
				if mode == "transport-error" {
					return nil, errors.New("synthetic transport error")
				}
				return referencedProviderResponseForModel(t, eligibleRaw(t, nil, nil), mode, "offline-eligible-gate", GroundedFullModel), nil
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
			s, err := InterpretSourceEligibleWithJournal(context.Background(), prompt, ledger, policy, base, journal)
			if err == nil || calls != want || s.Decision.Configuration != nil {
				t.Fatal("unsafe preflight/result", mode, err, calls, s.Outcome)
			}
			if _, err := InspectSourceEligibleJournal(journal); err == nil {
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
			if _, err := InterpretSourceEligibleWithJournal(context.Background(), prompt, ledger, policy, base, filepath.Join(root, "second-journal")); err == nil || calls != 1 {
				t.Fatal("unresolved/disputed history allowed another invocation", err)
			}
			after, err := os.ReadFile(ledger)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected history mutated", err)
			}
		})
	}
}

func TestSourceEligibleLedgerIsolationAndCaps(t *testing.T) {
	policy := LedgerPolicy{"offline-eligible-accounting", 2, 100000}
	if sourceEligibleProtocol.estimatedMicroUSD(100, 200) != 1800 || sourceEligibleProtocol.estimatedMicroUSD(SourceEligibleMaxRequestBytes, 1600) != 44800 {
		t.Fatal("wrong model pricing or reserve bound")
	}
	for _, old := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, directProtocol, groundedProtocol, groundedFullProtocol} {
		for _, pair := range [][2]extractionProtocol{{old, sourceEligibleProtocol}, {sourceEligibleProtocol, old}} {
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
			if _, err := reserveForProtocol(file, p, sourceEligibleProtocol); err != nil {
				t.Fatal(err)
			}
			input := 100
			if mode == "overrun" {
				input = 200000
			}
			err := finishReservationForProtocol(file, p, sourceEligibleProtocol, 1, "completed", "synthetic-accounting", input, 200)
			if (err != nil) != (mode == "overrun") {
				t.Fatal("wrong settlement result", err)
			}
			if mode == "duplicate-id" {
				if _, err := reserveForProtocol(file, p, sourceEligibleProtocol); err != nil {
					t.Fatal(err)
				}
				if err := finishReservationForProtocol(file, p, sourceEligibleProtocol, 2, "completed", "synthetic-accounting", 100, 200); err != nil {
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
			if _, err := reserveForProtocol(file, p, sourceEligibleProtocol); err == nil {
				t.Fatal("unsafe ledger accepted", mode)
			}
		})
	}
}

func TestSourceEligibleJournalTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, sourceEligibleProtocol)
}

func TestSourceEligibleHistoricalJournalIsolation(t *testing.T) {
	baseline := os.Getenv("KICADAI_ELIGIBLE_FULL_BASELINE")
	if baseline == "" {
		t.Skip("optional read-only historical evidence path not supplied")
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
			a, err := InspectGroundedFullJournal(root)
			if err != nil || a.Selection.OriginalRequest != c.Prompt || a.Selection.AdmissionVersion != GroundedEvidenceVersion || a.Ledger.AccountingProfile != GroundedFullAccountingProfile {
				t.Fatal("historical evidence no longer replays", err)
			}
			if _, err := InspectSourceEligibleJournal(root); err == nil {
				t.Fatal("v7 historical evidence accepted as new v8 output")
			}
		})
	}
}
