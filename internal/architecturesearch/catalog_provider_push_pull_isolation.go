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

const unusedIsolatorOutputLoadOhm = 1_000_000.0

type functionalIsolationChannel struct {
	input  string
	output string
}

func pushPullFunctionalIsolationRequested(request ProviderRequest) bool {
	if constraint, ok := namedConstraint(request.Constraints, "signaling_mode"); ok && constraintStringEquals(constraint, "push_pull") {
		return true
	}
	return slices.ContainsFunc(request.Ports, func(port RoleContract) bool {
		return port.Contract.Protocol != nil && strings.EqualFold(port.Contract.Protocol.Mode, "push_pull")
	})
}

func (provider *CatalogProvider) expandPushPullFunctionalIsolation(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := requireString(request.Constraints, "signaling_mode", "equal", "push_pull"); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "supply_loss_safe_state", "equal", "low"); err != nil {
		return nil, fmt.Errorf("push-pull functional isolation requires an explicit default-low supply-loss safe state: %w", err)
	}
	if err := requireBool(request.Constraints, "independent_startup", "required", true); err != nil {
		return nil, fmt.Errorf("push-pull functional isolation requires independent-startup evidence: %w", err)
	}
	forwardCount, reverseCount, err := functionalIsolationChannelCounts(request.Constraints)
	if err != nil {
		return nil, err
	}
	for _, role := range []string{"power_a", "reference_a", "power_b", "reference_b"} {
		if !hasRoleContract(request.Ports, role) {
			return nil, fmt.Errorf("push-pull functional isolation requires role %s", role)
		}
	}
	if forwardCount > 0 {
		for _, role := range []string{"side_a_forward", "side_b_forward"} {
			if !hasRoleContract(request.Ports, role) {
				return nil, fmt.Errorf("forward functional-isolation channels require role %s", role)
			}
		}
	}
	if reverseCount > 0 {
		for _, role := range []string{"side_b_reverse", "side_a_reverse"} {
			if !hasRoleContract(request.Ports, role) {
				return nil, fmt.Errorf("reverse functional-isolation channels require role %s", role)
			}
		}
	}
	frequency := maximumProtocolFrequency(request.Ports)
	if constrained, _, ok := firstNumericConstraint(request.Constraints, "bus_frequency", "maximum_frequency"); ok {
		frequency = math.Max(frequency, constrained)
	}
	if !finitePositive(frequency) {
		return nil, fmt.Errorf("push-pull functional isolation requires a bounded positive signal frequency")
	}
	voltageAMin, voltageAMax, okA := roleVoltageRange(request.Ports, "power_a")
	voltageBMin, voltageBMax, okB := roleVoltageRange(request.Ports, "power_b")
	if !okA || !okB || !finitePositive(voltageAMin) || !finitePositive(voltageAMax) ||
		!finitePositive(voltageBMin) || !finitePositive(voltageBMax) {
		return nil, fmt.Errorf("push-pull functional isolation requires bounded positive supplies on both sides")
	}
	workingVoltage, _, ok := firstNumericConstraint(request.Constraints, "isolation_working_voltage", "isolation_voltage")
	if !ok || !finitePositive(workingVoltage) {
		return nil, fmt.Errorf("push-pull functional isolation requires a positive working-isolation voltage")
	}
	requiredClearance := optionalPositiveConstraint(request.Constraints, "minimum_clearance", "clearance")
	requiredCreepage := optionalPositiveConstraint(request.Constraints, "minimum_creepage", "creepage")
	transientVoltage := optionalPositiveConstraint(request.Constraints, "isolation_transient_voltage", "transient_isolation_voltage")

	isolator, forwardPerPart, reversePerPart, err := provider.selectPushPullFunctionalIsolator(
		ctx, voltageAMin, voltageAMax, voltageBMin, voltageBMax, frequency,
		workingVoltage, transientVoltage, requiredClearance, requiredCreepage,
	)
	if err != nil {
		return nil, err
	}
	compactCount := functionalIsolationPartCount(forwardCount, reverseCount, forwardPerPart, reversePerPart)
	compact, err := provider.buildPushPullFunctionalIsolationExpansion(
		ctx, request, isolator, forwardCount, reverseCount, compactCount,
		forwardPerPart, reversePerPart, forwardPerPart, reversePerPart,
		frequency, voltageAMin, voltageAMax, voltageBMin, voltageBMax,
		workingVoltage, transientVoltage, requiredClearance, requiredCreepage, "compact",
	)
	if err != nil {
		return nil, err
	}
	segmentedForward := forwardPerPart
	if forwardCount > 1 {
		segmentedForward = 1
	}
	segmentedReverse := reversePerPart
	segmentedCount := functionalIsolationPartCount(forwardCount, reverseCount, segmentedForward, segmentedReverse)
	if segmentedCount == compactCount {
		return compact, nil
	}
	segmented, err := provider.buildPushPullFunctionalIsolationExpansion(
		ctx, request, isolator, forwardCount, reverseCount, segmentedCount,
		forwardPerPart, reversePerPart, segmentedForward, segmentedReverse,
		frequency, voltageAMin, voltageAMax, voltageBMin, voltageBMax,
		workingVoltage, transientVoltage, requiredClearance, requiredCreepage, "segmented",
	)
	if err != nil {
		return nil, err
	}
	return append(compact, segmented...), nil
}

func functionalIsolationChannelCounts(constraints []Constraint) (int, int, error) {
	parse := func(names ...string) (int, bool, error) {
		value, _, ok := firstNumericConstraint(constraints, names...)
		if !ok {
			return 0, false, nil
		}
		if value < 0 || value != math.Trunc(value) {
			return 0, true, fmt.Errorf("%s must be a nonnegative integer", names[0])
		}
		return int(value), true, nil
	}
	forward, forwardOK, err := parse("forward_channel_count", "forward_channels")
	if err != nil {
		return 0, 0, err
	}
	reverse, reverseOK, err := parse("reverse_channel_count", "reverse_channels")
	if err != nil {
		return 0, 0, err
	}
	if !forwardOK || !reverseOK || forward+reverse == 0 {
		return 0, 0, fmt.Errorf("push-pull functional isolation requires explicit channel counts with at least one nonzero direction")
	}
	return forward, reverse, nil
}

func optionalPositiveConstraint(constraints []Constraint, names ...string) float64 {
	value, _, ok := firstNumericConstraint(constraints, names...)
	if !ok || !finitePositive(value) {
		return 0
	}
	return value
}

func (provider *CatalogProvider) selectPushPullFunctionalIsolator(
	ctx context.Context,
	voltageAMin, voltageAMax, voltageBMin, voltageBMax, frequency,
	workingVoltage, transientVoltage, requiredClearance, requiredCreepage float64,
) (catalogPart, int, int, error) {
	var rejections []string
	for _, record := range provider.familyRecords["isolator"] {
		if record.Family != "isolator" || !catalogRecordHasSimulationModel(record, simmodel.PrimitivePushPullDigitalIsolatorV1) ||
			!slices.Contains(record.Tags, "default_low") {
			continue
		}
		forwardValue, forwardOK := recordValue(record, "forward_channel_count", "count")
		reverseValue, reverseOK := recordValue(record, "reverse_channel_count", "count")
		forward, reverse := int(forwardValue), int(reverseValue)
		if !forwardOK || !reverseOK || forward <= 0 || reverse <= 0 ||
			forwardValue != float64(forward) || reverseValue != float64(reverse) {
			rejections = append(rejections, record.ID+": invalid catalog channel-count evidence")
			continue
		}
		requirements := []components.RequiredRating{
			{Kind: "side_a_supply_voltage", Value: numericString(voltageAMin), Unit: "V"},
			{Kind: "side_a_supply_voltage", Value: numericString(voltageAMax), Unit: "V"},
			{Kind: "side_b_supply_voltage", Value: numericString(voltageBMin), Unit: "V"},
			{Kind: "side_b_supply_voltage", Value: numericString(voltageBMax), Unit: "V"},
			{Kind: "data_rate", Value: numericString(frequency), Unit: "Hz"},
			{Kind: "isolation_working_voltage", Value: numericString(workingVoltage), Unit: "V"},
		}
		if transientVoltage > 0 {
			requirements = append(requirements, components.RequiredRating{Kind: "isolation_transient_voltage", Value: numericString(transientVoltage), Unit: "V"})
		}
		if requiredClearance > 0 {
			requirements = append(requirements, components.RequiredRating{Kind: "clearance", Value: numericString(requiredClearance), Unit: "mm"})
		}
		if requiredCreepage > 0 {
			requirements = append(requirements, components.RequiredRating{Kind: "creepage", Value: numericString(requiredCreepage), Unit: "mm"})
		}
		part, selectErr := provider.selectComponent(ctx, "isolator", record.ID, requirements, true)
		if selectErr == nil && part.record.ID == record.ID {
			return part, forward, reverse, nil
		}
		if selectErr != nil {
			rejections = append(rejections, record.ID+": "+selectErr.Error())
		} else {
			rejections = append(rejections, record.ID+": deterministic selection returned "+part.record.ID)
		}
	}
	if len(rejections) == 0 {
		return catalogPart{}, 0, 0, fmt.Errorf("no catalog-backed default-low push-pull isolator exists")
	}
	return catalogPart{}, 0, 0, fmt.Errorf(
		"no catalog-backed default-low push-pull isolator proves supply, direction, speed, and working-isolation bounds: %s",
		strings.Join(rejections, "; "),
	)
}

func functionalIsolationPartCount(forwardCount, reverseCount, forwardPerPart, reversePerPart int) int {
	count := 0
	if forwardCount > 0 {
		count = int(math.Ceil(float64(forwardCount) / float64(forwardPerPart)))
	}
	if reverseCount > 0 {
		count = max(count, int(math.Ceil(float64(reverseCount)/float64(reversePerPart))))
	}
	return count
}

func functionalIsolationChannels(record components.ComponentRecord) ([]functionalIsolationChannel, []functionalIsolationChannel, error) {
	var (
		forwardReference []functionalIsolationChannel
		reverseReference []functionalIsolationChannel
		found            bool
	)
	for _, symbol := range record.Symbols {
		forward, reverse, present, err := functionalIsolationSymbolChannels(symbol)
		if err != nil {
			return nil, nil, fmt.Errorf("catalog isolator %s symbol %s: %w", record.ID, symbol.SymbolID, err)
		}
		if !present {
			continue
		}
		if !found {
			forwardReference, reverseReference, found = forward, reverse, true
			continue
		}
		if !slices.Equal(forwardReference, forward) || !slices.Equal(reverseReference, reverse) {
			return nil, nil, fmt.Errorf("catalog isolator %s has inconsistent channel functions across symbol variants", record.ID)
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("catalog isolator %s has no paired push-pull channel functions", record.ID)
	}
	return forwardReference, reverseReference, nil
}

func functionalIsolationSymbolChannels(symbol components.SymbolBinding) ([]functionalIsolationChannel, []functionalIsolationChannel, bool, error) {
	functions := make(map[string]bool, len(symbol.FunctionPins))
	present := false
	for _, pin := range symbol.FunctionPins {
		functions[pin.Function] = true
		if strings.HasPrefix(pin.Function, "INA") || strings.HasPrefix(pin.Function, "INB") ||
			strings.HasPrefix(pin.Function, "OUTA") || strings.HasPrefix(pin.Function, "OUTB") {
			present = true
		}
	}
	if !present {
		return nil, nil, false, nil
	}
	pairs := func(inputPrefix, outputPrefix string) ([]functionalIsolationChannel, error) {
		var channels []functionalIsolationChannel
		for function := range functions {
			suffix, match := strings.CutPrefix(function, inputPrefix)
			if !match || suffix == "" {
				continue
			}
			output := outputPrefix + suffix
			if !functions[output] {
				return nil, fmt.Errorf("input function %s lacks paired output function %s", function, output)
			}
			channels = append(channels, functionalIsolationChannel{input: function, output: output})
		}
		slices.SortStableFunc(channels, func(left, right functionalIsolationChannel) int {
			if order := strings.Compare(left.input, right.input); order != 0 {
				return order
			}
			return strings.Compare(left.output, right.output)
		})
		return channels, nil
	}
	forward, err := pairs("INA", "OUTB")
	if err != nil {
		return nil, nil, true, err
	}
	reverse, err := pairs("INB", "OUTA")
	if err != nil {
		return nil, nil, true, err
	}
	for function := range functions {
		switch {
		case strings.HasPrefix(function, "OUTB"):
			suffix := strings.TrimPrefix(function, "OUTB")
			if suffix == "" || !functions["INA"+suffix] {
				return nil, nil, true, fmt.Errorf("output function %s lacks paired input function INA%s", function, suffix)
			}
		case strings.HasPrefix(function, "OUTA"):
			suffix := strings.TrimPrefix(function, "OUTA")
			if suffix == "" || !functions["INB"+suffix] {
				return nil, nil, true, fmt.Errorf("output function %s lacks paired input function INB%s", function, suffix)
			}
		}
	}
	if len(forward)+len(reverse) == 0 {
		return nil, nil, true, fmt.Errorf("push-pull channel functions are unpaired")
	}
	return forward, reverse, true, nil
}

func (provider *CatalogProvider) buildPushPullFunctionalIsolationExpansion(
	ctx context.Context,
	request ProviderRequest,
	template catalogPart,
	forwardCount, reverseCount, partCount, forwardPerPart, reversePerPart,
	activeForwardPerPart, activeReversePerPart int,
	frequency, voltageAMin, voltageAMax, voltageBMin, voltageBMax,
	workingVoltage, transientVoltage, requiredClearance, requiredCreepage float64,
	variant string,
) ([]ProviderExpansion, error) {
	if partCount <= 0 || activeForwardPerPart <= 0 || activeForwardPerPart > forwardPerPart ||
		activeReversePerPart <= 0 || activeReversePerPart > reversePerPart {
		return nil, fmt.Errorf("push-pull functional isolation requires a positive bounded part allocation")
	}
	forwardChannels, reverseChannels, err := functionalIsolationChannels(template.record)
	if err != nil {
		return nil, err
	}
	if len(forwardChannels) != forwardPerPart || len(reverseChannels) != reversePerPart {
		return nil, fmt.Errorf(
			"catalog isolator %s channel pin map has %d forward and %d reverse pairs; evidence declares %d and %d",
			template.record.ID, len(forwardChannels), len(reverseChannels), forwardPerPart, reversePerPart,
		)
	}
	var parts []catalogPart
	for index := 0; index < partCount; index++ {
		part := template
		part.selected.InstanceID = fmt.Sprintf("functional_isolator_%02d", index+1)
		part.usage = "push_pull_functional_isolation"
		parts = append(parts, part)
		passives := []passivePart{
			{fmt.Sprintf("isolator_%02d_side_a_bypass", index+1), "capacitor", "decoupling", "100n"},
			{fmt.Sprintf("isolator_%02d_side_b_bypass", index+1), "capacitor", "decoupling", "100n"},
		}
		for local := 0; local < forwardPerPart; local++ {
			channel := index*activeForwardPerPart + local + 1
			if local < activeForwardPerPart && channel <= forwardCount {
				continue
			}
			passives = append(passives, passivePart{
				fmt.Sprintf("isolator_%02d_forward_%02d_unused_load", index+1, local+1),
				"resistor", "unused_output_load", engineeringValue(unusedIsolatorOutputLoadOhm, "Ohm"),
			})
		}
		for local := 0; local < reversePerPart; local++ {
			channel := index*activeReversePerPart + local + 1
			if local < activeReversePerPart && channel <= reverseCount {
				continue
			}
			passives = append(passives, passivePart{
				fmt.Sprintf("isolator_%02d_reverse_%02d_unused_load", index+1, local+1),
				"resistor", "unused_output_load", engineeringValue(unusedIsolatorOutputLoadOhm, "Ohm"),
			})
		}
		var err error
		parts, err = provider.appendPassiveParts(ctx, parts, passives)
		if err != nil {
			return nil, err
		}
	}

	first := parts[0]
	bindings := []RealizationPortBinding{
		{Role: "power_a", Instance: first.selected.InstanceID, Function: "VDD1"},
		{Role: "reference_a", Instance: first.selected.InstanceID, Function: "GND1"},
		{Role: "power_b", Instance: first.selected.InstanceID, Function: "VDD2"},
		{Role: "reference_b", Instance: first.selected.InstanceID, Function: "GND2"},
	}
	var powerA, referenceA, powerB, referenceB []RealizationEndpoint
	var unusedOutputConnections []RealizationConnection
	for index := 0; index < partCount; index++ {
		instance := fmt.Sprintf("functional_isolator_%02d", index+1)
		powerA = append(powerA,
			RealizationEndpoint{Instance: instance, Function: "VDD1"},
			RealizationEndpoint{Instance: instance, Function: "EN1"},
			passiveEndpoint(fmt.Sprintf("isolator_%02d_side_a_bypass", index+1), "A"),
		)
		referenceA = append(referenceA,
			RealizationEndpoint{Instance: instance, Function: "GND1"},
			RealizationEndpoint{Instance: instance, Function: "GND1_AUX"},
			passiveEndpoint(fmt.Sprintf("isolator_%02d_side_a_bypass", index+1), "B"),
		)
		powerB = append(powerB,
			RealizationEndpoint{Instance: instance, Function: "VDD2"},
			RealizationEndpoint{Instance: instance, Function: "EN2"},
			passiveEndpoint(fmt.Sprintf("isolator_%02d_side_b_bypass", index+1), "A"),
		)
		referenceB = append(referenceB,
			RealizationEndpoint{Instance: instance, Function: "GND2"},
			RealizationEndpoint{Instance: instance, Function: "GND2_AUX"},
			passiveEndpoint(fmt.Sprintf("isolator_%02d_side_b_bypass", index+1), "B"),
		)
		for local := 0; local < forwardPerPart; local++ {
			channel := index*activeForwardPerPart + local + 1
			input, output := forwardChannels[local].input, forwardChannels[local].output
			if local < activeForwardPerPart && channel <= forwardCount {
				lane := fmt.Sprintf("channel_%02d", channel)
				bindings = append(bindings,
					RealizationPortBinding{Role: "side_a_forward", Lane: lane, Instance: instance, Function: input},
					RealizationPortBinding{Role: "side_b_forward", Lane: lane, Instance: instance, Function: output},
				)
				continue
			}
			load := fmt.Sprintf("isolator_%02d_forward_%02d_unused_load", index+1, local+1)
			referenceA = append(referenceA, RealizationEndpoint{Instance: instance, Function: input})
			referenceB = append(referenceB, passiveEndpoint(load, "B"))
			unusedOutputConnections = append(unusedOutputConnections, semanticNet(
				fmt.Sprintf("functional_isolator_%02d_forward_%02d_unused_output", index+1, local+1),
				"digital_signal", RealizationEndpoint{Instance: instance, Function: output}, passiveEndpoint(load, "A"),
			))
		}
		for local := 0; local < reversePerPart; local++ {
			channel := index*activeReversePerPart + local + 1
			input, output := reverseChannels[local].input, reverseChannels[local].output
			if local < activeReversePerPart && channel <= reverseCount {
				lane := fmt.Sprintf("channel_%02d", channel)
				bindings = append(bindings,
					RealizationPortBinding{Role: "side_b_reverse", Lane: lane, Instance: instance, Function: input},
					RealizationPortBinding{Role: "side_a_reverse", Lane: lane, Instance: instance, Function: output},
				)
				continue
			}
			load := fmt.Sprintf("isolator_%02d_reverse_%02d_unused_load", index+1, local+1)
			referenceB = append(referenceB, RealizationEndpoint{Instance: instance, Function: input})
			referenceA = append(referenceA, passiveEndpoint(load, "B"))
			unusedOutputConnections = append(unusedOutputConnections, semanticNet(
				fmt.Sprintf("functional_isolator_%02d_reverse_%02d_unused_output", index+1, local+1),
				"digital_signal", RealizationEndpoint{Instance: instance, Function: output}, passiveEndpoint(load, "A"),
			))
		}
	}
	connections := []RealizationConnection{
		semanticNet("functional_isolator_power_a", "power", powerA...),
		semanticNet("functional_isolator_reference_a", "reference", referenceA...),
		semanticNet("functional_isolator_power_b", "power", powerB...),
		semanticNet("functional_isolator_reference_b", "reference", referenceB...),
	}
	connections = append(connections, unusedOutputConnections...)

	ratedFrequency, _ := recordRatingMaximum(template.record, "data_rate", "Hz")
	ratedWorkingVoltage, _ := recordRatingMaximum(template.record, "isolation_working_voltage", "V")
	ratedTransientVoltage, _ := recordRatingMaximum(template.record, "isolation_transient_voltage", "V")
	ratedClearance, _ := recordRatingMinimum(template.record, "clearance", "mm")
	ratedCreepage, _ := recordRatingMinimum(template.record, "creepage", "mm")
	ratedAMin, _ := recordRatingMinimum(template.record, "side_a_supply_voltage", "V")
	ratedAMax, _ := recordRatingMaximum(template.record, "side_a_supply_voltage", "V")
	ratedBMin, _ := recordRatingMinimum(template.record, "side_b_supply_voltage", "V")
	ratedBMax, _ := recordRatingMaximum(template.record, "side_b_supply_voltage", "V")
	bounds := []CalculationBound{
		minimumBound("forward_channel_capacity", float64(forwardCount), float64(partCount*activeForwardPerPart), "count"),
		minimumBound("reverse_channel_capacity", float64(reverseCount), float64(partCount*activeReversePerPart), "count"),
		minimumBound("maximum_frequency", frequency, ratedFrequency, "Hz"),
		minimumBound("working_isolation_voltage", workingVoltage, ratedWorkingVoltage, "V"),
		minimumBound("side_a_supply_minimum", ratedAMin, voltageAMin, "V"),
		maximumBound("side_a_supply_maximum", ratedAMax, voltageAMax, "V"),
		minimumBound("side_b_supply_minimum", ratedBMin, voltageBMin, "V"),
		maximumBound("side_b_supply_maximum", ratedBMax, voltageBMax, "V"),
		minimumBound("default_low_supply_loss_state", 1, 1, "bool"),
		minimumBound("independent_startup", 1, 1, "bool"),
	}
	if transientVoltage > 0 {
		bounds = append(bounds, minimumBound("transient_isolation_voltage", transientVoltage, ratedTransientVoltage, "V"))
	}
	if requiredClearance > 0 {
		bounds = append(bounds, minimumBound("clearance", requiredClearance, ratedClearance, "mm"))
	}
	if requiredCreepage > 0 {
		bounds = append(bounds, minimumBound("creepage", requiredCreepage, ratedCreepage, "mm"))
	}
	calculation := CalculationEvidence{
		ID: "push_pull_functional_isolation_bounds", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "forward_channel_count", Value: float64(forwardCount), Unit: "count"},
			{Name: "reverse_channel_count", Value: float64(reverseCount), Unit: "count"},
			{Name: "frequency", Value: frequency, Unit: "Hz"},
			{Name: "working_isolation_voltage", Value: workingVoltage, Unit: "V"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "isolator_count", Value: float64(partCount), Unit: "count"},
			{Name: "default_low_supply_loss_state", Value: 1, Unit: "bool"},
			{Name: "independent_startup", Value: 1, Unit: "bool"},
			{Name: "package_clearance", Value: ratedClearance, Unit: "mm"},
			{Name: "package_creepage", Value: ratedCreepage, Unit: "mm"},
		},
		Bounds: bounds,
		Pass:   true,
	}
	calculation, err = FinalizeCalculation(calculation)
	if err != nil {
		return nil, err
	}
	if !calculation.Pass {
		return nil, fmt.Errorf("push-pull functional isolation bounds failed")
	}
	return provider.expansion(
		request,
		fmt.Sprintf("push_pull_functional_isolation_%s_%02d_forward_%02d_reverse", variant, forwardCount, reverseCount),
		parts, bindings, connections, []CalculationEvidence{calculation}, 0,
	)
}
