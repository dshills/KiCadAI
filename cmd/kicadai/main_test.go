package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/config"
	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/manifest"
	"kicadai/internal/provenance"
	"kicadai/internal/repair"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
	"kicadai/internal/workflows"
)

func TestRunDefaultsToHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRunRejectsInvalidFormatWithoutCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--format", "yaml"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), `invalid --format "yaml"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckArtifactsSkipsMissingReport(t *testing.T) {
	artifacts := checkArtifacts(checks.CheckResult{
		Kind:       checks.CheckKindDRC,
		ReportPath: filepath.Join(t.TempDir(), "missing-drc.json"),
	})
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for missing report", artifacts)
	}
}

func TestRunConfigDefaultsToJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--token", "secret-token",
		"--client-name", "test-client",
		"--timeout-ms", "3000",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()

	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"timeout_ms": 3000`,
		`"token": "<redacted>"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}

	if strings.Contains(output, "secret-token") {
		t.Fatalf("token leaked in output: %s", output)
	}
}

func TestRunConfigTextRedactsToken(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--token", "secret-token",
		"--client-name", "test-client",
		"--format", "text",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"socket_path: ipc:///tmp/kicad/api.sock",
		"client_name: test-client",
		"token: <redacted>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
	if strings.Contains(output, "secret-token") {
		t.Fatalf("token leaked in output: %s", output)
	}
}

func TestRunJSONFlagOverridesFormatText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--format", "text",
		"--json",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"socket_path": "ipc:///tmp/kicad/api.sock"`) {
		t.Fatalf("expected JSON output, got %s", stdout.String())
	}
}

func TestRunJSONFalseDoesNotOverrideFormatText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--format", "text",
		"--json=false",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "socket_path: ipc:///tmp/kicad/api.sock") {
		t.Fatalf("expected text output, got %s", stdout.String())
	}
}

func TestRunJSONFalseDefaultsToText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json=false",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "socket_path: ipc:///tmp/kicad/api.sock") {
		t.Fatalf("expected text output, got %s", stdout.String())
	}
}

func TestRunDesignCreateMissingRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--output", filepath.Join(t.TempDir(), "out"), "design", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing request to return an error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "request" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunIntentPlanDefaultsToJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"intent", "plan"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing request error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "request" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunIntentPlanMissingRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "intent", "plan"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing request error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "request" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunIntentDraftFromText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a 3.3V I2C temperature sensor breakout", "intent", "draft"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"kind": "sensor_node"`, `"i2c"`, `"source_hash"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestRunIntentDraftFirstLanePromptGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{
			name: "led_indicator",
			text: "make a simple LED indicator board",
			want: []string{`"name": "led_indicator"`, `"kind": "breakout"`, `"kind": "gpio"`, `"kind": "indicator"`, `"voltage": "3.3V"`},
		},
		{
			name: "connector_power_led",
			text: "make a connector breakout with power LED",
			want: []string{`"kind": "breakout"`, `"kind": "gpio"`, `"kind": "indicator"`, `"voltage": "3.3V"`},
		},
		{
			name: "i2c_sensor",
			text: "make a 3.3V I2C temperature sensor breakout",
			want: []string{`"kind": "sensor_node"`, `"kind": "i2c"`, `"kind": "sensor"`, `"source_hash"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if err := run([]string{"--json", "--text", tc.text, "intent", "draft"}, &stdout, &stderr); err != nil {
				t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}
			output := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Fatalf("expected %q in output:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunIntentDraftWritesArtifacts(t *testing.T) {
	output := filepath.Join(t.TempDir(), "draft")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "USB-C 5V to 3.3V regulator module", "--output", output, "intent", "draft"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	for _, name := range []string{"intent-source.txt", "intent-draft.json", "intent-extraction.json", "intent-clarifications.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}
}

func TestRunIntentDraftStrictBlocksClarification(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--strict", "--text", "battery powered sensor", "intent", "draft"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected strict clarification error")
	}
	if !strings.Contains(stdout.String(), "battery chemistry") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunIntentPlanWritesArtifacts(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "intent.json")
	output := filepath.Join(dir, "out")
	request := `{
  "version": "0.1.0",
  "name": "intent_cli",
  "kind": "breakout",
  "board": {"width_mm": 40, "height_mm": 25, "layers": 2},
  "power": {"inputs": [{"kind": "usb_c", "voltage": "5V"}], "rails": [{"name": "VCC", "voltage": "3.3V"}]},
  "interfaces": [{"kind": "i2c", "voltage": "3.3V"}],
  "functions": [{"kind": "sensor", "family": "i2c_sensor"}]
}`
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "intent", "plan"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range []string{"intent-plan.json", "generated-request.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}
}

func TestRunIntentPlanIncludesSynthesisTrace(t *testing.T) {
	requestPath := filepath.Join("..", "..", "examples", "intent", "synthesis_mcu_i2c_explicit_supply.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--json", "--request", requestPath, "intent", "plan"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"synthesis"`, `"voltage_domain"`, `"bus_resolution"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestRunIntentPlanReportsSynthesisBlockedSupply(t *testing.T) {
	requestPath := filepath.Join("..", "..", "examples", "intent", "synthesis_unknown_supply_blocked.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", requestPath, "intent", "plan"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected blocked supply error")
	}
	output := stdout.String()
	for _, want := range []string{`"status": "blocked"`, "unknown supply alias", `"voltage_domain"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestRunIntentCreateRequiresOutput(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "intent.json")
	if err := os.WriteFile(requestPath, []byte(`{"version":"0.1.0","name":"missing_output","board":{"width_mm":10,"height_mm":10,"layers":2}}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", requestPath, "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing output error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || len(result.Issues) != 1 || result.Issues[0].Path != "output" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunIntentCreateReportsBlockedPlan(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "intent.json")
	output := filepath.Join(dir, "out")
	request := `{
  "version": "0.1.0",
  "name": "blocked_intent",
  "kind": "breakout",
  "board": {"width_mm": 40, "height_mm": 25, "layers": 2},
  "functions": [{"kind": "sensor", "family": "rf_sensor", "strength": "required"}]
}`
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", requestPath, "--output", output, "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected blocked intent error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || len(result.Issues) == 0 {
		t.Fatalf("result = %#v", result)
	}
	var payload struct {
		Data struct {
			AIStatus aiLaneStatus `json:"ai_status"`
		} `json:"data"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode ai status: %v\n%s", decodeErr, stdout.String())
	}
	if payload.Data.AIStatus.Status != aiLaneStatusUnsupported {
		t.Fatalf("ai status = %#v, want unsupported", payload.Data.AIStatus)
	}
	if payload.Data.AIStatus.Stage != "plan" || payload.Data.AIStatus.RetryAllowed {
		t.Fatalf("ai status should point to non-retryable plan blocker: %#v", payload.Data.AIStatus)
	}
}

func TestAILaneStatusMapping(t *testing.T) {
	t.Run("needs clarification", func(t *testing.T) {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "power.voltage", Message: "voltage is ambiguous"}
		status := buildAILaneStatus(intentplanner.PlanResult{Status: intentplanner.PlanStatusNeedsClarification}, nil, []reports.Issue{issue}, nil)
		if status.Status != aiLaneStatusNeedsClarification || !status.UserClarificationRequired {
			t.Fatalf("status = %#v, want needs_clarification with user prompt", status)
		}
		if status.RetryKey == "" {
			t.Fatalf("retry key is empty: %#v", status)
		}
	})
	t.Run("routing retry", func(t *testing.T) {
		issue := reports.Issue{Code: reports.CodeRouteGraphIncomplete, Severity: reports.SeverityError, Path: "routing.SDA", Message: "route graph incomplete", Nets: []string{"SDA"}}
		workflow := designworkflow.WorkflowResult{Stages: []designworkflow.StageResult{{
			Name:   designworkflow.StageRouting,
			Status: designworkflow.StageStatusBlocked,
			Issues: []reports.Issue{issue},
		}}}
		status := buildAILaneStatus(intentplanner.PlanResult{Status: intentplanner.PlanStatusReady}, &workflow, []reports.Issue{issue}, nil)
		if status.Status != aiLaneStatusBlocked || status.Stage != "routing" || !status.RetryAllowed || status.MaxAutomaticRetryAttempts != 1 {
			t.Fatalf("status = %#v, want retryable routing blocker", status)
		}
		if status.RepairCategory != repair.CategoryUnroutedNet {
			t.Fatalf("repair category = %q, want %q", status.RepairCategory, repair.CategoryUnroutedNet)
		}
	})
	t.Run("repair bundle guidance", func(t *testing.T) {
		issue := reports.Issue{Code: reports.CodePlacementOutsideBoard, Severity: reports.SeverityError, Path: "components.U1.position", Message: "fixed placement is outside usable board area", Refs: []string{"U1"}}
		workflow := designworkflow.WorkflowResult{Stages: []designworkflow.StageResult{{
			Name:   designworkflow.StagePlacement,
			Status: designworkflow.StageStatusBlocked,
			Issues: []reports.Issue{issue},
		}}}
		artifacts := []reports.Artifact{{Path: "project/.kicadai/repair-bundle.json"}}
		status := buildAILaneStatus(intentplanner.PlanResult{Status: intentplanner.PlanStatusReady}, &workflow, []reports.Issue{issue}, artifacts)
		if status.Status != aiLaneStatusBlocked || status.Stage != "placement" || !status.RetryAllowed {
			t.Fatalf("status = %#v, want retryable placement blocker", status)
		}
		if status.RepairCategory != repair.CategoryPlacementOutside {
			t.Fatalf("repair category = %q, want %q", status.RepairCategory, repair.CategoryPlacementOutside)
		}
		if status.RepairBundlePath != "project/.kicadai/repair-bundle.json" {
			t.Fatalf("repair bundle path = %q", status.RepairBundlePath)
		}
		if !strings.Contains(status.SuggestedNextAction, "rerun validation") {
			t.Fatalf("next action = %q", status.SuggestedNextAction)
		}
	})
	t.Run("candidate warning", func(t *testing.T) {
		workflow := designworkflow.WorkflowResult{Stages: []designworkflow.StageResult{{
			Name:   designworkflow.StageKiCadChecks,
			Status: designworkflow.StageStatusWarning,
		}}}
		status := buildAILaneStatus(intentplanner.PlanResult{Status: intentplanner.PlanStatusReady}, &workflow, nil, []reports.Artifact{{Path: ".kicadai/design-promotion.json"}})
		if status.Status != aiLaneStatusCandidate || status.Stage != "validation" {
			t.Fatalf("status = %#v, want candidate warning", status)
		}
		if len(status.ArtifactPaths) != 1 || status.ArtifactPaths[0] != ".kicadai/design-promotion.json" {
			t.Fatalf("artifact paths = %#v", status.ArtifactPaths)
		}
	})
}

func TestRunIntentCreateFromTextPersistsDraftArtifacts(t *testing.T) {
	output := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a 3.3V I2C temperature sensor breakout", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("intent create failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var payload struct {
		Data *struct {
			AIStatus *aiLaneStatus `json:"ai_status"`
		} `json:"data"`
		Artifacts []reports.Artifact `json:"artifacts"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode result: %v\nstdout=%s", decodeErr, stdout.String())
	}
	if payload.Data == nil || payload.Data.AIStatus == nil {
		t.Fatalf("missing ai_status: %s", stdout.String())
	}
	if payload.Data.AIStatus.Status != aiLaneStatusCandidate || payload.Data.AIStatus.Stage != string(designworkflow.StageValidation) {
		t.Fatalf("ai_status = %#v", payload.Data.AIStatus)
	}
	if !hasCLIArtifact(payload.Artifacts, reports.ArtifactPromotionReport, designworkflow.PromotionReportArtifactPath) {
		t.Fatalf("promotion artifact missing from CLI result: %#v", payload.Artifacts)
	}
	for _, name := range []string{"intent-source.txt", "intent-draft.json", "intent-extraction.json", "intent-clarifications.json", "intent-plan.json", "generated-request.json", "design-request.json", "workflow-result.json", "design-rationale.json", "validation-summary.json", "retry-state.json", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(output, ".kicadai", name)); err != nil {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}
	generatedManifest, status, err := manifest.Read(output)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !status.Present || status.Stale {
		t.Fatalf("manifest status = %#v", status)
	}
	if generatedManifest.AILane == nil || generatedManifest.AILane.Status == "" || generatedManifest.AILane.RetryStatePath != ".kicadai/retry-state.json" {
		t.Fatalf("manifest AI lane summary = %#v", generatedManifest.AILane)
	}
	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	if promotion.AchievedReadiness != designworkflow.PromotionReadinessCandidate || !promotion.MatchesExpectation {
		t.Fatalf("promotion report = %#v", promotion)
	}
}

func TestRunIntentCreateLEDPromptGoldenCandidate(t *testing.T) {
	output := filepath.Join(t.TempDir(), "led_project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a simple LED indicator board", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var payload struct {
		Data *struct {
			AIStatus *aiLaneStatus `json:"ai_status"`
		} `json:"data"`
		Artifacts []reports.Artifact `json:"artifacts"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode result: %v\nstdout=%s", decodeErr, stdout.String())
	}
	if payload.Data == nil || payload.Data.AIStatus == nil {
		t.Fatalf("missing ai_status: %s", stdout.String())
	}
	if payload.Data.AIStatus.Status != aiLaneStatusCandidate || payload.Data.AIStatus.Stage != "validation" {
		t.Fatalf("ai_status = %#v", payload.Data.AIStatus)
	}
	if payload.Data.AIStatus.RetryAllowed {
		t.Fatalf("candidate ai_status unexpectedly allows retry: %#v", payload.Data.AIStatus)
	}
	if payload.Data.AIStatus.MaxAutomaticRetryAttempts != 0 || payload.Data.AIStatus.IssueCode != "" {
		t.Fatalf("candidate ai_status still exposes retry/blocker fields: %#v", payload.Data.AIStatus)
	}
	if !hasCLIArtifact(payload.Artifacts, reports.ArtifactPromotionReport, designworkflow.PromotionReportArtifactPath) {
		t.Fatalf("promotion artifact missing from CLI result: %#v", payload.Artifacts)
	}
	for _, relPath := range []string{
		"led_indicator.kicad_pro",
		"led_indicator.kicad_sch",
		"led_indicator.kicad_pcb",
		".kicadai/design-promotion.json",
		".kicadai/transaction.json",
		".kicadai/validation-summary.json",
		".kicadai/workflow-result.json",
	} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(relPath))); err != nil {
			t.Fatalf("missing generated artifact %s: %v", relPath, err)
		}
	}
	var workflow struct {
		Stages    []workflowEvidenceStage          `json:"stages"`
		Promotion *designworkflow.PromotionSummary `json:"promotion,omitempty"`
		Artifacts []reports.Artifact               `json:"artifacts"`
	}
	readJSONFile(t, filepath.Join(output, ".kicadai", "workflow-result.json"), &workflow)
	wantStages := map[string]string{
		"block_planning":       string(designworkflow.StageStatusOK),
		"schematic":            string(designworkflow.StageStatusOK),
		"schematic_electrical": string(designworkflow.StageStatusOK),
		"pcb_realization":      string(designworkflow.StageStatusOK),
		"placement":            string(designworkflow.StageStatusOK),
		"routing":              string(designworkflow.StageStatusOK),
		"project_write":        string(designworkflow.StageStatusOK),
		"writer_correctness":   string(designworkflow.StageStatusWarning),
		"validation":           string(designworkflow.StageStatusOK),
	}
	for stage, wantStatus := range wantStages {
		if got := workflowStageStatus(workflow.Stages, stage); got != wantStatus {
			t.Fatalf("stage %s status = %q, want %q; stages = %#v", stage, got, wantStatus, workflow.Stages)
		}
	}
	if workflow.Promotion == nil || workflow.Promotion.AchievedReadiness != designworkflow.PromotionReadinessCandidate {
		t.Fatalf("workflow promotion = %#v", workflow.Promotion)
	}
	if stageHasIssue(workflow.Stages, "placement", reports.CodePlacementOutsideBoard) {
		t.Fatalf("placement still reports outside-board blocker: %#v", workflow.Stages)
	}
	if stageHasIssue(workflow.Stages, "routing", reports.CodeValidationFailed) {
		t.Fatalf("routing still reports validation blocker: %#v", workflow.Stages)
	}
	if stageHasIssue(workflow.Stages, "validation", reports.CodeDisconnectedPad) {
		t.Fatalf("validation still reports disconnected pads: %#v", workflow.Stages)
	}
	var status aiLaneStatus
	readJSONFile(t, filepath.Join(output, ".kicadai", "validation-summary.json"), &status)
	if status.Status != aiLaneStatusCandidate || status.Stage != "validation" {
		t.Fatalf("validation summary status = %#v", status)
	}
	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	if promotion.AchievedReadiness != designworkflow.PromotionReadinessCandidate || promotion.Status != designworkflow.PromotionStatusWarn {
		t.Fatalf("promotion report = %#v", promotion)
	}
}

func TestRunIntentCreateLEDPromptStrictPromotionCandidate(t *testing.T) {
	output := filepath.Join(t.TempDir(), "led_project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a simple LED indicator board", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	if promotion.AchievedReadiness != designworkflow.PromotionReadinessCandidate || promotion.Status != designworkflow.PromotionStatusWarn {
		t.Fatalf("promotion report = %#v, want strict candidate with missing-KiCad warning", promotion)
	}

	wantGates := map[string]designworkflow.PromotionGateStatus{
		"connectivity":       designworkflow.PromotionGateStatusPass,
		"kicad_checks":       designworkflow.PromotionGateStatusSkipped,
		"route_completion":   designworkflow.PromotionGateStatusPass,
		"writer_correctness": designworkflow.PromotionGateStatusWarn,
	}
	for gateID, wantStatus := range wantGates {
		gate := promotionGateByName(promotion.Gates, gateID)
		if gate == nil || gate.Status != wantStatus {
			t.Fatalf("promotion gate %s = %#v, want %q", gateID, gate, wantStatus)
		}
	}

	if promotionHasIssue(promotion.Issues, designworkflow.StageValidation, "kicad_drc", "KiCad DRC was not run because no KiCad CLI path was configured") {
		t.Fatalf("optional DRC should not report missing CLI as a validation issue: %#v", promotion.Issues)
	}

	if !promotionHasIssueCodePrefix(promotion.Issues, designworkflow.StageRouting, "routing_fixed_net_skipped_") {
		t.Fatalf("promotion issues missing fixed-net skipped baseline evidence: %#v", promotion.Issues)
	}
	if promotionHasIssue(promotion.Issues, designworkflow.StageRouting, "design.inter_block_route_groups[\"GND\"].branches[0].nets.GND.class", "power or high-current net has no explicit net class") ||
		promotionHasIssue(promotion.Issues, designworkflow.StageRouting, "design.inter_block_route_groups[\"GND\"].branches[1].nets.GND.class", "power or high-current net has no explicit net class") {
		t.Fatalf("missing-net-class warnings should be closed: %#v", promotion.Issues)
	}
	if promotionHasIssue(promotion.Issues, designworkflow.StageWriterCorrect, "schematic_to_pcb", "schematic-to-PCB transfer has no pad net hints") ||
		promotionHasIssue(promotion.Issues, designworkflow.StageWriterCorrect, "library_index", "library index not provided") {
		t.Fatalf("writer pinmap/library-index warnings should be closed: %#v", promotion.Issues)
	}
	if promotionHasIssue(promotion.Issues, designworkflow.StagePlacement, "design.inter_block_routing.connections[0].to", "connection endpoint does not resolve to a generated PCB pad") {
		t.Fatalf("endpoint-to-pad warning should be closed: %#v", promotion.Issues)
	}
	if promotionHasIssue(promotion.Issues, designworkflow.StageComponentSelection, "component_selection.power_header.connector", "block component has no component_id or component_query") ||
		promotionHasIssue(promotion.Issues, designworkflow.StageComponentSelection, "component_selection.connector.connector", "block component has no component_id or component_query") {
		t.Fatalf("component-selection placeholder warnings should be closed: %#v", promotion.Issues)
	}
}

func TestRunIntentCreateLEDPromptFakeKiCadPromotesPass(t *testing.T) {
	output := filepath.Join(t.TempDir(), "led_project")
	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	cli := fakeWorkflowKiCadCLI(t, 0, `{"coordinate_units":"mm","violations":[],"sheets":[]}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--require-erc", "--require-drc", "--keep-artifacts", "--artifact-dir", artifactDir, "--text", "make a simple LED indicator board", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if stdout.Len() == 0 {
		t.Fatalf("run returned no JSON: err=%v stderr=%s", err, stderr.String())
	}

	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	if promotion.AchievedReadiness != designworkflow.PromotionReadinessPass || promotion.Status != designworkflow.PromotionStatusPass {
		t.Fatalf("promotion report = %#v, want pass", promotion)
	}
	if promotion.KiCadVersion != "10.0.0" || !strings.Contains(promotion.ExternalEvidence, cli) {
		t.Fatalf("KiCad evidence = version %q external %q, want fake CLI path and version", promotion.KiCadVersion, promotion.ExternalEvidence)
	}
	kicadGate := promotionGateByName(promotion.Gates, "kicad_checks")
	if kicadGate == nil || kicadGate.Status != designworkflow.PromotionGateStatusPass {
		t.Fatalf("kicad_checks gate = %#v, want pass", kicadGate)
	}
	if len(kicadGate.RequiredFor) != 2 {
		t.Fatalf("kicad_checks required_for = %#v, want candidate/pass", kicadGate.RequiredFor)
	}
	if len(kicadGate.Artifacts) != 2 {
		t.Fatalf("kicad artifacts = %#v, want ERC and DRC reports", kicadGate.Artifacts)
	}
	writerGate := promotionGateByName(promotion.Gates, "writer_correctness")
	if writerGate == nil || writerGate.Status != designworkflow.PromotionGateStatusPass {
		t.Fatalf("writer gate = %#v, want pass with supplied KiCad CLI", writerGate)
	}
}

func TestRunIntentCreateLEDPromptFakeKiCadFindingsBlockPromotion(t *testing.T) {
	output := filepath.Join(t.TempDir(), "led_project")
	cli := fakeWorkflowKiCadCLI(t, 1, `{"coordinate_units":"mm","violations":[{"description":"clearance violation","severity":"error","type":"clearance","items":[{"description":"Track / Pad","uuid":"11111111-1111-4111-8111-111111111111"}]}],"sheets":[]}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--require-erc", "--require-drc", "--text", "make a simple LED indicator board", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected KiCad finding to report issues")
	}
	if stdout.Len() == 0 {
		t.Fatalf("run returned no JSON: err=%v stderr=%s", err, stderr.String())
	}

	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	if promotion.AchievedReadiness != designworkflow.PromotionReadinessBlocked || promotion.Status != designworkflow.PromotionStatusFailed {
		t.Fatalf("promotion report = %#v, want blocked failed", promotion)
	}
	kicadGate := promotionGateByName(promotion.Gates, "kicad_checks")
	if kicadGate == nil || kicadGate.Status != designworkflow.PromotionGateStatusFailed || len(kicadGate.IssueCodes) == 0 {
		t.Fatalf("kicad_checks gate = %#v, want failed with issue codes", kicadGate)
	}
	if !promotionHasIssue(promotion.Issues, designworkflow.StageKiCadChecks, "", "clearance violation") {
		t.Fatalf("promotion issues missing KiCad DRC finding: %#v", promotion.Issues)
	}
}

func TestRunIntentCreateLEDPromptOptionalKiCadSmoke(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	if cliPath == "" {
		t.Skipf("set %s to run optional KiCad-backed LED prompt smoke", checks.EnvKiCadCLI)
	}
	output := filepath.Join(t.TempDir(), "led_project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cliPath, "--require-erc", "--require-drc", "--text", "make a simple LED indicator board", "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if stdout.Len() == 0 {
		t.Fatalf("run returned no JSON: err=%v stderr=%s", err, stderr.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data *struct {
			AIStatus *aiLaneStatus `json:"ai_status"`
		} `json:"data"`
		Issues []reports.Issue `json:"issues"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode result: %v\nstdout=%s\nstderr=%s", decodeErr, stdout.String(), stderr.String())
	}
	if err != nil && len(payload.Issues) == 0 {
		t.Fatalf("run returned error without structured issues: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !payload.OK && len(payload.Issues) == 0 {
		t.Fatalf("run reported failure without structured issues: stdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if payload.Data == nil || payload.Data.AIStatus == nil {
		t.Fatalf("missing ai_status: %s", stdout.String())
	}
	if payload.Data.AIStatus.Status == aiLaneStatusToolError && payload.Data.AIStatus.IssueCode == reports.CodeSkippedExternalTool {
		t.Fatalf("KiCad smoke skipped despite configured %s=%s: %#v", checks.EnvKiCadCLI, cliPath, payload.Data.AIStatus)
	}
	var workflow struct {
		Stages []workflowEvidenceStage `json:"stages"`
	}
	readJSONFile(t, filepath.Join(output, ".kicadai", "workflow-result.json"), &workflow)
	kicadStage := workflowStageByName(workflow.Stages, string(designworkflow.StageKiCadChecks))
	if kicadStage == nil || kicadStage.Status == string(designworkflow.StageStatusSkipped) {
		t.Fatalf("kicad_checks stage = %#v, want KiCad-backed evidence", kicadStage)
	}
	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, ".kicadai", "design-promotion.json"), &promotion)
	gate := promotionGateByName(promotion.Gates, string(designworkflow.StageKiCadChecks))
	if gate == nil || gate.Status == designworkflow.PromotionGateStatusSkipped || gate.Status == designworkflow.PromotionGateStatusNotRun {
		t.Fatalf("promotion kicad_checks gate = %#v", gate)
	}
	if gate.Status == designworkflow.PromotionGateStatusPass {
		if promotion.AchievedReadiness != designworkflow.PromotionReadinessPass || promotion.Status != designworkflow.PromotionStatusPass {
			t.Fatalf("clean KiCad gate did not promote to pass: %#v", promotion)
		}
		if promotion.KiCadVersion == "" || promotion.ExternalEvidence == "" || len(gate.Artifacts) == 0 {
			t.Fatalf("promotion missing KiCad evidence: promotion=%#v gate=%#v", promotion, gate)
		}
	} else if len(gate.IssueCodes) == 0 {
		t.Fatalf("dirty KiCad gate lacks precise issue codes: %#v", gate)
	}
}

func TestRunIntentCreateSensorBreakoutPersistsRegulatorEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "sensor_breakout")
	requestPath := filepath.Join("..", "..", "examples", "intent", "sensor_breakout.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "--skip-routing", "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected downstream workflow validation to report issues for this fixture")
	}
	if !strings.Contains(err.Error(), "intent create reported issues") {
		t.Fatalf("unexpected intent create error: %v\nstdout_bytes=%d\nstderr=%s", err, stdout.Len(), stderr.String())
	}
	t.Logf("intent create returned expected error: %v; stdout_bytes=%d; stderr=%s", err, stdout.Len(), stderr.String())

	workflowPath := filepath.Join(output, ".kicadai", "workflow-result.json")
	var workflow struct {
		Stages []workflowEvidenceStage `json:"stages"`
	}
	readJSONFile(t, workflowPath, &workflow)
	if !workflowSelectedComponent(workflow.Stages, "regulator", "regulator", "regulator.linear.ap2112k_3v3.sot23_5") {
		t.Fatalf("workflow missing regulator selection: %#v", workflow.Stages)
	}
	if !workflowSelectedComponent(workflow.Stages, "regulator", "input_capacitor", "capacitor.murata.grm21br61a106ke19l.0805") {
		t.Fatalf("workflow missing input capacitor selection: %#v", workflow.Stages)
	}
	if !workflowSelectedComponent(workflow.Stages, "regulator", "output_capacitor", "capacitor.murata.grm21br61a106ke19l.0805") {
		t.Fatalf("workflow missing output capacitor selection: %#v", workflow.Stages)
	}

	generatedPath := filepath.Join(output, ".kicadai", "generated-request.json")
	var generated struct {
		Blocks          []generatedBlockEvidence `json:"blocks"`
		ComponentPolicy struct {
			Overrides map[string]componentEvidenceOverride `json:"overrides"`
		} `json:"component_policy"`
	}
	readJSONFile(t, generatedPath, &generated)
	if got := generatedBlockParam(generated.Blocks, "voltage_regulator", "regulator_symbol"); got != "Regulator_Linear:AP2112K-3.3" {
		t.Fatalf("generated request regulator_symbol = %q; blocks=%#v", got, generated.Blocks)
	}
	if got := generatedBlockParam(generated.Blocks, "voltage_regulator", "enable_mode"); got != "tied_input" {
		t.Fatalf("generated request enable_mode = %q; blocks=%#v", got, generated.Blocks)
	}
	if !generatedRequiredRating(generated.ComponentPolicy.Overrides, "regulator.regulator", "output_current", "0.1", "A") {
		t.Fatalf("generated request missing regulator current rating: %#v", generated.ComponentPolicy.Overrides)
	}
	if !generatedRequiredRating(generated.ComponentPolicy.Overrides, "regulator.input_capacitor", "voltage", "6.3", "V") {
		t.Fatalf("generated request missing input capacitor voltage rating: %#v", generated.ComponentPolicy.Overrides)
	}
}

func TestRunIntentPlanRegulatorEvidenceFixtures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		wantStatus string
		want       string
		wantAbsent string
	}{
		{
			name:       "ap2112k",
			path:       filepath.Join("..", "..", "examples", "intent", "regulator_ap2112k_sensor.json"),
			wantStatus: "ready",
			want:       "ap2112k_3v3_sot23_5",
		},
		{
			name:       "high_current_fallback",
			path:       filepath.Join("..", "..", "examples", "intent", "regulator_high_current_fallback.json"),
			wantStatus: "partial",
			wantAbsent: "Regulator_Linear:AP2112K-3.3",
		},
		{
			name:       "insufficient_headroom",
			path:       filepath.Join("..", "..", "examples", "intent", "regulator_insufficient_headroom_blocked.json"),
			wantStatus: "blocked",
			want:       "regulator input voltage lacks modeled dropout margin",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := run([]string{"--json", "--request", tc.path, "intent", "plan"}, &stdout, &stderr)
			if tc.wantStatus == "blocked" {
				if err == nil {
					t.Fatal("expected blocked fixture to return an error")
				}
			} else if err != nil {
				t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}
			output := stdout.String()
			var payload struct {
				Data struct {
					Status string `json:"status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("parse output: %v\nstdout=%s", err, output)
			}
			if payload.Data.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q; output=%s", payload.Data.Status, tc.wantStatus, output)
			}
			if tc.want != "" && !strings.Contains(output, tc.want) {
				t.Fatalf("missing %q in output=%s", tc.want, output)
			}
			if tc.wantAbsent != "" && strings.Contains(output, tc.wantAbsent) {
				t.Fatalf("unexpected %q in output=%s", tc.wantAbsent, output)
			}
		})
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("parse %s: %v\n%s", path, err, string(body))
	}
}

type workflowEvidenceStage struct {
	Name    string          `json:"name"`
	Status  string          `json:"status"`
	Summary map[string]any  `json:"summary"`
	Issues  []reports.Issue `json:"issues"`
}

func workflowStageStatus(stages []workflowEvidenceStage, name string) string {
	stage := workflowStageByName(stages, name)
	if stage == nil {
		return ""
	}
	return stage.Status
}

func workflowStageByName(stages []workflowEvidenceStage, name string) *workflowEvidenceStage {
	for index := range stages {
		if stages[index].Name == name {
			return &stages[index]
		}
	}
	return nil
}

func promotionGateByName(gates []designworkflow.PromotionGate, name string) *designworkflow.PromotionGate {
	for index := range gates {
		if gates[index].ID == name {
			return &gates[index]
		}
	}
	return nil
}

func promotionHasIssue(issues []designworkflow.PromotionIssue, stage designworkflow.StageName, path string, message string) bool {
	for _, issue := range issues {
		if issue.Stage == stage && issue.Path == path && strings.Contains(issue.Message, message) {
			return true
		}
	}
	return false
}

func promotionHasIssueCodePrefix(issues []designworkflow.PromotionIssue, stage designworkflow.StageName, codePrefix string) bool {
	for _, issue := range issues {
		if issue.Stage == stage && strings.HasPrefix(issue.Code, codePrefix) {
			return true
		}
	}
	return false
}

func stageHasIssue(stages []workflowEvidenceStage, name string, code reports.Code) bool {
	for _, stage := range stages {
		if stage.Name != name {
			continue
		}
		for _, issue := range stage.Issues {
			if issue.Code == code {
				return true
			}
		}
	}
	return false
}

func stageHasIssueForNet(stages []workflowEvidenceStage, name string, code reports.Code, net string, message string) bool {
	for _, stage := range stages {
		if stage.Name != name {
			continue
		}
		for _, issue := range stage.Issues {
			if issue.Code == code && strings.Contains(issue.Message, message) && slices.Contains(issue.Nets, net) {
				return true
			}
		}
	}
	return false
}

type componentEvidenceOverride struct {
	RequiredRatings []componentEvidenceRating `json:"required_ratings"`
}

type componentEvidenceRating struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type generatedBlockEvidence struct {
	ID      string         `json:"id"`
	BlockID string         `json:"block_id"`
	Params  map[string]any `json:"params"`
}

func workflowSelectedComponent(stages []workflowEvidenceStage, instanceID string, role string, componentID string) bool {
	for _, stage := range stages {
		if stage.Name != "component_selection" {
			continue
		}
		selected, ok := stage.Summary["selected_components"].([]any)
		if !ok {
			continue
		}
		for _, item := range selected {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if entry["instance_id"] == instanceID && entry["role"] == role && entry["component_id"] == componentID {
				return true
			}
		}
	}
	return false
}

func generatedRequiredRating(overrides map[string]componentEvidenceOverride, key string, kind string, value string, unit string) bool {
	override, ok := overrides[key]
	if !ok {
		return false
	}
	for _, rating := range override.RequiredRatings {
		if rating.Kind == kind && rating.Value == value && rating.Unit == unit {
			return true
		}
	}
	return false
}

func generatedBlockParam(blocks []generatedBlockEvidence, blockID string, key string) string {
	for _, block := range blocks {
		if block.BlockID != blockID {
			continue
		}
		if value, ok := block.Params[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func TestRunIntentCreateTextBlocksClarification(t *testing.T) {
	output := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "battery powered sensor", "--output", output, "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected clarification error")
	}
	if !strings.Contains(stdout.String(), "battery chemistry") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunIntentCreateHighVoltagePromptBlocksBeforeGeneration(t *testing.T) {
	output := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a mains powered LED board", "--output", output, "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected high-voltage prompt to block")
	}
	var payload struct {
		Data struct {
			AIStatus *aiLaneStatus `json:"ai_status"`
		} `json:"data"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode result: %v\nstdout=%s", decodeErr, stdout.String())
	}
	if payload.Data.AIStatus == nil {
		t.Fatalf("missing ai_status: %s", stdout.String())
	}
	if payload.Data.AIStatus.Status != aiLaneStatusNeedsClarification || payload.Data.AIStatus.Stage != "plan" || !payload.Data.AIStatus.UserClarificationRequired {
		t.Fatalf("ai_status = %#v", payload.Data.AIStatus)
	}
	if !strings.Contains(stdout.String(), "Mains and high-voltage designs are not supported") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunIntentExplainReportsSemanticEvidence(t *testing.T) {
	requestPath := filepath.Join("..", "..", "examples", "intent", "mcu_i2c_sensor.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--json", "--request", requestPath, "intent", "explain"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"I2C1_SDA", "supply:regulator.VOUT"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
}

func TestRunIntentExplainFromText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a 3.3V I2C temperature sensor breakout", "intent", "explain"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"draft"`, `"selected_blocks"`, `"i2c_sensor"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestRunIntentExplainTextBlocksClarification(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "battery powered sensor", "intent", "explain"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected clarification error")
	}
	if !strings.Contains(stdout.String(), "battery_voltage_missing") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunIntentRationaleFromText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "make a 3.3V I2C temperature sensor breakout", "intent", "rationale"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"schema": "kicadai.design.rationale.v1"`, `"source_hash"`, `"decisions"`, `"evidence"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestRunIntentRationaleTextBlocksClarification(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--text", "battery powered sensor", "intent", "rationale"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected clarification error")
	}
	for _, want := range []string{`"status": "needs_clarification"`, "battery chemistry"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, stdout.String())
		}
	}
}

func TestRunIntentRationaleWritesOutput(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "intent.json")
	output := filepath.Join(dir, "out")
	request := `{
  "version": "0.1.0",
  "name": "rationale_cli",
  "kind": "breakout",
  "board": {"width_mm": 40, "height_mm": 25, "layers": 2},
  "power": {"inputs": [{"kind": "usb_c", "voltage": "5V"}], "rails": [{"name": "VCC", "voltage": "3.3V"}]},
  "interfaces": [{"kind": "i2c", "voltage": "3.3V"}],
  "functions": [{"kind": "sensor", "family": "i2c_sensor"}]
}`
	if err := os.WriteFile(requestPath, []byte(request), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--json", "--request", requestPath, "--output", output, "intent", "rationale"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(output, "design-rationale.json")); err != nil {
		t.Fatalf("missing rationale artifact: %v", err)
	}
}

func TestRunIntentRationaleRejectsSourceConflict(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", "request.json", "--text", "make a board", "intent", "rationale"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected source conflict error")
	}
	if !strings.Contains(stdout.String(), "provide exactly one") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunIntentRationaleFromTarget(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, ".kicadai")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join("..", "..", "examples", "intent", "sensor_breakout.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--json", "--request", requestPath, "--output", meta, "--overwrite", "intent", "plan"}, &stdout, &stderr); err != nil {
		t.Fatalf("plan returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"--json", "--target", dir, "intent", "rationale"}, &stdout, &stderr); err != nil {
		t.Fatalf("rationale returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(meta, "design-rationale.json")); err != nil {
		t.Fatalf("missing target rationale artifact: %v", err)
	}
}

func TestRunIntentCreateBlocksAmbiguousSemanticTarget(t *testing.T) {
	requestPath := filepath.Join("..", "..", "examples", "intent", "multi_mcu_ambiguous_support.json")
	output := filepath.Join(t.TempDir(), "ambiguous")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "intent", "create"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected blocked intent create")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || !strings.Contains(stdout.String(), "multiple compatible mcu targets") {
		t.Fatalf("result = %#v\n%s", result, stdout.String())
	}
}

func TestRunDesignCreateRetrySummarySnapshot(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantRetry     bool
		wantAttempts  int
		wantStop      string
		wantStageStat string
	}{
		{name: "disabled", fixture: "disabled", wantStageStat: "skipped"},
		{name: "enabled", fixture: "non_improving", wantRetry: true, wantAttempts: 1, wantStop: "routed", wantStageStat: "ok"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestPath := filepath.Join("..", "..", "internal", "designworkflow", "testdata", "retry", tc.fixture, "request.json")
			output := filepath.Join(t.TempDir(), tc.fixture)
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			if err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "design", "create"}, &stdout, &stderr); err != nil {
				t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}
			var result cliRetrySnapshot
			if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
				t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
			}
			gotOutputInfo, gotOutputErr := os.Stat(result.Data.Project.OutputDir)
			wantOutputInfo, wantOutputErr := os.Stat(output)
			samePath := false
			if gotOutputErr == nil && wantOutputErr == nil {
				samePath = os.SameFile(gotOutputInfo, wantOutputInfo)
			} else {
				samePath = filepath.Clean(result.Data.Project.OutputDir) == filepath.Clean(output)
			}
			if !samePath {
				t.Fatalf("output_dir = %q, want %q", result.Data.Project.OutputDir, output)
			}
			routingStage := cliRetryStageByName(t, result.Data.Stages, "routing")
			if routingStage.Status != tc.wantStageStat {
				t.Fatalf("routing stage = %#v", routingStage)
			}
			retry, hasRetry := routingStage.Summary["routing_retry"].(map[string]any)
			if hasRetry != tc.wantRetry {
				t.Fatalf("routing_retry present = %v, want %v in %#v", hasRetry, tc.wantRetry, routingStage.Summary)
			}
			if !tc.wantRetry {
				return
			}
			enabled, enabledOK := retry["enabled"].(bool)
			attempts, attemptsOK := retry["attempts"].(float64)
			stopReason, stopOK := retry["stop_reason"].(string)
			if !enabledOK || !enabled || !attemptsOK || int(attempts) != tc.wantAttempts || !stopOK || stopReason != tc.wantStop {
				t.Fatalf("retry summary = %#v", retry)
			}
		})
	}
}

func TestRunDesignCreateFullBoardRetryEvidenceSnapshot(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "designworkflow", "testdata", "full_board_retry", "generated_led_connectivity", "request.json")
	output := filepath.Join(t.TempDir(), "generated_led_connectivity")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "design", "create"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result cliRetrySnapshot
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	placementStage := cliRetryStageByName(t, result.Data.Stages, "placement")
	assertCLIPadHydrationSummary(t, placementStage, 2, 2, 4)
	assertCLINestedSummaryNumber(t, placementStage, "mobility", "eligible_count", 2)
	assertCLINestedSummaryNumber(t, placementStage, "mobility", "group_transform_count", 2)
	routingStage := cliRetryStageByName(t, result.Data.Stages, "routing")
	if routingStage.Status != "ok" {
		t.Fatalf("routing stage status = %q, want ok", routingStage.Status)
	}
	assertCLINestedSummaryNumber(t, routingStage, "local_route_mobility", "total", 1)
	assertCLINestedSummaryNumber(t, routingStage, "local_route_mobility", "transformable", 1)
	retry, ok := routingStage.Summary["routing_retry"].(map[string]any)
	if !ok {
		t.Fatalf("routing retry summary missing: %#v", routingStage.Summary)
	}
	if enabled, ok := retry["enabled"].(bool); !ok || !enabled {
		t.Fatalf("retry enabled = %#v", retry["enabled"])
	}
	if attempts, ok := retry["attempts"].(float64); !ok || attempts != 1 {
		t.Fatalf("retry attempts = %#v", retry["attempts"])
	}
	if applied, ok := retry["applied"].(float64); !ok || applied != 0 {
		t.Fatalf("retry applied = %#v", retry["applied"])
	}
	if stop, ok := retry["stop_reason"].(string); !ok || stop != "routed" {
		t.Fatalf("retry stop = %#v", retry["stop_reason"])
	}
	if selectedAttempt, ok := retry["selected_attempt"].(float64); !ok || selectedAttempt != 1 {
		t.Fatalf("selected attempt = %#v", retry["selected_attempt"])
	}
	if selectedReason, ok := retry["selected_reason"].(string); !ok || selectedReason == "" {
		t.Fatalf("selected reason = %#v", retry["selected_reason"])
	}
	history, hasHistory := retry["attempt_history"].([]any)
	if !hasHistory || len(history) != 1 {
		t.Fatalf("retry attempt history = %#v, want baseline attempt only", retry["attempt_history"])
	}
	first, ok := history[0].(map[string]any)
	if !ok || first["attempt"] != float64(1) || first["selected"] != true {
		t.Fatalf("retry baseline attempt = %#v", history[0])
	}
	if status, ok := first["drc_status"].(string); !ok || status != "skipped" {
		t.Fatalf("baseline DRC status = %#v", first["drc_status"])
	}
	if source, ok := first["drc_source"].(string); !ok || source != "skipped" {
		t.Fatalf("baseline DRC source = %#v", first["drc_source"])
	}
	assertCLINestedSummaryNumber(t, routingStage, "route_connectivity", "endpoint_contacts_proven", 2)
	if cliRetryStageHasIssue(routingStage, "footprint pad summaries") {
		t.Fatalf("routing stage still has pad-summary issue: %#v", routingStage.Issues)
	}
}

func TestRunDesignCreateWritesPromotionReport(t *testing.T) {
	requestPath := filepath.Join("..", "..", "internal", "designworkflow", "testdata", "retry", "disabled", "request.json")
	output := filepath.Join(t.TempDir(), "promotion")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "--request", requestPath, "--output", output, "--overwrite", "design", "create"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		Data struct {
			Promotion *designworkflow.PromotionSummary `json:"promotion,omitempty"`
		} `json:"data"`
		Artifacts []reports.Artifact `json:"artifacts"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.Data.Promotion == nil {
		t.Fatalf("missing promotion summary in %s", stdout.String())
	}
	if result.Data.Promotion.ReportPath != designworkflow.PromotionReportArtifactPath {
		t.Fatalf("promotion report path = %q, want %q", result.Data.Promotion.ReportPath, designworkflow.PromotionReportArtifactPath)
	}
	promotionPath := filepath.Join(output, filepath.FromSlash(designworkflow.PromotionReportArtifactPath))
	if _, err := os.Stat(promotionPath); err != nil {
		t.Fatalf("missing promotion report artifact: %v", err)
	}
	if !hasCLIArtifact(result.Artifacts, reports.ArtifactPromotionReport, designworkflow.PromotionReportArtifactPath) {
		t.Fatalf("promotion artifact missing from result artifacts: %#v", result.Artifacts)
	}
}

func assertCLINestedSummaryNumber(t *testing.T, stage cliRetryStageSnapshot, summaryKey string, field string, want float64) {
	t.Helper()
	raw, ok := stage.Summary[summaryKey]
	if !ok {
		t.Fatalf("%s summary missing: %#v", summaryKey, stage.Summary)
	}
	summary, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s summary has type %T: %#v", summaryKey, raw, raw)
	}
	rawValue, ok := summary[field]
	if !ok {
		t.Fatalf("%s.%s is missing: %#v", summaryKey, field, summary)
	}
	value, ok := rawValue.(float64)
	if !ok {
		t.Fatalf("%s.%s has type %T: %#v, want float64", summaryKey, field, rawValue, rawValue)
	}
	if value != want {
		t.Fatalf("%s.%s = %v, want %v", summaryKey, field, value, want)
	}
}

func assertCLIPadHydrationSummary(t *testing.T, stage cliRetryStageSnapshot, wantHydrated float64, wantTemplates float64, wantPads float64) {
	t.Helper()
	hydrationValue, hasHydration := stage.Summary["pad_hydration"]
	if !hasHydration {
		t.Fatalf("pad hydration summary missing: %#v", stage.Summary)
	}
	hydration, hydrationOK := hydrationValue.(map[string]any)
	if !hydrationOK {
		t.Fatalf("pad hydration summary has type %T: %#v", hydrationValue, hydrationValue)
	}
	hydratedValue, hasHydrated := hydration["hydrated_components"]
	if !hasHydrated {
		t.Fatalf("hydrated_components missing: %#v", hydration)
	}
	hydrated, hydratedOK := hydratedValue.(float64)
	if !hydratedOK || hydrated != wantHydrated {
		t.Fatalf("hydrated_components = %#v, want %v", hydratedValue, wantHydrated)
	}
	sourceValue, hasSources := hydration["source_counts"]
	if !hasSources {
		t.Fatalf("source_counts missing: %#v", hydration)
	}
	sources, sourcesOK := sourceValue.(map[string]any)
	if !sourcesOK {
		t.Fatalf("source_counts has type %T: %#v", sourceValue, sourceValue)
	}
	templateCount, templateOK := sources["verified_template"].(float64)
	if !templateOK || templateCount != wantTemplates {
		t.Fatalf("verified_template source count = %#v, want %v", sources["verified_template"], wantTemplates)
	}
	padCount, padOK := hydration["pad_count"].(float64)
	if !padOK || padCount != wantPads {
		t.Fatalf("pad_count = %#v, want %v", hydration["pad_count"], wantPads)
	}
}

type cliRetrySnapshot struct {
	Data struct {
		Project struct {
			OutputDir string `json:"output_dir"`
		} `json:"project"`
		Stages []cliRetryStageSnapshot `json:"stages"`
	} `json:"data"`
}

type cliRetryStageSnapshot struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Summary map[string]any `json:"summary"`
	Issues  []struct {
		Message string `json:"message"`
	} `json:"issues,omitempty"`
}

func cliRetryStageByName(t *testing.T, stages []cliRetryStageSnapshot, name string) cliRetryStageSnapshot {
	t.Helper()
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}
	var names []string
	for _, stage := range stages {
		names = append(names, stage.Name)
	}
	t.Fatalf("missing stage %q; got stages %v", name, names)
	return cliRetryStageSnapshot{}
}

func cliRetryStageHasIssue(stage cliRetryStageSnapshot, text string) bool {
	for _, issue := range stage.Issues {
		if strings.Contains(issue.Message, text) {
			return true
		}
	}
	return false
}

func hasCLIArtifact(artifacts []reports.Artifact, kind reports.ArtifactKind, path string) bool {
	for _, artifact := range artifacts {
		if artifact.Kind == kind && artifact.Path == path {
			return true
		}
	}
	return false
}

func TestRunComponentListJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "component", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"records"`) || !strings.Contains(stdout.String(), `"resistor.generic.0805"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunComponentFindJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--family", "connector", "--package", "1x04", "component", "find"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"connector.pinheader.1x04.2_54mm"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunComponentSelectBlocksUnsafePlaceholder(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testUnsafeComponentCatalogDir(t), "--family", "opamp", "--acceptance", "connectivity", "component", "select"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected unsafe placeholder selection to fail")
	}
	if !strings.Contains(stdout.String(), string("COMPONENT_UNSAFE_CONFIDENCE")) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunComponentSelectRegulatorRequestJSON(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "regulator-select.json")
	if err := os.WriteFile(requestPath, []byte(`{
  "query": {
    "family": "regulator",
    "package": "sot223",
    "value_kind": "output_voltage",
    "value": "3.3"
  },
  "acceptance": "connectivity",
  "require_concrete": true,
  "require_companions": true,
  "required_ratings": [
    {"kind": "input_voltage", "value": "5", "unit": "V"},
    {"kind": "output_current", "value": "250", "unit": "mA"}
  ]
}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--request", requestPath, "component", "select"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var payload struct {
		Data struct {
			Candidate struct {
				ComponentID string `json:"component_id"`
			} `json:"candidate"`
			Component struct {
				Companions []componentSelectCompanion `json:"companions"`
			} `json:"component"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("parse output: %v\nstdout=%s", err, stdout.String())
	}
	if payload.Data.Candidate.ComponentID != "regulator.linear.ams1117_3v3.sot223" {
		t.Fatalf("selected component ID = %q", payload.Data.Candidate.ComponentID)
	}
	if !cliCompanionsIncludeRole(payload.Data.Component.Companions, "input_capacitor") {
		t.Fatalf("missing input capacitor companion: %+v", payload.Data.Component.Companions)
	}
	if !cliCompanionsIncludeRole(payload.Data.Component.Companions, "output_capacitor") {
		t.Fatalf("missing output capacitor companion: %+v", payload.Data.Component.Companions)
	}
}

func TestRunComponentSelectWithSourceEvidence(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "regulator-select.json")
	writeTestFile(t, requestPath, `{
  "query": {
    "family": "regulator",
    "package": "sot23_5",
    "value_kind": "output_voltage",
    "value": "3.3"
  },
  "acceptance": "connectivity",
  "required_ratings": [
    {"kind": "input_voltage", "value": "5", "unit": "V"}
  ]
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--source-dir", componentSourceFixtureDir(t, "valid"), "--request", requestPath, "component", "select"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"lifecycle_status": "active"`) || !strings.Contains(stdout.String(), `"source_id": "curated_seed_procurement"`) {
		t.Fatalf("missing procurement evidence: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"regulator_evidence"`) || !strings.Contains(stdout.String(), `"kind": "ceramic_stable"`) {
		t.Fatalf("missing AP2112K regulator stability evidence: %s", stdout.String())
	}
}

func TestRunComponentSelectBlocksAMS1117FabricationCandidateOnStabilityEvidence(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "regulator-select.json")
	writeTestFile(t, requestPath, `{
  "query": {
    "family": "regulator",
    "package": "sot223",
    "value_kind": "output_voltage",
    "value": "3.3"
  },
  "acceptance": "fabrication_candidate",
  "require_concrete": true,
  "require_companions": true,
  "required_ratings": [
    {"kind": "input_voltage", "value": "5", "unit": "V"},
    {"kind": "output_current", "value": "100", "unit": "mA"}
  ]
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--request", requestPath, "component", "select"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected AMS1117 fabrication-candidate selection to fail")
	}
	if !strings.Contains(stdout.String(), `"component.regulator.linear.ams1117_3v3.sot223.regulator_evidence.output_capacitor"`) ||
		!strings.Contains(stdout.String(), "ESR-window stability proof") {
		t.Fatalf("missing AMS1117 stability blocker: %s", stdout.String())
	}
}

func TestRunComponentSelectBlocksMLCCFabricationCandidateOnDeratingEvidence(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "capacitor-select.json")
	writeTestFile(t, requestPath, `{
  "query": {
    "family": "capacitor",
    "package": "0805",
    "value_kind": "capacitance",
    "value": "10u"
  },
  "acceptance": "fabrication_candidate",
  "require_concrete": true,
  "required_ratings": [
    {"kind": "voltage", "value": "3.3", "unit": "V"}
  ]
}`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--request", requestPath, "component", "select"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected MLCC fabrication-candidate selection to fail")
	}
	if !strings.Contains(stdout.String(), `"component.capacitor.murata.grm21br61a106ke19l.0805.capacitor_evidence.effective_capacitance_review"`) {
		t.Fatalf("missing MLCC effective-capacitance blocker: %s", stdout.String())
	}
}

func TestRunComponentValidateWithSourceDir(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--source-dir", componentSourceFixtureDir(t, "valid"), "component", "validate"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("result data = %#v", result.Data)
	}
	// Source snapshots intentionally cover concrete purchasable parts, not blocked placeholders.
	if got, ok := data["source_record_count"].(float64); !ok || got != 22 {
		t.Fatalf("source_record_count = %#v in %#v", data["source_record_count"], data)
	}
}

func TestRunComponentCoverageWithSourceDir(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "--source-dir", componentSourceFixtureDir(t, "valid"), "component", "coverage"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"lifecycle_evidence_records"`) || !strings.Contains(stdout.String(), `"availability_evidence_records"`) {
		t.Fatalf("missing source coverage: %s", stdout.String())
	}
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode coverage result: %v\n%s", err, stdout.String())
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("coverage data = %#v", result.Data)
	}
	alternativeCoverage, ok := data["alternative_coverage"].(map[string]any)
	if !ok {
		t.Fatalf("missing alternative coverage: %#v", data)
	}
	for _, field := range []string{"concrete_records", "generic_fallback_records", "equivalence_groups"} {
		if _, ok := alternativeCoverage[field]; !ok {
			t.Errorf("missing alternative coverage field %s: %#v", field, alternativeCoverage)
		}
	}
	if got, ok := alternativeCoverage["concrete_records"].(float64); !ok || got < 27 {
		t.Fatalf("concrete_records = %#v in %#v", alternativeCoverage["concrete_records"], alternativeCoverage)
	}
	if got, ok := alternativeCoverage["equivalence_groups"].(float64); !ok || got < 19 {
		t.Fatalf("equivalence_groups = %#v in %#v", alternativeCoverage["equivalence_groups"], alternativeCoverage)
	}
	sourceCoverage, ok := data["source_coverage"].(map[string]any)
	if !ok {
		t.Fatalf("missing source coverage: %#v", data)
	}
	if got, ok := sourceCoverage["lifecycle_evidence_records"].(float64); !ok || got < 22 {
		t.Fatalf("lifecycle_evidence_records = %#v in %#v", sourceCoverage["lifecycle_evidence_records"], sourceCoverage)
	}
}

type componentSelectCompanion struct {
	Role string `json:"role"`
}

func cliCompanionsIncludeRole(companions []componentSelectCompanion, role string) bool {
	for _, companion := range companions {
		if companion.Role == role {
			return true
		}
	}
	return false
}

func TestRunComponentValidateJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--catalog-dir", testComponentCatalogDir(t), "component", "validate"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"record_count"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunDesignCreateSimpleLED(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := t.TempDir()
	requestPath := filepath.Join(root, "request.json")
	output := filepath.Join(root, "status_board")
	writeTestFile(t, requestPath, `{
  "version": "0.1.0",
  "name": "status_board",
  "board": {"width_mm": 40, "height_mm": 25, "layers": 2},
  "blocks": [{"id": "status", "block_id": "led_indicator"}],
  "validation": {"acceptance": "structural", "skip_routing": true}
}`)

	err := run([]string{"--json", "--request", requestPath, "--output", output, "design", "create"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if _, statErr := os.Stat(filepath.Join(output, "status_board.kicad_pcb")); statErr != nil {
		t.Fatalf("missing pcb output: %v", statErr)
	}
}

func TestRunRepairPlanTargetMissingProvenance(t *testing.T) {
	output := writeRepairCLIProject(t)
	if err := os.RemoveAll(filepath.Join(output, provenance.RelativePath)); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--target", output, "repair", "plan"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing provenance to block")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.OK || !strings.Contains(stdout.String(), "generated KiCadAI provenance") {
		t.Fatalf("unexpected result: %s", stdout.String())
	}
}

func TestRunRepairApplyTargetRequiresExecute(t *testing.T) {
	output, bundlePath := writeRepairCLIBundle(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--target", output, "--request", bundlePath, "--overwrite", "repair", "apply"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing execute to block")
	}
	if !strings.Contains(stdout.String(), "execute=true") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunRepairApplyTargetRequiresOverwrite(t *testing.T) {
	output, bundlePath := writeRepairCLIBundle(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--target", output, "--request", bundlePath, "--execute", "repair", "apply"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing overwrite to block")
	}
	if !strings.Contains(stdout.String(), "overwrite=true") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunRepairApplyTargetSuccess(t *testing.T) {
	output, bundlePath := writeRepairCLIBundle(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--target", output, "--request", bundlePath, "--execute", "--overwrite", "repair", "apply"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"status": "repaired"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRepairOutputDirUsesParentForFileTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "demo.kicad_sch")
	if err := os.WriteFile(target, []byte("schematic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repairOutputDir("", target); got != dir {
		t.Fatalf("output dir = %q, want %q", got, dir)
	}
	if got := repairOutputDir(filepath.Join(dir, "override"), target); got != filepath.Join(dir, "override") {
		t.Fatalf("override output dir = %q", got)
	}
}

func writeRepairCLIBundle(t *testing.T) (string, string) {
	t.Helper()
	output, tx := writeRepairCLIProjectWithTransaction(t)
	bundlePath := filepath.Join(t.TempDir(), "repair-bundle.json")
	bundle := repair.Bundle{
		Schema:        repair.BundleSchemaV1,
		ProjectRoot:   output,
		ProjectName:   "demo",
		Generated:     true,
		Transaction:   &tx,
		RepairOptions: repair.Options{Enabled: true},
	}
	if err := repair.SaveBundle(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	return output, bundlePath
}

func writeRepairCLIProject(t *testing.T) string {
	t.Helper()
	output, _ := writeRepairCLIProjectWithTransaction(t)
	return output
}

func writeRepairCLIProjectWithTransaction(t *testing.T) (string, transactions.Transaction) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "demo")
	tx := mustTestTransaction(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"set_board_outline","board":{"width_mm":40,"height_mm":25}},
	  {"op":"write_project"}
	]}`)
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues: %#v", result.Issues)
	}
	return output, tx
}

func mustTestTransaction(t *testing.T, input string) transactions.Transaction {
	t.Helper()
	tx, err := transactions.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func testComponentCatalogDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(testProjectRoot(t), "data", "components")
}

func testUnsafeComponentCatalogDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(testProjectRoot(t), "internal", "components", "testdata", "catalog", "unsafe_placeholder")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("unsafe component catalog fixture not found: %s", dir)
	}
	return dir
}

func componentSourceFixtureDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(testProjectRoot(t), "internal", "components", "testdata", "sources", name)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("component source fixture not found: %s", dir)
	}
	return dir
}

func testProjectRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}

func TestParseDesignAllowPartialSetsExplicitFlag(t *testing.T) {
	opts, command, err := parse([]string{"--allow-partial", "design", "create"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if command != "design" || !opts.routeAllowPartialSet || !opts.routeAllowPartial {
		t.Fatalf("opts=%#v command=%q", opts, command)
	}
}

func TestParseDesignRepairApplyImpliesRepair(t *testing.T) {
	opts, command, err := parse([]string{"--repair-apply", "design", "create"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if command != "design" || !opts.repairApply || !opts.repairEnabled {
		t.Fatalf("opts=%#v command=%q", opts, command)
	}
}

func TestDesignCreateOptionsRejectsUnknownRouteMode(t *testing.T) {
	_, err := designCreateOptions(context.Background(), cliOptions{routeMode: "sideways"}, checks.Options{})
	if err == nil {
		t.Fatal("expected invalid route mode error")
	}
}

func TestDesignCreateOptionsMapsPlacementFlags(t *testing.T) {
	opts, err := designCreateOptions(context.Background(), cliOptions{
		placementEstWidth:    3.5,
		placementEstHeight:   2.5,
		placementBoardMargin: 4,
	}, checks.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Placement.DefaultBounds.WidthMM != 3.5 || opts.Placement.DefaultBounds.HeightMM != 2.5 {
		t.Fatalf("default bounds = %#v", opts.Placement.DefaultBounds)
	}
	if opts.Placement.Rules.BoardEdgeClearanceMM != 4 {
		t.Fatalf("placement rules = %#v", opts.Placement.Rules)
	}
}

func TestDesignCreateOptionsMapsComponentSourceDir(t *testing.T) {
	opts, err := designCreateOptions(context.Background(), cliOptions{
		sourceDir: "component-sources",
	}, checks.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Components.SourceDir != "component-sources" {
		t.Fatalf("component source dir = %q", opts.Components.SourceDir)
	}
}

func TestDesignCreateOptionsRejectsUnsafeComponentSourceDir(t *testing.T) {
	_, err := designCreateOptions(context.Background(), cliOptions{sourceDir: "../component-sources"}, checks.Options{})
	if err == nil {
		t.Fatal("expected unsafe component source dir error")
	}
}

func TestDesignCreateOptionsRejectsConfiguredEmptySymbolRoot(t *testing.T) {
	_, err := designCreateOptions(context.Background(), cliOptions{symbolsRoot: t.TempDir()}, checks.Options{})
	if err == nil || !strings.Contains(err.Error(), "configured symbol library root produced no records") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunPlaceRequestJSON(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "placement.json")
	if err := os.WriteFile(request, []byte(`{
	  "Board": {"WidthMM": 40, "HeightMM": 25, "MarginMM": 1},
	  "Components": [{
	    "Ref": "R1",
	    "FootprintID": "Resistor_SMD:R_0805_2012Metric",
	    "Bounds": {"WidthMM": 2, "HeightMM": 1.25, "Source": "explicit"},
	    "Pads": [{"Name": "1"}, {"Name": "2"}]
	  }],
	  "Nets": [{"Name": "N1", "Endpoints": [{"Ref": "R1", "Pin": "1"}]}]
	}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "--request", request, "place", "request"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{`"command": "place"`, `"status": "placed"`, `"op": "place_footprint"`, `"quality":`, `"ready": true`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPlaceRequestRequiresRequestPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "place", "request"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "place request requires --request") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "place request requires --request") {
		t.Fatalf("expected missing request issue, got %s", stdout.String())
	}
}

func TestRunUnknownCommandReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("run returned nil error")
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) || !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLibraryFormatTextUnsupported(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--format", "text", "library", "index"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "library requires --format json") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunLibraryIndexJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "index"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{`"symbol_count": 1`, `"footprint_count": 1`, `"symbol_file_count": 1`, `"footprint_file_count": 1`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunLibrarySymbolAndFootprintJSON(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "symbol", args: []string{"library", "symbol", "Device:R"}, want: `"library_id": "Device:R"`},
		{name: "footprint", args: []string{"library", "footprint", "Resistor_SMD:R_0805_2012Metric"}, want: `"footprint_id": "Resistor_SMD:R_0805_2012Metric"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibrarySearchJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "search-symbols", "resistor"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"library_id": "Device:R"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunLibrarySymbolsNestedCommands(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "list", args: []string{"library", "symbols", "list"}, want: `"pin_count": 2`},
		{name: "show", args: []string{"library", "symbols", "show", "Device:R"}, want: `"library_id": "Device:R"`},
		{name: "pins", args: []string{"library", "symbols", "pins", "Device:R"}, want: `"electrical_type": "passive"`},
		{name: "validate", args: []string{"library", "symbols", "validate", "Device:R"}, want: `"kind": "symbol"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibrarySymbolsMissingSymbol(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "symbols", "show", "Device:Missing"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "MISSING_FILE"`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunLibraryCompatibilityJSON(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "compatible-footprints", args: []string{"library", "compatible-footprints", "Device:R"}, want: `"status": "compatible"`},
		{name: "validate-assignment", args: []string{"library", "validate-assignment", "Device:R", "Resistor_SMD:R_0805_2012Metric"}, want: `"status": "compatible"`},
		{name: "pinmap-candidate", args: []string{"library", "pinmap-candidate", "Device:R", "Resistor_SMD:R_0805_2012Metric"}, want: `"pinmap_candidate"`},
		{name: "klc-symbol", args: []string{"library", "klc-symbol", "Device:R"}, want: `"kind": "symbol"`},
		{name: "klc-footprint", args: []string{"library", "klc-footprint", "Resistor_SMD:R_0805_2012Metric"}, want: `"kind": "footprint"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibraryTemplatesJSON(t *testing.T) {
	templates := writeCLITemplateFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "templates", args: []string{"library", "templates"}, want: `"name": "Demo"`},
		{name: "template", args: []string{"library", "template", "Demo"}, want: `"project_files": [`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--templates-root", templates}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibraryUnsupportedSubcommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "library", "bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stdout.String(), `"unsupported library subcommand bogus"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunLibraryMissingIDJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "symbol", "Device:Missing"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "MISSING_FILE"`) || !strings.Contains(stdout.String(), `library record not found: Device:Missing`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunValidateBoardJSON(t *testing.T) {
	projectDir := writeCLIValidationProject(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "validate", "board", projectDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		`"command": "validate"`,
		`"status": "pass"`,
		`"name": "pcb_structural_validation"`,
		`"name": "unrouted_net_validation"`,
		`"name": "kicad_drc"`,
		`"fabrication_ready": true`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunValidateBoardRequireDRCMissingJSON(t *testing.T) {
	projectDir := writeCLIValidationProject(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--require-drc", "validate", "board", projectDir}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected required DRC failure")
	}
	for _, want := range []string{
		`"command": "validate"`,
		`"ok": false`,
		`"code": "SKIPPED_EXTERNAL_TOOL"`,
		`"KiCad DRC was not run because no KiCad CLI path was configured"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunValidateBoardRejectsContradictoryDRCFlags(t *testing.T) {
	projectDir := writeCLIValidationProject(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--require-drc", "--allow-missing-drc", "validate", "board", projectDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected contradictory DRC flag error")
	}
	if !strings.Contains(stdout.String(), "--require-drc and --allow-missing-drc cannot both be set") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunWriterCheckPCBJSON(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "writer_demo.kicad_pro"), "{}")
	writeTestFile(t, filepath.Join(projectDir, "writer_demo.kicad_sch"), `(kicad_sch
  (version 20260306)
  (generator "kicadai-test")
  (uuid "00000000-0000-0000-0000-000000000001")
  (paper "A4")
  (symbol (lib_id "power:GND") (at 10 10 0)
    (property "Reference" "#PWR01" (at 10 10 0))
    (property "Value" "GND" (at 10 12 0))
  )
)`)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "writer", "check", projectDir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		`"command": "writer"`,
		`"ok": true`,
		`"name": "project_structure"`,
		`"name": "schematic_parse"`,
		`"name": "schematic_connectivity"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunWriterCheckReportsLibraryResolverIssues(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "writer_demo.kicad_pro"), "{}")
	writeTestFile(t, filepath.Join(projectDir, "writer_demo.kicad_sch"), `(kicad_sch
  (version 20260306)
  (generator "kicadai-test")
  (uuid "00000000-0000-0000-0000-000000000001")
  (paper "A4")
  (symbol (lib_id "Device:R") (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "1k" (at 10 12 0))
    (property "Footprint" "Resistor_SMD:R_0603" (at 10 14 0) hide)
  )
)`)
	missingSymbols := filepath.Join(t.TempDir(), "missing-symbols")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--symbols-root", missingSymbols, "writer", "check", projectDir}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected writer check to fail on library resolver issue")
	}
	for _, want := range []string{
		`"command": "writer"`,
		`"ok": false`,
		`"name": "library_resolver"`,
		`missing-symbols`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunWriterCheckMissingTargetJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "writer", "check", filepath.Join(t.TempDir(), "missing.kicad_pcb")}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected writer failure")
	}
	for _, want := range []string{
		`"command": "writer"`,
		`"ok": false`,
		`writer check reported blocking issues`,
	} {
		if !strings.Contains(stdout.String()+err.Error(), want) {
			t.Fatalf("output missing %q:\nstdout=%s\nerr=%v", want, stdout.String(), err)
		}
	}
}

func TestRunWriterRejectsUnsupportedSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "writer", "scan", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), "unsupported writer subcommand scan") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestWriteReportJSON(t *testing.T) {
	var stdout bytes.Buffer

	err := writeReportJSON(&stdout, reports.ErrorResult("inspect-project", reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     "project",
		Message:  "project path does not exist",
	}))
	if err != nil {
		t.Fatalf("writeReportJSON returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "inspect-project"`,
		`"code": "MISSING_FILE"`,
		`"path": "project"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func writeCLIValidationProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "validate_demo.kicad_pro"), "{}")
	writeTestFile(t, filepath.Join(dir, "validate_demo.kicad_pcb"), `(kicad_pcb
  (version 20260206)
  (generator "pcbnew")
  (generator_version "10.0")
  (general (thickness 1.6))
  (paper "A4")
  (layers
    (0 "F.Cu" signal)
    (2 "B.Cu" signal)
    (13 "F.Paste" user)
    (1 "F.Mask" user)
    (25 "Edge.Cuts" user)
    (31 "F.CrtYd" user)
    (35 "F.Fab" user)
    (5 "F.SilkS" user)
  )
  (net 0 "")
  (net 1 "SIGNAL")
  (footprint "Test:Pad"
    (layer "F.Cu")
    (uuid "11111111-1111-1111-1111-111111111111")
    (property "Reference" "U1" (at 10 8 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "U1" (at 10 12 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (at 10 10 0)
    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu" "F.Paste" "F.Mask") (net 1 "SIGNAL") (uuid "11111111-1111-1111-1111-111111111114"))
  )
  (footprint "Test:Pad"
    (layer "F.Cu")
    (uuid "11111111-1111-1111-1111-111111111115")
    (property "Reference" "U2" (at 20 8 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111116"))
    (property "Value" "U2" (at 20 12 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111117"))
    (at 20 10 0)
    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu" "F.Paste" "F.Mask") (net 1 "SIGNAL") (uuid "11111111-1111-1111-1111-111111111118"))
  )
  (segment (start 10 10) (end 20 10) (width 0.25) (layer "F.Cu") (net 1 "SIGNAL") (uuid "11111111-1111-1111-1111-111111111119"))
  (gr_line (start 0 0) (end 30 0) (stroke (width 0.1) (type default)) (layer "Edge.Cuts") (uuid "11111111-1111-1111-1111-111111111120"))
  (gr_line (start 30 0) (end 30 20) (stroke (width 0.1) (type default)) (layer "Edge.Cuts") (uuid "11111111-1111-1111-1111-111111111121"))
  (gr_line (start 30 20) (end 0 20) (stroke (width 0.1) (type default)) (layer "Edge.Cuts") (uuid "11111111-1111-1111-1111-111111111122"))
  (gr_line (start 0 20) (end 0 0) (stroke (width 0.1) (type default)) (layer "Edge.Cuts") (uuid "11111111-1111-1111-1111-111111111123"))
)`)
	return dir
}

func writeCLILibraryFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	writeTestFile(t, filepath.Join(symbols, "Device.kicad_sym"), `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (ki_keywords "resistor R")
    (ki_description "Resistor")
    (ki_fp_filters "R_*" "Resistor_*")
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
)`)
	writeTestFile(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), `
(footprint "R_0805_2012Metric"
  (version 20240108)
  (generator "kicadai-test")
  (descr "Resistor SMD 0805")
  (tags "resistor 0805")
  (attr smd)
  (fp_rect (start -1.2 -0.8) (end 1.2 0.8) (layer "F.CrtYd"))
  (pad "1" smd roundrect (at -0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25))
  (pad "2" smd roundrect (at 0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25))
)`)
	return symbols, footprints
}

func writeCLITemplateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "Projects", "Demo")
	writeTestFile(t, filepath.Join(project, "Demo.kicad_pro"), `{}`)
	writeTestFile(t, filepath.Join(project, "Demo.kicad_sch"), `(kicad_sch)`)
	writeTestFile(t, filepath.Join(project, "Demo.kicad_pcb"), `(kicad_pcb)`)
	writeTestFile(t, filepath.Join(project, "fp-lib-table"), ``)
	writeTestFile(t, filepath.Join(project, "meta", "info.html"), `<p>demo</p>`)
	return root
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSONOperation(t *testing.T, kind transactions.OperationKind, payload any) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.NewOperationWithRef(kind, raw, testOperationRef(payload))
}

func testOperationRef(payload any) string {
	switch value := payload.(type) {
	case transactions.AddSymbolOperation:
		return value.Ref
	case transactions.AssignFootprintOperation:
		return value.Ref
	case transactions.PlaceFootprintOperation:
		return value.Ref
	default:
		return ""
	}
}

func TestRunHelpFlagPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected usage, got %s", stdout.String())
	}
}

func TestRunHelpIncludesGenerateCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	for _, want := range []string{"generate-led-demo", "generate-project", "--output", "--with-pcb", "--overwrite"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunHelpIncludesStructuredCommandFamilies(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	for _, want := range []string{"inspect", "evaluate", "pinmap", "transaction", "roundtrip", "export", "generate", "block"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunStructuredCommandDefaultsToJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"inspect", "project", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.Command != "inspect" {
		t.Fatalf("result command = %q", result.Command)
	}
}

func TestRunBlockDefaultsToJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"block", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("err = %v stdout=%s", err, stdout.String())
	}
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode result: %v\n%s", decodeErr, stdout.String())
	}
	if result.Command != "block" {
		t.Fatalf("result command = %q", result.Command)
	}
}

func TestRunBlockListShowValidate(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "block", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("list err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"id": "connector_breakout"`) || !strings.Contains(stdout.String(), `"id": "led_indicator"`) {
		t.Fatalf("unexpected list output: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--json", "block", "show", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("show err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"name": "LED Indicator"`) || !strings.Contains(stdout.String(), `"required_libraries"`) {
		t.Fatalf("unexpected show output: %s", stdout.String())
	}

	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"block_id":"led_indicator","instance_id":"status","params":{"active_high":true}}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	stdout.Reset()
	if err := run([]string{"--json", "--request", request, "block", "validate", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("validate err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"block_id": "led_indicator"`) {
		t.Fatalf("unexpected validate output: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--json", "--request", request, "block", "instantiate", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("instantiate err = %v stdout=%s", err, stdout.String())
	}
	for _, want := range []string{`"operations": [`, `"op": "add_symbol"`, `"op": "connect"`, `"refs": [`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("instantiate output missing %q: %s", want, stdout.String())
		}
	}

	stdout.Reset()
	if err := run([]string{"--json", "--request", request, "block", "realize-pcb", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("realize-pcb err = %v stdout=%s", err, stdout.String())
	}
	for _, want := range []string{`"realization": {`, `"local_routes": [`, `"placement_request": {`, `"components": [`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("realize-pcb output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunBlockVerifyBuiltinsAndCase(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "--builtins", "block", "verify"}, &stdout, &stderr); err != nil {
		t.Fatalf("verify builtins err = %v stdout=%s", err, stdout.String())
	}
	for _, want := range []string{`"command": "block"`, `"count": `, `"case_id": "led_indicator_default"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("verify builtins output missing %q: %s", want, stdout.String())
		}
	}

	stdout.Reset()
	manifest := testBlockVerificationManifestPath("led_indicator_default")
	if err := run([]string{"--json", "--case", manifest, "block", "verify"}, &stdout, &stderr); err != nil {
		t.Fatalf("verify case err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"count": 1`) || !strings.Contains(stdout.String(), `"status": "pass"`) {
		t.Fatalf("unexpected verify case output: %s", stdout.String())
	}
}

func testBlockVerificationManifestPath(caseID string) string {
	return filepath.Join("..", "..", "internal", "blocks", "testdata", "verification", caseID, "manifest.json")
}

func TestRunBlockVerifyRequiresSelection(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "verify"}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), "requires exactly one of --case, --suite, or --builtins") {
		t.Fatalf("err = %v stdout=%s", err, stdout.String())
	}
}

func TestRunBlockReportsStructuredFailures(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "show", "missing_block"}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), `"code": "MISSING_FILE"`) {
		t.Fatalf("missing err = %v stdout=%s", err, stdout.String())
	}

	stdout.Reset()
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"block_id":"led_indicator","params":{"active_high":"yes"}}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	err = run([]string{"--json", "--request", request, "block", "validate", "led_indicator"}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), `"code": "VALIDATION_FAILED"`) || !strings.Contains(stdout.String(), "active_high must be a bool") {
		t.Fatalf("invalid err = %v stdout=%s", err, stdout.String())
	}
}

func TestRunStructuredCommandRequiresSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "inspect"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "inspect"`,
		`"code": "INVALID_ARGUMENT"`,
		`"message": "inspect subcommand required"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunStructuredCommandRejectsUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "evaluate", "gerbers", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	if !strings.Contains(output, `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(output, `"unsupported evaluate subcommand gerbers"`) {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunStructuredCommandRejectsMissingTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "roundtrip", "pcb"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	if !strings.Contains(output, `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(output, `"roundtrip pcb requires 1 argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunExportPreviewJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "export", "preview", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "export"`,
		`"status": "blocked"`,
		`"dry_run": true`,
		`"kind": "readiness_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunExportFabricationExecuteGeneratesPlotEvidence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLITestPCB(t, root)
	cliPath := writeFakeKiCadCLI(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--execute", "--overwrite", "--kicad-cli", cliPath, "export", "fabrication", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected blocked readiness from missing schematic/BOM evidence")
	}
	output := stdout.String()
	for _, want := range []string{
		`"kind": "gerber"`,
		`"generator": "kicad-cli"`,
		`"files": [`,
		`"gerbers/demo-F_Cu.gbr"`,
		`"drill/demo.drl"`,
		`"gerber": "pass"`,
		`"drill": "pass"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunExportFabricationExecuteMissingKiCadCLIReportsEvidence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLITestPCB(t, root)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--execute", "--overwrite", "export", "fabrication", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing KiCad CLI error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"path": "fabrication.kicad_cli"`,
		`"gerber": "missing"`,
		`"drill": "missing"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestParseManufacturerProfileFlag(t *testing.T) {
	var stderr bytes.Buffer
	opts, command, err := parse([]string{"--manufacturer-profile", "generic_assembly", "export", "preview", "."}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if command != "export" || opts.manufacturerProfile != "generic_assembly" {
		t.Fatalf("command=%q manufacturerProfile=%q", command, opts.manufacturerProfile)
	}
}

func TestParseManufacturerProfileDirFlagAndEnvironment(t *testing.T) {
	t.Setenv("KICADAI_FABRICATION_PROFILE_DIR", "/tmp/env-profiles")
	var stderr bytes.Buffer
	opts, command, err := parse([]string{"export", "preview", "."}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if command != "export" || opts.manufacturerProfileDir != "/tmp/env-profiles" {
		t.Fatalf("command=%q manufacturerProfileDir=%q", command, opts.manufacturerProfileDir)
	}

	stderr.Reset()
	opts, command, err = parse([]string{"--manufacturer-profile-dir", "/tmp/flag-profiles", "export", "preview", "."}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if command != "export" || opts.manufacturerProfileDir != "/tmp/flag-profiles" {
		t.Fatalf("command=%q manufacturerProfileDir=%q", command, opts.manufacturerProfileDir)
	}
}

func TestRunFabricationProfileListJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"fabrication", "profile", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		`"command": "fabrication"`,
		`"id": "generic_assembly"`,
		`"thresholds":`,
		`"hash":`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunFabricationProfileShowJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"fabrication", "profile", "show", "generic_assembly"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		`"command": "fabrication"`,
		`"id": "generic_assembly"`,
		`"profile":`,
		`"min_pad_annular_ring_mm": 0.15`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunFabricationProfileValidateJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	writeTestFabricationProfile(t, path, "local_cli_profile", true)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"fabrication", "profile", "validate", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		`"command": "fabrication"`,
		`"id": "local_cli_profile"`,
		`"ok": true`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunFabricationProfileValidateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	writeTestFabricationProfile(t, path, "bad local id", false)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"fabrication", "profile", "validate", path}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected validation error:\n%s", stdout.String())
	}
	for _, want := range []string{
		`"ok": false`,
		`"path": "fabrication_profile.id"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunFabricationProfileRejectsUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"fabrication", "profile", "delete", "generic_assembly"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected unsupported subcommand error:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"path": "fabrication.profile.delete"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func writeTestFabricationProfile(t *testing.T, path string, id string, valid bool) {
	t.Helper()
	version := "2026-06"
	units := "mm"
	if !valid {
		version = "bad version"
		units = "mil"
	}
	data := fmt.Sprintf(`{
  "schema": "kicadai.fabrication.profile.v1",
  "id": %q,
  "name": "CLI Test Profile",
  "version": %q,
  "units": %q,
  "stackup": {
    "min_layers": 2,
    "max_layers": 2,
    "allowed_layer_counts": [2],
    "min_board_thickness_mm": 1.0,
    "max_board_thickness_mm": 1.6,
    "default_board_thickness_mm": 1.6
  },
  "copper": {
    "min_trace_width_mm": 0.2,
    "min_spacing_mm": 0.2
  },
  "drill": {
    "min_drill_mm": 0.35,
    "min_pad_annular_ring_mm": 0.18
  },
  "solder_mask": {
    "min_solder_mask_web_mm": 0.12
  }
}`, id, version, units)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunEvaluateSchematicJSONIncludesElectricalCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch (version 20260306) (generator "kicadai"))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "evaluate", "schematic", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"command": "evaluate"`,
		`"name": "schematic_validation"`,
		`"name": "schematic_electrical"`,
		`"status": "passed"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunEvaluatePCBJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb (gr_rect (layer "Edge.Cuts")) (footprint "Test:One" (pad "1" smd rect (layers "F.Cu"))))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "evaluate", "pcb", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "evaluate"`,
		`"target": "` + path + `"`,
		`"name": "pcb_validation"`,
		`"status": "passed"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func writeCLITestPCB(t *testing.T, root string) {
	t.Helper()
	file, err := os.Create(filepath.Join(root, "demo.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	board := pcbfiles.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "kicadai-test",
		GeneratorVersion: "cli-phase5",
		General:          pcbfiles.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcbfiles.DefaultTwoLayerStack(),
		Setup:            pcbfiles.DefaultSetup(),
		Drawings: []pcbfiles.Drawing{{
			UUID:  kicadfiles.UUID("33333333-3333-4333-8333-333333333333"),
			Layer: kicadfiles.LayerEdge,
			Kind:  "line",
			Line: &pcbfiles.LineDrawing{
				Start: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(0)},
				End:   kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(0)},
				Width: kicadfiles.MM(0.1),
			},
		}},
		Vias: []pcbfiles.Via{{
			UUID:     kicadfiles.UUID("44444444-4444-4444-8444-444444444444"),
			Position: kicadfiles.Point{X: kicadfiles.MM(5), Y: kicadfiles.MM(5)},
			Size:     kicadfiles.MM(0.8),
			Drill:    kicadfiles.MM(0.4),
			Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerBCu},
		}},
	}
	if err := pcbfiles.Write(file, board); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeFakeKiCadCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake kicad-cli shell shim requires POSIX shell execution")
	}
	path := filepath.Join(t.TempDir(), "kicad-cli")
	script := `#!/bin/sh
kind=""
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    gerber|gerbers) kind="gerbers" ;;
    drill) kind="drill" ;;
    --output) shift; out="$1" ;;
  esac
  shift
done
mkdir -p "$out"
if [ "$kind" = "gerbers" ]; then
  for layer in F_Cu B_Cu F_Mask B_Mask F_SilkS B_SilkS Edge_Cuts; do
    printf "artifact\n" > "$out/demo-$layer.gbr"
  done
elif [ "$kind" = "drill" ]; then
  printf "artifact\n" > "$out/demo.drl"
else
  echo "unexpected export kind" >&2
  exit 1
fi
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunEvaluateMissingTargetJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "evaluate", "pcb", filepath.Join(t.TempDir(), "missing.kicad_pcb")}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "evaluate"`,
		`"code": "MISSING_FILE"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunInspectProjectJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch (version 20260306) (symbol))`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pcb"), []byte(`(kicad_pcb (gr_rect (layer "Edge.Cuts")) (footprint "Test:One" (pad "1" smd rect (layers "F.Cu"))))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "inspect", "project", root}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "inspect"`,
		`"name": "demo"`,
		`"symbol_count": 1`,
		`"footprint_count": 1`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPinmapListJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "pinmap", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "pinmap"`,
		`"source": "human_verified"`,
		`"symbol": "Device:R"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPinmapValidateReportsBlockingIssues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:U")
    (property "Reference" "U1")
    (property "Footprint" "Package_SO:SOIC-8")
    (uuid "33333333-3333-5333-8333-333333333333"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "pinmap", "validate", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "pinmap"`,
		`"code": "PINMAP_UNVERIFIED"`,
		`"fabrication_ready": false`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunEvaluateProjectWithBlockingIssuesReturnsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "evaluate", "project", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "evaluate"`,
		`"code": "MISSING_FILE"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripSkipsWhenKiCadCLIUnavailable(t *testing.T) {
	t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch (version 20260306))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "roundtrip", "schematic", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("skip should not fail command: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "roundtrip"`,
		`"code": "SKIPPED_EXTERNAL_TOOL"`,
		`"skipped": true`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckSkipsWhenKiCadCLIUnavailable(t *testing.T) {
	t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch)`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "check", "erc", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("skip should not fail command: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "check"`,
		`"code": "SKIPPED_EXTERNAL_TOOL"`,
		`"status": "skipped"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckERCWithFakeCLIReportsFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeCheckCLI(t, 1, `{"coordinate_units":"mm","sheets":[{"path":"/","violations":[{"description":"Input Power pin not driven by any Output Power pins","severity":"error","type":"power_pin_not_driven","items":[{"description":"Symbol #PWR01 Pin 1","pos":{"x":1,"y":2},"uuid":"11111111-1111-4111-8111-111111111111"}]}]}]}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--keep-artifacts", "--artifact-dir", filepath.Join(dir, "artifacts"), "check", "erc", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected validation failure")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "check"`,
		`"kind": "erc"`,
		`"status": "fail"`,
		`"power_pin_not_driven"`,
		`"erc_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckProjectWithFakeCLIReportsBothChecks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"demo.kicad_pro": "{}",
		"demo.kicad_sch": `(kicad_sch)`,
		"demo.kicad_pcb": `(kicad_pcb)`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cli := fakeCheckCLI(t, 0, `{"coordinate_units":"mm","violations":[],"sheets":[]}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "check", "project", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	if strings.Count(output, `"status": "pass"`) != 2 {
		t.Fatalf("expected two passing checks:\n%s", output)
	}
}

func TestRunRoundTripSchematicWithFakeCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch (version 20260306))`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 0)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--keep-artifacts", "--artifact-dir", filepath.Join(dir, "artifacts"), "roundtrip", "schematic", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"file_type": "schematic"`,
		`"equal": true`,
		`"roundtrip_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripReportsKiCadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 3)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "pcb", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "KICAD_CLI_FAILED"`,
		`"file_type": "pcb"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripRejectsNonExecutableKiCadCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not use Unix execute bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "pcb", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "KICAD_CLI_FAILED"`) || !strings.Contains(stdout.String(), "not executable") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunRoundTripProjectDiscoversRootFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"demo.kicad_pro": "{}",
		"demo.kicad_sch": `(kicad_sch (version 20260306))`,
		"demo.kicad_pcb": `(kicad_pcb)`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 0)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "project", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	if strings.Count(output, `"equal": true`) != 2 {
		t.Fatalf("expected two successful checks:\n%s", output)
	}
}

func TestRunGenerateStructuredCommandAllowsNoTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "generate", "example"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "UNSUPPORTED_OPERATION"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunGenerateBreakoutJSON(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "request.json")
	output := filepath.Join(dir, "out")
	body := `{
	  "kind":"breakout_board",
	  "name":"sensor_breakout",
	  "board":{"width_mm":50,"height_mm":30},
	  "connectors":[
	    {"ref":"J1","pins":["VCC","GND","SCL","SDA"]},
	    {"ref":"J2","pins":["VCC","GND","SCL","SDA"]}
	  ]
	}`
	if err := os.WriteFile(request, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "--output", output, "generate", "breakout"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "generate"`, `"kind": "pcb"`, `sensor_breakout.kicad_pcb`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "sensor_breakout.kicad_pcb")); err != nil {
		t.Fatalf("expected generated PCB: %v", err)
	}
}

func TestRunBlockInstantiateWritesProject(t *testing.T) {
	output := filepath.Join(t.TempDir(), "status_led")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--output", output, "--name", "status_led", "block", "instantiate", "led_indicator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "block"`,
		`"apply_result"`,
		`"kicad_project"`,
		`"feedback"`,
		`"stage": "placement"`,
		`"action": "assign_courtyard_footprint"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	for _, name := range []string{"status_led.kicad_pro", "status_led.kicad_sch", "status_led.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected generated %s: %v", name, err)
		}
	}
}

func TestRunBlockInstantiateCanSkipPlacementFeedback(t *testing.T) {
	output := filepath.Join(t.TempDir(), "status_led")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--skip-placement-feedback", "--output", output, "--name", "status_led", "block", "instantiate", "led_indicator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	if strings.Contains(text, `"feedback"`) {
		t.Fatalf("expected feedback to be omitted:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(output, "status_led.kicad_pcb")); err != nil {
		t.Fatalf("expected generated PCB: %v", err)
	}
}

func TestRunBlockInstantiatePlacementFeedbackUsesLibraryGeometry(t *testing.T) {
	_, footprints := writeCLILibraryFixture(t)
	dir := t.TempDir()
	output := filepath.Join(dir, "status_led")
	request := filepath.Join(dir, "request.json")
	body := `{
	  "block_id": "led_indicator",
	  "instance_id": "status_led",
	  "params": {
	    "resistor_footprint": "Resistor_SMD:R_0805_2012Metric",
	    "led_footprint": "Resistor_SMD:R_0805_2012Metric"
	  }
	}`
	if err := os.WriteFile(request, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--json",
		"--request", request,
		"--output", output,
		"--footprints-root", footprints,
		"block", "instantiate", "led_indicator",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{`"ok": true`, `"feedback"`, `"stage": "placement"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{`"estimated_bounds_refs"`, `"action": "assign_courtyard_footprint"`} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("expected resolver-backed feedback to omit %q:\n%s", unwanted, text)
		}
	}
}

func TestBlockPlacementLibraryIndexSkipsEmptyFootprintIndex(t *testing.T) {
	roots := libraryRootsFromOptions(cliOptions{footprintsRoot: filepath.Join(t.TempDir(), "missing")})
	_, ok, issues := blockPlacementLibraryIndex(context.Background(), roots, cliOptions{})
	if ok {
		t.Fatal("expected empty footprint index to be skipped")
	}
	if len(issues) == 0 {
		t.Fatal("expected load diagnostics")
	}
}

func TestReplaceSupersededPlacementOperationsBeforeWriteProject(t *testing.T) {
	oldPlace := mustJSONOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
		Op:          transactions.OpPlaceFootprint,
		Ref:         "R1",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
		At:          transactions.Point{XMM: 1, YMM: 2},
	})
	newPlace := mustJSONOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
		Op:          transactions.OpPlaceFootprint,
		Ref:         "R1",
		FootprintID: "Resistor_SMD:R_0805_2012Metric",
		At:          transactions.Point{XMM: 20, YMM: 30},
	})
	write := mustJSONOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject})
	tx := transactions.Transaction{Operations: []transactions.Operation{
		{Op: transactions.OpCreateProject},
		oldPlace,
		{Op: transactions.OpConnect},
		oldPlace,
		write,
	}}

	got := replaceSupersededPlacementOperationsBeforeWriteProject(tx, []transactions.Operation{newPlace})
	if len(got.Operations) != 4 {
		t.Fatalf("operation count = %d, want 4: %#v", len(got.Operations), got.Operations)
	}
	if got.Operations[1].Op != transactions.OpConnect || got.Operations[2].Op != transactions.OpPlaceFootprint || got.Operations[3].Op != transactions.OpWriteProject {
		t.Fatalf("operation order = %#v", got.Operations)
	}
	var payload transactions.PlaceFootprintOperation
	if err := json.Unmarshal(got.Operations[2].Raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.At.XMM != 20 || payload.At.YMM != 30 {
		t.Fatalf("replacement placement = %#v", payload.At)
	}
}

func TestRunBlockRejectsInvalidPlacementFeedbackBoard(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--placement-board-width", "4", "--placement-board-margin", "2", "block", "list"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--placement-board-margin must leave positive usable board area") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBlockRejectsNaNPlacementFeedbackBounds(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--placement-estimated-width", "NaN", "block", "list"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--placement-estimated-width must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBlockComposeWritesProject(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "request.json")
	output := filepath.Join(dir, "composed")
	body := `{
	  "project_name": "composed",
	  "instances": [
	    {"id": "header", "block_id": "connector_breakout", "params": {"pin_names": ["SIG", "GND"]}},
	    {"id": "status", "block_id": "led_indicator"}
	  ],
	  "connections": [
	    {"from": {"instance_id": "header", "port": "SIG"}, "to": {"instance_id": "status", "port": "IN"}, "net_alias": "LED_EN"}
	  ]
	}`
	if err := os.WriteFile(request, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "--output", output, "block", "compose"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "block"`,
		`"project_name": "composed"`,
		`"kind": "schematic"`,
		`"feedback"`,
		`"stage": "placement"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	for _, name := range []string{"composed.kicad_pro", "composed.kicad_sch", "composed.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected generated %s: %v", name, err)
		}
	}
}

func TestRunBlockComposeRequiresRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "compose"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block compose requires --request"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockComposeRejectsEmptyRequest(t *testing.T) {
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"project_name":"empty"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "block", "compose"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"composition request must contain at least one instance"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockRequiresSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block requires a subcommand"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockMissingBlockReturnsError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "show", "missing_block"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block not found: missing_block"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func fakeRoundTripCLI(t *testing.T, logPath string, upgradeExit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	var body string
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n" +
			"if \"%1\"==\"--version\" echo 10.0.0& exit /b 0\r\n" +
			"echo %* > \"" + logPath + "\"\r\n" +
			"if not \"" + strconv.Itoa(upgradeExit) + "\"==\"0\" echo failed 1>&2\r\n" +
			"exit /b " + strconv.Itoa(upgradeExit) + "\r\n"
	} else {
		body = "#!/bin/sh\n" +
			"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
			"printf '%s\\n' \"$*\" >> '" + logPath + "'\n"
		if upgradeExit != 0 {
			body += "printf '%s\\n' failed >&2\n"
		}
		body += "exit " + strconv.Itoa(upgradeExit) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake KiCad CLI: %v", err)
	}
	return path
}

func fakeCheckCLI(t *testing.T, exitCode int, reportJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	if runtime.GOOS == "windows" {
		path += ".bat"
		t.Skip("fake check CLI helper is implemented for POSIX shells")
	}
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
		"out=''\n" +
		"prev=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--output\" ]; then out=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"if [ -n \"$out\" ]; then cat > \"$out\" <<'EOF'\n" +
		reportJSON + "\nEOF\n" +
		"fi\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake check CLI: %v", err)
	}
	return path
}

func fakeWorkflowKiCadCLI(t *testing.T, checkExitCode int, reportJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	if runtime.GOOS == "windows" {
		path += ".bat"
		t.Skip("fake workflow KiCad CLI helper is implemented for POSIX shells")
	}
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
		"out=''\n" +
		"prev=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--output\" ]; then out=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"if { [ \"$1\" = \"sch\" ] && [ \"$2\" = \"erc\" ]; } || { [ \"$1\" = \"pcb\" ] && [ \"$2\" = \"drc\" ]; }; then\n" +
		"  if [ -n \"$out\" ]; then cat > \"$out\" <<'EOF'\n" +
		reportJSON + "\nEOF\n" +
		"  fi\n" +
		"  exit " + strconv.Itoa(checkExitCode) + "\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake workflow KiCad CLI: %v", err)
	}
	return path
}

func TestRunTransactionPlanRequiresProjectAndTransaction(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "transaction", "plan", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(stdout.String(), `"transaction plan requires 2 argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionValidateJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"},{"op":"write_project"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "validate", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "transaction"`,
		`"operation_count": 2`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, `"feedback"`) {
		t.Fatalf("default validate output should not include feedback wrapper:\n%s", output)
	}
}

func TestRunTransactionValidateFeedbackJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"route","net_name":"","points":[{"x_mm":0,"y_mm":0}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--feedback", "transaction", "validate", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected validation error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"validation":`,
		`"feedback":`,
		`"operation_id": "op-route-`,
		`"unlinked_count": 0`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionFromSchematicJSON(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	root := filepath.Join(t.TempDir(), "demo")
	writeTestFile(t, filepath.Join(root, "demo.kicad_pro"), `{}`)
	writeTestFile(t, filepath.Join(root, "demo.kicad_sch"), `
(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (wire (pts (xy 15.08 10) (xy 34.92 10)) (uuid "22222222-2222-5222-8222-222222222222"))
  (label "NET_A" (at 25 10 0) (uuid "33333333-3333-5333-8333-333333333333"))
  (symbol
    (lib_id "Device:R")
    (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "10k" (at 10 12.54 0))
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric" (at 10 15.08 0))
    (uuid "44444444-4444-5444-8444-444444444444"))
  (symbol
    (lib_id "Device:R")
    (at 40 10 0)
    (property "Reference" "R2" (at 40 10 0))
    (property "Value" "10k" (at 40 12.54 0))
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric" (at 40 15.08 0))
    (uuid "55555555-5555-5555-8555-555555555555"))
  (sheet
    (at 60 20)
    (size 20 20)
    (property "Sheetname" "Child")
    (property "Sheetfile" "child.kicad_sch")
    (uuid "66666666-6666-5666-8666-666666666666"))
)`)
	writeTestFile(t, filepath.Join(root, "child.kicad_sch"), `
(kicad_sch
  (version 20260306)
  (generator "eeschema")
  (generator_version "10.0")
  (uuid "77777777-7777-5777-8777-777777777777")
  (paper A4)
  (wire (pts (xy 15.08 10) (xy 25 10)) (uuid "88888888-8888-5888-8888-888888888888"))
  (label "CHILD_NET" (at 25 10 0) (uuid "99999999-9999-5999-8999-999999999999"))
  (symbol
    (lib_id "Device:R")
    (at 10 10 0)
    (property "Reference" "R3" (at 10 10 0))
    (property "Value" "4.7k" (at 10 12.54 0))
    (property "Footprint" "Resistor_SMD:R_0805_2012Metric" (at 10 15.08 0))
    (uuid "aaaaaaaa-aaaa-5aaa-8aaa-aaaaaaaaaaaa"))
)`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "transaction", "from-schematic", root}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "transaction"`,
		`"symbol_count": 3`,
		`"placed_count": 3`,
		`"net_hint_count": 3`,
		`"op": "place_footprint"`,
		`"ref": "R1"`,
		`"ref": "R2"`,
		`"ref": "R3"`,
		`"net": "NET_A"`,
		`"net": "Child/CHILD_NET"`,
		`"op": "write_project"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRouteRequestJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "route.json")
	request := `{
  "board":{"width_mm":30,"height_mm":20,"layers":[{"name":"F.Cu","kind":"copper","routable":true}]},
  "components":[
    {"ref":"J1","position":{"x_mm":5,"y_mm":10,"layer":"F.Cu"},"pads":[{"name":"1","net":"SIG","shape":"circle","type":"smd","size":{"width_mm":1,"height_mm":1},"layers":["F.Cu"]}]},
    {"ref":"J2","position":{"x_mm":20,"y_mm":10,"layer":"F.Cu"},"pads":[{"name":"1","net":"SIG","shape":"circle","type":"smd","size":{"width_mm":1,"height_mm":1},"layers":["F.Cu"]}]}
  ],
  "nets":[{"name":"SIG","endpoints":[{"ref":"J1","pin":"1"},{"ref":"J2","pin":"1"}]}],
  "rules":{"grid_mm":1,"trace_width_mm":0.1,"clearance_mm":0.01,"edge_clearance_mm":0.01},
  "strategy":{"mode":"single_layer"}
}`
	if err := os.WriteFile(path, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", path, "route", "request"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "route"`, `"status": "routed"`, `"operations"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRouteRejectsInvalidMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "route.json")
	if err := os.WriteFile(path, []byte(`{"board":{"width_mm":1,"height_mm":1},"rules":{},"strategy":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", path, "--mode", "diagonal", "route", "request"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if !strings.Contains(stdout.String(), `unsupported route mode`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionValidateReportsIssues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"route","net_name":"","points":[{"x_mm":0,"y_mm":0}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "validate", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "INVALID_ARGUMENT"`,
		`"path": "operations[0].net_name"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionPlanJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"},{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}},{"op":"write_project"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "plan", dir, path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"op": "add_symbol"`,
		`"refs": [`,
		`demo.kicad_sch`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionPlanFeedbackJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"},{"op":"remove_symbol","ref":"R1"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--feedback", "transaction", "plan", dir, path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected plan error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"plan":`,
		`"feedback":`,
		`"operation_id": "op-remove-symbol-`,
		`"blocking_count": 2`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionPlanBlocksExistingProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "plan", dir, path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"path": "operations[0].op"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionApplyJSON(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":-2.54},{"number":"2","x_mm":2.54}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "apply", output, path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"kicad_project"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(output, "demo.kicad_pcb")); err != nil {
		t.Fatalf("PCB not written: %v", err)
	}
}

func TestRunStructuredCommandRejectsExtraArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "inspect", "project", "demo", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(stdout.String(), `"inspect project received 1 unexpected argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunGenerateLEDDemoJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo-output")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--name", "led_indicator",
		"--seed", "cli-test",
		"--lib-resistor", "Custom:R_US",
		"--with-pcb",
		"--json",
		"generate-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	var result generationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result.ProjectName != "led_indicator" || len(result.WrittenFiles) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pcb")); err != nil {
		t.Fatalf("PCB not written: %v", err)
	}
	schematicBytes, err := os.ReadFile(filepath.Join(root, "led_indicator.kicad_sch"))
	if err != nil {
		t.Fatalf("schematic not readable: %v", err)
	}
	if !strings.Contains(string(schematicBytes), "Custom:R_US") {
		t.Fatalf("custom library ID missing from schematic:\n%s", schematicBytes)
	}
}

func TestRunGenerateLEDDemoRefusesOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--json",
		"generate-led-demo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"target exists`) {
		t.Fatalf("expected target exists JSON, got %s", stdout.String())
	}
}

func TestRunGenerateProjectOmitsPCBByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--json",
		"generate-project",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pcb")); !os.IsNotExist(err) {
		t.Fatalf("PCB should be omitted by default, stat error: %v", err)
	}
}

func TestRunGenerateRejectsCurrentDirectoryOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", ".",
		"--json",
		"generate-project",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), "project directory") {
		t.Fatalf("expected project directory error JSON, got %s", stdout.String())
	}
}

func TestRunPingJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"reachable": true`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPingJSONConnectFailure(t *testing.T) {
	want := errors.New("dial failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return nil, want
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	output := stdout.String()
	for _, wantText := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"reachable": false`,
		`"error": "dial failed"`,
	} {
		if !strings.Contains(output, wantText) {
			t.Fatalf("expected output to contain %q, got %s", wantText, output)
		}
	}
}

func TestRunPingTextConnectFailureDoesNotWriteJSON(t *testing.T) {
	want := errors.New("dial failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return nil, want
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--format", "text",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no text output on connect failure, got %s", stdout.String())
	}
}

func TestRunVersionJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"version",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"reachable": true`,
		`"full_version": "9.1.0"`,
		`"major": 9`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunVersionJSONFailureReturnsStructuredResult(t *testing.T) {
	want := errors.New("version failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{versionErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"version",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"reachable": false`) || !strings.Contains(stdout.String(), `"error": "version failed"`) {
		t.Fatalf("expected structured version failure, got %s", stdout.String())
	}
}

func TestRunPingJSONFailureReturnsError(t *testing.T) {
	want := errors.New("not reachable")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{pingErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"reachable": false`) {
		t.Fatalf("expected failure JSON, got %s", stdout.String())
	}
}

func TestRunDocumentsInvalidType(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document-type", "invalid",
		"documents",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unsupported document type") {
		t.Fatalf("run error = %v, want unknown document type", err)
	}
}

func TestRunDocumentsJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			documents: []kiapi.Document{{
				Type:       kiapi.DocumentTypeSchematic,
				Identifier: "/",
				SheetPath:  "/",
			}},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--document-type", "schematic",
		"--json",
		"documents",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"documents": [`,
		`"type": "schematic"`,
		`"sheet_path": "/"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunDocumentsJSONFailureReturnsStructuredResult(t *testing.T) {
	want := errors.New("documents failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{documentsErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"documents",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	output := stdout.String()
	if !strings.Contains(output, `"documents": []`) || !strings.Contains(output, `"error": "documents failed"`) {
		t.Fatalf("expected structured documents failure, got %s", output)
	}
}

func TestRunCapabilitiesJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"capabilities",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	var capabilities kiapi.Capabilities
	if err := json.Unmarshal(stdout.Bytes(), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities JSON: %v", err)
	}
	if capabilities.KiCadVersion != "9.1.0" {
		t.Fatalf("KiCadVersion = %q", capabilities.KiCadVersion)
	}
	if !capabilities.Supports(kiapi.CapabilitySchematicRead) {
		t.Fatalf("expected schematic read in supported capabilities")
	}
	for _, missing := range []kiapi.Capability{
		kiapi.CapabilitySchematicWrite,
		kiapi.CapabilitySymbolPlace,
	} {
		if capabilities.Supports(missing) {
			t.Fatalf("expected %s to be missing, got supported", missing)
		}
	}
}

func TestRunCapabilitiesJSONVersionFailure(t *testing.T) {
	want := errors.New("version failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{versionErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"capabilities",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"kicad_version": "unknown"`) || !strings.Contains(stdout.String(), `"error": "version failed"`) {
		t.Fatalf("expected structured capabilities failure, got %s", stdout.String())
	}
}

func TestRunPlanLEDDemoJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--origin-x", "1000",
		"--origin-y", "2000",
		"--prefix", "STATUS",
		"--lib-resistor", "Custom:R_US",
		"--json",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"operations": [`,
		`"kind": "add_symbol"`,
		`Custom:R_US`,
		`STATUS_OUT`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPlanLEDDemoJSONMissingDocument(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--json",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("run returned nil error")
	}
	if !strings.Contains(stdout.String(), `"success": false`) || !strings.Contains(stdout.String(), `"document is required"`) {
		t.Fatalf("expected structured planning error, got %s", stdout.String())
	}
}

func TestRunPlanLEDDemoTextOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--format", "text",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "1. add_symbol") {
		t.Fatalf("expected text plan output, got %s", stdout.String())
	}
}

func TestRunDrawLEDDemoJSONRequiresExecute(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--json",
		"draw-led-demo",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "requires --execute") {
		t.Fatalf("run error = %v, want --execute requirement", err)
	}
	if !strings.Contains(stdout.String(), `"success": false`) || !strings.Contains(stdout.String(), `"draw-led-demo requires --execute"`) {
		t.Fatalf("expected structured error result, got %s", stdout.String())
	}
}

func TestRunDrawLEDDemoJSONReportsMissingWriteCapability(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--document", "/",
		"--execute",
		"--json",
		"draw-led-demo",
	}, &stdout, &stderr)
	if !errors.Is(err, workflows.ErrMissingSchematicWriteCapability) {
		t.Fatalf("run error = %v, want %v", err, workflows.ErrMissingSchematicWriteCapability)
	}
	if !strings.Contains(stdout.String(), `"operations_completed": 0`) {
		t.Fatalf("expected structured result, got %s", stdout.String())
	}
}

type fakeAPIClient struct {
	pingErr      error
	version      *commontypes.KiCadVersion
	versionErr   error
	documents    []kiapi.Document
	documentsErr error
	closeErr     error
}

func (c *fakeAPIClient) Ping(context.Context) error {
	return c.pingErr
}

func (c *fakeAPIClient) GetVersion(context.Context) (*commontypes.KiCadVersion, error) {
	if c.versionErr != nil {
		return nil, c.versionErr
	}
	return c.version, nil
}

func (c *fakeAPIClient) GetOpenDocuments(context.Context, kiapi.DocumentType) ([]kiapi.Document, error) {
	if c.documentsErr != nil {
		return nil, c.documentsErr
	}
	return c.documents, nil
}

func (c *fakeAPIClient) Close() error {
	return c.closeErr
}

func TestSchematicIRValidateCLI(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestPath := filepath.Join("..", "..", "examples", "schematic-ir", "led_indicator.json")

	if err := run([]string{"--request", requestPath, "schematic-ir", "validate"}, &stdout, &stderr); err != nil {
		t.Fatalf("schematic-ir validate failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			InputPath string `json:"input_path"`
			Summary   struct {
				ComponentCount int `json:"component_count"`
				NetCount       int `json:"net_count"`
				GroupCount     int `json:"group_count"`
				PlacementCount int `json:"placement_count"`
			} `json:"summary"`
		} `json:"data"`
		Issues []reports.Issue `json:"issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode schematic-ir validate response: %v\n%s", err, stdout.String())
	}
	if !result.OK || len(result.Issues) != 0 {
		t.Fatalf("expected ok validate response, got %+v", result)
	}
	if result.Data.InputPath != requestPath {
		t.Fatalf("input path = %q, want %q", result.Data.InputPath, requestPath)
	}
	if result.Data.Summary.ComponentCount != 3 || result.Data.Summary.NetCount != 3 {
		t.Fatalf("unexpected summary: %+v", result.Data.Summary)
	}
	if result.Data.Summary.GroupCount != 0 || result.Data.Summary.PlacementCount != 0 {
		t.Fatalf("expected input layout summary, got %+v", result.Data.Summary)
	}
}

func TestSchematicIRNormalizeCLI(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestPath := filepath.Join("..", "..", "examples", "schematic-ir", "usb_c_led_indicator.json")

	if err := run([]string{"--request", requestPath, "schematic-ir", "normalize"}, &stdout, &stderr); err != nil {
		t.Fatalf("schematic-ir normalize failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Normalized struct {
				Layout struct {
					Groups     []any `json:"groups"`
					Placements []any `json:"placements"`
				} `json:"layout"`
			} `json:"normalized"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode schematic-ir normalize response: %v\n%s", err, stdout.String())
	}
	if !result.OK {
		t.Fatalf("expected ok normalize response: %s", stdout.String())
	}
	if len(result.Data.Normalized.Layout.Groups) == 0 || len(result.Data.Normalized.Layout.Placements) == 0 {
		t.Fatalf("expected normalized layout content: %s", stdout.String())
	}
}

func TestSchematicIRTransactionCLI(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestPath := filepath.Join("..", "..", "examples", "schematic-ir", "i2c_sensor_3v3_regulator.json")

	if err := run([]string{"--request", requestPath, "schematic-ir", "transaction"}, &stdout, &stderr); err != nil {
		t.Fatalf("schematic-ir transaction failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Summary struct {
				OperationCount int `json:"operation_count"`
			} `json:"summary"`
			Transaction transactions.Transaction `json:"transaction"`
			Validation  struct {
				Issues []reports.Issue `json:"issues"`
			} `json:"transaction_validation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode schematic-ir transaction response: %v\n%s", err, stdout.String())
	}
	if !result.OK {
		t.Fatalf("expected ok transaction response: %s", stdout.String())
	}
	if result.Data.Summary.OperationCount == 0 || len(result.Data.Transaction.Operations) == 0 {
		t.Fatalf("expected transaction operations: %s", stdout.String())
	}
	if len(result.Data.Validation.Issues) != 0 {
		t.Fatalf("expected clean transaction validation: %+v", result.Data.Validation.Issues)
	}
}

func TestSchematicIRWriteCLI(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestPath := filepath.Join("..", "..", "examples", "schematic-ir", "led_indicator.json")
	output := filepath.Join(t.TempDir(), "led_indicator")

	if err := run([]string{"--request", requestPath, "--output", output, "schematic-ir", "write"}, &stdout, &stderr); err != nil {
		t.Fatalf("schematic-ir write failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Summary struct {
				OperationCount int `json:"operation_count"`
			} `json:"summary"`
			Transaction transactions.Transaction `json:"transaction"`
			Validation  struct {
				Issues []reports.Issue `json:"issues"`
			} `json:"transaction_validation"`
		} `json:"data"`
		Issues []reports.Issue `json:"issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode schematic-ir write response: %v\n%s", err, stdout.String())
	}
	if !result.OK || len(result.Issues) != 0 || len(result.Data.Validation.Issues) != 0 {
		t.Fatalf("expected clean write response: %s", stdout.String())
	}
	if result.Data.Summary.OperationCount == 0 || len(result.Data.Transaction.Operations) == 0 {
		t.Fatalf("expected write transaction operations: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(output, "led_indicator.kicad_pro")); err != nil {
		t.Fatalf("expected generated project file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "led_indicator.kicad_sch")); err != nil {
		t.Fatalf("expected generated schematic file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "led_indicator.kicad_pcb")); err == nil {
		t.Fatal("schematic-ir write emitted a PCB file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat generated PCB file: %v", err)
	}
}

func TestSchematicIRWriteCLIUsesConfiguredSymbolResolver(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	requestPath := filepath.Join("..", "..", "examples", "schematic-ir", "external_connector_indicator.json")
	symbolsRoot := filepath.Join("..", "..", "internal", "schematicir", "testdata", "symbols")
	output := filepath.Join(t.TempDir(), "external_connector_indicator")

	if err := run([]string{"--request", requestPath, "--symbols-root", symbolsRoot, "--output", output, "schematic-ir", "write"}, &stdout, &stderr); err != nil {
		t.Fatalf("resolver-backed schematic-ir write failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result struct {
		OK     bool            `json:"ok"`
		Issues []reports.Issue `json:"issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode resolver-backed write response: %v\n%s", err, stdout.String())
	}
	if !result.OK {
		t.Fatalf("expected resolver-backed write response to be OK: %s", stdout.String())
	}
	for _, issue := range result.Issues {
		if issue.Blocking() {
			t.Fatalf("unexpected blocking resolver issue: %+v", issue)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "external_connector_indicator.kicad_sch")); err != nil {
		t.Fatalf("expected resolver-backed schematic: %v", err)
	}
}

func TestSchematicIRRelevantLibraryIssuesIgnoreUnreferencedRecords(t *testing.T) {
	document := schematicir.Document{Circuit: schematicir.Circuit{Components: []schematicir.Component{{Symbol: "Connector_Generic:Conn_02x02_Odd_Even"}}}}
	issues := []reports.Issue{
		{Path: "roots.symbols_root", Severity: reports.SeverityWarning},
		{Path: "library.symbol.Connector_Generic:Conn_02x02_Odd_Even", Severity: reports.SeverityError},
		{Path: "library.symbol.4xxx:Broken", Severity: reports.SeverityError},
		{Path: "library.footprint.Other:Broken", Severity: reports.SeverityError},
	}
	filtered := schematicIRRelevantLibraryIssues(document, libraryresolver.LibraryIndex{}, issues)
	if len(filtered) != 2 || filtered[1].Path != "library.symbol.Connector_Generic:Conn_02x02_Odd_Even" {
		t.Fatalf("filtered library issues = %#v", filtered)
	}
}

func TestSchematicIRRequiresRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"schematic-ir", "validate"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected missing request error")
	}
	if !strings.Contains(stdout.String(), `"schematic-ir.validate"`) || !strings.Contains(stdout.String(), `"schematic-ir requires --request"`) {
		t.Fatalf("unexpected missing request response:\n%s", stdout.String())
	}
}

func appWithClientFactory(factory func(context.Context, config.Config) (apiClient, error)) app {
	return app{newClient: factory}
}
