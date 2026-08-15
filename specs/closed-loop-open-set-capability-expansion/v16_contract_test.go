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

func TestVersionFifteenGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V15_GENERATION_ZERO_RETIREMENT.json"))
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
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v15" || retirement.Version != 15 ||
		retirement.Stage != "public_generation_zero" || retirement.RepositoryCommit != "68a1f0beae6fb1ec33ee9c6af491e4af670ddaff" ||
		retirement.EvaluatorFreezeCommit != "cecefcb9ddffb5068431160b98005af0adacfb05" || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "top_level_synthesis_hash_memory_pressure_manual_termination" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V15 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCheckpoints != 12 || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 1 ||
		retirement.AcceptedReport || retirement.PublicBaseline || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineCreated || retirement.HeldOutBaselineOpened || retirement.PartialEvidenceAccepted {
		t.Fatal("V15 retirement crossed its incomplete public-only boundary")
	}
	if retirement.ContractManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V15_CONTRACT.sha256")) ||
		retirement.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V15_EVALUATOR.sha256")) ||
		retirement.PartialCheckpointsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V15_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V15 retirement historical bindings do not reproduce")
	}
	root := bytes.TrimSuffix(v7ReadFile(t, filepath.Join(directory, "V15_GENERATION_ZERO_ROOT.json")), []byte{'\n'})
	rootDigest := sha256.Sum256(root)
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V15 evaluation-root commitment does not reproduce")
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
		t.Fatalf("V15 retirement self-hash does not reproduce: got %s want %s", hex.EncodeToString(digest[:]), retirement.Hash)
	}

	pattern := regexp.MustCompile(`^[0-9a-f]{64}  checkpoints/v10_case_(00[1-9]|01[0-2])\.json$`)
	scanner := bufio.NewScanner(bytes.NewReader(v7ReadFile(t, filepath.Join(directory, "V15_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))))
	count := 0
	for scanner.Scan() {
		if !pattern.MatchString(scanner.Text()) {
			t.Fatalf("invalid V15 checkpoint commitment %q", scanner.Text())
		}
		count++
	}
	if err := scanner.Err(); err != nil || count != 12 {
		t.Fatalf("V15 partial checkpoint count=%d error=%v", count, err)
	}
}

func TestVersionSixteenEvaluatorIsFrozenPublicOnlyAndStreamsSynthesisHash(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V16_CONTRACT.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V16_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                              string `json:"schema"`
		Version                             int    `json:"version"`
		FreezeParentCommit                  string `json:"freeze_parent_commit"`
		EvaluatorManifest                   string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256             string `json:"evaluator_manifest_sha256"`
		V15RetirementSHA256                 string `json:"v15_retirement_sha256"`
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
		CompleteOutputSemanticsPreserved    bool   `json:"complete_output_semantics_preserved"`
		BoundedProposalSizingGeneration     bool   `json:"bounded_proposal_sizing_generation"`
		VariantsBeyondLimitMaterialized     bool   `json:"whole_graph_value_variants_materialized_beyond_limit"`
		StreamedCanonicalRepairHashing      bool   `json:"streamed_canonical_repair_hashing"`
		CompleteRepairHashBufferRetained    bool   `json:"complete_repair_hash_buffer_retained"`
		StreamedCanonicalSynthesisHashing   bool   `json:"streamed_canonical_synthesis_hashing"`
		CompleteSynthesisHashBufferRetained bool   `json:"complete_synthesis_hash_buffer_retained"`
		ImmutableV10CorpusReused            bool   `json:"immutable_v10_corpus_reused"`
		V15CheckpointsReused                bool   `json:"v15_checkpoints_reused"`
		ProductionPath                      bool   `json:"production_path"`
		HeldOutAccessSurface                bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated                 bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened                   bool   `json:"external_key_opened"`
	}
	decodeV11Strict(t, data, &freeze)
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v16" || freeze.Version != 16 ||
		freeze.FreezeParentCommit != "68a1f0beae6fb1ec33ee9c6af491e4af670ddaff" || freeze.EvaluatorManifest != "V16_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V15RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V15_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V16 freeze identity: %+v", freeze)
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 || freeze.MaximumLiveReplays != 1 ||
		!freeze.PostReplayFullGC || !freeze.PostReplayOSMemoryRelease || freeze.CompleteReplayBufferRetained ||
		!freeze.LazyValueTrialMaterialization || freeze.EagerValueTrialGraphsRetained || !freeze.ValueTrialOrderPreserved ||
		freeze.BestFailureGraphsPerTopology != 1 || freeze.AllFailedGraphsRetained || !freeze.FailureRankingOrderPreserved ||
		!freeze.CompleteOutputSemanticsPreserved || !freeze.BoundedProposalSizingGeneration || freeze.VariantsBeyondLimitMaterialized ||
		!freeze.StreamedCanonicalRepairHashing || freeze.CompleteRepairHashBufferRetained ||
		!freeze.StreamedCanonicalSynthesisHashing || freeze.CompleteSynthesisHashBufferRetained ||
		!freeze.ImmutableV10CorpusReused || freeze.V15CheckpointsReused ||
		!freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V16 freeze does not enforce its bounded public-only evaluator")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
}
