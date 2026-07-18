package designworkflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/inspect"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type ProjectWriteOptions struct {
	OutputDir                 string
	Overwrite                 bool
	Seed                      string
	LibraryIndex              *libraryresolver.LibraryIndex
	PreserveFootprintGeometry bool
}

type ProjectWriteResult struct {
	Transaction transactions.Transaction      `json:"transaction"`
	Validation  transactions.ValidationResult `json:"validation"`
	ApplyResult transactions.ApplyResult      `json:"apply_result"`
	Inspection  inspect.ProjectSummary        `json:"inspection"`
	Stage       StageResult                   `json:"stage"`
}

func WriteProject(ctx context.Context, request *Request, plan *BlockPlanResult, placed *PlacementStageResult, routed *RoutingStageResult, opts ProjectWriteOptions) ProjectWriteResult {
	if err := ctx.Err(); err != nil {
		return ProjectWriteResult{Stage: NewStageResult(StageProjectWrite, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  err.Error(),
		}})}
	}
	if request == nil || plan == nil || placed == nil || routed == nil {
		return ProjectWriteResult{Stage: NewStageResult(StageProjectWrite, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "workflow",
			Message:  "request, plan, placement, and routing results are required",
		}})}
	}
	if plan.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(plan.Stage.Issues) {
		return ProjectWriteResult{Stage: skippedProjectWriteStage("block planning did not complete")}
	}
	if placed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(placed.Stage.Issues) {
		return ProjectWriteResult{Stage: skippedProjectWriteStage("placement did not complete")}
	}
	if routed.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(routed.Stage.Issues) {
		return ProjectWriteResult{Stage: skippedProjectWriteStage("routing did not complete")}
	}
	if strings.TrimSpace(opts.OutputDir) == "" {
		return ProjectWriteResult{Stage: NewStageResult(StageProjectWrite, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "output",
			Message:  "output directory is required",
		}})}
	}
	tx, issues := ProjectTransaction(request, plan, placed, routed, opts.Overwrite)
	validation := transactions.Validate(tx)
	issues = append(issues, validation.Issues...)
	netAssignment := SummarizeGeneratedNetAssignment(placed, routed)
	if reports.HasBlockingIssue(issues) {
		stage := NewStageResult(StageProjectWrite, issues)
		stage.Summary = map[string]any{
			"operation_count": len(tx.Operations),
			"net_assignment":  netAssignment,
		}
		return ProjectWriteResult{Transaction: tx, Validation: validation, Stage: stage}
	}
	if err := ctx.Err(); err != nil {
		return canceledProjectWriteResult(err, tx, validation, transactions.ApplyResult{}, issues)
	}
	outputDir := opts.OutputDir
	if strings.TrimSpace(outputDir) == "" {
		return ProjectWriteResult{Transaction: tx, Validation: validation, Stage: NewStageResult(StageProjectWrite, append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "output",
			Message:  "output directory is required",
		}))}
	}
	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return ProjectWriteResult{Transaction: tx, Validation: validation, Stage: NewStageResult(StageProjectWrite, append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "output",
			Message:  "resolve output directory: " + err.Error(),
		}))}
	}
	outputDir = absolute
	applyResult := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:                       outputDir,
		Overwrite:                       opts.Overwrite,
		Seed:                            opts.Seed,
		CopperLayers:                    request.Board.Layers,
		LibraryIndex:                    opts.LibraryIndex,
		SuppressPinmapWarnings:          opts.LibraryIndex != nil,
		SuppressExplicitPinSymbolErrors: opts.LibraryIndex != nil,
		// Routing was computed against transaction pads; resolver hydration here
		// would move anchors after routes have already been generated.
		PreserveFootprintGeometry: opts.PreserveFootprintGeometry,
	})
	issues = append(issues, applyResult.Issues...)
	var inspection inspect.ProjectSummary
	if !reports.HasBlockingIssue(applyResult.Issues) {
		var err error
		inspection, err = inspect.Project(outputDir)
		if err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "inspect",
				Message:  err.Error(),
			})
		} else {
			issues = append(issues, inspection.Issues...)
		}
	}
	stage := NewStageResult(StageProjectWrite, issues)
	stage.Artifacts = append([]reports.Artifact(nil), applyResult.Artifacts...)
	stage.Summary = map[string]any{
		"operation_count": len(tx.Operations),
		"artifact_count":  len(applyResult.Artifacts),
		"net_assignment":  netAssignment,
	}
	return ProjectWriteResult{Transaction: tx, Validation: validation, ApplyResult: applyResult, Inspection: inspection, Stage: stage}
}

func canceledProjectWriteResult(err error, tx transactions.Transaction, validation transactions.ValidationResult, applyResult transactions.ApplyResult, issues []reports.Issue) ProjectWriteResult {
	issues = append(issues, reports.Issue{
		Code:     reports.CodeOperationCanceled,
		Severity: reports.SeverityBlocked,
		Path:     "context",
		Message:  err.Error(),
	})
	stage := NewStageResult(StageProjectWrite, issues)
	stage.Artifacts = append([]reports.Artifact(nil), applyResult.Artifacts...)
	stage.Summary = map[string]any{
		"operation_count": len(tx.Operations),
		"artifact_count":  len(applyResult.Artifacts),
	}
	return ProjectWriteResult{Transaction: tx, Validation: validation, ApplyResult: applyResult, Stage: stage}
}

func ProjectTransaction(request *Request, plan *BlockPlanResult, placed *PlacementStageResult, routed *RoutingStageResult, overwrite bool) (transactions.Transaction, []reports.Issue) {
	if request == nil || plan == nil || placed == nil || routed == nil {
		return transactions.Transaction{}, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "workflow",
			Message:  "request, plan, placement, and routing results are required",
		}}
	}
	tx, issues := schematicTransaction(request, plan, overwrite)
	if reports.HasBlockingIssue(issues) {
		return tx, issues
	}
	boardOps, boardIssues := boardOperations(request)
	issues = append(issues, boardIssues...)
	placementOps := placementStageOperations(placed)
	insert := make([]transactions.Operation, 0, len(boardOps)+len(placementOps)+len(routed.Operations))
	insert = append(insert, boardOps...)
	insert = append(insert, placementOps...)
	insert = append(insert, routed.Operations...)
	tx = replacePlacementOperationsBeforeWriteProject(tx, insert)
	return tx, issues
}

func placementStageOperations(placed *PlacementStageResult) []transactions.Operation {
	if placed == nil {
		return nil
	}
	if len(placed.Result.Operations) == 0 {
		return nil
	}
	return append([]transactions.Operation(nil), placed.Result.Operations...)
}

func schematicTransaction(request *Request, plan *BlockPlanResult, overwrite bool) (transactions.Transaction, []reports.Issue) {
	projectName := request.Name
	composition := plan.Composition
	if composition.ProjectName != "" {
		projectName = composition.ProjectName
	}
	output := plan.Output
	paper := ""
	layout := request.SchematicLayout
	if plan.Request.SchematicLayout != nil {
		layout = plan.Request.SchematicLayout
	}
	if layout != nil {
		operations, selectedPaper, err := layoutSchematicOperations(output, *layout)
		if err != nil {
			return transactions.Transaction{}, []reports.Issue{{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityBlocked,
				Path:     "project.transaction.layout",
				Message:  err.Error(),
			}}
		}
		output.Operations = operations
		paper = selectedPaper
	}
	tx, err := blocks.ProjectTransactionForCompositionOutput(projectName, output, overwrite)
	if err != nil {
		return tx, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project.transaction",
			Message:  err.Error(),
		}}
	}
	tx, err = applyFabricationMetadata(tx, request.Fabrication)
	if err != nil {
		return tx, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project.transaction.fabrication",
			Message:  err.Error(),
		}}
	}
	if paper != "" {
		tx, err = applySchematicPaper(tx, paper)
		if err != nil {
			return tx, []reports.Issue{{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityBlocked,
				Path:     "project.transaction.layout",
				Message:  err.Error(),
			}}
		}
	}
	return tx, nil
}

func applyFabricationMetadata(tx transactions.Transaction, metadata FabricationMetadataSpec) (transactions.Transaction, error) {
	variables := map[string]string{}
	if finish := strings.TrimSpace(metadata.BoardFinish); finish != "" {
		variables["board_finish"] = finish
	}
	if notes := strings.TrimSpace(metadata.FabricationNotes); notes != "" {
		variables["fabrication_notes"] = notes
	}
	if len(variables) == 0 {
		return tx, nil
	}
	for index, operation := range tx.Operations {
		if operation.Op != transactions.OpCreateProject {
			continue
		}
		updated, err := applyFabricationMetadataToProjectOperation(operation, variables)
		if err != nil {
			return tx, err
		}
		tx.Operations[index] = updated
	}
	return tx, nil
}

func applyFabricationMetadataToProjectOperation(operation transactions.Operation, variables map[string]string) (transactions.Operation, error) {
	var payload transactions.CreateProjectOperation
	if err := json.Unmarshal(operation.Raw, &payload); err != nil {
		return transactions.Operation{}, err
	}
	if payload.TextVariables == nil {
		payload.TextVariables = map[string]string{}
	}
	for key, value := range variables {
		payload.TextVariables[key] = value
	}
	return workflowOperation(transactions.OpCreateProject, payload)
}

func boardOperations(request *Request) ([]transactions.Operation, []reports.Issue) {
	if request.Board.WidthMM <= 0 || request.Board.HeightMM <= 0 {
		return nil, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "board",
			Message:  "board dimensions must be positive",
		}}
	}
	operation, err := workflowOperation(transactions.OpSetBoardOutline, transactions.SetBoardOutlineOperation{
		Op:    transactions.OpSetBoardOutline,
		Board: &transactions.BoardSize{WidthMM: request.Board.WidthMM, HeightMM: request.Board.HeightMM},
	})
	if err != nil {
		return nil, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "board",
			Message:  err.Error(),
		}}
	}
	return []transactions.Operation{operation}, nil
}

func replacePlacementOperationsBeforeWriteProject(tx transactions.Transaction, operations []transactions.Operation) transactions.Transaction {
	if len(operations) == 0 {
		return tx
	}
	replacementRefs := map[string]struct{}{}
	for _, operation := range operations {
		normalizedRef := normalizedOperationRef(operation.Ref)
		if operation.Op == transactions.OpPlaceFootprint && normalizedRef != "" {
			replacementRefs[normalizedRef] = struct{}{}
		}
	}
	filtered := make([]transactions.Operation, 0, len(tx.Operations)+len(operations))
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpPlaceFootprint {
			if _, replace := replacementRefs[normalizedOperationRef(operation.Ref)]; replace {
				continue
			}
		}
		if operation.Op == transactions.OpSetBoardOutline {
			continue
		}
		filtered = append(filtered, operation)
	}
	insertAt := len(filtered)
	for index := range filtered {
		if filtered[index].Op == transactions.OpWriteProject {
			insertAt = index
			break
		}
	}
	next := make([]transactions.Operation, 0, len(filtered)+len(operations))
	next = append(next, filtered[:insertAt]...)
	next = append(next, operations...)
	next = append(next, filtered[insertAt:]...)
	tx.Operations = next
	return tx
}

func normalizedOperationRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func workflowOperation(kind transactions.OperationKind, payload any) (transactions.Operation, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperation(kind, raw), nil
}

func skippedProjectWriteStage(reason string) StageResult {
	return StageResult{Name: StageProjectWrite, Status: StageStatusSkipped, Summary: map[string]any{"reason": reason}}
}
