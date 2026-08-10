package blindbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishVerifyAndOpen(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	keyPath := filepath.Join(external, "baseline.key")
	payload := []byte(`{"sensitive":"held-out"}`)
	result, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath, ReservedKeyPaths: []string{filepath.Join(external, "source.key")}, Binding: testBinding(), Payload: payload, CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Verify(destination)
	if err != nil || manifest != result.Manifest {
		t.Fatalf("verify manifest = %#v, %v", manifest, err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(destination, CipherFile))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(key, manifest, ciphertext)
	if err != nil || !bytes.Equal(opened, payload) {
		t.Fatalf("opened payload = %q, %v", opened, err)
	}
	if _, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: filepath.Join(external, "second.key"), Binding: testBinding(), Payload: payload, CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x43}, 128))}); err == nil {
		t.Fatal("held-out baseline publication overwrote its destination")
	}
	if _, err := os.Stat(filepath.Join(external, "second.key")); !os.IsNotExist(err) {
		t.Fatal("no-replace publication failure retained its new external key")
	}
}

func TestAuthenticationRejectsMutation(t *testing.T) {
	key := bytes.Repeat([]byte{0x21}, 32)
	binding := testBinding()
	payload := []byte("sensitive")
	payloadHash := hashBytes(payload)
	aad, err := additionalData(binding, payloadHash, 18)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonceBytes, err := seal(key, payload, aad, bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Schema: ManifestSchema, Version: ManifestVersion, Algorithm: Algorithm, Binding: binding, PayloadSHA256: payloadHash, CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes, CaseCount: 18}
	manifest.Hash, err = manifestHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	mutated := append([]byte(nil), ciphertext...)
	mutated[len(mutated)-1] ^= 1
	if _, err := Open(key, manifest, mutated); err == nil {
		t.Fatal("held-out baseline authentication accepted ciphertext mutation")
	}
	if _, err := Open(bytes.Repeat([]byte{0x22}, 32), manifest, ciphertext); err == nil {
		t.Fatal("held-out baseline authentication accepted the wrong key")
	}
	if _, err := Open(bytes.Repeat([]byte{0x21}, 16), manifest, ciphertext); err == nil {
		t.Fatal("held-out baseline authentication accepted an AES-128 key")
	}
	manifest.Binding.SelectionSHA256 = testHash("f")
	manifest.Hash, err = manifestHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(key, manifest, ciphertext); err == nil {
		t.Fatal("held-out baseline authentication accepted binding mutation")
	}
}

func TestPublicationFailureCleansNewKey(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(external, "baseline.key")
	_, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath, Binding: testBinding(), Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err == nil {
		t.Fatal("publication unexpectedly succeeded")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("failed publication retained a new external key")
	}
}

func TestCreationFailurePreservesPreexistingKey(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	keyPath := filepath.Join(external, "baseline.key")
	want := bytes.Repeat([]byte{0x7c}, 32)
	if err := os.WriteFile(keyPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "sealed"), KeyPath: keyPath, Binding: testBinding(), Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err == nil {
		t.Fatal("publication unexpectedly replaced a pre-existing external key")
	}
	got, err := os.ReadFile(keyPath)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("pre-existing external key changed: %x, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "sealed")); !os.IsNotExist(err) {
		t.Fatal("failed exclusive key creation published repository artifacts")
	}
}

func TestPublishRejectsMalformedEvaluatorPolicyIdentifier(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	binding := testBinding()
	binding.EvaluatorPolicy = "Policy With Spaces"
	keyPath := filepath.Join(external, "baseline.key")
	_, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "sealed"), KeyPath: keyPath, Binding: binding, Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err == nil {
		t.Fatal("publication accepted a malformed evaluator policy identifier")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("rejected evaluator policy created a key")
	}
}

func TestPublishAcceptsSHA256CommitBindings(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	binding := testBinding()
	binding.StartingCommit = testHash("b")
	_, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "sealed"), KeyPath: filepath.Join(external, "baseline.key"), Binding: binding, Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err != nil {
		t.Fatalf("publication rejected a SHA-256 Git object ID: %v", err)
	}
}

func TestPublishRejectsReservedKeyAndExternalDestination(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	keyPath := filepath.Join(external, "same.key")
	request := Request{
		RepositoryRoot:   repository,
		DestinationRoot:  filepath.Join(repository, "sealed"),
		KeyPath:          keyPath,
		ReservedKeyPaths: []string{keyPath},
		Binding:          testBinding(),
		Payload:          []byte("sensitive"),
		CaseCount:        18,
		Random:           bytes.NewReader(bytes.Repeat([]byte{0x42}, 128)),
	}
	if _, err := Publish(request); err == nil {
		t.Fatal("publication accepted one path for source and baseline keys")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("rejected key-path collision created a key")
	}
	request.ReservedKeyPaths = nil
	request.DestinationRoot = filepath.Join(external, "sealed")
	if _, err := Publish(request); err == nil {
		t.Fatal("publication accepted an external artifact destination")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("rejected external destination created a key")
	}
}

func TestVerifyRejectsSelfConsistentAuditMutationAndTrailingManifest(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	_, err := Publish(Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: filepath.Join(external, "baseline.key"), Binding: testBinding(), Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(destination, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(destination, CipherFile))
	if err != nil {
		t.Fatal(err)
	}
	mutatedAudit := []byte("sensitive disclosure\n")
	if err := os.WriteFile(filepath.Join(destination, AuditFile), mutatedAudit, 0o644); err != nil {
		t.Fatal(err)
	}
	mutated := map[string][]byte{AuditFile: mutatedAudit, CipherFile: ciphertext, ManifestFile: manifest}
	if err := os.WriteFile(filepath.Join(destination, ChecksumFile), checksumManifest(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(destination); err == nil {
		t.Fatal("verification accepted a self-consistent noncanonical audit")
	}
	canonicalAudit := baselineAudit(mustDecodeManifest(t, manifest))
	trailing := append(append([]byte(nil), manifest...), []byte("{}\n")...)
	if err := os.WriteFile(filepath.Join(destination, AuditFile), canonicalAudit, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, ManifestFile), trailing, 0o644); err != nil {
		t.Fatal(err)
	}
	mutated = map[string][]byte{AuditFile: canonicalAudit, CipherFile: ciphertext, ManifestFile: trailing}
	if err := os.WriteFile(filepath.Join(destination, ChecksumFile), checksumManifest(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(destination); err == nil {
		t.Fatal("verification accepted trailing manifest JSON")
	}
}

func mustDecodeManifest(t *testing.T, data []byte) Manifest {
	t.Helper()
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func testBinding() Binding {
	return Binding{StartingCommit: testCommit("1"), ContractFreezeCommit: testCommit("2"), CorpusFreezeCommit: testCommit("3"), SelectionFreezeCommit: testCommit("4"), PublisherParentCommit: testCommit("5"), CorpusManifestSHA256: testHash("1"), SourceCiphertextSHA256: testHash("2"), SelectionSHA256: testHash("3"), EvaluatorPolicy: "policy", ImpactRegistrySHA256: testHash("4"), SynthesisPolicySHA256: testHash("5"), GapPolicySHA256: testHash("6"), SelectionPolicySHA256: testHash("7"), InventorySHA256: testHash("8"), CatalogSHA256: testHash("9"), ModelRegistrySHA256: testHash("a")}
}

func testCommit(character string) string { return string(bytes.Repeat([]byte(character), 40)) }
func testHash(character string) string   { return string(bytes.Repeat([]byte(character), 64)) }
