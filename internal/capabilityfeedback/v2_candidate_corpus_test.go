package capabilityfeedback

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV2AuthorManifestSchema = "kicadai.closed-loop-open-set-author-manifest.v2"
)

type closedLoopV2AuthorManifest struct {
	Schema  string                            `json:"schema"`
	Version int                               `json:"version"`
	Entries []closedLoopV2AuthorManifestEntry `json:"entries"`
}

type closedLoopV2AuthorManifestEntry struct {
	ID              string     `json:"id"`
	Role            CorpusRole `json:"role"`
	Domain          string     `json:"domain"`
	SafetyImpact    string     `json:"safety_impact"`
	SourceID        string     `json:"source_id"`
	RequirementFile string     `json:"requirement_file"`
}

var (
	closedLoopV2CaseIDPattern   = regexp.MustCompile(`^v2_case_[0-9]{3}$`)
	closedLoopV2SourceIDPattern = regexp.MustCompile(`^v2_source_[0-9]{3}$`)
)

// TestClosedLoopV2IndependentCorpus validates only independent authorship,
// behavior-schema correctness, neutrality, diversity, and separation from V1.
// It deliberately does not synthesize a case or inspect an expected outcome.
func TestClosedLoopV2IndependentCorpus(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "author_manifest.json"))
	var manifest closedLoopV2AuthorManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != closedLoopV2AuthorManifestSchema || manifest.Version != 2 ||
		len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatalf("V2 author manifest header/size = %q/%d/%d", manifest.Schema, manifest.Version, len(manifest.Entries))
	}

	wantDomains := []string{"analog", "digital", "mcu", "mixed_signal", "power", "sensor"}
	validSafety := map[string]bool{
		"non_safety": true, "review_required": true,
		"safety_relevant": true, "safety_critical": true,
	}
	counts := map[CorpusRole]map[string]int{
		RoleDiscovery: {},
		RoleHeldOut:   {},
	}
	seenSources := map[string]bool{}
	seenRequirementHashes := map[string]string{}
	v1SemanticHashes := closedLoopV1NeutralSemanticHashes(t)
	analysesByRole := map[CorpusRole]map[string]bool{
		RoleDiscovery: {},
		RoleHeldOut:   {},
	}

	for index, entry := range manifest.Entries {
		wantID := fmt.Sprintf("v2_case_%03d", index+1)
		wantRole := RoleDiscovery
		wantDirectory := "discovery"
		if index >= closedLoopCorpusSize/2 {
			wantRole = RoleHeldOut
			wantDirectory = "held_out"
		}
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || !closedLoopV2CaseIDPattern.MatchString(entry.ID) ||
			entry.Role != wantRole || entry.RequirementFile != wantFile {
			t.Fatalf("V2 author entry %d identity/role/path = %#v, want %s/%s/%s", index, entry, wantID, wantRole, wantFile)
		}
		if !slices.Contains(wantDomains, entry.Domain) || !validSafety[entry.SafetyImpact] {
			t.Fatalf("%s has invalid reporting domain or safety impact", entry.ID)
		}
		if !closedLoopV2SourceIDPattern.MatchString(entry.SourceID) || seenSources[entry.SourceID] {
			t.Fatalf("%s has invalid or duplicate opaque source ID %q", entry.ID, entry.SourceID)
		}
		seenSources[entry.SourceID] = true
		counts[entry.Role][entry.Domain]++

		data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s strict requirement issues: %#v", entry.ID, issues)
		}
		assertBehaviorOnly(t, entry.ID, data)
		if strings.TrimSpace(requirement.Project.Name) == "" ||
			strings.TrimSpace(requirement.Project.Title) == "" ||
			strings.TrimSpace(requirement.Project.Description) == "" {
			t.Fatalf("%s project identity/text is not opaque", entry.ID)
		}
		if !closedLoopV2AllAcceptanceGates(requirement.Acceptance) {
			t.Fatalf("%s does not require every strict acceptance gate", entry.ID)
		}
		semantic := closedLoopNeutralRequirementHash(t, requirement)
		if prior, duplicate := seenRequirementHashes[semantic]; duplicate {
			t.Fatalf("%s duplicates normalized V2 requirement %s", entry.ID, prior)
		}
		if prior, duplicate := v1SemanticHashes[semantic]; duplicate {
			t.Fatalf("%s duplicates normalized V1 requirement %s", entry.ID, prior)
		}
		seenRequirementHashes[semantic] = entry.ID
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			analysesByRole[entry.Role][assertion.Analysis] = true
		}
	}

	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		for _, domain := range wantDomains {
			if counts[role][domain] != 2 {
				t.Fatalf("V2 %s/%s count=%d, want 2", role, domain, counts[role][domain])
			}
		}
		if len(analysesByRole[role]) < 5 {
			t.Fatalf("V2 %s analysis diversity=%d, want at least 5", role, len(analysesByRole[role]))
		}
	}
}

func closedLoopV1NeutralSemanticHashes(t *testing.T) map[string]string {
	t.Helper()
	manifest := loadClosedLoopManifest(t)
	result := make(map[string]string, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("frozen V1 %s strict requirement issues: %#v", entry.ID, issues)
		}
		result[closedLoopNeutralRequirementHash(t, requirement)] = entry.ID
	}
	return result
}

func closedLoopNeutralRequirementHash(t *testing.T, requirement ots.Requirement) string {
	t.Helper()
	requirement.Project = ots.Project{}
	hash, err := ots.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func closedLoopV2AllAcceptanceGates(acceptance ots.Acceptance) bool {
	return acceptance.RequirePrimitiveOnly && acceptance.RequireTopologySearch &&
		acceptance.RequireSimulation && acceptance.RequireAllCorners &&
		acceptance.RequireModelProvenance && acceptance.RequireClosedLoopEvidence &&
		acceptance.RequireCompleteRouting && acceptance.RequireConnectivity &&
		acceptance.RequireWriterCorrectness && acceptance.RequireRoundTripZeroDiff &&
		acceptance.RequireERC && acceptance.RequireStrictDRC &&
		acceptance.RequireDeterministicReplay && acceptance.RequireFailClosed
}
