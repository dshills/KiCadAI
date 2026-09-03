package generic_analysis_model_solver_admission_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestV20ContractAndEvaluatorManifests(t *testing.T) {
	verifyManifest(t, "V20_EVALUATOR.sha256")
	verifyManifest(t, "V20_CONTRACT.sha256")
	data, err := os.ReadFile("V20_EVALUATOR_FREEZE.json")
	if err != nil {
		t.Fatal(err)
	}
	var freeze struct {
		Schema                       string   `json:"schema"`
		Version                      int      `json:"version"`
		EvaluatorManifestSHA256      string   `json:"evaluator_manifest_sha256"`
		DiscoveryCaseCount           int      `json:"discovery_case_count"`
		ReplaysPerCase               int      `json:"replays_per_case"`
		MaximumParallelCases         int      `json:"maximum_parallel_cases"`
		SelectedAnalyses             []string `json:"selected_analyses"`
		RequestAdmissionBeforeSearch bool     `json:"request_admission_before_search"`
		CandidateAdmissionBeforeEval bool     `json:"candidate_admission_before_numerical_evaluation"`
		ExactProvenanceRequired      bool     `json:"exact_model_solver_provenance_required"`
		SubstitutionForbidden        bool     `json:"implicit_model_substitution_forbidden"`
		V19ArtifactsUnchanged        bool     `json:"v19_artifacts_unchanged"`
		HeldOutAccess                bool     `json:"held_out_access_surface"`
		RealCorpusEvaluated          bool     `json:"real_corpus_evaluated"`
	}
	if err := json.Unmarshal(data, &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v20" || freeze.Version != 20 ||
		freeze.EvaluatorManifestSHA256 != fileSHA256(t, "V20_EVALUATOR.sha256") ||
		freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 ||
		!slices.Equal(freeze.SelectedAnalyses, []string{"ac_sweep", "dc_operating_point", "dc_sweep", "stability", "transient"}) ||
		!freeze.RequestAdmissionBeforeSearch || !freeze.CandidateAdmissionBeforeEval ||
		!freeze.ExactProvenanceRequired || !freeze.SubstitutionForbidden || !freeze.V19ArtifactsUnchanged ||
		freeze.HeldOutAccess || freeze.RealCorpusEvaluated {
		t.Fatalf("invalid V20 evaluator freeze: %+v", freeze)
	}
}

func verifyManifest(t *testing.T, name string) {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Fatalf("invalid %s entry: %q", name, scanner.Text())
		}
		data, err := os.ReadFile(filepath.Clean(filepath.Join(root, filepath.FromSlash(fields[1]))))
		if err != nil {
			t.Fatalf("read %s entry %s: %v", name, fields[1], err)
		}
		digest := sha256.Sum256(data)
		if actual := hex.EncodeToString(digest[:]); actual != fields[0] {
			t.Fatalf("%s entry drifted: %s = %s, want %s", name, fields[1], actual, fields[0])
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("%s is empty", name)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestHistoricalV18V19AndCorpusBoundaryIsImmutable(t *testing.T) {
	root := filepath.Join("..", "..")
	want := map[string]string{
		"specs/closed-loop-open-set-capability-expansion/V19_CONTRACT.sha256":                         "4e4ab898b35ef99833fc3b4d68be9123f0058977cde915d8c99f193d627a2332",
		"specs/closed-loop-open-set-capability-expansion/V19_EVALUATOR.sha256":                        "563d55f9ce667a612c3ffcc1d5ad00d1112960f624b20fb426bff38b9c078485",
		"specs/closed-loop-open-set-capability-expansion/V19_GENERATION_ZERO_RETIREMENT.json":         "6daf24104c869f04315c16124fa0fab585ecc6b2d8f9f02a03df1f1900e45327",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v19_generation_zero/report.json":   "0bc7c0880e390a8f0cc7c74e3535ccc81be8ebc674c85060a4bfd35d516df09a",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v19_generation_zero/report.sha256": "f5ec73426d1f9ff34693922efd8ff6bc2d3f0ce3f052b37f5d8c5f58650de9aa",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v18_generation_one/report.json":    "332983874f65d84099f5f7a8740b9dd815aa6e892358f784be24a6c043f8edad",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/CHECKSUMS.sha256":       "24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf",
	}
	for path, expected := range want {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("historical boundary changed: %s = %s, want %s", path, actual, expected)
		}
	}
}

func TestPublicFailureTaxonomyIsCanonicalAndBounded(t *testing.T) {
	data, err := os.ReadFile("V20_PUBLIC_FAILURE_TAXONOMY.json")
	if err != nil {
		t.Fatal(err)
	}
	var taxonomy struct {
		Schema                string   `json:"schema"`
		Version               int      `json:"version"`
		SourceReportSHA256    string   `json:"source_report_sha256"`
		AffectedCaseCount     int      `json:"affected_case_count"`
		ReportingDomainCount  int      `json:"reporting_domain_count"`
		SingleFamilyCaseCount int      `json:"single_family_case_count"`
		TypedAtoms            []string `json:"typed_atoms"`
		Cases                 []struct {
			ID              string   `json:"id"`
			ReportingDomain string   `json:"reporting_domain"`
			SingleFamily    bool     `json:"single_family"`
			Atoms           []string `json:"atoms"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &taxonomy); err != nil {
		t.Fatal(err)
	}
	if taxonomy.Schema != "kicadai.public-analysis-model-solver-failure-taxonomy.v20" || taxonomy.Version != 20 ||
		taxonomy.SourceReportSHA256 != "332983874f65d84099f5f7a8740b9dd815aa6e892358f784be24a6c043f8edad" ||
		taxonomy.AffectedCaseCount != 9 || taxonomy.ReportingDomainCount != 5 || taxonomy.SingleFamilyCaseCount != 7 ||
		len(taxonomy.Cases) != 9 || !slices.IsSorted(taxonomy.TypedAtoms) {
		t.Fatalf("invalid public taxonomy header: %+v", taxonomy)
	}
	seen := map[string]bool{}
	domains := map[string]bool{}
	singleFamily := 0
	previousID := ""
	for _, item := range taxonomy.Cases {
		if item.ID <= previousID || seen[item.ID] || len(item.Atoms) == 0 || !slices.IsSorted(item.Atoms) {
			t.Fatalf("non-canonical public taxonomy case: %+v", item)
		}
		previousID = item.ID
		seen[item.ID] = true
		domains[item.ReportingDomain] = true
		if item.SingleFamily {
			singleFamily++
		}
		for _, atom := range item.Atoms {
			if !slices.Contains(taxonomy.TypedAtoms, atom) {
				t.Fatalf("case %s has unknown atom %s", item.ID, atom)
			}
		}
	}
	if len(domains) != taxonomy.ReportingDomainCount || singleFamily != taxonomy.SingleFamilyCaseCount {
		t.Fatalf("taxonomy counts = domains %d single-family %d", len(domains), singleFamily)
	}
}
