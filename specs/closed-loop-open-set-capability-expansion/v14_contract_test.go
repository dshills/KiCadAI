package closedloopopensetcontract

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestVersionThirteenGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V13_GENERATION_ZERO_RETIREMENT.json"))
	var retirement struct {
		Schema                    string `json:"schema"`
		Version                   int    `json:"version"`
		Stage                     string `json:"stage"`
		RepositoryCommit          string `json:"repository_commit"`
		EvaluatorFreezeCommit     string `json:"evaluator_freeze_commit"`
		ContractManifestSHA256    string `json:"contract_manifest_sha256"`
		EvaluatorManifestSHA256   string `json:"evaluator_manifest_sha256"`
		CorpusManifestSHA256      string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256     string `json:"corpus_checksums_sha256"`
		EnvironmentSHA256         string `json:"environment_sha256"`
		EvaluationRootSHA256      string `json:"evaluation_root_sha256"`
		PartialCheckpointsSHA256  string `json:"partial_checkpoints_sha256"`
		RequiredCaseCount         int    `json:"required_case_count"`
		CompletedCaseCheckpoints  int    `json:"completed_case_checkpoints"`
		ReplaysPerCase            int    `json:"replays_per_case"`
		ParallelCaseLimit         int    `json:"parallel_case_limit"`
		AcceptedReportPublished   bool   `json:"accepted_report_published"`
		PublicBaselinePublished   bool   `json:"public_baseline_published"`
		HeldOutSourceKeyCreated   bool   `json:"held_out_source_key_created"`
		HeldOutSourceOpened       bool   `json:"held_out_source_opened"`
		HeldOutBaselineKeyCreated bool   `json:"held_out_baseline_key_created"`
		HeldOutBaselineKeyOpened  bool   `json:"held_out_baseline_key_opened"`
		PartialEvidenceAccepted   bool   `json:"partial_evidence_accepted"`
		Reason                    string `json:"reason"`
		FailureClass              string `json:"failure_class"`
		TerminalState             string `json:"terminal_state"`
		Hash                      string `json:"hash"`
	}
	decodeV11Strict(t, data, &retirement)
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v13" || retirement.Version != 13 ||
		retirement.Stage != "public_generation_zero" || retirement.RepositoryCommit != "94345d12b04c6db9555d9ca7f8a86cd453ab851c" ||
		retirement.EvaluatorFreezeCommit != retirement.RepositoryCommit || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "host_memory_pressure_sigkill" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V13 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCaseCheckpoints != 21 || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 1 ||
		retirement.AcceptedReportPublished || retirement.PublicBaselinePublished || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineKeyOpened || retirement.PartialEvidenceAccepted {
		t.Fatal("V13 retirement crossed its incomplete public-only boundary")
	}
	if retirement.ContractManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V13_CONTRACT.sha256")) ||
		retirement.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V13_EVALUATOR.sha256")) ||
		retirement.PartialCheckpointsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V13_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V13 retirement historical bindings do not reproduce")
	}
	root := bytes.TrimSuffix(v7ReadFile(t, filepath.Join(directory, "V13_GENERATION_ZERO_ROOT.json")), []byte{'\n'})
	rootDigest := sha256.Sum256(root)
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V13 evaluation-root commitment does not reproduce")
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if retirement.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		retirement.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V13 retirement corpus bindings do not reproduce")
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
		t.Fatal("V13 retirement self-hash does not reproduce")
	}

	manifest := v7ReadFile(t, filepath.Join(directory, "V13_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))
	pattern := regexp.MustCompile(`^([0-9a-f]{64})  checkpoints/v10_case_([0-9]{3})\.json$`)
	seen := map[int]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		match := pattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			t.Fatalf("invalid V13 checkpoint commitment %q", scanner.Text())
		}
		caseNumber, parseErr := strconv.Atoi(match[2])
		if parseErr != nil || caseNumber < 1 || caseNumber > 24 || seen[caseNumber] {
			t.Fatalf("invalid V13 checkpoint number %q", match[2])
		}
		seen[caseNumber] = true
	}
	if err := scanner.Err(); err != nil || len(seen) != 21 {
		t.Fatalf("V13 partial checkpoint manifest count=%d error=%v", len(seen), err)
	}
	for _, missing := range []int{22, 23, 24} {
		if seen[missing] {
			t.Fatalf("V13 incomplete case %03d was accepted", missing)
		}
	}
}

func TestVersionFourteenEvaluatorIsFrozenPublicOnlyAndLazyMaterialized(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V14_CONTRACT.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V14_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                             string `json:"schema"`
		Version                            int    `json:"version"`
		FreezeParentCommit                 string `json:"freeze_parent_commit"`
		EvaluatorManifest                  string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256            string `json:"evaluator_manifest_sha256"`
		V13RetirementSHA256                string `json:"v13_retirement_sha256"`
		CorpusManifestSHA256               string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256              string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount                 int    `json:"discovery_case_count"`
		ReplaysPerCase                     int    `json:"replays_per_case"`
		MaximumParallelCases               int    `json:"maximum_parallel_cases"`
		MaximumLiveReplaysPerWorker        int    `json:"maximum_live_replays_per_worker"`
		PostReplayFullGC                   bool   `json:"post_replay_full_gc"`
		PostReplayOSMemoryRelease          bool   `json:"post_replay_os_memory_release"`
		CompleteReplayBufferRetained       bool   `json:"complete_replay_buffer_retained"`
		LazyValueTrialGraphMaterialization bool   `json:"lazy_value_trial_graph_materialization"`
		EagerValueTrialGraphsRetained      bool   `json:"eager_value_trial_graphs_retained"`
		ValueTrialValidationOrderPreserved bool   `json:"value_trial_validation_order_preserved"`
		ImmutableV10CorpusReused           bool   `json:"immutable_v10_corpus_reused"`
		V13CheckpointsReused               bool   `json:"v13_checkpoints_reused"`
		ProductionPath                     bool   `json:"production_path"`
		HeldOutAccessSurface               bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated                bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened                  bool   `json:"external_key_opened"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&freeze); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("V14 evaluator freeze contains trailing JSON")
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v14" || freeze.Version != 14 ||
		freeze.FreezeParentCommit != "94345d12b04c6db9555d9ca7f8a86cd453ab851c" || freeze.EvaluatorManifest != "V14_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V13RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V13_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V14 evaluator freeze: %+v", freeze)
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if freeze.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		freeze.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V14 immutable corpus bindings do not reproduce")
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 ||
		freeze.MaximumLiveReplaysPerWorker != 1 || !freeze.PostReplayFullGC || !freeze.PostReplayOSMemoryRelease ||
		freeze.CompleteReplayBufferRetained || !freeze.LazyValueTrialGraphMaterialization || freeze.EagerValueTrialGraphsRetained ||
		!freeze.ValueTrialValidationOrderPreserved || !freeze.ImmutableV10CorpusReused || freeze.V13CheckpointsReused ||
		!freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V14 evaluator freeze crosses its pre-evaluation boundary")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.EvaluatorManifest)))
	for _, required := range []string{
		"../../cmd/kicadai-discovery-baseline-v14/main.go",
		"../../internal/capabilityexecutorv10/v14_executor.go",
		"../../internal/capabilityexecutorv10/v14_runner.go",
		"../../internal/opentopologysynthesis/synthesis_v14.go",
		"../../internal/opentopologysynthesis/synthesis_v14_test.go",
		"V14_EVALUATOR_PROTOCOL.md",
	} {
		if !strings.Contains(manifest, required) {
			t.Fatalf("V14 evaluator manifest omits %s", required)
		}
	}
	command := string(v7ReadFile(t, filepath.Join(directory, "../../cmd/kicadai-discovery-baseline-v14/main.go")))
	runner := string(v7ReadFile(t, filepath.Join(directory, "../../internal/capabilityexecutorv10/v14_runner.go")))
	synthesis := string(v7ReadFile(t, filepath.Join(directory, "../../internal/opentopologysynthesis/synthesis_v14.go")))
	if !strings.Contains(command, "NewV14().RunV14(") || strings.Contains(command, "OpenHeldOutV10") ||
		!strings.Contains(runner, "const v14ParallelCaseLimit = 1") || !strings.Contains(runner, "debug.FreeOSMemory()") ||
		!strings.Contains(runner, "releaseReplayMemoryV14()") || strings.Contains(runner, "RunV13(") ||
		!strings.Contains(synthesis, "trial          ValueTrial") || strings.Contains(synthesis, "trial          *ValueTrial\n\tgraph") ||
		!strings.Contains(synthesis, "run.Search.Candidates[candidateIndex].Graph, work.trial") {
		t.Fatal("V14 command does not bind the lazy-graph single-worker public path")
	}
}
