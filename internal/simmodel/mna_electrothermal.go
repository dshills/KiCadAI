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
	soaExcursions := map[string]transientSOAExcursion{}
	dynamicDevices := 0
	for _, device := range plan.Devices {
		devices[device.Component] = device
		if device.ThermalModel != nil {
			thermalStates[device.Component] = make([]float64, len(device.ThermalModel.Stages))
			dynamicDevices++
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
		resistanceScale := baseResistanceScale
		for _, event := range analysis.ConditionValueEvents {
			if event.Name != "thermal_resistance_scale" {
				continue
			}
			resistanceScale, _ = transientConditionEventValue(event, point.TimeS, analysis.TimeStepS)
		}
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
				maximum, hasMaximum := namedValue(deviceParameterMap(device), "max_temperature_c")
				if !hasMaximum {
					return result, []Diagnostic{{
						Path:       fmt.Sprintf("analyses.%s.devices.%s", analysis.ID, device.Component),
						Message:    "dynamic thermal RC evidence requires a catalog-backed maximum junction temperature",
						Suggestion: "select a complete reviewed electrothermal model",
					}}
				}
				if temperature > maximum {
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
				if margin < 1 {
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
	return result, nil
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
	if dcEnvelope, found := selectTransientSOADCEnvelope(device.TransientSOA); found {
		if _, covered := interpolateSOACurrent(dcEnvelope.Points, voltage); covered {
			dcMargin, diagnostic := transientSOAMarginForEnvelope(device, dcEnvelope, boundaryTemperature, voltage, current)
			if diagnostic != nil {
				return 0, prior, diagnostic
			}
			if dcMargin >= 1 {
				return dcMargin, transientSOAExcursion{}, nil
			}
		}
	}
	if timeStep <= 0 {
		margin, diagnostic := transientSOAMargin(device, 0, boundaryTemperature, voltage, current)
		return margin, transientSOAExcursion{active: true}, diagnostic
	}
	excursion := transientSOAExcursion{durationS: timeStep, active: true}
	if prior.active {
		excursion.durationS = prior.durationS + timeStep
	}
	margin, diagnostic := transientSOAMargin(device, excursion.durationS, boundaryTemperature, voltage, current)
	return margin, excursion, diagnostic
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
	return allowed / current, nil
}

func selectTransientSOAEnvelope(envelopes []TransientSOAEnvelope, elapsed float64) (TransientSOAEnvelope, bool) {
	if elapsed <= 0 {
		return selectTransientSOADCEnvelope(envelopes)
	}
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
