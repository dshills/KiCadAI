package schematicrules

import (
	"encoding/json"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestStatusForFindings(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     Status
	}{
		{name: "clean", want: StatusClean},
		{name: "info only", findings: []Finding{{Severity: reports.SeverityInfo}}, want: StatusClean},
		{name: "warning", findings: []Finding{{Severity: reports.SeverityWarning}}, want: StatusWarning},
		{name: "error blocks", findings: []Finding{{Severity: reports.SeverityError}}, want: StatusBlocked},
		{name: "blocked", findings: []Finding{{Severity: reports.SeverityBlocked}}, want: StatusBlocked},
		{name: "unknown severity", findings: []Finding{{Severity: reports.Severity("maybe")}}, want: StatusUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StatusForFindings(test.findings); got != test.want {
				t.Fatalf("StatusForFindings() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReportNormalizeSortsFindingsAndCounts(t *testing.T) {
	report := NewReport(Report{
		CheckedSymbols: 2,
		Findings: []Finding{
			{RuleID: RuleLabelFloating, Severity: reports.SeverityWarning, Category: CategoryNet, Path: "labels[1].position", Message: "floating"},
			{RuleID: RuleReferenceDuplicate, Severity: reports.SeverityBlocked, Category: CategoryReference, Reference: "R1", Message: "duplicate"},
			{RuleID: RulePowerSourceMissing, Severity: reports.SeverityError, Category: CategoryPower, Net: "VCC", Message: "missing source"},
		},
	})

	if report.Status != StatusBlocked || report.FindingCount != 3 {
		t.Fatalf("report status/count = %q/%d", report.Status, report.FindingCount)
	}
	if report.Findings[0].RuleID != RuleReferenceDuplicate {
		t.Fatalf("first finding = %s, want blocked duplicate reference", report.Findings[0].RuleID)
	}
	if report.Findings[1].RuleID != RulePowerSourceMissing {
		t.Fatalf("second finding = %s, want error before warning", report.Findings[1].RuleID)
	}
}

func TestNewReportDoesNotMutateInputFindings(t *testing.T) {
	findings := []Finding{
		{RuleID: RuleLabelFloating, Severity: reports.SeverityWarning, Category: CategoryNet, Message: "floating"},
		{RuleID: RuleReferenceDuplicate, Severity: reports.SeverityBlocked, Category: CategoryReference, Message: "duplicate"},
	}
	report := NewReport(Report{Findings: findings})

	if findings[0].RuleID != RuleLabelFloating {
		t.Fatalf("NewReport mutated caller findings: %#v", findings)
	}
	if report.Findings[0].RuleID != RuleReferenceDuplicate {
		t.Fatalf("report findings were not sorted: %#v", report.Findings)
	}
}

func TestNormalizePreservesNotApplicable(t *testing.T) {
	report := NewReport(Report{Status: StatusNotApplicable})
	if report.Status != StatusNotApplicable {
		t.Fatalf("status = %q, want not_applicable", report.Status)
	}
}

func TestReportJSONShape(t *testing.T) {
	report := NewReport(Report{
		CheckedSymbols:                8,
		CheckedNets:                   5,
		CheckedPowerRails:             2,
		CheckedRequiredPins:           14,
		CheckedDecouplingRequirements: 3,
		Findings: []Finding{{
			RuleID:    RulePowerSourceMissing,
			Severity:  reports.SeverityBlocked,
			Category:  CategoryPower,
			Reference: "U1",
			Pin:       "8",
			Net:       "VCC",
			Message:   "VCC has power sinks but no modeled source or accepted external driver",
			Repair:    "add a regulator, connector power input, battery source, or explicit external-driver policy",
		}},
	})

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	output := string(encoded)
	for _, want := range []string{
		`"status":"blocked"`,
		`"checked_symbols":8`,
		`"finding_count":1`,
		`"rule_id":"SCH_POWER_SOURCE_MISSING"`,
		`"severity":"blocked"`,
		`"category":"power"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("JSON output missing %s: %s", want, output)
		}
	}
}
