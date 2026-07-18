package fabrication

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/fabrication/physicalrules"
	fabricationprofiles "kicadai/internal/fabrication/profiles"
	"kicadai/internal/inspect"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	projectfiles "kicadai/internal/kicadfiles/project"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

type EvaluateOptions struct {
	KiCadCLI               string
	DryRun                 bool
	CLIPolicy              CLIPolicy
	ManufacturerProfile    string
	ManufacturerProfileDir string
	LibraryIndex           libraryresolver.LibraryIndex
	HasLibraryIndex        bool
	LibraryIssues          []reports.Issue
}

func Evaluate(ctx context.Context, targetPath string, opts EvaluateOptions) Result {
	target, err := resolveEvaluationTarget(targetPath)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "target",
			Message:  err.Error(),
		}
		evidence := map[string]EvidenceStatus{"project": EvidenceFail}
		return Result{
			Status:    CalculateStatus([]reports.Issue{issue}, evidence),
			Score:     Score(evidence),
			Summary:   Summary{Project: EvidenceFail},
			Issues:    []reports.Issue{issue},
			Artifacts: expectedFabricationArtifacts(),
			DryRun:    opts.DryRun,
		}
	}

	summary, err := inspect.ProjectContextWithProjectPath(ctx, target.Root, target.ProjectPath)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(target.Root),
			Message:  err.Error(),
		}
		evidence := map[string]EvidenceStatus{"project": EvidenceFail}
		return Result{
			Status:    CalculateStatus([]reports.Issue{issue}, evidence),
			Score:     Score(evidence),
			Summary:   Summary{Project: EvidenceFail},
			Issues:    []reports.Issue{issue},
			Artifacts: expectedFabricationArtifacts(),
			DryRun:    opts.DryRun,
		}
	}

	issues := slices.Clone(summary.Issues)
	evidence := map[string]EvidenceStatus{}
	resultSummary := Summary{}
	artifacts := expectedFabricationArtifacts()

	addPresenceEvidence(summary, evidence, &resultSummary, &issues)
	addManifestEvidence(summary, evidence, &resultSummary, &issues)
	validation := runValidationEvidence(ctx, target.Root, opts)
	evidence["writer_correctness"] = validation.Writer
	resultSummary.WriterCorrectness = validation.Writer
	evidence["board_validation"] = validation.Board
	resultSummary.BoardValidation = validation.Board
	evidence["drc"] = validation.DRC
	resultSummary.DRC = validation.DRC
	issues = append(issues, validation.Issues...)
	physical := evaluatePhysicalRules(target, opts)
	evidence["physical_rules"] = physicalStatusEvidence(physical.Status)
	resultSummary.PhysicalRules = evidence["physical_rules"]
	issues = append(issues, physical.Issues...)
	var manufacturerProfile *physicalrules.ProfileInfo
	if physical.ProfileDetails != nil && strings.TrimSpace(physical.ProfileDetails.ID) != "" && strings.TrimSpace(physical.ProfileDetails.Hash) != "" {
		profile := *physical.ProfileDetails
		manufacturerProfile = &profile
		if resultSummary.ManufacturerProfile == "" || resultSummary.ManufacturerProfile == EvidenceSkipped {
			resultSummary.ManufacturerProfile = EvidencePass
			evidence["manufacturer_profile"] = EvidencePass
		}
	}
	addERCEvidence(summary, opts, evidence, &resultSummary, &issues)
	addMissingExternalEvidence(opts, evidence, &resultSummary, &issues)

	issues = dedupeIssues(issues)
	slices.SortFunc(issues, compareIssues)
	status := CalculateStatus(issues, evidence)
	return Result{
		Status:              status,
		Score:               Score(evidence),
		Summary:             resultSummary,
		Issues:              issues,
		Artifacts:           artifacts,
		ManufacturerProfile: manufacturerProfile,
		PhysicalRules:       &physical,
		DryRun:              opts.DryRun,
	}
}

type evaluationTarget struct {
	Root        string
	Name        string
	ProjectPath string
}

func evaluatePhysicalRules(target evaluationTarget, opts EvaluateOptions) physicalrules.Report {
	ruleOptions, optionIssues := physicalRuleOptions(opts)
	pcbPath, pcbIssue := discoverPlotPCBPath(target.Root, target.Name)
	if pcbIssue != nil {
		report := physicalrules.EvaluateBoard(nil, nil, ruleOptions)
		report.Issues = append(report.Issues, *pcbIssue)
		report.Issues = append(report.Issues, optionIssues...)
		return physicalrules.Normalize(report)
	}
	board, err := pcbfiles.ReadFile(pcbPath)
	if err != nil {
		report := physicalrules.EvaluateBoard(nil, nil, ruleOptions)
		report.Issues = append(report.Issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(pcbPath),
			Message:  err.Error(),
		})
		report.Issues = append(report.Issues, optionIssues...)
		return physicalrules.Normalize(report)
	}
	var projectPtr *projectfiles.ProjectFile
	var projectIssues []reports.Issue
	if strings.TrimSpace(target.ProjectPath) != "" {
		project, err := projectfiles.ReadFile(target.ProjectPath)
		if err == nil {
			projectPtr = &project
		} else {
			projectIssues = append(projectIssues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       filepath.ToSlash(target.ProjectPath),
				Message:    err.Error(),
				Suggestion: "repair the .kicad_pro file so net classes and fabrication constraints can be evaluated",
			})
		}
	}
	report := physicalrules.EvaluateBoard(&board, projectPtr, ruleOptions)
	report.Board.Path = filepath.ToSlash(pcbPath)
	report.Issues = append(report.Issues, projectIssues...)
	report.Issues = append(report.Issues, optionIssues...)
	return physicalrules.Normalize(report)
}

func physicalRuleOptions(opts EvaluateOptions) (physicalrules.Options, []reports.Issue) {
	registry, issues := fabricationprofiles.LoadRegistry(fabricationprofiles.LoadOptions{ProfileDir: opts.ManufacturerProfileDir})
	profileID := strings.TrimSpace(opts.ManufacturerProfile)
	if profileID == "" {
		profileID = fabricationprofiles.DefaultProfileID
	}
	profile, resolveIssues := registry.Resolve(profileID)
	issues = append(issues, resolveIssues...)
	if len(resolveIssues) > 0 {
		return physicalrules.Options{ProfileID: profileID}, issues
	}
	summary := fabricationprofiles.Summarize(profile)
	ruleOptions := physicalrules.Options{
		ProfileID:                 profile.ID,
		ProfileDetails:            profileInfo(summary),
		RequireCourtyard:          profile.Assembly.RequireCourtyards,
		MinCopperEdgeMM:           profile.Copper.MinCopperToEdgeMM,
		MinHoleEdgeMM:             profile.Drill.MinHoleToEdgeMM,
		MinPlatedPadAnnularRingMM: profile.Drill.MinPadAnnularRingMM,
		MinViaRingMM:              profile.Drill.MinViaAnnularRingMM,
		MinCopperFeatureMM:        profile.Copper.MinCopperSliverMM,
		MinSolderMaskWebMM:        profile.SolderMask.MinSolderMaskWebMM,
		EdgePlatingPolicy:         edgePlatingPolicy(profile),
		RequireBoardFinish:        profile.Metadata.RequireBoardFinish,
		RequireFabricationNotes:   profile.Metadata.RequireFabricationNotes || profile.EdgePlating.RequiresEdgePlatingNotes,
		ImpedancePolicy:           impedancePolicy(profile),
		PanelizationPolicy:        panelizationPolicy(profile),
	}
	return ruleOptions, issues
}

func profileInfo(summary fabricationprofiles.Summary) *physicalrules.ProfileInfo {
	return &physicalrules.ProfileInfo{
		ID:         summary.ID,
		Name:       summary.Name,
		Version:    summary.Version,
		SourceKind: string(summary.Source.Kind),
		SourcePath: summary.Source.Path,
		Hash:       summary.Hash,
	}
}

func edgePlatingPolicy(profile fabricationprofiles.Profile) physicalrules.Policy {
	if profile.EdgePlating.AllowCastellations || profile.EdgePlating.AllowEdgePlating {
		return physicalrules.PolicyAllow
	}
	if profile.EdgePlating.RequiresManualReview || profile.EdgePlating.RequiresEdgePlatingNotes {
		return physicalrules.PolicyWarn
	}
	return physicalrules.PolicyWarn
}

func impedancePolicy(profile fabricationprofiles.Profile) physicalrules.Policy {
	if profile.Impedance.AllowClaimsWithoutSolver {
		return physicalrules.PolicyAllow
	}
	if profile.Impedance.RequireStackupForImpedance ||
		profile.Impedance.RequireDiffPairWidthGapEvidence ||
		profile.Impedance.RequireDiffPairSkewEvidence {
		return physicalrules.PolicyWarn
	}
	return physicalrules.PolicyIgnore
}

func panelizationPolicy(profile fabricationprofiles.Profile) physicalrules.Policy {
	if profile.Metadata.RequirePanelization {
		return physicalrules.PolicyWarn
	}
	return physicalrules.PolicyIgnore
}

func physicalStatusEvidence(status physicalrules.Status) EvidenceStatus {
	switch status {
	case physicalrules.StatusPass:
		return EvidencePass
	case physicalrules.StatusWarning:
		return EvidenceWarning
	case physicalrules.StatusBlocked:
		return EvidenceFail
	case physicalrules.StatusSkipped:
		return EvidenceSkipped
	default:
		return EvidenceFail
	}
}

func resolveEvaluationTarget(targetPath string) (evaluationTarget, error) {
	trimmed := strings.TrimSpace(targetPath)
	if trimmed == "" {
		return evaluationTarget{}, fmt.Errorf("fabrication readiness target is required")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return evaluationTarget{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return evaluationTarget{}, err
	}
	root := absolute
	if !info.IsDir() {
		ext := filepath.Ext(absolute)
		switch {
		case strings.EqualFold(ext, ".kicad_pro"):
			return evaluationTarget{
				Root:        filepath.Dir(absolute),
				Name:        inspect.ProjectNameFromPath(absolute),
				ProjectPath: absolute,
			}, nil
		case strings.EqualFold(ext, ".kicad_pcb"), strings.EqualFold(ext, ".kicad_sch"):
			root = filepath.Dir(absolute)
		default:
			return evaluationTarget{}, fmt.Errorf("target must be a KiCad project directory, .kicad_pro file, .kicad_sch file, or .kicad_pcb file")
		}
	}
	projectPath, ok, err := inspect.DiscoverProjectFile(root)
	if err != nil {
		return evaluationTarget{}, err
	}
	if !ok {
		return evaluationTarget{}, fmt.Errorf("%s: no .kicad_pro file found", filepath.ToSlash(root))
	}
	return evaluationTarget{Root: root, Name: inspect.ProjectNameFromPath(projectPath), ProjectPath: projectPath}, nil
}

func addPresenceEvidence(summary inspect.ProjectSummary, evidence map[string]EvidenceStatus, resultSummary *Summary, issues *[]reports.Issue) {
	project := EvidenceMissing
	schematic := EvidenceMissing
	pcb := EvidenceMissing
	for _, file := range summary.Files {
		switch file.Kind {
		case "project":
			if file.Exists {
				project = EvidencePass
			} else if project != EvidencePass {
				project = EvidenceMissing
				*issues = append(*issues, missingCoreFileIssue("project", file.Path))
			}
		case "schematic":
			if file.Exists {
				schematic = EvidencePass
			} else if schematic != EvidencePass {
				schematic = EvidenceMissing
				*issues = append(*issues, missingCoreFileIssue("schematic", file.Path))
			}
		case "pcb":
			if file.Exists {
				pcb = EvidencePass
			} else if pcb != EvidencePass {
				pcb = EvidenceMissing
				*issues = append(*issues, missingCoreFileIssue("pcb", file.Path))
			}
		}
	}
	if summary.Schematic != nil && len(summary.Schematic.Issues) > 0 {
		*issues = appendInspectionSubIssues(*issues, summary.Schematic.Issues)
		schematic = statusForIssues(summary.Schematic.Issues)
	}
	if summary.PCB != nil && len(summary.PCB.Issues) > 0 {
		*issues = appendInspectionSubIssues(*issues, summary.PCB.Issues)
		pcb = statusForIssues(summary.PCB.Issues)
	}
	evidence["project"] = project
	evidence["schematic"] = schematic
	evidence["pcb"] = pcb
	resultSummary.Project = project
	resultSummary.Schematic = schematic
	resultSummary.PCB = pcb
}

func appendInspectionSubIssues(issues []reports.Issue, subIssues []reports.Issue) []reports.Issue {
	if len(subIssues) == 0 {
		return issues
	}
	return dedupeIssues(append(issues, subIssues...))
}

func addManifestEvidence(summary inspect.ProjectSummary, evidence map[string]EvidenceStatus, resultSummary *Summary, issues *[]reports.Issue) {
	status := EvidenceMissing
	switch {
	case summary.Manifest.Present && !summary.Manifest.Stale:
		status = EvidencePass
		resultSummary.Generated = true
	case summary.Manifest.Present && summary.Manifest.Stale:
		status = EvidenceWarning
		*issues = append(*issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     summary.Manifest.Path,
			Message:  "KiCadAI generated-project manifest is stale",
		})
	default:
		*issues = append(*issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "manifest",
			Message:  "no KiCadAI generated-project provenance was found; project is preview-only",
		})
	}
	evidence["manifest"] = status
	resultSummary.Manifest = status
}

type validationEvidence struct {
	Writer EvidenceStatus
	Board  EvidenceStatus
	DRC    EvidenceStatus
	Issues []reports.Issue
}

type writerEvidenceResult struct {
	Status EvidenceStatus
	Issues []reports.Issue
}

type boardEvidenceResult struct {
	Status EvidenceStatus
	DRC    EvidenceStatus
	Issues []reports.Issue
}

func runValidationEvidence(ctx context.Context, root string, opts EvaluateOptions) validationEvidence {
	writerCh := make(chan writerEvidenceResult, 1)
	boardCh := make(chan boardEvidenceResult, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				writerCh <- writerEvidenceResult{Status: EvidenceFail, Issues: []reports.Issue{panicIssue("writer_correctness", recovered)}}
			}
		}()
		writerCh <- evaluateWriterEvidence(ctx, root, opts)
	}()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				boardCh <- boardEvidenceResult{Status: EvidenceFail, DRC: EvidenceMissing, Issues: []reports.Issue{panicIssue("board_validation", recovered)}}
			}
		}()
		boardCh <- evaluateBoardEvidence(ctx, root, opts)
	}()
	var writer writerEvidenceResult
	var board boardEvidenceResult
	writerDone := false
	boardDone := false
	for !writerDone || !boardDone {
		select {
		case writer = <-writerCh:
			writerDone = true
		case board = <-boardCh:
			boardDone = true
		case <-ctx.Done():
			return canceledValidationEvidence(ctx.Err())
		}
	}
	return validationEvidence{
		Writer: writer.Status,
		Board:  board.Status,
		DRC:    board.DRC,
		Issues: append(writer.Issues, board.Issues...),
	}
}

func panicIssue(path string, recovered any) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  fmt.Sprintf("validation panic: %v", recovered),
	}
}

func canceledValidationEvidence(err error) validationEvidence {
	message := "validation canceled"
	if err != nil {
		message = err.Error()
	}
	return validationEvidence{
		Writer: EvidenceFail,
		Board:  EvidenceFail,
		DRC:    EvidenceMissing,
		Issues: []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityError,
			Path:     "validation.context",
			Message:  message,
		}},
	}
}

func evaluateWriterEvidence(ctx context.Context, root string, opts EvaluateOptions) writerEvidenceResult {
	writer := writercorrectness.Validate(ctx, root, writercorrectness.Options{
		KiCadCLI:        validationKiCadCLI(opts),
		LibraryIndex:    opts.LibraryIndex,
		HasLibraryIndex: opts.HasLibraryIndex,
		LibraryIssues:   slices.Clone(opts.LibraryIssues),
	})
	status := EvidencePass
	if !writer.OK || reports.HasBlockingIssue(writer.Issues) {
		status = EvidenceFail
	} else if writer.OverallSummary.WarningCount > 0 || writer.OverallSummary.SkippedCount > 0 {
		status = EvidenceWarning
	}
	return writerEvidenceResult{Status: status, Issues: writer.Issues}
}

func evaluateBoardEvidence(ctx context.Context, root string, opts EvaluateOptions) boardEvidenceResult {
	policy := effectiveCLIPolicy(opts)
	board := boardvalidation.Validate(ctx, root, boardvalidation.Options{
		KiCadCLI:        validationKiCadCLI(opts),
		RequireDRC:      policy == CLIPolicyRequired,
		AllowMissingDRC: policy != CLIPolicyRequired,
	})
	status := EvidencePass
	switch board.Status {
	case boardvalidation.StatusFail, boardvalidation.StatusError:
		status = EvidenceFail
	case boardvalidation.StatusSkipped:
		status = EvidenceMissing
	}

	drc := EvidenceMissing
	for _, check := range board.Checks {
		if check.Name != boardvalidation.CheckKiCadDRC {
			continue
		}
		switch check.Status {
		case boardvalidation.StatusPass:
			if drc != EvidenceFail {
				drc = EvidencePass
			}
		case boardvalidation.StatusFail, boardvalidation.StatusError:
			drc = EvidenceFail
		case boardvalidation.StatusSkipped:
			if drc != EvidenceFail && drc != EvidencePass {
				drc = EvidenceMissing
			}
		}
	}
	return boardEvidenceResult{Status: status, DRC: drc, Issues: board.Issues}
}

func validationKiCadCLI(opts EvaluateOptions) string {
	if opts.DryRun || effectiveCLIPolicy(opts) == CLIPolicyDisabled {
		return ""
	}
	return opts.KiCadCLI
}

func addERCEvidence(summary inspect.ProjectSummary, opts EvaluateOptions, evidence map[string]EvidenceStatus, resultSummary *Summary, issues *[]reports.Issue) {
	if existing, ok := evidence["erc"]; ok {
		resultSummary.ERC = existing
		if existing != EvidenceMissing || issuePathExists(*issues, "erc") {
			return
		}
	}
	status := EvidenceMissing
	message := "KiCad ERC evidence has not been generated"
	if summary.Schematic != nil {
		message = "schematic inspection is available, but KiCad ERC evidence has not been generated"
	}
	severity := cliMissingSeverity(opts)
	evidence["erc"] = status
	resultSummary.ERC = status
	*issues = append(*issues, reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   severity,
		Path:       "erc",
		Message:    message,
		Suggestion: "export an ERC report through KiCad CLI or GUI and include it in the fabrication package evidence",
	})
}

func issuePathExists(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}

func addMissingExternalEvidence(opts EvaluateOptions, evidence map[string]EvidenceStatus, resultSummary *Summary, issues *[]reports.Issue) {
	severity := cliMissingSeverity(opts)
	missing := []struct {
		key         string
		path        string
		description string
		setSummary  func(EvidenceStatus)
	}{
		{"bom", "bom", "fabrication BOM has not been generated", func(status EvidenceStatus) { resultSummary.BOM = status }},
		{"cpl", "cpl", "component placement list has not been generated", func(status EvidenceStatus) { resultSummary.CPL = status }},
		{"gerber", "gerber", "Gerber fabrication plots have not been generated", func(status EvidenceStatus) { resultSummary.Gerber = status }},
		{"drill", "drill", "drill files have not been generated", func(status EvidenceStatus) { resultSummary.Drill = status }},
		{"component_readiness", "component_readiness", "component procurement readiness has not been proven", func(status EvidenceStatus) { resultSummary.ComponentReadiness = status }},
		{"block_readiness", "block_readiness", "circuit block readiness has not been proven", func(status EvidenceStatus) { resultSummary.BlockReadiness = status }},
	}
	for _, item := range missing {
		if _, ok := evidence[item.key]; ok {
			continue
		}
		evidence[item.key] = EvidenceMissing
		item.setSummary(EvidenceMissing)
		*issues = append(*issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: severityForMissingEvidence(item.key, severity),
			Path:     item.path,
			Message:  item.description,
		})
	}
}

func cliPolicy(policy CLIPolicy) CLIPolicy {
	switch policy {
	case "", CLIPolicyDisabled, CLIPolicyOptional, CLIPolicyRequired:
		return policy
	default:
		return CLIPolicyDisabled
	}
}

func effectiveCLIPolicy(opts EvaluateOptions) CLIPolicy {
	policy := cliPolicy(opts.CLIPolicy)
	if policy != "" {
		return policy
	}
	if strings.TrimSpace(opts.KiCadCLI) != "" {
		return CLIPolicyOptional
	}
	return CLIPolicyDisabled
}

func cliMissingSeverity(opts EvaluateOptions) reports.Severity {
	if effectiveCLIPolicy(opts) == CLIPolicyRequired {
		return reports.SeverityError
	}
	return reports.SeverityWarning
}

func severityForMissingEvidence(key string, cliSeverity reports.Severity) reports.Severity {
	switch key {
	case "bom", "cpl", "gerber", "drill":
		return cliSeverity
	default:
		return reports.SeverityWarning
	}
}

func missingCoreFileIssue(kind string, path string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     filepath.ToSlash(path),
		Message:  kind + " file is required for fabrication readiness",
	}
}

func statusForIssues(issues []reports.Issue) EvidenceStatus {
	if len(issues) == 0 {
		return EvidencePass
	}
	if reports.HasBlockingIssue(issues) {
		return EvidenceFail
	}
	return EvidenceWarning
}

func dedupeIssues(issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		key := dedupeKey(
			string(issue.Code),
			string(issue.Severity),
			issue.Path,
			issue.Message,
			sortedSliceKey(issue.Refs),
			sortedSliceKey(issue.Nets),
			issue.Suggestion,
			issue.OperationID,
		)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, issue)
	}
	return out
}

func dedupeKey(parts ...string) string {
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(strconv.Itoa(len(part)))
		builder.WriteByte(':')
		builder.WriteString(part)
	}
	return builder.String()
}

func sliceKey(values []string) string {
	return dedupeKey(values...)
}

func sortedSliceKey(values []string) string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sliceKey(sorted)
}

func expectedFabricationArtifacts() []Artifact {
	return []Artifact{
		{Kind: ArtifactReadinessReport, Path: "fabrication/readiness.json", Status: ArtifactExpected, Required: true, Description: "fabrication readiness report"},
		{Kind: ArtifactManifest, Path: "fabrication/package-manifest.json", Status: ArtifactExpected, Required: true, Description: "fabrication package manifest"},
		{Kind: ArtifactPhysicalRules, Path: "fabrication/physical-rules.json", Status: ArtifactExpected, Required: true, Description: "physical fabrication rule report"},
		{Kind: ArtifactBOM, Path: "fabrication/bom.csv", Status: ArtifactExpected, Required: true, Description: "bill of materials"},
		{Kind: ArtifactCPL, Path: "fabrication/cpl.csv", Status: ArtifactExpected, Required: true, Description: "component placement list"},
		{Kind: ArtifactERC, Path: "fabrication/erc.json", Status: ArtifactExpected, Required: true, Description: "KiCad electrical rules report"},
		{Kind: ArtifactDRC, Path: "fabrication/drc.json", Status: ArtifactExpected, Required: true, Description: "KiCad design rules report"},
		{Kind: ArtifactBlockReadiness, Path: "fabrication/block-readiness.json", Status: ArtifactExpected, Required: true, Description: "verified block-composition promotion report"},
		{Kind: ArtifactGerber, Path: "fabrication/gerbers", Status: ArtifactExpected, Required: true, Description: "Gerber plot directory"},
		{Kind: ArtifactDrill, Path: "fabrication/drill", Status: ArtifactExpected, Required: true, Description: "Excellon drill directory"},
	}
}
