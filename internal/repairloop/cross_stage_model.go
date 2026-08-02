package repairloop

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	CrossStageSchema  = "kicadai.cross-stage-autonomous-repair.v1"
	CrossStageVersion = 1
)

type CrossStage string

const (
	CrossStageRequirement  CrossStage = "requirement"
	CrossStageSynthesis    CrossStage = "synthesis"
	CrossStageSizing       CrossStage = "sizing"
	CrossStageSimulation   CrossStage = "simulation"
	CrossStageSchematic    CrossStage = "schematic"
	CrossStageERC          CrossStage = "erc"
	CrossStagePlacement    CrossStage = "placement"
	CrossStageRouting      CrossStage = "routing"
	CrossStageConnectivity CrossStage = "connectivity"
	CrossStageWriter       CrossStage = "writer"
	CrossStageRoundTrip    CrossStage = "roundtrip"
	CrossStageDRC          CrossStage = "drc"
)

var crossStageOrder = []CrossStage{
	CrossStageRequirement,
	CrossStageSynthesis,
	CrossStageSizing,
	CrossStageSimulation,
	CrossStageSchematic,
	CrossStageERC,
	CrossStagePlacement,
	CrossStageRouting,
	CrossStageConnectivity,
	CrossStageWriter,
	CrossStageRoundTrip,
	CrossStageDRC,
}

type CrossStageSeverity string

const (
	CrossStageSeverityInfo     CrossStageSeverity = "info"
	CrossStageSeverityWarning  CrossStageSeverity = "warning"
	CrossStageSeverityBlocking CrossStageSeverity = "blocking"
)

type CrossStageGateStatus string

const (
	CrossStageGateMissing CrossStageGateStatus = "missing"
	CrossStageGateBlocked CrossStageGateStatus = "blocked"
	CrossStageGateWarning CrossStageGateStatus = "warning"
	CrossStageGatePassed  CrossStageGateStatus = "passed"
)

type CrossStagePolicy struct {
	MaxTrials              int     `json:"max_trials"`
	MaxTrialsPerDiagnostic int     `json:"max_trials_per_diagnostic"`
	MarginTolerance        float64 `json:"margin_tolerance"`
}

func DefaultCrossStagePolicy() CrossStagePolicy {
	return CrossStagePolicy{MaxTrials: 24, MaxTrialsPerDiagnostic: 4, MarginTolerance: 1e-9}
}

type CrossStageDiagnostic struct {
	Stage        CrossStage         `json:"stage"`
	Code         string             `json:"code"`
	Category     string             `json:"category"`
	Severity     CrossStageSeverity `json:"severity"`
	EvidenceHash string             `json:"evidence_hash"`
	Scope        []string           `json:"scope"`
	Key          string             `json:"key"`
	Hash         string             `json:"hash"`
}

type CrossStageProposal struct {
	ID                  string       `json:"id"`
	DiagnosticHash      string       `json:"diagnostic_hash"`
	Operator            string       `json:"operator"`
	AffectedStages      []CrossStage `json:"affected_stages"`
	ReenterStage        CrossStage   `json:"reenter_stage"`
	ExpectedEffects     []string     `json:"expected_effects"`
	Scope               []string     `json:"scope"`
	ChangeCount         int          `json:"change_count"`
	NormalizedChange    float64      `json:"normalized_change"`
	ExpectedImprovement float64      `json:"expected_improvement"`
	Authorized          bool         `json:"authorized"`
	Rejection           string       `json:"rejection,omitempty"`
}

type CrossStageScopeHash struct {
	Scope string `json:"scope"`
	Hash  string `json:"hash"`
}

type CrossStageGate struct {
	ID           string               `json:"id"`
	Stage        CrossStage           `json:"stage"`
	Status       CrossStageGateStatus `json:"status"`
	Required     bool                 `json:"required"`
	EvidenceHash string               `json:"evidence_hash"`
}

type CrossStageMargin struct {
	ID           string     `json:"id"`
	Stage        CrossStage `json:"stage"`
	Headroom     float64    `json:"headroom"`
	Protected    bool       `json:"protected"`
	EvidenceHash string     `json:"evidence_hash"`
}

type CrossStageSnapshot struct {
	StateHash   string                `json:"state_hash"`
	ScopeHashes []CrossStageScopeHash `json:"scope_hashes"`
	Gates       []CrossStageGate      `json:"gates"`
	Margins     []CrossStageMargin    `json:"margins"`
	Hash        string                `json:"hash"`
}

type CrossStageCheckpoint struct {
	Snapshot CrossStageSnapshot `json:"snapshot"`
	Payload  []byte             `json:"-"`
}

type CrossStageTrial struct {
	Attempt             int                `json:"attempt"`
	Proposal            CrossStageProposal `json:"proposal"`
	Before              CrossStageSnapshot `json:"before"`
	Candidate           CrossStageSnapshot `json:"candidate,omitempty"`
	CandidateDiagnostic []string           `json:"candidate_diagnostics,omitempty"`
	ResolvedBlocking    int                `json:"resolved_blocking"`
	PassedGateDelta     int                `json:"passed_gate_delta"`
	Accepted            bool               `json:"accepted"`
	Selected            bool               `json:"selected"`
	Confirmed           bool               `json:"confirmed"`
	Restored            bool               `json:"restored"`
	Rejections          []string           `json:"rejections,omitempty"`
}

type CrossStageConsumption struct {
	Trials               int             `json:"trials"`
	TrialsByDiagnostic   []CrossStageUse `json:"trials_by_diagnostic"`
	CommittedRepairs     int             `json:"committed_repairs"`
	CheckpointRestores   int             `json:"checkpoint_restores"`
	RejectedCandidates   int             `json:"rejected_candidates"`
	ConfirmationAttempts int             `json:"confirmation_attempts"`
}

type CrossStageUse struct {
	DiagnosticKey string `json:"diagnostic_key"`
	Count         int    `json:"count"`
}

type CrossStageReport struct {
	Schema             string                 `json:"schema"`
	Version            int                    `json:"version"`
	Policy             CrossStagePolicy       `json:"policy"`
	Initial            CrossStageSnapshot     `json:"initial"`
	Final              CrossStageSnapshot     `json:"final"`
	InitialDiagnostics []CrossStageDiagnostic `json:"initial_diagnostics"`
	FinalDiagnostics   []CrossStageDiagnostic `json:"final_diagnostics"`
	Diagnostics        []CrossStageDiagnostic `json:"diagnostics"`
	Proposals          []CrossStageProposal   `json:"proposals"`
	Trials             []CrossStageTrial      `json:"trials"`
	Consumption        CrossStageConsumption  `json:"consumption"`
	Status             string                 `json:"status"`
	StopReason         string                 `json:"stop_reason"`
	Trace              Trace                  `json:"trace"`
	Hash               string                 `json:"hash"`
}

const (
	CrossStageStatusPassed  = "passed"
	CrossStageStatusBlocked = "blocked"

	CrossStageStopPassed                  = "all_required_gates_passed"
	CrossStageStopNoBlockingDiagnostic    = "required_gate_lacks_blocking_diagnostic"
	CrossStageStopNoAuthorizedProposal    = "no_authorized_proposal"
	CrossStageStopNoSafeImprovement       = "no_safe_improvement"
	CrossStageStopBudgetExhausted         = "trial_budget_exhausted"
	CrossStageStopRepeatedState           = "repeated_committed_state"
	CrossStageStopContextCanceled         = "context_canceled"
	CrossStageStopRestoreFailed           = "checkpoint_restore_failed"
	CrossStageStopConfirmationMismatch    = "confirmation_mismatch"
	CrossStageStopInvalidTargetEvidence   = "invalid_target_evidence"
	CrossStageStopConfirmationApplyFailed = "confirmation_apply_failed"
)

type CrossStageTarget interface {
	Capture(context.Context) (CrossStageCheckpoint, error)
	Restore(context.Context, CrossStageCheckpoint) error
	Diagnose(context.Context) ([]CrossStageDiagnostic, error)
	Propose(context.Context, CrossStageDiagnostic) ([]CrossStageProposal, error)
	Apply(context.Context, CrossStageProposal) error
	Reenter(context.Context, CrossStage) error
}

func NewCrossStageDiagnostic(stage CrossStage, code, category string, severity CrossStageSeverity, evidenceHash string, scope []string) CrossStageDiagnostic {
	diagnostic := CrossStageDiagnostic{
		Stage: stage, Code: strings.TrimSpace(code), Category: strings.TrimSpace(category),
		Severity: severity, EvidenceHash: strings.TrimSpace(evidenceHash), Scope: crossStageStrings(scope),
	}
	diagnostic.Key = crossStageHash(struct {
		Stage    CrossStage
		Code     string
		Category string
		Scope    []string
	}{diagnostic.Stage, diagnostic.Code, diagnostic.Category, diagnostic.Scope})
	diagnostic.Hash = crossStageHash(struct {
		Key          string
		Severity     CrossStageSeverity
		EvidenceHash string
	}{diagnostic.Key, diagnostic.Severity, diagnostic.EvidenceHash})
	return diagnostic
}

func NewCrossStageProposal(diagnostic CrossStageDiagnostic, operator string, affectedStages []CrossStage, expectedEffects, scope []string, changeCount int, normalizedChange, expectedImprovement float64, authorized bool, rejection string) CrossStageProposal {
	proposal := CrossStageProposal{
		DiagnosticHash: diagnostic.Hash, Operator: strings.TrimSpace(operator),
		AffectedStages: crossStages(affectedStages), ExpectedEffects: crossStageStrings(expectedEffects), Scope: crossStageStrings(scope),
		ChangeCount: changeCount, NormalizedChange: normalizedChange, ExpectedImprovement: expectedImprovement,
		Authorized: authorized, Rejection: strings.TrimSpace(rejection),
	}
	if len(proposal.AffectedStages) > 0 {
		proposal.ReenterStage = proposal.AffectedStages[0]
	}
	copy := proposal
	copy.ID = ""
	proposal.ID = "cross-stage-" + crossStageHash(copy)[:16]
	return proposal
}

func NewCrossStageCheckpoint(payload []byte, scopeHashes []CrossStageScopeHash, gates []CrossStageGate, margins []CrossStageMargin) CrossStageCheckpoint {
	payload = slices.Clone(payload)
	snapshot := normalizeCrossStageSnapshot(CrossStageSnapshot{
		StateHash: crossStageBytesHash(payload), ScopeHashes: scopeHashes, Gates: gates, Margins: margins,
	})
	return CrossStageCheckpoint{Snapshot: snapshot, Payload: payload}
}

func CrossStageRank(stage CrossStage) int {
	for index, candidate := range crossStageOrder {
		if stage == candidate {
			return index
		}
	}
	return -1
}

func ValidateCrossStageDiagnostic(diagnostic CrossStageDiagnostic) error {
	if CrossStageRank(diagnostic.Stage) < 0 || diagnostic.Code == "" || diagnostic.Category == "" || diagnostic.EvidenceHash == "" || len(diagnostic.Scope) == 0 {
		return errors.New("cross-stage diagnostic lacks structured stage, code, category, evidence, or scope")
	}
	if diagnostic.Severity != CrossStageSeverityInfo && diagnostic.Severity != CrossStageSeverityWarning && diagnostic.Severity != CrossStageSeverityBlocking {
		return fmt.Errorf("unknown cross-stage diagnostic severity %q", diagnostic.Severity)
	}
	canonical := NewCrossStageDiagnostic(diagnostic.Stage, diagnostic.Code, diagnostic.Category, diagnostic.Severity, diagnostic.EvidenceHash, diagnostic.Scope)
	if diagnostic.Key != canonical.Key || diagnostic.Hash != canonical.Hash {
		return errors.New("cross-stage diagnostic hashes are not canonical")
	}
	return nil
}

func ValidateCrossStageProposal(proposal CrossStageProposal, diagnostic CrossStageDiagnostic) error {
	if proposal.DiagnosticHash != diagnostic.Hash || proposal.Operator == "" || len(proposal.AffectedStages) == 0 ||
		len(proposal.ExpectedEffects) == 0 || len(proposal.Scope) == 0 || proposal.ChangeCount <= 0 ||
		math.IsNaN(proposal.NormalizedChange) || math.IsInf(proposal.NormalizedChange, 0) || proposal.NormalizedChange < 0 {
		return errors.New("cross-stage proposal lacks bounded structured evidence")
	}
	if math.IsNaN(proposal.ExpectedImprovement) || math.IsInf(proposal.ExpectedImprovement, 0) || proposal.ExpectedImprovement < 0 {
		return errors.New("cross-stage proposal lacks bounded improvement evidence")
	}
	if proposal.ReenterStage != proposal.AffectedStages[0] || CrossStageRank(proposal.ReenterStage) < 0 || CrossStageRank(proposal.ReenterStage) > CrossStageRank(diagnostic.Stage) {
		return errors.New("cross-stage proposal does not re-enter at its earliest affected stage")
	}
	canonical := NewCrossStageProposal(diagnostic, proposal.Operator, proposal.AffectedStages, proposal.ExpectedEffects, proposal.Scope, proposal.ChangeCount, proposal.NormalizedChange, proposal.ExpectedImprovement, proposal.Authorized, proposal.Rejection)
	if proposal.ID != canonical.ID {
		return errors.New("cross-stage proposal ID is not canonical")
	}
	if proposal.Authorized && proposal.Rejection != "" || !proposal.Authorized && proposal.Rejection == "" {
		return errors.New("cross-stage proposal authorization and rejection disagree")
	}
	return nil
}

func ValidateCrossStageCheckpoint(checkpoint CrossStageCheckpoint) error {
	if checkpoint.Snapshot.StateHash == "" || checkpoint.Snapshot.StateHash != crossStageBytesHash(checkpoint.Payload) {
		return errors.New("cross-stage checkpoint payload hash mismatch")
	}
	return validateCrossStageSnapshot(checkpoint.Snapshot)
}

func validateCrossStageSnapshot(snapshot CrossStageSnapshot) error {
	canonical := normalizeCrossStageSnapshot(snapshot)
	if snapshot.StateHash == "" || snapshot.Hash != canonical.Hash {
		return errors.New("cross-stage checkpoint snapshot is not canonical")
	}
	seenScopes := map[string]struct{}{}
	for _, scope := range snapshot.ScopeHashes {
		if scope.Scope == "" || scope.Hash == "" {
			return errors.New("cross-stage checkpoint has incomplete scope hash")
		}
		if _, exists := seenScopes[scope.Scope]; exists {
			return errors.New("cross-stage checkpoint has duplicate scope hash")
		}
		seenScopes[scope.Scope] = struct{}{}
	}
	seenGates := map[string]struct{}{}
	for _, gate := range snapshot.Gates {
		if gate.ID == "" || CrossStageRank(gate.Stage) < 0 || crossStageGateRank(gate.Status) < 0 || gate.EvidenceHash == "" {
			return errors.New("cross-stage checkpoint has incomplete gate evidence")
		}
		if _, exists := seenGates[gate.ID]; exists {
			return errors.New("cross-stage checkpoint has duplicate gate evidence")
		}
		seenGates[gate.ID] = struct{}{}
	}
	seenMargins := map[string]struct{}{}
	for _, margin := range snapshot.Margins {
		if margin.ID == "" || CrossStageRank(margin.Stage) < 0 || margin.EvidenceHash == "" || math.IsNaN(margin.Headroom) || math.IsInf(margin.Headroom, 0) {
			return errors.New("cross-stage checkpoint has incomplete margin evidence")
		}
		if _, exists := seenMargins[margin.ID]; exists {
			return errors.New("cross-stage checkpoint has duplicate margin evidence")
		}
		seenMargins[margin.ID] = struct{}{}
	}
	return nil
}

func ValidateCrossStageReport(report CrossStageReport) error {
	if report.Schema != CrossStageSchema || report.Version != CrossStageVersion {
		return errors.New("cross-stage report schema or version mismatch")
	}
	if report.Policy != normalizeCrossStagePolicy(report.Policy) || report.Consumption.Trials > report.Policy.MaxTrials || report.Consumption.Trials != len(report.Trials) {
		return errors.New("cross-stage report budget evidence is inconsistent")
	}
	if err := validateCrossStageSnapshot(report.Initial); err != nil {
		return err
	}
	if err := validateCrossStageSnapshot(report.Final); err != nil {
		return err
	}
	diagnosticByHash := make(map[string]CrossStageDiagnostic, len(report.Diagnostics))
	for _, diagnostic := range report.Diagnostics {
		if err := ValidateCrossStageDiagnostic(diagnostic); err != nil {
			return err
		}
		diagnosticByHash[diagnostic.Hash] = diagnostic
	}
	for _, proposal := range report.Proposals {
		diagnostic, ok := diagnosticByHash[proposal.DiagnosticHash]
		if !ok {
			return errors.New("cross-stage report proposal references missing diagnostic")
		}
		if err := ValidateCrossStageProposal(proposal, diagnostic); err != nil {
			return err
		}
	}
	committed := 0
	for index, trial := range report.Trials {
		if trial.Attempt != index+1 {
			return errors.New("cross-stage trial sequence is not contiguous")
		}
		if _, ok := diagnosticByHash[trial.Proposal.DiagnosticHash]; !ok {
			return errors.New("cross-stage trial references missing diagnostic")
		}
		if err := validateCrossStageSnapshot(trial.Before); err != nil {
			return err
		}
		if trial.Candidate.Hash != "" {
			if err := validateCrossStageSnapshot(trial.Candidate); err != nil {
				return err
			}
		}
		if trial.Confirmed && (!trial.Accepted || !trial.Selected) {
			return errors.New("confirmed cross-stage trial was not accepted and selected")
		}
		if trial.Confirmed {
			committed++
		}
	}
	if committed != report.Consumption.CommittedRepairs {
		return errors.New("cross-stage committed repair count is inconsistent")
	}
	if report.Status == CrossStageStatusPassed {
		if report.StopReason != CrossStageStopPassed || !crossStageRequiredGatesPassed(report.Final) || len(blockingCrossStageDiagnostics(report.FinalDiagnostics)) != 0 {
			return errors.New("passing cross-stage report lacks passing final evidence")
		}
	} else if report.Status != CrossStageStatusBlocked {
		return errors.New("cross-stage report has unknown status")
	}
	if report.Trace.Schema != Schema || report.Trace.Version != Version || report.Trace.Budget != report.Policy.MaxTrials || report.Trace.Consumed != report.Consumption.Trials {
		return errors.New("cross-stage report shared trace is incomplete")
	}
	traceCopy := NewTrace(report.Trace.Budget, report.Trace.Consumed, report.Trace.Diagnostics, report.Trace.Proposals, report.Trace.Outcomes)
	if traceCopy.Hash != report.Trace.Hash {
		return errors.New("cross-stage report shared trace hash mismatch")
	}
	copy := report
	copy.Hash = ""
	if report.Hash == "" || report.Hash != crossStageHash(copy) {
		return errors.New("cross-stage report hash mismatch")
	}
	return nil
}

func normalizeCrossStagePolicy(policy CrossStagePolicy) CrossStagePolicy {
	defaults := DefaultCrossStagePolicy()
	if policy.MaxTrials <= 0 {
		policy.MaxTrials = defaults.MaxTrials
	}
	if policy.MaxTrialsPerDiagnostic <= 0 {
		policy.MaxTrialsPerDiagnostic = defaults.MaxTrialsPerDiagnostic
	}
	if math.IsNaN(policy.MarginTolerance) || math.IsInf(policy.MarginTolerance, 0) || policy.MarginTolerance < 0 {
		policy.MarginTolerance = defaults.MarginTolerance
	}
	return policy
}

func normalizeCrossStageDiagnostics(source []CrossStageDiagnostic) []CrossStageDiagnostic {
	result := make([]CrossStageDiagnostic, 0, len(source))
	for _, diagnostic := range source {
		result = append(result, NewCrossStageDiagnostic(diagnostic.Stage, diagnostic.Code, diagnostic.Category, diagnostic.Severity, diagnostic.EvidenceHash, diagnostic.Scope))
	}
	slices.SortFunc(result, compareCrossStageDiagnostic)
	return slices.CompactFunc(result, func(left, right CrossStageDiagnostic) bool { return left.Hash == right.Hash })
}

func compareCrossStageDiagnostic(left, right CrossStageDiagnostic) int {
	return cmp.Or(
		cmp.Compare(CrossStageRank(left.Stage), CrossStageRank(right.Stage)),
		cmp.Compare(left.Key, right.Key),
		cmp.Compare(left.Hash, right.Hash),
	)
}

func normalizeCrossStageProposals(source []CrossStageProposal, diagnostics map[string]CrossStageDiagnostic) []CrossStageProposal {
	result := make([]CrossStageProposal, 0, len(source))
	for _, proposal := range source {
		diagnostic, ok := diagnostics[proposal.DiagnosticHash]
		if !ok {
			result = append(result, proposal)
			continue
		}
		result = append(result, NewCrossStageProposal(diagnostic, proposal.Operator, proposal.AffectedStages, proposal.ExpectedEffects, proposal.Scope, proposal.ChangeCount, proposal.NormalizedChange, proposal.ExpectedImprovement, proposal.Authorized, proposal.Rejection))
	}
	slices.SortFunc(result, compareCrossStageProposal)
	return slices.CompactFunc(result, func(left, right CrossStageProposal) bool { return left.ID == right.ID })
}

func compareCrossStageProposal(left, right CrossStageProposal) int {
	return cmp.Or(
		cmp.Compare(left.ChangeCount, right.ChangeCount),
		cmp.Compare(left.NormalizedChange, right.NormalizedChange),
		cmp.Compare(CrossStageRank(left.ReenterStage), CrossStageRank(right.ReenterStage)),
		cmp.Compare(left.ID, right.ID),
	)
}

func normalizeCrossStageSnapshot(snapshot CrossStageSnapshot) CrossStageSnapshot {
	snapshot.ScopeHashes = slices.Clone(snapshot.ScopeHashes)
	for index := range snapshot.ScopeHashes {
		snapshot.ScopeHashes[index].Scope = strings.TrimSpace(snapshot.ScopeHashes[index].Scope)
		snapshot.ScopeHashes[index].Hash = strings.TrimSpace(snapshot.ScopeHashes[index].Hash)
	}
	slices.SortFunc(snapshot.ScopeHashes, func(left, right CrossStageScopeHash) int { return cmp.Compare(left.Scope, right.Scope) })
	snapshot.ScopeHashes = slices.CompactFunc(snapshot.ScopeHashes, func(left, right CrossStageScopeHash) bool { return left.Scope == right.Scope })
	snapshot.Gates = slices.Clone(snapshot.Gates)
	for index := range snapshot.Gates {
		snapshot.Gates[index].ID = strings.TrimSpace(snapshot.Gates[index].ID)
		snapshot.Gates[index].EvidenceHash = strings.TrimSpace(snapshot.Gates[index].EvidenceHash)
	}
	slices.SortFunc(snapshot.Gates, func(left, right CrossStageGate) int { return cmp.Compare(left.ID, right.ID) })
	snapshot.Gates = slices.CompactFunc(snapshot.Gates, func(left, right CrossStageGate) bool { return left.ID == right.ID })
	snapshot.Margins = slices.Clone(snapshot.Margins)
	for index := range snapshot.Margins {
		snapshot.Margins[index].ID = strings.TrimSpace(snapshot.Margins[index].ID)
		snapshot.Margins[index].EvidenceHash = strings.TrimSpace(snapshot.Margins[index].EvidenceHash)
	}
	slices.SortFunc(snapshot.Margins, func(left, right CrossStageMargin) int { return cmp.Compare(left.ID, right.ID) })
	snapshot.Margins = slices.CompactFunc(snapshot.Margins, func(left, right CrossStageMargin) bool { return left.ID == right.ID })
	copy := snapshot
	copy.Hash = ""
	snapshot.Hash = crossStageHash(copy)
	return snapshot
}

func crossStages(source []CrossStage) []CrossStage {
	result := slices.Clone(source)
	slices.SortFunc(result, func(left, right CrossStage) int { return cmp.Compare(CrossStageRank(left), CrossStageRank(right)) })
	return slices.Compact(result)
}

func crossStageStrings(source []string) []string {
	result := make([]string, 0, len(source))
	for _, value := range source {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func crossStageGateRank(status CrossStageGateStatus) int {
	switch status {
	case CrossStageGateMissing:
		return 0
	case CrossStageGateBlocked:
		return 1
	case CrossStageGateWarning:
		return 2
	case CrossStageGatePassed:
		return 3
	default:
		return -1
	}
}

func crossStageHash(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic("cross-stage hash invariant: " + err.Error())
	}
	return crossStageBytesHash(data)
}

func crossStageBytesHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
