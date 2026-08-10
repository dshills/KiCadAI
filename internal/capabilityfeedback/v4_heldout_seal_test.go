package capabilityfeedback

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV4HeldOutPayloadSchema  = "kicadai.closed-loop-open-set-held-out-baseline-payload.v4"
	closedLoopV4HeldOutSealSchema     = "kicadai.closed-loop-open-set-held-out-baseline-seal.v4"
	closedLoopV4HeldOutBaselineKeyEnv = "KICADAI_V4_HELD_OUT_BASELINE_KEY_FILE"
	closedLoopV4HeldOutBaselineFile   = "held_out_baseline.sealed"
	closedLoopV4HeldOutBaselineUpdate = "UPDATE_CLOSED_LOOP_V4_HELD_OUT_BASELINE"
	closedLoopV4SelectionCommit       = "40431aa4f563eda2db6556af5aba7d417593a759"
)

type closedLoopV4HeldOutPayload struct {
	Schema                  string          `json:"schema"`
	Version                 int             `json:"version"`
	CorpusManifestHash      string          `json:"corpus_manifest_hash"`
	SelectionHash           string          `json:"selection_hash"`
	EvaluatorPolicy         string          `json:"evaluator_policy"`
	ImpactRegistryHash      string          `json:"impact_registry_hash"`
	SynthesisPolicyHash     string          `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash string          `json:"gap_transition_policy_sha256"`
	Cases                   []CaseEvidence  `json:"cases"`
	Aggregate               AggregateReport `json:"aggregate"`
	Hash                    string          `json:"hash"`
}

type closedLoopV4HeldOutSeal struct {
	Schema                  string `json:"schema"`
	Version                 int    `json:"version"`
	Algorithm               string `json:"algorithm"`
	CorpusManifestHash      string `json:"corpus_manifest_hash"`
	FreezeCommit            string `json:"freeze_commit"`
	SelectionHash           string `json:"selection_hash"`
	EvaluatorPolicy         string `json:"evaluator_policy"`
	ImpactRegistryHash      string `json:"impact_registry_hash"`
	SynthesisPolicyHash     string `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash string `json:"gap_transition_policy_sha256"`
	PayloadHash             string `json:"payload_hash"`
	CiphertextHash          string `json:"ciphertext_sha256"`
	CaseCount               int    `json:"case_count"`
	Hash                    string `json:"hash"`
}

func TestClosedLoopV4HeldOutBaselineSealIsFrozen(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	manifest := loadClosedLoopV4Manifest(t)
	selection := loadFrozenClosedLoopV4Selection(t, manifest)
	specRoot := closedLoopSpecDirectory(t)
	sealBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_HELD_OUT_BASELINE_SEAL.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_HELD_OUT_BASELINE_SEAL.sha256"), "V4_HELD_OUT_BASELINE_SEAL.json", sealBytes)
	var seal closedLoopV4HeldOutSeal
	decodeCorpusStrict(t, sealBytes, &seal)
	if seal.Schema != closedLoopV4HeldOutSealSchema || seal.Version != closedLoopV4BaselineVersion ||
		seal.Algorithm != closedLoopV4HeldOutSealAlgorithm || seal.CorpusManifestHash != corpusHash(manifestBytes) ||
		seal.FreezeCommit != closedLoopV4SelectionCommit || seal.SelectionHash != selection.Hash ||
		seal.EvaluatorPolicy != manifest.EvaluatorPolicy || seal.ImpactRegistryHash != manifest.ImpactRegistryHash ||
		seal.SynthesisPolicyHash != manifest.SynthesisPolicyHash || seal.GapTransitionPolicyHash != manifest.GapTransitionPolicyHash ||
		seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V4 held-out baseline seal metadata drifted from the frozen contract")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV4BaselineRoot, closedLoopV4HeldOutBaselineFile))
	if corpusHash(ciphertext) != seal.CiphertextHash {
		t.Fatal("V4 held-out baseline ciphertext hash drifted")
	}
	wantHash, err := hashClosedLoopV4HeldOutSeal(seal)
	if err != nil || seal.Hash != wantHash {
		t.Fatal("V4 held-out baseline seal content hash drifted")
	}
}

func TestUpdateClosedLoopV4HeldOutBaselineSeal(t *testing.T) {
	if os.Getenv(closedLoopV4HeldOutBaselineUpdate) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V4_HELD_OUT_BASELINE=1 to seal the untouched V4 held-out baseline")
	}
	baselineKeyPath := os.Getenv(closedLoopV4HeldOutBaselineKeyEnv)
	sourceKeyPath := os.Getenv(closedLoopV4HeldOutCorpusKeyEnv)
	if baselineKeyPath == "" || sourceKeyPath == "" {
		t.Fatal("separate external V4 source and baseline key paths are required")
	}
	closedLoopV4RequireExternalKeyPath(t, baselineKeyPath)
	closedLoopV4RequireExternalKeyPath(t, sourceKeyPath)
	if baselineKeyPath == sourceKeyPath {
		t.Fatal("V4 held-out source and baseline keys must be separate")
	}
	specRoot := closedLoopSpecDirectory(t)
	ciphertextPath := filepath.Join(closedLoopV4BaselineRoot, closedLoopV4HeldOutBaselineFile)
	sealPath := filepath.Join(specRoot, "V4_HELD_OUT_BASELINE_SEAL.json")
	checksumPath := strings.TrimSuffix(sealPath, filepath.Ext(sealPath)) + ".sha256"
	for _, path := range []string{ciphertextPath, sealPath, checksumPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V4 held-out baseline artifact already exists; refusing overwrite: %s", filepath.Base(path))
		}
	}

	manifest := loadClosedLoopV4Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	selection := loadFrozenClosedLoopV4Selection(t, manifest)
	registry, policy := closedLoopV4Policies(t)
	requirements := loadClosedLoopV4HeldOutRequirements(t, manifest, sourceKeyPath)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV4HeldOutBaseline(t, manifest, requirements, policy, inventory, environment)
	aggregate, err := EvaluateRealizabilityAware(RoleHeldOut, cases, registry)
	if err != nil {
		t.Fatal("sealed V4 held-out aggregation failed closed")
	}
	if len(aggregate.Clusters) != 0 {
		t.Fatal("V4 held-out evidence produced rankable clusters")
	}
	payload := closedLoopV4HeldOutPayload{
		Schema: closedLoopV4HeldOutPayloadSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: corpusHash(manifestBytes), SelectionHash: selection.Hash,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, GapTransitionPolicyHash: manifest.GapTransitionPolicyHash,
		Cases: cases, Aggregate: aggregate,
	}
	payloadHash, err := hashClosedLoopV4HeldOutPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Hash = payloadHash
	plaintext := corpusJSON(t, payload)
	baselineKey := closedLoopV4LoadOrCreateKey(t, baselineKeyPath)
	ciphertext := sealClosedLoopV4HeldOutPayload(t, baselineKey, payload, plaintext)
	if err := writeClosedLoopV4HeldOutCiphertext(ciphertextPath, ciphertext); err != nil {
		t.Fatal("write V4 held-out baseline ciphertext")
	}
	seal := closedLoopV4HeldOutSeal{
		Schema: closedLoopV4HeldOutSealSchema, Version: closedLoopV4BaselineVersion,
		Algorithm: closedLoopV4HeldOutSealAlgorithm, CorpusManifestHash: payload.CorpusManifestHash,
		FreezeCommit: closedLoopV4SelectionCommit, SelectionHash: selection.Hash,
		EvaluatorPolicy: payload.EvaluatorPolicy, ImpactRegistryHash: payload.ImpactRegistryHash,
		SynthesisPolicyHash: payload.SynthesisPolicyHash, GapTransitionPolicyHash: payload.GapTransitionPolicyHash,
		PayloadHash: payload.Hash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(cases),
	}
	sealHash, err := hashClosedLoopV4HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.Hash = sealHash
	writeClosedLoopArtifact(t, sealPath, seal)
}

func loadFrozenClosedLoopV4Selection(t *testing.T, manifest closedLoopV4CorpusManifest) closedLoopV4Selection {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "V4_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V4_SELECTION.sha256"), "V4_SELECTION.json", selectionBytes)
	var selection closedLoopV4Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	registry, _ := closedLoopV4Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, loadClosedLoopV4DiscoveryCases(t, manifest), registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	wantReport := buildClosedLoopV4DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	wantSelection := buildClosedLoopV4Selection(t, wantReport)
	if !bytes.Equal(selectionBytes, corpusJSON(t, wantSelection)) {
		t.Fatal("V4 held-out execution refused because discovery selection drifted")
	}
	return selection
}

func loadClosedLoopV4HeldOutRequirements(t *testing.T, manifest closedLoopV4CorpusManifest, sourceKeyPath string) map[string]ots.Requirement {
	t.Helper()
	key, err := os.ReadFile(sourceKeyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V4 held-out source key is invalid")
	}
	seal := manifest.HeldOutSourceSeal
	plaintext, err := closedLoopV4OpenHeldOutSource(
		key, manifest.AuthorManifestHash, seal.PayloadHash,
		mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, seal.File)),
	)
	if err != nil || corpusHash(plaintext) != seal.PayloadHash {
		t.Fatal("V4 held-out source authentication failed")
	}
	var payload closedLoopV4HeldOutSourcePayload
	decodeCorpusStrict(t, plaintext, &payload)
	if payload.Schema != closedLoopV4HeldOutSourceSchema || payload.Version != closedLoopV4CorpusVersion ||
		payload.AuthorManifestHash != manifest.AuthorManifestHash || len(payload.Cases) != seal.CaseCount {
		t.Fatal("V4 held-out source payload contract is invalid")
	}
	heldOutEntries := make(map[string]closedLoopV4CorpusEntry, seal.CaseCount)
	for _, entry := range manifest.Entries {
		if entry.Role == RoleHeldOut {
			if _, exists := heldOutEntries[entry.ID]; exists {
				t.Fatal("V4 held-out manifest contains a duplicate identity")
			}
			heldOutEntries[entry.ID] = entry
		}
	}
	if len(heldOutEntries) != len(payload.Cases) {
		t.Fatal("V4 held-out source count differs from the manifest")
	}
	result := make(map[string]ots.Requirement, len(payload.Cases))
	for _, candidate := range payload.Cases {
		entry, ok := heldOutEntries[candidate.ID]
		if !ok {
			t.Fatal("V4 held-out source identity is absent from the manifest")
		}
		if candidate.ID != entry.ID || candidate.RequirementFile != entry.RequirementFile ||
			candidate.RequirementHash != entry.RequirementHash || candidate.SemanticHash != entry.SemanticHash ||
			corpusHash(candidate.Requirement) != entry.RequirementHash {
			t.Fatal("V4 held-out source requirement commitment is invalid")
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(candidate.Requirement))
		if len(issues) != 0 {
			t.Fatal("sealed V4 held-out requirement failed strict decode")
		}
		result[entry.ID] = requirement
		delete(heldOutEntries, entry.ID)
	}
	if len(heldOutEntries) != 0 {
		t.Fatal("V4 held-out source omitted a manifest identity")
	}
	return result
}

func runClosedLoopV4HeldOutBaseline(
	t *testing.T,
	manifest closedLoopV4CorpusManifest,
	requirements map[string]ots.Requirement,
	policy ots.Policy,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) []CaseEvidence {
	t.Helper()
	results := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleHeldOut {
			continue
		}
		requirement, ok := requirements[entry.ID]
		if !ok {
			t.Fatal("sealed V4 held-out requirement is missing")
		}
		first := runClosedLoopV4SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV4SealedSynthesis(t, requirement, inventory, environment, policy)
		// Canonical JSON is the normative normalized synthesis evidence. It
		// deliberately excludes unexported runtime caches while retaining every
		// exported electrical value and decision input; any exported volatility
		// is therefore a replay-contract failure, not a field to ignore.
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			// Do not print a diff: held-out outcomes, gaps, and diagnostics must
			// remain unavailable to the implementation context even on failure.
			t.Fatal("sealed V4 held-out synthesis replay failed closed")
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, "sealed-held-out", first, environment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatal("sealed V4 held-out physical promotion failed closed")
			}
			promotion = &current
		}
		evidence, err := ObserveRealizabilityAware(CaseMeta{
			ID: entry.ID, Role: entry.Role,
			Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact),
		}, requirement, first, promotion)
		if err != nil {
			t.Fatal("sealed V4 held-out observation failed closed")
		}
		results = append(results, evidence)
	}
	if len(results) != closedLoopCorpusSize/2 {
		t.Fatal("sealed V4 held-out baseline did not produce the required case count")
	}
	return results
}

func runClosedLoopV4SealedSynthesis(
	t *testing.T,
	requirement ots.Requirement,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
	policy ots.Policy,
) ots.SynthesisRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	run := ots.Synthesize(ctx, requirement, inventory, environment, policy)
	if run.Report.Status == ots.StatusInvalid || run.Report.Status == ots.StatusCanceled {
		t.Fatal("sealed V4 held-out synthesis aborted; baseline was not recorded")
	}
	return run
}

func writeClosedLoopV4HeldOutCiphertext(path string, ciphertext []byte) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create exclusive V4 held-out ciphertext staging file: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(temporary)
	}
	if _, err := file.Write(ciphertext); err != nil {
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	// Link is the no-replace publication primitive: unlike Rename on Unix,
	// it fails atomically if another process created the frozen destination.
	// The staging path is in the destination directory, so it is necessarily
	// on the same filesystem and cannot encounter a cross-device link.
	if err := os.Link(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	_ = os.Remove(temporary)
	return nil
}

func TestWriteClosedLoopV4HeldOutCiphertextIsAtomicAndNoReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), closedLoopV4HeldOutBaselineFile)
	want := []byte("authenticated ciphertext")
	if err := writeClosedLoopV4HeldOutCiphertext(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("V4 held-out ciphertext publication changed the bytes")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("V4 held-out ciphertext publication changed the private mode")
	}
	if err := writeClosedLoopV4HeldOutCiphertext(path, []byte("replacement")); err == nil {
		t.Fatal("V4 held-out ciphertext publication overwrote a frozen artifact")
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("V4 held-out ciphertext changed after refused overwrite")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("V4 held-out ciphertext staging file leaked")
	}
}

func sealClosedLoopV4HeldOutPayload(t *testing.T, key []byte, payload closedLoopV4HeldOutPayload, plaintext []byte) []byte {
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
		t.Fatal("generate V4 held-out baseline nonce")
	}
	additionalData, err := closedLoopV4HeldOutAdditionalData(payload)
	if err != nil {
		t.Fatal(err)
	}
	return gcm.Seal(nonce, nonce, plaintext, additionalData)
}

func openClosedLoopV4HeldOutPayload(key []byte, payload closedLoopV4HeldOutPayload, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("V4 held-out baseline ciphertext is truncated")
	}
	additionalData, err := closedLoopV4HeldOutAdditionalData(payload)
	if err != nil {
		return nil, err
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, sealed, additionalData)
}

func closedLoopV4HeldOutAdditionalData(payload closedLoopV4HeldOutPayload) ([]byte, error) {
	fields := []string{
		payload.Schema, payload.CorpusManifestHash, payload.SelectionHash,
		payload.EvaluatorPolicy, payload.ImpactRegistryHash, payload.SynthesisPolicyHash,
		payload.GapTransitionPolicyHash, payload.Hash,
	}
	for _, field := range fields {
		if strings.ContainsRune(field, '\x00') {
			return nil, fmt.Errorf("V4 held-out authenticated metadata contains a forbidden NUL")
		}
	}
	return []byte(payload.Schema + "\x00" + payload.CorpusManifestHash + "\x00" + payload.SelectionHash + "\x00" +
		payload.EvaluatorPolicy + "\x00" + payload.ImpactRegistryHash + "\x00" + payload.SynthesisPolicyHash + "\x00" +
		payload.GapTransitionPolicyHash + "\x00" + payload.Hash), nil
}

func hashClosedLoopV4HeldOutPayload(payload closedLoopV4HeldOutPayload) (string, error) {
	payload.Hash = ""
	return digest(payload)
}

func hashClosedLoopV4HeldOutSeal(seal closedLoopV4HeldOutSeal) (string, error) {
	seal.Hash = ""
	return digest(seal)
}

func TestClosedLoopV4HeldOutSealUsesAuthenticatedEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x4d}, 32)
	payload := closedLoopV4HeldOutPayload{
		Schema:             closedLoopV4HeldOutPayloadSchema,
		CorpusManifestHash: fmt.Sprintf("%064d", 1), SelectionHash: fmt.Sprintf("%064d", 2),
		EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistryHash: fmt.Sprintf("%064d", 3),
		SynthesisPolicyHash: fmt.Sprintf("%064d", 4), GapTransitionPolicyHash: fmt.Sprintf("%064d", 5),
	}
	plaintext := []byte("sealed V4 held-out evidence")
	payload.Hash = corpusHash(plaintext)
	first := sealClosedLoopV4HeldOutPayload(t, key, payload, plaintext)
	second := sealClosedLoopV4HeldOutPayload(t, key, payload, plaintext)
	if bytes.Equal(first, second) || bytes.Contains(first, plaintext) || bytes.Contains(second, plaintext) {
		t.Fatal("V4 held-out baseline seal reused a nonce or exposed plaintext")
	}
	for _, ciphertext := range [][]byte{first, second} {
		opened, err := openClosedLoopV4HeldOutPayload(key, payload, ciphertext)
		if err != nil || !bytes.Equal(opened, plaintext) {
			t.Fatal("V4 held-out baseline seal did not authenticate and decrypt")
		}
	}
	tampered := bytes.Clone(first)
	tampered[len(tampered)-1] ^= 1
	if _, err := openClosedLoopV4HeldOutPayload(key, payload, tampered); err == nil {
		t.Fatal("V4 held-out baseline seal accepted tampered ciphertext")
	}
	mutated := payload
	mutated.GapTransitionPolicyHash = fmt.Sprintf("%064d", 6)
	if _, err := openClosedLoopV4HeldOutPayload(key, mutated, first); err == nil {
		t.Fatal("V4 held-out baseline seal accepted mutated authenticated metadata")
	}
	mutated = payload
	mutated.EvaluatorPolicy += "\x00injected"
	if _, err := openClosedLoopV4HeldOutPayload(key, mutated, first); err == nil {
		t.Fatal("V4 held-out baseline seal accepted ambiguous authenticated metadata")
	}
}

func TestClosedLoopV4HeldOutSealHashRejectsMutation(t *testing.T) {
	seal := closedLoopV4HeldOutSeal{
		Schema: closedLoopV4HeldOutSealSchema, Version: closedLoopV4BaselineVersion,
		CaseCount: closedLoopCorpusSize / 2,
	}
	hash, err := hashClosedLoopV4HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.CaseCount--
	mutated, err := hashClosedLoopV4HeldOutSeal(seal)
	if err != nil || mutated == hash {
		t.Fatal("V4 held-out baseline seal mutation did not change its content hash")
	}
}

func TestClosedLoopV4HeldOutBaselineDecryptsWithExternalKey(t *testing.T) {
	keyPath := os.Getenv(closedLoopV4HeldOutBaselineKeyEnv)
	if keyPath == "" {
		t.Skip("external V4 held-out baseline key is unavailable")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V4 held-out baseline key is invalid")
	}
	var seal closedLoopV4HeldOutSeal
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V4_HELD_OUT_BASELINE_SEAL.json")), &seal)
	payloadCommitment := closedLoopV4HeldOutPayload{
		Schema: closedLoopV4HeldOutPayloadSchema, Version: closedLoopV4BaselineVersion,
		CorpusManifestHash: seal.CorpusManifestHash, SelectionHash: seal.SelectionHash,
		EvaluatorPolicy: seal.EvaluatorPolicy, ImpactRegistryHash: seal.ImpactRegistryHash,
		SynthesisPolicyHash: seal.SynthesisPolicyHash, GapTransitionPolicyHash: seal.GapTransitionPolicyHash,
		Hash: seal.PayloadHash,
	}
	plaintext, err := openClosedLoopV4HeldOutPayload(
		key, payloadCommitment,
		mustCorpusRead(t, filepath.Join(closedLoopV4BaselineRoot, closedLoopV4HeldOutBaselineFile)),
	)
	if err != nil || corpusHash(plaintext) == "" {
		t.Fatal("V4 held-out baseline authentication failed")
	}
	var payload closedLoopV4HeldOutPayload
	decodeCorpusStrict(t, plaintext, &payload)
	wantHash, err := hashClosedLoopV4HeldOutPayload(payload)
	if err != nil || payload.Schema != closedLoopV4HeldOutPayloadSchema || payload.Version != closedLoopV4BaselineVersion ||
		payload.CorpusManifestHash != seal.CorpusManifestHash || payload.SelectionHash != seal.SelectionHash ||
		payload.EvaluatorPolicy != seal.EvaluatorPolicy || payload.ImpactRegistryHash != seal.ImpactRegistryHash ||
		payload.SynthesisPolicyHash != seal.SynthesisPolicyHash || payload.GapTransitionPolicyHash != seal.GapTransitionPolicyHash ||
		payload.Hash != seal.PayloadHash || payload.Hash != wantHash || len(payload.Cases) != seal.CaseCount ||
		payload.Aggregate.CorpusRole != RoleHeldOut || len(payload.Aggregate.Clusters) != 0 {
		t.Fatal("V4 held-out baseline payload contract is invalid")
	}
	for _, current := range payload.Cases {
		if current.PolicyVersion != RealizabilityPolicyVersion || current.Case.Role != RoleHeldOut {
			t.Fatal("V4 held-out baseline case metadata is invalid")
		}
	}
}
