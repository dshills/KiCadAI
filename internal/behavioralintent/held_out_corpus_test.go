package behavioralintent

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

	"kicadai/internal/architecturesearch"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const heldOutCorpusSchema = "kicadai.behavioral-intent-held-out-corpus.v1"

type heldOutManifest struct {
	Schema          string        `json:"schema"`
	Version         int           `json:"version"`
	FrozenAt        string        `json:"frozen_at"`
	AuthoringPolicy string        `json:"authoring_policy"`
	Cases           []heldOutCase `json:"cases"`
}

type heldOutCase struct {
	ID                string `json:"id"`
	Group             string `json:"group"`
	Domain            string `json:"domain"`
	Outcome           Status `json:"outcome"`
	Prompt            string `json:"prompt"`
	PromptSHA256      string `json:"prompt_sha256"`
	RequirementFile   string `json:"requirement_file,omitempty"`
	RequirementSHA256 string `json:"requirement_sha256,omitempty"`
	ClarificationID   string `json:"clarification_id,omitempty"`
	ClarificationPath string `json:"clarification_path,omitempty"`
	UncertaintyID     string `json:"uncertainty_id,omitempty"`
	UncertaintyKind   string `json:"uncertainty_kind,omitempty"`
	GapID             string `json:"gap_id,omitempty"`
	GapCapability     string `json:"gap_capability,omitempty"`
	GapPath           string `json:"gap_path,omitempty"`
}

func TestFrozenHeldOutBehavioralIntentCorpus(t *testing.T) {
	manifestData, err := os.ReadFile(filepath.Join("testdata", "held_out_corpus", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest heldOutManifest
	decodeHeldOutStrict(t, manifestData, &manifest)
	if manifest.Schema != heldOutCorpusSchema || manifest.Version != 1 || manifest.FrozenAt == "" || manifest.AuthoringPolicy == "" || len(manifest.Cases) < 20 {
		t.Fatalf("manifest identity or breadth = %#v", manifest)
	}

	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, registryIssues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(registryIssues) {
		t.Fatalf("catalog registry issues = %#v", registryIssues)
	}

	domains := map[string]bool{}
	outcomes := map[Status]bool{}
	groups := map[string][]string{}
	groupFingerprints := map[string]string{}
	searchedRequirements := map[string]architecturesearch.SearchResult{}
	seenIDs := map[string]bool{}
	previousID := ""
	for _, fixture := range manifest.Cases {
		if !validSemanticID(fixture.ID) || !validSemanticID(fixture.Group) || seenIDs[fixture.ID] {
			t.Fatalf("invalid or duplicate fixture identity %q", fixture.ID)
		}
		seenIDs[fixture.ID] = true
		if fixture.ID <= previousID {
			t.Fatalf("fixture IDs are not strictly sorted: %q after %q", fixture.ID, previousID)
		}
		previousID = fixture.ID
		domains[fixture.Domain] = true
		outcomes[fixture.Outcome] = true
		groups[fixture.Group] = append(groups[fixture.Group], fixture.ID)
		assertBehaviorOnlyPrompt(t, fixture)
		if hashText(fixture.Prompt) != fixture.PromptSHA256 {
			t.Fatalf("%s prompt hash changed", fixture.ID)
		}
		if statements := PrepareSource(fixture.Prompt).Statements; len(statements) == 0 {
			t.Fatalf("%s has no compiler-owned source statements", fixture.ID)
		}

		proposal, semanticFingerprint := heldOutProposal(t, fixture)
		compiled := Compile(fixture.Prompt, proposal, testCapabilitySHA256)
		if reports.HasBlockingIssue(compiled.Issues) || compiled.Status != fixture.Outcome || (compiled.Status == StatusReady) != (compiled.Requirement != nil) {
			t.Fatalf("%s compilation = %#v", fixture.ID, compiled)
		}
		if fixture.Outcome == StatusReady {
			search, exists := searchedRequirements[semanticFingerprint]
			if !exists {
				search = architecturesearch.Search(context.Background(), *compiled.Requirement, registry, architecturesearch.SearchOptions{CatalogHash: testCapabilitySHA256})
				searchedRequirements[semanticFingerprint] = search
			}
			compiled = ApplySearchEvidence(compiled, search)
			if compiled.Status != StatusReady || compiled.Requirement == nil || search.Status != architecturesearch.SearchSelected || search.Selected == nil {
				t.Fatalf("%s architecture qualification = compilation %#v search %#v", fixture.ID, compiled, search)
			}
		}
		if prior, exists := groupFingerprints[fixture.Group]; exists && prior != semanticFingerprint {
			t.Fatalf("paraphrase group %s changed semantic outcome: %s versus %s", fixture.Group, prior, semanticFingerprint)
		}
		groupFingerprints[fixture.Group] = semanticFingerprint
	}

	for _, domain := range []string{"amplifier", "filter", "power", "protection", "sensor", "mcu"} {
		if !domains[domain] {
			t.Fatalf("held-out corpus is missing domain %s", domain)
		}
	}
	for _, outcome := range []Status{StatusReady, StatusNeedsClarification, StatusUnsupported} {
		if !outcomes[outcome] {
			t.Fatalf("held-out corpus is missing outcome %s", outcome)
		}
	}
	for group, fixtures := range groups {
		if len(fixtures) < 2 {
			t.Fatalf("group %s lacks an independently worded paraphrase: %v", group, fixtures)
		}
	}
	allPrompts := strings.ToLower(string(manifestData))
	for _, required := range []string{"gpio", "i2c", "spi", "uart", "power", "reset", "programming"} {
		if !strings.Contains(allPrompts, required) {
			t.Fatalf("MCU interface corpus is missing %s", required)
		}
	}
}

func heldOutProposal(t *testing.T, fixture heldOutCase) (Proposal, string) {
	t.Helper()
	switch fixture.Outcome {
	case StatusReady:
		data, err := os.ReadFile(fixture.RequirementFile)
		if err != nil {
			t.Fatalf("%s requirement: %v", fixture.ID, err)
		}
		if hashBytesForCorpus(data) != fixture.RequirementSHA256 {
			t.Fatalf("%s requirement hash changed", fixture.ID)
		}
		requirement, issues := architecturesearch.DecodeStrict(bytes.NewReader(data))
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("%s requirement issues = %#v", fixture.ID, issues)
		}
		_, material := collectRequirementIDs(requirement)
		ids := make([]string, 0, len(material))
		for id := range material {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		references := make([]Reference, 0, len(ids))
		for _, id := range ids {
			references = append(references, Reference{Kind: "requirement", ID: id})
		}
		hash, err := architecturesearch.CanonicalHash(requirement)
		if err != nil {
			t.Fatal(err)
		}
		return Proposal{
			Version: ProposalVersion, Requirement: &requirement,
			Coverage:      heldOutCoverage(fixture.Prompt, DispositionCompiled, "the prompt explicitly supplies the behavioral contract and bounded acceptance conditions", references),
			Uncertainties: []Uncertainty{}, Clarifications: []Clarification{}, CapabilityGaps: []CapabilityGap{},
		}, "ready:" + hash
	case StatusNeedsClarification:
		proposal := Proposal{
			Version:        ProposalVersion,
			Coverage:       heldOutCoverage(fixture.Prompt, DispositionClarification, "a safety-relevant operating bound is absent", []Reference{{Kind: "clarification", ID: fixture.ClarificationID}, {Kind: "uncertainty", ID: fixture.UncertaintyID}}),
			Uncertainties:  []Uncertainty{{ID: fixture.UncertaintyID, Path: fixture.ClarificationPath, Kind: fixture.UncertaintyKind, Description: "the source does not provide the bounded value required for safe ratings and corner verification", Resolution: ResolutionClarification, ResolvedBy: fixture.ClarificationID}},
			Clarifications: []Clarification{{ID: fixture.ClarificationID, Path: fixture.ClarificationPath, Question: "What minimum and maximum operating and safety limits must be supported?", WhyNeeded: "safe ratings and worst-case verification require a bounded range", UncertaintyIDs: []string{fixture.UncertaintyID}}},
			CapabilityGaps: []CapabilityGap{},
		}
		return proposal, "clarification:" + fixture.ClarificationPath + ":" + fixture.UncertaintyKind
	case StatusUnsupported:
		proposal := Proposal{
			Version:       ProposalVersion,
			Coverage:      heldOutCoverage(fixture.Prompt, DispositionCapabilityGap, "installed trusted generation or verification capability is unavailable", []Reference{{Kind: "capability_gap", ID: fixture.GapID}}),
			Uncertainties: []Uncertainty{}, Clarifications: []Clarification{},
			CapabilityGaps: []CapabilityGap{{ID: fixture.GapID, Capability: fixture.GapCapability, Path: fixture.GapPath, Reason: "the installed registry cannot generate and verify this behavior fail-closed", RequiredEvidence: []string{"registered semantic architecture capability", "reviewed models and trusted analyses for every declared corner"}}},
		}
		return proposal, "unsupported:" + fixture.GapCapability + ":" + fixture.GapPath
	default:
		t.Fatalf("%s has unsupported expected outcome %q", fixture.ID, fixture.Outcome)
		return Proposal{}, ""
	}
}

func heldOutCoverage(prompt string, disposition Disposition, rationale string, references []Reference) []CoverageRecord {
	statements := PrepareSource(prompt).Statements
	result := make([]CoverageRecord, 0, len(statements))
	for _, statement := range statements {
		result = append(result, CoverageRecord{StatementID: statement.ID, Disposition: disposition, Rationale: rationale, References: slices.Clone(references)})
	}
	return result
}

func assertBehaviorOnlyPrompt(t *testing.T, fixture heldOutCase) {
	t.Helper()
	prompt := strings.ToLower(fixture.Prompt)
	for _, forbidden := range []string{"topology", "op-amp", "opamp", "mosfet", "bjt", "resistor", "capacitor", "footprint", "schematic", "pcb", " net ", "trace", " layer ", "route", "coordinate", "package", "class-a", "class-ab"} {
		if strings.Contains(" "+prompt+" ", forbidden) {
			t.Fatalf("%s prompt contains implementation-detail language %q", fixture.ID, forbidden)
		}
	}
}

func decodeHeldOutStrict(t *testing.T, data []byte, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("held-out corpus must contain exactly one JSON object: %v", err)
	}
}

func hashText(value string) string { return hashBytesForCorpus([]byte(value)) }

func hashBytesForCorpus(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
