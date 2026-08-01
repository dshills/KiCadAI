// Package repairloop defines identity-neutral evidence shared by electrical
// and physical diagnosis-driven correction loops.
package repairloop

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

const (
	Schema  = "kicadai.diagnosis-driven-repair.v1"
	Version = 1
)

type Diagnostic struct {
	Stage        string   `json:"stage"`
	Code         string   `json:"code"`
	Category     string   `json:"category"`
	Direction    string   `json:"direction,omitempty"`
	EvidenceHash string   `json:"evidence_hash"`
	Scope        []string `json:"scope,omitempty"`
	Hash         string   `json:"hash"`
}

type Proposal struct {
	ID             string   `json:"id"`
	DiagnosticHash string   `json:"diagnostic_hash"`
	Kind           string   `json:"kind"`
	ReenterStage   string   `json:"reenter_stage"`
	ExpectedEffect string   `json:"expected_effect"`
	Scope          []string `json:"scope,omitempty"`
	Authorized     bool     `json:"authorized"`
	Rejection      string   `json:"rejection,omitempty"`
}

type Outcome struct {
	ProposalID string `json:"proposal_id"`
	Status     string `json:"status"`
	BeforeHash string `json:"before_hash"`
	AfterHash  string `json:"after_hash,omitempty"`
	ResultHash string `json:"result_hash,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type Trace struct {
	Schema      string       `json:"schema"`
	Version     int          `json:"version"`
	Budget      int          `json:"budget"`
	Consumed    int          `json:"consumed"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Proposals   []Proposal   `json:"proposals"`
	Outcomes    []Outcome    `json:"outcomes"`
	Hash        string       `json:"hash"`
}

func NewTrace(budget, consumed int, diagnostics []Diagnostic, proposals []Proposal, outcomes []Outcome) Trace {
	trace := Trace{Schema: Schema, Version: Version, Budget: max(0, budget), Consumed: max(0, consumed)}
	trace.Diagnostics = normalizeDiagnostics(diagnostics)
	trace.Proposals = normalizeProposals(proposals)
	trace.Outcomes = normalizeOutcomes(outcomes)
	copy := trace
	copy.Hash = ""
	trace.Hash = hash(copy)
	return trace
}

func NewDiagnostic(stage, code, category, direction, evidenceHash string, scope []string) Diagnostic {
	diagnostic := Diagnostic{
		Stage: strings.TrimSpace(stage), Code: strings.TrimSpace(code),
		Category: strings.TrimSpace(category), Direction: strings.TrimSpace(direction),
		EvidenceHash: strings.TrimSpace(evidenceHash), Scope: compactStrings(scope),
	}
	copy := diagnostic
	copy.Hash = ""
	diagnostic.Hash = hash(copy)
	return diagnostic
}

func NewProposal(diagnostic Diagnostic, kind, reenterStage, expectedEffect string, scope []string, authorized bool, rejection string) Proposal {
	proposal := Proposal{
		DiagnosticHash: diagnostic.Hash, Kind: strings.TrimSpace(kind),
		ReenterStage: strings.TrimSpace(reenterStage), ExpectedEffect: strings.TrimSpace(expectedEffect),
		Scope: compactStrings(scope), Authorized: authorized, Rejection: strings.TrimSpace(rejection),
	}
	proposal.ID = "repair-" + hash(proposal)[:16]
	return proposal
}

func normalizeDiagnostics(source []Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(source))
	for _, item := range source {
		item.Scope = compactStrings(item.Scope)
		copy := item
		copy.Hash = ""
		item.Hash = hash(copy)
		result = append(result, item)
	}
	slices.SortFunc(result, func(left, right Diagnostic) int { return cmp.Compare(left.Hash, right.Hash) })
	return slices.CompactFunc(result, func(left, right Diagnostic) bool { return left.Hash == right.Hash })
}

func normalizeProposals(source []Proposal) []Proposal {
	result := append([]Proposal(nil), source...)
	for index := range result {
		result[index].Scope = compactStrings(result[index].Scope)
		copy := result[index]
		copy.ID = ""
		result[index].ID = "repair-" + hash(copy)[:16]
	}
	slices.SortFunc(result, func(left, right Proposal) int { return cmp.Compare(left.ID, right.ID) })
	return slices.CompactFunc(result, func(left, right Proposal) bool { return left.ID == right.ID })
}

func normalizeOutcomes(source []Outcome) []Outcome {
	result := append([]Outcome(nil), source...)
	slices.SortFunc(result, func(left, right Outcome) int {
		return cmp.Or(cmp.Compare(left.ProposalID, right.ProposalID), cmp.Compare(left.ResultHash, right.ResultHash), cmp.Compare(left.Status, right.Status))
	})
	return result
}

func compactStrings(source []string) []string {
	result := make([]string, 0, len(source))
	for _, item := range source {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func hash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
