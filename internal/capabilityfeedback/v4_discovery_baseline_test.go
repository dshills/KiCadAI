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
	closedLoopV4DiscoveryBaselineSchema = "kicadai.closed-loop-open-set-discovery-baseline.v4"
	closedLoopV4SelectionSchema         = "kicadai.closed-loop-open-set-selection.v4"
	closedLoopV4BaselineVersion         = 4
	closedLoopV4CorpusFreezeCommit      = "35675e05bff9599be4492edfbe38bccdfa7a594f"
	closedLoopV4BaselineRoot            = "testdata/closed_loop_open_set_v4_baseline"
	closedLoopV4BaselineUpdateEnv       = "UPDATE_CLOSED_LOOP_V4_DISCOVERY_BASELINE"
)

type closedLoopV4DiscoveryBaselineReport struct {
	Schema                  string                            `json:"schema"`
	Version                 int                               `json:"version"`
	CorpusManifestHash      string                            `json:"corpus_manifest_hash"`
	FreezeCommit            string                            `json:"freeze_commit"`
	EvaluatorPolicy         string                            `json:"evaluator_policy"`
	ImpactRegistryHash      string                            `json:"impact_registry_hash"`
	SynthesisPolicyHash     string                            `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash string                            `json:"gap_transition_policy_sha256"`
	Environment             closedLoopEnvironment             `json:"environment"`
	OutcomeCounts           []closedLoopOutcomeCount          `json:"outcome_counts"`
	Discovery               AggregateReport                   `json:"discovery"`
	ExpansionPlan           capabilityexpansion.ExpansionPlan `json:"expansion_plan"`
	Hash                    string                            `json:"hash"`
}

type closedLoopV4Selection struct {
	Schema                  string  `json:"schema"`
	Version                 int     `json:"version"`
	CorpusManifestHash      string  `json:"corpus_manifest_hash"`
	FreezeCommit            string  `json:"freeze_commit"`
	EvaluatorPolicy         string  `json:"evaluator_policy"`
	ImpactRegistryHash      string  `json:"impact_registry_hash"`
	SynthesisPolicyHash     string  `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash string  `json:"gap_transition_policy_sha256"`
	DiscoveryBaselineHash   string  `json:"discovery_baseline_hash"`
	Cluster                 Cluster `json:"cluster"`
	ExpansionPlanHash       string  `json:"expansion_plan_hash"`
	Hash                    string  `json:"hash"`
}

func TestClosedLoopV4DiscoveryBaselineArtifactsAreFrozen(t *testing.T) {
	specRoot := closedLoopSpecDirectory(t)
	reportPath := filepath.Join(specRoot, "V4_DISCOVERY_BASELINE_REPORT.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("V4 frozen discovery baseline is unavailable: %v", err)
	}
	manifest := loadClosedLoopV4Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	registry, _ := closedLoopV4Policies(t)
	cases := loadClosedLoopV4DiscoveryCases(t, manifest)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}

	reportBytes := mustCorpusRead(t, reportPath)
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_DISCOVERY_BASELINE_REPORT.sha256"), "V4_DISCOVERY_BASELINE_REPORT.json", reportBytes)
	var report closedLoopV4DiscoveryBaselineReport
	decodeCorpusStrict(t, reportBytes, &report)
	expected := buildClosedLoopV4DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	if !bytes.Equal(reportBytes, corpusJSON(t, expected)) {
		t.Fatal("V4 discovery baseline report does not reproduce from frozen evidence")
	}

	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_SELECTION.sha256"), "V4_SELECTION.json", selectionBytes)
	var selection closedLoopV4Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	if want := buildClosedLoopV4Selection(t, expected); !bytes.Equal(selectionBytes, corpusJSON(t, want)) {
		t.Fatal("V4 rank-one selection does not reproduce from discovery-only evidence")
	}
}

func TestUpdateClosedLoopV4DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV4BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V4_DISCOVERY_BASELINE=1 to record the untouched V4 discovery baseline")
	}
	specRoot := closedLoopSpecDirectory(t)
	for _, path := range []string{
		closedLoopV4BaselineRoot,
		filepath.Join(specRoot, "V4_DISCOVERY_BASELINE_REPORT.json"),
		filepath.Join(specRoot, "V4_SELECTION.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V4 discovery baseline artifact already exists; refusing overwrite: %s", filepath.Base(path))
		}
	}
	manifest := loadClosedLoopV4Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	registry, policy := closedLoopV4Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV4DiscoveryBaseline(t, manifest, policy, inventory, environment)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Clusters) == 0 || discovery.Clusters[0].Rank != 1 {
		t.Fatal("V4 discovery baseline did not produce an actionable rank-one cluster")
	}
	report := buildClosedLoopV4DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	writeClosedLoopV4DiscoveryEvidence(t, cases)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V4_DISCOVERY_BASELINE_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V4_SELECTION.json"), buildClosedLoopV4Selection(t, report))
}

func loadClosedLoopV4Manifest(t *testing.T) closedLoopV4CorpusManifest {
	t.Helper()
	var manifest closedLoopV4CorpusManifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json")), &manifest)
	return manifest
}

func closedLoopV4Policies(t *testing.T) (capabilityevaluation.ImpactRegistry, ots.Policy) {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	var registry capabilityevaluation.ImpactRegistry
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(specRoot, "V4_IMPACT_REGISTRY.json")), &registry)
	_, _, registryHash, err := normalizeImpactRegistry(registry)
	if err != nil || registryHash != closedLoopV4ImpactRegistryHash {
		t.Fatalf("V4 impact registry hash = %q, want %q: %v", registryHash, closedLoopV4ImpactRegistryHash, err)
	}
	var policy ots.Policy
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(specRoot, "V4_SYNTHESIS_POLICY.json")), &policy)
	policyHash, err := ots.PolicyHash(policy)
	if err != nil || policyHash != closedLoopV4SynthesisPolicyHash {
		t.Fatalf("V4 synthesis policy hash = %q, want %q: %v", policyHash, closedLoopV4SynthesisPolicyHash, err)
	}
	return registry, policy
}

func runClosedLoopV4DiscoveryBaseline(
	t *testing.T,
	manifest closedLoopV4CorpusManifest,
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
		t.Logf("V4 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s violates the frozen requirement contract", entry.ID)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		// Canonical JSON is the normative normalized synthesis evidence later
		// hashed and frozen. Unexported runtime caches are intentionally outside
		// the replay contract and cannot influence an evidence decision.
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil {
			t.Fatalf("%s synthesis replay encoding failed", entry.ID)
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs: first_sha256=%s second_sha256=%s", entry.ID, corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		// Nil is the explicit non-pass promotion envelope. Observe rejects it for
		// a passed synthesis, while non-passes must not fabricate promotion data.
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
		t.Logf("V4 discovery baseline %s outcome=%s stop=%s gaps=%d", entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps))
		results = append(results, evidence)
	}
	if len(results) != closedLoopCorpusSize/2 {
		t.Fatalf("V4 discovery baseline case count = %d, want %d", len(results), closedLoopCorpusSize/2)
	}
	return results
}

func loadClosedLoopV4DiscoveryCases(t *testing.T, manifest closedLoopV4CorpusManifest) []CaseEvidence {
	t.Helper()
	result := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleDiscovery {
			continue
		}
		current, err := DecodeCaseEvidence(bytes.NewReader(mustCorpusRead(t, filepath.Join(closedLoopV4BaselineRoot, "discovery", entry.ID+".json"))))
		if err != nil {
			t.Fatalf("%s V4 discovery evidence is invalid", entry.ID)
		}
		if current.PolicyVersion != RealizabilityPolicyVersion || current.Case.ID != entry.ID || current.Case.Role != entry.Role ||
			current.Case.Domain != capabilityevaluation.Domain(entry.Domain) || current.Case.SafetyImpact != capabilityevaluation.SafetyImpact(entry.SafetyImpact) {
			t.Fatalf("%s V4 discovery evidence metadata does not match the manifest", entry.ID)
		}
		result = append(result, current)
	}
	return result
}

func writeClosedLoopV4DiscoveryEvidence(t *testing.T, cases []CaseEvidence) {
	t.Helper()
	temporaryRoot := closedLoopV4BaselineRoot + ".tmp"
	if _, err := os.Stat(temporaryRoot); !os.IsNotExist(err) {
		t.Fatal("V4 discovery baseline staging root already exists")
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
	if err := os.Rename(temporaryRoot, closedLoopV4BaselineRoot); err != nil {
		t.Fatal("atomically freeze V4 discovery evidence")
	}
}

func buildClosedLoopV4DiscoveryBaselineReport(
	t *testing.T,
	manifestHash string,
	manifest closedLoopV4CorpusManifest,
	discovery AggregateReport,
	plan capabilityexpansion.ExpansionPlan,
) closedLoopV4DiscoveryBaselineReport {
	t.Helper()
	report := closedLoopV4DiscoveryBaselineReport{
		Schema: closedLoopV4DiscoveryBaselineSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopV4CorpusFreezeCommit,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, GapTransitionPolicyHash: manifest.GapTransitionPolicyHash,
		Environment: manifest.Environment, OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases),
		Discovery: discovery, ExpansionPlan: plan,
	}
	hash, err := hashClosedLoopV4DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	return report
}

func buildClosedLoopV4Selection(t *testing.T, report closedLoopV4DiscoveryBaselineReport) closedLoopV4Selection {
	t.Helper()
	if len(report.Discovery.Clusters) == 0 || report.Discovery.Clusters[0].Rank != 1 {
		t.Fatal("V4 discovery baseline lacks rank one")
	}
	selection := closedLoopV4Selection{
		Schema: closedLoopV4SelectionSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: report.CorpusManifestHash, FreezeCommit: report.FreezeCommit,
		EvaluatorPolicy: report.EvaluatorPolicy, ImpactRegistryHash: report.ImpactRegistryHash,
		SynthesisPolicyHash: report.SynthesisPolicyHash, GapTransitionPolicyHash: report.GapTransitionPolicyHash,
		DiscoveryBaselineHash: report.Hash, Cluster: report.Discovery.Clusters[0], ExpansionPlanHash: report.ExpansionPlan.Hash,
	}
	hash, err := hashClosedLoopV4Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.Hash = hash
	return selection
}

func hashClosedLoopV4DiscoveryBaseline(report closedLoopV4DiscoveryBaselineReport) (string, error) {
	report.Hash = ""
	return digest(report)
}

func hashClosedLoopV4Selection(selection closedLoopV4Selection) (string, error) {
	selection.Hash = ""
	return digest(selection)
}

func TestClosedLoopV4DiscoveryBaselineHashRejectsMutation(t *testing.T) {
	report := closedLoopV4DiscoveryBaselineReport{
		Schema: closedLoopV4DiscoveryBaselineSchema, Version: closedLoopV4BaselineVersion,
		FreezeCommit: closedLoopV4CorpusFreezeCommit,
	}
	hash, err := hashClosedLoopV4DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.FreezeCommit = closedLoopV4StartCommit
	mutated, err := hashClosedLoopV4DiscoveryBaseline(report)
	if err != nil || mutated == hash {
		t.Fatal("V4 discovery baseline mutation did not change its content hash")
	}
}

func TestClosedLoopV4DiscoveryCountsDoNotIncludeHeldOut(t *testing.T) {
	counts := closedLoopOutcomeCounts([]CaseEvidence{{
		Case: CaseMeta{Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog}, Outcome: OutcomePass,
	}})
	seenHeldOut := false
	for _, count := range counts {
		if count.Role != RoleHeldOut {
			continue
		}
		seenHeldOut = true
		if count.Pass != 0 || count.Unsupported != 0 || count.Unsafe != 0 || count.Exhausted != 0 {
			t.Fatal("held-out outcome leaked into V4 discovery counts")
		}
	}
	if !seenHeldOut {
		t.Fatal("V4 discovery counts omit the zero-valued held-out partition")
	}
}

func TestClosedLoopV4SelectionHashRejectsMutation(t *testing.T) {
	selection := closedLoopV4Selection{
		Schema: closedLoopV4SelectionSchema, Version: closedLoopV4BaselineVersion,
		DiscoveryBaselineHash: fmt.Sprintf("%064d", 1),
	}
	hash, err := hashClosedLoopV4Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.DiscoveryBaselineHash = fmt.Sprintf("%064d", 2)
	mutated, err := hashClosedLoopV4Selection(selection)
	if err != nil || mutated == hash {
		t.Fatal("V4 selection mutation did not change its content hash")
	}
}
