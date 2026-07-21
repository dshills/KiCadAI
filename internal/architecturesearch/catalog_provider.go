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
	"kicadai/internal/simmodel"
)

const (
	catalogProviderRevision         = "1.0.0"
	thresholdReferenceResistanceOhm = 10_000.0
	catalogRatingDeratingFactor     = 0.8
	lowVoltageGateDriveCeilingV     = 5.5
)

var catalogProviderCapabilities = []string{
	"class_a_amplification",
	"class_ab_bias_control",
	"class_ab_output_stage",
	"current_sensing",
	"environment_sensor",
	"fault_indication",
	"frequency_filter",
	"galvanic_isolation",
	"instrumentation_amplification",
	"load_switch",
	"logic_level_translation",
	"mute_control",
	"output_protection",
	"programmable_controller",
	"safety_interlock",
	"signal_amplification",
	"split_supply_generation",
	"threshold_detection",
	"transient_protection",
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
		if _, legacy := namedConstraint(request.Constraints, "output_polarity"); !legacy {
			return provider.expandGenericThreshold(ctx, request)
		}
		return provider.expandThreshold(ctx, request)
	case "load_switch":
		if _, legacy := namedConstraint(request.Constraints, "load_characteristic"); !legacy {
			return provider.expandGenericLoadSwitch(ctx, request)
		}
		return provider.expandLoadSwitch(ctx, request)
	case "voltage_regulation":
		if _, legacy := namedConstraint(request.Constraints, "adjustable_output"); !legacy {
			return provider.expandGenericRegulator(ctx, request)
		}
		return provider.expandRegulator(ctx, request)
	case "frequency_filter":
		if _, legacy := namedConstraint(request.Constraints, "approximation"); !legacy {
			return provider.expandGenericFilter(ctx, request)
		}
		return provider.expandFilter(ctx, request)
	case "logic_level_translation":
		if _, legacy := namedConstraint(request.Constraints, "signaling_mode"); !legacy {
			return provider.expandGenericTranslator(ctx, request)
		}
		return provider.expandTranslator(ctx, request)
	case "fault_indication":
		return provider.expandFaultIndication(ctx, request)
	case "safety_interlock":
		return provider.expandSafetyInterlock(ctx, request)
	case "transient_protection":
		return provider.expandTransientProtection(ctx, request)
	case "output_protection":
		return provider.expandOutputProtection(ctx, request)
	case "signal_amplification", "instrumentation_amplification":
		return provider.expandSignalAmplification(ctx, request)
	case "current_sensing":
		return provider.expandCurrentSensing(ctx, request)
	case "mute_control":
		return provider.expandMuteControl(ctx, request)
	case "class_ab_bias_control":
		return provider.expandClassABBias(ctx, request)
	case "class_ab_output_stage":
		return provider.expandClassABOutput(ctx, request)
	case "class_a_amplification":
		return provider.expandClassAAmplification(ctx, request)
	case "split_supply_generation":
		return provider.expandSplitSupply(ctx, request)
	case "galvanic_isolation":
		return provider.expandGalvanicIsolation(ctx, request)
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
	polarityConstraint, ok := namedConstraint(request.Constraints, "output_polarity")
	if !ok || polarityConstraint.Relation != "equal" {
		return nil, fmt.Errorf("threshold output polarity must be declared")
	}
	var outputPolarity string
	if err := json.Unmarshal(polarityConstraint.Value, &outputPolarity); err != nil {
		return nil, fmt.Errorf("threshold output polarity is invalid")
	}
	outputPolarity = canonicalIdentifier(outputPolarity)
	if outputPolarity != "active_low" && outputPolarity != "active_high" {
		return nil, fmt.Errorf("threshold output polarity must be active_low or active_high")
	}
	if err := requireBool(request.Constraints, "inactive_at_power_up", "required", true); err != nil {
		return nil, err
	}
	delay, _, err := requiredNumber(request.Constraints, "propagation_delay", "maximum", "us")
	if err != nil || delay <= 0 {
		return nil, fmt.Errorf("threshold propagation-delay constraint is invalid")
	}
	supplyMinimum, supplyMaximum, supplyRangeOK := roleVoltageRange(request.Ports, "power")
	if !supplyRangeOK {
		supplyMaximum = roleVoltageMaximum(request.Ports, "power")
		supplyMinimum = supplyMaximum
	}
	supply := .5 * (supplyMinimum + supplyMaximum)
	selection, err := provider.selectComponent(ctx, "comparator", "open_collector", []components.RequiredRating{{Kind: "supply_voltage", Value: numericString(supplyMaximum), Unit: "V"}}, true)
	if err != nil {
		return nil, err
	}
	observedDelay, ok := recordRatingMaximum(selection.record, "propagation_delay", "us")
	if !ok || observedDelay > delay {
		return nil, fmt.Errorf("selected comparator lacks propagation-delay evidence within %.9g us", delay)
	}
	response, err := ObservedCalculation("threshold_response", NamedQuantity{Name: "response_time", Value: observedDelay, Unit: "us"})
	if err != nil {
		return nil, err
	}
	referenceSupply := supplyMaximum
	hysteresisOutputHigh := supplyMaximum
	var referenceRegulator catalogPart
	useStableReference := thresholdRequiresStableReference(supplyMinimum, supplyMaximum, centerTolerance)
	if useStableReference {
		candidate, selectionErr := provider.selectComponent(ctx, "regulator", "AP2127R-3.3", []components.RequiredRating{
			{Kind: "input_voltage", Value: numericString(supplyMinimum), Unit: "V"},
			{Kind: "input_voltage", Value: numericString(supplyMaximum), Unit: "V"},
			{Kind: "output_current", Value: "0.001", Unit: "A"},
		}, true)
		if selectionErr == nil {
			candidateSupply, parameterOK := catalogSimulationParameter(candidate.record, "output_voltage_v")
			if parameterOK && candidateSupply > center && supplyMinimum-candidateSupply >= 0.3 {
				referenceRegulator = candidate
				referenceRegulator.selected.InstanceID = "threshold_reference_regulator"
				referenceRegulator.usage = "threshold_voltage_reference"
				referenceSupply = candidateSupply
				hysteresisOutputHigh = supply
			} else {
				useStableReference = false
			}
		} else {
			useStableReference = false
		}
	}
	calculation, issues := SolveHysteresis(HysteresisRequest{
		ID: "threshold_hysteresis", TargetCenterV: center, CenterTolerancePercent: centerTolerance,
		TargetWidthV: width, WidthTolerancePercent: widthTolerance, OutputLowV: 0.1,
		OutputHighV: hysteresisOutputHigh, OutputUncertaintyV: 0.01, ReferenceResistanceOhm: thresholdReferenceResistanceOhm,
		ReferenceTolerancePercent: 0.1, FeedbackTolerancePercent: 0.1,
		ReferenceVoltageTolerancePercent: 0.1, MinimumReferenceVoltageV: 0,
		MaximumReferenceVoltageV: referenceSupply, FeedbackSeries: SeriesE96,
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("threshold value solution failed: %s", issues[0].Message)
	}
	feedbackResistance, ok := calculationSelectedValue(calculation, "feedback_resistance")
	if !ok {
		return nil, fmt.Errorf("threshold solution omitted feedback resistance")
	}
	referenceVoltage, ok := calculationOutput(calculation, "reference_voltage")
	if !ok || referenceVoltage <= 0 || referenceVoltage >= referenceSupply {
		return nil, fmt.Errorf("threshold solution produced an invalid reference voltage")
	}
	referenceUpper := thresholdReferenceResistanceOhm * referenceSupply / referenceVoltage
	referenceLower := thresholdReferenceResistanceOhm / (1 - referenceVoltage/referenceSupply)
	parts := []catalogPart{selection}
	supplyBypassValue := "100n"
	if useStableReference {
		supplyBypassValue = "1u"
	}
	passives := []passivePart{
		{"threshold_upper", "resistor", "threshold_reference", engineeringValue(referenceUpper, "Ohm")},
		{"threshold_lower", "resistor", "threshold_reference", engineeringValue(referenceLower, "Ohm")},
		{"feedback_resistor", "resistor", "positive_feedback", engineeringValue(feedbackResistance, "Ohm")},
		{"output_pullup", "resistor", "open_collector_pullup", "4.7k"},
		{"supply_bypass", "capacitor", "decoupling", supplyBypassValue},
	}
	if useStableReference {
		parts = append(parts, referenceRegulator)
		passives = append(passives,
			passivePart{"reference_output_bypass", "capacitor", "reference_output_decoupling", "1u"},
		)
	}
	parts, err = provider.appendPassiveParts(ctx, parts, passives)
	if err != nil {
		return nil, err
	}
	var outputInverter catalogPart
	if outputPolarity == "active_high" {
		outputInverter, err = provider.selectComponentWithTemperature(ctx, "bjt", "npn", nil, true, temperatureRequirementFromConstraints(request.Constraints))
		if err != nil {
			return nil, err
		}
		outputInverter.selected.InstanceID = "output_inverter"
		outputInverter.usage = "active_high_output_buffer"
		parts = append(parts, outputInverter)
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{
			{"inverter_base", "resistor", "output_inverter_base", "10k"},
			{"inverted_output_pullup", "resistor", "active_high_output_pullup", "4.7k"},
		})
		if err != nil {
			return nil, err
		}
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"sense": "IN_MINUS", "input": "IN_MINUS", "output": "OUT", "power": "V_PLUS", "reference": "V_MINUS"})
	if outputPolarity == "active_high" {
		for index := range bindings {
			if bindings[index].Role == "output" {
				bindings[index].Instance = outputInverter.selected.InstanceID
				bindings[index].Function = "COLLECTOR"
			}
		}
	}
	thresholdPowerEndpoints := []RealizationEndpoint{endpoint(selection, "V_PLUS"), passiveEndpoint("output_pullup", "A"), passiveEndpoint("supply_bypass", "A")}
	thresholdGroundEndpoints := []RealizationEndpoint{endpoint(selection, "V_MINUS"), passiveEndpoint("threshold_lower", "B"), passiveEndpoint("supply_bypass", "B")}
	if !useStableReference {
		thresholdPowerEndpoints = append(thresholdPowerEndpoints, passiveEndpoint("threshold_upper", "A"))
	} else {
		thresholdPowerEndpoints = append(thresholdPowerEndpoints, endpoint(referenceRegulator, "VIN"))
		thresholdGroundEndpoints = append(thresholdGroundEndpoints, endpoint(referenceRegulator, "GND"), passiveEndpoint("reference_output_bypass", "B"))
	}
	connections := []RealizationConnection{
		semanticNet("threshold_reference", "analog_signal", endpoint(selection, "IN_PLUS"), passiveEndpoint("threshold_upper", "B"), passiveEndpoint("threshold_lower", "A"), passiveEndpoint("feedback_resistor", "B")),
		semanticNet("threshold_output", "open_collector_signal", endpoint(selection, "OUT"), passiveEndpoint("output_pullup", "B"), passiveEndpoint("feedback_resistor", "A")),
		semanticNet("threshold_power", "power", thresholdPowerEndpoints...),
		semanticNet("threshold_ground", "reference", thresholdGroundEndpoints...),
	}
	if useStableReference {
		connections = append(connections, semanticNet("threshold_reference_supply", "power", endpoint(referenceRegulator, "VOUT"), passiveEndpoint("threshold_upper", "A"), passiveEndpoint("reference_output_bypass", "A")))
	}
	if outputPolarity == "active_high" {
		connections[1].Endpoints = append(connections[1].Endpoints, passiveEndpoint("inverter_base", "A"))
		connections[2].Endpoints = append(connections[2].Endpoints, passiveEndpoint("inverted_output_pullup", "A"))
		connections[3].Endpoints = append(connections[3].Endpoints, endpoint(outputInverter, "EMITTER"))
		connections = append(connections,
			semanticNet("threshold_inverter_base", "logic_drive", passiveEndpoint("inverter_base", "B"), endpoint(outputInverter, "BASE")),
			semanticNet("threshold_active_high_output", "logic_signal", endpoint(outputInverter, "COLLECTOR"), passiveEndpoint("inverted_output_pullup", "B")),
		)
	}
	repairs := []RealizationRepairVariable{{
		ID: "threshold_feedback_resistance", Kind: "passive_value", Instance: "feedback_resistor", Value: feedbackResistance,
		AllowedValues: preferredRepairValues(feedbackResistance), Unit: "Ohm",
		Effects: []RealizationRepairEffect{{Analysis: "dc_operating_point", Metric: "hysteresis_voltage", Direction: "metric_decreases"}},
	}}
	return provider.expansionWithRepairs(request, "open_collector_hysteresis", parts, bindings, connections, []CalculationEvidence{calculation, response}, repairs, 0)
}

func (provider *CatalogProvider) expandLoadSwitch(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "load_characteristic", "equal", "inductive"); err != nil {
		return nil, err
	}
	controlState, ok := namedConstraint(request.Constraints, "control_active_state")
	if !ok || controlState.Relation != "equal" {
		return nil, fmt.Errorf("load-switch control active state must be declared")
	}
	var controlActiveState string
	if err := json.Unmarshal(controlState.Value, &controlActiveState); err != nil || (controlActiveState != "high" && controlActiveState != "high_disconnect") {
		return nil, fmt.Errorf("load-switch control active state must be high or high_disconnect")
	}
	failSafeDisconnect := controlActiveState == "high_disconnect"
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
	temperatureRequirement := temperatureRequirementFromConstraints(request.Constraints)
	controlMaximum := roleVoltageMaximum(request.Ports, "control")
	if offState, exists := namedConstraint(request.Constraints, "off_output_state"); exists && constraintStringEquals(offState, "low") {
		return provider.expandHighSideLoadSwitch(ctx, request, voltage, current, temperatureRequirement)
	}
	mosfetQuery := "logic_level"
	if controlMaximum > 0 && controlMaximum <= lowVoltageGateDriveCeilingV {
		mosfetQuery = "low_voltage_gate_drive"
	}
	selection, err := provider.selectComponentWithTemperature(ctx, "mosfet", mosfetQuery, []components.RequiredRating{
		{Kind: "drain_source_voltage", Value: numericString(voltage / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "drain_current", Value: numericString(current / catalogRatingDeratingFactor), Unit: "A"},
	}, true, temperatureRequirement)
	if err != nil {
		return nil, err
	}
	// The flyback is reverse-biased at the steady-state operating point, so its
	// nominal dissipation requirement is zero. A thermal workflow still requires
	// a complete package path; the coupled electrical solve supplies the actual
	// leakage/conduction loss used by the thermal assertion.
	flyback, err := provider.selectComponentWithRequirements(ctx, "diode", "flyback", []components.RequiredRating{
		{Kind: "current", Value: numericString(current / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "reverse_voltage", Value: numericString(voltage / catalogRatingDeratingFactor), Unit: "V"},
	}, true, temperatureRequirement, thermalRequirementFromConstraints(request.Constraints, 0))
	if err != nil {
		return nil, err
	}
	flyback.selected.InstanceID = "flyback_clamp"
	flyback.usage = "inductive_transient_clamp"
	controlClamp, err := provider.selectComponentWithTemperature(ctx, "diode", "zener", []components.RequiredRating{{Kind: "reverse_voltage", Value: numericString(controlMaximum), Unit: "V"}}, true, temperatureRequirement)
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
		{Kind: "drain_source_voltage", Required: voltage, Rated: ratedVoltage, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: selection.evidence},
		{Kind: "drain_current", Required: current, Rated: ratedCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: selection.evidence},
		{Kind: "gate_source_voltage", Required: controlMaximum, Rated: gateRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: selection.evidence},
		{Kind: "flyback_reverse_voltage", Required: voltage, Rated: flybackVoltage, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: flyback.evidence},
		{Kind: "flyback_current", Required: current, Rated: flybackCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: flyback.evidence},
		{Kind: "control_clamp_voltage", Required: controlMaximum, Rated: clampVoltage, DeratingFactor: 1, Unit: "V", Evidence: controlClamp.evidence},
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("switch rating solution failed: %s", issues[0].Message)
	}
	gateCharge, gateChargeOK := recordValueMaximum(selection.record, "total_gate_charge", "C")
	if !gateChargeOK {
		gateCharge, gateChargeOK = recordValue(selection.record, "total_gate_charge", "C")
	}
	if !gateChargeOK || controlMaximum <= 0 {
		return nil, fmt.Errorf("selected switch lacks gate-charge or control-voltage response evidence")
	}
	gateResponse, err := ObservedCalculation("switch_response", NamedQuantity{Name: "response_time", Value: gateCharge * 100 / controlMaximum, Unit: "s"})
	if err != nil {
		return nil, err
	}
	parts := []catalogPart{selection, flyback, controlClamp}
	hasLogicPower := hasRoleContract(request.Ports, "logic_power")
	gateSeries := passivePart{"gate_series", "resistor", "gate_drive", "100"}
	if failSafeDisconnect {
		gateSeries = passivePart{"gate_series", "resistor", "gate_buffer_input", "10k"}
	}
	passives := []passivePart{gateSeries}
	var controlInverter, gateInverter catalogPart
	if hasLogicPower || failSafeDisconnect {
		gateInverter, err = provider.selectComponentWithTemperature(ctx, "bjt", "npn", nil, true, temperatureRequirement)
		if err != nil {
			return nil, err
		}
		gateInverter.selected.InstanceID = "gate_inverter"
		gateInverter.usage = "gate_control_inverter"
		parts = append(parts, gateInverter)
		passives = append(passives, passivePart{"gate_drive_pullup", "resistor", "gate_drive_pullup", "4.7k"})
		if !failSafeDisconnect {
			passives = append(passives, passivePart{"gate_inverter_base", "resistor", "gate_buffer_interstage", "10k"})
		}
		if hasLogicPower {
			passives = append(passives, passivePart{"logic_bypass", "capacitor", "logic_supply_decoupling", "100n"})
		}
		if !failSafeDisconnect {
			controlInverter = gateInverter
			controlInverter.selected.InstanceID = "control_inverter"
			controlInverter.usage = "active_high_gate_buffer_stage_1"
			gateInverter.usage = "active_high_gate_buffer_stage_2"
			parts = append(parts, controlInverter)
			passives = append(passives, passivePart{"control_inverter_base", "resistor", "gate_buffer_input", "10k"}, passivePart{"control_inverter_pullup", "resistor", "gate_buffer_stage_pullup", "4.7k"})
		}
	} else {
		passives = append(passives, passivePart{"gate_pulldown", "resistor", "default_off", "100k"})
	}
	parts, err = provider.appendPassiveParts(ctx, parts, passives)
	if err != nil {
		return nil, err
	}
	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"control": "GATE", "load": "DRAIN", "output": "DRAIN", "reference": "SOURCE"})
	for index := range bindings {
		switch bindings[index].Role {
		case "control":
			if failSafeDisconnect {
				bindings[index].Instance, bindings[index].Function = "gate_series", "A"
			} else if hasLogicPower {
				bindings[index].Instance, bindings[index].Function = "control_inverter_base", "A"
			} else {
				bindings[index].Instance, bindings[index].Function = "gate_series", "A"
			}
		case "load_power":
			bindings[index].Instance, bindings[index].Function = flyback.selected.InstanceID, "K"
		case "power":
			bindings[index].Instance, bindings[index].Function = flyback.selected.InstanceID, "K"
		case "logic_power":
			bindings[index].Instance, bindings[index].Function = "logic_bypass", "A"
		}
	}
	connections := []RealizationConnection{
		semanticNet("switch_load", "switched_power", endpoint(selection, "DRAIN"), endpoint(flyback, "A")),
		semanticNet("switch_load_power", "power", endpoint(flyback, "K")),
	}
	if hasLogicPower {
		if failSafeDisconnect {
			connections = append(connections,
				semanticNet("switch_gate_inverter_base", "logic_drive", passiveEndpoint("gate_series", "B"), endpoint(gateInverter, "BASE")),
				semanticNet("switch_gate", "control", endpoint(gateInverter, "COLLECTOR"), passiveEndpoint("gate_drive_pullup", "B"), endpoint(selection, "GATE"), endpoint(controlClamp, "K")),
			)
		} else {
			connections = append(connections, semanticNet("switch_gate_inverter_base", "logic_drive", passiveEndpoint("gate_inverter_base", "B"), endpoint(gateInverter, "BASE")), semanticNet("switch_gate_drive", "control", endpoint(gateInverter, "COLLECTOR"), passiveEndpoint("gate_drive_pullup", "B"), passiveEndpoint("gate_series", "A")), semanticNet("switch_gate", "control", endpoint(selection, "GATE"), passiveEndpoint("gate_series", "B"), endpoint(controlClamp, "K")))
		}
		logicPowerEndpoints := []RealizationEndpoint{passiveEndpoint("gate_drive_pullup", "A"), passiveEndpoint("logic_bypass", "A")}
		groundEndpoints := []RealizationEndpoint{endpoint(selection, "SOURCE"), endpoint(controlClamp, "A"), endpoint(gateInverter, "EMITTER"), passiveEndpoint("logic_bypass", "B")}
		if !failSafeDisconnect {
			connections = append(connections, semanticNet("switch_control_inverter_base", "logic_drive", passiveEndpoint("control_inverter_base", "B"), endpoint(controlInverter, "BASE")), semanticNet("switch_gate_buffer_stage", "logic_drive", endpoint(controlInverter, "COLLECTOR"), passiveEndpoint("control_inverter_pullup", "B"), passiveEndpoint("gate_inverter_base", "A")))
			logicPowerEndpoints = append(logicPowerEndpoints, passiveEndpoint("control_inverter_pullup", "A"))
			groundEndpoints = append(groundEndpoints, endpoint(controlInverter, "EMITTER"))
		}
		connections = append(connections, semanticNet("switch_logic_power", "power", logicPowerEndpoints...), semanticNet("switch_ground", "reference", groundEndpoints...))
	} else if failSafeDisconnect {
		for index := range connections {
			if connections[index].ID == "switch_load_power" {
				connections[index].Endpoints = append(connections[index].Endpoints, passiveEndpoint("gate_drive_pullup", "A"))
				break
			}
		}
		connections = append(connections,
			semanticNet("switch_gate_inverter_base", "logic_drive", passiveEndpoint("gate_series", "B"), endpoint(gateInverter, "BASE")),
			semanticNet("switch_gate", "control", endpoint(gateInverter, "COLLECTOR"), passiveEndpoint("gate_drive_pullup", "B"), endpoint(selection, "GATE"), endpoint(controlClamp, "K")),
			semanticNet("switch_ground", "reference", endpoint(selection, "SOURCE"), endpoint(controlClamp, "A"), endpoint(gateInverter, "EMITTER")),
		)
	} else {
		connections = append(connections,
			semanticNet("switch_gate", "control", endpoint(selection, "GATE"), passiveEndpoint("gate_series", "B"), passiveEndpoint("gate_pulldown", "A"), endpoint(controlClamp, "K")),
			semanticNet("switch_ground", "reference", endpoint(selection, "SOURCE"), endpoint(controlClamp, "A"), passiveEndpoint("gate_pulldown", "B")),
		)
	}
	connections = retainSemanticNets(connections)
	return provider.expansion(request, "protected_low_side_switch", parts, bindings, connections, []CalculationEvidence{calculation, gateResponse}, 0)
}

func (provider *CatalogProvider) expandHighSideLoadSwitch(ctx context.Context, request ProviderRequest, voltage, current float64, temperatureRequirement *components.TemperatureRequirement) ([]ProviderExpansion, error) {
	selection, err := provider.selectComponentWithTemperature(ctx, "mosfet", "p_channel", []components.RequiredRating{
		{Kind: "drain_source_voltage", Value: numericString(voltage / catalogRatingDeratingFactor), Unit: "V"},
		{Kind: "drain_current", Value: numericString(current / catalogRatingDeratingFactor), Unit: "A"},
	}, true, temperatureRequirement)
	if err != nil {
		return nil, err
	}
	selection.selected.InstanceID, selection.usage = "high_side_switch", "default_off_high_side_switch"
	flyback, err := provider.selectComponentWithRequirements(ctx, "diode", "flyback", []components.RequiredRating{
		{Kind: "current", Value: numericString(current / catalogRatingDeratingFactor), Unit: "A"},
		{Kind: "reverse_voltage", Value: numericString(voltage / catalogRatingDeratingFactor), Unit: "V"},
	}, true, temperatureRequirement, thermalRequirementFromConstraints(request.Constraints, 0))
	if err != nil {
		return nil, err
	}
	flyback.selected.InstanceID, flyback.usage = "flyback_clamp", "inductive_transient_clamp"
	driver, err := provider.selectComponentWithTemperature(ctx, "bjt", "npn", []components.RequiredRating{{Kind: "collector_emitter_voltage", Value: numericString(voltage / catalogRatingDeratingFactor), Unit: "V"}}, true, temperatureRequirement)
	if err != nil {
		return nil, err
	}
	driver.selected.InstanceID, driver.usage = "high_side_gate_driver", "source_referenced_gate_sink"

	ratedVoltage, okVoltage := recordRatingMaximum(selection.record, "drain_source_voltage", "V")
	ratedCurrent, okCurrent := recordRatingMaximum(selection.record, "drain_current", "A")
	gateRated, okGate := recordRatingMaximum(selection.record, "gate_source_voltage", "V")
	gateOn, okGateOn := catalogSimulationParameter(selection.record, "gate_on_voltage_v")
	flybackVoltage, okFlybackVoltage := recordRatingMaximum(flyback.record, "reverse_voltage", "V")
	flybackCurrent, okFlybackCurrent := recordRatingMaximum(flyback.record, "current", "A")
	if !okVoltage || !okCurrent || !okGate || !okGateOn || !okFlybackVoltage || !okFlybackCurrent {
		return nil, fmt.Errorf("selected high-side switch or protection device lacks normalized rating and gate-drive evidence")
	}

	parts := []catalogPart{selection, flyback, driver}
	passives := []passivePart{{"gate_control_base", "resistor", "gate_drive", "10k"}, {"gate_pullup", "resistor", "default_off", "100k"}}
	gateSeriesResistance := 100.0
	clampVoltage := 0.0
	var clamps []catalogPart
	if voltage > gateRated*catalogRatingDeratingFactor {
		clamp, clampErr := provider.selectComponentWithTemperature(ctx, "diode", "zener", nil, true, temperatureRequirement)
		if clampErr != nil {
			return nil, clampErr
		}
		unitClampVoltage, clampOK := recordValue(clamp.record, "zener_voltage", "V")
		if !clampOK || unitClampVoltage <= 0 {
			return nil, fmt.Errorf("selected high-side gate clamp lacks a positive catalog Zener voltage")
		}
		count := int(math.Ceil(gateOn / unitClampVoltage))
		if count < 1 || count > 4 || float64(count)*unitClampVoltage > gateRated*catalogRatingDeratingFactor {
			return nil, fmt.Errorf("catalog Zener series cannot provide a bounded high-side gate-drive window")
		}
		clampVoltage = float64(count) * unitClampVoltage
		for index := 0; index < count; index++ {
			part := clamp
			part.selected.InstanceID = fmt.Sprintf("control_clamp_%d", index+1)
			part.usage = "series_gate_overvoltage_clamp"
			clamps = append(clamps, part)
			parts = append(parts, part)
		}
		gateSeriesResistance = 10000
		passives = append(passives, passivePart{"gate_sink_series", "resistor", "gate_clamp_current_limit", "10k"})
	} else {
		passives = append(passives, passivePart{"gate_sink_series", "resistor", "gate_stopper", "100"})
	}
	parts, err = provider.appendPassiveParts(ctx, parts, passives)
	if err != nil {
		return nil, err
	}

	gateDriveVoltage := voltage
	if clampVoltage > 0 {
		gateDriveVoltage = clampVoltage
	}
	ratings, issues := EvaluateRatings("high_side_switch_derating", []RatingRequirement{
		{Kind: "drain_source_voltage", Required: voltage, Rated: ratedVoltage, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: selection.evidence},
		{Kind: "drain_current", Required: current, Rated: ratedCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: selection.evidence},
		{Kind: "gate_source_voltage", Required: gateDriveVoltage, Rated: gateRated, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: selection.evidence},
		{Kind: "flyback_reverse_voltage", Required: voltage, Rated: flybackVoltage, DeratingFactor: catalogRatingDeratingFactor, Unit: "V", Evidence: flyback.evidence},
		{Kind: "flyback_current", Required: current, Rated: flybackCurrent, DeratingFactor: catalogRatingDeratingFactor, Unit: "A", Evidence: flyback.evidence},
	})
	if len(issues) != 0 {
		return nil, fmt.Errorf("high-side switch rating solution failed: %s", issues[0].Message)
	}
	gateCharge, gateChargeOK := recordValueMaximum(selection.record, "total_gate_charge", "C")
	if !gateChargeOK {
		gateCharge, gateChargeOK = recordValue(selection.record, "total_gate_charge", "C")
	}
	driveHeadroom := voltage - clampVoltage
	if clampVoltage == 0 {
		driveHeadroom = voltage
	}
	if !gateChargeOK || driveHeadroom <= 0 {
		return nil, fmt.Errorf("selected high-side switch lacks gate-charge or bounded sink-current evidence")
	}
	gateResponse, err := ObservedCalculation("high_side_switch_response", NamedQuantity{Name: "response_time", Value: gateCharge * gateSeriesResistance / driveHeadroom, Unit: "s"})
	if err != nil {
		return nil, err
	}

	bindings := bindRoles(request.Ports, selection.selected.InstanceID, map[string]string{"control": "GATE", "load": "DRAIN", "output": "DRAIN", "power": "SOURCE", "load_power": "SOURCE", "reference": "SOURCE"})
	for index := range bindings {
		switch bindings[index].Role {
		case "control":
			bindings[index].Instance, bindings[index].Function = "gate_control_base", "A"
		case "reference":
			bindings[index].Instance, bindings[index].Function = driver.selected.InstanceID, "EMITTER"
		}
	}
	connections := []RealizationConnection{
		semanticNet("high_side_supply", "power", endpoint(selection, "SOURCE"), passiveEndpoint("gate_pullup", "A")),
		semanticNet("high_side_output", "switched_power", endpoint(selection, "DRAIN"), endpoint(flyback, "K")),
		semanticNet("high_side_reference", "reference", endpoint(flyback, "A"), endpoint(driver, "EMITTER")),
		semanticNet("high_side_driver_base", "logic_drive", passiveEndpoint("gate_control_base", "B"), endpoint(driver, "BASE")),
		semanticNet("high_side_driver_collector", "gate_drive", endpoint(driver, "COLLECTOR"), passiveEndpoint("gate_sink_series", "B")),
		semanticNet("high_side_gate", "control", endpoint(selection, "GATE"), passiveEndpoint("gate_pullup", "B"), passiveEndpoint("gate_sink_series", "A")),
	}
	if len(clamps) != 0 {
		connections[0].Endpoints = append(connections[0].Endpoints, endpoint(clamps[0], "K"))
		connections[5].Endpoints = append(connections[5].Endpoints, endpoint(clamps[len(clamps)-1], "A"))
		for index := 0; index+1 < len(clamps); index++ {
			connections = append(connections, semanticNet(fmt.Sprintf("high_side_gate_clamp_series_%d", index+1), "gate_clamp", endpoint(clamps[index], "A"), endpoint(clamps[index+1], "K")))
		}
	}
	connections = retainSemanticNets(connections)
	return provider.expansion(request, "protected_high_side_switch", parts, bindings, connections, []CalculationEvidence{ratings, gateResponse}, 0)
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
	orderEvidence, err := ObservedCalculation("filter_order", NamedQuantity{Name: "filter_order", Value: 4})
	if err != nil {
		return nil, err
	}
	dualExpansion, err := provider.expansion(request, "dual_opamp_sallen_key_cascade", dualParts, dualBindings, dualConnections, []CalculationEvidence{first, second, orderEvidence}, 0)
	if err != nil {
		return nil, err
	}
	singleExpansion, err := provider.expansion(request, "two_single_opamp_sallen_key_cascade", singleParts, singleBindings, singleConnections, []CalculationEvidence{first, second, orderEvidence}, 0)
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
	pullupResistance := 4700.0
	minimumPullupResistance := 1.0
	maximumPullupResistance := pullupResistance
	if riseTime, _, hasRiseTime := firstNumericConstraint(request.Constraints, "rise_time"); hasRiseTime && riseTime > 0 {
		if _, loadCapacitance, hasLoadCapacitance := numericConstraintBounds(request.Constraints, "load_capacitance"); hasLoadCapacitance && loadCapacitance > 0 {
			// Bound a 10%-to-90% RC rise plus deterministic margin for switch,
			// protection, and interconnect capacitance that shares the bus.
			maximumPullupResistance = riseTime / (2.5 * loadCapacitance)
			if maximumChannelCurrent, currentOK := catalogSimulationParameter(selection.record, "max_channel_current_a"); currentOK && maximumChannelCurrent > 0 {
				minimumPullupResistance = high / (catalogRatingDeratingFactor * maximumChannelCurrent)
			}
			if !finitePositive(maximumPullupResistance) || maximumPullupResistance < minimumPullupResistance {
				return nil, fmt.Errorf("open-drain rise-time and sink-current requirements have no bounded pull-up solution")
			}
			candidates, candidateIssues := PreferredValueCandidates(maximumPullupResistance, SeriesE24, minimumPullupResistance, maximumPullupResistance, 1)
			if len(candidateIssues) != 0 || len(candidates) == 0 {
				return nil, fmt.Errorf("open-drain pull-up preferred-value solution failed")
			}
			pullupResistance = candidates[0]
		}
	}
	parts := []catalogPart{selection}
	pullupValue := engineeringValue(pullupResistance, "Ohm")
	parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{{"side_a_sda_pullup", "resistor", "bus_pullup", pullupValue}, {"side_a_scl_pullup", "resistor", "bus_pullup", pullupValue}, {"side_b_sda_pullup", "resistor", "bus_pullup", pullupValue}, {"side_b_scl_pullup", "resistor", "bus_pullup", pullupValue}, {"vcca_bypass", "capacitor", "decoupling", "100n"}, {"vccb_bypass", "capacitor", "decoupling", "100n"}})
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
	allowedPullups := preferredRepairValues(pullupResistance)
	allowedPullups = slices.DeleteFunc(allowedPullups, func(value float64) bool {
		return value < minimumPullupResistance || value > maximumPullupResistance
	})
	repairs := make([]RealizationRepairVariable, 0, 4)
	for _, instance := range []string{"side_a_sda_pullup", "side_a_scl_pullup", "side_b_sda_pullup", "side_b_scl_pullup"} {
		repairs = append(repairs, RealizationRepairVariable{
			ID: instance + "_resistance", Kind: "passive_value", Instance: instance, Value: pullupResistance, AllowedValues: allowedPullups, Unit: "Ohm",
			Effects: []RealizationRepairEffect{{Analysis: simmodel.AnalysisTransient, Metric: "rise_time", Direction: "metric_increases"}},
		})
	}
	return provider.expansionWithRepairs(request, "bidirectional_open_drain_translator", parts, bindings, connections, nil, repairs, 0)
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
	if family == "mcu" {
		usedFunctions := map[string]bool{}
		for _, function := range functions {
			if function != "" {
				usedFunctions[strings.ToUpper(function)] = true
			}
		}
		for _, port := range request.Ports {
			if functions[port.Role] != "" || port.Contract.Kind == "power" || port.Contract.Kind == "reference" || port.Contract.Kind == "digital_bus" {
				continue
			}
			gpioFunctions := availableGPIOFunctions(selection.record, port.Contract, usedFunctions)
			if len(gpioFunctions) == 0 {
				return nil, fmt.Errorf("programmable controller lacks a capability-compatible GPIO function for role %s (%s)", port.Role, port.Contract.Kind)
			}
			functions[port.Role] = gpioFunctions[0]
			markGPIOFunctionUsed(selection.record, gpioFunctions[0], usedFunctions)
		}
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

func availableGPIOFunctions(record components.ComponentRecord, contract PortContract, used map[string]bool) []string {
	seen := map[string]bool{}
	var functions []string
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			function := strings.ToUpper(strings.TrimSpace(pin.Function))
			if !gpioPinNamed(pin) || seen[function] || gpioPinUsed(pin, used) || !gpioPinSupportsContract(pin, contract) {
				continue
			}
			seen[function] = true
			functions = append(functions, function)
		}
	}
	slices.Sort(functions)
	return functions
}

func gpioPinNamed(pin components.FunctionPin) bool {
	if isGPIOPinName(pin.Function) {
		return true
	}
	return slices.ContainsFunc(pin.Aliases, isGPIOPinName)
}

func isGPIOPinName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if strings.HasPrefix(name, "GPIO") {
		return true
	}
	if len(name) < 2 || name[0] != 'P' {
		return false
	}
	if name[1] >= '0' && name[1] <= '9' {
		return true
	}
	return len(name) >= 3 && name[1] >= 'A' && name[1] <= 'Z' && name[2] >= '0' && name[2] <= '9'
}

func gpioPinUsed(pin components.FunctionPin, used map[string]bool) bool {
	if used[strings.ToUpper(strings.TrimSpace(pin.Function))] {
		return true
	}
	return slices.ContainsFunc(pin.Aliases, func(alias string) bool { return used[strings.ToUpper(strings.TrimSpace(alias))] })
}

func markGPIOFunctionUsed(record components.ComponentRecord, function string, used map[string]bool) {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if !strings.EqualFold(pin.Function, function) {
				continue
			}
			used[strings.ToUpper(strings.TrimSpace(pin.Function))] = true
			for _, alias := range pin.Aliases {
				used[strings.ToUpper(strings.TrimSpace(alias))] = true
			}
			return
		}
	}
}

func gpioPinSupportsContract(pin components.FunctionPin, contract PortContract) bool {
	capabilities := map[string]bool{"gpio": gpioPinNamed(pin)}
	for _, alias := range pin.Aliases {
		alias = strings.ToUpper(strings.TrimSpace(alias))
		switch {
		case strings.HasPrefix(alias, "ADC"):
			capabilities["adc"] = true
		case strings.HasPrefix(alias, "DAC"):
			capabilities["dac"] = true
		case strings.HasPrefix(alias, "PWM") || strings.HasPrefix(alias, "OC"):
			capabilities["pwm"] = true
		case strings.HasPrefix(alias, "I2C_SDA"):
			capabilities["i2c_sda"] = true
		case strings.HasPrefix(alias, "I2C_SCL"):
			capabilities["i2c_scl"] = true
		}
	}
	required := map[string]bool{}
	switch canonicalIdentifier(contract.Kind) {
	case "digital_logic":
		required["gpio"] = true
	case "analog_voltage":
		if contract.Direction == "sink" {
			required["adc"] = true
		} else {
			required["dac"] = true
		}
	case "analog_control":
		if contract.Direction == "sink" {
			required["adc"] = true
		} else {
			required["pwm"] = true
		}
	default:
		return false
	}
	for _, trait := range append(append([]string(nil), contract.Traits...), contract.RequiredTraits...) {
		switch canonicalIdentifier(trait) {
		case "adc", "analog_input":
			required["adc"] = true
		case "dac", "analog_output":
			required["dac"] = true
		case "pwm", "pwm_output":
			required["pwm"] = true
		}
	}
	for capability := range required {
		if !capabilities[capability] {
			return false
		}
	}
	return true
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
	return provider.selectComponentWithRequirements(ctx, family, text, ratings, concrete, nil, nil)
}

func (provider *CatalogProvider) selectComponentWithThermal(ctx context.Context, family, text string, ratings []components.RequiredRating, concrete bool, thermal *components.ThermalRequirement) (catalogPart, error) {
	return provider.selectComponentWithRequirements(ctx, family, text, ratings, concrete, nil, thermal)
}

func (provider *CatalogProvider) selectComponentWithTemperature(ctx context.Context, family, text string, ratings []components.RequiredRating, concrete bool, temperature *components.TemperatureRequirement) (catalogPart, error) {
	return provider.selectComponentWithRequirements(ctx, family, text, ratings, concrete, temperature, nil)
}

func (provider *CatalogProvider) selectComponentWithRequirements(ctx context.Context, family, text string, ratings []components.RequiredRating, concrete bool, temperature *components.TemperatureRequirement, thermal *components.ThermalRequirement) (catalogPart, error) {
	selection, result := components.Select(ctx, provider.catalog, components.SelectionRequest{
		Query:      components.Query{Text: text, Family: family, MinimumConfidence: components.ConfidenceRuleInferred, Limit: 64},
		Acceptance: components.AcceptanceStructural, AllowAlternatives: true,
		RequiredRatings: ratings, RequiredTemperature: temperature, RequiredThermal: thermal, RequireConcrete: concrete,
	})
	if !result.OK {
		return catalogPart{}, fmt.Errorf("no catalog-backed %s satisfies normalized ratings: %v", family, result.Issues)
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
	return provider.expansionWithTransitions(request, id, parts, bindings, nil, connections, calculations, unproven)
}

func (provider *CatalogProvider) expansionWithTransitions(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, transitions []RealizationSeriesTransition, connections []RealizationConnection, calculations []CalculationEvidence, unproven int) ([]ProviderExpansion, error) {
	return provider.expansionWithTransitionsAndRepairs(request, id, parts, bindings, transitions, connections, calculations, nil, unproven)
}

func (provider *CatalogProvider) expansionWithRepairs(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, connections []RealizationConnection, calculations []CalculationEvidence, repairs []RealizationRepairVariable, unproven int) ([]ProviderExpansion, error) {
	return provider.expansionWithTransitionsAndRepairs(request, id, parts, bindings, nil, connections, calculations, repairs, unproven)
}

func (provider *CatalogProvider) expansionWithTransitionsAndRepairs(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, transitions []RealizationSeriesTransition, connections []RealizationConnection, calculations []CalculationEvidence, repairs []RealizationRepairVariable, unproven int) ([]ProviderExpansion, error) {
	primary, err := provider.buildCatalogExpansion(request, id, parts, bindings, transitions, connections, calculations, repairs, unproven)
	if err != nil {
		return nil, err
	}
	expansions := []ProviderExpansion{primary}
	for index, part := range parts {
		alternative, ok := provider.catalogAlternativePart(part)
		if !ok {
			continue
		}
		alternativeParts := append([]catalogPart(nil), parts...)
		alternativeParts[index] = alternative
		candidate, err := provider.buildCatalogExpansion(request, id+"_alt", alternativeParts, bindings, transitions, connections, calculations, repairs, unproven)
		if err != nil {
			return nil, err
		}
		expansions = append(expansions, candidate)
		break
	}
	return expansions, nil
}

func (provider *CatalogProvider) buildCatalogExpansion(request ProviderRequest, id string, parts []catalogPart, bindings []RealizationPortBinding, transitions []RealizationSeriesTransition, connections []RealizationConnection, calculations []CalculationEvidence, repairs []RealizationRepairVariable, unproven int) (ProviderExpansion, error) {
	instances := make([]RealizationInstance, 0, len(parts))
	componentsSelected := make([]SelectedComponent, 0, len(parts))
	for _, part := range parts {
		componentsSelected = append(componentsSelected, part.selected)
		instances = append(instances, RealizationInstance{ID: part.selected.InstanceID, CatalogID: part.selected.CatalogID, VariantID: part.selected.VariantID, Usage: part.usage, Value: part.value})
	}
	parameters := calculationParameters(calculations)
	payload, err := MarshalFragmentRealization(FragmentRealization{Capability: request.Capability, Instances: instances, PortBindings: bindings, SeriesTransitions: transitions, Connections: connections, Parameters: parameters, RepairVariables: repairs})
	if err != nil {
		return ProviderExpansion{}, err
	}
	behavior, behaviorUnproven, err := catalogBehaviorCalculations(request, parts)
	if err != nil {
		return ProviderExpansion{}, err
	}
	unproven += behaviorUnproven
	calculations = append(calculations, behavior...)
	powerDemand, powerDemandProven, powerCalculations, err := catalogFragmentPowerDemand(request, parts, bindings, transitions, connections)
	if err != nil {
		return ProviderExpansion{}, err
	}
	calculations = append(calculations, powerCalculations...)
	margin := minimumCalculationMargin(calculations)
	area := catalogPartsAreaMM2(parts)
	metrics := ExpansionMetrics{UnprovenNonSafety: unproven, WorstMargin: &margin}
	if area > 0 {
		metrics.AreaMM2 = float64Pointer(area)
	}
	return ProviderExpansion{
		ID: id, OfferedPorts: offeredCatalogPorts(request, powerDemand, powerDemandProven), Components: componentsSelected,
		Calculations: calculations, Metrics: metrics,
		Evidence: ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:catalog-function-provider:" + catalogProviderRevision}},
		Payload:  payload,
	}, nil
}

func catalogPartsAreaMM2(parts []catalogPart) float64 {
	area := 0.0
	for _, part := range parts {
		for _, variant := range part.record.Packages {
			if variant.ID != part.selected.VariantID || variant.DimensionsMM == nil {
				continue
			}
			area += variant.DimensionsMM.Width * variant.DimensionsMM.Height
			break
		}
	}
	return quantize(area)
}

func (provider *CatalogProvider) catalogAlternativePart(original catalogPart) (catalogPart, bool) {
	if provider.catalog == nil || strings.TrimSpace(original.value) == "" {
		return catalogPart{}, false
	}
	requiredFunctions := catalogRecordFunctions(original.record)
	for _, record := range provider.catalog.Records {
		if record.ID == original.record.ID || record.Family != original.record.Family || !catalogRecordSupportsFunctions(record, requiredFunctions) {
			continue
		}
		valueKind := sharedCatalogValueKind(original.record, record)
		if valueKind == "" {
			continue
		}
		selection, result := components.Select(context.Background(), &components.Catalog{Version: provider.catalog.Version, Records: []components.ComponentRecord{record}}, components.SelectionRequest{
			Query:      components.Query{Family: record.Family, ValueKind: valueKind, Value: original.value, MinimumConfidence: components.ConfidenceRuleInferred, Limit: 8},
			Acceptance: components.AcceptanceStructural, AllowAlternatives: true,
		})
		if !result.OK {
			continue
		}
		evidence := componentEvidence(selection.Component, selection.Candidate.Confidence)
		alternative := original
		alternative.record = selection.Component
		alternative.evidence = evidence
		alternative.selected.CatalogID = selection.Component.ID
		alternative.selected.VariantID = selection.Variant.ID
		alternative.selected.Evidence = evidence.Confidence
		return alternative, true
	}
	return catalogPart{}, false
}

func sharedCatalogValueKind(left components.ComponentRecord, right components.ComponentRecord) string {
	for _, leftValue := range left.Values {
		for _, rightValue := range right.Values {
			if strings.EqualFold(strings.TrimSpace(leftValue.Kind), strings.TrimSpace(rightValue.Kind)) {
				return leftValue.Kind
			}
		}
	}
	return ""
}

func catalogRecordFunctions(record components.ComponentRecord) []string {
	seen := map[string]struct{}{}
	var functions []string
	for _, symbol := range record.Symbols {
		for _, binding := range symbol.FunctionPins {
			key := canonicalIdentifier(binding.Function)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			functions = append(functions, key)
		}
	}
	slices.Sort(functions)
	return functions
}

func catalogRecordSupportsFunctions(record components.ComponentRecord, required []string) bool {
	offered := catalogRecordFunctions(record)
	for _, function := range required {
		if !slices.Contains(offered, function) {
			return false
		}
	}
	return true
}

func offeredCatalogPorts(request ProviderRequest, powerDemandA map[string]float64, powerDemandProven map[string]bool) []RoleContract {
	ports := cloneRoleContractsJSON(request.Ports)
	startupState := ""
	if constraint, ok := namedConstraint(request.Constraints, "startup_state"); ok && constraint.Relation == "equal" {
		_ = json.Unmarshal(constraint.Value, &startupState)
		startupState = canonicalIdentifier(startupState)
	}
	for index := range ports {
		contract := &ports[index].Contract
		contract.ID = ""
		contract.MinimumEvidence = ""
		contract.Evidence = ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:catalog-function-provider:" + catalogProviderRevision}}
		contract.CurrentCapacityA = cloneFloat64(contract.RequiredCurrentCapacityA)
		contract.CurrentDemandA = cloneFloat64(contract.MaximumCurrentDemandA)
		if contract.Direction == "sink" && contract.Kind == "power" && powerDemandProven[ports[index].Role] {
			contract.CurrentDemandA = float64Pointer(powerDemandA[ports[index].Role])
		}
		contract.Traits = append(contract.Traits, contract.RequiredTraits...)
		contract.RequiredTraits = nil
		if startupState != "" && catalogStartupRole(ports[index].Role, *contract) {
			contract.Traits = append(contract.Traits, "startup_state_"+startupState)
			if contract.DefaultState == "" {
				contract.DefaultState = startupState
			}
		}
		if contract.Protocol != nil && canonicalIdentifier(contract.Protocol.Mode) == "open_drain" && contract.Direction != "sink" {
			contract.Traits = append(contract.Traits, "startup_state_released")
			if contract.DefaultState == "" {
				contract.DefaultState = "released"
			}
		}
		if canonicalIdentifier(request.Capability) == "output_protection" && ports[index].Role == "output" {
			contract.Traits = append(contract.Traits, "startup_state_inactive")
			if contract.DefaultState == "" {
				contract.DefaultState = "inactive"
			}
		}
		slices.Sort(contract.Traits)
	}
	return ports
}

func remapOfferedCatalogPorts(request ProviderRequest, existing []RoleContract) []RoleContract {
	ports := offeredCatalogPorts(request, nil, nil)
	for index := range ports {
		if ports[index].Contract.Direction != "sink" || ports[index].Contract.Kind != "power" {
			continue
		}
		for _, offered := range existing {
			if offered.Role == ports[index].Role && offered.Contract.CurrentDemandA != nil {
				ports[index].Contract.CurrentDemandA = cloneFloat64(offered.Contract.CurrentDemandA)
				break
			}
		}
	}
	return ports
}

func catalogStartupRole(role string, contract PortContract) bool {
	role = canonicalIdentifier(role)
	if role == "reference" || role == "power" || role == "positive_power" || role == "negative_power" || role == "load_power" {
		return false
	}
	return contract.Direction != "sink" || role == "output" || role == "load" || role == "permit" || role == "mute" || role == "bias"
}

func catalogBehaviorCalculations(request ProviderRequest, parts []catalogPart) ([]CalculationEvidence, int, error) {
	var result []CalculationEvidence
	unproven := 0
	for _, part := range parts {
		for _, analysis := range requiredCatalogAnalyses(request) {
			if !catalogRecordSupportsAnalysis(part.record, analysis) {
				unproven++
			}
		}
		if part.usage == "regulator" && regulatorThermalEvidenceRequired(request) {
			calculation, proven := catalogRegulatorThermalCalculation(request, part)
			if !proven {
				unproven++
			} else {
				result = append(result, calculation)
			}
		}
		if part.record.OpAmp == nil {
			continue
		}
		if phase, ok := catalogOpAmpPhaseMargin(request, part.record); ok {
			calculation, err := ObservedCalculation(part.selected.InstanceID+"_loop_stability", NamedQuantity{Name: "phase_margin", Value: phase, Unit: "deg"})
			if err != nil {
				return nil, 0, err
			}
			result = append(result, calculation)
		}
		if noise := part.record.OpAmp.VoltageNoiseDensity; noise != nil && noise.Value > 0 {
			calculation, err := ObservedCalculation(part.selected.InstanceID+"_noise", NamedQuantity{Name: "voltage_noise_density", Value: noise.Value, Unit: noise.Unit})
			if err != nil {
				return nil, 0, err
			}
			result = append(result, calculation)
		}
	}
	if canonicalIdentifier(request.Capability) == "class_a_amplification" {
		calculation, err := ObservedCalculation("open_loop_stability", NamedQuantity{Name: "phase_margin", Value: 90, Unit: "deg"})
		if err != nil {
			return nil, 0, err
		}
		result = append(result, calculation)
	}
	return result, unproven, nil
}

func requiredCatalogAnalyses(request ProviderRequest) []string {
	var analyses []string
	for _, constraint := range request.Constraints {
		if constraint.Relation != "required" || !strings.HasPrefix(constraint.Name, "analysis_") {
			continue
		}
		var required bool
		if json.Unmarshal(constraint.Value, &required) != nil || !required {
			continue
		}
		analyses = append(analyses, strings.TrimPrefix(constraint.Name, "analysis_"))
	}
	slices.Sort(analyses)
	return slices.Compact(analyses)
}

func catalogRecordSupportsAnalysis(record components.ComponentRecord, analysis string) bool {
	for _, model := range record.SimulationModels {
		if simmodel.SupportsAnalysis(model.ModelID, analysis) {
			return true
		}
	}
	return false
}

func regulatorThermalEvidenceRequired(request ProviderRequest) bool {
	_, junctionRequired := namedConstraint(request.Constraints, "junction_temperature")
	_, trackingRequired := namedConstraint(request.Constraints, "thermal_tracking")
	return canonicalIdentifier(request.Capability) == "voltage_regulation" && (junctionRequired || trackingRequired)
}

func catalogRegulatorThermalCalculation(request ProviderRequest, part catalogPart) (CalculationEvidence, bool) {
	inputMinimum, inputMaximum, inputOK := roleVoltageRange(request.Ports, "input")
	_ = inputMinimum
	output, _, outputOK := firstNumericConstraint(request.Constraints, "output_voltage", "dc_voltage")
	current, _, currentOK := firstNumericConstraint(request.Constraints, "continuous_output_current", "output_current", "load_current")
	ambient, _, ambientOK := firstNumericConstraint(request.Constraints, "ambient_temperature")
	thermalResistance, thermalOK := catalogSimulationParameter(part.record, "junction_to_ambient_c_per_w")
	maximumTemperature, maximumOK := catalogSimulationParameter(part.record, "max_temperature_c")
	quiescentCurrent, _ := catalogSimulationParameter(part.record, "quiescent_current_a")
	if !inputOK || !outputOK || !currentOK || !ambientOK || !thermalOK || !maximumOK || inputMaximum <= output || current <= 0 || thermalResistance <= 0 || maximumTemperature <= 0 || quiescentCurrent < 0 {
		return CalculationEvidence{}, false
	}
	dissipation := (inputMaximum-output)*current + inputMaximum*quiescentCurrent
	predictedJunction := ambient + dissipation*thermalResistance
	calculation, _ := EvaluateRatings(part.selected.InstanceID+"_thermal", []RatingRequirement{{
		Kind: "junction_temperature", Required: predictedJunction, Rated: maximumTemperature, DeratingFactor: 1, Unit: "degC", Evidence: part.evidence,
	}})
	return calculation, calculation.Hash != ""
}

func catalogSimulationParameter(record components.ComponentRecord, name string) (float64, bool) {
	for _, model := range record.SimulationModels {
		for _, parameter := range model.Parameters {
			if canonicalIdentifier(parameter.Name) == name && finiteNumbers(parameter.Value) {
				return parameter.Value, true
			}
		}
	}
	return 0, false
}

func catalogOpAmpPhaseMargin(request ProviderRequest, record components.ComponentRecord) (float64, bool) {
	if record.OpAmp == nil {
		return 0, false
	}
	gbw := 0.0
	if record.OpAmp.GainBandwidth != nil {
		gbw = record.OpAmp.GainBandwidth.Value
	}
	for _, model := range record.SimulationModels {
		if canonicalIdentifier(model.ModelID) != "mna_opamp_single_pole_v1" {
			continue
		}
		for _, parameter := range model.Parameters {
			if canonicalIdentifier(parameter.Name) == "gain_bandwidth_hz" {
				gbw = math.Max(gbw, parameter.Value)
			}
		}
	}
	if gbw <= 0 {
		return 0, false
	}
	gain := 1.0
	if value, _, ok := firstNumericConstraint(request.Constraints, "voltage_gain"); ok {
		gain = math.Max(value, 1)
	}
	ratio := 10.0
	if value, _, ok := firstNumericConstraint(request.Constraints, "gain_bandwidth_margin"); ok {
		ratio = value
	} else if frequency, _, ok := firstNumericConstraint(request.Constraints, "cutoff_frequency"); ok && frequency > 0 {
		ratio = gbw / (gain * frequency)
	}
	if ratio <= 0 {
		return 0, false
	}
	return math.Max(0, 90-math.Atan(1/ratio)*180/math.Pi), true
}

func catalogFragmentPowerDemand(request ProviderRequest, parts []catalogPart, bindings []RealizationPortBinding, transitions []RealizationSeriesTransition, connections []RealizationConnection) (map[string]float64, map[string]bool, []CalculationEvidence, error) {
	demand := map[string]float64{}
	proven := map[string]bool{}
	var sinkRoles []string
	for _, port := range request.Ports {
		if port.Contract.Direction == "sink" && port.Contract.Kind == "power" {
			sinkRoles = append(sinkRoles, port.Role)
			demand[port.Role] = 0
			proven[port.Role] = true
		}
	}
	if len(sinkRoles) == 0 {
		return demand, proven, nil, nil
	}

	for _, part := range parts {
		genericCurrent, genericOK := recordRatingMaximum(part.record, "supply_current", "A")
		for _, role := range sinkRoles {
			current, roleOK := recordRatingMaximum(part.record, "supply_current_"+canonicalIdentifier(role), "A")
			if roleOK {
				demand[role] += current
			} else if genericOK {
				demand[role] += genericCurrent
			} else if catalogRecordConsumesPower(part.record) && !catalogRecordProvidesPower(part.record) {
				// Retain the request ceiling for only the unevidenced rail. A
				// multi-rail fragment must never borrow evidence from another rail.
				proven[role] = false
			}
		}
	}
	if quiescent, _, ok := firstNumericConstraint(request.Constraints, "quiescent_current"); ok {
		for _, role := range sinkRoles {
			demand[role] += quiescent
		}
	}
	for role, current := range catalogResistorNetworkDemandA(request.Ports, parts, bindings, transitions, connections) {
		demand[role] += current
	}
	directSource, powerSource := sourceCurrentDemandBySinkRole(request.Ports, sinkRoles)
	converted := catalogConvertedPowerDemandA(request.Ports, parts, sinkRoles)
	for _, role := range sinkRoles {
		demand[role] += directSource[role] + math.Max(powerSource[role], converted[role])
	}

	calculations := make([]CalculationEvidence, 0, len(sinkRoles))
	for _, role := range sinkRoles {
		if !proven[role] {
			continue
		}
		demand[role] = quantize(demand[role])
		id := "catalog_power_current_demand"
		if len(sinkRoles) > 1 {
			id += "_" + canonicalIdentifier(role)
		}
		calculation, err := ObservedCalculation(id, NamedQuantity{Name: "power_current_demand", Value: demand[role], Unit: "A"})
		if err != nil {
			return nil, nil, nil, err
		}
		calculations = append(calculations, calculation)
	}
	return demand, proven, calculations, nil
}

func catalogRecordConsumesPower(record components.ComponentRecord) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if canonicalIdentifier(pin.Electrical) == "power_in" {
				return true
			}
		}
	}
	return false
}

func catalogRecordProvidesPower(record components.ComponentRecord) bool {
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if canonicalIdentifier(pin.Electrical) == "power_out" {
				return true
			}
		}
	}
	return false
}

func sourceCurrentDemandBySinkRole(ports []RoleContract, sinkRoles []string) (map[string]float64, map[string]float64) {
	direct := map[string]float64{}
	power := map[string]float64{}
	for _, source := range ports {
		if source.Contract.Direction != "source" || source.Contract.RequiredCurrentCapacityA == nil {
			continue
		}
		var matching []string
		for _, role := range sinkRoles {
			index := slices.IndexFunc(ports, func(port RoleContract) bool { return port.Role == role })
			if index >= 0 && ports[index].Contract.Domain != "" && ports[index].Contract.Domain == source.Contract.Domain {
				matching = append(matching, role)
			}
		}
		if len(matching) == 0 {
			matching = sinkRoles
		}
		for _, role := range matching {
			if source.Contract.Kind == "power" {
				power[role] += *source.Contract.RequiredCurrentCapacityA
			} else {
				direct[role] += *source.Contract.RequiredCurrentCapacityA
			}
		}
	}
	return direct, power
}

func catalogConvertedPowerDemandA(ports []RoleContract, parts []catalogPart, sinkRoles []string) map[string]float64 {
	result := map[string]float64{}
	outputPower := 0.0
	for _, port := range ports {
		contract := port.Contract
		if contract.Kind == "power" && contract.Direction == "source" && contract.RequiredCurrentCapacityA != nil && contract.Voltage.Maximum != nil {
			outputPower += math.Abs(*contract.Voltage.Maximum) * *contract.RequiredCurrentCapacityA
		}
	}
	if outputPower <= 0 {
		return result
	}
	efficiency := 0.0
	for _, part := range parts {
		if value, ok := recordValue(part.record, "efficiency", "%"); ok {
			efficiency = math.Max(efficiency, value/100)
		}
	}
	if efficiency <= 0 || efficiency > 1 {
		return result
	}
	for _, role := range sinkRoles {
		index := slices.IndexFunc(ports, func(port RoleContract) bool { return port.Role == role })
		if index < 0 || ports[index].Contract.Voltage.Minimum == nil || *ports[index].Contract.Voltage.Minimum <= 0 {
			continue
		}
		result[role] = outputPower / (*ports[index].Contract.Voltage.Minimum * efficiency)
	}
	return result
}

func catalogResistorNetworkDemandA(ports []RoleContract, parts []catalogPart, bindings []RealizationPortBinding, transitions []RealizationSeriesTransition, connections []RealizationConnection) map[string]float64 {
	type endpointKey struct{ instance, function string }
	parent := map[endpointKey]endpointKey{}
	var find func(endpointKey) endpointKey
	find = func(key endpointKey) endpointKey {
		root, exists := parent[key]
		if !exists {
			parent[key] = key
			return key
		}
		if root != key {
			parent[key] = find(root)
		}
		return parent[key]
	}
	union := func(left, right endpointKey) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot != rightRoot {
			parent[rightRoot] = leftRoot
		}
	}
	for _, connection := range connections {
		if len(connection.Endpoints) == 0 {
			continue
		}
		first := endpointKey{connection.Endpoints[0].Instance, connection.Endpoints[0].Function}
		find(first)
		for _, endpoint := range connection.Endpoints[1:] {
			union(first, endpointKey{endpoint.Instance, endpoint.Function})
		}
	}
	for _, binding := range bindings {
		find(endpointKey{binding.Instance, binding.Function})
	}

	fixed := map[endpointKey]float64{}
	powerNodes := map[endpointKey][]string{}
	for _, binding := range bindings {
		portIndex := slices.IndexFunc(ports, func(port RoleContract) bool { return port.Role == binding.Role })
		if portIndex < 0 {
			continue
		}
		contract := ports[portIndex].Contract
		root := find(endpointKey{binding.Instance, binding.Function})
		if contract.Kind == "reference" {
			fixed[root] = 0
		} else if contract.Kind == "power" {
			voltage, ok := maximumMagnitudeVoltage(contract.Voltage)
			if !ok {
				continue
			}
			fixed[root] = voltage
			powerNodes[root] = append(powerNodes[root], binding.Role)
		}
	}
	seriesInstances := map[string]struct{}{}
	for _, transition := range transitions {
		seriesInstances[transition.Input.Instance] = struct{}{}
		seriesInstances[transition.Output.Instance] = struct{}{}
	}
	type resistorEdge struct {
		left, right endpointKey
		resistance  float64
	}
	var edges []resistorEdge
	for _, part := range parts {
		if canonicalIdentifier(part.record.Family) != "resistor" {
			continue
		}
		if _, series := seriesInstances[part.selected.InstanceID]; series {
			continue
		}
		resistance, ok := components.ParseEngineeringValue(part.value)
		if !ok || resistance <= 0 {
			continue
		}
		left := find(endpointKey{part.selected.InstanceID, "A"})
		right := find(endpointKey{part.selected.InstanceID, "B"})
		if left != right {
			edges = append(edges, resistorEdge{left: left, right: right, resistance: resistance})
		}
	}
	if len(edges) == 0 || len(fixed) == 0 {
		return map[string]float64{}
	}
	reachable := map[endpointKey]bool{}
	for node := range fixed {
		reachable[node] = true
	}
	for changed := true; changed; {
		changed = false
		for _, edge := range edges {
			if reachable[edge.left] && !reachable[edge.right] {
				reachable[edge.right] = true
				changed = true
			} else if reachable[edge.right] && !reachable[edge.left] {
				reachable[edge.left] = true
				changed = true
			}
		}
	}
	edges = slices.DeleteFunc(edges, func(edge resistorEdge) bool { return !reachable[edge.left] || !reachable[edge.right] })
	if len(edges) == 0 {
		return map[string]float64{}
	}

	nodes := map[endpointKey]int{}
	for _, edge := range edges {
		if _, known := fixed[edge.left]; !known {
			if _, exists := nodes[edge.left]; !exists {
				nodes[edge.left] = len(nodes)
			}
		}
		if _, known := fixed[edge.right]; !known {
			if _, exists := nodes[edge.right]; !exists {
				nodes[edge.right] = len(nodes)
			}
		}
	}
	matrix := make([][]float64, len(nodes))
	rightHand := make([]float64, len(nodes))
	for index := range matrix {
		matrix[index] = make([]float64, len(nodes))
	}
	for _, edge := range edges {
		conductance := 1 / edge.resistance
		leftIndex, leftUnknown := nodes[edge.left]
		rightIndex, rightUnknown := nodes[edge.right]
		if leftUnknown {
			matrix[leftIndex][leftIndex] += conductance
			if rightUnknown {
				matrix[leftIndex][rightIndex] -= conductance
			} else {
				rightHand[leftIndex] += conductance * fixed[edge.right]
			}
		}
		if rightUnknown {
			matrix[rightIndex][rightIndex] += conductance
			if leftUnknown {
				matrix[rightIndex][leftIndex] -= conductance
			} else {
				rightHand[rightIndex] += conductance * fixed[edge.left]
			}
		}
	}
	voltages, ok := solveLinearSystem(matrix, rightHand)
	if !ok {
		return map[string]float64{}
	}
	voltageAt := func(node endpointKey) float64 {
		if voltage, known := fixed[node]; known {
			return voltage
		}
		return voltages[nodes[node]]
	}
	demand := map[string]float64{}
	for _, edge := range edges {
		current := math.Abs(voltageAt(edge.left)-voltageAt(edge.right)) / edge.resistance
		for _, role := range powerNodes[edge.left] {
			demand[role] += current
		}
		for _, role := range powerNodes[edge.right] {
			demand[role] += current
		}
	}
	return demand
}

func maximumMagnitudeVoltage(voltage NumericRange) (float64, bool) {
	if voltage.Minimum == nil && voltage.Maximum == nil {
		return 0, false
	}
	value := 0.0
	if voltage.Minimum != nil {
		value = *voltage.Minimum
	}
	if voltage.Maximum != nil && math.Abs(*voltage.Maximum) >= math.Abs(value) {
		value = *voltage.Maximum
	}
	return value, true
}

func solveLinearSystem(matrix [][]float64, rightHand []float64) ([]float64, bool) {
	if len(matrix) != len(rightHand) {
		return nil, false
	}
	augmented := make([][]float64, len(matrix))
	for row := range matrix {
		if len(matrix[row]) != len(matrix) {
			return nil, false
		}
		augmented[row] = append(append([]float64(nil), matrix[row]...), rightHand[row])
	}
	for pivot := range augmented {
		best := pivot
		for row := pivot + 1; row < len(augmented); row++ {
			if math.Abs(augmented[row][pivot]) > math.Abs(augmented[best][pivot]) {
				best = row
			}
		}
		if math.Abs(augmented[best][pivot]) < 1e-18 {
			return nil, false
		}
		augmented[pivot], augmented[best] = augmented[best], augmented[pivot]
		divisor := augmented[pivot][pivot]
		for column := pivot; column <= len(matrix); column++ {
			augmented[pivot][column] /= divisor
		}
		for row := range augmented {
			if row == pivot {
				continue
			}
			factor := augmented[row][pivot]
			for column := pivot; column <= len(matrix); column++ {
				augmented[row][column] -= factor * augmented[pivot][column]
			}
		}
	}
	result := make([]float64, len(matrix))
	for row := range augmented {
		result[row] = augmented[row][len(matrix)]
	}
	return result, true
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

func recordValueMaximum(record components.ComponentRecord, kind, unit string) (float64, bool) {
	for _, value := range record.Values {
		if value.Kind != kind || value.Max == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(value.Max, 64)
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
	if from == "A" && to == "mA" {
		return value * 1000, true
	}
	if from == "mV" && to == "V" {
		return value / 1000, true
	}
	if from == "V" && to == "mV" {
		return value * 1000, true
	}
	if from == "nC" && to == "C" {
		return value * 1e-9, true
	}
	if from == "C" && to == "nC" {
		return value * 1e9, true
	}
	return 0, false
}

func numericString(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
