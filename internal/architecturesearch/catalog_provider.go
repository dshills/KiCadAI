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

const catalogProviderRevision = "1.0.0"

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
		OutputHighV: supply, OutputUncertaintyV: 0.01, ReferenceResistanceOhm: 10000,
		ReferenceTolerancePercent: 0.1, FeedbackTolerancePercent: 0.1,
		ReferenceVoltageTolerancePercent: 0.1, MinimumReferenceVoltageV: 0,
		MaximumReferenceVoltageV: supply, FeedbackSeries: SeriesE96,
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("threshold value solution failed: %s", issues[0].Message)
	}
	parts := []catalogPart{selection}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"reference_resistor", "resistor", "threshold_reference"}, {"feedback_resistor", "resistor", "positive_feedback"}, {"output_pullup", "resistor", "open_collector_pullup"}, {"supply_bypass", "capacitor", "decoupling"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"sense": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	return provider.expansion(request, "open_collector_hysteresis", parts, bindings, []CalculationEvidence{calculation}, 1)
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
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"gate_pulldown", "resistor", "default_off"}, {"gate_series", "resistor", "gate_drive"}, {"supply_bypass", "capacitor", "decoupling"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"control": "GATE", "load": "DRAIN", "reference": "SOURCE"})
	for index := range bindings {
		switch bindings[index].Role {
		case "load_power":
			bindings[index].Instance, bindings[index].Function = flyback.selected.InstanceID, "K"
		case "logic_power":
			bindings[index].Instance, bindings[index].Function = "supply_bypass", "A"
		}
	}
	return provider.expansion(request, "protected_low_side_switch", parts, bindings, []CalculationEvidence{calculation}, 1)
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
	parts := []catalogPart{selection}
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"feedback_lower", "resistor", "feedback_divider"}, {"feedback_upper", "resistor", "feedback_divider"}, {"input_bypass", "capacitor", "decoupling"}, {"output_bypass", "capacitor", "decoupling"}})
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"input": "VIN", "output": "VOUT", "reference": "GND"})
	return provider.expansion(request, "adjustable_linear_regulator", parts, bindings, []CalculationEvidence{calculation}, 1)
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
	first, issues := SolveRCPole(RCPoleRequest{ID: "filter_stage_1_pole", TargetFrequencyHz: frequency, TargetTolerancePercent: tolerance, FixedResistanceOhm: 10000, FixedTolerancePercent: 0.1, SelectedTolerancePercent: 1, SelectedSeries: SeriesE96})
	if len(issues) != 0 {
		return nil, fmt.Errorf("first filter stage failed: %s", issues[0].Message)
	}
	second := first
	second.ID = "filter_stage_2_pole"
	second, err = FinalizeCalculation(second)
	if err != nil {
		return nil, fmt.Errorf("finalize second filter stage: %w", err)
	}
	passives := []passivePart{{"stage_1_r1", "resistor", "filter"}, {"stage_1_r2", "resistor", "filter"}, {"stage_1_c1", "capacitor", "filter"}, {"stage_1_c2", "capacitor", "filter"}, {"stage_2_r1", "resistor", "filter"}, {"stage_2_r2", "resistor", "filter"}, {"stage_2_c1", "capacitor", "filter"}, {"stage_2_c2", "capacitor", "filter"}, {"supply_bypass", "capacitor", "decoupling"}}
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
	singleBindings := bindRoles(request.Ports, singleOpamp.selected.InstanceID, map[string]string{"input": "IN_PLUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	dualExpansion, err := provider.expansion(request, "dual_opamp_sallen_key_cascade", dualParts, dualBindings, []CalculationEvidence{first, second}, 2)
	if err != nil {
		return nil, err
	}
	singleExpansion, err := provider.expansion(request, "two_single_opamp_sallen_key_cascade", singleParts, singleBindings, []CalculationEvidence{first, second}, 2)
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
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"side_a_pullup", "resistor", "bus_pullup"}, {"side_b_pullup", "resistor", "bus_pullup"}, {"output_enable_pulldown", "resistor", "power_up_default"}, {"vcca_bypass", "capacitor", "decoupling"}, {"vccb_bypass", "capacitor", "decoupling"}})
	if err != nil {
		return nil, err
	}
	functions := map[string]string{"side_a": "SDA1", "side_b": "SDA2", "reference": "GND"}
	if voltageA <= voltageB {
		functions["power_a"], functions["power_b"] = "VCCA", "VCCB"
	} else {
		functions["power_a"], functions["power_b"] = "VCCB", "VCCA"
		functions["side_a"], functions["side_b"] = "SDA2", "SDA1"
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, functions)
	return provider.expansion(request, "bidirectional_open_drain_translator", parts, bindings, nil, 1)
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
	selection, err := provider.selectComponent(ctx, family, searchText, []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supply), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	functions := map[string]string{busRole: busFunction}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, functions)
	return provider.expansion(request, usage, []catalogPart{selection}, bindings, nil, 1)
}

type catalogPart struct {
	selected SelectedComponent
	record   components.ComponentRecord
	usage    string
	evidence ContractEvidence
}

type passivePart struct{ id, family, usage string }

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
		parts = append(parts, part)
	}
	return parts, nil
}

func (provider *CatalogProvider) expansion(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, calculations []CalculationEvidence, unproven int) ([]ProviderExpansion, error) {
	instances := make([]RealizationInstance, 0, len(parts))
	componentsSelected := make([]SelectedComponent, 0, len(parts))
	for _, part := range parts {
		componentsSelected = append(componentsSelected, part.selected)
		instances = append(instances, RealizationInstance{ID: part.selected.InstanceID, CatalogID: part.selected.CatalogID, VariantID: part.selected.VariantID, Usage: part.usage})
	}
	parameters := calculationParameters(calculations)
	payload, err := MarshalFragmentRealization(FragmentRealization{Capability: request.Capability, Instances: instances, PortBindings: bindings, Parameters: parameters})
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
