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

const maximumProtocolBusSegments = 8

// expandProtocolAwareBus selects ordinary translation for a single uniform
// protocol group and a physically segmented topology when the requirement
// declares multiple independently isolated open-drain branches.
func (provider *CatalogProvider) expandProtocolAwareBus(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if hasMixedDirectionProtocolRoles(request.Ports) {
		return provider.expandMixedDirectionPushPullBus(ctx, request)
	}
	protocol, mode, direction, err := translatedProtocolContract(request.Ports)
	if err != nil {
		return nil, err
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "protocol-aware bus synthesis requires explicit partial-power-down and back-power prevention"}
	}
	if err := requireBool(request.Constraints, "hot_plug_isolation", "required", true); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "protocol-aware bus synthesis requires explicit hot-plug isolation"}
	}
	projected, err := cloneProviderRequest(request)
	if err != nil {
		return nil, err
	}
	frequency := maximumProtocolFrequency(request.Ports)
	if frequency <= 0 {
		frequency, _, _ = firstNumericConstraint(request.Constraints, "bus_frequency")
	}
	if frequency <= 0 {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "protocol-aware bus synthesis requires bounded frequency evidence"}
	}
	projected.Constraints = mergeProjectedConstraints(projected.Constraints, []Constraint{
		stringConstraint("protocol", "equal", protocol),
		stringConstraint("signaling_mode", "equal", mode),
		stringConstraint("direction", "equal", direction),
		boolConstraint("unpowered_backfeed_prevention", "required"),
		numericConstraint("bus_frequency", "minimum", frequency, "Hz", 0),
	})
	switch mode {
	case "open_drain":
		if err := requireString(projected.Constraints, "contention_policy", "equal", "wired_low"); err != nil {
			return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "open-drain bus synthesis requires an explicit wired-low contention policy"}
		}
	case "push_pull":
		if err := requireString(projected.Constraints, "contention_policy", "equal", "exclusive_drivers"); err != nil {
			return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "push-pull bus synthesis requires an explicit exclusive-driver contention policy"}
		}
	}
	segmentCount, _, hasSegments := firstNumericConstraint(projected.Constraints, "segment_count")
	if hasSegments && segmentCount > 1 {
		expansions, expandErr := provider.expandSegmentedOpenDrainBus(ctx, projected, protocol, mode, direction, int(segmentCount))
		return appendProtocolBusSafetyEvidence(expansions, expandErr)
	}
	switch mode {
	case "open_drain":
		expansions, expandErr := provider.expandTranslator(ctx, projected)
		return appendProtocolBusSafetyEvidence(expansions, expandErr)
	case "push_pull":
		expansions, expandErr := provider.expandPushPullTranslator(ctx, projected)
		return appendProtocolBusSafetyEvidence(expansions, expandErr)
	default:
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "protocol-aware bus synthesis supports only open-drain and push-pull signaling"}
	}
}

func (provider *CatalogProvider) expandMixedDirectionPushPullBus(ctx context.Context, request ProviderRequest) ([]ProviderExpansion, error) {
	if err := validateMixedDirectionProtocolRoles(request.Ports); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "signaling_mode", "equal", "push_pull"); err != nil {
		return nil, err
	}
	if err := requireString(request.Constraints, "direction", "equal", "mixed_unidirectional"); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires an explicit mixed_unidirectional direction contract"}
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires explicit partial-power-down and back-power prevention"}
	}
	if err := requireBool(request.Constraints, "hot_plug_isolation", "required", true); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires explicit hot-plug isolation"}
	}
	if err := requireString(request.Constraints, "contention_policy", "equal", "exclusive_drivers"); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires an explicit exclusive-driver contention policy"}
	}
	forwardChannels, _, forwardOK := firstNumericConstraint(request.Constraints, "forward_channel_count")
	reverseChannels, _, reverseOK := firstNumericConstraint(request.Constraints, "reverse_channel_count")
	if !forwardOK || !reverseOK || forwardChannels < 1 || reverseChannels < 1 ||
		forwardChannels != math.Trunc(forwardChannels) || reverseChannels != math.Trunc(reverseChannels) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires positive integral forward and reverse channel counts"}
	}

	forward, err := mixedDirectionSubrequest(request,
		"side_a_forward", "side_b_forward", "power_a", "power_b", int(forwardChannels))
	if err != nil {
		return nil, err
	}
	reverse, err := mixedDirectionSubrequest(request,
		"side_b_reverse", "side_a_reverse", "power_b", "power_a", int(reverseChannels))
	if err != nil {
		return nil, err
	}
	forwardExpansions, err := provider.expandPushPullTranslator(ctx, forward)
	if err != nil || len(forwardExpansions) == 0 {
		return nil, err
	}
	reverseExpansions, err := provider.expandPushPullTranslator(ctx, reverse)
	if err != nil || len(reverseExpansions) == 0 {
		return nil, err
	}
	var result []ProviderExpansion
	for _, forwardExpansion := range forwardExpansions {
		for _, reverseExpansion := range reverseExpansions {
			combined, combineErr := mergeMixedDirectionExpansions(request, forwardExpansion, reverseExpansion)
			if combineErr != nil {
				return nil, combineErr
			}
			combined[0].ID = fmt.Sprintf("mixed_direction_push_pull_%02d", len(result)+1)
			result = append(result, combined...)
		}
	}
	return appendProtocolBusSafetyEvidence(result, nil)
}

func appendProtocolBusSafetyEvidence(expansions []ProviderExpansion, err error) ([]ProviderExpansion, error) {
	if err != nil {
		return nil, err
	}
	safety, err := ObservedCalculation("protocol_bus_safety",
		NamedQuantity{Name: "contention_safe", Value: 1, Unit: "bool"},
		NamedQuantity{Name: "hot_plug_isolation", Value: 1, Unit: "bool"},
		NamedQuantity{Name: "partial_power_down", Value: 1, Unit: "bool"},
	)
	if err != nil {
		return nil, err
	}
	for index := range expansions {
		expansions[index].Calculations = append(expansions[index].Calculations, safety)
	}
	return expansions, nil
}

func hasMixedDirectionProtocolRoles(ports []RoleContract) bool {
	required := []string{"side_a_forward", "side_b_forward", "side_b_reverse", "side_a_reverse"}
	return slices.ContainsFunc(ports, func(port RoleContract) bool {
		return slices.Contains(required, port.Role)
	})
}

func validateMixedDirectionProtocolRoles(ports []RoleContract) error {
	required := []string{
		"side_a_forward", "side_b_forward", "side_b_reverse", "side_a_reverse",
		"power_a", "power_b", "reference", "enable",
	}
	seen := map[string]int{}
	for _, port := range ports {
		seen[port.Role]++
	}
	for _, role := range required {
		if seen[role] != 1 {
			return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis requires exactly one " + role + " role"}
		}
	}
	if len(seen) != len(required) {
		return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction push-pull synthesis received an unsupported role set"}
	}
	directions := map[string]string{
		"side_a_forward": "source",
		"side_b_forward": "sink",
		"side_b_reverse": "source",
		"side_a_reverse": "sink",
	}
	for role, direction := range directions {
		port, _ := mixedDirectionRoleByName(ports, role)
		if canonicalIdentifier(port.Contract.Direction) != direction ||
			port.Contract.Protocol == nil ||
			canonicalIdentifier(port.Contract.Protocol.Mode) != "push_pull" {
			return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "mixed-direction role " + role + " lacks its required push-pull direction contract"}
		}
	}
	return nil
}

func mixedDirectionRoleByName(ports []RoleContract, role string) (RoleContract, bool) {
	for _, port := range ports {
		if port.Role == role {
			return port, true
		}
	}
	return RoleContract{}, false
}

func mixedDirectionSubrequest(
	request ProviderRequest,
	sourceRole string,
	sinkRole string,
	sourcePowerRole string,
	sinkPowerRole string,
	channelCount int,
) (ProviderRequest, error) {
	projected, err := cloneProviderRequest(request)
	if err != nil {
		return ProviderRequest{}, err
	}
	roleMap := map[string]string{
		sourceRole:      "side_a",
		sinkRole:        "side_b",
		sourcePowerRole: "power_a",
		sinkPowerRole:   "power_b",
		"reference":     "reference",
		"enable":        "enable",
	}
	projected.Ports = nil
	for _, port := range request.Ports {
		role, ok := roleMap[port.Role]
		if !ok {
			continue
		}
		port.Role = role
		projected.Ports = append(projected.Ports, port)
	}
	projected.Constraints = slices.DeleteFunc(projected.Constraints, func(constraint Constraint) bool {
		switch canonicalIdentifier(constraint.Name) {
		case "direction", "channel_count", "forward_channel_count", "reverse_channel_count":
			return true
		default:
			return false
		}
	})
	projected.Constraints = mergeProjectedConstraints(projected.Constraints, []Constraint{
		stringConstraint("signaling_mode", "equal", "push_pull"),
		stringConstraint("direction", "equal", "unidirectional"),
		boolConstraint("unpowered_backfeed_prevention", "required"),
		numericConstraint("channel_count", "minimum", float64(channelCount), "count", 0),
	})
	return projected, nil
}

func mergeMixedDirectionExpansions(
	request ProviderRequest,
	forward ProviderExpansion,
	reverse ProviderExpansion,
) ([]ProviderExpansion, error) {
	forwardRealization, err := DecodeFragmentRealization(forward.Payload)
	if err != nil {
		return nil, err
	}
	reverseRealization, err := DecodeFragmentRealization(reverse.Payload)
	if err != nil {
		return nil, err
	}
	remapMixedDirectionRealization(&forwardRealization, "forward", map[string]string{
		"side_a": "side_a_forward", "side_b": "side_b_forward",
		"power_a": "power_a", "power_b": "power_b", "reference": "reference", "enable": "enable",
	})
	remapMixedDirectionRealization(&reverseRealization, "reverse", map[string]string{
		"side_a": "side_b_reverse", "side_b": "side_a_reverse",
		"power_a": "power_b", "power_b": "power_a", "reference": "reference", "enable": "enable",
	})
	for _, shared := range []struct {
		forward string
		reverse string
	}{
		{"forward_translator_power_a", "reverse_translator_power_b"},
		{"forward_translator_power_b", "reverse_translator_power_a"},
		{"forward_translator_reference", "reverse_translator_reference"},
		{"forward_translator_enable", "reverse_translator_enable"},
	} {
		if err := mergeMixedDirectionConnection(&forwardRealization, &reverseRealization, shared.forward, shared.reverse); err != nil {
			return nil, err
		}
	}
	reverseRealization.PortBindings = slices.DeleteFunc(reverseRealization.PortBindings, func(binding RealizationPortBinding) bool {
		switch binding.Role {
		case "power_a", "power_b", "reference", "enable":
			return true
		default:
			return false
		}
	})
	realization := FragmentRealization{
		Capability:   request.Capability,
		Instances:    append(forwardRealization.Instances, reverseRealization.Instances...),
		PortBindings: append(forwardRealization.PortBindings, reverseRealization.PortBindings...),
		Connections:  append(forwardRealization.Connections, reverseRealization.Connections...),
		Parameters:   append(forwardRealization.Parameters, reverseRealization.Parameters...),
	}
	payload, err := MarshalFragmentRealization(realization)
	if err != nil {
		return nil, err
	}
	components := append(prefixSelectedComponents(forward.Components, "forward"), prefixSelectedComponents(reverse.Components, "reverse")...)
	calculations, err := prefixCalculations(forward.Calculations, reverse.Calculations)
	if err != nil {
		return nil, err
	}
	offeredPorts, err := mixedDirectionOfferedPorts(request.Ports, forward.OfferedPorts, reverse.OfferedPorts)
	if err != nil {
		return nil, err
	}
	evidenceSources := append(slices.Clone(forward.Evidence.Sources), reverse.Evidence.Sources...)
	slices.Sort(evidenceSources)
	evidenceSources = slices.Compact(evidenceSources)
	return []ProviderExpansion{{
		ID:           "mixed_direction_push_pull",
		OfferedPorts: offeredPorts,
		Components:   components,
		Calculations: calculations,
		Metrics: ExpansionMetrics{
			UnprovenNonSafety:    forward.Metrics.UnprovenNonSafety + reverse.Metrics.UnprovenNonSafety,
			CatalogSubstitutions: forward.Metrics.CatalogSubstitutions + reverse.Metrics.CatalogSubstitutions,
		},
		Evidence: ContractEvidence{
			Confidence: EvidenceRuleInferred,
			Sources:    evidenceSources,
		},
		Payload: payload,
	}}, nil
}

func mixedDirectionOfferedPorts(
	request []RoleContract,
	forward []RoleContract,
	reverse []RoleContract,
) ([]RoleContract, error) {
	type sourceRole struct {
		ports []RoleContract
		role  string
	}
	sources := map[string]sourceRole{
		"side_a_forward": {forward, "side_a"},
		"side_b_forward": {forward, "side_b"},
		"side_b_reverse": {reverse, "side_a"},
		"side_a_reverse": {reverse, "side_b"},
		"power_a":        {forward, "power_a"},
		"power_b":        {forward, "power_b"},
		"reference":      {forward, "reference"},
		"enable":         {forward, "enable"},
	}
	result := make([]RoleContract, 0, len(request))
	for _, requested := range request {
		source, ok := sources[requested.Role]
		if !ok {
			return nil, fmt.Errorf("mixed-direction push-pull offered-port mapping lacks role %q", requested.Role)
		}
		offered, ok := mixedDirectionRoleByName(source.ports, source.role)
		if !ok {
			return nil, fmt.Errorf("mixed-direction push-pull sub-expansion lacks offered role %q", source.role)
		}
		offered.Role = requested.Role
		offered.Anchor = requested.Anchor
		if requested.Role == "power_a" {
			addMixedDirectionPowerDemand(&offered.Contract, reverse, "power_b")
		}
		if requested.Role == "power_b" {
			addMixedDirectionPowerDemand(&offered.Contract, reverse, "power_a")
		}
		result = append(result, offered)
	}
	return result, nil
}

func addMixedDirectionPowerDemand(contract *PortContract, ports []RoleContract, role string) {
	other, ok := mixedDirectionRoleByName(ports, role)
	if !ok {
		return
	}
	left, leftMaximum, leftOK := availableCurrentDemand(*contract)
	right, rightMaximum, rightOK := availableCurrentDemand(other.Contract)
	if !leftOK || !rightOK {
		return
	}
	value := left + right
	if leftMaximum || rightMaximum {
		contract.MaximumCurrentDemandA = &value
		return
	}
	contract.CurrentDemandA = &value
}

func availableCurrentDemand(contract PortContract) (value float64, maximum bool, ok bool) {
	if contract.MaximumCurrentDemandA != nil {
		return *contract.MaximumCurrentDemandA, true, true
	}
	if contract.CurrentDemandA != nil {
		return *contract.CurrentDemandA, false, true
	}
	return 0, false, false
}

func mergeMixedDirectionConnection(
	target *FragmentRealization,
	source *FragmentRealization,
	targetID string,
	sourceID string,
) error {
	targetIndex := slices.IndexFunc(target.Connections, func(connection RealizationConnection) bool {
		return connection.ID == targetID
	})
	sourceIndex := slices.IndexFunc(source.Connections, func(connection RealizationConnection) bool {
		return connection.ID == sourceID
	})
	if targetIndex < 0 || sourceIndex < 0 {
		return fmt.Errorf("mixed-direction push-pull composition lacks shared connection %q or %q", targetID, sourceID)
	}
	target.Connections[targetIndex].Endpoints = append(
		target.Connections[targetIndex].Endpoints,
		source.Connections[sourceIndex].Endpoints...,
	)
	source.Connections = slices.Delete(source.Connections, sourceIndex, sourceIndex+1)
	return nil
}

func remapMixedDirectionRealization(realization *FragmentRealization, prefix string, roles map[string]string) {
	prefix += "_"
	for index := range realization.Instances {
		realization.Instances[index].ID = prefix + realization.Instances[index].ID
		if realization.Instances[index].Near != "" {
			realization.Instances[index].Near = prefix + realization.Instances[index].Near
		}
	}
	for index := range realization.PortBindings {
		realization.PortBindings[index].Role = roles[realization.PortBindings[index].Role]
		realization.PortBindings[index].Instance = prefix + realization.PortBindings[index].Instance
	}
	for index := range realization.Connections {
		realization.Connections[index].ID = prefix + realization.Connections[index].ID
		for endpointIndex := range realization.Connections[index].Endpoints {
			realization.Connections[index].Endpoints[endpointIndex].Instance =
				prefix + realization.Connections[index].Endpoints[endpointIndex].Instance
		}
	}
	for index := range realization.RepairVariables {
		realization.RepairVariables[index].ID = prefix + realization.RepairVariables[index].ID
		realization.RepairVariables[index].Instance = prefix + realization.RepairVariables[index].Instance
		for instanceIndex := range realization.RepairVariables[index].Instances {
			realization.RepairVariables[index].Instances[instanceIndex] =
				prefix + realization.RepairVariables[index].Instances[instanceIndex]
		}
	}
}

func prefixSelectedComponents(components []SelectedComponent, prefix string) []SelectedComponent {
	result := slices.Clone(components)
	for index := range result {
		result[index].InstanceID = prefix + "_" + result[index].InstanceID
	}
	return result
}

func prefixCalculations(forward, reverse []CalculationEvidence) ([]CalculationEvidence, error) {
	result := make([]CalculationEvidence, 0, len(forward)+len(reverse))
	for _, group := range []struct {
		prefix       string
		calculations []CalculationEvidence
	}{{"forward", forward}, {"reverse", reverse}} {
		for _, calculation := range group.calculations {
			calculation.ID = group.prefix + "_" + strings.TrimSpace(calculation.ID)
			finalized, err := FinalizeCalculation(calculation)
			if err != nil {
				return nil, err
			}
			result = append(result, finalized)
		}
	}
	return result, nil
}

func (provider *CatalogProvider) expandSegmentedOpenDrainBus(
	ctx context.Context,
	request ProviderRequest,
	protocol string,
	mode string,
	direction string,
	segmentCount int,
) ([]ProviderExpansion, error) {
	if protocol != "i2c" && protocol != "smbus" || mode != "open_drain" || direction != "bidirectional" {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus synthesis requires bidirectional I2C or SMBus open-drain signaling"}
	}
	if segmentCount < 2 || segmentCount > maximumProtocolBusSegments {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: fmt.Sprintf("segmented bus count must be between 2 and %d", maximumProtocolBusSegments)}
	}
	rawSegmentCount, _, ok := firstNumericConstraint(request.Constraints, "segment_count")
	// The caller converts the count to int to select this topology. Recheck the
	// original value here so fractional or out-of-range conversions cannot
	// silently select a different number of branches.
	if !ok || rawSegmentCount != math.Trunc(rawSegmentCount) || int(rawSegmentCount) != segmentCount {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus count must be an explicit positive integer"}
	}
	if err := requireBool(request.Constraints, "branch_isolation", "required", true); err != nil {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus synthesis requires explicit branch-isolation behavior"}
	}
	if err := requireBool(request.Constraints, "unpowered_backfeed_prevention", "required", true); err != nil {
		return nil, err
	}
	if err := validateSegmentedBusRoles(request.Ports, segmentCount); err != nil {
		return nil, err
	}

	frequency, _, err := requiredNumber(request.Constraints, "bus_frequency", "minimum", "Hz")
	if err != nil {
		return nil, err
	}
	aggregateLoad, _, aggregateOK := firstNumericConstraint(request.Constraints, "aggregate_load_capacitance")
	trunkLoad, _, trunkOK := firstNumericConstraint(request.Constraints, "trunk_load_capacitance")
	segmentLoad, _, segmentOK := firstNumericConstraint(request.Constraints, "segment_load_capacitance")
	receiverCount, _, receiverOK := firstNumericConstraint(request.Constraints, "receiver_count")
	if !aggregateOK || !trunkOK || !segmentOK || !receiverOK ||
		!finitePositive(aggregateLoad) || !finitePositive(trunkLoad) || !finitePositive(segmentLoad) ||
		receiverCount < float64(segmentCount) || receiverCount != math.Trunc(receiverCount) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus synthesis requires positive aggregate, trunk, per-segment load, and integral receiver-count evidence"}
	}
	covered := trunkLoad + float64(segmentCount)*segmentLoad
	allocationTolerance := math.Max(1e-18, math.Abs(aggregateLoad)*1e-9)
	// These values are declared worst-case load envelopes, not capacity
	// budgets. Every portion of the aggregate envelope must be apportioned to
	// the trunk or a segment; a conservative over-allocation remains safe.
	if covered+allocationTolerance < aggregateLoad {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus load allocation does not cover the declared aggregate capacitance"}
	}

	voltageA, voltageB := roleVoltageMaximum(request.Ports, "power_a"), roleVoltageMaximum(request.Ports, "power_b")
	if !finitePositive(voltageA) || !finitePositive(voltageB) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceVoltageDomainMismatch, message: "segmented bus synthesis requires bounded positive voltage domains"}
	}
	low, high := math.Min(voltageA, voltageB), math.Max(voltageA, voltageB)
	template, err := provider.selectComponent(ctx, "level_translator", "partial_power_down", []components.RequiredRating{
		{Kind: "vcca_supply_voltage", Value: numericString(low), Unit: "V"},
		{Kind: "vccb_supply_voltage", Value: numericString(high), Unit: "V"},
		{Kind: "open_drain_data_rate", Value: numericString(frequency), Unit: "Hz"},
	}, true)
	if err != nil || !translatorEvidenceSupports(template.record, low, high, "open_drain", "bidirectional", frequency, 2, true) {
		return nil, &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "no catalog-backed translator proves segmented-bus voltage, speed, startup, and partial-power-down behavior"}
	}

	trunkRequest := segmentedBusPullupRequest(request, trunkLoad)
	trunkPullup, err := solveOpenDrainPullup(trunkRequest, template, "", voltageA, "max_channel_current_a")
	if err != nil {
		return nil, err
	}
	segmentRequest := segmentedBusPullupRequest(request, segmentLoad)
	segmentPullup, err := solveOpenDrainPullup(segmentRequest, template, "", voltageB, "max_channel_current_a")
	if err != nil {
		return nil, err
	}

	translators := make([]catalogPart, 0, segmentCount)
	parts := make([]catalogPart, 0, 2+5*segmentCount)
	passives := []passivePart{
		{"trunk_sda_pullup", "resistor", "bus_pullup", engineeringValue(trunkPullup.resistance, "Ohm")},
		{"trunk_scl_pullup", "resistor", "bus_pullup", engineeringValue(trunkPullup.resistance, "Ohm")},
	}
	for index := 1; index <= segmentCount; index++ {
		id := segmentedBusRole(index)
		part := template
		part.selected.InstanceID = id + "_translator"
		part.usage = "level_translator"
		translators = append(translators, part)
		parts = append(parts, part)
		passives = append(passives,
			passivePart{id + "_sda_pullup", "resistor", "bus_pullup", engineeringValue(segmentPullup.resistance, "Ohm")},
			passivePart{id + "_scl_pullup", "resistor", "bus_pullup", engineeringValue(segmentPullup.resistance, "Ohm")},
			passivePart{id + "_vcca_bypass", "capacitor", "decoupling", "100n"},
			passivePart{id + "_vccb_bypass", "capacitor", "decoupling", "100n"},
		)
	}
	parts, err = provider.appendPassiveParts(ctx, parts, passives)
	if err != nil {
		return nil, err
	}
	for index := range parts {
		for segment := 1; segment <= segmentCount; segment++ {
			role := segmentedBusRole(segment)
			if parts[index].selected.InstanceID == role+"_sda_pullup" ||
				parts[index].selected.InstanceID == role+"_scl_pullup" ||
				parts[index].selected.InstanceID == role+"_vcca_bypass" ||
				parts[index].selected.InstanceID == role+"_vccb_bypass" {
				parts[index].near = role + "_translator"
				parts[index].maxDistanceMM = 4
			}
		}
	}

	trunkFunctions, segmentFunctions := "A", "B"
	trunkPower, segmentPower := "VCCA", "VCCB"
	if voltageA > voltageB {
		trunkFunctions, segmentFunctions = "B", "A"
		trunkPower, segmentPower = "VCCB", "VCCA"
	}
	// A realization role is an external anchor, so the realization contract
	// permits exactly one direct binding for each power/reference role and one
	// binding per trunk lane. The semantic connections below fan those bound
	// anchors out to every translator power, reference, OE, and trunk
	// endpoint. Catalog power accounting walks all connected parts, not only
	// the endpoint chosen as the external anchor.
	bindings := []RealizationPortBinding{
		{Role: "trunk", Lane: "sda", NetRole: "open_drain_bus", Instance: translators[0].selected.InstanceID, Function: trunkFunctions + "1"},
		{Role: "trunk", Lane: "scl", NetRole: "open_drain_bus", Instance: translators[0].selected.InstanceID, Function: trunkFunctions + "2"},
		{Role: "power_a", Instance: translators[0].selected.InstanceID, Function: trunkPower},
		{Role: "power_b", Instance: translators[0].selected.InstanceID, Function: segmentPower},
		{Role: "reference", Instance: translators[0].selected.InstanceID, Function: "GND"},
	}
	connections := []RealizationConnection{
		semanticNet("segmented_bus_trunk_sda", "open_drain_bus", passiveEndpoint("trunk_sda_pullup", "B")),
		semanticNet("segmented_bus_trunk_scl", "open_drain_bus", passiveEndpoint("trunk_scl_pullup", "B")),
		semanticNet("segmented_bus_power_a", "power", passiveEndpoint("trunk_sda_pullup", "A"), passiveEndpoint("trunk_scl_pullup", "A")),
		semanticNet("segmented_bus_power_b", "power"),
		semanticNet("segmented_bus_reference", "reference"),
	}
	for index, translator := range translators {
		role := segmentedBusRole(index + 1)
		bindings = append(bindings,
			RealizationPortBinding{Role: role, Lane: "sda", NetRole: "open_drain_bus", Instance: translator.selected.InstanceID, Function: segmentFunctions + "1"},
			RealizationPortBinding{Role: role, Lane: "scl", NetRole: "open_drain_bus", Instance: translator.selected.InstanceID, Function: segmentFunctions + "2"},
		)
		connections[0].Endpoints = append(connections[0].Endpoints, endpoint(translator, trunkFunctions+"1"))
		connections[1].Endpoints = append(connections[1].Endpoints, endpoint(translator, trunkFunctions+"2"))
		connections[2].Endpoints = append(connections[2].Endpoints,
			endpoint(translator, trunkPower),
		)
		connections[3].Endpoints = append(connections[3].Endpoints,
			endpoint(translator, segmentPower),
			passiveEndpoint(role+"_sda_pullup", "A"),
			passiveEndpoint(role+"_scl_pullup", "A"),
		)
		if trunkPower == "VCCA" {
			connections[2].Endpoints = append(connections[2].Endpoints,
				endpoint(translator, "OE"),
				passiveEndpoint(role+"_vcca_bypass", "A"),
			)
			connections[3].Endpoints = append(connections[3].Endpoints,
				passiveEndpoint(role+"_vccb_bypass", "A"),
			)
		} else {
			connections[2].Endpoints = append(connections[2].Endpoints,
				passiveEndpoint(role+"_vccb_bypass", "A"),
			)
			connections[3].Endpoints = append(connections[3].Endpoints,
				endpoint(translator, "OE"),
				passiveEndpoint(role+"_vcca_bypass", "A"),
			)
		}
		connections[4].Endpoints = append(connections[4].Endpoints,
			endpoint(translator, "GND"),
			passiveEndpoint(role+"_vcca_bypass", "B"),
			passiveEndpoint(role+"_vccb_bypass", "B"),
		)
		connections = append(connections,
			semanticNet("segmented_bus_"+role+"_sda", "open_drain_bus", endpoint(translator, segmentFunctions+"1"), passiveEndpoint(role+"_sda_pullup", "B")),
			semanticNet("segmented_bus_"+role+"_scl", "open_drain_bus", endpoint(translator, segmentFunctions+"2"), passiveEndpoint(role+"_scl_pullup", "B")),
		)
	}

	calculation, err := ObservedCalculation("segmented_open_drain_bus",
		NamedQuantity{Name: "aggregate_load_capacitance", Value: aggregateLoad, Unit: "F"},
		NamedQuantity{Name: "branch_isolation", Value: 1, Unit: "bool"},
		NamedQuantity{Name: "receiver_count", Value: receiverCount, Unit: "count"},
		NamedQuantity{Name: "segment_count", Value: float64(segmentCount), Unit: "count"},
		NamedQuantity{Name: "segment_load_capacitance", Value: segmentLoad, Unit: "F"},
		NamedQuantity{Name: "trunk_load_capacitance", Value: trunkLoad, Unit: "F"},
	)
	if err != nil {
		return nil, err
	}
	repairs := segmentedBusPullupRepairs(segmentCount, trunkPullup, segmentPullup)
	return provider.expansionWithRepairs(request, "segmented_open_drain_bus", parts, bindings, connections, []CalculationEvidence{calculation}, repairs, 0)
}

func validateSegmentedBusRoles(ports []RoleContract, segmentCount int) error {
	required := map[string]bool{"trunk": false, "power_a": false, "power_b": false, "reference": false}
	for index := 1; index <= segmentCount; index++ {
		required[segmentedBusRole(index)] = false
	}
	for _, port := range ports {
		if _, ok := required[port.Role]; !ok {
			return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus contains an unsupported or ambiguous role " + port.Role}
		}
		if required[port.Role] {
			return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus role is duplicated: " + port.Role}
		}
		required[port.Role] = true
	}
	for role, present := range required {
		if !present {
			return &interfaceSynthesisError{code: CodeInterfaceTranslationUnavailable, message: "segmented bus is missing required role " + role}
		}
	}
	return nil
}

func segmentedBusRole(index int) string {
	return fmt.Sprintf("segment_%02d", index)
}

func segmentedBusPullupRequest(request ProviderRequest, loadCapacitance float64) ProviderRequest {
	projected := request
	projected.Constraints = slices.DeleteFunc(cloneConstraints(request.Constraints), func(constraint Constraint) bool {
		return constraint.Name == "rise_time" || constraint.Name == "load_capacitance"
	})
	riseTime, _, ok := firstNumericConstraint(request.Constraints, "rise_time")
	if ok {
		projected.Constraints = append(projected.Constraints, numericConstraint("rise_time", "maximum", riseTime, "s", 0))
	}
	projected.Constraints = append(projected.Constraints, numericConstraint("load_capacitance", "maximum", loadCapacitance, "F", 0))
	return projected
}

func segmentedBusPullupRepairs(segmentCount int, trunk, segment openDrainPullupSolution) []RealizationRepairVariable {
	repairs := []RealizationRepairVariable{
		pullupRepair("trunk_sda_pullup", trunk),
		pullupRepair("trunk_scl_pullup", trunk),
	}
	for index := 1; index <= segmentCount; index++ {
		role := segmentedBusRole(index)
		repairs = append(repairs,
			pullupRepair(role+"_sda_pullup", segment),
			pullupRepair(role+"_scl_pullup", segment),
		)
	}
	return repairs
}

func pullupRepair(instance string, solution openDrainPullupSolution) RealizationRepairVariable {
	return RealizationRepairVariable{
		ID: instance + "_resistance", Kind: "passive_value", Instance: instance,
		Value: solution.resistance, AllowedValues: slices.Clone(solution.allowedValues), Unit: "Ohm",
		Effects: []RealizationRepairEffect{{Analysis: simmodel.AnalysisTransient, Metric: "rise_time", Direction: "metric_increases"}},
	}
}
