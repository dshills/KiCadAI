package repair

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/manifest"
	"kicadai/internal/repairloop"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type CrossStageGeneratedFile struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

type CrossStageGeneratedProject struct {
	Files []CrossStageGeneratedFile `json:"files"`
}

type CrossStageGeneratedValidateFunc func(context.Context, repairloop.CrossStage, CrossStageGeneratedProject) (CrossStageTransactionEvidence, error)

type CrossStageGeneratedTargetOptions struct {
	Transaction    transactions.Transaction
	Apply          transactions.ApplyOptions
	Project        CrossStageGeneratedProject
	GeneratedPaths []string
	RequiredStages []repairloop.CrossStage
	Validate       CrossStageGeneratedValidateFunc
}

// CrossStageGeneratedTarget repairs output-layer schematic, ERC, writer, and
// round-trip drift by regenerating only authoritative generated files. Files
// outside GeneratedPaths are retained as protected user-owned scopes.
type CrossStageGeneratedTarget struct {
	opts             CrossStageGeneratedTargetOptions
	project          CrossStageGeneratedProject
	generatedPaths   []string
	evidence         CrossStageTransactionEvidence
	lastReenter      repairloop.CrossStage
	diagnosticIssues map[string]crossStageTransactionIssue
	proposals        map[string]repairloop.CrossStage
	pending          string
}

type crossStageGeneratedState struct {
	Project        CrossStageGeneratedProject `json:"project"`
	GeneratedPaths []string                   `json:"generated_paths"`
	LastReenter    repairloop.CrossStage      `json:"last_reenter,omitempty"`
}

func GenerateCrossStageProject(ctx context.Context, transaction transactions.Transaction, opts transactions.ApplyOptions) (CrossStageGeneratedProject, []string, error) {
	logicalRoot := strings.TrimSpace(opts.OutputDir)
	root, err := os.MkdirTemp("", "kicadai-cross-stage-generate-")
	if err != nil {
		return CrossStageGeneratedProject{}, nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	opts.OutputDir = root
	opts.Overwrite = true
	result := transactions.Apply(transaction, opts)
	if reports.HasBlockingIssue(result.Issues) {
		return CrossStageGeneratedProject{}, nil, fmt.Errorf("authoritative generation blocked: %s", generatedCrossStageIssueSummary(result.Issues))
	}
	if err := ctx.Err(); err != nil {
		return CrossStageGeneratedProject{}, nil, err
	}
	project, err := readCrossStageGeneratedProject(root)
	if err != nil {
		return CrossStageGeneratedProject{}, nil, err
	}
	project, err = canonicalizeCrossStageGeneratedManifest(project, root, logicalRoot)
	if err != nil {
		return CrossStageGeneratedProject{}, nil, err
	}
	paths := make([]string, 0, len(project.Files))
	for _, file := range project.Files {
		paths = append(paths, file.Path)
	}
	return project, paths, nil
}

func canonicalizeCrossStageGeneratedManifest(project CrossStageGeneratedProject, actualRoot, logicalRoot string) (CrossStageGeneratedProject, error) {
	index, found := slices.BinarySearchFunc(project.Files, manifest.RelativePath, func(file CrossStageGeneratedFile, target string) int {
		return strings.Compare(file.Path, target)
	})
	if !found {
		return project, nil
	}
	var generated manifest.Manifest
	if err := json.Unmarshal(project.Files[index].Data, &generated); err != nil {
		return CrossStageGeneratedProject{}, fmt.Errorf("decode generated manifest: %w", err)
	}
	for artifactIndex := range generated.Artifacts {
		path := filepath.FromSlash(generated.Artifacts[artifactIndex].Path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(actualRoot, path)
		}
		rel, err := filepath.Rel(actualRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
			return CrossStageGeneratedProject{}, fmt.Errorf("generated artifact is outside generation root: %s", generated.Artifacts[artifactIndex].Path)
		}
		if logicalRoot != "" && logicalRoot != "." {
			rel = filepath.Join(filepath.FromSlash(logicalRoot), rel)
		}
		generated.Artifacts[artifactIndex].Path = filepath.ToSlash(rel)
	}
	payload, err := json.MarshalIndent(generated, "", "  ")
	if err != nil {
		return CrossStageGeneratedProject{}, err
	}
	payload = append(payload, '\n')
	project.Files[index].Data = payload
	return project, nil
}

func NewCrossStageGeneratedTarget(opts CrossStageGeneratedTargetOptions) (*CrossStageGeneratedTarget, error) {
	if opts.Validate == nil {
		return nil, errors.New("cross-stage generated target requires structured validation")
	}
	project, err := normalizeCrossStageGeneratedProject(opts.Project)
	if err != nil {
		return nil, err
	}
	generatedPaths, err := normalizeCrossStageGeneratedPaths(opts.GeneratedPaths)
	if err != nil {
		return nil, err
	}
	if len(generatedPaths) == 0 {
		return nil, errors.New("cross-stage generated target requires authoritative generated paths")
	}
	for _, path := range generatedPaths {
		if _, found := crossStageGeneratedFile(project, path); !found {
			return nil, fmt.Errorf("generated path %q is missing from the current project", path)
		}
	}
	opts.Project = project
	opts.GeneratedPaths = generatedPaths
	opts.RequiredStages = normalizeTransactionCrossStages(opts.RequiredStages)
	target := &CrossStageGeneratedTarget{
		opts: opts, project: project, generatedPaths: generatedPaths,
		diagnosticIssues: map[string]crossStageTransactionIssue{}, proposals: map[string]repairloop.CrossStage{},
	}
	return target, nil
}

func (target *CrossStageGeneratedTarget) Capture(ctx context.Context) (repairloop.CrossStageCheckpoint, error) {
	if err := target.refreshEvidence(ctx); err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	state := crossStageGeneratedState{
		Project:        cloneCrossStageGeneratedProject(target.project),
		GeneratedPaths: slices.Clone(target.generatedPaths), LastReenter: target.lastReenter,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	return repairloop.NewCrossStageCheckpoint(
		payload, generatedCrossStageScopeHashes(target.opts.Transaction, target.project),
		crossStageTransactionGates(target.opts.RequiredStages, target.evidence.Stages), target.evidence.Margins,
	), nil
}

func (target *CrossStageGeneratedTarget) Restore(ctx context.Context, checkpoint repairloop.CrossStageCheckpoint) error {
	var state crossStageGeneratedState
	if err := json.Unmarshal(checkpoint.Payload, &state); err != nil {
		return err
	}
	project, err := normalizeCrossStageGeneratedProject(state.Project)
	if err != nil {
		return err
	}
	paths, err := normalizeCrossStageGeneratedPaths(state.GeneratedPaths)
	if err != nil {
		return err
	}
	target.project = project
	target.generatedPaths = paths
	target.lastReenter = state.LastReenter
	target.pending = ""
	return target.refreshEvidence(ctx)
}

func (target *CrossStageGeneratedTarget) Diagnose(ctx context.Context) ([]repairloop.CrossStageDiagnostic, error) {
	if err := target.refreshEvidence(ctx); err != nil {
		return nil, err
	}
	target.diagnosticIssues = map[string]crossStageTransactionIssue{}
	result := []repairloop.CrossStageDiagnostic{}
	for _, stage := range target.evidence.Stages {
		for _, issue := range stage.Issues {
			severity := crossStageSeverityForIssue(issue)
			if severity == repairloop.CrossStageSeverityInfo {
				continue
			}
			category := string(Classify(issue).Category)
			if category == string(CategoryUnknown) {
				category = "generated_output_drift"
			}
			diagnostic := repairloop.NewCrossStageDiagnostic(
				stage.Stage, string(issue.Code), category, severity,
				transactionIssueEvidenceHash(stage.Stage, issue), generatedCrossStageIssueScopes(stage.Stage, issue),
			)
			target.diagnosticIssues[diagnostic.Hash] = crossStageTransactionIssue{Stage: stage.Stage, Issue: issue}
			result = append(result, diagnostic)
		}
	}
	return result, nil
}

func (target *CrossStageGeneratedTarget) Propose(_ context.Context, diagnostic repairloop.CrossStageDiagnostic) ([]repairloop.CrossStageProposal, error) {
	entry, ok := target.diagnosticIssues[diagnostic.Hash]
	if !ok {
		return nil, errors.New("cross-stage generated diagnostic is not current")
	}
	operator, affected, supported := generatedCrossStageOperator(entry.Stage)
	scopes := make([]string, 0, len(target.generatedPaths))
	for _, path := range target.generatedPaths {
		scopes = append(scopes, "file:"+path)
	}
	rejection := ""
	if !supported {
		rejection = "diagnostic is not rooted in a regenerable output stage"
	}
	proposal := repairloop.NewCrossStageProposal(
		diagnostic, operator, affected, []string{"restore authoritative generated output"}, scopes,
		max(1, len(scopes)), float64(max(1, len(scopes))), 1, supported, rejection,
	)
	target.proposals[proposal.ID] = entry.Stage
	return []repairloop.CrossStageProposal{proposal}, nil
}

func (target *CrossStageGeneratedTarget) Apply(_ context.Context, proposal repairloop.CrossStageProposal) error {
	if _, ok := target.proposals[proposal.ID]; !ok {
		return errors.New("cross-stage generated proposal is not current")
	}
	target.pending = proposal.ID
	return nil
}

func (target *CrossStageGeneratedTarget) Reenter(ctx context.Context, stage repairloop.CrossStage) error {
	if target.pending == "" {
		return errors.New("cross-stage generated re-entry lacks an applied proposal")
	}
	generated, paths, err := GenerateCrossStageProject(ctx, target.opts.Transaction, target.opts.Apply)
	if err != nil {
		return err
	}
	current := make(map[string]CrossStageGeneratedFile, len(target.project.Files)+len(generated.Files))
	for _, file := range target.project.Files {
		current[file.Path] = file
	}
	for _, path := range target.generatedPaths {
		delete(current, path)
	}
	for _, file := range generated.Files {
		current[file.Path] = file
	}
	project := CrossStageGeneratedProject{Files: make([]CrossStageGeneratedFile, 0, len(current))}
	for _, file := range current {
		project.Files = append(project.Files, file)
	}
	project, err = normalizeCrossStageGeneratedProject(project)
	if err != nil {
		return err
	}
	evidence, err := target.validateEvidence(ctx, stage, project)
	if err != nil {
		return err
	}
	target.project = project
	target.generatedPaths = paths
	target.lastReenter = stage
	target.evidence = evidence
	target.pending = ""
	return nil
}

func (target *CrossStageGeneratedTarget) Project() CrossStageGeneratedProject {
	return cloneCrossStageGeneratedProject(target.project)
}

func (target *CrossStageGeneratedTarget) refreshEvidence(ctx context.Context) error {
	evidence, err := target.validateEvidence(ctx, target.lastReenter, target.project)
	if err != nil {
		return err
	}
	target.evidence = evidence
	return nil
}

func (target *CrossStageGeneratedTarget) validateEvidence(ctx context.Context, stage repairloop.CrossStage, project CrossStageGeneratedProject) (CrossStageTransactionEvidence, error) {
	evidence, err := target.opts.Validate(ctx, stage, cloneCrossStageGeneratedProject(project))
	if err != nil {
		return CrossStageTransactionEvidence{}, err
	}
	evidence.Stages = normalizeCrossStageTransactionStages(evidence.Stages)
	evidence.Margins = normalizeTransactionCrossStageMargins(evidence.Margins)
	return evidence, nil
}

func generatedCrossStageOperator(stage repairloop.CrossStage) (string, []repairloop.CrossStage, bool) {
	switch stage {
	case repairloop.CrossStageSchematic:
		return "regenerate_authoritative_schematic", []repairloop.CrossStage{repairloop.CrossStageSchematic}, true
	case repairloop.CrossStageERC:
		return "regenerate_authoritative_schematic", []repairloop.CrossStage{repairloop.CrossStageSchematic, repairloop.CrossStageERC}, true
	case repairloop.CrossStageWriter:
		return "rewrite_authoritative_output", []repairloop.CrossStage{repairloop.CrossStageWriter}, true
	case repairloop.CrossStageRoundTrip:
		return "canonicalize_writer_output", []repairloop.CrossStage{repairloop.CrossStageWriter, repairloop.CrossStageRoundTrip}, true
	default:
		return "regenerate_authoritative_output", []repairloop.CrossStage{stage}, false
	}
}

func generatedCrossStageIssueScopes(stage repairloop.CrossStage, issue reports.Issue) []string {
	path := normalizeCrossStageGeneratedPath(issue.Path)
	if path != "" {
		return []string{"file:" + path}
	}
	return []string{"stage:" + string(stage)}
}

func generatedCrossStageScopeHashes(transaction transactions.Transaction, project CrossStageGeneratedProject) []repairloop.CrossStageScopeHash {
	result := []repairloop.CrossStageScopeHash{{Scope: "authoritative:transaction", Hash: transactionCrossStageHash(transaction)}}
	for _, file := range project.Files {
		result = append(result, repairloop.CrossStageScopeHash{Scope: "file:" + file.Path, Hash: transactionCrossStageHash(file.Data)})
	}
	return result
}

func normalizeCrossStageGeneratedProject(project CrossStageGeneratedProject) (CrossStageGeneratedProject, error) {
	result := CrossStageGeneratedProject{Files: make([]CrossStageGeneratedFile, 0, len(project.Files))}
	seen := map[string]struct{}{}
	for _, file := range project.Files {
		path := normalizeCrossStageGeneratedPath(file.Path)
		if path == "" {
			return CrossStageGeneratedProject{}, fmt.Errorf("unsafe generated project path %q", file.Path)
		}
		if _, exists := seen[path]; exists {
			return CrossStageGeneratedProject{}, fmt.Errorf("duplicate generated project path %q", path)
		}
		seen[path] = struct{}{}
		result.Files = append(result.Files, CrossStageGeneratedFile{Path: path, Data: slices.Clone(file.Data)})
	}
	slices.SortFunc(result.Files, func(left, right CrossStageGeneratedFile) int { return strings.Compare(left.Path, right.Path) })
	return result, nil
}

func normalizeCrossStageGeneratedPaths(source []string) ([]string, error) {
	result := make([]string, 0, len(source))
	for _, item := range source {
		path := normalizeCrossStageGeneratedPath(item)
		if path == "" {
			return nil, fmt.Errorf("unsafe generated path %q", item)
		}
		result = append(result, path)
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func normalizeCrossStageGeneratedPath(source string) string {
	source = strings.TrimSpace(filepath.ToSlash(source))
	if source == "" || filepath.IsAbs(filepath.FromSlash(source)) {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(source)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func cloneCrossStageGeneratedProject(project CrossStageGeneratedProject) CrossStageGeneratedProject {
	result := CrossStageGeneratedProject{Files: make([]CrossStageGeneratedFile, len(project.Files))}
	for index, file := range project.Files {
		result.Files[index] = CrossStageGeneratedFile{Path: file.Path, Data: slices.Clone(file.Data)}
	}
	return result
}

func crossStageGeneratedFile(project CrossStageGeneratedProject, path string) (CrossStageGeneratedFile, bool) {
	index, found := slices.BinarySearchFunc(project.Files, path, func(file CrossStageGeneratedFile, target string) int {
		return strings.Compare(file.Path, target)
	})
	if !found {
		return CrossStageGeneratedFile{}, false
	}
	return project.Files[index], true
}

func readCrossStageGeneratedProject(root string) (CrossStageGeneratedProject, error) {
	project := CrossStageGeneratedProject{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("generated project contains non-regular file %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == transactions.ApplyLockFileName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		project.Files = append(project.Files, CrossStageGeneratedFile{Path: filepath.ToSlash(rel), Data: data})
		return nil
	})
	if err != nil {
		return CrossStageGeneratedProject{}, err
	}
	return normalizeCrossStageGeneratedProject(project)
}

func generatedCrossStageIssueSummary(issues []reports.Issue) string {
	parts := []string{}
	for _, issue := range issues {
		if issue.Blocking() {
			parts = append(parts, string(issue.Code)+":"+issue.Path)
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}
