package designworkflow

import (
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestBuildInternalPromotionReportCandidate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "led_indicator_kicad_smoke",
		Request:           "led_indicator_kicad_smoke.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedArtifacts: []string{".kicadai/transaction.json"},
		ExpectedStages:    []StageName{StageBlockPlanning, StageWriterCorrect, StageValidation, StageRouting, StageFabricationReady},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "led"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageWriterCorrect, Status: StageStatusOK, Artifacts: []reports.Artifact{{
			Path: ".kicadai/transaction.json",
			Kind: reports.ArtifactValidationReport,
		}}},
		{Name: StageValidation, Status: StageStatusOK},
		{Name: StageRouting, Status: StageStatusOK, Summary: map[string]any{
			"route_connectivity": LocalRouteConnectivitySummary{RoutesAttempted: 1, EndpointContactsProven: 2},
		}},
		{Name: StageFabricationReady, Status: StageStatusOK},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if report.AchievedReadiness != PromotionReadinessPass {
		t.Fatalf("achieved readiness = %q, want pass", report.AchievedReadiness)
	}
	if report.Status != PromotionStatusPass {
		t.Fatalf("status = %q, want pass", report.Status)
	}
	if report.MatchesExpectation {
		t.Fatal("matches_expectation = true, want false because declared candidate achieved pass")
	}
	if _, err := MarshalPromotionReportJSON(report); err != nil {
		t.Fatalf("promotion report validation failed: %v", err)
	}
	if len(report.Artifacts) == 0 || report.Artifacts[0].Kind != reports.ArtifactValidationReport || !report.Artifacts[0].Required {
		t.Fatalf("artifact metadata not merged: %#v", report.Artifacts)
	}
}

func TestBuildInternalPromotionReportExpectedFail(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "connector_led_kicad_smoke",
		Request:           "connector_led_kicad_smoke.json",
		Tier:              "block-composition",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		Acceptance:        AcceptanceERCDRC,
		ExpectedStages:    []StageName{StageBlockPlanning, StageValidation},
		KnownGaps:         []string{"route contact miss"},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "connector_led"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageValidation, Status: StageStatusBlocked, Issues: []reports.Issue{{
			Code:     reports.CodeRouteContactMiss,
			Severity: reports.SeverityError,
			Message:  "LED_EN route does not graph-connect all required contacts",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if report.AchievedReadiness != PromotionReadinessExpectedFail {
		t.Fatalf("achieved readiness = %q, want expected_fail", report.AchievedReadiness)
	}
	if report.Status != PromotionStatusExpectedFail {
		t.Fatalf("status = %q, want expected_fail", report.Status)
	}
	if !report.MatchesExpectation {
		t.Fatal("expected matches_expectation for declared expected_fail")
	}
	if _, err := MarshalPromotionReportJSON(report); err != nil {
		t.Fatalf("promotion report validation failed: %v", err)
	}
}

func TestBuildInternalPromotionReportMissingExpectedArtifact(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "led_indicator_kicad_smoke",
		Request:           "led_indicator_kicad_smoke.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedArtifacts: []string{".kicadai/transaction.json"},
		ExpectedStages:    []StageName{StageBlockPlanning},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "led"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if report.Status != PromotionStatusFailed {
		t.Fatalf("status = %q, want failed", report.Status)
	}
	var artifactGate PromotionGate
	for _, gate := range report.Gates {
		if gate.ID == "artifacts" {
			artifactGate = gate
			break
		}
	}
	if artifactGate.Status != PromotionGateStatusFailed {
		t.Fatalf("artifact gate status = %q, want failed", artifactGate.Status)
	}
}

func TestPromotionBuilderPassOnlyGateDoesNotBlockCandidate(t *testing.T) {
	builder := promotionReportBuilder{fixture: PromotionFixture{DeclaredReadiness: PromotionReadinessCandidate}}
	builder.gates = []PromotionGate{{
		ID:          "candidate",
		Status:      PromotionGateStatusPass,
		RequiredFor: []PromotionReadiness{PromotionReadinessCandidate},
	}, {
		ID:          "pass_only",
		Status:      PromotionGateStatusNotRun,
		RequiredFor: []PromotionReadiness{PromotionReadinessPass},
	}}
	if got := builder.achievedReadiness(); got != PromotionReadinessCandidate {
		t.Fatalf("achieved readiness = %q, want candidate", got)
	}
}

func TestPromotionBuilderDoesNotRequireIrrelevantStageGates(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "planning_only",
		Request:           "planning_only.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceStructural,
		ExpectedStages:    []StageName{StageBlockPlanning},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "planning"}, AcceptanceStructural, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if report.AchievedReadiness != PromotionReadinessPass {
		t.Fatalf("achieved readiness = %q, want pass from relevant gates only", report.AchievedReadiness)
	}
	for _, gate := range report.Gates {
		if gate.ID == "writer_correctness" && len(gate.RequiredFor) != 0 {
			t.Fatalf("irrelevant writer gate required_for = %#v, want empty", gate.RequiredFor)
		}
	}
}

func TestPromotionBuilderKeepsDuplicateWorkflowIssues(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "blocked",
		Request:           "blocked.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageValidation},
	}
	issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "same"}
	result := BuildWorkflowResult(ProjectSummary{Name: "blocked"}, AcceptanceConnectivity, []StageResult{
		{Name: StageValidation, Status: StageStatusBlocked, Issues: []reports.Issue{issue, issue}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if got := len(report.Issues); got != 2 {
		t.Fatalf("issue count = %d, want duplicate issues preserved", got)
	}
}

func TestPromotionBuilderKeepsBlankExpectedArtifactIssuesDistinct(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "bad_artifacts",
		Request:           "bad_artifacts.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceStructural,
		ExpectedArtifacts: []string{"", " "},
		ExpectedStages:    []StageName{StageBlockPlanning},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "bad_artifacts"}, AcceptanceStructural, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
	})
	report := BuildInternalPromotionReport(fixture, result)
	seen := map[string]bool{}
	for _, gate := range report.Gates {
		if gate.ID != "artifacts" {
			continue
		}
		for _, code := range gate.IssueCodes {
			if seen[code] {
				t.Fatalf("duplicate artifact issue code %q in %#v", code, gate.IssueCodes)
			}
			seen[code] = true
		}
	}
	if len(seen) != 2 {
		t.Fatalf("artifact issue code count = %d, want 2", len(seen))
	}
}

func TestBuildPromotionReportKiCadChecksMissingCLIBlocksCandidate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "requires_kicad",
		Request:           "requires_kicad.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "requires_kicad"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageKiCadChecks, Status: StageStatusBlocked, Issues: []reports.Issue{{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityBlocked,
			Message:  "kicad-cli not configured",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusSkipped {
		t.Fatalf("kicad gate status = %q, want skipped", gate.Status)
	}
	if len(gate.RequiredFor) != 2 {
		t.Fatalf("kicad gate required_for = %#v, want candidate/pass", gate.RequiredFor)
	}
	if report.AchievedReadiness != PromotionReadinessBlocked {
		t.Fatalf("achieved readiness = %q, want blocked", report.AchievedReadiness)
	}
}

func TestBuildPromotionReportKiCadChecksCleanEvidencePasses(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "clean_kicad",
		Request:           "clean_kicad.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "clean_kicad"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusOK,
			Summary: map[string]any{
				"erc": checks.CheckResult{Kind: checks.CheckKindERC, Status: checks.CheckStatusPass},
				"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusPass},
			},
			Artifacts: []reports.Artifact{{
				Kind: reports.ArtifactERCReport,
				Path: ".kicadai/erc.json",
			}, {
				Kind: reports.ArtifactDRCReport,
				Path: ".kicadai/drc.json",
			}},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusPass {
		t.Fatalf("kicad gate status = %q, want pass", gate.Status)
	}
	if len(gate.Artifacts) != 2 {
		t.Fatalf("kicad artifacts = %#v, want ERC and DRC reports", gate.Artifacts)
	}
}

func TestBuildPromotionReportKiCadChecksUnexpectedFindingFails(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "failing_kicad",
		Request:           "failing_kicad.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "failing_kicad"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusBlocked,
			Summary: map[string]any{
				"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusFail},
			},
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Message:  "clearance violation",
			}},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusFailed {
		t.Fatalf("kicad gate status = %q, want failed", gate.Status)
	}
	if report.Status != PromotionStatusFailed {
		t.Fatalf("status = %q, want failed", report.Status)
	}
}

func TestBuildPromotionReportKiCadChecksMissingRequiredSubcheckFails(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "missing_erc",
		Request:           "missing_erc.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "missing_erc"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusOK,
			Summary: map[string]any{
				"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusPass},
			},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusFailed {
		t.Fatalf("kicad gate status = %q, want failed", gate.Status)
	}
	if !containsPromotionIssueCode(gate.IssueCodes, "kicad_erc_missing") {
		t.Fatalf("issue codes = %#v, want kicad_erc_missing", gate.IssueCodes)
	}
}

func TestBuildPromotionReportKiCadChecksAllowedFindingsPass(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "allowed_kicad",
		Request:           "allowed_kicad.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "allowed_kicad"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusOK,
			Summary: map[string]any{
				"erc": checks.CheckResult{Kind: checks.CheckKindERC, Status: checks.CheckStatusPass, Allowed: []checks.CheckFinding{{Message: "allowlisted"}}},
			},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusPass {
		t.Fatalf("kicad gate status = %q, want pass for allowlisted findings", gate.Status)
	}
}

func TestPromotionKiCadSummaryStatusFailureTakesPrecedence(t *testing.T) {
	status := promotionGateStatusForKiCadStage(StageResult{
		Name:   StageKiCadChecks,
		Status: StageStatusOK,
		Summary: map[string]any{
			"erc": checks.CheckResult{Kind: checks.CheckKindERC, Status: checks.CheckStatusFail},
			"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusSkipped},
		},
	})
	if status != PromotionGateStatusFailed {
		t.Fatalf("status = %q, want failed", status)
	}
}

func TestPromotionKiCadSummaryStatusMalformedValueFails(t *testing.T) {
	status := promotionGateStatusForKiCadStage(StageResult{
		Name:   StageKiCadChecks,
		Status: StageStatusOK,
		Summary: map[string]any{
			"erc": 12,
		},
	})
	if status != PromotionGateStatusFailed {
		t.Fatalf("status = %q, want failed", status)
	}
}

func TestPromotionKiCadSummaryStatusNilPointerIsNotRun(t *testing.T) {
	var erc *checks.CheckResult
	status := promotionGateStatusForKiCadStage(StageResult{
		Name:   StageKiCadChecks,
		Status: StageStatusOK,
		Summary: map[string]any{
			"erc": erc,
		},
	})
	if status != PromotionGateStatusNotRun {
		t.Fatalf("status = %q, want not_run", status)
	}
}

func TestPromotionKiCadSummaryStatusNotRunOutranksWarn(t *testing.T) {
	status := promotionWorseGateStatus(PromotionGateStatusWarn, PromotionGateStatusNotRun)
	if status != PromotionGateStatusNotRun {
		t.Fatalf("status = %q, want not_run", status)
	}
}

func promotionGateByID(t *testing.T, report PromotionReport, id string) PromotionGate {
	t.Helper()
	for _, gate := range report.Gates {
		if gate.ID == id {
			return gate
		}
	}
	t.Fatalf("gate %q not found in %#v", id, report.Gates)
	return PromotionGate{}
}

func containsPromotionIssueCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}
