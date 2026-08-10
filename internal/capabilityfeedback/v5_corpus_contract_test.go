package capabilityfeedback

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV5CorpusRoot         = "testdata/closed_loop_open_set_v5_corpus"
	closedLoopV5CorpusManifestHash = "d703608d09d7d7bd834bb45698446dd03bb0dbe7b00733b636dd73250cac3f6d"
	closedLoopV5CorpusSize         = 36
	closedLoopV5RoleSize           = closedLoopV5CorpusSize / 2
	closedLoopV5AuthorSize         = 6
)

func TestClosedLoopV5CorpusFreeze(t *testing.T) {
	manifestPath := filepath.Join(closedLoopV5CorpusRoot, corpuspublication.ManifestFile)
	manifestBytes := mustCorpusRead(t, manifestPath)
	if got := corpusHash(manifestBytes); got != closedLoopV5CorpusManifestHash {
		t.Fatalf("V5 corpus manifest hash mismatch: got %s, want %s", got, closedLoopV5CorpusManifestHash)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV5CorpusRoot,
		filepath.Join(closedLoopV5CorpusRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V5 corpus checksums: %v", err)
	}

	var manifest corpuspublication.Manifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != corpuspublication.ManifestSchema || manifest.Version != corpuspublication.ManifestVersion ||
		manifest.DiscoveryCaseCount != closedLoopV5RoleSize || manifest.HeldOutCaseCount != closedLoopV5RoleSize ||
		len(manifest.Entries) != closedLoopV5CorpusSize {
		t.Fatal("V5 corpus header or case counts are invalid")
	}
	assertClosedLoopV5Commitments(t, manifest)
	assertClosedLoopV5Seal(t, manifest)
	assertClosedLoopV5Entries(t, manifest)
	assertClosedLoopV5FileSet(t, manifest)
}

func assertClosedLoopV5Commitments(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	specRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion")
	commitments := []struct {
		path string
		want string
	}{
		{filepath.Join(specRoot, "V5_CONTRACT.sha256"), manifest.ContractManifestSHA256},
		{filepath.Join(specRoot, "V5_VALIDATOR.sha256"), manifest.ValidatorManifestSHA256},
		{filepath.Join(specRoot, "V5_PUBLISHER.sha256"), manifest.PublisherManifestSHA256},
		{filepath.Join(specRoot, "V5_HISTORICAL_COMMITMENTS.json"), manifest.HistoricalCommitmentsSHA256},
		{filepath.Join(closedLoopV5CorpusRoot, corpuspublication.ValidationFile), manifest.ValidationReportSHA256},
	}
	for _, commitment := range commitments {
		if got := corpusHash(mustCorpusRead(t, commitment.path)); got != commitment.want {
			t.Fatalf("V5 commitment no longer matches %s", filepath.Base(commitment.path))
		}
	}
	for author, want := range manifest.AuthorshipSHA256 {
		path := filepath.Join(closedLoopV5CorpusRoot, "authorship", author+".json")
		if got := corpusHash(mustCorpusRead(t, path)); got != want {
			t.Fatalf("V5 authorship commitment no longer matches %s", author)
		}
	}
}

func assertClosedLoopV5Seal(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	seal := manifest.HeldOutSource
	if seal.Algorithm != corpuspublication.SealAlgorithm || seal.File != corpuspublication.HeldOutCipherFile ||
		seal.NonceBytes != 12 || seal.CaseCount != closedLoopV5RoleSize ||
		!closedLoopV5ValidHash(seal.PayloadSHA256) || !closedLoopV5ValidHash(seal.CiphertextSHA256) ||
		!closedLoopV5ValidHash(seal.AADSHA256) {
		t.Fatal("V5 held-out seal metadata is invalid")
	}
	ciphertext := mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, seal.File))
	if corpusHash(ciphertext) != seal.CiphertextSHA256 || len(ciphertext) <= seal.NonceBytes+aes.BlockSize {
		t.Fatal("V5 held-out ciphertext differs from its authenticated commitment")
	}
	if _, err := os.Stat(filepath.Join(closedLoopV5CorpusRoot, "held_out")); !os.IsNotExist(err) {
		t.Fatal("V5 held-out plaintext must not exist in the repository")
	}
}

func assertClosedLoopV5Entries(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	seenRequirement := map[string]bool{}
	seenNeutral := map[string]bool{}
	seenNormalized := map[string]bool{}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	for index, entry := range manifest.Entries {
		role, offset := "discovery", index
		if index >= closedLoopV5RoleSize {
			role, offset = "held_out", index-closedLoopV5RoleSize
		}
		wantID := fmt.Sprintf("v5_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", offset/closedLoopV5AuthorSize+1)
		wantPath := fmt.Sprintf("%s/%s.json", role, wantID)
		if entry.ID != wantID || entry.Role != role || entry.AuthorSlot != wantAuthor ||
			entry.Domain != domains[offset%len(domains)] || entry.StablePath != wantPath || entry.Sealed != (role == "held_out") ||
			!closedLoopV5ValidHash(entry.RequirementSHA256) || !closedLoopV5ValidHash(entry.NeutralSemanticSHA256) ||
			!closedLoopV5ValidHash(entry.NormalizedSemanticSHA256) || seenRequirement[entry.RequirementSHA256] ||
			seenNeutral[entry.NeutralSemanticSHA256] || seenNormalized[entry.NormalizedSemanticSHA256] {
			t.Fatalf("V5 corpus entry %d violates its frozen identity or diversity contract", index+1)
		}
		seenRequirement[entry.RequirementSHA256] = true
		seenNeutral[entry.NeutralSemanticSHA256] = true
		seenNormalized[entry.NormalizedSemanticSHA256] = true
		counts[role][entry.Domain]++
		if role == "held_out" {
			continue
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV5CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(data) != entry.RequirementSHA256 {
			t.Fatalf("V5 discovery entry %d differs from its source commitment", index+1)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 || closedLoopNeutralRequirementHash(t, requirement) != entry.NeutralSemanticSHA256 {
			t.Fatalf("V5 discovery entry %d violates its public or neutral semantic commitment", index+1)
		}
	}
	if !reflect.DeepEqual(counts, manifest.Counts) {
		t.Fatal("V5 manifest domain counts differ from its entries")
	}
}

func assertClosedLoopV5FileSet(t *testing.T, manifest corpuspublication.Manifest) {
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
	if err := filepath.WalkDir(closedLoopV5CorpusRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic path is not allowed: %s", path)
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(closedLoopV5CorpusRoot, path)
			if relErr != nil {
				return relErr
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk V5 corpus: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("V5 corpus has %d files; want exactly %d", len(got), len(want))
	}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("V5 corpus contains unexpected file %s", path)
		}
	}
}

func closedLoopV5ValidHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}
