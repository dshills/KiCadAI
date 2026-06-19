package repair

import (
	"context"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func BuiltInPostApplyValidators(opts PostValidationOptions) []PostApplyValidator {
	validators := []PostApplyValidator{}
	if opts.WriterCorrectness {
		validators = append(validators, WriterCorrectnessValidator{Options: opts})
	}
	return validators
}

type WriterCorrectnessValidator struct {
	Options PostValidationOptions
}

func (validator WriterCorrectnessValidator) ValidatePostApply(ctx context.Context, input PostApplyValidationContext) PostApplyValidation {
	if err := ctx.Err(); err != nil {
		return PostApplyValidation{Name: "writer_correctness", Issues: []reports.Issue{contextIssue(err)}}
	}
	target := strings.TrimSpace(input.OutputDir)
	if target == "" {
		target = strings.TrimSpace(input.Target.Root)
	}
	if target == "" {
		return PostApplyValidation{Name: "writer_correctness", Issues: []reports.Issue{persistedIssue(reports.CodeInvalidArgument, "output", "post-apply writer correctness target is required")}}
	}
	result := writercorrectness.Validate(ctx, target, writercorrectness.Options{
		KiCadCLI:              validator.Options.KiCadCLI,
		KeepArtifacts:         validator.Options.KeepArtifacts,
		ArtifactDir:           validator.Options.ArtifactDir,
		RequireKiCadRoundTrip: false,
	})
	return PostApplyValidation{
		Name:      "writer_correctness",
		Issues:    result.Issues,
		Artifacts: result.Artifacts,
	}
}
