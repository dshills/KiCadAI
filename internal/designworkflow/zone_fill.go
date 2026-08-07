package designworkflow

import (
	"context"
	"strings"

	"kicadai/internal/repair"
	"kicadai/internal/reports"
)

type generatedZoneFillResult struct {
	Required  bool
	Ran       bool
	Command   []string
	Artifacts []reports.Artifact
	Issues    []reports.Issue
}

func prepareGeneratedZoneFill(ctx context.Context, request Request, written *ProjectWriteResult, opts CreateOptions) generatedZoneFillResult {
	if request.ExplicitCircuit == nil || len(request.ExplicitCircuit.Zones) == 0 {
		return generatedZoneFillResult{}
	}
	result := generatedZoneFillResult{Required: request.Validation.StrictZones || request.Validation.RequireDRC}
	if !result.Required {
		return result
	}
	if written == nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked,
			Path: "zone_refill.project_write", Message: "project write result is required for zone refill",
		})
		return result
	}
	root := projectRootFromWrite(written)
	cli := strings.TrimSpace(opts.KiCadChecks.KiCadCLI)
	if cli == "" {
		cli = strings.TrimSpace(opts.Validation.KiCadCLI)
	}
	zoneResult := repair.RunZoneRefill(ctx, repair.Target{
		Path: root, Root: root, Kind: repair.TargetProjectDir,
		Generated: true, Mutable: true, Transaction: &written.Transaction,
	}, root, repair.ZoneRefillOptions{
		Policy:        repair.ZoneRefillBeforeValidation,
		KiCadCLI:      cli,
		KeepArtifacts: opts.KiCadChecks.KeepArtifacts || opts.Validation.KeepArtifacts,
		ArtifactDir:   firstNonEmptyString(opts.KiCadChecks.ArtifactDir, opts.Validation.ArtifactDir),
	}, opts.ZoneRefill)
	result.Ran = zoneResult.Ran
	result.Command = append([]string(nil), zoneResult.Command...)
	result.Artifacts = append(result.Artifacts, zoneResult.Artifacts...)
	result.Issues = append(result.Issues, zoneResult.Issues...)
	if reports.HasBlockingIssue(result.Issues) || !result.Ran {
		return result
	}
	manifestArtifact, err := repair.RefreshGeneratedManifest(root)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
			Path: "zone_refill.manifest", Message: err.Error(),
		})
		return result
	}
	result.Artifacts = append(result.Artifacts, manifestArtifact)
	return result
}

func generatedZoneFillWriterStage(result generatedZoneFillResult) StageResult {
	stage := NewStageResult(StageWriterCorrect, result.Issues)
	stage.Summary = generatedZoneFillSummary(result)
	stage.Artifacts = append([]reports.Artifact(nil), result.Artifacts...)
	return stage
}

func mergeGeneratedZoneFillEvidence(stage *StageResult, result generatedZoneFillResult) {
	if stage == nil || !result.Required {
		return
	}
	if stage.Summary == nil {
		stage.Summary = map[string]any{}
	}
	stage.Summary["zone_fill"] = generatedZoneFillSummary(result)
	stage.Issues = append(stage.Issues, result.Issues...)
	stage.Artifacts = append(stage.Artifacts, result.Artifacts...)
	stage.Status = StageStatusForIssues(stage.Issues)
}

func generatedZoneFillSummary(result generatedZoneFillResult) map[string]any {
	return map[string]any{
		"required": result.Required,
		"ran":      result.Ran,
		"command":  append([]string(nil), result.Command...),
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
