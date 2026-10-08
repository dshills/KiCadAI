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
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
)

func TestGroundedFullModelOnlyWireAndJournal(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-grounded-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override")
	b, err := os.ReadFile(filepath.Join("..", "..", "specs", "board-family-v2", "typed-evaluation-02", "cases-02.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(b, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus differs", err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			request, _, err := groundedProtocol.prepare(c.Prompt)
			if err != nil {
				t.Fatal(err)
			}
			var oldBody []byte
			mini, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-capture", Model: SelectionModel, MaxOutputTokens: 1600,
				HTTPClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					oldBody, err = io.ReadAll(r.Body)
					if err = errors.Join(err, r.Body.Close()); err != nil {
						t.Fatal(err)
					}
					return nil, errors.New("intentional in-memory capture")
				})}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mini.GenerateJSON(context.Background(), request); aiprovider.ErrorCodeOf(err) != aiprovider.ErrorTransport {
				t.Fatal(err)
			}
			// Optional retained historical wire joins this test to the actual
			// failed evaluation. Reads only; never replay a response as new output.
			if baseline := os.Getenv("KICADAI_FULL_COMPARISON_BASELINE"); baseline != "" {
				if !filepath.IsAbs(baseline) {
					t.Fatal("baseline path must be absolute")
				}
				actual, err := os.ReadFile(filepath.Join(baseline, c.ID, "journal", "request", "body.bin"))
				if err != nil || !bytes.Equal(actual, oldBody) {
					t.Fatal("historical request differs", err)
				}
				oldJournal := filepath.Join(baseline, c.ID, "journal")
				audit, err := InspectGroundedJournal(oldJournal)
				if err != nil || audit.Selection.Model != SelectionModel || audit.Ledger.Version != 2 || audit.Ledger.AccountingProfile != "" {
					t.Fatal("historical mini-model journal no longer replays", err)
				}
				if _, err := InspectGroundedFullJournal(oldJournal); err == nil {
					t.Fatal("old evidence was certified as full-model output")
				}
			}
			oldField := []byte(`"model":"` + SelectionModel + `"`)
			if bytes.Count(oldBody, oldField) != 1 {
				t.Fatal("ambiguous model replacement")
			}
			want := bytes.Replace(oldBody, oldField, []byte(`"model":"`+GroundedFullModel+`"`), 1)
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			calls := 0
			s, err := InterpretGroundedFullWithJournal(context.Background(), c.Prompt, filepath.Join(root, "ledger.json"), LedgerPolicy{"offline-full-wire", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, readErr := io.ReadAll(r.Body)
				if readErr = errors.Join(readErr, r.Body.Close()); readErr != nil {
					t.Fatal(readErr)
				}
				if !bytes.Equal(body, want) || len(body) > GroundedFullMaxRequestBytes {
					t.Fatal("non-model wire bytes changed or request too large")
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				checkGroundedFullProviderRequest(t, r, c.Prompt)
				return referencedProviderResponseForModel(t, groundedRaw(t, nil, nil), "refusal", "offline-full-"+c.ID, GroundedFullModel), nil
			}), journal)
			if err == nil || calls != 1 || s.Outcome != "provider_refusal" || s.Model != GroundedFullModel {
				t.Fatal("incorrect synthetic result", err, calls, s.Outcome)
			}
			a, err := InspectGroundedFullJournal(journal)
			if err != nil || a.Version != "partitioned-full-journal-audit-1" || a.Ledger.Version != 3 || a.Ledger.AccountingProfile != GroundedFullAccountingProfile || a.Ledger.Entries[0].EstimatedMicroUSD != 1800 {
				t.Fatal("full-model audit/accounting failed", err, a.Ledger)
			}
			for _, inspect := range []func(string) (ReferencedJournalAudit, error){InspectGroundedJournal, InspectDirectJournal, InspectConnectionJournal, InspectOwnedJournal, InspectReferencedJournal} {
				if _, err := inspect(journal); err == nil {
					t.Fatal("old inspector accepted full-model journal")
				}
			}
		})
	}
}

func TestGroundedFullAccountingIsolation(t *testing.T) {
	policy := LedgerPolicy{"offline-full-accounting", 14, 1000000}
	if groundedFullProtocol.estimatedMicroUSD(22066, 1772) != 58308 || groundedProtocol.estimatedMicroUSD(1, 1) != 2 {
		t.Fatal("rate or rounding changed")
	}
	if groundedFullProtocol.estimatedMicroUSD(GroundedFullMaxRequestBytes, 1600) != 44800 {
		t.Fatal("request bound no longer fits reservation")
	}
	for _, first := range []extractionProtocol{groundedProtocol, groundedFullProtocol} {
		t.Run(first.model(), func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "ledger.json")
			if _, err := reserveForProtocol(file, policy, first); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			other := groundedProtocol
			if first == groundedProtocol {
				other = groundedFullProtocol
			}
			if _, err := reserveForProtocol(file, policy, other); err == nil {
				t.Fatal("cross-model ledger reused")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected ledger changed", err)
			}
		})
	}
	file := filepath.Join(t.TempDir(), "full.json")
	for i := 1; i <= 14; i++ {
		n, err := reserveForProtocol(file, policy, groundedFullProtocol)
		if err != nil || n != i {
			t.Fatal("reservation", n, err)
		}
		if err := finishReservationForProtocol(file, policy, groundedFullProtocol, n, "completed", "synthetic-"+strconv.Itoa(n), 100, 200); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reserveForProtocol(file, policy, groundedFullProtocol); err == nil {
		t.Fatal("request cap ignored")
	}
	if err := finishReservationForProtocol(file, policy, groundedFullProtocol, 1, "completed", "synthetic", 100, 200); err == nil {
		t.Fatal("settlement repeated")
	}
	file = filepath.Join(t.TempDir(), "overrun.json")
	if _, err := reserveForProtocol(file, policy, groundedFullProtocol); err != nil {
		t.Fatal(err)
	}
	if err := finishReservationForProtocol(file, policy, groundedFullProtocol, 1, "completed", "synthetic-overrun", 200000, 200); err == nil {
		t.Fatal("overrun ignored")
	}
	if readReferencedTestLedger(t, file).HaltReason == "" {
		t.Fatal("overrun halt not persisted")
	}
	if _, err := reserveForProtocol(file, policy, groundedFullProtocol); err == nil {
		t.Fatal("halted ledger resumed")
	}
}

func TestGroundedFullPreflightAndNoRetry(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-grounded-placeholder")
	for _, mode := range []string{"no-budget", "oversize", "wrong-model", "transport-error", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			ledger := filepath.Join(root, "ledger.json")
			prompt := "Use BMP280 standard."
			policy := LedgerPolicy{"offline-full-gates", 1, 50000}
			if mode == "no-budget" {
				policy = LedgerPolicy{}
			}
			if mode == "oversize" {
				prompt = strings.Repeat("\x00", 2000)
			}
			calls := 0
			s, err := InterpretGroundedFullWithJournal(context.Background(), prompt, ledger, policy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkGroundedFullProviderRequest(t, r, prompt)
				if mode == "transport-error" {
					return nil, errors.New("synthetic transport error")
				}
				return referencedProviderResponseForModel(t, groundedRaw(t, nil, nil), mode, "offline-gate", GroundedFullModel), nil
			}), journal)
			want := 1
			if mode == "no-budget" || mode == "oversize" {
				want = 0
			}
			if err == nil || calls != want || s.Decision.Configuration != nil {
				t.Fatal("unsafe gate result", mode, err, calls, s.Outcome)
			}
			if want == 0 {
				if _, err := os.Stat(ledger); !os.IsNotExist(err) {
					t.Fatal("preflight reserved funds", err)
				}
			}
			if _, err := InspectGroundedFullJournal(journal); err == nil {
				t.Fatal("failed request audited as complete")
			}
		})
	}
}

func TestGroundedFullJournalTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, groundedFullProtocol)
}

func TestGroundedFullModelMismatchHaltsNextInvocation(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-grounded-placeholder")
	root := t.TempDir()
	ledger := filepath.Join(root, "ledger.json")
	policy := LedgerPolicy{"offline-full-model-mismatch", 2, 100000}
	calls := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		checkGroundedFullProviderRequest(t, r, "Use BMP280 standard.")
		return referencedProviderResponseForModel(t, groundedRaw(t, nil, nil), "wrong-model", "offline-wrong-model", GroundedFullModel), nil
	})
	s, err := InterpretGroundedFullWithJournal(context.Background(), "Use BMP280 standard.", ledger, policy, transport, filepath.Join(root, "first-journal"))
	if err == nil || s.Outcome != "model_mismatch" || s.Model == GroundedFullModel || calls != 1 || s.Decision.Configuration != nil {
		t.Fatal("model mismatch not preserved", s.Outcome, s.Model, calls, err)
	}
	receipt, err := os.ReadFile(filepath.Join(root, "first-journal", "selection", "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Outcome      string `json:"outcome"`
		LedgerJoined bool   `json:"ledger_joined"`
	}
	if err := json.Unmarshal(receipt, &recorded); err != nil || recorded.Outcome != "model_mismatch" || !recorded.LedgerJoined {
		t.Fatal("model mismatch journal lost its accounting join", recorded, err)
	}
	l := readReferencedTestLedger(t, ledger)
	if l.HaltReason == "" || len(l.Entries) != 1 || l.Entries[0].Status != "model_mismatch" || l.Entries[0].EstimatedMicroUSD != 0 || l.Entries[0].ReserveMicroUSD != RequestReserveMicroUSD {
		t.Fatal("disputed model was priced or left reusable", l)
	}
	before, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectGroundedFullJournal(filepath.Join(root, "first-journal")); err == nil {
		t.Fatal("model mismatch audited as complete")
	}
	if _, err := InterpretGroundedFullWithJournal(context.Background(), "Use BMP280 standard.", ledger, policy, transport, filepath.Join(root, "second-journal")); err == nil || calls != 1 {
		t.Fatal("model mismatch allowed another invocation", calls, err)
	}
	after, err := os.ReadFile(ledger)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("blocked invocation changed disputed ledger", err)
	}
}

func TestGroundedFullAccountingTampering(t *testing.T) {
	for _, mode := range []string{"profile-absent", "profile-other", "version", "cost", "model", "reserve", "budget"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-grounded-placeholder")
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			ledger := filepath.Join(root, "ledger.json")
			policy := LedgerPolicy{"offline-full-tamper", 2, 100000}
			_, err := InterpretGroundedFullWithJournal(context.Background(), "Use BMP280 standard.", ledger, policy,
				roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					checkGroundedFullProviderRequest(t, r, "Use BMP280 standard.")
					return referencedProviderResponseForModel(t, groundedRaw(t, nil, nil), "refusal", "synthetic-accounting", GroundedFullModel), nil
				}), journal)
			if err == nil {
				t.Fatal("synthetic refusal accepted")
			}
			if _, err := InspectGroundedFullJournal(journal); err != nil {
				t.Fatal("control audit", err)
			}
			l := readReferencedTestLedger(t, ledger)
			switch mode {
			case "profile-absent":
				l.AccountingProfile = ""
			case "profile-other":
				l.AccountingProfile = "unapproved-rates"
			case "version":
				l.Version = 2
			case "cost":
				l.Entries[0].EstimatedMicroUSD = groundedProtocol.estimatedMicroUSD(100, 200)
			case "model":
				l.Entries[0].Model = SelectionModel
			case "reserve":
				l.Entries[0].ReserveMicroUSD = 1
			case "budget":
				l.MaxRequests = 3
			}
			if err := writeReferencedJournalJSON(filepath.Join(journal, "selection", "ledger.json"), l); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(journal, "selection", "ledger.json"))
			if err != nil {
				t.Fatal(err)
			}
			// Coherently rehash the edited ledger: accounting checks, not just
			// a stale receipt hash, must reject it.
			file := filepath.Join(journal, "selection", "receipt.json")
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var receipt map[string]any
			if err := json.Unmarshal(raw, &receipt); err != nil {
				t.Fatal(err)
			}
			receipt["ledger_sha256"] = referencedDigest(b)
			if err := writeReferencedJournalJSON(file, receipt); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectGroundedFullJournal(journal); err == nil {
				t.Fatal("tampered accounting certified")
			}
			if err := os.WriteFile(ledger, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := reserveForProtocol(ledger, policy, groundedFullProtocol); err == nil {
				t.Fatal("tampered identity/accounting reused for spending")
			}
			after, err := os.ReadFile(ledger)
			if err != nil || !bytes.Equal(after, b) {
				t.Fatal("rejected history mutated", err)
			}
		})
	}
}

func TestGroundedFullTransportExactBounds(t *testing.T) {
	for _, size := range []int{0, GroundedFullMaxRequestBytes, GroundedFullMaxRequestBytes + 1} {
		root := t.TempDir()
		calls := 0
		transport := reservedTransport{protocol: groundedFullProtocol, Path: filepath.Join(root, "ledger.json"), Policy: LedgerPolicy{"offline-full-bound", 1, 50000}, Base: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("synthetic transport stop")
		})}
		r, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", strings.NewReader(strings.Repeat("x", size)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = transport.RoundTrip(r)
		if err == nil {
			t.Fatal("synthetic failure lost")
		}
		want := 0
		if size == GroundedFullMaxRequestBytes {
			want = 1
		}
		if calls != want || transport.Index != want {
			t.Fatal("incorrect request boundary", size, calls, transport.Index)
		}
		if want == 1 {
			if _, err := transport.RoundTrip(r); err == nil || calls != 1 {
				t.Fatal("retry permitted")
			}
		} else if _, err := os.Stat(transport.Path); !os.IsNotExist(err) {
			t.Fatal("invalid body reserved spend", err)
		}
		if err := r.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGroundedFullUnresolvedHistoryStops(t *testing.T) {
	for _, status := range []string{"reserved_unknown_outcome", "failed_or_unknown"} {
		t.Run(status, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "ledger.json")
			policy := LedgerPolicy{"offline-full-unknown", 2, 100000}
			if _, err := reserveForProtocol(file, policy, groundedFullProtocol); err != nil {
				t.Fatal(err)
			}
			if status == "failed_or_unknown" {
				if err := finishReservationForProtocol(file, policy, groundedFullProtocol, 1, status, "", 0, 0); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reserveForProtocol(file, policy, groundedFullProtocol); err == nil {
				t.Fatal("unresolved history allowed another request")
			}
			after, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("unresolved history changed", err)
			}
		})
	}
}
