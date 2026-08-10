package capabilityfeedback

import (
	"cmp"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV4FinalSchema               = "kicadai.closed-loop-open-set-final.v4"
	closedLoopV4FinalComparisonSchema     = "kicadai.closed-loop-open-set-comparison.v4"
	closedLoopV4FinalPromotionSchema      = "kicadai.closed-loop-open-set-promotion-matrix.v4"
	closedLoopV4HeldOutFinalPayloadSchema = "kicadai.closed-loop-open-set-held-out-final-payload.v4"
	closedLoopV4HeldOutFinalSealSchema    = "kicadai.closed-loop-open-set-held-out-final-seal.v4"
	closedLoopV4ImplementationSealSchema  = "kicadai.closed-loop-open-set-reviewed-implementation.v4"
	closedLoopV4FinalUpdateEnv            = "UPDATE_CLOSED_LOOP_V4_FINAL"
	closedLoopV4DiscoveryAdmissionEnv     = "RUN_CLOSED_LOOP_V4_DISCOVERY_FINAL"
	closedLoopV4HeldOutFinalKeyEnv        = "KICADAI_V4_HELD_OUT_FINAL_KEY_FILE"
	closedLoopV4FinalRoot                 = "testdata/closed_loop_open_set_v4_final"
	closedLoopV4HeldOutFinalFile          = "held_out_final.sealed"
	closedLoopV4ValidationAuditFile       = "V4_VALIDATION_AUDIT.md"
)

type closedLoopV4ImplementationSeal struct {
	Schema             string                       `json:"schema"`
	Version            int                          `json:"version"`
	SelectedCapability string                       `json:"selected_capability"`
	Review             string                       `json:"review"`
	Artifacts          []closedLoopArtifactEvidence `json:"artifacts"`
	Hash               string                       `json:"hash"`
}

type closedLoopV4FinalReport struct {
	Schema                     string                   `json:"schema"`
	Version                    int                      `json:"version"`
	CorpusManifestHash         string                   `json:"corpus_manifest_hash"`
	FreezeCommit               string                   `json:"freeze_commit"`
	SelectionHash              string                   `json:"selection_hash"`
	DiscoveryBaselineHash      string                   `json:"discovery_baseline_hash"`
	HeldOutBaselinePayloadHash string                   `json:"held_out_baseline_payload_hash"`
	ImplementationSealHash     string                   `json:"implementation_seal_hash"`
	EvaluatorPolicy            string                   `json:"evaluator_policy"`
	ImpactRegistryHash         string                   `json:"impact_registry_hash"`
	SynthesisPolicyHash        string                   `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash    string                   `json:"gap_transition_policy_sha256"`
	Environment                closedLoopEnvironment    `json:"environment"`
	OutcomeCounts              []closedLoopOutcomeCount `json:"outcome_counts"`
	Discovery                  AggregateReport          `json:"discovery"`
	HeldOutAggregateHash       string                   `json:"held_out_aggregate_hash"`
	Attribution                closedLoopAttribution    `json:"attribution"`
	Hash                       string                   `json:"hash"`
}

type closedLoopV4FinalComparison struct {
	Schema                    string `json:"schema"`
	Version                   int    `json:"version"`
	FinalReportHash           string `json:"final_report_hash"`
	SelectionHash             string `json:"selection_hash"`
	DiscoveryPassBefore       int    `json:"discovery_pass_before"`
	DiscoveryPassAfter        int    `json:"discovery_pass_after"`
	HeldOutPassBefore         int    `json:"held_out_pass_before"`
	HeldOutPassAfter          int    `json:"held_out_pass_after"`
	RankOneAffectedPassBefore int    `json:"rank_one_affected_pass_before"`
	RankOneAffectedPassAfter  int    `json:"rank_one_affected_pass_after"`
	NoBaselinePassRegression  bool   `json:"no_baseline_pass_regression"`
	UnsafeEvidencePreserved   bool   `json:"unsafe_evidence_preserved"`
	RemainingGapsStable       bool   `json:"remaining_gaps_stable"`
	Hash                      string `json:"hash"`
}

type closedLoopV4FinalPromotionMatrix struct {
	Schema          string                   `json:"schema"`
	Version         int                      `json:"version"`
	FinalReportHash string                   `json:"final_report_hash"`
	RequiredGates   []string                 `json:"required_gates"`
	Promotions      []closedLoopPromotionRow `json:"promotions"`
	Hash            string                   `json:"hash"`
}

type closedLoopV4HeldOutFinalPayload struct {
	Schema                  string                   `json:"schema"`
	Version                 int                      `json:"version"`
	CorpusManifestHash      string                   `json:"corpus_manifest_hash"`
	SelectionHash           string                   `json:"selection_hash"`
	ImplementationHash      string                   `json:"implementation_hash"`
	GapTransitionPolicyHash string                   `json:"gap_transition_policy_sha256"`
	Cases                   []CaseEvidence           `json:"cases"`
	Aggregate               AggregateReport          `json:"aggregate"`
	Promotions              []closedLoopPromotionRow `json:"promotions"`
	Hash                    string                   `json:"hash"`
}

type closedLoopV4HeldOutFinalSeal struct {
	Schema                  string `json:"schema"`
	Version                 int    `json:"version"`
	Algorithm               string `json:"algorithm"`
	CorpusManifestHash      string `json:"corpus_manifest_hash"`
	SelectionHash           string `json:"selection_hash"`
	ImplementationHash      string `json:"implementation_hash"`
	GapTransitionPolicyHash string `json:"gap_transition_policy_sha256"`
	BaselinePayloadHash     string `json:"baseline_payload_hash"`
	PayloadHash             string `json:"payload_hash"`
	AggregateHash           string `json:"aggregate_hash"`
	CiphertextHash          string `json:"ciphertext_sha256"`
	CaseCount               int    `json:"case_count"`
	Hash                    string `json:"hash"`
}

func TestClosedLoopV4ReviewedImplementationSealIsFrozen(t *testing.T) {
	// Historical evidence binds the reviewed bytes by digest. It must not
	// require the live production tree to remain at V4 forever; live-tree
	// equality is an admission gate of the one-time updater below.
	loadClosedLoopV4HistoricalImplementationSeal(t)
}

func TestClosedLoopV4ValidationAuditRetiresFinalUpdater(t *testing.T) {
	want := filepath.Join(closedLoopSpecDirectory(t), closedLoopV4ValidationAuditFile)
	if _, err := os.Stat(want); os.IsNotExist(err) {
		t.Skip("V4 final validation has not run")
	} else if err != nil {
		t.Fatalf("V4 validation audit does not retire the final updater: %v", err)
	}
	if !slices.Contains(closedLoopV4FinalBlockerPaths(t), want) {
		t.Fatal("V4 validation audit is not a final-update blocker")
	}
}

func TestClosedLoopV4FinalArtifactsAreFrozen(t *testing.T) {
	specRoot := closedLoopSpecDirectory(t)
	reportPath := filepath.Join(specRoot, "V4_FINAL_REPORT.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Skip("V4 final artifacts have not been produced")
	}
	var report closedLoopV4FinalReport
	reportBytes := mustCorpusRead(t, reportPath)
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_FINAL_REPORT.sha256"), "V4_FINAL_REPORT.json", reportBytes)
	decodeCorpusStrict(t, reportBytes, &report)
	if want, err := hashClosedLoopV4FinalReport(report); err != nil || want != report.Hash {
		t.Fatal("V4 final report hash is invalid")
	}
	manifest := loadClosedLoopV4Manifest(t)
	selection := loadFrozenClosedLoopV4Selection(t, manifest)
	if report.CorpusManifestHash != selection.CorpusManifestHash || report.SelectionHash != selection.Hash ||
		report.FreezeCommit != closedLoopV4SelectionCommit || report.DiscoveryBaselineHash != selection.DiscoveryBaselineHash ||
		report.EvaluatorPolicy != selection.EvaluatorPolicy || report.ImpactRegistryHash != selection.ImpactRegistryHash ||
		report.SynthesisPolicyHash != selection.SynthesisPolicyHash ||
		report.GapTransitionPolicyHash != selection.GapTransitionPolicyHash {
		t.Fatal("V4 final report policy bindings are invalid")
	}
	var comparison closedLoopV4FinalComparison
	comparisonBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_FINAL_COMPARISON.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_FINAL_COMPARISON.sha256"), "V4_FINAL_COMPARISON.json", comparisonBytes)
	decodeCorpusStrict(t, comparisonBytes, &comparison)
	if want, err := hashClosedLoopV4FinalComparison(comparison); err != nil || want != comparison.Hash ||
		comparison.FinalReportHash != report.Hash || comparison.SelectionHash != report.SelectionHash {
		t.Fatal("V4 final comparison is invalid")
	}
	var matrix closedLoopV4FinalPromotionMatrix
	matrixBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_PROMOTION_MATRIX.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_PROMOTION_MATRIX.sha256"), "V4_PROMOTION_MATRIX.json", matrixBytes)
	decodeCorpusStrict(t, matrixBytes, &matrix)
	if want, err := hashClosedLoopV4FinalPromotionMatrix(matrix); err != nil || want != matrix.Hash ||
		matrix.FinalReportHash != report.Hash || !slices.Equal(matrix.RequiredGates, closedLoopRequiredPromotionGates()) {
		t.Fatal("V4 promotion matrix is invalid")
	}
	var seal closedLoopV4HeldOutFinalSeal
	sealBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_HELD_OUT_FINAL_SEAL.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_HELD_OUT_FINAL_SEAL.sha256"), "V4_HELD_OUT_FINAL_SEAL.json", sealBytes)
	decodeCorpusStrict(t, sealBytes, &seal)
	if want, err := hashClosedLoopV4HeldOutFinalSeal(seal); err != nil || want != seal.Hash ||
		seal.CorpusManifestHash != report.CorpusManifestHash || seal.SelectionHash != report.SelectionHash ||
		seal.GapTransitionPolicyHash != report.GapTransitionPolicyHash ||
		seal.BaselinePayloadHash != report.HeldOutBaselinePayloadHash || seal.AggregateHash != report.HeldOutAggregateHash ||
		seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V4 held-out final seal is invalid")
	}
	if corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV4FinalRoot, closedLoopV4HeldOutFinalFile))) != seal.CiphertextHash {
		t.Fatal("V4 held-out final ciphertext hash drifted")
	}
	// Final evidence binds the immutable historical implementation seal. A
	// later production version may legitimately differ from those sealed bytes.
	implementation := loadClosedLoopV4HistoricalImplementationSeal(t)
	if implementation.Hash != report.ImplementationSealHash || implementation.Hash != seal.ImplementationHash {
		t.Fatal("V4 final artifacts are not bound to the reviewed implementation")
	}
	verifyClosedLoopV4Comparison(t, comparison)
	if len(matrix.Promotions) != comparison.DiscoveryPassAfter+comparison.HeldOutPassAfter {
		t.Fatal("V4 promotion count does not match final pass count")
	}
}

func TestClosedLoopV4DiscoveryFinalAdmission(t *testing.T) {
	if os.Getenv(closedLoopV4DiscoveryAdmissionEnv) != "1" {
		t.Skip("set RUN_CLOSED_LOOP_V4_DISCOVERY_FINAL=1 for the public-only V4 discovery admission")
	}
	loadClosedLoopV4CurrentImplementationSeal(t)
	manifest := loadClosedLoopV4Manifest(t)
	registry, policy := closedLoopV4Policies(t)
	selection := loadFrozenClosedLoopV4Selection(t, manifest)
	baseline := loadClosedLoopV4DiscoveryBaselineReport(t)
	if baseline.Hash != selection.DiscoveryBaselineHash ||
		baseline.CorpusManifestHash != selection.CorpusManifestHash ||
		baseline.EvaluatorPolicy != selection.EvaluatorPolicy ||
		baseline.ImpactRegistryHash != selection.ImpactRegistryHash ||
		baseline.SynthesisPolicyHash != selection.SynthesisPolicyHash ||
		baseline.GapTransitionPolicyHash != selection.GapTransitionPolicyHash {
		t.Fatal("V4 public discovery admission rejected frozen-policy drift")
	}
	inventory, environment := closedLoopSynthesisEnvironment(t)
	discoveryCases := runClosedLoopV4DiscoveryBaseline(t, manifest, policy, inventory, environment)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, discoveryCases, registry)
	if err != nil {
		t.Fatal("V4 public discovery admission aggregation failed closed")
	}
	if !closedLoopV4DiscoveryAdmissionPasses(selection, baseline.Discovery.Cases, discovery.Cases) {
		t.Fatal("V4 public discovery admission did not prove strict, preservation-safe uplift")
	}
}

func TestUpdateClosedLoopV4Final(t *testing.T) {
	if os.Getenv(closedLoopV4FinalUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V4_FINAL=1 for the one-time V4 final evaluation")
	}
	refuseExistingClosedLoopV4Final(t)
	implementation := loadClosedLoopV4CurrentImplementationSeal(t)
	manifest := loadClosedLoopV4Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	registry, policy := closedLoopV4Policies(t)
	selection := loadFrozenClosedLoopV4Selection(t, manifest)
	baseline := loadClosedLoopV4DiscoveryBaselineReport(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)

	// Discovery is the hard boundary: no held-out key or ciphertext is opened
	// until the reviewed implementation proves strict selected-cluster uplift.
	discoveryCases := runClosedLoopV4DiscoveryBaseline(t, manifest, policy, inventory, environment)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, discoveryCases, registry)
	if err != nil {
		t.Fatal("V4 final discovery aggregation failed closed")
	}
	discoveryBefore := countV4Outcome(baseline.Discovery.Cases, OutcomePass)
	discoveryAfter := countV4Outcome(discovery.Cases, OutcomePass)
	rankBefore, rankAfter, rankComplete := selectedV4PassCounts(selection, baseline.Discovery.Cases, discovery.Cases)
	if !rankComplete || !closedLoopV4DiscoveryAdmissionPasses(selection, baseline.Discovery.Cases, discovery.Cases) {
		t.Fatalf("V4 held-out final remains sealed: discovery pass %d->%d selected %d->%d", discoveryBefore, discoveryAfter, rankBefore, rankAfter)
	}

	sourceKeyPath := os.Getenv(closedLoopV4HeldOutCorpusKeyEnv)
	baselineKeyPath := os.Getenv(closedLoopV4HeldOutBaselineKeyEnv)
	finalKeyPath := os.Getenv(closedLoopV4HeldOutFinalKeyEnv)
	if sourceKeyPath == "" || baselineKeyPath == "" || finalKeyPath == "" {
		t.Fatal("separate external V4 source, baseline, and final key paths are required")
	}
	closedLoopV4RequireExternalKeyPath(t, sourceKeyPath)
	closedLoopV4RequireExternalKeyPath(t, baselineKeyPath)
	closedLoopV4RequireExternalKeyPath(t, finalKeyPath)
	if sourceKeyPath == finalKeyPath || sourceKeyPath == baselineKeyPath || finalKeyPath == baselineKeyPath {
		t.Fatal("V4 source, baseline, and final keys must be distinct")
	}
	defer func() {
		if t.Failed() {
			writeClosedLoopV4FailureAuditIfAbsent(t, implementation.Hash)
		}
	}()
	baselinePayload := loadClosedLoopV4HeldOutBaselinePayload(t, selection)
	requirements := loadClosedLoopV4HeldOutRequirements(t, manifest, sourceKeyPath)
	heldOutCases := runClosedLoopV4HeldOutBaseline(t, manifest, requirements, policy, inventory, environment)
	heldOut, err := EvaluateRealizabilityAware(RoleHeldOut, heldOutCases, registry)
	if err != nil {
		t.Fatal("V4 final held-out aggregation failed closed")
	}
	comparison := buildClosedLoopV4FinalComparison(t, selection, baseline.Discovery.Cases, discovery.Cases, baselinePayload.Cases, heldOut.Cases)
	if !closedLoopV4ComparisonPasses(comparison) {
		writeClosedLoopV4ValidationAudit(t, "failed", implementation.Hash)
		t.Fatal("V4 final validation failed after held-out reveal; outcome details remain sealed")
	}

	promotions := closedLoopV4PromotionRows(combineV4Cases(discovery.Cases, heldOut.Cases))
	payload := closedLoopV4HeldOutFinalPayload{
		Schema: closedLoopV4HeldOutFinalPayloadSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: corpusHash(manifestBytes), SelectionHash: selection.Hash,
		ImplementationHash: implementation.Hash, GapTransitionPolicyHash: manifest.GapTransitionPolicyHash,
		Cases: heldOut.Cases, Aggregate: heldOut,
		Promotions: closedLoopV4PromotionRows(heldOut.Cases),
	}
	payload.Hash, err = hashClosedLoopV4HeldOutFinalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	finalKey := closedLoopV4LoadOrCreateKey(t, finalKeyPath)
	ciphertext := sealClosedLoopV4HeldOutFinalPayload(t, finalKey, payload, corpusJSON(t, payload))
	report := buildClosedLoopV4FinalReport(t, manifest, corpusHash(manifestBytes), selection, baseline, baselinePayload, implementation, discovery, heldOut, promotions)
	comparison.FinalReportHash = report.Hash
	comparison.Hash, err = hashClosedLoopV4FinalComparison(comparison)
	if err != nil {
		t.Fatal(err)
	}
	matrix := closedLoopV4FinalPromotionMatrix{
		Schema: closedLoopV4FinalPromotionSchema, Version: closedLoopV4BaselineVersion,
		FinalReportHash: report.Hash, RequiredGates: closedLoopRequiredPromotionGates(), Promotions: promotions,
	}
	matrix.Hash, err = hashClosedLoopV4FinalPromotionMatrix(matrix)
	if err != nil {
		t.Fatal(err)
	}
	seal := closedLoopV4HeldOutFinalSeal{
		Schema: closedLoopV4HeldOutFinalSealSchema, Version: closedLoopV4BaselineVersion,
		Algorithm: closedLoopV4HeldOutSealAlgorithm, CorpusManifestHash: payload.CorpusManifestHash,
		SelectionHash: selection.Hash, ImplementationHash: implementation.Hash,
		GapTransitionPolicyHash: manifest.GapTransitionPolicyHash,
		BaselinePayloadHash:     baselinePayload.Hash, PayloadHash: payload.Hash,
		AggregateHash: heldOut.Hash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(heldOut.Cases),
	}
	seal.Hash, err = hashClosedLoopV4HeldOutFinalSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	writeClosedLoopV4Final(t, discovery.Cases, report, comparison, matrix, seal, ciphertext)
}

func loadClosedLoopV4DiscoveryBaselineReport(t *testing.T) closedLoopV4DiscoveryBaselineReport {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V4_DISCOVERY_BASELINE_REPORT.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V4_DISCOVERY_BASELINE_REPORT.sha256"), filepath.Base(path), data)
	var report closedLoopV4DiscoveryBaselineReport
	decodeCorpusStrict(t, data, &report)
	return report
}

func loadClosedLoopV4HeldOutBaselinePayload(t *testing.T, selection closedLoopV4Selection) closedLoopV4HeldOutPayload {
	t.Helper()
	key, err := os.ReadFile(os.Getenv(closedLoopV4HeldOutBaselineKeyEnv))
	if err != nil || len(key) != 32 {
		t.Fatal("external V4 held-out baseline key is invalid")
	}
	var seal closedLoopV4HeldOutSeal
	sealPath := filepath.Join(closedLoopSpecDirectory(t), "V4_HELD_OUT_BASELINE_SEAL.json")
	sealBytes := mustCorpusRead(t, sealPath)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V4_HELD_OUT_BASELINE_SEAL.sha256"), filepath.Base(sealPath), sealBytes)
	decodeCorpusStrict(t, sealBytes, &seal)
	if seal.Schema != closedLoopV4HeldOutSealSchema || seal.Version != closedLoopV4BaselineVersion ||
		seal.Algorithm != closedLoopV4HeldOutSealAlgorithm || seal.FreezeCommit != closedLoopV4SelectionCommit ||
		seal.CorpusManifestHash != selection.CorpusManifestHash || seal.SelectionHash != selection.Hash ||
		seal.EvaluatorPolicy != selection.EvaluatorPolicy || seal.ImpactRegistryHash != selection.ImpactRegistryHash ||
		seal.SynthesisPolicyHash != selection.SynthesisPolicyHash ||
		seal.GapTransitionPolicyHash != selection.GapTransitionPolicyHash || seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V4 held-out baseline seal metadata is invalid")
	}
	if want, err := hashClosedLoopV4HeldOutSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V4 held-out baseline seal hash is invalid")
	}
	metadata := closedLoopV4HeldOutPayload{
		Schema: closedLoopV4HeldOutPayloadSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: seal.CorpusManifestHash, SelectionHash: selection.Hash,
		EvaluatorPolicy: seal.EvaluatorPolicy, ImpactRegistryHash: seal.ImpactRegistryHash,
		SynthesisPolicyHash: seal.SynthesisPolicyHash, GapTransitionPolicyHash: seal.GapTransitionPolicyHash,
		Hash: seal.PayloadHash,
	}
	plaintext, err := openClosedLoopV4HeldOutPayload(key, metadata,
		mustCorpusRead(t, filepath.Join(closedLoopV4BaselineRoot, closedLoopV4HeldOutBaselineFile)))
	if err != nil {
		t.Fatal("V4 held-out baseline authentication failed")
	}
	var payload closedLoopV4HeldOutPayload
	decodeCorpusStrict(t, plaintext, &payload)
	wantHash, hashErr := hashClosedLoopV4HeldOutPayload(payload)
	if hashErr != nil || wantHash != payload.Hash || payload.Hash != seal.PayloadHash ||
		payload.CorpusManifestHash != seal.CorpusManifestHash || payload.SelectionHash != selection.Hash ||
		payload.EvaluatorPolicy != seal.EvaluatorPolicy || payload.ImpactRegistryHash != seal.ImpactRegistryHash ||
		payload.SynthesisPolicyHash != seal.SynthesisPolicyHash || payload.GapTransitionPolicyHash != seal.GapTransitionPolicyHash ||
		len(payload.Cases) != closedLoopCorpusSize/2 {
		t.Fatal("V4 held-out baseline payload contract is invalid")
	}
	return payload
}

func sealClosedLoopV4HeldOutFinalPayload(
	t *testing.T,
	key []byte,
	payload closedLoopV4HeldOutFinalPayload,
	plaintext []byte,
) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal("generate V4 held-out final nonce")
	}
	return gcm.Seal(nonce, nonce, plaintext, closedLoopV4FinalAdditionalData(payload))
}

func closedLoopV4FinalAdditionalData(payload closedLoopV4HeldOutFinalPayload) []byte {
	return []byte(v4LengthPrefixedStrings(
		payload.Schema,
		payload.CorpusManifestHash,
		payload.SelectionHash,
		payload.ImplementationHash,
		payload.GapTransitionPolicyHash,
		payload.Hash,
	))
}

func TestClosedLoopV4FinalAdditionalDataIsUnambiguous(t *testing.T) {
	left := closedLoopV4HeldOutFinalPayload{Schema: "ab", CorpusManifestHash: "c"}
	right := closedLoopV4HeldOutFinalPayload{Schema: "a", CorpusManifestHash: "bc"}
	if slices.Equal(closedLoopV4FinalAdditionalData(left), closedLoopV4FinalAdditionalData(right)) {
		t.Fatal("V4 final authenticated-data encoding accepted an ambiguous field boundary")
	}
	if !slices.Equal(closedLoopV4FinalAdditionalData(left), closedLoopV4FinalAdditionalData(left)) {
		t.Fatal("V4 final authenticated-data encoding is not deterministic")
	}
}

func buildClosedLoopV4FinalReport(
	t *testing.T,
	manifest closedLoopV4CorpusManifest,
	manifestHash string,
	selection closedLoopV4Selection,
	baseline closedLoopV4DiscoveryBaselineReport,
	heldOutBaseline closedLoopV4HeldOutPayload,
	implementation closedLoopV4ImplementationSeal,
	discovery AggregateReport,
	heldOut AggregateReport,
	promotions []closedLoopPromotionRow,
) closedLoopV4FinalReport {
	t.Helper()
	report := closedLoopV4FinalReport{
		Schema: closedLoopV4FinalSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopV4SelectionCommit,
		SelectionHash: selection.Hash, DiscoveryBaselineHash: baseline.Hash,
		HeldOutBaselinePayloadHash: heldOutBaseline.Hash, ImplementationSealHash: implementation.Hash,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, GapTransitionPolicyHash: manifest.GapTransitionPolicyHash,
		Environment:   manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(combineV4Cases(discovery.Cases, heldOut.Cases)),
		Discovery:     discovery, HeldOutAggregateHash: heldOut.Hash,
		Attribution: closedLoopAttribution{
			SelectedClusterKey: selection.Cluster.Key, GenericArtifacts: implementation.Artifacts,
			DiscoveryPromotionHashes: promotionHashes(promotions, RoleDiscovery),
			HeldOutPromotionHashes:   promotionHashes(promotions, RoleHeldOut),
		},
	}
	var err error
	report.Hash, err = hashClosedLoopV4FinalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func buildClosedLoopV4FinalComparison(
	t *testing.T,
	selection closedLoopV4Selection,
	discoveryBefore, discoveryAfter, heldOutBefore, heldOutAfter []CaseEvidence,
) closedLoopV4FinalComparison {
	t.Helper()
	allBefore := append(slices.Clone(discoveryBefore), heldOutBefore...)
	allAfter := append(slices.Clone(discoveryAfter), heldOutAfter...)
	rankBefore, rankAfter, complete := selectedV4PassCounts(selection, allBefore, allAfter)
	if !complete {
		t.Fatal("V4 rank-one comparison case sets do not match")
	}
	comparison := closedLoopV4FinalComparison{
		Schema: closedLoopV4FinalComparisonSchema, Version: closedLoopV4BaselineVersion,
		SelectionHash:       selection.Hash,
		DiscoveryPassBefore: countV4Outcome(discoveryBefore, OutcomePass), DiscoveryPassAfter: countV4Outcome(discoveryAfter, OutcomePass),
		HeldOutPassBefore: countV4Outcome(heldOutBefore, OutcomePass), HeldOutPassAfter: countV4Outcome(heldOutAfter, OutcomePass),
		RankOneAffectedPassBefore: rankBefore, RankOneAffectedPassAfter: rankAfter,
		NoBaselinePassRegression: noV4PassRegression(discoveryBefore, discoveryAfter) && noV4PassRegression(heldOutBefore, heldOutAfter),
		UnsafeEvidencePreserved:  v4UnsafePreserved(discoveryBefore, discoveryAfter) && v4UnsafePreserved(heldOutBefore, heldOutAfter),
		RemainingGapsStable: v4RemainingGapsStable(discoveryBefore, discoveryAfter, selection.Cluster) &&
			v4RemainingGapsStable(heldOutBefore, heldOutAfter, selection.Cluster),
	}
	return comparison
}

func verifyClosedLoopV4Comparison(t *testing.T, comparison closedLoopV4FinalComparison) {
	t.Helper()
	if !closedLoopV4ComparisonPasses(comparison) {
		t.Fatal("V4 strict improvement evidence is invalid")
	}
}

func closedLoopV4ComparisonPasses(comparison closedLoopV4FinalComparison) bool {
	return comparison.DiscoveryPassAfter > comparison.DiscoveryPassBefore &&
		comparison.HeldOutPassAfter > comparison.HeldOutPassBefore &&
		comparison.RankOneAffectedPassAfter > comparison.RankOneAffectedPassBefore &&
		comparison.NoBaselinePassRegression && comparison.UnsafeEvidencePreserved && comparison.RemainingGapsStable
}

func closedLoopV4DiscoveryAdmissionPasses(selection closedLoopV4Selection, before, after []CaseEvidence) bool {
	rankBefore, rankAfter, complete := selectedV4PassCounts(selection, before, after)
	return complete && countV4Outcome(after, OutcomePass) > countV4Outcome(before, OutcomePass) &&
		rankAfter > rankBefore && noV4PassRegression(before, after) &&
		v4UnsafePreserved(before, after) && v4RemainingGapsStable(before, after, selection.Cluster)
}

func TestClosedLoopV4ComparisonRequiresEveryStrictGate(t *testing.T) {
	valid := closedLoopV4FinalComparison{
		DiscoveryPassBefore: 1, DiscoveryPassAfter: 2,
		HeldOutPassBefore: 1, HeldOutPassAfter: 2,
		RankOneAffectedPassBefore: 0, RankOneAffectedPassAfter: 1,
		NoBaselinePassRegression: true, UnsafeEvidencePreserved: true, RemainingGapsStable: true,
	}
	if !closedLoopV4ComparisonPasses(valid) {
		t.Fatal("complete V4 strict-improvement evidence was rejected")
	}
	invalid := []closedLoopV4FinalComparison{
		func() closedLoopV4FinalComparison { value := valid; value.DiscoveryPassAfter = 1; return value }(),
		func() closedLoopV4FinalComparison { value := valid; value.HeldOutPassAfter = 1; return value }(),
		func() closedLoopV4FinalComparison { value := valid; value.RankOneAffectedPassAfter = 0; return value }(),
		func() closedLoopV4FinalComparison {
			value := valid
			value.NoBaselinePassRegression = false
			return value
		}(),
		func() closedLoopV4FinalComparison {
			value := valid
			value.UnsafeEvidencePreserved = false
			return value
		}(),
		func() closedLoopV4FinalComparison { value := valid; value.RemainingGapsStable = false; return value }(),
	}
	for index, comparison := range invalid {
		if closedLoopV4ComparisonPasses(comparison) {
			t.Fatalf("V4 comparison accepted missing strict gate %d", index)
		}
	}
}

func selectedV4PassCounts(selection closedLoopV4Selection, before, after []CaseEvidence) (int, int, bool) {
	beforeByID, beforeOK := uniqueV4FinalCases(before)
	afterByID, afterOK := uniqueV4FinalCases(after)
	if !beforeOK || !afterOK || len(beforeByID) != len(afterByID) {
		return 0, 0, false
	}
	for id := range beforeByID {
		if _, found := afterByID[id]; !found {
			return 0, 0, false
		}
	}
	affected := make(map[string]bool, len(selection.Cluster.Cases))
	for _, id := range selection.Cluster.Cases {
		affected[id] = true
	}
	for _, current := range before {
		for _, gap := range current.Gaps {
			if gap.Stage == selection.Cluster.Stage && gap.Scope == selection.Cluster.Scope &&
				gap.Capability == selection.Cluster.Capability && gap.Code == selection.Cluster.Code {
				affected[current.Case.ID] = true
				break
			}
		}
	}
	beforeCount, afterCount := 0, 0
	for id := range affected {
		beforeCase, beforeFound := beforeByID[id]
		afterCase, afterFound := afterByID[id]
		if !beforeFound || !afterFound {
			return 0, 0, false
		}
		if beforeCase.Outcome == OutcomePass {
			beforeCount++
		}
		if afterCase.Outcome == OutcomePass {
			afterCount++
		}
	}
	return beforeCount, afterCount, true
}

func TestSelectedV4PassCountsIncludesHeldOutClusterCases(t *testing.T) {
	selection := closedLoopV4Selection{Cluster: Cluster{
		Stage: "topology_search", Scope: ScopeTopology,
		Capability: "complete_topology", Code: "OPEN_TOPOLOGY_SEARCH_EXHAUSTED",
		Cases: []string{"discovery_case"},
	}}
	matchingGap := Gap{
		Stage: "topology_search", Scope: ScopeTopology,
		Capability: "complete_topology", Code: "OPEN_TOPOLOGY_SEARCH_EXHAUSTED",
	}
	before := []CaseEvidence{
		{Case: CaseMeta{ID: "discovery_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{matchingGap}},
		{Case: CaseMeta{ID: "held_out_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{matchingGap}},
		{Case: CaseMeta{ID: "other_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{{
			Stage: "simulation", Scope: ScopeSimulation, Capability: "transient_solver", Code: "SIMULATION_INVALID",
		}}},
	}
	after := []CaseEvidence{
		{Case: CaseMeta{ID: "discovery_case"}, Outcome: OutcomePass},
		{Case: CaseMeta{ID: "held_out_case"}, Outcome: OutcomePass},
		{Case: CaseMeta{ID: "other_case"}, Outcome: OutcomePass},
	}

	beforeCount, afterCount, complete := selectedV4PassCounts(selection, before, after)
	if beforeCount != 0 || afterCount != 2 || !complete {
		t.Fatalf("selected rank counts = (%d, %d, %t), want (0, 2, true)", beforeCount, afterCount, complete)
	}
}

func TestSelectedV4PassCountsRejectsMismatchedCaseSets(t *testing.T) {
	selection := closedLoopV4Selection{Cluster: Cluster{Cases: []string{"affected"}}}
	before := []CaseEvidence{{Case: CaseMeta{ID: "affected"}, Outcome: OutcomeUnsupported}}
	if _, _, complete := selectedV4PassCounts(selection, before, nil); complete {
		t.Fatal("mismatched selected case sets were accepted")
	}
}

func countV4Outcome(cases []CaseEvidence, outcome Outcome) int {
	count := 0
	for _, current := range cases {
		if current.Outcome == outcome {
			count++
		}
	}
	return count
}

func noV4PassRegression(before, after []CaseEvidence) bool {
	beforeByID, afterByID, complete := pairedV4FinalCases(before, after)
	if !complete {
		return false
	}
	for id, current := range beforeByID {
		if current.Outcome == OutcomePass && afterByID[id].Outcome != OutcomePass {
			return false
		}
	}
	return true
}

func v4UnsafePreserved(before, after []CaseEvidence) bool {
	beforeByID, afterByID, complete := pairedV4FinalCases(before, after)
	if !complete {
		return false
	}
	for id, current := range beforeByID {
		if current.Outcome == OutcomeUnsafe && afterByID[id].Outcome == OutcomePass {
			return false
		}
	}
	return true
}

func v4RemainingGapsStable(before, after []CaseEvidence, selected Cluster) bool {
	beforeByID, beforeOK := uniqueV4FinalCases(before)
	afterByID, afterOK := uniqueV4FinalCases(after)
	if !beforeOK || !afterOK || len(beforeByID) != len(afterByID) {
		return false
	}
	for id, current := range beforeByID {
		next, found := afterByID[id]
		if !found {
			return false
		}
		if next.Outcome == OutcomePass {
			continue
		}
		finalGaps := map[string]bool{}
		for _, gap := range next.Gaps {
			finalGaps[v4FinalGapIdentity(gap)] = true
		}
		for _, gap := range current.Gaps {
			if gap.Stage == selected.Stage && gap.Scope == selected.Scope &&
				gap.Capability == selected.Capability && gap.Code == selected.Code {
				continue
			}
			if !finalGaps[v4FinalGapIdentity(gap)] {
				return false
			}
		}
	}
	return true
}

func uniqueV4FinalCases(cases []CaseEvidence) (map[string]CaseEvidence, bool) {
	result := make(map[string]CaseEvidence, len(cases))
	for _, current := range cases {
		if current.Case.ID == "" {
			return nil, false
		}
		if _, exists := result[current.Case.ID]; exists {
			return nil, false
		}
		result[current.Case.ID] = current
	}
	return result, true
}

func pairedV4FinalCases(before, after []CaseEvidence) (map[string]CaseEvidence, map[string]CaseEvidence, bool) {
	beforeByID, beforeOK := uniqueV4FinalCases(before)
	afterByID, afterOK := uniqueV4FinalCases(after)
	if !beforeOK || !afterOK || len(beforeByID) != len(afterByID) {
		return nil, nil, false
	}
	for id := range beforeByID {
		if _, found := afterByID[id]; !found {
			return nil, nil, false
		}
	}
	return beforeByID, afterByID, true
}

func v4FinalGapIdentity(gap Gap) string {
	required := slices.Clone(gap.RequiredEvidence)
	slices.Sort(required)
	required = slices.Compact(required)
	values := append([]string{gap.Stage, string(gap.Scope), gap.Capability, gap.Code}, required...)
	return v4LengthPrefixedStrings(values...)
}

func v4LengthPrefixedStrings(values ...string) string {
	var encoded strings.Builder
	for _, value := range values {
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	return encoded.String()
}

func combineV4Cases(left, right []CaseEvidence) []CaseEvidence {
	combined := make([]CaseEvidence, 0, len(left)+len(right))
	combined = append(combined, left...)
	combined = append(combined, right...)
	return combined
}

func TestV4RemainingGapsUseExactMonotonicIdentity(t *testing.T) {
	selected := Cluster{Stage: "topology_search", Scope: ScopeTopology, Capability: "complete_topology", Code: "SELECTED"}
	selectedGap := Gap{Stage: selected.Stage, Scope: selected.Scope, Capability: selected.Capability, Code: selected.Code}
	remainingGap := Gap{Stage: "simulation", Scope: ScopeSimulation, Capability: "transient_solver", Code: "REMAINS", RequiredEvidence: []string{"erc", "drc"}}
	extraGap := Gap{Stage: "promotion", Scope: ScopePhysical, Capability: "physical_promotion", Code: "NEW"}
	evidence := func(gaps ...Gap) []CaseEvidence {
		return []CaseEvidence{{Case: CaseMeta{ID: "case"}, Outcome: OutcomeUnsupported, Gaps: gaps}}
	}
	tests := []struct {
		name   string
		before []CaseEvidence
		after  []CaseEvidence
		want   bool
	}{
		{name: "equal", before: evidence(remainingGap), after: evidence(remainingGap), want: true},
		{name: "final superset", before: evidence(remainingGap), after: evidence(remainingGap, extraGap), want: true},
		{name: "exact selected removal", before: evidence(selectedGap, remainingGap), after: evidence(remainingGap), want: true},
		{name: "different code removal", before: evidence(Gap{Stage: selected.Stage, Scope: selected.Scope, Capability: selected.Capability, Code: "OTHER"}), after: evidence(), want: false},
		{name: "required evidence mutation", before: evidence(remainingGap), after: evidence(Gap{Stage: remainingGap.Stage, Scope: remainingGap.Scope, Capability: remainingGap.Capability, Code: remainingGap.Code, RequiredEvidence: []string{"erc", "connectivity"}}), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := v4RemainingGapsStable(test.before, test.after, selected); got != test.want {
				t.Fatalf("v4RemainingGapsStable() = %t, want %t", got, test.want)
			}
		})
	}
	duplicate := append(evidence(remainingGap), evidence(remainingGap)...)
	if v4RemainingGapsStable(duplicate, evidence(remainingGap), selected) ||
		v4RemainingGapsStable(evidence(remainingGap), nil, selected) ||
		noV4PassRegression(duplicate, evidence(remainingGap)) ||
		v4UnsafePreserved(evidence(remainingGap), nil) {
		t.Fatal("V4 preservation gates accepted duplicate or missing case sets")
	}
}

func closedLoopV4PromotionRows(cases []CaseEvidence) []closedLoopPromotionRow {
	rows := make([]closedLoopPromotionRow, 0, len(cases))
	for _, current := range cases {
		if current.Outcome != OutcomePass || current.PromotionHash == "" || current.ProjectHash == "" {
			continue
		}
		rows = append(rows, closedLoopPromotionRow{
			CaseID: current.Case.ID, Role: current.Case.Role, SynthesisHash: current.SynthesisHash,
			PromotionHash: current.PromotionHash, ProjectHash: current.ProjectHash,
			Status: ots.PhysicalPromotionPassed, CleanRootRuns: 2, ReplayIdentical: true,
		})
	}
	slices.SortFunc(rows, func(left, right closedLoopPromotionRow) int { return cmp.Compare(left.CaseID, right.CaseID) })
	return rows
}

func loadClosedLoopV4HistoricalImplementationSeal(t *testing.T) closedLoopV4ImplementationSeal {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V4_REVIEWED_IMPLEMENTATION.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V4_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV4ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	if seal.Schema != closedLoopV4ImplementationSealSchema || seal.Version != closedLoopV4BaselineVersion ||
		seal.SelectedCapability != "complete_topology" ||
		seal.Review != "prism_reviewed_no_actionable_findings" || len(seal.Artifacts) == 0 {
		t.Fatal("V4 reviewed implementation seal metadata is invalid")
	}
	if want, err := hashClosedLoopV4ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V4 reviewed implementation seal hash is invalid")
	}
	return seal
}

func loadClosedLoopV4CurrentImplementationSeal(t *testing.T) closedLoopV4ImplementationSeal {
	t.Helper()
	seal := loadClosedLoopV4HistoricalImplementationSeal(t)
	// This stricter loader is intentionally exclusive to the one-time updater:
	// it proves the live tree is exactly the reviewed implementation immediately
	// before held-out evidence can be revealed.
	for _, artifact := range seal.Artifacts {
		if corpusHash(mustCorpusRead(t, filepath.Join(closedLoopModuleRoot(t), filepath.FromSlash(artifact.Path)))) != artifact.SHA256 {
			t.Fatalf("V4 reviewed implementation artifact drifted: %s", artifact.Path)
		}
	}
	return seal
}

func refuseExistingClosedLoopV4Final(t *testing.T) {
	t.Helper()
	for _, path := range closedLoopV4FinalBlockerPaths(t) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V4 final is one-time and refuses existing artifact: %s", path)
		}
	}
}

func closedLoopV4FinalBlockerPaths(t *testing.T) []string {
	t.Helper()
	return []string{
		closedLoopV4FinalRoot,
		filepath.Join(closedLoopSpecDirectory(t), "V4_FINAL_REPORT.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_FINAL_REPORT.sha256"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_FINAL_COMPARISON.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_FINAL_COMPARISON.sha256"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_PROMOTION_MATRIX.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_PROMOTION_MATRIX.sha256"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_HELD_OUT_FINAL_SEAL.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V4_HELD_OUT_FINAL_SEAL.sha256"),
		filepath.Join(closedLoopSpecDirectory(t), closedLoopV4ValidationAuditFile),
	}
}

func writeClosedLoopV4Final(
	t *testing.T,
	discovery []CaseEvidence,
	report closedLoopV4FinalReport,
	comparison closedLoopV4FinalComparison,
	matrix closedLoopV4FinalPromotionMatrix,
	seal closedLoopV4HeldOutFinalSeal,
	ciphertext []byte,
) {
	t.Helper()
	root := filepath.Join(closedLoopV4FinalRoot, "discovery")
	if err := os.Mkdir(closedLoopV4FinalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, current := range discovery {
		if !portableArtifactID(current.Case.ID) {
			t.Fatal("V4 discovery case ID is not a safe artifact filename")
		}
		writeClosedLoopV4ExclusiveFile(t, filepath.Join(root, current.Case.ID+".json"), corpusJSON(t, current), 0o644)
	}
	writeClosedLoopV4ExclusiveFile(t, filepath.Join(closedLoopV4FinalRoot, closedLoopV4HeldOutFinalFile), ciphertext, 0o600)
	specRoot := closedLoopSpecDirectory(t)
	writeClosedLoopV4ArtifactExclusive(t, filepath.Join(specRoot, "V4_FINAL_REPORT.json"), report)
	writeClosedLoopV4ArtifactExclusive(t, filepath.Join(specRoot, "V4_FINAL_COMPARISON.json"), comparison)
	writeClosedLoopV4ArtifactExclusive(t, filepath.Join(specRoot, "V4_PROMOTION_MATRIX.json"), matrix)
	writeClosedLoopV4ArtifactExclusive(t, filepath.Join(specRoot, "V4_HELD_OUT_FINAL_SEAL.json"), seal)
	writeClosedLoopV4ValidationAudit(t, "complete", report.ImplementationSealHash)
}

func writeClosedLoopV4ArtifactExclusive(t *testing.T, path string, value any) {
	t.Helper()
	data := corpusJSON(t, value)
	writeClosedLoopV4ExclusiveFile(t, path, data, 0o644)
	checksumPath := strings.TrimSuffix(path, ".json") + ".sha256"
	checksum := []byte(corpusHash(data) + "  " + filepath.Base(path) + "\n")
	writeClosedLoopV4ExclusiveFile(t, checksumPath, checksum, 0o644)
}

func writeClosedLoopV4ValidationAudit(t *testing.T, status, implementationHash string) {
	t.Helper()
	if status != "complete" && status != "failed" {
		t.Fatal("V4 validation audit status is invalid")
	}
	body := "# V4 Validation Audit\n\n" +
		"- Status: " + status + "\n" +
		"- Held-out corpus consumed: yes\n" +
		"- Reviewed implementation seal: `" + implementationHash + "`\n" +
		"- Final updater: retired\n"
	writeClosedLoopV4ExclusiveFile(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV4ValidationAuditFile), []byte(body), 0o644)
}

func writeClosedLoopV4FailureAuditIfAbsent(t *testing.T, implementationHash string) {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), closedLoopV4ValidationAuditFile)
	if _, err := os.Stat(path); err == nil {
		return
	} else if !os.IsNotExist(err) {
		t.Errorf("V4 failure audit state is unreadable: %v", err)
		return
	}
	writeClosedLoopV4ValidationAudit(t, "failed", implementationHash)
}

func writeClosedLoopV4ExclusiveFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := publishClosedLoopV4AtomicNoReplace(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func publishClosedLoopV4AtomicNoReplace(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".v4-final-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(temporary)
	}()
	if err = file.Chmod(mode); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Link(temporary, path)
}

func TestPublishClosedLoopV4AtomicNoReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := publishClosedLoopV4AtomicNoReplace(path, []byte("complete"), 0o640); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "complete" {
		t.Fatalf("published V4 artifact = %q, %v", got, err)
	}
	if err := publishClosedLoopV4AtomicNoReplace(path, []byte("replacement"), 0o640); !os.IsExist(err) {
		t.Fatalf("V4 artifact replacement error = %v, want exists", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "complete" {
		t.Fatalf("V4 artifact changed after rejected replacement: %q, %v", got, err)
	}
}

func hashClosedLoopV4ImplementationSeal(value closedLoopV4ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV4FinalReport(value closedLoopV4FinalReport) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV4FinalComparison(value closedLoopV4FinalComparison) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV4FinalPromotionMatrix(value closedLoopV4FinalPromotionMatrix) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV4HeldOutFinalPayload(value closedLoopV4HeldOutFinalPayload) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV4HeldOutFinalSeal(value closedLoopV4HeldOutFinalSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}

func TestClosedLoopV4FinalHashesRejectMutation(t *testing.T) {
	comparison := closedLoopV4FinalComparison{Schema: closedLoopV4FinalComparisonSchema, Version: closedLoopV4BaselineVersion}
	first, err := hashClosedLoopV4FinalComparison(comparison)
	if err != nil {
		t.Fatal(err)
	}
	comparison.DiscoveryPassAfter++
	second, err := hashClosedLoopV4FinalComparison(comparison)
	if err != nil || first == second {
		t.Fatal("V4 final comparison hash did not bind mutation")
	}
}
