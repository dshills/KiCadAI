package capabilityfeedback

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	closedLoopCorpusSchema      = "kicadai.closed-loop-open-set-corpus.v1"
	closedLoopCorpusVersion     = 1
	closedLoopCorpusSize        = 24
	closedLoopCorpusRoot        = "testdata/closed_loop_open_set_corpus"
	frozenGoMinimum             = "1.23"
	frozenKiCadVersion          = "10.0.3"
	frozenOperatingSystem       = "darwin"
	frozenProcessorArchitecture = "arm64"
)

type closedLoopManifest struct {
	Schema               string                              `json:"schema"`
	Version              int                                 `json:"version"`
	RequirementSchema    string                              `json:"requirement_schema"`
	EvaluatorPolicy      string                              `json:"evaluator_policy"`
	ImpactRegistry       capabilityevaluation.ImpactRegistry `json:"impact_registry"`
	ImpactRegistryHash   string                              `json:"impact_registry_hash"`
	SynthesisPolicy      ots.Policy                          `json:"synthesis_policy"`
	SynthesisPolicyHash  string                              `json:"synthesis_policy_hash"`
	Environment          closedLoopEnvironment               `json:"environment"`
	IndependentAuthoring string                              `json:"independent_authoring"`
	Entries              []closedLoopManifestEntry           `json:"entries"`
}

type closedLoopEnvironment struct {
	GoMinimum string `json:"go_minimum"`
	KiCad     string `json:"kicad"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type closedLoopManifestEntry struct {
	ID              string                            `json:"id"`
	Role            CorpusRole                        `json:"role"`
	Domain          capabilityevaluation.Domain       `json:"domain"`
	SafetyImpact    capabilityevaluation.SafetyImpact `json:"safety_impact"`
	RequirementFile string                            `json:"requirement_file"`
	RequirementHash string                            `json:"requirement_sha256"`
}

func TestClosedLoopCorpusFreeze(t *testing.T) {
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json"))
	checksum := strings.TrimSpace(string(mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.sha256"))))
	wantChecksum := corpusHash(manifestBytes) + "  manifest.json"
	if checksum != wantChecksum {
		t.Fatalf("manifest checksum = %q, want %q", checksum, wantChecksum)
	}

	var manifest closedLoopManifest
	decodeCorpusStrict(t, manifestBytes, &manifest)
	expected := buildClosedLoopManifest(t)
	expectedBytes := corpusJSON(t, expected)
	if !bytes.Equal(manifestBytes, expectedBytes) {
		t.Fatal("closed-loop corpus manifest bytes drifted from the frozen authoring contract")
	}
	seeds := closedLoopCorpusSeeds()
	if len(manifest.Entries) != closedLoopCorpusSize || len(seeds) != len(manifest.Entries) {
		t.Fatalf("manifest/seed entries = %d/%d, want %d/%d", len(manifest.Entries), len(seeds), closedLoopCorpusSize, closedLoopCorpusSize)
	}

	seenRequirements := map[string]string{}
	for index, entry := range manifest.Entries {
		seed := seeds[index]
		data := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		if got := corpusHash(data); got != entry.RequirementHash {
			t.Fatalf("%s requirement hash = %s, want %s", entry.ID, got, entry.RequirementHash)
		}
		generated := corpusJSON(t, closedLoopRequirement(seed))
		if !bytes.Equal(data, generated) {
			t.Fatalf("%s requirement bytes drifted from the frozen independent seed", entry.ID)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s strict requirement issues: %#v", entry.ID, issues)
		}
		assertBehaviorOnly(t, entry.ID, data)
		semantic, err := ots.CanonicalHash(requirement)
		if err != nil {
			t.Fatal(err)
		}
		if prior, duplicate := seenRequirements[semantic]; duplicate {
			t.Fatalf("%s duplicates normalized requirement %s", entry.ID, prior)
		}
		seenRequirements[semantic] = entry.ID
	}
	assertClosedLoopDiversity(t, manifest, seeds)
}

func TestUpdateClosedLoopCorpus(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_CORPUS") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_CORPUS=1 to regenerate frozen corpus bytes")
	}
	manifest := buildClosedLoopManifest(t)
	if err := os.MkdirAll(filepath.Join(closedLoopCorpusRoot, "discovery"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(closedLoopCorpusRoot, "held_out"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, seed := range closedLoopCorpusSeeds() {
		path := filepath.Join(closedLoopCorpusRoot, filepath.FromSlash(seed.File))
		if err := os.WriteFile(path, corpusJSON(t, closedLoopRequirement(seed)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestBytes := corpusJSON(t, manifest)
	if err := os.WriteFile(filepath.Join(closedLoopCorpusRoot, "manifest.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := []byte(corpusHash(manifestBytes) + "  manifest.json\n")
	if err := os.WriteFile(filepath.Join(closedLoopCorpusRoot, "manifest.sha256"), checksum, 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildClosedLoopManifest(t *testing.T) closedLoopManifest {
	t.Helper()
	registry := closedLoopImpactRegistry()
	_, _, registryHash, err := normalizeImpactRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	policy := ots.DefaultPolicy()
	policyHash, err := digest(policy)
	if err != nil {
		t.Fatal(err)
	}
	manifest := closedLoopManifest{
		Schema: closedLoopCorpusSchema, Version: closedLoopCorpusVersion,
		RequirementSchema: ots.RequirementSchema, EvaluatorPolicy: PolicyVersion,
		ImpactRegistry: registry, ImpactRegistryHash: registryHash,
		SynthesisPolicy: policy, SynthesisPolicyHash: policyHash,
		// These identify the version-1 reference environment. Regeneration is
		// deliberately host-independent; changing them creates corpus version 2.
		Environment:          closedLoopEnvironment{GoMinimum: frozenGoMinimum, KiCad: frozenKiCadVersion, OS: frozenOperatingSystem, Arch: frozenProcessorArchitecture},
		IndependentAuthoring: "Behavior-only electrical requirements were authored from public interface and analysis vocabulary before production outcomes were observed; no expected implementation or outcome was supplied.",
	}
	for _, seed := range closedLoopCorpusSeeds() {
		data := corpusJSON(t, closedLoopRequirement(seed))
		manifest.Entries = append(manifest.Entries, closedLoopManifestEntry{
			ID: seed.ID, Role: seed.Role, Domain: seed.Domain, SafetyImpact: seed.Safety,
			RequirementFile: seed.File, RequirementHash: corpusHash(data),
		})
	}
	return manifest
}

func closedLoopImpactRegistry() capabilityevaluation.ImpactRegistry {
	return capabilityevaluation.ImpactRegistry{Version: "closed-loop-open-set-impact-v1", Records: []capabilityevaluation.ImpactRecord{
		{Capability: "primitive_inventory", Consumers: []string{"catalog_value_domain", "physical_realization", "trusted_simulation_model"}},
		{Capability: "complete_topology", Consumers: []string{"passing_behavioral_evidence", "physical_realization"}},
		{Capability: "catalog_value_domain", Consumers: []string{"passing_behavioral_evidence", "physical_realization"}},
		{Capability: "trusted_simulation_model", Consumers: []string{"passing_behavioral_evidence"}},
		{Capability: "passing_behavioral_evidence", Consumers: []string{"physical_realization"}},
		{Capability: "physical_realization", Consumers: []string{"route_completion"}},
		{Capability: "route_completion", Consumers: []string{"installed_kicad_verification", "round_trip_fidelity"}},
	}}
}

func assertClosedLoopDiversity(t *testing.T, manifest closedLoopManifest, seeds []closedLoopSeed) {
	t.Helper()
	roleDomain := map[string]int{}
	features := map[CorpusRole]map[string]int{RoleDiscovery: {}, RoleHeldOut: {}}
	for index, entry := range manifest.Entries {
		roleDomain[string(entry.Role)+":"+string(entry.Domain)]++
		requirement := closedLoopRequirement(seeds[index])
		if len(requirement.Requirements.BehavioralRequirements) >= 4 {
			features[entry.Role]["multiple"]++
		}
		dynamic, faultOrStartup, safety, bipolar, switching := false, false, false, false, false
		for _, current := range requirement.Requirements.OperatingCases {
			for _, condition := range current.Conditions {
				bipolar = bipolar || condition.Min < 0
			}
			for _, currentEvent := range current.Events {
				faultOrStartup = faultOrStartup || currentEvent.Kind == "startup" || currentEvent.Kind == "short_circuit" || currentEvent.Kind == "rail_loss"
				switching = switching || currentEvent.Kind == "input_step" || currentEvent.Kind == "power_step"
			}
		}
		for _, current := range requirement.Requirements.BehavioralRequirements {
			dynamic = dynamic || current.Analysis == "transient" || current.Analysis == "ac_sweep" || current.Analysis == "startup"
			faultOrStartup = faultOrStartup || current.Analysis == "startup"
			safety = safety || current.Analysis == "thermal" || current.Analysis == "electrothermal"
			switching = switching || current.Metric == "duty_cycle" || current.Metric == "oscillation_frequency"
		}
		for name, present := range map[string]bool{"dynamic": dynamic, "fault_startup": faultOrStartup, "safety": safety, "bipolar": bipolar, "switching": switching} {
			if present {
				features[entry.Role][name]++
			}
		}
	}
	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		for _, domain := range []capabilityevaluation.Domain{capabilityevaluation.DomainAnalog, capabilityevaluation.DomainPower, capabilityevaluation.DomainDigital, capabilityevaluation.DomainMCU, capabilityevaluation.DomainSensor, capabilityevaluation.DomainMixedSignal} {
			if got := roleDomain[string(role)+":"+string(domain)]; got != 2 {
				t.Errorf("%s/%s count = %d, want 2", role, domain, got)
			}
		}
		for _, name := range []string{"multiple", "dynamic", "fault_startup", "safety"} {
			if got := features[role][name]; got < 4 {
				t.Errorf("%s %s cases = %d, want at least 4", role, name, got)
			}
		}
		for _, name := range []string{"bipolar", "switching"} {
			if got := features[role][name]; got < 2 {
				t.Errorf("%s %s cases = %d, want at least 2", role, name, got)
			}
		}
	}
}

// These word-bounded terms are intentionally strict: CORPUS_RULES.md forbids
// even otherwise ordinary pin, package, net, and provider wording because a
// requirement must remain purely behavioral.
var implementationLanguage = regexp.MustCompile(`(?i)\b(?:allowlist|block family|candidate|coordinate|footprint|manufacturer|model id|net|package|pad|part number|pin|provider|reference designator|repair|route|solver|symbol|topology|track|via)\b`)

func assertBehaviorOnly(t *testing.T, id string, data []byte) {
	t.Helper()
	var value any
	decodeCorpusStrict(t, data, &value)
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key != "schema" {
					walk(child)
				}
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case string:
			if match := implementationLanguage.FindString(typed); match != "" {
				t.Errorf("%s contains prohibited implementation language %q", id, match)
			}
		}
	}
	walk(value)
}

func corpusJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func corpusHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeCorpusStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("unexpected trailing JSON: %v", err)
	}
}

func mustCorpusRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestClosedLoopManifestReorderAndRoleLeakageFailClosed(t *testing.T) {
	expected := buildClosedLoopManifest(t)
	var whitespaceDecoded closedLoopManifest
	decodeCorpusStrict(t, append(corpusJSON(t, expected), []byte(" \n\t")...), &whitespaceDecoded)
	if whitespaceDecoded.Schema != closedLoopCorpusSchema {
		t.Fatal("strict decoder did not accept valid trailing JSON whitespace")
	}

	manifest := expected
	reordered := slices.Clone(manifest.Entries)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	if reordered[0].ID == "case_001" {
		t.Fatal("test did not reorder entries")
	}
	manifest.Entries = reordered
	if bytes.Equal(corpusJSON(t, manifest), corpusJSON(t, expected)) {
		t.Fatal("manifest reorder did not change frozen bytes")
	}

	requirement := corpusJSON(t, closedLoopRequirement(closedLoopCorpusSeeds()[0]))
	if bytes.Contains(requirement, []byte(`"role"`)) || bytes.Contains(requirement, []byte(`"safety_impact"`)) || bytes.Contains(requirement, []byte(`"domain":"analog"`)) {
		t.Fatal("reporting metadata leaked into requirement bytes")
	}
}

func TestClosedLoopImpactRegistryIsDeterministic(t *testing.T) {
	first := closedLoopImpactRegistry()
	second := closedLoopImpactRegistry()
	_, _, firstHash, err := normalizeImpactRegistry(first)
	if err != nil {
		t.Fatal(err)
	}
	_, _, secondHash, err := normalizeImpactRegistry(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || firstHash == "" {
		t.Fatalf("impact hashes differ: %s != %s", firstHash, secondHash)
	}
}

func TestClosedLoopCorpusPathsAreOpaque(t *testing.T) {
	for index, seed := range closedLoopCorpusSeeds() {
		wantID := fmt.Sprintf("case_%03d", index+1)
		wantName := fmt.Sprintf("request_%03d", index+1)
		if seed.ID != wantID || seed.Name != wantName || !strings.HasSuffix(seed.File, wantName+".json") {
			t.Errorf("non-opaque corpus identity at %d: %#v", index, seed)
		}
	}
}

func TestClosedLoopCorpusIdentityDoesNotLeakIntoProductionGo(t *testing.T) {
	identities := []string{regexp.QuoteMeta("closed_loop_open_set_corpus")}
	for _, seed := range closedLoopCorpusSeeds() {
		identities = append(identities, regexp.QuoteMeta(seed.ID))
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
				t.Errorf("production file %s contains frozen corpus identity %q", path, match)
				break
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
