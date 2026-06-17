package designworkflow

import (
	"context"
	"fmt"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

type CreateOptions struct {
	OutputDir     string
	Overwrite     bool
	Seed          string
	SkipRouting   bool
	Components    ComponentSelectionOptions
	Placement     PlacementOptions
	Routing       RoutingOptions
	Validation    ValidationOptions
	KiCadChecks   KiCadCheckOptions
	BlockRegistry blocks.Registry
}

func Create(ctx context.Context, request Request, opts CreateOptions) WorkflowResult {
	if opts.BlockRegistry == nil {
		opts.BlockRegistry = blocks.NewBuiltinRegistry()
	}
	normalized := NormalizeRequest(request)
	plan := PlanBlocks(ctx, opts.BlockRegistry, normalized)
	stages := []StageResult{plan.Stage}
	if workflowStageBlocked(plan.Stage) {
		stages = append(stages, skippedWorkflowStages("block planning did not complete", StageComponentSelection, StageSchematic, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	componentSelections := SelectWorkflowComponents(ctx, opts.BlockRegistry, plan, opts.Components)
	if !workflowStageBlocked(componentSelections.Stage) {
		selectionApplyIssues := ApplyComponentSelectionsToPlan(&plan, opts.BlockRegistry, componentSelections.Selections)
		if len(selectionApplyIssues) != 0 {
			componentSelections.Stage.Issues = append(componentSelections.Stage.Issues, selectionApplyIssues...)
			componentSelections.Stage.Status = StageStatusForIssues(componentSelections.Stage.Issues)
		}
	}
	stages = append(stages, componentSelections.Stage)
	if workflowStageBlocked(componentSelections.Stage) {
		stages = append(stages, skippedWorkflowStages("component selection did not complete", StageSchematic, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	schematicStage := schematicStageFromPlan(plan)
	stages = append(stages, schematicStage)
	fragments := RealizePCBFragments(ctx, opts.BlockRegistry, plan)
	stages = append(stages, fragments.Stage)
	if workflowStageBlocked(fragments.Stage) {
		stages = append(stages, skippedWorkflowStages("PCB realization did not complete", StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	placed := PlaceFragments(ctx, normalized, fragments, opts.Placement)
	stages = append(stages, placed.Stage)
	if workflowStageBlocked(placed.Stage) {
		stages = append(stages, skippedWorkflowStages("placement did not complete", StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	routingOpts := opts.Routing
	routingOpts.Skip = routingOpts.Skip || opts.SkipRouting || normalized.Validation.SkipRouting
	routed := RoutePlacement(ctx, normalized, fragments, placed, routingOpts)
	stages = append(stages, routed.Stage)
	if workflowStageBlocked(routed.Stage) {
		stages = append(stages, skippedWorkflowStages("routing did not complete", StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	written := WriteProject(ctx, &normalized, &plan, &placed, &routed, ProjectWriteOptions{
		OutputDir: opts.OutputDir,
		Overwrite: opts.Overwrite,
		Seed:      opts.Seed,
	})
	stages = append(stages, written.Stage)
	if workflowStageBlocked(written.Stage) {
		stages = append(stages, skippedWorkflowStages("project write did not complete", StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	writerChecked := CheckWriterCorrectness(ctx, &written)
	stages = append(stages, writerChecked.Stage)
	if workflowStageBlocked(writerChecked.Stage) {
		stages = append(stages, skippedWorkflowStages("writer correctness check did not complete", StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	validated := ValidateProject(ctx, &normalized, &written, opts.Validation)
	stages = append(stages, validated.Stage)
	checked := RunKiCadChecks(ctx, &normalized, &written, opts.KiCadChecks)
	stages = append(stages, checked.Stage)
	return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
}

func workflowStageBlocked(stage StageResult) bool {
	return stage.Status == StageStatusBlocked || reports.HasBlockingIssue(stage.Issues)
}

func skippedWorkflowStages(reason string, names ...StageName) []StageResult {
	stages := make([]StageResult, 0, len(names))
	for _, name := range names {
		stages = append(stages, StageResult{Name: name, Status: StageStatusSkipped, Summary: map[string]any{"reason": reason}})
	}
	return stages
}

func schematicStageFromPlan(plan BlockPlanResult) StageResult {
	if plan.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(plan.Stage.Issues) {
		return StageResult{Name: StageSchematic, Status: StageStatusSkipped, Summary: map[string]any{"reason": "block planning did not complete"}}
	}
	stage := NewStageResult(StageSchematic, nil)
	stage.Summary = map[string]any{
		"operation_count":  len(plan.Output.Operations),
		"symbol_count":     countPlanOperations(plan.Output.Operations, transactions.OpAddSymbol),
		"connection_count": countPlanOperations(plan.Output.Operations, transactions.OpConnect),
	}
	return stage
}

func countPlanOperations(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func WorkflowIssues(result WorkflowResult) []reports.Issue {
	count := 0
	for _, stage := range result.Stages {
		count += len(stage.Issues)
	}
	issues := make([]reports.Issue, 0, count)
	for _, stage := range result.Stages {
		issues = append(issues, stage.Issues...)
	}
	return issues
}

func WorkflowArtifacts(result WorkflowResult) []reports.Artifact {
	count := 0
	for _, stage := range result.Stages {
		count += len(stage.Artifacts)
	}
	artifacts := make([]reports.Artifact, 0, count)
	for _, stage := range result.Stages {
		artifacts = append(artifacts, stage.Artifacts...)
	}
	return artifacts
}

func ParseRoutingMode(value string) (routing.RouteMode, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", nil
	case string(routing.ModeSingleLayer):
		return routing.ModeSingleLayer, nil
	case string(routing.ModeTwoLayer):
		return routing.ModeTwoLayer, nil
	case string(routing.ModeValidateOnly):
		return routing.ModeValidateOnly, nil
	default:
		return "", fmt.Errorf("unsupported routing mode %q", value)
	}
}
