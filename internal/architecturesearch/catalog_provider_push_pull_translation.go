package architecturesearch

import (
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

const (
	translatorEnablePulldownOhm = 100_000.0
	translatorControlPullOhm    = 100_000.0
)

func (provider *CatalogProvider) expandPushPullTranslator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "signaling_mode", "equal", "push_pull"); err != nil {
		return nil, err
	}
	direction := optionalMCUConstraintString(request.Constraints, "direction")
	if direction == "bidirectional" {
		return provider.expandDirectionControlledTranslator(ctx, request)
	}
	if direction != "unidirectional" {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "installed push-pull translation requires one explicit source side and one explicit sink side"}
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(request.Ports, func(port RoleContract) bool { return port.Role == "enable" }) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "push-pull translation requires an enable role with a defined inactive startup state"}
	}
	frequency, err := pushPullTranslationFrequency(request)
	if err != nil {
		return nil, err
	}
	channelCount := 1
	if value, _, ok := firstNumericConstraint(request.Constraints, "channel_count", "bus_width"); ok {
		if value < 1 || value != math.Trunc(value) {
			return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "push-pull translation channel count must be a positive integer"}
		}
		channelCount = int(value)
	}
	voltageA, voltageB := roleVoltageMaximum(request.Ports, "power_a"), roleVoltageMaximum(request.Ports, "power_b")
	if !finitePositive(voltageA) || !finitePositive(voltageB) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceVoltageDomainMismatch, message: "push-pull translation requires bounded positive voltage domains on both sides"}
	}
	low, high := math.Min(voltageA, voltageB), math.Max(voltageA, voltageB)
	translator, err := provider.selectPushPullTranslator(ctx, low, high, frequency, direction)
	if err != nil {
		return nil, err
	}
	if translator.record.Translator == nil || translator.record.Translator.ChannelCount <= 0 {
		return nil, &interfaceSynthesisError{
			code:    CodeInterfaceTranslationUnavailable,
			message: "selected push-pull translator lacks a positive catalog-backed channel count",
		}
	}
	channelsPerPart := translator.record.Translator.ChannelCount
	compactCount := int(math.Ceil(float64(channelCount) / float64(channelsPerPart)))
	compact, err := provider.buildPushPullTranslatorExpansion(
		ctx, request, translator, voltageA, voltageB, frequency, channelCount, compactCount, channelsPerPart, "compact",
	)
	if err != nil {
		return nil, err
	}
	activeChannelsPerSegment := max(1, channelsPerPart/2)
	segmentedCount := int(math.Ceil(float64(channelCount) / float64(activeChannelsPerSegment)))
	if segmentedCount == compactCount {
		return compact, nil
	}
	segmented, err := provider.buildPushPullTranslatorExpansion(
		ctx, request, translator, voltageA, voltageB, frequency, channelCount, segmentedCount, activeChannelsPerSegment, "segmented",
	)
	if err != nil {
		return nil, err
	}
	return append(compact, segmented...), nil
}

func (provider *CatalogProvider) expandDirectionControlledTranslator(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "direction_change_state", "equal", "disabled"); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "direction-controlled translation requires direction changes only while outputs are disabled"}
	}
	for _, role := range []string{"enable", "direction_control"} {
		if !slices.ContainsFunc(request.Ports, func(port RoleContract) bool { return port.Role == role }) {
			return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "direction-controlled translation requires " + role + " control evidence"}
		}
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, err
	}
	frequency, err := pushPullTranslationFrequency(request)
	if err != nil {
		return nil, err
	}
	channelCount := 1
	if value, _, ok := firstNumericConstraint(request.Constraints, "channel_count", "bus_width"); ok {
		if value < 1 || value != math.Trunc(value) {
			return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "direction-controlled translation channel count must be a positive integer"}
		}
		channelCount = int(value)
	}
	voltageA, voltageB := roleVoltageMaximum(request.Ports, "power_a"), roleVoltageMaximum(request.Ports, "power_b")
	if !finitePositive(voltageA) || !finitePositive(voltageB) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceVoltageDomainMismatch, message: "direction-controlled translation requires bounded positive voltage domains on both sides"}
	}
	translator, err := provider.selectDirectionControlledTranslator(ctx, math.Min(voltageA, voltageB), math.Max(voltageA, voltageB), frequency)
	if err != nil {
		return nil, err
	}
	channelsPerPart := translator.record.Translator.ChannelCount
	compactCount := int(math.Ceil(float64(channelCount) / float64(channelsPerPart)))
	compact, err := provider.buildDirectionControlledTranslatorExpansion(ctx, request, translator, voltageA, voltageB, frequency, channelCount, compactCount, channelsPerPart, "compact")
	if err != nil {
		return nil, err
	}
	activeChannelsPerSegment := max(1, channelsPerPart/2)
	segmentedCount := int(math.Ceil(float64(channelCount) / float64(activeChannelsPerSegment)))
	if segmentedCount == compactCount {
		return compact, nil
	}
	segmented, err := provider.buildDirectionControlledTranslatorExpansion(ctx, request, translator, voltageA, voltageB, frequency, channelCount, segmentedCount, activeChannelsPerSegment, "segmented")
	if err != nil {
		return nil, err
	}
	return append(compact, segmented...), nil
}

func pushPullTranslationFrequency(request ProviderRequest) (float64, error) {
	frequency := maximumProtocolFrequency(request.Ports)
	if _, exists := namedConstraint(request.Constraints, "bus_frequency"); exists {
		constrained, _, err := requiredNumber(request.Constraints, "bus_frequency", "minimum", "Hz")
		if err != nil {
			return 0, err
		}
		frequency = math.Max(frequency, constrained)
	}
	if !finitePositive(frequency) {
		return 0, &interfaceSynthesisError{
			code:    CodeInterfaceTranslationUnavailable,
			message: "push-pull translation requires bounded positive protocol-frequency evidence",
		}
	}
	return frequency, nil
}

func (provider *CatalogProvider) selectDirectionControlledTranslator(ctx context.Context, low, high, frequency float64) (catalogPart, error) {
	for _, record := range provider.translatorRecords {
		evidence := record.Translator
		if record.Family != "level_translator" || evidence == nil ||
			!translatorEvidenceSupports(record, low, high, "push_pull", "direction_controlled", frequency, 1, true) ||
			evidence.DirectionChangePolicy != "outputs_disabled" || evidence.EnableActiveLevel != "low" ||
			!catalogRecordHasSimulationModel(record, simmodel.PrimitiveDirectionControlledTranslatorV1) {
			continue
		}
		part, selectErr := provider.selectComponent(ctx, "level_translator", record.ID, []components.RequiredRating{
			{Kind: "vcca_supply_voltage", Value: numericString(low), Unit: "V"},
			{Kind: "vccb_supply_voltage", Value: numericString(high), Unit: "V"},
			{Kind: "push_pull_data_rate", Value: numericString(frequency), Unit: "Hz"},
		}, true)
		if selectErr == nil && part.record.ID == record.ID {
			return part, nil
		}
	}
	return catalogPart{}, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "no catalog-backed direction-controlled translator proves voltage, speed, disabled direction changes, startup, and partial-power-down behavior"}
}

func (provider *CatalogProvider) buildDirectionControlledTranslatorExpansion(
	ctx context.Context,
	request ProviderRequest,
	template catalogPart,
	voltageA, voltageB, frequency float64,
	channelCount, partCount, activeChannelsPerPart int,
	variant string,
) ([]ProviderExpansion, error) {
	if partCount <= 0 || activeChannelsPerPart <= 0 || activeChannelsPerPart > template.record.Translator.ChannelCount {
		return nil, fmt.Errorf("direction-controlled translator requires at least one part")
	}
	channelsPerPart := template.record.Translator.ChannelCount
	var parts []catalogPart
	for index := 0; index < partCount; index++ {
		part := template
		part.selected.InstanceID = fmt.Sprintf("bus_transceiver_%02d", index+1)
		part.usage = "direction_controlled_level_translator"
		parts = append(parts, part)
		var err error
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{
			{fmt.Sprintf("transceiver_%02d_vcca_bypass", index+1), "capacitor", "decoupling", "100n"},
			{fmt.Sprintf("transceiver_%02d_vccb_bypass", index+1), "capacitor", "decoupling", "100n"},
			{fmt.Sprintf("transceiver_%02d_enable_pullup", index+1), "resistor", "enable_pullup", engineeringValue(translatorControlPullOhm, "Ohm")},
			{fmt.Sprintf("transceiver_%02d_direction_pulldown", index+1), "resistor", "direction_pulldown", engineeringValue(translatorControlPullOhm, "Ohm")},
		})
		if err != nil {
			return nil, err
		}
	}
	first := parts[0]
	bindings := []RealizationPortBinding{
		{Role: "power_a", Instance: first.selected.InstanceID, Function: "VCCA"},
		{Role: "power_b", Instance: first.selected.InstanceID, Function: "VCCB"},
		{Role: "reference", Instance: first.selected.InstanceID, Function: "GND"},
		{Role: "enable", Instance: first.selected.InstanceID, Function: "OE"},
		{Role: "direction_control", Instance: first.selected.InstanceID, Function: "DIR1"},
	}
	powerAEndpoints, powerBEndpoints := []RealizationEndpoint{}, []RealizationEndpoint{}
	groundEndpoints, enableEndpoints, directionEndpoints := []RealizationEndpoint{}, []RealizationEndpoint{}, []RealizationEndpoint{}
	for index := 0; index < partCount; index++ {
		instance := fmt.Sprintf("bus_transceiver_%02d", index+1)
		powerAEndpoints = append(powerAEndpoints,
			RealizationEndpoint{Instance: instance, Function: "VCCA"},
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_vcca_bypass", index+1), "A"),
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_enable_pullup", index+1), "A"),
		)
		powerBEndpoints = append(powerBEndpoints,
			RealizationEndpoint{Instance: instance, Function: "VCCB"},
			RealizationEndpoint{Instance: instance, Function: "VCCB_AUX"},
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_vccb_bypass", index+1), "A"),
		)
		groundEndpoints = append(groundEndpoints,
			RealizationEndpoint{Instance: instance, Function: "GND"},
			RealizationEndpoint{Instance: instance, Function: "GND_AUX"},
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_vcca_bypass", index+1), "B"),
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_vccb_bypass", index+1), "B"),
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_direction_pulldown", index+1), "A"),
		)
		enableEndpoints = append(enableEndpoints,
			RealizationEndpoint{Instance: instance, Function: "OE"},
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_enable_pullup", index+1), "B"),
		)
		directionEndpoints = append(directionEndpoints,
			RealizationEndpoint{Instance: instance, Function: "DIR1"},
			RealizationEndpoint{Instance: instance, Function: "DIR2"},
			passiveEndpoint(fmt.Sprintf("transceiver_%02d_direction_pulldown", index+1), "B"),
		)
		for local := 0; local < channelsPerPart; local++ {
			channel := index*activeChannelsPerPart + local + 1
			functionA, functionB := fmt.Sprintf("A%d", local+1), fmt.Sprintf("B%d", local+1)
			if local < activeChannelsPerPart && channel <= channelCount {
				lane := fmt.Sprintf("channel_%02d", channel)
				bindings = append(bindings,
					RealizationPortBinding{Role: "side_a", Lane: lane, Instance: instance, Function: functionA},
					RealizationPortBinding{Role: "side_b", Lane: lane, Instance: instance, Function: functionB},
				)
				continue
			}
			groundEndpoints = append(groundEndpoints,
				RealizationEndpoint{Instance: instance, Function: functionA},
				RealizationEndpoint{Instance: instance, Function: functionB},
			)
		}
	}
	connections := []RealizationConnection{
		semanticNet("transceiver_power_a", "power", powerAEndpoints...),
		semanticNet("transceiver_power_b", "power", powerBEndpoints...),
		semanticNet("transceiver_reference", "reference", groundEndpoints...),
		semanticNet("transceiver_enable", "control", enableEndpoints...),
		semanticNet("transceiver_direction", "control", directionEndpoints...),
	}
	calculation := CalculationEvidence{
		ID: "direction_controlled_translation_bounds", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "channel_count", Value: float64(channelCount), Unit: "count"},
			{Name: "frequency", Value: frequency, Unit: "Hz"},
			{Name: "side_a_voltage", Value: voltageA, Unit: "V"},
			{Name: "side_b_voltage", Value: voltageB, Unit: "V"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "translator_count", Value: float64(partCount), Unit: "count"},
			{Name: "startup_high_impedance", Value: 1, Unit: "bool"},
			{Name: "partial_power_down", Value: 1, Unit: "bool"},
			{Name: "direction_change_disabled", Value: 1, Unit: "bool"},
		},
		Bounds: []CalculationBound{
			minimumBound("channel_capacity", float64(channelCount), float64(partCount*activeChannelsPerPart), "count"),
			minimumBound("maximum_frequency", frequency, translatorModeMaximumFrequency(template.record.Translator, "push_pull"), "Hz"),
			minimumBound("side_a_minimum", *template.record.Translator.SideAVoltage.Minimum, voltageA, "V"),
			maximumBound("side_a_maximum", *template.record.Translator.SideAVoltage.Maximum, voltageA, "V"),
			minimumBound("side_b_minimum", *template.record.Translator.SideBVoltage.Minimum, voltageB, "V"),
			maximumBound("side_b_maximum", *template.record.Translator.SideBVoltage.Maximum, voltageB, "V"),
			minimumBound("startup_high_impedance", 1, 1, "bool"),
			minimumBound("partial_power_down", 1, 1, "bool"),
			minimumBound("direction_change_disabled", 1, 1, "bool"),
		},
		Pass: true,
	}
	calculation, err := FinalizeCalculation(calculation)
	if err != nil {
		return nil, err
	}
	return provider.expansion(request, fmt.Sprintf("direction_controlled_%s_%02d_channel", variant, channelCount), parts, bindings, connections, []CalculationEvidence{calculation}, 0)
}

func (provider *CatalogProvider) selectPushPullTranslator(
	ctx context.Context,
	low, high, frequency float64,
	direction string,
) (catalogPart, error) {
	for _, record := range provider.translatorRecords {
		if record.Family != "level_translator" ||
			!translatorEvidenceSupports(record, low, high, "push_pull", direction, frequency, 1, true) ||
			!catalogRecordHasSimulationModel(record, simmodel.PrimitivePushPullTranslatorV1) {
			continue
		}
		part, selectErr := provider.selectComponent(ctx, "level_translator", record.ID, []components.RequiredRating{
			{Kind: "vcca_supply_voltage", Value: numericString(low), Unit: "V"},
			{Kind: "vccb_supply_voltage", Value: numericString(high), Unit: "V"},
			{Kind: "push_pull_data_rate", Value: numericString(frequency), Unit: "Hz"},
		}, true)
		if selectErr == nil && part.record.ID == record.ID {
			return part, nil
		}
	}
	return catalogPart{}, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "no catalog-backed push-pull translator proves voltage, direction, speed, startup, and partial-power-down behavior"}
}

func (provider *CatalogProvider) buildPushPullTranslatorExpansion(
	ctx context.Context,
	request ProviderRequest,
	template catalogPart,
	voltageA, voltageB, frequency float64,
	channelCount, partCount, activeChannelsPerPart int,
	variant string,
) ([]ProviderExpansion, error) {
	if partCount <= 0 || activeChannelsPerPart <= 0 || activeChannelsPerPart > template.record.Translator.ChannelCount {
		return nil, fmt.Errorf("push-pull translator requires at least one part")
	}
	channelsPerPart := template.record.Translator.ChannelCount
	modelDirection, err := pushPullTranslatorModelDirection(request.Ports, voltageA <= voltageB)
	if err != nil {
		return nil, err
	}
	var parts []catalogPart
	for index := 0; index < partCount; index++ {
		part := template
		part.selected.InstanceID = fmt.Sprintf("bus_translator_%02d", index+1)
		part.usage = "push_pull_level_translator"
		part.parameters = append(part.parameters, RealizationParameter{Name: "direction", Value: modelDirection, Unit: "polarity"})
		parts = append(parts, part)
		var err error
		parts, err = provider.appendPassiveParts(ctx, parts, []passivePart{
			{fmt.Sprintf("translator_%02d_vcca_bypass", index+1), "capacitor", "decoupling", "100n"},
			{fmt.Sprintf("translator_%02d_vccb_bypass", index+1), "capacitor", "decoupling", "100n"},
			{fmt.Sprintf("translator_%02d_enable_pulldown", index+1), "resistor", "enable_pulldown", engineeringValue(translatorEnablePulldownOhm, "Ohm")},
		})
		if err != nil {
			return nil, err
		}
	}
	first := parts[0]
	lowOnA := voltageA <= voltageB
	powerAFunction, powerBFunction := "VCCA", "VCCB"
	if !lowOnA {
		powerAFunction, powerBFunction = "VCCB", "VCCA"
	}
	bindings := []RealizationPortBinding{
		{Role: "power_a", Instance: first.selected.InstanceID, Function: powerAFunction},
		{Role: "power_b", Instance: first.selected.InstanceID, Function: powerBFunction},
		{Role: "reference", Instance: first.selected.InstanceID, Function: "GND"},
		{Role: "enable", Instance: first.selected.InstanceID, Function: "OE"},
	}
	connections := []RealizationConnection{}
	powerAEndpoints, powerBEndpoints, groundEndpoints, enableEndpoints := []RealizationEndpoint{}, []RealizationEndpoint{}, []RealizationEndpoint{}, []RealizationEndpoint{}
	for index := 0; index < partCount; index++ {
		instance := fmt.Sprintf("bus_translator_%02d", index+1)
		powerABypass, powerBBypass := "vcca", "vccb"
		if powerAFunction == "VCCB" {
			powerABypass, powerBBypass = "vccb", "vcca"
		}
		powerAEndpoints = append(powerAEndpoints,
			RealizationEndpoint{Instance: instance, Function: powerAFunction},
			passiveEndpoint(fmt.Sprintf("translator_%02d_%s_bypass", index+1, powerABypass), "A"),
		)
		powerBEndpoints = append(powerBEndpoints,
			RealizationEndpoint{Instance: instance, Function: powerBFunction},
			passiveEndpoint(fmt.Sprintf("translator_%02d_%s_bypass", index+1, powerBBypass), "A"),
		)
		groundEndpoints = append(groundEndpoints,
			RealizationEndpoint{Instance: instance, Function: "GND"},
			passiveEndpoint(fmt.Sprintf("translator_%02d_vcca_bypass", index+1), "B"),
			passiveEndpoint(fmt.Sprintf("translator_%02d_vccb_bypass", index+1), "B"),
			passiveEndpoint(fmt.Sprintf("translator_%02d_enable_pulldown", index+1), "A"),
		)
		enableEndpoints = append(enableEndpoints,
			RealizationEndpoint{Instance: instance, Function: "OE"},
			passiveEndpoint(fmt.Sprintf("translator_%02d_enable_pulldown", index+1), "B"),
		)
		for local := 0; local < channelsPerPart; local++ {
			channel := index*activeChannelsPerPart + local + 1
			functionA, functionB := fmt.Sprintf("A%d", local+1), fmt.Sprintf("B%d", local+1)
			if local < activeChannelsPerPart && channel <= channelCount {
				sideAFunction, sideBFunction := functionA, functionB
				if !lowOnA {
					sideAFunction, sideBFunction = functionB, functionA
				}
				lane := fmt.Sprintf("channel_%02d", channel)
				bindings = append(bindings,
					RealizationPortBinding{Role: "side_a", Lane: lane, Instance: instance, Function: sideAFunction},
					RealizationPortBinding{Role: "side_b", Lane: lane, Instance: instance, Function: sideBFunction},
				)
				continue
			}
			// Auto-direction translators have internally biased bidirectional
			// I/Os. Leaving unused channel pairs unconnected preserves that
			// reviewed biasing and lets synthesis emit explicit no-connect
			// decisions instead of placing I/O pins on a flagged power rail.
		}
	}
	connections = append(connections,
		semanticNet("translator_power_a", "power", powerAEndpoints...),
		semanticNet("translator_power_b", "power", powerBEndpoints...),
		semanticNet("translator_reference", "reference", groundEndpoints...),
		semanticNet("translator_enable", "control", enableEndpoints...),
	)
	maximumFrequency := translatorModeMaximumFrequency(template.record.Translator, "push_pull")
	calculation := CalculationEvidence{
		ID: "push_pull_translation_bounds", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "channel_count", Value: float64(channelCount), Unit: "count"},
			{Name: "frequency", Value: frequency, Unit: "Hz"},
			{Name: "side_a_voltage", Value: voltageA, Unit: "V"},
			{Name: "side_b_voltage", Value: voltageB, Unit: "V"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "translator_count", Value: float64(partCount), Unit: "count"},
			{Name: "startup_high_impedance", Value: 1, Unit: "bool"},
			{Name: "partial_power_down", Value: 1, Unit: "bool"},
		},
		Bounds: []CalculationBound{
			minimumBound("channel_capacity", float64(channelCount), float64(partCount*activeChannelsPerPart), "count"),
			minimumBound("maximum_frequency", frequency, maximumFrequency, "Hz"),
			minimumBound("vcca_minimum", *template.record.Translator.SideAVoltage.Minimum, math.Min(voltageA, voltageB), "V"),
			maximumBound("vcca_maximum", *template.record.Translator.SideAVoltage.Maximum, math.Min(voltageA, voltageB), "V"),
			minimumBound("vccb_minimum", *template.record.Translator.SideBVoltage.Minimum, math.Max(voltageA, voltageB), "V"),
			maximumBound("vccb_maximum", *template.record.Translator.SideBVoltage.Maximum, math.Max(voltageA, voltageB), "V"),
			minimumBound("startup_high_impedance", 1, 1, "bool"),
			minimumBound("partial_power_down", 1, 1, "bool"),
		},
		Pass: true,
	}
	calculation, err = FinalizeCalculation(calculation)
	if err != nil {
		return nil, err
	}
	return provider.expansion(
		request,
		fmt.Sprintf("push_pull_%s_%02d_channel", variant, channelCount),
		parts, bindings, connections, []CalculationEvidence{calculation}, 0,
	)
}

func pushPullTranslatorModelDirection(ports []RoleContract, lowOnSideA bool) (float64, error) {
	sideADirection, sideBDirection := "", ""
	for _, port := range ports {
		switch port.Role {
		case "side_a":
			sideADirection = canonicalIdentifier(port.Contract.Direction)
		case "side_b":
			sideBDirection = canonicalIdentifier(port.Contract.Direction)
		}
	}
	sourceIsSideA := sideADirection == "source" && sideBDirection == "sink"
	sourceIsSideB := sideADirection == "sink" && sideBDirection == "source"
	if !sourceIsSideA && !sourceIsSideB {
		return 0, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "push-pull translation requires opposing source and sink direction evidence"}
	}
	sourceIsPhysicalA := sourceIsSideA == lowOnSideA
	if sourceIsPhysicalA {
		return 1, nil
	}
	return -1, nil
}

func translatorModeMaximumFrequency(evidence *components.TranslatorEvidence, mode string) float64 {
	if evidence == nil {
		return 0
	}
	measurement := evidence.MaximumFrequency
	if mode == "push_pull" && evidence.MaximumPushPullFrequency != nil {
		measurement = evidence.MaximumPushPullFrequency
	}
	if mode == "open_drain" && evidence.MaximumOpenDrainFrequency != nil {
		measurement = evidence.MaximumOpenDrainFrequency
	}
	if measurement == nil {
		return 0
	}
	value, _ := convertCatalogUnit(measurement.Value, measurement.Unit, "Hz")
	return value
}
