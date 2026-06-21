package designworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/reports"
)

const retryFixtureRoot = "testdata/retry"

func retryFixtureRequestPath(name string) string {
	return filepath.Join(retryFixtureRoot, name, "request.json")
}

func readRetryFixtureRequest(name string) (Request, []reports.Issue, error) {
	path := retryFixtureRequestPath(name)
	file, err := os.Open(path)
	if err != nil {
		return Request{}, nil, err
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	return request, issues, nil
}

func loadRetryFixtureRequest(t *testing.T, name string) Request {
	t.Helper()
	request, issues, err := readRetryFixtureRequest(name)
	if err != nil {
		t.Fatalf("load retry fixture %q from %s: %v", name, retryFixtureRequestPath(name), err)
	}
	if len(issues) != 0 {
		t.Fatalf("retry fixture %q decode issues = %#v", name, issues)
	}
	return request
}

func runRetryFixture(t *testing.T, name string) WorkflowResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return Create(ctx, loadRetryFixtureRequest(t, name), CreateOptions{OutputDir: output})
}

func retrySummaryFromStage(t *testing.T, stage StageResult) (placementRoutingRetrySummary, bool) {
	t.Helper()
	if stage.Summary == nil {
		return placementRoutingRetrySummary{}, false
	}
	raw, ok := stage.Summary["routing_retry"]
	if !ok {
		return placementRoutingRetrySummary{}, false
	}
	switch summary := raw.(type) {
	case placementRoutingRetrySummary:
		return summary, true
	case map[string]any:
		encoded, err := json.Marshal(summary)
		if err != nil {
			t.Fatalf("marshal routing_retry summary %#v: %v", summary, err)
		}
		var decoded placementRoutingRetrySummary
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode routing_retry summary %#v: %v", summary, err)
		}
		return decoded, true
	default:
		t.Fatalf("routing_retry summary has unsupported type %T: %#v", raw, raw)
		return placementRoutingRetrySummary{}, false
	}
}

func normalizeRetrySnapshotPaths(value any, outputDir string) any {
	return normalizeRetrySnapshotPathsWithOutput(value, filepath.ToSlash(outputDir))
}

// normalizeRetrySnapshotPathsWithOutput expects JSON-shaped values.
func normalizeRetrySnapshotPathsWithOutput(value any, slashOutput string) any {
	switch typed := value.(type) {
	case string:
		slashTyped := filepath.ToSlash(typed)
		if slashOutput != "" && (slashTyped == slashOutput || strings.HasPrefix(slashTyped, slashOutput+"/")) {
			return "<OUT>" + slashTyped[len(slashOutput):]
		}
		return slashTyped
	case []any:
		normalized := make([]any, len(typed))
		for index, item := range typed {
			normalized[index] = normalizeRetrySnapshotPathsWithOutput(item, slashOutput)
		}
		return normalized
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeRetrySnapshotPathsWithOutput(item, slashOutput)
		}
		return normalized
	default:
		return value
	}
}

func assertRetrySummaryInvariant(t *testing.T, summary placementRoutingRetrySummary, wantEnabled bool) {
	t.Helper()
	if summary.Enabled != wantEnabled {
		t.Fatalf("retry enabled = %v, want %v in %#v", summary.Enabled, wantEnabled, summary)
	}
	if summary.Attempts < 1 {
		t.Fatalf("retry attempts = %d, want at least 1 in %#v", summary.Attempts, summary)
	}
	if summary.Applied < 0 || summary.Applied > summary.Attempts-1 {
		t.Fatalf("retry applied count inconsistent in %#v", summary)
	}
}

func TestRetryGoldenHarnessMissingFixtureReportsPath(t *testing.T) {
	_, _, err := readRetryFixtureRequest("missing")
	if err == nil {
		t.Fatal("missing fixture unexpectedly loaded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing fixture error = %v, want not exist", err)
	}
	if !strings.Contains(err.Error(), retryFixtureRequestPath("missing")) {
		t.Fatalf("missing fixture error %q does not include request path", err)
	}
}

func TestRetryGoldenHarnessSummaryExtractionMissingSummary(t *testing.T) {
	if summary, ok := retrySummaryFromStage(t, StageResult{}); ok || summary.Attempts != 0 {
		t.Fatalf("summary = %#v ok=%v, want missing", summary, ok)
	}
}

func TestRetryGoldenHarnessNormalizesSnapshotPaths(t *testing.T) {
	output := filepath.Join(t.TempDir(), "retry")
	input := map[string]any{
		"artifact": filepath.Join(output, "retry.kicad_pro"),
		"sibling":  filepath.Join(output+"-sibling", "retry.kicad_pro"),
		"nested": []any{
			filepath.Join(output, "retry.kicad_sch"),
			"unchanged",
		},
	}

	normalized := normalizeRetrySnapshotPaths(input, output).(map[string]any)
	if normalized["artifact"] != "<OUT>/retry.kicad_pro" {
		t.Fatalf("artifact path = %#v", normalized["artifact"])
	}
	if normalized["sibling"] != filepath.ToSlash(filepath.Join(output+"-sibling", "retry.kicad_pro")) {
		t.Fatalf("sibling path = %#v", normalized["sibling"])
	}
	nested := normalized["nested"].([]any)
	if nested[0] != "<OUT>/retry.kicad_sch" || nested[1] != "unchanged" {
		t.Fatalf("nested paths = %#v", nested)
	}
}

func TestRetryGoldenDisabledFixtureKeepsSingleAttemptBehavior(t *testing.T) {
	request := loadRetryFixtureRequest(t, "disabled")
	if request.RoutingRetry.Enabled {
		t.Fatalf("disabled fixture enabled retry: %#v", request.RoutingRetry)
	}

	result := runRetryFixture(t, "disabled")
	stage, ok := stageByName(result, StageRouting)
	if !ok {
		t.Fatalf("routing stage missing: %#v", result.Stages)
	}
	if summary, ok := retrySummaryFromStage(t, stage); ok {
		t.Fatalf("disabled retry should not attach routing_retry summary: %#v", summary)
	}
	if stage.Status != StageStatusSkipped {
		t.Fatalf("disabled skip-routing stage status = %s, want skipped: %#v", stage.Status, stage)
	}
}
