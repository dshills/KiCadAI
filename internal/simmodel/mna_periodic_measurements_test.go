package simmodel

import (
	"math"
	"testing"
)

func TestPeriodicWaveformMetricsMeasureStableFrequencyAndDutyCycle(t *testing.T) {
	const (
		periodS = .001
		stepS   = .0001
	)
	times := make([]float64, 0, 51)
	values := make([]float64, 0, 51)
	for index := 0; index <= 50; index++ {
		timeS := float64(index) * stepS
		value := 0.0
		if math.Mod(timeS, periodS) < .0002 {
			value = 5
		}
		times = append(times, timeS)
		values = append(values, value)
	}
	assertion := Assertion{AnalysisID: "periodic", Node: "OUT", Quantity: QuantityDutyCyclePct}
	frequency, dutyCycle, diagnostic := periodicWaveformMetrics(times, values, assertion)
	if diagnostic != nil {
		t.Fatalf("periodic waveform diagnostic = %#v", diagnostic)
	}
	if math.Abs(frequency-1000) > 1e-6 || math.Abs(dutyCycle-20) > 1e-6 {
		t.Fatalf("periodic metrics = %.12g Hz %.12g%%, want 1000 Hz 20%%", frequency, dutyCycle)
	}
}

func TestPeriodicWaveformMetricsRejectUnsettledPeriods(t *testing.T) {
	times := []float64{0, .1, .2, .3, .4, .5, .6, .7, .8, .9, 1, 1.1, 1.2, 1.3, 1.4}
	values := []float64{0, 1, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}
	assertion := Assertion{AnalysisID: "drift", Node: "OUT", Quantity: QuantityOscillationFrequencyHz}
	if _, _, diagnostic := periodicWaveformMetrics(times, values, assertion); diagnostic == nil {
		t.Fatal("unsettled periods were accepted as stable oscillation")
	}
}

func TestPeriodicWaveformMetricsIgnoreBriefSwitchingOvershoot(t *testing.T) {
	const (
		periodS = 50e-6
		stepS   = .5e-6
	)
	times := make([]float64, 0, 1001)
	values := make([]float64, 0, 1001)
	for index := 0; index <= 1000; index++ {
		timeS := float64(index) * stepS
		phase := math.Mod(timeS, periodS)
		value := 0.0
		if phase >= periodS/2 {
			value = 12
			if phase < periodS/2+stepS {
				value = 32
			}
		}
		times = append(times, timeS)
		values = append(values, value)
	}
	assertion := Assertion{AnalysisID: "switching", Node: "OUT", Quantity: QuantityDutyCyclePct}
	frequency, dutyCycle, diagnostic := periodicWaveformMetrics(times, values, assertion)
	if diagnostic != nil {
		t.Fatalf("overshooting periodic waveform diagnostic = %#v", diagnostic)
	}
	if math.Abs(frequency-20_000) > 1e-6 || math.Abs(dutyCycle-50) > .5 {
		t.Fatalf("overshooting periodic metrics = %.12g Hz %.12g%%, want 20000 Hz 50%%", frequency, dutyCycle)
	}
}

func TestTransientOutputRippleUsesLastTwoKnownCycles(t *testing.T) {
	result := AnalysisResult{ID: "ripple", Kind: AnalysisTransient, FundamentalFrequencyHz: 1000}
	for index := 0; index <= 40; index++ {
		value := 5.0
		if index < 20 {
			value = 10 - float64(index)*.25
		} else if index%2 == 0 {
			value = 4.9
		} else {
			value = 5.1
		}
		result.Points = append(result.Points, AnalysisPoint{
			TimeS: float64(index) * .0001,
			Nodes: []NodeResult{{Node: "OUT", Real: value}},
		})
	}
	actual, diagnostic := transientDerivedValue(result, Assertion{
		AnalysisID: "ripple", Node: "OUT", Quantity: QuantityOutputRippleVPP,
	})
	if diagnostic != nil || math.Abs(actual-.2) > 1e-12 {
		t.Fatalf("steady-state ripple = %.12g, want .2; diagnostic=%#v", actual, diagnostic)
	}
}

func TestDCSweepVoltageSlopeMeasuresTransferRatio(t *testing.T) {
	result := AnalysisResult{ID: "gain", Kind: AnalysisDCOperatingPoint, Points: []AnalysisPoint{
		{Sweep: dcSweepForward, SweepValue: -2, Nodes: []NodeResult{{Node: "OUT", Real: -1}}},
		{Sweep: dcSweepForward, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepForward, SweepValue: 2, Nodes: []NodeResult{{Node: "OUT", Real: 1}}},
	}}
	actual, diagnostic := dcSweepSpanOrSlope(result, Assertion{
		AnalysisID: "gain", Node: "OUT", Quantity: QuantityDCSweepVoltageSlopeVPerV,
	})
	if diagnostic != nil || math.Abs(actual-.5) > 1e-12 {
		t.Fatalf("DC sweep voltage slope = %.12g, want .5; diagnostic=%#v", actual, diagnostic)
	}
}
