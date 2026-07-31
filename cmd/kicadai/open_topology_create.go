package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

type openTopologyCreateResult struct {
	Synthesis openTopologySynthesisSummary  `json:"synthesis"`
	Promotion *openTopologyPromotionSummary `json:"promotion,omitempty"`
}

type openTopologySynthesisSummary struct {
	Status           opentopologysynthesis.Status      `json:"status"`
	StopReason       opentopologysynthesis.StopReason  `json:"stop_reason"`
	RequirementHash  string                            `json:"requirement_hash"`
	InventoryHash    string                            `json:"inventory_hash"`
	PolicyHash       string                            `json:"policy_hash"`
	Consumption      opentopologysynthesis.Consumption `json:"consumption"`
	SelectedTopology string                            `json:"selected_topology_hash,omitempty"`
	PhysicalHash     string                            `json:"physical_hash,omitempty"`
	EvidenceHash     string                            `json:"evidence_hash"`
	EvidenceArtifact string                            `json:"evidence_artifact,omitempty"`
}

type openTopologyPromotionSummary struct {
	Status           opentopologysynthesis.PhysicalPromotionStatus `json:"status"`
	ReplayIdentical  bool                                          `json:"replay_identical"`
	ProjectHash      string                                        `json:"project_hash,omitempty"`
	RunCount         int                                           `json:"run_count"`
	EvidenceHash     string                                        `json:"evidence_hash"`
	EvidenceArtifact string                                        `json:"evidence_artifact,omitempty"`
}

func runOpenTopology(
	ctx context.Context,
	opts cliOptions,
	stdout io.Writer,
) error {
	if len(opts.commandArgs) != 1 ||
		strings.TrimSpace(opts.commandArgs[0]) != "create" {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "open_topology",
			Message:  "open-topology requires subcommand: create",
			Suggestion: "run kicadai open-topology create " +
				"--request requirement.json --output ./out",
		})
	}
	if !opts.jsonOutput {
		return errors.New("open-topology create requires --format json")
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "request",
			Message:  "--request is required",
		})
	}
	if strings.TrimSpace(opts.output) == "" {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "output",
			Message:  "--output is required",
		})
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     opts.requestPath,
			Message:  err.Error(),
		})
	}
	defer func() { _ = file.Close() }()
	requirement, decodeIssues :=
		opentopologysynthesis.DecodeStrict(file)
	if reports.HasBlockingIssue(decodeIssues) {
		return writeOpenTopologyIssues(stdout, decodeIssues)
	}
	catalog, err := loadComponentCatalogForOptions(ctx, opts)
	if err != nil {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "catalog",
			Message:  err.Error(),
		})
	}
	provenance, err := loadComponentModelsForOptions(ctx, opts)
	if err != nil {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Path:     "model_provenance",
			Message:  err.Error(),
		})
	}
	catalogHash := circuitgraph.NewResolver(
		circuitgraph.ResolveOptions{Catalog: catalog},
	).CatalogHash()
	inventory, inventoryIssues :=
		opentopologysynthesis.BuildPrimitiveInventory(
			catalog,
			catalogHash,
			provenance,
		)
	if reports.HasBlockingIssue(inventoryIssues) {
		return writeOpenTopologyIssues(stdout, inventoryIssues)
	}
	environment := opentopologysynthesis.SimulationEnvironment{
		Catalog:       catalog,
		CatalogHash:   catalogHash,
		ModelRegistry: provenance,
	}
	synthesis := opentopologysynthesis.Synthesize(
		ctx,
		requirement,
		inventory,
		environment,
		opentopologysynthesis.DefaultPolicy(),
	)
	data := openTopologyCreateResult{
		Synthesis: summarizeOpenTopologySynthesis(synthesis),
	}
	if synthesis.Report.Status != opentopologysynthesis.StatusPassed {
		issues := openTopologySynthesisIssues(synthesis)
		result := reports.ResultWithIssues(
			"open-topology.create",
			data,
			issues,
			nil,
		)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		return errors.New("open-topology synthesis did not produce a passing graph")
	}
	roots := libraryRootsFromOptions(opts)
	if strings.TrimSpace(roots.SymbolsRoot) == "" ||
		strings.TrimSpace(roots.FootprintsRoot) == "" {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityBlocked,
			Path:     "library_roots",
			Message: "open-topology physical promotion requires " +
				"symbol and footprint library roots",
			Suggestion: "set --symbols-root and --footprints-root",
		})
	}
	index, libraryIssues := libraryresolver.Load(
		ctx,
		roots,
		libraryresolver.LoadOptions{
			CachePath: opts.libraryCache,
			Refresh:   opts.refreshLibraryCache,
		},
	)
	if reports.HasBlockingIssue(libraryIssues) {
		return writeOpenTopologyIssues(stdout, libraryIssues)
	}
	cli, err := checks.DiscoverCLI(opts.kicadCLI)
	if err != nil {
		return writeOpenTopologyFailure(stdout, reports.Issue{
			Code:       reports.CodeKiCadCLIFailed,
			Severity:   reports.SeverityBlocked,
			Path:       "kicad_cli",
			Message:    err.Error(),
			Suggestion: "set --kicad-cli to the installed executable",
		})
	}
	timeout := 2 * time.Minute
	if strings.TrimSpace(opts.roundTimeout) != "" {
		parsed, parseErr := time.ParseDuration(opts.roundTimeout)
		if parseErr != nil || parsed <= 0 {
			return writeOpenTopologyFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "timeout",
				Message:  "timeout must be a positive duration",
			})
		}
		timeout = parsed
	}
	promotion := opentopologysynthesis.PromoteSynthesisRun(
		ctx,
		synthesis,
		environment,
		opentopologysynthesis.PhysicalPromotionOptions{
			OutputRoot:    opts.output,
			Overwrite:     opts.overwrite,
			KiCadCLI:      cli.Path,
			LibraryIndex:  &index,
			Timeout:       timeout,
			KeepArtifacts: opts.keepArtifacts,
		},
	)
	artifacts := []reports.Artifact{}
	for _, run := range promotion.Runs {
		artifacts = append(artifacts, run.Artifacts...)
	}
	artifactRoot := filepath.Join(opts.output, ".kicadai")
	artifactIssues := []reports.Issue{}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		artifactIssues = append(artifactIssues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     ".kicadai",
			Message:  err.Error(),
		})
	}
	synthesisEvidenceWritten := false
	promotionEvidenceWritten := false
	for _, artifact := range []struct {
		name        string
		value       any
		description string
	}{
		{
			name:        "open-topology-synthesis.json",
			value:       synthesis,
			description: "bounded primitive topology and simulation evidence",
		},
		{
			name:        "open-topology-promotion.json",
			value:       promotion,
			description: "two-run KiCad physical promotion evidence",
		},
	} {
		written, issue := writeJSONArtifact(
			filepath.Join(artifactRoot, artifact.name),
			artifact.value,
			reports.ArtifactValidationReport,
			filepath.ToSlash(filepath.Join(".kicadai", artifact.name)),
			artifact.description,
		)
		if issue != nil {
			artifactIssues = append(artifactIssues, *issue)
			continue
		}
		switch artifact.name {
		case "open-topology-synthesis.json":
			synthesisEvidenceWritten = true
		case "open-topology-promotion.json":
			promotionEvidenceWritten = true
		}
		artifacts = append(artifacts, written)
	}
	if synthesisEvidenceWritten {
		data.Synthesis.EvidenceArtifact =
			filepath.ToSlash(filepath.Join(".kicadai", "open-topology-synthesis.json"))
	}
	promotionSummary := summarizeOpenTopologyPromotion(promotion)
	if promotionEvidenceWritten {
		promotionSummary.EvidenceArtifact =
			filepath.ToSlash(filepath.Join(".kicadai", "open-topology-promotion.json"))
	}
	data.Promotion = &promotionSummary
	resultIssues := append([]reports.Issue(nil), promotion.Issues...)
	resultIssues = append(resultIssues, artifactIssues...)
	result := reports.ResultWithIssues(
		"open-topology.create",
		data,
		reports.SortedIssues(resultIssues),
		artifacts,
	)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK ||
		promotion.Status != opentopologysynthesis.PhysicalPromotionPassed {
		return errors.New("open-topology physical promotion reported blocking issues")
	}
	return nil
}

func summarizeOpenTopologySynthesis(
	run opentopologysynthesis.SynthesisRun,
) openTopologySynthesisSummary {
	summary := openTopologySynthesisSummary{
		Status:          run.Report.Status,
		StopReason:      run.Report.StopReason,
		RequirementHash: run.Report.RequirementHash,
		InventoryHash:   run.Report.PrimitiveInventoryHash,
		PolicyHash:      run.Report.PolicyHash,
		Consumption:     run.Report.Consumption,
		EvidenceHash:    run.Hash,
	}
	if run.Report.Selected != nil {
		summary.SelectedTopology = run.Report.Selected.TopologyHash
		summary.PhysicalHash = run.Report.Selected.PhysicalHash
	}
	return summary
}

func summarizeOpenTopologyPromotion(
	promotion opentopologysynthesis.PhysicalPromotionResult,
) openTopologyPromotionSummary {
	return openTopologyPromotionSummary{
		Status:          promotion.Status,
		ReplayIdentical: promotion.ReplayIdentical,
		ProjectHash:     promotion.ProjectHash,
		RunCount:        len(promotion.Runs),
		EvidenceHash:    promotion.Hash,
	}
}

func openTopologySynthesisIssues(
	run opentopologysynthesis.SynthesisRun,
) []reports.Issue {
	issues := []reports.Issue{}
	for _, diagnostic := range run.Report.Diagnostics {
		issues = append(issues, reports.Issue{
			Code:       reports.Code(diagnostic.Code),
			Severity:   reports.SeverityBlocked,
			Stage:      "open_topology_synthesis",
			Path:       diagnostic.Path,
			Message:    diagnostic.Message,
			Suggestion: diagnostic.Suggestion,
		})
	}
	if len(issues) == 0 {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Stage:    "open_topology_synthesis",
			Path:     "synthesis",
			Message: "bounded primitive topology search did not produce " +
				"a passing graph",
		})
	}
	return reports.SortedIssues(issues)
}

func writeOpenTopologyFailure(
	stdout io.Writer,
	issue reports.Issue,
) error {
	if err := writeReportJSON(
		stdout,
		reports.ErrorResult("open-topology.create", issue),
	); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func writeOpenTopologyIssues(
	stdout io.Writer,
	issues []reports.Issue,
) error {
	result := reports.ResultWithIssues(
		"open-topology.create",
		nil,
		issues,
		nil,
	)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	return errors.New("open-topology create reported blocking issues")
}
