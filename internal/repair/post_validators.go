package repair

import (
	"context"
	"strings"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

const (
	postValidatorWriterCorrectness = "writer_correctness"
	postValidatorBoardValidation   = "board_validation"
)

func BuiltInPostApplyValidators(opts PostValidationOptions) []PostApplyValidator {
	validators := []PostApplyValidator{}
	if opts.WriterCorrectness {
		validators = append(validators, WriterCorrectnessValidator{Options: opts})
	}
	if opts.BoardValidation {
		validators = append(validators, BoardValidationValidator{Options: opts})
	}
	return validators
}

type WriterCorrectnessValidator struct {
	Options PostValidationOptions
}

type BoardValidationValidator struct {
	Options PostValidationOptions
}

func (validator BoardValidationValidator) ValidatePostApply(ctx context.Context, input PostApplyValidationContext) PostApplyValidation {
	if err := ctx.Err(); err != nil {
		return PostApplyValidation{Name: postValidatorBoardValidation, Issues: []reports.Issue{contextIssue(err)}}
	}
	target := postApplyValidationTarget(input)
	if target == "" {
		return PostApplyValidation{Name: postValidatorBoardValidation, Issues: []reports.Issue{persistedIssue(reports.CodeInvalidArgument, "output", "post-apply board validation target is required")}}
	}
	result := boardvalidation.Validate(ctx, target, boardValidationOptions(validator.Options))
	return PostApplyValidation{
		Name:      postValidatorBoardValidation,
		Issues:    result.Issues,
		Artifacts: result.Artifacts,
	}
}

func boardValidationOptions(opts PostValidationOptions) boardvalidation.Options {
	requireDRC := opts.RequireKiCadDRC || opts.KiCadDRC
	return boardvalidation.Options{
		StrictZones:     opts.StrictZones,
		StrictUnrouted:  opts.StrictUnrouted,
		RequireDRC:      requireDRC,
		AllowMissingDRC: opts.AllowMissingKiCadChecks && !requireDRC,
		KiCadCLI:        opts.KiCadCLI,
		KeepArtifacts:   opts.KeepArtifacts,
		ArtifactDir:     opts.ArtifactDir,
	}
}

func (validator WriterCorrectnessValidator) ValidatePostApply(ctx context.Context, input PostApplyValidationContext) PostApplyValidation {
	if err := ctx.Err(); err != nil {
		return PostApplyValidation{Name: postValidatorWriterCorrectness, Issues: []reports.Issue{contextIssue(err)}}
	}
	target := postApplyValidationTarget(input)
	if target == "" {
		return PostApplyValidation{Name: postValidatorWriterCorrectness, Issues: []reports.Issue{persistedIssue(reports.CodeInvalidArgument, "output", "post-apply writer correctness target is required")}}
	}
	result := writercorrectness.Validate(ctx, target, writercorrectness.Options{
		KiCadCLI:              validator.Options.KiCadCLI,
		KeepArtifacts:         validator.Options.KeepArtifacts,
		ArtifactDir:           validator.Options.ArtifactDir,
		RequireKiCadRoundTrip: false,
	})
	return PostApplyValidation{
		Name:      postValidatorWriterCorrectness,
		Issues:    result.Issues,
		Artifacts: result.Artifacts,
	}
}

func postApplyValidationTarget(input PostApplyValidationContext) string {
	target := strings.TrimSpace(input.OutputDir)
	if target == "" {
		target = strings.TrimSpace(input.Target.Root)
	}
	return target
}
