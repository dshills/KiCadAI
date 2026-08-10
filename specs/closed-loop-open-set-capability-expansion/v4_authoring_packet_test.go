package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const v4AuthorManifestSchema = "kicadai.closed-loop-open-set-author-manifest.v4"

type v4AuthorManifest struct {
	Schema  string                  `json:"schema"`
	Version int                     `json:"version"`
	Entries []v4AuthorManifestEntry `json:"entries"`
}

type v4AuthorManifestEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

func TestVersionFourAuthoringPacketIsFrozenAndComplete(t *testing.T) {
	directory := filepath.Join(v4ContractDirectory(t), "v4-authoring-packet")
	manifest, err := os.Open(filepath.Join(directory, "PACKET.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= sha256.Size*2+2 || line[sha256.Size*2:sha256.Size*2+2] != "  " {
			t.Fatalf("invalid V4 authoring packet entry %q", scanner.Text())
		}
		wantHash, name := line[:sha256.Size*2], line[sha256.Size*2+2:]
		if strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") || seen[name] {
			t.Fatalf("invalid V4 authoring packet entry %q", scanner.Text())
		}
		if got := v4FileSHA256(t, filepath.Join(directory, name)); got != wantHash {
			t.Fatalf("V4 authoring packet %s hash = %s, want %s", name, got, wantHash)
		}
		seen[name] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.md", "AUTHOR_MANIFEST.json",
	} {
		if !seen[required] {
			t.Fatalf("V4 authoring packet omits %s", required)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("V4 authoring packet contains %d inputs, want exactly 5", len(seen))
	}
}

func TestVersionFourAuthorManifestAssignmentsAreCanonical(t *testing.T) {
	directory := filepath.Join(v4ContractDirectory(t), "v4-authoring-packet")
	var authored v4AuthorManifest
	v4DecodeStrictFile(t, filepath.Join(directory, "AUTHOR_MANIFEST.json"), &authored)
	if authored.Schema != v4AuthorManifestSchema || authored.Version != 4 || len(authored.Entries) != 24 {
		t.Fatalf("V4 author manifest header/size = %q/%d/%d", authored.Schema, authored.Version, len(authored.Entries))
	}

	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	seenIDs := map[string]bool{}
	seenSources := map[string]bool{}
	seenFiles := map[string]bool{}
	criticalDomains := map[string]map[string]bool{"discovery": {}, "held_out": {}}
	for index, entry := range authored.Entries {
		role, directoryName := "discovery", "discovery"
		if index >= 12 {
			role, directoryName = "held_out", "held_out"
		}
		wantID := fmt.Sprintf("v4_case_%03d", index+1)
		wantSource := fmt.Sprintf("v4_source_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", directoryName, index+1)
		wantDomain := domains[index%12/2]
		if entry.ID != wantID || entry.SourceID != wantSource || entry.Role != role || entry.Domain != wantDomain || entry.RequirementFile != wantFile {
			t.Fatalf("V4 author manifest entry %d has noncanonical identity, role, domain, or path: %+v", index+1, entry)
		}
		if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenFiles[entry.RequirementFile] {
			t.Fatalf("V4 author manifest entry %d duplicates an identity or path", index+1)
		}
		seenIDs[entry.ID], seenSources[entry.SourceID], seenFiles[entry.RequirementFile] = true, true, true
		counts[role][entry.Domain]++
		switch entry.SafetyImpact {
		case "non_safety", "review_required", "safety_relevant":
		case "safety_critical":
			criticalDomains[role][entry.Domain] = true
		default:
			t.Fatalf("V4 author manifest entry %d has invalid safety impact %q", index+1, entry.SafetyImpact)
		}
	}
	for role, byDomain := range counts {
		for _, domain := range domains {
			if byDomain[domain] != 2 {
				t.Fatalf("V4 author manifest %s/%s count = %d, want 2", role, domain, byDomain[domain])
			}
		}
		if len(criticalDomains[role]) < 3 {
			t.Fatalf("V4 author manifest %s safety-critical domains = %d, want at least 3", role, len(criticalDomains[role]))
		}
	}
}

func TestVersionFourAuthoringPacketContainsNoCorpusOrExecutableInput(t *testing.T) {
	directory := filepath.Join(v4ContractDirectory(t), "v4-authoring-packet")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"AUTHORSHIP_TEMPLATE.md":         true,
		"AUTHOR_MANIFEST.json":           true,
		"CORPUS_RULES.md":                true,
		"PACKET.sha256":                  true,
		"PUBLIC_REQUIREMENT_CONTRACT.md": true,
		"README.md":                      true,
	}
	for _, entry := range entries {
		if entry.IsDir() || !allowed[entry.Name()] {
			t.Fatalf("V4 authoring packet exposes non-public input %q", entry.Name())
		}
	}
}
