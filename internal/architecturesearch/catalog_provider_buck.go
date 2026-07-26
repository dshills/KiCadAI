package architecturesearch

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	buckSwitchingFrequencyHz = 500_000
	buckRippleFraction       = 0.30
	buckOutputCapacitanceF   = 220e-6
	buckFeedbackBottomOhm    = 10_000
	// The averaged first-order current-mode model has no finite -180-degree
	// phase crossing. Reports use this finite lower-bound sentinel because
	// calculation evidence deliberately rejects infinities.
	buckNoPhaseCrossingGainMarginDB = 240.0
)

func (provider *CatalogProvider) expandSynchronousBuckConversion(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	outputV, _, ok := firstNumericConstraint(request.Constraints, "output_voltage")
	if !ok || outputV <= 0 {
		return nil, fmt.Errorf("efficient voltage conversion requires a positive output-voltage target")
	}
	inputMinimumV, inputMaximumV, ok := roleVoltageRange(request.Ports, "input")
	if !ok || inputMinimumV <= 0 || inputMaximumV <= outputV {
		return nil, fmt.Errorf("efficient voltage conversion requires a bounded step-down input envelope")
	}
	if operatingMinimum, operatingMaximum, found := numericConstraintBounds(request.Constraints, "input_supply_voltage"); found {
		inputMinimumV = math.Min(inputMinimumV, operatingMinimum)
		inputMaximumV = math.Max(inputMaximumV, operatingMaximum)
	}
	outputCurrentA, _, ok := firstNumericConstraint(request.Constraints, "continuous_output_current", "output_current")
	if !ok {
		outputCurrentA = requiredRoleCurrentA(request.Ports, "output")
	}
	if outputCurrentA <= 0 {
		return nil, fmt.Errorf("efficient voltage conversion requires a positive output-current bound")
	}

	converter, err := provider.selectComponentMaximizingModelUncertaintyMinimumWithTemperature(
		ctx,
		"regulator",
		"buck",
		[]components.RequiredRating{
			{Kind: "input_voltage", Value: numericString(inputMinimumV), Unit: "V"},
			{Kind: "input_voltage", Value: numericString(inputMaximumV), Unit: "V"},
			{Kind: "output_current", Value: numericString(outputCurrentA), Unit: "A"},
		},
		true,
		temperatureRequirementFromConstraints(request.Constraints),
		nil,
		simmodel.PrimitiveSynchronousBuckRegulatorV1,
		"conversion_efficiency_fraction",
		"model_parameters.conversion_efficiency_fraction",
	)
	if err != nil {
		return nil, fmt.Errorf("no reviewed synchronous-buck controller proves the requested voltage and current envelope: %w", err)
	}
	if !catalogRecordHasSimulationModel(converter.record, simmodel.PrimitiveSynchronousBuckRegulatorV1) {
		return nil, fmt.Errorf("no reviewed synchronous-buck controller proves the requested voltage and current envelope")
	}
	converter.selected.InstanceID, converter.usage = "buck_controller", "synchronous_buck_controller"
	converter.parameters = []RealizationParameter{
		{Name: "nominal_input_voltage_v", Value: (inputMinimumV + inputMaximumV) / 2, Unit: "V"},
		{Name: "nominal_output_voltage_v", Value: outputV, Unit: "V"},
		{Name: "switching_frequency_hz", Value: buckSwitchingFrequencyHz, Unit: "Hz"},
	}

	inductor, inductorRippleA, peakInductorCurrentA, err := provider.selectBuckInductor(
		ctx, inputMaximumV, outputV, outputCurrentA, buckSwitchingFrequencyHz,
	)
	if err != nil {
		return nil, err
	}
	outputCapacitor, outputESROhm, err := provider.selectBuckOutputCapacitor(outputV, inductorRippleA)
	if err != nil {
		return nil, err
	}
	inputBulk, _, err := provider.selectReviewedLowESRCapacitor(220e-6, inputMaximumV, 0)
	if err != nil {
		return nil, fmt.Errorf("select reviewed buck input bulk capacitor: %w", err)
	}
	inputBulk.selected.InstanceID, inputBulk.usage, inputBulk.value = "buck_input_bulk", "input_bulk_capacitor", "220u"
	inputBulk.near, inputBulk.maxDistanceMM = converter.selected.InstanceID, 3

	inputBypass, err := provider.selectComponent(ctx, "capacitor", "100n", []components.RequiredRating{
		{Kind: "voltage", Value: numericString(inputMaximumV), Unit: "V"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("select reviewed buck high-frequency input bypass: %w", err)
	}
	inputBypass.selected.InstanceID, inputBypass.usage, inputBypass.value = "buck_input_bypass", "input_bypass_capacitor", "100n"
	inputBypass.near, inputBypass.maxDistanceMM = converter.selected.InstanceID, 2

	feedbackReferenceV, referenceOK := catalogSimulationParameter(converter.record, "reference_voltage_v")
	controlTransconductanceS, transconductanceOK := catalogSimulationParameter(converter.record, "control_transconductance_s")
	if !referenceOK || !transconductanceOK || feedbackReferenceV <= 0 || controlTransconductanceS <= 0 {
		return nil, fmt.Errorf("selected synchronous buck lacks bounded feedback-reference or load-regulation evidence")
	}
	// The averaged catalog model exposes finite DC control transconductance.
	// Program the divider against its bounded full-load feedback error so the
	// requested output is the regulated full-load operating point rather than
	// the unloaded intercept. Dynamic verification still proves every declared
	// load and tolerance corner, and may use the bounded divider repair below.
	fullLoadFeedbackV := feedbackReferenceV - outputCurrentA/controlTransconductanceS
	if fullLoadFeedbackV <= 0 {
		return nil, fmt.Errorf("selected synchronous buck cannot regulate the requested full-load current")
	}
	feedbackRatio := outputV / fullLoadFeedbackV
	if _, outputMaximumV, boundedOutput := roleVoltageRange(request.Ports, "output"); boundedOutput && outputMaximumV > outputV {
		_, referenceMaximumV := catalogUncertaintyInterval(
			converter.record, "model_parameters.reference_voltage_v", feedbackReferenceV,
		)
		const (
			feedbackResistanceToleranceFraction = .001
			upperHeadroomUseFraction            = .8
		)
		// Maximize lower-corner robustness without consuming the entire upper
		// voltage allowance. The bound includes the catalog reference maximum
		// and opposing divider tolerances; retaining twenty percent of the
		// remaining ratio headroom prevents nominal sizing from riding a limit.
		maximumSafeRatio := 1 + (outputMaximumV/referenceMaximumV-1)*
			(1-feedbackResistanceToleranceFraction)/(1+feedbackResistanceToleranceFraction)
		if maximumSafeRatio > feedbackRatio {
			feedbackRatio += upperHeadroomUseFraction * (maximumSafeRatio - feedbackRatio)
		}
	}
	feedbackTopOhm := buckFeedbackBottomOhm * (feedbackRatio - 1)
	if feedbackTopOhm <= 0 {
		return nil, fmt.Errorf("selected synchronous buck requires output voltage above its reviewed feedback reference")
	}
	feedbackFilterCapacitanceF, feedbackFilterPoleHz, err := buckFeedbackFilter(
		request, buckFeedbackBottomOhm,
	)
	if err != nil {
		return nil, err
	}
	passives := []passivePart{
		{"buck_output_esr", "resistor", "output_capacitor_esr", engineeringValue(outputESROhm, "Ohm")},
		{"buck_feedback_top", "resistor", "feedback_divider", engineeringValue(feedbackTopOhm, "Ohm")},
		{"buck_feedback_bottom", "resistor", "feedback_divider", engineeringValue(buckFeedbackBottomOhm, "Ohm")},
		{"buck_bootstrap", "capacitor", "bootstrap_capacitor", "100n"},
		{"buck_vcc_bypass", "capacitor", "controller_bypass_capacitor", "1u"},
		{"buck_soft_start", "capacitor", "soft_start_capacitor", "100n"},
		{"buck_rt", "resistor", "frequency_programming", "90.9k"},
		{"buck_pgood_pullup", "resistor", "open_collector_pullup", "10k"},
	}
	if feedbackFilterCapacitanceF > 0 {
		passives = append(passives, passivePart{
			"buck_feedback_filter", "capacitor", "loop_compensation",
			engineeringValue(feedbackFilterCapacitanceF, "F"),
		})
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{converter, inductor, outputCapacitor, inputBulk, inputBypass}, passives)
	if err != nil {
		return nil, err
	}
	for index := range parts {
		switch parts[index].selected.InstanceID {
		case "buck_inductor", "buck_bootstrap", "buck_vcc_bypass", "buck_input_bypass":
			parts[index].near, parts[index].maxDistanceMM = converter.selected.InstanceID, 3
		case "buck_feedback_top", "buck_feedback_bottom", "buck_feedback_filter":
			parts[index].near, parts[index].maxDistanceMM = converter.selected.InstanceID, 4
		}
	}

	calculations, err := buckArchitectureCalculations(
		request, converter.record, inputMinimumV, inputMaximumV, outputV, outputCurrentA,
		inductorRippleA, peakInductorCurrentA, buckOutputCapacitanceF, outputESROhm,
		feedbackFilterCapacitanceF, feedbackFilterPoleHz,
	)
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, converter.selected.InstanceID, map[string]string{
		"input": "PVIN", "output": "SW", "reference": "PGND",
	})
	for index := range bindings {
		if bindings[index].Role == "output" {
			bindings[index].Instance, bindings[index].Function = "buck_inductor", "B"
		}
	}
	inputEndpoints := []RealizationEndpoint{
		endpoint(converter, "PVIN"),
		passiveEndpoint("buck_input_bulk", "A"),
		passiveEndpoint("buck_input_bypass", "A"),
		endpoint(converter, "EN"),
	}
	inputEndpoints = appendCatalogFunctions(inputEndpoints, converter, "PVIN_AUX")
	switchEndpoints := []RealizationEndpoint{
		endpoint(converter, "SW"),
		passiveEndpoint("buck_inductor", "A"),
		passiveEndpoint("buck_bootstrap", "B"),
	}
	switchEndpoints = appendCatalogFunctions(switchEndpoints, converter, "SW_AUX1", "SW_AUX2")
	referenceEndpoints := []RealizationEndpoint{
		endpoint(converter, "AGND"),
		endpoint(converter, "PGND"),
		endpoint(converter, "SYNC_MODE"),
		passiveEndpoint("buck_input_bulk", "B"),
		passiveEndpoint("buck_input_bypass", "B"),
		passiveEndpoint("buck_output_capacitor", "B"),
		passiveEndpoint("buck_feedback_bottom", "B"),
		passiveEndpoint("buck_vcc_bypass", "B"),
		passiveEndpoint("buck_soft_start", "B"),
		passiveEndpoint("buck_rt", "B"),
	}
	if feedbackFilterCapacitanceF > 0 {
		referenceEndpoints = append(referenceEndpoints, passiveEndpoint("buck_feedback_filter", "B"))
	}
	referenceEndpoints = appendCatalogFunctions(
		referenceEndpoints,
		converter,
		"PGND_AUX", "NC_GND_19", "NC_GND_27", "NC_GND_28", "NC_GND_29", "NC_GND_30",
	)
	connections := []RealizationConnection{
		semanticNet("buck_input", "switching_current",
			inputEndpoints...),
		semanticNet("buck_switch_node", "switching_current",
			switchEndpoints...),
		semanticNet("buck_output", "regulated_power",
			passiveEndpoint("buck_inductor", "B"), passiveEndpoint("buck_output_esr", "A"),
			passiveEndpoint("buck_feedback_top", "A"), endpoint(converter, "BIAS"),
			passiveEndpoint("buck_pgood_pullup", "A")),
		semanticNet("buck_output_capacitor_internal", "power",
			passiveEndpoint("buck_output_esr", "B"), passiveEndpoint("buck_output_capacitor", "A")),
		semanticNet("buck_feedback", "feedback",
			passiveEndpoint("buck_feedback_top", "B"), passiveEndpoint("buck_feedback_bottom", "A"), endpoint(converter, "FB")),
		semanticNet("buck_bootstrap_supply", "switching_drive",
			endpoint(converter, "BOOT"), passiveEndpoint("buck_bootstrap", "A")),
		semanticNet("buck_controller_supply", "power",
			endpoint(converter, "VCC"), passiveEndpoint("buck_vcc_bypass", "A")),
		semanticNet("buck_power_good", "digital_signal",
			endpoint(converter, "PGOOD"), passiveEndpoint("buck_pgood_pullup", "B")),
		semanticNet("buck_reference", "reference",
			referenceEndpoints...),
		semanticNet("buck_soft_start_node", "analog_control",
			endpoint(converter, "SS_TRK"), passiveEndpoint("buck_soft_start", "A")),
		semanticNet("buck_frequency_program", "analog_control",
			endpoint(converter, "RT"), passiveEndpoint("buck_rt", "A")),
	}
	if feedbackFilterCapacitanceF > 0 {
		for index := range connections {
			if connections[index].ID == "buck_feedback" {
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint("buck_feedback_filter", "A"))
				break
			}
		}
	}
	repairs := []RealizationRepairVariable{{
		ID: "buck_feedback_top_resistance", Kind: "bias", Instance: "buck_feedback_top",
		Value: feedbackTopOhm, AllowedValues: precisionResistanceRepairValues(feedbackTopOhm), Unit: "Ohm",
		Effects: []RealizationRepairEffect{{
			Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "dc_voltage", Direction: "metric_increases",
		}},
	}}
	if feedbackFilterCapacitanceF > 0 {
		repairs = append(repairs, RealizationRepairVariable{
			ID: "buck_feedback_filter_capacitance", Kind: "compensation", Instance: "buck_feedback_filter",
			Value: feedbackFilterCapacitanceF, AllowedValues: preferredRepairValues(feedbackFilterCapacitanceF), Unit: "F",
			Effects: []RealizationRepairEffect{
				{Analysis: simmodel.AnalysisStability, Metric: "loop_crossover_frequency", Direction: "metric_decreases"},
				{Analysis: simmodel.AnalysisStability, Metric: "phase_margin", Direction: "metric_decreases"},
			},
		})
	}
	expansions, err := provider.expansionWithRepairs(
		request, "catalog_synchronous_buck_current_mode", parts, bindings, connections, calculations, repairs, 0,
	)
	if err != nil {
		return nil, err
	}
	converterCapacityA, converterCapacityOK := recordRatingMaximum(converter.record, "output_current", "A")
	inductorCapacityA, inductorCapacityOK := recordRatingMaximum(inductor.record, "rms_current", "A")
	if !converterCapacityOK || !inductorCapacityOK {
		return nil, fmt.Errorf("selected synchronous buck lacks bounded continuous output-current evidence")
	}
	outputCapacityA := math.Min(converterCapacityA, inductorCapacityA)
	minimumEfficiency, efficiencyOK := recordMinimumEfficiency(converter.record)
	quiescentCurrent, quiescentOK := recordSupplyCurrentA(converter.record)
	if !efficiencyOK || !quiescentOK {
		return nil, fmt.Errorf("selected synchronous buck lacks bounded input-current evidence")
	}
	inputDemandA := outputV*outputCurrentA/(inputMinimumV*minimumEfficiency) + quiescentCurrent
	for expansionIndex := range expansions {
		for portIndex := range expansions[expansionIndex].OfferedPorts {
			switch expansions[expansionIndex].OfferedPorts[portIndex].Role {
			case "input":
				expansions[expansionIndex].OfferedPorts[portIndex].Contract.CurrentDemandA = float64Pointer(inputDemandA)
			case "output":
				expansions[expansionIndex].OfferedPorts[portIndex].Contract.CurrentCapacityA = float64Pointer(outputCapacityA)
			}
		}
	}
	return expansions, nil
}

func (provider *CatalogProvider) selectBuckInductor(
	ctx context.Context,
	inputMaximumV, outputV, outputCurrentA, switchingFrequencyHz float64,
) (catalogPart, float64, float64, error) {
	rippleTargetA := math.Max(.05, outputCurrentA*buckRippleFraction)
	idealH := outputV * (1 - outputV/inputMaximumV) / (switchingFrequencyHz * rippleTargetA)
	candidates, issues := PreferredValueCandidates(idealH, SeriesE12, idealH/4, idealH*4, 8)
	if len(issues) != 0 {
		return catalogPart{}, 0, 0, fmt.Errorf("buck inductor preferred-value selection failed")
	}
	for _, candidateH := range candidates {
		rippleA := outputV * (1 - outputV/inputMaximumV) / (switchingFrequencyHz * candidateH)
		peakA := outputCurrentA + rippleA/2
		part, err := provider.selectPassiveComponentWithRatings(ctx, "inductor", "inductance", numericString(candidateH), []components.RequiredRating{
			{Kind: "rms_current", Value: numericString(outputCurrentA), Unit: "A"},
			{Kind: "saturation_current", Value: numericString(peakA * 1.2), Unit: "A"},
		})
		if err != nil || !catalogRecordHasSimulationModel(part.record, simmodel.PrimitiveInductorTransientV1) {
			continue
		}
		part.selected.InstanceID, part.usage, part.value = "buck_inductor", "switching_inductor", engineeringValue(candidateH, "H")
		return part, rippleA, peakA, nil
	}
	return catalogPart{}, 0, 0, fmt.Errorf("no reviewed inductor proves the calculated ripple, RMS-current, and saturation-current envelope")
}

func (provider *CatalogProvider) selectBuckOutputCapacitor(outputV, rippleA float64) (catalogPart, float64, error) {
	part, esr, err := provider.selectReviewedLowESRCapacitor(buckOutputCapacitanceF, outputV*1.2, rippleA/math.Sqrt(12))
	if err != nil {
		return catalogPart{}, 0, err
	}
	capacitance, ok := recordValue(part.record, "capacitance", "F")
	if !ok || math.Abs(capacitance-buckOutputCapacitanceF) > buckOutputCapacitanceF*1e-9 {
		return catalogPart{}, 0, fmt.Errorf("selected buck output capacitor does not match the deterministic capacitance target")
	}
	part.selected.InstanceID, part.usage, part.value = "buck_output_capacitor", "low_esr_output_capacitor", "220u"
	return part, esr, nil
}

func (provider *CatalogProvider) selectReviewedLowESRCapacitor(capacitanceF, minimumVoltageV, minimumRippleA float64) (catalogPart, float64, error) {
	type candidate struct {
		record components.ComponentRecord
		esr    float64
	}
	var candidates []candidate
	for _, record := range provider.catalog.Records {
		if record.Family != "capacitor" || record.Generic || record.Capacitor == nil ||
			record.Capacitor.ESR == nil || record.Capacitor.ESRReview != "proven" ||
			len(record.Symbols) == 0 || len(record.Packages) == 0 {
			continue
		}
		value, valueOK := recordValue(record, "capacitance", "F")
		voltage, voltageOK := recordRatingMaximum(record, "voltage", "V")
		esr, esrOK := convertCatalogUnit(record.Capacitor.ESR.Value, record.Capacitor.ESR.Unit, "Ohm")
		if !valueOK || math.Abs(value-capacitanceF) > capacitanceF*1e-9 ||
			!voltageOK || voltage < minimumVoltageV || !esrOK || esr <= 0 {
			continue
		}
		if minimumRippleA > 0 {
			if record.Capacitor.RippleCurrent == nil {
				continue
			}
			ripple, rippleOK := convertCatalogUnit(record.Capacitor.RippleCurrent.Value, record.Capacitor.RippleCurrent.Unit, "A")
			if !rippleOK && strings.EqualFold(strings.TrimSpace(record.Capacitor.RippleCurrent.Unit), "A_rms") {
				ripple, rippleOK = record.Capacitor.RippleCurrent.Value, true
			}
			if !rippleOK || ripple < minimumRippleA {
				continue
			}
		}
		candidates = append(candidates, candidate{record: record, esr: esr})
	}
	slices.SortStableFunc(candidates, func(left, right candidate) int {
		switch {
		case left.esr < right.esr:
			return -1
		case left.esr > right.esr:
			return 1
		default:
			return strings.Compare(left.record.ID, right.record.ID)
		}
	})
	if len(candidates) == 0 {
		return catalogPart{}, 0, fmt.Errorf("no concrete low-ESR capacitor proves the requested capacitance, voltage, and ripple envelope")
	}
	selected := candidates[0]
	evidence := componentEvidence(selected.record, selected.record.Verification.Confidence)
	return catalogPart{
		selected: SelectedComponent{
			InstanceID: canonicalIdentifier(selected.record.Family),
			CatalogID:  selected.record.ID,
			VariantID:  selected.record.Packages[0].ID,
			Evidence:   evidence.Confidence,
		},
		record: selected.record, usage: "capacitor", evidence: evidence,
	}, selected.esr, nil
}

func appendCatalogFunctions(endpoints []RealizationEndpoint, part catalogPart, functions ...string) []RealizationEndpoint {
	for _, function := range functions {
		if recordHasFunction(part.record, function) {
			endpoints = append(endpoints, endpoint(part, function))
		}
	}
	return endpoints
}

func buckFeedbackFilter(request ProviderRequest, lowerResistanceOhm float64) (float64, float64, error) {
	minimumCrossoverHz, maximumCrossoverHz, bounded := numericConstraintBounds(
		request.Constraints, "loop_crossover_frequency",
	)
	if !bounded || maximumCrossoverHz <= 0 {
		return 0, 0, nil
	}
	minimumPhaseMarginDeg, _ := numericConstraintLowerBound(request.Constraints, "phase_margin")
	// Keep the passive feedback pole well below the maximum permitted
	// crossover. The separation is derived from the requested phase margin,
	// with six-to-one as the conservative floor for requirements that omit a
	// phase bound. This network is across the lower divider resistor, so it
	// changes only the AC loop return and preserves the programmed DC output.
	separation := math.Max(6, minimumPhaseMarginDeg/10)
	filterPoleHz := maximumCrossoverHz / separation
	idealCapacitanceF := 1 / (2 * math.Pi * lowerResistanceOhm * filterPoleHz)
	candidates, issues := PreferredValueCandidates(
		idealCapacitanceF, SeriesE12, idealCapacitanceF*.5, idealCapacitanceF*2, 1,
	)
	if len(issues) != 0 || len(candidates) == 0 {
		return 0, 0, fmt.Errorf("synchronous-buck feedback-filter value solution failed")
	}
	selectedCapacitanceF := candidates[0]
	selectedPoleHz := 1 / (2 * math.Pi * lowerResistanceOhm * selectedCapacitanceF)
	if minimumCrossoverHz > 0 && selectedPoleHz <= 0 {
		return 0, 0, fmt.Errorf("synchronous-buck feedback-filter solution is outside the requested crossover envelope")
	}
	return selectedCapacitanceF, selectedPoleHz, nil
}

func buckArchitectureCalculations(
	request ProviderRequest,
	record components.ComponentRecord,
	inputMinimumV, inputMaximumV, outputV, outputCurrentA, rippleA, peakCurrentA, capacitanceF, esrOhm float64,
	feedbackFilterCapacitanceF, feedbackFilterPoleHz float64,
) ([]CalculationEvidence, error) {
	nominalEfficiency, ok := catalogSimulationParameter(record, "conversion_efficiency_fraction")
	if !ok {
		return nil, fmt.Errorf("selected synchronous buck lacks normalized efficiency evidence")
	}
	minimumEfficiency, _ := catalogUncertaintyInterval(
		record,
		"model_parameters.conversion_efficiency_fraction",
		nominalEfficiency,
	)
	gm, gmOK := catalogSimulationParameter(record, "control_transconductance_s")
	poleHz, poleOK := catalogSimulationParameter(record, "control_pole_hz")
	theta, thetaOK := catalogSimulationParameter(record, "junction_to_ambient_c_per_w")
	highR, highOK := catalogSimulationParameter(record, "high_side_on_resistance_ohm")
	lowR, lowOK := catalogSimulationParameter(record, "low_side_on_resistance_ohm")
	transitionS, transitionOK := catalogSimulationParameter(record, "switch_transition_time_s")
	quiescentA, quiescentOK := catalogSimulationParameter(record, "quiescent_current_a")
	peakCurrentLimitA, currentLimitOK := catalogSimulationParameter(record, "peak_current_limit_a")
	if !gmOK || !poleOK || !thetaOK || !highOK || !lowOK || !transitionOK || !quiescentOK || !currentLimitOK {
		return nil, fmt.Errorf("selected synchronous buck lacks complete control-loop or loss-model evidence")
	}
	rippleV := rippleA*esrOhm + rippleA/(8*buckSwitchingFrequencyHz*capacitanceF)
	beta := 1 / outputV
	loadOhm := outputV / outputCurrentA
	crossoverHz, phaseMarginDeg, ok := currentModeBuckLoopMargins(gm, poleHz, beta, loadOhm, capacitanceF)
	if !ok {
		return nil, fmt.Errorf("selected synchronous buck control loop has no deterministic unity-gain crossing")
	}
	dutyMaximum := outputV / inputMinimumV
	conductionW := outputCurrentA * outputCurrentA * (dutyMaximum*highR + (1-dutyMaximum)*lowR)
	switchingW := .5 * inputMaximumV * peakCurrentA * transitionS * buckSwitchingFrequencyHz
	dissipationW := conductionW + switchingW + inputMaximumV*quiescentA
	ambientMaximumC := 85.0
	if _, maximum, found := numericConstraintBounds(request.Constraints, "ambient_temperature"); found {
		ambientMaximumC = maximum
	}
	junctionC := ambientMaximumC + dissipationW*theta
	soaDurationS, _, hasSOADuration := firstNumericConstraint(request.Constraints, "transient_soa_duration")
	soaBoundaryA, soaOK := catalogTransientSOACurrent(record, soaDurationS, inputMaximumV)
	if !soaOK {
		return nil, fmt.Errorf("selected synchronous buck lacks a transient SOA envelope covering %.9g s at %.9g V", soaDurationS, inputMaximumV)
	}
	soaDemandA := peakCurrentA
	if hasSOADuration {
		soaDemandA = peakCurrentLimitA
	}
	soaMargin := soaBoundaryA / soaDemandA

	if maximum, _, found := firstNumericConstraint(request.Constraints, "peak_to_peak_ripple"); found && rippleV > maximum {
		return nil, fmt.Errorf("calculated buck ripple %.9g V exceeds required maximum %.9g V", rippleV, maximum)
	}
	if minimum, _, found := firstNumericConstraint(request.Constraints, "conversion_efficiency"); found && minimumEfficiency*100 < minimum {
		return nil, fmt.Errorf("reviewed buck minimum efficiency %.9g%% is below required minimum %.9g%%", minimumEfficiency*100, minimum)
	}
	if minimum, _, found := firstNumericConstraint(request.Constraints, "phase_margin"); found && phaseMarginDeg < minimum {
		return nil, fmt.Errorf("calculated buck phase margin %.9g deg is below required minimum %.9g deg", phaseMarginDeg, minimum)
	}
	if maximum, _, found := firstNumericConstraint(request.Constraints, "peak_junction_temperature"); found && junctionC > maximum {
		return nil, fmt.Errorf("calculated buck junction temperature %.9g C exceeds required maximum %.9g C", junctionC, maximum)
	}
	if minimum, _, found := firstNumericConstraint(request.Constraints, "transient_soa_margin"); found && soaMargin < minimum &&
		!hasEventScopedConstraint(request.Constraints, "transient_soa_margin") {
		return nil, fmt.Errorf("calculated buck transient SOA margin %.9g is below required minimum %.9g", soaMargin, minimum)
	}

	calculation, err := ObservedCalculation("synchronous_buck_dynamic_envelope",
		NamedQuantity{Name: "minimum_input_voltage", Value: inputMinimumV, Unit: "V"},
		NamedQuantity{Name: "maximum_input_voltage", Value: inputMaximumV, Unit: "V"},
		NamedQuantity{Name: "output_voltage", Value: outputV, Unit: "V"},
		NamedQuantity{Name: "output_current", Value: outputCurrentA, Unit: "A"},
		NamedQuantity{Name: "inductor_ripple_current", Value: rippleA, Unit: "A"},
		NamedQuantity{Name: "peak_inductor_current", Value: peakCurrentA, Unit: "A"},
		NamedQuantity{Name: "output_ripple", Value: rippleV, Unit: "V"},
		NamedQuantity{Name: "feedback_filter_capacitance", Value: feedbackFilterCapacitanceF, Unit: "F"},
		NamedQuantity{Name: "feedback_filter_pole", Value: feedbackFilterPoleHz, Unit: "Hz"},
		NamedQuantity{Name: "nominal_conversion_efficiency", Value: nominalEfficiency * 100, Unit: "%"},
		NamedQuantity{Name: "conversion_efficiency", Value: minimumEfficiency * 100, Unit: "%"},
		NamedQuantity{Name: "loop_crossover_frequency", Value: crossoverHz, Unit: "Hz"},
		NamedQuantity{Name: "phase_margin", Value: phaseMarginDeg, Unit: "deg"},
		NamedQuantity{Name: "gain_margin", Value: buckNoPhaseCrossingGainMarginDB, Unit: "dB"},
		NamedQuantity{Name: "peak_junction_temperature", Value: junctionC, Unit: "degC"},
		NamedQuantity{Name: "transient_soa_margin", Value: soaMargin, Unit: "ratio"},
	)
	if err != nil {
		return nil, err
	}
	return []CalculationEvidence{calculation}, nil
}

func hasEventScopedConstraint(constraints []Constraint, metric string) bool {
	suffix := "_" + derivedSemanticIdentifier(metric)
	for _, constraint := range constraints {
		name := derivedSemanticIdentifier(constraint.Name)
		if name != derivedSemanticIdentifier(metric) && strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func catalogTransientSOACurrent(record components.ComponentRecord, durationS, voltageV float64) (float64, bool) {
	if !finiteNumbers(durationS) || durationS < 0 || !finitePositive(voltageV) {
		return 0, false
	}
	for _, model := range record.SimulationModels {
		if model.ModelID != simmodel.PrimitiveSynchronousBuckRegulatorV1 {
			continue
		}
		var selected *simmodel.TransientSOAEnvelope
		for index := range model.TransientSOA {
			envelope := &model.TransientSOA[index]
			if durationS > 0 && envelope.PulseDurationS != nil && *envelope.PulseDurationS >= durationS-1e-15 {
				if selected == nil || *envelope.PulseDurationS < *selected.PulseDurationS {
					selected = envelope
				}
			}
		}
		if selected == nil {
			for index := range model.TransientSOA {
				if model.TransientSOA[index].DC {
					selected = &model.TransientSOA[index]
					break
				}
			}
		}
		if selected == nil || len(selected.Points) < 2 || voltageV > selected.Points[len(selected.Points)-1].VoltageV {
			return 0, false
		}
		if voltageV <= selected.Points[0].VoltageV {
			return selected.Points[0].CurrentA, true
		}
		for index := 1; index < len(selected.Points); index++ {
			right := selected.Points[index]
			if voltageV > right.VoltageV {
				continue
			}
			left := selected.Points[index-1]
			if !finitePositive(left.VoltageV) || !finitePositive(right.VoltageV) ||
				right.VoltageV <= left.VoltageV ||
				!finitePositive(left.CurrentA) || !finitePositive(right.CurrentA) {
				return 0, false
			}
			fraction := (math.Log(voltageV) - math.Log(left.VoltageV)) /
				(math.Log(right.VoltageV) - math.Log(left.VoltageV))
			currentA := math.Exp(math.Log(left.CurrentA) +
				fraction*(math.Log(right.CurrentA)-math.Log(left.CurrentA)))
			return currentA, finitePositive(currentA)
		}
	}
	return 0, false
}

func currentModeBuckLoopMargins(gmS, controllerPoleHz, feedbackRatio, loadOhm, capacitanceF float64) (float64, float64, bool) {
	magnitude := func(frequencyHz float64) float64 {
		omega := 2 * math.Pi * frequencyHz
		plant := 1 / math.Hypot(1/loadOhm, omega*capacitanceF)
		controller := gmS / math.Hypot(1, frequencyHz/controllerPoleHz)
		return controller * feedbackRatio * plant
	}
	low, high := 1.0, float64(buckSwitchingFrequencyHz)/2
	if magnitude(low) < 1 || magnitude(high) > 1 {
		return 0, 0, false
	}
	for iteration := 0; iteration < 80; iteration++ {
		mid := math.Sqrt(low * high)
		if magnitude(mid) >= 1 {
			low = mid
		} else {
			high = mid
		}
	}
	crossover := math.Sqrt(low * high)
	plantPhase := math.Atan(2*math.Pi*crossover*loadOhm*capacitanceF) * 180 / math.Pi
	controllerPhase := math.Atan(crossover/controllerPoleHz) * 180 / math.Pi
	return crossover, 180 - plantPhase - controllerPhase, true
}
