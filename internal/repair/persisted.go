package repair

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"kicadai/internal/inspect"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type PersistedApplyOptions struct {
	Execute        bool
	OutputDir      string
	Overwrite      bool
	Seed           string
	Repair         Options
	Board          *transactions.BoardSize
	PlacementOps   []transactions.Operation
	RouteOps       []transactions.Operation
	Footprints     map[string]FootprintEvidence
	PadNets        []PadNetHint
	LibraryIndex   *libraryresolver.LibraryIndex
	LibraryIssues  []reports.Issue
	InspectProject func(path string) (inspect.ProjectSummary, error)
	PostValidation PostValidationOptions
	PostValidators []PostApplyValidator
	ZoneRefill     ZoneRefillRunner
}

type PersistedApplyResult struct {
	Status      Status                   `json:"status"`
	Target      Target                   `json:"target"`
	Repair      Result                   `json:"repair"`
	Budget      *BudgetSummary           `json:"budget,omitempty"`
	Apply       transactions.ApplyResult `json:"apply,omitempty"`
	Transaction transactions.Transaction `json:"transaction,omitempty"`
	Validation  []PostApplyValidation    `json:"validation,omitempty"`
	ZoneRefill  *ZoneRefillResult        `json:"zone_refill,omitempty"`
	Summary     ValidationSummary        `json:"summary,omitempty"`
	Delta       ValidationDelta          `json:"delta,omitempty"`
	Issues      []reports.Issue          `json:"issues,omitempty"`
	Artifacts   []reports.Artifact       `json:"artifacts,omitempty"`
}

type PostApplyValidation struct {
	Name      string             `json:"name"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
	Skipped   bool               `json:"skipped,omitempty"`
}

type PostApplyValidationContext struct {
	OutputDir   string
	Target      Target
	Transaction transactions.Transaction
	Apply       transactions.ApplyResult
}

type PostApplyValidator interface {
	ValidatePostApply(context.Context, PostApplyValidationContext) PostApplyValidation
}

type PostApplyValidatorFunc func(context.Context, PostApplyValidationContext) PostApplyValidation

func (fn PostApplyValidatorFunc) ValidatePostApply(ctx context.Context, validationCtx PostApplyValidationContext) PostApplyValidation {
	return fn(ctx, validationCtx)
}

var managedKiCadExtensions = map[string]struct{}{
	".kicad_pro": {},
	".kicad_sch": {},
	".kicad_pcb": {},
	".kicad_dru": {},
	".kicad_prl": {},
	".kicad_sym": {},
	".kicad_mod": {},
}

func ApplyPersistedBundle(targetPath string, bundle Bundle, opts PersistedApplyOptions) PersistedApplyResult {
	return applyPersistedBundle(context.Background(), targetPath, bundle, opts)
}

func applyPersistedBundle(ctx context.Context, targetPath string, bundle Bundle, opts PersistedApplyOptions) PersistedApplyResult {
	inspectProject := opts.InspectProject
	if inspectProject == nil {
		inspectProject = inspect.Project
	}
	target := HydrateTarget(targetPath, HydrateOptions{Bundle: &bundle, InspectProject: inspectProject})
	result := PersistedApplyResult{Status: StatusBlocked, Target: target}
	if len(target.Issues) > 0 {
		result.Issues = append(result.Issues, target.Issues...)
	}
	if reports.HasBlockingIssue(target.Issues) {
		return finalizePersistedResult(result)
	}
	if !opts.Execute {
		result.Issues = append(result.Issues, persistedIssue(reports.CodeInvalidArgument, "execute", "repair apply requires execute=true"))
		return finalizePersistedResult(result)
	}
	if !target.Mutable {
		if len(result.Issues) == 0 {
			result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "target", "target project is not mutable"))
		}
		return finalizePersistedResult(result)
	}
	if bundle.Transaction == nil {
		result.Issues = append(result.Issues, persistedIssue(reports.CodeInvalidArgument, "bundle.transaction", "repair bundle transaction is required"))
		return finalizePersistedResult(result)
	}
	tx := bundle.Transaction.Clone()
	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		outputDir = filepath.FromSlash(target.Root)
	}
	overwriteRequired, err := requiresOverwrite(outputDir)
	if err != nil {
		result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output", err.Error()))
		return finalizePersistedResult(result)
	}
	if overwriteRequired && !opts.Overwrite {
		result.Issues = append(result.Issues, persistedIssue(reports.CodeInvalidArgument, "overwrite", "existing project output requires overwrite=true"))
		return finalizePersistedResult(result)
	}
	repairOptions := opts.Repair
	if !repairOptions.Enabled {
		repairOptions = bundle.RepairOptions
	}
	repairOptions = normalizeRepairOptions(repairOptions)
	repairOptions.Enabled = true
	repairOptions.Apply = true
	executor := NewExecutor(ExecutionContext{
		Transaction:  &tx,
		Board:        opts.Board,
		PlacementOps: opts.PlacementOps,
		RouteOps:     opts.RouteOps,
		Footprints:   opts.Footprints,
		PadNets:      opts.PadNets,
	})
	validator := ValidatorFunc(func() []reports.Issue {
		return transactions.Validate(tx).Issues
	})
	if err := ctx.Err(); err != nil {
		result.Issues = appendIssues(result.Issues, []reports.Issue{contextIssue(err)})
		result.Status = StatusBlocked
		return finalizePersistedResult(result)
	}
	repairResult := NewRunner(repairOptions, executor, validator).RunContext(ctx, bundle.StageIssues)
	result.Repair = repairResult
	result.Budget = repairBudgetSummary(repairOptions, repairResult)
	result.Transaction = tx
	if err := ctx.Err(); err != nil {
		result.Issues = appendIssues(result.Issues, []reports.Issue{contextIssue(err)})
		result.Status = StatusBlocked
		return finalizePersistedResult(result)
	}
	if repairResult.Status != StatusRepaired && repairResult.Status != StatusPartial && repairResult.Status != StatusNotNeeded {
		result.Issues = append(result.Issues, repairResult.FinalIssues...)
		return finalizePersistedResult(result)
	}
	if validation := transactions.Validate(tx); reports.HasBlockingIssue(validation.Issues) {
		result.Issues = append(result.Issues, validation.Issues...)
		return finalizePersistedResult(result)
	}
	if err := ctx.Err(); err != nil {
		result.Issues = appendIssues(result.Issues, []reports.Issue{contextIssue(err)})
		result.Status = StatusBlocked
		return finalizePersistedResult(result)
	}
	applyResult, artifacts, issues := replayGeneratedTransaction(ctx, tx, outputDir, opts)
	applyIssues := append([]reports.Issue(nil), applyResult.Issues...)
	result.Artifacts = appendArtifacts(result.Artifacts, artifacts)
	result.Issues = appendIssues(result.Issues, issues, applyIssues)
	applyResult.Artifacts = nil
	applyResult.Issues = nil
	result.Apply = applyResult
	if zoneRefillValidation, ok := runRequestedZoneRefill(ctx, target, outputDir, opts); ok {
		result.Validation = append(result.Validation, zoneRefillValidation.PostApplyValidation())
		result.ZoneRefill = &zoneRefillValidation
		if reports.HasBlockingIssue(zoneRefillValidation.Issues) {
			collectPostValidationEvidence(&result, result.Validation)
			return finalizePersistedValidationResult(result, bundle.StageIssues)
		}
	}
	postValidators := append(BuiltInPostApplyValidators(opts.PostValidation), opts.PostValidators...)
	result.Validation = append(result.Validation, runPostApplyValidators(ctx, PostApplyValidationContext{
		OutputDir:   outputDir,
		Target:      target,
		Transaction: tx,
		Apply:       applyResult,
	}, postValidators)...)
	collectPostValidationEvidence(&result, result.Validation)
	return finalizePersistedValidationResult(result, bundle.StageIssues)
}

func finalizePersistedValidationResult(result PersistedApplyResult, stageIssues []StageIssues) PersistedApplyResult {
	result.Summary = SummarizePostValidation(result.Validation)
	result.Delta = CompareValidationIssues(flattenIssues(stageIssues), result.Issues)
	result.Status = statusFromValidationDelta(result.Delta)
	return finalizePersistedResult(result)
}

func collectPostValidationEvidence(result *PersistedApplyResult, validations []PostApplyValidation) {
	for _, validation := range validations {
		result.Artifacts = appendArtifacts(result.Artifacts, validation.Artifacts)
		result.Issues = appendIssues(result.Issues, validation.Issues)
	}
}

func runRequestedZoneRefill(ctx context.Context, target Target, outputDir string, opts PersistedApplyOptions) (ZoneRefillResult, bool) {
	zoneOpts := zoneRefillOptionsFromPostValidation(opts.PostValidation)
	if normalizeZoneRefillPolicy(zoneOpts.Policy) == ZoneRefillNever {
		return ZoneRefillResult{}, false
	}
	return RunZoneRefill(ctx, target, outputDir, zoneOpts, opts.ZoneRefill), true
}

func normalizeRepairOptions(opts Options) Options {
	defaults := DefaultOptions()
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaults.MaxAttempts
	}
	if opts.MaxAttemptsPerIssue <= 0 {
		opts.MaxAttemptsPerIssue = defaults.MaxAttemptsPerIssue
	}
	return opts
}

func repairBudgetSummary(opts Options, result Result) *BudgetSummary {
	opts = normalizeRepairOptions(opts)
	summary := BudgetSummary{
		MaxAttempts:         opts.MaxAttempts,
		MaxAttemptsPerIssue: opts.MaxAttemptsPerIssue,
		AttemptCount:        result.Summary.AttemptCount,
		Exhausted:           len(result.FinalIssues) > 0 && (result.Summary.AttemptCount >= opts.MaxAttempts || perIssueBudgetReached(result.Attempts, opts.MaxAttemptsPerIssue)),
	}
	return &summary
}

func perIssueBudgetReached(attempts []Attempt, maxAttemptsPerIssue int) bool {
	if maxAttemptsPerIssue <= 0 {
		return false
	}
	counts := map[string]int{}
	for _, attempt := range attempts {
		key := StableIssueKey(attempt.Issue)
		counts[key]++
		if counts[key] >= maxAttemptsPerIssue {
			return true
		}
	}
	return false
}

func ApplyPersistedBundleContext(ctx context.Context, targetPath string, bundle Bundle, opts PersistedApplyOptions) PersistedApplyResult {
	if err := ctx.Err(); err != nil {
		return contextBlockedPersistedResult(err)
	}
	return applyPersistedBundle(ctx, targetPath, bundle, opts)
}

func contextBlockedPersistedResult(err error) PersistedApplyResult {
	return PersistedApplyResult{
		Status: StatusBlocked,
		Issues: []reports.Issue{contextIssue(err)},
	}
}

func contextIssue(err error) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeOperationCanceled,
		Severity: reports.SeverityBlocked,
		Path:     "context",
		Message:  err.Error(),
	}
}

func runPostApplyValidators(ctx context.Context, validationCtx PostApplyValidationContext, validators []PostApplyValidator) []PostApplyValidation {
	validations := make([]PostApplyValidation, 0, len(validators)+1)
	txIssues := transactions.Validate(validationCtx.Transaction).Issues
	validations = append(validations, PostApplyValidation{Name: "transaction", Issues: txIssues})
	for _, validator := range validators {
		if err := ctx.Err(); err != nil {
			validations = append(validations, PostApplyValidation{Name: "context", Issues: []reports.Issue{contextIssue(err)}})
			break
		}
		if validator == nil {
			validations = append(validations, PostApplyValidation{Name: "optional", Skipped: true})
			continue
		}
		validation := validator.ValidatePostApply(ctx, validationCtx)
		if strings.TrimSpace(validation.Name) == "" {
			validation.Name = "post_apply"
		}
		validations = append(validations, validation)
	}
	return validations
}

func statusFromPostValidation(before []StageIssues, final []reports.Issue) Status {
	return statusFromValidationDelta(CompareValidationIssues(flattenIssues(before), final))
}

func statusFromValidationDelta(delta ValidationDelta) Status {
	if delta.After.IssueCount == 0 {
		return StatusRepaired
	}
	if delta.After.BlockingCount > 0 {
		if delta.After.BlockingCount >= delta.Before.BlockingCount {
			return StatusBlocked
		}
		return StatusPartial
	}
	return StatusPartial
}

func blockingIssueCount(issues []reports.Issue) int {
	count := 0
	for _, issue := range issues {
		if issue.Blocking() {
			count++
		}
	}
	return count
}

func appendArtifacts(base []reports.Artifact, artifacts []reports.Artifact) []reports.Artifact {
	if len(artifacts) == 0 {
		return base
	}
	out := make([]reports.Artifact, 0, len(base)+len(artifacts))
	out = append(out, base...)
	out = append(out, artifacts...)
	return out
}

func appendIssues(base []reports.Issue, groups ...[]reports.Issue) []reports.Issue {
	total := len(base)
	for _, group := range groups {
		total += len(group)
	}
	if total == len(base) {
		return base
	}
	out := make([]reports.Issue, 0, total)
	out = append(out, base...)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func replayGeneratedTransaction(ctx context.Context, tx transactions.Transaction, outputDir string, opts PersistedApplyOptions) (transactions.ApplyResult, []reports.Artifact, []reports.Issue) {
	if err := ctx.Err(); err != nil {
		return transactions.ApplyResult{}, nil, []reports.Issue{contextIssue(err)}
	}
	existing, err := existingProjectDir(outputDir)
	if err != nil {
		return transactions.ApplyResult{}, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	if !existing {
		if err := ctx.Err(); err != nil {
			return transactions.ApplyResult{}, nil, []reports.Issue{contextIssue(err)}
		}
		apply := transactions.Apply(tx, transactions.ApplyOptions{
			OutputDir:     outputDir,
			Overwrite:     opts.Overwrite,
			Seed:          opts.Seed,
			LibraryIndex:  opts.LibraryIndex,
			LibraryIssues: opts.LibraryIssues,
		})
		return apply, apply.Artifacts, nil
	}
	stage, err := createReplayStage(outputDir)
	if err != nil {
		return transactions.ApplyResult{}, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	defer os.RemoveAll(stage)
	if err := ctx.Err(); err != nil {
		return transactions.ApplyResult{}, nil, []reports.Issue{contextIssue(err)}
	}
	apply := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:     stage,
		Overwrite:     true,
		Seed:          opts.Seed,
		LibraryIndex:  opts.LibraryIndex,
		LibraryIssues: opts.LibraryIssues,
	})
	if reports.HasBlockingIssue(apply.Issues) {
		apply.Artifacts = nil
		return apply, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return apply, nil, []reports.Issue{contextIssue(err)}
	}
	artifacts, err := replaceGeneratedOutput(stage, outputDir, apply.Artifacts)
	if err != nil {
		return apply, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	apply.Artifacts = artifacts
	return apply, artifacts, nil
}

func replaceGeneratedOutput(stage string, outputDir string, produced []reports.Artifact) ([]reports.Artifact, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	marker, err := writeRepairMarker(outputDir)
	if err != nil {
		return nil, err
	}
	defer os.Remove(marker)
	artifacts := make([]reports.Artifact, 0, len(produced))
	var manifestArtifacts []reports.Artifact
	for _, artifact := range produced {
		if strings.TrimSpace(artifact.Path) == "" {
			continue
		}
		rel, err := artifactRel(stage, artifact)
		if err != nil {
			return nil, err
		}
		if filepath.ToSlash(rel) == manifest.RelativePath {
			manifestArtifacts = append(manifestArtifacts, artifact)
			continue
		}
		copied, err := copyProducedArtifact(stage, outputDir, artifact)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied)
	}
	if err := removeStaleGeneratedFiles(stage, outputDir); err != nil {
		return nil, err
	}
	for _, artifact := range manifestArtifacts {
		copied, err := copyProducedArtifact(stage, outputDir, artifact)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied)
	}
	return artifacts, nil
}

func writeRepairMarker(outputDir string) (string, error) {
	path := filepath.Join(outputDir, ".kicadai", "repair-in-progress")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte("repair apply in progress\n"), 0o644)
}

func artifactRel(stage string, artifact reports.Artifact) (string, error) {
	source := artifactSourcePath(stage, artifact)
	return artifactRelFromSource(stage, source, artifact.Path)
}

func artifactRelFromSource(stage string, source string, artifactPath string) (string, error) {
	rel, err := filepath.Rel(stage, source)
	if err != nil {
		return "", err
	}
	relSlash := filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(relSlash, "../") {
		return "", fmt.Errorf("artifact is outside repair stage: %s", artifactPath)
	}
	return rel, nil
}

func copyProducedArtifact(stage string, outputDir string, artifact reports.Artifact) (reports.Artifact, error) {
	source := artifactSourcePath(stage, artifact)
	rel, err := artifactRelFromSource(stage, source, artifact.Path)
	if err != nil {
		return reports.Artifact{}, err
	}
	target := filepath.Join(outputDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return reports.Artifact{}, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return reports.Artifact{}, err
	}
	if info.IsDir() {
		return reports.Artifact{}, fmt.Errorf("directory artifact copy is not supported: %s", artifact.Path)
	}
	if err := atomicCopyFile(target, source, info.Mode().Perm()); err != nil {
		return reports.Artifact{}, err
	}
	copied := artifact
	copied.Path = filepath.ToSlash(target)
	return copied, nil
}

func artifactSourcePath(stage string, artifact reports.Artifact) string {
	source := filepath.FromSlash(artifact.Path)
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(stage, source)
}

func removeStaleGeneratedFiles(stage string, outputDir string) error {
	previous, status, err := manifest.Read(outputDir)
	if err != nil {
		return err
	}
	if !status.Present {
		return nil
	}
	managedDirs := map[string]struct{}{}
	previousFiles := map[string]struct{}{manifest.RelativePath: {}}
	for rel := range previous.FileHashes {
		previousFiles[filepath.ToSlash(rel)] = struct{}{}
	}
	for rel := range previousFiles {
		if !safeManifestRel(rel) {
			return fmt.Errorf("unsafe generated manifest path: %s", rel)
		}
		if !managedGeneratedFile(rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(rel))); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		target := filepath.Join(outputDir, filepath.FromSlash(rel))
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		for dir := filepath.Dir(target); childDir(outputDir, dir); dir = filepath.Dir(dir) {
			if _, ok := managedDirs[dir]; ok {
				break
			}
			relDir, err := filepath.Rel(outputDir, dir)
			if err != nil {
				break
			}
			if managedGeneratedDir(relDir) {
				managedDirs[dir] = struct{}{}
			}
		}
	}
	dirs := make([]string, 0, len(managedDirs))
	for dir := range managedDirs {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func childDir(base string, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../")
}

func managedGeneratedFile(rel string) bool {
	base := filepath.Base(rel)
	if _, ok := managedKiCadExtensions[strings.ToLower(filepath.Ext(base))]; ok {
		return true
	}
	slash := filepath.ToSlash(rel)
	if strings.EqualFold(slash, "sym-lib-table") || strings.EqualFold(slash, "fp-lib-table") {
		return true
	}
	return managedGeneratedPath(rel)
}

func safeManifestRel(rel string) bool {
	if strings.TrimSpace(rel) == "" {
		return false
	}
	native := filepath.FromSlash(rel)
	if filepath.IsAbs(native) {
		return false
	}
	clean := filepath.Clean(native)
	return clean != ".." && !strings.HasPrefix(filepath.ToSlash(clean), "../")
}

func managedGeneratedDir(rel string) bool {
	return managedGeneratedPath(rel)
}

func managedGeneratedPath(rel string) bool {
	slash := strings.ToLower(filepath.ToSlash(rel))
	return slash == ".kicadai" || strings.HasPrefix(slash, ".kicadai/")
}

func atomicCopyFile(path string, source string, perm os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	base := filepath.Base(path)
	if len(base) > 180 {
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		if len(ext) > 40 {
			ext = truncateRunes(ext, 40)
		}
		limit := 180 - len(ext)
		if limit < 20 {
			limit = 20
		}
		if len(stem) > limit {
			stem = truncateRunes(stem, limit)
		}
		base = stem + ext
	}
	file, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tempName := file.Name()
	closed := false
	removeTemp := true
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := io.Copy(file, input); err != nil {
		return err
	}
	if err := file.Chmod(perm); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	if err := renameWithRetry(tempName, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func renameWithRetry(oldPath string, newPath string) error {
	var err error
	err = os.Rename(oldPath, newPath)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
	}
	return err
}

func truncateRunes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	out := make([]rune, 0, maxBytes)
	total := 0
	for _, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		if total+size > maxBytes {
			break
		}
		out = append(out, r)
		total += size
	}
	return string(out)
}

func requiresOverwrite(outputDir string) (bool, error) {
	if strings.TrimSpace(outputDir) == "" {
		return false, nil
	}
	return existingProjectDir(outputDir)
}

func existingProjectDir(outputDir string) (bool, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".kicad_pro") {
			return true, nil
		}
	}
	return false, nil
}

func createReplayStage(outputDir string) (string, error) {
	parent := filepath.Join(outputDir, ".kicadai")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "repair-stage-*")
}

func finalizePersistedResult(result PersistedApplyResult) PersistedApplyResult {
	if reports.HasBlockingIssue(result.Issues) {
		result.Status = StatusBlocked
		return result
	}
	if len(result.Validation) > 0 && (result.Status == StatusRepaired || result.Status == StatusPartial || result.Status == StatusBlocked) {
		return result
	}
	switch result.Repair.Status {
	case StatusRepaired, StatusNotNeeded:
		result.Status = StatusRepaired
	case StatusPartial:
		result.Status = StatusPartial
	default:
		result.Status = StatusBlocked
		if len(result.Issues) == 0 {
			result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "repair", fmt.Sprintf("repair status is %s", result.Repair.Status)))
		}
	}
	return result
}

func persistedIssue(code reports.Code, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityBlocked, Path: path, Message: message}
}
