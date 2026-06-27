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

func TestDesignExamplesCurrentContractAudit(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	tests := []struct {
		name        string
		wantPath    string
		wantMessage string
	}{
		{
			name:        "led_indicator.json",
			wantPath:    "blocks[0].params.resistor_ohms",
			wantMessage: "unknown parameter resistor_ohms",
		},
		{
			name:        "sensor_breakout.json",
			wantPath:    "connections[4].from",
			wantMessage: "unknown port sensor.INT",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, issues := loadDesignExampleRequest(t, repoRoot, test.name)
			if len(issues) != 0 {
				requireOnlyExpectedIssue(t, test.name, issues, test.wantPath, test.wantMessage)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stage := designExamplePlanStage(ctx, request)
			requireOnlyExpectedIssue(t, test.name, stage.Issues, test.wantPath, test.wantMessage)
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

func requireOnlyExpectedIssue(t *testing.T, name string, issues []reports.Issue, path, message string) {
	t.Helper()
	if len(issues) != 1 {
		t.Fatalf("%s issues = %#v, want exactly one expected drift", name, issues)
	}
	issue := issues[0]
	if issue.Path != path || !strings.Contains(issue.Message, message) {
		t.Fatalf("%s issue = %#v, want path %s message containing %q", name, issue, path, message)
	}
}
