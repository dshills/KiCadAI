package simmodel

import (
	"fmt"
	"math"
	"slices"
)

// maximumTrustedOpenCircuitImpedanceOhm is the finite reporting ceiling used
// when the solved excitation current is exactly zero. Keeping the sentinel
// finite preserves canonical JSON and bounds downstream margin arithmetic.
const maximumTrustedOpenCircuitImpedanceOhm = 1e15

// TransientResponseOnsetFraction is the normalized waveform change used to
// identify the onset of an event response. Synthesis calculations that bound
// event latency use the same exported value so sizing and measurement cannot
// silently drift apart.
const TransientResponseOnsetFraction = 0.1

func acDerivedValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if assertion.Quantity == QuantityTransimpedanceOhm {
		for _, point := range result.Points {
			if math.Abs(point.FrequencyHz-assertion.FrequencyHz) > math.Max(1, math.Abs(point.FrequencyHz))*1e-12 {
				continue
			}
			output, outputFound := analysisNodeMagnitude(point, assertion.Node)
			current := 0.0
			currentFound := false
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					current = device.CurrentMagnitudeA
					currentFound = true
					break
				}
			}
			if !outputFound || !currentFound || current <= 0 {
				return 0, advancedAssertionDiagnostic(assertion, "AC transimpedance assertion requires a solved output voltage and nonzero excitation-source current")
			}
			return normalizedMNAFloat(output / current), nil
		}
		return 0, advancedAssertionDiagnostic(assertion, "AC transimpedance assertion frequency is absent from the solved sweep")
	}
	if assertion.Quantity == QuantityInputImpedanceOhm {
		for _, point := range result.Points {
			if math.Abs(point.FrequencyHz-assertion.FrequencyHz) > math.Max(1, math.Abs(point.FrequencyHz))*1e-12 {
				continue
			}
			input, inputFound := analysisNodeMagnitude(point, assertion.Node)
			reference := 0.0
			referenceFound := assertion.ReferenceNode == ""
			if assertion.ReferenceNode != "" {
				reference, referenceFound = analysisNodeMagnitude(point, assertion.ReferenceNode)
			}
			current := 0.0
			currentFound := false
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					current = device.CurrentMagnitudeA
					currentFound = true
					break
				}
			}
			if !inputFound || !referenceFound || !currentFound {
				return 0, advancedAssertionDiagnostic(assertion, "AC input-impedance assertion requires solved input/reference voltages and nonzero excitation-source current")
			}
			voltage := math.Abs(input - reference)
			if current <= 0 && voltage > 0 {
				// Currents below the solver's normalization floor represent an
				// effectively open modeled input. Return the evaluator's finite
				// trusted impedance ceiling so reports remain canonical JSON.
				return maximumTrustedOpenCircuitImpedanceOhm, nil
			}
			if current <= 0 {
				return 0, advancedAssertionDiagnostic(assertion, "AC input-impedance assertion requires nonzero excitation voltage")
			}
			return normalizedMNAFloat(voltage / current), nil
		}
		return 0, advancedAssertionDiagnostic(assertion, "AC input-impedance assertion frequency is absent from the solved sweep")
	}
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
		reference, referenceFound := 0.0, false
		if assertion.Component != "" {
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					reference, referenceFound = device.CurrentMagnitudeA, true
					break
				}
			}
		} else {
			reference, referenceFound = analysisNodeMagnitude(point, assertion.ReferenceNode)
		}
		if !outputFound || !referenceFound || reference <= 0 {
			return 0, advancedAssertionDiagnostic(assertion, "cutoff/bandwidth assertion requires solved output and nonzero voltage- or current-excitation magnitude")
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
	if assertion.Quantity == QuantityBandwidthHz &&
		gains[len(gains)-1] > threshold && assertion.Max >= 1e11 {
		// A minimum-only bandwidth requirement does not need the actual
		// upper pole when the solved sweep has already reached a frequency
		// above the requirement while remaining inside the -3 dB passband.
		// Report the final solved frequency as a conservative lower bound;
		// finite upper bounds still require a bracketed crossing.
		return normalizedMNAFloat(result.Points[len(result.Points)-1].FrequencyHz), nil
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
	case QuantityFinalAbsDeviceCurrentA:
		latestTime := math.Inf(-1)
		value := 0.0
		found := false
		for _, point := range result.Points {
			if !pointInAssertionWindow(point, assertion) || point.TimeS < latestTime {
				continue
			}
			for _, device := range point.Devices {
				if device.Component != assertion.Component {
					continue
				}
				latestTime = point.TimeS
				value = math.Max(math.Abs(device.CurrentA), device.CurrentMagnitudeA)
				found = true
			}
		}
		if !found {
			return 0, advancedAssertionDiagnostic(assertion, "final device-current assertion did not resolve to a solved device waveform")
		}
		return normalizedMNAFloat(value), nil
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
	case QuantityOscillationFrequencyHz, QuantityDutyCyclePct:
		times, values, diagnostic := waveform(result, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		frequency, dutyCycle, diagnostic := periodicWaveformMetrics(times, values, assertion)
		if diagnostic != nil {
			return 0, diagnostic
		}
		if assertion.Quantity == QuantityOscillationFrequencyHz {
			return frequency, nil
		}
		return dutyCycle, nil
	case QuantityOutputRippleVPP:
		for _, periodic := range result.PeriodicNodes {
			if periodic.Node == assertion.Node && periodic.Method != "" &&
				periodic.VoltageRippleVPP >= 0 && finite(periodic.VoltageRippleVPP) {
				return normalizedMNAFloat(periodic.VoltageRippleVPP), nil
			}
		}
		points := periodicSteadyStatePoints(result)
		steadyState := result
		steadyState.Points = points
		_, values, diagnostic := waveform(steadyState, assertion)
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
		if assertion.ResponseDirection == "falling" || (assertion.ResponseDirection == "" && values[len(values)-1] < values[0]) {
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
		if len(points) == len(result.Points) && len(result.PeriodicNodes) > 0 {
			var stable bool
			points, stable = averagedSteadyStatePowerPoints(result, assertion.Component, assertion.Components)
			if !stable {
				return 0, advancedAssertionDiagnostic(assertion, "conversion-efficiency assertion did not reach a stable trusted averaged-model power window")
			}
		} else if len(points) == len(result.Points) && len(points) > 1 {
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

// averagedSteadyStatePowerPoints excludes event-driven startup energy from a
// steady conversion-efficiency assertion only when the transient result
// carries trusted averaged periodic-node evidence. The final tenth of the
// solved waveform (and at least five samples) must already be stable for the
// load and every declared supply source; otherwise the measurement fails
// closed instead of treating an unfinished ramp as steady conversion.
func averagedSteadyStatePowerPoints(result AnalysisResult, load string, sources []string) ([]AnalysisPoint, bool) {
	if len(result.PeriodicNodes) == 0 || len(result.Points) < 5 {
		return nil, false
	}
	minimum := len(result.Points) / 10
	if minimum < 5 {
		minimum = 5
	}
	start := len(result.Points) - minimum
	reference, found := pointComponentPowers(result.Points[len(result.Points)-1], load, sources)
	if !found || reference[load] <= 0 {
		return nil, false
	}
	stable := func(point AnalysisPoint) bool {
		powers, complete := pointComponentPowers(point, load, sources)
		if !complete {
			return false
		}
		for component, expected := range reference {
			actual := powers[component]
			tolerance := math.Max(1e-12, .01*math.Max(math.Abs(expected), math.Abs(actual)))
			if math.Abs(actual-expected) > tolerance {
				return false
			}
		}
		return true
	}
	for index := start; index < len(result.Points); index++ {
		if !stable(result.Points[index]) {
			return nil, false
		}
	}
	for start > 0 && stable(result.Points[start-1]) {
		start--
	}
	return result.Points[start:], true
}

func pointComponentPowers(point AnalysisPoint, load string, sources []string) (map[string]float64, bool) {
	required := make(map[string]bool, len(sources)+1)
	required[load] = true
	for _, source := range sources {
		required[source] = true
	}
	powers := make(map[string]float64, len(required))
	for _, device := range point.Devices {
		if required[device.Component] {
			powers[device.Component] = math.Abs(device.VoltageV * device.CurrentA)
		}
	}
	return powers, len(powers) == len(required)
}

func periodicWaveformMetrics(times, values []float64, assertion Assertion) (float64, float64, *Diagnostic) {
	if len(times) != len(values) || len(values) < 3 {
		return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform assertion requires at least three ordered samples")
	}
	rawMinimum, rawMaximum := values[0], values[0]
	for index, value := range values {
		if !finite(value) || !finite(times[index]) || (index > 0 && times[index] <= times[index-1]) {
			return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform assertion requires finite samples with strictly increasing time")
		}
		rawMinimum, rawMaximum = math.Min(rawMinimum, value), math.Max(rawMaximum, value)
	}
	rawSpan := rawMaximum - rawMinimum
	if !finite(rawSpan) || rawSpan <= math.Max(1e-12, math.Max(math.Abs(rawMinimum), math.Abs(rawMaximum))*1e-9) {
		return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform assertion requires a nonconstant solved waveform")
	}
	minimum, maximum, levelsOK := periodicSteadyLevels(values)
	span := maximum - minimum
	if !levelsOK || !finite(span) || span <= math.Max(1e-12, math.Max(math.Abs(minimum), math.Abs(maximum))*1e-9) {
		return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform assertion requires sustained nonconstant solved waveform levels")
	}
	threshold := minimum + .5*span
	lowerThreshold := minimum + .25*span
	upperThreshold := minimum + .75*span
	edges := periodicHystereticRisingEdges(times, values, lowerThreshold, upperThreshold)
	if len(edges) < 3 {
		return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform assertion requires at least two complete measured periods")
	}
	const maximumMeasuredPeriods = 8
	if len(edges) > maximumMeasuredPeriods+1 {
		edges = edges[len(edges)-(maximumMeasuredPeriods+1):]
	}
	periodSum := 0.0
	periodMinimum, periodMaximum := math.Inf(1), 0.0
	for index := 1; index < len(edges); index++ {
		period := edges[index] - edges[index-1]
		if !finite(period) || period <= 0 {
			return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform edge ordering is invalid")
		}
		periodSum += period
		periodMinimum = math.Min(periodMinimum, period)
		periodMaximum = math.Max(periodMaximum, period)
	}
	averagePeriod := periodSum / float64(len(edges)-1)
	// A last-window period spread above ten percent is treated as unsettled or
	// non-periodic. This keeps startup bursts and relaxation drift from being
	// mislabeled as a stable oscillator solely because they contain crossings.
	if periodMaximum-periodMinimum > .1*averagePeriod {
		return 0, 0, advancedAssertionDiagnostic(assertion, fmt.Sprintf(
			"solved waveform does not establish a stable periodic interval within the trusted 10%% spread bound (minimum %.12g s, maximum %.12g s, average %.12g s, threshold %.12g)",
			periodMinimum,
			periodMaximum,
			averagePeriod,
			threshold,
		))
	}
	windowStart, windowEnd := edges[0], edges[len(edges)-1]
	highTime := waveformHystereticHighTime(times, values, lowerThreshold, upperThreshold, windowStart, windowEnd)
	if !finite(highTime) || highTime < 0 || highTime > windowEnd-windowStart {
		return 0, 0, advancedAssertionDiagnostic(assertion, "periodic waveform high-time integration is invalid")
	}
	return normalizedMNAFloat(1 / averagePeriod), normalizedMNAFloat(100 * highTime / (windowEnd - windowStart)), nil
}

func periodicHystereticRisingEdges(times, values []float64, lowerThreshold, upperThreshold float64) []float64 {
	edges := make([]float64, 0, 10)
	high := values[0] >= upperThreshold
	for index := 1; index < len(values); index++ {
		switch {
		case !high && crosses(values[index-1], values[index], upperThreshold, true):
			edges = append(edges, interpolateCrossing(times[index-1], times[index], values[index-1], values[index], upperThreshold))
			high = true
		case high && crosses(values[index-1], values[index], lowerThreshold, false):
			high = false
		}
	}
	return edges
}

func waveformHystereticHighTime(times, values []float64, lowerThreshold, upperThreshold, start, end float64) float64 {
	highTime := 0.0
	high := values[0] >= upperThreshold
	addHigh := func(left, right float64) {
		left, right = math.Max(left, start), math.Min(right, end)
		if right > left {
			highTime += right - left
		}
	}
	for index := 1; index < len(values); index++ {
		leftTime, rightTime := times[index-1], times[index]
		if rightTime <= start || leftTime >= end {
			continue
		}
		switch {
		case !high && crosses(values[index-1], values[index], upperThreshold, true):
			crossing := interpolateCrossing(leftTime, rightTime, values[index-1], values[index], upperThreshold)
			addHigh(crossing, rightTime)
			high = true
		case high && crosses(values[index-1], values[index], lowerThreshold, false):
			crossing := interpolateCrossing(leftTime, rightTime, values[index-1], values[index], lowerThreshold)
			addHigh(leftTime, crossing)
			high = false
		case high:
			addHigh(leftTime, rightTime)
		}
	}
	return highTime
}

func periodicSteadyLevels(values []float64) (float64, float64, bool) {
	if len(values) < 3 {
		return 0, 0, false
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	minimumGroup := len(ordered) / 100
	if minimumGroup < 1 {
		minimumGroup = 1
	}
	bestSplit, bestScore := 0, 0.0
	for split := minimumGroup; split <= len(ordered)-minimumGroup; split++ {
		gap := ordered[split] - ordered[split-1]
		if gap <= 0 {
			continue
		}
		// Weight the sorted-value separation by the geometric mean of the two
		// populations. Stable switching plateaus therefore outrank brief clamp
		// excursions, while a smooth periodic waveform selects a central split.
		score := gap * math.Sqrt(float64(split)*float64(len(ordered)-split))
		if score > bestScore {
			bestSplit, bestScore = split, score
		}
	}
	if bestSplit == 0 {
		return 0, 0, false
	}
	median := func(samples []float64) float64 {
		middle := len(samples) / 2
		if len(samples)%2 != 0 {
			return samples[middle]
		}
		return .5 * (samples[middle-1] + samples[middle])
	}
	return median(ordered[:bestSplit]), median(ordered[bestSplit:]), true
}

func transientResponseLatency(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	var baseline float64
	baselineFound := false
	var preEventTimes, preEventValues, times, deltas []float64
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
		deltas = append(deltas, value-expected)
		scale = math.Max(scale, math.Max(math.Abs(value), math.Abs(expected)))
	}
	if !baselineFound || len(deltas) < 2 {
		return 0, advancedAssertionDiagnostic(assertion, "event-response assertion requires a baseline and at least two solved samples inside its bounded window")
	}
	peakAbsolute := 0.0
	for _, delta := range deltas {
		peakAbsolute = math.Max(peakAbsolute, math.Abs(delta))
	}
	tolerance := 1e-12 * scale
	if peakAbsolute <= tolerance {
		return 0, advancedAssertionDiagnostic(assertion, fmt.Sprintf(
			"event-response assertion requires a nonconstant solved waveform (baseline=%.12g peak_delta=%.12g scale=%.12g samples=%d)",
			baseline, peakAbsolute, scale, len(deltas),
		))
	}
	// Measure onset in the contract direction when one is declared. Legacy
	// assertions infer the terminal direction for backward compatibility.
	// Scanning backward preserves bounded pulse responses that recover before
	// the window closes, while rejecting an earlier opposite-direction glitch
	// (for example an unpowered load pulling a sequenced rail below ground).
	direction := 0.0
	switch assertion.ResponseDirection {
	case "rising":
		direction = 1
	case "falling":
		direction = -1
	default:
		for index := len(deltas) - 1; index >= 0; index-- {
			if math.Abs(deltas[index]) <= tolerance {
				continue
			}
			direction = math.Copysign(1, deltas[index])
			break
		}
	}
	if direction == 0 {
		return 0, advancedAssertionDiagnostic(assertion, "trusted event window does not contain a directed response")
	}
	peak := 0.0
	for _, delta := range deltas {
		peak = math.Max(peak, math.Max(0, direction*delta))
	}
	if peak <= tolerance {
		return 0, advancedAssertionDiagnostic(assertion, "trusted event window does not contain a response in its required direction")
	}
	threshold := TransientResponseOnsetFraction * peak
	previousTime, previousResidual := assertion.WindowStartS, 0.0
	for index, delta := range deltas {
		residual := math.Max(0, direction*delta)
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
	switch assertion.Quantity {
	case QuantityFallingThresholdVoltageV:
		return dcSweepTransition(result, assertion, dcSweepReverse)
	case QuantityLowerThresholdVoltageV, QuantityUpperThresholdVoltageV:
		transitions, diagnostic := dcSweepTransitions(result, assertion, dcSweepForward)
		if diagnostic != nil {
			return 0, diagnostic
		}
		if len(transitions) < 2 {
			return 0, advancedAssertionDiagnostic(assertion, "ordered threshold assertion requires at least two unambiguous decision transitions")
		}
		if assertion.Quantity == QuantityLowerThresholdVoltageV {
			return normalizedMNAFloat(transitions[0]), nil
		}
		return normalizedMNAFloat(transitions[len(transitions)-1]), nil
	default:
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
}

func dcSweepTransition(result AnalysisResult, assertion Assertion, direction string) (float64, *Diagnostic) {
	transitions, diagnostic := dcSweepTransitions(result, assertion, direction)
	if diagnostic != nil {
		return 0, diagnostic
	}
	if len(transitions) != 1 {
		return 0, advancedAssertionDiagnostic(assertion, "DC sweep must contain exactly one unambiguous decision transition in each required direction")
	}
	return normalizedMNAFloat(transitions[0]), nil
}

func dcSweepTransitions(result AnalysisResult, assertion Assertion, direction string) ([]float64, *Diagnostic) {
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
			return []float64{censoredDCSweepAssertionValue(minimumSweep, maximumSweep, assertion)}, nil
		}
		return nil, advancedAssertionDiagnostic(assertion, fmt.Sprintf("DC sweep output does not contain enough finite decision samples (samples=%d, output_min=%.12g, output_max=%.12g)", len(samples), minimum, maximum))
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
	return transitions, nil
}

func dcSweepSpanOrSlope(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	minimumSweep, maximumSweep := math.Inf(1), math.Inf(-1)
	minimumValue, maximumValue := 0.0, 0.0
	minimumObserved, maximumObserved := math.Inf(1), math.Inf(-1)
	found := false
	for _, point := range result.Points {
		if point.Sweep != "" && point.Sweep != dcSweepForward {
			continue
		}
		value, valueFound := 0.0, false
		switch assertion.Quantity {
		case QuantityDCSweepVoltageSpanV, QuantityDCSweepVoltageSlopeVPerV:
			value, valueFound = analysisNodeReal(point, assertion.Node)
		case QuantityDCSweepDeviceSlopeAperV:
			for _, device := range point.Devices {
				if device.Component == assertion.Component {
					value, valueFound = device.CurrentMagnitudeA, true
					break
				}
			}
		}
		if !valueFound {
			continue
		}
		minimumObserved = math.Min(minimumObserved, value)
		maximumObserved = math.Max(maximumObserved, value)
		if !found || point.SweepValue < minimumSweep {
			minimumSweep, minimumValue = point.SweepValue, value
		}
		if !found || point.SweepValue > maximumSweep {
			maximumSweep, maximumValue = point.SweepValue, value
		}
		found = true
	}
	if !found || !finite(minimumSweep) || !finite(maximumSweep) || maximumSweep-minimumSweep <= 1e-15 {
		return 0, advancedAssertionDiagnostic(assertion, "DC sweep span/slope assertion requires two finite solved sweep endpoints")
	}
	if assertion.Quantity == QuantityDCSweepVoltageSpanV {
		return normalizedMNAFloat(maximumObserved - minimumObserved), nil
	}
	slope := math.Abs(maximumValue-minimumValue) / (maximumSweep - minimumSweep)
	return normalizedMNAFloat(slope), nil
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
