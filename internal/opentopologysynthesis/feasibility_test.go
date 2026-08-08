package opentopologysynthesis

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequirementFeasibilityProvesTransconductanceComplianceContradiction(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "illumination_proportional_power_control.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	issues := requirementFeasibilityIssues(requirement)
	if len(issues) != 1 || issues[0].Code != CodeRequirementInfeasible ||
		!strings.Contains(issues[0].Path, "proportional_transfer") ||
		!strings.Contains(issues[0].Message, "22.04 V") ||
		!strings.Contains(issues[0].Message, "15 V") {
		t.Fatalf("feasibility issues = %#v", issues)
	}

	feasible := requirement
	for caseIndex := range feasible.Requirements.OperatingCases {
		for conditionIndex := range feasible.Requirements.OperatingCases[caseIndex].Conditions {
			condition := &feasible.Requirements.OperatingCases[caseIndex].Conditions[conditionIndex]
			if condition.Axis == "load_resistance" && condition.Target == "power_output" {
				condition.Max = 15
			}
		}
	}
	if issues := requirementFeasibilityIssues(feasible); len(issues) != 0 {
		t.Fatalf("physically bounded envelope was rejected: %#v", issues)
	}
}

func TestRequirementFeasibilityUsesOutputSupplyDomainCeiling(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "illumination_proportional_power_control.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	for index := range requirement.Requirements.Ports {
		port := &requirement.Requirements.Ports[index]
		if port.ID == "power_output" {
			port.Domain = "control_supply"
			port.Electrical.MaxVoltageV = nil
		}
	}
	for caseIndex := range requirement.Requirements.OperatingCases {
		for conditionIndex := range requirement.Requirements.OperatingCases[caseIndex].Conditions {
			condition := &requirement.Requirements.OperatingCases[caseIndex].Conditions[conditionIndex]
			if condition.Axis == "load_resistance" && condition.Target == "power_output" {
				condition.Max = 15
			}
		}
	}
	issues := requirementFeasibilityIssues(requirement)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "5.25 V") {
		t.Fatalf("domain-bound feasibility issues = %#v", issues)
	}
}

func TestRequirementFeasibilityAllowsDeclaredBipolarSupplySpan(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "illumination_proportional_power_control.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	negativeMinimum, negativeNominal, negativeMaximum := -15.0, -12.0, -9.0
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{
		ID: "negative_supply", Kind: "supply", Source: "external",
		MinVoltageV: &negativeMinimum, NominalVoltageV: &negativeNominal, MaxVoltageV: &negativeMaximum,
	})
	for index := range requirement.Requirements.Ports {
		if requirement.Requirements.Ports[index].ID == "power_output" {
			requirement.Requirements.Ports[index].Electrical.MaxVoltageV = nil
		}
	}
	if issues := requirementFeasibilityIssues(requirement); len(issues) != 0 {
		t.Fatalf("declared bipolar rail span was rejected: %#v", issues)
	}
}

func TestSynthesizeFailsClosedBeforeSearchForProvenInfeasibleRequirement(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "illumination_proportional_power_control.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	first := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
	second := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
	if first.Report.Status != StatusInfeasible || first.Report.StopReason != StopRequirementInfeasible ||
		first.Report.Selected != nil || first.SelectedGraph != nil || first.Physical != nil ||
		first.Search.Schema != "" || len(first.Report.Diagnostics) != 1 ||
		first.Report.Diagnostics[0].Code != CodeRequirementInfeasible {
		t.Fatalf("infeasible synthesis did not fail closed before search: %#v", first)
	}
	if first.Hash == "" || first.Hash != second.Hash {
		t.Fatalf("infeasible synthesis replay hashes = %q and %q", first.Hash, second.Hash)
	}
}

func TestRequirementCapabilityRejectsUnreviewedClosedLoopSpeedBeforeSearch(t *testing.T) {
	requirement, decodeIssues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(multiStageOODCorpusRoot(), "adversarial_ultrafast_protected_feedback.json"),
	)))
	if len(decodeIssues) != 0 {
		t.Fatalf("requirement decode issues: %#v", decodeIssues)
	}
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	issues := requirementCapabilityIssues(requirement, inventory)
	if len(issues) == 0 || issues[0].Code != CodeModelUnavailable ||
		!strings.Contains(issues[0].Path, "response_bandwidth") {
		t.Fatalf("dynamic capability issues = %#v", issues)
	}
	first := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
	second := Synthesize(context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy())
	if first.Report.Status != StatusUnsupported || first.Report.StopReason != StopModelUnavailable ||
		first.Report.Selected != nil || first.SelectedGraph != nil || first.Physical != nil ||
		first.Search.Schema != "" || len(first.Report.Diagnostics) == 0 ||
		first.Report.Diagnostics[0].Code != CodeModelUnavailable {
		t.Fatalf("unsupported dynamic synthesis did not fail closed before search: %#v", first)
	}
	if first.Hash == "" || first.Hash != second.Hash {
		t.Fatalf("unsupported dynamic synthesis replay hashes = %q and %q", first.Hash, second.Hash)
	}
}
