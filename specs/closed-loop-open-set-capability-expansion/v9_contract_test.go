package closedloopopensetcontract

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	v9SpecHash             = "2960478aab3964738eb3f006fb2faad16a94b0e1de53009b5c1fcf044e8aff28"
	v9PlanHash             = "76aca4f9d1ea35afacc162f80ca2d0b84633bdc07b6eda26abec2ac08ba5d151"
	v9CorpusRulesHash      = "99033fbbdb35f06687759936665a2502bf862523f3ef4dfc0d3059d980711745"
	v9BaselineProtocolHash = "1e128f0e6ba5998ef45adcad04bfb7f78ce855bb9f6f5610d843727f23cd18a5"
	v9PrismReviewHash      = "72103462913aefe3f6feb36885646999d02d6e195fb42095824f1b2ff1c4b312"
	v9V8RetirementHash     = "8c16b1a406b717851de511377660edba9db0c701e375a7d2c189eaf2abe9c06e"
	v9V8RetirementSumsHash = "098cb6ac3de6cd30f9a7d38fd6a337896eaf7a39e8a693d9b88b87462d3d13dd"
)

func TestVersionNineContractInputsAreFrozen(t *testing.T) {
	directory := v8ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	files := map[string]string{
		filepath.Join(directory, "V9_SPEC_ADDENDUM.md"):     v9SpecHash,
		filepath.Join(directory, "V9_PLAN.md"):              v9PlanHash,
		filepath.Join(directory, "V9_CORPUS_RULES.md"):      v9CorpusRulesHash,
		filepath.Join(directory, "V9_BASELINE_PROTOCOL.md"): v9BaselineProtocolHash,
		filepath.Join(directory, "V9_PRISM_REVIEW.md"):      v9PrismReviewHash,
		filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v8_round_1_retirement", "retirement.json"):  v9V8RetirementHash,
		filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v8_round_1_retirement", "CHECKSUMS.sha256"): v9V8RetirementSumsHash,
	}
	for path, want := range files {
		if got := v8FileSHA256(t, path); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", filepath.Base(path), got, want)
		}
	}

	retirement := v8DecodeObject(t, v8ReadFile(t, filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v8_round_1_retirement", "retirement.json")))
	v8RequireKeys(t, retirement, []string{"schema", "version", "generation", "infrastructure_commit", "runner_manifest_sha256", "implementation_commit", "implementation_seal_sha256", "implementation_seal_file_sha256", "input_selection_sha256", "input_frontier_sha256", "reason", "held_out_opened", "hash"})
	if v8String(t, retirement, "schema") != "kicadai.closed-loop-open-set-retirement.v8" ||
		v8Int(t, retirement, "version") != 8 || v8Int(t, retirement, "generation") != 1 ||
		v8String(t, retirement, "reason") != "causal_lineage_invalid" || v8Bool(t, retirement, "held_out_opened") {
		t.Fatal("V9 does not start from the unopened V8 causal-lineage retirement boundary")
	}
	for _, field := range []string{"runner_manifest_sha256", "implementation_seal_sha256", "implementation_seal_file_sha256", "input_selection_sha256", "input_frontier_sha256", "hash"} {
		if !v8ValidSHA256(v8String(t, retirement, field)) {
			t.Fatalf("V8 retirement %s is not a SHA-256 commitment", field)
		}
	}
	for _, field := range []string{"infrastructure_commit", "implementation_commit"} {
		if !v8ValidHex(v8String(t, retirement, field), 40) {
			t.Fatalf("V8 retirement %s is not a full Git object ID", field)
		}
	}
}

func TestVersionNineProtocolFreezesExposureAndSiblingSafety(t *testing.T) {
	directory := v8ContractDirectory(t)
	checks := map[string][]string{
		"V9_SPEC_ADDENDUM.md": {
			"exactly 48 unique cases split 24/24",
			"`fully_covered_case_ids`",
			"`effect_exposure_case_ids`",
			"nonselected sibling path hashes",
			"exposed noncovered cases, ascending",
			"nonselected sibling paths in the exposure cohort, ascending",
			"at most two implementation rounds, six total atoms, and 18 total",
			"every committed nonselected sibling remains byte-identical",
			"`selected_path`, `nonselected_sibling`,",
			"successful encrypted admission, public\nretirement before final access, or consumed blind-final retirement",
		},
		"V9_PLAN.md": {
			"Bind the committed V8 retirement object",
			"four discovery and four held-out",
			"Rank with unlock/diversity first and collateral exposure/sibling burden next",
			"outcome-neutral sibling-preservation tests",
			"run all 24 discovery cases exactly twice",
		},
		"V9_CORPUS_RULES.md": {
			"exactly 48 requirements: 24 discovery and 24 held-out",
			"It may not prescribe or hint at parts, values as implementation choices,\ntopology, known circuit-family realization",
			"all 14 mandatory gates",
			"Corrections stop\nas soon as validation passes",
			"source, baseline, and final keys are distinct 32-byte 0600 regular",
		},
		"V9_BASELINE_PROTOCOL.md": {
			"All 24 discovery cases run in manifest order twice",
			"`effect_exposure_case_ids`",
			"all nonselected sibling path hashes per exposed case",
			"require every committed nonselected sibling path byte-identical",
			"require every non-exposed case byte-identical",
			"At most two\nrounds, six atoms, and 18 members",
			"A retirement artifact cannot be overwritten or retried",
		},
	}
	for name, required := range checks {
		source := strings.ReplaceAll(string(v8ReadFile(t, filepath.Join(directory, name))), "\r\n", "\n")
		for _, text := range required {
			if !strings.Contains(source, text) {
				t.Fatalf("%s omits frozen clause %q", name, text)
			}
		}
		for _, prohibited := range []string{"TODO", "TBD", "FIXME"} {
			if strings.Contains(source, prohibited) {
				t.Fatalf("%s contains unresolved marker %q", name, prohibited)
			}
		}
	}
}

func TestVersionNineContractChecksumManifest(t *testing.T) {
	directory := v8ContractDirectory(t)
	wantPaths := []string{
		"V9_SPEC_ADDENDUM.md",
		"V9_PLAN.md",
		"V9_CORPUS_RULES.md",
		"V9_BASELINE_PROTOCOL.md",
		"V9_PRISM_REVIEW.md",
		"v9_contract_test.go",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v8_round_1_retirement/retirement.json",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v8_round_1_retirement/CHECKSUMS.sha256",
	}
	file, err := os.Open(filepath.Join(directory, "V9_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actualPaths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		digest, relative, found := strings.Cut(scanner.Text(), "  ")
		if !found || relative == "" || !v8ValidSHA256(digest) {
			t.Fatalf("invalid V9 contract checksum line %q", scanner.Text())
		}
		path := filepath.Clean(filepath.Join(directory, filepath.FromSlash(relative)))
		if got := v8FileSHA256(t, path); got != digest {
			t.Fatalf("V9 contract checksum for %s = %s, want %s", relative, got, digest)
		}
		actualPaths = append(actualPaths, relative)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actualPaths, wantPaths) {
		t.Fatalf("V9 contract paths = %q, want %q", actualPaths, wantPaths)
	}
}
