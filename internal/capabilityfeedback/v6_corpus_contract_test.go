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
	closedLoopV6CorpusRoot         = "testdata/closed_loop_open_set_v6_corpus"
	closedLoopV6CorpusManifestHash = "0445db99a32b5d62e8fc897d532e994fe85ff072aceba294733183d06f6b685a"
	closedLoopV6PolicyHash         = "125ce10a3b86b8d201782b540c217425e59cb4c6f65c2264b2d3a9b1d8b3fa76"
	closedLoopV6CorpusSize         = 36
	closedLoopV6RoleSize           = closedLoopV6CorpusSize / 2
	closedLoopV6AuthorSize         = 6
)

func TestClosedLoopV6CorpusFreeze(t *testing.T) {
	manifestPath := filepath.Join(closedLoopV6CorpusRoot, corpuspublication.ManifestFile)
	manifestBytes := mustCorpusRead(t, manifestPath)
	if got := corpusHash(manifestBytes); got != closedLoopV6CorpusManifestHash {
		t.Fatalf("V6 corpus manifest hash mismatch: got %s, want %s", got, closedLoopV6CorpusManifestHash)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV6CorpusRoot,
		filepath.Join(closedLoopV6CorpusRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V6 corpus checksums: %v", err)
	}

	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != corpuspublication.ManifestSchemaV6 || manifest.Version != corpuspublication.ManifestVersionV6 ||
		manifest.DiscoveryCaseCount != closedLoopV6RoleSize || manifest.HeldOutCaseCount != closedLoopV6RoleSize ||
		len(manifest.Entries) != closedLoopV6CorpusSize {
		t.Fatal("V6 corpus header or case counts are invalid")
	}
	assertClosedLoopV6Commitments(t, manifest)
	assertClosedLoopV6Seal(t, manifest)
	assertClosedLoopV6Entries(t, manifest)
	assertClosedLoopV6FileSet(t, manifest)
}

func assertClosedLoopV6Commitments(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	wantCommits := corpuspublication.Commits{
		StartingCommit:        "9b6f8be61006f7de179099feb0b38080ff18ecb3",
		ContractFreezeCommit:  "0d0350f4542a6f7f97b813331d228cac969767cd",
		AuthoringPacketCommit: "caa781a1f172efc9a404f0c8787baf8c15fa679e",
		ValidatorCommit:       "fc822ae3f1bdd19f9316c0915b50532b8094d8c8",
		FreezeParentCommit:    "43fd9fb61e6c7a93a9b0bfbcfa3583121269b695",
	}
	if !reflect.DeepEqual(manifest.Commits, wantCommits) || manifest.PolicySHA256 != closedLoopV6PolicyHash {
		t.Fatal("V6 corpus lineage or policy commitment changed")
	}

	specRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion")
	packetRoot := filepath.Join(specRoot, "v6-authoring-packet")
	commitments := []struct {
		path string
		want string
	}{
		{filepath.Join(specRoot, "V6_CONTRACT.sha256"), manifest.ContractManifestSHA256},
		{filepath.Join(specRoot, "V6_VALIDATOR.sha256"), manifest.ValidatorManifestSHA256},
		{filepath.Join(specRoot, "V6_PUBLISHER.sha256"), manifest.PublisherManifestSHA256},
		{filepath.Join(specRoot, "V6_HISTORICAL_COMMITMENTS.json"), manifest.HistoricalCommitmentsSHA256},
		{filepath.Join(packetRoot, "PACKET_SET.sha256"), manifest.PacketSetSHA256},
		{filepath.Join(packetRoot, "CONTRACT_BINDING.json"), manifest.ContractBindingSHA256},
		{filepath.Join(closedLoopV6CorpusRoot, corpuspublication.ValidationFile), manifest.ValidationReportSHA256},
	}
	for _, commitment := range commitments {
		if got := corpusHash(mustCorpusRead(t, commitment.path)); got != commitment.want {
			t.Fatalf("V6 commitment no longer matches %s", filepath.Base(commitment.path))
		}
	}
	for author, want := range manifest.AuthorPacketSHA256 {
		path := filepath.Join(packetRoot, fmt.Sprintf("%s_PACKET.sha256", strings.ToUpper(author)))
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V6 author-packet commitment no longer matches %s", author)
		}
	}
	for author, want := range manifest.AssignmentSHA256 {
		path := filepath.Join(packetRoot, "assignments", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V6 assignment commitment no longer matches %s", author)
		}
	}
	for author, want := range manifest.AuthorshipSHA256 {
		path := filepath.Join(closedLoopV6CorpusRoot, "authorship", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V6 authorship commitment no longer matches %s", author)
		}
	}
}

func assertClosedLoopV6Seal(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	seal := manifest.HeldOutSource
	if seal.Algorithm != corpuspublication.SealAlgorithm {
		t.Fatalf("V6 held-out seal algorithm is %q; want %q", seal.Algorithm, corpuspublication.SealAlgorithm)
	}
	if seal.File != corpuspublication.HeldOutCipherFile {
		t.Fatalf("V6 held-out seal file is %q; want %q", seal.File, corpuspublication.HeldOutCipherFile)
	}
	if seal.NonceBytes != 12 || seal.CaseCount != closedLoopV6RoleSize {
		t.Fatalf("V6 held-out seal dimensions are nonce=%d cases=%d", seal.NonceBytes, seal.CaseCount)
	}
	for name, value := range map[string]string{
		"payload": seal.PayloadSHA256, "ciphertext": seal.CiphertextSHA256, "AAD": seal.AADSHA256,
	} {
		if !closedLoopV6ValidHash(value) {
			t.Fatalf("V6 held-out %s commitment is not a SHA-256 digest", name)
		}
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV6CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextSHA256 || len(ciphertext) <= seal.NonceBytes+aes.BlockSize {
		t.Fatal("V6 held-out ciphertext differs from its authenticated commitment")
	}
	if _, err := os.Stat(filepath.Join(closedLoopV6CorpusRoot, "held_out")); !os.IsNotExist(err) {
		t.Fatal("V6 held-out plaintext must not exist in the repository")
	}
}

func assertClosedLoopV6Entries(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	seenRequirement := map[string]bool{}
	seenNeutral := map[string]bool{}
	seenNormalized := map[string]bool{}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	for index, entry := range manifest.Entries {
		role, offset := "discovery", index
		if index >= closedLoopV6RoleSize {
			role, offset = "held_out", index-closedLoopV6RoleSize
		}
		wantID := fmt.Sprintf("v6_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", offset/closedLoopV6AuthorSize+1)
		wantPath := fmt.Sprintf("%s/%s.json", role, wantID)
		if entry.ID != wantID || entry.Role != role || entry.AuthorSlot != wantAuthor ||
			entry.Domain != domains[offset%len(domains)] || entry.StablePath != wantPath || entry.Sealed != (role == "held_out") ||
			!closedLoopV6ValidHash(entry.RequirementSHA256) || !closedLoopV6ValidHash(entry.NeutralSemanticSHA256) ||
			!closedLoopV6ValidHash(entry.NormalizedSemanticSHA256) || seenRequirement[entry.RequirementSHA256] ||
			seenNeutral[entry.NeutralSemanticSHA256] || seenNormalized[entry.NormalizedSemanticSHA256] {
			t.Fatalf("V6 corpus entry %d violates its frozen identity or diversity contract", index+1)
		}
		seenRequirement[entry.RequirementSHA256] = true
		seenNeutral[entry.NeutralSemanticSHA256] = true
		seenNormalized[entry.NormalizedSemanticSHA256] = true
		counts[role][entry.Domain]++
		if role == "held_out" {
			continue
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV6CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(data) != entry.RequirementSHA256 {
			t.Fatalf("V6 discovery entry %d differs from its source commitment", index+1)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 || closedLoopNeutralRequirementHash(t, requirement) != entry.NeutralSemanticSHA256 {
			t.Fatalf("V6 discovery entry %d violates its public or neutral semantic commitment", index+1)
		}
	}
	if !reflect.DeepEqual(counts, manifest.Counts) {
		t.Fatal("V6 manifest domain counts differ from its entries")
	}
}

func assertClosedLoopV6FileSet(t *testing.T, manifest corpuspublication.Manifest) {
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
	if err := filepath.WalkDir(closedLoopV6CorpusRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic path is not allowed: %s", path)
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV6CorpusRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk V6 corpus: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("V6 corpus has %d files; want exactly %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("V6 corpus contains unexpected file %s", path)
		}
	}
}

func closedLoopV6ValidHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
