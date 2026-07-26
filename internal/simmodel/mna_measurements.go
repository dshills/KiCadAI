package simmodel

import (
	"fmt"
	"math"
)

// TransientResponseOnsetFraction is the normalized waveform change used to
// identify the onset of an event response. Synthesis calculations that bound
// event latency use the same exported value so sizing and measurement cannot
// silently drift apart.
const TransientResponseOnsetFraction = 0.1

func acDerivedValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if assertion.Quantity == QuantityVoltageGainRatio {
		for _, point := range result.Points {
			if math.Abs(point.FrequencyHz-assertion.FrequencyHz) > math.Max(1, math.Abs(point.FrequencyHz))*1e-12 {
				continue
			}
			output, outputFound := analysisNodeMagnitude(point, assertion.Node)
			reference, referenceFound := analysisNodeMagnitude(point, assertion.ReferenceNode)
			if !outputFound || !referenceFound || reference <= 0 {
				return 0, advancedAssertionDiagnostic(assertion, "AC gain assertion requires solved output and nonzero reference-node magnitudes")
			}
			return normalizedMNAFloat(output / reference), nil
		}
		return 0, advancedAssertionDiagnostic(assertion, "AC gain assertion frequency is absent from the solved sweep")
	}
	if len(result.Points) < 2 {
		return 0, advancedAssertionDiagnostic(assertion, "cutoff/bandwidth assertion requires at least two solved sweep points")
	}
	gains := make([]float64, len(result.Points))
	for index, point := range result.Points {
		output, outputFound := analysisNodeMagnitude(point, assertion.Node)
		reference, referenceFound := analysisNodeMagnitude(point, assertion.ReferenceNode)
		if !outputFound || !referenceFound || reference <= 0 {
			return 0, advancedAssertionDiagnostic(assertion, "cutoff/bandwidth assertion requires solved output and nonzero reference-node magnitudes")
		}
		gains[index] = output / reference
	}
	threshold := gains[0] / math.Sqrt2
	if threshold <= 0 {
		return 0, advancedAssertionDiagnostic(assertion, "cutoff/bandwidth passband gain is zero")
	}
	for index := 1; index < len(gains); index++ {
		if gains[index-1] >= threshold && gains[index] <= threshold {
			fraction := logarithmicCrossingFraction(gains[index-1], gains[index], threshold)
			start := math.Log(result.Points[index-1].FrequencyHz)
			stop := math.Log(result.Points[index].FrequencyHz)
			return normalizedMNAFloat(math.Exp(start + fraction*(stop-start))), nil
		}
	}
	return 0, advancedAssertionDiagnostic(assertion, "solved AC sweep does not bracket the -3 dB cutoff")
}

func transientDerivedValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	switch assertion.Quantity {
	case QuantityPeakAbsVoltageV:
		return peakAbsVoltage(result, assertion)
	case QuantityPeakAbsDeviceVoltageV, QuantityPeakAbsDeviceCurrentA:
		peak := math.Inf(-1)
		found := false
		for _, point := range result.Points {
			if !pointInAssertionWindow(point, assertion) {
				continue
			}
			for _, device := range point.Devices {
				if device.Component != assertion.Component {
					continue
				}
				value := math.Abs(device.VoltageV)
				if assertion.Quantity == QuantityPeakAbsDeviceCurrentA {
					value = math.Max(math.Abs(device.CurrentA), device.CurrentMagnitudeA)
				}
				peak = math.Max(peak, value)
				found = true
			}
		}
		if !found {
			return 0, advancedAssertionDiagnostic(assertion, "peak device-stress assertion did not resolve to a solved device waveform")
		}
		return normalizedMNAFloat(peak), nil
	case QuantityOutputSwingVPP:
		_, values, diagnostic := waveform(result, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		minimum, maximum := values[0], values[0]
		for _, value := range values[1:] {
			minimum, maximum = math.Min(minimum, value), math.Max(maximum, value)
		}
		return normalizedMNAFloat(maximum - minimum), nil
	case QuantityOvershootVoltageV:
		_, values, diagnostic := waveform(result, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		initial, final := values[0], values[len(values)-1]
		overshoot := 0.0
		if final >= initial {
			maximum := final
			for _, value := range values {
				maximum = math.Max(maximum, value)
			}
			overshoot = maximum - final
		} else {
			minimum := final
			for _, value := range values {
				minimum = math.Min(minimum, value)
			}
			overshoot = final - minimum
		}
		return normalizedMNAFloat(math.Max(0, overshoot)), nil
	case QuantitySettlingTimeS:
		times, values, diagnostic := waveform(result, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		minimum, maximum := values[0], values[0]
		for _, value := range values[1:] {
			minimum, maximum = math.Min(minimum, value), math.Max(maximum, value)
		}
		tolerance := math.Max(1e-12, .02*(maximum-minimum))
		final := values[len(values)-1]
		for index := range values {
			settled := true
			for _, value := range values[index:] {
				if math.Abs(value-final) > tolerance {
					settled = false
					break
				}
			}
			if settled {
				return normalizedMNAFloat(times[index] - times[0]), nil
			}
		}
		return 0, advancedAssertionDiagnostic(assertion, "transient waveform does not settle inside the trusted 2% band")
	case QuantityResponseTimeS:
		if assertion.WindowEndS > assertion.WindowStartS {
			return transientResponseLatency(result, assertion)
		}
		times, values, diagnostic := waveform(result, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		quantity := QuantityRiseTimeS
		if values[len(values)-1] < values[0] {
			quantity = QuantityFallTimeS
		}
		copy := assertion
		copy.Quantity = quantity
		_ = times
		return transientEdgeTime(result, copy)
	case QuantityOutputPowerW:
		if len(result.Points) < 2 {
			return 0, advancedAssertionDiagnostic(assertion, "output-power assertion requires a solved waveform")
		}
		sum, count := 0.0, 0
		points := periodicSteadyStatePoints(result)
		if len(points) == len(result.Points) {
			points = points[1:]
		}
		for _, point := range points {
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					sum += math.Abs(device.VoltageV * device.CurrentA)
					count++
					break
				}
			}
		}
		if count == 0 {
			return 0, advancedAssertionDiagnostic(assertion, "output-power assertion did not resolve to load voltage/current evidence")
		}
		return normalizedMNAFloat(sum / float64(count)), nil
	case QuantityConversionEfficiencyPct:
		if assertion.Component == "" || len(assertion.Components) == 0 {
			return 0, advancedAssertionDiagnostic(assertion, "conversion-efficiency assertion requires one resolved load and at least one resolved supply source")
		}
		points := periodicSteadyStatePoints(result)
		if len(points) == len(result.Points) && len(points) > 1 {
			points = points[1:]
		}
		outputEnergy, inputEnergy, count := 0.0, 0.0, 0
		for _, point := range points {
			outputPower, outputFound := 0.0, false
			inputPower := 0.0
			inputFound := map[string]bool{}
			for _, device := range point.Devices {
				power := math.Abs(device.VoltageV * device.CurrentA)
				if device.Component == assertion.Component {
					outputPower, outputFound = power, true
				}
				for _, source := range assertion.Components {
					if device.Component == source {
						inputPower += power
						inputFound[source] = true
					}
				}
			}
			if !outputFound || len(inputFound) != len(assertion.Components) {
				continue
			}
			outputEnergy += outputPower
			inputEnergy += inputPower
			count++
		}
		if count == 0 || inputEnergy <= 0 {
			return 0, advancedAssertionDiagnostic(assertion, "conversion-efficiency assertion lacks complete nonzero load and supply power waveforms")
		}
		return normalizedMNAFloat(100 * outputEnergy / inputEnergy), nil
	}
	return 0, advancedAssertionDiagnostic(assertion, "unsupported transient-derived quantity")
}

func transientResponseLatency(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	var baseline float64
	baselineFound := false
	var preEventTimes, preEventValues, times, residuals []float64
	scale := 1.0
	for _, point := range result.Points {
		value, found := analysisNodeReal(point, assertion.Node)
		if !found {
			continue
		}
		if assertion.ReferenceNode != "" {
			reference, referenceFound := analysisNodeReal(point, assertion.ReferenceNode)
			if !referenceFound {
				return 0, advancedAssertionDiagnostic(assertion, "event-response assertion reference node is absent from a solved point")
			}
			value -= reference
		}
		if point.TimeS < assertion.WindowStartS {
			baseline, baselineFound = value, true
			preEventTimes = append(preEventTimes, point.TimeS)
			preEventValues = append(preEventValues, value)
			continue
		}
		if point.TimeS >= assertion.WindowEndS {
			break
		}
		if !baselineFound {
			baseline, baselineFound = value, true
		}
		expected := baseline
		if result.FundamentalFrequencyHz > 0 {
			var expectedFound bool
			expected, expectedFound = periodicEventBaseline(
				preEventTimes, preEventValues, point.TimeS, assertion.WindowStartS, 1/result.FundamentalFrequencyHz,
			)
			if !expectedFound {
				return 0, advancedAssertionDiagnostic(assertion, "periodic event-response assertion requires one complete solved pre-event cycle")
			}
		}
		times = append(times, point.TimeS)
		residuals = append(residuals, math.Abs(value-expected))
		scale = math.Max(scale, math.Max(math.Abs(value), math.Abs(expected)))
	}
	if !baselineFound || len(residuals) < 2 {
		return 0, advancedAssertionDiagnostic(assertion, "event-response assertion requires a baseline and at least two solved samples inside its bounded window")
	}
	peak := 0.0
	for _, residual := range residuals {
		peak = math.Max(peak, residual)
	}
	if peak <= 1e-12*scale {
		return 0, advancedAssertionDiagnostic(assertion, fmt.Sprintf(
			"event-response assertion requires a nonconstant solved waveform (baseline=%.12g peak_delta=%.12g scale=%.12g samples=%d)",
			baseline, peak, scale, len(residuals),
		))
	}
	threshold := TransientResponseOnsetFraction * peak
	previousTime, previousResidual := assertion.WindowStartS, 0.0
	for index, residual := range residuals {
		if residual >= threshold {
			crossing := interpolateCrossing(previousTime, times[index], previousResidual, residual, threshold)
			return normalizedMNAFloat(math.Max(0, crossing-assertion.WindowStartS)), nil
		}
		previousTime, previousResidual = times[index], residual
	}
	return 0, advancedAssertionDiagnostic(assertion, "trusted event window does not contain a complete response onset")
}

func periodicEventBaseline(times, values []float64, timeS, eventStartS, periodS float64) (float64, bool) {
	if len(times) != len(values) || len(times) < 2 || !finite(periodS) || periodS <= 0 {
		return 0, false
	}
	phase := math.Mod(timeS-eventStartS, periodS)
	if phase < 0 {
		phase += periodS
	}
	target := eventStartS - periodS + phase
	tolerance := math.Max(1, math.Max(math.Abs(target), math.Abs(eventStartS))) * 1e-12
	for index := range times {
		if math.Abs(times[index]-target) <= tolerance {
			return values[index], true
		}
		if index == 0 || times[index] < target || times[index-1] > target {
			continue
		}
		if times[index] == times[index-1] {
			return 0, false
		}
		fraction := (target - times[index-1]) / (times[index] - times[index-1])
		return values[index-1] + fraction*(values[index]-values[index-1]), true
	}
	return 0, false
}

func dcDeviceValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if len(result.Points) == 0 {
		return 0, advancedAssertionDiagnostic(assertion, "DC device assertion requires at least one operating point")
	}
	if len(result.Points) > 1 {
		if assertion.Quantity != QuantityTransimpedanceOhm {
			return 0, advancedAssertionDiagnostic(assertion, "DC device-current assertion requires exactly one operating point")
		}
		minimumCurrent, maximumCurrent := math.Inf(1), math.Inf(-1)
		minimumVoltage, maximumVoltage := 0.0, 0.0
		for _, point := range result.Points {
			if point.Sweep != "" && point.Sweep != dcSweepForward {
				continue
			}
			current, voltage, currentOK, voltageOK := 0.0, 0.0, false, false
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					current, currentOK = device.CurrentMagnitudeA, true
					break
				}
			}
			for _, node := range point.Nodes {
				if node.Node == assertion.Node {
					voltage, voltageOK = node.Real, true
					break
				}
			}
			if !currentOK || !voltageOK {
				continue
			}
			if current < minimumCurrent {
				minimumCurrent, minimumVoltage = current, voltage
			}
			if current > maximumCurrent {
				maximumCurrent, maximumVoltage = current, voltage
			}
		}
		span := maximumCurrent - minimumCurrent
		if !finite(span) || span <= 1e-15 {
			return 0, advancedAssertionDiagnostic(assertion, "transimpedance DC sweep does not span two finite input-current levels")
		}
		return normalizedMNAFloat(math.Abs((maximumVoltage - minimumVoltage) / span)), nil
	}
	if assertion.Quantity == QuantityTotalSupplyCurrentA {
		remaining := make(map[string]struct{}, len(assertion.Components))
		for _, component := range assertion.Components {
			remaining[component] = struct{}{}
		}
		total := 0.0
		for _, device := range result.Points[0].Devices {
			if _, ok := remaining[device.Component]; !ok {
				continue
			}
			total += device.CurrentMagnitudeA
			delete(remaining, device.Component)
		}
		if len(remaining) != 0 {
			return 0, advancedAssertionDiagnostic(assertion, "total-supply-current assertion component is absent from the solved point")
		}
		return normalizedMNAFloat(total), nil
	}
	for _, device := range result.Points[0].Devices {
		if device.Component != assertion.Component {
			continue
		}
		current := device.CurrentMagnitudeA
		if assertion.Quantity == QuantityDeviceCurrentA {
			return current, nil
		}
		if current <= 0 {
			return 0, advancedAssertionDiagnostic(assertion, "transimpedance assertion input current is zero")
		}
		for _, node := range result.Points[0].Nodes {
			if node.Node == assertion.Node {
				return normalizedMNAFloat(node.Real / current), nil
			}
		}
		return 0, advancedAssertionDiagnostic(assertion, "transimpedance assertion output node is absent")
	}
	return 0, advancedAssertionDiagnostic(assertion, "device-current assertion component is absent from the solved point")
}

func dcSweepDerivedValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	forward, diagnostic := dcSweepTransition(result, assertion, dcSweepForward)
	if diagnostic != nil {
		return 0, diagnostic
	}
	if assertion.Quantity != QuantityHysteresisVoltageV {
		return forward, nil
	}
	reverse, diagnostic := dcSweepTransition(result, assertion, dcSweepReverse)
	if diagnostic != nil {
		return 0, diagnostic
	}
	return normalizedMNAFloat(math.Abs(forward - reverse)), nil
}

func dcSweepTransition(result AnalysisResult, assertion Assertion, direction string) (float64, *Diagnostic) {
	type sample struct {
		sweep  float64
		output float64
	}
	var samples []sample
	minimum, maximum := math.Inf(1), math.Inf(-1)
	minimumSweep, maximumSweep := math.Inf(1), math.Inf(-1)
	for _, point := range result.Points {
		if point.Sweep != direction {
			continue
		}
		for _, node := range point.Nodes {
			if node.Node == assertion.Node {
				samples = append(samples, sample{sweep: point.SweepValue, output: node.Real})
				minimum, maximum = math.Min(minimum, node.Real), math.Max(maximum, node.Real)
				minimumSweep, maximumSweep = math.Min(minimumSweep, point.SweepValue), math.Max(maximumSweep, point.SweepValue)
				break
			}
		}
	}
	if len(samples) < 3 || !finite(minimum) || !finite(maximum) || maximum-minimum <= 1e-9*math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum))) {
		if len(samples) >= 3 && finite(minimum) && finite(maximum) {
			return censoredDCSweepAssertionValue(minimumSweep, maximumSweep, assertion), nil
		}
		return 0, advancedAssertionDiagnostic(assertion, fmt.Sprintf("DC sweep output does not contain enough finite decision samples (samples=%d, output_min=%.12g, output_max=%.12g)", len(samples), minimum, maximum))
	}
	midpoint := minimum + .5*(maximum-minimum)
	transitions := make([]float64, 0, 1)
	for index := 1; index < len(samples); index++ {
		left, right := samples[index-1], samples[index]
		leftDelta, rightDelta := left.output-midpoint, right.output-midpoint
		if leftDelta == 0 {
			transitions = append(transitions, left.sweep)
			continue
		}
		if rightDelta == 0 {
			transitions = append(transitions, right.sweep)
			continue
		}
		if leftDelta*rightDelta < 0 {
			fraction := math.Abs(leftDelta) / (math.Abs(leftDelta) + math.Abs(rightDelta))
			transitions = append(transitions, left.sweep+fraction*(right.sweep-left.sweep))
		}
	}
	if len(transitions) != 1 {
		return 0, advancedAssertionDiagnostic(assertion, "DC sweep must contain exactly one unambiguous decision transition in each required direction")
	}
	return normalizedMNAFloat(transitions[0]), nil
}

func censoredDCSweepAssertionValue(minimumSweep, maximumSweep float64, assertion Assertion) float64 {
	if minimumSweep < assertion.Min {
		return normalizedMNAFloat(minimumSweep)
	}
	if maximumSweep > assertion.Max {
		return normalizedMNAFloat(maximumSweep)
	}
	margin := math.Max(maximumSweep-minimumSweep, math.Max(1, math.Max(math.Abs(assertion.Min), math.Abs(assertion.Max)))*1e-9)
	return normalizedMNAFloat(assertion.Max + margin)
}

func waveform(result AnalysisResult, assertion Assertion) ([]float64, []float64, *Diagnostic) {
	times := make([]float64, 0, len(result.Points))
	values := make([]float64, 0, len(result.Points))
	for _, point := range result.Points {
		if !pointInAssertionWindow(point, assertion) {
			continue
		}
		value, found := analysisNodeReal(point, assertion.Node)
		if !found {
			continue
		}
		if assertion.ReferenceNode != "" {
			reference, referenceFound := analysisNodeReal(point, assertion.ReferenceNode)
			if !referenceFound {
				return nil, nil, advancedAssertionDiagnostic(assertion, "waveform-derived assertion reference node is absent from a solved point")
			}
			value -= reference
		}
		times = append(times, point.TimeS)
		values = append(values, value)
	}
	if len(values) < 2 {
		return nil, nil, advancedAssertionDiagnostic(assertion, "waveform-derived assertion requires at least two solved node samples")
	}
	return times, values, nil
}

func pointInAssertionWindow(point AnalysisPoint, assertion Assertion) bool {
	if assertion.WindowEndS == 0 {
		return true
	}
	return point.TimeS >= assertion.WindowStartS && point.TimeS < assertion.WindowEndS
}

func analysisNodeReal(point AnalysisPoint, node string) (float64, bool) {
	for _, result := range point.Nodes {
		if result.Node == node {
			return result.Real, true
		}
	}
	return 0, false
}
