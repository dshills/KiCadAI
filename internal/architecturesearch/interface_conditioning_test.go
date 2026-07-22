package architecturesearch

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
)

func TestCatalogProviderSynthesizesSourceTerminationDeterministically(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "signal_termination", Ports: []RoleContract{
		providerRole("input", "digital", "sink", 0, 3.3),
		providerRole("output", "digital", "source", 0, 3.3),
	}, Constraints: []Constraint{
		constraintNumber("driver_output_impedance", "target", 10, "Ohm", 0),
		constraintNumber("interconnect_target_impedance", "target", 50, "Ohm", 10),
	}}
	first, err := provider.Expand(context.Background(), request)
	if err != nil || len(first) == 0 {
		t.Fatalf("termination expansion = %#v err=%v", first, err)
	}
	second, err := provider.Expand(context.Background(), request)
	if err != nil || len(second) != len(first) || string(second[0].Payload) != string(first[0].Payload) {
		t.Fatalf("termination replay changed: first=%#v second=%#v err=%v", first, second, err)
	}
	realization, err := DecodeFragmentRealization(first[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(realization.Instances) != 1 || realization.Instances[0].Value != "39" || len(first[0].Calculations) != 1 || !first[0].Calculations[0].Pass {
		t.Fatalf("unexpected termination realization: %#v", realization)
	}
}

func TestCatalogProviderProvesCompleteClockInterfaceDeterministically(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := clockConditioningRequest()
	first, err := provider.Expand(context.Background(), request)
	if err != nil || len(first) == 0 {
		t.Fatalf("clock expansion = %#v err=%v", first, err)
	}
	shuffled := request
	shuffled.Ports = slices.Clone(request.Ports)
	shuffled.Constraints = slices.Clone(request.Constraints)
	slices.Reverse(shuffled.Ports)
	slices.Reverse(shuffled.Constraints)
	second, err := provider.Expand(context.Background(), shuffled)
	if err != nil || len(second) != len(first) || string(second[0].Payload) != string(first[0].Payload) {
		t.Fatalf("shuffled clock expansion changed: first=%#v second=%#v err=%v", first, second, err)
	}
	if !slices.ContainsFunc(first[0].Calculations, func(calculation CalculationEvidence) bool {
		return calculation.ID == "clock_interface_compatibility" && calculation.Pass
	}) {
		t.Fatalf("clock calculations = %#v", first[0].Calculations)
	}
}

func TestCatalogProviderClockConditioningFailsClosedWithoutJitterEvidence(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := clockConditioningRequest()
	request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool {
		return constraint.Name == "clock_rms_jitter"
	})
	_, err = provider.Expand(context.Background(), request)
	var typed *interfaceSynthesisError
	if !errors.As(err, &typed) || typed.code != CodeInterfaceClockConditionUnproven ||
		typed.message != "clock conditioning requires amplitude, common-mode, edge, frequency, jitter, startup, fanout, and loading evidence" {
		t.Fatalf("missing jitter error = %#v", err)
	}
}

func TestCatalogProviderClockConditioningChecksEdgeCurrent(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := clockConditioningRequest()
	for index := range request.Constraints {
		if request.Constraints[index].Name == "source_maximum_current" {
			request.Constraints[index] = constraintNumber("source_maximum_current", "maximum", 10e-3, "A", 0)
		}
	}
	_, err = provider.Expand(context.Background(), request)
	var typed *interfaceSynthesisError
	if !errors.As(err, &typed) || typed.code != CodeInterfaceClockConditionUnproven ||
		typed.message != "clock source and receiver electrical bounds are incompatible at the requested operating point" {
		t.Fatalf("edge-current error = %#v", err)
	}
}

func TestCatalogProviderSynthesizesPassiveADCDriveAndFailsClosed(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "adc_drive_conditioning", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 0, 3.3),
		providerRole("output", "analog_voltage", "source", 0, 3.3),
	}, Constraints: []Constraint{
		constraintNumber("source_impedance", "maximum", 100, "Ohm", 0),
		constraintNumber("adc_input_capacitance", "target", 20e-12, "F", 0),
		constraintNumber("acquisition_time", "minimum", 2e-6, "s", 0),
		constraintNumber("settling_accuracy", "maximum", 1e-4, "ratio", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 {
		t.Fatalf("ADC drive expansion = %#v err=%v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil || len(expansions[0].Calculations) != 1 || !expansions[0].Calculations[0].Pass {
		t.Fatalf("ADC drive realization = %#v err=%v", realization, err)
	}

	request.Constraints[2] = constraintNumber("acquisition_time", "minimum", 1e-12, "s", 0)
	_, err = provider.Expand(context.Background(), request)
	var typed *interfaceSynthesisError
	if !errors.As(err, &typed) || typed.code != CodeInterfaceADCDriveUnproven {
		t.Fatalf("impossible ADC drive error = %#v", err)
	}
}

func TestCatalogProviderSelectsProvenBufferedADCDrive(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "adc_drive_conditioning", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 2.5, 2.5),
		providerRole("output", "analog_voltage", "source", 2.5, 2.5),
		providerRole("power", "power", "sink", 5, 5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}, Constraints: bufferedADCConstraints()}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 || expansions[0].ID != "buffered_adc_drive_conditioning" {
		t.Fatalf("buffered ADC expansion = %#v err=%v", expansions, err)
	}
	realization, err := DecodeFragmentRealization(expansions[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(realization.Instances, func(instance RealizationInstance) bool { return instance.ID == "adc_buffer" }) ||
		!slices.ContainsFunc(expansions[0].Calculations, func(calculation CalculationEvidence) bool {
			return calculation.ID == "buffered_adc_drive_settling" && calculation.Pass
		}) {
		t.Fatalf("buffered ADC realization = %#v calculations=%#v", realization, expansions[0].Calculations)
	}
}

func TestBufferedADCDriveRejectsInvalidSettlingAccuracyBeforeLogarithm(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "adc_drive_conditioning", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 0, 3.3),
		providerRole("output", "analog_voltage", "source", 0, 3.3),
		providerRole("power", "power", "sink", 0, 5),
		providerRole("reference", "reference", "bidirectional", 0, 0),
	}}
	for _, accuracy := range []float64{0, 1, math.NaN(), math.Inf(1)} {
		_, err := provider.expandBufferedADCDrive(context.Background(), request, 100, 20e-12, 1e-6, accuracy)
		var typed *interfaceSynthesisError
		if !errors.As(err, &typed) || typed.code != CodeInterfaceADCDriveUnproven {
			t.Fatalf("accuracy %v error = %#v", accuracy, err)
		}
	}
}

func TestCatalogProviderMeasuresBufferedADCHeadroomFromReferenceRail(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{Capability: "adc_drive_conditioning", Ports: []RoleContract{
		providerRole("input", "analog_voltage", "sink", 4.5, 4.5),
		providerRole("output", "analog_voltage", "source", 4.5, 4.5),
		providerRole("power", "power", "sink", 7, 7),
		providerRole("reference", "reference", "bidirectional", 2, 2),
	}, Constraints: bufferedADCConstraints()}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 || expansions[0].ID != "buffered_adc_drive_conditioning" {
		t.Fatalf("reference-relative buffered ADC expansion = %#v err=%v", expansions, err)
	}
}

func clockConditioningRequest() ProviderRequest {
	return ProviderRequest{Capability: "clock_conditioning", Ports: []RoleContract{
		providerRole("input", "clock", "sink", 0, 3.3),
		providerRole("output", "clock", "source", 0, 3.3),
	}, Constraints: []Constraint{
		constraintNumber("clock_frequency", "target", 16e6, "Hz", 0),
		constraintNumber("clock_amplitude", "minimum", 3, "V", 0),
		constraintNumber("clock_common_mode", "target", 1.65, "V", 0),
		constraintNumber("clock_edge_time", "maximum", 2e-9, "s", 0),
		constraintNumber("receiver_low_threshold", "maximum", .8, "V", 0),
		constraintNumber("receiver_high_threshold", "minimum", 2, "V", 0),
		constraintNumber("receiver_maximum_edge_time", "maximum", 5e-9, "s", 0),
		constraintNumber("clock_rms_jitter", "maximum", 5e-12, "s", 0),
		constraintNumber("receiver_maximum_rms_jitter", "maximum", 20e-12, "s", 0),
		constraintNumber("clock_startup_time", "maximum", 2e-3, "s", 0),
		constraintNumber("maximum_clock_startup_time", "maximum", 10e-3, "s", 0),
		constraintNumber("clock_fanout", "target", 2, "count", 0),
		constraintNumber("receiver_input_capacitance", "maximum", 5e-12, "F", 0),
		constraintNumber("source_maximum_capacitive_load", "maximum", 20e-12, "F", 0),
		constraintNumber("source_maximum_current", "maximum", 20e-3, "A", 0),
		constraintNumber("source_maximum_frequency", "maximum", 25e6, "Hz", 0),
		constraintNumber("receiver_maximum_frequency", "maximum", 20e6, "Hz", 0),
		constraintNumber("driver_output_impedance", "target", 15, "Ohm", 0),
		constraintNumber("interconnect_target_impedance", "target", 50, "Ohm", 10),
		constraintString("clock_signaling_mode", "equal", "lvcmos"),
		constraintString("receiver_signaling_mode", "equal", "lvcmos"),
	}}
}

func bufferedADCConstraints() []Constraint {
	return []Constraint{
		constraintNumber("source_impedance", "maximum", 1e6, "Ohm", 0),
		constraintNumber("adc_input_capacitance", "target", 20e-12, "F", 0),
		constraintNumber("acquisition_time", "minimum", 2e-6, "s", 0),
		constraintNumber("settling_accuracy", "maximum", 1e-4, "ratio", 0),
		constraintNumber("maximum_input_step", "maximum", 3, "V", 0),
		constraintNumber("noise_bandwidth", "maximum", 100e3, "Hz", 0),
		constraintNumber("maximum_integrated_noise", "maximum", 10e-6, "V", 0),
		constraintNumber("maximum_output_load_current", "maximum", 1e-3, "A", 0),
		constraintNumber("ambient_temperature", "maximum", 70, "degC", 0),
		constraintNumber("minimum_thermal_margin", "minimum", 20, "degC", 0),
	}
}
