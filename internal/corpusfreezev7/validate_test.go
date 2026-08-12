package corpusfreezev7

import (
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

func TestPrimaryBehavioralRequirementUsesRawIDOrder(t *testing.T) {
	requirement := ots.Requirement{Requirements: ots.Requirements{BehavioralRequirements: []ots.BehavioralAssertion{
		{ID: "z_last", Analysis: "noise", Metric: "output_noise_rms"},
		{ID: "a_first", Analysis: "transient", Metric: "settling_time"},
	}}}
	got, err := primaryBehavioralRequirement(requirement)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "a_first" || got.Analysis != "transient" {
		t.Fatalf("primary assertion = %#v", got)
	}
	if _, err := primaryBehavioralRequirement(ots.Requirement{}); err == nil {
		t.Fatal("empty behavioral requirements unexpectedly selected a primary")
	}
}

func TestInterfaceShapeIsIdentifierNormalized(t *testing.T) {
	positive := 5.0
	first := ots.Requirement{Requirements: ots.Requirements{
		Domains: []ots.Domain{{ID: "zero", Kind: "reference"}, {ID: "rail", Kind: "supply", NominalVoltageV: &positive}},
		Ports:   []ots.Port{{ID: "in", Kind: "analog_voltage", Direction: "sink", Domain: "zero"}, {ID: "out", Kind: "analog_current", Direction: "source", Domain: "rail"}},
	}}
	second := ots.Requirement{Requirements: ots.Requirements{
		Domains: []ots.Domain{{ID: "supply_b", Kind: "supply", NominalVoltageV: &positive}, {ID: "reference_b", Kind: "reference"}},
		Ports:   []ots.Port{{ID: "result", Kind: "analog_current", Direction: "source", Domain: "supply_b"}, {ID: "stimulus", Kind: "analog_voltage", Direction: "sink", Domain: "reference_b"}},
	}}
	firstShape, err := interfaceShape(first)
	if err != nil {
		t.Fatal(err)
	}
	secondShape, err := interfaceShape(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstShape != secondShape {
		t.Fatalf("normalized shapes differ:\n%s\n%s", firstShape, secondShape)
	}
}

func TestInterfaceShapePreservesDomainDependency(t *testing.T) {
	positive := 5.0
	first := ots.Requirement{Requirements: ots.Requirements{
		Domains: []ots.Domain{{ID: "rail_a", Kind: "supply", NominalVoltageV: &positive}, {ID: "rail_b", Kind: "supply", NominalVoltageV: &positive}},
		Ports:   []ots.Port{{ID: "one", Kind: "power", Direction: "sink", Domain: "rail_a"}, {ID: "two", Kind: "analog_voltage", Direction: "source", Domain: "rail_a"}},
	}}
	second := first
	second.Requirements.Ports = append([]ots.Port(nil), first.Requirements.Ports...)
	second.Requirements.Ports[1].Domain = "rail_b"
	firstShape, err := interfaceShape(first)
	if err != nil {
		t.Fatal(err)
	}
	secondShape, err := interfaceShape(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstShape == secondShape {
		t.Fatal("normalized interface shape discarded domain dependency")
	}
	second.Requirements.Ports[1].Domain = "missing"
	if _, err := interfaceShape(second); err == nil {
		t.Fatal("unresolved interface domain unexpectedly produced a shape")
	}
}

func TestBehaviorSignatureRejectsIdentifierOnlyVariants(t *testing.T) {
	first := signatureFixture("input_a", "output_a", "case_a", "assertion_a")
	second := signatureFixture("stimulus_b", "result_b", "condition_b", "measurement_b")
	firstShape, err := interfaceShape(first)
	if err != nil {
		t.Fatal(err)
	}
	secondShape, err := interfaceShape(second)
	if err != nil {
		t.Fatal(err)
	}
	firstSignature := mustBehaviorSignature(t, first, firstShape)
	secondSignature := mustBehaviorSignature(t, second, secondShape)
	if firstSignature != secondSignature {
		t.Fatalf("identifier-only variants produced different signatures:\n%s\n%s", firstSignature, secondSignature)
	}
	second.Requirements.BehavioralRequirements[0].Metric = "settling_time"
	second.Requirements.BehavioralRequirements[0].Unit = "s"
	if firstSignature == mustBehaviorSignature(t, second, secondShape) {
		t.Fatal("electrically distinct metric did not change signature")
	}
	second = signatureFixture("stimulus_b", "result_b", "condition_b", "measurement_b")
	*second.Requirements.BehavioralRequirements[0].Max = 2
	if firstSignature == mustBehaviorSignature(t, second, secondShape) {
		t.Fatal("electrically distinct bound did not change signature")
	}
	second.Requirements.BehavioralRequirements[0].Observation.ID = "missing"
	if _, err := behaviorSignature(second, secondShape); err == nil {
		t.Fatal("unresolved signature observation unexpectedly passed")
	}
}

func TestFirstLimitViolationIsDeterministic(t *testing.T) {
	key, count := firstLimitViolation(map[string]int{"z": 20, "a": 11, "b": 12}, 10)
	if key != "a" || count != 11 {
		t.Fatalf("first violation = %s/%d", key, count)
	}
	if key, count := firstLimitViolation(map[string]int{"a": 10}, 10); key != "" || count != 0 {
		t.Fatalf("non-violation = %s/%d", key, count)
	}
}

func TestV7AggregateDiagnosticsAreOutcomeNeutral(t *testing.T) {
	for _, diagnostic := range []string{
		"V7_AGGREGATE_BUNDLE_MISSING", "V7_AGGREGATE_REQUIREMENT_MISSING", "V7_AGGREGATE_STRICT_DECODE",
		"V7_SAFETY_CATEGORY_TOTAL", "V7_PRIMARY_ANALYSIS_LIMIT", "V7_PRIMARY_METRIC_LIMIT",
		"V7_INTERFACE_SHAPE_LIMIT", "V7_NEAR_DUPLICATE_SIGNATURE",
	} {
		for _, prohibited := range []string{"pass", "unsupported", "unsafe", "exhausted", "capability", "synthesis"} {
			if strings.Contains(strings.ToLower(diagnostic), prohibited) {
				t.Fatalf("diagnostic %q leaks outcome or implementation term %q", diagnostic, prohibited)
			}
		}
	}
}

func signatureFixture(input, output, operatingCase, assertionID string) ots.Requirement {
	minimum, maximum := 0.0, 1.0
	return ots.Requirement{Requirements: ots.Requirements{
		Domains: []ots.Domain{{ID: "reference", Kind: "reference"}},
		Ports: []ots.Port{
			{ID: input, Kind: "analog_voltage", Direction: "sink", Domain: "reference"},
			{ID: output, Kind: "analog_voltage", Direction: "source", Domain: "reference"},
		},
		OperatingCases: []ots.OperatingCase{{ID: operatingCase, Conditions: []ots.OperatingCondition{{Axis: "input_voltage", Target: input, Min: 0, Max: 1, Unit: "V"}}}},
		BehavioralRequirements: []ots.BehavioralAssertion{{
			ID: assertionID, Analysis: "dc_sweep", Metric: "output_voltage", Excitation: &ots.Observation{Kind: "port", ID: input},
			Observation: ots.Observation{Kind: "port", ID: output}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{operatingCase},
		}},
	}}
}

func mustBehaviorSignature(t *testing.T, requirement ots.Requirement, shape string) string {
	t.Helper()
	signature, err := behaviorSignature(requirement, shape)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}
