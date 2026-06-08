package workflows

import (
	"errors"

	"kicadai/internal/kiapi"
)

var ErrMissingSchematicWriteCapability = errors.New("schematic write capability is not available")
var ErrExecutionNotImplemented = errors.New("schematic execution is not implemented")

type AutomationResult struct {
	Success             bool   `json:"success"`
	OperationsCompleted int    `json:"operations_completed"`
	FailedOperation     *int   `json:"failed_operation,omitempty"`
	Error               string `json:"error,omitempty"`
}

func ExecuteLEDDemoPlan(_ AutomationPlan, capabilities kiapi.Capabilities) (AutomationResult, error) {
	if !capabilities.Supports(kiapi.CapabilitySchematicWrite) {
		return AutomationResult{
			Success:             false,
			OperationsCompleted: 0,
			Error:               ErrMissingSchematicWriteCapability.Error(),
		}, ErrMissingSchematicWriteCapability
	}

	return AutomationResult{
		Success:             false,
		OperationsCompleted: 0,
		Error:               ErrExecutionNotImplemented.Error(),
	}, ErrExecutionNotImplemented
}
