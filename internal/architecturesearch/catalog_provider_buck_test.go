package architecturesearch

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/simmodel"
)

func TestCatalogProviderSynchronousBuckIsCatalogOrderDeterministicAndPublishesMargins(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	forward, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := NewCatalogProvider(reversedArchitectureCatalog(catalog))
	if err != nil {
		t.Fatal(err)
	}
	request := synchronousBuckProviderRequest(2)
	first, firstErr := forward.Expand(context.Background(), request)
	second, secondErr := reversed.Expand(context.Background(), request)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("forward err=%v reversed err=%v", firstErr, secondErr)
	}
	if !reflect.DeepEqual(first, second) || len(first) == 0 {
		firstJSON, _ := json.Marshal(first)
		secondJSON, _ := json.Marshal(second)
		t.Fatalf("catalog order changed synchronous-buck expansion\nforward=%s\nreversed=%s", firstJSON, secondJSON)
	}

	realization, err := DecodeFragmentRealization(first[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	usage := map[string]RealizationInstance{}
	for _, instance := range realization.Instances {
		usage[instance.Usage] = instance
		if strings.Contains(instance.Usage, "linear_regulator") {
			t.Fatalf("efficient conversion fell through to a linear regulator: %#v", realization.Instances)
		}
	}
	if usage["synchronous_buck_controller"].CatalogID != "regulator.analog_devices.lt8610mse.msop16" ||
		usage["switching_inductor"].CatalogID == "" ||
		usage["low_esr_output_capacitor"].CatalogID == "" {
		t.Fatalf("synchronous-buck realization = %#v", realization.Instances)
	}
	if len(realization.RepairVariables) != 1 ||
		realization.RepairVariables[0].ID != "buck_feedback_top_resistance" ||
		realization.RepairVariables[0].Kind != "bias" ||
		realization.RepairVariables[0].Instance != "buck_feedback_top" ||
		len(realization.RepairVariables[0].AllowedValues) < 2 ||
		realization.RepairVariables[0].Effects[0] != (RealizationRepairEffect{
			Analysis: "dc_operating_point", Metric: "dc_voltage", Direction: "metric_increases",
		}) {
		t.Fatalf("synchronous-buck bounded feedback repair = %#v", realization.RepairVariables)
	}
	feedbackTop := realization.RepairVariables[0].Value
	if feedbackTop <= 40_251 {
		t.Fatalf("synchronous-buck divider did not allocate bounded upper-band headroom: top=%.12g", feedbackTop)
	}
	var converterRecord components.ComponentRecord
	for _, record := range catalog.Records {
		if record.ID == usage["synchronous_buck_controller"].CatalogID {
			converterRecord = record
			break
		}
	}
	feedbackReferenceV, referenceOK := catalogSimulationParameter(converterRecord, "reference_voltage_v")
	if !referenceOK {
		t.Fatalf("selected synchronous buck lacks feedback reference: %#v", converterRecord)
	}
	_, referenceMaximumV := catalogUncertaintyInterval(
		converterRecord, "model_parameters.reference_voltage_v", feedbackReferenceV,
	)
	const feedbackTolerance = .001
	worstHighOutput := referenceMaximumV * (1 + feedbackTop*(1+feedbackTolerance)/(buckFeedbackBottomOhm*(1-feedbackTolerance)))
	if worstHighOutput > 5.15 || worstHighOutput < 5.10 {
		t.Fatalf("synchronous-buck upper-corner allocation = %.12g V", worstHighOutput)
	}
	requiredOutputs := map[string]bool{
		"conversion_efficiency": false, "peak_inductor_current": false,
		"phase_margin": false, "gain_margin": false,
		"peak_junction_temperature": false, "transient_soa_margin": false,
	}
	for _, calculation := range first[0].Calculations {
		for _, output := range calculation.NominalOutputs {
			if _, ok := requiredOutputs[output.Name]; ok {
				requiredOutputs[output.Name] = true
			}
		}
	}
	for output, present := range requiredOutputs {
		if !present {
			t.Fatalf("synchronous-buck expansion lacks %s evidence: %#v", output, first[0].Calculations)
		}
	}
	for _, port := range first[0].OfferedPorts {
		switch port.Role {
		case "input":
			if port.Contract.CurrentDemandA == nil || *port.Contract.CurrentDemandA <= 0 || *port.Contract.CurrentDemandA > 1.5 {
				t.Fatalf("synchronous-buck input demand = %#v", port.Contract.CurrentDemandA)
			}
		case "output":
			if port.Contract.CurrentCapacityA == nil || *port.Contract.CurrentCapacityA != 2.5 {
				t.Fatalf("synchronous-buck output capacity = %#v", port.Contract.CurrentCapacityA)
			}
		}
	}
}

func TestCatalogProviderSynchronousBuckFailsClosedWithoutReviewedInductor(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	filtered := &components.Catalog{
		Version: catalog.Version, GeneratedAt: catalog.GeneratedAt,
		Families: append([]components.FamilyDefinition(nil), catalog.Families...),
	}
	for _, record := range catalog.Records {
		if record.Family != "inductor" {
			filtered.Records = append(filtered.Records, record)
		}
	}
	components.RebuildCatalogIndexes(filtered)
	provider, err := NewCatalogProvider(filtered)
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), synchronousBuckProviderRequest(2))
	if err == nil || len(expansions) != 0 || !strings.Contains(err.Error(), "inductor") {
		t.Fatalf("missing reviewed inductor expansions=%#v err=%v", expansions, err)
	}
}

func TestCatalogProviderSynchronousBuckRanksReviewedEfficiencyLowerBound(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	for recordIndex := range catalog.Records {
		record := &catalog.Records[recordIndex]
		if record.ID != "regulator.analog_devices.lt8610mse.msop16" {
			continue
		}
		for modelIndex := range record.SimulationModels {
			model := &record.SimulationModels[modelIndex]
			if model.ModelID != "mna_synchronous_buck_current_mode_v1" {
				continue
			}
			for uncertaintyIndex := range model.Uncertainties {
				uncertainty := &model.Uncertainties[uncertaintyIndex]
				if uncertainty.Target == "model_parameters.conversion_efficiency_fraction" {
					uncertainty.Minimum = .80
				}
			}
		}
	}
	components.RebuildCatalogIndexes(catalog)
	provider, err := NewCatalogProvider(catalog)
	if err != nil {
		t.Fatal(err)
	}
	expansions, err := provider.Expand(context.Background(), synchronousBuckProviderRequest(2))
	if err != nil || len(expansions) == 0 {
		t.Fatalf("lower-bound-ranked buck expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool {
		return instance.Usage == "synchronous_buck_controller" &&
			instance.CatalogID == "regulator.ti.lm76002rnp.wqfn30"
	}) {
		t.Fatalf("lower reviewed LT8610 bound did not select the stronger LM76002 alternative: %#v", realization.Instances)
	}
}

func TestBuckArchitectureCalculationsRejectNominalPassWhenReviewedMinimumFails(t *testing.T) {
	catalog := loadArchitectureCatalog(t)
	var record components.ComponentRecord
	for _, candidate := range catalog.Records {
		if candidate.ID == "regulator.analog_devices.lt8610mse.msop16" {
			record = candidate
			break
		}
	}
	for modelIndex := range record.SimulationModels {
		for uncertaintyIndex := range record.SimulationModels[modelIndex].Uncertainties {
			uncertainty := &record.SimulationModels[modelIndex].Uncertainties[uncertaintyIndex]
			if uncertainty.Target == "model_parameters.conversion_efficiency_fraction" {
				uncertainty.Minimum = .81
			}
		}
	}
	request := synchronousBuckProviderRequest(2)
	request.Constraints = append(request.Constraints,
		constraintNumber("conversion_efficiency", "minimum", 82, "%", 0),
	)
	_, err := buckArchitectureCalculations(request, record, 18, 30, 5, 2, .6, 2.3, 220e-6, .056, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "minimum efficiency 81%") {
		t.Fatalf("nominal-pass/lower-bound-fail calculations err=%v", err)
	}
}

func TestCatalogProviderSizesDCNeutralBuckFeedbackFilterFromStabilityBounds(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := synchronousBuckProviderRequest(2)
	request.Constraints = append(request.Constraints,
		constraintNumber("loop_crossover_frequency", "target", 50_500, "Hz", 98.01980198019803),
		constraintNumber("phase_margin", "minimum", 50, "deg", 0),
	)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("stability-bounded buck expansions=%d err=%v", len(expansions), err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	var filterInstance *RealizationInstance
	for index := range realization.Instances {
		if realization.Instances[index].ID == "buck_feedback_filter" {
			filterInstance = &realization.Instances[index]
			break
		}
	}
	if filterInstance == nil || filterInstance.Value != "1n" {
		t.Fatalf("requirement-derived buck feedback filter = %#v", filterInstance)
	}
	hasFeedbackEndpoint, hasReferenceEndpoint := false, false
	for _, connection := range realization.Connections {
		for _, endpoint := range connection.Endpoints {
			if endpoint.Instance != "buck_feedback_filter" {
				continue
			}
			hasFeedbackEndpoint = hasFeedbackEndpoint || connection.ID == "buck_feedback" && endpoint.Function == "A"
			hasReferenceEndpoint = hasReferenceEndpoint || connection.ID == "buck_reference" && endpoint.Function == "B"
		}
	}
	if !hasFeedbackEndpoint || !hasReferenceEndpoint {
		t.Fatalf("feedback filter is not parallel with the lower divider: %#v", realization.Connections)
	}
	if !slices.ContainsFunc(realization.RepairVariables, func(variable RealizationRepairVariable) bool {
		return variable.ID == "buck_feedback_filter_capacitance" &&
			variable.Kind == "compensation" &&
			variable.Instance == "buck_feedback_filter" &&
			slices.Contains(variable.Effects, RealizationRepairEffect{
				Analysis: "stability", Metric: "loop_crossover_frequency", Direction: "metric_decreases",
			})
	}) {
		t.Fatalf("feedback-filter repair variable = %#v", realization.RepairVariables)
	}
}

func TestPrecisionResistanceRepairValuesIncludeNearestE192Neighbors(t *testing.T) {
	const nominal = 41816.75012547
	values := precisionResistanceRepairValues(nominal)
	for _, expected := range []float64{41700, nominal * 1.0005, nominal * 1.001} {
		if !slices.ContainsFunc(values, func(value float64) bool {
			return math.Abs(value-expected) <= math.Abs(expected)*1e-12
		}) {
			t.Fatalf("precision repair values %v lack fine neighbor %.12g", values, expected)
		}
	}
	if !slices.IsSorted(values) {
		t.Fatalf("precision repair values are not canonical: %v", values)
	}
}

func TestCatalogProviderUsesLoopModeledBuckForStabilityRequiredVoltageRegulation(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		outputV, currentA float64
	}{{5, .5}, {5, 1}, {3.3, .6}} {
		request := ProviderRequest{
			Capability: "voltage_regulation",
			Ports: []RoleContract{
				providerRole("input", "power", "sink", 10.8, 13.2),
				providerRole("output", "power", "source", tc.outputV*.97, tc.outputV*1.03),
				providerRole("reference", "reference", "bidirectional", 0, 0),
			},
			Constraints: []Constraint{
				constraintBool("adjustable_output", "required", true),
				constraintString("set_point_programming", "equal", "passive_feedback"),
				constraintBool("input_decoupling", "required", true),
				constraintBool("output_decoupling", "required", true),
				constraintRange("input_voltage", "range", 10.8, 13.2, "V"),
				constraintNumber("output_voltage", "target", tc.outputV, "V", 3),
				constraintNumber("continuous_output_current", "minimum", tc.currentA, "A", 0),
				constraintBool("analysis_stability", "required", true),
			},
		}
		if _, err := provider.expandSynchronousBuckConversion(context.Background(), request); err != nil {
			t.Fatalf("%.3g V %.3g A direct stability-required buck expansion: %v", tc.outputV, tc.currentA, err)
		}
		expansions, err := provider.Expand(context.Background(), request)
		if err != nil || len(expansions) == 0 {
			t.Fatalf("%.3g V %.3g A stability-required regulator expansions=%d err=%v", tc.outputV, tc.currentA, len(expansions), err)
		}
		if expansions[0].ID != "catalog_synchronous_buck_current_mode" {
			t.Fatalf("%.3g V %.3g A stability-required regulator selected %q", tc.outputV, tc.currentA, expansions[0].ID)
		}
	}
}

func TestBuckDefersEventScopedSOAMarginToComposedDynamicEvaluation(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := synchronousBuckProviderRequest(.5)
	request.Constraints = append(request.Constraints,
		constraintNumber("transient_soa_margin", "minimum", 1.2, "ratio", 0),
		constraintNumber("transient_soa_duration", "target", .05, "s", 0),
		constraintNumber("short_circuit_transient_soa_margin", "minimum", 1.2, "ratio", 0),
	)
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("event-scoped composed SOA evaluation was rejected locally: expansions=%d err=%v", len(expansions), err)
	}
}

func TestBuckRejectsUnscopedInsufficientSOAMarginLocally(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := synchronousBuckProviderRequest(.5)
	request.Constraints = append(request.Constraints,
		constraintNumber("transient_soa_margin", "minimum", 1.2, "ratio", 0),
		constraintNumber("transient_soa_duration", "target", .05, "s", 0),
	)
	if _, err := provider.Expand(context.Background(), request); err == nil || !strings.Contains(err.Error(), "transient SOA margin") {
		t.Fatalf("unscoped insufficient SOA margin did not fail closed: %v", err)
	}
}

func TestCatalogTransientSOAInterpolationRejectsInvalidBoundaries(t *testing.T) {
	duration := 1e-3
	record := components.ComponentRecord{SimulationModels: []simmodel.CatalogEvidence{{
		ModelID: simmodel.PrimitiveSynchronousBuckRegulatorV1,
		TransientSOA: []simmodel.TransientSOAEnvelope{{
			PulseDurationS: &duration,
			Points: []simmodel.TransientSOAPoint{
				{VoltageV: 1, CurrentA: 2},
				{VoltageV: 1, CurrentA: 1},
			},
		}},
	}}}
	if current, ok := catalogTransientSOACurrent(record, duration, math.NaN()); ok || current != 0 {
		t.Fatalf("invalid SOA interpolation boundary returned current=%g ok=%t", current, ok)
	}
}

func synchronousBuckProviderRequest(outputCurrentA float64) ProviderRequest {
	input := providerRole("input", "power", "sink", 18, 30)
	input.Contract.MaximumCurrentDemandA = float64Pointer(1.5)
	output := providerRole("output", "power", "source", 4.85, 5.15)
	output.Contract.RequiredCurrentCapacityA = float64Pointer(outputCurrentA)
	return ProviderRequest{
		Capability: "efficient_voltage_conversion",
		Ports: []RoleContract{
			input,
			output,
			providerRole("reference", "reference", "bidirectional", 0, 0),
		},
		Constraints: []Constraint{
			constraintNumber("output_voltage", "target", 5, "V", 3),
			constraintNumber("continuous_output_current", "minimum", outputCurrentA, "A", 0),
			constraintRange("input_supply_voltage", "within", 18, 30, "V"),
		},
	}
}
