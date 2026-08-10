package capabilityfeedback

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV4CandidateRoot         = "testdata/closed_loop_open_set_v4_candidate"
	closedLoopV4AuthorManifestSchema  = "kicadai.closed-loop-open-set-author-manifest.v4"
	closedLoopV4ValidationEnvironment = "VALIDATE_CLOSED_LOOP_V4_CANDIDATE"
)

type closedLoopV4AuthorManifest struct {
	Schema  string                            `json:"schema"`
	Version int                               `json:"version"`
	Entries []closedLoopV4AuthorManifestEntry `json:"entries"`
}

type closedLoopV4AuthorManifestEntry struct {
	ID              string     `json:"id"`
	Role            CorpusRole `json:"role"`
	Domain          string     `json:"domain"`
	SafetyImpact    string     `json:"safety_impact"`
	SourceID        string     `json:"source_id"`
	RequirementFile string     `json:"requirement_file"`
}

// TestClosedLoopV4CandidateQuarantine is content-blind with respect to
// synthesis, feasibility, classification, and outcomes. It performs only the
// public authoring-contract checks allowed before corpus freeze.
func TestClosedLoopV4CandidateQuarantine(t *testing.T) {
	if _, err := os.Stat(closedLoopV4CandidateRoot); err != nil {
		if os.IsNotExist(err) && os.Getenv(closedLoopV4ValidationEnvironment) == "" {
			t.Skip("V4 candidate quarantine is absent")
		}
		t.Fatalf("V4 candidate quarantine is unavailable: %v", err)
	}

	packetRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v4-authoring-packet")
	packetManifest := mustCorpusRead(t, filepath.Join(packetRoot, "AUTHOR_MANIFEST.json"))
	candidateManifest := mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, "AUTHOR_MANIFEST.json"))
	if !bytes.Equal(candidateManifest, packetManifest) {
		t.Fatal("V4 candidate author manifest differs from the frozen packet")
	}
	var manifest closedLoopV4AuthorManifest
	decodeCorpusStrict(t, candidateManifest, &manifest)
	if manifest.Schema != closedLoopV4AuthorManifestSchema || manifest.Version != 4 || len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatal("V4 candidate manifest header or size is invalid")
	}
	closedLoopV4ValidateCandidateFileSet(t, manifest)

	authorship := string(mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, "AUTHORSHIP.md")))
	for _, required := range []string{
		"was my only task input", "no repository", "independently conceived all 24", "did not synthesize",
		"no requirement prescribes an implementation", "Signed/attested by:",
	} {
		if !strings.Contains(authorship, required) {
			t.Fatalf("V4 candidate authorship record omits required attestation %q", required)
		}
	}
	if strings.ContainsAny(authorship, "[]") {
		t.Fatal("V4 candidate authorship record contains an unresolved template field")
	}

	seenRaw := closedLoopV4PriorRawHashes(t)
	seenSemantic := closedLoopV1NeutralSemanticHashes(t)
	for hash, id := range closedLoopV2NeutralSemanticHashes(t) {
		seenSemantic[hash] = id
	}
	for hash, id := range closedLoopV3DiscoveryNeutralSemanticHashes(t) {
		seenSemantic[hash] = id
	}
	diversity := map[CorpusRole]*closedLoopV3Diversity{
		RoleDiscovery: newClosedLoopV3Diversity(),
		RoleHeldOut:   newClosedLoopV3Diversity(),
	}
	counts := map[CorpusRole]map[string]int{RoleDiscovery: {}, RoleHeldOut: {}}
	for index, entry := range manifest.Entries {
		wantRole, wantDirectory := RoleDiscovery, "discovery"
		if index >= closedLoopCorpusSize/2 {
			wantRole, wantDirectory = RoleHeldOut, "held_out"
		}
		wantID := fmt.Sprintf("v4_case_%03d", index+1)
		wantSource := fmt.Sprintf("v4_source_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || entry.SourceID != wantSource || entry.Role != wantRole || entry.RequirementFile != wantFile {
			t.Fatalf("V4 candidate entry %d has invalid identity, role, source, or path", index+1)
		}
		counts[entry.Role][entry.Domain]++

		data := mustCorpusRead(t, filepath.Join(closedLoopV4CandidateRoot, filepath.FromSlash(entry.RequirementFile)))
		rawHash := corpusHash(data)
		if prior, duplicate := seenRaw[rawHash]; duplicate {
			t.Fatalf("%s duplicates raw requirement bytes from %s", entry.ID, prior)
		}
		seenRaw[rawHash] = entry.ID
		if bytes.Contains(data, []byte(entry.ID)) || bytes.Contains(data, []byte(entry.SourceID)) {
			t.Fatalf("%s leaks manifest identity into requirement bytes", entry.ID)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s violates the public requirement contract (%d issues)", entry.ID, len(issues))
		}
		if closedLoopV3ContainsImplementationLanguage(t, data) {
			t.Fatalf("%s contains prohibited implementation language", entry.ID)
		}
		if len(requirement.Requirements.OperatingCases) < 2 || len(requirement.Requirements.BehavioralRequirements) < 4 {
			t.Fatalf("%s does not meet minimum operating-case/assertion counts", entry.ID)
		}
		analyses := map[string]bool{}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			analyses[assertion.Analysis] = true
			if assertion.Metric == "thd" {
				t.Fatalf("%s uses the prohibited legacy metric alias thd", entry.ID)
			}
		}
		if len(analyses) < 2 {
			t.Fatalf("%s does not meet the minimum analysis-kind count", entry.ID)
		}
		if !closedLoopV2AllAcceptanceGates(requirement.Acceptance) {
			t.Fatalf("%s does not require all acceptance gates", entry.ID)
		}
		semantic := closedLoopNeutralRequirementHash(t, requirement)
		if prior, duplicate := seenSemantic[semantic]; duplicate {
			t.Fatalf("%s duplicates a normalized visible requirement from %s", entry.ID, prior)
		}
		seenSemantic[semantic] = entry.ID
		diversity[entry.Role].observe(entry.Domain, requirement)
	}
	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		for _, domain := range []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"} {
			if counts[role][domain] != 2 {
				t.Fatalf("V4 %s/%s count = %d, want 2", role, domain, counts[role][domain])
			}
		}
		closedLoopV4ValidateDiversity(t, role, diversity[role])
	}
}

func closedLoopV4ValidateCandidateFileSet(t *testing.T, manifest closedLoopV4AuthorManifest) {
	t.Helper()
	want := map[string]bool{"AUTHOR_MANIFEST.json": true, "AUTHORSHIP.md": true}
	for _, entry := range manifest.Entries {
		want[filepath.FromSlash(entry.RequirementFile)] = true
	}
	err := filepath.WalkDir(closedLoopV4CandidateRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path == closedLoopV4CandidateRoot {
			return nil
		}
		relative, err := filepath.Rel(closedLoopV4CandidateRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative != "discovery" && relative != "held_out" {
				return fmt.Errorf("unexpected candidate directory %s", relative)
			}
			return nil
		}
		if !want[relative] {
			return fmt.Errorf("unexpected candidate file %s", relative)
		}
		delete(want, relative)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 0 {
		t.Fatalf("V4 candidate bundle omits %d required files", len(want))
	}
}

func closedLoopV4ValidateDiversity(t *testing.T, role CorpusRole, diversity *closedLoopV3Diversity) {
	t.Helper()
	for name, values := range map[string]map[string]bool{
		"supply configuration": diversity.supplyConfigurations,
		"observation kind":     diversity.observations,
		"analysis category":    diversity.analyses,
		"variation category":   diversity.variations,
	} {
		minimum := 3
		if name == "variation category" {
			minimum = 4
		}
		if len(values) < minimum {
			t.Fatalf("V4 %s %s diversity = %d, want at least %d", role, name, len(values), minimum)
		}
	}
	for _, event := range []string{"input_step", "load_step", "power_step", "startup", "rail_loss", "short_circuit"} {
		if !diversity.events[event] {
			t.Fatalf("V4 %s omits required event category %s", role, event)
		}
	}
	if diversity.multiOutput < 3 || diversity.convergingExcitations < 3 || len(diversity.criticalDomains) < 3 {
		t.Fatalf("V4 %s structural diversity = multi-output:%d converging:%d critical-domains:%d; want at least 3 each",
			role, diversity.multiOutput, diversity.convergingExcitations, len(diversity.criticalDomains))
	}
}

// The retired V3 held-out plaintext is never opened. Its committed raw hashes
// still reject byte-identical reuse, while the isolated author has no access
// from which to derive a semantic transformation.
func closedLoopV4PriorRawHashes(t *testing.T) map[string]string {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	var manifest closedLoopV3CorpusManifest
	decodeCorpusStrict(t, data, &manifest)
	result := make(map[string]string, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		result[entry.RequirementHash] = entry.ID
	}
	return result
}

func closedLoopV3DiscoveryNeutralSemanticHashes(t *testing.T) map[string]string {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	var manifest closedLoopV3CorpusManifest
	decodeCorpusStrict(t, data, &manifest)
	result := map[string]string{}
	for _, entry := range manifest.Entries {
		if entry.Sealed {
			continue
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("frozen V3 %s violates its requirement contract", entry.ID)
		}
		result[closedLoopNeutralRequirementHash(t, requirement)] = entry.ID
	}
	return result
}
