package architecturesearch

import (
	"math"
	"reflect"
	"testing"
)

func TestButterworthStageQFourthOrder(t *testing.T) {
	first, ok := ButterworthStageQ(4, 0)
	if !ok || math.Abs(first-0.541196100146197) > 1e-12 {
		t.Fatalf("first Q = %.15g, ok=%v", first, ok)
	}
	second, ok := ButterworthStageQ(4, 1)
	if !ok || math.Abs(second-1.30656296487638) > 1e-12 {
		t.Fatalf("second Q = %.15g, ok=%v", second, ok)
	}
	if _, ok := ButterworthStageQ(3, 0); ok {
		t.Fatal("odd-order decomposition unexpectedly accepted")
	}
}

func TestSolveSallenKeyLowPassIsDeterministicAndCornerBounded(t *testing.T) {
	request := SallenKeyLowPassRequest{ID: "stage", TargetFrequencyHz: 2000, FrequencyTolerancePercent: 5, TargetQ: 1.30656296487638, QTolerancePercent: 5, ResistanceOhm: 10000, ResistanceTolerancePercent: 0.1, CapacitanceTolerancePercent: 1, CapacitanceSeries: SeriesE96}
	first, issues := SolveSallenKeyLowPass(request)
	if len(issues) != 0 {
		t.Fatalf("solve issues: %+v", issues)
	}
	second, issues := SolveSallenKeyLowPass(request)
	if len(issues) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("solver replay differs: issues=%+v\nfirst=%+v\nsecond=%+v", issues, first, second)
	}
	if !first.Pass || first.CornerEvaluations != 16 || len(ValidateCalculation(first)) != 0 {
		t.Fatalf("calculation is not complete and valid: %+v", first)
	}
	if frequency, ok := calculationOutput(first, "natural_frequency"); !ok || math.Abs(frequency-2000)/2000 > 0.05 {
		t.Fatalf("natural frequency = %g, ok=%v", frequency, ok)
	}
	if quality, ok := calculationOutput(first, "quality_factor"); !ok || math.Abs(quality-request.TargetQ)/request.TargetQ > 0.05 {
		t.Fatalf("quality factor = %g, ok=%v", quality, ok)
	}
}

func TestSolveSallenKeyLowPassRejectsImpossibleTolerance(t *testing.T) {
	_, issues := SolveSallenKeyLowPass(SallenKeyLowPassRequest{ID: "stage", TargetFrequencyHz: 2000, FrequencyTolerancePercent: 0, TargetQ: 1.30656296487638, QTolerancePercent: 0, ResistanceOhm: 10000, ResistanceTolerancePercent: 1, CapacitanceTolerancePercent: 5, CapacitanceSeries: SeriesE96})
	if !containsIssue(issues, CodeToleranceFailed, "sallen_key_low_pass") {
		t.Fatalf("issues = %+v", issues)
	}
}
