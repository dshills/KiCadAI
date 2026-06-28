package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestAmplifierDesignFixturesPlanToDeclaredAcceptance(t *testing.T) {
	repoRoot := designExampleRepoRoot(t)
	paths, err := filepath.Glob(filepath.Join(repoRoot, "examples", "design", "amplifier", "*.json"))
	if err != nil {
		t.Fatalf("glob amplifier fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no amplifier design fixtures found")
	}
	expectedAcceptance := map[string]AcceptanceLevel{
		"opamp_headphone_buffer": AcceptanceDraft,
	}
	sort.Strings(paths)
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open amplifier fixture: %v", err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues:\n%s", formatDesignExampleIssues(issues))
			}
			if request.Validation.Acceptance == "" {
				t.Fatal("acceptance must be specified")
			}
			fixtureName := strings.TrimSuffix(filepath.Base(path), ".json")
			expected, ok := expectedAcceptance[fixtureName]
			if !ok {
				t.Fatalf("fixture %q is missing an expected acceptance entry", fixtureName)
			}
			if request.Name != fixtureName {
				t.Fatalf("request name = %q, want fixture name %q", request.Name, fixtureName)
			}
			if request.Validation.Acceptance != expected {
				t.Fatalf("acceptance = %q, want %q", request.Validation.Acceptance, expected)
			}
			ctx, cancel := context.WithTimeout(context.Background(), designExamplePlanningTimeout)
			defer cancel()
			stage := designExamplePlanStage(ctx, request)
			if stage.Status != StageStatusOK || len(stage.Issues) != 0 {
				t.Fatalf("block planning status = %q issues:\n%s", stage.Status, formatDesignExampleIssues(stage.Issues))
			}
		})
	}
}
