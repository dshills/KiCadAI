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

	"kicadai/internal/aiprovider"
	"kicadai/internal/atomicdir"
)

const referencedJournalVersion = "indexed-evidence-journal-1"

type referencedJournalFile interface {
	io.WriteCloser
	Sync() error
	Stat() (os.FileInfo, error)
}

// Journal files are never resumed, replaced or automatically removed. Atomic
// publication is only used for immutable checkpoints, never for the raw spool.
type referencedJournal struct {
	protocol      extractionProtocol
	root          string
	owner         os.FileInfo
	policy        LedgerPolicy
	response      referencedJournalFile
	responseOwner os.FileInfo
	request       []byte
	err           error
	finished      bool
	selected      bool
	publish       func(string, func(string) error) error
	open          func(string) (referencedJournalFile, error)
}

func newReferencedJournal(path, prompt string, policy LedgerPolicy) (*referencedJournal, error) {
	return newProtocolJournal(path, prompt, policy, indexedProtocol)
}

func newProtocolJournal(path, prompt string, policy LedgerPolicy, protocol extractionProtocol) (*referencedJournal, error) {
	if path == "" {
		return nil, errors.New("a new evidence-journal directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	initial := map[string]any{"version": protocol.journalVersion(), "admission_version": protocol.admissionVersion(),
		"original_request": prompt, "policy": policy, "status": "prepared-not-transport-authority"}
	if err := atomicdir.Publish(abs, func(staging string) error {
		return writeReferencedJournalJSON(filepath.Join(staging, "start.json"), initial)
	}); err != nil {
		return nil, err
	}
	if err := os.Chmod(abs, 0700); err != nil {
		return nil, err
	}
	owner, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	return &referencedJournal{root: abs, owner: owner, policy: policy, protocol: protocol, publish: atomicdir.Publish,
		open: func(path string) (referencedJournalFile, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		}}, nil
}

func (j *referencedJournal) checkOwner() error {
	info, err := os.Lstat(j.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, j.owner) || info.Mode().Perm()&0077 != 0 {
		return errors.New("evidence journal directory ownership changed")
	}
	return nil
}

func writeReferencedJournalJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func (j *referencedJournal) prepareRequest(body []byte) error {
	if j.err != nil || j.response != nil || j.finished || len(body) == 0 || int64(len(body)) > j.protocol.requestLimit() {
		return errors.New("journal request is invalid or already attempted")
	}
	if err := j.checkOwner(); err != nil {
		j.err = err
		return err
	}
	f, err := j.open(filepath.Join(j.root, "response.bin"))
	if err != nil {
		j.err = err
		return err
	}
	j.response = f
	j.responseOwner, err = f.Stat()
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = j.publish(filepath.Join(j.root, "request"), func(staging string) error {
			if err := os.WriteFile(filepath.Join(staging, "body.bin"), body, 0600); err != nil {
				return err
			}
			return writeReferencedJournalJSON(filepath.Join(staging, "receipt.json"), map[string]any{
				"version": j.protocol.journalVersion(), "bytes": len(body), "sha256": referencedDigest(body)})
		})
	}
	// The publication syncs the journal directory, including the empty response
	// spool's directory entry, before the network transport can be invoked.
	if err != nil {
		j.err = err
		return err
	}
	j.request = append([]byte(nil), body...)
	return nil
}

func (j *referencedJournal) appendResponse(b []byte) error {
	if j.err != nil {
		return j.err
	}
	if j.response == nil || j.finished {
		return errors.New("response spool is not open")
	}
	n, err := j.response.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err != nil {
		j.err = err
	}
	return err
}

// finishCapture seals a complete OR explicitly partial capture. The receipt
// means that the recorded prefix is durable, never that the HTTP call succeeded.
func (j *referencedJournal) finishCapture(e *ReferencedProviderEvidence) error {
	if j.finished {
		return errors.New("response capture already finished")
	}
	j.finished = true
	if j.response == nil {
		return j.err // Provider preflight may have stopped without a request.
	}
	syncErr, closeErr := j.response.Sync(), j.response.Close()
	j.err = errors.Join(j.err, syncErr, closeErr, j.checkOwner())
	if j.err != nil {
		return j.err
	}
	path := filepath.Join(j.root, "response.bin")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, j.responseOwner) {
		return errors.New("response spool ownership changed")
	}
	if e == nil || info.Size() > aiprovider.MaxResponseBytes || !bytes.Equal(j.request, e.RequestBody) {
		return errors.New("recorded response does not match its journal request")
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, e.Body) || referencedDigest(b) != e.BodySHA256 {
		return errors.New("recorded response differs from its durable spool")
	}
	return j.publish(filepath.Join(j.root, "response"), func(staging string) error {
		return writeReferencedJournalJSON(filepath.Join(staging, "receipt.json"), map[string]any{
			"version": j.protocol.journalVersion(), "bytes": len(b), "sha256": referencedDigest(b),
			"request_sha256": e.RequestSHA256, "http_status": e.StatusCode, "content_type": e.ContentType,
			"eof_observed": e.EOFObserved, "truncated": e.Truncated, "transport_started": e.TransportStarted,
			"transport_error": e.TransportError, "read_error": e.ReadError, "close_error": e.CloseError})
	})
}

func failReferencedJournal(s *ReferencedSelection) {
	s.Outcome = "evidence_write_failure"
	s.Decision = localDecision(s.OriginalRequest, "clarify", "Evidence could not be durably verified; no board was generated.", nil)
	if s.ProviderEvidence != nil {
		s.ProviderEvidence.PersistenceError = true
	}
}

// Keep even disputed accounting as evidence, but never authorize generation
// from it. The snapshot is bounded and rejects symlinks/non-regular files.
func (j *referencedJournal) ledgerSnapshot(s *ReferencedSelection, path string) ([]byte, error) {
	if s.LedgerIndex == 0 {
		return nil, nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("ledger snapshot is unavailable or outside its bound")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("ledger snapshot ownership changed"), f.Close())
	}
	b, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err := errors.Join(readErr, f.Close()); err != nil || len(b) > 1<<20 {
		return nil, errors.New("ledger snapshot could not be read within its bound")
	}
	var l Ledger
	if err := validateIntentJSON(b); err != nil {
		return b, errors.New("ledger snapshot JSON is ambiguous or invalid")
	}
	if err := json.Unmarshal(b, &l); err != nil || len(l.Entries) != s.LedgerIndex || l.Goal != j.policy.Goal {
		return b, errors.New("ledger snapshot does not end at this selection and policy")
	}
	if !j.protocol.ledgerMatchesPolicy(l, j.policy) {
		return b, errors.New("journal ledger policy differs")
	}
	e := l.Entries[s.LedgerIndex-1]
	expectedStatus := "failed_or_unknown"
	expectedCost := j.protocol.estimatedMicroUSD(e.InputTokens, e.OutputTokens)
	expectedHalt := ""
	terminal, evidenceErr := inspectReferencedEvidence(s.ProviderEvidence)
	if evidenceErr == nil && terminal.Status == "completed" {
		expectedStatus = "completed"
	}
	if j.protocol == groundedFullProtocol && evidenceErr == nil && terminal.Model != j.protocol.model() {
		if s.Outcome != "model_mismatch" {
			return b, errors.New("disputed model must remain a failed selection")
		}
		expectedStatus, expectedCost = "model_mismatch", 0
		expectedHalt = "returned model does not match full-model accounting profile"
	}
	if l.HaltReason != expectedHalt || e.Index != s.LedgerIndex || e.Model != j.protocol.model() || e.ReserveMicroUSD != RequestReserveMicroUSD || e.Status != expectedStatus || e.ResponseID != s.ResponseID || e.InputTokens != s.Usage.InputTokens || e.OutputTokens != s.Usage.OutputTokens || e.EstimatedMicroUSD != expectedCost {
		return b, errors.New("journal ledger and selection do not join")
	}
	return b, nil
}

func (j *referencedJournal) saveSelection(s *ReferencedSelection, ledgerPath string) error {
	if j.selected {
		return errors.New("journal selection was already attempted")
	}
	j.selected = true
	if err := j.checkOwner(); err != nil {
		return err
	}
	ledger, joinErr := j.ledgerSnapshot(s, ledgerPath)
	if joinErr != nil {
		failReferencedJournal(s)
	}
	publishErr := j.publish(filepath.Join(j.root, "selection"), func(staging string) error {
		if err := writeReferencedJournalJSON(filepath.Join(staging, "selection.json"), s); err != nil {
			return err
		}
		if len(ledger) != 0 {
			if err := os.WriteFile(filepath.Join(staging, "ledger.json"), ledger, 0600); err != nil {
				return err
			}
		}
		return writeReferencedJournalJSON(filepath.Join(staging, "receipt.json"), map[string]any{
			"version": j.protocol.journalVersion(), "outcome": s.Outcome, "ledger_index": s.LedgerIndex,
			"ledger_sha256": referencedDigest(ledger), "ledger_joined": s.LedgerIndex != 0 && joinErr == nil,
			"ledger_snapshot_error": joinErr != nil, "status": "selection-recorded-not-process-terminal"})
	})
	return errors.Join(joinErr, publishErr)
}

// InterpretReferencedWithJournal is the experimental single-request path with
// a separately owned, immutable evidence journal. It is selected by offline
// tests and an explicit experimental CLI mode, never by the CLI default. The caller
// must not start native generation unless this returns without error.
func InterpretReferencedWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, indexedProtocol)
}

func interpretProtocolJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string, protocol extractionProtocol) (ReferencedSelection, error) {
	failure := ReferencedSelection{Selection: Selection{OriginalRequest: prompt, AdmissionVersion: protocol.admissionVersion(),
		Model: protocol.model(), Decision: localDecision(prompt, "clarify", "No validated requirement extraction is available; no board was generated.", nil)},
		SourceQuantities: []SourceQuantity{}, Outcome: "no_request"}
	policy = policy.effective()
	if err := protocol.validatePolicy(policy); err != nil {
		return failure, err
	}
	if err := ctx.Err(); err != nil {
		return failure, err
	}
	if base == nil {
		return failure, errors.New("experimental extraction requires an explicit transport")
	}
	if _, _, err := protocol.prepare(prompt); err != nil {
		return failure, err
	}
	j, err := newProtocolJournal(journalPath, prompt, policy, protocol)
	if err != nil {
		return failure, fmt.Errorf("prepare evidence journal: %w", err)
	}
	return interpretProtocolWithJournal(ctx, prompt, ledgerPath, policy, base, j, protocol)
}
