package opentopologysynthesis

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	multiBranchCorpusSchema       = "kicadai.multi-branch-analog-corpus.v1"
	multiBranchCorpusBaseCommit   = "8f6ac90426b308b69cfcccbda9146be9bf6cc5f0"
	multiBranchCorpusManifestHash = "86b80502caa7fe5bf4a0e94dc6af2f7cc543f310789978f1de78f1784419d8ec"
	multiBranchCorpusCaseCount    = 2
)

var multiBranchCorpusStages = []string{
	"integrity",
	"schema",
	"primitive_inventory",
	"topology_search",
	"value_derivation",
	"simulation",
	"repair",
	"lowering",
	"schematic",
	"placement",
	"routing",
	"writer",
	"erc",
	"drc",
	"round_trip",
	"replay",
}

type multiBranchCorpusManifest struct {
	Schema            string                          `json:"schema"`
	Version           int                             `json:"version"`
	BaseCommit        string                          `json:"base_commit"`
	FrozenAt          string                          `json:"frozen_at"`
	RequirementSchema string                          `json:"requirement_schema"`
	AuthoringPolicy   string                          `json:"authoring_policy"`
	Stages            []string                        `json:"stages"`
	Cases             []multiBranchCorpusManifestCase `json:"cases"`
}

type multiBranchCorpusManifestCase struct {
	ID                string   `json:"id"`
	BehaviorFamily    string   `json:"behavior_family"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	SafetyCritical    bool     `json:"safety_critical"`
}

func TestMultiBranchAnalogCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := filepath.Join("testdata", "multi_branch_analog_corpus")
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != multiBranchCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, multiBranchCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != multiBranchCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest multiBranchCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != multiBranchCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != multiBranchCorpusBaseCommit ||
		manifest.RequirementSchema != frozenRequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, multiBranchCorpusStages) {
		t.Fatalf("stages = %v, want %v", manifest.Stages, multiBranchCorpusStages)
	}
	if len(manifest.Cases) != multiBranchCorpusCaseCount {
		t.Fatalf("case count = %d, want %d", len(manifest.Cases), multiBranchCorpusCaseCount)
	}

	seenFamilies := map[string]bool{}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID {
			t.Fatalf("case IDs are not strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if strings.TrimSpace(entry.BehaviorFamily) == "" || seenFamilies[entry.BehaviorFamily] {
			t.Fatalf("%s behavior family = %q", entry.ID, entry.BehaviorFamily)
		}
		seenFamilies[entry.BehaviorFamily] = true
		if len(entry.RequiredAnalyses) < 3 || !slices.IsSorted(entry.RequiredAnalyses) {
			t.Fatalf("%s required analyses = %v", entry.ID, entry.RequiredAnalyses)
		}
		path := filepath.Join(root, entry.RequirementFile)
		if filepath.Base(path) != entry.RequirementFile || seenFiles[entry.RequirementFile] {
			t.Fatalf("%s unsafe or duplicate requirement file %q", entry.ID, entry.RequirementFile)
		}
		seenFiles[entry.RequirementFile] = true
		data := mustRead(t, path)
		if got := frozenHash(data); got != entry.RequirementSHA256 {
			t.Fatalf("%s sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectFrozenImplementationDetail(t, entry.ID, data)
		var frozen frozenRequirement
		decodeFrozenStrict(t, data, &frozen)
		validateFrozenRequirement(t, frozenManifestCase{
			ID:                entry.ID,
			BehaviorFamily:    entry.BehaviorFamily,
			RequirementFile:   entry.RequirementFile,
			RequirementSHA256: entry.RequirementSHA256,
			RequiredAnalyses:  entry.RequiredAnalyses,
			SafetyCritical:    entry.SafetyCritical,
		}, frozen)
		decoded, issues := DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s requirement is invalid: %#v", entry.ID, issues)
		}
		if decoded.Project.Name != entry.ID {
			t.Fatalf("%s decoded project name = %q", entry.ID, decoded.Project.Name)
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if !seenFiles[filepath.Base(path)] {
			t.Errorf("unmanifested corpus file %s", filepath.Base(path))
		}
	}
}
