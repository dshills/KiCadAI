package compositionlowering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

const (
	heldOutCapabilityBaselineModeEnv   = "KICADAI_HELD_OUT_CAPABILITY_BASELINE"
	heldOutCapabilityBaselineReportEnv = "KICADAI_HELD_OUT_CAPABILITY_REPORT"
	heldOutCapabilityArtifactDirEnv    = "KICADAI_HELD_OUT_CAPABILITY_ARTIFACT_DIR"
	heldOutCapabilityBaselineSchema    = "kicadai.held-out-capability-expansion-report.v1"
	heldOutCapabilityEvaluator         = "held-out-capability-stage-v1"
	heldOutCapabilityManifestSHA256    = "e0d55f484c749eba7d3279da13c21380f5b52f9953f333adf1916409620eb442"
	heldOutCapabilityBaselineSHA256    = "ba47d125141a56deb54127c3e51ae36f2ce5df2efbd875334b7433d9ee63582c"
	heldOutCapabilityFinalSHA256       = "9541502f81f2748aded5f00383ba0dd927591c102a61ec980e4e6f17d53047c2"
	heldOutCapabilityBaseCommit        = "6e4d2209c6aae04994febca30c9ec38a40051cc2"
)

type heldOutCapabilityEvaluationManifest struct {
	Schema     string                            `json:"schema"`
	Version    int                               `json:"version"`
	BaseCommit string                            `json:"base_commit"`
	FrozenAt   string                            `json:"frozen_at"`
	Stages     []string                          `json:"stages"`
	Cases      []heldOutCapabilityEvaluationCase `json:"cases"`
}

type heldOutCapabilityEvaluationCase struct {
	ID                string `json:"id"`
	Domain            string `json:"domain"`
	Family            string `json:"family"`
	Role              string `json:"role"`
	Prompt            string `json:"prompt"`
	PromptSHA256      string `json:"prompt_sha256"`
	RequirementFile   string `json:"requirement_file"`
	RequirementSHA256 string `json:"requirement_sha256"`
	SafetyCritical    bool   `json:"safety_critical"`
}

type heldOutCapabilityStageResult struct {
	Stage   string       `json:"stage"`
	Status  string       `json:"status"`
	Code    reports.Code `json:"code,omitempty"`
	Path    string       `json:"path,omitempty"`
	Message string       `json:"message,omitempty"`
}

type heldOutCapabilityCaseResult struct {
	ID                 string                         `json:"id"`
	Domain             string                         `json:"domain"`
	Family             string                         `json:"family"`
	Role               string                         `json:"role"`
	SafetyCritical     bool                           `json:"safety_critical"`
	Status             string                         `json:"status"`
	BlockingStage      string                         `json:"blocking_stage,omitempty"`
	BlockingCode       reports.Code                   `json:"blocking_code,omitempty"`
	BlockingPath       string                         `json:"blocking_path,omitempty"`
	BlockingMessage    string                         `json:"blocking_message,omitempty"`
	BlockingCapability string                         `json:"blocking_capability,omitempty"`
	CapabilityGaps     []string                       `json:"capability_gaps,omitempty"`
	Stages             []heldOutCapabilityStageResult `json:"stages"`
	EvidenceHashes     map[string]string              `json:"evidence_hashes"`
}

type heldOutCapabilityCluster struct {
	Rank                int      `json:"rank"`
	Key                 string   `json:"key"`
	Stage               string   `json:"stage"`
	Capability          string   `json:"capability,omitempty"`
	Code                string   `json:"code"`
	CaseCount           int      `json:"case_count"`
	DomainCount         int      `json:"domain_count"`
	SafetyCriticalCount int      `json:"safety_critical_count"`
	Cases               []string `json:"cases"`
	Domains             []string `json:"domains"`
}

type heldOutCapabilityAggregate struct {
	Cases        int                        `json:"cases"`
	Passed       int                        `json:"passed"`
	Blocked      int                        `json:"blocked"`
	StageReach   map[string]int             `json:"stage_reach"`
	ByStage      map[string]int             `json:"blocked_by_stage"`
	ByCode       map[string]int             `json:"blocked_by_code"`
	ByCapability map[string]int             `json:"blocked_by_capability"`
	Clusters     []heldOutCapabilityCluster `json:"ranked_failure_clusters"`
}

type heldOutCapabilityReport struct {
	Schema               string                        `json:"schema"`
	Version              int                           `json:"version"`
	GeneratedAt          string                        `json:"generated_at"`
	BenchmarkVersion     int                           `json:"benchmark_version"`
	ManifestSHA256       string                        `json:"manifest_sha256"`
	CapabilityBaseCommit string                        `json:"capability_base_commit"`
	Evaluator            string                        `json:"evaluator"`
	GateProfile          map[string]string             `json:"gate_profile"`
	Tools                map[string]string             `json:"tools"`
	Cases                []heldOutCapabilityCaseResult `json:"cases"`
	Aggregate            heldOutCapabilityAggregate    `json:"aggregate"`
}

func TestHeldOutCapabilityExpansionBaselineReportIsFrozen(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "held-out-capability-expansion", "BASELINE_REPORT.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := heldOutCapabilityHash(contents); got != heldOutCapabilityBaselineSHA256 {
		t.Fatalf("baseline sha256 = %s, want %s", got, heldOutCapabilityBaselineSHA256)
	}
	checksum, err := os.ReadFile(filepath.Join(filepath.Dir(path), "BASELINE_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(checksum)) != heldOutCapabilityBaselineSHA256+"  BASELINE_REPORT.json" {
		t.Fatal("baseline checksum sidecar does not match frozen report")
	}
	var report heldOutCapabilityReport
	if err := json.Unmarshal(contents, &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != heldOutCapabilityBaselineSchema || report.Version != 1 ||
		report.BenchmarkVersion != 1 || report.ManifestSHA256 != heldOutCapabilityManifestSHA256 ||
		report.CapabilityBaseCommit != heldOutCapabilityBaseCommit || report.Evaluator != heldOutCapabilityEvaluator {
		t.Fatalf("baseline header = %#v", report)
	}
	if report.Aggregate.Cases != 12 || report.Aggregate.Passed != 5 ||
		report.Aggregate.Blocked != 7 || len(report.Cases) != 12 {
		t.Fatalf("baseline aggregate = %#v", report.Aggregate)
	}
	wantReach := map[string]int{
		"integrity": 12, "schema": 12, "intent": 12, "architecture": 6,
		"component_evidence": 6, "simulation": 6, "lowering": 6,
		"schematic": 6, "placement": 6, "routing": 6, "writer": 6,
		"erc": 6, "drc": 5, "round_trip": 5, "replay": 5,
	}
	for stage, want := range wantReach {
		if got := report.Aggregate.StageReach[stage]; got != want {
			t.Errorf("stage reach %s = %d, want %d", stage, got, want)
		}
	}
	if len(report.Aggregate.Clusters) < 2 ||
		report.Aggregate.Clusters[0].Rank != 1 ||
		report.Aggregate.Clusters[0].Capability != "constant_current_regulation" ||
		report.Aggregate.Clusters[0].CaseCount != 3 ||
		report.Aggregate.Clusters[0].DomainCount != 3 ||
		report.Aggregate.Clusters[1].Rank != 2 ||
		report.Aggregate.Clusters[1].Capability != "precision_rectification" ||
		report.Aggregate.Clusters[1].CaseCount != 2 ||
		report.Aggregate.Clusters[1].DomainCount != 2 {
		t.Fatalf("baseline leading clusters = %#v", report.Aggregate.Clusters)
	}
	previousID := ""
	for _, result := range report.Cases {
		if result.ID <= previousID || len(result.Stages) != len(heldOutCapabilityStageNames()) {
			t.Fatalf("invalid case ordering or stage count at %q", result.ID)
		}
		previousID = result.ID
		blocked := false
		for index, stage := range result.Stages {
			if stage.Stage != heldOutCapabilityStageNames()[index] {
				t.Fatalf("%s stage %d = %q", result.ID, index, stage.Stage)
			}
			switch stage.Status {
			case "pass":
				if blocked {
					t.Fatalf("%s reaches %s after blocker", result.ID, stage.Stage)
				}
			case "blocked":
				if blocked || result.Status != "blocked" || stage.Stage != result.BlockingStage ||
					stage.Code != result.BlockingCode || stage.Message == "" {
					t.Fatalf("%s invalid blocking stage %#v", result.ID, stage)
				}
				blocked = true
			case "not_reached":
				if !blocked {
					t.Fatalf("%s marks %s not_reached before blocker", result.ID, stage.Stage)
				}
			default:
				t.Fatalf("%s stage %s has invalid status %q", result.ID, stage.Stage, stage.Status)
			}
		}
		if result.Status == "pass" {
			if blocked || result.EvidenceHashes["generated_project"] == "" ||
				result.EvidenceHashes["closed_loop"] == "" {
				t.Fatalf("%s pass lacks complete evidence: %#v", result.ID, result)
			}
		} else if !blocked || len(result.CapabilityGaps) == 0 {
			t.Fatalf("%s blocker lacks capability accounting: %#v", result.ID, result)
		}
	}
	for _, key := range []string{
		"catalog_hash", "installed_library_summary", "kicad_cli", "kicad_version",
		"model_registry_hash", "provider_registry_hash",
	} {
		if strings.TrimSpace(report.Tools[key]) == "" {
			t.Errorf("baseline tool evidence %s is missing", key)
		}
	}
}

func TestHeldOutCapabilityExpansionFinalReportProvesImprovement(t *testing.T) {
	root := filepath.Join("..", "..", "specs", "held-out-capability-expansion")
	loadReport := func(name string) ([]byte, heldOutCapabilityReport) {
		t.Helper()
		contents, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var report heldOutCapabilityReport
		if err := json.Unmarshal(contents, &report); err != nil {
			t.Fatal(err)
		}
		return contents, report
	}

	_, baseline := loadReport("BASELINE_REPORT.json")
	finalBytes, final := loadReport("FINAL_REPORT.json")
	if got := heldOutCapabilityHash(finalBytes); got != heldOutCapabilityFinalSHA256 {
		t.Fatalf("final report sha256 = %s, want %s", got, heldOutCapabilityFinalSHA256)
	}
	checksum, err := os.ReadFile(filepath.Join(root, "FINAL_REPORT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(checksum)) != heldOutCapabilityFinalSHA256+"  FINAL_REPORT.json" {
		t.Fatal("final report checksum sidecar does not match frozen report")
	}
	if final.Schema != baseline.Schema || final.Version != baseline.Version ||
		final.BenchmarkVersion != baseline.BenchmarkVersion ||
		final.ManifestSHA256 != baseline.ManifestSHA256 ||
		final.CapabilityBaseCommit != baseline.CapabilityBaseCommit ||
		final.Evaluator != baseline.Evaluator {
		t.Fatalf("final report identity does not match baseline: baseline=%#v final=%#v", baseline, final)
	}
	if !maps.Equal(final.GateProfile, baseline.GateProfile) {
		t.Fatalf("final gate profile changed: baseline=%#v final=%#v", baseline.GateProfile, final.GateProfile)
	}
	if final.Aggregate.Cases != baseline.Aggregate.Cases ||
		final.Aggregate.Passed <= baseline.Aggregate.Passed ||
		final.Aggregate.Passed != 11 || final.Aggregate.Blocked != 1 {
		t.Fatalf("final aggregate does not prove the expected improvement: baseline=%#v final=%#v", baseline.Aggregate, final.Aggregate)
	}
	baselineReach, finalReach := 0, 0
	for _, stage := range heldOutCapabilityStageNames() {
		baselineReach += baseline.Aggregate.StageReach[stage]
		finalReach += final.Aggregate.StageReach[stage]
		if final.Aggregate.StageReach[stage] < baseline.Aggregate.StageReach[stage] {
			t.Errorf("stage reach regressed at %s: baseline=%d final=%d",
				stage, baseline.Aggregate.StageReach[stage], final.Aggregate.StageReach[stage])
		}
	}
	if finalReach <= baselineReach {
		t.Fatalf("cumulative stage reach did not improve: baseline=%d final=%d", baselineReach, finalReach)
	}

	baselineCases := make(map[string]heldOutCapabilityCaseResult, len(baseline.Cases))
	for _, result := range baseline.Cases {
		baselineCases[result.ID] = result
	}
	promotedFamilies := map[string]int{
		"constant_current_regulation": 0,
		"precision_rectification":     0,
	}
	for _, result := range final.Cases {
		original, ok := baselineCases[result.ID]
		if !ok || result.Role != original.Role || result.Domain != original.Domain ||
			result.Family != original.Family || result.SafetyCritical != original.SafetyCritical {
			t.Fatalf("case identity changed for %q: baseline=%#v final=%#v", result.ID, original, result)
		}
		if result.Role == "control" && result.Status != "pass" {
			t.Errorf("control %s regressed: %#v", result.ID, result)
		}
		if _, promoted := promotedFamilies[result.Family]; promoted {
			if result.Role != "held_out" || result.Status != "pass" {
				t.Errorf("promoted family case %s did not pass: %#v", result.ID, result)
			}
			promotedFamilies[result.Family]++
		}
		if result.Status == "pass" {
			if len(result.Stages) != len(heldOutCapabilityStageNames()) ||
				result.EvidenceHashes["generated_project"] == "" ||
				result.EvidenceHashes["closed_loop"] == "" {
				t.Errorf("passing case %s lacks full evidence: %#v", result.ID, result)
			}
			for _, stage := range result.Stages {
				if stage.Status != "pass" {
					t.Errorf("passing case %s has non-passing stage %#v", result.ID, stage)
				}
			}
			continue
		}
		if result.ID != "digital_clock_source" ||
			result.BlockingStage != "architecture" ||
			result.BlockingCode != architecturesearch.CodeCapabilityUnsupported ||
			result.BlockingCapability != "clock_generation" {
			t.Errorf("unexpected remaining blocker: %#v", result)
		}
	}
	if len(baselineCases) != len(final.Cases) {
		t.Fatalf("case membership changed: baseline=%d final=%d", len(baselineCases), len(final.Cases))
	}
	if promotedFamilies["constant_current_regulation"] != 3 ||
		promotedFamilies["precision_rectification"] != 2 {
		t.Fatalf("promoted family coverage = %#v", promotedFamilies)
	}
	if len(final.Aggregate.Clusters) != 1 ||
		final.Aggregate.Clusters[0].Capability != "clock_generation" ||
		final.Aggregate.Clusters[0].Stage != "architecture" ||
		final.Aggregate.Clusters[0].Code != string(architecturesearch.CodeCapabilityUnsupported) ||
		final.Aggregate.Clusters[0].CaseCount != 1 {
		t.Fatalf("remaining failure clusters = %#v", final.Aggregate.Clusters)
	}
}

func TestWriteHeldOutCapabilityExpansionBaselineReport(t *testing.T) {
	if strings.TrimSpace(os.Getenv(heldOutCapabilityBaselineModeEnv)) != "write" {
		t.Skipf("set %s=write and %s to an output path", heldOutCapabilityBaselineModeEnv, heldOutCapabilityBaselineReportEnv)
	}
	reportPath := strings.TrimSpace(os.Getenv(heldOutCapabilityBaselineReportEnv))
	if reportPath == "" {
		t.Fatalf("%s is required", heldOutCapabilityBaselineReportEnv)
	}
	report := evaluateHeldOutCapabilityExpansion(t)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(reportPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s sha256=%s aggregate=%#v", reportPath, heldOutCapabilityHash(encoded), report.Aggregate)
}

func evaluateHeldOutCapabilityExpansion(t *testing.T) heldOutCapabilityReport {
	t.Helper()
	root := filepath.Join("..", "architecturesearch", "testdata", "held_out_capability_expansion_corpus")
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := heldOutCapabilityHash(manifestBytes); got != heldOutCapabilityManifestSHA256 {
		t.Fatalf("manifest sha256 = %s, want %s", got, heldOutCapabilityManifestSHA256)
	}
	var manifest heldOutCapabilityEvaluationManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || len(manifest.Cases) != 12 || !slices.Equal(manifest.Stages, heldOutCapabilityStageNames()) {
		t.Fatalf("unexpected frozen manifest identity: %#v", manifest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	cli := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if cli == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Fatalf("set %s, %s, and %s for the complete baseline", checks.EnvKiCadCLI, libraryresolver.EnvSymbolsRoot, libraryresolver.EnvFootprintsRoot)
	}
	index, _ := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot: symbolsRoot, FootprintsRoot: footprintsRoot,
	}, libraryresolver.LoadOptions{})
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		t.Fatalf("installed library resolution produced no usable symbols or footprints: %#v", libraryresolver.Summary(index))
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(registryIssues) {
		t.Fatalf("catalog registry issues: %#v", registryIssues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, provenanceDiagnostics := modelprovenance.LoadDefault()
	if len(provenanceDiagnostics) != 0 {
		t.Fatalf("model provenance diagnostics: %#v", provenanceDiagnostics)
	}
	modelRegistryHash, err := modelprovenance.Hash(provenance)
	if err != nil {
		t.Fatal(err)
	}

	report := heldOutCapabilityReport{
		Schema: heldOutCapabilityBaselineSchema, Version: 1, GeneratedAt: manifest.FrozenAt,
		BenchmarkVersion: 1, ManifestSHA256: heldOutCapabilityManifestSHA256,
		CapabilityBaseCommit: heldOutCapabilityBaseCommit, Evaluator: heldOutCapabilityEvaluator,
		GateProfile: map[string]string{
			"all_declared_corners": "required", "byte_identical_replay": "required",
			"clean_erc": "required", "connectivity": "required", "deterministic_architecture": "required",
			"strict_drc": "required", "trusted_simulation": "required", "verified_component_evidence": "required",
			"writer_correctness": "required", "zero_round_trip_diffs": "required",
		},
		Tools: map[string]string{
			"catalog_hash": resolver.CatalogHash(), "kicad_cli": cli,
			"installed_library_summary": heldOutCapabilityJSONHash(libraryresolver.Summary(index)),
			"kicad_version":             heldOutCapabilityToolVersion(t, cli),
			"model_registry_hash":       modelRegistryHash, "provider_registry_hash": registry.Hash(),
		},
		Aggregate: heldOutCapabilityAggregate{
			StageReach: map[string]int{}, ByStage: map[string]int{}, ByCode: map[string]int{}, ByCapability: map[string]int{},
		},
	}
	for _, entry := range manifest.Cases {
		result := evaluateHeldOutCapabilityCase(t, ctx, root, entry, manifest.Stages, registry, resolver, provenance, modelRegistryHash, index, cli)
		report.Cases = append(report.Cases, result)
	}
	slices.SortFunc(report.Cases, func(left, right heldOutCapabilityCaseResult) int {
		return strings.Compare(left.ID, right.ID)
	})
	report.Aggregate = aggregateHeldOutCapabilityResults(report.Cases, manifest.Stages)
	return report
}

func evaluateHeldOutCapabilityCase(
	t *testing.T,
	ctx context.Context,
	root string,
	entry heldOutCapabilityEvaluationCase,
	stages []string,
	registry *architecturesearch.Registry,
	resolver *circuitgraph.Resolver,
	provenance modelprovenance.Registry,
	modelRegistryHash string,
	index libraryresolver.LibraryIndex,
	cli string,
) heldOutCapabilityCaseResult {
	t.Helper()
	result := newHeldOutCapabilityCaseResult(entry, stages)
	if heldOutCapabilityHash([]byte(entry.Prompt)) != entry.PromptSHA256 {
		return blockHeldOutCapabilityCase(result, "integrity", reports.Issue{
			Code: reports.CodeInvalidArgument, Path: "prompt_sha256", Message: "prompt hash does not match frozen manifest",
		}, "")
	}
	requirementPath := filepath.Join(root, entry.RequirementFile)
	requirementBytes, err := os.ReadFile(requirementPath)
	if err != nil {
		return blockHeldOutCapabilityCase(result, "integrity", reports.Issue{
			Code: reports.CodeInvalidArgument, Path: "requirement_file", Message: err.Error(),
		}, "")
	}
	if heldOutCapabilityHash(requirementBytes) != entry.RequirementSHA256 {
		return blockHeldOutCapabilityCase(result, "integrity", reports.Issue{
			Code: reports.CodeInvalidArgument, Path: "requirement_sha256", Message: "requirement hash does not match frozen manifest",
		}, "")
	}
	passHeldOutCapabilityStage(&result, "integrity")
	result.EvidenceHashes["requirement_bytes"] = entry.RequirementSHA256

	requirement, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(requirementBytes))
	if reports.HasBlockingIssue(decodeIssues) {
		return blockHeldOutCapabilityCase(result, "schema", firstHeldOutBlockingIssue(decodeIssues, "strict requirement decode failed"), "")
	}
	passHeldOutCapabilityStage(&result, "schema")
	requirementHash, err := architecturesearch.CanonicalHash(requirement)
	if err != nil {
		return blockHeldOutCapabilityCase(result, "schema", reports.Issue{
			Code: reports.CodeValidationFailed, Path: "requirement", Message: err.Error(),
		}, "")
	}
	result.EvidenceHashes["requirement_canonical"] = requirementHash
	if !heldOutCapabilityFullAcceptance(requirement.Acceptance) {
		return blockHeldOutCapabilityCase(result, "intent", reports.Issue{
			Code: architecturesearch.CodeAcceptanceInvalid, Path: "acceptance", Message: "complete benchmark acceptance profile is required",
		}, "")
	}
	passHeldOutCapabilityStage(&result, "intent")

	search := architecturesearch.Search(ctx, requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
	result.EvidenceHashes["search"] = heldOutCapabilityJSONHash(search)
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		gaps := heldOutCapabilitySearchGaps(search)
		result.CapabilityGaps = make([]string, 0, len(gaps))
		for _, gap := range gaps {
			result.CapabilityGaps = append(result.CapabilityGaps, gap.Capability)
		}
		capability, path := "", ""
		if len(gaps) != 0 {
			capability, path = gaps[0].Capability, gaps[0].Path
		}
		issue := firstHeldOutBlockingIssue(search.Issues, "architecture search did not select a complete candidate")
		if issue.Path == "" {
			issue.Path = path
		}
		return blockHeldOutCapabilityCase(result, "architecture", issue, capability)
	}
	passHeldOutCapabilityStage(&result, "architecture")
	result.EvidenceHashes["selected_architecture"] = search.Selected.Fingerprint

	promotion, promotionIssues := SynthesizeClosedLoop(ctx, requirement, search, ArchitectureSimulationPlanResolver{
		GraphResolver: resolver, ProvenanceRegistry: provenance,
	}, modelRegistryHash, nil, closedloopsynthesis.DefaultPolicy())
	if reports.HasBlockingIssue(promotionIssues) || promotion.Report.Status != "pass" {
		issue := firstHeldOutBlockingIssue(promotionIssues, "closed-loop synthesis did not pass")
		stage := heldOutCapabilityClosedLoopStage(issue)
		return blockHeldOutCapabilityCase(result, stage, issue, entry.Family)
	}
	if promotion.Request.ExplicitCircuit == nil || promotion.Request.ExplicitCircuit.ClosedLoop == nil {
		return blockHeldOutCapabilityCase(result, "lowering", reports.Issue{
			Code: reports.CodeValidationFailed, Path: "request.explicit_circuit.closed_loop", Message: "closed-loop evidence was not bound to the lowered request",
		}, entry.Family)
	}
	passHeldOutCapabilityStage(&result, "component_evidence")
	passHeldOutCapabilityStage(&result, "simulation")
	passHeldOutCapabilityStage(&result, "lowering")
	result.EvidenceHashes["closed_loop"] = heldOutCapabilityJSONHash(promotion.Report)
	result.EvidenceHashes["resolved_circuit"] = promotion.Request.ExplicitCircuit.ResolutionHash

	artifactRoot := t.TempDir()
	if configured := strings.TrimSpace(os.Getenv(heldOutCapabilityArtifactDirEnv)); configured != "" {
		artifactRoot = filepath.Join(configured, entry.ID)
		if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
			t.Fatalf("create held-out artifact root: %v", err)
		}
		requestData, err := json.MarshalIndent(promotion.Request, "", "  ")
		if err != nil {
			t.Fatalf("marshal held-out design request: %v", err)
		}
		if err := os.WriteFile(filepath.Join(artifactRoot, "design-request.json"), append(requestData, '\n'), 0o644); err != nil {
			t.Fatalf("write held-out design request: %v", err)
		}
	}
	firstDir := filepath.Join(artifactRoot, "first")
	first := designworkflow.Create(ctx, promotion.Request, heldOutCapabilityCreateOptions(firstDir, index, cli))
	if strings.TrimSpace(os.Getenv(heldOutCapabilityArtifactDirEnv)) != "" {
		writeHeldOutCapabilityArtifactJSON(t, filepath.Join(artifactRoot, "first-workflow-result.json"), first)
	}
	passedWorkflowStages, stage, issue, failed := heldOutCapabilityWorkflowFailure(first)
	for _, passed := range passedWorkflowStages {
		passHeldOutCapabilityStage(&result, passed)
	}
	if failed {
		return blockHeldOutCapabilityCase(result, stage, issue, entry.Family)
	}
	passHeldOutCapabilityStage(&result, "erc")
	passHeldOutCapabilityStage(&result, "drc")
	passHeldOutCapabilityStage(&result, "round_trip")
	generatedHash, err := heldOutCapabilityGeneratedHash(firstDir, promotion.Request.Name)
	if err != nil {
		return blockHeldOutCapabilityCase(result, "writer", reports.Issue{
			Code: reports.CodeValidationFailed, Path: "generated_project", Message: err.Error(),
		}, entry.Family)
	}
	result.EvidenceHashes["generated_project"] = generatedHash

	secondDir := filepath.Join(artifactRoot, "second")
	second := designworkflow.Create(ctx, promotion.Request, heldOutCapabilityCreateOptions(secondDir, index, cli))
	if strings.TrimSpace(os.Getenv(heldOutCapabilityArtifactDirEnv)) != "" {
		writeHeldOutCapabilityArtifactJSON(t, filepath.Join(artifactRoot, "second-workflow-result.json"), second)
	}
	if _, _, issue, failed := heldOutCapabilityWorkflowFailure(second); failed {
		issue.Message = "deterministic replay workflow failed: " + issue.Message
		return blockHeldOutCapabilityCase(result, "replay", issue, entry.Family)
	}
	if err := heldOutCapabilityCompareProjects(firstDir, secondDir, promotion.Request.Name); err != nil {
		return blockHeldOutCapabilityCase(result, "replay", reports.Issue{
			Code: reports.CodeValidationFailed, Path: "project_replay", Message: err.Error(),
		}, entry.Family)
	}
	passHeldOutCapabilityStage(&result, "replay")
	result.Status = "pass"
	return result
}

func writeHeldOutCapabilityArtifactJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal held-out capability artifact %s: %v", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write held-out capability artifact %s: %v", filepath.Base(path), err)
	}
}

func heldOutCapabilityStageNames() []string {
	return []string{
		"integrity", "schema", "intent", "architecture", "component_evidence",
		"simulation", "lowering", "schematic", "placement", "routing", "writer",
		"erc", "drc", "round_trip", "replay",
	}
}

func newHeldOutCapabilityCaseResult(entry heldOutCapabilityEvaluationCase, stages []string) heldOutCapabilityCaseResult {
	result := heldOutCapabilityCaseResult{
		ID: entry.ID, Domain: entry.Domain, Family: entry.Family, Role: entry.Role,
		SafetyCritical: entry.SafetyCritical, Status: "blocked", EvidenceHashes: map[string]string{},
	}
	for _, stage := range stages {
		result.Stages = append(result.Stages, heldOutCapabilityStageResult{Stage: stage, Status: "not_reached"})
	}
	return result
}

func passHeldOutCapabilityStage(result *heldOutCapabilityCaseResult, stage string) {
	for index := range result.Stages {
		if result.Stages[index].Stage == stage {
			result.Stages[index].Status = "pass"
			return
		}
	}
}

func blockHeldOutCapabilityCase(result heldOutCapabilityCaseResult, stage string, issue reports.Issue, capability string) heldOutCapabilityCaseResult {
	if filepath.IsAbs(issue.Path) {
		issue.Path = "workflow." + stage
	}
	for index := range result.Stages {
		if result.Stages[index].Stage == stage {
			result.Stages[index] = heldOutCapabilityStageResult{
				Stage: stage, Status: "blocked", Code: issue.Code, Path: issue.Path, Message: issue.Message,
			}
			break
		}
	}
	result.Status = "blocked"
	result.BlockingStage = stage
	result.BlockingCode = issue.Code
	result.BlockingPath = issue.Path
	result.BlockingMessage = issue.Message
	result.BlockingCapability = capability
	if capability != "" && len(result.CapabilityGaps) == 0 {
		result.CapabilityGaps = []string{capability}
	}
	return result
}

func heldOutCapabilityFullAcceptance(acceptance architecturesearch.Acceptance) bool {
	return acceptance.RequireERC && acceptance.RequireStrictDRC &&
		acceptance.RequireCompleteRouting && acceptance.RequireConnectivity &&
		acceptance.RequireWriterCorrectness && acceptance.RequireRoundTripZeroDiff &&
		acceptance.RequireDeterministicReplay && acceptance.RequireContractComposition &&
		acceptance.RequireGlobalReasoning && acceptance.RequireCoverageAccounting &&
		acceptance.RequireAlternatives && acceptance.RequireFailClosed &&
		acceptance.RequireSimulation && acceptance.RequireAllCorners &&
		acceptance.RequireModelProvenance && acceptance.RequireClosedLoopEvidence
}

func heldOutCapabilitySearchGaps(search architecturesearch.SearchResult) []architecturesearch.CapabilityCoverageRecord {
	if search.Coverage == nil {
		return nil
	}
	var gaps []architecturesearch.CapabilityCoverageRecord
	for _, record := range search.Coverage.Records {
		if record.Status != architecturesearch.CoverageSelected {
			gaps = append(gaps, record)
		}
	}
	slices.SortStableFunc(gaps, func(left, right architecturesearch.CapabilityCoverageRecord) int {
		if order := strings.Compare(left.Capability, right.Capability); order != 0 {
			return order
		}
		return strings.Compare(left.Path, right.Path)
	})
	gaps = slices.CompactFunc(gaps, func(left, right architecturesearch.CapabilityCoverageRecord) bool {
		return left.Capability == right.Capability
	})
	return gaps
}

func firstHeldOutBlockingIssue(issues []reports.Issue, fallback string) reports.Issue {
	for _, issue := range issues {
		if issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked {
			return issue
		}
	}
	if len(issues) != 0 {
		return issues[0]
	}
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: fallback}
}

func heldOutCapabilityClosedLoopStage(issue reports.Issue) string {
	text := strings.ToLower(string(issue.Stage) + " " + issue.Path + " " + issue.Message)
	switch {
	case strings.Contains(text, "catalog"), strings.Contains(text, "component"),
		strings.Contains(text, "symbol"), strings.Contains(text, "footprint"),
		strings.Contains(text, "pin"), strings.Contains(text, "pad"):
		return "component_evidence"
	case strings.Contains(text, "simulation"), strings.Contains(text, "model"),
		strings.Contains(text, "analysis"), strings.Contains(text, "assertion"),
		strings.Contains(text, "closed_loop"):
		return "simulation"
	default:
		return "lowering"
	}
}

func heldOutCapabilityCreateOptions(output string, index libraryresolver.LibraryIndex, cli string) designworkflow.CreateOptions {
	return designworkflow.CreateOptions{
		OutputDir: output, Overwrite: true, LibraryIndex: &index,
		Validation: designworkflow.ValidationOptions{
			StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true,
			ArtifactDir: filepath.Join(output, ".kicadai", "validation"),
		},
		KiCadChecks: designworkflow.KiCadCheckOptions{
			KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true,
			KeepArtifacts: true, ArtifactDir: filepath.Join(output, ".kicadai", "checks"),
		},
		Writer: writercorrectness.Options{
			RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true,
			ArtifactDir:  filepath.Join(output, ".kicadai", "roundtrip"),
			LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true,
		},
	}
}

func heldOutCapabilityWorkflowFailure(result designworkflow.WorkflowResult) ([]string, string, reports.Issue, bool) {
	ordered := []struct {
		name           designworkflow.StageName
		stage          string
		completesStage string
	}{
		{designworkflow.StageSchematic, "schematic", ""},
		{designworkflow.StageSchematicElectrical, "schematic", ""},
		{designworkflow.StagePCBRealization, "schematic", "schematic"},
		{designworkflow.StagePlacement, "placement", "placement"},
		{designworkflow.StageRouting, "routing", "routing"},
		{designworkflow.StageProjectWrite, "writer", ""},
		{designworkflow.StageWriterCorrect, "writer", "writer"},
		{designworkflow.StageValidation, "routing", ""},
		{designworkflow.StageKiCadChecks, "erc", ""},
	}
	var passed []string
	for _, expected := range ordered {
		stage := openSetWorkflowStage(result, expected.name)
		if stage == nil {
			return passed, expected.stage, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
				Stage: string(expected.name), Path: "workflow.stages", Message: "required workflow stage is missing",
			}, true
		}
		if stage.Status == designworkflow.StageStatusOK {
			if expected.completesStage != "" {
				passed = append(passed, expected.completesStage)
			}
			continue
		}
		issue := firstHeldOutBlockingIssue(stage.Issues, "required workflow stage did not pass cleanly")
		mapped := expected.stage
		if expected.name == designworkflow.StageKiCadChecks {
			text := strings.ToLower(string(issue.Stage) + " " + issue.Path + " " + issue.Message)
			if strings.Contains(text, "drc") {
				mapped = "drc"
				passed = append(passed, "erc")
			}
		}
		return passed, mapped, issue, true
	}
	return passed, "", reports.Issue{}, false
}

func heldOutCapabilityCompareProjects(first, second, name string) error {
	for _, suffix := range []string{".kicad_pro", ".kicad_sch", ".kicad_pcb"} {
		firstBytes, err := os.ReadFile(filepath.Join(first, name+suffix))
		if err != nil {
			return err
		}
		secondBytes, err := os.ReadFile(filepath.Join(second, name+suffix))
		if err != nil {
			return err
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			return fmt.Errorf("%s differs byte-for-byte across deterministic replay", suffix)
		}
	}
	return nil
}

func heldOutCapabilityGeneratedHash(root, name string) (string, error) {
	hasher := sha256.New()
	for _, suffix := range []string{".kicad_pro", ".kicad_sch", ".kicad_pcb"} {
		data, err := os.ReadFile(filepath.Join(root, name+suffix))
		if err != nil {
			return "", err
		}
		_, _ = hasher.Write([]byte(suffix))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func aggregateHeldOutCapabilityResults(cases []heldOutCapabilityCaseResult, stages []string) heldOutCapabilityAggregate {
	aggregate := heldOutCapabilityAggregate{
		Cases: len(cases), StageReach: map[string]int{}, ByStage: map[string]int{},
		ByCode: map[string]int{}, ByCapability: map[string]int{},
	}
	type clusterAccumulator struct {
		cluster heldOutCapabilityCluster
		domains map[string]bool
	}
	clusters := map[string]*clusterAccumulator{}
	for _, result := range cases {
		if result.Status == "pass" {
			aggregate.Passed++
		} else {
			aggregate.Blocked++
			aggregate.ByStage[result.BlockingStage]++
			aggregate.ByCode[string(result.BlockingCode)]++
			gaps := slices.Clone(result.CapabilityGaps)
			if len(gaps) == 0 && result.BlockingCapability != "" {
				gaps = []string{result.BlockingCapability}
			}
			slices.Sort(gaps)
			gaps = slices.Compact(gaps)
			for _, capability := range gaps {
				aggregate.ByCapability[capability]++
				key := result.BlockingStage + ":" + capability + ":" + string(result.BlockingCode)
				current := clusters[key]
				if current == nil {
					current = &clusterAccumulator{
						cluster: heldOutCapabilityCluster{
							Key: key, Stage: result.BlockingStage, Capability: capability,
							Code: string(result.BlockingCode),
						},
						domains: map[string]bool{},
					}
					clusters[key] = current
				}
				current.cluster.CaseCount++
				current.cluster.Cases = append(current.cluster.Cases, result.ID)
				current.domains[result.Domain] = true
				if result.SafetyCritical {
					current.cluster.SafetyCriticalCount++
				}
			}
		}
		for _, stage := range result.Stages {
			if stage.Status == "pass" {
				aggregate.StageReach[stage.Stage]++
			}
		}
	}
	for _, current := range clusters {
		for domain := range current.domains {
			current.cluster.Domains = append(current.cluster.Domains, domain)
		}
		slices.Sort(current.cluster.Cases)
		slices.Sort(current.cluster.Domains)
		current.cluster.DomainCount = len(current.cluster.Domains)
		aggregate.Clusters = append(aggregate.Clusters, current.cluster)
	}
	slices.SortStableFunc(aggregate.Clusters, func(left, right heldOutCapabilityCluster) int {
		if left.CaseCount != right.CaseCount {
			return right.CaseCount - left.CaseCount
		}
		if left.DomainCount != right.DomainCount {
			return right.DomainCount - left.DomainCount
		}
		if left.SafetyCriticalCount != right.SafetyCriticalCount {
			return right.SafetyCriticalCount - left.SafetyCriticalCount
		}
		if order := strings.Compare(left.Capability, right.Capability); order != 0 {
			return order
		}
		return strings.Compare(left.Key, right.Key)
	})
	for index := range aggregate.Clusters {
		aggregate.Clusters[index].Rank = index + 1
	}
	for _, stage := range stages {
		if _, ok := aggregate.StageReach[stage]; !ok {
			aggregate.StageReach[stage] = 0
		}
	}
	return aggregate
}

func heldOutCapabilityToolVersion(t *testing.T, cli string) string {
	t.Helper()
	output, err := exec.Command(cli, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("read KiCad CLI version: %v: %s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func heldOutCapabilityJSONHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return heldOutCapabilityHash(encoded)
}

func heldOutCapabilityHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
