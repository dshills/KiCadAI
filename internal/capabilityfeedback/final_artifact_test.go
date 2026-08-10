package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopFinalSchema           = "kicadai.closed-loop-open-set-final.v1"
	closedLoopComparisonSchema      = "kicadai.closed-loop-open-set-comparison.v1"
	closedLoopPromotionMatrixSchema = "kicadai.closed-loop-open-set-promotion-matrix.v1"
	closedLoopFinalVersion          = 1
	closedLoopFinalRoot             = "testdata/closed_loop_open_set_final"
)

type closedLoopFinalReport struct {
	Schema              string                   `json:"schema"`
	Version             int                      `json:"version"`
	CorpusManifestHash  string                   `json:"corpus_manifest_hash"`
	FreezeCommit        string                   `json:"freeze_commit"`
	EvaluatorPolicy     string                   `json:"evaluator_policy"`
	ImpactRegistryHash  string                   `json:"impact_registry_hash"`
	SynthesisPolicyHash string                   `json:"synthesis_policy_hash"`
	BaselineReportHash  string                   `json:"baseline_report_hash"`
	SelectionHash       string                   `json:"selection_hash"`
	Environment         closedLoopEnvironment    `json:"environment"`
	OutcomeCounts       []closedLoopOutcomeCount `json:"outcome_counts"`
	Discovery           AggregateReport          `json:"discovery"`
	HeldOut             AggregateReport          `json:"held_out"`
	Attribution         closedLoopAttribution    `json:"attribution"`
	Hash                string                   `json:"hash"`
}

type closedLoopArtifactEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type closedLoopAttribution struct {
	SelectedClusterKey       string                       `json:"selected_cluster_key"`
	GenericArtifacts         []closedLoopArtifactEvidence `json:"generic_artifacts"`
	DiscoveryPromotionHashes []string                     `json:"discovery_promotion_hashes"`
	HeldOutPromotionHashes   []string                     `json:"held_out_promotion_hashes"`
}

type closedLoopOutcomeTransition struct {
	CaseID string     `json:"case_id"`
	Role   CorpusRole `json:"role"`
	Before Outcome    `json:"before"`
	After  Outcome    `json:"after"`
}

type closedLoopComparison struct {
	Schema                    string                        `json:"schema"`
	Version                   int                           `json:"version"`
	CorpusManifestHash        string                        `json:"corpus_manifest_hash"`
	BaselineReportHash        string                        `json:"baseline_report_hash"`
	FinalReportHash           string                        `json:"final_report_hash"`
	SelectionHash             string                        `json:"selection_hash"`
	DiscoveryPassBefore       int                           `json:"discovery_pass_before"`
	DiscoveryPassAfter        int                           `json:"discovery_pass_after"`
	HeldOutPassBefore         int                           `json:"held_out_pass_before"`
	HeldOutPassAfter          int                           `json:"held_out_pass_after"`
	RankOneAffectedPassBefore int                           `json:"rank_one_affected_pass_before"`
	RankOneAffectedPassAfter  int                           `json:"rank_one_affected_pass_after"`
	NoBaselinePassRegression  bool                          `json:"no_baseline_pass_regression"`
	UnsafeEvidencePreserved   bool                          `json:"unsafe_evidence_preserved"`
	RemainingClustersStable   bool                          `json:"remaining_clusters_stable"`
	NewlyPassing              []closedLoopOutcomeTransition `json:"newly_passing"`
	OtherTransitions          []closedLoopOutcomeTransition `json:"other_transitions"`
	Hash                      string                        `json:"hash"`
}

type closedLoopPromotionMatrix struct {
	Schema          string                   `json:"schema"`
	Version         int                      `json:"version"`
	FinalReportHash string                   `json:"final_report_hash"`
	RequiredGates   []string                 `json:"required_gates"`
	Promotions      []closedLoopPromotionRow `json:"promotions"`
	Hash            string                   `json:"hash"`
}

type closedLoopPromotionRow struct {
	CaseID          string                      `json:"case_id"`
	Role            CorpusRole                  `json:"role"`
	SynthesisHash   string                      `json:"synthesis_hash"`
	PromotionHash   string                      `json:"promotion_hash"`
	ProjectHash     string                      `json:"project_hash"`
	Status          ots.PhysicalPromotionStatus `json:"status"`
	CleanRootRuns   int                         `json:"clean_root_runs"`
	ReplayIdentical bool                        `json:"replay_identical"`
}

func TestClosedLoopFinalArtifactsAreReproducible(t *testing.T) {
	if _, err := os.Stat(closedLoopFinalRoot); os.IsNotExist(err) {
		t.Skip("historical V1 final artifacts were never produced")
	}
	manifest := loadClosedLoopManifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json"))
	baseline, selection := loadClosedLoopBaselineAndSelection(t)
	finalCases := loadClosedLoopFinalCases(t, manifest)
	discoveryCases, heldOutCases := splitClosedLoopCases(finalCases)
	discovery, err := Evaluate(RoleDiscovery, discoveryCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	heldOut, err := Evaluate(RoleHeldOut, heldOutCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	promotions := loadClosedLoopPromotionMatrix(t)

	reportBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "FINAL_REPORT.json"))
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "FINAL_REPORT.sha256"), "FINAL_REPORT.json", reportBytes)
	var report closedLoopFinalReport
	decodeCorpusStrict(t, reportBytes, &report)
	wantReport := buildClosedLoopFinalReport(t, corpusHash(manifestBytes), manifest, baseline, selection, discovery, heldOut, promotions.Promotions)
	if !bytes.Equal(reportBytes, corpusJSON(t, wantReport)) {
		t.Fatal("final report does not reproduce from frozen case and promotion evidence")
	}

	comparisonBytes := mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "FINAL_COMPARISON.json"))
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "FINAL_COMPARISON.sha256"), "FINAL_COMPARISON.json", comparisonBytes)
	var comparison closedLoopComparison
	decodeCorpusStrict(t, comparisonBytes, &comparison)
	wantComparison := buildClosedLoopComparison(t, baseline, selection, report)
	if !bytes.Equal(comparisonBytes, corpusJSON(t, wantComparison)) {
		t.Fatal("final comparison does not reproduce from frozen baseline and final evidence")
	}
	verifyClosedLoopImprovement(t, baseline, selection, report, comparison, promotions)
}

func TestUpdateClosedLoopFinal(t *testing.T) {
	if os.Getenv("UPDATE_CLOSED_LOOP_FINAL") != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_FINAL=1 to run and record the final local corpus evaluation")
	}
	manifest := loadClosedLoopManifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, "manifest.json"))
	baseline, selection := loadClosedLoopBaselineAndSelection(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)

	// Discovery completes before held-out synthesis begins. The implementation
	// is fixed at this point; held-out identity and outcomes cannot influence it.
	discoveryCases, discoveryPromotions := runClosedLoopFinalRole(t, manifest, RoleDiscovery, inventory, environment)
	discovery, err := Evaluate(RoleDiscovery, discoveryCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	heldOutCases, heldOutPromotions := runClosedLoopFinalRole(t, manifest, RoleHeldOut, inventory, environment)
	heldOut, err := Evaluate(RoleHeldOut, heldOutCases, manifest.ImpactRegistry)
	if err != nil {
		t.Fatal(err)
	}
	promotionRows := append(discoveryPromotions, heldOutPromotions...)
	slices.SortFunc(promotionRows, func(left, right closedLoopPromotionRow) int {
		if left.CaseID < right.CaseID {
			return -1
		}
		if left.CaseID > right.CaseID {
			return 1
		}
		return 0
	})
	report := buildClosedLoopFinalReport(t, corpusHash(manifestBytes), manifest, baseline, selection, discovery, heldOut, promotionRows)
	matrix := buildClosedLoopPromotionMatrix(t, report.Hash, promotionRows)
	comparison := buildClosedLoopComparison(t, baseline, selection, report)
	verifyClosedLoopImprovement(t, baseline, selection, report, comparison, matrix)
	writeClosedLoopFinalArtifacts(t, append(discoveryCases, heldOutCases...), report, comparison, matrix)
}

func runClosedLoopFinalRole(
	t *testing.T,
	manifest closedLoopManifest,
	role CorpusRole,
	inventory ots.PrimitiveInventory,
	environment ots.SimulationEnvironment,
) ([]CaseEvidence, []closedLoopPromotionRow) {
	t.Helper()
	results := []CaseEvidence{}
	promotions := []closedLoopPromotionRow{}
	for _, entry := range manifest.Entries {
		if entry.Role != role {
			continue
		}
		t.Logf("final %s %s starting", role, entry.ID)
		requirementBytes := mustCorpusRead(t, filepath.Join(closedLoopCorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(requirementBytes))
		if len(issues) != 0 {
			t.Fatalf("%s requirement issues: %#v", entry.ID, issues)
		}
		first := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		t.Logf("final %s %s synthesis-1 status=%s stop=%s", role, entry.ID, first.Report.Status, first.Report.StopReason)
		second := runClosedLoopSynthesis(t, requirement, inventory, environment, manifest.SynthesisPolicy)
		t.Logf("final %s %s synthesis-2 status=%s stop=%s", role, entry.ID, second.Report.Status, second.Report.StopReason)
		firstBytes, firstErr := json.Marshal(first)
		secondBytes, secondErr := json.Marshal(second)
		if firstErr != nil || secondErr != nil {
			t.Fatalf("%s encode synthesis replay: first=%v second=%v", entry.ID, firstErr, secondErr)
		}
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Fatalf("%s synthesis replay differs at byte %d: first_len=%d second_len=%d first_sha256=%s second_sha256=%s", entry.ID, firstDifferentByte(firstBytes, secondBytes), len(firstBytes), len(secondBytes), corpusHash(firstBytes), corpusHash(secondBytes))
		}
		var promotion *ots.PhysicalPromotionResult
		if first.Report.Status == ots.StatusPassed {
			t.Logf(
				"final %s %s selected topology=%s evaluation=%s physical=%s; lowering graph=%s evaluation=%s physical=%s",
				role, entry.ID,
				first.Report.Selected.TopologyHash, first.Report.Selected.EvaluationHash, first.Report.Selected.PhysicalHash,
				first.Physical.GraphHash, first.Physical.EvaluationHash, first.Physical.Hash,
			)
			current := promoteClosedLoopRun(t, entry.ID, first, environment)
			if current.Status != ots.PhysicalPromotionPassed || !current.ReplayIdentical || len(current.Runs) != 2 {
				t.Fatalf("%s physical promotion failed: status=%s replay=%t runs=%d issues=%#v", entry.ID, current.Status, current.ReplayIdentical, len(current.Runs), current.Issues)
			}
			promotion = &current
			promotions = append(promotions, closedLoopPromotionRow{
				CaseID: entry.ID, Role: entry.Role, SynthesisHash: first.Hash,
				PromotionHash: current.Hash, ProjectHash: current.ProjectHash,
				Status: current.Status, CleanRootRuns: len(current.Runs), ReplayIdentical: current.ReplayIdentical,
			})
		}
		evidence, err := Observe(CaseMeta{ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact}, requirement, first, promotion)
		if err != nil {
			t.Fatalf("%s observe: %v", entry.ID, err)
		}
		t.Logf("final %s %s outcome=%s stop=%s gaps=%d synthesis=%s", role, entry.ID, evidence.Outcome, evidence.StopReason, len(evidence.Gaps), evidence.SynthesisHash)
		results = append(results, evidence)
	}
	return results, promotions
}

func buildClosedLoopFinalReport(
	t *testing.T,
	manifestHash string,
	manifest closedLoopManifest,
	baseline closedLoopBaselineReport,
	selection closedLoopSelection,
	discovery AggregateReport,
	heldOut AggregateReport,
	promotions []closedLoopPromotionRow,
) closedLoopFinalReport {
	t.Helper()
	report := closedLoopFinalReport{
		Schema: closedLoopFinalSchema, Version: closedLoopFinalVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopCorpusFreezeCommit,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, BaselineReportHash: baseline.Hash,
		SelectionHash: selection.Hash, Environment: manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(append(append([]CaseEvidence{}, discovery.Cases...), heldOut.Cases...)),
		Discovery:     discovery, HeldOut: heldOut,
		Attribution: closedLoopAttribution{
			SelectedClusterKey:       selection.Cluster.Key,
			GenericArtifacts:         closedLoopGenericArtifactEvidence(t),
			DiscoveryPromotionHashes: promotionHashes(promotions, RoleDiscovery),
			HeldOutPromotionHashes:   promotionHashes(promotions, RoleHeldOut),
		},
	}
	hash, err := hashClosedLoopFinal(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	return report
}

func buildClosedLoopComparison(
	t *testing.T,
	baseline closedLoopBaselineReport,
	selection closedLoopSelection,
	final closedLoopFinalReport,
) closedLoopComparison {
	t.Helper()
	baselineCases := closedLoopCaseMap(append(append([]CaseEvidence{}, baseline.Discovery.Cases...), baseline.HeldOut.Cases...))
	finalCases := closedLoopCaseMap(append(append([]CaseEvidence{}, final.Discovery.Cases...), final.HeldOut.Cases...))
	comparison := closedLoopComparison{
		Schema: closedLoopComparisonSchema, Version: closedLoopFinalVersion,
		CorpusManifestHash: final.CorpusManifestHash, BaselineReportHash: baseline.Hash,
		FinalReportHash: final.Hash, SelectionHash: selection.Hash,
		DiscoveryPassBefore:      countRoleOutcome(baselineCases, RoleDiscovery, OutcomePass),
		DiscoveryPassAfter:       countRoleOutcome(finalCases, RoleDiscovery, OutcomePass),
		HeldOutPassBefore:        countRoleOutcome(baselineCases, RoleHeldOut, OutcomePass),
		HeldOutPassAfter:         countRoleOutcome(finalCases, RoleHeldOut, OutcomePass),
		NoBaselinePassRegression: true, UnsafeEvidencePreserved: true, RemainingClustersStable: true,
		NewlyPassing: []closedLoopOutcomeTransition{}, OtherTransitions: []closedLoopOutcomeTransition{},
	}
	for _, id := range selection.Cluster.Cases {
		if baselineCases[id].Outcome == OutcomePass {
			comparison.RankOneAffectedPassBefore++
		}
		if finalCases[id].Outcome == OutcomePass {
			comparison.RankOneAffectedPassAfter++
		}
	}
	for id, before := range baselineCases {
		after, ok := finalCases[id]
		if !ok {
			t.Fatalf("final evidence omits baseline case %s", id)
		}
		if before.Outcome == OutcomePass && after.Outcome != OutcomePass {
			comparison.NoBaselinePassRegression = false
		}
		if before.Outcome == OutcomeUnsafe && after.Outcome == OutcomePass {
			comparison.UnsafeEvidencePreserved = false
		}
		if after.Outcome != OutcomePass && !slices.Equal(closedLoopGapKeys(before), closedLoopGapKeys(after)) {
			comparison.RemainingClustersStable = false
		}
		if before.Outcome == after.Outcome {
			continue
		}
		transition := closedLoopOutcomeTransition{CaseID: id, Role: before.Case.Role, Before: before.Outcome, After: after.Outcome}
		if after.Outcome == OutcomePass {
			comparison.NewlyPassing = append(comparison.NewlyPassing, transition)
		} else {
			comparison.OtherTransitions = append(comparison.OtherTransitions, transition)
		}
	}
	sortClosedLoopTransitions(comparison.NewlyPassing)
	sortClosedLoopTransitions(comparison.OtherTransitions)
	hash, err := hashClosedLoopComparison(comparison)
	if err != nil {
		t.Fatal(err)
	}
	comparison.Hash = hash
	return comparison
}

func buildClosedLoopPromotionMatrix(t *testing.T, finalHash string, rows []closedLoopPromotionRow) closedLoopPromotionMatrix {
	t.Helper()
	matrix := closedLoopPromotionMatrix{
		Schema: closedLoopPromotionMatrixSchema, Version: closedLoopFinalVersion,
		FinalReportHash: finalHash, RequiredGates: closedLoopRequiredPromotionGates(),
		Promotions: slices.Clone(rows),
	}
	hash, err := hashClosedLoopPromotionMatrix(matrix)
	if err != nil {
		t.Fatal(err)
	}
	matrix.Hash = hash
	return matrix
}

func verifyClosedLoopImprovement(
	t *testing.T,
	baseline closedLoopBaselineReport,
	selection closedLoopSelection,
	final closedLoopFinalReport,
	comparison closedLoopComparison,
	matrix closedLoopPromotionMatrix,
) {
	t.Helper()
	if final.CorpusManifestHash != baseline.CorpusManifestHash ||
		final.FreezeCommit != baseline.FreezeCommit ||
		final.EvaluatorPolicy != baseline.EvaluatorPolicy ||
		final.ImpactRegistryHash != baseline.ImpactRegistryHash ||
		final.SynthesisPolicyHash != baseline.SynthesisPolicyHash ||
		final.Environment != baseline.Environment {
		t.Fatal("baseline and final evaluation identities differ")
	}
	if comparison.DiscoveryPassAfter <= comparison.DiscoveryPassBefore {
		t.Fatalf("discovery pass count did not improve: before=%d after=%d", comparison.DiscoveryPassBefore, comparison.DiscoveryPassAfter)
	}
	if comparison.HeldOutPassAfter <= comparison.HeldOutPassBefore {
		t.Fatalf("held-out pass count did not improve: before=%d after=%d", comparison.HeldOutPassBefore, comparison.HeldOutPassAfter)
	}
	if comparison.RankOneAffectedPassAfter <= comparison.RankOneAffectedPassBefore {
		t.Fatalf("rank-one affected discovery pass count did not improve: before=%d after=%d", comparison.RankOneAffectedPassBefore, comparison.RankOneAffectedPassAfter)
	}
	if !comparison.NoBaselinePassRegression || !comparison.UnsafeEvidencePreserved || !comparison.RemainingClustersStable {
		t.Fatalf("improvement preservation failed: pass_regression=%t unsafe_preserved=%t clusters_stable=%t", !comparison.NoBaselinePassRegression, comparison.UnsafeEvidencePreserved, comparison.RemainingClustersStable)
	}
	rows := map[string]closedLoopPromotionRow{}
	for _, row := range matrix.Promotions {
		if _, exists := rows[row.CaseID]; exists {
			t.Fatalf("duplicate promotion for %s", row.CaseID)
		}
		if row.Status != ots.PhysicalPromotionPassed || !row.ReplayIdentical || row.CleanRootRuns != 2 || row.PromotionHash == "" || row.ProjectHash == "" {
			t.Fatalf("%s promotion is incomplete: %#v", row.CaseID, row)
		}
		rows[row.CaseID] = row
	}
	for _, transition := range comparison.NewlyPassing {
		if _, ok := rows[transition.CaseID]; !ok {
			t.Fatalf("newly passing case %s lacks physical promotion", transition.CaseID)
		}
	}
	if len(rows) != len(comparison.NewlyPassing) {
		t.Fatalf("promotion count %d does not match newly passing count %d", len(rows), len(comparison.NewlyPassing))
	}
	if !slices.Equal(matrix.RequiredGates, closedLoopRequiredPromotionGates()) {
		t.Fatal("promotion gate contract drifted")
	}
	if final.Attribution.SelectedClusterKey != selection.Cluster.Key || len(final.Attribution.GenericArtifacts) == 0 {
		t.Fatal("final causal attribution is incomplete")
	}
	for _, artifact := range final.Attribution.GenericArtifacts {
		contents := mustCorpusRead(t, filepath.Join(closedLoopModuleRoot(t), filepath.FromSlash(artifact.Path)))
		if corpusHash(contents) != artifact.SHA256 {
			t.Fatalf("generic artifact %s hash drifted", artifact.Path)
		}
	}
	if !slices.Equal(final.Attribution.DiscoveryPromotionHashes, promotionHashes(matrix.Promotions, RoleDiscovery)) ||
		!slices.Equal(final.Attribution.HeldOutPromotionHashes, promotionHashes(matrix.Promotions, RoleHeldOut)) {
		t.Fatal("promotion hashes do not match causal attribution")
	}
}

func loadClosedLoopBaselineAndSelection(t *testing.T) (closedLoopBaselineReport, closedLoopSelection) {
	t.Helper()
	specRoot := closedLoopSpecDirectory(t)
	reportBytes := mustCorpusRead(t, filepath.Join(specRoot, "BASELINE_REPORT.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "BASELINE_REPORT.sha256"), "BASELINE_REPORT.json", reportBytes)
	var report closedLoopBaselineReport
	decodeCorpusStrict(t, reportBytes, &report)
	selectionBytes := mustCorpusRead(t, filepath.Join(specRoot, "SELECTION.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "SELECTION.sha256"), "SELECTION.json", selectionBytes)
	var selection closedLoopSelection
	decodeCorpusStrict(t, selectionBytes, &selection)
	return report, selection
}

func loadClosedLoopFinalCases(t *testing.T, manifest closedLoopManifest) []CaseEvidence {
	t.Helper()
	result := make([]CaseEvidence, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		current, err := DecodeCaseEvidence(bytes.NewReader(mustCorpusRead(t, filepath.Join(closedLoopFinalRoot, entry.ID+".json"))))
		if err != nil {
			t.Fatalf("%s: %v", entry.ID, err)
		}
		if current.Case.ID != entry.ID || current.Case.Role != entry.Role || current.Case.Domain != entry.Domain || current.Case.SafetyImpact != entry.SafetyImpact {
			t.Fatalf("%s final metadata does not match manifest", entry.ID)
		}
		result = append(result, current)
	}
	return result
}

func loadClosedLoopPromotionMatrix(t *testing.T) closedLoopPromotionMatrix {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "PROMOTION_MATRIX.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "PROMOTION_MATRIX.sha256"), "PROMOTION_MATRIX.json", data)
	var matrix closedLoopPromotionMatrix
	decodeCorpusStrict(t, data, &matrix)
	expected, err := hashClosedLoopPromotionMatrix(matrix)
	if err != nil || expected != matrix.Hash {
		t.Fatalf("promotion matrix hash is invalid: expected=%s actual=%s err=%v", expected, matrix.Hash, err)
	}
	return matrix
}

func writeClosedLoopFinalArtifacts(t *testing.T, cases []CaseEvidence, report closedLoopFinalReport, comparison closedLoopComparison, matrix closedLoopPromotionMatrix) {
	t.Helper()
	if err := os.MkdirAll(closedLoopFinalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, current := range cases {
		if err := os.WriteFile(filepath.Join(closedLoopFinalRoot, current.Case.ID+".json"), corpusJSON(t, current), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	specRoot := closedLoopSpecDirectory(t)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "FINAL_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "FINAL_COMPARISON.json"), comparison)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "PROMOTION_MATRIX.json"), matrix)
}

func closedLoopGenericArtifactEvidence(t *testing.T) []closedLoopArtifactEvidence {
	t.Helper()
	// These paths are deliberately explicit rather than discovered from the
	// package tree: final attribution binds the exact reviewed production and
	// regression sources that implement the selected capability. A refactor or
	// rename must update this list and therefore changes the evidence hash.
	paths := []string{
		"internal/opentopologysynthesis/search.go",
		"internal/opentopologysynthesis/search_test.go",
		"internal/opentopologysynthesis/simulation.go",
		"internal/opentopologysynthesis/simulation_test.go",
		"internal/opentopologysynthesis/synthesis.go",
		"internal/opentopologysynthesis/synthesis_test.go",
		"internal/opentopologysynthesis/value_domains.go",
		"internal/simmodel/tolerance.go",
		"internal/simmodel/tolerance_test.go",
	}
	result := make([]closedLoopArtifactEvidence, 0, len(paths))
	for _, path := range paths {
		result = append(result, closedLoopArtifactEvidence{Path: path, SHA256: corpusHash(mustCorpusRead(t, filepath.Join(closedLoopModuleRoot(t), filepath.FromSlash(path))))})
	}
	return result
}

func closedLoopRequiredPromotionGates() []string {
	return []string{
		"strict_decode_and_behavior_validation",
		"bounded_topology_and_value_search",
		"applicable_trusted_analyses",
		"readable_hierarchical_schematic",
		"deterministic_multilayer_placement_and_routing",
		"connectivity_and_route_completion",
		"writer_correctness",
		"installed_kicad_erc",
		"installed_kicad_strict_drc",
		"zero_round_trip_differences",
		"two_clean_root_identical_projects",
	}
}

func promotionHashes(rows []closedLoopPromotionRow, role CorpusRole) []string {
	result := []string{}
	for _, row := range rows {
		if row.Role == role {
			result = append(result, row.PromotionHash)
		}
	}
	slices.Sort(result)
	return result
}

func closedLoopCaseMap(cases []CaseEvidence) map[string]CaseEvidence {
	result := make(map[string]CaseEvidence, len(cases))
	for _, current := range cases {
		result[current.Case.ID] = current
	}
	return result
}

func countRoleOutcome(cases map[string]CaseEvidence, role CorpusRole, outcome Outcome) int {
	count := 0
	for _, current := range cases {
		if current.Case.Role == role && current.Outcome == outcome {
			count++
		}
	}
	return count
}

func closedLoopGapKeys(current CaseEvidence) []string {
	result := make([]string, 0, len(current.Gaps))
	for _, gap := range current.Gaps {
		result = append(result, clusterKey(gap, current.Outcome))
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func sortClosedLoopTransitions(transitions []closedLoopOutcomeTransition) {
	slices.SortFunc(transitions, func(left, right closedLoopOutcomeTransition) int {
		if left.CaseID < right.CaseID {
			return -1
		}
		if left.CaseID > right.CaseID {
			return 1
		}
		return 0
	})
}

func hashClosedLoopFinal(report closedLoopFinalReport) (string, error) {
	report.Hash = ""
	return digest(report)
}

func hashClosedLoopComparison(comparison closedLoopComparison) (string, error) {
	comparison.Hash = ""
	return digest(comparison)
}

func hashClosedLoopPromotionMatrix(matrix closedLoopPromotionMatrix) (string, error) {
	matrix.Hash = ""
	return digest(matrix)
}

func closedLoopModuleRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(closedLoopSpecDirectory(t)))
}

func TestClosedLoopFinalHashRejectsMutation(t *testing.T) {
	report := closedLoopFinalReport{Schema: closedLoopFinalSchema, Version: closedLoopFinalVersion}
	hash, err := hashClosedLoopFinal(report)
	if err != nil {
		t.Fatal(err)
	}
	report.Hash = hash
	report.BaselineReportHash = fmt.Sprintf("%064d", 0)
	mutated, err := hashClosedLoopFinal(report)
	if err != nil {
		t.Fatal(err)
	}
	if mutated == hash {
		t.Fatal("final report mutation did not change its content hash")
	}
}
