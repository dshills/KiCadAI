package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

const valueCandidatesPerInstance = architecturesearch.DefaultMaxValueCandidates

func BuildValueSearchPlan(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	policy Policy,
) ValueSearchPlan {
	result := ValueSearchPlan{
		Schema:        ValueSearchPlanSchema,
		Version:       ValueSearchPlanVersion,
		PolicyVersion: PolicyVersion,
		InventoryHash: inventory.Hash,
		Policy:        effectiveTopologyPolicy(policy),
		Status:        ValuePlanFailed,
		Domains:       []InstanceValueDomain{},
		Rejections:    []SearchRejection{},
		Issues:        []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash open-topology requirement: "+err.Error(), "")}
		return result
	}
	result.RequirementHash = requirementHash
	if issues := Validate(requirement); len(issues) != 0 {
		result.Issues = issues
		return result
	}
	if len(inventory.Primitives) == 0 || len(inventory.Hash) != 64 {
		result.Status = ValuePlanUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "inventory", "value search requires a nonempty hash-bound primitive inventory", "build the reviewed primitive inventory")}
		return result
	}
	normalizedGraph, err := NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize value-search graph: "+err.Error(), "")}
		return result
	}
	result.GraphHash, err = GraphHash(normalizedGraph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash value-search graph: "+err.Error(), "")}
		return result
	}
	limits := GraphLimits{
		MaxPrimitiveInstances: minPositive(result.Policy.MaxPrimitiveInstances, requirement.Requirements.Constraints.MaxComponents),
		MaxInternalNodes:      result.Policy.MaxInternalNodes,
	}
	if issues := ValidateCompleteGraph(normalizedGraph, inventory, limits); len(issues) != 0 {
		result.Issues = issues
		return result
	}
	if issues := validateGraphRequirementBinding(normalizedGraph, requirement); len(issues) != 0 {
		result.Issues = issues
		return result
	}

	rejections := map[string][]string{}
	requiredAnalyses := requirementAnalysisSet(requirement)
	inventoryByKey := primitiveInventoryByKey(inventory)
	for _, instance := range normalizedGraph.Instances {
		original, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			rejections["primitive_missing"] = append(rejections["primitive_missing"], instance.PrimitiveKey)
			continue
		}
		domain := InstanceValueDomain{
			InstanceID:           instance.ID,
			OriginalPrimitiveKey: instance.PrimitiveKey,
			PrimitiveKind:        instance.Kind,
			Candidates:           []ComponentValueCandidate{},
		}
		domain.AnalyticScales = append(
			deriveAnalyticScales(requirement, original),
			deriveTopologyAnalyticScales(
				requirement,
				normalizedGraph,
				instance,
				inventoryByKey,
			)...,
		)
		slices.SortFunc(domain.AnalyticScales, compareAnalyticScales)
		domain.AnalyticScales = compactAnalyticScales(domain.AnalyticScales)
		if original.ValueDomain != nil {
			domain.Quantity = original.ValueDomain.Kind
			domain.Unit = original.ValueDomain.Unit
		}
		variants := compatibleValueVariants(original, inventory, requiredAnalyses)
		for _, variant := range variants {
			candidates, variantRejections := valueCandidatesForVariant(
				requirement,
				variant,
				domain.AnalyticScales,
				valueCandidatesPerInstance,
			)
			for code, samples := range variantRejections {
				for _, sample := range samples {
					rejections[code] = append(rejections[code], instance.ID+":"+sample)
				}
			}
			domain.Candidates = append(domain.Candidates, candidates...)
		}
		slices.SortFunc(domain.Candidates, compareComponentValueCandidates)
		domain.Candidates = compactValueCandidates(domain.Candidates)
		if original.ValueDomain == nil {
			domain.Candidates = prioritizeOriginalFixedCandidate(
				domain.Candidates,
				instance.PrimitiveKey,
			)
		}
		if len(domain.Candidates) > valueCandidatesPerInstance {
			domain.Candidates = selectDiverseValueCandidates(
				domain.Candidates,
				valueCandidatesPerInstance,
			)
		}
		for index := range domain.Candidates {
			domain.Candidates[index].Rank = index + 1
		}
		if len(domain.Candidates) == 0 {
			rejections["value_domain_empty"] = append(rejections["value_domain_empty"], instance.ID)
		}
		result.CandidateValues += len(domain.Candidates)
		result.Domains = append(result.Domains, domain)
	}
	slices.SortFunc(result.Domains, func(left, right InstanceValueDomain) int {
		return cmp.Compare(left.InstanceID, right.InstanceID)
	})
	result.Rejections = normalizeSearchRejections(rejections)
	for _, domain := range result.Domains {
		if len(domain.Candidates) == 0 {
			result.Status = ValuePlanExhausted
			result.Issues = []reports.Issue{graphIssue(CodeValueExhausted, "domains."+domain.InstanceID, "no catalog-valid value or variant remains in the bounded domain", "onboard a compatible rated variant or widen the explicit behavioral envelope")}
			return result
		}
	}
	if len(result.Domains) == 0 {
		result.Status = ValuePlanUnsupported
		result.Issues = []reports.Issue{graphIssue(CodePrimitiveUnavailable, "graph.instances", "value search graph contains no primitive instances", "")}
		return result
	}
	result.Status = ValuePlanReady
	return result
}

func prioritizeOriginalFixedCandidate(
	candidates []ComponentValueCandidate,
	originalKey string,
) []ComponentValueCandidate {
	result := append([]ComponentValueCandidate(nil), candidates...)
	for index, candidate := range result {
		if candidate.PrimitiveKey != originalKey {
			continue
		}
		copy(result[1:index+1], result[0:index])
		result[0] = candidate
		break
	}
	return result
}

func validateGraphRequirementBinding(graph CandidateGraph, requirement Requirement) []reports.Issue {
	expected := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		expected[port.ID] = port
	}
	seen := map[string]bool{}
	issues := []reports.Issue{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" {
			continue
		}
		port, found := expected[node.SemanticID]
		if !found {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "external graph node is not declared by the behavioral requirement", "rebuild the graph from the normalized requirement"))
			continue
		}
		if seen[node.SemanticID] {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "behavioral port is bound more than once", "bind each semantic port exactly once"))
		}
		seen[node.SemanticID] = true
		if node.Domain != port.Domain || node.Role != graphRoleForPort(port) {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "external graph-node domain or role differs from its behavioral port", "rebuild the graph from the normalized requirement"))
		}
	}
	for portID := range expected {
		if !seen[portID] {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "requirements.ports."+portID, "behavioral port is absent from the candidate graph", "connect every required external interface"))
		}
	}
	return reports.SortedIssues(issues)
}

func EnumerateValueTrials(plan ValueSearchPlan, maximum int) ValueTrialEnumeration {
	result := ValueTrialEnumeration{Trials: []ValueTrial{}}
	if plan.Status != ValuePlanReady || maximum <= 0 || len(plan.Domains) == 0 {
		return result
	}
	result.TotalCombinations = 1
	for _, domain := range plan.Domains {
		if len(domain.Candidates) == 0 {
			result.TotalCombinations = 0
			return result
		}
		if result.TotalCombinations > math.MaxUint64/uint64(len(domain.Candidates)) {
			result.TotalCombinations = math.MaxUint64
		} else {
			result.TotalCombinations *= uint64(len(domain.Candidates))
		}
	}
	indices := make([]int, len(plan.Domains))
	maximumRank := 0
	for _, domain := range plan.Domains {
		maximumRank += len(domain.Candidates) - 1
	}
	appendTrial := func() {
		selections := make([]ValueTrialSelection, len(plan.Domains))
		for index, domain := range plan.Domains {
			candidate := domain.Candidates[indices[index]]
			selections[index] = ValueTrialSelection{
				InstanceID:    domain.InstanceID,
				PrimitiveKey:  candidate.PrimitiveKey,
				ValueSI:       cloneInventoryFloat(candidate.ValueSI),
				CandidateHash: candidate.Hash,
			}
		}
		trial := ValueTrial{
			Number:     len(result.Trials) + 1,
			Selections: selections,
		}
		trial.Hash = valueTrialHash(trial.Selections)
		result.Trials = append(result.Trials, trial)
	}
	var enumerateRank func(int, int)
	enumerateRank = func(index, remaining int) {
		if len(result.Trials) >= maximum {
			return
		}
		if index == len(plan.Domains) {
			if remaining == 0 {
				appendTrial()
			}
			return
		}
		domain := plan.Domains[index]
		for candidateIndex := 0; candidateIndex < len(domain.Candidates) && candidateIndex <= remaining; candidateIndex++ {
			indices[index] = candidateIndex
			enumerateRank(index+1, remaining-candidateIndex)
			if len(result.Trials) >= maximum {
				return
			}
		}
	}
	for rank := 0; rank <= maximumRank && len(result.Trials) < maximum; rank++ {
		enumerateRank(0, rank)
	}
	result.Exhausted = result.TotalCombinations > uint64(len(result.Trials))
	return result
}

func ApplyValueTrial(
	graph CandidateGraph,
	trial ValueTrial,
	inventory PrimitiveInventory,
) (CandidateGraph, error) {
	result := CloneGraph(graph)
	for _, selection := range trial.Selections {
		instanceIndex := graphInstanceIndex(result, selection.InstanceID)
		if instanceIndex < 0 {
			return graph, fmt.Errorf("%w: %s", ErrGraphInstanceNotFound, selection.InstanceID)
		}
		if result.Instances[instanceIndex].PrimitiveKey != selection.PrimitiveKey {
			var err error
			result, err = SubstitutePrimitive(result, inventory, selection.InstanceID, selection.PrimitiveKey)
			if err != nil {
				return graph, err
			}
			instanceIndex = graphInstanceIndex(result, selection.InstanceID)
		}
		primitive, found := primitiveByKey(inventory, selection.PrimitiveKey)
		if !found {
			return graph, fmt.Errorf("%w: %s", ErrGraphPrimitiveNotFound, selection.PrimitiveKey)
		}
		if primitive.ValueDomain == nil {
			if selection.ValueSI != nil {
				return graph, fmt.Errorf("fixed primitive %s received a value", selection.PrimitiveKey)
			}
			result.Instances[instanceIndex].ValueSI = nil
			continue
		}
		if selection.ValueSI == nil || !valueWithinPrimitiveDomain(*selection.ValueSI, *primitive.ValueDomain) {
			return graph, fmt.Errorf("value trial for %s is outside the catalog domain", selection.PrimitiveKey)
		}
		result.Instances[instanceIndex].ValueSI = cloneInventoryFloat(selection.ValueSI)
	}
	return result, nil
}

func compatibleValueVariants(
	original PrimitiveCandidate,
	inventory PrimitiveInventory,
	requiredAnalyses map[string]bool,
) []PrimitiveCandidate {
	result := []PrimitiveCandidate{}
	for _, candidate := range inventory.Primitives {
		if candidate.Kind != original.Kind ||
			!samePrimitiveTerminalContract(original, candidate) ||
			!primitiveCoversAllAnalyses(candidate, requiredAnalyses) {
			continue
		}
		if (candidate.ValueDomain == nil) != (original.ValueDomain == nil) {
			continue
		}
		if candidate.ValueDomain != nil &&
			(candidate.ValueDomain.Kind != original.ValueDomain.Kind ||
				candidate.ValueDomain.Unit != original.ValueDomain.Unit) {
			continue
		}
		result = append(result, candidate)
	}
	slices.SortFunc(result, func(left, right PrimitiveCandidate) int {
		return cmp.Or(
			cmp.Compare(primitiveEvidencePenalty(left.Evidence), primitiveEvidencePenalty(right.Evidence)),
			comparePositiveArea(left.AreaMM2, right.AreaMM2),
			cmp.Compare(left.Key, right.Key),
		)
	})
	return result
}

func primitiveCoversAllAnalyses(primitive PrimitiveCandidate, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}
	covered := map[string]bool{}
	for analysis := range required {
		for _, model := range primitive.Models {
			if reviewedModelSupportsCircuitAnalysis(model.AllowedAnalyses, analysis) {
				covered[analysis] = true
				break
			}
		}
		if !covered[analysis] {
			return false
		}
	}
	return true
}

func requirementAnalysisSet(requirement Requirement) map[string]bool {
	result := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		result[trustedModelAnalysisKind(assertion.Analysis)] = true
	}
	return result
}

func valueCandidatesForVariant(
	requirement Requirement,
	primitive PrimitiveCandidate,
	scales []AnalyticScale,
	limit int,
) ([]ComponentValueCandidate, map[string][]string) {
	rejections := map[string][]string{}
	if !ratingsCoverRequirement(requirement, primitive) {
		rejections["rating_envelope"] = append(rejections["rating_envelope"], primitive.Key)
		return nil, rejections
	}
	modelHashes := primitiveModelEvidenceHashes(primitive)
	if primitive.ValueDomain == nil {
		candidate := ComponentValueCandidate{
			PrimitiveKey:         primitive.Key,
			Derivation:           "fixed catalog primitive",
			CatalogEvidence:      primitive.Evidence,
			ModelEvidenceSHA256s: modelHashes,
			RatingEvidence:       append([]PrimitiveBound(nil), primitive.Ratings...),
		}
		candidate.Hash = componentValueCandidateHash(candidate)
		return []ComponentValueCandidate{candidate}, rejections
	}
	domain := *primitive.ValueDomain
	minimum, maximum, ok := effectiveValueRange(domain)
	if !ok {
		rejections["invalid_value_range"] = append(rejections["invalid_value_range"], primitive.Key)
		return nil, rejections
	}
	tolerance, toleranceProven := primitiveTolerancePercent(primitive, domain.Kind)
	if requirement.Acceptance.RequireAllCorners && !toleranceProven {
		rejections["tolerance_unproven"] = append(rejections["tolerance_unproven"], primitive.Key)
		return nil, rejections
	}
	series := preferredSeriesForDomain(domain.Kind, tolerance, toleranceProven)
	type idealSeed struct {
		value      float64
		derivation string
		priority   int
	}
	seeds := []idealSeed{}
	if domain.Nominal != nil && *domain.Nominal > 0 {
		seeds = append(seeds, idealSeed{value: *domain.Nominal, derivation: "catalog nominal", priority: 0})
	}
	for _, scale := range scales {
		if scale.Kind == domain.Kind && scale.Unit == domain.Unit && scale.ValueSI > 0 {
			seeds = append(seeds, idealSeed{value: scale.ValueSI, derivation: scale.ID, priority: scale.Priority})
		}
	}
	seeds = append(seeds, idealSeed{value: geometricMean(minimum, maximum), derivation: "catalog range geometric mean", priority: 80})
	if minimum == maximum {
		seeds = []idealSeed{{value: minimum, derivation: "fixed catalog value", priority: 0}}
	}
	slices.SortFunc(seeds, func(left, right idealSeed) int {
		return cmp.Or(
			cmp.Compare(left.priority, right.priority),
			cmp.Compare(left.value, right.value),
			cmp.Compare(left.derivation, right.derivation),
		)
	})
	candidates := []ComponentValueCandidate{}
	for _, seed := range seeds {
		ideal := seed.value
		if ideal < minimum {
			ideal = minimum
		}
		if ideal > maximum {
			ideal = maximum
		}
		values := []float64{ideal}
		if minimum != maximum {
			preferred, issues := architecturesearch.PreferredValueCandidates(
				ideal,
				series,
				minimum,
				maximum,
				minPositive(8, architecturesearch.DefaultMaxValueCandidates),
			)
			if len(issues) != 0 {
				rejections["preferred_value_unavailable"] = append(rejections["preferred_value_unavailable"], primitive.Key+":"+seed.derivation)
				continue
			}
			values = preferred
		}
		for _, value := range values {
			cornerMinimum := value * (1 - tolerance/100)
			cornerMaximum := value * (1 + tolerance/100)
			idealCopy := seed.value
			analyticPriority, relativeError, analyticID, analyticallyRanked :=
				analyticScaleFit(scales, domain.Kind, domain.Unit, value)
			if !analyticallyRanked {
				analyticPriority = 100
				relativeError = math.Abs(value-seed.value) / seed.value
			}
			valueCopy := value
			cornerMinimumCopy := cornerMinimum
			cornerMaximumCopy := cornerMaximum
			derivation := seed.derivation
			if analyticallyRanked {
				derivation += "; ranked against " + analyticID
			}
			candidate := ComponentValueCandidate{
				PrimitiveKey:         primitive.Key,
				ValueSI:              &valueCopy,
				Quantity:             domain.Kind,
				Unit:                 domain.Unit,
				PreferredSeries:      string(series),
				TolerancePercent:     tolerance,
				ToleranceProven:      toleranceProven,
				CornerMinimumSI:      &cornerMinimumCopy,
				CornerMaximumSI:      &cornerMaximumCopy,
				IdealSI:              &idealCopy,
				AnalyticPriority:     analyticPriority,
				RelativeError:        relativeError,
				Derivation:           derivation,
				CatalogEvidence:      primitive.Evidence,
				ModelEvidenceSHA256s: modelHashes,
				RatingEvidence:       append([]PrimitiveBound(nil), primitive.Ratings...),
			}
			candidate.Hash = componentValueCandidateHash(candidate)
			candidates = append(candidates, candidate)
		}
	}
	slices.SortFunc(candidates, compareComponentValueCandidates)
	candidates = compactValueCandidates(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, rejections
}

func analyticScaleFit(
	scales []AnalyticScale,
	kind string,
	unit string,
	value float64,
) (int, float64, string, bool) {
	bestPriority := int(^uint(0) >> 1)
	bestError := math.Inf(1)
	bestID := ""
	for _, scale := range scales {
		if scale.Kind != kind || scale.Unit != unit || scale.ValueSI <= 0 {
			continue
		}
		relativeError := math.Abs(value-scale.ValueSI) / scale.ValueSI
		if scale.Priority < bestPriority ||
			(scale.Priority == bestPriority &&
				(relativeError < bestError ||
					(relativeError == bestError && scale.ID < bestID))) {
			bestPriority = scale.Priority
			bestError = relativeError
			bestID = scale.ID
		}
	}
	return bestPriority, bestError, bestID, bestID != ""
}

func deriveAnalyticScales(requirement Requirement, primitive PrimitiveCandidate) []AnalyticScale {
	result := []AnalyticScale{}
	const anchorResistance = 10_000.0
	supplyVoltage := nominalSupplyVoltage(requirement)
	add := func(scale AnalyticScale) {
		if scale.ValueSI > 0 && finite(scale.ValueSI) {
			result = append(result, scale)
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			value := positiveMidpoint(condition.Min, condition.Max)
			switch condition.Axis {
			case "load_resistance", "source_resistance":
				add(AnalyticScale{ID: "condition:" + operatingCase.ID + ":" + condition.Axis, Kind: "resistance", ValueSI: value, Unit: "ohm", Derivation: "advisory operating-condition impedance", SourceKind: "operating_condition", SourceID: operatingCase.ID, Priority: 50})
			case "load_capacitance":
				add(AnalyticScale{ID: "condition:" + operatingCase.ID + ":" + condition.Axis, Kind: "capacitance", ValueSI: value, Unit: "F", Derivation: "advisory operating-condition capacitance", SourceKind: "operating_condition", SourceID: operatingCase.ID, Priority: 50})
			case "load_inductance":
				add(AnalyticScale{ID: "condition:" + operatingCase.ID + ":" + condition.Axis, Kind: "inductance", ValueSI: value, Unit: "H", Derivation: "advisory operating-condition inductance", SourceKind: "operating_condition", SourceID: operatingCase.ID, Priority: 50})
			}
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		value := assertionTarget(assertion)
		source := "assertion:" + assertion.ID
		switch assertion.Metric {
		case "transconductance":
			if value > 0 {
				add(AnalyticScale{ID: source + ":reciprocal_transconductance", Kind: "resistance", ValueSI: 1 / value, Unit: "ohm", Derivation: "R=1/gm dimensional scale", SourceKind: "behavioral_assertion", SourceID: assertion.ID, Priority: 5})
			}
		case "voltage_gain", "voltage_gain_at_frequency":
			addRatioDerivedResistanceScales(&result, source, assertion.ID, "gain ratio applied to neutral resistance anchor", value, anchorResistance, 20)
		case "cutoff_frequency":
			if value > 0 {
				add(AnalyticScale{ID: source + ":rc_capacitance", Kind: "capacitance", ValueSI: 1 / (2 * math.Pi * value * anchorResistance), Unit: "F", Derivation: "C=1/(2*pi*f*Ranchor)", SourceKind: "behavioral_assertion", SourceID: assertion.ID, Priority: 10})
			}
		case "settling_time", "propagation_delay", "rise_time", "fall_time":
			if value > 0 {
				add(AnalyticScale{ID: source + ":timing_capacitance", Kind: "capacitance", ValueSI: value / anchorResistance, Unit: "F", Derivation: "C=t/Ranchor dimensional scale", SourceKind: "behavioral_assertion", SourceID: assertion.ID, Priority: 25})
			}
		case "rising_threshold", "falling_threshold", "lower_threshold", "upper_threshold", "output_voltage":
			if value > 0 && supplyVoltage > 0 {
				addRatioDerivedResistanceScales(&result, source, assertion.ID, "voltage-to-supply ratio applied to neutral resistance anchor", value/supplyVoltage, anchorResistance, 20)
			}
		case "hysteresis":
			if value > 0 && supplyVoltage > 0 {
				addRatioDerivedResistanceScales(&result, source, assertion.ID, "threshold-span ratio applied to neutral resistance anchor", value/supplyVoltage, anchorResistance, 20)
			}
		}
		if assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 {
			add(AnalyticScale{ID: source + ":frequency_capacitance", Kind: "capacitance", ValueSI: 1 / (2 * math.Pi * *assertion.FrequencyHz * anchorResistance), Unit: "F", Derivation: "C=1/(2*pi*f*Ranchor)", SourceKind: "behavioral_assertion", SourceID: assertion.ID, Priority: 30})
		}
	}
	slices.SortFunc(result, compareAnalyticScales)
	return compactAnalyticScales(result)
}

// deriveTopologyAnalyticScales converts a primitive decision network's
// connectivity and bounded behavior into advisory dimensional targets. It
// recognizes only terminal roles and passive branches; it does not select or
// name a circuit family.
func deriveTopologyAnalyticScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	supply := nominalSupplyVoltage(requirement)
	if supply <= 0 {
		return nil
	}
	if scales := deriveConditionalTransferTopologyScales(
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveAnalogTransferTopologyScales(
		requirement,
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveTransconductanceTopologyScales(
		requirement,
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveControlledSwitchTopologyScales(
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if instance.Kind != "resistor" {
		return nil
	}
	lower, upper := 0.0, 0.0
	hasLower, hasUpper, hasHysteresis := false, false, false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		target := assertionTarget(assertion)
		switch assertion.Metric {
		case "falling_threshold":
			if target > 0 && (!hasLower || target < lower) {
				lower, hasLower = target, true
			}
		case "rising_threshold":
			if target > 0 && (!hasUpper || target > upper) {
				upper, hasUpper = target, true
			}
		case "hysteresis":
			hasHysteresis = target > 0
		}
	}
	if !hasLower || !hasUpper || !hasHysteresis ||
		lower >= upper || upper >= supply {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	resistorBranches := func(node string) map[string]string {
		branches := map[string]string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			other := ""
			switch {
			case candidate.Terminals[0].Node == node:
				other = candidate.Terminals[1].Node
			case candidate.Terminals[1].Node == node:
				other = candidate.Terminals[0].Node
			default:
				continue
			}
			role := nodeByID[other].Role
			if role != "" && branches[role] == "" {
				branches[role] = candidate.ID
			}
		}
		return branches
	}
	resistorNeighbors := func(node string) map[string]string {
		neighbors := map[string]string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			other := ""
			switch {
			case candidate.Terminals[0].Node == node:
				other = candidate.Terminals[1].Node
			case candidate.Terminals[1].Node == node:
				other = candidate.Terminals[0].Node
			}
			if other != "" && neighbors[other] == "" {
				neighbors[other] = candidate.ID
			}
		}
		return neighbors
	}
	referenceVoltageByNode := map[string]float64{}
	for _, candidate := range graph.Instances {
		if candidate.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		primitive := inventory[candidate.PrimitiveKey]
		for _, model := range primitive.Models {
			for _, parameter := range model.Parameters {
				if parameter.Name == "output_voltage_v" &&
					parameter.Value > 0 {
					referenceVoltageByNode[terminals["CATHODE"]] = parameter.Value
				}
			}
		}
	}
	const anchorResistance = 10_000.0
	feedbackRatio := (upper - lower) / supply
	decisionReference := upper / (1 + feedbackRatio)
	for sourceNode := range referenceVoltageByNode {
		neighbors := resistorNeighbors(sourceNode)
		for otherNode, resistorID := range neighbors {
			if resistorID == instance.ID &&
				nodeByID[otherNode].Role == "supply" {
				return []AnalyticScale{{
					ID:         "topology:absolute_reference_bias:" + sourceNode,
					Kind:       "resistance",
					ValueSI:    anchorResistance,
					Unit:       "ohm",
					Derivation: "neutral bias resistance for a reviewed two-terminal reference primitive",
					SourceKind: "candidate_topology",
					SourceID:   sourceNode,
					Priority:   1,
				}}
			}
		}
	}
	for _, active := range graph.Instances {
		if active.Kind != "comparator" && active.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		if active.Kind == "opamp" &&
			nodeByID[terminals["OUT"]].Scope == "internal" &&
			nodeByID[terminals["IN_MINUS"]].Scope == "internal" {
			referenceVoltage := referenceVoltageByNode[terminals["IN_PLUS"]]
			if referenceVoltage > 0 && decisionReference > referenceVoltage {
				neighbors := resistorNeighbors(terminals["IN_MINUS"])
				feedbackID := neighbors[terminals["OUT"]]
				referenceID := ""
				for otherNode, resistorID := range neighbors {
					if nodeByID[otherNode].Role == "reference" {
						referenceID = resistorID
						break
					}
				}
				gainRatio := decisionReference/referenceVoltage - 1
				values := map[string]float64{
					referenceID: anchorResistance,
					feedbackID:  anchorResistance * gainRatio,
				}
				if value := values[instance.ID]; value > 0 && finite(value) {
					return []AnalyticScale{{
						ID:         "topology:absolute_reference_gain:" + terminals["OUT"] + ":" + instance.ID,
						Kind:       "resistance",
						ValueSI:    value,
						Unit:       "ohm",
						Derivation: "non-inverting gain ratio derived from reviewed reference voltage and bounded decision thresholds",
						SourceKind: "candidate_topology",
						SourceID:   terminals["OUT"],
						Priority:   1,
					}}
				}
			}
		}
		output := nodeByID[terminals["OUT"]]
		if output.Role != "output" {
			continue
		}
		positiveNode := terminals["IN_PLUS"]
		referenceNode := terminals["IN_MINUS"]
		if nodeByID[positiveNode].Scope == "internal" &&
			nodeByID[referenceNode].Scope == "internal" {
			positiveBranches := resistorBranches(positiveNode)
			referenceBranches := resistorBranches(referenceNode)
			if positiveBranches["input"] != "" &&
				positiveBranches["output"] != "" {
				values := map[string]float64{
					positiveBranches["input"]:  anchorResistance,
					positiveBranches["output"]: anchorResistance / feedbackRatio,
				}
				if referenceBranches["supply"] != "" &&
					referenceBranches["reference"] != "" {
					supplyFraction := decisionReference / supply
					referenceFraction := 1 - supplyFraction
					maximumDividerFraction := max(supplyFraction, referenceFraction)
					values[referenceBranches["supply"]] =
						anchorResistance * maximumDividerFraction / supplyFraction
					values[referenceBranches["reference"]] =
						anchorResistance * maximumDividerFraction / referenceFraction
				}
				if value := values[instance.ID]; value > 0 && finite(value) {
					return []AnalyticScale{{
						ID:         "topology:decision_threshold:" + positiveNode + ":" + instance.ID,
						Kind:       "resistance",
						ValueSI:    value,
						Unit:       "ohm",
						Derivation: "resistance ratio derived from bounded decision center, hysteresis span, and rail span",
						SourceKind: "candidate_topology",
						SourceID:   positiveNode,
						Priority:   1,
					}}
				}
			}
		}
		for _, terminal := range []string{"IN_PLUS", "IN_MINUS"} {
			decisionNode := terminals[terminal]
			if nodeByID[decisionNode].Scope != "internal" {
				continue
			}
			branches := resistorBranches(decisionNode)
			if branches["supply"] == "" ||
				branches["reference"] == "" ||
				branches["output"] == "" {
				continue
			}
			role := ""
			for candidateRole, instanceID := range branches {
				if instanceID == instance.ID {
					role = candidateRole
					break
				}
			}
			if role == "" {
				continue
			}
			fractions := map[string]float64{
				"supply":    lower / supply,
				"output":    (upper - lower) / supply,
				"reference": 1 - upper/supply,
			}
			maximumFraction := max(
				fractions["supply"],
				fractions["reference"],
				fractions["output"],
			)
			fraction := fractions[role]
			if fraction <= 0 || maximumFraction <= 0 {
				continue
			}
			return []AnalyticScale{{
				ID:         "topology:decision_threshold:" + decisionNode + ":" + role,
				Kind:       "resistance",
				ValueSI:    anchorResistance * maximumFraction / fraction,
				Unit:       "ohm",
				Derivation: "conductance fraction derived from bounded decision thresholds and rail span",
				SourceKind: "candidate_topology",
				SourceID:   decisionNode,
				Priority:   1,
			}}
		}
	}
	return nil
}

func deriveConditionalTransferTopologyScales(
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	between := func(kind, left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != kind || len(candidate.Terminals) != 2 {
				continue
			}
			first := candidate.Terminals[0].Node
			second := candidate.Terminals[1].Node
			if (first == left && second == right) ||
				(first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	betweenAll := func(kind, left, right string) []string {
		result := []string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != kind || len(candidate.Terminals) != 2 {
				continue
			}
			first := candidate.Terminals[0].Node
			second := candidate.Terminals[1].Node
			if (first == left && second == right) ||
				(first == right && second == left) {
				result = append(result, candidate.ID)
			}
		}
		return result
	}
	const (
		biasResistance      = 47_000.0
		couplingCapacitance = 4.7e-6
	)
	for _, active := range graph.Instances {
		if active.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		biasNode := terminals["IN_PLUS"]
		driverNode := terminals["OUT"]
		if nodeByID[biasNode].Scope != "internal" ||
			nodeByID[driverNode].Scope != "internal" ||
			terminals["IN_MINUS"] != driverNode {
			continue
		}
		inputNode := ""
		outputNode := ""
		for _, node := range graph.Nodes {
			switch node.Role {
			case "input":
				if between("capacitor", node.ID, biasNode) != "" {
					inputNode = node.ID
				}
			case "output":
				if len(betweenAll("capacitor", driverNode, node.ID)) != 0 {
					outputNode = node.ID
				}
			}
		}
		referenceNode := ""
		supplyNode := ""
		for _, node := range graph.Nodes {
			switch node.Role {
			case "reference":
				referenceNode = node.ID
			case "supply":
				supplyNode = node.ID
			}
		}
		if inputNode == "" || outputNode == "" ||
			referenceNode == "" || supplyNode == "" {
			continue
		}
		inputCouplingID := between("capacitor", inputNode, biasNode)
		outputCouplingIDs := betweenAll("capacitor", driverNode, outputNode)
		biasHighID := between("resistor", supplyNode, biasNode)
		biasLowID := between("resistor", biasNode, referenceNode)
		outputReferenceID := between("resistor", outputNode, referenceNode)
		if inputCouplingID == "" ||
			len(outputCouplingIDs) == 0 ||
			biasHighID == "" ||
			biasLowID == "" ||
			outputReferenceID == "" {
			continue
		}
		hasControlledShunt := false
		for _, switching := range graph.Instances {
			if switching.Kind != "n_channel_mosfet" {
				continue
			}
			switchingTerminals := topologyTerminalNodes(switching)
			hasControlledShunt =
				switchingTerminals["DRAIN"] == outputNode &&
					nodeByID[switchingTerminals["GATE"]].Role == "control" &&
					switchingTerminals["SOURCE"] == referenceNode
			if hasControlledShunt {
				break
			}
		}
		if !hasControlledShunt {
			continue
		}
		if instance.Kind == "resistor" &&
			(instance.ID == biasHighID ||
				instance.ID == biasLowID ||
				instance.ID == outputReferenceID) {
			return []AnalyticScale{{
				ID:         "topology:conditional_transfer_bias:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    biasResistance,
				Unit:       "ohm",
				Derivation: "equal high-impedance bias and return resistance for an AC-coupled conditional transfer",
				SourceKind: "candidate_topology",
				SourceID:   biasNode,
				Priority:   1,
			}}
		}
		if instance.Kind == "capacitor" &&
			(instance.ID == inputCouplingID ||
				slices.Contains(outputCouplingIDs, instance.ID)) {
			return []AnalyticScale{{
				ID:         "topology:conditional_transfer_coupling:" + instance.ID,
				Kind:       "capacitance",
				ValueSI:    couplingCapacitance,
				Unit:       "F",
				Derivation: "bounded AC-coupling scale for a biased conditional signal path",
				SourceKind: "candidate_topology",
				SourceID:   outputNode,
				Priority:   1,
			}}
		}
	}
	return nil
}

func deriveAnalogTransferTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	gain, cutoff := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "voltage_gain":
			target := assertionTarget(assertion)
			if target > gain {
				gain = target
			}
		case "cutoff_frequency":
			target := assertionTarget(assertion)
			if target > 0 {
				cutoff = target
			}
		}
	}
	if gain <= 1 || cutoff <= 0 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	between := func(kind, left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != kind || len(candidate.Terminals) != 2 {
				continue
			}
			first := candidate.Terminals[0].Node
			second := candidate.Terminals[1].Node
			if (first == left && second == right) ||
				(first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	for _, active := range graph.Instances {
		if active.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		if nodeByID[terminals["OUT"]].Role != "output" ||
			nodeByID[terminals["IN_PLUS"]].Scope != "internal" ||
			nodeByID[terminals["IN_MINUS"]].Scope != "internal" {
			continue
		}
		inputNode := ""
		referenceNode := ""
		for _, node := range graph.Nodes {
			if node.Role == "input" &&
				between("resistor", terminals["IN_PLUS"], node.ID) != "" {
				inputNode = node.ID
			}
			if node.Role == "reference" {
				referenceNode = node.ID
			}
		}
		if inputNode == "" || referenceNode == "" {
			continue
		}
		inputResistor := between("resistor", terminals["IN_PLUS"], inputNode)
		filterCapacitor := between("capacitor", terminals["IN_PLUS"], referenceNode)
		gainReference := between("resistor", terminals["IN_MINUS"], referenceNode)
		gainFeedback := between("resistor", terminals["IN_MINUS"], terminals["OUT"])
		if inputResistor == "" || filterCapacitor == "" ||
			gainReference == "" || gainFeedback == "" {
			continue
		}
		const anchorResistance = 10_000.0
		switch instance.ID {
		case gainReference:
			return []AnalyticScale{{
				ID:         "topology:closed_loop_gain:reference",
				Kind:       "resistance",
				ValueSI:    anchorResistance,
				Unit:       "ohm",
				Derivation: "neutral reference resistance for bounded closed-loop gain",
				SourceKind: "candidate_topology",
				SourceID:   terminals["IN_MINUS"],
				Priority:   1,
			}}
		case gainFeedback:
			return []AnalyticScale{{
				ID:         "topology:closed_loop_gain:feedback",
				Kind:       "resistance",
				ValueSI:    anchorResistance * (gain - 1),
				Unit:       "ohm",
				Derivation: "feedback ratio derived from bounded non-inverting voltage gain",
				SourceKind: "candidate_topology",
				SourceID:   terminals["IN_MINUS"],
				Priority:   1,
			}}
		case inputResistor:
			return []AnalyticScale{
				{
					ID:         "topology:input_time_constant:resistance_1",
					Kind:       "resistance",
					ValueSI:    anchorResistance,
					Unit:       "ohm",
					Derivation: "first neutral resistance scale for bounded input cutoff",
					SourceKind: "candidate_topology",
					SourceID:   terminals["IN_PLUS"],
					Priority:   1,
				},
				{
					ID:         "topology:input_time_constant:resistance_2",
					Kind:       "resistance",
					ValueSI:    anchorResistance / 2,
					Unit:       "ohm",
					Derivation: "second neutral resistance scale for bounded input cutoff",
					SourceKind: "candidate_topology",
					SourceID:   terminals["IN_PLUS"],
					Priority:   1,
				},
			}
		case filterCapacitor:
			return []AnalyticScale{
				{
					ID:         "topology:input_time_constant:capacitance_1",
					Kind:       "capacitance",
					ValueSI:    1 / (2 * math.Pi * cutoff * anchorResistance),
					Unit:       "F",
					Derivation: "C=1/(2*pi*f*R) for first bounded input cutoff scale",
					SourceKind: "candidate_topology",
					SourceID:   terminals["IN_PLUS"],
					Priority:   1,
				},
				{
					ID:         "topology:input_time_constant:capacitance_2",
					Kind:       "capacitance",
					ValueSI:    2 / (2 * math.Pi * cutoff * anchorResistance),
					Unit:       "F",
					Derivation: "C=1/(2*pi*f*R) for second bounded input cutoff scale",
					SourceKind: "candidate_topology",
					SourceID:   terminals["IN_PLUS"],
					Priority:   1,
				},
			}
		}
	}
	return nil
}

func deriveControlledSwitchTopologyScales(
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	for _, decision := range graph.Instances {
		if decision.Kind != "comparator" {
			continue
		}
		decisionTerminals := topologyTerminalNodes(decision)
		gateNode := decisionTerminals["OUT"]
		if nodeByID[gateNode].Scope != "internal" {
			continue
		}
		hasControlledDevice := false
		for _, active := range graph.Instances {
			if active.Kind == "n_channel_mosfet" &&
				topologyTerminalNodes(active)["GATE"] == gateNode {
				hasControlledDevice = true
				break
			}
		}
		if !hasControlledDevice {
			continue
		}
		thresholdNode := ""
		hasExternalControl := false
		for _, terminal := range []string{"IN_PLUS", "IN_MINUS"} {
			node := decisionTerminals[terminal]
			switch {
			case nodeByID[node].Role == "control":
				hasExternalControl = true
			case nodeByID[node].Scope == "internal":
				thresholdNode = node
			}
		}
		if !hasExternalControl || thresholdNode == "" {
			continue
		}
		first := instance.Terminals[0].Node
		second := instance.Terminals[1].Node
		if instance.Kind == "capacitor" {
			if first != gateNode && second != gateNode {
				continue
			}
			return []AnalyticScale{{
				ID:         "topology:default_state_hold:" + instance.ID,
				Kind:       "capacitance",
				ValueSI:    10e-9,
				Unit:       "F",
				Derivation: "bounded control-node hold capacitance for deterministic startup state",
				SourceKind: "candidate_topology",
				SourceID:   decision.ID,
				Priority:   1,
			}}
		}
		if first != gateNode && second != gateNode &&
			first != thresholdNode && second != thresholdNode {
			continue
		}
		const anchorResistance = 10_000.0
		return []AnalyticScale{{
			ID:         "topology:bounded_control_interface:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    anchorResistance,
			Unit:       "ohm",
			Derivation: "neutral equal-ratio resistance for bounded control threshold and gate drive",
			SourceKind: "candidate_topology",
			SourceID:   decision.ID,
			Priority:   1,
		}}
	}
	return nil
}

func deriveTransconductanceTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
		return nil
	}
	transconductance := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "transconductance" {
			transconductance = assertionTarget(assertion)
			break
		}
	}
	if transconductance <= 0 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	between := func(left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first := candidate.Terminals[0].Node
			second := candidate.Terminals[1].Node
			if (first == left && second == right) ||
				(first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	for _, passDevice := range graph.Instances {
		if passDevice.Kind != "pnp_bjt" {
			continue
		}
		passTerminals := topologyTerminalNodes(passDevice)
		emitter := passTerminals["EMITTER"]
		collector := passTerminals["COLLECTOR"]
		supply := ""
		output := ""
		reference := ""
		for _, node := range graph.Nodes {
			if node.Role == "supply" &&
				(node.ID == emitter || between(node.ID, emitter) != "") {
				supply = node.ID
			}
			if node.Role == "output" && between(collector, node.ID) != "" {
				output = node.ID
			}
			if node.Role == "reference" {
				reference = node.ID
			}
		}
		if supply == "" ||
			nodeByID[collector].Scope != "internal" ||
			output == "" ||
			reference == "" {
			continue
		}
		ballast := between(supply, emitter)
		if emitter != supply && instance.ID == ballast {
			return []AnalyticScale{{
				ID:         "topology:parallel_pass_ballast:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    .22,
				Unit:       "ohm",
				Derivation: "low-value emitter ballast bounds current sharing between parallel analog pass devices",
				SourceKind: "candidate_topology",
				SourceID:   passDevice.ID,
				Priority:   1,
			}}
		}
		shunt := between(collector, output)
		if instance.ID == shunt {
			return []AnalyticScale{{
				ID:         "topology:current_sense_impedance:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    1 / transconductance,
				Unit:       "ohm",
				Derivation: "sense impedance is the reciprocal of bounded voltage-to-current transfer",
				SourceKind: "candidate_topology",
				SourceID:   passDevice.ID,
				Priority:   1,
			}}
		}
		bias := between(passTerminals["BASE"], supply)
		if instance.ID == bias {
			return []AnalyticScale{{
				ID:         "topology:pass_device_bias:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    10_000,
				Unit:       "ohm",
				Derivation: "bounded resistive bias keeps the analog pass-device control node defined",
				SourceKind: "candidate_topology",
				SourceID:   passDevice.ID,
				Priority:   1,
			}}
		}
		for _, active := range graph.Instances {
			if active.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if instance.ID != between(passTerminals["BASE"], terminals["OUT"]) {
				continue
			}
			return []AnalyticScale{{
				ID:         "topology:pass_device_drive:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    100,
				Unit:       "ohm",
				Derivation: "bounded series resistance isolates the feedback controller from the nonlinear pass-device input",
				SourceKind: "candidate_topology",
				SourceID:   passDevice.ID,
				Priority:   1,
			}}
		}
		for _, active := range graph.Instances {
			if active.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if between(terminals["OUT"], passTerminals["BASE"]) != "" ||
				nodeByID[terminals["OUT"]].Scope != "internal" ||
				nodeByID[terminals["IN_MINUS"]].Scope != "internal" ||
				nodeByID[terminals["IN_PLUS"]].Scope != "internal" {
				continue
			}
			network := map[string]bool{
				between(output, terminals["IN_MINUS"]):           true,
				between(terminals["OUT"], terminals["IN_MINUS"]): true,
				between(collector, terminals["IN_PLUS"]):         true,
				between(terminals["IN_PLUS"], reference):         true,
			}
			delete(network, "")
			if len(network) != 4 || !network[instance.ID] {
				continue
			}
			const anchorResistance = 10_000.0
			return []AnalyticScale{{
				ID:         "topology:differential_observation:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    anchorResistance,
				Unit:       "ohm",
				Derivation: "equal-ratio resistance for bounded differential current observation",
				SourceKind: "candidate_topology",
				SourceID:   active.ID,
				Priority:   1,
			}}
		}
	}
	return nil
}

func addRatioDerivedResistanceScales(
	result *[]AnalyticScale,
	id string,
	sourceID string,
	derivation string,
	ratio float64,
	anchor float64,
	priority int,
) {
	if ratio <= 0 || !finite(ratio) || anchor <= 0 {
		return
	}
	values := []float64{anchor, anchor * ratio, anchor / ratio}
	for index, value := range values {
		if value <= 0 || !finite(value) {
			continue
		}
		*result = append(*result, AnalyticScale{
			ID:         fmt.Sprintf("%s:ratio_resistance_%d", id, index+1),
			Kind:       "resistance",
			ValueSI:    value,
			Unit:       "ohm",
			Derivation: derivation,
			SourceKind: "behavioral_assertion",
			SourceID:   sourceID,
			Priority:   priority,
		})
	}
}

func assertionTarget(assertion BehavioralAssertion) float64 {
	switch {
	case assertion.Min != nil && assertion.Max != nil:
		return positiveMidpoint(*assertion.Min, *assertion.Max)
	case assertion.Max != nil && *assertion.Max > 0:
		return *assertion.Max
	case assertion.Min != nil && *assertion.Min > 0:
		return *assertion.Min
	default:
		return 0
	}
}

func positiveMidpoint(minimum float64, maximum float64) float64 {
	if minimum > 0 && maximum > 0 {
		return (minimum + maximum) / 2
	}
	if maximum > 0 {
		return maximum
	}
	if minimum > 0 {
		return minimum
	}
	return 0
}

func nominalSupplyVoltage(requirement Requirement) float64 {
	values := []float64{}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		switch {
		case domain.NominalVoltageV != nil && *domain.NominalVoltageV > 0:
			values = append(values, *domain.NominalVoltageV)
		case domain.MaxVoltageV != nil && *domain.MaxVoltageV > 0:
			values = append(values, *domain.MaxVoltageV)
		}
	}
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	return values[len(values)-1]
}

func effectiveValueRange(domain PrimitiveValueDomain) (float64, float64, bool) {
	if domain.Nominal != nil && *domain.Nominal > 0 &&
		(domain.Minimum == nil || *domain.Minimum <= 0) &&
		(domain.Maximum == nil || *domain.Maximum <= 0) {
		return *domain.Nominal, *domain.Nominal, true
	}
	minimum := 0.0
	maximum := 0.0
	if domain.Minimum != nil && *domain.Minimum > 0 {
		minimum = *domain.Minimum
	}
	if domain.Maximum != nil && *domain.Maximum > 0 {
		maximum = *domain.Maximum
	}
	if minimum == 0 && domain.Nominal != nil && *domain.Nominal > 0 {
		minimum = *domain.Nominal
	}
	if maximum == 0 && domain.Nominal != nil && *domain.Nominal > 0 {
		maximum = *domain.Nominal
	}
	if minimum == 0 && maximum > 0 {
		minimum = maximum * 1e-12
	}
	if maximum == 0 && minimum > 0 {
		maximum = minimum * 1e12
	}
	return minimum, maximum, minimum > 0 && maximum >= minimum && finite(minimum) && finite(maximum)
}

func primitiveTolerancePercent(primitive PrimitiveCandidate, quantity string) (float64, bool) {
	for _, tolerance := range primitive.Tolerances {
		if tolerance.Kind != quantity || tolerance.Unit != "%" {
			continue
		}
		if tolerance.Maximum != nil && *tolerance.Maximum > 0 {
			return *tolerance.Maximum, true
		}
		if tolerance.Nominal != nil && *tolerance.Nominal > 0 {
			return *tolerance.Nominal, true
		}
	}
	return 0, false
}

func preferredSeriesForDomain(
	quantity string,
	tolerance float64,
	proven bool,
) architecturesearch.PreferredSeries {
	if proven {
		switch {
		case tolerance <= 0.5:
			return architecturesearch.SeriesE192
		case tolerance <= 1:
			return architecturesearch.SeriesE96
		case tolerance <= 2:
			return architecturesearch.SeriesE48
		case tolerance <= 5:
			return architecturesearch.SeriesE24
		}
	}
	if quantity == "resistance" {
		return architecturesearch.SeriesE24
	}
	return architecturesearch.SeriesE12
}

func ratingsCoverRequirement(requirement Requirement, primitive PrimitiveCandidate) bool {
	requiredVoltage := 0.0
	requiredTemperature := 0.0
	for _, domain := range requirement.Requirements.Domains {
		if domain.MaxVoltageV != nil {
			requiredVoltage = math.Max(requiredVoltage, math.Abs(*domain.MaxVoltageV))
		}
		if domain.MinVoltageV != nil {
			requiredVoltage = math.Max(requiredVoltage, math.Abs(*domain.MinVoltageV))
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch condition.Axis {
			case "supply_voltage", "input_voltage":
				requiredVoltage = math.Max(requiredVoltage, math.Max(math.Abs(condition.Min), math.Abs(condition.Max)))
			case "ambient_temperature", "temperature":
				requiredTemperature = math.Max(requiredTemperature, math.Max(condition.Min, condition.Max))
			}
		}
	}
	for _, rating := range primitive.Ratings {
		limit := boundMaximum(rating)
		if limit <= 0 {
			continue
		}
		switch strings.ToLower(rating.Kind) {
		case "voltage", "maximum_voltage", "working_voltage":
			if requiredVoltage > limit {
				return false
			}
		case "temperature", "junction_temperature":
			if requiredTemperature > 0 && requiredTemperature > limit {
				return false
			}
		}
	}
	return true
}

func boundMaximum(bound PrimitiveBound) float64 {
	if bound.Maximum != nil {
		return *bound.Maximum
	}
	if bound.Nominal != nil {
		return *bound.Nominal
	}
	return 0
}

func primitiveModelEvidenceHashes(primitive PrimitiveCandidate) []string {
	result := []string{}
	for _, model := range primitive.Models {
		if len(model.ProvenanceSHA256) == 64 {
			result = append(result, model.ProvenanceSHA256)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func compareAnalyticScales(left, right AnalyticScale) int {
	return cmp.Or(
		cmp.Compare(left.Priority, right.Priority),
		cmp.Compare(left.Kind, right.Kind),
		cmp.Compare(left.ValueSI, right.ValueSI),
		cmp.Compare(left.ID, right.ID),
	)
}

func compactAnalyticScales(values []AnalyticScale) []AnalyticScale {
	result := make([]AnalyticScale, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		key := value.Kind + "|" + value.Unit + "|" + canonicalOptionalFloat(&value.ValueSI)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func compareComponentValueCandidates(left, right ComponentValueCandidate) int {
	return cmp.Or(
		cmp.Compare(left.AnalyticPriority, right.AnalyticPriority),
		cmp.Compare(
			componentCandidateWorstRelativeError(left),
			componentCandidateWorstRelativeError(right),
		),
		cmp.Compare(left.RelativeError, right.RelativeError),
		cmp.Compare(primitiveEvidencePenalty(left.CatalogEvidence), primitiveEvidencePenalty(right.CatalogEvidence)),
		cmp.Compare(canonicalOptionalFloat(left.ValueSI), canonicalOptionalFloat(right.ValueSI)),
		cmp.Compare(left.PrimitiveKey, right.PrimitiveKey),
		cmp.Compare(left.Derivation, right.Derivation),
		cmp.Compare(left.Hash, right.Hash),
	)
}

func componentCandidateWorstRelativeError(candidate ComponentValueCandidate) float64 {
	if candidate.ValueSI == nil {
		return candidate.RelativeError
	}
	if !candidate.ToleranceProven {
		return math.Inf(1)
	}
	return candidate.RelativeError + candidate.TolerancePercent/100
}

func compactValueCandidates(values []ComponentValueCandidate) []ComponentValueCandidate {
	result := make([]ComponentValueCandidate, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		key := value.PrimitiveKey + "|" + canonicalOptionalFloat(value.ValueSI)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func selectDiverseValueCandidates(
	candidates []ComponentValueCandidate,
	limit int,
) []ComponentValueCandidate {
	if limit <= 0 || len(candidates) <= limit {
		return append([]ComponentValueCandidate(nil), candidates...)
	}
	result := make([]ComponentValueCandidate, 0, limit)
	selected := map[string]bool{}
	seenValues := map[string]bool{}
	for _, candidate := range candidates {
		valueKey := candidate.Quantity + "|" + candidate.Unit + "|" +
			canonicalOptionalFloat(candidate.ValueSI)
		if seenValues[valueKey] {
			continue
		}
		seenValues[valueKey] = true
		result = append(result, candidate)
		selected[candidate.Hash] = true
		if len(result) == limit {
			return result
		}
	}
	for _, candidate := range candidates {
		if selected[candidate.Hash] {
			continue
		}
		result = append(result, candidate)
		if len(result) == limit {
			break
		}
	}
	return result
}

func componentValueCandidateHash(candidate ComponentValueCandidate) string {
	candidate.Hash = ""
	data, _ := json.Marshal(candidate)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func valueTrialHash(selections []ValueTrialSelection) string {
	data, _ := json.Marshal(selections)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
