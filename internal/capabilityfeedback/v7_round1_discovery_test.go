package capabilityfeedback

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/binary"
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
	"kicadai/internal/capabilityrounds"
	"kicadai/internal/corpuspublication"
)

const (
	closedLoopV7Round1UpdateEnv      = "UPDATE_CLOSED_LOOP_V7_ROUND_1_DISCOVERY"
	closedLoopV7Round1Root           = "testdata/closed_loop_open_set_v7_round_1_discovery"
	closedLoopV7Round1RetirementRoot = "testdata/closed_loop_open_set_v7_round_1_retirement"
	closedLoopV7Round1Schema         = "kicadai.closed-loop-open-set-round.v7"
	closedLoopV7Round1RetireSchema   = "kicadai.closed-loop-open-set-retirement.v7"
	closedLoopV7RoundRankingSchema   = "kicadai.closed-loop-open-set-round-ranking.v7"
	closedLoopV7RoundSelectionSchema = "kicadai.closed-loop-open-set-round-selection.v7"
	closedLoopV7RoundAuditTemplate   = "# V7 Round-One Public Discovery Audit\n\nAll 18 public discovery cases were synthesized twice under the frozen environment. Every pass received two clean-root installed-KiCad promotions. Held-out source, baseline plaintext, outcomes, gaps, and keys were not opened.\n\n- status: `%s`\n- discovery passes: %d -> %d\n- new active-cohort passes: %d\n- advanced cases: %d\n- advanced domains: %d\n- lineage edges: %d\n- implementation seal: `%s`\n- input frontier: `%s`\n- output frontier: `%s`\n- result hash: `%s`\n"
)

type closedLoopV7RoundResult struct {
	Schema                   string                         `json:"schema"`
	Version                  int                            `json:"version"`
	Generation               int                            `json:"generation"`
	InfrastructureCommit     string                         `json:"infrastructure_commit"`
	ImplementationSealSHA256 string                         `json:"implementation_seal_sha256"`
	InputSelectionSHA256     string                         `json:"input_selection_sha256"`
	InputFrontierSHA256      string                         `json:"input_frontier_sha256"`
	CorpusManifestSHA256     string                         `json:"corpus_manifest_sha256"`
	SelectionPolicySHA256    string                         `json:"selection_policy_sha256"`
	CaseArtifacts            []closedLoopV7ArtifactRef      `json:"case_artifacts"`
	Discovery                AggregateReport                `json:"discovery"`
	LineageEdges             []capabilityrounds.LineageEdge `json:"lineage_edges"`
	Evaluation               capabilityrounds.Evaluation    `json:"evaluation"`
	OutputFrontier           closedLoopV7FrontierGraph      `json:"output_frontier"`
	NextRanking              *closedLoopV7RoundRanking      `json:"next_ranking,omitempty"`
	NextPlan                 *closedLoopV7GenericPlan       `json:"next_plan,omitempty"`
	NextSelection            *closedLoopV7RoundSelection    `json:"next_selection,omitempty"`
	Hash                     string                         `json:"hash"`
}

type closedLoopV7RoundRanking struct {
	Schema                string                      `json:"schema"`
	Version               int                         `json:"version"`
	Generation            int                         `json:"generation"`
	InputFrontierSHA256   string                      `json:"input_frontier_sha256"`
	InputState            capabilityrounds.RoundState `json:"input_state"`
	SelectionPolicySHA256 string                      `json:"selection_policy_sha256"`
	Decisions             capabilityrounds.Selection  `json:"decisions"`
	Hash                  string                      `json:"hash"`
}

type closedLoopV7RoundSelection struct {
	Schema                      string                      `json:"schema"`
	Version                     int                         `json:"version"`
	Generation                  int                         `json:"generation"`
	SelectionFreezeParentCommit string                      `json:"selection_freeze_parent_commit"`
	InputFrontierSHA256         string                      `json:"input_frontier_sha256"`
	InputState                  capabilityrounds.RoundState `json:"input_state"`
	RankingSHA256               string                      `json:"ranking_sha256"`
	SelectionPolicySHA256       string                      `json:"selection_policy_sha256"`
	ActiveCohort                []string                    `json:"active_cohort"`
	Selected                    capabilityrounds.Candidate  `json:"selected"`
	GenericPlanSHA256           string                      `json:"generic_plan_sha256"`
	Hash                        string                      `json:"hash"`
}

type closedLoopV7RoundRetirement struct {
	Schema                   string `json:"schema"`
	Version                  int    `json:"version"`
	Generation               int    `json:"generation"`
	InfrastructureCommit     string `json:"infrastructure_commit"`
	ImplementationSealSHA256 string `json:"implementation_seal_sha256"`
	InputSelectionSHA256     string `json:"input_selection_sha256"`
	InputFrontierSHA256      string `json:"input_frontier_sha256"`
	Reason                   string `json:"reason"`
	HeldOutOpened            bool   `json:"held_out_opened"`
	Hash                     string `json:"hash"`
}

func TestClosedLoopV7Round1DiscoveryStateIsFrozen(t *testing.T) {
	_, roundErr := os.Stat(closedLoopV7Round1Root)
	_, retirementErr := os.Stat(closedLoopV7Round1RetirementRoot)
	if os.IsNotExist(roundErr) && os.IsNotExist(retirementErr) {
		t.Skip("V7 round-one discovery has not been consumed")
	}
	if roundErr == nil && retirementErr == nil {
		t.Fatal("V7 round-one success and retirement states coexist")
	}
	if roundErr == nil {
		verifyClosedLoopV7Round1Result(t)
		return
	}
	if retirementErr == nil {
		verifyClosedLoopV7Round1Retirement(t)
		return
	}
	if roundErr != nil && !os.IsNotExist(roundErr) {
		t.Fatal(roundErr)
	}
	if retirementErr != nil && !os.IsNotExist(retirementErr) {
		t.Fatal(retirementErr)
	}
}

func TestUpdateClosedLoopV7Round1Discovery(t *testing.T) {
	if os.Getenv(closedLoopV7Round1UpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V7_ROUND_1_DISCOVERY=1 to consume the V7 round-one discovery evaluation")
	}
	for _, root := range []string{closedLoopV7Round1Root, closedLoopV7Round1RetirementRoot} {
		if _, err := os.Stat(root); err == nil {
			t.Fatalf("V7 round-one discovery already consumed at %s", root)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	infrastructureCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	implementation := loadClosedLoopV7CurrentImplementationSeal(t)
	inputSelection := loadClosedLoopV7FrozenSelection(t)
	inputFrontier := loadClosedLoopV7GenerationZeroFrontier(t)
	manifest := loadClosedLoopV7Manifest(t)
	registry, synthesisPolicy := closedLoopV7Policies(t)
	selectionPolicy := loadClosedLoopV7SelectionPolicy(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV7PromotionEnvironment(t, repositoryRoot)
	artifacts := runClosedLoopV7DiscoveryBaseline(t, manifest, synthesisPolicy, inventory, environment, promotionEnvironment)
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		publishClosedLoopV7Round1Retirement(t, infrastructureCommit, implementation.Hash, inputSelection, "aggregate discovery evidence is invalid")
		t.Fatal(err)
	}
	previous := closedLoopV7RoundCases(loadClosedLoopV7GenerationZeroReport(t).Discovery)
	next := closedLoopV7RoundCases(discovery)
	edges, edgeErr := closedLoopV7RoundLineageEdges(previous, next, inputSelection.Selected, selectionPolicy)
	if edgeErr != nil {
		publishClosedLoopV7Round1Retirement(t, infrastructureCommit, implementation.Hash, inputSelection, edgeErr.Error())
		t.Fatal(edgeErr)
	}
	evaluation, evaluationErr := capabilityrounds.EvaluateRound(
		previous, next, inputSelection.Selected, capabilityrounds.RoundState{}, edges,
		capabilityrounds.RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true},
		selectionPolicy,
	)
	if evaluationErr != nil {
		publishClosedLoopV7Round1Retirement(t, infrastructureCommit, implementation.Hash, inputSelection, evaluationErr.Error())
		t.Fatal(evaluationErr)
	}
	frontier := buildClosedLoopV7RoundFrontier(t, artifacts, next, edges)
	var nextRanking *closedLoopV7RoundRanking
	var nextPlan *closedLoopV7GenericPlan
	var nextSelection *closedLoopV7RoundSelection
	if evaluation.Status == capabilityrounds.EvaluationContinue {
		decisions, selectErr := capabilityrounds.Select(next, evaluation.NextState, selectionPolicy)
		if selectErr != nil {
			publishClosedLoopV7Round1Retirement(t, infrastructureCommit, implementation.Hash, inputSelection, selectErr.Error())
			t.Fatal(selectErr)
		}
		ranking := buildClosedLoopV7RoundRanking(t, frontier.Hash, evaluation.NextState, decisions)
		plan := buildClosedLoopV7GenericPlan(t, decisions.Selected, next, frontier.Hash)
		selection := buildClosedLoopV7RoundSelection(t, infrastructureCommit, frontier.Hash, evaluation.NextState, ranking.Hash, decisions.Selected, closedLoopV7ActiveCohort(next), plan.Hash)
		nextRanking, nextPlan, nextSelection = &ranking, &plan, &selection
	}
	if err := atomicdir.Publish(closedLoopV7Round1Root, func(root string) error {
		refs, writeErr := writeClosedLoopV7CaseArtifacts(root, artifacts)
		if writeErr != nil {
			return writeErr
		}
		result := closedLoopV7RoundResult{
			Schema: closedLoopV7Round1Schema, Version: closedLoopV7BaselineVersion, Generation: 1,
			InfrastructureCommit: infrastructureCommit, ImplementationSealSHA256: implementation.Hash,
			InputSelectionSHA256: inputSelection.Hash, InputFrontierSHA256: inputFrontier.Hash,
			CorpusManifestSHA256: closedLoopV7CorpusManifestHash, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash,
			CaseArtifacts: refs, Discovery: discovery, LineageEdges: edges, Evaluation: evaluation,
			OutputFrontier: frontier, NextRanking: nextRanking, NextPlan: nextPlan, NextSelection: nextSelection,
		}
		result.Hash, writeErr = hashClosedLoopV7RoundResult(result)
		if writeErr != nil {
			return writeErr
		}
		if err := os.WriteFile(filepath.Join(root, "round.json"), corpusJSON(t, result), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "ROUND_AUDIT.md"), closedLoopV7RoundAudit(result), 0o644); err != nil {
			return err
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("V7 round one status=%s passes=%d->%d new_cohort_passes=%d", evaluation.Status, evaluation.DiscoveryPassBefore, evaluation.DiscoveryPassAfter, evaluation.NewActiveCohortPasses)
}

func loadClosedLoopV7GenerationZeroReport(t *testing.T) closedLoopV7BaselineReport {
	t.Helper()
	var report closedLoopV7BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "report.json")), &report)
	if want, err := hashClosedLoopV7BaselineReport(report); err != nil || want != report.Hash || report.Hash != closedLoopV7BaselineHash {
		t.Fatal("V7 generation-zero report is invalid")
	}
	return report
}

func loadClosedLoopV7GenerationZeroFrontier(t *testing.T) closedLoopV7FrontierGraph {
	t.Helper()
	var frontier closedLoopV7FrontierGraph
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7BaselineRoot, "frontier_graph.json")), &frontier)
	if want, err := hashClosedLoopV7FrontierGraph(frontier); err != nil || want != frontier.Hash || frontier.Hash != closedLoopV7FrontierHash {
		t.Fatal("V7 generation-zero frontier is invalid")
	}
	return frontier
}

func closedLoopV7RoundLineageEdges(previous, next []capabilityrounds.Case, selected capabilityrounds.Candidate, policy capabilityrounds.Policy) ([]capabilityrounds.LineageEdge, error) {
	if err := closedLoopV7ValidateLineageEvidence(previous, next); err != nil {
		return nil, err
	}
	selectedMembers := make(map[string]bool, len(selected.Members))
	for _, member := range selected.Members {
		selectedMembers[member.Key] = true
	}
	nextByID := make(map[string]capabilityrounds.Case, len(next))
	for _, current := range next {
		if _, duplicate := nextByID[current.ID]; duplicate {
			return nil, fmt.Errorf("round lineage has duplicate current case %s", current.ID)
		}
		nextByID[current.ID] = current
	}
	edges := []capabilityrounds.LineageEdge{}
	priorIDs := make(map[string]bool, len(previous))
	for _, prior := range previous {
		if priorIDs[prior.ID] {
			return nil, fmt.Errorf("round lineage has duplicate prior case %s", prior.ID)
		}
		priorIDs[prior.ID] = true
		current, found := nextByID[prior.ID]
		if !found {
			return nil, fmt.Errorf("round lineage lost case %s", prior.ID)
		}
		currentGapIdentities := make(map[string]bool, len(current.Frontier))
		for _, gap := range current.Frontier {
			currentGapIdentities[closedLoopV7GapIdentity(gap)] = true
		}
		for _, oldGap := range prior.Frontier {
			oldStage := closedLoopV7CanonicalStage(oldGap.Stage)
			oldOrdinal, found := policy.StageOrdinal[oldStage]
			if !found {
				return nil, fmt.Errorf("case %s has unknown prior gap stage %s", prior.ID, oldGap.Stage)
			}
			memberKey, err := capabilityrounds.MemberKey(oldStage, oldGap.Scope, oldGap.Capability, oldGap.Code)
			if err != nil {
				return nil, fmt.Errorf("case %s prior gap identity: %w", prior.ID, err)
			}
			oldIdentity := closedLoopV7GapIdentity(oldGap)
			if selectedMembers[memberKey] || currentGapIdentities[oldIdentity] {
				continue
			}
			successors := []capabilityrounds.Gap{}
			for _, candidate := range current.Frontier {
				candidateStage := closedLoopV7CanonicalStage(candidate.Stage)
				candidateOrdinal, stageFound := policy.StageOrdinal[candidateStage]
				if !stageFound {
					return nil, fmt.Errorf("case %s has unknown successor gap stage %s", prior.ID, candidate.Stage)
				}
				if candidate.CausalToken == oldGap.CausalToken && candidate.Scope == oldGap.Scope &&
					candidate.Capability == oldGap.Capability && closedLoopV7EvidenceSuperset(candidate.RequiredEvidence, oldGap.RequiredEvidence) &&
					candidateOrdinal >= oldOrdinal {
					successors = append(successors, candidate)
				}
			}
			// V7_BASELINE_PROTOCOL "Single round evaluation" step 6 requires
			// exactly one admitted deterministic successor for a removed
			// nonselected gap. Zero silently loses lineage; multiple is ambiguous.
			if len(successors) != 1 {
				return nil, fmt.Errorf("case %s gap %s:%s:%s:%s has %d admissible successors", prior.ID, oldGap.Stage, oldGap.Scope, oldGap.Capability, oldGap.Code, len(successors))
			}
			edges = append(edges, capabilityrounds.LineageEdge{CaseID: prior.ID, From: oldGap, To: successors[0]})
		}
	}
	slices.SortFunc(edges, func(left, right capabilityrounds.LineageEdge) int {
		if order := cmp.Compare(left.CaseID, right.CaseID); order != 0 {
			return order
		}
		if order := closedLoopV7CompareGap(left.From, right.From); order != 0 {
			return order
		}
		return closedLoopV7CompareGap(left.To, right.To)
	})
	return edges, nil
}

func TestClosedLoopV7RoundLineageRejectsMissingOrAmbiguousSuccessor(t *testing.T) {
	priorGap := capabilityrounds.Gap{Stage: "simulation", Scope: "global", Capability: "example", Code: "OLD", CausalToken: "cause", RequiredEvidence: []string{"simulation"}}
	previous := []capabilityrounds.Case{{ID: "case_001", Frontier: []capabilityrounds.Gap{priorGap}}}
	policy := capabilityrounds.Policy{StageOrdinal: map[string]int{"simulation": 1, "physical": 2}}
	tests := []struct {
		name     string
		frontier []capabilityrounds.Gap
	}{
		{name: "missing"},
		{name: "ambiguous", frontier: []capabilityrounds.Gap{
			{Stage: "physical", Scope: "global", Capability: "example", Code: "NEXT_A", CausalToken: "cause", RequiredEvidence: []string{"physical", "simulation"}},
			{Stage: "physical", Scope: "global", Capability: "example", Code: "NEXT_B", CausalToken: "cause", RequiredEvidence: []string{"physical", "simulation"}},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next := []capabilityrounds.Case{{ID: "case_001", Frontier: test.frontier}}
			if _, err := closedLoopV7RoundLineageEdges(previous, next, capabilityrounds.Candidate{}, policy); err == nil {
				t.Fatal("non-unique lineage successor unexpectedly admitted")
			}
		})
	}
	duplicate := []capabilityrounds.Case{{ID: "case_001", Frontier: []capabilityrounds.Gap{priorGap}}, {ID: "case_001", Frontier: []capabilityrounds.Gap{priorGap}}}
	if _, err := closedLoopV7RoundLineageEdges(previous, duplicate, capabilityrounds.Candidate{}, policy); err == nil {
		t.Fatal("duplicate current case IDs unexpectedly admitted")
	}
}

func closedLoopV7ValidateLineageEvidence(cohorts ...[]capabilityrounds.Case) error {
	for _, cases := range cohorts {
		for _, current := range cases {
			for _, gap := range current.Frontier {
				if len(gap.RequiredEvidence) == 0 || !slices.IsSorted(gap.RequiredEvidence) {
					return fmt.Errorf("case %s gap evidence is not canonical", current.ID)
				}
				for index, evidence := range gap.RequiredEvidence {
					if evidence == "" || (index > 0 && evidence == gap.RequiredEvidence[index-1]) {
						return fmt.Errorf("case %s gap evidence is not unique and nonempty", current.ID)
					}
				}
			}
		}
	}
	return nil
}

func closedLoopV7CompareGap(left, right capabilityrounds.Gap) int {
	for _, pair := range [][2]string{{left.Stage, right.Stage}, {left.Scope, right.Scope}, {left.Capability, right.Capability}, {left.Code, right.Code}, {left.CausalToken, right.CausalToken}} {
		if order := cmp.Compare(pair[0], pair[1]); order != 0 {
			return order
		}
	}
	return slices.Compare(left.RequiredEvidence, right.RequiredEvidence)
}

func closedLoopV7GapIdentity(gap capabilityrounds.Gap) string {
	var identity bytes.Buffer
	var prefix [4]byte
	writeField := func(field string) {
		binary.BigEndian.PutUint32(prefix[:], uint32(len(field)))
		identity.Write(prefix[:])
		identity.WriteString(field)
	}
	writeField(gap.Stage)
	writeField(gap.Scope)
	writeField(gap.Capability)
	writeField(gap.Code)
	writeField(gap.CausalToken)
	for _, evidence := range gap.RequiredEvidence {
		writeField(evidence)
	}
	return identity.String()
}

func closedLoopV7EvidenceSuperset(candidate, prior []string) bool {
	candidateIndex := 0
	for _, required := range prior {
		for candidateIndex < len(candidate) && candidate[candidateIndex] < required {
			candidateIndex++
		}
		if candidateIndex == len(candidate) || candidate[candidateIndex] != required {
			return false
		}
		candidateIndex++
	}
	return true
}

func buildClosedLoopV7RoundFrontier(t *testing.T, artifacts []closedLoopV7CaseArtifact, cases []capabilityrounds.Case, edges []capabilityrounds.LineageEdge) closedLoopV7FrontierGraph {
	t.Helper()
	frontier := buildClosedLoopV7FrontierGraph(t, artifacts, cases)
	frontier.Generation = 1
	byCase := map[string][]capabilityrounds.LineageEdge{}
	for _, edge := range edges {
		byCase[edge.CaseID] = append(byCase[edge.CaseID], edge)
	}
	for index := range frontier.Cases {
		frontier.Cases[index].CausalEdges = append([]capabilityrounds.LineageEdge(nil), byCase[frontier.Cases[index].CaseID]...)
		classes := []closedLoopV7TransitionClass{}
		for _, edge := range frontier.Cases[index].CausalEdges {
			fromKey, fromErr := capabilityrounds.MemberKey(closedLoopV7CanonicalStage(edge.From.Stage), edge.From.Scope, edge.From.Capability, edge.From.Code)
			toKey, toErr := capabilityrounds.MemberKey(closedLoopV7CanonicalStage(edge.To.Stage), edge.To.Scope, edge.To.Capability, edge.To.Code)
			if fromErr != nil || toErr != nil {
				t.Fatalf("V7 round-one transition has invalid member identity: from=%v to=%v", fromErr, toErr)
			}
			classes = append(classes, closedLoopV7TransitionClass{FromMemberKey: fromKey, ToMemberKey: toKey, Class: "admitted_successor"})
		}
		frontier.Cases[index].TransitionClasses = classes
	}
	frontier.Hash = ""
	var err error
	frontier.Hash, err = hashClosedLoopV7FrontierGraph(frontier)
	if err != nil {
		t.Fatal(err)
	}
	return frontier
}

func buildClosedLoopV7RoundRanking(t *testing.T, frontierHash string, state capabilityrounds.RoundState, decisions capabilityrounds.Selection) closedLoopV7RoundRanking {
	t.Helper()
	ranking := closedLoopV7RoundRanking{Schema: closedLoopV7RoundRankingSchema, Version: closedLoopV7BaselineVersion, Generation: state.Generation, InputFrontierSHA256: frontierHash, InputState: state, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash, Decisions: decisions}
	var err error
	ranking.Hash, err = hashClosedLoopV7RoundRanking(ranking)
	if err != nil {
		t.Fatal(err)
	}
	return ranking
}

func buildClosedLoopV7RoundSelection(t *testing.T, parentCommit, frontierHash string, state capabilityrounds.RoundState, rankingHash string, selected capabilityrounds.Candidate, activeCohort []string, planHash string) closedLoopV7RoundSelection {
	t.Helper()
	selection := closedLoopV7RoundSelection{Schema: closedLoopV7RoundSelectionSchema, Version: closedLoopV7BaselineVersion, Generation: state.Generation, SelectionFreezeParentCommit: parentCommit, InputFrontierSHA256: frontierHash, InputState: state, RankingSHA256: rankingHash, SelectionPolicySHA256: closedLoopV7SelectionPolicyHash, ActiveCohort: activeCohort, Selected: selected, GenericPlanSHA256: planHash}
	var err error
	selection.Hash, err = hashClosedLoopV7RoundSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func publishClosedLoopV7Round1Retirement(t *testing.T, infrastructureCommit, implementationHash string, selection closedLoopV7Selection, reason string) {
	t.Helper()
	retirement := closedLoopV7RoundRetirement{Schema: closedLoopV7Round1RetireSchema, Version: closedLoopV7BaselineVersion, Generation: 1, InfrastructureCommit: infrastructureCommit, ImplementationSealSHA256: implementationHash, InputSelectionSHA256: selection.Hash, InputFrontierSHA256: selection.InputFrontierSHA256, Reason: reason, HeldOutOpened: false}
	var err error
	retirement.Hash, err = hashClosedLoopV7RoundRetirement(retirement)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicdir.Publish(closedLoopV7Round1RetirementRoot, func(root string) error {
		if err := os.WriteFile(filepath.Join(root, "retirement.json"), corpusJSON(t, retirement), 0o644); err != nil {
			return err
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
}

func verifyClosedLoopV7Round1Result(t *testing.T) {
	t.Helper()
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV7Round1Root, filepath.Join(closedLoopV7Round1Root, corpuspublication.ChecksumFile)); err != nil {
		t.Fatal(err)
	}
	var result closedLoopV7RoundResult
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7Round1Root, "round.json")), &result)
	implementation := loadClosedLoopV7CurrentImplementationSeal(t)
	inputSelection := loadClosedLoopV7FrozenSelection(t)
	inputFrontier := loadClosedLoopV7GenerationZeroFrontier(t)
	if want, err := hashClosedLoopV7RoundResult(result); err != nil || want != result.Hash || result.Schema != closedLoopV7Round1Schema || result.Version != closedLoopV7BaselineVersion || result.Generation != 1 ||
		result.ImplementationSealSHA256 != implementation.Hash || result.InputSelectionSHA256 != inputSelection.Hash || result.InputFrontierSHA256 != inputFrontier.Hash ||
		result.CorpusManifestSHA256 != closedLoopV7CorpusManifestHash || result.SelectionPolicySHA256 != closedLoopV7SelectionPolicyHash {
		t.Fatal("V7 round-one result is invalid")
	}
	artifacts := assertClosedLoopV7RoundCaseArtifacts(t, result.CaseArtifacts)
	caseEvidence := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		caseEvidence[index] = artifacts[index].Observation
	}
	registry, _ := closedLoopV7Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, caseEvidence, registry)
	if err != nil || !bytes.Equal(corpusJSON(t, discovery), corpusJSON(t, result.Discovery)) {
		t.Fatal("V7 round-one aggregate discovery evidence does not reproduce")
	}
	policy := loadClosedLoopV7SelectionPolicy(t)
	previous := closedLoopV7RoundCases(loadClosedLoopV7GenerationZeroReport(t).Discovery)
	next := closedLoopV7RoundCases(discovery)
	edges, err := closedLoopV7RoundLineageEdges(previous, next, inputSelection.Selected, policy)
	if err != nil || !bytes.Equal(corpusJSON(t, edges), corpusJSON(t, result.LineageEdges)) {
		t.Fatal("V7 round-one lineage does not reproduce")
	}
	evaluation, err := capabilityrounds.EvaluateRound(previous, next, inputSelection.Selected, capabilityrounds.RoundState{}, edges, capabilityrounds.RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true}, policy)
	if err != nil || !bytes.Equal(corpusJSON(t, evaluation), corpusJSON(t, result.Evaluation)) {
		t.Fatal("V7 round-one evaluation does not reproduce")
	}
	frontier := buildClosedLoopV7RoundFrontier(t, artifacts, next, edges)
	if !bytes.Equal(corpusJSON(t, frontier), corpusJSON(t, result.OutputFrontier)) {
		t.Fatal("V7 round-one output frontier does not reproduce")
	}
	verifyClosedLoopV7NextSelection(t, result, next, policy)
	if !bytes.Equal(mustCorpusRead(t, filepath.Join(closedLoopV7Round1Root, "ROUND_AUDIT.md")), closedLoopV7RoundAudit(result)) {
		t.Fatal("V7 round-one audit differs from its result")
	}
	assertClosedLoopV7ExactRoundFileSet(t, closedLoopV7Round1Root, result.CaseArtifacts, []string{"CHECKSUMS.sha256", "ROUND_AUDIT.md", "round.json"})
}

func verifyClosedLoopV7NextSelection(t *testing.T, result closedLoopV7RoundResult, next []capabilityrounds.Case, policy capabilityrounds.Policy) {
	t.Helper()
	if result.Evaluation.Status == capabilityrounds.EvaluationPublicAdmitted {
		if result.NextRanking != nil || result.NextPlan != nil || result.NextSelection != nil {
			t.Fatal("publicly admitted V7 round contains a forbidden next selection")
		}
		return
	}
	if result.Evaluation.Status != capabilityrounds.EvaluationContinue || result.NextRanking == nil || result.NextPlan == nil || result.NextSelection == nil {
		t.Fatal("continuing V7 round lacks a complete next selection")
	}
	decisions, err := capabilityrounds.Select(next, result.Evaluation.NextState, policy)
	if err != nil {
		t.Fatal(err)
	}
	ranking := buildClosedLoopV7RoundRanking(t, result.OutputFrontier.Hash, result.Evaluation.NextState, decisions)
	plan := buildClosedLoopV7GenericPlan(t, decisions.Selected, next, result.OutputFrontier.Hash)
	selection := buildClosedLoopV7RoundSelection(t, result.InfrastructureCommit, result.OutputFrontier.Hash, result.Evaluation.NextState, ranking.Hash, decisions.Selected, closedLoopV7ActiveCohort(next), plan.Hash)
	if !plan.Executable || !bytes.Equal(corpusJSON(t, ranking), corpusJSON(t, *result.NextRanking)) ||
		!bytes.Equal(corpusJSON(t, plan), corpusJSON(t, *result.NextPlan)) ||
		!bytes.Equal(corpusJSON(t, selection), corpusJSON(t, *result.NextSelection)) {
		t.Fatal("V7 round-one next selection does not reproduce")
	}
}

func verifyClosedLoopV7Round1Retirement(t *testing.T) {
	t.Helper()
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV7Round1RetirementRoot, filepath.Join(closedLoopV7Round1RetirementRoot, corpuspublication.ChecksumFile)); err != nil {
		t.Fatal(err)
	}
	var retirement closedLoopV7RoundRetirement
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV7Round1RetirementRoot, "retirement.json")), &retirement)
	implementation := loadClosedLoopV7CurrentImplementationSeal(t)
	selection := loadClosedLoopV7FrozenSelection(t)
	if want, err := hashClosedLoopV7RoundRetirement(retirement); err != nil || want != retirement.Hash || retirement.Schema != closedLoopV7Round1RetireSchema || retirement.Version != closedLoopV7BaselineVersion || retirement.Generation != 1 || retirement.ImplementationSealSHA256 != implementation.Hash || retirement.InputSelectionSHA256 != selection.Hash || retirement.InputFrontierSHA256 != selection.InputFrontierSHA256 || retirement.Reason == "" || retirement.HeldOutOpened {
		t.Fatal("V7 round-one retirement is invalid")
	}
	assertClosedLoopV7ExactRoundFileSet(t, closedLoopV7Round1RetirementRoot, nil, []string{"CHECKSUMS.sha256", "retirement.json"})
}

func assertClosedLoopV7RoundCaseArtifacts(t *testing.T, refs []closedLoopV7ArtifactRef) []closedLoopV7CaseArtifact {
	t.Helper()
	if len(refs) != closedLoopV7RoleSize {
		t.Fatalf("V7 round-one case artifacts = %d, want %d", len(refs), closedLoopV7RoleSize)
	}
	artifacts := make([]closedLoopV7CaseArtifact, len(refs))
	for index, ref := range refs {
		wantID := fmt.Sprintf("v7_case_%03d", index+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json.gz"))
		if ref.CaseID != wantID || ref.Path != wantPath || !filepath.IsLocal(ref.Path) || !closedLoopV7ValidHash(ref.SHA256) {
			t.Fatalf("V7 round-one evidence reference %d is invalid", index+1)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV7Round1Root, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V7 round-one evidence %s differs from its commitment", ref.CaseID)
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
		expected, hashErr := hashClosedLoopV7CaseArtifact(artifact)
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil || hashErr != nil || artifact.Hash != expected || artifact.Schema != closedLoopV7CaseArtifactSchema || artifact.Version != closedLoopV7BaselineVersion || artifact.CaseID != wantID || len(artifact.Replays) != 2 {
			t.Fatalf("V7 round-one evidence %s is structurally invalid", ref.CaseID)
		}
		if !reflect.DeepEqual(artifact.Replays[0], artifact.Replays[1]) || artifact.Observation.SynthesisHash != artifact.Replays[0].Hash {
			t.Fatalf("V7 round-one evidence %s lacks deterministic replay", ref.CaseID)
		}
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != "passed" || !artifact.Promotion.ReplayIdentical || len(artifact.Promotion.Runs) != 2 || artifact.Promotion.Hash != artifact.Observation.PromotionHash || artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				t.Fatalf("V7 passing round-one case %s lacks physical promotion", ref.CaseID)
			}
		} else if artifact.Promotion != nil {
			t.Fatalf("V7 nonpassing round-one case %s contains promotion evidence", ref.CaseID)
		}
	}
	return artifacts
}

func assertClosedLoopV7ExactRoundFileSet(t *testing.T, root string, refs []closedLoopV7ArtifactRef, extra []string) {
	t.Helper()
	want := append([]string(nil), extra...)
	for _, ref := range refs {
		want = append(want, filepath.ToSlash(ref.Path))
	}
	sort.Strings(want)
	var got []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("V7 round-one file set = %#v, want %#v", got, want)
	}
}

func closedLoopV7RoundAudit(result closedLoopV7RoundResult) []byte {
	return []byte(fmt.Sprintf(closedLoopV7RoundAuditTemplate, result.Evaluation.Status, result.Evaluation.DiscoveryPassBefore, result.Evaluation.DiscoveryPassAfter, result.Evaluation.NewActiveCohortPasses, len(result.Evaluation.AdvancedCaseIDs), len(result.Evaluation.AdvancedReportingDomains), len(result.LineageEdges), result.ImplementationSealSHA256, result.InputFrontierSHA256, result.OutputFrontier.Hash, result.Hash))
}

func hashClosedLoopV7RoundResult(value closedLoopV7RoundResult) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV7RoundRetirement(value closedLoopV7RoundRetirement) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV7RoundRanking(value closedLoopV7RoundRanking) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV7RoundSelection(value closedLoopV7RoundSelection) (string, error) {
	value.Hash = ""
	return digest(value)
}
