package practicalboardeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"kicadai/internal/aiprovider"
)

// Journal entries are append-only. The campaign supervisor runs exactly one
// worker at a time; exclusive creation also rejects accidental duplicate use.
type Reservation struct {
	Number        int     `json:"number"`
	Campaign      string  `json:"campaign"`
	CaseID        string  `json:"case_id"`
	RequestSHA256 string  `json:"request_sha256"`
	RequestBytes  int     `json:"request_bytes"`
	ReservedUSD   float64 `json:"reserved_usd"`
	StartedUTC    string  `json:"started_utc"`
}

type Reconciliation struct {
	Usage                aiprovider.Usage `json:"usage"`
	ChargedOrReservedUSD float64          `json:"charged_or_reserved_usd"`
	UsageAvailable       bool             `json:"usage_available"`
}

func Reserve(root, campaign, caseID string, body []byte) (Reservation, error) {
	if !SafeName(caseID) || (campaign != "baseline" && campaign != "final") {
		return Reservation{}, fmt.Errorf("invalid live campaign identity")
	}
	if len(body) == 0 || len(body) > MaxRequestBytes {
		return Reservation{}, fmt.Errorf("request byte cap exceeded: %d", len(body))
	}
	entries, err := filepath.Glob(filepath.Join(root, "*.reservation.json"))
	if err != nil {
		return Reservation{}, err
	}
	spend := 0.0
	campaignCount := 0
	for _, path := range entries {
		var prior Reservation
		if err := ReadJSON(path, &prior); err != nil {
			return Reservation{}, err
		}
		if prior.Campaign == campaign {
			campaignCount++
		}
		cost := prior.ReservedUSD
		usagePath := strings.TrimSuffix(path, ".reservation.json") + ".usage.json"
		var reconciled Reconciliation
		if err := ReadJSON(usagePath, &reconciled); err == nil {
			cost = reconciled.ChargedOrReservedUSD
		} else if !os.IsNotExist(err) {
			return Reservation{}, err
		}
		if cost < 0 {
			return Reservation{}, fmt.Errorf("invalid prior cost")
		}
		spend += cost
	}
	reserved := float64(len(body)+4096)*4/1e6 + float64(MaxOutputTokens)*20/1e6
	if len(entries) >= MaxRequests || campaignCount >= MaxCampaignRequests || spend+reserved > MaxSpendUSD {
		return Reservation{}, fmt.Errorf("frozen request/spend cap reached")
	}
	r := Reservation{len(entries) + 1, campaign, caseID, SHA(body), len(body), reserved, time.Now().UTC().Format(time.RFC3339Nano)}
	if err := WriteJSON(filepath.Join(root, fmt.Sprintf("%03d.reservation.json", r.Number)), r); err != nil {
		return Reservation{}, err
	}
	return r, nil
}

type RecordingTransport struct {
	Base     http.RoundTripper
	Journal  string
	Output   string
	Campaign string
	CaseID   string
	Secret   string
	Last     *Reservation
}

var keyPattern = regexp.MustCompile(`sk-[A-Za-z0-9_-]+`)

func Redact(data []byte, secret string) []byte {
	if secret != "" {
		data = bytes.ReplaceAll(data, []byte(secret), []byte("<redacted>"))
	}
	return keyPattern.ReplaceAll(data, []byte("<redacted>"))
}

func (r *RecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.Last = nil
	if req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "api.openai.com" || req.URL.Path != "/v1/responses" || req.URL.RawQuery != "" || req.URL.User != nil {
		return nil, fmt.Errorf("unapproved evaluation endpoint")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, MaxRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if err := req.Body.Close(); err != nil {
		return nil, err
	}
	// This asserts the actual client request, not merely the intended settings.
	var config struct {
		Model           string `json:"model"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Store           bool   `json:"store"`
		Stream          bool   `json:"stream"`
		Background      bool   `json:"background"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	if config.Model != Model || config.MaxOutputTokens != MaxOutputTokens || config.Store || config.Background || !config.Stream {
		return nil, fmt.Errorf("provider configuration differs from freeze")
	}
	if r.Secret != "" && bytes.Contains(body, []byte(r.Secret)) {
		return nil, fmt.Errorf("credential found in request body")
	}
	reservation, err := Reserve(r.Journal, r.Campaign, r.CaseID, body)
	if err != nil {
		return nil, err
	}
	r.Last = &reservation
	prefix := filepath.Join(r.Output, fmt.Sprintf("http-%03d", reservation.Number))
	if err := WriteNew(prefix+".request.json", body); err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	base := r.Base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(req)
	if err != nil {
		if captureErr := WriteJSON(prefix+".transport-error.json", map[string]any{"error": string(Redact([]byte(err.Error()), r.Secret))}); captureErr != nil {
			return nil, captureErr
		}
		return nil, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	closeErr := response.Body.Close()
	if err := WriteNew(prefix+".response.txt", Redact(payload, r.Secret)); err != nil {
		return nil, err
	}
	if err := WriteJSON(prefix+".response-metadata.json", map[string]any{
		"status": response.StatusCode, "content_type": response.Header.Get("Content-Type"),
		"request_id": response.Header.Get("x-request-id"), "retained_bytes": len(payload),
		"redaction": "exact credential and key-shaped strings; authorization headers never captured",
	}); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(payload) > 8<<20 {
		return nil, fmt.Errorf("HTTP response evidence cap exceeded")
	}
	response.Body = io.NopCloser(bytes.NewReader(payload))
	return response, nil
}

func (r *RecordingTransport) Reconcile(usage aiprovider.Usage) error {
	if r.Last == nil {
		return nil
	}
	amount := r.Last.ReservedUSD
	available := usage.InputTokens >= 0 && usage.OutputTokens >= 0 && usage.TotalTokens > 0 && usage.InputTokens+usage.OutputTokens == usage.TotalTokens
	if available {
		amount = float64(usage.InputTokens)*4/1e6 + float64(usage.OutputTokens)*20/1e6
	}
	return WriteJSON(filepath.Join(r.Journal, fmt.Sprintf("%03d.usage.json", r.Last.Number)), Reconciliation{usage, amount, available})
}
