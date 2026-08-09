package capabilityfeedback

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV2HeldOutPayloadSchema = "kicadai.closed-loop-open-set-held-out-baseline-payload.v2"
	closedLoopV2HeldOutSealSchema    = "kicadai.closed-loop-open-set-held-out-baseline-seal.v2"
	closedLoopV2HeldOutSealAlgorithm = "AES-256-GCM/HMAC-SHA-256-payload-bound-nonce"
	closedLoopV2HeldOutSealKeyEnv    = "KICADAI_V2_HELD_OUT_SEAL_KEY_FILE"
	closedLoopV2HeldOutSealFile      = "held_out_baseline.sealed"
	closedLoopV2HeldOutSealFrozen    = false
)

type closedLoopV2HeldOutPayload struct {
	Schema             string          `json:"schema"`
	Version            int             `json:"version"`
	CorpusManifestHash string          `json:"corpus_manifest_hash"`
	SelectionHash      string          `json:"selection_hash"`
	Cases              []CaseEvidence  `json:"cases"`
	Aggregate          AggregateReport `json:"aggregate"`
	Hash               string          `json:"hash"`
}

type closedLoopV2HeldOutSeal struct {
	Schema             string `json:"schema"`
	Version            int    `json:"version"`
	Algorithm          string `json:"algorithm"`
	CorpusManifestHash string `json:"corpus_manifest_hash"`
	FreezeCommit       string `json:"freeze_commit"`
	SelectionHash      string `json:"selection_hash"`
	PayloadHash        string `json:"payload_hash"`
	CiphertextHash     string `json:"ciphertext_sha256"`
	CaseCount          int    `json:"case_count"`
	Hash               string `json:"hash"`
}

func TestClosedLoopV2HeldOutBaselineSealIsFrozen(t *testing.T) {
	if !closedLoopV2HeldOutSealFrozen {
		t.Skip("V2 held-out baseline has not been sealed yet")
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	selectionBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V2_SELECTION.json"))
	var selection closedLoopV2Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	sealBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V2_HELD_OUT_BASELINE_SEAL.json"))
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V2_HELD_OUT_BASELINE_SEAL.sha256"), "V2_HELD_OUT_BASELINE_SEAL.json", sealBytes)
	var seal closedLoopV2HeldOutSeal
	decodeCorpusStrict(t, sealBytes, &seal)
	if seal.Schema != closedLoopV2HeldOutSealSchema || seal.Version != closedLoopV2BaselineVersion ||
		seal.Algorithm != closedLoopV2HeldOutSealAlgorithm || seal.CorpusManifestHash != corpusHash(manifestBytes) ||
		seal.FreezeCommit != closedLoopV2CorpusFreezeCommit || seal.SelectionHash != selection.Hash ||
		seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V2 held-out seal metadata drifted from the frozen baseline contract")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV2BaselineRoot, closedLoopV2HeldOutSealFile))
	if corpusHash(ciphertext) != seal.CiphertextHash {
		t.Fatal("V2 held-out ciphertext hash drifted")
	}
	wantHash, err := hashClosedLoopV2HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	if seal.Hash != wantHash {
		t.Fatal("V2 held-out seal content hash drifted")
	}
}

func TestUpdateClosedLoopV2HeldOutBaselineSeal(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_V2_HELD_OUT_BASELINE") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V2_HELD_OUT_BASELINE=1 to run and seal the untouched held-out baseline")
	}
	keyPath := os.Getenv(closedLoopV2HeldOutSealKeyEnv)
	if keyPath == "" {
		t.Fatalf("%s is required", closedLoopV2HeldOutSealKeyEnv)
	}
	key := loadOrCreateClosedLoopV2SealKey(t, keyPath)
	manifest := loadClosedLoopV2Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	selection := loadFrozenClosedLoopV2Selection(t, manifest)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV2HeldOutBaseline(t, manifest, inventory, environment)
	aggregate, err := Evaluate(RoleHeldOut, cases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal("sealed held-out aggregation failed closed")
	}
	if len(aggregate.Clusters) != 0 {
		t.Fatal("held-out evidence produced rankable clusters")
	}
	payload := closedLoopV2HeldOutPayload{
		Schema: closedLoopV2HeldOutPayloadSchema, Version: closedLoopV2BaselineVersion,
		CorpusManifestHash: corpusHash(manifestBytes), SelectionHash: selection.Hash,
		Cases: cases, Aggregate: aggregate,
	}
	payloadHash, err := hashClosedLoopV2HeldOutPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Hash = payloadHash
	plaintext := corpusJSON(t, payload)
	ciphertext := sealClosedLoopV2HeldOutPayload(t, key, payload.CorpusManifestHash, selection.Hash, payload.Hash, plaintext)
	if err := os.MkdirAll(closedLoopV2BaselineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(closedLoopV2BaselineRoot, closedLoopV2HeldOutSealFile), ciphertext, 0o644); err != nil {
		t.Fatal(err)
	}
	seal := closedLoopV2HeldOutSeal{
		Schema: closedLoopV2HeldOutSealSchema, Version: closedLoopV2BaselineVersion,
		Algorithm: closedLoopV2HeldOutSealAlgorithm, CorpusManifestHash: payload.CorpusManifestHash,
		FreezeCommit: closedLoopV2CorpusFreezeCommit, SelectionHash: selection.Hash,
		PayloadHash: payload.Hash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(cases),
	}
	sealHash, err := hashClosedLoopV2HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.Hash = sealHash
	writeClosedLoopArtifact(t, filepath.Join(closedLoopSpecDirectory(t), "V2_HELD_OUT_BASELINE_SEAL.json"), seal)
}

func loadFrozenClosedLoopV2Selection(t *testing.T, manifest closedLoopV2Manifest) closedLoopV2Selection {
	t.Helper()
	selectionBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V2_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V2_SELECTION.sha256"), "V2_SELECTION.json", selectionBytes)
	var selection closedLoopV2Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	discovery := loadClosedLoopV2DiscoveryCases(t, manifest)
	report, err := Evaluate(RoleDiscovery, discovery, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(report)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	wantReport := buildClosedLoopV2DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, report, plan)
	wantSelection := buildClosedLoopV2Selection(t, wantReport)
	if !bytes.Equal(selectionBytes, corpusJSON(t, wantSelection)) {
		t.Fatal("V2 held-out execution refused because discovery selection drifted")
	}
	return selection
}

func runClosedLoopV2HeldOutBaseline(
	t *testing.T,
	manifest closedLoopV2Manifest,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) []CaseEvidence {
	t.Helper()
	results := make([]CaseEvidence, 0, closedLoopCorpusSize/2)
	for _, entry := range manifest.Entries {
		if entry.Role != RoleHeldOut {
			continue
		}
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatal("sealed held-out requirement failed strict decode")
		}
		first := runClosedLoopV2SealedSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		second := runClosedLoopV2SealedSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatal("sealed held-out synthesis replay failed closed")
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			promotion = &current
		}
		evidence, err := Observe(CaseMeta{ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact}, requirement, first, promotion)
		if err != nil {
			t.Fatal("sealed held-out observation failed closed")
		}
		results = append(results, evidence)
	}
	if len(results) != closedLoopCorpusSize/2 {
		t.Fatal("sealed held-out baseline did not produce the required case count")
	}
	return results
}

func runClosedLoopV2SealedSynthesis(
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
		t.Fatal("sealed held-out synthesis aborted; baseline was not recorded")
	}
	return run
}

func loadOrCreateClosedLoopV2SealKey(t *testing.T, path string) []byte {
	t.Helper()
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			t.Fatal("V2 held-out seal key has invalid length")
		}
		return key
	}
	if !os.IsNotExist(err) {
		t.Fatal("V2 held-out seal key is unavailable")
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("generate V2 held-out seal key")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal("create V2 held-out seal key directory")
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal("persist V2 held-out seal key outside the repository")
	}
	return key
}

func sealClosedLoopV2HeldOutPayload(t *testing.T, key []byte, manifestHash, selectionHash, payloadHash string, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(manifestHash + "\x00" + selectionHash + "\x00" + payloadHash)); err != nil {
		t.Fatal(err)
	}
	nonce := mac.Sum(nil)[:gcm.NonceSize()]
	additionalData := []byte(closedLoopV2HeldOutPayloadSchema + "\x00" + manifestHash + "\x00" + selectionHash + "\x00" + payloadHash)
	return gcm.Seal(nil, nonce, plaintext, additionalData)
}

func hashClosedLoopV2HeldOutPayload(payload closedLoopV2HeldOutPayload) (string, error) {
	payload.Hash = ""
	return digest(payload)
}

func hashClosedLoopV2HeldOutSeal(seal closedLoopV2HeldOutSeal) (string, error) {
	seal.Hash = ""
	return digest(seal)
}

func TestClosedLoopV2HeldOutSealHashRejectsMutation(t *testing.T) {
	seal := closedLoopV2HeldOutSeal{Schema: closedLoopV2HeldOutSealSchema, Version: closedLoopV2BaselineVersion, CaseCount: closedLoopCorpusSize / 2}
	hash, err := hashClosedLoopV2HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.CaseCount--
	mutated, err := hashClosedLoopV2HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	if mutated == hash {
		t.Fatal("V2 held-out seal mutation did not change its content hash")
	}
}

func TestClosedLoopV2HeldOutSealUsesAuthenticatedEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x5a}, 32)
	manifestHash, selectionHash := fmt.Sprintf("%064d", 1), fmt.Sprintf("%064d", 2)
	plaintext := []byte("sealed evidence")
	payloadHash := corpusHash(plaintext)
	first := sealClosedLoopV2HeldOutPayload(t, key, manifestHash, selectionHash, payloadHash, plaintext)
	second := sealClosedLoopV2HeldOutPayload(t, key, manifestHash, selectionHash, payloadHash, plaintext)
	if !bytes.Equal(first, second) || bytes.Contains(first, []byte("sealed evidence")) {
		t.Fatal("V2 held-out seal is nondeterministic or contains plaintext")
	}
	changedPlaintext := []byte("changed sealed evidence")
	changed := sealClosedLoopV2HeldOutPayload(t, key, manifestHash, selectionHash, corpusHash(changedPlaintext), changedPlaintext)
	if bytes.Equal(first, changed) {
		t.Fatal("changed V2 held-out payload reused deterministic ciphertext")
	}
}
