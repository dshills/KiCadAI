package capabilityfeedback

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV7CorpusRoot         = "testdata/closed_loop_open_set_v7_corpus"
	closedLoopV7CorpusManifestHash = "cf85a7eb8293abdd7f85215b3f897ccf4b89be993ac5c101d09013f2ca979f06"
	closedLoopV7PolicyHash         = "76ff7942d39b11ded3d82ce600fbcddb90b75c9c6685499d9d03dc9e9b82d444"
	closedLoopV7CorpusSize         = 36
	closedLoopV7RoleSize           = closedLoopV7CorpusSize / 2
	closedLoopV7AuthorSize         = 6
)

func TestClosedLoopV7CorpusFreeze(t *testing.T) {
	manifestPath := filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ManifestFile)
	manifestBytes := mustCorpusRead(t, manifestPath)
	if got := corpusHash(manifestBytes); got != closedLoopV7CorpusManifestHash {
		t.Fatalf("V7 corpus manifest hash mismatch: got %s, want %s", got, closedLoopV7CorpusManifestHash)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV7CorpusRoot,
		filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V7 corpus checksums: %v", err)
	}

	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != corpuspublication.ManifestSchemaV7 || manifest.Version != corpuspublication.ManifestVersionV7 ||
		manifest.DiscoveryCaseCount != closedLoopV7RoleSize || manifest.HeldOutCaseCount != closedLoopV7RoleSize ||
		len(manifest.Entries) != closedLoopV7CorpusSize {
		t.Fatal("V7 corpus header or case counts are invalid")
	}
	assertClosedLoopV7Commitments(t, manifest)
	assertClosedLoopV7Seal(t, manifest)
	assertClosedLoopV7Entries(t, manifest)
	assertClosedLoopV7FileSet(t, manifest)
}

func assertClosedLoopV7Commitments(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	wantCommits := corpuspublication.Commits{
		StartingCommit:        "156f7eb439ca5313471c504ddb91db1b8a8724f0",
		ContractFreezeCommit:  "e780c8cfca51623d81b9eae209fedf2b98816681",
		AuthoringPacketCommit: "5f2b0c72b7ca7418b14a5a943306d5a596bd3716",
		ValidatorCommit:       "d7677432aab118303954ca6b55420ae98a5074ad",
		FreezeParentCommit:    "cc11e78d2eeadf3019fc54ecacc140ab50ff0e19",
	}
	if !reflect.DeepEqual(manifest.Commits, wantCommits) || manifest.PolicySHA256 != closedLoopV7PolicyHash {
		t.Fatal("V7 corpus lineage or policy commitment changed")
	}

	specRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion")
	packetRoot := filepath.Join(specRoot, "v7-authoring-packet")
	commitments := []struct {
		path string
		want string
	}{
		{filepath.Join(specRoot, "V7_CONTRACT.sha256"), manifest.ContractManifestSHA256},
		{filepath.Join(specRoot, "V7_VALIDATOR_CONTRACT.sha256"), manifest.ValidatorManifestSHA256},
		{filepath.Join(specRoot, "V7_PUBLISHER.sha256"), manifest.PublisherManifestSHA256},
		{filepath.Join(specRoot, "V7_HISTORICAL_COMMITMENTS.json"), manifest.HistoricalCommitmentsSHA256},
		{filepath.Join(packetRoot, "PACKET_SET.sha256"), manifest.PacketSetSHA256},
		{filepath.Join(packetRoot, "CONTRACT_BINDING.json"), manifest.ContractBindingSHA256},
		{filepath.Join(closedLoopV7CorpusRoot, corpuspublication.ValidationFile), manifest.ValidationReportSHA256},
	}
	for _, commitment := range commitments {
		if got := corpusHash(mustCorpusRead(t, commitment.path)); got != commitment.want {
			t.Fatalf("V7 commitment no longer matches %s", filepath.Base(commitment.path))
		}
	}
	for author, want := range manifest.AuthorPacketSHA256 {
		path := filepath.Join(packetRoot, fmt.Sprintf("%s_PACKET.sha256", strings.ToUpper(author)))
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V7 author-packet commitment no longer matches %s", author)
		}
	}
	for author, want := range manifest.AssignmentSHA256 {
		path := filepath.Join(packetRoot, "assignments", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V7 assignment commitment no longer matches %s", author)
		}
	}
	for author, want := range manifest.AuthorshipSHA256 {
		path := filepath.Join(closedLoopV7CorpusRoot, "authorship", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V7 authorship commitment no longer matches %s", author)
		}
	}
}

func assertClosedLoopV7Seal(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	seal := manifest.HeldOutSource
	if seal.Algorithm != corpuspublication.SealAlgorithm {
		t.Fatalf("V7 held-out seal algorithm is %q; want %q", seal.Algorithm, corpuspublication.SealAlgorithm)
	}
	if seal.File != corpuspublication.HeldOutCipherFile {
		t.Fatalf("V7 held-out seal file is %q; want %q", seal.File, corpuspublication.HeldOutCipherFile)
	}
	if seal.NonceBytes != 12 || seal.CaseCount != closedLoopV7RoleSize {
		t.Fatalf("V7 held-out seal dimensions are nonce=%d cases=%d", seal.NonceBytes, seal.CaseCount)
	}
	for name, value := range map[string]string{
		"payload": seal.PayloadSHA256, "ciphertext": seal.CiphertextSHA256, "AAD": seal.AADSHA256,
	} {
		if !closedLoopV7ValidHash(value) {
			t.Fatalf("V7 held-out %s commitment is not a SHA-256 digest", name)
		}
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextSHA256 || len(ciphertext) <= seal.NonceBytes+aes.BlockSize {
		t.Fatal("V7 held-out ciphertext differs from its authenticated commitment")
	}
	if _, err := os.Stat(filepath.Join(closedLoopV7CorpusRoot, "held_out")); !os.IsNotExist(err) {
		t.Fatal("V7 held-out plaintext must not exist in the repository")
	}
}

func assertClosedLoopV7Entries(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	seenRequirement := map[string]bool{}
	seenNeutral := map[string]bool{}
	seenNormalized := map[string]bool{}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	for index, entry := range manifest.Entries {
		role, offset := "discovery", index
		if index >= closedLoopV7RoleSize {
			role, offset = "held_out", index-closedLoopV7RoleSize
		}
		wantID := fmt.Sprintf("v7_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", offset/closedLoopV7AuthorSize+1)
		wantPath := fmt.Sprintf("%s/%s.json", role, wantID)
		if entry.ID != wantID || entry.Role != role || entry.AuthorSlot != wantAuthor ||
			entry.Domain != domains[offset%len(domains)] || entry.StablePath != wantPath || entry.Sealed != (role == "held_out") ||
			!closedLoopV7ValidHash(entry.RequirementSHA256) || !closedLoopV7ValidHash(entry.NeutralSemanticSHA256) ||
			!closedLoopV7ValidHash(entry.NormalizedSemanticSHA256) || seenRequirement[entry.RequirementSHA256] ||
			seenNeutral[entry.NeutralSemanticSHA256] || seenNormalized[entry.NormalizedSemanticSHA256] {
			t.Fatalf("V7 corpus entry %d violates its frozen identity or diversity contract", index+1)
		}
		seenRequirement[entry.RequirementSHA256] = true
		seenNeutral[entry.NeutralSemanticSHA256] = true
		seenNormalized[entry.NormalizedSemanticSHA256] = true
		counts[role][entry.Domain]++
		if role == "held_out" {
			continue
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV7CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(data) != entry.RequirementSHA256 {
			t.Fatalf("V7 discovery entry %d differs from its source commitment", index+1)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 || closedLoopNeutralRequirementHash(t, requirement) != entry.NeutralSemanticSHA256 {
			t.Fatalf("V7 discovery entry %d violates its public or neutral semantic commitment", index+1)
		}
	}
	if !reflect.DeepEqual(counts, manifest.Counts) {
		t.Fatal("V7 manifest domain counts differ from its entries")
	}
}

func assertClosedLoopV7FileSet(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	want := map[string]bool{
		corpuspublication.AuditFile:         true,
		corpuspublication.ChecksumFile:      true,
		corpuspublication.HeldOutCipherFile: true,
		corpuspublication.ManifestFile:      true,
		corpuspublication.ValidationFile:    true,
	}
	for author := range manifest.AuthorshipSHA256 {
		want[filepath.ToSlash(filepath.Join("authorship", author+".json"))] = true
	}
	for _, entry := range manifest.Entries {
		if !entry.Sealed {
			want[entry.StablePath] = true
		}
	}
	var got []string
	if err := filepath.WalkDir(closedLoopV7CorpusRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic path is not allowed: %s", path)
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV7CorpusRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk V7 corpus: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("V7 corpus has %d files; want exactly %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("V7 corpus contains unexpected file %s", path)
		}
	}
}

func closedLoopV7ValidHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
