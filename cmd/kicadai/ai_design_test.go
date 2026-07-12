package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

func TestParseAIDesignFlags(t *testing.T) {
	opts, command, err := parse([]string{
		"--prompt", "build bmp280",
		"--provider", "recorded",
		"--provider-record", "response.json",
		"--model", "test-model",
		"--max-ai-attempts", "2",
		"--ai-background",
		"design", "create",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if command != "design" || len(opts.commandArgs) != 1 || opts.commandArgs[0] != "create" {
		t.Fatalf("command=%q args=%#v", command, opts.commandArgs)
	}
	if opts.aiPrompt != "build bmp280" || opts.aiProvider != "recorded" || opts.aiProviderRecord != "response.json" || opts.aiModel != "test-model" || opts.maxAIAttempts != 2 || !opts.aiBackground {
		t.Fatalf("options = %#v", opts)
	}
}

func TestRunAIDesignRejectsConflictingInputsBeforeOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "must-not-exist")
	var stdout bytes.Buffer
	err := run([]string{
		"--prompt", "build bmp280",
		"--request", "request.json",
		"--provider", "recorded",
		"--provider-record", filepath.Join("..", "..", "examples", "ai", "usb_c_bmp280_breakout", "recorded-response.json"),
		"--output", output,
		"design", "create",
	}, &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected input conflict")
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("output should not exist, stat error=%v", statErr)
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil || result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "prompt" {
		t.Fatalf("result=%#v decode=%v stdout=%s", result, decodeErr, stdout.String())
	}
}

func TestRunAIDesignMalformedRecordCreatesNoOutput(t *testing.T) {
	root := t.TempDir()
	record := filepath.Join(root, "bad-response.json")
	if err := os.WriteFile(record, []byte(`{"schema":"wrong","intent":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "must-not-exist")
	var stdout bytes.Buffer
	err := run([]string{
		"--prompt", "build bmp280",
		"--provider", "recorded",
		"--provider-record", record,
		"--output", output,
		"design", "create",
	}, &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected provider validation failure")
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("output should not exist, stat error=%v", statErr)
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil || result.OK || len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeAIOutputInvalid {
		t.Fatalf("result=%#v decode=%v stdout=%s", result, decodeErr, stdout.String())
	}
}

func TestRunAIDesignRecordedReferencePersistsSanitizedEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "project")
	promptPath := filepath.Join("..", "..", "examples", "ai", "usb_c_bmp280_breakout", "prompt.txt")
	recordPath := filepath.Join("..", "..", "examples", "ai", "usb_c_bmp280_breakout", "recorded-response.json")
	var stdout bytes.Buffer
	err := run([]string{
		"--prompt-file", promptPath,
		"--provider", "recorded",
		"--provider-record", recordPath,
		"--output", output,
		"--overwrite",
		"design", "create",
	}, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("recorded AI design create: %v", err)
	}
	var payload struct {
		Data aiDesignCreateResult `json:"data"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode result: %v", decodeErr)
	}
	if payload.Data.Provider.Name != "recorded" || !payload.Data.Provider.Recorded || payload.Data.Intent.Name != "usb_c_bmp280_breakout" {
		t.Fatalf("provider/intent = %#v / %#v", payload.Data.Provider, payload.Data.Intent)
	}
	if payload.Data.AIStatus == nil || payload.Data.AIStatus.Status != "candidate" {
		t.Fatalf("AI status = %#v", payload.Data.AIStatus)
	}
	for _, name := range []string{
		"ai-request.json",
		"ai-response.json",
		"ai-attempts.json",
		"intent-plan.json",
		"generated-request.json",
		"workflow-result.json",
		"design-promotion.json",
		"validation-summary.json",
		"manifest.json",
	} {
		if _, statErr := os.Stat(filepath.Join(output, ".kicadai", name)); statErr != nil {
			t.Fatalf("missing %s: %v", name, statErr)
		}
	}
	requestEvidence, readErr := os.ReadFile(filepath.Join(output, ".kicadai", "ai-request.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, forbidden := range []string{"protected USB-C", "OPENAI_API_KEY", "Authorization", "Bearer"} {
		if strings.Contains(string(requestEvidence), forbidden) {
			t.Fatalf("request evidence contains %q: %s", forbidden, requestEvidence)
		}
	}
	if _, statErr := os.Stat(filepath.Join(output, ".kicadai", "intent-source.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("provider lane must not persist plaintext prompt, stat=%v", statErr)
	}
	var response aiResponseEvidence
	readJSONFile(t, filepath.Join(output, ".kicadai", "ai-response.json"), &response)
	if response.Intent.Functions[0].Params["sensor_component_id"] != "sensor.bosch.bmp280.lga8" || response.IntentHash == "" {
		t.Fatalf("response evidence = %#v", response)
	}
}

func TestAIProviderIssueMapping(t *testing.T) {
	tests := []struct {
		providerCode aiprovider.ErrorCode
		reportCode   reports.Code
	}{
		{providerCode: aiprovider.ErrorConfiguration, reportCode: reports.CodeAIProviderConfiguration},
		{providerCode: aiprovider.ErrorAuthentication, reportCode: reports.CodeAIProviderAuthentication},
		{providerCode: aiprovider.ErrorRateLimit, reportCode: reports.CodeAIProviderRateLimit},
		{providerCode: aiprovider.ErrorTimeout, reportCode: reports.CodeAIProviderTimeout},
		{providerCode: aiprovider.ErrorRefusal, reportCode: reports.CodeAIProviderRefusal},
		{providerCode: aiprovider.ErrorIncomplete, reportCode: reports.CodeAIProviderIncomplete},
	}
	for _, test := range tests {
		err := &aiprovider.ProviderError{Code: test.providerCode, Message: "failed"}
		if issue := aiProviderIssue(err); issue.Code != test.reportCode {
			t.Fatalf("provider code %q mapped to %q, want %q", test.providerCode, issue.Code, test.reportCode)
		}
	}
}

func TestGenerateValidatedAIIntentRetriesSchemaFailureOnce(t *testing.T) {
	recordedData, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "usb_c_bmp280_breakout", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	validIntent, err := aiprovider.DecodeEnvelope(recordedData)
	if err != nil {
		t.Fatal(err)
	}
	provider := &sequenceAIProvider{results: []aiprovider.GenerateResult{
		{Provider: "sequence", Model: "test", ResponseID: "first", IntentJSON: json.RawMessage(`{"version":"0.1.0","name":"bad","unknown":true}`)},
		{Provider: "sequence", Model: "test", ResponseID: "second", IntentJSON: validIntent},
	}}
	result, _, plan, attempts, issues, err := generateValidatedAIIntent(context.Background(), provider, "build bmp280", 2)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.ResponseID != "second" || plan.Status != intentplanner.PlanStatusReady || reports.HasBlockingIssue(issues) {
		t.Fatalf("result=%#v plan=%s issues=%#v", result, plan.Status, issues)
	}
	if len(attempts) != 2 || attempts[0].Status != "invalid" || attempts[1].Status != "completed" {
		t.Fatalf("attempts = %#v", attempts)
	}
	if len(provider.requests) != 2 || len(provider.requests[1].Diagnostics) == 0 || provider.requests[1].Diagnostics[0].Code != "ai_output_schema_invalid" {
		t.Fatalf("requests = %#v", provider.requests)
	}
}

func TestGenerateValidatedAIIntentDoesNotRetryAuthentication(t *testing.T) {
	provider := &sequenceAIProvider{errors: []error{
		&aiprovider.ProviderError{Code: aiprovider.ErrorAuthentication, Message: "authentication failed"},
	}}
	_, _, _, attempts, _, err := generateValidatedAIIntent(context.Background(), provider, "build bmp280", 2)
	if aiprovider.ErrorCodeOf(err) != aiprovider.ErrorAuthentication || len(provider.requests) != 1 || len(attempts) != 1 {
		t.Fatalf("err=%v requests=%d attempts=%#v", err, len(provider.requests), attempts)
	}
}

func TestPrepareAIWorkflowRequestEnablesBoundedPlacementRepair(t *testing.T) {
	original := designworkflow.Request{
		Blocks: []designworkflow.BlockInstanceSpec{
			{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"sensor_component_id": "sensor.bosch.bmp280.lga8"}},
			{ID: "io", BlockID: "connector_breakout"},
		},
		Connections: []designworkflow.ConnectionSpec{
			{From: "io.GND", To: "sensor.GND", NetAlias: "GND"},
			{From: "io.SDA", To: "sensor.SDA", NetAlias: "SDA"},
			{From: "io.SCL", To: "sensor.SCL", NetAlias: "SCL"},
			{From: "io.VCC", To: "sensor.VCC", NetAlias: "VCC_3v3"},
		},
	}
	request := prepareAIWorkflowRequest(original)
	if !request.RoutingRetry.Enabled || request.RoutingRetry.MaxAttempts != 2 {
		t.Fatalf("routing retry = %#v", request.RoutingRetry)
	}
	if request.RoutingRetry.StopOnNewBlockers || !request.RoutingRetry.StopOnRepeatedSignature || !request.RoutingRetry.StopOnNonImprovement {
		t.Fatalf("routing retry stop policy = %#v", request.RoutingRetry)
	}
	if !slices.Equal(request.Constraints.LocalRouteObstacleNets, []string{"GND", "SCL", "SDA", "VCC_3v3"}) {
		t.Fatalf("selective local-route obstacles = %#v", request.Constraints.LocalRouteObstacleNets)
	}
	if request.Blocks[0].Params["fixed_pcb_layout"] != true || request.Blocks[1].Params["edge_facing"] != true || request.Blocks[1].Params["edge_side"] != "bottom" {
		t.Fatalf("AI block placement params = %#v", request.Blocks)
	}
	if _, exists := original.Blocks[0].Params["fixed_pcb_layout"]; exists || original.Blocks[1].Params != nil {
		t.Fatalf("prepareAIWorkflowRequest mutated caller blocks: %#v", original.Blocks)
	}
	preserved := prepareAIWorkflowRequest(designworkflow.Request{Blocks: []designworkflow.BlockInstanceSpec{{ID: "io", BlockID: "connector_breakout", Params: map[string]any{"edge_side": "left"}}}})
	if preserved.Blocks[0].Params["edge_side"] != "left" {
		t.Fatalf("AI connector edge override = %#v, want left preserved", preserved.Blocks[0].Params["edge_side"])
	}

	skipped := prepareAIWorkflowRequest(designworkflow.Request{Validation: designworkflow.ValidationSpec{SkipRouting: true}})
	if skipped.RoutingRetry.Enabled {
		t.Fatalf("skip-routing request enabled retry: %#v", skipped.RoutingRetry)
	}
}

type sequenceAIProvider struct {
	results  []aiprovider.GenerateResult
	errors   []error
	requests []aiprovider.GenerateRequest
}

func (provider *sequenceAIProvider) Name() string { return "sequence" }

func (provider *sequenceAIProvider) GenerateIntent(_ context.Context, request aiprovider.GenerateRequest) (aiprovider.GenerateResult, error) {
	provider.requests = append(provider.requests, request)
	index := len(provider.requests) - 1
	if index < len(provider.errors) && provider.errors[index] != nil {
		return aiprovider.GenerateResult{}, provider.errors[index]
	}
	if index >= len(provider.results) {
		return aiprovider.GenerateResult{}, &aiprovider.ProviderError{Code: aiprovider.ErrorIncomplete, Message: "missing sequence result"}
	}
	return provider.results[index], nil
}

func TestLoadAIPromptRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, aiprovider.MaxPromptBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, issue := loadAIPrompt(cliOptions{aiPromptFile: path})
	if issue == nil || issue.Path != "prompt_file" {
		t.Fatalf("issue = %#v", issue)
	}
}
