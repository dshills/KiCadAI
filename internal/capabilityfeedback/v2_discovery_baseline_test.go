package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityexpansion"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV2DiscoveryBaselineSchema = "kicadai.closed-loop-open-set-discovery-baseline.v2"
	closedLoopV2SelectionSchema         = "kicadai.closed-loop-open-set-selection.v2"
	closedLoopV2BaselineVersion         = 2
	closedLoopV2CorpusFreezeCommit      = "cea6040301230d16372aa1c390acb36903a0e711"
	closedLoopV2BaselineRoot            = "testdata/closed_loop_open_set_v2_baseline"
	closedLoopV2DiscoveryBaselineFrozen = false
)

type closedLoopV2DiscoveryBaselineReport struct {
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

type closedLoopV2Selection struct {
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

func TestClosedLoopV2DiscoveryBaselineArtifactsAreFrozen(t *testing.T) {
	if !closedLoopV2DiscoveryBaselineFrozen {
		t.Skip("V2 discovery baseline has not been recorded yet")
	}
	manifest := loadClosedLoopV2Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	cases := loadClosedLoopV2DiscoveryCases(t, manifest)
	discovery, err := Evaluate(RoleDiscovery, cases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}

	specRoot := closedLoopSpecDirectory(t)
	reportBytes := mustCorpusRead(t, filepath.Join(specRoot, "V2_DISCOVERY_BASELINE_REPORT.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V2_DISCOVERY_BASELINE_REPORT.sha256"), "V2_DISCOVERY_BASELINE_REPORT.json", reportBytes)
	var report closedLoopV2DiscoveryBaselineReport
	decodeCorpusStrict(t, reportBytes, &report)
	expected := buildClosedLoopV2DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	if !bytes.Equal(reportBytes, corpusJSON(t, expected)) {
		t.Fatal("V2 discovery baseline report does not reproduce from frozen evidence")
	}

	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "V2_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V2_SELECTION.sha256"), "V2_SELECTION.json", selectionBytes)
	var selection closedLoopV2Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	if want := buildClosedLoopV2Selection(t, expected); !bytes.Equal(selectionBytes, corpusJSON(t, want)) {
		t.Fatal("V2 rank-one selection does not reproduce from discovery-only evidence")
	}
}

func TestUpdateClosedLoopV2DiscoveryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_V2_DISCOVERY_BASELINE") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V2_DISCOVERY_BASELINE=1 to record the untouched V2 discovery baseline")
	}
	manifest := loadClosedLoopV2Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV2DiscoveryBaseline(t, manifest, inventory, environment)
	discovery, err := Evaluate(RoleDiscovery, cases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Clusters) == 0 || discovery.Clusters[0].Rank != 1 {
		t.Fatal("V2 discovery baseline did not produce an actionable rank-one cluster")
	}
	report := buildClosedLoopV2DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	specRoot := closedLoopSpecDirectory(t)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V2_DISCOVERY_BASELINE_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V2_SELECTION.json"), buildClosedLoopV2Selection(t, report))
}

func loadClosedLoopV2Manifest(t *testing.T) closedLoopV2Manifest {
	t.Helper()
	var manifest closedLoopV2Manifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json")), &manifest)
	return manifest
}

func runClosedLoopV2DiscoveryBaseline(
	t *testing.T,
	manifest closedLoopV2Manifest,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) []CaseEvidence {
	t.Helper()
	results := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleDiscovery {
			continue
		}
		t.Logf("V2 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil {
			t.Fatalf("%s encode synthesis replay: first=%v second=%v", entry.ID, firstErr, secondErr)
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs: first_sha256=%s second_sha256=%s", entry.ID, corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			promotion = &current
		}
		evidence, err := Observe(CaseMeta{ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observe: %v", entry.ID, err)
		}
		writeClosedLoopV2DiscoveryCaseEvidence(t, evidence)
		t.Logf("V2 discovery baseline %s outcome=%s stop=%s gaps=%d", entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps))
		results = append(results, evidence)
	}
	return results
}

func loadClosedLoopV2DiscoveryCases(t *testing.T, manifest closedLoopV2Manifest) []CaseEvidence {
	t.Helper()
	result := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleDiscovery {
			continue
		}
		path := filepath.Join(closedLoopV2BaselineRoot, "discovery", entry.ID+".json")
		current, err := DecodeCaseEvidence(bytes.NewReader(mustCorpusRead(t, path)))
		if err != nil {
			t.Fatalf("%s: %v", entry.ID, err)
		}
		if current.Case.ID != entry.ID || current.Case.Role != entry.Role || current.Case.Domain != entry.Domain || current.Case.SafetyImpact != entry.SafetyImpact {
			t.Fatalf("%s V2 discovery metadata does not match manifest", entry.ID)
		}
		result = append(result, current)
	}
	return result
}

func writeClosedLoopV2DiscoveryCaseEvidence(t *testing.T, current CaseEvidence) {
	t.Helper()
	root := filepath.Join(closedLoopV2BaselineRoot, "discovery")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, current.Case.ID+".json"), corpusJSON(t, current), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildClosedLoopV2DiscoveryBaselineReport(
	t *testing.T,
	manifestHash string,
	manifest closedLoopV2Manifest,
	discovery AggregateReport,
	plan capabilityexpansion.ExpansionPlan,
) closedLoopV2DiscoveryBaselineReport {
	t.Helper()
	report := closedLoopV2DiscoveryBaselineReport{
		Schema: closedLoopV2DiscoveryBaselineSchema, Version: closedLoopV2BaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopV2CorpusFreezeCommit,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, Environment: manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), Discovery: discovery, ExpansionPlan: plan,
	}
	hash, err := hashClosedLoopV2DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	return report
}

func buildClosedLoopV2Selection(t *testing.T, report closedLoopV2DiscoveryBaselineReport) closedLoopV2Selection {
	t.Helper()
	if len(report.Discovery.Clusters) == 0 || report.Discovery.Clusters[0].Rank != 1 {
		t.Fatal("V2 discovery baseline lacks rank one")
	}
	selection := closedLoopV2Selection{
		Schema: closedLoopV2SelectionSchema, Version: closedLoopV2BaselineVersion,
		CorpusManifestHash: report.CorpusManifestHash, FreezeCommit: report.FreezeCommit,
		EvaluatorPolicy: report.EvaluatorPolicy, ImpactRegistryHash: report.ImpactRegistryHash,
		SynthesisPolicyHash: report.SynthesisPolicyHash, DiscoveryBaselineHash: report.Hash,
		Cluster: report.Discovery.Clusters[0], ExpansionPlanHash: report.ExpansionPlan.Hash,
	}
	hash, err := hashClosedLoopV2Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.Hash = hash
	return selection
}

func hashClosedLoopV2DiscoveryBaseline(report closedLoopV2DiscoveryBaselineReport) (string, error) {
	report.Hash = ""
	return digest(report)
}

func hashClosedLoopV2Selection(selection closedLoopV2Selection) (string, error) {
	selection.Hash = ""
	return digest(selection)
}

func TestClosedLoopV2DiscoveryBaselineHashRejectsMutation(t *testing.T) {
	report := closedLoopV2DiscoveryBaselineReport{Schema: closedLoopV2DiscoveryBaselineSchema, Version: closedLoopV2BaselineVersion, FreezeCommit: closedLoopV2CorpusFreezeCommit}
	hash, err := hashClosedLoopV2DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	report.FreezeCommit = closedLoopV2StartCommit
	mutated, err := hashClosedLoopV2DiscoveryBaseline(report)
	if err != nil {
		t.Fatal(err)
	}
	if mutated == hash {
		t.Fatal("V2 discovery baseline mutation did not change its content hash")
	}
}

func TestClosedLoopV2DiscoveryCountsDoNotIncludeHeldOut(t *testing.T) {
	counts := closedLoopOutcomeCounts([]CaseEvidence{{Case: CaseMeta{Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog}, Outcome: OutcomePass}})
	for _, count := range counts {
		if count.Role == RoleHeldOut && (count.Pass != 0 || count.Unsupported != 0 || count.Unsafe != 0 || count.Exhausted != 0) {
			t.Fatalf("held-out outcome leaked into discovery counts: %#v", count)
		}
	}
}
