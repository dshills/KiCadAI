package architecturesearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const protocolAwareBusManifestSHA = "85739a96217506cb929f070b888a221739923877c8f39f330269caee7f4222a1"

type protocolAwareBusManifest struct {
	Schema            string                       `json:"schema"`
	Version           int                          `json:"version"`
	BaseCommit        string                       `json:"base_commit"`
	FrozenAt          string                       `json:"frozen_at"`
	RequirementSchema string                       `json:"requirement_schema"`
	AuthoringPolicy   string                       `json:"authoring_policy"`
	Cases             []protocolAwareBusCorpusCase `json:"cases"`
}

type protocolAwareBusCorpusCase struct {
	ID                string `json:"id"`
	Family            string `json:"family"`
	Protocol          string `json:"protocol"`
	Prompt            string `json:"prompt"`
	PromptSHA256      string `json:"prompt_sha256"`
	RequirementFile   string `json:"requirement_file"`
	RequirementSHA256 string `json:"requirement_sha256"`
	SafetyCritical    bool   `json:"safety_critical"`
}

func TestProtocolAwareBusCorpusIsFrozenAndBehaviorOnly(t *testing.T) {
	root := protocolAwareBusCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := protocolAwareBusHash(manifestBytes); got != protocolAwareBusManifestSHA {
		t.Fatalf("manifest sha256 = %s, want %s", got, protocolAwareBusManifestSHA)
	}
	sidecar, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(sidecar)), protocolAwareBusManifestSHA+"  manifest.json"; got != want {
		t.Fatalf("manifest sidecar = %q, want %q", got, want)
	}
	var manifest protocolAwareBusManifest
	decodeProtocolAwareBusStrict(t, manifestBytes, &manifest)
	if manifest.Schema != "kicadai.protocol-aware-bus-corpus.v1" ||
		manifest.Version != 1 ||
		manifest.BaseCommit != "75feb87401d7aebebf1ecb9ebd3ba76ac5efdd24" ||
		manifest.RequirementSchema != SchemaIDV3 ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		len(manifest.Cases) != 4 {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	protocols := map[string]bool{"i2c": false, "smbus": false, "spi": false, "uart": false}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID || !entry.SafetyCritical || entry.Family == "" {
			t.Fatalf("invalid sorted corpus entry %#v after %q", entry, previousID)
		}
		previousID = entry.ID
		if _, ok := protocols[entry.Protocol]; !ok {
			t.Fatalf("%s protocol = %q", entry.ID, entry.Protocol)
		}
		protocols[entry.Protocol] = true
		assertHeldOutBehaviorOnlyPrompt(t, heldOutCapabilityExpansionCase{ID: entry.ID, Prompt: entry.Prompt})
		if got := protocolAwareBusHash([]byte(entry.Prompt)); got != entry.PromptSHA256 {
			t.Fatalf("%s prompt sha256 = %s, want %s", entry.ID, got, entry.PromptSHA256)
		}
		requirementBytes, readErr := os.ReadFile(filepath.Join(root, entry.RequirementFile))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if got := protocolAwareBusHash(requirementBytes); got != entry.RequirementSHA256 {
			t.Fatalf("%s requirement sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectHeldOutCapabilityImplementationDetail(t, entry.ID, requirementBytes)
		requirement, issues := DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s strict decode issues = %#v", entry.ID, issues)
		}
		assertHeldOutCapabilityAcceptance(t, entry.ID, requirement.Acceptance)
		assertHeldOutCapabilityCanonicalReplay(t, entry.ID, requirement)
	}
	for protocol, seen := range protocols {
		if !seen {
			t.Fatalf("manifest lacks protocol %q", protocol)
		}
	}
}

func TestProtocolAwareBusCorpusSearchesDeterministically(t *testing.T) {
	registry, registryIssues := NewCatalogRegistry(loadArchitectureCatalog(t))
	if len(registryIssues) != 0 {
		t.Fatal(registryIssues)
	}
	for _, file := range []string{
		"i2c_partial_power.json",
		"segmented_smbus.json",
		"spi_mixed_direction.json",
		"uart_inactive_partial_power.json",
	} {
		t.Run(file, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(protocolAwareBusCorpusRoot(), file))
			if err != nil {
				t.Fatal(err)
			}
			requirement, issues := DecodeStrict(bytes.NewReader(contents))
			if len(issues) != 0 {
				t.Fatalf("strict decode issues = %#v", issues)
			}
			if len(requirement.Requirements.Objectives) == 0 ||
				!slices.ContainsFunc(requirement.Requirements.Objectives, func(objective Objective) bool {
					return objective.Capability == "bus_buffering_level_translation"
				}) {
				t.Fatalf("bus objective = %#v", requirement.Requirements.Objectives)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			first := Search(ctx, requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			second := Search(ctx, requirement, registry, SearchOptions{CatalogHash: registry.Hash()})
			if first.Status != SearchSelected || first.Selected == nil {
				t.Fatalf("first search = %#v", first)
			}
			if second.Status != SearchSelected || second.Selected == nil {
				t.Fatalf("second search = %#v", second)
			}
			if first.Selected.Fingerprint != second.Selected.Fingerprint {
				t.Fatalf("search is nondeterministic\nfirst: %#v\nsecond: %#v", first, second)
			}
			realizationCount := 0
			wholeBusCalculation := file != "segmented_smbus.json"
			for _, fragment := range first.Selected.Selections {
				if fragment.Capability == "bus_buffering_level_translation" {
					realizationCount++
				}
				wholeBusCalculation = wholeBusCalculation || slices.ContainsFunc(fragment.Calculations, func(calculation CalculationEvidence) bool {
					return calculation.ID == "segmented_open_drain_bus"
				})
			}
			if realizationCount != len(requirement.Requirements.Objectives) {
				t.Fatalf("bus realization count = %d, want %d", realizationCount, len(requirement.Requirements.Objectives))
			}
			if !wholeBusCalculation {
				t.Fatalf("candidate lacks whole-bus calculation: %#v", first.Selected.Selections)
			}
		})
	}
}

func protocolAwareBusCorpusRoot() string {
	return filepath.Join("testdata", "protocol_aware_bus_corpus")
}

func protocolAwareBusHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeProtocolAwareBusStrict(t *testing.T, data []byte, target any) {
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
