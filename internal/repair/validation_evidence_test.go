package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestStableIssueKeySortsIdentitySlices(t *testing.T) {
	left := reports.Issue{
		Code:        reports.CodeDisconnectedPad,
		Severity:    reports.SeverityError,
		Path:        `pcb\board.kicad_pcb`,
		Message:     "pad is disconnected",
		Refs:        []string{"U2", "U1"},
		Nets:        []string{"GND", "VCC"},
		UUIDs:       []string{"b", "a"},
		OperationID: "route-1",
	}
	right := left
	right.Path = "pcb/board.kicad_pcb"
	right.Refs = []string{"U1", "U2"}
	right.Nets = []string{"VCC", "GND"}
	right.UUIDs = []string{"a", "b"}
	if StableIssueKey(left) != StableIssueKey(right) {
		t.Fatalf("stable keys differ:\nleft=%q\nright=%q", StableIssueKey(left), StableIssueKey(right))
	}
}

func TestSummarizePostValidationCountsAdaptersIssuesAndArtifacts(t *testing.T) {
	summary := SummarizePostValidation([]PostApplyValidation{
		{
			Name: "writer",
			Issues: []reports.Issue{
				{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "warning"},
				{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing"},
			},
			Artifacts: []reports.Artifact{{Kind: reports.ArtifactValidationReport, Path: "writer-report.json"}},
		},
		{Name: "round_trip", Skipped: true},
	})
	if summary.AdapterCount != 2 || summary.SkippedCount != 1 || summary.IssueCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.BlockingCount != 1 || summary.WarningCount != 1 || summary.ArtifactCount != 1 {
		t.Fatalf("unexpected issue/artifact summary: %+v", summary)
	}
	if summary.ByAdapter["writer"] != 2 || summary.ByCode[string(reports.CodeMissingBoardOutline)] != 1 {
		t.Fatalf("missing buckets: %+v", summary)
	}
}

func TestSummarizePostValidationDeduplicatesProjectIssueCounts(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Path: "pcb", Message: "same"}
	summary := SummarizePostValidation([]PostApplyValidation{
		{Name: "board", Issues: []reports.Issue{issue}},
		{Name: "drc", Issues: []reports.Issue{issue}},
	})
	if summary.IssueCount != 1 || summary.BlockingCount != 1 {
		t.Fatalf("summary should count unique project issues: %+v", summary)
	}
	if summary.ByAdapter["board"] != 1 || summary.ByAdapter["drc"] != 1 {
		t.Fatalf("summary should retain per-adapter report counts: %+v", summary)
	}
}

func TestCompareValidationIssuesReportsClearedRepeatedNewAndWorsened(t *testing.T) {
	cleared := reports.Issue{Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Path: "a", Message: "cleared"}
	repeated := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityWarning, Path: "b", Message: "same"}
	newBlocking := reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: "c", Message: "new"}
	delta := CompareValidationIssues([]reports.Issue{cleared, repeated}, []reports.Issue{repeated, newBlocking})
	if len(delta.Cleared) != 1 || delta.Cleared[0].Message != "cleared" {
		t.Fatalf("cleared = %+v", delta.Cleared)
	}
	if len(delta.Repeated) != 1 || delta.Repeated[0].Message != "same" {
		t.Fatalf("repeated = %+v", delta.Repeated)
	}
	if len(delta.New) != 1 || delta.New[0].Message != "new" {
		t.Fatalf("new = %+v", delta.New)
	}
	if !delta.Worsened {
		t.Fatalf("delta should be worsened: %+v", delta)
	}
}

func TestCompareValidationIssuesDoesNotWorsenForWarningOnlyRegression(t *testing.T) {
	before := []reports.Issue{
		{Code: reports.CodeMissingFootprint, Severity: reports.SeverityError, Path: "a", Message: "blocking"},
	}
	after := []reports.Issue{
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "b", Message: "warning"},
	}
	delta := CompareValidationIssues(before, after)
	if delta.Worsened {
		t.Fatalf("warning-only after state should not be worsened: %+v", delta)
	}
}

func TestCompareValidationIssuesSummariesUseUniqueIssueIdentity(t *testing.T) {
	issue := reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Path: "pcb", Message: "same"}
	delta := CompareValidationIssues([]reports.Issue{issue, issue}, []reports.Issue{issue})
	if delta.Before.IssueCount != 1 || delta.After.IssueCount != 1 || len(delta.Repeated) != 1 {
		t.Fatalf("delta should summarize unique issues: %+v", delta)
	}
}

func TestNormalizeIssueDefaultsStructuredFields(t *testing.T) {
	issue := reports.Issue{
		Code:        reports.CodeDisconnectedPad,
		Severity:    reports.SeverityError,
		Path:        `out\demo.kicad_pcb/footprints/J1/pads/1`,
		Message:     "pad is disconnected",
		Refs:        []string{"J1"},
		Nets:        []string{"GND"},
		OperationID: "route-1",
	}
	finding := NormalizeIssue(issue, NormalizeFindingOptions{
		Source:  FindingSourceBoard,
		Adapter: "board",
		Subject: FindingSubject{Pad: "1", Layer: "F.Cu"},
	})
	if finding.Source != FindingSourceBoard || finding.Category != FindingCategoryConnectivity || finding.Repairability != RepairabilityRepairable {
		t.Fatalf("unexpected normalized classification: %+v", finding)
	}
	if finding.Subject.Ref != "J1" || finding.Subject.Net != "GND" || finding.Subject.Pad != "1" || finding.Subject.File != "out/demo.kicad_pcb" {
		t.Fatalf("unexpected subject: %+v", finding.Subject)
	}
	if finding.Path != "out/demo.kicad_pcb/footprints/J1/pads/1" || finding.OperationID != "route-1" || finding.Key == "" {
		t.Fatalf("unexpected normalized fields: %+v", finding)
	}
}

func TestNormalizedFindingKeyIncludesMessageWithStructuredSubject(t *testing.T) {
	base := NormalizeIssue(reports.Issue{
		Code:     reports.CodeInvalidNetAssignment,
		Severity: reports.SeverityError,
		Path:     "pcb/pad",
		Message:  "old wording",
	}, NormalizeFindingOptions{
		Source:   FindingSourceBoard,
		Category: FindingCategoryPadNet,
		Subject:  FindingSubject{Ref: "U1", Pad: "4", Net: "VCC"},
	})
	renamed := base
	renamed.Message = "new wording"
	renamed.Key = NormalizedFindingKey(renamed)
	if base.Key == renamed.Key {
		t.Fatalf("structured findings with different messages should not collide:\nbase=%q\nrenamed=%q", base.Key, renamed.Key)
	}
}

func TestNormalizedFindingKeySeparatesVariableLengthParts(t *testing.T) {
	left := NormalizedFinding{Source: "ab", Category: "c", Code: reports.CodeValidationFailed, Severity: reports.SeverityError}
	right := NormalizedFinding{Source: "a", Category: "bc", Code: reports.CodeValidationFailed, Severity: reports.SeverityError}
	if NormalizedFindingKey(left) == NormalizedFindingKey(right) {
		t.Fatalf("variable length key parts collided")
	}
}

func TestNormalizedFindingKeyFallsBackToMessageWhenSubjectIsEmpty(t *testing.T) {
	base := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "first"}, NormalizeFindingOptions{Source: FindingSourceRepair})
	renamed := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "second"}, NormalizeFindingOptions{Source: FindingSourceRepair})
	if base.Key == renamed.Key {
		t.Fatalf("sparse findings should use message fallback:\nbase=%q\nrenamed=%q", base.Key, renamed.Key)
	}
}

func TestNormalizedFindingKeyIncludesMessageForFileOnlySubject(t *testing.T) {
	base := NormalizeIssue(reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "out/demo.kicad_sch",
		Message:  "missing right paren",
	}, NormalizeFindingOptions{Source: FindingSourceWriter})
	other := NormalizeIssue(reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     "out/demo.kicad_sch",
		Message:  "unknown symbol library",
	}, NormalizeFindingOptions{Source: FindingSourceWriter})
	if base.Subject.File == "" || other.Subject.File == "" {
		t.Fatalf("expected file subjects: base=%+v other=%+v", base, other)
	}
	if base.Key == other.Key {
		t.Fatalf("file-only findings with different messages should not collide:\nbase=%q\nother=%q", base.Key, other.Key)
	}
}

func TestNormalizeIssuesSortsDeterministically(t *testing.T) {
	issues := []reports.Issue{
		{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Message: "b", Refs: []string{"U2"}},
		{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Message: "a", Refs: []string{"U1"}},
	}
	findings := NormalizeIssues(issues, NormalizeFindingOptions{Source: FindingSourceBoard})
	if len(findings) != 2 || findings[0].Subject.Ref != "U1" || findings[1].Subject.Ref != "U2" {
		t.Fatalf("findings not sorted deterministically: %+v", findings)
	}
}

func TestNormalizeIssueClassifiesExternalAndPreservationBlocked(t *testing.T) {
	external := NormalizeIssue(reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityError, Message: "missing kicad-cli"}, NormalizeFindingOptions{})
	if external.Category != FindingCategoryExternalTool || external.Repairability != RepairabilityExternalToolBlocked {
		t.Fatalf("external = %+v", external)
	}
	preservation := NormalizeIssue(reports.Issue{Code: reports.CodePreservationConflict, Severity: reports.SeverityBlocked, Message: "imported"}, NormalizeFindingOptions{})
	if preservation.Category != FindingCategoryPreservation || preservation.Repairability != RepairabilityPreservationBlocked {
		t.Fatalf("preservation = %+v", preservation)
	}
}

func TestNormalizeIssueTrimsCategoryBeforeDefaulting(t *testing.T) {
	finding := NormalizeIssue(
		reports.Issue{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Message: "missing outline"},
		NormalizeFindingOptions{Category: "   "},
	)
	if finding.Category != FindingCategoryOutline {
		t.Fatalf("category = %q, want outline", finding.Category)
	}
}

func TestCompareNormalizedFindingsReportsDeltaCounts(t *testing.T) {
	cleared := NormalizeIssue(reports.Issue{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Path: "outline", Message: "missing"}, NormalizeFindingOptions{Source: FindingSourceBoard})
	repeated := NormalizeIssue(reports.Issue{Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Path: "pad", Message: "disconnected", Refs: []string{"J1"}}, NormalizeFindingOptions{Source: FindingSourceBoard})
	newWarning := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "zone", Message: "zone fill evidence missing"}, NormalizeFindingOptions{Source: FindingSourceBoard, Category: FindingCategoryZone})
	delta := CompareNormalizedFindings([]NormalizedFinding{cleared, repeated}, []NormalizedFinding{repeated, newWarning})
	if delta.ClearedCount != 1 || delta.RepeatedCount != 1 || delta.NewCount != 1 {
		t.Fatalf("unexpected delta counts: %+v", delta)
	}
	if delta.Before.BlockingCount != 2 || delta.After.BlockingCount != 1 || !delta.Improved || delta.Worse {
		t.Fatalf("unexpected summary flags: %+v", delta)
	}
	if delta.ByCategory[string(FindingCategoryOutline)].Cleared != 1 || delta.ByCategory[string(FindingCategoryConnectivity)].Repeated != 1 {
		t.Fatalf("unexpected category deltas: %+v", delta.ByCategory)
	}
}

func TestCompareNormalizedFindingsPreservesDuplicateKeys(t *testing.T) {
	finding := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "same", Message: "same"}, NormalizeFindingOptions{Source: FindingSourceBoard})
	delta := CompareNormalizedFindings([]NormalizedFinding{finding, finding}, nil)
	if delta.Before.IssueCount != 2 || delta.ClearedCount != 2 {
		t.Fatalf("duplicate findings should be preserved: %+v", delta)
	}
}

func TestCompareNormalizedFindingsReportsWorsenedSeverity(t *testing.T) {
	before := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "drc", Message: "same"}, NormalizeFindingOptions{Source: FindingSourceKiCadDRC, Category: FindingCategoryBoardDRC})
	after := before
	after.Severity = reports.SeverityError
	after.Key = before.Key
	delta := CompareNormalizedFindings([]NormalizedFinding{before}, []NormalizedFinding{after})
	if delta.WorsenedCount != 1 || !delta.Worse || delta.StopReason != StopReasonRepeatedEvidence {
		t.Fatalf("unexpected worsened delta: %+v", delta)
	}
}

func TestCompareNormalizedFindingsTreatsSeverityReductionAsImprovement(t *testing.T) {
	before := NormalizeIssue(reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "drc", Message: "same"}, NormalizeFindingOptions{Source: FindingSourceKiCadDRC, Category: FindingCategoryBoardDRC})
	after := before
	after.Severity = reports.SeverityWarning
	after.Key = before.Key
	delta := CompareNormalizedFindings([]NormalizedFinding{before}, []NormalizedFinding{after})
	if !delta.Improved || delta.StopReason != StopReasonPartialNonBlocking {
		t.Fatalf("severity reduction should count as improvement to partial clean state: %+v", delta)
	}
}

func TestSelectConvergenceStopReasonPriority(t *testing.T) {
	clean := CompareNormalizedFindings(nil, nil)
	if clean.StopReason != StopReasonClean {
		t.Fatalf("clean stop reason = %q", clean.StopReason)
	}
	external := NormalizeIssue(reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityError, Message: "missing"}, NormalizeFindingOptions{Category: FindingCategoryExternalTool, Repairability: RepairabilityExternalToolBlocked})
	delta := CompareNormalizedFindings(nil, []NormalizedFinding{external})
	if delta.StopReason != StopReasonExternalToolBlocked {
		t.Fatalf("external stop reason = %q; delta=%+v", delta.StopReason, delta)
	}
	unsupported := NormalizeIssue(reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityBlocked, Message: "unsupported"}, NormalizeFindingOptions{Category: FindingCategoryUnsupported, Repairability: RepairabilityUnsupported})
	delta = CompareNormalizedFindings(nil, []NormalizedFinding{unsupported})
	if delta.StopReason != StopReasonUnsupportedFindings {
		t.Fatalf("unsupported stop reason = %q; delta=%+v", delta.StopReason, delta)
	}
}

func TestSelectConvergenceStopReasonDoesNotStopWhenStillImproving(t *testing.T) {
	cleared := NormalizeIssue(reports.Issue{Code: reports.CodeMissingBoardOutline, Severity: reports.SeverityError, Path: "outline", Message: "missing"}, NormalizeFindingOptions{Category: FindingCategoryOutline})
	remaining := NormalizeIssue(reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityBlocked, Path: "unsupported", Message: "unsupported"}, NormalizeFindingOptions{Category: FindingCategoryUnsupported, Repairability: RepairabilityUnsupported})
	delta := CompareNormalizedFindings([]NormalizedFinding{cleared, remaining}, []NormalizedFinding{remaining})
	if !delta.Improved {
		t.Fatalf("delta should show improvement: %+v", delta)
	}
	if delta.StopReason != "" {
		t.Fatalf("improving delta should not produce terminal stop reason yet: %+v", delta)
	}
}
