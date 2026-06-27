package designworkflow

import (
	"context"
	"os"
	"path/filepath"
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
	paths := optionalDesignExampleRequestFiles(t, repoRoot)
	if len(paths) == 0 {
		t.Skip("no optional KiCad-backed design examples found under examples/design/kicad-backed")
	}
	createTimeout := designExampleCreateTimeout(t)
	for _, path := range paths {
		path := path
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			request, issues := loadDesignExampleRequestPath(t, path)
			if len(issues) != 0 {
				t.Fatalf("decode %s issues:\n%s", path, formatDesignExampleIssues(issues))
			}
			projectName := NormalizeProjectName(request.Name)
			outputDir := filepath.Join(t.TempDir(), projectName)
			ctx, cancel := context.WithTimeout(context.Background(), createTimeout*2)
			defer cancel()
			result := Create(ctx, request, CreateOptions{
				OutputDir:   outputDir,
				Overwrite:   true,
				KiCadChecks: KiCadCheckOptions{KiCadCLI: cliPath, Timeout: createTimeout, RequireERC: true, RequireDRC: true, KeepArtifacts: true, ArtifactDir: filepath.Join(outputDir, ".kicadai", "checks")},
			})
			kicadChecks, ok := designExampleStageByName(result, StageKiCadChecks)
			if !ok {
				t.Fatalf("%s missing kicad_checks stage:\n%s", name, formatDesignExampleStages(result.Stages))
			}
			if kicadChecks.Status != StageStatusOK {
				t.Fatalf("%s kicad_checks status = %q, want %q:\n%s", name, kicadChecks.Status, StageStatusOK, formatDesignExampleIssues(kicadChecks.Issues))
			}
		})
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

func optionalDesignExampleRequestFiles(t *testing.T, repoRoot string) []string {
	t.Helper()
	pattern := filepath.Join(repoRoot, "examples", "design", "kicad-backed", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)
	return matches
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
