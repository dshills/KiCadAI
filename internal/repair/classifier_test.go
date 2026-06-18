package repair

import (
	"testing"

	"kicadai/internal/reports"
)

func TestClassifyKnownIssueCodes(t *testing.T) {
	tests := []struct {
		name       string
		issue      reports.Issue
		want       Category
		repairable bool
	}{
		{name: "missing footprint", issue: reports.Issue{Code: reports.CodeMissingFootprint}, want: CategoryMissingFootprint, repairable: true},
		{name: "disconnected pad", issue: reports.Issue{Code: reports.CodeDisconnectedPad}, want: CategoryDisconnectedPad, repairable: true},
		{name: "invalid net", issue: reports.Issue{Code: reports.CodeInvalidNetAssignment}, want: CategoryInvalidNetAssignment, repairable: true},
		{name: "missing outline", issue: reports.Issue{Code: reports.CodeMissingBoardOutline}, want: CategoryMissingBoardOutline, repairable: true},
		{name: "placement collision", issue: reports.Issue{Code: reports.CodePlacementCollision}, want: CategoryPlacementCollision, repairable: true},
		{name: "placement outside", issue: reports.Issue{Code: reports.CodePlacementOutsideBoard}, want: CategoryPlacementOutside, repairable: true},
		{name: "kicad unavailable", issue: reports.Issue{Code: reports.CodeSkippedExternalTool}, want: CategoryKiCadCLIUnavailable, repairable: false},
		{name: "roundtrip diff", issue: reports.Issue{Code: reports.CodeRoundTripDiff}, want: CategoryRoundTripDiff, repairable: false},
		{name: "unsupported", issue: reports.Issue{Code: reports.CodeUnsupportedImportedObject}, want: CategoryUnsupportedObject, repairable: false},
		{name: "unsafe", issue: reports.Issue{Code: reports.CodeUnsafeRemove}, want: CategoryUnsafeUserContent, repairable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.issue)
			if got.Category != tt.want || got.Repairable != tt.repairable {
				t.Errorf("Classify() = %#v, want category=%s repairable=%t", got, tt.want, tt.repairable)
			}
		})
	}
}

func TestClassifyMessagePatterns(t *testing.T) {
	tests := []struct {
		name  string
		issue reports.Issue
		want  Category
	}{
		{name: "unrouted", issue: reports.Issue{Code: reports.CodeValidationFailed, Message: "net GND is unrouted"}, want: CategoryUnroutedNet},
		{name: "clearance", issue: reports.Issue{Code: reports.CodeValidationFailed, Message: "track clearance violation"}, want: CategoryRouteClearance},
		{name: "zone unfilled", issue: reports.Issue{Code: reports.CodeValidationFailed, Message: "zone has no fill evidence"}, want: CategoryZoneUnfilled},
		{name: "zone wrong net", issue: reports.Issue{Code: reports.CodeInvalidNetAssignment, Path: "zones.GND", Message: "net name does not match"}, want: CategoryZoneWrongNet},
		{name: "unknown symbol", issue: reports.Issue{Code: reports.CodeValidationFailed, Message: "unknown symbol library Device"}, want: CategoryUnknownSymbol},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.issue)
			if got.Category != tt.want || !got.Repairable {
				t.Errorf("Classify() = %#v, want repairable %s", got, tt.want)
			}
		})
	}
}

func TestClassifyUnknownIssueIsNotRepairable(t *testing.T) {
	got := Classify(reports.Issue{Code: reports.CodeUnknown, Message: "mystery"})
	if got.Category != CategoryUnknown || got.Repairable || got.Reason == "" {
		t.Fatalf("Classify() = %#v", got)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.MaxAttempts != 3 || opts.MaxAttemptsPerIssue != 1 {
		t.Fatalf("DefaultOptions() = %#v", opts)
	}
}
