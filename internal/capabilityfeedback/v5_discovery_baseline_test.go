package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilitypackages"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV5BaselineSchema             = "kicadai.closed-loop-open-set-discovery-baseline.v5"
	closedLoopV5CaseArtifactSchema         = "kicadai.closed-loop-open-set-discovery-case.v5"
	closedLoopV5RankingSchema              = "kicadai.closed-loop-open-set-package-ranking.v5"
	closedLoopV5SelectionSchema            = "kicadai.closed-loop-open-set-selection.v5"
	closedLoopV5BaselineVersion            = 5
	closedLoopV5BaselineRoot               = "testdata/closed_loop_open_set_v5_baseline"
	closedLoopV5BaselineUpdateEnv          = "UPDATE_CLOSED_LOOP_V5_DISCOVERY_BASELINE"
	closedLoopV5CorpusFreezeCommit         = "82f0b7ce6b704fd3c7ca832f8ad0b194c0e38f8b"
	closedLoopV5InfrastructureCommit       = "3cf8e3c8df4ea6f2b4898dfd79871d2ca1590314"
	closedLoopV5SelectionPolicyHash        = "d5d50e3b865ff2629e67f52661c709a3aeeeb297889c6dd2ca67e32a748c57fb"
	closedLoopV5ImplementationManifestHash = "d06997de15c5afe71853058124f9b30a6afdd018fcf09d3a6da2e7df57d88b28"
	closedLoopV5ImpactRegistryFileHash     = "c0229f216b3024627992327ddaa90f44df7f3f1f97412d05b22161284d15afa0"
	closedLoopV5SynthesisPolicyFileHash    = "7e415c9a6b6d30142840c8bd56e598db70b1a2103bc663ccd73df762871cbb66"
	closedLoopV5GapPolicyFileHash          = "ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8"
)

type closedLoopV5CaseArtifact struct {
	Schema                 string                      `json:"schema"`
	Version                int                         `json:"version"`
	CaseID                 string                      `json:"case_id"`
	RequirementSHA256      string                      `json:"requirement_sha256"`
	NormalizedReplaySHA256 []string                    `json:"normalized_replay_sha256"`
	Synthesis              ots.SynthesisRun            `json:"synthesis"`
	Promotion              *closedLoopV5PromotionProof `json:"promotion,omitempty"`
	Observation            CaseEvidence                `json:"observation"`
	Hash                   string                      `json:"hash"`
}

type closedLoopV5PromotionProof struct {
	Schema          string                      `json:"schema"`
	Version         int                         `json:"version"`
	Status          ots.PhysicalPromotionStatus `json:"status"`
	ReplayIdentical bool                        `json:"replay_identical"`
	ProjectHash     string                      `json:"project_hash"`
	PromotionHash   string                      `json:"promotion_hash"`
	RunCount        int                         `json:"run_count"`
}

type closedLoopV5ArtifactRef struct {
	CaseID string `json:"case_id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopV5BaselineReport struct {
	Schema                       string                    `json:"schema"`
	Version                      int                       `json:"version"`
	CorpusManifestSHA256         string                    `json:"corpus_manifest_sha256"`
	CorpusFreezeCommit           string                    `json:"corpus_freeze_commit"`
	InfrastructureCommit         string                    `json:"infrastructure_commit"`
	SelectionFreezeParentCommit  string                    `json:"selection_freeze_parent_commit"`
	EvaluatorPolicy              string                    `json:"evaluator_policy"`
	ImpactRegistryFileSHA256     string                    `json:"impact_registry_file_sha256"`
	SynthesisPolicyFileSHA256    string                    `json:"synthesis_policy_file_sha256"`
	GapPolicyFileSHA256          string                    `json:"gap_policy_file_sha256"`
	SelectionPolicySHA256        string                    `json:"selection_policy_sha256"`
	ImplementationManifestSHA256 string                    `json:"implementation_manifest_sha256"`
	InventorySHA256              string                    `json:"inventory_sha256"`
	CatalogSHA256                string                    `json:"catalog_sha256"`
	ModelRegistrySHA256          string                    `json:"model_registry_sha256"`
	SynthesisPolicySHA256        string                    `json:"synthesis_policy_sha256"`
	OutcomeCounts                []closedLoopOutcomeCount  `json:"outcome_counts"`
	CaseArtifacts                []closedLoopV5ArtifactRef `json:"case_artifacts"`
	Discovery                    AggregateReport           `json:"discovery"`
	Hash                         string                    `json:"hash"`
}

type closedLoopV5Ranking struct {
	Schema                string                         `json:"schema"`
	Version               int                            `json:"version"`
	BaselineSHA256        string                         `json:"baseline_sha256"`
	SelectionPolicySHA256 string                         `json:"selection_policy_sha256"`
	Packages              []capabilitypackages.Candidate `json:"packages"`
	Hash                  string                         `json:"hash"`
}

type closedLoopV5Selection struct {
	Schema                      string                       `json:"schema"`
	Version                     int                          `json:"version"`
	StartingCommit              string                       `json:"starting_commit"`
	ContractFreezeCommit        string                       `json:"contract_freeze_commit"`
	CorpusFreezeCommit          string                       `json:"corpus_freeze_commit"`
	InfrastructureCommit        string                       `json:"infrastructure_commit"`
	SelectionFreezeParentCommit string                       `json:"selection_freeze_parent_commit"`
	CorpusManifestSHA256        string                       `json:"corpus_manifest_sha256"`
	BaselineSHA256              string                       `json:"baseline_sha256"`
	RankingSHA256               string                       `json:"ranking_sha256"`
	SelectionPolicySHA256       string                       `json:"selection_policy_sha256"`
	Selected                    capabilitypackages.Candidate `json:"selected"`
	GenericPlanSHA256           string                       `json:"generic_plan_sha256"`
	Hash                        string                       `json:"hash"`
}

func TestClosedLoopV5DiscoveryBaselineIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV5BaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V5 discovery baseline has not been frozen")
		}
		t.Fatal(err)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV5BaselineRoot, filepath.Join(closedLoopV5BaselineRoot, corpuspublication.ChecksumFile)); err != nil {
		t.Fatalf("verify V5 discovery baseline checksums: %v", err)
	}
	manifest := loadClosedLoopV5Manifest(t)
	registry, _ := closedLoopV4Policies(t)
	policy := loadClosedLoopV5SelectionPolicy(t)
	caseArtifacts := loadClosedLoopV5CaseArtifacts(t, manifest)
	cases := make([]CaseEvidence, len(caseArtifacts))
	refs := make([]closedLoopV5ArtifactRef, len(caseArtifacts))
	for index, artifact := range caseArtifacts {
		cases[index] = artifact.Observation
		path := filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json"))
		refs[index] = closedLoopV5ArtifactRef{CaseID: artifact.CaseID, Path: path, SHA256: corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, filepath.FromSlash(path))))}
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	report := buildClosedLoopV5BaselineReport(t, discovery, refs)
	assertClosedLoopV5ArtifactEqual(t, "report.json", report)
	observations, err := PackageObservations(discovery, registry)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := capabilitypackages.Build(observations, policy)
	if err != nil {
		t.Fatal(err)
	}
	ranking := buildClosedLoopV5Ranking(t, report.Hash, candidates)
	assertClosedLoopV5ArtifactEqual(t, "package_ranking.json", ranking)
	if len(candidates) == 0 {
		t.Fatal("V5 discovery baseline has no eligible capability package")
	}
	plan, err := capabilitypackages.BuildGenericPlan(candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	assertClosedLoopV5ArtifactEqual(t, "generic_plan.json", plan)
	selected, err := capabilitypackages.SelectRankOne(candidates, plan, policy)
	if err != nil {
		t.Fatal(err)
	}
	selection := buildClosedLoopV5Selection(t, report.Hash, ranking.Hash, selected, plan.Hash)
	assertClosedLoopV5ArtifactEqual(t, "selection.json", selection)
	if audit := mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, "BASELINE_AUDIT.md")); !bytes.Equal(audit, closedLoopV5BaselineAudit(report, ranking, selection)) {
		t.Fatal("V5 baseline audit does not reproduce from frozen aggregates")
	}
	assertClosedLoopV5BaselineFileSet(t, manifest)
}

func TestUpdateClosedLoopV5DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV5BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V5_DISCOVERY_BASELINE=1 to freeze the untouched V5 discovery baseline")
	}
	if _, err := os.Stat(closedLoopV5BaselineRoot); !os.IsNotExist(err) {
		t.Fatal("V5 discovery baseline already exists; refusing overwrite")
	}
	manifest := loadClosedLoopV5Manifest(t)
	registry, synthesisPolicy := closedLoopV4Policies(t)
	selectionPolicy := loadClosedLoopV5SelectionPolicy(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	caseArtifacts := runClosedLoopV5DiscoveryBaseline(t, manifest, synthesisPolicy, inventory, environment)
	cases := make([]CaseEvidence, len(caseArtifacts))
	for index := range caseArtifacts {
		cases[index] = caseArtifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := PackageObservations(discovery, registry)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := capabilitypackages.Build(observations, selectionPolicy)
	if err != nil || len(candidates) == 0 {
		t.Fatalf("build V5 capability packages: %v", err)
	}
	plan, err := capabilitypackages.BuildGenericPlan(candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	selected, err := capabilitypackages.SelectRankOne(candidates, plan, selectionPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicdir.Publish(closedLoopV5BaselineRoot, func(root string) error {
		refs, err := writeClosedLoopV5CaseArtifacts(root, caseArtifacts)
		if err != nil {
			return err
		}
		report := buildClosedLoopV5BaselineReport(t, discovery, refs)
		ranking := buildClosedLoopV5Ranking(t, report.Hash, candidates)
		selection := buildClosedLoopV5Selection(t, report.Hash, ranking.Hash, selected, plan.Hash)
		artifacts := map[string][]byte{
			"report.json":          corpusJSON(t, report),
			"package_ranking.json": corpusJSON(t, ranking),
			"generic_plan.json":    corpusJSON(t, plan),
			"selection.json":       corpusJSON(t, selection),
			"BASELINE_AUDIT.md":    closedLoopV5BaselineAudit(report, ranking, selection),
		}
		for path, data := range artifacts {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("V5 discovery baseline selected package scope=%s capability=%s cases=%d domains=%d members=%d", selected.Scope, selected.Capability, len(selected.Cases), len(selected.Domains), len(selected.Members))
}

func runClosedLoopV5DiscoveryBaseline(t *testing.T, manifest corpuspublication.Manifest, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment) []closedLoopV5CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV5CaseArtifact, 0, closedLoopV5RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		t.Logf("V5 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, filepath.FromSlash(entry.StablePath)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s violates the frozen requirement contract", entry.ID)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs: first_sha256=%s second_sha256=%s", entry.ID, corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		var promotionProof *closedLoopV5PromotionProof
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatalf("%s did not pass two clean-root installed-KiCad promotions", entry.ID)
			}
			promotion = &current
			promotionProof = &closedLoopV5PromotionProof{Schema: current.Schema, Version: current.Version, Status: current.Status, ReplayIdentical: current.ReplayIdentical, ProjectHash: current.ProjectHash, PromotionHash: current.Hash, RunCount: len(current.Runs)}
		}
		evidence, err := ObserveRealizabilityAware(CaseMeta{ID: entry.ID, Role: RoleDiscovery, Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact)}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observation failed: %v", entry.ID, err)
		}
		artifact := closedLoopV5CaseArtifact{Schema: closedLoopV5CaseArtifactSchema, Version: closedLoopV5BaselineVersion, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, NormalizedReplaySHA256: []string{corpusHash(firstBytes), corpusHash(secondBytes)}, Synthesis: first, Promotion: promotionProof, Observation: evidence}
		artifact.Hash, err = hashClosedLoopV5CaseArtifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, artifact)
		t.Logf("V5 discovery baseline %s outcome=%s stop=%s gaps=%d", entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps))
	}
	if len(artifacts) != closedLoopV5RoleSize {
		t.Fatalf("V5 discovery baseline case count = %d, want %d", len(artifacts), closedLoopV5RoleSize)
	}
	return artifacts
}

func loadClosedLoopV5Manifest(t *testing.T) corpuspublication.Manifest {
	t.Helper()
	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, corpuspublication.ManifestFile)), &manifest)
	return manifest
}

func loadClosedLoopV5SelectionPolicy(t *testing.T) capabilitypackages.SelectionPolicy {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V5_SELECTION_POLICY.json")
	data := mustCorpusRead(t, path)
	if corpusHash(data) != closedLoopV5SelectionPolicyHash {
		t.Fatal("V5 selection policy differs from the frozen commitment")
	}
	policy, err := capabilitypackages.DecodePolicy(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func loadClosedLoopV5CaseArtifacts(t *testing.T, manifest corpuspublication.Manifest) []closedLoopV5CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV5CaseArtifact, 0, closedLoopV5RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		var artifact closedLoopV5CaseArtifact
		decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, "discovery", entry.ID+".json")), &artifact)
		expected, err := hashClosedLoopV5CaseArtifact(artifact)
		synthesisBytes, marshalErr := json.Marshal(artifact.Synthesis)
		if err != nil || marshalErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV5CaseArtifactSchema || artifact.Version != closedLoopV5BaselineVersion || artifact.CaseID != entry.ID || artifact.RequirementSHA256 != entry.RequirementSHA256 || len(artifact.NormalizedReplaySHA256) != 2 || artifact.NormalizedReplaySHA256[0] != artifact.NormalizedReplaySHA256[1] || artifact.NormalizedReplaySHA256[0] != corpusHash(synthesisBytes) || artifact.Observation.SynthesisHash != artifact.Synthesis.Hash {
			t.Fatalf("V5 case artifact %s is invalid", entry.ID)
		}
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != ots.PhysicalPromotionPassed || !artifact.Promotion.ReplayIdentical || artifact.Promotion.RunCount != 2 || artifact.Promotion.PromotionHash != artifact.Observation.PromotionHash || artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				t.Fatalf("V5 passing case %s lacks complete promotion proof", entry.ID)
			}
		} else if artifact.Promotion != nil {
			t.Fatalf("V5 nonpassing case %s contains fabricated promotion proof", entry.ID)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func writeClosedLoopV5CaseArtifacts(root string, artifacts []closedLoopV5CaseArtifact) ([]closedLoopV5ArtifactRef, error) {
	discoveryRoot := filepath.Join(root, "discovery")
	if err := os.Mkdir(discoveryRoot, 0o755); err != nil {
		return nil, err
	}
	refs := make([]closedLoopV5ArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json"))
		data, err := json.MarshalIndent(artifact, "", "  ")
		if err != nil {
			return nil, err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0o644); err != nil {
			return nil, err
		}
		refs = append(refs, closedLoopV5ArtifactRef{CaseID: artifact.CaseID, Path: path, SHA256: corpusHash(data)})
	}
	return refs, nil
}

func buildClosedLoopV5BaselineReport(t *testing.T, discovery AggregateReport, refs []closedLoopV5ArtifactRef) closedLoopV5BaselineReport {
	t.Helper()
	inventoryHash, catalogHash, modelRegistryHash, synthesisPolicyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
	report := closedLoopV5BaselineReport{Schema: closedLoopV5BaselineSchema, Version: closedLoopV5BaselineVersion, CorpusManifestSHA256: closedLoopV5CorpusManifestHash, CorpusFreezeCommit: closedLoopV5CorpusFreezeCommit, InfrastructureCommit: closedLoopV5InfrastructureCommit, SelectionFreezeParentCommit: closedLoopV5InfrastructureCommit, EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistryFileSHA256: closedLoopV5ImpactRegistryFileHash, SynthesisPolicyFileSHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicyFileSHA256: closedLoopV5GapPolicyFileHash, SelectionPolicySHA256: closedLoopV5SelectionPolicyHash, ImplementationManifestSHA256: closedLoopV5ImplementationManifestHash, InventorySHA256: inventoryHash, CatalogSHA256: catalogHash, ModelRegistrySHA256: modelRegistryHash, SynthesisPolicySHA256: synthesisPolicyHash, OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs, Discovery: discovery}
	var err error
	report.Hash, err = hashClosedLoopV5BaselineReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func buildClosedLoopV5Ranking(t *testing.T, baselineHash string, candidates []capabilitypackages.Candidate) closedLoopV5Ranking {
	t.Helper()
	ranking := closedLoopV5Ranking{Schema: closedLoopV5RankingSchema, Version: closedLoopV5BaselineVersion, BaselineSHA256: baselineHash, SelectionPolicySHA256: closedLoopV5SelectionPolicyHash, Packages: candidates}
	var err error
	ranking.Hash, err = hashClosedLoopV5Ranking(ranking)
	if err != nil {
		t.Fatal(err)
	}
	return ranking
}

func buildClosedLoopV5Selection(t *testing.T, baselineHash, rankingHash string, selected capabilitypackages.Candidate, planHash string) closedLoopV5Selection {
	t.Helper()
	selection := closedLoopV5Selection{Schema: closedLoopV5SelectionSchema, Version: closedLoopV5BaselineVersion, StartingCommit: "d8e98b4dee3212823525c5955e8e025bd0039d03", ContractFreezeCommit: "a9249879d5e02575fe047925d613458ffec62030", CorpusFreezeCommit: closedLoopV5CorpusFreezeCommit, InfrastructureCommit: closedLoopV5InfrastructureCommit, SelectionFreezeParentCommit: closedLoopV5InfrastructureCommit, CorpusManifestSHA256: closedLoopV5CorpusManifestHash, BaselineSHA256: baselineHash, RankingSHA256: rankingHash, SelectionPolicySHA256: closedLoopV5SelectionPolicyHash, Selected: selected, GenericPlanSHA256: planHash}
	var err error
	selection.Hash, err = hashClosedLoopV5Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func hashClosedLoopV5CaseArtifact(artifact closedLoopV5CaseArtifact) (string, error) {
	artifact.Hash = ""
	return digest(artifact)
}
func hashClosedLoopV5BaselineReport(report closedLoopV5BaselineReport) (string, error) {
	report.Hash = ""
	return digest(report)
}
func hashClosedLoopV5Ranking(ranking closedLoopV5Ranking) (string, error) {
	ranking.Hash = ""
	return digest(ranking)
}
func hashClosedLoopV5Selection(selection closedLoopV5Selection) (string, error) {
	selection.Hash = ""
	return digest(selection)
}

func assertClosedLoopV5ArtifactEqual(t *testing.T, name string, expected any) {
	t.Helper()
	actual := mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, name))
	if !bytes.Equal(actual, corpusJSON(t, expected)) {
		t.Fatalf("V5 baseline artifact %s does not reproduce", name)
	}
}

func writeClosedLoopV5Checksums(root string) error {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	var lines strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		lines.WriteString(corpusHash(data))
		lines.WriteString("  ")
		lines.WriteString(path)
		lines.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(root, corpuspublication.ChecksumFile), []byte(lines.String()), 0o644)
}

func closedLoopV5EnvironmentBindings(t *testing.T, cases []CaseEvidence) (string, string, string, string) {
	t.Helper()
	if len(cases) == 0 {
		t.Fatal("V5 baseline has no discovery environment binding")
	}
	first := cases[0]
	if !closedLoopV5ValidHash(first.InventoryHash) || !closedLoopV5ValidHash(first.CatalogHash) || !closedLoopV5ValidHash(first.ModelRegistryHash) || !closedLoopV5ValidHash(first.SynthesisPolicyHash) {
		t.Fatal("V5 baseline environment commitment is invalid")
	}
	for _, current := range cases[1:] {
		if current.InventoryHash != first.InventoryHash || current.CatalogHash != first.CatalogHash || current.ModelRegistryHash != first.ModelRegistryHash || current.SynthesisPolicyHash != first.SynthesisPolicyHash {
			t.Fatal("V5 discovery cases used inconsistent synthesis environments")
		}
	}
	return first.InventoryHash, first.CatalogHash, first.ModelRegistryHash, first.SynthesisPolicyHash
}

func closedLoopV5BaselineAudit(report closedLoopV5BaselineReport, ranking closedLoopV5Ranking, selection closedLoopV5Selection) []byte {
	var counts []string
	for _, count := range report.OutcomeCounts {
		if count.Role == RoleDiscovery && count.Domain == "" {
			counts = append(counts, fmt.Sprintf("pass=%d unsupported=%d unsafe=%d exhausted=%d", count.Pass, count.Unsupported, count.Unsafe, count.Exhausted))
		}
	}
	return []byte(fmt.Sprintf("# V5 Discovery Baseline Audit\n\nThe public discovery baseline was produced from 18 frozen cases with two byte-identical synthesis runs per case. Installed-KiCad physical promotion was required twice for every pass. No held-out source, outcome, gap, diagnostic, or package membership was opened.\n\n- outcomes: %s\n- eligible packages: %d\n- selected scope: `%s`\n- selected capability: `%s`\n- affected discovery cases: %d\n- reporting domains: %d\n- exact member clusters: %d\n- baseline hash: `%s`\n- ranking hash: `%s`\n- selection hash: `%s`\n- selection freeze rule: the first Git commit containing this exact selection hash is the derived selection-freeze commit; `selection_freeze_parent_commit` records its non-circular parent.\n", strings.Join(counts, ", "), len(ranking.Packages), selection.Selected.Scope, selection.Selected.Capability, len(selection.Selected.Cases), len(selection.Selected.Domains), len(selection.Selected.Members), report.Hash, ranking.Hash, selection.Hash))
}

func assertClosedLoopV5BaselineFileSet(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	want := map[string]bool{corpuspublication.ChecksumFile: true, "BASELINE_AUDIT.md": true, "report.json": true, "package_ranking.json": true, "generic_plan.json": true, "selection.json": true}
	for _, entry := range manifest.Entries {
		if entry.Role == string(RoleDiscovery) {
			want[filepath.ToSlash(filepath.Join("discovery", entry.ID+".json"))] = true
		}
	}
	var got []string
	if err := filepath.WalkDir(closedLoopV5BaselineRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV5BaselineRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("V5 baseline file count = %d, want %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("V5 baseline contains unexpected file %s", path)
		}
		if strings.Contains(path, "held_out") {
			t.Fatalf("V5 baseline leaks held-out path %s", path)
		}
	}
}

func TestClosedLoopV5BaselineHashRejectsMutation(t *testing.T) {
	report := closedLoopV5BaselineReport{Schema: closedLoopV5BaselineSchema, Version: closedLoopV5BaselineVersion, CorpusFreezeCommit: closedLoopV5CorpusFreezeCommit}
	hash, err := hashClosedLoopV5BaselineReport(report)
	if err != nil {
		t.Fatal(err)
	}
	report.CorpusFreezeCommit = closedLoopV5InfrastructureCommit
	mutated, err := hashClosedLoopV5BaselineReport(report)
	if err != nil || mutated == hash {
		t.Fatal("V5 baseline hash did not bind mutation")
	}
}

func TestClosedLoopV5RankingIgnoresHeldOutByConstruction(t *testing.T) {
	policy := loadClosedLoopV5SelectionPolicy(t)
	if policy.HeldOutInfluence != "prohibited" {
		t.Fatal("V5 package policy permits held-out influence")
	}
	candidates, err := capabilitypackages.Build([]capabilitypackages.Observation{{Role: string(RoleHeldOut), CaseID: "hidden", ReportingDomain: "power", SafetyWeight: 5, Stage: "simulation", Scope: "simulation", Capability: "hidden", Code: "hidden"}}, policy)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("V5 held-out evidence influenced package ranking: %v %#v", err, candidates)
	}
}

func TestClosedLoopV5SelectionHashRejectsMutation(t *testing.T) {
	selection := closedLoopV5Selection{Schema: closedLoopV5SelectionSchema, Version: closedLoopV5BaselineVersion, CorpusFreezeCommit: closedLoopV5CorpusFreezeCommit}
	hash, err := hashClosedLoopV5Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.CorpusFreezeCommit = closedLoopV5InfrastructureCommit
	mutated, err := hashClosedLoopV5Selection(selection)
	if err != nil || mutated == hash {
		t.Fatal("V5 selection hash did not bind mutation")
	}
}
