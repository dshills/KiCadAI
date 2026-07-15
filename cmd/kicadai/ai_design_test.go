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
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

func TestParseAIDesignFlags(t *testing.T) {
	opts, command, err := parse([]string{
		"--prompt", "build bmp280",
		"--provider", "recorded",
		"--provider-record", "response.json",
		"--ai-profile", "generic-circuit-v1",
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
	if opts.aiPrompt != "build bmp280" || opts.aiProvider != "recorded" || opts.aiProviderRecord != "response.json" || opts.aiProfile != "generic-circuit-v1" || opts.aiModel != "test-model" || opts.maxAIAttempts != 2 || !opts.aiBackground {
		t.Fatalf("options = %#v", opts)
	}
}

func TestRecordedGenericCircuitProviderToWorkflow(t *testing.T) {
	fixture := filepath.Join("..", "..", "examples", "ai", "generic_parallel_resistors", "recorded-response.json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := aiprovider.NewRecordedProvider("generic_parallel_resistors", data)
	if err != nil {
		t.Fatal(err)
	}
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		t.Fatal(err)
	}
	index := genericResistorLibraryIndex()
	symbols, footprints := circuitgraph.LibraryEvidenceFromIndex(index)
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "test", LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: true})
	capability, err := circuitgraph.ProviderCapabilityContext(catalog, aiprovider.MaxCapabilityBytes)
	if err != nil {
		t.Fatal(err)
	}
	result, graph, resolved, request, attempts, issues, err := generateValidatedAIGraph(context.Background(), provider, aiprovider.GenericCircuitProfile(capability), resolver, "build two parallel resistors", 1)
	if err != nil || reports.HasBlockingIssue(issues) {
		t.Fatalf("generic preflight err=%v issues=%#v", err, issues)
	}
	if !result.Recorded || graph.Project.Name != "generic_parallel_resistors" || resolved.ResolutionHash == "" || request.ExplicitCircuit == nil || len(attempts) != 1 {
		t.Fatalf("generic preflight result=%#v graph=%#v resolved=%#v request=%#v attempts=%#v", result, graph, resolved, request, attempts)
	}
	request.Validation.Acceptance = designworkflow.AcceptanceDraft
	request.Validation.RequireERC = false
	request.Validation.RequireDRC = false
	workflow := designworkflow.Create(context.Background(), request, designworkflow.CreateOptions{OutputDir: filepath.Join(t.TempDir(), "project"), Overwrite: true, LibraryIndex: &index})
	if stage := testAIWorkflowStage(workflow, designworkflow.StageProjectWrite); stage == nil || stage.Status == designworkflow.StageStatusBlocked {
		t.Fatalf("project write stage=%#v issues=%#v", stage, designworkflow.WorkflowIssues(workflow))
	}
	if stage := testAIWorkflowStage(workflow, designworkflow.StageRouting); stage == nil || stage.Status == designworkflow.StageStatusBlocked || stage.Status == designworkflow.StageStatusSkipped {
		t.Fatalf("routing stage=%#v issues=%#v", stage, designworkflow.WorkflowIssues(workflow))
	}
}

func TestRunRecordedGenericCircuitCLIEndToEnd(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "examples", "ai", "generic_parallel_resistors")
	symbolsRoot, footprintsRoot := writeCLILibraryFixture(t)
	catalogDir, err := filepath.Abs(filepath.Join("..", "..", "data", "components"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	err = run([]string{
		"--prompt-file", filepath.Join(fixtureDir, "prompt.txt"),
		"--provider", "recorded", "--provider-record", filepath.Join(fixtureDir, "recorded-response.json"),
		"--ai-profile", "generic-circuit-v1", "--catalog-dir", catalogDir,
		"--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
		"--output", output, "--overwrite", "design", "create",
	}, &stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("generic CLI run: %v\n%s", err, stdout.String())
	}
	var payload struct {
		OK   bool                      `json:"ok"`
		Data aiGraphDesignCreateResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Data.Graph.Project.Name != "generic_parallel_resistors" || payload.Data.Request.ExplicitCircuit == nil || payload.Data.AIStatus == nil || payload.Data.AIStatus.Status != aiLaneStatusCandidate {
		t.Fatalf("generic CLI payload = %#v", payload)
	}
	for _, name := range []string{"circuit-graph.json", "circuit-resolution.json", "design-request.json", "ai-request.json", "ai-response.json", "ai-attempts.json", "workflow-result.json", "autonomous-correction.json"} {
		if _, err := os.Stat(filepath.Join(output, ".kicadai", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	var correction designworkflow.AutonomousCorrectionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "autonomous-correction.json"), &correction)
	if correction.SchemaVersion != designworkflow.AutonomousCorrectionSchemaV1 || correction.Scope != "generic-circuit-v1" || !correction.Enabled || correction.MaxAttempts != 3 {
		t.Fatalf("autonomous correction policy evidence = %#v", correction)
	}
	if correction.Attempts != 1 || correction.Applied != 0 || correction.StopReason != "routed" || correction.SelectedAttempt != 1 || !correction.ProtectedInvariantsPreserved || !correction.AllAttemptInvariantsPreserved {
		t.Fatalf("autonomous correction result evidence = %#v", correction)
	}
}

func TestAIDesignRejectsUnknownExplicitProfile(t *testing.T) {
	issue := validateAIDesignOptions(cliOptions{output: "out", aiPrompt: "test", aiProvider: "recorded", aiProviderRecord: "response.json", aiProfile: "unknown", maxAIAttempts: 1})
	if issue == nil || issue.Path != "ai_profile" {
		t.Fatalf("unknown profile issue = %#v", issue)
	}
}

func TestGenericCircuitProfileRejectsUnknownFieldBeforeWorkflow(t *testing.T) {
	data := []byte(`{"schema":"kicadai.ai.intent.v1","intent":{"schema":"kicadai.circuit-graph.v1","version":1,"unexpected":true}}`)
	provider, err := aiprovider.NewRecordedProvider("invalid", data)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, request, _, issues, err := generateValidatedAIGraph(context.Background(), provider, aiprovider.GenericCircuitProfile("catalog"), circuitgraph.NewResolver(circuitgraph.ResolveOptions{}), "invalid graph", 1)
	if err != nil || !reports.HasBlockingIssue(issues) || request.ExplicitCircuit != nil || !strings.HasPrefix(issues[0].Path, "provider.graph") {
		t.Fatalf("invalid graph err=%v issues=%#v request=%#v", err, issues, request)
	}
}

func genericResistorLibraryIndex() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {LibraryID: "Device:R", Name: "R", Pins: []libraryresolver.SymbolPin{
				{Number: "1", Unit: 1, Position: kicadfiles.Point{X: -2540000}, Electrical: "passive"},
				{Number: "2", Unit: 1, Position: kicadfiles.Point{X: 2540000}, Electrical: "passive"},
			}},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {
				FootprintID: "Resistor_SMD:R_0805_2012Metric", Name: "R_0805_2012Metric",
				BoundingBox:  libraryresolver.BoundingBox{Min: kicadfiles.Point{X: -1500000, Y: -1000000}, Max: kicadfiles.Point{X: 1500000, Y: 1000000}},
				CourtyardBox: libraryresolver.BoundingBox{Min: kicadfiles.Point{X: -1600000, Y: -1100000}, Max: kicadfiles.Point{X: 1600000, Y: 1100000}},
				Pads: []libraryresolver.FootprintPad{
					{Name: "1", Type: "smd", Shape: "rect", Position: kicadfiles.Point{X: -950000}, Size: kicadfiles.Point{X: 1000000, Y: 1200000}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}},
					{Name: "2", Type: "smd", Shape: "rect", Position: kicadfiles.Point{X: 950000}, Size: kicadfiles.Point{X: 1000000, Y: 1200000}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}},
				},
			},
		},
	}
}

func testAIWorkflowStage(result designworkflow.WorkflowResult, name designworkflow.StageName) *designworkflow.StageResult {
	for index := range result.Stages {
		if result.Stages[index].Name == name {
			return &result.Stages[index]
		}
	}
	return nil
}

func TestRunAIDesignRejectsConflictingInputsBeforeOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "must-not-exist")
	var stdout bytes.Buffer
	err := run([]string{
		"--prompt", "build a USB-C BMP280 breakout",
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
		"--prompt", "build a USB-C BMP280 breakout",
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

func TestRunAIDesignMalformedRecordPreservesExistingOutput(t *testing.T) {
	root := t.TempDir()
	record := filepath.Join(root, "bad-response.json")
	if err := os.WriteFile(record, []byte(`{"schema":"wrong","intent":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "existing-project")
	files := map[string][]byte{
		"existing-project.kicad_pro":               []byte("project-before\n"),
		"existing-project.kicad_sch":               []byte("schematic-before\n"),
		"existing-project.kicad_pcb":               []byte("pcb-before\n"),
		filepath.Join(".kicadai", "manifest.json"): []byte("manifest-before\n"),
	}
	for name, data := range files {
		path := filepath.Join(output, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	err := run([]string{
		"--prompt", "build a split-supply amplifier",
		"--provider", "recorded",
		"--provider-record", record,
		"--output", output,
		"--overwrite",
		"design", "create",
	}, &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected provider validation failure")
	}
	for name, want := range files {
		got, readErr := os.ReadFile(filepath.Join(output, name))
		if readErr != nil {
			t.Fatalf("read preserved %s: %v", name, readErr)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s changed after failed overwrite: got %q want %q", name, got, want)
		}
	}
}

func TestRunAIDesignRejectsUnsupportedProfilesBeforeProviderOrOutput(t *testing.T) {
	for _, prompt := range []string{
		"Create a USB-C motor controller",
		"Create a USB-C BMP280 breakout with an LED indicator",
	} {
		t.Run(prompt, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "must-not-exist")
			var stdout bytes.Buffer
			err := run([]string{
				"--prompt", prompt,
				"--provider", "recorded",
				"--provider-record", filepath.Join(t.TempDir(), "missing-provider-response.json"),
				"--output", output,
				"design", "create",
			}, &stdout, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected profile selection failure")
			}
			if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
				t.Fatalf("output should not exist, stat error=%v", statErr)
			}
			var result reports.Result
			if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
				t.Fatalf("decode result: %v; stdout=%s", decodeErr, stdout.String())
			}
			if result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "prompt" || result.Issues[0].Code != reports.CodeInvalidArgument {
				t.Fatalf("result = %#v", result)
			}
		})
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

func TestRunAIDesignRecordedProtectedLEDEndToEnd(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "examples", "ai", "usb_c_led_indicator_protected")
	var generated [][]byte
	for runIndex := 0; runIndex < 2; runIndex++ {
		output := filepath.Join(t.TempDir(), "project")
		var stdout bytes.Buffer
		err := run([]string{
			"--prompt-file", filepath.Join(fixtureDir, "prompt.txt"),
			"--provider", "recorded",
			"--provider-record", filepath.Join(fixtureDir, "recorded-response.json"),
			"--output", output,
			"--overwrite",
			"design", "create",
		}, &stdout, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("recorded protected LED run %d: %v\n%s", runIndex+1, err, stdout.String())
		}
		var payload struct {
			Data aiDesignCreateResult `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Data.AIStatus == nil || payload.Data.AIStatus.Status != aiLaneStatusCandidate || payload.Data.Plan.Status != intentplanner.PlanStatusReady || payload.Data.Plan.GeneratedRequest == nil {
			t.Fatalf("AI result = %#v", payload.Data)
		}
		request := payload.Data.Plan.GeneratedRequest
		usb := designRequestBlock(*request, "usb_power")
		indicator := designRequestBlock(*request, "indicator")
		if usb.BlockID != "usb_c_power" || usb.Params["include_fuse"] != true || usb.Params["include_tvs"] != true || usb.Params["include_bulk_capacitor"] != true {
			t.Fatalf("USB block = %#v", usb)
		}
		resistor, resistorOK := indicator.Params["resistor_value"].(string)
		current, currentOK := indicator.Params["led_current"].(string)
		if indicator.BlockID != "led_indicator" || !resistorOK || resistor != "600" || !currentOK || current != "5mA" {
			t.Fatalf("indicator block = %#v", indicator)
		}
		if _, exists := indicator.Params["led_current_ma"]; exists {
			t.Fatalf("calculation-only LED current leaked into workflow block: %#v", indicator.Params)
		}
		data, err := os.ReadFile(filepath.Join(output, ".kicadai", "generated-request.json"))
		if err != nil {
			t.Fatal(err)
		}
		generated = append(generated, data)
	}
	if !bytes.Equal(generated[0], generated[1]) {
		t.Fatal("recorded protected LED generated request is not deterministic")
	}
}

func designRequestBlock(request designworkflow.Request, id string) designworkflow.BlockInstanceSpec {
	for _, block := range request.Blocks {
		if block.ID == id {
			return block
		}
	}
	return designworkflow.BlockInstanceSpec{}
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
	result, _, plan, attempts, issues, err := generateValidatedAIIntent(context.Background(), provider, aiprovider.BMP280Profile(), "build bmp280", 2)
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
	_, _, _, attempts, _, err := generateValidatedAIIntent(context.Background(), provider, aiprovider.BMP280Profile(), "build bmp280", 2)
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

func TestAILaneStatusUsesAuthoritativePromotionPass(t *testing.T) {
	status := aiLaneStatus{Status: aiLaneStatusCandidate, Message: "warning-level evidence"}
	got := aiLaneStatusWithPromotionEvidence(status, designworkflow.PromotionReport{Status: designworkflow.PromotionStatusPass})
	if got.Status != aiLaneStatusReady || got.Stage != "validation" {
		t.Fatalf("promoted AI status = %#v", got)
	}
	blocked := aiLaneStatus{Status: aiLaneStatusBlocked}
	if got := aiLaneStatusWithPromotionEvidence(blocked, designworkflow.PromotionReport{Status: designworkflow.PromotionStatusPass}); got.Status != aiLaneStatusBlocked {
		t.Fatalf("promotion overrode blocked status: %#v", got)
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
