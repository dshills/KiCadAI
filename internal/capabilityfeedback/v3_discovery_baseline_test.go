package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityexpansion"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV3DiscoveryBaselineSchema = "kicadai.closed-loop-open-set-discovery-baseline.v3"
	closedLoopV3SelectionSchema         = "kicadai.closed-loop-open-set-selection.v3"
	closedLoopV3BaselineVersion         = 3
	closedLoopV3CorpusFreezeCommit      = "b222db5aa36c00e0f3bf60a5d1768d02062d2fd7"
	closedLoopV3BaselineRoot            = "testdata/closed_loop_open_set_v3_baseline"
	closedLoopV3BaselineUpdateEnv       = "UPDATE_CLOSED_LOOP_V3_DISCOVERY_BASELINE"
)

type closedLoopV3DiscoveryBaselineReport struct {
	Schema              string                            `json:"schema"`
	Version             int                               `json:"version"`
	CorpusManifestHash  string                            `json:"corpus_manifest_hash"`
	FreezeCommit        string                            `json:"freeze_commit"`
	EvaluatorPolicy     string                            `json:"evaluator_policy"`
	ImpactRegistryHash  string                            `json:"impact_registry_hash"`
	SynthesisPolicyHash string                            `json:"synthesis_policy_hash"`
	Environment         closedLoopEnvironment             `json:"environment"`
	OutcomeCounts       []closedLoopOutcomeCount          `json:"outcome_counts"`
	Discovery           AggregateReport                   `json:"discovery"`
	ExpansionPlan       capabilityexpansion.ExpansionPlan `json:"expansion_plan"`
	Hash                string                            `json:"hash"`
}

type closedLoopV3Selection struct {
	Schema                string  `json:"schema"`
	Version               int     `json:"version"`
	CorpusManifestHash    string  `json:"corpus_manifest_hash"`
	FreezeCommit          string  `json:"freeze_commit"`
	EvaluatorPolicy       string  `json:"evaluator_policy"`
	ImpactRegistryHash    string  `json:"impact_registry_hash"`
	SynthesisPolicyHash   string  `json:"synthesis_policy_hash"`
	DiscoveryBaselineHash string  `json:"discovery_baseline_hash"`
	Cluster               Cluster `json:"cluster"`
	ExpansionPlanHash     string  `json:"expansion_plan_hash"`
	Hash                  string  `json:"hash"`
}

func TestClosedLoopV3DiscoveryBaselineArtifactsAreFrozen(t *testing.T) {
	manifest := loadClosedLoopV3Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	registry, _ := closedLoopV3Policies(t)
	cases := loadClosedLoopV3DiscoveryCases(t, manifest)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}

	specRoot := closedLoopSpecDirectory(t)
	reportBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_DISCOVERY_BASELINE_REPORT.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_DISCOVERY_BASELINE_REPORT.sha256"), "V3_DISCOVERY_BASELINE_REPORT.json", reportBytes)
	var report closedLoopV3DiscoveryBaselineReport
	decodeCorpusStrict(t, reportBytes, &report)
	expected := buildClosedLoopV3DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	if !bytes.Equal(reportBytes, corpusJSON(t, expected)) {
		t.Fatal("V3 discovery baseline report does not reproduce from frozen evidence")
	}

	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_SELECTION.sha256"), "V3_SELECTION.json", selectionBytes)
	var selection closedLoopV3Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	if want := buildClosedLoopV3Selection(t, expected); !bytes.Equal(selectionBytes, corpusJSON(t, want)) {
		t.Fatal("V3 rank-one selection does not reproduce from discovery-only evidence")
	}
}

func TestUpdateClosedLoopV3DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV3BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V3_DISCOVERY_BASELINE=1 to record the untouched V3 discovery baseline")
	}
	specRoot := closedLoopSpecDirectory(t)
	for _, path := range []string{
		closedLoopV3BaselineRoot,
		filepath.Join(specRoot, "V3_DISCOVERY_BASELINE_REPORT.json"),
		filepath.Join(specRoot, "V3_SELECTION.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V3 discovery baseline artifact already exists; refusing overwrite: %s", filepath.Base(path))
		}
	}
	manifest := loadClosedLoopV3Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	registry, policy := closedLoopV3Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV3DiscoveryBaseline(t, manifest, policy, inventory, environment)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Clusters) == 0 || discovery.Clusters[0].Rank != 1 {
		t.Fatal("V3 discovery baseline did not produce an actionable rank-one cluster")
	}
	report := buildClosedLoopV3DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	writeClosedLoopV3DiscoveryEvidence(t, cases)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_DISCOVERY_BASELINE_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_SELECTION.json"), buildClosedLoopV3Selection(t, report))
}

func loadClosedLoopV3Manifest(t *testing.T) closedLoopV3CorpusManifest {
	t.Helper()
	var manifest closedLoopV3CorpusManifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json")), &manifest)
	return manifest
}

func closedLoopV3Policies(t *testing.T) (capabilityevaluation.ImpactRegistry, ots.Policy) {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	var registry capabilityevaluation.ImpactRegistry
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(specRoot, "V3_IMPACT_REGISTRY.json")), &registry)
	_, _, registryHash, err := normalizeImpactRegistry(registry)
	if err != nil || registryHash != closedLoopV3ImpactRegistryHash {
		t.Fatalf("V3 impact registry hash is invalid")
	}
	var policy ots.Policy
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(specRoot, "V3_SYNTHESIS_POLICY.json")), &policy)
	policyHash, err := ots.PolicyHash(policy)
	if err != nil || policyHash != closedLoopV3SynthesisPolicyHash {
		t.Fatalf("V3 synthesis policy hash is invalid")
	}
	return registry, policy
}

func runClosedLoopV3DiscoveryBaseline(
	t *testing.T,
	manifest closedLoopV3CorpusManifest,
	policy ots.Policy,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) []CaseEvidence {
	t.Helper()
	results := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleDiscovery {
			continue
		}
		t.Logf("V3 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s violates the frozen requirement contract", entry.ID)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil {
			t.Fatalf("%s synthesis replay encoding failed", entry.ID)
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs: first_sha256=%s second_sha256=%s", entry.ID, corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatalf("%s did not pass two clean-root installed-KiCad promotions", entry.ID)
			}
			promotion = &current
		}
		evidence, err := ObserveRealizabilityAware(CaseMeta{
			ID: entry.ID, Role: entry.Role,
			Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact),
		}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s policy-v2 observation failed: %v", entry.ID, err)
		}
		t.Logf("V3 discovery baseline %s outcome=%s stop=%s gaps=%d", entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps))
		results = append(results, evidence)
	}
	if len(results) != closedLoopCorpusSize/2 {
		t.Fatalf("V3 discovery baseline case count = %d, want %d", len(results), closedLoopCorpusSize/2)
	}
	return results
}

func loadClosedLoopV3DiscoveryCases(t *testing.T, manifest closedLoopV3CorpusManifest) []CaseEvidence {
	t.Helper()
	result := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleDiscovery {
			continue
		}
		current, err := DecodeCaseEvidence(bytes.NewReader(mustCorpusRead(t, filepath.Join(closedLoopV3BaselineRoot, "discovery", entry.ID+".json"))))
		if err != nil {
			t.Fatalf("%s V3 discovery evidence is invalid", entry.ID)
		}
		if current.PolicyVersion != RealizabilityPolicyVersion || current.Case.ID != entry.ID || current.Case.Role != entry.Role ||
			current.Case.Domain != capabilityevaluation.Domain(entry.Domain) || current.Case.SafetyImpact != capabilityevaluation.SafetyImpact(entry.SafetyImpact) {
			t.Fatalf("%s V3 discovery evidence metadata does not match the manifest", entry.ID)
		}
		result = append(result, current)
	}
	return result
}

func writeClosedLoopV3DiscoveryEvidence(t *testing.T, cases []CaseEvidence) {
	t.Helper()
	temporaryRoot := closedLoopV3BaselineRoot + ".tmp"
	if _, err := os.Stat(temporaryRoot); !os.IsNotExist(err) {
		t.Fatal("V3 discovery baseline staging root already exists")
	}
	root := filepath.Join(temporaryRoot, "discovery")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporaryRoot) })
	for _, current := range cases {
		if err := os.WriteFile(filepath.Join(root, current.Case.ID+".json"), corpusJSON(t, current), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(temporaryRoot, closedLoopV3BaselineRoot); err != nil {
		t.Fatal("atomically freeze V3 discovery evidence")
	}
}

func buildClosedLoopV3DiscoveryBaselineReport(
	t *testing.T,
	manifestHash string,
	manifest closedLoopV3CorpusManifest,
	discovery AggregateReport,
	plan capabilityexpansion.ExpansionPlan,
) closedLoopV3DiscoveryBaselineReport {
	t.Helper()
	report := closedLoopV3DiscoveryBaselineReport{
		Schema: closedLoopV3DiscoveryBaselineSchema, Version: closedLoopV3BaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopV3CorpusFreezeCommit,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, Environment: manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), Discovery: discovery, ExpansionPlan: plan,
	}
	hash, err := hashClosedLoopV3DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	return report
}

func buildClosedLoopV3Selection(t *testing.T, report closedLoopV3DiscoveryBaselineReport) closedLoopV3Selection {
	t.Helper()
	if len(report.Discovery.Clusters) == 0 || report.Discovery.Clusters[0].Rank != 1 {
		t.Fatal("V3 discovery baseline lacks rank one")
	}
	selection := closedLoopV3Selection{
		Schema: closedLoopV3SelectionSchema, Version: closedLoopV3BaselineVersion,
		CorpusManifestHash: report.CorpusManifestHash, FreezeCommit: report.FreezeCommit,
		EvaluatorPolicy: report.EvaluatorPolicy, ImpactRegistryHash: report.ImpactRegistryHash,
		SynthesisPolicyHash: report.SynthesisPolicyHash, DiscoveryBaselineHash: report.Hash,
		Cluster: report.Discovery.Clusters[0], ExpansionPlanHash: report.ExpansionPlan.Hash,
	}
	hash, err := hashClosedLoopV3Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.Hash = hash
	return selection
}

func hashClosedLoopV3DiscoveryBaseline(report closedLoopV3DiscoveryBaselineReport) (string, error) {
	report.Hash = ""
	return digest(report)
}

func hashClosedLoopV3Selection(selection closedLoopV3Selection) (string, error) {
	selection.Hash = ""
	return digest(selection)
}

func TestClosedLoopV3DiscoveryBaselineHashRejectsMutation(t *testing.T) {
	report := closedLoopV3DiscoveryBaselineReport{
		Schema: closedLoopV3DiscoveryBaselineSchema, Version: closedLoopV3BaselineVersion,
		FreezeCommit: closedLoopV3CorpusFreezeCommit,
	}
	hash, err := hashClosedLoopV3DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.FreezeCommit = closedLoopV3StartCommit
	mutated, err := hashClosedLoopV3DiscoveryBaseline(report)
	if err != nil || mutated == hash {
		t.Fatal("V3 discovery baseline mutation did not change its content hash")
	}
}

func TestClosedLoopV3DiscoveryCountsDoNotIncludeHeldOut(t *testing.T) {
	counts := closedLoopOutcomeCounts([]CaseEvidence{{
		Case: CaseMeta{Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog}, Outcome: OutcomePass,
	}})
	for _, count := range counts {
		if count.Role == RoleHeldOut && (count.Pass != 0 || count.Unsupported != 0 || count.Unsafe != 0 || count.Exhausted != 0) {
			t.Fatalf("held-out outcome leaked into V3 discovery counts")
		}
	}
}

func TestClosedLoopV3SelectionHashRejectsMutation(t *testing.T) {
	selection := closedLoopV3Selection{
		Schema: closedLoopV3SelectionSchema, Version: closedLoopV3BaselineVersion,
		DiscoveryBaselineHash: fmt.Sprintf("%064d", 1),
	}
	hash, err := hashClosedLoopV3Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.DiscoveryBaselineHash = fmt.Sprintf("%064d", 2)
	mutated, err := hashClosedLoopV3Selection(selection)
	if err != nil || mutated == hash {
		t.Fatal("V3 selection mutation did not change its content hash")
	}
}
