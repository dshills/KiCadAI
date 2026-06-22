package repair

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

const (
	postValidatorWriterCorrectness = "writer_correctness"
	postValidatorBoardValidation   = "board_validation"
	postValidatorKiCadERC          = "kicad_erc"
	postValidatorKiCadDRC          = "kicad_drc"
	postValidatorRoundTrip         = "kicad_round_trip"
)

func BuiltInPostApplyValidators(opts PostValidationOptions) []PostApplyValidator {
	validators := []PostApplyValidator{}
	if opts.WriterCorrectness {
		validators = append(validators, WriterCorrectnessValidator{Options: opts})
	}
	if opts.BoardValidation {
		validators = append(validators, BoardValidationValidator{Options: opts})
	}
	if opts.KiCadERC || opts.RequireKiCadERC {
		validators = append(validators, KiCadCheckValidator{Options: opts, Kind: checks.CheckKindERC})
	}
	if opts.KiCadDRC || opts.RequireKiCadDRC {
		validators = append(validators, KiCadCheckValidator{Options: opts, Kind: checks.CheckKindDRC})
	}
	if opts.RoundTrip || opts.RequireRoundTrip {
		validators = append(validators, RoundTripValidator{Options: opts})
	}
	return validators
}

type WriterCorrectnessValidator struct {
	Options PostValidationOptions
}

type BoardValidationValidator struct {
	Options PostValidationOptions
}

type KiCadCheckValidator struct {
	Options PostValidationOptions
	Kind    checks.CheckKind
}

type RoundTripValidator struct {
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

func (validator KiCadCheckValidator) ValidatePostApply(ctx context.Context, input PostApplyValidationContext) PostApplyValidation {
	name := postValidatorKiCadERC
	if validator.Kind == checks.CheckKindDRC {
		name = postValidatorKiCadDRC
	}
	if err := ctx.Err(); err != nil {
		return PostApplyValidation{Name: name, Issues: []reports.Issue{contextIssue(err)}}
	}
	target := postApplyValidationTarget(input)
	if target == "" {
		return PostApplyValidation{Name: name, Issues: []reports.Issue{persistedIssue(reports.CodeInvalidArgument, "output", "post-apply KiCad check target is required")}}
	}
	cli, err := checks.DiscoverCLI(validator.Options.KiCadCLI)
	if err != nil {
		return PostApplyValidation{Name: name, Issues: []reports.Issue{kicadUnavailableIssue(err, validator.Options, target)}}
	}
	checkOpts := checks.Options{
		KiCadCLI:      cli.Path,
		KeepArtifacts: validator.Options.KeepArtifacts,
		ArtifactDir:   validator.Options.ArtifactDir,
	}
	result, runErr := runKiCadCheck(ctx, validator.Kind, cli, target, checkOpts)
	issues, artifacts := kicadCheckIssuesAndArtifacts(target, result, runErr)
	return PostApplyValidation{Name: name, Issues: issues, Artifacts: artifacts}
}

func (validator RoundTripValidator) ValidatePostApply(ctx context.Context, input PostApplyValidationContext) PostApplyValidation {
	if err := ctx.Err(); err != nil {
		return PostApplyValidation{Name: postValidatorRoundTrip, Issues: []reports.Issue{contextIssue(err)}}
	}
	targetPath := postApplyValidationTarget(input)
	if targetPath == "" {
		return PostApplyValidation{Name: postValidatorRoundTrip, Issues: []reports.Issue{persistedIssue(reports.CodeInvalidArgument, "output", "post-apply round-trip target is required")}}
	}
	target, projectCheck := writercorrectness.ResolveTarget(targetPath, writercorrectness.Options{})
	if reports.HasBlockingIssue(projectCheck.Issues) {
		return PostApplyValidation{Name: postValidatorRoundTrip, Issues: projectCheck.Issues, Artifacts: projectCheck.Artifacts}
	}
	check := writercorrectness.CheckKiCadRoundTripEvidence(ctx, target, writercorrectness.Options{
		RequireKiCadRoundTrip: validator.Options.RequireRoundTrip || validator.Options.RoundTrip,
		KiCadCLI:              validator.Options.KiCadCLI,
		KeepArtifacts:         validator.Options.KeepArtifacts,
		ArtifactDir:           validator.Options.ArtifactDir,
	})
	return PostApplyValidation{Name: postValidatorRoundTrip, Issues: check.Issues, Artifacts: check.Artifacts, Skipped: check.Status == writercorrectness.CheckSkipped}
}

func boardValidationOptions(opts PostValidationOptions) boardvalidation.Options {
	requireDRC := opts.RequireKiCadDRC
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
		KiCadCLI:      validator.Options.KiCadCLI,
		KeepArtifacts: validator.Options.KeepArtifacts,
		ArtifactDir:   validator.Options.ArtifactDir,
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

func runKiCadCheck(ctx context.Context, kind checks.CheckKind, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, error) {
	if kind == checks.CheckKindDRC {
		return checks.RunDRC(ctx, cli, target, opts)
	}
	return checks.RunERC(ctx, cli, target, opts)
}

func kicadCheckIssuesAndArtifacts(target string, result checks.CheckResult, err error) ([]reports.Issue, []reports.Artifact) {
	issues := []reports.Issue{}
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   kicadFindingSeverity(finding.Severity),
			Path:       filepath.ToSlash(finding.File),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       kicadFindingNets(finding),
			Suggestion: fmt.Sprintf("KiCad %s rule %s", result.Kind, strings.TrimSpace(finding.Rule)),
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(result.ReportPath),
			Message:  parserIssue.Message,
		})
	}
	if err != nil {
		path := filepath.ToSlash(result.TargetPath)
		if strings.TrimSpace(path) == "" {
			path = filepath.ToSlash(target)
		}
		issues = append(issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     path,
			Message:  err.Error(),
		})
	}
	if strings.TrimSpace(result.ReportPath) == "" {
		return issues, nil
	}
	kind := reports.ArtifactERCReport
	if result.Kind == checks.CheckKindDRC {
		kind = reports.ArtifactDRCReport
	}
	return issues, []reports.Artifact{{
		Kind:        kind,
		Path:        filepath.ToSlash(result.ReportPath),
		Description: string(result.Kind) + " JSON report",
	}}
}

func kicadUnavailableIssue(err error, opts PostValidationOptions, path string) reports.Issue {
	severity := reports.SeverityWarning
	if opts.RequireKiCadERC || opts.RequireKiCadDRC || opts.RequireRoundTrip || !opts.AllowMissingKiCadChecks {
		severity = reports.SeverityError
	}
	return reports.Issue{
		Code:     reports.CodeSkippedExternalTool,
		Severity: severity,
		Path:     path,
		Message:  err.Error(),
	}
}

func kicadFindingSeverity(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func kicadFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	nets := []string{}
	add := func(net string) {
		net = strings.TrimSpace(net)
		if net == "" {
			return
		}
		if _, ok := seen[net]; ok {
			return
		}
		seen[net] = struct{}{}
		nets = append(nets, net)
	}
	for _, net := range finding.Nets {
		add(net)
	}
	add(finding.Net)
	return nets
}
