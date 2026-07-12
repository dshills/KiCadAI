package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/checks"
)

const aiPromotionFixtureSchema = "kicadai.ai.promotion-fixture.v1"

type aiPromotionFixtureMetadata struct {
	Schema           string `json:"schema"`
	ID               string `json:"id"`
	Prompt           string `json:"prompt"`
	RecordedResponse string `json:"recorded_response"`
	Readiness        string `json:"readiness"`
	RequireERC       bool   `json:"require_erc"`
	RequireDRC       bool   `json:"require_drc"`
	RequireRoundTrip bool   `json:"require_round_trip"`
	StrictDiffs      bool   `json:"strict_diffs"`
}

func TestAIProviderPromotionFixtureMetadata(t *testing.T) {
	for _, id := range aiPromotionFixtureIDs() {
		t.Run(id, func(t *testing.T) {
			metadata, fixtureDir := loadAIPromotionFixture(t, id)
			if metadata.Schema != aiPromotionFixtureSchema || metadata.ID != id || metadata.Readiness != "pass" {
				t.Fatalf("promotion metadata = %#v", metadata)
			}
			if !metadata.RequireERC || !metadata.RequireDRC || !metadata.RequireRoundTrip || !metadata.StrictDiffs {
				t.Fatalf("promotion gates = %#v, want all strict gates enabled", metadata)
			}
			for _, name := range []string{metadata.Prompt, metadata.RecordedResponse} {
				if filepath.Base(name) != name || name == "." || name == "" {
					t.Fatalf("unsafe fixture path %q", name)
				}
				if _, err := os.Stat(filepath.Join(fixtureDir, name)); err != nil {
					t.Fatalf("fixture path %q: %v", name, err)
				}
			}
		})
	}
}

func TestAIProviderOptionalKiCadPromotion(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	if cliPath == "" {
		t.Skipf("set %s to run AI provider KiCad-backed promotion fixtures", checks.EnvKiCadCLI)
	}
	for _, id := range aiPromotionFixtureIDs() {
		t.Run(id, func(t *testing.T) {
			metadata, fixtureDir := loadAIPromotionFixture(t, id)
			repoRoot := aiPromotionRepoRoot(t)
			generatedRoot := filepath.Join(repoRoot, "examples", ".generated")
			if err := os.MkdirAll(generatedRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			workspace, err := os.MkdirTemp(generatedRoot, "ai_"+id+"_promotion-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(workspace) })
			output := filepath.Join(workspace, "project")
			args := []string{
				"--prompt-file", filepath.Join(fixtureDir, metadata.Prompt),
				"--provider", "recorded",
				"--provider-record", filepath.Join(fixtureDir, metadata.RecordedResponse),
				"--output", output,
				"--overwrite",
				"--kicad-cli", cliPath,
			}
			if metadata.RequireRoundTrip {
				args = append(args, "--require-kicad-roundtrip")
			}
			if metadata.StrictDiffs {
				args = append(args, "--strict-diffs")
			}
			args = append(args, "design", "create")
			var stdout bytes.Buffer
			if err := run(args, &stdout, &bytes.Buffer{}); err != nil {
				t.Fatalf("AI %s promotion: %v\n%s", id, err, stdout.String())
			}
			var result struct {
				OK   bool                 `json:"ok"`
				Data aiDesignCreateResult `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode promotion result: %v", err)
			}
			if !result.OK || result.Data.AIStatus == nil || result.Data.AIStatus.Status != aiLaneStatusReady {
				t.Fatalf("promotion result ok=%v status=%#v, want ready", result.OK, result.Data.AIStatus)
			}
			var promotion designworkflow.PromotionReport
			readJSONFile(t, filepath.Join(output, designworkflow.PromotionReportArtifactPath), &promotion)
			if promotion.Status != designworkflow.PromotionStatusPass {
				t.Fatalf("promotion status = %q, gates=%#v", promotion.Status, promotion.Gates)
			}
		})
	}
}

func aiPromotionFixtureIDs() []string {
	return []string{"usb_c_bmp280_breakout", "usb_c_led_indicator_protected"}
}

func loadAIPromotionFixture(t *testing.T, id string) (aiPromotionFixtureMetadata, string) {
	t.Helper()
	fixtureDir := filepath.Join(aiPromotionRepoRoot(t), "examples", "ai", id)
	file, err := os.Open(filepath.Join(fixtureDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var metadata aiPromotionFixtureMetadata
	if err := decoder.Decode(&metadata); err != nil {
		t.Fatalf("decode promotion metadata: %v", err)
	}
	if decoder.Decode(&struct{}{}) == nil {
		t.Fatal("promotion metadata contains trailing JSON")
	}
	return metadata, fixtureDir
}

func aiPromotionRepoRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate AI promotion test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", ".."))
}
