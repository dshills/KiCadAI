package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

func TestCreateWritesWorkflowResult(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	output := filepath.Join(t.TempDir(), "status_board")

	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	if result.Project.OutputDir != output {
		t.Fatalf("project = %#v", result.Project)
	}
	if len(result.Stages) == 0 {
		t.Fatalf("stages missing")
	}
	if result.Acceptance.Achieved == "" {
		t.Fatalf("acceptance = %#v feedback = %#v", result.Acceptance, result.Feedback)
	}
	if !hasStage(result, StageWriterCorrect) {
		t.Fatalf("writer correctness stage missing: %#v", result.Stages)
	}
	componentStage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if got := componentStage.Summary["selection_count"]; got != 2 {
		t.Fatalf("component selection count = %#v, want 2", got)
	}
}

func TestCreateStructuralRequestSkipsFabricationReadiness(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "structural_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "structural_board")})
	if hasStage(result, StageFabricationReady) {
		t.Fatalf("fabrication readiness stage should not run for structural request: %#v", result.Stages)
	}
}

func TestFabricationReadinessStageBlocksMissingPackageEvidence(t *testing.T) {
	request := Request{Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate}}
	written := ProjectWriteResult{Inspection: inspect.ProjectSummary{Root: t.TempDir()}}
	stage := FabricationReadinessStage(context.Background(), &request, &written)
	if stage.Name != StageFabricationReady {
		t.Fatalf("stage name = %q", stage.Name)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v, want blocked", stage)
	}
	if !hasIssueCode(stage.Issues, reports.CodeValidationFailed) {
		t.Fatalf("expected readiness issue in %#v", stage.Issues)
	}
	if stage.Summary["dry_run"] != true {
		t.Fatalf("summary = %#v, want dry run", stage.Summary)
	}
}

func TestFabricationReadinessStageSummarizesPhysicalRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeWorkflowTestPCB(t, filepath.Join(root, "demo.kicad_pcb"))
	request := Request{Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate}}
	written := ProjectWriteResult{Inspection: inspect.ProjectSummary{Root: root}}

	stage := FabricationReadinessStage(context.Background(), &request, &written)

	physical, ok := stage.Summary["physical_rules"].(map[string]any)
	if !ok {
		t.Fatalf("physical_rules summary missing or wrong type: %#v", stage.Summary)
	}
	if physical["status"] == "" {
		t.Fatalf("physical_rules status missing: %#v", physical)
	}
	if physical["report_path"] != "fabrication/physical-rules.json" {
		t.Fatalf("physical_rules report_path = %#v", physical["report_path"])
	}
	if _, ok := physical["blocker_count"].(int); !ok {
		t.Fatalf("physical_rules blocker_count missing: %#v", physical)
	}
}

func TestWorkflowIssueAndArtifactCollectors(t *testing.T) {
	result := WorkflowResult{Stages: []StageResult{{
		Issues:    []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "warn"}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: "out/demo.kicad_pro"}},
	}}}
	if len(WorkflowIssues(result)) != 1 || len(WorkflowArtifacts(result)) != 1 {
		t.Fatalf("collectors failed")
	}
}

func writeWorkflowTestPCB(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	board := pcbfiles.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "kicadai-test",
		GeneratorVersion: "phase7",
		General:          pcbfiles.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcbfiles.DefaultTwoLayerStack(),
		Setup:            pcbfiles.DefaultSetup(),
		Drawings: []pcbfiles.Drawing{{
			UUID:  kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
			Layer: kicadfiles.LayerEdge,
			Rect: &pcbfiles.RectDrawing{
				Start: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(0)},
				End:   kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(15)},
				Width: kicadfiles.MM(0.1),
			},
		}},
	}
	if err := pcbfiles.Write(file, board); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func hasStage(result WorkflowResult, name StageName) bool {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return true
		}
	}
	return false
}

func stageByName(result WorkflowResult, name StageName) (StageResult, bool) {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func hasIssueCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestCreateShortCircuitsAfterPlanFailure(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "bad",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "missing", BlockID: "does_not_exist"}},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "bad")})
	if result.Stages[0].Status != StageStatusBlocked {
		t.Fatalf("plan stage = %#v", result.Stages[0])
	}
	for _, stage := range result.Stages[2:] {
		if stage.Status != StageStatusSkipped {
			t.Fatalf("stage %s = %#v, want skipped", stage.Name, stage)
		}
	}
}

func TestCreateComponentSelectionFailureBlocksBeforeWrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "blocked")
	request := Request{
		Version:    RequestVersion,
		Name:       "blocked",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Components: ComponentPolicySpec{CatalogDir: t.TempDir()},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("component stage = %#v, want blocked", stage)
	}
	projectWrite, ok := stageByName(result, StageProjectWrite)
	if !ok || projectWrite.Status != StageStatusSkipped {
		t.Fatalf("project write stage = %#v ok=%v, want skipped", projectWrite, ok)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output dir stat err = %v, want not exist", err)
	}
}

func TestCreateDraftComponentPolicyAllowsPlaceholder(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "draft_opamp",
		Board:   BoardSpec{WidthMM: 60, HeightMM: 35, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "gain", BlockID: "opamp_gain_stage"}},
		Components: ComponentPolicySpec{
			Acceptance: components.AcceptanceDraft,
		},
		Validation: ValidationSpec{Acceptance: AcceptanceDraft, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "draft")})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("draft component stage blocked: %#v", stage)
	}
	if !hasIssueCode(stage.Issues, components.CodeComponentUnsafe) {
		t.Fatalf("expected placeholder warning in %#v", stage.Issues)
	}
}

func TestCreateConnectivityRejectsPlaceholderActiveComponent(t *testing.T) {
	output := filepath.Join(t.TempDir(), "connectivity")
	request := Request{
		Version:    RequestVersion,
		Name:       "connectivity_opamp",
		Board:      BoardSpec{WidthMM: 60, HeightMM: 35, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "gain", BlockID: "opamp_gain_stage"}},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("component stage = %#v, want blocked", stage)
	}
	if !hasIssueCode(stage.Issues, components.CodeComponentUnsafe) {
		t.Fatalf("expected unsafe component issue in %#v", stage.Issues)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output dir stat err = %v, want not exist", err)
	}
}

func TestCreateComponentSelectionSummaryCarriesMetadata(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "metadata",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "metadata")})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	selected, ok := stage.Summary["selected_components"].([]map[string]any)
	if !ok {
		t.Fatalf("selected component summary type = %T", stage.Summary["selected_components"])
	}
	if len(selected) != 2 {
		t.Fatalf("selected components = %#v", selected)
	}
	if selected[0]["component_id"] == "" || selected[0]["footprint_id"] == "" {
		t.Fatalf("selected component metadata incomplete: %#v", selected)
	}
	if _, ok := selected[0]["pinmap_checked"].(bool); !ok {
		t.Fatalf("selected component evidence missing pinmap flag: %#v", selected[0])
	}
	if _, ok := selected[0]["rejected_count"].(int); !ok {
		t.Fatalf("selected component evidence missing rejected count: %#v", selected[0])
	}
}
