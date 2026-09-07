package closedloopopensetcontract

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const (
	v7StartingCommit       = "156f7eb439ca5313471c504ddb91db1b8a8724f0"
	v7SpecHash             = "81957ffc8bd4107ca13305710229a148748a726740608ea5ff84e08af0ef7f6d"
	v7PlanHash             = "7651ccfb10814dac54625f8b638d8d5c250c0789cf140407c5e58ff29062a77f"
	v7CorpusRulesHash      = "d3e568d9ef6b33980cd9e539093956d4c4c2f6bee8fedab760a5839fc8df026b"
	v7BaselineProtocolHash = "f96e356bac01dd861c9c6a6574bb8cf069f17628d941575a83b09745cf5ac795"
	v7SelectionPolicyHash  = "da0fbb3948d6e422627f17ca0c85f3063dbf5cf3b3fa0cc781335ad2e642a7e7"
	v7PrismReviewHash      = "24bda99991b8791712770bc3c41f3e01ee025810522e26b8172ebaea80d9b027"
	v7V6RetirementHash     = "822318265fdec0fa5a01c89cff49ea895321d173bd7d2e61feb1a3d11ec596bb"
	v7V6RetirementAudit    = "516689a90ae9b8d32cda0018255dfc29e8ffb5c45775ee7f02267b9685e13b49"
	v7V6RetirementSums     = "bdc12f543d63b9589c56ecd09b0077acadd3961163dd95b812c4e25edaa73d5c"
	v7RoundModelHash       = "0d446720ba37c27d9b8db0c639189a698d094392fb514987571c7e9b195ff6b3"
	v7RoundPolicyHash      = "bb22c538146284a6719c0f979a63b03b016a18a047c73b8cb1b0687da06e2e58"
	v7RoundSelectHash      = "a4424c1b95dd401011dcc3f7c62ec091dcf171247e87ee3a0ffd0f340cd6e80b"
	v7RoundSelectTestHash  = "ada5768630bfc04898d6c07424d32f719c86740b1230ad8286e029e81d18649d"
	v7RoundAdvanceHash     = "6fdd2bf31503ab0aad6732ecee3abd3616f179b4721df36aa2f13c9b9b51b770"
	v7RoundAdvanceTestHash = "30eaa7add9190cd2da49b95e68e250f1c0ca9ad77006f194d315e97dd0164031"
	v7PhaseZeroReviewHash  = "05b5fcc461f3bf62f3c1cfa2d34b44e345c54fae43b1da361f66c2ce5f37b7e4"
	v7HistoricalHelperHash = "c4b1f440d0d7abaca2d494bfa2b63548face60e1804f9634b69758c3327d75b5"
	v7V3ContractTestHash   = "5dd47ea3361df1a3baf7bb70c197d185ab602fc19fef480db17c4233c245aeb7"
	v7V4ContractTestHash   = "92055586f8dda667fc155908b331f3b8d4363d5f09da77dfd83e94a5dd8c845e"
	v7V5ContractTestHash   = "659f21fe606714dd32d90759b35cfe166f4d1e8b9651ecfbf6486ebdca878656"
	v7V5ValidatorTestHash  = "b5d471160475cafd84c9e381880e6c1f1da7fd656b4ddd5612362aa9094f4acb"
	v7V6ValidatorTestHash  = "2b0e87a33742ced6067211c4fc7add630eb9d49c733c0bbddc8101be619fc79d"
)

func TestVersionSevenContractInputsAreFrozen(t *testing.T) {
	directory := v7ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	files := map[string]string{
		filepath.Join(directory, "V7_SPEC_ADDENDUM.md"):       v7SpecHash,
		filepath.Join(directory, "V7_PLAN.md"):                v7PlanHash,
		filepath.Join(directory, "V7_CORPUS_RULES.md"):        v7CorpusRulesHash,
		filepath.Join(directory, "V7_BASELINE_PROTOCOL.md"):   v7BaselineProtocolHash,
		filepath.Join(directory, "V7_SELECTION_POLICY.json"):  v7SelectionPolicyHash,
		filepath.Join(directory, "V7_PRISM_REVIEW.md"):        v7PrismReviewHash,
		filepath.Join(directory, "V7_PHASE0_PRISM_REVIEW.md"): v7PhaseZeroReviewHash,
		filepath.Join(repositoryRoot, "internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/retirement.json"):     v7V6RetirementHash,
		filepath.Join(repositoryRoot, "internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/RETIREMENT_AUDIT.md"): v7V6RetirementAudit,
		filepath.Join(repositoryRoot, "internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/CHECKSUMS.sha256"):    v7V6RetirementSums,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/model.go"):                                                                 v7RoundModelHash,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/policy.go"):                                                                v7RoundPolicyHash,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/select.go"):                                                                v7RoundSelectHash,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/select_test.go"):                                                           v7RoundSelectTestHash,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/advance.go"):                                                               v7RoundAdvanceHash,
		filepath.Join(repositoryRoot, "internal/capabilityrounds/advance_test.go"):                                                          v7RoundAdvanceTestHash,
		filepath.Join(directory, "historical_manifest_test.go"):                                                                             v7HistoricalHelperHash,
		filepath.Join(directory, "v3_contract_test.go"):                                                                                     v7V3ContractTestHash,
		filepath.Join(directory, "v4_contract_test.go"):                                                                                     v7V4ContractTestHash,
		filepath.Join(directory, "v5_contract_test.go"):                                                                                     v7V5ContractTestHash,
		filepath.Join(directory, "v5_validator_contract_test.go"):                                                                           v7V5ValidatorTestHash,
		filepath.Join(directory, "v6_validator_contract_test.go"):                                                                           v7V6ValidatorTestHash,
	}
	for path, want := range files {
		if got := v7FileSHA256(t, path); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", filepath.Base(path), got, want)
		}
	}

	policy := v7PolicyObject(t)
	if got := v7String(t, policy, "starting_commit"); got != v7StartingCommit {
		t.Fatalf("V7 starting commit = %q, want %q", got, v7StartingCommit)
	}
	if got := v7String(t, policy, "starting_commit_expected_artifact"); got != "v6_retirement_commitment" {
		t.Fatalf("V7 starting artifact = %q", got)
	}

	retirementPath := filepath.Join(repositoryRoot, "internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/retirement.json")
	retirement := v7DecodeObject(t, v7ReadFile(t, retirementPath))
	v7RequireKeys(t, retirement, []string{
		"schema", "version", "selection_sha256", "implementation_hash", "comparison",
		"held_out_source_opened", "held_out_baseline_opened", "held_out_final_key_created",
		"final_updater", "hash",
	})
	if v7String(t, retirement, "schema") != "kicadai.closed-loop-open-set-public-retirement.v6" ||
		v7Int(t, retirement, "version") != 6 || v7Bool(t, retirement, "held_out_source_opened") ||
		v7Bool(t, retirement, "held_out_baseline_opened") || v7Bool(t, retirement, "held_out_final_key_created") ||
		v7String(t, retirement, "final_updater") != "permanently_retired" {
		t.Fatal("V7 starting state does not bind the unopened, permanently retired V6 boundary")
	}
}

func TestVersionSevenSelectionPolicyIsAdaptiveAndFailClosed(t *testing.T) {
	policy := v7PolicyObject(t)
	v7RequireKeys(t, policy, []string{
		"schema", "version", "starting_commit", "starting_commit_expected_artifact", "corpus_constraints",
		"candidate_unit", "atom_identity_fields", "member_identity_fields", "identity_encoding", "frontier_source",
		"eligible_outcomes", "adaptive_rounds", "cohort", "bundle_generation", "eligibility", "ranking",
		"safety_weight_value_type", "safety_weight_mapping", "publication_order", "tie_behavior", "tie_rationale",
		"round_plan_admission", "round_progress", "gap_stage_order", "gap_stage_aliases", "gap_stage_normalization",
		"unknown_gap_stage", "gap_lineage_successor", "unsafe_gate", "deterministic_execution", "environment_transition",
		"public_admission", "held_out_influence", "unsafe_case_unlock_credit", "external_key_management",
		"evidence_authority", "github_actions_role", "blind_final_preflight", "held_out_success_causality", "held_out_disclosure",
	})
	if v7String(t, policy, "schema") != "kicadai.closed-loop-open-set-selection-policy.v7" || v7Int(t, policy, "version") != 7 {
		t.Fatal("V7 selection policy schema or version is invalid")
	}
	if got := v7Strings(t, policy, "atom_identity_fields"); !slices.Equal(got, []string{"scope", "capability"}) {
		t.Fatalf("V7 atom identity = %q", got)
	}
	if got := v7Strings(t, policy, "member_identity_fields"); !slices.Equal(got, []string{"stage", "scope", "capability", "code"}) {
		t.Fatalf("V7 member identity = %q", got)
	}
	if v7String(t, policy, "identity_encoding") != "ordered_utf8_fields_each_prefixed_by_u32_big_endian_byte_length" ||
		v7String(t, policy, "held_out_influence") != "prohibited" ||
		v7String(t, policy, "unsafe_case_unlock_credit") != "prohibited" {
		t.Fatal("V7 identity or isolation policy is invalid")
	}

	corpus := v7Object(t, policy, "corpus_constraints")
	if v7Int(t, corpus, "total_case_count") != 36 || v7Int(t, corpus, "authors") != 3 ||
		v7Int(t, corpus, "cases_per_author") != 12 || v7Int(t, corpus, "acceptance_gate_count_per_case") != 14 ||
		v7Int(t, corpus, "maximum_correction_packets_per_author_context") != 3 ||
		v7Int(t, corpus, "maximum_replacement_author_contexts_per_assignment") != 2 {
		t.Fatal("V7 corpus cardinality or correction bounds are invalid")
	}
	roles := v7Object(t, corpus, "role_case_count")
	if v7Int(t, roles, "discovery") != 18 || v7Int(t, roles, "held_out") != 18 {
		t.Fatal("V7 role counts are invalid")
	}
	if got := v7Strings(t, corpus, "reporting_domains"); !slices.Equal(got, []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"}) {
		t.Fatalf("V7 reporting domains = %q", got)
	}
	if v7Int(t, corpus, "cases_per_domain_per_role") != 3 || v7Int(t, corpus, "cases_per_domain_total") != 6 ||
		v7Int(t, corpus, "safety_impact_category_total_minimum") != 6 ||
		v7Int(t, corpus, "safety_impact_category_total_maximum") != 12 ||
		v7Int(t, corpus, "primary_analysis_total_maximum") != 12 ||
		v7Int(t, corpus, "primary_metric_total_maximum") != 9 ||
		v7Int(t, corpus, "identifier_normalized_port_supply_shape_total_maximum") != 6 {
		t.Fatal("V7 diversity limits are invalid")
	}

	rounds := v7Object(t, policy, "adaptive_rounds")
	if v7Int(t, rounds, "maximum_rounds") != 3 || v7Int(t, rounds, "maximum_total_capability_atoms") != 9 ||
		v7Int(t, rounds, "maximum_total_exact_members") != 27 ||
		v7String(t, rounds, "prior_atom_reselection") != "prohibited" ||
		!v7Bool(t, rounds, "one_implementation_per_round") || !v7Bool(t, rounds, "one_discovery_evaluation_per_round") ||
		!v7Bool(t, rounds, "stop_at_first_public_pass_uplift") {
		t.Fatal("V7 adaptive-round bounds are invalid")
	}
	cohort := v7Object(t, policy, "cohort")
	if v7Int(t, cohort, "expected_discovery_case_count") != 18 || v7String(t, cohort, "regression_scope") != "all_discovery_cases_in_every_round" {
		t.Fatal("V7 cohort contract is invalid")
	}
	generation := v7Object(t, policy, "bundle_generation")
	if v7Int(t, generation, "maximum_round_capability_atoms") != 3 || v7Int(t, generation, "maximum_round_exact_members") != 9 ||
		v7Int(t, generation, "maximum_candidate_bundles") != 1<<18 || v7Int(t, generation, "minimum_atom_active_case_support") != 2 ||
		v7String(t, generation, "candidate_overflow") != "fail_closed_no_truncation" {
		t.Fatal("V7 candidate bounds are invalid")
	}
	eligibility := v7Object(t, policy, "eligibility")
	if v7Int(t, eligibility, "minimum_advanced_active_cases") != 2 || v7Int(t, eligibility, "minimum_reporting_domains") != 2 ||
		!v7Bool(t, eligibility, "applies_to_every_round") || v7Bool(t, eligibility, "final_round_relaxation") {
		t.Fatal("V7 generic reuse floor is invalid")
	}
	if got := v7Strings(t, policy, "ranking"); !slices.Equal(got, []string{
		"covered_active_case_count_desc", "covered_active_domain_count_desc", "covered_active_safety_weight_desc",
		"capability_atom_count_asc", "exact_member_count_asc",
	}) {
		t.Fatalf("V7 ranking = %q", got)
	}
	if v7String(t, policy, "tie_behavior") != "publish_complete_semantic_co_rank_one_set_then_select_canonical_bundle_key_asc" ||
		v7String(t, policy, "unknown_gap_stage") != "fail_closed" {
		t.Fatal("V7 tie or unknown-stage behavior is invalid")
	}

	v7ValidateStageOrder(t, policy)
	lineage := v7Object(t, policy, "gap_lineage_successor")
	for _, name := range []string{"same_case", "same_assertion_observation_causal_token", "same_scope", "same_capability"} {
		if !v7Bool(t, lineage, name) {
			t.Fatalf("V7 lineage %s is not required", name)
		}
	}
	if v7String(t, lineage, "required_evidence_relation") != "equal_or_strict_superset" ||
		v7String(t, lineage, "unrelated_beneficial_side_effect") != "reject_as_ambiguous_selected_bundle_causality" {
		t.Fatal("V7 lineage evidence or causality rule is invalid")
	}

	determinism := v7Object(t, policy, "deterministic_execution")
	if v7Int(t, determinism, "outcome_affecting_subprocess_concurrency") != 1 ||
		!v7Bool(t, determinism, "byte_identical_complete_export_required") {
		t.Fatal("V7 deterministic execution is invalid")
	}
	admission := v7Object(t, policy, "public_admission")
	if !v7Bool(t, admission, "require_total_pass_uplift") || v7Int(t, admission, "minimum_new_active_cohort_passes") != 1 ||
		!v7Bool(t, admission, "require_complete_lineage") || !v7Bool(t, admission, "require_no_pass_regression") ||
		!v7Bool(t, admission, "require_no_unsafe_to_pass") {
		t.Fatal("V7 public admission is invalid")
	}

	keys := v7Object(t, policy, "external_key_management")
	if v7Int(t, keys, "bytes") != 32 || v7String(t, keys, "cipher") != "AES-256-GCM" ||
		v7String(t, keys, "nonce") != "unique_random_12_bytes_96_bits_per_record" ||
		v7String(t, keys, "implementation_principal_access") != "prohibited" {
		t.Fatal("V7 external-key contract is invalid")
	}
	disclosure := v7Object(t, policy, "held_out_disclosure")
	if got := v7Strings(t, disclosure, "success_plaintext_fields"); !slices.Equal(got, []string{
		"total_case_count", "baseline_pass_count", "final_pass_count", "pass_uplift", "regression_count",
		"unsafe_to_pass_count", "causal_chain_covered", "deterministic_replay_passed", "complete_physical_evidence_passed",
	}) {
		t.Fatalf("V7 held-out success disclosure = %q", got)
	}
	if v7String(t, disclosure, "terminal_failure") != "non_revealing_permanent_audit_without_outcome_derived_aggregate" {
		t.Fatal("V7 terminal failure disclosure is invalid")
	}
}

func TestVersionSevenContractChecksumManifest(t *testing.T) {
	directory := v7ContractDirectory(t)
	wantPaths := []string{
		"V7_SPEC_ADDENDUM.md",
		"V7_PLAN.md",
		"V7_CORPUS_RULES.md",
		"V7_BASELINE_PROTOCOL.md",
		"V7_SELECTION_POLICY.json",
		"V7_PRISM_REVIEW.md",
		"V7_PHASE0_PRISM_REVIEW.md",
		"v7_contract_test.go",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/retirement.json",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/RETIREMENT_AUDIT.md",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v6_public_retirement/CHECKSUMS.sha256",
		"../../internal/capabilityrounds/model.go",
		"../../internal/capabilityrounds/policy.go",
		"../../internal/capabilityrounds/select.go",
		"../../internal/capabilityrounds/select_test.go",
		"../../internal/capabilityrounds/advance.go",
		"../../internal/capabilityrounds/advance_test.go",
		"historical_manifest_test.go",
		"v3_contract_test.go",
		"v4_contract_test.go",
		"v5_contract_test.go",
		"v5_validator_contract_test.go",
		"v6_validator_contract_test.go",
	}
	file, err := os.Open(filepath.Join(directory, "V7_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actualPaths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Fatalf("invalid V7 contract checksum line %q", scanner.Text())
		}
		path := filepath.Clean(filepath.Join(directory, filepath.FromSlash(fields[1])))
		if got := v7FileSHA256(t, path); got != fields[0] {
			t.Fatalf("V7 contract checksum for %s = %s, want %s", fields[1], got, fields[0])
		}
		actualPaths = append(actualPaths, fields[1])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actualPaths, wantPaths) {
		t.Fatalf("V7 contract paths = %q, want %q", actualPaths, wantPaths)
	}
}

func v7ValidateStageOrder(t *testing.T, policy map[string]json.RawMessage) {
	t.Helper()
	var groups []struct {
		Ordinal int      `json:"ordinal"`
		Stages  []string `json:"stages"`
	}
	v7DecodeRaw(t, policy["gap_stage_order"], &groups)
	seen := map[string]bool{}
	for index, group := range groups {
		if group.Ordinal != index || len(group.Stages) == 0 {
			t.Fatalf("V7 stage group %d is invalid", index)
		}
		for _, stage := range group.Stages {
			if stage == "" || seen[stage] {
				t.Fatalf("V7 stage %q is empty or duplicated", stage)
			}
			seen[stage] = true
		}
	}
	aliases := v7Object(t, policy, "gap_stage_aliases")
	for alias, raw := range aliases {
		var target string
		v7DecodeRaw(t, raw, &target)
		if alias == target || seen[alias] || !seen[target] {
			t.Fatalf("V7 stage alias %q -> %q is invalid", alias, target)
		}
	}
}

func v7PolicyObject(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	return v7DecodeObject(t, v7ReadFile(t, filepath.Join(v7ContractDirectory(t), "V7_SELECTION_POLICY.json")))
}

func v7DecodeObject(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var value map[string]json.RawMessage
	v7DecodeRaw(t, data, &value)
	return value
}

func v7DecodeRaw(t *testing.T, data []byte, value any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("JSON contains trailing data")
	}
}

func v7RequireKeys(t *testing.T, object map[string]json.RawMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("JSON keys = %q, want %q", got, want)
	}
}

func v7Object(t *testing.T, object map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	raw, ok := object[key]
	if !ok {
		t.Fatalf("missing object field %q", key)
	}
	return v7DecodeObject(t, raw)
}

func v7String(t *testing.T, object map[string]json.RawMessage, key string) string {
	t.Helper()
	var value string
	v7Field(t, object, key, &value)
	return value
}

func v7Strings(t *testing.T, object map[string]json.RawMessage, key string) []string {
	t.Helper()
	var value []string
	v7Field(t, object, key, &value)
	return value
}

func v7Int(t *testing.T, object map[string]json.RawMessage, key string) int {
	t.Helper()
	var value int
	v7Field(t, object, key, &value)
	return value
}

func v7Bool(t *testing.T, object map[string]json.RawMessage, key string) bool {
	t.Helper()
	var value bool
	v7Field(t, object, key, &value)
	return value
}

func v7Field(t *testing.T, object map[string]json.RawMessage, key string, value any) {
	t.Helper()
	raw, ok := object[key]
	if !ok {
		t.Fatalf("missing field %q", key)
	}
	v7DecodeRaw(t, raw, value)
}

func v7ContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V7 contract directory")
	}
	return filepath.Dir(file)
}

func v7ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v7FileSHA256(t *testing.T, path string) string {
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
