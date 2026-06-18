package repair

import (
	"encoding/json"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestExecutorMissingFootprintDryRunShowsAssignment(t *testing.T) {
	tx := transactions.Transaction{}
	attempt := Attempt{
		Action: ActionAssignFootprint,
		DryRun: true,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}).Execute(attempt)
	if got.Status != "" || len(tx.Operations) != 0 || len(got.Operations) != 1 {
		t.Fatalf("attempt = %#v tx = %#v", got, tx)
	}
}

func TestExecutorMissingFootprintApplyAddsOperation(t *testing.T) {
	tx := transactions.Transaction{}
	attempt := Attempt{
		Action: ActionAssignFootprint,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", Role: "resistor", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}).Execute(attempt)
	if got.Status != StatusRepaired || len(tx.Operations) != 1 || tx.Operations[0].Op != transactions.OpAssignFootprint {
		t.Fatalf("attempt = %#v tx = %#v", got, tx)
	}
	var payload transactions.AssignFootprintOperation
	if err := json.Unmarshal(tx.Operations[0].Raw, &payload); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	if payload.Ref != "R1" || payload.FootprintID != "Resistor_SMD:R_0805_2012Metric" || payload.Role != "resistor" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestExecutorNormalizesFootprintEvidenceKeys(t *testing.T) {
	tx := transactions.Transaction{}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"r1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}).Execute(Attempt{
		Action: ActionAssignFootprint,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	})
	if got.Status != StatusRepaired || len(tx.Operations) != 1 {
		t.Fatalf("attempt = %#v tx = %#v", got, tx)
	}
}

func TestExecutorMissingFootprintBlocksWithoutVerifiedEvidence(t *testing.T) {
	tx := transactions.Transaction{}
	attempt := Attempt{
		Action: ActionAssignFootprint,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric"},
		},
	}).Execute(attempt)
	if got.Status != StatusBlocked || len(tx.Operations) != 0 {
		t.Fatalf("attempt = %#v tx = %#v", got, tx)
	}
}

func TestExecutorMissingFootprintUpdatesExistingAssignment(t *testing.T) {
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpAssignFootprint, transactions.AssignFootprintOperation{
			Op:          transactions.OpAssignFootprint,
			Ref:         "R1",
			FootprintID: "Old:Footprint",
		}, "R1"),
		mustRepairOperation(t, transactions.OpAssignFootprint, transactions.AssignFootprintOperation{
			Op:          transactions.OpAssignFootprint,
			Ref:         "R1",
			FootprintID: "Older:Footprint",
		}, "R1"),
	}}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}).Execute(Attempt{
		Action: ActionAssignFootprint,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	})
	if got.Status != StatusRepaired || len(tx.Operations) != 2 {
		t.Fatalf("attempt = %#v tx = %#v", got, tx)
	}
	for index, operation := range tx.Operations {
		var payload transactions.AssignFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode operation %d: %v", index, err)
		}
		if payload.FootprintID != "Resistor_SMD:R_0805_2012Metric" {
			t.Fatalf("payload %d = %#v", index, payload)
		}
	}
}

func TestExecutorCanDeferRevalidation(t *testing.T) {
	tx := transactions.Transaction{}
	validator := &countingRevalidator{}
	got := NewExecutor(ExecutionContext{
		Transaction:     &tx,
		DeferValidation: true,
		Revalidate:      validator,
		Footprints: map[string]FootprintEvidence{
			"R1": {Ref: "R1", FootprintID: "Resistor_SMD:R_0805_2012Metric", Verified: true},
		},
	}).Execute(Attempt{
		Action: ActionAssignFootprint,
		Issue:  reports.Issue{Code: reports.CodeMissingFootprint, Refs: []string{"R1"}},
	})
	if got.Status != StatusRepaired || validator.calls != 0 {
		t.Fatalf("attempt = %#v validator calls = %d", got, validator.calls)
	}
}

func TestExecutorPadNetRepairAlreadyMatchedIsRepaired(t *testing.T) {
	net := "SIG"
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:   transactions.OpPlaceFootprint,
			Ref:  "J1",
			At:   transactions.Point{},
			Pads: []transactions.PadSpec{{Name: "1", Net: &net}},
		}, "J1"),
	}}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		PadNets:     []PadNetHint{{Ref: "J1", Pad: "1", Net: "SIG"}},
	}).Execute(Attempt{
		Action: ActionRegeneratePadNets,
		Issue:  reports.Issue{Code: reports.CodeInvalidNetAssignment, Refs: []string{"J1"}},
	})
	if got.Status != StatusRepaired {
		t.Fatalf("attempt = %#v", got)
	}
}

func TestExecutorPadNetRepairUpdatesGeneratedPlaceFootprint(t *testing.T) {
	net := "OLD"
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:  transactions.OpPlaceFootprint,
			Ref: "J1",
			At:  transactions.Point{XMM: 10, YMM: 20},
			Pads: []transactions.PadSpec{
				{Name: "1", Net: &net},
				{Name: "2"},
			},
		}, "J1"),
	}}
	attempt := Attempt{
		Action: ActionRegeneratePadNets,
		Issue:  reports.Issue{Code: reports.CodeInvalidNetAssignment, Refs: []string{"J1"}},
	}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		PadNets: []PadNetHint{
			{Ref: "J1", Pad: "1", Net: "VBUS"},
			{Ref: "J1", Pad: "2", Net: "GND"},
		},
	}).Execute(attempt)
	if got.Status != StatusRepaired {
		t.Fatalf("attempt = %#v", got)
	}
	var payload transactions.PlaceFootprintOperation
	if err := json.Unmarshal(tx.Operations[0].Raw, &payload); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	assertPadNetHint(t, payload.Pads, "1", "VBUS")
	assertPadNetHint(t, payload.Pads, "2", "GND")
	if payload.At.XMM != 10 || payload.At.YMM != 20 {
		t.Fatalf("placement changed: %#v", payload.At)
	}
}

func TestExecutorPadNetRepairRefusesUnknownTargets(t *testing.T) {
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:   transactions.OpPlaceFootprint,
			Ref:  "J1",
			At:   transactions.Point{XMM: 10, YMM: 20},
			Pads: []transactions.PadSpec{{Name: "1"}},
		}, "J1"),
	}}
	attempt := Attempt{
		Action: ActionRegeneratePadNets,
		Issue:  reports.Issue{Code: reports.CodeInvalidNetAssignment},
	}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		PadNets:     []PadNetHint{{Ref: "J1", Pad: "1", Net: "SIG"}},
	}).Execute(attempt)
	if got.Status != StatusBlocked {
		t.Fatalf("attempt = %#v", got)
	}
}

func TestExecutorPadNetRepairUsesAllRefsFromIssuePath(t *testing.T) {
	tx := transactions.Transaction{Operations: []transactions.Operation{
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{Op: transactions.OpPlaceFootprint, Ref: "J1", At: transactions.Point{}, Pads: []transactions.PadSpec{{Name: "1"}}}, "J1"),
		mustRepairOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{Op: transactions.OpPlaceFootprint, Ref: "J2", At: transactions.Point{}, Pads: []transactions.PadSpec{{Name: "1"}}}, "J2"),
	}}
	got := NewExecutor(ExecutionContext{
		Transaction: &tx,
		PadNets: []PadNetHint{
			{Ref: "J1", Pad: "1", Net: "A"},
			{Ref: "J2", Pad: "1", Net: "B"},
		},
	}).Execute(Attempt{
		Action: ActionRegeneratePadNets,
		Issue:  reports.Issue{Code: reports.CodeInvalidNetAssignment, Path: `pcb.footprints["J1"].to["J2"]`},
	})
	if got.Status != StatusRepaired {
		t.Fatalf("attempt = %#v", got)
	}
	for index, want := range []string{"A", "B"} {
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(tx.Operations[index].Raw, &payload); err != nil {
			t.Fatalf("decode operation %d: %v", index, err)
		}
		assertPadNetHint(t, payload.Pads, "1", want)
	}
}

type countingRevalidator struct {
	calls int
}

func (validator *countingRevalidator) Validate() []reports.Issue {
	validator.calls++
	return nil
}

func mustRepairOperation(t *testing.T, kind transactions.OperationKind, payload any, ref string) transactions.Operation {
	t.Helper()
	op, err := repairOperation(kind, payload, ref)
	if err != nil {
		t.Fatalf("repairOperation: %v", err)
	}
	return op
}

func assertPadNetHint(t *testing.T, pads []transactions.PadSpec, name string, want string) {
	t.Helper()
	for _, pad := range pads {
		if pad.Name != name {
			continue
		}
		if pad.Net == nil || *pad.Net != want {
			t.Fatalf("pad %s net = %#v, want %s", name, pad.Net, want)
		}
		return
	}
	t.Fatalf("missing pad %s in %#v", name, pads)
}
