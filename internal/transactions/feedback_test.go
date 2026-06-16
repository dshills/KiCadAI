package transactions

import (
	"testing"

	"kicadai/internal/reports"
)

func TestFeedbackFromPlanGroupsIssuesByOperationID(t *testing.T) {
	plan := Plan{
		Target: "out",
		Operations: []PlannedOperation{
			{ID: "op-add-symbol-ref-r1-aaaa", Index: 0, Op: OpAddSymbol, Refs: []string{"R1"}},
			{ID: "op-route-net-gnd-bbbb", Index: 1, Op: OpRoute, Nets: []string{"GND"}},
		},
		Issues: []reports.Issue{
			{Code: reports.CodeInvalidArgument, Severity: reports.SeverityWarning, OperationID: "op-route-net-gnd-bbbb", Nets: []string{"GND"}, Message: "weak route", Suggestion: "increase width"},
			{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, OperationID: "op-route-net-gnd-bbbb", Nets: []string{"GND"}, Message: "bad route", Suggestion: "increase width"},
			{Code: reports.CodeInvalidArgument, Severity: reports.SeverityWarning, Message: "unlinked"},
		},
	}
	feedback := FeedbackFromPlan(plan)
	if feedback.Target != "out" || feedback.Summary.OperationCount != 2 || feedback.Summary.IssueCount != 3 || feedback.Summary.ErrorCount != 1 || feedback.Summary.UnlinkedCount != 1 {
		t.Fatalf("unexpected summary: %#v", feedback)
	}
	if feedback.Operations[0].Severity != "" || len(feedback.Operations[0].Issues) != 0 {
		t.Fatalf("empty operation should have no severity: %#v", feedback.Operations[0])
	}
	if feedback.Operations[1].Severity != reports.SeverityError || len(feedback.Operations[1].Issues) != 2 {
		t.Fatalf("route feedback not grouped: %#v", feedback.Operations[1])
	}
	if len(feedback.Operations[1].Suggestions) != 1 || feedback.Operations[1].Suggestions[0] != "increase width" {
		t.Fatalf("suggestions not deduplicated: %#v", feedback.Operations[1].Suggestions)
	}
}

func TestFeedbackFromPlanIncludesArtifacts(t *testing.T) {
	artifact := reports.Artifact{Kind: reports.ArtifactPCB, Path: "out/demo.kicad_pcb"}
	plan := Plan{
		Operations: []PlannedOperation{{ID: "op-write-project-aaaa", Index: 0, Op: OpWriteProject, Artifacts: []reports.Artifact{artifact}}},
	}
	feedback := FeedbackFromPlan(plan)
	if len(feedback.Artifacts) != 1 || feedback.Artifacts[0].Path != artifact.Path {
		t.Fatalf("report artifacts missing: %#v", feedback.Artifacts)
	}
	if len(feedback.Operations) != 1 || len(feedback.Operations[0].Artifacts) != 1 {
		t.Fatalf("operation artifacts missing: %#v", feedback.Operations)
	}
}

func TestFeedbackCountsUnknownOperationIDAsUnlinked(t *testing.T) {
	plan := Plan{
		Operations: []PlannedOperation{{ID: "op-known", Index: 0, Op: OpWriteProject}},
		Issues: []reports.Issue{{
			Code:        reports.CodeInvalidArgument,
			Severity:    reports.SeverityError,
			OperationID: "op-missing",
			Message:     "stale issue",
		}},
	}
	feedback := FeedbackFromPlan(plan)
	if feedback.Summary.UnlinkedCount != 1 || len(feedback.Operations[0].Issues) != 0 {
		t.Fatalf("unknown operation id should be unlinked: %#v", feedback)
	}
}

func TestFeedbackFromValidationUsesValidatedOperations(t *testing.T) {
	validation := ValidationResult{
		OperationCount: 1,
		Operations: []ValidatedOperation{{
			ID:    "op-add-symbol-ref-r1-aaaa",
			Index: 0,
			Op:    OpAddSymbol,
			Refs:  []string{"R1"},
		}},
		Issues: []reports.Issue{{
			Code:        reports.CodeInvalidArgument,
			Severity:    reports.SeverityError,
			OperationID: "op-add-symbol-ref-r1-aaaa",
			Refs:        []string{"R1"},
			Message:     "missing library",
		}},
	}
	feedback := FeedbackFromValidation(validation)
	if feedback.Summary.OperationCount != 1 || feedback.Summary.BlockingCount != 1 {
		t.Fatalf("unexpected validation feedback summary: %#v", feedback.Summary)
	}
	if len(feedback.Operations) != 1 || feedback.Operations[0].Refs[0] != "R1" || feedback.Operations[0].Severity != reports.SeverityError {
		t.Fatalf("unexpected validation operation feedback: %#v", feedback.Operations)
	}
}
