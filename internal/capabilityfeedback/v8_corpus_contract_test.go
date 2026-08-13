package capabilityfeedback

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV8CorpusRoot         = "testdata/closed_loop_open_set_v8_corpus"
	closedLoopV8CorpusManifestHash = "548d8f38cdbc6186a737d9c1cfdea73906a25f6b1948b9a367e00897f7c66f1c"
	closedLoopV8PolicyHash         = "10f2d38097d564f731dddc8aee2e91205bed562fc8f2c566264c68adc57af76b"
	closedLoopV8RoleSize           = 18
)

func TestClosedLoopV8CorpusFreeze(t *testing.T) {
	manifestPath := filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ManifestFileV8)
	manifestBytes := mustCorpusRead(t, manifestPath)
	if got := corpusHash(manifestBytes); got != closedLoopV8CorpusManifestHash {
		t.Fatalf("V8 corpus manifest hash mismatch: got %s, want %s", got, closedLoopV8CorpusManifestHash)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV8CorpusRoot,
		filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ChecksumFileV8),
	); err != nil {
		t.Fatalf("verify V8 corpus checksums: %v", err)
	}

	var manifest corpuspublication.ManifestV8
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != corpuspublication.ManifestSchemaV8 || manifest.Version != corpuspublication.ManifestVersionV8 {
		t.Fatalf("V8 corpus header is schema=%q version=%d", manifest.Schema, manifest.Version)
	}
	if manifest.DiscoveryCaseCount != closedLoopV8RoleSize || manifest.HeldOutCaseCount != closedLoopV8RoleSize || len(manifest.Entries) != closedLoopV8RoleSize {
		t.Fatalf("V8 corpus counts are discovery=%d held_out=%d public_entries=%d", manifest.DiscoveryCaseCount, manifest.HeldOutCaseCount, len(manifest.Entries))
	}
	assertClosedLoopV8Commitments(t, manifest)
	assertClosedLoopV8Seal(t, manifest)
	assertClosedLoopV8Entries(t, manifest)
	assertClosedLoopV8FileSet(t, manifest)
}

func assertClosedLoopV8Commitments(t *testing.T, manifest corpuspublication.ManifestV8) {
	t.Helper()
	wantCommits := corpuspublication.Commits{
		StartingCommit:        "7333f7d604fcadc447bbe75f38255bfc56566262",
		ContractFreezeCommit:  "03bdfefa161c135ef7d8a4fef07d35445fbdfc2b",
		AuthoringPacketCommit: "58cea3d17af74033a3f3f9ca23c9549a12b8eb4c",
		ValidatorCommit:       "a930943cc8d2b7aa08a6f771a26d6239bf2be01f",
		FreezeParentCommit:    "c614ade0e623732f93686b433000366232873d04",
	}
	if manifest.Commits != wantCommits || manifest.PolicySHA256 != closedLoopV8PolicyHash {
		t.Fatal("V8 corpus lineage or policy commitment changed")
	}
	specRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion")
	packetRoot := filepath.Join(specRoot, "v8-authoring-packet")
	commitments := []struct{ path, want string }{
		{filepath.Join(specRoot, "V8_CONTRACT.sha256"), manifest.ContractManifestSHA256},
		{filepath.Join(specRoot, "V8_VALIDATOR_CONTRACT.sha256"), manifest.ValidatorManifestSHA256},
		{filepath.Join(specRoot, "V8_PUBLISHER.sha256"), manifest.PublisherManifestSHA256},
		{filepath.Join(specRoot, "V8_HISTORICAL_COMMITMENTS.json"), manifest.HistoricalCommitmentsSHA256},
		{filepath.Join(packetRoot, "PACKET_SET.sha256"), manifest.PacketSetSHA256},
		{filepath.Join(packetRoot, "CONTRACT_BINDING.json"), manifest.ContractBindingSHA256},
		{filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ValidationFileV8), manifest.ValidationReportSHA256},
	}
	for _, commitment := range commitments {
		if got := corpusHash(mustCorpusRead(t, commitment.path)); got != commitment.want {
			t.Fatalf("V8 commitment no longer matches %s", filepath.Base(commitment.path))
		}
	}
	for author, want := range manifest.AuthorPacketSHA256 {
		path := filepath.Join(packetRoot, fmt.Sprintf("%s_PACKET.sha256", strings.ToUpper(author)))
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V8 author-packet commitment no longer matches %s", author)
		}
	}
	for author, want := range manifest.AssignmentSHA256 {
		path := filepath.Join(packetRoot, "assignments", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V8 assignment commitment no longer matches %s", author)
		}
	}
	if len(manifest.AuthorshipSHA256) != 6 {
		t.Fatal("V8 authorship commitment set is incomplete")
	}
	for author, digest := range manifest.AuthorshipSHA256 {
		if !closedLoopV8ValidHash(digest) {
			t.Fatalf("V8 authorship commitment is invalid for %s", author)
		}
	}
}

func assertClosedLoopV8Seal(t *testing.T, manifest corpuspublication.ManifestV8) {
	t.Helper()
	seal := manifest.HeldOutSource
	if seal.Algorithm != corpuspublication.SealAlgorithmV8 || seal.File != corpuspublication.HeldOutCipherFileV8 ||
		seal.NonceBytes != 12 || seal.RecordCount != closedLoopV8RoleSize || len(seal.RecordCiphertextSHA256) != closedLoopV8RoleSize {
		t.Fatal("V8 held-out seal metadata is invalid")
	}
	for name, digest := range map[string]string{
		"ciphertext": seal.CiphertextSHA256, "plaintext": seal.PlaintextAggregateSHA256,
		"aad": seal.AADAggregateSHA256, "metadata": seal.MetadataAggregateSHA256,
	} {
		if !closedLoopV8ValidHash(digest) {
			t.Fatalf("V8 held-out %s commitment is invalid", name)
		}
	}
	for _, digest := range seal.RecordCiphertextSHA256 {
		if !closedLoopV8ValidHash(digest) {
			t.Fatal("V8 held-out record commitment is invalid")
		}
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextSHA256 {
		t.Fatal("V8 held-out ciphertext differs from its commitment")
	}
	for _, forbidden := range []string{"held_out", "authorship"} {
		if _, err := os.Stat(filepath.Join(closedLoopV8CorpusRoot, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("V8 private %s data must not exist in the repository", forbidden)
		}
	}
}

func assertClosedLoopV8Entries(t *testing.T, manifest corpuspublication.ManifestV8) {
	t.Helper()
	seenRequirement, seenNeutral, seenNormalized := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for index, entry := range manifest.Entries {
		wantID := fmt.Sprintf("v8_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", index/3+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json"))
		if entry.ID != wantID || entry.AuthorSlot != wantAuthor || entry.Role != "discovery" || entry.StablePath != wantPath || entry.Sealed {
			t.Fatalf("V8 corpus entry %d identity changed: %+v", index+1, entry)
		}
		if !closedLoopV8ValidHash(entry.RequirementSHA256) || !closedLoopV8ValidHash(entry.NeutralSemanticSHA256) ||
			!closedLoopV8ValidHash(entry.NormalizedSemanticSHA256) {
			t.Fatalf("V8 corpus entry %d contains an invalid digest", index+1)
		}
		if seenRequirement[entry.RequirementSHA256] || seenNeutral[entry.NeutralSemanticSHA256] || seenNormalized[entry.NormalizedSemanticSHA256] {
			t.Fatalf("V8 corpus entry %d duplicates a frozen source or semantic digest", index+1)
		}
		seenRequirement[entry.RequirementSHA256] = true
		seenNeutral[entry.NeutralSemanticSHA256] = true
		seenNormalized[entry.NormalizedSemanticSHA256] = true
		data := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(data) != entry.RequirementSHA256 {
			t.Fatalf("V8 discovery entry %d differs from its commitment", index+1)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 || closedLoopNeutralRequirementHash(t, requirement) != entry.NeutralSemanticSHA256 {
			t.Fatalf("V8 discovery entry %d violates its public or neutral semantic commitment", index+1)
		}
	}
}

func assertClosedLoopV8FileSet(t *testing.T, manifest corpuspublication.ManifestV8) {
	t.Helper()
	want := map[string]bool{
		corpuspublication.AuditFileV8: true, corpuspublication.ChecksumFileV8: true,
		corpuspublication.HeldOutCipherFileV8: true, corpuspublication.ManifestFileV8: true,
		corpuspublication.ValidationFileV8: true, corpuspublication.DiscoveryObligationsFileV8: true,
		corpuspublication.HeldOutCommitmentFileV8: true,
	}
	for _, entry := range manifest.Entries {
		want[entry.StablePath] = true
	}
	var got []string
	if err := filepath.WalkDir(closedLoopV8CorpusRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic path is not allowed: %s", path)
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV8CorpusRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk V8 corpus: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("V8 corpus has %d files; want exactly %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("V8 corpus contains unexpected file %s", path)
		}
	}
}

func closedLoopV8ValidHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
