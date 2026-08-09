package opentopologysynthesis

import (
	"math"
	"testing"
)

func TestDynamicTimeStepResolvesAuthoredTimingBoundAndEvent(t *testing.T) {
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: 10e-6}}}
	step := dynamicTimeStep(.01, operatingCase, 10e-9)
	if step != 10e-9 {
		t.Fatalf("timing-bound step = %.12g, want 10 ns", step)
	}
	stepsToEvent := operatingCase.Events[0].TriggerTimeS / step
	if math.Abs(stepsToEvent-math.Round(stepsToEvent)) > 1e-9 {
		t.Fatalf("event is not aligned to timing-bound step: %.12g steps", stepsToEvent)
	}
}

func TestDynamicTimeStepRejectsUnrepresentableDuration(t *testing.T) {
	if step := dynamicTimeStep(.1e-12, OperatingCase{}, 0); step != 0 {
		t.Fatalf("sub-tick duration step = %.12g, want fail-closed zero", step)
	}
}

func TestDynamicResolutionForAssertionIsNarrowAndBounded(t *testing.T) {
	maximum := 1e-6
	for _, metric := range []string{"rise_time", "fall_time", "propagation_delay", "settling_time"} {
		if got := dynamicResolutionForAssertion(BehavioralAssertion{Metric: metric, Max: &maximum}); got != 10e-9 {
			t.Fatalf("%s resolution = %.12g, want 10 ns", metric, got)
		}
	}
	if got := dynamicResolutionForAssertion(BehavioralAssertion{Metric: "peak_voltage", Max: &maximum}); got != 0 {
		t.Fatalf("non-timing resolution = %.12g, want zero", got)
	}
}

func TestDynamicDurationKeepsBoundedTimingWindowAfterEvent(t *testing.T) {
	maximum := 1e-6
	assertion := BehavioralAssertion{Metric: "rise_time", Max: &maximum}
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: 10e-6}}}
	duration := dynamicDurationAtFrequency(assertion, operatingCase, 0)
	if math.Abs(duration-20e-6) > 1e-15 {
		t.Fatalf("timing duration = %.12g, want 20 us", duration)
	}
	step := dynamicTimeStep(duration, operatingCase, dynamicResolutionForAssertion(assertion))
	if duration/step > 3900 {
		t.Fatalf("timing grid requires %.0f steps", duration/step)
	}
}

func TestDynamicDurationUsesPeriodicWindowOnlyWhenRequested(t *testing.T) {
	maximum, frequency := 1e-6, 1000.0
	assertion := BehavioralAssertion{Metric: "rise_time", Max: &maximum}
	if duration := dynamicDurationAtFrequency(assertion, OperatingCase{}, frequency); math.Abs(duration-10e-6) > 1e-15 {
		t.Fatalf("inferred-frequency edge duration = %.12g, want 10 us", duration)
	}
	assertion.FrequencyHz = &frequency
	if duration := dynamicDurationAtFrequency(assertion, OperatingCase{}, frequency); duration != .01 {
		t.Fatalf("explicit-frequency edge duration = %.12g, want 10 ms", duration)
	}
}

func TestDynamicDurationRetainsLongNonTimingObservationFloor(t *testing.T) {
	maximum := 5.0
	assertion := BehavioralAssertion{Metric: "peak_voltage", Max: &maximum}
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: 10e-6}}}
	if duration := dynamicDurationAtFrequency(assertion, operatingCase, 0); duration != .01 {
		t.Fatalf("non-timing duration = %.12g, want 10 ms", duration)
	}
}

func TestDynamicDurationRejectsEventBeyondSupportedHorizon(t *testing.T) {
	maximum := 5.0
	assertion := BehavioralAssertion{Metric: "peak_voltage", Max: &maximum}
	operatingCase := OperatingCase{Events: []OperatingEvent{{TriggerTimeS: maximumDynamicDurationS + 1}}}
	if duration := dynamicDurationAtFrequency(assertion, operatingCase, 0); duration != 0 {
		t.Fatalf("over-horizon event duration = %.12g, want fail-closed zero", duration)
	}
}
