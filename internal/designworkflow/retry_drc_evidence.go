package designworkflow

import (
	"context"
	"strings"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

type RetryDRCPolicy string

const (
	RetryDRCPolicyDisabled RetryDRCPolicy = "disabled"
	RetryDRCPolicyOptional RetryDRCPolicy = "optional"
	RetryDRCPolicyRequired RetryDRCPolicy = "required"
)

type AttemptDRCEvidence struct {
	Status        retryEvidenceStatus `json:"status"`
	IssueCount    int                 `json:"issue_count"`
	BlockingCount int                 `json:"blocking_count"`
	Issues        []reports.Issue     `json:"issues,omitempty"`
	Artifacts     []reports.Artifact  `json:"artifacts,omitempty"`
	Source        string              `json:"source"`
	MissingReason string              `json:"missing_reason,omitempty"`
}

type retryDRCEvidenceRequest struct {
	Attempt    int
	ProjectDir string
	PCBPath    string
	Options    KiCadCheckOptions
}

type retryDRCEvidenceAdapter interface {
	Evidence(context.Context, retryDRCEvidenceRequest) (AttemptDRCEvidence, error)
}

type fakeRetryDRCEvidenceAdapter struct {
	ByAttempt map[int]AttemptDRCEvidence
	Default   AttemptDRCEvidence
	Err       error
}

func (adapter fakeRetryDRCEvidenceAdapter) Evidence(_ context.Context, request retryDRCEvidenceRequest) (AttemptDRCEvidence, error) {
	if adapter.Err != nil {
		return AttemptDRCEvidence{}, adapter.Err
	}
	if adapter.ByAttempt != nil {
		if evidence, ok := adapter.ByAttempt[request.Attempt]; ok {
			return normalizeAttemptDRCEvidence(evidence), nil
		}
	}
	return normalizeAttemptDRCEvidence(adapter.Default), nil
}

type kicadRetryDRCEvidenceAdapter struct{}

func (adapter kicadRetryDRCEvidenceAdapter) Evidence(ctx context.Context, request retryDRCEvidenceRequest) (AttemptDRCEvidence, error) {
	if strings.TrimSpace(request.PCBPath) == "" {
		return normalizeAttemptDRCEvidence(AttemptDRCEvidence{Status: retryEvidenceMissing, Source: "kicad-cli", MissingReason: "PCB path is required for retry DRC evidence"}), nil
	}
	opts := request.Options
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	cli, err := checks.DiscoverCLI(opts.KiCadCLI)
	if err != nil {
		return normalizeAttemptDRCEvidence(AttemptDRCEvidence{Status: retryEvidenceMissing, Source: "missing", MissingReason: err.Error()}), nil
	}
	checkOpts := checks.Options{
		KiCadCLI:      cli.Path,
		Timeout:       opts.Timeout,
		KeepArtifacts: opts.KeepArtifacts,
		ArtifactDir:   opts.ArtifactDir,
		Allowlist:     opts.Allowlist,
	}
	result, err := checks.RunDRC(ctx, cli, request.PCBPath, checkOpts)
	_, issues, artifacts := workflowCheckResultWithIssues(result, err)
	evidence := AttemptDRCEvidence{
		Status:    retryEvidenceStatusForIssues(issues),
		Issues:    issues,
		Artifacts: artifacts,
		Source:    "kicad-cli",
	}
	return normalizeAttemptDRCEvidence(evidence), nil
}

func retryDRCEvidenceForAttempt(ctx context.Context, policy RetryDRCPolicy, adapter retryDRCEvidenceAdapter, request retryDRCEvidenceRequest) AttemptDRCEvidence {
	policy = normalizeRetryDRCPolicy(policy)
	if policy == RetryDRCPolicyDisabled {
		return AttemptDRCEvidence{Status: retryEvidenceSkipped, Source: "skipped"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if adapter == nil {
		return missingAttemptDRCEvidence(policy, "missing", "retry DRC evidence adapter is not configured")
	}
	evidence, err := adapter.Evidence(ctx, request)
	if err != nil {
		return missingAttemptDRCEvidence(policy, "missing", err.Error())
	}
	if evidence.Status == retryEvidenceMissing && policy == RetryDRCPolicyRequired && evidence.BlockingCount == 0 {
		evidence.Issues = append(evidence.Issues, missingDRCIssue("retry_drc", firstNonEmpty(evidence.MissingReason, "required retry DRC evidence is missing")))
		evidence.IssueCount, evidence.BlockingCount = summarizeRetryIssues(evidence.Issues)
		if evidence.BlockingCount > 0 {
			evidence.Status = retryEvidenceFail
		}
	}
	return evidence
}

func normalizeAttemptDRCEvidence(evidence AttemptDRCEvidence) AttemptDRCEvidence {
	if evidence.Status == "" || (len(evidence.Issues) > 0 && evidence.Status != retryEvidenceMissing && evidence.Status != retryEvidenceSkipped) {
		evidence.Status = retryEvidenceStatusForIssues(evidence.Issues)
	}
	if evidence.Source == "" {
		evidence.Source = "unknown"
	}
	evidence.Issues = cloneIssues(evidence.Issues)
	evidence.Artifacts = append([]reports.Artifact(nil), evidence.Artifacts...)
	evidence.IssueCount, evidence.BlockingCount = summarizeRetryIssues(evidence.Issues)
	if evidence.BlockingCount > 0 {
		evidence.Status = retryEvidenceFail
	}
	return evidence
}

func missingAttemptDRCEvidence(policy RetryDRCPolicy, source string, message string) AttemptDRCEvidence {
	evidence := AttemptDRCEvidence{Status: retryEvidenceMissing, Source: source, MissingReason: message}
	if policy == RetryDRCPolicyRequired {
		evidence.Issues = append(evidence.Issues, missingDRCIssue("retry_drc", message))
	}
	return normalizeAttemptDRCEvidence(evidence)
}

func missingDRCIssue(path string, message string) reports.Issue {
	if strings.TrimSpace(message) == "" {
		message = "retry DRC evidence is missing"
	}
	return reports.Issue{
		Code:       reports.CodeSkippedExternalTool,
		Severity:   reports.SeverityBlocked,
		Path:       path,
		Message:    message,
		Suggestion: "configure KiCad CLI or disable required retry DRC evidence",
	}
}

func retryEvidenceStatusForIssues(issues []reports.Issue) retryEvidenceStatus {
	if len(issues) == 0 {
		return retryEvidencePass
	}
	hasWarning := false
	for _, issue := range issues {
		if issue.Blocking() || issue.Severity == reports.SeverityError {
			return retryEvidenceFail
		}
		if issue.Severity == reports.SeverityWarning {
			hasWarning = true
		}
	}
	if hasWarning {
		return retryEvidenceWarning
	}
	return retryEvidencePass
}

func applyRetryDRCEvidenceToAttempt(summary *placementRoutingRetryAttemptSummary, evidence AttemptDRCEvidence) {
	if summary == nil {
		return
	}
	evidence = normalizeAttemptDRCEvidence(evidence)
	summary.DRCStatus = evidence.Status
	summary.DRCIssueCount = evidence.IssueCount
	summary.DRCBlockingCount = evidence.BlockingCount
	summary.DRCSource = evidence.Source
}

func normalizeRetryDRCPolicy(policy RetryDRCPolicy) RetryDRCPolicy {
	switch policy {
	case RetryDRCPolicyOptional, RetryDRCPolicyRequired:
		return policy
	default:
		return RetryDRCPolicyDisabled
	}
}
