package writercorrectness

import (
	"context"
	"fmt"

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
		_, schematicChecks := CheckSchematicsWithOptions(target, opts)
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
	if opts.LibraryResolutionUsed {
		result.AddCheck(CheckResult{
			Name:     CheckLibraryResolver,
			Required: true,
			Issues:   opts.LibraryIssues,
			Summary:  libraryResolverSummary(opts.LibraryIssues),
		})
	}

	result.Finish()
	return result
}

func libraryResolverSummary(issues []reports.Issue) string {
	issueCount := len(issues)
	if issueCount == 0 {
		return "resolved library inputs"
	}
	if reports.HasBlockingIssue(issues) {
		if issueCount == 1 {
			return "failed to resolve library inputs with 1 issue"
		}
		return fmt.Sprintf("failed to resolve library inputs with %d issues", issueCount)
	}
	if issueCount == 1 {
		return "resolved library inputs with 1 issue"
	}
	return fmt.Sprintf("resolved library inputs with %d issues", issueCount)
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
