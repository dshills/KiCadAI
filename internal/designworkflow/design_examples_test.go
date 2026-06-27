package designworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

const designExamplePlanningTimeout = 15 * time.Second

type designExampleMetadata struct {
	ID                string          `json:"id"`
	Request           string          `json:"request"`
	Tier              string          `json:"tier"`
	Readiness         string          `json:"readiness"`
	Acceptance        AcceptanceLevel `json:"acceptance"`
	RequiresERC       *bool           `json:"requires_erc"`
	RequiresDRC       *bool           `json:"requires_drc"`
	Allowlists        []string        `json:"allowlists,omitempty"`
	ExpectedArtifacts []string        `json:"expected_artifacts,omitempty"`
	ExpectedStages    []StageName     `json:"expected_stages"`
	KnownGaps         []string        `json:"known_gaps"`
	Notes             string          `json:"notes,omitempty"`
}

var designExampleMetadataTiers = map[string]struct{}{
	"smoke":             {},
	"block-composition": {},
	"routing":           {},
	"fabrication":       {},
}

var designExampleMetadataReadiness = map[string]struct{}{
	"candidate":     {},
	"pass":          {},
	"expected_fail": {},
	"blocked":       {},
}

var designExampleMetadataAcceptance = map[AcceptanceLevel]struct{}{
	AcceptanceDraft:                {},
	AcceptanceStructural:           {},
	AcceptanceConnectivity:         {},
	AcceptanceERCDRC:               {},
	AcceptanceFabricationCandidate: {},
}

func TestDesignExamplesPassBlockPlanning(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	for _, name := range designExampleRequestFiles(t, repoRoot) {
		t.Run(name, func(t *testing.T) {
			request, issues := loadDesignExampleRequest(t, repoRoot, name)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues: %#v", name, issues)
			}
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, request)
			if len(stage.Issues) != 0 {
				t.Fatalf("%s block planning issues:\n%s", name, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}

func TestDesignExampleValidationReportsContractDrift(t *testing.T) {
	tests := []struct {
		name        string
		request     Request
		wantPath    string
		wantMessage string
		wantIssues  int
	}{
		{
			name: "unknown LED parameter",
			request: Request{
				Version: RequestVersion,
				Name:    "invalid_led",
				Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
				Blocks: []BlockInstanceSpec{{
					ID:      "status",
					BlockID: "led_indicator",
					Params:  map[string]any{"resistor_ohms": 1000},
				}},
			},
			wantPath:    "blocks[0].params.resistor_ohms",
			wantMessage: "unknown parameter resistor_ohms",
			wantIssues:  1,
		},
		{
			name: "unknown connection endpoint",
			request: Request{
				Version: RequestVersion,
				Name:    "invalid_sensor",
				Board:   BoardSpec{WidthMM: 120, HeightMM: 90, Layers: 2},
				Blocks: []BlockInstanceSpec{
					{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48", "include_pullups": true}},
					{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"VCC", "GND", "SDA", "SCL", "INT"}}},
				},
				Connections: []ConnectionSpec{
					{From: "sensor.INT", To: "header.INT", NetAlias: "INT"},
				},
			},
			wantPath:    "connections[0].from",
			wantMessage: "unknown port sensor.INT",
			wantIssues:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, test.request)
			if len(stage.Issues) != test.wantIssues {
				t.Fatalf("issue count = %d, want %d:\n%s", len(stage.Issues), test.wantIssues, formatDesignExampleIssues(stage.Issues))
			}
			if !hasDesignExampleIssue(stage.Issues, test.wantPath, test.wantMessage) {
				t.Fatalf("missing expected issue %s containing %q in:\n%s", test.wantPath, test.wantMessage, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}

func TestDesignExamplesGenerateReadableProjectArtifacts(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	createTimeout := designExampleCreateTimeout(t)
	for _, name := range designExampleRequestFiles(t, repoRoot) {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request, issues := loadDesignExampleRequest(t, repoRoot, name)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues:\n%s", name, formatDesignExampleIssues(issues))
			}
			projectName := NormalizeProjectName(request.Name)
			outputDir := filepath.Join(t.TempDir(), projectName)
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout)
			defer cancel()
			result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
			if result.Acceptance.Achieved == "" {
				t.Fatalf("%s did not achieve an acceptance level:\n%s", name, formatDesignExampleStages(result.Stages))
			}
			projectWrite, ok := designExampleStageByName(result, StageProjectWrite)
			if !ok {
				t.Fatalf("%s missing project_write stage:\n%s", name, formatDesignExampleStages(result.Stages))
			}
			if projectWrite.Status != StageStatusOK {
				t.Fatalf("%s project_write status = %q, want %q:\n%s", name, projectWrite.Status, StageStatusOK, formatDesignExampleIssues(projectWrite.Issues))
			}
			projectPath := filepath.Join(outputDir, projectName+".kicad_pro")
			schematicPath := filepath.Join(outputDir, projectName+".kicad_sch")
			pcbPath := filepath.Join(outputDir, projectName+".kicad_pcb")
			transactionPath := filepath.Join(outputDir, ".kicadai", "transaction.json")
			manifestPath := filepath.Join(outputDir, ".kicadai", "manifest.json")
			requiredArtifacts := []string{projectPath, schematicPath, pcbPath, transactionPath, manifestPath}
			for _, path := range requiredArtifacts {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("%s missing generated artifact %s: %v", name, path, err)
				}
			}
			schematicFile, err := schematic.ReadFile(schematicPath)
			if err != nil {
				t.Fatalf("%s generated schematic is not readable: %v", name, err)
			}
			if got := countDesignExampleSymbolsWithProperty(schematicFile.Symbols, componentprops.PropertyComponentID); got == 0 {
				t.Fatalf("%s generated schematic has no %q properties", name, componentprops.PropertyComponentID)
			}
			if _, err := pcb.ReadFile(pcbPath); err != nil {
				t.Fatalf("%s generated PCB is not readable: %v", name, err)
			}
		})
	}
}

func TestDesignExamplesOptionalKiCadBackedTier(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	if cliPath == "" {
		t.Skipf("set %s to run optional KiCad-backed design examples", checks.EnvKiCadCLI)
	}
	repoRoot := designExampleRepoRoot(t)
	metadataPaths := optionalDesignExampleMetadataFiles(t, repoRoot)
	if len(metadataPaths) == 0 {
		t.Skip("no optional KiCad-backed design examples found under examples/design/kicad-backed")
	}
	createTimeout := designExampleCreateTimeout(t)
	for _, metadataPath := range metadataPaths {
		metadataPath := metadataPath
		t.Run(strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json"), func(t *testing.T) {
			metadata, err := loadDesignExampleMetadataPath(metadataPath)
			if err != nil {
				t.Fatalf("load %s: %v", metadataPath, err)
			}
			if metadata.Readiness == "blocked" {
				t.Skipf("%s blocked: %s", metadata.ID, strings.Join(metadata.KnownGaps, "; "))
			}
			requestPath, err := designExampleRequestPathForMetadata(metadataPath, metadata)
			if err != nil {
				t.Fatalf("%s request path: %v", metadata.ID, err)
			}
			request, issues := loadDesignExampleRequestPath(t, requestPath)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues:\n%s", requestPath, formatDesignExampleIssues(issues))
			}
			projectName := NormalizeProjectName(request.Name)
			outputDir := designExamplePersistentOutputDir(t, projectName)
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout*2)
			defer cancel()
			result := Create(ctx, request, CreateOptions{
				OutputDir: outputDir,
				Overwrite: true,
				KiCadChecks: KiCadCheckOptions{
					KiCadCLI:      cliPath,
					Timeout:       createTimeout,
					RequireERC:    requiredDesignExampleBool(t, metadata.ID, "requires_erc", metadata.RequiresERC),
					RequireDRC:    requiredDesignExampleBool(t, metadata.ID, "requires_drc", metadata.RequiresDRC),
					KeepArtifacts: true,
					ArtifactDir:   filepath.Join(outputDir, ".kicadai", "checks"),
				},
			})
			assertDesignExampleExpectedStages(t, metadata, result, outputDir)
			kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
			if !ok {
				t.Fatalf("%s missing kicad_checks stage:\n%s", metadata.ID, formatDesignExampleStages(result.Stages))
			}
			switch metadata.Readiness {
			case "pass", "candidate":
				if kicadChecks.Status != StageStatusOK {
					t.Fatalf("%s kicad_checks status = %q, want %q:\n%s", metadata.ID, kicadChecks.Status, StageStatusOK, formatDesignExampleRun(metadata, outputDir, result))
				}
			case "expected_fail":
				if kicadChecks.Status == StageStatusOK || !designExampleHasBlockedStage(result) {
					t.Fatalf("%s expected blocked evidence, got kicad_checks=%q:\n%s", metadata.ID, kicadChecks.Status, formatDesignExampleRun(metadata, outputDir, result))
				}
			default:
				t.Fatalf("%s unsupported readiness %q", metadata.ID, metadata.Readiness)
			}
		})
	}
}

func TestDesignExampleMetadataValidation(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "led_indicator_kicad_smoke.json")
	if err := os.WriteFile(requestPath, []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             "led_indicator_kicad_smoke",
		Request:        "led_indicator_kicad_smoke.json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceERCDRC,
		RequiresERC:    designExampleBool(true),
		RequiresDRC:    designExampleBool(true),
		ExpectedStages: []StageName{StageBlockPlanning, StageKiCadChecks},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "led_indicator_kicad_smoke.metadata.json", metadata)
	loaded, err := loadDesignExampleMetadataPath(path)
	if err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
	if !reflect.DeepEqual(loaded, metadata) {
		t.Fatalf("loaded metadata = %#v, want %#v", loaded, metadata)
	}
}

func TestDesignExampleMetadataRejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.metadata.json")
	if err := os.WriteFile(path, []byte(`{"id":"fixture","request":"fixture.json","tier":"smoke","readiness":"candidate"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "acceptance is required") {
		t.Fatalf("missing acceptance error = %v, want acceptance is required", err)
	}

	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path = writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err = loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "requires_erc is required") {
		t.Fatalf("missing requires_erc error = %v, want requires_erc is required", err)
	}
}

func TestDesignExampleMetadataRejectsInvalidEnums(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "fixture.json",
		Tier:           "slow",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported tier") {
		t.Fatalf("invalid enum error = %v, want unsupported tier", err)
	}
}

func TestDesignExampleMetadataRejectsRequestPathTraversal(t *testing.T) {
	dir := t.TempDir()
	metadata := designExampleMetadata{
		ID:             "fixture",
		Request:        "../fixture.json",
		Tier:           "smoke",
		Readiness:      "candidate",
		Acceptance:     AcceptanceStructural,
		ExpectedStages: []StageName{StageBlockPlanning},
		KnownGaps:      []string{},
	}
	path := writeDesignExampleMetadataFixture(t, dir, "fixture.metadata.json", metadata)
	_, err := loadDesignExampleMetadataPath(path)
	if err == nil || !strings.Contains(err.Error(), "request must be a local JSON filename") {
		t.Fatalf("path traversal error = %v, want local filename rejection", err)
	}
}

func TestFormatDesignExampleStagesGroupsIssuesUnderStage(t *testing.T) {
	got := formatDesignExampleStages([]StageResult{{
		Name:   StageProjectWrite,
		Status: StageStatusBlocked,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "operations[0]",
			Message:  "example failure",
		}, {
			Code:     reports.CodeMissingFootprint,
			Severity: reports.SeverityWarning,
			Path:     "components.R1",
			Message:  "missing footprint evidence",
		}},
	}})
	want := "- project_write: blocked\n  - error VALIDATION_FAILED at operations[0]: example failure\n  - warning MISSING_FOOTPRINT at components.R1: missing footprint evidence"
	if got != want {
		t.Fatalf("formatted stages:\n%q\nwant:\n%q", got, want)
	}
}

func designExamplePlanStage(ctx context.Context, request Request) StageResult {
	planResult := PlanBlocks(ctx, blocks.NewBuiltinRegistry(), request)
	return planResult.Stage
}

func loadDesignExampleRequest(t *testing.T, repoRoot, name string) (Request, []reports.Issue) {
	t.Helper()
	return loadDesignExampleRequestPath(t, filepath.Join(repoRoot, "examples", "design", name))
}

func loadDesignExampleRequestPath(t *testing.T, path string) (Request, []reports.Issue) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	return request, issues
}

func loadDesignExampleMetadataPath(path string) (designExampleMetadata, error) {
	var metadata designExampleMetadata
	file, err := os.Open(path)
	if err != nil {
		return metadata, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return designExampleMetadata{}, err
	}
	if err := validateDesignExampleMetadata(filepath.Dir(path), metadata); err != nil {
		return designExampleMetadata{}, err
	}
	return metadata, nil
}

func validateDesignExampleMetadata(dir string, metadata designExampleMetadata) error {
	if strings.TrimSpace(metadata.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(metadata.Request) == "" {
		return fmt.Errorf("request is required")
	}
	if filepath.Base(metadata.Request) != metadata.Request || filepath.Ext(metadata.Request) != ".json" || strings.HasSuffix(metadata.Request, ".metadata.json") {
		return fmt.Errorf("request must be a local JSON filename")
	}
	if strings.TrimSuffix(filepath.Base(metadata.Request), ".json") != metadata.ID {
		return fmt.Errorf("id must match request basename")
	}
	requestPath := filepath.Join(dir, metadata.Request)
	info, err := os.Stat(requestPath)
	if err != nil {
		return fmt.Errorf("request %s: %w", metadata.Request, err)
	}
	if info.IsDir() {
		return fmt.Errorf("request %s must be a file", metadata.Request)
	}
	if _, ok := designExampleMetadataTiers[metadata.Tier]; !ok {
		return fmt.Errorf("unsupported tier %q", metadata.Tier)
	}
	if _, ok := designExampleMetadataReadiness[metadata.Readiness]; !ok {
		return fmt.Errorf("unsupported readiness %q", metadata.Readiness)
	}
	if _, ok := designExampleMetadataAcceptance[metadata.Acceptance]; !ok {
		if metadata.Acceptance == "" {
			return fmt.Errorf("acceptance is required")
		}
		return fmt.Errorf("unsupported acceptance %q", metadata.Acceptance)
	}
	if metadata.RequiresERC == nil {
		return fmt.Errorf("requires_erc is required")
	}
	if metadata.RequiresDRC == nil {
		return fmt.Errorf("requires_drc is required")
	}
	if len(metadata.ExpectedStages) == 0 {
		return fmt.Errorf("expected_stages must not be empty")
	}
	if metadata.KnownGaps == nil {
		return fmt.Errorf("known_gaps is required")
	}
	return nil
}

func designExampleBool(value bool) *bool {
	return &value
}

func requiredDesignExampleBool(t *testing.T, id, field string, value *bool) bool {
	t.Helper()
	if value == nil {
		t.Fatalf("%s metadata missing %s", id, field)
	}
	return *value
}

func designExamplePersistentOutputDir(t *testing.T, projectName string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kicadai-design-example-*")
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(dir, projectName)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("preserved design example output at %s", outputDir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("remove design example temp dir %s: %v", dir, err)
		}
	})
	return outputDir
}

func writeDesignExampleMetadataFixture(t *testing.T, dir, name string, metadata designExampleMetadata) string {
	t.Helper()
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func designExampleRequestFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	pattern := filepath.Join(repoRoot, "examples", "design", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("no design examples matched %s", pattern)
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	sort.Strings(names)
	return names
}

func optionalDesignExampleMetadataFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	pattern := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "*.metadata.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)
	return matches
}

func designExampleRequestPathForMetadata(metadataPath string, metadata designExampleMetadata) (string, error) {
	baseDir := filepath.Clean(filepath.Dir(metadataPath))
	requestPath := filepath.Clean(filepath.Join(baseDir, metadata.Request))
	expectedPath := filepath.Clean(filepath.Join(baseDir, filepath.Base(requestPath)))
	if requestPath != expectedPath {
		return "", fmt.Errorf("request %q must stay in metadata directory", metadata.Request)
	}
	return requestPath, nil
}

func designExampleRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func hasDesignExampleIssue(issues []reports.Issue, path, message string) bool {
	for _, issue := range issues {
		if issue.Path == path && strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func assertDesignExampleExpectedStages(t *testing.T, metadata designExampleMetadata, result WorkflowResult, outputDir string) {
	t.Helper()
	for _, name := range metadata.ExpectedStages {
		stage, ok := designExampleStageByName(result, name)
		if !ok {
			t.Fatalf("%s missing expected stage %q:\n%s", metadata.ID, name, formatDesignExampleRun(metadata, outputDir, result))
		}
		if metadata.Readiness != "expected_fail" && stage.Status == StageStatusBlocked {
			t.Fatalf("%s stage %q blocked unexpectedly:\n%s", metadata.ID, name, formatDesignExampleRun(metadata, outputDir, result))
		}
	}
}

func designExampleHasBlockedStage(result WorkflowResult) bool {
	for _, stage := range result.Stages {
		if stage.Status == StageStatusBlocked {
			return true
		}
	}
	return false
}

func countDesignExampleSymbolsWithProperty(symbols []schematic.SchematicSymbol, name string) int {
	count := 0
	for _, symbol := range symbols {
		for _, property := range symbol.Properties {
			if property.Name == name {
				count++
				break
			}
		}
	}
	return count
}

func formatDesignExampleIssues(issues []reports.Issue) string {
	if len(issues) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, issue := range issues {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(string(issue.Severity))
		builder.WriteByte(' ')
		builder.WriteString(string(issue.Code))
		if issue.Path != "" {
			builder.WriteString(" at ")
			builder.WriteString(issue.Path)
		}
		builder.WriteString(": ")
		builder.WriteString(issue.Message)
	}
	return builder.String()
}

func designExampleStageByName(result WorkflowResult, name StageName) (StageResult, bool) {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func formatDesignExampleStages(stages []StageResult) string {
	if len(stages) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, stage := range stages {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(string(stage.Name))
		builder.WriteString(": ")
		builder.WriteString(string(stage.Status))
		if len(stage.Issues) != 0 {
			builder.WriteString("\n")
			builder.WriteString(indentDesignExampleText(formatDesignExampleIssues(stage.Issues), "  "))
		}
	}
	return builder.String()
}

func formatDesignExampleRun(metadata designExampleMetadata, outputDir string, result WorkflowResult) string {
	var builder strings.Builder
	builder.WriteString("fixture: ")
	builder.WriteString(metadata.ID)
	builder.WriteString("\nreadiness: ")
	builder.WriteString(metadata.Readiness)
	builder.WriteString("\noutput: ")
	builder.WriteString(outputDir)
	builder.WriteString("\nstages:\n")
	builder.WriteString(indentDesignExampleText(formatDesignExampleStages(result.Stages), "  "))
	return builder.String()
}

func designExampleCreateTimeout(t *testing.T) time.Duration {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv("KICADAI_EXAMPLE_TEST_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			t.Fatalf("KICADAI_EXAMPLE_TEST_TIMEOUT=%q is not a valid duration: %v", value, err)
		}
		return duration
	}
	return time.Minute
}

func indentDesignExampleText(text, prefix string) string {
	if text == "" {
		return text
	}
	trimmed := strings.TrimSuffix(text, "\n")
	return prefix + strings.ReplaceAll(trimmed, "\n", "\n"+prefix)
}
