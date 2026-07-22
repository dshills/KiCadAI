package architecturesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

// The methods in this file are reusable capability providers for v2 requests.
// They consume only typed ports and the common constraint contract; none sees
// project identity, corpus identity, or fixture-specific data.

func (provider *CatalogProvider) expandGenericThreshold(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	threshold, tolerance, ok := firstNumericConstraint(request.Constraints, "threshold_voltage", "rising_threshold", "falling_threshold")
	if !ok {
		return nil, fmt.Errorf("threshold detection requires a target threshold")
	}
	hysteresis, hysteresisTolerance, ok := firstNumericConstraint(request.Constraints, "hysteresis_width", "hysteresis")
	if !ok {
		hysteresis = math.Max(0.02*math.Abs(threshold), 0.01)
		hysteresisTolerance = 10
	}
	if constraint, exists := namedConstraint(request.Constraints, "hysteresis"); exists && constraint.Relation == "minimum" {
		hysteresis *= 1.1
	}
	if hysteresisTolerance == 0 {
		hysteresisTolerance = 10
	}
	legacy := cloneProviderRequest(request)
	legacy.Constraints = []Constraint{
		numericConstraint("threshold_voltage", "target", threshold, "V", tolerance),
		numericConstraint("hysteresis_width", "target", hysteresis, "V", hysteresisTolerance),
		stringConstraint("output_polarity", "equal", "active_high"),
		boolConstraint("inactive_at_power_up", "required"),
		numericConstraint("propagation_delay", "maximum", 10, "us", 0),
	}
	legacy.Constraints = append(legacy.Constraints, environmentalConstraints(request.Constraints)...)
	return provider.expandThreshold(ctx, legacy)
}

func (provider *CatalogProvider) expandGenericLoadSwitch(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	current, tolerance, ok := firstNumericConstraint(request.Constraints, "load_current")
	if !ok || current <= 0 {
		return nil, fmt.Errorf("load switch requires a positive load-current bound")
	}
	voltage := maximumPortVoltage(request.Ports)
	legacy := cloneProviderRequest(request)
	for index := range legacy.Ports {
		if legacy.Ports[index].Role == "control" && legacy.Ports[index].Contract.Voltage.Maximum != nil && *legacy.Ports[index].Contract.Voltage.Maximum > 5.1 {
			legacy.Ports[index].Contract.Voltage.Minimum = float64Pointer(0)
			legacy.Ports[index].Contract.Voltage.Maximum = float64Pointer(5.1)
		}
	}
	controlActiveState := "high"
	if constraint, ok := namedConstraint(request.Constraints, "fail_safe_interlock"); ok && constraint.Relation == "required" {
		controlActiveState = "high_disconnect"
	}
	legacy.Constraints = []Constraint{
		stringConstraint("load_characteristic", "equal", "inductive"),
		stringConstraint("control_active_state", "equal", controlActiveState),
		boolConstraint("default_off", "required"),
		boolConstraint("inductive_transient_clamp", "required"),
		boolConstraint("control_overvoltage_clamp", "required"),
		numericConstraint("load_voltage", "minimum", voltage, "V", 0),
		numericConstraint("load_current", "minimum", current, "A", tolerance),
	}
	if startupMaximum, _, startupErr := requiredNumber(request.Constraints, "startup_output_voltage", "maximum", "V"); startupErr == nil && startupMaximum < .5*voltage {
		legacy.Constraints = append(legacy.Constraints, stringConstraint("off_output_state", "equal", "low"))
	}
	legacy.Constraints = append(legacy.Constraints, environmentalConstraints(request.Constraints)...)
	expansions, err := provider.expandLoadSwitch(ctx, legacy)
	if err != nil {
		return nil, err
	}
	for index := range expansions {
		expansions[index].OfferedPorts = remapOfferedCatalogPorts(request, expansions[index].OfferedPorts)
	}
	return expansions, nil
}

func environmentalConstraints(constraints []Constraint) []Constraint {
	preserved := make([]Constraint, 0, 4)
	for _, constraint := range constraints {
		switch constraint.Name {
		case "ambient_temperature_minimum", "ambient_temperature", "case_temperature", "junction_temperature":
			preserved = append(preserved, constraint)
		}
	}
	return preserved
}

func (provider *CatalogProvider) expandGenericRegulator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if constraint, ok := namedConstraint(request.Constraints, "isolation_required"); ok && constraint.Relation == "required" {
		return provider.expandIsolatedRegulator(ctx, request)
	}
	output, tolerance, ok := firstNumericConstraint(request.Constraints, "output_voltage")
	if !ok || output <= 0 {
		return nil, fmt.Errorf("voltage regulation requires a positive output-voltage target")
	}
	inputMinimum, inputMaximum, ok := roleVoltageRange(request.Ports, "input")
	if !ok || inputMaximum <= output {
		return nil, fmt.Errorf("regulator input range does not provide positive headroom")
	}
	current, currentTolerance, ok := firstNumericConstraint(request.Constraints, "continuous_output_current", "output_current")
	if !ok {
		current = requiredRoleCurrentA(request.Ports, "output")
	}
	if current <= 0 {
		return nil, fmt.Errorf("voltage regulation requires an output-current bound")
	}
	legacy := cloneProviderRequest(request)
	legacy.Constraints = mergeProjectedConstraints(legacy.Constraints, []Constraint{
		boolConstraint("adjustable_output", "required"),
		stringConstraint("set_point_programming", "equal", "passive_feedback"),
		boolConstraint("input_decoupling", "required"),
		boolConstraint("output_decoupling", "required"),
		rangeConstraint("input_voltage", inputMinimum, inputMaximum, "V"),
		numericConstraint("output_voltage", "target", output, "V", tolerance),
		numericConstraint("continuous_output_current", "minimum", current, "A", currentTolerance),
	})
	adjustable, adjustableErr := provider.expandRegulator(ctx, legacy)
	fixed, fixedErr := provider.expandFixedRegulators(ctx, request, output, tolerance, inputMinimum, inputMaximum, current)
	if fixedErr != nil {
		return nil, fixedErr
	}
	if adjustableErr == nil {
		return append(adjustable, fixed...), nil
	}
	if len(fixed) != 0 {
		return fixed, nil
	}
	return nil, adjustableErr
}

func (provider *CatalogProvider) expandFixedRegulators(ctx context.Context, request ProviderRequest, output, tolerance, inputMinimum, inputMaximum, current float64) ([]ProviderExpansion, error) {
	var result []ProviderExpansion
	for _, record := range provider.catalog.Records {
		if record.Family != "regulator" || recordHasFunction(record, "ADJ") || !recordHasFunction(record, "VIN") || !recordHasFunction(record, "VOUT") || !recordHasFunction(record, "GND") {
			continue
		}
		fixedOutput, ok := recordValue(record, "output_voltage", "V")
		allowedError := math.Max(1e-9, math.Abs(output)*math.Abs(tolerance)/100)
		if !ok || math.Abs(fixedOutput-output) > allowedError {
			continue
		}
		requiredRatings := []components.RequiredRating{
			{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
			{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
			{Kind: "output_current", Value: numericString(current), Unit: "A"},
		}
		if !recordSupportsRatings(record, requiredRatings) {
			continue
		}
		dropout, dropoutOK := recordRegulatorDropoutV(record)
		if !dropoutOK || inputMinimum-output < dropout {
			continue
		}
		if recordHasFunction(record, "EN") {
			enableMaximum, enableOK := recordRatingMaximum(record, "enable_voltage", "V")
			absoluteMaximum, absoluteOK := recordRatingMaximum(record, "enable_voltage_abs_max", "V")
			if !enableOK || !absoluteOK || inputMaximum > enableMaximum || inputMaximum > absoluteMaximum {
				continue
			}
		}
		part, err := provider.selectComponent(ctx, "regulator", record.MPN, requiredRatings, true)
		if err != nil || part.record.ID != record.ID {
			continue
		}
		outputCapacitance, transientCalculation, err := regulatorOutputCapacitor(request, part.record)
		if err != nil {
			return nil, err
		}
		parts := []catalogPart{part}
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"input_bypass", "capacitor", "decoupling", "1u"}, {"output_bypass", "capacitor", "decoupling", outputCapacitance}})
		if err != nil {
			return nil, err
		}
		bindings := bindRoles(request.Ports, part.selected.InstanceID, map[string]string{"input": "VIN", "output": "VOUT", "reference": "GND"})
		inputEndpoints := []RealizationEndpoint{endpoint(part, "VIN"), passiveEndpoint("input_bypass", "A")}
		if recordHasFunction(part.record, "EN") {
			inputEndpoints = append(inputEndpoints, endpoint(part, "EN"))
		}
		connections := []RealizationConnection{
			semanticNet("regulator_input", "power", inputEndpoints...),
			semanticNet("regulator_output", "power", endpoint(part, "VOUT"), passiveEndpoint("output_bypass", "A")),
			semanticNet("regulator_ground", "reference", endpoint(part, "GND"), passiveEndpoint("input_bypass", "B"), passiveEndpoint("output_bypass", "B")),
		}
		dropoutCalculation, err := regulatorDropoutCalculation(inputMinimum, output, part.record)
		if err != nil {
			return nil, err
		}
		calculations := []CalculationEvidence{dropoutCalculation}
		if transientCalculation != nil {
			calculations = append(calculations, *transientCalculation)
		}
		expansions, err := provider.expansion(request, "fixed_linear_regulator_"+derivedSemanticIdentifier(record.ID), parts, bindings, connections, calculations, 0)
		if err != nil {
			return nil, err
		}
		result = append(result, expansions...)
	}
	return result, nil
}

func (provider *CatalogProvider) expandIsolatedRegulator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	output, tolerance, ok := firstNumericConstraint(request.Constraints, "output_voltage")
	if !ok || output <= 0 {
		return nil, fmt.Errorf("isolated voltage regulation requires a positive output target")
	}
	inputMinimum, inputMaximum, ok := roleVoltageRange(request.Ports, "input")
	if !ok {
		return nil, fmt.Errorf("isolated voltage regulation requires a bounded input range")
	}
	outputCurrent := requiredRoleCurrentA(request.Ports, "output")
	if outputCurrent <= 0 {
		return nil, fmt.Errorf("isolated voltage regulation requires an output-current contract")
	}
	converter, err := provider.selectComponent(ctx, "isolated_converter", "MEE1S0505SC", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
		{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "output_current", Value: numericString(outputCurrent / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "isolation_voltage", Value: "1000", Unit: "V"},
	}, true)
	if err != nil {
		return nil, err
	}
	converter.selected.InstanceID, converter.usage = "isolation_converter", "isolated_power_stage"
	regulator, err := provider.selectComponent(ctx, "regulator", "AP2112K 3.3", []components.RequiredRating{
		{Kind: "input_voltage", Value: "5", Unit: "V"},
		{Kind: "output_current", Value: numericString(outputCurrent), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	if fixedOutput, found := recordValue(regulator.record, "output_voltage", "V"); !found || math.Abs(fixedOutput-output) > output*tolerance/100 {
		return nil, fmt.Errorf("isolated post-regulator output is outside the requested tolerance")
	}
	regulator.selected.InstanceID, regulator.usage = "isolated_post_regulator", "isolated_low_noise_regulator"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{converter, regulator}, []passivePart{
		{"converter_input_bypass", "capacitor", "decoupling", "1u"}, {"converter_output_bypass", "capacitor", "decoupling", "1u"},
		{"isolated_output_bypass", "capacitor", "stability", "1u"},
	})
	if err != nil {
		return nil, err
	}
	converterCurrent, currentOK := recordRatingMaximum(converter.record, "output_current", "A")
	isolation, isolationOK := recordRatingMaximum(converter.record, "isolation_voltage", "V")
	regulatorCurrent, regulatorCurrentOK := recordRatingMaximum(regulator.record, "output_current", "A")
	if !currentOK || !isolationOK || !regulatorCurrentOK {
		return nil, fmt.Errorf("isolated regulator chain lacks normalized current or isolation evidence")
	}
	ratings, ratingIssues := EvaluateRatings("isolated_regulator_margins", []RatingRequirement{
		{Kind: "converter_output_current", Required: outputCurrent, Rated: converterCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: converter.evidence},
		{Kind: "isolation_voltage", Required: 1000, Rated: isolation, DeratingFactor: 1, Unit: "V", Evidence: converter.evidence},
		{Kind: "post_regulator_current", Required: outputCurrent, Rated: regulatorCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: regulator.evidence},
		{Kind: "post_regulator_headroom", Required: output + 0.4, Rated: 4.5, DeratingFactor: 1, Unit: "V", Evidence: converter.evidence},
	})
	if len(ratingIssues) != 0 {
		return nil, fmt.Errorf("isolated regulator rating solution failed: %s", ratingIssues[0].Message)
	}
	bindings := bindRoles(request.Ports, converter.selected.InstanceID, map[string]string{"input": "VIN_PLUS", "output": "VOUT", "reference": "VOUT_MINUS"})
	for index := range bindings {
		switch bindings[index].Role {
		case "output":
			bindings[index].Instance, bindings[index].Function = regulator.selected.InstanceID, "VOUT"
		case "reference":
			bindings[index].Instance, bindings[index].Function = converter.selected.InstanceID, "VOUT_MINUS"
		}
	}
	bindings = append(bindings, RealizationPortBinding{Role: "input", Lane: "return", Instance: converter.selected.InstanceID, Function: "VIN_MINUS"})
	connections := []RealizationConnection{
		semanticNet("isolated_converter_input", "power", endpoint(converter, "VIN_PLUS"), passiveEndpoint("converter_input_bypass", "A")),
		semanticNet("isolated_converter_input_return", "reference", endpoint(converter, "VIN_MINUS"), passiveEndpoint("converter_input_bypass", "B")),
		semanticNet("isolated_raw_power", "power", endpoint(converter, "VOUT_PLUS"), endpoint(regulator, "VIN"), endpoint(regulator, "EN"), passiveEndpoint("converter_output_bypass", "A")),
		semanticNet("isolated_output", "power", endpoint(regulator, "VOUT"), passiveEndpoint("isolated_output_bypass", "A")),
		semanticNet("isolated_reference", "reference", endpoint(converter, "VOUT_MINUS"), endpoint(regulator, "GND"), passiveEndpoint("converter_output_bypass", "B"), passiveEndpoint("isolated_output_bypass", "B")),
	}
	return provider.expansion(request, "isolated_converter_with_post_regulation", parts, bindings, connections, []CalculationEvidence{ratings}, 0)
}

func (provider *CatalogProvider) expandGenericTranslator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	frequency := maximumProtocolFrequency(request.Ports)
	if frequency <= 0 {
		frequency, _, _ = firstNumericConstraint(request.Constraints, "bus_frequency")
	}
	if frequency <= 0 {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "logic translation requires protocol frequency evidence"}
	}
	legacy := cloneProviderRequest(request)
	legacy.Constraints = mergeProjectedConstraints(legacy.Constraints, []Constraint{
		stringConstraint("protocol", "equal", "i2c"),
		stringConstraint("signaling_mode", "equal", "open_drain"),
		stringConstraint("direction", "equal", "bidirectional"),
		boolConstraint("unpowered_backfeed_prevention", "required"),
		numericConstraint("bus_frequency", "minimum", frequency, "Hz", 0),
	})
	return provider.expandTranslator(ctx, legacy)
}

func (provider *CatalogProvider) expandGenericFilter(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if constraint, ok := namedConstraint(request.Constraints, "response"); !ok || !constraintStringEquals(constraint, "low_pass") {
		return nil, fmt.Errorf("generic filter provider requires a low-pass response")
	}
	frequency, tolerance, ok := firstNumericConstraint(request.Constraints, "cutoff_frequency")
	if !ok || frequency <= 0 {
		return nil, fmt.Errorf("generic filter provider requires a positive cutoff-frequency target")
	}
	if order, _, hasOrder := firstNumericConstraint(request.Constraints, "order"); hasOrder {
		constraint, _ := namedConstraint(request.Constraints, "order")
		compatible := constraint.Relation == "minimum" && order <= 2 || constraint.Relation == "maximum" && order >= 2 ||
			(constraint.Relation == "equal" || constraint.Relation == "target") && order == 2
		if !compatible {
			return nil, fmt.Errorf("generic filter provider supports a second-order low-pass realization")
		}
	}
	supplySpan := maximumSupplySpan(request.Ports)
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
	opampQuery := "single"
	if !hasNegativePower {
		opampQuery = "rail_to_rail"
	}
	opamp, err := provider.selectComponent(ctx, "opamp", opampQuery, []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplySpan), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	calculation, issues := SolveSallenKeyLowPass(SallenKeyLowPassRequest{
		ID: "filter_stage", TargetFrequencyHz: frequency, FrequencyTolerancePercent: tolerance,
		TargetQ: 0.70710678, QTolerancePercent: 5, ResistanceOhm: 10000,
		ResistanceTolerancePercent: 0.1, CapacitanceTolerancePercent: 1, CapacitanceSeries: SeriesE96,
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("filter value solution failed: %s", issues[0].Message)
	}
	c1, c1OK := calculationSelectedValue(calculation, "capacitance_1")
	c2, c2OK := calculationSelectedValue(calculation, "capacitance_2")
	if !c1OK || !c2OK {
		return nil, fmt.Errorf("filter solution omitted capacitance values")
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{opamp}, []passivePart{
		{"filter_r1", "resistor", "filter", "10k"}, {"filter_r2", "resistor", "filter", "10k"},
		{"filter_c1", "capacitor", "filter", engineeringValue(c1, "F")}, {"filter_c2", "capacitor", "filter", engineeringValue(c2, "F")},
		{"supply_bypass", "capacitor", "decoupling_capacitor", "100n"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, opamp.selected.InstanceID, map[string]string{
		"input": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "positive_power": "V_PLUS",
		"negative_power": "V_MINUS", "reference": "V_MINUS",
	})
	for index := range bindings {
		if bindings[index].Role == "input" {
			bindings[index].Instance, bindings[index].Function = "filter_r1", "A"
		} else if bindings[index].Role == "reference" && hasNegativePower {
			bindings[index].Instance, bindings[index].Function = "filter_c2", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("filter_node_1", "analog_signal", passiveEndpoint("filter_r1", "B"), passiveEndpoint("filter_r2", "A"), passiveEndpoint("filter_c1", "A")),
		semanticNet("filter_node_2", "analog_signal", passiveEndpoint("filter_r2", "B"), passiveEndpoint("filter_c2", "A"), endpoint(opamp, "IN_PLUS")),
		semanticNet("filter_output", "analog_signal", endpoint(opamp, "OUT"), endpoint(opamp, "IN_MINUS"), passiveEndpoint("filter_c1", "B")),
		semanticNet("filter_positive_power", "power", endpoint(opamp, "V_PLUS"), passiveEndpoint("supply_bypass", "A")),
	}
	if hasNegativePower {
		connections = append(connections, semanticNet("filter_negative_power", "power", endpoint(opamp, "V_MINUS"), passiveEndpoint("supply_bypass", "B")))
	} else {
		connections = append(connections, semanticNet("filter_negative_reference", "reference", endpoint(opamp, "V_MINUS"), passiveEndpoint("filter_c2", "B"), passiveEndpoint("supply_bypass", "B")))
	}
	orderEvidence, err := ObservedCalculation("filter_order", NamedQuantity{Name: "filter_order", Value: 2})
	if err != nil {
		return nil, err
	}
	return provider.expansion(request, "catalog_sallen_key_low_pass", parts, bindings, connections, []CalculationEvidence{calculation, orderEvidence}, 0)
}

func (provider *CatalogProvider) expandFaultIndication(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	supply := maximumPortVoltage(request.Ports)
	transistor, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply), Unit: "V"},
		{Kind: "collector_current", Value: "0.01", Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	transistor.selected.InstanceID = "indicator_driver"
	transistor.usage = "indicator_driver"
	led, err := provider.selectComponent(ctx, "led", "indicator", nil, true)
	if err != nil {
		return nil, err
	}
	led.selected.InstanceID = "indicator_led"
	led.usage = "fault_indicator"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{transistor, led}, []passivePart{
		{"base_resistor", "resistor", "drive_limit", "10k"},
		{"led_resistor", "resistor", "current_limit", "1k"},
		{"output_pulldown", "resistor", "default_state", "100k"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, transistor.selected.InstanceID, map[string]string{"state": "BASE", "input": "BASE", "output": "EMITTER", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		if bindings[index].Role == "state" || bindings[index].Role == "input" {
			bindings[index].Instance, bindings[index].Function = "base_resistor", "A"
		} else if bindings[index].Role == "reference" {
			bindings[index].Instance, bindings[index].Function = "indicator_led", "K"
		}
	}
	connections := []RealizationConnection{
		semanticNet("indicator_drive", "control", passiveEndpoint("base_resistor", "B"), endpoint(transistor, "BASE")),
		semanticNet("indicator_output", "digital_signal", endpoint(transistor, "EMITTER"), passiveEndpoint("led_resistor", "A"), passiveEndpoint("output_pulldown", "A")),
		semanticNet("indicator_led_drive", "indicator", passiveEndpoint("led_resistor", "B"), endpoint(led, "A")),
		semanticNet("indicator_reference", "reference", endpoint(led, "K"), passiveEndpoint("output_pulldown", "B")),
	}
	return provider.expansion(request, "buffered_fault_indicator", parts, bindings, retainSemanticNets(connections), nil, 0)
}

func (provider *CatalogProvider) expandSafetyInterlock(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	supply := maximumPortVoltage(request.Ports)
	first, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{{Kind: "collector_emitter_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	first.selected.InstanceID, first.usage = "fault_a_clamp", "fail_safe_interlock"
	second := first
	second.selected.InstanceID, second.usage = "fault_b_clamp", "fail_safe_interlock"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{first, second}, []passivePart{
		{"fault_a_resistor", "resistor", "drive_limit", "10k"}, {"fault_b_resistor", "resistor", "drive_limit", "10k"},
		{"permit_pullup", "resistor", "default_block", "10k"}, {"fault_a_pulldown", "resistor", "default_fault", "100k"}, {"fault_b_pulldown", "resistor", "default_fault", "100k"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, first.selected.InstanceID, map[string]string{"fault_a": "BASE", "fault_b": "BASE", "permit": "COLLECTOR", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		switch bindings[index].Role {
		case "fault_a":
			bindings[index].Instance, bindings[index].Function = "fault_a_resistor", "A"
		case "fault_b":
			bindings[index].Instance, bindings[index].Function = "fault_b_resistor", "A"
		case "power":
			bindings[index].Instance, bindings[index].Function = "permit_pullup", "A"
		case "reference":
			bindings[index].Instance, bindings[index].Function = first.selected.InstanceID, "EMITTER"
		}
	}
	connections := []RealizationConnection{
		semanticNet("fault_a_drive", "control", passiveEndpoint("fault_a_resistor", "B"), endpoint(first, "BASE"), passiveEndpoint("fault_a_pulldown", "A")),
		semanticNet("fault_b_drive", "control", passiveEndpoint("fault_b_resistor", "B"), endpoint(second, "BASE"), passiveEndpoint("fault_b_pulldown", "A")),
		semanticNet("permit_output", "control", endpoint(first, "COLLECTOR"), endpoint(second, "COLLECTOR"), passiveEndpoint("permit_pullup", "B")),
		semanticNet("interlock_reference", "reference", endpoint(first, "EMITTER"), endpoint(second, "EMITTER"), passiveEndpoint("fault_a_pulldown", "B"), passiveEndpoint("fault_b_pulldown", "B")),
	}
	return provider.expansion(request, "dual_fault_fail_safe_interlock", parts, bindings, connections, nil, 0)
}

func cloneProviderRequest(request ProviderRequest) ProviderRequest {
	encoded, _ := json.Marshal(request)
	var cloned ProviderRequest
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func firstNumericConstraint(constraints []Constraint, names ...string) (float64, float64, bool) {
	for _, name := range names {
		constraint, ok := namedConstraint(constraints, name)
		if !ok {
			continue
		}
		var value float64
		if json.Unmarshal(constraint.Value, &value) != nil || !finiteNumbers(value) {
			continue
		}
		tolerance := 0.0
		if constraint.TolerancePercent != nil {
			tolerance = *constraint.TolerancePercent
		}
		return value, tolerance, true
	}
	return 0, 0, false
}

func numericConstraintLowerBound(constraints []Constraint, name string) (float64, bool) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok {
		return 0, false
	}
	var value float64
	if json.Unmarshal(constraint.Value, &value) != nil || !finiteNumbers(value) {
		return 0, false
	}
	switch constraint.Relation {
	case "minimum":
		return value, value > 0
	case "target":
		tolerance := 0.0
		if constraint.TolerancePercent != nil {
			tolerance = *constraint.TolerancePercent
		}
		minimum := value * (1 - tolerance/100)
		return minimum, minimum > 0
	default:
		return 0, false
	}
}

func numericConstraintBounds(constraints []Constraint, name string) (float64, float64, bool) {
	value, tolerance, ok := firstNumericConstraint(constraints, name)
	if !ok || value <= 0 || tolerance < 0 || tolerance >= 100 {
		return 0, 0, false
	}
	constraint, _ := namedConstraint(constraints, name)
	switch constraint.Relation {
	case "target":
		minimum := value * (1 - tolerance/100)
		maximum := value * (1 + tolerance/100)
		return minimum, maximum, minimum > 0 && maximum >= minimum
	case "minimum":
		return value, value, true
	case "maximum":
		return value, value, true
	default:
		return 0, 0, false
	}
}

func preferredRepairValues(value float64) []float64 {
	result := []float64{value}
	for _, scale := range []float64{0.67, 0.82, 0.91, 0.95, 0.97, 0.98, 0.99, 1.01, 1.02, 1.03, 1.05, 1.1, 1.22, 1.3, 1.5, 1.82} {
		candidates, issues := PreferredValueCandidates(value*scale, SeriesE96, value*.5, value*2, 1)
		if len(issues) == 0 && len(candidates) != 0 {
			result = append(result, candidates[0])
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func numericConstraint(name, relation string, value float64, unit string, tolerance float64) Constraint {
	encoded, _ := json.Marshal(value)
	constraint := Constraint{Name: name, Relation: relation, Value: encoded, Unit: unit}
	if relation == "target" {
		constraint.TolerancePercent = float64Pointer(tolerance)
	}
	return constraint
}

func rangeConstraint(name string, minimum, maximum float64, unit string) Constraint {
	encoded, _ := json.Marshal([]float64{minimum, maximum})
	return Constraint{Name: name, Relation: "range", Value: encoded, Unit: unit}
}

func stringConstraint(name, relation, value string) Constraint {
	encoded, _ := json.Marshal(value)
	return Constraint{Name: name, Relation: relation, Value: encoded}
}

func boolConstraint(name, relation string) Constraint {
	return Constraint{Name: name, Relation: relation, Value: json.RawMessage(`true`)}
}

func constraintStringEquals(constraint Constraint, want string) bool {
	var value string
	return json.Unmarshal(constraint.Value, &value) == nil && canonicalIdentifier(value) == canonicalIdentifier(want)
}

func roleVoltageRange(ports []RoleContract, role string) (float64, float64, bool) {
	for _, port := range ports {
		if port.Role == role && port.Contract.Voltage.Minimum != nil && port.Contract.Voltage.Maximum != nil {
			return *port.Contract.Voltage.Minimum, *port.Contract.Voltage.Maximum, true
		}
	}
	return 0, 0, false
}

func thresholdRequiresStableReference(supplyMinimum, supplyMaximum, thresholdTolerancePercent float64) bool {
	if supplyMinimum <= 0 || supplyMaximum < supplyMinimum || thresholdTolerancePercent <= 0 {
		return false
	}
	center := .5 * (supplyMinimum + supplyMaximum)
	if center == 0 {
		return false
	}
	supplyTolerancePercent := 100 * .5 * (supplyMaximum - supplyMinimum) / center
	// Preserve at least half of the public threshold budget for reference,
	// feedback, active-device, and upstream-transfer uncertainty.
	return supplyTolerancePercent >= .5*thresholdTolerancePercent
}

func requiredRoleCurrentA(ports []RoleContract, role string) float64 {
	for _, port := range ports {
		if port.Role == role && port.Contract.RequiredCurrentCapacityA != nil {
			return *port.Contract.RequiredCurrentCapacityA
		}
	}
	return 0
}

func maximumRoleCurrentDemandA(ports []RoleContract, role string) float64 {
	for _, port := range ports {
		if port.Role == role && port.Contract.MaximumCurrentDemandA != nil {
			return *port.Contract.MaximumCurrentDemandA
		}
	}
	return 0
}

func maximumProtocolFrequency(ports []RoleContract) float64 {
	maximum := 0.0
	for _, port := range ports {
		if port.Contract.Protocol != nil {
			maximum = math.Max(maximum, port.Contract.Protocol.MaxFrequencyHz)
		}
		if port.Contract.FrequencyMaxHz != nil {
			maximum = math.Max(maximum, *port.Contract.FrequencyMaxHz)
		}
	}
	return maximum
}

func maximumSupplySpan(ports []RoleContract) float64 {
	minimum, maximum := 0.0, 0.0
	for _, port := range ports {
		if port.Contract.Kind != "power" {
			continue
		}
		if port.Contract.Voltage.Minimum != nil {
			minimum = math.Min(minimum, *port.Contract.Voltage.Minimum)
		}
		if port.Contract.Voltage.Maximum != nil {
			maximum = math.Max(maximum, *port.Contract.Voltage.Maximum)
		}
	}
	return maximum - minimum
}

func (provider *CatalogProvider) expandTransientProtection(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	for _, port := range request.Ports {
		if (port.Role == "input" || port.Role == "output") && port.Contract.Kind == "digital_bus" {
			return provider.expandBusTransientProtection(ctx, request)
		}
	}
	if _, requested := namedConstraint(request.Constraints, "reverse_current_blocking"); requested {
		if err := requireBool(request.Constraints, "reverse_current_blocking", "required", true); err != nil {
			return nil, err
		}
		return provider.expandReverseBlockingTransientProtection(ctx, request)
	}
	protector, err := provider.selectComponent(ctx, "protection", "ESD TVS", nil, true)
	if err != nil {
		return nil, err
	}
	protector.selected.InstanceID, protector.usage = "transient_clamp", "transient_protection"
	seriesValue := "22"
	for _, port := range request.Ports {
		if port.Role == "input" && port.Contract.Kind == "power" {
			seriesValue = "0.1"
		}
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{protector}, []passivePart{{"series_element", "resistor", "transient_series_impedance", seriesValue}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, "series_element", map[string]string{"input": "A", "output": "B", "reference": "A"})
	for index := range bindings {
		if bindings[index].Role == "reference" {
			bindings[index].Instance, bindings[index].Function = protector.selected.InstanceID, "ANODE"
		}
	}
	connections := []RealizationConnection{
		semanticNet("protected_node", "protected_signal", passiveEndpoint("series_element", "B"), endpoint(protector, "CATHODE")),
	}
	return provider.expansion(request, "series_shunt_transient_protection", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandReverseBlockingTransientProtection(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	inputMaximumV := roleVoltageMaximum(request.Ports, "input")
	currentA := max(requiredRoleCurrentA(request.Ports, "input"), maximumRoleCurrentDemandA(request.Ports, "output"))
	switchPart, err := provider.selectComponent(ctx, "protection", "reverse current blocking", []components.RequiredRating{
		{Kind: "supply_voltage", Value: numericString(inputMaximumV), Unit: "V"},
		{Kind: "continuous_current", Value: numericString(currentA), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	switchPart.selected.InstanceID, switchPart.usage = "reverse_blocking_switch", "reverse_current_blocking"
	clamp, err := provider.selectComponent(ctx, "protection", "ESD TVS", nil, true)
	if err != nil {
		return nil, err
	}
	clamp.selected.InstanceID, clamp.usage = "transient_clamp", "transient_protection"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{switchPart, clamp}, []passivePart{
		{"input_bypass", "capacitor", "decoupling", "1u"},
		{"output_bypass", "capacitor", "decoupling", "1u"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, switchPart.selected.InstanceID, map[string]string{"input": "VIN", "output": "VOUT", "reference": "GND"})
	connections := []RealizationConnection{
		semanticNet("protected_input", "power", endpoint(switchPart, "VIN"), endpoint(switchPart, "ON"), passiveEndpoint("input_bypass", "A")),
		semanticNet("protected_output", "power", endpoint(switchPart, "VOUT"), endpoint(clamp, "CATHODE"), passiveEndpoint("output_bypass", "A")),
		semanticNet("protection_ground", "reference", endpoint(switchPart, "GND"), endpoint(clamp, "ANODE"), passiveEndpoint("input_bypass", "B"), passiveEndpoint("output_bypass", "B")),
	}
	return provider.expansion(request, "reverse_blocking_load_switch_with_shunt_clamp", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandBusTransientProtection(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	protector, err := provider.selectComponent(ctx, "protection", "ESD TVS", nil, true)
	if err != nil {
		return nil, err
	}
	protector.selected.InstanceID, protector.usage = "sda_transient_clamp", "bus_transient_protection"
	sclProtector := protector
	sclProtector.selected.InstanceID = "scl_transient_clamp"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{protector, sclProtector}, []passivePart{
		{"sda_series", "resistor", "transient_series_impedance", "22"}, {"scl_series", "resistor", "transient_series_impedance", "22"},
	})
	if err != nil {
		return nil, err
	}
	var bindings []RealizationPortBinding
	for _, port := range request.Ports {
		switch port.Role {
		case "input":
			bindings = append(bindings,
				RealizationPortBinding{Role: port.Role, Lane: "sda", Instance: "sda_series", Function: "A"},
				RealizationPortBinding{Role: port.Role, Lane: "scl", Instance: "scl_series", Function: "A"},
			)
		case "output":
			bindings = append(bindings,
				RealizationPortBinding{Role: port.Role, Lane: "sda", Instance: "sda_series", Function: "B"},
				RealizationPortBinding{Role: port.Role, Lane: "scl", Instance: "scl_series", Function: "B"},
			)
		case "reference":
			bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: protector.selected.InstanceID, Function: "ANODE"})
		default:
			return nil, fmt.Errorf("bus transient protection does not support role %s", port.Role)
		}
	}
	connections := []RealizationConnection{
		semanticNet("protected_sda", "protected_signal", passiveEndpoint("sda_series", "B"), endpoint(protector, "CATHODE")),
		semanticNet("protected_scl", "protected_signal", passiveEndpoint("scl_series", "B"), endpoint(sclProtector, "CATHODE")),
		semanticNet("bus_protection_reference", "reference", endpoint(protector, "ANODE"), endpoint(sclProtector, "ANODE")),
	}
	return provider.expansion(request, "dual_line_series_shunt_transient_protection", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandOutputProtection(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	requiredWorkingVoltage := 0.0
	for _, port := range request.Ports {
		if port.Contract.Voltage.Minimum != nil {
			requiredWorkingVoltage = math.Max(requiredWorkingVoltage, math.Abs(*port.Contract.Voltage.Minimum))
		}
		if port.Contract.Voltage.Maximum != nil {
			requiredWorkingVoltage = math.Max(requiredWorkingVoltage, math.Abs(*port.Contract.Voltage.Maximum))
		}
	}
	if power, _, hasPower := firstNumericConstraint(request.Constraints, "continuous_output_power"); hasPower && power > 0 {
		if load, _, hasLoad := firstNumericConstraint(request.Constraints, "load_impedance"); hasLoad && load > 0 {
			requiredWorkingVoltage = math.Max(requiredWorkingVoltage, math.Sqrt(2*power*load))
		}
	}
	protectorQuery := "ESD TVS"
	var protectorRatings []components.RequiredRating
	if requiredWorkingVoltage > 0 {
		protectorQuery = "tvs"
		protectorRatings = []components.RequiredRating{{Kind: "working_voltage", Value: numericString(requiredWorkingVoltage), Unit: "V"}}
	}
	protector, err := provider.selectComponent(ctx, "protection", protectorQuery, protectorRatings, true)
	if err != nil {
		return nil, err
	}
	protector.selected.InstanceID, protector.usage = "output_clamp", "output_transient_clamp"
	limit, _, hasLimit := firstNumericConstraint(request.Constraints, "overcurrent_limit")
	if !hasLimit {
		power, _, hasPower := firstNumericConstraint(request.Constraints, "continuous_output_power")
		load, _, hasLoad := firstNumericConstraint(request.Constraints, "load_impedance")
		if hasPower && hasLoad && power > 0 && load > 0 {
			limit, hasLimit = math.Sqrt(2*power/load), true
		}
	}
	if !hasLimit || limit <= 0 {
		limit = .5
	}
	fuse, err := provider.selectComponent(ctx, "fuse", "", []components.RequiredRating{{Kind: "current", Value: numericString(limit), Unit: "A"}}, true)
	if err != nil {
		return nil, err
	}
	ratedCurrent := recordRatingOrZero(fuse.record, "current", "A")
	if ratedCurrent <= 0 {
		return nil, fmt.Errorf("selected output fuse lacks a bounded catalog current rating")
	}
	value := engineeringValue(ratedCurrent, "A")
	fuse.selected.InstanceID, fuse.usage, fuse.value = "output_fuse", "overcurrent_limit", value
	if startupIsolation, requiresStartupIsolation := namedConstraint(request.Constraints, "startup_isolation"); requiresStartupIsolation && startupIsolation.Relation == "required" && hasRoleContract(request.Ports, "power") {
		relayRatings := []components.RequiredRating{
			{Kind: "contact_current_dc", Value: numericString(limit), Unit: "A"},
			{Kind: "contact_voltage_dc", Value: numericString(maximumPortVoltage(request.Ports)), Unit: "V"},
		}
		// Prefer silent solid-state isolation when it satisfies the electrical
		// envelope, then broaden deterministically to any catalog-backed relay.
		// The shared rating and supply checks remain authoritative for both.
		relay, relayErr := provider.selectComponent(ctx, "relay", "solid_state", relayRatings, true)
		if relayErr != nil {
			relay, relayErr = provider.selectComponent(ctx, "relay", "", relayRatings, true)
		}
		if relayErr != nil {
			return nil, fmt.Errorf("startup-isolating output relay selection failed: %w", relayErr)
		}
		coilVoltage, coilVoltageOK := recordValue(relay.record, "coil_voltage", "V")
		coilResistance, coilResistanceOK := recordValue(relay.record, "coil_resistance", "ohm")
		powerMinimum, powerMaximum, powerOK := roleVoltageRange(request.Ports, "power")
		if !coilVoltageOK || !coilResistanceOK || !powerOK || coilVoltage <= 0 || coilResistance <= 0 || powerMinimum < coilVoltage*.75 {
			return nil, fmt.Errorf("startup-isolating output relay lacks a compatible bounded coil supply")
		}
		coilCurrent := coilVoltage / coilResistance
		designPower := (powerMinimum + powerMaximum) / 2
		if controlCurrent, controlCurrentOK := recordValue(relay.record, "control_current", "A"); controlCurrentOK && controlCurrent > 0 {
			coilCurrent = controlCurrent
			coilResistance = coilVoltage / coilCurrent
			designPower = powerMinimum
		}
		// The catalog control current is an operate threshold, not a nominal
		// design point. Preserve deterministic margin for supply tolerance, clamp
		// leakage, and coil variation at the minimum available rail.
		const relayOperateCurrentMargin = 1.2
		designCoilCurrent := relayOperateCurrentMargin * coilCurrent
		coilSeriesResistance := math.Max(1, (designPower-designCoilCurrent*coilResistance)/designCoilCurrent)
		relay.selected.InstanceID, relay.usage = "output_relay", "delayed_startup_isolation"
		flyback, flybackErr := provider.selectComponent(ctx, "diode", "switching", nil, true)
		if flybackErr != nil {
			return nil, flybackErr
		}
		flyback.selected.InstanceID, flyback.usage = "relay_flyback", "coil_flyback_clamp"
		parts, err := provider.appendPassiveParts(ctx, []catalogPart{fuse, protector, relay, flyback}, []passivePart{
			{"relay_coil_series", "resistor", "relay_coil_drop", engineeringValue(coilSeriesResistance, "Ohm")},
			{"output_precharge", "resistor", "startup_precharge", "100k"},
			{"output_pulldown", "resistor", "startup_inactive", "100k"},
		})
		if err != nil {
			return nil, err
		}
		bindings := bindRoles(request.Ports, relay.selected.InstanceID, map[string]string{"input": "CONTACT_IN", "output": "CONTACT_OUT", "reference": "COIL_B", "power": "COIL_A"})
		for index := range bindings {
			switch bindings[index].Role {
			case "output":
				bindings[index].Instance, bindings[index].Function = fuse.selected.InstanceID, "B"
			case "reference":
				bindings[index].Instance, bindings[index].Function = protector.selected.InstanceID, "ANODE"
			case "power":
				bindings[index].Instance, bindings[index].Function = "relay_coil_series", "A"
			}
		}
		connections := []RealizationConnection{
			semanticNet("relay_contact_output", "protected_output", endpoint(relay, "CONTACT_OUT"), endpoint(fuse, "A")),
			semanticNet("protected_output_node", "protected_output", endpoint(fuse, "B"), endpoint(protector, "CATHODE"), passiveEndpoint("output_pulldown", "A")),
			semanticNet("relay_coil_supply", "power", passiveEndpoint("relay_coil_series", "B"), endpoint(relay, "COIL_A"), endpoint(flyback, "K")),
			semanticNet("output_protection_input_precharge", "protected_output", endpoint(relay, "CONTACT_IN"), passiveEndpoint("output_precharge", "A")),
			semanticNet("output_protection_reference", "reference", endpoint(protector, "ANODE"), passiveEndpoint("output_pulldown", "B"), passiveEndpoint("output_precharge", "B"), endpoint(relay, "COIL_B"), endpoint(flyback, "A")),
		}
		return provider.expansion(request, "delayed_relay_output_fault_protection", parts, bindings, connections, nil, 0)
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{fuse, protector}, []passivePart{{"output_pulldown", "resistor", "startup_inactive", "100k"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, fuse.selected.InstanceID, map[string]string{"input": "A", "output": "B", "reference": "A"})
	for index := range bindings {
		if bindings[index].Role == "reference" {
			bindings[index].Instance, bindings[index].Function = protector.selected.InstanceID, "ANODE"
		}
	}
	connections := []RealizationConnection{
		semanticNet("protected_output_node", "protected_output", endpoint(fuse, "B"), endpoint(protector, "CATHODE"), passiveEndpoint("output_pulldown", "A")),
		semanticNet("output_protection_reference", "reference", endpoint(protector, "ANODE"), passiveEndpoint("output_pulldown", "B")),
	}
	if constraint, ok := namedConstraint(request.Constraints, "dc_fault_disconnect"); ok && constraint.Relation == "required" {
		dcBlockValue := "220u"
		if load, _, hasLoad := firstNumericConstraint(request.Constraints, "load_impedance"); hasLoad && load > 0 {
			const maximumProtectionHighPassCornerHz = 20.0
			minimumCapacitance := 1 / (2 * math.Pi * maximumProtectionHighPassCornerHz * load)
			candidates, candidateIssues := PreferredValueCandidates(minimumCapacitance, SeriesE12, minimumCapacitance, minimumCapacitance*10, 1)
			if len(candidateIssues) != 0 || len(candidates) == 0 {
				return nil, fmt.Errorf("output-protection DC-block solution failed")
			}
			dcBlockValue = engineeringValue(candidates[0], "F")
		}
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"dc_block", "capacitor", "dc_fault_disconnect", dcBlockValue}})
		if err != nil {
			return nil, err
		}
		for index := range bindings {
			if bindings[index].Role == "input" {
				bindings[index].Instance, bindings[index].Function = "dc_block", "A"
			}
		}
		connections = append(connections, semanticNet("dc_block_to_fuse", "protected_output", passiveEndpoint("dc_block", "B"), endpoint(fuse, "A")))
	}
	return provider.expansion(request, "passive_output_fault_protection", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandSignalAmplification(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	gain, tolerance, ok := firstNumericConstraint(request.Constraints, "voltage_gain")
	if !ok || gain < 1 {
		return nil, fmt.Errorf("signal amplification requires a gain target of at least one")
	}
	if tolerance == 0 {
		tolerance = 2
	}
	supplySpan := maximumSupplySpan(request.Ports)
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
	opampQuery := "single"
	if !hasNegativePower {
		opampQuery = "rail_to_rail"
	}
	opamp, err := provider.selectComponent(ctx, "opamp", opampQuery, []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplySpan), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	opamp.selected.InstanceID, opamp.usage = "gain_amplifier", canonicalIdentifier(request.Capability)
	calculation, lower, issues := solveAmplifierGain(gain, tolerance)
	if len(issues) != 0 {
		return nil, fmt.Errorf("amplifier gain solution failed: %s", issues[0].Message)
	}
	upper, ok := calculationSelectedValue(calculation, "upper_resistance")
	if !ok {
		return nil, fmt.Errorf("amplifier gain solution omitted feedback resistance")
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{opamp}, []passivePart{
		{"feedback_upper", "resistor", "gain_feedback", engineeringValue(upper, "Ohm")},
		{"feedback_lower", "resistor", "gain_feedback", engineeringValue(lower, "Ohm")},
		{"supply_bypass", "capacitor", "decoupling_capacitor", "100n"},
	})
	if err != nil {
		return nil, err
	}
	outputInstance, outputFunction := opamp.selected.InstanceID, "OUT"
	outputCurrent, _, hasOutputCurrent := firstNumericConstraint(request.Constraints, "output_current")
	if hasOutputCurrent && outputCurrent > 0.02 {
		npn, selectErr := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{{Kind: "collector_current", Value: numericString(outputCurrent), Unit: "A"}}, true)
		if selectErr != nil {
			return nil, selectErr
		}
		pnp, selectErr := provider.selectComponent(ctx, "bjt", "PNP", []components.RequiredRating{{Kind: "collector_current", Value: numericString(outputCurrent), Unit: "A"}}, true)
		if selectErr != nil {
			return nil, selectErr
		}
		npn.selected.InstanceID, npn.usage = "output_npn", "complementary_buffer"
		pnp.selected.InstanceID, pnp.usage = "output_pnp", "complementary_buffer"
		parts = append(parts, npn, pnp)
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"npn_base_stop", "resistor", "base_stop", "47"}, {"pnp_base_stop", "resistor", "base_stop", "47"}})
		if err != nil {
			return nil, err
		}
		outputInstance, outputFunction = npn.selected.InstanceID, "EMITTER"
	}
	bindings := bindRoles(request.Ports, opamp.selected.InstanceID, map[string]string{
		"input": "IN_PLUS", "output": outputFunction, "power": "V_PLUS", "positive_power": "V_PLUS",
		"negative_power": "V_MINUS", "reference": "V_MINUS",
	})
	for index := range bindings {
		if bindings[index].Role == "output" {
			bindings[index].Instance = outputInstance
		} else if bindings[index].Role == "reference" && hasNegativePower {
			bindings[index].Instance, bindings[index].Function = "feedback_lower", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("amplifier_feedback", "analog_signal", endpoint(opamp, "IN_MINUS"), passiveEndpoint("feedback_upper", "B"), passiveEndpoint("feedback_lower", "A")),
		semanticNet("amplifier_positive_power", "power", endpoint(opamp, "V_PLUS"), passiveEndpoint("supply_bypass", "A")),
	}
	if hasNegativePower {
		connections = append(connections, semanticNet("amplifier_negative_power", "power", endpoint(opamp, "V_MINUS"), passiveEndpoint("supply_bypass", "B")))
	} else {
		connections = append(connections, semanticNet("amplifier_reference", "reference", endpoint(opamp, "V_MINUS"), passiveEndpoint("feedback_lower", "B"), passiveEndpoint("supply_bypass", "B")))
	}
	if outputInstance != opamp.selected.InstanceID {
		for index := range connections {
			switch connections[index].ID {
			case "amplifier_positive_power":
				connections[index].Endpoints = append(connections[index].Endpoints, RealizationEndpoint{Instance: "output_npn", Function: "COLLECTOR"})
			case "amplifier_reference":
				connections[index].Endpoints = append(connections[index].Endpoints, RealizationEndpoint{Instance: "output_pnp", Function: "COLLECTOR"})
			}
		}
		connections = append(connections,
			semanticNet("buffer_base_drive", "analog_signal", endpoint(opamp, "OUT"), passiveEndpoint("npn_base_stop", "A"), passiveEndpoint("pnp_base_stop", "A")),
			semanticNet("buffer_npn_base", "analog_signal", passiveEndpoint("npn_base_stop", "B"), RealizationEndpoint{Instance: "output_npn", Function: "BASE"}),
			semanticNet("buffer_pnp_base", "analog_signal", passiveEndpoint("pnp_base_stop", "B"), RealizationEndpoint{Instance: "output_pnp", Function: "BASE"}),
			semanticNet("buffer_output", "analog_signal", RealizationEndpoint{Instance: "output_npn", Function: "EMITTER"}, RealizationEndpoint{Instance: "output_pnp", Function: "EMITTER"}, passiveEndpoint("feedback_upper", "A")),
		)
	} else {
		connections = append(connections, semanticNet("amplifier_output_feedback", "analog_signal", endpoint(opamp, "OUT"), passiveEndpoint("feedback_upper", "A")))
	}
	return provider.expansion(request, "catalog_feedback_amplifier", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func solveAmplifierGain(gain, tolerance float64) (CalculationEvidence, float64, []reports.Issue) {
	lowerCandidates, issues := PreferredValueCandidates(10_000, SeriesE96, 1_000, 100_000, DefaultMaxValueCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, 0, issues
	}
	var lastIssues []reports.Issue
	for _, lower := range lowerCandidates {
		calculation, solveIssues := SolveDivider(DividerRequest{
			ID: "amplifier_gain", Mode: DividerFeedback, SourceVoltageV: 1, TargetVoltageV: gain,
			TargetTolerancePercent: tolerance, LowerResistanceOhm: lower, LowerTolerancePercent: 0.1,
			UpperTolerancePercent: 0.1, UpperSeries: SeriesE96,
		})
		if len(solveIssues) == 0 {
			return calculation, lower, nil
		}
		lastIssues = solveIssues
	}
	return CalculationEvidence{}, 0, lastIssues
}

func lowerEdgeAmplifierGainTarget(gain, requirementTolerancePercent, feedbackResistorTolerancePercent float64) float64 {
	if gain <= 1 || requirementTolerancePercent <= 0 || requirementTolerancePercent >= 100 || feedbackResistorTolerancePercent < 0 || feedbackResistorTolerancePercent >= 100 {
		return gain
	}
	minimumGain := gain * (1 - requirementTolerancePercent/100)
	if minimumGain <= 1 {
		return gain
	}
	tolerance := feedbackResistorTolerancePercent / 100
	// For a non-inverting stage, the feedback contribution is Rupper/Rlower.
	// Center it so the worst opposing resistor tolerances land on the public
	// lower gain edge. This avoids overdriving a bounded output-power harness
	// that is intentionally referenced to that same lower edge.
	return 1 + (minimumGain-1)*(1+tolerance)/(1-tolerance)
}

func failSafeMutePulldownResistance(mutedLimitV, normalOutputPeakV, contactOffResistanceOhm float64) (float64, bool) {
	if mutedLimitV <= 0 || normalOutputPeakV <= mutedLimitV || contactOffResistanceOhm <= 0 || !finiteNumbers(mutedLimitV, normalOutputPeakV, contactOffResistanceOhm) {
		return 0, false
	}
	attenuation := .5 * mutedLimitV / normalOutputPeakV
	maximumPulldown := contactOffResistanceOhm * attenuation / (1 - attenuation)
	candidates, issues := PreferredValueCandidates(maximumPulldown, SeriesE24, maximumPulldown*.1, maximumPulldown, 1)
	if len(issues) != 0 || len(candidates) == 0 {
		return 0, false
	}
	return candidates[0], true
}

func hasRoleContract(ports []RoleContract, role string) bool {
	for _, port := range ports {
		if port.Role == role {
			return true
		}
	}
	return false
}

func (provider *CatalogProvider) expandCurrentSensing(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	fullScale, tolerance, ok := firstNumericConstraint(request.Constraints, "full_scale_current")
	if !ok || fullScale <= 0 || tolerance <= 0 {
		return nil, fmt.Errorf("current sensing requires a positive full-scale current and tolerance")
	}
	if constraint, exists := namedConstraint(request.Constraints, "fail_safe_interlock"); !exists || constraint.Relation != "required" {
		return nil, fmt.Errorf("current sensing requires an explicit fail-safe interlock contract")
	}
	commonMode := roleVoltageMaximum(request.Ports, "power")
	_, measurementMaximum, ok := roleVoltageRange(request.Ports, "measurement")
	if !ok {
		_, measurementMaximum, ok = roleVoltageRange(request.Ports, "output")
	}
	targetOutput := measurementMaximum * 0.8
	if transimpedance, transferTolerance, hasTransfer := firstNumericConstraint(request.Constraints, "transimpedance"); hasTransfer && transimpedance > 0 {
		targetOutput = transimpedance * fullScale
		if !ok || measurementMaximum < targetOutput {
			measurementMaximum = targetOutput * (1 + math.Max(transferTolerance, 1)/100)
			ok = true
		}
	}
	if !ok || measurementMaximum <= 0 || targetOutput <= 0 {
		return nil, fmt.Errorf("current sensing requires a bounded positive measurement range")
	}
	sensor, err := provider.selectComponent(ctx, "current_sensor", "precision", []components.RequiredRating{
		{Kind: "supply_voltage", Value: "5", Unit: "V"},
		{Kind: "common_mode_voltage", Value: numericString(commonMode), Unit: "V"},
		{Kind: "bandwidth", Value: "10000", Unit: "Hz"},
	}, true)
	if err != nil {
		return nil, err
	}
	sensor.selected.InstanceID, sensor.usage = "current_monitor", "high_side_current_measurement"
	shunt, err := provider.selectComponent(ctx, "resistor", "WSLT2512", []components.RequiredRating{
		{Kind: "continuous_current", Value: numericString(fullScale), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	shunt.selected.InstanceID, shunt.usage = "current_shunt", "series_current_shunt"
	shunt.value = "10m"
	regulator, err := provider.selectComponentWithTemperature(ctx, "regulator", "adjustable", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(commonMode), Unit: "V"},
		{Kind: "output_current", Value: "0.01", Unit: "A"},
	}, true, temperatureRequirementFromConstraints(request.Constraints))
	if err != nil {
		return nil, err
	}
	regulator.selected.InstanceID, regulator.usage = "sense_supply", "local_measurement_supply"
	clamp, err := provider.selectComponent(ctx, "diode", "schottky", nil, true)
	if err != nil {
		return nil, err
	}
	clamp.selected.InstanceID, clamp.usage = "fault_clamp", "fail_safe_fault_clamp"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{sensor, shunt, regulator, clamp}, []passivePart{
		{"sense_feedback_lower", "resistor", "feedback_divider", "240"}, {"sense_feedback_upper", "resistor", "feedback_divider", "720"},
		{"sense_input_bypass", "capacitor", "decoupling", "1u"}, {"sense_output_bypass", "capacitor", "decoupling", "1u"},
		{"control_series", "resistor", "interlock_drive", "10k"}, {"permit_pulldown", "resistor", "default_off", "100k"},
	})
	if err != nil {
		return nil, err
	}
	gain, gainOK := recordValue(sensor.record, "gain", "V/V")
	gainError, gainErrorOK := recordValueMaximum(sensor.record, "gain_error", "%")
	offsetMicrovolts, offsetOK := recordValueMaximum(sensor.record, "input_offset_voltage", "uV")
	shuntResistance, shuntOK := recordValue(shunt.record, "resistance", "Ohm")
	shuntTolerance, shuntToleranceOK := recordValueMaximum(shunt.record, "tolerance", "%")
	shuntPower, shuntPowerOK := recordRatingMaximum(shunt.record, "power_dissipation", "W")
	if !gainOK || !gainErrorOK || !offsetOK || !shuntOK || !shuntToleranceOK || !shuntPowerOK {
		return nil, fmt.Errorf("selected current-sense chain lacks normalized gain, offset, shunt, tolerance, or power evidence")
	}
	transfer, transferIssues := SolveCurrentSense(CurrentSenseRequest{
		ID: "current_sense_transfer", FullScaleCurrentA: fullScale, TargetOutputVoltageV: targetOutput,
		OutputTolerancePercent: tolerance, MaximumOutputVoltageV: measurementMaximum,
		ShuntResistanceOhm: shuntResistance, ShuntTolerancePercent: shuntTolerance, ShuntPowerRatingW: shuntPower,
		AmplifierGain: gain, AmplifierGainErrorPercent: gainError, InputOffsetVoltageV: offsetMicrovolts / 1e6,
	})
	if len(transferIssues) != 0 {
		return nil, fmt.Errorf("current-sense transfer solution failed: %s", transferIssues[0].Message)
	}
	regulatorFeedback, feedbackIssues := SolveDivider(DividerRequest{
		ID: "sense_supply_feedback", Mode: DividerFeedback, SourceVoltageV: 1.25, SourceTolerancePercent: 0.5,
		TargetVoltageV: 5, TargetTolerancePercent: 2, LowerResistanceOhm: 240, LowerTolerancePercent: 0.1,
		UpperTolerancePercent: 0.1, UpperSeries: SeriesE96,
	})
	if len(feedbackIssues) != 0 {
		return nil, fmt.Errorf("current-sense supply solution failed: %s", feedbackIssues[0].Message)
	}
	allBindings := bindRoles(request.Ports, sensor.selected.InstanceID, map[string]string{
		"control": "IN_PLUS", "fault": "REF1", "measurement": "OUT", "output": "OUT", "permit": "OUT", "reference": "GND_A",
	})
	bindings := make([]RealizationPortBinding, 0, len(allBindings)-1)
	for _, binding := range allBindings {
		if binding.Role == "power" {
			continue
		}
		switch binding.Role {
		case "control":
			binding.Instance, binding.Function = "control_series", "A"
		case "fault":
			binding.Instance, binding.Function = clamp.selected.InstanceID, "K"
		case "permit":
			binding.Instance, binding.Function = "control_series", "B"
		case "reference":
			binding.Instance, binding.Function = sensor.selected.InstanceID, "GND_A"
		}
		bindings = append(bindings, binding)
	}
	connections := []RealizationConnection{
		semanticNet("sense_source_side", "power", endpoint(shunt, "A"), endpoint(sensor, "IN_PLUS"), endpoint(regulator, "VIN"), passiveEndpoint("sense_input_bypass", "A")),
		semanticNet("sense_load_side", "power", endpoint(shunt, "B"), endpoint(sensor, "IN_MINUS")),
		semanticNet("sense_supply", "power", endpoint(regulator, "VOUT"), endpoint(sensor, "VCC"), passiveEndpoint("sense_feedback_lower", "A"), passiveEndpoint("sense_output_bypass", "A")),
		semanticNet("sense_supply_feedback", "analog_signal", endpoint(regulator, "ADJ"), passiveEndpoint("sense_feedback_lower", "B"), passiveEndpoint("sense_feedback_upper", "A")),
		semanticNet("sense_measurement", "analog_signal", endpoint(sensor, "OUT"), passiveEndpoint("control_series", "A"), endpoint(clamp, "K")),
		semanticNet("permit_interlock", "control", passiveEndpoint("control_series", "B"), endpoint(clamp, "A"), passiveEndpoint("permit_pulldown", "A")),
		semanticNet("sense_reference", "reference", endpoint(sensor, "GND_A"), endpoint(sensor, "GND_B"), endpoint(sensor, "REF1"), endpoint(sensor, "REF2"), passiveEndpoint("sense_feedback_upper", "B"), passiveEndpoint("sense_input_bypass", "B"), passiveEndpoint("sense_output_bypass", "B"), passiveEndpoint("permit_pulldown", "B")),
	}
	transitions := []RealizationSeriesTransition{{Role: "power", Input: endpoint(shunt, "A"), Output: endpoint(shunt, "B")}}
	bandwidth, bandwidthOK := recordRatingMaximum(sensor.record, "bandwidth", "Hz")
	if !bandwidthOK || bandwidth <= 0 {
		return nil, fmt.Errorf("selected current sensor lacks bandwidth response evidence")
	}
	response, err := ObservedCalculation("current_sensor_response", NamedQuantity{Name: "response_time", Value: 1 / bandwidth, Unit: "s"})
	if err != nil {
		return nil, err
	}
	return provider.expansionWithTransitions(request, "precision_high_side_current_interlock", parts, bindings, transitions, connections, []CalculationEvidence{transfer, regulatorFeedback, response}, 0)
}

func (provider *CatalogProvider) expandMuteControl(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	powerMinimum, powerMaximum, powerOK := roleVoltageRange(request.Ports, "power")
	if !powerOK || powerMinimum <= 0 || powerMaximum < powerMinimum {
		return nil, fmt.Errorf("mute control requires a bounded positive logic supply")
	}
	muteIndex := slices.IndexFunc(request.Ports, func(port RoleContract) bool { return port.Role == "mute" })
	hasSeriesSignalPath := hasRoleContract(request.Ports, "signal") || hasRoleContract(request.Ports, "input") || hasRoleContract(request.Ports, "output")
	if muteIndex >= 0 && request.Ports[muteIndex].Contract.Direction == "source" && !hasSeriesSignalPath {
		return provider.expandLogicMuteControl(ctx, request, powerMaximum)
	}
	relay, err := provider.selectComponent(ctx, "relay", "solid_state", []components.RequiredRating{
		{Kind: "contact_current_dc", Value: "0.01", Unit: "A"},
		{Kind: "contact_voltage_dc", Value: numericString(powerMaximum), Unit: "V"},
	}, true)
	if err != nil {
		return nil, err
	}
	coilVoltage, coilVoltageOK := recordValue(relay.record, "coil_voltage", "V")
	controlCurrent, controlCurrentOK := recordValue(relay.record, "control_current", "A")
	if !coilVoltageOK || !controlCurrentOK || coilVoltage <= 0 || controlCurrent <= 0 || powerMinimum <= coilVoltage {
		return nil, fmt.Errorf("selected mute relay lacks compatible bounded control-drive evidence")
	}
	transistor, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(powerMaximum), Unit: "V"},
		{Kind: "collector_current", Value: numericString(controlCurrent), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	relay.selected.InstanceID, relay.usage = "mute_relay", "fail_safe_bidirectional_mute"
	transistor.selected.InstanceID, transistor.usage = "mute_driver", "mute_relay_driver"
	// Size from the minimum declared logic rail so the catalog operate current
	// is met at every supply corner; the maximum rail remains bounded by the
	// relay's reviewed control-current rating.
	designPower := powerMinimum
	coilSeriesResistance := math.Max(1, (designPower-coilVoltage)/controlCurrent)
	outputPulldownResistance := 1_000_000.0
	if mutedLimit, _, mutedOK := firstNumericConstraint(request.Constraints, "muted_output_voltage"); mutedOK && mutedLimit > 0 {
		outputPeak := 0.0
		if power, _, powerOK := firstNumericConstraint(request.Constraints, "continuous_output_power"); powerOK && power > 0 {
			if load, _, loadOK := firstNumericConstraint(request.Constraints, "load_impedance"); loadOK && load > 0 {
				outputPeak = math.Sqrt(2 * power * load)
			}
		}
		if swing, _, swingOK := firstNumericConstraint(request.Constraints, "output_swing"); swingOK && swing > 0 {
			outputPeak = math.Max(outputPeak, swing/2)
		}
		if outputPeak <= mutedLimit {
			if contactVoltage, contactVoltageOK := recordValue(relay.record, "contact_voltage_dc", "V"); contactVoltageOK {
				outputPeak = contactVoltage
			}
		}
		contactOffResistance, offResistanceOK := catalogSimulationParameter(relay.record, "contact_off_resistance_ohm")
		if outputPeak > mutedLimit && offResistanceOK && contactOffResistance > 0 {
			// The series contact and pull-down form the fail-safe attenuation path.
			// Output peak and downstream closed-loop gain cancel when both normal
			// and muted requirements observe the same output, so the required
			// attenuation is bounded directly by their declared voltage ratio.
			// Reserve half the permitted residual for catalog/model uncertainty.
			if resistance, solved := failSafeMutePulldownResistance(mutedLimit, outputPeak, contactOffResistance); solved {
				outputPulldownResistance = resistance
			}
		}
	}
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{relay, transistor}, []passivePart{
		{"enable_resistor", "resistor", "logic_drive", "10k"},
		{"enable_pulldown", "resistor", "default_mute", "100k"},
		{"coil_series", "resistor", "relay_control_current", engineeringValue(coilSeriesResistance, "Ohm")},
		{"output_pulldown", "resistor", "default_mute", engineeringValue(outputPulldownResistance, "Ohm")},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, relay.selected.InstanceID, map[string]string{
		"signal": "CONTACT_IN", "input": "CONTACT_IN", "output": "CONTACT_OUT",
		"enable": "COIL_A", "control": "COIL_A", "mute": "COIL_A", "power": "COIL_A", "positive_power": "COIL_A",
		"reference": "COIL_B", "negative_power": "COIL_B",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "enable", "control":
			bindings[index].Instance, bindings[index].Function = "enable_resistor", "A"
		case "mute":
			portIndex := slices.IndexFunc(request.Ports, func(port RoleContract) bool { return port.Role == "mute" })
			if portIndex >= 0 && request.Ports[portIndex].Contract.Direction == "source" {
				// A composed mute-state output observes the switched coil return.
				// A mute command input still drives the bounded base resistor. This
				// direction-based distinction keeps both roles electrically unique
				// when a controller command and a downstream mute-state signal coexist.
				bindings[index].Instance, bindings[index].Function = transistor.selected.InstanceID, "COLLECTOR"
			} else {
				bindings[index].Instance, bindings[index].Function = "enable_resistor", "A"
			}
		case "power", "positive_power":
			bindings[index].Instance, bindings[index].Function = "coil_series", "A"
		case "reference", "negative_power":
			bindings[index].Instance, bindings[index].Function = transistor.selected.InstanceID, "EMITTER"
		}
	}
	connections := []RealizationConnection{
		semanticNet("mute_enable_drive", "control", passiveEndpoint("enable_resistor", "B"), passiveEndpoint("enable_pulldown", "A"), endpoint(transistor, "BASE")),
		semanticNet("mute_coil_supply", "power", passiveEndpoint("coil_series", "B"), endpoint(relay, "COIL_A")),
		semanticNet("mute_coil_return", "control", endpoint(relay, "COIL_B"), endpoint(transistor, "COLLECTOR")),
		semanticNet("mute_output", "analog_signal", endpoint(relay, "CONTACT_OUT"), passiveEndpoint("output_pulldown", "A")),
		semanticNet("mute_reference", "reference", endpoint(transistor, "EMITTER"), passiveEndpoint("enable_pulldown", "B"), passiveEndpoint("output_pulldown", "B")),
	}
	return provider.expansion(request, "fail_safe_bidirectional_series_mute", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandLogicMuteControl(ctx context.Context, request ProviderRequest, powerMaximum float64) ([]ProviderExpansion, error) {
	switchDevice, err := provider.selectComponent(ctx, "mosfet", "logic-level N-channel", []components.RequiredRating{
		{Kind: "drain_source_voltage", Value: numericString(powerMaximum), Unit: "V"},
	}, true)
	if err != nil {
		return nil, err
	}
	switchDevice.selected.InstanceID, switchDevice.usage = "mute_switch", "fail_safe_logic_mute"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{switchDevice}, []passivePart{
		{"gate_pulldown", "resistor", "default_mute", "100k"},
		{"output_pullup", "resistor", "logic_pullup", "10k"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, switchDevice.selected.InstanceID, map[string]string{
		"enable": "GATE", "control": "GATE", "mute": "DRAIN", "output": "DRAIN",
		"power": "DRAIN", "positive_power": "DRAIN", "reference": "SOURCE", "negative_power": "SOURCE",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "power", "positive_power":
			bindings[index].Instance, bindings[index].Function = "output_pullup", "A"
		}
	}
	connections := []RealizationConnection{
		semanticNet("mute_logic_drive", "control", endpoint(switchDevice, "GATE"), passiveEndpoint("gate_pulldown", "A")),
		semanticNet("mute_logic_output", "control", endpoint(switchDevice, "DRAIN"), passiveEndpoint("output_pullup", "B")),
		semanticNet("mute_logic_reference", "reference", endpoint(switchDevice, "SOURCE"), passiveEndpoint("gate_pulldown", "B")),
	}
	return provider.expansion(request, "fail_safe_open_drain_logic_mute", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandClassABBias(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if constraint, ok := namedConstraint(request.Constraints, "thermal_tracking"); !ok || constraint.Relation != "required" {
		return nil, fmt.Errorf("Class-AB bias control requires thermal tracking")
	}
	diode, err := provider.selectComponent(ctx, "diode", "switching", nil, true)
	if err != nil {
		return nil, err
	}
	diode.selected.InstanceID, diode.usage = "tracking_diode_1", "thermal_tracking"
	secondDiode := diode
	secondDiode.selected.InstanceID = "tracking_diode_2"
	hasEnable := hasRoleContract(request.Ports, "enable") || hasRoleContract(request.Ports, "input")
	catalogParts := []catalogPart{diode, secondDiode}
	passives := []passivePart{{"bias_feed", "resistor", "bias_current", "10k"}}
	var inverter, clamp catalogPart
	if hasEnable {
		supply := maximumPortVoltage(request.Ports)
		inverter, err = provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{{Kind: "collector_emitter_voltage", Value: numericString(supply), Unit: "V"}}, true)
		if err != nil {
			return nil, err
		}
		inverter.selected.InstanceID, inverter.usage = "bias_enable_inverter", "fail_safe_enable"
		clamp = inverter
		clamp.selected.InstanceID, clamp.usage = "bias_clamp", "startup_bias_clamp"
		catalogParts = append(catalogParts, inverter, clamp)
		passives = append(passives,
			passivePart{"enable_resistor", "resistor", "logic_drive", "10k"},
			passivePart{"inverter_pullup", "resistor", "default_clamp", "10k"},
		)
	}
	parts, err := provider.appendPassiveParts(ctx, catalogParts, passives)
	if err != nil {
		return nil, err
	}
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
	bindings := bindRoles(request.Ports, diode.selected.InstanceID, map[string]string{
		"enable": "A", "input": "A", "bias": "A", "output": "A",
		"power": "A", "positive_power": "A", "reference": "K", "negative_power": "K",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "enable", "input":
			if hasEnable {
				bindings[index].Instance, bindings[index].Function = "enable_resistor", "A"
			}
		case "bias", "output":
			bindings[index].Instance, bindings[index].Function = diode.selected.InstanceID, "A"
		case "power", "positive_power":
			bindings[index].Instance, bindings[index].Function = "bias_feed", "A"
		case "reference", "negative_power":
			bindings[index].Instance, bindings[index].Function = secondDiode.selected.InstanceID, "K"
		}
	}
	biasOutputEndpoints := []RealizationEndpoint{passiveEndpoint("bias_feed", "B"), endpoint(diode, "A")}
	biasPowerEndpoints := []RealizationEndpoint{passiveEndpoint("bias_feed", "A")}
	connections := []RealizationConnection{
		semanticNet("bias_output", "analog_control", biasOutputEndpoints...),
		semanticNet("tracking_junction", "analog_control", endpoint(diode, "K"), endpoint(secondDiode, "A")),
	}
	lowerRailID, lowerRailRole := "bias_reference", "reference"
	if hasNegativePower {
		lowerRailID, lowerRailRole = "bias_negative_power", "power"
	}
	lowerRailEndpoints := []RealizationEndpoint{endpoint(secondDiode, "K")}
	if hasEnable {
		connections = append(connections,
			semanticNet("bias_enable_drive", "control", passiveEndpoint("enable_resistor", "B"), endpoint(inverter, "BASE")),
			semanticNet("bias_clamp_drive", "control", endpoint(inverter, "COLLECTOR"), passiveEndpoint("inverter_pullup", "B"), endpoint(clamp, "BASE")),
		)
		for index := range connections {
			if connections[index].ID == "bias_output" {
				connections[index].Endpoints = append(connections[index].Endpoints, endpoint(clamp, "COLLECTOR"))
			}
		}
		biasPowerEndpoints = append(biasPowerEndpoints, passiveEndpoint("inverter_pullup", "A"))
		lowerRailEndpoints = append(lowerRailEndpoints, endpoint(inverter, "EMITTER"), endpoint(clamp, "EMITTER"))
	}
	if len(biasPowerEndpoints) > 1 {
		connections = append(connections, semanticNet("bias_power", "power", biasPowerEndpoints...))
	}
	if len(lowerRailEndpoints) > 1 {
		connections = append(connections, semanticNet(lowerRailID, lowerRailRole, lowerRailEndpoints...))
	}
	return provider.expansion(request, "thermally_tracked_fail_safe_bias", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandClassABOutput(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	load, _, ok := firstNumericConstraint(request.Constraints, "load_impedance")
	if !ok || load <= 0 {
		return nil, fmt.Errorf("Class-AB output requires a positive load impedance")
	}
	power, _, ok := firstNumericConstraint(request.Constraints, "continuous_output_power")
	if !ok || power <= 0 {
		return nil, fmt.Errorf("Class-AB output requires a positive continuous-output-power bound")
	}
	peakCurrent := math.Sqrt(2 * power / load)
	supply := maximumPortVoltage(request.Ports)
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
	thermalRailMagnitude := supply
	if !hasNegativePower {
		// A ground-referenced, capacitor-coupled single-supply output biases at
		// midrail, so each output device dissipates against half the total rail
		// span. Split supplies already express one rail magnitude per role.
		thermalRailMagnitude = supply / 2
	}
	stageDissipation := math.Pow(thermalRailMagnitude, 2) / (math.Pi * math.Pi * load)
	thermalRequirement := thermalRequirementFromConstraints(request.Constraints, stageDissipation)
	selectionThermalRequirement := thermalRequirement
	if selectionThermalRequirement == nil {
		selectionThermalRequirement = &components.ThermalRequirement{
			DissipationW:          stageDissipation,
			Reference:             components.ThermalReferenceAmbient,
			ReferenceTemperatureC: 25,
		}
	}
	const maximumParallelOutputDevices = 8
	var npn, pnp catalogPart
	parallelOutputCount := 0
	var lastSelectionError error
	for count := 1; count <= maximumParallelOutputDevices; count++ {
		deviceCurrent := peakCurrent / float64(count)
		deviceDissipation := stageDissipation / float64(count)
		copy := *selectionThermalRequirement
		copy.DissipationW = deviceDissipation
		deviceThermalRequirement := &copy
		npnCandidate, npnErr := provider.selectComponentWithThermal(ctx, "bjt", "NPN audio", []components.RequiredRating{
			{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
			{Kind: "collector_current", Value: numericString(deviceCurrent / catalogRatingDeratingFactor), Unit: "A"},
			{Kind: "power_dissipation", Value: numericString(deviceDissipation / catalogRatingDeratingFactor), Unit: "W"},
		}, true, deviceThermalRequirement)
		if npnErr != nil {
			lastSelectionError = npnErr
			continue
		}
		pnpCandidate, pnpErr := provider.selectComponentWithThermal(ctx, "bjt", "PNP audio", []components.RequiredRating{
			{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
			{Kind: "collector_current", Value: numericString(deviceCurrent / catalogRatingDeratingFactor), Unit: "A"},
			{Kind: "power_dissipation", Value: numericString(deviceDissipation / catalogRatingDeratingFactor), Unit: "W"},
		}, true, deviceThermalRequirement)
		if pnpErr != nil {
			lastSelectionError = pnpErr
			continue
		}
		npn, pnp, parallelOutputCount = npnCandidate, pnpCandidate, count
		break
	}
	if parallelOutputCount == 0 {
		return nil, fmt.Errorf("Class-AB output bank lacks a bounded catalog-backed current and thermal solution: %w", lastSelectionError)
	}
	npn.selected.InstanceID, npn.usage = "output_npn_1", "class_ab_output"
	pnp.selected.InstanceID, pnp.usage = "output_pnp_1", "class_ab_output"
	npnBeta, npnBetaOK := catalogSimulationParameter(npn.record, "forward_beta")
	pnpBeta, pnpBetaOK := catalogSimulationParameter(pnp.record, "forward_beta")
	if !npnBetaOK || !pnpBetaOK || npnBeta <= 0 || pnpBeta <= 0 {
		return nil, fmt.Errorf("selected Class-AB output pair lacks reviewed forward-beta evidence")
	}
	opamp, err := provider.selectComponent(ctx, "opamp", "single rail_to_rail", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	opamp.selected.InstanceID, opamp.usage = "voltage_driver", "class_ab_voltage_driver"
	// The predriver carries the output bank's aggregate base current, not the
	// speaker current. Preserve a bounded 20 percent drive margin plus 5 mA for
	// the bias network; overstating this as a power-stage current forces a
	// low-gain power transistor into the voltage-driver position and consumes
	// the very rail headroom that the complementary-feedback pair is meant to
	// preserve.
	driverCurrent := math.Max(.02, 1.2*peakCurrent/math.Min(npnBeta, pnpBeta)+.005)
	// For a complementary Class-B/AB stage, maximum steady-state dissipation in
	// either predriver is the output-device dissipation reflected through the
	// guaranteed output-device beta. A rail-voltage times peak-current product
	// is an instantaneous bound and substantially overstates the thermal load.
	driverDissipation := stageDissipation / math.Min(npnBeta, pnpBeta)
	driverThermalRequirement := thermalRequirementFromConstraints(request.Constraints, driverDissipation)
	if driverThermalRequirement == nil {
		driverThermalRequirement = &components.ThermalRequirement{
			DissipationW:          driverDissipation,
			Reference:             components.ThermalReferenceAmbient,
			ReferenceTemperatureC: 25,
		}
	}
	npnDriver, err := provider.selectComponentWithThermal(ctx, "bjt", "NPN medium-power driver", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(driverCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true, driverThermalRequirement)
	if err != nil {
		return nil, err
	}
	pnpDriver, err := provider.selectComponentWithThermal(ctx, "bjt", "PNP medium-power driver", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(driverCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true, driverThermalRequirement)
	if err != nil {
		return nil, err
	}
	npnDriver.selected.InstanceID, npnDriver.usage = "driver_npn", "class_ab_predriver"
	pnpDriver.selected.InstanceID, pnpDriver.usage = "driver_pnp", "class_ab_predriver"
	npnDriverBeta, npnDriverBetaOK := catalogSimulationParameter(npnDriver.record, "forward_beta")
	pnpDriverBeta, pnpDriverBetaOK := catalogSimulationParameter(pnpDriver.record, "forward_beta")
	if !npnDriverBetaOK || !pnpDriverBetaOK || npnDriverBeta <= 0 || pnpDriverBeta <= 0 {
		return nil, fmt.Errorf("selected Class-AB driver pair lacks reviewed forward-beta evidence")
	}
	// Catalog-identical diode-connected devices establish the two-junction
	// Class-AB spread. Using the same device model for each driver and its tracker
	// keeps the bias compensation deterministic across temperature corners.
	upperBiasTracker := npnDriver
	upperBiasTracker.selected.InstanceID, upperBiasTracker.usage = "upper_bias_tracker", "class_ab_thermal_bias"
	lowerBiasTracker := pnpDriver
	lowerBiasTracker.selected.InstanceID, lowerBiasTracker.usage = "lower_bias_tracker", "class_ab_thermal_bias"
	supplySpan := maximumSupplySpan(request.Ports)
	if supplySpan <= 0 {
		return nil, fmt.Errorf("Class-AB output requires a positive supply span")
	}
	requiredPeakVoltage := math.Sqrt(2 * power * load)
	availablePeakVoltage := supplySpan / 2
	if positiveMinimum, positiveMaximum, positiveOK := roleVoltageRange(request.Ports, "positive_power"); positiveOK {
		if negativeMinimum, negativeMaximum, negativeOK := roleVoltageRange(request.Ports, "negative_power"); negativeOK {
			positiveRail := math.Min(math.Abs(positiveMinimum), math.Abs(positiveMaximum))
			negativeRail := math.Min(math.Abs(negativeMinimum), math.Abs(negativeMaximum))
			availablePeakVoltage = math.Min(positiveRail, negativeRail)
		}
	}
	railHeadroom := availablePeakVoltage - requiredPeakVoltage
	if railHeadroom <= 0 {
		return nil, fmt.Errorf("Class-AB output-power target exceeds the minimum available rail swing")
	}
	devicePeakCurrent := peakCurrent / float64(parallelOutputCount)
	// The complementary-feedback pair must preserve most of the worst-case rail
	// headroom for the output junction and the conducting driver's VCE. Allocate
	// only bounded fractions to ballast and drive resistors; simulation remains
	// authoritative for the resulting clipping and distortion margins.
	emitterResistanceIdeal := math.Min(.22, railHeadroom*.05/devicePeakCurrent)
	emitterCandidates, emitterIssues := PreferredValueCandidates(emitterResistanceIdeal, SeriesE24, .01, emitterResistanceIdeal, 1)
	if len(emitterIssues) != 0 || len(emitterCandidates) == 0 {
		return nil, fmt.Errorf("Class-AB emitter-ballast solution failed")
	}
	emitterResistance := emitterCandidates[0]
	outputHeadroom := railHeadroom - devicePeakCurrent*emitterResistance
	if outputHeadroom <= 0 {
		return nil, fmt.Errorf("Class-AB output-power target leaves no bounded output-stage headroom")
	}
	peakBaseCurrent := peakCurrent / math.Min(npnBeta, pnpBeta)
	baseStopIdeal := math.Min(47, outputHeadroom*.05/peakBaseCurrent)
	baseStopCandidates, baseStopIssues := PreferredValueCandidates(baseStopIdeal, SeriesE24, .1, baseStopIdeal, 1)
	if len(baseStopIssues) != 0 || len(baseStopCandidates) == 0 {
		return nil, fmt.Errorf("Class-AB output base-stop solution failed")
	}
	outputBaseStop := baseStopCandidates[0]
	driverEmitterIdeal := math.Min(10, outputHeadroom*.05/peakBaseCurrent)
	driverEmitterCandidates, driverEmitterIssues := PreferredValueCandidates(driverEmitterIdeal, SeriesE24, .1, driverEmitterIdeal, 1)
	if len(driverEmitterIssues) != 0 || len(driverEmitterCandidates) == 0 {
		return nil, fmt.Errorf("Class-AB driver-degeneration solution failed")
	}
	driverEmitterResistance := driverEmitterCandidates[0]
	peakDriverBaseCurrent := driverCurrent / math.Min(npnDriverBeta, pnpDriverBeta)
	driverInputDriveVoltage := availablePeakVoltage - .8
	if driverInputDriveVoltage <= 0 {
		return nil, fmt.Errorf("Class-AB driver input lacks bounded rail headroom")
	}
	driverInputBaseStopIdeal := math.Min(68, driverInputDriveVoltage*.25/peakDriverBaseCurrent)
	driverInputBaseStopCandidates, driverInputBaseStopIssues := PreferredValueCandidates(driverInputBaseStopIdeal, SeriesE24, .1, driverInputBaseStopIdeal, 1)
	if len(driverInputBaseStopIssues) != 0 || len(driverInputBaseStopCandidates) == 0 {
		return nil, fmt.Errorf("Class-AB driver input base-stop solution failed")
	}
	driverInputBaseStop := driverInputBaseStopCandidates[0]
	// Keep the turn-off shunt much larger than the series stopper so it removes
	// stored base charge without consuming material signal-frequency headroom.
	// A fixed ratio makes the pair scale with the calculated drive network while
	// preferred-value selection keeps the emitted circuit reproducible.
	driverTurnoffIdeal := 150 * driverInputBaseStop
	driverTurnoffCandidates, driverTurnoffIssues := PreferredValueCandidates(driverTurnoffIdeal, SeriesE24, driverTurnoffIdeal*.5, driverTurnoffIdeal*2, 1)
	if len(driverTurnoffIssues) != 0 || len(driverTurnoffCandidates) == 0 {
		return nil, fmt.Errorf("Class-AB driver turn-off solution failed")
	}
	driverTurnoffResistance := driverTurnoffCandidates[0]
	outputNPNs := make([]catalogPart, parallelOutputCount)
	outputPNPs := make([]catalogPart, parallelOutputCount)
	for index := 0; index < parallelOutputCount; index++ {
		outputNPNs[index] = npn
		outputNPNs[index].selected.InstanceID = fmt.Sprintf("output_npn_%d", index+1)
		outputPNPs[index] = pnp
		outputPNPs[index].selected.InstanceID = fmt.Sprintf("output_pnp_%d", index+1)
	}
	gain, gainTolerance, hasGain := firstNumericConstraint(request.Constraints, "voltage_gain")
	if !hasGain {
		gain = 1
	}
	if gain < 1 {
		return nil, fmt.Errorf("Class-AB output requires a voltage gain of at least one")
	}
	upperBiasFeedResistance, lowerBiasFeedResistance := 2000.0, 2000.0
	var biasFeedCalculation *CalculationEvidence
	if minimumBias, maximumBias, ok := numericConstraintBounds(request.Constraints, "quiescent_current"); ok {
		targetBias := .5 * (minimumBias + maximumBias)
		// A bounded fraction of the requested total idle current biases each
		// diode-connected tracker. The rest remains available to the output bank,
		// voltage driver, protection, and rail-divider loads. Scaling this current
		// with the public target keeps the provider useful beyond one fixed rail or
		// amplifier size.
		trackerCurrent := math.Max(.005, .18*targetBias)
		biasFeed := func(driver catalogPart) (float64, error) {
			saturationCurrent, saturationOK := catalogSimulationParameter(driver.record, "saturation_current_a")
			emission, emissionOK := catalogSimulationParameter(driver.record, "emission_coefficient")
			temperatureK, temperatureOK := catalogSimulationParameter(driver.record, "junction_temperature_k")
			if !saturationOK || !emissionOK || !temperatureOK || saturationCurrent <= 0 || emission <= 0 || temperatureK <= 0 {
				return 0, fmt.Errorf("selected Class-AB driver lacks reviewed junction evidence for bias-feed sizing")
			}
			const boltzmannVoltsPerKelvin = 8.617333262145e-5
			junctionVoltage := emission * boltzmannVoltsPerKelvin * temperatureK * math.Log1p(trackerCurrent/saturationCurrent)
			ideal := (availablePeakVoltage - junctionVoltage) / trackerCurrent
			if !finiteNumbers(junctionVoltage, ideal) || junctionVoltage <= 0 || ideal <= 0 {
				return 0, fmt.Errorf("Class-AB bias-feed target exceeds the minimum available rail magnitude")
			}
			candidates, issues := PreferredValueCandidates(ideal, SeriesE24, ideal*.5, ideal*2, 1)
			if len(issues) != 0 || len(candidates) == 0 {
				return 0, fmt.Errorf("Class-AB bias-feed preferred-value solution failed")
			}
			return candidates[0], nil
		}
		var biasErr error
		upperBiasFeedResistance, biasErr = biasFeed(npnDriver)
		if biasErr != nil {
			return nil, biasErr
		}
		lowerBiasFeedResistance, biasErr = biasFeed(pnpDriver)
		if biasErr != nil {
			return nil, biasErr
		}
		calculation, calculationErr := ObservedCalculation("class_ab_bias_feed",
			NamedQuantity{Name: "minimum_rail_magnitude", Value: availablePeakVoltage, Unit: "V"},
			NamedQuantity{Name: "target_quiescent_current", Value: targetBias, Unit: "A"},
			NamedQuantity{Name: "tracker_current", Value: trackerCurrent, Unit: "A"},
			NamedQuantity{Name: "upper_feed_resistance", Value: upperBiasFeedResistance, Unit: "Ohm"},
			NamedQuantity{Name: "lower_feed_resistance", Value: lowerBiasFeedResistance, Unit: "Ohm"},
		)
		if calculationErr != nil {
			return nil, calculationErr
		}
		biasFeedCalculation = &calculation
	}
	var gainCalculation, compensationCalculation *CalculationEvidence
	passives := []passivePart{
		{"input_coupling", "capacitor", "ac_coupling", "4.7u"},
		{"input_bias", "resistor", "input_bias", "1M"},
		{"midpoint_upper", "resistor", "midpoint_bias", "47k"}, {"midpoint_lower", "resistor", "midpoint_bias", "47k"},
		{"upper_bias_feed", "resistor", "bias_current", engineeringValue(upperBiasFeedResistance, "Ohm")}, {"lower_bias_feed", "resistor", "bias_current", engineeringValue(lowerBiasFeedResistance, "Ohm")},
		{"npn_base_stop", "resistor", "base_stop", engineeringValue(driverInputBaseStop, "Ohm")}, {"pnp_base_stop", "resistor", "base_stop", engineeringValue(driverInputBaseStop, "Ohm")},
		{"npn_driver_emitter", "resistor", "driver_emitter_degeneration", engineeringValue(driverEmitterResistance, "Ohm")}, {"pnp_driver_emitter", "resistor", "driver_emitter_degeneration", engineeringValue(driverEmitterResistance, "Ohm")},
		{"npn_driver_base_emitter", "resistor", "driver_turnoff", engineeringValue(driverTurnoffResistance, "Ohm")}, {"pnp_driver_base_emitter", "resistor", "driver_turnoff", engineeringValue(driverTurnoffResistance, "Ohm")},
		{"npn_output_base_stop", "resistor", "base_stop", engineeringValue(outputBaseStop, "Ohm")}, {"pnp_output_base_stop", "resistor", "base_stop", engineeringValue(outputBaseStop, "Ohm")},
		{"npn_output_base_emitter", "resistor", "output_bank_turnoff", "100"}, {"pnp_output_base_emitter", "resistor", "output_bank_turnoff", "100"},
		{"supply_bypass", "capacitor", "decoupling_capacitor", "100n"},
	}
	for index := 0; index < parallelOutputCount; index++ {
		suffix := fmt.Sprintf("_%d", index+1)
		passives = append(passives,
			passivePart{"npn_emitter" + suffix, "resistor", "emitter_resistor", engineeringValue(emitterResistance, "Ohm")},
			passivePart{"pnp_emitter" + suffix, "resistor", "emitter_resistor", engineeringValue(emitterResistance, "Ohm")},
		)
	}
	if gain > 1 {
		if gainTolerance == 0 {
			gainTolerance = 2
		}
		calculation, lower, issues := solveAmplifierGain(gain, gainTolerance)
		if len(issues) != 0 {
			return nil, fmt.Errorf("Class-AB gain solution failed: %s", issues[0].Message)
		}
		upper, ok := calculationSelectedValue(calculation, "upper_resistance")
		if !ok {
			return nil, fmt.Errorf("Class-AB gain solution omitted feedback resistance")
		}
		gainCalculation = &calculation
		opampBandwidth, bandwidthOK := catalogSimulationParameter(opamp.record, "gain_bandwidth_hz")
		if !bandwidthOK || opampBandwidth <= 0 {
			return nil, fmt.Errorf("Class-AB feedback compensation requires reviewed op-amp gain-bandwidth evidence")
		}
		compensationPoleHz := opampBandwidth / (4 * gain)
		// This capacitor closes the nested loop from the op-amp output to the
		// inverting node while the global feedback resistor returns from the
		// power-stage output. Its first-order interaction is therefore with the
		// upper (output-side) feedback resistor, not the lower resistor to the
		// signal reference. Using the lower resistor over-compensates every gain
		// above unity by approximately the closed-loop gain.
		idealCompensation := 1 / (2 * math.Pi * upper * compensationPoleHz)
		compensationCandidates, compensationIssues := PreferredValueCandidates(idealCompensation, SeriesE12, idealCompensation*.5, idealCompensation*2, 1)
		if len(compensationIssues) != 0 || len(compensationCandidates) == 0 {
			return nil, fmt.Errorf("Class-AB feedback compensation value solution failed")
		}
		compensation := compensationCandidates[0]
		compensationEvidence, evidenceErr := ObservedCalculation("class_ab_feedback_compensation",
			NamedQuantity{Name: "opamp_gain_bandwidth", Value: opampBandwidth, Unit: "Hz"},
			NamedQuantity{Name: "closed_loop_gain", Value: gain, Unit: "ratio"},
			NamedQuantity{Name: "feedback_resistance", Value: upper, Unit: "Ohm"},
			NamedQuantity{Name: "compensation_pole", Value: compensationPoleHz, Unit: "Hz"},
			NamedQuantity{Name: "compensation_capacitance", Value: compensation, Unit: "F"},
		)
		if evidenceErr != nil {
			return nil, evidenceErr
		}
		compensationCalculation = &compensationEvidence
		passives = append(passives,
			passivePart{"feedback_upper", "resistor", "gain_feedback", engineeringValue(upper, "Ohm")},
			passivePart{"feedback_lower", "resistor", "gain_feedback", engineeringValue(lower, "Ohm")},
			passivePart{"feedback_compensation", "capacitor", "loop_compensation", engineeringValue(compensation, "F")},
		)
	}
	if !hasNegativePower {
		passives = append(passives, passivePart{"midpoint_bypass", "capacitor", "midpoint_bypass", "100u"})
	}
	if hasRoleContract(request.Ports, "bias") {
		passives = append(passives, passivePart{"bias_injection", "resistor", "bias_injection", "1G"})
	}
	if hasRoleContract(request.Ports, "mute") {
		passives = append(passives, passivePart{"mute_drive", "resistor", "mute_drive", "10k"})
	}
	activeParts := []catalogPart{opamp, npnDriver, pnpDriver, upperBiasTracker, lowerBiasTracker}
	activeParts = append(activeParts, outputNPNs...)
	activeParts = append(activeParts, outputPNPs...)
	parts, err := provider.appendPassiveParts(ctx, activeParts, passives)
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, opamp.selected.InstanceID, map[string]string{
		"input": "IN_PLUS", "bias": "IN_PLUS", "mute": "IN_PLUS", "output": "OUT",
		"power": "V_PLUS", "positive_power": "V_PLUS", "reference": "V_MINUS", "negative_power": "V_MINUS",
	})
	for index := range bindings {
		switch bindings[index].Role {
		case "input":
			bindings[index].Instance, bindings[index].Function = "input_coupling", "A"
		case "bias":
			bindings[index].Instance, bindings[index].Function = "bias_injection", "A"
		case "mute":
			bindings[index].Instance, bindings[index].Function = "mute_drive", "A"
		case "output":
			bindings[index].Instance, bindings[index].Function = pnp.selected.InstanceID, "COLLECTOR"
		case "reference":
			if hasNegativePower {
				bindings[index].Instance, bindings[index].Function = "input_bias", "B"
			}
		}
	}
	driverInputEndpoints := []RealizationEndpoint{passiveEndpoint("input_coupling", "B"), endpoint(opamp, "IN_PLUS"), passiveEndpoint("input_bias", "A")}
	if hasRoleContract(request.Ports, "mute") {
		driverInputEndpoints = append(driverInputEndpoints, passiveEndpoint("mute_drive", "B"))
	}
	midpointEndpoints := []RealizationEndpoint{passiveEndpoint("midpoint_upper", "B"), passiveEndpoint("midpoint_lower", "A"), passiveEndpoint("input_bias", "B")}
	if !hasNegativePower {
		midpointEndpoints = append(midpointEndpoints, passiveEndpoint("midpoint_bypass", "A"))
	}
	baseDriveEndpoints := []RealizationEndpoint{endpoint(opamp, "OUT"), endpoint(upperBiasTracker, "EMITTER"), endpoint(lowerBiasTracker, "EMITTER"), passiveEndpoint("npn_base_stop", "A"), passiveEndpoint("pnp_base_stop", "A")}
	if hasRoleContract(request.Ports, "bias") {
		baseDriveEndpoints = append(baseDriveEndpoints, passiveEndpoint("bias_injection", "B"))
	}
	// Each opposite-polarity driver and power transistor forms a
	// complementary-feedback pair. Returning the driver emitter to the output
	// closes the pair's local feedback; returning it to the signal reference
	// instead turns the stage into an open-loop common-emitter cascade whose
	// bias and gain collapse across ordinary rail and temperature corners.
	// Base-stop names follow the driver polarity: the NPN driver's collector
	// drives the PNP output bases, and the PNP driver's collector drives the NPN
	// output bases.
	pnpBaseEndpoints := []RealizationEndpoint{passiveEndpoint("npn_output_base_stop", "B"), passiveEndpoint("pnp_output_base_emitter", "A")}
	npnBaseEndpoints := []RealizationEndpoint{passiveEndpoint("pnp_output_base_stop", "B"), passiveEndpoint("npn_output_base_emitter", "A")}
	classABOutputEndpoints := []RealizationEndpoint{passiveEndpoint("npn_driver_emitter", "B"), passiveEndpoint("pnp_driver_emitter", "B")}
	classABPowerEndpoints := []RealizationEndpoint{passiveEndpoint("midpoint_upper", "A"), passiveEndpoint("upper_bias_feed", "A"), passiveEndpoint("supply_bypass", "A"), endpoint(opamp, "V_PLUS")}
	classABNegativeEndpoints := []RealizationEndpoint{passiveEndpoint("midpoint_lower", "B"), passiveEndpoint("lower_bias_feed", "B"), passiveEndpoint("supply_bypass", "B"), endpoint(opamp, "V_MINUS")}
	for index := 0; index < parallelOutputCount; index++ {
		suffix := fmt.Sprintf("_%d", index+1)
		pnpBaseEndpoints = append(pnpBaseEndpoints, endpoint(outputPNPs[index], "BASE"))
		npnBaseEndpoints = append(npnBaseEndpoints, endpoint(outputNPNs[index], "BASE"))
		classABOutputEndpoints = append(classABOutputEndpoints, endpoint(outputNPNs[index], "COLLECTOR"), endpoint(outputPNPs[index], "COLLECTOR"))
		classABPowerEndpoints = append(classABPowerEndpoints, passiveEndpoint("pnp_emitter"+suffix, "B"))
		classABNegativeEndpoints = append(classABNegativeEndpoints, passiveEndpoint("npn_emitter"+suffix, "B"))
	}
	connections := []RealizationConnection{
		semanticNet("driver_input", "analog_signal", driverInputEndpoints...),
		semanticNet("driver_reference", "bias", midpointEndpoints...),
		semanticNet("base_drive", "analog_signal", baseDriveEndpoints...),
		semanticNet("npn_driver_base", "analog_signal", endpoint(upperBiasTracker, "BASE"), endpoint(upperBiasTracker, "COLLECTOR"), passiveEndpoint("upper_bias_feed", "B"), passiveEndpoint("npn_base_stop", "B"), passiveEndpoint("npn_driver_base_emitter", "A"), endpoint(npnDriver, "BASE")),
		semanticNet("npn_driver_output", "analog_signal", endpoint(npnDriver, "COLLECTOR"), passiveEndpoint("npn_output_base_stop", "A")),
		semanticNet("pnp_base", "analog_signal", pnpBaseEndpoints...),
		semanticNet("pnp_driver_base", "analog_signal", endpoint(lowerBiasTracker, "BASE"), endpoint(lowerBiasTracker, "COLLECTOR"), passiveEndpoint("lower_bias_feed", "A"), passiveEndpoint("pnp_base_stop", "B"), passiveEndpoint("pnp_driver_base_emitter", "A"), endpoint(pnpDriver, "BASE")),
		semanticNet("pnp_driver_output", "analog_signal", endpoint(pnpDriver, "COLLECTOR"), passiveEndpoint("pnp_output_base_stop", "A")),
		semanticNet("npn_base", "analog_signal", npnBaseEndpoints...),
		semanticNet("npn_driver_emitter", "analog_signal", endpoint(npnDriver, "EMITTER"), passiveEndpoint("npn_driver_emitter", "A"), passiveEndpoint("npn_driver_base_emitter", "B")),
		semanticNet("pnp_driver_emitter", "analog_signal", endpoint(pnpDriver, "EMITTER"), passiveEndpoint("pnp_driver_emitter", "A"), passiveEndpoint("pnp_driver_base_emitter", "B")),
		semanticNet("class_ab_output", "analog_output", classABOutputEndpoints...),
		semanticNet("class_ab_power", "power", classABPowerEndpoints...),
	}
	for index := 0; index < parallelOutputCount; index++ {
		suffix := fmt.Sprintf("_%d", index+1)
		npnEmitterEndpoints := []RealizationEndpoint{endpoint(outputNPNs[index], "EMITTER"), passiveEndpoint("npn_emitter"+suffix, "A")}
		pnpEmitterEndpoints := []RealizationEndpoint{endpoint(outputPNPs[index], "EMITTER"), passiveEndpoint("pnp_emitter"+suffix, "A")}
		if index == 0 {
			npnEmitterEndpoints = append(npnEmitterEndpoints, passiveEndpoint("npn_output_base_emitter", "B"))
			pnpEmitterEndpoints = append(pnpEmitterEndpoints, passiveEndpoint("pnp_output_base_emitter", "B"))
		}
		connections = append(connections,
			semanticNet("npn_emitter_node"+suffix, "power_signal", npnEmitterEndpoints...),
			semanticNet("pnp_emitter_node"+suffix, "power_signal", pnpEmitterEndpoints...),
		)
	}
	if gain > 1 {
		connections = append(connections,
			semanticNet("class_ab_feedback", "feedback", endpoint(opamp, "IN_MINUS"), passiveEndpoint("feedback_upper", "B"), passiveEndpoint("feedback_lower", "A"), passiveEndpoint("feedback_compensation", "A")),
		)
		for index := range connections {
			switch connections[index].ID {
			case "class_ab_output":
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint("feedback_upper", "A"))
			case "base_drive":
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint("feedback_compensation", "B"))
			case "driver_reference":
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint("feedback_lower", "B"))
			}
		}
	} else {
		for index := range connections {
			if connections[index].ID == "class_ab_output" {
				connections[index].Endpoints = append(connections[index].Endpoints, endpoint(opamp, "IN_MINUS"))
			}
		}
	}
	if hasNegativePower {
		connections = append(connections, semanticNet("class_ab_negative_power", "power", classABNegativeEndpoints...))
	} else {
		classABNegativeEndpoints = append(classABNegativeEndpoints, passiveEndpoint("midpoint_bypass", "B"))
		connections = append(connections, semanticNet("class_ab_reference", "reference", classABNegativeEndpoints...))
	}
	calculation, issues := EvaluateRatings("class_ab_output_ratings", []RatingRequirement{
		{Kind: "npn_voltage", Required: supply, Rated: recordRatingOrZero(npn.record, "collector_emitter_voltage", "V"), DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: npn.evidence},
		{Kind: "npn_current", Required: devicePeakCurrent, Rated: recordRatingOrZero(npn.record, "collector_current", "A"), DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: npn.evidence},
		{Kind: "pnp_voltage", Required: supply, Rated: recordRatingOrZero(pnp.record, "collector_emitter_voltage", "V"), DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: pnp.evidence},
		{Kind: "pnp_current", Required: devicePeakCurrent, Rated: recordRatingOrZero(pnp.record, "collector_current", "A"), DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: pnp.evidence},
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("Class-AB output rating solution failed: %s", issues[0].Message)
	}
	deviceDissipation := stageDissipation / float64(parallelOutputCount)
	copy := *selectionThermalRequirement
	copy.DissipationW = deviceDissipation
	deviceThermalRequirement := &copy
	npnThermal, err := thermalMarginCalculation("class_ab_npn_thermal", npn.record, deviceDissipation, deviceThermalRequirement)
	if err != nil {
		return nil, err
	}
	pnpThermal, err := thermalMarginCalculation("class_ab_pnp_thermal", pnp.record, deviceDissipation, deviceThermalRequirement)
	if err != nil {
		return nil, err
	}
	bankCalculation, err := ObservedCalculation("class_ab_parallel_output_bank",
		NamedQuantity{Name: "devices_per_polarity", Value: float64(parallelOutputCount), Unit: "count"},
		NamedQuantity{Name: "peak_current_per_device", Value: devicePeakCurrent, Unit: "A"},
		NamedQuantity{Name: "dissipation_per_device", Value: deviceDissipation, Unit: "W"},
	)
	if err != nil {
		return nil, err
	}
	calculations := []CalculationEvidence{calculation, bankCalculation, npnThermal, pnpThermal}
	if biasFeedCalculation != nil {
		calculations = append(calculations, *biasFeedCalculation)
	}
	if gainCalculation != nil {
		calculations = append(calculations, *gainCalculation)
	}
	if compensationCalculation != nil {
		calculations = append(calculations, *compensationCalculation)
	}
	return provider.expansion(request, "ground_referenced_common_emitter_class_ab", parts, bindings, connections, calculations, 0)
}

func (provider *CatalogProvider) expandClassAAmplification(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	gain, _, ok := firstNumericConstraint(request.Constraints, "voltage_gain")
	if !ok || gain <= 1 {
		return nil, fmt.Errorf("Class-A amplification requires a gain greater than one")
	}
	quiescent, currentTolerance, ok := firstNumericConstraint(request.Constraints, "quiescent_current")
	if !ok || quiescent <= 0 {
		return nil, fmt.Errorf("Class-A amplification requires a positive quiescent-current target")
	}
	supplyMinimum, supplyMaximum, supplyRangeOK := roleVoltageRange(request.Ports, "power")
	if !supplyRangeOK {
		supplyMaximum = maximumPortVoltage(request.Ports)
		supplyMinimum = supplyMaximum
	}
	designSupply := .5 * (supplyMinimum + supplyMaximum)
	activeBudget := quiescent * (1 - currentTolerance/100)
	if activeBudget <= 0 {
		return nil, fmt.Errorf("Class-A quiescent-current tolerance leaves no positive active-stage budget")
	}
	minimumLoad, maximumLoad, hasLoadRange := numericConstraintBounds(request.Constraints, "load_impedance")
	if !hasLoadRange || minimumLoad <= 0 {
		minimumLoad, maximumLoad = 10_000, 10_000
	}
	peakSwing := 0.0
	if swing, _, hasSwing := firstNumericConstraint(request.Constraints, "output_swing"); hasSwing && swing > 0 {
		peakSwing = swing / 2
	}
	emitterBiasVoltage := designSupply / 2
	if peakSwing > 0 && emitterBiasVoltage <= peakSwing+1 {
		return nil, fmt.Errorf("Class-A output swing leaves no bounded emitter-follower bias headroom")
	}
	bufferCurrent := activeBudget * .25
	if peakSwing > 0 {
		required := peakSwing / minimumLoad / (1 - peakSwing/emitterBiasVoltage)
		bufferCurrent = math.Max(bufferCurrent, required*2)
	}
	gainStageCurrent := activeBudget - bufferCurrent
	if gainStageCurrent <= activeBudget*.25 || bufferCurrent <= 0 {
		return nil, fmt.Errorf("Class-A current, load, and swing targets leave no bounded two-stage bias allocation")
	}
	bufferEmitterIdeal := emitterBiasVoltage / bufferCurrent
	bufferEmitterCandidates, bufferIssues := PreferredValueCandidates(bufferEmitterIdeal, SeriesE96, bufferEmitterIdeal*.8, bufferEmitterIdeal*1.2, DefaultMaxValueCandidates)
	if len(bufferIssues) != 0 || len(bufferEmitterCandidates) == 0 {
		return nil, fmt.Errorf("Class-A output-buffer bias solution failed")
	}
	bufferEmitter := bufferEmitterCandidates[0]
	bufferCurrent = emitterBiasVoltage / bufferEmitter
	gainStageCurrent = activeBudget - bufferCurrent
	bufferBaseVoltage := emitterBiasVoltage + .65
	collectorIdeal := (designSupply - bufferBaseVoltage) / gainStageCurrent
	if collectorIdeal <= 0 {
		return nil, fmt.Errorf("Class-A voltage-gain stage has no positive collector resistance")
	}

	transistorDissipation := math.Max(gainStageCurrent*math.Max(bufferBaseVoltage-.65, .1), bufferCurrent*math.Max(supplyMaximum-emitterBiasVoltage, .1))
	thermalRequirement := thermalRequirementFromConstraints(request.Constraints, transistorDissipation)
	transistor, err := provider.selectComponentWithThermal(ctx, "bjt", "NPN audio", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supplyMaximum / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(math.Max(gainStageCurrent, bufferCurrent) / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "power_dissipation", Value: numericString(transistorDissipation / catalogRatingDeratingFactor), Unit: "W"},
	}, true, thermalRequirement)
	if err != nil {
		return nil, err
	}
	forwardBeta, betaOK := catalogSimulationParameter(transistor.record, "forward_beta")
	if !betaOK || forwardBeta <= 0 {
		return nil, fmt.Errorf("selected Class-A transistor lacks reviewed forward-beta evidence")
	}
	currentGain := forwardBeta / (forwardBeta + 1)
	loadedEmitterMinimum := bufferEmitter * minimumLoad / (bufferEmitter + minimumLoad)
	loadedEmitterMaximum := bufferEmitter * maximumLoad / (bufferEmitter + maximumLoad)
	bufferIntrinsic := .02585 / bufferCurrent
	bufferInputMinimum := (forwardBeta + 1) * (loadedEmitterMinimum + bufferIntrinsic)
	bufferInputMaximum := (forwardBeta + 1) * (loadedEmitterMaximum + bufferIntrinsic)
	effectiveCollectorMinimum := collectorIdeal * bufferInputMinimum / (collectorIdeal + bufferInputMinimum)
	effectiveCollectorMaximum := collectorIdeal * bufferInputMaximum / (collectorIdeal + bufferInputMaximum)
	bufferGainMinimum := loadedEmitterMinimum / (loadedEmitterMinimum + bufferIntrinsic)
	bufferGainMaximum := loadedEmitterMaximum / (loadedEmitterMaximum + bufferIntrinsic)
	designEffectiveCollector := math.Sqrt(effectiveCollectorMinimum * effectiveCollectorMaximum)
	designBufferGain := math.Sqrt(bufferGainMinimum * bufferGainMaximum)
	intrinsicEmitterResistance := .02585 / gainStageCurrent
	emitterIdeal := currentGain*designEffectiveCollector*designBufferGain/gain - intrinsicEmitterResistance
	if emitterIdeal <= 0 {
		return nil, fmt.Errorf("Class-A gain target leaves no positive emitter-degeneration resistance")
	}
	emitterCandidates, emitterIssues := PreferredValueCandidates(emitterIdeal, SeriesE96, emitterIdeal*.7, emitterIdeal*1.3, DefaultMaxValueCandidates)
	collectorCandidates, collectorIssues := PreferredValueCandidates(collectorIdeal, SeriesE96, collectorIdeal*.8, collectorIdeal*1.2, DefaultMaxValueCandidates)
	if len(emitterIssues) != 0 || len(collectorIssues) != 0 || len(emitterCandidates) == 0 || len(collectorCandidates) == 0 {
		return nil, fmt.Errorf("Class-A preferred-value gain solution failed")
	}
	emitter, collector := emitterCandidates[0], collectorCandidates[0]

	biasFeedResistance := 1000.0
	baseTarget := gainStageCurrent*emitter + .65
	biasRegulatorTarget := baseTarget + gainStageCurrent/forwardBeta*biasFeedResistance
	biasRegulator, err := provider.selectComponent(ctx, "regulator", "adjustable", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(supplyMaximum), Unit: "V"},
		{Kind: "output_current", Value: numericString(gainStageCurrent / forwardBeta), Unit: "A"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("Class-A regulated bias selection failed: %w", err)
	}
	biasReference, referenceOK := recordValue(biasRegulator.record, "reference_voltage", "V")
	if !referenceOK || biasRegulatorTarget <= biasReference {
		return nil, fmt.Errorf("selected Class-A bias regulator cannot program the required base voltage")
	}
	biasHeadroom, headroomOK := catalogSimulationParameter(biasRegulator.record, "min_headroom_v")
	if !headroomOK || supplyMinimum-biasRegulatorTarget < biasHeadroom {
		return nil, fmt.Errorf("selected Class-A bias regulator lacks input headroom across the supply range")
	}
	biasLower, biasCalculationReference, biasCalculationTarget := 10_000.0, biasReference, biasRegulatorTarget
	if !recordHasFunction(biasRegulator.record, "GND") {
		adjustmentCurrent, adjustmentOK := catalogSimulationParameter(biasRegulator.record, "adjustment_pin_current_a")
		if !adjustmentOK || adjustmentCurrent < 0 {
			return nil, fmt.Errorf("selected floating Class-A bias regulator lacks adjustment-current evidence")
		}
		biasCalculationReference += adjustmentCurrent * biasLower
		biasCalculationTarget += adjustmentCurrent * biasLower
	}
	biasCalculation, biasIssues := SolveDivider(DividerRequest{
		ID: "class_a_bias", Mode: DividerFeedback, SourceVoltageV: biasCalculationReference, SourceTolerancePercent: .5,
		TargetVoltageV: biasCalculationTarget, TargetTolerancePercent: math.Max(currentTolerance, 5), LowerResistanceOhm: biasLower,
		LowerTolerancePercent: .1, UpperTolerancePercent: .1, UpperSeries: SeriesE96,
	})
	if len(biasIssues) != 0 {
		return nil, fmt.Errorf("Class-A bias solution failed: %s", biasIssues[0].Message)
	}
	biasUpper, _ := calculationSelectedValue(biasCalculation, "upper_resistance")
	biasReferenceResistance, biasProgramResistance := biasUpper, biasLower
	if !recordHasFunction(biasRegulator.record, "GND") {
		biasReferenceResistance, biasProgramResistance = biasLower, biasUpper
	}

	gainDevice, outputBuffer := transistor, transistor
	gainDevice.selected.InstanceID, gainDevice.usage = "class_a_gain_device", "class_a_voltage_gain"
	outputBuffer.selected.InstanceID, outputBuffer.usage = "class_a_output_buffer", "class_a_emitter_follower"
	biasRegulator.selected.InstanceID, biasRegulator.usage = "class_a_bias_regulator", "regulated_base_bias"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{gainDevice, outputBuffer, biasRegulator}, []passivePart{
		{"input_coupling", "capacitor", "ac_coupling", "470u"}, {"output_coupling", "capacitor", "ac_coupling", "220u"},
		{"collector_resistor", "resistor", "collector_load", engineeringValue(collector, "Ohm")},
		{"emitter_resistor", "resistor", "gain_degeneration", engineeringValue(emitter, "Ohm")},
		{"buffer_emitter_resistor", "resistor", "class_a_output_bias", engineeringValue(bufferEmitter, "Ohm")},
		{"bias_reference_resistor", "resistor", "base_bias_feedback", engineeringValue(biasReferenceResistance, "Ohm")},
		{"bias_program_resistor", "resistor", "base_bias_feedback", engineeringValue(biasProgramResistance, "Ohm")},
		{"bias_feed_resistor", "resistor", "base_bias_isolation", engineeringValue(biasFeedResistance, "Ohm")},
		{"bias_input_bypass", "capacitor", "bias_supply_decoupling", "1u"}, {"bias_output_bypass", "capacitor", "bias_reference_decoupling", "1u"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, gainDevice.selected.InstanceID, map[string]string{"input": "BASE", "output": "COLLECTOR", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		switch bindings[index].Role {
		case "input":
			bindings[index].Instance, bindings[index].Function = "input_coupling", "A"
		case "output":
			bindings[index].Instance, bindings[index].Function = "output_coupling", "B"
		case "power":
			bindings[index].Instance, bindings[index].Function = "collector_resistor", "A"
		case "reference":
			bindings[index].Instance, bindings[index].Function = "emitter_resistor", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("class_a_base", "analog_signal", passiveEndpoint("input_coupling", "B"), endpoint(gainDevice, "BASE"), passiveEndpoint("bias_feed_resistor", "B")),
		semanticNet("class_a_bias_output", "analog_signal", endpoint(biasRegulator, "VOUT"), passiveEndpoint("bias_reference_resistor", "A"), passiveEndpoint("bias_output_bypass", "A"), passiveEndpoint("bias_feed_resistor", "A")),
		semanticNet("class_a_bias_feedback", "analog_signal", endpoint(biasRegulator, "ADJ"), passiveEndpoint("bias_reference_resistor", "B"), passiveEndpoint("bias_program_resistor", "A")),
		semanticNet("class_a_driver", "analog_signal", endpoint(gainDevice, "COLLECTOR"), passiveEndpoint("collector_resistor", "B"), endpoint(outputBuffer, "BASE")),
		semanticNet("class_a_gain_emitter", "analog_signal", endpoint(gainDevice, "EMITTER"), passiveEndpoint("emitter_resistor", "A")),
		semanticNet("class_a_output", "analog_signal", endpoint(outputBuffer, "EMITTER"), passiveEndpoint("buffer_emitter_resistor", "A"), passiveEndpoint("output_coupling", "A")),
		semanticNet("class_a_power", "power", passiveEndpoint("collector_resistor", "A"), endpoint(outputBuffer, "COLLECTOR"), endpoint(biasRegulator, "VIN"), passiveEndpoint("bias_input_bypass", "A")),
		semanticNet("class_a_reference", "reference", passiveEndpoint("emitter_resistor", "B"), passiveEndpoint("buffer_emitter_resistor", "B"), passiveEndpoint("bias_program_resistor", "B"), passiveEndpoint("bias_input_bypass", "B"), passiveEndpoint("bias_output_bypass", "B")),
	}
	if recordHasFunction(biasRegulator.record, "GND") {
		connections[len(connections)-1].Endpoints = append(connections[len(connections)-1].Endpoints, endpoint(biasRegulator, "GND"))
	}
	if recordHasFunction(biasRegulator.record, "EN") {
		connections[len(connections)-2].Endpoints = append(connections[len(connections)-2].Endpoints, endpoint(biasRegulator, "EN"))
	}
	gainThermal, err := thermalMarginCalculation("class_a_gain_thermal", gainDevice.record, gainStageCurrent*math.Max(bufferBaseVoltage-gainStageCurrent*emitter, .1), thermalRequirement)
	if err != nil {
		return nil, err
	}
	bufferThermal, err := thermalMarginCalculation("class_a_buffer_thermal", outputBuffer.record, bufferCurrent*math.Max(supplyMaximum-emitterBiasVoltage, .1), thermalRequirement)
	if err != nil {
		return nil, err
	}
	repairs := []RealizationRepairVariable{
		{ID: "class_a_collector_resistance", Kind: "gain", Instance: "collector_resistor", Value: collector, AllowedValues: preferredRepairValues(collector), Unit: "Ohm", Effects: []RealizationRepairEffect{{Analysis: "ac_sweep", Metric: "voltage_gain", Direction: "metric_increases"}, {Analysis: "transient", Metric: "output_swing", Direction: "metric_increases"}}},
		{ID: "class_a_emitter_resistance", Kind: "gain", Instance: "emitter_resistor", Value: emitter, AllowedValues: preferredRepairValues(emitter), Unit: "Ohm", Effects: []RealizationRepairEffect{{Analysis: "ac_sweep", Metric: "voltage_gain", Direction: "metric_decreases"}, {Analysis: "distortion", Metric: "total_harmonic_distortion", Direction: "metric_decreases"}}},
		{ID: "class_a_buffer_bias_resistance", Kind: "bias", Instance: "buffer_emitter_resistor", Value: bufferEmitter, AllowedValues: preferredRepairValues(bufferEmitter), Unit: "Ohm", Effects: []RealizationRepairEffect{{Analysis: "dc_operating_point", Metric: "quiescent_current", Direction: "metric_decreases"}, {Analysis: "transient", Metric: "output_swing", Direction: "metric_decreases"}, {Analysis: "thermal", Metric: "junction_temperature", Direction: "metric_decreases"}}},
	}
	return provider.expansionWithRepairs(request, "buffered_resistively_degenerated_class_a", parts, bindings, connections, []CalculationEvidence{biasCalculation, gainThermal, bufferThermal}, repairs, 0)
}

func (provider *CatalogProvider) expandResistiveClassAAmplification(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	gain, tolerance, ok := firstNumericConstraint(request.Constraints, "voltage_gain")
	if !ok || gain <= 1 {
		return nil, fmt.Errorf("Class-A amplification requires a gain greater than one")
	}
	quiescent, currentTolerance, ok := firstNumericConstraint(request.Constraints, "quiescent_current")
	if !ok || quiescent <= 0 {
		return nil, fmt.Errorf("Class-A amplification requires a positive quiescent-current target")
	}
	supplyMinimum, supplyMaximum, supplyRangeOK := roleVoltageRange(request.Ports, "power")
	if !supplyRangeOK {
		supplyMaximum = maximumPortVoltage(request.Ports)
		supplyMinimum = supplyMaximum
	}
	designSupply := .5 * (supplyMinimum + supplyMaximum)
	// A circuit-level quiescent-current band also has to supply bias and
	// protection support. Seed the active stage at the declared lower edge so
	// those catalog-backed consumers can fit without hiding their current.
	stageQuiescent := quiescent * (1 - currentTolerance/100)
	if stageQuiescent <= 0 {
		return nil, fmt.Errorf("Class-A quiescent-current tolerance leaves no positive stage current")
	}
	collectorIdeal := designSupply / (2 * stageQuiescent)
	minimumLoad, _, hasLoadRange := numericConstraintBounds(request.Constraints, "load_impedance")
	if swing, _, hasSwing := firstNumericConstraint(request.Constraints, "output_swing"); hasSwing && swing > 0 {
		minimumCollector := swing / (2 * stageQuiescent)
		collectorIdeal = math.Min(collectorIdeal, minimumCollector*1.01)
	}
	effectiveCollector := collectorIdeal
	if hasLoadRange {
		minimumEffective := collectorIdeal * minimumLoad / (collectorIdeal + minimumLoad)
		effectiveCollector = minimumEffective
	}
	transistorDissipation := stageQuiescent * math.Max(supplyMaximum-stageQuiescent*collectorIdeal-.65, .1)
	thermalRequirement := thermalRequirementFromConstraints(request.Constraints, transistorDissipation)
	transistor, err := provider.selectComponentWithThermal(ctx, "bjt", "NPN audio", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supplyMaximum / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(stageQuiescent / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "power_dissipation", Value: numericString(transistorDissipation / catalogRatingDeratingFactor), Unit: "W"},
	}, true, thermalRequirement)
	if err != nil {
		return nil, err
	}
	forwardBeta, betaOK := catalogSimulationParameter(transistor.record, "forward_beta")
	if !betaOK || forwardBeta <= 0 {
		return nil, fmt.Errorf("selected Class-A transistor lacks reviewed forward-beta evidence")
	}
	currentGain := forwardBeta / (forwardBeta + 1)
	if maximumDistortion, _, hasDistortion := firstNumericConstraint(request.Constraints, "total_harmonic_distortion"); hasDistortion && maximumDistortion > 0 {
		peakSwing := 0.0
		if swing, _, hasSwing := firstNumericConstraint(request.Constraints, "output_swing"); hasSwing && swing > 0 {
			peakSwing = swing / 2
		}
		maximumCollector := math.Inf(1)
		if peakSwing > 0 {
			maximumCollector = (supplyMinimum - peakSwing - 1) / stageQuiescent
		}
		for iteration := 0; iteration < 128 && peakSwing > 0 && collectorIdeal < maximumCollector; iteration++ {
			effective := collectorIdeal
			if hasLoadRange {
				minimumEffective := collectorIdeal * minimumLoad / (collectorIdeal + minimumLoad)
				effective = minimumEffective
			}
			emitterEstimate := math.Max(currentGain*effective/gain-0.02585/stageQuiescent, 1e-6)
			modulation := peakSwing / (stageQuiescent * collectorIdeal)
			predictedPercent := 100 * 0.02585 * modulation / (4 * (stageQuiescent*emitterEstimate + 0.02585))
			if predictedPercent <= maximumDistortion*.75 {
				break
			}
			collectorIdeal *= 1.05
		}
		if collectorIdeal > maximumCollector {
			return nil, fmt.Errorf("Class-A distortion and swing targets exceed the bounded resistive-load headroom")
		}
	}
	effectiveCollector = collectorIdeal
	if hasLoadRange {
		minimumEffective := collectorIdeal * minimumLoad / (collectorIdeal + minimumLoad)
		effectiveCollector = minimumEffective
	}
	intrinsicEmitterResistance := 0.02585 / stageQuiescent
	emitterIdeal := currentGain*effectiveCollector/gain - intrinsicEmitterResistance
	// The public gain is measured at the graph ingress, so include the bounded
	// source-coupling attenuation caused by the transistor input resistance and
	// the regulated bias feed. This remains an analytic seed; simulation is the
	// authority for promotion.
	const inputCouplingCapacitanceF = 470e-6
	const gainObservationFrequencyHz = 10.0
	biasFeedResistance := 1000.0
	for iteration := 0; iteration < 8 && emitterIdeal > 0; iteration++ {
		collectorCurrent := stageQuiescent * currentGain
		baseInputResistance := forwardBeta*0.02585/collectorCurrent + (forwardBeta+1)*emitterIdeal
		inputResistance := biasFeedResistance * baseInputResistance / (biasFeedResistance + baseInputResistance)
		capacitiveReactance := 1 / (2 * math.Pi * gainObservationFrequencyHz * inputCouplingCapacitanceF)
		attenuation := inputResistance / math.Hypot(inputResistance, capacitiveReactance)
		emitterIdeal = currentGain*effectiveCollector*attenuation/gain - intrinsicEmitterResistance
	}
	if emitterIdeal <= 0 {
		return nil, fmt.Errorf("Class-A gain target leaves no positive emitter-degeneration resistance")
	}
	emitterCandidates, candidateIssues := PreferredValueCandidates(emitterIdeal, SeriesE96, emitterIdeal*0.5, emitterIdeal*2, DefaultMaxValueCandidates)
	if len(candidateIssues) != 0 {
		return nil, fmt.Errorf("Class-A emitter value solution failed: %s", candidateIssues[0].Message)
	}
	var gainCalculation CalculationEvidence
	var solveIssues []reports.Issue
	emitter := 0.0
	for _, candidate := range emitterCandidates {
		gainCalculation, solveIssues = SolveDivider(DividerRequest{
			ID: "class_a_gain", Mode: DividerFeedback, SourceVoltageV: 1, TargetVoltageV: collectorIdeal/candidate + 1,
			TargetTolerancePercent: tolerance, LowerResistanceOhm: candidate, LowerTolerancePercent: 1,
			UpperTolerancePercent: 1, UpperSeries: SeriesE96,
		})
		if len(solveIssues) == 0 {
			emitter = candidate
			break
		}
	}
	if len(solveIssues) != 0 || emitter == 0 {
		return nil, fmt.Errorf("Class-A gain solution failed: %s", solveIssues[0].Message)
	}
	collector, _ := calculationSelectedValue(gainCalculation, "upper_resistance")
	baseTarget := stageQuiescent*emitter + 0.65
	biasRegulatorTarget := baseTarget + stageQuiescent/forwardBeta*biasFeedResistance
	biasRegulator, err := provider.selectComponent(ctx, "regulator", "adjustable", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(supplyMaximum), Unit: "V"},
		{Kind: "output_current", Value: numericString(stageQuiescent / forwardBeta), Unit: "A"},
	}, true)
	if err != nil {
		return nil, fmt.Errorf("Class-A regulated bias selection failed: %w", err)
	}
	biasReference, referenceOK := recordValue(biasRegulator.record, "reference_voltage", "V")
	if !referenceOK || biasRegulatorTarget <= biasReference {
		return nil, fmt.Errorf("selected Class-A bias regulator cannot program the required base voltage")
	}
	biasHeadroom, headroomOK := catalogSimulationParameter(biasRegulator.record, "min_headroom_v")
	if !headroomOK || supplyMinimum-biasRegulatorTarget < biasHeadroom {
		return nil, fmt.Errorf("selected Class-A bias regulator lacks input headroom across the supply range")
	}
	biasLower := 10_000.0
	biasCalculationReference := biasReference
	biasCalculationTarget := biasRegulatorTarget
	if !recordHasFunction(biasRegulator.record, "GND") {
		adjustmentCurrent, adjustmentOK := catalogSimulationParameter(biasRegulator.record, "adjustment_pin_current_a")
		if !adjustmentOK || adjustmentCurrent < 0 {
			return nil, fmt.Errorf("selected floating Class-A bias regulator lacks adjustment-current evidence")
		}
		biasCalculationReference += adjustmentCurrent * biasLower
		biasCalculationTarget += adjustmentCurrent * biasLower
	}
	biasCalculation, biasIssues := SolveDivider(DividerRequest{
		ID: "class_a_bias", Mode: DividerFeedback, SourceVoltageV: biasCalculationReference, SourceTolerancePercent: 0.5,
		TargetVoltageV: biasCalculationTarget, TargetTolerancePercent: math.Max(currentTolerance, 5),
		LowerResistanceOhm: biasLower, LowerTolerancePercent: 0.1, UpperTolerancePercent: 0.1, UpperSeries: SeriesE96,
	})
	if len(biasIssues) != 0 {
		return nil, fmt.Errorf("Class-A bias solution failed: %s", biasIssues[0].Message)
	}
	biasUpper, _ := calculationSelectedValue(biasCalculation, "upper_resistance")
	biasReferenceResistance, biasProgramResistance := biasUpper, biasLower
	if !recordHasFunction(biasRegulator.record, "GND") {
		// Three-terminal floating regulators hold VOUT-ADJ at their reference;
		// their physical programming ratio is the inverse of a ground-referenced
		// adjustable regulator's divider.
		biasReferenceResistance, biasProgramResistance = biasLower, biasUpper
	}
	transistor.selected.InstanceID, transistor.usage = "class_a_device", "class_a_gain_stage"
	biasRegulator.selected.InstanceID, biasRegulator.usage = "class_a_bias_regulator", "regulated_base_bias"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{transistor, biasRegulator}, []passivePart{
		{"input_coupling", "capacitor", "ac_coupling", "470u"}, {"output_coupling", "capacitor", "ac_coupling", "220u"},
		{"collector_resistor", "resistor", "collector_load", engineeringValue(collector, "Ohm")},
		{"emitter_resistor", "resistor", "bias_stabilization", engineeringValue(emitter, "Ohm")},
		{"bias_reference_resistor", "resistor", "base_bias_feedback", engineeringValue(biasReferenceResistance, "Ohm")},
		{"bias_program_resistor", "resistor", "base_bias_feedback", engineeringValue(biasProgramResistance, "Ohm")},
		{"bias_feed_resistor", "resistor", "base_bias_isolation", engineeringValue(biasFeedResistance, "Ohm")},
		{"bias_input_bypass", "capacitor", "bias_supply_decoupling", "1u"},
		{"bias_output_bypass", "capacitor", "bias_reference_decoupling", "1u"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, transistor.selected.InstanceID, map[string]string{"input": "BASE", "output": "COLLECTOR", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		switch bindings[index].Role {
		case "input":
			bindings[index].Instance, bindings[index].Function = "input_coupling", "A"
		case "output":
			bindings[index].Instance, bindings[index].Function = "output_coupling", "B"
		case "power":
			bindings[index].Instance, bindings[index].Function = "collector_resistor", "A"
		case "reference":
			bindings[index].Instance, bindings[index].Function = "emitter_resistor", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("class_a_base", "analog_signal", passiveEndpoint("input_coupling", "B"), endpoint(transistor, "BASE"), passiveEndpoint("bias_feed_resistor", "B")),
		semanticNet("class_a_bias_output", "analog_signal", endpoint(biasRegulator, "VOUT"), passiveEndpoint("bias_reference_resistor", "A"), passiveEndpoint("bias_output_bypass", "A"), passiveEndpoint("bias_feed_resistor", "A")),
		semanticNet("class_a_bias_feedback", "analog_signal", endpoint(biasRegulator, "ADJ"), passiveEndpoint("bias_reference_resistor", "B"), passiveEndpoint("bias_program_resistor", "A")),
		semanticNet("class_a_collector", "analog_signal", endpoint(transistor, "COLLECTOR"), passiveEndpoint("collector_resistor", "B"), passiveEndpoint("output_coupling", "A")),
		semanticNet("class_a_emitter", "analog_signal", endpoint(transistor, "EMITTER"), passiveEndpoint("emitter_resistor", "A")),
		semanticNet("class_a_power", "power", passiveEndpoint("collector_resistor", "A"), endpoint(biasRegulator, "VIN"), passiveEndpoint("bias_input_bypass", "A")),
		semanticNet("class_a_reference", "reference", passiveEndpoint("emitter_resistor", "B"), passiveEndpoint("bias_program_resistor", "B"), passiveEndpoint("bias_input_bypass", "B"), passiveEndpoint("bias_output_bypass", "B")),
	}
	if recordHasFunction(biasRegulator.record, "GND") {
		connections[len(connections)-1].Endpoints = append(connections[len(connections)-1].Endpoints, endpoint(biasRegulator, "GND"))
	}
	if recordHasFunction(biasRegulator.record, "EN") {
		connections[len(connections)-2].Endpoints = append(connections[len(connections)-2].Endpoints, endpoint(biasRegulator, "EN"))
	}
	thermal, err := thermalMarginCalculation("class_a_thermal", transistor.record, transistorDissipation, thermalRequirement)
	if err != nil {
		return nil, err
	}
	repairs := []RealizationRepairVariable{
		{
			ID: "class_a_collector_resistance", Kind: "gain", Instance: "collector_resistor", Value: collector, AllowedValues: preferredRepairValues(collector), Unit: "Ohm",
			Effects: []RealizationRepairEffect{
				{Analysis: "ac_sweep", Metric: "bandwidth", Direction: "metric_decreases"},
				{Analysis: "ac_sweep", Metric: "voltage_gain", Direction: "metric_increases"},
				{Analysis: "distortion", Metric: "total_harmonic_distortion", Direction: "metric_decreases"},
				{Analysis: "startup", Metric: "startup_output_voltage", Direction: "metric_decreases"},
				{Analysis: "thermal", Metric: "junction_temperature", Direction: "metric_decreases"},
				{Analysis: "transient", Metric: "output_swing", Direction: "metric_increases"},
			},
		},
		{
			ID: "class_a_emitter_resistance", Kind: "gain", Instance: "emitter_resistor", Value: emitter, AllowedValues: preferredRepairValues(emitter), Unit: "Ohm",
			Effects: []RealizationRepairEffect{
				{Analysis: "ac_sweep", Metric: "voltage_gain", Direction: "metric_decreases"},
				{Analysis: "dc_operating_point", Metric: "quiescent_current", Direction: "metric_decreases"},
				{Analysis: "distortion", Metric: "total_harmonic_distortion", Direction: "metric_decreases"},
				{Analysis: "thermal", Metric: "junction_temperature", Direction: "metric_decreases"},
				{Analysis: "transient", Metric: "output_swing", Direction: "metric_decreases"},
			},
		},
	}
	return provider.expansionWithRepairs(request, "resistively_degenerated_class_a", parts, bindings, connections, []CalculationEvidence{gainCalculation, biasCalculation, thermal}, repairs, 0)
}

func thermalRequirementFromConstraints(constraints []Constraint, dissipationW float64) *components.ThermalRequirement {
	requirement := &components.ThermalRequirement{
		DissipationW:          dissipationW,
		Reference:             components.ThermalReferenceAmbient,
		ReferenceTemperatureC: 25,
	}
	_, _, hasAmbient := firstNumericConstraint(constraints, "ambient_temperature")
	_, _, hasCase := firstNumericConstraint(constraints, "case_temperature")
	_, _, hasJunction := firstNumericConstraint(constraints, "junction_temperature")
	if !hasAmbient && !hasCase && !hasJunction {
		return nil
	}
	if ambient, _, ok := firstNumericConstraint(constraints, "ambient_temperature"); ok {
		requirement.ReferenceTemperatureC = ambient
	}
	if caseTemperature, _, ok := firstNumericConstraint(constraints, "case_temperature"); ok {
		requirement.Reference = components.ThermalReferenceCase
		requirement.ReferenceTemperatureC = caseTemperature
	}
	if maximumJunction, _, ok := firstNumericConstraint(constraints, "junction_temperature"); ok {
		requirement.MaximumJunctionTemperatureC = maximumJunction
	}
	return requirement
}

func temperatureRequirementFromConstraints(constraints []Constraint) *components.TemperatureRequirement {
	minimum, _, hasMinimum := firstNumericConstraint(constraints, "ambient_temperature_minimum")
	maximum, _, hasMaximum := firstNumericConstraint(constraints, "ambient_temperature")
	if !hasMinimum && !hasMaximum {
		return nil
	}
	requirement := &components.TemperatureRequirement{}
	if hasMinimum {
		requirement.MinimumC = float64Pointer(minimum)
	}
	if hasMaximum {
		requirement.MaximumC = float64Pointer(maximum)
	}
	return requirement
}

func thermalMarginCalculation(id string, record components.ComponentRecord, dissipationW float64, requirement *components.ThermalRequirement) (CalculationEvidence, error) {
	if !finitePositive(dissipationW) || record.PowerSemiconductor == nil || record.PowerSemiconductor.MaxJunctionTemperatureC == nil {
		return CalculationEvidence{}, fmt.Errorf("selected power device lacks operating-dissipation or junction-temperature evidence")
	}
	evidence := record.PowerSemiconductor
	thermalResistance := 0.0
	reference, referenceTemperature := components.ThermalReferenceAmbient, 25.0
	if requirement != nil {
		reference, referenceTemperature = requirement.Reference, requirement.ReferenceTemperatureC
	} else if evidence.JunctionToAmbientCPerW == nil && evidence.JunctionToCaseCPerW != nil {
		// Legacy structural requirements did not expose a thermal boundary.
		// Preserve their recorded calculation without using it to satisfy a
		// simulation-grounded thermal assertion.
		reference = components.ThermalReferenceCase
	}
	switch reference {
	case components.ThermalReferenceAmbient:
		if evidence.JunctionToAmbientCPerW != nil {
			thermalResistance = *evidence.JunctionToAmbientCPerW
		}
	case components.ThermalReferenceCase:
		if evidence.JunctionToCaseCPerW != nil {
			thermalResistance = *evidence.JunctionToCaseCPerW
		}
	default:
		return CalculationEvidence{}, fmt.Errorf("thermal reference must be ambient or case")
	}
	if thermalResistance <= 0 {
		return CalculationEvidence{}, fmt.Errorf("selected power device lacks a complete junction-to-%s thermal path", reference)
	}
	maximumJunction := *evidence.MaxJunctionTemperatureC
	if requirement != nil && requirement.MaximumJunctionTemperatureC > 0 {
		maximumJunction = math.Min(maximumJunction, requirement.MaximumJunctionTemperatureC)
	}
	junctionTemperature := referenceTemperature + dissipationW*thermalResistance
	margin := maximumJunction - junctionTemperature
	if margin < 0 {
		return CalculationEvidence{}, fmt.Errorf("selected power device exceeds its catalog-backed junction-temperature envelope")
	}
	return ObservedCalculation(id,
		NamedQuantity{Name: "thermal_margin", Value: margin, Unit: "degC"},
		NamedQuantity{Name: "junction_temperature", Value: junctionTemperature, Unit: "degC"},
		NamedQuantity{Name: "power_dissipation", Value: dissipationW, Unit: "W"},
		NamedQuantity{Name: reference + "_temperature", Value: referenceTemperature, Unit: "degC"},
	)
}

func recordRatingOrZero(record components.ComponentRecord, kind, unit string) float64 {
	value, _ := recordRatingMaximum(record, kind, unit)
	return value
}

func (provider *CatalogProvider) expandSplitSupply(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	positiveVoltage, positiveTolerance, positiveOK := firstNumericConstraint(request.Constraints, "positive_voltage")
	negativeVoltage, negativeTolerance, negativeOK := firstNumericConstraint(request.Constraints, "negative_voltage")
	if !positiveOK || !negativeOK || positiveVoltage <= 0 || negativeVoltage >= 0 || positiveTolerance <= 0 || negativeTolerance <= 0 {
		return nil, fmt.Errorf("split-supply generation requires positive and negative voltage targets with tolerances")
	}
	inputMinimum, inputMaximum, inputOK := roleVoltageRange(request.Ports, "input")
	if !inputOK {
		return nil, fmt.Errorf("split-supply generation requires a bounded input range")
	}
	positiveCurrent := requiredRoleCurrentA(request.Ports, "positive_output")
	negativeCurrent := requiredRoleCurrentA(request.Ports, "negative_output")
	if positiveCurrent <= 0 || negativeCurrent <= 0 {
		return nil, fmt.Errorf("split-supply generation requires current contracts on both outputs")
	}
	temperature := temperatureRequirementFromConstraints(request.Constraints)
	converter, err := provider.selectComponentWithTemperature(ctx, "isolated_converter", "dual_output", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
		{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "positive_output_current", Value: numericString(positiveCurrent / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "negative_output_current", Value: numericString(negativeCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true, temperature)
	if err != nil {
		return nil, err
	}
	converter.selected.InstanceID, converter.usage = "dual_rail_converter", "split_supply_converter"
	positiveRegulator, err := provider.selectComponentWithTemperature(ctx, "regulator", "positive", []components.RequiredRating{{Kind: "output_current", Value: numericString(positiveCurrent), Unit: "A"}}, true, temperature)
	if err != nil {
		return nil, err
	}
	positiveRegulator.selected.InstanceID, positiveRegulator.usage = "positive_regulator", "positive_post_regulator"
	negativeRegulator, err := provider.selectComponentWithTemperature(ctx, "regulator", "negative", []components.RequiredRating{{Kind: "output_current", Value: numericString(negativeCurrent), Unit: "A"}}, true, temperature)
	if err != nil {
		return nil, err
	}
	negativeRegulator.selected.InstanceID, negativeRegulator.usage = "negative_regulator", "negative_post_regulator"
	positiveFeedback, positiveIssues := SolveDivider(DividerRequest{
		ID: "positive_rail_feedback", Mode: DividerFeedback, SourceVoltageV: 1.25, SourceTolerancePercent: 0.5,
		TargetVoltageV: positiveVoltage, TargetTolerancePercent: positiveTolerance, LowerResistanceOhm: 240,
		LowerTolerancePercent: 0.1, UpperTolerancePercent: 0.1, UpperSeries: SeriesE96,
	})
	if len(positiveIssues) != 0 {
		return nil, fmt.Errorf("positive split-rail feedback solution failed: %s", positiveIssues[0].Message)
	}
	negativeFeedback, negativeIssues := SolveDivider(DividerRequest{
		ID: "negative_rail_feedback", Mode: DividerFeedback, SourceVoltageV: 1.25, SourceTolerancePercent: 0.5,
		TargetVoltageV: math.Abs(negativeVoltage), TargetTolerancePercent: negativeTolerance, LowerResistanceOhm: 240,
		LowerTolerancePercent: 0.1, UpperTolerancePercent: 0.1, UpperSeries: SeriesE96,
	})
	if len(negativeIssues) != 0 {
		return nil, fmt.Errorf("negative split-rail feedback solution failed: %s", negativeIssues[0].Message)
	}
	positiveSet, _ := calculationSelectedValue(positiveFeedback, "upper_resistance")
	negativeSet, _ := calculationSelectedValue(negativeFeedback, "upper_resistance")
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{converter, positiveRegulator, negativeRegulator}, []passivePart{
		{"split_input_bypass", "capacitor", "decoupling", "1u"}, {"raw_positive_bypass", "capacitor", "decoupling", "1u"}, {"raw_negative_bypass", "capacitor", "decoupling", "1u"},
		{"positive_feedback_sense", "resistor", "feedback_divider", "240"}, {"positive_feedback_set", "resistor", "feedback_divider", engineeringValue(positiveSet, "Ohm")},
		{"negative_feedback_sense", "resistor", "feedback_divider", "240"}, {"negative_feedback_set", "resistor", "feedback_divider", engineeringValue(negativeSet, "Ohm")},
		{"positive_output_bypass", "capacitor", "stability", "1u"}, {"negative_output_bypass", "capacitor", "stability", "1u"},
	})
	if err != nil {
		return nil, err
	}
	positiveRated, positiveRatedOK := recordRatingMaximum(converter.record, "positive_output_current", "A")
	negativeRated, negativeRatedOK := recordRatingMaximum(converter.record, "negative_output_current", "A")
	positiveRegRated, positiveRegOK := recordRatingMaximum(positiveRegulator.record, "output_current", "A")
	negativeRegRated, negativeRegOK := recordRatingMaximum(negativeRegulator.record, "output_current", "A")
	if !positiveRatedOK || !negativeRatedOK || !positiveRegOK || !negativeRegOK {
		return nil, fmt.Errorf("split-supply chain lacks normalized output-current evidence")
	}
	ratings, ratingIssues := EvaluateRatings("split_supply_margins", []RatingRequirement{
		{Kind: "positive_converter_current", Required: positiveCurrent, Rated: positiveRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: converter.evidence},
		{Kind: "negative_converter_current", Required: negativeCurrent, Rated: negativeRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: converter.evidence},
		{Kind: "positive_regulator_current", Required: positiveCurrent, Rated: positiveRegRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: positiveRegulator.evidence},
		{Kind: "negative_regulator_current", Required: negativeCurrent, Rated: negativeRegRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: negativeRegulator.evidence},
		{Kind: "input_power", Required: (positiveVoltage*positiveCurrent + math.Abs(negativeVoltage)*negativeCurrent) / 0.87, Rated: inputMinimum * maximumRoleCurrentDemandA(request.Ports, "input"), DeratingFactor: catalogRatingDeratingFactor, Unit: "W", Evidence: converter.evidence},
		{Kind: "positive_headroom", Required: positiveVoltage + 2, Rated: 12, DeratingFactor: 1, Unit: "V", Evidence: converter.evidence},
		{Kind: "negative_headroom", Required: math.Abs(negativeVoltage) + 2, Rated: 12, DeratingFactor: 1, Unit: "V", Evidence: converter.evidence},
	})
	if len(ratingIssues) != 0 {
		return nil, fmt.Errorf("split-supply rating solution failed: %s", ratingIssues[0].Message)
	}
	bindings := bindRoles(request.Ports, converter.selected.InstanceID, map[string]string{"input": "VIN_PLUS", "positive_output": "VOUT_PLUS", "negative_output": "VOUT_MINUS", "reference": "COMMON"})
	for index := range bindings {
		switch bindings[index].Role {
		case "positive_output":
			bindings[index].Instance, bindings[index].Function = positiveRegulator.selected.InstanceID, "VOUT"
		case "negative_output":
			bindings[index].Instance, bindings[index].Function = negativeRegulator.selected.InstanceID, "VOUT"
		}
	}
	bindings = append(bindings, RealizationPortBinding{Role: "input", Lane: "return", Instance: converter.selected.InstanceID, Function: "VIN_MINUS"})
	connections := []RealizationConnection{
		semanticNet("split_input", "power", endpoint(converter, "VIN_PLUS"), passiveEndpoint("split_input_bypass", "A")),
		semanticNet("split_input_return", "reference", endpoint(converter, "VIN_MINUS"), passiveEndpoint("split_input_bypass", "B")),
		semanticNet("raw_positive_rail", "power", endpoint(converter, "VOUT_PLUS"), endpoint(positiveRegulator, "VIN"), passiveEndpoint("raw_positive_bypass", "A")),
		semanticNet("raw_negative_rail", "power", endpoint(converter, "VOUT_MINUS"), endpoint(negativeRegulator, "VIN"), passiveEndpoint("raw_negative_bypass", "A")),
		semanticNet("positive_rail", "power", endpoint(positiveRegulator, "VOUT"), passiveEndpoint("positive_feedback_sense", "A"), passiveEndpoint("positive_output_bypass", "A")),
		semanticNet("positive_feedback", "analog_signal", endpoint(positiveRegulator, "ADJ"), passiveEndpoint("positive_feedback_sense", "B"), passiveEndpoint("positive_feedback_set", "A")),
		semanticNet("negative_rail", "power", endpoint(negativeRegulator, "VOUT"), passiveEndpoint("negative_feedback_sense", "A"), passiveEndpoint("negative_output_bypass", "A")),
		semanticNet("negative_feedback", "analog_signal", endpoint(negativeRegulator, "ADJ"), passiveEndpoint("negative_feedback_sense", "B"), passiveEndpoint("negative_feedback_set", "A")),
		semanticNet("split_reference", "reference", endpoint(converter, "COMMON"), passiveEndpoint("raw_positive_bypass", "B"), passiveEndpoint("raw_negative_bypass", "B"), passiveEndpoint("positive_feedback_set", "B"), passiveEndpoint("negative_feedback_set", "B"), passiveEndpoint("positive_output_bypass", "B"), passiveEndpoint("negative_output_bypass", "B")),
	}
	return provider.expansion(request, "dual_output_converter_with_adjustable_post_regulation", parts, bindings, connections, []CalculationEvidence{positiveFeedback, negativeFeedback, ratings}, 0)
}

func (provider *CatalogProvider) expandGalvanicIsolation(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	isolationRequired, _, ok := firstNumericConstraint(request.Constraints, "isolation_voltage")
	if !ok || isolationRequired <= 0 {
		return nil, fmt.Errorf("galvanic isolation requires an isolation-voltage bound")
	}
	frequency := maximumProtocolFrequency(request.Ports)
	if frequency <= 0 {
		return nil, fmt.Errorf("galvanic isolation requires a bounded bus frequency")
	}
	voltageAMin, voltageAMax, okA := roleVoltageRange(request.Ports, "power_a")
	voltageBMin, voltageBMax, okB := roleVoltageRange(request.Ports, "power_b")
	if !okA || !okB {
		return nil, fmt.Errorf("galvanic isolation requires bounded supplies on both sides")
	}
	isolator, err := provider.selectComponent(ctx, "isolator", "i2c", []components.RequiredRating{
		{Kind: "side_a_supply_voltage", Value: numericString(voltageAMin), Unit: "V"},
		{Kind: "side_a_supply_voltage", Value: numericString(voltageAMax), Unit: "V"},
		{Kind: "side_b_supply_voltage", Value: numericString(voltageBMin), Unit: "V"},
		{Kind: "side_b_supply_voltage", Value: numericString(voltageBMax), Unit: "V"},
		{Kind: "data_rate", Value: numericString(frequency), Unit: "Hz"},
		{Kind: "isolation_voltage", Value: numericString(isolationRequired), Unit: "V"},
	}, true)
	if err != nil {
		return nil, err
	}
	isolator.selected.InstanceID, isolator.usage = "bus_isolator", "bidirectional_i2c_isolation"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{isolator}, []passivePart{
		{"isolation_a_sda_pullup", "resistor", "bus_pullup", "4.7k"}, {"isolation_a_scl_pullup", "resistor", "bus_pullup", "4.7k"},
		{"isolation_b_sda_pullup", "resistor", "bus_pullup", "4.7k"}, {"isolation_b_scl_pullup", "resistor", "bus_pullup", "4.7k"},
		{"isolation_a_bypass", "capacitor", "decoupling", "100n"}, {"isolation_b_bypass", "capacitor", "decoupling", "100n"},
	})
	if err != nil {
		return nil, err
	}
	isolationRated, isolationOK := recordRatingMaximum(isolator.record, "isolation_voltage", "V")
	frequencyRated, frequencyOK := recordRatingMaximum(isolator.record, "data_rate", "Hz")
	if !isolationOK || !frequencyOK {
		return nil, fmt.Errorf("selected isolator lacks normalized isolation or frequency evidence")
	}
	ratings, ratingIssues := EvaluateRatings("digital_isolation_margins", []RatingRequirement{
		{Kind: "isolation_voltage", Required: isolationRequired, Rated: isolationRated, DeratingFactor: 1, Unit: "V", Evidence: isolator.evidence},
		{Kind: "bus_frequency", Required: frequency, Rated: frequencyRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "Hz", Evidence: isolator.evidence},
	})
	if len(ratingIssues) != 0 {
		return nil, fmt.Errorf("galvanic-isolation rating solution failed: %s", ratingIssues[0].Message)
	}
	bindings := bindBusRoles(request.Ports, isolator.selected.InstanceID, map[string]string{
		"side_a_sda": "SDA1", "side_a_scl": "SCL1", "side_b_sda": "SDA2", "side_b_scl": "SCL2",
		"power_a": "VDD1", "power_b": "VDD2", "reference_a": "GND1", "reference_b": "GND2",
	})
	connections := []RealizationConnection{
		semanticNet("isolator_side_a_sda", "open_drain_bus", endpoint(isolator, "SDA1"), passiveEndpoint("isolation_a_sda_pullup", "B")),
		semanticNet("isolator_side_a_scl", "open_drain_bus", endpoint(isolator, "SCL1"), passiveEndpoint("isolation_a_scl_pullup", "B")),
		semanticNet("isolator_side_b_sda", "open_drain_bus", endpoint(isolator, "SDA2"), passiveEndpoint("isolation_b_sda_pullup", "B")),
		semanticNet("isolator_side_b_scl", "open_drain_bus", endpoint(isolator, "SCL2"), passiveEndpoint("isolation_b_scl_pullup", "B")),
		semanticNet("isolator_power_a", "power", endpoint(isolator, "VDD1"), passiveEndpoint("isolation_a_sda_pullup", "A"), passiveEndpoint("isolation_a_scl_pullup", "A"), passiveEndpoint("isolation_a_bypass", "A")),
		semanticNet("isolator_reference_a", "reference", endpoint(isolator, "GND1"), passiveEndpoint("isolation_a_bypass", "B")),
		semanticNet("isolator_power_b", "power", endpoint(isolator, "VDD2"), passiveEndpoint("isolation_b_sda_pullup", "A"), passiveEndpoint("isolation_b_scl_pullup", "A"), passiveEndpoint("isolation_b_bypass", "A")),
		semanticNet("isolator_reference_b", "reference", endpoint(isolator, "GND2"), passiveEndpoint("isolation_b_bypass", "B")),
	}
	return provider.expansion(request, "dual_domain_bidirectional_i2c_isolator", parts, bindings, connections, []CalculationEvidence{ratings}, 0)
}
