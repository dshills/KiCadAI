package opentopologysynthesis

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	protectedCurrentOutputCorpusSchema       = "kicadai.protected-current-output-corpus.v1"
	protectedCurrentOutputCorpusBaseCommit   = "86c7a6e2de158ed8dd46530c87d1a9c503073131"
	protectedCurrentOutputCorpusManifestHash = "229cd7821a7ad5cf29ae0bad21f57f40f9e833a5d40cb2e6ce376caa33d8462f"
	protectedCurrentOutputCorpusCaseCount    = 3
)

type protectedCurrentOutputCorpusManifest struct {
	Schema            string                             `json:"schema"`
	Version           int                                `json:"version"`
	BaseCommit        string                             `json:"base_commit"`
	FrozenAt          string                             `json:"frozen_at"`
	RequirementSchema string                             `json:"requirement_schema"`
	AuthoringPolicy   string                             `json:"authoring_policy"`
	Cases             []protectedCurrentOutputCorpusCase `json:"cases"`
}

type protectedCurrentOutputCorpusCase struct {
	ID                string   `json:"id"`
	CurrentRole       string   `json:"current_role"`
	Independence      string   `json:"independence"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	RequiredBehaviors []string `json:"required_behaviors"`
}

func TestProtectedCurrentOutputCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := protectedCurrentOutputCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != protectedCurrentOutputCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, protectedCurrentOutputCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != protectedCurrentOutputCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest protectedCurrentOutputCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != protectedCurrentOutputCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != protectedCurrentOutputCorpusBaseCommit ||
		manifest.RequirementSchema != RequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		len(manifest.Cases) != protectedCurrentOutputCorpusCaseCount {
		t.Fatalf("manifest identity = %#v", manifest)
	}

	seenIDs := map[string]bool{}
	seenHashes := map[string]bool{}
	roles := map[string]int{}
	independentVariants := 0
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID || seenIDs[entry.ID] {
			t.Fatalf("case IDs are not unique and sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		seenIDs[entry.ID] = true
		if entry.CurrentRole != "source" && entry.CurrentRole != "sink" {
			t.Fatalf("%s current role = %q", entry.ID, entry.CurrentRole)
		}
		roles[entry.CurrentRole]++
		if strings.TrimSpace(entry.Independence) == "" || seenHashes[entry.RequirementSHA256] {
			t.Fatalf("%s independence/hash = %q/%q", entry.ID, entry.Independence, entry.RequirementSHA256)
		}
		seenHashes[entry.RequirementSHA256] = true
		if !strings.HasPrefix(entry.RequirementFile, "../") {
			independentVariants++
		}
		if len(entry.RequiredAnalyses) < 4 || !slices.IsSorted(entry.RequiredAnalyses) ||
			len(entry.RequiredBehaviors) < 5 || !slices.IsSorted(entry.RequiredBehaviors) {
			t.Fatalf("%s coverage is incomplete: analyses=%v behaviors=%v", entry.ID, entry.RequiredAnalyses, entry.RequiredBehaviors)
		}

		path := filepath.Clean(filepath.Join(root, entry.RequirementFile))
		relative, err := filepath.Rel("testdata", path)
		if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("%s requirement escapes frozen testdata: %q", entry.ID, entry.RequirementFile)
		}
		data := mustRead(t, path)
		if got := frozenHash(data); got != entry.RequirementSHA256 {
			t.Fatalf("%s sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectFrozenImplementationDetail(t, entry.ID, data)
		requirement, issues := DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s requirement is invalid: %#v", entry.ID, issues)
		}
		if requirement.Project.Name != entry.ID {
			t.Fatalf("%s project name = %q", entry.ID, requirement.Project.Name)
		}
		analyses := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
		behaviors := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if !slices.Contains(analyses, assertion.Analysis) {
				analyses = append(analyses, assertion.Analysis)
			}
			behaviors = append(behaviors, assertion.ID)
		}
		slices.Sort(analyses)
		slices.Sort(behaviors)
		if !slices.Equal(analyses, entry.RequiredAnalyses) {
			t.Fatalf("%s analyses = %v, want %v", entry.ID, analyses, entry.RequiredAnalyses)
		}
		if !slices.Equal(behaviors, entry.RequiredBehaviors) {
			t.Fatalf("%s behaviors = %v, want %v", entry.ID, behaviors, entry.RequiredBehaviors)
		}
		matchedRole := false
		for _, port := range requirement.Requirements.Ports {
			if port.Kind == "controlled_current" && port.Direction == entry.CurrentRole {
				matchedRole = true
			}
		}
		if !matchedRole {
			t.Fatalf("%s lacks controlled-current %s port", entry.ID, entry.CurrentRole)
		}
	}
	if independentVariants < 2 || roles["source"] == 0 || roles["sink"] == 0 {
		t.Fatalf("corpus coverage variants=%d roles=%v", independentVariants, roles)
	}
}

func protectedCurrentOutputCorpusRoot() string {
	return filepath.Join("testdata", "protected_current_output_corpus")
}
