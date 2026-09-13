package airequirementeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kicadai/internal/aiprovider"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/reports"
)

type reservation struct {
	Number           int    `json:"number"`
	CaseID           string `json:"case_id"`
	Leg              string `json:"leg"`
	Attempt          int    `json:"attempt"`
	RequestSHA256    string `json:"request_sha256"`
	RequestBytes     int    `json:"request_bytes"`
	ReservedMicroUSD int64  `json:"reserved_microusd"`
	StartedUTC       string `json:"started_utc"`
}
type reconciliation struct {
	Usage                       aiprovider.Usage `json:"usage"`
	UsageAvailable              bool             `json:"usage_available"`
	EstimatedOrReservedMicroUSD int64            `json:"estimated_or_reserved_microusd"`
	AccountingViolation         bool             `json:"accounting_violation"`
}

func reserveAmount(requestBytes int) int64 { return int64(requestBytes+4096)*5 + MaxOutputTokens*20 }

func journalTotals(root string) (int, int64, error) {
	entries, err := filepath.Glob(filepath.Join(root, "*.reservation.json"))
	if err != nil {
		return 0, 0, err
	}
	var cost int64
	for i, path := range entries {
		var prior reservation
		if err := readJSON(path, &prior); err != nil {
			return 0, 0, err
		}
		if prior.Number != i+1 || filepath.Base(path) != fmt.Sprintf("%03d.reservation.json", i+1) || prior.RequestBytes < 1 || prior.RequestBytes > MaxRequestBytes || prior.ReservedMicroUSD != reserveAmount(prior.RequestBytes) {
			return 0, 0, fmt.Errorf("invalid prior reservation")
		}
		amount := prior.ReservedMicroUSD
		var usage reconciliation
		err := readJSON(strings.TrimSuffix(path, ".reservation.json")+".usage.json", &usage)
		if err == nil {
			amount = usage.EstimatedOrReservedMicroUSD
			want := prior.ReservedMicroUSD
			if usage.UsageAvailable {
				if usage.Usage.InputTokens < 0 || usage.Usage.OutputTokens < 0 || usage.Usage.TotalTokens <= 0 || usage.Usage.InputTokens+usage.Usage.OutputTokens != usage.Usage.TotalTokens || usage.Usage.InputTokens > prior.RequestBytes+4096 || usage.Usage.OutputTokens > MaxOutputTokens {
					return 0, 0, fmt.Errorf("invalid prior accepted usage")
				}
				want = int64(usage.Usage.InputTokens)*5 + int64(usage.Usage.OutputTokens)*20
			}
			if amount < 0 || amount != want || usage.AccountingViolation {
				return 0, 0, fmt.Errorf("prior usage/accounting integrity failure")
			}
		} else if !os.IsNotExist(err) {
			return 0, 0, err
		}
		cost += amount
	}
	if len(entries) > MaxRequests || cost > 25_000_000 {
		return 0, 0, fmt.Errorf("campaign budget exceeded")
	}
	return len(entries), cost, nil
}

type wireRequest struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Input        string `json:"input"`
	Text         struct {
		Format struct {
			Type   string         `json:"type"`
			Name   string         `json:"name"`
			Strict bool           `json:"strict"`
			Schema map[string]any `json:"schema"`
		} `json:"format"`
	} `json:"text"`
	MaxOutputTokens int  `json:"max_output_tokens"`
	Store           bool `json:"store"`
	Stream          bool `json:"stream"`
	Background      bool `json:"background"`
}

func validateWire(body []byte, expected aiprovider.GenerateRequest, instructionSHA string) error {
	if len(body) == 0 || len(body) > MaxRequestBytes {
		return fmt.Errorf("encoded request exceeds frozen byte cap")
	}
	var config wireRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if config.Model != Model || config.MaxOutputTokens != MaxOutputTokens || config.Store || config.Background || !config.Stream || sha([]byte(config.Instructions)) != instructionSHA {
		return fmt.Errorf("wire provider settings differ from freeze")
	}
	if config.Text.Format.Type != "json_schema" || !config.Text.Format.Strict || config.Text.Format.Name != expected.OutputSchemaName {
		return fmt.Errorf("wire schema configuration differs from freeze")
	}
	actualSchema, err := json.Marshal(config.Text.Format.Schema)
	if err != nil {
		return err
	}
	expectedSchema, err := json.Marshal(expected.OutputSchema)
	if err != nil || !bytes.Equal(actualSchema, expectedSchema) {
		return fmt.Errorf("wire schema differs from frozen production schema")
	}
	var input struct {
		Prompt            string                  `json:"prompt"`
		CapabilityContext string                  `json:"capability_context"`
		Attempt           int                     `json:"attempt"`
		Diagnostics       []aiprovider.Diagnostic `json:"diagnostics"`
	}
	decoder = json.NewDecoder(strings.NewReader(config.Input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	if input.Prompt != expected.Prompt || input.CapabilityContext != expected.CapabilityContext || input.Attempt != expected.Attempt || input.Attempt < 1 || input.Attempt > 2 {
		return fmt.Errorf("wire source/context/attempt differs from scheduled leg")
	}
	a, _ := json.Marshal(input.Diagnostics)
	b, _ := json.Marshal(expected.Diagnostics)
	if !bytes.Equal(a, b) || len(input.Diagnostics) > aiprovider.MaxDiagnostics {
		return fmt.Errorf("wire diagnostics differ from bounded correction")
	}
	return nil
}

type recordingTransport struct {
	Base                                                 http.RoundTripper
	Journal, Output, CaseID, Leg, Secret, InstructionSHA string
	Expected                                             aiprovider.GenerateRequest
	Guard                                                func() error
	Last                                                 *reservation
	mu                                                   sync.Mutex
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !r.mu.TryLock() {
		return nil, fmt.Errorf("concurrent provider request forbidden")
	}
	defer r.mu.Unlock()
	r.Last = nil
	if req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "api.openai.com" || req.URL.Path != "/v1/responses" || req.URL.RawQuery != "" || req.URL.User != nil || req.URL.Fragment != "" {
		return nil, fmt.Errorf("unapproved provider endpoint")
	}
	if req.Header.Get("Authorization") != "Bearer "+r.Secret || r.Secret == "" {
		return nil, fmt.Errorf("missing authorized provider credential")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, MaxRequestBytes+1))
	closeErr := req.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := validateWire(body, r.Expected, r.InstructionSHA); err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte(r.Secret)) {
		return nil, fmt.Errorf("credential appears in provider input")
	}
	if r.Guard == nil {
		return nil, fmt.Errorf("missing pre-dispatch freeze/resource guard")
	}
	if err := r.Guard(); err != nil {
		return nil, err
	}
	count, cost, err := journalTotals(r.Journal)
	if err != nil {
		return nil, err
	}
	amount := reserveAmount(len(body))
	if count >= MaxRequests || cost+amount > 25_000_000 {
		return nil, fmt.Errorf("new campaign request/spend cap reached")
	}
	if err := r.validateSchedule(); err != nil {
		return nil, err
	}
	receipt := reservation{count + 1, r.CaseID, r.Leg, r.Expected.Attempt, sha(body), len(body), amount, time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeJSON(filepath.Join(r.Journal, fmt.Sprintf("%03d.reservation.json", receipt.Number)), receipt); err != nil {
		return nil, err
	}
	r.Last = &receipt
	prefix := filepath.Join(r.Output, fmt.Sprintf("http-%03d", receipt.Number))
	if err := writeNew(prefix+".request.json", body); err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	base := r.Base
	if base == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil // Direct TLS to the approved host; no ambient proxy or redirect.
		base = transport
		defer transport.CloseIdleConnections()
	}
	response, err := base.RoundTrip(req)
	if err != nil {
		saveErr := writeJSON(prefix+".transport-error.json", map[string]any{"error": string(redact([]byte(err.Error()), r.Secret)), "completion": "unknown", "retry_permitted": false})
		if saveErr != nil {
			return nil, saveErr
		}
		return nil, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, CaptureBytes+1))
	closeErr = response.Body.Close()
	retained := redact(payload, r.Secret)
	if err := writeNew(prefix+".response.txt", retained); err != nil {
		return nil, err
	}
	meta := map[string]any{"http_status": response.StatusCode, "content_type": response.Header.Get("Content-Type"), "request_id": response.Header.Get("x-request-id"), "captured_bytes": len(payload), "retained_bytes": len(retained), "captured_sha256": sha(payload), "retained_sha256": sha(retained), "redacted": !bytes.Equal(payload, retained), "capture_complete": readErr == nil && closeErr == nil && len(payload) <= CaptureBytes, "capture_truncated": len(payload) > CaptureBytes, "read_error": readErr != nil, "close_error": closeErr != nil, "authorization_headers_retained": false}
	if err := writeJSON(prefix+".response-metadata.json", meta); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(payload) > CaptureBytes {
		return nil, fmt.Errorf("bounded evidence capture exceeded; retained prefix is not accepted output")
	}
	if !bytes.Equal(payload, retained) {
		return nil, fmt.Errorf("response required credential-shaped redaction; integrity review required")
	}
	response.Body = io.NopCloser(bytes.NewReader(payload))
	return response, nil
}

func (r *recordingTransport) validateSchedule() error {
	if !slices.Contains([]string{"I01", "I02", "I03", "I04", "R01", "R02", "C01", "C02"}, r.CaseID) || (r.Leg != "initial" && r.Leg != "follow-up") || (r.Leg == "follow-up" && !strings.HasPrefix(r.CaseID, "C")) {
		return fmt.Errorf("unapproved case or leg")
	}
	entries, err := filepath.Glob(filepath.Join(r.Journal, "*.reservation.json"))
	if err != nil {
		return err
	}
	count := 0
	for _, path := range entries {
		var prior reservation
		if err := readJSON(path, &prior); err != nil {
			return err
		}
		if prior.CaseID == r.CaseID && prior.Leg == r.Leg {
			count++
		}
	}
	if count != r.Expected.Attempt-1 {
		return fmt.Errorf("attempt already consumed or out of order")
	}
	if r.Expected.Attempt == 2 {
		var compiled behavioralintent.Result
		if err := readJSON(filepath.Join(r.Output, "attempt-1.compilation.json"), &compiled); err != nil || compiled.Status != behavioralintent.StatusInvalid || compiled.Requirement != nil || !reports.HasBlockingIssue(compiled.Issues) {
			return fmt.Errorf("correction lacks a recorded compiler-invalid first attempt")
		}
	}
	return nil
}

func (r *recordingTransport) reconcile(usage aiprovider.Usage, accepted bool) error {
	if r.Last == nil {
		return nil
	}
	available := accepted && usage.InputTokens >= 0 && usage.OutputTokens >= 0 && usage.TotalTokens > 0 && usage.InputTokens+usage.OutputTokens == usage.TotalTokens
	violation := available && (usage.InputTokens > r.Last.RequestBytes+4096 || usage.OutputTokens > MaxOutputTokens)
	amount := r.Last.ReservedMicroUSD
	if available {
		amount = int64(usage.InputTokens)*5 + int64(usage.OutputTokens)*20
	}
	value := reconciliation{usage, available, amount, violation}
	if err := writeJSON(filepath.Join(r.Journal, fmt.Sprintf("%03d.usage.json", r.Last.Number)), value); err != nil {
		return err
	}
	if violation {
		return fmt.Errorf("reported usage exceeded conservative reservation assumptions")
	}
	return nil
}
