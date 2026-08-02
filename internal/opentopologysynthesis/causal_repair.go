package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	causalMaximumSingleTrials = 48
	causalMaximumBeamWidth    = 8
	causalMaximumChanges      = 2
	causalEpsilon             = 1e-12
)

type causalCandidate struct {
	graph         CandidateGraph
	perturbations []CausalPerturbation
	repair        Repair
	coordinated   bool
}

type causalEvaluatedCandidate struct {
	graph CandidateGraph
	trial CausalRepairTrial
}

func analyzeCausalRepairs(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) (CausalRepairAnalysis, []causalEvaluatedCandidate) {
	graphHash, _ := GraphHash(graph)
	trialBudget := minPositive(
		minPositive(causalMaximumSingleTrials, policy.MaxValueTrials),
		policy.MaxCandidateSimulations,
	)
	if trialBudget < 0 {
		trialBudget = 0
	}
	coordinatedBudget := min(
		max(0, trialBudget/4),
		max(0, policy.MaxTopologyRepairs/2),
	)
	analysis := CausalRepairAnalysis{
		Schema:                CausalRepairSchema,
		Version:               CausalRepairVersion,
		PolicyVersion:         PolicyVersion,
		RequirementHash:       initial.RequirementHash,
		InventoryHash:         inventory.Hash,
		InitialGraphHash:      graphHash,
		InitialEvaluationHash: initial.Hash,
		Budget: CausalRepairBudget{
			Trials:               trialBudget,
			ValueTrials:          policy.MaxValueTrials,
			TopologyTrials:       policy.MaxTopologyRepairs,
			CoordinatedTrials:    coordinatedBudget,
			MaximumChanges:       causalMaximumChanges,
			CandidateSimulations: policy.MaxCandidateSimulations,
			CornerEvaluations:    policy.MaxCornerEvaluations,
		},
		Trials: []CausalRepairTrial{},
		Status: "no_safe_improvement",
	}
	if trialBudget == 0 || initial.Hash == "" {
		analysis.Consumption.BudgetExhausted = trialBudget == 0
		return finalizeCausalRepairAnalysis(analysis), nil
	}

	candidates := generateCausalCandidates(requirement, graph, initial, inventory, policy)
	evaluated := make([]causalEvaluatedCandidate, 0, min(len(candidates), trialBudget))
	seen := map[string]struct{}{graphHash: {}}
	evaluate := func(candidate causalCandidate) bool {
		if err := ctx.Err(); err != nil {
			analysis.Status = "canceled"
			return false
		}
		singleTrialLimit := max(0, analysis.Budget.Trials-analysis.Budget.CoordinatedTrials)
		if !candidate.coordinated && analysis.Consumption.Trials >= singleTrialLimit {
			return true
		}
		if candidate.coordinated && analysis.Consumption.CoordinatedTrials >= analysis.Budget.CoordinatedTrials {
			return true
		}
		if analysis.Consumption.Trials >= analysis.Budget.Trials ||
			analysis.Consumption.CandidateSimulations >= analysis.Budget.CandidateSimulations ||
			analysis.Consumption.CornerEvaluations >= analysis.Budget.CornerEvaluations {
			analysis.Consumption.BudgetExhausted = true
			return false
		}
		usesTopology := causalCandidateUsesTopology(candidate)
		if usesTopology && analysis.Consumption.TopologyTrials >= analysis.Budget.TopologyTrials {
			return true
		}
		if !usesTopology && analysis.Consumption.ValueTrials >= analysis.Budget.ValueTrials {
			return true
		}
		candidateHash, err := GraphHash(candidate.graph)
		if err != nil {
			return true
		}
		if _, duplicate := seen[candidateHash]; duplicate {
			return true
		}
		seen[candidateHash] = struct{}{}
		remaining := policy
		remaining.MaxCandidateSimulations = max(0, analysis.Budget.CandidateSimulations-analysis.Consumption.CandidateSimulations)
		remaining.MaxCornerEvaluations = max(0, analysis.Budget.CornerEvaluations-analysis.Consumption.CornerEvaluations)
		evaluation := EvaluateCandidate(ctx, requirement, candidate.graph, nil, inventory, environment, remaining)
		analysis.Consumption.Trials++
		if usesTopology {
			analysis.Consumption.TopologyTrials++
		} else {
			analysis.Consumption.ValueTrials++
		}
		if candidate.coordinated {
			analysis.Consumption.CoordinatedTrials++
		}
		analysis.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
		analysis.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
		trial := causalTrialEvidence(requirement, initial, evaluation, candidate, candidateHash)
		trial.Number = len(analysis.Trials) + 1
		trial.Hash = causalTrialHash(trial)
		analysis.Trials = append(analysis.Trials, trial)
		evaluated = append(evaluated, causalEvaluatedCandidate{graph: candidate.graph, trial: trial})
		return true
	}

	for _, candidate := range candidates {
		if !evaluate(candidate) {
			break
		}
	}

	if analysis.Consumption.Trials < analysis.Budget.Trials &&
		analysis.Consumption.CoordinatedTrials < analysis.Budget.CoordinatedTrials {
		for _, candidate := range coordinatedCausalCandidates(graph, evaluated, analysis.Budget.CoordinatedTrials) {
			if !evaluate(candidate) {
				break
			}
		}
	}

	rankCausalTrials(analysis.Trials)
	byHash := make(map[string]CausalRepairTrial, len(analysis.Trials))
	for _, trial := range analysis.Trials {
		byHash[trial.Hash] = trial
		if trial.Rank == 1 && trial.Authorized {
			analysis.SelectedTrialHash = trial.Hash
			if trial.Status == SimulationEvaluationPassed {
				analysis.Status = "passing_repair_found"
			} else {
				analysis.Status = "safe_improvement_found"
			}
		}
	}
	for index := range evaluated {
		evaluated[index].trial = byHash[evaluated[index].trial.Hash]
	}
	slices.SortFunc(evaluated, func(left, right causalEvaluatedCandidate) int {
		return cmp.Or(
			cmp.Compare(left.trial.Rank, right.trial.Rank),
			cmp.Compare(left.trial.Hash, right.trial.Hash),
		)
	})
	return finalizeCausalRepairAnalysis(analysis), evaluated
}

func generateCausalCandidates(
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	policy Policy,
) []causalCandidate {
	result := causalValueCandidates(requirement, graph, inventory, policy)
	result = append(result, causalPolarityCandidates(graph, inventory, initial.Diagnoses)...)
	for _, proposal := range generateRepairProposals(requirement, graph, initial.Diagnoses, inventory, policy) {
		repair := proposal.repair
		if repair.Operator == "substitute_compatible_primitive" {
			continue
		}
		repair.Operator = causalOperatorForProposal(repair, initial.Diagnoses)
		result = append(result, sizedCausalProposalCandidates(
			requirement, graph, proposal.graph, repair, inventory, policy,
		)...)
	}
	slices.SortFunc(result, compareCausalCandidates)
	return diversifyCausalCandidates(compactCausalCandidates(result))
}

func sizedCausalProposalCandidates(
	requirement Requirement,
	before CandidateGraph,
	proposed CandidateGraph,
	repair Repair,
	inventory PrimitiveInventory,
	policy Policy,
) []causalCandidate {
	maximum := max(1, min(4, policy.MaxValueTrials))
	sized := repairedGraphValueTrials(requirement, proposed, inventory, maximum, policy)
	sized = append([]repairedValueCandidate{{graph: proposed}}, sized...)
	result := make([]causalCandidate, 0, len(sized))
	seen := map[string]struct{}{}
	for _, candidate := range sized {
		candidateHash, err := GraphHash(candidate.graph)
		if err != nil {
			continue
		}
		if _, duplicate := seen[candidateHash]; duplicate {
			continue
		}
		seen[candidateHash] = struct{}{}
		candidateRepair := repair
		valueChanges := causalValueChanges(proposed, candidate.graph)
		candidateRepair.Changes = append(append([]GraphChange(nil), repair.Changes...), valueChanges...)
		candidateRepair.AfterGraphHash, _ = GraphHash(candidate.graph)
		perturbations := causalPerturbationsForChanges(before, repair.Changes)
		perturbations = append(perturbations, causalPerturbationsForChanges(proposed, valueChanges)...)
		if len(perturbations) == 0 || len(perturbations) > causalMaximumChanges {
			continue
		}
		result = append(result, causalCandidate{
			graph: candidate.graph, perturbations: perturbations, repair: candidateRepair,
		})
	}
	return result
}

func causalValueChanges(before, after CandidateGraph) []GraphChange {
	result := []GraphChange{}
	for _, instance := range after.Instances {
		beforeIndex := graphInstanceIndex(before, instance.ID)
		if beforeIndex < 0 {
			continue
		}
		original := before.Instances[beforeIndex]
		if sameGraphInstanceValue(original, instance) {
			continue
		}
		kind := "set_value"
		if original.PrimitiveKey != instance.PrimitiveKey {
			kind = "substitute_primitive"
		}
		result = append(result, GraphChange{
			Kind: kind, Primitive: instance.ID,
			FromNode: original.PrimitiveKey, ToNode: instance.PrimitiveKey,
			FromValue: cloneInventoryFloat(original.ValueSI), ToValue: cloneInventoryFloat(instance.ValueSI),
		})
	}
	slices.SortFunc(result, func(left, right GraphChange) int {
		return cmp.Or(cmp.Compare(left.Primitive, right.Primitive), cmp.Compare(left.Kind, right.Kind))
	})
	return result
}

func sameGraphInstanceValue(left, right GraphInstance) bool {
	if left.PrimitiveKey != right.PrimitiveKey {
		return false
	}
	if left.ValueSI == nil || right.ValueSI == nil {
		return left.ValueSI == nil && right.ValueSI == nil
	}
	return math.Float64bits(*left.ValueSI) == math.Float64bits(*right.ValueSI)
}

func causalValueCandidates(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	policy Policy,
) []causalCandidate {
	plan := BuildValueSearchPlan(requirement, graph, inventory, policy)
	if plan.Status != ValuePlanReady {
		return nil
	}
	result := []causalCandidate{}
	for _, domain := range plan.Domains {
		instanceIndex := graphInstanceIndex(graph, domain.InstanceID)
		if instanceIndex < 0 {
			continue
		}
		instance := graph.Instances[instanceIndex]
		for _, candidate := range domain.Candidates {
			if sameCausalValue(instance, candidate) {
				continue
			}
			selection := ValueTrialSelection{
				InstanceID: domain.InstanceID, PrimitiveKey: candidate.PrimitiveKey,
				ValueSI: cloneInventoryFloat(candidate.ValueSI), CandidateHash: candidate.Hash,
			}
			trial := ValueTrial{Number: 1, Selections: []ValueTrialSelection{selection}}
			trial.Hash = valueTrialHash(trial.Selections)
			candidateGraph, err := ApplyValueTrial(graph, trial, inventory)
			if err != nil {
				continue
			}
			kind := "adjust_value"
			operator := "adjust_component_value"
			changeKind := "set_value"
			if candidate.PrimitiveKey != instance.PrimitiveKey {
				changeKind = "substitute_primitive"
			}
			if candidate.PrimitiveKey != instance.PrimitiveKey && domain.Quantity == "" {
				kind = "substitute_rated_device"
				operator = "substitute_compatible_primitive"
			}
			perturbation := newCausalPerturbation(CausalPerturbation{
				Kind: kind, InstanceID: instance.ID,
				FromPrimitiveKey: instance.PrimitiveKey, ToPrimitiveKey: candidate.PrimitiveKey,
				FromValue: cloneInventoryFloat(instance.ValueSI), ToValue: cloneInventoryFloat(candidate.ValueSI),
				Magnitude: causalValueMagnitude(instance.ValueSI, candidate.ValueSI, instance.PrimitiveKey != candidate.PrimitiveKey),
			})
			beforeHash, _ := GraphHash(graph)
			afterHash, _ := GraphHash(candidateGraph)
			result = append(result, causalCandidate{
				graph:         candidateGraph,
				perturbations: []CausalPerturbation{perturbation},
				repair: Repair{
					Operator: operator, BeforeGraphHash: beforeHash, AfterGraphHash: afterHash,
					Changes: []GraphChange{{
						Kind: changeKind, Primitive: instance.ID,
						FromNode: instance.PrimitiveKey, ToNode: candidate.PrimitiveKey,
						FromValue: cloneInventoryFloat(instance.ValueSI), ToValue: cloneInventoryFloat(candidate.ValueSI),
					}},
				},
			})
		}
	}
	return result
}

func causalPolarityCandidates(
	graph CandidateGraph,
	inventory PrimitiveInventory,
	diagnoses []Diagnosis,
) []causalCandidate {
	result := []causalCandidate{}
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		for _, pair := range causalPolarityTerminalPairs(primitive) {
			leftIndex, rightIndex := -1, -1
			for index, terminal := range instance.Terminals {
				switch terminal.Terminal {
				case pair[0]:
					leftIndex = index
				case pair[1]:
					rightIndex = index
				}
			}
			if leftIndex < 0 || rightIndex < 0 || instance.Terminals[leftIndex].Node == instance.Terminals[rightIndex].Node {
				continue
			}
			candidate := CloneGraph(graph)
			candidateIndex := graphInstanceIndex(candidate, instance.ID)
			leftNode := candidate.Instances[candidateIndex].Terminals[leftIndex].Node
			rightNode := candidate.Instances[candidateIndex].Terminals[rightIndex].Node
			candidate.Instances[candidateIndex].Terminals[leftIndex].Node = rightNode
			candidate.Instances[candidateIndex].Terminals[rightIndex].Node = leftNode
			candidate, err := NormalizeGraph(candidate)
			if err != nil {
				continue
			}
			beforeHash, _ := GraphHash(graph)
			afterHash, _ := GraphHash(candidate)
			diagnosis := firstRepairDiagnosis(diagnoses)
			changes := []GraphChange{
				{Kind: "redirect_terminal", Primitive: instance.ID, Terminal: pair[0], FromNode: leftNode, ToNode: rightNode},
				{Kind: "redirect_terminal", Primitive: instance.ID, Terminal: pair[1], FromNode: rightNode, ToNode: leftNode},
			}
			perturbations := causalPerturbationsForChanges(graph, changes)
			for index := range perturbations {
				perturbations[index].Kind = "correct_polarity"
				perturbations[index] = newCausalPerturbation(perturbations[index])
			}
			result = append(result, causalCandidate{
				graph:         candidate,
				perturbations: perturbations,
				repair: Repair{
					Operator:      "correct_feedback_polarity",
					DiagnosisCode: diagnosis.Code, DiagnosisRequirementID: diagnosis.RequirementID,
					DiagnosisEvidenceHash: diagnosis.EvidenceHash,
					BeforeGraphHash:       beforeHash, AfterGraphHash: afterHash,
					ExpectedDirection: diagnosis.Direction, Changes: changes,
				},
			})
		}
	}
	return result
}

func causalPolarityTerminalPairs(primitive PrimitiveCandidate) [][2]string {
	present := map[string]bool{}
	for _, terminal := range primitive.Terminals {
		present[strings.ToUpper(strings.TrimSpace(terminal.Terminal))] = true
	}
	candidates := [][2]string{{"IN_PLUS", "IN_MINUS"}, {"PLUS", "MINUS"}, {"POSITIVE", "NEGATIVE"}}
	result := [][2]string{}
	for _, pair := range candidates {
		if present[pair[0]] && present[pair[1]] {
			result = append(result, pair)
		}
	}
	return result
}

func causalOperatorForProposal(repair Repair, diagnoses []Diagnosis) string {
	category := ""
	for _, diagnosis := range diagnoses {
		if diagnosis.Code == repair.DiagnosisCode && diagnosis.RequirementID == repair.DiagnosisRequirementID {
			category = electricalRepairCategory(diagnosis)
			break
		}
	}
	switch {
	case repair.Operator == "substitute_compatible_primitive":
		return "substitute_rated_device"
	case category == "bias_or_reference_access":
		return "repair_bias_reference"
	case category == "feedback_or_compensation" && repair.Operator == "add_passive_edge":
		return "add_compensation_edge"
	case category == "feedback_or_compensation":
		return "repair_feedback_edge"
	default:
		return repair.Operator
	}
}

func coordinatedCausalCandidates(
	base CandidateGraph,
	evaluated []causalEvaluatedCandidate,
	maximum int,
) []causalCandidate {
	if maximum <= 0 {
		return nil
	}
	eligible := append([]causalEvaluatedCandidate(nil), evaluated...)
	slices.SortStableFunc(eligible, func(left, right causalEvaluatedCandidate) int {
		return cmp.Or(
			cmp.Compare(causalTrialStatusRank(left.trial), causalTrialStatusRank(right.trial)),
			cmp.Compare(right.trial.Improvement, left.trial.Improvement),
			cmp.Compare(right.trial.Sensitivity, left.trial.Sensitivity),
			cmp.Compare(left.trial.ChangeMagnitude, right.trial.ChangeMagnitude),
			cmp.Compare(left.trial.Hash, right.trial.Hash),
		)
	})
	byInstance := map[string][]causalEvaluatedCandidate{}
	instanceOrder := []string{}
	for _, entry := range eligible {
		if !entry.trial.Authorized || entry.trial.Improvement <= causalEpsilon ||
			len(entry.trial.Perturbations) != 1 ||
			(entry.trial.Perturbations[0].Kind != "adjust_value" && entry.trial.Perturbations[0].Kind != "substitute_rated_device") {
			continue
		}
		instanceID := entry.trial.Perturbations[0].InstanceID
		if _, found := byInstance[instanceID]; !found {
			instanceOrder = append(instanceOrder, instanceID)
		}
		byInstance[instanceID] = append(byInstance[instanceID], entry)
	}
	slices.Sort(instanceOrder)
	beam := []causalEvaluatedCandidate{}
	for depth := 0; len(beam) < causalMaximumBeamWidth; depth++ {
		appended := false
		for _, instanceID := range instanceOrder {
			if depth >= len(byInstance[instanceID]) {
				continue
			}
			beam = append(beam, byInstance[instanceID][depth])
			appended = true
			if len(beam) >= causalMaximumBeamWidth {
				break
			}
		}
		if !appended {
			break
		}
	}
	result := []causalCandidate{}
	seenPairs := map[string]struct{}{}
	for left := 0; left < len(beam); left++ {
		for right := left + 1; right < len(beam); right++ {
			leftPerturbation := beam[left].trial.Perturbations[0]
			rightPerturbation := beam[right].trial.Perturbations[0]
			if leftPerturbation.InstanceID == rightPerturbation.InstanceID {
				continue
			}
			pairKey := strings.Join([]string{leftPerturbation.Hash, rightPerturbation.Hash}, "\x1f")
			if _, duplicate := seenPairs[pairKey]; duplicate {
				continue
			}
			seenPairs[pairKey] = struct{}{}
			candidate := CloneGraph(base)
			changes := []GraphChange{}
			valid := true
			for _, perturbation := range []CausalPerturbation{leftPerturbation, rightPerturbation} {
				index := graphInstanceIndex(candidate, perturbation.InstanceID)
				if index < 0 {
					valid = false
					break
				}
				if perturbation.ToPrimitiveKey != "" && candidate.Instances[index].PrimitiveKey != perturbation.ToPrimitiveKey {
					candidate.Instances[index].PrimitiveKey = perturbation.ToPrimitiveKey
				}
				candidate.Instances[index].ValueSI = cloneInventoryFloat(perturbation.ToValue)
				kind := "set_value"
				if perturbation.Kind == "substitute_rated_device" {
					kind = "substitute_primitive"
				}
				changes = append(changes, GraphChange{
					Kind: kind, Primitive: perturbation.InstanceID,
					FromNode: perturbation.FromPrimitiveKey, ToNode: perturbation.ToPrimitiveKey,
					FromValue: cloneInventoryFloat(perturbation.FromValue), ToValue: cloneInventoryFloat(perturbation.ToValue),
				})
			}
			if !valid {
				continue
			}
			candidate, err := NormalizeGraph(candidate)
			if err != nil {
				continue
			}
			beforeHash, _ := GraphHash(base)
			afterHash, _ := GraphHash(candidate)
			result = append(result, causalCandidate{
				graph: candidate, coordinated: true,
				perturbations: []CausalPerturbation{leftPerturbation, rightPerturbation},
				repair: Repair{
					Operator: "coordinate_component_values", BeforeGraphHash: beforeHash,
					AfterGraphHash: afterHash, Changes: changes,
				},
			})
			if len(result) >= maximum {
				return result
			}
		}
	}
	return result
}

func causalTrialEvidence(
	requirement Requirement,
	baseline SimulationEvaluation,
	evaluation SimulationEvaluation,
	candidate causalCandidate,
	graphHash string,
) CausalRepairTrial {
	magnitude := 0.0
	for _, perturbation := range candidate.perturbations {
		magnitude += math.Max(causalEpsilon, perturbation.Magnitude)
	}
	effects, regressions, baselineViolation, trialViolation := causalAssertionEffects(
		requirement, baseline, evaluation, magnitude,
	)
	improvement := baselineViolation - trialViolation
	authorized := improvement > causalEpsilon && len(regressions) == 0 &&
		evaluation.Status != SimulationEvaluationUnsupported &&
		evaluation.Status != SimulationEvaluationCanceled
	rejection := ""
	if !authorized {
		switch {
		case len(regressions) != 0:
			rejection = "trial degrades a previously passing assertion, safety requirement, or operating corner"
		case evaluation.Status == SimulationEvaluationUnsupported:
			rejection = "trial loses required reviewed simulation evidence"
		case improvement <= causalEpsilon:
			rejection = "trial does not reduce normalized requirement violation"
		default:
			rejection = "trial is not an admissible safe improvement"
		}
	}
	repair := candidate.repair
	if len(baseline.Diagnoses) != 0 && repair.DiagnosisCode == "" {
		diagnosis := baseline.Diagnoses[0]
		repair.DiagnosisCode = diagnosis.Code
		repair.DiagnosisRequirementID = diagnosis.RequirementID
		repair.DiagnosisEvidenceHash = diagnosis.EvidenceHash
		repair.ExpectedDirection = diagnosis.Direction
	}
	return CausalRepairTrial{
		Perturbations: candidate.perturbations, GraphHash: graphHash,
		EvaluationHash: evaluation.Hash, Status: evaluation.Status,
		Effects: effects, BaselineViolation: baselineViolation, TrialViolation: trialViolation,
		Improvement: improvement, Sensitivity: improvement / math.Max(causalEpsilon, magnitude),
		ChangeMagnitude: magnitude, Regressions: regressions,
		Authorized: authorized, Rejection: rejection, Coordinated: candidate.coordinated,
		Repair: repair, Evaluation: evaluation,
	}
}

func causalAssertionEffects(
	requirement Requirement,
	baseline SimulationEvaluation,
	trial SimulationEvaluation,
	magnitude float64,
) ([]CausalAssertionEffect, []string, float64, float64) {
	baselineAttempts := causalAttemptMap(baseline.Attempts)
	trialAttempts := causalAttemptMap(trial.Attempts)
	keys := make([]string, 0, len(baselineAttempts)+len(trialAttempts))
	for key := range baselineAttempts {
		keys = append(keys, key)
	}
	for key := range trialAttempts {
		if _, exists := baselineAttempts[key]; !exists {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	effects := make([]CausalAssertionEffect, 0, len(keys))
	regressions := []string{}
	baselineViolationTotal := 0.0
	trialViolationTotal := 0.0
	for _, key := range keys {
		baselineAttempt, baselineFound := baselineAttempts[key]
		trialAttempt, trialFound := trialAttempts[key]
		effect := CausalAssertionEffect{}
		if baselineFound {
			effect.RequirementID = baselineAttempt.RequirementID
			effect.OperatingCase = baselineAttempt.OperatingCase
			effect.CornerID = baselineAttempt.CornerID
			effect.Analysis = baselineAttempt.Analysis
			effect.Metric = baselineAttempt.Metric
			effect.Critical = critical[baselineAttempt.RequirementID]
			effect.BaselinePass = baselineAttempt.AssertionPass
			effect.BaselineViolation = causalAttemptViolation(baselineAttempt)
			effect.BaselineMargin = causalAttemptMargin(baselineAttempt)
		} else {
			effect.RequirementID = trialAttempt.RequirementID
			effect.OperatingCase = trialAttempt.OperatingCase
			effect.CornerID = trialAttempt.CornerID
			effect.Analysis = trialAttempt.Analysis
			effect.Metric = trialAttempt.Metric
			effect.Critical = critical[trialAttempt.RequirementID]
		}
		if trialFound {
			effect.TrialPass = trialAttempt.AssertionPass
			effect.TrialViolation = causalAttemptViolation(trialAttempt)
			effect.TrialMargin = causalAttemptMargin(trialAttempt)
		} else {
			effect.TrialViolation = 1
			effect.TrialMargin = -1
		}
		effect.ViolationDelta = effect.BaselineViolation - effect.TrialViolation
		effect.MarginDelta = effect.TrialMargin - effect.BaselineMargin
		effect.Sensitivity = effect.ViolationDelta / math.Max(causalEpsilon, magnitude)
		switch {
		case baselineFound && baselineAttempt.AssertionPass && !trialFound:
			effect.Regression = true
			effect.Reason = "previously evaluated passing corner is missing"
		case baselineFound && baselineAttempt.AssertionPass && !trialAttempt.AssertionPass:
			effect.Regression = true
			effect.Reason = "previously passing assertion now fails"
		case baselineFound && baselineAttempt.AssertionPass && effect.Critical && effect.MarginDelta < -causalEpsilon:
			effect.Regression = true
			effect.Reason = "passing assertion margin decreased"
		case !baselineFound && trialFound && !trialAttempt.AssertionPass:
			effect.Regression = true
			effect.Reason = "trial introduced a new failed assertion or corner"
		}
		if effect.Regression {
			regressions = append(regressions, key+":"+effect.Reason)
		}
		baselineViolationTotal += effect.BaselineViolation
		trialViolationTotal += effect.TrialViolation
		effects = append(effects, effect)
	}
	return effects, regressions, baselineViolationTotal, trialViolationTotal
}

func causalAttemptMap(attempts []SimulationAttempt) map[string]SimulationAttempt {
	result := make(map[string]SimulationAttempt, len(attempts))
	for _, attempt := range attempts {
		result[causalAttemptKey(attempt)] = attempt
	}
	return result
}

func causalAttemptKey(attempt SimulationAttempt) string {
	return strings.Join([]string{
		attempt.RequirementID, attempt.OperatingCase, attempt.CornerID,
		attempt.Analysis, attempt.Metric,
	}, "\x1f")
}

func causalAttemptViolation(attempt SimulationAttempt) float64 {
	if attempt.AssertionPass {
		return 0
	}
	if attempt.Actual == nil {
		return 1
	}
	scale := math.Max(1, math.Abs(*attempt.Actual))
	violation := 0.0
	if attempt.RequiredMin != nil && *attempt.Actual < *attempt.RequiredMin {
		scale = math.Max(scale, math.Abs(*attempt.RequiredMin))
		violation += (*attempt.RequiredMin - *attempt.Actual) / scale
	}
	if attempt.RequiredMax != nil && *attempt.Actual > *attempt.RequiredMax {
		scale = math.Max(scale, math.Abs(*attempt.RequiredMax))
		violation += (*attempt.Actual - *attempt.RequiredMax) / scale
	}
	if violation == 0 {
		return 1
	}
	return violation
}

func causalAttemptMargin(attempt SimulationAttempt) float64 {
	if !attempt.AssertionPass {
		return -causalAttemptViolation(attempt)
	}
	if attempt.Actual == nil {
		return 0
	}
	scale := math.Max(1, math.Abs(*attempt.Actual))
	margin := math.Inf(1)
	if attempt.RequiredMin != nil {
		scale = math.Max(scale, math.Abs(*attempt.RequiredMin))
		margin = math.Min(margin, (*attempt.Actual-*attempt.RequiredMin)/scale)
	}
	if attempt.RequiredMax != nil {
		scale = math.Max(scale, math.Abs(*attempt.RequiredMax))
		margin = math.Min(margin, (*attempt.RequiredMax-*attempt.Actual)/scale)
	}
	if math.IsInf(margin, 1) {
		return 0
	}
	return margin
}

func rankCausalTrials(trials []CausalRepairTrial) {
	slices.SortStableFunc(trials, func(left, right CausalRepairTrial) int {
		return cmp.Or(
			cmp.Compare(causalTrialStatusRank(left), causalTrialStatusRank(right)),
			cmp.Compare(len(left.Perturbations), len(right.Perturbations)),
			cmp.Compare(left.ChangeMagnitude, right.ChangeMagnitude),
			cmp.Compare(right.Sensitivity, left.Sensitivity),
			cmp.Compare(left.Hash, right.Hash),
		)
	})
	for index := range trials {
		trials[index].Rank = index + 1
	}
}

func causalTrialStatusRank(trial CausalRepairTrial) int {
	if trial.Authorized && trial.Status == SimulationEvaluationPassed {
		return 0
	}
	if trial.Authorized {
		return 1
	}
	return 2
}

func compareCausalCandidates(left, right causalCandidate) int {
	return cmp.Or(
		cmp.Compare(causalCandidateRank(left), causalCandidateRank(right)),
		cmp.Compare(len(left.perturbations), len(right.perturbations)),
		cmp.Compare(causalCandidateMagnitude(left), causalCandidateMagnitude(right)),
		cmp.Compare(causalPerturbationKey(left.perturbations), causalPerturbationKey(right.perturbations)),
		cmp.Compare(left.repair.AfterGraphHash, right.repair.AfterGraphHash),
	)
}

func causalCandidateMagnitude(candidate causalCandidate) float64 {
	result := 0.0
	for _, perturbation := range candidate.perturbations {
		result += perturbation.Magnitude
	}
	return result
}

func causalCandidateRank(candidate causalCandidate) int {
	if len(candidate.perturbations) == 0 {
		return 99
	}
	switch candidate.perturbations[0].Kind {
	case "adjust_value":
		return 0
	case "correct_polarity":
		return 1
	case "add_primitive":
		return 2
	case "redirect_terminal":
		return 3
	case "substitute_rated_device":
		return 4
	default:
		return 5
	}
}

func diversifyCausalCandidates(source []causalCandidate) []causalCandidate {
	groups := map[string][]causalCandidate{}
	order := []string{}
	for _, candidate := range source {
		key := "unknown"
		if len(candidate.perturbations) != 0 {
			perturbation := candidate.perturbations[0]
			key = perturbation.Kind
			if perturbation.Kind == "adjust_value" || perturbation.Kind == "substitute_rated_device" {
				key += "\x1f" + perturbation.InstanceID
			}
		}
		if _, found := groups[key]; !found {
			order = append(order, key)
		}
		groups[key] = append(groups[key], candidate)
	}
	slices.SortFunc(order, func(left, right string) int {
		leftRank, rightRank := 99, 99
		if len(groups[left]) != 0 {
			leftRank = causalCandidateRank(groups[left][0])
		}
		if len(groups[right]) != 0 {
			rightRank = causalCandidateRank(groups[right][0])
		}
		return cmp.Or(cmp.Compare(leftRank, rightRank), cmp.Compare(left, right))
	})
	result := make([]causalCandidate, 0, len(source))
	for index := 0; len(result) < len(source); index++ {
		appended := false
		for _, key := range order {
			if index >= len(groups[key]) {
				continue
			}
			result = append(result, groups[key][index])
			appended = true
		}
		if !appended {
			break
		}
	}
	return result
}

func causalCandidateUsesTopology(candidate causalCandidate) bool {
	for _, change := range candidate.repair.Changes {
		if change.Kind == "add_primitive" || change.Kind == "redirect_terminal" {
			return true
		}
	}
	return false
}

func compactCausalCandidates(source []causalCandidate) []causalCandidate {
	result := make([]causalCandidate, 0, len(source))
	seen := map[string]struct{}{}
	for _, candidate := range source {
		hash, err := GraphHash(candidate.graph)
		if err != nil {
			continue
		}
		if _, duplicate := seen[hash]; duplicate {
			continue
		}
		seen[hash] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func causalPerturbationsForChanges(graph CandidateGraph, changes []GraphChange) []CausalPerturbation {
	result := make([]CausalPerturbation, 0, len(changes))
	for _, change := range changes {
		kind := change.Kind
		if kind == "redirect_terminal" {
			kind = "redirect_terminal"
		}
		if kind == "substitute_primitive" {
			kind = "substitute_rated_device"
		}
		result = append(result, newCausalPerturbation(CausalPerturbation{
			Kind: kind, InstanceID: change.Primitive, Terminal: change.Terminal,
			FromNode: change.FromNode, ToNode: change.ToNode,
			FromPrimitiveKey: change.FromNode, ToPrimitiveKey: change.ToNode,
			FromValue: cloneInventoryFloat(change.FromValue), ToValue: cloneInventoryFloat(change.ToValue),
			Magnitude: causalGraphChangeMagnitude(change),
		}))
	}
	return result
}

func newCausalPerturbation(perturbation CausalPerturbation) CausalPerturbation {
	copy := perturbation
	copy.Hash = ""
	perturbation.Hash = hashJSON(copy)
	return perturbation
}

func causalGraphChangeMagnitude(change GraphChange) float64 {
	if change.FromValue != nil || change.ToValue != nil {
		return causalValueMagnitude(change.FromValue, change.ToValue, change.Kind == "substitute_primitive")
	}
	return 1
}

func causalValueMagnitude(from, to *float64, substituted bool) float64 {
	magnitude := 0.0
	if from == nil || to == nil {
		magnitude = 1
	} else if *from == 0 || *to == 0 {
		magnitude = math.Abs(*to - *from)
	} else {
		magnitude = math.Abs(math.Log(math.Abs(*to / *from)))
	}
	if substituted {
		magnitude += 1
	}
	return math.Max(causalEpsilon, magnitude)
}

func sameCausalValue(instance GraphInstance, candidate ComponentValueCandidate) bool {
	if instance.PrimitiveKey != candidate.PrimitiveKey {
		return false
	}
	if instance.ValueSI == nil || candidate.ValueSI == nil {
		return instance.ValueSI == nil && candidate.ValueSI == nil
	}
	return math.Float64bits(*instance.ValueSI) == math.Float64bits(*candidate.ValueSI)
}

func causalPerturbationKey(perturbations []CausalPerturbation) string {
	parts := make([]string, len(perturbations))
	for index, perturbation := range perturbations {
		parts[index] = perturbation.Hash
	}
	return strings.Join(parts, "\x1f")
}

func firstRepairDiagnosis(diagnoses []Diagnosis) Diagnosis {
	if len(diagnoses) == 0 {
		return Diagnosis{}
	}
	result := append([]Diagnosis(nil), diagnoses...)
	slices.SortFunc(result, compareDiagnoses)
	return result[0]
}

func causalTrialHash(trial CausalRepairTrial) string {
	copy := trial
	copy.Number = 0
	copy.Rank = 0
	copy.Hash = ""
	return hashJSON(copy)
}

func finalizeCausalRepairAnalysis(analysis CausalRepairAnalysis) CausalRepairAnalysis {
	copy := analysis
	copy.Hash = ""
	analysis.Hash = hashJSON(copy)
	return analysis
}

func validateCausalRepairAnalysis(analysis CausalRepairAnalysis) error {
	if analysis.Schema != CausalRepairSchema || analysis.Version != CausalRepairVersion || analysis.Hash == "" ||
		analysis.PolicyVersion == "" || analysis.RequirementHash == "" || analysis.InventoryHash == "" ||
		analysis.InitialGraphHash == "" || analysis.InitialEvaluationHash == "" {
		return fmt.Errorf("invalid causal repair identity")
	}
	if analysis.Budget.Trials < 0 || analysis.Budget.ValueTrials < 0 || analysis.Budget.TopologyTrials < 0 ||
		analysis.Budget.CoordinatedTrials < 0 || analysis.Budget.MaximumChanges < 0 ||
		analysis.Budget.CandidateSimulations < 0 || analysis.Budget.CornerEvaluations < 0 {
		return fmt.Errorf("invalid causal repair budget")
	}
	if analysis.Consumption.Trials > analysis.Budget.Trials ||
		analysis.Consumption.ValueTrials > analysis.Budget.ValueTrials ||
		analysis.Consumption.TopologyTrials > analysis.Budget.TopologyTrials ||
		analysis.Consumption.CoordinatedTrials > analysis.Budget.CoordinatedTrials ||
		analysis.Consumption.CandidateSimulations > analysis.Budget.CandidateSimulations ||
		analysis.Consumption.CornerEvaluations > analysis.Budget.CornerEvaluations {
		return fmt.Errorf("causal repair consumption exceeds budget")
	}
	if analysis.Consumption.Trials != len(analysis.Trials) {
		return fmt.Errorf("causal repair trial consumption mismatch")
	}
	numbers := make(map[int]struct{}, len(analysis.Trials))
	ranks := make(map[int]struct{}, len(analysis.Trials))
	selectedFound := false
	valueTrials, topologyTrials, coordinatedTrials := 0, 0, 0
	candidateSimulations, cornerEvaluations := 0, 0
	for _, trial := range analysis.Trials {
		if trial.Number < 1 || trial.Number > len(analysis.Trials) {
			return fmt.Errorf("invalid causal trial number")
		}
		if _, duplicate := numbers[trial.Number]; duplicate {
			return fmt.Errorf("duplicate causal trial number")
		}
		numbers[trial.Number] = struct{}{}
		if trial.Rank < 1 || trial.Rank > len(analysis.Trials) {
			return fmt.Errorf("invalid causal trial rank")
		}
		if _, duplicate := ranks[trial.Rank]; duplicate {
			return fmt.Errorf("duplicate causal trial rank")
		}
		ranks[trial.Rank] = struct{}{}
		if trial.Hash == "" || trial.Hash != causalTrialHash(trial) || trial.GraphHash == "" ||
			trial.EvaluationHash == "" || trial.Evaluation.Hash != trial.EvaluationHash ||
			trial.Evaluation.GraphHash != trial.GraphHash || len(trial.Effects) == 0 ||
			len(trial.Perturbations) == 0 || len(trial.Perturbations) > analysis.Budget.MaximumChanges {
			return fmt.Errorf("incomplete causal trial evidence")
		}
		for _, perturbation := range trial.Perturbations {
			copy := perturbation
			copy.Hash = ""
			if perturbation.Hash == "" || hashJSON(copy) != perturbation.Hash || perturbation.Magnitude <= 0 {
				return fmt.Errorf("invalid causal perturbation evidence")
			}
		}
		if trial.Authorized && (len(trial.Regressions) != 0 || trial.Rejection != "") {
			return fmt.Errorf("regressing or rejected causal trial was authorized")
		}
		if !trial.Authorized && trial.Rejection == "" {
			return fmt.Errorf("unauthorized causal trial lacks rejection evidence")
		}
		usesTopology := false
		for _, change := range trial.Repair.Changes {
			if change.Kind == "add_primitive" || change.Kind == "redirect_terminal" {
				usesTopology = true
				break
			}
		}
		if usesTopology {
			topologyTrials++
		} else {
			valueTrials++
		}
		if trial.Coordinated {
			coordinatedTrials++
			if len(trial.Perturbations) < 2 {
				return fmt.Errorf("coordinated causal trial lacks multiple perturbations")
			}
		}
		candidateSimulations += trial.Evaluation.Consumption.CandidateSimulations
		cornerEvaluations += trial.Evaluation.Consumption.CornerEvaluations
		if trial.Hash == analysis.SelectedTrialHash {
			if !trial.Authorized || trial.Rank != 1 {
				return fmt.Errorf("selected causal trial is not the authorized top-ranked trial")
			}
			selectedFound = true
		}
	}
	if valueTrials != analysis.Consumption.ValueTrials || topologyTrials != analysis.Consumption.TopologyTrials ||
		coordinatedTrials != analysis.Consumption.CoordinatedTrials ||
		candidateSimulations != analysis.Consumption.CandidateSimulations ||
		cornerEvaluations != analysis.Consumption.CornerEvaluations {
		return fmt.Errorf("causal repair detailed consumption mismatch")
	}
	if analysis.SelectedTrialHash != "" && !selectedFound {
		return fmt.Errorf("selected causal trial is missing")
	}
	if (analysis.Status == "passing_repair_found" || analysis.Status == "safe_improvement_found") != selectedFound {
		return fmt.Errorf("causal repair status and selection disagree")
	}
	copy := analysis
	copy.Hash = ""
	if hashJSON(copy) != analysis.Hash {
		return fmt.Errorf("causal repair hash mismatch")
	}
	return nil
}
