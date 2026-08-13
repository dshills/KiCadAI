package capabilityfeedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityroundsv8"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/obligationanchor"
)

func TestBuildV8DiscoveryRoundCasesBindsAndAnchorsCompleteFrontier(t *testing.T) {
	manifestSource, obligationSource, report, registry := v8FrontierFixture(t)
	cases, err := BuildV8DiscoveryRoundCases(manifestSource, obligationSource, report, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != capabilityroundsv8.FrozenPolicy().ExpectedDiscoveryCases {
		t.Fatalf("case count = %d", len(cases))
	}
	first := cases[0]
	if len(first.Frontier) != 1 || len(first.SatisfiedObligations) != 1 {
		t.Fatalf("first frontier/satisfied = %d/%d", len(first.Frontier), len(first.SatisfiedObligations))
	}
	leaf := first.Frontier[0].Path[0]
	if leaf.Stage != "model" || leaf.Category != "model" || leaf.Scope != "model" ||
		leaf.Capability != "fixture_model" || leaf.Code != "MODEL_UNAVAILABLE" ||
		!reflect.DeepEqual(first.Frontier[0].Diagnostics, []string{"downstream:SEARCH_EXHAUSTED", "evidence_sha256:" + feedbackHash("frontier")}) {
		t.Fatalf("first root gap = %#v", first.Frontier[0])
	}
	if _, err := capabilityroundsv8.PathHash(first.Frontier[0]); err != nil {
		t.Fatalf("root path is invalid: %v", err)
	}
	if first.Frontier[0].ObligationAnchor == first.SatisfiedObligations[0] {
		t.Fatal("failed obligation was also marked satisfied")
	}
	if _, err := capabilityroundsv8.Select(cases, nil, capabilityroundsv8.RoundState{}, capabilityroundsv8.FrozenPolicy()); err == nil {
		t.Fatal("unplanned fixture frontier unexpectedly produced an eligible selection")
	}
}

func TestBuildV8DiscoveryRoundCasesRejectsManifestDriftAndAnchorTampering(t *testing.T) {
	manifestSource, obligationSource, report, registry := v8FrontierFixture(t)
	driftedManifest := append(append([]byte(nil), manifestSource...), '\n')
	if _, err := BuildV8DiscoveryRoundCases(driftedManifest, obligationSource, report, registry); err == nil {
		t.Fatal("byte-drifted manifest retained its obligation binding")
	}

	var obligations corpuspublication.DiscoveryObligationsV8
	if err := json.Unmarshal(obligationSource, &obligations); err != nil {
		t.Fatal(err)
	}
	obligations.Obligations[0].Anchor = strings.Repeat("0", 64)
	tampered, err := json.Marshal(obligations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildV8DiscoveryRoundCases(manifestSource, tampered, report, registry); err == nil {
		t.Fatal("tampered obligation anchor was accepted")
	}
}

func TestV8RootFrontierFailsClosedOnSelectorsAndDuplicatePaths(t *testing.T) {
	obligation := corpuspublication.ObligationV8{
		Anchor: strings.Repeat("1", 64), Role: "discovery", CaseID: "v8_case_001",
		OperatingCaseID: "nominal", AssertionID: "gain", ObservationKind: "port",
		ObservationID: "output", OutputID: "output",
	}
	gap := Gap{
		Stage: "simulation", Scope: ScopeSimulation, Capability: "gain_evidence", Code: "ASSERTION_BELOW_MINIMUM",
		RequirementIDs: []string{"unknown"}, OperatingCases: []string{"nominal"},
		RequiredEvidence: []string{"trusted simulation"}, EvidenceHashes: []string{feedbackHash("selector")},
	}
	if _, _, err := v8RootFrontier([]corpuspublication.ObligationV8{obligation}, []Gap{gap}); err == nil {
		t.Fatal("unknown assertion selector was accepted")
	}
	gap.RequirementIDs = []string{"gain"}
	if _, _, err := v8RootFrontier([]corpuspublication.ObligationV8{obligation}, []Gap{gap, gap}); err == nil {
		t.Fatal("duplicate obligation path was accepted")
	}
}

func TestV8RootFrontierMapsOneCausalGapToMultipleObligationAnchors(t *testing.T) {
	obligations := []corpuspublication.ObligationV8{
		{Anchor: strings.Repeat("1", 64), Role: "discovery", CaseID: "v8_case_001", OperatingCaseID: "nominal", AssertionID: "gain", ObservationKind: "port", ObservationID: "output", OutputID: "output"},
		{Anchor: strings.Repeat("2", 64), Role: "discovery", CaseID: "v8_case_001", OperatingCaseID: "overdrive", AssertionID: "recovery", ObservationKind: "port", ObservationID: "output", OutputID: "output"},
	}
	gap := Gap{
		Stage: "simulation", Scope: ScopeModel, Capability: "shared_model", Code: "MODEL_UNAVAILABLE",
		RequiredEvidence: []string{"reviewed model"}, EvidenceHashes: []string{feedbackHash("shared-model")},
	}
	frontier, satisfied, err := v8RootFrontier(obligations, []Gap{gap})
	if err != nil {
		t.Fatal(err)
	}
	if len(frontier) != 2 || len(satisfied) != 0 {
		t.Fatalf("frontier/satisfied = %d/%d", len(frontier), len(satisfied))
	}
	first, firstErr := capabilityroundsv8.PathHash(frontier[0])
	second, secondErr := capabilityroundsv8.PathHash(frontier[1])
	if firstErr != nil || secondErr != nil || first == second {
		t.Fatalf("obligation-bound path hashes = %q/%q, errors = %v/%v", first, second, firstErr, secondErr)
	}
}

func TestV8GapCategoryUsesFrozenCausalCategories(t *testing.T) {
	tests := map[GapScope]string{
		ScopeTopology: "topology", ScopeComponent: "component", ScopeModel: "model",
		ScopeSimulation: "simulation", ScopePhysical: "physical_design",
		ScopeRouting: "physical_design", ScopeVerification: "verification",
	}
	for scope, want := range tests {
		got, err := v8GapCategory(scope)
		if err != nil || got != want {
			t.Fatalf("category(%q) = %q, %v", scope, got, err)
		}
	}
}

func v8FrontierFixture(t *testing.T) ([]byte, []byte, AggregateReport, capabilityevaluation.ImpactRegistry) {
	t.Helper()
	policy := capabilityroundsv8.FrozenPolicy()
	domains := []string{
		"analog_signal_path", "power_energy_conversion", "digital_control",
		"mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity",
	}
	roles := []string{
		"source_bias", "amplification_conditioning", "conversion_regulation",
		"sensing_measurement", "interface_control", "protection_supervision",
	}
	safety := []string{"non_safety", "review_required", "safety_relevant", "safety_critical"}
	manifest := corpuspublication.ManifestV8{
		Schema: corpuspublication.ManifestSchemaV8, Version: corpuspublication.ManifestVersionV8,
		DiscoveryCaseCount: policy.ExpectedDiscoveryCases, HeldOutCaseCount: policy.ExpectedDiscoveryCases,
	}
	evidence := make([]CaseEvidence, 0, policy.ExpectedDiscoveryCases)
	for index := 1; index <= policy.ExpectedDiscoveryCases; index++ {
		id := fmt.Sprintf("v8_case_%03d", index)
		feedbackDomain, err := v8FeedbackDomain(domains[(index-1)%len(domains)])
		if err != nil {
			t.Fatal(err)
		}
		gap := Gap{
			Stage: "simulation", Scope: ScopeModel, Capability: "fixture_model", Code: "MODEL_UNAVAILABLE",
			RequirementIDs: []string{"assertion_primary"}, OperatingCases: []string{"nominal"},
			RequiredEvidence: []string{"reviewed model"}, EvidenceHashes: []string{feedbackHash("frontier")},
			DownstreamSymptoms: []string{"SEARCH_EXHAUSTED"},
		}
		current := feedbackSealedCaseForPolicy(
			t, RealizabilityPolicyVersion, id, RoleDiscovery,
			feedbackDomain,
			capabilityevaluation.SafetyImpact(safety[(index-1)%len(safety)]),
			[]string{"dc_operating_point"}, gap,
		)
		evidence = append(evidence, current)
		manifest.Entries = append(manifest.Entries, corpuspublication.EntryV8{
			ID: id, Role: "discovery", Domain: domains[(index-1)%len(domains)],
			CircuitRole: roles[(index-1)%len(roles)], SafetyImpact: safety[(index-1)%len(safety)],
			RequirementSHA256: current.RequirementHash,
		})
	}
	for index := 1; index <= policy.ExpectedDiscoveryCases; index++ {
		manifest.Entries = append(manifest.Entries, corpuspublication.EntryV8{
			ID:   fmt.Sprintf("v8_case_%03d", policy.ExpectedDiscoveryCases+index),
			Role: "held_out", Sealed: true,
		})
	}
	manifestSource, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifestSource)
	manifestHash := hex.EncodeToString(manifestDigest[:])
	obligations := corpuspublication.DiscoveryObligationsV8{
		Schema: discoveryObligationsSchemaV8, Version: corpuspublication.ManifestVersionV8,
		CorpusManifestSHA256: manifestHash,
	}
	for index := 1; index <= policy.ExpectedDiscoveryCases; index++ {
		id := fmt.Sprintf("v8_case_%03d", index)
		obligations.Obligations = append(obligations.Obligations, v8FixtureObligation(t, manifestHash, id, "assertion_primary"))
		if index == 1 {
			obligations.Obligations = append(obligations.Obligations, v8FixtureObligation(t, manifestHash, id, "assertion_secondary"))
		}
	}
	slicesSortObligations(obligations.Obligations)
	obligationSource, err := json.Marshal(obligations)
	if err != nil {
		t.Fatal(err)
	}
	registry := capabilityevaluation.ImpactRegistry{Version: "v8-frontier-test"}
	report, err := EvaluateRealizabilityAware(RoleDiscovery, evidence, registry)
	if err != nil {
		t.Fatal(err)
	}
	return manifestSource, obligationSource, report, registry
}

func v8FixtureObligation(t *testing.T, manifestHash, caseID, assertionID string) corpuspublication.ObligationV8 {
	t.Helper()
	current := corpuspublication.ObligationV8{
		Role: "discovery", CaseID: caseID, OperatingCaseID: "nominal", AssertionID: assertionID,
		ObservationKind: "port", ObservationID: "output", OutputID: "output",
	}
	anchor, err := obligationanchor.Derive(obligationanchor.Input{
		CorpusManifestSHA256: manifestHash, Role: current.Role, CaseID: current.CaseID,
		OperatingCaseID: current.OperatingCaseID, AssertionID: current.AssertionID,
		ObservationKind: current.ObservationKind, ObservationID: current.ObservationID, OutputID: current.OutputID,
	})
	if err != nil {
		t.Fatal(err)
	}
	current.Anchor = anchor
	return current
}

func slicesSortObligations(values []corpuspublication.ObligationV8) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor].Anchor < values[cursor-1].Anchor; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}
