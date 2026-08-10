package closedloopopensetcontract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	v6ContractFreezeCommit = "65f6da46c25abe7f77479754f0260e02403f7f8c"
	v6ContractManifestHash = "19e2deaf80c4d97a3cf2f7405a51d97df7ce6f2172a1e1830c57177b7c2a7c60"
	v6PacketSetHash        = "3ca4f144bb455aeeb8e454eb74584b5c7286f2e0e76b0eaf3c5f2e83dd1e5e49"
)

type v6PacketBinding struct {
	Schema                 string `json:"schema"`
	Version                int    `json:"version"`
	ContractFreezeCommit   string `json:"contract_freeze_commit"`
	ContractManifest       string `json:"contract_manifest"`
	ContractManifestSHA256 string `json:"contract_manifest_sha256"`
	RetiredVersions        []int  `json:"retired_versions"`
	RetiredHeldOutReuse    string `json:"retired_held_out_reuse"`
}

type v6AuthorAssignment struct {
	Schema     string              `json:"schema"`
	Version    int                 `json:"version"`
	AuthorSlot string              `json:"author_slot"`
	Entries    []v6AssignmentEntry `json:"entries"`
}

type v6AssignmentEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

func TestVersionSixAuthorPacketIsFrozenAndDisjoint(t *testing.T) {
	root := filepath.Join(v6ContractDirectory(t), "v6-authoring-packet")
	if got := v6FileSHA256(t, filepath.Join(root, "PACKET_SET.sha256")); got != v6PacketSetHash {
		t.Fatalf("V6 packet-set SHA-256 = %s, want %s", got, v6PacketSetHash)
	}
	packetPaths := []string{
		"README_CUSTODIAN.md", "README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md",
		"AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
		"assignments/author_1.json", "assignments/author_2.json", "assignments/author_3.json",
		"AUTHOR_1_PACKET.sha256", "AUTHOR_2_PACKET.sha256", "AUTHOR_3_PACKET.sha256",
	}
	v6VerifyChecksumManifest(t, root, "PACKET_SET.sha256", packetPaths)
	for author := 1; author <= 3; author++ {
		v6VerifyChecksumManifest(t, root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), []string{
			"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json",
			"CONTRACT_BINDING.json", fmt.Sprintf("assignments/author_%d.json", author),
		})
	}

	var binding v6PacketBinding
	v6DecodeStrict(t, filepath.Join(root, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v6" || binding.Version != 6 ||
		binding.ContractFreezeCommit != v6ContractFreezeCommit || binding.ContractManifest != "V6_CONTRACT.sha256" ||
		binding.ContractManifestSHA256 != v6ContractManifestHash || !slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4, 5}) ||
		binding.RetiredHeldOutReuse != "prohibited" {
		t.Fatal("V6 author packet binding is invalid")
	}

	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	safety := map[string]bool{"non_safety": true, "review_required": true, "safety_relevant": true, "safety_critical": true}
	seenIDs, seenSources, seenPaths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for author := 1; author <= 3; author++ {
		var assignment v6AuthorAssignment
		v6DecodeStrict(t, filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), &assignment)
		if assignment.Schema != "kicadai.closed-loop-open-set-author-assignment.v6" || assignment.Version != 6 ||
			assignment.AuthorSlot != fmt.Sprintf("author_%d", author) || len(assignment.Entries) != 12 {
			t.Fatalf("V6 author_%d assignment metadata is invalid", author)
		}
		for index, entry := range assignment.Entries {
			role := "discovery"
			number := (author-1)*6 + index + 1
			if index >= 6 {
				role = "held_out"
				number = 18 + (author-1)*6 + (index - 6) + 1
			}
			wantID := fmt.Sprintf("v6_case_%03d", number)
			wantSource := fmt.Sprintf("v6_source_%03d", number)
			wantPath := fmt.Sprintf("%s/request_%03d.json", role, number)
			if entry.ID != wantID || entry.SourceID != wantSource || entry.RequirementFile != wantPath ||
				entry.Role != role || entry.Domain != domains[index%6] || !safety[entry.SafetyImpact] {
				t.Fatalf("V6 author_%d entry %d is invalid: %#v", author, index, entry)
			}
			if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] {
				t.Fatal("V6 assignments are not disjoint")
			}
			seenIDs[entry.ID], seenSources[entry.SourceID], seenPaths[entry.RequirementFile] = true, true, true
		}
	}
	if len(seenIDs) != 36 || len(seenSources) != 36 || len(seenPaths) != 36 {
		t.Fatal("V6 assignments do not cover exactly 36 unique entries")
	}
}

func TestVersionSixAuthorVisibleCommonFilesContainNoConcreteIDs(t *testing.T) {
	root := filepath.Join(v6ContractDirectory(t), "v6-authoring-packet")
	concreteIdentity := regexp.MustCompile(`v6_(case|source)_[0-9]{3}`)
	for _, name := range []string{
		"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
	} {
		if match := concreteIdentity.Find(v6ReadFile(t, filepath.Join(root, name))); match != nil {
			t.Fatalf("author-visible common file %s leaks concrete identity %q", name, match)
		}
	}
}

func TestVersionSixAuthorPacketRootManifest(t *testing.T) {
	v6VerifyChecksumManifest(t, v6ContractDirectory(t), "V6_AUTHOR_PACKET.sha256", []string{
		"v6-authoring-packet/PACKET_SET.sha256",
		"v6_author_packet_test.go",
	})
}

func v6VerifyChecksumManifest(t *testing.T, root, name string, wantPaths []string) {
	t.Helper()
	file, err := os.Open(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	paths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Fatalf("invalid V6 packet checksum line %q", scanner.Text())
		}
		if got := v6FileSHA256(t, filepath.Join(root, filepath.FromSlash(fields[1]))); got != fields[0] {
			t.Fatalf("V6 packet checksum for %s = %s, want %s", fields[1], got, fields[0])
		}
		paths = append(paths, fields[1])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("V6 %s paths = %q, want %q", name, paths, wantPaths)
	}
}

func v6DecodeStrict(t *testing.T, path string, value any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(string(v6ReadFile(t, path))))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("%s contains trailing JSON", filepath.Base(path))
	}
}
