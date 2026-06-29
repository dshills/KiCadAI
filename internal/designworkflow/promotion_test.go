package designworkflow

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestMarshalPromotionReportJSONStable(t *testing.T) {
	report := PromotionReport{
		ID:                 "connector_led_kicad_smoke",
		Request:            "connector_led_kicad_smoke.json",
		Tier:               "block-composition",
		DeclaredReadiness:  PromotionReadinessExpectedFail,
		AchievedReadiness:  PromotionReadinessExpectedFail,
		Acceptance:         AcceptanceERCDRC,
		Status:             PromotionStatusExpectedFail,
		MatchesExpectation: true,
		Summary:            "Inter-block LED_EN route emits contact-miss evidence.",
		Gates: []PromotionGate{{
			ID:          "route_completion",
			Status:      PromotionGateStatusFailed,
			RequiredFor: []PromotionReadiness{PromotionReadinessPass, PromotionReadinessCandidate},
			IssueCodes:  []string{"route_contact_miss"},
			Artifacts:   []string{".kicadai/transaction.json"},
		}, {
			ID:          "metadata",
			Status:      PromotionGateStatusPass,
			RequiredFor: []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass},
		}},
		Stages: PromotionStageReport{
			Expected:  []StageName{StageValidation, StageBlockPlanning},
			Reached:   []StageName{StageBlockPlanning, StageValidation},
			StoppedAt: StageValidation,
		},
		Issues: []PromotionIssue{{
			Code:     "route_contact_miss",
			Severity: reports.SeverityError,
			Stage:    StageValidation,
			Message:  "LED_EN route does not graph-connect all required contacts.",
			Repair:   "Retry inter-block routing with pad-anchor endpoints.",
		}},
		Artifacts: []PromotionArtifact{{
			Path: ".kicadai/transaction.json",
			Kind: reports.ArtifactValidationReport,
		}},
	}
	data, err := MarshalPromotionReportJSON(report)
	if err != nil {
		t.Fatalf("marshal promotion report: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		`"id": "connector_led_kicad_smoke"`,
		`"status": "expected_fail"`,
		`"id": "metadata"`,
		`"id": "route_completion"`,
		`"issue_codes": [`,
		`"path": ".kicadai/transaction.json"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("promotion report JSON missing %q:\n%s", want, got)
		}
	}
	first := strings.Index(got, `"id": "metadata"`)
	second := strings.Index(got, `"id": "route_completion"`)
	if first < 0 || second < 0 || first > second {
		t.Fatalf("gates not sorted by id:\n%s", got)
	}
}

func TestPromotionReportValidationRejectsEmptyReport(t *testing.T) {
	if _, err := MarshalPromotionReportJSON(PromotionReport{}); err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("empty report error = %v, want id is required", err)
	}
}

func TestPromotionReportValidationRejectsUnsupportedGateStatus(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: "almost",
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "unsupported promotion gate status") {
		t.Fatalf("gate status error = %v, want unsupported promotion gate status", err)
	}
}

func TestPromotionReportValidationRejectsUnsupportedRequiredReadiness(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:          "metadata",
			Status:      PromotionGateStatusPass,
			RequiredFor: []PromotionReadiness{"almost"},
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "unsupported promotion gate required_for readiness") {
		t.Fatalf("required_for error = %v, want unsupported promotion gate required_for readiness", err)
	}
}

func TestPromotionReportValidationRejectsInvalidAcceptanceAndTimestamp(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	withAcceptance := base
	withAcceptance.Acceptance = "almost"
	if _, err := MarshalPromotionReportJSON(withAcceptance); err == nil || !strings.Contains(err.Error(), "unsupported promotion acceptance") {
		t.Fatalf("acceptance validation error = %v, want unsupported promotion acceptance", err)
	}
	withTimestamp := base
	withTimestamp.GeneratedAt = "not-a-time"
	if _, err := MarshalPromotionReportJSON(withTimestamp); err == nil || !strings.Contains(err.Error(), "generated_at must be RFC3339") {
		t.Fatalf("timestamp validation error = %v, want generated_at must be RFC3339", err)
	}
}

func TestPromotionReportValidationRejectsInvalidStageAndArtifactKind(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	withStage := base
	withStage.Stages.Expected = []StageName{"almost"}
	if _, err := MarshalPromotionReportJSON(withStage); err == nil || !strings.Contains(err.Error(), "unsupported promotion expected stage") {
		t.Fatalf("stage validation error = %v, want unsupported promotion expected stage", err)
	}
	withArtifact := base
	withArtifact.Artifacts = []PromotionArtifact{{
		Path: ".kicadai/report.json",
		Kind: "almost",
	}}
	if _, err := MarshalPromotionReportJSON(withArtifact); err == nil || !strings.Contains(err.Error(), "unsupported promotion artifact kind") {
		t.Fatalf("artifact kind validation error = %v, want unsupported promotion artifact kind", err)
	}
}

func TestNormalizePromotionReportDoesNotMutateInput(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:          "route",
			Status:      PromotionGateStatusPass,
			RequiredFor: []PromotionReadiness{PromotionReadinessPass, PromotionReadinessCandidate},
			IssueCodes:  []string{"z", "a"},
			Artifacts:   []string{"z.json", "a.json"},
		}},
		Stages: PromotionStageReport{
			Expected: []StageName{StageBlockPlanning, StageValidation},
			Reached:  []StageName{StageBlockPlanning, StageValidation},
		},
		Issues: []PromotionIssue{{
			Code: "issue",
			Refs: []string{"Z", "A"},
			Nets: []string{"Z", "A"},
		}},
	}
	_ = NormalizePromotionReport(report)
	if got := report.Gates[0].RequiredFor[0]; got != PromotionReadinessPass {
		t.Fatalf("input RequiredFor mutated, first = %q", got)
	}
	if got := report.Gates[0].IssueCodes[0]; got != "z" {
		t.Fatalf("input IssueCodes mutated, first = %q", got)
	}
	if got := report.Issues[0].Refs[0]; got != "Z" {
		t.Fatalf("input issue refs mutated, first = %q", got)
	}
	if got := report.Stages.Expected[0]; got != StageBlockPlanning {
		t.Fatalf("stage order mutated, first = %q", got)
	}
}

func TestNormalizePromotionReportCompactsSortedRepeatedValues(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:          "metadata",
			Status:      PromotionGateStatusPass,
			RequiredFor: []PromotionReadiness{PromotionReadinessPass, PromotionReadinessPass, PromotionReadinessCandidate},
			IssueCodes:  []string{"a", "a"},
			Artifacts:   []string{".kicadai/report.json", ".kicadai/report.json"},
		}},
		Issues: []PromotionIssue{{
			Code:     "a",
			Severity: reports.SeverityWarning,
			Message:  "warning",
			Refs:     []string{"R1", "R1"},
			Nets:     []string{"GND", "GND"},
		}},
		Artifacts: []PromotionArtifact{{Path: ".kicadai/report.json"}},
	}
	normalized := NormalizePromotionReport(report)
	if got := len(normalized.Gates[0].RequiredFor); got != 2 {
		t.Fatalf("required_for count = %d, want 2", got)
	}
	if got := len(normalized.Gates[0].IssueCodes); got != 1 {
		t.Fatalf("issue_codes count = %d, want 1", got)
	}
	if got := len(normalized.Issues[0].Refs); got != 1 {
		t.Fatalf("refs count = %d, want 1", got)
	}
}

func TestPromotionReportValidationRejectsIncompleteIssuesAndArtifacts(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	withIssue := base
	withIssue.Issues = []PromotionIssue{{Message: "missing code"}}
	if _, err := MarshalPromotionReportJSON(withIssue); err == nil || !strings.Contains(err.Error(), "promotion issue code is required") {
		t.Fatalf("issue validation error = %v, want issue code required", err)
	}
	withArtifact := base
	withArtifact.Artifacts = []PromotionArtifact{{Kind: reports.ArtifactValidationReport}}
	if _, err := MarshalPromotionReportJSON(withArtifact); err == nil || !strings.Contains(err.Error(), "promotion artifact path is required") {
		t.Fatalf("artifact validation error = %v, want artifact path required", err)
	}
	withBadSeverity := base
	withBadSeverity.Issues = []PromotionIssue{{
		Code:     "issue",
		Severity: "almost",
		Message:  "bad severity",
	}}
	if _, err := MarshalPromotionReportJSON(withBadSeverity); err == nil || !strings.Contains(err.Error(), "unsupported promotion issue severity") {
		t.Fatalf("severity validation error = %v, want unsupported promotion issue severity", err)
	}
	withMissingMessage := base
	withMissingMessage.Issues = []PromotionIssue{{
		Code:     "issue",
		Severity: reports.SeverityError,
	}}
	if _, err := MarshalPromotionReportJSON(withMissingMessage); err == nil || !strings.Contains(err.Error(), "promotion issue message is required") {
		t.Fatalf("message validation error = %v, want promotion issue message required", err)
	}
	withDuplicateIssue := base
	withDuplicateIssue.Issues = []PromotionIssue{{
		Code:     "issue",
		Severity: reports.SeverityError,
		Message:  "first",
	}, {
		Code:     "issue",
		Severity: reports.SeverityWarning,
		Message:  "second",
	}}
	if _, err := MarshalPromotionReportJSON(withDuplicateIssue); err == nil || !strings.Contains(err.Error(), "duplicate promotion issue code") {
		t.Fatalf("duplicate issue validation error = %v, want duplicate promotion issue code", err)
	}
}

func TestPromotionReportValidationRejectsDuplicateGatesAndArtifacts(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}, {
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	if _, err := MarshalPromotionReportJSON(base); err == nil || !strings.Contains(err.Error(), "duplicate promotion gate id") {
		t.Fatalf("duplicate gate error = %v, want duplicate promotion gate id", err)
	}
	withArtifact := base
	withArtifact.Gates = withArtifact.Gates[:1]
	withArtifact.Artifacts = []PromotionArtifact{{
		Path: ".kicadai/report.json",
	}, {
		Path: ".kicadai/report.json",
	}}
	if _, err := MarshalPromotionReportJSON(withArtifact); err == nil || !strings.Contains(err.Error(), "duplicate promotion artifact path") {
		t.Fatalf("duplicate artifact error = %v, want duplicate promotion artifact path", err)
	}
}

func TestPromotionReportValidationRejectsMissingGateReferences(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessBlocked,
		Status:            PromotionStatusFailed,
		Gates: []PromotionGate{{
			ID:         "route",
			Status:     PromotionGateStatusFailed,
			IssueCodes: []string{"missing_issue"},
		}},
	}
	if _, err := MarshalPromotionReportJSON(base); err == nil || !strings.Contains(err.Error(), "references missing issue code") {
		t.Fatalf("missing issue reference error = %v, want missing issue code", err)
	}
	withArtifact := base
	withArtifact.Gates = []PromotionGate{{
		ID:        "route",
		Status:    PromotionGateStatusFailed,
		Artifacts: []string{".kicadai/missing.json"},
	}}
	if _, err := MarshalPromotionReportJSON(withArtifact); err == nil || !strings.Contains(err.Error(), "references missing artifact") {
		t.Fatalf("missing artifact reference error = %v, want missing artifact", err)
	}
	withIssueArtifact := base
	withIssueArtifact.Gates = []PromotionGate{{
		ID:     "route",
		Status: PromotionGateStatusFailed,
	}}
	withIssueArtifact.Issues = []PromotionIssue{{
		Code:     "issue",
		Severity: reports.SeverityError,
		Message:  "references artifact",
		Artifact: ".kicadai/missing.json",
	}}
	if _, err := MarshalPromotionReportJSON(withIssueArtifact); err == nil || !strings.Contains(err.Error(), "promotion issue \"issue\" references missing artifact") {
		t.Fatalf("missing issue artifact reference error = %v, want missing artifact", err)
	}
}

func TestPromotionReportValidationRejectsStoppedStageThatWasNotReached(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
		Stages: PromotionStageReport{
			Reached:   []StageName{StageBlockPlanning},
			StoppedAt: StageValidation,
		},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "was not reached") {
		t.Fatalf("stopped_at validation error = %v, want was not reached", err)
	}
}

func TestPromotionReportValidationRejectsPassWithFailedRequiredGate(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessPass,
		AchievedReadiness: PromotionReadinessPass,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:          "route_completion",
			Status:      PromotionGateStatusFailed,
			RequiredFor: []PromotionReadiness{PromotionReadinessPass},
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "blocks achieved readiness") {
		t.Fatalf("readiness consistency error = %v, want blocks achieved readiness", err)
	}
}

func TestPromotionReportValidationAllowsCandidateWithRequiredWarningGate(t *testing.T) {
	report := PromotionReport{
		ID:                 "fixture",
		DeclaredReadiness:  PromotionReadinessCandidate,
		AchievedReadiness:  PromotionReadinessCandidate,
		Status:             PromotionStatusWarn,
		MatchesExpectation: true,
		Gates: []PromotionGate{{
			ID:          "physical_rules",
			Status:      PromotionGateStatusWarn,
			RequiredFor: []PromotionReadiness{PromotionReadinessCandidate},
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err != nil {
		t.Fatalf("candidate warning gate rejected: %v", err)
	}
}

func TestPromotionReportValidationRequiresUnexpectedPassStatus(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "unexpected pass requires unexpected_pass status") {
		t.Fatalf("unexpected pass validation error = %v, want unexpected_pass status", err)
	}
	report.Status = PromotionStatusUnexpectedPass
	if _, err := MarshalPromotionReportJSON(report); err != nil {
		t.Fatalf("unexpected_pass status rejected: %v", err)
	}
}

func TestPromotionReportValidationRejectsDuplicateGateReferences(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:          "metadata",
			Status:      PromotionGateStatusPass,
			RequiredFor: []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessCandidate},
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "duplicate required_for") {
		t.Fatalf("duplicate required_for error = %v, want duplicate required_for", err)
	}
	report.Gates[0].RequiredFor = nil
	report.Gates[0].IssueCodes = []string{"issue", "issue"}
	report.Issues = []PromotionIssue{{Code: "issue", Severity: reports.SeverityError, Message: "issue"}}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "duplicate issue code") {
		t.Fatalf("duplicate issue code error = %v, want duplicate issue code", err)
	}
	report.Gates[0].IssueCodes = nil
	report.Gates[0].Artifacts = []string{".kicadai/report.json", ".kicadai/report.json"}
	report.Artifacts = []PromotionArtifact{{Path: ".kicadai/report.json"}}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "duplicate artifact") {
		t.Fatalf("duplicate artifact error = %v, want duplicate artifact", err)
	}
}

func TestPromotionReportValidationRejectsInvalidNextActionReferences(t *testing.T) {
	base := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
	}
	missingGate := base
	missingGate.NextActions = []PromotionNextAction{{
		Gate:     "missing",
		Severity: reports.SeverityWarning,
		Summary:  "missing gate",
		Action:   "fix the missing gate",
	}}
	if _, err := MarshalPromotionReportJSON(missingGate); err == nil || !strings.Contains(err.Error(), "references missing gate") {
		t.Fatalf("missing next-action gate error = %v, want references missing gate", err)
	}
	missingIssue := base
	missingIssue.NextActions = []PromotionNextAction{{
		Gate:       "metadata",
		Severity:   reports.SeverityWarning,
		Summary:    "missing issue",
		Action:     "fix the missing issue",
		IssueCodes: []string{"missing_issue"},
	}}
	if _, err := MarshalPromotionReportJSON(missingIssue); err == nil || !strings.Contains(err.Error(), "references missing issue code") {
		t.Fatalf("missing next-action issue error = %v, want references missing issue code", err)
	}
}

func TestPromotionReportValidationRejectsDuplicateNextActionsIgnoringReferenceOrder(t *testing.T) {
	report := PromotionReport{
		ID:                "fixture",
		DeclaredReadiness: PromotionReadinessCandidate,
		AchievedReadiness: PromotionReadinessCandidate,
		Status:            PromotionStatusPass,
		Gates: []PromotionGate{{
			ID:     "metadata",
			Status: PromotionGateStatusPass,
		}},
		Issues: []PromotionIssue{{
			Code:     "a",
			Severity: reports.SeverityWarning,
			Message:  "a",
		}, {
			Code:     "b",
			Severity: reports.SeverityWarning,
			Message:  "b",
		}},
		Artifacts: []PromotionArtifact{{Path: "a.json"}, {Path: "b.json"}},
		NextActions: []PromotionNextAction{{
			Gate:       "metadata",
			Severity:   reports.SeverityWarning,
			Summary:    "same",
			Action:     "same action",
			IssueCodes: []string{"a", "b"},
			Artifacts:  []string{"a.json", "b.json"},
		}, {
			Gate:       "metadata",
			Severity:   reports.SeverityWarning,
			Summary:    "same",
			Action:     "same action",
			IssueCodes: []string{"b", "a"},
			Artifacts:  []string{"b.json", "a.json"},
		}},
	}
	if _, err := MarshalPromotionReportJSON(report); err == nil || !strings.Contains(err.Error(), "duplicate promotion next action") {
		t.Fatalf("duplicate next-action error = %v, want duplicate promotion next action", err)
	}
}
