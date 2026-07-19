package architecturesearch

import (
	"context"
	"encoding/json"
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
	if selection.ProviderID != "catalog_function_fragments" || len(selection.Calculations) != 1 || len(selection.Components) != 5 {
		t.Fatalf("selection = %#v", selection)
	}
	realization, err := DecodeFragmentRealization(selection.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if realization.Capability != "threshold_detection" || len(realization.PortBindings) != 4 {
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
		{name: "adjustable_regulator_input_out_of_range", request: regulatorProviderRequest(15, 5, 0.25), wantError: true},
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

func TestCatalogProviderOffersAndRanksRealFilterAlternative(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, _ := NewCatalogProvider(catalog)
	expansions, err := provider.Expand(context.Background(), filterProviderRequest(5, 2000))
	if err != nil || len(expansions) != 2 {
		t.Fatalf("filter expansions = %#v, %v", expansions, err)
	}
	if expansions[0].ID == expansions[1].ID || len(expansions[0].Components) == len(expansions[1].Components) {
		t.Fatalf("filter alternatives are not distinct: %#v", expansions)
	}
}

func TestCatalogProviderOutputIgnoresCatalogOrdering(t *testing.T) {
	firstCatalog := loadArchitectureCatalog(t)
	secondCatalog := *firstCatalog
	secondCatalog.Records = append([]components.ComponentRecord(nil), firstCatalog.Records...)
	slices.Reverse(secondCatalog.Records)
	components.SortCatalog(&secondCatalog)
	first, _ := NewCatalogProvider(firstCatalog)
	second, _ := NewCatalogProvider(&secondCatalog)
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
