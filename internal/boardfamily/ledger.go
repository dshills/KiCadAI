package boardfamily

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const SelectionModel = "gpt-4.1-mini-2025-04-14"

// Legacy v1 allowance, including separately approved extensions on September 13,
// 2026. Its exhausted ledger remains unchanged; these constants grant no new spend.
const MaxLiveRequests = 41
const MaxLiveMicroUSD = 10_000_000
const RequestReserveMicroUSD = 50_000

type LedgerEntry struct {
	Index             int    `json:"index"`
	Started           string `json:"started_utc"`
	Status            string `json:"status"`
	Model             string `json:"model"`
	ReserveMicroUSD   int64  `json:"reserve_micro_usd"`
	ResponseID        string `json:"response_id,omitempty"`
	InputTokens       int    `json:"input_tokens,omitempty"`
	OutputTokens      int    `json:"output_tokens,omitempty"`
	EstimatedMicroUSD int64  `json:"estimated_micro_usd,omitempty"`
}
type Ledger struct {
	Version     int           `json:"version"`
	Goal        string        `json:"goal"`
	MaxRequests int           `json:"max_requests,omitempty"`
	MaxMicroUSD int64         `json:"max_micro_usd,omitempty"`
	HaltReason  string        `json:"halt_reason,omitempty"`
	Entries     []LedgerEntry `json:"entries"`
}

const ledgerGoal = "board-family-v1-2026-09-13"

func ledgerChange(path string, f func(*Ledger) error) (err error) {
	return ledgerChangeWithPolicy(path, legacyLedgerPolicy(), f)
}

func ledgerChangeWithPolicy(path string, policy LedgerPolicy, f func(*Ledger) error) (err error) {
	policy = policy.effective()
	if err = policy.validate(); err != nil {
		return err
	}
	if path == "" {
		return errors.New("a persistent --ledger path is required for live requests")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	lock := path + ".lock"
	if e := os.Mkdir(lock, 0700); e != nil {
		return errors.New("ledger locked; no request sent (inspect previous process before recovery)")
	}
	defer func() { err = errors.Join(err, os.Remove(lock)) }()
	l := Ledger{Version: 1, Goal: policy.Goal, Entries: []LedgerEntry{}}
	if policy != legacyLedgerPolicy() {
		l.Version, l.MaxRequests, l.MaxMicroUSD = 2, policy.MaxRequests, policy.MaxMicroUSD
	}
	if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
		return errors.New("ledger must be a regular file, not a symlink")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	b, e := os.ReadFile(path)
	if e == nil {
		// Missing metadata must not inherit expected values from a new ledger.
		l = Ledger{}
		if e = json.Unmarshal(b, &l); e != nil {
			return errors.New("invalid ledger; refusing to reset history")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if l.Goal != policy.Goal || policy == legacyLedgerPolicy() && (l.Version != 1 || l.MaxRequests != 0 || l.MaxMicroUSD != 0) || policy != legacyLedgerPolicy() && (l.Version != 2 || l.MaxRequests != policy.MaxRequests || l.MaxMicroUSD != policy.MaxMicroUSD) {
		return errors.New("ledger belongs to a different goal/version/budget; refusing to reset or expand it")
	}
	if l.Entries == nil || len(l.Entries) > policy.MaxRequests {
		return errors.New("ledger history is absent or exceeds its immutable budget")
	}
	for i, x := range l.Entries {
		if x.Index != i+1 || x.Model != SelectionModel || x.ReserveMicroUSD != RequestReserveMicroUSD {
			return errors.New("ledger history is inconsistent")
		}
	}
	if e = f(&l); e != nil {
		return e
	}
	b, e = json.MarshalIndent(l, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".board-family-ledger-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer func() {
		// A successful rename already removed the temporary path.
		if cleanupErr := os.Remove(name); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if _, e = tmp.Write(append(b, '\n')); e != nil {
		return errors.Join(e, tmp.Close())
	}
	if e = tmp.Sync(); e != nil {
		return errors.Join(e, tmp.Close())
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	directory, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func reserve(path string) (int, error) {
	return reserveWithPolicy(path, legacyLedgerPolicy())
}

func reserveWithPolicy(path string, policy LedgerPolicy) (int, error) {
	policy = policy.effective()
	index := 0
	e := ledgerChangeWithPolicy(path, policy, func(l *Ledger) error {
		if l.HaltReason != "" {
			return fmt.Errorf("goal API ledger halted: %s; no request sent", l.HaltReason)
		}
		var spent int64
		for _, e := range l.Entries {
			spent += e.ReserveMicroUSD
		}
		if len(l.Entries) >= policy.MaxRequests || spent+RequestReserveMicroUSD > policy.MaxMicroUSD {
			return errors.New("goal API request/cost limit reached; no request sent")
		}
		index = len(l.Entries) + 1
		l.Entries = append(l.Entries, LedgerEntry{Index: index, Started: time.Now().UTC().Format(time.RFC3339Nano), Status: "reserved_unknown_outcome", Model: SelectionModel, ReserveMicroUSD: RequestReserveMicroUSD})
		return nil
	})
	return index, e
}

func finishReservation(path string, index int, status, id string, input, output int) error {
	return finishReservationWithPolicy(path, legacyLedgerPolicy(), index, status, id, input, output)
}

func finishReservationWithPolicy(path string, policy LedgerPolicy, index int, status, id string, input, output int) error {
	var settlementErr error
	err := ledgerChangeWithPolicy(path, policy, func(l *Ledger) error {
		if index < 1 || index > len(l.Entries) {
			return errors.New("reservation absent")
		}
		e := &l.Entries[index-1]
		if e.Status != "reserved_unknown_outcome" {
			return errors.New("reservation already settled")
		}
		e.Status = status
		e.ResponseID = id
		e.InputTokens = input
		e.OutputTokens = output
		// Persist a halt even when accounting cannot be trusted. Returning an
		// error from the mutation itself would discard this safety state.
		if input < 0 || output < 0 || input > 1_000_000 || output > 1_000_000 {
			l.HaltReason = "invalid or implausible provider usage"
			settlementErr = errors.New(l.HaltReason)
			return nil
		}
		// Full input price (ignore caching discounts); round microdollars upward.
		e.EstimatedMicroUSD = (int64(input)*4 + int64(output)*16 + 9) / 10
		if e.EstimatedMicroUSD > e.ReserveMicroUSD {
			l.HaltReason = "reported usage exceeds conservative reservation"
			settlementErr = errors.New(l.HaltReason)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return settlementErr
}

type reservedTransport struct {
	protocol extractionProtocol
	Path     string
	Policy   LedgerPolicy
	Index    int
	Base     http.RoundTripper
}

func (t *reservedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.Index != 0 {
		return nil, errors.New("automatic HTTP retries are forbidden")
	}
	if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/responses" {
		return nil, errors.New("only the approved OpenAI Responses endpoint is permitted")
	}
	// Even treating every request byte as a token is below the $0.05 reserve at
	// the recorded prices: 24k legacy input bytes (64KiB for owned-v4 only)
	// plus 1600 output tokens and ample overhead. No policy limit is increased.
	if r.ContentLength <= 0 || r.ContentLength > t.protocol.requestLimit() {
		return nil, fmt.Errorf("request size %d is outside the accounted bound", r.ContentLength)
	}
	index, e := reserveWithPolicy(t.Path, t.Policy)
	if e != nil {
		return nil, e
	}
	t.Index = index
	return t.Base.RoundTrip(r)
}
