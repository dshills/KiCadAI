package workflows

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kiapi"
	"kicadai/internal/schematic"
)

func TestSafeOperationsIncludesOnlyLEDImplemented(t *testing.T) {
	operations := SafeOperations()

	implemented := map[OperationName]bool{}
	for _, operation := range operations {
		implemented[operation.Name] = operation.Implemented
	}

	if !implemented[OperationCreateLEDIndicator] {
		t.Fatalf("%s should be implemented", OperationCreateLEDIndicator)
	}
	for _, operation := range []OperationName{OperationPlaceDecouplingCapacitor, OperationCreateConnectorBlock} {
		if implemented[operation] {
			t.Fatalf("%s should not be implemented yet", operation)
		}
	}
}

func TestPlanOperationCreateLEDIndicator(t *testing.T) {
	request, err := NewCreateLEDIndicatorRequest(LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"},
	})
	if err != nil {
		t.Fatalf("NewCreateLEDIndicatorRequest returned error: %v", err)
	}

	result, err := PlanOperation(request)
	if err != nil {
		t.Fatalf("PlanOperation returned error: %v", err)
	}
	if result.Status != OperationStatusPlanned {
		t.Fatalf("Status = %q, want %q", result.Status, OperationStatusPlanned)
	}
	if result.Plan == nil || len(result.Plan.Operations) == 0 {
		t.Fatalf("expected non-empty plan, got %+v", result.Plan)
	}
}

func TestPlanOperationRejectsMissingLEDIntent(t *testing.T) {
	result, err := PlanOperation(OperationRequest{Operation: OperationCreateLEDIndicator})
	if !errors.Is(err, ErrMissingOperationIntent) {
		t.Fatalf("PlanOperation error = %v, want %v", err, ErrMissingOperationIntent)
	}
	assertIssue(t, result, "missing_intent")
	assertOperationErrorIssue(t, err, "missing_intent")
}

func TestPlanOperationRejectsInvalidPayload(t *testing.T) {
	result, err := PlanOperation(OperationRequest{
		Operation: OperationCreateLEDIndicator,
		Payload:   json.RawMessage(`{`),
	})
	if err == nil {
		t.Fatalf("PlanOperation returned nil error")
	}
	assertIssue(t, result, "invalid_payload")
}

func TestPlanOperationRejectsFutureOperation(t *testing.T) {
	result, err := PlanOperation(OperationRequest{Operation: OperationCreateConnectorBlock})
	if !errors.Is(err, ErrOperationNotImplemented) {
		t.Fatalf("PlanOperation error = %v, want %v", err, ErrOperationNotImplemented)
	}
	assertIssue(t, result, "not_implemented")
}

func TestPlanOperationRejectsUnknownOperation(t *testing.T) {
	result, err := PlanOperation(OperationRequest{Operation: OperationName("route_power_plane")})
	if !errors.Is(err, ErrUnknownOperation) {
		t.Fatalf("PlanOperation error = %v, want %v", err, ErrUnknownOperation)
	}
	if !strings.Contains(err.Error(), `"route_power_plane" is not a safe workflow operation`) {
		t.Fatalf("PlanOperation error = %v, want operation-specific message", err)
	}
	assertIssue(t, result, "unknown_operation")
}

func TestBuildRegistryRejectsEmptyOperationName(t *testing.T) {
	assertPanics(t, func() {
		buildRegistry([]registeredOperation{{descriptor: OperationDescriptor{}}})
	})
}

func TestBuildRegistryRejectsDuplicateOperationName(t *testing.T) {
	assertPanics(t, func() {
		buildRegistry([]registeredOperation{
			{descriptor: OperationDescriptor{Name: OperationCreateLEDIndicator}},
			{descriptor: OperationDescriptor{Name: OperationCreateLEDIndicator}},
		})
	})
}

func assertIssue(t *testing.T, result OperationResult, code string) {
	t.Helper()
	if len(result.Issues) != 1 {
		t.Fatalf("Issues = %+v, want one issue", result.Issues)
	}
	if result.Issues[0].Severity != ValidationSeverityError || result.Issues[0].Code != code {
		t.Fatalf("Issue = %+v, want error/%s", result.Issues[0], code)
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}

func assertOperationErrorIssue(t *testing.T, err error, code string) {
	t.Helper()

	var operationErr *OperationError
	if !errors.As(err, &operationErr) {
		t.Fatalf("error %v does not expose OperationError", err)
	}
	if len(operationErr.Issues) != 1 || operationErr.Issues[0].Code != code {
		t.Fatalf("OperationError issues = %+v, want %s", operationErr.Issues, code)
	}
}
