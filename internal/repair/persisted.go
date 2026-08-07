package repair

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/atomicfile"
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
	Normalized  []NormalizedFinding      `json:"normalized_findings,omitempty"`
	Convergence NormalizedDeltaSummary   `json:"convergence,omitempty"`
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
	validator := &persistedRepairValidator{transaction: &tx, remaining: flattenIssues(bundle.StageIssues)}
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
	applyResult, artifacts, pendingCommit, issues := replayGeneratedTransaction(ctx, tx, outputDir, opts)
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
			if pendingCommit != nil {
				if rollbackErr := pendingCommit.Rollback(); rollbackErr != nil {
					result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.rollback", rollbackErr.Error()))
				}
				pendingCommit = nil
			}
			collectPostValidationEvidence(&result, result.Validation)
			return finalizePersistedValidationResult(result, bundle.StageIssues)
		}
		if zoneRefillValidation.Ran {
			artifact, err := refreshGeneratedManifest(outputDir)
			if err != nil {
				result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "manifest", err.Error()))
				if pendingCommit != nil {
					if rollbackErr := pendingCommit.Rollback(); rollbackErr != nil {
						result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.rollback", rollbackErr.Error()))
					}
					pendingCommit = nil
				}
				collectPostValidationEvidence(&result, result.Validation)
				return finalizePersistedValidationResult(result, bundle.StageIssues)
			}
			result.Artifacts = appendArtifacts(result.Artifacts, []reports.Artifact{artifact})
			if pendingCommit != nil {
				if err := pendingCommit.AdoptCurrent(); err != nil {
					result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.post_process", err.Error()))
					if rollbackErr := pendingCommit.Rollback(); rollbackErr != nil {
						result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.rollback", rollbackErr.Error()))
					}
					pendingCommit = nil
					collectPostValidationEvidence(&result, result.Validation)
					return finalizePersistedValidationResult(result, bundle.StageIssues)
				}
			}
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
	finalized := finalizePersistedValidationResult(result, bundle.StageIssues)
	if pendingCommit == nil {
		return finalized
	}
	if finalized.Delta.Worsened || finalized.Convergence.Worse {
		rollbackErr := pendingCommit.Rollback()
		result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "post_validation", "post-apply validation worsened; restored the prior project"))
		if rollbackErr != nil {
			result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.rollback", rollbackErr.Error()))
		}
		return finalizePersistedValidationResult(result, bundle.StageIssues)
	}
	if commitErr := pendingCommit.Commit(); commitErr != nil {
		result.Issues = append(result.Issues, persistedIssue(reports.CodeValidationFailed, "output.commit", commitErr.Error()))
		return finalizePersistedValidationResult(result, bundle.StageIssues)
	}
	return finalized
}

func refreshGeneratedManifest(outputDir string) (reports.Artifact, error) {
	current, status, err := manifest.Read(outputDir)
	if err != nil {
		return reports.Artifact{}, err
	}
	if !status.Present {
		return reports.Artifact{}, fmt.Errorf("generated manifest is missing")
	}
	current.Artifacts = normalizeRepairStageManifestArtifacts(outputDir, current.Artifacts)
	for rel := range current.FileHashes {
		if strings.HasPrefix(filepath.ToSlash(rel), ".kicadai/repair-stage-") {
			delete(current.FileHashes, rel)
		}
	}
	artifact, err := manifest.Write(outputDir, current)
	if err != nil {
		return reports.Artifact{}, err
	}
	_, refreshed, err := manifest.Read(outputDir)
	if err != nil {
		return reports.Artifact{}, err
	}
	if refreshed.Stale {
		return reports.Artifact{}, fmt.Errorf("generated manifest remains stale after refresh: %s", strings.Join(refreshed.Issues, "; "))
	}
	return artifact, nil
}

// RefreshGeneratedManifest rebinds a generated project's manifest after an
// authorized deterministic post-process, such as KiCad zone refill, mutates
// project files in place.
func RefreshGeneratedManifest(outputDir string) (reports.Artifact, error) {
	return refreshGeneratedManifest(outputDir)
}

type persistedRepairValidator struct {
	transaction *transactions.Transaction
	remaining   []reports.Issue
}

func (validator *persistedRepairValidator) Validate() []reports.Issue {
	if validator == nil || validator.transaction == nil {
		return nil
	}
	issues := transactions.Validate(*validator.transaction).Issues
	if len(issues) > 0 {
		return issues
	}
	return append([]reports.Issue(nil), validator.remaining...)
}

func (validator *persistedRepairValidator) ValidateAttempt(attempt Attempt, current []reports.Issue) []reports.Issue {
	if validator == nil || validator.transaction == nil {
		return nil
	}
	issues := transactions.Validate(*validator.transaction).Issues
	if len(issues) > 0 {
		validator.remaining = append([]reports.Issue(nil), issues...)
		return issues
	}
	if !repairAttemptResolvedByTransaction(*validator.transaction, attempt) {
		validator.remaining = append([]reports.Issue(nil), current...)
		return append([]reports.Issue(nil), validator.remaining...)
	}
	validator.remaining = removeAttemptedIssue(current, attempt.Issue)
	return append([]reports.Issue(nil), validator.remaining...)
}

func repairAttemptResolvedByTransaction(tx transactions.Transaction, attempt Attempt) bool {
	switch attempt.Action {
	case ActionAssignFootprint:
		refs := normalizedIssueRefs(attempt.Issue)
		if len(refs) == 0 {
			return false
		}
		assigned := map[string]bool{}
		for _, operation := range tx.Operations {
			if operation.Op == transactions.OpWriteProject {
				break
			}
			if operation.Op != transactions.OpAssignFootprint {
				continue
			}
			ref := normalizeRef(operation.Ref)
			if ref == "" {
				continue
			}
			assigned[ref] = true
		}
		for _, ref := range refs {
			if !assigned[ref] {
				return false
			}
		}
		return true
	case ActionGenerateOutline:
		for _, operation := range tx.Operations {
			if operation.Op == transactions.OpWriteProject {
				break
			}
			if operation.Op == transactions.OpSetBoardOutline {
				return true
			}
		}
	}
	return false
}

func normalizedIssueRefs(issue reports.Issue) []string {
	refs := make([]string, 0, len(issue.Refs))
	seen := map[string]bool{}
	for _, ref := range issue.Refs {
		normalized := normalizeRef(ref)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		refs = append(refs, normalized)
	}
	sort.Strings(refs)
	return refs
}

func normalizeRepairStageManifestArtifacts(outputDir string, artifacts []reports.Artifact) []reports.Artifact {
	absRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return artifacts
	}
	normalized := make([]reports.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		portablePath := filepath.ToSlash(artifact.Path)
		relSlash := portablePath
		if artifactPathIsAbs(artifact.Path) {
			rel, err := filepath.Rel(absRoot, filepath.FromSlash(portablePath))
			if err != nil {
				normalized = append(normalized, artifact)
				continue
			}
			relSlash = filepath.ToSlash(rel)
		}
		if strings.HasPrefix(relSlash, ".kicadai/repair-stage-") {
			parts := strings.SplitN(relSlash, "/", 3)
			if len(parts) == 3 {
				artifact.Path = parts[2]
			}
		}
		normalized = append(normalized, artifact)
	}
	return normalized
}

func artifactPathIsAbs(value string) bool {
	portable := filepath.ToSlash(value)
	if path.IsAbs(portable) || filepath.IsAbs(filepath.FromSlash(portable)) {
		return true
	}
	return len(portable) >= 3 && portable[1] == ':' && portable[2] == '/'
}

func finalizePersistedValidationResult(result PersistedApplyResult, stageIssues []StageIssues) PersistedApplyResult {
	result.Summary = SummarizePostValidation(result.Validation)
	result.Delta = CompareValidationIssues(flattenIssues(stageIssues), result.Issues)
	before := NormalizeStageIssues(stageIssues)
	after := NormalizePostApplyValidations(result.Validation)
	result.Normalized = after
	result.Convergence = CompareNormalizedFindings(before, after)
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

func replayGeneratedTransaction(ctx context.Context, tx transactions.Transaction, outputDir string, opts PersistedApplyOptions) (transactions.ApplyResult, []reports.Artifact, *persistedOutputCommit, []reports.Issue) {
	if err := ctx.Err(); err != nil {
		return transactions.ApplyResult{}, nil, nil, []reports.Issue{contextIssue(err)}
	}
	existing, err := existingProjectDir(outputDir)
	if err != nil {
		return transactions.ApplyResult{}, nil, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	if !existing {
		if err := ctx.Err(); err != nil {
			return transactions.ApplyResult{}, nil, nil, []reports.Issue{contextIssue(err)}
		}
		apply := transactions.Apply(tx, transactions.ApplyOptions{
			OutputDir:     outputDir,
			Overwrite:     opts.Overwrite,
			Seed:          opts.Seed,
			LibraryIndex:  opts.LibraryIndex,
			LibraryIssues: opts.LibraryIssues,
		})
		return apply, apply.Artifacts, nil, nil
	}
	stage, err := createReplayStage(outputDir)
	if err != nil {
		return transactions.ApplyResult{}, nil, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := ctx.Err(); err != nil {
		return transactions.ApplyResult{}, nil, nil, []reports.Issue{contextIssue(err)}
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
		return apply, nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return apply, nil, nil, []reports.Issue{contextIssue(err)}
	}
	artifacts, pending, err := replaceGeneratedOutput(stage, outputDir, apply.Artifacts)
	if err != nil {
		return apply, nil, nil, []reports.Issue{persistedIssue(reports.CodeValidationFailed, "output", err.Error())}
	}
	apply.Artifacts = artifacts
	return apply, artifacts, pending, nil
}

type persistedOutputCommit struct {
	group   *atomicfile.Group
	release func()
	closed  bool
}

func (commit *persistedOutputCommit) Commit() error {
	if commit == nil || commit.closed {
		return nil
	}
	commit.closed = true
	err := commit.group.Commit()
	commit.release()
	return err
}

func (commit *persistedOutputCommit) Rollback() error {
	if commit == nil || commit.closed {
		return nil
	}
	commit.closed = true
	err := commit.group.Rollback()
	commit.release()
	return err
}

func (commit *persistedOutputCommit) AdoptCurrent() error {
	if commit == nil || commit.closed {
		return nil
	}
	return commit.group.AdoptCurrent()
}

func replaceGeneratedOutput(stage string, outputDir string, produced []reports.Artifact) ([]reports.Artifact, *persistedOutputCommit, error) {
	return replaceGeneratedOutputWithOptions(stage, outputDir, produced, atomicfile.GroupOptions{})
}

func replaceGeneratedOutputWithOptions(stage string, outputDir string, produced []reports.Artifact, groupOptions atomicfile.GroupOptions) ([]reports.Artifact, *persistedOutputCommit, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, nil, err
	}
	releaseLock, err := transactions.AcquireProjectApplyLock(outputDir)
	if err != nil {
		return nil, nil, err
	}
	mutations, artifacts, err := generatedOutputMutations(stage, outputDir, produced)
	if err != nil {
		releaseLock()
		return nil, nil, err
	}
	groupOptions.Root = outputDir
	group, err := atomicfile.BeginGroup(mutations, groupOptions)
	if err != nil {
		releaseLock()
		return nil, nil, err
	}
	return artifacts, &persistedOutputCommit{group: group, release: releaseLock}, nil
}

func generatedOutputMutations(stage string, outputDir string, produced []reports.Artifact) ([]atomicfile.Mutation, []reports.Artifact, error) {
	mutations := make([]atomicfile.Mutation, 0, len(produced))
	artifacts := make([]reports.Artifact, 0, len(produced))
	producedRels := map[string]bool{}
	for _, artifact := range produced {
		if strings.TrimSpace(artifact.Path) == "" {
			continue
		}
		source := artifactSourcePath(stage, artifact)
		rel, err := artifactRelFromSource(stage, source, artifact.Path)
		if err != nil {
			return nil, nil, err
		}
		relSlash := filepath.ToSlash(rel)
		if producedRels[relSlash] {
			return nil, nil, fmt.Errorf("duplicate generated artifact: %s", relSlash)
		}
		producedRels[relSlash] = true
		info, err := os.Stat(source)
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("directory artifact copy is not supported: %s", artifact.Path)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, nil, err
		}
		target := filepath.Join(outputDir, filepath.FromSlash(relSlash))
		mutations = append(mutations, atomicfile.Mutation{Path: target, Data: data, Mode: info.Mode().Perm()})
		copied := artifact
		copied.Path = filepath.ToSlash(target)
		artifacts = append(artifacts, copied)
	}
	stale, err := staleGeneratedMutations(outputDir, producedRels)
	if err != nil {
		return nil, nil, err
	}
	mutations = append(mutations, stale...)
	return mutations, artifacts, nil
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

func artifactSourcePath(stage string, artifact reports.Artifact) string {
	source := filepath.FromSlash(artifact.Path)
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(stage, source)
}

func staleGeneratedMutations(outputDir string, produced map[string]bool) ([]atomicfile.Mutation, error) {
	previous, status, err := manifest.Read(outputDir)
	if err != nil {
		return nil, err
	}
	if !status.Present {
		return nil, nil
	}
	previousFiles := map[string]struct{}{manifest.RelativePath: {}}
	for rel := range previous.FileHashes {
		previousFiles[filepath.ToSlash(rel)] = struct{}{}
	}
	rels := make([]string, 0, len(previousFiles))
	for rel := range previousFiles {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	var mutations []atomicfile.Mutation
	for _, rel := range rels {
		if !safeManifestRel(rel) {
			return nil, fmt.Errorf("unsafe generated manifest path: %s", rel)
		}
		if !managedGeneratedFile(rel) || produced[filepath.ToSlash(rel)] || reservedMutationPath(rel) {
			continue
		}
		target := filepath.Join(outputDir, filepath.FromSlash(rel))
		if info, err := os.Stat(target); err == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("stale generated target is not a regular file: %s", rel)
			}
			mutations = append(mutations, atomicfile.Mutation{Path: target, Delete: true, Mode: info.Mode().Perm()})
		} else if os.IsNotExist(err) {
			continue
		} else {
			return nil, err
		}
	}
	return mutations, nil
}

func reservedMutationPath(rel string) bool {
	slash := filepath.ToSlash(rel)
	return slash == atomicfile.JournalRelativePath ||
		slash == atomicfile.MutationLockRelativePath ||
		slash == transactions.ApplyLockFileName ||
		strings.HasPrefix(slash, ".kicadai/repair-stage-")
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

func managedGeneratedPath(rel string) bool {
	slash := strings.ToLower(filepath.ToSlash(rel))
	return slash == ".kicadai" || strings.HasPrefix(slash, ".kicadai/")
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
