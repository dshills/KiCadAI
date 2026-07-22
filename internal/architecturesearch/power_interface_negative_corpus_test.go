package architecturesearch

import (
	"context"
	"errors"
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

// TestPowerInterfaceNegativeRequestCorpus is the failure-driven companion to
// the physical promotion corpus. Every case is topology-neutral and must emit
// the same behavioral code and diagnostic after semantically irrelevant input
// reordering.
func TestPowerInterfaceNegativeRequestCorpus(t *testing.T) {
	provider, err := NewCatalogProvider(loadArchitectureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		code reports.Code
		run  func(bool) (reports.Code, string)
	}{
		{name: "missing_rail_source", code: CodePowerRailSourceMissing, run: func(reversed bool) (reports.Code, string) {
			requirement := powerTreeRequirement(false)
			if reversed {
				slices.Reverse(requirement.Requirements.Domains)
				slices.Reverse(requirement.Requirements.Objectives)
			}
			_, rejection := validatePowerTreeTopology(requirement, nil)
			return rejection.Code, rejection.Message
		}},
		{name: "rail_cycle", code: CodePowerRailCycle, run: func(reversed bool) (reports.Code, string) {
			requirement, selections := powerTreeRequirement(true), powerTreeSelections()
			if reversed {
				slices.Reverse(requirement.Requirements.Domains)
				slices.Reverse(requirement.Requirements.Objectives)
				slices.Reverse(selections)
			}
			_, rejection := validatePowerTreeTopology(requirement, selections)
			return rejection.Code, rejection.Message
		}},
		{name: "capacitor_stability", code: CodePowerCapacitorStabilityUnproven, run: func(reversed bool) (reports.Code, string) {
			request := transientNegativeRequest()
			if reversed {
				slices.Reverse(request.Constraints)
			}
			_, _, err := regulatorOutputCapacitor(request, components.ComponentRecord{})
			return synthesisErrorCode(err)
		}},
		{name: "transient_capacitance", code: CodePowerTransientCapacitanceUnavailable, run: func(reversed bool) (reports.Code, string) {
			request := transientNegativeRequest()
			if reversed {
				slices.Reverse(request.Constraints)
			}
			_, _, err := regulatorOutputCapacitor(request, regulatorWithCapWindow("1u", "1m"))
			return synthesisErrorCode(err)
		}},
		{name: "sequence", code: CodePowerSequenceUnproven, run: func(reversed bool) (reports.Code, string) {
			requirement, selections := powerTreeRequirement(false), powerSequenceSelections(t, .003, .002)
			if reversed {
				slices.Reverse(requirement.Requirements.Domains)
				slices.Reverse(requirement.Requirements.Objectives)
				slices.Reverse(selections)
			}
			constraint := Constraint{Name: "rail_sequence_before", Relation: "required", Value: []byte(`["rail_a_signal","rail_b_signal"]`)}
			_, rejection := validatePowerSequenceConstraint(requirement, selections, constraint, "sequence")
			return rejection.Code, rejection.Message
		}},
		{name: "translation_domain", code: CodeInterfaceVoltageDomainMismatch, run: providerNegativeCase(provider, func() ProviderRequest {
			return translatorProviderRequest(3.3, 0)
		})},
		{name: "pullup_window", code: CodeInterfacePullupWindowEmpty, run: providerNegativeCase(provider, func() ProviderRequest {
			request := translatorProviderRequest(3.3, 5)
			request.Constraints = append(request.Constraints,
				constraintNumber("rise_time", "maximum", 1e-12, "s", 0),
				constraintNumber("load_capacitance", "maximum", 1e-6, "F", 0))
			return request
		})},
		{name: "termination", code: CodeInterfaceTerminationUnproven, run: providerNegativeCase(provider, func() ProviderRequest {
			return ProviderRequest{Capability: "signal_termination", Constraints: []Constraint{
				constraintNumber("driver_output_impedance", "target", 10, "Ohm", 0),
			}}
		})},
		{name: "clock", code: CodeInterfaceClockConditionUnproven, run: providerNegativeCase(provider, func() ProviderRequest {
			request := clockConditioningRequest()
			request.Constraints = slices.DeleteFunc(request.Constraints, func(constraint Constraint) bool { return constraint.Name == "clock_rms_jitter" })
			return request
		})},
		{name: "adc_settling", code: CodeInterfaceADCDriveUnproven, run: providerNegativeCase(provider, func() ProviderRequest {
			return ProviderRequest{Capability: "adc_drive_conditioning", Constraints: []Constraint{
				constraintNumber("source_impedance", "maximum", 100, "Ohm", 0),
				constraintNumber("adc_input_capacitance", "target", 20e-12, "F", 0),
				constraintNumber("acquisition_time", "minimum", 1e-12, "s", 0),
				constraintNumber("settling_accuracy", "maximum", 1e-4, "ratio", 0),
			}}
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstCode, firstMessage := test.run(false)
			secondCode, secondMessage := test.run(true)
			if firstCode != test.code || secondCode != test.code || firstMessage == "" || firstMessage != secondMessage {
				t.Fatalf("negative replay = (%s, %q), (%s, %q); want stable %s", firstCode, firstMessage, secondCode, secondMessage, test.code)
			}
		})
	}
}

func transientNegativeRequest() ProviderRequest {
	return ProviderRequest{Constraints: []Constraint{
		numericConstraint("transient_load_current", "maximum", 1, "A", 0),
		numericConstraint("transient_duration", "maximum", .01, "s", 0),
		numericConstraint("maximum_voltage_droop", "maximum", .1, "V", 0),
	}}
}

func providerNegativeCase(provider *CatalogProvider, build func() ProviderRequest) func(bool) (reports.Code, string) {
	return func(reversed bool) (reports.Code, string) {
		request := build()
		if reversed {
			slices.Reverse(request.Ports)
			slices.Reverse(request.Constraints)
		}
		_, err := provider.Expand(context.Background(), request)
		var typed *interfaceSynthesisError
		if !errors.As(err, &typed) {
			return "", errorString(err)
		}
		return typed.code, typed.message
	}
}

func synthesisErrorCode(err error) (reports.Code, string) {
	var typed *powerSynthesisError
	if !errors.As(err, &typed) {
		return "", errorString(err)
	}
	return typed.code, typed.message
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
