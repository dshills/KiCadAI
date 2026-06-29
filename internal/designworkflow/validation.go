package designworkflow

import (
	"context"
	"fmt"
	"sync"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/evaluate"
	"kicadai/internal/reports"
)

type ValidationOptions struct {
	StrictZones    bool
	StrictUnrouted bool
	RequireDRC     bool
	KiCadCLI       string
	KeepArtifacts  bool
	ArtifactDir    string
}

type ValidationStageResult struct {
	Evaluation      evaluate.Report        `json:"evaluation"`
	BoardValidation boardvalidation.Result `json:"board_validation"`
	Stage           StageResult            `json:"stage"`
}

func ValidateProject(ctx context.Context, request *Request, write *ProjectWriteResult, opts ValidationOptions) ValidationStageResult {
	if ctx == nil {
		return ValidationStageResult{Stage: NewStageResult(StageValidation, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  "context is required",
		}})}
	}
	if err := ctx.Err(); err != nil {
		return ValidationStageResult{Stage: NewStageResult(StageValidation, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  err.Error(),
		}})}
	}
	if write == nil {
		return ValidationStageResult{Stage: NewStageResult(StageValidation, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project_write",
			Message:  "project write result is required",
		}})}
	}
	if write.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(write.Stage.Issues) {
		return ValidationStageResult{Stage: StageResult{Name: StageValidation, Status: StageStatusSkipped, Summary: map[string]any{"reason": "project write did not complete"}}}
	}
	projectRoot := projectRootFromWrite(write)
	if projectRoot == "" {
		return ValidationStageResult{Stage: NewStageResult(StageValidation, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project",
			Message:  "project root is required for validation",
		}})}
	}

	var issues []reports.Issue
	var evaluation evaluate.Report
	var evaluationErr error
	var board boardvalidation.Result
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				evaluationErr = fmt.Errorf("evaluation panic: %v", recovered)
			}
		}()
		evaluation, evaluationErr = evaluate.ProjectContext(ctx, projectRoot)
	}()
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				board.Issues = append(board.Issues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityBlocked,
					Path:     "board_validation",
					Message:  fmt.Sprintf("board validation panic: %v", recovered),
				})
			}
		}()
		board = boardvalidation.Validate(ctx, projectRoot, boardValidationOptions(request, opts))
	}()
	wg.Wait()
	if evaluationErr != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "evaluate",
			Message:  evaluationErr.Error(),
		})
	} else {
		issues = append(issues, evaluation.Issues...)
	}
	issues = append(issues, board.Issues...)
	stage := NewStageResult(StageValidation, issues)
	stage.Summary = map[string]any{
		"evaluation_checks":       len(evaluation.Checks),
		"board_validation_checks": len(board.Checks),
		"board_status":            board.Status,
		"fabrication_ready":       board.FabricationReady,
	}
	stage.Artifacts = append([]reports.Artifact{}, board.Artifacts...)
	return ValidationStageResult{Evaluation: evaluation, BoardValidation: board, Stage: stage}
}

func projectRootFromWrite(write *ProjectWriteResult) string {
	if write == nil {
		return ""
	}
	return write.Inspection.Root
}

func boardValidationOptions(request *Request, opts ValidationOptions) boardvalidation.Options {
	if request != nil {
		if request.Validation.StrictZones {
			opts.StrictZones = true
		}
		if request.Validation.StrictUnrouted {
			opts.StrictUnrouted = true
		}
	}
	return boardvalidation.Options{
		StrictZones:    opts.StrictZones,
		StrictUnrouted: opts.StrictUnrouted,
		RequireDRC:     opts.RequireDRC,
		KiCadCLI:       opts.KiCadCLI,
		KeepArtifacts:  opts.KeepArtifacts,
		ArtifactDir:    opts.ArtifactDir,
	}
}
