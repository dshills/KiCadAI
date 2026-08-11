package capabilityfeedback

import (
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
	"kicadai/internal/capabilitybundles"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV6BaselineSchema        = "kicadai.closed-loop-open-set-discovery-baseline.v6"
	closedLoopV6CaseArtifactSchema    = "kicadai.closed-loop-open-set-discovery-case.v6"
	closedLoopV6RankingSchema         = "kicadai.closed-loop-open-set-bundle-ranking.v6"
	closedLoopV6GenericPlanSchema     = "kicadai.closed-loop-open-set-generic-plan.v6"
	closedLoopV6SelectionSchema       = "kicadai.closed-loop-open-set-selection.v6"
	closedLoopV6BaselineVersion       = 6
	closedLoopV6BaselineRoot          = "testdata/closed_loop_open_set_v6_baseline"
	closedLoopV6BaselineUpdateEnv     = "UPDATE_CLOSED_LOOP_V6_DISCOVERY_BASELINE"
	closedLoopV6FullEvidenceVerifyEnv = "VERIFY_CLOSED_LOOP_V6_FULL_EVIDENCE"
	closedLoopV6CorpusFreezeCommit    = "209c84ca1d7a0b65beca74f3aa2c46a1fe95bed9"
	closedLoopV6SelectionPolicyHash   = "1d82387013cccff736b33b497194586254c25a395db97d25d923abf1b658e2f3"
	closedLoopV6ContractManifestHash  = "61f76c00477f2f6eb350556f4e2d0ba85b338846a9b61ed92691263c9552f591"

	// These are populated exactly once in the selection-freeze commit. Their
	// empty infrastructure values make an accidentally published baseline fail
	// closed until its exact bytes have been reviewed and frozen.
	closedLoopV6InfrastructureCommit = "e07c423ae36cd969e7aa199304299e6c6eae3632"
	closedLoopV6BaselineHash         = "4183ad9ba2759ddb1f1d6dd8585d2afed45573ab42e1c41e69b16d347a3e56a5"
	closedLoopV6RankingHash          = "6d2c5c79d4e99e181a181460aff9dc0b54d86a141c360e868ba451031fcc6d50"
	closedLoopV6GenericPlanHash      = "7da60ccc693eaef1457f48ff77000c30e91896d3665a9d780181fdc6917062bc"
	closedLoopV6SelectionHash        = "9f1a9f8120d81b9c09a75dea382b6fcf6b94b15f620c05b37beb25217efab13b"
)

type closedLoopV6CaseArtifact struct {
	Schema            string                       `json:"schema"`
	Version           int                          `json:"version"`
	CaseID            string                       `json:"case_id"`
	RequirementSHA256 string                       `json:"requirement_sha256"`
	Replays           []ots.SynthesisRun           `json:"replays"`
	Promotion         *ots.PhysicalPromotionResult `json:"promotion,omitempty"`
	Observation       CaseEvidence                 `json:"observation"`
	Hash              string                       `json:"hash"`
}

type closedLoopV6ArtifactRef struct {
	CaseID string `json:"case_id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopV6BaselineReport struct {
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
	CaseArtifacts             []closedLoopV6ArtifactRef        `json:"case_artifacts"`
	Discovery                 AggregateReport                  `json:"discovery"`
	Hash                      string                           `json:"hash"`
}

type closedLoopV6Ranking struct {
	Schema                string                   `json:"schema"`
	Version               int                      `json:"version"`
	BaselineSHA256        string                   `json:"baseline_sha256"`
	SelectionPolicySHA256 string                   `json:"selection_policy_sha256"`
	Decisions             capabilitybundles.Result `json:"decisions"`
	Hash                  string                   `json:"hash"`
}

type closedLoopV6PlanStep struct {
	Order            int      `json:"order"`
	MemberKey        string   `json:"member_key"`
	Stage            string   `json:"stage"`
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	RequiredEvidence []string `json:"required_evidence"`
}

type closedLoopV6GenericPlan struct {
	Schema           string                         `json:"schema"`
	Version          int                            `json:"version"`
	BundleKey        string                         `json:"bundle_key"`
	Admission        capabilitybundles.PlanEvidence `json:"admission"`
	RequiredEvidence []string                       `json:"required_evidence"`
	Steps            []closedLoopV6PlanStep         `json:"steps"`
	Hash             string                         `json:"hash"`
}

type closedLoopV6Selection struct {
	Schema                      string                      `json:"schema"`
	Version                     int                         `json:"version"`
	StartingCommit              string                      `json:"starting_commit"`
	ContractFreezeCommit        string                      `json:"contract_freeze_commit"`
	CorpusFreezeCommit          string                      `json:"corpus_freeze_commit"`
	InfrastructureCommit        string                      `json:"infrastructure_commit"`
	SelectionFreezeParentCommit string                      `json:"selection_freeze_parent_commit"`
	CorpusManifestSHA256        string                      `json:"corpus_manifest_sha256"`
	BaselineSHA256              string                      `json:"baseline_sha256"`
	RankingSHA256               string                      `json:"ranking_sha256"`
	SelectionPolicySHA256       string                      `json:"selection_policy_sha256"`
	Selected                    capabilitybundles.Candidate `json:"selected"`
	GenericPlanSHA256           string                      `json:"generic_plan_sha256"`
	Hash                        string                      `json:"hash"`
}

func TestClosedLoopV6DiscoveryBaselineIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV6BaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V6 discovery baseline has not been frozen")
		}
		t.Fatal(err)
	}
	if closedLoopV6InfrastructureCommit == "" || closedLoopV6BaselineHash == "" || closedLoopV6RankingHash == "" || closedLoopV6GenericPlanHash == "" || closedLoopV6SelectionHash == "" {
		t.Fatal("V6 discovery baseline exists without literal freeze commitments")
	}
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV6BaselineRoot, filepath.Join(closedLoopV6BaselineRoot, corpuspublication.ChecksumFile)); err != nil {
		t.Fatalf("verify V6 discovery baseline checksums: %v", err)
	}
	var report closedLoopV6BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6BaselineRoot, "report.json")), &report)
	var ranking closedLoopV6Ranking
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6BaselineRoot, "bundle_ranking.json")), &ranking)
	var plan closedLoopV6GenericPlan
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6BaselineRoot, "generic_plan.json")), &plan)
	var selection closedLoopV6Selection
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6BaselineRoot, "selection.json")), &selection)
	if report.Hash != closedLoopV6BaselineHash || ranking.Hash != closedLoopV6RankingHash || plan.Hash != closedLoopV6GenericPlanHash || selection.Hash != closedLoopV6SelectionHash {
		t.Fatal("V6 discovery baseline literal commitments changed")
	}
	if got, _ := hashClosedLoopV6BaselineReport(report); got != report.Hash {
		t.Fatal("V6 discovery baseline report is not self-consistent")
	}
	if got, _ := hashClosedLoopV6Ranking(ranking); got != ranking.Hash || ranking.BaselineSHA256 != report.Hash {
		t.Fatal("V6 bundle ranking is not self-consistent")
	}
	if got, _ := hashClosedLoopV6GenericPlan(plan); got != plan.Hash {
		t.Fatal("V6 generic plan is not self-consistent")
	}
	if got, _ := hashClosedLoopV6Selection(selection); got != selection.Hash || selection.BaselineSHA256 != report.Hash || selection.RankingSHA256 != ranking.Hash || selection.GenericPlanSHA256 != plan.Hash {
		t.Fatal("V6 selection is not self-consistent")
	}
	assertClosedLoopV6CaseArtifacts(t, report)
	policy := loadClosedLoopV6SelectionPolicy(t)
	decisions, err := capabilitybundles.Build(closedLoopV6BundleCases(report.Discovery), policy)
	if err != nil || !bytes.Equal(corpusJSON(t, decisions), corpusJSON(t, ranking.Decisions)) {
		t.Fatal("V6 causal bundle decisions do not reproduce from discovery root gaps")
	}
	rankOne, err := capabilitybundles.SelectRankOne(decisions)
	if err != nil || rankOne.Key != selection.Selected.Key {
		t.Fatal("V6 rank-one selection does not reproduce")
	}
	rebuiltPlan := buildClosedLoopV6GenericPlan(t, rankOne)
	if !bytes.Equal(corpusJSON(t, rebuiltPlan), corpusJSON(t, plan)) {
		t.Fatal("V6 generic plan does not reproduce from rank one")
	}
	admitted, err := capabilitybundles.AdmitRankOne(decisions, map[string]capabilitybundles.PlanEvidence{rankOne.Key: plan.Admission})
	if err != nil || !bytes.Equal(corpusJSON(t, admitted), corpusJSON(t, selection.Selected)) {
		t.Fatal("V6 rank-one plan admission does not reproduce")
	}
	assertClosedLoopV6BaselineFileSet(t)
}

func TestUpdateClosedLoopV6DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV6BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V6_DISCOVERY_BASELINE=1 to freeze the untouched V6 discovery baseline")
	}
	if _, err := os.Stat(closedLoopV6BaselineRoot); !os.IsNotExist(err) {
		t.Fatal("V6 discovery baseline already exists; refusing overwrite")
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	infrastructureCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	manifest := loadClosedLoopV6Manifest(t)
	registry, synthesisPolicy := closedLoopV6Policies(t)
	selectionPolicy := loadClosedLoopV6SelectionPolicy(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV6PromotionEnvironment(t, repositoryRoot)
	artifacts := runClosedLoopV6DiscoveryBaseline(t, manifest, synthesisPolicy, inventory, environment, promotionEnvironment)
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal(err)
	}
	bundleCases := closedLoopV6BundleCases(discovery)
	decisions, err := capabilitybundles.Build(bundleCases, selectionPolicy)
	if err != nil {
		t.Fatalf("build V6 causal bundles: %v", err)
	}
	rankOne, err := capabilitybundles.SelectRankOne(decisions)
	if err != nil {
		t.Fatalf("select V6 rank one: %v", err)
	}
	plan := buildClosedLoopV6GenericPlan(t, rankOne)
	selected, err := capabilitybundles.AdmitRankOne(decisions, map[string]capabilitybundles.PlanEvidence{rankOne.Key: plan.Admission})
	if err != nil {
		t.Fatalf("admit V6 rank one plan: %v", err)
	}

	if err := atomicdir.Publish(closedLoopV6BaselineRoot, func(root string) error {
		refs, err := writeClosedLoopV6CaseArtifacts(root, artifacts)
		if err != nil {
			return err
		}
		report := buildClosedLoopV6BaselineReport(t, infrastructureCommit, promotionEnvironment.Public, discovery, refs)
		ranking := buildClosedLoopV6Ranking(t, report.Hash, decisions)
		selection := buildClosedLoopV6Selection(t, infrastructureCommit, report.Hash, ranking.Hash, selected, plan.Hash)
		files := map[string][]byte{
			"report.json": corpusJSON(t, report), "bundle_ranking.json": corpusJSON(t, ranking),
			"generic_plan.json": corpusJSON(t, plan), "selection.json": corpusJSON(t, selection),
			"BASELINE_AUDIT.md": closedLoopV6BaselineAudit(report, ranking, selection),
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
	t.Logf("V6 discovery baseline selected bundle=%s cases=%d domains=%d atoms=%d members=%d", selected.Key, len(selected.UnlockedCases), len(selected.ReportingDomains), len(selected.Atoms), len(selected.Members))
}

func runClosedLoopV6DiscoveryBaseline(t *testing.T, manifest corpuspublication.Manifest, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) []closedLoopV6CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV6CaseArtifact, 0, closedLoopV6RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		t.Logf("V6 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV6CorpusRoot, filepath.FromSlash(entry.StablePath)))
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
		artifact := closedLoopV6CaseArtifact{Schema: closedLoopV6CaseArtifactSchema, Version: closedLoopV6BaselineVersion, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, Replays: []ots.SynthesisRun{first, second}, Promotion: promotion, Observation: observation}
		artifact.Hash, err = hashClosedLoopV6CaseArtifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, artifact)
		t.Logf("V6 discovery baseline %s outcome=%s stop=%s root_gaps=%d", entry.ID, observation.Outcome, observation.StopReason, len(observation.Gaps))
	}
	if len(artifacts) != closedLoopV6RoleSize {
		t.Fatalf("V6 discovery baseline case count = %d, want %d", len(artifacts), closedLoopV6RoleSize)
	}
	return artifacts
}

func loadClosedLoopV6Manifest(t *testing.T) corpuspublication.Manifest {
	t.Helper()
	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6CorpusRoot, corpuspublication.ManifestFile)), &manifest)
	if manifest.Schema != corpuspublication.ManifestSchemaV6 || corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV6CorpusRoot, corpuspublication.ManifestFile))) != closedLoopV6CorpusManifestHash {
		t.Fatal("V6 corpus manifest is not frozen")
	}
	return manifest
}

func loadClosedLoopV6SelectionPolicy(t *testing.T) capabilitybundles.Policy {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V6_SELECTION_POLICY.json"))
	if corpusHash(data) != closedLoopV6SelectionPolicyHash {
		t.Fatal("V6 selection policy differs from its frozen commitment")
	}
	policy, err := capabilitybundles.DecodePolicy(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func closedLoopV6Policies(t *testing.T) (capabilityevaluation.ImpactRegistry, ots.Policy) {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	for name, want := range map[string]string{
		"V4_IMPACT_REGISTRY.json":       closedLoopV5ImpactRegistryFileHash,
		"V4_SYNTHESIS_POLICY.json":      closedLoopV5SynthesisPolicyFileHash,
		"V4_GAP_TRANSITION_POLICY.json": closedLoopV5GapPolicyFileHash,
		"V6_CONTRACT.sha256":            closedLoopV6ContractManifestHash,
	} {
		if got := corpusHash(mustCorpusRead(t, filepath.Join(specRoot, name))); got != want {
			t.Fatalf("V6 inherited commitment %s = %s, want %s", name, got, want)
		}
	}
	return closedLoopV4Policies(t)
}

func resolveClosedLoopV6PromotionEnvironment(t *testing.T, repositoryRoot string) closedLoopV5ResolvedPromotionEnvironment {
	t.Helper()
	resolved := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot)
	resolved.Public.Schema = "kicadai.closed-loop-open-set-promotion-environment.v6"
	resolved.Public.Version = closedLoopV6BaselineVersion
	var err error
	resolved.Public.Hash, err = hashClosedLoopV5PromotionEnvironment(resolved.Public)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func closedLoopV6BundleCases(report AggregateReport) []capabilitybundles.Case {
	cases := make([]capabilitybundles.Case, 0, len(report.Cases))
	for _, evidence := range report.Cases {
		gaps := make([]capabilitybundles.Gap, len(evidence.Gaps))
		for index, gap := range evidence.Gaps {
			gaps[index] = capabilitybundles.Gap{Stage: gap.Stage, Scope: string(gap.Scope), Capability: gap.Capability, Code: gap.Code, RequiredEvidence: append([]string(nil), gap.RequiredEvidence...)}
		}
		cases = append(cases, capabilitybundles.Case{Role: string(evidence.Case.Role), ID: evidence.Case.ID, ReportingDomain: string(evidence.Case.Domain), SafetyWeight: int64(safetyWeight(evidence.Case.SafetyImpact)), Outcome: string(evidence.Outcome), Gaps: gaps})
	}
	return cases
}

func buildClosedLoopV6GenericPlan(t *testing.T, selected capabilitybundles.Candidate) closedLoopV6GenericPlan {
	t.Helper()
	atomKeys := make([]string, len(selected.Atoms))
	for index := range selected.Atoms {
		atomKeys[index] = selected.Atoms[index].Key
	}
	memberKeys := make([]string, len(selected.Members))
	steps := make([]closedLoopV6PlanStep, len(selected.Members))
	for index, member := range selected.Members {
		memberKeys[index] = member.Key
		var memberEvidence []string
		prefix := member.Capability + ":"
		for _, evidence := range selected.RequiredEvidence {
			if strings.HasPrefix(evidence, prefix) {
				memberEvidence = append(memberEvidence, evidence)
			}
		}
		if len(memberEvidence) == 0 {
			t.Fatalf("V6 generic-plan member %s has no capability-specific evidence", member.Key)
		}
		steps[index] = closedLoopV6PlanStep{Order: index + 1, MemberKey: member.Key, Stage: member.Stage, Scope: member.Scope, Capability: member.Capability, Code: member.Code, RequiredEvidence: memberEvidence}
	}
	plan := closedLoopV6GenericPlan{Schema: closedLoopV6GenericPlanSchema, Version: closedLoopV6BaselineVersion, BundleKey: selected.Key, Admission: capabilitybundles.PlanEvidence{Executable: true, AtomKeys: atomKeys, MemberKeys: memberKeys}, RequiredEvidence: append([]string(nil), selected.RequiredEvidence...), Steps: steps}
	var err error
	plan.Hash, err = hashClosedLoopV6GenericPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func writeClosedLoopV6CaseArtifacts(root string, artifacts []closedLoopV6CaseArtifact) ([]closedLoopV6ArtifactRef, error) {
	discoveryRoot := filepath.Join(root, "discovery")
	if err := os.Mkdir(discoveryRoot, 0o755); err != nil {
		return nil, err
	}
	refs := make([]closedLoopV6ArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json.gz"))
		digest, err := writeClosedLoopV6CompressedArtifact(filepath.Join(root, filepath.FromSlash(path)), artifact)
		if err != nil {
			return nil, err
		}
		refs = append(refs, closedLoopV6ArtifactRef{CaseID: artifact.CaseID, Path: path, SHA256: digest})
	}
	return refs, nil
}

func writeClosedLoopV6CompressedArtifact(path string, artifact closedLoopV6CaseArtifact) (digest string, err error) {
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
		return "", fmt.Errorf("encode compressed V6 evidence %s: %w", artifact.CaseID, err)
	}
	if err := compressed.Close(); err != nil {
		return "", fmt.Errorf("close compressed V6 evidence %s: %w", artifact.CaseID, err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync compressed V6 evidence %s: %w", artifact.CaseID, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close V6 evidence file %s: %w", artifact.CaseID, err)
	}
	closed = true
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func assertClosedLoopV6CaseArtifacts(t *testing.T, report closedLoopV6BaselineReport) {
	t.Helper()
	if len(report.CaseArtifacts) != closedLoopV6RoleSize || report.Discovery.CaseCount != closedLoopV6RoleSize {
		t.Fatal("V6 discovery evidence has an invalid case count")
	}
	for index, ref := range report.CaseArtifacts {
		wantID := fmt.Sprintf("v6_case_%03d", index+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json.gz"))
		if ref.CaseID != wantID || ref.Path != wantPath || !closedLoopV6ValidHash(ref.SHA256) {
			t.Fatalf("V6 discovery evidence reference %d is invalid", index+1)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV6BaselineRoot, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V6 discovery evidence %s differs from its report commitment", ref.CaseID)
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("V6 discovery evidence %s is not a gzip stream", ref.CaseID)
		}
		if os.Getenv(closedLoopV6FullEvidenceVerifyEnv) != "1" {
			firstByte := []byte{0}
			read, readErr := reader.Read(firstByte)
			closeErr := reader.Close()
			if read != 1 || readErr != nil || closeErr != nil || firstByte[0] != '{' {
				t.Fatalf("V6 discovery evidence %s has an invalid canonical JSON stream", ref.CaseID)
			}
			continue
		}
		var artifact closedLoopV6CaseArtifact
		decoder := json.NewDecoder(reader)
		decodeErr := decoder.Decode(&artifact)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		closeErr := reader.Close()
		expected, hashErr := hashClosedLoopV6CaseArtifact(artifact)
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || hashErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV6CaseArtifactSchema || artifact.Version != closedLoopV6BaselineVersion || artifact.CaseID != wantID || len(artifact.Replays) != 2 {
			t.Fatalf("V6 discovery evidence %s is structurally invalid", ref.CaseID)
		}
		first, firstErr := json.Marshal(artifact.Replays[0])
		second, secondErr := json.Marshal(artifact.Replays[1])
		if firstErr != nil || secondErr != nil || !bytes.Equal(first, second) || artifact.Observation.SynthesisHash != artifact.Replays[0].Hash {
			t.Fatalf("V6 discovery evidence %s lacks byte-identical complete synthesis replay", ref.CaseID)
		}
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != ots.PhysicalPromotionPassed || !artifact.Promotion.ReplayIdentical || len(artifact.Promotion.Runs) != 2 || artifact.Promotion.Hash != artifact.Observation.PromotionHash || artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				t.Fatalf("V6 passing discovery case %s lacks complete installed-KiCad promotion evidence", ref.CaseID)
			}
		} else if artifact.Promotion != nil {
			t.Fatalf("V6 nonpassing discovery case %s contains promotion evidence", ref.CaseID)
		}
	}
}

func buildClosedLoopV6BaselineReport(t *testing.T, infrastructureCommit string, promotionEnvironment closedLoopV5PromotionEnvironment, discovery AggregateReport, refs []closedLoopV6ArtifactRef) closedLoopV6BaselineReport {
	t.Helper()
	inventoryHash, catalogHash, modelRegistryHash, synthesisPolicyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
	report := closedLoopV6BaselineReport{Schema: closedLoopV6BaselineSchema, Version: closedLoopV6BaselineVersion, CorpusManifestSHA256: closedLoopV6CorpusManifestHash, CorpusFreezeCommit: closedLoopV6CorpusFreezeCommit, InfrastructureCommit: infrastructureCommit, ContractManifestSHA256: closedLoopV6ContractManifestHash, EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistryFileSHA256: closedLoopV5ImpactRegistryFileHash, SynthesisPolicyFileSHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicyFileSHA256: closedLoopV5GapPolicyFileHash, SelectionPolicySHA256: closedLoopV6SelectionPolicyHash, InventorySHA256: inventoryHash, CatalogSHA256: catalogHash, ModelRegistrySHA256: modelRegistryHash, SynthesisPolicySHA256: synthesisPolicyHash, PromotionEnvironment: promotionEnvironment, OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs, Discovery: discovery}
	var err error
	report.Hash, err = hashClosedLoopV6BaselineReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func buildClosedLoopV6Ranking(t *testing.T, baselineHash string, decisions capabilitybundles.Result) closedLoopV6Ranking {
	t.Helper()
	ranking := closedLoopV6Ranking{Schema: closedLoopV6RankingSchema, Version: closedLoopV6BaselineVersion, BaselineSHA256: baselineHash, SelectionPolicySHA256: closedLoopV6SelectionPolicyHash, Decisions: decisions}
	var err error
	ranking.Hash, err = hashClosedLoopV6Ranking(ranking)
	if err != nil {
		t.Fatal(err)
	}
	return ranking
}

func buildClosedLoopV6Selection(t *testing.T, infrastructureCommit, baselineHash, rankingHash string, selected capabilitybundles.Candidate, planHash string) closedLoopV6Selection {
	t.Helper()
	selection := closedLoopV6Selection{Schema: closedLoopV6SelectionSchema, Version: closedLoopV6BaselineVersion, StartingCommit: "9b6f8be61006f7de179099feb0b38080ff18ecb3", ContractFreezeCommit: "0d0350f4542a6f7f97b813331d228cac969767cd", CorpusFreezeCommit: closedLoopV6CorpusFreezeCommit, InfrastructureCommit: infrastructureCommit, SelectionFreezeParentCommit: infrastructureCommit, CorpusManifestSHA256: closedLoopV6CorpusManifestHash, BaselineSHA256: baselineHash, RankingSHA256: rankingHash, SelectionPolicySHA256: closedLoopV6SelectionPolicyHash, Selected: selected, GenericPlanSHA256: planHash}
	var err error
	selection.Hash, err = hashClosedLoopV6Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func hashClosedLoopV6CaseArtifact(value closedLoopV6CaseArtifact) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV6BaselineReport(value closedLoopV6BaselineReport) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV6Ranking(value closedLoopV6Ranking) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV6GenericPlan(value closedLoopV6GenericPlan) (string, error) {
	value.Hash = ""
	return digest(value)
}
func hashClosedLoopV6Selection(value closedLoopV6Selection) (string, error) {
	value.Hash = ""
	return digest(value)
}

func closedLoopV6BaselineAudit(report closedLoopV6BaselineReport, ranking closedLoopV6Ranking, selection closedLoopV6Selection) []byte {
	var counts []string
	for _, count := range report.OutcomeCounts {
		if count.Role == RoleDiscovery && count.Domain == "" {
			counts = append(counts, fmt.Sprintf("pass=%d unsupported=%d unsafe=%d exhausted=%d", count.Pass, count.Unsupported, count.Unsafe, count.Exhausted))
		}
	}
	eligible := 0
	for _, candidate := range ranking.Decisions.Candidates {
		if candidate.Eligible {
			eligible++
		}
	}
	return []byte(fmt.Sprintf("# V6 Discovery Baseline Audit\n\nThe public discovery baseline used 18 frozen cases, two byte-identical complete synthesis exports per case, and two clean-root installed-KiCad promotions for every pass. Selection consumed only complete normalized discovery root gaps; no held-out source, outcome, gap, diagnostic, or bundle membership was opened.\n\n- outcomes: %s\n- candidate decisions: %d\n- eligible bundles: %d\n- selected bundle key: `%s`\n- claimed discovery unlocks: %d\n- reporting domains: %d\n- capability atoms: %d\n- exact root members: %d\n- baseline hash: `%s`\n- ranking hash: `%s`\n- selection hash: `%s`\n", strings.Join(counts, ", "), len(ranking.Decisions.Candidates), eligible, selection.Selected.Key, len(selection.Selected.UnlockedCases), len(selection.Selected.ReportingDomains), len(selection.Selected.Atoms), len(selection.Selected.Members), report.Hash, ranking.Hash, selection.Hash))
}

func assertClosedLoopV6BaselineFileSet(t *testing.T) {
	t.Helper()
	want := map[string]bool{corpuspublication.ChecksumFile: true, "BASELINE_AUDIT.md": true, "report.json": true, "bundle_ranking.json": true, "generic_plan.json": true, "selection.json": true}
	for index := 1; index <= closedLoopV6RoleSize; index++ {
		want[fmt.Sprintf("discovery/v6_case_%03d.json.gz", index)] = true
	}
	var got []string
	if err := filepath.WalkDir(closedLoopV6BaselineRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV6BaselineRoot, path)
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
		t.Fatalf("V6 baseline file count = %d, want %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] || strings.Contains(path, "held_out") {
			t.Fatalf("V6 baseline contains forbidden file %s", path)
		}
	}
}

func TestClosedLoopV6RankingIgnoresHeldOutByConstruction(t *testing.T) {
	policy := loadClosedLoopV6SelectionPolicy(t)
	result, err := capabilitybundles.Build([]capabilitybundles.Case{{Role: string(RoleHeldOut), ID: "hidden", ReportingDomain: "power", SafetyWeight: 5, Outcome: string(OutcomeUnsupported), Gaps: []capabilitybundles.Gap{{Stage: "simulation", Scope: "simulation", Capability: "hidden", Code: "hidden"}}}}, policy)
	if err == nil || len(result.Candidates) != 0 {
		t.Fatalf("V6 held-out evidence did not fail closed: %v %#v", err, result)
	}
}

func TestClosedLoopV6SelectionHashRejectsMutation(t *testing.T) {
	selection := closedLoopV6Selection{Schema: closedLoopV6SelectionSchema, Version: closedLoopV6BaselineVersion, CorpusFreezeCommit: closedLoopV6CorpusFreezeCommit}
	hash, err := hashClosedLoopV6Selection(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.CorpusFreezeCommit = "0000000000000000000000000000000000000000"
	mutated, err := hashClosedLoopV6Selection(selection)
	if err != nil || mutated == hash {
		t.Fatal("V6 selection hash did not bind mutation")
	}
}

func TestClosedLoopV6CompressedEvidenceIsDeterministic(t *testing.T) {
	artifact := closedLoopV6CaseArtifact{Schema: closedLoopV6CaseArtifactSchema, Version: closedLoopV6BaselineVersion, CaseID: "v6_case_001", RequirementSHA256: strings.Repeat("a", 64)}
	firstRoot, secondRoot := t.TempDir(), t.TempDir()
	firstRefs, err := writeClosedLoopV6CaseArtifacts(firstRoot, []closedLoopV6CaseArtifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	secondRefs, err := writeClosedLoopV6CaseArtifacts(secondRoot, []closedLoopV6CaseArtifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(corpusJSON(t, firstRefs), corpusJSON(t, secondRefs)) {
		t.Fatal("V6 compressed evidence references are nondeterministic")
	}
	first := mustCorpusRead(t, filepath.Join(firstRoot, filepath.FromSlash(firstRefs[0].Path)))
	second := mustCorpusRead(t, filepath.Join(secondRoot, filepath.FromSlash(secondRefs[0].Path)))
	if !bytes.Equal(first, second) {
		t.Fatal("V6 compressed evidence bytes are nondeterministic")
	}
}
