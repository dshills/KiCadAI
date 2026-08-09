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

const v3AuthorManifestSchema = "kicadai.closed-loop-open-set-author-manifest.v3"

type v3AuthorManifest struct {
	Schema  string                  `json:"schema"`
	Version int                     `json:"version"`
	Entries []v3AuthorManifestEntry `json:"entries"`
}

type v3AuthorManifestEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

func TestVersionThreeAuthoringPacketIsFrozenAndComplete(t *testing.T) {
	directory := filepath.Join(v3ContractDirectory(t), "v3-authoring-packet")
	manifest, err := os.Open(filepath.Join(directory, "PACKET.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || filepath.Base(fields[1]) != fields[1] || seen[fields[1]] {
			t.Fatalf("invalid V3 authoring packet entry %q", scanner.Text())
		}
		if got := v3FileSHA256(t, filepath.Join(directory, fields[1])); got != fields[0] {
			t.Fatalf("V3 authoring packet %s hash = %s, want %s", fields[1], got, fields[0])
		}
		seen[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP.md", "AUTHOR_MANIFEST.json"} {
		if !seen[required] {
			t.Fatalf("V3 authoring packet omits %s", required)
		}
	}

	var authored v3AuthorManifest
	v3DecodeStrictFile(t, filepath.Join(directory, "AUTHOR_MANIFEST.json"), &authored)
	if authored.Schema != v3AuthorManifestSchema || authored.Version != 3 || len(authored.Entries) != 24 {
		t.Fatalf("V3 author manifest header/size = %q/%d/%d", authored.Schema, authored.Version, len(authored.Entries))
	}
	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	seenSources := map[string]bool{}
	for index, entry := range authored.Entries {
		role, directoryName := "discovery", "discovery"
		if index >= 12 {
			role, directoryName = "held_out", "held_out"
		}
		wantID := fmt.Sprintf("v3_case_%03d", index+1)
		wantSource := fmt.Sprintf("v3_source_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", directoryName, index+1)
		wantDomain := domains[index%12/2]
		if entry.ID != wantID || entry.SourceID != wantSource || entry.Role != role || entry.Domain != wantDomain || entry.RequirementFile != wantFile {
			t.Fatalf("V3 author manifest entry %d has noncanonical identity, role, domain, or path", index+1)
		}
		if seenSources[entry.SourceID] {
			t.Fatalf("V3 author manifest source identity %d is duplicated", index+1)
		}
		seenSources[entry.SourceID] = true
		counts[role][entry.Domain]++
		switch entry.SafetyImpact {
		case "non_safety", "review_required", "safety_relevant", "safety_critical":
		default:
			t.Fatalf("V3 author manifest entry %d has invalid safety impact", index+1)
		}
	}
	for role, byDomain := range counts {
		for _, domain := range domains {
			if byDomain[domain] != 2 {
				t.Fatalf("V3 author manifest %s/%s count = %d, want 2", role, domain, byDomain[domain])
			}
		}
	}
}
