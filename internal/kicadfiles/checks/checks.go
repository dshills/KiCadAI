package checks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

func RunERC(ctx context.Context, cli KiCadCLI, target string, opts Options) (CheckResult, error) {
	return runCheck(ctx, ExecRunner{}, cli, CheckKindERC, target, opts)
}

func RunDRC(ctx context.Context, cli KiCadCLI, target string, opts Options) (CheckResult, error) {
	return runCheck(ctx, ExecRunner{}, cli, CheckKindDRC, target, opts)
}

func RunERCWithRunner(ctx context.Context, runner Runner, cli KiCadCLI, target string, opts Options) (CheckResult, error) {
	return runCheck(ctx, runner, cli, CheckKindERC, target, opts)
}

func RunDRCWithRunner(ctx context.Context, runner Runner, cli KiCadCLI, target string, opts Options) (CheckResult, error) {
	return runCheck(ctx, runner, cli, CheckKindDRC, target, opts)
}

func runCheck(ctx context.Context, runner Runner, cli KiCadCLI, kind CheckKind, target string, opts Options) (CheckResult, error) {
	opts = opts.withDefaults()
	if strings.TrimSpace(cli.Path) == "" {
		return CheckResult{}, fmt.Errorf("kicad-cli path is empty")
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	workspace, cleanup, err := NewArtifactWorkspace(string(kind), opts)
	if err != nil {
		return CheckResult{}, err
	}
	defer cleanup()

	prepared, err := prepareTarget(workspace, kind, target)
	if err != nil {
		return CheckResult{}, err
	}
	reportName := string(kind) + ".json"
	reportPath, err := workspace.Path(reportName)
	if err != nil {
		return CheckResult{}, err
	}
	versionCtx, cancelVersion := contextWithTimeout(ctx, 2*time.Second)
	version, versionErr := runner.Version(versionCtx, cli.Path)
	cancelVersion()
	if versionErr != nil {
		version = ""
	}
	args := checkArgs(kind, opts.Units, reportPath, prepared.CheckPath)
	runCtx, cancelRun := contextWithTimeout(ctx, opts.Timeout)
	result := runner.Run(runCtx, prepared.WorkingDir, cli.Path, args...)
	cancelRun()
	findings, parserIssues, units := parseReportIfPresent(kind, reportPath)
	if units == "" {
		units = opts.Units
	}
	remaining, allowed := FilterAllowedFindings(findings, opts.Allowlist)
	toolErrorKind := classifyToolError(result, parserIssues, findings, reportPath)
	toolError := toolErrorKind != ToolErrorNone
	status := ClassifyStatus(toolError, false, remaining, parserIssues)
	check := CheckResult{
		Kind:            kind,
		Status:          status,
		TargetPath:      filepath.ToSlash(target),
		FileType:        prepared.FileType,
		ProjectContext:  prepared.Context,
		Units:           units,
		KiCadCLIPath:    cli.Path,
		KiCadVersion:    version,
		Command:         append([]string{cli.Path}, args...),
		WorkingDir:      filepath.ToSlash(prepared.WorkingDir),
		ExitCode:        result.ExitCode,
		DurationMS:      result.Duration.Milliseconds(),
		ReportPath:      filepath.ToSlash(reportPath),
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ToolErrorKind:   toolErrorKind,
		Findings:        remaining,
		Allowed:         allowed,
		ParserIssues:    parserIssues,
		ContextWarnings: prepared.Warnings,
		Summary:         SummarizeFindings(remaining),
	}
	if toolError {
		return check, fmt.Errorf("%s check failed with exit code %d", kind, result.ExitCode)
	}
	return check, nil
}

func checkArgs(kind CheckKind, units string, reportPath string, inputPath string) []string {
	group := "sch"
	command := "erc"
	severity := "--severity-all"
	if kind == CheckKindDRC {
		group = "pcb"
		command = "drc"
		severity = "--severity-all"
	}
	return []string{
		group,
		command,
		"--format", "json",
		severity,
		"--exit-code-violations",
		"--units", units,
		"--output", reportPath,
		inputPath,
	}
}

func parseReportIfPresent(kind CheckKind, reportPath string) ([]CheckFinding, []ParserIssue, string) {
	if _, err := os.Stat(reportPath); err != nil {
		return nil, nil, ""
	}
	findings, issues, units, err := ParseReportFile(kind, reportPath)
	if err != nil {
		return nil, []ParserIssue{{Message: err.Error()}}, ""
	}
	return findings, issues, units
}

func classifyToolError(result CommandResult, parserIssues []ParserIssue, findings []CheckFinding, reportPath string) ToolErrorKind {
	if result.Err == nil {
		if len(parserIssues) > 0 {
			return ToolErrorReportParse
		}
		return ToolErrorNone
	}
	if result.ExitCode > 0 && len(parserIssues) == 0 && len(findings) > 0 {
		return ToolErrorNone
	}
	if len(parserIssues) > 0 {
		return ToolErrorReportParse
	}
	reportExists := reportFileExists(reportPath)
	hasOutput := hasNonWhitespace(result.Stdout) || hasNonWhitespace(result.Stderr)
	if len(findings) == 0 && !hasOutput && !reportExists {
		return ToolErrorNoOutputCrash
	}
	if len(findings) == 0 && !reportExists {
		return ToolErrorMissingReport
	}
	return ToolErrorCommandFailed
}

func reportFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	return !info.IsDir()
}

func hasNonWhitespace(value string) bool {
	for _, char := range value {
		if !unicode.IsSpace(char) {
			return true
		}
	}
	return false
}

func contextWithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

type preparedTarget struct {
	CheckPath  string
	WorkingDir string
	FileType   FileType
	Context    ProjectContext
	Warnings   []ContextWarning
}

func prepareTarget(workspace ArtifactWorkspace, kind CheckKind, target string) (preparedTarget, error) {
	info, err := os.Stat(target)
	if err != nil {
		return preparedTarget{}, err
	}
	if info.IsDir() {
		return prepareProjectTarget(workspace, kind, target)
	}
	fileType := FileTypeSchematic
	if kind == CheckKindDRC {
		fileType = FileTypePCB
	}
	copied, err := workspace.CopyFile(target, filepath.Base(target))
	if err != nil {
		return preparedTarget{}, err
	}
	prepared := preparedTarget{
		CheckPath:  copied,
		WorkingDir: filepath.Dir(copied),
		FileType:   fileType,
		Context:    ProjectContextStandalone,
	}
	if kind == CheckKindERC {
		prepared.Warnings = append(prepared.Warnings, ContextWarning{
			Code:    "STANDALONE_SCHEMATIC",
			Message: "ERC ran without full project context; results may differ from project-scoped ERC",
			Path:    filepath.ToSlash(target),
		})
	}
	return prepared, nil
}

func prepareProjectTarget(workspace ArtifactWorkspace, kind CheckKind, projectDir string) (preparedTarget, error) {
	target, err := discoverProjectCheckFile(projectDir, kind)
	if err != nil {
		return preparedTarget{}, err
	}
	rootName := sanitizePathComponent(filepath.Base(filepath.Clean(projectDir)))
	if rootName == "" {
		rootName = "project"
	}
	if err := copyProjectContext(workspace, projectDir, rootName); err != nil {
		return preparedTarget{}, err
	}
	rel, err := filepath.Rel(projectDir, target)
	if err != nil {
		return preparedTarget{}, err
	}
	checkPath, err := workspace.Path(rootName, rel)
	if err != nil {
		return preparedTarget{}, err
	}
	fileType := FileTypeSchematic
	if kind == CheckKindDRC {
		fileType = FileTypePCB
	}
	return preparedTarget{
		CheckPath:  checkPath,
		WorkingDir: filepath.Dir(checkPath),
		FileType:   fileType,
		Context:    ProjectContextFull,
	}, nil
}

func discoverProjectCheckFile(projectDir string, kind CheckKind) (string, error) {
	ext := ".kicad_sch"
	if kind == CheckKindDRC {
		ext = ".kicad_pcb"
	}
	var matches []string
	err := filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && shouldSkipProjectDir(d.Name()) && path != projectDir {
			return filepath.SkipDir
		}
		if !d.IsDir() && filepath.Ext(path) == ext {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no %s file found in %s", ext, projectDir)
	}
	base := filepath.Base(filepath.Clean(projectDir))
	for _, match := range matches {
		if strings.TrimSuffix(filepath.Base(match), ext) == base {
			return match, nil
		}
	}
	return matches[0], nil
}

func copyProjectContext(workspace ArtifactWorkspace, projectDir string, rootName string) error {
	return filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != projectDir && shouldSkipProjectDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldCopyProjectFile(path) {
			return nil
		}
		rel, err := filepath.Rel(projectDir, path)
		if err != nil {
			return err
		}
		_, err = workspace.CopyFile(path, filepath.Join(rootName, rel))
		return err
	})
}

func shouldSkipProjectDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".kicadai", "reports", "fabrication", "gerbers", "plot", "plots", "shapes3d", "backup", "backups":
		return true
	default:
		return false
	}
}

func shouldCopyProjectFile(path string) bool {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".kicad_pro", ".kicad_sch", ".kicad_pcb", ".kicad_sym", ".kicad_wks", ".kicad_prl", ".kicad_pri", ".kicad_dru", ".kicad_mod":
		return true
	}
	switch name {
	case "sym-lib-table", "fp-lib-table":
		return true
	default:
		return false
	}
}
