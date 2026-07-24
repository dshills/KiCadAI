package architecturesearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/reports"
)

const (
	standaloneClockCorpusSchema      = "kicadai.standalone-clock-generation-corpus.v1"
	standaloneClockCorpusBaseCommit  = "0cd2b17aa725839b192f2f4bbb82538c963c6ebc"
	standaloneClockCorpusManifestSHA = "7d75d83e8f50ee9780b40472a106aff06462f9b0a42f7d9509005f157c94deaa"
	standaloneClockBaselineSchema    = "kicadai.standalone-clock-generation-baseline.v1"
	standaloneClockBaselineSHA       = "789a69b718c215b9f4ee6e4edbc9fc57a34118ea01c65333362fc3512fc95246"
	standaloneClockBaselineModeEnv   = "KICADAI_STANDALONE_CLOCK_BASELINE"
	standaloneClockBaselineReportEnv = "KICADAI_STANDALONE_CLOCK_REPORT"
)

var standaloneClockStages = []string{
	"integrity",
	"schema",
	"intent",
	"architecture",
	"component_evidence",
	"simulation",
	"lowering",
	"schematic",
	"placement",
	"routing",
	"writer",
	"erc",
	"drc",
	"round_trip",
	"replay",
}

type standaloneClockManifest struct {
	Schema            string                `json:"schema"`
	Version           int                   `json:"version"`
	BaseCommit        string                `json:"base_commit"`
	FrozenAt          string                `json:"frozen_at"`
	RequirementSchema string                `json:"requirement_schema"`
	AuthoringPolicy   string                `json:"authoring_policy"`
	Stages            []string              `json:"stages"`
	Cases             []standaloneClockCase `json:"cases"`
}

type standaloneClockCase struct {
	ID                string `json:"id"`
	Domain            string `json:"domain"`
	Family            string `json:"family"`
	Prompt            string `json:"prompt"`
	PromptSHA256      string `json:"prompt_sha256"`
	RequirementFile   string `json:"requirement_file"`
	RequirementSHA256 string `json:"requirement_sha256"`
	SafetyCritical    bool   `json:"safety_critical"`
}

type standaloneClockBaselineCase struct {
	ID         string `json:"id"`
	Family     string `json:"family"`
	Status     string `json:"status"`
	Stage      string `json:"blocking_stage"`
	Code       string `json:"blocking_code"`
	Path       string `json:"blocking_path"`
	Message    string `json:"blocking_message"`
	Capability string `json:"blocking_capability"`
}

type standaloneClockBaselineReport struct {
	Schema               string                        `json:"schema"`
	Version              int                           `json:"version"`
	GeneratedAt          string                        `json:"generated_at"`
	ManifestSHA256       string                        `json:"manifest_sha256"`
	CapabilityBaseCommit string                        `json:"capability_base_commit"`
	Evaluator            string                        `json:"evaluator"`
	ProviderRegistryHash string                        `json:"provider_registry_hash"`
	Cases                []standaloneClockBaselineCase `json:"cases"`
	Passed               int                           `json:"passed"`
	Blocked              int                           `json:"blocked"`
}

func TestStandaloneClockGenerationCorpusIsFrozenBeforeProductionSupport(t *testing.T) {
	root := standaloneClockCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := standaloneClockHash(manifestBytes); got != standaloneClockCorpusManifestSHA {
		t.Fatalf("manifest sha256 = %s, want %s", got, standaloneClockCorpusManifestSHA)
	}
	sidecar, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantSidecar := standaloneClockCorpusManifestSHA + "  manifest.json"
	if got := strings.TrimSpace(string(sidecar)); got != wantSidecar {
		t.Fatalf("manifest checksum sidecar = %q, want %q", got, wantSidecar)
	}

	var manifest standaloneClockManifest
	decodeStandaloneClockStrict(t, manifestBytes, &manifest)
	if manifest.Schema != standaloneClockCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != standaloneClockCorpusBaseCommit ||
		manifest.RequirementSchema != SchemaIDV3 || strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		!slices.Equal(manifest.Stages, standaloneClockStages) ||
		len(manifest.Cases) != 2 {
		t.Fatalf("manifest identity = %#v", manifest)
	}

	previousID := ""
	families := map[string]int{}
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID || entry.Domain != "digital" || entry.Family == "" {
			t.Fatalf("invalid sorted case identity: %#v after %q", entry, previousID)
		}
		previousID = entry.ID
		families[entry.Family]++
		assertHeldOutBehaviorOnlyPrompt(t, heldOutCapabilityExpansionCase{ID: entry.ID, Prompt: entry.Prompt})
		if got := standaloneClockHash([]byte(entry.Prompt)); got != entry.PromptSHA256 {
			t.Fatalf("%s prompt sha256 = %s, want %s", entry.ID, got, entry.PromptSHA256)
		}

		requirementBytes, err := os.ReadFile(filepath.Join(root, entry.RequirementFile))
		if err != nil {
			t.Fatal(err)
		}
		if got := standaloneClockHash(requirementBytes); got != entry.RequirementSHA256 {
			t.Fatalf("%s requirement sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectHeldOutCapabilityImplementationDetail(t, entry.ID, requirementBytes)
		requirement, issues := DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s strict decode issues = %#v", entry.ID, issues)
		}
		assertHeldOutCapabilityAcceptance(t, entry.ID, requirement.Acceptance)
		assertStandaloneClockBehaviorContract(t, entry.ID, requirement)
		assertHeldOutCapabilityCanonicalReplay(t, entry.ID, requirement)
	}
	if len(families) != 2 ||
		families["tight_accuracy_clock_generation"] != 1 ||
		families["relaxed_accuracy_clock_generation"] != 1 {
		t.Fatalf("clock family pressures = %#v", families)
	}
}

func TestStandaloneClockGenerationUntouchedBaselineIsFrozen(t *testing.T) {
	root := filepath.Join("..", "..", "specs", "standalone-clock-generation")
	contents, err := os.ReadFile(filepath.Join(root, "BASELINE_REPORT.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := standaloneClockHash(contents); got != standaloneClockBaselineSHA {
		t.Fatalf("baseline sha256 = %s, want %s", got, standaloneClockBaselineSHA)
	}
	sidecar, err := os.ReadFile(filepath.Join(root, "BASELINE_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantSidecar := standaloneClockBaselineSHA + "  BASELINE_REPORT.json"
	if got := strings.TrimSpace(string(sidecar)); got != wantSidecar {
		t.Fatalf("baseline checksum sidecar = %q, want %q", got, wantSidecar)
	}
	var report standaloneClockBaselineReport
	decodeStandaloneClockStrict(t, contents, &report)
	if report.Schema != standaloneClockBaselineSchema || report.Version != 1 ||
		report.ManifestSHA256 != standaloneClockCorpusManifestSHA ||
		report.CapabilityBaseCommit != standaloneClockCorpusBaseCommit ||
		report.Evaluator != "standalone-clock-architecture-v1" ||
		report.ProviderRegistryHash == "" ||
		report.Passed != 0 || report.Blocked != 2 || len(report.Cases) != 2 {
		t.Fatalf("baseline report identity = %#v", report)
	}
	for index, result := range report.Cases {
		if result.ID == "" || result.ID != []string{"precision_logic_clock", "relaxed_logic_clock"}[index] ||
			result.Status != "blocked" || result.Stage != "architecture" ||
			result.Code != string(CodeCapabilityUnsupported) ||
			result.Capability != "clock_generation" ||
			result.Path == "" || result.Message == "" {
			t.Fatalf("baseline case %d = %#v", index, result)
		}
	}
}

func TestWriteStandaloneClockGenerationBaselineReport(t *testing.T) {
	if strings.TrimSpace(os.Getenv(standaloneClockBaselineModeEnv)) != "write" {
		t.Skipf("set %s=write and %s to an output path", standaloneClockBaselineModeEnv, standaloneClockBaselineReportEnv)
	}
	output := strings.TrimSpace(os.Getenv(standaloneClockBaselineReportEnv))
	if output == "" {
		t.Fatalf("%s is required", standaloneClockBaselineReportEnv)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(standaloneClockCorpusRoot(), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest standaloneClockManifest
	decodeStandaloneClockStrict(t, manifestBytes, &manifest)
	registry, issues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	report := standaloneClockBaselineReport{
		Schema: standaloneClockBaselineSchema, Version: 1,
		GeneratedAt:          "2026-07-24T00:00:00Z",
		ManifestSHA256:       standaloneClockCorpusManifestSHA,
		CapabilityBaseCommit: standaloneClockCorpusBaseCommit,
		Evaluator:            "standalone-clock-architecture-v1",
		ProviderRegistryHash: registry.Hash(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, entry := range manifest.Cases {
		contents, readErr := os.ReadFile(filepath.Join(standaloneClockCorpusRoot(), entry.RequirementFile))
		if readErr != nil {
			t.Fatal(readErr)
		}
		requirement, decodeIssues := DecodeStrict(bytes.NewReader(contents))
		if len(decodeIssues) != 0 {
			t.Fatalf("%s decode issues = %#v", entry.ID, decodeIssues)
		}
		result := Search(ctx, requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
		issueIndex := slices.IndexFunc(result.Issues, func(issue reports.Issue) bool {
			return issue.Code == CodeCapabilityUnsupported
		})
		if result.Status != SearchUnsupported || issueIndex < 0 ||
			!rejectionSummaryContains(result.Rejections, CodeCapabilityUnsupported) {
			t.Fatalf("%s untouched search = %#v", entry.ID, result)
		}
		issue := result.Issues[issueIndex]
		report.Cases = append(report.Cases, standaloneClockBaselineCase{
			ID: entry.ID, Family: entry.Family, Status: "blocked",
			Stage: "architecture", Code: string(issue.Code), Path: issue.Path,
			Message: issue.Message, Capability: "clock_generation",
		})
		report.Blocked++
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	hash := standaloneClockHash(encoded)
	sidecar := strings.TrimSuffix(output, filepath.Ext(output)) + ".sha256"
	if err := os.WriteFile(sidecar, []byte(hash+"  "+filepath.Base(output)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s and %s sha256=%s", output, sidecar, hash)
}

func assertStandaloneClockBehaviorContract(t *testing.T, id string, requirement Requirement) {
	t.Helper()
	if len(requirement.Requirements.Objectives) != 1 ||
		requirement.Requirements.Objectives[0].Capability != "clock_generation" ||
		len(requirement.Requirements.OperatingCases) == 0 ||
		len(requirement.Requirements.BehavioralRequirements) < 4 {
		t.Fatalf("%s lacks a complete clock behavior contract", id)
	}
	constraints := map[string]bool{}
	for _, constraint := range requirement.Requirements.Objectives[0].Constraints {
		constraints[constraint.Name] = true
	}
	for _, name := range []string{
		"output_frequency",
		"duty_cycle",
		"maximum_startup_time",
		"clock_fanout",
		"maximum_rms_jitter",
	} {
		if !constraints[name] {
			t.Errorf("%s lacks %s", id, name)
		}
	}
	axes := map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			axes[condition.Axis] = true
		}
	}
	for _, axis := range []string{"supply_voltage", "load_capacitance", "ambient_temperature"} {
		if !axes[axis] {
			t.Errorf("%s lacks %s corner", id, axis)
		}
	}
	metrics := map[string]bool{}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		metrics[behavior.Metric] = true
	}
	for _, metric := range []string{"output_high_voltage", "rise_time", "fall_time", "startup_output_voltage"} {
		if !metrics[metric] {
			t.Errorf("%s lacks %s behavior", id, metric)
		}
	}
}

func standaloneClockCorpusRoot() string {
	return filepath.Join("testdata", "standalone_clock_generation_corpus")
}

func decodeStandaloneClockStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("unexpected trailing JSON: %v", err)
	}
}

func standaloneClockHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
