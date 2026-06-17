package writercorrectness

import (
	"context"

	"kicadai/internal/reports"
)

func Validate(ctx context.Context, input string, opts Options) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	result := NewResult(input)
	target, projectCheck := ResolveTarget(input, opts)
	result.Target = target
	result.AddCheck(projectCheck)

	if !projectCheckFailed(projectCheck) {
		if appendCanceled(ctx, &result) {
			result.Finish()
			return result
		}
		_, schematicChecks := CheckSchematics(target)
		for _, check := range schematicChecks {
			result.AddCheck(check)
		}
		if appendCanceled(ctx, &result) {
			result.Finish()
			return result
		}
		_, transferCheck := CheckSchematicToPCBTransfer(target)
		result.AddCheck(transferCheck)
		if appendCanceled(ctx, &result) {
			result.Finish()
			return result
		}
		_, pcbChecks := CheckPCBFootprintPads(target)
		for _, check := range pcbChecks {
			result.AddCheck(check)
		}
		if appendCanceled(ctx, &result) {
			result.Finish()
			return result
		}
		_, copperChecks := CheckPCBCopperZones(target)
		for _, check := range copperChecks {
			result.AddCheck(check)
		}
		result.AddCheck(CheckKiCadRoundTripEvidence(ctx, target, opts))
	}

	result.Finish()
	return result
}

func appendCanceled(ctx context.Context, result *Result) bool {
	if err := ctx.Err(); err != nil {
		result.AddCheck(CheckResult{
			Name:     "context",
			Required: true,
			Issues: []reports.Issue{{
				Code:     reports.CodeOperationCanceled,
				Severity: reports.SeverityError,
				Path:     "writer.context",
				Message:  err.Error(),
			}},
		})
		return true
	}
	return false
}

func projectCheckFailed(check CheckResult) bool {
	for _, issue := range check.Issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}
