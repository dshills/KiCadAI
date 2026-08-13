package capabilityfeedback

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityroundsv8"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV8BaselineRoot      = "testdata/closed_loop_open_set_v8_baseline"
	closedLoopV8BaselineUpdateEnv = "UPDATE_CLOSED_LOOP_V8_DISCOVERY_BASELINE"
	closedLoopV8BaselineSchema    = "kicadai.closed-loop-open-set-discovery-baseline.v8"
	closedLoopV8FrontierSchema    = "kicadai.closed-loop-open-set-frontier.v8"
	closedLoopV8ArtifactSchema    = "kicadai.closed-loop-open-set-discovery-case.v8"
)

type closedLoopV8BaselineReport struct {
	Schema                     string                           `json:"schema"`
	Version                    int                              `json:"version"`
	CorpusManifestSHA256       string                           `json:"corpus_manifest_sha256"`
	InfrastructureCommit       string                           `json:"infrastructure_commit"`
	EvaluatorManifestSHA256    string                           `json:"evaluator_manifest_sha256"`
	DiscoveryObligationsSHA256 string                           `json:"discovery_obligations_sha256"`
	EvaluatorPolicy            string                           `json:"evaluator_policy"`
	InventorySHA256            string                           `json:"inventory_sha256"`
	CatalogSHA256              string                           `json:"catalog_sha256"`
	ModelRegistrySHA256        string                           `json:"model_registry_sha256"`
	SynthesisPolicySHA256      string                           `json:"synthesis_policy_sha256"`
	PromotionEnvironment       closedLoopV5PromotionEnvironment `json:"promotion_environment"`
	OutcomeCounts              []closedLoopOutcomeCount         `json:"outcome_counts"`
	CaseArtifacts              []closedLoopV8ArtifactRef        `json:"case_artifacts"`
	FrontierSHA256             string                           `json:"frontier_sha256"`
	Discovery                  AggregateReport                  `json:"discovery"`
	Hash                       string                           `json:"hash"`
}

type closedLoopV8CaseArtifact struct {
	Schema            string                       `json:"schema"`
	Version           int                          `json:"version"`
	CaseID            string                       `json:"case_id"`
	RequirementSHA256 string                       `json:"requirement_sha256"`
	Replays           []ots.SynthesisRun           `json:"replays"`
	Promotion         *ots.PhysicalPromotionResult `json:"promotion,omitempty"`
	Observation       CaseEvidence                 `json:"observation"`
	Hash              string                       `json:"hash"`
}

type closedLoopV8ArtifactRef struct {
	CaseID string `json:"case_id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopV8FrontierCase struct {
	Case                 capabilityroundsv8.Case  `json:"case"`
	CandidateInventory   []ots.CandidateReport    `json:"candidate_inventory"`
	SuppressedFailures   []ots.SelectionRejection `json:"suppressed_failures"`
	SynthesisDiagnostics []ots.Diagnostic         `json:"synthesis_diagnostics"`
}

type closedLoopV8Frontier struct {
	Schema     string                     `json:"schema"`
	Version    int                        `json:"version"`
	Generation int                        `json:"generation"`
	Cases      []closedLoopV8FrontierCase `json:"cases"`
	Hash       string                     `json:"hash"`
}

func TestClosedLoopV8DiscoveryBaselineIsStructurallyFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV8BaselineRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V8 discovery baseline has not been published")
		}
		t.Fatal(err)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV8BaselineRoot, filepath.Join(closedLoopV8BaselineRoot, "CHECKSUMS.sha256")); err != nil {
		t.Fatal(err)
	}
	var report closedLoopV8BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "report.json")), &report)
	var frontier closedLoopV8Frontier
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "frontier.json")), &frontier)
	reportHash, reportErr := hashClosedLoopV8BaselineReport(report)
	frontierHash, frontierErr := hashClosedLoopV8Frontier(frontier)
	if reportErr != nil || frontierErr != nil || report.Hash != reportHash || frontier.Hash != frontierHash ||
		report.FrontierSHA256 != frontier.Hash || report.CorpusManifestSHA256 != closedLoopV8CorpusManifestHash ||
		len(report.CaseArtifacts) != closedLoopV8RoleSize || len(frontier.Cases) != closedLoopV8RoleSize {
		t.Fatal("V8 discovery baseline is not self-consistent")
	}
	assertClosedLoopV8Artifacts(t, report.CaseArtifacts)
}

func TestUpdateClosedLoopV8DiscoveryBaseline(t *testing.T) {
	if os.Getenv(closedLoopV8BaselineUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V8_DISCOVERY_BASELINE=1 to publish generation zero")
	}
	if _, err := os.Stat(closedLoopV8BaselineRoot); !os.IsNotExist(err) {
		t.Fatal("V8 discovery baseline already exists; refusing overwrite")
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	infrastructureCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	manifestSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ManifestFileV8))
	obligationSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.DiscoveryObligationsFileV8))
	var manifest corpuspublication.ManifestV8
	decodeCorpusStrict(t, manifestSource, &manifest)
	registry, synthesisPolicy := closedLoopV4Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV8PromotionEnvironment(t, repositoryRoot)
	requirementSources := loadClosedLoopV8DiscoveryRequirements(t, manifest)
	artifacts := runClosedLoopV8Discovery(t, manifest, synthesisPolicy, inventory, environment, promotionEnvironment)
	evidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		evidence[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, evidence, registry)
	if err != nil {
		t.Fatal(err)
	}
	roundCases, err := BuildV8DiscoveryRoundCases(manifestSource, obligationSource, requirementSources, discovery, registry)
	if err != nil {
		t.Fatal(err)
	}
	frontier := buildClosedLoopV8Frontier(t, artifacts, roundCases)
	evaluatorHash := corpusHash(mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V8_EVALUATOR.sha256")))
	obligationHash := corpusHash(obligationSource)
	if err := atomicdir.Publish(closedLoopV8BaselineRoot, func(root string) error {
		refs, writeErr := writeClosedLoopV8CaseArtifacts(root, artifacts)
		if writeErr != nil {
			return writeErr
		}
		inventoryHash, catalogHash, modelHash, policyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
		report := closedLoopV8BaselineReport{
			Schema: closedLoopV8BaselineSchema, Version: 8, CorpusManifestSHA256: closedLoopV8CorpusManifestHash,
			InfrastructureCommit: infrastructureCommit, EvaluatorManifestSHA256: evaluatorHash,
			DiscoveryObligationsSHA256: obligationHash, EvaluatorPolicy: RealizabilityPolicyVersion,
			InventorySHA256: inventoryHash, CatalogSHA256: catalogHash, ModelRegistrySHA256: modelHash,
			SynthesisPolicySHA256: policyHash, PromotionEnvironment: promotionEnvironment.Public,
			OutcomeCounts: closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs,
			FrontierSHA256: frontier.Hash, Discovery: discovery,
		}
		report.Hash, writeErr = hashClosedLoopV8BaselineReport(report)
		if writeErr != nil {
			return writeErr
		}
		for path, data := range map[string][]byte{
			"report.json": corpusJSON(t, report), "frontier.json": corpusJSON(t, frontier),
			"BASELINE_AUDIT.md": []byte(fmt.Sprintf("# V8 Discovery Baseline Audit\n\nGeneration zero evaluated %d public discovery cases twice and anchored every root frontier to the authenticated V8 obligation set. Held-out source and keys were not opened.\n\n- infrastructure commit: `%s`\n- report hash: `%s`\n- frontier hash: `%s`\n", len(artifacts), infrastructureCommit, report.Hash, frontier.Hash)),
		} {
			if writeErr = os.WriteFile(filepath.Join(root, path), data, 0o644); writeErr != nil {
				return writeErr
			}
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
}

func loadClosedLoopV8DiscoveryRequirements(t *testing.T, manifest corpuspublication.ManifestV8) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, filepath.FromSlash(entry.StablePath)))
		if corpusHash(data) != entry.RequirementSHA256 {
			t.Fatalf("V8 discovery requirement %s differs from its raw commitment", entry.ID)
		}
		_, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("V8 discovery requirement %s violates the frozen contract", entry.ID)
		}
		result[entry.ID] = data
	}
	return result
}

func runClosedLoopV8Discovery(t *testing.T, manifest corpuspublication.ManifestV8, policy ots.Policy, inventory ots.PrimitiveInventory, environment ots.SimulationEnvironment, promotionEnvironment closedLoopV5ResolvedPromotionEnvironment) []closedLoopV8CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV8CaseArtifact, 0, closedLoopV8RoleSize)
	for _, entry := range manifest.Entries {
		t.Logf("V8 discovery baseline %s starting", entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, filepath.FromSlash(entry.StablePath)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s violates the frozen requirement contract", entry.ID)
		}
		first := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		second := runClosedLoopV5SealedSynthesis(t, requirement, inventory, environment, policy)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil || !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs", entry.ID)
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			current := promoteClosedLoopV5SealedRun(t, first, environment, promotionEnvironment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatalf("%s did not pass two clean-root installed-KiCad promotions", entry.ID)
			}
			promotion = &current
		}
		feedbackDomain, err := v8FeedbackDomain(entry.Domain)
		if err != nil {
			t.Fatal(err)
		}
		observation, err := ObserveRealizabilityAware(CaseMeta{ID: entry.ID, Role: RoleDiscovery, Domain: feedbackDomain, SafetyImpact: capabilityevaluation.SafetyImpact(entry.SafetyImpact)}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observation failed: %v", entry.ID, err)
		}
		artifact := closedLoopV8CaseArtifact{Schema: closedLoopV8ArtifactSchema, Version: 8, CaseID: entry.ID, RequirementSHA256: entry.RequirementSHA256, Replays: []ots.SynthesisRun{first, second}, Promotion: promotion, Observation: observation}
		artifact.Hash, err = hashClosedLoopV8CaseArtifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		artifacts = append(artifacts, artifact)
		t.Logf("V8 discovery baseline %s outcome=%s gaps=%d", entry.ID, observation.Outcome, len(observation.Gaps))
	}
	if len(artifacts) != closedLoopV8RoleSize {
		t.Fatalf("V8 discovery baseline case count = %d", len(artifacts))
	}
	return artifacts
}

func buildClosedLoopV8Frontier(t *testing.T, artifacts []closedLoopV8CaseArtifact, cases []capabilityroundsv8.Case) closedLoopV8Frontier {
	t.Helper()
	byID := map[string]capabilityroundsv8.Case{}
	for _, current := range cases {
		byID[current.ID] = current
	}
	frontier := closedLoopV8Frontier{Schema: closedLoopV8FrontierSchema, Version: 8, Generation: 0}
	for _, artifact := range artifacts {
		report := artifact.Replays[0].Report
		var suppressed []ots.SelectionRejection
		if report.Selected != nil {
			suppressed = append(suppressed, report.Selected.Ranking.Rejections...)
		}
		frontier.Cases = append(frontier.Cases, closedLoopV8FrontierCase{Case: byID[artifact.CaseID], CandidateInventory: append([]ots.CandidateReport(nil), report.Candidates...), SuppressedFailures: suppressed, SynthesisDiagnostics: append([]ots.Diagnostic(nil), report.Diagnostics...)})
	}
	var err error
	frontier.Hash, err = hashClosedLoopV8Frontier(frontier)
	if err != nil {
		t.Fatal(err)
	}
	return frontier
}

func resolveClosedLoopV8PromotionEnvironment(t *testing.T, repositoryRoot string) closedLoopV5ResolvedPromotionEnvironment {
	t.Helper()
	resolved := resolveClosedLoopV5PromotionEnvironment(t, repositoryRoot)
	resolved.Public.Schema = "kicadai.closed-loop-open-set-promotion-environment.v8"
	resolved.Public.Version = 8
	var err error
	resolved.Public.Hash, err = hashClosedLoopV5PromotionEnvironment(resolved.Public)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func assertClosedLoopV8Artifacts(t *testing.T, refs []closedLoopV8ArtifactRef) {
	t.Helper()
	for _, ref := range refs {
		data := mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V8 discovery artifact %s differs from its file commitment", ref.CaseID)
		}
		compressed, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		var artifact closedLoopV8CaseArtifact
		decoder := json.NewDecoder(compressed)
		decoder.DisallowUnknownFields()
		decodeErr := decoder.Decode(&artifact)
		trailingErr := decoder.Decode(&struct{}{})
		closeErr := compressed.Close()
		expected, hashErr := hashClosedLoopV8CaseArtifact(artifact)
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || hashErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV8ArtifactSchema || artifact.Version != 8 || len(artifact.Replays) != 2 {
			t.Fatalf("V8 discovery artifact %s is invalid", ref.CaseID)
		}
		first, firstErr := json.Marshal(artifact.Replays[0])
		second, secondErr := json.Marshal(artifact.Replays[1])
		if firstErr != nil || secondErr != nil || !bytes.Equal(first, second) {
			t.Fatalf("V8 discovery artifact %s is not deterministic", ref.CaseID)
		}
	}
}

func writeClosedLoopV8CaseArtifacts(root string, artifacts []closedLoopV8CaseArtifact) ([]closedLoopV8ArtifactRef, error) {
	discoveryRoot := filepath.Join(root, "discovery")
	if err := os.Mkdir(discoveryRoot, 0o755); err != nil {
		return nil, err
	}
	refs := make([]closedLoopV8ArtifactRef, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json.gz"))
		digest, err := writeClosedLoopV8CompressedArtifact(filepath.Join(root, filepath.FromSlash(path)), artifact)
		if err != nil {
			return nil, err
		}
		refs = append(refs, closedLoopV8ArtifactRef{CaseID: artifact.CaseID, Path: path, SHA256: digest})
	}
	return refs, nil
}

func writeClosedLoopV8CompressedArtifact(path string, artifact closedLoopV8CaseArtifact) (digest string, err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	hash := sha256.New()
	compressed, err := gzip.NewWriterLevel(io.MultiWriter(file, hash), gzip.BestCompression)
	if err != nil {
		return "", err
	}
	compressed.Header.ModTime = time.Unix(0, 0).UTC()
	compressed.Header.OS = 255
	if err = json.NewEncoder(compressed).Encode(artifact); err != nil {
		_ = compressed.Close()
		return "", fmt.Errorf("encode compressed V8 evidence %s: %w", artifact.CaseID, err)
	}
	if err = compressed.Close(); err != nil {
		return "", fmt.Errorf("close compressed V8 evidence %s: %w", artifact.CaseID, err)
	}
	if err = file.Sync(); err != nil {
		return "", fmt.Errorf("sync compressed V8 evidence %s: %w", artifact.CaseID, err)
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("close V8 evidence file %s: %w", artifact.CaseID, err)
	}
	closed = true
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashClosedLoopV8CaseArtifact(value closedLoopV8CaseArtifact) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV8BaselineReport(value closedLoopV8BaselineReport) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV8Frontier(value closedLoopV8Frontier) (string, error) {
	value.Hash = ""
	return digest(value)
}
