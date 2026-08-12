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
	"strconv"
	"strings"
	"testing"
)

const (
	v8AuthorContractFreezeCommit = "03bdfefa161c135ef7d8a4fef07d35445fbdfc2b"
	v8AuthorContractManifestHash = "0c5dfaae62c52bd73445a4a24bb06fe310d10260a4d8c4eb09195f99f1d11d3e"
	v8AuthorPacketSetHash        = "5a243103b6dee088470a521617a88f33685cf2bfb170c68cffa0e1f93bfacc76"
	v8PredecessorRetirementHash  = "1bc6f74cda6745abb8c19cc43dfc760b066277ab57b5169636e289c3195d2706"
	v8SafetyNonSafety            = "non_safety"
	v8SafetyReviewRequired       = "review_required"
	v8SafetyRelevant             = "safety_relevant"
	v8SafetyCritical             = "safety_critical"
)

type v8PacketBinding struct {
	Schema                      string `json:"schema"`
	Version                     int    `json:"version"`
	ContractFreezeCommit        string `json:"contract_freeze_commit"`
	ContractManifest            string `json:"contract_manifest"`
	ContractManifestSHA256      string `json:"contract_manifest_sha256"`
	PredecessorRetirementSHA256 string `json:"predecessor_retirement_sha256"`
	RetiredVersions             []int  `json:"retired_versions"`
	RetiredHeldOutReuse         string `json:"retired_held_out_reuse"`
}

type v8AuthorAssignment struct {
	Schema     string              `json:"schema"`
	Version    int                 `json:"version"`
	AuthorSlot string              `json:"author_slot"`
	Entries    []v8AssignmentEntry `json:"entries"`
}

type v8AssignmentEntry struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	Domain          string `json:"domain"`
	CircuitRole     string `json:"circuit_role"`
	SafetyImpact    string `json:"safety_impact"`
	SourceID        string `json:"source_id"`
	RequirementFile string `json:"requirement_file"`
}

type v8AuthorshipTemplate struct {
	Schema                  string `json:"schema"`
	Version                 int    `json:"version"`
	AuthorContextIdentity   string `json:"author_context_identity"`
	AuthorSlot              string `json:"author_slot"`
	AuthoringToolModel      string `json:"authoring_tool_model_version"`
	AuthoringStartedUTC     string `json:"authoring_started_utc"`
	AuthoringEndedUTC       string `json:"authoring_ended_utc"`
	PerAuthorPacketManifest string `json:"per_author_packet_manifest"`
	PerAuthorPacketSHA256   string `json:"per_author_packet_sha256"`
	ContractBindingSHA256   string `json:"contract_binding_sha256"`
	AssignmentSHA256        string `json:"assignment_sha256"`
	ReturnedBundleRoot      string `json:"returned_bundle_root"`
	RequirementSourceSHA256 []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"requirement_source_sha256"`
	Uncertainties []string `json:"uncertainties"`
	Attestations  struct {
		PacketOnlyInput                                  bool `json:"packet_only_input"`
		ContractBoundBeforeAuthoring                     bool `json:"contract_bound_before_authoring"`
		NoRepositoryOrPriorCorpusAccess                  bool `json:"no_repository_or_prior_corpus_access"`
		NoCrossAuthorAssignmentOrContentAccess           bool `json:"no_cross_author_assignment_or_content_access"`
		IndependentlyConceivedBehaviorOnlyRequirements   bool `json:"independently_conceived_behavior_only_requirements"`
		NoSynthesisSimulationClassificationRankingOrFeas bool `json:"no_synthesis_simulation_classification_ranking_or_feasibility"`
		FixedDiscoveryHeldOutMembership                  bool `json:"fixed_discovery_held_out_membership"`
		NoImplementationOrExpectedOutcomePrescription    bool `json:"no_implementation_or_expected_outcome_prescription"`
		NoObligationAnchorOrCausalPathAuthorship         bool `json:"no_obligation_anchor_or_causal_path_authorship"`
		NoPostEvaluationInspectionOrModification         bool `json:"no_post_evaluation_inspection_or_modification"`
		AllUncertaintiesDisclosed                        bool `json:"all_uncertainties_disclosed"`
	} `json:"attestations"`
}

func TestVersionEightAuthorPacketIsFrozenDisjointAndBalanced(t *testing.T) {
	root := filepath.Join(v8PacketContractDirectory(t), "v8-authoring-packet")
	if got := v8PacketFileSHA256(t, filepath.Join(root, "PACKET_SET.sha256")); got != v8AuthorPacketSetHash {
		t.Fatalf("V8 packet-set SHA-256 = %s, want %s", got, v8AuthorPacketSetHash)
	}
	packetPaths := []string{
		"README_CUSTODIAN.md", "README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md",
		"AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
		"assignments/author_1.json", "assignments/author_2.json", "assignments/author_3.json",
		"assignments/author_4.json", "assignments/author_5.json", "assignments/author_6.json",
		"AUTHOR_1_PACKET.sha256", "AUTHOR_2_PACKET.sha256", "AUTHOR_3_PACKET.sha256",
		"AUTHOR_4_PACKET.sha256", "AUTHOR_5_PACKET.sha256", "AUTHOR_6_PACKET.sha256",
	}
	v8VerifyPacketChecksumManifest(t, root, "PACKET_SET.sha256", packetPaths)
	for author := 1; author <= 6; author++ {
		v8VerifyPacketChecksumManifest(t, root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), []string{
			"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json",
			"CONTRACT_BINDING.json", fmt.Sprintf("assignments/author_%d.json", author),
		})
	}

	var binding v8PacketBinding
	v8DecodePacketStrict(t, filepath.Join(root, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v8" || binding.Version != 8 ||
		binding.ContractFreezeCommit != v8AuthorContractFreezeCommit || binding.ContractManifest != "V8_CONTRACT.sha256" ||
		binding.ContractManifestSHA256 != v8AuthorContractManifestHash || binding.PredecessorRetirementSHA256 != v8PredecessorRetirementHash ||
		!slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4, 5, 6, 7}) || binding.RetiredHeldOutReuse != "prohibited" {
		t.Fatal("V8 author packet binding is invalid")
	}
	var authorship v8AuthorshipTemplate
	v8DecodePacketStrict(t, filepath.Join(root, "AUTHORSHIP_TEMPLATE.json"), &authorship)
	if authorship.Schema != "kicadai.closed-loop-open-set-authorship.v8" || authorship.Version != 8 || len(authorship.RequirementSourceSHA256) != 6 {
		t.Fatal("V8 authorship template does not freeze exactly six source-hash records")
	}

	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	circuitRoles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	safety := map[string]bool{v8SafetyNonSafety: true, v8SafetyReviewRequired: true, v8SafetyRelevant: true, v8SafetyCritical: true}
	safetyCounts := map[string]int{}
	seenIDs, seenSources, seenPaths, seenPairs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	dimensionRoleCounts := map[string]int{}
	dimensionRoleHighSafety := map[string]int{}

	for author := 1; author <= 6; author++ {
		var assignment v8AuthorAssignment
		v8DecodePacketStrict(t, filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), &assignment)
		if assignment.Schema != "kicadai.closed-loop-open-set-author-assignment.v8" || assignment.Version != 8 ||
			assignment.AuthorSlot != fmt.Sprintf("author_%d", author) || len(assignment.Entries) != 6 {
			t.Fatalf("V8 author_%d assignment metadata is invalid", author)
		}
		authorDomains, authorRoles := map[string]bool{}, map[string]bool{}
		for index, entry := range assignment.Entries {
			if (index < 3 && entry.Role != "discovery") || (index >= 3 && entry.Role != "held_out") {
				t.Fatalf("V8 author_%d entry %d has invalid ordered role %q", author, index, entry.Role)
			}
			number := v8AssignmentNumber(t, entry.ID)
			wantRole := "discovery"
			if number > 18 {
				wantRole = "held_out"
			}
			wantSource := fmt.Sprintf("v8_source_%03d", number)
			wantPath := fmt.Sprintf("%s/request_%03d.json", wantRole, number)
			if entry.Role != wantRole || entry.SourceID != wantSource || entry.RequirementFile != wantPath ||
				!slices.Contains(domains, entry.Domain) || !slices.Contains(circuitRoles, entry.CircuitRole) || !safety[entry.SafetyImpact] {
				t.Fatalf("V8 author_%d entry %d is invalid: %#v", author, index, entry)
			}
			if index > 0 && assignment.Entries[index-1].Role == entry.Role && v8AssignmentNumber(t, assignment.Entries[index-1].ID) >= number {
				t.Fatalf("V8 author_%d assignment is not ordered", author)
			}
			pair := entry.Domain + "\x00" + entry.CircuitRole
			if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] || seenPairs[pair] {
				t.Fatal("V8 assignments are not disjoint or Latin-pair unique")
			}
			seenIDs[entry.ID], seenSources[entry.SourceID], seenPaths[entry.RequirementFile], seenPairs[pair] = true, true, true, true
			authorDomains[entry.Domain], authorRoles[entry.CircuitRole] = true, true
			safetyCounts[entry.SafetyImpact]++
			for _, dimension := range []string{"domain:" + entry.Domain, "circuit_role:" + entry.CircuitRole} {
				key := dimension + "\x00" + entry.Role
				dimensionRoleCounts[key]++
				if entry.SafetyImpact == v8SafetyRelevant || entry.SafetyImpact == v8SafetyCritical {
					dimensionRoleHighSafety[key]++
				}
			}
		}
		if len(authorDomains) != 6 || len(authorRoles) != 6 {
			t.Fatalf("V8 author_%d does not cover every domain and circuit role exactly once", author)
		}
	}

	if len(seenIDs) != 36 || len(seenSources) != 36 || len(seenPaths) != 36 || len(seenPairs) != 36 {
		t.Fatal("V8 assignments do not cover exactly 36 unique identities and Latin pairs")
	}
	for number := 1; number <= 36; number++ {
		if !seenIDs[fmt.Sprintf("v8_case_%03d", number)] {
			t.Fatalf("V8 assignment case %03d is missing", number)
		}
	}
	for category := range safety {
		if safetyCounts[category] != 9 {
			t.Fatalf("V8 safety allocation for %s = %d, want 9", category, safetyCounts[category])
		}
	}
	for _, role := range []string{"discovery", "held_out"} {
		for _, domain := range domains {
			v8RequireDimensionRoleBalance(t, dimensionRoleCounts, dimensionRoleHighSafety, "domain:"+domain+"\x00"+role)
		}
		for _, circuitRole := range circuitRoles {
			v8RequireDimensionRoleBalance(t, dimensionRoleCounts, dimensionRoleHighSafety, "circuit_role:"+circuitRole+"\x00"+role)
		}
	}
}

func TestVersionEightAuthorVisibleCommonFilesContainNoConcreteIDs(t *testing.T) {
	root := filepath.Join(v8PacketContractDirectory(t), "v8-authoring-packet")
	concreteIdentity := regexp.MustCompile(`v8_(case|source)_[0-9]{3}`)
	for _, name := range []string{"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json"} {
		if match := concreteIdentity.Find(v8PacketReadFile(t, filepath.Join(root, name))); match != nil {
			t.Fatalf("author-visible common file %s leaks concrete identity %q", name, match)
		}
	}
}

func TestVersionEightAuthorPacketRootManifest(t *testing.T) {
	v8VerifyPacketChecksumManifest(t, v8PacketContractDirectory(t), "V8_AUTHOR_PACKET.sha256", []string{"v8-authoring-packet/PACKET_SET.sha256", "V8_AUTHOR_PACKET_PRISM_REVIEW.md", "v8_author_packet_test.go"})
}

func v8AssignmentNumber(t *testing.T, id string) int {
	t.Helper()
	if !strings.HasPrefix(id, "v8_case_") || len(id) != len("v8_case_000") {
		t.Fatalf("invalid V8 assignment ID %q", id)
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, "v8_case_"))
	if err != nil || number < 1 || number > 36 {
		t.Fatalf("invalid V8 assignment ID %q", id)
	}
	return number
}

func v8RequireDimensionRoleBalance(t *testing.T, counts, high map[string]int, key string) {
	t.Helper()
	if counts[key] != 3 || high[key] < 1 {
		t.Fatalf("V8 assignment balance for %q is count=%d high_safety=%d, want 3 and at least 1", key, counts[key], high[key])
	}
}

func v8VerifyPacketChecksumManifest(t *testing.T, root, name string, wantPaths []string) {
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
			t.Fatalf("invalid V8 packet checksum line %q", line)
		}
		digest, manifestPath := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("invalid V8 packet checksum digest %q", digest)
		}
		cleanPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(manifestPath)))
		if filepath.IsAbs(manifestPath) || manifestPath == "." || manifestPath != cleanPath || manifestPath == ".." ||
			strings.HasPrefix(manifestPath, "../") || strings.Contains(manifestPath, `\`) {
			t.Fatalf("unsafe V8 packet checksum path %q", manifestPath)
		}
		if got := v8PacketFileSHA256(t, filepath.Join(root, filepath.FromSlash(manifestPath))); got != digest {
			t.Fatalf("V8 packet checksum for %s = %s, want %s", manifestPath, got, digest)
		}
		paths = append(paths, manifestPath)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("V8 %s paths = %q, want %q", name, paths, wantPaths)
	}
}

func v8DecodePacketStrict(t *testing.T, path string, value any) {
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

func v8PacketContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V8 packet contract directory")
	}
	return filepath.Dir(file)
}

func v8PacketReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v8PacketFileSHA256(t *testing.T, path string) string {
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
