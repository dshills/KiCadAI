package capabilityfeedback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kicadai/internal/blindbaseline"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/externalkey"
	"kicadai/internal/libraryresolver"
	ots "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/promotiontoolchain"
	"kicadai/internal/reports"
)

const (
	closedLoopV5HeldOutPayloadSchema  = "kicadai.closed-loop-open-set-held-out-baseline-payload.v5"
	closedLoopV5HeldOutCaseSchema     = "kicadai.closed-loop-open-set-held-out-baseline-case.v5"
	closedLoopV5HeldOutBaselineRoot   = "testdata/closed_loop_open_set_v5_held_out_baseline"
	closedLoopV5HeldOutBaselineUpdate = "UPDATE_CLOSED_LOOP_V5_HELD_OUT_BASELINE"
	closedLoopV5HeldOutSourceKeyEnv   = "KICADAI_V5_HELD_OUT_SOURCE_KEY_FILE"
	closedLoopV5HeldOutBaselineKeyEnv = "KICADAI_V5_HELD_OUT_BASELINE_KEY_FILE"
	closedLoopV5PromotionVerifyEnv    = "VERIFY_CLOSED_LOOP_V5_PROMOTION_ENVIRONMENT"
	closedLoopV5SelectionFreezeCommit = "ffcff3881ca2e03a454fd350124637664bf4d4e4"
	closedLoopV5MaximumKiCadCLIBytes  = 1 << 30
	// The artifact-free harness commit intentionally leaves this empty. The
	// verifier skips while the no-replace bundle is absent and fails closed if
	// any bundle appears before its exact manifest hash is frozen.
	closedLoopV5HeldOutManifestHash = ""
)

type closedLoopV5HeldOutPayload struct {
	Schema               string                           `json:"schema"`
	Version              int                              `json:"version"`
	Binding              blindbaseline.Binding            `json:"binding"`
	PromotionEnvironment closedLoopV5PromotionEnvironment `json:"promotion_environment"`
	Cases                []closedLoopV5CaseArtifact       `json:"cases"`
	Aggregate            AggregateReport                  `json:"aggregate"`
	Hash                 string                           `json:"hash"`
}

type closedLoopV5PromotionEnvironment struct {
	Schema               string `json:"schema"`
	Version              int    `json:"version"`
	Platform             string `json:"platform"`
	KiCadVersion         string `json:"kicad_version"`
	ToolchainLockSHA256  string `json:"toolchain_lock_sha256"`
	KiCadCLISHA256       string `json:"kicad_cli_sha256"`
	SymbolTableSHA256    string `json:"symbol_table_sha256"`
	FootprintTableSHA256 string `json:"footprint_table_sha256"`
	SymbolsSHA256        string `json:"symbols_sha256"`
	SymbolsFileCount     int    `json:"symbols_file_count"`
	SymbolsByteCount     int64  `json:"symbols_byte_count"`
	FootprintsSHA256     string `json:"footprints_sha256"`
	FootprintsFileCount  int    `json:"footprints_file_count"`
	FootprintsByteCount  int64  `json:"footprints_byte_count"`
	Hash                 string `json:"hash"`
}

type closedLoopV5ResolvedPromotionEnvironment struct {
	Evidence promotiontoolchain.Evidence
	Public   closedLoopV5PromotionEnvironment
}

func TestClosedLoopV5HeldOutBaselineSealIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV5HeldOutBaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V5 held-out baseline has not been sealed")
		}
		t.Fatal(err)
	}
	if closedLoopV5HeldOutManifestHash == "" {
		t.Fatal("V5 held-out baseline exists without a frozen manifest commitment")
	}
	manifest, err := blindbaseline.Verify(closedLoopV5HeldOutBaselineRoot)
	if err != nil {
		t.Fatalf("verify V5 held-out baseline seal: %v", err)
	}
	corpus := loadClosedLoopV5Manifest(t)
	discovery := loadClosedLoopV5FrozenBaselineReport(t)
	binding := manifest.Binding
	if manifest.Hash != closedLoopV5HeldOutManifestHash || manifest.CaseCount != closedLoopV5RoleSize ||
		binding.StartingCommit != "d8e98b4dee3212823525c5955e8e025bd0039d03" ||
		binding.ContractFreezeCommit != "a9249879d5e02575fe047925d613458ffec62030" ||
		binding.CorpusFreezeCommit != closedLoopV5CorpusFreezeCommit ||
		binding.SelectionFreezeCommit != closedLoopV5SelectionFreezeCommit ||
		binding.CorpusManifestSHA256 != closedLoopV5CorpusManifestHash ||
		binding.ContractManifestSHA256 != corpus.ContractManifestSHA256 ||
		binding.ValidatorManifestSHA256 != corpus.ValidatorManifestSHA256 ||
		binding.PublisherManifestSHA256 != corpus.PublisherManifestSHA256 ||
		binding.ValidationReportSHA256 != corpus.ValidationReportSHA256 ||
		binding.PacketSetSHA256 != corpus.PacketSetSHA256 ||
		binding.ContractBindingSHA256 != corpus.ContractBindingSHA256 ||
		binding.HistoricalCommitmentsSHA256 != corpus.HistoricalCommitmentsSHA256 ||
		binding.SourceCiphertextSHA256 != corpus.HeldOutSource.CiphertextSHA256 ||
		binding.SelectionSHA256 != closedLoopV5SelectionHash ||
		binding.DiscoveryBaselineSHA256 != closedLoopV5BaselineHash ||
		binding.RankingSHA256 != closedLoopV5RankingHash ||
		binding.GenericPlanSHA256 != closedLoopV5GenericPlanHash ||
		binding.EvaluatorPolicy != RealizabilityPolicyVersion ||
		binding.ImpactRegistrySHA256 != closedLoopV5ImpactRegistryFileHash ||
		binding.SynthesisPolicySHA256 != closedLoopV5SynthesisPolicyFileHash ||
		binding.GapPolicySHA256 != closedLoopV5GapPolicyFileHash ||
		binding.SelectionPolicySHA256 != closedLoopV5SelectionPolicyHash ||
		binding.ImplementationManifestSHA256 != closedLoopV5ImplementationManifestHash ||
		binding.InventorySHA256 != discovery.InventorySHA256 || binding.CatalogSHA256 != discovery.CatalogSHA256 ||
		binding.ModelRegistrySHA256 != discovery.ModelRegistrySHA256 || binding.EnvironmentPolicySHA256 != discovery.SynthesisPolicySHA256 {
		t.Fatal("V5 held-out baseline public commitments drifted")
	}
}

func TestUpdateClosedLoopV5HeldOutBaseline(t *testing.T) {
	if os.Getenv(closedLoopV5HeldOutBaselineUpdate) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V5_HELD_OUT_BASELINE=1 in the isolated custodian context")
	}
	sourceKeyPath := os.Getenv(closedLoopV5HeldOutSourceKeyEnv)
	baselineKeyPath := os.Getenv(closedLoopV5HeldOutBaselineKeyEnv)
	if sourceKeyPath == "" || baselineKeyPath == "" {
		t.Fatal("both external V5 held-out key paths are required")
	}
	repositoryRoot := filepath.Dir(filepath.Dir(closedLoopSpecDirectory(t)))
	if err := externalkey.Distinct(repositoryRoot, sourceKeyPath, baselineKeyPath); err != nil {
		t.Fatal("V5 held-out key paths failed the distinct external-path gate")
	}
	destinationRoot := filepath.Join(repositoryRoot, "internal", "capabilityfeedback", closedLoopV5HeldOutBaselineRoot)
	for _, path := range []string{baselineKeyPath, destinationRoot} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("V5 held-out baseline updater is retired or its destination is unavailable")
		}
	}
	publisherParent := closedLoopV5CleanPublisherCommit(t, repositoryRoot)

	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, corpuspublication.ManifestFile))
	if corpusHash(manifestBytes) != closedLoopV5CorpusManifestHash {
		t.Fatal("V5 corpus manifest differs from its frozen commitment")
	}
	manifest := loadClosedLoopV5Manifest(t)
	selection := loadClosedLoopV5FrozenSelection(t)
	discoveryBaseline := loadClosedLoopV5FrozenBaselineReport(t)
	assertClosedLoopV5ImplementationBoundary(t, repositoryRoot)
	registry, policy := closedLoopV5Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot)

	sourceKey, err := externalkey.Read(repositoryRoot, sourceKeyPath)
	if err != nil {
		t.Fatal("read external V5 held-out source key")
	}
	defer zeroClosedLoopV5Secret(sourceKey)
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, corpuspublication.HeldOutCipherFile))
	sealedCases, err := corpuspublication.OpenHeldOut(sourceKey, manifest, ciphertext)
	if err != nil {
		t.Fatal("authenticate and open V5 held-out source")
	}
	assertClosedLoopV5HeldOutSources(t, manifest, sealedCases)

	artifacts := runClosedLoopV5HeldOutBaseline(t, sealedCases, policy, inventory, environment, promotionEnvironment)
	if after := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot); after.Public != promotionEnvironment.Public {
		t.Fatal("locked V5 installed-KiCad promotion environment changed during the held-out baseline")
	}
	evidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		evidence[index] = artifacts[index].Observation
	}
	aggregate, err := EvaluateRealizabilityAware(RoleHeldOut, evidence, registry)
	if err != nil {
		t.Fatal("aggregate sealed V5 held-out baseline")
	}
	inventoryHash, catalogHash, modelRegistryHash, environmentPolicyHash := closedLoopV5SealedEnvironmentBindings(t, evidence)
	if inventoryHash != discoveryBaseline.InventorySHA256 || catalogHash != discoveryBaseline.CatalogSHA256 ||
		modelRegistryHash != discoveryBaseline.ModelRegistrySHA256 || environmentPolicyHash != discoveryBaseline.SynthesisPolicySHA256 {
		t.Fatal("sealed V5 held-out baseline did not use the frozen discovery synthesis environment")
	}
	binding := blindbaseline.Binding{
		StartingCommit:               selection.StartingCommit,
		ContractFreezeCommit:         selection.ContractFreezeCommit,
		CorpusFreezeCommit:           selection.CorpusFreezeCommit,
		SelectionFreezeCommit:        closedLoopV5SelectionFreezeCommit,
		PublisherParentCommit:        publisherParent,
		CorpusManifestSHA256:         closedLoopV5CorpusManifestHash,
		ContractManifestSHA256:       manifest.ContractManifestSHA256,
		ValidatorManifestSHA256:      manifest.ValidatorManifestSHA256,
		PublisherManifestSHA256:      manifest.PublisherManifestSHA256,
		ValidationReportSHA256:       manifest.ValidationReportSHA256,
		PacketSetSHA256:              manifest.PacketSetSHA256,
		ContractBindingSHA256:        manifest.ContractBindingSHA256,
		HistoricalCommitmentsSHA256:  manifest.HistoricalCommitmentsSHA256,
		SourceCiphertextSHA256:       manifest.HeldOutSource.CiphertextSHA256,
		DiscoveryBaselineSHA256:      selection.BaselineSHA256,
		RankingSHA256:                selection.RankingSHA256,
		SelectionSHA256:              selection.Hash,
		GenericPlanSHA256:            selection.GenericPlanSHA256,
		EvaluatorPolicy:              RealizabilityPolicyVersion,
		ImpactRegistrySHA256:         closedLoopV5ImpactRegistryFileHash,
		SynthesisPolicySHA256:        closedLoopV5SynthesisPolicyFileHash,
		GapPolicySHA256:              closedLoopV5GapPolicyFileHash,
		SelectionPolicySHA256:        selection.SelectionPolicySHA256,
		ImplementationManifestSHA256: closedLoopV5ImplementationManifestHash,
		InventorySHA256:              inventoryHash,
		CatalogSHA256:                catalogHash,
		ModelRegistrySHA256:          modelRegistryHash,
		EnvironmentPolicySHA256:      environmentPolicyHash,
		PromotionPlatform:            promotionEnvironment.Public.Platform,
		KiCadVersion:                 promotionEnvironment.Public.KiCadVersion,
		PromotionToolchainSHA256:     promotionEnvironment.Public.Hash,
		PromotionToolchainLockSHA256: promotionEnvironment.Public.ToolchainLockSHA256,
		KiCadCLISHA256:               promotionEnvironment.Public.KiCadCLISHA256,
		SymbolTableSHA256:            promotionEnvironment.Public.SymbolTableSHA256,
		FootprintTableSHA256:         promotionEnvironment.Public.FootprintTableSHA256,
		SymbolsSHA256:                promotionEnvironment.Public.SymbolsSHA256,
		FootprintsSHA256:             promotionEnvironment.Public.FootprintsSHA256,
	}
	payload := closedLoopV5HeldOutPayload{Schema: closedLoopV5HeldOutPayloadSchema, Version: closedLoopV5BaselineVersion, Binding: binding, PromotionEnvironment: promotionEnvironment.Public, Cases: artifacts, Aggregate: aggregate}
	payload.Hash, err = hashClosedLoopV5HeldOutPayload(payload)
	if err != nil {
		t.Fatal("hash sealed V5 held-out baseline")
	}
	result, err := blindbaseline.Publish(blindbaseline.Request{
		RepositoryRoot: repositoryRoot, DestinationRoot: destinationRoot,
		KeyPath: baselineKeyPath, ReservedKeyPaths: []string{sourceKeyPath}, Binding: binding,
		Payload: corpusJSON(t, payload), CaseCount: len(artifacts),
	})
	if err != nil {
		t.Fatal("atomically publish encrypted V5 held-out baseline")
	}
	if result.Manifest.CaseCount != closedLoopV5RoleSize {
		t.Fatal("published V5 held-out baseline count is invalid")
	}
	t.Log("sealed 18 V5 held-out baseline cases; no outcome, gap, diagnostic, membership, or promotion detail disclosed")
}

func loadClosedLoopV5FrozenSelection(t *testing.T) closedLoopV5Selection {
	t.Helper()
	var selection closedLoopV5Selection
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, "selection.json")), &selection)
	want, err := hashClosedLoopV5Selection(selection)
	if err != nil || selection.Hash != want || selection.Hash != closedLoopV5SelectionHash ||
		selection.CorpusManifestSHA256 != closedLoopV5CorpusManifestHash || selection.CorpusFreezeCommit != closedLoopV5CorpusFreezeCommit ||
		selection.BaselineSHA256 != closedLoopV5BaselineHash || selection.RankingSHA256 != closedLoopV5RankingHash ||
		selection.SelectionPolicySHA256 != closedLoopV5SelectionPolicyHash || selection.GenericPlanSHA256 != closedLoopV5GenericPlanHash {
		t.Fatal("V5 rank-one selection is not the frozen committed selection")
	}
	return selection
}

func loadClosedLoopV5FrozenBaselineReport(t *testing.T) closedLoopV5BaselineReport {
	t.Helper()
	var report closedLoopV5BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV5BaselineRoot, "report.json")), &report)
	want, err := hashClosedLoopV5BaselineReport(report)
	if err != nil || report.Hash != want || report.Hash != closedLoopV5BaselineHash || report.CorpusManifestSHA256 != closedLoopV5CorpusManifestHash ||
		report.CorpusFreezeCommit != closedLoopV5CorpusFreezeCommit || report.EvaluatorPolicy != RealizabilityPolicyVersion ||
		report.ImpactRegistryFileSHA256 != closedLoopV5ImpactRegistryFileHash || report.SynthesisPolicyFileSHA256 != closedLoopV5SynthesisPolicyFileHash ||
		report.GapPolicyFileSHA256 != closedLoopV5GapPolicyFileHash || report.SelectionPolicySHA256 != closedLoopV5SelectionPolicyHash ||
		report.ImplementationManifestSHA256 != closedLoopV5ImplementationManifestHash {
		t.Fatal("V5 discovery baseline report is not the frozen committed report")
	}
	return report
}

func assertClosedLoopV5ImplementationBoundary(t *testing.T, repositoryRoot string) {
	t.Helper()
	manifestPath := filepath.Join(closedLoopSpecDirectory(t), "V5_IMPLEMENTATION.sha256")
	data := mustCorpusRead(t, manifestPath)
	if corpusHash(data) != closedLoopV5ImplementationManifestHash {
		t.Fatal("V5 implementation boundary manifest differs from its frozen commitment")
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		hash, path, ok := strings.Cut(line, "  ")
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		if !ok || !closedLoopV5ValidHash(hash) || clean != path || filepath.IsAbs(path) || strings.HasPrefix(path, "../") || seen[path] {
			t.Fatal("V5 implementation boundary manifest contains an invalid entry")
		}
		seen[path] = true
		if corpusHash(mustCorpusRead(t, filepath.Join(repositoryRoot, filepath.FromSlash(path)))) != hash {
			t.Fatal("outcome-affecting V5 implementation bytes differ from the frozen starting state")
		}
	}
	if len(seen) != 4 {
		t.Fatal("V5 implementation boundary manifest has an invalid entry count")
	}
}

func assertClosedLoopV5HeldOutSources(t *testing.T, manifest corpuspublication.Manifest, sealed []corpuspublication.HeldOutCase) {
	t.Helper()
	want := make([]corpuspublication.Entry, 0, closedLoopV5RoleSize)
	for _, entry := range manifest.Entries {
		if entry.Role == string(RoleHeldOut) {
			want = append(want, entry)
		}
	}
	if len(want) != closedLoopV5RoleSize || len(sealed) != len(want) {
		t.Fatal("sealed V5 held-out source has an invalid case set")
	}
	for index := range want {
		if sealed[index].Entry != want[index] || corpusHash(sealed[index].Source) != want[index].RequirementSHA256 {
			t.Fatal("sealed V5 held-out source differs from immutable manifest order or identity")
		}
	}
}

func runClosedLoopV5HeldOutBaseline(t *testing.T, sealed []corpuspublication.HeldOutCase, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) []closedLoopV5CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV5CaseArtifact, 0, len(sealed))
	for index := range sealed {
		requirement, issues := ots.DecodeStrict(bytes.NewReader(sealed[index].Source))
		if len(issues) != 0 {
			t.Fatal("sealed V5 held-out requirement failed strict decode")
		}
		sealed[index].Source = nil
		first := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatal("sealed V5 held-out synthesis replay failed closed")
		}
		var promotion *ots.PhysicalPromotionResult
		var proof *closedLoopV5PromotionProof
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopV5SealedRun(t, first, environment, promotionEnvironment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatal("sealed V5 held-out physical promotion failed closed")
			}
			promotion = &current
			proof = &closedLoopV5PromotionProof{Schema: current.Schema, Version: current.Version, Status: current.Status, ReplayIdentical: current.ReplayIdentical, ProjectHash: current.ProjectHash, PromotionHash: current.Hash, RunCount: len(current.Runs)}
		}
		entry := sealed[index].Entry
		observation, err := ObserveRealizabilityAware(CaseMeta{ID: entry.ID, Role: RoleHeldOut, Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact)}, requirement, first, promotion)
		if err != nil {
			t.Fatal("sealed V5 held-out observation failed closed")
		}
		artifact := closedLoopV5CaseArtifact{Schema: closedLoopV5HeldOutCaseSchema, Version: closedLoopV5BaselineVersion, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, NormalizedReplaySHA256: []string{corpusHash(firstBytes), corpusHash(secondBytes)}, SynthesisSHA256: first.Hash, Promotion: proof, Observation: observation}
		artifact.Hash, err = hashClosedLoopV5CaseArtifact(artifact)
		if err != nil {
			t.Fatal("hash sealed V5 held-out case")
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) != closedLoopV5RoleSize {
		t.Fatal("sealed V5 held-out baseline case count is invalid")
	}
	return artifacts
}

func runClosedLoopV5SealedSynthesis(t *testing.T, requirement ots.Requirement, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, policy ots.Policy) ots.SynthesisRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	run := ots.Synthesize(ctx, requirement, inventory, environment, policy)
	if run.Report.Status == ots.StatusInvalid || run.Report.Status == ots.StatusCanceled {
		t.Fatal("sealed V5 held-out synthesis aborted; baseline was not recorded")
	}
	return run
}

func promoteClosedLoopV5SealedRun(t *testing.T, run ots.SynthesisRun, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) ots.PhysicalPromotionResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	index, issues := libraryresolver.Load(ctx, libraryresolver.LibraryRoots{
		SymbolsRoot:    promotionEnvironment.Evidence.SymbolsRoot,
		FootprintsRoot: promotionEnvironment.Evidence.FootprintsRoot,
	}, libraryresolver.LoadOptions{})
	closure, closureIssues := resolveClosedLoopLibraryClosure(index, run)
	closureIssues = append(closureIssues, libraryresolver.DesignClosureIssuesFrom(issues, closure)...)
	for _, issue := range closureIssues {
		if issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked {
			t.Fatal("sealed V5 held-out library closure failed closed")
		}
	}
	return ots.PromoteSynthesisRun(ctx, run, environment, ots.PhysicalPromotionOptions{
		OutputRoot: filepath.Join(t.TempDir(), "sealed-v5-held-out"), KiCadCLI: promotionEnvironment.Evidence.KiCadCLI, LibraryIndex: &index, Timeout: 3 * time.Minute,
	})
}

func resolveClosedLoopV5PromotionEnvironment(t *testing.T, repositoryRoot string) closedLoopV5ResolvedPromotionEnvironment {
	t.Helper()
	document, err := promotiontoolchain.Load(filepath.Join(repositoryRoot, "toolchain", "kicad-promotion.lock.json"))
	if err != nil {
		t.Fatal("load locked V5 installed-KiCad promotion toolchain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	evidence, err := promotiontoolchain.Resolve(ctx, document, promotiontoolchain.ResolveOptions{})
	if err != nil {
		t.Fatal("resolve locked V5 installed-KiCad promotion toolchain")
	}
	cliHash, err := hashClosedLoopV5RegularFile(evidence.KiCadCLI)
	if err != nil {
		t.Fatal("hash locked V5 kicad-cli executable")
	}
	public := closedLoopV5PromotionEnvironment{
		Schema: "kicadai.closed-loop-open-set-promotion-environment.v5", Version: closedLoopV5BaselineVersion,
		Platform: evidence.OS + "/" + evidence.Arch, KiCadVersion: evidence.KiCadVersion,
		ToolchainLockSHA256: evidence.LockSHA256, KiCadCLISHA256: cliHash,
		SymbolTableSHA256: evidence.SymbolTableSHA256, FootprintTableSHA256: evidence.FootprintTableSHA256,
		SymbolsSHA256: evidence.SymbolsIdentity.SHA256, SymbolsFileCount: evidence.SymbolsIdentity.FileCount, SymbolsByteCount: evidence.SymbolsIdentity.ByteCount,
		FootprintsSHA256: evidence.FootprintsIdentity.SHA256, FootprintsFileCount: evidence.FootprintsIdentity.FileCount, FootprintsByteCount: evidence.FootprintsIdentity.ByteCount,
	}
	public.Hash, err = hashClosedLoopV5PromotionEnvironment(public)
	if err != nil || !closedLoopV5ValidHash(public.Hash) || public.SymbolsFileCount <= 0 || public.SymbolsByteCount <= 0 || public.FootprintsFileCount <= 0 || public.FootprintsByteCount <= 0 {
		t.Fatal("locked V5 installed-KiCad promotion environment is invalid")
	}
	return closedLoopV5ResolvedPromotionEnvironment{Evidence: evidence, Public: public}
}

func hashClosedLoopV5RegularFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() <= 0 || opened.Size() > closedLoopV5MaximumKiCadCLIBytes {
		return "", fmt.Errorf("promotion tool is not a nonempty regular file")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(opened, pathInfo) {
		return "", fmt.Errorf("promotion tool path changed or is not regular")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	after, err := file.Stat()
	if err != nil || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return "", fmt.Errorf("promotion tool changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashClosedLoopV5PromotionEnvironment(environment closedLoopV5PromotionEnvironment) (string, error) {
	environment.Hash = ""
	return digest(environment)
}

func closedLoopV5SealedEnvironmentBindings(t *testing.T, cases []CaseEvidence) (string, string, string, string) {
	t.Helper()
	if len(cases) != closedLoopV5RoleSize {
		t.Fatal("sealed V5 held-out environment binding has an invalid case count")
	}
	first := cases[0]
	if !closedLoopV5ValidHash(first.InventoryHash) || !closedLoopV5ValidHash(first.CatalogHash) || !closedLoopV5ValidHash(first.ModelRegistryHash) || !closedLoopV5ValidHash(first.SynthesisPolicyHash) {
		t.Fatal("sealed V5 held-out environment commitment is invalid")
	}
	for _, current := range cases[1:] {
		if current.InventoryHash != first.InventoryHash || current.CatalogHash != first.CatalogHash || current.ModelRegistryHash != first.ModelRegistryHash || current.SynthesisPolicyHash != first.SynthesisPolicyHash {
			t.Fatal("sealed V5 held-out cases used inconsistent synthesis environments")
		}
	}
	return first.InventoryHash, first.CatalogHash, first.ModelRegistryHash, first.SynthesisPolicyHash
}

func closedLoopV5CleanPublisherCommit(t *testing.T, repositoryRoot string) string {
	t.Helper()
	// This is an explicitly gated, repository-only custodian updater. Deriving
	// HEAD and cleanliness from Git is part of its trust boundary; accepting a
	// caller-provided commit would allow an unbound or dirty publication.
	status, err := exec.Command("git", "-C", repositoryRoot, "status", "--porcelain", "--untracked-files=all").CombinedOutput()
	if err != nil || len(status) != 0 {
		t.Fatal("V5 held-out publisher requires an exact clean repository")
	}
	output, err := exec.Command("git", "-C", repositoryRoot, "rev-parse", "--verify", "HEAD^{commit}").CombinedOutput()
	commit := strings.TrimSpace(string(output))
	if err != nil || !closedLoopV5CanonicalGitObjectID(commit) {
		t.Fatal("resolve V5 held-out publisher parent commit")
	}
	return commit
}

func closedLoopV5CanonicalGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func hashClosedLoopV5HeldOutPayload(payload closedLoopV5HeldOutPayload) (string, error) {
	payload.Hash = ""
	return digest(payload)
}

func zeroClosedLoopV5Secret(secret []byte) {
	clear(secret)
	runtime.KeepAlive(secret)
}

func TestClosedLoopV5HeldOutPayloadHashRejectsMutation(t *testing.T) {
	payload := closedLoopV5HeldOutPayload{Schema: closedLoopV5HeldOutPayloadSchema, Version: closedLoopV5BaselineVersion, Binding: blindbaseline.Binding{EvaluatorPolicy: RealizabilityPolicyVersion}}
	first, err := hashClosedLoopV5HeldOutPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Version++
	second, err := hashClosedLoopV5HeldOutPayload(payload)
	if err != nil || first == second {
		t.Fatal("V5 held-out payload mutation did not change its commitment")
	}
}

func TestClosedLoopV5CanonicalGitObjectID(t *testing.T) {
	for _, valid := range []string{strings.Repeat("a", 40), strings.Repeat("b", 64)} {
		if !closedLoopV5CanonicalGitObjectID(valid) {
			t.Fatalf("rejected canonical Git object ID length %d", len(valid))
		}
	}
	for _, invalid := range []string{"", strings.Repeat("a", 39), strings.Repeat("A", 40), strings.Repeat("g", 64)} {
		if closedLoopV5CanonicalGitObjectID(invalid) {
			t.Fatalf("accepted noncanonical Git object ID %q", invalid)
		}
	}
}

func TestClosedLoopV5PromotionEnvironmentCommitments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kicad-cli")
	content := []byte("locked promotion executable")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := hashClosedLoopV5RegularFile(path)
	if err != nil || got != corpusHash(content) {
		t.Fatalf("promotion executable hash = %q, %v", got, err)
	}
	link := filepath.Join(t.TempDir(), "kicad-cli-link")
	if err := os.Symlink(path, link); err == nil {
		if _, err := hashClosedLoopV5RegularFile(link); err == nil {
			t.Fatal("promotion executable hash accepted a symbolic link")
		}
	}
	environment := closedLoopV5PromotionEnvironment{Schema: "schema", Version: 5, Platform: "test/arm64", KiCadVersion: "10.0.3", ToolchainLockSHA256: testV5Hash("1"), KiCadCLISHA256: testV5Hash("2"), SymbolTableSHA256: testV5Hash("3"), FootprintTableSHA256: testV5Hash("4"), SymbolsSHA256: testV5Hash("5"), SymbolsFileCount: 1, SymbolsByteCount: 2, FootprintsSHA256: testV5Hash("6"), FootprintsFileCount: 3, FootprintsByteCount: 4}
	first, err := hashClosedLoopV5PromotionEnvironment(environment)
	if err != nil {
		t.Fatal(err)
	}
	environment.KiCadCLISHA256 = testV5Hash("7")
	second, err := hashClosedLoopV5PromotionEnvironment(environment)
	if err != nil || first == second {
		t.Fatal("promotion toolchain mutation did not change its commitment")
	}
}

func TestClosedLoopV5PromotionEnvironmentResolvesWhenRequested(t *testing.T) {
	if os.Getenv(closedLoopV5PromotionVerifyEnv) != "1" {
		t.Skip("set VERIFY_CLOSED_LOOP_V5_PROMOTION_ENVIRONMENT=1 to resolve and hash the installed locked toolchain")
	}
	repositoryRoot := filepath.Dir(filepath.Dir(closedLoopSpecDirectory(t)))
	resolved := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot)
	if resolved.Public.Platform != runtime.GOOS+"/"+runtime.GOARCH || resolved.Public.KiCadVersion != "10.0.3" || !closedLoopV5ValidHash(resolved.Public.Hash) || !closedLoopV5ValidHash(resolved.Public.KiCadCLISHA256) {
		t.Fatal("resolved V5 promotion environment differs from its locked platform or version")
	}
}

func testV5Hash(character string) string {
	return strings.Repeat(character, 64)
}
