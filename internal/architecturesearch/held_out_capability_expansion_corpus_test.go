package architecturesearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	heldOutCapabilityExpansionCorpusSchema = "kicadai.held-out-capability-expansion-corpus.v1"
	heldOutCapabilityExpansionBaseCommit   = "a026fc78b7c79210b1f632590fcb4828e63cbbd0"
	heldOutCapabilityExpansionManifestHash = "e0d55f484c749eba7d3279da13c21380f5b52f9953f333adf1916409620eb442"
)

var heldOutCapabilityExpansionStages = []string{
	"integrity",
	"schema",
	"intent",
	"architecture",
	"component_evidence",
	"simulation",
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

type heldOutCapabilityExpansionManifest struct {
	Schema            string                           `json:"schema"`
	Version           int                              `json:"version"`
	BaseCommit        string                           `json:"base_commit"`
	FrozenAt          string                           `json:"frozen_at"`
	RequirementSchema string                           `json:"requirement_schema"`
	AuthoringPolicy   string                           `json:"authoring_policy"`
	Stages            []string                         `json:"stages"`
	Cases             []heldOutCapabilityExpansionCase `json:"cases"`
}

type heldOutCapabilityExpansionCase struct {
	ID                string `json:"id"`
	Domain            string `json:"domain"`
	Family            string `json:"family"`
	Role              string `json:"role"`
	Prompt            string `json:"prompt"`
	PromptSHA256      string `json:"prompt_sha256"`
	RequirementFile   string `json:"requirement_file"`
	RequirementSHA256 string `json:"requirement_sha256"`
	SafetyCritical    bool   `json:"safety_critical"`
}

func TestHeldOutCapabilityExpansionCorpusIsFrozenBeforeProductionClosure(t *testing.T) {
	root := heldOutCapabilityExpansionCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := hashHeldOutCapabilityBytes(manifestBytes); got != heldOutCapabilityExpansionManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, heldOutCapabilityExpansionManifestHash)
	}
	checksum, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(checksum)) != heldOutCapabilityExpansionManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar does not match frozen manifest")
	}
	var manifest heldOutCapabilityExpansionManifest
	decodeHeldOutCapabilityExpansionStrict(t, manifestBytes, &manifest)
	if manifest.Schema != heldOutCapabilityExpansionCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != heldOutCapabilityExpansionBaseCommit ||
		manifest.RequirementSchema != SchemaIDV3 || strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, heldOutCapabilityExpansionStages) {
		t.Fatalf("stages = %v, want %v", manifest.Stages, heldOutCapabilityExpansionStages)
	}
	if len(manifest.Cases) != 12 {
		t.Fatalf("case count = %d, want 12", len(manifest.Cases))
	}

	testdataRoot, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	domains := map[string]int{}
	roles := map[string]int{}
	heldOutFamilies := map[string]int{}
	seenIDs := map[string]bool{}
	seenRequirementFiles := map[string]bool{}
	localFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID || seenIDs[entry.ID] {
			t.Fatalf("case IDs are not unique and strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		seenIDs[entry.ID] = true
		if entry.Domain == "" || entry.Family == "" || (entry.Role != "control" && entry.Role != "held_out") {
			t.Fatalf("%s has invalid reporting identity: %#v", entry.ID, entry)
		}
		domains[entry.Domain]++
		roles[entry.Role]++
		if entry.Role == "held_out" {
			heldOutFamilies[entry.Family]++
		}

		assertHeldOutBehaviorOnlyPrompt(t, entry)
		if got := hashHeldOutCapabilityBytes([]byte(entry.Prompt)); got != entry.PromptSHA256 {
			t.Fatalf("%s prompt sha256 = %s, want %s", entry.ID, got, entry.PromptSHA256)
		}

		requirementPath, err := filepath.Abs(filepath.Join(root, entry.RequirementFile))
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(testdataRoot, requirementPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			t.Fatalf("%s requirement path escapes architecturesearch testdata: %q", entry.ID, entry.RequirementFile)
		}
		if seenRequirementFiles[requirementPath] {
			t.Fatalf("%s reuses requirement file %q", entry.ID, entry.RequirementFile)
		}
		seenRequirementFiles[requirementPath] = true
		if filepath.Dir(requirementPath) == mustHeldOutCapabilityAbs(t, root) {
			localFiles[filepath.Base(requirementPath)] = true
		}
		requirementBytes, err := os.ReadFile(requirementPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := hashHeldOutCapabilityBytes(requirementBytes); got != entry.RequirementSHA256 {
			t.Fatalf("%s requirement sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectHeldOutCapabilityImplementationDetail(t, entry.ID, requirementBytes)

		requirement, issues := DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s strict decode issues: %#v", entry.ID, issues)
		}
		if requirement.Schema != SchemaIDV3 || requirement.Version != VersionV3 {
			t.Fatalf("%s requirement identity = %s/%d", entry.ID, requirement.Schema, requirement.Version)
		}
		assertHeldOutCapabilityAcceptance(t, entry.ID, requirement.Acceptance)
		if len(requirement.Requirements.OperatingCases) == 0 || len(requirement.Requirements.BehavioralRequirements) < 4 {
			t.Fatalf("%s lacks operating cases or measurable behavior", entry.ID)
		}
		if entry.Role == "held_out" && !requirementHasHeldOutCapability(requirement, entry.Family) {
			t.Fatalf("%s does not request held-out family %q", entry.ID, entry.Family)
		}
		assertHeldOutCapabilityCanonicalReplay(t, entry.ID, requirement)
	}

	for _, domain := range []string{"analog", "digital", "mcu", "mixed_signal", "power", "sensor"} {
		if domains[domain] != 2 {
			t.Errorf("domain %s count = %d, want 2", domain, domains[domain])
		}
	}
	if roles["control"] != 6 || roles["held_out"] != 6 {
		t.Errorf("role counts = %#v, want six control and six held_out", roles)
	}
	if heldOutFamilies["constant_current_regulation"] != 3 ||
		heldOutFamilies["precision_rectification"] != 2 ||
		heldOutFamilies["clock_generation"] != 1 {
		t.Errorf("held-out family pressures = %#v", heldOutFamilies)
	}

	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if !localFiles[filepath.Base(path)] {
			t.Errorf("unmanifested local corpus file %s", filepath.Base(path))
		}
	}
}

func heldOutCapabilityExpansionCorpusRoot() string {
	return filepath.Join("testdata", "held_out_capability_expansion_corpus")
}

func decodeHeldOutCapabilityExpansionStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("unexpected trailing manifest content: %v", err)
	}
}

var heldOutPromptImplementationPattern = regexp.MustCompile(`(?i)\b(topology|schematic|footprint|symbol|pin|pad|net|coordinate|layer|track|route|via|provider|solver|model|repair|resistor|capacitor|diode|transistor|mosfet|bjt|op[- ]?amp)\b`)

func assertHeldOutBehaviorOnlyPrompt(t *testing.T, entry heldOutCapabilityExpansionCase) {
	t.Helper()
	if strings.TrimSpace(entry.Prompt) == "" || len(entry.Prompt) > 1200 {
		t.Fatalf("%s prompt length is invalid", entry.ID)
	}
	if match := heldOutPromptImplementationPattern.FindString(entry.Prompt); match != "" {
		t.Fatalf("%s prompt contains implementation detail %q", entry.ID, match)
	}
}

func rejectHeldOutCapabilityImplementationDetail(t *testing.T, id string, data []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	prohibited := []string{
		"block_family", "component_id", "coordinate", "expected_part", "expected_value",
		"expansion_id", "footprint", "layer", "net", "pad", "pin", "provider_id",
		"repair_action", "route", "solver", "symbol", "topology", "track", "via",
	}
	var walk func(any, string)
	walk = func(current any, path string) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				for _, blocked := range prohibited {
					if lower == blocked || strings.HasPrefix(lower, blocked+"_") || strings.HasSuffix(lower, "_"+blocked) {
						t.Errorf("%s contains implementation field %s.%s", id, path, key)
					}
				}
				walk(child, path+"."+key)
			}
		case []any:
			for index, child := range typed {
				walk(child, path+"["+jsonIndex(index)+"]")
			}
		}
	}
	walk(value, "$")
}

func assertHeldOutCapabilityAcceptance(t *testing.T, id string, acceptance Acceptance) {
	t.Helper()
	if !acceptance.RequireERC || !acceptance.RequireStrictDRC ||
		!acceptance.RequireCompleteRouting || !acceptance.RequireConnectivity ||
		!acceptance.RequireWriterCorrectness || !acceptance.RequireRoundTripZeroDiff ||
		!acceptance.RequireDeterministicReplay || !acceptance.RequireContractComposition ||
		!acceptance.RequireGlobalReasoning || !acceptance.RequireCoverageAccounting ||
		!acceptance.RequireAlternatives || !acceptance.RequireFailClosed ||
		!acceptance.RequireSimulation || !acceptance.RequireAllCorners ||
		!acceptance.RequireModelProvenance || !acceptance.RequireClosedLoopEvidence {
		t.Fatalf("%s does not require the complete acceptance profile: %#v", id, acceptance)
	}
}

func requirementHasHeldOutCapability(requirement Requirement, capability string) bool {
	for _, participant := range requirement.Requirements.Participants {
		if participant.Capability == capability {
			return true
		}
	}
	for _, objective := range requirement.Requirements.Objectives {
		if objective.Capability == capability {
			return true
		}
	}
	return false
}

func assertHeldOutCapabilityCanonicalReplay(t *testing.T, id string, requirement Requirement) {
	t.Helper()
	first, err := CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(requirement)
	if err != nil {
		t.Fatal(err)
	}
	var reordered Requirement
	if err := json.Unmarshal(encoded, &reordered); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(reordered.Requirements.Domains)
	slices.Reverse(reordered.Requirements.Ports)
	slices.Reverse(reordered.Requirements.Signals)
	slices.Reverse(reordered.Requirements.Participants)
	slices.Reverse(reordered.Requirements.Objectives)
	slices.Reverse(reordered.Requirements.SystemConstraints)
	slices.Reverse(reordered.Requirements.OperatingCases)
	slices.Reverse(reordered.Requirements.BehavioralRequirements)
	for index := range reordered.Requirements.Participants {
		slices.Reverse(reordered.Requirements.Participants[index].RequiredPorts)
		slices.Reverse(reordered.Requirements.Participants[index].Constraints)
	}
	for index := range reordered.Requirements.Objectives {
		slices.Reverse(reordered.Requirements.Objectives[index].Bindings)
		slices.Reverse(reordered.Requirements.Objectives[index].Constraints)
	}
	for index := range reordered.Requirements.OperatingCases {
		slices.Reverse(reordered.Requirements.OperatingCases[index].Conditions)
	}
	for index := range reordered.Requirements.BehavioralRequirements {
		slices.Reverse(reordered.Requirements.BehavioralRequirements[index].OperatingCases)
	}
	second, err := CanonicalHash(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("%s canonical hash changed under semantic reordering: %s != %s", id, first, second)
	}
}

func hashHeldOutCapabilityBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func mustHeldOutCapabilityAbs(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func jsonIndex(index int) string {
	const digits = "0123456789"
	if index == 0 {
		return "0"
	}
	var reversed [20]byte
	position := len(reversed)
	for index > 0 {
		position--
		reversed[position] = digits[index%10]
		index /= 10
	}
	return string(reversed[position:])
}
