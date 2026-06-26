package intentdraft

import (
	"encoding/json"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func TestDraftEmptyInputReportsBlockingIssue(t *testing.T) {
	result := Draft("   ", Options{})
	if len(result.Issues) == 0 {
		t.Fatal("expected issue for empty input")
	}
	if result.Issues[0].Path != "text" || result.Issues[0].Severity != reports.SeverityError {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Request.Version == "" || result.Request.Name == "" {
		t.Fatalf("request was not normalized: %#v", result.Request)
	}
}

func TestDraftSourceHashStable(t *testing.T) {
	first := Draft("make a sensor board", Options{SourceID: "prompt"})
	second := Draft(" make a sensor board\n", Options{SourceID: "prompt"})
	if first.Extraction.SourceHash != second.Extraction.SourceHash {
		t.Fatalf("hash mismatch: %s != %s", first.Extraction.SourceHash, second.Extraction.SourceHash)
	}
	if first.Extraction.SourceID != "prompt" || first.Extraction.SourceType != SourceTypeText {
		t.Fatalf("extraction = %#v", first.Extraction)
	}
}

func TestDraftAppliesAcceptanceOverrideAndJSONDeterministic(t *testing.T) {
	result := Draft("make a board", Options{AcceptanceOverride: designworkflow.AcceptanceConnectivity})
	if result.Request.Acceptance != designworkflow.AcceptanceConnectivity {
		t.Fatalf("acceptance = %q", result.Request.Acceptance)
	}
	first, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("nondeterministic json:\n%s\n%s", first, second)
	}
}
