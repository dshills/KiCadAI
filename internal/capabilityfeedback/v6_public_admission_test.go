package capabilityfeedback

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilitybundles"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV6PublicAdmissionSchema    = "kicadai.closed-loop-open-set-public-admission.v6"
	closedLoopV6PublicAdmissionRoot      = "testdata/closed_loop_open_set_v6_public_admission"
	closedLoopV6PublicAdmissionUpdateEnv = "UPDATE_CLOSED_LOOP_V6_PUBLIC_ADMISSION"
	closedLoopV6PublicRetirementSchema   = "kicadai.closed-loop-open-set-public-retirement.v6"
	closedLoopV6PublicRetirementRoot     = "testdata/closed_loop_open_set_v6_public_retirement"
)

type closedLoopV6PublicAdmissionComparison struct {
	DiscoveryPassBefore           int  `json:"discovery_pass_before"`
	DiscoveryPassAfter            int  `json:"discovery_pass_after"`
	ClaimedUnlockPassBefore       int  `json:"claimed_unlock_pass_before"`
	ClaimedUnlockPassAfter        int  `json:"claimed_unlock_pass_after"`
	AtLeastOneClaimedUnlock       bool `json:"at_least_one_claimed_unlock"`
	ExactUniqueCaseSet            bool `json:"exact_unique_case_set"`
	NoBaselinePassRegression      bool `json:"no_baseline_pass_regression"`
	NoBaselineUnsafeBecamePass    bool `json:"no_baseline_unsafe_became_pass"`
	SelectedMemberRemovalOnly     bool `json:"selected_member_removal_only"`
	DeterministicReplayComplete   bool `json:"deterministic_replay_complete"`
	PhysicalPromotionComplete     bool `json:"physical_promotion_complete"`
	SynthesisEnvironmentPreserved bool `json:"synthesis_environment_preserved"`
	PromotionEnvironmentPreserved bool `json:"promotion_environment_preserved"`
}

type closedLoopV6PublicAdmissionReport struct {
	Schema                    string                                `json:"schema"`
	Version                   int                                   `json:"version"`
	AdmissionCommit           string                                `json:"admission_commit"`
	CorpusManifestSHA256      string                                `json:"corpus_manifest_sha256"`
	BaselineSHA256            string                                `json:"baseline_sha256"`
	SelectionSHA256           string                                `json:"selection_sha256"`
	ImplementationSHA256      string                                `json:"implementation_sha256"`
	EvaluatorPolicy           string                                `json:"evaluator_policy"`
	ImpactRegistryFileSHA256  string                                `json:"impact_registry_file_sha256"`
	SynthesisPolicyFileSHA256 string                                `json:"synthesis_policy_file_sha256"`
	GapPolicyFileSHA256       string                                `json:"gap_policy_file_sha256"`
	SelectionPolicySHA256     string                                `json:"selection_policy_sha256"`
	InventorySHA256           string                                `json:"inventory_sha256"`
	CatalogSHA256             string                                `json:"catalog_sha256"`
	ModelRegistrySHA256       string                                `json:"model_registry_sha256"`
	SynthesisPolicySHA256     string                                `json:"synthesis_policy_sha256"`
	PromotionEnvironment      closedLoopV5PromotionEnvironment      `json:"promotion_environment"`
	OutcomeCountsBefore       []closedLoopOutcomeCount              `json:"outcome_counts_before"`
	OutcomeCountsAfter        []closedLoopOutcomeCount              `json:"outcome_counts_after"`
	CaseArtifacts             []closedLoopV6ArtifactRef             `json:"case_artifacts"`
	Discovery                 AggregateReport                       `json:"discovery"`
	Comparison                closedLoopV6PublicAdmissionComparison `json:"comparison"`
	Hash                      string                                `json:"hash"`
}

type closedLoopV6PublicRetirement struct {
	Schema                 string                                `json:"schema"`
	Version                int                                   `json:"version"`
	SelectionSHA256        string                                `json:"selection_sha256"`
	ImplementationHash     string                                `json:"implementation_hash"`
	Comparison             closedLoopV6PublicAdmissionComparison `json:"comparison"`
	HeldOutSourceOpened    bool                                  `json:"held_out_source_opened"`
	HeldOutBaselineOpened  bool                                  `json:"held_out_baseline_opened"`
	HeldOutFinalKeyCreated bool                                  `json:"held_out_final_key_created"`
	Hash                   string                                `json:"hash"`
}

func TestClosedLoopV6PublicAdmissionArtifactsAreFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV6PublicAdmissionRoot); os.IsNotExist(err) {
		t.Skip("V6 public admission has not been published")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(closedLoopV6PublicRetirementRoot); !os.IsNotExist(err) {
		t.Fatal("successful V6 public admission and retirement artifacts coexist")
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV6PublicAdmissionRoot,
		filepath.Join(closedLoopV6PublicAdmissionRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V6 public admission checksums: %v", err)
	}

	manifest := loadClosedLoopV6Manifest(t)
	baseline := loadClosedLoopV6FrozenBaselineReport(t)
	selection := loadClosedLoopV6FrozenSelection(t)
	implementation := loadClosedLoopV6HistoricalImplementationSeal(t)
	reportBytes := mustCorpusRead(t, filepath.Join(closedLoopV6PublicAdmissionRoot, "report.json"))
	var report closedLoopV6PublicAdmissionReport
	decodeCorpusStrict(t, reportBytes, &report)
	assertClosedLoopV6PublicAdmissionBindings(t, report, baseline, selection, implementation)
	if want, err := hashClosedLoopV6PublicAdmissionReport(report); err != nil || want != report.Hash {
		t.Fatal("V6 public admission report hash is invalid")
	}
	artifacts := loadClosedLoopV6PublicAdmissionCaseArtifacts(t, manifest, report.CaseArtifacts)
	cases := closedLoopV6ArtifactCases(artifacts)
	registry, _ := closedLoopV6Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal("reaggregate frozen V6 public admission")
	}
	rebuilt := buildClosedLoopV6PublicAdmissionReport(
		t, report.AdmissionCommit, baseline, selection, implementation,
		artifacts, report.CaseArtifacts, discovery, report.PromotionEnvironment,
	)
	if !bytes.Equal(reportBytes, corpusJSON(t, rebuilt)) {
		t.Fatal("V6 public admission report does not reproduce from frozen evidence")
	}
	if !closedLoopV6PublicAdmissionPasses(rebuilt.Comparison) {
		t.Fatal("V6 public admission does not satisfy every strict gate")
	}
	if audit := mustCorpusRead(t, filepath.Join(closedLoopV6PublicAdmissionRoot, "ADMISSION_AUDIT.md")); !bytes.Equal(audit, closedLoopV6PublicAdmissionAudit(rebuilt)) {
		t.Fatal("V6 public admission audit does not reproduce")
	}
	assertClosedLoopV6PublicAdmissionFileSet(t, manifest)
}

func TestUpdateClosedLoopV6PublicAdmission(t *testing.T) {
	if os.Getenv(closedLoopV6PublicAdmissionUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V6_PUBLIC_ADMISSION=1 to run the public-only V6 admission")
	}
	for _, root := range []string{closedLoopV6PublicAdmissionRoot, closedLoopV6PublicRetirementRoot} {
		if _, err := os.Stat(root); err == nil {
			t.Fatalf("V6 public admission is already consumed at %s", root)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat V6 public admission state %s: %v", root, err)
		}
	}
	repositoryRoot := closedLoopModuleRoot(t)
	admissionCommit := closedLoopV5CleanPublisherCommit(t, repositoryRoot)
	implementation := loadClosedLoopV6CurrentImplementationSeal(t)
	manifest := loadClosedLoopV6Manifest(t)
	baseline := loadClosedLoopV6FrozenBaselineReport(t)
	selection := loadClosedLoopV6FrozenSelection(t)
	registry, synthesisPolicy := closedLoopV6Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	promotionEnvironment := resolveClosedLoopV6PromotionEnvironment(t, repositoryRoot)
	artifacts := runClosedLoopV6DiscoveryBaseline(
		t, manifest, synthesisPolicy, inventory, environment, promotionEnvironment,
	)
	cases := closedLoopV6ArtifactCases(artifacts)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal("aggregate V6 public admission")
	}
	refs := make([]closedLoopV6ArtifactRef, len(artifacts))
	for index, artifact := range artifacts {
		refs[index] = closedLoopV6ArtifactRef{
			CaseID: artifact.CaseID,
			Path:   filepath.ToSlash(filepath.Join("discovery", artifact.CaseID+".json.gz")),
		}
	}
	report := buildClosedLoopV6PublicAdmissionReport(
		t, admissionCommit, baseline, selection, implementation,
		artifacts, refs, discovery, promotionEnvironment.Public,
	)
	if !closedLoopV6PublicAdmissionPasses(report.Comparison) {
		publishClosedLoopV6PublicRetirement(t, selection, implementation, report.Comparison)
		t.Fatal("V6 public admission failed and V6 was permanently retired before held-out access")
	}
	if err := atomicdir.Publish(closedLoopV6PublicAdmissionRoot, func(root string) error {
		publishedRefs, err := writeClosedLoopV6CaseArtifacts(root, artifacts)
		if err != nil {
			return err
		}
		report.CaseArtifacts = publishedRefs
		report.Hash, err = hashClosedLoopV6PublicAdmissionReport(report)
		if err != nil {
			return err
		}
		if !closedLoopV6PublicAdmissionPasses(report.Comparison) {
			return fmt.Errorf("V6 public admission changed during publication")
		}
		reportBytes, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		reportBytes = append(reportBytes, '\n')
		if err := os.WriteFile(filepath.Join(root, "report.json"), reportBytes, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "ADMISSION_AUDIT.md"), closedLoopV6PublicAdmissionAudit(report), 0o644); err != nil {
			return err
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	assertClosedLoopV6PublicAdmissionFileSet(t, manifest)
	t.Logf(
		"V6 public admission passed: discovery=%d->%d claimed=%d->%d",
		report.Comparison.DiscoveryPassBefore, report.Comparison.DiscoveryPassAfter,
		report.Comparison.ClaimedUnlockPassBefore, report.Comparison.ClaimedUnlockPassAfter,
	)
}

func TestClosedLoopV6PublicRetirementIsFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV6PublicRetirementRoot); os.IsNotExist(err) {
		t.Skip("V6 public admission has not been retired")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(closedLoopV6PublicAdmissionRoot); !os.IsNotExist(err) {
		t.Fatal("retired V6 must not contain successful public admission artifacts")
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV6PublicRetirementRoot,
		filepath.Join(closedLoopV6PublicRetirementRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V6 public retirement checksums: %v", err)
	}
	var retirement closedLoopV6PublicRetirement
	decodeCorpusStrict(t, mustCorpusRead(t, filepath.Join(closedLoopV6PublicRetirementRoot, "retirement.json")), &retirement)
	if retirement.Schema != closedLoopV6PublicRetirementSchema || retirement.Version != closedLoopV6BaselineVersion ||
		retirement.HeldOutSourceOpened || retirement.HeldOutBaselineOpened || retirement.HeldOutFinalKeyCreated ||
		closedLoopV6PublicAdmissionPasses(retirement.Comparison) {
		t.Fatal("V6 public retirement boundary is invalid")
	}
	if want, err := hashClosedLoopV6PublicRetirement(retirement); err != nil || want != retirement.Hash {
		t.Fatal("V6 public retirement hash is invalid")
	}
	assertClosedLoopV6PublicRetirementFileSet(t)
}

func buildClosedLoopV6PublicAdmissionReport(
	t *testing.T,
	admissionCommit string,
	baseline closedLoopV6BaselineReport,
	selection closedLoopV6Selection,
	implementation closedLoopV6ImplementationSeal,
	artifacts []closedLoopV6CaseArtifact,
	refs []closedLoopV6ArtifactRef,
	discovery AggregateReport,
	promotionEnvironment closedLoopV5PromotionEnvironment,
) closedLoopV6PublicAdmissionReport {
	t.Helper()
	inventoryHash, catalogHash, modelRegistryHash, synthesisPolicyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
	comparison := closedLoopV6PublicAdmissionComparison{
		DiscoveryPassBefore:         closedLoopV5OutcomeCount(baseline.Discovery.Cases, OutcomePass),
		DiscoveryPassAfter:          closedLoopV5OutcomeCount(discovery.Cases, OutcomePass),
		ClaimedUnlockPassBefore:     closedLoopV6ClaimedPassCount(selection, baseline.Discovery.Cases),
		ClaimedUnlockPassAfter:      closedLoopV6ClaimedPassCount(selection, discovery.Cases),
		ExactUniqueCaseSet:          closedLoopV5ExactCaseSet(baseline.Discovery.Cases, discovery.Cases),
		NoBaselinePassRegression:    closedLoopV5NoPassRegression(baseline.Discovery.Cases, discovery.Cases),
		NoBaselineUnsafeBecamePass:  closedLoopV5UnsafePreserved(baseline.Discovery.Cases, discovery.Cases),
		SelectedMemberRemovalOnly:   closedLoopV6RemainingGapsStable(baseline.Discovery.Cases, discovery.Cases, selection),
		DeterministicReplayComplete: closedLoopV6ReplayEvidenceComplete(artifacts),
		PhysicalPromotionComplete:   closedLoopV6PromotionEvidenceComplete(artifacts),
		SynthesisEnvironmentPreserved: inventoryHash == baseline.InventorySHA256 &&
			catalogHash == baseline.CatalogSHA256 && modelRegistryHash == baseline.ModelRegistrySHA256 &&
			synthesisPolicyHash == baseline.SynthesisPolicySHA256,
		PromotionEnvironmentPreserved: promotionEnvironment.Hash == baseline.PromotionEnvironment.Hash,
	}
	comparison.AtLeastOneClaimedUnlock = comparison.ClaimedUnlockPassAfter > comparison.ClaimedUnlockPassBefore
	report := closedLoopV6PublicAdmissionReport{
		Schema: closedLoopV6PublicAdmissionSchema, Version: closedLoopV6BaselineVersion,
		AdmissionCommit: admissionCommit, CorpusManifestSHA256: closedLoopV6CorpusManifestHash,
		BaselineSHA256: baseline.Hash, SelectionSHA256: selection.Hash, ImplementationSHA256: implementation.Hash,
		EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistryFileSHA256: closedLoopV5ImpactRegistryFileHash,
		SynthesisPolicyFileSHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicyFileSHA256: closedLoopV5GapPolicyFileHash,
		SelectionPolicySHA256: closedLoopV6SelectionPolicyHash, InventorySHA256: inventoryHash,
		CatalogSHA256: catalogHash, ModelRegistrySHA256: modelRegistryHash, SynthesisPolicySHA256: synthesisPolicyHash,
		PromotionEnvironment: promotionEnvironment, OutcomeCountsBefore: closedLoopOutcomeCounts(baseline.Discovery.Cases),
		OutcomeCountsAfter: closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs, Discovery: discovery,
		Comparison: comparison,
	}
	var err error
	report.Hash, err = hashClosedLoopV6PublicAdmissionReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func assertClosedLoopV6PublicAdmissionBindings(
	t *testing.T,
	report closedLoopV6PublicAdmissionReport,
	baseline closedLoopV6BaselineReport,
	selection closedLoopV6Selection,
	implementation closedLoopV6ImplementationSeal,
) {
	t.Helper()
	if report.Schema != closedLoopV6PublicAdmissionSchema || report.Version != closedLoopV6BaselineVersion ||
		!closedLoopV6ValidHash(report.AdmissionCommit) || report.CorpusManifestSHA256 != closedLoopV6CorpusManifestHash ||
		report.BaselineSHA256 != baseline.Hash || report.SelectionSHA256 != selection.Hash ||
		report.ImplementationSHA256 != implementation.Hash || report.EvaluatorPolicy != RealizabilityPolicyVersion ||
		report.ImpactRegistryFileSHA256 != closedLoopV5ImpactRegistryFileHash ||
		report.SynthesisPolicyFileSHA256 != closedLoopV5SynthesisPolicyFileHash ||
		report.GapPolicyFileSHA256 != closedLoopV5GapPolicyFileHash ||
		report.SelectionPolicySHA256 != closedLoopV6SelectionPolicyHash {
		t.Fatal("V6 public admission policy bindings are invalid")
	}
}

func closedLoopV6PublicAdmissionPasses(comparison closedLoopV6PublicAdmissionComparison) bool {
	return comparison.DiscoveryPassBefore >= 0 && comparison.ClaimedUnlockPassBefore >= 0 &&
		comparison.DiscoveryPassAfter > comparison.DiscoveryPassBefore &&
		comparison.ClaimedUnlockPassAfter > comparison.ClaimedUnlockPassBefore &&
		comparison.AtLeastOneClaimedUnlock && comparison.ExactUniqueCaseSet &&
		comparison.NoBaselinePassRegression && comparison.NoBaselineUnsafeBecamePass &&
		comparison.SelectedMemberRemovalOnly && comparison.DeterministicReplayComplete &&
		comparison.PhysicalPromotionComplete && comparison.SynthesisEnvironmentPreserved &&
		comparison.PromotionEnvironmentPreserved
}

func TestClosedLoopV6PublicAdmissionRequiresEveryStrictGate(t *testing.T) {
	valid := closedLoopV6PublicAdmissionComparison{
		DiscoveryPassBefore: 0, DiscoveryPassAfter: 1,
		ClaimedUnlockPassBefore: 0, ClaimedUnlockPassAfter: 1, AtLeastOneClaimedUnlock: true,
		ExactUniqueCaseSet: true, NoBaselinePassRegression: true, NoBaselineUnsafeBecamePass: true,
		SelectedMemberRemovalOnly: true, DeterministicReplayComplete: true, PhysicalPromotionComplete: true,
		SynthesisEnvironmentPreserved: true, PromotionEnvironmentPreserved: true,
	}
	if !closedLoopV6PublicAdmissionPasses(valid) {
		t.Fatal("complete V6 public admission evidence was rejected")
	}
	mutations := []func(*closedLoopV6PublicAdmissionComparison){
		func(value *closedLoopV6PublicAdmissionComparison) {
			value.DiscoveryPassAfter = value.DiscoveryPassBefore
		},
		func(value *closedLoopV6PublicAdmissionComparison) { value.ClaimedUnlockPassBefore = -1 },
		func(value *closedLoopV6PublicAdmissionComparison) {
			value.ClaimedUnlockPassAfter = value.ClaimedUnlockPassBefore
		},
		func(value *closedLoopV6PublicAdmissionComparison) { value.AtLeastOneClaimedUnlock = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.ExactUniqueCaseSet = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.NoBaselinePassRegression = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.NoBaselineUnsafeBecamePass = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.SelectedMemberRemovalOnly = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.DeterministicReplayComplete = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.PhysicalPromotionComplete = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.SynthesisEnvironmentPreserved = false },
		func(value *closedLoopV6PublicAdmissionComparison) { value.PromotionEnvironmentPreserved = false },
	}
	for index, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if closedLoopV6PublicAdmissionPasses(candidate) {
			t.Fatalf("V6 public admission accepted missing strict gate %d", index)
		}
	}
}

func TestClosedLoopV6PublicAdmissionPreservesExactSelectedMembers(t *testing.T) {
	selection := closedLoopV6Selection{}
	selection.Selected.Members = []capabilitybundles.Member{{
		Stage: "simulation", Scope: string(ScopeSimulation), Capability: "selected", Code: "SELECTED",
	}}
	selected := Gap{Stage: "simulation", Scope: ScopeSimulation, Capability: "selected", Code: "SELECTED"}
	remaining := Gap{
		Stage: "promotion", Scope: ScopePhysical, Capability: "physical_promotion", Code: "REMAINS",
		RequiredEvidence: []string{"connectivity", "drc"},
	}
	extra := Gap{Stage: "simulation", Scope: ScopeSimulation, Capability: "new_gap", Code: "NEW"}
	evidence := func(gaps ...Gap) []CaseEvidence {
		return []CaseEvidence{{Case: CaseMeta{ID: "case"}, Outcome: OutcomeUnsupported, Gaps: gaps}}
	}
	tests := []struct {
		name   string
		before []CaseEvidence
		after  []CaseEvidence
		want   bool
	}{
		{name: "equal", before: evidence(remaining), after: evidence(remaining), want: true},
		{name: "final superset", before: evidence(remaining), after: evidence(remaining, extra), want: true},
		{name: "selected member removed", before: evidence(selected, remaining), after: evidence(remaining), want: true},
		{name: "nonselected code removed", before: evidence(Gap{Stage: selected.Stage, Scope: selected.Scope, Capability: selected.Capability, Code: "OTHER"}), after: evidence(), want: false},
		{name: "required evidence changed", before: evidence(remaining), after: evidence(Gap{Stage: remaining.Stage, Scope: remaining.Scope, Capability: remaining.Capability, Code: remaining.Code, RequiredEvidence: []string{"drc"}}), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := closedLoopV6RemainingGapsStable(test.before, test.after, selection); got != test.want {
				t.Fatalf("closedLoopV6RemainingGapsStable() = %t, want %t", got, test.want)
			}
		})
	}
	duplicate := append(evidence(remaining), evidence(remaining)...)
	if closedLoopV6RemainingGapsStable(duplicate, evidence(remaining), selection) {
		t.Fatal("V6 preservation gate accepted duplicate case identities")
	}
}

func TestClosedLoopV6ClaimedPassCountFailsClosed(t *testing.T) {
	selection := closedLoopV6Selection{}
	selection.Selected.UnlockedCases = []string{"claimed"}
	cases := []CaseEvidence{{Case: CaseMeta{ID: "claimed"}, Outcome: OutcomePass}}
	if got := closedLoopV6ClaimedPassCount(selection, cases); got != 1 {
		t.Fatalf("V6 claimed pass count = %d, want 1", got)
	}
	if got := closedLoopV6ClaimedPassCount(selection, nil); got != -1 {
		t.Fatalf("V6 missing claimed case count = %d, want -1", got)
	}
	selection.Selected.UnlockedCases = []string{"claimed", "claimed"}
	if got := closedLoopV6ClaimedPassCount(selection, cases); got != -1 {
		t.Fatalf("V6 duplicate claimed case count = %d, want -1", got)
	}
}

func closedLoopV6ClaimedPassCount(selection closedLoopV6Selection, cases []CaseEvidence) int {
	byID, ok := closedLoopV5UniqueCases(cases)
	if !ok {
		return -1
	}
	seen := map[string]bool{}
	count := 0
	for _, id := range selection.Selected.UnlockedCases {
		if id == "" || seen[id] {
			return -1
		}
		seen[id] = true
		current, found := byID[id]
		if !found {
			return -1
		}
		if current.Outcome == OutcomePass {
			count++
		}
	}
	return count
}

func closedLoopV6RemainingGapsStable(before, after []CaseEvidence, selection closedLoopV6Selection) bool {
	beforeByID, afterByID, ok := closedLoopV5PairedCases(before, after)
	if !ok || len(selection.Selected.Members) == 0 {
		return false
	}
	for id, current := range beforeByID {
		next := afterByID[id]
		if next.Outcome == OutcomePass {
			continue
		}
		finalGaps := map[string]bool{}
		for _, gap := range next.Gaps {
			finalGaps[closedLoopV5GapIdentity(gap)] = true
		}
		for _, gap := range current.Gaps {
			if closedLoopV6SelectedGap(gap, selection) {
				continue
			}
			if !finalGaps[closedLoopV5GapIdentity(gap)] {
				return false
			}
		}
	}
	return true
}

func closedLoopV6SelectedGap(gap Gap, selection closedLoopV6Selection) bool {
	for _, member := range selection.Selected.Members {
		if gap.Stage == member.Stage && string(gap.Scope) == member.Scope &&
			gap.Capability == member.Capability && gap.Code == member.Code {
			return true
		}
	}
	return false
}

func closedLoopV6ArtifactCases(artifacts []closedLoopV6CaseArtifact) []CaseEvidence {
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	return cases
}

func closedLoopV6ReplayEvidenceComplete(artifacts []closedLoopV6CaseArtifact) bool {
	if len(artifacts) != closedLoopV6RoleSize {
		return false
	}
	for _, artifact := range artifacts {
		if len(artifact.Replays) != 2 || !closedLoopV6ValidHash(artifact.Replays[0].Hash) ||
			artifact.Replays[0].Hash != artifact.Observation.SynthesisHash {
			return false
		}
		if !reflect.DeepEqual(artifact.Replays[0], artifact.Replays[1]) {
			return false
		}
	}
	return true
}

func closedLoopV6PromotionEvidenceComplete(artifacts []closedLoopV6CaseArtifact) bool {
	if len(artifacts) != closedLoopV6RoleSize {
		return false
	}
	for _, artifact := range artifacts {
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != ots.PhysicalPromotionPassed ||
				!artifact.Promotion.ReplayIdentical || len(artifact.Promotion.Runs) != 2 ||
				!closedLoopV6ValidHash(artifact.Promotion.Hash) || !closedLoopV6ValidHash(artifact.Promotion.ProjectHash) ||
				artifact.Promotion.Hash != artifact.Observation.PromotionHash ||
				artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				return false
			}
		} else if artifact.Promotion != nil {
			return false
		}
	}
	return true
}

func loadClosedLoopV6PublicAdmissionCaseArtifacts(
	t *testing.T,
	manifest corpuspublication.Manifest,
	refs []closedLoopV6ArtifactRef,
) []closedLoopV6CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV6CaseArtifact, 0, closedLoopV6RoleSize)
	refIndex := 0
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		if refIndex >= len(refs) {
			t.Fatal("V6 public admission artifact references are incomplete")
		}
		ref := refs[refIndex]
		expectedPath := filepath.ToSlash(filepath.Join("discovery", entry.ID+".json.gz"))
		if ref.CaseID != entry.ID || ref.Path != expectedPath || !closedLoopV6ValidHash(ref.SHA256) {
			t.Fatalf("V6 public admission artifact reference %s is invalid", entry.ID)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV6PublicAdmissionRoot, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V6 public admission artifact %s checksum drifted", entry.ID)
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("V6 public admission artifact %s is not gzip: %v", entry.ID, err)
		}
		var artifact closedLoopV6CaseArtifact
		decoder := json.NewDecoder(reader)
		decodeErr := decoder.Decode(&artifact)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		closeErr := reader.Close()
		if decodeErr != nil || trailingErr != io.EOF || closeErr != nil {
			t.Fatalf("V6 public admission artifact %s has invalid JSON framing", entry.ID)
		}
		want, hashErr := hashClosedLoopV6CaseArtifact(artifact)
		if hashErr != nil || artifact.Hash != want ||
			artifact.Schema != closedLoopV6CaseArtifactSchema || artifact.Version != closedLoopV6BaselineVersion ||
			artifact.CaseID != entry.ID || artifact.RequirementSHA256 != entry.RequirementSHA256 {
			t.Fatalf("V6 public admission artifact %s is invalid", entry.ID)
		}
		artifacts = append(artifacts, artifact)
		refIndex++
	}
	if refIndex != len(refs) || len(artifacts) != closedLoopV6RoleSize ||
		!closedLoopV6ReplayEvidenceComplete(artifacts) || !closedLoopV6PromotionEvidenceComplete(artifacts) {
		t.Fatal("V6 public admission artifact set is invalid")
	}
	return artifacts
}

func closedLoopV6PublicAdmissionAudit(report closedLoopV6PublicAdmissionReport) []byte {
	return []byte(fmt.Sprintf(
		"# V6 Public Discovery Admission Audit\n\n"+
			"The sealed generic implementation passed public-only admission before any held-out source or baseline was opened.\n\n"+
			"- report hash: `%s`\n"+
			"- implementation seal: `%s`\n"+
			"- discovery pass uplift: %d -> %d\n"+
			"- claimed-unlock pass uplift: %d -> %d\n"+
			"- exact unique case set: yes\n"+
			"- baseline pass regressions: none\n"+
			"- unsafe-to-pass transitions: none\n"+
			"- removed baseline gaps: selected exact members only\n"+
			"- deterministic replay and installed-KiCad promotion evidence: complete\n"+
			"- held-out source opened: no\n"+
			"- held-out baseline opened: no\n",
		report.Hash, report.ImplementationSHA256,
		report.Comparison.DiscoveryPassBefore, report.Comparison.DiscoveryPassAfter,
		report.Comparison.ClaimedUnlockPassBefore, report.Comparison.ClaimedUnlockPassAfter,
	))
}

func publishClosedLoopV6PublicRetirement(
	t *testing.T,
	selection closedLoopV6Selection,
	implementation closedLoopV6ImplementationSeal,
	comparison closedLoopV6PublicAdmissionComparison,
) {
	t.Helper()
	retirement := closedLoopV6PublicRetirement{
		Schema: closedLoopV6PublicRetirementSchema, Version: closedLoopV6BaselineVersion,
		SelectionSHA256: selection.Hash, ImplementationHash: implementation.Hash, Comparison: comparison,
	}
	var err error
	retirement.Hash, err = hashClosedLoopV6PublicRetirement(retirement)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicdir.Publish(closedLoopV6PublicRetirementRoot, func(root string) error {
		if err := os.WriteFile(filepath.Join(root, "retirement.json"), corpusJSON(t, retirement), 0o644); err != nil {
			return err
		}
		audit := []byte(fmt.Sprintf(
			"# V6 Public Admission Retirement Audit\n\n"+
				"Public discovery admission failed a strict gate. V6 is permanently retired.\n\n"+
				"- retirement hash: `%s`\n"+
				"- held-out source opened: no\n"+
				"- held-out baseline opened: no\n"+
				"- held-out final key created: no\n"+
				"- final updater: permanently retired\n",
			retirement.Hash,
		))
		if err := os.WriteFile(filepath.Join(root, "RETIREMENT_AUDIT.md"), audit, 0o644); err != nil {
			return err
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
}

func assertClosedLoopV6PublicAdmissionFileSet(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	want := map[string]bool{
		"ADMISSION_AUDIT.md": true, corpuspublication.ChecksumFile: true, "report.json": true,
	}
	for _, entry := range manifest.Entries {
		if entry.Role == string(RoleDiscovery) {
			want[filepath.ToSlash(filepath.Join("discovery", entry.ID+".json.gz"))] = true
		}
	}
	assertClosedLoopV6ExactFileSet(t, closedLoopV6PublicAdmissionRoot, want)
}

func assertClosedLoopV6PublicRetirementFileSet(t *testing.T) {
	t.Helper()
	assertClosedLoopV6ExactFileSet(t, closedLoopV6PublicRetirementRoot, map[string]bool{
		corpuspublication.ChecksumFile: true, "retirement.json": true, "RETIREMENT_AUDIT.md": true,
	})
}

func assertClosedLoopV6ExactFileSet(t *testing.T, root string, want map[string]bool) {
	t.Helper()
	seen := map[string]bool{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular V6 public artifact %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !want[relative] {
			return fmt.Errorf("unexpected V6 public artifact %s", relative)
		}
		seen[relative] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(want) {
		t.Fatalf("V6 public artifact file count = %d, want %d", len(seen), len(want))
	}
}

func hashClosedLoopV6PublicAdmissionReport(value closedLoopV6PublicAdmissionReport) (string, error) {
	value.Hash = ""
	return digest(value)
}

func hashClosedLoopV6PublicRetirement(value closedLoopV6PublicRetirement) (string, error) {
	value.Hash = ""
	return digest(value)
}
