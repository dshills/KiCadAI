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
	v8SpecHash             = "b9fcb3e98b9876597b02678be80221557e1e6ccdc59e8e3f04193e49295a75ba"
	v8PlanHash             = "5940e40d8c2338269b3e6204dccf3343244301226599bb6ce3515212b922a788"
	v8CorpusRulesHash      = "8b76b96c63d0de64c50c7a70fd8f02ffb1cb651cbd2fd075e2c770938b393247"
	v8BaselineProtocolHash = "25b464874035a1ee08a6b40f2e61f89d393b176dcbf0095c6ccbfefc4ecdbabe"
	v8SelectionPolicyHash  = "d428cbc4cc3564dd9216a7fe1d1d9e1ac6adb929e2fc258d106473d2c15e61f4"
	v8PrismReviewHash      = "b7f0fce5bb552be57cc73adc981fcf51e9187f51b61cfd8699977af147798760"
	v8V7RetirementHash     = "1bc6f74cda6745abb8c19cc43dfc760b066277ab57b5169636e289c3195d2706"
	v8V7RetirementSumsHash = "5c559e2c11b9f765c34f5824b72029828c08b761d9914c73be4da71c234b6dec"
)

func TestVersionEightContractInputsAreFrozen(t *testing.T) {
	directory := v8ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	files := map[string]string{
		filepath.Join(directory, "V8_SPEC_ADDENDUM.md"):      v8SpecHash,
		filepath.Join(directory, "V8_PLAN.md"):               v8PlanHash,
		filepath.Join(directory, "V8_CORPUS_RULES.md"):       v8CorpusRulesHash,
		filepath.Join(directory, "V8_BASELINE_PROTOCOL.md"):  v8BaselineProtocolHash,
		filepath.Join(directory, "V8_SELECTION_POLICY.json"): v8SelectionPolicyHash,
		filepath.Join(directory, "V8_PRISM_REVIEW.md"):       v8PrismReviewHash,
		filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v7_round_1_retirement", "retirement.json"):  v8V7RetirementHash,
		filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v7_round_1_retirement", "CHECKSUMS.sha256"): v8V7RetirementSumsHash,
	}
	for path, want := range files {
		if got := v8FileSHA256(t, path); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", filepath.Base(path), got, want)
		}
	}

	retirementPath := filepath.Join(repositoryRoot, "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v7_round_1_retirement", "retirement.json")
	retirement := v8DecodeObject(t, v8ReadFile(t, retirementPath))
	v8RequireKeys(t, retirement, []string{"schema", "version", "generation", "infrastructure_commit", "implementation_seal_sha256", "input_selection_sha256", "input_frontier_sha256", "reason", "held_out_opened", "hash"})
	if v8String(t, retirement, "schema") != "kicadai.closed-loop-open-set-retirement.v7" ||
		v8Int(t, retirement, "version") != 7 || v8Int(t, retirement, "generation") != 1 ||
		v8Bool(t, retirement, "held_out_opened") || v8String(t, retirement, "reason") == "" {
		t.Fatal("V8 does not start from the unopened permanent V7 public-retirement boundary")
	}
	if !v8ValidHex(v8String(t, retirement, "infrastructure_commit"), 40) {
		t.Fatal("V7 retirement infrastructure_commit is not a full Git object ID")
	}
	for _, field := range []string{"implementation_seal_sha256", "input_selection_sha256", "input_frontier_sha256", "hash"} {
		if !v8ValidSHA256(v8String(t, retirement, field)) {
			t.Fatalf("V7 retirement %s is not a SHA-256 commitment", field)
		}
	}
}

func TestVersionEightSelectionPolicyFreezesObligationLineageAndEffectClosure(t *testing.T) {
	policy := v8PolicyObject(t)
	v8RequireKeys(t, policy, []string{
		"schema", "version", "predecessor", "corpus", "eligible_outcomes", "all_outcomes", "gap_categories",
		"obligation_anchor", "causal_path", "effect_closure", "adaptive_rounds", "candidate_generation", "ranking",
		"safety_weights", "tie_behavior", "identity_order_is_impact_evidence", "round_gates", "public_admission",
		"validation_gates", "deterministic_execution", "held_out", "prohibitions",
	})
	if v8String(t, policy, "schema") != "kicadai.closed-loop-open-set-selection-policy.v8" || v8Int(t, policy, "version") != 8 {
		t.Fatal("V8 selection policy schema or version is invalid")
	}

	predecessor := v8Object(t, policy, "predecessor")
	v8RequireKeys(t, predecessor, []string{"version", "required_terminal_state", "required_generation", "require_held_out_opened_false", "reuse_predecessor_corpus", "reuse_predecessor_keys"})
	if v8Int(t, predecessor, "version") != 7 || v8String(t, predecessor, "required_terminal_state") != "public_retirement" ||
		v8Int(t, predecessor, "required_generation") != 1 || !v8Bool(t, predecessor, "require_held_out_opened_false") ||
		v8Bool(t, predecessor, "reuse_predecessor_corpus") || v8Bool(t, predecessor, "reuse_predecessor_keys") {
		t.Fatal("V8 predecessor isolation policy is invalid")
	}

	corpus := v8Object(t, policy, "corpus")
	v8RequireKeys(t, corpus, []string{"author_count", "requirements_per_author", "discovery_per_author", "held_out_per_author", "discovery_case_count", "held_out_case_count", "reporting_domains", "circuit_roles", "safety_impacts", "safety_impact_global_count_each", "maximum_fresh_replacements_per_assignment"})
	if v8Int(t, corpus, "author_count") != 6 || v8Int(t, corpus, "requirements_per_author") != 6 ||
		v8Int(t, corpus, "discovery_per_author") != 3 || v8Int(t, corpus, "held_out_per_author") != 3 ||
		v8Int(t, corpus, "discovery_case_count") != 18 || v8Int(t, corpus, "held_out_case_count") != 18 ||
		v8Int(t, corpus, "safety_impact_global_count_each") != 9 || v8Int(t, corpus, "maximum_fresh_replacements_per_assignment") != 2 {
		t.Fatal("V8 corpus cardinality or replacement bounds are invalid")
	}
	if got := v8Strings(t, corpus, "reporting_domains"); len(got) != 6 || !v8UniqueNonempty(got) {
		t.Fatalf("V8 reporting domains are invalid: %q", got)
	}
	if got := v8Strings(t, corpus, "circuit_roles"); len(got) != 6 || !v8UniqueNonempty(got) {
		t.Fatalf("V8 circuit roles are invalid: %q", got)
	}
	if got := v8Strings(t, corpus, "safety_impacts"); !slices.Equal(got, []string{"non_safety", "review_required", "safety_relevant", "safety_critical"}) {
		t.Fatalf("V8 safety impacts = %q", got)
	}

	if got := v8Strings(t, policy, "eligible_outcomes"); !slices.Equal(got, []string{"unsupported", "exhausted"}) {
		t.Fatalf("V8 eligible outcomes = %q", got)
	}
	if got := v8Strings(t, policy, "all_outcomes"); !slices.Equal(got, []string{"pass", "unsupported", "unsafe", "exhausted"}) {
		t.Fatalf("V8 outcomes = %q", got)
	}
	if got := v8Strings(t, policy, "gap_categories"); !slices.Equal(got, []string{"topology", "component", "model", "simulation", "physical_design", "verification"}) {
		t.Fatalf("V8 gap categories = %q", got)
	}

	anchor := v8Object(t, policy, "obligation_anchor")
	v8RequireKeys(t, anchor, []string{"digest", "encoding", "fields", "exclude_outcome_and_failure_classification", "publisher_derived", "duplicate_anchor"})
	if v8String(t, anchor, "digest") != "sha256" || v8String(t, anchor, "encoding") != "ordered_utf8_fields_each_prefixed_by_u32_big_endian_byte_length" ||
		!v8Bool(t, anchor, "exclude_outcome_and_failure_classification") || !v8Bool(t, anchor, "publisher_derived") ||
		v8String(t, anchor, "duplicate_anchor") != "retire_before_synthesis" {
		t.Fatal("V8 obligation-anchor policy is invalid")
	}
	if got := v8Strings(t, anchor, "fields"); !slices.Equal(got, []string{"corpus_manifest_sha256", "role", "case_id", "operating_case_id", "assertion_id", "observation_kind", "observation_id", "output_id"}) {
		t.Fatalf("V8 obligation-anchor fields = %q", got)
	}

	path := v8Object(t, policy, "causal_path")
	v8RequireKeys(t, path, []string{"generation_zero_leaf_count", "successor_requires_exact_prior_prefix", "successor_appended_leaf_count", "minimum_successors_for_still_failing_obligation", "maximum_successors_per_removed_leaf", "successor_same_obligation_anchor", "successor_unique_member_and_path", "successor_required_evidence", "successor_stage_relation", "path_rewrite_or_truncation", "unknown_stage", "stage_ordinal"})
	if v8Int(t, path, "generation_zero_leaf_count") != 1 || !v8Bool(t, path, "successor_requires_exact_prior_prefix") ||
		v8Int(t, path, "successor_appended_leaf_count") != 1 || v8Int(t, path, "minimum_successors_for_still_failing_obligation") != 1 ||
		v8Int(t, path, "maximum_successors_per_removed_leaf") != 4 || !v8Bool(t, path, "successor_same_obligation_anchor") ||
		!v8Bool(t, path, "successor_unique_member_and_path") || v8String(t, path, "successor_required_evidence") != "equal_or_strict_superset" ||
		v8String(t, path, "successor_stage_relation") != "same_or_higher_with_different_current_member" || v8String(t, path, "path_rewrite_or_truncation") != "retire" ||
		v8String(t, path, "unknown_stage") != "retire" {
		t.Fatal("V8 causal-path policy is invalid")
	}
	stageOrdinal := v8Object(t, path, "stage_ordinal")
	v8RequireKeys(t, stageOrdinal, []string{"topology", "component", "model", "simulation", "physical_design", "verification"})
	for index, stage := range []string{"topology", "component", "model", "simulation", "physical_design", "verification"} {
		if v8Int(t, stageOrdinal, stage) != index+1 {
			t.Fatalf("V8 stage %s ordinal is invalid", stage)
		}
	}

	closure := v8Object(t, policy, "effect_closure")
	v8RequireKeys(t, closure, []string{"required_before_implementation", "construction_may_use_corpus_outcomes", "required_mechanical_evidence", "unbounded_dynamic_lookup", "unmapped_outcome_affecting_consumer", "independent_separable_effect", "atoms_and_members_count_against_all_budgets", "changed_gap_outside_selection_or_closure"})
	if !v8Bool(t, closure, "required_before_implementation") || v8Bool(t, closure, "construction_may_use_corpus_outcomes") ||
		v8String(t, closure, "unbounded_dynamic_lookup") != "reject_plan" || v8String(t, closure, "unmapped_outcome_affecting_consumer") != "reject_plan" ||
		v8String(t, closure, "independent_separable_effect") != "count_as_capability_member" ||
		!v8Bool(t, closure, "atoms_and_members_count_against_all_budgets") || v8String(t, closure, "changed_gap_outside_selection_or_closure") != "retire" {
		t.Fatal("V8 effect-closure policy is invalid")
	}
	if evidence := v8Strings(t, closure, "required_mechanical_evidence"); len(evidence) != 6 || !v8UniqueNonempty(evidence) {
		t.Fatalf("V8 effect-closure evidence = %q", evidence)
	}

	rounds := v8Object(t, policy, "adaptive_rounds")
	v8RequireKeys(t, rounds, []string{"maximum_rounds", "maximum_total_capability_atoms", "maximum_total_exact_members", "maximum_round_capability_atoms", "maximum_round_exact_members", "maximum_candidate_bundles", "minimum_atom_active_case_support", "minimum_advanced_active_cases", "minimum_reporting_domains", "minimum_circuit_roles", "prior_atom_reselection", "one_implementation_per_round", "one_discovery_evaluation_per_round", "stop_at_first_public_pass_uplift"})
	for field, want := range map[string]int{"maximum_rounds": 3, "maximum_total_capability_atoms": 9, "maximum_total_exact_members": 27, "maximum_round_capability_atoms": 3, "maximum_round_exact_members": 9, "maximum_candidate_bundles": 1 << 18, "minimum_atom_active_case_support": 2, "minimum_advanced_active_cases": 2, "minimum_reporting_domains": 2, "minimum_circuit_roles": 2} {
		if got := v8Int(t, rounds, field); got != want {
			t.Fatalf("V8 %s = %d, want %d", field, got, want)
		}
	}
	if v8String(t, rounds, "prior_atom_reselection") != "prohibited" || !v8Bool(t, rounds, "one_implementation_per_round") ||
		!v8Bool(t, rounds, "one_discovery_evaluation_per_round") || !v8Bool(t, rounds, "stop_at_first_public_pass_uplift") {
		t.Fatal("V8 adaptive-round behavior is invalid")
	}

	candidates := v8Object(t, policy, "candidate_generation")
	v8RequireKeys(t, candidates, []string{"discovery_only", "complete_nonempty_case_subset_union_closure", "exact_deduplication", "exact_dominance_pruning", "truncation", "overflow", "require_complete_current_frontier_cover", "reject_empty_frontiers", "reject_duplicate_atoms", "reject_duplicate_members", "reject_duplicate_cases"})
	for _, field := range []string{"discovery_only", "complete_nonempty_case_subset_union_closure", "exact_deduplication", "exact_dominance_pruning", "require_complete_current_frontier_cover", "reject_empty_frontiers", "reject_duplicate_atoms", "reject_duplicate_members", "reject_duplicate_cases"} {
		if !v8Bool(t, candidates, field) {
			t.Fatalf("V8 candidate generation does not require %s", field)
		}
	}
	if v8String(t, candidates, "truncation") != "prohibited" || v8String(t, candidates, "overflow") != "fail_closed" {
		t.Fatal("V8 candidate overflow policy is invalid")
	}

	if got := v8Strings(t, policy, "ranking"); !slices.Equal(got, []string{"covered_active_case_count_desc", "covered_reporting_domain_count_desc", "covered_circuit_role_count_desc", "covered_safety_weight_desc", "capability_atom_count_including_closure_asc", "exact_member_count_including_closure_asc"}) {
		t.Fatalf("V8 ranking = %q", got)
	}
	safetyWeights := v8Object(t, policy, "safety_weights")
	v8RequireKeys(t, safetyWeights, []string{"non_safety", "review_required", "safety_relevant", "safety_critical"})
	if v8Int(t, safetyWeights, "non_safety") != 0 || v8Int(t, safetyWeights, "review_required") != 1 ||
		v8Int(t, safetyWeights, "safety_relevant") != 3 || v8Int(t, safetyWeights, "safety_critical") != 5 {
		t.Fatal("V8 safety weights are invalid")
	}
	if v8String(t, policy, "tie_behavior") != "publish_complete_semantic_co_rank_one_set_then_select_canonical_bundle_key_asc" || v8Bool(t, policy, "identity_order_is_impact_evidence") {
		t.Fatal("V8 tie behavior is invalid")
	}

	gates := v8Object(t, policy, "round_gates")
	v8RequireKeys(t, gates, []string{"require_exact_case_set", "require_all_selected_leaves_disappear", "require_nonselected_nonclosure_gaps_byte_identical", "require_complete_successor_paths", "require_nonempty_frontier_for_retained_nonpass", "require_no_pass_regression", "require_no_unsafe_to_pass", "require_deterministic_replay", "require_complete_physical_promotion", "require_seal_bound_environment"})
	for _, field := range []string{"require_exact_case_set", "require_all_selected_leaves_disappear", "require_nonselected_nonclosure_gaps_byte_identical", "require_complete_successor_paths", "require_nonempty_frontier_for_retained_nonpass", "require_no_pass_regression", "require_no_unsafe_to_pass", "require_deterministic_replay", "require_complete_physical_promotion", "require_seal_bound_environment"} {
		if !v8Bool(t, gates, field) {
			t.Fatalf("V8 round gate %s is disabled", field)
		}
	}
	admission := v8Object(t, policy, "public_admission")
	v8RequireKeys(t, admission, []string{"require_strict_total_pass_uplift", "minimum_new_active_cohort_passes", "stop_immediately_on_admission"})
	if !v8Bool(t, admission, "require_strict_total_pass_uplift") || v8Int(t, admission, "minimum_new_active_cohort_passes") != 1 || !v8Bool(t, admission, "stop_immediately_on_admission") {
		t.Fatal("V8 public-admission policy is invalid")
	}
	if validation := v8Strings(t, policy, "validation_gates"); len(validation) != 14 || !v8UniqueNonempty(validation) {
		t.Fatalf("V8 validation gates = %q", validation)
	}

	determinism := v8Object(t, policy, "deterministic_execution")
	v8RequireKeys(t, determinism, []string{"outcome_affecting_subprocess_concurrency", "synthesis_replays_per_case", "physical_promotions_per_pass", "byte_identical_complete_export_required", "fixed_worker_count", "fixed_traversal_order", "fixed_random_sources", "fixed_locale_timezone_and_floating_point_mode"})
	if v8Int(t, determinism, "outcome_affecting_subprocess_concurrency") != 1 || v8Int(t, determinism, "synthesis_replays_per_case") != 2 ||
		v8Int(t, determinism, "physical_promotions_per_pass") != 2 || !v8Bool(t, determinism, "byte_identical_complete_export_required") ||
		!v8Bool(t, determinism, "fixed_worker_count") || !v8Bool(t, determinism, "fixed_traversal_order") ||
		!v8Bool(t, determinism, "fixed_random_sources") || !v8Bool(t, determinism, "fixed_locale_timezone_and_floating_point_mode") {
		t.Fatal("V8 deterministic-execution policy is invalid")
	}
	heldOut := v8Object(t, policy, "held_out")
	v8RequireKeys(t, heldOut, []string{"influence_on_selection_or_implementation", "source_key_distinct", "baseline_key_distinct", "final_result_key_distinct", "key_bytes", "key_file_mode", "cipher", "nonce_bytes", "blind_final_requires_public_admission", "require_strict_pass_uplift", "minimum_new_lineage_covered_passes", "disclose_case_or_outcome_detail"})
	if v8String(t, heldOut, "influence_on_selection_or_implementation") != "prohibited" || !v8Bool(t, heldOut, "source_key_distinct") ||
		!v8Bool(t, heldOut, "baseline_key_distinct") || !v8Bool(t, heldOut, "final_result_key_distinct") || v8Int(t, heldOut, "key_bytes") != 32 ||
		v8String(t, heldOut, "key_file_mode") != "0600" || v8String(t, heldOut, "cipher") != "AES-256-GCM" || v8Int(t, heldOut, "nonce_bytes") != 12 ||
		!v8Bool(t, heldOut, "blind_final_requires_public_admission") || !v8Bool(t, heldOut, "require_strict_pass_uplift") ||
		v8Int(t, heldOut, "minimum_new_lineage_covered_passes") != 1 || v8Bool(t, heldOut, "disclose_case_or_outcome_detail") {
		t.Fatal("V8 held-out boundary is invalid")
	}
	if got := v8Strings(t, policy, "prohibitions"); !slices.Equal(got, []string{"fixture_specific_templates", "fixture_specific_coordinates", "corpus_identity_dispatch", "author_identity_dispatch", "expected_outcome_dispatch", "manual_selection_override", "post_synthesis_corpus_mutation", "budget_or_gate_relaxation", "held_out_disclosure", "manual_github_actions_run"}) {
		t.Fatalf("V8 prohibitions = %q", got)
	}
}

func TestVersionEightContractChecksumManifest(t *testing.T) {
	directory := v8ContractDirectory(t)
	wantPaths := []string{
		"V8_SPEC_ADDENDUM.md",
		"V8_PLAN.md",
		"V8_CORPUS_RULES.md",
		"V8_BASELINE_PROTOCOL.md",
		"V8_SELECTION_POLICY.json",
		"V8_PRISM_REVIEW.md",
		"v8_contract_test.go",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v7_round_1_retirement/retirement.json",
		"../../internal/capabilityfeedback/testdata/closed_loop_open_set_v7_round_1_retirement/CHECKSUMS.sha256",
	}
	file, err := os.Open(filepath.Join(directory, "V8_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actualPaths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || !v8ValidSHA256(fields[0]) {
			t.Fatalf("invalid V8 contract checksum line %q", scanner.Text())
		}
		path := filepath.Clean(filepath.Join(directory, filepath.FromSlash(fields[1])))
		if got := v8FileSHA256(t, path); got != fields[0] {
			t.Fatalf("V8 contract checksum for %s = %s, want %s", fields[1], got, fields[0])
		}
		actualPaths = append(actualPaths, fields[1])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actualPaths, wantPaths) {
		t.Fatalf("V8 contract paths = %q, want %q", actualPaths, wantPaths)
	}
}

func v8PolicyObject(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	return v8DecodeObject(t, v8ReadFile(t, filepath.Join(v8ContractDirectory(t), "V8_SELECTION_POLICY.json")))
}

func v8DecodeObject(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var value map[string]json.RawMessage
	v8DecodeRaw(t, data, &value)
	return value
}

func v8DecodeRaw(t *testing.T, data []byte, value any) {
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

func v8RequireKeys(t *testing.T, object map[string]json.RawMessage, want []string) {
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

func v8Object(t *testing.T, object map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	raw, ok := object[key]
	if !ok {
		t.Fatalf("missing object field %q", key)
	}
	return v8DecodeObject(t, raw)
}

func v8String(t *testing.T, object map[string]json.RawMessage, key string) string {
	t.Helper()
	var value string
	v8Field(t, object, key, &value)
	return value
}

func v8Strings(t *testing.T, object map[string]json.RawMessage, key string) []string {
	t.Helper()
	var value []string
	v8Field(t, object, key, &value)
	return value
}

func v8Int(t *testing.T, object map[string]json.RawMessage, key string) int {
	t.Helper()
	var value int
	v8Field(t, object, key, &value)
	return value
}

func v8Bool(t *testing.T, object map[string]json.RawMessage, key string) bool {
	t.Helper()
	var value bool
	v8Field(t, object, key, &value)
	return value
}

func v8Field(t *testing.T, object map[string]json.RawMessage, key string, value any) {
	t.Helper()
	raw, ok := object[key]
	if !ok {
		t.Fatalf("missing field %q", key)
	}
	v8DecodeRaw(t, raw, value)
}

func v8UniqueNonempty(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func v8ValidSHA256(value string) bool {
	return v8ValidHex(value, 64)
}

func v8ValidHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func v8ContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V8 contract directory")
	}
	return filepath.Dir(file)
}

func v8ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v8FileSHA256(t *testing.T, path string) string {
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
