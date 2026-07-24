package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/compositionlowering"
	"kicadai/internal/creationevidence"
	"kicadai/internal/designworkflow"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
)

func runRequirement(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) != 1 || strings.TrimSpace(opts.commandArgs[0]) != "create" {
		return writeRequirementFailure(stdout, reports.Issue{
			Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "requirement",
			Message:    "requirement requires subcommand: create",
			Suggestion: "run kicadai requirement create --request requirement.json --output ./out/project",
		})
	}
	if !opts.jsonOutput {
		return errors.New("requirement create requires --format json")
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "--request is required"})
	}
	if strings.TrimSpace(opts.output) == "" {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "--output is required"})
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()})
	}
	defer file.Close()
	requirement, decodeIssues := architecturesearch.DecodeStrict(file)
	if reports.HasBlockingIssue(decodeIssues) {
		return writeRequirementIssues(stdout, decodeIssues)
	}

	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: opts.catalogDir})
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "catalog", Message: err.Error()})
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(registryIssues) {
		return writeRequirementIssues(stdout, registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		issues := make([]reports.Issue, 0, len(provenanceDiagnostics))
		for _, diagnostic := range provenanceDiagnostics {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: filepath.ToSlash(filepath.Join("model_provenance", diagnostic.Path)), Message: diagnostic.Message,
			})
		}
		return writeRequirementIssues(stdout, issues)
	}
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "model_provenance", Message: err.Error()})
	}

	search := architecturesearch.Search(ctx, requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		issues := append([]reports.Issue(nil), search.Issues...)
		if len(issues) == 0 {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: "architecture", Message: "behavior-only architecture search did not select a complete candidate",
			})
		}
		return writeRequirementIssues(stdout, issues)
	}
	promotion, promotionIssues := compositionlowering.SynthesizeClosedLoop(
		ctx,
		requirement,
		search,
		compositionlowering.ArchitectureSimulationPlanResolver{
			GraphResolver: resolver, ProvenanceRegistry: provenance,
		},
		modelRegistryHash,
		nil,
		closedloopsynthesis.DefaultPolicy(),
	)
	if reports.HasBlockingIssue(promotionIssues) || promotion.Report.Status != "pass" {
		if len(promotionIssues) == 0 {
			promotionIssues = append(promotionIssues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: "closed_loop", Message: "closed-loop synthesis did not select a passing architecture",
			})
		}
		return writeRequirementIssues(stdout, promotionIssues)
	}

	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()})
	}
	createOpts, err := designCreateOptions(ctx, opts, checkOpts)
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "mode", Message: err.Error()})
	}
	workflow := designworkflow.Create(ctx, promotion.Request, createOpts)
	promotionFixture, err := designPromotionFixture(opts, promotion.Request, workflow)
	if err != nil {
		return writeRequirementFailure(stdout, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()})
	}
	physicalPromotion := designworkflow.BuildInternalPromotionReport(promotionFixture, workflow)
	workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(physicalPromotion, designworkflow.PromotionReportArtifactPath))

	artifactRoot := filepath.Join(opts.output, ".kicadai")
	extraArtifacts := []reports.Artifact{}
	extraIssues := []reports.Issue{}
	for _, artifact := range []struct {
		name        string
		value       any
		description string
	}{
		{name: "normalized-requirement.json", value: architecturesearch.Normalize(requirement), description: "normalized behavior-only requirement"},
		{name: "architecture-search.json", value: search, description: "deterministic architecture selection and evidence hashes"},
	} {
		written, issue := writeJSONArtifact(
			filepath.Join(artifactRoot, artifact.name),
			artifact.value,
			reports.ArtifactValidationReport,
			filepath.ToSlash(filepath.Join(".kicadai", artifact.name)),
			artifact.description,
		)
		if issue != nil {
			extraIssues = append(extraIssues, *issue)
			continue
		}
		extraArtifacts = append(extraArtifacts, written)
	}
	coreArtifacts, artifactIssues := creationevidence.Write(opts.output, creationevidence.Bundle{
		Lane:     "requirement",
		Request:  promotion.Request,
		Workflow: workflow,
		Validation: creationevidence.ValidationSummary{
			Status: string(physicalPromotion.Status), Stage: "promotion",
			Message: physicalPromotion.Summary, Gates: creationevidence.GatesFromWorkflow(workflow),
		},
		Promotion: &physicalPromotion,
		Artifacts: normalizeManifestArtifacts(
			opts.output,
			append(designworkflow.WorkflowArtifacts(workflow), extraArtifacts...),
		),
	})
	extraIssues = append(extraIssues, artifactIssues...)
	result := designWorkflowReport(workflow, extraIssues, coreArtifacts)
	result.Command = "requirement.create"
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK || reports.HasBlockingIssue(result.Issues) {
		return errors.New("requirement create reported blocking issues")
	}
	return nil
}

func writeRequirementFailure(stdout io.Writer, issue reports.Issue) error {
	if err := writeReportFailure(stdout, "requirement.create", issue); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func writeRequirementIssues(stdout io.Writer, issues []reports.Issue) error {
	result := reports.ResultWithIssues("requirement.create", nil, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	return errors.New("requirement create reported blocking issues")
}
