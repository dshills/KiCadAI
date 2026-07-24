package architecturesearch

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
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
	if selection.ProviderID != "catalog_function_fragments" || len(selection.Calculations) != 3 || len(selection.Components) != 8 {
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
	if realization.Capability != "threshold_detection" || len(realization.PortBindings) != 4 || len(realization.Connections) != 5 {
		t.Fatalf("realization = %#v", realization)
	}
	if len(realization.RepairVariables) != 1 {
		t.Fatalf("threshold repair variables = %#v", realization.RepairVariables)
	}
	feedbackRepair := realization.RepairVariables[0]
	if feedbackRepair.ID != "threshold_feedback_resistance" || feedbackRepair.Instance != "feedback_resistor" || len(feedbackRepair.AllowedValues) < 2 || !slices.Contains(feedbackRepair.AllowedValues, feedbackRepair.Value) {
		t.Fatalf("threshold feedback repair = %#v", feedbackRepair)
	}
	if len(feedbackRepair.Effects) != 1 || feedbackRepair.Effects[0] != (RealizationRepairEffect{Analysis: "dc_operating_point", Metric: "hysteresis_voltage", Direction: "metric_decreases"}) {
		t.Fatalf("threshold feedback repair effects = %#v", feedbackRepair.Effects)
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

func TestCatalogAlternativePreservesUniqueSimulationEvidence(t *testing.T) {
	inferred := components.ComponentRecord{Family: "resistor"}
	unique := components.ComponentRecord{Family: "resistor", SimulationModels: []simmodel.CatalogEvidence{{ModelID: simmodel.PrimitiveResistorV1}}}
	legacyAugmented := components.ComponentRecord{Family: "resistor", SimulationModels: []simmodel.CatalogEvidence{
		{ModelID: simmodel.PrimitiveResistorV1},
		{ModelID: simmodel.ModelResistorDividerDCV1},
	}}
	ambiguous := components.ComponentRecord{Family: "capacitor", SimulationModels: []simmodel.CatalogEvidence{
		{ModelID: simmodel.PrimitiveCapacitorV1},
		{ModelID: simmodel.PrimitiveCapacitorTransientV1},
	}}
	if !catalogAlternativePreservesSimulationEvidence(inferred, unique) {
		t.Fatal("single explicit primitive should preserve an inferred family primitive")
	}
	if !catalogAlternativePreservesSimulationEvidence(inferred, legacyAugmented) {
		t.Fatal("legacy workflow evidence must not be mistaken for an additional device primitive")
	}
	if catalogAlternativePreservesSimulationEvidence(components.ComponentRecord{Family: "capacitor"}, ambiguous) {
		t.Fatal("multiple explicit device primitives must not replace a uniquely inferred family primitive")
	}
	if !catalogAlternativePreservesSimulationEvidence(unique, unique) {
		t.Fatal("identical explicit primitive sets should be compatible")
	}
	if !catalogAlternativePreservesSimulationEvidence(unique, legacyAugmented) {
		t.Fatal("an alternative may add capabilities when it preserves every explicit original primitive")
	}
}

func TestCatalogProviderBindsPassiveSelectionToRequestedElectricalValue(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	parts, err := provider.appendPassiveParts(context.Background(), nil, []passivePart{
		{id: "exact_47r", family: "resistor", usage: "damping", value: "47"},
		{id: "exact_4k7", family: "resistor", usage: "pullup", value: "4.7k"},
		{id: "generic_22r", family: "resistor", usage: "series", value: "22"},
		{id: "exact_150u", family: "capacitor", usage: "bulk", value: "150u"},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]catalogPart{}
	for _, part := range parts {
		byID[part.selected.InstanceID] = part
	}
	if got := byID["exact_47r"].record.ID; got != "resistor.yageo.rc0805fr_0747rl.0805" {
		t.Fatalf("47 ohm selection = %q", got)
	}
	if got := byID["exact_4k7"].record.ID; got != "resistor.yageo.rc0805fr_074k7l.0805" {
		t.Fatalf("4.7 kohm selection = %q", got)
	}
	if part := byID["generic_22r"]; !part.record.Generic {
		t.Fatalf("22 ohm selection reused mismatched fixed-value part: %#v", part.record)
	}
	if part := byID["exact_150u"]; part.record.ID != "capacitor.panasonic.eeufr1a151.radial" ||
		!catalogRecordSupportsFunctions(part.record, []string{"a", "b"}) {
		t.Fatalf("150 uF polarized selection = %#v", part.record)
	}
}

func TestCatalogProviderRequiresExactVerifiedPrecisionValueDeterministically(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	part, err := provider.selectComponentWithTolerance(context.Background(), "resistor", "resistance", "11.8k", "resistance", .1, "%", 25)
	if err != nil {
		t.Fatal(err)
	}
	if got := part.record.ID; got != "resistor.vishay.tnpu1206.e192.0p02.2ppm" {
		t.Fatalf("exact series selection = %q", got)
	}
	if part.value != "11.8k" {
		t.Fatalf("selected catalog value = %q, want 11.8k", part.value)
	}
}

func TestCatalogProviderRejectsApproximateFixedPrecisionValue(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	catalog.Records = slices.DeleteFunc(catalog.Records, func(record components.ComponentRecord) bool {
		return record.ID == "resistor.vishay.tnpw1206.e192.0p1" ||
			record.ID == "resistor.vishay.mca1206at.e192.0p1" ||
			record.ID == "resistor.vishay.mca1206at.e192.0p1.10ppm" ||
			record.ID == "resistor.vishay.tnpu1206.e192.0p05.5ppm" ||
			record.ID == "resistor.vishay.tnpu1206.e192.0p02.2ppm"
	})
	components.RebuildCatalogIndexes(catalog)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.selectComponentWithTolerance(context.Background(), "resistor", "resistance", "11.8k", "resistance", .1, "%", 25); err == nil {
		t.Fatal("approximate 11.7k fixed resistor silently substituted for requested 11.8k")
	}
}

func TestPreferredRepairValuesIncludeFineAndCoarseE96Neighbors(t *testing.T) {
	values := preferredRepairValues(15.8)
	for _, expected := range []float64{14.3, 15.4, 16.2, 17.4, 19.1} {
		if !slices.Contains(values, expected) {
			t.Fatalf("preferred repair values %v lack E96 neighbor %.3g", values, expected)
		}
	}
}

func TestLowerEdgeAmplifierGainTargetCoversOpposingFeedbackTolerance(t *testing.T) {
	target := lowerEdgeAmplifierGainTarget(20, 5, .1)
	feedbackTolerance := .001
	worstCaseGain := 1 + (target-1)*(1-feedbackTolerance)/(1+feedbackTolerance)
	if worstCaseGain < 19 {
		t.Fatalf("worst-case gain = %.12g, want at least lower requirement edge 19", worstCaseGain)
	}
	if target >= 20 {
		t.Fatalf("target gain = %.12g, want lower-edge-centered target below band midpoint", target)
	}
}

func TestFailSafeMutePulldownIsBoundedByResidualOutputRatio(t *testing.T) {
	resistance, ok := failSafeMutePulldownResistance(.05, math.Sqrt(2*10*8), 60_000_000)
	if !ok {
		t.Fatal("failSafeMutePulldownResistance() did not solve bounded request")
	}
	attenuation := resistance / (60_000_000 + resistance)
	residual := math.Sqrt(2*10*8) * attenuation
	if residual > .025 {
		t.Fatalf("residual output = %.12g V with %.12g ohm pull-down, want at most half of 50 mV requirement", residual, resistance)
	}
}

func TestCatalogProviderDispatchesBehaviorDerivedThresholdToGenericAdapter(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := thresholdProviderRequest(5, 1.65, 0.2)
	request.Constraints = request.Constraints[:2]
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("behavior-derived threshold dispatch expansions=%d err=%v", len(expansions), err)
	}
}

func TestCatalogProviderSelectsSplitSupplyConverterForWorstCaseRegulatorHeadroom(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		output    float64
		catalogID string
	}{
		{name: "lower rails use minimum qualifying converter", output: 5, catalogID: "isolated_converter.traco.tel12_1222.dip16"},
		{name: "higher rails preserve tolerance and dropout headroom", output: 9, catalogID: "isolated_converter.traco.tel12_1223.dip16"},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputCurrent, outputCurrent := .5, .12
			input := providerRole("input", "power", "sink", 13.5, 16.5)
			input.Contract.MaximumCurrentDemandA = &inputCurrent
			positive := providerRole("positive_output", "power", "source", .98*test.output, 1.02*test.output)
			positive.Contract.RequiredCurrentCapacityA = &outputCurrent
			negative := providerRole("negative_output", "power", "source", -1.02*test.output, -.98*test.output)
			negative.Contract.RequiredCurrentCapacityA = &outputCurrent
			request := ProviderRequest{Capability: "split_supply_generation", Ports: []RoleContract{
				input, positive, negative, providerRole("reference", "reference", "bidirectional", 0, 0),
			}, Constraints: []Constraint{
				constraintNumber("positive_voltage", "target", test.output, "V", 2),
				constraintNumber("negative_voltage", "target", -test.output, "V", 2),
			}}
			expansions, err := provider.Expand(context.Background(), request)
			if err != nil || len(expansions) == 0 {
				t.Fatalf("split-supply expansions=%d err=%v", len(expansions), err)
			}
			if !slices.ContainsFunc(expansions[0].Components, func(component SelectedComponent) bool {
				return component.CatalogID == test.catalogID
			}) {
				t.Fatalf("split-supply components = %#v, want %q", expansions[0].Components, test.catalogID)
			}
			if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
				return calculation.ID == "split_supply_margins" && calculation.Pass
			}) {
				t.Fatalf("split-supply calculations = %#v", expansions[0].Calculations)
			}
		})
	}
}

func TestCatalogProviderKeepsCurrentMeasurementSeparateFromFailSafeFaultInput(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("control", "digital_logic", "sink", 0, 3.3),
		providerRole("fault", "digital_logic", "sink", 0, 3.3),
		providerRole("measurement", "analog_voltage", "source", 0, 2.5),
		providerRole("permit", "digital_logic", "source", 0, 3.3),
		providerRole("power", "power", "sink", 21.6, 26.4),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 2),
		constraintBool("fail_safe_interlock", "required", true),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	var measurement, fault RealizationPortBinding
	for _, binding := range realization.PortBindings {
		switch binding.Role {
		case "measurement":
			measurement = binding
		case "fault":
			fault = binding
		}
	}
	if measurement.Instance != "current_monitor" || measurement.Function != "OUT" {
		t.Fatalf("measurement binding = %#v", measurement)
	}
	if fault.Instance != "fault_base_resistor" || fault.Function != "A" ||
		(fault.Instance == measurement.Instance && fault.Function == measurement.Function) {
		t.Fatalf("fault binding = %#v, measurement binding = %#v", fault, measurement)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "fault_inverter" && instance.Usage == "fail_safe_enable"
	}) {
		t.Fatalf("current-sensing instances lack a fail-safe fault pull-down: %#v", realization.Instances)
	}
}

func TestCatalogProviderOmitsUnexposedCurrentSenseInterlock(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("output", "analog_voltage", "source", 0, 2.5),
		providerRole("power", "power", "sink", 10.8, 13.2),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 2),
		constraintBool("fail_safe_interlock", "required", true),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	if expansions[0].ID != "precision_high_side_current_measurement" {
		t.Fatalf("expansion id = %q", expansions[0].ID)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range realization.Instances {
		switch instance.ID {
		case "fault_inverter", "control_series", "fault_base_resistor":
			t.Fatalf("measurement-only realization contains interlock instance %#v", instance)
		}
	}
}

func TestCatalogProviderRejectsPartialCurrentSenseInterlock(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("control", "digital_logic", "sink", 0, 3.3),
		providerRole("output", "analog_voltage", "source", 0, 2.5),
		providerRole("power", "power", "sink", 10.8, 13.2),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 2),
		constraintBool("fail_safe_interlock", "required", true),
	}}
	_, err = provider.Expand(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "requires control, fault, and permit") {
		t.Fatalf("partial interlock error = %v", err)
	}
}

func TestCatalogProviderUsesHighDisconnectControlForFailSafeLoad(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(24, 2)
	request.Ports = slices.DeleteFunc(request.Ports, func(port RoleContract) bool { return port.Role == "logic_power" })
	request.Constraints = []Constraint{
		constraintNumber("load_current", "minimum", 2, "A", 10),
		constraintBool("fail_safe_interlock", "required", true),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("fail-safe load expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	components := map[string]bool{}
	for _, component := range realization.Instances {
		components[component.ID] = true
	}
	if !components["gate_inverter"] || components["control_inverter"] {
		t.Fatalf("fail-safe load components = %#v", components)
	}
}

func TestCatalogProviderUsesDefaultOffHighSideSwitchForLowStartupOutput(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(28, 2)
	request.Ports = slices.DeleteFunc(request.Ports, func(port RoleContract) bool { return port.Role == "logic_power" })
	request.Constraints = []Constraint{
		constraintNumber("load_current", "minimum", 2, "A", 10),
		constraintNumber("startup_output_voltage", "maximum", .5, "V", 0),
		constraintBool("fail_safe_interlock", "required", true),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("startup-safe load expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool { return instance.CatalogID == "mosfet.onsemi.fqp47p06.to220" }) {
		t.Fatalf("startup-safe load did not select a trusted high-side PMOS: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.Usage == "series_gate_overvoltage_clamp"
	}) {
		t.Fatalf("full gate swing exceeds the selected PMOS rating without a synthesized series clamp: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "high_side_switch_derating" && calculation.Pass
	}) {
		t.Fatalf("startup-safe load lacks passing gate-drive derating evidence: %#v", expansions[0].Calculations)
	}
	seenOutput := false
	for _, binding := range realization.PortBindings {
		if (binding.Role == "power" || binding.Role == "load_power") && (binding.Instance != "high_side_switch" || binding.Function != "SOURCE") {
			t.Fatalf("high-side power binding = %#v", binding)
		}
		if (binding.Role == "output" || binding.Role == "load") && binding.Instance == "high_side_switch" && binding.Function == "DRAIN" {
			seenOutput = true
		}
	}
	if !seenOutput {
		t.Fatalf("high-side output binding = %#v", realization.PortBindings)
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

func TestCatalogProviderPreservesEnvironmentalConstraintsAcrossGenericLoadSwitchAdapter(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(28, 2)
	request.Constraints = []Constraint{
		constraintNumber("load_current", "minimum", 2, "A", 0),
		constraintNumber("ambient_temperature_minimum", "minimum", -20, "degC", 0),
		constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
		constraintNumber("junction_temperature", "maximum", 125, "degC", 0),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("thermal load-switch expansion = %#v, %v", expansions, err)
	}
	flybackIndex := slices.IndexFunc(expansions[0].Components, func(component SelectedComponent) bool {
		return component.InstanceID == "flyback_clamp"
	})
	if flybackIndex < 0 {
		t.Fatalf("thermal load-switch expansion did not select the ambient-qualified flyback: %#v", expansions[0].Components)
	}
	flyback := expansions[0].Components[flybackIndex]
	resolved, resolvedResult := components.ResolveBinding(context.Background(), catalog, flyback.CatalogID, flyback.VariantID)
	if !resolvedResult.OK || resolved.Component.Thermal == nil || resolved.Component.Thermal.MaxJunctionTemperatureC == nil || resolved.Component.Thermal.JunctionToAmbientCPerW == nil {
		t.Fatalf("selected flyback lacks ambient thermal evidence: selected=%#v resolved=%#v issues=%#v", flyback, resolved, resolvedResult.Issues)
	}
	if !slices.ContainsFunc(expansions[0].Components, func(component SelectedComponent) bool {
		return component.CatalogID == "mosfet.onsemi.rfd16n05lsm.to252"
	}) {
		t.Fatalf("low-voltage control expansion did not select a MOSFET with guaranteed gate-drive margin: %#v", expansions[0].Components)
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

func TestCatalogProviderMaterializesFilterWithSolverBackedPassiveTolerances(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	request := filterProviderRequest(5, 2000)
	request.Constraints = []Constraint{
		constraintString("response", "equal", "low_pass"),
		constraintNumber("order", "equal", 2, "", 0),
		constraintNumber("cutoff_frequency", "target", 2000, "Hz", 10),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "catalog_sallen_key_low_pass"
	})
	if index < 0 {
		t.Fatalf("generic filter expansion missing: %#v", expansions)
	}
	realization, err := DecodeFragmentRealization(expansions[index].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"filter_r1", "filter_r2"} {
		instanceIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id
		})
		if instanceIndex < 0 {
			t.Fatalf("%s missing: %#v", id, realization.Instances)
		}
		recordIndex := slices.IndexFunc(catalog.Records, func(record components.ComponentRecord) bool {
			return record.ID == realization.Instances[instanceIndex].CatalogID
		})
		if recordIndex < 0 {
			t.Fatalf("%s catalog record missing: %#v", id, realization.Instances[instanceIndex])
		}
		tolerance, ok := catalogToleranceMaximum(catalog.Records[recordIndex], "resistance", "%")
		if !ok || tolerance > 0.1 {
			t.Fatalf("%s lacks catalog-backed 0.1%% tolerance: %#v", id, realization.Instances)
		}
	}
	for _, id := range []string{"filter_c1", "filter_c2"} {
		instanceIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id
		})
		if instanceIndex < 0 || realization.Instances[instanceIndex].CatalogID != "capacitor.kemet.mil-prf-32535.c0g.1210.e12.1p0" {
			t.Fatalf("%s lacks catalog-backed 1%% C0G tolerance: %#v", id, realization.Instances)
		}
	}
}

func TestCatalogProviderOffersFixedAndAdjustableRegulatorTopologies(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := regulatorProviderRequest(5.5, 3.3, 0.15)
	request.Ports[0].Contract.Voltage.Minimum = float64Pointer(4.5)
	request.Constraints = request.Constraints[:2]
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixed, adjustable := false, false
	for _, expansion := range expansions {
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		for _, instance := range realization.Instances {
			if instance.Usage != "regulator" {
				continue
			}
			if slices.Contains(instance.RequiredFunctions, "ADJ") {
				adjustable = true
			} else if slices.Contains(instance.RequiredFunctions, "VIN") && slices.Contains(instance.RequiredFunctions, "VOUT") && slices.Contains(instance.RequiredFunctions, "GND") {
				fixed = true
			}
		}
	}
	if !fixed || !adjustable {
		t.Fatalf("regulator topology coverage fixed=%t adjustable=%t expansions=%d", fixed, adjustable, len(expansions))
	}
}

func TestCatalogProviderOrientsFloatingRegulatorFeedbackForItsReferenceEquation(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := regulatorProviderRequest(19.8, 15, 0.25)
	request.Ports[0].Contract.Voltage.Minimum = float64Pointer(18)
	request.Constraints[2] = constraintRange("input_voltage", "range", 18, 19.8, "V")
	expansions, err := provider.expandRegulator(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("floating regulator expansion = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	floating := false
	for _, instance := range realization.Instances {
		if instance.Usage == "regulator" {
			floating = slices.Contains(instance.RequiredFunctions, "ADJ") && !slices.Contains(instance.RequiredFunctions, "GND")
		}
		if instance.ID == "feedback_lower" || instance.ID == "feedback_upper" {
			value, ok := components.ParseEngineeringValue(instance.Value)
			if !ok {
				t.Fatalf("%s value %q is not an engineering value", instance.ID, instance.Value)
			}
			values[instance.ID] = value
		}
	}
	if !floating {
		t.Fatalf("expected a floating adjustable regulator realization: %#v", realization.Instances)
	}
	if values["feedback_upper"] > 125 || values["feedback_upper"] < 60 || values["feedback_lower"] <= values["feedback_upper"] {
		t.Fatalf("floating feedback values = %#v; VOUT-ADJ must use the fixed reference resistor and ADJ-reference the larger programming resistor", values)
	}
}

func TestCatalogUnitConversionIsSymmetricForSupportedScaledUnits(t *testing.T) {
	tests := []struct {
		value    float64
		from, to string
		want     float64
	}{{0.15, "A", "mA", 150}, {150, "mA", "A", 0.15}, {3.3, "V", "mV", 3300}, {3300, "mV", "V", 3.3}, {2e-9, "C", "nC", 2}, {2, "nC", "C", 2e-9}}
	for _, test := range tests {
		got, ok := convertCatalogUnit(test.value, test.from, test.to)
		if !ok || math.Abs(got-test.want) > math.Max(1e-15, math.Abs(test.want)*1e-12) {
			t.Fatalf("convertCatalogUnit(%g, %q, %q) = %g, %t; want %g", test.value, test.from, test.to, got, ok, test.want)
		}
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

func TestCatalogProviderTiesFixedRegulatorEnableOnlyWithinCatalogRatings(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "voltage_regulation", Ports: []RoleContract{
		providerRole("input", "power", "sink", 4.75, 5.25),
		providerRole("output", "power", "source", 3.2, 3.4),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}}
	expansions, err := provider.expandFixedRegulators(context.Background(), request, 3.3, 3, 4.75, 5.25, 0.1)
	if err != nil {
		t.Fatal(err)
	}
	for _, expansion := range expansions {
		if len(expansion.Components) == 0 || expansion.Components[0].CatalogID != "regulator.linear.ap2112k_3v3.sot23_5" {
			continue
		}
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		for _, connection := range realization.Connections {
			if connection.ID != "regulator_input" {
				continue
			}
			if slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool { return endpoint.Function == "EN" }) {
				return
			}
		}
		t.Fatalf("AP2112 EN is not tied to its validated input rail: %#v", realization.Connections)
	}
	t.Fatal("AP2112 fixed-regulator expansion is missing")
}

func TestCatalogProviderSelectsESP32FromRequiredWirelessCapability(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	request.Constraints = append(request.Constraints, constraintStringArray("required_capabilities", "all_of", []string{"wifi", "bluetooth"}))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 1 {
		t.Fatalf("wireless controller expansion = %#v, %v", expansions, err)
	}
	if len(expansions[0].Components) == 0 || expansions[0].Components[0].CatalogID != "mcu.espressif.esp32_wroom_32e" {
		t.Fatalf("wireless capability did not select ESP32: %#v", expansions[0].Components)
	}
}

func TestCatalogProviderPrioritizesExplicitMCUComponentSearch(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	request.Constraints = append(request.Constraints, constraintString("component_search", "equal", "ESP32-WROOM-32E"))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 1 {
		t.Fatalf("explicit MCU expansion = %#v, %v", expansions, err)
	}
	if len(expansions[0].Components) == 0 || expansions[0].Components[0].CatalogID != "mcu.espressif.esp32_wroom_32e" {
		t.Fatalf("explicit component search did not prioritize ESP32: %#v", expansions[0].Components)
	}
}

func TestCatalogProviderSelectsSTM32FromProgrammingKind(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	request.Constraints = append(request.Constraints, constraintString("programming_kind", "equal", "swd"))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 1 {
		t.Fatalf("SWD controller expansion = %#v, %v", expansions, err)
	}
	if len(expansions[0].Components) == 0 || expansions[0].Components[0].CatalogID != "mcu.st.stm32g031k8t6.lqfp32" {
		t.Fatalf("SWD capability did not select STM32: %#v", expansions[0].Components)
	}
}

func TestCatalogProviderBindsCompleteMixedMCUPeripheralBundles(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	request.Constraints = append(request.Constraints, constraintString("programming_kind", "equal", "swd"))
	uart := providerRole("console", "digital_bus", "bidirectional", 3.1, 3.5)
	uart.Contract.Protocol = &Protocol{Name: "uart", Mode: "push_pull", MaxFrequencyHz: 1_000_000}
	spi := providerRole("storage", "digital_bus", "bidirectional", 3.1, 3.5)
	spi.Contract.Protocol = &Protocol{Name: "spi", Mode: "push_pull", MaxFrequencyHz: 8_000_000}
	adc := providerRole("measurement", "analog_voltage", "sink", 0, 3.3)
	pwm := providerRole("drive", "analog_control", "source", 0, 3.3)
	interrupt := providerRole("alarm_irq", "digital_logic", "sink", 0, 3.3)
	interrupt.Contract.RequiredTraits = []string{"interrupt"}
	request.Ports = append(request.Ports, uart, spi, adc, pwm, interrupt)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 1 {
		t.Fatalf("mixed controller expansion = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	wantLanes := map[string]int{"sensor_bus": 2, "console": 2, "storage": 4, "measurement": 1, "drive": 1, "alarm_irq": 1}
	gotLanes := map[string]int{}
	for _, binding := range realization.PortBindings {
		gotLanes[binding.Role]++
	}
	for role, want := range wantLanes {
		if gotLanes[role] != want {
			t.Fatalf("role %s has %d bindings, want %d: %#v", role, gotLanes[role], want, realization.PortBindings)
		}
	}
}

func TestCatalogProviderDerivesConditionalMCUSupportNetworks(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	request.Constraints = append(request.Constraints, constraintString("programming_kind", "equal", "swd"))
	internal, err := provider.Expand(context.Background(), request)
	if err != nil || len(internal) < 1 {
		t.Fatalf("internal-clock expansion = %#v, %v", internal, err)
	}
	internalRealization, err := DecodeFragmentRealization(internal[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range internalRealization.Instances {
		if strings.Contains(instance.ID, "external_hse") {
			t.Fatalf("internal clock unexpectedly populated external oscillator: %#v", internalRealization.Instances)
		}
	}
	assertMCUSupportUsages(t, internalRealization, map[string]bool{"i2c_pullup": false})
	controllerID := ""
	for _, instance := range internalRealization.Instances {
		if instance.Usage == "programmable_controller" {
			controllerID = instance.ID
		}
	}
	if controllerID == "" {
		t.Fatalf("MCU realization lacks controller instance: %#v", internalRealization.Instances)
	}
	pullupConnections := 0
	for _, connection := range internalRealization.Connections {
		hasController, hasPullup := false, false
		for _, endpoint := range connection.Endpoints {
			hasController = hasController || endpoint.Instance == controllerID
			hasPullup = hasPullup || strings.Contains(endpoint.Instance, "i2c_pullups")
			if strings.HasPrefix(strings.ToLower(endpoint.Function), "peripheral:") {
				t.Fatalf("unresolved MCU peripheral role in support connection: %#v", connection)
			}
		}
		if hasController && hasPullup {
			pullupConnections++
		}
	}
	if pullupConnections != 3 {
		t.Fatalf("I2C pull-up nets connected to controller = %d, want 3: %#v", pullupConnections, internalRealization.Connections)
	}
	request.Constraints = append(request.Constraints, constraintString("clock_source", "equal", "external_hse"))
	external, err := provider.Expand(context.Background(), request)
	if err != nil || len(external) < 1 {
		t.Fatalf("external-clock expansion = %#v, %v", external, err)
	}
	externalRealization, err := DecodeFragmentRealization(external[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertMCUSupportUsages(t, externalRealization, map[string]bool{"clock": false, "load_capacitor": false})
}

func assertMCUSupportUsages(t *testing.T, realization FragmentRealization, want map[string]bool) {
	t.Helper()
	for _, instance := range realization.Instances {
		if _, exists := want[instance.Usage]; exists {
			want[instance.Usage] = true
		}
	}
	for usage, found := range want {
		if !found {
			t.Fatalf("MCU realization lacks %s support: %#v", usage, realization.Instances)
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

func TestTranslatorEvidenceModeAndDirectionMatchingIsCaseInsensitive(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	for _, record := range catalog.Records {
		if record.Translator == nil {
			continue
		}
		record.Translator.SignalingModes = []string{"OPEN_DRAIN"}
		record.Translator.Directions = []string{"BIDIRECTIONAL"}
		if !translatorEvidenceSupports(record, 1.8, 3.3, "open_drain", "bidirectional", 400000, 2, true) {
			t.Fatalf("case-normalized translator evidence was rejected: %#v", record.Translator)
		}
		return
	}
	t.Fatal("checked-in catalog has no translator evidence")
}

func TestCatalogProviderSizesOpenDrainPullupsFromRiseTimeAndCapacitance(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := translatorProviderRequest(3.3, 5)
	request.Constraints = append(request.Constraints,
		constraintNumber("rise_time", "maximum", 1e-6, "s", 0),
		constraintNumber("load_capacitance", "maximum", 4e-10, "F", 0),
	)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"side_a_sda_pullup", "side_a_scl_pullup", "side_b_sda_pullup", "side_b_scl_pullup"} {
		if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id && instance.Value == "910"
		}) {
			t.Fatalf("%s is not rise-time sized: %#v", id, realization.Instances)
		}
		if !slices.ContainsFunc(realization.RepairVariables, func(variable RealizationRepairVariable) bool {
			return variable.Instance == id && variable.Value == 910 && len(variable.AllowedValues) != 0
		}) {
			t.Fatalf("%s lacks bounded timing repair values: %#v", id, realization.RepairVariables)
		}
	}

	impossible := translatorProviderRequest(3.3, 5)
	impossible.Constraints = append(impossible.Constraints,
		constraintNumber("rise_time", "maximum", 1e-12, "s", 0),
		constraintNumber("load_capacitance", "maximum", 1e-6, "F", 0),
	)
	_, err = provider.Expand(context.Background(), impossible)
	var typed *interfaceSynthesisError
	if !errors.As(err, &typed) || typed.code != CodeInterfacePullupWindowEmpty {
		t.Fatalf("impossible pull-up window error = %#v", err)
	}
}

func TestCatalogProviderTranslationFailuresHaveStableCodes(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		request ProviderRequest
		code    reports.Code
	}{
		{name: "missing_domain", request: translatorProviderRequest(3.3, 0), code: CodeInterfaceVoltageDomainMismatch},
		{name: "unsupported_domain", request: translatorProviderRequest(3.3, 1.2), code: CodeInterfaceTranslationUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := provider.Expand(context.Background(), test.request)
			var typed *interfaceSynthesisError
			if !errors.As(err, &typed) || typed.code != test.code {
				t.Fatalf("Expand() error = %#v; want %s", err, test.code)
			}
			if typed.ArchitectureRejectionCode() != test.code {
				t.Fatalf("ArchitectureRejectionCode() = %s; want %s", typed.ArchitectureRejectionCode(), test.code)
			}
		})
	}
}

func TestCatalogProviderKeepsClassABNegativeRailDistinctFromReference(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -2, 2),
		providerRole("output", "analog_voltage", "source", -13, 13),
		providerRole("positive_power", "power", "sink", 15, 15),
		providerRole("negative_power", "power", "sink", -15, -15),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "target", 8, "ohm", 0),
		constraintNumber("continuous_output_power", "minimum", 10, "W", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	negativeRail := false
	referenceBinding := false
	driverEmitterReturns := map[string]bool{}
	biasTrackerJunctions := map[string]map[string]bool{}
	selectedParts := map[string]string{}
	selectedValues := map[string]string{}
	for _, instance := range realization.Instances {
		selectedParts[instance.ID] = instance.CatalogID
		selectedValues[instance.ID] = instance.Value
	}
	if selectedParts["output_npn_1"] != "bjt.onsemi.d44h11g.to220" || selectedParts["output_pnp_1"] != "bjt.onsemi.d45h11g.to220" {
		t.Fatalf("Class AB output pair = %q/%q, want thermally qualified D44H11G/D45H11G", selectedParts["output_npn_1"], selectedParts["output_pnp_1"])
	}
	if selectedParts["driver_npn"] != "bjt.onsemi.pzt3904t1g.sot223" || selectedParts["driver_pnp"] != "bjt.onsemi.pzt3906t1g.sot223" {
		t.Fatalf("Class AB predriver pair = %q/%q, want thermally qualified PZT3904T1G/PZT3906T1G", selectedParts["driver_npn"], selectedParts["driver_pnp"])
	}
	if selectedValues["npn_base_stop"] != "68" || selectedValues["pnp_base_stop"] != "68" {
		t.Fatalf("Class AB driver input stoppers = %q/%q, want bounded 68-ohm pair", selectedValues["npn_base_stop"], selectedValues["pnp_base_stop"])
	}
	if selectedValues["npn_driver_base_emitter"] != "10k" || selectedValues["pnp_driver_base_emitter"] != "10k" {
		t.Fatalf("Class AB driver turn-off shunts = %q/%q, want ratio-derived 10-kilohm pair", selectedValues["npn_driver_base_emitter"], selectedValues["pnp_driver_base_emitter"])
	}
	for _, binding := range realization.PortBindings {
		if binding.Role == "reference" && binding.Instance == "input_bias" && binding.Function == "B" {
			referenceBinding = true
		}
	}
	for _, connection := range realization.Connections {
		if connection.Role == "reference" {
			t.Fatalf("dual-rail Class AB realization invented a reference net: %#v", connection)
		}
		if connection.ID == "class_ab_negative_power" && connection.Role == "power" {
			negativeRail = true
		}
		if connection.ID == "class_ab_output" {
			for _, endpoint := range connection.Endpoints {
				if endpoint.Function == "B" && (endpoint.Instance == "npn_driver_emitter" || endpoint.Instance == "pnp_driver_emitter") {
					driverEmitterReturns[endpoint.Instance] = true
				}
			}
		}
		for _, endpoint := range connection.Endpoints {
			if endpoint.Instance != "upper_bias_tracker" && endpoint.Instance != "lower_bias_tracker" {
				continue
			}
			if biasTrackerJunctions[endpoint.Instance] == nil {
				biasTrackerJunctions[endpoint.Instance] = map[string]bool{}
			}
			biasTrackerJunctions[endpoint.Instance][connection.ID+":"+endpoint.Function] = true
		}
	}
	if !negativeRail {
		t.Fatalf("dual-rail Class AB realization lacks a power-role negative rail: %#v", realization.Connections)
	}
	if !referenceBinding {
		t.Fatalf("dual-rail Class AB realization does not bind signal reference separately: %#v", realization.PortBindings)
	}
	if !driverEmitterReturns["npn_driver_emitter"] || !driverEmitterReturns["pnp_driver_emitter"] {
		t.Fatalf("Class AB complementary-feedback driver emitters do not return to the output: %#v", realization.Connections)
	}
	if !biasTrackerJunctions["upper_bias_tracker"]["npn_driver_base:BASE"] || !biasTrackerJunctions["upper_bias_tracker"]["npn_driver_base:COLLECTOR"] || !biasTrackerJunctions["upper_bias_tracker"]["base_drive:EMITTER"] {
		t.Fatalf("upper Class AB bias tracker is not diode-connected across the NPN driver junction: %#v", realization.Connections)
	}
	if !biasTrackerJunctions["lower_bias_tracker"]["pnp_driver_base:BASE"] || !biasTrackerJunctions["lower_bias_tracker"]["pnp_driver_base:COLLECTOR"] || !biasTrackerJunctions["lower_bias_tracker"]["base_drive:EMITTER"] {
		t.Fatalf("lower Class AB bias tracker is not diode-connected across the PNP driver junction: %#v", realization.Connections)
	}

	biasRequest := ProviderRequest{Capability: "class_ab_bias_control", Ports: []RoleContract{
		providerRole("output", "analog_voltage", "source", -1, 1),
		providerRole("positive_power", "power", "sink", 15, 15),
		providerRole("negative_power", "power", "sink", -15, -15),
	}, Constraints: []Constraint{constraintBool("thermal_tracking", "required", true)}}
	biasExpansions, err := provider.Expand(context.Background(), biasRequest)
	if err != nil || len(biasExpansions) == 0 {
		t.Fatalf("bias Expand() = %#v, %v", biasExpansions, err)
	}
	biasRealization, err := DecodeFragmentRealization(biasExpansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	biasNegativeRail := false
	for _, binding := range biasRealization.PortBindings {
		if binding.Role == "negative_power" && binding.Instance == "tracking_diode_2" && binding.Function == "K" {
			biasNegativeRail = true
		}
	}
	for _, connection := range biasRealization.Connections {
		if connection.Role == "reference" {
			t.Fatalf("dual-rail Class AB bias realization invented a reference net: %#v", connection)
		}
	}
	if !biasNegativeRail {
		t.Fatalf("dual-rail Class AB bias realization lacks a negative-rail binding: %#v", biasRealization.PortBindings)
	}
	for _, instance := range biasRealization.Instances {
		if instance.ID == "bias_enable_inverter" || instance.ID == "bias_clamp" || instance.ID == "enable_resistor" {
			t.Fatalf("always-on Class AB bias contains a dead enable/clamp device: %#v", biasRealization.Instances)
		}
	}
}

func TestCatalogProviderClassABBiasFeedUsesThermallySuitableParts(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -2, 2),
		providerRole("output", "analog_voltage", "source", -13, 13),
		providerRole("positive_power", "power", "sink", 13.5, 16.5),
		providerRole("negative_power", "power", "sink", -16.5, -13.5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "target", 8, "ohm", 0),
		constraintNumber("continuous_output_power", "minimum", 10, "W", 0),
		constraintNumber("quiescent_current", "target", .07, "A", 42.8571428571),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	feeds := map[string]RealizationInstance{}
	for _, instance := range realization.Instances {
		if instance.ID == "upper_bias_feed" || instance.ID == "lower_bias_feed" {
			feeds[instance.ID] = instance
		}
	}
	for _, id := range []string{"upper_bias_feed", "lower_bias_feed"} {
		if feeds[id].CatalogID != "resistor.vishay.pr02.1k00.2w" || feeds[id].Value != "1k" {
			t.Fatalf("%s = %#v, want catalog-backed 1 kOhm 2 W part", id, feeds[id])
		}
	}
}

func TestCatalogProviderBindsSingleSupplyClassABReferenceToNegativeRail(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -1, 1),
		providerRole("output", "analog_voltage", "source", -10, 10),
		providerRole("power", "power", "sink", 21.6, 26.4),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "minimum", 8, "ohm", 0),
		constraintNumber("continuous_output_power", "minimum", 8, "W", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "reference" && binding.Instance == "voltage_driver" && binding.Function == "V_MINUS"
	}) {
		t.Fatalf("single-supply reference is not bound to the negative rail: %#v", realization.PortBindings)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "class_ab_reference" && slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool {
			return endpoint.Instance == "voltage_driver" && endpoint.Function == "V_MINUS"
		})
	}) {
		t.Fatalf("single-supply negative rail lacks the external reference endpoint: %#v", realization.Connections)
	}
}

func TestCatalogProviderConnectsSingleSupplyClassABOpAmpNegativeSupplyToReference(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 0, 2),
		providerRole("output", "analog_voltage", "source", 0, 13),
		providerRole("positive_power", "power", "sink", 15, 15),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "target", 8, "ohm", 0),
		constraintNumber("continuous_output_power", "minimum", 1, "W", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range realization.Connections {
		if connection.ID != "class_ab_reference" {
			continue
		}
		if slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool {
			return endpoint.Instance == "voltage_driver" && endpoint.Function == "V_MINUS"
		}) {
			return
		}
	}
	t.Fatalf("single-supply Class AB reference does not connect the voltage-driver negative supply: %#v", realization.Connections)
}

func TestCatalogProviderSizesOutputFuseFromBehavioralPowerAndLoad(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -13, 13),
		providerRole("output", "protected_output", "source", -13, 13),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("continuous_output_power", "minimum", 10, "W", 0),
		constraintNumber("load_impedance", "target", 8, "ohm", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_fuse" && instance.CatalogID == "fuse.littelfuse.0483002dr.1206" && instance.Value == "2"
	}) {
		t.Fatalf("10 W / 8 ohm protection did not select the catalog 2 A fuse: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_clamp" && instance.CatalogID == "protection.littelfuse.smbj18ca.smb"
	}) {
		t.Fatalf("audio output protection did not select a voltage-qualified TVS: %#v", realization.Instances)
	}
}

func TestCatalogProviderRaisesOutputTVSWorkingVoltageFromPortContract(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -19.8, 19.8),
		providerRole("output", "protected_output", "source", -19.8, 19.8),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_clamp" && instance.CatalogID == "protection.littelfuse.smbj20ca.smb"
	}) {
		t.Fatalf("19.8 V output did not select the 20 V working TVS: %#v", realization.Instances)
	}
}

func TestCatalogProviderBroadensRelayTechnologyWhenPreferredRelayIsUnderrated(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -13, 13),
		providerRole("output", "protected_output", "source", -13, 13),
		providerRole("reference", "reference", "bidirectional", 0, 0),
		providerRole("power", "power", "sink", 13.5, 16.5),
	}, Constraints: []Constraint{
		constraintNumber("continuous_output_power", "minimum", 10, "W", 0),
		constraintNumber("load_impedance", "target", 8, "ohm", 0),
		constraintBool("startup_isolation", "required", true),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_relay" && instance.CatalogID == "relay.omron.g5q_1a.dc12"
	}) {
		t.Fatalf("high-current startup isolation did not select a suitably rated relay: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "relay_coil_series" && instance.Value == "30"
	}) {
		t.Fatalf("startup-isolation relay lacks minimum-rail operate-current margin: %#v", realization.Instances)
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

func reversedArchitectureCatalog(catalog *components.Catalog) *components.Catalog {
	reversed := &components.Catalog{
		Version:     catalog.Version,
		GeneratedAt: catalog.GeneratedAt,
		Records:     append([]components.ComponentRecord(nil), catalog.Records...),
		Families:    append([]components.FamilyDefinition(nil), catalog.Families...),
		Diagnostics: append([]reports.Issue(nil), catalog.Diagnostics...),
	}
	slices.Reverse(reversed.Records)
	components.RebuildCatalogIndexes(reversed)
	return reversed
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
	inputMinimum := output + .5
	return ProviderRequest{Capability: "voltage_regulation", Ports: []RoleContract{
		providerRole("input", "power", "sink", inputMinimum, inputMaximum),
		providerRole("output", "power", "source", output*0.98, output*1.02),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("output_voltage", "target", output, "V", 2),
		constraintNumber("continuous_output_current", "minimum", current, "A", 0),
		constraintRange("input_voltage", "range", inputMinimum, inputMaximum, "V"),
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
