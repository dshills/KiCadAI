// Package behavioralintent defines the fail-closed boundary between ordinary
// circuit-design language and KiCadAI's topology-neutral behavioral contract.
package behavioralintent

import (
	"kicadai/internal/architecturesearch"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/reports"
)

const (
	ProposalVersion  = 1
	ResultSchema     = "kicadai.behavioral-intent-compilation.v1"
	ResultVersion    = 1
	MaxProposalBytes = architecturesearch.MaxRequirementBytes
	FollowUpSchema   = "kicadai.behavioral-intent-follow-up.v1"
	FollowUpVersion  = 1
	MaxAnswerBytes   = 4096
)

type Status string

const (
	StatusReady              Status = "ready"
	StatusNeedsClarification Status = "needs_clarification"
	StatusUnsupported        Status = "unsupported"
	StatusInvalid            Status = "invalid"
)

type Disposition string

const (
	DispositionCompiled      Disposition = "compiled"
	DispositionClarification Disposition = "clarification"
	DispositionCapabilityGap Disposition = "capability_gap"
	DispositionContext       Disposition = "context"
)

type UncertaintyResolution string

const (
	ResolutionExplicit      UncertaintyResolution = "explicit"
	ResolutionBounded       UncertaintyResolution = "bounded"
	ResolutionClarification UncertaintyResolution = "clarification"
	ResolutionCapabilityGap UncertaintyResolution = "capability_gap"
)

// Proposal is untrusted AI output. Compile is the only supported way to turn a
// proposal into an executable architecture requirement.
type Proposal struct {
	Version        int                             `json:"version"`
	Requirement    *architecturesearch.Requirement `json:"requirement,omitempty"`
	Coverage       []CoverageRecord                `json:"coverage"`
	Uncertainties  []Uncertainty                   `json:"uncertainties"`
	Clarifications []Clarification                 `json:"clarifications"`
	CapabilityGaps []CapabilityGap                 `json:"capability_gaps"`
}

// Source is deterministic compiler-owned prompt evidence. Providers receive
// statement IDs and account for them; they never choose source boundaries.
type Source struct {
	SHA256     string            `json:"sha256"`
	ByteLength int               `json:"byte_length"`
	Statements []SourceStatement `json:"statements"`
}

type SourceStatement struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	StartByte int    `json:"start_byte"`
	EndByte   int    `json:"end_byte"`
}

type CoverageRecord struct {
	StatementID string      `json:"statement_id"`
	Disposition Disposition `json:"disposition"`
	Rationale   string      `json:"rationale"`
	References  []Reference `json:"references"`
}

type Reference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Uncertainty records how an uncertain source claim was made safe. Ready
// compilations may retain only explicit or bounded uncertainty.
type Uncertainty struct {
	ID          string                `json:"id"`
	Path        string                `json:"path"`
	Kind        string                `json:"kind"`
	Description string                `json:"description"`
	Resolution  UncertaintyResolution `json:"resolution"`
	ResolvedBy  string                `json:"resolved_by,omitempty"`
}

type Clarification struct {
	ID             string   `json:"id"`
	Path           string   `json:"path"`
	Question       string   `json:"question"`
	WhyNeeded      string   `json:"why_needed"`
	Choices        []string `json:"choices,omitempty"`
	UncertaintyIDs []string `json:"uncertainty_ids"`
}

// FollowUp is a compiler-bound answer envelope. Hashes prevent answers from
// being replayed against a different prompt, capability snapshot, proposal, or
// prior compilation.
type FollowUp struct {
	Schema                 string                `json:"schema"`
	Version                int                   `json:"version"`
	SourceSHA256           string                `json:"source_sha256"`
	CapabilitySHA256       string                `json:"capability_sha256"`
	PriorProposalSHA256    string                `json:"prior_proposal_sha256"`
	PriorCompilationSHA256 string                `json:"prior_compilation_sha256"`
	Answers                []ClarificationAnswer `json:"answers"`
}

type ClarificationAnswer struct {
	ClarificationID string   `json:"clarification_id"`
	UncertaintyIDs  []string `json:"uncertainty_ids"`
	Answer          string   `json:"answer"`
}

// CapabilityGap is a stable, machine-readable refusal to guess. Capability is
// semantic (for example, phase_margin_analysis), never a fixture or topology.
type CapabilityGap struct {
	ID               string   `json:"id"`
	Capability       string   `json:"capability"`
	Path             string   `json:"path"`
	Reason           string   `json:"reason"`
	RequiredEvidence []string `json:"required_evidence"`
}

type Result struct {
	Schema           string                          `json:"schema"`
	Version          int                             `json:"version"`
	Status           Status                          `json:"status"`
	Source           Source                          `json:"source"`
	CapabilitySHA256 string                          `json:"capability_sha256"`
	Architecture     *ArchitectureEvidence           `json:"architecture,omitempty"`
	ClosedLoop       *ClosedLoopEvidence             `json:"closed_loop,omitempty"`
	Requirement      *architecturesearch.Requirement `json:"requirement,omitempty"`
	Coverage         []CoverageRecord                `json:"coverage"`
	Uncertainties    []Uncertainty                   `json:"uncertainties"`
	Clarifications   []Clarification                 `json:"clarifications"`
	CapabilityGaps   []CapabilityGap                 `json:"capability_gaps"`
	Issues           []reports.Issue                 `json:"issues,omitempty"`
}

type ClosedLoopEvidence struct {
	Status              string                         `json:"status"`
	StopReason          closedloopsynthesis.StopReason `json:"stop_reason"`
	RequirementHash     string                         `json:"requirement_hash"`
	RegistryHash        string                         `json:"registry_hash"`
	CatalogHash         string                         `json:"catalog_hash"`
	ModelRegistryHash   string                         `json:"model_registry_hash"`
	SelectedCircuitHash string                         `json:"selected_circuit_hash,omitempty"`
}

type ArchitectureEvidence struct {
	Status          architecturesearch.SearchStatus `json:"status"`
	RequirementHash string                          `json:"requirement_hash"`
	RegistryHash    string                          `json:"registry_hash"`
	CatalogHash     string                          `json:"catalog_hash,omitempty"`
}
