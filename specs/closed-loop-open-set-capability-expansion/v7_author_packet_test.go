package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const (
	v7AuthorContractFreezeCommit = "e780c8cfca51623d81b9eae209fedf2b98816681"
	v7AuthorContractManifestHash = "40d1f64af6f06763bcb3c04275b56fd4d0c24dafe1940577618d78415408020e"
	v7AuthorPacketSetHash        = "7b0bffb5869cfc215aa97d333bfecb56ee87b730862bceb11fd619181a268451"
)

type v7PacketBinding struct {
	Schema                 string `json:"schema"`
	Version                int    `json:"version"`
	ContractFreezeCommit   string `json:"contract_freeze_commit"`
	ContractManifest       string `json:"contract_manifest"`
	ContractManifestSHA256 string `json:"contract_manifest_sha256"`
	RetiredVersions        []int  `json:"retired_versions"`
	RetiredHeldOutReuse    string `json:"retired_held_out_reuse"`
}

type v7AuthorAssignment struct {
	Schema     string              `json:"schema"`
	Version    int                 `json:"version"`
	AuthorSlot string              `json:"author_slot"`
	Entries    []v7AssignmentEntry `json:"entries"`
}

type v7AssignmentEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

func TestVersionSevenAuthorPacketIsFrozenAndDisjoint(t *testing.T) {
	root := filepath.Join(v7PacketContractDirectory(t), "v7-authoring-packet")
	if got := v7PacketFileSHA256(t, filepath.Join(root, "PACKET_SET.sha256")); got != v7AuthorPacketSetHash {
		t.Fatalf("V7 packet-set SHA-256 = %s, want %s", got, v7AuthorPacketSetHash)
	}
	packetPaths := []string{
		"README_CUSTODIAN.md", "README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md",
		"AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
		"assignments/author_1.json", "assignments/author_2.json", "assignments/author_3.json",
		"AUTHOR_1_PACKET.sha256", "AUTHOR_2_PACKET.sha256", "AUTHOR_3_PACKET.sha256",
	}
	v7VerifyPacketChecksumManifest(t, root, "PACKET_SET.sha256", packetPaths)
	for author := 1; author <= 3; author++ {
		v7VerifyPacketChecksumManifest(t, root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), []string{
			"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json",
			"CONTRACT_BINDING.json", fmt.Sprintf("assignments/author_%d.json", author),
		})
	}

	var binding v7PacketBinding
	v7DecodePacketStrict(t, filepath.Join(root, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v7" || binding.Version != 7 ||
		binding.ContractFreezeCommit != v7AuthorContractFreezeCommit || binding.ContractManifest != "V7_CONTRACT.sha256" ||
		binding.ContractManifestSHA256 != v7AuthorContractManifestHash ||
		!slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4, 5, 6}) || binding.RetiredHeldOutReuse != "prohibited" {
		t.Fatal("V7 author packet binding is invalid")
	}

	domains := []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}
	safety := map[string]bool{"non_safety": true, "review_required": true, "safety_relevant": true, "safety_critical": true}
	safetyCounts := map[string]int{}
	seenIDs, seenSources, seenPaths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for author := 1; author <= 3; author++ {
		var assignment v7AuthorAssignment
		v7DecodePacketStrict(t, filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), &assignment)
		if assignment.Schema != "kicadai.closed-loop-open-set-author-assignment.v7" || assignment.Version != 7 ||
			assignment.AuthorSlot != fmt.Sprintf("author_%d", author) || len(assignment.Entries) != 12 {
			t.Fatalf("V7 author_%d assignment metadata is invalid", author)
		}
		for index, entry := range assignment.Entries {
			role := "discovery"
			number := (author-1)*6 + index + 1
			if index >= 6 {
				role = "held_out"
				number = 18 + (author-1)*6 + (index - 6) + 1
			}
			wantID := fmt.Sprintf("v7_case_%03d", number)
			wantSource := fmt.Sprintf("v7_source_%03d", number)
			wantPath := fmt.Sprintf("%s/request_%03d.json", role, number)
			if entry.ID != wantID || entry.SourceID != wantSource || entry.RequirementFile != wantPath ||
				entry.Role != role || entry.Domain != domains[index%6] || !safety[entry.SafetyImpact] {
				t.Fatalf("V7 author_%d entry %d is invalid: %#v", author, index, entry)
			}
			if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] {
				t.Fatal("V7 assignments are not disjoint")
			}
			seenIDs[entry.ID], seenSources[entry.SourceID], seenPaths[entry.RequirementFile] = true, true, true
			safetyCounts[entry.SafetyImpact]++
		}
	}
	if len(seenIDs) != 36 || len(seenSources) != 36 || len(seenPaths) != 36 {
		t.Fatal("V7 assignments do not cover exactly 36 unique entries")
	}
	for category := range safety {
		if safetyCounts[category] < 6 || safetyCounts[category] > 12 {
			t.Fatalf("V7 safety allocation for %s = %d, want 6..12", category, safetyCounts[category])
		}
	}
}

func TestVersionSevenAuthorVisibleCommonFilesContainNoConcreteIDs(t *testing.T) {
	root := filepath.Join(v7PacketContractDirectory(t), "v7-authoring-packet")
	concreteIdentity := regexp.MustCompile(`v7_(case|source)_[0-9]{3}`)
	for _, name := range []string{
		"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
	} {
		if match := concreteIdentity.Find(v7PacketReadFile(t, filepath.Join(root, name))); match != nil {
			t.Fatalf("author-visible common file %s leaks concrete identity %q", name, match)
		}
	}
}

func TestVersionSevenAuthorPacketRootManifest(t *testing.T) {
	v7VerifyPacketChecksumManifest(t, v7PacketContractDirectory(t), "V7_AUTHOR_PACKET.sha256", []string{
		"v7-authoring-packet/PACKET_SET.sha256",
		"v7_author_packet_test.go",
	})
}

func v7VerifyPacketChecksumManifest(t *testing.T, root, name string, wantPaths []string) {
	t.Helper()
	file, err := os.Open(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	paths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " || strings.TrimSpace(line[66:]) == "" {
			t.Fatalf("invalid V7 packet checksum line %q", line)
		}
		digest, manifestPath := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("invalid V7 packet checksum digest %q", digest)
		}
		cleanPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(manifestPath)))
		if filepath.IsAbs(manifestPath) || manifestPath == "." || manifestPath != cleanPath ||
			manifestPath == ".." || strings.HasPrefix(manifestPath, "../") || strings.Contains(manifestPath, `\`) {
			t.Fatalf("unsafe V7 packet checksum path %q", manifestPath)
		}
		if got := v7PacketFileSHA256(t, filepath.Join(root, filepath.FromSlash(manifestPath))); got != digest {
			t.Fatalf("V7 packet checksum for %s = %s, want %s", manifestPath, got, digest)
		}
		paths = append(paths, manifestPath)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("V7 %s paths = %q, want %q", name, paths, wantPaths)
	}
}

func v7DecodePacketStrict(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("%s contains trailing JSON", filepath.Base(path))
	}
}

func v7PacketContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V7 packet contract directory")
	}
	return filepath.Dir(file)
}

func v7PacketReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v7PacketFileSHA256(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
