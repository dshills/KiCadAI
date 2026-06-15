package reports

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestOKResultInitializesStableEnvelope(t *testing.T) {
	result := OKResult("inspect-project", map[string]string{"name": "demo"}, []Artifact{{
		Kind: ArtifactKiCadProject,
		Path: "demo/demo.kicad_pro",
	}})

	if !result.OK {
		t.Fatal("OK = false, want true")
	}
	if result.Command != "inspect-project" || result.Version != Version {
		t.Fatalf("unexpected envelope: %#v", result)
	}
	if result.Issues == nil || result.Artifacts == nil {
		t.Fatalf("slices should be initialized: %#v", result)
	}
}

func TestResultWithIssuesMarksBlockingSeverities(t *testing.T) {
	result := ResultWithIssues("evaluate-project", nil, []Issue{
		{Code: CodeSkippedExternalTool, Severity: SeverityWarning, Message: "kicad-cli unavailable"},
	}, nil)
	if !result.OK {
		t.Fatalf("warning-only result should be OK: %#v", result)
	}

	result = ResultWithIssues("evaluate-project", nil, []Issue{
		{Code: CodeMissingFootprint, Severity: SeverityError, Message: "R1 has no footprint"},
	}, nil)
	if result.OK {
		t.Fatalf("error result should not be OK: %#v", result)
	}

	result = ResultWithIssues("evaluate-project", nil, []Issue{
		{Code: CodeUnsupportedOperation, Severity: SeverityBlocked, Message: "reader unavailable"},
	}, nil)
	if result.OK {
		t.Fatalf("blocked result should not be OK: %#v", result)
	}
}

func TestErrorResultUsesSingleIssue(t *testing.T) {
	issue := Issue{Code: CodeInvalidArgument, Severity: SeverityError, Message: "missing path"}
	result := ErrorResult("inspect-project", issue)

	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != CodeInvalidArgument {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestErrorResultAllowsNonBlockingIssue(t *testing.T) {
	result := ErrorResult("roundtrip-pcb", Issue{
		Code:     CodeSkippedExternalTool,
		Severity: SeverityWarning,
		Message:  "kicad-cli unavailable",
	})

	if !result.OK {
		t.Fatalf("warning result should be OK: %#v", result)
	}
}

func TestIssueFromError(t *testing.T) {
	issue, ok := IssueFromError(errors.New("failed"))
	if !ok {
		t.Fatal("IssueFromError returned ok false")
	}
	if issue.Code != CodeValidationFailed || issue.Severity != SeverityError || issue.Message != "failed" {
		t.Fatalf("unexpected issue: %#v", issue)
	}
	if got, ok := IssueFromError(nil); ok {
		t.Fatalf("nil error issue = %#v, want ok false", got)
	}
}

func TestWriteJSONShape(t *testing.T) {
	var buf bytes.Buffer
	result := ResultWithIssues("evaluate-pcb", map[string]bool{"ready": false}, []Issue{{
		Code:        CodeDisconnectedPad,
		Severity:    SeverityError,
		Path:        `pcb.footprints[J1].pads["1"]`,
		OperationID: "op-route-net-gnd-1234567890",
		Message:     "pad is disconnected",
		UUIDs:       []string{"12345678-1234-5678-9234-123456789abc"},
		Refs:        []string{"J1"},
		Nets:        []string{"GND"},
		Suggestion:  "route GND",
	}}, []Artifact{{
		Kind: ArtifactValidationReport,
		Path: "reports/validation.json",
	}})

	if err := WriteJSON(&buf, result); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "evaluate-pcb"`,
		`"code": "DISCONNECTED_PAD"`,
		`"severity": "error"`,
		`"operation_id": "op-route-net-gnd-1234567890"`,
		`"uuids": [`,
		`"kind": "validation_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	var decoded Result
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}
