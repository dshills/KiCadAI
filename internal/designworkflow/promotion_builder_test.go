package designworkflow

import (
	"testing"

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
