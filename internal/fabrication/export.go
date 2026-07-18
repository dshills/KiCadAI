package fabrication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/fabrication/physicalrules"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func ExportPreview(ctx context.Context, targetPath string, opts Options) Result {
	result := Evaluate(ctx, targetPath, evaluateOptionsForExport(opts))
	return exportReadiness(ctx, targetPath, opts, result, nil, nil, false)
}

func MarshalResultJSON(result Result) ([]byte, error) {
	normalized := result
	normalized.Issues = dedupeIssues(slices.Clone(result.Issues))
	normalized.Artifacts = slices.Clone(result.Artifacts)
	slices.SortFunc(normalized.Issues, compareIssues)
	slices.SortFunc(normalized.Artifacts, compareArtifacts)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func ExportBOM(ctx context.Context, targetPath string, opts Options) Result {
	result := Evaluate(ctx, targetPath, evaluateOptionsForExport(opts))
	reportData, err := BuildReports(ctx, targetPath)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "bom", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, false)
	}
	reportData = ApplyProcurementSnapshots(ctx, reportData, opts)
	result.Issues = append(result.Issues, reportData.Issues...)
	applyReportEvidence(&result, reportData, opts)
	bomCSV, err := MarshalBOMCSV(reportData.BOM)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "bom.csv", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, false)
	}
	return exportReadiness(ctx, targetPath, opts, result, bomCSV, nil, false)
}

func ExportPackage(ctx context.Context, targetPath string, opts Options) Result {
	result := Evaluate(ctx, targetPath, evaluateOptionsForExport(opts))
	reportData, err := BuildReports(ctx, targetPath)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "package", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, true)
	}
	reportData = ApplyProcurementSnapshots(ctx, reportData, opts)
	result.Issues = append(result.Issues, reportData.Issues...)
	applyReportEvidence(&result, reportData, opts)
	bomCSV, err := MarshalBOMCSV(reportData.BOM)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "bom.csv", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, true)
	}
	cplCSV, err := MarshalCPLCSV(reportData.CPL)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "cpl.csv", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, bomCSV, nil, true)
	}
	return exportReadiness(ctx, targetPath, opts, result, bomCSV, cplCSV, true)
}

func evaluateOptionsForExport(opts Options) EvaluateOptions {
	return EvaluateOptions{
		KiCadCLI: opts.KiCadCLI, DryRun: !opts.Execute, CLIPolicy: opts.CLIPolicy,
		ManufacturerProfile: opts.ManufacturerProfile, ManufacturerProfileDir: opts.ManufacturerProfileDir,
		LibraryIndex: opts.LibraryIndex, HasLibraryIndex: opts.HasLibraryIndex, LibraryIssues: slices.Clone(opts.LibraryIssues),
	}
}

func exportReadiness(ctx context.Context, targetPath string, opts Options, result Result, bomCSV []byte, cplCSV []byte, includeFabricationOutputs bool) Result {
	target, err := resolveEvaluationTarget(targetPath)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "target", Message: err.Error()})
		result.Status = CalculateStatus(result.Issues, summaryEvidence(result.Summary))
		return result
	}
	outputDir, issue := resolveOutputDir(target.Root, opts.Output)
	if issue != nil {
		result.Issues = append(result.Issues, *issue)
		result.Status = CalculateStatus(result.Issues, summaryEvidence(result.Summary))
		return result
	}
	result.DryRun = !opts.Execute
	metadataWrites := []exportWrite{
		{Rel: "readiness.json", Kind: ArtifactReadinessReport},
		{Rel: "package-manifest.json", Kind: ArtifactManifest},
	}
	dataWrites := []exportWrite{}
	if result.PhysicalRules != nil {
		if reportJSON, err := physicalrules.MarshalReportJSON(*result.PhysicalRules); err == nil {
			dataWrites = append(dataWrites, exportWrite{Rel: "physical-rules.json", Kind: ArtifactPhysicalRules, Data: reportJSON})
		} else {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "physical-rules.json", Message: err.Error()})
			result = finalizeExportResult(result)
		}
	}
	if bomCSV != nil {
		dataWrites = append(dataWrites, exportWrite{Rel: "bom.csv", Kind: ArtifactBOM, Data: bomCSV})
	}
	if cplCSV != nil {
		dataWrites = append(dataWrites, exportWrite{Rel: "cpl.csv", Kind: ArtifactCPL, Data: cplCSV})
	}
	if includeFabricationOutputs {
		dataWrites = appendFabricationCheckEvidence(ctx, target.Root, opts, &result, dataWrites)
		dataWrites = appendBlockReadinessEvidence(target.Root, &result, dataWrites, opts.BlockReadinessReport)
	}
	for _, write := range append(slices.Clone(metadataWrites), dataWrites...) {
		relPath, err := exportRelPath(target.Root, outputDir, write.Rel)
		if err != nil {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: write.Rel, Message: err.Error()})
			continue
		}
		result.Artifacts = markArtifact(result.Artifacts, write.Kind, filepath.ToSlash(relPath), ArtifactExpected)
	}
	for _, write := range dataWrites {
		if !opts.Execute {
			continue
		}
		if !writeArtifact(ctx, &result, target.Root, outputDir, write, opts.Overwrite) {
			break
		}
		if (write.Kind == ArtifactERC || write.Kind == ArtifactDRC || write.Kind == ArtifactBlockReadiness) && artifactGenerated(result.Artifacts, write.Kind) {
			relPath, _ := exportRelPath(target.Root, outputDir, write.Rel)
			generator := GeneratorKiCad
			if write.Kind == ArtifactBlockReadiness {
				generator = GeneratorKiCadAI
			}
			result.Artifacts = setArtifactEvidence(result.Artifacts, write.Kind, generator, []string{filepath.ToSlash(relPath)})
		}
	}
	if opts.Execute {
		if artifactGenerated(result.Artifacts, ArtifactBOM) {
			result.Summary.BOM = EvidencePass
			result.Issues = removeExactIssuePath(result.Issues, "bom")
		}
		if artifactGenerated(result.Artifacts, ArtifactCPL) {
			result.Summary.CPL = EvidencePass
			result.Issues = removeExactIssuePath(result.Issues, "cpl")
		}
	}
	if includeFabricationOutputs {
		gerberRel, gerberRelErr := exportRelPath(target.Root, outputDir, "gerbers")
		drillRel, drillRelErr := exportRelPath(target.Root, outputDir, "drill")
		pcbPath, pcbIssue := discoverPlotPCBPath(target.Root, target.Name)
		if pcbIssue != nil {
			result.Issues = append(result.Issues, *pcbIssue)
		}
		if gerberRelErr != nil {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "gerbers", Message: gerberRelErr.Error()})
		}
		if drillRelErr != nil {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "drill", Message: drillRelErr.Error()})
		}
		if gerberRelErr == nil && drillRelErr == nil && pcbIssue == nil {
			result.Artifacts = markArtifact(result.Artifacts, ArtifactGerber, filepath.ToSlash(gerberRel), ArtifactExpected)
			result.Artifacts = markArtifact(result.Artifacts, ArtifactDrill, filepath.ToSlash(drillRel), ArtifactExpected)
			plotRequest := PlotRequest{
				ProjectRoot: target.Root,
				ProjectName: target.Name,
				PCBPath:     pcbPath,
				PackageDir:  outputDir,
				GerberDir:   filepath.Join(outputDir, "gerbers"),
				DrillDir:    filepath.Join(outputDir, "drill"),
				Execute:     opts.Execute,
				Overwrite:   opts.Overwrite,
				KiCadCLI:    opts.KiCadCLI,
				CLIPolicy:   opts.CLIPolicy,
			}
			plot := PlotFabricationOutputs(ctx, plotRequest, opts.PlotRunner)
			result.Issues = append(result.Issues, plot.Issues...)
			result.Artifacts = applyPlotArtifacts(result.Artifacts, plot, filepath.ToSlash(gerberRel), filepath.ToSlash(drillRel))
			validation := ValidateFabricationArtifacts(ctx, plotRequest)
			result.Summary.Gerber = validation.Gerber
			result.Summary.Drill = validation.Drill
			if validation.Gerber == EvidencePass {
				result.Issues = removeExactIssuePath(result.Issues, "gerber")
			}
			if validation.Drill == EvidencePass {
				result.Issues = removeExactIssuePath(result.Issues, "drill")
			}
			result.Issues = append(result.Issues, validation.Issues...)
			result.Issues = dedupeIssues(result.Issues)
			result.Artifacts = applyArtifactValidation(result.Artifacts, validation, filepath.ToSlash(gerberRel), filepath.ToSlash(drillRel))
		}
	}
	result = finalizeExportResult(result)
	manifest := Manifest{
		Project:             ProjectRef{Name: target.Name, Root: filepath.ToSlash(target.Root)},
		Status:              result.Status,
		Score:               result.Score,
		Generated:           result.Summary.Generated,
		ManufacturerProfile: result.ManufacturerProfile,
		Artifacts:           slices.Clone(result.Artifacts),
		Evidence:            summaryEvidence(result.Summary),
		Issues:              slices.Clone(result.Issues),
		Options:             opts,
	}
	readinessJSON, err := MarshalResultJSON(result)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "readiness.json", Message: err.Error()})
		return finalizeExportResult(result)
	}
	manifestJSON, err := MarshalManifest(manifest)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "package-manifest.json", Message: err.Error()})
		return finalizeExportResult(result)
	}
	metadataWrites[0].Data = readinessJSON
	metadataWrites[1].Data = manifestJSON
	for _, write := range metadataWrites {
		if !opts.Execute {
			continue
		}
		if !writeArtifact(ctx, &result, target.Root, outputDir, write, opts.Overwrite) {
			break
		}
	}
	if opts.Execute {
		result.ManifestPath = filepath.ToSlash(filepath.Join(outputDir, "package-manifest.json"))
	}
	return finalizeExportResult(result)
}

func artifactGenerated(artifacts []Artifact, kind ArtifactKind) bool {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact.Status == ArtifactGenerated
		}
	}
	return false
}

func removeExactIssuePath(issues []reports.Issue, path string) []reports.Issue {
	out := issues[:0]
	for _, issue := range issues {
		if issue.Path != path {
			out = append(out, issue)
		}
	}
	return out
}

func appendFabricationCheckEvidence(ctx context.Context, targetRoot string, opts Options, result *Result, writes []exportWrite) []exportWrite {
	if !opts.Execute || strings.TrimSpace(opts.KiCadCLI) == "" || cliPolicy(opts.CLIPolicy) == CLIPolicyDisabled {
		return writes
	}
	artifactRoot, err := os.MkdirTemp("", "kicadai-fabrication-checks-")
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "fabrication/checks", Message: err.Error()})
		return writes
	}
	defer os.RemoveAll(artifactRoot)
	checkOptions := checks.Options{KeepArtifacts: true, ArtifactDir: artifactRoot}
	cli := checks.KiCadCLI{Path: opts.KiCadCLI}
	for _, item := range []struct {
		kind         checks.CheckKind
		artifactKind ArtifactKind
		rel          string
		run          func() (checks.CheckResult, error)
		setSummary   func(EvidenceStatus)
	}{
		{kind: checks.CheckKindERC, artifactKind: ArtifactERC, rel: "erc.json", run: func() (checks.CheckResult, error) {
			if opts.CheckRunner != nil {
				return checks.RunERCWithRunner(ctx, opts.CheckRunner, cli, targetRoot, checkOptions)
			}
			return checks.RunERC(ctx, cli, targetRoot, checkOptions)
		}, setSummary: func(status EvidenceStatus) { result.Summary.ERC = status }},
		{kind: checks.CheckKindDRC, artifactKind: ArtifactDRC, rel: "drc.json", run: func() (checks.CheckResult, error) {
			if opts.CheckRunner != nil {
				return checks.RunDRCWithRunner(ctx, opts.CheckRunner, cli, targetRoot, checkOptions)
			}
			return checks.RunDRC(ctx, cli, targetRoot, checkOptions)
		}, setSummary: func(status EvidenceStatus) { result.Summary.DRC = status }},
	} {
		check, runErr := item.run()
		path := string(item.kind)
		result.Issues = removeExactIssuePath(result.Issues, path)
		switch check.Status {
		case checks.CheckStatusPass:
			item.setSummary(EvidencePass)
		case checks.CheckStatusFail:
			item.setSummary(EvidenceFail)
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: path + " report contains rule violations"})
		case checks.CheckStatusSkipped:
			item.setSummary(EvidenceSkipped)
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityWarning, Path: path, Message: path + " check was skipped"})
		default:
			item.setSummary(EvidenceFail)
			message := path + " check failed"
			if runErr != nil {
				message += ": " + runErr.Error()
			}
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message})
		}
		if check.ReportPath == "" {
			continue
		}
		data, readErr := os.ReadFile(filepath.FromSlash(check.ReportPath))
		if readErr != nil {
			result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: item.rel, Message: readErr.Error()})
			continue
		}
		writes = append(writes, exportWrite{Rel: item.rel, Kind: item.artifactKind, Data: data})
	}
	return writes
}

type blockReadinessReport struct {
	Status             string `json:"status"`
	AchievedReadiness  string `json:"achieved_readiness"`
	MatchesExpectation bool   `json:"matches_expectation"`
	Gates              []struct {
		Status string `json:"status"`
	} `json:"gates"`
}

func appendBlockReadinessEvidence(targetRoot string, result *Result, writes []exportWrite, provided []byte) []exportWrite {
	data := slices.Clone(provided)
	var err error
	if len(data) == 0 {
		path := filepath.Join(targetRoot, ".kicadai", "design-promotion.json")
		data, err = os.ReadFile(path)
		if os.IsNotExist(err) {
			return writes
		}
	}
	result.Issues = removeExactIssuePath(result.Issues, "block_readiness")
	if err != nil {
		result.Summary.BlockReadiness = EvidenceFail
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "block_readiness", Message: err.Error()})
		return writes
	}
	var report blockReadinessReport
	if err := json.Unmarshal(data, &report); err != nil {
		result.Summary.BlockReadiness = EvidenceFail
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "block_readiness", Message: "invalid design promotion report: " + err.Error()})
		return writes
	}
	status := EvidencePass
	if report.Status != "pass" || report.AchievedReadiness != "pass" || !report.MatchesExpectation || len(report.Gates) == 0 {
		status = EvidenceFail
	} else {
		for _, gate := range report.Gates {
			if gate.Status != "pass" {
				status = EvidenceFail
				break
			}
		}
	}
	result.Summary.BlockReadiness = status
	if status == EvidenceFail {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "block_readiness", Message: "design promotion report does not prove pass readiness"})
	}
	return append(writes, exportWrite{Rel: "block-readiness.json", Kind: ArtifactBlockReadiness, Data: data})
}

type exportWrite struct {
	Rel  string
	Kind ArtifactKind
	Data []byte
}

func exportRelPath(root string, outputDir string, filename string) (string, error) {
	return filepath.Rel(root, filepath.Join(outputDir, filename))
}

func discoverPlotPCBPath(root string, projectName string) (string, *reports.Issue) {
	candidate := filepath.Join(root, projectName+".kicad_pcb")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	matches, err := filepath.Glob(filepath.Join(root, "*.kicad_pcb"))
	if err != nil {
		return "", &reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "fabrication/pcb",
			Message:  err.Error(),
		}
	}
	if len(matches) == 0 {
		return "", &reports.Issue{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "fabrication/pcb",
			Message:  "PCB file is required to generate fabrication outputs",
		}
	}
	slices.Sort(matches)
	if len(matches) > 1 {
		return "", &reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "fabrication/pcb",
			Message:  "multiple PCB files found and no project-named PCB file exists",
		}
	}
	return matches[0], nil
}

func writeArtifact(ctx context.Context, result *Result, root string, outputDir string, write exportWrite, overwrite bool) bool {
	if err := ctx.Err(); err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "export.context", Message: err.Error()})
		return false
	}
	path := filepath.Join(outputDir, write.Rel)
	relPath, err := exportRelPath(root, outputDir, write.Rel)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: write.Rel, Message: err.Error()})
		return true
	}
	if err := writeExportFile(path, write.Data, overwrite); err != nil {
		result.Artifacts = markArtifact(result.Artifacts, write.Kind, filepath.ToSlash(relPath), ArtifactBlocked)
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: filepath.ToSlash(relPath), Message: err.Error()})
		return true
	}
	result.Artifacts = markArtifact(result.Artifacts, write.Kind, filepath.ToSlash(relPath), ArtifactGenerated)
	return true
}

func resolveOutputDir(root string, output string) (string, *reports.Issue) {
	out := strings.TrimSpace(output)
	if out == "" {
		out = filepath.Join(root, "fabrication")
	} else if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: err.Error()}
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: err.Error()}
	}
	rel, err := filepath.Rel(absRoot, absOut)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "output path must be inside project root"}
	}
	return absOut, nil
}

func writeExportFile(path string, data []byte, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if overwrite {
		return os.WriteFile(path, data, 0o644)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func markArtifact(artifacts []Artifact, kind ArtifactKind, path string, status ArtifactStatus) []Artifact {
	for index := range artifacts {
		if artifacts[index].Kind == kind {
			artifacts[index].Path = path
			artifacts[index].Status = status
			return artifacts
		}
	}
	artifacts = append(artifacts, Artifact{Kind: kind, Path: path, Status: status, Required: true})
	slices.SortFunc(artifacts, compareArtifacts)
	return artifacts
}

func applyPlotArtifacts(artifacts []Artifact, plot PlotResult, gerberPath string, drillPath string) []Artifact {
	gerberStatus := ArtifactExpected
	drillStatus := ArtifactExpected
	if len(plot.Issues) > 0 {
		gerberStatus = ArtifactBlocked
		drillStatus = ArtifactBlocked
	}
	for _, command := range plot.Commands {
		status := ArtifactExpected
		generator := Generator("")
		if command.SkippedReason != "" {
			if len(plot.Issues) > 0 {
				status = ArtifactBlocked
			} else {
				status = ArtifactSkipped
			}
		} else if command.ExitCode != 0 {
			generator = GeneratorKiCad
			status = ArtifactBlocked
		} else if len(command.GeneratedPaths) > 0 {
			generator = GeneratorKiCad
			status = ArtifactGenerated
		} else {
			generator = GeneratorKiCad
		}
		switch command.Kind {
		case PlotKindGerber:
			gerberStatus = combineArtifactStatus(gerberStatus, status)
			artifacts = setArtifactEvidence(artifacts, ArtifactGerber, generator, command.GeneratedPaths)
		case PlotKindDrill:
			drillStatus = combineArtifactStatus(drillStatus, status)
			artifacts = setArtifactEvidence(artifacts, ArtifactDrill, generator, command.GeneratedPaths)
		}
	}
	if len(plot.Issues) > 0 {
		artifacts = setArtifactIssues(artifacts, ArtifactGerber, plot.Issues)
		artifacts = setArtifactIssues(artifacts, ArtifactDrill, plot.Issues)
	}
	artifacts = finalizeArtifactFiles(artifacts, ArtifactGerber)
	artifacts = finalizeArtifactFiles(artifacts, ArtifactDrill)
	artifacts = markArtifact(artifacts, ArtifactGerber, gerberPath, gerberStatus)
	artifacts = markArtifact(artifacts, ArtifactDrill, drillPath, drillStatus)
	return artifacts
}

func setArtifactEvidence(artifacts []Artifact, kind ArtifactKind, generator Generator, files []string) []Artifact {
	for index := range artifacts {
		if artifacts[index].Kind != kind {
			continue
		}
		if generator != "" {
			artifacts[index].Generator = generator
		}
		if len(files) > 0 {
			artifacts[index].Files = append(artifacts[index].Files, files...)
		}
	}
	return artifacts
}

func finalizeArtifactFiles(artifacts []Artifact, kind ArtifactKind) []Artifact {
	for index := range artifacts {
		if artifacts[index].Kind != kind {
			continue
		}
		artifacts[index].Files = dedupeStrings(artifacts[index].Files)
		slices.Sort(artifacts[index].Files)
	}
	return artifacts
}

func setArtifactIssues(artifacts []Artifact, kind ArtifactKind, issues []reports.Issue) []Artifact {
	for index := range artifacts {
		if artifacts[index].Kind != kind {
			continue
		}
		artifacts[index].Issues = dedupeIssues(append(artifacts[index].Issues, issues...))
		slices.SortFunc(artifacts[index].Issues, compareIssues)
	}
	return artifacts
}

func combineArtifactStatus(current ArtifactStatus, next ArtifactStatus) ArtifactStatus {
	if current == ArtifactExpected {
		return next
	}
	if next == ArtifactExpected {
		return current
	}
	if artifactStatusRank(next) < artifactStatusRank(current) {
		return next
	}
	return current
}

func artifactStatusRank(status ArtifactStatus) int {
	switch status {
	case ArtifactBlocked:
		return 0
	case ArtifactMissing:
		return 1
	case ArtifactGenerated:
		return 2
	case ArtifactSkipped:
		return 3
	case ArtifactExpected:
		return 4
	default:
		return 0
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func applyArtifactValidation(artifacts []Artifact, validation FabricationArtifactValidation, gerberPath string, drillPath string) []Artifact {
	artifacts = markArtifact(artifacts, ArtifactGerber, gerberPath, artifactStatusForEvidence(validation.Gerber))
	artifacts = markArtifact(artifacts, ArtifactDrill, drillPath, artifactStatusForEvidence(validation.Drill))
	return artifacts
}

func artifactStatusForEvidence(status EvidenceStatus) ArtifactStatus {
	switch status {
	case EvidencePass:
		return ArtifactGenerated
	case EvidenceMissing:
		return ArtifactMissing
	case EvidenceSkipped:
		return ArtifactSkipped
	case EvidenceWarning:
		return ArtifactExpected
	case EvidenceFail:
		return ArtifactBlocked
	default:
		return ArtifactBlocked
	}
}

func summaryEvidence(summary Summary) map[string]EvidenceStatus {
	return map[string]EvidenceStatus{
		"project":              summary.Project,
		"schematic":            summary.Schematic,
		"pcb":                  summary.PCB,
		"writer_correctness":   summary.WriterCorrectness,
		"board_validation":     summary.BoardValidation,
		"erc":                  summary.ERC,
		"drc":                  summary.DRC,
		"bom":                  summary.BOM,
		"cpl":                  summary.CPL,
		"gerber":               summary.Gerber,
		"drill":                summary.Drill,
		"manifest":             summary.Manifest,
		"component_readiness":  summary.ComponentReadiness,
		"block_readiness":      summary.BlockReadiness,
		"component_identity":   summary.ComponentIdentity,
		"bom_cpl_consistency":  summary.BOMCPLConsistency,
		"manufacturer_profile": summary.ManufacturerProfile,
		"assembly_readiness":   summary.AssemblyReadiness,
		"physical_rules":       summary.PhysicalRules,
	}
}

func finalizeExportResult(result Result) Result {
	result.Issues = dedupeIssues(result.Issues)
	slices.SortFunc(result.Issues, compareIssues)
	result.Status = CalculateStatus(result.Issues, summaryEvidence(result.Summary))
	result.Score = Score(summaryEvidence(result.Summary))
	return result
}
