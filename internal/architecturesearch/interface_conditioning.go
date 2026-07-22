package architecturesearch

import (
	"context"
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
	if clock {
		code = CodeInterfaceClockConditionUnproven
		fragmentID = "clock_source_series_conditioning"
		if maximumProtocolFrequency(request.Ports) <= 0 {
			if frequency, _, ok := firstNumericConstraint(request.Constraints, "clock_frequency"); !ok || frequency <= 0 {
				return nil, &interfaceSynthesisError{code: code, message: "clock conditioning requires bounded frequency evidence"}
			}
		}
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
	return provider.expansion(request, fragmentID, parts, bindings, nil, []CalculationEvidence{evidence}, 0)
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
	bindings := bindRoles(request.Ports, "adc_isolation", roleFunctions)
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
	gbw := opampEvidence.GainBandwidth.Value
	logAccuracy := -math.Log(accuracy)
	opampSettling := logAccuracy / (2 * math.Pi * gbw)
	remaining := acquisition - opampSettling
	if remaining <= 0 {
		return fail("selected ADC buffer bandwidth cannot meet the acquisition-time bound")
	}
	maximumResistance := remaining / (inputCapacitance * logAccuracy)
	if maximumResistance < 1 {
		return fail("buffered ADC isolation cannot meet the acquisition-time bound")
	}
	ideal := math.Min(100, maximumResistance)
	values, issues := PreferredValueCandidates(ideal, SeriesE24, 1, maximumResistance, 1)
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
		Inputs:         []NamedQuantity{{Name: "source_impedance", Value: source, Unit: "Ohm"}, {Name: "adc_input_capacitance", Value: inputCapacitance, Unit: "F"}, {Name: "acquisition_time", Value: acquisition, Unit: "s"}, {Name: "settling_accuracy", Value: accuracy, Unit: "ratio"}, {Name: "buffer_gain_bandwidth", Value: gbw, Unit: "Hz"}},
		SelectedValues: []SelectedValueEvidence{{Name: "isolation_resistance", Ideal: ideal, Selected: seriesResistance, Unit: "Ohm", Series: SeriesE24, RelativeError: math.Abs(seriesResistance-ideal) / ideal}},
		NominalOutputs: []NamedQuantity{{Name: "opamp_settling_time", Value: opampSettling, Unit: "s"}, {Name: "rc_settling_time", Value: rcSettling, Unit: "s"}, {Name: "settling_time", Value: settlingTime, Unit: "s"}},
		Bounds:         []CalculationBound{maximumBound("acquisition_time", acquisition, settlingTime, "s")}, WorstMargin: quantize(margin / acquisition), Pass: true,
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
			evidence.InputCommonModeStatus != "proven" || evidence.OutputSwingStatus != "proven" ||
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
