package architecturesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	protectedConverterOutputCapacitanceF        = 1e-6
	protectedConverterOutputCapacitanceMaximumF = 1.2e-6
	protectedConverterInputCapacitanceF         = 1e-6
	protectedConverterMinimumOutputTolerancePct = 1.0
	efuseRampTimePerVoltPerFarad                = 8_000.0
	efuseOVPReferenceMinimumV                   = 1.17
	efuseOVPReferenceTypicalV                   = 1.19
	efuseOVPReferenceMaximumV                   = 1.225
	efuseMaximumExternalOVPV                    = 55.0
)

type protectedConverterInputProtection struct {
	efuse                   catalogPart
	upstreamBypass          catalogPart
	currentLimitResistor    catalogPart
	rampCapacitor           catalogPart
	ovpTopResistor          catalogPart
	ovpBottomResistor       catalogPart
	programmedCurrentLimitA float64
	minimumCurrentLimitA    float64
	maximumCurrentLimitA    float64
	maximumOutputSlewVPerS  float64
	ovpMinimumV             float64
	ovpMaximumV             float64
	inputDemandA            float64
	maximumOnResistanceOhm  float64
}

type protectedConverterPreregulation struct {
	regulator             catalogPart
	inputBypass           catalogPart
	outputVoltageV        float64
	outputVoltageMinimumV float64
	outputVoltageMaximumV float64
	outputCurrentDemandA  float64
	outputCurrentRatingA  float64
	inputMinimumV         float64
	inputMaximumV         float64
	efficiency            float64
	predictedJunctionC    float64
	maximumTemperatureC   float64
}

func protectedIsolatedConversionRequested(request ProviderRequest) bool {
	if optionalPositiveConstraint(request.Constraints, "maximum_inrush_current", "inrush_current") > 0 ||
		hasRoleContract(request.Ports, "shutdown") ||
		hasRoleContract(request.Ports, "enable") {
		return true
	}
	return slices.ContainsFunc(request.Constraints, func(constraint Constraint) bool {
		return constraint.Name == "shutdown_discharge_time" ||
			constraint.Name == "shutdown_discharge_voltage"
	})
}

func (provider *CatalogProvider) expandProtectedIsolatedConverter(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	output, tolerancePercent, ok := firstNumericConstraint(request.Constraints, "output_voltage")
	if !ok || output <= 0 {
		return nil, fmt.Errorf("protected isolated conversion requires a positive output-voltage target")
	}
	inputMinimum, inputMaximum, ok := roleVoltageRange(request.Ports, "input")
	if !ok {
		return nil, fmt.Errorf("protected isolated conversion requires a bounded input range")
	}
	outputCurrent := requiredRoleCurrentA(request.Ports, "output")
	if outputCurrent <= 0 {
		return nil, fmt.Errorf("protected isolated conversion requires an output-current contract")
	}
	isolationRequired := 1000.0
	if value, _, found := firstNumericConstraint(request.Constraints, "isolation_working_voltage", "isolation_voltage"); found && value > 0 {
		isolationRequired = value
	}
	inrushLimit := optionalPositiveConstraint(request.Constraints, "maximum_inrush_current", "inrush_current")
	shutdownRequested := slices.ContainsFunc(request.Ports, func(port RoleContract) bool {
		return port.Role == "shutdown" || port.Role == "enable"
	})
	var converter catalogPart
	var preregulation protectedConverterPreregulation
	ambientMaximum, _, ambientBounded := firstNumericConstraint(request.Constraints, "ambient_temperature")
	junctionMaximum, _, junctionBounded := firstNumericConstraint(request.Constraints, "junction_temperature", "peak_junction_temperature")
	selectedJunction := 0.0
	for _, record := range provider.familyRecords["isolated_converter"] {
		if record.Family != "isolated_converter" || !catalogRecordHasSimulationModel(record, simmodel.PrimitiveProtectedIsolatedConverterV1) {
			continue
		}
		fixedOutput, found := recordValue(record, "output_voltage", "V")
		allowedError := output * math.Max(tolerancePercent, protectedConverterMinimumOutputTolerancePct) / 100
		if !found || math.Abs(fixedOutput-output) > allowedError {
			continue
		}
		selected, err := provider.selectComponent(ctx, "isolated_converter", record.ID, []components.RequiredRating{
			{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
			{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
			{Kind: "output_current", Value: numericString(outputCurrent), Unit: "A"},
			{Kind: "isolation_working_voltage", Value: numericString(isolationRequired), Unit: "V"},
		}, true)
		if err != nil || selected.record.ID != record.ID {
			continue
		}
		if ambientBounded && junctionBounded {
			predicted, predictedOK := protectedConverterPredictedJunction(record, ambientMaximum, output*outputCurrent)
			if !predictedOK || predicted > junctionMaximum {
				continue
			}
			selectedJunction = predicted
		}
		if protectedConverterRequiresInputProtection(record, inrushLimit, shutdownRequested) {
			converterMinimum, minimumOK := catalogSimulationParameter(record, "input_min_v")
			if !minimumOK || converterMinimum >= inputMinimum {
				continue
			}
		}
		converter = selected
		break
	}
	if converter.record.ID == "" {
		var cascadeErr error
		converter, preregulation, selectedJunction, cascadeErr = provider.selectProtectedConverterCascade(
			ctx,
			provider.familyRecords["isolated_converter"],
			provider.familyRecords["regulator"],
			inputMinimum,
			inputMaximum,
			output,
			tolerancePercent,
			outputCurrent,
			isolationRequired,
			ambientMaximum,
			junctionMaximum,
			ambientBounded && junctionBounded,
		)
		if cascadeErr != nil {
			return nil, fmt.Errorf("no protected isolated converter covers the requested input, output, load, and working-voltage envelope: %w", cascadeErr)
		}
	}
	nativeInrushMaximum, nativeInrushMaximumProven := recordRatingMaximum(converter.record, "maximum_inrush_current", "A")
	converterHasRemote := recordHasFunction(converter.record, "REMOTE")
	requiresInputProtection := inrushLimit > 0 && (!nativeInrushMaximumProven || nativeInrushMaximum > inrushLimit)
	if shutdownRequested && !converterHasRemote {
		requiresInputProtection = true
	}
	if requiresInputProtection && inrushLimit <= 0 {
		return nil, fmt.Errorf("protected isolated converter without remote control requires a positive bounded input-current limit")
	}
	dischargeTime, dischargeVoltage, dischargeRequired, err := protectedConverterDischargeRequirement(request.Constraints, output)
	if err != nil {
		return nil, err
	}
	converter.selected.InstanceID, converter.usage = "protected_isolated_converter", "protected_isolated_power_stage"
	inputBypass, err := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", "1u", []components.RequiredRating{{
		Kind: "voltage", Value: numericString(inputMaximum / catalogRatingDeratingFactor), Unit: "V",
	}})
	if err != nil {
		return nil, fmt.Errorf("select voltage-qualified protected-converter input bypass: %w", err)
	}
	inputBypass.selected.InstanceID, inputBypass.usage, inputBypass.value = "converter_input_bypass", "input_bypass_capacitor", "1u"
	outputBypass, err := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", "1u", []components.RequiredRating{{
		Kind: "voltage", Value: numericString(output / catalogRatingDeratingFactor), Unit: "V",
	}})
	if err != nil {
		return nil, fmt.Errorf("select voltage-qualified protected-converter output bypass: %w", err)
	}
	outputBypass.selected.InstanceID, outputBypass.usage, outputBypass.value = "converter_output_bypass", "low_esr_output_capacitor", "1u"
	parts := []catalogPart{converter, inputBypass, outputBypass}
	if preregulation.regulator.record.ID != "" {
		parts = append(parts, preregulation.regulator, preregulation.inputBypass)
	}
	var inputProtection protectedConverterInputProtection
	if requiresInputProtection {
		inputDemand, demandOK := protectedConverterInputDemand(
			converter.record,
			preregulation,
			inputMinimum,
			output*outputCurrent,
		)
		if !demandOK {
			return nil, fmt.Errorf("protected isolated converter lacks a bounded end-to-end efficiency path")
		}
		inputProtection, err = provider.selectProtectedConverterInputProtection(
			ctx, inputMinimum, inputMaximum, inputDemand, inrushLimit,
		)
		if err != nil {
			return nil, err
		}
		parts = append(parts,
			inputProtection.efuse,
			inputProtection.upstreamBypass,
			inputProtection.currentLimitResistor,
			inputProtection.rampCapacitor,
			inputProtection.ovpTopResistor,
			inputProtection.ovpBottomResistor,
		)
	}
	if converterHasRemote {
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{
			{"converter_remote_pullup", "resistor", "startup_inactive", "100k"},
		})
		if err != nil {
			return nil, err
		}
	}
	var dischargeResistance float64
	if dischargeRequired {
		discharge, resistance, selectErr := provider.selectProtectedConverterDischargeResistor(
			ctx, output, dischargeVoltage, dischargeTime,
		)
		if selectErr != nil {
			return nil, selectErr
		}
		parts = append(parts, discharge)
		dischargeResistance = resistance
	}
	bindings := []RealizationPortBinding{}
	for _, port := range request.Ports {
		switch port.Role {
		case "input":
			if requiresInputProtection {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: inputProtection.efuse.selected.InstanceID, Function: "VIN"})
			} else {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: converter.selected.InstanceID, Function: "VIN_PLUS"})
			}
		case "output":
			bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: converter.selected.InstanceID, Function: "VOUT_PLUS"})
		case "reference":
			bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: converter.selected.InstanceID, Function: "VOUT_MINUS"})
		case "shutdown", "enable":
			if requiresInputProtection {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: inputProtection.efuse.selected.InstanceID, Function: "SHDN"})
			} else if converterHasRemote {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: converter.selected.InstanceID, Function: "REMOTE"})
			}
		}
	}
	if requiresInputProtection {
		bindings = append(bindings, RealizationPortBinding{Role: "input", Lane: "return", Instance: inputProtection.efuse.selected.InstanceID, Function: "RTN"})
	} else {
		bindings = append(bindings, RealizationPortBinding{Role: "input", Lane: "return", Instance: converter.selected.InstanceID, Function: "VIN_MINUS"})
	}
	inputPowerEndpoints := []RealizationEndpoint{endpoint(converter, "VIN_PLUS"), passiveEndpoint("converter_input_bypass", "A")}
	inputReturnEndpoints := []RealizationEndpoint{endpoint(converter, "VIN_MINUS"), passiveEndpoint("converter_input_bypass", "B")}
	if preregulation.regulator.record.ID != "" {
		inputPowerEndpoints = append(inputPowerEndpoints, endpoint(preregulation.regulator, "VOUT"))
		inputReturnEndpoints = append(inputReturnEndpoints,
			endpoint(preregulation.regulator, "GND"),
			passiveEndpoint(preregulation.inputBypass.selected.InstanceID, "B"),
		)
	}
	if recordHasFunction(converter.record, "VIN_MINUS_AUX") {
		inputReturnEndpoints = append(inputReturnEndpoints, endpoint(converter, "VIN_MINUS_AUX"))
	}
	if converterHasRemote {
		inputPowerEndpoints = append(inputPowerEndpoints, passiveEndpoint("converter_remote_pullup", "A"))
	}
	connections := []RealizationConnection{
		semanticNet("protected_converter_input", "power", inputPowerEndpoints...),
		semanticNet("protected_converter_input_return", "reference", inputReturnEndpoints...),
		semanticNet("protected_converter_output", "power", endpoint(converter, "VOUT_PLUS"), passiveEndpoint("converter_output_bypass", "A")),
		semanticNet("protected_converter_output_return", "reference", endpoint(converter, "VOUT_MINUS"), passiveEndpoint("converter_output_bypass", "B")),
	}
	if converterHasRemote {
		connections = append(connections,
			semanticNet("protected_converter_remote", "control", endpoint(converter, "REMOTE"), passiveEndpoint("converter_remote_pullup", "B")),
		)
	}
	if requiresInputProtection {
		if preregulation.regulator.record.ID == "" {
			connections[0].Endpoints = append(connections[0].Endpoints,
				endpoint(inputProtection.efuse, "VOUT"),
				endpoint(inputProtection.efuse, "VOUT_AUX"),
			)
		}
		connections[1].Endpoints = append(connections[1].Endpoints,
			endpoint(inputProtection.efuse, "RTN"),
			endpoint(inputProtection.efuse, "GND"),
			endpoint(inputProtection.efuse, "RTN_EP"),
			endpoint(inputProtection.efuse, "UVLO"),
			endpoint(inputProtection.efuse, "MODE"),
			passiveEndpoint("converter_input_efuse_bypass", "B"),
			passiveEndpoint("converter_input_current_limit", "B"),
			passiveEndpoint("converter_input_ramp", "B"),
			passiveEndpoint("converter_input_ovp_bottom", "B"),
		)
		connections = append(connections,
			semanticNet("protected_converter_upstream_input", "power",
				endpoint(inputProtection.efuse, "VIN"),
				endpoint(inputProtection.efuse, "VIN_AUX"),
				passiveEndpoint("converter_input_efuse_bypass", "A"),
				passiveEndpoint("converter_input_ovp_top", "A"),
			),
			semanticNet("protected_converter_current_limit", "control",
				endpoint(inputProtection.efuse, "ILIM"),
				passiveEndpoint("converter_input_current_limit", "A"),
			),
			semanticNet("protected_converter_output_ramp", "control",
				endpoint(inputProtection.efuse, "DVDT"),
				passiveEndpoint("converter_input_ramp", "A"),
			),
			semanticNet("protected_converter_overvoltage_threshold", "control",
				endpoint(inputProtection.efuse, "OVP"),
				passiveEndpoint("converter_input_ovp_top", "B"),
				passiveEndpoint("converter_input_ovp_bottom", "A"),
			),
		)
		if preregulation.regulator.record.ID != "" {
			connections = append(connections,
				semanticNet("protected_converter_pre_regulator_input", "power",
					endpoint(inputProtection.efuse, "VOUT"),
					endpoint(inputProtection.efuse, "VOUT_AUX"),
					endpoint(preregulation.regulator, "VIN"),
					passiveEndpoint(preregulation.inputBypass.selected.InstanceID, "A"),
				),
			)
		}
	}
	if dischargeRequired {
		connections[2].Endpoints = append(connections[2].Endpoints, RealizationEndpoint{Instance: "converter_output_discharge", Function: "A"})
		connections[3].Endpoints = append(connections[3].Endpoints, RealizationEndpoint{Instance: "converter_output_discharge", Function: "B"})
	}
	outputRated, _ := recordRatingMaximum(converter.record, "output_current", "A")
	isolationRated, _ := recordRatingMaximum(converter.record, "isolation_working_voltage", "V")
	shortRated, _ := recordRatingMaximum(converter.record, "short_circuit_current", "A")
	inputBypassRated, _ := recordRatingMaximum(inputBypass.record, "voltage", "V")
	outputBypassRated, _ := recordRatingMaximum(outputBypass.record, "voltage", "V")
	bounds := []CalculationBound{
		minimumBound("output_current", outputCurrent, outputRated, "A"),
		minimumBound("isolation_working_voltage", isolationRequired, isolationRated, "V"),
		minimumBound("short_circuit_limit", outputCurrent, shortRated, "A"),
		minimumBound("input_bypass_voltage", inputMaximum, catalogRatingDeratingFactor*inputBypassRated, "V"),
		minimumBound("output_bypass_voltage", output, catalogRatingDeratingFactor*outputBypassRated, "V"),
	}
	nominalOutputs := []NamedQuantity{
		{Name: "short_circuit_protected", Value: 1, Unit: "bool"},
		{Name: "remote_shutdown", Value: 1, Unit: "bool"},
	}
	if preregulation.regulator.record.ID != "" {
		converterInputMinimum, _ := catalogSimulationParameter(converter.record, "input_min_v")
		converterInputMaximum, _ := catalogSimulationParameter(converter.record, "input_max_v")
		preregulatorBypassRated, _ := recordRatingMaximum(preregulation.inputBypass.record, "voltage", "V")
		nominalOutputs = append(nominalOutputs,
			NamedQuantity{Name: "pre_regulator_output_voltage", Value: preregulation.outputVoltageV, Unit: "V"},
			NamedQuantity{Name: "pre_regulator_predicted_junction_temperature", Value: preregulation.predictedJunctionC, Unit: "degC"},
		)
		bounds = append(bounds,
			minimumBound("pre_regulator_output_current", preregulation.outputCurrentDemandA, preregulation.outputCurrentRatingA, "A"),
			minimumBound("isolated_converter_input_minimum", converterInputMinimum, preregulation.outputVoltageMinimumV, "V"),
			maximumBound("isolated_converter_input_maximum", converterInputMaximum, preregulation.outputVoltageMaximumV, "V"),
			maximumBound("pre_regulator_maximum_temperature", preregulation.maximumTemperatureC, preregulation.predictedJunctionC, "degC"),
			minimumBound("pre_regulator_input_bypass_voltage", inputMaximum, catalogRatingDeratingFactor*preregulatorBypassRated, "V"),
		)
	}
	if ambientBounded && junctionBounded {
		nominalOutputs = append(nominalOutputs, NamedQuantity{Name: "predicted_junction_temperature", Value: selectedJunction, Unit: "degC"})
		bounds = append(bounds, maximumBound("junction_temperature", junctionMaximum, selectedJunction, "degC"))
	}
	provenInrushMaximum := nativeInrushMaximum
	if requiresInputProtection {
		provenInrushMaximum = inputProtection.maximumCurrentLimitA
		upstreamBypassRated, _ := recordRatingMaximum(inputProtection.upstreamBypass.record, "voltage", "V")
		capacitiveInrushMaximum := protectedConverterInputCapacitanceF * inputProtection.maximumOutputSlewVPerS
		nominalOutputs = append(nominalOutputs,
			NamedQuantity{Name: "programmed_input_current_limit", Value: inputProtection.programmedCurrentLimitA, Unit: "A"},
			NamedQuantity{Name: "minimum_input_current_limit", Value: inputProtection.minimumCurrentLimitA, Unit: "A"},
			NamedQuantity{Name: "maximum_inrush_current", Value: inputProtection.maximumCurrentLimitA, Unit: "A"},
			NamedQuantity{Name: "capacitive_inrush_current", Value: capacitiveInrushMaximum, Unit: "A"},
			NamedQuantity{Name: "minimum_overvoltage_cutoff", Value: inputProtection.ovpMinimumV, Unit: "V"},
			NamedQuantity{Name: "maximum_overvoltage_cutoff", Value: inputProtection.ovpMaximumV, Unit: "V"},
		)
		bounds = append(bounds,
			minimumBound("input_current_limit_capacity", inputProtection.inputDemandA, inputProtection.minimumCurrentLimitA, "A"),
			maximumBound("current_limited_inrush", inrushLimit, inputProtection.maximumCurrentLimitA, "A"),
			maximumBound("slew_limited_capacitive_inrush", inrushLimit, capacitiveInrushMaximum, "A"),
			minimumBound("overvoltage_operating_margin", inputMaximum, inputProtection.ovpMinimumV, "V"),
			maximumBound("external_overvoltage_setting", efuseMaximumExternalOVPV, inputProtection.ovpMaximumV, "V"),
			minimumBound("efuse_input_bypass_voltage", inputMaximum, catalogRatingDeratingFactor*upstreamBypassRated, "V"),
		)
		if preregulation.regulator.record.ID != "" {
			protectedInputMinimum := inputMinimum - inputProtection.maximumCurrentLimitA*inputProtection.maximumOnResistanceOhm
			nominalOutputs = append(nominalOutputs,
				NamedQuantity{Name: "minimum_protected_pre_regulator_input", Value: protectedInputMinimum, Unit: "V"},
			)
			bounds = append(bounds,
				minimumBound("pre_regulator_input_headroom", preregulation.inputMinimumV, protectedInputMinimum, "V"),
			)
		}
	} else if nativeInrushMaximumProven {
		nominalOutputs = append(nominalOutputs, NamedQuantity{Name: "maximum_inrush_current", Value: nativeInrushMaximum, Unit: "A"})
	}
	if inrushLimit > 0 && !requiresInputProtection {
		bounds = append(bounds, maximumBound("maximum_inrush_current", inrushLimit, provenInrushMaximum, "A"))
	}
	if dischargeRequired {
		remainingVoltage := output * math.Exp(-dischargeTime/(dischargeResistance*protectedConverterOutputCapacitanceMaximumF))
		nominalOutputs = append(nominalOutputs,
			NamedQuantity{Name: "output_discharge_resistance", Value: dischargeResistance, Unit: "Ohm"},
			NamedQuantity{Name: "shutdown_output_voltage", Value: remainingVoltage, Unit: "V"},
		)
		bounds = append(bounds, maximumBound("shutdown_output_voltage", dischargeVoltage, remainingVoltage, "V"))
	}
	calculation := CalculationEvidence{
		ID: "protected_isolated_converter_bounds", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "input_minimum", Value: inputMinimum, Unit: "V"}, {Name: "input_maximum", Value: inputMaximum, Unit: "V"},
			{Name: "output_voltage", Value: output, Unit: "V"}, {Name: "output_current", Value: outputCurrent, Unit: "A"},
			{Name: "isolation_working_voltage", Value: isolationRequired, Unit: "V"},
		},
		NominalOutputs: nominalOutputs,
		Bounds:         bounds,
		Pass:           true,
	}
	calculation, err = FinalizeCalculation(calculation)
	if err != nil {
		return nil, err
	}
	return provider.expansion(request, "protected_wide_input_isolated_converter", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func protectedConverterRequiresInputProtection(
	record components.ComponentRecord,
	inrushLimit float64,
	shutdownRequested bool,
) bool {
	nativeInrushMaximum, nativeInrushMaximumProven := recordRatingMaximum(record, "maximum_inrush_current", "A")
	return (inrushLimit > 0 && (!nativeInrushMaximumProven || nativeInrushMaximum > inrushLimit)) ||
		(shutdownRequested && !recordHasFunction(record, "REMOTE"))
}

func protectedConverterInputDemand(
	converter components.ComponentRecord,
	preregulation protectedConverterPreregulation,
	inputMinimum, outputPower float64,
) (float64, bool) {
	converterEfficiency, converterEfficiencyOK := recordMinimumEfficiency(converter)
	if !converterEfficiencyOK || !finitePositive(converterEfficiency) || converterEfficiency > 1 ||
		!finitePositive(inputMinimum) || !finitePositive(outputPower) {
		return 0, false
	}
	efficiency := converterEfficiency
	if preregulation.regulator.record.ID != "" {
		if !finitePositive(preregulation.efficiency) || preregulation.efficiency > 1 {
			return 0, false
		}
		efficiency *= preregulation.efficiency
	}
	demand := outputPower / (efficiency * inputMinimum)
	return demand, finitePositive(demand)
}

func (provider *CatalogProvider) selectProtectedConverterCascade(
	ctx context.Context,
	converterRecords, regulatorRecords []components.ComponentRecord,
	inputMinimum, inputMaximum, output, tolerancePercent, outputCurrent, isolationRequired float64,
	ambientMaximum, junctionMaximum float64,
	thermalBounded bool,
) (catalogPart, protectedConverterPreregulation, float64, error) {
	var lastErr error
	for _, converterRecord := range converterRecords {
		if converterRecord.Family != "isolated_converter" ||
			!catalogRecordHasSimulationModel(converterRecord, simmodel.PrimitiveProtectedIsolatedConverterV1) {
			continue
		}
		fixedOutput, found := recordValue(converterRecord, "output_voltage", "V")
		allowedError := output * math.Max(tolerancePercent, 1) / 100
		if !found || math.Abs(fixedOutput-output) > allowedError {
			continue
		}
		converter, selectErr := provider.selectComponent(ctx, "isolated_converter", converterRecord.ID, []components.RequiredRating{
			{Kind: "output_current", Value: numericString(outputCurrent), Unit: "A"},
			{Kind: "isolation_working_voltage", Value: numericString(isolationRequired), Unit: "V"},
		}, true)
		if selectErr != nil || converter.record.ID != converterRecord.ID {
			continue
		}
		selectedJunction := 0.0
		if thermalBounded {
			predicted, predictedOK := protectedConverterPredictedJunction(converterRecord, ambientMaximum, output*outputCurrent)
			if !predictedOK || predicted > junctionMaximum {
				continue
			}
			selectedJunction = predicted
		}
		converterInputMinimum, minimumOK := catalogSimulationParameter(converterRecord, "input_min_v")
		converterInputMaximum, maximumOK := catalogSimulationParameter(converterRecord, "input_max_v")
		converterEfficiency, efficiencyOK := recordMinimumEfficiency(converterRecord)
		if !minimumOK || !maximumOK || !efficiencyOK || !finitePositive(converterEfficiency) {
			continue
		}
		for _, regulatorRecord := range regulatorRecords {
			if regulatorRecord.Family != "regulator" ||
				!catalogRecordHasSimulationModel(regulatorRecord, simmodel.PrimitiveFixedBuckModuleV1) {
				continue
			}
			regulatorInputMinimum, regulatorMinimumOK := catalogSimulationParameter(regulatorRecord, "input_min_v")
			regulatorInputMaximum, regulatorMaximumOK := catalogSimulationParameter(regulatorRecord, "input_max_v")
			regulatorOutput, outputOK := catalogSimulationParameter(regulatorRecord, "output_voltage_v")
			regulatorEfficiency, regulatorEfficiencyOK := recordMinimumEfficiency(regulatorRecord)
			outputAccuracyPercent, accuracyOK := recordRatingMaximum(regulatorRecord, "output_voltage_accuracy", "%")
			outputCurrentRating, currentRatingOK := recordRatingMaximum(regulatorRecord, "output_current", "A")
			if !regulatorMinimumOK || !regulatorMaximumOK || !outputOK || !regulatorEfficiencyOK ||
				!accuracyOK || !currentRatingOK ||
				inputMinimum <= regulatorInputMinimum || inputMaximum > regulatorInputMaximum {
				continue
			}
			regulatorOutputMinimum := regulatorOutput * (1 - outputAccuracyPercent/100)
			regulatorOutputMaximum := regulatorOutput * (1 + outputAccuracyPercent/100)
			if !finitePositive(regulatorOutputMinimum) ||
				regulatorOutputMinimum < converterInputMinimum || regulatorOutputMaximum > converterInputMaximum {
				continue
			}
			regulatorOutputDemand := output * outputCurrent / (converterEfficiency * regulatorOutputMinimum)
			if !finitePositive(regulatorOutputDemand) || regulatorOutputDemand > outputCurrentRating {
				continue
			}
			regulator, regulatorErr := provider.selectComponent(ctx, "regulator", regulatorRecord.ID, []components.RequiredRating{
				{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
				{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
				{Kind: "output_current", Value: numericString(regulatorOutputDemand), Unit: "A"},
			}, true)
			if regulatorErr != nil || regulator.record.ID != regulatorRecord.ID {
				continue
			}
			regulatorPredicted, regulatorThermalOK := protectedConverterPredictedJunction(
				regulatorRecord,
				ambientMaximum,
				output*outputCurrent/converterEfficiency,
			)
			if !regulatorThermalOK {
				continue
			}
			regulatorMaximumTemperature, temperatureOK := catalogSimulationParameter(regulatorRecord, "max_temperature_c")
			if !temperatureOK {
				continue
			}
			inputBypass, capacitorErr := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", "22u", []components.RequiredRating{{
				Kind: "voltage", Value: numericString(inputMaximum / catalogRatingDeratingFactor), Unit: "V",
			}})
			if capacitorErr != nil {
				lastErr = fmt.Errorf("select voltage-qualified fixed-module input bypass: %w", capacitorErr)
				continue
			}
			converter.selected.InstanceID, converter.usage = "protected_isolated_converter", "protected_isolated_power_stage"
			regulator.selected.InstanceID, regulator.usage = "converter_pre_regulator", "synchronous_buck_controller"
			inputBypass.selected.InstanceID, inputBypass.usage, inputBypass.value =
				"converter_pre_regulator_input_bypass", "input_bypass_capacitor", "22u"
			inputBypass.near = regulator.selected.InstanceID
			inputBypass.maxDistanceMM = 5
			return converter, protectedConverterPreregulation{
				regulator:             regulator,
				inputBypass:           inputBypass,
				outputVoltageV:        regulatorOutput,
				outputVoltageMinimumV: regulatorOutputMinimum,
				outputVoltageMaximumV: regulatorOutputMaximum,
				outputCurrentDemandA:  regulatorOutputDemand,
				outputCurrentRatingA:  outputCurrentRating,
				inputMinimumV:         regulatorInputMinimum,
				inputMaximumV:         regulatorInputMaximum,
				efficiency:            regulatorEfficiency,
				predictedJunctionC:    regulatorPredicted,
				maximumTemperatureC:   regulatorMaximumTemperature,
			}, selectedJunction, nil
		}
	}
	if lastErr != nil {
		return catalogPart{}, protectedConverterPreregulation{}, 0, lastErr
	}
	return catalogPart{}, protectedConverterPreregulation{}, 0, fmt.Errorf("no reviewed fixed step-down cascade provides converter input headroom")
}

func protectedConverterPredictedJunction(
	record components.ComponentRecord,
	ambientC, outputPowerW float64,
) (float64, bool) {
	efficiency, efficiencyOK := recordMinimumEfficiency(record)
	thermalResistance, thermalOK := catalogSimulationParameter(record, "junction_to_ambient_c_per_w")
	maximumTemperature, maximumOK := catalogSimulationParameter(record, "max_temperature_c")
	if !efficiencyOK || !thermalOK || !maximumOK ||
		!finitePositive(efficiency) || efficiency > 1 ||
		!finitePositive(thermalResistance) || !finitePositive(outputPowerW) {
		return 0, false
	}
	predicted := ambientC + outputPowerW*(1/efficiency-1)*thermalResistance
	return predicted, finiteNumbers(predicted) && predicted <= maximumTemperature
}

func (provider *CatalogProvider) selectProtectedConverterInputProtection(
	ctx context.Context,
	inputMinimum, inputMaximum, inputDemand, inrushLimit float64,
) (protectedConverterInputProtection, error) {
	if !finitePositive(inrushLimit) {
		return protectedConverterInputProtection{}, fmt.Errorf("protected-converter input protection requires a positive inrush-current bound")
	}
	if !finitePositive(inputDemand) {
		return protectedConverterInputProtection{}, fmt.Errorf("protected-converter input-current demand is not finite and positive")
	}

	var efuse catalogPart
	for _, record := range provider.familyRecords["protection"] {
		if record.Family != "protection" || !catalogRecordHasSimulationModel(record, simmodel.PrimitiveCurrentLimitingEFuseV1) {
			continue
		}
		minimum, minimumOK := catalogSimulationParameter(record, "input_min_v")
		maximum, maximumOK := catalogSimulationParameter(record, "input_max_v")
		programmableMaximum, programmableOK := recordRatingMaximum(record, "programmable_current_limit", "A")
		if !minimumOK || !maximumOK || !programmableOK ||
			inputMinimum < minimum || inputMaximum > maximum ||
			inputDemand > programmableMaximum || inrushLimit > programmableMaximum {
			continue
		}
		selected, selectErr := provider.selectComponent(ctx, "protection", record.ID, nil, true)
		if selectErr == nil && selected.record.ID == record.ID {
			efuse = selected
			break
		}
	}
	if efuse.record.ID == "" {
		return protectedConverterInputProtection{}, fmt.Errorf(
			"no reviewed programmable input-protection device covers %.12g..%.12g V and a %.12g A inrush bound",
			inputMinimum, inputMaximum, inrushLimit,
		)
	}
	efuse.selected.InstanceID, efuse.usage = "converter_input_efuse", "overcurrent_limit"
	maximumOnResistance, resistanceOK := catalogSimulationParameter(efuse.record, "on_resistance_ohm")
	if !resistanceOK || !finitePositive(maximumOnResistance) {
		return protectedConverterInputProtection{}, fmt.Errorf("selected protected-converter input device lacks a bounded on-resistance")
	}
	programmingConstant, constantOK := recordValue(efuse.record, "current_limit_programming_constant", "A*Ohm")
	programmedReference, programmedOK := catalogSimulationParameter(efuse.record, "programmed_current_limit_a")
	minimumReference, minimumOK := catalogSimulationParameter(efuse.record, "minimum_current_limit_a")
	maximumReference, maximumOK := catalogSimulationParameter(efuse.record, "maximum_current_limit_a")
	if !constantOK || !finitePositive(programmingConstant) ||
		!programmedOK || !finitePositive(programmedReference) ||
		!minimumOK || !finitePositive(minimumReference) ||
		!maximumOK || !finitePositive(maximumReference) ||
		minimumReference > programmedReference || programmedReference > maximumReference {
		return protectedConverterInputProtection{}, fmt.Errorf(
			"selected protected-converter input device lacks a valid catalog-backed current-limit programming relationship",
		)
	}
	minimumRatio := minimumReference / programmedReference
	maximumRatio := maximumReference / programmedReference

	currentLimitResistor, programmedCurrent, minimumCurrent, maximumCurrent, err :=
		provider.selectProtectedConverterCurrentLimit(
			ctx, inputDemand, inrushLimit, programmingConstant, minimumRatio, maximumRatio,
		)
	if err != nil {
		return protectedConverterInputProtection{}, err
	}
	rampCapacitor, maximumSlew, err := provider.selectProtectedConverterRampCapacitor(ctx, inrushLimit)
	if err != nil {
		return protectedConverterInputProtection{}, err
	}
	ovpTop, ovpBottom, ovpMinimum, ovpMaximum, err :=
		provider.selectProtectedConverterOVPDivider(ctx, inputMaximum)
	if err != nil {
		return protectedConverterInputProtection{}, err
	}
	upstreamBypass, err := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", "1u", []components.RequiredRating{{
		Kind: "voltage", Value: numericString(inputMaximum / catalogRatingDeratingFactor), Unit: "V",
	}})
	if err != nil {
		return protectedConverterInputProtection{}, fmt.Errorf("select voltage-qualified eFuse input bypass: %w", err)
	}
	upstreamBypass.selected.InstanceID, upstreamBypass.usage, upstreamBypass.value =
		"converter_input_efuse_bypass", "input_bypass_capacitor", "1u"

	efuse.parameters = append(efuse.parameters,
		RealizationParameter{Name: "programmed_current_limit_a", Value: programmedCurrent, Unit: "A"},
		RealizationParameter{Name: "minimum_current_limit_a", Value: minimumCurrent, Unit: "A"},
		RealizationParameter{Name: "maximum_current_limit_a", Value: maximumCurrent, Unit: "A"},
		RealizationParameter{Name: "maximum_output_slew_v_per_s", Value: maximumSlew, Unit: "V/s"},
	)
	for _, part := range []*catalogPart{&upstreamBypass, &currentLimitResistor, &rampCapacitor, &ovpTop, &ovpBottom} {
		part.near = efuse.selected.InstanceID
		part.maxDistanceMM = 5
	}
	return protectedConverterInputProtection{
		efuse: efuse, upstreamBypass: upstreamBypass,
		currentLimitResistor: currentLimitResistor, rampCapacitor: rampCapacitor,
		ovpTopResistor: ovpTop, ovpBottomResistor: ovpBottom,
		programmedCurrentLimitA: programmedCurrent,
		minimumCurrentLimitA:    minimumCurrent,
		maximumCurrentLimitA:    maximumCurrent,
		maximumOutputSlewVPerS:  maximumSlew,
		ovpMinimumV:             ovpMinimum,
		ovpMaximumV:             ovpMaximum,
		inputDemandA:            inputDemand,
		maximumOnResistanceOhm:  maximumOnResistance,
	}, nil
}

func (provider *CatalogProvider) selectProtectedConverterCurrentLimit(
	ctx context.Context,
	requiredCurrent, maximumCurrent, programmingConstant, minimumRatio, maximumRatio float64,
) (catalogPart, float64, float64, float64, error) {
	if !finitePositive(programmingConstant) || !finitePositive(minimumRatio) ||
		!finitePositive(maximumRatio) || minimumRatio > 1 || maximumRatio < 1 {
		return catalogPart{}, 0, 0, 0, fmt.Errorf("programmable current-limit relationship is invalid")
	}
	minimumNominal := requiredCurrent / minimumRatio
	maximumNominal := maximumCurrent / maximumRatio
	if minimumNominal >= maximumNominal {
		return catalogPart{}, 0, 0, 0, fmt.Errorf(
			"no programmable current-limit tolerance envelope can supply %.12g A while bounding inrush to %.12g A",
			requiredCurrent, maximumCurrent,
		)
	}
	targetNominal := (minimumNominal + maximumNominal) / 2
	idealResistance := programmingConstant / targetNominal
	candidates, err := provider.preferredResistanceCandidates(ctx, idealResistance, 5, 0, DefaultMaxValueCandidates)
	if err != nil {
		return catalogPart{}, 0, 0, 0, fmt.Errorf("select protected-converter current-limit resistance: %w", err)
	}
	for _, resistance := range candidates {
		part, selectErr := provider.selectPassiveComponent(
			ctx, "resistor", "resistance", engineeringValue(resistance, "Ohm"),
		)
		if selectErr != nil {
			continue
		}
		tolerancePercent, toleranceOK := catalogToleranceMaximum(part.record, "resistance", "%")
		if !toleranceOK || tolerancePercent < 0 || tolerancePercent >= 100 {
			continue
		}
		tolerance := tolerancePercent / 100
		programmed := programmingConstant / resistance
		minimum := programmed * minimumRatio / (1 + tolerance)
		maximum := programmed * maximumRatio / (1 - tolerance)
		if minimum < requiredCurrent || maximum > maximumCurrent {
			continue
		}
		part.selected.InstanceID, part.usage, part.value =
			"converter_input_current_limit", "overcurrent_limit", engineeringValue(resistance, "Ohm")
		return part, programmed, minimum, maximum, nil
	}
	return catalogPart{}, 0, 0, 0, fmt.Errorf(
		"no catalog-backed current-limit resistor supplies %.12g A while bounding inrush to %.12g A",
		requiredCurrent, maximumCurrent,
	)
}

func (provider *CatalogProvider) selectProtectedConverterRampCapacitor(
	ctx context.Context,
	maximumInrushCurrent float64,
) (catalogPart, float64, error) {
	minimumCapacitance := protectedConverterInputCapacitanceF /
		(efuseRampTimePerVoltPerFarad * maximumInrushCurrent)
	candidates, issues := PreferredValueCandidates(
		minimumCapacitance*1.25, SeriesE12,
		minimumCapacitance, minimumCapacitance*100,
		DefaultMaxValueCandidates,
	)
	if len(issues) != 0 {
		return catalogPart{}, 0, fmt.Errorf("protected-converter output-ramp capacitance has no bounded preferred-value candidates")
	}
	for _, capacitance := range candidates {
		part, err := provider.selectPassiveComponentWithRatings(
			ctx, "capacitor", "capacitance", engineeringValue(capacitance, "F"),
			[]components.RequiredRating{{Kind: "voltage", Value: "6.25", Unit: "V"}},
		)
		if err != nil {
			continue
		}
		tolerancePercent, toleranceOK := catalogToleranceMaximum(part.record, "capacitance", "%")
		if !toleranceOK || tolerancePercent < 0 || tolerancePercent >= 100 {
			continue
		}
		minimumEffective := capacitance * (1 - tolerancePercent/100)
		maximumSlew := 1 / (efuseRampTimePerVoltPerFarad * minimumEffective)
		if protectedConverterInputCapacitanceF*maximumSlew > maximumInrushCurrent {
			continue
		}
		part.selected.InstanceID, part.usage, part.value =
			"converter_input_ramp", "soft_start_capacitor", engineeringValue(capacitance, "F")
		return part, maximumSlew, nil
	}
	return catalogPart{}, 0, fmt.Errorf(
		"no catalog-backed dV/dt capacitor bounds known %.12g F input capacitance to %.12g A",
		protectedConverterInputCapacitanceF, maximumInrushCurrent,
	)
}

func (provider *CatalogProvider) selectProtectedConverterOVPDivider(
	ctx context.Context,
	inputMaximum float64,
) (catalogPart, catalogPart, float64, float64, error) {
	if inputMaximum >= efuseMaximumExternalOVPV {
		return catalogPart{}, catalogPart{}, 0, 0, fmt.Errorf(
			"input maximum %.12g V leaves no margin below reviewed %.12g V external OVP limit",
			inputMaximum, efuseMaximumExternalOVPV,
		)
	}
	targetThreshold := math.Min(inputMaximum*1.15, efuseMaximumExternalOVPV*.9)
	bottomCandidates, err := provider.preferredResistanceCandidates(ctx, 30_100, 5, 0, 8)
	if err != nil {
		return catalogPart{}, catalogPart{}, 0, 0, fmt.Errorf("select protected-converter OVP bottom resistance: %w", err)
	}
	for _, bottomResistance := range bottomCandidates {
		bottom, selectErr := provider.selectPassiveComponent(
			ctx, "resistor", "resistance", engineeringValue(bottomResistance, "Ohm"),
		)
		if selectErr != nil {
			continue
		}
		bottomTolerancePercent, bottomToleranceOK := catalogToleranceMaximum(bottom.record, "resistance", "%")
		if !bottomToleranceOK || bottomTolerancePercent < 0 || bottomTolerancePercent >= 100 {
			continue
		}
		idealTop := bottomResistance * (targetThreshold/efuseOVPReferenceTypicalV - 1)
		topCandidates, topErr := provider.preferredResistanceCandidates(ctx, idealTop, 5, 0, 12)
		if topErr != nil {
			continue
		}
		for _, topResistance := range topCandidates {
			top, topSelectErr := provider.selectPassiveComponent(
				ctx, "resistor", "resistance", engineeringValue(topResistance, "Ohm"),
			)
			if topSelectErr != nil {
				continue
			}
			topTolerancePercent, topToleranceOK := catalogToleranceMaximum(top.record, "resistance", "%")
			if !topToleranceOK || topTolerancePercent < 0 || topTolerancePercent >= 100 {
				continue
			}
			topMinimum := topResistance * (1 - topTolerancePercent/100)
			topMaximum := topResistance * (1 + topTolerancePercent/100)
			bottomMinimum := bottomResistance * (1 - bottomTolerancePercent/100)
			bottomMaximum := bottomResistance * (1 + bottomTolerancePercent/100)
			thresholdMinimum := efuseOVPReferenceMinimumV * (topMinimum + bottomMaximum) / bottomMaximum
			thresholdMaximum := efuseOVPReferenceMaximumV * (topMaximum + bottomMinimum) / bottomMinimum
			if thresholdMinimum <= inputMaximum || thresholdMaximum > efuseMaximumExternalOVPV {
				continue
			}
			top.selected.InstanceID, top.usage, top.value =
				"converter_input_ovp_top", "threshold_divider", engineeringValue(topResistance, "Ohm")
			bottom.selected.InstanceID, bottom.usage, bottom.value =
				"converter_input_ovp_bottom", "threshold_divider", engineeringValue(bottomResistance, "Ohm")
			return top, bottom, thresholdMinimum, thresholdMaximum, nil
		}
	}
	return catalogPart{}, catalogPart{}, 0, 0, fmt.Errorf(
		"no catalog-backed OVP divider operates through %.12g V while remaining below %.12g V",
		inputMaximum, efuseMaximumExternalOVPV,
	)
}

func protectedConverterDischargeRequirement(constraints []Constraint, outputVoltage float64) (float64, float64, bool, error) {
	timeConstraint, timeOK := namedConstraint(constraints, "shutdown_discharge_time")
	if !timeOK {
		timeConstraint, timeOK = namedConstraint(constraints, "output_discharge_time")
	}
	voltageConstraint, voltageOK := namedConstraint(constraints, "shutdown_discharge_voltage")
	if !voltageOK {
		voltageConstraint, voltageOK = namedConstraint(constraints, "output_discharge_voltage")
	}
	if !timeOK && !voltageOK {
		return 0, 0, false, nil
	}
	if !timeOK || !voltageOK {
		return 0, 0, false, fmt.Errorf("protected isolated conversion requires both shutdown discharge time and voltage bounds")
	}
	timeSeconds, timeUnit, timeParsed := protectedConverterConstraintNumber(timeConstraint)
	voltage, voltageUnit, voltageParsed := protectedConverterConstraintNumber(voltageConstraint)
	if !timeParsed || !voltageParsed || timeConstraint.Relation != "maximum" || voltageConstraint.Relation != "maximum" {
		return 0, 0, false, fmt.Errorf("protected isolated conversion shutdown discharge bounds must be numeric maxima")
	}
	timeSeconds, timeConverted := convertCatalogUnit(timeSeconds, timeUnit, "s")
	voltage, voltageConverted := convertCatalogUnit(voltage, voltageUnit, "V")
	if !timeConverted || !voltageConverted || !finitePositive(timeSeconds) || !finitePositive(voltage) || voltage >= outputVoltage {
		return 0, 0, false, fmt.Errorf("protected isolated conversion shutdown discharge envelope is invalid")
	}
	return timeSeconds, voltage, true, nil
}

func protectedConverterConstraintNumber(constraint Constraint) (float64, string, bool) {
	var value float64
	if err := json.Unmarshal(constraint.Value, &value); err != nil || !finitePositive(value) {
		return 0, "", false
	}
	return value, constraint.Unit, true
}

func (provider *CatalogProvider) selectProtectedConverterDischargeResistor(
	ctx context.Context,
	outputVoltage, targetVoltage, maximumTime float64,
) (catalogPart, float64, error) {
	maximumResistance := maximumTime /
		(protectedConverterOutputCapacitanceMaximumF * math.Log(outputVoltage/targetVoltage))
	candidates, err := provider.preferredResistanceCandidates(ctx, maximumResistance*.9, 5, 0, DefaultMaxValueCandidates)
	if err != nil {
		return catalogPart{}, 0, fmt.Errorf("select protected-converter discharge resistance: %w", err)
	}
	for _, resistance := range candidates {
		if resistance > maximumResistance {
			continue
		}
		requiredPower := outputVoltage * outputVoltage / resistance
		part, selectErr := provider.selectPassiveComponentWithRatings(
			ctx, "resistor", "resistance", engineeringValue(resistance, "Ohm"),
			[]components.RequiredRating{{Kind: "power", Value: numericString(requiredPower), Unit: "W"}},
		)
		if selectErr != nil {
			continue
		}
		part.selected.InstanceID = "converter_output_discharge"
		part.usage = "shutdown_discharge"
		part.value = engineeringValue(resistance, "Ohm")
		return part, resistance, nil
	}
	return catalogPart{}, 0, fmt.Errorf(
		"no catalog-backed discharge resistor proves %.12g V to %.12g V within %.12g s",
		outputVoltage, targetVoltage, maximumTime,
	)
}
