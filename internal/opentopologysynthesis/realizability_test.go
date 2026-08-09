package opentopologysynthesis

import (
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestRequirementRealizabilityClassifiesDirectVoltageBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		metric     string
		minimum    *float64
		maximum    *float64
		wantEnergy bool
	}{
		{name: "high exact positive rail", metric: "output_high_voltage", minimum: realizabilityFloat(3)},
		{name: "high above positive rail", metric: "output_high_voltage", minimum: realizabilityFloat(3.0001), wantEnergy: true},
		{name: "output interval at positive rail", metric: "output_voltage", minimum: realizabilityFloat(2.8), maximum: realizabilityFloat(3)},
		{name: "output interval above positive rail", metric: "output_voltage", minimum: realizabilityFloat(3.1), maximum: realizabilityFloat(3.2), wantEnergy: true},
		{name: "low exact reference", metric: "output_low_voltage", maximum: realizabilityFloat(0)},
		{name: "low below reference", metric: "output_low_voltage", maximum: realizabilityFloat(-0.01), wantEnergy: true},
		{name: "peak exact rail magnitude", metric: "peak_voltage", minimum: realizabilityFloat(3)},
		{name: "peak above rail magnitude", metric: "peak_voltage", minimum: realizabilityFloat(3.01), wantEnergy: true},
		{name: "swing exact rail span", metric: "output_swing", minimum: realizabilityFloat(3)},
		{name: "swing above rail span", metric: "output_swing", minimum: realizabilityFloat(3.01), wantEnergy: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement := realizabilityTestRequirement()
			assertion := &requirement.Requirements.BehavioralRequirements[0]
			assertion.Metric, assertion.Min, assertion.Max = test.metric, test.minimum, test.maximum
			assessment := AssessRequirementRealizability(requirement)
			if len(assessment.Issues) != 0 {
				t.Fatalf("assessment issues: %#v", assessment.Issues)
			}
			gotEnergy := findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil
			if gotEnergy != test.wantEnergy {
				t.Fatalf("energy-domain finding = %t, want %t: %#v", gotEnergy, test.wantEnergy, assessment.Findings)
			}
		})
	}
}

func TestRequirementRealizabilityUsesLeastFavorableBipolarAndCrossingZeroCorners(t *testing.T) {
	requirement := realizabilityTestRequirement()
	negativeMinimum, negativeNominal, negativeMaximum := -5.0, -4.0, -3.0
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{
		ID: "negative", Kind: "supply", Source: "external",
		MinVoltageV: &negativeMinimum, NominalVoltageV: &negativeNominal, MaxVoltageV: &negativeMaximum,
	})
	requirement.Requirements.OperatingCases[0].Conditions = append(
		requirement.Requirements.OperatingCases[0].Conditions,
		OperatingCondition{Axis: "supply_voltage", Target: "negative", Min: -5, Max: -3, Unit: "V"},
	)
	assertion := &requirement.Requirements.BehavioralRequirements[0]
	assertion.Metric, assertion.Min, assertion.Max = "output_swing", realizabilityFloat(6), nil
	if assessment := AssessRequirementRealizability(requirement); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil {
		t.Fatalf("exact bipolar span was rejected: %#v", assessment)
	}
	assertion.Min = realizabilityFloat(6.01)
	if assessment := AssessRequirementRealizability(requirement); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) == nil {
		t.Fatalf("beyond-bipolar span was not classified: %#v", assessment)
	}

	crossing := realizabilityTestRequirement()
	crossing.Requirements.OperatingCases[0].Conditions[0].Min = -0.1
	crossing.Requirements.BehavioralRequirements[0].Min = realizabilityFloat(0.1)
	if assessment := AssessRequirementRealizability(crossing); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) == nil {
		t.Fatalf("zero-crossing supply falsely guaranteed a positive rail: %#v", assessment)
	}
}

func TestRequirementRealizabilityLeavesMissingSupplyBoundsUnclassified(t *testing.T) {
	requirement := realizabilityTestRequirement()
	supply := &requirement.Requirements.Domains[1]
	supply.MinVoltageV, supply.NominalVoltageV, supply.MaxVoltageV = nil, nil, nil
	requirement.Requirements.OperatingCases[0].Conditions = requirement.Requirements.OperatingCases[0].Conditions[1:]
	requirement.Requirements.BehavioralRequirements[0].Min = realizabilityFloat(100)
	assessment := AssessRequirementRealizability(requirement)
	if len(assessment.Issues) != 0 || findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil {
		t.Fatalf("unknown supply envelope was treated as a proven bound: %#v", assessment)
	}

	nominalOnly := realizabilityTestRequirement()
	nominalOnly.Requirements.Domains[1].MinVoltageV = nil
	nominalOnly.Requirements.Domains[1].MaxVoltageV = nil
	nominalOnly.Requirements.OperatingCases[0].Conditions = nominalOnly.Requirements.OperatingCases[0].Conditions[1:]
	nominalOnly.Requirements.BehavioralRequirements[0].Min = realizabilityFloat(100)
	assessment = AssessRequirementRealizability(nominalOnly)
	if len(assessment.Issues) != 0 || findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil {
		t.Fatalf("nominal-only supply was treated as a guaranteed bound: %#v", assessment)
	}
}

func TestRequirementRealizabilityIntersectsSharedDomainSupplyConditions(t *testing.T) {
	requirement := realizabilityTestRequirement()
	requirement.Requirements.OperatingCases[0].Conditions = append(
		requirement.Requirements.OperatingCases[0].Conditions,
		OperatingCondition{Axis: "supply_voltage", Target: "supply_in", Min: 4, Max: 5, Unit: "V"},
	)
	requirement.Requirements.BehavioralRequirements[0].Min = realizabilityFloat(4)
	if assessment := AssessRequirementRealizability(requirement); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil {
		t.Fatalf("exact intersected supply floor was rejected: %#v", assessment)
	}
	requirement.Requirements.BehavioralRequirements[0].Min = realizabilityFloat(4.01)
	if assessment := AssessRequirementRealizability(requirement); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) == nil {
		t.Fatalf("shared-domain condition intersection was ignored: %#v", assessment)
	}

	requirement.Requirements.OperatingCases[0].Conditions[0].Max = 3.5
	if assessment := AssessRequirementRealizability(requirement); findingWithCode(assessment.Findings, CodeEnergyDomainCreationRequired) != nil {
		t.Fatalf("contradictory shared-domain conditions were not left uncertain: %#v", assessment)
	}
}

func TestRequirementRealizabilityClassifiesIndependentObligationGraphs(t *testing.T) {
	multiOutput := realizabilityTestRequirement()
	multiOutput.Requirements.Ports = append(multiOutput.Requirements.Ports, Port{
		ID: "secondary_out", Kind: "digital", Direction: "source", Domain: "reference",
		Electrical: Electrical{MinVoltageV: realizabilityFloat(0), MaxVoltageV: realizabilityFloat(3)},
	})
	multiOutput.Requirements.BehavioralRequirements = append(multiOutput.Requirements.BehavioralRequirements, BehavioralAssertion{
		ID: "secondary_level", Metric: "output_high_voltage", Analysis: "dc_operating_point",
		Observation: Observation{Kind: "port", ID: "secondary_out"}, Min: realizabilityFloat(2.5),
		Unit: "V", OperatingCases: []string{"nominal"},
	})
	assessment := AssessRequirementRealizability(multiOutput)
	if findingWithCode(assessment.Findings, CodeMultiOutputCompositionRequired) == nil ||
		findingWithCode(assessment.Findings, CodeMultiControlCompositionRequired) != nil {
		t.Fatalf("multi-output assessment = %#v", assessment)
	}

	multiControl := realizabilityTestRequirement()
	multiControl.Requirements.Ports = append(multiControl.Requirements.Ports, Port{
		ID: "enable_in", Kind: "digital", Direction: "sink", Domain: "reference",
		Electrical: Electrical{MinVoltageV: realizabilityFloat(0), MaxVoltageV: realizabilityFloat(3)},
	})
	multiControl.Requirements.BehavioralRequirements = append(
		multiControl.Requirements.BehavioralRequirements,
		BehavioralAssertion{
			ID: "signal_trip", Metric: "threshold_voltage", Analysis: "dc_sweep",
			Excitation: &Observation{Kind: "port", ID: "signal_in"}, Observation: Observation{Kind: "port", ID: "signal_out"},
			Min: realizabilityFloat(1), Max: realizabilityFloat(2), Unit: "V", OperatingCases: []string{"nominal"},
		},
		BehavioralAssertion{
			ID: "enable_delay", Metric: "propagation_delay", Analysis: "transient",
			Excitation: &Observation{Kind: "port", ID: "enable_in"}, Observation: Observation{Kind: "port", ID: "signal_out"},
			Max: realizabilityFloat(.001), Unit: "s", OperatingCases: []string{"nominal"},
		},
	)
	assessment = AssessRequirementRealizability(multiControl)
	finding := findingWithCode(assessment.Findings, CodeMultiControlCompositionRequired)
	if finding == nil || findingWithCode(assessment.Findings, CodeMultiOutputCompositionRequired) != nil ||
		!reflect.DeepEqual(finding.RequirementIDs, []string{"enable_delay", "signal_trip"}) {
		t.Fatalf("multi-control assessment = %#v", assessment)
	}

	singleControl := multiControl
	singleControl.Requirements.BehavioralRequirements[2].Excitation.ID = "signal_in"
	if assessment := AssessRequirementRealizability(singleControl); findingWithCode(assessment.Findings, CodeMultiControlCompositionRequired) != nil {
		t.Fatalf("repeated single control was misclassified: %#v", assessment)
	}
}

func TestRequirementRealizabilityRejectsInvalidAndReplaysDeterministically(t *testing.T) {
	requirement := realizabilityTestRequirement()
	first := AssessRequirementRealizability(requirement)
	second := AssessRequirementRealizability(requirement)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("assessment replay drift:\nfirst=%#v\nsecond=%#v", first, second)
	}
	requirement.Schema = "invalid"
	invalid := AssessRequirementRealizability(requirement)
	if len(invalid.Issues) == 0 || len(invalid.Findings) != 0 {
		t.Fatalf("invalid requirement assessment = %#v", invalid)
	}
}

func realizabilityTestRequirement() Requirement {
	return Requirement{
		Schema: RequirementSchema, Version: RequirementVersion,
		Project: Project{Name: "public_boundary", Title: "Public Boundary", Description: "Hand-checkable realizability boundary."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "reference", Kind: "reference", Source: "external", MinVoltageV: realizabilityFloat(0), NominalVoltageV: realizabilityFloat(0), MaxVoltageV: realizabilityFloat(0)},
				{ID: "supply", Kind: "supply", Source: "external", MinVoltageV: realizabilityFloat(3), NominalVoltageV: realizabilityFloat(4), MaxVoltageV: realizabilityFloat(5)},
			},
			Ports: []Port{
				{ID: "supply_in", Kind: "power", Direction: "sink", Domain: "supply", Electrical: Electrical{MinVoltageV: realizabilityFloat(3), NominalVoltageV: realizabilityFloat(4), MaxVoltageV: realizabilityFloat(5)}},
				{ID: "signal_in", Kind: "analog_voltage", Direction: "sink", Domain: "reference", Electrical: Electrical{MinVoltageV: realizabilityFloat(0), NominalVoltageV: realizabilityFloat(.5), MaxVoltageV: realizabilityFloat(1)}},
				{ID: "signal_out", Kind: "analog_voltage", Direction: "source", Domain: "reference", Electrical: Electrical{MinVoltageV: realizabilityFloat(0), NominalVoltageV: realizabilityFloat(1.5), MaxVoltageV: realizabilityFloat(3)}},
			},
			OperatingCases: []OperatingCase{{
				ID: "nominal",
				Conditions: []OperatingCondition{
					{Axis: "supply_voltage", Target: "supply", Min: 3, Max: 5, Unit: "V"},
					{Axis: "input_voltage", Target: "signal_in", Min: 0, Max: 1, Unit: "V"},
				},
			}},
			BehavioralRequirements: []BehavioralAssertion{{
				ID: "output_level", Metric: "output_high_voltage", Analysis: "dc_operating_point",
				Observation: Observation{Kind: "port", ID: "signal_out"}, Min: realizabilityFloat(3),
				Unit: "V", OperatingCases: []string{"nominal"},
			}},
			Constraints: BoardLimits{MaxComponents: 12, MaxWidthMM: 40, MaxHeightMM: 30},
		},
		Acceptance: Acceptance{
			RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true,
			RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
			RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
			RequireDeterministicReplay: true, RequireFailClosed: true,
		},
	}
}

func findingWithCode(findings []RequirementRealizabilityFinding, code reports.Code) *RequirementRealizabilityFinding {
	for index := range findings {
		if findings[index].Code == code {
			return &findings[index]
		}
	}
	return nil
}

func realizabilityFloat(value float64) *float64 {
	return &value
}
