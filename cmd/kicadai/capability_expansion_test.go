package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/capabilityexpansion"
	"kicadai/internal/capabilitygate"
	"kicadai/internal/reports"
)

func TestCapabilityExpansionPlanCLIIsDeterministic(t *testing.T) {
	evidenceHash := sha256.Sum256([]byte("normalized requirement"))
	assessment, err := capabilitygate.Assess(capabilitygate.Input{
		Stage: "architecture_selection",
		Requirements: []capabilitygate.Requirement{{
			Kind: capabilitygate.RequirementArchitecture, ID: "precision_buffering",
			Description: "reusable precision buffering", EvidenceIDs: []string{"domain"},
		}},
		Evidence: []capabilitygate.Evidence{{
			ID: "domain", Kind: "normalized_requirement", Status: capabilitygate.EvidenceVerified,
			Source: "requirement://domain", Digest: hex.EncodeToString(evidenceHash[:]),
		}},
		Gaps: []capabilitygate.Gap{{
			Code: "ARCHITECTURE_CAPABILITY_UNSUPPORTED", Kind: capabilitygate.RequirementArchitecture,
			ID: "precision_buffering", Stage: "architecture_selection", Reason: "no provider",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessmentData, err := assessment.MarshalJSONStable()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	assessmentPath := filepath.Join(tempDir, "assessment.json")
	planPath := filepath.Join(tempDir, "plan.json")
	if err := os.WriteFile(assessmentPath, assessmentData, 0o600); err != nil {
		t.Fatal(err)
	}

	runPlan := func() ([]byte, reports.Result) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		err := run([]string{
			"--request", assessmentPath, "--output", planPath,
			"capability", "expansion", "plan",
		}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("run expansion plan: %v; stderr=%s", err, stderr.String())
		}
		var result reports.Result
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(planPath)
		if err != nil {
			t.Fatal(err)
		}
		return data, result
	}

	first, result := runPlan()
	second, _ := runPlan()
	if !bytes.Equal(first, second) {
		t.Fatal("identical unsupported assessment produced different plan bytes")
	}
	if !result.OK || result.Command != "capability.expansion.plan" {
		t.Fatalf("result = %#v", result)
	}
	plan, err := capabilityexpansion.DecodePlan(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Needs) != 1 || plan.Needs[0].CapabilityID != "precision_buffering" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestCapabilityExpansionPlanCLIFailsClosedOnUnknownInput(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(requestPath, []byte(`{"schema":"unknown","unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{
		"--request", requestPath, "capability", "expansion", "plan",
	}, &stdout, &stderr); err == nil {
		t.Fatal("expected strict decode failure")
	}
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || len(result.Issues) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCapabilityExpansionSourcePathCannotEscapeManifestDirectory(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, "manifest")
	if err := os.Mkdir(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(root, "outside.json")
	if err := os.WriteFile(outsidePath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCapabilitySourcePath(manifestDir, filepath.Join("..", "outside.json")); err == nil {
		t.Fatal("source path traversal unexpectedly succeeded")
	}
	linkPath := filepath.Join(manifestDir, "outside-link.json")
	if err := os.Symlink(outsidePath, linkPath); err == nil {
		if _, err := resolveCapabilitySourcePath(manifestDir, "outside-link.json"); err == nil {
			t.Fatal("source symlink escape unexpectedly succeeded")
		}
	}
	insidePath := filepath.Join(manifestDir, "inside.json")
	if err := os.WriteFile(insidePath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveCapabilitySourcePath(manifestDir, "inside.json")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(insidePath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("resolved path = %q, want %q", resolved, expected)
	}
}

func TestDecodeCapabilityFileRejectsOversizedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	content := append([]byte(`{"ok":true}`), bytes.Repeat([]byte(" "), maxCapabilityRequestBytes)...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCapabilityFile[map[string]bool](path); err == nil {
		t.Fatal("oversized capability request unexpectedly decoded")
	}
}
