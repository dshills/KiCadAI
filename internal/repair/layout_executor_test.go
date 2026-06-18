package repair

import (
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestExecutorGenerateOutlineInsertsBeforeWrite(t *testing.T) {
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Board:       &transactions.BoardSize{WidthMM: 40, HeightMM: 25},
	}).Execute(Attempt{Action: ActionGenerateOutline, Issue: reports.Issue{Code: reports.CodeMissingBoardOutline}})
	if got.Status != StatusRepaired || len(tx.Operations) != 2 || tx.Operations[0].Op != transactions.OpSetBoardOutline || tx.Operations[1].Op != transactions.OpWriteProject {
		t.Fatalf("attempt = %#v operations = %#v", got, tx.Operations)
	}
}

func TestExecutorGenerateOutlineBlocksWithoutDimensions(t *testing.T) {
	tx := transactions.Transaction{}
	got := NewExecutor(ExecutionContext{Transaction: &tx}).Execute(Attempt{Action: ActionGenerateOutline})
	if got.Status != StatusBlocked || len(tx.Operations) != 0 {
		t.Fatalf("attempt = %#v operations = %#v", got, tx.Operations)
	}
}

func TestExecutorRetryPlacementReplacesExistingPlacementOps(t *testing.T) {
	oldOp := mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{Op: transactions.OpPlaceFootprint, Ref: "R1", At: transactions.Point{XMM: 1}}, "R1")
	newOp := mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{Op: transactions.OpPlaceFootprint, Ref: "R1", At: transactions.Point{XMM: 5}}, "R1")
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpCreateProject, transactions.CreateProjectOperation{Op: transactions.OpCreateProject, Name: "demo"}, ""),
		oldOp,
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	got := NewExecutor(ExecutionContext{Transaction: &tx, PlacementOps: []transactions.Operation{newOp}}).Execute(Attempt{Action: ActionRetryPlacement})
	if got.Status != StatusRepaired || len(tx.Operations) != 3 || tx.Operations[1].Raw == nil || tx.Operations[1].Op != transactions.OpPlaceFootprint {
		t.Fatalf("attempt = %#v operations = %#v", got, tx.Operations)
	}
}

func TestExecutorRerouteBlocksWithoutRoutes(t *testing.T) {
	tx := transactions.Transaction{}
	got := NewExecutor(ExecutionContext{Transaction: &tx}).Execute(Attempt{Action: ActionRerouteNet})
	if got.Status != StatusBlocked {
		t.Fatalf("attempt = %#v", got)
	}
}

func TestExecutorRerouteReplacesRouteOps(t *testing.T) {
	oldOp := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{Op: transactions.OpRoute, NetName: "SIG", Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 1, YMM: 1}}}, "")
	newOp := mustRepairOperation(t, transactions.OpRoute, transactions.RouteOperation{Op: transactions.OpRoute, NetName: "SIG", Points: []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 5, YMM: 1}}}, "")
	tx := transactions.Transaction{Operations: []transactions.Operation{
		oldOp,
		mustRepairOperation(t, transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, ""),
	}}
	got := NewExecutor(ExecutionContext{Transaction: &tx, RouteOps: []transactions.Operation{newOp}}).Execute(Attempt{Action: ActionRerouteNet})
	if got.Status != StatusRepaired || len(tx.Operations) != 2 || tx.Operations[0].Op != transactions.OpRoute {
		t.Fatalf("attempt = %#v operations = %#v", got, tx.Operations)
	}
}
