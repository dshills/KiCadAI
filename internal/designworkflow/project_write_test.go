package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestWriteProjectGeneratesInspectablePCBProject(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	routed := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Skip: true})
	output := filepath.Join(t.TempDir(), "status_board")

	result := WriteProject(context.Background(), &request, &plan, &placed, &routed, ProjectWriteOptions{OutputDir: output})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("write issues = %#v", result.Stage.Issues)
	}
	for _, suffix := range []string{".kicad_pro", ".kicad_sch", ".kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, "status_board"+suffix)); err != nil {
			t.Fatalf("missing generated %s: %v", suffix, err)
		}
	}
	if result.Inspection.PCB == nil || !result.Inspection.PCB.HasBoardOutline {
		t.Fatalf("pcb inspection = %#v", result.Inspection.PCB)
	}
	if result.Inspection.PCB.FootprintCount != 2 {
		t.Fatalf("pcb footprint count = %d", result.Inspection.PCB.FootprintCount)
	}
	netAssignment, ok := result.Stage.Summary["net_assignment"].(GeneratedNetAssignmentSummary)
	if !ok {
		t.Fatalf("project_write net assignment summary missing: %#v", result.Stage.Summary)
	}
	if netAssignment.NetCount == 0 || netAssignment.AssignedPads == 0 || netAssignment.AssignedCopperObjects == 0 {
		t.Fatalf("net assignment summary = %#v, want assigned nets, pads, and copper", netAssignment)
	}
}

func TestWriteProjectPersistsFabricationMetadata(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "fab_metadata",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Fabrication: FabricationMetadataSpec{
			BoardFinish:      "ENIG",
			FabricationNotes: "Lead-free assembly.",
		},
		Blocks: []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	routed := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Skip: true})
	output := filepath.Join(t.TempDir(), request.Name)

	result := WriteProject(context.Background(), &request, &plan, &placed, &routed, ProjectWriteOptions{OutputDir: output})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("write issues = %#v", result.Stage.Issues)
	}
	written, err := project.ReadFile(filepath.Join(output, request.Name+".kicad_pro"))
	if err != nil {
		t.Fatal(err)
	}
	if written.TextVariables["board_finish"] != "ENIG" || written.TextVariables["fabrication_notes"] != "Lead-free assembly." {
		t.Fatalf("text variables = %#v", written.TextVariables)
	}
}

func TestApplyFabricationMetadataPreservesExistingTextVariables(t *testing.T) {
	create, err := workflowOperation(transactions.OpCreateProject, transactions.CreateProjectOperation{
		Op:            transactions.OpCreateProject,
		Name:          "metadata_merge",
		TextVariables: map[string]string{"assembly_variant": "prototype"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondCreate, err := workflowOperation(transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: "metadata_merge_secondary"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := applyFabricationMetadata(
		transactions.Transaction{Operations: []transactions.Operation{create, secondCreate}},
		FabricationMetadataSpec{BoardFinish: "ENIG"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, operation := range tx.Operations {
		var payload transactions.CreateProjectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.TextVariables["board_finish"] != "ENIG" || index == 0 && payload.TextVariables["assembly_variant"] != "prototype" {
			t.Fatalf("operation %d text variables = %#v, want fabrication metadata on every project and existing variables preserved", index, payload.TextVariables)
		}
	}
}

func TestProjectTransactionIncludesOutlinePlacementAndRoutesBeforeWrite(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	routed := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Skip: true})

	tx, issues := ProjectTransaction(&request, &plan, &placed, &routed, false)
	if len(issues) != 0 {
		t.Fatalf("transaction issues = %#v", issues)
	}
	writeIndex := slices.IndexFunc(tx.Operations, func(operation transactions.Operation) bool {
		return operation.Op == transactions.OpWriteProject
	})
	if writeIndex < 0 {
		t.Fatalf("transaction missing write op: %#v", tx.Operations)
	}
	if countTransactionOps(tx.Operations[:writeIndex], transactions.OpSetBoardOutline) != 1 {
		t.Fatalf("transaction missing board outline before write: %#v", tx.Operations)
	}
	if countTransactionOps(tx.Operations[:writeIndex], transactions.OpPlaceFootprint) != 2 {
		t.Fatalf("transaction has unexpected placement ops: %#v", tx.Operations)
	}
	if countTransactionOps(tx.Operations[:writeIndex], transactions.OpRoute) == 0 {
		t.Fatalf("transaction missing local route before write: %#v", tx.Operations)
	}
}

func TestWriteProjectSkipsAfterPlacementFailure(t *testing.T) {
	request := validRequest()
	plan := BlockPlanResult{}
	placed := PlacementStageResult{
		Stage: NewStageResult(StagePlacement, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "bad"}}),
	}
	routed := RoutingStageResult{}
	result := WriteProject(context.Background(), &request, &plan, &placed, &routed, ProjectWriteOptions{OutputDir: t.TempDir()})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestWriteProjectSkipsAfterRoutingFailure(t *testing.T) {
	request := validRequest()
	plan := BlockPlanResult{}
	placed := PlacementStageResult{
		Stage: NewStageResult(StagePlacement, nil),
	}
	routed := RoutingStageResult{
		Stage: NewStageResult(StageRouting, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "bad"}}),
	}
	result := WriteProject(context.Background(), &request, &plan, &placed, &routed, ProjectWriteOptions{OutputDir: t.TempDir()})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestProjectTransactionRejectsInvalidBoardDimensions(t *testing.T) {
	request := validRequest()
	request.Board.WidthMM = 0
	plan := BlockPlanResult{}
	placed := PlacementStageResult{}
	routed := RoutingStageResult{}

	_, issues := ProjectTransaction(&request, &plan, &placed, &routed, false)
	assertIssuePath(t, issues, "board")
}
