package boardfamily

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kicadai/internal/aiprovider"
)

// ReferencedProviderEvidence records application-visible HTTP body bytes, not
// TLS packets or provider attestation. []byte JSON fields use lossless base64,
// including when the body contains invalid UTF-8 or non-JSON model output.
// No request headers or credentials are recorded. A partial body stays partial.
type ReferencedProviderEvidence struct {
	RequestBody      []byte `json:"request_body_base64"`
	RequestSHA256    string `json:"request_sha256"`
	StatusCode       int    `json:"http_status"`
	ContentType      string `json:"content_type"`
	Body             []byte `json:"response_body_base64"`
	BodySHA256       string `json:"response_sha256"`
	BytesRead        int    `json:"response_bytes_read"`
	EOFObserved      bool   `json:"eof_observed"`
	Truncated        bool   `json:"truncated"`
	TransportStarted bool   `json:"transport_started"`
	TransportError   bool   `json:"transport_error"`
	ReadError        bool   `json:"read_error"`
	CloseError       bool   `json:"close_error"`
	PersistenceError bool   `json:"persistence_error"`
}

type referencedRecordingTransport struct {
	protocol extractionProtocol
	base     http.RoundTripper
	evidence *ReferencedProviderEvidence
	journal  *referencedJournal
}

func (t *referencedRecordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.evidence != nil {
		return nil, errors.New("recorded request cannot be retried")
	}
	t.evidence = &ReferencedProviderEvidence{}
	if r.GetBody == nil {
		return nil, errors.New("request body cannot be recorded")
	}
	body, err := r.GetBody()
	if err != nil {
		return nil, errors.New("request body could not be opened for recording")
	}
	request, readErr := io.ReadAll(io.LimitReader(body, t.protocol.requestLimit()+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || len(request) == 0 || int64(len(request)) > t.protocol.requestLimit() || int64(len(request)) != r.ContentLength {
		return nil, errors.New("request body could not be recorded within its bound")
	}
	t.evidence.RequestBody, t.evidence.RequestSHA256 = request, referencedDigest(request)
	if t.journal != nil {
		if err := t.journal.prepareRequest(request); err != nil {
			t.evidence.PersistenceError = true
			return nil, errors.New("request evidence could not be made durable")
		}
	}
	t.evidence.TransportStarted = true
	response, err := t.base.RoundTrip(r)
	if err != nil {
		t.evidence.TransportError = true
	}
	if response != nil {
		t.evidence.StatusCode = response.StatusCode
		t.evidence.ContentType = response.Header.Get("Content-Type")
		if response.Body != nil {
			response.Body = &referencedRecordingBody{ReadCloser: response.Body, evidence: t.evidence, journal: t.journal}
			if err != nil {
				// net/http ignores responses returned alongside an error.
				err = errors.Join(err, response.Body.Close())
				return nil, err
			}
		}
	}
	return response, err
}

type referencedRecordingBody struct {
	io.ReadCloser
	evidence *ReferencedProviderEvidence
	journal  *referencedJournal
}

func (b *referencedRecordingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	e := b.evidence
	e.BytesRead += n
	// The candidate's 1600-output-token provider limit has a 2 MiB HTTP
	// response bound. Keep at most that prefix, not an unbounded diagnostic.
	keep := min(n, aiprovider.MaxResponseBytes-len(e.Body))
	e.Body = append(e.Body, p[:keep]...)
	e.Truncated = e.Truncated || keep < n
	if err == io.EOF {
		e.EOFObserved = true
	} else if err != nil {
		e.ReadError = true
	}
	if b.journal != nil && keep > 0 {
		if writeErr := b.journal.appendResponse(p[:keep]); writeErr != nil {
			e.PersistenceError = true
			return n, errors.New("response evidence write failed")
		}
	}
	return n, err
}

func (b *referencedRecordingBody) Close() error {
	err := b.ReadCloser.Close()
	b.evidence.CloseError = b.evidence.CloseError || err != nil
	return err
}

func referencedDigest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type referencedTerminal struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Usage  *struct {
		Input  *int `json:"input_tokens"`
		Output *int `json:"output_tokens"`
		Total  *int `json:"total_tokens"`
	} `json:"usage"`
}

// Terminal metadata is derived independently from the retained bytes. In
// particular, a missing model or usage is never filled with requested defaults.
func inspectReferencedEvidence(e *ReferencedProviderEvidence) (referencedTerminal, error) {
	var terminal referencedTerminal
	if e == nil || e.TransportError || e.ReadError || e.CloseError || e.Truncated || !e.EOFObserved || e.StatusCode < 200 || e.StatusCode >= 300 {
		return terminal, errors.New("complete successful HTTP response evidence is unavailable")
	}
	if len(e.Body) != e.BytesRead || referencedDigest(e.Body) != e.BodySHA256 || referencedDigest(e.RequestBody) != e.RequestSHA256 {
		return terminal, errors.New("recorded HTTP body integrity mismatch")
	}
	if strings.Contains(strings.ToLower(e.ContentType), "text/event-stream") {
		if err := checkReferencedStreamJSON(e.Body); err != nil {
			return terminal, err
		}
	}
	raw, err := aiprovider.RecordedOpenAITerminal(e.Body, e.ContentType, 1600)
	if err != nil {
		return terminal, err
	}
	if err := validateReferencedEnvelopeJSON(raw); err != nil {
		return terminal, fmt.Errorf("terminal response JSON is ambiguous or invalid: %w", err)
	}
	if err := json.Unmarshal(raw, &terminal); err != nil {
		return terminal, errors.New("terminal response metadata is malformed")
	}
	if strings.TrimSpace(terminal.ID) == "" || strings.TrimSpace(terminal.Model) == "" || !member(terminal.Status, "completed", "incomplete", "failed") {
		return terminal, errors.New("terminal response identity, model or status is missing")
	}
	u := terminal.Usage
	if u == nil || u.Input == nil || u.Output == nil || u.Total == nil || *u.Input < 0 || *u.Output < 0 || *u.Output > 1600 || *u.Total < *u.Input || *u.Total-*u.Input != *u.Output {
		return terminal, errors.New("terminal response usage is missing or inconsistent")
	}
	return terminal, nil
}

// Validate every event's JSON before the shared framing parser can apply
// encoding/json's last-value-wins behavior to duplicate event fields.
// OpenAI echoes the output schema inside an envelope, adding nesting that is
// absent from the model-authored intent. The full body remains byte-bounded by
// the recording transport; only envelope JSON receives this separate bound.
const maxReferencedEnvelopeJSONDepth = 64

func validateReferencedEnvelopeJSON(raw []byte) error {
	return validateJSONDepth(raw, maxReferencedEnvelopeJSONDepth, "provider envelope")
}

func checkReferencedStreamJSON(body []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), len(body)+1)
	var lines []string
	check := func() error {
		payload := strings.Join(lines, "\n")
		lines = nil
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		if err := validateReferencedEnvelopeJSON([]byte(payload)); err != nil {
			return fmt.Errorf("recorded stream event JSON is ambiguous or invalid: %w", err)
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := check(); err != nil {
				return err
			}
		} else if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if scanner.Err() != nil {
		return errors.New("recorded stream event exceeded its bound")
	}
	return check()
}
