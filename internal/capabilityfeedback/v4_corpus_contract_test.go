package capabilityfeedback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	// These full commit identities are normative V4 inputs. Rewriting or moving
	// history must fail the freeze instead of silently rebinding the experiment.
	closedLoopV4CorpusSchema            = "kicadai.closed-loop-open-set-corpus.v4"
	closedLoopV4CorpusVersion           = 4
	closedLoopV4CorpusRoot              = "testdata/closed_loop_open_set_v4_corpus"
	closedLoopV4StartCommit             = "3d2d9bb0e8ff3e68ae6a160c136030b5a3b6d7db"
	closedLoopV4ContractFreezeCommit    = "f88efb866dca97ab4966a67ac6e8d50ec6e245f4"
	closedLoopV4AuthoringPacketCommit   = "f42f8f99a66dcb3962a7915646274973e506c155"
	closedLoopV4ImpactRegistryHash      = "64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377"
	closedLoopV4SynthesisPolicyHash     = "4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4"
	closedLoopV4GapTransitionPolicyHash = "ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8"
	closedLoopV4HeldOutSourceSchema     = "kicadai.closed-loop-open-set-held-out-source.v4"
	closedLoopV4HeldOutSealAlgorithm    = "AES-256-GCM/random-nonce-prefixed"
	closedLoopV4HeldOutSealFile         = "held_out_requirements.sealed"
	closedLoopV4HeldOutCorpusKeyEnv     = "KICADAI_V4_HELD_OUT_CORPUS_KEY_FILE"
	closedLoopV4CorpusUpdateEnvironment = "UPDATE_CLOSED_LOOP_V4_CORPUS"
)

type closedLoopV4CorpusManifest struct {
	Schema                  string                        `json:"schema"`
	Version                 int                           `json:"version"`
	StartingCommit          string                        `json:"starting_commit"`
	ContractFreezeCommit    string                        `json:"contract_freeze_commit"`
	AuthoringPacketCommit   string                        `json:"authoring_packet_commit"`
	RequirementSchema       string                        `json:"requirement_schema"`
	EvaluatorPolicy         string                        `json:"evaluator_policy"`
	ImpactRegistryHash      string                        `json:"impact_registry_hash"`
	SynthesisPolicyHash     string                        `json:"synthesis_policy_hash"`
	GapTransitionPolicyHash string                        `json:"gap_transition_policy_sha256"`
	Environment             closedLoopEnvironment         `json:"environment"`
	PacketManifestHash      string                        `json:"packet_manifest_sha256"`
	AuthorManifestHash      string                        `json:"author_manifest_sha256"`
	AuthorshipRecordHash    string                        `json:"authorship_record_sha256"`
	ValidationContractHash  string                        `json:"validation_contract_sha256"`
	HeldOutSourceSeal       closedLoopV4HeldOutSourceSeal `json:"held_out_source_seal"`
	Entries                 []closedLoopV4CorpusEntry     `json:"entries"`
}

type closedLoopV4CorpusEntry struct {
	ID              string     `json:"id"`
	Role            CorpusRole `json:"role"`
	Domain          string     `json:"domain"`
	SafetyImpact    string     `json:"safety_impact"`
	SourceID        string     `json:"source_id"`
	RequirementFile string     `json:"requirement_file"`
	RequirementHash string     `json:"requirement_sha256"`
	SemanticHash    string     `json:"neutral_semantic_sha256"`
	Sealed          bool       `json:"sealed"`
}

type closedLoopV4HeldOutSourceSeal struct {
	Algorithm      string `json:"algorithm"`
	File           string `json:"file"`
	PayloadHash    string `json:"payload_sha256"`
	CiphertextHash string `json:"ciphertext_sha256"`
	CaseCount      int    `json:"case_count"`
}

type closedLoopV4HeldOutSourcePayload struct {
	Schema             string                          `json:"schema"`
	Version            int                             `json:"version"`
	AuthorManifestHash string                          `json:"author_manifest_sha256"`
	Cases              []closedLoopV4HeldOutSourceCase `json:"cases"`
}

type closedLoopV4HeldOutSourceCase struct {
	ID              string `json:"id"`
	RequirementFile string `json:"requirement_file"`
	RequirementHash string `json:"requirement_sha256"`
	SemanticHash    string `json:"neutral_semantic_sha256"`
	Requirement     []byte `json:"requirement"`
}

func TestClosedLoopV4CorpusFreeze(t *testing.T) {
	if _, err := os.Stat(closedLoopV4CorpusRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V4 corpus has not been frozen")
		}
		t.Fatal(err)
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	checksum := strings.TrimSpace(string(mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.sha256"))))
	if want := corpusHash(manifestBytes) + "  manifest.json"; checksum != want {
		t.Fatal("V4 corpus manifest checksum is invalid")
	}
	var manifest closedLoopV4CorpusManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != closedLoopV4CorpusSchema || manifest.Version != closedLoopV4CorpusVersion ||
		manifest.StartingCommit != closedLoopV4StartCommit || manifest.ContractFreezeCommit != closedLoopV4ContractFreezeCommit ||
		manifest.AuthoringPacketCommit != closedLoopV4AuthoringPacketCommit || manifest.RequirementSchema != ots.RequirementSchema ||
		manifest.EvaluatorPolicy != RealizabilityPolicyVersion || manifest.ImpactRegistryHash != closedLoopV4ImpactRegistryHash ||
		manifest.SynthesisPolicyHash != closedLoopV4SynthesisPolicyHash ||
		manifest.GapTransitionPolicyHash != closedLoopV4GapTransitionPolicyHash || len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatal("V4 corpus manifest header, commit, policy, or size is invalid")
	}

	packetRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v4-authoring-packet")
	packetManifest := mustCorpusRead(t, filepath.Join(packetRoot, "PACKET.sha256"))
	authorManifest := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "AUTHOR_MANIFEST.json"))
	// Exact validator bytes are committed deliberately: even a seemingly
	// nonfunctional edit must be reviewed as a new validation contract.
	if manifest.PacketManifestHash != corpusHash(packetManifest) || manifest.AuthorManifestHash != corpusHash(authorManifest) ||
		manifest.AuthorshipRecordHash != corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "AUTHORSHIP.md"))) ||
		manifest.ValidationContractHash != corpusHash(mustCorpusRead(t, "v4_candidate_corpus_test.go")) {
		t.Fatal("V4 packet, authorship, or validation-contract commitment is invalid")
	}
	if !bytes.Equal(authorManifest, mustCorpusRead(t, filepath.Join(packetRoot, "AUTHOR_MANIFEST.json"))) {
		t.Fatal("V4 frozen author manifest differs from the frozen packet")
	}
	seal := manifest.HeldOutSourceSeal
	if seal.Algorithm != closedLoopV4HeldOutSealAlgorithm || seal.File != closedLoopV4HeldOutSealFile ||
		seal.CaseCount != closedLoopCorpusSize/2 || len(seal.PayloadHash) != sha256.Size*2 || len(seal.CiphertextHash) != sha256.Size*2 {
		t.Fatal("V4 held-out source seal metadata is invalid")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextHash || len(ciphertext) <= seal.CaseCount*aes.BlockSize {
		t.Fatal("V4 held-out source ciphertext commitment is invalid")
	}
	if _, err := os.Stat(filepath.Join(closedLoopV4CorpusRoot, "held_out")); !os.IsNotExist(err) {
		t.Fatal("V4 held-out plaintext directory must not exist in the frozen corpus")
	}
	if _, err := os.Stat(closedLoopV4CandidateRoot); !os.IsNotExist(err) {
		t.Fatal("V4 plaintext candidate quarantine must be destroyed after freeze")
	}

	var authored closedLoopV4AuthorManifest
	decodeCorpusStrict(t, authorManifest, &authored)
	seenSemantic := map[string]bool{}
	for index, entry := range manifest.Entries {
		wantRole, wantDirectory, wantSealed := RoleDiscovery, "discovery", false
		if index >= closedLoopCorpusSize/2 {
			wantRole, wantDirectory, wantSealed = RoleHeldOut, "held_out", true
		}
		wantID := fmt.Sprintf("v4_case_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || entry.Role != wantRole || entry.RequirementFile != wantFile || entry.Sealed != wantSealed ||
			len(entry.RequirementHash) != sha256.Size*2 || len(entry.SemanticHash) != sha256.Size*2 || seenSemantic[entry.SemanticHash] ||
			index >= len(authored.Entries) || entry.SourceID != authored.Entries[index].SourceID ||
			entry.Domain != authored.Entries[index].Domain || entry.SafetyImpact != authored.Entries[index].SafetyImpact {
			t.Fatalf("V4 corpus entry %d has invalid frozen metadata", index+1)
		}
		seenSemantic[entry.SemanticHash] = true
		if !entry.Sealed {
			data := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
			if corpusHash(data) != entry.RequirementHash {
				t.Fatalf("V4 discovery entry %d hash is invalid", index+1)
			}
			requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
			if len(issues) != 0 || closedLoopNeutralRequirementHash(t, requirement) != entry.SemanticHash {
				t.Fatalf("V4 discovery entry %d violates its public or semantic commitment", index+1)
			}
		}
	}
}

func TestUpdateClosedLoopV4Corpus(t *testing.T) {
	if os.Getenv(closedLoopV4CorpusUpdateEnvironment) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V4_CORPUS=1 to freeze the validated V4 candidate")
	}
	if _, err := os.Stat(closedLoopV4CorpusRoot); !os.IsNotExist(err) {
		t.Fatal("V4 corpus already exists; update mode refuses overwrite")
	}
	keyPath := os.Getenv(closedLoopV4HeldOutCorpusKeyEnv)
	if keyPath == "" {
		t.Fatalf("%s is required", closedLoopV4HeldOutCorpusKeyEnv)
	}
	closedLoopV4RequireExternalKeyPath(t, keyPath)
	TestClosedLoopV4CandidateQuarantine(t)

	authorManifest := mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, "AUTHOR_MANIFEST.json"))
	var authored closedLoopV4AuthorManifest
	decodeCorpusStrict(t, authorManifest, &authored)
	temporaryRoot := closedLoopV4CorpusRoot + ".tmp"
	if err := os.Mkdir(temporaryRoot, 0o755); err != nil {
		t.Fatal("create V4 corpus staging root")
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporaryRoot) })
	if err := os.Mkdir(filepath.Join(temporaryRoot, "discovery"), 0o755); err != nil {
		t.Fatal("create V4 discovery staging root")
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "AUTHOR_MANIFEST.json"), authorManifest, 0o644); err != nil {
		t.Fatal("freeze V4 author manifest")
	}
	authorship := mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, "AUTHORSHIP.md"))
	if err := os.WriteFile(filepath.Join(temporaryRoot, "AUTHORSHIP.md"), authorship, 0o644); err != nil {
		t.Fatal("freeze V4 authorship record")
	}

	manifest := closedLoopV4CorpusManifest{
		Schema: closedLoopV4CorpusSchema, Version: closedLoopV4CorpusVersion, StartingCommit: closedLoopV4StartCommit,
		ContractFreezeCommit: closedLoopV4ContractFreezeCommit, AuthoringPacketCommit: closedLoopV4AuthoringPacketCommit,
		RequirementSchema: ots.RequirementSchema, EvaluatorPolicy: RealizabilityPolicyVersion,
		ImpactRegistryHash: closedLoopV4ImpactRegistryHash, SynthesisPolicyHash: closedLoopV4SynthesisPolicyHash,
		GapTransitionPolicyHash: closedLoopV4GapTransitionPolicyHash,
		Environment:             closedLoopEnvironment{GoMinimum: frozenGoMinimum, KiCad: frozenKiCadVersion, OS: frozenOperatingSystem, Arch: frozenProcessorArchitecture},
		PacketManifestHash:      corpusHash(mustCorpusRead(t, filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v4-authoring-packet", "PACKET.sha256"))),
		AuthorManifestHash:      corpusHash(authorManifest),
		AuthorshipRecordHash:    corpusHash(authorship),
		ValidationContractHash:  corpusHash(mustCorpusRead(t, "v4_candidate_corpus_test.go")),
	}
	payload := closedLoopV4HeldOutSourcePayload{
		Schema: closedLoopV4HeldOutSourceSchema, Version: closedLoopV4CorpusVersion, AuthorManifestHash: manifest.AuthorManifestHash,
	}
	for index, entry := range authored.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("V4 candidate %s changed after validation", entry.ID)
		}
		frozenEntry := closedLoopV4CorpusEntry{
			ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact, SourceID: entry.SourceID,
			RequirementFile: entry.RequirementFile, RequirementHash: corpusHash(data),
			SemanticHash: closedLoopNeutralRequirementHash(t, requirement), Sealed: entry.Role == RoleHeldOut,
		}
		manifest.Entries = append(manifest.Entries, frozenEntry)
		if entry.Role == RoleDiscovery {
			if err := os.WriteFile(filepath.Join(temporaryRoot, "discovery", fmt.Sprintf("request_%03d.json", index+1)), data, 0o644); err != nil {
				t.Fatal("freeze V4 discovery requirement")
			}
			continue
		}
		payload.Cases = append(payload.Cases, closedLoopV4HeldOutSourceCase{
			ID: entry.ID, RequirementFile: entry.RequirementFile, RequirementHash: frozenEntry.RequirementHash,
			SemanticHash: frozenEntry.SemanticHash, Requirement: data,
		})
	}
	plaintext := corpusJSON(t, payload)
	payloadHash := corpusHash(plaintext)
	key := closedLoopV4LoadOrCreateKey(t, keyPath)
	ciphertext := closedLoopV4SealHeldOutSource(t, key, manifest.AuthorManifestHash, payloadHash, plaintext)
	manifest.HeldOutSourceSeal = closedLoopV4HeldOutSourceSeal{
		Algorithm: closedLoopV4HeldOutSealAlgorithm, File: closedLoopV4HeldOutSealFile,
		PayloadHash: payloadHash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(payload.Cases),
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, closedLoopV4HeldOutSealFile), ciphertext, 0o600); err != nil {
		t.Fatal("seal V4 held-out source")
	}
	manifestBytes := corpusJSON(t, manifest)
	if err := os.WriteFile(filepath.Join(temporaryRoot, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal("write V4 corpus manifest")
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "manifest.sha256"), []byte(corpusHash(manifestBytes)+"  manifest.json\n"), 0o644); err != nil {
		t.Fatal("write V4 corpus manifest checksum")
	}
	if err := os.Rename(temporaryRoot, closedLoopV4CorpusRoot); err != nil {
		t.Fatal("atomically freeze V4 corpus")
	}
}

func closedLoopV4RequireExternalKeyPath(t *testing.T, keyPath string) {
	t.Helper()
	absoluteKey, err := filepath.Abs(keyPath)
	if err != nil {
		t.Fatal("resolve V4 held-out key path")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal("resolve repository path")
	}
	relative, err := filepath.Rel(repository, absoluteKey)
	if err != nil || relative == "." || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("V4 held-out key must be stored outside the repository")
	}
}

func closedLoopV4LoadOrCreateKey(t *testing.T, path string) []byte {
	t.Helper()
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			t.Fatal("V4 held-out corpus key has invalid length")
		}
		return key
	}
	if !os.IsNotExist(err) {
		t.Fatal("V4 held-out corpus key is unavailable")
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("generate V4 held-out corpus key")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal("create external V4 held-out key directory")
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal("persist V4 held-out corpus key")
	}
	return key
}

func closedLoopV4SealHeldOutSource(t *testing.T, key []byte, authorManifestHash, payloadHash string, plaintext []byte) []byte {
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
		t.Fatal("generate V4 held-out source nonce")
	}
	additionalData := []byte(closedLoopV4HeldOutSourceSchema + "\x00" + authorManifestHash + "\x00" + payloadHash)
	return gcm.Seal(nonce, nonce, plaintext, additionalData)
}

func closedLoopV4OpenHeldOutSource(key []byte, authorManifestHash, payloadHash string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("V4 held-out source ciphertext is truncated")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	additionalData := []byte(closedLoopV4HeldOutSourceSchema + "\x00" + authorManifestHash + "\x00" + payloadHash)
	return gcm.Open(nil, nonce, sealed, additionalData)
}

func TestClosedLoopV4HeldOutSourceSealDecryptsWithExternalKey(t *testing.T) {
	keyPath := os.Getenv(closedLoopV4HeldOutCorpusKeyEnv)
	if keyPath == "" {
		t.Skip("external V4 held-out corpus key is unavailable")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V4 held-out corpus key is invalid")
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, "manifest.json"))
	var manifest closedLoopV4CorpusManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	seal := manifest.HeldOutSourceSeal
	plaintext, err := closedLoopV4OpenHeldOutSource(key, manifest.AuthorManifestHash, seal.PayloadHash,
		mustCorpusRead(t, filepath.Join(closedLoopV4CorpusRoot, seal.File)))
	if err != nil {
		t.Fatalf("V4 held-out source authentication failed: %v", err)
	}
	if corpusHash(plaintext) != seal.PayloadHash {
		t.Fatal("V4 held-out source plaintext hash does not match its commitment")
	}
	var payload closedLoopV4HeldOutSourcePayload
	decodeCorpusStrict(t, plaintext, &payload)
	if payload.Schema != closedLoopV4HeldOutSourceSchema || payload.Version != closedLoopV4CorpusVersion ||
		payload.AuthorManifestHash != manifest.AuthorManifestHash || len(payload.Cases) != seal.CaseCount {
		t.Fatal("V4 held-out source payload contract is invalid")
	}
	for index, candidate := range payload.Cases {
		entry := manifest.Entries[index+closedLoopCorpusSize/2]
		requirement, issues := ots.DecodeStrict(bytes.NewReader(candidate.Requirement))
		if candidate.ID != entry.ID || candidate.RequirementFile != entry.RequirementFile ||
			candidate.RequirementHash != entry.RequirementHash || candidate.SemanticHash != entry.SemanticHash ||
			corpusHash(candidate.Requirement) != entry.RequirementHash || len(issues) != 0 ||
			closedLoopNeutralRequirementHash(t, requirement) != entry.SemanticHash {
			t.Fatalf("V4 held-out source entry %d does not match its commitments", index+1)
		}
	}
}

func TestClosedLoopV4HeldOutSourceSealUsesAuthenticatedEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x4c}, 32)
	authorHash := fmt.Sprintf("%064d", 1)
	plaintext := []byte("sealed V4 held-out source")
	payloadHash := corpusHash(plaintext)
	first := closedLoopV4SealHeldOutSource(t, key, authorHash, payloadHash, plaintext)
	second := closedLoopV4SealHeldOutSource(t, key, authorHash, payloadHash, plaintext)
	if bytes.Equal(first, second) || bytes.Contains(first, plaintext) || bytes.Contains(second, plaintext) {
		t.Fatal("V4 held-out source seal reuses a nonce or exposes plaintext")
	}
	for index, sealed := range [][]byte{first, second} {
		opened, err := closedLoopV4OpenHeldOutSource(key, authorHash, payloadHash, sealed)
		if err != nil || !bytes.Equal(opened, plaintext) {
			t.Fatalf("V4 held-out source seal %d did not authenticate and decrypt", index+1)
		}
	}
	tampered := bytes.Clone(first)
	tampered[len(tampered)-1] ^= 1
	if _, err := closedLoopV4OpenHeldOutSource(key, authorHash, payloadHash, tampered); err == nil {
		t.Fatal("V4 held-out source seal accepted tampered ciphertext")
	}
	changed := []byte("changed V4 held-out source")
	if bytes.Equal(first, closedLoopV4SealHeldOutSource(t, key, authorHash, corpusHash(changed), changed)) {
		t.Fatal("V4 held-out source seal did not bind changed plaintext")
	}
}
