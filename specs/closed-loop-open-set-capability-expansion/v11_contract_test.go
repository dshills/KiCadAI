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

func TestVersionTenGenerationZeroRetirementIsAuthenticatedAndFailClosed(t *testing.T) {
	directory := v9BaselinePublisherContractDirectory(t)
	data := v9BaselinePublisherReadFile(t, filepath.Join(directory, "V10_GENERATION_ZERO_RETIREMENT.json"))
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
	if retirement.Schema != "kicadai.closed-loop-open-set-generation-zero-retirement.v10" || retirement.Version != 10 ||
		retirement.Stage != "public_generation_zero" || retirement.Reason != "frozen_evaluator_resource_exhaustion" ||
		retirement.FailureClass != "host_memory_pressure_sigkill" || retirement.TerminalState != "permanently_retired" {
		t.Fatalf("invalid V10 retirement state: %+v", retirement)
	}
	if retirement.RequiredCaseCount != 24 || retirement.CompletedCaseCheckpoints != 21 ||
		retirement.CompletedCaseCheckpoints >= retirement.RequiredCaseCount || retirement.ReplaysPerCase != 2 || retirement.ParallelCaseLimit != 4 {
		t.Fatalf("invalid V10 incomplete-run counts: %+v", retirement)
	}
	if retirement.AcceptedReportPublished || retirement.PublicBaselinePublished || retirement.HeldOutSourceOpened ||
		retirement.HeldOutBaselineKeyCreated || retirement.HeldOutBaselineKeyOpened || retirement.PartialEvidenceAccepted ||
		!retirement.HeldOutSourceKeyCreated {
		t.Fatal("V10 retirement crossed its fail-closed evidence boundary")
	}
	if retirement.RepositoryCommit != "de363cbe7298f5312b0121b22f1c893dd60924b7" ||
		retirement.EvaluatorFreezeCommit != "afcb4dad7467c8f7e44170fff25e937a30b16f3d" ||
		retirement.ContractManifestSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V10_CONTRACT.sha256")) ||
		retirement.EvaluatorManifestSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V10_EVALUATOR.sha256")) ||
		retirement.PartialCheckpointsSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, "V10_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256")) {
		t.Fatal("V10 retirement bindings do not reproduce")
	}
	rootBytes := v9BaselinePublisherReadFile(t, filepath.Join(directory, "V10_GENERATION_ZERO_ROOT.json"))
	rootDigest := sha256.Sum256(bytes.TrimSuffix(rootBytes, []byte{'\n'}))
	if hex.EncodeToString(rootDigest[:]) != retirement.EvaluationRootSHA256 {
		t.Fatal("V10 evaluation-root commitment does not reproduce")
	}
	corpusDirectory := filepath.Clean(filepath.Join(directory, "..", "..", "internal", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if retirement.CorpusManifestSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(corpusDirectory, "MANIFEST.json")) ||
		retirement.CorpusChecksumsSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(corpusDirectory, "CHECKSUMS.sha256")) {
		t.Fatal("V10 retirement corpus bindings do not reproduce")
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
	if got := hex.EncodeToString(digest[:]); got != retirement.Hash {
		t.Fatalf("V10 retirement hash = %s, want %s", retirement.Hash, got)
	}
}

func TestVersionTenPartialCheckpointCommitmentIsIncompleteAndOutcomeFree(t *testing.T) {
	directory := v9BaselinePublisherContractDirectory(t)
	data := v9BaselinePublisherReadFile(t, filepath.Join(directory, "V10_GENERATION_ZERO_PARTIAL_CHECKPOINTS.sha256"))
	linePattern := regexp.MustCompile(`^([0-9a-f]{64})  checkpoints/v10_case_([0-9]{3})\.json$`)
	seen := map[int]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		match := linePattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			t.Fatalf("invalid checkpoint commitment line %q", scanner.Text())
		}
		caseNumber, err := strconv.Atoi(match[2])
		if err != nil || caseNumber < 1 || caseNumber > 24 || seen[caseNumber] {
			t.Fatalf("invalid or duplicate checkpoint case %q", match[2])
		}
		seen[caseNumber] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 21 || seen[13] || seen[20] || seen[22] {
		t.Fatalf("unexpected V10 checkpoint frontier: %v", seen)
	}
	if strings.Contains(string(data), "outcome") || strings.Contains(string(data), "status") || strings.Contains(string(data), "observation") {
		t.Fatal("partial checkpoint commitment discloses outcome fields")
	}
}

func TestVersionElevenCorrectiveContractPreservesCorpusAndRestartsEvaluation(t *testing.T) {
	directory := v9BaselinePublisherContractDirectory(t)
	files := []string{"V11_SPEC_ADDENDUM.md", "V11_CORPUS_RULES.md", "V11_BASELINE_PROTOCOL.md", "V11_PLAN.md"}
	var combined strings.Builder
	for _, name := range files {
		combined.Write(v9BaselinePublisherReadFile(t, filepath.Join(directory, name)))
	}
	text := combined.String()
	required := []string{
		"V10_GENERATION_ZERO_RETIREMENT.json",
		"0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94",
		"24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf",
		"all 24 discovery cases",
		"exactly two sequential replays",
		"four case workers",
		"does not resume or copy V10 checkpoints",
		"no worker may retain two complete replay values",
		"byte-for-byte",
		"held-out",
		"fail closed",
	}
	for _, phrase := range required {
		if !strings.Contains(text, phrase) {
			t.Fatalf("V11 corrective contract missing %q", phrase)
		}
	}
}

func decodeV11Strict(t *testing.T, data []byte, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("JSON contains trailing content")
	}
}
