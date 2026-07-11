package designworkflow

import (
	"context"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type SchematicApplyOptions struct {
	OutputDir string
	Overwrite bool
	Seed      string
}

type SchematicApplyResult struct {
	Transaction transactions.Transaction      `json:"transaction"`
	Validation  transactions.ValidationResult `json:"validation"`
	ApplyResult transactions.ApplyResult      `json:"apply_result"`
	Stage       StageResult                   `json:"stage"`
}

func ApplySchematic(ctx context.Context, plan BlockPlanResult, opts SchematicApplyOptions) SchematicApplyResult {
	var issues []reports.Issue
	if ctx == nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "context",
			Message:  "context is required",
		})
	} else if err := ctx.Err(); err != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityError,
			Path:     "context",
			Message:  err.Error(),
		})
	}
	if reports.HasBlockingIssue(plan.Stage.Issues) {
		stage := StageResult{
			Name:   StageSchematic,
			Status: StageStatusSkipped,
			Summary: map[string]any{
				"reason": "block planning did not complete",
			},
		}
		return SchematicApplyResult{Stage: stage}
	}
	if strings.TrimSpace(opts.OutputDir) == "" {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "output",
			Message:  "output directory is required",
		})
	}
	if reports.HasBlockingIssue(issues) {
		return SchematicApplyResult{Stage: NewStageResult(StageSchematic, issues)}
	}
	projectName := plan.Composition.ProjectName
	if projectName == "" {
		projectName = plan.Request.Name
	}
	output := plan.Output
	paper := ""
	if requiresGeneratedSchematicLayout(projectName) {
		operations, selectedPaper, err := layoutSchematicOperations(output.Operations)
		if err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "schematic.layout",
				Message:  err.Error(),
			})
			return SchematicApplyResult{Stage: NewStageResult(StageSchematic, issues)}
		}
		output.Operations = operations
		paper = selectedPaper
	}
	tx, err := blocks.ProjectTransactionForCompositionOutput(projectName, output, opts.Overwrite)
	if err != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "schematic.transaction",
			Message:  err.Error(),
		})
		return SchematicApplyResult{Transaction: tx, Stage: NewStageResult(StageSchematic, issues)}
	}
	if paper != "" {
		tx, err = applySchematicPaper(tx, paper)
		if err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "schematic.layout",
				Message:  err.Error(),
			})
			return SchematicApplyResult{Transaction: tx, Stage: NewStageResult(StageSchematic, issues)}
		}
	}
	validation := transactions.Validate(tx)
	issues = append(issues, validation.Issues...)
	if reports.HasBlockingIssue(issues) {
		stage := NewStageResult(StageSchematic, issues)
		stage.Summary = map[string]any{"operation_count": len(tx.Operations)}
		return SchematicApplyResult{Transaction: tx, Validation: validation, Stage: stage}
	}
	applyResult := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir: opts.OutputDir,
		Overwrite: opts.Overwrite,
		Seed:      opts.Seed,
	})
	issues = append(issues, applyResult.Issues...)
	stage := NewStageResult(StageSchematic, issues)
	stage.Artifacts = append([]reports.Artifact(nil), applyResult.Artifacts...)
	stage.Summary = map[string]any{
		"operation_count": len(tx.Operations),
		"artifact_count":  len(applyResult.Artifacts),
	}
	return SchematicApplyResult{
		Transaction: tx,
		Validation:  validation,
		ApplyResult: applyResult,
		Stage:       stage,
	}
}
