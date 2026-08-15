package closedloopopensetcontract

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVersionElevenProductionEvaluatorIsFrozenPublicOnlyAndMemoryBounded(t *testing.T) {
	directory := v7ContractDirectory(t)
	data := v7ReadFile(t, filepath.Join(directory, "V11_EVALUATOR_FREEZE.json"))
	var freeze struct {
		Schema                       string `json:"schema"`
		Version                      int    `json:"version"`
		FreezeParentCommit           string `json:"freeze_parent_commit"`
		EvaluatorManifest            string `json:"evaluator_manifest"`
		EvaluatorManifestSHA256      string `json:"evaluator_manifest_sha256"`
		V10RetirementSHA256          string `json:"v10_retirement_sha256"`
		CorpusManifestSHA256         string `json:"corpus_manifest_sha256"`
		CorpusChecksumsSHA256        string `json:"corpus_checksums_sha256"`
		DiscoveryCaseCount           int    `json:"discovery_case_count"`
		ReplaysPerCase               int    `json:"replays_per_case"`
		MaximumParallelCases         int    `json:"maximum_parallel_cases"`
		MaximumLiveReplaysPerWorker  int    `json:"maximum_live_replays_per_worker"`
		CompleteReplayBufferRetained bool   `json:"complete_replay_buffer_retained"`
		ImmutableV10CorpusReused     bool   `json:"immutable_v10_corpus_reused"`
		V10CheckpointsReused         bool   `json:"v10_checkpoints_reused"`
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
		t.Fatal("V11 evaluator freeze contains trailing JSON")
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-evaluator-freeze.v11" || freeze.Version != 11 ||
		freeze.FreezeParentCommit != "08212cec107b857ae5f24db8e99a86dab0bc81f4" || freeze.EvaluatorManifest != "V11_EVALUATOR.sha256" ||
		freeze.EvaluatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvaluatorManifest)) ||
		freeze.V10RetirementSHA256 != v7FileSHA256(t, filepath.Join(directory, "V10_GENERATION_ZERO_RETIREMENT.json")) {
		t.Fatalf("invalid V11 evaluator freeze: %+v", freeze)
	}
	corpus := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if freeze.CorpusManifestSHA256 != v7FileSHA256(t, filepath.Join(corpus, "manifest.json")) ||
		freeze.CorpusChecksumsSHA256 != v7FileSHA256(t, filepath.Join(corpus, "CHECKSUMS.sha256")) {
		t.Fatal("V11 immutable corpus bindings do not reproduce")
	}
	if freeze.DiscoveryCaseCount != 24 || freeze.ReplaysPerCase != 2 || freeze.MaximumParallelCases != 4 ||
		freeze.MaximumLiveReplaysPerWorker != 1 || freeze.CompleteReplayBufferRetained || !freeze.ImmutableV10CorpusReused ||
		freeze.V10CheckpointsReused || !freeze.ProductionPath || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V11 evaluator freeze crosses its corrective pre-evaluation boundary")
	}
	v8VerifyManifest(t, directory, freeze.EvaluatorManifest)
	assertV11EvaluatorManifestIsCompleteAndPublicOnly(t, directory, freeze.EvaluatorManifest)
}

func assertV11EvaluatorManifestIsCompleteAndPublicOnly(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	listed := map[string]bool{}
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid V11 evaluator manifest line %q", line)
		}
		listed[filepath.ToSlash(fields[1])] = true
		if !strings.HasSuffix(fields[1], ".go") || strings.HasSuffix(fields[1], "_test.go") {
			continue
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(fields[1])))
		if strings.Contains(string(source), "OpenHeldOutV10") || strings.Contains(string(source), "VerifyPublicationV10WithKey") ||
			strings.Contains(string(source), "held-out-source.key") {
			t.Fatalf("V11 public evaluator %s contains a held-out opening surface", fields[1])
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), fields[1], source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "kicadai/internal/blindbaseline" || path == "kicadai/internal/externalkey" {
				t.Fatalf("V11 public evaluator %s imports forbidden package %q", fields[1], path)
			}
		}
	}
	required := []string{
		"../../cmd/kicadai-discovery-baseline-v11/main.go",
		"../../internal/canonicaljsonstream/encode.go",
		"../../internal/capabilityexecutorv10/v11_replay_store.go",
		"../../internal/capabilityexecutorv10/v11_runner.go",
		"V11_EVALUATOR_PROTOCOL.md",
	}
	for _, path := range required {
		if !listed[path] {
			t.Fatalf("V11 evaluator manifest omits %s", path)
		}
	}
	if productionSources != 9 {
		t.Fatalf("V11 evaluator production sources = %d, want 9", productionSources)
	}
	command := string(v7ReadFile(t, filepath.Join(directory, "../../cmd/kicadai-discovery-baseline-v11/main.go")))
	runner := string(v7ReadFile(t, filepath.Join(directory, "../../internal/capabilityexecutorv10/v11_runner.go")))
	store := string(v7ReadFile(t, filepath.Join(directory, "../../internal/capabilityexecutorv10/v11_replay_store.go")))
	if !strings.Contains(command, ".RunV11(") || strings.Contains(command, ".Run(ctx") ||
		!strings.Contains(command, "index.Diagnostics = libraryIssues") || strings.Contains(command, "load library index: blocking") ||
		!strings.Contains(runner, "run = opentopologysynthesis.SynthesisRun{}") || strings.Contains(runner, "runs :=") ||
		!strings.Contains(store, "canonicaljsonstream.Encode") {
		t.Fatal("V11 command does not bind the memory-bounded production path")
	}
}
