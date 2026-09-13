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

// Increased from 20 to 35 by explicit user approval on September 13, 2026.
// The same ledger and every prior reservation remain in force; USD cap unchanged.
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
	Version    int           `json:"version"`
	Goal       string        `json:"goal"`
	HaltReason string        `json:"halt_reason,omitempty"`
	Entries    []LedgerEntry `json:"entries"`
}

const ledgerGoal = "board-family-v1-2026-09-13"

func ledgerChange(path string, f func(*Ledger) error) (err error) {
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
	l := Ledger{Version: 1, Goal: ledgerGoal, Entries: []LedgerEntry{}}
	b, e := os.ReadFile(path)
	if e == nil {
		if e = json.Unmarshal(b, &l); e != nil {
			return errors.New("invalid ledger; refusing to reset history")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if l.Version != 1 || l.Goal != ledgerGoal {
		return errors.New("ledger belongs to a different goal/version")
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
	return os.Rename(name, path)
}

func reserve(path string) (int, error) {
	index := 0
	e := ledgerChange(path, func(l *Ledger) error {
		if l.HaltReason != "" {
			return fmt.Errorf("goal API ledger halted: %s; no request sent", l.HaltReason)
		}
		var spent int64
		for _, e := range l.Entries {
			spent += e.ReserveMicroUSD
		}
		if len(l.Entries) >= MaxLiveRequests || spent+RequestReserveMicroUSD > MaxLiveMicroUSD {
			return errors.New("goal API request/cost limit reached; no request sent")
		}
		index = len(l.Entries) + 1
		l.Entries = append(l.Entries, LedgerEntry{Index: index, Started: time.Now().UTC().Format(time.RFC3339Nano), Status: "reserved_unknown_outcome", Model: SelectionModel, ReserveMicroUSD: RequestReserveMicroUSD})
		return nil
	})
	return index, e
}

func finishReservation(path string, index int, status, id string, input, output int) error {
	var settlementErr error
	err := ledgerChange(path, func(l *Ledger) error {
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
	Path  string
	Index int
	Base  http.RoundTripper
}

func (t *reservedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.Index != 0 {
		return nil, errors.New("automatic HTTP retries are forbidden")
	}
	if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/responses" {
		return nil, errors.New("only the approved OpenAI Responses endpoint is permitted")
	}
	// Even treating every request byte as a token is below the $0.05 reserve at
	// the recorded prices: 24k input bytes + 1600 output tokens + ample overhead.
	if r.ContentLength <= 0 || r.ContentLength > 24000 {
		return nil, fmt.Errorf("request size %d is outside the accounted bound", r.ContentLength)
	}
	index, e := reserve(t.Path)
	if e != nil {
		return nil, e
	}
	t.Index = index
	return t.Base.RoundTrip(r)
}
