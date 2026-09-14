package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"kicadai/internal/aiprovider"
)

// ReferencedJournalAudit establishes local byte/contract/accounting consistency,
// not provider attestation, semantic accuracy, or a process exit. A collector
// must separately observe a fresh child exit and validate any generated bundle.
type ReferencedJournalAudit struct {
	Version     string            `json:"version"`
	Outcome     string            `json:"outcome"`
	Selection   Selection         `json:"selection"`
	Policy      LedgerPolicy      `json:"policy"`
	Ledger      Ledger            `json:"ledger"`
	FilesSHA256 map[string]string `json:"files_sha256"`
}

func readReferencedAuditFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit {
		return nil, errors.New("journal audit requires bounded private regular files")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("journal audit file changed"), f.Close())
	}
	b, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("journal audit file exceeded its bound")
	}
	return b, nil
}

func decodeReferencedAuditJSON(b []byte, v any) error {
	if err := validateIntentJSON(b); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func sameReferencedJSON(a, b any) bool {
	x, ea := json.Marshal(a)
	y, eb := json.Marshal(b)
	return ea == nil && eb == nil && bytes.Equal(x, y)
}

// InspectReferencedJournal performs bounded local reads and replays only the
// recorded HTTP bytes through an in-memory transport. It neither reads a key nor
// performs network I/O. Failed/unknown transport or persistence cannot pass it.
func InspectReferencedJournal(root string) (ReferencedJournalAudit, error) {
	var audit ReferencedJournalAudit
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return audit, errors.New("journal audit requires its private directory")
	}
	expected := []string{"request/body.bin", "request/receipt.json", "response.bin", "response/receipt.json", "selection/ledger.json", "selection/receipt.json", "selection/selection.json", "start.json"}
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("journal audit rejects symlinks")
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			if len(files) > len(expected) {
				return errors.New("extra journal files")
			}
		}
		return nil
	})
	sort.Strings(files)
	if err != nil || !reflect.DeepEqual(files, expected) {
		return audit, errors.New("journal inventory incomplete or unexpected")
	}
	data := map[string][]byte{}
	audit.FilesSHA256 = map[string]string{}
	for _, name := range files {
		limit := int64(1 << 20)
		if name == "response.bin" {
			limit = aiprovider.MaxResponseBytes
		}
		if name == "selection/selection.json" {
			limit = 4 << 20
		}
		b, err := readReferencedAuditFile(filepath.Join(root, name), limit)
		if err != nil {
			return audit, fmt.Errorf("audit %s: %w", name, err)
		}
		data[name], audit.FilesSHA256[name] = b, referencedDigest(b)
	}
	var start struct {
		Version          string
		AdmissionVersion string `json:"admission_version"`
		OriginalRequest  string `json:"original_request"`
		Policy           LedgerPolicy
		Status           string
	}
	if err := decodeReferencedAuditJSON(data["start.json"], &start); err != nil {
		return audit, err
	}
	if start.Version != referencedJournalVersion || start.AdmissionVersion != ReferenceIntentVersion || start.Status != "prepared-not-transport-authority" || start.Policy.validate() != nil {
		return audit, errors.New("journal start contract differs")
	}
	var s ReferencedSelection
	if err := decodeReferencedAuditJSON(data["selection/selection.json"], &s); err != nil {
		return audit, err
	}
	if s.AdmissionVersion != ReferenceIntentVersion || s.OriginalRequest != start.OriginalRequest || s.LedgerIndex < 1 {
		return audit, errors.New("journal selection identity differs")
	}
	source, err := PrepareReferencedRequest(s.OriginalRequest)
	if err != nil || !reflect.DeepEqual(s.RequestClauses, source.Clauses) || !reflect.DeepEqual(s.SourceQuantities, source.Quantities) {
		return audit, errors.New("journal source inventory differs")
	}
	e := s.ProviderEvidence
	terminal, err := inspectReferencedEvidence(e)
	if err != nil || e.PersistenceError || !e.TransportStarted || terminal.Status != "completed" || terminal.Model != SelectionModel || *terminal.Usage.Input > 1_000_000 {
		return audit, errors.New("journal has no complete verified pinned response")
	}
	if !bytes.Equal(data["request/body.bin"], e.RequestBody) || !bytes.Equal(data["response.bin"], e.Body) || s.ResponseID != terminal.ID || s.Model != terminal.Model || s.Usage.InputTokens != *terminal.Usage.Input || s.Usage.OutputTokens != *terminal.Usage.Output || s.Usage.TotalTokens != *terminal.Usage.Total {
		return audit, errors.New("journal response/selection bytes or metadata differ")
	}
	request, _, err := prepareReferencedGenerateRequest(s.OriginalRequest)
	if err != nil {
		return audit, err
	}
	replay := &referencedAuditReplay{evidence: e}
	provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-journal-replay", Model: SelectionModel, HTTPClient: &http.Client{Transport: replay}, MaxOutputTokens: 1600})
	if err != nil {
		return audit, err
	}
	response, providerErr := provider.GenerateJSON(context.Background(), request)
	if !replay.matched {
		return audit, errors.New("journal request is not this runtime's exact contract")
	}
	decision := localDecision(s.OriginalRequest, "clarify", "No validated requirement extraction is available; no board was generated.", nil)
	outcome := "decision"
	if providerErr != nil {
		switch aiprovider.ErrorCodeOf(providerErr) {
		case aiprovider.ErrorMalformed:
			outcome = "invalid_extraction"
		case aiprovider.ErrorRefusal:
			outcome = "provider_refusal"
		default:
			return audit, errors.New("recorded response cannot be safely replayed")
		}
	} else {
		decision, err = DecodeReferencedIntent(s.OriginalRequest, response.IntentJSON)
		if err != nil {
			outcome = "invalid_extraction"
		}
	}
	if s.Outcome != outcome || !sameReferencedJSON(s.RawIntent, response.IntentJSON) || !sameReferencedJSON(s.Decision, decision) {
		return audit, errors.New("journal extraction outcome differs from byte replay")
	}
	j := referencedJournal{policy: start.Policy}
	ledger, err := j.ledgerSnapshot(&s, filepath.Join(root, "selection/ledger.json"))
	if err != nil || !bytes.Equal(ledger, data["selection/ledger.json"]) {
		return audit, errors.New("journal ledger does not join")
	}
	if err := decodeReferencedAuditJSON(ledger, &audit.Ledger); err != nil {
		return audit, err
	}
	if len(audit.Ledger.Entries) > start.Policy.MaxRequests || int64(len(audit.Ledger.Entries))*RequestReserveMicroUSD > start.Policy.MaxMicroUSD {
		return audit, errors.New("journal ledger exceeds approved policy")
	}
	ids := map[string]bool{}
	for i, entry := range audit.Ledger.Entries {
		if entry.Index != i+1 || entry.Status != "completed" || entry.Model != SelectionModel || entry.ReserveMicroUSD != RequestReserveMicroUSD || entry.ResponseID == "" || ids[entry.ResponseID] || entry.InputTokens < 0 || entry.InputTokens > 1_000_000 || entry.OutputTokens < 0 || entry.OutputTokens > 1600 || entry.EstimatedMicroUSD != (int64(entry.InputTokens)*4+int64(entry.OutputTokens)*16+9)/10 || entry.EstimatedMicroUSD > RequestReserveMicroUSD {
			return audit, errors.New("journal ledger history is disputed")
		}
		ids[entry.ResponseID] = true
	}
	receipts := map[string]map[string]any{
		"request/receipt.json":   {"version": referencedJournalVersion, "bytes": len(e.RequestBody), "sha256": e.RequestSHA256},
		"response/receipt.json":  {"version": referencedJournalVersion, "bytes": len(e.Body), "sha256": e.BodySHA256, "request_sha256": e.RequestSHA256, "http_status": e.StatusCode, "content_type": e.ContentType, "eof_observed": e.EOFObserved, "truncated": e.Truncated, "transport_started": e.TransportStarted, "transport_error": e.TransportError, "read_error": e.ReadError, "close_error": e.CloseError},
		"selection/receipt.json": {"version": referencedJournalVersion, "outcome": s.Outcome, "ledger_index": s.LedgerIndex, "ledger_sha256": referencedDigest(ledger), "ledger_joined": true, "ledger_snapshot_error": false, "status": "selection-recorded-not-process-terminal"},
	}
	for name, want := range receipts {
		var got map[string]any
		if err := decodeReferencedAuditJSON(data[name], &got); err != nil || !sameReferencedJSON(got, want) {
			return audit, fmt.Errorf("journal receipt differs: %s", name)
		}
	}
	audit.Version, audit.Outcome, audit.Selection, audit.Policy = "indexed-journal-audit-1", s.Outcome, s.Selection, start.Policy
	return audit, nil
}

type referencedAuditReplay struct {
	evidence *ReferencedProviderEvidence
	matched  bool
}

func (r *referencedAuditReplay) RoundTrip(request *http.Request) (*http.Response, error) {
	if r.matched {
		return nil, errors.New("journal replay cannot retry")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 24001))
	if err = errors.Join(err, request.Body.Close()); err != nil {
		return nil, err
	}
	if request.Method != "POST" || request.URL.String() != "https://api.openai.com/v1/responses" || !bytes.Equal(body, r.evidence.RequestBody) {
		return nil, errors.New("journal request contract mismatch")
	}
	r.matched = true
	return &http.Response{StatusCode: r.evidence.StatusCode, Header: http.Header{"Content-Type": []string{r.evidence.ContentType}}, Body: io.NopCloser(bytes.NewReader(r.evidence.Body))}, nil
}
