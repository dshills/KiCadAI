package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/blocks"
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

func designExamplePlanStage(ctx context.Context, request Request) StageResult {
	planResult := PlanBlocks(ctx, blocks.NewBuiltinRegistry(), request)
	return planResult.Stage
}

func loadDesignExampleRequest(t *testing.T, repoRoot, name string) (Request, []reports.Issue) {
	t.Helper()
	path := filepath.Join(repoRoot, "examples", "design", name)
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
	return names
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
