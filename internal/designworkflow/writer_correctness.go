package designworkflow

import (
	"context"

	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

type WriterCorrectnessStageResult struct {
	Writer writercorrectness.Result `json:"writer"`
	Stage  StageResult              `json:"stage"`
}

func CheckWriterCorrectness(ctx context.Context, write *ProjectWriteResult) WriterCorrectnessStageResult {
	return CheckWriterCorrectnessWithOptions(ctx, write, writercorrectness.Options{})
}

func CheckWriterCorrectnessWithOptions(ctx context.Context, write *ProjectWriteResult, opts writercorrectness.Options) WriterCorrectnessStageResult {
	if ctx == nil {
		return WriterCorrectnessStageResult{Stage: NewStageResult(StageWriterCorrect, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  "context is required",
		}})}
	}
	if err := ctx.Err(); err != nil {
		return WriterCorrectnessStageResult{Stage: NewStageResult(StageWriterCorrect, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  err.Error(),
		}})}
	}
	if write == nil {
		return WriterCorrectnessStageResult{Stage: NewStageResult(StageWriterCorrect, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project_write",
			Message:  "project write result is required",
		}})}
	}
	if write.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(write.Stage.Issues) {
		return WriterCorrectnessStageResult{Stage: StageResult{Name: StageWriterCorrect, Status: StageStatusSkipped, Summary: map[string]any{"reason": "project write did not complete"}}}
	}
	projectRoot := projectRootFromWrite(write)
	if projectRoot == "" {
		return WriterCorrectnessStageResult{Stage: NewStageResult(StageWriterCorrect, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project",
			Message:  "project root is required for writer correctness",
		}})}
	}
	writer := writercorrectness.Validate(ctx, projectRoot, opts)
	stage := NewStageResult(StageWriterCorrect, writer.Issues)
	stage.Summary = map[string]any{
		"ok":             writer.OK,
		"check_count":    writer.OverallSummary.CheckCount,
		"fail_count":     writer.OverallSummary.FailCount,
		"warning_count":  writer.OverallSummary.WarningCount,
		"skipped_count":  writer.OverallSummary.SkippedCount,
		"blocking_count": writer.OverallSummary.BlockingCount,
	}
	stage.Artifacts = make([]reports.Artifact, len(writer.Artifacts))
	copy(stage.Artifacts, writer.Artifacts)
	return WriterCorrectnessStageResult{Writer: writer, Stage: stage}
}
