package capabilityfeedback

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilityroundsv8"
	"kicadai/internal/corpuspublication"
)

const (
	closedLoopV8SelectionRoot      = "testdata/closed_loop_open_set_v8_selection"
	closedLoopV8SelectionUpdateEnv = "UPDATE_CLOSED_LOOP_V8_SELECTION"
	closedLoopV8EffectPlanFile     = "V8_ROUND_1_EFFECT_PLAN.json"
	closedLoopV8SelectionManifest  = "V8_SELECTION_RUNNER.sha256"
	closedLoopV8RankingSchema      = "kicadai.closed-loop-open-set-ranking.v8"
	closedLoopV8SelectionSchema    = "kicadai.closed-loop-open-set-selection.v8"
)

type closedLoopV8EffectPlanDocument struct {
	Schema                 string                      `json:"schema"`
	Version                int                         `json:"version"`
	Generation             int                         `json:"generation"`
	BaselineSHA256         string                      `json:"baseline_sha256"`
	FrontierSHA256         string                      `json:"frontier_sha256"`
	DirectAtomKeys         []string                    `json:"direct_atom_keys"`
	DirectMemberKeys       []string                    `json:"direct_member_keys"`
	ClosureAtoms           []capabilityroundsv8.Atom   `json:"closure_atoms"`
	ClosureMembers         []capabilityroundsv8.Member `json:"closure_members"`
	PlannedMemberKeys      []string                    `json:"planned_member_keys"`
	RequiredEvidence       []string                    `json:"required_evidence"`
	Executable             bool                        `json:"executable"`
	MechanicallyProven     bool                        `json:"mechanically_proven"`
	UnboundedDynamicLookup bool                        `json:"unbounded_dynamic_lookup"`
	UnmappedConsumers      []string                    `json:"unmapped_consumers"`
	StaticEvidence         closedLoopV8StaticEvidence  `json:"static_evidence"`
}

type closedLoopV8StaticEvidence struct {
	ProductionFiles                  []closedLoopV8SourceBinding `json:"production_files"`
	VerificationFiles                []closedLoopV8SourceBinding `json:"verification_files"`
	ReverseCallGraph                 []string                    `json:"reverse_call_graph"`
	RegistryReferences               []string                    `json:"registry_references"`
	ConfigurationLoaderReferences    []string                    `json:"configuration_loader_references"`
	CatalogModelReferences           []string                    `json:"catalog_model_references"`
	DataReferences                   []string                    `json:"data_references"`
	FocusedNonCorpusRuntimeConsumers []string                    `json:"focused_non_corpus_runtime_consumers"`
}

type closedLoopV8SourceBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopV8Ranking struct {
	Schema                string                        `json:"schema"`
	Version               int                           `json:"version"`
	Generation            int                           `json:"generation"`
	BaselineSHA256        string                        `json:"baseline_sha256"`
	FrontierSHA256        string                        `json:"frontier_sha256"`
	EffectPlanSHA256      string                        `json:"effect_plan_sha256"`
	SelectionRunnerSHA256 string                        `json:"selection_runner_sha256"`
	State                 capabilityroundsv8.RoundState `json:"state"`
	Ranking               capabilityroundsv8.Selection  `json:"ranking"`
	Hash                  string                        `json:"hash"`
}

type closedLoopV8SelectionDecision struct {
	Schema                string                       `json:"schema"`
	Version               int                          `json:"version"`
	Generation            int                          `json:"generation"`
	BaselineSHA256        string                       `json:"baseline_sha256"`
	FrontierSHA256        string                       `json:"frontier_sha256"`
	EffectPlanSHA256      string                       `json:"effect_plan_sha256"`
	SelectionRunnerSHA256 string                       `json:"selection_runner_sha256"`
	Selected              capabilityroundsv8.Candidate `json:"selected"`
	Hash                  string                       `json:"hash"`
}

func TestClosedLoopV8SelectionIsStructurallyFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV8SelectionRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("V8 public selection has not been published")
		}
		t.Fatal(err)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(closedLoopV8SelectionRoot, filepath.Join(closedLoopV8SelectionRoot, "CHECKSUMS.sha256")); err != nil {
		t.Fatal(err)
	}
	var ranking closedLoopV8Ranking
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "ranking.json")), &ranking)
	var decision closedLoopV8SelectionDecision
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "selection.json")), &decision)
	rankingHash, rankingErr := hashClosedLoopV8Ranking(ranking)
	decisionHash, decisionErr := hashClosedLoopV8SelectionDecision(decision)
	if rankingErr != nil || decisionErr != nil || ranking.Hash != rankingHash || decision.Hash != decisionHash ||
		ranking.Schema != closedLoopV8RankingSchema || decision.Schema != closedLoopV8SelectionSchema ||
		ranking.Version != 8 || decision.Version != 8 || ranking.Generation != 0 || decision.Generation != 0 ||
		ranking.Ranking.Selected.Key != decision.Selected.Key || ranking.EffectPlanSHA256 != decision.EffectPlanSHA256 ||
		ranking.SelectionRunnerSHA256 != decision.SelectionRunnerSHA256 {
		t.Fatal("V8 public selection is not self-consistent")
	}
	verifyClosedLoopV8SelectionInputs(t, ranking, decision)
}

func TestUpdateClosedLoopV8Selection(t *testing.T) {
	if os.Getenv(closedLoopV8SelectionUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V8_SELECTION=1 to publish generation-zero selection")
	}
	if _, err := os.Stat(closedLoopV8SelectionRoot); !os.IsNotExist(err) {
		t.Fatal("V8 public selection already exists; refusing overwrite")
	}
	var report closedLoopV8BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "report.json")), &report)
	var frontier closedLoopV8Frontier
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "frontier.json")), &frontier)
	if report.Hash != mustClosedLoopV8BaselineHash(t, report) || frontier.Hash != mustClosedLoopV8FrontierHash(t, frontier) || report.FrontierSHA256 != frontier.Hash {
		t.Fatal("V8 baseline inputs are not self-consistent")
	}
	planSource := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8EffectPlanFile))
	plan, planHash := decodeClosedLoopV8EffectPlan(t, planSource, report.Hash, frontier.Hash)
	runnerHash := corpusHash(mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8SelectionManifest)))
	cases := make([]capabilityroundsv8.Case, len(frontier.Cases))
	active := make([]string, 0, len(frontier.Cases))
	policy := capabilityroundsv8.FrozenPolicy()
	for index := range frontier.Cases {
		cases[index] = frontier.Cases[index].Case
		if policy.EligibleOutcomes[cases[index].Outcome] {
			active = append(active, cases[index].ID)
		}
	}
	slices.Sort(active)
	state := capabilityroundsv8.RoundState{Generation: 0, ActiveCohortIDs: active}
	ranking, err := capabilityroundsv8.Select(cases, []capabilityroundsv8.EffectPlan{plan}, state, policy)
	if err != nil {
		t.Fatal(err)
	}
	if ranking.Generation != 0 || len(ranking.Selected.CoveredCaseIDs) < policy.MinimumCaseSupport ||
		len(ranking.Selected.ReportingDomains) < policy.MinimumDomains || len(ranking.Selected.CircuitRoles) < policy.MinimumRoles ||
		ranking.Selected.EffectPlanSHA256 != planHash {
		t.Fatalf("V8 selection does not satisfy the frozen public gate: %+v", ranking.Selected)
	}
	rankingArtifact := closedLoopV8Ranking{Schema: closedLoopV8RankingSchema, Version: 8, Generation: 0,
		BaselineSHA256: report.Hash, FrontierSHA256: frontier.Hash, EffectPlanSHA256: planHash,
		SelectionRunnerSHA256: runnerHash, State: state, Ranking: ranking}
	rankingArtifact.Hash, err = hashClosedLoopV8Ranking(rankingArtifact)
	if err != nil {
		t.Fatal(err)
	}
	decision := closedLoopV8SelectionDecision{Schema: closedLoopV8SelectionSchema, Version: 8, Generation: 0,
		BaselineSHA256: report.Hash, FrontierSHA256: frontier.Hash, EffectPlanSHA256: planHash,
		SelectionRunnerSHA256: runnerHash, Selected: ranking.Selected}
	decision.Hash, err = hashClosedLoopV8SelectionDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicdir.Publish(closedLoopV8SelectionRoot, func(root string) error {
		for path, data := range map[string][]byte{
			"ranking.json":       corpusJSON(t, rankingArtifact),
			"selection.json":     corpusJSON(t, decision),
			"effect_plan.json":   append([]byte(nil), planSource...),
			"SELECTION_AUDIT.md": []byte(fmt.Sprintf("# V8 Generation-Zero Public Selection Audit\n\nThe frozen selector ranked the committed discovery frontier using one mechanically proven effect plan. Held-out source and keys were not opened.\n\n- baseline hash: `%s`\n- frontier hash: `%s`\n- effect-plan file hash: `%s`\n- selection-runner manifest hash: `%s`\n- selected candidate: `%s`\n- fully covered cases: %d\n- reporting domains: %d\n- circuit roles: %d\n", report.Hash, frontier.Hash, planHash, runnerHash, ranking.Selected.Key, len(ranking.Selected.CoveredCaseIDs), len(ranking.Selected.ReportingDomains), len(ranking.Selected.CircuitRoles))),
		} {
			if writeErr := os.WriteFile(filepath.Join(root, path), data, 0o644); writeErr != nil {
				return writeErr
			}
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
}

func decodeClosedLoopV8EffectPlan(t *testing.T, source []byte, baselineHash, frontierHash string) (capabilityroundsv8.EffectPlan, string) {
	t.Helper()
	var document closedLoopV8EffectPlanDocument
	decodeCorpusStrict(t, source, &document)
	if document.Schema != "kicadai.closed-loop-open-set-effect-plan.v8" || document.Version != 8 || document.Generation != 0 ||
		document.BaselineSHA256 != baselineHash || document.FrontierSHA256 != frontierHash {
		t.Fatal("V8 effect plan does not bind the committed generation-zero evidence")
	}
	repositoryRoot := filepath.Clean(filepath.Join(closedLoopSpecDirectory(t), "..", ".."))
	bindings := append(append([]closedLoopV8SourceBinding(nil), document.StaticEvidence.ProductionFiles...), document.StaticEvidence.VerificationFiles...)
	for _, binding := range bindings {
		data := mustCorpusRead(t, filepath.Join(repositoryRoot, filepath.FromSlash(binding.Path)))
		if corpusHash(data) != binding.SHA256 {
			t.Fatalf("V8 effect-plan source binding drifted: %s", binding.Path)
		}
	}
	if len(document.StaticEvidence.ReverseCallGraph) == 0 || len(document.StaticEvidence.FocusedNonCorpusRuntimeConsumers) == 0 ||
		document.StaticEvidence.RegistryReferences == nil || document.StaticEvidence.ConfigurationLoaderReferences == nil ||
		document.StaticEvidence.CatalogModelReferences == nil || document.StaticEvidence.DataReferences == nil {
		t.Fatal("V8 effect plan omits required mechanical evidence")
	}
	planHash := corpusHash(source)
	return capabilityroundsv8.EffectPlan{DirectAtomKeys: document.DirectAtomKeys, DirectMemberKeys: document.DirectMemberKeys,
		ClosureAtoms: document.ClosureAtoms, ClosureMembers: document.ClosureMembers, PlannedMemberKeys: document.PlannedMemberKeys,
		RequiredEvidence: document.RequiredEvidence, Executable: document.Executable, MechanicallyProven: document.MechanicallyProven,
		UnboundedDynamicLookup: document.UnboundedDynamicLookup, UnmappedConsumers: document.UnmappedConsumers, PlanSHA256: planHash}, planHash
}

func verifyClosedLoopV8SelectionInputs(t *testing.T, ranking closedLoopV8Ranking, decision closedLoopV8SelectionDecision) {
	t.Helper()
	var report closedLoopV8BaselineReport
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "report.json")), &report)
	var frontier closedLoopV8Frontier
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV8BaselineRoot, "frontier.json")), &frontier)
	planSource := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8EffectPlanFile))
	_, planHash := decodeClosedLoopV8EffectPlan(t, planSource, report.Hash, frontier.Hash)
	runnerHash := corpusHash(mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), closedLoopV8SelectionManifest)))
	if ranking.BaselineSHA256 != report.Hash || decision.BaselineSHA256 != report.Hash ||
		ranking.FrontierSHA256 != frontier.Hash || decision.FrontierSHA256 != frontier.Hash ||
		ranking.EffectPlanSHA256 != planHash || decision.EffectPlanSHA256 != planHash ||
		ranking.SelectionRunnerSHA256 != runnerHash || decision.SelectionRunnerSHA256 != runnerHash ||
		!bytes.Equal(planSource, mustCorpusRead(t, filepath.Join(closedLoopV8SelectionRoot, "effect_plan.json"))) {
		t.Fatal("V8 public selection input binding drifted")
	}
}

func mustClosedLoopV8BaselineHash(t *testing.T, value closedLoopV8BaselineReport) string {
	t.Helper()
	hash, err := hashClosedLoopV8BaselineReport(value)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func mustClosedLoopV8FrontierHash(t *testing.T, value closedLoopV8Frontier) string {
	t.Helper()
	hash, err := hashClosedLoopV8Frontier(value)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func hashClosedLoopV8Ranking(value closedLoopV8Ranking) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV8SelectionDecision(value closedLoopV8SelectionDecision) (string, error) {
	value.Hash = ""
	return digest(value)
}
