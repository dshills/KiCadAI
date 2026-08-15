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

func TestVersionElevenGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V11_GENERATION_ZERO_RETIREMENT.json"))
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
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v11" || retirement.Version != 11 ||
		retirement.Stage != "public_generation_zero" || retirement.RepositoryCommit != "9e349b6afc7c47726a46f751ffd7a7dfa95c840e" ||
		retirement.EvaluatorFreezeCommit != retirement.RepositoryCommit || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "host_memory_pressure_sigkill" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V11 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCaseCheckpoints != 19 || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 4 ||
		retirement.AcceptedReportPublished || retirement.PublicBaselinePublished || retirement.HeldOutSourceKeyCreated || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineKeyOpened || retirement.PartialEvidenceAccepted {
		t.Fatal("V11 retirement crossed its incomplete public-only boundary")
	}
	if retirement.ContractManifestSHA256 != "06a759c48515eafc179e7f7dc34a211cfbcd1eb8b1f68c5547ecd08f859e3755" ||
		retirement.EvaluatorManifestSHA256 != "3d2c7835d2928a60cf8793b8633ed5def01533ddf2b6220753fbbbdbb8d9431f" ||
		retirement.PartialCheckpointsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V11_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V11 retirement historical bindings do not reproduce")
	}
	root := bytes.TrimSuffix(v7ReadFile(t, filepath.Join(directory, "V11_GENERATION_ZERO_ROOT.json")), []byte{'\n'})
	rootDigest := sha256.Sum256(root)
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V11 evaluation-root commitment does not reproduce")
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if retirement.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		retirement.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V11 retirement corpus bindings do not reproduce")
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
		t.Fatal("V11 retirement self-hash does not reproduce")
	}

	manifest := v7ReadFile(t, filepath.Join(directory, "V11_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))
	pattern := regexp.MustCompile(`^([0-9a-f]{64})  checkpoints/v10_case_([0-9]{3})\.json$`)
	seen := map[int]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		match := pattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			t.Fatalf("invalid V11 checkpoint commitment %q", scanner.Text())
		}
		caseNumber, err := strconv.Atoi(match[2])
		if err != nil || caseNumber < 1 || caseNumber > 24 || seen[caseNumber] {
			t.Fatalf("invalid V11 checkpoint number %q", match[2])
		}
		seen[caseNumber] = true
	}
	if err := scanner.Err(); err != nil || len(seen) != 19 {
		t.Fatalf("V11 partial checkpoint manifest count=%d error=%v", len(seen), err)
	}
	for _, missing := range []int{13, 20, 22, 23, 24} {
		if seen[missing] {
			t.Fatalf("V11 incomplete case %03d was accepted", missing)
		}
	}
}

func TestVersionTwelveEvaluatorIsFrozenPublicOnlyAndAggregateMemoryBounded(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, directory, "V12_CONTRACT.sha256")
	data := v7ReadFile(t, filepath.Join(directory, "V12_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                       string `json:"schema"`
		Version                      int    `json:"version"`
		FreezeParentCommit           string `json:"freeze_parent_commit"`
		EvaluatorManifest            string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256      string `json:"evaluator_manifest_sha256"`
		V11RetirementSHA256          string `json:"v11_retirement_sha256"`
		CorpusManifestSHA256         string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256        string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount           int    `json:"discovery_case_count"`
		ReplaysPerCase               int    `json:"replays_per_case"`
		MaximumParallelCases         int    `json:"maximum_parallel_cases"`
		MaximumLiveReplaysPerWorker  int    `json:"maximum_live_replays_per_worker"`
		CompleteReplayBufferRetained bool   `json:"complete_replay_buffer_retained"`
		ImmutableV10CorpusReused     bool   `json:"immutable_v10_corpus_reused"`
		V11CheckpointsReused         bool   `json:"v11_checkpoints_reused"`
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
		t.Fatal("V12 evaluator freeze contains trailing JSON")
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v12" || freeze.Version != 12 ||
		freeze.FreezeParentCommit != "a322449d84472e35f9a753a010bdf0066b64a473" || freeze.EvaluatorManifest != "V12_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V11RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V11_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V12 evaluator freeze: %+v", freeze)
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if freeze.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		freeze.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V12 immutable corpus bindings do not reproduce")
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 2 ||
		freeze.MaximumLiveReplaysPerWorker != 1 || freeze.CompleteReplayBufferRetained || !freeze.ImmutableV10CorpusReused ||
		freeze.V11CheckpointsReused || !freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V12 evaluator freeze crosses its pre-evaluation boundary")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
	manifest := string(v7ReadFile(t, filepath.Join(directory, freeze.EvaluatorManifest)))
	for _, required := range []string{
		"../../cmd/kicadai-discovery-baseline-v12/main.go",
		"../../internal/capabilityexecutorv10/v12_runner.go",
		"V12_EVALUATOR_PROTOCOL.md",
	} {
		if !strings.Contains(manifest, required) {
			t.Fatalf("V12 evaluator manifest omits %s", required)
		}
	}
	command := string(v7ReadFile(t, filepath.Join(directory, "../../cmd/kicadai-discovery-baseline-v12/main.go")))
	runner := string(v7ReadFile(t, filepath.Join(directory, "../../internal/capabilityexecutorv10/v12_runner.go")))
	if !strings.Contains(command, ".RunV12(") || strings.Contains(command, "OpenHeldOutV10") ||
		!strings.Contains(runner, "const v12ParallelCaseLimit = 2") || !strings.Contains(runner, "runCaseV12") ||
		strings.Contains(runner, "RunV11(") {
		t.Fatal("V12 command does not bind the aggregate-memory-bounded public path")
	}
}
