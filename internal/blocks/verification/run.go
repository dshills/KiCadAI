package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/boardvalidation"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
	"kicadai/internal/writercorrectness"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusWarning Status = "warning"
	StatusBlocked Status = "blocked"
	StatusSkipped Status = "skipped"
)

const (
	defaultPlacementToleranceMM  = 0.001
	defaultPlacementToleranceDeg = 0.1
	projectSentinelName          = ".kicadai-block-verification"
)

type RunOptions struct {
	Registry      blocks.Registry
	Strict        bool
	OutputDir     string
	Overwrite     bool
	KeepArtifacts bool
	WriterOptions writercorrectness.Options
	KiCadCLI      string
	RequireERC    bool
	RequireDRC    bool
	AllowlistPath string
	CheckOptions  checks.Options
	CheckRunner   CheckRunner
}

type CheckRunner func(ctx context.Context, kind checks.CheckKind, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, error)

type RunResult struct {
	CaseID        string              `json:"case_id"`
	BlockID       string              `json:"block_id"`
	EvidenceLevel EvidenceLevel       `json:"evidence_level"`
	Status        Status              `json:"status"`
	Stages        []StageResult       `json:"stages"`
	Output        *blocks.BlockOutput `json:"output,omitempty"`
	Issues        []reports.Issue     `json:"issues,omitempty"`
	Artifacts     []reports.Artifact  `json:"artifacts,omitempty"`
}

type StageResult struct {
	Name    string          `json:"name"`
	Status  Status          `json:"status"`
	Issues  []reports.Issue `json:"issues,omitempty"`
	Summary string          `json:"summary,omitempty"`
}

type semanticSummary struct {
	Components map[string]actualComponent
	Nets       map[string][]actualPin
	Ports      map[string]blocks.BlockPort
	PCB        actualPCB
}

type actualComponent struct {
	Role        string
	Ref         string
	SymbolID    string
	FootprintID string
	Value       string
}

type actualPin struct {
	Ref string
	Pin string
}

type actualPCB struct {
	Placements map[string]actualPlacement
	PadNets    map[padKey]string
	Routes     map[string]struct{}
	ZoneNames  map[string]struct{}
	ZoneNets   map[string]struct{}
}

type actualPlacement struct {
	Ref         string
	Role        string
	FootprintID string
	XMM         float64
	YMM         float64
	RotationDeg float64
}

type padKey struct {
	Ref string
	Pad string
}

type connectEdge struct {
	NetName string
	From    actualPin
	To      actualPin
}

func RunCase(ctx context.Context, manifest Manifest, opts RunOptions) RunResult {
	activeRegistry := opts.Registry
	if activeRegistry == nil {
		activeRegistry = blocks.NewBuiltinRegistry()
	}
	result := RunResult{
		CaseID:        manifest.ID,
		BlockID:       manifest.BlockID,
		EvidenceLevel: manifest.Expected.EvidenceLevel,
		Status:        StatusPass,
	}
	manifestIssues := ValidateManifest(manifest, activeRegistry)
	result.addStage(StageResult{Name: "manifest", Issues: manifestIssues, Summary: "validated manifest"})
	if reports.HasBlockingIssue(manifestIssues) {
		result.finish()
		return result
	}
	request := blocks.BlockRequest{BlockID: manifest.BlockID, InstanceID: requestInstanceID(manifest), Params: manifest.Request.Params}
	output, instantiateIssues := activeRegistry.Instantiate(ctx, request)
	result.Output = &output
	instantiateIssues = append(instantiateIssues, output.Issues...)
	result.addStage(StageResult{Name: "instantiate", Issues: instantiateIssues, Summary: fmt.Sprintf("generated %d operation(s)", len(output.Operations))})
	if reports.HasBlockingIssue(instantiateIssues) {
		result.finish()
		return result
	}
	summary, summaryIssues := summarizeOutput(output)
	semanticIssues := append(summaryIssues, assertSemantics(manifest, summary, opts)...)
	result.addStage(StageResult{Name: "semantic_assertions", Issues: semanticIssues, Summary: "checked expected components, ports, nets, and pins"})
	if reports.HasBlockingIssue(semanticIssues) {
		result.finish()
		return result
	}
	if pcbRealizationRequested(manifest.Expected.PCB) {
		definition, ok := activeRegistry.GetBlock(manifest.BlockID)
		if !ok {
			result.addStage(StageResult{Name: "pcb_realization", Issues: []reports.Issue{runIssue("verification."+pathID(manifest.ID)+".pcb_realization.block", "block definition not found "+manifest.BlockID)}, Summary: "failed to load block definition"})
			result.finish()
			return result
		}
		stage := runPCBRealizationStage(manifest, definition, output)
		result.addStage(stage)
		if reports.HasBlockingIssue(stage.Issues) {
			result.finish()
			return result
		}
	}
	if pcbAssertionsRequested(manifest.Expected) {
		pcbIssues := assertPCB(manifest, summary)
		result.addStage(StageResult{Name: "pcb_assertions", Issues: pcbIssues, Summary: "checked expected placements, pad nets, routes, and zones"})
		if reports.HasBlockingIssue(pcbIssues) {
			result.finish()
			return result
		}
	}
	if writerRequested(manifest.Expected.Writer) {
		stage, artifacts := runWriterStage(ctx, manifest, &output, opts)
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.addStage(stage)
		if reports.HasBlockingIssue(stage.Issues) {
			result.finish()
			return result
		}
	}
	if manifest.Expected.PCB.RequireBoardValidation {
		stage, artifacts := runBoardValidationStage(ctx, manifest, &output, opts)
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.addStage(stage)
		if reports.HasBlockingIssue(stage.Issues) {
			result.finish()
			return result
		}
	}
	if ercDRCRequested(manifest.Expected, opts) {
		ercRunOpts := opts
		if writerRequested(manifest.Expected.Writer) || manifest.Expected.PCB.RequireBoardValidation {
			ercRunOpts.Overwrite = false
		}
		stage, artifacts := runERCDRCStage(ctx, manifest, &output, ercRunOpts)
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.addStage(stage)
	}
	result.finish()
	return result
}

func (result *RunResult) addStage(stage StageResult) {
	if stage.Status == "" {
		stage.Status = statusForIssues(stage.Issues)
	}
	result.Stages = append(result.Stages, stage)
	result.Issues = append(result.Issues, stage.Issues...)
}

func (result *RunResult) finish() {
	SortIssues(result.Issues)
	result.Artifacts = dedupeArtifacts(result.Artifacts)
	result.Status = StatusPass
	for _, stage := range result.Stages {
		switch stage.Status {
		case StatusBlocked:
			result.Status = StatusBlocked
		case StatusWarning:
			if result.Status == StatusPass {
				result.Status = StatusWarning
			}
		}
	}
}

func dedupeArtifacts(artifacts []reports.Artifact) []reports.Artifact {
	if len(artifacts) < 2 {
		return artifacts
	}
	seen := map[string]struct{}{}
	deduped := make([]reports.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		key := string(artifact.Kind) + "\x00" + filepath.ToSlash(filepath.Clean(artifact.Path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, artifact)
	}
	return deduped
}

func writerRequested(writer ExpectedWriter) bool {
	return writer.Required || writer.OK || writer.AllowUnrouted || writer.RequireRoundTrip
}

func pcbRealizationRequested(pcb ExpectedPCB) bool {
	return pcb.RequireRealization ||
		pcb.RequireBoardValidation ||
		len(pcb.RequiredLocalRoutes) > 0 ||
		len(pcb.TimingFixtures) > 0
}

func runPCBRealizationStage(manifest Manifest, definition blocks.BlockDefinition, output blocks.BlockOutput) StageResult {
	realized := blocks.RealizeBlockPCB(definition, output, blocks.PCBRealizationOptions{})
	issues := append([]reports.Issue(nil), realized.Issues...)
	issues = append(issues, assertPCBRealization(manifest, realized)...)
	summary := fmt.Sprintf("realized %d component(s), %d local route(s), %d timing fixture(s)", len(realized.Components), len(realized.LocalRoutes), len(realized.Timing))
	return StageResult{Name: "pcb_realization", Issues: issues, Summary: summary}
}

func runBoardValidationStage(ctx context.Context, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) (StageResult, []reports.Artifact) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		return StageResult{
			Name:    "board_validation",
			Issues:  []reports.Issue{runIssue("verification."+pathID(manifest.ID)+".board_validation.output_dir", "board validation requires an output directory")},
			Summary: "board validation failed: missing output directory",
		}, nil
	}
	projectDir, artifacts, issues := ensureProjectForExternalChecks(manifest, output, opts)
	issues = contextualizeBoardValidationPrepIssues(manifest, issues)
	if reports.HasBlockingIssue(issues) {
		return StageResult{Name: "board_validation", Issues: issues, Summary: "failed to prepare generated project for board validation"}, artifacts
	}
	result := boardvalidation.Validate(ctx, projectDir, boardvalidation.Options{
		StrictUnrouted:  !manifest.Expected.PCB.AllowUnrouted,
		KiCadCLI:        opts.KiCadCLI,
		AllowMissingDRC: true,
	})
	artifacts = append(artifacts, result.Artifacts...)
	issues = append(issues, contextualizeBoardValidationIssues(manifest, result.Issues)...)
	summary := fmt.Sprintf("board validation %s: checks=%d issues=%d blocking=%d", result.Status, result.Summary.TotalChecks, result.Summary.TotalIssues, result.Summary.BlockingIssues)
	return StageResult{Name: "board_validation", Issues: issues, Summary: summary}, artifacts
}

func contextualizeBoardValidationIssues(manifest Manifest, issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	out := cloneReportIssues(issues)
	prefix := "verification." + pathID(manifest.ID) + ".board_validation"
	for i := range out {
		if out[i].Path == "" {
			out[i].Path = prefix
		} else {
			out[i].Path = prefix + "." + pathSegment(out[i].Path)
		}
		if out[i].Suggestion == "" {
			out[i].Suggestion = "review board validation evidence for case " + manifest.ID + " block " + manifest.BlockID
		}
	}
	return out
}

func contextualizeBoardValidationPrepIssues(manifest Manifest, issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	out := cloneReportIssues(issues)
	prefix := "verification." + pathID(manifest.ID) + ".board_validation.prepare"
	for i := range out {
		previousPath := strings.TrimSpace(out[i].Path)
		if previousPath == "" {
			out[i].Path = prefix
		} else {
			out[i].Path = prefix + "." + pathSegment(previousPath)
		}
		if out[i].Suggestion == "" {
			out[i].Suggestion = "fix generated project preparation for board validation case " + manifest.ID + " block " + manifest.BlockID
		}
	}
	return out
}

// cloneReportIssues should stay in sync with reports.Issue reference fields.
func cloneReportIssues(issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]reports.Issue, len(issues))
	for index, issue := range issues {
		issue.UUIDs = slices.Clone(issue.UUIDs)
		issue.Refs = slices.Clone(issue.Refs)
		issue.Nets = slices.Clone(issue.Nets)
		out[index] = issue
	}
	return out
}

func assertPCBRealization(manifest Manifest, realized blocks.BlockPCBRealizationResult) []reports.Issue {
	var issues []reports.Issue
	basePath := "verification." + pathID(manifest.ID) + ".pcb_realization"
	localRoutes := map[string]blocks.RealizedPCBLocalRoute{}
	for _, route := range realized.LocalRoutes {
		localRoutes[strings.TrimSpace(route.ID)] = route
	}
	for _, routeID := range manifest.Expected.PCB.RequiredLocalRoutes {
		routeID = strings.TrimSpace(routeID)
		if _, ok := localRoutes[routeID]; !ok {
			issues = append(issues, runIssue(basePath+".local_routes."+pathSegment(routeID), "missing required local route "+routeID))
		}
	}
	timingByID := map[string]blocks.TimingFixtureEvidence{}
	for _, timing := range realized.Timing {
		timingByID[strings.TrimSpace(timing.ID)] = timing
	}
	for _, expected := range manifest.Expected.PCB.TimingFixtures {
		timingID := strings.TrimSpace(expected.ID)
		path := basePath + ".timing." + pathSegment(timingID)
		actual, ok := timingByID[timingID]
		if !ok {
			issues = append(issues, runIssue(path, "missing expected timing fixture "+timingID))
			continue
		}
		if expected.Satisfied != nil && actual.Satisfied != *expected.Satisfied {
			issues = append(issues, runIssue(path+".satisfied", fmt.Sprintf("expected timing fixture satisfied=%t, got %t", *expected.Satisfied, actual.Satisfied)))
		}
		findings := timingFindingIDSet(actual.Findings)
		for _, findingID := range expected.RequiredFindings {
			findingID = strings.TrimSpace(findingID)
			if _, ok := findings[findingID]; !ok {
				issues = append(issues, runIssue(path+".required_findings."+pathSegment(findingID), "missing required timing finding "+findingID))
			}
		}
		for _, findingID := range expected.ForbiddenFindings {
			findingID = strings.TrimSpace(findingID)
			if _, ok := findings[findingID]; ok {
				issues = append(issues, runIssue(path+".forbidden_findings."+pathSegment(findingID), "forbidden timing finding present "+findingID))
			}
		}
	}
	return issues
}

func timingFindingIDSet(findings []blocks.TimingFixtureFinding) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, finding := range findings {
		if id := strings.TrimSpace(finding.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func runWriterStage(ctx context.Context, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) (StageResult, []reports.Artifact) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		if manifest.Expected.Writer.Required {
			return StageResult{
				Name:   "writer_correctness",
				Issues: []reports.Issue{writerRunIssue(manifest, "output_dir", "writer verification requires an output directory")},
			}, nil
		}
		return StageResult{Name: "writer_correctness", Status: StatusSkipped, Summary: "writer verification skipped because no output directory was provided"}, nil
	}
	projectName := pathID(manifest.ID)
	projectDir := caseOutputDir(opts.OutputDir, manifest.ID)
	tx, err := blocks.ProjectTransactionForBlockOutputPtr(projectName, output, opts.Overwrite)
	if err != nil {
		return StageResult{
			Name:    "writer_correctness",
			Issues:  []reports.Issue{writerRunIssue(manifest, "transaction", err.Error())},
			Summary: "failed to build project transaction",
		}, nil
	}
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: projectDir, Overwrite: opts.Overwrite})
	issues := contextualizeIssues(manifest, "apply", apply.Issues)
	artifacts := apply.Artifacts
	if reports.HasBlockingIssue(issues) {
		return StageResult{Name: "writer_correctness", Issues: issues, Summary: "failed to write generated project"}, artifacts
	}
	writerOptions := opts.WriterOptions
	writerOptions.AllowUnrouted = writerOptions.AllowUnrouted || manifest.Expected.Writer.AllowUnrouted
	writerOptions.RequireKiCadRoundTrip = writerOptions.RequireKiCadRoundTrip || manifest.Expected.Writer.RequireRoundTrip
	writerOptions.KeepArtifacts = writerOptions.KeepArtifacts || opts.KeepArtifacts
	writerResult := writercorrectness.Validate(ctx, projectDir, writerOptions)
	artifacts = append(artifacts, writerResult.Artifacts...)
	writerIssues := contextualizeIssues(manifest, "writer", writerResult.Issues)
	issues = append(issues, writerIssues...)
	if manifest.Expected.Writer.OK && !writerResult.OK && len(writerIssues) == 0 {
		issues = append(issues, writerRunIssue(manifest, "ok", fmt.Sprintf("writer correctness did not report OK; checks=%d failures=%d warnings=%d skipped=%d", len(writerResult.Checks), writerResult.OverallSummary.FailCount, writerResult.OverallSummary.WarningCount, writerResult.OverallSummary.SkippedCount)))
	}
	if reports.HasBlockingIssue(issues) {
		summary := fmt.Sprintf("wrote %s and ran %d writer correctness check(s)", projectDir, len(writerResult.Checks))
		return StageResult{Name: "writer_correctness", Issues: issues, Summary: summary}, artifacts
	}
	if err := writeProjectSentinel(projectDir, manifest, output, opts); err != nil {
		return StageResult{
			Name:    "writer_correctness",
			Issues:  []reports.Issue{writerRunIssue(manifest, "sentinel", err.Error())},
			Summary: "failed to mark generated project",
		}, artifacts
	}
	summary := fmt.Sprintf("wrote %s and ran %d writer correctness check(s)", projectDir, len(writerResult.Checks))
	return StageResult{Name: "writer_correctness", Issues: issues, Summary: summary}, artifacts
}

func caseOutputDir(root string, caseID string) string {
	cleanRoot := filepath.Clean(root)
	dir := filepath.Join(cleanRoot, pathID(caseID))
	rel, err := filepath.Rel(cleanRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(cleanRoot, "unknown_"+stableStringHash(caseID))
	}
	return dir
}

func writerRunIssue(manifest Manifest, path string, message string) reports.Issue {
	issue := runIssue("verification."+pathID(manifest.ID)+".writer."+pathSegment(path), message)
	issue.Suggestion = "case " + manifest.ID + " block " + manifest.BlockID
	return issue
}

func ercDRCRequested(expected Expected, opts RunOptions) bool {
	return opts.RequireERC ||
		opts.RequireDRC ||
		expected.ERCDRC.Required ||
		expected.ERCDRC.RequireERC ||
		expected.ERCDRC.RequireDRC ||
		expected.ERCDRC.Runner != "" ||
		expected.ERCDRC.MinKiCadVersion != "" ||
		expected.ERCDRC.MaxKiCadVersion != "" ||
		len(expected.ERCDRC.AllowedCodes) > 0 ||
		len(expected.ERCDRC.ExpectedIssues) > 0 ||
		expected.EvidenceLevel == EvidenceERCDRCVerified ||
		expected.EvidenceLevel == EvidenceReferenceVerified
}

func runERCDRCStage(ctx context.Context, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) (StageResult, []reports.Artifact) {
	requiredERC, requiredDRC := ercDRCRequirements(manifest.Expected, opts)
	if strings.TrimSpace(opts.OutputDir) == "" {
		if requiredERC || requiredDRC {
			return StageResult{
				Name:    "erc_drc",
				Status:  StatusBlocked,
				Issues:  []reports.Issue{ercDRCRunIssue(manifest, "output_dir", "ERC/DRC verification requires an output directory")},
				Summary: "ERC/DRC blocked: missing output directory",
			}, nil
		}
		return StageResult{Name: "erc_drc", Status: StatusSkipped, Summary: "ERC/DRC skipped because no output directory was provided"}, nil
	}

	projectDir, artifacts, issues := ensureProjectForExternalChecks(manifest, output, opts)
	if reports.HasBlockingIssue(issues) {
		return StageResult{Name: "erc_drc", Issues: issues, Summary: "failed to prepare generated project for ERC/DRC"}, artifacts
	}

	activeCheckOpts, allowlistIssues := checkOptions(manifest, opts)
	issues = append(issues, allowlistIssues...)
	if reports.HasBlockingIssue(allowlistIssues) {
		return StageResult{Name: "erc_drc", Issues: issues, Summary: "failed to configure ERC/DRC allowlist"}, artifacts
	}

	cli, err := checks.DiscoverCLI(activeCheckOpts.KiCadCLI)
	if err != nil {
		if requiredERC || requiredDRC {
			issues = append(issues, ercDRCRunIssue(manifest, "kicad_cli", err.Error()))
			return StageResult{Name: "erc_drc", Issues: issues, Summary: "KiCad CLI is required but unavailable"}, artifacts
		}
		return StageResult{Name: "erc_drc", Status: StatusSkipped, Summary: "ERC/DRC skipped because KiCad CLI is unavailable"}, artifacts
	}
	activeCheckOpts.KiCadCLI = cli.Path

	runner := opts.CheckRunner
	if runner == nil {
		runner = defaultCheckRunner
	}
	kinds := ercDRCKinds(manifest.Expected, opts)
	allFindings := []checks.CheckFinding{}
	runFailed := false
	for _, kind := range kinds {
		checkResult, runErr := runner(ctx, kind, cli, projectDir, activeCheckOpts)
		artifacts = append(artifacts, checkArtifacts(checkResult)...)
		issues = append(issues, checkResultIssues(manifest, checkResult, runErr)...)
		if runErr != nil {
			runFailed = true
		}
		allFindings = append(allFindings, checkResult.Findings...)
		allFindings = append(allFindings, checkResult.Allowed...)
	}
	if !runFailed {
		issues = append(issues, missingExpectedCheckIssues(manifest, allFindings, manifest.Expected.ERCDRC.ExpectedIssues)...)
	}

	summary := fmt.Sprintf("ran %d KiCad ERC/DRC check(s) for %s and produced %d artifact(s)", len(kinds), projectDir, len(artifacts))
	return StageResult{Name: "erc_drc", Status: statusForIssues(issues), Issues: issues, Summary: summary}, artifacts
}

func ercDRCRequirements(expected Expected, opts RunOptions) (bool, bool) {
	requireERC := opts.RequireERC || expected.ERCDRC.RequireERC
	requireDRC := opts.RequireDRC || expected.ERCDRC.RequireDRC
	if expected.EvidenceLevel == EvidenceERCDRCVerified || expected.EvidenceLevel == EvidenceReferenceVerified {
		requireERC = true
		requireDRC = true
	}
	if (expected.ERCDRC.Required || expected.ERCDRC.Runner == ERCDRCRunnerRequiredReal) && !expected.ERCDRC.RequireERC && !expected.ERCDRC.RequireDRC {
		requireERC = true
		requireDRC = true
	}
	return requireERC, requireDRC
}

func ercDRCKinds(expected Expected, opts RunOptions) []checks.CheckKind {
	requireERC, requireDRC := ercDRCRequirements(expected, opts)
	if !requireERC && !requireDRC && ercDRCRequested(expected, opts) {
		// Optional evidence policies such as allowed_codes and expected_issues
		// still need concrete checks when KiCad is available.
		requireERC = true
		requireDRC = true
	}
	kinds := make([]checks.CheckKind, 0, 2)
	if requireERC {
		kinds = append(kinds, checks.CheckKindERC)
	}
	if requireDRC {
		kinds = append(kinds, checks.CheckKindDRC)
	}
	return kinds
}

func ensureProjectForExternalChecks(manifest Manifest, output *blocks.BlockOutput, opts RunOptions) (string, []reports.Artifact, []reports.Issue) {
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return "", nil, []reports.Issue{ercDRCRunIssue(manifest, "output_dir", err.Error())}
	}
	projectDir := caseOutputDir(opts.OutputDir, manifest.ID)
	_, err := os.Stat(projectDir)
	if err == nil && !opts.Overwrite {
		if projectSentinelMatches(projectDir, manifest, output, opts) {
			return projectDir, existingProjectArtifacts(projectDir), nil
		}
		return projectDir, nil, []reports.Issue{ercDRCRunIssue(manifest, "project", "existing generated project is stale or incomplete; rerun with overwrite")}
	}
	if err != nil && !os.IsNotExist(err) {
		return projectDir, nil, []reports.Issue{ercDRCRunIssue(manifest, "project", err.Error())}
	}
	tx, err := blocks.ProjectTransactionForBlockOutputPtr(pathID(manifest.ID), output, opts.Overwrite)
	if err != nil {
		return projectDir, nil, []reports.Issue{ercDRCRunIssue(manifest, "transaction", err.Error())}
	}
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: projectDir, Overwrite: opts.Overwrite})
	issues := contextualizeIssues(manifest, "erc_drc.apply", apply.Issues)
	if reports.HasBlockingIssue(issues) {
		return projectDir, apply.Artifacts, issues
	}
	if err := writeProjectSentinel(projectDir, manifest, output, opts); err != nil {
		issues = append(issues, ercDRCRunIssue(manifest, "sentinel", err.Error()))
	}
	return projectDir, apply.Artifacts, issues
}

func checkOptions(manifest Manifest, opts RunOptions) (checks.Options, []reports.Issue) {
	checkOpts := opts.CheckOptions
	checkOpts.Allowlist = append([]checks.AllowlistEntry(nil), opts.CheckOptions.Allowlist...)
	checkOpts.KeepArtifacts = checkOpts.KeepArtifacts || opts.KeepArtifacts
	if strings.TrimSpace(opts.KiCadCLI) != "" {
		checkOpts.KiCadCLI = opts.KiCadCLI
	}
	for _, code := range manifest.Expected.ERCDRC.AllowedCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		checkOpts.Allowlist = append(checkOpts.Allowlist, checks.AllowlistEntry{
			Code:   code,
			Reason: "allowed by block verification manifest " + manifest.ID,
		})
	}
	if strings.TrimSpace(opts.AllowlistPath) == "" {
		return checkOpts, nil
	}
	data, err := os.ReadFile(opts.AllowlistPath)
	if err != nil {
		return checkOpts, []reports.Issue{ercDRCRunIssue(manifest, "allowlist", err.Error())}
	}
	var entries []checks.AllowlistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return checkOpts, []reports.Issue{ercDRCRunIssue(manifest, "allowlist", err.Error())}
	}
	checkOpts.Allowlist = append(checkOpts.Allowlist, entries...)
	return checkOpts, nil
}

func defaultCheckRunner(ctx context.Context, kind checks.CheckKind, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, error) {
	if kind == checks.CheckKindDRC {
		return checks.RunDRC(ctx, cli, target, opts)
	}
	return checks.RunERC(ctx, cli, target, opts)
}

func checkResultIssues(manifest Manifest, result checks.CheckResult, err error) []reports.Issue {
	var issues []reports.Issue
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   checkSeverity(finding.Severity),
			Path:       fmt.Sprintf("verification.%s.erc_drc.%s.%s.%s", pathID(manifest.ID), result.Kind, findingPathSegment(finding), findingFingerprint(finding)),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       checkFindingNets(finding),
			Suggestion: "case " + manifest.ID + " block " + manifest.BlockID,
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       "verification." + pathID(manifest.ID) + ".erc_drc." + string(result.Kind) + ".parser",
			Message:    parserIssue.Message,
			Suggestion: "case " + manifest.ID + " block " + manifest.BlockID,
		})
	}
	if err != nil {
		issues = append(issues, ercDRCRunIssue(manifest, string(result.Kind), err.Error()))
	}
	return issues
}

func missingExpectedCheckIssues(manifest Manifest, findings []checks.CheckFinding, expected []string) []reports.Issue {
	if len(expected) == 0 {
		return nil
	}
	var issues []reports.Issue
	for _, want := range expected {
		want = strings.TrimSpace(want)
		if want == "" || checkFindingMatchesExpectation(findings, want) {
			continue
		}
		issues = append(issues, ercDRCRunIssue(manifest, "expected."+pathSegment(want), "missing expected ERC/DRC issue "+want))
	}
	return issues
}

func checkFindingMatchesExpectation(findings []checks.CheckFinding, want string) bool {
	wantKey := strings.ToLower(strings.TrimSpace(want))
	for _, finding := range findings {
		for _, candidate := range []string{finding.Code, finding.Rule, finding.ID} {
			if strings.ToLower(strings.TrimSpace(candidate)) == wantKey {
				return true
			}
		}
		message := strings.ToLower(strings.TrimSpace(finding.Message))
		if strings.Contains(message, wantKey) {
			return true
		}
	}
	return false
}

func findingPathSegment(finding checks.CheckFinding) string {
	if key := firstNonEmpty(finding.Code, finding.Rule, finding.ID); key != "" {
		return pathSegment(key)
	}
	return "finding"
}

func checkSeverity(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func checkFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	nets := make([]string, 0, len(finding.Nets)+1)
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

func checkArtifacts(result checks.CheckResult) []reports.Artifact {
	if strings.TrimSpace(result.ReportPath) == "" {
		return nil
	}
	kind := reports.ArtifactERCReport
	if result.Kind == checks.CheckKindDRC {
		kind = reports.ArtifactDRCReport
	}
	return []reports.Artifact{{
		Kind:        kind,
		Path:        filepath.ToSlash(result.ReportPath),
		Description: string(result.Kind) + " JSON report",
	}}
}

func existingProjectArtifacts(projectDir string) []reports.Artifact {
	return []reports.Artifact{{
		Kind:        reports.ArtifactKiCadProject,
		Path:        filepath.ToSlash(projectDir),
		Description: "existing generated KiCad project",
	}}
}

func findingFingerprint(finding checks.CheckFinding) string {
	hash := fnv.New64a()
	hashStrings(hash,
		string(finding.Kind),
		finding.Code,
		finding.Rule,
		finding.ID,
		filepath.Base(finding.File),
		finding.Sheet,
		finding.Net,
		finding.Layer,
	)
	hashStringSlice(hash, finding.Nets)
	hashStringSlice(hash, finding.References)
	return fmt.Sprintf("%016x", hash.Sum64())
}

func writeProjectSentinel(projectDir string, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) error {
	return os.WriteFile(filepath.Join(projectDir, projectSentinelName), []byte(projectSignature(manifest, output, opts)), 0o644)
}

func projectSentinelMatches(projectDir string, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) bool {
	data, err := os.ReadFile(filepath.Join(projectDir, projectSentinelName))
	return err == nil && strings.TrimSpace(string(data)) == projectSignature(manifest, output, opts)
}

func projectSignature(manifest Manifest, output *blocks.BlockOutput, opts RunOptions) string {
	hash := fnv.New64a()
	effectiveAllowUnrouted := opts.WriterOptions.AllowUnrouted || manifest.Expected.Writer.AllowUnrouted
	effectiveRoundTrip := opts.WriterOptions.RequireKiCadRoundTrip || manifest.Expected.Writer.RequireRoundTrip
	hashStrings(hash,
		manifest.ID,
		manifest.BlockID,
		fmt.Sprintf("expected_writer_required=%t", manifest.Expected.Writer.Required),
		fmt.Sprintf("expected_writer_ok=%t", manifest.Expected.Writer.OK),
		fmt.Sprintf("expected_writer_allow_unrouted=%t", manifest.Expected.Writer.AllowUnrouted),
		fmt.Sprintf("expected_writer_roundtrip=%t", manifest.Expected.Writer.RequireRoundTrip),
		fmt.Sprintf("effective_writer_allow_unrouted=%t", effectiveAllowUnrouted),
		fmt.Sprintf("effective_writer_roundtrip=%t", effectiveRoundTrip),
	)
	for _, operation := range output.Operations {
		hashStrings(hash, string(operation.Op))
		_, _ = hash.Write(operation.Raw)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", hash.Sum64())
}

func stableStringHash(value string) string {
	hash := fnv.New64a()
	hashStrings(hash, value)
	return fmt.Sprintf("%016x", hash.Sum64())
}

type stringHasher interface {
	Write([]byte) (int, error)
}

func hashStrings(hash stringHasher, values ...string) {
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
}

func hashStringSlice(hash stringHasher, values []string) {
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write([]byte{0})
}

func ercDRCRunIssue(manifest Manifest, path string, message string) reports.Issue {
	issue := runIssue("verification."+pathID(manifest.ID)+".erc_drc."+pathSegment(path), message)
	issue.Suggestion = "case " + manifest.ID + " block " + manifest.BlockID
	return issue
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contextualizeIssues(manifest Manifest, stage string, issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	contextualized := make([]reports.Issue, 0, len(issues))
	prefix := "verification." + pathID(manifest.ID) + "." + pathSegment(stage)
	for _, issue := range issues {
		if strings.TrimSpace(issue.Path) == "" {
			issue.Path = prefix
		} else {
			issue.Path = prefix + "." + strings.TrimPrefix(issue.Path, ".")
		}
		if issue.Suggestion == "" {
			issue.Suggestion = "case " + manifest.ID + " block " + manifest.BlockID
		}
		contextualized = append(contextualized, issue)
	}
	return contextualized
}

func statusForIssues(issues []reports.Issue) Status {
	status := StatusPass
	for _, issue := range issues {
		if issue.Blocking() {
			return StatusBlocked
		}
		if issue.Severity == reports.SeverityError {
			return StatusBlocked
		}
		if issue.Severity == reports.SeverityWarning {
			status = StatusWarning
		}
	}
	return status
}

func summarizeOutput(output blocks.BlockOutput) (semanticSummary, []reports.Issue) {
	summary := semanticSummary{
		Components: map[string]actualComponent{},
		Nets:       map[string][]actualPin{},
		Ports:      map[string]blocks.BlockPort{},
		PCB: actualPCB{
			Placements: map[string]actualPlacement{},
			PadNets:    map[padKey]string{},
			Routes:     map[string]struct{}{},
			ZoneNames:  map[string]struct{}{},
			ZoneNets:   map[string]struct{}{},
		},
	}
	for _, port := range output.Instance.Ports {
		summary.Ports[port.Name] = port
	}
	var issues []reports.Issue
	var connects []connectEdge
	for index, operation := range output.Operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode add_symbol operation %d: %v", index, err)))
				continue
			}
			component := summary.Components[payload.Ref]
			component.Ref = payload.Ref
			component.Role = payload.Role
			component.SymbolID = payload.LibraryID
			component.Value = payload.Value
			summary.Components[payload.Ref] = component
		case transactions.OpAssignFootprint:
			var payload transactions.AssignFootprintOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode assign_footprint operation %d: %v", index, err)))
				continue
			}
			component := summary.Components[payload.Ref]
			component.Ref = payload.Ref
			if component.Role == "" {
				component.Role = payload.Role
			}
			component.FootprintID = payload.FootprintID
			summary.Components[payload.Ref] = component
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode connect operation %d: %v", index, err)))
				continue
			}
			connects = append(connects, connectEdge{
				NetName: strings.TrimSpace(payload.NetName),
				From:    actualPin{Ref: payload.From.Ref, Pin: payload.From.Pin},
				To:      actualPin{Ref: payload.To.Ref, Pin: payload.To.Pin},
			})
		case transactions.OpPlaceFootprint:
			var payload transactions.PlaceFootprintOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode place_footprint operation %d: %v", index, err)))
				continue
			}
			ref := strings.TrimSpace(payload.Ref)
			if ref == "" {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("place_footprint operation %d requires ref", index)))
				continue
			}
			placement := actualPlacement{
				Ref:         ref,
				Role:        payload.Role,
				FootprintID: payload.FootprintID,
				XMM:         payload.At.XMM,
				YMM:         payload.At.YMM,
				RotationDeg: payload.Rotation,
			}
			summary.PCB.Placements[ref] = placement
			for _, pad := range payload.Pads {
				if pad.Net == nil {
					continue
				}
				summary.PCB.PadNets[padKey{Ref: ref, Pad: pad.Name}] = *pad.Net
			}
		case transactions.OpRoute:
			var payload transactions.RouteOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode route operation %d: %v", index, err)))
				continue
			}
			netName := strings.TrimSpace(payload.NetName)
			if netName != "" {
				summary.PCB.Routes[netName] = struct{}{}
			}
		case transactions.OpAddZone:
			var payload transactions.AddZoneOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode add_zone operation %d: %v", index, err)))
				continue
			}
			zoneName := strings.TrimSpace(payload.Name)
			if zoneName != "" {
				summary.PCB.ZoneNames[zoneName] = struct{}{}
			}
			if payload.NetName != nil {
				netName := strings.TrimSpace(*payload.NetName)
				if netName != "" {
					summary.PCB.ZoneNets[netName] = struct{}{}
				}
			}
		}
	}
	summary.Nets = summarizeConnects(connects)
	for netName, pins := range summary.Nets {
		summary.Nets[netName] = uniquePins(pins)
	}
	return summary, issues
}

func summarizeConnects(connects []connectEdge) map[string][]actualPin {
	nets := map[string][]actualPin{}
	if len(connects) == 0 {
		return nets
	}
	sets := newPinDisjointSet()
	for _, connect := range connects {
		sets.union(connect.From, connect.To)
	}
	pinsByRoot := map[actualPin][]actualPin{}
	namesByRoot := map[actualPin]map[string]struct{}{}
	for _, connect := range connects {
		root := sets.find(connect.From)
		pinsByRoot[root] = append(pinsByRoot[root], connect.From, connect.To)
		if connect.NetName != "" {
			if namesByRoot[root] == nil {
				namesByRoot[root] = map[string]struct{}{}
			}
			namesByRoot[root][connect.NetName] = struct{}{}
		}
	}
	anonymousRoots := make([]actualPin, 0, len(pinsByRoot))
	for root, pins := range pinsByRoot {
		pins = uniquePins(pins)
		names := sortedSetValues(namesByRoot[root])
		if len(names) == 0 {
			anonymousRoots = append(anonymousRoots, root)
			pinsByRoot[root] = pins
			continue
		}
		for _, name := range names {
			nets[name] = append(nets[name], pins...)
		}
	}
	slices.SortFunc(anonymousRoots, func(a, b actualPin) int {
		return comparePins(a, b)
	})
	for index, root := range anonymousRoots {
		nets[fmt.Sprintf("__anonymous_net_%d", index)] = pinsByRoot[root]
	}
	return nets
}

func assertSemantics(manifest Manifest, summary semanticSummary, opts RunOptions) []reports.Issue {
	var issues []reports.Issue
	roleToComponents := componentsByRole(summary.Components)
	componentIssues, matchedRefs := assertExpectedComponents(manifest, summary, roleToComponents)
	issues = append(issues, componentIssues...)
	for _, expected := range manifest.Expected.Ports {
		path := "verification." + pathID(manifest.ID) + ".ports." + pathSegment(expected.Name)
		port, ok := summary.Ports[expected.Name]
		if !ok {
			issues = append(issues, runIssue(path, "missing expected port "+expected.Name))
			continue
		}
		if expected.Direction != "" && string(port.Direction) != expected.Direction {
			issues = append(issues, runIssue(path+".direction", fmt.Sprintf("expected direction %s, got %s", expected.Direction, port.Direction)))
		}
	}
	for _, expected := range manifest.Expected.Nets {
		path := "verification." + pathID(manifest.ID) + ".nets." + pathSegment(expected.Name)
		actualPins, ok := summary.Nets[expected.Name]
		if !ok {
			issues = append(issues, runIssue(path, "missing expected net "+expected.Name))
			continue
		}
		actualPinSet := pinSet(actualPins)
		for _, expectedPin := range expected.Pins {
			ref := expectedPin.Ref
			if ref == "" && expectedPin.Role != "" {
				if !rolePinInNet(actualPinSet, roleToComponents[expectedPin.Role], expectedPin.Pin) {
					issues = append(issues, runIssue(path+".pins."+pathSegment(expectedPin.Role), fmt.Sprintf("expected role %s pin %s on net %s", expectedPin.Role, expectedPin.Pin, expected.Name)))
				}
				continue
			}
			if _, ok := actualPinSet[actualPin{Ref: ref, Pin: expectedPin.Pin}]; !ok {
				issues = append(issues, runIssue(path+".pins."+pathSegment(ref)+"."+pathSegment(expectedPin.Pin), fmt.Sprintf("expected pin %s:%s on net %s", ref, expectedPin.Pin, expected.Name)))
			}
		}
	}
	if opts.Strict {
		issues = append(issues, assertStrictSemantics(manifest, summary, matchedRefs)...)
	}
	return issues
}

func pcbAssertionsRequested(expected Expected) bool {
	if expected.EvidenceLevel == EvidencePCBVerified || expected.EvidenceLevel == EvidenceERCDRCVerified || expected.EvidenceLevel == EvidenceReferenceVerified {
		return true
	}
	return len(expected.PCB.Placements) > 0 ||
		len(expected.PCB.PadNets) > 0 ||
		len(expected.PCB.RequiredRoutes) > 0 ||
		len(expected.PCB.RequiredZones) > 0 ||
		expected.PCB.RequireRoutes ||
		expected.PCB.RequireZones
}

func assertPCB(manifest Manifest, summary semanticSummary) []reports.Issue {
	var issues []reports.Issue
	basePath := "verification." + pathID(manifest.ID) + ".pcb"
	roleToPlacements := placementsByRole(summary.PCB.Placements)
	matchedPlacements := map[string]struct{}{}
	for _, item := range orderedExpectedPlacements(manifest.Expected.PCB.Placements) {
		index := item.Index
		expected := item.Placement
		path := fmt.Sprintf("%s.placements.%d", basePath, index)
		placement, ok := matchPlacement(expected, summary.PCB.Placements, roleToPlacements, matchedPlacements)
		if !ok {
			if candidate, candidateOK := placementCandidate(expected, summary.PCB.Placements, roleToPlacements, matchedPlacements); candidateOK {
				issues = append(issues, runIssue(path, placementMismatchMessage(expected, candidate)))
				continue
			}
			issues = append(issues, runIssue(path, "missing expected PCB placement"))
			continue
		}
		matchedPlacements[placement.Ref] = struct{}{}
	}
	for _, expected := range manifest.Expected.PCB.PadNets {
		path := basePath + ".pad_nets." + pathSegment(expected.Ref) + "." + pathSegment(expected.Pad)
		net, ok := summary.PCB.PadNets[padKey{Ref: expected.Ref, Pad: expected.Pad}]
		if !ok {
			issues = append(issues, runIssue(path, fmt.Sprintf("missing expected pad net %s:%s=%s", expected.Ref, expected.Pad, expected.Net)))
			continue
		}
		if net != expected.Net {
			issues = append(issues, runIssue(path, fmt.Sprintf("expected pad net %s, got %s", expected.Net, net)))
		}
	}
	if manifest.Expected.PCB.RequireRoutes && len(summary.PCB.Routes) == 0 {
		issues = append(issues, runIssue(basePath+".routes", "expected at least one route"))
	}
	for _, netName := range manifest.Expected.PCB.RequiredRoutes {
		netName = strings.TrimSpace(netName)
		if _, ok := summary.PCB.Routes[netName]; !ok {
			issues = append(issues, runIssue(basePath+".routes."+pathSegment(netName), "missing required route "+netName))
		}
	}
	if manifest.Expected.PCB.RequireZones && len(summary.PCB.ZoneNames) == 0 && len(summary.PCB.ZoneNets) == 0 {
		issues = append(issues, runIssue(basePath+".zones", "expected at least one zone"))
	}
	for _, zone := range manifest.Expected.PCB.RequiredZones {
		zone = strings.TrimSpace(zone)
		_, nameOK := summary.PCB.ZoneNames[zone]
		_, netOK := summary.PCB.ZoneNets[zone]
		if !nameOK && !netOK {
			issues = append(issues, runIssue(basePath+".zones."+pathSegment(zone), "missing required zone "+zone))
		}
	}
	return issues
}

type expectedPlacementItem struct {
	Index     int
	Placement ExpectedPlacement
	Score     int
}

func orderedExpectedPlacements(placements []ExpectedPlacement) []expectedPlacementItem {
	items := make([]expectedPlacementItem, 0, len(placements))
	for index, placement := range placements {
		items = append(items, expectedPlacementItem{
			Index:     index,
			Placement: placement,
			Score:     placementSpecificity(placement),
		})
	}
	slices.SortStableFunc(items, func(a, b expectedPlacementItem) int {
		return b.Score - a.Score
	})
	return items
}

func placementSpecificity(placement ExpectedPlacement) int {
	score := 0
	if strings.TrimSpace(placement.Ref) != "" {
		score += 100
	}
	if strings.TrimSpace(placement.FootprintID) != "" {
		score += 20
	}
	if placement.XMM != nil {
		score += 10
	}
	if placement.YMM != nil {
		score += 10
	}
	if placement.RotationDeg != nil {
		score += 10
	}
	return score
}

func placementsByRole(placements map[string]actualPlacement) map[string][]actualPlacement {
	byRole := map[string][]actualPlacement{}
	for _, placement := range placements {
		byRole[placement.Role] = append(byRole[placement.Role], placement)
	}
	for role := range byRole {
		slices.SortFunc(byRole[role], func(a, b actualPlacement) int {
			return strings.Compare(a.Ref, b.Ref)
		})
	}
	return byRole
}

func matchPlacement(expected ExpectedPlacement, placements map[string]actualPlacement, roleToPlacements map[string][]actualPlacement, used map[string]struct{}) (actualPlacement, bool) {
	expectedRef := strings.TrimSpace(expected.Ref)
	if expectedRef != "" {
		placement, ok := placements[expectedRef]
		if !ok {
			return actualPlacement{}, false
		}
		if expected.Role != "" && placement.Role != expected.Role {
			return actualPlacement{}, false
		}
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			return actualPlacement{}, false
		}
		if !placementMatchesExpected(placement, expected) {
			return actualPlacement{}, false
		}
		return placement, ok
	}
	matches := roleToPlacements[expected.Role]
	for _, placement := range matches {
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			continue
		}
		if !expectedPlacementHasConstraints(expected) || placementMatchesExpected(placement, expected) {
			return placement, true
		}
	}
	return actualPlacement{}, false
}

func placementCandidate(expected ExpectedPlacement, placements map[string]actualPlacement, roleToPlacements map[string][]actualPlacement, used map[string]struct{}) (actualPlacement, bool) {
	expectedRef := strings.TrimSpace(expected.Ref)
	if expectedRef != "" {
		placement, ok := placements[expectedRef]
		if !ok {
			return actualPlacement{}, false
		}
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			return actualPlacement{}, false
		}
		return placement, true
	}
	for _, placement := range roleToPlacements[expected.Role] {
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			continue
		}
		return placement, true
	}
	return actualPlacement{}, false
}

func placementMismatchMessage(expected ExpectedPlacement, placement actualPlacement) string {
	role := expected.Role
	if role == "" {
		role = "any"
	}
	footprint := expected.FootprintID
	if footprint == "" {
		footprint = "any"
	}
	actualFootprint := placement.FootprintID
	if actualFootprint == "" {
		actualFootprint = "none"
	}
	return fmt.Sprintf(
		"expected placement role %s footprint %s at %s,%s rot %s got ref %s role %s footprint %s at %.6f,%.6f rot %.6f",
		role,
		footprint,
		expectedFloatString(expected.XMM),
		expectedFloatString(expected.YMM),
		expectedFloatString(expected.RotationDeg),
		placement.Ref,
		placement.Role,
		actualFootprint,
		placement.XMM,
		placement.YMM,
		placement.RotationDeg,
	)
}

func placementMatchesExpected(placement actualPlacement, expected ExpectedPlacement) bool {
	if expected.FootprintID != "" && placement.FootprintID != expected.FootprintID {
		return false
	}
	toleranceMM, toleranceDeg := placementTolerances(expected)
	if expected.XMM != nil && math.Abs(placement.XMM-*expected.XMM) > toleranceMM {
		return false
	}
	if expected.YMM != nil && math.Abs(placement.YMM-*expected.YMM) > toleranceMM {
		return false
	}
	if expected.RotationDeg != nil && angleDeltaDeg(placement.RotationDeg, *expected.RotationDeg) > toleranceDeg {
		return false
	}
	return true
}

func placementTolerances(expected ExpectedPlacement) (float64, float64) {
	tolerance := defaultPlacementToleranceMM
	if expected.ToleranceMM != nil {
		tolerance = *expected.ToleranceMM
	}
	rotationTolerance := defaultPlacementToleranceDeg
	if expected.ToleranceDeg != nil {
		rotationTolerance = *expected.ToleranceDeg
	}
	return tolerance, rotationTolerance
}

func expectedPlacementHasConstraints(expected ExpectedPlacement) bool {
	return expected.FootprintID != "" || expected.XMM != nil || expected.YMM != nil || expected.RotationDeg != nil
}

func expectedFloatString(value *float64) string {
	if value == nil {
		return "any"
	}
	return fmt.Sprintf("%.6f", *value)
}

func angleDeltaDeg(a float64, b float64) float64 {
	diff := math.Mod(math.Abs(a-b), 360)
	if diff > 180 {
		return 360 - diff
	}
	return diff
}

func assertExpectedComponents(manifest Manifest, summary semanticSummary, roleToComponents map[string][]actualComponent) ([]reports.Issue, map[string]struct{}) {
	var issues []reports.Issue
	matchedRefs := map[string]struct{}{}
	for index, expected := range manifest.Expected.Components {
		if expected.Ref == "" {
			continue
		}
		issues = append(issues, assertExpectedComponent(manifest, index, expected, summary.Components, roleToComponents, matchedRefs)...)
	}
	for index, expected := range manifest.Expected.Components {
		if expected.Ref != "" {
			continue
		}
		issues = append(issues, assertExpectedComponent(manifest, index, expected, summary.Components, roleToComponents, matchedRefs)...)
	}
	return issues, matchedRefs
}

func assertExpectedComponent(manifest Manifest, index int, expected ExpectedComponent, components map[string]actualComponent, roleToComponents map[string][]actualComponent, matchedRefs map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	component, ok := matchComponent(expected, components, roleToComponents, matchedRefs)
	path := expectedComponentIssuePath(manifest, index, expected)
	if !ok {
		issues = append(issues, runIssue(path, "missing expected component "+expectedComponentPathID(expected)))
		return issues
	}
	if expected.Role != "" && component.Role != expected.Role {
		issues = append(issues, runIssue(path+".role", fmt.Sprintf("expected role %s, got %s", expected.Role, component.Role)))
	}
	if expected.SymbolID != "" && component.SymbolID != expected.SymbolID {
		issues = append(issues, runIssue(path+".symbol_id", fmt.Sprintf("expected symbol %s, got %s", expected.SymbolID, component.SymbolID)))
	}
	if expected.FootprintID != "" && component.FootprintID != expected.FootprintID {
		issues = append(issues, runIssue(path+".footprint_id", fmt.Sprintf("expected footprint %s, got %s", expected.FootprintID, component.FootprintID)))
	}
	if expected.RefPrefix != "" && !strings.HasPrefix(component.Ref, expected.RefPrefix) {
		issues = append(issues, runIssue(path+".ref_prefix", fmt.Sprintf("expected ref prefix %s, got %s", expected.RefPrefix, component.Ref)))
	}
	if expected.Value != "" && component.Value != expected.Value {
		issues = append(issues, runIssue(path+".value", fmt.Sprintf("expected value %s, got %s", expected.Value, component.Value)))
	}
	return issues
}

func expectedComponentPathID(expected ExpectedComponent) string {
	if strings.TrimSpace(expected.Role) != "" && strings.TrimSpace(expected.Ref) != "" {
		return expected.Role + "." + expected.Ref
	}
	if strings.TrimSpace(expected.Role) != "" {
		return expected.Role
	}
	return expected.Ref
}

func expectedComponentIssuePath(manifest Manifest, index int, expected ExpectedComponent) string {
	path := "verification." + pathID(manifest.ID) + ".components." + pathSegment(expectedComponentPathID(expected))
	if expected.Ref == "" {
		path += fmt.Sprintf(".%d", index)
	}
	return path
}

func assertStrictSemantics(manifest Manifest, summary semanticSummary, matchedRefs map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	for _, component := range summary.Components {
		if _, ok := matchedRefs[component.Ref]; ok {
			continue
		}
		issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".components."+pathSegment(component.Ref), "unexpected generated component "+component.Ref))
	}
	expectedPorts := map[string]struct{}{}
	for _, port := range manifest.Expected.Ports {
		expectedPorts[port.Name] = struct{}{}
	}
	for portName := range summary.Ports {
		if _, ok := expectedPorts[portName]; !ok {
			issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".ports."+pathSegment(portName), "unexpected generated port "+portName))
		}
	}
	expectedNetNames := expectedNetNameSet(manifest.Expected.Nets)
	for netName := range summary.Nets {
		if _, ok := expectedNetNames[netName]; !ok {
			issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".nets."+pathSegment(netName), "unexpected generated net "+netName))
		}
	}
	return issues
}

func matchComponent(expected ExpectedComponent, components map[string]actualComponent, roleToComponents map[string][]actualComponent, matchedRefs map[string]struct{}) (actualComponent, bool) {
	if expected.Ref != "" {
		component, ok := components[expected.Ref]
		if ok {
			if _, used := matchedRefs[component.Ref]; used {
				return actualComponent{}, false
			}
			matchedRefs[component.Ref] = struct{}{}
		}
		return component, ok
	}
	matches := roleToComponents[expected.Role]
	for _, component := range matches {
		if !componentMatchesExpected(component, expected) {
			continue
		}
		if _, used := matchedRefs[component.Ref]; used {
			continue
		}
		matchedRefs[component.Ref] = struct{}{}
		return component, true
	}
	return actualComponent{}, false
}

func componentMatchesExpected(component actualComponent, expected ExpectedComponent) bool {
	return expected.RefPrefix == "" || strings.HasPrefix(component.Ref, expected.RefPrefix)
}

func rolePinInNet(actualPinSet map[actualPin]struct{}, components []actualComponent, pin string) bool {
	for _, component := range components {
		if _, ok := actualPinSet[actualPin{Ref: component.Ref, Pin: pin}]; ok {
			return true
		}
	}
	return false
}

func componentsByRole(components map[string]actualComponent) map[string][]actualComponent {
	byRole := map[string][]actualComponent{}
	for _, component := range components {
		byRole[component.Role] = append(byRole[component.Role], component)
	}
	for role := range byRole {
		slices.SortFunc(byRole[role], func(a, b actualComponent) int {
			if a.Ref < b.Ref {
				return -1
			}
			if a.Ref > b.Ref {
				return 1
			}
			return 0
		})
	}
	return byRole
}

func uniquePins(pins []actualPin) []actualPin {
	seen := map[actualPin]struct{}{}
	unique := make([]actualPin, 0, len(pins))
	for _, pin := range pins {
		if _, ok := seen[pin]; ok {
			continue
		}
		seen[pin] = struct{}{}
		unique = append(unique, pin)
	}
	slices.SortFunc(unique, func(a, b actualPin) int {
		return comparePins(a, b)
	})
	return unique
}

func pinSet(pins []actualPin) map[actualPin]struct{} {
	set := make(map[actualPin]struct{}, len(pins))
	for _, actual := range pins {
		set[actual] = struct{}{}
	}
	return set
}

type pinDisjointSet struct {
	parent map[actualPin]actualPin
}

func newPinDisjointSet() *pinDisjointSet {
	return &pinDisjointSet{parent: map[actualPin]actualPin{}}
}

func (sets *pinDisjointSet) find(pin actualPin) actualPin {
	parent, ok := sets.parent[pin]
	if !ok {
		sets.parent[pin] = pin
		return pin
	}
	root := pin
	for parent != root {
		root = parent
		parent = sets.parent[root]
	}
	for current := pin; sets.parent[current] != root; {
		next := sets.parent[current]
		sets.parent[current] = root
		current = next
	}
	return root
}

func (sets *pinDisjointSet) union(a actualPin, b actualPin) {
	rootA := sets.find(a)
	rootB := sets.find(b)
	if rootA == rootB {
		return
	}
	if comparePins(rootB, rootA) < 0 {
		rootA, rootB = rootB, rootA
	}
	sets.parent[rootB] = rootA
}

func sortedSetValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	slices.Sort(sorted)
	return sorted
}

func comparePins(a actualPin, b actualPin) int {
	if a.Ref < b.Ref {
		return -1
	}
	if a.Ref > b.Ref {
		return 1
	}
	return comparePinNames(a.Pin, b.Pin)
}

func comparePinNames(a string, b string) int {
	aIndex, bIndex := 0, 0
	for aIndex < len(a) && bIndex < len(b) {
		aDigit := isASCIIDigit(a[aIndex])
		bDigit := isASCIIDigit(b[bIndex])
		if aDigit && bDigit {
			aStart, bStart := aIndex, bIndex
			for aIndex < len(a) && isASCIIDigit(a[aIndex]) {
				aIndex++
			}
			for bIndex < len(b) && isASCIIDigit(b[bIndex]) {
				bIndex++
			}
			if compare := compareDigitRuns(a[aStart:aIndex], b[bStart:bIndex]); compare != 0 {
				return compare
			}
			continue
		}
		if a[aIndex] < b[bIndex] {
			return -1
		}
		if a[aIndex] > b[bIndex] {
			return 1
		}
		aIndex++
		bIndex++
	}
	if aIndex < len(a) {
		return 1
	}
	if bIndex < len(b) {
		return -1
	}
	return 0
}

func compareDigitRuns(a string, b string) int {
	aTrimmed := trimLeadingZeros(a)
	bTrimmed := trimLeadingZeros(b)
	if len(aTrimmed) < len(bTrimmed) {
		return -1
	}
	if len(aTrimmed) > len(bTrimmed) {
		return 1
	}
	if aTrimmed < bTrimmed {
		return -1
	}
	if aTrimmed > bTrimmed {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func trimLeadingZeros(value string) string {
	trimmed := strings.TrimLeft(value, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func decodeOperation(operation transactions.Operation, payload any) error {
	if len(operation.Raw) == 0 {
		return fmt.Errorf("operation %s has no raw payload", operation.Op)
	}
	return json.Unmarshal(operation.Raw, payload)
}

func runIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}

func warningIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: path, Message: message}
}
