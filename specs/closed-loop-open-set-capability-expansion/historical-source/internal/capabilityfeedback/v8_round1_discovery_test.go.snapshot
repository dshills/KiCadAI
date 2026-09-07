package capabilityfeedback

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilityroundsv8"
	"kicadai/internal/corpuspublication"
)

const (
	closedLoopV8Round1UpdateEnv      = "UPDATE_CLOSED_LOOP_V8_ROUND_1_DISCOVERY"
	closedLoopV8Round1Root           = "testdata/closed_loop_open_set_v8_round_1_discovery"
	closedLoopV8Round1RetirementRoot = "testdata/closed_loop_open_set_v8_round_1_retirement"
	closedLoopV8Round1Schema         = "kicadai.closed-loop-open-set-round.v8"
	closedLoopV8Round1RetireSchema   = "kicadai.closed-loop-open-set-retirement.v8"
	closedLoopV8Round1RunnerManifest = "V8_ROUND_1_RUNNER.sha256"
	closedLoopV8Round1AuditTemplate  = "# V8 Round-One Public Discovery Audit\n\nAll 18 public discovery cases were synthesized twice under the frozen environment. Every pass received two clean-root installed-KiCad promotions. Held-out source, baseline plaintext, outcomes, gaps, and keys were not opened.\n\n- status: `%s`\n- discovery passes: %d -> %d\n- new active-cohort passes: %d\n- advanced cases: %d\n- advanced domains: %d\n- advanced circuit roles: %d\n- successors: %d\n- implementation seal: `%s`\n- input frontier: `%s`\n- output frontier: `%s`\n- result hash: `%s`\n"
)

type closedLoopV8RoundResult struct {
	Schema                       string                        `json:"schema"`
	Version                      int                           `json:"version"`
	Generation                   int                           `json:"generation"`
	InfrastructureCommit         string                        `json:"infrastructure_commit"`
	RunnerManifestSHA256         string                        `json:"runner_manifest_sha256"`
	ImplementationCommit         string                        `json:"implementation_commit"`
	ImplementationSealSHA256     string                        `json:"implementation_seal_sha256"`
	ImplementationSealFileSHA256 string                        `json:"implementation_seal_file_sha256"`
	InputSelectionSHA256         string                        `json:"input_selection_sha256"`
	InputFrontierSHA256          string                        `json:"input_frontier_sha256"`
	CorpusManifestSHA256         string                        `json:"corpus_manifest_sha256"`
	SelectionRunnerSHA256        string                        `json:"selection_runner_sha256"`
	CaseArtifacts                []closedLoopV8ArtifactRef     `json:"case_artifacts"`
	Discovery                    AggregateReport               `json:"discovery"`
	Evaluation                   capabilityroundsv8.Evaluation `json:"evaluation"`
	OutputFrontier               closedLoopV8Frontier          `json:"output_frontier"`
	RequiresNextSelection        bool                          `json:"requires_next_selection"`
	Hash                         string                        `json:"hash"`
}

type closedLoopV8RoundRetirement struct {
	Schema                       string `json:"schema"`
	Version                      int    `json:"version"`
	Generation                   int    `json:"generation"`
	InfrastructureCommit         string `json:"infrastructure_commit"`
	RunnerManifestSHA256         string `json:"runner_manifest_sha256"`
	ImplementationCommit         string `json:"implementation_commit"`
	ImplementationSealSHA256     string `json:"implementation_seal_sha256"`
	ImplementationSealFileSHA256 string `json:"implementation_seal_file_sha256"`
	InputSelectionSHA256         string `json:"input_selection_sha256"`
	InputFrontierSHA256          string `json:"input_frontier_sha256"`
	Reason                       string `json:"reason"`
	HeldOutOpened                bool   `json:"held_out_opened"`
	Hash                         string `json:"hash"`
}

type closedLoopV8RoundInputs struct {
	Baseline          closedLoopV8BaselineReport
	Frontier          closedLoopV8Frontier
	Ranking           closedLoopV8Ranking
	Selection         closedLoopV8SelectionDecision
	Implementation    closedLoopV8ImplementationSeal
	ImplementationRaw []byte
	RunnerManifest    []byte
}

func TestClosedLoopV8Round1RunnerIsFrozen(t *testing.T) {
	repositoryRoot := closedLoopModuleRoot(t)
	manifestPath := filepath.Join(closedLoopSpecDirectory(t), closedLoopV8Round1RunnerManifest)
	if _, err := corpuspublication.VerifyChecksumManifest(repositoryRoot, manifestPath); err != nil {
		t.Fatalf("verify V8 round-one runner manifest: %v", err)
	}
}

func TestClosedLoopV8Round1DiscoveryStateIsFrozen(t *testing.T) {
	_, roundErr := os.Stat(closedLoopV8Round1Root)
	_, retirementErr := os.Stat(closedLoopV8Round1RetirementRoot)
	if os.IsNotExist(roundErr) && os.IsNotExist(retirementErr) {
		t.Skip("V8 round-one discovery has not been consumed")
	}
	if roundErr == nil && retirementErr == nil {
		t.Fatal("V8 round-one success and retirement states coexist")
	}
	if roundErr == nil {
		verifyClosedLoopV8Round1Result(t)
		return
	}
	if retirementErr == nil {
		verifyClosedLoopV8Round1Retirement(t)
		return
	}
	if roundErr != nil && !os.IsNotExist(roundErr) {
		t.Fatal(roundErr)
	}
	if retirementErr != nil && !os.IsNotExist(retirementErr) {
		t.Fatal(retirementErr)
	}
}

func TestUpdateClosedLoopV8Round1Discovery(t *testing.T) {
	if os.Getenv(closedLoopV8Round1UpdateEnv) != "1" {
		t.Skip("set " + closedLoopV8Round1UpdateEnv + "=1 to consume the V8 round-one public discovery evaluation")
	}
	for _, root := range []string{closedLoopV8Round1Root, closedLoopV8Round1RetirementRoot} {
		if _, err := os.Stat(root); err == nil {
			t.Fatalf("V8 round-one discovery already consumed at %s", root)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	repositoryRoot := closedLoopModuleRoot(t)
	infrastructureCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	inputs := loadClosedLoopV8RoundInputs(t)
	manifestSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ManifestFileV8))
	obligationSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.DiscoveryObligationsFileV8))
	var manifest corpuspublication.ManifestV8
	decodeCorpusStrict(t, manifestSource, &manifest)
	registry, synthesisPolicy := closedLoopV4Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotion := resolveClosedLoopV8PromotionEnvironment(t, repositoryRoot)
	artifacts := runClosedLoopV8Discovery(t, manifest, synthesisPolicy, inventory, environment, promotion)
	caseEvidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		caseEvidence[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, caseEvidence, registry)
	if err != nil {
		publishClosedLoopV8Round1Retirement(t, infrastructureCommit, inputs, "aggregate_discovery_invalid")
		t.Fatal(err)
	}
	inventoryHash, catalogHash, modelRegistryHash, synthesisPolicyHash := closedLoopV5SealedEnvironmentBindings(t, caseEvidence)
	if inventoryHash != inputs.Baseline.InventorySHA256 || catalogHash != inputs.Baseline.CatalogSHA256 ||
		modelRegistryHash != inputs.Baseline.ModelRegistrySHA256 || synthesisPolicyHash != inputs.Baseline.SynthesisPolicySHA256 ||
		promotion.Public != inputs.Baseline.PromotionEnvironment ||
		resolveClosedLoopV8PromotionEnvironment(t, repositoryRoot).Public != promotion.Public {
		publishClosedLoopV8Round1Retirement(t, infrastructureCommit, inputs, "frozen_environment_drift")
		t.Fatal("V8 round-one environment differs from generation zero")
	}
	requirementSources := loadClosedLoopV8DiscoveryRequirements(t, manifest)
	observed, err := BuildV8DiscoveryRoundCases(manifestSource, obligationSource, requirementSources, discovery, registry)
	if err != nil {
		publishClosedLoopV8Round1Retirement(t, infrastructureCommit, inputs, "frontier_reconstruction_invalid")
		t.Fatal(err)
	}
	previous := closedLoopV8FrontierCases(inputs.Frontier)
	next, err := linkClosedLoopV8RoundCases(previous, observed, inputs.Selection.Selected, capabilityroundsv8.FrozenPolicy())
	if err != nil {
		publishClosedLoopV8Round1Retirement(t, infrastructureCommit, inputs, "causal_lineage_invalid")
		t.Fatal(err)
	}
	evaluation, err := capabilityroundsv8.EvaluateRound(
		previous,
		next,
		inputs.Selection.Selected,
		inputs.Ranking.State,
		capabilityroundsv8.RoundEvidence{
			DeterministicReplayComplete: true,
			PhysicalPromotionComplete:   true,
			SealEnvironmentValid:        true,
			EffectClosureValid:          true,
		},
		capabilityroundsv8.FrozenPolicy(),
	)
	if err != nil {
		publishClosedLoopV8Round1Retirement(t, infrastructureCommit, inputs, "public_round_gate_failed")
		t.Fatal(err)
	}
	frontier := buildClosedLoopV8RoundFrontier(t, artifacts, next)
	if err := atomicdir.Publish(closedLoopV8Round1Root, func(root string) error {
		refs, writeErr := writeClosedLoopV8CaseArtifacts(root, artifacts)
		if writeErr != nil {
			return writeErr
		}
		result := closedLoopV8RoundResult{
			Schema: closedLoopV8Round1Schema, Version: 8, Generation: 1,
			InfrastructureCommit: infrastructureCommit, RunnerManifestSHA256: corpusHash(inputs.RunnerManifest),
			ImplementationCommit:     inputs.Implementation.ImplementationCommit,
			ImplementationSealSHA256: inputs.Implementation.Hash, ImplementationSealFileSHA256: corpusHash(inputs.ImplementationRaw),
			InputSelectionSHA256: inputs.Selection.Hash, InputFrontierSHA256: inputs.Frontier.Hash,
			CorpusManifestSHA256: closedLoopV8CorpusManifestHash, SelectionRunnerSHA256: inputs.Selection.SelectionRunnerSHA256,
			CaseArtifacts: refs, Discovery: discovery, Evaluation: evaluation, OutputFrontier: frontier,
			RequiresNextSelection: evaluation.Status == capabilityroundsv8.EvaluationContinue,
		}
		result.Hash, writeErr = hashClosedLoopV8RoundResult(result)
		if writeErr != nil {
			return writeErr
		}
		if writeErr = os.WriteFile(filepath.Join(root, "round.json"), corpusJSON(t, result), 0o644); writeErr != nil {
			return writeErr
		}
		if writeErr = os.WriteFile(filepath.Join(root, "ROUND_AUDIT.md"), closedLoopV8RoundAudit(result), 0o644); writeErr != nil {
			return writeErr
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("V8 round one status=%s passes=%d->%d new_cohort_passes=%d", evaluation.Status, evaluation.DiscoveryPassBefore, evaluation.DiscoveryPassAfter, evaluation.NewActiveCohortPasses)
}

func loadClosedLoopV8RoundInputs(t *testing.T) closedLoopV8RoundInputs {
	t.Helper()
	repositoryRoot := closedLoopModuleRoot(t)
	// The generation-zero selection records the hash of its pre-implementation
	// runner, while the selected implementation intentionally changes one of
	// that runner's files. verifyClosedLoopV8SelectionInputs validates the raw
	// runner commitment; only manifests whose current source closure must remain
	// byte-identical are replayed here.
	for _, name := range []string{"V8_EVALUATOR.sha256", closedLoopV8Round1RunnerManifest} {
		if _, err := corpuspublication.VerifyChecksumManifest(repositoryRoot, filepath.Join(closedLoopSpecDirectory(t), name)); err != nil {
			t.Fatalf("verify frozen V8 manifest %s: %v", name, err)
		}
	}
	for _, root := range []string{closedLoopV8BaselineRoot, closedLoopV8SelectionRoot} {
		if _, err := corpuspublication.VerifyChecksumManifest(root, filepath.Join(root, corpuspublication.ChecksumFile)); err != nil {
			t.Fatalf("verify frozen V8 input %s: %v", root, err)
		}
	}
	var result closedLoopV8RoundInputs
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "report.json")), &result.Baseline)
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "frontier.json")), &result.Frontier)
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "ranking.json")), &result.Ranking)
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "selection.json")), &result.Selection)
	result.ImplementationRaw = mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8ImplementationSealFile))
	decodeCorpusStrict(t, result.ImplementationRaw, &result.Implementation)
	result.RunnerManifest = mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8Round1RunnerManifest))
	if result.Baseline.EvaluatorManifestSHA256 != corpusHash(mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V8_EVALUATOR.sha256"))) {
		t.Fatal("V8 baseline evaluator manifest binding drifted")
	}
	if result.Baseline.Hash != mustClosedLoopV8BaselineHash(t, result.Baseline) ||
		result.Frontier.Hash != mustClosedLoopV8FrontierHash(t, result.Frontier) ||
		result.Baseline.FrontierSHA256 != result.Frontier.Hash {
		t.Fatal("V8 generation-zero baseline or frontier is invalid")
	}
	if rankingHash, rankingErr := hashClosedLoopV8Ranking(result.Ranking); rankingErr != nil || rankingHash != result.Ranking.Hash {
		t.Fatal("V8 generation-zero ranking is invalid")
	}
	if selectionHash, selectionErr := hashClosedLoopV8SelectionDecision(result.Selection); selectionErr != nil || selectionHash != result.Selection.Hash ||
		result.Selection.Selected.Key != result.Ranking.Ranking.Selected.Key || result.Selection.Generation != 0 {
		t.Fatal("V8 generation-zero selection is invalid")
	}
	verifyClosedLoopV8SelectionInputs(t, result.Ranking, result.Selection)
	if implementationHash, implementationErr := hashClosedLoopV8ImplementationSeal(result.Implementation); implementationErr != nil ||
		implementationHash != result.Implementation.Hash || result.Implementation.ImplementationCommit != closedLoopV8ImplementationCommit ||
		result.Implementation.SelectionSHA256 != result.Selection.Hash || result.Implementation.EffectPlanSHA256 != result.Selection.EffectPlanSHA256 {
		t.Fatal("V8 reviewed implementation seal is invalid")
	}
	verifyClosedLoopV8ImplementationSeal(t, result.ImplementationRaw, result.Implementation, result.Selection)
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopModuleRoot(t), filepath.Join(closedLoopSpecDirectory(t), closedLoopV8Round1RunnerManifest)); err != nil {
		t.Fatal(err)
	}
	return result
}

func closedLoopV8FrontierCases(frontier closedLoopV8Frontier) []capabilityroundsv8.Case {
	result := make([]capabilityroundsv8.Case, len(frontier.Cases))
	for index := range frontier.Cases {
		result[index] = frontier.Cases[index].Case
	}
	return result
}

// linkClosedLoopV8RoundCases converts newly observed root gaps into the
// append-only causal paths required by EvaluateRound. It may only append a
// unique same/higher-stage, evidence-preserving successor to a selected leaf;
// unchanged and nonselected paths are left byte-for-byte for the evaluator to
// enforce.
func linkClosedLoopV8RoundCases(
	previous, observed []capabilityroundsv8.Case,
	selected capabilityroundsv8.Candidate,
	policy capabilityroundsv8.Policy,
) ([]capabilityroundsv8.Case, error) {
	previousByID := make(map[string]capabilityroundsv8.Case, len(previous))
	for _, current := range previous {
		if _, duplicate := previousByID[current.ID]; duplicate {
			return nil, fmt.Errorf("duplicate prior case %s", current.ID)
		}
		previousByID[current.ID] = current
	}
	selectedMembers := make(map[string]bool, len(selected.Members))
	for _, member := range selected.Members {
		selectedMembers[member.Key] = true
	}
	result := make([]capabilityroundsv8.Case, 0, len(observed))
	seenCases := map[string]bool{}
	for _, current := range observed {
		prior, found := previousByID[current.ID]
		if !found || seenCases[current.ID] {
			return nil, fmt.Errorf("round case set changed at %s", current.ID)
		}
		seenCases[current.ID] = true
		priorByHash := make(map[string]capabilityroundsv8.Gap, len(prior.Frontier))
		selectedPrior := []capabilityroundsv8.Gap{}
		for _, gap := range prior.Frontier {
			hash, err := capabilityroundsv8.PathHash(gap)
			if err != nil {
				return nil, fmt.Errorf("case %s has invalid prior path: %w", current.ID, err)
			}
			if priorByHash[hash].ObligationAnchor != "" {
				return nil, fmt.Errorf("case %s has duplicate prior path", current.ID)
			}
			priorByHash[hash] = gap
			leaf := gap.Path[len(gap.Path)-1]
			member, memberErr := capabilityroundsv8.MemberKey(leaf)
			if memberErr != nil {
				return nil, memberErr
			}
			if selectedMembers[member] {
				selectedPrior = append(selectedPrior, gap)
			}
		}
		linked := make([]capabilityroundsv8.Gap, 0, len(current.Frontier))
		for _, root := range current.Frontier {
			rootHash, err := capabilityroundsv8.PathHash(root)
			if err != nil {
				return nil, fmt.Errorf("case %s has invalid observed root path: %w", current.ID, err)
			}
			if len(root.Path) != 1 {
				return nil, fmt.Errorf("case %s observed path is not a root", current.ID)
			}
			if _, unchanged := priorByHash[rootHash]; unchanged {
				linked = append(linked, cloneClosedLoopV8Gap(root))
				continue
			}
			matches := []capabilityroundsv8.Gap{}
			rootLeaf := root.Path[0]
			rootMember, memberErr := capabilityroundsv8.MemberKey(rootLeaf)
			if memberErr != nil {
				return nil, memberErr
			}
			for _, candidate := range selectedPrior {
				priorLeaf := candidate.Path[len(candidate.Path)-1]
				priorMember, priorErr := capabilityroundsv8.MemberKey(priorLeaf)
				rootStage, rootStageKnown := policy.StageOrdinal[rootLeaf.Stage]
				priorStage, priorStageKnown := policy.StageOrdinal[priorLeaf.Stage]
				if priorErr == nil && candidate.ObligationAnchor == root.ObligationAnchor && priorMember != rootMember &&
					rootStageKnown && priorStageKnown && rootStage >= priorStage &&
					closedLoopV8StringSubset(priorLeaf.RequiredEvidence, rootLeaf.RequiredEvidence) {
					matches = append(matches, candidate)
				}
			}
			if len(matches) != 1 {
				return nil, fmt.Errorf("case %s observed gap has %d selected causal parents", current.ID, len(matches))
			}
			successor := cloneClosedLoopV8Gap(root)
			successor.Path = append(cloneClosedLoopV8Leaves(matches[0].Path), rootLeaf)
			linked = append(linked, successor)
		}
		current.Frontier = linked
		sortClosedLoopV8Gaps(current.Frontier)
		result = append(result, current)
	}
	if len(seenCases) != len(previousByID) {
		return nil, fmt.Errorf("round case set lost prior cases")
	}
	slices.SortFunc(result, func(left, right capabilityroundsv8.Case) int { return cmp.Compare(left.ID, right.ID) })
	return result, nil
}

func cloneClosedLoopV8Gap(value capabilityroundsv8.Gap) capabilityroundsv8.Gap {
	value.Path = cloneClosedLoopV8Leaves(value.Path)
	value.Diagnostics = slices.Clone(value.Diagnostics)
	return value
}

func cloneClosedLoopV8Leaves(values []capabilityroundsv8.Leaf) []capabilityroundsv8.Leaf {
	result := make([]capabilityroundsv8.Leaf, len(values))
	for index := range values {
		result[index] = values[index]
		result[index].RequiredEvidence = slices.Clone(values[index].RequiredEvidence)
	}
	return result
}

func closedLoopV8StringSubset(want, have []string) bool {
	haveSet := make(map[string]bool, len(have))
	for _, value := range have {
		haveSet[value] = true
	}
	for _, value := range want {
		if !haveSet[value] {
			return false
		}
	}
	return true
}

func sortClosedLoopV8Gaps(gaps []capabilityroundsv8.Gap) {
	slices.SortFunc(gaps, func(left, right capabilityroundsv8.Gap) int {
		if order := cmp.Compare(left.ObligationAnchor, right.ObligationAnchor); order != 0 {
			return order
		}
		leftHash, _ := capabilityroundsv8.PathHash(left)
		rightHash, _ := capabilityroundsv8.PathHash(right)
		return cmp.Compare(leftHash, rightHash)
	})
}

func buildClosedLoopV8RoundFrontier(t *testing.T, artifacts []closedLoopV8CaseArtifact, cases []capabilityroundsv8.Case) closedLoopV8Frontier {
	t.Helper()
	frontier := buildClosedLoopV8Frontier(t, artifacts, cases)
	frontier.Generation = 1
	frontier.Hash = ""
	var err error
	frontier.Hash, err = hashClosedLoopV8Frontier(frontier)
	if err != nil {
		t.Fatal(err)
	}
	return frontier
}

func publishClosedLoopV8Round1Retirement(t *testing.T, infrastructureCommit string, inputs closedLoopV8RoundInputs, reason string) {
	t.Helper()
	retirement := closedLoopV8RoundRetirement{
		Schema: closedLoopV8Round1RetireSchema, Version: 8, Generation: 1,
		InfrastructureCommit: infrastructureCommit, RunnerManifestSHA256: corpusHash(inputs.RunnerManifest),
		ImplementationCommit:     inputs.Implementation.ImplementationCommit,
		ImplementationSealSHA256: inputs.Implementation.Hash, ImplementationSealFileSHA256: corpusHash(inputs.ImplementationRaw),
		InputSelectionSHA256: inputs.Selection.Hash, InputFrontierSHA256: inputs.Frontier.Hash,
		Reason: reason, HeldOutOpened: false,
	}
	var err error
	retirement.Hash, err = hashClosedLoopV8RoundRetirement(retirement)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicdir.Publish(closedLoopV8Round1RetirementRoot, func(root string) error {
		if writeErr := os.WriteFile(filepath.Join(root, "retirement.json"), corpusJSON(t, retirement), 0o644); writeErr != nil {
			return writeErr
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
}

func verifyClosedLoopV8Round1Result(t *testing.T) {
	t.Helper()
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV8Round1Root, filepath.Join(closedLoopV8Round1Root, corpuspublication.ChecksumFile)); err != nil {
		t.Fatal(err)
	}
	var result closedLoopV8RoundResult
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8Round1Root, "round.json")), &result)
	inputs := loadClosedLoopV8RoundInputs(t)
	if want, err := hashClosedLoopV8RoundResult(result); err != nil || want != result.Hash || result.Schema != closedLoopV8Round1Schema || result.Version != 8 || result.Generation != 1 ||
		result.RunnerManifestSHA256 != corpusHash(inputs.RunnerManifest) || result.ImplementationCommit != inputs.Implementation.ImplementationCommit ||
		result.ImplementationSealSHA256 != inputs.Implementation.Hash || result.ImplementationSealFileSHA256 != corpusHash(inputs.ImplementationRaw) ||
		result.InputSelectionSHA256 != inputs.Selection.Hash || result.InputFrontierSHA256 != inputs.Frontier.Hash ||
		result.CorpusManifestSHA256 != closedLoopV8CorpusManifestHash || result.SelectionRunnerSHA256 != inputs.Selection.SelectionRunnerSHA256 {
		t.Fatal("V8 round-one result is invalid")
	}
	artifacts := assertClosedLoopV8RoundCaseArtifacts(t, result.CaseArtifacts)
	caseEvidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		caseEvidence[index] = artifacts[index].Observation
	}
	registry, _ := closedLoopV4Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, caseEvidence, registry)
	if err != nil || !reflect.DeepEqual(discovery, result.Discovery) {
		t.Fatal("V8 round-one aggregate discovery evidence does not reproduce")
	}
	manifestSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.ManifestFileV8))
	obligationSource := mustCorpusRead(t, filepath.Join(closedLoopV8CorpusRoot, corpuspublication.DiscoveryObligationsFileV8))
	var manifest corpuspublication.ManifestV8
	decodeCorpusStrict(t, manifestSource, &manifest)
	observed, err := BuildV8DiscoveryRoundCases(manifestSource, obligationSource, loadClosedLoopV8DiscoveryRequirements(t, manifest), discovery, registry)
	if err != nil {
		t.Fatal(err)
	}
	previous := closedLoopV8FrontierCases(inputs.Frontier)
	next, err := linkClosedLoopV8RoundCases(previous, observed, inputs.Selection.Selected, capabilityroundsv8.FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := capabilityroundsv8.EvaluateRound(previous, next, inputs.Selection.Selected, inputs.Ranking.State, capabilityroundsv8.RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true, EffectClosureValid: true}, capabilityroundsv8.FrozenPolicy())
	if err != nil || !reflect.DeepEqual(evaluation, result.Evaluation) {
		t.Fatal("V8 round-one evaluation does not reproduce")
	}
	frontier := buildClosedLoopV8RoundFrontier(t, artifacts, next)
	if !reflect.DeepEqual(frontier, result.OutputFrontier) || result.RequiresNextSelection != (evaluation.Status == capabilityroundsv8.EvaluationContinue) {
		t.Fatal("V8 round-one output frontier or continuation state does not reproduce")
	}
	if !bytes.Equal(mustCorpusRead(t, filepath.Join(closedLoopV8Round1Root, "ROUND_AUDIT.md")), closedLoopV8RoundAudit(result)) {
		t.Fatal("V8 round-one audit differs from its result")
	}
	assertClosedLoopV8ExactRoundFileSet(t, closedLoopV8Round1Root, result.CaseArtifacts, []string{"CHECKSUMS.sha256", "ROUND_AUDIT.md", "round.json"})
}

func verifyClosedLoopV8Round1Retirement(t *testing.T) {
	t.Helper()
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV8Round1RetirementRoot, filepath.Join(closedLoopV8Round1RetirementRoot, corpuspublication.ChecksumFile)); err != nil {
		t.Fatal(err)
	}
	var retirement closedLoopV8RoundRetirement
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8Round1RetirementRoot, "retirement.json")), &retirement)
	inputs := loadClosedLoopV8RoundInputs(t)
	if want, err := hashClosedLoopV8RoundRetirement(retirement); err != nil || want != retirement.Hash || retirement.Schema != closedLoopV8Round1RetireSchema || retirement.Version != 8 || retirement.Generation != 1 ||
		retirement.RunnerManifestSHA256 != corpusHash(inputs.RunnerManifest) || retirement.ImplementationCommit != inputs.Implementation.ImplementationCommit ||
		retirement.ImplementationSealSHA256 != inputs.Implementation.Hash || retirement.ImplementationSealFileSHA256 != corpusHash(inputs.ImplementationRaw) ||
		retirement.InputSelectionSHA256 != inputs.Selection.Hash || retirement.InputFrontierSHA256 != inputs.Frontier.Hash || retirement.Reason == "" || retirement.HeldOutOpened {
		t.Fatal("V8 round-one retirement is invalid")
	}
	assertClosedLoopV8ExactRoundFileSet(t, closedLoopV8Round1RetirementRoot, nil, []string{"CHECKSUMS.sha256", "retirement.json"})
}

func assertClosedLoopV8RoundCaseArtifacts(t *testing.T, refs []closedLoopV8ArtifactRef) []closedLoopV8CaseArtifact {
	t.Helper()
	if len(refs) != closedLoopV8RoleSize {
		t.Fatalf("V8 round-one case artifacts = %d, want %d", len(refs), closedLoopV8RoleSize)
	}
	artifacts := make([]closedLoopV8CaseArtifact, len(refs))
	for index, ref := range refs {
		wantID := fmt.Sprintf("v8_case_%03d", index+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json.gz"))
		if ref.CaseID != wantID || ref.Path != wantPath || !filepath.IsLocal(ref.Path) || !closedLoopV8ValidHash(ref.SHA256) {
			t.Fatalf("V8 round-one evidence reference %d is invalid", index+1)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV8Round1Root, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V8 round-one evidence %s differs from its commitment", ref.CaseID)
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(reader)
		decodeErr := decoder.Decode(&artifacts[index])
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		closeErr := reader.Close()
		artifact := artifacts[index]
		expected, hashErr := hashClosedLoopV8CaseArtifact(artifact)
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || hashErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV8ArtifactSchema || artifact.Version != 8 || artifact.CaseID != wantID || len(artifact.Replays) != 2 {
			t.Fatalf("V8 round-one evidence %s is structurally invalid", ref.CaseID)
		}
		if !reflect.DeepEqual(artifact.Replays[0], artifact.Replays[1]) || artifact.Observation.SynthesisHash != artifact.Replays[0].Hash {
			t.Fatalf("V8 round-one evidence %s lacks deterministic replay", ref.CaseID)
		}
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != "passed" || !artifact.Promotion.ReplayIdentical || len(artifact.Promotion.Runs) != 2 || artifact.Promotion.Hash != artifact.Observation.PromotionHash || artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				t.Fatalf("V8 passing round-one case %s lacks physical promotion", ref.CaseID)
			}
		} else if artifact.Promotion != nil {
			t.Fatalf("V8 nonpassing round-one case %s contains promotion evidence", ref.CaseID)
		}
	}
	return artifacts
}

func assertClosedLoopV8ExactRoundFileSet(t *testing.T, root string, refs []closedLoopV8ArtifactRef, extra []string) {
	t.Helper()
	want := append([]string(nil), extra...)
	for _, ref := range refs {
		want = append(want, filepath.ToSlash(ref.Path))
	}
	sort.Strings(want)
	got := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		got = append(got, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("V8 round-one file set = %#v, want %#v", got, want)
	}
}

func closedLoopV8RoundAudit(result closedLoopV8RoundResult) []byte {
	return []byte(fmt.Sprintf(closedLoopV8Round1AuditTemplate, result.Evaluation.Status, result.Evaluation.DiscoveryPassBefore, result.Evaluation.DiscoveryPassAfter, result.Evaluation.NewActiveCohortPasses, len(result.Evaluation.AdvancedCaseIDs), len(result.Evaluation.AdvancedReportingDomains), len(result.Evaluation.AdvancedCircuitRoles), len(result.Evaluation.Successors), result.ImplementationSealSHA256, result.InputFrontierSHA256, result.OutputFrontier.Hash, result.Hash))
}

func hashClosedLoopV8RoundResult(value closedLoopV8RoundResult) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV8RoundRetirement(value closedLoopV8RoundRetirement) (string, error) {
	value.Hash = ""
	return digest(value)
}

func TestLinkClosedLoopV8RoundCasesAppendsOnlyUniqueSelectedSuccessors(t *testing.T) {
	policy := capabilityroundsv8.FrozenPolicy()
	anchor := feedbackHash("round-link-anchor")
	oldLeaf := capabilityroundsv8.Leaf{Stage: "simulation", Category: "simulation", Scope: "simulation", Capability: "dc_sweep_solver", Code: "SIMULATION_INVALID", RequiredEvidence: []string{"trusted simulation"}}
	newLeaf := capabilityroundsv8.Leaf{Stage: "verification", Category: "verification", Scope: "verification", Capability: "physical_proof", Code: "PHYSICAL_PROOF_REQUIRED", RequiredEvidence: []string{"installed KiCad", "trusted simulation"}}
	oldGap := capabilityroundsv8.Gap{ObligationAnchor: anchor, Path: []capabilityroundsv8.Leaf{oldLeaf}, Diagnostics: []string{"old"}}
	newRoot := capabilityroundsv8.Gap{ObligationAnchor: anchor, Path: []capabilityroundsv8.Leaf{newLeaf}, Diagnostics: []string{"new"}}
	memberKey, err := capabilityroundsv8.MemberKey(oldLeaf)
	if err != nil {
		t.Fatal(err)
	}
	selected := capabilityroundsv8.Candidate{Members: []capabilityroundsv8.Member{{Key: memberKey, Stage: oldLeaf.Stage, Category: oldLeaf.Category, Scope: oldLeaf.Scope, Capability: oldLeaf.Capability, Code: oldLeaf.Code}}}
	previous := []capabilityroundsv8.Case{{ID: "case_001", Frontier: []capabilityroundsv8.Gap{oldGap}}}
	observed := []capabilityroundsv8.Case{{ID: "case_001", Frontier: []capabilityroundsv8.Gap{newRoot}}}
	linked, err := linkClosedLoopV8RoundCases(previous, observed, selected, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(linked) != 1 || len(linked[0].Frontier) != 1 || len(linked[0].Frontier[0].Path) != 2 || !reflect.DeepEqual(linked[0].Frontier[0].Path[0], oldLeaf) || !reflect.DeepEqual(linked[0].Frontier[0].Path[1], newLeaf) {
		t.Fatalf("linked successor = %#v", linked)
	}
	secondOldGap := cloneClosedLoopV8Gap(oldGap)
	secondOldGap.Path[0].RequiredEvidence = []string{"installed KiCad"}
	previous[0].Frontier = append(previous[0].Frontier, secondOldGap)
	if _, err := linkClosedLoopV8RoundCases(previous, observed, selected, policy); err == nil {
		t.Fatal("ambiguous selected predecessor was accepted")
	}
	previous[0].Frontier = []capabilityroundsv8.Gap{oldGap}
	unknownStage := cloneClosedLoopV8Gap(newRoot)
	unknownStage.Path[0].Stage = "unknown_stage"
	observed[0].Frontier = []capabilityroundsv8.Gap{unknownStage}
	if _, err := linkClosedLoopV8RoundCases(previous, observed, selected, policy); err == nil {
		t.Fatal("unknown successor stage was accepted")
	}
}
