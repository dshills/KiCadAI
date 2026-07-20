package simmodel

import (
	"math"
)

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
		for _, point := range result.Points[1:] {
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
	}
	return 0, advancedAssertionDiagnostic(assertion, "unsupported transient-derived quantity")
}

func dcDeviceValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if len(result.Points) != 1 {
		return 0, advancedAssertionDiagnostic(assertion, "DC device assertion requires exactly one operating point")
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
	for _, point := range result.Points {
		if point.Sweep != direction {
			continue
		}
		for _, node := range point.Nodes {
			if node.Node == assertion.Node {
				samples = append(samples, sample{sweep: point.SweepValue, output: node.Real})
				minimum, maximum = math.Min(minimum, node.Real), math.Max(maximum, node.Real)
				break
			}
		}
	}
	if len(samples) < 3 || !finite(minimum) || !finite(maximum) || maximum-minimum <= 1e-9*math.Max(1, math.Max(math.Abs(minimum), math.Abs(maximum))) {
		return 0, advancedAssertionDiagnostic(assertion, "DC sweep output does not contain a resolved decision transition")
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

func waveform(result AnalysisResult, assertion Assertion) ([]float64, []float64, *Diagnostic) {
	times := make([]float64, 0, len(result.Points))
	values := make([]float64, 0, len(result.Points))
	for _, point := range result.Points {
		for _, node := range point.Nodes {
			if node.Node == assertion.Node {
				times = append(times, point.TimeS)
				values = append(values, node.Real)
				break
			}
		}
	}
	if len(values) < 2 {
		return nil, nil, advancedAssertionDiagnostic(assertion, "waveform-derived assertion requires at least two solved node samples")
	}
	return times, values, nil
}
