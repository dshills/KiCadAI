package checks

import "testing"

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Timeout <= 0 {
		t.Fatal("expected positive default timeout")
	}
	if opts.Units != "mm" {
		t.Fatalf("default units = %q, want mm", opts.Units)
	}
}

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		name         string
		toolError    bool
		skipped      bool
		findings     []CheckFinding
		parserIssues []ParserIssue
		want         CheckStatus
	}{
		{name: "pass", want: CheckStatusPass},
		{name: "fail", findings: []CheckFinding{{Message: "violation"}}, want: CheckStatusFail},
		{name: "skipped", skipped: true, findings: []CheckFinding{{Message: "ignored"}}, want: CheckStatusSkipped},
		{name: "tool error", toolError: true, want: CheckStatusError},
		{name: "parser error", parserIssues: []ParserIssue{{Message: "bad report"}}, want: CheckStatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyStatus(tt.toolError, tt.skipped, tt.findings, tt.parserIssues); got != tt.want {
				t.Fatalf("ClassifyStatus() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNormalizeFindingsAddsStableIDsAndSorts(t *testing.T) {
	findings := []CheckFinding{
		{Severity: "warning", Rule: "unconnected", Message: "B", References: []string{"R2"}},
		{Severity: "error", Rule: "clearance", Message: "A", References: []string{"R1"}},
	}
	normalized := NormalizeFindings(CheckKindDRC, findings)
	if len(normalized) != 2 {
		t.Fatalf("len = %d, want 2", len(normalized))
	}
	if normalized[0].Severity != "error" {
		t.Fatalf("first severity = %q, want error", normalized[0].Severity)
	}
	if normalized[0].ID == "" || normalized[1].ID == "" {
		t.Fatalf("expected IDs: %#v", normalized)
	}
	again := NormalizeFindings(CheckKindDRC, findings)
	if normalized[0].ID != again[0].ID || normalized[1].ID != again[1].ID {
		t.Fatalf("IDs are not stable: %#v vs %#v", normalized, again)
	}
}

func TestClassifyRepairCategory(t *testing.T) {
	tests := []struct {
		name string
		in   CheckFinding
		want RepairCategory
	}{
		{name: "clearance", in: CheckFinding{Code: "clearance_violation"}, want: RepairClearance},
		{name: "outline", in: CheckFinding{Message: "missing board outline"}, want: RepairOutline},
		{name: "power", in: CheckFinding{Message: "power input pin not driven"}, want: RepairPower},
		{name: "unknown", in: CheckFinding{Message: "custom violation"}, want: RepairUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRepairCategory(tt.in); got != tt.want {
				t.Fatalf("ClassifyRepairCategory() = %s, want %s", got, tt.want)
			}
		})
	}
}
