package capabilityfeedback

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilityevaluation"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV2CorpusSchema  = "kicadai.closed-loop-open-set-corpus.v2"
	closedLoopV2CorpusVersion = 2
	closedLoopV2CorpusRoot    = "testdata/closed_loop_open_set_v2_corpus"
	closedLoopV2StartCommit   = "8bdc31e668152b7324066bd75182d86d7320d3f8"
)

type closedLoopV2Manifest struct {
	Schema               string                              `json:"schema"`
	Version              int                                 `json:"version"`
	StartingCommit       string                              `json:"starting_commit"`
	RequirementSchema    string                              `json:"requirement_schema"`
	EvaluatorPolicy      string                              `json:"evaluator_policy"`
	ImpactRegistry       capabilityevaluation.ImpactRegistry `json:"impact_registry"`
	ImpactRegistryHash   string                              `json:"impact_registry_hash"`
	SynthesisPolicy      ots.Policy                          `json:"synthesis_policy"`
	SynthesisPolicyHash  string                              `json:"synthesis_policy_hash"`
	Environment          closedLoopEnvironment               `json:"environment"`
	AuthorManifestHash   string                              `json:"author_manifest_sha256"`
	AuthorshipRecordHash string                              `json:"authorship_record_sha256"`
	Entries              []closedLoopV2ManifestEntry         `json:"entries"`
}

type closedLoopV2ManifestEntry struct {
	ID              string                            `json:"id"`
	Role            CorpusRole                        `json:"role"`
	Domain          capabilityevaluation.Domain       `json:"domain"`
	SafetyImpact    capabilityevaluation.SafetyImpact `json:"safety_impact"`
	SourceID        string                            `json:"source_id"`
	RequirementFile string                            `json:"requirement_file"`
	RequirementHash string                            `json:"requirement_sha256"`
}

func TestClosedLoopV2CorpusFreeze(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.json"))
	checksum := strings.TrimSpace(string(mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "manifest.sha256"))))
	wantChecksum := corpusHash(manifestBytes) + "  manifest.json"
	if checksum != wantChecksum {
		t.Fatalf("V2 manifest checksum = %q, want %q", checksum, wantChecksum)
	}

	var manifest closedLoopV2Manifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	expected := buildClosedLoopV2Manifest(t)
	if expectedBytes := corpusJSON(t, expected); !bytes.Equal(manifestBytes, expectedBytes) {
		t.Fatal("V2 corpus manifest bytes drifted from the frozen independent-author contract")
	}
	if len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatalf("V2 manifest entries = %d, want %d", len(manifest.Entries), closedLoopCorpusSize)
	}

	seenSemantic := map[string]string{}
	for _, entry := range manifest.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		if got := corpusHash(data); got != entry.RequirementHash {
			t.Fatalf("%s requirement hash = %s, want %s", entry.ID, got, entry.RequirementHash)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s strict requirement issues: %#v", entry.ID, issues)
		}
		assertBehaviorOnly(t, entry.ID, data)
		semantic := closedLoopNeutralRequirementHash(t, requirement)
		if prior, duplicate := seenSemantic[semantic]; duplicate {
			t.Fatalf("%s duplicates normalized V2 requirement %s", entry.ID, prior)
		}
		seenSemantic[semantic] = entry.ID
	}
}

func TestUpdateClosedLoopV2CorpusManifest(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_V2_CORPUS") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V2_CORPUS=1 to regenerate the V2 manifest from frozen authored bytes")
	}
	manifestBytes := corpusJSON(t, buildClosedLoopV2Manifest(t))
	if err := os.WriteFile(filepath.Join(closedLoopV2CorpusRoot, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := []byte(corpusHash(manifestBytes) + "  manifest.json\n")
	if err := os.WriteFile(filepath.Join(closedLoopV2CorpusRoot, "manifest.sha256"), checksum, 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildClosedLoopV2Manifest(t *testing.T) closedLoopV2Manifest {
	t.Helper()
	authorBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "author_manifest.json"))
	var author closedLoopV2AuthorManifest
	decodeCorpusStrict(t, authorBytes, &author)
	if author.Schema != closedLoopV2AuthorManifestSchema || author.Version != 2 || len(author.Entries) != closedLoopCorpusSize {
		t.Fatalf("invalid V2 author manifest header or size")
	}
	registry := closedLoopImpactRegistry()
	_, _, registryHash, err := normalizeImpactRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	policy := closedLoopSynthesisPolicy()
	policyHash, err := digest(policy)
	if err != nil {
		t.Fatal(err)
	}
	manifest := closedLoopV2Manifest{
		Schema: closedLoopV2CorpusSchema, Version: closedLoopV2CorpusVersion,
		StartingCommit: closedLoopV2StartCommit, RequirementSchema: ots.RequirementSchema,
		EvaluatorPolicy: PolicyVersion, ImpactRegistry: registry, ImpactRegistryHash: registryHash,
		SynthesisPolicy: policy, SynthesisPolicyHash: policyHash,
		Environment:          closedLoopEnvironment{GoMinimum: frozenGoMinimum, KiCad: frozenKiCadVersion, OS: frozenOperatingSystem, Arch: frozenProcessorArchitecture},
		AuthorManifestHash:   corpusHash(authorBytes),
		AuthorshipRecordHash: corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "AUTHORSHIP.md"))),
	}
	for index, entry := range author.Entries {
		wantID := fmt.Sprintf("v2_case_%03d", index+1)
		wantRole, wantDirectory := RoleDiscovery, "discovery"
		if index >= closedLoopCorpusSize/2 {
			wantRole, wantDirectory = RoleHeldOut, "held_out"
		}
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || entry.Role != wantRole || entry.RequirementFile != wantFile {
			t.Fatalf("invalid V2 author identity, role, or path at entry %d", index)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		manifest.Entries = append(manifest.Entries, closedLoopV2ManifestEntry{
			ID: entry.ID, Role: entry.Role, Domain: capabilityevaluation.Domain(entry.Domain),
			SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact), SourceID: entry.SourceID,
			RequirementFile: entry.RequirementFile, RequirementHash: corpusHash(data),
		})
	}
	return manifest
}

func TestClosedLoopV2ManifestReorderAndRoleLeakageFailClosed(t *testing.T) {
	expected := buildClosedLoopV2Manifest(t)
	manifest := expected
	manifest.Entries = slices.Clone(expected.Entries)
	manifest.Entries[0], manifest.Entries[1] = manifest.Entries[1], manifest.Entries[0]
	if bytes.Equal(corpusJSON(t, manifest), corpusJSON(t, expected)) {
		t.Fatal("V2 manifest reorder did not change frozen bytes")
	}
	for _, entry := range expected.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		if bytes.Contains(data, []byte(`"role"`)) || bytes.Contains(data, []byte(`"safety_impact"`)) || bytes.Contains(data, []byte(entry.SourceID)) {
			t.Fatalf("%s reporting or provenance metadata leaked into requirement bytes", entry.ID)
		}
	}
}

func TestClosedLoopV2CorpusIdentityDoesNotLeakIntoProductionGo(t *testing.T) {
	identities := []string{regexp.QuoteMeta("closed_loop_open_set_v2_corpus")}
	for index := range closedLoopCorpusSize {
		identities = append(identities, regexp.QuoteMeta(fmt.Sprintf("v2_case_%03d", index+1)))
	}
	pattern := regexp.MustCompile(strings.Join(identities, "|"))
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			if match := pattern.Find(scanner.Bytes()); match != nil {
				t.Errorf("production file %s contains frozen V2 corpus identity %q", path, match)
				break
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
