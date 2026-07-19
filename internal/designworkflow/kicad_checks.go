package designworkflow

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reportartifacts"
	"kicadai/internal/reports"
)

type KiCadCheckOptions struct {
	KiCadCLI   string
	Timeout    time.Duration
	RequireERC bool
	RequireDRC bool
	// EnforceRequirements makes a missing KiCad CLI fatal. When false, a
	// request-level ERC/DRC requirement remains pending external evidence.
	EnforceRequirements bool
	KeepArtifacts       bool
	ArtifactDir         string
	Allowlist           []checks.AllowlistEntry
}

type KiCadCheckStageResult struct {
	ERC   checks.CheckResult `json:"erc,omitempty"`
	DRC   checks.CheckResult `json:"drc,omitempty"`
	Stage StageResult        `json:"stage"`
}

func RunKiCadChecks(ctx context.Context, request *Request, write *ProjectWriteResult, opts KiCadCheckOptions) KiCadCheckStageResult {
	if ctx == nil {
		return KiCadCheckStageResult{Stage: NewStageResult(StageKiCadChecks, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  "context is required",
		}})}
	}
	if write == nil {
		return KiCadCheckStageResult{Stage: NewStageResult(StageKiCadChecks, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "project_write",
			Message:  "project write result is required",
		}})}
	}
	if write.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(write.Stage.Issues) {
		return KiCadCheckStageResult{Stage: StageResult{Name: StageKiCadChecks, Status: StageStatusSkipped, Summary: map[string]any{"reason": "project write did not complete"}}}
	}
	opts = mergeKiCadCheckOptions(request, opts)
	if !opts.RequireERC && !opts.RequireDRC {
		return KiCadCheckStageResult{Stage: StageResult{Name: StageKiCadChecks, Status: StageStatusSkipped, Summary: map[string]any{"reason": "kicad checks not required"}}}
	}
	if opts.Timeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		ctx = timeoutCtx
	}
	cli, err := checks.DiscoverCLI(opts.KiCadCLI)
	if err != nil {
		severity := reports.SeverityWarning
		if opts.EnforceRequirements {
			severity = reports.SeverityBlocked
		}
		stage := NewStageResult(StageKiCadChecks, []reports.Issue{{
			Code:       reports.CodeSkippedExternalTool,
			Severity:   severity,
			Path:       "kicad_cli",
			Message:    err.Error(),
			Suggestion: "set --kicad-cli or KICADAI_KICAD_CLI to run KiCad ERC/DRC checks",
		}})
		stage.Summary = map[string]any{"reason": "kicad-cli unavailable"}
		if !opts.EnforceRequirements {
			stage.Status = StageStatusSkipped
		}
		return KiCadCheckStageResult{Stage: stage}
	}

	checkOpts := checks.Options{
		KiCadCLI:      cli.Path,
		Timeout:       opts.Timeout,
		KeepArtifacts: opts.KeepArtifacts,
		ArtifactDir:   opts.ArtifactDir,
		Allowlist:     opts.Allowlist,
	}
	var result KiCadCheckStageResult
	var issues []reports.Issue
	var artifacts []reports.Artifact
	var ercResult checks.CheckResult
	var drcResult checks.CheckResult
	var ercIssues []reports.Issue
	var drcIssues []reports.Issue
	var ercArtifacts []reports.Artifact
	var drcArtifacts []reports.Artifact
	if opts.RequireERC {
		target := kicadCheckTargetFromWrite(write, checks.CheckKindERC)
		if target == "" {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityBlocked,
				Path:     "schematic",
				Message:  "schematic path or project root is required for ERC",
			})
		} else {
			ercResult, ercIssues, ercArtifacts = runWorkflowERC(ctx, cli, target, checkOpts)
		}
	}
	if opts.RequireDRC {
		target := kicadCheckTargetFromWrite(write, checks.CheckKindDRC)
		if target == "" {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityBlocked,
				Path:     "pcb",
				Message:  "PCB path or project root is required for DRC",
			})
		} else {
			drcResult, drcIssues, drcArtifacts = runWorkflowDRC(ctx, cli, target, checkOpts)
		}
	}
	result.ERC = ercResult
	result.DRC = drcResult
	issues = append(issues, ercIssues...)
	issues = append(issues, drcIssues...)
	artifacts = append(artifacts, ercArtifacts...)
	artifacts = append(artifacts, drcArtifacts...)
	stage := NewStageResult(StageKiCadChecks, issues)
	stage.Artifacts = artifacts
	stage.Summary = map[string]any{
		"erc_required":   opts.RequireERC,
		"drc_required":   opts.RequireDRC,
		"artifact_count": len(artifacts),
	}
	if opts.RequireERC {
		stage.Summary[promotionKiCadERCSummaryKey] = result.ERC
	}
	if opts.RequireDRC {
		stage.Summary[promotionKiCadDRCSummaryKey] = result.DRC
	}
	result.Stage = stage
	return result
}

// kicadCheckTargetFromWrite prefers the project root because checks.RunERC and
// checks.RunDRC intentionally accept project directories, copy project context,
// discover the matching .kicad_sch/.kicad_pcb, then pass that file to KiCad.
func kicadCheckTargetFromWrite(write *ProjectWriteResult, kind checks.CheckKind) string {
	if write == nil {
		return ""
	}
	if root := strings.TrimSpace(write.Inspection.Root); root != "" {
		return root
	}
	if kind == checks.CheckKindDRC {
		return pcbPathFromWrite(write)
	}
	return schematicPathFromWrite(write)
}

func schematicPathFromWrite(write *ProjectWriteResult) string {
	if write == nil {
		return ""
	}
	inspection := write.Inspection
	if inspection.Schematic == nil {
		return ""
	}
	return inspection.Schematic.Path
}

func pcbPathFromWrite(write *ProjectWriteResult) string {
	if write == nil {
		return ""
	}
	inspection := write.Inspection
	if inspection.PCB == nil {
		return ""
	}
	return inspection.PCB.Path
}

func mergeKiCadCheckOptions(request *Request, opts KiCadCheckOptions) KiCadCheckOptions {
	if request != nil {
		opts.RequireERC = opts.RequireERC || request.Validation.RequireERC
		opts.RequireDRC = opts.RequireDRC || request.Validation.RequireDRC
	}
	return opts
}

func runWorkflowERC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	if strings.TrimSpace(target) == "" {
		return checks.CheckResult{}, []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityBlocked,
			Path:     "schematic",
			Message:  "schematic path is required for ERC",
		}}, nil
	}
	result, err := checks.RunERC(ctx, cli, target, opts)
	return workflowCheckResultWithIssues(result, err)
}

func runWorkflowDRC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	if strings.TrimSpace(target) == "" {
		return checks.CheckResult{}, []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityBlocked,
			Path:     "pcb",
			Message:  "PCB path is required for DRC",
		}}, nil
	}
	result, err := checks.RunDRC(ctx, cli, target, opts)
	return workflowCheckResultWithIssues(result, err)
}

func workflowCheckResultWithIssues(result checks.CheckResult, err error) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	issues := []reports.Issue{}
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   workflowCheckSeverity(finding.Severity),
			Path:       filepath.ToSlash(finding.File),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       workflowFindingNets(finding),
			Suggestion: "repair category: " + string(finding.RepairCategory),
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: result.ReportPath, Message: parserIssue.Message})
	}
	if result.ToolErrorKind != checks.ToolErrorNone {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeKiCadCLIFailed,
			Severity:   workflowCheckToolErrorSeverity(result),
			Path:       result.TargetPath,
			Message:    workflowCheckToolErrorMessage(result, err),
			Suggestion: workflowCheckToolErrorSuggestion(result),
		})
	}
	return result, issues, workflowCheckArtifacts(result)
}

func workflowCheckToolErrorSeverity(result checks.CheckResult) reports.Severity {
	if workflowCheckToolErrorHasNoFindingDRCInstability(result) {
		return reports.SeverityWarning
	}
	return reports.SeverityError
}

func workflowCheckToolErrorHasNoFindingDRCInstability(result checks.CheckResult) bool {
	return result.Kind == checks.CheckKindDRC &&
		result.ToolErrorKind == checks.ToolErrorNoOutputCrash
}

func workflowCheckToolErrorMessage(result checks.CheckResult, err error) string {
	if result.ToolErrorKind == checks.ToolErrorReportParse {
		if err != nil {
			return string(result.Kind) + " check produced an unparseable KiCad report: " + err.Error()
		}
		return string(result.Kind) + " check produced an unparseable KiCad report"
	}
	if err != nil {
		return string(result.Kind) + " check failed: " + err.Error()
	}
	return string(result.Kind) + " check failed"
}

func workflowCheckToolErrorSuggestion(result checks.CheckResult) string {
	if workflowCheckToolErrorHasNoFindingDRCInstability(result) {
		return "KiCad " + string(result.Kind) + " exited before writing a report; verify kicad-cli outside this process or use a stable KiCad CLI before requiring pass evidence"
	}
	if result.ToolErrorKind == checks.ToolErrorMissingReport {
		return "KiCad CLI failed without producing the requested report artifact; rerun with --keep-artifacts and inspect stdout/stderr"
	}
	if result.ToolErrorKind == checks.ToolErrorReportParse {
		return "KiCad CLI produced a report that KiCadAI could not parse; keep the report artifact and update the parser for this KiCad report shape"
	}
	return "Inspect KiCad CLI stdout/stderr and rerun the same command outside KiCadAI to separate design findings from tool/runtime failures"
}

func workflowCheckSeverity(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func workflowFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	nets := make([]string, 0, len(finding.Nets)+1)
	for _, net := range finding.Nets {
		addWorkflowFindingNet(&nets, seen, net)
	}
	addWorkflowFindingNet(&nets, seen, finding.Net)
	return nets
}

func addWorkflowFindingNet(nets *[]string, seen map[string]struct{}, net string) {
	net = strings.TrimSpace(net)
	if net == "" {
		return
	}
	if _, ok := seen[net]; ok {
		return
	}
	seen[net] = struct{}{}
	*nets = append(*nets, net)
}

func workflowCheckArtifacts(result checks.CheckResult) []reports.Artifact {
	kind := reports.ArtifactERCReport
	if result.Kind == checks.CheckKindDRC {
		kind = reports.ArtifactDRCReport
	}
	return reportartifacts.ExistingReportArtifact(kind, result.ReportPath, string(result.Kind)+" JSON report")
}
