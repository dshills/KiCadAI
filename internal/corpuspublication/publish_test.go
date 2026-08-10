package corpuspublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
)

func TestPublishPreservesDiscoveryAndSealsHeldOutAtomically(t *testing.T) {
	request, sources := publicationFixture(t)
	result, err := Publish(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.DiscoveryCases != expectedDiscovery || result.HeldOutCases != expectedHeldOut || len(result.Manifest.Entries) != expectedCases {
		t.Fatalf("published counts = %d/%d/%d", result.DiscoveryCases, result.HeldOutCases, len(result.Manifest.Entries))
	}
	if err := verifyChecksums(request.DestinationRoot); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(request.DestinationRoot, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(manifestBytes) != result.ManifestSHA256 {
		t.Fatal("published manifest hash mismatch")
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if !sort.SliceIsSorted(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].ID < manifest.Entries[j].ID }) {
		t.Fatal("manifest entries are not ordered by case identity")
	}
	for _, entry := range manifest.Entries {
		if entry.Role == corpusfreeze.RoleHeldOut {
			if _, err := os.Lstat(filepath.Join(request.DestinationRoot, filepath.FromSlash(entry.StablePath))); !os.IsNotExist(err) {
				t.Fatalf("held-out plaintext path was published: %s", entry.StablePath)
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(request.DestinationRoot, filepath.FromSlash(entry.StablePath)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, sources[entry.ID]) {
			t.Fatalf("discovery bytes changed for %s", entry.ID)
		}
	}

	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("external key length = %d", len(key))
	}
	info, err := os.Stat(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("external key mode = %o", info.Mode().Perm())
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFile))
	if err != nil {
		t.Fatal(err)
	}
	heldOut, err := OpenHeldOut(key, manifest, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if len(heldOut) != expectedHeldOut {
		t.Fatalf("opened held-out cases = %d", len(heldOut))
	}
	for _, item := range heldOut {
		if !bytes.Equal(item.Source, sources[item.Entry.ID]) {
			t.Fatalf("held-out bytes changed for %s", item.Entry.ID)
		}
	}
}

func TestOpenHeldOutRejectsCiphertextAndAADTampering(t *testing.T) {
	request, _ := publicationFixture(t)
	result, err := Publish(request)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFile))
	if err != nil {
		t.Fatal(err)
	}
	tamperedCiphertext := append([]byte(nil), ciphertext...)
	tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0xff
	if _, err := OpenHeldOut(key, result.Manifest, tamperedCiphertext); err == nil {
		t.Fatal("tampered held-out ciphertext was accepted")
	}
	tamperedManifest := result.Manifest
	tamperedManifest.PacketSetSHA256 = strings.Repeat("f", 64)
	if _, err := OpenHeldOut(key, tamperedManifest, ciphertext); err == nil {
		t.Fatal("tampered held-out AAD binding was accepted")
	}
}

func TestPublishRefusesOverwriteAndRepositoryKey(t *testing.T) {
	request, _ := publicationFixture(t)
	if _, err := Publish(request); err != nil {
		t.Fatal(err)
	}
	secondKey := filepath.Join(filepath.Dir(request.RepositoryRoot), "second.key")
	request.KeyPath = secondKey
	if _, err := Publish(request); err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("second publication error = %v", err)
	}
	if _, err := os.Lstat(secondKey); !os.IsNotExist(err) {
		t.Fatal("refused publication created a new key")
	}

	insideRequest, _ := publicationFixture(t)
	insideRequest.KeyPath = filepath.Join(insideRequest.RepositoryRoot, "private.key")
	if _, err := Publish(insideRequest); err == nil || !strings.Contains(err.Error(), "key must be outside") {
		t.Fatalf("inside-key error = %v", err)
	}
}

func TestPublishCleansStageAndKeyOnEntropyFailure(t *testing.T) {
	request, _ := publicationFixture(t)
	request.Random = bytes.NewReader(make([]byte, 8))
	if _, err := Publish(request); err == nil {
		t.Fatal("publication accepted truncated entropy")
	}
	if _, err := os.Lstat(request.DestinationRoot); !os.IsNotExist(err) {
		t.Fatal("failed publication left a destination")
	}
	if _, err := os.Lstat(request.KeyPath); !os.IsNotExist(err) {
		t.Fatal("failed publication left a key")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(request.DestinationRoot), ".corpus.stage-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("failed publication left staging paths: %v", matches)
	}
}

func TestPublishRefusesExistingExternalKey(t *testing.T) {
	request, _ := publicationFixture(t)
	if err := os.MkdirAll(filepath.Dir(request.KeyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.KeyPath, bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(request); err == nil || !strings.Contains(err.Error(), "fresh external key") {
		t.Fatalf("existing-key error = %v", err)
	}
	if _, err := os.Lstat(request.DestinationRoot); !os.IsNotExist(err) {
		t.Fatal("existing-key refusal published a destination")
	}
}

func publicationFixture(t *testing.T) (Request, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	report := corpusfreeze.Report{
		Schema: "kicadai.behavior-corpus-validation-report.v1", Version: 1,
		PolicySHA256: hashBytes([]byte("policy")), PacketSetSHA256: hashBytes([]byte("packet")),
		ContractBindingSHA256: hashBytes([]byte("binding")), HistoricalCommitmentsSHA256: hashBytes([]byte("history")),
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
		Counts: map[string]map[string]int{
			corpusfreeze.RoleDiscovery: {"analog": 3, "power": 3, "digital": 3, "mcu": 3, "sensor": 3, "mixed_signal": 3},
			corpusfreeze.RoleHeldOut:   {"analog": 3, "power": 3, "digital": 3, "mcu": 3, "sensor": 3, "mixed_signal": 3},
		},
	}
	bundles := map[string]corpusfreeze.Bundle{}
	sources := map[string][]byte{}
	for authorIndex := 1; authorIndex <= expectedAuthors; authorIndex++ {
		author := fmt.Sprintf("author_%d", authorIndex)
		authorship := []byte(fmt.Sprintf("{\"author_slot\":%q}\n", author))
		report.AuthorPacketSHA256[author] = hashBytes([]byte("packet-" + author))
		report.AssignmentSHA256[author] = hashBytes([]byte("assignment-" + author))
		report.AuthorshipSHA256[author] = hashBytes(authorship)
		bundles[author] = corpusfreeze.Bundle{AuthorshipJSON: authorship, Requirements: map[string][]byte{}}
	}
	for index := 1; index <= expectedCases; index++ {
		id := fmt.Sprintf("v5_case_%03d", index)
		author := fmt.Sprintf("author_%d", ((index-1)/12)+1)
		role := corpusfreeze.RoleDiscovery
		if index > expectedDiscovery {
			role = corpusfreeze.RoleHeldOut
		}
		requirementFile := fmt.Sprintf("%s/request_%03d.json", role, index)
		source := []byte(fmt.Sprintf("{\n  \"case\": %d\n}\n", index))
		bundle := bundles[author]
		bundle.Requirements[requirementFile] = source
		bundles[author] = bundle
		sources[id] = source
		report.Entries = append(report.Entries, corpusfreeze.EntryEvidence{
			ID: id, AuthorSlot: author, Role: role, Domain: "analog", SafetyImpact: "non_safety",
			SourceID: fmt.Sprintf("v5_source_%03d", index), RequirementFile: requirementFile,
			RequirementSHA256: hashBytes(source), NeutralSemanticSHA256: hashBytes([]byte("neutral-" + id)),
			NormalizedSemanticSHA256: hashBytes([]byte("normalized-" + id)),
		})
	}
	return Request{
		RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "corpus"), KeyPath: filepath.Join(root, "keys", "held-out.key"),
		ContractManifestSHA256: hashBytes([]byte("contract")), ValidatorManifest: []byte("validator manifest\n"),
		PublisherManifest: []byte("publisher manifest\n"),
		Commits: Commits{
			StartingCommit: strings.Repeat("1", 40), ContractFreezeCommit: strings.Repeat("2", 40),
			AuthoringPacketCommit: strings.Repeat("3", 40), ValidatorCommit: strings.Repeat("4", 40),
			FreezeParentCommit: strings.Repeat("5", 40),
		},
		Report: report, Bundles: bundles, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 128)),
	}, sources
}

func TestDecodeHeldOutPayloadRejectsTrailingAndTruncatedData(t *testing.T) {
	request, _ := publicationFixture(t)
	prepared, err := prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeHeldOutPayload(prepared.heldOut)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeHeldOutPayload(payload[:len(payload)-1]); err == nil {
		t.Fatal("truncated payload was accepted")
	}
	if _, err := decodeHeldOutPayload(append(payload, 0)); err == nil {
		t.Fatal("payload with trailing data was accepted")
	}
}

func TestPublishPropagatesRandomFailure(t *testing.T) {
	request, _ := publicationFixture(t)
	request.Random = errorReader{}
	if _, err := Publish(request); err == nil {
		t.Fatal("random source failure was accepted")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
