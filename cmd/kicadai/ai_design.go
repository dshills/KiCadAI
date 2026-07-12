package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

const aiReferenceSchemaName = "kicadai_bmp280_intent_v1"

type aiDesignCreateResult struct {
	Provider aiProviderSummary             `json:"provider"`
	Intent   intentplanner.Request         `json:"intent"`
	Plan     intentplanner.PlanResult      `json:"plan"`
	Workflow designworkflow.WorkflowResult `json:"workflow,omitempty"`
	AIStatus *aiLaneStatus                 `json:"ai_status,omitempty"`
}

type aiProviderSummary struct {
	Name       string `json:"name"`
	Model      string `json:"model,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	Recorded   bool   `json:"recorded,omitempty"`
}

type aiRequestEvidence struct {
	Schema       string `json:"schema"`
	Provider     string `json:"provider"`
	Model        string `json:"model,omitempty"`
	PromptSource string `json:"prompt_source"`
	PromptHash   string `json:"prompt_hash"`
	Attempt      int    `json:"attempt"`
	MaxAttempts  int    `json:"max_attempts"`
	Background   bool   `json:"background,omitempty"`
}

type aiResponseEvidence struct {
	Schema       string                `json:"schema"`
	Provider     string                `json:"provider"`
	Model        string                `json:"model,omitempty"`
	ResponseID   string                `json:"response_id,omitempty"`
	Intent       intentplanner.Request `json:"intent"`
	IntentHash   string                `json:"intent_hash"`
	Usage        aiprovider.Usage      `json:"usage,omitempty"`
	FinishReason string                `json:"finish_reason,omitempty"`
	Recorded     bool                  `json:"recorded,omitempty"`
}

type aiAttemptEvidence struct {
	Attempt     int                     `json:"attempt"`
	Provider    string                  `json:"provider"`
	Model       string                  `json:"model,omitempty"`
	ResponseID  string                  `json:"response_id,omitempty"`
	Status      string                  `json:"status"`
	Diagnostics []aiprovider.Diagnostic `json:"diagnostics,omitempty"`
	IntentHash  string                  `json:"intent_hash,omitempty"`
}

type aiAttemptsEvidence struct {
	Schema   string              `json:"schema"`
	Attempts []aiAttemptEvidence `json:"attempts"`
}

func hasAIPromptSource(opts cliOptions) bool {
	return strings.TrimSpace(opts.aiPrompt) != "" || strings.TrimSpace(opts.aiPromptFile) != ""
}

func aiDesignOptionsPresent(opts cliOptions) bool {
	return hasAIPromptSource(opts) || strings.TrimSpace(opts.aiProvider) != "" || strings.TrimSpace(opts.aiModel) != "" || strings.TrimSpace(opts.aiProviderRecord) != "" || opts.aiBackground || opts.maxAIAttempts != 1
}

func runAIDesignCreate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if issue := validateAIDesignOptions(opts); issue != nil {
		return writeDesignFailure(stdout, *issue)
	}
	prompt, promptSource, issue := loadAIPrompt(opts)
	if issue != nil {
		return writeDesignFailure(stdout, *issue)
	}
	provider, err := aiProviderFromOptions(opts)
	if err != nil {
		return writeDesignFailure(stdout, aiProviderIssue(err))
	}
	providerResult, intent, plan, attempts, issues, err := generateValidatedAIIntent(ctx, provider, prompt, opts.maxAIAttempts)
	if err != nil {
		return writeDesignFailure(stdout, aiProviderIssue(err))
	}
	if reports.HasBlockingIssue(issues) || plan.GeneratedRequest == nil || plan.Status == intentplanner.PlanStatusBlocked || plan.Status == intentplanner.PlanStatusNeedsClarification {
		return writeAIDesignPreflightFailure(stdout, providerResult, intent, plan, issues)
	}
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()})
	}
	createOpts, err := designCreateOptions(ctx, opts, checkOpts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "mode", Message: err.Error()})
	}
	request := prepareAIWorkflowRequest(*plan.GeneratedRequest)
	plan.GeneratedRequest = &request
	workflow := designworkflow.Create(ctx, request, createOpts)
	promotion := designworkflow.BuildInternalPromotionReport(designPromotionFixture(opts, request, workflow), workflow)
	workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(promotion, designworkflow.PromotionReportArtifactPath))
	promotionArtifact, promotionIssue := designworkflow.WritePromotionReportArtifact(opts.output, promotion, opts.overwrite)
	var artifactIssues []reports.Issue
	var artifacts []reports.Artifact
	if promotionIssue != nil {
		artifactIssues = append(artifactIssues, *promotionIssue)
	} else if strings.TrimSpace(promotionArtifact.Path) != "" {
		artifacts = append(artifacts, promotionArtifact)
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	plan, planIssues := intentplanner.WriteArtifacts(plan, intentplanner.ArtifactOptions{OutputDir: artifactDir, Overwrite: true})
	artifactIssues = append(artifactIssues, planIssues...)
	for _, artifact := range plan.Artifacts {
		artifact.Path = filepath.ToSlash(filepath.Join(".kicadai", artifact.Path))
		artifacts = append(artifacts, artifact)
	}
	workflowIssues := writeWorkflowResultArtifact(artifactDir, workflow)
	artifactIssues = append(artifactIssues, workflowIssues...)
	if len(workflowIssues) == 0 {
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactValidationReport, Path: ".kicadai/workflow-result.json", Description: "AI design workflow result"})
	}
	providerArtifacts, providerArtifactIssues := writeAIProviderArtifacts(artifactDir, opts, promptSource, prompt, intent, providerResult, attempts)
	artifacts = append(artifacts, providerArtifacts...)
	artifactIssues = append(artifactIssues, providerArtifactIssues...)
	allIssues := append([]reports.Issue(nil), issues...)
	allIssues = append(allIssues, artifactIssues...)
	allIssues = append(allIssues, designworkflow.WorkflowIssues(workflow)...)
	status := buildAILaneStatus(plan, &workflow, allIssues, artifacts)
	aiArtifacts, aiArtifactIssues := writeAILaneArtifacts(opts.output, plan, nil, prompt, status, artifacts)
	artifacts = append(artifacts, aiArtifacts...)
	allIssues = append(allIssues, aiArtifactIssues...)
	status.ArtifactPaths = artifactPaths(artifacts)
	data := aiDesignCreateResult{
		Provider: aiProviderSummary{Name: providerResult.Provider, Model: providerResult.Model, ResponseID: providerResult.ResponseID, Recorded: providerResult.Recorded},
		Intent:   intent,
		Plan:     plan,
		Workflow: workflow,
		AIStatus: &status,
	}
	result := reports.Result{
		OK:        designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved) && !reports.HasBlockingIssue(allIssues),
		Command:   "design",
		Version:   reports.Version,
		Data:      data,
		Issues:    allIssues,
		Artifacts: artifacts,
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
		return errors.New("AI design create reported blocking issues")
	}
	return nil
}

func prepareAIWorkflowRequest(request designworkflow.Request) designworkflow.Request {
	for index := range request.Blocks {
		if request.Blocks[index].Params == nil {
			request.Blocks[index].Params = map[string]any{}
		}
		switch request.Blocks[index].BlockID {
		case "i2c_sensor":
			if componentID, ok := request.Blocks[index].Params["sensor_component_id"].(string); ok && strings.TrimSpace(componentID) != "" {
				request.Blocks[index].Params["fixed_pcb_layout"] = true
			}
		case "connector_breakout":
			request.Blocks[index].Params["edge_facing"] = true
		}
	}
	if request.Validation.SkipRouting {
		return request
	}
	request.RoutingRetry.Enabled = true
	if request.RoutingRetry.MaxAttempts < 2 {
		request.RoutingRetry.MaxAttempts = 2
	}
	request.RoutingRetry.StopOnRepeatedSignature = true
	request.RoutingRetry.StopOnNonImprovement = true
	return request
}

func generateValidatedAIIntent(ctx context.Context, provider aiprovider.Provider, prompt string, maxAttempts int) (aiprovider.GenerateResult, intentplanner.Request, intentplanner.PlanResult, []aiAttemptEvidence, []reports.Issue, error) {
	var diagnostics []aiprovider.Diagnostic
	var attempts []aiAttemptEvidence
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := provider.GenerateIntent(ctx, aiprovider.GenerateRequest{
			Prompt:            prompt,
			CapabilityContext: aiprovider.BMP280ReferenceCapabilityContext,
			OutputSchemaName:  aiReferenceSchemaName,
			OutputSchema:      aiprovider.BMP280ReferenceIntentEnvelopeSchema(),
			SchemaVersion:     aiprovider.EnvelopeSchemaV1,
			Attempt:           attempt,
			Diagnostics:       diagnostics,
		})
		if err != nil {
			attempts = append(attempts, aiAttemptEvidence{Attempt: attempt, Provider: provider.Name(), Status: "provider_error", Diagnostics: append([]aiprovider.Diagnostic(nil), diagnostics...)})
			if attempt < maxAttempts && aiProviderErrorRetryable(err) {
				diagnostics = []aiprovider.Diagnostic{{Code: string(aiprovider.ErrorCodeOf(err)), Path: "provider", Message: boundedDiagnosticMessage(err.Error())}}
				continue
			}
			return aiprovider.GenerateResult{}, intentplanner.Request{}, intentplanner.PlanResult{}, attempts, nil, err
		}
		intent, issues := aiprovider.DecodeIntent(result.IntentJSON)
		intent = intentplanner.NormalizeRequest(intent)
		var plan intentplanner.PlanResult
		if !reports.HasBlockingIssue(issues) {
			plan = intentplanner.Plan(intent)
			issues = append(issues, plan.Issues...)
		}
		intentData, marshalErr := json.Marshal(intent)
		if marshalErr != nil {
			return result, intent, plan, attempts, issues, fmt.Errorf("encode normalized AI intent: %w", marshalErr)
		}
		status := "completed"
		if reports.HasBlockingIssue(issues) || plan.GeneratedRequest == nil || plan.Status == intentplanner.PlanStatusBlocked || plan.Status == intentplanner.PlanStatusNeedsClarification {
			status = "invalid"
		}
		attempts = append(attempts, aiAttemptEvidence{
			Attempt:     attempt,
			Provider:    result.Provider,
			Model:       result.Model,
			ResponseID:  result.ResponseID,
			Status:      status,
			Diagnostics: append([]aiprovider.Diagnostic(nil), diagnostics...),
			IntentHash:  hashBytes(intentData),
		})
		if status == "completed" {
			return result, intent, plan, attempts, issues, nil
		}
		retryDiagnostics := aiRetryDiagnostics(issues)
		if attempt >= maxAttempts || len(retryDiagnostics) == 0 {
			return result, intent, plan, attempts, issues, nil
		}
		diagnostics = retryDiagnostics
	}
	return aiprovider.GenerateResult{}, intentplanner.Request{}, intentplanner.PlanResult{}, attempts, nil, &aiprovider.ProviderError{Code: aiprovider.ErrorIncomplete, Message: "AI intent attempts exhausted"}
}

func aiProviderErrorRetryable(err error) bool {
	switch aiprovider.ErrorCodeOf(err) {
	case aiprovider.ErrorMalformed, aiprovider.ErrorSchema:
		return true
	default:
		return false
	}
}

func aiRetryDiagnostics(issues []reports.Issue) []aiprovider.Diagnostic {
	diagnostics := make([]aiprovider.Diagnostic, 0, aiprovider.MaxDiagnostics)
	for _, issue := range issues {
		if !issue.Blocking() || !strings.HasPrefix(issue.Path, "provider.intent") {
			continue
		}
		code := "ai_intent_field_invalid"
		if issue.Path == "provider.intent" {
			code = "ai_output_schema_invalid"
		}
		diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: code, Path: issue.Path, Message: boundedDiagnosticMessage(issue.Message)})
		if len(diagnostics) == aiprovider.MaxDiagnostics {
			break
		}
	}
	return diagnostics
}

func boundedDiagnosticMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= aiprovider.MaxDiagnosticLen {
		return message
	}
	return message[:aiprovider.MaxDiagnosticLen]
}

func validateAIDesignOptions(opts cliOptions) *reports.Issue {
	if strings.TrimSpace(opts.output) == "" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "--output is required"}
	}
	sources := 0
	for _, value := range []string{opts.aiPrompt, opts.aiPromptFile} {
		if strings.TrimSpace(value) != "" {
			sources++
		}
	}
	if sources != 1 {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "prompt", Message: "use exactly one of --prompt or --prompt-file"}
	}
	if strings.TrimSpace(opts.requestPath) != "" || strings.TrimSpace(opts.intentText) != "" || strings.TrimSpace(opts.intentFile) != "" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "prompt", Message: "--prompt/--prompt-file cannot be combined with --request, --text, or --file"}
	}
	if strings.TrimSpace(opts.aiProvider) == "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider", Message: "--provider is required with an AI prompt"}
	}
	if opts.maxAIAttempts < 1 || opts.maxAIAttempts > 2 {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "max_ai_attempts", Message: "--max-ai-attempts must be 1 or 2"}
	}
	if strings.EqualFold(strings.TrimSpace(opts.aiProvider), "recorded") && strings.TrimSpace(opts.aiProviderRecord) == "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider_record", Message: "--provider-record is required for the recorded provider"}
	}
	if !strings.EqualFold(strings.TrimSpace(opts.aiProvider), "recorded") && strings.TrimSpace(opts.aiProviderRecord) != "" {
		return &reports.Issue{Code: reports.CodeAIProviderConfiguration, Severity: reports.SeverityError, Path: "provider_record", Message: "--provider-record is only valid with --provider recorded"}
	}
	return nil
}

func loadAIPrompt(opts cliOptions) (string, string, *reports.Issue) {
	if strings.TrimSpace(opts.aiPrompt) != "" {
		prompt := strings.TrimSpace(opts.aiPrompt)
		if len(prompt) > aiprovider.MaxPromptBytes {
			return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "prompt", Message: fmt.Sprintf("prompt exceeds %d-byte limit", aiprovider.MaxPromptBytes)}
		}
		return prompt, "argument", nil
	}
	data, err := readBoundedFile(opts.aiPromptFile, aiprovider.MaxPromptBytes)
	if err != nil {
		code := reports.CodeInvalidArgument
		if os.IsNotExist(err) {
			code = reports.CodeMissingFile
		}
		return "", "", &reports.Issue{Code: code, Severity: reports.SeverityError, Path: "prompt_file", Message: err.Error()}
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "prompt_file", Message: "prompt file is empty"}
	}
	return prompt, "file", nil
}

func aiProviderFromOptions(opts cliOptions) (aiprovider.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(opts.aiProvider)) {
	case "openai":
		options := aiprovider.OpenAIOptionsFromEnvironment()
		if strings.TrimSpace(opts.aiModel) != "" {
			options.Model = strings.TrimSpace(opts.aiModel)
		}
		if opts.aiBackground {
			options.Background = true
		}
		return aiprovider.NewOpenAIProvider(options)
	case "recorded":
		data, err := readBoundedFile(opts.aiProviderRecord, aiprovider.MaxResponseBytes)
		if err != nil {
			return nil, &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: "read recorded AI response: " + err.Error()}
		}
		return aiprovider.NewRecordedProvider(filepath.Base(opts.aiProviderRecord), data)
	default:
		return nil, &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: fmt.Sprintf("unsupported AI provider %q", opts.aiProvider)}
	}
}

func readBoundedFile(path string, limit int) ([]byte, error) {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s exceeds %d-byte limit", filepath.Base(path), limit)
	}
	return data, nil
}

func aiProviderIssue(err error) reports.Issue {
	code := reports.CodeAIProviderTransport
	switch aiprovider.ErrorCodeOf(err) {
	case aiprovider.ErrorConfiguration:
		code = reports.CodeAIProviderConfiguration
	case aiprovider.ErrorAuthentication:
		code = reports.CodeAIProviderAuthentication
	case aiprovider.ErrorRateLimit:
		code = reports.CodeAIProviderRateLimit
	case aiprovider.ErrorTimeout:
		code = reports.CodeAIProviderTimeout
	case aiprovider.ErrorRefusal:
		code = reports.CodeAIProviderRefusal
	case aiprovider.ErrorIncomplete:
		code = reports.CodeAIProviderIncomplete
	case aiprovider.ErrorMalformed, aiprovider.ErrorSchema:
		code = reports.CodeAIOutputInvalid
	}
	return reports.Issue{Code: code, Severity: reports.SeverityError, Path: "provider", Message: err.Error()}
}

func writeAIDesignPreflightFailure(stdout io.Writer, providerResult aiprovider.GenerateResult, intent intentplanner.Request, plan intentplanner.PlanResult, issues []reports.Issue) error {
	data := aiDesignCreateResult{
		Provider: aiProviderSummary{Name: providerResult.Provider, Model: providerResult.Model, ResponseID: providerResult.ResponseID, Recorded: providerResult.Recorded},
		Intent:   intent,
		Plan:     plan,
	}
	result := reports.ResultWithIssues("design", data, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	return errors.New("AI design provider output failed validation")
}

func writeAIProviderArtifacts(artifactDir string, opts cliOptions, promptSource string, prompt string, intent intentplanner.Request, result aiprovider.GenerateResult, attempts []aiAttemptEvidence) ([]reports.Artifact, []reports.Issue) {
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: ".kicadai", Message: err.Error()}}
	}
	promptHash := hashText(prompt)
	intentData, err := json.Marshal(intent)
	if err != nil {
		return nil, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: ".kicadai/ai-response.json", Message: "encode normalized AI intent: " + err.Error()}}
	}
	intentHash := hashBytes(intentData)
	requestEvidence := aiRequestEvidence{
		Schema:       aiprovider.EnvelopeSchemaV1,
		Provider:     strings.ToLower(strings.TrimSpace(opts.aiProvider)),
		Model:        result.Model,
		PromptSource: promptSource,
		PromptHash:   promptHash,
		Attempt:      len(attempts),
		MaxAttempts:  opts.maxAIAttempts,
		Background:   result.Background,
	}
	responseEvidence := aiResponseEvidence{
		Schema:       aiprovider.EnvelopeSchemaV1,
		Provider:     result.Provider,
		Model:        result.Model,
		ResponseID:   result.ResponseID,
		Intent:       intent,
		IntentHash:   intentHash,
		Usage:        result.Usage,
		FinishReason: result.FinishReason,
		Recorded:     result.Recorded,
	}
	attemptHistory := aiAttemptsEvidence{
		Schema:   "kicadai.ai.attempts.v1",
		Attempts: append([]aiAttemptEvidence(nil), attempts...),
	}
	files := []struct {
		name        string
		value       any
		description string
	}{
		{name: "ai-request.json", value: requestEvidence, description: "sanitized AI provider request evidence"},
		{name: "ai-response.json", value: responseEvidence, description: "validated AI provider response evidence"},
		{name: "ai-attempts.json", value: attemptHistory, description: "bounded AI provider attempt history"},
	}
	var artifacts []reports.Artifact
	var issues []reports.Issue
	for _, file := range files {
		relPath := filepath.ToSlash(filepath.Join(".kicadai", file.name))
		artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, file.name), file.value, reports.ArtifactValidationReport, relPath, file.description)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, issues
}

func hashText(value string) string {
	return hashBytes([]byte(value))
}

func hashBytes(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
