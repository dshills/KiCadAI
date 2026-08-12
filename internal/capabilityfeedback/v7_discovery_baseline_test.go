package capabilityfeedback

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityrounds"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV7BaselineSchema        = "kicadai.closed-loop-open-set-discovery-baseline.v7"
	closedLoopV7CaseArtifactSchema    = "kicadai.closed-loop-open-set-discovery-case.v7"
	closedLoopV7RankingSchema         = "kicadai.closed-loop-open-set-bundle-ranking.v7"
	closedLoopV7GenericPlanSchema     = "kicadai.closed-loop-open-set-generic-plan.v7"
	closedLoopV7SelectionSchema       = "kicadai.closed-loop-open-set-selection.v7"
	closedLoopV7BaselineVersion       = 7
	closedLoopV7BaselineRoot          = "testdata/closed_loop_open_set_v7_baseline"
	closedLoopV7BaselineUpdateEnv     = "UPDATE_CLOSED_LOOP_V7_DISCOVERY_BASELINE"
	closedLoopV7FullEvidenceVerifyEnv = "VERIFY_CLOSED_LOOP_V7_FULL_EVIDENCE"
	closedLoopV7CorpusFreezeCommit    = "00512aa1a480c3ddca353e369d15f676d88a7b54"
	closedLoopV7SelectionPolicyHash   = "da0fbb3948d6e422627f17ca0c85f3063dbf5cf3b3fa0cc781335ad2e642a7e7"
	closedLoopV7ContractManifestHash  = "40d1f64af6f06763bcb3c04275b56fd4d0c24dafe1940577618d78415408020e"

	// These are populated exactly once in the selection-freeze commit. Their
	// empty infrastructure values make an accidentally published baseline fail
	// closed until its exact bytes have been reviewed and frozen.
	closedLoopV7InfrastructureCommit = "109302c2cfe3af18fca0ec0e4ab3d6ee15d72206"
	closedLoopV7BaselineHash         = "de66137f7692693714a58a46101a9279417f12c4eba6604e4a263c01e7f86813"
	closedLoopV7FrontierHash         = "871e673e3b07c38b7369a0e595dcc8b48976c335f5e3a10ada519bf4a027837b"
	closedLoopV7RankingHash          = "69d8888ff5f45d124d064f9bba32a4078daae7368617917fae83482463f004b2"
	closedLoopV7GenericPlanHash      = "c020b6116610d8d392eb580cbaeebfa39655cf65219617781d698827364e1d93"
	closedLoopV7SelectionHash        = "87915abf1d8371b94a652c939df8122e05ee0d0bd18d2b674af7504f9813738d"
)

type closedLoopV7CaseArtifact struct {
	Schema            string                       `json:"schema"`
	Version           int                          `json:"version"`
	CaseID            string                       `json:"case_id"`
	RequirementSHA256 string                       `json:"requirement_sha256"`
	Replays           []ots.SynthesisRun           `json:"replays"`
	Promotion         *ots.PhysicalPromotionResult `json:"promotion,omitempty"`
	Observation       CaseEvidence                 `json:"observation"`
	Hash              string                       `json:"hash"`
}

type closedLoopV7ArtifactRef struct {
	CaseID string `json:"case_id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopV7BaselineReport struct {
	Schema                    string                           `json:"schema"`
	Version                   int                              `json:"version"`
	CorpusManifestSHA256      string                           `json:"corpus_manifest_sha256"`
	CorpusFreezeCommit        string                           `json:"corpus_freeze_commit"`
	InfrastructureCommit      string                           `json:"infrastructure_commit"`
	ContractManifestSHA256    string                           `json:"contract_manifest_sha256"`
	EvaluatorPolicy           string                           `json:"evaluator_policy"`
	ImpactRegistryFileSHA256  string                           `json:"impact_registry_file_sha256"`
	SynthesisPolicyFileSHA256 string                           `json:"synthesis_policy_file_sha256"`
	GapPolicyFileSHA256       string                           `json:"gap_policy_file_sha256"`
	SelectionPolicySHA256     string                           `json:"selection_policy_sha256"`
	InventorySHA256           string                           `json:"inventory_sha256"`
	CatalogSHA256             string                           `json:"catalog_sha256"`
	ModelRegistrySHA256       string                           `json:"model_registry_sha256"`
	SynthesisPolicySHA256     string                           `json:"synthesis_policy_sha256"`
	PromotionEnvironment      closedLoopV5PromotionEnvironment `json:"promotion_environment"`
	OutcomeCounts             []closedLoopOutcomeCount         `json:"outcome_counts"`
	CaseArtifacts             []closedLoopV7ArtifactRef        `json:"case_artifacts"`
	FrontierSHA256            string                           `json:"frontier_sha256"`
	Discovery                 AggregateReport                  `json:"discovery"`
	Hash                      string                           `json:"hash"`
}

type closedLoopV7Ranking struct {
	Schema                string                     `json:"schema"`
	Version               int                        `json:"version"`
	BaselineSHA256        string                     `json:"baseline_sha256"`
	SelectionPolicySHA256 string                     `json:"selection_policy_sha256"`
	Decisions             capabilityrounds.Selection `json:"decisions"`
	Hash                  string                     `json:"hash"`
}

type closedLoopV7FrontierCase struct {
	CaseID             string                         `json:"case_id"`
	Outcome            string                         `json:"outcome"`
	RootFrontier       []capabilityrounds.Gap         `json:"root_frontier"`
	CandidateInventory []ots.CandidateReport          `json:"candidate_inventory"`
	SuppressedFailures []ots.SelectionRejection       `json:"suppressed_failures"`
	Diagnostics        []ots.Diagnostic               `json:"diagnostics"`
	CausalEdges        []capabilityrounds.LineageEdge `json:"causal_edges"`
	TransitionClasses  []closedLoopV7TransitionClass  `json:"transition_classes"`
}

type closedLoopV7TransitionClass struct {
	FromMemberKey string `json:"from_member_key"`
	ToMemberKey   string `json:"to_member_key"`
	Class         string `json:"class"`
}

type closedLoopV7FrontierGraph struct {
	Schema     string                     `json:"schema"`
	Version    int                        `json:"version"`
	Generation int                        `json:"generation"`
	Cases      []closedLoopV7FrontierCase `json:"cases"`
	Hash       string                     `json:"hash"`
}

type closedLoopV7PlanStep struct {
	Order            int      `json:"order"`
	MemberKey        string   `json:"member_key"`
	Stage            string   `json:"stage"`
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	RequiredEvidence []string `json:"required_evidence"`
}

type closedLoopV7GenericPlan struct {
	Schema              string                 `json:"schema"`
	Version             int                    `json:"version"`
	BundleKey           string                 `json:"bundle_key"`
	InputFrontierSHA256 string                 `json:"input_frontier_sha256"`
	Executable          bool                   `json:"executable"`
	AtomKeys            []string               `json:"atom_keys"`
	MemberKeys          []string               `json:"member_keys"`
	RequiredEvidence    []string               `json:"required_evidence"`
	Steps               []closedLoopV7PlanStep `json:"steps"`
	Hash                string                 `json:"hash"`
}

type closedLoopV7Selection struct {
	Schema                      string                     `json:"schema"`
	Version                     int                        `json:"version"`
	StartingCommit              string                     `json:"starting_commit"`
	ContractFreezeCommit        string                     `json:"contract_freeze_commit"`
	CorpusFreezeCommit          string                     `json:"corpus_freeze_commit"`
	InfrastructureCommit        string                     `json:"infrastructure_commit"`
	SelectionFreezeParentCommit string                     `json:"selection_freeze_parent_commit"`
	CorpusManifestSHA256        string                     `json:"corpus_manifest_sha256"`
	BaselineSHA256              string                     `json:"baseline_sha256"`
	RankingSHA256               string                     `json:"ranking_sha256"`
	SelectionPolicySHA256       string                     `json:"selection_policy_sha256"`
	Generation                  int                        `json:"generation"`
	ActiveCohort                []string                   `json:"active_cohort"`
	InputFrontierSHA256         string                     `json:"input_frontier_sha256"`
	Selected                    capabilityrounds.Candidate `json:"selected"`
	GenericPlanSHA256           string                     `json:"generic_plan_sha256"`
	Hash                        string                     `json:"hash"`
}

func TestClosedLoopV7DiscoveryBaselineIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV7BaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V7 discovery baseline has not been frozen")
		}
		t.Fatal(err)
	}
	if closedLoopV7InfrastructureCommit == "" || closedLoopV7BaselineHash == "" || closedLoopV7FrontierHash == "" || closedLoopV7RankingHash == "" || closedLoopV7GenericPlanHash == "" || closedLoopV7SelectionHash == "" {
		t.Fatal("V7 discovery baseline exists without literal freeze commitments")
	}
	if err := verifyClosedLoopV7BaselineChecksums(closedLoopV7BaselineRoot); err != nil {
		t.Fatalf("verify V7 discovery baseline checksums: %v", err)
	}
	var report closedLoopV7BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "report.json")), &report)
	var ranking closedLoopV7Ranking
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "bundle_ranking.json")), &ranking)
	var frontier closedLoopV7FrontierGraph
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "frontier_graph.json")), &frontier)
	var plan closedLoopV7GenericPlan
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "generic_plan.json")), &plan)
	var selection closedLoopV7Selection
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "selection.json")), &selection)
	if report.Hash != closedLoopV7BaselineHash || frontier.Hash != closedLoopV7FrontierHash || ranking.Hash != closedLoopV7RankingHash || plan.Hash != closedLoopV7GenericPlanHash || selection.Hash != closedLoopV7SelectionHash {
		t.Fatal("V7 discovery baseline literal commitments changed")
	}
	gotReportHash, err := hashClosedLoopV7BaselineReport(report)
	if err != nil || gotReportHash != report.Hash {
		t.Fatal("V7 discovery baseline report is not self-consistent")
	}
	gotRankingHash, err := hashClosedLoopV7Ranking(ranking)
	if err != nil || gotRankingHash != ranking.Hash || ranking.BaselineSHA256 != report.Hash {
		t.Fatal("V7 bundle ranking is not self-consistent")
	}
	gotFrontierHash, err := hashClosedLoopV7FrontierGraph(frontier)
	if err != nil || gotFrontierHash != frontier.Hash || report.FrontierSHA256 != frontier.Hash {
		t.Fatal("V7 frontier graph is not self-consistent")
	}
	if frontier.Generation != 0 || len(frontier.Cases) != closedLoopV7RoleSize {
		t.Fatal("V7 generation-zero frontier graph has invalid dimensions")
	}
	for index, current := range frontier.Cases {
		if current.CaseID != fmt.Sprintf("v7_case_%03d", index+1) || len(current.CausalEdges) != 0 || len(current.TransitionClasses) != 0 {
			t.Fatalf("V7 generation-zero frontier case %d is invalid", index+1)
		}
	}
	gotPlanHash, err := hashClosedLoopV7GenericPlan(plan)
	if err != nil || gotPlanHash != plan.Hash || plan.InputFrontierSHA256 != frontier.Hash {
		t.Fatal("V7 generic plan is not self-consistent")
	}
	gotSelectionHash, err := hashClosedLoopV7Selection(selection)
	if err != nil || gotSelectionHash != selection.Hash || selection.BaselineSHA256 != report.Hash || selection.RankingSHA256 != ranking.Hash || selection.GenericPlanSHA256 != plan.Hash || selection.InputFrontierSHA256 != frontier.Hash {
		t.Fatal("V7 selection is not self-consistent")
	}
	assertClosedLoopV7CaseArtifacts(t, report)
	policy := loadClosedLoopV7SelectionPolicy(t)
	roundCases := closedLoopV7RoundCases(report.Discovery)
	decisions, err := capabilityrounds.Select(roundCases, capabilityrounds.RoundState{}, policy)
	if err != nil || !bytes.Equal(corpusJSON(t, decisions), corpusJSON(t, ranking.Decisions)) {
		t.Fatal("V7 causal bundle decisions do not reproduce from discovery root gaps")
	}
	if decisions.Selected.Key != selection.Selected.Key {
		t.Fatal("V7 rank-one selection does not reproduce")
	}
	rebuiltPlan := buildClosedLoopV7GenericPlan(t, decisions.Selected, roundCases, frontier.Hash)
	if !bytes.Equal(corpusJSON(t, rebuiltPlan), corpusJSON(t, plan)) {
		t.Fatal("V7 generic plan does not reproduce from rank one")
	}
	if !plan.Executable || !bytes.Equal(corpusJSON(t, decisions.Selected), corpusJSON(t, selection.Selected)) ||
		!bytes.Equal(corpusJSON(t, closedLoopV7ActiveCohort(roundCases)), corpusJSON(t, selection.ActiveCohort)) {
		t.Fatal("V7 rank-one plan admission or active cohort does not reproduce")
	}
	assertClosedLoopV7BaselineFileSet(t)
}

func TestUpdateClosedLoopV7DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV7BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V7_DISCOVERY_BASELINE=1 to freeze the untouched V7 discovery baseline")
	}
	if _, err := os.Stat(closedLoopV7BaselineRoot); !os.IsNotExist(err) {
		t.Fatal("V7 discovery baseline already exists; refusing overwrite")
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	infrastructureCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	manifest := loadClosedLoopV7Manifest(t)
	registry, synthesisPolicy := closedLoopV7Policies(t)
	selectionPolicy := loadClosedLoopV7SelectionPolicy(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV7PromotionEnvironment(t, repositoryRoot)
	artifacts := runClosedLoopV7DiscoveryBaseline(t, manifest, synthesisPolicy, inventory, environment, promotionEnvironment)
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	roundCases := closedLoopV7RoundCases(discovery)
	decisions, err := capabilityrounds.Select(roundCases, capabilityrounds.RoundState{}, selectionPolicy)
	if err != nil {
		t.Fatalf("build V7 causal bundles: %v", err)
	}
	frontier := buildClosedLoopV7FrontierGraph(t, artifacts, roundCases)
	plan := buildClosedLoopV7GenericPlan(t, decisions.Selected, roundCases, frontier.Hash)
	selected := decisions.Selected

	if err := atomicdir.Publish(closedLoopV7BaselineRoot, func(root string) error {
		refs, err := writeClosedLoopV7CaseArtifacts(root, artifacts)
		if err != nil {
			return err
		}
		report := buildClosedLoopV7BaselineReport(t, infrastructureCommit, promotionEnvironment.Public, discovery, refs, frontier.Hash)
		ranking := buildClosedLoopV7Ranking(t, report.Hash, decisions)
		selection := buildClosedLoopV7Selection(t, infrastructureCommit, report.Hash, ranking.Hash, selected, closedLoopV7ActiveCohort(roundCases), frontier.Hash, plan.Hash)
		files := map[string][]byte{
			"report.json": corpusJSON(t, report), "bundle_ranking.json": corpusJSON(t, ranking),
			"frontier_graph.json": corpusJSON(t, frontier),
			"generic_plan.json":   corpusJSON(t, plan), "selection.json": corpusJSON(t, selection),
			"BASELINE_AUDIT.md": closedLoopV7BaselineAudit(report, ranking, selection),
		}
		for path, data := range files {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("V7 discovery baseline selected bundle=%s cases=%d domains=%d atoms=%d members=%d", selected.Key, len(selected.CoveredCaseIDs), len(selected.ReportingDomains), len(selected.Atoms), len(selected.Members))
}

func runClosedLoopV7DiscoveryBaseline(t *testing.T, manifest corpuspublication.Manifest, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) []closedLoopV7CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV7CaseArtifact, 0, closedLoopV7RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		t.Logf("V7 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, filepath.FromSlash(entry.StablePath)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s violates the frozen requirement contract", entry.ID)
		}
		first := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs: first_sha256=%s second_sha256=%s", entry.ID, corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopV5SealedRun(t, first, environment, promotionEnvironment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatalf("%s did not pass two clean-root installed-KiCad promotions", entry.ID)
			}
			promotion = &current
		}
		observation, err := ObserveRealizabilityAware(CaseMeta{ID: entry.ID, Role: RoleDiscovery, Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact)}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observation failed: %v", entry.ID, err)
		}
		artifact := closedLoopV7CaseArtifact{Schema: closedLoopV7CaseArtifactSchema, Version: closedLoopV7BaselineVersion, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, Replays: []ots.SynthesisRun{first, second}, Promotion: promotion, Observation: observation}
		artifact.Hash, err = hashClosedLoopV7CaseArtifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, artifact)
		t.Logf("V7 discovery baseline %s outcome=%s stop=%s root_gaps=%d", entry.ID, observation.Outcome, observation.StopReason, len(observation.Gaps))
	}
	if len(artifacts) != closedLoopV7RoleSize {
		t.Fatalf("V7 discovery baseline case count = %d, want %d", len(artifacts), closedLoopV7RoleSize)
	}
	return artifacts
}

func loadClosedLoopV7Manifest(t *testing.T) corpuspublication.Manifest {
	t.Helper()
	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ManifestFile)), &manifest)
	if manifest.Schema != corpuspublication.ManifestSchemaV7 || corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ManifestFile))) != closedLoopV7CorpusManifestHash {
		t.Fatal("V7 corpus manifest is not frozen")
	}
	return manifest
}

func loadClosedLoopV7SelectionPolicy(t *testing.T) capabilityrounds.Policy {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V7_SELECTION_POLICY.json"))
	if corpusHash(data) != closedLoopV7SelectionPolicyHash {
		t.Fatal("V7 selection policy differs from its frozen commitment")
	}
	policy, err := capabilityrounds.DecodePolicy(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func closedLoopV7Policies(t *testing.T) (capabilityevaluation.ImpactRegistry, ots.Policy) {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	for name, want := range map[string]string{
		"V4_IMPACT_REGISTRY.json":       closedLoopV5ImpactRegistryFileHash,
		"V4_SYNTHESIS_POLICY.json":      closedLoopV5SynthesisPolicyFileHash,
		"V4_GAP_TRANSITION_POLICY.json": closedLoopV5GapPolicyFileHash,
		"V7_CONTRACT.sha256":            closedLoopV7ContractManifestHash,
	} {
		if got := corpusHash(mustCorpusRead(t, filepath.Join(specRoot, name))); got != want {
			t.Fatalf("V7 inherited commitment %s = %s, want %s", name, got, want)
		}
	}
	return closedLoopV4Policies(t)
}

func resolveClosedLoopV7PromotionEnvironment(t *testing.T, repositoryRoot string) closedLoopV5ResolvedPromotionEnvironment {
	t.Helper()
	resolved := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot)
	resolved.Public.Schema = "kicadai.closed-loop-open-set-promotion-environment.v7"
	resolved.Public.Version = closedLoopV7BaselineVersion
	var err error
	resolved.Public.Hash, err = hashClosedLoopV5PromotionEnvironment(resolved.Public)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func closedLoopV7RoundCases(report AggregateReport) []capabilityrounds.Case {
	cases := make([]capabilityrounds.Case, 0, len(report.Cases))
	for _, evidence := range report.Cases {
		gaps := make([]capabilityrounds.Gap, len(evidence.Gaps))
		for index, gap := range evidence.Gaps {
			required := append([]string(nil), gap.RequiredEvidence...)
			sort.Strings(required)
			gaps[index] = capabilityrounds.Gap{Stage: gap.Stage, Scope: string(gap.Scope), Capability: gap.Capability, Code: gap.Code, CausalToken: closedLoopV7CausalToken(gap), RequiredEvidence: required}
		}
		cases = append(cases, capabilityrounds.Case{Role: string(evidence.Case.Role), ID: evidence.Case.ID, ReportingDomain: string(evidence.Case.Domain), SafetyImpact: string(evidence.Case.SafetyImpact), Outcome: string(evidence.Outcome), Frontier: gaps})
	}
	return cases
}

func closedLoopV7CausalToken(gap Gap) string {
	value := struct {
		Stage            string   `json:"stage"`
		Scope            GapScope `json:"scope"`
		Capability       string   `json:"capability"`
		Code             string   `json:"code"`
		RequirementIDs   []string `json:"requirement_ids"`
		OperatingCases   []string `json:"operating_cases"`
		AnalysisKinds    []string `json:"analysis_kinds"`
		RequiredEvidence []string `json:"required_evidence"`
	}{gap.Stage, gap.Scope, gap.Capability, gap.Code, gap.RequirementIDs, gap.OperatingCases, gap.AnalysisKinds, gap.RequiredEvidence}
	digest, err := digest(value)
	if err != nil {
		panic(err)
	}
	return digest
}

func buildClosedLoopV7GenericPlan(t *testing.T, selected capabilityrounds.Candidate, cases []capabilityrounds.Case, frontierHash string) closedLoopV7GenericPlan {
	t.Helper()
	atomKeys := make([]string, len(selected.Atoms))
	for index := range selected.Atoms {
		atomKeys[index] = selected.Atoms[index].Key
	}
	memberKeys := make([]string, len(selected.Members))
	steps := make([]closedLoopV7PlanStep, len(selected.Members))
	allEvidence := map[string]bool{}
	for index, member := range selected.Members {
		memberKeys[index] = member.Key
		memberEvidenceSet := map[string]bool{}
		for _, current := range cases {
			for _, gap := range current.Frontier {
				if closedLoopV7CanonicalStage(gap.Stage) != member.Stage || gap.Scope != member.Scope || gap.Capability != member.Capability || gap.Code != member.Code {
					continue
				}
				for _, evidence := range gap.RequiredEvidence {
					memberEvidenceSet[evidence] = true
					allEvidence[evidence] = true
				}
			}
		}
		memberEvidence := sortedStringBoolKeys(memberEvidenceSet)
		if len(memberEvidence) == 0 {
			t.Fatalf("V7 generic-plan member %s has no capability-specific evidence", member.Key)
		}
		steps[index] = closedLoopV7PlanStep{Order: index + 1, MemberKey: member.Key, Stage: member.Stage, Scope: member.Scope, Capability: member.Capability, Code: member.Code, RequiredEvidence: memberEvidence}
	}
	plan := closedLoopV7GenericPlan{Schema: closedLoopV7GenericPlanSchema, Version: closedLoopV7BaselineVersion, BundleKey: selected.Key, InputFrontierSHA256: frontierHash, Executable: true, AtomKeys: atomKeys, MemberKeys: memberKeys, RequiredEvidence: sortedStringBoolKeys(allEvidence), Steps: steps}
	var err error
	plan.Hash, err = hashClosedLoopV7GenericPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func closedLoopV7CanonicalStage(stage string) string {
	if stage == "roundtrip" {
		return "round_trip"
	}
	return stage
}

func sortedStringBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func closedLoopV7ActiveCohort(cases []capabilityrounds.Case) []string {
	var ids []string
	for _, current := range cases {
		if current.Outcome == string(OutcomeUnsupported) || current.Outcome == string(OutcomeExhausted) {
			ids = append(ids, current.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func buildClosedLoopV7FrontierGraph(t *testing.T, artifacts []closedLoopV7CaseArtifact, cases []capabilityrounds.Case) closedLoopV7FrontierGraph {
	t.Helper()
	byID := make(map[string]capabilityrounds.Case, len(cases))
	for _, current := range cases {
		byID[current.ID] = current
	}
	frontier := closedLoopV7FrontierGraph{Schema: "kicadai.closed-loop-open-set-frontier.v7", Version: closedLoopV7BaselineVersion, Generation: 0}
	for _, artifact := range artifacts {
		current, ok := byID[artifact.CaseID]
		if !ok || len(artifact.Replays) != 2 {
			t.Fatalf("V7 frontier source is incomplete for %s", artifact.CaseID)
		}
		report := artifact.Replays[0].Report
		var suppressed []ots.SelectionRejection
		if report.Selected != nil {
			suppressed = append(suppressed, report.Selected.Ranking.Rejections...)
		}
		frontier.Cases = append(frontier.Cases, closedLoopV7FrontierCase{CaseID: artifact.CaseID, Outcome: current.Outcome, RootFrontier: current.Frontier, CandidateInventory: append([]ots.CandidateReport(nil), report.Candidates...), SuppressedFailures: suppressed, Diagnostics: append([]ots.Diagnostic(nil), report.Diagnostics...), CausalEdges: []capabilityrounds.LineageEdge{}, TransitionClasses: []closedLoopV7TransitionClass{}})
	}
	var err error
	frontier.Hash, err = hashClosedLoopV7FrontierGraph(frontier)
	if err != nil {
		t.Fatal(err)
	}
	return frontier
}

func writeClosedLoopV7CaseArtifacts(root string, artifacts []closedLoopV7CaseArtifact) ([]closedLoopV7ArtifactRef, error) {
	discoveryRoot := filepath.Join(root, "discovery")
	if err := os.Mkdir(discoveryRoot, 0o755); err != nil {
		return nil, err
	}
	refs := make([]closedLoopV7ArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json.gz"))
		digest, err := writeClosedLoopV7CompressedArtifact(filepath.Join(root, filepath.FromSlash(path)), artifact)
		if err != nil {
			return nil, err
		}
		refs = append(refs, closedLoopV7ArtifactRef{CaseID: artifact.CaseID, Path: path, SHA256: digest})
	}
	return refs, nil
}

func writeClosedLoopV7CompressedArtifact(path string, artifact closedLoopV7CaseArtifact) (digest string, err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	hash := sha256.New()
	compressed, err := gzip.NewWriterLevel(io.MultiWriter(file, hash), gzip.BestCompression)
	if err != nil {
		return "", err
	}
	compressed.Header.ModTime = time.Unix(0, 0).UTC()
	compressed.Header.OS = 255
	if err := json.NewEncoder(compressed).Encode(artifact); err != nil {
		_ = compressed.Close()
		return "", fmt.Errorf("encode compressed V7 evidence %s: %w", artifact.CaseID, err)
	}
	if err := compressed.Close(); err != nil {
		return "", fmt.Errorf("close compressed V7 evidence %s: %w", artifact.CaseID, err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync compressed V7 evidence %s: %w", artifact.CaseID, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close V7 evidence file %s: %w", artifact.CaseID, err)
	}
	closed = true
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func assertClosedLoopV7CaseArtifacts(t *testing.T, report closedLoopV7BaselineReport) {
	t.Helper()
	if len(report.CaseArtifacts) != closedLoopV7RoleSize || report.Discovery.CaseCount != closedLoopV7RoleSize {
		t.Fatal("V7 discovery evidence has an invalid case count")
	}
	for index, ref := range report.CaseArtifacts {
		wantID := fmt.Sprintf("v7_case_%03d", index+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json.gz"))
		if ref.CaseID != wantID || ref.Path != wantPath || !closedLoopV7ValidHash(ref.SHA256) {
			t.Fatalf("V7 discovery evidence reference %d is invalid", index+1)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V7 discovery evidence %s differs from its report commitment", ref.CaseID)
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("V7 discovery evidence %s is not a gzip stream", ref.CaseID)
		}
		if os.Getenv(closedLoopV7FullEvidenceVerifyEnv) != "1" {
			firstByte := []byte{0}
			read, readErr := reader.Read(firstByte)
			closeErr := reader.Close()
			if read != 1 || readErr != nil || closeErr != nil || firstByte[0] != '{' {
				t.Fatalf("V7 discovery evidence %s has an invalid canonical JSON stream", ref.CaseID)
			}
			continue
		}
		var artifact closedLoopV7CaseArtifact
		decoder := json.NewDecoder(reader)
		decodeErr := decoder.Decode(&artifact)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		closeErr := reader.Close()
		expected, hashErr := hashClosedLoopV7CaseArtifact(artifact)
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || hashErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV7CaseArtifactSchema || artifact.Version != closedLoopV7BaselineVersion || artifact.CaseID != wantID || len(artifact.Replays) != 2 {
			t.Fatalf("V7 discovery evidence %s is structurally invalid", ref.CaseID)
		}
		first, firstErr := json.Marshal(artifact.Replays[0])
		second, secondErr := json.Marshal(artifact.Replays[1])
		if firstErr != nil || secondErr != nil || !bytes.Equal(first, second) || artifact.Observation.SynthesisHash != artifact.Replays[0].Hash {
			t.Fatalf("V7 discovery evidence %s lacks byte-identical complete synthesis replay", ref.CaseID)
		}
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != ots.PhysicalPromotionPassed || !artifact.Promotion.ReplayIdentical || len(artifact.Promotion.Runs) != 2 || artifact.Promotion.Hash != artifact.Observation.PromotionHash || artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				t.Fatalf("V7 passing discovery case %s lacks complete installed-KiCad promotion evidence", ref.CaseID)
			}
		} else if artifact.Promotion != nil {
			t.Fatalf("V7 nonpassing discovery case %s contains promotion evidence", ref.CaseID)
		}
	}
}

func buildClosedLoopV7BaselineReport(t *testing.T, infrastructureCommit string, promotionEnvironment closedLoopV5PromotionEnvironment, discovery AggregateReport, refs []closedLoopV7ArtifactRef, frontierHash string) closedLoopV7BaselineReport {
	t.Helper()
	inventoryHash, catalogHash, modelRegistryHash, synthesisPolicyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
	report := closedLoopV7BaselineReport{Schema: closedLoopV7BaselineSchema, Version: closedLoopV7BaselineVersion, CorpusManifestSHA256: closedLoopV7CorpusManifestHash, CorpusFreezeCommit: closedLoopV7CorpusFreezeCommit, InfrastructureCommit: infrastructureCommit, ContractManifestSHA256: closedLoopV7ContractManifestHash, EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistryFileSHA256: closedLoopV5ImpactRegistryFileHash, SynthesisPolicyFileSHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicyFileSHA256: closedLoopV5GapPolicyFileHash, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash, InventorySHA256: inventoryHash, CatalogSHA256: catalogHash, ModelRegistrySHA256: modelRegistryHash, SynthesisPolicySHA256: synthesisPolicyHash, PromotionEnvironment: promotionEnvironment, OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs, FrontierSHA256: frontierHash, Discovery: discovery}
	var err error
	report.Hash, err = hashClosedLoopV7BaselineReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func buildClosedLoopV7Ranking(t *testing.T, baselineHash string, decisions capabilityrounds.Selection) closedLoopV7Ranking {
	t.Helper()
	ranking := closedLoopV7Ranking{Schema: closedLoopV7RankingSchema, Version: closedLoopV7BaselineVersion, BaselineSHA256: baselineHash, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash, Decisions: decisions}
	var err error
	ranking.Hash, err = hashClosedLoopV7Ranking(ranking)
	if err != nil {
		t.Fatal(err)
	}
	return ranking
}

func buildClosedLoopV7Selection(t *testing.T, infrastructureCommit, baselineHash, rankingHash string, selected capabilityrounds.Candidate, activeCohort []string, frontierHash, planHash string) closedLoopV7Selection {
	t.Helper()
	selection := closedLoopV7Selection{Schema: closedLoopV7SelectionSchema, Version: closedLoopV7BaselineVersion, StartingCommit: "156f7eb439ca5313471c504ddb91db1b8a8724f0", ContractFreezeCommit: "e780c8cfca51623d81b9eae209fedf2b98816681", CorpusFreezeCommit: closedLoopV7CorpusFreezeCommit, InfrastructureCommit: infrastructureCommit, SelectionFreezeParentCommit: infrastructureCommit, CorpusManifestSHA256: closedLoopV7CorpusManifestHash, BaselineSHA256: baselineHash, RankingSHA256: rankingHash, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash, Generation: 0, ActiveCohort: activeCohort, InputFrontierSHA256: frontierHash, Selected: selected, GenericPlanSHA256: planHash}
	var err error
	selection.Hash, err = hashClosedLoopV7Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func hashClosedLoopV7CaseArtifact(value closedLoopV7CaseArtifact) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV7BaselineReport(value closedLoopV7BaselineReport) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV7Ranking(value closedLoopV7Ranking) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV7FrontierGraph(value closedLoopV7FrontierGraph) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV7GenericPlan(value closedLoopV7GenericPlan) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV7Selection(value closedLoopV7Selection) (string, error) {
	value.Hash = ""
	return digest(value)
}

func closedLoopV7BaselineAudit(report closedLoopV7BaselineReport, ranking closedLoopV7Ranking, selection closedLoopV7Selection) []byte {
	var counts []string
	for _, count := range report.OutcomeCounts {
		if count.Role == RoleDiscovery && count.Domain == "" {
			counts = append(counts, fmt.Sprintf("pass=%d unsupported=%d unsafe=%d exhausted=%d", count.Pass, count.Unsupported, count.Unsafe, count.Exhausted))
		}
	}
	return []byte(fmt.Sprintf("# V7 Discovery Baseline Audit\n\nThe public discovery baseline used 18 frozen cases, two byte-identical complete synthesis exports per case, and two clean-root installed-KiCad promotions for every pass. Selection consumed only complete normalized discovery root gaps and published the complete semantic co-rank-one set. Generation-zero causal edges and transition classifications are explicitly empty; no held-out source, outcome, gap, diagnostic, frontier, or bundle membership was opened.\n\n- outcomes: %s\n- active cohort cases: %d\n- candidate bundles: %d\n- eligible bundles: %d\n- semantic co-rank-one bundles: %d\n- selected bundle key: `%s`\n- covered discovery cases: %d\n- reporting domains: %d\n- capability atoms: %d\n- exact root members: %d\n- frontier hash: `%s`\n- baseline hash: `%s`\n- ranking hash: `%s`\n- selection hash: `%s`\n", strings.Join(counts, ", "), len(selection.ActiveCohort), ranking.Decisions.CandidateCount, len(ranking.Decisions.EligibleCandidates), len(ranking.Decisions.CoRankOne), selection.Selected.Key, len(selection.Selected.CoveredCaseIDs), len(selection.Selected.ReportingDomains), len(selection.Selected.Atoms), len(selection.Selected.Members), selection.InputFrontierSHA256, report.Hash, ranking.Hash, selection.Hash))
}

func assertClosedLoopV7BaselineFileSet(t *testing.T) {
	t.Helper()
	want := map[string]bool{corpuspublication.ChecksumFile: true, "BASELINE_AUDIT.md": true, "report.json": true, "bundle_ranking.json": true, "frontier_graph.json": true, "generic_plan.json": true, "selection.json": true}
	for index := 1; index <= closedLoopV7RoleSize; index++ {
		want[fmt.Sprintf("discovery/v7_case_%03d.json.gz", index)] = true
	}
	var got []string
	if err := filepath.WalkDir(closedLoopV7BaselineRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV7BaselineRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("V7 baseline file count = %d, want %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] || strings.Contains(path, "held_out") {
			t.Fatalf("V7 baseline contains forbidden file %s", path)
		}
	}
}

func verifyClosedLoopV7BaselineChecksums(root string) error {
	manifestPath := filepath.Join(root, corpuspublication.ChecksumFile)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil || len(manifest) == 0 || len(manifest) > 1<<20 {
		return fmt.Errorf("V7 baseline checksum manifest is invalid")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	seen := map[string]bool{}
	previous := ""
	for scanner.Scan() {
		digest, relative, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || !closedLoopV7ValidHash(digest) || relative <= previous || seen[relative] || filepath.IsAbs(relative) || filepath.ToSlash(filepath.Clean(relative)) != relative || relative == "." || strings.HasPrefix(relative, "../") {
			return fmt.Errorf("V7 baseline checksum manifest contains an invalid entry")
		}
		seen[relative] = true
		previous = relative
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<20 {
			return fmt.Errorf("V7 baseline checksum source is not a bounded regular file")
		}
		realPath, realErr := filepath.EvalSymlinks(path)
		rel, relErr := filepath.Rel(root, realPath)
		if realErr != nil || relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("V7 baseline checksum source escapes its root")
		}
		file, openErr := os.Open(realPath)
		if openErr != nil {
			return openErr
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != digest {
			return fmt.Errorf("V7 baseline checksum entry does not match its commitment")
		}
	}
	if err := scanner.Err(); err != nil || len(seen) == 0 {
		return fmt.Errorf("V7 baseline checksum manifest is empty or unreadable")
	}
	return nil
}

func TestClosedLoopV7RankingIgnoresHeldOutByConstruction(t *testing.T) {
	policy := loadClosedLoopV7SelectionPolicy(t)
	cases := make([]capabilityrounds.Case, policy.ExpectedDiscoveryCaseCount)
	cases[0] = capabilityrounds.Case{Role: string(RoleHeldOut), ID: "hidden", ReportingDomain: "power", SafetyImpact: "safety_critical", Outcome: string(OutcomeUnsupported), Frontier: []capabilityrounds.Gap{{Stage: "simulation", Scope: "simulation", Capability: "hidden", Code: "hidden", CausalToken: "hidden", RequiredEvidence: []string{"evidence"}}}}
	result, err := capabilityrounds.Select(cases, capabilityrounds.RoundState{}, policy)
	if err == nil || result.CandidateCount != 0 {
		t.Fatalf("V7 held-out evidence did not fail closed: %v %#v", err, result)
	}
}

func TestClosedLoopV7SelectionHashRejectsMutation(t *testing.T) {
	selection := closedLoopV7Selection{Schema: closedLoopV7SelectionSchema, Version: closedLoopV7BaselineVersion, CorpusFreezeCommit: closedLoopV7CorpusFreezeCommit}
	hash, err := hashClosedLoopV7Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.CorpusFreezeCommit = "0000000000000000000000000000000000000000"
	mutated, err := hashClosedLoopV7Selection(selection)
	if err != nil || mutated == hash {
		t.Fatal("V7 selection hash did not bind mutation")
	}
}

func TestClosedLoopV7CompressedEvidenceIsDeterministic(t *testing.T) {
	artifact := closedLoopV7CaseArtifact{Schema: closedLoopV7CaseArtifactSchema, Version: closedLoopV7BaselineVersion, CaseID: "v7_case_001", RequirementSHA256: strings.Repeat("a", 64)}
	firstRoot, secondRoot := t.TempDir(), t.TempDir()
	firstRefs, err := writeClosedLoopV7CaseArtifacts(firstRoot, []closedLoopV7CaseArtifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	secondRefs, err := writeClosedLoopV7CaseArtifacts(secondRoot, []closedLoopV7CaseArtifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(corpusJSON(t, firstRefs), corpusJSON(t, secondRefs)) {
		t.Fatal("V7 compressed evidence references are nondeterministic")
	}
	first := mustCorpusRead(t, filepath.Join(firstRoot, filepath.FromSlash(firstRefs[0].Path)))
	second := mustCorpusRead(t, filepath.Join(secondRoot, filepath.FromSlash(secondRefs[0].Path)))
	if !bytes.Equal(first, second) {
		t.Fatal("V7 compressed evidence bytes are nondeterministic")
	}
}
