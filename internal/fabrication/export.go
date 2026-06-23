package fabrication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

func ExportPreview(ctx context.Context, targetPath string, opts Options) Result {
	result := Evaluate(ctx, targetPath, EvaluateOptions{KiCadCLI: opts.KiCadCLI, DryRun: !opts.Execute, CLIPolicy: opts.CLIPolicy})
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
	result := Evaluate(ctx, targetPath, EvaluateOptions{KiCadCLI: opts.KiCadCLI, DryRun: !opts.Execute, CLIPolicy: opts.CLIPolicy})
	reportData, err := BuildReports(ctx, targetPath)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "bom", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, false)
	}
	result.Issues = append(result.Issues, reportData.Issues...)
	bomCSV, err := MarshalBOMCSV(reportData.BOM)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "bom.csv", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, false)
	}
	return exportReadiness(ctx, targetPath, opts, result, bomCSV, nil, false)
}

func ExportPackage(ctx context.Context, targetPath string, opts Options) Result {
	result := Evaluate(ctx, targetPath, EvaluateOptions{KiCadCLI: opts.KiCadCLI, DryRun: !opts.Execute, CLIPolicy: opts.CLIPolicy})
	reportData, err := BuildReports(ctx, targetPath)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "package", Message: err.Error()})
		return exportReadiness(ctx, targetPath, opts, result, nil, nil, true)
	}
	result.Issues = append(result.Issues, reportData.Issues...)
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
	if bomCSV != nil {
		dataWrites = append(dataWrites, exportWrite{Rel: "bom.csv", Kind: ArtifactBOM, Data: bomCSV})
	}
	if cplCSV != nil {
		dataWrites = append(dataWrites, exportWrite{Rel: "cpl.csv", Kind: ArtifactCPL, Data: cplCSV})
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
			plot := PlotFabricationOutputs(ctx, PlotRequest{
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
			}, opts.PlotRunner)
			result.Issues = append(result.Issues, plot.Issues...)
			result.Artifacts = applyPlotArtifacts(result.Artifacts, plot, filepath.ToSlash(gerberRel), filepath.ToSlash(drillRel))
		}
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
	}
	result = finalizeExportResult(result)
	manifest := Manifest{
		Project:   ProjectRef{Name: target.Name, Root: filepath.ToSlash(target.Root)},
		Status:    result.Status,
		Score:     result.Score,
		Generated: result.Summary.Generated,
		Artifacts: slices.Clone(result.Artifacts),
		Evidence:  summaryEvidence(result.Summary),
		Issues:    slices.Clone(result.Issues),
		Options:   opts,
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
		if command.SkippedReason != "" {
			if len(plot.Issues) > 0 {
				status = ArtifactBlocked
			}
		} else if command.ExitCode != 0 {
			status = ArtifactBlocked
		} else if len(command.GeneratedPaths) > 0 {
			status = ArtifactGenerated
		}
		switch command.Kind {
		case PlotKindGerber:
			gerberStatus = status
		case PlotKindDrill:
			drillStatus = status
		}
	}
	artifacts = markArtifact(artifacts, ArtifactGerber, gerberPath, gerberStatus)
	artifacts = markArtifact(artifacts, ArtifactDrill, drillPath, drillStatus)
	return artifacts
}

func summaryEvidence(summary Summary) map[string]EvidenceStatus {
	return map[string]EvidenceStatus{
		"project":             summary.Project,
		"schematic":           summary.Schematic,
		"pcb":                 summary.PCB,
		"writer_correctness":  summary.WriterCorrectness,
		"board_validation":    summary.BoardValidation,
		"erc":                 summary.ERC,
		"drc":                 summary.DRC,
		"bom":                 summary.BOM,
		"cpl":                 summary.CPL,
		"gerber":              summary.Gerber,
		"drill":               summary.Drill,
		"manifest":            summary.Manifest,
		"component_readiness": summary.ComponentReadiness,
		"block_readiness":     summary.BlockReadiness,
	}
}

func finalizeExportResult(result Result) Result {
	result.Issues = dedupeIssues(result.Issues)
	slices.SortFunc(result.Issues, compareIssues)
	result.Status = CalculateStatus(result.Issues, summaryEvidence(result.Summary))
	result.Score = Score(summaryEvidence(result.Summary))
	return result
}
