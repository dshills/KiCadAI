package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"kicadai/internal/generationcapability"
	"kicadai/internal/reports"
)

func runGenerationCapability(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) > 0 && strings.TrimSpace(opts.commandArgs[0]) == "expansion" {
		return runCapabilityExpansion(ctx, opts, stdout)
	}
	if len(opts.commandArgs) == 0 || strings.TrimSpace(opts.commandArgs[0]) != "generation" {
		issue := reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "capability",
			Message:    "capability requires subcommand: generation or expansion",
			Suggestion: "run kicadai capability generation --json or kicadai capability expansion plan --request <assessment.json>",
		}
		return writeReportFailure(stdout, "capability", issue)
	}
	for _, argument := range opts.commandArgs[1:] {
		if strings.TrimSpace(argument) != "--json" {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "capability", Message: "capability generation accepts no arguments other than --json"}
			return writeReportFailure(stdout, "capability", issue)
		}
	}

	catalog, err := loadComponentCatalogForOptions(ctx, opts)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "catalog", Message: fmt.Sprintf("load generation catalog: %v", err)}
		return writeReportFailure(stdout, "capability", issue)
	}
	document, err := generationcapability.BuildDocument(catalog)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "generation", Message: err.Error()}
		return writeReportFailure(stdout, "capability", issue)
	}
	return writeReportJSON(stdout, reports.OKResult("capability", document, nil))
}
