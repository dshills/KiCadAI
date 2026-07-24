package architecturesearch

import (
	"context"
	"fmt"
	"math"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	precisionRectifierResistanceOhm     = 680_000.0
	precisionRectifierResistancePercent = 1.0
	precisionRectifierTempcoPPMPerC     = 25.0
)

func (provider *CatalogProvider) expandPrecisionRectification(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	inputPeak, _, ok := firstNumericConstraint(request.Constraints, "input_peak")
	errorLimit, _, errorOK := firstNumericConstraint(request.Constraints, "transfer_error")
	inputMinimum, inputMaximum, inputOK := roleVoltageRange(request.Ports, "input")
	powerMinimum, powerMaximum, powerOK := roleVoltageRange(request.Ports, "power")
	if !inputOK || !powerOK || inputMinimum >= 0 || inputMaximum <= 0 || powerMinimum <= 0 || powerMaximum < powerMinimum {
		return nil, fmt.Errorf("precision rectification requires a bipolar input and bounded positive supply")
	}
	if !ok || inputPeak <= 0 {
		inputPeak = math.Max(math.Abs(inputMinimum), math.Abs(inputMaximum))
	}
	if inputPeak <= 0 || !errorOK || errorLimit <= 0 || errorLimit >= inputPeak {
		return nil, fmt.Errorf("precision rectification requires positive input-peak and bounded transfer-error evidence")
	}
	minimumInputImpedance := precisionRectifierMinimumInputImpedance(request)
	if minimumInputImpedance <= 0 {
		return nil, fmt.Errorf("precision rectification requires a positive minimum input impedance")
	}

	bias, err := provider.selectComponent(ctx, "regulator", "LM7705", []components.RequiredRating{
		{Kind: "supply_voltage", Value: numericString(powerMinimum), Unit: "V"},
		{Kind: "supply_voltage", Value: numericString(powerMaximum), Unit: "V"},
		{Kind: "output_current", Value: "0.003", Unit: "A"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier negative-bias selection failed: %w", err)
	}
	bias.selected.InstanceID, bias.usage = "negative_bias", "regulator"
	biasMinimum, biasMaximum := catalogUncertaintyBounds(bias.record, "model_parameters.reference_voltage_v", .232)
	if biasMinimum <= 0 || biasMaximum < biasMinimum {
		return nil, fmt.Errorf("selected negative-bias generator lacks bounded output-voltage evidence")
	}

	supplySpanMinimum := powerMinimum + biasMinimum
	supplySpanMaximum := powerMaximum + biasMaximum
	opamp, err := provider.selectComponent(ctx, "opamp", "OPA197", []components.RequiredRating{
		{Kind: "supply_voltage", Value: numericString(supplySpanMinimum), Unit: "V"},
		{Kind: "supply_voltage", Value: numericString(supplySpanMaximum), Unit: "V"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier amplifier selection failed: %w", err)
	}
	sumAmplifier := opamp
	sumAmplifier.selected.InstanceID, sumAmplifier.usage = "magnitude_amplifier", "opamp"
	steeringAmplifier := opamp
	steeringAmplifier.selected.InstanceID, steeringAmplifier.usage = "steering_amplifier", "opamp"

	diodeCurrent := 2 * inputPeak / precisionRectifierResistanceOhm
	diode, err := provider.selectComponent(ctx, "diode", "1N4148W", []components.RequiredRating{
		{Kind: "current", Value: numericString(diodeCurrent), Unit: "A"},
		{Kind: "reverse_voltage", Value: numericString(inputPeak), Unit: "V"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier steering-diode selection failed: %w", err)
	}
	diode.selected.InstanceID, diode.usage = "steering_diode", "default_clamp"
	dampingResistor, err := provider.selectComponent(ctx, "resistor", "RC0805FR-0747RL", nil, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier steering-loop damping selection failed: %w", err)
	}
	dampingResistor.selected.InstanceID, dampingResistor.usage = "steering_damping", "feedback"
	dampingResistor.value = engineeringValue(47, "Ohm")

	resistor, err := provider.selectComponent(ctx, "resistor", "680k", nil, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier resistor selection failed: %w", err)
	}
	resistorTolerance, toleranceOK := catalogToleranceMaximum(resistor.record, "resistance", "%")
	resistorTempco, tempcoOK := recordValueMaximum(resistor.record, "temperature_coefficient", "ppm/C")
	if !toleranceOK || resistorTolerance > precisionRectifierResistancePercent || !tempcoOK || resistorTempco > precisionRectifierTempcoPPMPerC {
		return nil, fmt.Errorf("selected precision-rectifier resistor lacks bounded tolerance and temperature coefficient")
	}
	resistor.value = engineeringValue(precisionRectifierResistanceOhm, "Ohm")
	inputResistor := resistor
	inputResistor.selected.InstanceID, inputResistor.usage = "input_sum", "feedback"
	feedbackResistor := resistor
	feedbackResistor.selected.InstanceID, feedbackResistor.usage = "sum_feedback", "feedback"
	steeringResistor := resistor
	steeringResistor.selected.InstanceID, steeringResistor.usage = "steering_input", "feedback"

	largeCapacitor, err := provider.selectComponent(ctx, "capacitor", "10u", []components.RequiredRating{{Kind: "voltage", Value: numericString(supplySpanMaximum), Unit: "V"}}, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier charge-storage capacitor selection failed: %w", err)
	}
	largeCapacitor.value = engineeringValue(10e-6, "F")
	smallCapacitor, err := provider.selectComponent(ctx, "capacitor", "100n", []components.RequiredRating{{Kind: "voltage", Value: numericString(supplySpanMaximum), Unit: "V"}}, true)
	if err != nil {
		return nil, fmt.Errorf("precision-rectifier bypass capacitor selection failed: %w", err)
	}
	smallCapacitor.value = engineeringValue(100e-9, "F")

	parts := []catalogPart{bias, sumAmplifier, steeringAmplifier, diode, dampingResistor, inputResistor, feedbackResistor, steeringResistor}
	for _, support := range []struct {
		id, usage string
		base      catalogPart
	}{
		{"charge_fly", "decoupling_capacitor", largeCapacitor},
		{"charge_reserve_a", "decoupling_capacitor", largeCapacitor},
		{"charge_reserve_b", "decoupling_capacitor", largeCapacitor},
		{"bias_output_bypass", "decoupling_capacitor", smallCapacitor},
		{"bias_input_bypass", "decoupling_capacitor", smallCapacitor},
		{"sum_supply_bypass", "decoupling_capacitor", smallCapacitor},
		{"steering_supply_bypass", "decoupling_capacitor", smallCapacitor},
	} {
		part := support.base
		part.selected.InstanceID, part.usage = support.id, support.usage
		parts = append(parts, part)
	}

	calculation, err := precisionRectifierCalculation(precisionRectifierCalculationRequest{
		InputPeakV:                 inputPeak,
		TransferErrorLimitV:        errorLimit,
		MinimumInputImpedanceOhm:   minimumInputImpedance,
		ResistanceOhm:              precisionRectifierResistanceOhm,
		ResistanceTolerancePercent: resistorTolerance,
		DiodeLeakageA:              mustCatalogValueMaximum(diode.record, "reverse_leakage_current", "A"),
		OpAmpOffsetV:               mustCatalogValueMaximum(opamp.record, "input_offset_voltage", "V"),
		OpAmpGainBandwidthHz:       mustCatalogSimulationParameter(opamp.record, simmodel.PrimitiveOpAmpV1, "gain_bandwidth_hz"),
		OpAmpSlewRateVPerS:         opamp.record.OpAmp.SlewRate.Value,
		OpAmpNoiseDensityVPerSqrtHz: mustCatalogSimulationParameter(
			opamp.record, simmodel.PrimitiveOpAmpV1, "input_voltage_noise_density_v_sqrt_hz",
		),
		FrequencyHz: precisionRectifierFrequency(request),
	})
	if err != nil {
		return nil, err
	}

	bindings := bindRoles(request.Ports, inputResistor.selected.InstanceID, map[string]string{
		"input": "A", "output": "OUT", "power": "VIN", "reference": "ADJ",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "output":
			bindings[index].Instance, bindings[index].Function = sumAmplifier.selected.InstanceID, "OUT"
		case "power":
			bindings[index].Instance, bindings[index].Function = bias.selected.InstanceID, "VIN"
		case "reference":
			bindings[index].Instance, bindings[index].Function = bias.selected.InstanceID, "ADJ"
		}
	}
	connections := []RealizationConnection{
		semanticNet("rectifier_input", "analog_signal", endpoint(inputResistor, "A"), endpoint(steeringResistor, "A")),
		semanticNet("rectifier_sum_node", "analog_signal", endpoint(inputResistor, "B"), endpoint(feedbackResistor, "A"), endpoint(sumAmplifier, "IN_MINUS")),
		semanticNet("rectifier_output", "analog_signal", endpoint(feedbackResistor, "B"), endpoint(sumAmplifier, "OUT")),
		semanticNet("rectifier_steering_node", "analog_signal", endpoint(steeringResistor, "B"), endpoint(sumAmplifier, "IN_PLUS"), endpoint(steeringAmplifier, "IN_MINUS"), endpoint(diode, "CATHODE")),
		semanticNet("rectifier_steering_output", "analog_signal", endpoint(steeringAmplifier, "OUT"), endpoint(dampingResistor, "A")),
		semanticNet("rectifier_steering_diode_drive", "analog_signal", endpoint(dampingResistor, "B"), endpoint(diode, "ANODE")),
		semanticNet("rectifier_power", "power", endpoint(bias, "VIN"), endpoint(sumAmplifier, "V_PLUS"), endpoint(steeringAmplifier, "V_PLUS"), passiveEndpoint("bias_input_bypass", "A"), passiveEndpoint("sum_supply_bypass", "A"), passiveEndpoint("steering_supply_bypass", "A")),
		semanticNet("rectifier_negative_bias", "power", endpoint(bias, "VOUT"), endpoint(sumAmplifier, "V_MINUS"), endpoint(steeringAmplifier, "V_MINUS"), passiveEndpoint("bias_output_bypass", "A"), passiveEndpoint("sum_supply_bypass", "B"), passiveEndpoint("steering_supply_bypass", "B")),
		semanticNet("rectifier_reference", "reference", endpoint(steeringAmplifier, "IN_PLUS"), endpoint(bias, "ADJ"), endpoint(bias, "GROUND_SECONDARY"), endpoint(bias, "ENABLE_N"), passiveEndpoint("bias_input_bypass", "B"), passiveEndpoint("bias_output_bypass", "B"), passiveEndpoint("charge_reserve_a", "B"), passiveEndpoint("charge_reserve_b", "B")),
		semanticNet("charge_pump_fly_positive", "power", endpoint(bias, "CF_PLUS"), passiveEndpoint("charge_fly", "A")),
		semanticNet("charge_pump_fly_negative", "power", endpoint(bias, "CF_MINUS"), passiveEndpoint("charge_fly", "B")),
		semanticNet("charge_pump_reserve", "power", endpoint(bias, "RESERVE"), passiveEndpoint("charge_reserve_a", "A"), passiveEndpoint("charge_reserve_b", "A")),
	}
	expansions, err := provider.expansion(request, "single_supply_precision_full_wave_rectifier", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
	if err != nil {
		return nil, err
	}
	inputImpedance, _ := calculationOutput(calculation, "minimum_input_impedance")
	for expansionIndex := range expansions {
		for portIndex := range expansions[expansionIndex].OfferedPorts {
			if expansions[expansionIndex].OfferedPorts[portIndex].Role == "input" {
				expansions[expansionIndex].OfferedPorts[portIndex].Contract.InputImpedanceMinOhm = float64Pointer(inputImpedance)
			}
		}
	}
	return expansions, nil
}

type precisionRectifierCalculationRequest struct {
	InputPeakV                  float64
	TransferErrorLimitV         float64
	MinimumInputImpedanceOhm    float64
	ResistanceOhm               float64
	ResistanceTolerancePercent  float64
	DiodeLeakageA               float64
	OpAmpOffsetV                float64
	OpAmpGainBandwidthHz        float64
	OpAmpSlewRateVPerS          float64
	OpAmpNoiseDensityVPerSqrtHz float64
	FrequencyHz                 float64
}

func precisionRectifierCalculation(request precisionRectifierCalculationRequest) (CalculationEvidence, error) {
	if request.InputPeakV <= 0 || request.TransferErrorLimitV <= 0 || request.MinimumInputImpedanceOhm <= 0 ||
		request.ResistanceOhm <= 0 || request.ResistanceTolerancePercent < 0 || request.ResistanceTolerancePercent >= 100 ||
		request.DiodeLeakageA < 0 || request.OpAmpOffsetV < 0 || request.OpAmpGainBandwidthHz <= 0 ||
		request.OpAmpSlewRateVPerS <= 0 || request.OpAmpNoiseDensityVPerSqrtHz <= 0 || request.FrequencyHz <= 0 {
		return CalculationEvidence{}, fmt.Errorf("precision-rectifier calculation requires finite positive bounded evidence")
	}
	tolerance := request.ResistanceTolerancePercent / 100
	resistanceMinimum := request.ResistanceOhm * (1 - tolerance)
	resistanceMaximum := request.ResistanceOhm * (1 + tolerance)
	minimumInputImpedance := resistanceMinimum / 2
	maximumGain := (1 + tolerance) / (1 - tolerance)
	negativeError := request.InputPeakV*(maximumGain-1) + 4*request.OpAmpOffsetV
	positiveError := 2*request.DiodeLeakageA*resistanceMaximum + 4*request.OpAmpOffsetV
	worstError := math.Max(negativeError, positiveError)
	noiseGain := 2.0
	closedLoopBandwidth := request.OpAmpGainBandwidthHz / noiseGain
	linearSettling := -math.Log(request.TransferErrorLimitV/request.InputPeakV) / (2 * math.Pi * closedLoopBandwidth)
	slewSettling := request.InputPeakV / request.OpAmpSlewRateVPerS
	settlingTime := math.Max(linearSettling, slewSettling)
	resistorNoiseDensity := math.Sqrt(4 * boltzmannConstantJPerK * 300.15 * resistanceMaximum)
	integratedNoise := math.Hypot(noiseGain*request.OpAmpNoiseDensityVPerSqrtHz, resistorNoiseDensity) * math.Sqrt(request.FrequencyHz)
	errorMargin := (request.TransferErrorLimitV - worstError) / request.TransferErrorLimitV
	impedanceMargin := (minimumInputImpedance - request.MinimumInputImpedanceOhm) / request.MinimumInputImpedanceOhm
	evidence := CalculationEvidence{
		ID: "precision_rectifier_worst_case", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "input_peak", Value: request.InputPeakV, Unit: "V"},
			{Name: "transfer_error_limit", Value: request.TransferErrorLimitV, Unit: "V"},
			{Name: "minimum_input_impedance_requirement", Value: request.MinimumInputImpedanceOhm, Unit: "Ohm"},
			{Name: "resistance_tolerance", Value: request.ResistanceTolerancePercent, Unit: "%"},
			{Name: "diode_reverse_leakage", Value: request.DiodeLeakageA, Unit: "A"},
			{Name: "opamp_input_offset", Value: request.OpAmpOffsetV, Unit: "V"},
			{Name: "opamp_gain_bandwidth", Value: request.OpAmpGainBandwidthHz, Unit: "Hz"},
			{Name: "opamp_slew_rate", Value: request.OpAmpSlewRateVPerS, Unit: "V/s"},
		},
		SelectedValues: []SelectedValueEvidence{{
			Name: "matched_resistance", Ideal: request.ResistanceOhm, Selected: request.ResistanceOhm,
			Unit: "Ohm", Series: SeriesE24,
		}},
		NominalOutputs: []NamedQuantity{
			{Name: "worst_case_transfer_error", Value: worstError, Unit: "V"},
			{Name: "minimum_input_impedance", Value: minimumInputImpedance, Unit: "Ohm"},
			{Name: "settling_time", Value: settlingTime, Unit: "s"},
			{Name: "integrated_output_noise", Value: integratedNoise, Unit: "V_rms"},
		},
		Bounds: []CalculationBound{
			maximumBound("transfer_error", request.TransferErrorLimitV, worstError, "V"),
			minimumBound("input_impedance", request.MinimumInputImpedanceOhm, minimumInputImpedance, "Ohm"),
		},
		WorstMargin: quantize(math.Min(errorMargin, impedanceMargin)),
		Pass:        worstError <= request.TransferErrorLimitV && minimumInputImpedance >= request.MinimumInputImpedanceOhm,
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil || !finalized.Pass {
		return CalculationEvidence{}, fmt.Errorf("catalog-backed precision rectifier cannot prove the requested transfer-error and input-impedance envelope")
	}
	return finalized, nil
}

const boltzmannConstantJPerK = 1.380649e-23

func precisionRectifierMinimumInputImpedance(request ProviderRequest) float64 {
	minimum, _, ok := firstNumericConstraint(request.Constraints, "minimum_input_impedance")
	for _, port := range request.Ports {
		if port.Role == "input" && port.Contract.InputImpedanceMinOhm != nil {
			minimum = math.Max(minimum, *port.Contract.InputImpedanceMinOhm)
			ok = true
		}
	}
	if !ok {
		return 0
	}
	return minimum
}

func precisionRectifierFrequency(request ProviderRequest) float64 {
	frequency := 1.0
	for _, port := range request.Ports {
		if (port.Role == "input" || port.Role == "output") && port.Contract.FrequencyMaxHz != nil {
			frequency = math.Max(frequency, *port.Contract.FrequencyMaxHz)
		}
	}
	return frequency
}

func mustCatalogValueMaximum(record components.ComponentRecord, kind, unit string) float64 {
	value, _ := recordValueMaximum(record, kind, unit)
	return value
}

func mustCatalogSimulationParameter(record components.ComponentRecord, modelID, name string) float64 {
	value, _ := catalogSimulationParameterForModel(record, modelID, name)
	return value
}
