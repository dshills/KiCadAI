package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const journalTestPrompt = "Please use BMP280 with standard profile."

var journalTestPolicy = LedgerPolicy{"offline-journal", 1, 50000}

func journalTestResponse(t testing.TB) *http.Response {
	t.Helper()
	raw := syntheticReferencedIntent(t, referencedChoice("sensor", "BMP280", "required", 0), referencedChoice("profile", "standard", "required", 0))
	return referencedProviderResponse(t, raw, "", "offline-journal-response")
}

func readJournalSelection(t testing.TB, root string) ReferencedSelection {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "selection", "selection.json"))
	var s ReferencedSelection
	if err != nil || json.Unmarshal(b, &s) != nil {
		t.Fatal("journal selection unavailable", err)
	}
	return s
}

func journalSelectionsEqual(a, b ReferencedSelection) bool {
	// RawIntent is a JSON object and MarshalIndent changes its whitespace.
	// Wire request/response []byte fields remain byte-exact base64 separately.
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

func checkJournalFile(t testing.TB, root, name string, present bool) {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, name))
	if (err == nil) != present || (err != nil && !os.IsNotExist(err)) {
		t.Fatalf("journal %s presence: %v, wanted %v", name, err, present)
	}
}

func TestReferencedJournalComplete(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
	dir := t.TempDir()
	root, ledger := filepath.Join(dir, "evidence"), filepath.Join(dir, "ledger.json")
	calls := 0
	s, err := InterpretReferencedWithJournal(context.Background(), journalTestPrompt, ledger, journalTestPolicy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		checkJournalFile(t, root, "request/receipt.json", true)
		checkJournalFile(t, root, "response.bin", true)
		checkJournalFile(t, root, "selection", false)
		checkReferencedProviderRequest(t, r, journalTestPrompt)
		return journalTestResponse(t), nil
	}), root)
	if err != nil || calls != 1 || s.Decision.Configuration == nil || s.Outcome != "decision" {
		t.Fatal("durable selection failed", err, s.Outcome)
	}
	saved := readJournalSelection(t, root)
	if !journalSelectionsEqual(saved, s) || s.Seconds <= 0 || !s.ProviderEvidence.TransportStarted || s.ProviderEvidence.PersistenceError {
		t.Fatal("saved and returned evidence differ")
	}
	for _, file := range []string{"start.json", "request/body.bin", "request/receipt.json", "response.bin", "response/receipt.json", "selection/selection.json", "selection/ledger.json", "selection/receipt.json"} {
		b, err := os.ReadFile(filepath.Join(root, file))
		info, statErr := os.Stat(filepath.Join(root, file))
		if err != nil || statErr != nil || info.Mode().Perm() != 0600 || bytes.Contains(b, []byte("offline-referenced-placeholder")) {
			t.Fatal("missing/private/credential-free journal file", file)
		}
	}
	request, _ := os.ReadFile(filepath.Join(root, "request", "body.bin"))
	body, _ := os.ReadFile(filepath.Join(root, "response.bin"))
	if !bytes.Equal(request, s.ProviderEvidence.RequestBody) || !bytes.Equal(body, s.ProviderEvidence.Body) {
		t.Fatal("durable wire bytes differ")
	}
	var receipt struct {
		LedgerJoined bool   `json:"ledger_joined"`
		LedgerSHA    string `json:"ledger_sha256"`
		Status       string
	}
	b, _ := os.ReadFile(filepath.Join(root, "selection", "receipt.json"))
	lb, _ := os.ReadFile(filepath.Join(root, "selection", "ledger.json"))
	if json.Unmarshal(b, &receipt) != nil || !receipt.LedgerJoined || receipt.LedgerSHA != referencedDigest(lb) || receipt.Status != "selection-recorded-not-process-terminal" {
		t.Fatal("invalid accounting checkpoint")
	}
	if _, err := newReferencedJournal(root, journalTestPrompt, journalTestPolicy); err == nil {
		t.Fatal("existing evidence was overwritten")
	}
}

type journalFaultFile struct {
	*os.File
	mode  string
	syncs int
}

func (f *journalFaultFile) Write(b []byte) (int, error) {
	if f.mode == "short-write" || f.mode == "write-error" {
		n, err := f.File.Write(b[:len(b)/2])
		if f.mode == "write-error" {
			err = errors.Join(err, errors.New("synthetic disk error"))
		}
		return n, err
	}
	return f.File.Write(b)
}

func (f *journalFaultFile) Sync() error {
	f.syncs++
	if (f.mode == "initial-sync" && f.syncs == 1) || (f.mode == "final-sync" && f.syncs == 2) {
		return errors.New("synthetic sync error")
	}
	return f.File.Sync()
}

func (f *journalFaultFile) Close() error {
	err := f.File.Close()
	if f.mode == "close-error" {
		return errors.Join(err, errors.New("synthetic close error"))
	}
	return err
}

func TestReferencedJournalFaults(t *testing.T) {
	for _, mode := range []string{"open-error", "initial-sync", "request-publish", "short-write", "write-error", "final-sync", "close-error", "response-publish", "selection-publish", "settlement-lock"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
			dir := t.TempDir()
			root, ledger := filepath.Join(dir, "evidence"), filepath.Join(dir, "ledger.json")
			j, err := newReferencedJournal(root, journalTestPrompt, journalTestPolicy)
			if err != nil {
				t.Fatal(err)
			}
			publish := j.publish
			j.publish = func(path string, build func(string) error) error {
				if mode == filepath.Base(path)+"-publish" {
					return errors.New("synthetic publication error")
				}
				return publish(path, build)
			}
			j.open = func(path string) (referencedJournalFile, error) {
				if mode == "open-error" {
					return nil, errors.New("synthetic open error")
				}
				f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return nil, err
				}
				return &journalFaultFile{File: f, mode: mode}, nil
			}
			calls := 0
			s, err := interpretReferencedWithJournal(context.Background(), journalTestPrompt, ledger, journalTestPolicy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				checkReferencedProviderRequest(t, r, journalTestPrompt)
				if mode == "settlement-lock" {
					if err := os.Mkdir(ledger+".lock", 0700); err != nil {
						t.Fatal(err)
					}
				}
				return journalTestResponse(t), nil
			}), j)
			wantCalls := 1
			if member(mode, "open-error", "initial-sync", "request-publish") {
				wantCalls = 0
			}
			if err == nil || calls != wantCalls || s.Outcome != "evidence_write_failure" || s.Decision.Configuration != nil || !s.ProviderEvidence.PersistenceError || s.ProviderEvidence.TransportStarted != (calls == 1) {
				t.Fatalf("unsafe fault result: mode %s calls %d outcome %s err %v", mode, calls, s.Outcome, err)
			}
			l := readReferencedTestLedger(t, ledger)
			if len(l.Entries) != 1 || l.Entries[0].ReserveMicroUSD != 50000 {
				t.Fatal("reservation was lost or refunded")
			}
			checkJournalFile(t, root, "response.bin", mode != "open-error")
			checkJournalFile(t, root, "response/receipt.json", member(mode, "selection-publish", "settlement-lock"))
			checkJournalFile(t, root, "selection/selection.json", mode != "selection-publish")
			if mode != "selection-publish" {
				saved := readJournalSelection(t, root)
				if !journalSelectionsEqual(saved, s) {
					t.Fatal("failed selection not retained faithfully")
				}
			}
			if member(mode, "short-write", "write-error") {
				b, err := os.ReadFile(filepath.Join(root, "response.bin"))
				if err != nil || len(b) == 0 || !bytes.HasPrefix(s.ProviderEvidence.Body, b) || len(b) >= len(s.ProviderEvidence.Body) {
					t.Fatal("partial spool deleted or overstated")
				}
			}
			if mode == "settlement-lock" {
				b, _ := os.ReadFile(filepath.Join(root, "selection", "receipt.json"))
				if l.Entries[0].Status != "reserved_unknown_outcome" || !bytes.Contains(b, []byte(`"ledger_joined": false`)) || s.ResponseID == "" || s.Usage.InputTokens != 100 {
					t.Fatal("disputed settlement was discarded or presented as joined")
				}
			}
		})
	}
}

func TestReferencedJournalOwnershipAndPreflight(t *testing.T) {
	for _, mode := range []string{"existing-directory", "existing-file", "symlink", "replaced-owner", "public-owner", "no-key", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
			dir := t.TempDir()
			root, ledger := filepath.Join(dir, "evidence"), filepath.Join(dir, "ledger.json")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var j *referencedJournal
			var err error
			switch mode {
			case "existing-directory":
				err = os.Mkdir(root, 0700)
			case "existing-file":
				err = os.WriteFile(root, []byte("existing user evidence"), 0600)
			case "symlink":
				err = os.Symlink(dir, root)
			case "replaced-owner", "public-owner":
				j, err = newReferencedJournal(root, journalTestPrompt, journalTestPolicy)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "public-owner" {
					err = os.Chmod(root, 0755)
				} else {
					err = os.Rename(root, root+".original")
					if err == nil {
						err = os.Mkdir(root, 0700)
					}
				}
			case "no-key":
				t.Setenv("OPENAI_API_KEY", "")
			case "cancelled":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			transport := roundTripperFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not send") })
			var s ReferencedSelection
			if j == nil {
				s, err = InterpretReferencedWithJournal(ctx, journalTestPrompt, ledger, journalTestPolicy, transport, root)
			} else {
				s, err = interpretReferencedWithJournal(ctx, journalTestPrompt, ledger, journalTestPolicy, transport, j)
			}
			if err == nil || calls != 0 || s.Decision.Configuration != nil {
				t.Fatal("preflight allowed a request or configuration")
			}
			if mode == "no-key" {
				saved := readJournalSelection(t, root)
				if saved.Outcome != "no_request" || saved.ProviderEvidence != nil {
					t.Fatal("no-key preflight invented provider evidence")
				}
				checkJournalFile(t, root, "request", false)
			}
			if mode == "existing-file" {
				b, err := os.ReadFile(root)
				if err != nil || string(b) != "existing user evidence" {
					t.Fatal("overwrote existing file")
				}
			}
			if mode == "replaced-owner" {
				checkJournalFile(t, root+".original", "start.json", true)
			}
		})
	}
}

func TestReferencedJournalLedgerSnapshotRejectsTampering(t *testing.T) {
	for _, mode := range []string{"unchanged", "symlink", "oversized", "duplicate-json", "unsettled", "changed-policy", "changed-identity", "changed-status", "changed-cost"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			j, err := newReferencedJournal(filepath.Join(dir, "evidence"), journalTestPrompt, journalTestPolicy)
			if err != nil {
				t.Fatal(err)
			}
			ledger := filepath.Join(dir, "ledger.json")
			l := Ledger{Version: 2, Goal: journalTestPolicy.Goal, MaxRequests: 1, MaxMicroUSD: 50000, Entries: []LedgerEntry{{Index: 1, Status: "completed", Model: SelectionModel, ReserveMicroUSD: 50000, ResponseID: "offline-journal-response", InputTokens: 100, OutputTokens: 200, EstimatedMicroUSD: 360}}}
			s := ReferencedSelection{Selection: Selection{LedgerIndex: 1, ResponseID: "offline-journal-response"}}
			s.Usage.InputTokens, s.Usage.OutputTokens = 100, 200
			response := journalTestResponse(t)
			wire, readErr := io.ReadAll(response.Body)
			if err := errors.Join(readErr, response.Body.Close()); err != nil {
				t.Fatal(err)
			}
			s.ProviderEvidence = &ReferencedProviderEvidence{RequestBody: []byte("{}"), RequestSHA256: referencedDigest([]byte("{}")), Body: wire,
				BodySHA256: referencedDigest(wire), BytesRead: len(wire), EOFObserved: true, TransportStarted: true, StatusCode: 200, ContentType: "text/event-stream"}
			if mode == "unsettled" {
				l.Entries[0].Status = "reserved_unknown_outcome"
			}
			if mode == "changed-policy" {
				l.MaxRequests = 2
			}
			if mode == "changed-identity" {
				l.Entries[0].ResponseID = "other"
			}
			if mode == "changed-status" {
				l.Entries[0].Status = "failed_or_unknown"
			}
			if mode == "changed-cost" {
				l.Entries[0].EstimatedMicroUSD++
			}
			b, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "oversized" {
				b = bytes.Repeat([]byte(" "), (1<<20)+1)
			}
			if mode == "duplicate-json" {
				b = []byte(`{"version":2,"version":1}`)
			}
			if mode == "symlink" {
				if err := os.WriteFile(ledger+".real", b, 0600); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(ledger+".real", ledger)
			} else {
				err = os.WriteFile(ledger, b, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unchanged" {
				if _, err := j.ledgerSnapshot(&s, ledger); err != nil {
					t.Fatal("valid baseline rejected", err)
				}
				return
			}
			if err := j.saveSelection(&s, ledger); err == nil || s.Outcome != "evidence_write_failure" {
				t.Fatal("tampered ledger accepted")
			}
			saved := readJournalSelection(t, j.root)
			if saved.Outcome != "evidence_write_failure" || saved.Decision.Configuration != nil {
				t.Fatal("disputed selection not retained as failed")
			}
			if err := j.saveSelection(&s, ledger); err == nil {
				t.Fatal("immutable selection was replaced")
			}
		})
	}
}

// Subprocesses exit at real checkpoints, with no sockets or real credentials.
// The parent observes terminal exit status, not a file that claims completion.
func TestReferencedJournalAbruptExit(t *testing.T) {
	if root := os.Getenv("KICADAI_JOURNAL_CHILD_ROOT"); root != "" {
		mode := os.Getenv("KICADAI_JOURNAL_CHILD_MODE")
		t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
		_, err := InterpretReferencedWithJournal(context.Background(), journalTestPrompt, filepath.Join(root, "ledger.json"), journalTestPolicy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			checkReferencedProviderRequest(t, r, journalTestPrompt)
			if mode == "after-request" {
				os.Exit(71)
			}
			response := journalTestResponse(t)
			if mode == "partial-response" {
				response.Body = &journalExitReader{Reader: strings.NewReader("data: {\"type\":\"response.created\"}\n\n")}
			}
			return response, nil
		}), filepath.Join(root, "evidence"))
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(73) // A settled selection exists; native generation has not begun.
	}
	for _, tc := range []struct {
		mode string
		code int
	}{{"after-request", 71}, {"partial-response", 72}, {"after-selection", 73}} {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReferencedJournalAbruptExit$")
			for _, e := range os.Environ() {
				key, _, _ := strings.Cut(e, "=")
				if !member(key, "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_LIVE_PROVIDER_TESTS", "KICADAI_JOURNAL_CHILD_ROOT", "KICADAI_JOURNAL_CHILD_MODE") {
					cmd.Env = append(cmd.Env, e)
				}
			}
			cmd.Env = append(cmd.Env, "KICADAI_JOURNAL_CHILD_ROOT="+dir, "KICADAI_JOURNAL_CHILD_MODE="+tc.mode)
			output, err := cmd.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != tc.code {
				t.Fatalf("child not terminal at expected checkpoint: %v %s", err, output)
			}
			root := filepath.Join(dir, "evidence")
			checkJournalFile(t, root, "request/receipt.json", true)
			checkJournalFile(t, root, "response.bin", true)
			checkJournalFile(t, root, "response/receipt.json", tc.mode == "after-selection")
			checkJournalFile(t, root, "selection/selection.json", tc.mode == "after-selection")
			l := readReferencedTestLedger(t, filepath.Join(dir, "ledger.json"))
			status := "reserved_unknown_outcome"
			if tc.mode == "after-selection" {
				status = "completed"
			}
			if len(l.Entries) != 1 || l.Entries[0].Status != status || l.Entries[0].ReserveMicroUSD != 50000 {
				t.Fatal("crash lost reservation or fabricated settlement")
			}
			if tc.mode == "partial-response" {
				b, err := os.ReadFile(filepath.Join(root, "response.bin"))
				if err != nil || !bytes.HasPrefix(b, []byte("data:")) {
					t.Fatal("process exit lost written prefix")
				}
			}
		})
	}
}

type journalExitReader struct {
	io.Reader
	read bool
}

func (r *journalExitReader) Read(b []byte) (int, error) {
	if r.read {
		os.Exit(72)
	}
	r.read = true
	n, _ := r.Reader.Read(b)
	return n, nil // Let the recorder write this prefix before the next Read exits.
}

func (*journalExitReader) Close() error { return nil }
