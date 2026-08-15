package closedloopopensetcontract

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"testing"
)

func TestVersionSixteenGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V16_GENERATION_ZERO_RETIREMENT.json"))
	var retirement struct {
		Schema                   string `json:"schema"`
		Version                  int    `json:"version"`
		Stage                    string `json:"stage"`
		RepositoryCommit         string `json:"repository_commit"`
		EvaluatorFreezeCommit    string `json:"evaluator_freeze_commit"`
		ContractManifestSHA256   string `json:"contract_manifest_sha256"`
		EvaluatorManifestSHA256  string `json:"evaluator_manifest_sha256"`
		CorpusManifestSHA256     string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256    string `json:"corpus_checksums_sha256"`
		EnvironmentSHA256        string `json:"environment_sha256"`
		EvaluationRootSHA256     string `json:"evaluation_root_sha256"`
		PartialCheckpointsSHA256 string `json:"partial_checkpoints_sha256"`
		RequiredCaseCount        int    `json:"required_case_count"`
		CompletedCheckpoints     int    `json:"completed_case_checkpoints"`
		ReplaysPerCase           int    `json:"replays_per_case"`
		ParallelCaseLimit        int    `json:"parallel_case_limit"`
		AcceptedReport           bool   `json:"accepted_report_published"`
		PublicBaseline           bool   `json:"public_baseline_published"`
		HeldOutSourceKeyCreated  bool   `json:"held_out_source_key_created"`
		HeldOutSourceOpened      bool   `json:"held_out_source_opened"`
		HeldOutBaselineCreated   bool   `json:"held_out_baseline_key_created"`
		HeldOutBaselineOpened    bool   `json:"held_out_baseline_key_opened"`
		PartialEvidenceAccepted  bool   `json:"partial_evidence_accepted"`
		Reason                   string `json:"reason"`
		FailureClass             string `json:"failure_class"`
		TerminalState            string `json:"terminal_state"`
		Hash                     string `json:"hash"`
	}
	decodeV11Strict(t, data, &retirement)
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v16" || retirement.Version != 16 ||
		retirement.Stage != "public_generation_zero" || retirement.RepositoryCommit != "cdaa70aa06e6bfadbc2e92e06516877c2bea386f" ||
		retirement.EvaluatorFreezeCommit != "68a1f0beae6fb1ec33ee9c6af491e4af670ddaff" || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "transient_observation_retention_memory_pressure_manual_termination" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V16 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCheckpoints != 21 || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 1 ||
		retirement.AcceptedReport || retirement.PublicBaseline || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineCreated || retirement.HeldOutBaselineOpened || retirement.PartialEvidenceAccepted {
		t.Fatal("V16 retirement crossed its incomplete public-only boundary")
	}
	if retirement.ContractManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V16_CONTRACT.sha256")) ||
		retirement.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V16_EVALUATOR.sha256")) ||
		retirement.PartialCheckpointsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V16_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V16 retirement historical bindings do not reproduce")
	}
	root := bytes.TrimSuffix(v7ReadFile(t, filepath.Join(directory, "V16_GENERATION_ZERO_ROOT.json")), []byte{'\n'})
	rootDigest := sha256.Sum256(root)
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V16 evaluation-root commitment does not reproduce")
	}
	var canonical map[string]any
	if err := json.Unmarshal(data, &canonical); err != nil {
		t.Fatal(err)
	}
	delete(canonical, "hash")
	canonicalBytes, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonicalBytes)
	if hex.EncodeToString(digest[:]) != retirement.Hash {
		t.Fatalf("V16 retirement self-hash does not reproduce: got %s want %s", hex.EncodeToString(digest[:]), retirement.Hash)
	}

	pattern := regexp.MustCompile(`^[0-9a-f]{64}  checkpoints/v10_case_(00[1-9]|01[0-9]|02[0-1])\.json$`)
	scanner := bufio.NewScanner(bytes.NewReader(v7ReadFile(t, filepath.Join(directory, "V16_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))))
	count := 0
	for scanner.Scan() {
		if !pattern.MatchString(scanner.Text()) {
			t.Fatalf("invalid V16 checkpoint commitment %q", scanner.Text())
		}
		count++
	}
	if err := scanner.Err(); err != nil || count != 21 {
		t.Fatalf("V16 partial checkpoint count=%d error=%v", count, err)
	}
}

func TestVersionSeventeenEvaluatorIsFrozenPublicOnlyAndBoundsTransientEvidence(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V17_CONTRACT.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V17_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                              string `json:"schema"`
		Version                             int    `json:"version"`
		FreezeParentCommit                  string `json:"freeze_parent_commit"`
		EvaluatorManifest                   string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256             string `json:"evaluator_manifest_sha256"`
		V16RetirementSHA256                 string `json:"v16_retirement_sha256"`
		CorpusManifestSHA256                string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256               string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount                  int    `json:"discovery_case_count"`
		ReplaysPerCase                      int    `json:"replays_per_case"`
		MaximumParallelCases                int    `json:"maximum_parallel_cases"`
		MaximumLiveReplays                  int    `json:"maximum_live_replays_per_worker"`
		PostReplayFullGC                    bool   `json:"post_replay_full_gc"`
		PostReplayOSMemoryRelease           bool   `json:"post_replay_os_memory_release"`
		CompleteReplayBufferRetained        bool   `json:"complete_replay_buffer_retained"`
		LazyValueTrialMaterialization       bool   `json:"lazy_value_trial_graph_materialization"`
		EagerValueTrialGraphsRetained       bool   `json:"eager_value_trial_graphs_retained"`
		ValueTrialOrderPreserved            bool   `json:"value_trial_validation_order_preserved"`
		BestFailureGraphsPerTopology        int    `json:"best_failure_graphs_per_topology"`
		AllFailedGraphsRetained             bool   `json:"all_failed_graphs_retained"`
		FailureRankingOrderPreserved        bool   `json:"failure_ranking_order_preserved"`
		BoundedProposalSizingGeneration     bool   `json:"bounded_proposal_sizing_generation"`
		VariantsBeyondLimitMaterialized     bool   `json:"whole_graph_value_variants_materialized_beyond_limit"`
		StreamedCanonicalRepairHashing      bool   `json:"streamed_canonical_repair_hashing"`
		CompleteRepairHashBufferRetained    bool   `json:"complete_repair_hash_buffer_retained"`
		StreamedCanonicalSynthesisHashing   bool   `json:"streamed_canonical_synthesis_hashing"`
		CompleteSynthesisHashBufferRetained bool   `json:"complete_synthesis_hash_buffer_retained"`
		MaximumDynamicTimeSteps             int    `json:"maximum_dynamic_time_steps"`
		RetainedReportPoints                int    `json:"retained_report_points"`
		StreamedSimulationReportHashing     bool   `json:"streamed_simulation_report_hashing"`
		CompleteSimulationReportRetained    bool   `json:"complete_simulation_report_retained"`
		OutputSchemaPreserved               bool   `json:"output_schema_preserved"`
		FullSimulationProofHashesPreserved  bool   `json:"full_simulation_proof_hashes_preserved"`
		AssertionResultSemanticsPreserved   bool   `json:"assertion_result_semantics_preserved"`
		LegacyEvaluatorBehaviorUnchanged    bool   `json:"legacy_evaluator_behavior_unchanged"`
		ImmutableV10CorpusReused            bool   `json:"immutable_v10_corpus_reused"`
		V16CheckpointsReused                bool   `json:"v16_checkpoints_reused"`
		ProductionPath                      bool   `json:"production_path"`
		HeldOutAccessSurface                bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated                 bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened                   bool   `json:"external_key_opened"`
	}
	decodeV11Strict(t, data, &freeze)
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v17" || freeze.Version != 17 ||
		freeze.FreezeParentCommit != "cdaa70aa06e6bfadbc2e92e06516877c2bea386f" || freeze.EvaluatorManifest != "V17_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V16RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V16_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V17 freeze identity: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 || freeze.MaximumLiveReplays != 1 ||
		!freeze.PostReplayFullGC || !freeze.PostReplayOSMemoryRelease || freeze.CompleteReplayBufferRetained ||
		!freeze.LazyValueTrialMaterialization || freeze.EagerValueTrialGraphsRetained || !freeze.ValueTrialOrderPreserved ||
		freeze.BestFailureGraphsPerTopology != 1 || freeze.AllFailedGraphsRetained || !freeze.FailureRankingOrderPreserved ||
		!freeze.BoundedProposalSizingGeneration || freeze.VariantsBeyondLimitMaterialized ||
		!freeze.StreamedCanonicalRepairHashing || freeze.CompleteRepairHashBufferRetained ||
		!freeze.StreamedCanonicalSynthesisHashing || freeze.CompleteSynthesisHashBufferRetained ||
		freeze.MaximumDynamicTimeSteps != 100_000 || freeze.RetainedReportPoints != 256 ||
		!freeze.StreamedSimulationReportHashing || freeze.CompleteSimulationReportRetained ||
		!freeze.OutputSchemaPreserved || !freeze.FullSimulationProofHashesPreserved ||
		!freeze.AssertionResultSemanticsPreserved || !freeze.LegacyEvaluatorBehaviorUnchanged ||
		!freeze.ImmutableV10CorpusReused || freeze.V16CheckpointsReused ||
		!freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V17 freeze does not enforce its bounded public-only evaluator")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
}
