package boardfamily

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// LedgerPolicy constrains a separately authorized goal. Supplying a policy is
// not authorization to spend. A ledger's goal and limits cannot change in place.
type LedgerPolicy struct {
	Goal        string `json:"goal"`
	MaxRequests int    `json:"max_requests"`
	MaxMicroUSD int64  `json:"max_micro_usd"`
}

func legacyLedgerPolicy() LedgerPolicy {
	return LedgerPolicy{ledgerGoal, MaxLiveRequests, MaxLiveMicroUSD}
}

func (p LedgerPolicy) effective() LedgerPolicy {
	if p == (LedgerPolicy{}) {
		return legacyLedgerPolicy()
	}
	return p
}

func (p LedgerPolicy) validate() error {
	if p == legacyLedgerPolicy() {
		return nil
	}
	if len(p.Goal) < 1 || len(p.Goal) > 100 || p.Goal == ledgerGoal {
		return errors.New("invalid or reserved budget goal")
	}
	for _, r := range p.Goal {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return errors.New("budget goal must use lowercase letters, digits and hyphens")
		}
	}
	if p.MaxRequests < 1 || p.MaxRequests > MaxLiveRequests || p.MaxMicroUSD < RequestReserveMicroUSD || p.MaxMicroUSD > MaxLiveMicroUSD {
		return errors.New("budget request/cost bounds are invalid")
	}
	return nil
}

// DecodeLedgerPolicy accepts only explicit new-goal policies, not the zero-value
// legacy fallback. Integer microdollars avoid floating-point budget comparisons.
func DecodeLedgerPolicy(r io.Reader) (LedgerPolicy, error) {
	var p LedgerPolicy
	b, err := io.ReadAll(io.LimitReader(r, 4097))
	if err != nil {
		return p, err
	}
	if len(b) > 4096 {
		return p, errors.New("budget policy exceeds 4096 bytes")
	}
	// Decode the exact key set, rejecting duplicate fields rather than accepting
	// JSON's otherwise ambiguous last-value-wins behavior for spending limits.
	d := json.NewDecoder(bytes.NewReader(b))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return p, errors.New("budget policy must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return p, err
		}
		key, ok := tok.(string)
		if !ok || seen[key] {
			return p, errors.New("duplicate or invalid budget policy field")
		}
		seen[key] = true
		switch key {
		case "goal":
			err = d.Decode(&p.Goal)
		case "max_requests":
			err = d.Decode(&p.MaxRequests)
		case "max_micro_usd":
			err = d.Decode(&p.MaxMicroUSD)
		default:
			return p, fmt.Errorf("unknown budget policy field %q", key)
		}
		if err != nil {
			return p, err
		}
	}
	if _, err = d.Token(); err != nil {
		return p, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return p, errors.New("trailing budget policy content")
	}
	if len(seen) != 3 || p.Goal == ledgerGoal {
		return p, errors.New("a complete new-goal budget policy is required")
	}
	return p, p.validate()
}
