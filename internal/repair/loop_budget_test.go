package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestNormalizeLoopBudgetOptionsUsesSafeDefaults(t *testing.T) {
	opts := NormalizeLoopBudgetOptions(LoopBudgetOptions{})
	if intOptionValue(opts.MaxCycles) != 2 || intOptionValue(opts.MaxRepairs) != 2 {
		t.Fatalf("unexpected max defaults: %+v", opts)
	}
	if !boolOptionValue(opts.StopOnNoImprovement) || !boolOptionValue(opts.StopOnRepeatedEvidence) {
		t.Fatalf("stop defaults should be enabled: %+v", opts)
	}
	if opts.MaxPerCategory[string(FindingCategoryRoute)] != 2 || opts.MaxPerCategory[string(FindingCategoryConnectivity)] != 2 {
		t.Fatalf("missing route/connectivity defaults: %+v", opts.MaxPerCategory)
	}
}

func TestLoopBudgetLedgerExhaustsTotalCycles(t *testing.T) {
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(1), MaxRepairs: intPointer(3), StopOnNoImprovement: boolPointer(true)})
	if reason := ledger.RecordCycle(NormalizedDeltaSummary{Improved: true, After: NormalizedEvidenceSummary{BlockingCount: 1}}); reason != "" {
		t.Fatalf("first cycle reason = %q", reason)
	}
	if reason := ledger.RecordCycle(NormalizedDeltaSummary{Improved: true, After: NormalizedEvidenceSummary{BlockingCount: 1}}); reason != StopReasonTotalBudgetExhausted {
		t.Fatalf("second cycle reason = %q", reason)
	}
	summary := ledger.Summary()
	if !summary.Exhausted || summary.RemainingCycles != 0 || summary.StopReason != StopReasonTotalBudgetExhausted {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestLoopBudgetLedgerExhaustsCategoryBudget(t *testing.T) {
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{
		MaxCycles:      intPointer(5),
		MaxRepairs:     intPointer(5),
		MaxPerCategory: map[string]int{string(FindingCategoryZone): 1},
	})
	if reason := ledger.RecordRepair(FindingCategoryZone); reason != "" {
		t.Fatalf("first zone repair reason = %q", reason)
	}
	if reason := ledger.RecordRepair(FindingCategoryZone); reason != StopReasonCategoryBudgetExhausted {
		t.Fatalf("second zone repair reason = %q", reason)
	}
	summary := ledger.Summary()
	if summary.ExhaustedCategory != string(FindingCategoryZone) || summary.CategoryAttempts[string(FindingCategoryZone)] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestLoopBudgetLedgerStopsOnRepeatedEvidence(t *testing.T) {
	repeated := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "same", Message: "same"}, NormalizeFindingOptions{Source: FindingSourceBoard})
	delta := CompareNormalizedFindings([]NormalizedFinding{repeated}, []NormalizedFinding{repeated})
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(3), MaxRepairs: intPointer(3)})
	if reason := ledger.RecordCycle(delta); reason != StopReasonRepeatedEvidence {
		t.Fatalf("reason = %q; delta=%+v", reason, delta)
	}
	if summary := ledger.Summary(); summary.RepeatedEvidenceKey == "" || !summary.Exhausted {
		t.Fatalf("missing repeated evidence key: %+v", ledger.Summary())
	}
}

func TestLoopBudgetLedgerStopsOnNoImprovement(t *testing.T) {
	finding := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "new", Message: "new"}, NormalizeFindingOptions{Source: FindingSourceBoard})
	delta := CompareNormalizedFindings(nil, []NormalizedFinding{finding})
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(3), MaxRepairs: intPointer(3)})
	if reason := ledger.RecordCycle(delta); reason != StopReasonNoImprovement {
		t.Fatalf("reason = %q; delta=%+v", reason, delta)
	}
	if !ledger.Summary().Exhausted {
		t.Fatalf("no-improvement stop should exhaust convergence ledger: %+v", ledger.Summary())
	}
}

func TestLoopBudgetLedgerCountsRepairsAndRemainingBudget(t *testing.T) {
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(4), MaxRepairs: intPointer(2)})
	if reason := ledger.RecordRepair(FindingCategoryRoute); reason != "" {
		t.Fatalf("route repair reason = %q", reason)
	}
	if reason := ledger.RecordRepair(FindingCategoryConnectivity); reason != "" {
		t.Fatalf("connectivity repair reason = %q", reason)
	}
	if reason := ledger.RecordRepair(FindingCategoryRoute); reason != StopReasonTotalBudgetExhausted {
		t.Fatalf("third repair reason = %q", reason)
	}
	summary := ledger.Summary()
	if summary.RepairsUsed != 2 || summary.RemainingRepairs != 0 || !summary.Exhausted {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestLoopBudgetLedgerCanDisableNoImprovementStop(t *testing.T) {
	finding := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "new", Message: "new"}, NormalizeFindingOptions{Source: FindingSourceBoard})
	delta := CompareNormalizedFindings(nil, []NormalizedFinding{finding})
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(3), MaxRepairs: intPointer(3), StopOnNoImprovement: boolPointer(false)})
	if reason := ledger.RecordCycle(delta); reason != "" {
		t.Fatalf("reason = %q; disabled no-improvement stop should continue until another limit", reason)
	}
}

func TestLoopBudgetLedgerAllowsExplicitZeroBudget(t *testing.T) {
	ledger := NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(0), MaxRepairs: intPointer(0)})
	if reason := ledger.RecordCycle(NormalizedDeltaSummary{Improved: true}); reason != StopReasonTotalBudgetExhausted {
		t.Fatalf("zero cycle budget reason = %q", reason)
	}
	if ledger.Summary().CyclesUsed != 0 {
		t.Fatalf("zero cycle budget should not record a cycle: %+v", ledger.Summary())
	}
	ledger = NewLoopBudgetLedger(LoopBudgetOptions{MaxCycles: intPointer(1), MaxRepairs: intPointer(2), MaxPerCategory: map[string]int{string(FindingCategoryZone): 0}})
	if reason := ledger.RecordRepair(FindingCategoryZone); reason != StopReasonCategoryBudgetExhausted {
		t.Fatalf("zero category budget reason = %q", reason)
	}
}

func TestNilLoopBudgetLedgerFailsClosed(t *testing.T) {
	var ledger *LoopBudgetLedger
	if reason := ledger.RecordCycle(NormalizedDeltaSummary{}); reason != StopReasonTotalBudgetExhausted {
		t.Fatalf("nil cycle reason = %q", reason)
	}
	if reason := ledger.RecordRepair(FindingCategoryRoute); reason != StopReasonTotalBudgetExhausted {
		t.Fatalf("nil repair reason = %q", reason)
	}
	if summary := ledger.Summary(); !summary.Exhausted || summary.StopReason != StopReasonTotalBudgetExhausted {
		t.Fatalf("nil summary = %+v", summary)
	}
}
