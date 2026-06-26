package intentplanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

var intentFixtureExpectations = map[string]struct {
	Status     PlanStatus
	KnownGapID string
	IssuePath  string
}{
	"amplifier_module.json":                        {Status: PlanStatusPartial},
	"fabrication_sensor.json":                      {Status: PlanStatusPartial},
	"multi_mcu_ambiguous_support.json":             {Status: PlanStatusBlocked, IssuePath: "functions[2].target"},
	"mcu_external_clock_limited.json":              {Status: PlanStatusPartial, KnownGapID: "mcu.clock.topology_unsupported.clock"},
	"mcu_isp_programmer.json":                      {Status: PlanStatusPartial},
	"mcu_programmer.json":                          {Status: PlanStatusPartial},
	"power_module.json":                            {Status: PlanStatusPartial},
	"regulator_high_current_fallback.json":         {Status: PlanStatusPartial},
	"regulator_insufficient_headroom_blocked.json": {Status: PlanStatusBlocked, IssuePath: "blocks.regulator.params.input_voltage"},
	"synthesis_external_clock_blocked.json":        {Status: PlanStatusPartial, KnownGapID: "mcu.clock.topology_unsupported.clock"},
	"synthesis_uart_programming.json":              {Status: PlanStatusPartial},
	"synthesis_unknown_supply_blocked.json":        {Status: PlanStatusBlocked, IssuePath: "blocks.sensor.supply"},
}

func TestIntentFixturesPlan(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "intent", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no golden intent fixtures found")
	}
	seenExpectations := map[string]bool{}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			plan := Plan(request)
			base := filepath.Base(path)
			expectation, hasExpectation := intentFixtureExpectations[base]
			if hasExpectation {
				seenExpectations[base] = true
			}
			if expectation.Status != "" {
				if plan.Status != expectation.Status {
					t.Fatalf("status = %s, want %s; issues=%#v", plan.Status, expectation.Status, plan.Issues)
				}
				if expectation.IssuePath != "" && !goldenHasIssuePath(plan.Issues, expectation.IssuePath) {
					t.Fatalf("missing expected issue path %s in %#v", expectation.IssuePath, plan.Issues)
				}
			} else if plan.Status != PlanStatusReady || hasErrorIssue(plan.Issues) {
				t.Fatalf("plan failed: status=%s issues=%#v", plan.Status, plan.Issues)
			}
			if expectation.KnownGapID != "" && !hasKnownGapWithID(plan, expectation.KnownGapID) {
				t.Fatalf("missing expected known gap %s in %#v", expectation.KnownGapID, plan.KnownGaps)
			}
			if plan.Status == PlanStatusBlocked {
				return
			}
			if plan.GeneratedRequest == nil {
				t.Fatalf("generated request missing: status=%s issues=%#v", plan.Status, plan.Issues)
			}
			if len(plan.Requirements) == 0 {
				t.Fatalf("requirements missing")
			}
			if len(plan.SelectedBlocks) == 0 {
				t.Fatalf("selected blocks missing")
			}
		})
	}
	for name := range intentFixtureExpectations {
		if !seenExpectations[name] {
			t.Fatalf("unused intent fixture expectation for %s", name)
		}
	}
}

func goldenHasIssuePath(issues []reports.Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}

func hasKnownGapWithID(plan PlanResult, id string) bool {
	for _, gap := range plan.KnownGaps {
		if gap.ID == id || strings.HasPrefix(gap.ID, id+".") {
			return true
		}
	}
	return false
}

func hasErrorIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Severity == reports.SeverityError || issue.Severity == reports.SeverityBlocked {
			return true
		}
	}
	return false
}
