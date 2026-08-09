package opentopologysynthesis

import (
	"testing"

	"kicadai/internal/simmodel"
)

func TestLogicLevelStaticHarnessSeparatesHighAndLowStates(t *testing.T) {
	low, high := 0.0, 3.3
	requirement := Requirement{Requirements: Requirements{Ports: []Port{
		{ID: "input", Kind: "digital", Direction: "sink", Domain: "ground", Electrical: Electrical{MinVoltageV: &low, MaxVoltageV: &high}},
		{ID: "output", Kind: "digital", Direction: "source", Domain: "ground"},
	}, OperatingCases: []OperatingCase{{ID: "edge", Events: []OperatingEvent{{
		Kind: "input_step", Target: "input", Initial: low, Applied: high,
	}}}}, BehavioralRequirements: []BehavioralAssertion{{
		Metric: "rise_time", Analysis: simmodel.AnalysisTransient,
		Observation: Observation{Kind: "port", ID: "output"}, OperatingCases: []string{"edge"},
	}}}}
	for _, test := range []struct {
		metric string
		want   float64
	}{
		{metric: "output_high_voltage", want: high},
		{metric: "output_low_voltage", want: low},
	} {
		conditions := logicLevelStaticHarnessConditions(requirement, BehavioralAssertion{
			Metric: test.metric, Analysis: simmodel.AnalysisDCOperatingPoint,
			Observation: Observation{Kind: "port", ID: "output"},
		}, nil)
		if len(conditions) != 1 || conditions[0].Axis != "input_voltage" ||
			conditions[0].Target != "input" || conditions[0].Min != test.want ||
			conditions[0].Max != test.want || conditions[0].Unit != "V" {
			t.Fatalf("%s conditions = %#v", test.metric, conditions)
		}
	}
}

func TestLogicLevelStaticHarnessUsesAuthoredDynamicPolarity(t *testing.T) {
	low, high := 0.0, 3.3
	requirement := Requirement{Requirements: Requirements{
		Ports: []Port{
			{ID: "input", Kind: "digital", Direction: "sink", Electrical: Electrical{MinVoltageV: &low, MaxVoltageV: &high}},
			{ID: "output", Kind: "digital", Direction: "source"},
		},
		OperatingCases: []OperatingCase{{ID: "falling_input", Events: []OperatingEvent{{
			Kind: "input_step", Target: "input", Initial: high, Applied: low,
		}}}},
		BehavioralRequirements: []BehavioralAssertion{{
			Metric: "rise_time", Analysis: simmodel.AnalysisTransient,
			Observation: Observation{Kind: "port", ID: "output"}, OperatingCases: []string{"falling_input"},
		}},
	}}
	conditions := logicLevelStaticHarnessConditions(requirement, BehavioralAssertion{
		Metric: "output_high_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: Observation{Kind: "port", ID: "output"},
	}, nil)
	if len(conditions) != 1 || conditions[0].Min != low || conditions[0].Max != low {
		t.Fatalf("inverting high-state conditions = %#v, want low input", conditions)
	}
}

func TestLogicLevelStaticHarnessRejectsMultiTransitionPolarity(t *testing.T) {
	low, high := 0.0, 3.3
	requirement := Requirement{Requirements: Requirements{
		Ports: []Port{
			{ID: "input", Kind: "digital", Direction: "sink", Electrical: Electrical{MinVoltageV: &low, MaxVoltageV: &high}},
			{ID: "output", Kind: "digital", Direction: "source"},
		},
		OperatingCases: []OperatingCase{{ID: "pulse", Events: []OperatingEvent{
			{Kind: "input_step", Target: "input", Initial: low, Applied: high},
			{Kind: "input_step", Target: "input", Initial: high, Applied: low},
		}}},
		BehavioralRequirements: []BehavioralAssertion{{
			Metric: "rise_time", Analysis: simmodel.AnalysisTransient,
			Observation: Observation{Kind: "port", ID: "output"}, OperatingCases: []string{"pulse"},
		}},
	}}
	conditions := logicLevelStaticHarnessConditions(requirement, BehavioralAssertion{
		Metric: "output_high_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: Observation{Kind: "port", ID: "output"},
	}, nil)
	if len(conditions) != 0 {
		t.Fatalf("multi-transition case produced inferred conditions: %#v", conditions)
	}
}

func TestLogicLevelStaticHarnessDoesNotGuessAmbiguousOrAuthoredStimulus(t *testing.T) {
	low, high := 0.0, 3.3
	input := Port{ID: "input", Kind: "digital", Direction: "sink", Electrical: Electrical{MinVoltageV: &low, MaxVoltageV: &high}}
	assertion := BehavioralAssertion{
		Metric: "output_high_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: Observation{Kind: "port", ID: "output"},
	}
	authored := []OperatingCondition{{Axis: "input_voltage", Target: "input", Min: 1, Max: 2, Unit: "V"}}
	if got := logicLevelStaticHarnessConditions(Requirement{Requirements: Requirements{Ports: []Port{input}}}, assertion, authored); len(got) != 1 || got[0] != authored[0] {
		t.Fatalf("authored condition was replaced: %#v", got)
	}
	second := input
	second.ID = "other_input"
	if got := logicLevelStaticHarnessConditions(Requirement{Requirements: Requirements{Ports: []Port{input, second}}}, assertion, nil); len(got) != 0 {
		t.Fatalf("ambiguous inputs produced inferred conditions: %#v", got)
	}
	excitation := Observation{Kind: "port", ID: "input"}
	assertion.Excitation = &excitation
	if got := logicLevelStaticHarnessConditions(Requirement{Requirements: Requirements{Ports: []Port{input}}}, assertion, nil); len(got) != 0 {
		t.Fatalf("explicit excitation produced inferred conditions: %#v", got)
	}
}
