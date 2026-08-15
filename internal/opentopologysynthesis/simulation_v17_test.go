package opentopologysynthesis

import "testing"

func TestDynamicTimeStepWithLimitBoundsPathologicalEventGrid(t *testing.T) {
	duration := 1.0
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: 0.123456789012}}}
	legacy := dynamicTimeStep(duration, operatingCase, 0)
	bounded := dynamicTimeStepV17(duration, operatingCase, 0)
	if bounded < duration/float64(v17MaximumDynamicTimeSteps) {
		t.Fatalf("bounded step %.12g is below the deterministic limit", bounded)
	}
	if bounded < legacy {
		t.Fatalf("bounded step %.12g is finer than legacy %.12g", bounded, legacy)
	}
	if steps := duration / bounded; steps > float64(v17MaximumDynamicTimeSteps)*(1+1e-12) {
		t.Fatalf("bounded grid has %.12g steps", steps)
	}
}

func TestV17StreamingHashMatchesLegacyCanonicalBytes(t *testing.T) {
	value := SimulationEvaluation{
		Schema: SimulationEvaluationSchema,
		Status: SimulationEvaluationFailed,
		Attempts: []SimulationAttempt{{
			Number: 1, RequirementID: "behavior", Diagnostics: []SimulationDiagnostic{},
		}},
		Diagnoses: []Diagnosis{},
		Issues:    nil,
	}
	got := hashJSONV17(value)
	want := hashJSON(value)
	if got != want {
		t.Fatalf("streamed hash %s differs from canonical legacy hash %s", got, want)
	}
}

func TestV17OptionsAreStrictlyBounded(t *testing.T) {
	if v17MaximumDynamicTimeSteps <= 0 || v17MaximumDynamicTimeSteps >= maximumDynamicTimeSteps {
		t.Fatalf("invalid V17 dynamic step limit %d", v17MaximumDynamicTimeSteps)
	}
	if v17RetainedReportPoints < 2 {
		t.Fatalf("invalid V17 retained report point limit %d", v17RetainedReportPoints)
	}
}
