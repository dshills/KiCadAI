package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/simmodel"
)

const aiPromotionFixtureSchema = "kicadai.ai.promotion-fixture.v1"
const genericCircuitPromotionProfile = "generic-circuit-v1"

type aiPromotionFixtureMetadata struct {
	Schema           string `json:"schema"`
	ID               string `json:"id"`
	Prompt           string `json:"prompt"`
	RecordedResponse string `json:"recorded_response"`
	Profile          string `json:"profile,omitempty"`
	Readiness        string `json:"readiness"`
	RequireERC       bool   `json:"require_erc"`
	RequireDRC       bool   `json:"require_drc"`
	RequireRoundTrip bool   `json:"require_round_trip"`
	StrictDiffs      bool   `json:"strict_diffs"`
	ExpectedStage    string `json:"expected_stage,omitempty"`
	ExpectedIssue    string `json:"expected_issue_code,omitempty"`
}

func TestAIProviderPromotionFixtureMetadata(t *testing.T) {
	for _, id := range aiPromotionFixtureIDs() {
		t.Run(id, func(t *testing.T) {
			metadata, fixtureDir := loadAIPromotionFixture(t, id)
			if metadata.Schema != aiPromotionFixtureSchema || metadata.ID != id || (metadata.Readiness != "pass" && metadata.Readiness != "candidate") {
				t.Fatalf("promotion metadata = %#v", metadata)
			}
			if metadata.Readiness == "candidate" && (metadata.ExpectedStage == "" || metadata.ExpectedIssue == "") {
				t.Fatalf("candidate fixture lacks expected blocker metadata: %#v", metadata)
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
			if metadata.Profile == genericCircuitPromotionProfile {
				if strings.TrimSpace(os.Getenv("KICADAI_SYMBOLS_ROOT")) == "" || strings.TrimSpace(os.Getenv("KICADAI_FOOTPRINTS_ROOT")) == "" {
					t.Skip("generic circuit promotion requires KICADAI_SYMBOLS_ROOT and KICADAI_FOOTPRINTS_ROOT")
				}
			}
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
			if metadata.Profile != "" {
				args = append(args,
					"--ai-profile", metadata.Profile,
					"--catalog-dir", filepath.Join(repoRoot, "data", "components"),
				)
			}
			if strings.TrimSpace(metadata.Readiness) != "" {
				args = append(args, "--promotion-readiness", metadata.Readiness)
			}
			if metadata.RequireRoundTrip {
				args = append(args, "--require-kicad-roundtrip")
			}
			if metadata.RequireERC {
				args = append(args, "--require-erc")
			}
			if metadata.RequireDRC {
				args = append(args, "--require-drc")
			}
			if metadata.StrictDiffs {
				args = append(args, "--strict-diffs")
			}
			args = append(args, "design", "create")
			var stdout bytes.Buffer
			runErr := run(args, &stdout, &bytes.Buffer{})
			if metadata.Readiness == "pass" && runErr != nil {
				t.Fatalf("AI %s promotion: %v\n%s", id, runErr, stdout.String())
			}
			if metadata.Readiness == "candidate" && runErr == nil {
				t.Fatalf("AI %s unexpectedly passed candidate promotion\n%s", id, stdout.String())
			}
			var result struct {
				OK   bool                 `json:"ok"`
				Data aiDesignCreateResult `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode promotion result: %v", err)
			}
			if result.Data.AIStatus == nil {
				t.Fatalf("promotion result lacks AI status: ok=%v", result.OK)
			}
			var promotion designworkflow.PromotionReport
			readJSONFile(t, filepath.Join(output, designworkflow.PromotionReportArtifactPath), &promotion)
			if metadata.Readiness == "pass" {
				if !result.OK {
					t.Fatalf("promotion result ok=false status=%#v", result.Data.AIStatus)
				}
				if result.Data.AIStatus.Status != aiLaneStatusReady {
					t.Fatalf("AI status = %#v, want ready", result.Data.AIStatus)
				}
				if promotion.Status != designworkflow.PromotionStatusPass {
					t.Fatalf("promotion status = %q, want pass", promotion.Status)
				}
				if promotion.DeclaredReadiness != designworkflow.PromotionReadinessPass {
					t.Fatalf("declared readiness = %q, want pass", promotion.DeclaredReadiness)
				}
				if promotion.AchievedReadiness != designworkflow.PromotionReadinessPass {
					t.Fatalf("achieved readiness = %q, want pass", promotion.AchievedReadiness)
				}
				if !promotion.MatchesExpectation {
					t.Fatal("promotion does not match declared pass readiness")
				}
				if id == "generic_filtered_divider_hierarchy" || id == "generic_mna_buffered_two_pole" || id == "generic_nonlinear_npn_bias" {
					assertTrustedHierarchyReplay(t, output, cliPath, repoRoot, metadata, id)
				}
				return
			}
			if result.OK || result.Data.AIStatus.Stage != metadata.ExpectedStage || string(result.Data.AIStatus.IssueCode) != metadata.ExpectedIssue || promotion.Status == designworkflow.PromotionStatusPass {
				t.Fatalf("candidate result ok=%v status=%#v promotion=%q, want %s/%s", result.OK, result.Data.AIStatus, promotion.Status, metadata.ExpectedStage, metadata.ExpectedIssue)
			}
		})
	}
}

func assertTrustedHierarchyReplay(t *testing.T, output, cliPath, repoRoot string, metadata aiPromotionFixtureMetadata, fixtureID string) {
	t.Helper()
	var promotion designworkflow.PromotionReport
	readJSONFile(t, filepath.Join(output, designworkflow.PromotionReportArtifactPath), &promotion)
	gates := make(map[string]designworkflow.PromotionGate, len(promotion.Gates))
	for _, gate := range promotion.Gates {
		gates[gate.ID] = gate
	}
	for _, id := range []string{"connectivity", "kicad_checks", "route_completion", "schematic_electrical", "simulation", "stages", "writer_correctness"} {
		if gates[id].Status != designworkflow.PromotionGateStatusPass {
			t.Fatalf("required held-out promotion gate %s = %#v", id, gates[id])
		}
	}
	var simulation simmodel.Report
	readJSONFile(t, filepath.Join(output, designworkflow.ExplicitSimulationArtifactPath), &simulation)
	expectedModel := simmodel.ModelResistorDividerDCV1
	if fixtureID == "generic_mna_buffered_two_pole" {
		expectedModel = simmodel.ModelLinearCircuitMNAV1
	} else if fixtureID == "generic_nonlinear_npn_bias" {
		expectedModel = simmodel.ModelNonlinearCircuitDCV1
	}
	if simulation.Status != "pass" || simulation.ModelID != expectedModel || simulation.RegistryHash != simmodel.RegistryHash() || simulation.CatalogHash == "" {
		t.Fatalf("trusted simulation evidence = %#v", simulation)
	}
	if expectedModel == simmodel.ModelLinearCircuitMNAV1 || expectedModel == simmodel.ModelNonlinearCircuitDCV1 {
		minimumDevices, expectedAnalyses := 7, 2
		if expectedModel == simmodel.ModelNonlinearCircuitDCV1 {
			minimumDevices, expectedAnalyses = 6, 1
		}
		if simulation.TopologyHash == "" || len(simulation.Devices) < minimumDevices || len(simulation.Analyses) != expectedAnalyses || len(simulation.Assertions) < 4 {
			t.Fatalf("graph MNA evidence is incomplete: %#v", simulation)
		}
		for _, assertion := range simulation.Assertions {
			if !assertion.Pass {
				t.Fatalf("graph MNA assertion did not pass: %#v", assertion)
			}
		}
		if expectedModel == simmodel.ModelNonlinearCircuitDCV1 && (len(simulation.Analyses) != 1 || len(simulation.Analyses[0].Points) != 1 || simulation.Analyses[0].Points[0].Solver == nil) {
			t.Fatalf("nonlinear convergence evidence is incomplete: %#v", simulation)
		}
	}
	files := generatedKiCadFiles(t, output)
	childCount := 0
	for _, relative := range files {
		if strings.HasPrefix(filepath.ToSlash(relative), "sch/") && strings.HasSuffix(relative, ".kicad_sch") {
			childCount++
		}
	}
	if fixtureID != "generic_nonlinear_npn_bias" && childCount < 2 {
		t.Fatalf("automatic hierarchy child count = %d, files=%v", childCount, files)
	}
	replayPath := filepath.Join(output, filepath.FromSlash(aiReplayArtifactRelativePath))
	replayOutput := filepath.Join(t.TempDir(), "recorded-replay")
	replayArgs := []string{
		"--provider", "recorded", "--provider-record", replayPath,
		"--ai-profile", genericCircuitPromotionProfile,
		"--catalog-dir", filepath.Join(repoRoot, "data", "components"),
		"--promotion-readiness", metadata.Readiness,
		"--output", replayOutput, "--overwrite", "--kicad-cli", cliPath,
		"--require-kicad-roundtrip", "--require-erc", "--require-drc", "--strict-diffs",
		"design", "create",
	}
	var stdout bytes.Buffer
	if err := run(replayArgs, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("recorded replay failed: %v\n%s", err, stdout.String())
	}
	replayFiles := generatedKiCadFiles(t, replayOutput)
	if strings.Join(files, "\n") != strings.Join(replayFiles, "\n") {
		t.Fatalf("recorded replay file set differs\nfirst=%v\nreplay=%v", files, replayFiles)
	}
	for _, relative := range files {
		first, err := os.ReadFile(filepath.Join(output, relative))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(replayOutput, relative))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("recorded replay differs for %s", relative)
		}
	}
	firstSimulation, err := os.ReadFile(filepath.Join(output, designworkflow.ExplicitSimulationArtifactPath))
	if err != nil {
		t.Fatal(err)
	}
	secondSimulation, err := os.ReadFile(filepath.Join(replayOutput, designworkflow.ExplicitSimulationArtifactPath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstSimulation, secondSimulation) {
		t.Fatal("recorded replay trusted simulation artifact differs")
	}
}

func generatedKiCadFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		extension := filepath.Ext(path)
		if _, tracked := generatedKiCadExtensions[extension]; !tracked {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

var generatedKiCadExtensions = map[string]struct{}{
	".kicad_sch": {},
	".kicad_pcb": {},
	".kicad_pro": {},
	".kicad_prl": {},
	".kicad_dru": {},
	".kicad_wks": {},
}

func aiPromotionFixtureIDs() []string {
	return []string{
		"generic_dual_lmv321_signal_conditioner",
		"generic_filtered_divider_hierarchy",
		"generic_lm358_buffered_signal_conditioner",
		"generic_lmv321_ac_gain_stage",
		"generic_mna_buffered_two_pole",
		"generic_nonlinear_npn_bias",
		"generic_rc_filter",
		"generic_usb_c_bmp280_breakout",
		"generic_usb_c_led_indicator_protected",
		"usb_c_bmp280_breakout",
		"usb_c_led_indicator_protected",
	}
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
