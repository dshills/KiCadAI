package designworkflow

import (
	"context"
	"errors"
	"testing"

	"kicadai/internal/reports"
)

func TestRetryDRCEvidenceDisabledSkips(t *testing.T) {
	evidence := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyDisabled, fakeRetryDRCEvidenceAdapter{
		Default: AttemptDRCEvidence{Status: retryEvidenceFail, Source: "fixture"},
	}, retryDRCEvidenceRequest{Attempt: 1})

	if evidence.Status != retryEvidenceSkipped || evidence.Source != "skipped" || evidence.IssueCount != 0 {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestRetryDRCEvidenceOptionalMissingIsVisibleNonBlocking(t *testing.T) {
	evidence := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyOptional, nil, retryDRCEvidenceRequest{Attempt: 1})

	if evidence.Status != retryEvidenceMissing || evidence.Source != "missing" || evidence.BlockingCount != 0 || len(evidence.Issues) != 0 {
		t.Fatalf("optional missing evidence = %#v", evidence)
	}
}

func TestRetryDRCEvidenceRequiredMissingBlocks(t *testing.T) {
	evidence := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyRequired, nil, retryDRCEvidenceRequest{Attempt: 1})

	if evidence.Status != retryEvidenceFail || evidence.Source != "missing" || evidence.BlockingCount != 1 {
		t.Fatalf("required missing evidence = %#v", evidence)
	}
	if len(evidence.Issues) != 1 || evidence.Issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("required missing issues = %#v", evidence.Issues)
	}
}

func TestRetryDRCEvidenceFakeAdapterByAttempt(t *testing.T) {
	adapter := fakeRetryDRCEvidenceAdapter{ByAttempt: map[int]AttemptDRCEvidence{
		2: {
			Source: "fixture",
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Message:  "clearance violation",
			}},
		},
	}}

	evidence := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyOptional, adapter, retryDRCEvidenceRequest{Attempt: 2})
	if evidence.Status != retryEvidenceFail || evidence.IssueCount != 1 || evidence.BlockingCount != 1 || evidence.Source != "fixture" {
		t.Fatalf("fake evidence = %#v", evidence)
	}
}

func TestRetryDRCEvidenceAdapterErrorUsesPolicy(t *testing.T) {
	adapter := fakeRetryDRCEvidenceAdapter{Err: errors.New("adapter failed")}

	optional := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyOptional, adapter, retryDRCEvidenceRequest{Attempt: 1})
	if optional.Status != retryEvidenceMissing || optional.BlockingCount != 0 {
		t.Fatalf("optional adapter error = %#v", optional)
	}
	required := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyRequired, adapter, retryDRCEvidenceRequest{Attempt: 1})
	if required.Status != retryEvidenceFail || required.BlockingCount != 1 {
		t.Fatalf("required adapter error = %#v", required)
	}
}

func TestKiCadRetryDRCEvidenceAdapterMissingPCBDoesNotInjectPolicy(t *testing.T) {
	adapter := kicadRetryDRCEvidenceAdapter{}

	direct, err := adapter.Evidence(context.Background(), retryDRCEvidenceRequest{Attempt: 1})
	if err != nil {
		t.Fatalf("adapter returned error: %v", err)
	}
	if direct.Status != retryEvidenceMissing || direct.BlockingCount != 0 || len(direct.Issues) != 0 || direct.MissingReason == "" {
		t.Fatalf("direct adapter evidence = %#v", direct)
	}

	required := retryDRCEvidenceForAttempt(context.Background(), RetryDRCPolicyRequired, adapter, retryDRCEvidenceRequest{Attempt: 1})
	if required.Status != retryEvidenceFail || required.BlockingCount != 1 {
		t.Fatalf("required wrapped evidence = %#v", required)
	}
	if required.Issues[0].Message != direct.MissingReason {
		t.Fatalf("required message = %q, want %q", required.Issues[0].Message, direct.MissingReason)
	}
}

func TestApplyRetryDRCEvidenceToAttempt(t *testing.T) {
	attempt := placementRoutingRetryAttemptSummary{Attempt: 2}
	applyRetryDRCEvidenceToAttempt(&attempt, AttemptDRCEvidence{
		Source: "fixture",
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Message:  "DRC failed",
		}},
	})

	if attempt.DRCStatus != retryEvidenceFail || attempt.DRCSource != "fixture" || attempt.DRCIssueCount != 1 || attempt.DRCBlockingCount != 1 {
		t.Fatalf("attempt = %#v", attempt)
	}
}
