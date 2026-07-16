package designworkflow

import (
	"path/filepath"
	"strings"
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
	if len(report.NextActions) == 0 {
		t.Fatalf("expected next actions for blocked promotion report")
	}
	if !promotionReportHasNextAction(report, "stages", "resolve the stage blockers") {
		t.Fatalf("missing stage next action in %#v", report.NextActions)
	}
}

func TestBuildPromotionReportIncludesSchematicElectricalGate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "schematic_electrical",
		Request:           "bad schematic",
		Tier:              "generated",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageBlockPlanning, StageSchematic, StageSchematicElectrical},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "bad"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageSchematic, Status: StageStatusOK},
		{Name: StageSchematicElectrical, Status: StageStatusBlocked, Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Message:  "schematic electrical rule failed",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "schematic_electrical")
	if gate.Status != PromotionGateStatusFailed {
		t.Fatalf("schematic electrical gate = %#v", gate)
	}
	if report.AchievedReadiness != PromotionReadinessBlocked {
		t.Fatalf("readiness = %q", report.AchievedReadiness)
	}
}

func TestBuildPromotionReportSimulationGateOptionalWhenNotExpected(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "simulation_optional",
		Request:           "LED",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageBlockPlanning},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "led"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "simulation")
	if gate.Status != PromotionGateStatusNotRun || len(gate.RequiredFor) != 0 {
		t.Fatalf("simulation gate = %#v, want optional not_run", gate)
	}
}

func TestBuildPromotionReportSimulationGatePassesWithEvidence(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "simulation_pass",
		Request:           "Class AB headphone amplifier",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageBlockPlanning, StageSimulation},
		ExpectedArtifacts: []string{".kicadai/amplifier-simulation.json"},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "amp"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageSimulation, Status: StageStatusOK, Artifacts: []reports.Artifact{{
			Path: ".kicadai/amplifier-simulation.json",
			Kind: reports.ArtifactSimulationReport,
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "simulation")
	if gate.Status != PromotionGateStatusPass || len(gate.Artifacts) != 1 {
		t.Fatalf("simulation gate = %#v, want pass with artifact", gate)
	}
	if _, err := MarshalPromotionReportJSON(report); err != nil {
		t.Fatalf("promotion report validation failed: %v", err)
	}
}

func TestBuildPromotionReportSimulationGateBlocksCandidate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "simulation_fail",
		Request:           "Class AB headphone amplifier",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageBlockPlanning, StageSimulation},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "amp"}, AcceptanceConnectivity, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageSimulation, Status: StageStatusBlocked, Issues: []reports.Issue{{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Message:    "simulation ac_gain 0.5 is outside 1.8..2.2",
			Suggestion: "adjust feedback ratio",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "simulation")
	if gate.Status != PromotionGateStatusFailed {
		t.Fatalf("simulation gate = %#v, want failed", gate)
	}
	if report.AchievedReadiness != PromotionReadinessBlocked {
		t.Fatalf("readiness = %q, want blocked", report.AchievedReadiness)
	}
	if !promotionReportHasNextAction(report, "simulation", "resolve the simulation blockers") {
		t.Fatalf("missing simulation next action in %#v", report.NextActions)
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
	if !promotionReportHasNextAction(report, "artifacts", "missing required promotion artifacts") {
		t.Fatalf("missing artifact next action in %#v", report.NextActions)
	}
}

func TestPromotionBuilderMatchesAbsoluteArtifactUnderOutputDir(t *testing.T) {
	output := filepath.Join(t.TempDir(), "project")
	fixture := PromotionFixture{
		ID:                "artifact_paths",
		Request:           "artifact_paths.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceStructural,
		ExpectedArtifacts: []string{".kicadai/manifest.json"},
		ExpectedStages:    []StageName{StageBlockPlanning},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "artifact_paths", OutputDir: output}, AcceptanceStructural, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK, Artifacts: []reports.Artifact{{
			Path: filepath.Join(output, ".kicadai", "manifest.json"),
			Kind: reports.ArtifactValidationReport,
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "artifacts")
	if gate.Status != PromotionGateStatusPass {
		t.Fatalf("artifact gate = %#v, want pass", gate)
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

func TestPromotionBuilderAddsRepairGuidance(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "repair",
		Request:           "repair.json",
		Tier:              "smoke",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		Acceptance:        AcceptanceConnectivity,
		ExpectedStages:    []StageName{StageValidation},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "repair"}, AcceptanceConnectivity, []StageResult{
		{Name: StageValidation, Status: StageStatusBlocked, Issues: []reports.Issue{{
			Code:       reports.CodeDisconnectedPad,
			Severity:   reports.SeverityError,
			Message:    "pad is disconnected",
			Suggestion: "connect pad 1",
		}, {
			Code:     reports.CodeInvalidNetAssignment,
			Severity: reports.SeverityError,
			Message:  "net mismatch",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	if !promotionReportHasRepair(report, "connect pad 1") {
		t.Fatalf("missing explicit repair in %#v", report.Issues)
	}
	if !promotionReportHasRepair(report, "repair net-to-pad assignments") {
		t.Fatalf("missing default repair in %#v", report.Issues)
	}
}

func TestPromotionBuilderAddsSyntheticRepairGuidance(t *testing.T) {
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
	})
	report := BuildInternalPromotionReport(fixture, result)
	if !promotionReportHasRepair(report, "configure kicad-cli") {
		t.Fatalf("missing KiCad synthetic repair in %#v", report.Issues)
	}
	if !promotionReportHasNextAction(report, "kicad_checks", "run required KiCad ERC/DRC checks") {
		t.Fatalf("missing KiCad next action in %#v", report.NextActions)
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

func TestBuildPromotionReportStructuralSkippedKiCadAllowsCandidate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "structural_kicad_optional",
		Request:           "structural_kicad_optional.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceStructural,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "structural_kicad_optional"}, AcceptanceStructural, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageKiCadChecks, Status: StageStatusSkipped, Issues: []reports.Issue{{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Message:  "kicad-cli not configured",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusSkipped {
		t.Fatalf("kicad gate status = %q, want skipped", gate.Status)
	}
	if len(gate.RequiredFor) != 1 || gate.RequiredFor[0] != PromotionReadinessPass {
		t.Fatalf("kicad gate required_for = %#v, want pass-only", gate.RequiredFor)
	}
	if report.AchievedReadiness != PromotionReadinessCandidate {
		t.Fatalf("achieved readiness = %q, want candidate", report.AchievedReadiness)
	}
	if report.Status != PromotionStatusWarn {
		t.Fatalf("status = %q, want warn", report.Status)
	}
}

func TestBuildPromotionReportOptionalKiCadChecksSkippedAllowsCandidate(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "optional_kicad_erc_drc",
		Request:           "optional_kicad_erc_drc.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "optional_kicad_erc_drc"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{Name: StageKiCadChecks, Status: StageStatusSkipped, Issues: []reports.Issue{{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Message:  "kicad-cli not configured",
		}}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if len(gate.RequiredFor) != 1 || gate.RequiredFor[0] != PromotionReadinessPass {
		t.Fatalf("kicad gate required_for = %#v, want pass-only", gate.RequiredFor)
	}
	if report.AchievedReadiness != PromotionReadinessCandidate {
		t.Fatalf("achieved readiness = %q, want candidate", report.AchievedReadiness)
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

func TestBuildPromotionReportCleanKiCadEvidenceSatisfiesDeferredConnectivityWarnings(t *testing.T) {
	fixture := PromotionFixture{
		ID: "deferred_connectivity", Request: "deferred_connectivity.json", Tier: "pass",
		DeclaredReadiness: PromotionReadinessPass, Acceptance: AcceptanceERCDRC,
		RequireERC: true, RequireDRC: true, ExpectedStages: []StageName{StageValidation, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "deferred_connectivity"}, AcceptanceERCDRC, []StageResult{
		{Name: StageValidation, Status: StageStatusWarning, Issues: []reports.Issue{
			{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityInfo, Message: "erc_validation is available through the `check` command and is not run by default during structural evaluation"},
			{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityInfo, Message: "drc_validation is available through the `check` command and is not run by default during structural evaluation"},
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "zone has no fill evidence; run KiCad refill/DRC for authoritative zone connectivity"},
		}},
		{Name: StageKiCadChecks, Status: StageStatusOK, Summary: map[string]any{
			"erc": checks.CheckResult{Kind: checks.CheckKindERC, Status: checks.CheckStatusPass},
			"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusPass},
		}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "connectivity")
	if gate.Status != PromotionGateStatusPass || len(gate.IssueCodes) != 0 {
		t.Fatalf("connectivity gate = %#v, want evidence-backed pass", gate)
	}
	if report.AchievedReadiness != PromotionReadinessPass || report.Status != PromotionStatusPass {
		t.Fatalf("promotion = %q/%q, want pass/pass", report.AchievedReadiness, report.Status)
	}
}

func TestBuildPromotionReportDoesNotSatisfyZoneWarningWithoutCleanDRC(t *testing.T) {
	fixture := PromotionFixture{
		ID: "deferred_zone", Request: "deferred_zone.json", Tier: "pass",
		DeclaredReadiness: PromotionReadinessPass, Acceptance: AcceptanceERCDRC,
		RequireDRC: true, ExpectedStages: []StageName{StageValidation, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "deferred_zone"}, AcceptanceERCDRC, []StageResult{
		{Name: StageValidation, Status: StageStatusWarning, Issues: []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning,
			Message: "zone has no fill evidence; run KiCad refill/DRC for authoritative zone connectivity",
		}}},
		{Name: StageKiCadChecks, Status: StageStatusWarning, Summary: map[string]any{
			"drc": checks.CheckResult{Kind: checks.CheckKindDRC, Status: checks.CheckStatusFail, Findings: []checks.CheckFinding{{Severity: "warning"}}},
		}},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "connectivity")
	if gate.Status != PromotionGateStatusWarn || len(gate.IssueCodes) != 1 {
		t.Fatalf("connectivity gate = %#v, want unresolved warning", gate)
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

func TestBuildPromotionReportKiCadWarningOnlyFindingsWarn(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "warning_kicad",
		Request:           "warning_kicad.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "warning_kicad"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusWarning,
			Summary: map[string]any{
				"drc": checks.CheckResult{
					Kind:   checks.CheckKindDRC,
					Status: checks.CheckStatusFail,
					Findings: []checks.CheckFinding{{
						Severity: "warning",
						Message:  "silkscreen clearance",
					}},
				},
			},
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Message:  "silkscreen clearance",
			}},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusWarn {
		t.Fatalf("kicad gate status = %q, want warn", gate.Status)
	}
	if report.AchievedReadiness != PromotionReadinessCandidate {
		t.Fatalf("achieved readiness = %q, want candidate", report.AchievedReadiness)
	}
}

func TestBuildPromotionReportKiCadZeroFindingToolErrorWarns(t *testing.T) {
	fixture := PromotionFixture{
		ID:                "unstable_drc",
		Request:           "unstable_drc.json",
		Tier:              "candidate",
		DeclaredReadiness: PromotionReadinessCandidate,
		Acceptance:        AcceptanceERCDRC,
		RequireDRC:        true,
		ExpectedStages:    []StageName{StageBlockPlanning, StageKiCadChecks},
	}
	result := BuildWorkflowResult(ProjectSummary{Name: "unstable_drc"}, AcceptanceERCDRC, []StageResult{
		{Name: StageBlockPlanning, Status: StageStatusOK},
		{
			Name:   StageKiCadChecks,
			Status: StageStatusWarning,
			Summary: map[string]any{
				"drc": checks.CheckResult{
					Kind:          checks.CheckKindDRC,
					Status:        checks.CheckStatusError,
					ExitCode:      -1,
					ToolErrorKind: checks.ToolErrorNoOutputCrash,
				},
			},
			Issues: []reports.Issue{{
				Code:     reports.CodeKiCadCLIFailed,
				Severity: reports.SeverityWarning,
				Message:  "drc check failed with exit code -1",
			}},
		},
	})
	report := BuildInternalPromotionReport(fixture, result)
	gate := promotionGateByID(t, report, "kicad_checks")
	if gate.Status != PromotionGateStatusWarn {
		t.Fatalf("kicad gate status = %q, want warn", gate.Status)
	}
	if report.AchievedReadiness != PromotionReadinessCandidate {
		t.Fatalf("achieved readiness = %q, want candidate", report.AchievedReadiness)
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

func promotionReportHasRepair(report PromotionReport, text string) bool {
	for _, issue := range report.Issues {
		if strings.Contains(issue.Repair, text) {
			return true
		}
	}
	return false
}

func promotionReportHasNextAction(report PromotionReport, gate string, text string) bool {
	for _, action := range report.NextActions {
		if action.Gate == gate && strings.Contains(action.Action, text) {
			return true
		}
	}
	return false
}
