package placement

import (
	"testing"

	"kicadai/internal/transactions"
)

func TestValidateResultReadyForPlacedResult(t *testing.T) {
	request := minimalRequest()
	result := Place(request)

	report := ValidateResult(&request, &result)
	if !report.Ready {
		t.Fatalf("report not ready: %#v", report)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("expected no validation issues, got %#v", report.Issues)
	}
	if report.TransactionResult.OperationCount != len(result.Operations) {
		t.Fatalf("transaction operation count = %d, want %d", report.TransactionResult.OperationCount, len(result.Operations))
	}
}

func TestValidateResultReportsOperationIssues(t *testing.T) {
	request := minimalRequest()
	result := Place(request)
	result.Operations = []transactions.Operation{{Op: transactions.OpPlaceFootprint}}

	report := ValidateResult(&request, &result)
	if report.Ready {
		t.Fatal("report ready with invalid transaction operation")
	}
	if len(report.TransactionResult.Issues) == 0 {
		t.Fatalf("expected transaction validation issues, got %#v", report)
	}
}
