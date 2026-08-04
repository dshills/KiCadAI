package opentopologysynthesis

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	protectedVoltageOutputCorpusSchema       = "kicadai.protected-voltage-output-corpus.v1"
	protectedVoltageOutputCorpusBaseCommit   = "75d0f39b06e7bb3f91e61ae3e90f58ddf1568e7e"
	protectedVoltageOutputCorpusManifestHash = "963d5052574b2ccb6bdb18b652c9a2fbeb6e5dbf0bed1237dc7ce78fbcc89d4c"
	protectedVoltageOutputCorpusCaseCount    = 3
)

var protectedVoltageOutputCorpusRoles = []string{
	"adjustable_positive",
	"bidirectional_midrail",
	"protected_high_power",
}

type protectedVoltageOutputCorpusManifest struct {
	Schema            string                             `json:"schema"`
	Version           int                                `json:"version"`
	BaseCommit        string                             `json:"base_commit"`
	FrozenAt          string                             `json:"frozen_at"`
	RequirementSchema string                             `json:"requirement_schema"`
	AuthoringPolicy   string                             `json:"authoring_policy"`
	Cases             []protectedVoltageOutputCorpusCase `json:"cases"`
}

type protectedVoltageOutputCorpusCase struct {
	ID                string   `json:"id"`
	VoltageRole       string   `json:"voltage_role"`
	Independence      string   `json:"independence"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	RequiredBehaviors []string `json:"required_behaviors"`
}

func TestProtectedVoltageOutputCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := protectedVoltageOutputCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != protectedVoltageOutputCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, protectedVoltageOutputCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != protectedVoltageOutputCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest protectedVoltageOutputCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != protectedVoltageOutputCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != protectedVoltageOutputCorpusBaseCommit ||
		manifest.RequirementSchema != RequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		len(manifest.Cases) != protectedVoltageOutputCorpusCaseCount {
		t.Fatalf("manifest identity = %#v", manifest)
	}

	seenIDs := map[string]bool{}
	seenHashes := map[string]bool{}
	roles := map[string]int{}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID || seenIDs[entry.ID] {
			t.Fatalf("case IDs are not unique and sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		seenIDs[entry.ID] = true
		if !slices.Contains(protectedVoltageOutputCorpusRoles, entry.VoltageRole) {
			t.Fatalf("%s voltage role = %q", entry.ID, entry.VoltageRole)
		}
		roles[entry.VoltageRole]++
		if strings.TrimSpace(entry.Independence) == "" || seenHashes[entry.RequirementSHA256] {
			t.Fatalf("%s independence/hash = %q/%q", entry.ID, entry.Independence, entry.RequirementSHA256)
		}
		seenHashes[entry.RequirementSHA256] = true
		if strings.HasPrefix(entry.RequirementFile, "../") {
			t.Fatalf("%s must be independently frozen in this corpus: %q", entry.ID, entry.RequirementFile)
		}
		if len(entry.RequiredAnalyses) < 7 || !slices.IsSorted(entry.RequiredAnalyses) ||
			len(entry.RequiredBehaviors) < 10 || !slices.IsSorted(entry.RequiredBehaviors) {
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
		assertProtectedVoltageBehaviorIndependence(t, entry, requirement)
	}
	for _, role := range protectedVoltageOutputCorpusRoles {
		if roles[role] != 1 {
			t.Fatalf("corpus role coverage = %v", roles)
		}
	}
}

func assertProtectedVoltageBehaviorIndependence(t *testing.T, entry protectedVoltageOutputCorpusCase, requirement Requirement) {
	t.Helper()
	outputVoltageAssertions := 0
	shortInitialSigns := map[int]bool{}
	loadSpansZero := false
	outputPort := false
	adjustableControl := false
	defaultOffControl := false
	for _, port := range requirement.Requirements.Ports {
		outputPort = outputPort || (port.Kind == "power" && port.Direction == "source")
		adjustableControl = adjustableControl || port.Kind == "analog_voltage"
		defaultOffControl = defaultOffControl || (port.Kind == "digital" && port.Electrical.DefaultState == "low")
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			loadSpansZero = loadSpansZero || (condition.Axis == "load_current" && condition.Min < 0 && condition.Max > 0)
		}
		for _, event := range operatingCase.Events {
			if event.Kind != "short_circuit" {
				continue
			}
			if event.Applied < 0 {
				shortInitialSigns[-1] = true
			} else if event.Applied > 0 {
				shortInitialSigns[1] = true
			}
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "output_voltage" {
			outputVoltageAssertions++
		}
	}
	switch entry.VoltageRole {
	case "adjustable_positive":
		if !adjustableControl || outputVoltageAssertions < 2 || !shortInitialSigns[1] {
			t.Fatalf("%s lacks independent adjustable/short behavior", entry.ID)
		}
	case "protected_high_power":
		if !defaultOffControl || !shortInitialSigns[1] {
			t.Fatalf("%s lacks independent default-off/short behavior", entry.ID)
		}
	case "bidirectional_midrail":
		if !outputPort || !loadSpansZero || !shortInitialSigns[-1] || !shortInitialSigns[1] {
			t.Fatalf("%s lacks independent bidirectional behavior", entry.ID)
		}
	}
}

func protectedVoltageOutputCorpusRoot() string {
	return filepath.Join("testdata", "protected_voltage_output_corpus")
}
