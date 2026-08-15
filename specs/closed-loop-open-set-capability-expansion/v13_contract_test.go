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

func TestVersionTwelveGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V12_GENERATION_ZERO_RETIREMENT.json"))
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
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v12" || retirement.Version != 12 ||
		retirement.Stage != "public_generation_zero" || retirement.RepositoryCommit != "8881364e91944dc35aceefd8811d16a3fac95f05" ||
		retirement.EvaluatorFreezeCommit != retirement.RepositoryCommit || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "host_memory_pressure_sigkill" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V12 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCaseCheckpoints != 21 || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 2 ||
		retirement.AcceptedReportPublished || retirement.PublicBaselinePublished || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineKeyOpened || retirement.PartialEvidenceAccepted {
		t.Fatal("V12 retirement crossed its incomplete public-only boundary")
	}
	if retirement.ContractManifestSHA256 != "49d813015f4f0ced8701b2db5934bdf2566e72afac6efed49c55e64379ab1658" ||
		retirement.EvaluatorManifestSHA256 != "9527f35c75edf8cb877bfaacd1373df0606031d0b675506dbd5a2de1be85be19" ||
		retirement.PartialCheckpointsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V12_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V12 retirement historical bindings do not reproduce")
	}
	root := bytes.TrimSuffix(v7ReadFile(t, filepath.Join(directory, "V12_GENERATION_ZERO_ROOT.json")), []byte{'\n'})
	rootDigest := sha256.Sum256(root)
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V12 evaluation-root commitment does not reproduce")
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if retirement.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		retirement.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V12 retirement corpus bindings do not reproduce")
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
		t.Fatal("V12 retirement self-hash does not reproduce")
	}

	manifest := v7ReadFile(t, filepath.Join(directory, "V12_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))
	pattern := regexp.MustCompile(`^([0-9a-f]{64})  checkpoints/v10_case_([0-9]{3})\.json$`)
	seen := map[int]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		match := pattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			t.Fatalf("invalid V12 checkpoint commitment %q", scanner.Text())
		}
		caseNumber, err := strconv.Atoi(match[2])
		if err != nil || caseNumber < 1 || caseNumber > 24 || seen[caseNumber] {
			t.Fatalf("invalid V12 checkpoint number %q", match[2])
		}
		seen[caseNumber] = true
	}
	if err := scanner.Err(); err != nil || len(seen) != 21 {
		t.Fatalf("V12 partial checkpoint manifest count=%d error=%v", len(seen), err)
	}
	for _, missing := range []int{22, 23, 24} {
		if seen[missing] {
			t.Fatalf("V12 incomplete case %03d was accepted", missing)
		}
	}
}

func TestVersionThirteenEvaluatorIsFrozenPublicOnlyAndMemoryReleased(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V13_CONTRACT.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V13_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                       string `json:"schema"`
		Version                      int    `json:"version"`
		FreezeParentCommit           string `json:"freeze_parent_commit"`
		EvaluatorManifest            string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256      string `json:"evaluator_manifest_sha256"`
		V12RetirementSHA256          string `json:"v12_retirement_sha256"`
		CorpusManifestSHA256         string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256        string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount           int    `json:"discovery_case_count"`
		ReplaysPerCase               int    `json:"replays_per_case"`
		MaximumParallelCases         int    `json:"maximum_parallel_cases"`
		MaximumLiveReplaysPerWorker  int    `json:"maximum_live_replays_per_worker"`
		PostReplayFullGC             bool   `json:"post_replay_full_gc"`
		PostReplayOSMemoryRelease    bool   `json:"post_replay_os_memory_release"`
		CompleteReplayBufferRetained bool   `json:"complete_replay_buffer_retained"`
		ImmutableV10CorpusReused     bool   `json:"immutable_v10_corpus_reused"`
		V12CheckpointsReused         bool   `json:"v12_checkpoints_reused"`
		ProductionPath               bool   `json:"production_path"`
		HeldOutAccessSurface         bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated          bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened            bool   `json:"external_key_opened"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&freeze); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("V13 evaluator freeze contains trailing JSON")
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v13" || freeze.Version != 13 ||
		freeze.FreezeParentCommit != "5696054b19207e8f83059fc4c736d6f7d664a4ca" || freeze.EvaluatorManifest != "V13_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V12RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V12_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V13 evaluator freeze: %+v", freeze)
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if freeze.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		freeze.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V13 immutable corpus bindings do not reproduce")
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 1 ||
		freeze.MaximumLiveReplaysPerWorker != 1 || !freeze.PostReplayFullGC || !freeze.PostReplayOSMemoryRelease ||
		freeze.CompleteReplayBufferRetained || !freeze.ImmutableV10CorpusReused || freeze.V12CheckpointsReused ||
		!freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V13 evaluator freeze crosses its pre-evaluation boundary")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.EvaluatorManifest)))
	for _, required := range []string{
		"../../cmd/kicadai-discovery-baseline-v13/main.go",
		"../../internal/capabilityexecutorv10/v13_runner.go",
		"V13_EVALUATOR_PROTOCOL.md",
	} {
		if !strings.Contains(manifest, required) {
			t.Fatalf("V13 evaluator manifest omits %s", required)
		}
	}
	command := string(v7ReadFile(t, filepath.Join(directory, "../../cmd/kicadai-discovery-baseline-v13/main.go")))
	runner := string(v7ReadFile(t, filepath.Join(directory, "../../internal/capabilityexecutorv10/v13_runner.go")))
	if !strings.Contains(command, ".RunV13(") || strings.Contains(command, "OpenHeldOutV10") ||
		!strings.Contains(runner, "const v13ParallelCaseLimit = 1") || !strings.Contains(runner, "debug.FreeOSMemory()") ||
		!strings.Contains(runner, "releaseReplayMemoryV13()") || strings.Contains(runner, "RunV12(") {
		t.Fatal("V13 command does not bind the single-worker memory-release public path")
	}
}
