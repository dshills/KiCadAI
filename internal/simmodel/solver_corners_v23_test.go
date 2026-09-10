package simmodel

import (
	"fmt"
	"slices"
	"testing"
)

func TestPassingCornersV23ExactScheduleAndRefusals(t *testing.T) {
	for _, worstCase := range []bool{false, true} {
		plan := sweepPlanV23(t, worstCase)
		report, diagnostics, _ := EvaluateWithSolverV23(plan)
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		if err := VerifyPassingCornersV23(plan, report); err != nil {
			t.Fatal(err)
		}
		if !worstCase {
			continue
		}
		for name, mutate := range map[string]func(*Report){
			"missing corners":       func(r *Report) { r.Corners = nil },
			"missing single corner": func(r *Report) { r.Corners = r.Corners[:len(r.Corners)-1] },
			"extra corner":          func(r *Report) { r.Corners = append(r.Corners, r.Corners[0]) },
			"reordered corners":     func(r *Report) { slices.Reverse(r.Corners) },
			"changed assignment":    func(r *Report) { r.Corners[1].Assignments[0].Value++ },
			"changed bounds":        func(r *Report) { r.Corners[1].Assertions[0].Min-- },
			"changed assertion":     func(r *Report) { r.Corners[1].Assertions[0].Node = "IN" },
			"false corner status":   func(r *Report) { r.Corners[1].Status = "blocked" },
			"false numerical pass":  func(r *Report) { r.Corners[1].Assertions[0].Actual = 99 },
		} {
			t.Run(name, func(t *testing.T) {
				changed := CloneReport(report)
				mutate(&changed)
				if VerifyPassingCornersV23(plan, changed) == nil {
					t.Fatal("incomplete or false corner proof accepted")
				}
			})
		}
	}
}

func TestPassingCornersV23PreservesDirectedSchedule(t *testing.T) {
	var extra []ComponentEvidence
	var uncertainties []Uncertainty
	for i := 0; i < maxExhaustiveWorstCaseGroups+1; i++ {
		id := fmt.Sprintf("independent_load_%d", i)
		extra = append(extra, resistorEvidence(id, 47000, "OUT", "GND"))
		uncertainties = append(uncertainties, Uncertainty{Target: "devices." + id + ".value_si", Source: "independent-tolerance", Nominal: 47000, Minimum: 46000, Maximum: 48000})
	}
	plan := activeStatePlanV23(t, 2.4, extra...)
	plan.WorstCase, plan.Uncertainties = true, uncertainties
	report, diagnostics, _ := EvaluateWithSolverV23(plan)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if err := VerifyPassingCornersV23(plan, report); err != nil {
		t.Fatal(err)
	}
	report.Corners = report.Corners[:len(report.Corners)-1]
	if VerifyPassingCornersV23(plan, report) == nil {
		t.Fatal("directed schedule omission accepted")
	}
}
