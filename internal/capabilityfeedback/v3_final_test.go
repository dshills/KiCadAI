package capabilityfeedback

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV3FinalSchema               = "kicadai.closed-loop-open-set-final.v3"
	closedLoopV3FinalComparisonSchema     = "kicadai.closed-loop-open-set-comparison.v3"
	closedLoopV3FinalPromotionSchema      = "kicadai.closed-loop-open-set-promotion-matrix.v3"
	closedLoopV3HeldOutFinalPayloadSchema = "kicadai.closed-loop-open-set-held-out-final-payload.v3"
	closedLoopV3HeldOutFinalSealSchema    = "kicadai.closed-loop-open-set-held-out-final-seal.v3"
	closedLoopV3ImplementationSealSchema  = "kicadai.closed-loop-open-set-reviewed-implementation.v3"
	closedLoopV3FinalUpdateEnv            = "UPDATE_CLOSED_LOOP_V3_FINAL"
	closedLoopV3HeldOutFinalKeyEnv        = "KICADAI_V3_HELD_OUT_FINAL_KEY_FILE"
	closedLoopV3FinalRoot                 = "testdata/closed_loop_open_set_v3_final"
	closedLoopV3HeldOutFinalFile          = "held_out_final.sealed"
)

type closedLoopV3ImplementationSeal struct {
	Schema             string                       `json:"schema"`
	Version            int                          `json:"version"`
	SelectedCapability string                       `json:"selected_capability"`
	Review             string                       `json:"review"`
	Artifacts          []closedLoopArtifactEvidence `json:"artifacts"`
	Hash               string                       `json:"hash"`
}

type closedLoopV3FinalReport struct {
	Schema                     string                   `json:"schema"`
	Version                    int                      `json:"version"`
	CorpusManifestHash         string                   `json:"corpus_manifest_hash"`
	FreezeCommit               string                   `json:"freeze_commit"`
	SelectionHash              string                   `json:"selection_hash"`
	DiscoveryBaselineHash      string                   `json:"discovery_baseline_hash"`
	HeldOutBaselinePayloadHash string                   `json:"held_out_baseline_payload_hash"`
	ImplementationSealHash     string                   `json:"implementation_seal_hash"`
	EvaluatorPolicy            string                   `json:"evaluator_policy"`
	ImpactRegistryHash         string                   `json:"impact_registry_hash"`
	SynthesisPolicyHash        string                   `json:"synthesis_policy_hash"`
	Environment                closedLoopEnvironment    `json:"environment"`
	OutcomeCounts              []closedLoopOutcomeCount `json:"outcome_counts"`
	Discovery                  AggregateReport          `json:"discovery"`
	HeldOutAggregateHash       string                   `json:"held_out_aggregate_hash"`
	Attribution                closedLoopAttribution    `json:"attribution"`
	Hash                       string                   `json:"hash"`
}

type closedLoopV3FinalComparison struct {
	Schema                    string `json:"schema"`
	Version                   int    `json:"version"`
	FinalReportHash           string `json:"final_report_hash"`
	SelectionHash             string `json:"selection_hash"`
	DiscoveryPassBefore       int    `json:"discovery_pass_before"`
	DiscoveryPassAfter        int    `json:"discovery_pass_after"`
	HeldOutPassBefore         int    `json:"held_out_pass_before"`
	HeldOutPassAfter          int    `json:"held_out_pass_after"`
	RankOneAffectedPassBefore int    `json:"rank_one_affected_pass_before"`
	RankOneAffectedPassAfter  int    `json:"rank_one_affected_pass_after"`
	NoBaselinePassRegression  bool   `json:"no_baseline_pass_regression"`
	UnsafeEvidencePreserved   bool   `json:"unsafe_evidence_preserved"`
	RemainingGapsStable       bool   `json:"remaining_gaps_stable"`
	Hash                      string `json:"hash"`
}

type closedLoopV3FinalPromotionMatrix struct {
	Schema          string                   `json:"schema"`
	Version         int                      `json:"version"`
	FinalReportHash string                   `json:"final_report_hash"`
	RequiredGates   []string                 `json:"required_gates"`
	Promotions      []closedLoopPromotionRow `json:"promotions"`
	Hash            string                   `json:"hash"`
}

type closedLoopV3HeldOutFinalPayload struct {
	Schema             string                   `json:"schema"`
	Version            int                      `json:"version"`
	CorpusManifestHash string                   `json:"corpus_manifest_hash"`
	SelectionHash      string                   `json:"selection_hash"`
	ImplementationHash string                   `json:"implementation_hash"`
	Cases              []CaseEvidence           `json:"cases"`
	Aggregate          AggregateReport          `json:"aggregate"`
	Promotions         []closedLoopPromotionRow `json:"promotions"`
	Hash               string                   `json:"hash"`
}

type closedLoopV3HeldOutFinalSeal struct {
	Schema              string `json:"schema"`
	Version             int    `json:"version"`
	Algorithm           string `json:"algorithm"`
	CorpusManifestHash  string `json:"corpus_manifest_hash"`
	SelectionHash       string `json:"selection_hash"`
	ImplementationHash  string `json:"implementation_hash"`
	BaselinePayloadHash string `json:"baseline_payload_hash"`
	PayloadHash         string `json:"payload_hash"`
	AggregateHash       string `json:"aggregate_hash"`
	CiphertextHash      string `json:"ciphertext_sha256"`
	CaseCount           int    `json:"case_count"`
	Hash                string `json:"hash"`
}

func TestClosedLoopV3ReviewedImplementationIsFrozen(t *testing.T) {
	loadClosedLoopV3ImplementationSeal(t)
}

func TestClosedLoopV3FinalArtifactsAreFrozen(t *testing.T) {
	specRoot := closedLoopSpecDirectory(t)
	reportPath := filepath.Join(specRoot, "V3_FINAL_REPORT.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Skip("V3 final artifacts have not been produced")
	}
	var report closedLoopV3FinalReport
	reportBytes := mustCorpusRead(t, reportPath)
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_FINAL_REPORT.sha256"), "V3_FINAL_REPORT.json", reportBytes)
	decodeCorpusStrict(t, reportBytes, &report)
	if want, err := hashClosedLoopV3FinalReport(report); err != nil || want != report.Hash {
		t.Fatal("V3 final report hash is invalid")
	}
	var comparison closedLoopV3FinalComparison
	comparisonBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_FINAL_COMPARISON.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_FINAL_COMPARISON.sha256"), "V3_FINAL_COMPARISON.json", comparisonBytes)
	decodeCorpusStrict(t, comparisonBytes, &comparison)
	if want, err := hashClosedLoopV3FinalComparison(comparison); err != nil || want != comparison.Hash ||
		comparison.FinalReportHash != report.Hash || comparison.SelectionHash != report.SelectionHash {
		t.Fatal("V3 final comparison is invalid")
	}
	var matrix closedLoopV3FinalPromotionMatrix
	matrixBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_PROMOTION_MATRIX.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_PROMOTION_MATRIX.sha256"), "V3_PROMOTION_MATRIX.json", matrixBytes)
	decodeCorpusStrict(t, matrixBytes, &matrix)
	if want, err := hashClosedLoopV3FinalPromotionMatrix(matrix); err != nil || want != matrix.Hash ||
		matrix.FinalReportHash != report.Hash || !slices.Equal(matrix.RequiredGates, closedLoopRequiredPromotionGates()) {
		t.Fatal("V3 promotion matrix is invalid")
	}
	var seal closedLoopV3HeldOutFinalSeal
	sealBytes := mustCorpusRead(t, filepath.Join(specRoot, "V3_HELD_OUT_FINAL_SEAL.json"))
	assertArtifactChecksum(t, filepath.Join(specRoot, "V3_HELD_OUT_FINAL_SEAL.sha256"), "V3_HELD_OUT_FINAL_SEAL.json", sealBytes)
	decodeCorpusStrict(t, sealBytes, &seal)
	if want, err := hashClosedLoopV3HeldOutFinalSeal(seal); err != nil || want != seal.Hash ||
		seal.CorpusManifestHash != report.CorpusManifestHash || seal.SelectionHash != report.SelectionHash ||
		seal.BaselinePayloadHash != report.HeldOutBaselinePayloadHash || seal.AggregateHash != report.HeldOutAggregateHash ||
		seal.CaseCount != closedLoopCorpusSize/2 {
		t.Fatal("V3 held-out final seal is invalid")
	}
	if corpusHash(mustCorpusRead(t, filepath.Join(closedLoopV3FinalRoot, closedLoopV3HeldOutFinalFile))) != seal.CiphertextHash {
		t.Fatal("V3 held-out final ciphertext hash drifted")
	}
	implementation := loadClosedLoopV3ImplementationSeal(t)
	if implementation.Hash != report.ImplementationSealHash || implementation.Hash != seal.ImplementationHash {
		t.Fatal("V3 final artifacts are not bound to the reviewed implementation")
	}
	verifyClosedLoopV3Comparison(t, comparison)
	if len(matrix.Promotions) != comparison.DiscoveryPassAfter+comparison.HeldOutPassAfter {
		t.Fatal("V3 promotion count does not match final pass count")
	}
}

func TestUpdateClosedLoopV3Final(t *testing.T) {
	if os.Getenv(closedLoopV3FinalUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V3_FINAL=1 for the one-time V3 final evaluation")
	}
	refuseExistingClosedLoopV3Final(t)
	implementation := loadClosedLoopV3ImplementationSeal(t)
	manifest := loadClosedLoopV3Manifest(t)
	manifestBytes := mustCorpusRead(t, filepath.Join(closedLoopV3CorpusRoot, "manifest.json"))
	registry, policy := closedLoopV3Policies(t)
	selection := loadFrozenClosedLoopV3Selection(t, manifest)
	baseline := loadClosedLoopV3DiscoveryBaselineReport(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)

	// Discovery is the hard boundary: no held-out key or ciphertext is opened
	// until the reviewed implementation proves strict selected-cluster uplift.
	discoveryCases := runClosedLoopV3DiscoveryBaseline(t, manifest, policy, inventory, environment)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, discoveryCases, registry)
	if err != nil {
		t.Fatal("V3 final discovery aggregation failed closed")
	}
	discoveryBefore := countV3Outcome(baseline.Discovery.Cases, OutcomePass)
	discoveryAfter := countV3Outcome(discovery.Cases, OutcomePass)
	rankBefore, rankAfter, rankComplete := selectedV3PassCounts(selection, baseline.Discovery.Cases, discovery.Cases)
	if !rankComplete || discoveryAfter <= discoveryBefore || rankAfter <= rankBefore {
		t.Fatalf("V3 held-out final remains sealed: discovery pass %d->%d selected %d->%d", discoveryBefore, discoveryAfter, rankBefore, rankAfter)
	}

	baselinePayload := loadClosedLoopV3HeldOutBaselinePayload(t, selection)
	sourceKeyPath := os.Getenv(closedLoopV3HeldOutCorpusKeyEnv)
	baselineKeyPath := os.Getenv(closedLoopV3HeldOutBaselineKeyEnv)
	finalKeyPath := os.Getenv(closedLoopV3HeldOutFinalKeyEnv)
	if sourceKeyPath == "" || baselineKeyPath == "" || finalKeyPath == "" {
		t.Fatal("separate external V3 source, baseline, and final key paths are required")
	}
	if sourceKeyPath == finalKeyPath || sourceKeyPath == baselineKeyPath || finalKeyPath == baselineKeyPath {
		t.Fatal("V3 source, baseline, and final keys must be distinct")
	}
	requirements := loadClosedLoopV3HeldOutRequirements(t, manifest, sourceKeyPath)
	heldOutCases := runClosedLoopV3HeldOutBaseline(t, manifest, requirements, policy, inventory, environment)
	heldOut, err := EvaluateRealizabilityAware(RoleHeldOut, heldOutCases, registry)
	if err != nil {
		t.Fatal("V3 final held-out aggregation failed closed")
	}
	comparison := buildClosedLoopV3FinalComparison(t, selection, baseline.Discovery.Cases, discovery.Cases, baselinePayload.Cases, heldOut.Cases)
	verifyClosedLoopV3Comparison(t, comparison)

	promotions := closedLoopV3PromotionRows(append(append([]CaseEvidence{}, discovery.Cases...), heldOut.Cases...))
	payload := closedLoopV3HeldOutFinalPayload{
		Schema: closedLoopV3HeldOutFinalPayloadSchema, Version: closedLoopV3BaselineVersion,
		CorpusManifestHash: corpusHash(manifestBytes), SelectionHash: selection.Hash,
		ImplementationHash: implementation.Hash, Cases: heldOut.Cases, Aggregate: heldOut,
		Promotions: closedLoopV3PromotionRows(heldOut.Cases),
	}
	payload.Hash, err = hashClosedLoopV3HeldOutFinalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	finalKey := closedLoopV3LoadOrCreateKey(t, finalKeyPath)
	ciphertext := sealClosedLoopV3HeldOutPayload(t, finalKey, payload.CorpusManifestHash, selection.Hash, payload.Hash, corpusJSON(t, payload))
	report := buildClosedLoopV3FinalReport(t, manifest, corpusHash(manifestBytes), selection, baseline, baselinePayload, implementation, discovery, heldOut, promotions)
	comparison.FinalReportHash = report.Hash
	comparison.Hash, err = hashClosedLoopV3FinalComparison(comparison)
	if err != nil {
		t.Fatal(err)
	}
	matrix := closedLoopV3FinalPromotionMatrix{
		Schema: closedLoopV3FinalPromotionSchema, Version: closedLoopV3BaselineVersion,
		FinalReportHash: report.Hash, RequiredGates: closedLoopRequiredPromotionGates(), Promotions: promotions,
	}
	matrix.Hash, err = hashClosedLoopV3FinalPromotionMatrix(matrix)
	if err != nil {
		t.Fatal(err)
	}
	seal := closedLoopV3HeldOutFinalSeal{
		Schema: closedLoopV3HeldOutFinalSealSchema, Version: closedLoopV3BaselineVersion,
		Algorithm: closedLoopV3HeldOutSealAlgorithm, CorpusManifestHash: payload.CorpusManifestHash,
		SelectionHash: selection.Hash, ImplementationHash: implementation.Hash,
		BaselinePayloadHash: baselinePayload.Hash, PayloadHash: payload.Hash,
		AggregateHash: heldOut.Hash, CiphertextHash: corpusHash(ciphertext), CaseCount: len(heldOut.Cases),
	}
	seal.Hash, err = hashClosedLoopV3HeldOutFinalSeal(seal)
	if err != nil {
		t.Fatal(err)
	}
	writeClosedLoopV3Final(t, discovery.Cases, report, comparison, matrix, seal, ciphertext)
}

func loadClosedLoopV3DiscoveryBaselineReport(t *testing.T) closedLoopV3DiscoveryBaselineReport {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V3_DISCOVERY_BASELINE_REPORT.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V3_DISCOVERY_BASELINE_REPORT.sha256"), filepath.Base(path), data)
	var report closedLoopV3DiscoveryBaselineReport
	decodeCorpusStrict(t, data, &report)
	return report
}

func loadClosedLoopV3HeldOutBaselinePayload(t *testing.T, selection closedLoopV3Selection) closedLoopV3HeldOutPayload {
	t.Helper()
	key, err := os.ReadFile(os.Getenv(closedLoopV3HeldOutBaselineKeyEnv))
	if err != nil || len(key) != 32 {
		t.Fatal("external V3 held-out baseline key is invalid")
	}
	var seal closedLoopV3HeldOutSeal
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopSpecDirectory(t), "V3_HELD_OUT_BASELINE_SEAL.json")), &seal)
	plaintext, err := openClosedLoopV3HeldOutPayload(key, seal.CorpusManifestHash, selection.Hash, seal.PayloadHash,
		mustCorpusRead(t, filepath.Join(closedLoopV3BaselineRoot, closedLoopV3HeldOutBaselineFile)))
	if err != nil {
		t.Fatal("V3 held-out baseline authentication failed")
	}
	var payload closedLoopV3HeldOutPayload
	decodeCorpusStrict(t, plaintext, &payload)
	if payload.Hash != seal.PayloadHash || payload.SelectionHash != selection.Hash || len(payload.Cases) != closedLoopCorpusSize/2 {
		t.Fatal("V3 held-out baseline payload contract is invalid")
	}
	return payload
}

func buildClosedLoopV3FinalReport(
	t *testing.T,
	manifest closedLoopV3CorpusManifest,
	manifestHash string,
	selection closedLoopV3Selection,
	baseline closedLoopV3DiscoveryBaselineReport,
	heldOutBaseline closedLoopV3HeldOutPayload,
	implementation closedLoopV3ImplementationSeal,
	discovery AggregateReport,
	heldOut AggregateReport,
	promotions []closedLoopPromotionRow,
) closedLoopV3FinalReport {
	t.Helper()
	report := closedLoopV3FinalReport{
		Schema: closedLoopV3FinalSchema, Version: closedLoopV3BaselineVersion,
		CorpusManifestHash: manifestHash, FreezeCommit: closedLoopV3SelectionCommit,
		SelectionHash: selection.Hash, DiscoveryBaselineHash: baseline.Hash,
		HeldOutBaselinePayloadHash: heldOutBaseline.Hash, ImplementationSealHash: implementation.Hash,
		EvaluatorPolicy: manifest.EvaluatorPolicy, ImpactRegistryHash: manifest.ImpactRegistryHash,
		SynthesisPolicyHash: manifest.SynthesisPolicyHash, Environment: manifest.Environment,
		OutcomeCounts: closedLoopOutcomeCounts(append(append([]CaseEvidence{}, discovery.Cases...), heldOut.Cases...)),
		Discovery:     discovery, HeldOutAggregateHash: heldOut.Hash,
		Attribution: closedLoopAttribution{
			SelectedClusterKey: selection.Cluster.Key, GenericArtifacts: implementation.Artifacts,
			DiscoveryPromotionHashes: promotionHashes(promotions, RoleDiscovery),
			HeldOutPromotionHashes:   promotionHashes(promotions, RoleHeldOut),
		},
	}
	var err error
	report.Hash, err = hashClosedLoopV3FinalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func buildClosedLoopV3FinalComparison(
	t *testing.T,
	selection closedLoopV3Selection,
	discoveryBefore, discoveryAfter, heldOutBefore, heldOutAfter []CaseEvidence,
) closedLoopV3FinalComparison {
	t.Helper()
	allBefore := append(slices.Clone(discoveryBefore), heldOutBefore...)
	allAfter := append(slices.Clone(discoveryAfter), heldOutAfter...)
	rankBefore, rankAfter, complete := selectedV3PassCounts(selection, allBefore, allAfter)
	if !complete {
		t.Fatal("V3 rank-one comparison case sets do not match")
	}
	comparison := closedLoopV3FinalComparison{
		Schema: closedLoopV3FinalComparisonSchema, Version: closedLoopV3BaselineVersion,
		SelectionHash:       selection.Hash,
		DiscoveryPassBefore: countV3Outcome(discoveryBefore, OutcomePass), DiscoveryPassAfter: countV3Outcome(discoveryAfter, OutcomePass),
		HeldOutPassBefore: countV3Outcome(heldOutBefore, OutcomePass), HeldOutPassAfter: countV3Outcome(heldOutAfter, OutcomePass),
		RankOneAffectedPassBefore: rankBefore, RankOneAffectedPassAfter: rankAfter,
		NoBaselinePassRegression: noV3PassRegression(discoveryBefore, discoveryAfter) && noV3PassRegression(heldOutBefore, heldOutAfter),
		UnsafeEvidencePreserved:  v3UnsafePreserved(discoveryBefore, discoveryAfter) && v3UnsafePreserved(heldOutBefore, heldOutAfter),
		RemainingGapsStable: v3RemainingGapsStable(discoveryBefore, discoveryAfter, selection.Cluster.Capability) &&
			v3RemainingGapsStable(heldOutBefore, heldOutAfter, selection.Cluster.Capability),
	}
	return comparison
}

func verifyClosedLoopV3Comparison(t *testing.T, comparison closedLoopV3FinalComparison) {
	t.Helper()
	if comparison.DiscoveryPassAfter <= comparison.DiscoveryPassBefore ||
		comparison.HeldOutPassAfter <= comparison.HeldOutPassBefore ||
		comparison.RankOneAffectedPassAfter <= comparison.RankOneAffectedPassBefore ||
		!comparison.NoBaselinePassRegression || !comparison.UnsafeEvidencePreserved || !comparison.RemainingGapsStable {
		t.Fatalf("V3 strict improvement failed: %#v", comparison)
	}
}

func selectedV3PassCounts(selection closedLoopV3Selection, before, after []CaseEvidence) (int, int, bool) {
	beforeByID, afterByID := closedLoopV3CaseMap(before), closedLoopV3CaseMap(after)
	affected := make(map[string]bool, len(selection.Cluster.Cases))
	for _, id := range selection.Cluster.Cases {
		affected[id] = true
	}
	for _, current := range before {
		for _, gap := range current.Gaps {
			if gap.Stage == selection.Cluster.Stage && gap.Scope == selection.Cluster.Scope &&
				gap.Capability == selection.Cluster.Capability && gap.Code == selection.Cluster.Code {
				affected[current.Case.ID] = true
				break
			}
		}
	}
	beforeCount, afterCount := 0, 0
	for id := range affected {
		beforeCase, beforeFound := beforeByID[id]
		afterCase, afterFound := afterByID[id]
		if !beforeFound || !afterFound {
			return 0, 0, false
		}
		if beforeCase.Outcome == OutcomePass {
			beforeCount++
		}
		if afterCase.Outcome == OutcomePass {
			afterCount++
		}
	}
	return beforeCount, afterCount, true
}

func TestSelectedV3PassCountsIncludesHeldOutClusterCases(t *testing.T) {
	selection := closedLoopV3Selection{Cluster: Cluster{
		Stage: "simulation", Scope: ScopeSimulation,
		Capability: "dc_operating_point_solver", Code: "SIMULATION_INVALID",
		Cases: []string{"discovery_case"},
	}}
	matchingGap := Gap{
		Stage: "simulation", Scope: ScopeSimulation,
		Capability: "dc_operating_point_solver", Code: "SIMULATION_INVALID",
	}
	before := []CaseEvidence{
		{Case: CaseMeta{ID: "discovery_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{matchingGap}},
		{Case: CaseMeta{ID: "held_out_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{matchingGap}},
		{Case: CaseMeta{ID: "other_case"}, Outcome: OutcomeUnsupported, Gaps: []Gap{{
			Stage: "simulation", Scope: ScopeSimulation, Capability: "transient_solver", Code: "SIMULATION_INVALID",
		}}},
	}
	after := []CaseEvidence{
		{Case: CaseMeta{ID: "discovery_case"}, Outcome: OutcomePass},
		{Case: CaseMeta{ID: "held_out_case"}, Outcome: OutcomePass},
		{Case: CaseMeta{ID: "other_case"}, Outcome: OutcomePass},
	}

	beforeCount, afterCount, complete := selectedV3PassCounts(selection, before, after)
	if beforeCount != 0 || afterCount != 2 || !complete {
		t.Fatalf("selected rank counts = (%d, %d, %t), want (0, 2, true)", beforeCount, afterCount, complete)
	}
}

func TestSelectedV3PassCountsRejectsMismatchedCaseSets(t *testing.T) {
	selection := closedLoopV3Selection{Cluster: Cluster{Cases: []string{"affected"}}}
	before := []CaseEvidence{{Case: CaseMeta{ID: "affected"}, Outcome: OutcomeUnsupported}}
	if _, _, complete := selectedV3PassCounts(selection, before, nil); complete {
		t.Fatal("mismatched selected case sets were accepted")
	}
}

func countV3Outcome(cases []CaseEvidence, outcome Outcome) int {
	count := 0
	for _, current := range cases {
		if current.Outcome == outcome {
			count++
		}
	}
	return count
}

func closedLoopV3CaseMap(cases []CaseEvidence) map[string]CaseEvidence {
	result := make(map[string]CaseEvidence, len(cases))
	for _, current := range cases {
		result[current.Case.ID] = current
	}
	return result
}

func noV3PassRegression(before, after []CaseEvidence) bool {
	afterByID := closedLoopV3CaseMap(after)
	for _, current := range before {
		if current.Outcome == OutcomePass && afterByID[current.Case.ID].Outcome != OutcomePass {
			return false
		}
	}
	return true
}

func v3UnsafePreserved(before, after []CaseEvidence) bool {
	afterByID := closedLoopV3CaseMap(after)
	for _, current := range before {
		if current.Outcome == OutcomeUnsafe && afterByID[current.Case.ID].Outcome == OutcomePass {
			return false
		}
	}
	return true
}

func v3RemainingGapsStable(before, after []CaseEvidence, selectedCapability string) bool {
	afterByID := closedLoopV3CaseMap(after)
	for _, current := range before {
		next := afterByID[current.Case.ID]
		if next.Outcome == OutcomePass {
			continue
		}
		if !slices.Equal(v3GapKeysExcept(current, selectedCapability), v3GapKeysExcept(next, selectedCapability)) {
			return false
		}
	}
	return true
}

func v3GapKeysExcept(current CaseEvidence, capability string) []string {
	keys := []string{}
	for _, gap := range current.Gaps {
		if gap.Capability == capability {
			continue
		}
		scope := string(gap.Scope)
		keys = append(keys, fmt.Sprintf("%d:%s%d:%s%d:%s%d:%s",
			len(gap.Stage), gap.Stage, len(scope), scope,
			len(gap.Capability), gap.Capability, len(gap.Code), gap.Code))
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func closedLoopV3PromotionRows(cases []CaseEvidence) []closedLoopPromotionRow {
	rows := make([]closedLoopPromotionRow, 0, len(cases))
	for _, current := range cases {
		if current.Outcome != OutcomePass || current.PromotionHash == "" || current.ProjectHash == "" {
			continue
		}
		rows = append(rows, closedLoopPromotionRow{
			CaseID: current.Case.ID, Role: current.Case.Role, SynthesisHash: current.SynthesisHash,
			PromotionHash: current.PromotionHash, ProjectHash: current.ProjectHash,
			Status: ots.PhysicalPromotionPassed, CleanRootRuns: 2, ReplayIdentical: true,
		})
	}
	slices.SortFunc(rows, func(left, right closedLoopPromotionRow) int { return cmp.Compare(left.CaseID, right.CaseID) })
	return rows
}

func loadClosedLoopV3ImplementationSeal(t *testing.T) closedLoopV3ImplementationSeal {
	t.Helper()
	path := filepath.Join(closedLoopSpecDirectory(t), "V3_REVIEWED_IMPLEMENTATION.json")
	data := mustCorpusRead(t, path)
	assertArtifactChecksum(t, filepath.Join(closedLoopSpecDirectory(t), "V3_REVIEWED_IMPLEMENTATION.sha256"), filepath.Base(path), data)
	var seal closedLoopV3ImplementationSeal
	decodeCorpusStrict(t, data, &seal)
	if seal.Schema != closedLoopV3ImplementationSealSchema || seal.Version != closedLoopV3BaselineVersion ||
		seal.SelectedCapability != "dc_operating_point_solver" ||
		seal.Review != "prism_reviewed_no_actionable_findings" || len(seal.Artifacts) == 0 {
		t.Fatal("V3 reviewed implementation seal metadata is invalid")
	}
	if want, err := hashClosedLoopV3ImplementationSeal(seal); err != nil || want != seal.Hash {
		t.Fatal("V3 reviewed implementation seal hash is invalid")
	}
	for _, artifact := range seal.Artifacts {
		if corpusHash(mustCorpusRead(t, filepath.Join(closedLoopModuleRoot(t), filepath.FromSlash(artifact.Path)))) != artifact.SHA256 {
			t.Fatalf("V3 reviewed implementation artifact drifted: %s", artifact.Path)
		}
	}
	return seal
}

func refuseExistingClosedLoopV3Final(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		closedLoopV3FinalRoot,
		filepath.Join(closedLoopSpecDirectory(t), "V3_FINAL_REPORT.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V3_FINAL_COMPARISON.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V3_PROMOTION_MATRIX.json"),
		filepath.Join(closedLoopSpecDirectory(t), "V3_HELD_OUT_FINAL_SEAL.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("V3 final is one-time and refuses existing artifact: %s", path)
		}
	}
}

func writeClosedLoopV3Final(
	t *testing.T,
	discovery []CaseEvidence,
	report closedLoopV3FinalReport,
	comparison closedLoopV3FinalComparison,
	matrix closedLoopV3FinalPromotionMatrix,
	seal closedLoopV3HeldOutFinalSeal,
	ciphertext []byte,
) {
	t.Helper()
	root := filepath.Join(closedLoopV3FinalRoot, "discovery")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, current := range discovery {
		if !portableArtifactID(current.Case.ID) {
			t.Fatal("V3 discovery case ID is not a safe artifact filename")
		}
		if err := os.WriteFile(filepath.Join(root, current.Case.ID+".json"), corpusJSON(t, current), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(closedLoopV3FinalRoot, closedLoopV3HeldOutFinalFile), ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	specRoot := closedLoopSpecDirectory(t)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_FINAL_REPORT.json"), report)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_FINAL_COMPARISON.json"), comparison)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_PROMOTION_MATRIX.json"), matrix)
	writeClosedLoopArtifact(t, filepath.Join(specRoot, "V3_HELD_OUT_FINAL_SEAL.json"), seal)
}

func portableArtifactID(id string) bool {
	if id == "" {
		return false
	}
	for _, character := range id {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	switch strings.ToUpper(id) {
	case "CON", "PRN", "AUX", "NUL",
		"COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	default:
		return true
	}
}

func TestPortableArtifactID(t *testing.T) {
	for _, id := range []string{"v3_case_001", "logic-edge"} {
		if !portableArtifactID(id) {
			t.Fatalf("portable artifact ID %q was rejected", id)
		}
	}
	for _, id := range []string{"", ".", "../case", `dir\case`, "case:name", "CON", "com0", "lpt0"} {
		if portableArtifactID(id) {
			t.Fatalf("non-portable artifact ID %q was accepted", id)
		}
	}
}

func hashClosedLoopV3ImplementationSeal(value closedLoopV3ImplementationSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV3FinalReport(value closedLoopV3FinalReport) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV3FinalComparison(value closedLoopV3FinalComparison) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV3FinalPromotionMatrix(value closedLoopV3FinalPromotionMatrix) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV3HeldOutFinalPayload(value closedLoopV3HeldOutFinalPayload) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV3HeldOutFinalSeal(value closedLoopV3HeldOutFinalSeal) (string, error) {
	value.Hash = ""
	return digest(value)
}

func TestClosedLoopV3FinalHashesRejectMutation(t *testing.T) {
	comparison := closedLoopV3FinalComparison{Schema: closedLoopV3FinalComparisonSchema, Version: closedLoopV3BaselineVersion}
	first, err := hashClosedLoopV3FinalComparison(comparison)
	if err != nil {
		t.Fatal(err)
	}
	comparison.DiscoveryPassAfter++
	second, err := hashClosedLoopV3FinalComparison(comparison)
	if err != nil || first == second {
		t.Fatal("V3 final comparison hash did not bind mutation")
	}
}
