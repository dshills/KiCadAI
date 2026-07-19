package architecturesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	catalogProviderRevision         = "1.0.0"
	thresholdReferenceResistanceOhm = 10_000.0
)

var catalogProviderCapabilities = []string{
	"environment_sensor",
	"frequency_filter",
	"load_switch",
	"logic_level_translation",
	"programmable_controller",
	"threshold_detection",
	"voltage_regulation",
}

type CatalogProvider struct {
	catalog *components.Catalog
}

func NewCatalogProvider(catalog *components.Catalog) (*CatalogProvider, error) {
	if catalog == nil {
		return nil, fmt.Errorf("catalog provider requires a component catalog")
	}
	return &CatalogProvider{catalog: catalog}, nil
}

func NewCatalogRegistry(catalog *components.Catalog) (*Registry, []reports.Issue) {
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		return nil, []reports.Issue{architectureIssue(CodeProviderInvalid, "providers.catalog", err.Error())}
	}
	return NewRegistry(provider)
}

func (provider *CatalogProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID: "catalog_function_fragments", Revision: catalogProviderRevision,
		Capabilities: append([]string(nil), catalogProviderCapabilities...),
		Evidence: ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{
			"kicadai:catalog-selection", "kicadai:typed-port-contracts", "kicadai:value-solvers:" + FormulaRevision,
		}},
	}
}

func (provider *CatalogProvider) Expand(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if provider == nil || provider.catalog == nil {
		return nil, fmt.Errorf("catalog provider is not initialized")
	}
	switch canonicalIdentifier(request.Capability) {
	case "threshold_detection":
		return provider.expandThreshold(ctx, request)
	case "load_switch":
		return provider.expandLoadSwitch(ctx, request)
	case "voltage_regulation":
		return provider.expandRegulator(ctx, request)
	case "frequency_filter":
		return provider.expandFilter(ctx, request)
	case "logic_level_translation":
		return provider.expandTranslator(ctx, request)
	case "programmable_controller":
		return provider.expandSingleComponent(ctx, request, "mcu", "programmable_controller", "sensor_bus", "I2C_SDA")
	case "environment_sensor":
		return provider.expandSingleComponent(ctx, request, "sensor", "environment_sensor", "controller_bus", "SDA")
	default:
		return nil, fmt.Errorf("catalog provider does not support capability %s", request.Capability)
	}
}

func (provider *CatalogProvider) expandThreshold(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	center, centerTolerance, err := requiredNumber(request.Constraints, "threshold_voltage", "target", "V")
	if err != nil {
		return nil, err
	}
	width, widthTolerance, err := requiredNumber(request.Constraints, "hysteresis_width", "target", "V")
	if err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "output_polarity", "equal", "active_low"); err != nil {
		return nil, err
	}
	if err := requireBool(request.Constraints, "inactive_at_power_up", "required", true); err != nil {
		return nil, err
	}
	if delay, _, err := requiredNumber(request.Constraints, "propagation_delay", "maximum", "us"); err != nil || delay <= 0 {
		return nil, fmt.Errorf("threshold propagation-delay constraint is invalid")
	}
	supply := roleVoltageMaximum(request.Ports, "power")
	selection, err := provider.selectComponent(ctx, "comparator", "open_collector", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	calculation, issues := SolveHysteresis(HysteresisRequest{
		ID: "threshold_hysteresis", TargetCenterV: center, CenterTolerancePercent: centerTolerance,
		TargetWidthV: width, WidthTolerancePercent: widthTolerance, OutputLowV: 0.1,
		OutputHighV: supply, OutputUncertaintyV: 0.01, ReferenceResistanceOhm: thresholdReferenceResistanceOhm,
		ReferenceTolerancePercent: 0.1, FeedbackTolerancePercent: 0.1,
		ReferenceVoltageTolerancePercent: 0.1, MinimumReferenceVoltageV: 0,
		MaximumReferenceVoltageV: supply, FeedbackSeries: SeriesE96,
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("threshold value solution failed: %s", issues[0].Message)
	}
	feedbackResistance, ok := calculationSelectedValue(calculation, "feedback_resistance")
	if !ok {
		return nil, fmt.Errorf("threshold solution omitted feedback resistance")
	}
	referenceVoltage, ok := calculationOutput(calculation, "reference_voltage")
	if !ok || referenceVoltage <= 0 || referenceVoltage >= supply {
		return nil, fmt.Errorf("threshold solution produced an invalid reference voltage")
	}
	referenceUpper := thresholdReferenceResistanceOhm * supply / referenceVoltage
	referenceLower := thresholdReferenceResistanceOhm / (1 - referenceVoltage/supply)
	parts := []catalogPart{selection}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{
		{"threshold_upper", "resistor", "threshold_reference", engineeringValue(referenceUpper, "Ohm")},
		{"threshold_lower", "resistor", "threshold_reference", engineeringValue(referenceLower, "Ohm")},
		{"feedback_resistor", "resistor", "positive_feedback", engineeringValue(feedbackResistance, "Ohm")},
		{"output_pullup", "resistor", "open_collector_pullup", "4.7k"},
		{"supply_bypass", "capacitor", "decoupling", "100n"},
	})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"sense": "IN_MINUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	connections := []RealizationConnection{
		semanticNet("threshold_reference", "analog_signal", endpoint(selection, "IN_PLUS"), passiveEndpoint("threshold_upper", "B"), passiveEndpoint("threshold_lower", "A"), passiveEndpoint("feedback_resistor", "B")),
		semanticNet("threshold_output", "open_collector_signal", endpoint(selection, "OUT"), passiveEndpoint("output_pullup", "B"), passiveEndpoint("feedback_resistor", "A")),
		semanticNet("threshold_power", "power", endpoint(selection, "V_PLUS"), passiveEndpoint("threshold_upper", "A"), passiveEndpoint("output_pullup", "A"), passiveEndpoint("supply_bypass", "A")),
		semanticNet("threshold_ground", "reference", endpoint(selection, "V_MINUS"), passiveEndpoint("threshold_lower", "B"), passiveEndpoint("supply_bypass", "B")),
	}
	return provider.expansion(request, "open_collector_hysteresis", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func (provider *CatalogProvider) expandLoadSwitch(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "load_characteristic", "equal", "inductive"); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "control_active_state", "equal", "high"); err != nil {
		return nil, err
	}
	for _, name := range []string{"default_off", "inductive_transient_clamp", "control_overvoltage_clamp"} {
		if err := requireBool(request.Constraints, name, "required", true); err != nil {
			return nil, err
		}
	}
	voltage, _, err := requiredNumber(request.Constraints, "load_voltage", "minimum", "V")
	if err != nil {
		return nil, err
	}
	current, _, err := requiredNumber(request.Constraints, "load_current", "minimum", "A")
	if err != nil {
		return nil, err
	}
	selection, err := provider.selectComponent(ctx, "mosfet", "logic_level", []components.RequiredRating{
		{Kind: "drain_source_voltage", Value: numericString(voltage), Unit: "V"},
		{Kind: "drain_current", Value: numericString(current), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	flyback, err := provider.selectComponent(ctx, "diode", "flyback", []components.RequiredRating{
		{Kind: "current", Value: numericString(current), Unit: "A"},
		{Kind: "reverse_voltage", Value: numericString(voltage), Unit: "V"},
	}, true)
	if err != nil {
		return nil, err
	}
	flyback.selected.InstanceID = "flyback_clamp"
	flyback.usage = "inductive_transient_clamp"
	controlMaximum := roleVoltageMaximum(request.Ports, "control")
	controlClamp, err := provider.selectComponent(ctx, "diode", "zener", []components.RequiredRating{{Kind: "reverse_voltage", Value: numericString(controlMaximum), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	controlClamp.selected.InstanceID = "control_clamp"
	controlClamp.usage = "control_overvoltage_clamp"
	ratedVoltage, okVoltage := recordRatingMaximum(selection.record, "drain_source_voltage", "V")
	ratedCurrent, okCurrent := recordRatingMaximum(selection.record, "drain_current", "A")
	gateRated, okGate := recordRatingMaximum(selection.record, "gate_source_voltage", "V")
	flybackVoltage, okFlybackVoltage := recordRatingMaximum(flyback.record, "reverse_voltage", "V")
	flybackCurrent, okFlybackCurrent := recordRatingMaximum(flyback.record, "current", "A")
	clampVoltage, okClamp := recordRatingMaximum(controlClamp.record, "reverse_voltage", "V")
	if !okVoltage || !okCurrent || !okGate || !okFlybackVoltage || !okFlybackCurrent || !okClamp {
		return nil, fmt.Errorf("selected switch or protection device lacks normalized rating evidence")
	}
	calculation, issues := EvaluateRatings("switch_derating", []RatingRequirement{
		{Kind: "drain_source_voltage", Required: voltage, Rated: ratedVoltage, DeratingFactor: 0.8, Unit: "V", Evidence: selection.evidence},
		{Kind: "drain_current", Required: current, Rated: ratedCurrent, DeratingFactor: 0.8, Unit: "A", Evidence: selection.evidence},
		{Kind: "gate_source_voltage", Required: controlMaximum, Rated: gateRated, DeratingFactor: 0.8, Unit: "V", Evidence: selection.evidence},
		{Kind: "flyback_reverse_voltage", Required: voltage, Rated: flybackVoltage, DeratingFactor: 0.8, Unit: "V", Evidence: flyback.evidence},
		{Kind: "flyback_current", Required: current, Rated: flybackCurrent, DeratingFactor: 0.8, Unit: "A", Evidence: flyback.evidence},
		{Kind: "control_clamp_voltage", Required: controlMaximum, Rated: clampVoltage, DeratingFactor: 0.8, Unit: "V", Evidence: controlClamp.evidence},
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("switch rating solution failed: %s", issues[0].Message)
	}
	parts := []catalogPart{selection, flyback, controlClamp}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"gate_pulldown", "resistor", "default_off", "100k"}, {"gate_series", "resistor", "gate_drive", "100"}, {"supply_bypass", "capacitor", "decoupling", "100n"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"control": "GATE", "load": "DRAIN", "reference": "SOURCE"})
	for index := range bindings {
		switch bindings[index].Role {
		case "control":
			bindings[index].Instance, bindings[index].Function = "gate_series", "A"
		case "load_power":
			bindings[index].Instance, bindings[index].Function = flyback.selected.InstanceID, "K"
		case "logic_power":
			bindings[index].Instance, bindings[index].Function = "supply_bypass", "A"
		}
	}
	connections := []RealizationConnection{
		semanticNet("switch_gate", "control", endpoint(selection, "GATE"), passiveEndpoint("gate_series", "B"), passiveEndpoint("gate_pulldown", "A"), endpoint(controlClamp, "K")),
		semanticNet("switch_load", "switched_power", endpoint(selection, "DRAIN"), endpoint(flyback, "A")),
		semanticNet("switch_load_power", "power", endpoint(flyback, "K")),
		semanticNet("switch_logic_power", "power", passiveEndpoint("supply_bypass", "A")),
		semanticNet("switch_ground", "reference", endpoint(selection, "SOURCE"), endpoint(controlClamp, "A"), passiveEndpoint("gate_pulldown", "B"), passiveEndpoint("supply_bypass", "B")),
	}
	connections = retainSemanticNets(connections)
	return provider.expansion(request, "protected_low_side_switch", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func (provider *CatalogProvider) expandRegulator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireBool(request.Constraints, "adjustable_output", "required", true); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "set_point_programming", "equal", "passive_feedback"); err != nil {
		return nil, err
	}
	for _, name := range []string{"input_decoupling", "output_decoupling"} {
		if err := requireBool(request.Constraints, name, "required", true); err != nil {
			return nil, err
		}
	}
	inputRange, err := requiredRange(request.Constraints, "input_voltage", "range", "V")
	if err != nil {
		return nil, err
	}
	output, tolerance, err := requiredNumber(request.Constraints, "output_voltage", "target", "V")
	if err != nil {
		return nil, err
	}
	current, _, err := requiredNumber(request.Constraints, "continuous_output_current", "minimum", "A")
	if err != nil {
		return nil, err
	}
	inputMaximum := roleVoltageMaximum(request.Ports, "input")
	if inputRange[1] != inputMaximum {
		return nil, fmt.Errorf("input-voltage range and input port contract disagree")
	}
	selection, err := provider.selectComponent(ctx, "regulator", "adjustable", []components.RequiredRating{
		{Kind: "input_voltage", Value: numericString(inputMaximum), Unit: "V"},
		{Kind: "output_current", Value: numericString(current), Unit: "A"},
	}, true)
	if err != nil {
		return nil, err
	}
	reference, ok := recordValue(selection.record, "reference_voltage", "V")
	if !ok {
		return nil, fmt.Errorf("selected adjustable regulator lacks reference-voltage evidence")
	}
	calculation, issues := SolveDivider(DividerRequest{
		ID: "regulator_feedback", Mode: DividerFeedback, SourceVoltageV: reference,
		SourceTolerancePercent: 0.5, TargetVoltageV: output, TargetTolerancePercent: tolerance,
		LowerResistanceOhm: 10000, LowerTolerancePercent: 0.1, UpperTolerancePercent: 0.1,
		UpperSeries: SeriesE96,
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("regulator feedback solution failed: %s", issues[0].Message)
	}
	feedbackUpper, ok := calculationSelectedValue(calculation, "upper_resistance")
	if !ok {
		return nil, fmt.Errorf("regulator solution omitted upper feedback resistance")
	}
	parts := []catalogPart{selection}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"feedback_lower", "resistor", "feedback_divider", "10k"}, {"feedback_upper", "resistor", "feedback_divider", engineeringValue(feedbackUpper, "Ohm")}, {"input_bypass", "capacitor", "decoupling", "1u"}, {"output_bypass", "capacitor", "decoupling", "1u"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"input": "VIN", "output": "VOUT", "reference": "GND"})
	if !recordHasFunction(selection.record, "GND") {
		for index := range bindings {
			if bindings[index].Role == "reference" {
				bindings[index].Instance, bindings[index].Function = "feedback_lower", "B"
			}
		}
	}
	inputEndpoints := []RealizationEndpoint{endpoint(selection, "VIN"), passiveEndpoint("input_bypass", "A")}
	if recordHasFunction(selection.record, "EN") {
		inputEndpoints = append(inputEndpoints, endpoint(selection, "EN"))
	}
	groundEndpoints := []RealizationEndpoint{passiveEndpoint("feedback_lower", "B"), passiveEndpoint("input_bypass", "B"), passiveEndpoint("output_bypass", "B")}
	if recordHasFunction(selection.record, "GND") {
		groundEndpoints = append(groundEndpoints, endpoint(selection, "GND"))
	}
	connections := []RealizationConnection{
		semanticNet("regulator_input", "power", inputEndpoints...),
		semanticNet("regulator_output", "power", endpoint(selection, "VOUT"), passiveEndpoint("feedback_upper", "A"), passiveEndpoint("output_bypass", "A")),
		semanticNet("regulator_feedback", "analog_signal", endpoint(selection, "ADJ"), passiveEndpoint("feedback_upper", "B"), passiveEndpoint("feedback_lower", "A")),
		semanticNet("regulator_ground", "reference", groundEndpoints...),
	}
	return provider.expansion(request, "adjustable_linear_regulator", parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func (provider *CatalogProvider) expandFilter(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "response", "equal", "low_pass"); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "approximation", "equal", "butterworth"); err != nil {
		return nil, err
	}
	gain, _, err := requiredNumber(request.Constraints, "passband_gain", "target", "ratio")
	if err != nil || gain != 1 {
		return nil, fmt.Errorf("active filter provider supports unity passband gain")
	}
	ripple, _, err := requiredNumber(request.Constraints, "passband_ripple", "maximum", "dB")
	if err != nil || ripple < 0.1 {
		return nil, fmt.Errorf("active filter passband-ripple constraint is unsupported")
	}
	order, _, err := requiredNumber(request.Constraints, "order", "equal", "")
	if err != nil || int(order) != 4 {
		return nil, fmt.Errorf("active filter provider supports a fourth-order low-pass realization")
	}
	frequency, tolerance, err := requiredNumber(request.Constraints, "cutoff_frequency", "target", "Hz")
	if err != nil {
		return nil, err
	}
	supply := roleVoltageMaximum(request.Ports, "power")
	dualOpamp, err := provider.selectComponent(ctx, "opamp", "dual", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	singleOpamp, err := provider.selectComponent(ctx, "opamp", "single", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	firstQ, firstQOK := ButterworthStageQ(4, 0)
	secondQ, secondQOK := ButterworthStageQ(4, 1)
	if !firstQOK || !secondQOK {
		return nil, fmt.Errorf("fourth-order Butterworth stage factors are unavailable")
	}
	first, issues := SolveSallenKeyLowPass(SallenKeyLowPassRequest{ID: "filter_stage_1", TargetFrequencyHz: frequency, FrequencyTolerancePercent: tolerance, TargetQ: firstQ, QTolerancePercent: 5, ResistanceOhm: 10000, ResistanceTolerancePercent: 0.1, CapacitanceTolerancePercent: 1, CapacitanceSeries: SeriesE96})
	if len(issues) != 0 {
		return nil, fmt.Errorf("first filter stage failed: %s", issues[0].Message)
	}
	second, issues := SolveSallenKeyLowPass(SallenKeyLowPassRequest{ID: "filter_stage_2", TargetFrequencyHz: frequency, FrequencyTolerancePercent: tolerance, TargetQ: secondQ, QTolerancePercent: 5, ResistanceOhm: 10000, ResistanceTolerancePercent: 0.1, CapacitanceTolerancePercent: 1, CapacitanceSeries: SeriesE96})
	if len(issues) != 0 {
		return nil, fmt.Errorf("second filter stage failed: %s", issues[0].Message)
	}
	firstC1, firstC1OK := calculationSelectedValue(first, "capacitance_1")
	firstC2, firstC2OK := calculationSelectedValue(first, "capacitance_2")
	secondC1, secondC1OK := calculationSelectedValue(second, "capacitance_1")
	secondC2, secondC2OK := calculationSelectedValue(second, "capacitance_2")
	if !firstC1OK || !firstC2OK || !secondC1OK || !secondC2OK {
		return nil, fmt.Errorf("filter solution omitted a stage capacitance")
	}
	passives := []passivePart{{"stage_1_r1", "resistor", "filter", "10k"}, {"stage_1_r2", "resistor", "filter", "10k"}, {"stage_1_c1", "capacitor", "filter", engineeringValue(firstC1, "F")}, {"stage_1_c2", "capacitor", "filter", engineeringValue(firstC2, "F")}, {"stage_2_r1", "resistor", "filter", "10k"}, {"stage_2_r2", "resistor", "filter", "10k"}, {"stage_2_c1", "capacitor", "filter", engineeringValue(secondC1, "F")}, {"stage_2_c2", "capacitor", "filter", engineeringValue(secondC2, "F")}, {"supply_bypass", "capacitor", "decoupling", "100n"}}
	dualParts, err := provider.appendPassiveParts(ctx, []catalogPart{dualOpamp}, passives)
	if err != nil {
		return nil, err
	}
	secondSingle := singleOpamp
	secondSingle.selected.InstanceID = "amplifier_2"
	singleParts, err := provider.appendPassiveParts(ctx, []catalogPart{singleOpamp, secondSingle}, passives)
	if err != nil {
		return nil, err
	}
	dualBindings := bindRoles(request.Ports, dualOpamp.selected.InstanceID, map[string]string{"input": "CHANNEL_1_IN_PLUS", "output": "CHANNEL_2_OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	for index := range dualBindings {
		if dualBindings[index].Role == "input" {
			dualBindings[index].Instance, dualBindings[index].Function = "stage_1_r1", "A"
		}
	}
	singleBindings := bindRoles(request.Ports, singleOpamp.selected.InstanceID, map[string]string{"input": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	for index := range singleBindings {
		if singleBindings[index].Role == "input" {
			singleBindings[index].Instance, singleBindings[index].Function = "stage_1_r1", "A"
		} else if singleBindings[index].Role == "output" {
			singleBindings[index].Instance, singleBindings[index].Function = "amplifier_2", "OUT"
		}
	}
	dualConnections := sallenKeyConnections(dualOpamp.selected.InstanceID, dualOpamp.selected.InstanceID, "CHANNEL_1", "CHANNEL_2")
	singleConnections := sallenKeyConnections(singleOpamp.selected.InstanceID, "amplifier_2", "", "")
	dualExpansion, err := provider.expansion(request, "dual_opamp_sallen_key_cascade", dualParts, dualBindings, dualConnections, []CalculationEvidence{first, second}, 0)
	if err != nil {
		return nil, err
	}
	singleExpansion, err := provider.expansion(request, "two_single_opamp_sallen_key_cascade", singleParts, singleBindings, singleConnections, []CalculationEvidence{first, second}, 0)
	if err != nil {
		return nil, err
	}
	return append(dualExpansion, singleExpansion...), nil
}

func (provider *CatalogProvider) expandTranslator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	for name, value := range map[string]string{"protocol": "i2c", "signaling_mode": "open_drain", "direction": "bidirectional"} {
		if err := requireString(request.Constraints, name, "equal", value); err != nil {
			return nil, err
		}
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, err
	}
	frequency, _, err := requiredNumber(request.Constraints, "bus_frequency", "minimum", "Hz")
	if err != nil {
		return nil, err
	}
	voltageA := roleVoltageMaximum(request.Ports, "power_a")
	voltageB := roleVoltageMaximum(request.Ports, "power_b")
	low, high := math.Min(voltageA, voltageB), math.Max(voltageA, voltageB)
	selection, err := provider.selectComponent(ctx, "level_translator", "partial_power_down", []components.RequiredRating{
		{Kind: "vcca_supply_voltage", Value: numericString(low), Unit: "V"},
		{Kind: "vccb_supply_voltage", Value: numericString(high), Unit: "V"},
		{Kind: "open_drain_data_rate", Value: numericString(frequency), Unit: "Hz"},
	}, true)
	if err != nil {
		return nil, err
	}
	parts := []catalogPart{selection}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"side_a_sda_pullup", "resistor", "bus_pullup", "4.7k"}, {"side_a_scl_pullup", "resistor", "bus_pullup", "4.7k"}, {"side_b_sda_pullup", "resistor", "bus_pullup", "4.7k"}, {"side_b_scl_pullup", "resistor", "bus_pullup", "4.7k"}, {"vcca_bypass", "capacitor", "decoupling", "100n"}, {"vccb_bypass", "capacitor", "decoupling", "100n"}})
	if err != nil {
		return nil, err
	}
	functions := map[string]string{"side_a_sda": "A1", "side_a_scl": "A2", "side_b_sda": "B1", "side_b_scl": "B2", "reference": "GND"}
	if voltageA <= voltageB {
		functions["power_a"], functions["power_b"] = "VCCA", "VCCB"
	} else {
		functions["power_a"], functions["power_b"] = "VCCB", "VCCA"
		functions["side_a_sda"], functions["side_a_scl"] = "B1", "B2"
		functions["side_b_sda"], functions["side_b_scl"] = "A1", "A2"
	}
	bindings := bindBusRoles(request.Ports, selection.selected.InstanceID, functions)
	connections := []RealizationConnection{
		semanticNet("translator_side_a_sda", "open_drain_bus", endpoint(selection, functions["side_a_sda"]), passiveEndpoint("side_a_sda_pullup", "B")),
		semanticNet("translator_side_a_scl", "open_drain_bus", endpoint(selection, functions["side_a_scl"]), passiveEndpoint("side_a_scl_pullup", "B")),
		semanticNet("translator_side_b_sda", "open_drain_bus", endpoint(selection, functions["side_b_sda"]), passiveEndpoint("side_b_sda_pullup", "B")),
		semanticNet("translator_side_b_scl", "open_drain_bus", endpoint(selection, functions["side_b_scl"]), passiveEndpoint("side_b_scl_pullup", "B")),
		semanticNet("translator_power_a", "power", endpoint(selection, functions["power_a"]), passiveEndpoint("side_a_sda_pullup", "A"), passiveEndpoint("side_a_scl_pullup", "A"), bypassPowerEndpoint(voltageA <= voltageB, "A")),
		semanticNet("translator_power_b", "power", endpoint(selection, functions["power_b"]), passiveEndpoint("side_b_sda_pullup", "A"), passiveEndpoint("side_b_scl_pullup", "A"), bypassPowerEndpoint(voltageA > voltageB, "A")),
		semanticNet("translator_ground", "reference", endpoint(selection, "GND"), passiveEndpoint("vcca_bypass", "B"), passiveEndpoint("vccb_bypass", "B")),
	}
	for index := range connections {
		if connections[index].ID == "translator_power_a" && functions["power_a"] == "VCCA" || connections[index].ID == "translator_power_b" && functions["power_b"] == "VCCA" {
			connections[index].Endpoints = append(connections[index].Endpoints, endpoint(selection, "OE"))
		}
	}
	return provider.expansion(request, "bidirectional_open_drain_translator", parts, bindings, connections, nil, 0)
}

func (provider *CatalogProvider) expandSingleComponent(ctx context.Context, request ProviderRequest, family, usage, busRole, busFunction string) ([]ProviderExpansion, error) {
	searchText := ""
	switch request.Capability {
	case "programmable_controller":
		if err := requireBool(request.Constraints, "programmable_interface", "required", true); err != nil {
			return nil, err
		}
	case "environment_sensor":
		measurements, err := requiredStringArray(request.Constraints, "measurement", "one_of")
		if err != nil {
			return nil, err
		}
		searchText = measurements[0]
	}
	supply := maximumPortVoltage(request.Ports)
	ratings := []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}
	selection, err := provider.selectComponent(ctx, family, searchText, ratings, true)
	if family == "mcu" {
		selection, err = provider.selectSmallestComponent(ctx, family, ratings)
	}
	if err != nil {
		return nil, err
	}
	powerFunction := firstRecordFunction(selection.record, "VDD", "VCC")
	functions := map[string]string{"power": powerFunction, "reference": "GND", busRole + "_sda": busFunction}
	if family == "mcu" {
		functions[busRole+"_scl"] = "I2C_SCL"
	} else {
		functions[busRole+"_scl"] = "SCL"
	}
	bindings := bindBusRoles(request.Ports, selection.selected.InstanceID, functions)
	var connections []RealizationConnection
	parts := []catalogPart{selection}
	if family == "mcu" {
		powerEndpoints := recordSemanticEndpoints(selection, powerFunction, "AVCC", "VDDIO", "VDDA")
		groundEndpoints := recordSemanticEndpoints(selection, "GND", "AGND", "DGND", "PGND", "VSS")
		if len(powerEndpoints) > 1 {
			connections = append(connections, semanticNet("controller_power", "power", powerEndpoints...))
		}
		if len(groundEndpoints) > 1 {
			connections = append(connections, semanticNet("controller_ground", "reference", groundEndpoints...))
		}
	} else if family == "sensor" {
		connections = append(connections, semanticNet("sensor_power", "power", endpoint(selection, "VDD"), endpoint(selection, "VDDIO"), endpoint(selection, "CSB")))
		if recordHasFunction(selection.record, "SDO") {
			parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"address_strap", "resistor", "configuration_strap", "10k"}})
			if err != nil {
				return nil, err
			}
			connections = append(connections,
				semanticNet("sensor_ground", "reference", endpoint(selection, "GND"), passiveEndpoint("address_strap", "B")),
				semanticNet("sensor_address", "bias", endpoint(selection, "SDO"), passiveEndpoint("address_strap", "A")),
			)
		} else {
			connections = append(connections, semanticNet("sensor_ground", "reference", endpoint(selection, "GND")))
		}
	}
	return provider.expansion(request, usage, parts, bindings, connections, nil, 0)
}

func recordSemanticEndpoints(part catalogPart, functions ...string) []RealizationEndpoint {
	endpoints := make([]RealizationEndpoint, 0, len(functions))
	seen := map[string]bool{}
	for _, function := range functions {
		function = strings.ToUpper(strings.TrimSpace(function))
		if function == "" || seen[function] || !recordHasFunction(part.record, function) {
			continue
		}
		seen[function] = true
		endpoints = append(endpoints, endpoint(part, function))
	}
	return endpoints
}

func (provider *CatalogProvider) selectSmallestComponent(_ context.Context, family string, ratings []components.RequiredRating) (catalogPart, error) {
	type candidate struct {
		part      catalogPart
		area      float64
		variantID string
	}
	var candidates []candidate
	for _, record := range provider.catalog.Records {
		if record.Family != family || record.Generic || record.MPN == "" || !recordSupportsRatings(record, ratings) {
			continue
		}
		for _, variant := range record.Packages {
			if variant.DimensionsMM == nil || confidenceRank(EvidenceConfidence(variant.Verification.Confidence)) < confidenceRank(EvidenceRuleInferred) {
				continue
			}
			area := variant.DimensionsMM.Width * variant.DimensionsMM.Height
			evidence := componentEvidence(record, variant.Verification.Confidence)
			candidates = append(candidates, candidate{part: catalogPart{
				selected: SelectedComponent{InstanceID: canonicalIdentifier(family), CatalogID: record.ID, VariantID: variant.ID, Evidence: evidence.Confidence},
				record:   record, usage: canonicalIdentifier(family), evidence: evidence,
			}, area: area, variantID: variant.ID})
		}
	}
	if len(candidates) == 0 {
		return catalogPart{}, fmt.Errorf("no dimensioned catalog-backed %s satisfies normalized ratings", family)
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
		return strings.Compare(left.variantID, right.variantID)
	})
	return candidates[0].part, nil
}

func recordSupportsRatings(record components.ComponentRecord, required []components.RequiredRating) bool {
	for _, want := range required {
		value, err := strconv.ParseFloat(want.Value, 64)
		if err != nil {
			return false
		}
		satisfied := false
		for _, rating := range record.Ratings {
			if !strings.EqualFold(rating.Kind, want.Kind) {
				continue
			}
			if rating.Min != "" {
				minimum, err := strconv.ParseFloat(rating.Min, 64)
				if err != nil {
					return false
				}
				converted, ok := convertCatalogUnit(value, want.Unit, rating.Unit)
				if !ok || quantize(converted) < quantize(minimum) {
					continue
				}
			}
			if rating.Max != "" {
				maximum, err := strconv.ParseFloat(rating.Max, 64)
				if err != nil {
					return false
				}
				converted, ok := convertCatalogUnit(value, want.Unit, rating.Unit)
				if !ok || quantize(converted) > quantize(maximum) {
					continue
				}
			}
			satisfied = true
			break
		}
		if !satisfied {
			return false
		}
	}
	return true
}

type catalogPart struct {
	selected SelectedComponent
	record   components.ComponentRecord
	usage    string
	value    string
	evidence ContractEvidence
}

type passivePart struct{ id, family, usage, value string }

func (provider *CatalogProvider) selectComponent(ctx context.Context, family, text string, ratings []components.RequiredRating, concrete bool) (catalogPart, error) {
	selection, result := components.Select(ctx, provider.catalog, components.SelectionRequest{
		Query:      components.Query{Text: text, Family: family, MinimumConfidence: components.ConfidenceRuleInferred, Limit: 64},
		Acceptance: components.AcceptanceStructural, AllowAlternatives: true,
		RequiredRatings: ratings, RequireConcrete: concrete,
	})
	if !result.OK {
		return catalogPart{}, fmt.Errorf("no catalog-backed %s satisfies normalized ratings", family)
	}
	evidence := componentEvidence(selection.Component, selection.Candidate.Confidence)
	return catalogPart{
		selected: SelectedComponent{InstanceID: canonicalIdentifier(family), CatalogID: selection.Component.ID, VariantID: selection.Variant.ID, Evidence: evidence.Confidence},
		record:   selection.Component, usage: canonicalIdentifier(family), evidence: evidence,
	}, nil
}

func (provider *CatalogProvider) appendPassiveParts(ctx context.Context, parts []catalogPart, requested []passivePart) ([]catalogPart, error) {
	for _, passive := range requested {
		part, err := provider.selectComponent(ctx, passive.family, "", nil, false)
		if err != nil {
			return nil, err
		}
		part.selected.InstanceID = passive.id
		part.usage = passive.usage
		part.value = passive.value
		parts = append(parts, part)
	}
	return parts, nil
}

func (provider *CatalogProvider) expansion(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, connections []RealizationConnection, calculations []CalculationEvidence, unproven int) ([]ProviderExpansion, error) {
	instances := make([]RealizationInstance, 0, len(parts))
	componentsSelected := make([]SelectedComponent, 0, len(parts))
	for _, part := range parts {
		componentsSelected = append(componentsSelected, part.selected)
		instances = append(instances, RealizationInstance{ID: part.selected.InstanceID, CatalogID: part.selected.CatalogID, VariantID: part.selected.VariantID, Usage: part.usage, Value: part.value})
	}
	parameters := calculationParameters(calculations)
	payload, err := MarshalFragmentRealization(FragmentRealization{Capability: request.Capability, Instances: instances, PortBindings: bindings, Connections: connections, Parameters: parameters})
	if err != nil {
		return nil, err
	}
	margin := minimumCalculationMargin(calculations)
	return []ProviderExpansion{{
		ID: id, OfferedPorts: offeredCatalogPorts(request.Ports), Components: componentsSelected,
		Calculations: calculations, Metrics: ExpansionMetrics{UnprovenNonSafety: unproven, WorstMargin: &margin},
		Evidence: ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:catalog-function-provider:" + catalogProviderRevision}},
		Payload:  payload,
	}}, nil
}

func offeredCatalogPorts(required []RoleContract) []RoleContract {
	ports := cloneRoleContractsJSON(required)
	for index := range ports {
		contract := &ports[index].Contract
		contract.ID = ""
		contract.MinimumEvidence = ""
		contract.Evidence = ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:catalog-function-provider:" + catalogProviderRevision}}
		contract.CurrentCapacityA = cloneFloat64(contract.RequiredCurrentCapacityA)
		contract.CurrentDemandA = cloneFloat64(contract.MaximumCurrentDemandA)
		contract.Traits = append(contract.Traits, contract.RequiredTraits...)
		contract.RequiredTraits = nil
	}
	return ports
}

func cloneRoleContractsJSON(ports []RoleContract) []RoleContract {
	encoded, _ := json.Marshal(ports)
	var result []RoleContract
	_ = json.Unmarshal(encoded, &result)
	return result
}

func bindRoles(ports []RoleContract, instance string, functions map[string]string) []RealizationPortBinding {
	bindings := make([]RealizationPortBinding, 0, len(ports))
	for _, port := range ports {
		function := functions[port.Role]
		if function == "" {
			function = strings.ToUpper(port.Role)
		}
		bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: instance, Function: function})
	}
	return bindings
}

func bindBusRoles(ports []RoleContract, instance string, functions map[string]string) []RealizationPortBinding {
	bindings := make([]RealizationPortBinding, 0, len(ports)+2)
	for _, port := range ports {
		sda, scl := functions[port.Role+"_sda"], functions[port.Role+"_scl"]
		if sda != "" || scl != "" {
			if sda != "" {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Lane: "sda", Instance: instance, Function: sda})
			}
			if scl != "" {
				bindings = append(bindings, RealizationPortBinding{Role: port.Role, Lane: "scl", Instance: instance, Function: scl})
			}
			continue
		}
		function := functions[port.Role]
		if function == "" {
			function = strings.ToUpper(port.Role)
		}
		bindings = append(bindings, RealizationPortBinding{Role: port.Role, Instance: instance, Function: function})
	}
	return bindings
}

func endpoint(part catalogPart, function string) RealizationEndpoint {
	return RealizationEndpoint{Instance: part.selected.InstanceID, Function: function}
}

func passiveEndpoint(instance, function string) RealizationEndpoint {
	return RealizationEndpoint{Instance: instance, Function: function}
}

func semanticNet(id, role string, endpoints ...RealizationEndpoint) RealizationConnection {
	return RealizationConnection{ID: id, Role: role, Endpoints: endpoints}
}

func retainSemanticNets(connections []RealizationConnection) []RealizationConnection {
	result := make([]RealizationConnection, 0, len(connections))
	for _, connection := range connections {
		if len(connection.Endpoints) >= 2 {
			result = append(result, connection)
		}
	}
	return result
}

func bypassPowerEndpoint(vcca bool, function string) RealizationEndpoint {
	if vcca {
		return passiveEndpoint("vcca_bypass", function)
	}
	return passiveEndpoint("vccb_bypass", function)
}

func sallenKeyConnections(firstAmplifier, secondAmplifier, firstChannel, secondChannel string) []RealizationConnection {
	ampFunction := func(channel, function string) string {
		if channel == "" {
			return function
		}
		return channel + "_" + function
	}
	connections := []RealizationConnection{
		semanticNet("filter_stage_1_node_1", "analog_signal", passiveEndpoint("stage_1_r1", "B"), passiveEndpoint("stage_1_r2", "A"), passiveEndpoint("stage_1_c1", "A")),
		semanticNet("filter_stage_1_node_2", "analog_signal", passiveEndpoint("stage_1_r2", "B"), passiveEndpoint("stage_1_c2", "A"), passiveEndpoint(firstAmplifier, ampFunction(firstChannel, "IN_PLUS"))),
		semanticNet("filter_stage_1_output", "analog_signal", passiveEndpoint(firstAmplifier, ampFunction(firstChannel, "OUT")), passiveEndpoint(firstAmplifier, ampFunction(firstChannel, "IN_MINUS")), passiveEndpoint("stage_1_c1", "B"), passiveEndpoint("stage_2_r1", "A")),
		semanticNet("filter_stage_2_node_1", "analog_signal", passiveEndpoint("stage_2_r1", "B"), passiveEndpoint("stage_2_r2", "A"), passiveEndpoint("stage_2_c1", "A")),
		semanticNet("filter_stage_2_node_2", "analog_signal", passiveEndpoint("stage_2_r2", "B"), passiveEndpoint("stage_2_c2", "A"), passiveEndpoint(secondAmplifier, ampFunction(secondChannel, "IN_PLUS"))),
		semanticNet("filter_stage_2_output", "analog_signal", passiveEndpoint(secondAmplifier, ampFunction(secondChannel, "OUT")), passiveEndpoint(secondAmplifier, ampFunction(secondChannel, "IN_MINUS")), passiveEndpoint("stage_2_c1", "B")),
		semanticNet("filter_power", "power", passiveEndpoint(firstAmplifier, "V_PLUS"), passiveEndpoint("supply_bypass", "A")),
		semanticNet("filter_ground", "reference", passiveEndpoint(firstAmplifier, "V_MINUS"), passiveEndpoint("stage_1_c2", "B"), passiveEndpoint("stage_2_c2", "B"), passiveEndpoint("supply_bypass", "B")),
	}
	if secondAmplifier != firstAmplifier {
		for index := range connections {
			switch connections[index].ID {
			case "filter_power":
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint(secondAmplifier, "V_PLUS"))
			case "filter_ground":
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint(secondAmplifier, "V_MINUS"))
			}
		}
	}
	return connections
}

func calculationSelectedValue(calculation CalculationEvidence, name string) (float64, bool) {
	for _, selected := range calculation.SelectedValues {
		if selected.Name == name {
			return selected.Selected, true
		}
	}
	return 0, false
}

func calculationOutput(calculation CalculationEvidence, name string) (float64, bool) {
	for _, output := range calculation.NominalOutputs {
		if output.Name == name {
			return output.Value, true
		}
	}
	return 0, false
}

func engineeringValue(value float64, unit string) string {
	if unit == "Ohm" {
		switch {
		case value >= 1e6:
			return compactNumber(value/1e6) + "M"
		case value >= 1e3:
			return compactNumber(value/1e3) + "k"
		default:
			return compactNumber(value)
		}
	}
	if unit == "F" {
		switch {
		case value >= 1e-3:
			return compactNumber(value*1e3) + "m"
		case value >= 1e-6:
			return compactNumber(value*1e6) + "u"
		case value >= 1e-9:
			return compactNumber(value*1e9) + "n"
		default:
			return compactNumber(value*1e12) + "p"
		}
	}
	return compactNumber(value)
}

func compactNumber(value float64) string {
	return strconv.FormatFloat(quantize(value), 'f', -1, 64)
}

func calculationParameters(calculations []CalculationEvidence) []RealizationParameter {
	var result []RealizationParameter
	for _, calculation := range calculations {
		for _, selected := range calculation.SelectedValues {
			result = append(result, RealizationParameter{Name: calculation.ID + "_" + selected.Name, Value: selected.Selected, Unit: selected.Unit})
		}
	}
	return result
}

func minimumCalculationMargin(calculations []CalculationEvidence) float64 {
	if len(calculations) == 0 {
		return 0.01
	}
	margin := math.Inf(1)
	for _, calculation := range calculations {
		margin = math.Min(margin, calculation.WorstMargin)
	}
	return quantize(margin)
}

func componentEvidence(record components.ComponentRecord, selectedConfidence components.ConfidenceLevel) ContractEvidence {
	confidence := EvidenceConfidence(selectedConfidence)
	return ContractEvidence{Confidence: confidence, Sources: append([]string(nil), record.Verification.Sources...)}
}

func requiredNumber(constraints []Constraint, name, relation, unit string) (float64, float64, error) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok || constraint.Relation != relation || (unit != "" && constraint.Unit != unit) {
		return 0, 0, fmt.Errorf("constraint %s must use relation %s and unit %s", name, relation, unit)
	}
	var value float64
	if err := json.Unmarshal(constraint.Value, &value); err != nil || !finiteNumbers(value) {
		return 0, 0, fmt.Errorf("constraint %s requires a finite numeric value", name)
	}
	tolerance := 0.0
	if constraint.TolerancePercent != nil {
		tolerance = *constraint.TolerancePercent
	}
	return value, tolerance, nil
}

func requireString(constraints []Constraint, name, relation, want string) error {
	constraint, ok := namedConstraint(constraints, name)
	if !ok || constraint.Relation != relation {
		return fmt.Errorf("constraint %s must use relation %s", name, relation)
	}
	var value string
	if err := json.Unmarshal(constraint.Value, &value); err != nil || canonicalIdentifier(value) != want {
		return fmt.Errorf("constraint %s value is unsupported", name)
	}
	return nil
}

func requireBool(constraints []Constraint, name, relation string, want bool) error {
	constraint, ok := namedConstraint(constraints, name)
	if !ok || constraint.Relation != relation {
		return fmt.Errorf("constraint %s must use relation %s", name, relation)
	}
	var value bool
	if err := json.Unmarshal(constraint.Value, &value); err != nil || value != want {
		return fmt.Errorf("constraint %s value is unsupported", name)
	}
	return nil
}

func requiredRange(constraints []Constraint, name, relation, unit string) ([2]float64, error) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok || constraint.Relation != relation || constraint.Unit != unit {
		return [2]float64{}, fmt.Errorf("constraint %s must use relation %s and unit %s", name, relation, unit)
	}
	var values []float64
	if err := json.Unmarshal(constraint.Value, &values); err != nil || len(values) != 2 || !finiteNumbers(values...) || values[0] >= values[1] {
		return [2]float64{}, fmt.Errorf("constraint %s requires an increasing numeric range", name)
	}
	return [2]float64{values[0], values[1]}, nil
}

func requiredStringArray(constraints []Constraint, name, relation string) ([]string, error) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok || constraint.Relation != relation {
		return nil, fmt.Errorf("constraint %s must use relation %s", name, relation)
	}
	var values []string
	if err := json.Unmarshal(constraint.Value, &values); err != nil || len(values) == 0 {
		return nil, fmt.Errorf("constraint %s requires one or more string values", name)
	}
	for index := range values {
		values[index] = canonicalIdentifier(values[index])
		if !validSemanticID(values[index]) {
			return nil, fmt.Errorf("constraint %s contains an invalid value", name)
		}
	}
	slices.Sort(values)
	return values, nil
}

func namedConstraint(constraints []Constraint, name string) (Constraint, bool) {
	for _, constraint := range constraints {
		if constraint.Name == name {
			return constraint, true
		}
	}
	return Constraint{}, false
}

func roleVoltageMaximum(ports []RoleContract, role string) float64 {
	for _, port := range ports {
		if port.Role == role && port.Contract.Voltage.Maximum != nil {
			return *port.Contract.Voltage.Maximum
		}
	}
	return 0
}

func maximumPortVoltage(ports []RoleContract) float64 {
	maximum := 0.0
	for _, port := range ports {
		if port.Contract.Voltage.Maximum != nil {
			maximum = math.Max(maximum, *port.Contract.Voltage.Maximum)
		}
	}
	return maximum
}

func recordRatingMaximum(record components.ComponentRecord, kind, unit string) (float64, bool) {
	for _, rating := range record.Ratings {
		if rating.Kind != kind || rating.Max == "" {
			continue
		}
		value, err := strconv.ParseFloat(rating.Max, 64)
		if err != nil {
			return 0, false
		}
		converted, ok := convertCatalogUnit(value, rating.Unit, unit)
		return converted, ok
	}
	return 0, false
}

func recordValue(record components.ComponentRecord, kind, unit string) (float64, bool) {
	for _, value := range record.Values {
		if value.Kind != kind || value.Typ == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(value.Typ, 64)
		if err != nil {
			return 0, false
		}
		return convertCatalogUnit(parsed, value.Unit, unit)
	}
	return 0, false
}

func recordHasFunction(record components.ComponentRecord, function string) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if strings.EqualFold(pin.Function, function) {
				return true
			}
			for _, alias := range pin.Aliases {
				if strings.EqualFold(alias, function) {
					return true
				}
			}
		}
	}
	return false
}

func firstRecordFunction(record components.ComponentRecord, candidates ...string) string {
	for _, candidate := range candidates {
		if recordHasFunction(record, candidate) {
			return candidate
		}
	}
	return ""
}

func convertCatalogUnit(value float64, from, to string) (float64, bool) {
	if strings.EqualFold(from, to) {
		return value, true
	}
	if from == "mA" && to == "A" {
		return value / 1000, true
	}
	if from == "mV" && to == "V" {
		return value / 1000, true
	}
	return 0, false
}

func numericString(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
