package designworkflow

import (
	"context"
	"path/filepath"

	"kicadai/internal/inspect"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

func createExplicitCircuit(ctx context.Context, request Request, opts CreateOptions) WorkflowResult {
	project := ProjectSummary{Name: request.Name, OutputDir: opts.OutputDir}
	issues := ValidateRequest(request)
	planning := NewStageResult(StageBlockPlanning, issues)
	planning.Summary = map[string]any{"mode": "explicit_circuit", "component_count": explicitComponentCount(request)}
	stages := []StageResult{planning}
	if workflowStageBlocked(planning) {
		stages = append(stages, skippedWorkflowStages("explicit circuit validation did not complete", StageComponentSelection, StageSchematic, StageSchematicElectrical, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	var selectionIssues []reports.Issue
	if opts.LibraryIndex == nil {
		selectionIssues = append(selectionIssues, reports.Issue{
			Code: reports.CodeInvalidArgument, Severity: reports.SeverityError,
			Path: "library_index", Message: "explicit circuit workflow requires a resolved symbol and footprint library index",
		})
	}
	selection := NewStageResult(StageComponentSelection, selectionIssues)
	selection.Summary = map[string]any{
		"mode":            "catalog_resolved",
		"component_count": len(request.ExplicitCircuit.Components),
		"resolution_hash": request.ExplicitCircuit.ResolutionHash,
		"catalog_hash":    request.ExplicitCircuit.CatalogHash,
	}
	stages = append(stages, selection)
	if workflowStageBlocked(selection) {
		stages = append(stages, skippedWorkflowStages("explicit component library resolution did not complete", StageSchematic, StageSchematicElectrical, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	schematicTx, txIssues := explicitSchematicTransaction(request, opts.LibraryIndex)
	schematic := NewStageResult(StageSchematic, txIssues)
	schematic.Summary = map[string]any{"operation_count": len(schematicTx.Operations), "mode": "schematic_ir"}
	stages = append(stages, schematic)
	if workflowStageBlocked(schematic) {
		stages = append(stages, skippedWorkflowStages("explicit schematic generation did not complete", StageSchematicElectrical, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	electrical := schematicElectricalStageFromTransaction(schematicTx)
	stages = append(stages, electrical)
	if workflowStageBlocked(electrical) {
		stages = append(stages, skippedWorkflowStages("schematic electrical rules did not pass", StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	pcbRealization := NewStageResult(StagePCBRealization, nil)
	pcbRealization.Summary = map[string]any{"footprint_count": len(request.ExplicitCircuit.Components), "net_count": len(request.ExplicitCircuit.Nets)}
	stages = append(stages, pcbRealization)
	placementOpts := opts.Placement
	placementOpts.LibraryIndex = opts.LibraryIndex
	placed := PlaceExplicitCircuit(ctx, request, placementOpts)
	stages = append(stages, placed.Stage)
	if workflowStageBlocked(placed.Stage) {
		stages = append(stages, skippedWorkflowStages("explicit placement did not complete", StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	routingOpts := opts.Routing
	routingOpts.Skip = routingOpts.Skip || opts.SkipRouting || request.Validation.SkipRouting
	routed := RouteExplicitCircuit(ctx, request, placed, routingOpts)
	stages = append(stages, routed.Stage)
	if workflowStageBlocked(routed.Stage) {
		stages = append(stages, skippedWorkflowStages("explicit routing did not complete", StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	tx, projectTxIssues := explicitCircuitTransaction(request, schematicTx, placed, routed, opts.Overwrite)
	if reports.HasBlockingIssue(projectTxIssues) {
		writeStage := NewStageResult(StageProjectWrite, projectTxIssues)
		stages = append(stages, writeStage)
		stages = append(stages, skippedWorkflowStages("explicit project transaction did not complete", StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	written := writeExplicitCircuitProject(ctx, request, tx, opts)
	stages = append(stages, written.Stage)
	if workflowStageBlocked(written.Stage) {
		stages = append(stages, skippedWorkflowStages("project write did not complete", StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	writerChecked := CheckWriterCorrectnessWithOptions(ctx, &written, opts.Writer)
	stages = append(stages, writerChecked.Stage)
	if workflowStageBlocked(writerChecked.Stage) {
		stages = append(stages, skippedWorkflowStages("writer correctness check did not complete", StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	validated := ValidateProject(ctx, &request, &written, opts.Validation)
	stages = append(stages, validated.Stage)
	kicadOpts := opts.KiCadChecks
	kicadOpts.RequireERC = kicadOpts.RequireERC || request.Validation.RequireERC
	kicadOpts.RequireDRC = kicadOpts.RequireDRC || request.Validation.RequireDRC
	checked := RunKiCadChecks(ctx, &request, &written, kicadOpts)
	stages = append(stages, checked.Stage)
	return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
}

func explicitComponentCount(request Request) int {
	if request.ExplicitCircuit == nil {
		return 0
	}
	return len(request.ExplicitCircuit.Components)
}

func explicitSchematicTransaction(request Request, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	if request.ExplicitCircuit == nil {
		return transactions.Transaction{}, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit", Message: "explicit circuit is required"}}
	}
	var tx transactions.Transaction
	var issues []reports.Issue
	if index != nil {
		tx, issues = schematicir.ToTransactionWithLibraryIndex(request.ExplicitCircuit.Schematic, index)
	} else {
		tx, issues = schematicir.ToTransaction(request.ExplicitCircuit.Schematic)
	}
	if reports.HasBlockingIssue(issues) {
		return tx, issues
	}
	return tx, issues
}

func explicitCircuitTransaction(request Request, schematicTx transactions.Transaction, placed PlacementStageResult, routed RoutingStageResult, overwrite bool) (transactions.Transaction, []reports.Issue) {
	tx := schematicTx
	var issues []reports.Issue
	boardOps, boardIssues := boardOperations(&request)
	issues = append(issues, boardIssues...)
	tx.Operations = append(tx.Operations, boardOps...)
	placementOps, placementIssues := explicitPlacementWriteOperations(placed.Result.Operations)
	issues = append(issues, placementIssues...)
	tx.Operations = append(tx.Operations, placementOps...)
	tx.Operations = append(tx.Operations, routed.Operations...)
	zoneOps, zoneIssues := explicitZoneOperations(request)
	issues = append(issues, zoneIssues...)
	tx.Operations = append(tx.Operations, zoneOps...)
	appendExplicitOperation(&tx, transactions.OpWriteProject, transactions.WriteProjectOperation{
		Op: transactions.OpWriteProject, Overwrite: overwrite,
		RequireSchematicReadability: request.ExplicitCircuit.Schematic.Policy.Acceptance == schematicir.AcceptanceReadable,
	}, &issues)
	return tx, issues
}

func writeExplicitCircuitProject(ctx context.Context, request Request, tx transactions.Transaction, opts CreateOptions) ProjectWriteResult {
	validation := transactions.Validate(tx)
	issues := append([]reports.Issue(nil), validation.Issues...)
	if err := ctx.Err(); err != nil {
		return canceledProjectWriteResult(err, tx, validation, transactions.ApplyResult{}, issues)
	}
	if opts.OutputDir == "" {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked, Path: "output", Message: "output directory is required"})
	}
	if reports.HasBlockingIssue(issues) {
		return ProjectWriteResult{Transaction: tx, Validation: validation, Stage: NewStageResult(StageProjectWrite, issues)}
	}
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked, Path: "output", Message: err.Error()})
		return ProjectWriteResult{Transaction: tx, Validation: validation, Stage: NewStageResult(StageProjectWrite, issues)}
	}
	applyResult := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir: outputDir, Overwrite: opts.Overwrite, Seed: opts.Seed, LibraryIndex: opts.LibraryIndex,
		SuppressPinmapWarnings: opts.LibraryIndex != nil, SuppressExplicitPinSymbolErrors: opts.LibraryIndex != nil,
	})
	issues = append(issues, applyResult.Issues...)
	var inspection inspect.ProjectSummary
	if !reports.HasBlockingIssue(applyResult.Issues) {
		inspection, err = inspect.Project(outputDir)
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "inspect", Message: err.Error()})
		} else {
			issues = append(issues, inspection.Issues...)
		}
	}
	stage := NewStageResult(StageProjectWrite, issues)
	stage.Artifacts = append([]reports.Artifact(nil), applyResult.Artifacts...)
	stage.Summary = map[string]any{"operation_count": len(tx.Operations), "artifact_count": len(applyResult.Artifacts), "mode": "explicit_circuit"}
	return ProjectWriteResult{Transaction: tx, Validation: validation, ApplyResult: applyResult, Inspection: inspection, Stage: stage}
}

func appendExplicitOperation(tx *transactions.Transaction, kind transactions.OperationKind, payload any, issues *[]reports.Issue) {
	op, err := workflowOperation(kind, payload)
	if err != nil {
		*issues = append(*issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.transaction", Message: err.Error()})
		return
	}
	tx.Operations = append(tx.Operations, op)
}
