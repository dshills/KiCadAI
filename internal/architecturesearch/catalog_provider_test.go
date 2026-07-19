package architecturesearch

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
)

func TestCatalogProviderSearchesSyntheticThresholdDeterministically(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	registry, issues := NewCatalogRegistry(catalog)
	if len(issues) != 0 {
		t.Fatalf("registry issues = %#v", issues)
	}
	requirement := validRequirement()
	requirement.Requirements.Objectives[0].Constraints = append(requirement.Requirements.Objectives[0].Constraints,
		constraintNumber("hysteresis_width", "target", 0.2, "V", 10),
		constraintString("output_polarity", "equal", "active_low"),
		constraintNumber("propagation_delay", "maximum", 10, "us", 0),
	)
	result := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "synthetic-catalog"})
	if result.Status != SearchSelected || result.Selected == nil || len(result.Selected.Selections) != 1 {
		t.Fatalf("catalog search = %#v", result)
	}
	selection := result.Selected.Selections[0]
	if selection.ProviderID != "catalog_function_fragments" || len(selection.Calculations) != 3 || len(selection.Components) != 6 {
		t.Fatalf("selection = %#v", selection)
	}
	if !slices.ContainsFunc(selection.Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "catalog_power_current_demand"
	}) {
		t.Fatalf("selection lacks catalog-backed power demand: %#v", selection.Calculations)
	}
	realization, err := DecodeFragmentRealization(selection.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if realization.Capability != "threshold_detection" || len(realization.PortBindings) != 4 || len(realization.Connections) != 4 {
		t.Fatalf("realization = %#v", realization)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"synthetic_threshold", "Synthetic threshold", "objective:detect", "external:"} {
		if strings.Contains(string(selection.Payload), forbidden) {
			t.Fatalf("provider payload contains identity %q: %s", forbidden, selection.Payload)
		}
	}
	second := Search(context.Background(), requirement, registry, SearchOptions{CatalogHash: "synthetic-catalog"})
	secondEncoded, _ := json.Marshal(second)
	if string(encoded) != string(secondEncoded) {
		t.Fatalf("catalog provider replay differs\n%s\n%s", encoded, secondEncoded)
	}
}

func TestCatalogProviderGenericCapabilityMutations(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		request   ProviderRequest
		wantError bool
	}{
		{name: "threshold_in_range", request: thresholdProviderRequest(5, 1.65, 0.2)},
		{name: "threshold_supply_too_low", request: thresholdProviderRequest(1.8, 0.9, 0.1), wantError: true},
		{name: "load_switch_in_range", request: loadSwitchProviderRequest(13.2, 2)},
		{name: "load_switch_voltage_out_of_range", request: loadSwitchProviderRequest(250, 2), wantError: true},
		{name: "adjustable_regulator_in_range", request: regulatorProviderRequest(5.5, 3.3, 0.25)},
		{name: "adjustable_regulator_input_out_of_range", request: regulatorProviderRequest(50, 5, 0.25), wantError: true},
		{name: "filter_in_range", request: filterProviderRequest(5, 2000)},
		{name: "filter_supply_out_of_range", request: filterProviderRequest(50, 2000), wantError: true},
		{name: "translator_in_range", request: translatorProviderRequest(3.3, 1.8)},
		{name: "translator_low_domain_out_of_range", request: translatorProviderRequest(3.3, 1.2), wantError: true},
		{name: "controller_in_range", request: participantProviderRequest("programmable_controller", "sensor_bus", 3.3)},
		{name: "controller_supply_out_of_range", request: participantProviderRequest("programmable_controller", "sensor_bus", 6), wantError: true},
		{name: "sensor_in_range", request: participantProviderRequest("environment_sensor", "controller_bus", 1.8)},
		{name: "sensor_supply_out_of_range", request: participantProviderRequest("environment_sensor", "controller_bus", 1.6), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expansions, err := provider.Expand(context.Background(), test.request)
			if test.wantError {
				if err == nil || len(expansions) != 0 {
					t.Fatalf("Expand() = %#v, %v; want fail-closed error", expansions, err)
				}
				return
			}
			if err != nil || len(expansions) == 0 {
				t.Fatalf("Expand() = %#v, %v", expansions, err)
			}
			if expansions[0].Evidence.Confidence != EvidenceRuleInferred || len(expansions[0].Components) == 0 {
				t.Fatalf("expansion evidence = %#v", expansions[0])
			}
			if _, err := DecodeFragmentRealization(expansions[0].Payload); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCatalogProviderUsesRatedReverseBlockingPowerPathWhenRequired(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	input := providerRole("input", "power", "sink", 1.7, 1.9)
	output := providerRole("output", "power", "source", 1.7, 1.9)
	current := 0.08
	input.Contract.RequiredCurrentCapacityA = &current
	output.Contract.MaximumCurrentDemandA = &current
	expansions, err := provider.Expand(context.Background(), ProviderRequest{
		Capability:  "transient_protection",
		Ports:       []RoleContract{input, output, providerRole("reference", "reference", "bidirectional", 0, 0)},
		Constraints: []Constraint{constraintBool("reverse_current_blocking", "required", true)},
	})
	if err != nil || len(expansions) < 2 {
		t.Fatalf("expansions = %#v err = %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.CatalogID == "protection.ti.tps22917dbv.sot23_6" && slices.Contains(instance.RequiredFunctions, "VOUT")
	}) {
		t.Fatalf("reverse-blocking realization = %#v", realization)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool {
			return endpoint.Instance == "reverse_blocking_switch" && endpoint.Function == "VOUT"
		})
	}) {
		t.Fatalf("reverse-blocking output is not connected: %#v", realization.Connections)
	}
}

func TestCatalogProviderOffersAndRanksRealFilterAlternative(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, _ := NewCatalogProvider(catalog)
	expansions, err := provider.Expand(context.Background(), filterProviderRequest(5, 2000))
	if err != nil || len(expansions) < 2 {
		t.Fatalf("filter expansions = %#v, %v", expansions, err)
	}
	hasDifferentComponentCount := false
	for _, expansion := range expansions[1:] {
		if len(expansions[0].Components) != len(expansion.Components) {
			hasDifferentComponentCount = true
			break
		}
	}
	if !hasDifferentComponentCount {
		t.Fatalf("filter alternatives are not distinct: %#v", expansions)
	}
}

func TestCatalogProviderConnectsAuxiliaryMCUSupplyPinsToTheirDomains(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), participantProviderRequest("programmable_controller", "sensor_bus", 3.3))
	if err != nil || len(expansions) < 1 {
		t.Fatalf("controller expansion = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]bool{
		"controller_power":  {"VCC": false, "AVCC": false},
		"controller_ground": {"GND": false, "AGND": false},
	}
	for _, connection := range realization.Connections {
		functions, ok := want[connection.ID]
		if !ok {
			continue
		}
		for _, endpoint := range connection.Endpoints {
			if _, expected := functions[endpoint.Function]; expected {
				functions[endpoint.Function] = true
			}
		}
	}
	for connection, functions := range want {
		for function, found := range functions {
			if !found {
				t.Fatalf("%s does not contain %s: %#v", connection, function, realization.Connections)
			}
		}
	}
}

func TestCatalogProviderIsolatesSensorAddressStrapFromPowerFlagDomain(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), participantProviderRequest("environment_sensor", "sensor_bus", 1.8))
	if err != nil || len(expansions) < 1 {
		t.Fatalf("sensor expansion = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	foundAddress := false
	for _, connection := range realization.Connections {
		for _, endpoint := range connection.Endpoints {
			if connection.ID == "sensor_ground" && endpoint.Function == "SDO" {
				t.Fatalf("address-select pin was tied directly to the flagged ground domain: %#v", connection)
			}
			if connection.ID == "sensor_address" && endpoint.Function == "SDO" {
				foundAddress = true
			}
		}
	}
	if !foundAddress {
		t.Fatalf("sensor address strap is missing: %#v", realization.Connections)
	}
}

func TestCatalogProviderOutputIgnoresCatalogOrdering(t *testing.T) {
	firstCatalog := loadArchitectureCatalog(t)
	secondCatalog := loadArchitectureCatalog(t)
	slices.Reverse(secondCatalog.Records)
	components.SortCatalog(secondCatalog)
	first, _ := NewCatalogProvider(firstCatalog)
	second, _ := NewCatalogProvider(secondCatalog)
	request := translatorProviderRequest(3.3, 1.8)
	firstExpansion, firstErr := first.Expand(context.Background(), request)
	secondExpansion, secondErr := second.Expand(context.Background(), request)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("expand errors = %v, %v", firstErr, secondErr)
	}
	firstJSON, _ := json.Marshal(firstExpansion)
	secondJSON, _ := json.Marshal(secondExpansion)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("catalog order changed expansion bytes\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestRecordSupportsEveryRequiredRating(t *testing.T) {
	record := components.ComponentRecord{Ratings: []components.RatingConstraint{
		{Kind: "supply_voltage", Min: "2.2", Max: "36", Unit: "V"},
		{Kind: "output_sink_current", Max: "0.02", Unit: "A"},
	}}
	if !recordSupportsRatings(record, []components.RequiredRating{
		{Kind: "supply_voltage", Value: "12", Unit: "V"},
		{Kind: "output_sink_current", Value: "10", Unit: "mA"},
	}) {
		t.Fatal("record satisfying every required rating was rejected")
	}
	if recordSupportsRatings(record, []components.RequiredRating{
		{Kind: "supply_voltage", Value: "12", Unit: "V"},
		{Kind: "output_sink_current", Value: "25", Unit: "mA"},
	}) {
		t.Fatal("record satisfying only the first required rating was accepted")
	}
	if recordSupportsRatings(record, []components.RequiredRating{{Kind: "power_dissipation", Value: "1", Unit: "W"}}) {
		t.Fatal("record missing a required rating was accepted")
	}
	if !recordSupportsRatings(components.ComponentRecord{Ratings: []components.RatingConstraint{{Kind: "voltage", Max: "0.3", Unit: "V"}}}, []components.RequiredRating{{Kind: "voltage", Value: numericString(0.1 + 0.2), Unit: "V"}}) {
		t.Fatal("quantized floating-point boundary was rejected")
	}
}

func TestCatalogPowerDemandUsesSelectedPartEvidence(t *testing.T) {
	maximum := 0.1
	request := ProviderRequest{Capability: "synthetic_powered_fragment", Ports: []RoleContract{{
		Role: "power", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: float64Pointer(4.5), Maximum: float64Pointer(5.5)}, MaximumCurrentDemandA: &maximum},
	}}}
	powered := catalogPart{
		selected: SelectedComponent{InstanceID: "active", CatalogID: "active.synthetic", VariantID: "package"},
		record: components.ComponentRecord{
			ID: "active.synthetic", Family: "active",
			Ratings: []components.RatingConstraint{{Kind: "supply_current", Max: "2.4", Unit: "mA"}},
			Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{{Function: "VCC", Electrical: "power_in"}}}},
		},
	}
	demand, proven, calculations, err := catalogFragmentPowerDemand(request, []catalogPart{powered}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proven["power"] || len(calculations) != 1 || demand["power"] != 0.0024 {
		t.Fatalf("demand=%v proven=%v calculations=%#v", demand, proven, calculations)
	}
	parts := []catalogPart{{
		selected: SelectedComponent{InstanceID: "active", CatalogID: "active.synthetic", VariantID: "package"},
		record: components.ComponentRecord{
			ID: "active.synthetic", Family: "active",
			Ratings: []components.RatingConstraint{{Kind: "supply_current", Max: "2.4", Unit: "mA"}},
			Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{{Function: "VCC", Electrical: "power_in"}}}},
		},
	}}
	for index := 0; index < 20; index++ {
		parts = append(parts, catalogPart{
			selected: SelectedComponent{InstanceID: "passive_" + numericString(float64(index))},
			record:   components.ComponentRecord{Family: "capacitor"}, value: "100n",
		})
	}
	second, secondProven, _, err := catalogFragmentPowerDemand(request, parts, nil, nil, nil)
	if err != nil || !secondProven["power"] || second["power"] != demand["power"] {
		t.Fatalf("passive count changed demand: first=%v second=%v proven=%v err=%v", demand, second, secondProven, err)
	}
}

func TestCatalogPowerDemandFallsBackToRequestCeilingWithoutEvidence(t *testing.T) {
	maximum := 0.01
	request := ProviderRequest{Capability: "synthetic_powered_fragment", Ports: []RoleContract{{
		Role: "power", Contract: PortContract{Kind: "power", Direction: "sink", MaximumCurrentDemandA: &maximum},
	}}}
	part := catalogPart{record: components.ComponentRecord{Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{{Function: "VCC", Electrical: "power_in"}}}}}}
	_, proven, calculations, err := catalogFragmentPowerDemand(request, []catalogPart{part}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if proven["power"] || len(calculations) != 0 {
		t.Fatalf("missing evidence was treated as proven: proven=%v calculations=%#v", proven, calculations)
	}
	offered := offeredCatalogPorts(request, nil, nil)
	if len(offered) != 1 || offered[0].Contract.CurrentDemandA == nil || *offered[0].Contract.CurrentDemandA != maximum {
		t.Fatalf("fallback ports = %#v", offered)
	}
}

func TestCatalogPowerDemandSolvesStaticResistorNetwork(t *testing.T) {
	maximum := 0.01
	request := ProviderRequest{Capability: "synthetic_divider", Ports: []RoleContract{
		{Role: "power", Contract: PortContract{Kind: "power", Direction: "sink", Voltage: NumericRange{Minimum: float64Pointer(5), Maximum: float64Pointer(5)}, MaximumCurrentDemandA: &maximum}},
		{Role: "reference", Contract: PortContract{Kind: "reference", Direction: "bidirectional", Voltage: NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(0)}}},
	}}
	parts := []catalogPart{
		{selected: SelectedComponent{InstanceID: "upper"}, record: components.ComponentRecord{Family: "resistor"}, value: "10k"},
		{selected: SelectedComponent{InstanceID: "lower"}, record: components.ComponentRecord{Family: "resistor"}, value: "10k"},
	}
	bindings := []RealizationPortBinding{{Role: "power", Instance: "upper", Function: "A"}, {Role: "reference", Instance: "lower", Function: "B"}}
	connections := []RealizationConnection{semanticNet("midpoint", "analog_signal", passiveEndpoint("upper", "B"), passiveEndpoint("lower", "A"))}
	demand, proven, _, err := catalogFragmentPowerDemand(request, parts, bindings, nil, connections)
	if err != nil {
		t.Fatal(err)
	}
	if !proven["power"] || math.Abs(demand["power"]-0.00025) > 1e-12 {
		t.Fatalf("divider demand=%v proven=%v", demand, proven)
	}
}

func TestCatalogPowerDemandIsAccountedPerRail(t *testing.T) {
	maximum := 0.01
	request := ProviderRequest{Capability: "synthetic_multi_rail", Ports: []RoleContract{
		{Role: "power_a", Contract: PortContract{Kind: "power", Direction: "sink", Domain: "a", Voltage: NumericRange{Minimum: float64Pointer(5), Maximum: float64Pointer(5)}, MaximumCurrentDemandA: &maximum}},
		{Role: "power_b", Contract: PortContract{Kind: "power", Direction: "sink", Domain: "b", Voltage: NumericRange{Minimum: float64Pointer(3.3), Maximum: float64Pointer(3.3)}, MaximumCurrentDemandA: &maximum}},
		{Role: "reference", Contract: PortContract{Kind: "reference", Direction: "bidirectional", Voltage: NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(0)}}},
	}}
	part := catalogPart{record: components.ComponentRecord{
		Ratings: []components.RatingConstraint{{Kind: "supply_current", Max: "1", Unit: "mA"}},
		Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{{Function: "VCCA", Electrical: "power_in"}, {Function: "VCCB", Electrical: "power_in"}}}},
	}}
	demand, proven, calculations, err := catalogFragmentPowerDemand(request, []catalogPart{part}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proven["power_a"] || !proven["power_b"] || demand["power_a"] != 0.001 || demand["power_b"] != 0.001 || len(calculations) != 2 {
		t.Fatalf("per-rail demand=%v proven=%v calculations=%#v", demand, proven, calculations)
	}
}

func TestGPIOAllocationRequiresPinCapabilitiesAndAvoidsAliasReuse(t *testing.T) {
	record := components.ComponentRecord{Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{
		{Function: "GPIO_1"},
		{Function: "GPIO_2", Aliases: []string{"ADC0"}},
		{Function: "GPIO_3", Aliases: []string{"PWM_OC1"}},
		{Function: "GPIO_4", Aliases: []string{"I2C_SDA"}},
		{Function: "P0.1"},
		{Function: "PA0", Aliases: []string{"ADC1"}},
		{Function: "PWR1"},
	}}}}
	if got := availableGPIOFunctions(record, PortContract{Kind: "digital_logic", Direction: "source"}, map[string]bool{"I2C_SDA": true}); !slices.Equal(got, []string{"GPIO_1", "GPIO_2", "GPIO_3", "P0.1", "PA0"}) {
		t.Fatalf("digital GPIO candidates = %v", got)
	}
	if got := availableGPIOFunctions(record, PortContract{Kind: "analog_voltage", Direction: "sink"}, nil); !slices.Equal(got, []string{"GPIO_2", "PA0"}) {
		t.Fatalf("ADC candidates = %v", got)
	}
	if got := availableGPIOFunctions(record, PortContract{Kind: "analog_control", Direction: "source"}, nil); !slices.Equal(got, []string{"GPIO_3"}) {
		t.Fatalf("PWM candidates = %v", got)
	}
	if got := availableGPIOFunctions(record, PortContract{Kind: "analog_voltage", Direction: "source"}, nil); len(got) != 0 {
		t.Fatalf("digital-only record offered a DAC candidate: %v", got)
	}
}

func TestCatalogPowerDemandAddsActiveLoadToAlternativeConversionBound(t *testing.T) {
	maximum := 1.0
	request := ProviderRequest{Capability: "synthetic_converter", Ports: []RoleContract{
		{Role: "input", Contract: PortContract{Kind: "power", Direction: "sink", Domain: "input", Voltage: NumericRange{Minimum: float64Pointer(5), Maximum: float64Pointer(5)}, MaximumCurrentDemandA: &maximum}},
		{Role: "output", Contract: PortContract{Kind: "power", Direction: "source", Domain: "output", Voltage: NumericRange{Minimum: float64Pointer(12), Maximum: float64Pointer(12)}, RequiredCurrentCapacityA: float64Pointer(0.1)}},
	}}
	part := catalogPart{record: components.ComponentRecord{
		Ratings: []components.RatingConstraint{{Kind: "supply_current", Max: "1", Unit: "mA"}},
		Values:  []components.ValueConstraint{{Kind: "efficiency", Typ: "80", Unit: "%"}},
		Symbols: []components.SymbolBinding{{FunctionPins: []components.FunctionPin{{Function: "VIN", Electrical: "power_in"}}}},
	}}
	demand, proven, _, err := catalogFragmentPowerDemand(request, []catalogPart{part}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proven["input"] || math.Abs(demand["input"]-0.301) > 1e-12 {
		t.Fatalf("converter demand=%v proven=%v", demand, proven)
	}
}

func loadArchitectureCatalog(t *testing.T) *components.Catalog {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func thresholdProviderRequest(supply, center, width float64) ProviderRequest {
	return ProviderRequest{Capability: "threshold_detection", Ports: []RoleContract{
		providerRole("sense", "analog_voltage", "sink", 0, supply),
		providerRole("output", "digital_logic", "source", 0, supply),
		providerRole("power", "power", "sink", supply, supply),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("threshold_voltage", "target", center, "V", 2),
		constraintNumber("hysteresis_width", "target", width, "V", 10),
		constraintString("output_polarity", "equal", "active_low"),
		constraintBool("inactive_at_power_up", "required", true),
		constraintNumber("propagation_delay", "maximum", 10, "us", 0),
	}}
}

func loadSwitchProviderRequest(voltage, current float64) ProviderRequest {
	return ProviderRequest{Capability: "load_switch", Ports: []RoleContract{
		providerRole("control", "digital_logic", "sink", 0, 3.3),
		providerRole("load", "switched_load", "sink", 0, voltage),
		providerRole("load_power", "power", "sink", voltage, voltage),
		providerRole("logic_power", "power", "sink", 3.3, 3.3),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_current", "minimum", current, "A", 0),
		constraintNumber("load_voltage", "minimum", voltage, "V", 0),
		constraintString("load_characteristic", "equal", "inductive"),
		constraintString("control_active_state", "equal", "high"),
		constraintBool("default_off", "required", true),
		constraintBool("inductive_transient_clamp", "required", true),
		constraintBool("control_overvoltage_clamp", "required", true),
	}}
}

func regulatorProviderRequest(inputMaximum, output, current float64) ProviderRequest {
	return ProviderRequest{Capability: "voltage_regulation", Ports: []RoleContract{
		providerRole("input", "power", "sink", 2.5, inputMaximum),
		providerRole("output", "power", "source", output*0.98, output*1.02),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("output_voltage", "target", output, "V", 2),
		constraintNumber("continuous_output_current", "minimum", current, "A", 0),
		constraintRange("input_voltage", "range", 2.5, inputMaximum, "V"),
		constraintBool("adjustable_output", "required", true),
		constraintString("set_point_programming", "equal", "passive_feedback"),
		constraintBool("input_decoupling", "required", true),
		constraintBool("output_decoupling", "required", true),
	}}
}

func filterProviderRequest(supply, frequency float64) ProviderRequest {
	return ProviderRequest{Capability: "frequency_filter", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 0.5, supply-0.5),
		providerRole("output", "analog_voltage", "source", 0.5, supply-0.5),
		providerRole("power", "power", "sink", supply, supply),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintString("response", "equal", "low_pass"),
		constraintString("approximation", "equal", "butterworth"),
		constraintNumber("order", "equal", 4, "", 0),
		constraintNumber("cutoff_frequency", "target", frequency, "Hz", 5),
		constraintNumber("passband_gain", "target", 1, "ratio", 2),
		constraintNumber("passband_ripple", "maximum", 0.5, "dB", 0),
	}}
}

func translatorProviderRequest(sideA, sideB float64) ProviderRequest {
	busA := providerRole("side_a", "digital_bus", "bidirectional", 0, sideA)
	busB := providerRole("side_b", "digital_bus", "bidirectional", 0, sideB)
	busA.Contract.Protocol = &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400000}
	busB.Contract.Protocol = &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400000}
	return ProviderRequest{Capability: "logic_level_translation", Ports: []RoleContract{
		busA, busB,
		providerRole("power_a", "power", "sink", sideA, sideA),
		providerRole("power_b", "power", "sink", sideB, sideB),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintString("protocol", "equal", "i2c"),
		constraintString("signaling_mode", "equal", "open_drain"),
		constraintString("direction", "equal", "bidirectional"),
		constraintNumber("bus_frequency", "minimum", 400000, "Hz", 0),
		constraintBool("unpowered_backfeed_prevention", "required", true),
	}}
}

func participantProviderRequest(capability, role string, supply float64) ProviderRequest {
	port := providerRole(role, "digital_bus", "bidirectional", supply*0.95, supply*1.05)
	port.Contract.Protocol = &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400000}
	constraints := []Constraint{constraintBool("programmable_interface", "required", true)}
	if capability == "environment_sensor" {
		constraints = []Constraint{constraintStringArray("measurement", "one_of", []string{"temperature", "humidity", "pressure"})}
	}
	return ProviderRequest{Capability: capability, Ports: []RoleContract{port}, Constraints: constraints}
}

func providerRole(role, kind, direction string, minimum, maximum float64) RoleContract {
	return RoleContract{Role: role, Contract: PortContract{
		Kind: kind, Direction: direction, Domain: "synthetic_domain",
		Voltage:         NumericRange{Minimum: float64Pointer(minimum), Maximum: float64Pointer(maximum)},
		MinimumEvidence: EvidenceRuleInferred,
	}}
}

func constraintNumber(name, relation string, value float64, unit string, tolerance float64) Constraint {
	raw, _ := json.Marshal(value)
	constraint := Constraint{Name: name, Relation: relation, Value: raw, Unit: unit}
	if tolerance > 0 {
		constraint.TolerancePercent = float64Pointer(tolerance)
	}
	return constraint
}

func constraintString(name, relation, value string) Constraint {
	raw, _ := json.Marshal(value)
	return Constraint{Name: name, Relation: relation, Value: raw}
}

func constraintBool(name, relation string, value bool) Constraint {
	raw, _ := json.Marshal(value)
	return Constraint{Name: name, Relation: relation, Value: raw}
}

func constraintRange(name, relation string, minimum, maximum float64, unit string) Constraint {
	raw, _ := json.Marshal([]float64{minimum, maximum})
	return Constraint{Name: name, Relation: relation, Value: raw, Unit: unit}
}

func constraintStringArray(name, relation string, values []string) Constraint {
	raw, _ := json.Marshal(values)
	return Constraint{Name: name, Relation: relation, Value: raw}
}
