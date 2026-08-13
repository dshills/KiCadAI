package blindbaseline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestV10RecordPublicationVerifyOpenAndVersionSeparation(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	keyPath := filepath.Join(external, "baseline.key")
	records := testRecordsV10()
	result, err := PublishV10(V10Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath,
		ReservedKeyPaths: []string{filepath.Join(external, "source.key"), filepath.Join(external, "final.key")},
		Binding:          testV10Binding(), Records: records, Random: bytes.NewReader(testEntropyV10())})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyV10(destination)
	if err != nil || !reflect.DeepEqual(manifest, result.Manifest) {
		t.Fatalf("verify V10 manifest: %v", err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0o600 || len(key) != 32 {
		t.Fatal("V10 external baseline key metadata is invalid")
	}
	ciphertext, err := os.ReadFile(filepath.Join(destination, CipherFileV10))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenV10(key, manifest, ciphertext)
	if err != nil || !equalRecordsV10(opened, records) {
		t.Fatalf("open V10 records: %v", err)
	}
	for _, name := range []string{ManifestFile, AuditFile, ChecksumFile} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range records {
			if bytes.Contains(data, record) {
				t.Fatalf("public V10 artifact %s exposed held-out evidence", name)
			}
		}
	}
	if _, err := VerifyV8(destination); err == nil {
		t.Fatal("V7 verifier accepted V10 publication")
	}
	if _, err := OpenV8(key, V8Manifest{}, ciphertext); err == nil {
		t.Fatal("V7 opener accepted V10 record set")
	}
}

func TestV10AuthenticationNonceUniquenessAndNoReplace(t *testing.T) {
	repository := t.TempDir()
	external := t.TempDir()
	destination := filepath.Join(repository, "sealed")
	keyPath := filepath.Join(external, "baseline.key")
	result, err := PublishV10(V10Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: keyPath,
		Binding: testV10Binding(), Records: testRecordsV10(), Random: bytes.NewReader(testEntropyV10())})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := os.ReadFile(keyPath)
	ciphertext, _ := os.ReadFile(filepath.Join(destination, CipherFileV10))
	mutated := append([]byte(nil), ciphertext...)
	mutated[len(mutated)-1] ^= 1
	if _, err := OpenV10(key, result.Manifest, mutated); err == nil {
		t.Fatal("V10 opener accepted ciphertext mutation")
	}
	wrongKey := bytes.Repeat([]byte{0xff}, 32)
	if _, err := OpenV10(wrongKey, result.Manifest, ciphertext); err == nil {
		t.Fatal("V10 opener accepted the wrong key")
	}
	manifest := result.Manifest
	manifest.Binding.SelectionSHA256 = testHash("f")
	manifest.Hash = hashBytes(manifestCommitmentV10(manifest))
	if _, err := OpenV10(key, manifest, ciphertext); err == nil {
		t.Fatal("V10 opener accepted authenticated binding mutation")
	}
	secondKey := filepath.Join(external, "second.key")
	if _, err := PublishV10(V10Request{RepositoryRoot: repository, DestinationRoot: destination, KeyPath: secondKey,
		Binding: testV10Binding(), Records: testRecordsV10(), Random: bytes.NewReader(testEntropyV10())}); err == nil {
		t.Fatal("V10 publication overwrote its destination")
	}
	if _, err := os.Stat(secondKey); !os.IsNotExist(err) {
		t.Fatal("failed V10 no-replace publication retained its new key")
	}
	duplicateNonceEntropy := append(bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, nonceBytesV10*expectedCasesV10)...)
	if _, err := PublishV10(V10Request{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "duplicate"), KeyPath: filepath.Join(external, "duplicate.key"),
		Binding: testV10Binding(), Records: testRecordsV10(), Random: bytes.NewReader(duplicateNonceEntropy)}); err == nil {
		t.Fatal("V10 publication accepted nonce reuse")
	}
	if _, err := os.Stat(filepath.Join(external, "duplicate.key")); !os.IsNotExist(err) {
		t.Fatal("failed V10 nonce-reuse publication retained its new key")
	}
}

func TestV10BindingRegistryAndExactRecordCount(t *testing.T) {
	typeOfBinding := reflect.TypeOf(V10Binding{})
	fields := (V10Binding{}).fields()
	if len(fields) != typeOfBinding.NumField() {
		t.Fatalf("V10 binding registry fields = %d, struct fields = %d", len(fields), typeOfBinding.NumField())
	}
	seen := map[string]bool{}
	for index := range fields {
		name := strings.Split(typeOfBinding.Field(index).Tag.Get("json"), ",")[0]
		if name == "" || name != fields[index].name || seen[name] {
			t.Fatalf("V10 binding registry entry %d does not match unique JSON field %q", index, name)
		}
		seen[name] = true
	}
	request := V10Request{RepositoryRoot: t.TempDir(), DestinationRoot: filepath.Join(t.TempDir(), "sealed"), KeyPath: filepath.Join(t.TempDir(), "key"), Binding: testV10Binding(), Records: testRecordsV10()[:23]}
	if _, err := PublishV10(request); err == nil {
		t.Fatal("V10 publication accepted a partial held-out cohort")
	}
}

func testRecordsV10() [][]byte {
	records := make([][]byte, expectedCasesV10)
	for index := range records {
		records[index] = []byte(fmt.Sprintf("private-v10-held-out-evidence-%03d-%s", index+1, strings.Repeat("x", index+1)))
	}
	return records
}

func testEntropyV10() []byte {
	data := make([]byte, 32+nonceBytesV10*expectedCasesV10)
	for index := range data {
		data[index] = byte(index)
	}
	return data
}

func testV10Binding() V10Binding {
	return V10Binding{
		StartingCommit: testCommit("1"), ContractFreezeCommit: testCommit("2"), CorpusFreezeCommit: testCommit("3"), EvaluatorFreezeCommit: testCommit("4"), SelectionFreezeCommit: testCommit("5"), PublisherParentCommit: testCommit("6"),
		CorpusManifestSHA256: testHash("1"), ContractManifestSHA256: testHash("2"), ValidatorManifestSHA256: testHash("3"), CorpusPublisherManifestSHA256: testHash("4"), BaselinePublisherManifestSHA256: testHash("5"), ValidationReportSHA256: testHash("6"), PacketSetSHA256: testHash("7"), ContractBindingSHA256: testHash("8"), HistoricalCommitmentsSHA256: testHash("9"), SourceCiphertextSHA256: testHash("a"), DiscoveryObligationsSHA256: testHash("b"), HeldOutObligationCommitmentSHA256: testHash("c"),
		DiscoveryBaselineSHA256: testHash("d"), FrontierSHA256: testHash("e"), RankingSHA256: testHash("f"), SelectionSHA256: testHash("1"), GenericPlanSHA256: testHash("2"), EvaluatorManifestSHA256: testHash("3"), EvaluatorPolicy: "policy.v10", GapRegistrySHA256: testHash("4"), ImpactRegistrySHA256: testHash("5"), SynthesisPolicySHA256: testHash("6"), GapPolicySHA256: testHash("7"), SelectionPolicySHA256: testHash("8"), InventorySHA256: testHash("9"), CatalogSHA256: testHash("a"), ModelRegistrySHA256: testHash("b"), EnvironmentPolicySHA256: testHash("c"), ResourceCeilingsSHA256: testHash("d"), SeedSetSHA256: testHash("e"),
		PromotionPlatform: "test/arm64", KiCadVersion: "10.0.3", PromotionToolchainSHA256: testHash("f"), PromotionToolchainLockSHA256: testHash("1"), KiCadCLISHA256: testHash("2"), SymbolTableSHA256: testHash("3"), FootprintTableSHA256: testHash("4"), SymbolsSHA256: testHash("5"), FootprintsSHA256: testHash("6"),
	}
}
