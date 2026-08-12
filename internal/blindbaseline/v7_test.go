package blindbaseline

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestV7PublishVerifyOpenAndVersionSeparation(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	keyPath := filepath.Join(external, "baseline.key")
	payload := []byte(`{"sensitive":"v7-held-out"}`)
	result, err := PublishV7(V7Request{
		RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath,
		ReservedKeyPaths: []string{filepath.Join(external, "source.key")}, Binding: testV7Binding(),
		Payload: payload, CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128)),
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyV7(destination)
	if err != nil || manifest != result.Manifest {
		t.Fatalf("verify V7 manifest = %#v, %v", manifest, err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(destination, CipherFile))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenV7(key, manifest, ciphertext)
	if err != nil || !bytes.Equal(opened, payload) {
		t.Fatalf("opened V7 payload = %q, %v", opened, err)
	}
	if _, err := Verify(destination); err == nil {
		t.Fatal("V5 verifier accepted V7 publication")
	}
	v5Binding := testBinding()
	v5PayloadHash := hashBytes(payload)
	v5AAD, err := additionalData(v5Binding, v5PayloadHash, 18)
	if err != nil {
		t.Fatal(err)
	}
	v5Manifest := Manifest{Schema: ManifestSchema, Version: ManifestVersion, Algorithm: Algorithm, Binding: v5Binding, PayloadSHA256: v5PayloadHash, CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(v5AAD), NonceBytes: 12, CaseCount: 18}
	v5Manifest.Hash, err = manifestHash(v5Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(key, v5Manifest, ciphertext); err == nil {
		t.Fatal("V5 opener accepted V7 ciphertext")
	}
	v5Ciphertext, _, err := seal(key, payload, []byte("v5"), bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	v7Manifest := manifest
	v7Manifest.CiphertextSHA256 = hashBytes(v5Ciphertext)
	v7Manifest.Hash = hashBytes(manifestCommitmentV7(v7Manifest))
	if _, err := OpenV7(key, v7Manifest, v5Ciphertext); err == nil {
		t.Fatal("V7 opener accepted V5 ciphertext")
	}
}

func TestV7AuthenticationRejectsMutationAndNoReplaceCleansKey(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	keyPath := filepath.Join(external, "baseline.key")
	result, err := PublishV7(V7Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath, Binding: testV7Binding(), Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := os.ReadFile(keyPath)
	ciphertext, _ := os.ReadFile(filepath.Join(destination, CipherFile))
	mutated := append([]byte(nil), ciphertext...)
	mutated[len(mutated)-1] ^= 1
	if _, err := OpenV7(key, result.Manifest, mutated); err == nil {
		t.Fatal("V7 opener accepted ciphertext mutation")
	}
	manifest := result.Manifest
	manifest.Binding.SelectionSHA256 = testHash("f")
	manifest.Hash = hashBytes(manifestCommitmentV7(manifest))
	if _, err := OpenV7(key, manifest, ciphertext); err == nil {
		t.Fatal("V7 opener accepted authenticated binding mutation")
	}
	secondKey := filepath.Join(external, "second.key")
	if _, err := PublishV7(V7Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: secondKey, Binding: testV7Binding(), Payload: []byte("sensitive"), CaseCount: 18, Random: bytes.NewReader(bytes.Repeat([]byte{0x43}, 128))}); err == nil {
		t.Fatal("V7 publication overwrote its destination")
	}
	if _, err := os.Stat(secondKey); !os.IsNotExist(err) {
		t.Fatal("failed V7 no-replace publication retained its new key")
	}
}

func TestV7BindingFieldRegistryCoversJSONContractInOrder(t *testing.T) {
	typeOfBinding := reflect.TypeOf(V7Binding{})
	fields := (V7Binding{}).fields()
	if len(fields) != typeOfBinding.NumField() {
		t.Fatalf("V7 binding registry fields = %d, struct fields = %d", len(fields), typeOfBinding.NumField())
	}
	seen := map[string]bool{}
	for index := range fields {
		name := strings.Split(typeOfBinding.Field(index).Tag.Get("json"), ",")[0]
		if name == "" || name != fields[index].name || seen[name] {
			t.Fatalf("V7 binding registry entry %d does not match unique JSON field %q", index, name)
		}
		seen[name] = true
	}
}

func testV7Binding() V7Binding {
	return V7Binding{
		StartingCommit: testCommit("1"), ContractFreezeCommit: testCommit("2"), CorpusFreezeCommit: testCommit("3"), SelectionFreezeCommit: testCommit("4"), PublisherParentCommit: testCommit("5"),
		CorpusManifestSHA256: testHash("1"), ContractManifestSHA256: testHash("2"), ValidatorManifestSHA256: testHash("3"), PublisherManifestSHA256: testHash("4"), ValidationReportSHA256: testHash("5"), PacketSetSHA256: testHash("6"), ContractBindingSHA256: testHash("7"), HistoricalCommitmentsSHA256: testHash("8"), SourceCiphertextSHA256: testHash("9"),
		DiscoveryBaselineSHA256: testHash("a"), FrontierSHA256: testHash("f"), RankingSHA256: testHash("b"), SelectionSHA256: testHash("c"), GenericPlanSHA256: testHash("d"), EvaluatorPolicy: "policy",
		ImpactRegistrySHA256: testHash("e"), SynthesisPolicySHA256: testHash("f"), GapPolicySHA256: testHash("1"), SelectionPolicySHA256: testHash("2"), InventorySHA256: testHash("4"), CatalogSHA256: testHash("5"), ModelRegistrySHA256: testHash("6"), EnvironmentPolicySHA256: testHash("7"),
		PromotionPlatform: "test/arm64", KiCadVersion: "10.0.3", PromotionToolchainSHA256: testHash("8"), PromotionToolchainLockSHA256: testHash("9"), KiCadCLISHA256: testHash("a"), SymbolTableSHA256: testHash("b"), FootprintTableSHA256: testHash("c"), SymbolsSHA256: testHash("d"), FootprintsSHA256: testHash("e"),
	}
}
