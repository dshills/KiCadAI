package boardfamily

import (
	"context"
	"errors"
	"net/http"
)

// This closed experimental profile changes only the model on the provider wire.
// It does not change the default selector or the frozen v7 extraction contract.
const GroundedFullModel = "gpt-4.1-2025-04-14"
const GroundedFullAccountingProfile = "gpt-4.1-full-standard-2026-09-16"
const GroundedFullMaxRequestBytes = 16000

// Closed selection: never derive pricing or halt behavior from provider output.
func (p extractionProtocol) fullModelAccounting() bool {
	return p == groundedFullProtocol || p == sourceEligibleProtocol
}

func (p extractionProtocol) model() string {
	if p.fullModelAccounting() {
		return GroundedFullModel
	}
	return SelectionModel
}

func (p extractionProtocol) accountingProfile() string {
	if p == sourceEligibleProtocol {
		return SourceEligibleAccountingProfile
	}
	if p == groundedFullProtocol {
		return GroundedFullAccountingProfile
	}
	return ""
}

func (p extractionProtocol) estimatedMicroUSD(input, output int) int64 {
	// Standard, uncached prices. Full model: $2/$8 per million input/output
	// tokens. Mini: $0.40/$1.60; retain its exact historical rounding.
	// Source: https://developers.openai.com/api/docs/models/gpt-4.1
	// Checked 2026-09-16. New rates require a new profile, never an in-place edit.
	if p.fullModelAccounting() {
		return int64(input)*2 + int64(output)*8
	}
	return (int64(input)*4 + int64(output)*16 + 9) / 10
}

func (p extractionProtocol) validatePolicy(policy LedgerPolicy) error {
	if err := policy.validate(); err != nil {
		return err
	}
	if p.fullModelAccounting() && policy == legacyLedgerPolicy() {
		return errors.New("full-model comparison requires an explicit separate budget")
	}
	return nil
}

func (p extractionProtocol) validFullModelHistory(entries []LedgerEntry) bool {
	ids := map[string]bool{}
	for _, e := range entries {
		if e.Status != "completed" || e.ResponseID == "" || ids[e.ResponseID] || e.InputTokens < 0 || e.InputTokens > 1_000_000 || e.OutputTokens < 0 || e.OutputTokens > 1600 || e.EstimatedMicroUSD != p.estimatedMicroUSD(e.InputTokens, e.OutputTokens) || e.EstimatedMicroUSD > e.ReserveMicroUSD {
			return false
		}
		ids[e.ResponseID] = true
	}
	return true
}

// ledgerMatchesPolicy never infers a model from untrusted ledger contents.
func (p extractionProtocol) ledgerMatchesPolicy(l Ledger, policy LedgerPolicy) bool {
	if l.Goal != policy.Goal || l.AccountingProfile != p.accountingProfile() {
		return false
	}
	if p.fullModelAccounting() {
		return l.Version == 3 && l.MaxRequests == policy.MaxRequests && l.MaxMicroUSD == policy.MaxMicroUSD
	}
	if policy == legacyLedgerPolicy() {
		return l.Version == 1 && l.MaxRequests == 0 && l.MaxMicroUSD == 0
	}
	return l.Version == 2 && l.MaxRequests == policy.MaxRequests && l.MaxMicroUSD == policy.MaxMicroUSD
}

// InterpretGroundedFullWithJournal requires a fresh separately authorized
// budget and journal. Providing either is not itself permission to spend.
func InterpretGroundedFullWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, groundedFullProtocol)
}

// InspectGroundedFullJournal replays local bytes only; no key or network.
func InspectGroundedFullJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, groundedFullProtocol)
}

func GroundedFullEvidenceContract(prompt string) (map[string]any, error) {
	c, err := GroundedEvidenceContract(prompt)
	if err != nil {
		return nil, err
	}
	c["model"] = GroundedFullModel
	c["accounting_profile"] = GroundedFullAccountingProfile
	c["max_request_bytes"] = GroundedFullMaxRequestBytes
	c["journal_version"] = groundedFullProtocol.journalVersion()
	c["other_payload"] = "Complete original request and application-owned clause/quantity tables, unchanged v7 extraction instructions and partitioned schema. Only the provider model changes. Direct input JSON, attempt 1, no retries, no diagnostics, no gold answers, catalog, source code, geometry, environment values or credentials in the body. Requires fresh separate spending authorization, a full-model version-3 ledger and a new partitioned-full-v7 evidence journal; this export grants none."
	return c, nil
}
