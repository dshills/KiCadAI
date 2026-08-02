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
		providerRole("input", "analog_voltage", "sink", -1, 1),
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
	supplyComponentIndex := slices.IndexFunc(expansions[0].Components, func(component SelectedComponent) bool {
		return component.InstanceID == "sense_supply"
	})
	if supplyComponentIndex < 0 {
		t.Fatalf("selected current-sensing regulator is missing: %#v", expansions[0].Components)
	}
	supplyCatalogID := expansions[0].Components[supplyComponentIndex].CatalogID
	supplyRecordIndex := slices.IndexFunc(provider.catalog.Records, func(record components.ComponentRecord) bool {
		return record.ID == supplyCatalogID
	})
	if supplyRecordIndex < 0 {
		t.Fatalf("selected current-sensing regulator %q is absent from the catalog", supplyCatalogID)
	}
	supplyRecord := provider.catalog.Records[supplyRecordIndex]
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_reference" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_feedback_lower", Function: "B"})
	}) {
		t.Fatalf("current-sensing feedback reference is not attached to the shared reference: %#v", realization.Connections)
	}
	hasInvalidGroundEndpoint := slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_supply", Function: "GND"})
	})
	if hasInvalidGroundEndpoint != recordHasFunction(supplyRecord, "GND") {
		t.Fatalf("current-sensing regulator ground endpoint does not match catalog functions: connections=%#v record=%#v", realization.Connections, supplyRecord)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_supply" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_supply", Function: "VOUT"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_feedback_upper", Function: "A"})
	}) || !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_supply_feedback" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_supply", Function: "ADJ"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_feedback_upper", Function: "B"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_feedback_lower", Function: "A"})
	}) {
		t.Fatalf("current-sensing regulator feedback divider is not oriented from output through upper and lower resistors to reference: %#v", realization.Connections)
	}
	calculationIndex := slices.IndexFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "sense_supply_feedback"
	})
	if calculationIndex < 0 {
		t.Fatalf("current-sensing regulator feedback calculation is missing: %#v", expansions[0].Calculations)
	}
	calculation := expansions[0].Calculations[calculationIndex]
	referenceVoltage, referenceOK := catalogSimulationParameter(supplyRecord, "reference_voltage_v")
	if !referenceOK || !slices.Contains(calculation.Inputs, NamedQuantity{Name: "source_voltage", Value: referenceVoltage, Unit: "V"}) {
		t.Fatalf("current-sensing regulator feedback calculation is not bound to the selected catalog reference %g V: %#v", referenceVoltage, calculation.Inputs)
	}
	selectedName, instanceID := "upper_resistance", "sense_feedback_upper"
	if catalogRecordHasSimulationModel(supplyRecord, simmodel.PrimitiveFloatingAdjustableRegulatorV1) {
		selectedName, instanceID = "lower_resistance", "sense_feedback_lower"
	}
	selectedResistance, selectedOK := calculationSelectedValue(calculation, selectedName)
	if !selectedOK || !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == instanceID && instance.Value == engineeringValue(selectedResistance, "Ohm")
	}) {
		t.Fatalf("current-sensing regulator feedback resistor does not match its selected calculation: selected=%g realization=%#v", selectedResistance, realization.Instances)
	}
	for _, instance := range realization.Instances {
		switch instance.ID {
		case "fault_inverter", "control_series", "fault_base_resistor":
			t.Fatalf("measurement-only realization contains interlock instance %#v", instance)
		}
	}
}

func TestCatalogProviderPlacesCurrentSenseShuntInAvailableSeriesRole(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("input", "power", "sink", 4.5, 5.5),
		providerRole("output", "analog_voltage", "source", 0, 2.5),
		providerRole("power", "power", "sink", 4.5, 5.5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 2),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(realization.SeriesTransitions) != 1 || realization.SeriesTransitions[0].Role != "input" {
		t.Fatalf("current-sensing series transitions = %#v", realization.SeriesTransitions)
	}
	if slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "input"
	}) {
		t.Fatalf("series input was also emitted as a scalar binding: %#v", realization.PortBindings)
	}
	if slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "sense_supply"
	}) {
		t.Fatalf("compatible dedicated sensor rail retained an unnecessary regulator: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "power" && binding.Instance == "current_monitor" && (binding.Function == "VCC" || binding.Function == "V_PLUS")
	}) {
		t.Fatalf("compatible dedicated sensor rail was not bound directly: %#v", realization.PortBindings)
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "current_sensor_direct_supply" && calculation.Pass
	}) {
		t.Fatalf("direct sensor-supply proof is missing: %#v", expansions[0].Calculations)
	}
}

func TestCatalogProviderPowersCurrentSensorDirectlyFromCompatibleSeriesRail(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("input", "power", "sink", 4.85, 5.15),
		providerRole("output", "analog_voltage", "source", 0, 2.5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 2),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "sense_supply"
	}) {
		t.Fatalf("compatible sensed series rail retained an unnecessary regulator: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "current_monitor" && instance.CatalogID == "current_sensor.ti.ina168na.sot23_5"
	}) {
		t.Fatalf("current-sense selection did not minimize catalog-backed quiescent current within the required common-mode interval: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "sense_output_load" && instance.Value == "50k"
	}) {
		t.Fatalf("current-output sensor omitted its catalog-programmed load resistance: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_output_load" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "current_monitor", Function: "OUT"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_output_load", Function: "A"})
	}) {
		t.Fatalf("current-output sensor load is not connected to the measurement output: %#v", realization.Connections)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_load_side" &&
			slices.ContainsFunc(connection.Endpoints, func(endpoint RealizationEndpoint) bool {
				return endpoint.Instance == "current_monitor" && (endpoint.Function == "VCC" || endpoint.Function == "V_PLUS")
			}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_input_bypass", Function: "A"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "sense_output_bypass", Function: "A"})
	}) {
		t.Fatalf("compatible sensed load-side rail does not power the current sensor and its bypass network: %#v", realization.Connections)
	}
	if slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_supply"
	}) {
		t.Fatalf("compatible sensed series rail was split into a duplicate supply net: %#v", realization.Connections)
	}
}

func TestCatalogProviderSelectsBipolarCommonModeCurrentSensor(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -12, 12),
		providerRole("output", "analog_voltage", "source", 0, 5),
		providerRole("power", "power", "sink", 16.2, 19.8),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 3, "A", 5),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	if !slices.ContainsFunc(expansions[0].Components, func(component SelectedComponent) bool {
		return component.InstanceID == "current_monitor" && component.CatalogID == "current_sensor.ti.ina149d.soic8"
	}) {
		t.Fatalf("bipolar current-sensing components = %#v", expansions[0].Components)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_output_reference" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "current_monitor", Function: "REF_A"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "current_monitor", Function: "REF_B"})
	}) {
		t.Fatalf("bipolar current-sensing reference network = %#v", realization.Connections)
	}
}

func TestCatalogProviderIncludesDeenergizedSwitchedLoadInCurrentSenseCommonMode(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("input", "switched_load", "sink", 21.6, 26.4),
		providerRole("output", "analog_voltage", "source", 0, 5),
		providerRole("power", "power", "sink", 4.5, 5.5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("full_scale_current", "target", 2, "A", 5),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	componentIndex := slices.IndexFunc(expansions[0].Components, func(component SelectedComponent) bool {
		return component.InstanceID == "current_monitor"
	})
	if componentIndex < 0 {
		t.Fatalf("de-energizable switched-load current sensing omitted its monitor: %#v", expansions[0].Components)
	}
	recordIndex := slices.IndexFunc(provider.catalog.Records, func(record components.ComponentRecord) bool {
		return record.ID == expansions[0].Components[componentIndex].CatalogID
	})
	if recordIndex < 0 {
		t.Fatalf("selected current monitor is absent from the catalog: %#v", expansions[0].Components[componentIndex])
	}
	minimum, minimumOK := recordRatingMinimum(provider.catalog.Records[recordIndex], "common_mode_voltage", "V")
	maximum, maximumOK := recordRatingMaximum(provider.catalog.Records[recordIndex], "common_mode_voltage", "V")
	if !minimumOK || !maximumOK || minimum > 0 || maximum < 26.4 {
		t.Fatalf("selected monitor common-mode range %.9g..%.9g V does not cover de-energized through energized states", minimum, maximum)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	sourceHasMinus := slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_source_side" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "current_monitor", Function: "IN_MINUS"})
	})
	loadHasPlus := slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "sense_load_side" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "current_monitor", Function: "IN_PLUS"})
	})
	if !sourceHasMinus || !loadHasPlus {
		t.Fatalf("switched-load current-sense polarity = %#v", realization.Connections)
	}
}

func TestCatalogProviderSizesCurrentSenseFromMinimumLoadResistance(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "current_sensing", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -16, 16),
		providerRole("output", "analog_voltage", "source", 0, 5),
		providerRole("power", "power", "sink", 16.2, 19.8),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("continuous_output_power", "minimum", 15, "W", 0),
		constraintNumber("load_impedance", "target", 6, "Ohm", 100.0/3),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-sensing expansions=%d err=%v", len(expansions), err)
	}
	required := math.Sqrt(2 * 15.0 / 4)
	found := false
	for _, calculation := range expansions[0].Calculations {
		if calculation.ID != "current_sense_transfer" {
			continue
		}
		for _, input := range calculation.Inputs {
			if input.Name == "full_scale_current" && math.Abs(input.Value-required) <= 1e-9 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("current-sense calculation was not sized from the minimum load resistance: %#v", expansions[0].Calculations)
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
		if binding.Role == "input" {
			t.Fatalf("high-side input must be provided by a series transition, got scalar binding %#v", binding)
		}
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

func TestCatalogProviderHighSideSwitchRealizesIndependentLogicPower(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(28, 2)
	request.Constraints = append(request.Constraints, constraintNumber("startup_output_voltage", "maximum", .5, "V", 0))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("logic-powered high-side expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "logic_power" && binding.Instance == "logic_bypass" && binding.Function == "A"
	}) {
		t.Fatalf("logic-power binding = %#v", realization.PortBindings)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool { return instance.ID == "logic_bypass" }) {
		t.Fatalf("logic-powered high-side topology lacks local bypass: %#v", realization.Instances)
	}
}

func TestCatalogProviderSynthesizesBoundedActiveHighDisconnectInversion(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(28, 2)
	request.Constraints = []Constraint{
		constraintNumber("load_current", "minimum", 2, "A", 10),
		constraintNumber("startup_output_voltage", "maximum", .5, "V", 0),
		constraintString("control_active_state", "equal", "high_disconnect"),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("semantic high-disconnect expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool { return instance.ID == "control_inverter" }) {
		t.Fatalf("semantic high-disconnect path lacks control inverter: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "logic_power" && binding.Instance == "control_inverter_pullup" && binding.Function == "A"
	}) {
		t.Fatalf("semantic high-disconnect logic power binding = %#v", realization.PortBindings)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool { return instance.ID == "logic_bypass" }) {
		t.Fatalf("semantic high-disconnect path lacks local logic bypass: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.RepairVariables, func(variable RealizationRepairVariable) bool {
		return variable.ID == "high_side_control_bias_resistance" && variable.Instance == "control_inverter_base" && len(variable.AllowedValues) >= 2
	}) {
		t.Fatalf("semantic high-disconnect path lacks bounded bias repair: %#v", realization.RepairVariables)
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
		{name: "filter_supply_out_of_range", request: filterProviderRequest(100, 2000), wantError: true},
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

func TestCatalogProviderRequiresReviewedDynamicSwitchEvidenceForElectrothermalLoad(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	request := loadSwitchProviderRequest(30, 4)
	request.Constraints = append(request.Constraints, requiredConstraint("analysis_electrothermal"))

	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("dynamic load-switch expansions = %#v, %v", expansions, err)
	}
	switchID := ""
	for _, component := range expansions[0].Components {
		if component.InstanceID == "mosfet" {
			switchID = component.CatalogID
			break
		}
	}
	for _, record := range catalog.Records {
		if record.ID == switchID && recordHasDynamicElectrothermalEvidence(record) {
			return
		}
	}
	t.Fatalf("dynamic load switch selected %q without reviewed thermal-RC and transient-SOA evidence", switchID)
}

func TestCatalogProviderPublishesSelectedPartSourceCapacity(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), regulatorProviderRequest(5.5, 3.3, 0.25))
	if err != nil || len(expansions) == 0 {
		t.Fatalf("regulator expansions = %#v, %v", expansions, err)
	}
	for _, expansion := range expansions {
		outputIndex := slices.IndexFunc(expansion.OfferedPorts, func(port RoleContract) bool {
			return port.Role == "output"
		})
		if outputIndex < 0 || expansion.OfferedPorts[outputIndex].Contract.CurrentCapacityA == nil {
			t.Fatalf("output capacity missing from %#v", expansion)
		}
		if *expansion.OfferedPorts[outputIndex].Contract.CurrentCapacityA <= 0.25 {
			t.Fatalf("output capacity = %g A, want selected catalog rating above requested load", *expansion.OfferedPorts[outputIndex].Contract.CurrentCapacityA)
		}
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
	if err != nil || len(expansions) == 0 {
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

func TestCatalogProviderVoltageQualifiesShuntTransientClamp(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	protected := providerRole("protected", "switched_load", "source", 18, 30)
	current := 4.0
	protected.Contract.RequiredCurrentCapacityA = &current
	expansions, err := provider.Expand(context.Background(), ProviderRequest{
		Capability: "transient_protection",
		Ports: []RoleContract{
			protected,
			providerRole("reference", "reference", "bidirectional", 0, 0),
		},
	})
	if err != nil || len(expansions) == 0 {
		t.Fatalf("expansions = %#v err = %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.CatalogID == "protection.littelfuse.smcj33ca.smc"
	}) {
		t.Fatalf("30 V protected node did not select its voltage-qualified pulse clamp: %#v", realization.Instances)
	}
}

func TestCatalogProviderVoltageQualifiesSeriesShuntTransientClampFromSignalPath(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	input := providerRole("input", "power", "sink", 11.88, 12.12)
	output := providerRole("output", "power", "source", 11.88, 12.12)
	current := .25
	output.Contract.MaximumCurrentDemandA = &current
	expansions, err := provider.Expand(context.Background(), ProviderRequest{
		Capability: "transient_protection",
		Ports: []RoleContract{
			input, output,
			providerRole("reference", "reference", "bidirectional", 0, 0),
		},
	})
	if err != nil || len(expansions) == 0 {
		t.Fatalf("expansions = %#v err = %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.CatalogID == "protection.littelfuse.smbj18ca.smb"
	}) {
		t.Fatalf("12 V input/output path did not select a voltage-qualified clamp: %#v", realization.Instances)
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

func TestCatalogProviderOffersCatalogProvenThermalBallastWhenFixedRegulatorNeedsPowerSharing(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := regulatorProviderRequest(5.5, 3.3, 0.3)
	request.Ports[0].Contract.Voltage.Minimum = float64Pointer(4.5)
	request.Constraints = append(request.Constraints,
		constraintNumber("ambient_temperature_minimum", "minimum", -20, "degC", 0),
		constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
		constraintBool("analysis_thermal", "required", true),
	)
	expansions, err := provider.expandFixedRegulators(context.Background(), request, 3.3, 2, 4.5, 5.5, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	for _, expansion := range expansions {
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		index := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == "thermal_ballast"
		})
		if index < 0 {
			continue
		}
		ballast := realization.Instances[index]
		if ballast.CatalogID != "resistor.vishay.ac03.1r6.axial" || ballast.Value != "1.6" {
			t.Fatalf("thermal ballast = %#v, want catalog-proven 1.6 ohm AC03", ballast)
		}
		thermalProven := slices.ContainsFunc(expansion.Calculations, func(calculation CalculationEvidence) bool {
			return calculation.ID == "regulator_thermal" && calculation.Pass && calculation.WorstMargin > 0
		})
		powerProven := slices.ContainsFunc(expansion.Calculations, func(calculation CalculationEvidence) bool {
			return calculation.ID == "thermal_ballast_power" && calculation.Pass && calculation.WorstMargin > 0
		})
		if !thermalProven || !powerProven {
			t.Fatalf("thermally ballasted topology lacks positive regulator and resistor power margins: %#v", expansion.Calculations)
		}
		return
	}
	t.Fatalf("fixed-regulator expansions omit a catalog-proven thermal ballast: %#v", expansions)
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
		ok       bool
	}{
		{0.15, "A", "mA", 150, true}, {150, "mA", "A", 0.15, true},
		{3.3, "V", "mV", 3300, true}, {3300, "mV", "V", 3.3, true},
		{2e-9, "C", "nC", 2, true}, {2, "nC", "C", 2e-9, true},
		{16, "MHz", "Hz", 16e6, true}, {16, "mhz", "Hz", 0, false}, {16, "mhz", "mhz", 0, false},
		{22, "pf", "F", 22e-12, true}, {22e-12, "F", "pF", 22, true}, {4.7, "μF", "F", 4.7e-6, true},
		{4.7, "KHz", "Hz", 4.7e3, true}, {4.7, "kohm", "Ohm", 4.7e3, true}, {4.7, "mohm", "mohm", 0, false},
		{1, "mHz", "MHz", 1e-9, true},
	}
	for _, test := range tests {
		got, ok := convertCatalogUnit(test.value, test.from, test.to)
		if !test.ok {
			if ok {
				t.Fatalf("ambiguous convertCatalogUnit(%g, %q, %q) unexpectedly succeeded with %g", test.value, test.from, test.to, got)
			}
			continue
		}
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

func TestCatalogProviderQualifiesMCUProgrammingElectricalLoads(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	qualified := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	qualified.Constraints = append(qualified.Constraints,
		constraintString("programming_kind", "equal", "swd"),
		constraintNumber("debug_load_capacitance", "maximum", 8e-12, "F", 0),
		constraintBool("debug_pins_shared", "required", true),
		constraintNumber("debugger_voltage", "target", 3.3, "V", 0),
	)
	expansions, err := provider.Expand(context.Background(), qualified)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("qualified debug load = %#v, %v", expansions, err)
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "mcu_programming_interface_worst_case" &&
			calculation.Pass && calculation.Hash != "" && len(calculation.Bounds) >= 7
	}) {
		t.Fatalf("qualified debug expansion lacks finalized programming-interface evidence: %#v", expansions[0].Calculations)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if count := countRealizationUsage(realization, "programming_header"); count != 1 {
		t.Fatalf("qualified debug realization uses %d programming headers, want 1", count)
	}
	if count := countRealizationUsage(realization, "programming_series_isolation"); count != 3 {
		t.Fatalf("qualified SWD realization uses %d isolation resistors, want 3", count)
	}
	for _, instance := range realization.Instances {
		if instance.Usage != "programming_series_isolation" {
			continue
		}
		parentNet, headerNet := -1, -1
		for index, connection := range realization.Connections {
			if slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: instance.ID, Function: "A"}) {
				parentNet = index
			}
			if slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: instance.ID, Function: "B"}) {
				headerNet = index
			}
		}
		if parentNet < 0 || headerNet < 0 || parentNet == headerNet {
			t.Fatalf("programming isolation resistor %s does not split the tool and MCU nets: %#v", instance.ID, realization.Connections)
		}
	}

	tests := []struct {
		name       string
		constraint Constraint
	}{
		{"capacitance", constraintNumber("debug_load_capacitance", "maximum", 20e-12, "F", 0)},
		{"unpowered", constraintBool("debugger_connected_while_unpowered", "required", true)},
		{"voltage", constraintNumber("debugger_voltage", "target", 1.8, "V", 0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
			request.Constraints = append(request.Constraints,
				constraintString("programming_kind", "equal", "swd"),
				test.constraint,
			)
			_, err := provider.Expand(context.Background(), request)
			var assignmentErr *mcuAssignmentError
			if !errors.As(err, &assignmentErr) || assignmentErr.Code != CodeMCUProgrammingLoad {
				t.Fatalf("programming-load error = %v, want %s", err, CodeMCUProgrammingLoad)
			}
		})
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
	request.Constraints = append(request.Constraints,
		constraintString("clock_source", "equal", "external_hse"),
		constraintNumber("clock_frequency", "target", 16_000_000, "Hz", 0.1),
		constraintNumber("crystal_stray_capacitance", "maximum", 7e-12, "F", 0),
		constraintNumber("maximum_clock_startup_time", "maximum", 0.02, "s", 0),
	)
	external, err := provider.Expand(context.Background(), request)
	if err != nil || len(external) < 1 {
		t.Fatalf("external-clock expansion = %#v, %v", external, err)
	}
	externalRealization, err := DecodeFragmentRealization(external[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertMCUSupportUsages(t, externalRealization, map[string]bool{"external_crystal_resonator": false, "load_capacitor": false})
	if !slices.ContainsFunc(external[0].Calculations, func(calculation CalculationEvidence) bool {
		if calculation.ID != "mcu_external_crystal_worst_case" ||
			!calculation.Pass || calculation.Hash == "" || len(calculation.Bounds) < 4 {
			return false
		}
		return slices.ContainsFunc(calculation.NominalOutputs, func(output NamedQuantity) bool {
			return output.Name == "selected_load_capacitor_each" && math.Abs(output.Value-22e-12) < 1e-18
		}) && slices.ContainsFunc(calculation.NominalOutputs, func(output NamedQuantity) bool {
			return output.Name == "effective_load_capacitance" && math.Abs(output.Value-18e-12) < 1e-18
		})
	}) {
		t.Fatalf("external clock lacks finalized load, drive, accuracy, and startup evidence: %#v", external[0].Calculations)
	}
	loadCapacitors := 0
	for _, instance := range externalRealization.Instances {
		if instance.Usage == "load_capacitor" {
			loadCapacitors++
			if instance.Value != "22p" {
				t.Fatalf("external crystal load capacitor = %q, want calculated 22p", instance.Value)
			}
		}
	}
	if loadCapacitors != 2 {
		t.Fatalf("external crystal load capacitors = %d, want 2", loadCapacitors)
	}

	missingStray := participantProviderRequest("programmable_controller", "sensor_bus", 3.3)
	missingStray.Constraints = append(missingStray.Constraints,
		constraintString("programming_kind", "equal", "swd"),
		constraintString("clock_source", "equal", "external_hse"),
		constraintNumber("clock_frequency", "target", 16_000_000, "Hz", 0.1),
	)
	_, err = provider.Expand(context.Background(), missingStray)
	var assignmentErr *mcuAssignmentError
	if !errors.As(err, &assignmentErr) || assignmentErr.Code != CodeMCUClockUnavailable {
		t.Fatalf("missing crystal stray-capacitance evidence error = %v, want %s", err, CodeMCUClockUnavailable)
	}
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

func TestCatalogProviderPublishesIsolatedTranslatorAsSupportedFunction(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := translatorProviderRequest(3.3, 5)
	request.Constraints = append(request.Constraints, constraintBool("reference_separation", "required", true))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "bus_isolator" && instance.Usage == "bidirectional_i2c_isolation"
	}) {
		t.Fatalf("isolated translator lacks supported function identity: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "power_b" && binding.Lane == "return" && binding.Instance == "bus_isolator" && binding.Function == "GND2"
	}) {
		t.Fatalf("isolated translator lacks distinct side-B return binding: %#v", realization.PortBindings)
	}
}

func TestCatalogProviderSelectsProtectedWideInputIsolatedConverter(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	input := providerRole("input", "power", "sink", 9, 30)
	output := providerRole("output", "power", "source", 5, 5)
	output.Contract.RequiredCurrentCapacityA = float64Pointer(.4)
	request := ProviderRequest{
		Capability: "voltage_regulation",
		Ports: []RoleContract{
			input, output,
			providerRole("reference", "reference", "bidirectional", 0, 0),
		},
		Constraints: []Constraint{
			constraintBool("isolation_required", "required", true),
			constraintNumber("output_voltage", "target", 5, "V", 5),
			constraintNumber("isolation_working_voltage", "minimum", 1000, "V", 0),
		},
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("protected isolated conversion = %#v, %v", expansions, err)
	}
	index := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "protected_wide_input_isolated_converter"
	})
	if index < 0 {
		t.Fatalf("protected isolated conversion missing from %#v", expansions)
	}
	realization, err := DecodeFragmentRealization(expansions[index].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.CatalogID == "isolated_converter.traco.tec3_2411ui.sip8" && instance.Usage == "protected_isolated_power_stage"
	}) {
		t.Fatalf("protected converter selection = %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_input_bypass" &&
			instance.CatalogID == "capacitor.murata.gcm21br71h105ka03l.0805" &&
			instance.Usage == "input_bypass_capacitor"
	}) {
		t.Fatalf("protected converter lacks a voltage-qualified input bypass: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(expansions[index].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "protected_isolated_converter_bounds" && calculation.Pass &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "input_bypass_voltage" && bound.Pass && bound.ObservedWorst >= 30
			})
	}) {
		t.Fatalf("protected converter lacks finalized bounds: %#v", realization)
	}
}

func TestCatalogProviderSizesProtectedConverterShutdownDischarge(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	input := providerRole("input", "power", "sink", 18, 36)
	output := providerRole("output", "power", "source", 12, 12)
	output.Contract.RequiredCurrentCapacityA = float64Pointer(.25)
	request := ProviderRequest{
		Capability: "voltage_regulation",
		Ports: []RoleContract{
			input, output,
			providerRole("reference", "reference", "bidirectional", 0, 0),
			providerRole("shutdown", "digital_logic", "sink", 0, 3.3),
		},
		Constraints: []Constraint{
			constraintBool("isolation_required", "required", true),
			constraintNumber("output_voltage", "target", 12, "V", 1),
			constraintNumber("isolation_working_voltage", "minimum", 1000, "V", 0),
			constraintNumber("maximum_inrush_current", "maximum", .3, "A", 0),
			constraintNumber("shutdown_discharge_voltage", "maximum", 2, "V", 0),
			constraintNumber("shutdown_discharge_time", "maximum", .5, "s", 0),
			constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
			constraintNumber("junction_temperature", "maximum", 85, "degC", 0),
		},
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("protected isolated conversion = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "protected_isolated_converter" &&
			instance.CatalogID == "isolated_converter.traco.tri10_1212.dip24"
	}) {
		t.Fatalf("thermal envelope did not select the qualifying protected converter: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_pre_regulator" &&
			instance.CatalogID == "regulator.traco.tsr1_24120.sip3" &&
			instance.Usage == "synchronous_buck_controller"
	}) {
		t.Fatalf("wide-input request lacks a reviewed fixed preregulator: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_pre_regulator_input_bypass" &&
			instance.CatalogID == "capacitor.panasonic.eeufr1h220.radial" &&
			instance.Value == "22u"
	}) {
		t.Fatalf("wide-input preregulator lacks its voltage-qualified input bypass: %#v", realization.Instances)
	}
	efuseIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_input_efuse" &&
			instance.CatalogID == "protection.ti.tps26600pwp.htssop16" &&
			instance.Usage == "overcurrent_limit"
	})
	if efuseIndex < 0 {
		t.Fatalf("protected converter lacks programmable input protection: %#v", realization.Instances)
	}
	parameters := map[string]float64{}
	for _, parameter := range realization.Instances[efuseIndex].Parameters {
		parameters[parameter.Name] = parameter.Value
	}
	requiredInputCurrent := 12 * .25 / (.86 * .92 * 18)
	if parameters["minimum_current_limit_a"] < requiredInputCurrent ||
		parameters["maximum_current_limit_a"] > .3 ||
		parameters["programmed_current_limit_a"] <= 0 ||
		parameters["maximum_output_slew_v_per_s"] <= 0 {
		t.Fatalf("programmable input-protection parameters = %#v, required input current %.12g A", parameters, requiredInputCurrent)
	}
	limitResistorIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_input_current_limit"
	})
	catalogEFuseIndex := slices.IndexFunc(catalog.Records, func(record components.ComponentRecord) bool {
		return record.ID == "protection.ti.tps26600pwp.htssop16"
	})
	if limitResistorIndex < 0 || catalogEFuseIndex < 0 {
		t.Fatalf("current-limit relationship lacks resolved resistor or eFuse catalog record")
	}
	limitResistance, resistanceOK := components.ParseEngineeringValue(realization.Instances[limitResistorIndex].Value)
	programmingConstant, constantOK := recordValue(
		catalog.Records[catalogEFuseIndex], "current_limit_programming_constant", "A*Ohm",
	)
	if !resistanceOK || !constantOK ||
		math.Abs(parameters["programmed_current_limit_a"]*limitResistance-programmingConstant) > programmingConstant*1e-9 {
		t.Fatalf(
			"programmed current %.12g A and resistor %.12g Ohm do not reproduce catalog constant %.12g A*Ohm",
			parameters["programmed_current_limit_a"], limitResistance, programmingConstant,
		)
	}
	if !slices.Contains(realization.PortBindings, RealizationPortBinding{
		Role: "shutdown", Instance: "converter_input_efuse", Function: "SHDN",
	}) {
		t.Fatalf("shutdown is not bound to the current-limiting input stage: %#v", realization.PortBindings)
	}
	netFor := func(endpoint RealizationEndpoint) string {
		for _, connection := range realization.Connections {
			if slices.Contains(connection.Endpoints, endpoint) {
				return connection.ID
			}
		}
		return ""
	}
	upstreamNet := netFor(RealizationEndpoint{Instance: "converter_input_efuse", Function: "VIN"})
	protectedNet := netFor(RealizationEndpoint{Instance: "converter_input_efuse", Function: "VOUT"})
	converterInputNet := netFor(RealizationEndpoint{Instance: "protected_isolated_converter", Function: "VIN_PLUS"})
	if upstreamNet == "" || protectedNet == "" || converterInputNet == "" ||
		upstreamNet == protectedNet || protectedNet == converterInputNet ||
		protectedNet != netFor(RealizationEndpoint{Instance: "converter_pre_regulator", Function: "VIN"}) ||
		converterInputNet != netFor(RealizationEndpoint{Instance: "converter_pre_regulator", Function: "VOUT"}) {
		t.Fatalf("eFuse and preregulator do not form distinct series stages ahead of the converter: %#v", realization.Connections)
	}
	dischargeIndex := slices.IndexFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "converter_output_discharge" && instance.Usage == "shutdown_discharge"
	})
	if dischargeIndex < 0 {
		t.Fatalf("protected converter lacks discharge resistor: %#v", realization.Instances)
	}
	discharge := realization.Instances[dischargeIndex]
	resistance, ok := components.ParseEngineeringValue(discharge.Value)
	if !ok || resistance <= 0 {
		t.Fatalf("discharge resistance = %q", discharge.Value)
	}
	remaining := 12 * math.Exp(-.5/(resistance*protectedConverterOutputCapacitanceMaximumF))
	if remaining > 2 {
		t.Fatalf("discharge leaves %.12g V, want <= 2 V", remaining)
	}
	for _, function := range []string{"A", "B"} {
		if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
			return slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: discharge.ID, Function: function})
		}) {
			t.Fatalf("discharge resistor function %s is not connected: %#v", function, realization.Connections)
		}
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "protected_isolated_converter_bounds" && calculation.Pass &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "shutdown_output_voltage" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "input_current_limit_capacity" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "current_limited_inrush" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "overvoltage_operating_margin" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "junction_temperature" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "pre_regulator_output_current" && bound.Pass
			}) &&
			slices.ContainsFunc(calculation.Bounds, func(bound CalculationBound) bool {
				return bound.Name == "pre_regulator_input_headroom" && bound.Pass
			})
	}) {
		t.Fatalf("protected converter lacks discharge proof: %#v", expansions[0].Calculations)
	}

	impossible := request
	impossible.Constraints = slices.Clone(request.Constraints)
	for index := range impossible.Constraints {
		if impossible.Constraints[index].Name == "shutdown_discharge_time" {
			impossible.Constraints[index] = constraintNumber("shutdown_discharge_time", "maximum", 1e-6, "s", 0)
		}
	}
	if _, err := provider.Expand(context.Background(), impossible); err == nil {
		t.Fatal("impossible shutdown discharge was accepted")
	}
}

func TestCatalogProviderAllowsExplicitSharedReferenceTranslator(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := translatorProviderRequest(3.3, 5)
	request.Constraints = append(request.Constraints, constraintBool("reference_separation", "required", false))
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.Usage == "level_translator"
	}) {
		t.Fatalf("explicit shared-reference request lacks a level translator: %#v", realization.Instances)
	}
	if slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return strings.Contains(strings.ToLower(instance.Usage), "isolat")
	}) {
		t.Fatalf("explicit shared-reference request selected isolation: %#v", realization.Instances)
	}
}

func TestTranslatorEvidenceModeAndDirectionMatchingIsCaseInsensitive(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	for _, record := range catalog.Records {
		if record.Translator == nil || record.Translator.MaximumOpenDrainFrequency == nil {
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

func TestTranslatorEvidenceUsesModeSpecificFrequencyBounds(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	for _, record := range catalog.Records {
		if record.ID != "level_translator.ti.txs0104epw.tssop14" {
			continue
		}
		if !translatorEvidenceSupports(record, 1.8, 3.3, "push_pull", "bidirectional", 24_000_000, 4, true) {
			t.Fatal("TXS0104E push-pull evidence was rejected")
		}
		if translatorEvidenceSupports(record, 1.8, 3.3, "open_drain", "bidirectional", 3_000_000, 2, true) {
			t.Fatal("TXS0104E open-drain evidence exceeded its mode-specific bound")
		}
		return
	}
	t.Fatal("TXS0104E record is missing")
}

func TestCatalogProviderComposesWholePushPullTranslationBus(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := pushPullTranslatorProviderRequest(1.8, 3.3, 8, 8_000_000)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatalf("push-pull translation: %v", err)
	}
	compactIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "push_pull_compact_08_channel"
	})
	segmentedIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "push_pull_segmented_08_channel"
	})
	if compactIndex < 0 || segmentedIndex < 0 {
		t.Fatalf("push-pull translation lacks materially distinct compact and segmented expansions (count=%d)", len(expansions))
	}
	realization, err := DecodeFragmentRealization(expansions[compactIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	translators := 0
	for _, instance := range realization.Instances {
		if instance.Usage == "push_pull_level_translator" {
			translators++
			if !slices.Contains(instance.Parameters, RealizationParameter{Name: "direction", Value: 1, Unit: "polarity"}) {
				t.Fatalf("translator lacks selected simulation direction: %#v", instance)
			}
		}
	}
	if translators != 2 {
		t.Fatalf("translator count = %d, realization %#v", translators, realization.Instances)
	}
	sideBindings := 0
	for _, binding := range realization.PortBindings {
		if binding.Role == "side_a" || binding.Role == "side_b" {
			sideBindings++
			if binding.Lane == "" {
				t.Fatalf("whole-bus binding lacks a lane: %#v", binding)
			}
		}
	}
	if sideBindings != 16 {
		t.Fatalf("whole-bus bindings = %d, want 16: %#v", sideBindings, realization.PortBindings)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.Usage == "enable_pulldown"
	}) {
		t.Fatalf("push-pull translation lacks a defined inactive startup state: %#v", realization.Instances)
	}
	segmented, err := DecodeFragmentRealization(expansions[segmentedIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	segmentedTranslators := 0
	for _, instance := range segmented.Instances {
		if instance.Usage == "push_pull_level_translator" {
			segmentedTranslators++
		}
	}
	if segmentedTranslators != 4 {
		t.Fatalf("segmented translation uses %d translators, want 4", segmentedTranslators)
	}
}

func TestCatalogProviderLeavesUnusedAutoDirectionChannelsUnconnected(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := pushPullTranslatorProviderRequest(1.8, 5, 1, 4_000_000)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatalf("push-pull translation: %v", err)
	}
	index := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "push_pull_compact_01_channel"
	})
	if index < 0 {
		t.Fatalf("missing compact one-channel expansion: %#v", expansions)
	}
	realization, err := DecodeFragmentRealization(expansions[index].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range realization.Connections {
		for _, endpoint := range connection.Endpoints {
			if endpoint.Instance != "bus_translator_01" {
				continue
			}
			switch endpoint.Function {
			case "A2", "A3", "A4", "B2", "B3", "B4":
				t.Fatalf("unused auto-direction channel was connected to %s: %#v", connection.Role, connection)
			}
		}
	}
}

func TestCatalogProviderPushPullTranslationFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tooFast := pushPullTranslatorProviderRequest(1.8, 3.3, 2, 25_000_000)
	if _, err := provider.Expand(context.Background(), tooFast); err == nil {
		t.Fatal("out-of-envelope push-pull rate was accepted")
	}
	missingEnable := pushPullTranslatorProviderRequest(1.8, 3.3, 2, 8_000_000)
	missingEnable.Ports = slices.DeleteFunc(missingEnable.Ports, func(port RoleContract) bool { return port.Role == "enable" })
	if _, err := provider.Expand(context.Background(), missingEnable); err == nil {
		t.Fatal("push-pull translation without startup enable was accepted")
	}
	bidirectional := pushPullTranslatorProviderRequest(1.8, 3.3, 2, 8_000_000)
	for index := range bidirectional.Ports {
		if bidirectional.Ports[index].Role == "side_a" || bidirectional.Ports[index].Role == "side_b" {
			bidirectional.Ports[index].Contract.Direction = "bidirectional"
		}
	}
	if _, err := provider.Expand(context.Background(), bidirectional); err == nil {
		t.Fatal("bidirectional push-pull translation without direction-control evidence was accepted")
	}
}

func TestCatalogProviderComposesDirectionControlledTranslationBus(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := directionControlledTranslatorProviderRequest(1.8, 3.3, 8, 8_000_000)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatalf("direction-controlled translation: %v", err)
	}
	compactIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "direction_controlled_compact_08_channel"
	})
	segmentedIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "direction_controlled_segmented_08_channel"
	})
	if compactIndex < 0 || segmentedIndex < 0 {
		t.Fatalf("direction-controlled translation lacks compact and segmented expansions: %#v", expansions)
	}
	realization, err := DecodeFragmentRealization(expansions[compactIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if count := countRealizationUsage(realization, "direction_controlled_level_translator"); count != 1 {
		t.Fatalf("compact translation uses %d transceivers, want 1", count)
	}
	for _, usage := range []string{"enable_pullup", "direction_pulldown"} {
		if count := countRealizationUsage(realization, usage); count != 1 {
			t.Fatalf("compact translation uses %d %s parts, want 1", count, usage)
		}
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "bus_transceiver_01", Function: "VCCB_AUX"})
	}) {
		t.Fatal("second VCCB package pin is not connected")
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "bus_transceiver_01", Function: "GND_AUX"})
	}) {
		t.Fatal("second ground package pin is not connected")
	}
	segmented, err := DecodeFragmentRealization(expansions[segmentedIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if count := countRealizationUsage(segmented, "direction_controlled_level_translator"); count != 2 {
		t.Fatalf("segmented translation uses %d transceivers, want 2", count)
	}
	if !slices.ContainsFunc(segmented.Connections, func(connection RealizationConnection) bool {
		return slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "bus_transceiver_01", Function: "A5"}) &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "bus_transceiver_01", Function: "B5"})
	}) {
		t.Fatal("segmented alternative does not ground unused data inputs")
	}
}

func TestCatalogProviderDirectionControlledTranslationFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	missingDirectionRole := directionControlledTranslatorProviderRequest(1.8, 3.3, 8, 8_000_000)
	missingDirectionRole.Ports = slices.DeleteFunc(missingDirectionRole.Ports, func(port RoleContract) bool { return port.Role == "direction_control" })
	if _, err := provider.Expand(context.Background(), missingDirectionRole); err == nil {
		t.Fatal("direction-controlled translation without a direction role was accepted")
	}
	unsafeDirectionChange := directionControlledTranslatorProviderRequest(1.8, 3.3, 8, 8_000_000)
	unsafeDirectionChange.Constraints = slices.DeleteFunc(unsafeDirectionChange.Constraints, func(constraint Constraint) bool { return constraint.Name == "direction_change_state" })
	if _, err := provider.Expand(context.Background(), unsafeDirectionChange); err == nil {
		t.Fatal("direction-controlled translation without disabled direction-change proof was accepted")
	}
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

func TestCatalogProviderSizesGalvanicIsolationPullupsFromRiseTimeAndCapacitance(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := translatorProviderRequest(3.3, 1.8)
	request.Capability = "galvanic_isolation"
	request.Constraints = []Constraint{
		constraintNumber("isolation_voltage", "minimum", 1000, "V", 0),
		constraintNumber("side_b_rise_time", "maximum", 1e-6, "s", 0),
		constraintNumber("side_b_load_capacitance", "maximum", 4e-10, "F", 0),
	}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"isolation_b_sda_pullup", "isolation_b_scl_pullup"} {
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
	for _, id := range []string{"isolation_a_sda_pullup", "isolation_a_scl_pullup"} {
		if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id && instance.Value == "4.7k"
		}) {
			t.Fatalf("%s should retain its unconstrained default: %#v", id, realization.Instances)
		}
	}
}

func TestCatalogProviderComposesMultiPartPushPullFunctionalIsolation(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := pushPullFunctionalIsolationProviderRequest(3.3, 5, 16, 1, 8_000_000)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil {
		t.Fatalf("push-pull functional isolation: %v", err)
	}
	compactIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "push_pull_functional_isolation_compact_16_forward_01_reverse"
	})
	segmentedIndex := slices.IndexFunc(expansions, func(expansion ProviderExpansion) bool {
		return expansion.ID == "push_pull_functional_isolation_segmented_16_forward_01_reverse"
	})
	if compactIndex < 0 || segmentedIndex < 0 {
		t.Fatalf("functional isolation lacks compact and segmented alternatives: %#v", expansions)
	}
	realization, err := DecodeFragmentRealization(expansions[compactIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := countRealizationUsage(realization, "push_pull_functional_isolation"); got != 6 {
		t.Fatalf("compact isolator count = %d, want 6: %#v", got, realization.Instances)
	}
	roleCounts := map[string]int{}
	for _, binding := range realization.PortBindings {
		roleCounts[binding.Role]++
		if strings.Contains(binding.Role, "forward") || strings.Contains(binding.Role, "reverse") {
			if binding.Lane == "" {
				t.Fatalf("functional isolation signal binding lacks channel lane: %#v", binding)
			}
		}
	}
	for role, want := range map[string]int{
		"side_a_forward": 16, "side_b_forward": 16,
		"side_b_reverse": 1, "side_a_reverse": 1,
	} {
		if roleCounts[role] != want {
			t.Fatalf("role %s bindings = %d, want %d: %#v", role, roleCounts[role], want, realization.PortBindings)
		}
	}
	if !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "functional_isolator_power_a" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "functional_isolator_01", Function: "EN1"})
	}) || !slices.ContainsFunc(realization.Connections, func(connection RealizationConnection) bool {
		return connection.ID == "functional_isolator_power_b" &&
			slices.Contains(connection.Endpoints, RealizationEndpoint{Instance: "functional_isolator_01", Function: "EN2"})
	}) {
		t.Fatalf("isolator enables are not deterministically tied high: %#v", realization.Connections)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.Usage == "unused_output_load" && instance.Value == "1M"
	}) {
		t.Fatalf("unused physical channels lack bounded output loads: %#v", realization.Instances)
	}
	if len(expansions[compactIndex].Calculations) != 1 ||
		!slices.ContainsFunc(expansions[compactIndex].Calculations[0].Bounds, func(bound CalculationBound) bool {
			return bound.Name == "working_isolation_voltage" && bound.Pass && bound.Required == 1000 && bound.ObservedWorst == 1500
		}) {
		t.Fatalf("working-isolation evidence is missing: %#v", expansions[compactIndex].Calculations)
	}
	segmented, err := DecodeFragmentRealization(expansions[segmentedIndex].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := countRealizationUsage(segmented, "push_pull_functional_isolation"); got != 16 {
		t.Fatalf("segmented isolator count = %d, want 16", got)
	}
}

func TestFunctionalIsolationChannelsDeriveFromCatalogPinMap(t *testing.T) {
	record := components.ComponentRecord{
		ID: "isolator.test.reordered",
		Symbols: []components.SymbolBinding{{
			SymbolID: "test:reordered",
			FunctionPins: []components.FunctionPin{
				{Function: "OUTA3"}, {Function: "INA2"}, {Function: "OUTB7"},
				{Function: "INB3"}, {Function: "OUTB2"}, {Function: "INA7"},
			},
		}},
	}
	forward, reverse, err := functionalIsolationChannels(record)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(forward, []functionalIsolationChannel{
		{input: "INA2", output: "OUTB2"},
		{input: "INA7", output: "OUTB7"},
	}) || !slices.Equal(reverse, []functionalIsolationChannel{
		{input: "INB3", output: "OUTA3"},
	}) {
		t.Fatalf("derived channels = forward %#v reverse %#v", forward, reverse)
	}
}

func TestCatalogProviderPushPullFunctionalIsolationFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ProviderRequest)
	}{
		{name: "missing_working_voltage", mutate: func(request *ProviderRequest) {
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
				return constraint.Name == "isolation_working_voltage"
			})
		}},
		{name: "unsafe_default", mutate: func(request *ProviderRequest) {
			for index := range request.Constraints {
				if request.Constraints[index].Name == "supply_loss_safe_state" {
					request.Constraints[index] = constraintString("supply_loss_safe_state", "equal", "high")
				}
			}
		}},
		{name: "outside_working_voltage", mutate: func(request *ProviderRequest) {
			for index := range request.Constraints {
				if request.Constraints[index].Name == "isolation_working_voltage" {
					request.Constraints[index] = constraintNumber("isolation_working_voltage", "minimum", 1600, "V", 0)
				}
			}
		}},
		{name: "outside_frequency", mutate: func(request *ProviderRequest) {
			for index := range request.Ports {
				if request.Ports[index].Contract.Protocol != nil {
					request.Ports[index].Contract.Protocol.MaxFrequencyHz = 101_000_000
				}
			}
		}},
		{name: "missing_reverse_endpoint", mutate: func(request *ProviderRequest) {
			request.Ports = slices.DeleteFunc(request.Ports, func(port RoleContract) bool {
				return port.Role == "side_a_reverse"
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := pushPullFunctionalIsolationProviderRequest(3.3, 5, 3, 1, 8_000_000)
			test.mutate(&request)
			if expansions, err := provider.Expand(context.Background(), request); err == nil {
				t.Fatalf("out-of-envelope request produced expansions: %#v", expansions)
			}
		})
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
	}}
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

func TestCatalogProviderSelectsSignalAmplifierForProjectedPowerSwing(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "signal_amplification", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -16, 16),
		providerRole("output", "analog_voltage", "source", -16, 16),
		providerRole("positive_power", "power", "sink", 16.2, 19.8),
		providerRole("negative_power", "power", "sink", -19.8, -16.2),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("continuous_output_power", "minimum", 15, "W", 0),
		constraintNumber("load_impedance", "target", 6, "Ohm", 100.0/3),
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
		return instance.ID == "gain_amplifier" && instance.CatalogID == "opamp.ti.opa992idbvr.sot23_5"
	}) {
		t.Fatalf("power-swing-constrained signal amplifier did not select reviewed rail-headroom evidence: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		required, requiredOK := calculationOutput(calculation, "required_peak_output")
		return calculation.ID == "signal_amplifier_output_swing" &&
			requiredOK && math.Abs(required-math.Sqrt(2*15*8)) <= 1e-9
	}) {
		t.Fatalf("power-swing-constrained signal amplifier lacks output-swing evidence: %#v", expansions[0].Calculations)
	}
}

func TestCatalogProviderAddsResponseDrivenClassABCurrentLimiting(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -2, 2),
		providerRole("output", "analog_voltage", "source", -16, 16),
		providerRole("positive_power", "power", "sink", 16.2, 19.8),
		providerRole("negative_power", "power", "sink", -19.8, -16.2),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "target", 6, "ohm", 100.0/3),
		constraintNumber("continuous_output_power", "minimum", 15, "W", 0),
		constraintNumber("protection_response_time", "maximum", .001, "s", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for id, catalogID := range map[string]string{
		"upper_current_limiter_1": "bjt.onsemi.mje253g.to225",
		"lower_current_limiter_1": "bjt.onsemi.mje243g.to225",
	} {
		if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id && instance.CatalogID == catalogID
		}) {
			t.Fatalf("response-driven Class AB output omitted %s: %#v", id, realization.Instances)
		}
	}
	if !slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
		responseTime, responseOK := calculationOutput(calculation, "response_time")
		currentLimit, currentOK := calculationOutput(calculation, "current_limit")
		worstRequiredPeakCurrent := math.Sqrt(2*15*8) / 4
		return calculation.ID == "class_ab_current_limit_response" &&
			responseOK && responseTime <= .001 &&
			currentOK && currentLimit > worstRequiredPeakCurrent && currentLimit <= 8.1
	}) {
		t.Fatalf("response-driven Class AB output lacks normal-load headroom and bounded response evidence: %#v", expansions[0].Calculations)
	}
	if !slices.ContainsFunc(realization.RepairVariables, func(variable RealizationRepairVariable) bool {
		return variable.ID == "class_ab_current_limit_sense_resistance" &&
			len(variable.Instances) >= 2 &&
			slices.ContainsFunc(variable.Effects, func(effect RealizationRepairEffect) bool {
				return effect.Analysis == simmodel.AnalysisElectrothermal &&
					effect.Metric == "transient_soa_margin" &&
					effect.Direction == "metric_increases"
			})
	}) {
		t.Fatalf("response-driven Class AB output lacks bounded current-limit repair: %#v", realization.RepairVariables)
	}
}

func TestCatalogProviderCompensatesUnityGainClassABLoopWhenStabilityIsRequired(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_output_stage", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -2, 2),
		providerRole("output", "analog_voltage", "source", -16, 16),
		providerRole("positive_power", "power", "sink", 16.2, 19.8),
		providerRole("negative_power", "power", "sink", -19.8, -16.2),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("load_impedance", "target", 6, "ohm", 100.0/3),
		constraintNumber("continuous_output_power", "minimum", 15, "W", 0),
		constraintNumber("phase_margin", "minimum", 50, "deg", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"feedback_upper", "feedback_compensation"} {
		if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
			return instance.ID == id
		}) {
			t.Fatalf("stability-constrained unity Class-AB output omitted %s: %#v", id, realization.Instances)
		}
	}
	if slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "feedback_lower"
	}) {
		t.Fatalf("unity Class-AB compensation introduced finite DC noise gain: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "voltage_driver" && instance.CatalogID == "opamp.ti.opa992idbvr.sot23_5"
	}) {
		t.Fatalf("stability-constrained Class-AB voltage driver lacks reviewed supply-span and output-swing headroom: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.RepairVariables, func(variable RealizationRepairVariable) bool {
		return variable.ID == "class_ab_feedback_compensation" &&
			slices.ContainsFunc(variable.Effects, func(effect RealizationRepairEffect) bool {
				return effect.Analysis == simmodel.AnalysisStability &&
					effect.Metric == "phase_margin" &&
					effect.Direction == "metric_increases"
			})
	}) {
		t.Fatalf("stability-constrained Class-AB output lacks bounded compensation repair: %#v", realization.RepairVariables)
	}
}

func TestCatalogProviderDoesNotTreatClassABSignalInputAsEnable(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "class_ab_bias_control", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", -2, 2),
		providerRole("output", "analog_voltage", "source", -2, 2),
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
	for _, instance := range realization.Instances {
		switch instance.ID {
		case "bias_enable_inverter", "bias_clamp", "enable_resistor", "inverter_pullup":
			t.Fatalf("analog signal input created digital enable hardware: %#v", realization.Instances)
		}
	}
	inputBinding := slices.IndexFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "input" && binding.Instance == "signal_path" && binding.Function == "A"
	})
	outputBinding := slices.IndexFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
		return binding.Role == "output" && binding.Instance == "signal_path" && binding.Function == "B"
	})
	if inputBinding < 0 || outputBinding < 0 {
		t.Fatalf("analog bias signal path bindings = %#v", realization.PortBindings)
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

func TestCatalogProviderOutputFuseIncludesDownstreamSupportCurrent(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("input", "power", "sink", 4.85, 5.15),
		providerRole("output", "power", "source", 4.85, 5.15),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}}
	request.Ports[1].Contract.RequiredCurrentCapacityA = float64Pointer(2)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_fuse" && instance.CatalogID == "fuse.littelfuse.0437003wr.1206"
	}) {
		t.Fatalf("series fuse did not include its downstream support current and minimize cold resistance within the smallest adequate current tier: %#v", realization.Instances)
	}
}

func TestCatalogProviderUsesReviewedControlledDisconnectForOutputProtection(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("control", "digital_logic", "sink", 0, 3.3),
		providerRole("input", "power", "sink", 4.85, 5.15),
		providerRole("output", "power", "source", 4.85, 5.15),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("overcurrent_limit", "maximum", 0.8, "A", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("Expand() = %#v, %v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if expansions[0].ID != "controlled_high_side_output_protection" || !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_disconnect" && instance.CatalogID == "mosfet.aos.aod21357.to252"
	}) {
		t.Fatalf("controlled output protection = %#v", realization)
	}
	for role, target := range map[string]RealizationEndpoint{
		"control": {Instance: "disconnect_control_series", Function: "A"},
		"input":   {Instance: "output_disconnect", Function: "SOURCE"},
	} {
		if !slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool {
			return binding.Role == role && binding.Instance == target.Instance && binding.Function == target.Function
		}) {
			t.Fatalf("%s binding absent from %#v", role, realization.PortBindings)
		}
	}
	if slices.ContainsFunc(realization.PortBindings, func(binding RealizationPortBinding) bool { return binding.Role == "output" }) ||
		len(realization.SeriesTransitions) != 1 ||
		realization.SeriesTransitions[0].Role != "output" ||
		realization.SeriesTransitions[0].Input != (RealizationEndpoint{Instance: "output_disconnect", Function: "SOURCE"}) ||
		realization.SeriesTransitions[0].Output != (RealizationEndpoint{Instance: "output_fuse", Function: "B"}) {
		t.Fatalf("controlled output protection does not preserve its source-to-protected series path: %#v", realization)
	}
}

func TestBindRolesDoesNotInventCatalogFunctions(t *testing.T) {
	bindings := bindRoles([]RoleContract{
		providerRole("input", "power", "sink", 4.75, 5.25),
		providerRole("fault", "digital_logic", "source", 0, 5.25),
	}, "device", map[string]string{"input": "VIN"})
	if len(bindings) != 1 || bindings[0].Role != "input" || bindings[0].Function != "VIN" {
		t.Fatalf("bindRoles invented an unmapped catalog function: %#v", bindings)
	}
}

func TestCatalogProviderRealizesSensedFaultOutputProtectionRoles(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("input", "power", "sink", 4.85, 5.15),
		providerRole("output", "power", "source", 4.85, 5.15),
		providerRole("sense", "analog_voltage", "sink", 0, 5.15),
		providerRole("fault", "digital_logic", "source", 0, 5.15),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("overcurrent_limit", "maximum", 2, "A", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) < 2 {
		t.Fatalf("sensed output-protection expansions=%d err=%v", len(expansions), err)
	}
	for _, expansion := range expansions {
		realization, decodeErr := DecodeFragmentRealization(expansion.Payload)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		roles := map[string]bool{}
		for _, binding := range realization.PortBindings {
			roles[binding.Role] = true
			if binding.Function == "SENSE" || binding.Function == "FAULT" {
				t.Fatalf("%s invented catalog function in %#v", expansion.ID, binding)
			}
		}
		for _, transition := range realization.SeriesTransitions {
			roles[transition.Role] = true
		}
		for _, role := range []string{"input", "output", "sense", "fault", "reference"} {
			if !roles[role] {
				t.Fatalf("%s omitted role %s: %#v", expansion.ID, role, realization)
			}
		}
	}
}

func TestCatalogProviderRealizesCurrentLimitCommandInterlockRoles(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "safety_interlock", Ports: []RoleContract{
		providerRole("command", "digital_logic", "sink", 0, 5.25),
		providerRole("sense", "analog_voltage", "sink", 0, 5.25),
		providerRole("control", "analog_control", "source", 0, 5.25),
		providerRole("fault", "digital_logic", "source", 0, 5.25),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("current-limit interlock expansions=%d err=%v", len(expansions), err)
	}
	if expansions[0].ID != "current_limit_command_interlock" {
		t.Fatalf("current-limit interlock id = %q", expansions[0].ID)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]bool{}
	for _, binding := range realization.PortBindings {
		roles[binding.Role] = true
		switch binding.Function {
		case "COMMAND", "CONTROL", "FAULT", "SENSE":
			t.Fatalf("interlock invented catalog function in %#v", binding)
		}
	}
	for _, role := range []string{"command", "control", "fault", "sense", "reference"} {
		if !roles[role] {
			t.Fatalf("interlock omitted role %s: %#v", role, realization)
		}
	}
	latchInstance := false
	for _, instance := range realization.Instances {
		if instance.ID == "fault_latch_feedback" && instance.Value == "39k" && instance.Usage == "threshold_divider" {
			latchInstance = true
		}
	}
	latchNets := map[string]bool{}
	for _, connection := range realization.Connections {
		for _, endpoint := range connection.Endpoints {
			if endpoint.Instance == "fault_latch_feedback" {
				latchNets[connection.ID+"\x00"+endpoint.Function] = true
			}
		}
	}
	if !latchInstance ||
		!latchNets["interlock_fault_output\x00A"] ||
		!latchNets["interlock_sense_threshold\x00B"] {
		t.Fatalf("interlock fault latch is incomplete: instances=%#v connections=%#v", realization.Instances, realization.Connections)
	}
}

func TestCatalogProviderRealizesPolarityNormalizedProtectionDominantPermit(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name           string
		enablePolarity string
		faultPolarity  string
		permitPolarity string
	}{
		{name: "all_active_high", enablePolarity: "active_high", faultPolarity: "active_high", permitPolarity: "active_high"},
		{name: "active_low_enable", enablePolarity: "active_low", faultPolarity: "active_high", permitPolarity: "active_high"},
		{name: "active_low_fault", enablePolarity: "active_high", faultPolarity: "active_low", permitPolarity: "active_high"},
		{name: "active_low_output", enablePolarity: "active_high", faultPolarity: "active_high", permitPolarity: "active_low"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := ProviderRequest{Capability: "safety_interlock", Ports: []RoleContract{
				providerRole("enable", "digital_logic", "sink", 0, 5.5),
				providerRole("fault", "digital_logic", "sink", 0, 5.5),
				providerRole("permit", "digital_logic", "source", 0, 5.5),
				providerRole("power", "power", "sink", 4.5, 5.5),
				providerRole("reference", "reference", "bidirectional", 0, 0),
			}, Constraints: []Constraint{
				stringConstraint("enable_control_function", "equal", "enable"),
				stringConstraint("enable_control_polarity", "equal", test.enablePolarity),
				stringConstraint("enable_control_startup_state", "equal", "deasserted"),
				stringConstraint("fault_control_function", "equal", "fault"),
				stringConstraint("fault_control_polarity", "equal", test.faultPolarity),
				stringConstraint("fault_control_startup_state", "equal", "deasserted"),
				stringConstraint("fault_control_safe_state", "equal", "asserted"),
				stringConstraint("permit_control_function", "equal", "enable"),
				stringConstraint("permit_control_polarity", "equal", test.permitPolarity),
				stringConstraint("permit_control_startup_state", "equal", "deasserted"),
				stringConstraint("permit_control_safe_state", "equal", "deasserted"),
				boolConstraint("safety_dominance", "required"),
				boolConstraint("default_off", "required"),
			}}
			expansions, err := provider.Expand(context.Background(), request)
			if err != nil || len(expansions) == 0 {
				t.Fatalf("expansions=%d err=%v", len(expansions), err)
			}
			if expansions[0].ID != "enable_protection_dominant_interlock" {
				t.Fatalf("expansion id = %q", expansions[0].ID)
			}
			realization, err := DecodeFragmentRealization(expansions[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			roles := map[string]bool{}
			for _, binding := range realization.PortBindings {
				roles[binding.Role] = true
			}
			for _, role := range []string{"enable", "fault", "permit", "power", "reference"} {
				if !roles[role] {
					t.Fatalf("missing role %s: %#v", role, realization.PortBindings)
				}
			}
			instances := map[string]bool{}
			for _, instance := range realization.Instances {
				instances[instance.ID] = true
			}
			requiredInstances := []string{"enable_inverter", "enable_block_clamp", "fault_dominance_clamp", "permit_pullup"}
			if test.enablePolarity == "active_high" && test.faultPolarity == "active_high" && test.permitPolarity == "active_low" {
				requiredInstances = []string{"permit_sink", "fault_dominance_clamp", "permit_pullup"}
			}
			for _, required := range requiredInstances {
				if !instances[required] {
					t.Fatalf("missing dominance instance %s", required)
				}
			}
			if got := instances["enable_polarity_inverter"]; got != (test.enablePolarity == "active_low") {
				t.Fatalf("enable normalization present=%t", got)
			}
			if got := instances["fault_polarity_inverter"]; got != (test.faultPolarity == "active_low") {
				t.Fatalf("fault normalization present=%t", got)
			}
			wantPermitInverter := test.permitPolarity == "active_low" && !(test.enablePolarity == "active_high" && test.faultPolarity == "active_high")
			if got := instances["permit_polarity_inverter"]; got != wantPermitInverter {
				t.Fatalf("permit normalization present=%t", got)
			}
			if len(realization.RepairVariables) != 1 || realization.RepairVariables[0].Instance != "permit_pullup" {
				t.Fatalf("repair variables = %#v", realization.RepairVariables)
			}
		})
	}
}

func TestCatalogProviderControlledDisconnectFuseMeetsOutputDropBudget(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "output_protection", Ports: []RoleContract{
		providerRole("control", "digital_logic", "sink", 0, 3.3),
		providerRole("input", "power", "sink", 3.2, 3.4),
		providerRole("output", "power", "source", 3.2, 3.4),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("dc_voltage", "target", 3.3, "V", 3.030303),
		constraintNumber("overcurrent_limit", "maximum", 0.6, "A", 0),
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
		return instance.ID == "output_disconnect" && instance.CatalogID == "mosfet.aos.ao3401.sot23"
	}) {
		t.Fatalf("3.3 V controlled output did not select a gate-compatible low-resistance disconnect: %#v", realization.Instances)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.ID == "output_fuse" && instance.CatalogID == "fuse.littelfuse.0437003wr.1206"
	}) {
		t.Fatalf("3.3 V controlled output did not select a voltage-drop-qualified fuse: %#v", realization.Instances)
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

func TestCatalogProviderSizesAutomaticMuteRelayFromBehavioralLoadCurrent(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "mute_control", Ports: []RoleContract{
		providerRole("protected", "protected_output", "sink", -19.8, 19.8),
		providerRole("power", "power", "sink", 16.2, 19.8),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintNumber("continuous_output_power", "minimum", 15, "W", 0),
		constraintNumber("load_impedance", "target", 4, "ohm", 0),
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
		return instance.ID == "mute_relay" && instance.CatalogID == "relay.omron.g5q_1a.dc12"
	}) {
		t.Fatalf("15 W / 4 ohm automatic mute did not select a peak-current-qualified relay: %#v", realization.Instances)
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

func pushPullTranslatorProviderRequest(sideA, sideB float64, channels int, frequency float64) ProviderRequest {
	busA := providerRole("side_a", "digital_bus", "source", 0, sideA)
	busB := providerRole("side_b", "digital_bus", "sink", 0, sideB)
	busA.Contract.Protocol = &Protocol{Name: "parallel", Mode: "push_pull", MaxFrequencyHz: frequency}
	busB.Contract.Protocol = &Protocol{Name: "parallel", Mode: "push_pull", MaxFrequencyHz: frequency}
	enable := providerRole("enable", "digital_logic", "sink", 0, sideA)
	enable.Contract.DefaultState = "inactive"
	return ProviderRequest{Capability: "logic_level_translation", Ports: []RoleContract{
		busA, busB,
		providerRole("power_a", "power", "sink", sideA, sideA),
		providerRole("power_b", "power", "sink", sideB, sideB),
		providerRole("reference", "reference", "bidirectional", 0, 0),
		enable,
	}, Constraints: []Constraint{
		constraintNumber("channel_count", "minimum", float64(channels), "count", 0),
	}}
}

func directionControlledTranslatorProviderRequest(sideA, sideB float64, channels int, frequency float64) ProviderRequest {
	request := pushPullTranslatorProviderRequest(sideA, sideB, channels, frequency)
	for index := range request.Ports {
		if request.Ports[index].Role == "side_a" || request.Ports[index].Role == "side_b" {
			request.Ports[index].Contract.Direction = "bidirectional"
		}
	}
	directionControl := providerRole("direction_control", "digital_logic", "sink", 0, sideA)
	directionControl.Contract.DefaultState = "inactive"
	request.Ports = append(request.Ports, directionControl)
	request.Constraints = append(request.Constraints,
		constraintString("direction", "equal", "bidirectional"),
		constraintString("direction_change_state", "equal", "disabled"),
	)
	return request
}

func pushPullFunctionalIsolationProviderRequest(sideA, sideB float64, forwardChannels, reverseChannels int, frequency float64) ProviderRequest {
	forwardA := providerRole("side_a_forward", "digital_bus", "source", 0, sideA)
	forwardB := providerRole("side_b_forward", "digital_bus", "sink", 0, sideB)
	reverseB := providerRole("side_b_reverse", "digital_bus", "source", 0, sideB)
	reverseA := providerRole("side_a_reverse", "digital_bus", "sink", 0, sideA)
	forwardA.Contract.Protocol = &Protocol{Name: "parallel", Mode: "push_pull", MaxFrequencyHz: frequency}
	forwardB.Contract.Protocol = &Protocol{Name: "parallel", Mode: "push_pull", MaxFrequencyHz: frequency}
	reverseB.Contract.Protocol = &Protocol{Name: "fault", Mode: "push_pull", MaxFrequencyHz: frequency}
	reverseA.Contract.Protocol = &Protocol{Name: "fault", Mode: "push_pull", MaxFrequencyHz: frequency}
	return ProviderRequest{Capability: "galvanic_isolation", Ports: []RoleContract{
		forwardA, forwardB, reverseB, reverseA,
		providerRole("power_a", "power", "sink", sideA, sideA),
		providerRole("reference_a", "reference", "bidirectional", 0, 0),
		providerRole("power_b", "power", "sink", sideB, sideB),
		providerRole("reference_b", "reference", "bidirectional", 0, 0),
	}, Constraints: []Constraint{
		constraintString("signaling_mode", "equal", "push_pull"),
		constraintNumber("forward_channel_count", "minimum", float64(forwardChannels), "count", 0),
		constraintNumber("reverse_channel_count", "minimum", float64(reverseChannels), "count", 0),
		constraintNumber("isolation_working_voltage", "minimum", 1000, "V", 0),
		constraintNumber("isolation_transient_voltage", "minimum", 5000, "V", 0),
		constraintNumber("minimum_clearance", "minimum", 6, "mm", 0),
		constraintNumber("minimum_creepage", "minimum", 6, "mm", 0),
		constraintString("supply_loss_safe_state", "equal", "low"),
		constraintBool("independent_startup", "required", true),
	}}
}

func countRealizationUsage(realization FragmentRealization, usage string) int {
	count := 0
	for _, instance := range realization.Instances {
		if instance.Usage == usage {
			count++
		}
	}
	return count
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
