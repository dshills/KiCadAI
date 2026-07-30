package architecturesearch

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestCatalogProviderSynthesizesSegmentedProtocolBus(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := segmentedProtocolBusProviderRequest("smbus", 6)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if realization.Capability != "bus_buffering_level_translation" {
		t.Fatalf("capability = %q", realization.Capability)
	}
	if count := countRealizationUsage(realization, "level_translator"); count != 6 {
		t.Fatalf("translator count = %d, want 6", count)
	}
	if count := countRealizationUsage(realization, "bus_pullup"); count != 14 {
		t.Fatalf("pull-up count = %d, want 14", count)
	}
	for _, connectionID := range []string{"segmented_bus_trunk_sda", "segmented_bus_trunk_scl"} {
		connection := realizationConnectionByID(t, realization, connectionID)
		if len(connection.Endpoints) != 7 {
			t.Fatalf("%s endpoint count = %d, want one pull-up and six translators", connectionID, len(connection.Endpoints))
		}
	}
	for _, connectionID := range []string{"segmented_bus_power_a", "segmented_bus_power_b", "segmented_bus_reference"} {
		connection := realizationConnectionByID(t, realization, connectionID)
		for index := 1; index <= 6; index++ {
			role := segmentedBusRole(index)
			if !slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool {
				return endpoint.Instance == role+"_translator"
			}) {
				t.Fatalf("%s lacks %s translator endpoint: %#v", connectionID, role, connection.Endpoints)
			}
		}
	}
	for _, role := range []string{"power_a", "power_b"} {
		offered := roleContractByRole(t, expansions[0].OfferedPorts, role)
		demand := offered.Contract.MaximumCurrentDemandA
		if demand == nil {
			demand = offered.Contract.CurrentDemandA
		}
		if demand == nil || *demand <= 6e-6 {
			t.Fatalf("%s demand = maximum %#v current %#v, want aggregate demand from all translators", role, offered.Contract.MaximumCurrentDemandA, offered.Contract.CurrentDemandA)
		}
	}
	for index := 1; index <= 6; index++ {
		role := segmentedBusRole(index)
		for _, lane := range []string{"sda", "scl"} {
			if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
				return binding.Role == role && binding.Lane == lane && binding.NetRole == "open_drain_bus"
			}) {
				t.Fatalf("%s lacks %s binding: %#v", role, lane, realization.PortBindings)
			}
		}
		if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
			return connection.ID == "segmented_bus_"+role+"_sda" && len(connection.Endpoints) == 2
		}) {
			t.Fatalf("%s lacks an independently terminated SDA segment", role)
		}
		for _, suffix := range []string{"sda_pullup", "scl_pullup", "vcca_bypass", "vccb_bypass"} {
			if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
				return instance.ID == role+"_"+suffix && instance.Near == role+"_translator" && instance.MaxDistanceMM == 4
			}) {
				t.Fatalf("%s_%s lacks deterministic local placement: %#v", role, suffix, realization.Instances)
			}
		}
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "segmented_open_drain_bus" &&
			slices.ContainsFunc(calculation.NominalOutputs, func(output NamedQuantity) bool {
				return output.Name == "aggregate_load_capacitance" && output.Value == 360e-12
			})
	}) {
		t.Fatalf("whole-bus loading calculation is absent: %#v", expansions[0].Calculations)
	}
	firstPayload := append([]byte(nil), expansions[0].Payload...)
	reordered := request
	reordered.Ports = slices.Clone(request.Ports)
	slices.Reverse(reordered.Ports)
	again, err := provider.Expand(context.Background(), reordered)
	if err != nil || len(again) == 0 {
		t.Fatalf("reordered Expand() = %#v, %v", again, err)
	}
	if string(firstPayload) != string(again[0].Payload) {
		t.Fatalf("segmented realization depends on input role order\nfirst: %s\nagain: %s", firstPayload, again[0].Payload)
	}

	reversed := segmentedProtocolBusProviderRequest("smbus", 6)
	for index := range reversed.Ports {
		switch reversed.Ports[index].Role {
		case "trunk", "power_a":
			reversed.Ports[index].Contract.Voltage = NumericRange{Minimum: float64Pointer(3.3), Maximum: float64Pointer(3.3)}
		case "power_b":
			reversed.Ports[index].Contract.Voltage = NumericRange{Minimum: float64Pointer(1.8), Maximum: float64Pointer(1.8)}
		default:
			if slices.Contains(protocolBusSegmentRoles(6), reversed.Ports[index].Role) {
				reversed.Ports[index].Contract.Voltage = NumericRange{Minimum: float64Pointer(1.8), Maximum: float64Pointer(1.8)}
			}
		}
	}
	reversedExpansions, err := provider.Expand(context.Background(), reversed)
	if err != nil || len(reversedExpansions) == 0 {
		t.Fatalf("reversed-domain Expand() = %#v, %v", reversedExpansions, err)
	}
	reversedRealization, err := DecodeFragmentRealization(reversedExpansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 6; index++ {
		role := segmentedBusRole(index)
		assertEndpointsShareConnection(t, reversedRealization,
			RealizationEndpoint{Instance: role + "_translator", Function: "VCCA"},
			RealizationEndpoint{Instance: role + "_translator", Function: "OE"},
			RealizationEndpoint{Instance: role + "_vcca_bypass", Function: "A"},
		)
		assertEndpointsShareConnection(t, reversedRealization,
			RealizationEndpoint{Instance: role + "_translator", Function: "VCCB"},
			RealizationEndpoint{Instance: role + "_vccb_bypass", Function: "A"},
		)
	}
}

func TestCatalogProviderProtocolBusSupportsI2CSMBusSPIAndUART(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		request ProviderRequest
		usage   string
	}{
		{name: "i2c", request: protocolOpenDrainProviderRequest("i2c"), usage: "level_translator"},
		{name: "smbus", request: protocolOpenDrainProviderRequest("smbus"), usage: "level_translator"},
		{name: "spi", request: protocolPushPullProviderRequest("spi"), usage: "push_pull_level_translator"},
		{name: "uart", request: protocolPushPullProviderRequest("uart"), usage: "push_pull_level_translator"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expansions, err := provider.Expand(context.Background(), test.request)
			if err != nil || len(expansions) == 0 {
				t.Fatalf("Expand() = %#v, %v", expansions, err)
			}
			found := false
			for _, expansion := range expansions {
				realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				found = found || slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
					return instance.Usage == test.usage
				})
			}
			if !found {
				t.Fatalf("protocol architecture lacks usage %q: %#v", test.usage, expansions)
			}
		})
	}
}

func TestCatalogProviderComposesMixedDirectionPushPullBus(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := mixedDirectionProtocolBusProviderRequest("spi", 2, 1)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 2 {
		t.Fatalf("Expand() = %#v, %v, want deterministic alternatives", expansions, err)
	}
	first, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"side_a_forward", "side_b_forward", "side_b_reverse", "side_a_reverse"} {
		if !slices.ContainsFunc(first.PortBindings, func(binding RealizationPortBinding) bool {
			return binding.Role == role
		}) {
			t.Fatalf("mixed-direction realization lacks %s: %#v", role, first.PortBindings)
		}
	}
	for _, role := range []string{"power_a", "power_b", "reference", "enable"} {
		if count := countPortBindingRole(first.PortBindings, role); count != 1 {
			t.Fatalf("%s binding count = %d, want 1", role, count)
		}
	}
	if count := countRealizationUsage(first, "push_pull_level_translator"); count != 2 {
		t.Fatalf("translator count = %d, want 2", count)
	}
	reordered := request
	reordered.Ports = slices.Clone(request.Ports)
	slices.Reverse(reordered.Ports)
	again, err := provider.Expand(context.Background(), reordered)
	if err != nil || len(again) != len(expansions) {
		t.Fatalf("reordered Expand() = %#v, %v", again, err)
	}
	for index := range expansions {
		if string(expansions[index].Payload) != string(again[index].Payload) {
			t.Fatalf("mixed-direction payload %d depends on input role order", index)
		}
	}
}

func TestCatalogProviderMixedDirectionPushPullBusFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ProviderRequest)
	}{
		{name: "missing reverse role", mutate: func(request *ProviderRequest) {
			request.Ports = slices.DeleteFunc(request.Ports, func(port RoleContract) bool {
				return port.Role == "side_b_reverse"
			})
		}},
		{name: "unsafe direction", mutate: func(request *ProviderRequest) {
			for index := range request.Constraints {
				if request.Constraints[index].Name == "direction" {
					request.Constraints[index] = constraintString("direction", "equal", "bidirectional")
				}
			}
		}},
		{name: "missing partial power", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "unpowered_backfeed_prevention"
			})
		}},
		{name: "missing hot plug isolation", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "hot_plug_isolation"
			})
		}},
		{name: "unsafe contention policy", mutate: func(request *ProviderRequest) {
			for index := range request.Constraints {
				if request.Constraints[index].Name == "contention_policy" {
					request.Constraints[index] = constraintString("contention_policy", "equal", "allow_overlap")
				}
			}
		}},
		{name: "active enable at startup", mutate: func(request *ProviderRequest) {
			for index := range request.Ports {
				if request.Ports[index].Role == "enable" {
					request.Ports[index].Contract.DefaultState = "active"
				}
			}
		}},
		{name: "zero return channels", mutate: func(request *ProviderRequest) {
			replaceProtocolBusNumber(request, "reverse_channel_count", 0)
		}},
		{name: "ambiguous lane direction", mutate: func(request *ProviderRequest) {
			for index := range request.Ports {
				if request.Ports[index].Role == "side_a_reverse" {
					request.Ports[index].Contract.Direction = "source"
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := mixedDirectionProtocolBusProviderRequest("uart", 1, 1)
			test.mutate(&request)
			_, err := provider.Expand(context.Background(), request)
			var typed *interfaceSynthesisError
			if !errors.As(err, &typed) {
				t.Fatalf("error = %#v, want typed fail-closed interface error", err)
			}
		})
	}
}

func TestAddMixedDirectionPowerDemandConservativelyCombinesMixedEvidence(t *testing.T) {
	nominal := 2e-6
	maximum := 7e-6
	contract := PortContract{CurrentDemandA: &nominal}
	ports := []RoleContract{{
		Role: "power", Contract: PortContract{MaximumCurrentDemandA: &maximum},
	}}
	addMixedDirectionPowerDemand(&contract, ports, "power")
	if contract.MaximumCurrentDemandA == nil || *contract.MaximumCurrentDemandA != nominal+maximum {
		t.Fatalf("combined maximum demand = %#v, want %.12g", contract.MaximumCurrentDemandA, nominal+maximum)
	}
}

func TestCatalogProviderSegmentedProtocolBusFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ProviderRequest)
	}{
		{name: "partial power proof", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "unpowered_backfeed_prevention"
			})
		}},
		{name: "hot plug proof", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "hot_plug_isolation"
			})
		}},
		{name: "contention policy", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "contention_policy"
			})
		}},
		{name: "branch isolation", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "branch_isolation"
			})
		}},
		{name: "aggregate allocation", mutate: func(request *ProviderRequest) {
			replaceProtocolBusNumber(request, "aggregate_load_capacitance", 500e-12)
		}},
		{name: "receiver fanout", mutate: func(request *ProviderRequest) {
			replaceProtocolBusNumber(request, "receiver_count", 5)
		}},
		{name: "missing segment role", mutate: func(request *ProviderRequest) {
			request.Ports = slices.DeleteFunc(request.Ports, func(port RoleContract) bool {
				return port.Role == "segment_04"
			})
		}},
		{name: "segment bound", mutate: func(request *ProviderRequest) {
			replaceProtocolBusNumber(request, "segment_count", maximumProtocolBusSegments+1)
		}},
		{name: "impossible rise time", mutate: func(request *ProviderRequest) {
			replaceProtocolBusNumber(request, "rise_time", 1e-12)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := segmentedProtocolBusProviderRequest("i2c", 6)
			test.mutate(&request)
			_, err := provider.Expand(context.Background(), request)
			var typed *interfaceSynthesisError
			if !errors.As(err, &typed) {
				t.Fatalf("error = %#v, want typed fail-closed interface error", err)
			}
		})
	}
}

func segmentedProtocolBusProviderRequest(protocol string, segments int) ProviderRequest {
	trunk := protocolBusRole("trunk", protocol, "bidirectional", 1.8)
	ports := []RoleContract{trunk}
	for index := 1; index <= segments; index++ {
		ports = append(ports, protocolBusRole(segmentedBusRole(index), protocol, "bidirectional", 3.3))
	}
	ports = append(ports,
		providerRole("power_a", "power", "sink", 1.8, 1.8),
		providerRole("power_b", "power", "sink", 3.3, 3.3),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	)
	return ProviderRequest{
		Capability: "bus_buffering_level_translation",
		Ports:      ports,
		Constraints: []Constraint{
			constraintString("protocol", "equal", protocol),
			constraintString("signaling_mode", "equal", "open_drain"),
			constraintString("direction", "equal", "bidirectional"),
			constraintNumber("bus_frequency", "minimum", 1_000_000, "Hz", 0),
			constraintNumber("segment_count", "equal", float64(segments), "count", 0),
			constraintNumber("receiver_count", "minimum", float64(segments), "count", 0),
			constraintNumber("aggregate_load_capacitance", "maximum", 360e-12, "F", 0),
			constraintNumber("trunk_load_capacitance", "maximum", 30e-12, "F", 0),
			constraintNumber("segment_load_capacitance", "maximum", 55e-12, "F", 0),
			constraintNumber("rise_time", "maximum", 120e-9, "s", 0),
			constraintBool("branch_isolation", "required", true),
			constraintBool("unpowered_backfeed_prevention", "required", true),
			constraintBool("hot_plug_isolation", "required", true),
			constraintString("contention_policy", "equal", "wired_low"),
		},
	}
}

func protocolOpenDrainProviderRequest(protocol string) ProviderRequest {
	request := translatorProviderRequest(1.8, 3.3)
	request.Capability = "bus_buffering_level_translation"
	for index := range request.Ports {
		if request.Ports[index].Contract.Protocol != nil {
			request.Ports[index].Contract.Protocol.Name = protocol
		}
	}
	for index := range request.Constraints {
		if request.Constraints[index].Name == "protocol" {
			request.Constraints[index] = constraintString("protocol", "equal", protocol)
		}
	}
	request.Constraints = append(request.Constraints,
		constraintNumber("load_capacitance", "maximum", 80e-12, "F", 0),
		constraintNumber("rise_time", "maximum", 300e-9, "s", 0),
		constraintBool("hot_plug_isolation", "required", true),
		constraintString("contention_policy", "equal", "wired_low"),
	)
	return request
}

func protocolPushPullProviderRequest(protocol string) ProviderRequest {
	request := pushPullTranslatorProviderRequest(1.8, 3.3, 2, 4_000_000)
	request.Capability = "bus_buffering_level_translation"
	for index := range request.Ports {
		if request.Ports[index].Contract.Protocol != nil {
			request.Ports[index].Contract.Protocol.Name = protocol
		}
	}
	request.Constraints = append(request.Constraints,
		constraintString("signaling_mode", "equal", "push_pull"),
		constraintString("direction", "equal", "unidirectional"),
		constraintBool("unpowered_backfeed_prevention", "required", true),
		constraintBool("hot_plug_isolation", "required", true),
		constraintString("contention_policy", "equal", "exclusive_drivers"),
	)
	return request
}

func mixedDirectionProtocolBusProviderRequest(protocol string, forwardChannels, reverseChannels int) ProviderRequest {
	forwardA := providerRole("side_a_forward", "digital_bus", "source", 0, 1.8)
	forwardB := providerRole("side_b_forward", "digital_bus", "sink", 0, 3.3)
	reverseB := providerRole("side_b_reverse", "digital_bus", "source", 0, 3.3)
	reverseA := providerRole("side_a_reverse", "digital_bus", "sink", 0, 1.8)
	for _, port := range []*RoleContract{&forwardA, &forwardB, &reverseB, &reverseA} {
		port.Contract.Protocol = &Protocol{Name: protocol, Mode: "push_pull", MaxFrequencyHz: 8_000_000}
		port.Contract.DefaultState = "inactive"
	}
	enable := providerRole("enable", "digital_logic", "sink", 0, 1.8)
	enable.Contract.DefaultState = "inactive"
	return ProviderRequest{
		Capability: "bus_buffering_level_translation",
		Ports: []RoleContract{
			forwardA, forwardB, reverseB, reverseA,
			providerRole("power_a", "power", "sink", 1.8, 1.8),
			providerRole("power_b", "power", "sink", 3.3, 3.3),
			providerRole("reference", "reference", "bidirectional", 0, 0),
			enable,
		},
		Constraints: []Constraint{
			constraintString("protocol", "equal", protocol),
			constraintString("signaling_mode", "equal", "push_pull"),
			constraintString("direction", "equal", "mixed_unidirectional"),
			constraintNumber("forward_channel_count", "minimum", float64(forwardChannels), "count", 0),
			constraintNumber("reverse_channel_count", "minimum", float64(reverseChannels), "count", 0),
			constraintBool("unpowered_backfeed_prevention", "required", true),
			constraintBool("hot_plug_isolation", "required", true),
			constraintString("contention_policy", "equal", "exclusive_drivers"),
		},
	}
}

func protocolBusRole(role, protocol, direction string, voltage float64) RoleContract {
	result := providerRole(role, "digital_bus", direction, 0, voltage)
	result.Contract.Protocol = &Protocol{Name: protocol, Mode: "open_drain", MaxFrequencyHz: 1_000_000}
	result.Contract.DefaultState = "inactive"
	return result
}

func replaceProtocolBusNumber(request *ProviderRequest, name string, value float64) {
	for index := range request.Constraints {
		if request.Constraints[index].Name != name {
			continue
		}
		var raw json.RawMessage
		raw, _ = json.Marshal(value)
		request.Constraints[index].Value = raw
		return
	}
}

func protocolBusSegmentRoles(count int) []string {
	roles := make([]string, 0, count)
	for index := 1; index <= count; index++ {
		roles = append(roles, segmentedBusRole(index))
	}
	return roles
}

func assertEndpointsShareConnection(t *testing.T, realization FragmentRealization, endpoints ...RealizationEndpoint) {
	t.Helper()
	for _, connection := range realization.Connections {
		allFound := true
		for _, endpoint := range endpoints {
			allFound = allFound && slices.Contains(connection.Endpoints, endpoint)
		}
		if allFound {
			return
		}
	}
	t.Fatalf("endpoints do not share one connection: %#v in %#v", endpoints, realization.Connections)
}

func countPortBindingRole(bindings []RealizationPortBinding, role string) int {
	count := 0
	for _, binding := range bindings {
		if binding.Role == role {
			count++
		}
	}
	return count
}

func realizationConnectionByID(t *testing.T, realization FragmentRealization, id string) RealizationConnection {
	t.Helper()
	for _, connection := range realization.Connections {
		if connection.ID == id {
			return connection
		}
	}
	t.Fatalf("realization lacks connection %q", id)
	return RealizationConnection{}
}

func roleContractByRole(t *testing.T, ports []RoleContract, role string) RoleContract {
	t.Helper()
	for _, port := range ports {
		if port.Role == role {
			return port
		}
	}
	t.Fatalf("offered ports lack role %q", role)
	return RoleContract{}
}
