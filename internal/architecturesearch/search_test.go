package architecturesearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

type staticTestProvider struct {
	descriptor ProviderDescriptor
	expand     func(ProviderRequest) ([]ProviderExpansion, error)
}

type typedTestProviderError struct {
	code reports.Code
}

func (err typedTestProviderError) Error() string { return "typed provider rejection" }

func (err typedTestProviderError) ArchitectureRejectionCode() reports.Code { return err.code }

func (provider staticTestProvider) Descriptor() ProviderDescriptor {
	return provider.descriptor
}

func (provider staticTestProvider) Expand(_ context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	return provider.expand(request)
}

func TestRegistryAndSearchAreDeterministicUnderProviderRequestAndExpansionOrder(t *testing.T) {
	precision := scoredTestProvider("precision_threshold", "threshold_detection", []testExpansionScore{{id: "precision", margin: 0.6, components: 2, power: 0.002, area: 12}})
	compact := scoredTestProvider("compact_threshold", "threshold_detection", []testExpansionScore{{id: "compact", margin: 0.5, components: 1, power: 0.001, area: 8}})
	firstRegistry, issues := NewRegistry(precision, compact)
	if len(issues) != 0 {
		t.Fatalf("first registry issues = %#v", issues)
	}
	secondRegistry, issues := NewRegistry(compact, precision)
	if len(issues) != 0 {
		t.Fatalf("second registry issues = %#v", issues)
	}
	if firstRegistry.Hash() != secondRegistry.Hash() {
		t.Fatalf("registry hash changed with registration order: %s != %s", firstRegistry.Hash(), secondRegistry.Hash())
	}

	firstRequirement := validRequirement()
	secondRequirement := cloneRequirement(firstRequirement)
	slices.Reverse(secondRequirement.Requirements.Domains)
	slices.Reverse(secondRequirement.Requirements.Ports)
	slices.Reverse(secondRequirement.Requirements.Objectives[0].Bindings)
	slices.Reverse(secondRequirement.Requirements.Objectives[0].Constraints)
	first := Search(context.Background(), firstRequirement, firstRegistry, SearchOptions{CatalogHash: "catalog-test"})
	second := Search(context.Background(), secondRequirement, secondRegistry, SearchOptions{CatalogHash: "catalog-test"})
	if first.Status != SearchSelected || first.Selected == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.Selected.Selections[0].ProviderID != "precision_threshold" {
		t.Fatalf("selected provider = %s, want precision_threshold; score=%#v", first.Selected.Selections[0].ProviderID, first.Selected.Score)
	}
	if len(first.Alternatives) != 1 || first.Alternatives[0].Selections[0].ProviderID != "compact_threshold" {
		t.Fatalf("alternatives = %#v", first.Alternatives)
	}
	if first.Rationale == nil || len(first.Rationale.Comparisons) != 1 || first.Rationale.Comparisons[0].FirstScoreField != "worst_margin" {
		t.Fatalf("selection rationale = %#v", first.Rationale)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("search bytes changed under input/provider reordering\n%s\n%s", firstJSON, secondJSON)
	}
	if first.Consumption.ExpandedStates != 1 || first.Consumption.GeneratedStates != 2 || first.Consumption.CompleteCandidates != 2 {
		t.Fatalf("consumption = %#v", first.Consumption)
	}
}

func TestRetainedFrontierPrefersSemanticTopologyDiversityOverPartVariants(t *testing.T) {
	direct := func(catalogID, value string) json.RawMessage {
		payload, err := MarshalFragmentRealization(FragmentRealization{
			Capability: "voltage_regulation",
			Instances: []RealizationInstance{{
				ID: "regulator", CatalogID: catalogID, Usage: "regulator", Value: value,
				RequiredFunctions: []string{"VIN", "VOUT"},
			}},
			PortBindings: []RealizationPortBinding{{Role: "input", Instance: "regulator", Function: "VIN"}, {Role: "output", Instance: "regulator", Function: "VOUT"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	feedback, err := MarshalFragmentRealization(FragmentRealization{
		Capability: "voltage_regulation",
		Instances: []RealizationInstance{
			{ID: "regulator", CatalogID: "regulator.adjustable", Usage: "regulator", RequiredFunctions: []string{"VIN", "VOUT", "ADJ"}},
			{ID: "feedback", CatalogID: "resistor.feedback", Usage: "feedback", Value: "10k", RequiredFunctions: []string{"A", "B"}},
		},
		PortBindings: []RealizationPortBinding{{Role: "input", Instance: "regulator", Function: "VIN"}, {Role: "output", Instance: "regulator", Function: "VOUT"}},
		Connections:  []RealizationConnection{{ID: "feedback", Role: "signal", Endpoints: []RealizationEndpoint{{Instance: "regulator", Function: "ADJ"}, {Instance: "feedback", Function: "A"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidates := []CandidateResult{
		{Fingerprint: "best", Selections: []FragmentSelection{{ObligationPath: "objectives.regulate", Capability: "voltage_regulation", Payload: direct("regulator.fixed.a", "3.3V")}}},
		{Fingerprint: "part-variant", Selections: []FragmentSelection{{ObligationPath: "objectives.regulate", Capability: "voltage_regulation", Payload: direct("regulator.fixed.b", "3.3V")}}},
		{Fingerprint: "feedback-topology", Selections: []FragmentSelection{{ObligationPath: "objectives.regulate", Capability: "voltage_regulation", Payload: feedback}}},
	}
	retained := retainTopologicallyDiverseCandidates(candidates, 2)
	if len(retained) != 2 || retained[0].Fingerprint != "best" || retained[1].Fingerprint != "feedback-topology" {
		t.Fatalf("retained candidates = %#v", retained)
	}
	if candidateTopologySignature(candidates[0]) != candidateTopologySignature(candidates[1]) {
		t.Fatal("part identity or value changed the semantic topology signature")
	}
}

func TestProviderRequestExcludesFixtureAndRequirementIdentity(t *testing.T) {
	provider := staticTestProvider{
		descriptor: validProviderDescriptor("identity_blind", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			for _, port := range request.Ports {
				if port.Anchor != "" || port.Contract.ID != "" {
					return nil, fmt.Errorf("provider received identity-bearing port %#v", port)
				}
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				return nil, err
			}
			for _, forbidden := range []string{"synthetic_threshold", "Synthetic threshold", "objective:detect", "external:"} {
				if strings.Contains(string(encoded), forbidden) {
					return nil, fmt.Errorf("provider request contains forbidden identity %q", forbidden)
				}
			}
			return []ProviderExpansion{compatibleExpansion(request, "blind", 1, 0.2)}, nil
		},
	}
	registry, issues := NewRegistry(provider)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchSelected {
		t.Fatalf("identity-blind search = %#v", result)
	}
}

func TestSignalProcessingObjectiveInheritsSharedReferenceDomain(t *testing.T) {
	requirement := Requirement{Requirements: Requirements{Domains: []Domain{{ID: "ground", Kind: "reference", NominalVoltageV: 0, Source: "external"}}}}
	objective := Objective{ID: "amplify", Capability: "signal_processing"}
	ports := []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "analog_voltage", Direction: "sink", Domain: "ground"}},
		{Role: "output", Contract: PortContract{Kind: "analog_voltage", Direction: "source", Domain: "ground"}},
	}
	reference, ok := inferredSignalReferenceRole(requirement, objective, ports, EvidenceRuleInferred)
	if !ok || reference.Role != "reference" || reference.Anchor != "domain:ground" || reference.Contract.Domain != "ground" || reference.Contract.Direction != "bidirectional" {
		t.Fatalf("inferred reference = %#v, ok=%t", reference, ok)
	}
	if _, ok := inferredSignalReferenceRole(requirement, objective, ports[1:], EvidenceRuleInferred); ok {
		t.Fatal("output-only objective inherited a signal-processing reference")
	}
	ports = append(ports, RoleContract{Role: "reference", Contract: PortContract{Kind: "reference", Direction: "bidirectional", Domain: "ground"}})
	if _, ok := inferredSignalReferenceRole(requirement, objective, ports, EvidenceRuleInferred); ok {
		t.Fatal("objective with an explicit reference inherited a duplicate")
	}
}

func TestSearchComposesChildCapabilityObligations(t *testing.T) {
	root := staticTestProvider{
		descriptor: validProviderDescriptor("threshold_root", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansion := compatibleExpansion(request, "protected_threshold", 1, 0.4)
			expansion.Children = []ChildObligation{{
				ID: "input_guard", Capability: "input_protection",
				Ports: []RoleContract{{Role: "protected", Contract: PortContract{
					Kind: "analog_voltage", Direction: "sink", Domain: "vcc",
					Voltage:         NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(5)},
					MinimumEvidence: EvidenceRuleInferred,
				}}},
			}}
			return []ProviderExpansion{expansion}, nil
		},
	}
	guard := staticTestProvider{
		descriptor: validProviderDescriptor("generic_input_guard", "input_protection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			return []ProviderExpansion{compatibleExpansion(request, "clamped_input", 1, 0.3)}, nil
		},
	}
	registry, issues := NewRegistry(guard, root)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchSelected || result.Selected == nil || len(result.Selected.Selections) != 2 || result.Selected.Score.ComponentCount != 2 || result.Selected.Score.FragmentCount != 2 {
		t.Fatalf("composed result = %#v", result)
	}
	capabilities := []string{result.Selected.Selections[0].Capability, result.Selected.Selections[1].Capability}
	slices.Sort(capabilities)
	if !reflect.DeepEqual(capabilities, []string{"input_protection", "threshold_detection"}) {
		t.Fatalf("selected capabilities = %#v", capabilities)
	}
}

func TestSearchComposesIndependentProvidersThroughTypedSignal(t *testing.T) {
	producer := lowDemandTestProvider("generic_producer_provider", "generic_producer", "producer", 0.02)
	consumer := lowDemandTestProvider("generic_consumer_provider", "generic_consumer", "consumer", 0.02)
	registry, issues := NewRegistry(producer, consumer)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	requirement := validMultiFunctionRequirement()
	first := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "synthetic_catalog"})
	if first.Status != SearchSelected || first.Selected == nil || len(first.Selected.Selections) != 2 {
		t.Fatalf("typed-signal search = %#v", first)
	}
	if first.PolicyVersion != PolicyVersionV2 || first.Coverage == nil || first.Coverage.Metrics.Total != 2 || first.Coverage.Metrics.Selected != 2 || first.Coverage.Metrics.Rejected != 0 || first.Coverage.Metrics.Unsupported != 0 || first.Coverage.Metrics.Ambiguous != 0 || first.Coverage.Metrics.BudgetExhausted != 0 {
		t.Fatalf("typed-signal coverage = %#v", first.Coverage)
	}
	if len(first.Selected.GlobalChecks) < 2 {
		t.Fatalf("global checks do not cover signal and domain budgets: %#v", first.Selected.GlobalChecks)
	}
	var endpoints []RoleContract
	for _, selection := range first.Selected.Selections {
		for _, port := range selection.Ports {
			if port.Anchor == signalAnchor("conditioned") {
				endpoints = append(endpoints, port)
			}
		}
	}
	if len(endpoints) != 2 || endpoints[0].Contract.Direction == endpoints[1].Contract.Direction {
		t.Fatalf("shared signal endpoints = %#v", endpoints)
	}

	reordered := cloneRequirement(requirement)
	slices.Reverse(reordered.Requirements.Objectives)
	for index := range reordered.Requirements.Objectives {
		slices.Reverse(reordered.Requirements.Objectives[index].Bindings)
	}
	second := Search(context.Background(), reordered, registry, SearchOptions{CatalogHash: "synthetic_catalog"})
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("typed-signal search changed under input reordering\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestV2GlobalCurrentBudgetAndHeadroomFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		demandA    float64
		headroomPC *float64
	}{
		{name: "aggregate demand", demandA: 0.06},
		{name: "required headroom", demandA: 0.03, headroomPC: float64Pointer(50)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement := validMultiFunctionRequirement()
			if test.headroomPC != nil {
				requirement.Requirements.SystemConstraints = append(requirement.Requirements.SystemConstraints, Constraint{Name: "supply_current_headroom", Relation: "minimum", Value: json.RawMessage(fmt.Sprintf("%g", *test.headroomPC)), Unit: "%"})
			}
			registry, issues := NewRegistry(
				lowDemandTestProvider("producer", "generic_producer", "producer", test.demandA),
				lowDemandTestProvider("consumer", "generic_consumer", "consumer", test.demandA),
			)
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			result := Search(context.Background(), requirement, registry, SearchOptions{})
			if result.Status != SearchUnsupported || result.Selected != nil || !rejectionSummaryContains(result.Rejections, CodeGlobalCurrentExceeded) {
				t.Fatalf("global-current result = %#v", result)
			}
		})
	}
}

func TestV2ZeroCurrentBudgetCannotProveHeadroom(t *testing.T) {
	requirement := Requirement{
		Version: VersionV2,
		Requirements: Requirements{
			Domains:           []Domain{{ID: "zero_supply", Kind: "supply", MaxCurrentA: float64Pointer(0)}},
			SystemConstraints: []Constraint{{Name: "supply_current_headroom", Relation: "minimum", Value: json.RawMessage(`20`), Unit: "%"}},
		},
	}
	selections := []FragmentSelection{{Ports: []RoleContract{{
		Role: "power", Anchor: "domain:zero_supply", Contract: PortContract{Kind: "power", Direction: "sink", Domain: "zero_supply", CurrentDemandA: float64Pointer(0)},
	}}}}
	_, rejection := validateCandidateGlobal(requirement, selections)
	if rejection == nil || rejection.Code != CodeGlobalCurrentUnknown {
		t.Fatalf("zero current budget headroom rejection = %#v", rejection)
	}
}

func TestV2UnknownSystemConstraintFailsClosed(t *testing.T) {
	requirement := validMultiFunctionRequirement()
	requirement.Requirements.SystemConstraints = []Constraint{{Name: "unmodeled_behavior", Relation: "required", Value: json.RawMessage(`true`)}}
	registry, issues := NewRegistry(
		lowDemandTestProvider("producer", "generic_producer", "producer", 0.02),
		lowDemandTestProvider("consumer", "generic_consumer", "consumer", 0.02),
	)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), requirement, registry, SearchOptions{})
	if result.Status != SearchUnsupported || result.Selected != nil || !rejectionSummaryContains(result.Rejections, CodeGlobalConstraintUnproven) {
		t.Fatalf("unknown global constraint did not fail closed: %#v", result)
	}
}

func TestV2CapabilityCoverageClassifiesUnsupportedRequest(t *testing.T) {
	requirement := validMultiFunctionRequirement()
	registry, issues := NewRegistry(scoredTestProvider("producer_only", "generic_producer", []testExpansionScore{{id: "producer", margin: 0.2, components: 1}}))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), requirement, registry, SearchOptions{})
	if result.Status != SearchUnsupported || result.Coverage == nil || result.Coverage.Metrics.Total != 2 || result.Coverage.Metrics.Unsupported != 1 {
		t.Fatalf("unsupported coverage = %#v result=%#v", result.Coverage, result)
	}
	if result.Coverage.Metrics.Total != result.Coverage.Metrics.Selected+result.Coverage.Metrics.Rejected+result.Coverage.Metrics.Unsupported+result.Coverage.Metrics.Ambiguous+result.Coverage.Metrics.BudgetExhausted {
		t.Fatalf("coverage metrics do not reconcile: %#v", result.Coverage.Metrics)
	}
}

func TestSearchRecordsDeterministicElectricalRejectionsAndFailsClosed(t *testing.T) {
	provider := staticTestProvider{
		descriptor: validProviderDescriptor("unsafe_threshold", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansion := compatibleExpansion(request, "unsafe", 1, 0.1)
			expansion.OfferedPorts[0].Contract.Voltage = NumericRange{}
			expansion.OfferedPorts[0].Contract.Evidence.Confidence = EvidencePlaceholder
			return []ProviderExpansion{expansion}, nil
		},
	}
	registry, issues := NewRegistry(provider)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchUnsupported || result.Selected != nil || len(result.Issues) != 1 || result.Issues[0].Code != CodeSearchNoCandidate {
		t.Fatalf("unsafe result = %#v", result)
	}
	for _, code := range []reports.Code{CodeVoltageEvidenceMissing, CodeEvidenceInsufficient} {
		if !rejectionSummaryContains(result.Rejections, code) {
			t.Fatalf("rejections lack %s: %#v", code, result.Rejections)
		}
	}
	encodedFirst, _ := json.Marshal(result.Rejections)
	second := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	encodedSecond, _ := json.Marshal(second.Rejections)
	if string(encodedFirst) != string(encodedSecond) {
		t.Fatalf("rejection evidence is not deterministic\n%s\n%s", encodedFirst, encodedSecond)
	}
}

func TestSearchPreservesTypedProviderRejectionCode(t *testing.T) {
	provider := staticTestProvider{
		descriptor: validProviderDescriptor("typed_rejection", "threshold_detection"),
		expand: func(ProviderRequest) ([]ProviderExpansion, error) {
			return nil, typedTestProviderError{code: CodeMCUPinAssignmentImpossible}
		},
	}
	registry, issues := NewRegistry(provider)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchUnsupported || !rejectionSummaryContains(result.Rejections, CodeMCUPinAssignmentImpossible) {
		t.Fatalf("typed provider rejection was not preserved: %#v", result)
	}
}

func TestSearchRejectsTamperedOrFailedCalculationEvidence(t *testing.T) {
	calculation, issues := SolveDivider(DividerRequest{
		ID: "provider_divider", Mode: DividerAttenuator,
		SourceVoltageV: 5, TargetVoltageV: 2.5, TargetTolerancePercent: 3,
		LowerResistanceOhm: 10000, LowerTolerancePercent: 1, UpperTolerancePercent: 1,
		UpperSeries: SeriesE96, MinimumUpperOhm: 1000, MaximumUpperOhm: 100000,
	})
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	calculation.Hash = "tampered"
	provider := staticTestProvider{
		descriptor: validProviderDescriptor("tampered_calculation", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansion := compatibleExpansion(request, "tampered", 1, 0.2)
			expansion.Calculations = []CalculationEvidence{calculation}
			return []ProviderExpansion{expansion}, nil
		},
	}
	registry, registryIssues := NewRegistry(provider)
	if len(registryIssues) != 0 {
		t.Fatal(registryIssues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchUnsupported || !rejectionSummaryContains(result.Rejections, CodeValueInputInvalid) {
		t.Fatalf("tampered calculation result = %#v", result)
	}
}

func TestUnsupportedCapabilityFailsClosed(t *testing.T) {
	registry, issues := NewRegistry()
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchUnsupported || len(result.Issues) != 1 || result.Issues[0].Code != CodeCapabilityUnsupported || !rejectionSummaryContains(result.Rejections, CodeCapabilityUnsupported) {
		t.Fatalf("unsupported result = %#v", result)
	}
}

func TestSearchBudgetExhaustionDoesNotReturnPartialSelection(t *testing.T) {
	recursive := staticTestProvider{
		descriptor: validProviderDescriptor("recursive_provider", "threshold_detection", "recursive_capability"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansion := compatibleExpansion(request, "recurse", 0, 0.1)
			expansion.Children = []ChildObligation{{ID: "again", Capability: "recursive_capability", Ports: cloneRoleContracts(request.Ports)}}
			return []ProviderExpansion{expansion}, nil
		},
	}
	registry, issues := NewRegistry(recursive)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	policy := DefaultSearchPolicy()
	policy.MaxDepth = 1
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{Policy: policy})
	if result.Status != SearchExhausted || result.Selected != nil || len(result.Issues) != 1 || result.Issues[0].Code != CodeSearchBudgetExhausted {
		t.Fatalf("depth-exhausted result = %#v", result)
	}

	many := staticTestProvider{
		descriptor: validProviderDescriptor("wide_provider", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansions := make([]ProviderExpansion, DefaultMaxProviderExpansions+1)
			for index := range expansions {
				expansions[index] = compatibleExpansion(request, fmt.Sprintf("candidate_%02d", index), 1, 0.1)
			}
			return expansions, nil
		},
	}
	wideRegistry, issues := NewRegistry(many)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	wide := Search(context.Background(), validRequirement(), wideRegistry, SearchOptions{})
	if wide.Status != SearchExhausted || wide.Selected != nil || !rejectionSummaryContains(wide.Rejections, CodeProviderExpansionLimit) {
		t.Fatalf("provider-limit result = %#v", wide)
	}
}

func TestTiedUserVisibleArchitecturesFailAsAmbiguous(t *testing.T) {
	provider := staticTestProvider{
		descriptor: validProviderDescriptor("choice_provider", "threshold_detection"),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			linear := compatibleExpansion(request, "linear", 1, 0.5)
			linear.DecisionClass = "linear"
			linear.RequiresUserChoice = true
			switching := compatibleExpansion(request, "switching", 1, 0.5)
			switching.DecisionClass = "switching"
			switching.RequiresUserChoice = true
			return []ProviderExpansion{switching, linear}, nil
		},
	}
	registry, issues := NewRegistry(provider)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{})
	if result.Status != SearchAmbiguous || result.Selected != nil || len(result.Issues) != 1 || result.Issues[0].Code != CodeSearchAmbiguous {
		t.Fatalf("ambiguous result = %#v", result)
	}
}

func TestRegistryAndPolicyValidationFailBeforeSearch(t *testing.T) {
	valid := scoredTestProvider("same_id", "threshold_detection", []testExpansionScore{{id: "one", margin: 0.1, components: 1}})
	duplicate := scoredTestProvider("same_id", "threshold_detection", []testExpansionScore{{id: "two", margin: 0.1, components: 1}})
	if _, issues := NewRegistry(valid, duplicate); !containsIssue(issues, CodeProviderDuplicate, ".id") {
		t.Fatalf("duplicate provider issues = %#v", issues)
	}
	placeholder := staticTestProvider{
		descriptor: ProviderDescriptor{ID: "placeholder", Revision: "1", Capabilities: []string{"threshold_detection"}, Evidence: ContractEvidence{Confidence: EvidencePlaceholder}},
		expand:     func(ProviderRequest) ([]ProviderExpansion, error) { return nil, errors.New("must not run") },
	}
	if _, issues := NewRegistry(placeholder); !containsIssue(issues, CodeProviderInvalid, "providers") {
		t.Fatalf("placeholder provider issues = %#v", issues)
	}

	registry, issues := NewRegistry(valid)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	policy := DefaultSearchPolicy()
	policy.MaxExpandedStates++
	result := Search(context.Background(), validRequirement(), registry, SearchOptions{Policy: policy})
	if result.Status != SearchFailed || !containsIssue(result.Issues, CodeLimitExceeded, "max_expanded_states") || result.Consumption.ExpandedStates != 0 {
		t.Fatalf("invalid policy result = %#v", result)
	}
}

type testExpansionScore struct {
	id         string
	margin     float64
	components int
	power      float64
	area       float64
}

func scoredTestProvider(id, capability string, scores []testExpansionScore) staticTestProvider {
	return staticTestProvider{
		descriptor: validProviderDescriptor(id, capability),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansions := make([]ProviderExpansion, 0, len(scores))
			for _, score := range scores {
				expansion := compatibleExpansion(request, score.id, score.components, score.margin)
				if score.power > 0 {
					expansion.Metrics.QuiescentPowerW = float64Pointer(score.power)
				}
				if score.area > 0 {
					expansion.Metrics.AreaMM2 = float64Pointer(score.area)
				}
				expansions = append(expansions, expansion)
			}
			slices.Reverse(expansions)
			return expansions, nil
		},
	}
}

func lowDemandTestProvider(id, capability, expansionID string, demandA float64) staticTestProvider {
	return staticTestProvider{
		descriptor: validProviderDescriptor(id, capability),
		expand: func(request ProviderRequest) ([]ProviderExpansion, error) {
			expansions := []ProviderExpansion{
				compatibleExpansion(request, expansionID, 1, 0.3),
				compatibleExpansion(request, expansionID+"_alternative", 2, 0.2),
			}
			for expansionIndex := range expansions {
				for index := range expansions[expansionIndex].OfferedPorts {
					contract := &expansions[expansionIndex].OfferedPorts[index].Contract
					if contract.Kind == "power" && contract.Direction == "sink" {
						contract.CurrentDemandA = float64Pointer(demandA)
					}
					if expansions[expansionIndex].OfferedPorts[index].Role == "output" && contract.Direction == "source" {
						contract.DefaultState = "inactive"
					}
				}
			}
			return expansions, nil
		},
	}
}

func compatibleExpansion(request ProviderRequest, id string, componentCount int, margin float64) ProviderExpansion {
	ports := cloneRoleContracts(request.Ports)
	for index := range ports {
		contract := &ports[index].Contract
		contract.ID = ""
		contract.MinimumEvidence = ""
		contract.Evidence = ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"synthetic_datasheet"}}
		contract.CurrentCapacityA = cloneFloat64(contract.RequiredCurrentCapacityA)
		contract.CurrentDemandA = cloneFloat64(contract.MaximumCurrentDemandA)
		contract.Traits = append(contract.Traits, contract.RequiredTraits...)
		contract.RequiredTraits = nil
	}
	components := make([]SelectedComponent, componentCount)
	for index := range components {
		components[index] = SelectedComponent{InstanceID: fmt.Sprintf("part_%02d", index), CatalogID: fmt.Sprintf("catalog_part_%02d", index), Evidence: EvidenceVerified}
	}
	return ProviderExpansion{
		ID: id, OfferedPorts: ports, Components: components,
		Metrics:  ExpansionMetrics{WorstMargin: float64Pointer(margin)},
		Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"synthetic_provider"}},
	}
}

func validProviderDescriptor(id string, capabilities ...string) ProviderDescriptor {
	return ProviderDescriptor{ID: id, Revision: "1.0.0", Capabilities: capabilities, Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"synthetic_provider_contract"}}}
}

func cloneRoleContracts(ports []RoleContract) []RoleContract {
	encoded, _ := json.Marshal(ports)
	var cloned []RoleContract
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}
