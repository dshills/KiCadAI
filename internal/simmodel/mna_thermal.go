package simmodel

import (
	"fmt"
	"math"
)

// solveThermalAnalysis couples either a trusted DC operating point or the
// average dissipation over the final complete cycles of a bounded periodic
// drive to a steady-state catalog thermal-resistance model. It never invents a
// package or board thermal path: junction temperature remains unavailable and
// assertions fail closed unless the resolved component claim contains one.
func solveThermalAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisThermal}
	if analysis.DurationS > 0 || analysis.TimeStepS > 0 {
		return solvePeriodicallyDrivenThermalAnalysis(plan, analysis)
	}
	dc := analysis
	dc.Kind = AnalysisDCOperatingPoint
	dc.Conditions = nil
	model, _ := definitionByID(plan.ModelID)
	var system mnaSystem
	var solution []complex128
	if model.NonlinearDC {
		var evidence SolverEvidence
		var diagnostic *Diagnostic
		system, solution, evidence, diagnostic = solveNonlinearDC(plan, dc)
		if diagnostic != nil {
			diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
			return result, []Diagnostic{*diagnostic}
		}
		_ = evidence
		if diagnostics := validateNonlinearOperatingLimits(plan, system, solution); len(diagnostics) != 0 {
			return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
		}
	} else {
		var diagnostics []Diagnostic
		system, diagnostics = buildMNASystem(plan, dc, 0)
		if len(diagnostics) != 0 {
			return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
		}
		var diagnostic *Diagnostic
		solution, diagnostic = solveMNA(system)
		if diagnostic != nil {
			diagnostic.Path = "analyses." + analysis.ID + "." + diagnostic.Path
			return result, []Diagnostic{*diagnostic}
		}
		system, solution, diagnostics = solveBoundedOpAmpDC(plan, dc, system, solution)
		if len(diagnostics) != 0 {
			return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
		}
		if diagnostics = validateResolvedOperatingLimits(plan, system, solution, false); len(diagnostics) != 0 {
			return result, prefixAnalysisDiagnostics(analysis.ID, diagnostics)
		}
	}

	ambient := namedValueMap(analysis.Conditions)["ambient_temperature_c"]
	deviceResults := make([]DeviceResult, 0, len(plan.Devices))
	for _, device := range plan.Devices {
		dissipation, dissipative := thermalDeviceDissipation(device, system, solution)
		if !dissipative {
			continue
		}
		entry := DeviceResult{Component: device.Component, DissipationW: normalizedMNAFloat(dissipation)}
		parameters := namedValueMap(device.ModelParameters)
		maximum, hasMaximum := namedValue(parameters, "max_temperature_c")
		theta, reference, hasTheta := resolvedThermalPath(parameters, analysis.Conditions, ambient)
		if hasTheta != hasMaximum {
			return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "steady-state thermal evidence requires both thermal_resistance_c_per_w and max_temperature_c", Suggestion: "select a reviewed package model with a complete thermal path"}}
		}
		if hasTheta {
			if device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
				if _, hasQuiescent := namedValue(parameters, "quiescent_current_a"); !hasQuiescent {
					return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "op-amp thermal analysis requires catalog-backed quiescent current", Suggestion: "select a reviewed active-device model with supply-current evidence"}}
				}
			}
			temperature := normalizedMNAFloat(reference + dissipation*theta)
			entry.JunctionTemperatureC = &temperature
			if temperature > maximum {
				return result, []Diagnostic{{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: fmt.Sprintf("predicted steady-state temperature %.12g C exceeds catalog-backed maximum %.12g C", temperature, maximum), Suggestion: "reduce dissipation, improve the reviewed thermal path, or select a suitably rated component"}}
			}
		}
		deviceResults = append(deviceResults, entry)
	}
	result.Points = []AnalysisPoint{{Nodes: nodeResults(plan, system, solution), Devices: deviceResults}}
	return result, nil
}

func solvePeriodicallyDrivenThermalAnalysis(plan Plan, analysis Analysis) (AnalysisResult, []Diagnostic) {
	result := AnalysisResult{ID: analysis.ID, Kind: AnalysisThermal}
	waveform, diagnostics := solveTransientAnalysis(plan, analysis)
	if len(diagnostics) != 0 {
		return result, diagnostics
	}
	frequency := 0.0
	for _, excitation := range analysis.Excitations {
		if excitation.SineFrequencyHz > 0 {
			frequency = excitation.SineFrequencyHz
			break
		}
	}
	if frequency <= 0 || len(waveform.Points) < 2 {
		return result, []Diagnostic{{Path: "analyses." + analysis.ID, Message: "periodically driven thermal analysis produced no complete periodic waveform"}}
	}
	cycles := math.Min(2, math.Floor(analysis.DurationS*frequency+1e-9))
	windowStart := analysis.DurationS - cycles/frequency
	energyByComponent := map[string]float64{}
	for index := 1; index < len(waveform.Points); index++ {
		left, right := waveform.Points[index-1], waveform.Points[index]
		if left.TimeS < windowStart-1e-15 {
			continue
		}
		leftDissipation := deviceDissipationByComponent(left.Devices)
		for component, rightDissipation := range deviceDissipationByComponent(right.Devices) {
			energyByComponent[component] += .5 * (leftDissipation[component] + rightDissipation) * (right.TimeS - left.TimeS)
		}
	}
	windowDuration := analysis.DurationS - windowStart
	deviceResults := make([]DeviceResult, 0, len(plan.Devices))
	ambient := namedValueMap(analysis.Conditions)["ambient_temperature_c"]
	for _, device := range plan.Devices {
		if !thermalDeviceSupportsDissipation(device) {
			continue
		}
		dissipation := normalizedMNAFloat(energyByComponent[device.Component] / windowDuration)
		entry, diagnostic := thermalDeviceResult(device, analysis, ambient, dissipation)
		if diagnostic != nil {
			return result, []Diagnostic{*diagnostic}
		}
		deviceResults = append(deviceResults, entry)
	}
	result.Points = []AnalysisPoint{{Nodes: waveform.Points[len(waveform.Points)-1].Nodes, Devices: deviceResults}}
	return result, nil
}

func deviceDissipationByComponent(devices []DeviceResult) map[string]float64 {
	result := make(map[string]float64, len(devices))
	for _, device := range devices {
		result[device.Component] = device.DissipationW
	}
	return result
}

func thermalDeviceSupportsDissipation(device ResolvedDevice) bool {
	switch device.PrimitiveModel {
	case PrimitiveResistorV1, PrimitiveFuseClosedStateV1, PrimitiveDiodeShockleyV1, PrimitiveBidirectionalTVSV1,
		PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1, PrimitiveBJTNPNV1, PrimitiveBJTPNPV1,
		PrimitiveOpAmpV1, PrimitiveComparatorOpenCollectorV1,
		PrimitiveAdjustableLinearRegulatorV1, PrimitiveFixedLinearRegulatorV1, PrimitiveFloatingAdjustableRegulatorV1,
		PrimitiveBidirectionalOpenDrainTranslatorV1, PrimitiveMCUStaticSupplyLoadV1, PrimitiveSensorStaticSupplyLoadV1:
		return true
	default:
		return false
	}
}

func thermalDeviceResult(device ResolvedDevice, analysis Analysis, ambient, dissipation float64) (DeviceResult, *Diagnostic) {
	entry := DeviceResult{Component: device.Component, DissipationW: normalizedMNAFloat(dissipation)}
	parameters := namedValueMap(device.ModelParameters)
	maximum, hasMaximum := namedValue(parameters, "max_temperature_c")
	theta, reference, hasTheta := resolvedThermalPath(parameters, analysis.Conditions, ambient)
	if hasTheta != hasMaximum {
		return DeviceResult{}, &Diagnostic{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "steady-state thermal evidence requires both thermal_resistance_c_per_w and max_temperature_c", Suggestion: "select a reviewed package model with a complete thermal path"}
	}
	if !hasTheta {
		return entry, nil
	}
	if device.PrimitiveModel == PrimitiveOpAmpV1 || device.PrimitiveModel == PrimitiveComparatorOpenCollectorV1 {
		if _, hasQuiescent := namedValue(parameters, "quiescent_current_a"); !hasQuiescent {
			return DeviceResult{}, &Diagnostic{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: "op-amp thermal analysis requires catalog-backed quiescent current", Suggestion: "select a reviewed active-device model with supply-current evidence"}
		}
	}
	temperature := normalizedMNAFloat(reference + dissipation*theta)
	entry.JunctionTemperatureC = &temperature
	if temperature > maximum {
		return DeviceResult{}, &Diagnostic{Path: "analyses." + analysis.ID + ".devices." + device.Component, Message: fmt.Sprintf("predicted steady-state temperature %.12g C exceeds catalog-backed maximum %.12g C", temperature, maximum), Suggestion: "reduce dissipation, improve the reviewed thermal path, or select a suitably rated component"}
	}
	return entry, nil
}

func thermalDeviceDissipation(device ResolvedDevice, system mnaSystem, solution []complex128) (float64, bool) {
	if dissipation, ok := adjustableLinearRegulatorDissipation(device, system, solution); ok {
		return dissipation, true
	}
	if dissipation, ok := openDrainTranslatorDissipation(device, system, solution); ok {
		return dissipation, true
	}
	terminals := terminalMap(device)
	voltage := func(node string) float64 { return real(solvedNodeVoltage(system, solution, node)) }
	switch device.PrimitiveModel {
	case PrimitiveResistorV1:
		delta := voltage(terminals["A"]) - voltage(terminals["B"])
		return delta * delta / *device.ValueSI, true
	case PrimitiveFuseClosedStateV1:
		delta := voltage(terminals["A"]) - voltage(terminals["B"])
		return delta * delta / namedValueMap(device.ModelParameters)["cold_resistance_ohm"], true
	case PrimitiveDiodeShockleyV1:
		delta := voltage(terminals["ANODE"]) - voltage(terminals["CATHODE"])
		current, _ := diodeCurrentAndGradient(delta, namedValueMap(device.ModelParameters))
		return math.Abs(delta * current), true
	case PrimitiveBidirectionalTVSV1:
		delta := voltage(terminals["ANODE"]) - voltage(terminals["CATHODE"])
		current, _ := bidirectionalTVSCurrentAndGradient(delta, namedValueMap(device.ModelParameters))
		return math.Abs(delta * current), true
	case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
		delta := voltage(terminals["DRAIN"]) - voltage(terminals["SOURCE"])
		conductance := mosfetSwitchConductance(compiledNonlinearDevice{
			primitive: device.PrimitiveModel, terminals: terminals, parameters: namedValueMap(device.ModelParameters), polarity: mosfetPolarity(device.PrimitiveModel),
		}, &system, solution)
		return delta * delta * conductance, true
	case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
		polarity := 1.0
		if device.PrimitiveModel == PrimitiveBJTPNPV1 {
			polarity = -1
		}
		vb, vc, ve := voltage(terminals["BASE"]), voltage(terminals["COLLECTOR"]), voltage(terminals["EMITTER"])
		currents, _ := bjtCurrentsAndJacobian(vb, vc, ve, namedValueMap(device.ModelParameters), polarity)
		return math.Abs(vb*currents[0] + vc*currents[1] + ve*currents[2]), true
	case PrimitiveOpAmpV1:
		branch, exists := system.branchIndex[device.Component]
		if !exists {
			return 0, true
		}
		parameters := namedValueMap(device.ModelParameters)
		outputPower := math.Abs(voltage(terminals["OUT"]) * real(solution[branch]))
		quiescent, _ := namedValue(parameters, "quiescent_current_a")
		supply := math.Abs(voltage(terminals["V_PLUS"]) - voltage(terminals["V_MINUS"]))
		return outputPower + quiescent*supply, true
	case PrimitiveComparatorOpenCollectorV1:
		parameters := namedValueMap(device.ModelParameters)
		output := voltage(terminals["OUT"]) - voltage(terminals["V_MINUS"])
		resistance := parameters["output_off_resistance_ohm"]
		if comparatorOn(device, system, solution) {
			resistance = parameters["output_on_resistance_ohm"]
		}
		outputPower := output * output / resistance
		supply := math.Abs(voltage(terminals["V_PLUS"]) - voltage(terminals["V_MINUS"]))
		return outputPower + parameters["quiescent_current_a"]*supply, true
	case PrimitiveMCUStaticSupplyLoadV1, PrimitiveSensorStaticSupplyLoadV1:
		parameters := namedValueMap(device.ModelParameters)
		supply := math.Abs(voltage(terminals["POWER"]) - voltage(terminals["GROUND"]))
		return parameters["maximum_supply_current_a"] * supply, true
	default:
		return 0, false
	}
}

func thermalAssertionValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	if len(result.Points) != 1 {
		return 0, advancedAssertionDiagnostic(assertion, "steady-state thermal result must contain exactly one operating point")
	}
	for _, device := range result.Points[0].Devices {
		if device.Component != assertion.Component {
			continue
		}
		switch assertion.Quantity {
		case QuantityDeviceDissipationW:
			return device.DissipationW, nil
		case QuantityJunctionTemperatureC:
			if device.JunctionTemperatureC == nil {
				return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Component, Message: "junction-temperature assertion lacks a complete catalog-backed thermal path", Suggestion: "select a reviewed component model with thermal resistance and maximum temperature evidence"}
			}
			return *device.JunctionTemperatureC, nil
		}
	}
	return 0, &Diagnostic{Path: "assertions." + assertion.AnalysisID + "." + assertion.Component, Message: "thermal assertion did not resolve to a dissipative device result"}
}

func namedValue(values map[string]float64, name string) (float64, bool) {
	value, exists := values[name]
	return value, exists
}

func resolvedThermalPath(parameters map[string]float64, conditions []NamedValue, ambient float64) (float64, float64, bool) {
	if theta, exists := namedValue(parameters, "thermal_resistance_c_per_w"); exists {
		return theta, ambient, true
	}
	if theta, exists := namedValue(parameters, "junction_to_ambient_c_per_w"); exists {
		return theta, ambient, true
	}
	if theta, exists := namedValue(parameters, "junction_to_case_c_per_w"); exists {
		caseTemperature, hasCase := namedValue(namedValueMap(conditions), "case_temperature_c")
		if hasCase {
			return theta, caseTemperature, true
		}
	}
	return 0, 0, false
}
