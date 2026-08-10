package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
)

const (
	v5AuthorAssignmentSchema = "kicadai.closed-loop-open-set-author-assignment.v5"
	v5ContractFreezeCommit   = "a9249879d5e02575fe047925d613458ffec62030"
)

type v5AuthorAssignment struct {
	Schema     string                    `json:"schema"`
	Version    int                       `json:"version"`
	AuthorSlot string                    `json:"author_slot"`
	Entries    []v5AuthorAssignmentEntry `json:"entries"`
}

type v5AuthorAssignmentEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

type v5AuthoringContractBinding struct {
	Schema                    string `json:"schema"`
	Version                   int    `json:"version"`
	ContractFreezeCommit      string `json:"contract_freeze_commit"`
	ContractManifest          string `json:"contract_manifest"`
	ContractManifestSHA256    string `json:"contract_manifest_sha256"`
	RetiredVersions           []int  `json:"retired_versions"`
	RetiredHeldOutReusePolicy string `json:"retired_held_out_reuse"`
}

func TestVersionFiveAuthoringPacketSetIsFrozenAndComplete(t *testing.T) {
	directory := v5AuthoringPacketDirectory(t)
	want := map[string]bool{
		"README_CUSTODIAN.md":            true,
		"README.md":                      true,
		"PUBLIC_REQUIREMENT_CONTRACT.md": true,
		"CORPUS_RULES.md":                true,
		"AUTHORSHIP_TEMPLATE.json":       true,
		"CONTRACT_BINDING.json":          true,
		"assignments/author_1.json":      true,
		"assignments/author_2.json":      true,
		"assignments/author_3.json":      true,
		"AUTHOR_1_PACKET.sha256":         true,
		"AUTHOR_2_PACKET.sha256":         true,
		"AUTHOR_3_PACKET.sha256":         true,
	}
	v5VerifyPacketManifest(t, directory, "PACKET_SET.sha256", want)

	rootEntries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := map[string]bool{
		"README_CUSTODIAN.md": true, "README.md": true,
		"PUBLIC_REQUIREMENT_CONTRACT.md": true, "CORPUS_RULES.md": true,
		"AUTHORSHIP_TEMPLATE.json": true, "CONTRACT_BINDING.json": true,
		"assignments":            true,
		"AUTHOR_1_PACKET.sha256": true, "AUTHOR_2_PACKET.sha256": true,
		"AUTHOR_3_PACKET.sha256": true, "PACKET_SET.sha256": true,
	}
	for _, entry := range rootEntries {
		if !wantRoot[entry.Name()] {
			t.Fatalf("V5 authoring packet set exposes unexpected root input %q", entry.Name())
		}
		delete(wantRoot, entry.Name())
	}
	if len(wantRoot) != 0 {
		t.Fatalf("V5 authoring packet set omits root inputs: %v", wantRoot)
	}

	assignmentEntries, err := os.ReadDir(filepath.Join(directory, "assignments"))
	if err != nil {
		t.Fatal(err)
	}
	if len(assignmentEntries) != 3 {
		t.Fatalf("V5 assignment count = %d, want 3", len(assignmentEntries))
	}
	for index, entry := range assignmentEntries {
		wantName := fmt.Sprintf("author_%d.json", index+1)
		if entry.IsDir() || entry.Name() != wantName {
			t.Fatalf("V5 assignment entry %d = %q, want %q", index, entry.Name(), wantName)
		}
	}
}

func TestVersionFivePerAuthorPacketsAreDisjointAndFrozen(t *testing.T) {
	directory := v5AuthoringPacketDirectory(t)
	common := map[string]bool{
		"README.md": true, "PUBLIC_REQUIREMENT_CONTRACT.md": true,
		"CORPUS_RULES.md": true, "AUTHORSHIP_TEMPLATE.json": true,
		"CONTRACT_BINDING.json": true,
	}
	for author := 1; author <= 3; author++ {
		want := make(map[string]bool, len(common)+1)
		for name := range common {
			want[name] = true
		}
		want[fmt.Sprintf("assignments/author_%d.json", author)] = true
		v5VerifyPacketManifest(t, directory, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), want)
	}
}

func TestVersionFiveAuthoringPacketBindsFrozenContract(t *testing.T) {
	directory := v5AuthoringPacketDirectory(t)
	var binding v5AuthoringContractBinding
	v5DecodeStrictFile(t, filepath.Join(directory, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v5" || binding.Version != 5 {
		t.Fatalf("V5 authoring contract binding header = %q/%d", binding.Schema, binding.Version)
	}
	if binding.ContractFreezeCommit != v5ContractFreezeCommit {
		t.Fatalf("V5 authoring contract freeze commit = %q", binding.ContractFreezeCommit)
	}
	if binding.ContractManifest != "V5_CONTRACT.sha256" {
		t.Fatalf("V5 authoring contract manifest = %q", binding.ContractManifest)
	}
	manifestPath := filepath.Join(v5ContractDirectory(t), binding.ContractManifest)
	if got := v5FileSHA256(t, manifestPath); got != binding.ContractManifestSHA256 {
		t.Fatalf("V5 authoring contract manifest hash = %s, want %s", got, binding.ContractManifestSHA256)
	}
	if !slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4}) || binding.RetiredHeldOutReusePolicy != "prohibited" {
		t.Fatalf("V5 authoring retirement binding = %v/%q", binding.RetiredVersions, binding.RetiredHeldOutReusePolicy)
	}
}

func TestVersionFiveAuthorshipTemplateIsStrictAndComplete(t *testing.T) {
	path := filepath.Join(v5AuthoringPacketDirectory(t), "AUTHORSHIP_TEMPLATE.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	authorship, err := corpusfreeze.DecodeAuthorshipStrict(data)
	if err != nil {
		t.Fatal(err)
	}
	if authorship.Schema != "kicadai.closed-loop-open-set-authorship.v5" || authorship.Version != 5 || !authorship.Attestations.AllTrue() {
		t.Fatalf("V5 authorship template header or attestations are incomplete")
	}
	if len(authorship.RequirementSourceSHA256) != 1 || !strings.Contains(authorship.AuthorSlot, "[") ||
		!strings.Contains(authorship.RequirementSourceSHA256[0].Path, "[") || !strings.Contains(authorship.RequirementSourceSHA256[0].SHA256, "[") {
		t.Fatal("V5 authorship template does not expose the required replaceable fields")
	}
}

func TestVersionFiveAuthorAssignmentsAreCanonicalAndBalanced(t *testing.T) {
	directory := v5AuthoringPacketDirectory(t)
	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	counts := map[string]map[string]int{"discovery": {}, "held_out": {}}
	criticalDomains := map[string]map[string]bool{"discovery": {}, "held_out": {}}
	seenIDs := map[string]bool{}
	seenSources := map[string]bool{}
	seenFiles := map[string]bool{}

	for author := 1; author <= 3; author++ {
		path := filepath.Join(directory, "assignments", fmt.Sprintf("author_%d.json", author))
		var assignment v5AuthorAssignment
		v5DecodeStrictFile(t, path, &assignment)
		wantSlot := fmt.Sprintf("author_%d", author)
		if assignment.Schema != v5AuthorAssignmentSchema || assignment.Version != 5 || assignment.AuthorSlot != wantSlot || len(assignment.Entries) != 12 {
			t.Fatalf("V5 author %d assignment header/size = %q/%d/%q/%d", author, assignment.Schema, assignment.Version, assignment.AuthorSlot, len(assignment.Entries))
		}

		for localIndex, entry := range assignment.Entries {
			role := "discovery"
			caseNumber := (author-1)*6 + localIndex + 1
			if localIndex >= 6 {
				role = "held_out"
				caseNumber = 18 + (author-1)*6 + localIndex - 5
			}
			wantID := fmt.Sprintf("v5_case_%03d", caseNumber)
			wantSource := fmt.Sprintf("v5_source_%03d", caseNumber)
			wantFile := fmt.Sprintf("%s/request_%03d.json", role, caseNumber)
			wantDomain := domains[localIndex%6]
			if entry.ID != wantID || entry.SourceID != wantSource || entry.Role != role || entry.Domain != wantDomain || entry.RequirementFile != wantFile {
				t.Fatalf("V5 %s entry %d is noncanonical: %+v", wantSlot, localIndex+1, entry)
			}
			if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenFiles[entry.RequirementFile] {
				t.Fatalf("V5 %s entry %d duplicates an identity or path", wantSlot, localIndex+1)
			}
			seenIDs[entry.ID], seenSources[entry.SourceID], seenFiles[entry.RequirementFile] = true, true, true
			counts[role][entry.Domain]++
			switch entry.SafetyImpact {
			case "non_safety", "review_required", "safety_relevant":
			case "safety_critical":
				criticalDomains[role][entry.Domain] = true
			default:
				t.Fatalf("V5 %s entry %d has invalid safety impact %q", wantSlot, localIndex+1, entry.SafetyImpact)
			}
		}
	}

	if len(seenIDs) != 36 || len(seenSources) != 36 || len(seenFiles) != 36 {
		t.Fatalf("V5 assignment uniqueness counts = %d/%d/%d, want 36 each", len(seenIDs), len(seenSources), len(seenFiles))
	}
	for role, byDomain := range counts {
		for _, domain := range domains {
			if byDomain[domain] != 3 {
				t.Fatalf("V5 assignment %s/%s count = %d, want 3", role, domain, byDomain[domain])
			}
		}
		if len(criticalDomains[role]) < 4 {
			t.Fatalf("V5 assignment %s safety-critical domains = %d, want at least 4", role, len(criticalDomains[role]))
		}
	}
}

func TestVersionFiveAuthorInputsContainNoCorpusOrExecutableMaterial(t *testing.T) {
	directory := v5AuthoringPacketDirectory(t)
	for author := 1; author <= 3; author++ {
		manifestName := fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author)
		names := v5PacketManifestNames(t, filepath.Join(directory, manifestName))
		wantAssignment := fmt.Sprintf("assignments/author_%d.json", author)
		for _, name := range names {
			extension := strings.ToLower(filepath.Ext(name))
			if extension != ".md" && extension != ".json" {
				t.Fatalf("V5 author %d receives non-document input %q", author, name)
			}
			if strings.Contains(name, "README_CUSTODIAN") || strings.Contains(name, "PACKET_SET") || (strings.HasPrefix(name, "assignments/") && name != wantAssignment) {
				t.Fatalf("V5 author %d receives prohibited packet-set or cross-author input %q", author, name)
			}
		}
	}
}

func v5AuthoringPacketDirectory(t *testing.T) string {
	t.Helper()
	return filepath.Join(v5ContractDirectory(t), "v5-authoring-packet")
}

func v5VerifyPacketManifest(t *testing.T, directory, manifestName string, want map[string]bool) {
	t.Helper()
	got := map[string]bool{}
	for _, name := range v5PacketManifestNames(t, filepath.Join(directory, manifestName)) {
		if !want[name] {
			t.Fatalf("V5 packet manifest %s contains unexpected entry %q", manifestName, name)
		}
		if got[name] {
			t.Fatalf("V5 packet manifest %s duplicates %q", manifestName, name)
		}
		got[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("V5 packet manifest %s has %d entries, want %d", manifestName, len(got), len(want))
	}
}

func v5PacketManifestNames(t *testing.T, manifestPath string) []string {
	t.Helper()
	return v5PacketManifestNamesAt(t, manifestPath, filepath.Dir(manifestPath))
}

func v5PacketManifestNamesAt(t *testing.T, manifestPath, contentRoot string) []string {
	t.Helper()
	manifest, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	var names []string
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= sha256.Size*2+2 || line[sha256.Size*2:sha256.Size*2+2] != "  " {
			t.Fatalf("invalid V5 packet manifest entry %q", line)
		}
		wantHash, name := line[:sha256.Size*2], line[sha256.Size*2+2:]
		if strings.TrimSpace(name) != name || strings.ContainsAny(name, `\:`) || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
			t.Fatalf("unsafe V5 packet manifest entry %q", line)
		}
		if gotHash := v5FileSHA256(t, filepath.Join(contentRoot, filepath.FromSlash(name))); gotHash != wantHash {
			t.Fatalf("V5 packet file %s hash = %s, want %s", name, gotHash, wantHash)
		}
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}
