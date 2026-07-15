package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

type aiGraphDesignCreateResult struct {
	Provider   aiProviderSummary             `json:"provider"`
	Graph      circuitgraph.Document         `json:"graph"`
	Resolution circuitgraph.ResolvedDocument `json:"resolution"`
	Request    designworkflow.Request        `json:"request"`
	Workflow   designworkflow.WorkflowResult `json:"workflow,omitempty"`
	AIStatus   *aiLaneStatus                 `json:"ai_status,omitempty"`
}

func runAIGenericCircuitCreate(ctx context.Context, opts cliOptions, prompt, promptSource string, stdout io.Writer) error {
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()})
	}
	createOpts, err := designCreateOptions(ctx, opts, checkOpts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "mode", Message: err.Error()})
	}
	if createOpts.LibraryIndex == nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library", Message: "generic circuit profile requires --symbols-root and --footprints-root or a populated --library-cache"})
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: opts.catalogDir})
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "catalog_dir", Message: err.Error()})
	}
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, aiprovider.MaxCapabilityBytes)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "provider.capability", Message: err.Error()})
	}
	provider, err := aiProviderFromOptions(opts)
	if err != nil {
		return writeDesignFailure(stdout, aiProviderIssue(err))
	}
	profile := profileWithEffectiveOutputTokenLimit(opts, aiprovider.GenericCircuitProfile(capability))
	replayCapture := newAIReplayCapture(opts, profile.ID)
	symbols, footprints := circuitgraph.LibraryEvidenceFromIndex(*createOpts.LibraryIndex)
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{
		Catalog: catalog, CatalogID: "catalog:" + filepath.Base(filepath.Clean(opts.catalogDir)), LibrarySymbols: symbols,
		LibraryFootprints: footprints, RequireLibraryEvidence: true,
	})
	providerResult, graph, resolved, request, attempts, issues, err := generateValidatedAIGraph(ctx, provider, profile, resolver, prompt, opts.maxAIAttempts, replayCapture.Capture)
	if err != nil {
		return writeAIProviderFailure(stdout, aiProviderIssue(err), replayCapture)
	}
	if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
		return writeAIGraphPreflightFailure(stdout, providerResult, graph, resolved, attempts, issues, replayCapture)
	}
	workflow := designworkflow.Create(ctx, request, createOpts)
	if err := replayCapture.Restore(); err != nil {
		return writeAIProviderFailure(stdout, aiProviderIssue(err), replayCapture)
	}
	promotion := designworkflow.BuildInternalPromotionReport(designPromotionFixture(opts, request, workflow), workflow)
	workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(promotion, designworkflow.PromotionReportArtifactPath))
	promotionArtifact, promotionIssue := designworkflow.WritePromotionReportArtifact(opts.output, promotion, opts.overwrite)
	var artifacts []reports.Artifact
	var artifactIssues []reports.Issue
	if promotionIssue != nil {
		artifactIssues = append(artifactIssues, *promotionIssue)
	} else if promotionArtifact.Path != "" {
		artifacts = append(artifacts, promotionArtifact)
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	artifactIssues = append(artifactIssues, writeWorkflowResultArtifact(artifactDir, workflow)...)
	if len(artifactIssues) == 0 {
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactValidationReport, Path: ".kicadai/workflow-result.json", Description: "generic AI design workflow result"})
	}
	if correctionArtifact, correctionIssue := writeAutonomousCorrectionArtifact(artifactDir, request, workflow); correctionIssue != nil {
		artifactIssues = append(artifactIssues, *correctionIssue)
	} else {
		artifacts = append(artifacts, correctionArtifact)
	}
	graphArtifacts, graphIssues := writeAIGraphArtifacts(artifactDir, graph, resolved)
	artifacts = append(artifacts, graphArtifacts...)
	artifactIssues = append(artifactIssues, graphIssues...)
	providerArtifacts, providerIssues := writeAIGraphProviderArtifacts(artifactDir, opts, promptSource, prompt, graph, providerResult, attempts)
	providerArtifacts = append(providerArtifacts, replayCapture.Artifacts()...)
	artifacts = append(artifacts, providerArtifacts...)
	artifactIssues = append(artifactIssues, providerIssues...)
	allIssues := append([]reports.Issue(nil), issues...)
	allIssues = append(allIssues, artifactIssues...)
	allIssues = append(allIssues, designworkflow.WorkflowIssues(workflow)...)
	plan := intentplanner.PlanResult{Status: intentplanner.PlanStatusReady, GeneratedRequest: &request}
	status := buildAILaneStatus(plan, &workflow, allIssues, artifacts)
	status = aiLaneStatusWithPromotionEvidence(status, promotion)
	aiArtifacts, aiArtifactIssues := writeAILaneArtifacts(opts.output, plan, nil, prompt, status, artifacts)
	artifacts = append(artifacts, aiArtifacts...)
	allIssues = append(allIssues, aiArtifactIssues...)
	status.ArtifactPaths = artifactPaths(artifacts)
	data := aiGraphDesignCreateResult{
		Provider: replayCapture.ProviderSummary(providerResult),
		Graph:    graph, Resolution: resolved, Request: request, Workflow: workflow, AIStatus: &status,
	}
	result := reports.Result{
		OK:      designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved) && !reports.HasBlockingIssue(allIssues),
		Command: "design", Version: reports.Version, Data: data, Issues: allIssues, Artifacts: artifacts,
	}
	if result.Issues == nil {
		result.Issues = []reports.Issue{}
	}
	if result.Artifacts == nil {
		result.Artifacts = []reports.Artifact{}
	}
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("generic AI design create reported blocking issues")
	}
	return nil
}

func writeAutonomousCorrectionArtifact(artifactDir string, request designworkflow.Request, workflow designworkflow.WorkflowResult) (reports.Artifact, *reports.Issue) {
	report, ok := designworkflow.AutonomousCorrectionEvidence(request, workflow)
	if !ok {
		return reports.Artifact{}, &reports.Issue{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
			Path: ".kicadai/autonomous-correction.json", Message: "generic workflow did not produce autonomous correction evidence",
		}
	}
	return writeJSONArtifact(
		filepath.Join(artifactDir, "autonomous-correction.json"), report,
		reports.ArtifactValidationReport, ".kicadai/autonomous-correction.json",
		"generic autonomous placement and routing correction evidence",
	)
}

func generateValidatedAIGraph(ctx context.Context, provider aiprovider.Provider, profile aiprovider.ReferenceProfile, resolver *circuitgraph.Resolver, prompt string, maxAttempts int, captures ...aiProviderCaptureFunc) (aiprovider.GenerateResult, circuitgraph.Document, circuitgraph.ResolvedDocument, designworkflow.Request, []aiAttemptEvidence, []reports.Issue, error) {
	var diagnostics []aiprovider.Diagnostic
	var attempts []aiAttemptEvidence
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := provider.GenerateIntent(ctx, aiprovider.GenerateRequest{
			Prompt: prompt, CapabilityContext: profile.CapabilityContext, OutputSchemaName: profile.SchemaName,
			OutputSchema: profile.IntentEnvelopeSchema(), SchemaVersion: aiprovider.EnvelopeSchemaV1,
			Attempt: attempt, Diagnostics: diagnostics, MaxOutputTokens: profile.MaxOutputTokens,
		})
		if err != nil {
			attempts = append(attempts, aiProviderErrorAttempt(attempt, provider.Name(), profile.MaxOutputTokens, diagnostics, err))
			if attempt < maxAttempts && aiProviderErrorRetryable(err) {
				diagnostics = []aiprovider.Diagnostic{{Code: string(aiprovider.ErrorCodeOf(err)), Path: "provider", Message: boundedDiagnosticMessage(err.Error())}}
				continue
			}
			return aiprovider.GenerateResult{}, circuitgraph.Document{}, circuitgraph.ResolvedDocument{}, designworkflow.Request{}, attempts, nil, err
		}
		result = providerResultWithOutputTokenLimit(result, profile.MaxOutputTokens)
		if err := runAIProviderCaptures(captures, result); err != nil {
			return aiprovider.GenerateResult{}, circuitgraph.Document{}, circuitgraph.ResolvedDocument{}, designworkflow.Request{}, attempts, nil, err
		}
		decode := circuitgraph.DecodeStrict
		if result.Recorded {
			decode = circuitgraph.DecodeRecordedStrict
		}
		graph, issues := decode(bytes.NewReader(result.IntentJSON))
		var resolved circuitgraph.ResolvedDocument
		var request designworkflow.Request
		if !reports.HasBlockingIssue(issues) {
			resolved, issues = resolver.Resolve(ctx, graph)
		}
		if !reports.HasBlockingIssue(issues) {
			var loweringIssues []reports.Issue
			request, loweringIssues = circuitgraph.ToDesignRequest(resolved)
			issues = append(issues, loweringIssues...)
		}
		issues = prefixAIGraphIssues(issues)
		graphData, marshalErr := json.Marshal(graph)
		if marshalErr != nil {
			return result, graph, resolved, request, attempts, issues, fmt.Errorf("encode normalized circuit graph: %w", marshalErr)
		}
		status := "completed"
		if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
			status = "invalid"
		}
		attempts = append(attempts, aiAttemptEvidence{
			Attempt: attempt, Provider: result.Provider, Model: result.Model, ResponseID: result.ResponseID,
			Status: status, Diagnostics: append([]aiprovider.Diagnostic(nil), diagnostics...), IntentHash: hashBytes(graphData),
			MaxOutputTokens: result.MaxOutputTokens, Usage: result.Usage, FinishReason: result.FinishReason,
		})
		if status == "completed" {
			return result, graph, resolved, request, attempts, issues, nil
		}
		diagnostics = aiGraphRetryDiagnostics(issues)
		if attempt >= maxAttempts || len(diagnostics) == 0 {
			return result, graph, resolved, request, attempts, issues, nil
		}
	}
	return aiprovider.GenerateResult{}, circuitgraph.Document{}, circuitgraph.ResolvedDocument{}, designworkflow.Request{}, attempts, nil, &aiprovider.ProviderError{
		Code: aiprovider.ErrorIncomplete, Message: "generic AI graph attempts exhausted", MaxOutputTokens: profile.MaxOutputTokens,
	}
}

func prefixAIGraphIssues(issues []reports.Issue) []reports.Issue {
	result := append([]reports.Issue(nil), issues...)
	for index := range result {
		path := strings.TrimSpace(result[index].Path)
		if path == "" || path == "document" {
			result[index].Path = "provider.graph"
		} else if !strings.HasPrefix(path, "provider.graph") {
			result[index].Path = "provider.graph." + path
		}
	}
	return result
}

func aiGraphRetryDiagnostics(issues []reports.Issue) []aiprovider.Diagnostic {
	diagnostics := make([]aiprovider.Diagnostic, 0, aiprovider.MaxDiagnostics)
	for _, issue := range issues {
		if !issue.Blocking() || !strings.HasPrefix(issue.Path, "provider.graph") {
			continue
		}
		diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: string(issue.Code), Path: issue.Path, Message: boundedDiagnosticMessage(issue.Message)})
		if len(diagnostics) == aiprovider.MaxDiagnostics {
			break
		}
	}
	return diagnostics
}

func writeAIGraphPreflightFailure(stdout io.Writer, provider aiprovider.GenerateResult, graph circuitgraph.Document, resolved circuitgraph.ResolvedDocument, attempts []aiAttemptEvidence, issues []reports.Issue, capture *aiReplayCapture) error {
	result := reports.Result{
		OK: false, Command: "design", Version: reports.Version,
		Data: map[string]any{
			"provider": capture.ProviderSummary(provider),
			"graph":    graph, "resolution": resolved, "attempts": attempts,
		},
		Issues: issues, Artifacts: capture.Artifacts(),
	}
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	return errors.New("generic AI graph preflight failed")
}

func writeAIGraphArtifacts(artifactDir string, graph circuitgraph.Document, resolved circuitgraph.ResolvedDocument) ([]reports.Artifact, []reports.Issue) {
	files := []struct {
		name, path, description string
		value                   any
	}{
		{name: "circuit-graph.json", path: ".kicadai/circuit-graph.json", description: "validated generic AI circuit graph", value: graph},
		{name: "circuit-resolution.json", path: ".kicadai/circuit-resolution.json", description: "catalog and library resolution evidence", value: resolved},
	}
	var artifacts []reports.Artifact
	var issues []reports.Issue
	for _, file := range files {
		artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, file.name), file.value, reports.ArtifactValidationReport, file.path, file.description)
		if issue != nil {
			issues = append(issues, *issue)
		} else {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, issues
}

func writeAIGraphProviderArtifacts(artifactDir string, opts cliOptions, promptSource, prompt string, graph circuitgraph.Document, result aiprovider.GenerateResult, attempts []aiAttemptEvidence) ([]reports.Artifact, []reports.Issue) {
	graphData, err := json.Marshal(graph)
	if err != nil {
		return nil, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: ".kicadai/ai-response.json", Message: "encode normalized AI graph: " + err.Error()}}
	}
	requestEvidence := aiRequestEvidence{
		Schema: aiprovider.EnvelopeSchemaV1, Profile: circuitgraph.ProviderProfileID, Provider: strings.ToLower(strings.TrimSpace(opts.aiProvider)), Model: result.Model,
		PromptSource: promptSource, PromptHash: hashText(prompt), Attempt: len(attempts), MaxAttempts: opts.maxAIAttempts, Background: result.Background,
		MaxOutputTokens: result.MaxOutputTokens,
	}
	responseEvidence := struct {
		Schema       string                `json:"schema"`
		Profile      string                `json:"profile"`
		Provider     string                `json:"provider"`
		Model        string                `json:"model,omitempty"`
		ResponseID   string                `json:"response_id,omitempty"`
		Graph        circuitgraph.Document `json:"graph"`
		GraphHash    string                `json:"graph_hash"`
		Usage        aiprovider.Usage      `json:"usage,omitempty"`
		FinishReason string                `json:"finish_reason,omitempty"`
		Recorded     bool                  `json:"recorded,omitempty"`
	}{
		Schema: aiprovider.EnvelopeSchemaV1, Profile: circuitgraph.ProviderProfileID, Provider: result.Provider,
		Model: result.Model, ResponseID: result.ResponseID, Graph: graph, GraphHash: hashBytes(graphData),
		Usage: result.Usage, FinishReason: result.FinishReason, Recorded: result.Recorded,
	}
	attemptHistory := aiAttemptsEvidence{Schema: "kicadai.ai.attempts.v1", Attempts: append([]aiAttemptEvidence(nil), attempts...)}
	files := []struct {
		name, description string
		value             any
	}{
		{name: "ai-request.json", description: "sanitized generic AI provider request evidence", value: requestEvidence},
		{name: "ai-response.json", description: "validated generic AI provider response evidence", value: responseEvidence},
		{name: "ai-attempts.json", description: "bounded generic AI provider attempt history", value: attemptHistory},
	}
	var artifacts []reports.Artifact
	var issues []reports.Issue
	for _, file := range files {
		path := filepath.ToSlash(filepath.Join(".kicadai", file.name))
		artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, file.name), file.value, reports.ArtifactValidationReport, path, file.description)
		if issue != nil {
			issues = append(issues, *issue)
		} else {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, issues
}
