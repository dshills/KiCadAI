package corpusfreeze

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

type validationFixture struct {
	assignments map[string][]byte
	bundles     map[string]Bundle
	binding     Binding
	policy      Policy
}

func TestValidateAcceptsDeterministicOutcomeBlindCorpus(t *testing.T) {
	fixture := validValidationFixture(t)
	first, err := Validate(fixture.assignments, fixture.bundles, fixture.binding, HistoricalCommitments{}, fixture.policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Validate(fixture.assignments, fixture.bundles, fixture.binding, HistoricalCommitments{}, fixture.policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("corpus validation report is nondeterministic")
	}
	if len(first.Entries) != 2 || first.Entries[0].ID != "case_001" || first.Entries[1].ID != "case_002" {
		t.Fatalf("unexpected validated entries: %#v", first.Entries)
	}
	if first.Schema != "kicadai.behavior-corpus-validation-report.v1" || first.Version != 1 || !validSHA256(first.PolicySHA256) ||
		first.ContractBindingSHA256 != fixture.binding.ContractBindingSHA256 || !validSHA256(first.AssignmentSHA256["author_1"]) ||
		!validSHA256(first.AuthorshipSHA256["author_1"]) || first.AuthorPacketSHA256["author_1"] != fixture.binding.AuthorPacketSHA256["author_1"] {
		t.Fatalf("validation report commitments are incomplete: %#v", first)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"discovery behavior", "held out behavior", "signal_input", "signal_output"} {
		if bytes.Contains(encoded, []byte(prohibited)) {
			t.Fatalf("validation report leaks requirement content %q", prohibited)
		}
	}
}

func TestValidateFailsClosedOnInvalidQuarantineEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *validationFixture)
		want   string
	}{
		{
			name: "unknown assignment field",
			mutate: func(t *testing.T, fixture *validationFixture) {
				fixture.assignments["author_1"] = addUnknownJSONField(t, fixture.assignments["author_1"])
			},
			want: "decode assignment",
		},
		{
			name: "unknown authorship field",
			mutate: func(t *testing.T, fixture *validationFixture) {
				bundle := fixture.bundles["author_1"]
				bundle.AuthorshipJSON = addUnknownJSONField(t, bundle.AuthorshipJSON)
				fixture.bundles["author_1"] = bundle
			},
			want: "decode authorship",
		},
		{
			name: "false isolation attestation",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.Attestations.PacketOnlyInput = false
				})
			},
			want: "attestations are incomplete",
		},
		{
			name: "unresolved provenance placeholder",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.AuthorContextIdentity = "[unknown]"
				})
			},
			want: "is unresolved",
		},
		{
			name: "invalid authoring interval",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.AuthoringEndedUTC = "2026-08-10T09:59:59Z"
				})
			},
			want: "authorship end is invalid",
		},
		{
			name: "non UTC authoring timestamp",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.AuthoringStartedUTC = "2026-08-10T05:00:00-05:00"
				})
			},
			want: "authorship start is not RFC3339",
		},
		{
			name: "packet binding mismatch",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.PerAuthorPacketSHA256 = strings.Repeat("c", 64)
				})
			},
			want: "authorship binding is invalid",
		},
		{
			name: "source hash mismatch",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.RequirementSourceSHA256[0].SHA256 = strings.Repeat("d", 64)
				})
			},
			want: "source hash mismatch",
		},
		{
			name: "unexpected bundle file",
			mutate: func(t *testing.T, fixture *validationFixture) {
				fixture.bundles["author_1"].Requirements["discovery/extra.json"] = []byte("{}")
			},
			want: "bundle requirement count",
		},
		{
			name: "assignment substitution",
			mutate: func(t *testing.T, fixture *validationFixture) {
				assignment := decodeTestAssignment(t, fixture.assignments["author_1"])
				assignment.Entries[0].RequirementFile = "../request.json"
				fixture.assignments["author_1"] = marshalTestJSON(t, assignment)
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.AssignmentSHA256 = hashBytes(fixture.assignments["author_1"])
				})
			},
			want: "authorship binding is invalid",
		},
		{
			name: "unsafe frozen assignment path",
			mutate: func(t *testing.T, fixture *validationFixture) {
				assignment := decodeTestAssignment(t, fixture.assignments["author_1"])
				assignment.Entries[0].RequirementFile = "../request.json"
				fixture.assignments["author_1"] = marshalTestJSON(t, assignment)
				fixture.binding.AssignmentSHA256["author_1"] = hashBytes(fixture.assignments["author_1"])
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.AssignmentSHA256 = fixture.binding.AssignmentSHA256["author_1"]
				})
			},
			want: "invalid entry metadata",
		},
		{
			name: "source hash rows out of assignment order",
			mutate: func(t *testing.T, fixture *validationFixture) {
				mutateAuthorship(t, fixture, func(authorship *Authorship) {
					authorship.RequirementSourceSHA256[0], authorship.RequirementSourceSHA256[1] = authorship.RequirementSourceSHA256[1], authorship.RequirementSourceSHA256[0]
				})
			},
			want: "not in assignment order",
		},
		{
			name: "manifest identity leak",
			mutate: func(t *testing.T, fixture *validationFixture) {
				replaceFixtureRequirement(t, fixture, "discovery/request_001.json", func(requirement *ots.Requirement) {
					requirement.Project.Description = "case_001 behavior"
				})
			},
			want: "prohibited manifest identity prefix",
		},
		{
			name: "implementation language leak",
			mutate: func(t *testing.T, fixture *validationFixture) {
				replaceFixtureRequirement(t, fixture, "discovery/request_001.json", func(requirement *ots.Requirement) {
					requirement.Project.Description = "choose a topology"
				})
			},
			want: "prohibited implementation language",
		},
		{
			name: "missing acceptance gate",
			mutate: func(t *testing.T, fixture *validationFixture) {
				replaceFixtureRequirement(t, fixture, "discovery/request_001.json", func(requirement *ots.Requirement) {
					requirement.Acceptance.RequireStrictDRC = false
				})
			},
			want: "violates the public requirement contract",
		},
		{
			name: "raw duplicate",
			mutate: func(t *testing.T, fixture *validationFixture) {
				first := fixture.bundles["author_1"].Requirements["discovery/request_001.json"]
				fixture.bundles["author_1"].Requirements["held_out/request_002.json"] = append([]byte(nil), first...)
				refreshFixtureSourceHashes(t, fixture)
			},
			want: "duplicates raw requirement",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validValidationFixture(t)
			test.mutate(t, &fixture)
			_, err := Validate(fixture.assignments, fixture.bundles, fixture.binding, HistoricalCommitments{}, fixture.policy)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsHistoricalRawAndSemanticReuse(t *testing.T) {
	fixture := validValidationFixture(t)
	report, err := Validate(fixture.assignments, fixture.bundles, fixture.binding, HistoricalCommitments{}, fixture.policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, historical := range []HistoricalCommitments{
		{RawSHA256: map[string]string{report.Entries[0].RequirementSHA256: "retired_raw"}},
		{NeutralSemanticSHA256: map[string]string{report.Entries[0].NeutralSemanticSHA256: "retired_semantic"}},
	} {
		if _, err := Validate(fixture.assignments, fixture.bundles, fixture.binding, historical, fixture.policy); err == nil || !strings.Contains(err.Error(), "historical") {
			t.Fatalf("historical reuse error = %v", err)
		}
	}
}

func TestNormalizedSemanticHashIgnoresProjectTextAndSemanticIDSpelling(t *testing.T) {
	first := testRequirement(t, false)
	second := testRequirement(t, false)
	second.Project.Name, second.Project.Title, second.Project.Description = "renamed", "Different title", "Different behavior description"
	renameRequirementIDs(&second)
	firstNeutral, firstHash, err := semanticHashes(first)
	if err != nil {
		t.Fatal(err)
	}
	secondNeutral, secondHash, err := semanticHashes(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("normalized hashes differ after text/ID renaming: %s != %s", firstHash, secondHash)
	}
	if firstNeutral == secondNeutral {
		t.Fatal("neutral semantic hash unexpectedly ignores semantic ID spelling")
	}
	permuted := cloneRequirement(first)
	reverseSlice(permuted.Requirements.Domains)
	reverseSlice(permuted.Requirements.Ports)
	reverseSlice(permuted.Requirements.OperatingCases)
	for index := range permuted.Requirements.OperatingCases {
		reverseSlice(permuted.Requirements.OperatingCases[index].Conditions)
		reverseSlice(permuted.Requirements.OperatingCases[index].Events)
	}
	reverseSlice(permuted.Requirements.BehavioralRequirements)
	for index := range permuted.Requirements.BehavioralRequirements {
		reverseSlice(permuted.Requirements.BehavioralRequirements[index].OperatingCases)
	}
	_, permutedHash, err := semanticHashes(permuted)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != permutedHash {
		t.Fatalf("normalized hashes differ after slice permutation: %s != %s", firstHash, permutedHash)
	}
}

func reverseSlice[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func TestV5PolicyFreezesThreeAuthorDiversityContract(t *testing.T) {
	policy := V5Policy()
	if policy.Version != 5 || len(policy.AuthorSlots) != 3 || len(policy.Roles) != 2 || len(policy.Domains) != 6 || policy.CasesPerAuthorRoleDomain != 1 {
		t.Fatalf("V5 policy cardinality is invalid: %#v", policy)
	}
	if policy.MinimumOperatingCases != 2 || policy.MinimumAssertions != 4 || policy.MinimumAnalysesPerRequirement != 2 ||
		policy.MinimumAnalysisKindsPerAuthor != 4 || policy.MinimumEventKindsPerAuthor != 3 ||
		policy.MinimumMultiOutputPerRole != 5 || policy.MinimumConvergingInputsPerRole != 5 || policy.MinimumCriticalDomainsPerRole != 4 {
		t.Fatalf("V5 policy minima are invalid: %#v", policy)
	}
	if !reflect.DeepEqual(policy.RequiredEventKinds, []string{"input_step", "load_step", "power_step", "startup", "rail_loss", "short_circuit"}) {
		t.Fatalf("V5 policy event contract = %v", policy.RequiredEventKinds)
	}
}

func TestStructuralSignatureHistoryRejectsNonAdjacentDuplicate(t *testing.T) {
	state := validationState{domainSignatures: map[string]map[string]map[[3]string]bool{
		"author_1": {},
	}}
	first := [3]string{"ports-a", "assertions-a", "analyses-a"}
	second := [3]string{"ports-b", "assertions-b", "analyses-b"}
	if state.recordStructuralSignature("author_1", "analog", first) || state.recordStructuralSignature("author_1", "analog", second) {
		t.Fatal("new structural signatures were reported as duplicates")
	}
	if !state.recordStructuralSignature("author_1", "analog", first) {
		t.Fatal("nonadjacent structural signature duplicate was not rejected")
	}
}

func TestDiversityDoesNotTreatNonPortObservationAsCollidingPort(t *testing.T) {
	requirement := testRequirement(t, false)
	for index := range requirement.Requirements.BehavioralRequirements {
		requirement.Requirements.BehavioralRequirements[index].Observation.Kind = "circuit"
	}
	evidence := newDiversityEvidence()
	evidence.observe("analog", requirement)
	if evidence.multiOutput != 0 || evidence.convergingExcitations != 0 {
		t.Fatalf("non-port observations counted as ports: multi-output=%d converging=%d", evidence.multiOutput, evidence.convergingExcitations)
	}
}

func TestAllTrueRejectsEveryIndividualAttestationFalse(t *testing.T) {
	attestations := allTestAttestations()
	value := reflect.ValueOf(&attestations).Elem()
	for index := 0; index < value.NumField(); index++ {
		value.Field(index).SetBool(false)
		if attestations.AllTrue() {
			t.Fatalf("AllTrue ignored false attestation field %s", value.Type().Field(index).Name)
		}
		value.Field(index).SetBool(true)
	}
	if !attestations.AllTrue() {
		t.Fatal("AllTrue rejected complete attestations")
	}
}

func TestRequirementCloneDoesNotShareMutableState(t *testing.T) {
	original := testRequirement(t, false)
	clone := cloneRequirement(original)
	*clone.Requirements.Domains[1].MinVoltageV = 99
	*clone.Requirements.Ports[0].Electrical.MinVoltageV = 98
	*clone.Requirements.BehavioralRequirements[0].Min = 97
	clone.Requirements.OperatingCases[0].Conditions[0].Min = 96
	clone.Requirements.BehavioralRequirements[0].OperatingCases[0] = "changed"
	clone.Requirements.BehavioralRequirements[0].Excitation.ID = "changed"
	if *original.Requirements.Domains[1].MinVoltageV == 99 || *original.Requirements.Ports[0].Electrical.MinVoltageV == 98 ||
		*original.Requirements.BehavioralRequirements[0].Min == 97 || original.Requirements.OperatingCases[0].Conditions[0].Min == 96 ||
		original.Requirements.BehavioralRequirements[0].OperatingCases[0] == "changed" || original.Requirements.BehavioralRequirements[0].Excitation.ID == "changed" {
		t.Fatal("requirement clone shares mutable state with its source")
	}
}

func validValidationFixture(t *testing.T) validationFixture {
	t.Helper()
	policy := Policy{
		AssignmentSchema: "assignment.test", AuthorshipSchema: "authorship.test", Version: 1,
		AuthorSlots: []string{"author_1"}, Roles: []string{RoleDiscovery, RoleHeldOut}, Domains: []string{"analog"},
		SafetyImpacts: []string{"non_safety", "safety_critical"}, CasesPerAuthorRoleDomain: 1,
		MinimumOperatingCases: 2, MinimumAssertions: 4, MinimumAnalysesPerRequirement: 2,
		MinimumAnalysisKindsPerAuthor: 2, MinimumEventKindsPerAuthor: 1,
		ProhibitedIdentityPrefixes: []string{"case_", "source_"}, ProhibitedTerms: []string{"component", "topology"},
	}
	assignment := Assignment{Schema: policy.AssignmentSchema, Version: policy.Version, AuthorSlot: "author_1", Entries: []AssignmentEntry{
		{ID: "case_001", Role: RoleDiscovery, Domain: "analog", SafetyImpact: "non_safety", SourceID: "source_001", RequirementFile: "discovery/request_001.json"},
		{ID: "case_002", Role: RoleHeldOut, Domain: "analog", SafetyImpact: "safety_critical", SourceID: "source_002", RequirementFile: "held_out/request_002.json"},
	}}
	assignmentData := marshalTestJSON(t, assignment)
	requirements := map[string][]byte{
		"discovery/request_001.json": marshalTestJSON(t, testRequirement(t, false)),
		"held_out/request_002.json":  marshalTestJSON(t, testRequirement(t, true)),
	}
	binding := Binding{
		ContractBindingSHA256: strings.Repeat("a", 64),
		AuthorPacketSHA256:    map[string]string{"author_1": strings.Repeat("b", 64)},
		AssignmentSHA256:      map[string]string{"author_1": hashBytes(assignmentData)},
	}
	authorship := Authorship{
		Schema: policy.AuthorshipSchema, Version: policy.Version, AuthorContextIdentity: "isolated-context-1", AuthorSlot: "author_1",
		AuthoringToolModelVersion: "test-author 1.0", AuthoringStartedUTC: "2026-08-10T10:00:00Z", AuthoringEndedUTC: "2026-08-10T10:30:00Z",
		PerAuthorPacketManifest: "AUTHOR_1_PACKET.sha256", PerAuthorPacketSHA256: binding.AuthorPacketSHA256["author_1"],
		ContractBindingSHA256: binding.ContractBindingSHA256, AssignmentSHA256: hashBytes(assignmentData), ReturnedBundleRoot: "quarantine_author_1",
		RequirementSourceSHA256: []SourceHash{
			{Path: "discovery/request_001.json", SHA256: hashBytes(requirements["discovery/request_001.json"])},
			{Path: "held_out/request_002.json", SHA256: hashBytes(requirements["held_out/request_002.json"])},
		},
		Uncertainties: []string{}, Attestations: allTestAttestations(),
	}
	return validationFixture{
		assignments: map[string][]byte{"author_1": assignmentData},
		bundles:     map[string]Bundle{"author_1": {AuthorshipJSON: marshalTestJSON(t, authorship), Requirements: requirements}},
		binding:     binding, policy: policy,
	}
}

func testRequirement(t *testing.T, variant bool) ots.Requirement {
	t.Helper()
	minimumSupply, nominalSupply, maximumSupply, maximumCurrent := 4.5, 5.0, 5.5, 0.1
	minimumInput, nominalInput, maximumInput, inputImpedance := 0.0, 0.5, 1.0, 10_000.0
	minimumOutput, nominalOutput, maximumOutput, outputCurrent := 0.0, 2.5, 5.0, 0.02
	minimumGain, maximumGain, maximumSettling, maximumNoise, frequency := 2.0, 4.0, 0.001, 0.02, 1_000.0
	lastMetric, lastAnalysis, lastUnit := "output_noise_rms", "noise", "V_rms"
	if variant {
		lastMetric, lastAnalysis, lastUnit = "peak_voltage", "transient", "V"
		maximumNoise = 5.1
	}
	return ots.Requirement{
		Schema: ots.RequirementSchema, Version: ots.RequirementVersion,
		Project: ots.Project{Name: "behavior_study", Title: "Behavior study", Description: map[bool]string{false: "discovery behavior", true: "held out behavior"}[variant]},
		Requirements: ots.Requirements{
			Domains: []ots.Domain{
				{ID: "ground", Kind: "reference", Source: "external"},
				{ID: "supply", Kind: "supply", MinVoltageV: &minimumSupply, NominalVoltageV: &nominalSupply, MaxVoltageV: &maximumSupply, MaxCurrentA: &maximumCurrent, Source: "external"},
			},
			Ports: []ots.Port{
				{ID: "signal_input", Kind: "analog_voltage", Direction: "sink", Domain: "ground", Electrical: ots.Electrical{MinVoltageV: &minimumInput, NominalVoltageV: &nominalInput, MaxVoltageV: &maximumInput, InputImpedanceMinOhm: &inputImpedance}},
				{ID: "signal_output", Kind: "analog_voltage", Direction: "source", Domain: "ground", Electrical: ots.Electrical{MinVoltageV: &minimumOutput, NominalVoltageV: &nominalOutput, MaxVoltageV: &maximumOutput, MaxCurrentA: &outputCurrent}},
				{ID: "monitor_output", Kind: "analog_voltage", Direction: "source", Domain: "ground", Electrical: ots.Electrical{MinVoltageV: &minimumOutput, NominalVoltageV: &nominalOutput, MaxVoltageV: &maximumOutput, MaxCurrentA: &outputCurrent}},
				{ID: "power_input", Kind: "power", Direction: "sink", Domain: "supply", Electrical: ots.Electrical{MinVoltageV: &minimumSupply, NominalVoltageV: &nominalSupply, MaxVoltageV: &maximumSupply, MaxCurrentA: &maximumCurrent}},
			},
			OperatingCases: []ots.OperatingCase{
				{ID: "nominal", Conditions: []ots.OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: 4.5, Max: 5.5, Unit: "V"}, {Axis: "load_resistance", Target: "signal_output", Min: 1_000, Max: 10_000, Unit: "ohm"}}},
				{ID: "dynamic", Conditions: []ots.OperatingCondition{{Axis: "ambient_temperature", Target: "ground", Min: -20, Max: 70, Unit: "degC"}}, Events: []ots.OperatingEvent{{ID: "input_change", Kind: "input_step", Target: "signal_input", TriggerTimeS: 0.001, Initial: 0, Applied: 1, Unit: "V"}}},
			},
			BehavioralRequirements: []ots.BehavioralAssertion{
				{ID: "steady", Metric: "output_voltage", Analysis: "dc_operating_point", Excitation: &ots.Observation{Kind: "port", ID: "signal_input"}, Observation: ots.Observation{Kind: "port", ID: "signal_output"}, Min: &minimumOutput, Max: &maximumOutput, Unit: "V", OperatingCases: []string{"nominal"}},
				{ID: "gain", Metric: "voltage_gain", Analysis: "ac_sweep", Excitation: &ots.Observation{Kind: "port", ID: "signal_input"}, Observation: ots.Observation{Kind: "port", ID: "signal_output"}, Min: &minimumGain, Max: &maximumGain, Unit: "ratio", FrequencyHz: &frequency, OperatingCases: []string{"nominal"}},
				{ID: "settling", Metric: "settling_time", Analysis: "transient", Excitation: &ots.Observation{Kind: "port", ID: "signal_input"}, Observation: ots.Observation{Kind: "port", ID: "monitor_output"}, Max: &maximumSettling, Unit: "s", OperatingCases: []string{"dynamic"}},
				{ID: "boundary", Metric: lastMetric, Analysis: lastAnalysis, Observation: ots.Observation{Kind: "port", ID: "monitor_output"}, Max: &maximumNoise, Unit: lastUnit, FrequencyHz: &frequency, OperatingCases: []string{"nominal", "dynamic"}, Critical: true},
			},
			Constraints: ots.BoardLimits{MaxComponents: 16, MaxWidthMM: 50, MaxHeightMM: 40},
		},
		Acceptance: allTestAcceptance(),
	}
}

func allTestAcceptance() ots.Acceptance {
	return ots.Acceptance{RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true, RequireAllCorners: true,
		RequireModelProvenance: true, RequireClosedLoopEvidence: true, RequireCompleteRouting: true, RequireConnectivity: true,
		RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
		RequireDeterministicReplay: true, RequireFailClosed: true}
}

func allTestAttestations() AuthorshipAttestations {
	return AuthorshipAttestations{PacketOnlyInput: true, ContractBoundBeforeAuthoring: true, NoRepositoryOrPriorCorpusAccess: true,
		NoCrossAuthorAssignmentOrContentAccess: true, IndependentlyConceivedBehaviorOnlyRequirements: true,
		NoSynthesisSimulationClassificationOrFeasibility: true, FixedDiscoveryHeldOutMembership: true,
		NoImplementationOrExpectedOutcomePrescription: true, NoPostEvaluationInspectionOrModification: true, AllUncertaintiesDisclosed: true}
}

func mutateAuthorship(t *testing.T, fixture *validationFixture, mutate func(*Authorship)) {
	t.Helper()
	bundle := fixture.bundles["author_1"]
	authorship, err := DecodeAuthorshipStrict(bundle.AuthorshipJSON)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&authorship)
	bundle.AuthorshipJSON = marshalTestJSON(t, authorship)
	fixture.bundles["author_1"] = bundle
}

func replaceFixtureRequirement(t *testing.T, fixture *validationFixture, path string, mutate func(*ots.Requirement)) {
	t.Helper()
	bundle := fixture.bundles["author_1"]
	requirement, issues := ots.DecodeStrict(bytes.NewReader(bundle.Requirements[path]))
	if len(issues) != 0 {
		t.Fatalf("decode fixture requirement: %#v", issues)
	}
	mutate(&requirement)
	bundle.Requirements[path] = marshalTestJSON(t, requirement)
	fixture.bundles["author_1"] = bundle
	refreshFixtureSourceHashes(t, fixture)
}

func refreshFixtureSourceHashes(t *testing.T, fixture *validationFixture) {
	t.Helper()
	mutateAuthorship(t, fixture, func(authorship *Authorship) {
		for index := range authorship.RequirementSourceSHA256 {
			path := authorship.RequirementSourceSHA256[index].Path
			authorship.RequirementSourceSHA256[index].SHA256 = hashBytes(fixture.bundles["author_1"].Requirements[path])
		}
	})
}

func decodeTestAssignment(t *testing.T, data []byte) Assignment {
	t.Helper()
	assignment, err := DecodeAssignmentStrict(data)
	if err != nil {
		t.Fatal(err)
	}
	return assignment
}

func addUnknownJSONField(t *testing.T, data []byte) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["unexpected"] = true
	return marshalTestJSON(t, value)
}

func marshalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func renameRequirementIDs(requirement *ots.Requirement) {
	domainMap := map[string]string{"ground": "return", "supply": "energy"}
	portMap := map[string]string{"signal_input": "stimulus", "signal_output": "observed", "monitor_output": "monitor", "power_input": "energy_input"}
	caseMap := map[string]string{"nominal": "steady_case", "dynamic": "event_case"}
	for index := range requirement.Requirements.Domains {
		requirement.Requirements.Domains[index].ID = domainMap[requirement.Requirements.Domains[index].ID]
	}
	for index := range requirement.Requirements.Ports {
		port := &requirement.Requirements.Ports[index]
		port.ID, port.Domain = portMap[port.ID], domainMap[port.Domain]
	}
	for index := range requirement.Requirements.OperatingCases {
		operatingCase := &requirement.Requirements.OperatingCases[index]
		operatingCase.ID = caseMap[operatingCase.ID]
		for conditionIndex := range operatingCase.Conditions {
			condition := &operatingCase.Conditions[conditionIndex]
			if renamed := domainMap[condition.Target]; renamed != "" {
				condition.Target = renamed
			} else {
				condition.Target = portMap[condition.Target]
			}
		}
		for eventIndex := range operatingCase.Events {
			operatingCase.Events[eventIndex].ID = "renamed_event"
			operatingCase.Events[eventIndex].Target = portMap[operatingCase.Events[eventIndex].Target]
		}
	}
	for index := range requirement.Requirements.BehavioralRequirements {
		assertion := &requirement.Requirements.BehavioralRequirements[index]
		assertion.ID = "renamed_assertion_" + string(rune('a'+index))
		if assertion.Excitation != nil {
			assertion.Excitation.ID = portMap[assertion.Excitation.ID]
		}
		assertion.Observation.ID = portMap[assertion.Observation.ID]
		for caseIndex := range assertion.OperatingCases {
			assertion.OperatingCases[caseIndex] = caseMap[assertion.OperatingCases[caseIndex]]
		}
	}
}
