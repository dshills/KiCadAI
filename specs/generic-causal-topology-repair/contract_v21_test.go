package genericcausaltopologyrepair

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type v21EvaluatorFreeze struct {
	Schema                           string   `json:"schema"`
	Version                          int      `json:"version"`
	FreezeParentCommit               string   `json:"freeze_parent_commit"`
	EvaluatorManifest                string   `json:"evaluator_manifest"`
	EvaluatorManifestSHA256          string   `json:"evaluator_manifest_sha256"`
	InheritedV20ManifestSHA256       string   `json:"inherited_v20_evaluator_manifest_sha256"`
	V20ReportSHA256                  string   `json:"v20_generation_zero_report_sha256"`
	SelectedPopulationSHA256         string   `json:"selected_population_sha256"`
	CorpusManifestSHA256             string   `json:"corpus_manifest_sha256"`
	CorpusChecksumsSHA256            string   `json:"corpus_checksums_sha256"`
	DiscoveryCaseCount               int      `json:"discovery_case_count"`
	ReplaysPerCase                   int      `json:"replays_per_case"`
	MaximumParallelCases             int      `json:"maximum_parallel_cases"`
	TransportSchemaVersion           int      `json:"transport_schema_version"`
	SelectedCaseCount                int      `json:"selected_case_count"`
	SelectedReportingDomainCount     int      `json:"selected_reporting_domain_count"`
	CausalRepairOccurrences          int      `json:"causal_topology_repair_occurrences"`
	CompleteTopologyOccurrences      int      `json:"complete_topology_occurrences"`
	SelectedStopReasons              []string `json:"selected_stop_reasons"`
	V20FirstDelegation               bool     `json:"v20_first_delegation_required"`
	IneligibleByteIdentical          bool     `json:"ineligible_results_byte_identical"`
	UnsafeResultsTerminal            bool     `json:"unsafe_results_terminal"`
	V20AdmissionBeforeTopology       bool     `json:"v20_request_admission_before_topology"`
	V20AdmissionBeforeEvaluation     bool     `json:"v20_candidate_admission_before_numerical_evaluation"`
	ExactProvenanceRequired          bool     `json:"exact_model_solver_provenance_required"`
	ImplicitSubstitutionForbidden    bool     `json:"implicit_model_substitution_forbidden"`
	MaximumDepth                     int      `json:"maximum_depth"`
	MaximumWidth                     int      `json:"maximum_width"`
	MaximumWork                      int      `json:"maximum_work"`
	MaximumRetained                  int      `json:"maximum_retained"`
	MaximumGraphBytes                int      `json:"maximum_graph_bytes"`
	TopologyWorkers                  int      `json:"topology_workers"`
	CanonicalOrder                   bool     `json:"canonical_operation_order_required"`
	StableHashDeduplication          bool     `json:"stable_hash_deduplication_required"`
	CycleDetection                   bool     `json:"cycle_detection_required"`
	StrictImprovement                bool     `json:"strict_structural_improvement_required"`
	RepairProvenance                 bool     `json:"repair_provenance_required"`
	MaterialCaseThreshold            int      `json:"material_case_threshold"`
	MaterialReportingDomainThreshold int      `json:"material_reporting_domain_threshold"`
	FailureRenamingDoesNotCount      bool     `json:"failure_renaming_does_not_count"`
	InstalledKiCadPromotionRequired  bool     `json:"installed_kicad_promotion_required_for_pass"`
	ImmutableV10CorpusReused         bool     `json:"immutable_v10_corpus_reused"`
	V18V19V20ArtifactsUnchanged      bool     `json:"v18_v19_v20_artifacts_unchanged"`
	ProductionPath                   bool     `json:"production_path"`
	HeldOutAccess                    bool     `json:"held_out_access_surface"`
	RealCorpusEvaluated              bool     `json:"real_corpus_evaluated"`
	PublicOutcomesObserved           bool     `json:"public_outcomes_observed"`
	ExternalKeyOpened                bool     `json:"external_key_opened"`
}

func TestV21ContractAndEvaluatorManifests(t *testing.T) {
	verifyV21Manifest(t, "V21_EVALUATOR.sha256")
	verifyV21Manifest(t, "V21_CONTRACT.sha256")
	data, err := os.ReadFile("V21_EVALUATOR_FREEZE.json")
	if err != nil {
		t.Fatal(err)
	}
	var freeze v21EvaluatorFreeze
	if err := json.Unmarshal(data, &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v21" || freeze.Version != 21 || freeze.FreezeParentCommit != "0136c091a8aeb1dcd4ccab75dba5010f98507d4d" {
		t.Fatalf("invalid V21 identity freeze: %+v", freeze)
	}
	if freeze.EvaluatorManifest != "V21_EVALUATOR.sha256" || freeze.EvaluatorManifestSHA256 != v21FileSHA256(t, freeze.EvaluatorManifest) || freeze.InheritedV20ManifestSHA256 != "1004d00da2e73f912bbb6b2422291c5c03ebddea75c173270f1b3be14f1bf2e5" {
		t.Fatalf("invalid V21 manifest freeze: %+v", freeze)
	}
	if freeze.V20ReportSHA256 != "6ea9b697f8852cb8f4d752f75e5fa44aca93de7bbad9bb5e5fc6c063b10ff6aa" || freeze.SelectedPopulationSHA256 != v21FileSHA256(t, "V21_PUBLIC_TOPOLOGY_POPULATION.json") || freeze.CorpusManifestSHA256 != "0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94" || freeze.CorpusChecksumsSHA256 != "24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf" {
		t.Fatalf("invalid V21 evidence-source freeze: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 || freeze.TransportSchemaVersion != 17 || freeze.SelectedCaseCount != 8 || freeze.SelectedReportingDomainCount != 5 || freeze.CausalRepairOccurrences != 47 || freeze.CompleteTopologyOccurrences != 9 || !slices.Equal(freeze.SelectedStopReasons, []string{"causal_topology_repair", "complete_topology"}) {
		t.Fatalf("invalid V21 population or replay freeze: %+v", freeze)
	}
	if !freeze.V20FirstDelegation || !freeze.IneligibleByteIdentical || !freeze.UnsafeResultsTerminal || !freeze.V20AdmissionBeforeTopology || !freeze.V20AdmissionBeforeEvaluation || !freeze.ExactProvenanceRequired || !freeze.ImplicitSubstitutionForbidden {
		t.Fatalf("invalid V21 inherited behavior freeze: %+v", freeze)
	}
	if freeze.MaximumDepth != 3 || freeze.MaximumWidth != 8 || freeze.MaximumWork != 48 || freeze.MaximumRetained != 64 || freeze.MaximumGraphBytes != 1<<20 || freeze.TopologyWorkers != 1 || !freeze.CanonicalOrder || !freeze.StableHashDeduplication || !freeze.CycleDetection || !freeze.StrictImprovement || !freeze.RepairProvenance {
		t.Fatalf("invalid V21 topology-policy freeze: %+v", freeze)
	}
	if freeze.MaterialCaseThreshold != 3 || freeze.MaterialReportingDomainThreshold != 2 || !freeze.FailureRenamingDoesNotCount || !freeze.InstalledKiCadPromotionRequired || !freeze.ImmutableV10CorpusReused || !freeze.V18V19V20ArtifactsUnchanged || !freeze.ProductionPath || freeze.HeldOutAccess || freeze.RealCorpusEvaluated || freeze.PublicOutcomesObserved || freeze.ExternalKeyOpened {
		t.Fatalf("invalid V21 preservation or publication freeze: %+v", freeze)
	}
}

func verifyV21Manifest(t *testing.T, name string) {
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
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			t.Fatalf("invalid %s entry: %q", name, line)
		}
		expected, relativePath := line[:64], line[66:]
		if actual := v21FileSHA256(t, filepath.Clean(filepath.Join(root, filepath.FromSlash(relativePath)))); actual != expected {
			t.Fatalf("%s entry drifted: %s = %s, want %s", name, relativePath, actual, expected)
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

func v21FileSHA256(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func TestHistoricalV20BoundaryIsImmutable(t *testing.T) {
	root := filepath.Join("..", "..")
	want := map[string]string{
		"specs/generic-analysis-model-solver-admission/V20_CONTRACT.sha256":                           "7a445b133027d1a31f5c4fce5efc4268e2d06862cc796b1e0c175f9e553493bf",
		"specs/generic-analysis-model-solver-admission/V20_EVALUATOR.sha256":                          "1004d00da2e73f912bbb6b2422291c5c03ebddea75c173270f1b3be14f1bf2e5",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v20_generation_zero/report.json":   "6ea9b697f8852cb8f4d752f75e5fa44aca93de7bbad9bb5e5fc6c063b10ff6aa",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v20_generation_zero/report.sha256": "02ba245d16b2a25ce729dc3a0a6ed39fc8bfdced00b29707865ebcf305d6fead",
		"internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus/CHECKSUMS.sha256":       "24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf",
	}
	for path, expected := range want {
		if actual := v21FileSHA256(t, filepath.Join(root, filepath.FromSlash(path))); actual != expected {
			t.Fatalf("historical V20 boundary changed: %s = %s, want %s", path, actual, expected)
		}
	}
}
