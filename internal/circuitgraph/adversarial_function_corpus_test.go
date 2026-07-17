package circuitgraph

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

const (
	frozenAdversarialCorpusManifestSHA256 = "e40a02f2ee89aa6ef3c5a98628c2712cf633ac67b2fbaba487e84d52c607ce32"
	frozenAdversarialCorpusBaseCommit     = "9fead2eccc576f1833a68f07328417d6ee55c87d"
)

type adversarialCorpusFixture struct {
	ID        string   `json:"id"`
	File      string   `json:"file"`
	Domains   []string `json:"domains"`
	Pressures []string `json:"pressures"`
	SHA256    string   `json:"sha256"`
}

type adversarialCorpusManifest struct {
	Schema        string                     `json:"schema"`
	Version       int                        `json:"version"`
	FrozenAt      string                     `json:"frozen_at"`
	BaseCommit    string                     `json:"base_commit"`
	PolicyVersion string                     `json:"policy_version"`
	Fixtures      []adversarialCorpusFixture `json:"fixtures"`
}

func TestAdversarialFunctionCorpusIsFrozenAndIdentityNeutral(t *testing.T) {
	root := adversarialFunctionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(manifestBytes); got != frozenAdversarialCorpusManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s; adversarial membership is frozen before implementation", got, frozenAdversarialCorpusManifestSHA256)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantChecksum := frozenAdversarialCorpusManifestSHA256 + "  manifest.json\n"
	if string(checksumBytes) != wantChecksum {
		t.Fatalf("manifest.sha256 = %q, want %q", checksumBytes, wantChecksum)
	}

	var manifest adversarialCorpusManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.adversarial-function-corpus.v1" || manifest.Version != 1 || manifest.PolicyVersion != SynthesisPolicyVersion || manifest.BaseCommit != frozenAdversarialCorpusBaseCommit {
		t.Fatalf("invalid adversarial corpus manifest header: %#v", manifest)
	}
	if len(manifest.Fixtures) != 18 {
		t.Fatalf("fixture count = %d, want 18", len(manifest.Fixtures))
	}

	coveredDomains := map[string]bool{}
	coveredPressures := map[string]bool{}
	seenIDs := map[string]bool{}
	seenFiles := map[string]bool{}
	for index, fixture := range manifest.Fixtures {
		if fixture.ID == "" || fixture.File != fixture.ID+".json" || seenIDs[fixture.ID] || seenFiles[fixture.File] {
			t.Fatalf("fixtures[%d] has invalid or duplicate identity: %#v", index, fixture)
		}
		if index > 0 && manifest.Fixtures[index-1].ID >= fixture.ID {
			t.Fatalf("manifest fixtures are not strictly sorted at %q", fixture.ID)
		}
		if !slices.IsSorted(fixture.Domains) || !slices.IsSorted(fixture.Pressures) || len(fixture.Pressures) == 0 {
			t.Fatalf("fixtures[%d] domains or pressures are incomplete/unsorted: %#v", index, fixture)
		}
		seenIDs[fixture.ID] = true
		seenFiles[fixture.File] = true
		for _, domain := range fixture.Domains {
			coveredDomains[domain] = true
		}
		for _, pressure := range fixture.Pressures {
			coveredPressures[pressure] = true
		}

		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(contents); got != fixture.SHA256 {
			t.Fatalf("%s hash = %s, want %s", fixture.File, got, fixture.SHA256)
		}
		var raw map[string]any
		if err := json.Unmarshal(contents, &raw); err != nil {
			t.Fatalf("decode %s: %v", fixture.File, err)
		}
		if raw["schema"] != SchemaID || int(raw["version"].(float64)) != Version || raw["synthesis"] == nil {
			t.Fatalf("%s is not a function-level %s document", fixture.File, SchemaID)
		}
		for _, explicit := range []string{"components", "nets", "no_connects", "power_flags", "buses", "schematic", "pcb", "simulation"} {
			if _, exists := raw[explicit]; exists {
				t.Fatalf("%s contains explicit graph field %q", fixture.File, explicit)
			}
		}
		assertNoFunctionCorpusImplementationDetail(t, fixture.File, raw, "")

		document, issues := DecodeStrict(strings.NewReader(string(contents)))
		if reports.HasBlockingIssue(issues) || document.Synthesis == nil {
			t.Fatalf("%s is not valid frozen function intent: issues=%#v", fixture.File, issues)
		}
		if document.Project.Name != fixture.ID {
			t.Fatalf("%s project name = %q, want %q", fixture.File, document.Project.Name, fixture.ID)
		}
		for functionIndex, function := range document.Synthesis.Functions {
			if function.ComponentID != "" || function.Query == nil {
				t.Fatalf("%s synthesis.functions[%d] must use a semantic catalog query only", fixture.File, functionIndex)
			}
		}
	}

	for _, required := range []string{"analog", "interface", "mcu", "power", "protection", "sensor", "transient", "transistor"} {
		if !coveredDomains[required] {
			t.Fatalf("frozen adversarial corpus does not cover domain %s", required)
		}
	}
	for _, required := range []string{
		"adjustable_regulator", "bipolar_supply", "bjt_transient", "dense_routing", "diode_transient",
		"gpio", "level_translation", "multi_unit_analog", "multiple_voltage_domains", "negative_supply",
		"routing_repair", "spi", "uart", "wide_connector",
	} {
		if !coveredPressures[required] {
			t.Fatalf("frozen adversarial corpus does not cover pressure %s", required)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "manifest.json" {
			continue
		}
		if !seenFiles[entry.Name()] {
			t.Fatalf("unmanifested adversarial fixture %s", entry.Name())
		}
	}

	assertAdversarialFixtureIDsAbsentFromProduction(t, manifest.Fixtures)
}

func assertAdversarialFixtureIDsAbsentFromProduction(t *testing.T, fixtures []adversarialCorpusFixture) {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate adversarial corpus test source")
	}
	internalRoot := filepath.Dir(sourcePath)
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, fixture := range fixtures {
			if strings.Contains(string(contents), fixture.ID) {
				t.Fatalf("production source %s contains frozen fixture identity %q", path, fixture.ID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func adversarialFunctionCorpusRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate adversarial function corpus test source")
	}
	return filepath.Join(filepath.Dir(sourcePath), "testdata", "adversarial_function_corpus")
}
