package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/blindbaseline"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/externalkey"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV7HeldOutPayloadSchema  = "kicadai.closed-loop-open-set-held-out-baseline-payload.v7"
	closedLoopV7HeldOutCaseSchema     = "kicadai.closed-loop-open-set-held-out-baseline-case.v7"
	closedLoopV7HeldOutBaselineRoot   = "testdata/closed_loop_open_set_v7_held_out_baseline"
	closedLoopV7HeldOutBaselineUpdate = "UPDATE_CLOSED_LOOP_V7_HELD_OUT_BASELINE"
	closedLoopV7HeldOutSourceKeyEnv   = "KICADAI_V7_HELD_OUT_SOURCE_KEY_FILE"
	closedLoopV7HeldOutBaselineKeyEnv = "KICADAI_V7_HELD_OUT_BASELINE_KEY_FILE"
	closedLoopV7SelectionFreezeCommit = "a4d0935071b89a99db88cd5a823ccc4f32ba0b8f"

	// These empty sentinels are valid only while the no-replace artifact root is
	// absent: the freeze test skips before consulting them. They are populated
	// exactly once after this outcome-neutral harness is committed and the
	// custodian publication succeeds; any artifact appearing first fails closed.
	closedLoopV7HeldOutPublisherCommit = "294ed4eb1bee64e69c41ddeb0d6e2b4517f3f2b3"
	// The semantic commitment authenticates manifest fields in the V7 wire
	// protocol; the file hash separately freezes the canonical JSON bytes.
	closedLoopV7HeldOutManifestCommitment = "b830b34b5b41dc4c7af71740e6d46349e28400e932583a63398c9d3de334045a"
	closedLoopV7HeldOutManifestFileHash   = "3225a2b4645cfc2b16e6260e033631fcbf69307e724f4fbdff7cb8f28ae5dc2c"
)

type closedLoopV7HeldOutPayload struct {
	Schema               string                            `json:"schema"`
	Version              int                               `json:"version"`
	Binding              blindbaseline.V7Binding           `json:"binding"`
	PromotionEnvironment closedLoopV5PromotionEnvironment  `json:"promotion_environment"`
	Cases                []closedLoopV7HeldOutCaseArtifact `json:"cases"`
	Aggregate            AggregateReport                   `json:"aggregate"`
	Hash                 string                            `json:"hash"`
}

type closedLoopV7HeldOutCaseArtifact struct {
	Schema                 string                      `json:"schema"`
	Version                int                         `json:"version"`
	CaseID                 string                      `json:"case_id"`
	RequirementSHA256      string                      `json:"requirement_sha256"`
	NormalizedReplaySHA256 []string                    `json:"normalized_replay_sha256"`
	SynthesisSHA256        string                      `json:"synthesis_sha256"`
	Promotion              *closedLoopV5PromotionProof `json:"promotion,omitempty"`
	Observation            CaseEvidence                `json:"observation"`
	Hash                   string                      `json:"hash"`
}

func TestClosedLoopV7HeldOutBaselineSealIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV7HeldOutBaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V7 held-out baseline has not been sealed")
		}
		t.Fatal(err)
	}
	if closedLoopV7HeldOutPublisherCommit == "" || closedLoopV7HeldOutManifestCommitment == "" {
		t.Fatal("V7 held-out baseline exists without literal publisher and manifest commitments")
	}
	manifest, err := blindbaseline.VerifyV7(closedLoopV7HeldOutBaselineRoot)
	if err != nil {
		t.Fatalf("verify V7 held-out baseline seal: %v", err)
	}
	if got := corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV7HeldOutBaselineRoot, blindbaseline.ManifestFile))); got != closedLoopV7HeldOutManifestFileHash {
		t.Fatalf("V7 held-out baseline manifest file hash = %s, want %s", got, closedLoopV7HeldOutManifestFileHash)
	}
	corpus := loadClosedLoopV7Manifest(t)
	discovery := loadClosedLoopV7FrozenBaselineReport(t)
	selection := loadClosedLoopV7FrozenSelection(t)
	promotion := discovery.PromotionEnvironment
	binding := manifest.Binding
	if manifest.Hash != closedLoopV7HeldOutManifestCommitment || manifest.CaseCount != closedLoopV7RoleSize ||
		binding.StartingCommit != selection.StartingCommit || binding.ContractFreezeCommit != selection.ContractFreezeCommit ||
		binding.CorpusFreezeCommit != closedLoopV7CorpusFreezeCommit || binding.SelectionFreezeCommit != closedLoopV7SelectionFreezeCommit ||
		binding.PublisherParentCommit != closedLoopV7HeldOutPublisherCommit || binding.CorpusManifestSHA256 != closedLoopV7CorpusManifestHash ||
		binding.ContractManifestSHA256 != corpus.ContractManifestSHA256 || binding.ValidatorManifestSHA256 != corpus.ValidatorManifestSHA256 ||
		binding.PublisherManifestSHA256 != corpus.PublisherManifestSHA256 || binding.ValidationReportSHA256 != corpus.ValidationReportSHA256 ||
		binding.PacketSetSHA256 != corpus.PacketSetSHA256 || binding.ContractBindingSHA256 != corpus.ContractBindingSHA256 ||
		binding.HistoricalCommitmentsSHA256 != corpus.HistoricalCommitmentsSHA256 || binding.SourceCiphertextSHA256 != corpus.HeldOutSource.CiphertextSHA256 ||
		binding.DiscoveryBaselineSHA256 != selection.BaselineSHA256 || binding.FrontierSHA256 != selection.InputFrontierSHA256 || binding.RankingSHA256 != selection.RankingSHA256 ||
		binding.SelectionSHA256 != selection.Hash || binding.GenericPlanSHA256 != selection.GenericPlanSHA256 ||
		binding.EvaluatorPolicy != RealizabilityPolicyVersion || binding.ImpactRegistrySHA256 != closedLoopV5ImpactRegistryFileHash ||
		binding.SynthesisPolicySHA256 != closedLoopV5SynthesisPolicyFileHash || binding.GapPolicySHA256 != closedLoopV5GapPolicyFileHash ||
		binding.SelectionPolicySHA256 != closedLoopV7SelectionPolicyHash || binding.InventorySHA256 != discovery.InventorySHA256 ||
		binding.CatalogSHA256 != discovery.CatalogSHA256 || binding.ModelRegistrySHA256 != discovery.ModelRegistrySHA256 ||
		binding.EnvironmentPolicySHA256 != discovery.SynthesisPolicySHA256 || binding.PromotionPlatform != promotion.Platform ||
		binding.KiCadVersion != promotion.KiCadVersion || binding.PromotionToolchainSHA256 != promotion.Hash ||
		binding.PromotionToolchainLockSHA256 != promotion.ToolchainLockSHA256 || binding.KiCadCLISHA256 != promotion.KiCadCLISHA256 ||
		binding.SymbolTableSHA256 != promotion.SymbolTableSHA256 || binding.FootprintTableSHA256 != promotion.FootprintTableSHA256 ||
		binding.SymbolsSHA256 != promotion.SymbolsSHA256 || binding.FootprintsSHA256 != promotion.FootprintsSHA256 {
		t.Fatal("V7 held-out baseline public commitments drifted")
	}
}

func TestUpdateClosedLoopV7HeldOutBaseline(t *testing.T) {
	if os.Getenv(closedLoopV7HeldOutBaselineUpdate) != "1" {
		t.Skip("set " + closedLoopV7HeldOutBaselineUpdate + "=1 in the isolated V7 custodian context")
	}
	sourceKeyPath := os.Getenv(closedLoopV7HeldOutSourceKeyEnv)
	baselineKeyPath := os.Getenv(closedLoopV7HeldOutBaselineKeyEnv)
	if sourceKeyPath == "" || baselineKeyPath == "" {
		t.Fatal("both external V7 held-out key paths are required")
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	if err := externalkey.Distinct(repositoryRoot, sourceKeyPath, baselineKeyPath); err != nil {
		t.Fatal("V7 held-out key paths failed the distinct external-path gate")
	}
	destinationRoot := filepath.Join(repositoryRoot, "internal", "capabilityfeedback", closedLoopV7HeldOutBaselineRoot)
	for _, path := range []string{baselineKeyPath, destinationRoot} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("V7 held-out baseline updater is retired or its destination is unavailable")
		}
	}
	publisherParent := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	assertClosedLoopV7PreimplementationBoundary(t, repositoryRoot, publisherParent)

	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ManifestFile))
	if corpusHash(manifestBytes) != closedLoopV7CorpusManifestHash {
		t.Fatal("V7 corpus manifest differs from its frozen commitment")
	}
	manifest := loadClosedLoopV7Manifest(t)
	selection := loadClosedLoopV7FrozenSelection(t)
	discoveryBaseline := loadClosedLoopV7FrozenBaselineReport(t)
	registry, policy := closedLoopV7Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV7PromotionEnvironment(t, repositoryRoot)

	sourceKey, err := externalkey.Read(repositoryRoot, sourceKeyPath)
	if err != nil {
		t.Fatal("read external V7 held-out source key")
	}
	defer zeroClosedLoopV5Secret(sourceKey)
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, corpuspublication.HeldOutCipherFile))
	sealedCases, err := corpuspublication.OpenHeldOutV7(sourceKey, manifest, ciphertext)
	if err != nil {
		t.Fatal("authenticate and open V7 held-out source")
	}
	assertClosedLoopV7HeldOutSources(t, manifest, sealedCases)

	artifacts := runClosedLoopV7HeldOutBaseline(t, sealedCases, policy, inventory, environment, promotionEnvironment)
	if after := resolveClosedLoopV7PromotionEnvironment(t, repositoryRoot); after.Public != promotionEnvironment.Public {
		t.Fatal("locked V7 installed-KiCad promotion environment changed during the held-out baseline")
	}
	evidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		evidence[index] = artifacts[index].Observation
	}
	aggregate, err := EvaluateRealizabilityAware(RoleHeldOut, evidence, registry)
	if err != nil {
		t.Fatal("aggregate sealed V7 held-out baseline")
	}
	inventoryHash, catalogHash, modelRegistryHash, environmentPolicyHash := closedLoopV5SealedEnvironmentBindings(t, evidence)
	if inventoryHash != discoveryBaseline.InventorySHA256 || catalogHash != discoveryBaseline.CatalogSHA256 || modelRegistryHash != discoveryBaseline.ModelRegistrySHA256 || environmentPolicyHash != discoveryBaseline.SynthesisPolicySHA256 {
		t.Fatal("sealed V7 held-out baseline did not use the frozen discovery synthesis environment")
	}
	binding := closedLoopV7BaselineBinding(manifest, selection, discoveryBaseline, promotionEnvironment.Public, publisherParent)
	payload := closedLoopV7HeldOutPayload{Schema: closedLoopV7HeldOutPayloadSchema, Version: closedLoopV7BaselineVersion, Binding: binding, PromotionEnvironment: promotionEnvironment.Public, Cases: artifacts, Aggregate: aggregate}
	payload.Hash, err = hashClosedLoopV7HeldOutPayload(payload)
	if err != nil {
		t.Fatal("hash sealed V7 held-out baseline")
	}
	result, err := blindbaseline.PublishV7(blindbaseline.V7Request{
		RepositoryRoot: repositoryRoot, DestinationRoot: destinationRoot, KeyPath: baselineKeyPath,
		ReservedKeyPaths: []string{sourceKeyPath}, Binding: binding, Payload: corpusJSON(t, payload), CaseCount: len(artifacts),
	})
	if err != nil {
		t.Fatal("atomically publish encrypted V7 held-out baseline")
	}
	if result.Manifest.CaseCount != closedLoopV7RoleSize {
		t.Fatal("published V7 held-out baseline count is invalid")
	}
	t.Log("sealed 18 V7 held-out baseline cases; no outcome, gap, diagnostic, membership, or promotion detail disclosed")
}

func loadClosedLoopV7FrozenSelection(t *testing.T) closedLoopV7Selection {
	t.Helper()
	var selection closedLoopV7Selection
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "selection.json")), &selection)
	want, err := hashClosedLoopV7Selection(selection)
	if err != nil || selection.Hash != want || selection.Hash != closedLoopV7SelectionHash || selection.CorpusManifestSHA256 != closedLoopV7CorpusManifestHash || selection.CorpusFreezeCommit != closedLoopV7CorpusFreezeCommit || selection.BaselineSHA256 != closedLoopV7BaselineHash || selection.InputFrontierSHA256 != closedLoopV7FrontierHash || selection.RankingSHA256 != closedLoopV7RankingHash || selection.SelectionPolicySHA256 != closedLoopV7SelectionPolicyHash || selection.GenericPlanSHA256 != closedLoopV7GenericPlanHash || selection.Generation != 0 {
		t.Fatal("V7 rank-one selection is not the frozen committed selection")
	}
	return selection
}

func loadClosedLoopV7FrozenBaselineReport(t *testing.T) closedLoopV7BaselineReport {
	t.Helper()
	var report closedLoopV7BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "report.json")), &report)
	want, err := hashClosedLoopV7BaselineReport(report)
	if err != nil || report.Hash != want || report.Hash != closedLoopV7BaselineHash || report.FrontierSHA256 != closedLoopV7FrontierHash || report.CorpusManifestSHA256 != closedLoopV7CorpusManifestHash || report.CorpusFreezeCommit != closedLoopV7CorpusFreezeCommit || report.EvaluatorPolicy != RealizabilityPolicyVersion || report.ImpactRegistryFileSHA256 != closedLoopV5ImpactRegistryFileHash || report.SynthesisPolicyFileSHA256 != closedLoopV5SynthesisPolicyFileHash || report.GapPolicyFileSHA256 != closedLoopV5GapPolicyFileHash || report.SelectionPolicySHA256 != closedLoopV7SelectionPolicyHash {
		t.Fatal("V7 discovery baseline report is not the frozen committed report")
	}
	return report
}

func assertClosedLoopV7PreimplementationBoundary(t *testing.T, repositoryRoot, publisherParent string) {
	t.Helper()
	if publisherParent == closedLoopV7SelectionFreezeCommit {
		t.Fatal("V7 held-out baseline requires a separately committed outcome-neutral custodian harness")
	}
	ancestor := exec.Command("git", "-C", repositoryRoot, "merge-base", "--is-ancestor", closedLoopV7SelectionFreezeCommit, publisherParent)
	if err := ancestor.Run(); err != nil {
		t.Fatal("V7 selection freeze is not an ancestor of the baseline publisher")
	}
	output, err := exec.Command("git", "-C", repositoryRoot, "diff", "--name-only", closedLoopV7SelectionFreezeCommit+".."+publisherParent).CombinedOutput()
	if err != nil {
		t.Fatal("inspect V7 preimplementation boundary")
	}
	allowed := map[string]bool{
		"internal/blindbaseline/v7_model.go": true, "internal/blindbaseline/v7_publish.go": true,
		"internal/blindbaseline/v7_seal.go": true, "internal/blindbaseline/v7_test.go": true,
		"internal/blindbaseline/v7_verify.go": true, "internal/capabilityfeedback/v7_heldout_baseline_test.go": true,
	}
	seen := map[string]bool{}
	for _, path := range strings.Fields(string(output)) {
		if !allowed[path] || seen[path] {
			t.Fatal("V7 outcome-affecting bytes changed before the held-out baseline")
		}
		seen[path] = true
	}
	if len(seen) != len(allowed) {
		t.Fatal("V7 outcome-neutral custodian harness is incomplete")
	}
}

func assertClosedLoopV7HeldOutSources(t *testing.T, manifest corpuspublication.Manifest, sealed []corpuspublication.HeldOutCase) {
	t.Helper()
	want := make([]corpuspublication.Entry, 0, closedLoopV7RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role == string(RoleHeldOut) {
			want = append(want, entry)
		}
	}
	if len(want) != closedLoopV7RoleSize || len(sealed) != len(want) {
		t.Fatal("sealed V7 held-out source has an invalid case set")
	}
	for index := range want {
		if sealed[index].Entry != want[index] || corpusHash(sealed[index].Source) != want[index].RequirementSHA256 {
			t.Fatal("sealed V7 held-out source differs from immutable manifest order or identity")
		}
	}
}

func runClosedLoopV7HeldOutBaseline(t *testing.T, sealed []corpuspublication.HeldOutCase, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) []closedLoopV7HeldOutCaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV7HeldOutCaseArtifact, 0, len(sealed))
	for index := range sealed {
		requirement, issues := ots.DecodeStrict(bytes.NewReader(sealed[index].Source))
		if len(issues) != 0 {
			t.Fatal("sealed V7 held-out requirement failed strict decode")
		}
		sealed[index].Source = nil
		first := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatal("sealed V7 held-out synthesis replay failed closed")
		}
		var promotion *ots.PhysicalPromotionResult
		var proof *closedLoopV5PromotionProof
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopV5SealedRun(t, first, environment, promotionEnvironment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatal("sealed V7 held-out physical promotion failed closed")
			}
			promotion = &current
			proof = &closedLoopV5PromotionProof{Schema: current.Schema, Version: current.Version, Status: current.Status, ReplayIdentical: current.ReplayIdentical, ProjectHash: current.ProjectHash, PromotionHash: current.Hash, RunCount: len(current.Runs)}
		}
		entry := sealed[index].Entry
		observation, err := ObserveRealizabilityAware(CaseMeta{ID: entry.ID, Role: RoleHeldOut, Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact)}, requirement, first, promotion)
		if err != nil {
			t.Fatal("sealed V7 held-out observation failed closed")
		}
		artifact := closedLoopV7HeldOutCaseArtifact{Schema: closedLoopV7HeldOutCaseSchema, Version: closedLoopV7BaselineVersion, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, NormalizedReplaySHA256: []string{corpusHash(firstBytes), corpusHash(secondBytes)}, SynthesisSHA256: first.Hash, Promotion: proof, Observation: observation}
		artifact.Hash, err = hashClosedLoopV7HeldOutCaseArtifact(artifact)
		if err != nil {
			t.Fatal("hash sealed V7 held-out case")
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) != closedLoopV7RoleSize {
		t.Fatal("sealed V7 held-out baseline case count is invalid")
	}
	return artifacts
}

func closedLoopV7BaselineBinding(manifest corpuspublication.Manifest, selection closedLoopV7Selection, discovery closedLoopV7BaselineReport, promotion closedLoopV5PromotionEnvironment, publisherParent string) blindbaseline.V7Binding {
	return blindbaseline.V7Binding{
		StartingCommit: selection.StartingCommit, ContractFreezeCommit: selection.ContractFreezeCommit, CorpusFreezeCommit: selection.CorpusFreezeCommit, SelectionFreezeCommit: closedLoopV7SelectionFreezeCommit, PublisherParentCommit: publisherParent,
		CorpusManifestSHA256: closedLoopV7CorpusManifestHash, ContractManifestSHA256: manifest.ContractManifestSHA256, ValidatorManifestSHA256: manifest.ValidatorManifestSHA256, PublisherManifestSHA256: manifest.PublisherManifestSHA256, ValidationReportSHA256: manifest.ValidationReportSHA256, PacketSetSHA256: manifest.PacketSetSHA256, ContractBindingSHA256: manifest.ContractBindingSHA256, HistoricalCommitmentsSHA256: manifest.HistoricalCommitmentsSHA256, SourceCiphertextSHA256: manifest.HeldOutSource.CiphertextSHA256,
		DiscoveryBaselineSHA256: selection.BaselineSHA256, FrontierSHA256: selection.InputFrontierSHA256, RankingSHA256: selection.RankingSHA256, SelectionSHA256: selection.Hash, GenericPlanSHA256: selection.GenericPlanSHA256, EvaluatorPolicy: RealizabilityPolicyVersion,
		ImpactRegistrySHA256: closedLoopV5ImpactRegistryFileHash, SynthesisPolicySHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicySHA256: closedLoopV5GapPolicyFileHash, SelectionPolicySHA256: selection.SelectionPolicySHA256,
		InventorySHA256: discovery.InventorySHA256, CatalogSHA256: discovery.CatalogSHA256, ModelRegistrySHA256: discovery.ModelRegistrySHA256, EnvironmentPolicySHA256: discovery.SynthesisPolicySHA256,
		PromotionPlatform: promotion.Platform, KiCadVersion: promotion.KiCadVersion, PromotionToolchainSHA256: promotion.Hash, PromotionToolchainLockSHA256: promotion.ToolchainLockSHA256, KiCadCLISHA256: promotion.KiCadCLISHA256, SymbolTableSHA256: promotion.SymbolTableSHA256, FootprintTableSHA256: promotion.FootprintTableSHA256, SymbolsSHA256: promotion.SymbolsSHA256, FootprintsSHA256: promotion.FootprintsSHA256,
	}
}

func hashClosedLoopV7HeldOutCaseArtifact(artifact closedLoopV7HeldOutCaseArtifact) (string, error) {
	artifact.Hash = ""
	return digest(artifact)
}

func hashClosedLoopV7HeldOutPayload(payload closedLoopV7HeldOutPayload) (string, error) {
	payload.Hash = ""
	return digest(payload)
}

func TestClosedLoopV7HeldOutPayloadHashRejectsMutation(t *testing.T) {
	payload := closedLoopV7HeldOutPayload{Schema: closedLoopV7HeldOutPayloadSchema, Version: closedLoopV7BaselineVersion, Binding: blindbaseline.V7Binding{EvaluatorPolicy: RealizabilityPolicyVersion}}
	first, err := hashClosedLoopV7HeldOutPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Version++
	second, err := hashClosedLoopV7HeldOutPayload(payload)
	if err != nil || first == second {
		t.Fatal("V7 held-out payload mutation did not change its commitment")
	}
}
