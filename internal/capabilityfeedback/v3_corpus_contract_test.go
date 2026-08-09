package capabilityfeedback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
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
	closedLoopV3CorpusSchema            = "kicadai.closed-loop-open-set-corpus.v3"
	closedLoopV3CorpusVersion           = 3
	closedLoopV3CorpusRoot              = "testdata/closed_loop_open_set_v3_corpus"
	closedLoopV3StartCommit             = "859c8df068db8254b715042b691c441a0d135fab"
	closedLoopV3ImpactRegistryHash      = "64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377"
	closedLoopV3SynthesisPolicyHash     = "4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4"
	closedLoopV3HeldOutSourceSchema     = "kicadai.closed-loop-open-set-held-out-source.v3"
	closedLoopV3HeldOutSealAlgorithm    = "AES-256-GCM/HMAC-SHA-256-payload-bound-nonce"
	closedLoopV3HeldOutSealFile         = "held_out_requirements.sealed"
	closedLoopV3HeldOutCorpusKeyEnv     = "KICADAI_V3_HELD_OUT_CORPUS_KEY_FILE"
	closedLoopV3CorpusUpdateEnvironment = "UPDATE_CLOSED_LOOP_V3_CORPUS"
)

type closedLoopV3CorpusManifest struct {
	Schema               string                        `json:"schema"`
	Version              int                           `json:"version"`
	StartingCommit       string                        `json:"starting_commit"`
	RequirementSchema    string                        `json:"requirement_schema"`
	EvaluatorPolicy      string                        `json:"evaluator_policy"`
	ImpactRegistryHash   string                        `json:"impact_registry_hash"`
	SynthesisPolicyHash  string                        `json:"synthesis_policy_hash"`
	Environment          closedLoopEnvironment         `json:"environment"`
	PacketManifestHash   string                        `json:"packet_manifest_sha256"`
	AuthorManifestHash   string                        `json:"author_manifest_sha256"`
	AuthorshipRecordHash string                        `json:"authorship_record_sha256"`
	HeldOutSourceSeal    closedLoopV3HeldOutSourceSeal `json:"held_out_source_seal"`
	Entries              []closedLoopV3CorpusEntry     `json:"entries"`
}

type closedLoopV3CorpusEntry struct {
	ID              string     `json:"id"`
	Role            CorpusRole `json:"role"`
	Domain          string     `json:"domain"`
	SafetyImpact    string     `json:"safety_impact"`
	SourceID        string     `json:"source_id"`
	RequirementFile string     `json:"requirement_file"`
	RequirementHash string     `json:"requirement_sha256"`
	Sealed          bool       `json:"sealed"`
}

type closedLoopV3HeldOutSourceSeal struct {
	Algorithm      string `json:"algorithm"`
	File           string `json:"file"`
	PayloadHash    string `json:"payload_sha256"`
	CiphertextHash string `json:"ciphertext_sha256"`
	CaseCount      int    `json:"case_count"`
}

type closedLoopV3HeldOutSourcePayload struct {
	Schema             string                          `json:"schema"`
	Version            int                             `json:"version"`
	AuthorManifestHash string                          `json:"author_manifest_sha256"`
	Cases              []closedLoopV3HeldOutSourceCase `json:"cases"`
}

type closedLoopV3HeldOutSourceCase struct {
	ID              string `json:"id"`
	RequirementFile string `json:"requirement_file"`
	RequirementHash string `json:"requirement_sha256"`
	Requirement     []byte `json:"requirement"`
}

func TestClosedLoopV3CorpusFreeze(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	checksum := strings.TrimSpace(string(mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.sha256"))))
	if want := corpusHash(manifestBytes) + "  manifest.json"; checksum != want {
		t.Fatal("V3 corpus manifest checksum is invalid")
	}
	var manifest closedLoopV3CorpusManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != closedLoopV3CorpusSchema || manifest.Version != closedLoopV3CorpusVersion ||
		manifest.StartingCommit != closedLoopV3StartCommit || manifest.RequirementSchema != ots.RequirementSchema ||
		manifest.EvaluatorPolicy != RealizabilityPolicyVersion || manifest.ImpactRegistryHash != closedLoopV3ImpactRegistryHash ||
		manifest.SynthesisPolicyHash != closedLoopV3SynthesisPolicyHash || len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatal("V3 corpus manifest header, policy, or size is invalid")
	}
	packetRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v3-authoring-packet")
	packetManifest := mustCorpusRead(t, filepath.Join(packetRoot, "PACKET.sha256"))
	authorManifest := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "AUTHOR_MANIFEST.json"))
	if manifest.PacketManifestHash != corpusHash(packetManifest) || manifest.AuthorManifestHash != corpusHash(authorManifest) ||
		manifest.AuthorshipRecordHash != corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "AUTHORSHIP.md"))) {
		t.Fatal("V3 packet or authorship commitment is invalid")
	}
	if !bytes.Equal(authorManifest, mustCorpusRead(t, filepath.Join(packetRoot, "AUTHOR_MANIFEST.json"))) {
		t.Fatal("V3 frozen author manifest differs from the sealed packet")
	}
	seal := manifest.HeldOutSourceSeal
	if seal.Algorithm != closedLoopV3HeldOutSealAlgorithm || seal.File != closedLoopV3HeldOutSealFile ||
		seal.CaseCount != closedLoopCorpusSize/2 || len(seal.PayloadHash) != sha256.Size*2 || len(seal.CiphertextHash) != sha256.Size*2 {
		t.Fatal("V3 held-out source seal metadata is invalid")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextHash || len(ciphertext) <= seal.CaseCount*aes.BlockSize {
		t.Fatal("V3 held-out source ciphertext commitment is invalid")
	}
	if _, err := os.Stat(filepath.Join(closedLoopV3CorpusRoot, "held_out")); !os.IsNotExist(err) {
		t.Fatal("V3 held-out plaintext directory must not exist in the frozen corpus")
	}

	var authored closedLoopV2AuthorManifest
	decodeCorpusStrict(t, authorManifest, &authored)
	for index, entry := range manifest.Entries {
		wantRole, wantDirectory, wantSealed := RoleDiscovery, "discovery", false
		if index >= closedLoopCorpusSize/2 {
			wantRole, wantDirectory, wantSealed = RoleHeldOut, "held_out", true
		}
		wantID := fmt.Sprintf("v3_case_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || entry.Role != wantRole || entry.RequirementFile != wantFile || entry.Sealed != wantSealed ||
			len(entry.RequirementHash) != sha256.Size*2 || index >= len(authored.Entries) ||
			entry.SourceID != authored.Entries[index].SourceID || entry.Domain != authored.Entries[index].Domain ||
			entry.SafetyImpact != authored.Entries[index].SafetyImpact {
			t.Fatalf("V3 corpus entry %d has invalid frozen metadata", index+1)
		}
		if !entry.Sealed {
			data := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
			if corpusHash(data) != entry.RequirementHash {
				t.Fatalf("V3 discovery entry %d hash is invalid", index+1)
			}
			if _, issues := ots.DecodeStrict(bytes.NewReader(data)); len(issues) != 0 {
				t.Fatalf("V3 discovery entry %d violates the public contract", index+1)
			}
		}
	}
}

func TestUpdateClosedLoopV3Corpus(t *testing.T) {
	if os.Getenv(closedLoopV3CorpusUpdateEnvironment) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V3_CORPUS=1 to freeze the validated V3 candidate")
	}
	if _, err := os.Stat(closedLoopV3CorpusRoot); !os.IsNotExist(err) {
		t.Fatal("V3 corpus already exists; update mode refuses overwrite")
	}
	keyPath := os.Getenv(closedLoopV3HeldOutCorpusKeyEnv)
	if keyPath == "" {
		t.Fatalf("%s is required", closedLoopV3HeldOutCorpusKeyEnv)
	}
	closedLoopV3RequireExternalKeyPath(t, keyPath)
	TestClosedLoopV3CandidateQuarantine(t)

	authorManifest := mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, "AUTHOR_MANIFEST.json"))
	var authored closedLoopV2AuthorManifest
	decodeCorpusStrict(t, authorManifest, &authored)
	temporaryRoot := closedLoopV3CorpusRoot + ".tmp"
	if err := os.Mkdir(temporaryRoot, 0o755); err != nil {
		t.Fatal("create V3 corpus staging root")
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporaryRoot) })
	if err := os.Mkdir(filepath.Join(temporaryRoot, "discovery"), 0o755); err != nil {
		t.Fatal("create V3 discovery staging root")
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "AUTHOR_MANIFEST.json"), authorManifest, 0o644); err != nil {
		t.Fatal("freeze V3 author manifest")
	}
	authorship := mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, "AUTHORSHIP.md"))
	if err := os.WriteFile(filepath.Join(temporaryRoot, "AUTHORSHIP.md"), authorship, 0o644); err != nil {
		t.Fatal("freeze V3 authorship record")
	}

	manifest := closedLoopV3CorpusManifest{
		Schema: closedLoopV3CorpusSchema, Version: closedLoopV3CorpusVersion, StartingCommit: closedLoopV3StartCommit,
		RequirementSchema: ots.RequirementSchema, EvaluatorPolicy: RealizabilityPolicyVersion,
		ImpactRegistryHash: closedLoopV3ImpactRegistryHash, SynthesisPolicyHash: closedLoopV3SynthesisPolicyHash,
		Environment:          closedLoopEnvironment{GoMinimum: frozenGoMinimum, KiCad: frozenKiCadVersion, OS: frozenOperatingSystem, Arch: frozenProcessorArchitecture},
		PacketManifestHash:   corpusHash(mustCorpusRead(t, filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v3-authoring-packet", "PACKET.sha256"))),
		AuthorManifestHash:   corpusHash(authorManifest),
		AuthorshipRecordHash: corpusHash(authorship),
	}
	payload := closedLoopV3HeldOutSourcePayload{
		Schema: closedLoopV3HeldOutSourceSchema, Version: closedLoopV3CorpusVersion,
		AuthorManifestHash: manifest.AuthorManifestHash,
	}
	for index, entry := range authored.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, filepath.FromSlash(entry.RequirementFile)))
		frozenEntry := closedLoopV3CorpusEntry{
			ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact,
			SourceID: entry.SourceID, RequirementFile: entry.RequirementFile, RequirementHash: corpusHash(data),
			Sealed: entry.Role == RoleHeldOut,
		}
		manifest.Entries = append(manifest.Entries, frozenEntry)
		if entry.Role == RoleDiscovery {
			if err := os.WriteFile(filepath.Join(temporaryRoot, "discovery", fmt.Sprintf("request_%03d.json", index+1)), data, 0o644); err != nil {
				t.Fatal("freeze V3 discovery requirement")
			}
			continue
		}
		payload.Cases = append(payload.Cases, closedLoopV3HeldOutSourceCase{
			ID: entry.ID, RequirementFile: entry.RequirementFile, RequirementHash: frozenEntry.RequirementHash, Requirement: data,
		})
	}
	plaintext := corpusJSON(t, payload)
	payloadHash := corpusHash(plaintext)
	key := closedLoopV3LoadOrCreateKey(t, keyPath)
	ciphertext := closedLoopV3SealHeldOutSource(t, key, manifest.AuthorManifestHash, payloadHash, plaintext)
	manifest.HeldOutSourceSeal = closedLoopV3HeldOutSourceSeal{
		Algorithm: closedLoopV3HeldOutSealAlgorithm, File: closedLoopV3HeldOutSealFile,
		PayloadHash: payloadHash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(payload.Cases),
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, closedLoopV3HeldOutSealFile), ciphertext, 0o600); err != nil {
		t.Fatal("seal V3 held-out source")
	}
	manifestBytes := corpusJSON(t, manifest)
	if err := os.WriteFile(filepath.Join(temporaryRoot, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal("write V3 corpus manifest")
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "manifest.sha256"), []byte(corpusHash(manifestBytes)+"  manifest.json\n"), 0o644); err != nil {
		t.Fatal("write V3 corpus manifest checksum")
	}
	if err := os.Rename(temporaryRoot, closedLoopV3CorpusRoot); err != nil {
		t.Fatal("atomically freeze V3 corpus")
	}
}

func closedLoopV3RequireExternalKeyPath(t *testing.T, keyPath string) {
	t.Helper()
	absoluteKey, err := filepath.Abs(keyPath)
	if err != nil {
		t.Fatal("resolve V3 held-out key path")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal("resolve repository path")
	}
	relative, err := filepath.Rel(repository, absoluteKey)
	if err != nil || relative == "." || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("V3 held-out key must be stored outside the repository")
	}
}

func closedLoopV3LoadOrCreateKey(t *testing.T, path string) []byte {
	t.Helper()
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			t.Fatal("V3 held-out corpus key has invalid length")
		}
		return key
	}
	if !os.IsNotExist(err) {
		t.Fatal("V3 held-out corpus key is unavailable")
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("generate V3 held-out corpus key")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal("create external V3 held-out key directory")
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal("persist V3 held-out corpus key")
	}
	return key
}

func closedLoopV3SealHeldOutSource(t *testing.T, key []byte, authorManifestHash, payloadHash string, plaintext []byte) []byte {
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
	_, _ = mac.Write([]byte(authorManifestHash + "\x00" + payloadHash))
	nonce := mac.Sum(nil)[:gcm.NonceSize()]
	additionalData := []byte(closedLoopV3HeldOutSourceSchema + "\x00" + authorManifestHash + "\x00" + payloadHash)
	return gcm.Seal(nil, nonce, plaintext, additionalData)
}

func closedLoopV3OpenHeldOutSource(key []byte, authorManifestHash, payloadHash string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(authorManifestHash + "\x00" + payloadHash))
	nonce := mac.Sum(nil)[:gcm.NonceSize()]
	additionalData := []byte(closedLoopV3HeldOutSourceSchema + "\x00" + authorManifestHash + "\x00" + payloadHash)
	return gcm.Open(nil, nonce, ciphertext, additionalData)
}

func TestClosedLoopV3HeldOutSourceSealDecryptsWithExternalKey(t *testing.T) {
	keyPath := os.Getenv(closedLoopV3HeldOutCorpusKeyEnv)
	if keyPath == "" {
		t.Skip("external V3 held-out corpus key is unavailable")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		t.Fatal("external V3 held-out corpus key is invalid")
	}
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	var manifest closedLoopV3CorpusManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
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
	for index, candidate := range payload.Cases {
		manifestIndex := index + closedLoopCorpusSize/2
		if manifestIndex >= len(manifest.Entries) {
			t.Fatal("V3 held-out source payload count exceeds the manifest")
		}
		entry := manifest.Entries[manifestIndex]
		if candidate.ID != entry.ID || candidate.RequirementFile != entry.RequirementFile ||
			candidate.RequirementHash != entry.RequirementHash || corpusHash(candidate.Requirement) != entry.RequirementHash {
			t.Fatalf("V3 held-out source entry %d does not match its commitment", index+1)
		}
		if _, issues := ots.DecodeStrict(bytes.NewReader(candidate.Requirement)); len(issues) != 0 {
			t.Fatalf("V3 held-out source entry %d violates the public contract", index+1)
		}
	}
}

func TestClosedLoopV3HeldOutSourceSealUsesAuthenticatedEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x3c}, 32)
	authorHash := fmt.Sprintf("%064d", 1)
	plaintext := []byte("sealed held-out source")
	payloadHash := corpusHash(plaintext)
	first := closedLoopV3SealHeldOutSource(t, key, authorHash, payloadHash, plaintext)
	second := closedLoopV3SealHeldOutSource(t, key, authorHash, payloadHash, plaintext)
	if !bytes.Equal(first, second) || bytes.Contains(first, plaintext) {
		t.Fatal("V3 held-out source seal is nondeterministic or exposes plaintext")
	}
	opened, err := closedLoopV3OpenHeldOutSource(key, authorHash, payloadHash, first)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatal("V3 held-out source seal did not authenticate and decrypt")
	}
	tampered := bytes.Clone(first)
	tampered[len(tampered)-1] ^= 1
	if _, err := closedLoopV3OpenHeldOutSource(key, authorHash, payloadHash, tampered); err == nil {
		t.Fatal("V3 held-out source seal accepted tampered ciphertext")
	}
	changed := []byte("changed held-out source")
	if bytes.Equal(first, closedLoopV3SealHeldOutSource(t, key, authorHash, corpusHash(changed), changed)) {
		t.Fatal("V3 held-out source seal did not bind changed plaintext")
	}
}
