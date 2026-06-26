package rationale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentdraft"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

func TestBuildFromPlanMinimalDeterministic(t *testing.T) {
	request := intentplanner.Request{
		Version:    intentplanner.RequestVersion,
		Name:       "sensor",
		Kind:       intentplanner.IntentSensorNode,
		Acceptance: designworkflow.AcceptanceConnectivity,
		Functions:  []intentplanner.FunctionIntent{{Kind: "sensor", Family: "i2c_sensor"}},
		Interfaces: []intentplanner.InterfaceIntent{{Kind: "i2c"}},
	}
	plan := intentplanner.Plan(request)
	report := BuildFromPlan(plan, SourceSummary{Mode: "request", Path: "request.json"})
	if report.Schema != Schema {
		t.Fatalf("schema = %q", report.Schema)
	}
	if report.Status == "" {
		t.Fatalf("status was empty")
	}
	first, err := MarshalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("marshal output is not deterministic")
	}
	if !strings.Contains(string(first), `"schema": "kicadai.design.rationale.v1"`) {
		t.Fatalf("missing schema in JSON: %s", first)
	}
}

func TestBuildFromDraftAndPlanBlockingClarification(t *testing.T) {
	draft := intentdraft.Draft("make a battery sensor board", intentdraft.Options{})
	plan := intentplanner.Plan(draft.Request)
	report := BuildFromDraftAndPlan(draft, plan, SourceSummary{Mode: "text"})
	if report.Status != StatusNeedsClarification {
		t.Fatalf("status = %q, want %q", report.Status, StatusNeedsClarification)
	}
	if len(report.Clarifications) == 0 {
		t.Fatalf("expected clarifications")
	}
	found := false
	for _, limit := range report.KnownLimits {
		if limit.Category == "validation_blocked" || limit.Category == "unsupported_intent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected clarification known limit, got %#v", report.KnownLimits)
	}
}

func TestBuildWithWorkflowSummary(t *testing.T) {
	issue := reports.Issue{
		Code:       reports.CodeMissingFootprint,
		Severity:   reports.SeverityError,
		Path:       "blocks.u1",
		Message:    "missing footprint",
		Suggestion: "assign a verified footprint",
	}
	workflow := designworkflow.BuildWorkflowResult(
		designworkflow.ProjectSummary{Name: "demo"},
		designworkflow.AcceptanceERCDRC,
		[]designworkflow.StageResult{
			designworkflow.NewStageResult(designworkflow.StageSchematic, nil),
			designworkflow.NewStageResult(designworkflow.StageComponentSelection, []reports.Issue{issue}),
		},
	)
	report := Build(BuildOptions{
		Source:   SourceSummary{Mode: "request"},
		Workflow: &workflow,
	})
	if report.Status != StatusBlocked {
		t.Fatalf("status = %q", report.Status)
	}
	if report.Validation.BlockingCount == 0 {
		t.Fatalf("expected blocking count")
	}
	if len(report.NextActions) == 0 {
		t.Fatalf("expected next actions")
	}
}

func TestBuildWithWorkflowProcurementEvidence(t *testing.T) {
	workflow := designworkflow.BuildWorkflowResult(
		designworkflow.ProjectSummary{Name: "demo"},
		designworkflow.AcceptanceConnectivity,
		[]designworkflow.StageResult{{
			Name:   designworkflow.StageComponentSelection,
			Status: designworkflow.StageStatusOK,
			Summary: map[string]any{
				"selected_components": []map[string]any{{
					"role":         "regulator",
					"component_id": "regulator.linear.ap2112k_3v3.sot23_5",
					"procurement": &components.ProcurementEvidence{
						Manufacturer:           "Diodes Incorporated",
						MPN:                    "AP2112K-3.3",
						SourceID:               "curated_seed_procurement",
						LifecycleStatus:        components.LifecycleActive,
						LifecycleSourceDate:    "2026-06-26",
						AvailabilityStatus:     components.AvailabilityNotChecked,
						AvailabilitySourceDate: "2026-06-26",
						Outcome:                "accepted",
					},
				}},
			},
		}},
	)
	report := Build(BuildOptions{Source: SourceSummary{Mode: "request"}, Workflow: &workflow})
	found := false
	for _, evidence := range report.Evidence {
		if evidence.Kind == "component_evidence" && strings.Contains(evidence.Summary, "lifecycle=active") && strings.Contains(evidence.Summary, "source=curated_seed_procurement") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("procurement evidence missing from %#v", report.Evidence)
	}
}

func TestBuildFromPlanMapsSynthesisTrace(t *testing.T) {
	plan := intentplanner.Plan(intentplanner.Request{
		Version: intentplanner.RequestVersion,
		Name:    "external_clock",
		Kind:    intentplanner.IntentMCUMinimal,
		Power:   intentplanner.PowerIntent{Inputs: []intentplanner.PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []intentplanner.FunctionIntent{
			{Kind: "mcu", Params: map[string]any{"supply_voltage": "5V"}},
			{Kind: "clock", Family: "crystal_oscillator", Params: map[string]any{"load_cap_pf": 18}},
		},
	})
	report := BuildFromPlan(plan, SourceSummary{Mode: "request"})
	if !hasDecisionType(report, "known_gap") {
		t.Fatalf("missing synthesis known-gap decision: %#v", report.Decisions)
	}
	if !hasEvidenceSummary(report, "crystal_load_cap") {
		t.Fatalf("missing synthesis calculation evidence: %#v", report.Evidence)
	}
	if !hasKnownLimitCategory(report, "unsupported_intent") {
		t.Fatalf("missing synthesis known limit: %#v", report.KnownLimits)
	}
}

func TestBuildFromPlanReportsAppliedCalculationDetails(t *testing.T) {
	plan := intentplanner.Plan(intentplanner.Request{
		Version: intentplanner.RequestVersion,
		Name:    "led_calc",
		Kind:    intentplanner.IntentBreakout,
		Power:   intentplanner.PowerIntent{Inputs: []intentplanner.PowerInputIntent{{Kind: "external", Voltage: "5V"}}},
		Functions: []intentplanner.FunctionIntent{
			{Kind: "indicator", Params: map[string]any{"supply_voltage": "5V", "led_forward_voltage": "2V", "led_current": "10mA"}},
		},
	})
	report := BuildFromPlan(plan, SourceSummary{Mode: "request"})
	if !hasEvidenceSummary(report, "led_resistor applied") {
		t.Fatalf("missing applied calculation summary: %#v", report.Evidence)
	}
	if !hasEvidenceNote(report, "applied blocks.indicator.params.resistor_value=300") {
		t.Fatalf("missing applied calculation note: %#v", report.Evidence)
	}
	if !hasEvidenceNote(report, "requires resistor power") {
		t.Fatalf("missing calculated requirement note: %#v", report.Evidence)
	}
}

func TestLoadFromTarget(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, MetadataDirName)
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	request := intentplanner.Request{Version: intentplanner.RequestVersion, Name: "demo", Kind: intentplanner.IntentBreakout}
	plan := intentplanner.Plan(request)
	writeJSON(t, filepath.Join(meta, "intent-plan.json"), plan)
	result := LoadFromTarget(dir)
	if reports.HasBlockingIssue(result.Issues) {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if result.Report.Source.Mode != "target" {
		t.Fatalf("source mode = %q", result.Report.Source.Mode)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasDecisionType(report Report, kind string) bool {
	for _, decision := range report.Decisions {
		if decision.Type == kind {
			return true
		}
	}
	return false
}

func hasEvidenceSummary(report Report, text string) bool {
	for _, evidence := range report.Evidence {
		if strings.Contains(evidence.Summary, text) {
			return true
		}
	}
	return false
}

func hasEvidenceNote(report Report, text string) bool {
	for _, evidence := range report.Evidence {
		for _, note := range evidence.Notes {
			if strings.Contains(note, text) {
				return true
			}
		}
	}
	return false
}

func hasKnownLimitCategory(report Report, category string) bool {
	for _, limit := range report.KnownLimits {
		if limit.Category == category {
			return true
		}
	}
	return false
}
