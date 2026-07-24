package architecturesearch

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodeInterfaceTerminationUnproven    reports.Code = "INTERFACE_TERMINATION_UNPROVEN"
	CodeInterfaceADCDriveUnproven       reports.Code = "INTERFACE_ADC_DRIVE_UNPROVEN"
	CodeInterfaceClockConditionUnproven reports.Code = "INTERFACE_CLOCK_CONDITIONING_UNPROVEN"
	CodeInterfacePullupWindowEmpty      reports.Code = "INTERFACE_PULLUP_WINDOW_EMPTY"
	CodeInterfaceVoltageDomainMismatch  reports.Code = "INTERFACE_VOLTAGE_DOMAIN_MISMATCH"
	CodeInterfaceTranslationUnavailable reports.Code = "INTERFACE_TRANSLATION_UNAVAILABLE"
)

type interfaceSynthesisError struct {
	code    reports.Code
	message string
}

func (err *interfaceSynthesisError) Error() string { return err.message }

func (err *interfaceSynthesisError) ArchitectureRejectionCode() reports.Code { return err.code }

func (provider *CatalogProvider) expandSourceTermination(ctx context.Context, request ProviderRequest, clock bool) ([]ProviderExpansion, error) {
	code := CodeInterfaceTerminationUnproven
	fragmentID := "source_series_termination"
	var clockEvidence *CalculationEvidence
	if clock {
		code = CodeInterfaceClockConditionUnproven
		fragmentID = "clock_source_series_conditioning"
		validated, err := proveClockInterface(request)
		if err != nil {
			return nil, err
		}
		clockEvidence = &validated
	}
	driver, _, driverOK := firstNumericConstraint(request.Constraints, "driver_output_impedance")
	target, tolerance, targetOK := firstNumericConstraint(request.Constraints, "interconnect_target_impedance", "target_impedance")
	if !driverOK || !targetOK || driver < 0 || target <= driver || tolerance < 0 || tolerance >= 100 {
		return nil, &interfaceSynthesisError{code: code, message: "source termination requires compatible driver and target impedance evidence"}
	}
	if tolerance == 0 {
		tolerance = 5
	}
	ideal := target - driver
	minimum := math.Max(1e-3, target*(1-tolerance/100)-driver)
	maximum := target*(1+tolerance/100) - driver
	values, issues := PreferredValueCandidates(ideal, SeriesE24, minimum, maximum, 1)
	if len(issues) != 0 || len(values) != 1 {
		return nil, &interfaceSynthesisError{code: code, message: "no preferred source-series resistor satisfies the impedance window"}
	}
	selected := values[0]
	observed := driver + selected
	margin := math.Min(observed-minimum-driver, maximum+driver-observed)
	evidence := CalculationEvidence{
		ID: fragmentID, FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs:         []NamedQuantity{{Name: "driver_output_impedance", Value: driver, Unit: "Ohm"}, {Name: "interconnect_target_impedance", Value: target, Unit: "Ohm"}},
		SelectedValues: []SelectedValueEvidence{{Name: "series_resistance", Ideal: ideal, Selected: selected, Unit: "Ohm", Series: SeriesE24, RelativeError: math.Abs(selected-ideal) / ideal}},
		NominalOutputs: []NamedQuantity{{Name: "source_impedance", Value: observed, Unit: "Ohm"}},
		Bounds:         []CalculationBound{minimumBound("source_impedance_minimum", target*(1-tolerance/100), observed, "Ohm"), maximumBound("source_impedance_maximum", target*(1+tolerance/100), observed, "Ohm")},
		WorstMargin:    quantize(margin / math.Max(target, 1)), Pass: margin >= 0,
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil || !evidence.Pass {
		return nil, &interfaceSynthesisError{code: code, message: "could not prove source-series termination"}
	}
	parts, err := provider.appendPassiveParts(ctx, nil, []passivePart{{"series_termination", "resistor", "source_termination", engineeringValue(selected, "Ohm")}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, "series_termination", map[string]string{"input": "A", "output": "B"})
	calculations := []CalculationEvidence{evidence}
	if clockEvidence != nil {
		calculations = append([]CalculationEvidence{*clockEvidence}, calculations...)
	}
	return provider.expansion(request, fragmentID, parts, bindings, nil, calculations, 0)
}

func proveClockInterface(request ProviderRequest) (CalculationEvidence, error) {
	fail := func(message string) (CalculationEvidence, error) {
		return CalculationEvidence{}, &interfaceSynthesisError{code: CodeInterfaceClockConditionUnproven, message: message}
	}
	frequency, _, frequencyOK := firstNumericConstraint(request.Constraints, "clock_frequency")
	if frequency <= 0 && maximumProtocolFrequency(request.Ports) > 0 {
		frequency, frequencyOK = maximumProtocolFrequency(request.Ports), true
	}
	amplitude, _, amplitudeOK := firstNumericConstraint(request.Constraints, "clock_amplitude")
	commonMode, _, commonModeOK := firstNumericConstraint(request.Constraints, "clock_common_mode")
	edgeTime, _, edgeOK := firstNumericConstraint(request.Constraints, "clock_edge_time")
	lowThreshold, _, lowOK := firstNumericConstraint(request.Constraints, "receiver_low_threshold")
	highThreshold, _, highOK := firstNumericConstraint(request.Constraints, "receiver_high_threshold")
	maximumEdgeTime, _, maximumEdgeOK := firstNumericConstraint(request.Constraints, "receiver_maximum_edge_time")
	jitter, _, jitterOK := firstNumericConstraint(request.Constraints, "clock_rms_jitter")
	maximumJitter, _, maximumJitterOK := firstNumericConstraint(request.Constraints, "receiver_maximum_rms_jitter")
	startup, _, startupOK := firstNumericConstraint(request.Constraints, "clock_startup_time")
	maximumStartup, _, maximumStartupOK := firstNumericConstraint(request.Constraints, "maximum_clock_startup_time")
	fanout, _, fanoutOK := firstNumericConstraint(request.Constraints, "clock_fanout")
	receiverCapacitance, _, capacitanceOK := firstNumericConstraint(request.Constraints, "receiver_input_capacitance")
	maximumCapacitiveLoad, _, maximumCapacitanceOK := firstNumericConstraint(request.Constraints, "source_maximum_capacitive_load")
	maximumOutputCurrent, _, maximumCurrentOK := firstNumericConstraint(request.Constraints, "source_maximum_current")
	sourceMaximumFrequency, _, sourceFrequencyOK := firstNumericConstraint(request.Constraints, "source_maximum_frequency")
	receiverMaximumFrequency, _, receiverFrequencyOK := firstNumericConstraint(request.Constraints, "receiver_maximum_frequency")
	if !frequencyOK || !amplitudeOK || !commonModeOK || !edgeOK || !lowOK || !highOK || !maximumEdgeOK ||
		!jitterOK || !maximumJitterOK || !startupOK || !maximumStartupOK || !fanoutOK || !capacitanceOK ||
		!maximumCapacitanceOK || !maximumCurrentOK || !sourceFrequencyOK || !receiverFrequencyOK {
		return fail("clock conditioning requires amplitude, common-mode, edge, frequency, jitter, startup, fanout, and loading evidence")
	}
	if frequency <= 0 || amplitude <= 0 || edgeTime <= 0 || lowThreshold >= highThreshold || maximumEdgeTime <= 0 ||
		jitter < 0 || maximumJitter < 0 || startup < 0 || maximumStartup < 0 || fanout < 1 || fanout != math.Trunc(fanout) ||
		receiverCapacitance <= 0 || maximumCapacitiveLoad <= 0 || maximumOutputCurrent <= 0 ||
		sourceMaximumFrequency <= 0 || receiverMaximumFrequency <= 0 {
		return fail("clock conditioning evidence contains invalid or unbounded values")
	}
	sourceMode, sourceModeOK := clockConstraintString(request.Constraints, "clock_signaling_mode")
	receiverMode, receiverModeOK := clockConstraintString(request.Constraints, "receiver_signaling_mode")
	if !sourceModeOK || !receiverModeOK || canonicalIdentifier(sourceMode) != canonicalIdentifier(receiverMode) {
		return fail("clock source and receiver signaling modes are missing or incompatible")
	}
	sourceMinimum, sourceMaximum, sourceRangeOK := roleVoltageRange(request.Ports, "input")
	receiverMinimum, receiverMaximum, receiverRangeOK := roleVoltageRange(request.Ports, "output")
	if !sourceRangeOK || !receiverRangeOK {
		return fail("clock conditioning requires bounded source and receiver voltage ranges")
	}
	clockLow := commonMode - amplitude/2
	clockHigh := commonMode + amplitude/2
	loadCapacitance := fanout * receiverCapacitance
	averageDynamicCurrent := loadCapacitance * amplitude * frequency
	edgeDynamicCurrent := loadCapacitance * amplitude / edgeTime
	requiredOutputCurrent := math.Max(averageDynamicCurrent, edgeDynamicCurrent)
	type margin struct {
		name     string
		required float64
		observed float64
		unit     string
		value    float64
		scale    float64
		maximum  bool
	}
	margins := []margin{
		{name: "source_low_voltage", required: math.Max(sourceMinimum, receiverMinimum), observed: clockLow, unit: "V", value: clockLow - math.Max(sourceMinimum, receiverMinimum), scale: math.Max(amplitude, 1e-12)},
		{name: "source_high_voltage", required: math.Min(sourceMaximum, receiverMaximum), observed: clockHigh, unit: "V", value: math.Min(sourceMaximum, receiverMaximum) - clockHigh, scale: math.Max(amplitude, 1e-12), maximum: true},
		{name: "receiver_low_threshold", required: lowThreshold, observed: clockLow, unit: "V", value: lowThreshold - clockLow, scale: math.Max(amplitude, 1e-12), maximum: true},
		{name: "receiver_high_threshold", required: highThreshold, observed: clockHigh, unit: "V", value: clockHigh - highThreshold, scale: math.Max(amplitude, 1e-12)},
		{name: "receiver_maximum_edge_time", required: maximumEdgeTime, observed: edgeTime, unit: "s", value: maximumEdgeTime - edgeTime, scale: maximumEdgeTime, maximum: true},
		{name: "receiver_maximum_rms_jitter", required: maximumJitter, observed: jitter, unit: "s", value: maximumJitter - jitter, scale: math.Max(maximumJitter, 1e-15), maximum: true},
		{name: "maximum_clock_startup_time", required: maximumStartup, observed: startup, unit: "s", value: maximumStartup - startup, scale: math.Max(maximumStartup, 1e-15), maximum: true},
		{name: "source_maximum_frequency", required: sourceMaximumFrequency, observed: frequency, unit: "Hz", value: sourceMaximumFrequency - frequency, scale: sourceMaximumFrequency, maximum: true},
		{name: "receiver_maximum_frequency", required: receiverMaximumFrequency, observed: frequency, unit: "Hz", value: receiverMaximumFrequency - frequency, scale: receiverMaximumFrequency, maximum: true},
		{name: "source_maximum_capacitive_load", required: maximumCapacitiveLoad, observed: loadCapacitance, unit: "F", value: maximumCapacitiveLoad - loadCapacitance, scale: maximumCapacitiveLoad, maximum: true},
		{name: "source_maximum_current", required: maximumOutputCurrent, observed: requiredOutputCurrent, unit: "A", value: maximumOutputCurrent - requiredOutputCurrent, scale: maximumOutputCurrent, maximum: true},
	}
	worstMargin := math.Inf(1)
	var bounds []CalculationBound
	for _, item := range margins {
		if item.value < 0 {
			return fail("clock source and receiver electrical bounds are incompatible at the requested operating point")
		}
		normalized := item.value / math.Max(item.scale, 1e-15)
		worstMargin = math.Min(worstMargin, normalized)
		if item.maximum {
			bounds = append(bounds, maximumBound(item.name, item.required, item.observed, item.unit))
		} else {
			bounds = append(bounds, minimumBound(item.name, item.required, item.observed, item.unit))
		}
	}
	evidence := CalculationEvidence{
		ID: "clock_interface_compatibility", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "clock_frequency", Value: frequency, Unit: "Hz"}, {Name: "clock_amplitude", Value: amplitude, Unit: "V"},
			{Name: "clock_common_mode", Value: commonMode, Unit: "V"}, {Name: "clock_edge_time", Value: edgeTime, Unit: "s"},
			{Name: "clock_rms_jitter", Value: jitter, Unit: "s"}, {Name: "clock_startup_time", Value: startup, Unit: "s"},
			{Name: "clock_fanout", Value: fanout, Unit: "count"}, {Name: "receiver_input_capacitance", Value: receiverCapacitance, Unit: "F"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "clock_low_voltage", Value: clockLow, Unit: "V"}, {Name: "clock_high_voltage", Value: clockHigh, Unit: "V"},
			{Name: "total_capacitive_load", Value: loadCapacitance, Unit: "F"}, {Name: "average_dynamic_output_current", Value: averageDynamicCurrent, Unit: "A"},
			{Name: "edge_dynamic_output_current", Value: edgeDynamicCurrent, Unit: "A"}, {Name: "required_output_current", Value: requiredOutputCurrent, Unit: "A"},
		},
		Bounds: bounds, WorstMargin: quantize(worstMargin), Pass: true,
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil {
		return fail("could not finalize clock interface compatibility evidence")
	}
	return finalized, nil
}

func clockConstraintString(constraints []Constraint, name string) (string, bool) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(constraint.Value, &value) != nil || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func (provider *CatalogProvider) expandADCDrive(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	fail := func(message string) ([]ProviderExpansion, error) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceADCDriveUnproven, message: message}
	}
	source, _, sourceOK := firstNumericConstraint(request.Constraints, "source_impedance")
	inputCapacitance, _, capOK := firstNumericConstraint(request.Constraints, "adc_input_capacitance")
	acquisition, _, acquisitionOK := firstNumericConstraint(request.Constraints, "acquisition_time")
	accuracy, _, accuracyOK := firstNumericConstraint(request.Constraints, "settling_accuracy")
	if !sourceOK || !capOK || !acquisitionOK || !accuracyOK || source < 0 || inputCapacitance <= 0 || acquisition <= 0 || accuracy <= 0 || accuracy >= 1 {
		return fail("ADC drive requires source impedance, input capacitance, acquisition time, and fractional settling accuracy evidence")
	}
	maximumResistance := acquisition/(inputCapacitance*-math.Log(accuracy)) - source
	if maximumResistance < 1 {
		return provider.expandBufferedADCDrive(ctx, request, source, inputCapacitance, acquisition, accuracy)
	}
	ideal := math.Min(100, maximumResistance)
	values, issues := PreferredValueCandidates(ideal, SeriesE24, 1, maximumResistance, 1)
	if len(issues) != 0 || len(values) != 1 {
		return fail("no preferred ADC isolation resistor satisfies the settling window")
	}
	seriesResistance := values[0]
	filterCapacitance := 0.0
	idealFilterCapacitance := 0.0
	passives := []passivePart{{"adc_isolation", "resistor", "adc_drive_isolation", engineeringValue(seriesResistance, "Ohm")}}
	if cutoff, _, ok := firstNumericConstraint(request.Constraints, "anti_alias_cutoff"); ok {
		if cutoff <= 0 || !hasRoleContract(request.Ports, "reference") {
			return fail("anti-alias filtering requires a positive cutoff and reference port")
		}
		idealFilterCapacitance = 1 / (2 * math.Pi * (source + seriesResistance) * cutoff)
		candidates, candidateIssues := PreferredValueCandidates(idealFilterCapacitance, SeriesE24, idealFilterCapacitance*.9, idealFilterCapacitance*1.1, 1)
		if len(candidateIssues) != 0 || len(candidates) != 1 {
			return fail("no preferred anti-alias capacitor satisfies the requested cutoff")
		}
		filterCapacitance = candidates[0]
		passives = append(passives, passivePart{"adc_filter", "capacitor", "adc_anti_alias", engineeringValue(filterCapacitance, "F")})
	}
	settlingTime := -math.Log(accuracy) * (source + seriesResistance) * (inputCapacitance + filterCapacitance)
	if settlingTime > acquisition {
		return fail("selected passive ADC drive network exceeds the acquisition-time bound")
	}
	margin := acquisition - settlingTime
	evidence := CalculationEvidence{
		ID: "adc_drive_settling", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs:         []NamedQuantity{{Name: "source_impedance", Value: source, Unit: "Ohm"}, {Name: "adc_input_capacitance", Value: inputCapacitance, Unit: "F"}, {Name: "acquisition_time", Value: acquisition, Unit: "s"}, {Name: "settling_accuracy", Value: accuracy, Unit: "ratio"}},
		SelectedValues: []SelectedValueEvidence{{Name: "isolation_resistance", Ideal: ideal, Selected: seriesResistance, Unit: "Ohm", Series: SeriesE24, RelativeError: math.Abs(seriesResistance-ideal) / ideal}},
		NominalOutputs: []NamedQuantity{{Name: "settling_time", Value: settlingTime, Unit: "s"}},
		Bounds:         []CalculationBound{maximumBound("acquisition_time", acquisition, settlingTime, "s")}, WorstMargin: quantize(margin / acquisition), Pass: true,
	}
	if filterCapacitance > 0 {
		evidence.SelectedValues = append(evidence.SelectedValues, SelectedValueEvidence{Name: "filter_capacitance", Ideal: idealFilterCapacitance, Selected: filterCapacitance, Unit: "F", Series: SeriesE24, RelativeError: math.Abs(filterCapacitance-idealFilterCapacitance) / idealFilterCapacitance})
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return fail("could not finalize ADC settling evidence")
	}
	parts, err := provider.appendPassiveParts(ctx, nil, passives)
	if err != nil {
		return nil, err
	}
	roleFunctions := map[string]string{"input": "A", "output": "B"}
	if filterCapacitance > 0 {
		roleFunctions["reference"] = "B"
	}
	var bindings []RealizationPortBinding
	for _, port := range request.Ports {
		function, bound := roleFunctions[port.Role]
		if !bound {
			continue
		}
		bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: "adc_isolation", Function: function})
	}
	var connections []RealizationConnection
	if filterCapacitance > 0 {
		for index := range bindings {
			if bindings[index].Role == "reference" {
				bindings[index].Instance, bindings[index].Function = "adc_filter", "B"
			}
		}
		connections = append(connections, semanticNet("adc_drive_node", "analog_signal", passiveEndpoint("adc_isolation", "B"), passiveEndpoint("adc_filter", "A")))
	}
	return provider.expansion(request, "passive_adc_drive_conditioning", parts, bindings, connections, []CalculationEvidence{evidence}, 0)
}

func (provider *CatalogProvider) expandBufferedADCDrive(ctx context.Context, request ProviderRequest, source, inputCapacitance, acquisition, accuracy float64) ([]ProviderExpansion, error) {
	fail := func(message string) ([]ProviderExpansion, error) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceADCDriveUnproven, message: message}
	}
	if accuracy <= 0 || accuracy >= 1 || math.IsNaN(accuracy) || math.IsInf(accuracy, 0) {
		return fail("buffered ADC drive requires fractional settling accuracy between zero and one")
	}
	if !hasRoleContract(request.Ports, "power") || !hasRoleContract(request.Ports, "reference") {
		return fail("buffered ADC drive requires explicit power and reference ports")
	}
	supplyMinimum, supplyMaximum, supplyOK := roleVoltageRange(request.Ports, "power")
	referenceMinimum, referenceMaximum, referenceOK := roleVoltageRange(request.Ports, "reference")
	inputMinimum, inputMaximum, inputOK := roleVoltageRange(request.Ports, "input")
	supplySpanMinimum := supplyMinimum - referenceMaximum
	supplySpanMaximum := supplyMaximum - referenceMinimum
	inputAboveReferenceMinimum := inputMinimum - referenceMaximum
	inputAboveReferenceMaximum := inputMaximum - referenceMinimum
	if !supplyOK || !referenceOK || !inputOK || supplySpanMinimum <= 0 || supplySpanMaximum < supplySpanMinimum {
		return fail("buffered ADC drive requires bounded supply, reference, and input voltage ranges")
	}
	ratings := []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplySpanMinimum), Unit: "V"}, {Kind: "supply_voltage", Value: numericString(supplySpanMaximum), Unit: "V"}}
	buffer, ok := provider.selectProvenADCBuffer(ratings)
	if !ok || buffer.record.OpAmp == nil || buffer.record.OpAmp.GainBandwidth == nil || buffer.record.OpAmp.GainBandwidth.Value <= 0 {
		return fail("no catalog op-amp has complete ADC buffer bandwidth, stability, drive, common-mode, and swing evidence")
	}
	opampEvidence := buffer.record.OpAmp
	if opampEvidence.InputCommonMode == nil || opampEvidence.OutputSwing == nil ||
		opampEvidence.InputCommonMode.NegativeRailHeadroomV == nil || opampEvidence.InputCommonMode.PositiveRailHeadroomV == nil ||
		opampEvidence.OutputSwing.NegativeRailHeadroomV == nil || opampEvidence.OutputSwing.PositiveRailHeadroomV == nil ||
		inputAboveReferenceMinimum < *opampEvidence.InputCommonMode.NegativeRailHeadroomV ||
		inputAboveReferenceMaximum > supplySpanMinimum-*opampEvidence.InputCommonMode.PositiveRailHeadroomV ||
		inputAboveReferenceMinimum < *opampEvidence.OutputSwing.NegativeRailHeadroomV ||
		inputAboveReferenceMaximum > supplySpanMinimum-*opampEvidence.OutputSwing.PositiveRailHeadroomV {
		return fail("selected ADC buffer does not cover the requested input and output range")
	}
	inputStep, _, inputStepOK := firstNumericConstraint(request.Constraints, "maximum_input_step")
	noiseBandwidth, _, noiseBandwidthOK := firstNumericConstraint(request.Constraints, "noise_bandwidth")
	maximumNoise, _, maximumNoiseOK := firstNumericConstraint(request.Constraints, "maximum_integrated_noise")
	loadCurrent, _, loadCurrentOK := firstNumericConstraint(request.Constraints, "maximum_output_load_current")
	ambientTemperature, _, ambientOK := firstNumericConstraint(request.Constraints, "ambient_temperature")
	minimumThermalMargin, _, thermalMarginOK := firstNumericConstraint(request.Constraints, "minimum_thermal_margin")
	stability := opampEvidence.CapacitiveLoadStability
	if !inputStepOK || !noiseBandwidthOK || !maximumNoiseOK || !loadCurrentOK || !ambientOK || !thermalMarginOK ||
		inputStep <= 0 || noiseBandwidth <= 0 || maximumNoise <= 0 || loadCurrent < 0 || minimumThermalMargin < 0 ||
		opampEvidence.SlewRate == nil || opampEvidence.SlewRate.Value <= 0 || opampEvidence.SlewRate.Unit != "V/s" ||
		opampEvidence.OutputCurrent == nil || opampEvidence.OutputCurrent.Value <= 0 || opampEvidence.OutputCurrent.Unit != "A" ||
		opampEvidence.VoltageNoiseDensity == nil || opampEvidence.VoltageNoiseDensity.Value <= 0 || opampEvidence.VoltageNoiseDensity.Unit != "V/sqrt(Hz)" ||
		opampEvidence.NoiseStatus != "proven" || opampEvidence.MaxJunctionTemperatureC == nil || opampEvidence.JunctionToAmbientCPerW == nil ||
		stability == nil || stability.DirectLoadMaximum == nil || stability.DirectLoadMaximum.Unit != "F" ||
		stability.IsolatedLoadMaximum == nil || stability.IsolatedLoadMaximum.Unit != "F" || stability.IsolationResistance == nil ||
		stability.IsolationResistance.Minimum == nil || stability.IsolationResistance.Maximum == nil || stability.IsolationResistance.Unit != "Ohm" ||
		stability.MinimumPhaseMarginDeg == nil || *stability.MinimumPhaseMarginDeg <= 0 {
		return fail("buffered ADC drive requires slew, noise, output-current, thermal, and capacitive-load stability evidence")
	}
	if inputCapacitance > stability.IsolatedLoadMaximum.Value {
		return fail("ADC input capacitance exceeds the op-amp's reviewed isolated-load envelope")
	}
	gbw := opampEvidence.GainBandwidth.Value
	logAccuracy := -math.Log(accuracy)
	smallSignalSettling := logAccuracy / (2 * math.Pi * gbw)
	slewSettling := inputStep / opampEvidence.SlewRate.Value
	opampSettling := math.Max(smallSignalSettling, slewSettling)
	remaining := acquisition - opampSettling
	if remaining <= 0 {
		return fail("selected ADC buffer bandwidth cannot meet the acquisition-time bound")
	}
	maximumResistance := remaining / (inputCapacitance * logAccuracy)
	if maximumResistance < 1 {
		return fail("buffered ADC isolation cannot meet the acquisition-time bound")
	}
	minimumIsolation := *stability.IsolationResistance.Minimum
	maximumIsolation := math.Min(maximumResistance, *stability.IsolationResistance.Maximum)
	if minimumIsolation <= 0 || maximumIsolation < minimumIsolation {
		return fail("op-amp stability and ADC settling isolation-resistance windows do not overlap")
	}
	ideal := math.Min(math.Max(100, minimumIsolation), maximumIsolation)
	values, issues := PreferredValueCandidates(ideal, SeriesE24, minimumIsolation, maximumIsolation, 1)
	if len(issues) != 0 || len(values) != 1 {
		return fail("no preferred buffered ADC isolation resistor satisfies settling")
	}
	seriesResistance := values[0]
	rcSettling := logAccuracy * seriesResistance * inputCapacitance
	settlingTime := opampSettling + rcSettling
	margin := acquisition - settlingTime
	if margin < 0 {
		return fail("buffered ADC drive exceeds the acquisition-time bound")
	}
	integratedNoise := opampEvidence.VoltageNoiseDensity.Value * math.Sqrt(noiseBandwidth)
	requiredOutputCurrent := loadCurrent + inputCapacitance*inputStep/acquisition
	quiescentCurrent, quiescentOK := recordRatingMaximum(buffer.record, "supply_current", "A")
	if !quiescentOK || integratedNoise > maximumNoise || requiredOutputCurrent > opampEvidence.OutputCurrent.Value {
		return fail("selected ADC buffer does not satisfy noise or output-current bounds")
	}
	worstCaseDissipation := supplySpanMaximum * (quiescentCurrent + requiredOutputCurrent)
	junctionTemperature := ambientTemperature + worstCaseDissipation*(*opampEvidence.JunctionToAmbientCPerW)
	thermalMargin := *opampEvidence.MaxJunctionTemperatureC - junctionTemperature
	if thermalMargin < minimumThermalMargin {
		return fail("selected ADC buffer does not satisfy the requested thermal margin")
	}
	buffer.selected.InstanceID, buffer.usage = "adc_buffer", "adc_drive_buffer"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{buffer}, []passivePart{
		{"adc_isolation", "resistor", "adc_drive_isolation", engineeringValue(seriesResistance, "Ohm")},
		{"buffer_bypass", "capacitor", "decoupling_capacitor", "100n"},
	})
	if err != nil {
		return nil, err
	}
	evidence := CalculationEvidence{
		ID: "buffered_adc_drive_settling", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "source_impedance", Value: source, Unit: "Ohm"}, {Name: "adc_input_capacitance", Value: inputCapacitance, Unit: "F"},
			{Name: "acquisition_time", Value: acquisition, Unit: "s"}, {Name: "settling_accuracy", Value: accuracy, Unit: "ratio"},
			{Name: "buffer_gain_bandwidth", Value: gbw, Unit: "Hz"}, {Name: "maximum_input_step", Value: inputStep, Unit: "V"},
			{Name: "buffer_slew_rate", Value: opampEvidence.SlewRate.Value, Unit: "V/s"}, {Name: "noise_bandwidth", Value: noiseBandwidth, Unit: "Hz"},
			{Name: "ambient_temperature", Value: ambientTemperature, Unit: "degC"},
		},
		SelectedValues: []SelectedValueEvidence{{Name: "isolation_resistance", Ideal: ideal, Selected: seriesResistance, Unit: "Ohm", Series: SeriesE24, RelativeError: math.Abs(seriesResistance-ideal) / ideal}},
		NominalOutputs: []NamedQuantity{
			{Name: "small_signal_settling_time", Value: smallSignalSettling, Unit: "s"}, {Name: "slew_settling_time", Value: slewSettling, Unit: "s"},
			{Name: "opamp_settling_time", Value: opampSettling, Unit: "s"}, {Name: "rc_settling_time", Value: rcSettling, Unit: "s"},
			{Name: "settling_time", Value: settlingTime, Unit: "s"}, {Name: "integrated_noise", Value: integratedNoise, Unit: "V"},
			{Name: "required_output_current", Value: requiredOutputCurrent, Unit: "A"}, {Name: "junction_temperature", Value: junctionTemperature, Unit: "degC"},
		},
		Bounds: []CalculationBound{
			maximumBound("acquisition_time", acquisition, settlingTime, "s"), maximumBound("maximum_integrated_noise", maximumNoise, integratedNoise, "V"),
			maximumBound("output_current", opampEvidence.OutputCurrent.Value, requiredOutputCurrent, "A"), minimumBound("minimum_thermal_margin", minimumThermalMargin, thermalMargin, "degC"),
			maximumBound("isolated_capacitive_load", stability.IsolatedLoadMaximum.Value, inputCapacitance, "F"),
			minimumBound("isolation_resistance_minimum", minimumIsolation, seriesResistance, "Ohm"), maximumBound("isolation_resistance_maximum", *stability.IsolationResistance.Maximum, seriesResistance, "Ohm"),
			minimumBound("minimum_phase_margin", *stability.MinimumPhaseMarginDeg, *stability.MinimumPhaseMarginDeg, "deg"),
		},
		WorstMargin: quantize(minimumFinite(
			margin/acquisition,
			(maximumNoise-integratedNoise)/maximumNoise,
			(opampEvidence.OutputCurrent.Value-requiredOutputCurrent)/opampEvidence.OutputCurrent.Value,
			(thermalMargin-minimumThermalMargin)/math.Max(minimumThermalMargin, 1),
			(stability.IsolatedLoadMaximum.Value-inputCapacitance)/stability.IsolatedLoadMaximum.Value,
		)), Pass: true,
	}
	evidence, err = FinalizeCalculation(evidence)
	if err != nil {
		return fail("could not finalize buffered ADC settling evidence")
	}
	bindings := bindRoles(request.Ports, buffer.selected.InstanceID, map[string]string{"input": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	for index := range bindings {
		if bindings[index].Role == "output" {
			bindings[index].Instance, bindings[index].Function = "adc_isolation", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("adc_buffer_output", "analog_signal", endpoint(buffer, "OUT"), endpoint(buffer, "IN_MINUS"), passiveEndpoint("adc_isolation", "A")),
		semanticNet("adc_buffer_power", "power", endpoint(buffer, "V_PLUS"), passiveEndpoint("buffer_bypass", "A")),
		semanticNet("adc_buffer_reference", "reference", endpoint(buffer, "V_MINUS"), passiveEndpoint("buffer_bypass", "B")),
	}
	return provider.expansion(request, "buffered_adc_drive_conditioning", parts, bindings, connections, []CalculationEvidence{evidence}, 0)
}

func minimumFinite(values ...float64) float64 {
	minimum := math.Inf(1)
	for _, value := range values {
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			minimum = math.Min(minimum, value)
		}
	}
	return minimum
}

func (provider *CatalogProvider) selectProvenADCBuffer(ratings []components.RequiredRating) (catalogPart, bool) {
	type candidate struct {
		part catalogPart
		area float64
	}
	var candidates []candidate
	for _, record := range provider.catalog.Records {
		evidence := record.OpAmp
		if record.Family != "opamp" || record.Generic || evidence == nil || !recordSupportsRatings(record, ratings) ||
			evidence.OutputDriveStatus != "proven" || evidence.LoadCompatibilityStatus != "proven" ||
			evidence.GainBandwidthStatus != "proven" || evidence.StabilityStatus != "proven" ||
			evidence.InputCommonModeStatus != "proven" || evidence.OutputSwingStatus != "proven" || evidence.NoiseStatus != "proven" ||
			evidence.SlewRate == nil || evidence.OutputCurrent == nil || evidence.VoltageNoiseDensity == nil ||
			evidence.MaxJunctionTemperatureC == nil || evidence.JunctionToAmbientCPerW == nil || evidence.CapacitiveLoadStability == nil ||
			!recordHasFunction(record, "IN_PLUS") || !recordHasFunction(record, "IN_MINUS") || !recordHasFunction(record, "OUT") || !recordHasFunction(record, "V_PLUS") || !recordHasFunction(record, "V_MINUS") {
			continue
		}
		for _, variant := range record.Packages {
			if variant.DimensionsMM == nil || confidenceRank(EvidenceConfidence(variant.Verification.Confidence)) < confidenceRank(EvidenceRuleInferred) {
				continue
			}
			contractEvidence := componentEvidence(record, variant.Verification.Confidence)
			candidates = append(candidates, candidate{part: catalogPart{selected: SelectedComponent{CatalogID: record.ID, VariantID: variant.ID, Evidence: contractEvidence.Confidence}, record: record, evidence: contractEvidence}, area: variant.DimensionsMM.Width * variant.DimensionsMM.Height})
		}
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		if left.area < right.area {
			return -1
		}
		if left.area > right.area {
			return 1
		}
		if order := strings.Compare(left.part.record.ID, right.part.record.ID); order != 0 {
			return order
		}
		return strings.Compare(left.part.selected.VariantID, right.part.selected.VariantID)
	})
	if len(candidates) == 0 {
		return catalogPart{}, false
	}
	return candidates[0].part, true
}
