package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/repairloop"
	"kicadai/internal/reports"
)

type repairProposal struct {
	graph  CandidateGraph
	repair Repair
}

func RepairCandidate(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) RepairSearchResult {
	policy = effectiveTopologyPolicy(policy)
	result := RepairSearchResult{
		Schema:                RepairSearchSchema,
		Version:               RepairSearchVersion,
		PolicyVersion:         PolicyVersion,
		InventoryHash:         inventory.Hash,
		InitialEvaluationHash: initial.Hash,
		Status:                RepairSearchFailed,
		Policy:                policy,
		Attempts:              []RepairAttempt{},
		Issues:                []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash repair requirement: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	result.RequirementHash = requirementHash
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize repair graph: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash repair graph: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	if initial.GraphHash != result.InitialGraphHash || initial.RequirementHash != requirementHash ||
		initial.InventoryHash != inventory.Hash || initial.Hash == "" {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation", "repair requires a hash-bound evaluation of the supplied graph, requirement, and inventory", "evaluate the exact candidate before repair")}
		return finalizeRepairSearch(result)
	}
	if initial.Status == SimulationEvaluationPassed {
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: graph, Repairs: []Repair{}, Evaluation: initial}
		return finalizeRepairSearch(result)
	}
	if len(initial.Diagnoses) == 0 {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation.diagnoses", "failed candidate has no stable diagnosis for generic repair", "retain normalized simulation diagnoses")}
		return finalizeRepairSearch(result)
	}
	result.traceDiagnoses = append([]Diagnosis(nil), initial.Diagnoses...)

	type repairState struct {
		graph      CandidateGraph
		evaluation SimulationEvaluation
		repairs    []Repair
		penalty    float64
		hash       string
	}
	frontier := []repairState{{
		graph: graph, evaluation: initial, repairs: []Repair{},
		penalty: simulationEvaluationPenalty(initial), hash: result.InitialGraphHash,
	}}
	seenGraphs := map[string]struct{}{result.InitialGraphHash: {}}
	generatedProposal := false
	for len(frontier) != 0 {
		slices.SortFunc(frontier, func(left, right repairState) int {
			return cmp.Or(cmp.Compare(left.penalty, right.penalty), cmp.Compare(left.hash, right.hash))
		})
		state := frontier[0]
		frontier = frontier[1:]
		proposals := generateRepairProposals(requirement, state.graph, state.evaluation.Diagnoses, inventory, policy)
		if len(proposals) == 0 {
			continue
		}
		generatedProposal = true
		for _, proposal := range proposals {
			if err := ctx.Err(); err != nil {
				result.Status = RepairSearchCanceled
				result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "open-topology repair canceled", "retry with an active context")}
				return finalizeRepairSearch(result)
			}
			if result.Consumption.TopologyRepairs >= policy.MaxTopologyRepairs ||
				result.Consumption.CandidateSimulations >= policy.MaxCandidateSimulations ||
				result.Consumption.CornerEvaluations >= policy.MaxCornerEvaluations ||
				result.Consumption.ValueTrials >= policy.MaxValueTrials {
				result.Consumption.BudgetExhausted = true
				break
			}
			result.Consumption.TopologyRepairs++
			proposal.repair.Number = result.Consumption.TopologyRepairs
			remainingValueTrials := policy.MaxValueTrials - result.Consumption.ValueTrials
			perRepairValueTrials := max(1, policy.MaxValueTrials/max(1, policy.MaxTopologyRepairs))
			trials := repairedGraphValueTrials(requirement, proposal.graph, inventory, min(remainingValueTrials, perRepairValueTrials), policy)
			for _, candidate := range trials {
				if result.Consumption.CandidateSimulations >= policy.MaxCandidateSimulations ||
					result.Consumption.CornerEvaluations >= policy.MaxCornerEvaluations {
					result.Consumption.BudgetExhausted = true
					break
				}
				if !repairGraphDeltaPreserved(
					state.graph,
					candidate.graph,
					proposal.repair,
				) {
					continue
				}
				candidateHash, hashErr := GraphHash(candidate.graph)
				if hashErr != nil {
					continue
				}
				if _, duplicate := seenGraphs[candidateHash]; duplicate {
					continue
				}
				seenGraphs[candidateHash] = struct{}{}
				if candidate.trial != nil {
					result.Consumption.ValueTrials++
				}
				evaluationPolicy := policy
				evaluationPolicy.MaxCandidateSimulations = policy.MaxCandidateSimulations - result.Consumption.CandidateSimulations
				evaluationPolicy.MaxCornerEvaluations = policy.MaxCornerEvaluations - result.Consumption.CornerEvaluations
				evaluation := EvaluateCandidate(ctx, requirement, candidate.graph, candidate.trial, inventory, environment, evaluationPolicy)
				result.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
				result.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
				topologyHash, _ := TopologyHash(candidate.graph)
				attempt := RepairAttempt{
					Number:       len(result.Attempts) + 1,
					Repair:       proposal.repair,
					ValueTrial:   candidate.trial,
					GraphHash:    candidateHash,
					TopologyHash: topologyHash,
					Evaluation:   evaluation,
					Improved:     simulationEvaluationPenalty(evaluation) < state.penalty,
					Status:       RepairSearchFailed,
				}
				if evaluation.Status == SimulationEvaluationPassed {
					attempt.Status = RepairSearchPassed
					result.Attempts = append(result.Attempts, attempt)
					result.Status = RepairSearchPassed
					repairs := append(append([]Repair(nil), state.repairs...), proposal.repair)
					result.Selected = &RepairedCandidate{
						Graph: candidate.graph, Repair: proposal.repair,
						Repairs: repairs, ValueTrial: candidate.trial, Evaluation: evaluation,
					}
					return finalizeRepairSearch(result)
				}
				if evaluation.Status == SimulationEvaluationCanceled {
					attempt.Status = RepairSearchCanceled
					result.Attempts = append(result.Attempts, attempt)
					result.Status = RepairSearchCanceled
					result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "open-topology repair canceled", "retry with an active context")}
					return finalizeRepairSearch(result)
				}
				if evaluation.Status == SimulationEvaluationUnsupported {
					attempt.Status = RepairSearchUnsupported
				} else if evaluation.Status == SimulationEvaluationExhausted {
					attempt.Status = RepairSearchExhausted
				}
				result.Attempts = append(result.Attempts, attempt)
				if attempt.Improved && evaluation.Status == SimulationEvaluationFailed {
					repairs := append(append([]Repair(nil), state.repairs...), proposal.repair)
					frontier = append(frontier, repairState{
						graph: candidate.graph, evaluation: evaluation, repairs: repairs,
						penalty: simulationEvaluationPenalty(evaluation), hash: candidateHash,
					})
				}
			}
			if result.Consumption.BudgetExhausted {
				break
			}
		}
		if len(frontier) > policy.MaxRetainedCandidates {
			slices.SortFunc(frontier, func(left, right repairState) int {
				return cmp.Or(cmp.Compare(left.penalty, right.penalty), cmp.Compare(left.hash, right.hash))
			})
			frontier = frontier[:policy.MaxRetainedCandidates]
		}
		if result.Consumption.BudgetExhausted {
			break
		}
	}
	result.Status = RepairSearchExhausted
	result.Issues = []reports.Issue{graphIssue(CodeRepairExhausted, "repair", "bounded generic repair did not produce a passing candidate", "inspect the strongest improved attempt or expand count budgets")}
	if (!generatedProposal || len(result.Attempts) == 0) && !result.Consumption.BudgetExhausted {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "repair", "all generated repairs collapsed to repeated or invalid graph states", "expand admissible generic repair operators")}
	}
	return finalizeRepairSearch(result)
}

func repairGraphDeltaPreserved(
	before CandidateGraph,
	after CandidateGraph,
	repair Repair,
) bool {
	if len(repair.Changes) == 0 {
		return false
	}
	beforeTopology, beforeErr := TopologyHash(before)
	afterTopology, afterErr := TopologyHash(after)
	for _, change := range repair.Changes {
		switch change.Kind {
		case "add_primitive", "redirect_terminal":
			if beforeErr != nil || afterErr != nil ||
				beforeTopology == afterTopology {
				return false
			}
		case "substitute_primitive":
			instanceIndex := graphInstanceIndex(after, change.Primitive)
			if instanceIndex < 0 ||
				after.Instances[instanceIndex].PrimitiveKey != change.ToNode {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type repairedValueCandidate struct {
	graph CandidateGraph
	trial *ValueTrial
}

func repairedGraphValueTrials(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	maximum int,
	policy Policy,
) []repairedValueCandidate {
	if maximum <= 0 {
		return nil
	}
	plan := BuildValueSearchPlan(requirement, graph, inventory, policy)
	if plan.Status != ValuePlanReady || len(plan.Domains) == 0 {
		return []repairedValueCandidate{{graph: graph}}
	}
	enumeration := EnumerateValueTrials(plan, maximum)
	result := make([]repairedValueCandidate, 0, len(enumeration.Trials))
	for index := range enumeration.Trials {
		trial := enumeration.Trials[index]
		candidateGraph, err := ApplyValueTrial(graph, trial, inventory)
		if err != nil {
			continue
		}
		result = append(result, repairedValueCandidate{graph: candidateGraph, trial: &trial})
	}
	return result
}

func generateRepairProposals(
	requirement Requirement,
	graph CandidateGraph,
	diagnoses []Diagnosis,
	inventory PrimitiveInventory,
	policy Policy,
) []repairProposal {
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      policy.MaxInternalNodes,
	}
	beforeHash, _ := GraphHash(graph)
	result := []repairProposal{}
	seen := map[string]struct{}{}
	add := func(candidate CandidateGraph, repair Repair) {
		if len(ValidateCompleteGraph(candidate, inventory, limits)) != 0 {
			return
		}
		afterHash, err := GraphHash(candidate)
		if err != nil || afterHash == beforeHash {
			return
		}
		if _, exists := seen[afterHash]; exists {
			return
		}
		seen[afterHash] = struct{}{}
		repair.BeforeGraphHash = beforeHash
		repair.AfterGraphHash = afterHash
		result = append(result, repairProposal{graph: candidate, repair: repair})
	}
	uniqueDiagnoses := compactRepairDiagnoses(diagnoses)
	requiredAnalyses := requirementAnalysisSet(requirement)
	for _, diagnosis := range uniqueDiagnoses {
		assertion, found := behavioralAssertionByID(requirement, diagnosis.RequirementID)
		if !found {
			continue
		}
		observationNode := observationNodeID(graph, requirement, assertion.Observation)
		targetNodes := repairTargetNodes(graph, requirement, assertion, observationNode)
		for _, kind := range repairPrimitiveKinds(assertion.Analysis) {
			primitive, found := firstRepairPrimitive(requirement, inventory, requiredAnalyses, kind)
			if !found {
				continue
			}
			for _, pair := range repairNodePairs(observationNode, targetNodes) {
				candidate, err := BridgeNodesWithPrimitive(graph, primitive, seedPrimitiveValue(primitive), pair[0], pair[1])
				if err != nil {
					continue
				}
				add(candidate, Repair{
					Operator:               "add_passive_edge",
					DiagnosisCode:          diagnosis.Code,
					DiagnosisRequirementID: diagnosis.RequirementID,
					DiagnosisEvidenceHash:  diagnosis.EvidenceHash,
					ExpectedDirection:      diagnosis.Direction,
					Changes: []GraphChange{{
						Kind: "add_primitive", Primitive: primitive.Key,
						FromNode: pair[0], ToNode: pair[1], ToValue: seedPrimitiveValue(primitive),
					}},
				})
			}
		}
		for _, instance := range graph.Instances {
			primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
			if !found || len(primitive.Terminals) != 2 || !repairPassiveKind(instance.Kind) {
				continue
			}
			for _, connection := range instance.Terminals {
				for _, target := range targetNodes {
					if target == connection.Node {
						continue
					}
					candidate, err := RedirectPrimitiveTerminal(graph, inventory, instance.ID, connection.Terminal, target)
					if err != nil {
						continue
					}
					add(candidate, Repair{
						Operator:               "redirect_passive_edge",
						DiagnosisCode:          diagnosis.Code,
						DiagnosisRequirementID: diagnosis.RequirementID,
						DiagnosisEvidenceHash:  diagnosis.EvidenceHash,
						ExpectedDirection:      diagnosis.Direction,
						Changes: []GraphChange{{
							Kind: "redirect_terminal", Primitive: instance.ID, Terminal: connection.Terminal,
							FromNode: connection.Node, ToNode: target,
						}},
					})
				}
			}
		}
	}
	for _, instance := range graph.Instances {
		current, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		for _, replacement := range inventory.Primitives {
			if replacement.Key == current.Key || replacement.Kind != current.Kind ||
				!samePrimitiveTerminalContract(current, replacement) ||
				!primitiveCoversAllAnalyses(replacement, requiredAnalyses) ||
				!ratingsCoverRequirement(requirement, replacement) {
				continue
			}
			candidate, err := SubstitutePrimitive(graph, inventory, instance.ID, replacement.Key)
			if err != nil {
				continue
			}
			diagnosis := uniqueDiagnoses[0]
			add(candidate, Repair{
				Operator:               "substitute_compatible_primitive",
				DiagnosisCode:          diagnosis.Code,
				DiagnosisRequirementID: diagnosis.RequirementID,
				DiagnosisEvidenceHash:  diagnosis.EvidenceHash,
				ExpectedDirection:      diagnosis.Direction,
				Changes: []GraphChange{{
					Kind: "substitute_primitive", Primitive: instance.ID,
					FromNode: current.Key, ToNode: replacement.Key,
				}},
			})
			break
		}
	}
	slices.SortFunc(result, compareRepairProposals)
	return result
}

func compactRepairDiagnoses(diagnoses []Diagnosis) []Diagnosis {
	result := append([]Diagnosis(nil), diagnoses...)
	slices.SortFunc(result, compareDiagnoses)
	result = slices.CompactFunc(result, func(left, right Diagnosis) bool {
		return left.Code == right.Code && left.RequirementID == right.RequirementID &&
			left.Analysis == right.Analysis && left.Direction == right.Direction
	})
	return result
}

func behavioralAssertionByID(requirement Requirement, id string) (BehavioralAssertion, bool) {
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID == id {
			return assertion, true
		}
	}
	return BehavioralAssertion{}, false
}

func repairPrimitiveKinds(analysis string) []string {
	switch trustedModelAnalysisKind(analysis) {
	case "ac_sweep", "noise", "stability", "distortion", "transient", "startup":
		return []string{"capacitor", "resistor"}
	case "electrothermal":
		return []string{"diode", "capacitor", "resistor"}
	default:
		return []string{"resistor"}
	}
}

func firstRepairPrimitive(
	requirement Requirement,
	inventory PrimitiveInventory,
	requiredAnalyses map[string]bool,
	kind string,
) (PrimitiveCandidate, bool) {
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != kind || len(primitive.Terminals) != 2 ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		return primitive, true
	}
	return PrimitiveCandidate{}, false
}

func repairPassiveKind(kind string) bool {
	return kind == "resistor" || kind == "capacitor" || kind == "inductor" || kind == "diode"
}

func repairTargetNodes(
	graph CandidateGraph,
	requirement Requirement,
	assertion BehavioralAssertion,
	observationNode string,
) []string {
	result := []string{}
	if assertion.Excitation != nil {
		if node := observationNodeID(graph, requirement, *assertion.Excitation); node != "" && node != observationNode {
			result = append(result, node)
		}
	}
	for _, role := range []string{"reference", "supply", "input", "control"} {
		for _, node := range graph.Nodes {
			if node.Scope == "external" && node.Role == role && node.ID != observationNode {
				result = append(result, node.ID)
			}
		}
	}
	// Internal signal nodes are admissible repair endpoints. Limiting repair
	// targets to external ports prevents a failed active stage from acquiring
	// the feedback, bias, or compensation edge identified by its simulation
	// diagnosis. Graph limits and complete-graph validation still bound every
	// generated proposal, and canonical sorting keeps the expansion stable.
	for _, node := range graph.Nodes {
		if node.Scope == "internal" && node.ID != observationNode {
			result = append(result, node.ID)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func repairNodePairs(observationNode string, targets []string) [][2]string {
	if observationNode == "" {
		return nil
	}
	result := make([][2]string, 0, len(targets))
	for _, target := range targets {
		if target != observationNode {
			result = append(result, [2]string{observationNode, target})
		}
	}
	return result
}

func compareRepairProposals(left, right repairProposal) int {
	operatorRank := func(operator string) int {
		switch operator {
		case "add_passive_edge":
			return 0
		case "redirect_passive_edge":
			return 1
		default:
			return 2
		}
	}
	return cmp.Or(
		cmp.Compare(operatorRank(left.repair.Operator), operatorRank(right.repair.Operator)),
		cmp.Compare(left.repair.DiagnosisCode, right.repair.DiagnosisCode),
		cmp.Compare(repairChangeKey(left.repair.Changes), repairChangeKey(right.repair.Changes)),
		cmp.Compare(left.repair.AfterGraphHash, right.repair.AfterGraphHash),
	)
}

func repairChangeKey(changes []GraphChange) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		value := ""
		if change.ToValue != nil {
			value = fmt.Sprintf("%.12g", *change.ToValue)
		}
		parts = append(parts, strings.Join([]string{
			change.Kind, change.Primitive, change.Terminal,
			change.FromNode, change.ToNode, value,
		}, "\x1f"))
	}
	return strings.Join(parts, "\x1e")
}

func simulationEvaluationPenalty(evaluation SimulationEvaluation) float64 {
	switch evaluation.Status {
	case SimulationEvaluationPassed:
		return 0
	case SimulationEvaluationCanceled:
		return math.Inf(1)
	case SimulationEvaluationUnsupported, SimulationEvaluationExhausted:
		return 1e15 + float64(len(evaluation.Diagnoses))
	}
	penalty := float64(len(evaluation.Diagnoses)) * 1e6
	for _, diagnosis := range evaluation.Diagnoses {
		if diagnosis.Actual == nil {
			penalty += 1e5
			continue
		}
		scale := math.Max(1, math.Abs(*diagnosis.Actual))
		if diagnosis.RequiredMin != nil && *diagnosis.Actual < *diagnosis.RequiredMin {
			scale = math.Max(scale, math.Abs(*diagnosis.RequiredMin))
			penalty += (*diagnosis.RequiredMin - *diagnosis.Actual) / scale
		}
		if diagnosis.RequiredMax != nil && *diagnosis.Actual > *diagnosis.RequiredMax {
			scale = math.Max(scale, math.Abs(*diagnosis.RequiredMax))
			penalty += (*diagnosis.Actual - *diagnosis.RequiredMax) / scale
		}
	}
	return penalty
}

func finalizeRepairSearch(result RepairSearchResult) RepairSearchResult {
	result.Trace = electricalRepairTrace(result)
	copy := result
	copy.Hash = ""
	result.Hash = hashJSON(copy)
	return result
}

func electricalRepairTrace(result RepairSearchResult) repairloop.Trace {
	diagnostics := make([]repairloop.Diagnostic, 0, len(result.traceDiagnoses))
	byDiagnosis := map[string]repairloop.Diagnostic{}
	for _, diagnosis := range compactRepairDiagnoses(result.traceDiagnoses) {
		evidenceHash := diagnosis.EvidenceHash
		if evidenceHash == "" {
			evidenceHash = hashJSON(diagnosis)
		}
		scope := []string{diagnosis.RequirementID, diagnosis.OperatingCase, diagnosis.AffectedConeHash}
		normalized := repairloop.NewDiagnostic(
			"simulation", diagnosis.Code, electricalRepairCategory(diagnosis),
			diagnosis.Direction, evidenceHash, scope,
		)
		diagnostics = append(diagnostics, normalized)
		byDiagnosis[electricalRepairDiagnosisKey(diagnosis.Code, diagnosis.RequirementID, diagnosis.EvidenceHash)] = normalized
	}
	proposals := []repairloop.Proposal{}
	outcomes := []repairloop.Outcome{}
	covered := map[string]bool{}
	for _, attempt := range result.Attempts {
		diagnostic, ok := byDiagnosis[electricalRepairDiagnosisKey(attempt.Repair.DiagnosisCode, attempt.Repair.DiagnosisRequirementID, attempt.Repair.DiagnosisEvidenceHash)]
		if !ok {
			continue
		}
		covered[diagnostic.Hash] = true
		scope := []string{"candidate:" + attempt.Repair.AfterGraphHash}
		for _, change := range attempt.Repair.Changes {
			scope = append(scope, change.Primitive, change.Terminal, change.FromNode, change.ToNode)
		}
		effect := strings.TrimSpace(attempt.Repair.ExpectedDirection)
		if effect == "" {
			effect = "resolve the diagnosed simulation failure without weakening declared constraints"
		}
		proposal := repairloop.NewProposal(diagnostic, attempt.Repair.Operator, "equation_sizing", effect, scope, true, "")
		proposals = append(proposals, proposal)
		status := "failed"
		reason := "candidate did not satisfy all trusted assertions"
		if attempt.Status == RepairSearchPassed {
			status, reason = "passed", "all trusted assertions passed"
		} else if attempt.Improved {
			status, reason = "improved", "normalized simulation penalty decreased"
		} else if attempt.Status == RepairSearchUnsupported {
			status, reason = "rejected", "candidate evaluation was unsupported"
		}
		outcomes = append(outcomes, repairloop.Outcome{
			ProposalID: proposal.ID, Status: status,
			BeforeHash: attempt.Repair.BeforeGraphHash, AfterHash: attempt.GraphHash,
			ResultHash: attempt.Evaluation.Hash, Reason: reason,
		})
	}
	for _, diagnostic := range diagnostics {
		if covered[diagnostic.Hash] {
			continue
		}
		reason := electricalRepairRejection(diagnostic.Category)
		proposal := repairloop.NewProposal(diagnostic, "no_safe_operator", "simulation", "retain fail-closed evidence", diagnostic.Scope, false, reason)
		proposals = append(proposals, proposal)
		outcomes = append(outcomes, repairloop.Outcome{ProposalID: proposal.ID, Status: "rejected", BeforeHash: result.InitialGraphHash, ResultHash: result.InitialEvaluationHash, Reason: reason})
	}
	return repairloop.NewTrace(result.Policy.MaxTopologyRepairs, result.Consumption.TopologyRepairs, diagnostics, proposals, outcomes)
}

func electricalRepairDiagnosisKey(code, requirementID, evidenceHash string) string {
	return strings.Join([]string{code, requirementID, evidenceHash}, "\x1f")
}

func electricalRepairCategory(diagnosis Diagnosis) string {
	switch diagnosis.Code {
	case diagnosisNonconvergent, diagnosisOperatingPointInvalid:
		return "bias_or_reference_access"
	case diagnosisUnstable:
		return "feedback_or_compensation"
	case diagnosisAssertionBelowMinimum, diagnosisAssertionAboveMaximum:
		switch trustedModelAnalysisKind(diagnosis.Analysis) {
		case "thermal", "electrothermal":
			return "rating_thermal_or_soa"
		default:
			return "value_domain_or_feedback"
		}
	case diagnosisThermalUnavailable:
		return "thermal_evidence"
	case diagnosisModelUnavailable, diagnosisMetricUnsupported:
		return "model_evidence"
	default:
		return "unsupported_simulation_diagnostic"
	}
}

func electricalRepairRejection(category string) string {
	switch category {
	case "model_evidence", "thermal_evidence":
		return "repair cannot synthesize missing reviewed model or thermal evidence"
	case "unsupported_simulation_diagnostic":
		return "diagnostic does not identify a bounded electrical repair operator"
	default:
		return "no admissible graph change survived graph, rating, and deterministic-budget validation"
	}
}
