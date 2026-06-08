// Package workflows provides safe planning and execution boundaries for KiCad automation intents.
package workflows

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// OperationName identifies a safe workflow operation that AI orchestration may request.
type OperationName string

// OperationStatus describes the planning outcome for a safe workflow operation.
type OperationStatus string

const (
	OperationCreateLEDIndicator       OperationName = "create_led_indicator"
	OperationPlaceDecouplingCapacitor OperationName = "place_decoupling_capacitor"
	OperationCreateConnectorBlock     OperationName = "create_connector_block"
)

const (
	ValidationSeverityError = "error"

	OperationStatusPlanned        OperationStatus = "planned"
	OperationStatusInvalid        OperationStatus = "invalid"
	OperationStatusNotImplemented OperationStatus = "not_implemented"
)

var (
	ErrUnknownOperation        = errors.New("unknown workflow operation")
	ErrOperationNotImplemented = errors.New("workflow operation is not implemented")
	ErrMissingOperationIntent  = errors.New("workflow operation intent is required")
)

// ValidationIssue is a structured diagnostic returned by workflow validation.
type ValidationIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// OperationDescriptor describes a registered workflow operation.
type OperationDescriptor struct {
	Name        OperationName `json:"name"`
	Description string        `json:"description"`
	Implemented bool          `json:"implemented"`
}

// OperationRequest is the generic request envelope for AI-safe workflow operations.
type OperationRequest struct {
	Operation OperationName   `json:"operation"`
	Payload   json.RawMessage `json:"payload"`
}

// OperationResult is the structured response from planning a workflow operation.
type OperationResult struct {
	Operation OperationName     `json:"operation"`
	Status    OperationStatus   `json:"status"`
	Plan      *AutomationPlan   `json:"plan,omitempty"`
	Issues    []ValidationIssue `json:"issues,omitempty"`
}

// OperationError preserves structured workflow issues while satisfying the error interface.
type OperationError struct {
	Operation OperationName
	Status    OperationStatus
	Issues    []ValidationIssue
	Cause     error
}

func (e *OperationError) Error() string {
	if len(e.Issues) > 0 {
		messages := make([]string, 0, len(e.Issues))
		for _, issue := range e.Issues {
			messages = append(messages, issue.Message)
		}
		message := strings.Join(messages, "; ")
		if e.Cause != nil {
			return fmt.Sprintf("workflow %s: %s (cause: %v)", e.Operation, message, e.Cause)
		}
		return fmt.Sprintf("workflow %s: %s", e.Operation, message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("workflow %s: %v", e.Operation, e.Cause)
	}
	return fmt.Sprintf("workflow %s: operation failed", e.Operation)
}

func (e *OperationError) Unwrap() error {
	return e.Cause
}

type operationHandler func(OperationRequest) (OperationResult, error)

type registeredOperation struct {
	descriptor OperationDescriptor
	handler    operationHandler
}

var operationRegistry = []registeredOperation{
	{
		descriptor: OperationDescriptor{
			Name:        OperationCreateLEDIndicator,
			Description: "Create a simple VCC to resistor to LED to GND schematic indicator plan.",
		},
		handler: planCreateLEDIndicator,
	},
	{
		descriptor: OperationDescriptor{
			Name:        OperationPlaceDecouplingCapacitor,
			Description: "Place a decoupling capacitor near a power pin.",
		},
	},
	{
		descriptor: OperationDescriptor{
			Name:        OperationCreateConnectorBlock,
			Description: "Create a labeled connector block.",
		},
	},
}

var operationLookup, safeOperationDescriptors = buildRegistry(operationRegistry)

// NewCreateLEDIndicatorRequest builds a typed request for the initial LED indicator workflow.
func NewCreateLEDIndicatorRequest(intent LEDDemoIntent) (OperationRequest, error) {
	payload, err := json.Marshal(intent)
	if err != nil {
		return OperationRequest{}, err
	}
	return OperationRequest{
		Operation: OperationCreateLEDIndicator,
		Payload:   payload,
	}, nil
}

// SafeOperations returns the registered workflow operations available to AI orchestration.
func SafeOperations() []OperationDescriptor {
	return slices.Clone(safeOperationDescriptors)
}

func buildRegistry(registry []registeredOperation) (map[OperationName]registeredOperation, []OperationDescriptor) {
	lookup := make(map[OperationName]registeredOperation, len(registry))
	descriptors := make([]OperationDescriptor, 0, len(registry))
	for i := range registry {
		operation := registry[i]
		if operation.descriptor.Name == "" {
			panic("workflow operation name is required")
		}
		if _, exists := lookup[operation.descriptor.Name]; exists {
			panic(fmt.Sprintf("duplicate workflow operation %q", operation.descriptor.Name))
		}
		operation.descriptor.Implemented = operation.handler != nil
		registry[i].descriptor.Implemented = operation.descriptor.Implemented
		lookup[operation.descriptor.Name] = operation
		descriptors = append(descriptors, operation.descriptor)
	}
	return lookup, descriptors
}

// PlanOperation validates and plans a named workflow operation without touching KiCad transport APIs.
// Validation failures return both a structured OperationResult for JSON-style callers and an OperationError for idiomatic Go callers.
func PlanOperation(request OperationRequest) (OperationResult, error) {
	operation, ok := operationLookup[request.Operation]
	if !ok {
		return operationError(request.Operation, OperationStatusInvalid, ErrUnknownOperation, "unknown_operation", fmt.Sprintf("%q is not a safe workflow operation", request.Operation))
	}
	if operation.handler == nil {
		return operationError(request.Operation, OperationStatusNotImplemented, ErrOperationNotImplemented, "not_implemented", fmt.Sprintf("%q is not implemented", request.Operation))
	}
	return operation.handler(request)
}

func planCreateLEDIndicator(request OperationRequest) (OperationResult, error) {
	if len(request.Payload) == 0 {
		return operationError(request.Operation, OperationStatusInvalid, ErrMissingOperationIntent, "missing_intent", fmt.Sprintf("%s intent is required", OperationCreateLEDIndicator))
	}

	var intent LEDDemoIntent
	if err := json.Unmarshal(request.Payload, &intent); err != nil {
		return operationError(request.Operation, OperationStatusInvalid, err, "invalid_payload", err.Error())
	}

	plan, err := PlanLEDDemo(intent)
	if err != nil {
		return operationError(request.Operation, OperationStatusInvalid, err, "invalid_intent", err.Error())
	}

	return OperationResult{
		Operation: request.Operation,
		Status:    OperationStatusPlanned,
		Plan:      &plan,
	}, nil
}

func operationError(operation OperationName, status OperationStatus, err error, code string, message string) (OperationResult, error) {
	issues := []ValidationIssue{{
		Severity: ValidationSeverityError,
		Code:     code,
		Message:  message,
	}}
	return OperationResult{
			Operation: operation,
			Status:    status,
			Issues:    issues,
		}, &OperationError{
			Operation: operation,
			Status:    status,
			Issues:    issues,
			Cause:     err,
		}
}
