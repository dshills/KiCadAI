package opentopologysynthesis

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	architectureCorpusSchema       = "kicadai.simulation-grounded-architecture-corpus.v1"
	architectureBaselineSchema     = "kicadai.simulation-grounded-architecture-baseline.v1"
	architectureCorpusBaseCommit   = "92fe092ad0135bd3d1c64e457cbb6ff13f26ba18"
	architectureCorpusManifestHash = "ca269d07ca110e9e26b9a1366e042c7fe50f679461c5cc8d20945599ea8131fc"
	architectureCorpusCaseCount    = 3
)

var architectureCorpusStages = []string{
	"integrity",
	"schema",
	"primitive_inventory",
	"architecture_generation",
	"equation_sizing",
	"simulation",
	"rating_bias_thermal_rejection",
	"ranking",
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

type architectureCorpusManifest struct {
	Schema            string                           `json:"schema"`
	Version           int                              `json:"version"`
	BaseCommit        string                           `json:"base_commit"`
	FrozenAt          string                           `json:"frozen_at"`
	RequirementSchema string                           `json:"requirement_schema"`
	AuthoringPolicy   string                           `json:"authoring_policy"`
	Stages            []string                         `json:"stages"`
	Cases             []architectureCorpusManifestCase `json:"cases"`
}

type architectureCorpusManifestCase struct {
	ID                string   `json:"id"`
	BehaviorFamily    string   `json:"behavior_family"`
	AcceptanceClaim   string   `json:"acceptance_claim"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	SafetyCritical    bool     `json:"safety_critical"`
}

type architectureCorpusBaseline struct {
	Schema       string                            `json:"schema"`
	Version      int                               `json:"version"`
	BaseCommit   string                            `json:"base_commit"`
	ManifestHash string                            `json:"manifest_sha256"`
	MeasuredAt   string                            `json:"measured_at"`
	Summary      architectureCorpusBaselineSummary `json:"summary"`
	Cases        []architectureCorpusBaselineCase  `json:"cases"`
}

type architectureCorpusBaselineSummary struct {
	Total          int `json:"total"`
	CompletePasses int `json:"complete_passes"`
	FailClosed     int `json:"fail_closed"`
}

type architectureCorpusBaselineCase struct {
	ID                string `json:"id"`
	RequirementSHA256 string `json:"requirement_sha256"`
	RequirementHash   string `json:"requirement_hash"`
	Status            string `json:"status"`
	StoppedAt         string `json:"stopped_at"`
	Code              string `json:"code"`
	ProviderBypass    bool   `json:"provider_bypass"`
	EvidenceHash      string `json:"evidence_hash"`
}

func TestArchitectureCorpusIsFrozenBeforeArchitectureGrammar(t *testing.T) {
	root := architectureCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != architectureCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, architectureCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != architectureCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest architectureCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != architectureCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != architectureCorpusBaseCommit ||
		manifest.RequirementSchema != frozenRequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, architectureCorpusStages) {
		t.Fatalf("stages = %v, want %v", manifest.Stages, architectureCorpusStages)
	}
	if len(manifest.Cases) != architectureCorpusCaseCount {
		t.Fatalf("case count = %d, want %d", len(manifest.Cases), architectureCorpusCaseCount)
	}

	wantClaims := map[string]bool{
		"class_a_stage":                true,
		"class_ab_amplifier":           true,
		"non_amplifier_analog_circuit": true,
	}
	seenClaims := map[string]bool{}
	seenFamilies := map[string]bool{}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID {
			t.Fatalf("case IDs are not unique and strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if strings.TrimSpace(entry.BehaviorFamily) == "" || seenFamilies[entry.BehaviorFamily] {
			t.Fatalf("%s behavior family must be nonempty and unique", entry.ID)
		}
		seenFamilies[entry.BehaviorFamily] = true
		if !wantClaims[entry.AcceptanceClaim] || seenClaims[entry.AcceptanceClaim] {
			t.Fatalf("%s acceptance claim is missing, duplicate, or unknown: %q", entry.ID, entry.AcceptanceClaim)
		}
		seenClaims[entry.AcceptanceClaim] = true
		if len(entry.RequiredAnalyses) < 5 || !slices.IsSorted(entry.RequiredAnalyses) {
			t.Fatalf("%s required analyses are incomplete or unsorted: %v", entry.ID, entry.RequiredAnalyses)
		}

		path := filepath.Join(root, entry.RequirementFile)
		if filepath.Base(path) != entry.RequirementFile || seenFiles[entry.RequirementFile] {
			t.Fatalf("%s has unsafe or duplicate requirement file %q", entry.ID, entry.RequirementFile)
		}
		seenFiles[entry.RequirementFile] = true
		data := mustRead(t, path)
		if got := frozenHash(data); got != entry.RequirementSHA256 {
			t.Fatalf("%s sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectFrozenImplementationDetail(t, entry.ID, data)

		var requirement frozenRequirement
		decodeFrozenStrict(t, data, &requirement)
		validateFrozenRequirement(t, frozenManifestCase{
			ID:                entry.ID,
			BehaviorFamily:    entry.BehaviorFamily,
			RequirementFile:   entry.RequirementFile,
			RequirementSHA256: entry.RequirementSHA256,
			RequiredAnalyses:  entry.RequiredAnalyses,
			SafetyCritical:    entry.SafetyCritical,
		}, requirement)

		decoded, issues := DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s open-topology requirement is invalid: %#v", entry.ID, issues)
		}
		if decoded.Project.Name != entry.ID {
			t.Fatalf("%s decoded project name = %q", entry.ID, decoded.Project.Name)
		}
	}
	if len(seenClaims) != len(wantClaims) {
		t.Fatalf("acceptance claims = %v, want %v", seenClaims, wantClaims)
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

func TestArchitectureCorpusUntouchedBaseline(t *testing.T) {
	var manifest architectureCorpusManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(architectureCorpusRoot(), "manifest.json")), &manifest)

	var baseline architectureCorpusBaseline
	decodeFrozenStrict(t, mustRead(t, filepath.Join("..", "..", "specs", "simulation-grounded-architecture-synthesis", "BASELINE_REPORT.json")), &baseline)
	if baseline.Schema != architectureBaselineSchema || baseline.Version != 1 ||
		baseline.BaseCommit != architectureCorpusBaseCommit ||
		baseline.ManifestHash != architectureCorpusManifestHash ||
		strings.TrimSpace(baseline.MeasuredAt) == "" {
		t.Fatalf("baseline identity = %#v", baseline)
	}
	if baseline.Summary.Total != architectureCorpusCaseCount || baseline.Summary.CompletePasses != 0 ||
		baseline.Summary.FailClosed != architectureCorpusCaseCount || len(baseline.Cases) != len(manifest.Cases) {
		t.Fatalf("baseline summary = %#v", baseline.Summary)
	}
	wantStops := map[string]string{
		"continuous_conduction_audio_stage": "architecture_generation",
		"efficient_audio_power_stage":       "architecture_generation",
		"mains_notch_filter":                "equation_sizing",
	}
	for index, result := range baseline.Cases {
		manifestCase := manifest.Cases[index]
		if result.ID != manifestCase.ID || result.RequirementSHA256 != manifestCase.RequirementSHA256 ||
			result.Status != "fail_closed" || result.StoppedAt != wantStops[result.ID] ||
			strings.TrimSpace(result.Code) == "" || !result.ProviderBypass ||
			len(result.RequirementHash) != 64 || len(result.EvidenceHash) != 64 {
			t.Errorf("baseline case %d = %#v", index, result)
		}
	}
}

func architectureCorpusRoot() string {
	return filepath.Join("testdata", "architecture_corpus")
}
