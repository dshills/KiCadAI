package capabilityfeedback

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV3HeldOutPayloadSchema  = "kicadai.closed-loop-open-set-held-out-baseline-payload.v3"
	closedLoopV3HeldOutSealSchema     = "kicadai.closed-loop-open-set-held-out-baseline-seal.v3"
	closedLoopV3HeldOutBaselineKeyEnv = "KICADAI_V3_HELD_OUT_BASELINE_KEY_FILE"
	closedLoopV3HeldOutBaselineFile   = "held_out_baseline.sealed"
	closedLoopV3HeldOutBaselineUpdate = "UPDATE_CLOSED_LOOP_V3_HELD_OUT_BASELINE"
	closedLoopV3SelectionCommit       = "99d1c33a9ad6a2967a110935ff69999f81193835"
)

type closedLoopV3HeldOutPayload struct {
	Schema             string          `json:"schema"`
	Version            int             `json:"version"`
	CorpusManifestHash string          `json:"corpus_manifest_hash"`
	SelectionHash      string          `json:"selection_hash"`
	Cases              []CaseEvidence  `json:"cases"`
	Aggregate          AggregateReport `json:"aggregate"`
	Hash               string          `json:"hash"`
}

type closedLoopV3HeldOutSeal struct {
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

func TestClosedLoopV3HeldOutBaselineSealIsFrozen(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	selection := loadFrozenClosedLoopV3Selection(t, loadClosedLoopV3Manifest(t))
	specRoot := closedLoopSpecDirectory(t)
	sealBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_HELD_OUT_BASELINE_SEAL.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_HELD_OUT_BASELINE_SEAL.sha256"), "V3_HELD_OUT_BASELINE_SEAL.json", sealBytes)
	var seal closedLoopV3HeldOutSeal
	decodeCorpusStrict(t, sealBytes, &seal)
	if seal.Schema != closedLoopV3HeldOutSealSchema || seal.Version != closedLoopV3BaselineVersion ||
		seal.Algorithm != closedLoopV3HeldOutSealAlgorithm || seal.CorpusManifestHash != corpusHash(manifestBytes) ||
		seal.FreezeCommit != closedLoopV3SelectionCommit || seal.SelectionHash != selection.Hash ||
		seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V3 held-out baseline seal metadata drifted from the frozen contract")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV3BaselineRoot, closedLoopV3HeldOutBaselineFile))
	if corpusHash(ciphertext) != seal.CiphertextHash {
		t.Fatal("V3 held-out baseline ciphertext hash drifted")
	}
	wantHash, err := hashClosedLoopV3HeldOutSeal(seal)
	if err != nil || seal.Hash != wantHash {
		t.Fatal("V3 held-out baseline seal content hash drifted")
	}
}

func TestUpdateClosedLoopV3HeldOutBaselineSeal(t *testing.T) {
	if os.Getenv(closedLoopV3HeldOutBaselineUpdate) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V3_HELD_OUT_BASELINE=1 to run and seal the untouched V3 held-out baseline")
	}
	baselineKeyPath := os.Getenv(closedLoopV3HeldOutBaselineKeyEnv)
	sourceKeyPath := os.Getenv(closedLoopV3HeldOutCorpusKeyEnv)
	if baselineKeyPath == "" || sourceKeyPath == "" {
		t.Fatal("separate external V3 source and baseline key paths are required")
	}
	closedLoopV3RequireExternalKeyPath(t, baselineKeyPath)
	closedLoopV3RequireExternalKeyPath(t, sourceKeyPath)
	if baselineKeyPath == sourceKeyPath {
		t.Fatal("V3 held-out source and baseline keys must be separate")
	}
	specRoot := closedLoopSpecDirectory(t)
	for _, path := range []string{
		filepath.Join(closedLoopV3BaselineRoot, closedLoopV3HeldOutBaselineFile),
		filepath.Join(specRoot, "V3_HELD_OUT_BASELINE_SEAL.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V3 held-out baseline artifact already exists; refusing overwrite: %s", filepath.Base(path))
		}
	}

	manifest := loadClosedLoopV3Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	selection := loadFrozenClosedLoopV3Selection(t, manifest)
	registry, policy := closedLoopV3Policies(t)
	heldOutRequirements := loadClosedLoopV3HeldOutRequirements(t, manifest, sourceKeyPath)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	cases := runClosedLoopV3HeldOutBaseline(t, manifest, heldOutRequirements, policy, inventory, environment)
	aggregate, err := EvaluateRealizabilityAware(RoleHeldOut, cases, registry)
	if err != nil {
		t.Fatal("sealed V3 held-out aggregation failed closed")
	}
	if len(aggregate.Clusters) != 0 {
		t.Fatal("V3 held-out evidence produced rankable clusters")
	}
	payload := closedLoopV3HeldOutPayload{
		Schema: closedLoopV3HeldOutPayloadSchema, Version: closedLoopV3BaselineVersion,
		CorpusManifestHash: corpusHash(manifestBytes), SelectionHash: selection.Hash,
		Cases: cases, Aggregate: aggregate,
	}
	payloadHash, err := hashClosedLoopV3HeldOutPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload.Hash = payloadHash
	plaintext := corpusJSON(t, payload)
	baselineKey := closedLoopV3LoadOrCreateKey(t, baselineKeyPath)
	ciphertext := sealClosedLoopV3HeldOutPayload(t, baselineKey, payload.CorpusManifestHash, selection.Hash, payload.Hash, plaintext)
	if err := os.WriteFile(filepath.Join(closedLoopV3BaselineRoot, closedLoopV3HeldOutBaselineFile), ciphertext, 0o600); err != nil {
		t.Fatal("write V3 held-out baseline ciphertext")
	}
	seal := closedLoopV3HeldOutSeal{
		Schema: closedLoopV3HeldOutSealSchema, Version: closedLoopV3BaselineVersion,
		Algorithm: closedLoopV3HeldOutSealAlgorithm, CorpusManifestHash: payload.CorpusManifestHash,
		FreezeCommit: closedLoopV3SelectionCommit, SelectionHash: selection.Hash,
		PayloadHash: payload.Hash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(cases),
	}
	sealHash, err := hashClosedLoopV3HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.Hash = sealHash
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_HELD_OUT_BASELINE_SEAL.json"), seal)
}

func loadFrozenClosedLoopV3Selection(t *testing.T, manifest closedLoopV3CorpusManifest) closedLoopV3Selection {
	t.Helper()
	selectionBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V3_SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V3_SELECTION.sha256"), "V3_SELECTION.json", selectionBytes)
	var selection closedLoopV3Selection
	decodeCorpusStrict(t, selectionBytes, &selection)
	registry, _ := closedLoopV3Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, loadClosedLoopV3DiscoveryCases(t, manifest), registry)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRankOneExpansionPlan(discovery)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	wantReport := buildClosedLoopV3DiscoveryBaselineReport(t, corpusHash(manifestBytes), manifest, discovery, plan)
	wantSelection := buildClosedLoopV3Selection(t, wantReport)
	if !bytes.Equal(selectionBytes, corpusJSON(t, wantSelection)) {
		t.Fatal("V3 held-out execution refused because discovery selection drifted")
	}
	return selection
}

func loadClosedLoopV3HeldOutRequirements(t *testing.T, manifest closedLoopV3CorpusManifest, sourceKeyPath string) map[string]ots.Requirement {
	t.Helper()
	key, err := os.ReadFile(sourceKeyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V3 held-out source key is invalid")
	}
	seal := manifest.HeldOutSourceSeal
	plaintext, err := closedLoopV3OpenHeldOutSource(
		key, manifest.AuthorManifestHash, seal.PayloadHash,
		mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, seal.File)),
	)
	if err != nil || corpusHash(plaintext) != seal.PayloadHash {
		t.Fatal("V3 held-out source authentication failed")
	}
	var payload closedLoopV3HeldOutSourcePayload
	decodeCorpusStrict(t, plaintext, &payload)
	if payload.Schema != closedLoopV3HeldOutSourceSchema || payload.Version != closedLoopV3CorpusVersion ||
		payload.AuthorManifestHash != manifest.AuthorManifestHash || len(payload.Cases) != seal.CaseCount {
		t.Fatal("V3 held-out source payload contract is invalid")
	}
	result := make(map[string]ots.Requirement, len(payload.Cases))
	for index, candidate := range payload.Cases {
		manifestIndex := index + closedLoopCorpusSize/2
		if manifestIndex >= len(manifest.Entries) {
			t.Fatal("V3 held-out source payload count exceeds the manifest")
		}
		entry := manifest.Entries[manifestIndex]
		if candidate.ID != entry.ID || candidate.RequirementFile != entry.RequirementFile ||
			candidate.RequirementHash != entry.RequirementHash || corpusHash(candidate.Requirement) != entry.RequirementHash {
			t.Fatal("V3 held-out source requirement commitment is invalid")
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(candidate.Requirement))
		if len(issues) != 0 {
			t.Fatal("sealed V3 held-out requirement failed strict decode")
		}
		result[entry.ID] = requirement
	}
	return result
}

func runClosedLoopV3HeldOutBaseline(
	t *testing.T,
	manifest closedLoopV3CorpusManifest,
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
			t.Fatal("sealed V3 held-out requirement is missing")
		}
		first := runClosedLoopV3SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV3SealedSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatal("sealed V3 held-out synthesis replay failed closed")
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopRun(t, "sealed-held-out", first, environment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatal("sealed V3 held-out physical promotion failed closed")
			}
			promotion = &current
		}
		evidence, err := ObserveRealizabilityAware(CaseMeta{
			ID: entry.ID, Role: entry.Role,
			Domain: capabilityevaluation.Domain(entry.Domain), SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact),
		}, requirement, first, promotion)
		if err != nil {
			t.Fatal("sealed V3 held-out observation failed closed")
		}
		results = append(results, evidence)
	}
	if len(results) != closedLoopCorpusSize/2 {
		t.Fatal("sealed V3 held-out baseline did not produce the required case count")
	}
	return results
}

func runClosedLoopV3SealedSynthesis(
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
		t.Fatal("sealed V3 held-out synthesis aborted; baseline was not recorded")
	}
	return run
}

func sealClosedLoopV3HeldOutPayload(t *testing.T, key []byte, manifestHash, selectionHash, payloadHash string, plaintext []byte) []byte {
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
	_, _ = mac.Write([]byte(manifestHash + "\x00" + selectionHash + "\x00" + payloadHash))
	nonce := mac.Sum(nil)[:gcm.NonceSize()]
	additionalData := []byte(closedLoopV3HeldOutPayloadSchema + "\x00" + manifestHash + "\x00" + selectionHash + "\x00" + payloadHash)
	return gcm.Seal(nil, nonce, plaintext, additionalData)
}

func openClosedLoopV3HeldOutPayload(key []byte, manifestHash, selectionHash, payloadHash string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(manifestHash + "\x00" + selectionHash + "\x00" + payloadHash))
	nonce := mac.Sum(nil)[:gcm.NonceSize()]
	additionalData := []byte(closedLoopV3HeldOutPayloadSchema + "\x00" + manifestHash + "\x00" + selectionHash + "\x00" + payloadHash)
	return gcm.Open(nil, nonce, ciphertext, additionalData)
}

func hashClosedLoopV3HeldOutPayload(payload closedLoopV3HeldOutPayload) (string, error) {
	payload.Hash = ""
	return digest(payload)
}

func hashClosedLoopV3HeldOutSeal(seal closedLoopV3HeldOutSeal) (string, error) {
	seal.Hash = ""
	return digest(seal)
}

func TestClosedLoopV3HeldOutSealUsesAuthenticatedEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x6d}, 32)
	manifestHash, selectionHash := fmt.Sprintf("%064d", 1), fmt.Sprintf("%064d", 2)
	plaintext := []byte("sealed V3 held-out evidence")
	payloadHash := corpusHash(plaintext)
	first := sealClosedLoopV3HeldOutPayload(t, key, manifestHash, selectionHash, payloadHash, plaintext)
	second := sealClosedLoopV3HeldOutPayload(t, key, manifestHash, selectionHash, payloadHash, plaintext)
	if !bytes.Equal(first, second) || bytes.Contains(first, plaintext) {
		t.Fatal("V3 held-out baseline seal is nondeterministic or exposes plaintext")
	}
	opened, err := openClosedLoopV3HeldOutPayload(key, manifestHash, selectionHash, payloadHash, first)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatal("V3 held-out baseline seal did not authenticate and decrypt")
	}
	tampered := bytes.Clone(first)
	tampered[len(tampered)-1] ^= 1
	if _, err := openClosedLoopV3HeldOutPayload(key, manifestHash, selectionHash, payloadHash, tampered); err == nil {
		t.Fatal("V3 held-out baseline seal accepted tampered ciphertext")
	}
}

func TestClosedLoopV3HeldOutSealHashRejectsMutation(t *testing.T) {
	seal := closedLoopV3HeldOutSeal{
		Schema: closedLoopV3HeldOutSealSchema, Version: closedLoopV3BaselineVersion,
		CaseCount: closedLoopCorpusSize / 2,
	}
	hash, err := hashClosedLoopV3HeldOutSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	seal.CaseCount--
	mutated, err := hashClosedLoopV3HeldOutSeal(seal)
	if err != nil || mutated == hash {
		t.Fatal("V3 held-out baseline seal mutation did not change its content hash")
	}
}

func TestClosedLoopV3HeldOutBaselineDecryptsWithExternalKey(t *testing.T) {
	keyPath := os.Getenv(closedLoopV3HeldOutBaselineKeyEnv)
	if keyPath == "" {
		t.Skip("external V3 held-out baseline key is unavailable")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V3 held-out baseline key is invalid")
	}
	var seal closedLoopV3HeldOutSeal
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V3_HELD_OUT_BASELINE_SEAL.json")), &seal)
	plaintext, err := openClosedLoopV3HeldOutPayload(
		key, seal.CorpusManifestHash, seal.SelectionHash, seal.PayloadHash,
		mustCorpusRead(t, filepath.Join(closedLoopV3BaselineRoot, closedLoopV3HeldOutBaselineFile)),
	)
	if err != nil || corpusHash(plaintext) == "" {
		t.Fatal("V3 held-out baseline authentication failed")
	}
	var payload closedLoopV3HeldOutPayload
	decodeCorpusStrict(t, plaintext, &payload)
	wantHash, err := hashClosedLoopV3HeldOutPayload(payload)
	if err != nil || payload.Schema != closedLoopV3HeldOutPayloadSchema || payload.Version != closedLoopV3BaselineVersion ||
		payload.CorpusManifestHash != seal.CorpusManifestHash || payload.SelectionHash != seal.SelectionHash ||
		payload.Hash != seal.PayloadHash || payload.Hash != wantHash || len(payload.Cases) != seal.CaseCount ||
		payload.Aggregate.CorpusRole != RoleHeldOut || len(payload.Aggregate.Clusters) != 0 {
		t.Fatal("V3 held-out baseline payload contract is invalid")
	}
	for _, current := range payload.Cases {
		if current.PolicyVersion != RealizabilityPolicyVersion || current.Case.Role != RoleHeldOut {
			t.Fatal("V3 held-out baseline case metadata is invalid")
		}
	}
}
