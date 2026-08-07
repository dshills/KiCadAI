package designworkflow

import (
	"context"
	"encoding/json"
	"path/filepath"

	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
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
		stages = appendSkippedCreateStages(stages, StageBlockPlanning, "explicit circuit validation did not complete")
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
		stages = appendSkippedCreateStages(stages, StageComponentSelection, "explicit component library resolution did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	schematicTx, txIssues := explicitSchematicTransaction(request, opts.LibraryIndex)
	schematic := NewStageResult(StageSchematic, txIssues)
	schematic.Summary = map[string]any{"operation_count": len(schematicTx.Operations), "mode": "schematic_ir"}
	stages = append(stages, schematic)
	if workflowStageBlocked(schematic) {
		stages = appendSkippedCreateStages(stages, StageSchematic, "explicit schematic generation did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	electrical := schematicElectricalStageFromTransaction(schematicTx)
	stages = append(stages, electrical)
	if workflowStageBlocked(electrical) {
		stages = appendSkippedCreateStages(stages, StageSchematicElectrical, "schematic electrical rules did not pass")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	pcbRealization := NewStageResult(StagePCBRealization, nil)
	pcbRealization.Summary = map[string]any{"footprint_count": len(request.ExplicitCircuit.Components), "net_count": len(request.ExplicitCircuit.Nets)}
	stages = append(stages, pcbRealization)
	placementOpts := opts.Placement
	placementOpts.LibraryIndex = opts.LibraryIndex
	placed := PlaceExplicitCircuit(ctx, request, placementOpts)
	placementStageIndex := len(stages)
	stages = append(stages, placed.Stage)
	if workflowStageBlocked(placed.Stage) {
		stages = appendSkippedCreateStages(stages, StagePlacement, "explicit placement did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	routingOpts := opts.Routing
	routingOpts.Skip = routingOpts.Skip || opts.SkipRouting || request.Validation.SkipRouting
	routingOpts.yieldToPlacementRepair = routingRetryAllowsPlacementHint(request.RoutingRetry, PlacementRetryImproveFanout)
	routed := RouteExplicitCircuit(ctx, request, placed, routingOpts)
	placed, routed, _ = maybeRetryExplicitPlacementRouting(ctx, request, placed, routed, routingOpts, request.RoutingRetry)
	stages[placementStageIndex] = placed.Stage
	stages = append(stages, routed.Stage)
	if workflowStageBlocked(routed.Stage) {
		stages = appendSkippedCreateStages(stages, StageRouting, "explicit routing did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	hierarchy, hierarchyIssues := explicitCircuitHierarchy(request, opts.LibraryIndex)
	tx, projectTxIssues := explicitCircuitTransaction(request, schematicTx, placed, routed, opts.Overwrite, hierarchy, opts.LibraryIndex)
	projectTxIssues = append(projectTxIssues, hierarchyIssues...)
	if reports.HasBlockingIssue(projectTxIssues) {
		writeStage := NewStageResult(StageProjectWrite, projectTxIssues)
		stages = append(stages, writeStage)
		stages = appendSkippedCreateStages(stages, StageProjectWrite, "explicit project transaction did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	written := writeExplicitCircuitProject(ctx, request, tx, placed, routed, opts)
	stages = append(stages, written.Stage)
	if workflowStageBlocked(written.Stage) {
		stages = appendSkippedCreateStages(stages, StageProjectWrite, "project write did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	zoneFill := prepareGeneratedZoneFill(ctx, request, &written, opts)
	if reports.HasBlockingIssue(zoneFill.Issues) {
		stages = append(stages, generatedZoneFillWriterStage(zoneFill))
		stages = appendSkippedCreateStages(stages, StageWriterCorrect, "zone fill did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	writerChecked := CheckWriterCorrectnessWithOptions(ctx, &written, opts.Writer)
	mergeGeneratedZoneFillEvidence(&writerChecked.Stage, zoneFill)
	stages = append(stages, writerChecked.Stage)
	if workflowStageBlocked(writerChecked.Stage) {
		stages = appendSkippedCreateStages(stages, StageWriterCorrect, "writer correctness check did not complete")
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}
	validationOpts, kicadOpts := createValidationOptions(request, opts)
	validated := ValidateProject(ctx, &request, &written, validationOpts)
	checked := RunKiCadChecks(ctx, &request, &written, kicadOpts)
	validated.Stage = reconcileDeferredZoneFillValidation(validated.Stage, checked.DRC)
	stages = append(stages, validated.Stage, checked.Stage)
	stages = append(stages, runExplicitSimulation(request, opts.OutputDir, opts.Overwrite))
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
	for index, operation := range tx.Operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.schematic", Message: err.Error()})
			continue
		}
		payload.PreferResolverSymbol = true
		raw, err := json.Marshal(payload)
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.schematic", Message: err.Error()})
			continue
		}
		tx.Operations[index].Raw = raw
	}
	return tx, issues
}

func explicitCircuitHierarchy(request Request, index *libraryresolver.LibraryIndex) (*transactions.SchematicHierarchy, []reports.Issue) {
	if request.ExplicitCircuit == nil || !request.ExplicitCircuit.AutoHierarchy {
		return nil, nil
	}
	return schematicir.HierarchyForProject(request.ExplicitCircuit.Schematic, index)
}

func explicitCircuitTransaction(request Request, schematicTx transactions.Transaction, placed PlacementStageResult, routed RoutingStageResult, overwrite bool, hierarchy *transactions.SchematicHierarchy, libraryIndex *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
	tx := schematicTx
	var issues []reports.Issue
	boardOps, boardIssues := boardOperations(&request)
	issues = append(issues, boardIssues...)
	tx.Operations = append(tx.Operations, boardOps...)
	placementOps, placementIssues := explicitPlacementWriteOperations(placed.Result.Operations, libraryIndex)
	issues = append(issues, placementIssues...)
	tx.Operations = append(tx.Operations, placementOps...)
	tx.Operations = append(tx.Operations, routed.Operations...)
	zoneOps, zoneIssues := explicitZoneOperations(request)
	issues = append(issues, zoneIssues...)
	tx.Operations = append(tx.Operations, zoneOps...)
	appendExplicitOperation(&tx, transactions.OpWriteProject, transactions.WriteProjectOperation{
		Op: transactions.OpWriteProject, Overwrite: overwrite,
		RequireSchematicReadability: request.ExplicitCircuit.Schematic.Policy.Acceptance == schematicir.AcceptanceReadable,
		Hierarchy:                   hierarchy,
	}, &issues)
	return tx, issues
}

func writeExplicitCircuitProject(ctx context.Context, request Request, tx transactions.Transaction, placed PlacementStageResult, routed RoutingStageResult, opts CreateOptions) ProjectWriteResult {
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
		OutputDir: outputDir, Overwrite: opts.Overwrite, Seed: opts.Seed, CopperLayers: request.Board.Layers, LibraryIndex: opts.LibraryIndex,
		SuppressPinmapWarnings: opts.LibraryIndex != nil, SuppressExplicitPinSymbolErrors: opts.LibraryIndex != nil,
		DefaultNetClassClearance:   kicadfiles.MM(projectNetClassClearanceMM(&routed, &placed)),
		MinimumCopperEdgeClearance: kicadfiles.MM(projectMinimumCopperEdgeClearanceMM(&routed)),
		MinimumViaDiameter:         kicadfiles.MM(projectMinimumViaDiameterMM(tx.Operations)),
		MinimumThroughHoleDiameter: kicadfiles.MM(projectMinimumThroughHoleDiameterMM(&placed, tx.Operations)),
		PreserveFootprintGeometry:  true,
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
