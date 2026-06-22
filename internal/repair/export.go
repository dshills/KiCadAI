package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/inspect"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type ExportOptions struct {
	TargetPath     string
	OutputPath     string
	StageIssues    []StageIssues
	Execute        bool
	Overwrite      bool
	Repair         Options
	Transaction    *transactions.Transaction
	InspectProject func(path string) (inspect.ProjectSummary, error)
}

type ExportResult struct {
	Target     Target             `json:"target"`
	BundlePath string             `json:"bundle_path"`
	DryRun     bool               `json:"dry_run"`
	Summary    ExportSummary      `json:"summary"`
	Issues     []reports.Issue    `json:"issues,omitempty"`
	Artifacts  []reports.Artifact `json:"artifacts,omitempty"`
}

type ExportSummary struct {
	StageCount     int  `json:"stage_count"`
	IssueCount     int  `json:"issue_count"`
	BlockingCount  int  `json:"blocking_count"`
	HasTransaction bool `json:"has_transaction"`
	Generated      bool `json:"generated"`
}

func ExportBundle(opts ExportOptions) (result ExportResult) {
	result = ExportResult{DryRun: !opts.Execute}
	repairOptions := normalizeRepairOptions(opts.Repair)
	repairOptions.Enabled = true
	repairOptions.Apply = true
	target := hydrateExportTarget(opts.TargetPath, opts)
	defer func() {
		result.Summary = exportSummary(opts.StageIssues, result.Issues, target.Generated, opts.Transaction != nil)
	}()
	result.Target = target
	if len(target.Issues) > 0 {
		result.Issues = append(result.Issues, target.Issues...)
	}
	if reports.HasBlockingIssue(result.Issues) {
		return result
	}
	if stageIssueCount(opts.StageIssues) == 0 {
		result.Issues = append(result.Issues, exportIssue(reports.CodeInvalidArgument, "repair.stage_issues", "repair export-bundle requires at least one stage issue"))
		return result
	}
	bundlePath, err := exportBundlePath(target.Root, opts.OutputPath)
	if err != nil {
		result.Issues = append(result.Issues, exportIssue(reports.CodeInvalidArgument, "output", err.Error()))
		return result
	}
	result.BundlePath = filepath.ToSlash(bundlePath)
	artifact := reports.Artifact{Kind: reports.ArtifactValidationReport, Path: result.BundlePath, Description: "repair bundle"}
	result.Artifacts = append(result.Artifacts, artifact)
	if err := ensureExportPathInsideRoot(target.Root, bundlePath); err != nil {
		result.Issues = append(result.Issues, exportIssue(reports.CodeInvalidArgument, "output", err.Error()))
		return result
	}
	if info, err := os.Stat(bundlePath); err == nil {
		if info.IsDir() {
			result.Issues = append(result.Issues, exportIssue(reports.CodeInvalidArgument, "output", "repair bundle output path is a directory"))
			return result
		}
		if !opts.Overwrite {
			result.Issues = append(result.Issues, exportIssue(reports.CodeInvalidArgument, "overwrite", "existing repair bundle requires overwrite=true"))
			return result
		}
	} else if err != nil && !os.IsNotExist(err) {
		result.Issues = append(result.Issues, exportIssue(reports.CodeValidationFailed, "output", err.Error()))
		return result
	}
	if !opts.Execute {
		return result
	}
	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
		result.Issues = append(result.Issues, exportIssue(reports.CodeValidationFailed, "output", err.Error()))
		return result
	}
	bundle := Bundle{
		Schema:        BundleSchemaV1,
		ProjectRoot:   target.Root,
		ProjectName:   exportProjectName(target),
		Generated:     true,
		Transaction:   opts.Transaction,
		StageIssues:   opts.StageIssues,
		RepairOptions: repairOptions,
	}
	if err := SaveBundle(bundlePath, bundle); err != nil {
		result.Issues = append(result.Issues, exportIssue(reports.CodeValidationFailed, "repair.bundle", err.Error()))
	}
	return result
}

func hydrateExportTarget(path string, opts ExportOptions) Target {
	path = strings.TrimSpace(path)
	target := Target{Path: filepath.ToSlash(path), Kind: TargetUnknown}
	if target.Path == "" {
		target.Issues = append(target.Issues, exportIssue(reports.CodeInvalidArgument, "target", "repair export-bundle requires --target"))
		return target
	}
	info, err := os.Stat(path)
	if err != nil {
		target.Issues = append(target.Issues, exportIssue(reports.CodeMissingFile, "target", err.Error()))
		return target
	}
	root := path
	if info.IsDir() {
		target.Kind = TargetProjectDir
	} else {
		target.Kind = targetKindForPath(path)
		root = filepath.Dir(path)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		target.Issues = append(target.Issues, exportIssue(reports.CodeInvalidArgument, "target", err.Error()))
		return target
	}
	target.Root = filepath.ToSlash(absRoot)
	inspector := opts.InspectProject
	if inspector == nil {
		inspector = inspect.Project
	}
	summary, err := inspector(absRoot)
	if err != nil {
		target.Issues = append(target.Issues, exportIssue(reports.CodeValidationFailed, "target.inspect", err.Error()))
		return target
	}
	target.Inspection = &summary
	target.Issues = append(target.Issues, summary.Issues...)
	if hasUnsupportedContent(summary) {
		target.Issues = append(target.Issues, reports.Issue{
			Code:     reports.CodePreservationConflict,
			Severity: reports.SeverityBlocked,
			Path:     "target",
			Message:  "target contains preserved unsupported content; repair bundle export is blocked",
		})
	}
	target.Generated = summary.Manifest.Present && !summary.Manifest.Stale
	if !target.Generated {
		target.Issues = append(target.Issues, reports.Issue{
			Code:     reports.CodePreservationConflict,
			Severity: reports.SeverityBlocked,
			Path:     "target.generated",
			Message:  "repair export-bundle requires generated KiCadAI provenance",
		})
	}
	target.Mutable = false
	return target
}

func exportBundlePath(root string, output string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("target root is required")
	}
	if strings.TrimSpace(output) == "" {
		return filepath.Join(filepath.FromSlash(root), ".kicadai", "repair-bundle.json"), nil
	}
	if filepath.IsAbs(output) {
		return filepath.Clean(output), nil
	}
	return filepath.Abs(output)
}

func ensureExportPathInsideRoot(root string, output string) error {
	absRoot, err := filepath.Abs(filepath.FromSlash(root))
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absOutput)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repair bundle output must be inside target root")
	}
	if rel == "." {
		return fmt.Errorf("repair bundle output cannot be the target root directory")
	}
	return nil
}

func exportSummary(groups []StageIssues, globalIssues []reports.Issue, generated bool, hasTransaction bool) ExportSummary {
	summary := ExportSummary{StageCount: len(groups), Generated: generated, HasTransaction: hasTransaction}
	for _, group := range groups {
		for _, issue := range group.Issues {
			summary.IssueCount++
			if issue.Blocking() {
				summary.BlockingCount++
			}
		}
	}
	globalBlocking := 0
	for _, issue := range globalIssues {
		summary.IssueCount++
		if issue.Blocking() {
			summary.BlockingCount++
			globalBlocking++
		}
	}
	if globalBlocking > 0 {
		summary.Generated = false
	}
	return summary
}

func stageIssueCount(groups []StageIssues) int {
	count := 0
	for _, group := range groups {
		count += len(group.Issues)
	}
	return count
}

func exportProjectName(target Target) string {
	if target.Inspection != nil && strings.TrimSpace(target.Inspection.Name) != "" {
		return strings.TrimSpace(target.Inspection.Name)
	}
	root := filepath.FromSlash(target.Root)
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

func exportIssue(code reports.Code, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityBlocked, Path: path, Message: message}
}
