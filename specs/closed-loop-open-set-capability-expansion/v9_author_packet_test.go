package closedloopopensetcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/atomicdir"
)

const (
	v9AuthorPacketUpdateEnv   = "UPDATE_CLOSED_LOOP_V9_AUTHOR_PACKET"
	v9AuthorContractCommit    = "089f45f794029de35fe5489b73a6d2087528edc2"
	v9AuthorContractHash      = "5636cd4829f1a1323c19bd98720ddaa46465cb716410ac86919eaa063b3753e4"
	v9AuthorCorpusRulesHash   = "99033fbbdb35f06687759936665a2502bf862523f3ef4dfc0d3059d980711745"
	v9AuthorPredecessorHash   = "8c16b1a406b717851de511377660edba9db0c701e375a7d2c189eaf2abe9c06e"
	v9AuthorPacketSetHash     = "276c9741b299a75e9838bd2aab6e48683634fad7d8db77446eb5676e4a6af6a1"
	v9AuthorPacketSchema      = "kicadai.closed-loop-open-set-author-assignment.v9"
	v9AuthorPacketRoot        = "v9-authoring-packet"
	v9AuthorPacketSetManifest = "PACKET_SET.sha256"
)

var (
	v9AuthorDomains = []string{
		"analog_signal_path", "power_energy_conversion", "digital_control",
		"mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity",
	}
	v9AuthorCircuitRoles = []string{
		"source_bias", "amplification_conditioning", "conversion_regulation",
		"sensing_measurement", "interface_control", "protection_supervision",
	}
	v9AuthorSafety    = []string{"non_safety", "review_required", "safety_relevant", "safety_critical"}
	v9StaticAnalyses  = []string{"dc_operating_point", "dc_sweep", "thermal", "electrothermal"}
	v9DynamicAnalyses = []string{"transient", "ac_sweep", "startup", "stability"}
)

type v9PacketBinding struct {
	Schema                      string `json:"schema"`
	Version                     int    `json:"version"`
	ContractFreezeCommit        string `json:"contract_freeze_commit"`
	ContractManifest            string `json:"contract_manifest"`
	ContractManifestSHA256      string `json:"contract_manifest_sha256"`
	PredecessorRetirementSHA256 string `json:"predecessor_retirement_sha256"`
	RetiredVersions             []int  `json:"retired_versions"`
	RetiredHeldOutReuse         string `json:"retired_held_out_reuse"`
}

type v9AuthorAssignment struct {
	Schema     string              `json:"schema"`
	Version    int                 `json:"version"`
	AuthorSlot string              `json:"author_slot"`
	Entries    []v9AssignmentEntry `json:"entries"`
}

type v9AssignmentEntry struct {
	ID                      string `json:"id"`
	Role                    string `json:"role"`
	Domain                  string `json:"domain"`
	CircuitRole             string `json:"circuit_role"`
	SafetyImpact            string `json:"safety_impact"`
	PrimaryClass            string `json:"primary_class"`
	RequiredPrimaryAnalysis string `json:"required_primary_analysis"`
	OutputMultiplicity      string `json:"output_multiplicity"`
	RequireOffNominal       bool   `json:"require_off_nominal"`
	SourceID                string `json:"source_id"`
	RequirementFile         string `json:"requirement_file"`
}

func TestUpdateVersionNineAuthorPacket(t *testing.T) {
	if os.Getenv(v9AuthorPacketUpdateEnv) != "1" {
		t.Skip("set " + v9AuthorPacketUpdateEnv + "=1 to create the V9 author packet")
	}
	directory := v9PacketContractDirectory(t)
	root := filepath.Join(directory, v9AuthorPacketRoot)
	if _, err := os.Stat(root); err == nil {
		t.Fatal("V9 author packet root already exists; refusing replacement")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect V9 author packet root: %v", err)
	}
	if got := v9PacketFileSHA256(t, filepath.Join(directory, "V9_CONTRACT.sha256")); got != v9AuthorContractHash {
		t.Fatalf("V9 contract manifest = %s, want %s", got, v9AuthorContractHash)
	}
	if got := v9PacketFileSHA256(t, filepath.Join(directory, "V9_CORPUS_RULES.md")); got != v9AuthorCorpusRulesHash {
		t.Fatalf("V9 packet corpus-rules source = %s, want %s", got, v9AuthorCorpusRulesHash)
	}
	if err := atomicdir.Publish(root, func(staging string) error {
		return writeV9AuthorPacket(directory, staging)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVersionNineAuthorPacketIsFrozenDisjointAndBalanced(t *testing.T) {
	root := filepath.Join(v9PacketContractDirectory(t), v9AuthorPacketRoot)
	if got := v9PacketFileSHA256(t, filepath.Join(root, v9AuthorPacketSetManifest)); got != v9AuthorPacketSetHash {
		t.Fatalf("V9 packet-set SHA-256 = %s, want %s", got, v9AuthorPacketSetHash)
	}
	packetPaths := v9PacketSetPaths()
	v9VerifyPacketChecksumManifest(t, root, v9AuthorPacketSetManifest, packetPaths)
	wantFiles := append(slices.Clone(packetPaths), v9AuthorPacketSetManifest)
	slices.Sort(wantFiles)
	actualFiles := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		actualFiles = append(actualFiles, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(actualFiles)
	if !slices.Equal(actualFiles, wantFiles) {
		t.Fatalf("V9 author packet files = %q, want %q", actualFiles, wantFiles)
	}
	for author := 1; author <= 6; author++ {
		v9VerifyPacketChecksumManifest(t, root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), v9PerAuthorPaths(author))
	}

	var binding v9PacketBinding
	v9DecodePacketStrict(t, filepath.Join(root, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v9" || binding.Version != 9 ||
		binding.ContractFreezeCommit != v9AuthorContractCommit || binding.ContractManifest != "V9_CONTRACT.sha256" ||
		binding.ContractManifestSHA256 != v9AuthorContractHash || binding.PredecessorRetirementSHA256 != v9AuthorPredecessorHash ||
		!slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4, 5, 6, 7, 8}) || binding.RetiredHeldOutReuse != "prohibited" {
		t.Fatal("V9 author packet binding is invalid")
	}

	seenIDs, seenSources, seenPaths, seenTriples := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	safetyCounts := map[string]int{}
	dimensionRoleCounts := map[string]int{}
	for author := 1; author <= 6; author++ {
		var assignment v9AuthorAssignment
		v9DecodePacketStrict(t, filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), &assignment)
		validateV9AuthorAssignment(t, author, assignment, seenIDs, seenSources, seenPaths, seenTriples, safetyCounts, dimensionRoleCounts)
	}
	if len(seenIDs) != 48 || len(seenSources) != 48 || len(seenPaths) != 48 || len(seenTriples) != 48 {
		t.Fatal("V9 assignments do not cover exactly 48 unique identities, sources, paths, and role/domain/role triples")
	}
	for number := 1; number <= 48; number++ {
		if !seenIDs[fmt.Sprintf("v9_case_%03d", number)] {
			t.Fatalf("V9 assignment case %03d is missing", number)
		}
	}
	for _, category := range v9AuthorSafety {
		if safetyCounts[category] != 12 {
			t.Fatalf("V9 safety allocation for %s = %d, want 12", category, safetyCounts[category])
		}
	}
	for _, role := range []string{"discovery", "held_out"} {
		for _, domain := range v9AuthorDomains {
			if got := dimensionRoleCounts[role+"\x00domain:"+domain]; got != 4 {
				t.Fatalf("V9 %s domain %s count = %d, want 4", role, domain, got)
			}
		}
		for _, circuitRole := range v9AuthorCircuitRoles {
			if got := dimensionRoleCounts[role+"\x00circuit_role:"+circuitRole]; got != 4 {
				t.Fatalf("V9 %s circuit role %s count = %d, want 4", role, circuitRole, got)
			}
		}
	}
}

func TestVersionNineAuthorVisibleFilesContainNoConcreteIDs(t *testing.T) {
	root := filepath.Join(v9PacketContractDirectory(t), v9AuthorPacketRoot)
	concreteIdentity := regexp.MustCompile(`v9_(case|source)_[0-9]{3}`)
	for _, name := range []string{"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json"} {
		if match := concreteIdentity.Find(v9PacketReadFile(t, filepath.Join(root, name))); match != nil {
			t.Fatalf("author-visible common file %s leaks concrete identity %q", name, match)
		}
	}
}

func TestVersionNineAuthorPacketRootManifest(t *testing.T) {
	v9VerifyPacketChecksumManifest(t, v9PacketContractDirectory(t), "V9_AUTHOR_PACKET.sha256", []string{
		"v9-authoring-packet/PACKET_SET.sha256",
		"V9_AUTHOR_PACKET_PRISM_REVIEW.md",
		"v9_author_packet_test.go",
	})
}

func TestVersionNineAuthorPacketGeneratorReproducesFrozenBytes(t *testing.T) {
	directory := v9PacketContractDirectory(t)
	wantRoot := filepath.Join(directory, v9AuthorPacketRoot)
	gotRoot := filepath.Join(t.TempDir(), v9AuthorPacketRoot)
	if err := writeV9AuthorPacket(directory, gotRoot); err != nil {
		t.Fatal(err)
	}
	wantFiles := append(v9PacketSetPaths(), v9AuthorPacketSetManifest)
	for _, relative := range wantFiles {
		want, err := os.ReadFile(filepath.Join(wantRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(gotRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("V9 packet generator differs for %s", relative)
		}
	}
}

func validateV9AuthorAssignment(t *testing.T, author int, assignment v9AuthorAssignment, seenIDs, seenSources, seenPaths, seenTriples map[string]bool, safetyCounts, dimensionRoleCounts map[string]int) {
	t.Helper()
	if assignment.Schema != v9AuthorPacketSchema || assignment.Version != 9 || assignment.AuthorSlot != fmt.Sprintf("author_%d", author) || len(assignment.Entries) != 8 {
		t.Fatalf("V9 author_%d assignment metadata is invalid", author)
	}
	classCounts, analysisSet, safetyPerAuthor := map[string]int{}, map[string]bool{}, map[string]int{}
	multiByRole, offNominalByRole := map[string]int{}, map[string]int{}
	for index, entry := range assignment.Entries {
		wantRole := "discovery"
		if index >= 4 {
			wantRole = "held_out"
		}
		number := v9AssignmentNumber(t, entry.ID)
		if (number > 24) != (wantRole == "held_out") || entry.Role != wantRole ||
			entry.SourceID != fmt.Sprintf("v9_source_%03d", number) ||
			entry.RequirementFile != fmt.Sprintf("%s/request_%03d.json", wantRole, number) ||
			!slices.Contains(v9AuthorDomains, entry.Domain) || !slices.Contains(v9AuthorCircuitRoles, entry.CircuitRole) ||
			!slices.Contains(v9AuthorSafety, entry.SafetyImpact) ||
			(entry.PrimaryClass != "static" && entry.PrimaryClass != "dynamic") ||
			(entry.OutputMultiplicity != "single" && entry.OutputMultiplicity != "multiple") {
			t.Fatalf("V9 author_%d entry %d is invalid: %#v", author, index, entry)
		}
		if entry.PrimaryClass == "static" && !slices.Contains(v9StaticAnalyses, entry.RequiredPrimaryAnalysis) ||
			entry.PrimaryClass == "dynamic" && !slices.Contains(v9DynamicAnalyses, entry.RequiredPrimaryAnalysis) {
			t.Fatalf("V9 author_%d entry %d analysis/class mismatch", author, index)
		}
		triple := entry.Role + "\x00" + entry.Domain + "\x00" + entry.CircuitRole
		if seenIDs[entry.ID] || seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] || seenTriples[triple] {
			t.Fatal("V9 assignments are not disjoint")
		}
		seenIDs[entry.ID], seenSources[entry.SourceID], seenPaths[entry.RequirementFile], seenTriples[triple] = true, true, true, true
		classCounts[entry.PrimaryClass]++
		analysisSet[entry.RequiredPrimaryAnalysis] = true
		safetyCounts[entry.SafetyImpact]++
		safetyPerAuthor[entry.SafetyImpact]++
		dimensionRoleCounts[entry.Role+"\x00domain:"+entry.Domain]++
		dimensionRoleCounts[entry.Role+"\x00circuit_role:"+entry.CircuitRole]++
		if entry.OutputMultiplicity == "multiple" {
			multiByRole[entry.Role]++
		}
		if entry.RequireOffNominal {
			offNominalByRole[entry.Role]++
		}
	}
	if classCounts["static"] != 4 || classCounts["dynamic"] != 4 || len(analysisSet) != 8 ||
		multiByRole["discovery"] != 1 || multiByRole["held_out"] != 1 ||
		offNominalByRole["discovery"] != 1 || offNominalByRole["held_out"] != 1 {
		t.Fatalf("V9 author_%d diversity allocation is invalid", author)
	}
	for _, category := range v9AuthorSafety {
		if safetyPerAuthor[category] != 2 {
			t.Fatalf("V9 author_%d safety %s count = %d, want 2", author, category, safetyPerAuthor[category])
		}
	}
}

func writeV9AuthorPacket(directory, root string) error {
	writeFile := func(path string, data []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	}
	publicSource, err := os.ReadFile(filepath.Join(directory, "v8-authoring-packet", "PUBLIC_REQUIREMENT_CONTRACT.md"))
	if err != nil {
		return err
	}
	corpusRules, err := os.ReadFile(filepath.Join(directory, "V9_CORPUS_RULES.md"))
	if err != nil {
		return err
	}
	publicContract := bytes.ReplaceAll(publicSource, []byte("V8"), []byte("V9"))
	publicContract = bytes.ReplaceAll(publicContract, []byte("v8_"), []byte("v9_"))
	binding := v9PacketBinding{
		Schema: "kicadai.closed-loop-open-set-authoring-packet-binding.v9", Version: 9,
		ContractFreezeCommit: v9AuthorContractCommit, ContractManifest: "V9_CONTRACT.sha256", ContractManifestSHA256: v9AuthorContractHash,
		PredecessorRetirementSHA256: v9AuthorPredecessorHash, RetiredVersions: []int{1, 2, 3, 4, 5, 6, 7, 8}, RetiredHeldOutReuse: "prohibited",
	}
	bindingBytes, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	bindingBytes = append(bindingBytes, '\n')
	common := map[string][]byte{
		"README.md":                      []byte(v9AuthorREADME),
		"README_CUSTODIAN.md":            []byte(v9CustodianREADME),
		"PUBLIC_REQUIREMENT_CONTRACT.md": publicContract,
		"CORPUS_RULES.md":                corpusRules,
		"AUTHORSHIP_TEMPLATE.json":       []byte(v9AuthorshipTemplate),
		"CONTRACT_BINDING.json":          bindingBytes,
	}
	for path, data := range common {
		if err := writeFile(filepath.Join(root, path), data); err != nil {
			return err
		}
	}
	for author := 1; author <= 6; author++ {
		assignment := buildV9AuthorAssignment(author)
		data, marshalErr := json.MarshalIndent(assignment, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		if err := writeFile(filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), append(data, '\n')); err != nil {
			return err
		}
	}
	for author := 1; author <= 6; author++ {
		data, manifestErr := buildV9ChecksumManifest(root, v9PerAuthorPaths(author))
		if manifestErr != nil {
			return manifestErr
		}
		if err := writeFile(filepath.Join(root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author)), data); err != nil {
			return err
		}
	}
	packetSet, err := buildV9ChecksumManifest(root, v9PacketSetPaths())
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, v9AuthorPacketSetManifest), packetSet)
}

func buildV9AuthorAssignment(author int) v9AuthorAssignment {
	entries := make([]v9AssignmentEntry, 0, 8)
	for position := 0; position < 8; position++ {
		heldOut := position >= 4
		local := position % 4
		// Each partition intentionally has its own complete 0..23 index.
		// The held-out +2 rotation changes domain exposure without skipping
		// authors; the frozen verifier proves four occurrences per dimension
		// and partition plus unique role/domain/circuit-role triples.
		partitionIndex := 4*(author-1) + local
		number := partitionIndex + 1
		role := "discovery"
		if heldOut {
			number += 24
			role = "held_out"
		}
		domainIndex := partitionIndex % len(v9AuthorDomains)
		if heldOut {
			domainIndex = (domainIndex + 2) % len(v9AuthorDomains)
		}
		circuitRoleIndex := (domainIndex + partitionIndex/6) % len(v9AuthorCircuitRoles)
		if heldOut {
			circuitRoleIndex = (circuitRoleIndex + 3) % len(v9AuthorCircuitRoles)
		}
		primaryClass := "dynamic"
		if (position%2 == 0) != heldOut {
			primaryClass = "static"
		}
		analysis := v9DynamicAnalyses[(author-1+local)%len(v9DynamicAnalyses)]
		if primaryClass == "static" {
			analysis = v9StaticAnalyses[(author-1+local)%len(v9StaticAnalyses)]
		}
		outputMultiplicity := "single"
		if !heldOut && local == 0 || heldOut && local == 1 {
			outputMultiplicity = "multiple"
		}
		requireOffNominal := !heldOut && local == 2 || heldOut && local == 3
		entries = append(entries, v9AssignmentEntry{
			ID: fmt.Sprintf("v9_case_%03d", number), Role: role, Domain: v9AuthorDomains[domainIndex], CircuitRole: v9AuthorCircuitRoles[circuitRoleIndex],
			SafetyImpact: v9AuthorSafety[(position+2*(author-1))%len(v9AuthorSafety)], PrimaryClass: primaryClass, RequiredPrimaryAnalysis: analysis,
			OutputMultiplicity: outputMultiplicity,
			RequireOffNominal:  requireOffNominal, SourceID: fmt.Sprintf("v9_source_%03d", number),
			RequirementFile: fmt.Sprintf("%s/request_%03d.json", role, number),
		})
	}
	return v9AuthorAssignment{Schema: v9AuthorPacketSchema, Version: 9, AuthorSlot: fmt.Sprintf("author_%d", author), Entries: entries}
}

func buildV9ChecksumManifest(root string, paths []string) ([]byte, error) {
	var result strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		fmt.Fprintf(&result, "%s  %s\n", hex.EncodeToString(digest[:]), path)
	}
	return []byte(result.String()), nil
}

func v9PerAuthorPaths(author int) []string {
	return []string{
		"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json",
		fmt.Sprintf("assignments/author_%d.json", author),
	}
}

func v9PacketSetPaths() []string {
	paths := []string{"README_CUSTODIAN.md", "README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json"}
	for author := 1; author <= 6; author++ {
		paths = append(paths, fmt.Sprintf("assignments/author_%d.json", author))
	}
	for author := 1; author <= 6; author++ {
		paths = append(paths, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author))
	}
	return paths
}

func v9AssignmentNumber(t *testing.T, id string) int {
	t.Helper()
	if !strings.HasPrefix(id, "v9_case_") || len(id) != len("v9_case_000") {
		t.Fatalf("invalid V9 assignment ID %q", id)
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, "v9_case_"))
	if err != nil || number < 1 || number > 48 {
		t.Fatalf("invalid V9 assignment ID %q", id)
	}
	return number
}

// These wrappers give V9 call sites version-appropriate names while reusing
// the byte-frozen strict checksum/JSON helpers already exercised by V8.
func v9PacketContractDirectory(t *testing.T) string {
	t.Helper()
	return v8ContractDirectory(t)
}

func v9PacketFileSHA256(t *testing.T, path string) string {
	t.Helper()
	return v8PacketFileSHA256(t, path)
}

func v9VerifyPacketChecksumManifest(t *testing.T, root, name string, wantPaths []string) {
	t.Helper()
	v8VerifyPacketChecksumManifest(t, root, name, wantPaths)
}

func v9DecodePacketStrict(t *testing.T, path string, value any) {
	t.Helper()
	v8DecodePacketStrict(t, path, value)
}

func v9PacketReadFile(t *testing.T, path string) []byte {
	t.Helper()
	return v8PacketReadFile(t, path)
}

const v9AuthorREADME = `# V9 Isolated Corpus Author Packet

This packet is the complete and only input permitted for one independent V9
corpus author. It contains exactly one assignment fixing eight identities,
roles, domains, circuit roles, safety impacts, primary classes and analyses,
output multiplicity, off-nominal obligations, and returned paths.

The author must:

1. verify the supplied AUTHOR_N_PACKET.sha256 completely before assignment access;
2. verify CONTRACT_BINDING.json names the committed V9 contract freeze;
3. read PUBLIC_REQUIREMENT_CONTRACT.md and CORPUS_RULES.md completely;
4. create exactly the eight JSON files named by the sole assignment;
5. preserve every packet input byte-for-byte;
6. instantiate AUTHORSHIP_TEMPLATE.json as AUTHORSHIP.json with exactly eight ordered source hashes;
7. run no repository, KiCadAI, synthesis, simulation, classifier, feasibility, ranking, outcome, network, or cross-author tool;
8. return only AUTHORSHIP.json and the eight requirement files in the designated empty quarantine; and
9. disclose uncertainty instead of guessing.

All behavior must be independently conceived. Manifest-only v9_case_* and
v9_source_* identities must not occur inside requirements. Authors do not
create obligation, gap, frontier, exposure, or causal-path identities.

Any non-packet input invalidates the complete context and requires a fresh
replacement with entirely fresh requirements.
`

const v9CustodianREADME = `# V9 Corpus Authoring Packet Set

This directory defines six disjoint author packets and is never sent wholesale
to an author. For author_N provide only README.md,
PUBLIC_REQUIREMENT_CONTRACT.md, CORPUS_RULES.md, AUTHORSHIP_TEMPLATE.json,
CONTRACT_BINDING.json, assignments/author_N.json, and AUTHOR_N_PACKET.sha256.

Do not provide another assignment, this custodian file, PACKET_SET.sha256,
repository or conversation context, prior corpus material, outcomes,
diagnostics, rankings, expected behavior, or implementation information.

Each author returns eight requirements plus AUTHORSHIP.json into a separate
verified-empty quarantine. Authors never share contexts or see other output.
Bounded waves are permitted only when later waves receive no earlier output.
`

const v9AuthorshipTemplate = `{
  "schema": "kicadai.closed-loop-open-set-authorship.v9",
  "version": 9,
  "author_context_identity": "[record opaque context identity]",
  "author_slot": "[record exact assignment author_slot]",
  "authoring_tool_model_version": "[record exact tool/model identity and version]",
  "authoring_started_utc": "[record RFC3339 timestamp]",
  "authoring_ended_utc": "[record RFC3339 timestamp]",
  "per_author_packet_manifest": "[record exact AUTHOR_N_PACKET.sha256 filename]",
  "per_author_packet_sha256": "[record lowercase hexadecimal SHA-256 of that manifest]",
  "contract_binding_sha256": "[record lowercase hexadecimal SHA-256 of CONTRACT_BINDING.json]",
  "assignment_sha256": "[record lowercase hexadecimal SHA-256 of the supplied assignment]",
  "returned_bundle_root": "[record quarantine bundle name]",
  "requirement_source_sha256": [
    {"path":"[record assigned requirement path 1]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 1]"},
    {"path":"[record assigned requirement path 2]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 2]"},
    {"path":"[record assigned requirement path 3]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 3]"},
    {"path":"[record assigned requirement path 4]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 4]"},
    {"path":"[record assigned requirement path 5]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 5]"},
    {"path":"[record assigned requirement path 6]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 6]"},
    {"path":"[record assigned requirement path 7]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 7]"},
    {"path":"[record assigned requirement path 8]","sha256":"[record lowercase hexadecimal SHA-256 of exact returned JSON bytes 8]"}
  ],
  "uncertainties": [],
  "attestations": {
    "packet_only_input": true,
    "contract_bound_before_authoring": true,
    "no_repository_or_prior_corpus_access": true,
    "no_cross_author_assignment_or_content_access": true,
    "independently_conceived_behavior_only_requirements": true,
    "no_synthesis_simulation_classification_ranking_or_feasibility": true,
    "fixed_discovery_held_out_membership": true,
    "no_implementation_or_expected_outcome_prescription": true,
    "no_obligation_anchor_gap_exposure_or_causal_path_authorship": true,
    "no_post_evaluation_inspection_or_modification": true,
    "all_uncertainties_disclosed": true
  }
}
`
