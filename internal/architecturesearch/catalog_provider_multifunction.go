package architecturesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

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
		stringConstraint("output_polarity", "equal", "active_low"),
		boolConstraint("inactive_at_power_up", "required"),
		numericConstraint("propagation_delay", "maximum", 10, "us", 0),
	}
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
	legacy.Constraints = []Constraint{
		stringConstraint("load_characteristic", "equal", "inductive"),
		stringConstraint("control_active_state", "equal", "high"),
		boolConstraint("default_off", "required"),
		boolConstraint("inductive_transient_clamp", "required"),
		boolConstraint("control_overvoltage_clamp", "required"),
		numericConstraint("load_voltage", "minimum", voltage, "V", 0),
		numericConstraint("load_current", "minimum", current, "A", tolerance),
	}
	expansions, err := provider.expandLoadSwitch(ctx, legacy)
	if err != nil {
		return nil, err
	}
	for index := range expansions {
		expansions[index].OfferedPorts = remapOfferedCatalogPorts(request, expansions[index].OfferedPorts)
	}
	return expansions, nil
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
	legacy.Constraints = []Constraint{
		boolConstraint("adjustable_output", "required"),
		stringConstraint("set_point_programming", "equal", "passive_feedback"),
		boolConstraint("input_decoupling", "required"),
		boolConstraint("output_decoupling", "required"),
		rangeConstraint("input_voltage", inputMinimum, inputMaximum, "V"),
		numericConstraint("output_voltage", "target", output, "V", tolerance),
		numericConstraint("continuous_output_current", "minimum", current, "A", currentTolerance),
	}
	return provider.expandRegulator(ctx, legacy)
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
		return nil, fmt.Errorf("logic translation requires protocol frequency evidence")
	}
	legacy := cloneProviderRequest(request)
	legacy.Constraints = []Constraint{
		stringConstraint("protocol", "equal", "i2c"),
		stringConstraint("signaling_mode", "equal", "open_drain"),
		stringConstraint("direction", "equal", "bidirectional"),
		boolConstraint("unpowered_backfeed_prevention", "required"),
		numericConstraint("bus_frequency", "minimum", frequency, "Hz", 0),
	}
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
	opamp, err := provider.selectComponent(ctx, "opamp", "single", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplySpan), Unit: "V"}}, true)
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
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
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
	bindings := bindRoles(request.Ports, transistor.selected.InstanceID, map[string]string{"state": "BASE", "output": "EMITTER", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		if bindings[index].Role == "state" {
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
	protector, err := provider.selectComponent(ctx, "protection", "ESD TVS", nil, true)
	if err != nil {
		return nil, err
	}
	protector.selected.InstanceID, protector.usage = "output_clamp", "output_transient_clamp"
	limit, _, hasLimit := firstNumericConstraint(request.Constraints, "overcurrent_limit")
	value := "500mA"
	if hasLimit {
		value = engineeringValue(limit, "A")
	}
	fuse, err := provider.selectComponent(ctx, "fuse", "", nil, false)
	if err != nil {
		return nil, err
	}
	fuse.selected.InstanceID, fuse.usage, fuse.value = "output_fuse", "overcurrent_limit", value
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
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"dc_block", "capacitor", "dc_fault_disconnect", "220u"}})
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
	opamp, err := provider.selectComponent(ctx, "opamp", "single", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplySpan), Unit: "V"}}, true)
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
	hasNegativePower := hasRoleContract(request.Ports, "negative_power")
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
	if !ok || measurementMaximum <= 0 {
		return nil, fmt.Errorf("current sensing requires a bounded positive measurement range")
	}
	targetOutput := measurementMaximum * 0.8
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
	regulator, err := provider.selectComponent(ctx, "regulator", "LM317T", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(commonMode), Unit: "V"},
		{Kind: "output_current", Value: "0.01", Unit: "A"},
	}, true)
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
		"control": "IN_PLUS", "fault": "REF1", "measurement": "OUT", "permit": "OUT", "reference": "GND_A",
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
	supply := maximumPortVoltage(request.Ports)
	transistor, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply), Unit: "V"},
		{Kind: "collector_current", Value: "0.01", Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	transistor.selected.InstanceID, transistor.usage = "mute_driver", "default_active_mute"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{transistor}, []passivePart{
		{"enable_resistor", "resistor", "logic_drive", "10k"},
		{"mute_pullup", "resistor", "startup_mute", "10k"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, transistor.selected.InstanceID, map[string]string{"enable": "BASE", "mute": "COLLECTOR", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		switch bindings[index].Role {
		case "enable":
			bindings[index].Instance, bindings[index].Function = "enable_resistor", "A"
		case "power":
			bindings[index].Instance, bindings[index].Function = "mute_pullup", "A"
		}
	}
	connections := []RealizationConnection{
		semanticNet("mute_enable_drive", "control", passiveEndpoint("enable_resistor", "B"), endpoint(transistor, "BASE")),
		semanticNet("mute_state", "control", endpoint(transistor, "COLLECTOR"), passiveEndpoint("mute_pullup", "B")),
	}
	return provider.expansion(request, "fail_safe_open_collector_mute", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandClassABBias(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if constraint, ok := namedConstraint(request.Constraints, "thermal_tracking"); !ok || constraint.Relation != "required" {
		return nil, fmt.Errorf("Class-AB bias control requires thermal tracking")
	}
	supply := maximumPortVoltage(request.Ports)
	inverter, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{{Kind: "collector_emitter_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	inverter.selected.InstanceID, inverter.usage = "bias_enable_inverter", "fail_safe_enable"
	clamp := inverter
	clamp.selected.InstanceID, clamp.usage = "bias_clamp", "startup_bias_clamp"
	diode, err := provider.selectComponent(ctx, "diode", "switching", nil, true)
	if err != nil {
		return nil, err
	}
	diode.selected.InstanceID, diode.usage = "tracking_diode_1", "thermal_tracking"
	secondDiode := diode
	secondDiode.selected.InstanceID = "tracking_diode_2"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{inverter, clamp, diode, secondDiode}, []passivePart{
		{"enable_resistor", "resistor", "logic_drive", "10k"},
		{"inverter_pullup", "resistor", "default_clamp", "10k"},
		{"clamp_base_resistor", "resistor", "clamp_drive", "10k"},
		{"bias_feed", "resistor", "bias_current", "1k"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, inverter.selected.InstanceID, map[string]string{"enable": "BASE", "bias": "COLLECTOR", "power": "COLLECTOR", "reference": "EMITTER"})
	for index := range bindings {
		switch bindings[index].Role {
		case "enable":
			bindings[index].Instance, bindings[index].Function = "enable_resistor", "A"
		case "bias":
			bindings[index].Instance, bindings[index].Function = diode.selected.InstanceID, "A"
		case "power":
			bindings[index].Instance, bindings[index].Function = "bias_feed", "A"
		case "reference":
			bindings[index].Instance, bindings[index].Function = secondDiode.selected.InstanceID, "K"
		}
	}
	connections := []RealizationConnection{
		semanticNet("bias_enable_drive", "control", passiveEndpoint("enable_resistor", "B"), endpoint(inverter, "BASE")),
		semanticNet("bias_clamp_drive", "control", endpoint(inverter, "COLLECTOR"), passiveEndpoint("inverter_pullup", "B"), passiveEndpoint("clamp_base_resistor", "A")),
		semanticNet("bias_clamp_base", "control", passiveEndpoint("clamp_base_resistor", "B"), endpoint(clamp, "BASE")),
		semanticNet("bias_output", "analog_control", passiveEndpoint("bias_feed", "B"), endpoint(diode, "A"), endpoint(clamp, "COLLECTOR")),
		semanticNet("tracking_junction", "analog_control", endpoint(diode, "K"), endpoint(secondDiode, "A")),
		semanticNet("bias_reference", "reference", endpoint(secondDiode, "K"), endpoint(inverter, "EMITTER"), endpoint(clamp, "EMITTER")),
		semanticNet("bias_power", "power", passiveEndpoint("bias_feed", "A"), passiveEndpoint("inverter_pullup", "A")),
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
	npn, err := provider.selectComponent(ctx, "bjt", "NPN power_output", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(peakCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	pnp, err := provider.selectComponent(ctx, "bjt", "PNP power_output", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(peakCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	npn.selected.InstanceID, npn.usage = "output_npn", "class_ab_output"
	pnp.selected.InstanceID, pnp.usage = "output_pnp", "class_ab_output"
	opamp, err := provider.selectComponent(ctx, "opamp", "single", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	opamp.selected.InstanceID, opamp.usage = "voltage_driver", "class_ab_voltage_driver"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{opamp, npn, pnp}, []passivePart{
		{"input_coupling", "capacitor", "ac_coupling", "1u"},
		{"midpoint_upper", "resistor", "midpoint_bias", "47k"}, {"midpoint_lower", "resistor", "midpoint_bias", "47k"},
		{"midpoint_bypass", "capacitor", "midpoint_bypass", "100u"},
		{"npn_base_stop", "resistor", "base_stop", "47"}, {"pnp_base_stop", "resistor", "base_stop", "47"},
		{"npn_emitter", "resistor", "emitter_resistor", "0.22"}, {"pnp_emitter", "resistor", "emitter_resistor", "0.22"},
		{"bias_injection", "resistor", "bias_injection", "10k"}, {"mute_drive", "resistor", "mute_drive", "10k"},
		{"supply_bypass", "capacitor", "decoupling_capacitor", "100n"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, opamp.selected.InstanceID, map[string]string{"input": "IN_PLUS", "bias": "IN_PLUS", "mute": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	for index := range bindings {
		switch bindings[index].Role {
		case "input":
			bindings[index].Instance, bindings[index].Function = "input_coupling", "A"
		case "bias":
			bindings[index].Instance, bindings[index].Function = "bias_injection", "A"
		case "mute":
			bindings[index].Instance, bindings[index].Function = "mute_drive", "A"
		case "output":
			bindings[index].Instance, bindings[index].Function = "npn_emitter", "B"
		}
	}
	connections := []RealizationConnection{
		semanticNet("driver_input", "analog_signal", passiveEndpoint("input_coupling", "B"), endpoint(opamp, "IN_PLUS"), passiveEndpoint("midpoint_upper", "B"), passiveEndpoint("midpoint_lower", "A"), passiveEndpoint("midpoint_bypass", "A"), passiveEndpoint("bias_injection", "B"), passiveEndpoint("mute_drive", "B")),
		semanticNet("base_drive", "analog_signal", endpoint(opamp, "OUT"), passiveEndpoint("npn_base_stop", "A"), passiveEndpoint("pnp_base_stop", "A")),
		semanticNet("npn_base", "analog_signal", passiveEndpoint("npn_base_stop", "B"), endpoint(npn, "BASE")),
		semanticNet("pnp_base", "analog_signal", passiveEndpoint("pnp_base_stop", "B"), endpoint(pnp, "BASE")),
		semanticNet("npn_emitter_node", "power_signal", endpoint(npn, "EMITTER"), passiveEndpoint("npn_emitter", "A")),
		semanticNet("pnp_emitter_node", "power_signal", endpoint(pnp, "EMITTER"), passiveEndpoint("pnp_emitter", "A")),
		semanticNet("class_ab_output", "analog_output", passiveEndpoint("npn_emitter", "B"), passiveEndpoint("pnp_emitter", "B"), endpoint(opamp, "IN_MINUS")),
		semanticNet("class_ab_power", "power", endpoint(opamp, "V_PLUS"), endpoint(npn, "COLLECTOR"), passiveEndpoint("midpoint_upper", "A"), passiveEndpoint("supply_bypass", "A")),
		semanticNet("class_ab_reference", "reference", endpoint(opamp, "V_MINUS"), endpoint(pnp, "COLLECTOR"), passiveEndpoint("midpoint_lower", "B"), passiveEndpoint("midpoint_bypass", "B"), passiveEndpoint("supply_bypass", "B")),
	}
	calculation, issues := EvaluateRatings("class_ab_output_ratings", []RatingRequirement{
		{Kind: "npn_voltage", Required: supply, Rated: recordRatingOrZero(npn.record, "collector_emitter_voltage", "V"), DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: npn.evidence},
		{Kind: "npn_current", Required: peakCurrent, Rated: recordRatingOrZero(npn.record, "collector_current", "A"), DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: npn.evidence},
		{Kind: "pnp_voltage", Required: supply, Rated: recordRatingOrZero(pnp.record, "collector_emitter_voltage", "V"), DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: pnp.evidence},
		{Kind: "pnp_current", Required: peakCurrent, Rated: recordRatingOrZero(pnp.record, "collector_current", "A"), DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: pnp.evidence},
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("Class-AB output rating solution failed: %s", issues[0].Message)
	}
	perDeviceDissipation := math.Pow(supply/2, 2) / (math.Pi * math.Pi * load)
	npnThermal, err := thermalMarginCalculation("class_ab_npn_thermal", npn.record, perDeviceDissipation)
	if err != nil {
		return nil, err
	}
	pnpThermal, err := thermalMarginCalculation("class_ab_pnp_thermal", pnp.record, perDeviceDissipation)
	if err != nil {
		return nil, err
	}
	return provider.expansion(request, "complementary_emitter_follower", parts, bindings, connections, []CalculationEvidence{calculation, npnThermal, pnpThermal}, 0)
}

func (provider *CatalogProvider) expandClassAAmplification(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	gain, tolerance, ok := firstNumericConstraint(request.Constraints, "voltage_gain")
	if !ok || gain <= 1 {
		return nil, fmt.Errorf("Class-A amplification requires a gain greater than one")
	}
	quiescent, currentTolerance, ok := firstNumericConstraint(request.Constraints, "quiescent_current")
	if !ok || quiescent <= 0 {
		return nil, fmt.Errorf("Class-A amplification requires a positive quiescent-current target")
	}
	supply := maximumPortVoltage(request.Ports)
	collectorIdeal := supply / (2 * quiescent)
	emitterIdeal := collectorIdeal / gain
	emitterCandidates, candidateIssues := PreferredValueCandidates(emitterIdeal, SeriesE96, emitterIdeal*0.5, emitterIdeal*2, DefaultMaxValueCandidates)
	if len(candidateIssues) != 0 {
		return nil, fmt.Errorf("Class-A emitter value solution failed: %s", candidateIssues[0].Message)
	}
	var gainCalculation CalculationEvidence
	var solveIssues []reports.Issue
	emitter := 0.0
	for _, candidate := range emitterCandidates {
		gainCalculation, solveIssues = SolveDivider(DividerRequest{
			ID: "class_a_gain", Mode: DividerFeedback, SourceVoltageV: 1, TargetVoltageV: gain + 1,
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
	baseTarget := quiescent*emitter + 0.65
	biasCalculation, biasIssues := SolveDivider(DividerRequest{
		ID: "class_a_bias", Mode: DividerAttenuator, SourceVoltageV: supply, TargetVoltageV: baseTarget,
		TargetTolerancePercent: math.Max(currentTolerance, 5), LowerResistanceOhm: 2200, LowerTolerancePercent: 1,
		UpperTolerancePercent: 1, UpperSeries: SeriesE96,
	})
	if len(biasIssues) != 0 {
		return nil, fmt.Errorf("Class-A bias solution failed: %s", biasIssues[0].Message)
	}
	biasUpper, _ := calculationSelectedValue(biasCalculation, "upper_resistance")
	transistorDissipation := supply * quiescent / 2
	transistor, err := provider.selectComponent(ctx, "bjt", "NPN", []components.RequiredRating{
		{Kind: "collector_emitter_voltage", Value: numericString(supply / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "collector_current", Value: numericString(quiescent / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "power_dissipation", Value: numericString(transistorDissipation / catalogRatingDeratingFactor), Unit: "W"},
	}, true)
	if err != nil {
		return nil, err
	}
	transistor.selected.InstanceID, transistor.usage = "class_a_device", "class_a_gain_stage"
	parts, err := provider.appendPassiveParts(ctx, []catalogPart{transistor}, []passivePart{
		{"input_coupling", "capacitor", "ac_coupling", "1u"}, {"output_coupling", "capacitor", "ac_coupling", "220u"},
		{"collector_resistor", "resistor", "collector_load", engineeringValue(collector, "Ohm")},
		{"emitter_resistor", "resistor", "bias_stabilization", engineeringValue(emitter, "Ohm")},
		{"bias_upper", "resistor", "base_bias", engineeringValue(biasUpper, "Ohm")}, {"bias_lower", "resistor", "base_bias", "2.2k"},
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
		semanticNet("class_a_base", "analog_signal", passiveEndpoint("input_coupling", "B"), endpoint(transistor, "BASE"), passiveEndpoint("bias_upper", "B"), passiveEndpoint("bias_lower", "A")),
		semanticNet("class_a_collector", "analog_signal", endpoint(transistor, "COLLECTOR"), passiveEndpoint("collector_resistor", "B"), passiveEndpoint("output_coupling", "A")),
		semanticNet("class_a_emitter", "analog_signal", endpoint(transistor, "EMITTER"), passiveEndpoint("emitter_resistor", "A")),
		semanticNet("class_a_power", "power", passiveEndpoint("collector_resistor", "A"), passiveEndpoint("bias_upper", "A")),
		semanticNet("class_a_reference", "reference", passiveEndpoint("emitter_resistor", "B"), passiveEndpoint("bias_lower", "B")),
	}
	thermal, err := thermalMarginCalculation("class_a_thermal", transistor.record, transistorDissipation)
	if err != nil {
		return nil, err
	}
	return provider.expansion(request, "resistively_degenerated_class_a", parts, bindings, connections, []CalculationEvidence{gainCalculation, biasCalculation, thermal}, 0)
}

func thermalMarginCalculation(id string, record components.ComponentRecord, dissipationW float64) (CalculationEvidence, error) {
	if !finitePositive(dissipationW) || record.PowerSemiconductor == nil || record.PowerSemiconductor.MaxJunctionTemperatureC == nil {
		return CalculationEvidence{}, fmt.Errorf("selected power device lacks operating-dissipation or junction-temperature evidence")
	}
	evidence := record.PowerSemiconductor
	thermalResistance := 0.0
	if evidence.JunctionToAmbientCPerW != nil {
		thermalResistance = *evidence.JunctionToAmbientCPerW
	} else if evidence.JunctionToCaseCPerW != nil {
		thermalResistance = *evidence.JunctionToCaseCPerW
	}
	baseTemperature := 25.0
	if evidence.PowerDissipation != nil && evidence.PowerDissipation.TemperatureC != nil {
		baseTemperature = *evidence.PowerDissipation.TemperatureC
	}
	if thermalResistance <= 0 {
		return CalculationEvidence{}, fmt.Errorf("selected power device lacks thermal-resistance evidence")
	}
	junctionTemperature := baseTemperature + dissipationW*thermalResistance
	margin := *evidence.MaxJunctionTemperatureC - junctionTemperature
	if margin < 0 {
		return CalculationEvidence{}, fmt.Errorf("selected power device exceeds its catalog-backed junction-temperature envelope")
	}
	return ObservedCalculation(id,
		NamedQuantity{Name: "thermal_margin", Value: margin, Unit: "degC"},
		NamedQuantity{Name: "junction_temperature", Value: junctionTemperature, Unit: "degC"},
		NamedQuantity{Name: "power_dissipation", Value: dissipationW, Unit: "W"},
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
	converter, err := provider.selectComponent(ctx, "isolated_converter", "TRACO", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(inputMinimum), Unit: "V"},
		{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "positive_output_current", Value: numericString(positiveCurrent / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "negative_output_current", Value: numericString(negativeCurrent / catalogRatingDeratingFactor), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	converter.selected.InstanceID, converter.usage = "dual_rail_converter", "split_supply_converter"
	positiveRegulator, err := provider.selectComponent(ctx, "regulator", "LM317T", []components.RequiredRating{{Kind: "output_current", Value: numericString(positiveCurrent), Unit: "A"}}, true)
	if err != nil {
		return nil, err
	}
	positiveRegulator.selected.InstanceID, positiveRegulator.usage = "positive_regulator", "positive_post_regulator"
	negativeRegulator, err := provider.selectComponent(ctx, "regulator", "LM337T", []components.RequiredRating{{Kind: "output_current", Value: numericString(negativeCurrent), Unit: "A"}}, true)
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
