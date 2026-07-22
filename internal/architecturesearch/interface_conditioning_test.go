package architecturesearch

import (
	"context"
	"errors"
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
	}, Constraints: []Constraint{
		constraintNumber("source_impedance", "maximum", 1e6, "Ohm", 0),
		constraintNumber("adc_input_capacitance", "target", 20e-12, "F", 0),
		constraintNumber("acquisition_time", "minimum", 2e-6, "s", 0),
		constraintNumber("settling_accuracy", "maximum", 1e-4, "ratio", 0),
	}}
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
	}, Constraints: []Constraint{
		constraintNumber("source_impedance", "maximum", 1e6, "Ohm", 0),
		constraintNumber("adc_input_capacitance", "target", 20e-12, "F", 0),
		constraintNumber("acquisition_time", "minimum", 2e-6, "s", 0),
		constraintNumber("settling_accuracy", "maximum", 1e-4, "ratio", 0),
	}}
	expansions, err := provider.Expand(context.Background(), request)
	if err != nil || len(expansions) == 0 || expansions[0].ID != "buffered_adc_drive_conditioning" {
		t.Fatalf("reference-relative buffered ADC expansion = %#v err=%v", expansions, err)
	}
}
