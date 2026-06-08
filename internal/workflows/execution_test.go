package workflows

import (
	"errors"
	"testing"

	"kicadai/internal/kiapi"
	"kicadai/internal/schematic"
)

func TestExecuteLEDDemoPlanRequiresSchematicWrite(t *testing.T) {
	result, err := ExecuteLEDDemoPlan(AutomationPlan{
		Operations: []schematic.PlannedOperation{{Kind: schematic.OpKindAddSymbol}},
	}, kiapi.Capabilities{})

	if !errors.Is(err, ErrMissingSchematicWriteCapability) {
		t.Fatalf("ExecuteLEDDemoPlan error = %v, want %v", err, ErrMissingSchematicWriteCapability)
	}
	if result.OperationsCompleted != 0 || result.FailedOperation != nil {
		t.Fatalf("result = %+v", result)
	}
	if result.Success {
		t.Fatalf("Success = true, want false")
	}
}

func TestExecuteLEDDemoPlanNotImplementedWhenCapabilityPresent(t *testing.T) {
	result, err := ExecuteLEDDemoPlan(AutomationPlan{
		Operations: []schematic.PlannedOperation{
			{Kind: schematic.OpKindAddSymbol},
			{Kind: schematic.OpKindAddWire},
		},
	}, kiapi.Capabilities{Supported: []kiapi.Capability{kiapi.CapabilitySchematicWrite}})

	if !errors.Is(err, ErrExecutionNotImplemented) {
		t.Fatalf("ExecuteLEDDemoPlan error = %v, want %v", err, ErrExecutionNotImplemented)
	}
	if result.Success || result.OperationsCompleted != 0 {
		t.Fatalf("result = %+v", result)
	}
}
