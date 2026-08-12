package corpuspublication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev8"
)

func TestPublishV8RecordSealsAndObligations(t *testing.T) {
	request, sources := publicationFixtureV8(t)
	result, err := PublishV8(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Schema != ManifestSchemaV8 || result.Manifest.Version != 8 || result.DiscoveryCases != 18 || result.HeldOutCases != 18 ||
		result.DiscoveryObligations == 0 || result.HeldOutObligations == 0 || len(result.Manifest.Entries) != 18 || len(result.Manifest.HeldOutSource.RecordCiphertextSHA256) != 18 {
		t.Fatalf("unexpected V8 result: %+v", result)
	}
	if err := verifyChecksums(request.DestinationRoot); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(request.DestinationRoot, ManifestFileV8))
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(manifestBytes) != result.ManifestSHA256 {
		t.Fatal("V8 manifest hash mismatch")
	}
	for _, entry := range result.Manifest.Entries {
		if entry.Role != "discovery" || entry.Sealed {
			t.Fatalf("public manifest disclosed a held-out entry: %+v", entry)
		}
		publishedPath := filepath.Join(request.DestinationRoot, filepath.FromSlash(entry.StablePath))
		data, err := os.ReadFile(publishedPath)
		if err != nil || !bytes.Equal(data, sources[entry.ID]) {
			t.Fatalf("discovery mismatch: %s", entry.ID)
		}
	}
	for _, name := range []string{ManifestFileV8, ValidationFileV8, DiscoveryObligationsFileV8, AuditFileV8} {
		data, err := os.ReadFile(filepath.Join(request.DestinationRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("v8_case_019")) || bytes.Contains(data, []byte("held_out/request_019")) {
			t.Fatalf("public artifact %s disclosed held-out case mapping", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(request.DestinationRoot, "authorship")); !os.IsNotExist(err) {
		t.Fatal("authorship metadata was published")
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(request.KeyPath)
	if err != nil || info.Mode().Perm() != 0o600 || len(key) != 32 {
		t.Fatalf("V8 key metadata invalid")
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFileV8))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenHeldOutV8(key, result.Manifest, ciphertext)
	if err != nil || len(opened) != 18 {
		t.Fatalf("open V8 records: %d, %v", len(opened), err)
	}
	for _, item := range opened {
		if !bytes.Equal(item.Source, sources[item.Entry.ID]) {
			t.Fatalf("held-out mismatch: %s", item.Entry.ID)
		}
	}

	var discovery DiscoveryObligationsV8
	data, err := os.ReadFile(filepath.Join(request.DestinationRoot, DiscoveryObligationsFileV8))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &discovery); err != nil || discovery.CorpusManifestSHA256 != result.ManifestSHA256 || len(discovery.Obligations) != result.DiscoveryObligations {
		t.Fatalf("discovery obligations invalid: %v", err)
	}
	var heldOut HeldOutObligationCommitmentV8
	data, err = os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCommitmentFileV8))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &heldOut); err != nil || heldOut.CorpusManifestSHA256 != result.ManifestSHA256 || heldOut.ObligationCount != result.HeldOutObligations || !validSHA256(heldOut.AggregateSHA256) {
		t.Fatalf("held-out obligation commitment invalid: %v", err)
	}
}

func TestOpenHeldOutV8RejectsRecordAndAADTampering(t *testing.T) {
	request, _ := publicationFixtureV8(t)
	result, err := PublishV8(request)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := os.ReadFile(request.KeyPath)
	ciphertext, _ := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFileV8))
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 1
	if _, err := OpenHeldOutV8(key, result.Manifest, tampered); err == nil {
		t.Fatal("tampered V8 record set accepted")
	}
	metadata := result.Manifest
	metadata.PacketSetSHA256 = strings.Repeat("a", 64)
	if _, err := OpenHeldOutV8(key, metadata, ciphertext); err == nil {
		t.Fatal("tampered V8 AAD metadata accepted")
	}
}

func TestPublishV8FailsClosedOnOccupiedDestinationAndEntropy(t *testing.T) {
	request, _ := publicationFixtureV8(t)
	if err := os.MkdirAll(request.DestinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishV8(request); err == nil {
		t.Fatal("occupied V8 destination accepted")
	}
	if _, err := os.Lstat(request.KeyPath); !os.IsNotExist(err) {
		t.Fatal("refused V8 publication created a key")
	}

	request, _ = publicationFixtureV8(t)
	request.Random = bytes.NewReader(make([]byte, 40))
	if _, err := PublishV8(request); err == nil {
		t.Fatal("short V8 entropy accepted")
	}
	if _, err := os.Lstat(request.DestinationRoot); !os.IsNotExist(err) {
		t.Fatal("failed V8 publication left destination")
	}
	if _, err := os.Lstat(request.KeyPath); !os.IsNotExist(err) {
		t.Fatal("failed V8 publication left key")
	}
}

func publicationFixtureV8(t *testing.T) (RequestV8, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "opentopologysynthesis", "testdata", "architecture_generalization_corpus", "regulated_low_voltage_output.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := corpusfreezev8.Report{Schema: "kicadai.behavior-corpus-validation-report.v8", Version: 8,
		PolicySHA256: hashBytes([]byte("policy-v8")), PacketSetSHA256: hashBytes([]byte("packet-v8")),
		ContractBindingSHA256: hashBytes([]byte("binding-v8")), HistoricalCommitmentsSHA256: hashBytes([]byte("history-v8")),
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
		Counts: map[string]map[string]int{"discovery": {"analog_signal_path": 18}, "held_out": {"analog_signal_path": 18}}}
	bundles := map[string]corpusfreeze.Bundle{}
	sources := map[string][]byte{}
	for authorIndex := 1; authorIndex <= expectedAuthorsV8; authorIndex++ {
		author := fmt.Sprintf("author_%d", authorIndex)
		authorship := []byte(fmt.Sprintf("{\"author_slot\":%q}\n", author))
		report.AuthorPacketSHA256[author] = hashBytes([]byte("packet-" + author))
		report.AssignmentSHA256[author] = hashBytes([]byte("assignment-" + author))
		report.AuthorshipSHA256[author] = hashBytes(authorship)
		bundles[author] = corpusfreeze.Bundle{AuthorshipJSON: authorship, Requirements: map[string][]byte{}}
	}
	for index := 1; index <= expectedCasesV8; index++ {
		id := fmt.Sprintf("v8_case_%03d", index)
		author := fmt.Sprintf("author_%d", ((index-1)/6)+1)
		role := "discovery"
		if index > 18 {
			role = "held_out"
		}
		requestPath := fmt.Sprintf("%s/request_%03d.json", role, index)
		bundle := bundles[author]
		bundle.Requirements[requestPath] = source
		bundles[author] = bundle
		sources[id] = source
		report.Entries = append(report.Entries, corpusfreezev8.EntryEvidence{ID: id, AuthorSlot: author, Role: role, Domain: "analog_signal_path", CircuitRole: "conversion_regulation", SafetyImpact: "non_safety",
			SourceID: fmt.Sprintf("v8_source_%03d", index), RequirementFile: requestPath, RequirementSHA256: hashBytes(source),
			NeutralSemanticSHA256: hashBytes([]byte("neutral-" + id)), NormalizedSemanticSHA256: hashBytes([]byte("normalized-" + id))})
	}
	entropy := make([]byte, 2048)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	return RequestV8{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "corpus"), KeyPath: filepath.Join(root, "keys", "v8-source.key"),
		ContractManifestSHA256: hashBytes([]byte("contract-v8")), ValidatorManifest: []byte("validator-v8\n"), PublisherManifest: []byte("publisher-v8\n"),
		Commits: Commits{StartingCommit: strings.Repeat("1", 40), ContractFreezeCommit: strings.Repeat("2", 40), AuthoringPacketCommit: strings.Repeat("3", 40), ValidatorCommit: strings.Repeat("4", 40), FreezeParentCommit: strings.Repeat("5", 40)},
		Report:  report, Bundles: bundles, Random: bytes.NewReader(entropy)}, sources
}
