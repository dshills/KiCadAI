package architecturesearch

import (
	"math"
	"testing"
)

func TestSolveCurrentSenseProvesWorstCaseTransferAndPower(t *testing.T) {
	evidence, issues := SolveCurrentSense(CurrentSenseRequest{
		ID: "load_current", FullScaleCurrentA: 2, TargetOutputVoltageV: 2, OutputTolerancePercent: 2, MaximumOutputVoltageV: 2.5,
		ShuntResistanceOhm: 0.01, ShuntTolerancePercent: 0.5, ShuntPowerRatingW: 1,
		AmplifierGain: 100, AmplifierGainErrorPercent: 0.2, InputOffsetVoltageV: 25e-6,
	})
	if len(issues) != 0 || !evidence.Pass {
		t.Fatalf("expected passing evidence, issues=%v evidence=%+v", issues, evidence)
	}
	if evidence.CornerEvaluations != 2 || evidence.Hash == "" {
		t.Fatalf("expected deterministic corner and hash evidence: %+v", evidence)
	}
}

func TestSolveCurrentSenseFailsClosedOnInsufficientAccuracy(t *testing.T) {
	_, issues := SolveCurrentSense(CurrentSenseRequest{
		ID: "load_current", FullScaleCurrentA: 2, TargetOutputVoltageV: 2, OutputTolerancePercent: 0.5, MaximumOutputVoltageV: 2.5,
		ShuntResistanceOhm: 0.01, ShuntTolerancePercent: 1, ShuntPowerRatingW: 1,
		AmplifierGain: 100, AmplifierGainErrorPercent: 1, InputOffsetVoltageV: 100e-6,
	})
	if len(issues) == 0 || issues[0].Code != CodeToleranceFailed {
		t.Fatalf("expected tolerance failure, got %v", issues)
	}
}

func TestSolveCurrentSenseRejectsNonFiniteTolerances(t *testing.T) {
	base := CurrentSenseRequest{
		ID: "load_current", FullScaleCurrentA: 2, TargetOutputVoltageV: 2, OutputTolerancePercent: 2, MaximumOutputVoltageV: 2.5,
		ShuntResistanceOhm: 0.01, ShuntTolerancePercent: 0.5, ShuntPowerRatingW: 1,
		AmplifierGain: 100, AmplifierGainErrorPercent: 0.2, InputOffsetVoltageV: 25e-6,
	}
	for _, test := range []struct {
		name   string
		mutate func(*CurrentSenseRequest)
	}{
		{name: "shunt_nan", mutate: func(request *CurrentSenseRequest) { request.ShuntTolerancePercent = math.NaN() }},
		{name: "shunt_inf", mutate: func(request *CurrentSenseRequest) { request.ShuntTolerancePercent = math.Inf(1) }},
		{name: "gain_nan", mutate: func(request *CurrentSenseRequest) { request.AmplifierGainErrorPercent = math.NaN() }},
		{name: "gain_inf", mutate: func(request *CurrentSenseRequest) { request.AmplifierGainErrorPercent = math.Inf(1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			_, issues := SolveCurrentSense(request)
			if len(issues) == 0 || issues[0].Code != CodeValueInputInvalid {
				t.Fatalf("expected invalid-input rejection, got %v", issues)
			}
		})
	}
}
