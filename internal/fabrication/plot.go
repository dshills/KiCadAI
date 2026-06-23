package fabrication

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/reports"
)

const maxPlotCommandOutputBytes = 64 * 1024

type PlotKind string

const (
	PlotKindGerber PlotKind = "gerber"
	PlotKindDrill  PlotKind = "drill"
)

type PlotRequest struct {
	ProjectRoot     string    `json:"project_root"`
	ProjectName     string    `json:"project_name,omitempty"`
	PCBPath         string    `json:"pcb_path"`
	PackageDir      string    `json:"package_dir"`
	GerberDir       string    `json:"gerber_dir"`
	DrillDir        string    `json:"drill_dir"`
	Execute         bool      `json:"execute"`
	Overwrite       bool      `json:"overwrite"`
	KiCadCLI        string    `json:"kicad_cli,omitempty"`
	CLIPolicy       CLIPolicy `json:"cli_policy,omitempty"`
	PackageDirLabel string    `json:"package_dir_label,omitempty"`
}

type PlotCommand struct {
	Kind      PlotKind `json:"kind"`
	Argv      []string `json:"argv"`
	OutputDir string   `json:"output_dir"`
}

type PlotCommandEvidence struct {
	Kind           PlotKind `json:"kind"`
	Argv           []string `json:"argv,omitempty"`
	OutputDir      string   `json:"output_dir,omitempty"`
	ExitCode       int      `json:"exit_code,omitempty"`
	StdoutSnippet  string   `json:"stdout_snippet,omitempty"`
	StderrSnippet  string   `json:"stderr_snippet,omitempty"`
	GeneratedPaths []string `json:"generated_paths,omitempty"`
	SkippedReason  string   `json:"skipped_reason,omitempty"`
}

type PlotResult struct {
	Attempted      bool                  `json:"attempted"`
	SkippedReason  string                `json:"skipped_reason,omitempty"`
	GeneratedPaths []string              `json:"generated_paths,omitempty"`
	Commands       []PlotCommandEvidence `json:"commands,omitempty"`
	Issues         []reports.Issue       `json:"issues,omitempty"`
}

type PlotRunner interface {
	RunPlotCommand(ctx context.Context, command PlotCommand) PlotCommandEvidence
}

type ExecPlotRunner struct{}

func PlotFabricationOutputs(ctx context.Context, request PlotRequest, runner PlotRunner) PlotResult {
	normalized, issues := normalizePlotRequest(request)
	result := PlotResult{Issues: issues}
	if len(issues) > 0 {
		return finalizePlotResult(result)
	}
	if !normalized.Execute {
		result.SkippedReason = "dry_run"
		result.Commands = append(result.Commands,
			skippedPlotEvidence(PlotKindGerber, normalized.GerberDir, "dry_run"),
			skippedPlotEvidence(PlotKindDrill, normalized.DrillDir, "dry_run"),
		)
		return finalizePlotResult(result)
	}
	if strings.TrimSpace(normalized.KiCadCLI) == "" {
		result.SkippedReason = "missing_kicad_cli"
		result.Issues = append(result.Issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       "fabrication.kicad_cli",
			Message:    "KiCad CLI is required to generate Gerber and drill artifacts",
			Suggestion: "rerun export fabrication with --kicad-cli and --execute",
		})
		result.Commands = append(result.Commands,
			skippedPlotEvidence(PlotKindGerber, normalized.GerberDir, "missing_kicad_cli"),
			skippedPlotEvidence(PlotKindDrill, normalized.DrillDir, "missing_kicad_cli"),
		)
		return finalizePlotResult(result)
	}
	if runner == nil {
		runner = ExecPlotRunner{}
	}
	for _, dir := range []struct {
		path string
		name string
	}{
		{path: normalized.GerberDir, name: "fabrication/gerbers"},
		{path: normalized.DrillDir, name: "fabrication/drill"},
	} {
		if issue := preparePlotOutputDir(dir.path, dir.name, normalized.PackageDir, normalized.Overwrite); issue != nil {
			result.Issues = append(result.Issues, *issue)
		}
	}
	if len(result.Issues) > 0 {
		return finalizePlotResult(result)
	}
	commands := []PlotCommand{
		{
			Kind:      PlotKindGerber,
			Argv:      gerberPlotArgv(normalized),
			OutputDir: normalized.GerberDir,
		},
		{
			Kind:      PlotKindDrill,
			Argv:      drillPlotArgv(normalized),
			OutputDir: normalized.DrillDir,
		},
	}
	if err := ctx.Err(); err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "fabrication/plot", Message: err.Error()})
		return finalizePlotResult(result)
	}
	result.Attempted = true
	evidenceByIndex := make([]PlotCommandEvidence, len(commands))
	var wg sync.WaitGroup
	launched := 0
	for index, command := range commands {
		if err := ctx.Err(); err != nil {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "fabrication/plot", Message: err.Error()})
			break
		}
		launched++
		wg.Add(1)
		go func(index int, command PlotCommand) {
			defer wg.Done()
			evidenceByIndex[index] = runner.RunPlotCommand(ctx, command)
		}(index, command)
	}
	wg.Wait()
	for index, command := range commands[:launched] {
		evidence := evidenceByIndex[index]
		evidence.GeneratedPaths = normalizeGeneratedPlotPaths(normalized.PackageDir, evidence.GeneratedPaths)
		result.Commands = append(result.Commands, evidence)
		result.GeneratedPaths = append(result.GeneratedPaths, evidence.GeneratedPaths...)
		if evidence.ExitCode != 0 {
			message := fmt.Sprintf("KiCad CLI %s export failed", command.Kind)
			if strings.TrimSpace(evidence.StderrSnippet) != "" {
				message += ": " + evidence.StderrSnippet
			}
			result.Issues = append(result.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     plotIssuePath(command.Kind),
				Message:  message,
			})
		}
	}
	return finalizePlotResult(result)
}

func plotIssuePath(kind PlotKind) string {
	switch kind {
	case PlotKindGerber:
		return "fabrication/gerbers"
	case PlotKindDrill:
		return "fabrication/drill"
	default:
		return "fabrication/plot"
	}
}

func skippedPlotEvidence(kind PlotKind, outputDir string, reason string) PlotCommandEvidence {
	return PlotCommandEvidence{Kind: kind, OutputDir: outputDir, SkippedReason: reason}
}

func gerberPlotArgv(request PlotRequest) []string {
	return []string{
		request.KiCadCLI,
		"pcb",
		"export",
		"gerbers",
		"--output",
		request.GerberDir,
		request.PCBPath,
	}
}

func drillPlotArgv(request PlotRequest) []string {
	return []string{
		request.KiCadCLI,
		"pcb",
		"export",
		"drill",
		"--output",
		request.DrillDir,
		request.PCBPath,
	}
}

func preparePlotOutputDir(path string, label string, packageDir string, overwrite bool) *reports.Issue {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: "fabrication output path exists and is not a directory"}
		}
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: readErr.Error()}
		}
		if len(entries) > 0 && !overwrite {
			return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: "overwrite is required to replace existing fabrication output"}
		}
		if overwrite {
			if err := validateRemovablePlotDir(path, packageDir); err != nil {
				return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: err.Error()}
			}
			if removeErr := os.RemoveAll(path); removeErr != nil {
				return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: removeErr.Error()}
			}
		}
	} else if !os.IsNotExist(err) {
		return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: err.Error()}
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: label, Message: err.Error()}
	}
	return nil
}

func validateRemovablePlotDir(path string, packageDir string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolutePackage, err := filepath.Abs(packageDir)
	if err != nil {
		return err
	}
	cleaned := filepath.Clean(absolute)
	parent := filepath.Dir(cleaned)
	if cleaned == parent {
		return fmt.Errorf("refusing to remove unsafe fabrication output path")
	}
	if !pathInside(filepath.Clean(absolutePackage), cleaned) {
		return fmt.Errorf("refusing to remove fabrication output outside package directory")
	}
	base := filepath.Base(cleaned)
	if base != "gerbers" && base != "drill" {
		return fmt.Errorf("refusing to remove non-fabrication output directory")
	}
	return nil
}

func normalizeGeneratedPlotPaths(packageDir string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := packageRelativePath(packageDir, path)
		if err != nil {
			out = append(out, filepath.ToSlash(filepath.Base(path)))
			continue
		}
		out = append(out, rel)
	}
	slices.Sort(out)
	return out
}

func finalizePlotResult(result PlotResult) PlotResult {
	result.Issues = dedupeIssues(result.Issues)
	slices.SortFunc(result.Issues, compareIssues)
	slices.Sort(result.GeneratedPaths)
	return result
}

func (ExecPlotRunner) RunPlotCommand(ctx context.Context, command PlotCommand) PlotCommandEvidence {
	evidence := PlotCommandEvidence{
		Kind:      command.Kind,
		Argv:      slices.Clone(command.Argv),
		OutputDir: command.OutputDir,
	}
	if len(command.Argv) == 0 {
		evidence.ExitCode = -1
		evidence.StderrSnippet = "missing command"
		return evidence
	}
	if !allowedKiCadCLIExecutable(command.Argv[0]) {
		evidence.ExitCode = -1
		evidence.StderrSnippet = "plot command executable must be kicad-cli"
		return evidence
	}
	beforeFiles, beforeErr := scanPlotFileStates(command.OutputDir)
	cmd := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	cmd.Dir = plotCommandWorkingDir(command.OutputDir)
	stdout := &limitedBuffer{Limit: maxPlotCommandOutputBytes}
	stderr := &limitedBuffer{Limit: maxPlotCommandOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	evidence.StdoutSnippet = commandSnippet(stdout.String())
	stderrText := stderr.String()
	afterFiles, afterErr := scanPlotFileStates(command.OutputDir)
	if beforeErr == nil {
		evidence.GeneratedPaths = changedPlotPaths(beforeFiles, afterFiles)
	}
	if err != nil {
		errorText := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			evidence.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			evidence.ExitCode = -1
			errorText = ctx.Err().Error()
		} else {
			evidence.ExitCode = -1
			errorText = err.Error()
		}
		if errorText != "" {
			stderrText = joinCommandError(stderrText, errorText)
		}
	}
	if beforeErr != nil || afterErr != nil {
		if err == nil {
			evidence.ExitCode = -1
		}
		stderrText = joinCommandError(stderrText, firstErrorText(beforeErr, afterErr))
	}
	evidence.StderrSnippet = commandSnippet(stderrText)
	return evidence
}

type FakePlotRunner struct {
	Evidence []PlotCommandEvidence
	Calls    []PlotCommand
	mu       sync.Mutex
}

func (runner *FakePlotRunner) RunPlotCommand(ctx context.Context, command PlotCommand) PlotCommandEvidence {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.Calls = append(runner.Calls, PlotCommand{
		Kind:      command.Kind,
		Argv:      slices.Clone(command.Argv),
		OutputDir: command.OutputDir,
	})
	if ctx.Err() != nil {
		return PlotCommandEvidence{
			Kind:          command.Kind,
			Argv:          slices.Clone(command.Argv),
			OutputDir:     command.OutputDir,
			ExitCode:      -1,
			StderrSnippet: commandSnippet(ctx.Err().Error()),
		}
	}
	if len(runner.Evidence) == 0 {
		return PlotCommandEvidence{
			Kind:      command.Kind,
			Argv:      slices.Clone(command.Argv),
			OutputDir: command.OutputDir,
		}
	}
	evidence := runner.Evidence[0]
	runner.Evidence = runner.Evidence[1:]
	if len(evidence.Argv) == 0 {
		evidence.Argv = slices.Clone(command.Argv)
	}
	if strings.TrimSpace(evidence.OutputDir) == "" {
		evidence.OutputDir = command.OutputDir
	}
	if evidence.Kind == "" {
		evidence.Kind = command.Kind
	}
	evidence.GeneratedPaths = slices.Clone(evidence.GeneratedPaths)
	return evidence
}

func joinCommandError(stderr string, errText string) string {
	stderr = strings.TrimSpace(stderr)
	errText = strings.TrimSpace(errText)
	switch {
	case stderr == "":
		return errText
	case errText == "":
		return stderr
	default:
		return stderr + "\nexecution error: " + errText
	}
}

func commandSnippet(value string) string {
	trimmed := strings.TrimSpace(value)
	count := 0
	for index := range trimmed {
		if count == 500 {
			return trimmed[:index]
		}
		count++
	}
	return trimmed
}

type limitedBuffer struct {
	Limit     int
	buf       bytes.Buffer
	truncated bool
	mu        sync.Mutex
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.Limit <= 0 {
		_, _ = buffer.buf.Write(data)
		return len(data), nil
	}
	remaining := buffer.Limit - buffer.buf.Len()
	if remaining > 0 {
		if len(data) <= remaining {
			_, _ = buffer.buf.Write(data)
		} else {
			_, _ = buffer.buf.Write(data[:remaining])
			buffer.truncated = true
		}
	} else {
		buffer.truncated = true
	}
	return len(data), nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if !buffer.truncated {
		return strings.ToValidUTF8(buffer.buf.String(), "")
	}
	return strings.ToValidUTF8(buffer.buf.String(), "") + "\n[output truncated]"
}

type plotFileState struct {
	Size    int64
	ModUnix int64
}

func scanPlotPaths(outputDir string) ([]string, error) {
	files, err := scanPlotFileStates(outputDir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths, nil
}

func scanPlotFileStates(outputDir string) (map[string]plotFileState, error) {
	trimmed := strings.TrimSpace(outputDir)
	if trimmed == "" {
		return nil, nil
	}
	info, err := os.Stat(trimmed)
	if err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s is not a directory", filepath.ToSlash(trimmed))
	}
	var filePaths []string
	err = filepath.WalkDir(trimmed, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		filePaths = append(filePaths, filepath.Clean(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return statPlotFiles(filePaths), nil
}

func statPlotFiles(filePaths []string) map[string]plotFileState {
	out := make(map[string]plotFileState, len(filePaths))
	if len(filePaths) == 0 {
		return out
	}
	workers := 4
	if len(filePaths) < workers {
		workers = len(filePaths)
	}
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				mu.Lock()
				out[path] = plotFileState{Size: info.Size(), ModUnix: info.ModTime().UnixNano()}
				mu.Unlock()
			}
		}()
	}
	for _, path := range filePaths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()
	return out
}

func changedPlotPaths(before map[string]plotFileState, after map[string]plotFileState) []string {
	var out []string
	for path, afterState := range after {
		beforeState, ok := before[path]
		if ok && beforeState == afterState {
			continue
		}
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func firstErrorText(errs ...error) string {
	for _, err := range errs {
		if err != nil {
			return err.Error()
		}
	}
	return ""
}

func allowedKiCadCLIExecutable(value string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(value)))
	return base == "kicad-cli" || base == "kicad-cli.exe" || strings.HasPrefix(base, "kicad-cli-")
}

func plotCommandWorkingDir(outputDir string) string {
	trimmed := strings.TrimSpace(outputDir)
	if trimmed == "" {
		return ""
	}
	if info, err := os.Stat(trimmed); err == nil && info.IsDir() {
		return trimmed
	}
	parent := filepath.Dir(trimmed)
	if info, err := os.Stat(parent); err == nil && info.IsDir() {
		return parent
	}
	return ""
}

func normalizePlotRequest(request PlotRequest) (PlotRequest, []reports.Issue) {
	normalized := request
	var issues []reports.Issue
	root, issue := requiredAbsDirLikePath("project_root", request.ProjectRoot)
	if issue != nil {
		issues = append(issues, *issue)
	} else {
		normalized.ProjectRoot = root
	}
	pcb, issue := requiredAbsDirLikePath("pcb_path", request.PCBPath)
	if issue != nil {
		issues = append(issues, *issue)
	} else {
		normalized.PCBPath = pcb
	}
	packageDir, issue := requiredAbsDirLikePath("package_dir", request.PackageDir)
	if issue != nil {
		issues = append(issues, *issue)
	} else {
		normalized.PackageDir = packageDir
	}
	gerberDir, issue := requiredAbsDirLikePath("gerber_dir", request.GerberDir)
	if issue != nil {
		issues = append(issues, *issue)
	} else {
		normalized.GerberDir = gerberDir
	}
	drillDir, issue := requiredAbsDirLikePath("drill_dir", request.DrillDir)
	if issue != nil {
		issues = append(issues, *issue)
	} else {
		normalized.DrillDir = drillDir
	}
	if len(issues) > 0 {
		slices.SortFunc(issues, compareIssues)
		return normalized, issues
	}
	for _, check := range []struct {
		name string
		path string
	}{
		{name: "package_dir", path: normalized.PackageDir},
		{name: "gerber_dir", path: normalized.GerberDir},
		{name: "drill_dir", path: normalized.DrillDir},
	} {
		if !pathInside(normalized.ProjectRoot, check.path) {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     check.name,
				Message:  "path must be inside project root",
			})
		}
	}
	slices.SortFunc(issues, compareIssues)
	return normalized, issues
}

func requiredAbsDirLikePath(field string, value string) (string, *reports.Issue) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: field, Message: field + " is required"}
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: field, Message: err.Error()}
	}
	return filepath.Clean(absolute), nil
}

func pathInside(root string, value string) bool {
	rel, err := filepath.Rel(root, value)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func packageRelativePath(packageDir string, value string) (string, error) {
	if strings.TrimSpace(packageDir) == "" || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("package and artifact paths are required")
	}
	absPackage, err := filepath.Abs(packageDir)
	if err != nil {
		return "", err
	}
	absValue, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if !pathInside(absPackage, absValue) {
		return "", fmt.Errorf("artifact path must be inside package directory")
	}
	rel, err := filepath.Rel(absPackage, absValue)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
