package simmodel

import (
	"fmt"
	"math"
)

type transientSOAExcursion struct {
	durationS float64
	active    bool
}

// solveElectrothermalAnalysis couples the deterministic electrical transient
// trajectory to reviewed Foster thermal networks and transient SOA envelopes.
// The currently registered electrical models do not declare temperature
// feedback coefficients, so coupling is intentionally one-way; adding such a
// coefficient requires a separately reviewed primitive and bounded iteration.
func solveElectrothermalAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result, diagnostics := solveTransientAnalysis(plan, analysis)
	if len(diagnostics) != 0 {
		return AnalysisResult{ID: analysis.ID, Kind: AnalysisElectrothermal}, diagnostics
	}
	result.ID = analysis.ID
	result.Kind = AnalysisElectrothermal

	conditions := namedValueMap(analysis.Conditions)
	ambient := conditions["ambient_temperature_c"]
	baseResistanceScale := 1.0
	if value, exists := conditions["thermal_resistance_scale"]; exists {
		baseResistanceScale = value
	}
	devices := make(map[string]ResolvedDevice, len(plan.Devices))
	thermalStates := map[string][]float64{}
	steadyThermalEvidence := map[string]bool{}
	soaExcursions := map[string]transientSOAExcursion{}
	temperatureAssertionMaximums := electrothermalTemperatureAssertionMaximums(plan, analysis.ID)
	temperatureAssertionPreservesCatalogLimits := map[string]bool{}
	maximumTemperatureByComponent := map[string]float64{}
	soaAssertionComponents := electrothermalSOAAssertionCoverage(plan, analysis.ID)
	dynamicDevices := 0
	for _, device := range plan.Devices {
		devices[device.Component] = device
		parameters := deviceParameterMap(device)
		maximum, hasMaximum := namedValue(parameters, "max_temperature_c")
		if hasMaximum {
			maximumTemperatureByComponent[device.Component] = maximum
		}
		if assertionMaximum, asserted := temperatureAssertionMaximums[device.Component]; asserted && hasMaximum && assertionMaximum <= maximum {
			temperatureAssertionPreservesCatalogLimits[device.Component] = true
		}
		if device.ThermalModel != nil {
			thermalStates[device.Component] = make([]float64, len(device.ThermalModel.Stages))
			dynamicDevices++
		} else if thermalDeviceSupportsDissipation(device) {
			_, _, hasPath := resolvedThermalPath(nil, parameters, analysis.Conditions, ambient)
			if hasMaximum || hasPath {
				steadyThermalEvidence[device.Component] = true
				dynamicDevices++
			}
		}
		if len(device.TransientSOA) != 0 && device.ThermalModel == nil {
			dynamicDevices++
		}
	}
	if dynamicDevices == 0 {
		return result, []Diagnostic{{
			Path:       "analyses." + analysis.ID + ".devices",
			Message:    "electrothermal analysis resolved no reviewed thermal RC or transient SOA evidence",
			Suggestion: "select a catalog component with reviewed dynamic thermal and stress evidence",
		}}
	}

	previousTime := 0.0
	for pointIndex := range result.Points {
		point := &result.Points[pointIndex]
		resistanceScale := electrothermalResistanceScaleAt(analysis, point.TimeS, baseResistanceScale)
		timeStep := point.TimeS - previousTime
		if pointIndex == 0 {
			timeStep = 0
		}
		for deviceIndex := range point.Devices {
			observation := &point.Devices[deviceIndex]
			device, exists := devices[observation.Component]
			if !exists {
				continue
			}
			if device.ThermalModel != nil {
				boundary, diagnostic := dynamicThermalBoundary(device, analysis, ambient)
				if diagnostic != nil {
					return result, []Diagnostic{*diagnostic}
				}
				states := thermalStates[device.Component]
				for stageIndex, stage := range device.ThermalModel.Stages {
					stageScale := resistanceScale
					if stage.CoolingCoupling == "fixed" {
						stageScale = 1
					}
					resistance := stage.ThermalResistanceCPerW * stageScale
					timeConstant := resistance * stage.ThermalCapacitanceJPerC
					decay := 1.0
					if timeStep > 0 {
						decay = math.Exp(-timeStep / timeConstant)
					}
					states[stageIndex] = states[stageIndex]*decay + math.Max(0, observation.DissipationW)*resistance*(1-decay)
				}
				temperature := boundary
				for _, rise := range states {
					temperature += rise
				}
				temperature = normalizedMNAFloat(temperature)
				observation.JunctionTemperatureC = &temperature
				maximum, hasMaximum := maximumTemperatureByComponent[device.Component]
				if !hasMaximum {
					return result, []Diagnostic{{
						Path:       fmt.Sprintf("analyses.%s.devices.%s", analysis.ID, device.Component),
						Message:    "dynamic thermal RC evidence requires a catalog-backed maximum junction temperature",
						Suggestion: "select a complete reviewed electrothermal model",
					}}
				}
				if temperature > maximum && !temperatureAssertionPreservesCatalogLimits[device.Component] {
					return result, []Diagnostic{{
						Path:       fmt.Sprintf("analyses.%s.points[%d].devices.%s.junction_temperature_c", analysis.ID, pointIndex, device.Component),
						Message:    fmt.Sprintf("predicted dynamic junction temperature %.12g C exceeds catalog-backed maximum %.12g C at %.12g s", temperature, maximum, point.TimeS),
						Suggestion: "reduce dissipation, improve the reviewed thermal path, or select a suitably rated component",
					}}
				}
			}
			if len(device.TransientSOA) != 0 {
				boundaryTemperature, diagnostic := dynamicSOABoundaryTemperature(device, analysis, ambient)
				if diagnostic != nil {
					return result, []Diagnostic{*diagnostic}
				}
				margin, excursion, diagnostic := transientSOAObservationMargin(
					device,
					soaExcursions[device.Component],
					timeStep,
					boundaryTemperature,
					math.Abs(observation.VoltageV),
					math.Max(math.Abs(observation.CurrentA), observation.CurrentMagnitudeA),
				)
				if diagnostic != nil {
					diagnostic.Path = fmt.Sprintf("analyses.%s.points[%d].devices.%s.transient_soa", analysis.ID, pointIndex, device.Component)
					return result, []Diagnostic{*diagnostic}
				}
				soaExcursions[device.Component] = excursion
				observation.TransientSOAMargin = normalizedMNAFloat(margin)
				observation.TransientSOAEvaluated = true
				if margin < 1 && !soaAssertionComponents[device.Component] {
					return result, []Diagnostic{{
						Path:       fmt.Sprintf("analyses.%s.points[%d].devices.%s.transient_soa", analysis.ID, pointIndex, device.Component),
						Message:    fmt.Sprintf("transient SOA margin %.12g is below unity at %.12g s", margin, point.TimeS),
						Suggestion: "reduce voltage/current stress, shorten the event, or select a reviewed wider-SOA device",
					}}
				}
			}
		}
		previousTime = point.TimeS
	}
	if diagnostic := applyPeriodicSteadyThermalEvidence(
		&result, devices, steadyThermalEvidence, analysis, ambient, baseResistanceScale,
		temperatureAssertionPreservesCatalogLimits,
	); diagnostic != nil {
		return result, []Diagnostic{*diagnostic}
	}
	return result, nil
}

func electrothermalTemperatureAssertionMaximums(plan Plan, analysisID string) map[string]float64 {
	result := map[string]float64{}
	add := func(component string, maximum float64) {
		if component == "" || !finite(maximum) {
			return
		}
		if current, exists := result[component]; !exists || maximum < current {
			result[component] = maximum
		}
	}
	for _, assertion := range plan.Assertions {
		if assertion.AnalysisID != analysisID {
			continue
		}
		switch assertion.Quantity {
		case QuantityJunctionTemperatureC:
			add(assertion.Component, assertion.Max)
		case QuantityMaximumJunctionTemperatureC:
			for _, component := range assertion.Components {
				add(component, assertion.Max)
			}
		}
	}
	return result
}

func electrothermalSOAAssertionCoverage(plan Plan, analysisID string) map[string]bool {
	result := map[string]bool{}
	for _, assertion := range plan.Assertions {
		if assertion.AnalysisID != analysisID || assertion.Min < 1 {
			continue
		}
		switch assertion.Quantity {
		case QuantityTransientSOAMargin:
			if assertion.Component != "" {
				result[assertion.Component] = true
			}
		case QuantityMinimumTransientSOAMargin:
			for _, component := range assertion.Components {
				result[component] = true
			}
		}
	}
	return result
}

func electrothermalResistanceScaleAt(analysis Analysis, timeS, base float64) float64 {
	result := base
	for _, event := range analysis.ConditionValueEvents {
		if event.Name == "thermal_resistance_scale" {
			result, _ = transientConditionEventValue(event, timeS, analysis.TimeStepS)
		}
	}
	return result
}

func applyPeriodicSteadyThermalEvidence(
	result *AnalysisResult,
	devices map[string]ResolvedDevice,
	steadyThermalEvidence map[string]bool,
	analysis Analysis,
	ambient float64,
	baseResistanceScale float64,
	temperatureAssertionPreservesCatalogLimits map[string]bool,
) *Diagnostic {
	if len(steadyThermalEvidence) == 0 {
		return nil
	}
	frequency := result.FundamentalFrequencyHz
	if len(result.Points) < 2 {
		return &Diagnostic{Path: "analyses." + analysis.ID, Message: "electrothermal analysis produced fewer than two transient observations"}
	}
	applyTemperatures := func(temperatures map[string]*float64) {
		for pointIndex := range result.Points {
			for deviceIndex := range result.Points[pointIndex].Devices {
				observation := &result.Points[pointIndex].Devices[deviceIndex]
				if temperature, found := temperatures[observation.Component]; found {
					observation.JunctionTemperatureC = temperature
				}
			}
		}
	}
	if frequency <= 0 {
		// A passive thermal RC trajectory cannot exceed the steady-state rise
		// produced by its maximum applied dissipation. When reviewed evidence
		// supplies resistance but no capacitance, use that peak-power steady
		// state as a conservative upper bound instead of inventing time-domain
		// storage or discarding the component from the temperature assertion.
		maximumScaledDissipation := map[string]float64{}
		for _, point := range result.Points {
			scale := electrothermalResistanceScaleAt(analysis, point.TimeS, baseResistanceScale)
			for _, observation := range point.Devices {
				if steadyThermalEvidence[observation.Component] {
					maximumScaledDissipation[observation.Component] = math.Max(
						maximumScaledDissipation[observation.Component],
						math.Max(0, observation.DissipationW)*scale,
					)
				}
			}
		}
		temperatures := map[string]*float64{}
		for component := range steadyThermalEvidence {
			steady, diagnostic := thermalDeviceResultAtResistanceScaleWithRating(
				devices[component], analysis, ambient, maximumScaledDissipation[component], 1,
				temperatureAssertionPreservesCatalogLimits[component],
			)
			if diagnostic != nil {
				return diagnostic
			}
			temperatures[component] = steady.JunctionTemperatureC
		}
		applyTemperatures(temperatures)
		return nil
	}
	finalTime := result.Points[len(result.Points)-1].TimeS
	cycles := math.Min(2, math.Floor(finalTime*frequency+1e-9))
	if cycles < 1 {
		return &Diagnostic{Path: "analyses." + analysis.ID, Message: "electrothermal analysis produced no complete periodic cycle for steady thermal averaging"}
	}
	windowStart := finalTime - cycles/frequency
	weightedEnergy := map[string]float64{}
	dissipationForComponent := func(observations []DeviceResult, component string) float64 {
		dissipation := 0.0
		for _, observation := range observations {
			if observation.Component == component {
				dissipation = observation.DissipationW
			}
		}
		return dissipation
	}
	for index := 1; index < len(result.Points); index++ {
		left, right := result.Points[index-1], result.Points[index]
		if right.TimeS <= windowStart {
			continue
		}
		leftTime := math.Max(left.TimeS, windowStart)
		fraction := 0.0
		if right.TimeS > left.TimeS {
			fraction = (leftTime - left.TimeS) / (right.TimeS - left.TimeS)
		}
		leftScale := electrothermalResistanceScaleAt(analysis, left.TimeS, baseResistanceScale)
		rightScale := electrothermalResistanceScaleAt(analysis, right.TimeS, baseResistanceScale)
		startScale := leftScale + fraction*(rightScale-leftScale)
		for component := range steadyThermalEvidence {
			leftDissipation := dissipationForComponent(left.Devices, component)
			rightDissipation := dissipationForComponent(right.Devices, component)
			startPower := leftDissipation + fraction*(rightDissipation-leftDissipation)
			weightedEnergy[component] += .5 * (startPower*startScale + rightDissipation*rightScale) * (right.TimeS - leftTime)
		}
	}
	windowDuration := finalTime - windowStart
	temperatures := map[string]*float64{}
	for component := range steadyThermalEvidence {
		device := devices[component]
		averageScaledDissipation := weightedEnergy[component] / windowDuration
		steady, diagnostic := thermalDeviceResultAtResistanceScaleWithRating(
			device, analysis, ambient, averageScaledDissipation, 1,
			temperatureAssertionPreservesCatalogLimits[component],
		)
		if diagnostic != nil {
			return diagnostic
		}
		temperatures[component] = steady.JunctionTemperatureC
	}
	applyTemperatures(temperatures)
	return nil
}

func dynamicThermalBoundary(device ResolvedDevice, analysis Analysis, ambient float64) (float64, *Diagnostic) {
	switch device.ThermalModel.Reference {
	case "junction_to_ambient":
		return ambient, nil
	case "junction_to_case":
		if value, exists := namedValue(namedValueMap(analysis.Conditions), "case_temperature_c"); exists {
			return value, nil
		}
		return 0, &Diagnostic{
			Path:       "analyses." + analysis.ID + ".conditions.case_temperature_c",
			Message:    "junction-to-case dynamic thermal model requires an explicit case boundary temperature",
			Suggestion: "declare the reviewed case or heatsink boundary for this operating event",
		}
	default:
		return 0, &Diagnostic{Path: "analyses." + analysis.ID + ".devices." + device.Component + ".thermal_model.reference", Message: "dynamic thermal model has an unsupported boundary reference"}
	}
}

func dynamicSOABoundaryTemperature(device ResolvedDevice, analysis Analysis, ambient float64) (float64, *Diagnostic) {
	if device.ThermalModel == nil || device.ThermalModel.Reference == "junction_to_ambient" {
		return ambient, nil
	}
	return dynamicThermalBoundary(device, analysis, ambient)
}

// transientSOAObservationMargin applies a finite pulse envelope only while a
// device is outside its reviewed DC boundary. Returning to the DC-safe region
// resets the excursion clock. A stressed operating point at the initial
// observation is pre-existing by definition and therefore must satisfy a DC
// envelope rather than receiving a fresh pulse allowance.
func transientSOAObservationMargin(device ResolvedDevice, prior transientSOAExcursion, timeStep, boundaryTemperature, voltage, current float64) (float64, transientSOAExcursion, *Diagnostic) {
	if current <= 1e-18 {
		return maxMNASolutionValue, transientSOAExcursion{}, nil
	}
	dcMargin, dcDiagnostic := transientSOADCMargin(device, boundaryTemperature, voltage, current)
	if dcDiagnostic == nil && dcMargin >= 1 {
		return dcMargin, transientSOAExcursion{}, nil
	}
	if timeStep <= 0 {
		return dcMargin, transientSOAExcursion{active: true}, dcDiagnostic
	}
	// A newly observed excursion starts at this sample, so no stressed time
	// has elapsed yet. Subsequent stressed observations accumulate the time
	// between samples. This keeps a pulse whose endpoints align to the
	// transient grid within its reviewed duration instead of charging an
	// extra inclusive timestep.
	excursion := transientSOAExcursion{active: true}
	if prior.active {
		excursion.durationS = prior.durationS + timeStep
	}
	margin, diagnostic := transientSOAMargin(device, excursion.durationS, boundaryTemperature, voltage, current)
	return margin, excursion, diagnostic
}

func transientSOADCMargin(device ResolvedDevice, boundaryTemperature, voltage, current float64) (float64, *Diagnostic) {
	envelope, found := selectTransientSOADCEnvelope(device.TransientSOA)
	if !found {
		return 0, &Diagnostic{Message: "device has no reviewed DC SOA envelope", Suggestion: "select a device with a reviewed DC safe-operating-area envelope"}
	}
	return transientSOAMarginForEnvelope(device, envelope, boundaryTemperature, voltage, current)
}

func transientSOAMargin(device ResolvedDevice, elapsed, boundaryTemperature, voltage, current float64) (float64, *Diagnostic) {
	if current <= 1e-18 {
		return maxMNASolutionValue, nil
	}
	envelope, found := selectTransientSOAEnvelope(device.TransientSOA, elapsed)
	if !found {
		return 0, &Diagnostic{Message: "event duration is outside the reviewed transient SOA envelopes", Suggestion: "select a device with a reviewed envelope covering the complete event"}
	}
	return transientSOAMarginForEnvelope(device, envelope, boundaryTemperature, voltage, current)
}

func transientSOAMarginForEnvelope(device ResolvedDevice, envelope TransientSOAEnvelope, boundaryTemperature, voltage, current float64) (float64, *Diagnostic) {
	allowed, found := interpolateSOACurrent(envelope.Points, voltage)
	if !found {
		return 0, &Diagnostic{Message: fmt.Sprintf("device voltage %.12g V exceeds the reviewed transient SOA voltage boundary", voltage)}
	}
	maximum, hasMaximum := namedValue(deviceParameterMap(device), "max_temperature_c")
	if !hasMaximum {
		return 0, &Diagnostic{Message: "transient SOA evidence requires a catalog-backed maximum junction temperature"}
	}
	if boundaryTemperature > envelope.CaseTemperatureC {
		denominator := maximum - envelope.CaseTemperatureC
		if denominator <= 0 || boundaryTemperature >= maximum {
			return 0, &Diagnostic{Message: "event thermal boundary leaves no reviewed transient SOA derating range"}
		}
		allowed *= (maximum - boundaryTemperature) / denominator
	}
	// A margin is a lower-bounded safety quantity. Saturating a value above the
	// solver's trusted reporting range preserves that proof without turning an
	// exceptionally light load into an artificial upper-bound failure.
	return math.Min(allowed/current, maxMNASolutionValue), nil
}

func selectTransientSOAEnvelope(envelopes []TransientSOAEnvelope, elapsed float64) (TransientSOAEnvelope, bool) {
	for _, envelope := range envelopes {
		if envelope.PulseDurationS != nil && *envelope.PulseDurationS >= elapsed-1e-15 {
			return envelope, true
		}
	}
	for _, envelope := range envelopes {
		if envelope.DC {
			return envelope, true
		}
	}
	return TransientSOAEnvelope{}, false
}

func selectTransientSOADCEnvelope(envelopes []TransientSOAEnvelope) (TransientSOAEnvelope, bool) {
	for index := len(envelopes) - 1; index >= 0; index-- {
		if envelopes[index].DC {
			return envelopes[index], true
		}
	}
	return TransientSOAEnvelope{}, false
}

func interpolateSOACurrent(points []TransientSOAPoint, voltage float64) (float64, bool) {
	if len(points) < 2 || voltage > points[len(points)-1].VoltageV {
		return 0, false
	}
	if voltage <= points[0].VoltageV {
		return points[0].CurrentA, true
	}
	for index := 1; index < len(points); index++ {
		if voltage > points[index].VoltageV {
			continue
		}
		left, right := points[index-1], points[index]
		fraction := (math.Log(voltage) - math.Log(left.VoltageV)) / (math.Log(right.VoltageV) - math.Log(left.VoltageV))
		current := math.Exp(math.Log(left.CurrentA) + fraction*(math.Log(right.CurrentA)-math.Log(left.CurrentA)))
		return current, true
	}
	return 0, false
}
