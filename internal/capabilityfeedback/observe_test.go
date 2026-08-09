package capabilityfeedback

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

func TestObserveClassifiesAuthoritativeTerminalOutcomes(t *testing.T) {
	requirement := feedbackTestRequirement(t)
	meta := CaseMeta{ID: "case-001", Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog, SafetyImpact: capabilityevaluation.SafetyRelevant}

	unsupported := feedbackTestRun(t, requirement, opentopologysynthesis.StatusUnsupported, opentopologysynthesis.StopModelUnavailable)
	unsupported.Report.Diagnostics = []opentopologysynthesis.Diagnostic{{
		Code: opentopologysynthesis.CodeModelUnavailable, Path: "requirements.behavioral_requirements.transfer",
	}}
	first, err := Observe(meta, requirement, unsupported, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Observe(meta, requirement, unsupported, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != OutcomeUnsupported || len(first.Gaps) != 1 || first.Gaps[0].Scope != ScopeModel ||
		first.Gaps[0].Capability != "trusted_simulation_model" || first.Hash == "" || !reflect.DeepEqual(first, second) {
		t.Fatalf("unsupported observation = %#v", first)
	}

	infeasible := feedbackTestRun(t, requirement, opentopologysynthesis.StatusInfeasible, opentopologysynthesis.StopRequirementInfeasible)
	infeasible.Report.Diagnostics = []opentopologysynthesis.Diagnostic{{Code: opentopologysynthesis.CodeRequirementInfeasible, Path: "requirements.behavioral_requirements.transfer"}}
	observed, err := Observe(meta, requirement, infeasible, nil)
	if err != nil || observed.Outcome != OutcomeUnsafe || observed.Gaps[0].Capability != "behavioral_feasibility" {
		t.Fatalf("infeasible observation = %#v err=%v", observed, err)
	}

	exhausted := feedbackTestRun(t, requirement, opentopologysynthesis.StatusExhausted, opentopologysynthesis.StopRepairExhausted)
	exhausted.Report.Consumption.BudgetExhausted = true
	exhausted.Report.Diagnostics = []opentopologysynthesis.Diagnostic{{Code: opentopologysynthesis.CodeRepairExhausted}}
	observed, err = Observe(meta, requirement, exhausted, nil)
	if err != nil || observed.Outcome != OutcomeExhausted || observed.Gaps[0].Scope != ScopeTopology {
		t.Fatalf("exhausted observation = %#v err=%v", observed, err)
	}

	passed := feedbackTestRun(t, requirement, opentopologysynthesis.StatusPassed, opentopologysynthesis.StopPassed)
	promotion := feedbackPassingPromotion(t, requirement, passed)
	observed, err = Observe(meta, requirement, passed, &promotion)
	if err != nil || observed.Outcome != OutcomePass || len(observed.Gaps) != 0 || observed.ProjectHash != promotion.ProjectHash {
		t.Fatalf("passing observation = %#v err=%v", observed, err)
	}

	invalid := feedbackTestRun(t, requirement, opentopologysynthesis.StatusInvalid, opentopologysynthesis.StopRequirementInvalid)
	if _, err := Observe(meta, requirement, invalid, nil); err == nil {
		t.Fatal("invalid synthesis was counted as a product capability outcome")
	}
	if _, err := Observe(meta, requirement, passed, nil); err == nil {
		t.Fatal("passing synthesis without physical promotion was accepted")
	}
}

func TestObserveValidatesSeparateTopologyAndFullGraphHashes(t *testing.T) {
	requirement := feedbackTestRequirement(t)
	run := feedbackTestRun(t, requirement, opentopologysynthesis.StatusPassed, opentopologysynthesis.StopPassed)
	if run.Report.Selected.TopologyHash == run.Physical.GraphHash {
		t.Fatal("test graph does not distinguish topology and full graph hashes")
	}
	promotion := feedbackPassingPromotion(t, requirement, run)
	meta := CaseMeta{ID: "case-hash-domains", Role: RoleDiscovery, Domain: capabilityevaluation.DomainAnalog, SafetyImpact: capabilityevaluation.SafetyReviewRequired}
	if _, err := Observe(meta, requirement, run, &promotion); err != nil {
		t.Fatalf("valid separate hash domains were rejected: %v", err)
	}
	run.Physical.GraphHash = run.Report.Selected.TopologyHash
	if _, err := Observe(meta, requirement, run, &promotion); err == nil {
		t.Fatal("topology hash substituted for full physical graph hash was accepted")
	}
}

func TestObserveUsesUniversalDiagnosisAsRootAndSuppressesTerminalWrapper(t *testing.T) {
	requirement := feedbackTestRequirement(t)
	run := feedbackTestRun(t, requirement, opentopologysynthesis.StatusExhausted, opentopologysynthesis.StopRepairExhausted)
	run.Report.Diagnostics = []opentopologysynthesis.Diagnostic{{Code: opentopologysynthesis.CodeRepairExhausted}}
	for _, fingerprint := range []string{"candidate-a", "candidate-b"} {
		run.Candidates = append(run.Candidates, opentopologysynthesis.SynthesisCandidateEvidence{
			Fingerprint: fingerprint,
			Evaluations: []opentopologysynthesis.SimulationEvaluation{{
				Diagnoses: []opentopologysynthesis.Diagnosis{{
					Code: "model_unavailable", RequirementID: "transfer", OperatingCase: "nominal",
					Analysis: "dc_sweep", Metric: "voltage_gain", EvidenceHash: feedbackHash("diagnosis"),
				}},
			}},
		})
	}
	meta := CaseMeta{ID: "case-002", Role: RoleDiscovery, Domain: capabilityevaluation.DomainMixedSignal, SafetyImpact: capabilityevaluation.SafetyReviewRequired}
	observed, err := Observe(meta, requirement, run, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Gaps) != 1 || observed.Gaps[0].Scope != ScopeModel ||
		observed.Gaps[0].Capability != "dc_sweep_model" ||
		!reflect.DeepEqual(observed.Gaps[0].DownstreamSymptoms, []string{string(opentopologysynthesis.CodeRepairExhausted)}) {
		t.Fatalf("universal causal gap = %#v", observed.Gaps)
	}
}

func TestObserveCorrelatesPhysicalRootCauseWithoutRankingSymptoms(t *testing.T) {
	requirement := feedbackTestRequirement(t)
	run := feedbackTestRun(t, requirement, opentopologysynthesis.StatusPassed, opentopologysynthesis.StopPassed)
	requirementHash, err := opentopologysynthesis.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	promotion := opentopologysynthesis.PhysicalPromotionResult{
		Schema: opentopologysynthesis.PhysicalPromotionSchema, Version: opentopologysynthesis.PhysicalPromotionVersion,
		RequirementHash: requirementHash, SynthesisHash: run.Hash, Status: opentopologysynthesis.PhysicalPromotionFailed,
		Hash: feedbackHash("failed-promotion"),
		Issues: []reports.Issue{
			{IssueID: "root", Code: reports.CodeRouteCopperConflict, Severity: reports.SeverityBlocked, Stage: "routing", Message: "root"},
			{RootCauseID: "root", Code: reports.CodeDisconnectedPad, Severity: reports.SeverityError, Stage: "validation", Message: "symptom"},
		},
	}
	meta := CaseMeta{ID: "case-003", Role: RoleDiscovery, Domain: capabilityevaluation.DomainPower, SafetyImpact: capabilityevaluation.SafetyRelevant}
	observed, err := Observe(meta, requirement, run, &promotion)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Outcome != OutcomeUnsupported || len(observed.Gaps) != 1 || observed.Gaps[0].Scope != ScopeRouting ||
		!reflect.DeepEqual(observed.Gaps[0].DownstreamSymptoms, []string{string(reports.CodeDisconnectedPad)}) {
		t.Fatalf("physical causal gaps = %#v", observed)
	}
}

func feedbackTestRequirement(t *testing.T) opentopologysynthesis.Requirement {
	t.Helper()
	zero, one, two, fourPointSevenFive, five, fivePointTwoFive := 0.0, 1.0, 2.0, 4.75, 5.0, 5.25
	requirement := opentopologysynthesis.Requirement{
		Schema: opentopologysynthesis.RequirementSchema, Version: opentopologysynthesis.RequirementVersion,
		Project: opentopologysynthesis.Project{Name: "opaque_requirement", Title: "Opaque behavior requirement", Description: "A behavior-only transfer requirement used to verify generic capability feedback."},
		Requirements: opentopologysynthesis.Requirements{
			Domains: []opentopologysynthesis.Domain{
				{ID: "ground", Kind: "reference", NominalVoltageV: &zero, Source: "external"},
				{ID: "supply", Kind: "supply", MinVoltageV: &fourPointSevenFive, NominalVoltageV: &five, MaxVoltageV: &fivePointTwoFive, Source: "external"},
			},
			Ports: []opentopologysynthesis.Port{
				{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"},
				{ID: "power", Kind: "power", Direction: "sink", Domain: "supply"},
				{ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground", Electrical: opentopologysynthesis.Electrical{MinVoltageV: &zero, NominalVoltageV: &one, MaxVoltageV: &two}},
				{ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "ground", Electrical: opentopologysynthesis.Electrical{MinVoltageV: &zero, NominalVoltageV: &one, MaxVoltageV: &two}},
			},
			OperatingCases: []opentopologysynthesis.OperatingCase{{
				ID: "nominal", Conditions: []opentopologysynthesis.OperatingCondition{{Axis: "input_voltage", Target: "input", Min: 0, Max: 2, Unit: "V"}},
			}},
			BehavioralRequirements: []opentopologysynthesis.BehavioralAssertion{{
				ID: "transfer", Metric: "voltage_gain", Analysis: "dc_sweep",
				Excitation:  &opentopologysynthesis.Observation{Kind: "port", ID: "input"},
				Observation: opentopologysynthesis.Observation{Kind: "port", ID: "output"},
				Min:         &one, Max: &one, Unit: "ratio", OperatingCases: []string{"nominal"}, Critical: true,
			}},
			Constraints: opentopologysynthesis.BoardLimits{MaxComponents: 20, MaxWidthMM: 50, MaxHeightMM: 40},
		},
		Acceptance: opentopologysynthesis.Acceptance{
			RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true,
			RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true,
			RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true,
			RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true,
			RequireDeterministicReplay: true, RequireFailClosed: true,
		},
	}
	requirement = opentopologysynthesis.Normalize(requirement)
	if issues := opentopologysynthesis.Validate(requirement); len(issues) != 0 {
		t.Fatalf("test requirement issues: %#v", issues)
	}
	return requirement
}

func feedbackTestRun(t *testing.T, requirement opentopologysynthesis.Requirement, status opentopologysynthesis.Status, reason opentopologysynthesis.StopReason) opentopologysynthesis.SynthesisRun {
	t.Helper()
	requirementHash, err := opentopologysynthesis.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	run := opentopologysynthesis.SynthesisRun{
		Schema: opentopologysynthesis.SynthesisRunSchema, Version: opentopologysynthesis.SynthesisRunVersion,
		Report: opentopologysynthesis.Report{
			Schema: opentopologysynthesis.ReportSchema, Version: opentopologysynthesis.ReportVersion,
			PolicyVersion: opentopologysynthesis.PolicyVersion, PolicyHash: feedbackHash("policy"), RequirementHash: requirementHash,
			PrimitiveInventoryHash: feedbackHash("inventory"), CatalogHash: feedbackHash("catalog"), ModelRegistryHash: feedbackHash("models"),
			Status: status, StopReason: reason,
		},
		Hash: feedbackHash("synthesis-" + string(status) + "-" + string(reason)),
	}
	if status == opentopologysynthesis.StatusPassed {
		physicalHash := feedbackHash("physical-lowering")
		resistance := 1000.0
		graph := opentopologysynthesis.CandidateGraph{
			Schema: opentopologysynthesis.CandidateGraphSchema, Version: opentopologysynthesis.CandidateGraphVersion,
			Nodes: []opentopologysynthesis.GraphNode{
				{ID: "input", Scope: "external", SemanticKind: "port", SemanticID: "input"},
				{ID: "output", Scope: "external", SemanticKind: "port", SemanticID: "output"},
			},
			Instances: []opentopologysynthesis.GraphInstance{{
				ID: "r1", PrimitiveKey: "resistor", Kind: "resistor", ValueSI: &resistance,
				Terminals: []opentopologysynthesis.TerminalConnection{{Terminal: "1", Node: "input"}, {Terminal: "2", Node: "output"}},
			}},
		}
		graphHash, err := opentopologysynthesis.GraphHash(graph)
		if err != nil {
			t.Fatal(err)
		}
		topologyHash, err := opentopologysynthesis.TopologyHash(graph)
		if err != nil {
			t.Fatal(err)
		}
		run.Report.Selected = &opentopologysynthesis.SelectedResult{
			Fingerprint: "selected", TopologyHash: topologyHash,
			ActiveStructureHash: feedbackHash("active-structure"), EvaluationHash: feedbackHash("evaluation"),
			PhysicalHash: physicalHash,
		}
		run.SelectedGraph = &graph
		run.SelectedTrial = &opentopologysynthesis.ValueTrial{Number: 1, Hash: feedbackHash("value-trial")}
		run.Physical = &opentopologysynthesis.PhysicalLoweringResult{
			Schema: opentopologysynthesis.PhysicalLoweringSchema, Version: opentopologysynthesis.PhysicalLoweringVersion,
			PolicyVersion: opentopologysynthesis.PolicyVersion, RequirementHash: requirementHash,
			InventoryHash: run.Report.PrimitiveInventoryHash, GraphHash: graphHash,
			EvaluationHash: run.Report.Selected.EvaluationHash, Status: opentopologysynthesis.PhysicalLoweringReady,
			Hash: physicalHash,
		}
	}
	return run
}

func feedbackPassingPromotion(t *testing.T, requirement opentopologysynthesis.Requirement, run opentopologysynthesis.SynthesisRun) opentopologysynthesis.PhysicalPromotionResult {
	t.Helper()
	requirementHash, err := opentopologysynthesis.CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	projectHash := feedbackHash("project")
	return opentopologysynthesis.PhysicalPromotionResult{
		Schema: opentopologysynthesis.PhysicalPromotionSchema, Version: opentopologysynthesis.PhysicalPromotionVersion,
		RequirementHash: requirementHash, SynthesisHash: run.Hash, Status: opentopologysynthesis.PhysicalPromotionPassed,
		ReplayIdentical: true, ProjectHash: projectHash, Hash: feedbackHash("promotion"),
		Runs: []opentopologysynthesis.PhysicalPromotionRun{{Number: 1, ProjectHash: projectHash}, {Number: 2, ProjectHash: projectHash}},
	}
}

func feedbackHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
