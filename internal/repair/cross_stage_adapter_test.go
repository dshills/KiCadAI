package repair

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/repairloop"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestCrossStageTransactionTargetRepairsOnlyAffectedRouteAndReplays(t *testing.T) {
	firstReport, firstTransaction := runCrossStageTransactionRouteRepair(t)
	secondReport, secondTransaction := runCrossStageTransactionRouteRepair(t)
	if firstReport.Status != repairloop.CrossStageStatusPassed || firstReport.Consumption.CommittedRepairs != 1 {
		t.Fatalf("cross-stage transaction report = %#v", firstReport)
	}
	if err := repairloop.ValidateCrossStageReport(firstReport); err != nil {
		t.Fatalf("cross-stage transaction report invalid: %v", err)
	}
	firstJSON, _ := json.Marshal(firstReport)
	secondJSON, _ := json.Marshal(secondReport)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("cross-stage transaction reports differ")
	}
	if !reflect.DeepEqual(firstTransaction, secondTransaction) {
		t.Fatal("cross-stage transaction replay differs")
	}
}

func TestTransactionActionScopesIncludeEveryGeneratedMutation(t *testing.T) {
	execution := ExecutionContext{
		PlacementOps: []transactions.Operation{
			{Op: transactions.OpPlaceFootprint, Ref: "R2"},
			{Op: transactions.OpPlaceFootprint, Ref: "R1"},
		},
		RouteOps: []transactions.Operation{
			{Op: transactions.OpRoute, Net: "SIG_B"},
			{Op: transactions.OpRoute, Net: "SIG_A"},
		},
	}
	if got, want := transactionActionScopes(ActionRetryPlacement, []string{"ref:R1"}, execution), []string{"ref:R1", "ref:R2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("placement scopes=%#v want=%#v", got, want)
	}
	if got, want := transactionActionScopes(ActionRerouteNet, []string{"net:sig_a"}, execution), []string{"net:sig_a", "net:sig_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("routing scopes=%#v want=%#v", got, want)
	}
	if got, want := transactionActionScopes(ActionGenerateOutline, []string{"stage:placement"}, execution), []string{"board:outline", "stage:placement"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outline scopes=%#v want=%#v", got, want)
	}
}

func TestCrossStageTransactionTargetRepairsPhysicalStages(t *testing.T) {
	for _, test := range []struct {
		stage repairloop.CrossStage
		code  reports.Code
	}{
		{stage: repairloop.CrossStageConnectivity, code: reports.CodeRouteGraphIncomplete},
		{stage: repairloop.CrossStageDRC, code: reports.CodeRouteCopperConflict},
	} {
		t.Run(string(test.stage), func(t *testing.T) {
			report, _ := runCrossStageTransactionRouteStageRepair(t, test.stage, test.code)
			if report.Status != repairloop.CrossStageStatusPassed || report.Consumption.CommittedRepairs != 1 || len(report.Trials) != 1 {
				t.Fatalf("stage %s report=%#v", test.stage, report)
			}
			if trial := report.Trials[0]; !trial.Confirmed || trial.Proposal.ReenterStage != repairloop.CrossStageRouting {
				t.Fatalf("stage %s trial=%#v", test.stage, trial)
			}
		})
	}

	t.Run("placement", func(t *testing.T) {
		failed := mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op: transactions.OpPlaceFootprint, Ref: "R1", At: transactions.Point{XMM: 1, YMM: 1},
		}, "R1")
		replacement := mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op: transactions.OpPlaceFootprint, Ref: "R1", At: transactions.Point{XMM: 5, YMM: 5},
		}, "R1")
		transaction := transactions.Transaction{Name: "placement", Project: "placement", Operations: []transactions.Operation{failed}}
		validate := func(_ context.Context, _ repairloop.CrossStage, current *transactions.Transaction) (CrossStageTransactionEvidence, error) {
			stage := CrossStageTransactionStage{Stage: repairloop.CrossStagePlacement}
			for _, operation := range current.Operations {
				if operation.Op != transactions.OpPlaceFootprint || operation.Ref != "R1" {
					continue
				}
				var placement transactions.PlaceFootprintOperation
				if err := json.Unmarshal(operation.Raw, &placement); err != nil {
					return CrossStageTransactionEvidence{}, err
				}
				if placement.At.XMM < 5 {
					stage.Issues = []reports.Issue{{
						Code: reports.CodePlacementOutsideBoard, Severity: reports.SeverityBlocked,
						Path: "placements.R1", Refs: []string{"R1"}, Message: "outside generated region",
					}}
				}
			}
			return CrossStageTransactionEvidence{Stages: []CrossStageTransactionStage{stage}}, nil
		}
		opts := DefaultOptions()
		opts.AllowPlacementRetry = true
		target, err := NewCrossStageTransactionTarget(CrossStageTransactionTargetOptions{
			Repair: opts, Execution: ExecutionContext{Transaction: &transaction, PlacementOps: []transactions.Operation{replacement}},
			RequiredStages: []repairloop.CrossStage{repairloop.CrossStagePlacement}, Validate: validate,
		})
		if err != nil {
			t.Fatal(err)
		}
		report, err := repairloop.RunCrossStageRepair(context.Background(), target, repairloop.DefaultCrossStagePolicy())
		if err != nil {
			t.Fatal(err)
		}
		if report.Status != repairloop.CrossStageStatusPassed || report.Consumption.CommittedRepairs != 1 || len(report.Trials) != 1 {
			t.Fatalf("placement report=%#v", report)
		}
		if trial := report.Trials[0]; !trial.Confirmed || trial.Proposal.ReenterStage != repairloop.CrossStagePlacement {
			t.Fatalf("placement trial=%#v", trial)
		}
	})
}

func runCrossStageTransactionRouteStageRepair(t *testing.T, stage repairloop.CrossStage, code reports.Code) (repairloop.CrossStageReport, transactions.Transaction) {
	t.Helper()
	failed := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: 0.25,
		Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 1, YMM: 1}},
	}, "")
	replacement := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: 0.25,
		Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 1}},
	}, "")
	transaction := transactions.Transaction{Name: "physical", Project: "physical", Operations: []transactions.Operation{failed}}
	validate := func(_ context.Context, _ repairloop.CrossStage, current *transactions.Transaction) (CrossStageTransactionEvidence, error) {
		result := CrossStageTransactionStage{Stage: stage}
		for _, operation := range current.Operations {
			if operation.Op != transactions.OpRoute || operation.Net != "SIG" {
				continue
			}
			var route transactions.RouteOperation
			if err := json.Unmarshal(operation.Raw, &route); err != nil {
				return CrossStageTransactionEvidence{}, err
			}
			if len(route.Points) < 2 || route.Points[len(route.Points)-1].XMM < 5 {
				result.Issues = []reports.Issue{{
					Code: code, Severity: reports.SeverityBlocked, Path: "nets.SIG", Nets: []string{"SIG"},
					Message: "structured physical fault",
				}}
			}
		}
		return CrossStageTransactionEvidence{Stages: []CrossStageTransactionStage{result}}, nil
	}
	opts := DefaultOptions()
	opts.AllowRoutingRetry = true
	target, err := NewCrossStageTransactionTarget(CrossStageTransactionTargetOptions{
		Repair: opts, Execution: ExecutionContext{Transaction: &transaction, RouteOps: []transactions.Operation{replacement}},
		RequiredStages: []repairloop.CrossStage{stage}, Validate: validate,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := repairloop.RunCrossStageRepair(context.Background(), target, repairloop.DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := repairloop.ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	return report, transaction
}

func runCrossStageTransactionRouteRepair(t *testing.T) (repairloop.CrossStageReport, transactions.Transaction) {
	t.Helper()
	failed := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: 0.25,
		Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 1, YMM: 1}},
	}, "")
	replacement := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "SIG", Layer: "F.Cu", WidthMM: 0.25,
		Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 1}},
	}, "")
	unrelated := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{
		Op: transactions.OpRoute, NetName: "KEEP", Layer: "B.Cu", WidthMM: 0.5,
		Points: []transactions.Point{{XMM: 0, YMM: 3}, {XMM: 5, YMM: 3}},
	}, "")
	transaction := transactions.Transaction{Name: "cross-stage", Project: "cross-stage", Operations: []transactions.Operation{
		failed,
		unrelated,
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	unrelatedBefore := unrelated.Clone()
	validate := func(_ context.Context, _ repairloop.CrossStage, current *transactions.Transaction) (CrossStageTransactionEvidence, error) {
		stages := []CrossStageTransactionStage{
			{Stage: repairloop.CrossStageRouting},
			{Stage: repairloop.CrossStageConnectivity},
			{Stage: repairloop.CrossStageWriter},
			{Stage: repairloop.CrossStageRoundTrip},
			{Stage: repairloop.CrossStageDRC},
		}
		for _, operation := range current.Operations {
			if operation.Op != transactions.OpRoute || operation.Net != "SIG" {
				continue
			}
			var route transactions.RouteOperation
			if err := json.Unmarshal(operation.Raw, &route); err != nil {
				return CrossStageTransactionEvidence{}, err
			}
			if len(route.Points) < 2 || route.Points[len(route.Points)-1].XMM < 5 {
				stages[0].Issues = []reports.Issue{{
					Code: reports.CodeRouteCopperConflict, Severity: reports.SeverityBlocked,
					Path: "nets.SIG", Nets: []string{"SIG"},
					Message: "test-only frozen fault detail is not used for classification",
				}}
			}
		}
		return CrossStageTransactionEvidence{
			Stages: stages,
			Margins: []repairloop.CrossStageMargin{{
				ID: "physical_clearance", Stage: repairloop.CrossStageDRC, Headroom: 0.5,
				Protected: true, EvidenceHash: transactionCrossStageHash("physical_clearance:0.5"),
			}},
		}, nil
	}
	opts := DefaultOptions()
	opts.AllowRoutingRetry = true
	target, err := NewCrossStageTransactionTarget(CrossStageTransactionTargetOptions{
		Repair: opts,
		Execution: ExecutionContext{
			Transaction: &transaction,
			RouteOps:    []transactions.Operation{replacement},
		},
		RequiredStages: []repairloop.CrossStage{
			repairloop.CrossStageRouting,
			repairloop.CrossStageConnectivity,
			repairloop.CrossStageWriter,
			repairloop.CrossStageRoundTrip,
			repairloop.CrossStageDRC,
		},
		Validate: validate,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := repairloop.RunCrossStageRepair(context.Background(), target, repairloop.DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(transaction.Operations) != 3 || !reflect.DeepEqual(transaction.Operations[1], unrelatedBefore) {
		t.Fatalf("unrelated route changed: %#v", transaction.Operations)
	}
	return report, transaction
}
