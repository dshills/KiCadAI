package designworkflow

import (
	"context"
	"math"
	"path/filepath"
	"sort"

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

	tx, txIssues := explicitCircuitTransaction(request, opts.Overwrite, opts.LibraryIndex)
	schematic := NewStageResult(StageSchematic, txIssues)
	schematic.Summary = map[string]any{"operation_count": len(tx.Operations), "mode": "schematic_ir"}
	stages = append(stages, schematic)
	if workflowStageBlocked(schematic) {
		stages = append(stages, skippedWorkflowStages("explicit schematic generation did not complete", StageSchematicElectrical, StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	electrical := schematicElectricalStageFromTransaction(tx)
	stages = append(stages, electrical)
	if workflowStageBlocked(electrical) {
		stages = append(stages, skippedWorkflowStages("schematic electrical rules did not pass", StagePCBRealization, StagePlacement, StageRouting, StageProjectWrite, StageWriterCorrect, StageValidation, StageKiCadChecks)...)
		return BuildWorkflowResult(project, request.Validation.Acceptance, stages)
	}

	pcbRealization := NewStageResult(StagePCBRealization, nil)
	pcbRealization.Summary = map[string]any{"footprint_count": len(request.ExplicitCircuit.Components), "net_count": len(request.ExplicitCircuit.Nets)}
	placement := NewStageResult(StagePlacement, nil)
	placement.Summary = map[string]any{"placement_count": len(request.ExplicitCircuit.Components), "mode": "deterministic_grid"}
	routing := StageResult{Name: StageRouting, Status: StageStatusSkipped, Summary: map[string]any{"reason": "explicit graph routing constraints are applied in the next workflow phase"}}
	stages = append(stages, pcbRealization, placement, routing)

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

func explicitCircuitTransaction(request Request, overwrite bool, index *libraryresolver.LibraryIndex) (transactions.Transaction, []reports.Issue) {
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
	boardOps, boardIssues := boardOperations(&request)
	issues = append(issues, boardIssues...)
	tx.Operations = append(tx.Operations, boardOps...)
	components := sortedExplicitComponents(request.ExplicitCircuit.Components)
	for i, component := range components {
		appendExplicitOperation(&tx, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op: transactions.OpPlaceFootprint, Ref: component.Reference, Role: component.Role,
			FootprintID: component.FootprintID, Value: component.Value,
			At: explicitGridPoint(i, len(components), request.Board), Layer: "F.Cu",
			Pads: explicitPadSpecs(component.Pads), HideDefaultFootprintText: true,
		}, &issues)
	}
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

func explicitGridPoint(index, count int, board BoardSpec) transactions.Point {
	columns := int(math.Ceil(math.Sqrt(float64(count))))
	if columns < 1 {
		columns = 1
	}
	rows := (count + columns - 1) / columns
	margin := math.Max(2, board.EdgeClearanceMM+1)
	usableWidth := math.Max(0, board.WidthMM-2*margin)
	usableHeight := math.Max(0, board.HeightMM-2*margin)
	column, row := index%columns, index/columns
	x, y := board.WidthMM/2, board.HeightMM/2
	if columns > 1 {
		x = margin + usableWidth*float64(column)/float64(columns-1)
	}
	if rows > 1 {
		y = margin + usableHeight*float64(row)/float64(rows-1)
	}
	return transactions.Point{XMM: x, YMM: y}
}

func sortedExplicitComponents(components []ExplicitComponentSpec) []ExplicitComponentSpec {
	result := append([]ExplicitComponentSpec(nil), components...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].Reference < result[j].Reference })
	return result
}

func explicitPadSpecs(pads []ExplicitPadSpec) []transactions.PadSpec {
	result := make([]transactions.PadSpec, 0, len(pads))
	for _, pad := range pads {
		var net *string
		if pad.Net != "" {
			value := pad.Net
			net = &value
		}
		result = append(result, transactions.PadSpec{Name: pad.Name, Net: net})
	}
	return result
}

func appendExplicitOperation(tx *transactions.Transaction, kind transactions.OperationKind, payload any, issues *[]reports.Issue) {
	op, err := workflowOperation(kind, payload)
	if err != nil {
		*issues = append(*issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "explicit_circuit.transaction", Message: err.Error()})
		return
	}
	tx.Operations = append(tx.Operations, op)
}
