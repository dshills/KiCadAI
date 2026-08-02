package repairloop

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func RunCrossStageRepair(ctx context.Context, target CrossStageTarget, policy CrossStagePolicy) (CrossStageReport, error) {
	if target == nil {
		return CrossStageReport{}, errors.New("cross-stage repair target is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	policy = normalizeCrossStagePolicy(policy)
	initial, err := captureCrossStageTarget(ctx, target)
	if err != nil {
		return CrossStageReport{}, err
	}
	initialDiagnostics, err := diagnoseCrossStageTarget(ctx, target)
	if err != nil {
		return CrossStageReport{}, err
	}
	report := CrossStageReport{
		Schema: CrossStageSchema, Version: CrossStageVersion, Policy: policy,
		Initial: initial.Snapshot, Final: initial.Snapshot, InitialDiagnostics: initialDiagnostics,
		Status: CrossStageStatusBlocked, StopReason: CrossStageStopNoSafeImprovement,
	}
	diagnosticSeen := map[string]struct{}{}
	allDiagnostics := appendUniqueCrossStageDiagnostics(nil, diagnosticSeen, initialDiagnostics)
	allProposals := []CrossStageProposal{}
	trialsByDiagnostic := map[string]int{}
	evaluated := map[string]struct{}{}
	seenCommittedStates := map[string]struct{}{initial.Snapshot.StateHash: {}}

	for {
		if err := ctx.Err(); err != nil {
			report.StopReason = CrossStageStopContextCanceled
			break
		}
		base, err := captureCrossStageTarget(ctx, target)
		if err != nil {
			return CrossStageReport{}, err
		}
		diagnostics, err := diagnoseCrossStageTarget(ctx, target)
		if err != nil {
			return CrossStageReport{}, err
		}
		allDiagnostics = appendUniqueCrossStageDiagnostics(allDiagnostics, diagnosticSeen, diagnostics)
		blocking := blockingCrossStageDiagnostics(diagnostics)
		if len(blocking) == 0 {
			if crossStageRequiredGatesPassed(base.Snapshot) {
				report.Status = CrossStageStatusPassed
				report.StopReason = CrossStageStopPassed
			} else {
				report.StopReason = CrossStageStopNoBlockingDiagnostic
			}
			break
		}
		if report.Consumption.Trials >= policy.MaxTrials {
			report.StopReason = CrossStageStopBudgetExhausted
			break
		}
		earliest := blocking[0].Stage
		stageDiagnostics := make([]CrossStageDiagnostic, 0, len(blocking))
		for _, diagnostic := range blocking {
			if diagnostic.Stage == earliest {
				stageDiagnostics = append(stageDiagnostics, diagnostic)
			}
		}
		proposals, err := crossStageTargetProposals(ctx, target, stageDiagnostics)
		if err != nil {
			return CrossStageReport{}, err
		}
		allProposals = append(allProposals, proposals...)
		authorized := make([]CrossStageProposal, 0, len(proposals))
		for _, proposal := range proposals {
			if proposal.Authorized {
				authorized = append(authorized, proposal)
			}
		}
		if len(authorized) == 0 {
			report.StopReason = CrossStageStopNoAuthorizedProposal
			break
		}

		acceptedTrials := []int{}
		for _, proposal := range authorized {
			if report.Consumption.Trials >= policy.MaxTrials {
				break
			}
			diagnostic := crossStageDiagnosticByHash(stageDiagnostics, proposal.DiagnosticHash)
			if trialsByDiagnostic[diagnostic.Key] >= policy.MaxTrialsPerDiagnostic {
				continue
			}
			evaluationKey := base.Snapshot.StateHash + ":" + proposal.ID
			if _, exists := evaluated[evaluationKey]; exists {
				continue
			}
			evaluated[evaluationKey] = struct{}{}
			trial := CrossStageTrial{Attempt: report.Consumption.Trials + 1, Proposal: proposal, Before: base.Snapshot}
			report.Consumption.Trials++
			trialsByDiagnostic[diagnostic.Key]++
			applyErr := target.Apply(ctx, proposal)
			if applyErr != nil {
				trial.Rejections = []string{"apply_failed"}
			} else if err := target.Reenter(ctx, proposal.ReenterStage); err != nil {
				trial.Rejections = []string{"stage_reentry_failed"}
			} else {
				candidate, captureErr := captureCrossStageTarget(ctx, target)
				if captureErr != nil {
					trial.Rejections = []string{"candidate_evidence_invalid"}
				} else {
					trial.Candidate = candidate.Snapshot
					candidateDiagnostics, diagnoseErr := diagnoseCrossStageTarget(ctx, target)
					if diagnoseErr != nil {
						trial.Rejections = []string{"candidate_diagnosis_failed"}
					} else {
						allDiagnostics = appendUniqueCrossStageDiagnostics(allDiagnostics, diagnosticSeen, candidateDiagnostics)
						trial.CandidateDiagnostic = crossStageDiagnosticHashes(candidateDiagnostics)
						trial.Rejections, trial.ResolvedBlocking, trial.PassedGateDelta = assessCrossStageCandidate(
							base.Snapshot, diagnostics, candidate.Snapshot, candidateDiagnostics, diagnostic, proposal, policy.MarginTolerance,
						)
						trial.Accepted = len(trial.Rejections) == 0
					}
				}
			}
			restored, restoreErr := restoreCrossStageTarget(ctx, target, base)
			trial.Restored = restored
			report.Consumption.CheckpointRestores++
			if restoreErr != nil {
				report.Trials = append(report.Trials, normalizeCrossStageTrial(trial))
				report.StopReason = CrossStageStopRestoreFailed
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), restoreErr
			}
			if !trial.Accepted {
				report.Consumption.RejectedCandidates++
			}
			report.Trials = append(report.Trials, normalizeCrossStageTrial(trial))
			if trial.Accepted {
				acceptedTrials = append(acceptedTrials, len(report.Trials)-1)
			}
		}
		if err := ctx.Err(); err != nil {
			report.StopReason = CrossStageStopContextCanceled
			break
		}
		if len(acceptedTrials) == 0 {
			if report.Consumption.Trials >= policy.MaxTrials {
				report.StopReason = CrossStageStopBudgetExhausted
			} else {
				report.StopReason = CrossStageStopNoSafeImprovement
			}
			break
		}
		slices.SortFunc(acceptedTrials, func(left, right int) int {
			return compareCrossStageTrial(report.Trials[left], report.Trials[right])
		})
		selectedIndex := acceptedTrials[0]
		selected := &report.Trials[selectedIndex]
		selected.Selected = true
		report.Consumption.ConfirmationAttempts++
		if err := target.Apply(ctx, selected.Proposal); err != nil {
			selected.Rejections = crossStageStrings(append(selected.Rejections, "confirmation_apply_failed"))
			report.StopReason = CrossStageStopConfirmationApplyFailed
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), restoreErr
			}
			break
		}
		if err := target.Reenter(ctx, selected.Proposal.ReenterStage); err != nil {
			selected.Rejections = crossStageStrings(append(selected.Rejections, "confirmation_reentry_failed"))
			report.StopReason = CrossStageStopConfirmationMismatch
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), restoreErr
			}
			break
		}
		confirmed, err := captureCrossStageTarget(ctx, target)
		if err != nil {
			report.StopReason = CrossStageStopInvalidTargetEvidence
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), errors.Join(err, restoreErr)
			}
			return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), err
		}
		confirmedDiagnostics, err := diagnoseCrossStageTarget(ctx, target)
		if err != nil {
			report.StopReason = CrossStageStopInvalidTargetEvidence
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), errors.Join(err, restoreErr)
			}
			return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), err
		}
		if confirmed.Snapshot.Hash != selected.Candidate.Hash || !slices.Equal(crossStageDiagnosticHashes(confirmedDiagnostics), selected.CandidateDiagnostic) {
			selected.Rejections = crossStageStrings(append(selected.Rejections, "confirmation_mismatch"))
			report.StopReason = CrossStageStopConfirmationMismatch
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), restoreErr
			}
			break
		}
		if _, repeated := seenCommittedStates[confirmed.Snapshot.StateHash]; repeated {
			selected.Rejections = crossStageStrings(append(selected.Rejections, "repeated_state"))
			report.StopReason = CrossStageStopRepeatedState
			report.Consumption.CheckpointRestores++
			if _, restoreErr := restoreCrossStageTarget(ctx, target, base); restoreErr != nil {
				return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, base.Snapshot, diagnostics), restoreErr
			}
			break
		}
		selected.Confirmed = true
		report.Consumption.CommittedRepairs++
		seenCommittedStates[confirmed.Snapshot.StateHash] = struct{}{}
	}

	finalCheckpoint, err := captureCrossStageTarget(ctx, target)
	if err != nil {
		return CrossStageReport{}, err
	}
	finalDiagnostics, err := diagnoseCrossStageTarget(ctx, target)
	if err != nil {
		return CrossStageReport{}, err
	}
	return finalizeCrossStageReport(report, allDiagnostics, allProposals, trialsByDiagnostic, finalCheckpoint.Snapshot, finalDiagnostics), nil
}

func captureCrossStageTarget(ctx context.Context, target CrossStageTarget) (CrossStageCheckpoint, error) {
	checkpoint, err := target.Capture(ctx)
	if err != nil {
		return CrossStageCheckpoint{}, err
	}
	if err := ValidateCrossStageCheckpoint(checkpoint); err != nil {
		return CrossStageCheckpoint{}, fmt.Errorf("%s: %w", CrossStageStopInvalidTargetEvidence, err)
	}
	return checkpoint, nil
}

func diagnoseCrossStageTarget(ctx context.Context, target CrossStageTarget) ([]CrossStageDiagnostic, error) {
	diagnostics, err := target.Diagnose(ctx)
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range diagnostics {
		if err := ValidateCrossStageDiagnostic(diagnostic); err != nil {
			return nil, fmt.Errorf("%s: %w", CrossStageStopInvalidTargetEvidence, err)
		}
	}
	return normalizeCrossStageDiagnostics(diagnostics), nil
}

func crossStageTargetProposals(ctx context.Context, target CrossStageTarget, diagnostics []CrossStageDiagnostic) ([]CrossStageProposal, error) {
	diagnosticByHash := make(map[string]CrossStageDiagnostic, len(diagnostics))
	var proposals []CrossStageProposal
	for _, diagnostic := range diagnostics {
		diagnosticByHash[diagnostic.Hash] = diagnostic
		items, err := target.Propose(ctx, diagnostic)
		if err != nil {
			return nil, err
		}
		for _, proposal := range items {
			if err := ValidateCrossStageProposal(proposal, diagnostic); err != nil {
				return nil, fmt.Errorf("%s: %w", CrossStageStopInvalidTargetEvidence, err)
			}
		}
		proposals = append(proposals, items...)
	}
	proposals = normalizeCrossStageProposals(proposals, diagnosticByHash)
	for _, proposal := range proposals {
		diagnostic, ok := diagnosticByHash[proposal.DiagnosticHash]
		if !ok {
			return nil, errors.New("cross-stage proposal references an unknown diagnostic")
		}
		if err := ValidateCrossStageProposal(proposal, diagnostic); err != nil {
			return nil, fmt.Errorf("%s: %w", CrossStageStopInvalidTargetEvidence, err)
		}
	}
	return proposals, nil
}

func restoreCrossStageTarget(ctx context.Context, target CrossStageTarget, checkpoint CrossStageCheckpoint) (bool, error) {
	restoreCtx := context.WithoutCancel(ctx)
	if err := target.Restore(restoreCtx, checkpoint); err != nil {
		return false, fmt.Errorf("%s: %w", CrossStageStopRestoreFailed, err)
	}
	restored, err := captureCrossStageTarget(restoreCtx, target)
	if err != nil {
		return false, err
	}
	if restored.Snapshot.Hash != checkpoint.Snapshot.Hash {
		return false, fmt.Errorf("%s: restored snapshot %s differs from checkpoint %s", CrossStageStopRestoreFailed, restored.Snapshot.Hash, checkpoint.Snapshot.Hash)
	}
	return true, nil
}

func assessCrossStageCandidate(before CrossStageSnapshot, beforeDiagnostics []CrossStageDiagnostic, after CrossStageSnapshot, afterDiagnostics []CrossStageDiagnostic, addressed CrossStageDiagnostic, proposal CrossStageProposal, tolerance float64) ([]string, int, int) {
	rejections := []string{}
	if before.StateHash == after.StateHash {
		rejections = append(rejections, "state_unchanged")
	}
	affectedScopes := make(map[string]struct{}, len(proposal.Scope))
	for _, scope := range proposal.Scope {
		affectedScopes[scope] = struct{}{}
	}
	afterScopes := make(map[string]string, len(after.ScopeHashes))
	beforeScopes := make(map[string]string, len(before.ScopeHashes))
	for _, scope := range before.ScopeHashes {
		beforeScopes[scope.Scope] = scope.Hash
	}
	for _, scope := range after.ScopeHashes {
		afterScopes[scope.Scope] = scope.Hash
		if _, existed := beforeScopes[scope.Scope]; !existed {
			if _, affected := affectedScopes[scope.Scope]; !affected {
				rejections = append(rejections, "unrelated_scope_added:"+scope.Scope)
			}
		}
	}
	for _, scope := range before.ScopeHashes {
		if _, affected := affectedScopes[scope.Scope]; affected {
			continue
		}
		if afterScopes[scope.Scope] != scope.Hash {
			rejections = append(rejections, "unrelated_scope_changed:"+scope.Scope)
		}
	}
	afterGates := make(map[string]CrossStageGate, len(after.Gates))
	beforeGates := make(map[string]CrossStageGate, len(before.Gates))
	for _, gate := range before.Gates {
		beforeGates[gate.ID] = gate
	}
	for _, gate := range after.Gates {
		afterGates[gate.ID] = gate
		if previous, existed := beforeGates[gate.ID]; !existed && gate.Required && gate.Status == CrossStageGateBlocked {
			rejections = append(rejections, "new_blocking_gate:"+gate.ID)
		} else if existed && previous.Required && !gate.Required {
			rejections = append(rejections, "required_gate_removed:"+gate.ID)
		} else if existed && previous.Required && crossStageGateRank(previous.Status) >= crossStageGateRank(CrossStageGateWarning) && crossStageGateRank(gate.Status) < crossStageGateRank(previous.Status) {
			rejections = append(rejections, "gate_regressed:"+gate.ID)
		}
	}
	passedBefore := 0
	for _, gate := range before.Gates {
		if gate.Required && gate.Status == CrossStageGatePassed {
			passedBefore++
		}
		_, exists := afterGates[gate.ID]
		if gate.Required && crossStageGateRank(gate.Status) >= crossStageGateRank(CrossStageGateWarning) && !exists {
			rejections = append(rejections, "required_gate_removed:"+gate.ID)
		}
	}
	passedAfter := 0
	for _, gate := range after.Gates {
		if gate.Required && gate.Status == CrossStageGatePassed {
			passedAfter++
		}
	}
	afterMargins := make(map[string]CrossStageMargin, len(after.Margins))
	for _, margin := range after.Margins {
		afterMargins[margin.ID] = margin
	}
	for _, margin := range before.Margins {
		if !margin.Protected {
			continue
		}
		candidate, exists := afterMargins[margin.ID]
		if !exists {
			rejections = append(rejections, "protected_margin_removed:"+margin.ID)
			continue
		}
		if !candidate.Protected {
			rejections = append(rejections, "protected_margin_unprotected:"+margin.ID)
		}
		if candidate.Headroom+tolerance < margin.Headroom {
			rejections = append(rejections, "protected_margin_regressed:"+margin.ID)
		}
	}
	beforeBlocking := map[string]struct{}{}
	for _, diagnostic := range beforeDiagnostics {
		if diagnostic.Severity == CrossStageSeverityBlocking {
			beforeBlocking[diagnostic.Key] = struct{}{}
		}
	}
	afterBlocking := map[string]struct{}{}
	for _, diagnostic := range afterDiagnostics {
		if diagnostic.Severity != CrossStageSeverityBlocking {
			continue
		}
		afterBlocking[diagnostic.Key] = struct{}{}
		if _, existed := beforeBlocking[diagnostic.Key]; !existed {
			rejections = append(rejections, "new_blocking_diagnostic:"+diagnostic.Key)
		}
	}
	resolved := 0
	for key := range beforeBlocking {
		if _, remains := afterBlocking[key]; !remains {
			resolved++
		}
	}
	if _, remains := afterBlocking[addressed.Key]; remains {
		rejections = append(rejections, "addressed_diagnostic_remains")
	}
	if resolved == 0 {
		rejections = append(rejections, "no_blocking_evidence_resolved")
	}
	return crossStageStrings(rejections), resolved, passedAfter - passedBefore
}

func blockingCrossStageDiagnostics(diagnostics []CrossStageDiagnostic) []CrossStageDiagnostic {
	result := make([]CrossStageDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == CrossStageSeverityBlocking {
			result = append(result, diagnostic)
		}
	}
	slices.SortFunc(result, compareCrossStageDiagnostic)
	return result
}

func crossStageRequiredGatesPassed(snapshot CrossStageSnapshot) bool {
	required := 0
	for _, gate := range snapshot.Gates {
		if !gate.Required {
			continue
		}
		required++
		if gate.Status != CrossStageGatePassed {
			return false
		}
	}
	return required > 0
}

func crossStageDiagnosticByHash(diagnostics []CrossStageDiagnostic, hash string) CrossStageDiagnostic {
	for _, diagnostic := range diagnostics {
		if diagnostic.Hash == hash {
			return diagnostic
		}
	}
	return CrossStageDiagnostic{}
}

func crossStageDiagnosticHashes(diagnostics []CrossStageDiagnostic) []string {
	result := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, diagnostic.Hash)
	}
	slices.Sort(result)
	return result
}

func appendUniqueCrossStageDiagnostics(destination []CrossStageDiagnostic, seen map[string]struct{}, source []CrossStageDiagnostic) []CrossStageDiagnostic {
	for _, diagnostic := range source {
		if _, exists := seen[diagnostic.Hash]; exists {
			continue
		}
		seen[diagnostic.Hash] = struct{}{}
		destination = append(destination, diagnostic)
	}
	return destination
}

func normalizeCrossStageTrial(trial CrossStageTrial) CrossStageTrial {
	trial.CandidateDiagnostic = crossStageStrings(trial.CandidateDiagnostic)
	trial.Rejections = crossStageStrings(trial.Rejections)
	return trial
}

func compareCrossStageTrial(left, right CrossStageTrial) int {
	return cmp.Or(
		cmp.Compare(right.ResolvedBlocking, left.ResolvedBlocking),
		cmp.Compare(right.PassedGateDelta, left.PassedGateDelta),
		compareCrossStageProposal(left.Proposal, right.Proposal),
		cmp.Compare(left.Candidate.Hash, right.Candidate.Hash),
	)
}

func finalizeCrossStageReport(report CrossStageReport, allDiagnostics []CrossStageDiagnostic, allProposals []CrossStageProposal, trialsByDiagnostic map[string]int, final CrossStageSnapshot, finalDiagnostics []CrossStageDiagnostic) CrossStageReport {
	report.Final = final
	report.FinalDiagnostics = normalizeCrossStageDiagnostics(finalDiagnostics)
	report.InitialDiagnostics = normalizeCrossStageDiagnostics(report.InitialDiagnostics)
	for index := range report.Trials {
		report.Trials[index] = normalizeCrossStageTrial(report.Trials[index])
	}
	report.Consumption.TrialsByDiagnostic = report.Consumption.TrialsByDiagnostic[:0]
	for key, count := range trialsByDiagnostic {
		report.Consumption.TrialsByDiagnostic = append(report.Consumption.TrialsByDiagnostic, CrossStageUse{DiagnosticKey: key, Count: count})
	}
	slices.SortFunc(report.Consumption.TrialsByDiagnostic, func(left, right CrossStageUse) int { return cmp.Compare(left.DiagnosticKey, right.DiagnosticKey) })
	allDiagnostics = normalizeCrossStageDiagnostics(append(allDiagnostics, finalDiagnostics...))
	report.Diagnostics = slices.Clone(allDiagnostics)
	diagnosticByHash := make(map[string]CrossStageDiagnostic, len(allDiagnostics))
	for _, diagnostic := range allDiagnostics {
		diagnosticByHash[diagnostic.Hash] = diagnostic
	}
	allProposals = normalizeCrossStageProposals(allProposals, diagnosticByHash)
	report.Proposals = slices.Clone(allProposals)
	traceDiagnostics := make([]Diagnostic, 0, len(allDiagnostics))
	for _, diagnostic := range allDiagnostics {
		traceDiagnostics = append(traceDiagnostics, NewDiagnostic(string(diagnostic.Stage), diagnostic.Code, diagnostic.Category, string(diagnostic.Severity), diagnostic.EvidenceHash, diagnostic.Scope))
	}
	traceProposals := make([]Proposal, 0, len(allProposals))
	for _, proposal := range allProposals {
		diagnostic := diagnosticByHash[proposal.DiagnosticHash]
		traceProposals = append(traceProposals, NewProposal(
			NewDiagnostic(string(diagnostic.Stage), diagnostic.Code, diagnostic.Category, string(diagnostic.Severity), diagnostic.EvidenceHash, diagnostic.Scope),
			proposal.Operator, string(proposal.ReenterStage), strings.Join(proposal.ExpectedEffects, ";"), proposal.Scope, proposal.Authorized, proposal.Rejection,
		))
	}
	traceOutcomes := make([]Outcome, 0, len(report.Trials))
	for _, trial := range report.Trials {
		status := "rejected"
		if trial.Confirmed {
			status = "committed"
		} else if trial.Accepted {
			status = "candidate"
		}
		diagnostic := diagnosticByHash[trial.Proposal.DiagnosticHash]
		traceProposal := NewProposal(
			NewDiagnostic(string(diagnostic.Stage), diagnostic.Code, diagnostic.Category, string(diagnostic.Severity), diagnostic.EvidenceHash, diagnostic.Scope),
			trial.Proposal.Operator, string(trial.Proposal.ReenterStage), strings.Join(trial.Proposal.ExpectedEffects, ";"), trial.Proposal.Scope, trial.Proposal.Authorized, trial.Proposal.Rejection,
		)
		traceOutcomes = append(traceOutcomes, Outcome{
			ProposalID: traceProposal.ID, Status: status, BeforeHash: trial.Before.StateHash,
			AfterHash: trial.Candidate.StateHash, ResultHash: trial.Candidate.Hash, Reason: strings.Join(trial.Rejections, ";"),
		})
	}
	report.Trace = NewTrace(report.Policy.MaxTrials, report.Consumption.Trials, traceDiagnostics, traceProposals, traceOutcomes)
	copy := report
	copy.Hash = ""
	report.Hash = crossStageHash(copy)
	return report
}
