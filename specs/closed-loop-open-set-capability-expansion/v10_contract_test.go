package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestVersionTenStartsFromUnopenedV9PrepublicationRetirement(t *testing.T) {
	directory := v10ContractDirectory(t)
	data := v10ReadFile(t, filepath.Join(directory, "V9_PREPUBLICATION_RETIREMENT.json"))
	var retirement struct {
		Schema                    string `json:"schema"`
		Version                   int    `json:"version"`
		Reason                    string `json:"reason"`
		TerminalState             string `json:"terminal_state"`
		CorpusPublished           bool   `json:"corpus_published"`
		BaselineStarted           bool   `json:"baseline_started"`
		HeldOutSourceKeyCreated   bool   `json:"held_out_source_key_created"`
		HeldOutSourceOpened       bool   `json:"held_out_source_opened"`
		HeldOutBaselineKeyCreated bool   `json:"held_out_baseline_key_created"`
		HeldOutBaselineOpened     bool   `json:"held_out_baseline_opened"`
	}
	if err := json.Unmarshal(data, &retirement); err != nil {
		t.Fatal(err)
	}
	if retirement.Schema != "kicadai.closed-loop-open-set-prepublication-retirement.v9" || retirement.Version != 9 ||
		retirement.Reason != "frozen_assignment_validator_inconsistent" || retirement.TerminalState != "permanently_retired" ||
		retirement.CorpusPublished || retirement.BaselineStarted || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineOpened {
		t.Fatalf("V10 predecessor boundary is invalid: %+v", retirement)
	}
}

func TestVersionTenContractFreezesPreauthorFeasibilityAndOpenSetLoop(t *testing.T) {
	directory := v10ContractDirectory(t)
	required := map[string][]string{
		"V10_SPEC_ADDENDUM.md": {
			"production preflight must accept the complete assignment metadata",
			"at least one assigned high-safety case for every reporting domain",
			"at least one assigned high-safety case for every circuit role",
			"must not reuse V9 authored requirement bytes",
			"complete effect-exposure selection",
		},
		"V10_PLAN.md": {
			"Run the production assignment-feasibility preflight before writing packets",
			"Dispatch fresh authors only after separate explicit authorization",
			"Create a new external 0600 V10 source key",
			"Rank by unlock/diversity first and collateral exposure/sibling burden next",
			"Never manually run GitHub Actions",
		},
		"V10_CORPUS_RULES.md": {
			"exactly 48 fresh requirements: 24 discovery and 24 held- out",
			"complete high-safety domain and circuit-role coverage",
			"All 14 gates are mandatory",
			"may not prescribe parts, values as implementation choices, topology",
			"closed-loop-open-set/v10/",
		},
		"V10_BASELINE_PROTOCOL.md": {
			"All 24 discovery cases run in manifest order twice",
			"Every pass satisfies all 14 gates",
			"all nonselected sibling path hashes",
			"non-exposed cases remain byte-identical",
			"at most two rounds, six atoms, and 18 members",
			"There is no retry",
		},
	}
	for name, clauses := range required {
		source := strings.ReplaceAll(string(v10ReadFile(t, filepath.Join(directory, name))), "\r\n", "\n")
		normalized := strings.Join(strings.Fields(source), " ")
		for _, clause := range clauses {
			if !strings.Contains(normalized, clause) {
				t.Fatalf("%s omits frozen clause %q", name, clause)
			}
		}
		for _, marker := range []string{"TODO", "TBD", "FIXME"} {
			if strings.Contains(source, marker) {
				t.Fatalf("%s contains unresolved marker %q", name, marker)
			}
		}
	}
}

func TestVersionTenContractChecksumManifest(t *testing.T) {
	directory := v10ContractDirectory(t)
	wantPaths := []string{
		"V10_SPEC_ADDENDUM.md",
		"V10_PLAN.md",
		"V10_CORPUS_RULES.md",
		"V10_BASELINE_PROTOCOL.md",
		"V10_PRISM_REVIEW.md",
		"v10_contract_test.go",
		"V9_PREPUBLICATION_RETIREMENT.json",
		"../../internal/corpusassignment/preflight.go",
		"../../internal/corpusassignment/preflight_test.go",
	}
	file, err := os.Open(filepath.Join(directory, "V10_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var actualPaths []string
	scanner := bufio.NewScanner(io.LimitReader(file, 1<<20))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		digest, relative, ok := splitChecksumLineV10(line)
		if !ok {
			t.Fatalf("invalid V10 contract checksum line %q", scanner.Text())
		}
		artifact := v10ArtifactPath(t, directory, relative)
		if got := v10FileSHA256(t, artifact); got != digest {
			t.Fatalf("V10 contract checksum for %s = %s, want %s", relative, got, digest)
		}
		actualPaths = append(actualPaths, relative)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actualPaths) == 0 {
		t.Fatal("V10 contract checksum manifest is empty")
	}
	if !slices.Equal(actualPaths, wantPaths) {
		t.Fatalf("V10 contract paths = %q, want %q", actualPaths, wantPaths)
	}
}

func digestPatternForV10(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

func splitChecksumLineV10(line string) (digest, relative string, ok bool) {
	if len(line) <= sha256.Size*2 {
		return "", "", false
	}
	digest = line[:sha256.Size*2]
	remainder := line[sha256.Size*2:]
	if !digestPatternForV10(digest) || (remainder[0] != ' ' && remainder[0] != '\t') {
		return "", "", false
	}
	relative = strings.TrimLeft(remainder, " \t")
	return digest, relative, relative != ""
}

func v10ContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V10 contract directory")
	}
	return filepath.Dir(file)
}

func v10ReadFile(t *testing.T, filePath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v10FileSHA256(t *testing.T, filePath string) string {
	t.Helper()
	pathInfo, err := os.Lstat(filePath)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Size() < 0 || pathInfo.Size() > 32<<20 {
		t.Fatalf("V10 contract artifact is not a bounded regular file: %s", filePath)
	}
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil || !openInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openInfo) {
		t.Fatalf("V10 contract artifact changed while opening: %s", filePath)
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, (32<<20)+1))
	if err != nil {
		t.Fatal(err)
	}
	if written > 32<<20 {
		t.Fatalf("V10 contract artifact exceeds size limit: %s", filePath)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func v10ArtifactPath(t *testing.T, directory, relative string) string {
	t.Helper()
	if relative == "" || path.IsAbs(relative) || filepath.IsAbs(relative) || strings.Contains(relative, `\`) || path.Clean(relative) != relative {
		t.Fatalf("invalid V10 contract manifest path %q", relative)
	}
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	artifact, err := filepath.Abs(filepath.Join(directory, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	within, err := filepath.Rel(repository, artifact)
	if err != nil || within == ".." || filepath.IsAbs(within) || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		t.Fatalf("V10 contract manifest path escapes the repository: %q", relative)
	}
	return artifact
}
