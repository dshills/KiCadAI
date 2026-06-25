package intentplanner

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestIntentFixturesPlan(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "intent", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no golden intent fixtures found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			plan := Plan(request)
			if plan.Status == PlanStatusBlocked || hasErrorIssue(plan.Issues) {
				t.Fatalf("plan failed: status=%s issues=%#v", plan.Status, plan.Issues)
			}
			if plan.GeneratedRequest == nil {
				t.Fatalf("generated request missing: status=%s issues=%#v", plan.Status, plan.Issues)
			}
			if len(plan.Requirements) == 0 {
				t.Fatalf("requirements missing")
			}
			if len(plan.SelectedBlocks) == 0 {
				t.Fatalf("selected blocks missing")
			}
		})
	}
}

func hasErrorIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked {
			return true
		}
	}
	return false
}
