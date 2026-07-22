package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"testing"
)

func TestBoundedResultRetainsCountsAndDeterministicSamples(t *testing.T) {
	issues := make([]Issue, 0, 180)
	for index := 0; index < 180; index++ {
		issues = append(issues, Issue{
			Code:     CodeUnknownSymbolLibrary,
			Severity: SeverityError,
			Stage:    "library_context",
			Path:     fmt.Sprintf("source-%02d", index%12),
			Message:  fmt.Sprintf("missing symbol %03d", 179-index),
		})
	}
	forward := BoundedResult(ResultWithIssues("circuit.preflight", nil, issues, nil))
	reversedIssues := append([]Issue(nil), issues...)
	slices.Reverse(reversedIssues)
	reversed := BoundedResult(ResultWithIssues("circuit.preflight", nil, reversedIssues, nil))

	if len(forward.Issues) != DefaultMaxEmittedIssues {
		t.Fatalf("emitted issues = %d, want %d", len(forward.Issues), DefaultMaxEmittedIssues)
	}
	if forward.Diagnostics == nil {
		t.Fatal("missing diagnostic summary")
	}
	if forward.Diagnostics.TotalCount != 180 || forward.Diagnostics.EmittedCount != 128 || forward.Diagnostics.OmittedCount != 52 {
		t.Fatalf("unexpected issue counts: %#v", forward.Diagnostics)
	}
	if forward.Diagnostics.GroupCount != 12 || forward.Diagnostics.EmittedGroupCount != 12 || forward.Diagnostics.OmittedGroupCount != 0 {
		t.Fatalf("unexpected group counts: %#v", forward.Diagnostics)
	}
	for _, group := range forward.Diagnostics.Groups {
		if group.TotalCount != 15 || group.EmittedCount != 3 || group.OmittedCount != 12 || len(group.Samples) != 3 {
			t.Fatalf("unexpected group: %#v", group)
		}
	}
	if !reflect.DeepEqual(forward, reversed) {
		t.Fatal("bounded result changed when input order was reversed")
	}
}

func TestBoundedResultCapsGroupMetadata(t *testing.T) {
	issues := make([]Issue, 0, 160)
	for index := 0; index < 160; index++ {
		issues = append(issues, Issue{Code: CodeMissingFootprint, Severity: SeverityError, Stage: "library_context", Path: fmt.Sprintf("source-%03d", index), Message: "missing footprint"})
	}
	bounded := BoundedResult(ResultWithIssues("circuit.preflight", nil, issues, nil))
	if bounded.Diagnostics == nil || bounded.Diagnostics.GroupCount != 160 || bounded.Diagnostics.EmittedGroupCount != DefaultMaxDiagnosticGroups || bounded.Diagnostics.OmittedGroupCount != 128 {
		t.Fatalf("unexpected group summary: %#v", bounded.Diagnostics)
	}
}

func TestWriteJSONEmitsExactlyOneBoundedDocument(t *testing.T) {
	issues := make([]Issue, DefaultMaxEmittedIssues+10)
	for index := range issues {
		issues[index] = Issue{Code: CodeValidationFailed, Severity: SeverityError, Stage: "validation", Path: "stock-library", Message: fmt.Sprintf("failure %03d", index)}
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, ResultWithIssues("circuit.preflight", nil, issues, nil)); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	decoder := json.NewDecoder(&output)
	var result Result
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("second Decode() error = %v, want io.EOF", err)
	}
	if result.Diagnostics == nil || result.Diagnostics.OmittedCount != 10 {
		t.Fatalf("unexpected decoded diagnostics: %#v", result.Diagnostics)
	}
	if output.Len() > 100_000 {
		t.Fatalf("representative bounded output = %d bytes, want <= 100000", output.Len())
	}
}

func TestBoundedResultLeavesCompactEnvelopeUnchanged(t *testing.T) {
	result := ResultWithIssues("test", nil, []Issue{{Code: CodeUnknown, Severity: SeverityWarning, Message: "small"}}, nil)
	if got := BoundedResult(result); !reflect.DeepEqual(got, result) {
		t.Fatalf("BoundedResult() = %#v, want unchanged %#v", got, result)
	}
}
