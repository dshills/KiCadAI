package architecturesearch

import (
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/components"
)

const (
	currentRegulatorSetResistanceOhm       = 20_000.0
	currentRegulatorPassiveTolerance       = 0.1
	currentRegulatorPassiveTempcoPPMPerC   = 25.0
	currentRegulatorLeakageOhm             = 10_000_000.0
	currentRegulatorInputDischargeOhm      = 10_000.0
	currentRegulatorGatePullupOhm          = 100_000.0
	currentRegulatorGateDriveOhm           = 2_200.0
	currentRegulatorEnableResistanceOhm    = 100_000.0
	currentRegulatorEnablePulldownOhm      = 150_000.0
	currentRegulatorEnableCapacitanceF     = 10e-9
	currentRegulatorReferenceBiasOhm       = 4_700.0
	currentRegulatorDividerLowerOhm        = 1_000.0
	currentRegulatorDividerMinimumVoltageV = 0.05
	currentRegulatorDividerStepVoltageV    = 0.0005
	currentRegulatorHeadroomGuardV         = 0.0001
)

func (provider *CatalogProvider) expandConstantCurrentRegulation(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	current, tolerance, err := requiredNumber(request.Constraints, "output_current", "target", "A")
	if err != nil || current <= 0 || tolerance <= 0 {
		return nil, fmt.Errorf("constant-current regulation requires a positive bounded output-current target")
	}
	compliance, _, err := requiredNumber(request.Constraints, "minimum_compliance_voltage", "minimum", "V")
	if err != nil || compliance < 0 {
		return nil, fmt.Errorf("constant-current regulation requires a nonnegative minimum compliance voltage")
	}
	inputMinimum, inputMaximum, ok := roleVoltageRange(request.Ports, "input")
	if !ok || inputMinimum <= compliance {
		return nil, fmt.Errorf("constant-current input range does not provide positive compliance headroom")
	}
	outputMinimum, outputMaximum, outputRangeOK := roleVoltageRange(request.Ports, "output")
	if !outputRangeOK {
		outputMinimum, outputMaximum = 0, compliance
	}
	if err := requireBool(request.Constraints, "startup_isolation", "required", true); err != nil {
		return nil, fmt.Errorf("constant-current regulation requires startup isolation: %w", err)
	}

	temperature := temperatureRequirementFromConstraints(request.Constraints)
	temperatureDeltaC := currentRegulatorMaximumTemperatureDelta(temperature)
	passiveTolerance := currentRegulatorEffectivePassiveTolerance(temperature)
	precisionMode := tolerance <= 1.5
	worstDissipation := inputMaximum * current
	regulatorRatings := []components.RequiredRating{
		{Kind: "input_output_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "output_current", Value: numericString(current), Unit: "A"},
	}
	var regulator catalogPart
	if precisionMode {
		regulator, err = provider.selectComponentMinimizingModelUncertaintyWithTemperature(
			ctx, "current_regulator", "", regulatorRatings, true, temperature,
			thermalRequirementFromConstraints(request.Constraints, worstDissipation),
			"mna_programmable_current_source_v1", "offset_voltage_v", "model_parameters.offset_voltage_v",
			map[string]float64{"min_headroom_v": inputMinimum - compliance - currentRegulatorHeadroomGuardV},
		)
	} else {
		regulator, err = provider.selectComponentMinimizingModelParameterWithTemperature(
			ctx, "current_regulator", "", regulatorRatings, true, temperature,
			thermalRequirementFromConstraints(request.Constraints, worstDissipation),
			"mna_programmable_current_source_v1", "min_headroom_v", nil,
		)
	}
	if err != nil {
		return nil, err
	}
	regulator.selected.InstanceID, regulator.usage = "current_regulator", "regulator"
	minimumOutputCurrent, minimumCurrentOK := recordValueMaximum(regulator.record, "minimum_output_current", "A")
	if !minimumCurrentOK {
		minimumOutputCurrent, minimumCurrentOK = recordValue(regulator.record, "minimum_output_current", "A")
	}
	if !minimumCurrentOK || minimumOutputCurrent <= 0 {
		return nil, fmt.Errorf("selected current regulator lacks a positive catalog-backed minimum output current")
	}
	if current < minimumOutputCurrent {
		return nil, fmt.Errorf("requested output current %.12g A is below selected regulator minimum %.12g A", current, minimumOutputCurrent)
	}

	inputSwitch, err := provider.selectComponentMinimizingModelParameterWithTemperature(ctx, "mosfet", "p_channel", []components.RequiredRating{
		{Kind: "drain_current", Value: numericString(current), Unit: "A"},
		{Kind: "drain_source_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "gate_source_voltage", Value: numericString(inputMaximum / catalogRatingDeratingFactor), Unit: "V"},
	}, true, temperature, nil, "mna_pmos_guaranteed_switch_v1", "on_resistance_ohm", map[string]float64{
		"gate_on_voltage_v": inputMinimum,
	})
	if err != nil {
		return nil, fmt.Errorf("constant-current input-switch selection failed: %w", err)
	}
	inputSwitch.selected.InstanceID, inputSwitch.usage = "current_enable", "default_off_high_side_switch"
	enableDriver, err := provider.selectComponentWithTemperature(ctx, "mosfet", "AO3400A", []components.RequiredRating{
		{Kind: "drain_current", Value: numericString(inputMaximum / currentRegulatorGateDriveOhm), Unit: "A"},
		{Kind: "drain_source_voltage", Value: numericString(inputMaximum), Unit: "V"},
	}, true, temperature)
	if err != nil {
		return nil, fmt.Errorf("constant-current enable-driver selection failed: %w", err)
	}
	enableDriver.selected.InstanceID, enableDriver.usage = "current_enable_driver", "gate_control_inverter"
	enableThreshold, enableThresholdOK := catalogSimulationParameterForModel(enableDriver.record, "mna_nmos_guaranteed_switch_v1", "gate_on_voltage_v")
	if !enableThresholdOK || enableThreshold <= 0 || enableThreshold >= inputMinimum {
		return nil, fmt.Errorf("constant-current enable driver lacks a compatible gate threshold")
	}
	referenceCurrent, ok := catalogSimulationParameter(regulator.record, "reference_current_a")
	if !ok || referenceCurrent <= 0 || current <= referenceCurrent {
		return nil, fmt.Errorf("selected current regulator lacks a compatible positive reference-current model")
	}
	referenceCurrentMinimum, referenceCurrentMaximum := catalogUncertaintyInterval(regulator.record, "model_parameters.reference_current_a", referenceCurrent)
	offsetVoltage, _ := catalogSimulationParameterAllowZero(regulator.record, "offset_voltage_v")
	offsetMinimum, offsetMaximum := catalogUncertaintyInterval(regulator.record, "model_parameters.offset_voltage_v", offsetVoltage)
	minimumHeadroom, ok := catalogSimulationParameter(regulator.record, "min_headroom_v")
	if !ok || minimumHeadroom <= 0 {
		return nil, fmt.Errorf("selected current regulator lacks a catalog-backed headroom limit")
	}
	switchResistance, ok := catalogSimulationParameterForModel(inputSwitch.record, "mna_pmos_guaranteed_switch_v1", "on_resistance_ohm")
	if !ok || switchResistance <= 0 {
		return nil, fmt.Errorf("selected input switch lacks a catalog-backed on-resistance")
	}
	gateOnVoltage, ok := catalogSimulationParameterForModel(inputSwitch.record, "mna_pmos_guaranteed_switch_v1", "gate_on_voltage_v")
	if !ok || gateOnVoltage <= 0 || gateOnVoltage > inputMinimum {
		return nil, fmt.Errorf("selected input switch lacks compatible bounded gate-drive evidence")
	}
	gateRating, gateRatingOK := recordRatingMaximum(inputSwitch.record, "gate_source_voltage", "V")
	if !gateRatingOK || inputMaximum > gateRating*catalogRatingDeratingFactor {
		return nil, fmt.Errorf("selected input switch does not prove the source-referenced gate-drive envelope")
	}
	gateCharge, gateChargeOK := recordValueMaximum(inputSwitch.record, "total_gate_charge", "C")
	if !gateChargeOK {
		gateCharge, gateChargeOK = recordValue(inputSwitch.record, "total_gate_charge", "C")
	}
	if !gateChargeOK || gateCharge <= 0 {
		return nil, fmt.Errorf("selected input switch lacks bounded gate-charge evidence")
	}

	setVoltageNominal := referenceCurrent * currentRegulatorSetResistanceOhm
	setVoltageMinimum := referenceCurrentMinimum * currentRegulatorSetResistanceOhm * (1 - passiveTolerance/100)
	setVoltageMaximum := referenceCurrentMaximum * currentRegulatorSetResistanceOhm * (1 + passiveTolerance/100)
	setResistance := currentRegulatorSetResistanceOhm
	designCurrent := current
	parallelOutputCurrentMinimum := 0.0
	parallelOutputCurrentMaximum := 0.0
	parallelResistance := 0.0
	dividerUpperResistance := 0.0
	dividerReferenceVoltage := 0.0
	dividerReferenceMinimum := 0.0
	dividerReferenceMaximum := 0.0
	parts := []catalogPart{regulator, inputSwitch, enableDriver}
	passives := []passivePart{
		{"output_program", "resistor", "series_current_shunt", ""},
		{"gate_pullup", "resistor", "gate_drive_pullup", engineeringValue(currentRegulatorGatePullupOhm, "Ohm")},
		{"gate_drive", "resistor", "gate_clamp_current_limit", engineeringValue(currentRegulatorGateDriveOhm, "Ohm")},
		{"enable_base", "resistor", "gate_buffer_input", engineeringValue(currentRegulatorEnableResistanceOhm, "Ohm")},
		{"enable_base_pulldown", "resistor", "default_off", engineeringValue(currentRegulatorEnablePulldownOhm, "Ohm")},
		{"output_pulldown", "resistor", "default_off", engineeringValue(currentRegulatorLeakageOhm, "Ohm")},
		{"input_discharge", "resistor", "startup_inactive", engineeringValue(currentRegulatorInputDischargeOhm, "Ohm")},
	}
	controlled := hasRoleContract(request.Ports, "control")
	if !controlled {
		passives = append(passives, passivePart{"enable_delay", "capacitor", "delayed_startup_isolation", engineeringValue(currentRegulatorEnableCapacitanceF, "F")})
	}
	var reference catalogPart
	if precisionMode {
		reference, err = provider.selectComponentMinimizingModelParameterWithTemperature(ctx, "voltage_reference", "", []components.RequiredRating{
			{Kind: "bias_current", Value: "0.001", Unit: "A"},
		}, true, temperature, nil, "mna_shunt_voltage_reference_v1", "min_bias_current_a", nil)
		if err != nil {
			return nil, fmt.Errorf("precision constant-current reference selection failed: %w", err)
		}
		reference.selected.InstanceID, reference.usage = "current_reference", "threshold_voltage_reference"
		parts = append(parts, reference)
		setVoltageNominal, ok = catalogSimulationParameter(reference.record, "output_voltage_v")
		if !ok || setVoltageNominal <= 0 {
			return nil, fmt.Errorf("selected precision current reference lacks output-voltage evidence")
		}
		setVoltageMinimum, setVoltageMaximum = catalogUncertaintyBounds(reference.record, "model_parameters.output_voltage_v", setVoltageNominal)
		minimumBiasCurrent, minimumBiasOK := catalogSimulationParameter(reference.record, "min_bias_current_a")
		maximumBiasCurrent, maximumBiasOK := catalogSimulationParameter(reference.record, "max_bias_current_a")
		if !minimumBiasOK || !maximumBiasOK || minimumBiasCurrent <= 0 || maximumBiasCurrent < minimumBiasCurrent {
			return nil, fmt.Errorf("selected precision current reference lacks bounded bias-current evidence")
		}
		if referenceCurrentMinimum >= minimumBiasCurrent && referenceCurrentMaximum <= maximumBiasCurrent {
			designCurrent = current
		} else {
			parallelResistance, parallelOutputCurrentMinimum, parallelOutputCurrentMaximum, err = provider.solveCurrentReferenceBias(
				ctx,
				inputMinimum, inputMaximum,
				outputMinimum, outputMaximum,
				setVoltageMinimum, setVoltageMaximum,
				minimumBiasCurrent, maximumBiasCurrent,
				passiveTolerance,
			)
			if err != nil {
				return nil, err
			}
			designCurrent = current - (parallelOutputCurrentMinimum+parallelOutputCurrentMaximum)/2
			passives = append(passives, passivePart{"reference_bias", "resistor", "bias_current", engineeringValue(parallelResistance, "Ohm")})
		}
		if designCurrent < minimumOutputCurrent {
			return nil, fmt.Errorf("precision reference burden leaves regulator design current %.12g A below minimum %.12g A", designCurrent, minimumOutputCurrent)
		}
	} else {
		standardCalculation, standardErr := currentRegulatorCalculation(currentRegulatorCalculationRequest{
			CurrentA: current, TolerancePercent: tolerance,
			ComplianceV: compliance, InputMinimumV: inputMinimum, InputMaximumV: inputMaximum,
			ReferenceCurrentA: referenceCurrent, ReferenceCurrentMinimumA: referenceCurrentMinimum, ReferenceCurrentMaximumA: referenceCurrentMaximum,
			OffsetVoltageMinimumV: offsetMinimum, OffsetVoltageMaximumV: offsetMaximum,
			DesignCurrentA: designCurrent, SetVoltageNominalV: setVoltageNominal,
			SetVoltageMinimumV: setVoltageMinimum, SetVoltageMaximumV: setVoltageMaximum,
			SetResistanceOhm: setResistance, OutputResistanceOhm: setVoltageNominal / (current - referenceCurrent),
			PassiveTolerancePercent: passiveTolerance,
			MinimumHeadroomV:        minimumHeadroom, SwitchResistanceOhm: switchResistance,
		})
		if standardErr == nil && standardCalculation.Pass {
			passives = append(passives, passivePart{"set_program", "resistor", "bias_current", engineeringValue(setResistance, "Ohm")})
		} else {
			setVoltageLimit, limitOK := recordRatingMaximum(regulator.record, "set_pin_voltage", "V")
			if !limitOK || setVoltageLimit <= 0 {
				return nil, fmt.Errorf("selected current regulator lacks a bounded SET-voltage rating")
			}
			directSolution, directErr := solveDirectCurrentSetpoint(directCurrentSetpointRequest{
				CurrentA: current, TolerancePercent: tolerance,
				ComplianceV: compliance, InputMinimumV: inputMinimum, InputMaximumV: inputMaximum,
				ReferenceCurrentA: referenceCurrent, ReferenceCurrentMinimumA: referenceCurrentMinimum, ReferenceCurrentMaximumA: referenceCurrentMaximum,
				OffsetVoltageMinimumV: offsetMinimum, OffsetVoltageMaximumV: offsetMaximum,
				MaximumSetVoltageV:      setVoltageLimit * catalogRatingDeratingFactor,
				PassiveTolerancePercent: passiveTolerance,
				MinimumHeadroomV:        minimumHeadroom, SwitchResistanceOhm: switchResistance,
			})
			if directErr == nil {
				designCurrent = directSolution.DesignCurrentA
				setResistance = directSolution.SetResistanceOhm
				setVoltageNominal, setVoltageMinimum, setVoltageMaximum = directSolution.NominalV, directSolution.MinimumV, directSolution.MaximumV
				passives = append(passives, passivePart{"set_program", "resistor", "bias_current", engineeringValue(setResistance, "Ohm")})
			} else {
				reference, err = provider.selectComponentMinimizingModelParameterWithTemperature(ctx, "voltage_reference", "", []components.RequiredRating{
					{Kind: "bias_current", Value: "0.001", Unit: "A"},
				}, true, temperature, nil, "mna_shunt_voltage_reference_v1", "min_bias_current_a", nil)
				if err != nil {
					return nil, fmt.Errorf("low-headroom constant-current reference selection failed: %w", err)
				}
				reference.selected.InstanceID, reference.usage = "current_reference", "threshold_voltage_reference"
				parts = append(parts, reference)
				referenceVoltage, voltageOK := catalogSimulationParameter(reference.record, "output_voltage_v")
				if !voltageOK || referenceVoltage <= 0 {
					return nil, fmt.Errorf("selected low-headroom reference lacks output-voltage evidence")
				}
				referenceMinimum, referenceMaximum := catalogUncertaintyBounds(reference.record, "model_parameters.output_voltage_v", referenceVoltage)
				parallelOutputCurrentMaximum = inputMaximum / (currentRegulatorReferenceBiasOhm * (1 - passiveTolerance/100))
				parallelResistance = currentRegulatorReferenceBiasOhm
				setSolution, solveErr := solveLowHeadroomSetpoint(lowHeadroomSetpointRequest{
					CurrentA: current, TolerancePercent: tolerance,
					ComplianceV: compliance, InputMinimumV: inputMinimum, InputMaximumV: inputMaximum,
					ReferenceCurrentA: referenceCurrent, ReferenceCurrentMinimumA: referenceCurrentMinimum, ReferenceCurrentMaximumA: referenceCurrentMaximum,
					OffsetVoltageMinimumV: offsetMinimum, OffsetVoltageMaximumV: offsetMaximum,
					ParallelOutputCurrentMaximumA: parallelOutputCurrentMaximum,
					ReferenceVoltageV:             referenceVoltage,
					ReferenceMinimumV:             referenceMinimum, ReferenceMaximumV: referenceMaximum,
					PassiveTolerancePercent: passiveTolerance,
					MinimumHeadroomV:        minimumHeadroom, SwitchResistanceOhm: switchResistance,
				})
				if solveErr != nil {
					return nil, solveErr
				}
				precisionMode = true
				designCurrent = setSolution.DesignCurrentA
				setVoltageNominal, setVoltageMinimum, setVoltageMaximum = setSolution.NominalV, setSolution.MinimumV, setSolution.MaximumV
				dividerUpperResistance = setSolution.UpperResistanceOhm
				dividerReferenceVoltage, dividerReferenceMinimum, dividerReferenceMaximum = referenceVoltage, referenceMinimum, referenceMaximum
				passives = append(passives,
					passivePart{"reference_bias", "resistor", "bias_current", engineeringValue(currentRegulatorReferenceBiasOhm, "Ohm")},
					passivePart{"set_divider_upper", "resistor", "threshold_divider", engineeringValue(setSolution.UpperResistanceOhm, "Ohm")},
					passivePart{"set_divider_lower", "resistor", "threshold_divider", engineeringValue(currentRegulatorDividerLowerOhm, "Ohm")},
				)
			}
		}
	}

	preferred, err := provider.realizeCurrentRegulatorPreferredValues(ctx, currentRegulatorPreferredRealizationRequest{
		Calculation: currentRegulatorCalculationRequest{
			CurrentA: current, TolerancePercent: tolerance,
			ComplianceV: compliance, InputMinimumV: inputMinimum, InputMaximumV: inputMaximum,
			ReferenceCurrentA: referenceCurrent, ReferenceCurrentMinimumA: referenceCurrentMinimum, ReferenceCurrentMaximumA: referenceCurrentMaximum,
			OffsetVoltageMinimumV: offsetMinimum, OffsetVoltageMaximumV: offsetMaximum,
			DesignCurrentA: designCurrent, ParallelOutputCurrentMaximumA: parallelOutputCurrentMaximum,
			ParallelOutputCurrentMinimumA: parallelOutputCurrentMinimum,
			SetVoltageNominalV:            setVoltageNominal, SetVoltageMinimumV: setVoltageMinimum, SetVoltageMaximumV: setVoltageMaximum,
			SetResistanceOhm: setResistance, PrecisionMode: precisionMode,
			PassiveTolerancePercent: passiveTolerance,
			MinimumHeadroomV:        minimumHeadroom, SwitchResistanceOhm: switchResistance,
		},
		DividerUpperResistanceOhm: dividerUpperResistance,
		DividerLowerResistanceOhm: currentRegulatorDividerLowerOhm,
		ReferenceVoltageV:         dividerReferenceVoltage,
		ReferenceMinimumV:         dividerReferenceMinimum,
		ReferenceMaximumV:         dividerReferenceMaximum,
		ParallelResistanceOhm:     parallelResistance,
		TemperatureDeltaC:         temperatureDeltaC,
	})
	if err != nil {
		return nil, err
	}
	setResistance = preferred.SetResistanceOhm
	dividerUpperResistance = preferred.DividerUpperResistanceOhm
	setVoltageNominal, setVoltageMinimum, setVoltageMaximum = preferred.SetVoltageNominalV, preferred.SetVoltageMinimumV, preferred.SetVoltageMaximumV
	outputResistance, outputTrimResistance, designCurrent := preferred.OutputResistanceOhm, preferred.OutputTrimResistanceOhm, preferred.DesignCurrentA
	for index := range passives {
		switch passives[index].id {
		case "output_program":
			passives[index].value = engineeringValue(outputResistance, "Ohm")
		case "set_program":
			passives[index].value = engineeringValue(setResistance, "Ohm")
		case "set_divider_upper":
			passives[index].value = engineeringValue(dividerUpperResistance, "Ohm")
		}
	}
	if outputTrimResistance > 0 {
		passives = append(passives, passivePart{
			"output_program_trim", "resistor", "series_current_shunt", engineeringValue(outputTrimResistance, "Ohm"),
		})
	}
	precisionPassives := map[string]float64{
		"output_program":      currentRegulatorPassiveTolerance,
		"output_program_trim": currentRegulatorPassiveTolerance,
		"set_program":         currentRegulatorPassiveTolerance,
	}
	if precisionMode {
		precisionPassives["reference_bias"] = currentRegulatorPassiveTolerance
		precisionPassives["set_divider_upper"] = currentRegulatorPassiveTolerance
		precisionPassives["set_divider_lower"] = currentRegulatorPassiveTolerance
	}
	parts, err = provider.appendPassivePartsWithTolerances(ctx, parts, passives, precisionPassives, currentRegulatorPassiveTempcoPPMPerC)
	if err != nil {
		return nil, err
	}

	calculation := preferred.Calculation
	responseTime := gateCharge * currentRegulatorGateDriveOhm / inputMinimum
	if !controlled {
		finalBaseVoltage := inputMinimum * currentRegulatorEnablePulldownOhm / (currentRegulatorEnableResistanceOhm + currentRegulatorEnablePulldownOhm)
		if finalBaseVoltage <= enableThreshold {
			return nil, fmt.Errorf("autonomous startup-delay drive cannot reach the enable threshold")
		}
		timeConstant := currentRegulatorEnableCapacitanceF / (1/currentRegulatorEnableResistanceOhm + 1/currentRegulatorEnablePulldownOhm)
		responseTime += -timeConstant * math.Log(1-enableThreshold/finalBaseVoltage)
	}
	response, err := boundedObservedCalculation("current_enable_response", "response_time", responseTime, "s", request.Constraints)
	if err != nil {
		return nil, err
	}

	bindings := bindRoles(request.Ports, regulator.selected.InstanceID, map[string]string{
		"input": "IN", "output": "OUT", "reference": "OUT", "control": "SET",
	})
	outputProgramEndpointID := "output_program"
	if outputTrimResistance > 0 {
		outputProgramEndpointID = "output_program_trim"
	}
	for index := range bindings {
		switch bindings[index].Role {
		case "input":
			bindings[index].Instance, bindings[index].Function = inputSwitch.selected.InstanceID, "SOURCE"
		case "output":
			bindings[index].Instance, bindings[index].Function = outputProgramEndpointID, "B"
		case "reference":
			bindings[index].Instance, bindings[index].Function = "output_pulldown", "B"
		case "control":
			bindings[index].Instance, bindings[index].Function = "enable_base", "A"
		}
	}

	inputEndpoints := []RealizationEndpoint{endpoint(inputSwitch, "SOURCE"), passiveEndpoint("gate_pullup", "A")}
	if !controlled {
		inputEndpoints = append(inputEndpoints, passiveEndpoint("enable_base", "A"))
	}
	setEndpoints := []RealizationEndpoint{endpoint(regulator, "SET")}
	referenceEndpoints := []RealizationEndpoint{
		passiveEndpoint("output_pulldown", "B"),
		passiveEndpoint("input_discharge", "B"),
		passiveEndpoint("enable_base_pulldown", "B"),
		endpoint(enableDriver, "SOURCE"),
	}
	outputEndpoints := []RealizationEndpoint{passiveEndpoint(outputProgramEndpointID, "B"), passiveEndpoint("output_pulldown", "A")}
	lowHeadroomMode := precisionMode && slices.ContainsFunc(passives, func(passive passivePart) bool {
		return passive.id == "set_divider_upper"
	})
	if lowHeadroomMode {
		setEndpoints = append(setEndpoints, passiveEndpoint("set_divider_upper", "B"), passiveEndpoint("set_divider_lower", "A"))
		outputEndpoints = append(outputEndpoints, endpoint(reference, "ANODE"), passiveEndpoint("set_divider_lower", "B"))
	} else if precisionMode {
		setEndpoints = append(setEndpoints, endpoint(reference, "CATHODE"))
		if parallelResistance > 0 {
			setEndpoints = append(setEndpoints, passiveEndpoint("reference_bias", "B"))
		}
		outputEndpoints = append(outputEndpoints, endpoint(reference, "ANODE"))
	} else {
		setEndpoints = append(setEndpoints, passiveEndpoint("set_program", "A"))
		outputEndpoints = append(outputEndpoints, passiveEndpoint("set_program", "B"))
	}
	if !controlled {
		referenceEndpoints = append(referenceEndpoints, passiveEndpoint("enable_delay", "B"))
	}
	connections := []RealizationConnection{
		semanticNet("current_source_input", "power", inputEndpoints...),
		semanticNet("current_source_enabled_input", "switched_power", endpoint(inputSwitch, "DRAIN"), endpoint(regulator, "IN"), passiveEndpoint("input_discharge", "A")),
		semanticNet("current_source_enable_gate", "control", endpoint(inputSwitch, "GATE"), passiveEndpoint("gate_pullup", "B"), passiveEndpoint("gate_drive", "A")),
		semanticNet("current_source_enable_collector", "control", passiveEndpoint("gate_drive", "B"), endpoint(enableDriver, "DRAIN")),
		semanticNet("current_source_enable_base", "control", passiveEndpoint("enable_base", "B"), passiveEndpoint("enable_base_pulldown", "A"), endpoint(enableDriver, "GATE")),
		semanticNet("current_source_set", "bias", setEndpoints...),
		semanticNet("current_source_buffer", "regulated_current", endpoint(regulator, "OUT"), endpoint(regulator, "OUT_TAB"), passiveEndpoint("output_program", "A")),
		semanticNet("current_source_output", "regulated_current", outputEndpoints...),
		semanticNet("current_source_reference", "reference", referenceEndpoints...),
	}
	if !controlled {
		connections[4].Endpoints = append(connections[4].Endpoints, passiveEndpoint("enable_delay", "A"))
	}
	if precisionMode && parallelResistance > 0 {
		connections[1].Endpoints = append(connections[1].Endpoints, passiveEndpoint("reference_bias", "A"))
	}
	if outputTrimResistance > 0 {
		connections = append(connections,
			semanticNet(
				"current_source_output_program",
				"regulated_current",
				passiveEndpoint("output_program", "B"),
				passiveEndpoint("output_program_trim", "A"),
			),
		)
	}
	if lowHeadroomMode {
		connections = append(connections,
			semanticNet("current_source_precision_reference", "reference_voltage", passiveEndpoint("reference_bias", "B"), endpoint(reference, "CATHODE"), passiveEndpoint("set_divider_upper", "A")),
		)
	}
	connections = retainSemanticNets(connections)
	return provider.expansion(request, "default_off_programmable_current_source", parts, bindings, connections, []CalculationEvidence{calculation, response}, 0)
}

func currentRegulatorEffectivePassiveTolerance(temperature *components.TemperatureRequirement) float64 {
	maximumDeltaC := currentRegulatorMaximumTemperatureDelta(temperature)
	return currentRegulatorPassiveTolerance + currentRegulatorPassiveTempcoPPMPerC*maximumDeltaC/10_000
}

func currentRegulatorMaximumTemperatureDelta(temperature *components.TemperatureRequirement) float64 {
	maximumDeltaC := 150.0
	if temperature != nil {
		maximumDeltaC = 0
		if temperature.MinimumC != nil {
			maximumDeltaC = math.Max(maximumDeltaC, math.Abs(*temperature.MinimumC-25))
		}
		if temperature.MaximumC != nil {
			maximumDeltaC = math.Max(maximumDeltaC, math.Abs(*temperature.MaximumC-25))
		}
	}
	return maximumDeltaC
}

func catalogResistorEffectiveTolerance(record components.ComponentRecord, maximumDeltaC float64) (float64, bool) {
	tolerance, toleranceOK := catalogToleranceMaximum(record, "resistance", "%")
	tempco, tempcoOK := recordValueMaximum(record, "temperature_coefficient", "ppm/C")
	if !toleranceOK || !tempcoOK || tolerance <= 0 || tempco <= 0 || maximumDeltaC < 0 {
		return 0, false
	}
	return tolerance + tempco*maximumDeltaC/10_000, true
}

func (provider *CatalogProvider) solveCurrentReferenceBias(
	ctx context.Context,
	inputMinimum, inputMaximum,
	outputMinimum, outputMaximum,
	referenceMinimum, referenceMaximum,
	minimumBiasCurrent, maximumBiasCurrent,
	passiveTolerancePercent float64,
) (float64, float64, float64, error) {
	minimumAvailableVoltage := inputMinimum - outputMaximum - referenceMaximum
	maximumAvailableVoltage := inputMaximum - outputMinimum - referenceMinimum
	if minimumAvailableVoltage <= 0 || maximumAvailableVoltage < minimumAvailableVoltage || minimumBiasCurrent <= 0 || maximumBiasCurrent < minimumBiasCurrent {
		return 0, 0, 0, fmt.Errorf("precision current reference has no positive bounded bias-voltage envelope")
	}
	tolerance := passiveTolerancePercent / 100
	maximumResistance := minimumAvailableVoltage / (minimumBiasCurrent * (1 + tolerance))
	candidates, err := provider.preferredResistanceCandidates(
		ctx, maximumResistance, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("precision current reference bias realization failed: %w", err)
	}
	for _, resistance := range candidates {
		if resistance > maximumResistance {
			continue
		}
		minimumCurrent := minimumAvailableVoltage / (resistance * (1 + tolerance))
		maximumCurrent := maximumAvailableVoltage / (resistance * (1 - tolerance))
		if minimumCurrent < minimumBiasCurrent || maximumCurrent > maximumBiasCurrent {
			continue
		}
		return resistance, minimumCurrent, maximumCurrent, nil
	}
	return 0, 0, 0, fmt.Errorf("precision current reference has no catalog-realizable bias resistor satisfying its operating-current envelope")
}

type currentRegulatorCalculationRequest struct {
	CurrentA                      float64
	DesignCurrentA                float64
	TolerancePercent              float64
	ComplianceV                   float64
	InputMinimumV                 float64
	InputMaximumV                 float64
	ReferenceCurrentA             float64
	ReferenceCurrentMinimumA      float64
	ReferenceCurrentMaximumA      float64
	OffsetVoltageMinimumV         float64
	OffsetVoltageMaximumV         float64
	SetVoltageNominalV            float64
	SetVoltageMinimumV            float64
	SetVoltageMaximumV            float64
	SetResistanceOhm              float64
	OutputResistanceOhm           float64
	ParallelOutputCurrentMinimumA float64
	ParallelOutputCurrentMaximumA float64
	PrecisionMode                 bool
	PassiveTolerancePercent       float64
	SetTolerancePercent           float64
	OutputTolerancePercent        float64
	MinimumHeadroomV              float64
	SwitchResistanceOhm           float64
	SelectedValues                []SelectedValueEvidence
}

type currentRegulatorPreferredRealizationRequest struct {
	Calculation               currentRegulatorCalculationRequest
	DividerUpperResistanceOhm float64
	DividerLowerResistanceOhm float64
	ReferenceVoltageV         float64
	ReferenceMinimumV         float64
	ReferenceMaximumV         float64
	ParallelResistanceOhm     float64
	TemperatureDeltaC         float64
}

type currentRegulatorPreferredRealization struct {
	Calculation               CalculationEvidence
	SetResistanceOhm          float64
	DividerUpperResistanceOhm float64
	OutputResistanceOhm       float64
	OutputTrimResistanceOhm   float64
	SetVoltageNominalV        float64
	SetVoltageMinimumV        float64
	SetVoltageMaximumV        float64
	DesignCurrentA            float64
}

func (provider *CatalogProvider) realizeCurrentRegulatorPreferredValues(ctx context.Context, request currentRegulatorPreferredRealizationRequest) (currentRegulatorPreferredRealization, error) {
	base := request.Calculation
	setCandidates := []float64{base.SetResistanceOhm}
	if !base.PrecisionMode {
		var err error
		setCandidates, err = provider.preferredResistanceCandidates(
			ctx, base.SetResistanceOhm, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
		)
		if err != nil {
			return currentRegulatorPreferredRealization{}, fmt.Errorf("constant-current SET resistor realization failed: %w", err)
		}
		offsetNominal := (base.OffsetVoltageMinimumV + base.OffsetVoltageMaximumV) / 2
		idealOutputResistance := (base.SetVoltageNominalV + offsetNominal) / (base.DesignCurrentA - base.ReferenceCurrentA)
		outputCandidates, outputErr := provider.preferredResistanceCandidates(
			ctx, idealOutputResistance, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
		)
		if outputErr == nil && len(outputCandidates) > 0 && outputCandidates[0] > idealOutputResistance {
			neededSetResistance := ((base.DesignCurrentA-base.ReferenceCurrentA)*outputCandidates[0] - offsetNominal) / base.ReferenceCurrentA
			if finitePositive(neededSetResistance) {
				accessCandidates, accessErr := provider.preferredResistanceCandidates(
					ctx, neededSetResistance, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
				)
				if accessErr == nil {
					seen := make(map[float64]bool, len(setCandidates)+len(accessCandidates))
					for _, candidate := range setCandidates {
						seen[candidate] = true
					}
					for _, candidate := range accessCandidates {
						if !seen[candidate] {
							setCandidates = append(setCandidates, candidate)
							seen[candidate] = true
						}
					}
				}
			}
		}
	}
	dividerCandidates := []float64{request.DividerUpperResistanceOhm}
	if request.DividerUpperResistanceOhm > 0 {
		var err error
		dividerCandidates, err = provider.preferredResistanceCandidates(
			ctx, request.DividerUpperResistanceOhm, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
		)
		if err != nil {
			return currentRegulatorPreferredRealization{}, fmt.Errorf("constant-current divider realization failed: %w", err)
		}
	}
	offsetNominal := (base.OffsetVoltageMinimumV + base.OffsetVoltageMaximumV) / 2
	evaluated := 0
	var firstErr error
	var lastErr error
	for _, setResistance := range setCandidates {
		for _, dividerUpper := range dividerCandidates {
			candidate := base
			candidate.SelectedValues = nil
			if !candidate.PrecisionMode {
				setPart, selectErr := provider.selectComponentWithTolerance(
					ctx, "resistor", "resistance", engineeringValue(setResistance, "Ohm"),
					"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
				)
				if selectErr != nil {
					continue
				}
				setTolerance, toleranceOK := catalogResistorEffectiveTolerance(setPart.record, request.TemperatureDeltaC)
				if !toleranceOK {
					continue
				}
				candidate.SetTolerancePercent = setTolerance
				candidate.SetResistanceOhm = setResistance
				candidate.SetVoltageNominalV = candidate.ReferenceCurrentA * setResistance
				candidate.SetVoltageMinimumV = candidate.ReferenceCurrentMinimumA * setResistance * (1 - setTolerance/100)
				candidate.SetVoltageMaximumV = candidate.ReferenceCurrentMaximumA * setResistance * (1 + setTolerance/100)
				candidate.SelectedValues = append(candidate.SelectedValues, SelectedValueEvidence{
					Name: "set_program_resistance", Ideal: base.SetResistanceOhm, Selected: setResistance, Unit: "Ohm",
					Series: SeriesE192, TolerancePercent: setTolerance,
					RelativeError: quantize(math.Abs(setResistance-base.SetResistanceOhm) / base.SetResistanceOhm),
				})
			} else if dividerUpper > 0 {
				denominator := 1/dividerUpper + 1/request.DividerLowerResistanceOhm
				if !finitePositive(denominator) {
					continue
				}
				upperPart, upperErr := provider.selectComponentWithTolerance(
					ctx, "resistor", "resistance", engineeringValue(dividerUpper, "Ohm"),
					"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
				)
				lowerPart, lowerErr := provider.selectComponentWithTolerance(
					ctx, "resistor", "resistance", engineeringValue(request.DividerLowerResistanceOhm, "Ohm"),
					"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
				)
				upperTolerance, upperToleranceOK := catalogResistorEffectiveTolerance(upperPart.record, request.TemperatureDeltaC)
				lowerTolerance, lowerToleranceOK := catalogResistorEffectiveTolerance(lowerPart.record, request.TemperatureDeltaC)
				if upperErr != nil || lowerErr != nil || !upperToleranceOK || !lowerToleranceOK {
					continue
				}
				setTolerance := math.Max(upperTolerance, lowerTolerance)
				candidate.SetTolerancePercent = setTolerance
				candidate.SetVoltageNominalV = (request.ReferenceVoltageV/dividerUpper + candidate.ReferenceCurrentA) / denominator
				boundsRequest := lowHeadroomSetpointRequest{
					ReferenceCurrentMinimumA: candidate.ReferenceCurrentMinimumA,
					ReferenceCurrentMaximumA: candidate.ReferenceCurrentMaximumA,
					ReferenceMinimumV:        request.ReferenceMinimumV,
					ReferenceMaximumV:        request.ReferenceMaximumV,
					PassiveTolerancePercent:  setTolerance,
				}
				candidate.SetVoltageMinimumV, candidate.SetVoltageMaximumV = precisionDividerSetpointBounds(boundsRequest, dividerUpper)
				candidate.SelectedValues = append(candidate.SelectedValues,
					SelectedValueEvidence{
						Name: "set_divider_upper_resistance", Ideal: request.DividerUpperResistanceOhm, Selected: dividerUpper, Unit: "Ohm",
						Series: SeriesE192, TolerancePercent: upperTolerance,
						RelativeError: quantize(math.Abs(dividerUpper-request.DividerUpperResistanceOhm) / request.DividerUpperResistanceOhm),
					},
					SelectedValueEvidence{
						Name: "set_divider_lower_resistance", Ideal: request.DividerLowerResistanceOhm, Selected: request.DividerLowerResistanceOhm, Unit: "Ohm",
						Series: SeriesE192, TolerancePercent: lowerTolerance,
					},
				)
			}
			if request.ParallelResistanceOhm > 0 {
				biasPart, biasErr := provider.selectComponentWithTolerance(
					ctx, "resistor", "resistance", engineeringValue(request.ParallelResistanceOhm, "Ohm"),
					"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
				)
				biasTolerance, biasToleranceOK := catalogResistorEffectiveTolerance(biasPart.record, request.TemperatureDeltaC)
				if biasErr != nil || !biasToleranceOK {
					continue
				}
				candidate.SelectedValues = append(candidate.SelectedValues, SelectedValueEvidence{
					Name: "reference_bias_resistance", Ideal: request.ParallelResistanceOhm, Selected: request.ParallelResistanceOhm, Unit: "Ohm",
					Series: SeriesE192, TolerancePercent: biasTolerance,
				})
			}
			idealOutputResistance := (candidate.SetVoltageNominalV + offsetNominal) / (base.DesignCurrentA - candidate.ReferenceCurrentA)
			if !finitePositive(idealOutputResistance) {
				continue
			}
			outputCandidates, err := provider.preferredResistanceCandidates(
				ctx, idealOutputResistance, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
			)
			if err != nil {
				lastErr = err
				continue
			}
			evaluateOutput := func(outputResistance, outputTrimResistance float64) (currentRegulatorPreferredRealization, bool) {
				trial := candidate
				outputPart, selectErr := provider.selectComponentWithTolerance(
					ctx, "resistor", "resistance", engineeringValue(outputResistance, "Ohm"),
					"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
				)
				if selectErr != nil {
					return currentRegulatorPreferredRealization{}, false
				}
				outputTolerance, toleranceOK := catalogResistorEffectiveTolerance(outputPart.record, request.TemperatureDeltaC)
				if !toleranceOK {
					return currentRegulatorPreferredRealization{}, false
				}
				totalOutputResistance := outputResistance
				selectedValues := []SelectedValueEvidence{{
					Name: "output_program_resistance", Ideal: idealOutputResistance, Selected: outputResistance, Unit: "Ohm",
					Series: SeriesE192, TolerancePercent: outputTolerance,
					RelativeError: quantize(math.Abs(outputResistance-idealOutputResistance) / idealOutputResistance),
				}}
				if outputTrimResistance > 0 {
					trimPart, trimErr := provider.selectComponentWithTolerance(
						ctx, "resistor", "resistance", engineeringValue(outputTrimResistance, "Ohm"),
						"resistance", currentRegulatorPassiveTolerance, "%", currentRegulatorPassiveTempcoPPMPerC,
					)
					if trimErr != nil {
						return currentRegulatorPreferredRealization{}, false
					}
					trimTolerance, trimToleranceOK := catalogResistorEffectiveTolerance(trimPart.record, request.TemperatureDeltaC)
					if !trimToleranceOK {
						return currentRegulatorPreferredRealization{}, false
					}
					totalOutputResistance += outputTrimResistance
					outputTolerance = (outputResistance*outputTolerance + outputTrimResistance*trimTolerance) / totalOutputResistance
					selectedValues = append(selectedValues, SelectedValueEvidence{
						Name: "output_program_trim_resistance", Ideal: idealOutputResistance - outputResistance,
						Selected: outputTrimResistance, Unit: "Ohm", Series: SeriesE192, TolerancePercent: trimTolerance,
						RelativeError: quantize(math.Abs(totalOutputResistance-idealOutputResistance) / idealOutputResistance),
					})
				}
				trial.OutputTolerancePercent = outputTolerance
				trial.OutputResistanceOhm = totalOutputResistance
				trial.DesignCurrentA = (trial.SetVoltageNominalV+offsetNominal)/totalOutputResistance +
					trial.ReferenceCurrentA + (trial.ParallelOutputCurrentMinimumA+trial.ParallelOutputCurrentMaximumA)/2
				trial.SelectedValues = append(slices.Clone(candidate.SelectedValues), selectedValues...)
				calculation, err := currentRegulatorCalculation(trial)
				evaluated++
				if err != nil || !calculation.Pass {
					if err != nil {
						lastErr = fmt.Errorf(
							"SET %.12g Ohm, divider %.12g Ohm, output %.12g+%.12g Ohm: %w",
							setResistance, dividerUpper, outputResistance, outputTrimResistance, err,
						)
						if firstErr == nil {
							firstErr = lastErr
						}
					}
					return currentRegulatorPreferredRealization{}, false
				}
				return currentRegulatorPreferredRealization{
					Calculation: calculation, SetResistanceOhm: trial.SetResistanceOhm,
					DividerUpperResistanceOhm: dividerUpper, OutputResistanceOhm: outputResistance,
					OutputTrimResistanceOhm: outputTrimResistance,
					SetVoltageNominalV:      trial.SetVoltageNominalV, SetVoltageMinimumV: trial.SetVoltageMinimumV,
					SetVoltageMaximumV: trial.SetVoltageMaximumV, DesignCurrentA: trial.DesignCurrentA,
				}, true
			}
			for _, outputResistance := range outputCandidates {
				if realization, ok := evaluateOutput(outputResistance, 0); ok {
					return realization, nil
				}
			}
			for _, outputResistance := range outputCandidates {
				remainingResistance := idealOutputResistance - outputResistance
				if !finitePositive(remainingResistance) {
					continue
				}
				trimCandidates, trimErr := provider.preferredResistanceCandidates(
					ctx, remainingResistance, currentRegulatorPassiveTolerance, currentRegulatorPassiveTempcoPPMPerC, DefaultMaxValueCandidates,
				)
				if trimErr != nil {
					lastErr = trimErr
					continue
				}
				for _, outputTrimResistance := range trimCandidates {
					if realization, ok := evaluateOutput(outputResistance, outputTrimResistance); ok {
						return realization, nil
					}
				}
			}
		}
	}
	return currentRegulatorPreferredRealization{}, fmt.Errorf(
		"constant-current envelope has no catalog-realizable preferred-value solution within tolerance and compliance headroom after %d candidates (first rejection: %v; last rejection: %v)",
		evaluated, firstErr, lastErr,
	)
}

type directCurrentSetpointRequest struct {
	CurrentA                 float64
	TolerancePercent         float64
	ComplianceV              float64
	InputMinimumV            float64
	InputMaximumV            float64
	ReferenceCurrentA        float64
	ReferenceCurrentMinimumA float64
	ReferenceCurrentMaximumA float64
	OffsetVoltageMinimumV    float64
	OffsetVoltageMaximumV    float64
	MaximumSetVoltageV       float64
	PassiveTolerancePercent  float64
	MinimumHeadroomV         float64
	SwitchResistanceOhm      float64
}

type directCurrentSetpointSolution struct {
	NominalV            float64
	MinimumV            float64
	MaximumV            float64
	SetResistanceOhm    float64
	DesignCurrentA      float64
	OutputResistanceOhm float64
}

func solveDirectCurrentSetpoint(request directCurrentSetpointRequest) (directCurrentSetpointSolution, error) {
	if request.PassiveTolerancePercent <= 0 {
		request.PassiveTolerancePercent = currentRegulatorPassiveTolerance
	}
	requiredMinimum := request.CurrentA * (1 - request.TolerancePercent/100)
	if request.ReferenceCurrentA <= 0 || request.MaximumSetVoltageV < currentRegulatorDividerMinimumVoltageV || requiredMinimum <= request.ReferenceCurrentA {
		return directCurrentSetpointSolution{}, fmt.Errorf("constant-current envelope has no positive direct SET-resistor search interval")
	}
	accuracyCandidate := func(nominal float64) (directCurrentSetpointSolution, bool) {
		low, high := requiredMinimum, request.CurrentA
		if high <= request.ReferenceCurrentA {
			return directCurrentSetpointSolution{}, false
		}
		highMinimum, _ := directCurrentOutputBounds(request, nominal, high)
		if highMinimum < requiredMinimum {
			return directCurrentSetpointSolution{}, false
		}
		for iteration := 0; iteration < 64; iteration++ {
			mid := (low + high) / 2
			minimum, _ := directCurrentOutputBounds(request, nominal, mid)
			if minimum >= requiredMinimum {
				high = mid
			} else {
				low = mid
			}
		}
		high = math.Min(request.CurrentA, high+math.Max(request.CurrentA*1e-10, 1e-15))
		_, outputMaximum := directCurrentOutputBounds(request, nominal, high)
		requiredMaximum := request.CurrentA * (1 + request.TolerancePercent/100)
		if outputMaximum > requiredMaximum {
			return directCurrentSetpointSolution{}, false
		}
		setResistance := nominal / request.ReferenceCurrentA
		return directCurrentSetpointSolution{
			NominalV:         nominal,
			MinimumV:         request.ReferenceCurrentMinimumA * setResistance * (1 - request.PassiveTolerancePercent/100),
			MaximumV:         request.ReferenceCurrentMaximumA * setResistance * (1 + request.PassiveTolerancePercent/100),
			SetResistanceOhm: setResistance, DesignCurrentA: high,
			OutputResistanceOhm: nominal / (high - request.ReferenceCurrentA),
		}, true
	}
	if _, ok := accuracyCandidate(request.MaximumSetVoltageV); !ok {
		return directCurrentSetpointSolution{}, fmt.Errorf("constant-current envelope has no direct SET-resistor solution within tolerance, component limits, and compliance headroom")
	}
	low, high := currentRegulatorDividerMinimumVoltageV, request.MaximumSetVoltageV
	for iteration := 0; iteration < 64; iteration++ {
		mid := (low + high) / 2
		if _, ok := accuracyCandidate(mid); ok {
			high = mid
		} else {
			low = mid
		}
	}
	firstStep := math.Ceil((high-currentRegulatorDividerMinimumVoltageV)/currentRegulatorDividerStepVoltageV - 1e-12)
	for offset := 0.0; offset <= 2; offset++ {
		nominal := currentRegulatorDividerMinimumVoltageV + (firstStep+offset)*currentRegulatorDividerStepVoltageV
		if nominal > request.MaximumSetVoltageV {
			break
		}
		solution, ok := accuracyCandidate(nominal)
		if !ok {
			continue
		}
		calculation, err := currentRegulatorCalculation(currentRegulatorCalculationRequest{
			CurrentA: request.CurrentA, DesignCurrentA: solution.DesignCurrentA, TolerancePercent: request.TolerancePercent,
			ComplianceV: request.ComplianceV, InputMinimumV: request.InputMinimumV, InputMaximumV: request.InputMaximumV,
			ReferenceCurrentA: request.ReferenceCurrentA, ReferenceCurrentMinimumA: request.ReferenceCurrentMinimumA, ReferenceCurrentMaximumA: request.ReferenceCurrentMaximumA,
			OffsetVoltageMinimumV: request.OffsetVoltageMinimumV, OffsetVoltageMaximumV: request.OffsetVoltageMaximumV,
			SetVoltageNominalV: solution.NominalV, SetVoltageMinimumV: solution.MinimumV, SetVoltageMaximumV: solution.MaximumV, SetResistanceOhm: solution.SetResistanceOhm,
			OutputResistanceOhm: solution.OutputResistanceOhm, MinimumHeadroomV: request.MinimumHeadroomV, SwitchResistanceOhm: request.SwitchResistanceOhm,
		})
		if err == nil && calculation.Pass {
			return solution, nil
		}
	}
	return directCurrentSetpointSolution{}, fmt.Errorf("constant-current envelope has no direct SET-resistor solution within tolerance, component limits, and compliance headroom")
}

func directCurrentOutputBounds(request directCurrentSetpointRequest, nominal, designCurrent float64) (float64, float64) {
	setResistance := nominal / request.ReferenceCurrentA
	outputResistance := nominal / (designCurrent - request.ReferenceCurrentA)
	outputMinimum, outputMaximum := math.Inf(1), math.Inf(-1)
	tolerance := request.PassiveTolerancePercent / 100
	for _, referenceCurrent := range []float64{request.ReferenceCurrentMinimumA, request.ReferenceCurrentMaximumA} {
		for _, offset := range []float64{request.OffsetVoltageMinimumV, request.OffsetVoltageMaximumV} {
			for _, setScale := range []float64{1 - tolerance, 1 + tolerance} {
				for _, outputScale := range []float64{1 - tolerance, 1 + tolerance} {
					setVoltage := referenceCurrent * setResistance * setScale
					outputCurrent := (setVoltage+offset)/(outputResistance*outputScale) + referenceCurrent
					outputMinimum = math.Min(outputMinimum, outputCurrent)
					outputMaximum = math.Max(outputMaximum, outputCurrent)
				}
			}
		}
	}
	leakageUncertainty := (request.InputMaximumV + request.ComplianceV) / currentRegulatorLeakageOhm
	return outputMinimum - leakageUncertainty, outputMaximum + leakageUncertainty
}

type lowHeadroomSetpointRequest struct {
	CurrentA                      float64
	TolerancePercent              float64
	ComplianceV                   float64
	InputMinimumV                 float64
	InputMaximumV                 float64
	ReferenceCurrentA             float64
	ReferenceCurrentMinimumA      float64
	ReferenceCurrentMaximumA      float64
	OffsetVoltageMinimumV         float64
	OffsetVoltageMaximumV         float64
	ParallelOutputCurrentMaximumA float64
	ReferenceVoltageV             float64
	ReferenceMinimumV             float64
	ReferenceMaximumV             float64
	PassiveTolerancePercent       float64
	MinimumHeadroomV              float64
	SwitchResistanceOhm           float64
}

type lowHeadroomSetpointSolution struct {
	NominalV           float64
	MinimumV           float64
	MaximumV           float64
	UpperResistanceOhm float64
	DesignCurrentA     float64
}

func solveLowHeadroomSetpoint(request lowHeadroomSetpointRequest) (lowHeadroomSetpointSolution, error) {
	if request.PassiveTolerancePercent <= 0 {
		request.PassiveTolerancePercent = currentRegulatorPassiveTolerance
	}
	requiredMinimum := request.CurrentA * (1 - request.TolerancePercent/100)
	minimumNominal := math.Max(currentRegulatorDividerMinimumVoltageV, request.ReferenceCurrentA*currentRegulatorDividerLowerOhm+currentRegulatorDividerStepVoltageV)
	maximumNominal := request.ReferenceVoltageV - currentRegulatorDividerStepVoltageV
	if requiredMinimum <= request.ReferenceCurrentA || maximumNominal < minimumNominal {
		return lowHeadroomSetpointSolution{}, fmt.Errorf("constant-current envelope has no positive precision-divider search interval")
	}
	accuracyCandidate := func(nominal float64) (lowHeadroomSetpointSolution, bool) {
		denominator := nominal/currentRegulatorDividerLowerOhm - request.ReferenceCurrentA
		if denominator <= 0 {
			return lowHeadroomSetpointSolution{}, false
		}
		upperResistance := (request.ReferenceVoltageV - nominal) / denominator
		if !finitePositive(upperResistance) {
			return lowHeadroomSetpointSolution{}, false
		}
		minimum, maximum := precisionDividerSetpointBounds(request, upperResistance)
		low, high := requiredMinimum, request.CurrentA
		highMinimum, _ := lowHeadroomOutputBounds(request, nominal, minimum, maximum, high)
		if highMinimum < requiredMinimum {
			return lowHeadroomSetpointSolution{}, false
		}
		for iteration := 0; iteration < 64; iteration++ {
			mid := (low + high) / 2
			outputMinimum, _ := lowHeadroomOutputBounds(request, nominal, minimum, maximum, mid)
			if outputMinimum >= requiredMinimum {
				high = mid
			} else {
				low = mid
			}
		}
		high = math.Min(request.CurrentA, high+math.Max(request.CurrentA*1e-10, 1e-15))
		_, outputMaximum := lowHeadroomOutputBounds(request, nominal, minimum, maximum, high)
		if outputMaximum > request.CurrentA*(1+request.TolerancePercent/100) {
			return lowHeadroomSetpointSolution{}, false
		}
		return lowHeadroomSetpointSolution{
			NominalV: nominal, MinimumV: minimum, MaximumV: maximum,
			UpperResistanceOhm: upperResistance, DesignCurrentA: high,
		}, true
	}
	if _, ok := accuracyCandidate(maximumNominal); !ok {
		return lowHeadroomSetpointSolution{}, fmt.Errorf("constant-current envelope has no precision-divider solution within tolerance and compliance headroom")
	}
	low, high := minimumNominal, maximumNominal
	for iteration := 0; iteration < 64; iteration++ {
		mid := (low + high) / 2
		if _, ok := accuracyCandidate(mid); ok {
			high = mid
		} else {
			low = mid
		}
	}
	firstStep := math.Ceil((high-currentRegulatorDividerMinimumVoltageV)/currentRegulatorDividerStepVoltageV - 1e-12)
	for offset := 0.0; offset <= 2; offset++ {
		nominal := currentRegulatorDividerMinimumVoltageV + (firstStep+offset)*currentRegulatorDividerStepVoltageV
		if nominal > maximumNominal {
			break
		}
		solution, ok := accuracyCandidate(nominal)
		if !ok {
			continue
		}
		outputResistance := nominal / (solution.DesignCurrentA - request.ReferenceCurrentA)
		calculation, err := currentRegulatorCalculation(currentRegulatorCalculationRequest{
			CurrentA: request.CurrentA, DesignCurrentA: solution.DesignCurrentA, TolerancePercent: request.TolerancePercent,
			ComplianceV: request.ComplianceV, InputMinimumV: request.InputMinimumV, InputMaximumV: request.InputMaximumV,
			ReferenceCurrentA: request.ReferenceCurrentA, ReferenceCurrentMinimumA: request.ReferenceCurrentMinimumA, ReferenceCurrentMaximumA: request.ReferenceCurrentMaximumA,
			OffsetVoltageMinimumV: request.OffsetVoltageMinimumV, OffsetVoltageMaximumV: request.OffsetVoltageMaximumV,
			SetVoltageNominalV: nominal, SetVoltageMinimumV: solution.MinimumV, SetVoltageMaximumV: solution.MaximumV,
			OutputResistanceOhm: outputResistance, ParallelOutputCurrentMaximumA: request.ParallelOutputCurrentMaximumA, PrecisionMode: true,
			MinimumHeadroomV: request.MinimumHeadroomV, SwitchResistanceOhm: request.SwitchResistanceOhm,
		})
		if err == nil && calculation.Pass {
			return solution, nil
		}
	}
	return lowHeadroomSetpointSolution{}, fmt.Errorf("constant-current envelope has no precision-divider solution within tolerance and compliance headroom")
}

func lowHeadroomOutputBounds(request lowHeadroomSetpointRequest, nominal, minimumSetVoltage, maximumSetVoltage, designCurrent float64) (float64, float64) {
	outputResistance := nominal / (designCurrent - request.ReferenceCurrentA)
	outputMinimum, outputMaximum := math.Inf(1), math.Inf(-1)
	tolerance := request.PassiveTolerancePercent / 100
	for _, referenceCurrent := range []float64{request.ReferenceCurrentMinimumA, request.ReferenceCurrentMaximumA} {
		for _, offset := range []float64{request.OffsetVoltageMinimumV, request.OffsetVoltageMaximumV} {
			for _, setVoltage := range []float64{minimumSetVoltage, maximumSetVoltage} {
				for _, outputScale := range []float64{1 - tolerance, 1 + tolerance} {
					outputCurrent := (setVoltage+offset)/(outputResistance*outputScale) + referenceCurrent
					outputMinimum = math.Min(outputMinimum, outputCurrent)
					outputMaximum = math.Max(outputMaximum, outputCurrent)
				}
			}
		}
	}
	leakageUncertainty := (request.InputMaximumV + request.ComplianceV) / currentRegulatorLeakageOhm
	return outputMinimum - leakageUncertainty, outputMaximum + leakageUncertainty + request.ParallelOutputCurrentMaximumA
}

func precisionDividerSetpointBounds(request lowHeadroomSetpointRequest, upperResistance float64) (float64, float64) {
	minimum, maximum := math.Inf(1), math.Inf(-1)
	tolerance := request.PassiveTolerancePercent / 100
	for _, referenceVoltage := range []float64{request.ReferenceMinimumV, request.ReferenceMaximumV} {
		for _, referenceCurrent := range []float64{request.ReferenceCurrentMinimumA, request.ReferenceCurrentMaximumA} {
			for _, upperScale := range []float64{1 - tolerance, 1 + tolerance} {
				for _, lowerScale := range []float64{1 - tolerance, 1 + tolerance} {
					upper := upperResistance * upperScale
					lower := currentRegulatorDividerLowerOhm * lowerScale
					setpoint := (referenceVoltage/upper + referenceCurrent) / (1/upper + 1/lower)
					minimum = math.Min(minimum, setpoint)
					maximum = math.Max(maximum, setpoint)
				}
			}
		}
	}
	return minimum, maximum
}

func currentRegulatorCalculation(request currentRegulatorCalculationRequest) (CalculationEvidence, error) {
	if request.DesignCurrentA <= 0 {
		request.DesignCurrentA = request.CurrentA
	}
	if request.SetResistanceOhm <= 0 {
		request.SetResistanceOhm = currentRegulatorSetResistanceOhm
	}
	if request.PassiveTolerancePercent <= 0 {
		request.PassiveTolerancePercent = currentRegulatorPassiveTolerance
	}
	setTolerance := request.SetTolerancePercent
	if setTolerance <= 0 {
		setTolerance = request.PassiveTolerancePercent
	}
	outputTolerance := request.OutputTolerancePercent
	if outputTolerance <= 0 {
		outputTolerance = request.PassiveTolerancePercent
	}
	setTolerance /= 100
	outputTolerance /= 100
	referenceMinimum := request.ReferenceCurrentMinimumA
	referenceMaximum := request.ReferenceCurrentMaximumA
	if referenceMinimum <= 0 || referenceMaximum < referenceMinimum {
		referenceMinimum, referenceMaximum = request.ReferenceCurrentA, request.ReferenceCurrentA
	}
	outputMinimum, outputMaximum := math.Inf(1), math.Inf(-1)
	var corners []CornerEvidence
	for _, referenceCurrent := range []float64{referenceMinimum, referenceMaximum} {
		for _, offset := range []float64{request.OffsetVoltageMinimumV, request.OffsetVoltageMaximumV} {
			for _, setScale := range []float64{1 - setTolerance, 1 + setTolerance} {
				for _, outputScale := range []float64{1 - outputTolerance, 1 + outputTolerance} {
					setVoltage := request.SetVoltageNominalV
					if request.PrecisionMode {
						if setScale < 1 {
							setVoltage = request.SetVoltageMinimumV
						} else {
							setVoltage = request.SetVoltageMaximumV
						}
					} else {
						setVoltage = referenceCurrent * request.SetResistanceOhm * setScale
					}
					outputResistance := request.OutputResistanceOhm * outputScale
					outputCurrent := (setVoltage+offset)/outputResistance + referenceCurrent
					outputMinimum = math.Min(outputMinimum, outputCurrent)
					outputMaximum = math.Max(outputMaximum, outputCurrent)
					corners = append(corners, CornerEvidence{
						ID: fmt.Sprintf("reference_%0.12g_offset_%0.12g_set_%0.12g_output_%0.12g", referenceCurrent, offset, setScale, outputScale),
						Inputs: []NamedQuantity{
							{Name: "reference_current", Value: referenceCurrent, Unit: "A"},
							{Name: "offset_voltage", Value: offset, Unit: "V"},
							{Name: "set_voltage", Value: setVoltage, Unit: "V"},
							{Name: "output_resistance", Value: outputResistance, Unit: "Ohm"},
						},
						Outputs: []NamedQuantity{{Name: "output_current", Value: outputCurrent, Unit: "A"}},
					})
				}
			}
		}
	}
	leakageUncertainty := (request.InputMaximumV + request.ComplianceV) / currentRegulatorLeakageOhm
	outputMinimum += request.ParallelOutputCurrentMinimumA - leakageUncertainty
	outputMaximum += leakageUncertainty + request.ParallelOutputCurrentMaximumA
	requiredMinimum := request.CurrentA * (1 - request.TolerancePercent/100)
	requiredMaximum := request.CurrentA * (1 + request.TolerancePercent/100)
	worstOutputVoltage := request.ComplianceV
	if request.CurrentA > 0 {
		worstOutputVoltage *= outputMaximum / request.CurrentA
	}
	headroom := request.InputMinimumV - worstOutputVoltage - request.SetVoltageMaximumV - request.OffsetVoltageMaximumV - outputMaximum*request.SwitchResistanceOhm
	guardedHeadroom := headroom - currentRegulatorHeadroomGuardV
	bounds := []CalculationBound{
		minimumBound("output_current", requiredMinimum, outputMinimum, "A"),
		maximumBound("output_current", requiredMaximum, outputMaximum, "A"),
		minimumBound("compliance_headroom", request.MinimumHeadroomV, guardedHeadroom, "V"),
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	evidence := CalculationEvidence{
		ID: "constant_current_worst_case", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "target_current", Value: request.CurrentA, Unit: "A"},
			{Name: "minimum_input_voltage", Value: request.InputMinimumV, Unit: "V"},
			{Name: "minimum_compliance_voltage", Value: request.ComplianceV, Unit: "V"},
			{Name: "nominal_set_voltage", Value: request.SetVoltageNominalV, Unit: "V"},
			{Name: "set_program_resistance", Value: request.SetResistanceOhm, Unit: "Ohm"},
			{Name: "output_program_resistance", Value: request.OutputResistanceOhm, Unit: "Ohm"},
		},
		SelectedValues: request.SelectedValues,
		NominalOutputs: []NamedQuantity{{Name: "output_current", Value: request.DesignCurrentA, Unit: "A"}, {Name: "compliance_headroom", Value: headroom, Unit: "V"}},
		Corners:        corners, Bounds: bounds, CornerEvaluations: len(corners), WorstMargin: worstMargin, Pass: pass,
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, fmt.Errorf("constant-current calculation finalization failed: %w", err)
	}
	if !pass {
		return CalculationEvidence{}, fmt.Errorf("constant-current envelope is unproven: output %.12g..%.12g A, guarded headroom %.12g V", outputMinimum, outputMaximum, guardedHeadroom)
	}
	return finalized, nil
}

func boundedObservedCalculation(id, metric string, observed float64, unit string, constraints []Constraint) (CalculationEvidence, error) {
	required, _, err := requiredNumber(constraints, metric, "maximum", unit)
	if err != nil || required <= 0 {
		return CalculationEvidence{}, fmt.Errorf("%s requires a positive maximum bound", metric)
	}
	bounds := []CalculationBound{maximumBound(metric, required, observed, unit)}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	evidence := CalculationEvidence{
		ID: id, FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs:         []NamedQuantity{{Name: "required_" + metric, Value: required, Unit: unit}},
		NominalOutputs: []NamedQuantity{{Name: metric, Value: observed, Unit: unit}},
		Bounds:         bounds, WorstMargin: worstMargin, Pass: pass,
	}
	finalized, finalizeErr := FinalizeCalculation(evidence)
	if finalizeErr != nil {
		return CalculationEvidence{}, finalizeErr
	}
	if !pass {
		return CalculationEvidence{}, fmt.Errorf("%s %.12g %s exceeds required maximum %.12g %s", metric, observed, unit, required, unit)
	}
	return finalized, nil
}

func catalogSimulationParameterForModel(record components.ComponentRecord, modelID, name string) (float64, bool) {
	for _, model := range record.SimulationModels {
		if model.ModelID != modelID {
			continue
		}
		for _, parameter := range model.Parameters {
			if parameter.Name == name && finitePositive(parameter.Value) {
				return parameter.Value, true
			}
		}
	}
	return 0, false
}

func catalogSimulationParameterForModelNonNegative(record components.ComponentRecord, modelID, name string) (float64, bool) {
	for _, model := range record.SimulationModels {
		if model.ModelID != modelID {
			continue
		}
		for _, parameter := range model.Parameters {
			if parameter.Name == name && parameter.Value >= 0 && finiteNumbers(parameter.Value) {
				return parameter.Value, true
			}
		}
	}
	return 0, false
}

func catalogSimulationParameterAllowZero(record components.ComponentRecord, name string) (float64, bool) {
	for _, model := range record.SimulationModels {
		for _, parameter := range model.Parameters {
			if parameter.Name == name && finiteNumbers(parameter.Value) {
				return parameter.Value, true
			}
		}
	}
	return 0, false
}

func catalogUncertaintyBounds(record components.ComponentRecord, target string, nominal float64) (float64, float64) {
	for _, model := range record.SimulationModels {
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == target && finitePositive(uncertainty.Minimum) && finitePositive(uncertainty.Maximum) {
				return uncertainty.Minimum, uncertainty.Maximum
			}
		}
	}
	return nominal, nominal
}

func catalogUncertaintyInterval(record components.ComponentRecord, target string, nominal float64) (float64, float64) {
	for _, model := range record.SimulationModels {
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == target && finiteNumbers(uncertainty.Minimum, uncertainty.Maximum) && uncertainty.Minimum <= nominal && nominal <= uncertainty.Maximum {
				return uncertainty.Minimum, uncertainty.Maximum
			}
		}
	}
	return nominal, nominal
}
