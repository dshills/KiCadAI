package designworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/fabrication"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/repair"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
	"kicadai/internal/writercorrectness"
)

type CreateOptions struct {
	OutputDir     string
	Overwrite     bool
	Seed          string
	SkipRouting   bool
	Components    ComponentSelectionOptions
	Placement     PlacementOptions
	Routing       RoutingOptions
	Validation    ValidationOptions
	KiCadChecks   KiCadCheckOptions
	Writer        writercorrectness.Options
	Fabrication   *fabrication.Options
	Repair        repair.Options
	PostRepair    repair.PostValidationOptions
	BlockRegistry blocks.Registry
	LibraryIndex  *libraryresolver.LibraryIndex
}

func Create(ctx context.Context, request Request, opts CreateOptions) WorkflowResult {
	if opts.BlockRegistry == nil {
		opts.BlockRegistry = blocks.NewBuiltinRegistry()
	}
	// A writer correctness index is also the authoritative source needed while
	// materializing resolver-backed symbols. Keep project emission and readback
	// on the same library snapshot when callers provide it through Writer.
	if opts.LibraryIndex == nil && opts.Writer.HasLibraryIndex {
		opts.LibraryIndex = &opts.Writer.LibraryIndex
	}
	normalized := NormalizeRequest(request)
	if normalized.ExplicitCircuit != nil {
		return createExplicitCircuit(ctx, normalized, opts)
	}
	plan := PlanBlocks(ctx, opts.BlockRegistry, normalized)
	stages := []StageResult{plan.Stage}
	if workflowStageBlocked(plan.Stage) {
		return blockedCreateResult(normalized, opts, stages, StageBlockPlanning, "block planning did not complete")
	}
	componentSelections := SelectWorkflowComponents(ctx, opts.BlockRegistry, plan, opts.Components)
	if !workflowStageBlocked(componentSelections.Stage) {
		selectionApplyIssues := ApplyComponentSelectionsToPlan(&plan, opts.BlockRegistry, componentSelections.Selections)
		if len(selectionApplyIssues) != 0 {
			componentSelections.Stage.Issues = append(componentSelections.Stage.Issues, selectionApplyIssues...)
			componentSelections.Stage.Status = StageStatusForIssues(componentSelections.Stage.Issues)
		}
	}
	stages = append(stages, componentSelections.Stage)
	if workflowStageBlocked(componentSelections.Stage) {
		return blockedCreateResult(normalized, opts, stages, StageComponentSelection, "component selection did not complete")
	}
	schematicStage := schematicStageFromPlan(plan)
	stages = append(stages, schematicStage)
	if workflowStageBlocked(schematicStage) {
		return blockedCreateResult(normalized, opts, stages, StageSchematic, "schematic generation did not complete")
	}
	schematicElectricalStage := SchematicElectricalStage(plan)
	stages = append(stages, schematicElectricalStage)
	if workflowStageBlocked(schematicElectricalStage) {
		return blockedCreateResult(normalized, opts, stages, StageSchematicElectrical, "schematic electrical rules did not pass")
	}
	fragments := RealizePCBFragments(ctx, opts.BlockRegistry, plan)
	stages = append(stages, fragments.Stage)
	if workflowStageBlocked(fragments.Stage) {
		return blockedCreateResult(normalized, opts, stages, StagePCBRealization, "PCB realization did not complete")
	}
	placementOpts := placementOptionsForCreate(opts, componentSelections.Selections)
	placed := PlaceFragments(ctx, normalized, fragments, placementOpts)
	placementStageIndex := len(stages)
	stages = append(stages, placed.Stage)
	if workflowStageBlocked(placed.Stage) {
		return blockedCreateResult(normalized, opts, stages, StagePlacement, "placement did not complete")
	}
	routingOpts := opts.Routing
	routingOpts.ComponentSelections = componentSelections.Selections
	routingOpts.Skip = routingOpts.Skip || opts.SkipRouting || normalized.Validation.SkipRouting
	routed := RoutePlacement(ctx, normalized, fragments, placed, routingOpts)
	placed, routed, _ = maybeRetryPlacementRouting(ctx, normalized, fragments, placed, routed, routingOpts, normalized.RoutingRetry)
	stages[placementStageIndex] = placed.Stage
	stages = append(stages, routed.Stage)
	if workflowStageBlocked(routed.Stage) {
		return blockedCreateResult(normalized, opts, stages, StageRouting, "routing did not complete")
	}
	written := WriteProject(ctx, &normalized, &plan, &placed, &routed, ProjectWriteOptions{
		OutputDir:                 opts.OutputDir,
		Overwrite:                 opts.Overwrite,
		Seed:                      opts.Seed,
		LibraryIndex:              opts.LibraryIndex,
		PreserveFootprintGeometry: true,
	})
	stages = append(stages, written.Stage)
	if workflowStageBlocked(written.Stage) {
		return blockedCreateResult(normalized, opts, stages, StageProjectWrite, "project write did not complete")
	}
	writerChecked := CheckWriterCorrectnessWithOptions(ctx, &written, opts.Writer)
	stages = append(stages, writerChecked.Stage)
	if workflowStageBlocked(writerChecked.Stage) {
		stages = append(stages, skippedWorkflowStages("writer correctness check did not complete", StageValidation, StageKiCadChecks)...)
		groups := []repair.StageIssues{
			{Stage: string(StageWriterCorrect), Issues: writerChecked.Stage.Issues},
		}
		if repairStageShouldRun(opts.Repair, groups) {
			stages = append(stages, repairStageForGroups(ctx, &normalized, written, groups, opts))
		}
		return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
	}
	validated := ValidateProject(ctx, &normalized, &written, opts.Validation)
	stages = append(stages, validated.Stage)
	kicadCheckOpts := opts.KiCadChecks
	kicadCheckOpts.RequireERC = kicadCheckOpts.RequireERC || normalized.Validation.RequireERC
	kicadCheckOpts.RequireDRC = kicadCheckOpts.RequireDRC || normalized.Validation.RequireDRC
	checked := RunKiCadChecks(ctx, &normalized, &written, kicadCheckOpts)
	stages = append(stages, checked.Stage)
	fabricationOptions := opts.Fabrication
	var blockReadinessReportIssue *reports.Issue
	if fabricationOptions != nil && len(fabricationOptions.BlockReadinessReport) == 0 {
		configured := *fabricationOptions
		report, err := fabricationBlockReadinessReport(plan.Stage)
		if err != nil {
			issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "fabrication.block_readiness_report", Message: err.Error()}
			blockReadinessReportIssue = &issue
		} else {
			configured.BlockReadinessReport = report
		}
		fabricationOptions = &configured
	}
	fabricationStage := fabricationReadinessStageWithOptions(ctx, &normalized, &written, fabricationOptions)
	if blockReadinessReportIssue != nil {
		fabricationStage.Issues = append(fabricationStage.Issues, *blockReadinessReportIssue)
		status := NewStageResult(StageFabricationReady, fabricationStage.Issues)
		fabricationStage.Name = StageFabricationReady
		fabricationStage.Status = status.Status
	}
	if fabricationStage.Name != "" {
		stages = append(stages, fabricationStage)
	}
	if opts.Repair.Enabled {
		groups := []repair.StageIssues{
			{Stage: string(StageWriterCorrect), Issues: writerChecked.Stage.Issues},
			{Stage: string(StageValidation), Issues: validated.Stage.Issues},
			{Stage: string(StageKiCadChecks), Issues: checked.Stage.Issues},
		}
		if fabricationStage.Name != "" {
			groups = append(groups, repair.StageIssues{Stage: string(StageFabricationReady), Issues: fabricationStage.Issues})
		}
		if repairStageShouldRun(opts.Repair, groups) {
			stages = append(stages, repairStageForGroups(ctx, &normalized, written, groups, opts))
		}
	}
	return BuildWorkflowResult(ProjectSummary{Name: normalized.Name, OutputDir: opts.OutputDir}, normalized.Validation.Acceptance, stages)
}

func placementOptionsForCreate(opts CreateOptions, selections []ComponentSelectionEntry) PlacementOptions {
	placementOpts := opts.Placement
	if placementOpts.LibraryIndex == nil {
		placementOpts.LibraryIndex = opts.LibraryIndex
	}
	placementOpts.ComponentSelections = selections
	return placementOpts
}

func fabricationBlockReadinessReport(plan StageResult) ([]byte, error) {
	evidence, ok := plan.Summary["block_evidence"].([]BlockEvidenceSummary)
	if !ok || len(evidence) == 0 {
		return nil, nil
	}
	type gate struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	report := struct {
		Status             string `json:"status"`
		AchievedReadiness  string `json:"achieved_readiness"`
		MatchesExpectation bool   `json:"matches_expectation"`
		Gates              []gate `json:"gates"`
	}{Status: "pass", AchievedReadiness: "pass", MatchesExpectation: true}
	for _, item := range evidence {
		if item.Status != "verified" {
			return nil, nil
		}
		report.Gates = append(report.Gates, gate{ID: firstNonEmpty(item.InstanceID, item.BlockID), Status: "pass"})
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode fabrication block-readiness report: %w", err)
	}
	return append(data, '\n'), nil
}

func FabricationReadinessStage(ctx context.Context, request *Request, written *ProjectWriteResult) StageResult {
	return fabricationReadinessStageWithOptions(ctx, request, written, nil)
}

func fabricationReadinessStageWithOptions(ctx context.Context, request *Request, written *ProjectWriteResult, exportOptions *fabrication.Options) StageResult {
	if request == nil || written == nil || request.Validation.Acceptance != AcceptanceFabricationCandidate {
		return StageResult{}
	}
	outputDir := strings.TrimSpace(written.Inspection.Root)
	if outputDir == "" {
		return NewStageResult(StageFabricationReady, []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "fabrication.output_dir",
			Message:  "fabrication readiness requires a written project output directory",
		}})
	}
	policy := fabrication.CLIPolicyDisabled
	if request.Validation.RequireDRC || request.Validation.RequireERC {
		policy = fabrication.CLIPolicyRequired
	}
	options := fabrication.Options{CLIPolicy: policy}
	if exportOptions != nil {
		options = *exportOptions
		if options.CLIPolicy == "" {
			options.CLIPolicy = policy
		}
	}
	var result fabrication.Result
	if exportOptions == nil {
		result = fabrication.ExportPreview(ctx, outputDir, options)
	} else {
		// ExportPackage reruns the same Evaluate preflight as ExportPreview
		// before producing artifacts, so select one complete path rather than
		// overwriting or duplicating preflight results.
		result = fabrication.ExportPackage(ctx, outputDir, options)
	}
	issues := append([]reports.Issue(nil), result.Issues...)
	if result.Status != fabrication.StatusReady {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       "fabrication.status",
			Message:    "fabrication readiness is " + string(result.Status) + ", not ready",
			Suggestion: "run export fabrication with required ERC/DRC, BOM, CPL, Gerber, and drill evidence before claiming fabrication readiness",
		})
	}
	stage := NewStageResult(StageFabricationReady, issues)
	if result.Status == fabrication.StatusBlocked {
		stage.Status = StageStatusBlocked
	}
	stage.Artifacts = fabrication.ReportArtifacts(result.Artifacts)
	stage.Summary = map[string]any{
		"status":  result.Status,
		"score":   result.Score,
		"dry_run": result.DryRun,
	}
	if result.PhysicalRules != nil {
		physicalSummary := map[string]any{
			"status":        result.PhysicalRules.Status,
			"blocker_count": result.PhysicalRules.Summary.BlockedCount,
			"warning_count": result.PhysicalRules.Summary.WarningCount,
			"profile":       result.PhysicalRules.Profile,
		}
		if reportPath := physicalRulesArtifactPath(result.Artifacts); reportPath != "" {
			physicalSummary["report_path"] = reportPath
		}
		stage.Summary["physical_rules"] = physicalSummary
	}
	return stage
}

func physicalRulesArtifactPath(artifacts []fabrication.Artifact) string {
	for _, artifact := range artifacts {
		if artifact.Kind == fabrication.ArtifactPhysicalRules {
			return artifact.Path
		}
	}
	return ""
}

func repairStageShouldRun(opts repair.Options, groups []repair.StageIssues) bool {
	return opts.Enabled && (opts.Apply || repairStageIssueCount(groups) > 0)
}

func repairStageIssueCount(groups []repair.StageIssues) int {
	count := 0
	for _, group := range groups {
		count += len(group.Issues)
	}
	return count
}

func repairStageForGroups(ctx context.Context, request *Request, written ProjectWriteResult, groups []repair.StageIssues, opts CreateOptions) StageResult {
	if opts.Repair.Apply {
		return persistedValidationRepairStage(ctx, request, written, groups, opts)
	}
	return validationRepairStage(groups, opts.Repair)
}

func persistedValidationRepairStage(ctx context.Context, request *Request, written ProjectWriteResult, groups []repair.StageIssues, opts CreateOptions) StageResult {
	if err := ctx.Err(); err != nil {
		return NewStageResult(StageValidationRepair, []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  err.Error(),
		}})
	}
	tx := written.Transaction
	bundle := repair.Bundle{
		Schema:        repair.BundleSchemaV1,
		ProjectRoot:   opts.OutputDir,
		ProjectName:   request.Name,
		Generated:     true,
		Transaction:   &tx,
		StageIssues:   groups,
		RepairOptions: opts.Repair,
	}
	result := repair.ApplyPersistedBundleContext(ctx, opts.OutputDir, bundle, repair.PersistedApplyOptions{
		Execute:        true,
		OutputDir:      opts.OutputDir,
		Overwrite:      opts.Overwrite,
		Seed:           opts.Seed,
		Repair:         opts.Repair,
		Board:          &transactions.BoardSize{WidthMM: request.Board.WidthMM, HeightMM: request.Board.HeightMM},
		PostValidation: opts.PostRepair,
	})
	attemptCount, appliedCount := repairAttemptCounts(&result)
	stage := StageResult{Name: StageValidationRepair, Status: repairStageStatus(result.Status)}
	stage.Issues = append(stage.Issues, result.Issues...)
	stage.Artifacts = append(stage.Artifacts, result.Artifacts...)
	if artifact, err := writeRepairBundleArtifact(opts.OutputDir, &bundle); err == nil {
		stage.Artifacts = append(stage.Artifacts, artifact)
	} else {
		stage.Issues = append(stage.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "repair.bundle", Message: err.Error()})
	}
	stage.Summary = map[string]any{
		"status":           result.Status,
		"attempt_count":    attemptCount,
		"applied_count":    appliedCount,
		"validation_count": len(result.Validation),
		"artifact_count":   len(result.Artifacts),
		"validation_delta": result.Delta,
		"convergence":      result.Convergence,
	}
	if len(stage.Issues) > 0 {
		stage.Status = moreSevereStageStatus(stage.Status, StageStatusForIssues(stage.Issues))
	}
	return stage
}

func writeRepairBundleArtifact(outputDir string, bundle *repair.Bundle) (reports.Artifact, error) {
	if strings.TrimSpace(outputDir) == "" {
		return reports.Artifact{}, fmt.Errorf("output directory is required")
	}
	if bundle == nil {
		return reports.Artifact{}, fmt.Errorf("repair bundle is required")
	}
	dir := filepath.Join(outputDir, ".kicadai")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return reports.Artifact{}, fmt.Errorf("create repair bundle directory: %w", err)
	}
	path := filepath.Join(dir, "repair-bundle.json")
	file, err := os.Create(path)
	if err != nil {
		return reports.Artifact{}, fmt.Errorf("create repair bundle: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bundle); err != nil {
		_ = file.Close()
		return reports.Artifact{}, fmt.Errorf("write repair bundle: %w", err)
	}
	if err := file.Close(); err != nil {
		return reports.Artifact{}, fmt.Errorf("close repair bundle: %w", err)
	}
	return reports.Artifact{Kind: reports.ArtifactValidationReport, Path: filepath.ToSlash(path), Description: "post-repair validation bundle"}, nil
}

func repairAttemptCounts(result *repair.PersistedApplyResult) (int, int) {
	return result.Repair.Summary.AttemptCount, result.Repair.Summary.AppliedCount
}

func moreSevereStageStatus(a StageStatus, b StageStatus) StageStatus {
	if stageStatusRank(b) > stageStatusRank(a) {
		return b
	}
	return a
}

func stageStatusRank(status StageStatus) int {
	const (
		rankOK = iota
		rankSkipped
		rankWarning
		rankBlocked
	)
	switch status {
	case StageStatusBlocked:
		return rankBlocked
	case StageStatusWarning:
		return rankWarning
	case StageStatusSkipped:
		return rankSkipped
	case StageStatusOK:
		return rankOK
	default:
		return rankBlocked
	}
}

func validationRepairStage(groups []repair.StageIssues, opts repair.Options) StageResult {
	plan := repair.BuildPlan(groups, opts)
	stage := StageResult{Name: StageValidationRepair, Status: repairStageStatus(plan.Status)}
	stage.Summary = map[string]any{
		"status":        plan.Status,
		"attempt_count": plan.Summary.AttemptCount,
		"planned_count": plan.Summary.PlannedCount,
		"blocked_count": plan.Summary.BlockedCount,
		"skipped_count": plan.Summary.SkippedCount,
	}
	for _, attempt := range plan.Attempts {
		if attempt.Status == repair.StatusBlocked {
			stage.Issues = append(stage.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityBlocked,
				Path:     "validation_repair",
				Message:  attempt.Message,
				Refs:     append([]string(nil), attempt.Issue.Refs...),
				Nets:     append([]string(nil), attempt.Issue.Nets...),
			})
		}
	}
	if len(stage.Issues) > 0 {
		stage.Status = StageStatusForIssues(stage.Issues)
	}
	return stage
}

func repairStageStatus(status repair.Status) StageStatus {
	switch status {
	case repair.StatusNotNeeded, repair.StatusRepaired:
		return StageStatusOK
	case repair.StatusPartial, repair.StatusPlanned:
		return StageStatusWarning
	case repair.StatusSkipped:
		return StageStatusSkipped
	default:
		return StageStatusBlocked
	}
}

func workflowStageBlocked(stage StageResult) bool {
	return stage.Status == StageStatusBlocked || reports.HasBlockingIssue(stage.Issues)
}

func skippedWorkflowStages(reason string, names ...StageName) []StageResult {
	stages := make([]StageResult, 0, len(names))
	for _, name := range names {
		stages = append(stages, StageResult{Name: name, Status: StageStatusSkipped, Summary: map[string]any{"reason": reason}})
	}
	return stages
}

func schematicStageFromPlan(plan BlockPlanResult) StageResult {
	if plan.Stage.Status == StageStatusBlocked || reports.HasBlockingIssue(plan.Stage.Issues) {
		return StageResult{Name: StageSchematic, Status: StageStatusSkipped, Summary: map[string]any{"reason": "block planning did not complete"}}
	}
	stage := NewStageResult(StageSchematic, nil)
	stage.Summary = map[string]any{
		"operation_count":  len(plan.Output.Operations),
		"symbol_count":     countPlanOperations(plan.Output.Operations, transactions.OpAddSymbol),
		"connection_count": countPlanOperations(plan.Output.Operations, transactions.OpConnect),
	}
	stage.Summary["readability"] = schematicReadabilitySummary(plan.Output.Operations)
	return stage
}

func countPlanOperations(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}

func WorkflowIssues(result WorkflowResult) []reports.Issue {
	count := 0
	for _, stage := range result.Stages {
		count += len(stage.Issues)
	}
	issues := make([]reports.Issue, 0, count)
	for _, stage := range result.Stages {
		issues = append(issues, stage.Issues...)
	}
	return issues
}

func WorkflowArtifacts(result WorkflowResult) []reports.Artifact {
	count := 0
	for _, stage := range result.Stages {
		count += len(stage.Artifacts)
	}
	artifacts := make([]reports.Artifact, 0, count)
	for _, stage := range result.Stages {
		artifacts = append(artifacts, stage.Artifacts...)
	}
	return artifacts
}

func ParseRoutingMode(value string) (routing.RouteMode, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", nil
	case string(routing.ModeSingleLayer):
		return routing.ModeSingleLayer, nil
	case string(routing.ModeTwoLayer):
		return routing.ModeTwoLayer, nil
	case string(routing.ModeValidateOnly):
		return routing.ModeValidateOnly, nil
	default:
		return "", fmt.Errorf("unsupported routing mode %q", value)
	}
}
