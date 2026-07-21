package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/compositionlowering"
	"kicadai/internal/designworkflow"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
)

type behavioralIntentCompileOutput struct {
	Provider    aiProviderSummary                    `json:"provider"`
	Compilation behavioralintent.Result              `json:"compilation"`
	Search      *architecturesearch.SearchResult     `json:"search,omitempty"`
	ClosedLoop  *behavioralintent.ClosedLoopEvidence `json:"closed_loop,omitempty"`
	FollowUp    *behavioralintent.FollowUp           `json:"follow_up,omitempty"`
	Attempts    []aiAttemptEvidence                  `json:"attempts"`
}

type behavioralFollowUpExecution struct {
	Input            behavioralintent.FollowUp
	PriorProposal    behavioralintent.Proposal
	PriorCompilation behavioralintent.Result
}

func runBehavioralIntentCompile(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("intent compile requires --format json")
	}
	if issue := validateBehavioralIntentOptions(opts); issue != nil {
		return writeBehavioralIntentFailure(stdout, *issue)
	}
	prompt, _, _, inputIssues := loadIntentDraftText(opts)
	if len(inputIssues) != 0 {
		return writeBehavioralIntentFailure(stdout, inputIssues[0])
	}
	prompt = strings.TrimSpace(prompt)
	if len(prompt) > aiprovider.MaxPromptBytes {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "text", Message: fmt.Sprintf("intent text exceeds %d-byte provider limit", aiprovider.MaxPromptBytes)})
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: opts.catalogDir})
	if err != nil {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "catalog_dir", Message: err.Error()})
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(registryIssues) {
		return writeBehavioralIntentFailure(stdout, registryIssues[0])
	}
	capabilities, err := installedBehavioralCapabilities(catalog, registry)
	if err != nil {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities", Message: err.Error()})
	}
	installed, err := behavioralintent.ValidateInstalledCapabilities(capabilities)
	if err != nil {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities", Message: err.Error()})
	}
	contextJSON, err := behavioralintent.BuildProviderContext(prompt, capabilities)
	if err != nil {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities", Message: err.Error()})
	}
	var providerContext behavioralintent.ProviderContext
	if err := json.Unmarshal([]byte(contextJSON), &providerContext); err != nil {
		return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities", Message: err.Error()})
	}
	var followUp *behavioralFollowUpExecution
	if strings.TrimSpace(opts.behavioralFollowUp) != "" {
		followUp, err = loadBehavioralFollowUpExecution(opts)
		if err != nil {
			return writeBehavioralIntentFailure(stdout, reports.Issue{Code: behavioralintent.CodeFollowUpInvalid, Severity: reports.SeverityError, Path: "follow_up", Message: err.Error()})
		}
		contextJSON, followUpIssues := behavioralintent.BuildFollowUpProviderContext(prompt, capabilities, followUp.PriorProposal, followUp.PriorCompilation, followUp.Input)
		if reports.HasBlockingIssue(followUpIssues) {
			return writeBehavioralIntentFailure(stdout, followUpIssues[0])
		}
		if err := json.Unmarshal([]byte(contextJSON), &providerContext); err != nil {
			return writeBehavioralIntentFailure(stdout, reports.Issue{Code: behavioralintent.CodeFollowUpInvalid, Severity: reports.SeverityError, Path: "follow_up", Message: err.Error()})
		}
	}
	provider, err := aiProviderFromOptions(opts)
	if err != nil {
		return writeBehavioralIntentFailure(stdout, aiProviderIssue(err))
	}
	profile := profileWithEffectiveOutputTokenLimit(opts, aiprovider.BehavioralIntentProfile(contextJSON))
	compileProposal := func(proposal behavioralintent.Proposal) behavioralintent.Result {
		return behavioralintent.Compile(prompt, proposal, providerContext.CapabilitySHA256)
	}
	if followUp != nil {
		compileProposal = func(proposal behavioralintent.Proposal) behavioralintent.Result {
			return behavioralintent.CompileFollowUp(prompt, followUp.PriorProposal, followUp.PriorCompilation, followUp.Input, proposal, providerContext.CapabilitySHA256)
		}
	}
	providerResult, proposal, compilation, attempts, err := generateValidatedBehavioralIntentWithCompiler(ctx, provider, profile, prompt, compileProposal, opts.maxAIAttempts)
	if err != nil {
		return writeBehavioralIntentFailure(stdout, aiProviderIssue(err))
	}
	var search *architecturesearch.SearchResult
	if compilation.Status == behavioralintent.StatusReady && compilation.Requirement != nil {
		qualified := architecturesearch.Search(ctx, *compilation.Requirement, registry, architecturesearch.SearchOptions{CatalogHash: installed.CatalogSHA256})
		search = &qualified
		compilation = behavioralintent.ApplySearchEvidence(compilation, qualified)
	}
	var closedLoop *closedloopsynthesis.Report
	var designRequest *designworkflow.Request
	var synthesisIssues []reports.Issue
	if compilation.Status == behavioralintent.StatusReady && compilation.Requirement != nil && search != nil {
		models, diagnostics := modelprovenance.LoadDefault()
		if len(diagnostics) != 0 {
			return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities.models", Message: diagnostics[0].Message})
		}
		modelHash, err := modelprovenance.Hash(models)
		if err != nil || modelHash != installed.ModelRegistrySHA256 {
			return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider.capabilities.models", Message: "executing model registry does not match the installed capability snapshot"})
		}
		resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "installed", CatalogHash: installed.CatalogSHA256})
		promotion, issues := compositionlowering.SynthesizeClosedLoop(ctx, *compilation.Requirement, *search, compositionlowering.ArchitectureSimulationPlanResolver{
			GraphResolver: resolver, ProvenanceRegistry: models,
		}, modelHash, nil, closedloopsynthesis.DefaultPolicy())
		synthesisIssues = issues
		for _, issue := range synthesisIssues {
			if issue.Blocking() {
				return writeBehavioralIntentFailure(stdout, issue)
			}
		}
		if err := ctx.Err(); err != nil {
			return writeBehavioralIntentFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "closed_loop", Message: err.Error()})
		}
		closedLoop = &promotion.Report
		compilation = behavioralintent.ApplyClosedLoopEvidence(compilation, promotion.Report)
		if compilation.Status == behavioralintent.StatusReady {
			designRequest = &promotion.Request
		}
	}
	var followUpInput *behavioralintent.FollowUp
	if followUp != nil {
		followUpInput = &followUp.Input
	}
	artifacts, artifactIssues := writeBehavioralIntentArtifacts(opts, prompt, capabilities, proposal, compilation, search, closedLoop, designRequest, followUpInput, attempts)
	allIssues := append([]reports.Issue(nil), compilation.Issues...)
	allIssues = append(allIssues, synthesisIssues...)
	allIssues = append(allIssues, artifactIssues...)
	data := behavioralIntentCompileOutput{
		Provider:    aiProviderSummary{Name: providerResult.Provider, Model: providerResult.Model, ResponseID: providerResult.ResponseID, Recorded: providerResult.Recorded},
		Compilation: compilation, Search: search, ClosedLoop: compilation.ClosedLoop, FollowUp: followUpInput, Attempts: attempts,
	}
	result := reports.ResultWithIssues("intent", data, allIssues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK || compilation.Status == behavioralintent.StatusInvalid {
		return errors.New("behavioral intent compilation reported invalid provider output")
	}
	return nil
}

func installedBehavioralCapabilities(catalog *components.Catalog, registry *architecturesearch.Registry) (json.RawMessage, error) {
	architectureCapabilities, err := architecturesearch.EncodeSemanticCapabilities(registry, 0)
	if err != nil {
		return nil, err
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("load trusted model registry: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	modelHash, err := modelprovenance.Hash(models)
	if err != nil {
		return nil, err
	}
	analysisSet := map[string]bool{}
	for _, record := range models.Records {
		for _, analysis := range record.Provenance.AllowedAnalyses {
			analysisSet[analysis] = true
		}
	}
	analyses := make([]string, 0, len(analysisSet))
	for analysis := range analysisSet {
		analyses = append(analyses, analysis)
	}
	slices.Sort(analyses)
	snapshot, err := behavioralintent.BuildInstalledCapabilities(architectureCapabilities, catalogHash, modelHash, analyses)
	if err != nil {
		return nil, err
	}
	if len(snapshot) > aiprovider.MaxCapabilityBytes {
		return nil, fmt.Errorf("installed behavioral capability snapshot is %d bytes, exceeds %d-byte provider limit", len(snapshot), aiprovider.MaxCapabilityBytes)
	}
	return snapshot, nil
}

func validateBehavioralIntentOptions(opts cliOptions) *reports.Issue {
	if strings.TrimSpace(opts.aiProvider) == "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider", Message: "--provider is required for intent compile"}
	}
	if profile := strings.TrimSpace(opts.aiProfile); profile != "" && profile != aiprovider.BehavioralIntentProfileID {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "ai_profile", Message: "intent compile requires --ai-profile " + aiprovider.BehavioralIntentProfileID}
	}
	if opts.maxAIAttempts < 1 || opts.maxAIAttempts > 2 {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "max_ai_attempts", Message: "--max-ai-attempts must be 1 or 2"}
	}
	if opts.aiMaxOutputTokens != 0 {
		if err := aiprovider.ValidateOutputTokenLimit(opts.aiMaxOutputTokens); err != nil {
			return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "ai_max_output_tokens", Message: err.Error()}
		}
	}
	if strings.EqualFold(strings.TrimSpace(opts.aiProvider), "recorded") && strings.TrimSpace(opts.aiProviderRecord) == "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider_record", Message: "--provider-record is required for the recorded provider"}
	}
	if !strings.EqualFold(strings.TrimSpace(opts.aiProvider), "recorded") && strings.TrimSpace(opts.aiProviderRecord) != "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider_record", Message: "--provider-record is only valid with --provider recorded"}
	}
	if strings.TrimSpace(opts.behavioralFollowUp) != "" {
		if strings.TrimSpace(opts.output) == "" {
			return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "follow_up", Message: "--follow-up requires --output containing the prior behavioral compilation artifacts"}
		}
		if !opts.overwrite {
			return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "follow_up", Message: "--follow-up requires --overwrite to replace prior behavioral compilation artifacts after validation"}
		}
	}
	return nil
}

func generateValidatedBehavioralIntent(ctx context.Context, provider aiprovider.Provider, profile aiprovider.ReferenceProfile, prompt, capabilitySHA256 string, maxAttempts int) (aiprovider.GenerateResult, behavioralintent.Proposal, behavioralintent.Result, []aiAttemptEvidence, error) {
	return generateValidatedBehavioralIntentWithCompiler(ctx, provider, profile, prompt, func(proposal behavioralintent.Proposal) behavioralintent.Result {
		return behavioralintent.Compile(prompt, proposal, capabilitySHA256)
	}, maxAttempts)
}

func generateValidatedBehavioralIntentWithCompiler(ctx context.Context, provider aiprovider.Provider, profile aiprovider.ReferenceProfile, prompt string, compileProposal func(behavioralintent.Proposal) behavioralintent.Result, maxAttempts int) (aiprovider.GenerateResult, behavioralintent.Proposal, behavioralintent.Result, []aiAttemptEvidence, error) {
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
			return aiprovider.GenerateResult{}, behavioralintent.Proposal{}, behavioralintent.Result{}, attempts, err
		}
		result = providerResultWithOutputTokenLimit(result, profile.MaxOutputTokens)
		proposal, decodeIssues := behavioralintent.DecodeProposalStrict(bytes.NewReader(result.IntentJSON))
		compilation := compileProposal(proposal)
		compilation.Issues = append(decodeIssues, compilation.Issues...)
		status := "completed"
		if reports.HasBlockingIssue(compilation.Issues) || compilation.Status == behavioralintent.StatusInvalid {
			status = "invalid"
		}
		attempts = append(attempts, aiAttemptEvidence{
			Attempt: attempt, Provider: result.Provider, Model: result.Model, ResponseID: result.ResponseID,
			Status: status, Diagnostics: append([]aiprovider.Diagnostic(nil), diagnostics...), IntentHash: hashBytes(result.IntentJSON),
			MaxOutputTokens: result.MaxOutputTokens, Usage: result.Usage, FinishReason: result.FinishReason,
		})
		if status == "completed" {
			return result, proposal, compilation, attempts, nil
		}
		diagnostics = behavioralIntentRetryDiagnostics(compilation.Issues)
		if attempt >= maxAttempts || len(diagnostics) == 0 {
			return result, proposal, compilation, attempts, nil
		}
	}
	return aiprovider.GenerateResult{}, behavioralintent.Proposal{}, behavioralintent.Result{}, attempts, &aiprovider.ProviderError{Code: aiprovider.ErrorIncomplete, Message: "behavioral intent attempts exhausted", MaxOutputTokens: profile.MaxOutputTokens}
}

func behavioralIntentRetryDiagnostics(issues []reports.Issue) []aiprovider.Diagnostic {
	diagnostics := make([]aiprovider.Diagnostic, 0, aiprovider.MaxDiagnostics)
	for _, issue := range issues {
		if !issue.Blocking() {
			continue
		}
		diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: string(issue.Code), Path: issue.Path, Message: boundedDiagnosticMessage(issue.Message)})
		if len(diagnostics) == aiprovider.MaxDiagnostics {
			break
		}
	}
	return diagnostics
}

func writeBehavioralIntentArtifacts(opts cliOptions, prompt string, capabilities json.RawMessage, proposal behavioralintent.Proposal, compilation behavioralintent.Result, search *architecturesearch.SearchResult, closedLoop *closedloopsynthesis.Report, designRequest *designworkflow.Request, followUp *behavioralintent.FollowUp, attempts []aiAttemptEvidence) ([]reports.Artifact, []reports.Issue) {
	if strings.TrimSpace(opts.output) == "" {
		return nil, nil
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: err.Error()}}
	}
	type artifactValue struct {
		name        string
		value       any
		kind        reports.ArtifactKind
		description string
	}
	values := []artifactValue{
		{name: "behavioral-capabilities.json", value: json.RawMessage(capabilities), kind: reports.ArtifactValidationReport, description: "installed behavioral capability snapshot"},
		{name: "behavioral-proposal.json", value: proposal, kind: reports.ArtifactPreview, description: "untrusted behavioral intent proposal"},
		{name: "behavioral-compilation.json", value: compilation, kind: reports.ArtifactValidationReport, description: "validated behavioral intent compilation"},
		{name: "behavioral-attempts.json", value: attempts, kind: reports.ArtifactValidationReport, description: "bounded provider attempt evidence"},
	}
	if compilation.Status == behavioralintent.StatusNeedsClarification {
		answers := make([]behavioralintent.ClarificationAnswer, 0, len(compilation.Clarifications))
		for _, clarification := range compilation.Clarifications {
			answers = append(answers, behavioralintent.ClarificationAnswer{ClarificationID: clarification.ID, UncertaintyIDs: slices.Clone(clarification.UncertaintyIDs)})
		}
		template, err := behavioralintent.BindFollowUp(proposal, compilation, answers)
		if err != nil {
			return nil, []reports.Issue{{Code: behavioralintent.CodeFollowUpInvalid, Severity: reports.SeverityError, Path: ".kicadai/behavioral-follow-up-template.json", Message: err.Error()}}
		}
		values = append(values, artifactValue{name: "behavioral-follow-up-template.json", value: template, kind: reports.ArtifactPreview, description: "source- and capability-bound clarification answer template"})
	}
	if search != nil {
		values = append(values, artifactValue{name: "behavioral-architecture-search.json", value: search, kind: reports.ArtifactValidationReport, description: "deterministic architecture-search qualification evidence"})
	}
	if closedLoop != nil {
		values = append(values, artifactValue{name: "behavioral-closed-loop.json", value: closedLoop, kind: reports.ArtifactValidationReport, description: "trusted all-corner closed-loop simulation evidence"})
	}
	if designRequest != nil {
		values = append(values, artifactValue{name: "behavioral-design-request.json", value: designRequest, kind: reports.ArtifactPreview, description: "deterministic design request bound to the selected simulated circuit"})
	}
	if followUp != nil {
		values = append(values, artifactValue{name: "behavioral-follow-up.json", value: followUp, kind: reports.ArtifactValidationReport, description: "validated clarification answer evidence"})
	}
	var artifacts []reports.Artifact
	var issues []reports.Issue
	for _, value := range values {
		data, err := json.MarshalIndent(value.value, "", "  ")
		relPath := filepath.ToSlash(filepath.Join(".kicadai", value.name))
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: relPath, Message: err.Error()})
			continue
		}
		if issue := writeLocalArtifact(filepath.Join(artifactDir, value.name), append(data, '\n'), opts.overwrite); issue != nil {
			issue.Path = relPath
			issues = append(issues, *issue)
			continue
		}
		artifacts = append(artifacts, reports.Artifact{Kind: value.kind, Path: relPath, Description: value.description})
	}
	sourcePath := filepath.Join(artifactDir, "behavioral-source.txt")
	if issue := writeLocalArtifact(sourcePath, []byte(prompt+"\n"), opts.overwrite); issue != nil {
		issue.Path = ".kicadai/behavioral-source.txt"
		issues = append(issues, *issue)
	} else {
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactPreview, Path: ".kicadai/behavioral-source.txt", Description: "original behavioral intent source"})
	}
	return artifacts, issues
}

func loadBehavioralFollowUpExecution(opts cliOptions) (*behavioralFollowUpExecution, error) {
	followUpFile, err := os.Open(strings.TrimSpace(opts.behavioralFollowUp))
	if err != nil {
		return nil, fmt.Errorf("open follow-up: %w", err)
	}
	defer followUpFile.Close()
	followUp, issues := behavioralintent.DecodeFollowUpStrict(followUpFile)
	if reports.HasBlockingIssue(issues) {
		return nil, errors.New(issues[0].Message)
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	proposalFile, err := os.Open(filepath.Join(artifactDir, "behavioral-proposal.json"))
	if err != nil {
		return nil, fmt.Errorf("open prior behavioral proposal: %w", err)
	}
	defer proposalFile.Close()
	priorProposal, proposalIssues := behavioralintent.DecodeProposalStrict(proposalFile)
	if reports.HasBlockingIssue(proposalIssues) {
		return nil, errors.New(proposalIssues[0].Message)
	}
	var priorCompilation behavioralintent.Result
	if err := decodeBehavioralArtifact(filepath.Join(artifactDir, "behavioral-compilation.json"), &priorCompilation); err != nil {
		return nil, fmt.Errorf("decode prior behavioral compilation: %w", err)
	}
	return &behavioralFollowUpExecution{Input: followUp, PriorProposal: priorProposal, PriorCompilation: priorCompilation}, nil
}

func decodeBehavioralArtifact(path string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, behavioralintent.MaxProposalBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("artifact must contain exactly one JSON object")
	}
	return nil
}

func writeBehavioralIntentFailure(stdout io.Writer, issue reports.Issue) error {
	if err := writeReportJSON(stdout, reports.ResultWithIssues("intent", nil, []reports.Issue{issue}, nil)); err != nil {
		return err
	}
	return errors.New(issue.Message)
}
