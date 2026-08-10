package capabilityfeedback

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilitypackages"
	"kicadai/internal/corpuspublication"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV5PublicAdmissionSchema    = "kicadai.closed-loop-open-set-public-admission.v5"
	closedLoopV5PublicAdmissionRoot      = "testdata/closed_loop_open_set_v5_public_admission"
	closedLoopV5PublicAdmissionUpdateEnv = "UPDATE_CLOSED_LOOP_V5_PUBLIC_ADMISSION"
)

type closedLoopV5PublicAdmissionComparison struct {
	DiscoveryPassBefore           int  `json:"discovery_pass_before"`
	DiscoveryPassAfter            int  `json:"discovery_pass_after"`
	SelectedAffectedPassBefore    int  `json:"selected_affected_pass_before"`
	SelectedAffectedPassAfter     int  `json:"selected_affected_pass_after"`
	ExactCaseSet                  bool `json:"exact_case_set"`
	NoBaselinePassRegression      bool `json:"no_baseline_pass_regression"`
	NoBaselineUnsafeBecamePass    bool `json:"no_baseline_unsafe_became_pass"`
	NonselectedGapsPreserved      bool `json:"nonselected_gaps_preserved"`
	DeterministicReplayComplete   bool `json:"deterministic_replay_complete"`
	PhysicalPromotionComplete     bool `json:"physical_promotion_complete"`
	SynthesisEnvironmentPreserved bool `json:"synthesis_environment_preserved"`
}

type closedLoopV5PublicAdmissionReport struct {
	Schema                  string                                `json:"schema"`
	Version                 int                                   `json:"version"`
	CorpusManifestSHA256    string                                `json:"corpus_manifest_sha256"`
	BaselineSHA256          string                                `json:"baseline_sha256"`
	SelectionSHA256         string                                `json:"selection_sha256"`
	ImplementationSHA256    string                                `json:"implementation_sha256"`
	EvaluatorPolicy         string                                `json:"evaluator_policy"`
	ImpactRegistrySHA256    string                                `json:"impact_registry_sha256"`
	SynthesisPolicySHA256   string                                `json:"synthesis_policy_sha256"`
	GapPolicySHA256         string                                `json:"gap_policy_sha256"`
	SelectionPolicySHA256   string                                `json:"selection_policy_sha256"`
	InventorySHA256         string                                `json:"inventory_sha256"`
	CatalogSHA256           string                                `json:"catalog_sha256"`
	ModelRegistrySHA256     string                                `json:"model_registry_sha256"`
	EnvironmentPolicySHA256 string                                `json:"environment_policy_sha256"`
	OutcomeCountsBefore     []closedLoopOutcomeCount              `json:"outcome_counts_before"`
	OutcomeCountsAfter      []closedLoopOutcomeCount              `json:"outcome_counts_after"`
	CaseArtifacts           []closedLoopV5ArtifactRef             `json:"case_artifacts"`
	Discovery               AggregateReport                       `json:"discovery"`
	Comparison              closedLoopV5PublicAdmissionComparison `json:"comparison"`
	Hash                    string                                `json:"hash"`
}

func TestClosedLoopV5PublicAdmissionArtifactsAreFrozen(t *testing.T) {
	if _, err := os.Stat(closedLoopV5PublicAdmissionRoot); os.IsNotExist(err) {
		t.Skip("V5 public admission has not been published")
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := corpuspublication.VerifyChecksumManifest(
		closedLoopV5PublicAdmissionRoot,
		filepath.Join(closedLoopV5PublicAdmissionRoot, corpuspublication.ChecksumFile),
	); err != nil {
		t.Fatalf("verify V5 public admission checksums: %v", err)
	}

	manifest := loadClosedLoopV5Manifest(t)
	baseline := loadClosedLoopV5FrozenBaselineReport(t)
	selection := loadClosedLoopV5FrozenSelection(t)
	implementation := loadClosedLoopV5HistoricalImplementationSeal(t)
	reportBytes := mustCorpusRead(t, filepath.Join(closedLoopV5PublicAdmissionRoot, "report.json"))
	var report closedLoopV5PublicAdmissionReport
	decodeCorpusStrict(t, reportBytes, &report)
	if report.Schema != closedLoopV5PublicAdmissionSchema || report.Version != closedLoopV5BaselineVersion ||
		report.CorpusManifestSHA256 != closedLoopV5CorpusManifestHash || report.BaselineSHA256 != baseline.Hash ||
		report.SelectionSHA256 != selection.Hash || report.ImplementationSHA256 != implementation.Hash ||
		report.EvaluatorPolicy != RealizabilityPolicyVersion || report.ImpactRegistrySHA256 != closedLoopV5ImpactRegistryFileHash ||
		report.SynthesisPolicySHA256 != closedLoopV5SynthesisPolicyFileHash || report.GapPolicySHA256 != closedLoopV5GapPolicyFileHash ||
		report.SelectionPolicySHA256 != closedLoopV5SelectionPolicyHash {
		t.Fatal("V5 public admission policy bindings are invalid")
	}
	if want, err := hashClosedLoopV5PublicAdmissionReport(report); err != nil || want != report.Hash {
		t.Fatal("V5 public admission report hash is invalid")
	}

	artifacts := loadClosedLoopV5PublicAdmissionCaseArtifacts(t, manifest, report.CaseArtifacts)
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	registry, _ := closedLoopV5Policies(t)
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal("reaggregate frozen V5 public admission")
	}
	rebuilt := buildClosedLoopV5PublicAdmissionReport(t, baseline, selection, implementation, artifacts, report.CaseArtifacts, discovery)
	if !bytes.Equal(reportBytes, corpusJSON(t, rebuilt)) {
		t.Fatal("V5 public admission report does not reproduce from frozen evidence")
	}
	if !closedLoopV5PublicAdmissionPasses(rebuilt.Comparison) {
		t.Fatal("V5 public admission does not satisfy every strict gate")
	}
	if audit := mustCorpusRead(t, filepath.Join(closedLoopV5PublicAdmissionRoot, "ADMISSION_AUDIT.md")); !bytes.Equal(audit, closedLoopV5PublicAdmissionAudit(rebuilt)) {
		t.Fatal("V5 public admission audit does not reproduce")
	}
	assertClosedLoopV5PublicAdmissionFileSet(t, manifest)
}

func TestUpdateClosedLoopV5PublicAdmission(t *testing.T) {
	if os.Getenv(closedLoopV5PublicAdmissionUpdateEnv) != "1" {
		t.Skip("set UPDATE_CLOSED_LOOP_V5_PUBLIC_ADMISSION=1 to publish the public-only V5 admission")
	}
	if _, err := os.Stat(closedLoopV5PublicAdmissionRoot); !os.IsNotExist(err) {
		t.Fatal("V5 public admission already exists; refusing overwrite")
	}
	implementation := loadClosedLoopV5CurrentImplementationSeal(t)
	manifest := loadClosedLoopV5Manifest(t)
	baseline := loadClosedLoopV5FrozenBaselineReport(t)
	selection := loadClosedLoopV5FrozenSelection(t)
	registry, synthesisPolicy := closedLoopV5Policies(t)
	inventory, environment := closedLoopSynthesisEnvironment(t)
	artifacts := runClosedLoopV5DiscoveryBaseline(t, manifest, synthesisPolicy, inventory, environment)
	cases := make([]CaseEvidence, len(artifacts))
	for index := range artifacts {
		cases[index] = artifacts[index].Observation
	}
	discovery, err := EvaluateRealizabilityAware(RoleDiscovery, cases, registry)
	if err != nil {
		t.Fatal("aggregate V5 public admission")
	}

	if err := atomicdir.Publish(closedLoopV5PublicAdmissionRoot, func(root string) error {
		refs, err := writeClosedLoopV5CaseArtifacts(root, artifacts)
		if err != nil {
			return err
		}
		report := buildClosedLoopV5PublicAdmissionReport(t, baseline, selection, implementation, artifacts, refs, discovery)
		if !closedLoopV5PublicAdmissionPasses(report.Comparison) {
			return fmt.Errorf("V5 public admission did not prove strict, preservation-safe uplift")
		}
		if err := os.WriteFile(filepath.Join(root, "report.json"), corpusJSON(t, report), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "ADMISSION_AUDIT.md"), closedLoopV5PublicAdmissionAudit(report), 0o644); err != nil {
			return err
		}
		return writeClosedLoopV5Checksums(root)
	}); err != nil {
		t.Fatal(err)
	}
	assertClosedLoopV5PublicAdmissionFileSet(t, manifest)
	t.Logf("V5 public admission passed: discovery=%d->%d selected=%d->%d",
		closedLoopV5OutcomeCount(baseline.Discovery.Cases, OutcomePass),
		closedLoopV5OutcomeCount(discovery.Cases, OutcomePass),
		closedLoopV5SelectedPassCount(selection, baseline.Discovery.Cases),
		closedLoopV5SelectedPassCount(selection, discovery.Cases),
	)
}

func buildClosedLoopV5PublicAdmissionReport(
	t *testing.T,
	baseline closedLoopV5BaselineReport,
	selection closedLoopV5Selection,
	implementation closedLoopV5ImplementationSeal,
	artifacts []closedLoopV5CaseArtifact,
	refs []closedLoopV5ArtifactRef,
	discovery AggregateReport,
) closedLoopV5PublicAdmissionReport {
	t.Helper()
	inventoryHash, catalogHash, modelRegistryHash, environmentPolicyHash := closedLoopV5EnvironmentBindings(t, discovery.Cases)
	comparison := closedLoopV5PublicAdmissionComparison{
		DiscoveryPassBefore:         closedLoopV5OutcomeCount(baseline.Discovery.Cases, OutcomePass),
		DiscoveryPassAfter:          closedLoopV5OutcomeCount(discovery.Cases, OutcomePass),
		SelectedAffectedPassBefore:  closedLoopV5SelectedPassCount(selection, baseline.Discovery.Cases),
		SelectedAffectedPassAfter:   closedLoopV5SelectedPassCount(selection, discovery.Cases),
		ExactCaseSet:                closedLoopV5ExactCaseSet(baseline.Discovery.Cases, discovery.Cases),
		NoBaselinePassRegression:    closedLoopV5NoPassRegression(baseline.Discovery.Cases, discovery.Cases),
		NoBaselineUnsafeBecamePass:  closedLoopV5UnsafePreserved(baseline.Discovery.Cases, discovery.Cases),
		NonselectedGapsPreserved:    closedLoopV5RemainingGapsStable(baseline.Discovery.Cases, discovery.Cases, selection),
		DeterministicReplayComplete: closedLoopV5ReplayEvidenceComplete(artifacts),
		PhysicalPromotionComplete:   closedLoopV5PromotionEvidenceComplete(artifacts),
		SynthesisEnvironmentPreserved: inventoryHash == baseline.InventorySHA256 && catalogHash == baseline.CatalogSHA256 &&
			modelRegistryHash == baseline.ModelRegistrySHA256 && environmentPolicyHash == baseline.SynthesisPolicySHA256,
	}
	report := closedLoopV5PublicAdmissionReport{
		Schema: closedLoopV5PublicAdmissionSchema, Version: closedLoopV5BaselineVersion,
		CorpusManifestSHA256: closedLoopV5CorpusManifestHash, BaselineSHA256: baseline.Hash,
		SelectionSHA256: selection.Hash, ImplementationSHA256: implementation.Hash,
		EvaluatorPolicy: RealizabilityPolicyVersion, ImpactRegistrySHA256: closedLoopV5ImpactRegistryFileHash,
		SynthesisPolicySHA256: closedLoopV5SynthesisPolicyFileHash, GapPolicySHA256: closedLoopV5GapPolicyFileHash,
		SelectionPolicySHA256: closedLoopV5SelectionPolicyHash, InventorySHA256: inventoryHash,
		CatalogSHA256: catalogHash, ModelRegistrySHA256: modelRegistryHash, EnvironmentPolicySHA256: environmentPolicyHash,
		OutcomeCountsBefore: closedLoopOutcomeCounts(baseline.Discovery.Cases),
		OutcomeCountsAfter:  closedLoopOutcomeCounts(discovery.Cases), CaseArtifacts: refs,
		Discovery: discovery, Comparison: comparison,
	}
	var err error
	report.Hash, err = hashClosedLoopV5PublicAdmissionReport(report)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func closedLoopV5PublicAdmissionPasses(comparison closedLoopV5PublicAdmissionComparison) bool {
	return comparison.DiscoveryPassBefore >= 0 && comparison.SelectedAffectedPassBefore >= 0 &&
		comparison.DiscoveryPassAfter > comparison.DiscoveryPassBefore &&
		comparison.SelectedAffectedPassAfter > comparison.SelectedAffectedPassBefore &&
		comparison.ExactCaseSet && comparison.NoBaselinePassRegression && comparison.NoBaselineUnsafeBecamePass &&
		comparison.NonselectedGapsPreserved && comparison.DeterministicReplayComplete &&
		comparison.PhysicalPromotionComplete && comparison.SynthesisEnvironmentPreserved
}

func TestClosedLoopV5PublicAdmissionRequiresEveryStrictGate(t *testing.T) {
	valid := closedLoopV5PublicAdmissionComparison{
		DiscoveryPassBefore: 0, DiscoveryPassAfter: 1,
		SelectedAffectedPassBefore: 0, SelectedAffectedPassAfter: 1,
		ExactCaseSet: true, NoBaselinePassRegression: true, NoBaselineUnsafeBecamePass: true,
		NonselectedGapsPreserved: true, DeterministicReplayComplete: true,
		PhysicalPromotionComplete: true, SynthesisEnvironmentPreserved: true,
	}
	if !closedLoopV5PublicAdmissionPasses(valid) {
		t.Fatal("complete V5 public admission evidence was rejected")
	}
	mutations := []func(*closedLoopV5PublicAdmissionComparison){
		func(value *closedLoopV5PublicAdmissionComparison) { value.SelectedAffectedPassBefore = -1 },
		func(value *closedLoopV5PublicAdmissionComparison) {
			value.DiscoveryPassAfter = value.DiscoveryPassBefore
		},
		func(value *closedLoopV5PublicAdmissionComparison) {
			value.SelectedAffectedPassAfter = value.SelectedAffectedPassBefore
		},
		func(value *closedLoopV5PublicAdmissionComparison) { value.ExactCaseSet = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.NoBaselinePassRegression = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.NoBaselineUnsafeBecamePass = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.NonselectedGapsPreserved = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.DeterministicReplayComplete = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.PhysicalPromotionComplete = false },
		func(value *closedLoopV5PublicAdmissionComparison) { value.SynthesisEnvironmentPreserved = false },
	}
	for index, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if closedLoopV5PublicAdmissionPasses(candidate) {
			t.Fatalf("V5 public admission accepted missing strict gate %d", index)
		}
	}
}

func TestClosedLoopV5PublicAdmissionPreservesExactGapIdentity(t *testing.T) {
	selection := closedLoopV5Selection{Selected: capabilitypackages.Candidate{Members: []capabilitypackages.Member{{
		Stage: "simulation", Scope: string(ScopeSimulation), Capability: "selected", Code: "SELECTED",
	}}}}
	selected := Gap{Stage: "simulation", Scope: ScopeSimulation, Capability: "selected", Code: "SELECTED"}
	remaining := Gap{
		Stage: "promotion", Scope: ScopePhysical, Capability: "physical_promotion", Code: "REMAINS",
		RequiredEvidence: []string{"drc", "connectivity"},
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
			if got := closedLoopV5RemainingGapsStable(test.before, test.after, selection); got != test.want {
				t.Fatalf("closedLoopV5RemainingGapsStable() = %t, want %t", got, test.want)
			}
		})
	}
	duplicate := append(evidence(remaining), evidence(remaining)...)
	if closedLoopV5RemainingGapsStable(duplicate, evidence(remaining), selection) ||
		closedLoopV5ExactCaseSet(evidence(remaining), nil) ||
		closedLoopV5NoPassRegression(duplicate, evidence(remaining)) ||
		closedLoopV5UnsafePreserved(evidence(remaining), nil) {
		t.Fatal("V5 preservation gates accepted duplicate or missing case sets")
	}
}

func TestClosedLoopV5SelectedPassCountFailsClosed(t *testing.T) {
	selection := closedLoopV5Selection{Selected: capabilitypackages.Candidate{Cases: []string{"affected"}}}
	if got := closedLoopV5SelectedPassCount(selection, []CaseEvidence{{Case: CaseMeta{ID: "affected"}, Outcome: OutcomePass}}); got != 1 {
		t.Fatalf("selected pass count = %d, want 1", got)
	}
	if got := closedLoopV5SelectedPassCount(selection, nil); got != -1 {
		t.Fatalf("missing selected case count = %d, want -1", got)
	}
	selection.Selected.Cases = []string{"affected", "affected"}
	if got := closedLoopV5SelectedPassCount(selection, []CaseEvidence{{Case: CaseMeta{ID: "affected"}}}); got != -1 {
		t.Fatalf("duplicate selected case count = %d, want -1", got)
	}
}

func closedLoopV5SelectedPassCount(selection closedLoopV5Selection, cases []CaseEvidence) int {
	byID, ok := closedLoopV5UniqueCases(cases)
	if !ok {
		return -1
	}
	seen := map[string]bool{}
	count := 0
	for _, id := range selection.Selected.Cases {
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

func closedLoopV5OutcomeCount(cases []CaseEvidence, outcome Outcome) int {
	count := 0
	for _, current := range cases {
		if current.Outcome == outcome {
			count++
		}
	}
	return count
}

func closedLoopV5ExactCaseSet(before, after []CaseEvidence) bool {
	_, _, ok := closedLoopV5PairedCases(before, after)
	return ok
}

func closedLoopV5NoPassRegression(before, after []CaseEvidence) bool {
	beforeByID, afterByID, ok := closedLoopV5PairedCases(before, after)
	if !ok {
		return false
	}
	for id, current := range beforeByID {
		if current.Outcome == OutcomePass && afterByID[id].Outcome != OutcomePass {
			return false
		}
	}
	return true
}

func closedLoopV5UnsafePreserved(before, after []CaseEvidence) bool {
	beforeByID, afterByID, ok := closedLoopV5PairedCases(before, after)
	if !ok {
		return false
	}
	for id, current := range beforeByID {
		if current.Outcome == OutcomeUnsafe && afterByID[id].Outcome == OutcomePass {
			return false
		}
	}
	return true
}

func closedLoopV5RemainingGapsStable(before, after []CaseEvidence, selection closedLoopV5Selection) bool {
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
			if closedLoopV5SelectedGap(gap, selection) {
				continue
			}
			if !finalGaps[closedLoopV5GapIdentity(gap)] {
				return false
			}
		}
	}
	return true
}

func closedLoopV5SelectedGap(gap Gap, selection closedLoopV5Selection) bool {
	for _, member := range selection.Selected.Members {
		if gap.Stage == member.Stage && string(gap.Scope) == member.Scope &&
			gap.Capability == member.Capability && gap.Code == member.Code {
			return true
		}
	}
	return false
}

func closedLoopV5UniqueCases(cases []CaseEvidence) (map[string]CaseEvidence, bool) {
	result := make(map[string]CaseEvidence, len(cases))
	for _, current := range cases {
		if current.Case.ID == "" {
			return nil, false
		}
		if _, exists := result[current.Case.ID]; exists {
			return nil, false
		}
		result[current.Case.ID] = current
	}
	return result, true
}

func closedLoopV5PairedCases(before, after []CaseEvidence) (map[string]CaseEvidence, map[string]CaseEvidence, bool) {
	beforeByID, beforeOK := closedLoopV5UniqueCases(before)
	afterByID, afterOK := closedLoopV5UniqueCases(after)
	if !beforeOK || !afterOK || len(beforeByID) != len(afterByID) {
		return nil, nil, false
	}
	for id := range beforeByID {
		if _, found := afterByID[id]; !found {
			return nil, nil, false
		}
	}
	return beforeByID, afterByID, true
}

func closedLoopV5GapIdentity(gap Gap) string {
	required := slices.Clone(gap.RequiredEvidence)
	slices.Sort(required)
	required = slices.Compact(required)
	values := append([]string{gap.Stage, string(gap.Scope), gap.Capability, gap.Code}, required...)
	var encoded strings.Builder
	for _, value := range values {
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	return encoded.String()
}

func closedLoopV5ReplayEvidenceComplete(artifacts []closedLoopV5CaseArtifact) bool {
	if len(artifacts) != closedLoopV5RoleSize {
		return false
	}
	for _, artifact := range artifacts {
		if len(artifact.NormalizedReplaySHA256) != 2 ||
			artifact.NormalizedReplaySHA256[0] != artifact.NormalizedReplaySHA256[1] ||
			!closedLoopV5ValidHash(artifact.NormalizedReplaySHA256[0]) ||
			!closedLoopV5ValidHash(artifact.SynthesisSHA256) ||
			artifact.Observation.SynthesisHash != artifact.SynthesisSHA256 {
			return false
		}
	}
	return true
}

func closedLoopV5PromotionEvidenceComplete(artifacts []closedLoopV5CaseArtifact) bool {
	if len(artifacts) != closedLoopV5RoleSize {
		return false
	}
	for _, artifact := range artifacts {
		if artifact.Observation.Outcome == OutcomePass {
			if artifact.Promotion == nil || artifact.Promotion.Status != ots.PhysicalPromotionPassed ||
				!artifact.Promotion.ReplayIdentical || artifact.Promotion.RunCount != 2 ||
				!closedLoopV5ValidHash(artifact.Promotion.PromotionHash) ||
				!closedLoopV5ValidHash(artifact.Promotion.ProjectHash) ||
				artifact.Promotion.PromotionHash != artifact.Observation.PromotionHash ||
				artifact.Promotion.ProjectHash != artifact.Observation.ProjectHash {
				return false
			}
		} else if artifact.Promotion != nil {
			return false
		}
	}
	return true
}

func loadClosedLoopV5PublicAdmissionCaseArtifacts(
	t *testing.T,
	manifest corpuspublication.Manifest,
	refs []closedLoopV5ArtifactRef,
) []closedLoopV5CaseArtifact {
	t.Helper()
	artifacts := make([]closedLoopV5CaseArtifact, 0, closedLoopV5RoleSize)
	refIndex := 0
	for _, entry := range manifest.Entries {
		if entry.Role != string(RoleDiscovery) {
			continue
		}
		if refIndex >= len(refs) {
			t.Fatal("V5 public admission artifact references are incomplete")
		}
		ref := refs[refIndex]
		expectedPath := filepath.ToSlash(filepath.Join("discovery", entry.ID+".json"))
		if ref.CaseID != entry.ID || ref.Path != expectedPath || !closedLoopV5ValidHash(ref.SHA256) {
			t.Fatalf("V5 public admission artifact reference %s is invalid", entry.ID)
		}
		data := mustCorpusRead(t, filepath.Join(closedLoopV5PublicAdmissionRoot, filepath.FromSlash(ref.Path)))
		if corpusHash(data) != ref.SHA256 {
			t.Fatalf("V5 public admission artifact %s checksum drifted", entry.ID)
		}
		var artifact closedLoopV5CaseArtifact
		decodeCorpusStrict(t, data, &artifact)
		want, err := hashClosedLoopV5CaseArtifact(artifact)
		if err != nil || artifact.Hash != want || artifact.Schema != closedLoopV5CaseArtifactSchema ||
			artifact.Version != closedLoopV5BaselineVersion || artifact.CaseID != entry.ID ||
			artifact.RequirementSHA256 != entry.RequirementSHA256 {
			t.Fatalf("V5 public admission artifact %s is invalid", entry.ID)
		}
		artifacts = append(artifacts, artifact)
		refIndex++
	}
	if refIndex != len(refs) || len(artifacts) != closedLoopV5RoleSize ||
		!closedLoopV5ReplayEvidenceComplete(artifacts) || !closedLoopV5PromotionEvidenceComplete(artifacts) {
		t.Fatal("V5 public admission artifact set is invalid")
	}
	return artifacts
}

func closedLoopV5PublicAdmissionAudit(report closedLoopV5PublicAdmissionReport) []byte {
	return []byte(fmt.Sprintf(
		"# V5 Public Discovery Admission Audit\n\n"+
			"The frozen reviewed implementation passed the public-only admission before any held-out final evidence was opened.\n\n"+
			"- report hash: `%s`\n"+
			"- implementation seal: `%s`\n"+
			"- discovery pass uplift: %d -> %d\n"+
			"- selected-package affected pass uplift: %d -> %d\n"+
			"- exact case set: yes\n"+
			"- baseline pass regressions: none\n"+
			"- unsafe-to-pass transitions: none\n"+
			"- nonselected baseline gaps: preserved for every still-nonpassing case\n"+
			"- deterministic replay and installed-KiCad promotion evidence: complete\n",
		report.Hash, report.ImplementationSHA256,
		report.Comparison.DiscoveryPassBefore, report.Comparison.DiscoveryPassAfter,
		report.Comparison.SelectedAffectedPassBefore, report.Comparison.SelectedAffectedPassAfter,
	))
}

func assertClosedLoopV5PublicAdmissionFileSet(t *testing.T, manifest corpuspublication.Manifest) {
	t.Helper()
	want := map[string]bool{
		"ADMISSION_AUDIT.md":           true,
		corpuspublication.ChecksumFile: true,
		"report.json":                  true,
	}
	for _, entry := range manifest.Entries {
		if entry.Role == string(RoleDiscovery) {
			want[filepath.ToSlash(filepath.Join("discovery", entry.ID+".json"))] = true
		}
	}
	seen := map[string]bool{}
	err := filepath.Walk(closedLoopV5PublicAdmissionRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular V5 public admission artifact %s", path)
		}
		relative, err := filepath.Rel(closedLoopV5PublicAdmissionRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !want[relative] {
			return fmt.Errorf("unexpected V5 public admission artifact %s", relative)
		}
		seen[relative] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(want) {
		t.Fatalf("V5 public admission file count = %d, want %d", len(seen), len(want))
	}
}

func hashClosedLoopV5PublicAdmissionReport(report closedLoopV5PublicAdmissionReport) (string, error) {
	report.Hash = ""
	return digest(report)
}
