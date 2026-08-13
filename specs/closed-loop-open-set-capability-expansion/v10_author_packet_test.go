package closedloopopensetcontract

import (
	"bufio"
	"bytes"
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

	"kicadai/internal/atomicdir"
	"kicadai/internal/corpusassignment"
)

const (
	v10AuthorPacketUpdateEnv   = "UPDATE_CLOSED_LOOP_V10_AUTHOR_PACKET"
	v10AuthorContractCommit    = "2bc33a1857feb88011d53a0f5f405569468ae2d1"
	v10AuthorContractHash      = "9a04332c0483b4b42285932d3761fbf2356854994fcf2177a835cedd4af6cedf"
	v10AuthorCorpusRulesHash   = "66b56cc334bca8827d83ab515a5c6c2af3fb053da6176ff74b1a50a5d21a82d2"
	v10AuthorPredecessorHash   = "e80a9bcc3098719cede0f1b2c8ee3f946040c2b683e172c7b1451e1ee0940826"
	v10AuthorPacketSchema      = "kicadai.closed-loop-open-set-author-assignment.v10"
	v10AuthorPacketRoot        = "v10-authoring-packet"
	v10AuthorPacketSetManifest = "PACKET_SET.sha256"
	v10PacketArtifactMaxBytes  = 32 << 20
)

var (
	v10AuthorDomains = []string{
		"analog_signal_path", "power_energy_conversion", "digital_control",
		"mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity",
	}
	v10AuthorCircuitRoles = []string{
		"source_bias", "amplification_conditioning", "conversion_regulation",
		"sensing_measurement", "interface_control", "protection_supervision",
	}
	v10AuthorSafety    = []string{"non_safety", "review_required", "safety_relevant", "safety_critical"}
	v10StaticAnalyses  = []string{"dc_operating_point", "dc_sweep", "thermal", "electrothermal"}
	v10DynamicAnalyses = []string{"transient", "ac_sweep", "startup", "shutdown", "stability", "distortion", "noise"}
)

type v10PacketBinding struct {
	Schema                      string `json:"schema"`
	Version                     int    `json:"version"`
	ContractFreezeCommit        string `json:"contract_freeze_commit"`
	ContractManifest            string `json:"contract_manifest"`
	ContractManifestSHA256      string `json:"contract_manifest_sha256"`
	PredecessorRetirementSHA256 string `json:"predecessor_retirement_sha256"`
	RetiredVersions             []int  `json:"retired_versions"`
	RetiredHeldOutReuse         string `json:"retired_held_out_reuse"`
}

type v10AuthorAssignment struct {
	Schema     string               `json:"schema"`
	Version    int                  `json:"version"`
	AuthorSlot string               `json:"author_slot"`
	Entries    []v10AssignmentEntry `json:"entries"`
}

type v10AssignmentEntry struct {
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

func TestUpdateVersionTenAuthorPacket(t *testing.T) {
	if os.Getenv(v10AuthorPacketUpdateEnv) != "1" {
		t.Skip("set " + v10AuthorPacketUpdateEnv + "=1 to create the V10 author packet")
	}
	directory := v10PacketDirectory(t)
	root := filepath.Join(directory, v10AuthorPacketRoot)
	if _, err := os.Stat(root); err == nil {
		t.Fatal("V10 author packet root already exists; refusing replacement")
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect V10 author packet root: %v", err)
	}
	if got := v10PacketFileSHA256(t, filepath.Join(directory, "V10_CONTRACT.sha256")); got != v10AuthorContractHash {
		t.Fatalf("V10 contract manifest = %s, want %s", got, v10AuthorContractHash)
	}
	if got := v10PacketFileSHA256(t, filepath.Join(directory, "V10_CORPUS_RULES.md")); got != v10AuthorCorpusRulesHash {
		t.Fatalf("V10 packet corpus-rules source = %s, want %s", got, v10AuthorCorpusRulesHash)
	}
	assignments := buildV10AuthorAssignments()
	if _, err := corpusassignment.Validate(v10PreflightEntries(assignments), v10AssignmentPolicy()); err != nil {
		t.Fatalf("V10 packet candidate failed production preflight before write: %v", err)
	}
	if err := atomicdir.Publish(root, func(staging string) error {
		return writeV10AuthorPacket(directory, staging, assignments)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVersionTenAuthorPacketIsFrozenDisjointAndFeasible(t *testing.T) {
	root := filepath.Join(v10PacketDirectory(t), v10AuthorPacketRoot)
	packetPaths := v10PacketSetPaths()
	v10VerifyChecksumManifest(t, root, v10AuthorPacketSetManifest, packetPaths)
	wantFiles := append(slices.Clone(packetPaths), v10AuthorPacketSetManifest)
	slices.Sort(wantFiles)
	var actualFiles []string
	if err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
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
		t.Fatalf("V10 author packet files = %q, want %q", actualFiles, wantFiles)
	}
	for author := 1; author <= 6; author++ {
		v10VerifyChecksumManifest(t, root, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), v10PerAuthorPaths(author))
	}

	var binding v10PacketBinding
	v10DecodeStrict(t, filepath.Join(root, "CONTRACT_BINDING.json"), &binding)
	if binding.Schema != "kicadai.closed-loop-open-set-authoring-packet-binding.v10" || binding.Version != 10 ||
		binding.ContractFreezeCommit != v10AuthorContractCommit || binding.ContractManifest != "V10_CONTRACT.sha256" ||
		binding.ContractManifestSHA256 != v10AuthorContractHash || binding.PredecessorRetirementSHA256 != v10AuthorPredecessorHash ||
		!slices.Equal(binding.RetiredVersions, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}) || binding.RetiredHeldOutReuse != "prohibited" {
		t.Fatal("V10 author packet binding is invalid")
	}

	assignments := make([]v10AuthorAssignment, 0, 6)
	seenSources, seenPaths := map[string]bool{}, map[string]bool{}
	seenAnalyses := map[string]bool{}
	for author := 1; author <= 6; author++ {
		var assignment v10AuthorAssignment
		v10DecodeStrict(t, filepath.Join(root, "assignments", fmt.Sprintf("author_%d.json", author)), &assignment)
		v10ValidateAssignmentShape(t, author, assignment)
		for _, entry := range assignment.Entries {
			if seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] {
				t.Fatal("V10 assignments repeat a source identity or requirement path")
			}
			seenSources[entry.SourceID], seenPaths[entry.RequirementFile] = true, true
			seenAnalyses[entry.RequiredPrimaryAnalysis] = true
		}
		assignments = append(assignments, assignment)
	}
	if len(seenSources) != 48 || len(seenPaths) != 48 {
		t.Fatal("V10 assignments do not cover exactly 48 source identities and requirement paths")
	}
	for _, analysis := range append(slices.Clone(v10StaticAnalyses), v10DynamicAnalyses...) {
		if !seenAnalyses[analysis] {
			t.Fatalf("V10 assignments omit required primary analysis %q", analysis)
		}
	}
	report, err := corpusassignment.Validate(v10PreflightEntries(assignments), v10AssignmentPolicy())
	if err != nil {
		t.Fatalf("frozen V10 assignments fail independent production preflight: %v; report=%+v", err, report)
	}
	if report.EntryCount != 48 || len(report.Partitions) != 2 {
		t.Fatalf("unexpected V10 assignment preflight report: %+v", report)
	}
}

func TestVersionTenAuthorVisibleFilesContainNoConcreteIDs(t *testing.T) {
	root := filepath.Join(v10PacketDirectory(t), v10AuthorPacketRoot)
	concreteIdentity := regexp.MustCompile(`v10_(case|source)_[0-9]{3}`)
	for _, name := range []string{"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json"} {
		if match := concreteIdentity.Find(v10PacketReadFile(t, filepath.Join(root, name))); match != nil {
			t.Fatalf("author-visible common file %s leaks concrete identity %q", name, match)
		}
	}
}

func TestVersionTenAuthorPacketGeneratorReproducesFrozenBytes(t *testing.T) {
	directory := v10PacketDirectory(t)
	wantRoot := filepath.Join(directory, v10AuthorPacketRoot)
	gotRoot := filepath.Join(t.TempDir(), v10AuthorPacketRoot)
	assignments := buildV10AuthorAssignments()
	if _, err := corpusassignment.Validate(v10PreflightEntries(assignments), v10AssignmentPolicy()); err != nil {
		t.Fatalf("V10 reproduced packet failed pre-write production preflight: %v", err)
	}
	if err := writeV10AuthorPacket(directory, gotRoot, assignments); err != nil {
		t.Fatal(err)
	}
	for _, relative := range append(v10PacketSetPaths(), v10AuthorPacketSetManifest) {
		want := v10PacketReadFile(t, filepath.Join(wantRoot, filepath.FromSlash(relative)))
		got := v10PacketReadFile(t, filepath.Join(gotRoot, filepath.FromSlash(relative)))
		if !bytes.Equal(got, want) {
			t.Fatalf("V10 packet generator differs for %s", relative)
		}
	}
}

func TestVersionTenAuthorPacketRootManifest(t *testing.T) {
	v10VerifyChecksumManifest(t, v10PacketDirectory(t), "V10_AUTHOR_PACKET.sha256", []string{
		"v10-authoring-packet/PACKET_SET.sha256",
		"V10_PUBLIC_REQUIREMENT_CONTRACT.md",
		"V10_AUTHORSHIP_TEMPLATE.json",
		"V10_CONTRACT.sha256",
		"V10_CORPUS_RULES.md",
		"V10_AUTHOR_PACKET_PRISM_REVIEW.md",
	})
}

func buildV10AuthorAssignments() []v10AuthorAssignment {
	assignments := make([]v10AuthorAssignment, 0, 6)
	for author := 1; author <= 6; author++ {
		entries := make([]v10AssignmentEntry, 0, 8)
		for position := 0; position < 8; position++ {
			heldOut := position >= 4
			local := position % 4
			partitionIndex := 4*(author-1) + local
			number := partitionIndex + 1
			partition := "discovery"
			if heldOut {
				number += 24
				partition = "held_out"
			}
			domainIndex := partitionIndex % len(v10AuthorDomains)
			if heldOut {
				domainIndex = (domainIndex + 2) % len(v10AuthorDomains)
			}
			roleIndex := (domainIndex + partitionIndex/6) % len(v10AuthorCircuitRoles)
			if heldOut {
				roleIndex = (roleIndex + 3) % len(v10AuthorCircuitRoles)
			}
			primaryClass := "dynamic"
			if (position%2 == 0) != heldOut {
				primaryClass = "static"
			}
			analysis := v10DynamicAnalyses[(author-1+local)%len(v10DynamicAnalyses)]
			if primaryClass == "static" {
				analysis = v10StaticAnalyses[(author-1+local)%len(v10StaticAnalyses)]
			}
			multiplicity := "single"
			if !heldOut && local == 0 || heldOut && local == 1 {
				multiplicity = "multiple"
			}
			entries = append(entries, v10AssignmentEntry{
				ID: fmt.Sprintf("v10_case_%03d", number), Role: partition,
				Domain: v10AuthorDomains[domainIndex], CircuitRole: v10AuthorCircuitRoles[roleIndex],
				SafetyImpact: v10AuthorSafety[(local+author-1)%len(v10AuthorSafety)],
				PrimaryClass: primaryClass, RequiredPrimaryAnalysis: analysis, OutputMultiplicity: multiplicity,
				RequireOffNominal: !heldOut && local == 2 || heldOut && local == 3,
				SourceID:          fmt.Sprintf("v10_source_%03d", number), RequirementFile: fmt.Sprintf("%s/request_%03d.json", partition, number),
			})
		}
		assignments = append(assignments, v10AuthorAssignment{Schema: v10AuthorPacketSchema, Version: 10, AuthorSlot: fmt.Sprintf("author_%d", author), Entries: entries})
	}
	return assignments
}

func v10AssignmentPolicy() corpusassignment.Policy {
	authors := make([]string, 6)
	for index := range authors {
		authors[index] = fmt.Sprintf("author_%d", index+1)
	}
	return corpusassignment.Policy{
		Authors: authors, Partitions: []string{"discovery", "held_out"}, Domains: slices.Clone(v10AuthorDomains),
		CircuitRoles: slices.Clone(v10AuthorCircuitRoles), SafetyImpacts: slices.Clone(v10AuthorSafety),
		HighSafetyImpacts: []string{"safety_relevant", "safety_critical"}, CasesPerAuthor: 8, CasesPerPartition: 24,
		DimensionCountPerPartition: 4, SafetyCountPerPartition: 6, MinimumStaticPerAuthor: 4, MinimumDynamicPerAuthor: 4,
		MinimumMultiOutputPerPartition: 6, MinimumOffNominalPerAuthor: 2,
		RequireHighSafetyDomains: true, RequireHighSafetyCircuitRoles: true,
	}
}

func v10PreflightEntries(assignments []v10AuthorAssignment) []corpusassignment.Entry {
	entries := make([]corpusassignment.Entry, 0, 48)
	for _, assignment := range assignments {
		for _, entry := range assignment.Entries {
			entries = append(entries, corpusassignment.Entry{
				ID: entry.ID, Author: assignment.AuthorSlot, Partition: entry.Role, Domain: entry.Domain, CircuitRole: entry.CircuitRole,
				SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass, OutputMultiplicity: entry.OutputMultiplicity,
				RequireOffNominal: entry.RequireOffNominal,
			})
		}
	}
	return entries
}

func v10ValidateAssignmentShape(t *testing.T, author int, assignment v10AuthorAssignment) {
	t.Helper()
	if assignment.Schema != v10AuthorPacketSchema || assignment.Version != 10 || assignment.AuthorSlot != fmt.Sprintf("author_%d", author) || len(assignment.Entries) != 8 {
		t.Fatalf("V10 author_%d assignment metadata is invalid", author)
	}
	seenSources, seenPaths := map[string]bool{}, map[string]bool{}
	classCounts, analysisSet, safetyCounts := map[string]int{}, map[string]bool{}, map[string]int{}
	multiByRole, offNominalByRole := map[string]int{}, map[string]int{}
	for index, entry := range assignment.Entries {
		wantRole := "discovery"
		if index >= 4 {
			wantRole = "held_out"
		}
		number := v10AssignmentNumber(t, entry.ID)
		if (number > 24) != (wantRole == "held_out") || entry.Role != wantRole ||
			entry.SourceID != fmt.Sprintf("v10_source_%03d", number) || entry.RequirementFile != fmt.Sprintf("%s/request_%03d.json", wantRole, number) ||
			!slices.Contains(v10AuthorDomains, entry.Domain) || !slices.Contains(v10AuthorCircuitRoles, entry.CircuitRole) || !slices.Contains(v10AuthorSafety, entry.SafetyImpact) {
			t.Fatalf("V10 author_%d entry %d is invalid: %#v", author, index, entry)
		}
		if entry.PrimaryClass == "static" && !slices.Contains(v10StaticAnalyses, entry.RequiredPrimaryAnalysis) ||
			entry.PrimaryClass == "dynamic" && !slices.Contains(v10DynamicAnalyses, entry.RequiredPrimaryAnalysis) {
			t.Fatalf("V10 author_%d entry %d analysis/class mismatch", author, index)
		}
		if seenSources[entry.SourceID] || seenPaths[entry.RequirementFile] {
			t.Fatalf("V10 author_%d repeats a source or path", author)
		}
		seenSources[entry.SourceID], seenPaths[entry.RequirementFile] = true, true
		classCounts[entry.PrimaryClass]++
		analysisSet[entry.RequiredPrimaryAnalysis] = true
		safetyCounts[entry.SafetyImpact]++
		if entry.OutputMultiplicity == "multiple" {
			multiByRole[entry.Role]++
		}
		if entry.RequireOffNominal {
			offNominalByRole[entry.Role]++
		}
	}
	if classCounts["static"] != 4 || classCounts["dynamic"] != 4 || len(analysisSet) != 8 ||
		multiByRole["discovery"] != 1 || multiByRole["held_out"] != 1 || offNominalByRole["discovery"] != 1 || offNominalByRole["held_out"] != 1 {
		t.Fatalf("V10 author_%d diversity allocation is invalid", author)
	}
	for _, safety := range v10AuthorSafety {
		if safetyCounts[safety] != 2 {
			t.Fatalf("V10 author_%d safety %s count = %d, want 2", author, safety, safetyCounts[safety])
		}
	}
}

func writeV10AuthorPacket(directory, root string, assignments []v10AuthorAssignment) error {
	writeFile := func(relative string, data []byte) error {
		filePath := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filePath, data, 0o644)
	}
	publicContract, err := os.ReadFile(filepath.Join(directory, "V10_PUBLIC_REQUIREMENT_CONTRACT.md"))
	if err != nil {
		return err
	}
	authorship, err := os.ReadFile(filepath.Join(directory, "V10_AUTHORSHIP_TEMPLATE.json"))
	if err != nil {
		return err
	}
	corpusRules, err := os.ReadFile(filepath.Join(directory, "V10_CORPUS_RULES.md"))
	if err != nil {
		return err
	}
	binding := v10PacketBinding{Schema: "kicadai.closed-loop-open-set-authoring-packet-binding.v10", Version: 10,
		ContractFreezeCommit: v10AuthorContractCommit, ContractManifest: "V10_CONTRACT.sha256", ContractManifestSHA256: v10AuthorContractHash,
		PredecessorRetirementSHA256: v10AuthorPredecessorHash, RetiredVersions: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, RetiredHeldOutReuse: "prohibited"}
	bindingBytes, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	common := map[string][]byte{
		"README.md": []byte(v10AuthorREADME), "README_CUSTODIAN.md": []byte(v10CustodianREADME),
		"PUBLIC_REQUIREMENT_CONTRACT.md": publicContract, "CORPUS_RULES.md": corpusRules,
		"AUTHORSHIP_TEMPLATE.json": authorship, "CONTRACT_BINDING.json": append(bindingBytes, '\n'),
	}
	for relative, data := range common {
		if err := writeFile(relative, data); err != nil {
			return err
		}
	}
	for _, assignment := range assignments {
		data, err := json.MarshalIndent(assignment, "", "  ")
		if err != nil {
			return err
		}
		authorNumber := strings.TrimPrefix(assignment.AuthorSlot, "author_")
		if err := writeFile("assignments/author_"+authorNumber+".json", append(data, '\n')); err != nil {
			return err
		}
	}
	for author := 1; author <= 6; author++ {
		manifest, err := v10BuildChecksumManifest(root, v10PerAuthorPaths(author))
		if err != nil {
			return err
		}
		if err := writeFile(fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author), manifest); err != nil {
			return err
		}
	}
	packetSet, err := v10BuildChecksumManifest(root, v10PacketSetPaths())
	if err != nil {
		return err
	}
	return writeFile(v10AuthorPacketSetManifest, packetSet)
}

func v10BuildChecksumManifest(root string, paths []string) ([]byte, error) {
	var result strings.Builder
	for _, relative := range paths {
		canonical := filepath.ToSlash(relative)
		digest, err := v10ChecksumFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&result, "%s  %s\n", digest, canonical)
	}
	return []byte(result.String()), nil
}

func v10ChecksumFile(filePath string) (string, error) {
	info, err := os.Lstat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > v10PacketArtifactMaxBytes {
		return "", fmt.Errorf("V10 packet source is not a bounded regular file: %s", filePath)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		return "", fmt.Errorf("V10 packet source changed while opening: %s", filePath)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, io.LimitReader(file, v10PacketArtifactMaxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written != info.Size() || written > v10PacketArtifactMaxBytes {
		return "", fmt.Errorf("V10 packet source size changed while hashing: %s", filePath)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func v10PerAuthorPaths(author int) []string {
	return []string{"README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json", fmt.Sprintf("assignments/author_%d.json", author)}
}

func v10PacketSetPaths() []string {
	paths := []string{"README_CUSTODIAN.md", "README.md", "PUBLIC_REQUIREMENT_CONTRACT.md", "CORPUS_RULES.md", "AUTHORSHIP_TEMPLATE.json", "CONTRACT_BINDING.json"}
	for author := 1; author <= 6; author++ {
		paths = append(paths, fmt.Sprintf("assignments/author_%d.json", author))
	}
	for author := 1; author <= 6; author++ {
		paths = append(paths, fmt.Sprintf("AUTHOR_%d_PACKET.sha256", author))
	}
	return paths
}

func v10AssignmentNumber(t *testing.T, id string) int {
	t.Helper()
	if !strings.HasPrefix(id, "v10_case_") || len(id) != len("v10_case_000") {
		t.Fatalf("invalid V10 assignment ID %q", id)
	}
	number, err := strconv.Atoi(strings.TrimPrefix(id, "v10_case_"))
	if err != nil || number < 1 || number > 48 {
		t.Fatalf("invalid V10 assignment ID %q", id)
	}
	return number
}

func v10VerifyChecksumManifest(t *testing.T, root, name string, wantPaths []string) {
	t.Helper()
	manifestPath := filepath.Join(root, name)
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > 1<<20 {
		t.Fatalf("%s is not a bounded regular manifest", name)
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		t.Fatalf("%s changed while opening", name)
	}
	scanner := bufio.NewScanner(io.LimitReader(file, (1<<20)+1))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	index := 0
	for scanner.Scan() {
		// bufio.ScanLines removes an optional trailing CR, so canonical LF and
		// externally transported CRLF manifests verify identically.
		line := scanner.Text()
		if index >= len(wantPaths) {
			t.Fatalf("%s contains more than %d entries", name, len(wantPaths))
		}
		digest, relative, ok := splitV10PacketChecksumLine(line)
		if !ok || relative != wantPaths[index] {
			t.Fatalf("%s line %d is invalid", name, index+1)
		}
		// Target artifacts are hashed as bounded streams; they are never loaded
		// wholesale by manifest verification.
		if got := v10PacketFileSHA256(t, filepath.Join(root, filepath.FromSlash(relative))); got != digest {
			t.Fatalf("%s hash for %s = %s, want %s", name, relative, got, digest)
		}
		index++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(wantPaths) {
		t.Fatalf("%s contains %d entries, want %d", name, index, len(wantPaths))
	}
}

func v10DecodeStrict(t *testing.T, filePath string, value any) {
	t.Helper()
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		t.Fatalf("%s contains trailing JSON", filePath)
	}
}

func v10PacketDirectory(t *testing.T) string {
	t.Helper()
	_, filePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V10 author packet directory")
	}
	return filepath.Dir(filePath)
}

func v10PacketReadFile(t *testing.T, filePath string) []byte {
	t.Helper()
	info, err := os.Lstat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > v10PacketArtifactMaxBytes {
		t.Fatalf("V10 packet artifact is not a bounded regular file: %s", filePath)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) != info.Size() {
		t.Fatalf("V10 packet artifact changed while reading: %s", filePath)
	}
	return data
}

func v10PacketFileSHA256(t *testing.T, filePath string) string {
	t.Helper()
	digest, err := v10ChecksumFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func splitV10PacketChecksumLine(line string) (digest, relative string, ok bool) {
	const delimiter = "  "
	if len(line) <= sha256.Size*2+len(delimiter) || line[sha256.Size*2:sha256.Size*2+len(delimiter)] != delimiter {
		return "", "", false
	}
	digest = line[:sha256.Size*2]
	if _, err := hex.DecodeString(digest); err != nil || strings.ToLower(digest) != digest {
		return "", "", false
	}
	relative = line[sha256.Size*2+len(delimiter):]
	return digest, relative, relative != ""
}

const v10AuthorREADME = `# V10 Isolated Corpus Author Packet

This packet is the complete and only input permitted for one independent V10
corpus author. It contains exactly one assignment fixing eight behavior-only
requirement identities and obligations.

The author must verify the supplied checksum manifest before assignment access,
read every supplied contract file completely, create exactly the eight assigned
JSON files, preserve packet inputs byte-for-byte, instantiate AUTHORSHIP.json
with ordered full-byte hashes, run no repository/synthesis/simulation/outcome or
cross-author tool, return only assigned requirements plus AUTHORSHIP.json, and
disclose uncertainty instead of guessing.

All behavior must be independently conceived. Manifest-only v10_case_* and
v10_source_* identities must not occur inside requirements. Any non-packet input
invalidates the complete context and requires a fresh replacement.
`

const v10CustodianREADME = `# V10 Corpus Authoring Packet Set

This directory defines six disjoint author packets and is never sent wholesale
to an author. Provide one author only the five common author files, its sole
assignment, and its AUTHOR_N_PACKET.sha256. Never provide another assignment,
this custodian file, packet-set manifest, repository/conversation context, prior
corpus material, outcomes, diagnostics, rankings, or implementation details.
`
