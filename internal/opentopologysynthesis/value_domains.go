package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	valueCandidatesPerInstance              = architecturesearch.DefaultMaxValueCandidates
	maxCatalogResistanceDividerCombinations = 4_096
	// A nominal RC corner at the exact minimum bandwidth can fall below the
	// requirement under reviewed component tolerances. Keep a fixed 25% design
	// headroom in value ranking; trusted worst-case simulation still decides
	// acceptance and this factor does not relax the declared bound.
	minimumBandwidthDesignFactor = 1.25
)

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
	convergentDigitalScales := deriveConvergentDigitalTopologyScales(requirement, normalizedGraph)
	preserveDerivedVoltageSeeds := len(regulatedVoltageRelationships(requirement)) != 0
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
				convergentDigitalScales,
			)...,
		)
		slices.SortFunc(domain.AnalyticScales, compareAnalyticScales)
		domain.AnalyticScales = compactAnalyticScales(domain.AnalyticScales)
		if original.ValueDomain != nil {
			domain.Quantity = original.ValueDomain.Kind
			domain.Unit = original.ValueDomain.Unit
		}
		variants := compatibleValueVariants(original, inventory, requiredAnalyses)
		variants = slices.DeleteFunc(variants, func(variant PrimitiveCandidate) bool {
			if primitiveSupplySpanCoversConnectedRails(requirement, normalizedGraph, instance, variant) {
				return false
			}
			rejections["supply_span_envelope"] = append(rejections["supply_span_envelope"], instance.ID+":"+variant.Key)
			return true
		})
		variants = slices.DeleteFunc(variants, func(variant PrimitiveCandidate) bool {
			return !stepDownOutputCapacitorVariantAllowed(
				requirement, normalizedGraph, instance, variant, inventoryByKey,
			)
		})
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
		if original.ValueDomain == nil || preserveDerivedVoltageSeeds {
			// Behavior-derived voltage relationships jointly select coupled
			// feedback, drive, ballast, and protection values. Preserve those
			// catalog seeds as the first deterministic trial while retaining the
			// bounded domain for later exploration and repair.
			domain.Candidates = prioritizeOriginalCandidate(
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

func prioritizeOriginalCandidate(
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
	expectedPorts := map[string]Port{}
	for _, port := range requirement.Requirements.Ports {
		expectedPorts[port.ID] = port
	}
	expectedDomains := map[string]Domain{}
	for _, domain := range requirement.Requirements.Domains {
		expectedDomains[domain.ID] = domain
	}
	seenPorts := map[string]bool{}
	seenDomains := map[string]bool{}
	issues := []reports.Issue{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" {
			continue
		}
		if node.SemanticKind == "domain" {
			domain, found := expectedDomains[node.SemanticID]
			if !found || node.Domain != node.SemanticID || node.Role != domain.Kind {
				issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "external source-domain node is not declared by the behavioral requirement", "rebuild the graph from the normalized requirement"))
				continue
			}
			if seenDomains[node.SemanticID] {
				message, suggestion := "behavioral source domain is bound more than once", "bind each semantic source exactly once"
				if domain.Kind == "reference" {
					message, suggestion = "behavioral reference domain is bound more than once", "bind each semantic reference exactly once"
				}
				issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, message, suggestion))
			}
			seenDomains[node.SemanticID] = true
			continue
		}
		port, found := expectedPorts[node.SemanticID]
		if !found {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "external graph node is not declared by the behavioral requirement", "rebuild the graph from the normalized requirement"))
			continue
		}
		if seenPorts[node.SemanticID] {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "behavioral port is bound more than once", "bind each semantic port exactly once"))
		}
		seenPorts[node.SemanticID] = true
		role := graphRoleForPort(port)
		if role == "reference" || role == "supply" {
			if seenDomains[port.Domain] {
				message, suggestion := "behavioral source domain is bound more than once", "bind each semantic source exactly once"
				if role == "reference" {
					message, suggestion = "behavioral reference domain is bound more than once", "bind each semantic reference exactly once"
				}
				issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, message, suggestion))
			}
			seenDomains[port.Domain] = true
		}
		if node.Domain != port.Domain || node.Role != role {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "nodes."+node.ID, "external graph-node domain or role differs from its behavioral port", "rebuild the graph from the normalized requirement"))
		}
	}
	for portID := range expectedPorts {
		if !seenPorts[portID] {
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "requirements.ports."+portID, "behavioral port is absent from the candidate graph", "connect every required external interface"))
		}
	}
	for domainID, domain := range expectedDomains {
		if !seenDomains[domainID] {
			message, suggestion := "behavioral source domain is absent from the candidate graph", "materialize every required semantic source"
			if domain.Kind == "reference" {
				message, suggestion = "behavioral reference domain is absent from the candidate graph", "materialize every required semantic reference"
			}
			issues = append(issues, graphIssue(CodeNoCompleteGraph, "requirements.domains."+domainID, message, suggestion))
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
			if reviewedPrimitiveModelSupportsCircuitAnalysis(model, analysis) {
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
				relativeError = multiplicativeRelativeError(value, seed.value)
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
		relativeError := multiplicativeRelativeError(value, scale.ValueSI)
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

// multiplicativeRelativeError measures engineering values symmetrically over
// their naturally logarithmic range. A value ten times above or below its
// analytic scale therefore carries the same error instead of systematically
// favoring undersized parts when the catalog has sparse decade coverage.
func multiplicativeRelativeError(value, scale float64) float64 {
	if value <= 0 || scale <= 0 {
		return math.Inf(1)
	}
	return math.Max(value/scale, scale/value) - 1
}

// catalogResistanceDivider finds the deterministic reviewed resistor pair
// whose ratio most closely realizes a derived divider ratio. This keeps
// coupled divider values electrically coherent when the concrete catalog is
// sparse instead of rounding the two legs independently.
type catalogResistanceValue struct {
	nominal float64
	minimum float64
	maximum float64
}

func reviewedCatalogResistanceValues(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
) []catalogResistanceValue {
	requiredAnalyses := requirementAnalysisSet(requirement)
	valueByNominal := map[float64]catalogResistanceValue{}
	for _, primitive := range inventory {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!ratingsCoverRequirement(requirement, primitive) ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) {
			continue
		}
		value := 0.0
		switch {
		case primitive.ValueDomain.Nominal != nil:
			value = *primitive.ValueDomain.Nominal
		case primitive.ValueDomain.Minimum != nil && primitive.ValueDomain.Maximum != nil &&
			*primitive.ValueDomain.Minimum == *primitive.ValueDomain.Maximum:
			value = *primitive.ValueDomain.Minimum
		}
		if value < 1_000 || value > 1_000_000 {
			continue
		}
		tolerance, proven := primitiveTolerancePercent(primitive, "resistance")
		if !proven {
			continue
		}
		fraction := tolerance / 100
		candidate := catalogResistanceValue{
			nominal: value,
			minimum: value * (1 - fraction),
			maximum: value * (1 + fraction),
		}
		existing, exists := valueByNominal[value]
		if !exists || candidate.maximum-candidate.minimum < existing.maximum-existing.minimum {
			valueByNominal[value] = candidate
		}
	}
	values := make([]catalogResistanceValue, 0, len(valueByNominal))
	for _, value := range valueByNominal {
		values = append(values, value)
	}
	slices.SortFunc(values, func(left, right catalogResistanceValue) int {
		return cmp.Compare(left.nominal, right.nominal)
	})
	return values
}

func catalogResistanceDivider(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	targetRatio float64,
	anchorLower float64,
	lowerBranchCount int,
	minimumSupplyVoltage float64,
	maximumSupplyVoltage float64,
	minimumOutputVoltage float64,
	maximumOutputVoltage float64,
) (float64, []float64, bool) {
	if targetRatio <= 0 || anchorLower <= 0 || minimumSupplyVoltage <= 0 ||
		maximumSupplyVoltage < minimumSupplyVoltage || lowerBranchCount <= 0 {
		return 0, nil, false
	}
	values := reviewedCatalogResistanceValues(requirement, inventory)
	if !combinationCountWithinBudget(
		len(values),
		lowerBranchCount,
		maxCatalogResistanceDividerCombinations,
	) {
		return 0, nil, false
	}
	bestUpper := 0.0
	bestLower := []float64(nil)
	bestWorstRatioError, bestRatioError, bestAnchorError := math.Inf(1), math.Inf(1), math.Inf(1)
	lowerCombinations := [][]catalogResistanceValue{}
	indices := make([]int, lowerBranchCount)
	for {
		combination := make([]catalogResistanceValue, lowerBranchCount)
		for index, valueIndex := range indices {
			combination[index] = values[valueIndex]
		}
		lowerCombinations = append(lowerCombinations, combination)
		position := len(indices) - 1
		for position >= 0 && indices[position] == len(values)-1 {
			position--
		}
		if position < 0 {
			break
		}
		next := indices[position] + 1
		for index := position; index < len(indices); index++ {
			indices[index] = next
		}
	}
	for _, upper := range values {
		for _, lowerBranches := range lowerCombinations {
			nominalConductance, minimumConductance, maximumConductance := 0.0, 0.0, 0.0
			for _, lower := range lowerBranches {
				nominalConductance += 1 / lower.nominal
				minimumConductance += 1 / lower.maximum
				maximumConductance += 1 / lower.minimum
			}
			lowerNominal := 1 / nominalConductance
			lowerMinimum := 1 / maximumConductance
			lowerMaximum := 1 / minimumConductance
			minimumDividerOutput := minimumSupplyVoltage * lowerMinimum / (upper.maximum + lowerMinimum)
			maximumDividerOutput := maximumSupplyVoltage * lowerMaximum / (upper.minimum + lowerMaximum)
			if (minimumOutputVoltage > 0 && minimumDividerOutput < minimumOutputVoltage) ||
				(maximumOutputVoltage > 0 && maximumDividerOutput > maximumOutputVoltage) {
				continue
			}
			ratioError := multiplicativeRelativeError(upper.nominal/lowerNominal, targetRatio)
			worstRatioError := math.Max(
				multiplicativeRelativeError(upper.maximum/lowerMinimum, targetRatio),
				multiplicativeRelativeError(upper.minimum/lowerMaximum, targetRatio),
			)
			anchorError := multiplicativeRelativeError(lowerNominal, anchorLower)
			branchNominals := make([]float64, len(lowerBranches))
			for index, lower := range lowerBranches {
				branchNominals[index] = lower.nominal
			}
			if worstRatioError < bestWorstRatioError ||
				(worstRatioError == bestWorstRatioError &&
					(ratioError < bestRatioError ||
						(ratioError == bestRatioError &&
							(anchorError < bestAnchorError ||
								(anchorError == bestAnchorError &&
									(upper.nominal < bestUpper ||
										(upper.nominal == bestUpper && slices.Compare(branchNominals, bestLower) < 0))))))) {
				bestUpper, bestLower = upper.nominal, branchNominals
				bestWorstRatioError, bestRatioError, bestAnchorError = worstRatioError, ratioError, anchorError
			}
		}
	}
	return bestUpper, bestLower, bestUpper > 0 && len(bestLower) == lowerBranchCount
}

// catalogControllerFeedbackDivider chooses a reviewed resistor pair whose
// complete tolerance envelope keeps a controller-referenced output inside the
// declared voltage bounds. The equilibrium is Vout=Vref*(1+Rupper/Rlower), so
// both controller-reference uncertainty and resistor tolerance participate in
// the ranking instead of being deferred to a repair that may never enumerate
// the coupled pair.
func catalogControllerFeedbackDivider(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	referenceNominal, referenceMinimum, referenceMaximum float64,
	outputMinimum, outputMaximum, anchorLower float64,
) (float64, float64, bool) {
	if referenceNominal <= 0 || referenceMinimum <= 0 ||
		referenceMaximum < referenceMinimum || outputMinimum <= referenceMaximum ||
		outputMaximum < outputMinimum || anchorLower <= 0 {
		return 0, 0, false
	}
	values := reviewedCatalogResistanceValues(requirement, inventory)
	if !combinationCountWithinBudget(len(values), 2, maxCatalogResistanceDividerCombinations) {
		return 0, 0, false
	}
	targetOutput := (outputMinimum + outputMaximum) / 2
	bestUpper, bestLower := 0.0, 0.0
	bestNominalError, bestCornerImbalance, bestAnchorError := math.Inf(1), math.Inf(1), math.Inf(1)
	for _, upper := range values {
		for _, lower := range values {
			minimumOutput := referenceMinimum * (1 + upper.minimum/lower.maximum)
			maximumOutput := referenceMaximum * (1 + upper.maximum/lower.minimum)
			if minimumOutput < outputMinimum || maximumOutput > outputMaximum {
				continue
			}
			nominalOutput := referenceNominal * (1 + upper.nominal/lower.nominal)
			nominalError := math.Abs(nominalOutput - targetOutput)
			cornerImbalance := math.Abs((minimumOutput - outputMinimum) - (outputMaximum - maximumOutput))
			anchorError := multiplicativeRelativeError(lower.nominal, anchorLower)
			if nominalError < bestNominalError ||
				nominalError == bestNominalError && (cornerImbalance < bestCornerImbalance ||
					cornerImbalance == bestCornerImbalance && (anchorError < bestAnchorError ||
						anchorError == bestAnchorError && (upper.nominal < bestUpper ||
							upper.nominal == bestUpper && lower.nominal < bestLower))) {
				bestUpper, bestLower = upper.nominal, lower.nominal
				bestNominalError, bestCornerImbalance, bestAnchorError = nominalError, cornerImbalance, anchorError
			}
		}
	}
	if bestUpper <= 0 || bestLower <= 0 {
		return 0, 0, false
	}
	return bestUpper, bestLower, true
}

// catalogControllerSeriesFeedbackDivider extends the same controller-bound
// tolerance search to a two-part series upper leg. Sparse reviewed catalogs
// commonly lack one exact ratio even though two catalog resistors realize it
// safely. Enumeration remains explicitly bounded and deterministic.
func catalogControllerSeriesFeedbackDivider(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	referenceNominal, referenceMinimum, referenceMaximum float64,
	outputMinimum, outputMaximum, anchorLower float64,
) (float64, float64, float64, bool) {
	if referenceNominal <= 0 || referenceMinimum <= 0 ||
		referenceMaximum < referenceMinimum || outputMinimum <= referenceMaximum ||
		outputMaximum < outputMinimum || anchorLower <= 0 {
		return 0, 0, 0, false
	}
	values := reviewedCatalogResistanceValues(requirement, inventory)
	if !catalogSeriesFeedbackDividerSearchWithinBudget(
		len(values), maxCatalogResistanceDividerCombinations,
	) {
		return 0, 0, 0, false
	}
	targetOutput := (outputMinimum + outputMaximum) / 2
	bestFirst, bestSecond, bestLower := 0.0, 0.0, 0.0
	bestNominalError, bestCornerImbalance, bestAnchorError := math.Inf(1), math.Inf(1), math.Inf(1)
	for firstIndex, first := range values {
		for _, second := range values[firstIndex:] {
			for _, lower := range values {
				minimumOutput := referenceMinimum * (1 + (first.minimum+second.minimum)/lower.maximum)
				maximumOutput := referenceMaximum * (1 + (first.maximum+second.maximum)/lower.minimum)
				if minimumOutput < outputMinimum || maximumOutput > outputMaximum {
					continue
				}
				nominalOutput := referenceNominal * (1 + (first.nominal+second.nominal)/lower.nominal)
				nominalError := math.Abs(nominalOutput - targetOutput)
				cornerImbalance := math.Abs((minimumOutput - outputMinimum) - (outputMaximum - maximumOutput))
				anchorError := multiplicativeRelativeError(lower.nominal, anchorLower)
				if nominalError < bestNominalError ||
					nominalError == bestNominalError && (cornerImbalance < bestCornerImbalance ||
						cornerImbalance == bestCornerImbalance && (anchorError < bestAnchorError ||
							anchorError == bestAnchorError && (first.nominal < bestFirst ||
								first.nominal == bestFirst && (second.nominal < bestSecond ||
									second.nominal == bestSecond && lower.nominal < bestLower)))) {
					bestFirst, bestSecond, bestLower = first.nominal, second.nominal, lower.nominal
					bestNominalError, bestCornerImbalance, bestAnchorError = nominalError, cornerImbalance, anchorError
				}
			}
		}
	}
	if bestFirst <= 0 || bestSecond <= 0 || bestLower <= 0 {
		return 0, 0, 0, false
	}
	return bestFirst, bestSecond, bestLower, true
}

// catalogSeriesFeedbackDividerSearchWithinBudget bounds the exact loop shape:
// an unordered two-resistor upper leg with repetition, multiplied by every
// possible lower-leg value. This is n*C(n+1,2), not C(n+2,3), because the
// lower leg has a distinct electrical role.
func catalogSeriesFeedbackDividerSearchWithinBudget(valueCount, budget int) bool {
	if valueCount <= 0 || budget <= 0 || valueCount > budget {
		return false
	}
	return combinationCountWithinBudget(valueCount, 2, budget/valueCount)
}

// combinationCountWithinBudget reports whether selecting branchCount values
// with repetition from valueCount catalog entries stays within the explicit
// deterministic work budget. It avoids overflow by rejecting before the next
// binomial recurrence can exceed the budget.
func combinationCountWithinBudget(valueCount, branchCount, budget int) bool {
	if valueCount <= 0 || branchCount <= 0 || budget <= 0 || branchCount > budget {
		return false
	}
	count := 1
	for branch := 1; branch <= branchCount; branch++ {
		factor := valueCount + branch - 1
		if count > budget*branch/factor {
			return false
		}
		count = count * factor / branch
	}
	return count <= budget
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
		case "transimpedance":
			if value > 0 {
				add(AnalyticScale{ID: source + ":transimpedance_resistance", Kind: "resistance", ValueSI: value, Unit: "ohm", Derivation: "R=V/I dimensional scale", SourceKind: "behavioral_assertion", SourceID: assertion.ID, Priority: 5})
			}
		case "voltage_gain", "voltage_gain_at_frequency":
			addRatioDerivedResistanceScales(&result, source, assertion.ID, "gain ratio applied to neutral resistance anchor", value, anchorResistance, 20)
		case "cutoff_frequency", "bandwidth":
			if assertion.Metric == "bandwidth" && assertion.Min != nil {
				value = *assertion.Min * minimumBandwidthDesignFactor
			}
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
	convergentDigitalScales ...map[string][]AnalyticScale,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	supply := nominalSupplyVoltage(requirement)
	if supply <= 0 {
		return nil
	}
	convergent := map[string][]AnalyticScale(nil)
	if len(convergentDigitalScales) != 0 {
		convergent = convergentDigitalScales[0]
	} else {
		convergent = deriveConvergentDigitalTopologyScales(requirement, graph)
	}
	if scales := convergent[instance.ID]; len(scales) != 0 {
		return scales
	}
	if scales := deriveAutonomousPeriodicTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveBoundedBipolarTopologyScales(
		requirement,
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveFrequencySelectiveTopologyScales(
		requirement,
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveBandpassTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveRegulatedVoltageTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := derivePowerTransferTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveWindowedBallastedSwitchTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveWindowedControlledCurrentTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveWindowTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveConditionalTransferTopologyScales(
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveFullWaveTopologyScales(
		requirement,
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveAnalogTransferTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveTransimpedanceTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveTransconductanceTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveRegulatedLoadRailTopologyScales(
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveCurrentLimitedSwitchTopologyScales(
		graph,
		instance,
	); len(scales) != 0 {
		return scales
	}
	if scales := deriveControlledSwitchTopologyScales(
		requirement,
		graph,
		instance,
		inventory,
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
	seriesResistorPath := func(start, end string) []string {
		paths := [][]string{}
		for _, node := range graph.Nodes {
			if node.Scope != "internal" || node.Role != "internal" ||
				node.ID == start || node.ID == end {
				continue
			}
			first, second := "", ""
			for _, candidate := range graph.Instances {
				if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
					continue
				}
				left, right := candidate.Terminals[0].Node, candidate.Terminals[1].Node
				switch {
				case left == start && right == node.ID || right == start && left == node.ID:
					first = candidate.ID
				case left == node.ID && right == end || right == node.ID && left == end:
					second = candidate.ID
				}
			}
			if first != "" && second != "" && first != second {
				path := []string{first, second}
				slices.Sort(path)
				paths = append(paths, path)
			}
		}
		slices.SortFunc(paths, func(left, right []string) int { return slices.Compare(left, right) })
		if len(paths) == 0 {
			return nil
		}
		return paths[0]
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
	decisionOutputLow, decisionOutputHigh := 0.0, supply
	decisionFeedbackRatio := (upper - lower) / supply
	decisionSeriesValues := map[string]float64{}
	for _, active := range graph.Instances {
		if active.Kind != "comparator" && active.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		if nodeByID[terminals["OUT"]].Role != "output" ||
			nodeByID[terminals["IN_PLUS"]].Scope != "internal" {
			continue
		}
		branches := resistorBranches(terminals["IN_PLUS"])
		seriesFeedback := []string(nil)
		if branches["output"] == "" {
			seriesFeedback = seriesResistorPath(terminals["IN_PLUS"], terminals["OUT"])
		}
		if branches["input"] == "" || branches["output"] == "" && len(seriesFeedback) != 2 {
			continue
		}
		if modeledLow, modeledHigh, ok := topologyDecisionOutputSwing(
			requirement,
			graph,
			active,
			inventory,
		); ok {
			decisionOutputLow, decisionOutputHigh = modeledLow, modeledHigh
		}
		outputSpan := decisionOutputHigh - decisionOutputLow
		if outputSpan <= 0 {
			break
		}
		idealRatio := (upper - lower) / outputSpan
		inputResistance, inputOK := topologyCatalogResistanceClosest(
			requirement,
			inventory,
			anchorResistance,
		)
		feedbackResistance, feedbackOK := 0.0, false
		if len(seriesFeedback) == 2 {
			leftID, rightID := seriesFeedback[0], seriesFeedback[1]
			leftInstance := graph.Instances[graphInstanceIndex(graph, leftID)]
			rightInstance := graph.Instances[graphInstanceIndex(graph, rightID)]
			left, right, found := catalogSeriesResistancePairPreservingBranch(
				requirement,
				inventory,
				anchorResistance/idealRatio,
				leftInstance,
				rightInstance,
			)
			if found {
				decisionSeriesValues[leftID] = left
				decisionSeriesValues[rightID] = right
				feedbackResistance, feedbackOK = left+right, true
			}
		} else {
			feedbackResistance, feedbackOK = topologyCatalogResistanceClosest(
				requirement,
				inventory,
				anchorResistance/idealRatio,
			)
		}
		if inputOK && feedbackOK && feedbackResistance > 0 {
			decisionFeedbackRatio = inputResistance / feedbackResistance
		} else {
			decisionFeedbackRatio = idealRatio
		}
		break
	}
	if decisionFeedbackRatio <= 0 || !finite(decisionFeedbackRatio) {
		return nil
	}
	decisionReference := (upper + decisionFeedbackRatio*decisionOutputLow) /
		(1 + decisionFeedbackRatio)
	if value := decisionSeriesValues[instance.ID]; value > 0 && finite(value) {
		return []AnalyticScale{{
			ID:         "topology:decision_threshold_series_feedback:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    value,
			Unit:       "ohm",
			Derivation: "catalog-ranked series feedback composition derived from bounded decision thresholds and modeled output swing",
			SourceKind: "candidate_topology",
			SourceID:   instance.ID,
			Priority:   1,
		}}
	}
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
		if active.Kind == "comparator" &&
			resistorNeighbors(terminals["OUT"])[terminals["V_PLUS"]] == instance.ID {
			return []AnalyticScale{{
				ID:         "topology:decision_pullup:" + terminals["OUT"] + ":" + instance.ID,
				Kind:       "resistance",
				ValueSI:    anchorResistance,
				Unit:       "ohm",
				Derivation: "bounded pull-up for a reviewed open-collector decision primitive",
				SourceKind: "candidate_topology",
				SourceID:   terminals["OUT"],
				Priority:   1,
			}}
		}
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
					positiveBranches["output"]: anchorResistance / decisionFeedbackRatio,
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

// deriveConvergentDigitalTopologyScales recognizes explicit primitive roles
// in a multi-input digital decision graph once per candidate. It preserves the
// catalog-ranked value selected for each bounded input and bias resistor
// without changing value ordering for unrelated topology families.
func deriveConvergentDigitalTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
) map[string][]AnalyticScale {
	result := map[string][]AnalyticScale{}
	groups := declarativeBinaryDecisionGroups(requirement)
	if len(groups) == 0 {
		return result
	}
	for _, decisions := range groups {
		output := "port_" + decisions[0].observation
		inputNodes := map[string]bool{}
		for _, decision := range decisions {
			inputNodes["port_"+decision.excitation] = true
		}
		resistors := []GraphInstance{}
		levelShifters := []GraphInstance{}
		sourcesByGate := map[string][]GraphInstance{}
		inputsByBase := map[string]map[string]bool{}
		for _, candidate := range graph.Instances {
			switch candidate.Kind {
			case "resistor":
				if candidate.ValueSI == nil || len(candidate.Terminals) != 2 {
					continue
				}
				resistors = append(resistors, candidate)
				ends := topologySortedTwoTerminalNodes(candidate)
				input, base := "", ""
				if inputNodes[ends[0]] {
					input, base = ends[0], ends[1]
				} else if inputNodes[ends[1]] {
					input, base = ends[1], ends[0]
				}
				if input != "" {
					if inputsByBase[base] == nil {
						inputsByBase[base] = map[string]bool{}
					}
					inputsByBase[base][input] = true
				}
			case "npn_bjt":
				levelShifters = append(levelShifters, candidate)
			case "p_channel_mosfet":
				pmos := topologyTerminalNodes(candidate)
				if pmos["GATE"] != "" && pmos["DRAIN"] == output {
					sourcesByGate[pmos["GATE"]] = append(sourcesByGate[pmos["GATE"]], candidate)
				}
			}
		}
		for _, levelShifter := range levelShifters {
			npn := topologyTerminalNodes(levelShifter)
			reference, found := graphNodeByID(graph, npn["EMITTER"])
			if !found || reference.Role != "reference" {
				continue
			}
			sources := sourcesByGate[npn["COLLECTOR"]]
			if npn["COLLECTOR"] == "" || len(sources) != 1 ||
				len(inputsByBase[npn["BASE"]]) != len(inputNodes) {
				continue
			}
			pmos := topologyTerminalNodes(sources[0])
			if pmos["SOURCE"] == "" {
				continue
			}
			for _, instance := range resistors {
				ends := topologySortedTwoTerminalNodes(instance)
				role := ""
				switch {
				case (ends[0] == npn["BASE"] && inputNodes[ends[1]]) ||
					(ends[1] == npn["BASE"] && inputNodes[ends[0]]):
					role = "input_isolation"
				case (ends[0] == npn["COLLECTOR"] && ends[1] == pmos["SOURCE"]) ||
					(ends[1] == npn["COLLECTOR"] && ends[0] == pmos["SOURCE"]):
					role = "gate_bias"
				case (ends[0] == output && ends[1] == npn["EMITTER"]) ||
					(ends[1] == output && ends[0] == npn["EMITTER"]):
					role = "output_bias"
				}
				if role == "" {
					continue
				}
				result[instance.ID] = []AnalyticScale{{
					ID:         "topology:convergent_digital:" + role + ":" + instance.ID,
					Kind:       "resistance",
					ValueSI:    *instance.ValueSI,
					Unit:       "ohm",
					Derivation: "catalog-ranked bounded value for an explicit multi-input digital decision role",
					SourceKind: "topology",
					SourceID:   instance.ID,
					Priority:   1,
				}}
			}
		}
	}
	return result
}

func topologySortedTwoTerminalNodes(instance GraphInstance) [2]string {
	result := [2]string{instance.Terminals[0].Node, instance.Terminals[1].Node}
	if result[1] < result[0] {
		result[0], result[1] = result[1], result[0]
	}
	return result
}

// deriveCurrentLimitedSwitchTopologyScales preserves the catalog values that
// jointly define a source shunt and the two reference dividers in a
// current-limited switch. The topology is recognized structurally by two
// open-collector decisions sharing a MOSFET gate, with one decision observing
// the MOSFET source. Gate pull values remain owned by the ordinary controlled
// switch derivation so device gate limits continue to set that network.
func deriveCurrentLimitedSwitchTopologyScales(
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if instance.Kind != "resistor" || instance.ValueSI == nil ||
		*instance.ValueSI <= 0 || !finite(*instance.ValueSI) {
		return nil
	}
	for _, controlled := range graph.Instances {
		if controlled.Kind != "n_channel_mosfet" {
			continue
		}
		controlledTerminals := topologyTerminalNodes(controlled)
		gate, sense := controlledTerminals["GATE"], controlledTerminals["SOURCE"]
		if gate == "" || sense == "" {
			continue
		}
		decisions := []GraphInstance{}
		for _, decision := range graph.Instances {
			if decision.Kind == "comparator" && topologyTerminalNodes(decision)["OUT"] == gate {
				decisions = append(decisions, decision)
			}
		}
		if len(decisions) != 2 {
			continue
		}
		thresholds := map[string]bool{}
		currentThreshold := ""
		currentDecisionFound, commandDecisionFound := false, false
		for _, decision := range decisions {
			terminals := topologyTerminalNodes(decision)
			switch {
			case terminals["IN_MINUS"] == sense:
				currentThreshold = terminals["IN_PLUS"]
				thresholds[currentThreshold] = true
				currentDecisionFound = true
			case terminals["IN_PLUS"] == sense:
				currentThreshold = terminals["IN_MINUS"]
				thresholds[currentThreshold] = true
				currentDecisionFound = true
			default:
				for _, input := range []string{terminals["IN_PLUS"], terminals["IN_MINUS"]} {
					for _, node := range graph.Nodes {
						if node.ID == input && node.Scope == "external" &&
							(node.Role == "control" || node.Role == "input") {
							other := terminals["IN_PLUS"]
							if other == input {
								other = terminals["IN_MINUS"]
							}
							thresholds[other] = true
							commandDecisionFound = true
						}
					}
				}
			}
		}
		if !currentDecisionFound || !commandDecisionFound {
			continue
		}
		feedbackPath := topologyInternalResistorPath(graph, gate, currentThreshold)
		if slices.Contains(feedbackPath, instance.ID) {
			return []AnalyticScale{{
				ID:         "topology:current_limited_switch_hysteresis:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    *instance.ValueSI,
				Unit:       "ohm",
				Derivation: "catalog-selected positive feedback keeps the current decision bistable below the bounded peak-current limit",
				SourceKind: "candidate_topology",
				SourceID:   controlled.ID,
				Priority:   1,
			}}
		}
		first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
		if first != sense && second != sense && !thresholds[first] && !thresholds[second] {
			continue
		}
		if first == gate || second == gate {
			continue
		}
		return []AnalyticScale{{
			ID:         "topology:current_limited_switch_role:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    *instance.ValueSI,
			Unit:       "ohm",
			Derivation: "catalog-selected current shunt and decision-reference values realize the bounded peak-current relationship",
			SourceKind: "candidate_topology",
			SourceID:   controlled.ID,
			Priority:   1,
		}}
	}
	return nil
}

func topologyInternalResistorPath(graph CandidateGraph, start, end string) []string {
	if start == "" || end == "" || start == end {
		return nil
	}
	internal := map[string]bool{start: true, end: true}
	for _, node := range graph.Nodes {
		if node.Scope == "internal" {
			internal[node.ID] = true
		}
	}
	type step struct {
		node      string
		instances []string
	}
	adjacency := map[string][]step{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
			continue
		}
		left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
		if !internal[left] || !internal[right] {
			continue
		}
		adjacency[left] = append(adjacency[left], step{node: right, instances: []string{instance.ID}})
		adjacency[right] = append(adjacency[right], step{node: left, instances: []string{instance.ID}})
	}
	for node := range adjacency {
		slices.SortFunc(adjacency[node], func(left, right step) int {
			return cmp.Or(cmp.Compare(left.node, right.node), cmp.Compare(left.instances[0], right.instances[0]))
		})
	}
	queue := []step{{node: start}}
	visited := map[string]bool{start: true}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range adjacency[current.node] {
			if visited[edge.node] {
				continue
			}
			path := append(append([]string(nil), current.instances...), edge.instances[0])
			if edge.node == end {
				return path
			}
			visited[edge.node] = true
			queue = append(queue, step{node: edge.node, instances: path})
		}
	}
	return nil
}

func deriveAutonomousPeriodicTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	behavior, required := topologyAutonomousPeriodicBehavior(requirement)
	if !required || !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	outputID := behavior.output
	output := "port_" + outputID
	between := func(candidate GraphInstance, left, right string) bool {
		return len(candidate.Terminals) == 2 &&
			(candidate.Terminals[0].Node == left && candidate.Terminals[1].Node == right ||
				candidate.Terminals[0].Node == right && candidate.Terminals[1].Node == left)
	}
	var comparator GraphInstance
	for _, candidate := range graph.Instances {
		terminals := topologyTerminalNodes(candidate)
		if (candidate.Kind == "comparator" || candidate.Kind == "opamp") && terminals["OUT"] == output &&
			terminals["IN_PLUS"] != "" && terminals["IN_MINUS"] != "" {
			comparator = candidate
			break
		}
	}
	if comparator.ID == "" {
		return nil
	}
	terminals := topologyTerminalNodes(comparator)
	threshold, timing := terminals["IN_PLUS"], terminals["IN_MINUS"]
	highRail, reference := terminals["V_PLUS"], terminals["V_MINUS"]
	minimumLoad := math.Inf(1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "load_resistance" && condition.Target == outputID && condition.Min > 0 {
				minimumLoad = math.Min(minimumLoad, condition.Min)
			}
		}
	}
	resistanceAnchor := 10_000.0
	if finite(minimumLoad) {
		resistanceAnchor = math.Max(resistanceAnchor, 5*minimumLoad)
	}
	timingResistance, timingCapacitance, pairOK := 0.0, 0.0, false
	if behavior.timingPort != "" {
		timingResistance, pairOK = topologyExternalTimingResistance(requirement, behavior)
	} else {
		oscillatorCutoff := behavior.frequency * math.Log(2) / math.Pi
		timingResistance, timingCapacitance, pairOK = catalogRCPair(
			requirement,
			inventory,
			oscillatorCutoff,
			resistanceAnchor,
		)
	}
	if !pairOK {
		return nil
	}
	thresholdResistance := resistanceAnchor
	isolationResistance := 10 * thresholdResistance
	result := AnalyticScale{
		Kind:       "resistance",
		Unit:       "ohm",
		SourceKind: "candidate_topology",
		SourceID:   outputID,
		Priority:   1,
	}
	switch {
	case instance.Kind == "resistor" && between(instance, output, timing):
		result.ID = "topology:autonomous_periodic_timing_resistance:" + instance.ID
		result.ValueSI = timingResistance
		result.Derivation = "catalog R-C pair realizes the bounded autonomous oscillation frequency"
	case instance.Kind == "capacitor" && between(instance, timing, reference):
		result.ID = "topology:autonomous_periodic_timing_capacitance:" + instance.ID
		result.Kind, result.Unit = "capacitance", "F"
		result.ValueSI = timingCapacitance
		result.Derivation = "catalog R-C pair realizes the bounded autonomous oscillation frequency"
	case instance.Kind == "resistor" &&
		(between(instance, output, threshold) || between(instance, highRail, threshold) || between(instance, threshold, reference)):
		result.ID = "topology:autonomous_periodic_threshold:" + instance.ID
		result.ValueSI = thresholdResistance
		result.Derivation = "equal conductances establish symmetric one-third and two-thirds hysteresis thresholds"
	case instance.Kind == "resistor" && between(instance, output, highRail):
		if !finite(minimumLoad) {
			return nil
		}
		low, high, swingOK := topologyDecisionOutputSwing(requirement, graph, comparator, inventory)
		requiredSwing := requirementMinimumMetric(requirement, "output_swing")
		maximumPullup := 0.0
		if swingOK && requiredSwing > 0 && high-low > requiredSwing {
			maximumPullup = minimumLoad * ((high-low)/requiredSwing - 1)
		}
		if maximumPullup <= 0 {
			return nil
		}
		result.ID = "topology:autonomous_periodic_pullup:" + instance.ID
		result.ValueSI = .5 * maximumPullup
		result.Derivation = "pull-up resistance preserves the declared loaded output swing with deterministic headroom"
	case instance.Kind == "resistor" && between(instance, threshold, "port_"+topologyAutonomousControlPort(requirement)):
		result.ID = "topology:autonomous_periodic_enable_isolation:" + instance.ID
		result.ValueSI = isolationResistance
		result.Derivation = "enable isolation is one decade above the hysteresis network impedance"
	default:
		return nil
	}
	if result.ValueSI <= 0 || !finite(result.ValueSI) {
		return nil
	}
	return []AnalyticScale{result}
}

func topologyAutonomousControlPort(requirement Requirement) string {
	for _, port := range requirement.Requirements.Ports {
		if port.Kind == "digital" && port.Direction == "sink" {
			return port.ID
		}
	}
	return ""
}

func deriveBoundedBipolarTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
		return nil
	}
	envelope, required := topologyBoundedBipolarEnvelope(requirement)
	if !required {
		return nil
	}
	input, output := "port_"+envelope.input, "port_"+envelope.output
	between := func(candidate GraphInstance, left, right string) bool {
		return len(candidate.Terminals) == 2 &&
			(candidate.Terminals[0].Node == left && candidate.Terminals[1].Node == right ||
				candidate.Terminals[0].Node == right && candidate.Terminals[1].Node == left)
	}
	parallel := []string{}
	for _, candidate := range graph.Instances {
		if candidate.Kind == "resistor" && between(candidate, input, output) {
			parallel = append(parallel, candidate.ID)
		}
	}
	if len(parallel) == 2 && slices.Contains(parallel, instance.ID) {
		currentLimit := topologyBipolarReferenceCurrentLimit(requirement)
		minimumEffective := (envelope.maximumInputAbsV - envelope.maximumOutputAbsV) / currentLimit
		maximumEffective := envelope.minimumLoadOhm * (1/envelope.minimumGain - 1)
		if currentLimit > 0 && minimumEffective > 0 && maximumEffective >= minimumEffective {
			return []AnalyticScale{{
				ID:         "topology:bounded_bipolar_series:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    2 * geometricMean(minimumEffective, maximumEffective),
				Unit:       "ohm",
				Derivation: "equal parallel branches realize the bounded clamp-current and passband-gain resistance interval",
				SourceKind: "candidate_topology",
				SourceID:   envelope.output,
				Priority:   1,
			}}
		}
	}
	references := topologyNodesByRole(graph, "reference")
	if len(references) == 1 && between(instance, output, references[0]) {
		return []AnalyticScale{{
			ID:         "topology:bounded_bipolar_bias:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    math.Max(1e6, envelope.minimumLoadOhm*100),
			Unit:       "ohm",
			Derivation: "high-impedance output return derived from the minimum declared load",
			SourceKind: "candidate_topology",
			SourceID:   envelope.output,
			Priority:   1,
		}}
	}
	return nil
}

func seriesPairAssignmentError(instance GraphInstance, value float64) float64 {
	if instance.ValueSI == nil || *instance.ValueSI <= 0 {
		return 0
	}
	return multiplicativeRelativeError(*instance.ValueSI, value)
}

func topologyCatalogResistanceClosest(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	target float64,
) (float64, bool) {
	bestValue := 0.0
	bestError := math.Inf(1)
	bestKey := ""
	for _, primitive := range inventory {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		domain := *primitive.ValueDomain
		minimum, maximum, ok := effectiveValueRange(domain)
		if !ok {
			continue
		}
		candidate := min(max(target, minimum), maximum)
		if minimum != maximum {
			tolerance, toleranceProven := primitiveTolerancePercent(primitive, domain.Kind)
			if requirement.Acceptance.RequireAllCorners && !toleranceProven {
				continue
			}
			preferred, issues := architecturesearch.PreferredValueCandidates(
				candidate,
				preferredSeriesForDomain(domain.Kind, tolerance, toleranceProven),
				minimum,
				maximum,
				1,
			)
			if len(issues) != 0 || len(preferred) == 0 {
				continue
			}
			candidate = preferred[0]
		}
		error := multiplicativeRelativeError(candidate, target)
		if error < bestError ||
			(error == bestError && (candidate < bestValue ||
				(candidate == bestValue && (bestKey == "" || primitive.Key < bestKey)))) {
			bestValue = candidate
			bestError = error
			bestKey = primitive.Key
		}
	}
	return bestValue, bestKey != ""
}

// topologyDecisionOutputSwing returns the nominal output limits of the selected
// push-pull decision primitive when its reviewed model covers the declared
// supply domain. Open-collector stages deliberately fall back to their rail
// span because their high state is established by the surrounding pull-up
// branch and their low state depends on that branch's current. Simulation
// remains authoritative for both forms.
func topologyDecisionOutputSwing(
	requirement Requirement,
	graph CandidateGraph,
	active GraphInstance,
	inventory map[string]PrimitiveCandidate,
) (float64, float64, bool) {
	if active.Kind != "opamp" {
		return 0, 0, false
	}
	terminals := topologyTerminalNodes(active)
	negative, negativeOK := topologyNodeNominalVoltage(
		requirement,
		graph,
		terminals["V_MINUS"],
	)
	positive, positiveOK := topologyNodeNominalVoltage(
		requirement,
		graph,
		terminals["V_PLUS"],
	)
	if !negativeOK || !positiveOK || positive <= negative {
		return 0, 0, false
	}
	minimumSupplySpan, maximumSupplySpan := positive-negative, positive-negative
	if negativeMinimum, negativeMaximum, ok := topologyDeclaredNodeVoltageRange(
		requirement,
		graph,
		terminals["V_MINUS"],
	); ok {
		if positiveMinimum, positiveMaximum, positiveRangeOK := topologyDeclaredNodeVoltageRange(
			requirement,
			graph,
			terminals["V_PLUS"],
		); positiveRangeOK {
			minimumSupplySpan = positiveMinimum - negativeMaximum
			maximumSupplySpan = positiveMaximum - negativeMinimum
		}
	}
	if minimumSupplySpan <= 0 || maximumSupplySpan < minimumSupplySpan {
		return 0, 0, false
	}
	primitive, found := inventory[active.PrimitiveKey]
	if !found {
		return 0, 0, false
	}
	return primitiveDecisionOutputSwing(
		negative,
		positive,
		minimumSupplySpan,
		maximumSupplySpan,
		primitive,
	)
}

func topologyDeclaredNodeVoltageRange(
	requirement Requirement,
	graph CandidateGraph,
	nodeID string,
) (float64, float64, bool) {
	domainID := ""
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			domainID = node.Domain
			break
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID != domainID {
			continue
		}
		minimum, maximum := 0.0, 0.0
		haveMinimum, haveMaximum := false, false
		if domain.MinVoltageV != nil {
			minimum, haveMinimum = *domain.MinVoltageV, true
		}
		if domain.MaxVoltageV != nil {
			maximum, haveMaximum = *domain.MaxVoltageV, true
		}
		if domain.NominalVoltageV != nil {
			if !haveMinimum {
				minimum, haveMinimum = *domain.NominalVoltageV, true
			}
			if !haveMaximum {
				maximum, haveMaximum = *domain.NominalVoltageV, true
			}
		}
		return minimum, maximum, haveMinimum && haveMaximum && maximum >= minimum
	}
	return 0, 0, false
}

// primitiveSupplySpanCoversConnectedRails rejects a catalog variant when its
// reviewed supply model cannot cover the full declared voltage difference
// between the instance's positive and negative supply terminals. Components
// without that terminal contract or without supply bounds remain eligible.
func primitiveSupplySpanCoversConnectedRails(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	primitive PrimitiveCandidate,
) bool {
	terminals := topologyTerminalNodes(instance)
	negativeNode, haveNegative := terminals["V_MINUS"]
	positiveNode, havePositive := terminals["V_PLUS"]
	if !haveNegative || !havePositive {
		return true
	}
	negativeMinimum, negativeMaximum, negativeOK := topologyDeclaredNodeVoltageRange(requirement, graph, negativeNode)
	positiveMinimum, positiveMaximum, positiveOK := topologyDeclaredNodeVoltageRange(requirement, graph, positiveNode)
	if !negativeOK || !positiveOK {
		return true
	}
	minimumSpan := positiveMinimum - negativeMaximum
	maximumSpan := positiveMaximum - negativeMinimum
	if minimumSpan <= 0 || maximumSpan < minimumSpan {
		return false
	}
	modelMinimum, modelMaximum := 0.0, math.Inf(1)
	haveMinimum, haveMaximum := false, false
	for _, model := range primitive.Models {
		for _, parameter := range model.Parameters {
			switch parameter.Name {
			case "supply_min_v":
				modelMinimum = math.Max(modelMinimum, parameter.Value)
				haveMinimum = true
			case "supply_max_v":
				modelMaximum = math.Min(modelMaximum, parameter.Value)
				haveMaximum = true
			}
		}
	}
	if !haveMinimum || !haveMaximum {
		return true
	}
	return modelMinimum <= minimumSpan && modelMaximum >= maximumSpan
}

func primitiveDecisionOutputSwing(
	negative float64,
	positive float64,
	minimumSupplySpan float64,
	maximumSupplySpan float64,
	primitive PrimitiveCandidate,
) (float64, float64, bool) {
	lowMargin, highMargin := 0.0, 0.0
	haveLow, haveHigh := false, false
	modelSupplyMinimum, modelSupplyMaximum := 0.0, math.Inf(1)
	haveSupplyMinimum, haveSupplyMaximum := false, false
	for _, model := range primitive.Models {
		for _, parameter := range model.Parameters {
			switch parameter.Name {
			case "output_low_margin_v":
				lowMargin = math.Max(lowMargin, parameter.Value)
				haveLow = true
			case "output_high_margin_v":
				highMargin = math.Max(highMargin, parameter.Value)
				haveHigh = true
			case "supply_min_v":
				modelSupplyMinimum = math.Max(modelSupplyMinimum, parameter.Value)
				haveSupplyMinimum = true
			case "supply_max_v":
				modelSupplyMaximum = math.Min(modelSupplyMaximum, parameter.Value)
				haveSupplyMaximum = true
			}
		}
		for _, uncertainty := range model.Uncertainties {
			switch uncertainty.Target {
			case "model_parameters.output_low_margin_v":
				lowMargin = math.Max(lowMargin, uncertainty.Maximum)
				haveLow = true
			case "model_parameters.output_high_margin_v":
				highMargin = math.Max(highMargin, uncertainty.Maximum)
				haveHigh = true
			}
		}
	}
	if !haveLow || !haveHigh || !haveSupplyMinimum || !haveSupplyMaximum ||
		modelSupplyMinimum > minimumSupplySpan ||
		modelSupplyMaximum < maximumSupplySpan {
		return 0, 0, false
	}
	low := negative + lowMargin
	high := positive - highMargin
	return low, high, finite(low) && finite(high) && high > low
}

func deriveFrequencySelectiveTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	rejectionFrequency, ok := topologyRejectionFrequency(requirement)
	if !ok {
		return nil
	}
	references := topologyNodesByRole(graph, "reference")
	if len(references) == 0 {
		return nil
	}
	reference := references[0]
	between := func(candidate GraphInstance, left, right string) bool {
		if len(candidate.Terminals) != 2 {
			return false
		}
		first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
		return first == left && second == right || first == right && second == left
	}
	otherNode := func(candidate GraphInstance, node string) string {
		if len(candidate.Terminals) != 2 {
			return ""
		}
		if candidate.Terminals[0].Node == node {
			return candidate.Terminals[1].Node
		}
		if candidate.Terminals[1].Node == node {
			return candidate.Terminals[0].Node
		}
		return ""
	}
	resistorPath := func(left, right string) []GraphInstance {
		for _, candidate := range graph.Instances {
			if candidate.Kind == "resistor" && between(candidate, left, right) {
				return []GraphInstance{candidate}
			}
		}
		for _, node := range graph.Nodes {
			if node.Scope != "internal" || node.ID == left || node.ID == right {
				continue
			}
			var first, second GraphInstance
			for _, candidate := range graph.Instances {
				if candidate.Kind != "resistor" {
					continue
				}
				if between(candidate, left, node.ID) {
					first = candidate
				}
				if between(candidate, node.ID, right) {
					second = candidate
				}
			}
			if first.ID != "" && second.ID != "" {
				return []GraphInstance{first, second}
			}
		}
		return nil
	}
	minimumInputResistance := 0.0
	for _, port := range requirement.Requirements.Ports {
		if port.Direction == "sink" && port.Electrical.InputImpedanceMinOhm != nil {
			minimumInputResistance = math.Max(minimumInputResistance, *port.Electrical.InputImpedanceMinOhm)
		}
	}
	resistance := math.Max(10_000, 1.5*minimumInputResistance)
	for _, resistiveMid := range graph.Nodes {
		if resistiveMid.Scope != "internal" {
			continue
		}
		for _, bridgeReference := range graph.Nodes {
			if bridgeReference.ID == resistiveMid.ID {
				continue
			}
			capacitiveShunts := []GraphInstance{}
			for _, candidate := range graph.Instances {
				if candidate.Kind == "capacitor" && between(candidate, resistiveMid.ID, bridgeReference.ID) {
					capacitiveShunts = append(capacitiveShunts, candidate)
				}
			}
			if len(capacitiveShunts) != 2 {
				continue
			}
			for _, capacitiveMid := range graph.Nodes {
				if capacitiveMid.Scope != "internal" || capacitiveMid.ID == resistiveMid.ID {
					continue
				}
				capacitiveSeries := []GraphInstance{}
				resistiveShunts := []GraphInstance{}
				for _, candidate := range graph.Instances {
					switch {
					case candidate.Kind == "capacitor" && otherNode(candidate, capacitiveMid.ID) != "" &&
						otherNode(candidate, capacitiveMid.ID) != bridgeReference.ID:
						capacitiveSeries = append(capacitiveSeries, candidate)
					case candidate.Kind == "resistor" && between(candidate, capacitiveMid.ID, bridgeReference.ID):
						resistiveShunts = append(resistiveShunts, candidate)
					}
				}
				if len(capacitiveSeries) != 2 || (len(resistiveShunts) != 1 && len(resistiveShunts) != 2) {
					continue
				}
				endpoints := []string{
					otherNode(capacitiveSeries[0], capacitiveMid.ID),
					otherNode(capacitiveSeries[1], capacitiveMid.ID),
				}
				slices.Sort(endpoints)
				firstPath := resistorPath(endpoints[0], resistiveMid.ID)
				secondPath := resistorPath(endpoints[1], resistiveMid.ID)
				if len(firstPath) == 0 || len(secondPath) == 0 {
					continue
				}
				participating := map[string]bool{}
				bridgeResistors := append(append(append([]GraphInstance{}, firstPath...), secondPath...), resistiveShunts...)
				for _, candidate := range append(append(bridgeResistors, capacitiveSeries...), capacitiveShunts...) {
					participating[candidate.ID] = true
				}
				if !participating[instance.ID] {
					feedbackDivider := ""
					feedbackOutput := ""
					outputFeedback := ""
					for _, active := range graph.Instances {
						if active.Kind != "opamp" {
							continue
						}
						terminals := topologyTerminalNodes(active)
						if terminals["OUT"] == bridgeReference.ID && terminals["IN_MINUS"] == bridgeReference.ID {
							feedbackDivider = terminals["IN_PLUS"]
						}
						if terminals["IN_PLUS"] == endpoints[0] || terminals["IN_PLUS"] == endpoints[1] {
							feedbackOutput = terminals["OUT"]
							outputFeedback = terminals["IN_MINUS"]
						}
					}
					if feedbackDivider == "" || feedbackOutput == "" || instance.Kind != "resistor" {
						continue
					}
					fraction := topologyFrequencySelectiveFeedbackFraction(requirement, rejectionFrequency)
					anchor := resistance
					value, id, derivation := 0.0, "", ""
					switch {
					case between(instance, feedbackOutput, feedbackDivider):
						value = anchor
						id = "topology:frequency_selective:feedback_upper"
						derivation = "bounded bridge-reference feedback fraction narrows the rejected band while preserving adjacent passbands"
					case between(instance, feedbackDivider, reference):
						value = anchor
						id = "topology:frequency_selective:feedback_lower"
						derivation = "bounded bridge-reference feedback fraction narrows the rejected band while preserving adjacent passbands"
					case outputFeedback != "" && between(instance, feedbackOutput, outputFeedback):
						gain := topologyFrequencySelectiveOutputGain(requirement, rejectionFrequency, fraction)
						value = anchor * (gain - 1)
						id = "topology:frequency_selective:output_gain_upper"
						derivation = "passband lower and upper gain bounds determine the minimum bounded non-inverting recovery gain"
					case outputFeedback != "" && between(instance, outputFeedback, reference):
						value = anchor
						id = "topology:frequency_selective:output_gain_lower"
						derivation = "passband lower and upper gain bounds determine the minimum bounded non-inverting recovery gain"
					default:
						continue
					}
					return []AnalyticScale{{
						ID: id, Kind: "resistance", ValueSI: value, Unit: "ohm",
						Derivation: derivation,
						SourceKind: "candidate_topology", SourceID: feedbackDivider, Priority: 1,
					}}
				}
				trimmed := len(firstPath) == 2 && len(secondPath) == 2 && len(resistiveShunts) == 1
				effectiveResistance := resistance
				if trimmed {
					effectiveResistance *= 1.05
				}
				capacitance := 1 / (2 * math.Pi * rejectionFrequency * effectiveResistance)
				if instance.Kind == "resistor" {
					value, id, derivation := resistance, "topology:frequency_selective:balanced_bridge_resistance", "equal reviewed resistors realize the balanced bridge resistance ratio"
					if trimmed {
						switch {
						case between(instance, capacitiveMid.ID, bridgeReference.ID):
							value, id = effectiveResistance/2, "topology:frequency_selective:balanced_bridge_shunt_resistance"
							derivation = "one reviewed shunt resistor realizes half the series-arm resistance"
						case between(instance, endpoints[0], otherNode(firstPath[0], endpoints[0])) || between(instance, endpoints[1], otherNode(secondPath[0], endpoints[1])):
							value, id = resistance*0.05, "topology:frequency_selective:balanced_bridge_trim_resistance"
							derivation = "catalog-bounded series trim aligns the rejected frequency without nonstandard component values"
						}
					}
					return []AnalyticScale{{
						ID: id, Kind: "resistance", ValueSI: value, Unit: "ohm", Derivation: derivation,
						SourceKind: "candidate_topology", SourceID: resistiveMid.ID, Priority: 1,
					}}
				}
				return []AnalyticScale{{
					ID: "topology:frequency_selective:balanced_bridge_capacitance", Kind: "capacitance",
					ValueSI: capacitance, Unit: "F",
					Derivation: "four equal reviewed capacitors realize two series C arms and two parallel C arms (2C), with f_reject=1/(2*pi*R*C)",
					SourceKind: "candidate_topology", SourceID: capacitiveMid.ID, Priority: 1,
				}}
			}
		}
	}
	return nil
}

func deriveBandpassTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	envelope, ok := topologyBandpassBehaviorEnvelope(requirement)
	if !ok || envelope.rejectionMaximum <= 0 || envelope.rejectionMaximum >= 1 {
		return nil
	}
	between := func(kind, left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != kind || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (first == left && second == right) || (first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	reference := referenceNodeForDomain(requirement, graph, envelope.input)
	const neutralFilterResistanceAnchor = 10_000.0
	resistanceAnchor := neutralFilterResistanceAnchor
	for _, port := range requirement.Requirements.Ports {
		inputImpedanceMinimum := port.Electrical.InputImpedanceMinOhm
		if port.ID == envelope.input.ID && inputImpedanceMinimum != nil {
			resistanceAnchor = math.Max(resistanceAnchor, 1.5**inputImpedanceMinimum)
		}
	}
	type stage struct {
		kind                string
		filterNode          string
		resistor, capacitor string
	}
	stages := []stage{}
	for _, node := range graph.Nodes {
		if node.Scope != "internal" {
			continue
		}
		filterNode := node.ID
		shuntResistor := between("resistor", filterNode, reference)
		shuntCapacitor := between("capacitor", filterNode, reference)
		seriesResistors, seriesCapacitors := []string{}, []string{}
		for _, candidate := range graph.Instances {
			if len(candidate.Terminals) != 2 {
				continue
			}
			left, right := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if left != filterNode && right != filterNode {
				continue
			}
			other := left
			if other == filterNode {
				other = right
			}
			if other == reference {
				continue
			}
			switch candidate.Kind {
			case "resistor":
				seriesResistors = append(seriesResistors, candidate.ID)
			case "capacitor":
				seriesCapacitors = append(seriesCapacitors, candidate.ID)
			}
		}
		switch {
		case shuntResistor != "" && len(seriesCapacitors) == 1:
			stages = append(stages, stage{kind: "lower", filterNode: filterNode, resistor: shuntResistor, capacitor: seriesCapacitors[0]})
		case shuntCapacitor != "" && len(seriesResistors) == 1:
			stages = append(stages, stage{kind: "upper", filterNode: filterNode, resistor: seriesResistors[0], capacitor: shuntCapacitor})
		}
	}
	lowerCount, upperCount := 0, 0
	for _, filterStage := range stages {
		if filterStage.kind == "lower" {
			lowerCount++
		} else {
			upperCount++
		}
	}
	for _, filterStage := range stages {
		count := upperCount
		if filterStage.kind == "lower" {
			count = lowerCount
		}
		if count == 0 || (instance.ID != filterStage.resistor && instance.ID != filterStage.capacitor) {
			continue
		}
		// Higher-order cascades have enough passband headroom to reserve more
		// rejection margin. A first-order side keeps twenty percent of the
		// declared rejection bound in reserve; each additional pole tightens the
		// target deterministically without using a named circuit family.
		minimumOrder := max(1, min(lowerCount, upperCount))
		marginFactor := 0.8 / float64(minimumOrder)
		totalAttenuation := math.Max(0.01, math.Min(0.99, envelope.rejectionMaximum*marginFactor))
		stageAttenuation := math.Pow(totalAttenuation, 1/float64(count))
		factor := math.Sqrt(1/(stageAttenuation*stageAttenuation) - 1)
		cutoff := envelope.upperHz / factor
		cornerID := "upper"
		if filterStage.kind == "lower" {
			cutoff = envelope.lowerHz * factor
			cornerID = "lower"
		}
		resistance, capacitance, found := catalogRCPair(requirement, inventory, cutoff, resistanceAnchor)
		if !found {
			return nil
		}
		if instance.ID == filterStage.resistor {
			return []AnalyticScale{{
				ID: "topology:bracketed_passband:" + cornerID + "_corner_resistance", Kind: "resistance", ValueSI: resistance, Unit: "ohm",
				Derivation: "catalog-ranked R-C pair for the per-stage bounded rejection ratio", SourceKind: "candidate_topology", SourceID: filterStage.filterNode, Priority: 1,
			}}
		}
		return []AnalyticScale{{
			ID: "topology:bracketed_passband:" + cornerID + "_corner_capacitance", Kind: "capacitance", ValueSI: capacitance, Unit: "F",
			Derivation: "catalog-ranked R-C pair for the per-stage bounded rejection ratio", SourceKind: "candidate_topology", SourceID: filterStage.filterNode, Priority: 1,
		}}
	}
	recoveryGain := 0.0
	recoveryGainReady := finite(envelope.passMinimum) && finite(envelope.passMaximum) &&
		envelope.passMinimum > 0 && envelope.passMaximum > envelope.passMinimum
	if recoveryGainReady {
		// A single recovery stage raises the lower passband bound toward the
		// center of its permitted span without targeting the maximum bound.
		passbandMidpoint := (envelope.passMinimum + envelope.passMaximum) / 2
		recoveryGain = passbandMidpoint / envelope.passMinimum
		recoveryGainReady = finite(recoveryGain) && recoveryGain > 1
	}
	if instance.Kind == "resistor" && recoveryGainReady {
		for _, active := range graph.Instances {
			if active.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if terminals["OUT"] == terminals["IN_MINUS"] {
				continue
			}
			upper := between("resistor", terminals["OUT"], terminals["IN_MINUS"])
			lower := between("resistor", terminals["IN_MINUS"], reference)
			if instance.ID != upper && instance.ID != lower {
				continue
			}
			lowerResistance, lowerFound := topologyCatalogResistanceClosest(
				requirement, inventory, neutralFilterResistanceAnchor,
			)
			upperResistance, upperFound := topologyCatalogResistanceClosest(
				requirement, inventory, lowerResistance*(recoveryGain-1),
			)
			if !lowerFound || !upperFound {
				return nil
			}
			value := lowerResistance
			id := "topology:bracketed_passband:recovery_gain_lower"
			if instance.ID == upper {
				value = upperResistance
				id = "topology:bracketed_passband:recovery_gain_upper"
			}
			return []AnalyticScale{{
				ID: id, Kind: "resistance", ValueSI: value, Unit: "ohm",
				Derivation: "bounded passband gain span recovers catalog-quantized passive insertion loss",
				SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
			}}
		}
	}
	return nil
}

func catalogRCPair(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	cutoffHz float64,
	resistanceAnchor float64,
) (float64, float64, bool) {
	if cutoffHz <= 0 || resistanceAnchor <= 0 {
		return 0, 0, false
	}
	type choice struct {
		value float64
		key   string
	}
	valuesNear := func(primitive PrimitiveCandidate, target float64) []float64 {
		if primitive.ValueDomain == nil || target <= 0 {
			return nil
		}
		domain := *primitive.ValueDomain
		minimum, maximum, ok := effectiveValueRange(domain)
		if !ok {
			return nil
		}
		if minimum == maximum {
			return []float64{minimum}
		}
		tolerance, proven := primitiveTolerancePercent(primitive, domain.Kind)
		if requirement.Acceptance.RequireAllCorners && !proven {
			return nil
		}
		values, issues := architecturesearch.PreferredValueCandidates(
			target,
			preferredSeriesForDomain(domain.Kind, tolerance, proven),
			minimum,
			maximum,
			minPositive(8, architecturesearch.DefaultMaxValueCandidates),
		)
		if len(issues) != 0 {
			return nil
		}
		return values
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	eligibleResistors, eligibleCapacitors := []PrimitiveCandidate{}, []PrimitiveCandidate{}
	for _, primitive := range inventory {
		if primitive.ValueDomain == nil || !primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		switch primitive.Kind {
		case "resistor":
			eligibleResistors = append(eligibleResistors, primitive)
		case "capacitor":
			eligibleCapacitors = append(eligibleCapacitors, primitive)
		}
	}
	resistors := []choice{}
	for _, primitive := range eligibleResistors {
		for _, value := range valuesNear(primitive, resistanceAnchor) {
			resistors = append(resistors, choice{value: value, key: primitive.Key})
		}
	}
	bestResistance, bestCapacitance := 0.0, 0.0
	bestScoreBucket := int64(math.MaxInt64)
	bestError, bestAnchorError, bestKey := math.Inf(1), math.Inf(1), ""
	for _, resistor := range resistors {
		idealCapacitance := 1 / (2 * math.Pi * cutoffHz * resistor.value)
		for _, primitive := range eligibleCapacitors {
			for _, value := range valuesNear(primitive, idealCapacitance) {
				actual := 1 / (2 * math.Pi * resistor.value * value)
				error := math.Abs(math.Log(actual / cutoffHz))
				anchorError := math.Abs(math.Log(resistor.value / resistanceAnchor))
				// Keep impedance within a practical neighborhood of the declared
				// input-loading anchor while making cutoff error the dominant term.
				const impedancePenalty = 0.1
				score := error + impedancePenalty*anchorError
				// A nanounit bucket absorbs platform-level transcendental noise;
				// exact error, anchor error, and catalog key resolve bucket ties.
				const scoreResolution = 1e-9
				scoreBucket := int64(math.Round(score / scoreResolution))
				key := resistor.key + "|" + primitive.Key
				if scoreBucket < bestScoreBucket || scoreBucket == bestScoreBucket &&
					(error < bestError || error == bestError &&
						(anchorError < bestAnchorError || anchorError == bestAnchorError && (bestKey == "" || key < bestKey))) {
					bestResistance, bestCapacitance = resistor.value, value
					bestScoreBucket, bestError, bestAnchorError, bestKey = scoreBucket, error, anchorError, key
				}
			}
		}
	}
	return bestResistance, bestCapacitance, bestKey != ""
}

func topologyFrequencySelectiveFeedbackFraction(requirement Requirement, rejectionFrequency float64) float64 {
	_ = requirement
	_ = rejectionFrequency
	// A small damping margin below one-half makes the notch robust to
	// catalog quantization while the bounded output stage recovers passband
	// insertion loss.
	return 0.5
}

func topologyFrequencySelectiveOutputGain(requirement Requirement, rejectionFrequency, feedbackFraction float64) float64 {
	required, permitted := 1.0, math.Inf(1)
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "voltage_gain_at_frequency" || assertion.FrequencyHz == nil ||
			*assertion.FrequencyHz <= 0 || *assertion.FrequencyHz == rejectionFrequency {
			continue
		}
		x := *assertion.FrequencyHz / rejectionFrequency
		numerator := math.Abs(1 - x*x)
		response := numerator / math.Hypot(numerator, 4*(1-feedbackFraction)*x)
		if response <= 0 {
			continue
		}
		if assertion.Min != nil {
			required = math.Max(required, *assertion.Min/response)
		}
		if assertion.Max != nil {
			permitted = math.Min(permitted, *assertion.Max/response)
		}
	}
	target := math.Max(1, required*1.005)
	if finite(permitted) {
		target = math.Min(target, permitted)
	}
	return math.Min(2, target)
}

type powerTransferSizingTargets struct {
	gain              float64
	bandwidthHz       float64
	minimumSignalHz   float64
	distortionMaximum float64
	loadResistance    float64
	minimumLoadOhm    float64
	maximumLoadOhm    float64
	inputResistance   float64
	quiescentCurrent  float64
	quiescentMinimum  float64
	quiescentMaximum  float64
	outputPeakVoltage float64
	drivePeakVoltage  float64
}

// derivePowerTransferTopologyScales recognizes terminal relationships rather
// than a named amplifier family. It sizes each passive from the external
// behavior using conventional first-order bias, gain, degeneration, and pole
// equations. These are advisory trial anchors; catalog quantization and trusted
// simulation remain authoritative.
func deriveRegulatedVoltageTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	outputTarget, outputMinimum, outputMaximum := 0.0, 0.0, 0.0
	outputID := ""
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "output_voltage" && assertion.Observation.Kind == "port" {
			outputTarget = assertionTarget(assertion)
			if assertion.Min != nil {
				outputMinimum = *assertion.Min
			}
			if assertion.Max != nil {
				outputMaximum = *assertion.Max
			}
			outputID = "port_" + assertion.Observation.ID
			break
		}
	}
	if outputTarget <= 0 || outputID == "" {
		return nil
	}
	supplies := topologyNodesByRole(graph, "supply")
	references := topologyNodesByRole(graph, "reference")
	if len(supplies) != 1 || len(references) != 1 {
		return nil
	}
	between := func(left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (first == left && second == right) || (first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	seriesBetween := func(left, right string) []string {
		paths := [][]string{}
		for _, node := range graph.Nodes {
			if node.Scope != "internal" || node.ID == left || node.ID == right {
				continue
			}
			first := between(left, node.ID)
			second := between(node.ID, right)
			if first == "" || second == "" || first == second {
				continue
			}
			path := []string{first, second}
			slices.Sort(path)
			paths = append(paths, path)
		}
		slices.SortFunc(paths, func(left, right []string) int { return slices.Compare(left, right) })
		if len(paths) == 0 {
			return nil
		}
		return paths[0]
	}
	// A switching regulator with a reviewed internal reference exposes that
	// reference through its FB terminal rather than through a separate shunt
	// device. Size the explicit output divider from the same terminal
	// relationship used by the trusted model. Keeping this recognition model-
	// and terminal-based makes it apply to any catalog part backed by the
	// synchronous-buck contract.
	for _, controller := range graph.Instances {
		referenceVoltage, referenceMinimum, referenceMaximum, referenceFound := primitiveModelParameterRange(
			inventory[controller.PrimitiveKey],
			simmodel.PrimitiveSynchronousBuckRegulatorV1,
			"reference_voltage_v",
		)
		if !referenceFound || outputTarget <= referenceVoltage ||
			outputMinimum <= 0 || outputMaximum < outputMinimum {
			continue
		}
		terminals := topologyTerminalNodes(controller)
		feedback := terminals["FB"]
		if feedback == "" || terminals["AGND"] != references[0] {
			continue
		}
		upperID, lowerID := between(outputID, feedback), between(feedback, references[0])
		upperSeries := []string(nil)
		if upperID == "" {
			upperSeries = seriesBetween(outputID, feedback)
		}
		if lowerID == "" || upperID == "" && len(upperSeries) != 2 ||
			instance.ID != upperID && instance.ID != lowerID && !slices.Contains(upperSeries, instance.ID) {
			continue
		}
		if len(upperSeries) == 2 {
			first, second, lower, found := catalogControllerSeriesFeedbackDivider(
				requirement, inventory,
				referenceVoltage, referenceMinimum, referenceMaximum,
				outputMinimum, outputMaximum, 10_000,
			)
			if !found {
				return nil
			}
			values := map[string]float64{
				upperSeries[0]: first,
				upperSeries[1]: second,
				lowerID:        lower,
			}
			return []AnalyticScale{{
				ID:   "topology:regulated_feedback_series:" + instance.ID,
				Kind: "resistance", ValueSI: values[instance.ID], Unit: "ohm",
				Derivation: "catalog-ranked compound divider derived from a reviewed controller reference and bounded output voltage",
				SourceKind: "candidate_topology", SourceID: feedback, Priority: 1,
			}}
		}
		upper, lower, found := catalogControllerFeedbackDivider(
			requirement, inventory,
			referenceVoltage, referenceMinimum, referenceMaximum,
			outputMinimum, outputMaximum, 10_000,
		)
		if !found {
			fallbackUpper, fallbackLower, fallbackFound := catalogResistanceDivider(
				requirement, inventory, outputTarget/referenceVoltage-1, 10_000, 1,
				outputTarget, outputTarget, 0, 0,
			)
			if !fallbackFound {
				return nil
			}
			upper, lower, found = fallbackUpper, fallbackLower[0], true
		}
		value, id := lower, "topology:regulated_feedback_lower"
		if instance.ID == upperID {
			value, id = upper, "topology:regulated_feedback_upper"
		}
		return []AnalyticScale{{
			ID: id + ":" + instance.ID, Kind: "resistance", ValueSI: value, Unit: "ohm",
			Derivation: "catalog-ranked divider ratio derived from a reviewed controller reference and bounded output voltage",
			SourceKind: "candidate_topology", SourceID: feedback, Priority: 1,
		}}
	}
	if !topologyGraphHasReferenceRegulatedOutput(graph, false) {
		return nil
	}
	absoluteReference, referenceVoltage := "", 0.0
	for _, candidate := range graph.Instances {
		if candidate.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["ANODE"] != references[0] {
			continue
		}
		for _, model := range inventory[candidate.PrimitiveKey].Models {
			if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
				continue
			}
			for _, parameter := range model.Parameters {
				if parameter.Name == "output_voltage_v" && parameter.Value > 0 {
					absoluteReference, referenceVoltage = terminals["CATHODE"], parameter.Value
				}
			}
		}
	}
	if absoluteReference == "" || referenceVoltage <= 0 || outputTarget <= referenceVoltage {
		return nil
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	makeScale := func(id string, value float64, derivation, source string) []AnalyticScale {
		if value <= 0 || !finite(value) {
			return nil
		}
		kind, unit := "resistance", "ohm"
		if instance.Kind == "capacitor" {
			kind, unit = "capacitance", "F"
		}
		return []AnalyticScale{{
			ID: id + ":" + instance.ID, Kind: kind, ValueSI: value, Unit: unit,
			Derivation: derivation, SourceKind: "candidate_topology", SourceID: source, Priority: 1,
		}}
	}
	if instance.Kind == "capacitor" {
		for _, controller := range graph.Instances {
			if controller.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(controller)
			if (first == terminals["OUT"] && second == terminals["IN_MINUS"]) ||
				(second == terminals["OUT"] && first == terminals["IN_MINUS"]) {
				return makeScale("topology:regulated_loop_compensation", 10e-9, "bounded dominant-pole compensation for a discrete feedback-regulated pass stage", outputID)
			}
		}
		switch {
		case (first == supplies[0] && second == references[0]) ||
			(second == supplies[0] && first == references[0]):
			return makeScale("topology:regulated_input_bypass", 100e-9, "bounded local input bypass for a feedback-regulated power stage", supplies[0])
		case (first == outputID && second == references[0]) ||
			(second == outputID && first == references[0]):
			return makeScale("topology:regulated_output_capacitance", 10e-6, "bounded output capacitance for startup and loop stability", outputID)
		}
		return nil
	}
	if instance.ID == between(supplies[0], absoluteReference) {
		return makeScale("topology:regulated_reference_bias", 10_000, "bounded bias resistance for a reviewed voltage reference", absoluteReference)
	}
	for _, node := range graph.Nodes {
		if node.Scope != "internal" {
			continue
		}
		upperID := between(outputID, node.ID)
		upperSeries := []string(nil)
		if upperID == "" {
			upperSeries = seriesBetween(outputID, node.ID)
		}
		lowerID := between(node.ID, references[0])
		if lowerID == "" || upperID == "" && len(upperSeries) != 2 ||
			instance.ID != upperID && instance.ID != lowerID && !slices.Contains(upperSeries, instance.ID) {
			continue
		}
		if len(upperSeries) == 2 {
			lower, lowerFound := topologyCatalogResistanceClosest(requirement, inventory, 10_000)
			leftID, rightID := upperSeries[0], upperSeries[1]
			leftInstance := graph.Instances[graphInstanceIndex(graph, leftID)]
			rightInstance := graph.Instances[graphInstanceIndex(graph, rightID)]
			left, right, upperFound := catalogSeriesResistancePairPreservingBranch(
				requirement,
				inventory,
				(outputTarget/referenceVoltage-1)*lower,
				leftInstance,
				rightInstance,
			)
			if !lowerFound || !upperFound {
				return nil
			}
			values := map[string]float64{leftID: left, rightID: right, lowerID: lower}
			return makeScale(
				"topology:regulated_feedback_series",
				values[instance.ID],
				"catalog-ranked series feedback composition derived from an absolute reference and bounded output voltage",
				node.ID,
			)
		}
		upper, lower, found := catalogResistanceDivider(
			requirement, inventory, outputTarget/referenceVoltage-1, 10_000, 1,
			outputTarget, outputTarget, 0, 0,
		)
		if !found {
			return nil
		}
		if instance.ID == upperID {
			return makeScale("topology:regulated_feedback_upper", upper, "catalog-ranked divider ratio derived from an absolute reference and bounded output voltage", node.ID)
		}
		return makeScale("topology:regulated_feedback_lower", lower[0], "catalog-ranked divider ratio derived from an absolute reference and bounded output voltage", node.ID)
	}
	for _, controller := range graph.Instances {
		if controller.Kind != "opamp" {
			continue
		}
		controllerTerminals := topologyTerminalNodes(controller)
		for _, passDevice := range graph.Instances {
			if passDevice.Kind != "npn_bjt" {
				continue
			}
			passTerminals := topologyTerminalNodes(passDevice)
			if instance.ID == between(controllerTerminals["OUT"], passTerminals["BASE"]) {
				return makeScale("topology:regulated_pass_drive", 100, "bounded series resistance isolates the controller from the nonlinear pass-device input", passDevice.ID)
			}
			for _, driver := range graph.Instances {
				if driver.Kind != "pnp_bjt" {
					continue
				}
				driverTerminals := topologyTerminalNodes(driver)
				if driverTerminals["EMITTER"] != supplies[0] ||
					driverTerminals["COLLECTOR"] != passTerminals["BASE"] {
					continue
				}
				switch instance.ID {
				case between(controllerTerminals["OUT"], driverTerminals["BASE"]):
					return makeScale("topology:regulated_buffer_drive", 100, "bounded series resistance isolates the controller from an inserted active drive stage", driver.ID)
				case between(supplies[0], driverTerminals["BASE"]):
					return makeScale("topology:regulated_buffer_bias", 10_000, "bounded emitter-referenced bias preserves a defined active drive-stage state", driver.ID)
				}
			}
			if instance.ID == between(passTerminals["BASE"], passTerminals["EMITTER"]) {
				return makeScale("topology:regulated_pass_bleeder", 10_000, "bounded emitter-referenced base discharge preserves a defined pass-device off state", passDevice.ID)
			}
		}
	}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && (node.Role == "control" || node.Role == "input") &&
			instance.ID == between(supplies[0], node.ID) {
			return makeScale("topology:regulated_default_enable", 10_000, "bounded pull-up preserves a declared high default control state", node.ID)
		}
	}
	return nil
}

func derivePowerTransferTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !topologyRequiresPowerTransfer(requirement) {
		return nil
	}
	targets := derivePowerTransferSizingTargets(requirement)
	if targets.gain <= 1 || targets.loadResistance <= 0 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	between := func(candidate GraphInstance, left, right string) bool {
		if len(candidate.Terminals) != 2 {
			return false
		}
		first := candidate.Terminals[0].Node
		second := candidate.Terminals[1].Node
		return (first == left && second == right) ||
			(first == right && second == left)
	}
	inputs := topologyNodesByRole(graph, "input", "control")
	outputs := topologyNodesByRole(graph, "output")
	references := topologyNodesByRole(graph, "reference")
	if len(inputs) == 0 || len(outputs) == 0 || len(references) == 0 {
		return nil
	}
	highRail, lowRail := topologyPowerRails(requirement, graph)
	if highRail == "" {
		return nil
	}
	if lowRail == "" {
		lowRail = references[0]
	}
	highVoltage, highOK := topologyNodeNominalVoltage(requirement, graph, highRail)
	lowVoltage, lowOK := topologyNodeNominalVoltage(requirement, graph, lowRail)
	if !highOK {
		return nil
	}
	if !lowOK {
		lowVoltage = 0
	}
	supplySpan := highVoltage - lowVoltage
	if supplySpan <= 0 {
		return nil
	}
	minimumSupplySpan, maximumSupplySpan := supplySpan, supplySpan
	if highMinimum, highMaximum, ok := topologyNodeVoltageRange(requirement, graph, highRail); ok {
		lowMinimum, lowMaximum := lowVoltage, lowVoltage
		if minimum, maximum, lowRangeOK := topologyNodeVoltageRange(requirement, graph, lowRail); lowRangeOK {
			lowMinimum, lowMaximum = minimum, maximum
		}
		minimumSupplySpan = highMinimum - lowMaximum
		maximumSupplySpan = highMaximum - lowMinimum
		if minimumSupplySpan <= 0 || maximumSupplySpan < minimumSupplySpan {
			minimumSupplySpan, maximumSupplySpan = supplySpan, supplySpan
		}
	}
	makeScale := func(id, kind string, value float64, unit, derivation, sourceID string) []AnalyticScale {
		if value <= 0 || !finite(value) {
			return nil
		}
		return []AnalyticScale{{
			ID:         id,
			Kind:       kind,
			ValueSI:    value,
			Unit:       unit,
			Derivation: derivation,
			SourceKind: "candidate_topology",
			SourceID:   sourceID,
			Priority:   1,
		}}
	}

	// Closed-loop feedback and compensation apply to both a direct active
	// path and a controller followed by discrete output devices.
	for _, active := range graph.Instances {
		if active.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		feedback := terminals["IN_MINUS"]
		hasExternalFeedbackGain := false
		seriesFeedbackValues := map[string]float64{}
		for _, output := range outputs {
			for _, candidate := range graph.Instances {
				hasExternalFeedbackGain = hasExternalFeedbackGain ||
					(candidate.Kind == "resistor" && between(candidate, feedback, output))
			}
			if leftValue, rightValue, found := catalogSeriesResistancePair(
				requirement,
				inventory,
				10_000*(targets.gain-1),
			); found {
				for _, middle := range graph.Nodes {
					if middle.Scope != "internal" || middle.ID == feedback || middle.ID == output {
						continue
					}
					incident := []GraphInstance{}
					for _, candidate := range graph.Instances {
						for _, terminal := range candidate.Terminals {
							if terminal.Node == middle.ID {
								incident = append(incident, candidate)
								break
							}
						}
					}
					if len(incident) != 2 || incident[0].Kind != "resistor" || incident[1].Kind != "resistor" {
						continue
					}
					leftID, rightID := "", ""
					for _, candidate := range incident {
						if between(candidate, feedback, middle.ID) {
							if leftID != "" {
								leftID = ""
								break
							}
							leftID = candidate.ID
						}
						if between(candidate, middle.ID, output) {
							if rightID != "" {
								rightID = ""
								break
							}
							rightID = candidate.ID
						}
					}
					if leftID != "" && rightID != "" {
						seriesFeedbackValues[leftID] = leftValue
						seriesFeedbackValues[rightID] = rightValue
						hasExternalFeedbackGain = true
						break
					}
				}
			}
		}
		if value, found := seriesFeedbackValues[instance.ID]; found {
			return makeScale(
				"topology:power_transfer:series_feedback_gain",
				"resistance", value, "ohm",
				"catalog-ranked series feedback composition realizes the bounded non-inverting closed-loop gain",
				feedback,
			)
		}
		for _, reference := range references {
			if hasExternalFeedbackGain && instance.Kind == "resistor" && between(instance, feedback, reference) {
				return makeScale(
					"topology:power_transfer:feedback_reference",
					"resistance", 10_000, "ohm",
					"R_reference=10k neutral impedance anchor for the bounded closed-loop gain equation",
					feedback,
				)
			}
		}
		for _, output := range outputs {
			if instance.Kind == "resistor" && between(instance, feedback, output) {
				return makeScale(
					"topology:power_transfer:feedback_gain",
					"resistance", 10_000*(targets.gain-1), "ohm",
					"R_feedback=R_reference*(A_v-1) for bounded non-inverting closed-loop gain",
					feedback,
				)
			}
		}
		if instance.Kind == "capacitor" && between(instance, terminals["OUT"], feedback) &&
			targets.bandwidthHz > 0 {
			feedbackResistance := 10_000 * (targets.gain - 1)
			return makeScale(
				"topology:power_transfer:feedback_compensation",
				"capacitance", 1/(2*math.Pi*targets.bandwidthHz*feedbackResistance), "F",
				"C_comp=1/(2*pi*bandwidth*R_feedback) first-order feedback compensation anchor",
				terminals["OUT"],
			)
		}
		for _, output := range outputs {
			if instance.Kind != "resistor" || !between(instance, terminals["OUT"], output) ||
				targets.outputPeakVoltage <= 0 || targets.loadResistance <= 0 {
				continue
			}
			hasNPN, hasPNP := false, false
			minimumBeta := 0.0
			for _, device := range graph.Instances {
				hasNPN = hasNPN || device.Kind == "npn_bjt"
				hasPNP = hasPNP || device.Kind == "pnp_bjt"
				if device.Kind != "npn_bjt" && device.Kind != "pnp_bjt" {
					continue
				}
				for _, model := range inventory[device.PrimitiveKey].Models {
					for _, parameter := range model.Parameters {
						if parameter.Name == "forward_beta" && parameter.Value > 0 &&
							(minimumBeta == 0 || parameter.Value < minimumBeta) {
							minimumBeta = parameter.Value
						}
					}
				}
			}
			if !hasNPN || !hasPNP || minimumBeta <= 0 {
				continue
			}
			peakBaseCurrent := targets.outputPeakVoltage / (targets.loadResistance * minimumBeta)
			if peakBaseCurrent <= 0 {
				continue
			}
			return makeScale(
				"topology:power_transfer:controller_ballast",
				"resistance", 0.65/peakBaseCurrent, "ohm",
				"R_ballast=V_BE/I_base_peak=0.65V/(V_peak/(R_load*beta_min)); the controller drives the zero crossing and the complementary power pair supplies current above this threshold",
				terminals["OUT"],
			)
		}
	}

	// A single voltage-controlled three-terminal device with AC coupling at
	// both endpoints is sized around half-supply collector/drain bias.
	for _, active := range graph.Instances {
		if active.Kind != "npn_bjt" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		base, emitter, collector := terminals["BASE"], terminals["EMITTER"], terminals["COLLECTOR"]
		for _, controller := range graph.Instances {
			if controller.Kind != "opamp" || topologyTerminalNodes(controller)["OUT"] != base ||
				targets.quiescentCurrent <= 0 {
				continue
			}
			controllerTerminals := topologyTerminalNodes(controller)
			hasResistor := func(left, right string) bool {
				for _, candidate := range graph.Instances {
					if candidate.Kind == "resistor" && between(candidate, left, right) {
						return true
					}
				}
				return false
			}
			// Non-inverting controller around a single-ended emitter follower.
			followerInput := controllerTerminals["IN_PLUS"]
			followerFeedback := controllerTerminals["IN_MINUS"]
			followerMidRail := ""
			for _, node := range graph.Nodes {
				if node.Scope == "internal" &&
					hasResistor(highRail, node.ID) &&
					hasResistor(node.ID, references[0]) &&
					hasResistor(node.ID, followerInput) &&
					hasResistor(node.ID, followerFeedback) {
					followerMidRail = node.ID
					break
				}
			}
			followerInputCoupled, followerOutputCoupled := false, false
			parallelStandingLoads := 0
			sinkSense, sinkReference := "", ""
			for _, sink := range graph.Instances {
				if sink.Kind != "npn_bjt" || sink.ID == active.ID {
					continue
				}
				sinkTerminals := topologyTerminalNodes(sink)
				if sinkTerminals["COLLECTOR"] != emitter {
					continue
				}
				for _, sinkController := range graph.Instances {
					if sinkController.Kind != "opamp" {
						continue
					}
					sinkControllerTerminals := topologyTerminalNodes(sinkController)
					if sinkControllerTerminals["OUT"] == sinkTerminals["BASE"] &&
						sinkControllerTerminals["IN_MINUS"] == sinkTerminals["EMITTER"] {
						sinkSense = sinkTerminals["EMITTER"]
						sinkReference = sinkControllerTerminals["IN_PLUS"]
					}
				}
			}
			stableReferenceNode := ""
			stableReferenceVoltage := 0.0
			stableReferenceMinimum := 0.0
			stableReferenceMaximum := 0.0
			stableReferenceMinimumBias := 0.0
			stableReferenceMaximumBias := 0.0
			for _, candidate := range graph.Instances {
				if candidate.Kind != "reference_diode" {
					continue
				}
				terminals := topologyTerminalNodes(candidate)
				if terminals["ANODE"] != references[0] ||
					!hasResistor(highRail, terminals["CATHODE"]) ||
					!hasResistor(terminals["CATHODE"], sinkReference) {
					continue
				}
				for _, model := range inventory[candidate.PrimitiveKey].Models {
					for _, parameter := range model.Parameters {
						switch parameter.Name {
						case "output_voltage_v":
							stableReferenceVoltage = parameter.Value
						case "min_bias_current_a":
							stableReferenceMinimumBias = parameter.Value
						case "max_bias_current_a":
							stableReferenceMaximumBias = parameter.Value
						}
					}
					for _, uncertainty := range model.Uncertainties {
						if uncertainty.Target == "model_parameters.output_voltage_v" {
							stableReferenceMinimum = uncertainty.Minimum
							stableReferenceMaximum = uncertainty.Maximum
						}
					}
				}
				if stableReferenceVoltage > 0 {
					stableReferenceNode = terminals["CATHODE"]
					if stableReferenceMinimum <= 0 {
						stableReferenceMinimum = stableReferenceVoltage
					}
					if stableReferenceMaximum <= 0 {
						stableReferenceMaximum = stableReferenceVoltage
					}
					break
				}
			}
			for _, candidate := range graph.Instances {
				for _, input := range inputs {
					followerInputCoupled = followerInputCoupled ||
						(candidate.Kind == "capacitor" && between(candidate, input, followerInput))
				}
				for _, output := range outputs {
					followerOutputCoupled = followerOutputCoupled ||
						(candidate.Kind == "capacitor" && between(candidate, emitter, output))
				}
				if candidate.Kind == "resistor" && between(candidate, emitter, references[0]) {
					parallelStandingLoads++
				}
			}
			hasControlledSink := sinkSense != "" && sinkReference != "" &&
				hasResistor(sinkSense, references[0]) &&
				(hasResistor(highRail, sinkReference) || stableReferenceNode != "") &&
				hasResistor(sinkReference, references[0])
			if collector == highRail && followerMidRail != "" &&
				hasResistor(emitter, followerFeedback) &&
				followerInputCoupled && followerOutputCoupled &&
				(parallelStandingLoads > 0 || hasControlledSink) {
				sinkSenseBranches := 0
				sinkReferenceLowerInstances := []string{}
				if hasControlledSink {
					for _, candidate := range graph.Instances {
						if candidate.Kind == "resistor" && between(candidate, sinkSense, references[0]) {
							sinkSenseBranches++
						}
						if candidate.Kind == "resistor" && between(candidate, sinkReference, references[0]) {
							sinkReferenceLowerInstances = append(sinkReferenceLowerInstances, candidate.ID)
						}
					}
					slices.Sort(sinkReferenceLowerInstances)
				}
				standingCurrent := targets.quiescentCurrent
				if targets.outputPeakVoltage > 0 && targets.loadResistance > 0 {
					cutoffMargin := 1.1
					if hasControlledSink {
						cutoffMargin = 1.05
					}
					standingCurrent = math.Max(
						standingCurrent,
						cutoffMargin*targets.outputPeakVoltage/targets.loadResistance,
					)
				}
				standingResistance := 0.0
				if parallelStandingLoads > 0 {
					standingResistance = (supplySpan * 0.5) / standingCurrent
				}
				midCurrent := math.Max(0.001, standingCurrent/500)
				sinkReferenceVoltage := math.Min(0.5, math.Max(0.25, 0.0175*supplySpan))
				parallelEquivalent := func(values []float64) float64 {
					conductance := 0.0
					for _, value := range values {
						if value > 0 {
							conductance += 1 / value
						}
					}
					if conductance == 0 {
						return 0
					}
					return 1 / conductance
				}
				sinkReferenceLower := make([]float64, max(1, len(sinkReferenceLowerInstances)))
				for index := range sinkReferenceLower {
					sinkReferenceLower[index] = 10_000 * float64(len(sinkReferenceLower))
				}
				sinkReferenceLowerEquivalent := parallelEquivalent(sinkReferenceLower)
				sinkReferenceUpper := sinkReferenceLowerEquivalent * (supplySpan/sinkReferenceVoltage - 1)
				controllerQuiescentCurrent := 0.0
				for _, candidate := range graph.Instances {
					if candidate.Kind != "opamp" {
						continue
					}
					deviceCurrent := 0.0
					for _, model := range inventory[candidate.PrimitiveKey].Models {
						for _, parameter := range model.Parameters {
							if parameter.Name == "quiescent_current_a" {
								deviceCurrent = math.Max(deviceCurrent, parameter.Value)
							}
						}
					}
					controllerQuiescentCurrent += deviceCurrent
				}
				minimumReferenceVoltage := 0.0
				if targets.quiescentMinimum > controllerQuiescentCurrent {
					minimumReferenceVoltage = sinkReferenceVoltage *
						(targets.quiescentMinimum - controllerQuiescentCurrent) / standingCurrent
				}
				maximumReferenceVoltage := 0.0
				if targets.quiescentMaximum > controllerQuiescentCurrent {
					maximumReferenceVoltage = sinkReferenceVoltage *
						(0.98 * (targets.quiescentMaximum - controllerQuiescentCurrent)) / standingCurrent
				}
				referenceSourceVoltage := supplySpan
				referenceSourceMinimum := minimumSupplySpan
				referenceSourceMaximum := maximumSupplySpan
				if stableReferenceNode != "" {
					referenceSourceVoltage = stableReferenceVoltage
					referenceSourceMinimum = stableReferenceMinimum
					referenceSourceMaximum = stableReferenceMaximum
				}
				if upper, lower, ok := catalogResistanceDivider(
					requirement,
					inventory,
					referenceSourceVoltage/sinkReferenceVoltage-1,
					10_000,
					len(sinkReferenceLowerInstances),
					referenceSourceMinimum,
					referenceSourceMaximum,
					minimumReferenceVoltage,
					maximumReferenceVoltage,
				); ok {
					sinkReferenceUpper = upper
					sinkReferenceLower = lower
					sinkReferenceLowerEquivalent = parallelEquivalent(lower)
					sinkReferenceVoltage = referenceSourceVoltage * sinkReferenceLowerEquivalent /
						(upper + sinkReferenceLowerEquivalent)
				}
				switch {
				case instance.Kind == "resistor" && between(instance, emitter, references[0]):
					return makeScale(
						"topology:power_transfer:follower_standing_current_load",
						"resistance", standingResistance*float64(parallelStandingLoads), "ohm",
						"R_each=N*(V_supply/2)/max(I_q,1.1*V_peak/R_load) for N equal parallel continuous-conduction branches with ten-percent cutoff margin",
						emitter,
					)
				case instance.Kind == "resistor" &&
					(between(instance, highRail, followerMidRail) || between(instance, followerMidRail, references[0])):
					return makeScale(
						"topology:power_transfer:follower_midrail_reference",
						"resistance", supplySpan/(2*midCurrent), "ohm",
						"R_mid=(V_supply/2)/max(1mA,I_standing/500) for an equal half-supply reference divider",
						followerMidRail,
					)
				case instance.Kind == "resistor" && between(instance, followerMidRail, followerInput):
					return makeScale(
						"topology:power_transfer:follower_input_bias_impedance",
						"resistance", targets.inputResistance, "ohm",
						"R_input_bias=R_input_min to preserve the requested external loading bound",
						followerInput,
					)
				case instance.Kind == "resistor" && between(instance, followerMidRail, followerFeedback):
					return makeScale(
						"topology:power_transfer:follower_feedback_reference",
						"resistance", 10_000, "ohm",
						"R_reference=10k neutral impedance anchor around the half-supply emitter follower",
						followerFeedback,
					)
				case instance.Kind == "resistor" && between(instance, emitter, followerFeedback):
					return makeScale(
						"topology:power_transfer:follower_feedback_gain",
						"resistance", 10_000*(targets.gain-1), "ohm",
						"R_feedback=R_reference*(A_v-1) around the AC-coupled emitter-follower output",
						followerFeedback,
					)
				case hasControlledSink && instance.Kind == "resistor" && between(instance, sinkSense, references[0]):
					return makeScale(
						"topology:power_transfer:controlled_sink_sense",
						"resistance", float64(sinkSenseBranches)*sinkReferenceVoltage/standingCurrent, "ohm",
						"R_each=N*V_reference/max(I_q,1.05*V_peak/R_load) for N equal parallel sense branches, with a catalog-quantized 0.25..0.5V low-loss reference",
						sinkSense,
					)
				case hasControlledSink && instance.Kind == "resistor" &&
					(between(instance, highRail, sinkReference) ||
						(stableReferenceNode != "" && between(instance, stableReferenceNode, sinkReference))):
					return makeScale(
						"topology:power_transfer:controlled_sink_reference_upper",
						"resistance", sinkReferenceUpper, "ohm",
						"R_upper=R_lower*(V_supply/V_reference-1) for the feedback-regulated standing-current reference",
						sinkReference,
					)
				case stableReferenceNode != "" && instance.Kind == "resistor" &&
					between(instance, highRail, stableReferenceNode):
					targetBiasCurrent := math.Max(0.001, 10*stableReferenceMinimumBias)
					if stableReferenceMaximumBias > 0 {
						targetBiasCurrent = math.Min(targetBiasCurrent, stableReferenceMaximumBias/2)
					}
					return makeScale(
						"topology:power_transfer:controlled_sink_reference_bias",
						"resistance", (supplySpan-stableReferenceVoltage)/targetBiasCurrent, "ohm",
						"R_bias=(V_supply-V_absolute_reference)/I_bias with bias centered inside the reviewed shunt-reference current envelope",
						stableReferenceNode,
					)
				case hasControlledSink && instance.Kind == "resistor" && between(instance, sinkReference, references[0]):
					lowerIndex, found := slices.BinarySearch(sinkReferenceLowerInstances, instance.ID)
					if !found || lowerIndex >= len(sinkReferenceLower) {
						return nil
					}
					return makeScale(
						fmt.Sprintf("topology:power_transfer:controlled_sink_reference_lower_%d", lowerIndex),
						"resistance", sinkReferenceLower[lowerIndex], "ohm",
						"parallel lower-leg resistor selected with its peers from reviewed values so the divider satisfies standing-current bounds at supply and tolerance corners",
						sinkReference,
					)
				}
				if instance.Kind == "capacitor" && between(instance, followerMidRail, references[0]) &&
					targets.minimumSignalHz > 0 {
					dividerResistance := supplySpan / (2 * midCurrent)
					dividerTheveninResistance := dividerResistance / 2
					return makeScale(
						"topology:power_transfer:follower_midrail_bypass",
						"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/100)*dividerTheveninResistance), "F",
						"C_mid=1/(2*pi*(f_min/100)*(R_upper||R_lower)) for two decades of margin so the half-supply reference remains an AC ground inside the feedback network",
						followerMidRail,
					)
				}
				if hasControlledSink && instance.Kind == "capacitor" &&
					between(instance, sinkReference, references[0]) && targets.minimumSignalHz > 0 {
					referenceTheveninResistance := 1 / (1/sinkReferenceLowerEquivalent + 1/sinkReferenceUpper)
					return makeScale(
						"topology:power_transfer:controlled_sink_reference_bypass",
						"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/100)*referenceTheveninResistance), "F",
						"C_reference=1/(2*pi*(f_min/100)*(R_upper||R_lower)) for a quiet standing-current reference two decades below the signal band",
						sinkReference,
					)
				}
				for _, input := range inputs {
					if instance.Kind == "capacitor" && between(instance, input, followerInput) &&
						targets.minimumSignalHz > 0 && targets.inputResistance > 0 {
						return makeScale(
							"topology:power_transfer:follower_input_coupling",
							"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.inputResistance), "F",
							"C_input=1/(2*pi*(f_min/10)*R_input_min) for a decade of low-frequency margin",
							followerInput,
						)
					}
				}
				for _, output := range outputs {
					if instance.Kind == "capacitor" && between(instance, emitter, output) && targets.minimumSignalHz > 0 {
						return makeScale(
							"topology:power_transfer:follower_output_coupling",
							"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.loadResistance), "F",
							"C_output=1/(2*pi*(f_min/10)*R_load) for a decade of low-frequency margin",
							emitter,
						)
					}
				}
			}

			biasedInput := controllerTerminals["IN_MINUS"]
			feedback := controllerTerminals["IN_PLUS"]
			midRail := ""
			for _, node := range graph.Nodes {
				if node.Scope == "internal" &&
					hasResistor(highRail, node.ID) &&
					hasResistor(node.ID, references[0]) &&
					hasResistor(node.ID, feedback) {
					midRail = node.ID
					break
				}
			}
			if midRail == "" || !hasResistor(collector, feedback) {
				continue
			}
			hasInputCoupling, hasOutputCoupling := false, false
			parallelCollectorLoads := 0
			for _, candidate := range graph.Instances {
				for _, input := range inputs {
					hasInputCoupling = hasInputCoupling ||
						(candidate.Kind == "capacitor" && between(candidate, input, biasedInput))
				}
				for _, output := range outputs {
					hasOutputCoupling = hasOutputCoupling ||
						(candidate.Kind == "capacitor" && between(candidate, collector, output))
				}
				if candidate.Kind == "resistor" && between(candidate, highRail, collector) {
					parallelCollectorLoads++
				}
			}
			if !hasInputCoupling || !hasOutputCoupling {
				continue
			}
			collectorResistance := (supplySpan * 0.5) / targets.quiescentCurrent
			emitterResistance := (supplySpan * 0.05) / targets.quiescentCurrent
			midCurrent := math.Max(0.001, targets.quiescentCurrent/500)
			switch {
			case instance.Kind == "resistor" && between(instance, highRail, collector):
				return makeScale(
					"topology:power_transfer:controlled_standing_current_load",
					"resistance", collectorResistance*float64(max(1, parallelCollectorLoads)), "ohm",
					"R_each=N*(V_supply/2)/I_q for N equal parallel controller-biased load branches",
					collector,
				)
			case instance.Kind == "resistor" && between(instance, emitter, references[0]):
				return makeScale(
					"topology:power_transfer:controlled_emitter_degeneration",
					"resistance", emitterResistance, "ohm",
					"R_emitter=(0.05*V_supply)/I_q for five-percent DC degeneration headroom",
					emitter,
				)
			case instance.Kind == "resistor" &&
				(between(instance, highRail, midRail) || between(instance, midRail, references[0])):
				return makeScale(
					"topology:power_transfer:midrail_reference",
					"resistance", supplySpan/(2*midCurrent), "ohm",
					"R_mid=(V_supply/2)/max(1mA,I_q/500) for an equal mid-rail divider",
					midRail,
				)
			case instance.Kind == "resistor" && between(instance, midRail, biasedInput):
				return makeScale(
					"topology:power_transfer:input_bias_impedance",
					"resistance", targets.inputResistance, "ohm",
					"R_input_bias=R_input_min to preserve the requested external loading bound",
					biasedInput,
				)
			case instance.Kind == "resistor" && between(instance, midRail, feedback):
				return makeScale(
					"topology:power_transfer:controlled_feedback_reference",
					"resistance", 10_000, "ohm",
					"R_reference=10k neutral impedance anchor around the biased output stage",
					feedback,
				)
			case instance.Kind == "resistor" && between(instance, collector, feedback):
				return makeScale(
					"topology:power_transfer:controlled_feedback_gain",
					"resistance", 10_000*(targets.gain-1), "ohm",
					"R_feedback=R_reference*(A_v-1) around the AC-coupled single-ended output stage",
					feedback,
				)
			case instance.Kind == "capacitor" && between(instance, emitter, references[0]) && targets.minimumSignalHz > 0:
				return makeScale(
					"topology:power_transfer:controlled_emitter_bypass",
					"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*emitterResistance), "F",
					"C_bypass=1/(2*pi*(f_min/10)*R_emitter) for a decade of low-frequency margin",
					emitter,
				)
			}
			for _, input := range inputs {
				if instance.Kind == "capacitor" && between(instance, input, biasedInput) &&
					targets.minimumSignalHz > 0 && targets.inputResistance > 0 {
					return makeScale(
						"topology:power_transfer:controlled_input_coupling",
						"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.inputResistance), "F",
						"C_input=1/(2*pi*(f_min/10)*R_input_min) for a decade of low-frequency margin",
						biasedInput,
					)
				}
			}
			for _, output := range outputs {
				if instance.Kind == "capacitor" && between(instance, collector, output) &&
					targets.minimumSignalHz > 0 {
					return makeScale(
						"topology:power_transfer:controlled_output_coupling",
						"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.loadResistance), "F",
						"C_output=1/(2*pi*(f_min/10)*R_load) for a decade of low-frequency margin",
						collector,
					)
				}
			}
		}
		hasInputCoupling, hasOutputCoupling := false, false
		parallelCollectorLoads := 0
		for _, candidate := range graph.Instances {
			for _, input := range inputs {
				hasInputCoupling = hasInputCoupling ||
					(candidate.Kind == "capacitor" && between(candidate, input, base))
			}
			for _, output := range outputs {
				hasOutputCoupling = hasOutputCoupling ||
					(candidate.Kind == "capacitor" && between(candidate, collector, output))
			}
			if candidate.Kind == "resistor" && between(candidate, highRail, collector) {
				parallelCollectorLoads++
			}
		}
		if !hasInputCoupling || !hasOutputCoupling || targets.quiescentCurrent <= 0 {
			continue
		}
		collectorResistance := (supplySpan * 0.5) / targets.quiescentCurrent
		emitterResistance := collectorResistance / targets.gain
		emitterVoltage := targets.quiescentCurrent * emitterResistance
		baseVoltage := emitterVoltage + 0.65
		if baseVoltage <= 0 || baseVoltage >= supplySpan*0.5 {
			continue
		}
		switch {
		case instance.Kind == "resistor" && between(instance, highRail, collector):
			return makeScale(
				"topology:power_transfer:standing_current_load",
				"resistance", collectorResistance*float64(max(1, parallelCollectorLoads)), "ohm",
				"R_each=N*(V_supply/2)/I_q for N equal parallel standing-current load branches",
				collector,
			)
		case instance.Kind == "resistor" && between(instance, emitter, references[0]):
			return makeScale(
				"topology:power_transfer:emitter_degeneration",
				"resistance", emitterResistance, "ohm",
				"R_emitter=R_collector/A_v first-order degenerated voltage-gain equation",
				emitter,
			)
		case instance.Kind == "resistor" && between(instance, collector, base):
			return makeScale(
				"topology:power_transfer:collector_feedback_bias",
				"resistance", (supplySpan*0.5-baseVoltage)/(targets.quiescentCurrent/100), "ohm",
				"R_bias=(V_collector-V_base)/(I_q/beta), beta=100, V_collector=V_supply/2, V_base=I_q*R_emitter+0.65V",
				base,
			)
		case instance.Kind == "capacitor" && between(instance, emitter, references[0]) && targets.minimumSignalHz > 0:
			return makeScale(
				"topology:power_transfer:emitter_bypass",
				"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*emitterResistance), "F",
				"C_bypass=1/(2*pi*(f_min/10)*R_emitter) for a decade of low-frequency margin",
				emitter,
			)
		}
		for _, input := range inputs {
			if instance.Kind == "capacitor" && between(instance, input, base) &&
				targets.minimumSignalHz > 0 && targets.inputResistance > 0 {
				return makeScale(
					"topology:power_transfer:input_coupling",
					"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.inputResistance), "F",
					"C_input=1/(2*pi*(f_min/10)*R_input_min) for a decade of low-frequency margin",
					base,
				)
			}
		}
		for _, output := range outputs {
			if instance.Kind == "capacitor" && between(instance, collector, output) &&
				targets.minimumSignalHz > 0 {
				return makeScale(
					"topology:power_transfer:output_coupling",
					"capacitance", 1/(2*math.Pi*(targets.minimumSignalHz/10)*targets.loadResistance), "F",
					"C_output=1/(2*pi*(f_min/10)*R_load) for a decade of low-frequency margin",
					collector,
				)
			}
		}
	}

	// Complementary follower degeneration limits the requested peak-voltage
	// loss to two percent at I_peak=V_peak/R_load. Algebraically this is
	// R_deg=0.02*R_load, independent of amplitude.
	powerBaseEmitterBleed := false
	if instance.Kind == "resistor" {
		for _, active := range graph.Instances {
			if active.Kind != "npn_bjt" && active.Kind != "pnp_bjt" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if !between(instance, terminals["BASE"], terminals["EMITTER"]) {
				continue
			}
			for _, output := range outputs {
				for _, candidate := range graph.Instances {
					powerBaseEmitterBleed = powerBaseEmitterBleed ||
						(candidate.Kind == "resistor" && between(candidate, terminals["EMITTER"], output))
				}
			}
		}
	}
	for _, active := range graph.Instances {
		if powerBaseEmitterBleed {
			continue
		}
		outputTerminal := ""
		switch active.Kind {
		case "npn_bjt", "pnp_bjt":
			outputTerminal = topologyTerminalNodes(active)["EMITTER"]
		case "n_channel_mosfet", "p_channel_mosfet":
			outputTerminal = topologyTerminalNodes(active)["SOURCE"]
		}
		if outputTerminal == "" {
			continue
		}
		for _, output := range outputs {
			onOutputPath := instance.Kind == "resistor" && between(instance, outputTerminal, output)
			if instance.Kind == "resistor" && !onOutputPath {
				for _, node := range graph.Nodes {
					if node.Scope != "internal" ||
						(!between(instance, outputTerminal, node.ID) &&
							!between(instance, node.ID, output)) {
						continue
					}
					hasFirst, hasSecond := false, false
					for _, candidate := range graph.Instances {
						if candidate.Kind != "resistor" {
							continue
						}
						hasFirst = hasFirst || between(candidate, outputTerminal, node.ID)
						hasSecond = hasSecond || between(candidate, node.ID, output)
					}
					onOutputPath = onOutputPath || (hasFirst && hasSecond)
				}
			}
			if onOutputPath {
				parallelOutputs := 0
				activeTerminals := topologyTerminalNodes(active)
				for _, sibling := range graph.Instances {
					if sibling.Kind != active.Kind {
						continue
					}
					siblingTerminals := topologyTerminalNodes(sibling)
					if siblingTerminals["BASE"] != activeTerminals["BASE"] ||
						siblingTerminals["COLLECTOR"] != activeTerminals["COLLECTOR"] {
						continue
					}
					for _, candidate := range graph.Instances {
						if candidate.Kind == "resistor" &&
							between(candidate, siblingTerminals["EMITTER"], output) {
							parallelOutputs++
							break
						}
					}
				}
				return makeScale(
					"topology:power_transfer:output_degeneration",
					"resistance", 0.02*targets.loadResistance*float64(max(1, parallelOutputs)), "ohm",
					"R_each=N*0.02*R_load for N symmetric output branches whose parallel equivalent limits peak loss to two percent",
					outputTerminal,
				)
			}
		}
	}

	// A compound emitter follower needs a base-emitter bleed on each power
	// device so the driver current not consumed as power-device base current has
	// a deterministic path. Without it, matched bias trackers overdrive the
	// cascaded pair because the driver emitter is otherwise connected only to
	// the next base junction.
	for _, active := range graph.Instances {
		if active.Kind != "npn_bjt" && active.Kind != "pnp_bjt" {
			continue
		}
		terminals := topologyTerminalNodes(active)
		base, emitter := terminals["BASE"], terminals["EMITTER"]
		if instance.Kind != "resistor" || !between(instance, base, emitter) {
			continue
		}
		onOutputPath := false
		for _, output := range outputs {
			for _, candidate := range graph.Instances {
				onOutputPath = onOutputPath ||
					(candidate.Kind == "resistor" && between(candidate, emitter, output))
			}
		}
		if !onOutputPath || targets.quiescentCurrent <= 0 {
			continue
		}
		forwardBeta := 0.0
		for _, model := range inventory[active.PrimitiveKey].Models {
			for _, parameter := range model.Parameters {
				if parameter.Name == "forward_beta" && parameter.Value > 0 &&
					(forwardBeta == 0 || parameter.Value < forwardBeta) {
					forwardBeta = parameter.Value
				}
			}
		}
		if forwardBeta <= 0 {
			continue
		}
		biasCurrent := math.Max(0.0005, targets.quiescentCurrent/4)
		branchCurrent := targets.quiescentCurrent / 4
		bleedCurrent := biasCurrent - branchCurrent/forwardBeta
		if bleedCurrent <= 0 {
			continue
		}
		parallelBleeds := 0
		for _, sibling := range graph.Instances {
			if sibling.Kind != active.Kind {
				continue
			}
			siblingTerminals := topologyTerminalNodes(sibling)
			if siblingTerminals["BASE"] != base || siblingTerminals["COLLECTOR"] != terminals["COLLECTOR"] {
				continue
			}
			siblingOnOutputPath := false
			for _, output := range outputs {
				for _, candidate := range graph.Instances {
					siblingOnOutputPath = siblingOnOutputPath ||
						(candidate.Kind == "resistor" && between(candidate, siblingTerminals["EMITTER"], output))
				}
			}
			if !siblingOnOutputPath {
				continue
			}
			for _, candidate := range graph.Instances {
				if candidate.Kind == "resistor" && between(candidate, base, siblingTerminals["EMITTER"]) {
					parallelBleeds++
					break
				}
			}
		}
		return makeScale(
			"topology:power_transfer:compound_base_emitter_bleed",
			"resistance", 0.65/bleedCurrent*float64(max(1, parallelBleeds)), "ohm",
			"R_each=N*V_BE/(I_bias-I_q_branch/beta_min) for N symmetric bleeds; their parallel equivalent bypasses the driver-current remainder while the requested total branch idle current remains",
			base,
		)
	}

	// A diode-spread complementary bipolar path uses symmetric rail feed.
	// Choose bias-chain current from the requested standing current, bounded to
	// a practical 2-20 mA range, and account for two 0.65 V junction drops.
	biasJunctionEdge := func(candidate GraphInstance) (string, string, bool) {
		terminals := topologyTerminalNodes(candidate)
		switch candidate.Kind {
		case "signal_diode", "reference_diode":
			return terminals["ANODE"], terminals["CATHODE"],
				terminals["ANODE"] != "" && terminals["CATHODE"] != ""
		case "npn_bjt":
			if terminals["BASE"] != "" && terminals["BASE"] == terminals["COLLECTOR"] {
				return terminals["BASE"], terminals["EMITTER"], true
			}
		case "pnp_bjt":
			if terminals["BASE"] != "" && terminals["BASE"] == terminals["COLLECTOR"] {
				return terminals["EMITTER"], terminals["BASE"], true
			}
		}
		return "", "", false
	}
	var npnBase, pnpBase string
	transitionFrequency := map[string]float64{}
	forwardBetaByBase := map[string]float64{}
	for _, active := range graph.Instances {
		_, _, isBiasTracker := biasJunctionEdge(active)
		if isBiasTracker && (active.Kind == "npn_bjt" || active.Kind == "pnp_bjt") {
			continue
		}
		switch active.Kind {
		case "npn_bjt":
			npnBase = topologyTerminalNodes(active)["BASE"]
		case "pnp_bjt":
			pnpBase = topologyTerminalNodes(active)["BASE"]
		}
		if active.Kind == "npn_bjt" || active.Kind == "pnp_bjt" {
			activeBeta := 0.0
			for _, model := range inventory[active.PrimitiveKey].Models {
				for _, parameter := range model.Parameters {
					switch parameter.Name {
					case "transition_frequency_hz":
						transitionFrequency[topologyTerminalNodes(active)["BASE"]] = parameter.Value
					case "forward_beta":
						if parameter.Value > 0 &&
							(activeBeta == 0 || parameter.Value < activeBeta) {
							activeBeta = parameter.Value
						}
					}
				}
			}
			if activeBeta > 0 {
				base := topologyTerminalNodes(active)["BASE"]
				if existing := forwardBetaByBase[base]; existing == 0 || activeBeta < existing {
					forwardBetaByBase[base] = activeBeta
				}
			}
		}
	}
	npnBetaProduct, npnBetaCount := powerTransferSeriesBeta(graph, inventory, "npn_bjt")
	pnpBetaProduct, pnpBetaCount := powerTransferSeriesBeta(graph, inventory, "pnp_bjt")
	drive := ""
	for _, active := range graph.Instances {
		if active.Kind == "opamp" {
			drive = topologyTerminalNodes(active)["OUT"]
			break
		}
	}
	upperBias, lowerBias := npnBase, pnpBase
	upperJunctions, lowerJunctions := 0, 0
	if drive != "" {
		upperByLower := map[string]string{}
		lowerByUpper := map[string]string{}
		for _, junction := range graph.Instances {
			upper, lower, ok := biasJunctionEdge(junction)
			if !ok {
				continue
			}
			if _, exists := upperByLower[lower]; !exists {
				upperByLower[lower] = upper
			}
			if _, exists := lowerByUpper[upper]; !exists {
				lowerByUpper[upper] = lower
			}
		}
		walkBiasChain := func(start string, nextByNode map[string]string) (string, int) {
			current := start
			visited := map[string]struct{}{start: {}}
			for len(visited) <= len(graph.Instances) {
				next := nextByNode[current]
				if next == "" {
					break
				}
				if _, cycle := visited[next]; cycle {
					break
				}
				visited[next] = struct{}{}
				current = next
			}
			return current, len(visited) - 1
		}
		upperBias, upperJunctions = walkBiasChain(drive, upperByLower)
		lowerBias, lowerJunctions = walkBiasChain(drive, lowerByUpper)
		if upperJunctions == 0 && lowerJunctions == 0 {
			upperBias = drive
			lowerBias = drive
		}
	}
	if instance.Kind == "resistor" && targets.bandwidthHz > 0 {
		for _, connection := range [][2]string{{npnBase, upperBias}, {pnpBase, lowerBias}} {
			base, bias := connection[0], connection[1]
			if base == "" || bias == "" || base == bias || !between(instance, base, bias) {
				continue
			}
			maximumResistance := math.Inf(1)
			limits := []string{}
			if ft := transitionFrequency[base]; ft > 0 && targets.quiescentCurrent > 0 {
				branchCurrent := targets.quiescentCurrent / 2
				maximumResistance = math.Min(
					maximumResistance,
					ft*0.02585/(20*targets.bandwidthHz*branchCurrent),
				)
				limits = append(limits, "the small-signal base pole stays at least twenty times the requested bandwidth")
			}
			if beta := forwardBetaByBase[base]; beta > 0 &&
				targets.outputPeakVoltage > 0 && targets.loadResistance > 0 {
				peakBaseCurrent := targets.outputPeakVoltage / (targets.loadResistance * beta)
				if peakBaseCurrent > 0 {
					maximumLossFraction := 0.02
					if targets.distortionMaximum > 0 {
						maximumLossFraction = math.Min(maximumLossFraction, targets.distortionMaximum)
					}
					maximumResistance = math.Min(
						maximumResistance,
						maximumLossFraction*targets.outputPeakVoltage/peakBaseCurrent,
					)
					limits = append(limits, "peak base-current loss stays within the tighter of two percent of requested output swing and the normalized distortion limit")
				}
			}
			if !finite(maximumResistance) || maximumResistance <= 0 {
				continue
			}
			return makeScale(
				"topology:power_transfer:base_stop",
				"resistance", maximumResistance, "ohm",
				"R_stop is the tighter catalog-bound limit so "+strings.Join(limits, " and "),
				base,
			)
		}
	}
	if npnBase != "" && pnpBase != "" && instance.Kind == "resistor" {
		biasCurrent := math.Max(0.0005, targets.quiescentCurrent/4)
		minimumDriveBeta := 0.0
		if npnBetaCount > 0 && pnpBetaCount > 0 {
			minimumDriveBeta = math.Min(npnBetaProduct, pnpBetaProduct)
		}
		peakVoltage := targets.drivePeakVoltage
		if peakVoltage <= 0 {
			peakVoltage = targets.outputPeakVoltage
		}
		peakLoadResistance := targets.loadResistance
		if npnBetaCount > 1 && pnpBetaCount > 1 && targets.minimumLoadOhm > 0 {
			peakLoadResistance = targets.minimumLoadOhm
		}
		peakBaseCurrent := 0.0
		if minimumDriveBeta > 0 && peakVoltage > 0 && peakLoadResistance > 0 {
			peakBaseCurrent = peakVoltage / (peakLoadResistance * minimumDriveBeta)
			biasCurrent = math.Max(
				biasCurrent,
				peakBaseCurrent,
			)
		}
		junctionDrop := 0.65 * float64(max(1, max(upperJunctions, lowerJunctions)))
		biasResistance := (supplySpan/2 - junctionDrop) / biasCurrent
		if peakBaseCurrent > 0 {
			peakHeadroom := minimumSupplySpan/2 - junctionDrop - peakVoltage
			if peakHeadroom > 0 {
				biasResistance = math.Min(biasResistance, peakHeadroom/peakBaseCurrent)
			}
		}
		parallelBranches := func(left, right string) int {
			count := 0
			for _, candidate := range graph.Instances {
				if candidate.Kind == "resistor" && between(candidate, left, right) {
					count++
				}
			}
			return max(1, count)
		}
		switch {
		case between(instance, highRail, upperBias):
			return makeScale(
				"topology:power_transfer:bias_chain_upper",
				"resistance", biasResistance*float64(parallelBranches(highRail, upperBias)), "ohm",
				"parallel R_bias branches realize the tighter of center-bias current and minimum-rail peak-headroom limits; matched diode-connected trackers set standing current while furnishing cascaded-device base drive",
				upperBias,
			)
		case between(instance, lowerBias, lowRail):
			return makeScale(
				"topology:power_transfer:bias_chain_lower",
				"resistance", biasResistance*float64(parallelBranches(lowerBias, lowRail)), "ohm",
				"parallel R_bias branches realize the tighter of center-bias current and minimum-rail peak-headroom limits; matched diode-connected trackers set standing current while furnishing cascaded-device base drive",
				lowerBias,
			)
		}
	}
	_ = nodeByID
	return nil
}

// powerTransferSeriesBeta returns the weakest series current-gain path for one
// polarity. Parallel output devices are alternative children of the same
// driver stage and therefore share current; they must not be multiplied as if
// they were additional cascaded gain stages.
func powerTransferSeriesBeta(
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
	kind string,
) (float64, int) {
	type stage struct {
		base    string
		emitter string
		beta    float64
	}
	stages := []stage{}
	for _, instance := range graph.Instances {
		if instance.Kind != kind {
			continue
		}
		terminals := topologyTerminalNodes(instance)
		if terminals["BASE"] == "" || terminals["BASE"] == terminals["COLLECTOR"] ||
			terminals["EMITTER"] == "" {
			continue
		}
		beta := 0.0
		for _, model := range inventory[instance.PrimitiveKey].Models {
			for _, parameter := range model.Parameters {
				if parameter.Name == "forward_beta" && parameter.Value > 0 &&
					(beta == 0 || parameter.Value < beta) {
					beta = parameter.Value
				}
			}
		}
		if beta > 0 {
			stages = append(stages, stage{base: terminals["BASE"], emitter: terminals["EMITTER"], beta: beta})
		}
	}
	if len(stages) == 0 {
		return 0, 0
	}
	drivenBase := map[string]bool{}
	for _, candidate := range stages {
		drivenBase[candidate.emitter] = true
	}
	bestProduct := math.Inf(1)
	bestCount := 0
	var walk func(int, map[int]bool) (float64, int)
	walk = func(index int, visited map[int]bool) (float64, int) {
		if visited[index] {
			return 0, 0
		}
		nextVisited := maps.Clone(visited)
		nextVisited[index] = true
		childProduct := math.Inf(1)
		childCount := 0
		for childIndex, child := range stages {
			if child.base != stages[index].emitter {
				continue
			}
			product, count := walk(childIndex, nextVisited)
			if product > 0 && product < childProduct {
				childProduct, childCount = product, count
			}
		}
		if !finite(childProduct) {
			return stages[index].beta, 1
		}
		return stages[index].beta * childProduct, childCount + 1
	}
	for index, candidate := range stages {
		if drivenBase[candidate.base] {
			continue
		}
		product, count := walk(index, map[int]bool{})
		if product > 0 && product < bestProduct {
			bestProduct, bestCount = product, count
		}
	}
	if !finite(bestProduct) {
		minimumBeta := math.Inf(1)
		for _, candidate := range stages {
			minimumBeta = math.Min(minimumBeta, candidate.beta)
		}
		if finite(minimumBeta) {
			return minimumBeta, 1
		}
		return 0, 0
	}
	return bestProduct, bestCount
}

func derivePowerTransferSizingTargets(requirement Requirement) powerTransferSizingTargets {
	targets := powerTransferSizingTargets{}
	minimumOutputPower, maximumOutputPower := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		target := assertionTarget(assertion)
		switch assertion.Metric {
		case "voltage_gain":
			targets.gain = math.Max(targets.gain, target)
		case "bandwidth", "cutoff_frequency":
			targets.bandwidthHz = math.Max(targets.bandwidthHz, target)
		case "thd", "total_harmonic_distortion":
			if assertion.Max != nil && *assertion.Max > 0 {
				maximum := *assertion.Max
				if assertion.Metric == "total_harmonic_distortion" {
					maximum /= 100
				}
				if targets.distortionMaximum == 0 || maximum < targets.distortionMaximum {
					targets.distortionMaximum = maximum
				}
			}
		case "quiescent_current":
			targets.quiescentCurrent = math.Max(targets.quiescentCurrent, target)
			if assertion.Min != nil {
				targets.quiescentMinimum = math.Max(targets.quiescentMinimum, *assertion.Min)
			}
			if assertion.Max != nil &&
				(targets.quiescentMaximum == 0 || *assertion.Max < targets.quiescentMaximum) {
				targets.quiescentMaximum = *assertion.Max
			}
		case "peak_voltage":
			targets.outputPeakVoltage = math.Max(targets.outputPeakVoltage, target)
		case "output_swing":
			targets.outputPeakVoltage = math.Max(targets.outputPeakVoltage, target/2)
		case "output_power":
			if assertion.Min != nil && *assertion.Min > minimumOutputPower {
				minimumOutputPower = *assertion.Min
			}
			if assertion.Max != nil && (maximumOutputPower == 0 || *assertion.Max < maximumOutputPower) {
				maximumOutputPower = *assertion.Max
			}
		}
		if assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 &&
			(targets.minimumSignalHz == 0 || *assertion.FrequencyHz < targets.minimumSignalHz) {
			targets.minimumSignalHz = *assertion.FrequencyHz
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "load_resistance" {
				value := positiveMidpoint(condition.Min, condition.Max)
				if value > 0 && (targets.loadResistance == 0 || value < targets.loadResistance) {
					targets.loadResistance = value
				}
				if condition.Min > 0 && (targets.minimumLoadOhm == 0 || condition.Min < targets.minimumLoadOhm) {
					targets.minimumLoadOhm = condition.Min
				}
				if condition.Max > targets.maximumLoadOhm {
					targets.maximumLoadOhm = condition.Max
				}
			}
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if port.Electrical.InputImpedanceMinOhm != nil &&
			*port.Electrical.InputImpedanceMinOhm > targets.inputResistance {
			targets.inputResistance = *port.Electrical.InputImpedanceMinOhm
		}
	}
	if targets.outputPeakVoltage == 0 && targets.loadResistance > 0 {
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if assertion.Metric == "output_power" {
				if power := assertionTarget(assertion); power > 0 {
					targets.outputPeakVoltage = math.Sqrt(2 * power * targets.loadResistance)
				}
			}
		}
	}
	if minimumOutputPower > 0 && maximumOutputPower > 0 &&
		targets.minimumLoadOhm > 0 && targets.maximumLoadOhm > 0 {
		minimumPeak := math.Sqrt(2 * minimumOutputPower * targets.maximumLoadOhm)
		maximumPeak := math.Sqrt(2 * maximumOutputPower * targets.minimumLoadOhm)
		if minimumPeak > 0 && minimumPeak <= maximumPeak {
			targets.drivePeakVoltage = (minimumPeak + maximumPeak) / 2
		}
	}
	return targets
}

// powerTransferBiasFeedBranchCount determines how many equal catalog resistor
// branches are required to preserve output-device base drive at the minimum
// rail while the requested power waveform approaches its peak. The topology
// generator uses the same behavioral targets and reviewed catalog values as
// the value planner, so sparse catalogs are composed deterministically rather
// than silently accepting a center-biased resistor that starves at the peak.
func powerTransferBiasFeedBranchCount(
	requirement Requirement,
	graph CandidateGraph,
	inventory map[string]PrimitiveCandidate,
	upper PrimitiveCandidate,
	lower PrimitiveCandidate,
) int {
	targets := derivePowerTransferSizingTargets(requirement)
	peakVoltage := targets.drivePeakVoltage
	if peakVoltage <= 0 {
		peakVoltage = targets.outputPeakVoltage
	}
	beta := math.Min(primitiveMinimumForwardBeta(upper), primitiveMinimumForwardBeta(lower))
	if peakVoltage <= 0 || targets.loadResistance <= 0 || beta <= 0 {
		return 2
	}
	highRail, lowRail := topologyPowerRails(requirement, graph)
	if highRail == "" || lowRail == "" {
		return 2
	}
	highNominal, highOK := topologyNodeNominalVoltage(requirement, graph, highRail)
	lowNominal, lowOK := topologyNodeNominalVoltage(requirement, graph, lowRail)
	highMinimum, _, highRangeOK := topologyNodeVoltageRange(requirement, graph, highRail)
	_, lowMaximum, lowRangeOK := topologyNodeVoltageRange(requirement, graph, lowRail)
	if !highOK || !lowOK || !highRangeOK || !lowRangeOK {
		return 2
	}
	nominalHalfSpan := (highNominal - lowNominal) / 2
	minimumHalfSpan := (highMinimum - lowMaximum) / 2
	peakBaseCurrent := peakVoltage / (targets.loadResistance * beta)
	centerHeadroom := nominalHalfSpan - 0.65
	peakHeadroom := minimumHalfSpan - 0.65 - peakVoltage
	if peakBaseCurrent <= 0 || centerHeadroom <= 0 || peakHeadroom <= 0 {
		return 2
	}
	centerResistance := centerHeadroom / peakBaseCurrent
	maximumEffectiveResistance := peakHeadroom / peakBaseCurrent
	catalogResistance, found := topologyCatalogResistanceClosest(requirement, inventory, centerResistance)
	if !found || catalogResistance <= 0 || maximumEffectiveResistance <= 0 {
		return 2
	}
	return max(2, int(math.Ceil(catalogResistance/maximumEffectiveResistance)))
}

// deriveWindowedBallastedSwitchTopologyScales recognizes two passive
// boundary decisions followed by a level-shifted source switch. A regulated
// rail and its catalog-rated ballast, when present, remain fixed graph values;
// this function couples only the threshold, hysteresis, and switching-control
// passives around them.
func deriveWindowedBallastedSwitchTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	envelope, required := topologyWindowThresholdEnvelope(requirement)
	if !required {
		return nil
	}
	inputNode, outputNode := "port_"+envelope.input, "port_"+envelope.output
	references := topologyNodesByRole(graph, "reference")
	if len(references) != 1 {
		return nil
	}
	referenceNode := references[0]
	between := func(left, right string) string {
		id := ""
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if first != left || second != right {
				if first != right || second != left {
					continue
				}
			}
			if id != "" {
				return ""
			}
			id = candidate.ID
		}
		return id
	}
	absoluteReference, referencePrimitiveKey, referenceVoltage := "", "", 0.0
	for _, candidate := range graph.Instances {
		if candidate.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["ANODE"] != referenceNode {
			continue
		}
		voltage, found := topologyPrimitiveReferenceVoltage(inventory[candidate.PrimitiveKey])
		if found {
			absoluteReference, referencePrimitiveKey, referenceVoltage = terminals["CATHODE"], candidate.PrimitiveKey, voltage
			break
		}
	}
	if absoluteReference == "" || referenceVoltage <= 0 {
		return nil
	}
	var upperDecision, lowerDecision GraphInstance
	for _, candidate := range graph.Instances {
		if candidate.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["IN_MINUS"] == inputNode {
			upperDecision = candidate
			break
		}
	}
	if upperDecision.ID == "" {
		return nil
	}
	upperTerminals := topologyTerminalNodes(upperDecision)
	insideNode := upperTerminals["OUT"]
	for _, candidate := range graph.Instances {
		if candidate.Kind != "comparator" || candidate.ID == upperDecision.ID {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["OUT"] == insideNode && between(inputNode, terminals["IN_PLUS"]) != "" {
			lowerDecision = candidate
			break
		}
	}
	if lowerDecision.ID == "" || insideNode == "" {
		return nil
	}
	lowerTerminals := topologyTerminalNodes(lowerDecision)
	lowerSignal, lowerReference := lowerTerminals["IN_PLUS"], lowerTerminals["IN_MINUS"]
	upperThreshold := upperTerminals["IN_PLUS"]
	driveSupply := upperTerminals["V_PLUS"]
	driveVoltage, driveFound := topologyNodeNominalVoltage(requirement, graph, driveSupply)
	if !driveFound || driveVoltage <= envelope.upperV {
		return nil
	}

	var switchDevice, levelShifter GraphInstance
	levelDriveNode := ""
	for _, candidate := range graph.Instances {
		if candidate.Kind != "p_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["DRAIN"] == outputNode {
			switchDevice = candidate
			break
		}
	}
	if switchDevice.ID == "" {
		return nil
	}
	switchTerminals := topologyTerminalNodes(switchDevice)
	for _, candidate := range graph.Instances {
		if candidate.Kind != "npn_bjt" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["COLLECTOR"] != switchTerminals["GATE"] ||
			terminals["EMITTER"] != referenceNode {
			continue
		}
		for _, node := range graph.Nodes {
			if between(node.ID, terminals["BASE"]) == "" {
				continue
			}
			if node.ID == insideNode {
				levelShifter, levelDriveNode = candidate, node.ID
				break
			}
			for _, buffer := range graph.Instances {
				if buffer.Kind != "opamp" {
					continue
				}
				bufferTerminals := topologyTerminalNodes(buffer)
				if bufferTerminals["IN_PLUS"] == insideNode &&
					bufferTerminals["IN_MINUS"] == node.ID &&
					bufferTerminals["OUT"] == node.ID {
					levelShifter, levelDriveNode = candidate, node.ID
					break
				}
			}
			if levelShifter.ID != "" {
				break
			}
		}
		if levelShifter.ID != "" {
			break
		}
	}
	if levelShifter.ID == "" || levelDriveNode == "" {
		return nil
	}
	levelTerminals := topologyTerminalNodes(levelShifter)

	lowerMinimum, lowerMaximum := envelope.lowerV, envelope.lowerV
	upperMinimum, upperMaximum := envelope.upperV, envelope.upperV
	hysteresisMinimum, hysteresisMaximum := 0.0, math.Inf(1)
	hysteresisTarget := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "lower_threshold":
			if assertion.Min != nil {
				lowerMinimum = *assertion.Min
			}
			if assertion.Max != nil {
				lowerMaximum = *assertion.Max
			}
		case "upper_threshold":
			if assertion.Min != nil {
				upperMinimum = *assertion.Min
			}
			if assertion.Max != nil {
				upperMaximum = *assertion.Max
			}
		case "hysteresis":
			if assertion.Min != nil {
				hysteresisMinimum = *assertion.Min
			}
			if assertion.Max != nil {
				hysteresisMaximum = *assertion.Max
			}
			hysteresisTarget = assertionTarget(assertion)
		}
	}
	if hysteresisTarget <= 0 || hysteresisMaximum < hysteresisMinimum {
		return nil
	}
	inputImpedanceMinimum := 0.0
	for _, port := range requirement.Requirements.Ports {
		if "port_"+port.ID == inputNode && port.Electrical.InputImpedanceMinOhm != nil {
			inputImpedanceMinimum = math.Max(inputImpedanceMinimum, *port.Electrical.InputImpedanceMinOhm)
		}
	}
	values := reviewedCatalogResistanceValues(requirement, inventory)
	bestInput, bestFeedback := 0.0, 0.0
	bestHysteresisError, bestImpedanceError, bestKey := math.Inf(1), math.Inf(1), ""
	for _, input := range values {
		for _, feedback := range values {
			if input.minimum+feedback.minimum < inputImpedanceMinimum {
				continue
			}
			minimumHysteresis := referenceVoltage * input.minimum / feedback.maximum
			maximumHysteresis := referenceVoltage * input.maximum / feedback.minimum
			if minimumHysteresis < hysteresisMinimum || maximumHysteresis > hysteresisMaximum {
				continue
			}
			nominalHysteresis := referenceVoltage * input.nominal / feedback.nominal
			hysteresisError := math.Abs(nominalHysteresis-hysteresisTarget) / hysteresisTarget
			impedanceError := multiplicativeRelativeError(
				input.nominal+feedback.nominal,
				math.Max(inputImpedanceMinimum, 10_000),
			)
			key := canonicalOptionalFloat(&input.nominal) + "|" + canonicalOptionalFloat(&feedback.nominal)
			if hysteresisError < bestHysteresisError ||
				(hysteresisError == bestHysteresisError &&
					(impedanceError < bestImpedanceError ||
						(impedanceError == bestImpedanceError && (bestKey == "" || key < bestKey)))) {
				bestInput, bestFeedback = input.nominal, feedback.nominal
				bestHysteresisError, bestImpedanceError, bestKey = hysteresisError, impedanceError, key
			}
		}
	}
	if bestInput <= 0 || bestFeedback <= 0 {
		return nil
	}
	findResistance := func(nominal float64) (catalogResistanceValue, bool) {
		for _, value := range values {
			if value.nominal == nominal {
				return value, true
			}
		}
		return catalogResistanceValue{}, false
	}
	anchor, anchorFound := topologyCatalogResistanceClosest(requirement, inventory, 10_000)
	decisionPull, decisionPullFound := topologyCatalogResistanceClosest(requirement, inventory, 1_000)
	decisionPullRange, decisionPullRangeFound := findResistance(decisionPull)
	lowerInputRange, lowerInputRangeFound := findResistance(bestInput)
	lowerFeedbackRange, lowerFeedbackRangeFound := findResistance(bestFeedback)
	if !anchorFound || !decisionPullFound || !decisionPullRangeFound ||
		!lowerInputRangeFound || !lowerFeedbackRangeFound {
		return nil
	}
	referenceMinimum, referenceMaximum := referenceVoltage, referenceVoltage
	for _, model := range inventory[referencePrimitiveKey].Models {
		if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
			continue
		}
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == "model_parameters.output_voltage_v" {
				referenceMinimum, referenceMaximum = uncertainty.Minimum, uncertainty.Maximum
			}
		}
	}
	if referenceMinimum <= 0 || referenceMaximum < referenceMinimum {
		return nil
	}
	upperOffsetMinimum, upperOffsetMaximum := 0.0, 0.0
	upperPrimitive := inventory[upperDecision.PrimitiveKey]
	for _, model := range upperPrimitive.Models {
		if model.ModelID != simmodel.PrimitiveComparatorOpenCollectorV1 {
			continue
		}
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == "model_parameters.input_offset_v" {
				upperOffsetMinimum, upperOffsetMaximum = uncertainty.Minimum, uncertainty.Maximum
			}
		}
	}
	outputOnResistance := primitiveModelParameter(
		upperPrimitive, simmodel.PrimitiveComparatorOpenCollectorV1, "output_on_resistance_ohm",
	)
	outputOffResistance := primitiveModelParameter(
		upperPrimitive, simmodel.PrimitiveComparatorOpenCollectorV1, "output_off_resistance_ohm",
	)
	if outputOnResistance <= 0 || outputOffResistance <= 0 {
		return nil
	}
	upperTransition := func(
		supplyResistance, groundResistance, feedbackResistance,
		pullResistance, lowerInputResistance, lowerFeedbackResistance,
		reference float64,
		onComparators int,
	) float64 {
		upperConductance := 1/supplyResistance + 1/groundResistance + 1/feedbackResistance
		fromSupply := (1 / supplyResistance) / upperConductance
		fromInside := (1 / feedbackResistance) / upperConductance
		lowerInputFraction := lowerFeedbackResistance / (lowerInputResistance + lowerFeedbackResistance)
		insideFeedbackConductance := lowerInputFraction/lowerFeedbackResistance + 1/feedbackResistance
		sinkConductance := float64(onComparators)/outputOnResistance +
			float64(2-onComparators)/outputOffResistance
		insideDenominator := 1/pullResistance + insideFeedbackConductance + sinkConductance
		insideConstant := (reference / pullResistance) / insideDenominator
		insideFromThreshold := insideFeedbackConductance / insideDenominator
		denominator := 1 - fromInside*insideFromThreshold
		if denominator <= 0 {
			return math.NaN()
		}
		return (fromSupply*driveVoltage + fromInside*insideConstant) / denominator
	}

	bestUpperSupply, bestUpperGround, bestUpperFeedback := 0.0, 0.0, 0.0
	bestUpperError, bestUpperAnchor, bestUpperKey := math.Inf(1), math.Inf(1), ""
	upperTarget := positiveMidpoint(upperMinimum, upperMaximum)
	upperTolerance := math.Max(.5*(upperMaximum-upperMinimum), .005*upperTarget)
	for _, supply := range values {
		for _, ground := range values {
			for _, feedback := range values {
				forward := upperTransition(
					supply.nominal, ground.nominal, feedback.nominal,
					decisionPullRange.nominal, lowerInputRange.nominal, lowerFeedbackRange.nominal,
					referenceVoltage, 0,
				)
				reverse := upperTransition(
					supply.nominal, ground.nominal, feedback.nominal,
					decisionPullRange.nominal, lowerInputRange.nominal, lowerFeedbackRange.nominal,
					referenceVoltage, 1,
				)
				hysteresis := forward - reverse
				if forward < upperMinimum || forward > upperMaximum ||
					hysteresis < hysteresisMinimum || hysteresis > hysteresisMaximum {
					continue
				}
				cornerForwardMinimum, cornerForwardMaximum := math.Inf(1), math.Inf(-1)
				cornerHysteresisMinimum, cornerHysteresisMaximum := math.Inf(1), math.Inf(-1)
				for corner := 0; corner < 128; corner++ {
					choose := func(bit int, minimum, maximum float64) float64 {
						if corner&(1<<bit) != 0 {
							return maximum
						}
						return minimum
					}
					cornerSupply := choose(0, supply.minimum, supply.maximum)
					cornerGround := choose(1, ground.minimum, ground.maximum)
					cornerFeedback := choose(2, feedback.minimum, feedback.maximum)
					cornerPull := choose(3, decisionPullRange.minimum, decisionPullRange.maximum)
					cornerLowerInput := choose(4, lowerInputRange.minimum, lowerInputRange.maximum)
					cornerLowerFeedback := choose(5, lowerFeedbackRange.minimum, lowerFeedbackRange.maximum)
					cornerReference := choose(6, referenceMinimum, referenceMaximum)
					cornerForward := upperTransition(
						cornerSupply, cornerGround, cornerFeedback,
						cornerPull, cornerLowerInput, cornerLowerFeedback,
						cornerReference, 0,
					)
					cornerReverse := upperTransition(
						cornerSupply, cornerGround, cornerFeedback,
						cornerPull, cornerLowerInput, cornerLowerFeedback,
						cornerReference, 1,
					)
					cornerForwardMinimum = math.Min(cornerForwardMinimum, cornerForward+upperOffsetMinimum)
					cornerForwardMaximum = math.Max(cornerForwardMaximum, cornerForward+upperOffsetMaximum)
					cornerHysteresis := cornerForward - cornerReverse
					cornerHysteresisMinimum = math.Min(cornerHysteresisMinimum, cornerHysteresis)
					cornerHysteresisMaximum = math.Max(cornerHysteresisMaximum, cornerHysteresis)
				}
				if cornerForwardMinimum < upperMinimum || cornerForwardMaximum > upperMaximum ||
					cornerHysteresisMinimum < hysteresisMinimum || cornerHysteresisMaximum > hysteresisMaximum {
					continue
				}
				error := math.Abs(forward-upperTarget)/upperTolerance +
					math.Abs(hysteresis-hysteresisTarget)/hysteresisTarget
				anchorError := multiplicativeRelativeError(ground.nominal, 10_000)
				key := canonicalOptionalFloat(&supply.nominal) + "|" +
					canonicalOptionalFloat(&ground.nominal) + "|" +
					canonicalOptionalFloat(&feedback.nominal)
				if error < bestUpperError ||
					(error == bestUpperError && (anchorError < bestUpperAnchor ||
						(anchorError == bestUpperAnchor && (bestUpperKey == "" || key < bestUpperKey)))) {
					bestUpperSupply, bestUpperGround, bestUpperFeedback = supply.nominal, ground.nominal, feedback.nominal
					bestUpperError, bestUpperAnchor, bestUpperKey = error, anchorError, key
				}
			}
		}
	}
	if bestUpperSupply <= 0 || bestUpperGround <= 0 || bestUpperFeedback <= 0 {
		return nil
	}
	upperSupplyRange, upperSupplyFound := findResistance(bestUpperSupply)
	upperGroundRange, upperGroundFound := findResistance(bestUpperGround)
	upperFeedbackRange, upperFeedbackFound := findResistance(bestUpperFeedback)
	if !upperSupplyFound || !upperGroundFound || !upperFeedbackFound {
		return nil
	}
	lowerOffsetMinimum, lowerOffsetMaximum := 0.0, 0.0
	lowerPrimitive := inventory[lowerDecision.PrimitiveKey]
	for _, model := range lowerPrimitive.Models {
		if model.ModelID != simmodel.PrimitiveComparatorOpenCollectorV1 {
			continue
		}
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == "model_parameters.input_offset_v" {
				lowerOffsetMinimum, lowerOffsetMaximum = uncertainty.Minimum, uncertainty.Maximum
			}
		}
	}
	lowerTransition := func(
		dividerUpper, dividerGround,
		lowerInputResistance, lowerFeedbackResistance, pullResistance,
		upperSupplyResistance, upperGroundResistance, upperFeedbackResistance,
		reference float64,
		onComparators int,
	) float64 {
		lowerReferenceVoltage := reference * dividerGround / (dividerUpper + dividerGround)
		lowerInputFraction := lowerFeedbackResistance / (lowerInputResistance + lowerFeedbackResistance)
		upperConductance := 1/upperSupplyResistance + 1/upperGroundResistance + 1/upperFeedbackResistance
		upperFromSupply := (1 / upperSupplyResistance) / upperConductance
		upperFromInside := (1 / upperFeedbackResistance) / upperConductance
		sinkConductance := float64(onComparators)/outputOnResistance +
			float64(2-onComparators)/outputOffResistance
		insideDenominator := 1/pullResistance + 1/lowerFeedbackResistance +
			(1-upperFromInside)/upperFeedbackResistance + sinkConductance
		insideVoltage := (reference/pullResistance + lowerReferenceVoltage/lowerFeedbackResistance +
			upperFromSupply*driveVoltage/upperFeedbackResistance) / insideDenominator
		return (lowerReferenceVoltage - (1-lowerInputFraction)*insideVoltage) / lowerInputFraction
	}
	lowerTarget := positiveMidpoint(lowerMinimum, lowerMaximum)
	lowerTolerance := math.Max(.5*(lowerMaximum-lowerMinimum), .005*lowerTarget)
	bestLowerUpper, bestLowerGround := 0.0, 0.0
	bestLowerError, bestLowerAnchor, bestLowerKey := math.Inf(1), math.Inf(1), ""
	for _, dividerUpper := range values {
		for _, dividerGround := range values {
			forward := lowerTransition(
				dividerUpper.nominal, dividerGround.nominal,
				lowerInputRange.nominal, lowerFeedbackRange.nominal, decisionPullRange.nominal,
				upperSupplyRange.nominal, upperGroundRange.nominal, upperFeedbackRange.nominal,
				referenceVoltage, 1,
			)
			reverse := lowerTransition(
				dividerUpper.nominal, dividerGround.nominal,
				lowerInputRange.nominal, lowerFeedbackRange.nominal, decisionPullRange.nominal,
				upperSupplyRange.nominal, upperGroundRange.nominal, upperFeedbackRange.nominal,
				referenceVoltage, 0,
			)
			hysteresis := forward - reverse
			if forward < lowerMinimum || forward > lowerMaximum ||
				hysteresis < hysteresisMinimum || hysteresis > hysteresisMaximum {
				continue
			}
			cornerForwardMinimum, cornerForwardMaximum := math.Inf(1), math.Inf(-1)
			cornerHysteresisMinimum, cornerHysteresisMaximum := math.Inf(1), math.Inf(-1)
			for corner := 0; corner < 512; corner++ {
				choose := func(bit int, minimum, maximum float64) float64 {
					if corner&(1<<bit) != 0 {
						return maximum
					}
					return minimum
				}
				cornerForward := lowerTransition(
					choose(0, dividerUpper.minimum, dividerUpper.maximum),
					choose(1, dividerGround.minimum, dividerGround.maximum),
					choose(2, lowerInputRange.minimum, lowerInputRange.maximum),
					choose(3, lowerFeedbackRange.minimum, lowerFeedbackRange.maximum),
					choose(4, decisionPullRange.minimum, decisionPullRange.maximum),
					choose(5, upperSupplyRange.minimum, upperSupplyRange.maximum),
					choose(6, upperGroundRange.minimum, upperGroundRange.maximum),
					choose(7, upperFeedbackRange.minimum, upperFeedbackRange.maximum),
					choose(8, referenceMinimum, referenceMaximum),
					1,
				)
				cornerReverse := lowerTransition(
					choose(0, dividerUpper.minimum, dividerUpper.maximum),
					choose(1, dividerGround.minimum, dividerGround.maximum),
					choose(2, lowerInputRange.minimum, lowerInputRange.maximum),
					choose(3, lowerFeedbackRange.minimum, lowerFeedbackRange.maximum),
					choose(4, decisionPullRange.minimum, decisionPullRange.maximum),
					choose(5, upperSupplyRange.minimum, upperSupplyRange.maximum),
					choose(6, upperGroundRange.minimum, upperGroundRange.maximum),
					choose(7, upperFeedbackRange.minimum, upperFeedbackRange.maximum),
					choose(8, referenceMinimum, referenceMaximum),
					0,
				)
				cornerForwardMinimum = math.Min(cornerForwardMinimum, cornerForward+lowerOffsetMinimum)
				cornerForwardMaximum = math.Max(cornerForwardMaximum, cornerForward+lowerOffsetMaximum)
				cornerHysteresis := cornerForward - cornerReverse
				cornerHysteresisMinimum = math.Min(cornerHysteresisMinimum, cornerHysteresis)
				cornerHysteresisMaximum = math.Max(cornerHysteresisMaximum, cornerHysteresis)
			}
			if cornerForwardMinimum < lowerMinimum || cornerForwardMaximum > lowerMaximum ||
				cornerHysteresisMinimum < hysteresisMinimum || cornerHysteresisMaximum > hysteresisMaximum {
				continue
			}
			error := math.Abs(forward-lowerTarget)/lowerTolerance +
				math.Abs(hysteresis-hysteresisTarget)/hysteresisTarget
			anchorError := multiplicativeRelativeError(dividerGround.nominal, 10_000)
			key := canonicalOptionalFloat(&dividerUpper.nominal) + "|" + canonicalOptionalFloat(&dividerGround.nominal)
			if error < bestLowerError ||
				(error == bestLowerError && (anchorError < bestLowerAnchor ||
					(anchorError == bestLowerAnchor && (bestLowerKey == "" || key < bestLowerKey)))) {
				bestLowerUpper, bestLowerGround = dividerUpper.nominal, dividerGround.nominal
				bestLowerError, bestLowerAnchor, bestLowerKey = error, anchorError, key
			}
		}
	}
	if bestLowerUpper <= 0 || bestLowerGround <= 0 {
		return nil
	}
	assignments := map[string]float64{
		between(driveSupply, absoluteReference):                     anchor,
		between(absoluteReference, insideNode):                      decisionPull,
		between(absoluteReference, lowerReference):                  bestLowerUpper,
		between(lowerReference, referenceNode):                      bestLowerGround,
		between(inputNode, lowerSignal):                             bestInput,
		between(insideNode, lowerSignal):                            bestFeedback,
		between(driveSupply, upperThreshold):                        bestUpperSupply,
		between(upperThreshold, referenceNode):                      bestUpperGround,
		between(insideNode, upperThreshold):                         bestUpperFeedback,
		between(levelDriveNode, levelTerminals["BASE"]):             anchor,
		between(switchTerminals["SOURCE"], switchTerminals["GATE"]): anchor,
	}
	if instance.Kind == "resistor" {
		value := assignments[instance.ID]
		if value <= 0 || !finite(value) {
			return nil
		}
		return []AnalyticScale{{
			ID:         "topology:windowed_ballasted_switch:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    value,
			Unit:       "ohm",
			Derivation: "catalog value jointly derived from ordered thresholds, hysteresis, input impedance, and source-switch relationships",
			SourceKind: "candidate_topology",
			SourceID:   insideNode,
			Priority:   1,
		}}
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	if (first == driveSupply && second == referenceNode) ||
		(first == referenceNode && second == driveSupply) ||
		(first == switchTerminals["SOURCE"] && second == switchTerminals["GATE"]) ||
		(first == switchTerminals["GATE"] && second == switchTerminals["SOURCE"]) {
		return []AnalyticScale{{
			ID:         "topology:windowed_ballasted_switch_capacitance:" + instance.ID,
			Kind:       "capacitance",
			ValueSI:    100e-9,
			Unit:       "F",
			Derivation: "bounded local bypass or gate-transition capacitance for a protected source switch",
			SourceKind: "candidate_topology",
			SourceID:   insideNode,
			Priority:   1,
		}}
	}
	return nil
}

// deriveWindowedControlledCurrentTopologyScales recognizes a pair of
// open-collector boundary decisions driving a low-side current controller. It
// derives the complete passive network as one coupled assignment: changing a
// feedback resistance changes both hysteresis and the reference voltages
// needed to preserve the requested thresholds. Recognition follows terminal
// relationships only and is independent of project, port, primitive, or node
// identities.
func deriveWindowedControlledCurrentTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	envelope, required := topologyWindowThresholdEnvelope(requirement)
	if !required {
		return nil
	}
	referenceNodes := topologyNodesByRole(graph, "reference")
	if len(referenceNodes) != 1 {
		return nil
	}
	referenceNode := referenceNodes[0]
	inputNode, outputNode := "port_"+envelope.input, "port_"+envelope.output

	between := func(left, right string) []string {
		ids := []string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if first == left && second == right || first == right && second == left {
				ids = append(ids, candidate.ID)
			}
		}
		slices.Sort(ids)
		return ids
	}
	seriesPair := func(left, right string) []string {
		pairs := [][]string{}
		for _, node := range graph.Nodes {
			if node.Scope != "internal" || node.ID == left || node.ID == right {
				continue
			}
			first, second := between(left, node.ID), between(node.ID, right)
			if len(first) != 1 || len(second) != 1 {
				continue
			}
			degree := 0
			for _, candidate := range graph.Instances {
				if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
					continue
				}
				if candidate.Terminals[0].Node == node.ID || candidate.Terminals[1].Node == node.ID {
					degree++
				}
			}
			if degree != 2 {
				continue
			}
			pair := []string{first[0], second[0]}
			slices.Sort(pair)
			pairs = append(pairs, pair)
		}
		slices.SortFunc(pairs, func(left, right []string) int { return slices.Compare(left, right) })
		if len(pairs) == 0 {
			return nil
		}
		return pairs[0]
	}
	uniqueBetween := func(left, right string) string {
		ids := between(left, right)
		if len(ids) != 1 {
			return ""
		}
		return ids[0]
	}

	absoluteReference, referenceVoltage := "", 0.0
	for _, candidate := range graph.Instances {
		if candidate.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["ANODE"] != referenceNode {
			continue
		}
		voltage, found := topologyPrimitiveReferenceVoltage(inventory[candidate.PrimitiveKey])
		if found {
			absoluteReference, referenceVoltage = terminals["CATHODE"], voltage
			break
		}
	}
	if absoluteReference == "" || referenceVoltage <= 0 {
		return nil
	}

	var upperDecision, lowerDecision GraphInstance
	for _, candidate := range graph.Instances {
		if candidate.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["IN_MINUS"] == inputNode {
			upperDecision = candidate
			break
		}
	}
	if upperDecision.ID == "" {
		return nil
	}
	upperDecisionTerminals := topologyTerminalNodes(upperDecision)
	insideNode := upperDecisionTerminals["OUT"]
	for _, candidate := range graph.Instances {
		if candidate.Kind != "comparator" || candidate.ID == upperDecision.ID {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["OUT"] == insideNode && uniqueBetween(inputNode, terminals["IN_PLUS"]) != "" {
			lowerDecision = candidate
			break
		}
	}
	if lowerDecision.ID == "" || insideNode == "" {
		return nil
	}
	lowerDecisionTerminals := topologyTerminalNodes(lowerDecision)
	lowerSignal := lowerDecisionTerminals["IN_PLUS"]
	lowerReference := lowerDecisionTerminals["IN_MINUS"]
	upperThreshold := upperDecisionTerminals["IN_PLUS"]

	var lowerAmplifier, upperAmplifier GraphInstance
	for _, candidate := range graph.Instances {
		if candidate.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		switch {
		case terminals["OUT"] == lowerReference:
			lowerAmplifier = candidate
		case uniqueBetween(terminals["OUT"], upperThreshold) != "":
			upperAmplifier = candidate
		}
	}
	if lowerAmplifier.ID == "" || upperAmplifier.ID == "" {
		return nil
	}
	lowerAmplifierTerminals := topologyTerminalNodes(lowerAmplifier)
	upperAmplifierTerminals := topologyTerminalNodes(upperAmplifier)
	upperReference := upperAmplifierTerminals["OUT"]
	if lowerAmplifierTerminals["IN_PLUS"] == absoluteReference ||
		upperAmplifierTerminals["IN_PLUS"] != absoluteReference {
		return nil
	}

	var switchDevice, currentController GraphInstance
	for _, candidate := range graph.Instances {
		if candidate.Kind != "n_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["DRAIN"] == outputNode {
			switchDevice = candidate
			break
		}
	}
	if switchDevice.ID == "" {
		return nil
	}
	switchTerminals := topologyTerminalNodes(switchDevice)
	for _, candidate := range graph.Instances {
		if candidate.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		feedback := terminals["IN_MINUS"]
		if terminals["IN_PLUS"] == insideNode &&
			(feedback == switchTerminals["SOURCE"] ||
				uniqueBetween(feedback, switchTerminals["SOURCE"]) != "") &&
			terminals["OUT"] == switchTerminals["GATE"] {
			currentController = candidate
			break
		}
	}
	if currentController.ID == "" {
		return nil
	}
	controllerTerminals := topologyTerminalNodes(currentController)
	driveSupply := controllerTerminals["V_PLUS"]
	senseNode, gateNode := switchTerminals["SOURCE"], switchTerminals["GATE"]
	currentFeedbackNode := controllerTerminals["IN_MINUS"]

	inputResistance := 10_000.0
	for _, port := range requirement.Requirements.Ports {
		if "port_"+port.ID == inputNode && port.Electrical.InputImpedanceMinOhm != nil {
			inputResistance = math.Max(inputResistance, 1.5**port.Electrical.InputImpedanceMinOhm)
		}
	}
	inputResistance, inputFound := topologyCatalogResistanceClosest(
		requirement, inventory, inputResistance,
	)
	hysteresis := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "hysteresis" {
			hysteresis = math.Max(hysteresis, assertionTarget(assertion))
		}
	}
	if !inputFound || hysteresis <= 0 || hysteresis >= referenceVoltage {
		return nil
	}
	feedbackRatio := hysteresis / (referenceVoltage - hysteresis)
	feedbackValues := []float64(nil)
	feedbackFound := false
	if feedbackRatio > 0 {
		var left, right float64
		left, right, feedbackFound = catalogSeriesResistancePair(
			requirement, inventory, inputResistance/feedbackRatio,
		)
		feedbackValues = []float64{left, right}
	}
	if !feedbackFound {
		return nil
	}
	feedbackResistance := feedbackValues[0] + feedbackValues[1]
	actualFeedbackRatio := inputResistance / feedbackResistance
	lowerTarget := positiveMidpoint(envelope.lowerV, envelope.lowerV)
	upperTarget := positiveMidpoint(envelope.upperV, envelope.upperV)
	lowerReferenceVoltage := lowerTarget / (1 + actualFeedbackRatio)
	upperReferenceVoltage := upperTarget + actualFeedbackRatio*(upperTarget-referenceVoltage)
	if lowerReferenceVoltage <= 0 || lowerReferenceVoltage >= referenceVoltage ||
		upperReferenceVoltage <= referenceVoltage {
		return nil
	}

	const anchorResistance = 10_000.0
	lowerUpper, lowerBranches, lowerDividerFound := catalogResistanceDivider(
		requirement,
		inventory,
		referenceVoltage/lowerReferenceVoltage-1,
		anchorResistance,
		1,
		referenceVoltage,
		referenceVoltage,
		0,
		0,
	)
	upperGround, upperFeedback, upperDividerFound := catalogNonInvertingGainTriplet(
		requirement, inventory, upperReferenceVoltage/referenceVoltage,
	)
	anchorValue, anchorFound := topologyCatalogResistanceClosest(
		requirement, inventory, anchorResistance,
	)
	commandSense, commandBias := "", ""
	commandBiasValue, commandBiasFound := anchorValue, true
	if currentFeedbackNode != senseNode {
		commandSense = uniqueBetween(senseNode, currentFeedbackNode)
		commandBias = uniqueBetween(absoluteReference, currentFeedbackNode)
		lowMargin := primitiveModelParameter(
			inventory[lowerDecision.PrimitiveKey],
			simmodel.PrimitiveComparatorOpenCollectorV1,
			"output_low_margin_v",
		)
		offsetVoltage := math.Max(4*lowMargin, .2*referenceVoltage)
		if offsetVoltage <= 0 || offsetVoltage >= referenceVoltage {
			return nil
		}
		biasRatio := offsetVoltage / (referenceVoltage - offsetVoltage)
		commandBiasValue, commandBiasFound = topologyCatalogResistanceClosest(
			requirement, inventory, anchorResistance/biasRatio,
		)
	}
	currentTarget := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric == "output_current" {
			currentTarget = math.Max(currentTarget, assertionTarget(assertion))
		}
	}
	senseValue, senseFound := topologyCatalogResistanceClosest(
		requirement, inventory, referenceVoltage/currentTarget,
	)
	if !lowerDividerFound || !upperDividerFound || !anchorFound || !commandBiasFound ||
		currentTarget <= 0 || !senseFound {
		return nil
	}

	lowerInput := uniqueBetween(inputNode, lowerSignal)
	lowerFeedback := seriesPair(insideNode, lowerSignal)
	upperInput := uniqueBetween(upperReference, upperThreshold)
	upperDecisionFeedback := seriesPair(insideNode, upperThreshold)
	lowerReferenceUpper := uniqueBetween(absoluteReference, lowerAmplifierTerminals["IN_PLUS"])
	lowerReferenceLower := uniqueBetween(lowerAmplifierTerminals["IN_PLUS"], referenceNode)
	lowerBufferFeedback := uniqueBetween(lowerReference, lowerAmplifierTerminals["IN_MINUS"])
	upperReferenceGround := uniqueBetween(upperAmplifierTerminals["IN_MINUS"], referenceNode)
	upperReferenceFeedback := seriesPair(upperReference, upperAmplifierTerminals["IN_MINUS"])
	referenceBias := uniqueBetween(driveSupply, absoluteReference)
	insidePullup := uniqueBetween(absoluteReference, insideNode)
	gatePulldown := uniqueBetween(gateNode, referenceNode)
	senseReturn := uniqueBetween(senseNode, referenceNode)
	if lowerInput == "" || len(lowerFeedback) != 2 || upperInput == "" ||
		len(upperDecisionFeedback) != 2 || lowerReferenceUpper == "" ||
		lowerReferenceLower == "" || lowerBufferFeedback == "" ||
		upperReferenceGround == "" || len(upperReferenceFeedback) != 2 ||
		referenceBias == "" || insidePullup == "" || gatePulldown == "" || senseReturn == "" {
		return nil
	}
	if currentFeedbackNode != senseNode && (commandSense == "" || commandBias == "") {
		return nil
	}

	values := map[string]float64{
		lowerInput:           inputResistance,
		upperInput:           inputResistance,
		lowerReferenceUpper:  lowerUpper,
		lowerReferenceLower:  lowerBranches[0],
		lowerBufferFeedback:  anchorValue,
		upperReferenceGround: upperGround,
		referenceBias:        anchorValue,
		insidePullup:         anchorValue,
		gatePulldown:         anchorValue,
		senseReturn:          senseValue,
	}
	if commandSense != "" {
		values[commandSense] = anchorValue
		values[commandBias] = commandBiasValue
	}
	for index, id := range lowerFeedback {
		values[id] = feedbackValues[index]
	}
	for index, id := range upperDecisionFeedback {
		values[id] = feedbackValues[index]
	}
	for index, id := range upperReferenceFeedback {
		values[id] = upperFeedback[index]
	}
	if instance.Kind == "resistor" {
		value := values[instance.ID]
		if value <= 0 || !finite(value) {
			return nil
		}
		return []AnalyticScale{{
			ID:         "topology:windowed_current_control:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    value,
			Unit:       "ohm",
			Derivation: "coupled catalog value derived from bounded thresholds, hysteresis, input impedance, reference voltage, and controlled output current",
			SourceKind: "candidate_topology",
			SourceID:   insideNode,
			Priority:   1,
		}}
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	if (first == driveSupply && second == referenceNode) ||
		(first == referenceNode && second == driveSupply) {
		return []AnalyticScale{{
			ID:         "topology:windowed_current_control_bypass:" + instance.ID,
			Kind:       "capacitance",
			ValueSI:    100e-9,
			Unit:       "F",
			Derivation: "bounded local bypass for the shared decision and current-control supply",
			SourceKind: "candidate_topology",
			SourceID:   driveSupply,
			Priority:   1,
		}}
	}
	if (first == gateNode && second == referenceNode) ||
		(first == referenceNode && second == gateNode) {
		return []AnalyticScale{{
			ID:         "topology:windowed_current_control_gate:" + instance.ID,
			Kind:       "capacitance",
			ValueSI:    100e-9,
			Unit:       "F",
			Derivation: "bounded gate filtering for deterministic startup and switching response",
			SourceKind: "candidate_topology",
			SourceID:   gateNode,
			Priority:   1,
		}}
	}
	return nil
}

func deriveWindowTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) || len(instance.Terminals) != 2 {
		return nil
	}
	envelope, required := topologyWindowBehaviorEnvelope(requirement)
	if !required {
		return nil
	}
	if envelope.signed {
		return deriveSignedWindowTopologyScales(requirement, graph, instance, inventory, envelope)
	}
	supplyNodes := topologyNodesByRole(graph, "supply")
	referenceNodes := topologyNodesByRole(graph, "reference")
	if len(supplyNodes) != 1 || len(referenceNodes) != 1 {
		return nil
	}
	if instance.Kind == "capacitor" {
		first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
		if (first == supplyNodes[0] && second == referenceNodes[0]) ||
			(first == referenceNodes[0] && second == supplyNodes[0]) {
			return []AnalyticScale{{
				ID:   "topology:window_supply_bypass:" + instance.ID,
				Kind: "capacitance", ValueSI: 100e-9, Unit: "F",
				Derivation: "bounded local bypass for a composed multi-decision transfer",
				SourceKind: "candidate_topology", SourceID: supplyNodes[0], Priority: 1,
			}}
		}
		return nil
	}
	supplyVoltage := nominalSupplyVoltage(requirement)
	if supplyVoltage <= envelope.upperV {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	var finalDecision GraphInstance
	for _, decision := range graph.Instances {
		if decision.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(decision)
		if nodeByID[terminals["OUT"]].Scope == "external" &&
			nodeByID[terminals["OUT"]].Role == "output" &&
			nodeByID[terminals["IN_MINUS"]].Scope == "internal" &&
			nodeByID[terminals["IN_PLUS"]].Scope == "internal" {
			finalDecision = decision
			break
		}
	}
	if finalDecision.ID == "" {
		return nil
	}
	finalTerminals := topologyTerminalNodes(finalDecision)
	insideNode, inverterReference := "", ""
	for _, candidateNode := range []string{finalTerminals["IN_PLUS"], finalTerminals["IN_MINUS"]} {
		outputs := 0
		for _, decision := range graph.Instances {
			if decision.Kind == "comparator" && decision.ID != finalDecision.ID &&
				topologyTerminalNodes(decision)["OUT"] == candidateNode {
				outputs++
			}
		}
		if outputs == 2 {
			insideNode = candidateNode
		}
	}
	if insideNode == finalTerminals["IN_PLUS"] {
		inverterReference = finalTerminals["IN_MINUS"]
	} else if insideNode == finalTerminals["IN_MINUS"] {
		inverterReference = finalTerminals["IN_PLUS"]
	}
	if insideNode == "" || inverterReference == "" {
		return nil
	}
	thresholdByNode := map[string]float64{inverterReference: supplyVoltage / 2}
	insideDecisions := 0
	for _, decision := range graph.Instances {
		if decision.Kind != "comparator" || decision.ID == finalDecision.ID {
			continue
		}
		terminals := topologyTerminalNodes(decision)
		if terminals["OUT"] != insideNode {
			continue
		}
		switch {
		case nodeByID[terminals["IN_PLUS"]].Scope == "external" &&
			nodeByID[terminals["IN_PLUS"]].Role == "input" &&
			nodeByID[terminals["IN_MINUS"]].Scope == "internal":
			thresholdByNode[terminals["IN_MINUS"]] = envelope.lowerV
			insideDecisions++
		case nodeByID[terminals["IN_MINUS"]].Scope == "external" &&
			nodeByID[terminals["IN_MINUS"]].Role == "input" &&
			nodeByID[terminals["IN_PLUS"]].Scope == "internal":
			thresholdByNode[terminals["IN_PLUS"]] = envelope.upperV
			insideDecisions++
		}
	}
	if insideDecisions != 2 || len(thresholdByNode) != 3 {
		return nil
	}
	between := func(left, right string) []string {
		ids := []string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (first == left && second == right) || (first == right && second == left) {
				ids = append(ids, candidate.ID)
			}
		}
		slices.Sort(ids)
		return ids
	}
	const anchorResistance = 10_000.0
	absoluteReference := ""
	referenceVoltage := 0.0
	for _, candidate := range graph.Instances {
		if candidate.Kind != "reference_diode" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["ANODE"] != referenceNodes[0] {
			continue
		}
		for _, model := range inventory[candidate.PrimitiveKey].Models {
			if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
				continue
			}
			for _, parameter := range model.Parameters {
				if parameter.Name == "output_voltage_v" && parameter.Value > 0 {
					absoluteReference = terminals["CATHODE"]
					referenceVoltage = parameter.Value
				}
			}
		}
	}
	if absoluteReference == "" || referenceVoltage <= 0 {
		return nil
	}
	if slices.Contains(between(supplyNodes[0], absoluteReference), instance.ID) {
		return []AnalyticScale{{
			ID:   "topology:window_reference_bias:" + instance.ID,
			Kind: "resistance", ValueSI: anchorResistance, Unit: "ohm",
			Derivation: "bounded bias resistance for a reviewed absolute reference",
			SourceKind: "candidate_topology", SourceID: absoluteReference, Priority: 1,
		}}
	}
	for thresholdNode, thresholdVoltage := range thresholdByNode {
		if thresholdNode == inverterReference {
			continue
		}
		var amplifier GraphInstance
		for _, candidate := range graph.Instances {
			if candidate.Kind == "opamp" && topologyTerminalNodes(candidate)["OUT"] == thresholdNode {
				amplifier = candidate
				break
			}
		}
		if amplifier.ID == "" {
			continue
		}
		terminals := topologyTerminalNodes(amplifier)
		values := map[string]float64{}
		derivation := ""
		switch {
		case terminals["IN_PLUS"] == absoluteReference:
			groundBranches := between(terminals["IN_MINUS"], referenceNodes[0])
			feedbackBranches := []string{}
			for _, node := range graph.Nodes {
				if node.Scope != "internal" {
					continue
				}
				first := between(thresholdNode, node.ID)
				second := between(node.ID, terminals["IN_MINUS"])
				if len(first) == 1 && len(second) == 1 {
					feedbackBranches = []string{first[0], second[0]}
					break
				}
			}
			slices.Sort(feedbackBranches)
			if len(groundBranches) != 1 || len(feedbackBranches) != 2 {
				continue
			}
			groundValue, feedbackValues, found := catalogNonInvertingGainTriplet(
				requirement, inventory, thresholdVoltage/referenceVoltage,
			)
			if !found {
				continue
			}
			values[groundBranches[0]] = groundValue
			for index, branch := range feedbackBranches {
				values[branch] = feedbackValues[index]
			}
			derivation = "catalog-ranked non-inverting ratio derived from a reviewed reference and bounded window threshold"
		case thresholdVoltage < referenceVoltage:
			upperBranches := between(absoluteReference, terminals["IN_PLUS"])
			lowerBranches := between(terminals["IN_PLUS"], referenceNodes[0])
			bufferFeedback := between(thresholdNode, terminals["IN_MINUS"])
			if len(upperBranches) != 1 || len(lowerBranches) != 1 || len(bufferFeedback) != 1 {
				continue
			}
			upper, lower, found := catalogResistanceDivider(
				requirement,
				inventory,
				referenceVoltage/thresholdVoltage-1,
				anchorResistance,
				1,
				referenceVoltage,
				referenceVoltage,
				0,
				0,
			)
			feedback, feedbackFound := topologyCatalogResistanceClosest(
				requirement,
				inventory,
				anchorResistance,
			)
			if !found || !feedbackFound {
				continue
			}
			values[upperBranches[0]] = upper
			values[lowerBranches[0]] = lower[0]
			values[bufferFeedback[0]] = feedback
			derivation = "catalog-ranked attenuation ratio and unity buffer derived from a reviewed reference and bounded window threshold"
		default:
			continue
		}
		value := values[instance.ID]
		if value > 0 && finite(value) {
			return []AnalyticScale{{
				ID:   "topology:window_reference:" + thresholdNode + ":" + instance.ID,
				Kind: "resistance", ValueSI: value, Unit: "ohm",
				Derivation: derivation,
				SourceKind: "candidate_topology", SourceID: thresholdNode, Priority: 1,
			}}
		}
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	outputNode := finalTerminals["OUT"]
	if (first == supplyNodes[0] && (second == insideNode || second == outputNode)) ||
		(second == supplyNodes[0] && (first == insideNode || first == outputNode)) {
		return []AnalyticScale{{
			ID:   "topology:window_pullup:" + instance.ID,
			Kind: "resistance", ValueSI: anchorResistance, Unit: "ohm",
			Derivation: "bounded open-collector pull-up for a composed decision node",
			SourceKind: "candidate_topology", SourceID: outputNode, Priority: 1,
		}}
	}
	return nil
}

func deriveSignedWindowTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
	envelope windowBehaviorEnvelope,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) || len(instance.Terminals) != 2 {
		return nil
	}
	positiveRail, negativeRail, reference := "", "", ""
	positiveVoltage, negativeVoltage := math.Inf(-1), math.Inf(1)
	for _, node := range graph.Nodes {
		if node.Role == "reference" {
			if reference != "" {
				return nil
			}
			reference = node.ID
		}
		if node.Role != "supply" {
			continue
		}
		voltage, found := topologyNodeNominalVoltage(requirement, graph, node.ID)
		if !found {
			continue
		}
		if voltage > positiveVoltage {
			positiveRail, positiveVoltage = node.ID, voltage
		}
		if voltage < negativeVoltage {
			negativeRail, negativeVoltage = node.ID, voltage
		}
	}
	if positiveRail == "" || negativeRail == "" || reference == "" || positiveVoltage <= 0 || negativeVoltage >= 0 {
		return nil
	}
	first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
	if instance.Kind == "capacitor" {
		if (first == positiveRail && second == reference) || (first == reference && second == positiveRail) {
			return []AnalyticScale{{
				ID:   "topology:signed_window_supply_bypass:" + instance.ID,
				Kind: "capacitance", ValueSI: 100e-9, Unit: "F",
				Derivation: "bounded local bypass for a signed multi-decision transfer",
				SourceKind: "candidate_topology", SourceID: positiveRail, Priority: 1,
			}}
		}
		return nil
	}
	outputNode, insideNode := "port_"+envelope.output, ""
	for _, candidate := range graph.Instances {
		if candidate.Kind != "p_channel_mosfet" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["SOURCE"] == positiveRail && terminals["DRAIN"] == outputNode {
			insideNode = terminals["GATE"]
			break
		}
	}
	if insideNode == "" {
		return nil
	}
	lowerReference, upperReference := "", ""
	inputNode := "port_" + envelope.input
	for _, candidate := range graph.Instances {
		if candidate.Kind != "comparator" {
			continue
		}
		terminals := topologyTerminalNodes(candidate)
		if terminals["OUT"] != insideNode {
			continue
		}
		switch {
		case terminals["IN_PLUS"] == inputNode:
			lowerReference = terminals["IN_MINUS"]
		case terminals["IN_MINUS"] == inputNode:
			upperReference = terminals["IN_PLUS"]
		}
	}
	if lowerReference == "" || upperReference == "" {
		return nil
	}
	absoluteReference, lowerSum := "", ""
	referenceVoltage := 0.0
	for _, candidate := range graph.Instances {
		terminals := topologyTerminalNodes(candidate)
		switch candidate.Kind {
		case "reference_diode":
			if terminals["ANODE"] != reference {
				continue
			}
			for _, model := range inventory[candidate.PrimitiveKey].Models {
				if model.ModelID != simmodel.PrimitiveShuntVoltageReferenceV1 {
					continue
				}
				for _, parameter := range model.Parameters {
					if parameter.Name == "output_voltage_v" && parameter.Value > 0 {
						absoluteReference, referenceVoltage = terminals["CATHODE"], parameter.Value
					}
				}
			}
		case "opamp":
			if terminals["OUT"] == lowerReference && terminals["IN_PLUS"] == reference {
				lowerSum = terminals["IN_MINUS"]
			}
		}
	}
	if absoluteReference == "" || lowerSum == "" || referenceVoltage <= envelope.upperV {
		return nil
	}
	between := func(left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			candidateFirst, candidateSecond := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (candidateFirst == left && candidateSecond == right) ||
				(candidateFirst == right && candidateSecond == left) {
				return candidate.ID
			}
		}
		return ""
	}
	values := map[string]float64{}
	const anchorResistance = 10_000.0
	pullup, pullupFound := topologyCatalogResistanceClosest(requirement, inventory, anchorResistance)
	pulldown, pulldownFound := topologyCatalogResistanceClosest(requirement, inventory, 100_000)
	if !pullupFound || !pulldownFound {
		return nil
	}
	values[between(positiveRail, insideNode)] = pullup
	values[between(outputNode, reference)] = pulldown
	values[between(positiveRail, absoluteReference)] = pullup
	upperDivider, upperLower, upperFound := catalogResistanceDivider(
		requirement, inventory, referenceVoltage/envelope.upperV-1, anchorResistance, 1,
		referenceVoltage, referenceVoltage, 0, 0,
	)
	negativeFeedback, negativeInput, negativeFound := catalogResistanceDivider(
		requirement, inventory, math.Abs(envelope.lowerV)/referenceVoltage, anchorResistance, 1,
		1, 1, 0, 0,
	)
	if !upperFound || len(upperLower) != 1 || !negativeFound || len(negativeInput) != 1 {
		return nil
	}
	values[between(absoluteReference, upperReference)] = upperDivider
	values[between(upperReference, reference)] = upperLower[0]
	values[between(absoluteReference, lowerSum)] = negativeInput[0]
	values[between(lowerReference, lowerSum)] = negativeFeedback
	for id := range values {
		if id == "" {
			return nil
		}
	}
	if len(values) != 7 {
		return nil
	}
	value, found := values[instance.ID]
	if !found || value <= 0 || !finite(value) {
		return nil
	}
	return []AnalyticScale{{
		ID:   "topology:signed_window:" + instance.ID,
		Kind: "resistance", ValueSI: value, Unit: "ohm",
		Derivation: "catalog-ranked signed threshold divider or decision-node bias derived from bipolar window bounds",
		SourceKind: "candidate_topology", SourceID: outputNode, Priority: 1,
	}}
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

func deriveFullWaveTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) || len(instance.Terminals) != 2 {
		return nil
	}
	envelope, required := topologyFullWaveBehaviorEnvelope(requirement)
	if !required {
		return nil
	}
	opamps, diodes := 0, 0
	for _, candidate := range graph.Instances {
		if candidate.Kind == "opamp" {
			opamps++
		}
		if candidate.Kind == "signal_diode" {
			diodes++
		}
	}
	if opamps != 2 || diodes != 2 {
		return nil
	}
	if instance.Kind == "capacitor" {
		first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
		for _, amplifier := range graph.Instances {
			if amplifier.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(amplifier)
			if (first == terminals["OUT"] && second == terminals["IN_MINUS"]) ||
				(first == terminals["IN_MINUS"] && second == terminals["OUT"]) {
				return []AnalyticScale{{
					ID:   "topology:full_wave_compensation:" + instance.ID,
					Kind: "capacitance", ValueSI: 10e-12, Unit: "F",
					Derivation: "bounded feedback compensation above the declared full-wave signal band",
					SourceKind: "candidate_topology", SourceID: envelope.output, Priority: 1,
				}}
			}
		}
		return nil
	}
	between := func(left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (first == left && second == right) || (first == right && second == left) {
				return candidate.ID
			}
		}
		return ""
	}
	references := topologyNodesByRole(graph, "reference")
	if len(references) != 1 {
		return nil
	}
	reference := references[0]
	halfSum, halfDrive, halfOutput := "", "", ""
	for _, amplifier := range graph.Instances {
		if amplifier.Kind != "opamp" {
			continue
		}
		amplifierTerminals := topologyTerminalNodes(amplifier)
		if amplifierTerminals["IN_PLUS"] != reference {
			continue
		}
		for _, forward := range graph.Instances {
			if forward.Kind != "signal_diode" {
				continue
			}
			forwardTerminals := topologyTerminalNodes(forward)
			if forwardTerminals["ANODE"] != amplifierTerminals["OUT"] {
				continue
			}
			for _, alternate := range graph.Instances {
				if alternate.Kind != "signal_diode" || alternate.ID == forward.ID {
					continue
				}
				alternateTerminals := topologyTerminalNodes(alternate)
				if alternateTerminals["CATHODE"] == amplifierTerminals["OUT"] &&
					alternateTerminals["ANODE"] == amplifierTerminals["IN_MINUS"] {
					halfSum = amplifierTerminals["IN_MINUS"]
					halfDrive = amplifierTerminals["OUT"]
					halfOutput = forwardTerminals["CATHODE"]
					break
				}
			}
			if halfOutput != "" {
				break
			}
		}
		if halfOutput != "" {
			break
		}
	}
	if halfSum == "" || halfDrive == "" || halfOutput == "" {
		return nil
	}
	value := 47_000.0
	if instance.ID == between("port_"+envelope.input, halfSum) ||
		instance.ID == between(halfOutput, halfSum) {
		value = 169_000
	} else if instance.ID == between(halfOutput, reference) {
		value = 1_000
	}
	return []AnalyticScale{{
		ID:   "topology:full_wave_ratio:" + instance.ID,
		Kind: "resistance", ValueSI: value, Unit: "ohm",
		Derivation: "equal-ratio resistance for a bounded precision full-wave transfer",
		SourceKind: "candidate_topology", SourceID: envelope.output, Priority: 1,
	}}
}

func deriveAnalogTransferTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	gain, cutoff := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "voltage_gain":
			target := assertionTarget(assertion)
			if target > gain {
				gain = target
			}
		case "cutoff_frequency", "bandwidth":
			target := assertionTarget(assertion)
			if assertion.Metric == "bandwidth" && assertion.Min != nil {
				target = *assertion.Min * minimumBandwidthDesignFactor
			}
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
		supplyNode := ""
		for _, node := range graph.Nodes {
			if node.Role == "input" &&
				between("resistor", terminals["IN_MINUS"], node.ID) != "" {
				inputNode = node.ID
			}
			if node.Role == "reference" {
				referenceNode = node.ID
			}
			if node.Role == "supply" {
				supplyNode = node.ID
			}
		}
		feedbackInput := between("resistor", terminals["IN_MINUS"], inputNode)
		feedbackOutput := between("resistor", terminals["IN_MINUS"], terminals["OUT"])
		seriesFeedbackValues := map[string]float64{}
		if feedbackOutput == "" {
			leftValue, rightValue, found := catalogSeriesResistancePair(
				requirement, inventory, 10_000*gain,
			)
			if found {
				for _, middle := range graph.Nodes {
					if middle.Scope != "internal" || middle.ID == terminals["IN_MINUS"] ||
						middle.ID == terminals["IN_PLUS"] || middle.ID == terminals["OUT"] {
						continue
					}
					leftID := between("resistor", terminals["IN_MINUS"], middle.ID)
					rightID := between("resistor", middle.ID, terminals["OUT"])
					if leftID != "" && rightID != "" {
						seriesFeedbackValues[leftID] = leftValue
						seriesFeedbackValues[rightID] = rightValue
						break
					}
				}
			}
		}
		biasUpper := between("resistor", supplyNode, terminals["IN_PLUS"])
		biasLower := between("resistor", terminals["IN_PLUS"], referenceNode)
		biasCapacitor := between("capacitor", terminals["IN_PLUS"], referenceNode)
		if feedbackInput != "" && (feedbackOutput != "" || len(seriesFeedbackValues) == 2) && biasUpper != "" &&
			biasLower != "" && biasCapacitor != "" {
			bias, ok := topologyBiasedInvertingReference(
				requirement, graph, inputNode, terminals["OUT"], supplyNode, referenceNode, gain,
			)
			supplyVoltage, supplyOK := topologyNodeNominalVoltage(requirement, graph, supplyNode)
			referenceVoltage, referenceOK := topologyNodeNominalVoltage(requirement, graph, referenceNode)
			if !ok || !supplyOK || !referenceOK {
				continue
			}
			const anchorResistance = 10_000.0
			lowerResistance := anchorResistance
			upperResistance := lowerResistance * (supplyVoltage - bias) / (bias - referenceVoltage)
			theveninResistance := upperResistance * lowerResistance / (upperResistance + lowerResistance)
			switch instance.ID {
			case feedbackInput:
				return []AnalyticScale{{
					ID: "topology:affine_transfer:input", Kind: "resistance", ValueSI: anchorResistance, Unit: "ohm",
					Derivation: "neutral input resistance for bounded inverting magnitude gain", SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
				}}
			case feedbackOutput:
				return []AnalyticScale{{
					ID: "topology:affine_transfer:feedback", Kind: "resistance", ValueSI: anchorResistance * gain, Unit: "ohm",
					Derivation: "R_feedback=R_input*|A_v| for bounded affine magnitude gain", SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
				}}
			case biasLower:
				return []AnalyticScale{{
					ID: "topology:affine_transfer:bias_lower", Kind: "resistance", ValueSI: lowerResistance, Unit: "ohm",
					Derivation: "neutral lower resistance for the behavior-derived affine reference", SourceKind: "candidate_topology", SourceID: terminals["IN_PLUS"], Priority: 1,
				}}
			case biasUpper:
				return []AnalyticScale{{
					ID: "topology:affine_transfer:bias_upper", Kind: "resistance", ValueSI: upperResistance, Unit: "ohm",
					Derivation: "rail-divider ratio derived from nominal input, output, and magnitude gain", SourceKind: "candidate_topology", SourceID: terminals["IN_PLUS"], Priority: 1,
				}}
			case biasCapacitor:
				return []AnalyticScale{{
					ID: "topology:affine_transfer:bias_bypass", Kind: "capacitance", ValueSI: 10 / (2 * math.Pi * cutoff * theveninResistance), Unit: "F",
					Derivation: "bias-reference pole placed one decade below the required signal bandwidth", SourceKind: "candidate_topology", SourceID: terminals["IN_PLUS"], Priority: 1,
				}}
			}
			if value, found := seriesFeedbackValues[instance.ID]; found {
				side := "input_side"
				if between("resistor", instance.Terminals[0].Node, terminals["OUT"]) == instance.ID ||
					between("resistor", instance.Terminals[1].Node, terminals["OUT"]) == instance.ID {
					side = "output_side"
				}
				return []AnalyticScale{{
					ID: "topology:affine_transfer:series_feedback_" + side, Kind: "resistance", ValueSI: value, Unit: "ohm",
					Derivation: "catalog-ranked series feedback composition realizes the bounded affine magnitude gain", SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
				}}
			}
		}
		inputNode = ""
		referenceNode = ""
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

func deriveTransimpedanceTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	transimpedance, bandwidth := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "transimpedance":
			transimpedance = math.Max(transimpedance, assertionTarget(assertion))
		case "bandwidth":
			bandwidth = math.Max(bandwidth, assertionTarget(assertion))
		}
	}
	if transimpedance <= 0 {
		return nil
	}
	between := func(kind, left, right string) string {
		for _, candidate := range graph.Instances {
			if candidate.Kind != kind || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if (first == left && second == right) || (first == right && second == left) {
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
		feedbackResistor := between("resistor", terminals["IN_MINUS"], terminals["OUT"])
		seriesFeedback := []string{}
		if feedbackResistor == "" {
			for _, node := range graph.Nodes {
				if node.Scope != "internal" {
					continue
				}
				first := between("resistor", terminals["IN_MINUS"], node.ID)
				second := between("resistor", node.ID, terminals["OUT"])
				if first != "" && second != "" && first != second {
					seriesFeedback = []string{first, second}
					break
				}
			}
		}
		switch instance.ID {
		case feedbackResistor:
			if feedbackResistor == "" {
				break
			}
			return []AnalyticScale{{
				ID: "topology:current_to_voltage:feedback_resistance", Kind: "resistance",
				ValueSI: transimpedance, Unit: "ohm",
				Derivation: "R=V/I for bounded current-to-voltage feedback",
				SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
			}}
		}
		if slices.Contains(seriesFeedback, instance.ID) {
			slices.Sort(seriesFeedback)
			branchValue := transimpedance / float64(len(seriesFeedback))
			if len(seriesFeedback) == 2 {
				left, right, found := catalogSeriesResistancePair(requirement, inventory, transimpedance)
				if found {
					if instance.ID == seriesFeedback[0] {
						branchValue = left
					} else {
						branchValue = right
					}
				}
			}
			return []AnalyticScale{{
				ID: "topology:current_to_voltage:series_feedback_resistance", Kind: "resistance",
				ValueSI: branchValue, Unit: "ohm",
				Derivation: "catalog-ranked series resistance composition for a bounded current-to-voltage feedback impedance",
				SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
			}}
		}
		feedbackCapacitor := between("capacitor", terminals["IN_MINUS"], terminals["OUT"])
		if instance.ID == feedbackCapacitor && bandwidth > 0 {
			return []AnalyticScale{{
				ID: "topology:current_to_voltage:feedback_capacitance", Kind: "capacitance",
				ValueSI: 1 / (2 * math.Pi * bandwidth * transimpedance), Unit: "F",
				Derivation: "C=1/(2*pi*f*R) for bounded current-to-voltage feedback bandwidth",
				SourceKind: "candidate_topology", SourceID: terminals["IN_MINUS"], Priority: 1,
			}}
		}
	}
	return nil
}

func catalogSeriesResistancePair(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	target float64,
) (float64, float64, bool) {
	if target <= 0 {
		return 0, 0, false
	}
	type choice struct {
		value float64
		key   string
	}
	choices := []choice{}
	requiredAnalyses := requirementAnalysisSet(requirement)
	for _, primitive := range inventory {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		minimum, maximum, ok := effectiveValueRange(*primitive.ValueDomain)
		if !ok || minimum != maximum || minimum <= 0 {
			continue
		}
		choices = append(choices, choice{value: minimum, key: primitive.Key})
	}
	derivedTargets := []float64{target / 2}
	for _, existing := range choices {
		if existing.value < target {
			derivedTargets = append(derivedTargets, target-existing.value)
		}
	}
	for _, derivedTarget := range derivedTargets {
		value, found := topologyCatalogResistanceClosest(requirement, inventory, derivedTarget)
		if !found || value <= 0 {
			continue
		}
		choices = append(choices, choice{value: value, key: "derived:" + canonicalOptionalFloat(&value)})
	}
	slices.SortFunc(choices, func(left, right choice) int {
		return cmp.Or(cmp.Compare(left.value, right.value), cmp.Compare(left.key, right.key))
	})
	choices = slices.CompactFunc(choices, func(left, right choice) bool {
		return left.value == right.value
	})
	bestLeft, bestRight, bestError := 0.0, 0.0, math.Inf(1)
	bestKey := ""
	for leftIndex, left := range choices {
		for rightIndex := leftIndex; rightIndex < len(choices); rightIndex++ {
			right := choices[rightIndex]
			error := math.Abs(left.value+right.value-target) / target
			key := canonicalOptionalFloat(&left.value) + "|" + left.key + "|" + canonicalOptionalFloat(&right.value) + "|" + right.key
			if error < bestError || (error == bestError && (bestKey == "" || key < bestKey)) {
				bestLeft, bestRight, bestError, bestKey = left.value, right.value, error, key
			}
		}
	}
	return bestLeft, bestRight, bestKey != ""
}

// catalogSeriesParallelResistanceTriplet realizes a target resistance from
// one series branch followed by two parallel branches. Only reviewed,
// fixed-value primitives that cover every requested analysis and rating are
// considered. This gives sparse catalogs a deterministic way to approach a
// derived impedance without manufacturing an unevidenced component value.
func catalogSeriesParallelResistanceTriplet(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	target float64,
) (float64, float64, float64, bool) {
	if target <= 0 {
		return 0, 0, 0, false
	}
	type choice struct {
		value float64
		key   string
	}
	choices := []choice{}
	requiredAnalyses := requirementAnalysisSet(requirement)
	for _, primitive := range inventory {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		minimum, maximum, ok := effectiveValueRange(*primitive.ValueDomain)
		if !ok || minimum != maximum || minimum <= 0 {
			continue
		}
		choices = append(choices, choice{value: minimum, key: primitive.Key})
	}
	slices.SortFunc(choices, func(left, right choice) int {
		return cmp.Or(cmp.Compare(left.value, right.value), cmp.Compare(left.key, right.key))
	})
	choices = slices.CompactFunc(choices, func(left, right choice) bool {
		return left.value == right.value
	})
	type parallelChoice struct {
		value     float64
		first     float64
		second    float64
		branchKey string
	}
	parallelChoices := make([]parallelChoice, 0, len(choices)*(len(choices)+1)/2)
	for firstIndex, first := range choices {
		for secondIndex := firstIndex; secondIndex < len(choices); secondIndex++ {
			second := choices[secondIndex]
			parallelChoices = append(parallelChoices, parallelChoice{
				value:  1 / (1/first.value + 1/second.value),
				first:  first.value,
				second: second.value,
				branchKey: canonicalOptionalFloat(&first.value) + "|" + first.key + "|" +
					canonicalOptionalFloat(&second.value) + "|" + second.key,
			})
		}
	}
	slices.SortFunc(parallelChoices, func(left, right parallelChoice) int {
		return cmp.Or(cmp.Compare(left.value, right.value), cmp.Compare(left.branchKey, right.branchKey))
	})
	parallelChoices = slices.CompactFunc(parallelChoices, func(left, right parallelChoice) bool {
		return left.value == right.value
	})
	bestSeries, bestParallelA, bestParallelB := 0.0, 0.0, 0.0
	bestError := math.Inf(1)
	bestKey := ""
	for _, series := range choices {
		desiredParallel := target - series.value
		index, _ := slices.BinarySearchFunc(parallelChoices, desiredParallel, func(candidate parallelChoice, target float64) int {
			return cmp.Compare(candidate.value, target)
		})
		for _, candidateIndex := range []int{index - 1, index} {
			if candidateIndex < 0 || candidateIndex >= len(parallelChoices) {
				continue
			}
			parallel := parallelChoices[candidateIndex]
			realized := series.value + parallel.value
			error := math.Abs(realized-target) / target
			key := canonicalOptionalFloat(&series.value) + "|" + series.key + "|" + parallel.branchKey
			if error < bestError || (error == bestError && (bestKey == "" || key < bestKey)) {
				bestSeries, bestParallelA, bestParallelB = series.value, parallel.first, parallel.second
				bestError, bestKey = error, key
			}
		}
	}
	return bestSeries, bestParallelA, bestParallelB, bestKey != ""
}

// catalogSeriesResistancePairPreservingBranch treats a series split as a
// minimal graph repair: keep one existing branch value and select only the
// complementary catalog value needed to approach the derived total. Both
// orientations are considered, so the result does not depend on instance ID
// or terminal order. Trusted simulation remains the authority when the sparse
// catalog cannot realize the exact total.
func catalogSeriesResistancePairPreservingBranch(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	target float64,
	left GraphInstance,
	right GraphInstance,
) (float64, float64, bool) {
	if target <= 0 {
		return 0, 0, false
	}
	type candidate struct {
		left        float64
		right       float64
		targetError float64
		changeError float64
		key         string
	}
	candidates := []candidate{}
	add := func(preserved GraphInstance, preserveLeft bool) {
		if preserved.ValueSI == nil || *preserved.ValueSI <= 0 || *preserved.ValueSI >= target {
			return
		}
		complement, found := topologyCatalogResistanceClosest(
			requirement,
			inventory,
			target-*preserved.ValueSI,
		)
		if !found {
			return
		}
		leftValue, rightValue := complement, *preserved.ValueSI
		if preserveLeft {
			leftValue, rightValue = *preserved.ValueSI, complement
		}
		candidates = append(candidates, candidate{
			left:        leftValue,
			right:       rightValue,
			targetError: math.Abs(leftValue+rightValue-target) / target,
			changeError: seriesPairAssignmentError(left, leftValue) + seriesPairAssignmentError(right, rightValue),
			key:         canonicalOptionalFloat(&leftValue) + "|" + canonicalOptionalFloat(&rightValue),
		})
	}
	add(left, true)
	add(right, false)
	if len(candidates) == 0 {
		return catalogSeriesResistancePair(requirement, inventory, target)
	}
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Or(
			cmp.Compare(left.targetError, right.targetError),
			cmp.Compare(left.changeError, right.changeError),
			cmp.Compare(left.key, right.key),
		)
	})
	return candidates[0].left, candidates[0].right, true
}

func catalogNonInvertingGainTriplet(
	requirement Requirement,
	inventory map[string]PrimitiveCandidate,
	targetGain float64,
) (float64, []float64, bool) {
	if targetGain <= 1 || !finite(targetGain) {
		return 0, nil, false
	}
	requiredAnalyses := requirementAnalysisSet(requirement)
	valueSet := map[float64]bool{}
	for _, primitive := range inventory {
		if primitive.Kind != "resistor" || primitive.ValueDomain == nil ||
			!primitiveCoversAllAnalyses(primitive, requiredAnalyses) ||
			!ratingsCoverRequirement(requirement, primitive) {
			continue
		}
		minimum, maximum, ok := effectiveValueRange(*primitive.ValueDomain)
		if !ok || minimum != maximum || minimum < 1_000 || minimum > 1_000_000 {
			continue
		}
		valueSet[minimum] = true
	}
	values := make([]float64, 0, len(valueSet))
	for value := range valueSet {
		values = append(values, value)
	}
	slices.Sort(values)
	bestGround, bestError, bestAnchorError := 0.0, math.Inf(1), math.Inf(1)
	bestFeedback := []float64(nil)
	bestKey := ""
	for _, ground := range values {
		for leftIndex, left := range values {
			for rightIndex := leftIndex; rightIndex < len(values); rightIndex++ {
				right := values[rightIndex]
				gain := 1 + (left+right)/ground
				error := multiplicativeRelativeError(gain, targetGain)
				anchorError := multiplicativeRelativeError(ground, 10_000)
				key := canonicalOptionalFloat(&ground) + "|" +
					canonicalOptionalFloat(&left) + "|" + canonicalOptionalFloat(&right)
				if error < bestError ||
					(error == bestError && (anchorError < bestAnchorError ||
						(anchorError == bestAnchorError && (bestKey == "" || key < bestKey)))) {
					bestGround, bestFeedback = ground, []float64{left, right}
					bestError, bestAnchorError, bestKey = error, anchorError, key
				}
			}
		}
	}
	return bestGround, bestFeedback, bestKey != ""
}

func deriveControlledSwitchTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	lowerThreshold, upperThreshold := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		target := assertionTarget(assertion)
		switch assertion.Metric {
		case "falling_threshold":
			if target > 0 && (lowerThreshold == 0 || target < lowerThreshold) {
				lowerThreshold = target
			}
		case "rising_threshold":
			if target > upperThreshold {
				upperThreshold = target
			}
		}
	}
	between := func(left, right string) string {
		ids := []string{}
		for _, candidate := range graph.Instances {
			if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
				continue
			}
			first, second := candidate.Terminals[0].Node, candidate.Terminals[1].Node
			if first == left && second == right || first == right && second == left {
				ids = append(ids, candidate.ID)
			}
		}
		slices.Sort(ids)
		if len(ids) == 0 {
			return ""
		}
		return ids[0]
	}
	externalInputBranch := func(node string) (string, string) {
		for _, external := range graph.Nodes {
			if external.Scope != "external" ||
				(external.Role != "input" && external.Role != "control") ||
				(!requirementPortIsControl(requirement, external.SemanticID) &&
					!requirementPortDrivesDecision(requirement, external.SemanticID)) {
				continue
			}
			if branch := between(node, external.ID); branch != "" {
				return external.ID, branch
			}
		}
		return "", ""
	}
	branchByRole := func(node, role string) (string, string) {
		for _, other := range graph.Nodes {
			if other.Scope != "external" || other.Role != role {
				continue
			}
			if branch := between(node, other.ID); branch != "" {
				return other.ID, branch
			}
		}
		return "", ""
	}
	closest := func(target float64) (float64, bool) {
		return topologyCatalogResistanceClosest(requirement, inventory, target)
	}
	if scale, ok := deriveLevelShiftedHighSideSwitchScale(
		requirement, graph, instance, inventory,
	); ok {
		return []AnalyticScale{scale}
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
		controlledOutputs := map[string]bool{}
		var controlledDevice GraphInstance
		for _, active := range graph.Instances {
			if (active.Kind == "n_channel_mosfet" || active.Kind == "p_channel_mosfet") &&
				topologyTerminalNodes(active)["GATE"] == gateNode {
				controlledOutputs[topologyTerminalNodes(active)["DRAIN"]] = true
				if controlledDevice.ID == "" || active.ID < controlledDevice.ID {
					controlledDevice = active
				}
			}
		}
		if len(controlledOutputs) == 0 {
			continue
		}
		signalNode, thresholdNode, inputBranch := "", "", ""
		for _, terminal := range []string{"IN_PLUS", "IN_MINUS"} {
			node := decisionTerminals[terminal]
			if nodeByID[node].Scope == "external" &&
				(nodeByID[node].Role == "control" ||
					requirementPortIsControl(requirement, nodeByID[node].SemanticID) ||
					requirementPortDrivesDecision(requirement, nodeByID[node].SemanticID)) {
				signalNode = node
			} else if _, branch := externalInputBranch(node); branch != "" {
				signalNode, inputBranch = node, branch
			} else if nodeByID[node].Scope == "internal" {
				thresholdNode = node
			}
		}
		if signalNode == "" || thresholdNode == "" {
			continue
		}
		gateSupplyNode, gatePullup := branchByRole(gateNode, "supply")
		gateReferenceNode, gatePulldown := branchByRole(gateNode, "reference")
		highSide := controlledDevice.Kind == "p_channel_mosfet"
		if highSide && gateReferenceNode == "" {
			gateReferenceNode = decisionTerminals["V_MINUS"]
		}
		if gatePullup == "" || (!highSide && gatePulldown == "") {
			continue
		}
		gateSupply, supplyOK := topologyNodeNominalVoltage(requirement, graph, gateSupplyNode)
		gateReference, referenceOK := topologyNodeNominalVoltage(requirement, graph, gateReferenceNode)
		if !supplyOK || !referenceOK || gateSupply <= gateReference {
			continue
		}
		const pullupTarget = 10_000.0
		pulldownTarget := math.Inf(1)
		if gatePulldown != "" {
			pulldownTarget = 100_000
		}
		gateLimit := 0.0
		for _, rating := range inventory[controlledDevice.PrimitiveKey].Ratings {
			if strings.EqualFold(rating.Kind, "gate_source_voltage") {
				gateLimit = boundMaximum(rating)
				break
			}
		}
		gateReferenceMinimum, _, gateReferenceRangeOK := topologyDeclaredNodeVoltageRange(
			requirement, graph, gateReferenceNode,
		)
		_, gateSupplyMaximum, gateSupplyRangeOK := topologyDeclaredNodeVoltageRange(
			requirement, graph, gateSupplyNode,
		)
		maximumGateSpan := gateSupplyMaximum - gateReferenceMinimum
		if gateLimit > 0 && gateSupplyRangeOK && gateReferenceRangeOK &&
			maximumGateSpan > gateLimit {
			// Reserve 25 percent of the absolute gate rating for catalog and
			// operating-corner variation. The simulator remains authoritative
			// for both turn-on sufficiency and the exact rating envelope.
			targetFraction := .75 * gateLimit / maximumGateSpan
			if targetFraction > 0 && targetFraction < 1 {
				pulldownTarget = pullupTarget * targetFraction / (1 - targetFraction)
			}
		}
		pullup, pullupOK := closest(pullupTarget)
		pulldown, pulldownOK := math.Inf(1), true
		if gatePulldown != "" {
			pulldown, pulldownOK = closest(pulldownTarget)
		}
		if !pullupOK || !pulldownOK || pullup <= 0 || pulldown <= 0 {
			continue
		}
		primitive := inventory[decision.PrimitiveKey]
		onResistance := primitiveModelParameter(
			primitive,
			simmodel.PrimitiveComparatorOpenCollectorV1,
			"output_on_resistance_ohm",
		)
		if onResistance <= 0 {
			onResistance = pullup / 100
		}
		offResistance := primitiveModelParameter(
			primitive,
			simmodel.PrimitiveComparatorOpenCollectorV1,
			"output_off_resistance_ohm",
		)
		if offResistance <= 0 {
			offResistance = math.Inf(1)
		}
		parallel := func(left, right float64) float64 {
			if math.IsInf(left, 1) {
				return right
			}
			if math.IsInf(right, 1) {
				return left
			}
			return left * right / (left + right)
		}
		gateAt := func(shunt float64) float64 {
			if math.IsInf(shunt, 1) {
				return gateSupply
			}
			return (gateSupply*shunt + gateReference*pullup) / (pullup + shunt)
		}
		outputLow := gateAt(parallel(pulldown, onResistance))
		outputHigh := gateAt(parallel(pulldown, offResistance))
		if outputHigh <= outputLow {
			continue
		}
		values := map[string]float64{
			gatePullup: pullup,
		}
		if gatePulldown != "" {
			values[gatePulldown] = pulldown
		}
		derivation := "catalog-ranked open-collector pull network preserves a bounded inactive and active gate state"
		if lowerThreshold > 0 && upperThreshold > lowerThreshold {
			feedbackNode := ""
			if between(gateNode, signalNode) != "" {
				feedbackNode = signalNode
			} else if between(gateNode, thresholdNode) != "" {
				feedbackNode = thresholdNode
			}
			feedbackBranch := between(gateNode, feedbackNode)
			spanFraction := (upperThreshold - lowerThreshold) / (outputHigh - outputLow)
			switch {
			case feedbackNode == signalNode && spanFraction > 0:
				inputResistance, inputOK := closest(10_000)
				feedbackResistance, feedbackOK := closest(inputResistance / spanFraction)
				actualFeedbackFraction := spanFraction
				if inputOK && feedbackOK && feedbackResistance > 0 {
					actualFeedbackFraction = inputResistance / feedbackResistance
				}
				referenceVoltage := (upperThreshold + actualFeedbackFraction*outputLow) /
					(1 + actualFeedbackFraction)
				thresholdSupplyNode, thresholdUpper := branchByRole(thresholdNode, "supply")
				thresholdReferenceNode, thresholdLower := branchByRole(thresholdNode, "reference")
				thresholdSupply, thresholdSupplyOK := topologyNodeNominalVoltage(
					requirement, graph, thresholdSupplyNode,
				)
				thresholdReference, thresholdReferenceOK := topologyNodeNominalVoltage(
					requirement, graph, thresholdReferenceNode,
				)
				dividerRatio := (thresholdSupply - referenceVoltage) /
					(referenceVoltage - thresholdReference)
				dividerUpper, dividerLowers, dividerOK := catalogResistanceDivider(
					requirement,
					inventory,
					dividerRatio,
					10_000,
					1,
					thresholdSupply-thresholdReference,
					thresholdSupply-thresholdReference,
					0,
					0,
				)
				if inputOK && feedbackOK && thresholdSupplyOK && thresholdReferenceOK &&
					thresholdSupply > referenceVoltage && referenceVoltage > thresholdReference &&
					dividerOK && len(dividerLowers) == 1 && feedbackBranch != "" &&
					inputBranch != "" && thresholdUpper != "" && thresholdLower != "" {
					values[inputBranch] = inputResistance
					values[feedbackBranch] = feedbackResistance
					values[thresholdUpper] = dividerUpper
					values[thresholdLower] = dividerLowers[0]
					derivation = "coupled catalog ratios derive Schmitt thresholds from the requested rising and falling decisions and reviewed output swing"
				}
			case feedbackNode == thresholdNode && spanFraction > 0:
				thresholdSupplyNode, thresholdSupplyBranch := branchByRole(thresholdNode, "supply")
				thresholdReferenceNode, thresholdReferenceBranch := branchByRole(thresholdNode, "reference")
				thresholdSupply, thresholdSupplyOK := topologyNodeNominalVoltage(
					requirement, graph, thresholdSupplyNode,
				)
				thresholdReference, thresholdReferenceOK := topologyNodeNominalVoltage(
					requirement, graph, thresholdReferenceNode,
				)
				if thresholdSupplyOK && thresholdReferenceOK && thresholdSupply > thresholdReference &&
					thresholdSupplyBranch != "" && thresholdReferenceBranch != "" && feedbackBranch != "" {
					outputFraction := spanFraction
					supplyFraction := (lowerThreshold - thresholdReference -
						outputFraction*(outputLow-thresholdReference)) /
						(thresholdSupply - thresholdReference)
					referenceFraction := 1 - outputFraction - supplyFraction
					maximumFraction := max(outputFraction, supplyFraction, referenceFraction)
					if outputFraction > 0 && supplyFraction > 0 && referenceFraction > 0 {
						feedbackResistance, feedbackOK := closest(10_000 * maximumFraction / outputFraction)
						supplyResistance, supplyResistanceOK := closest(10_000 * maximumFraction / supplyFraction)
						referenceResistance, referenceResistanceOK := closest(10_000 * maximumFraction / referenceFraction)
						inputResistance, inputOK := closest(10_000)
						if feedbackOK && supplyResistanceOK && referenceResistanceOK && inputOK {
							values[feedbackBranch] = feedbackResistance
							values[thresholdSupplyBranch] = supplyResistance
							values[thresholdReferenceBranch] = referenceResistance
							if inputBranch != "" {
								values[inputBranch] = inputResistance
							}
							derivation = "catalog conductance fractions derive Schmitt thresholds from the requested rising and falling decisions and reviewed output swing"
						}
					}
				}
			}
		}
		first := instance.Terminals[0].Node
		second := instance.Terminals[1].Node
		if instance.Kind == "capacitor" {
			if first != gateNode && second != gateNode {
				continue
			}
			const anchorResistance = 10_000.0
			shuntBranches := 0
			for _, candidate := range graph.Instances {
				if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
					continue
				}
				if candidate.Terminals[0].Node == gateNode || candidate.Terminals[1].Node == gateNode {
					shuntBranches++
				}
			}
			if shuntBranches == 0 {
				shuntBranches = 1
			}
			theveninResistance := anchorResistance / float64(shuntBranches)
			edgeLimit := math.Inf(1)
			maximumFrequency := 0.0
			for _, assertion := range requirement.Requirements.BehavioralRequirements {
				observedNode := "port_" + assertion.Observation.ID
				if assertion.Observation.Kind != "port" || !controlledOutputs[observedNode] {
					continue
				}
				if slices.Contains([]string{"rise_time", "fall_time"}, assertion.Metric) &&
					assertion.Max != nil && *assertion.Max > 0 {
					edgeLimit = math.Min(edgeLimit, *assertion.Max)
				}
				if assertion.FrequencyHz != nil && *assertion.FrequencyHz > maximumFrequency {
					maximumFrequency = *assertion.FrequencyHz
				}
			}
			if maximumFrequency > 0 {
				edgeLimit = math.Min(edgeLimit, 1/(20*maximumFrequency))
			}
			capacitance := 10e-9
			if finite(edgeLimit) {
				capacitance = math.Min(capacitance, edgeLimit/(2.2*theveninResistance))
			}
			return []AnalyticScale{{
				ID:         "topology:default_state_hold:" + instance.ID,
				Kind:       "capacitance",
				ValueSI:    capacitance,
				Unit:       "F",
				Derivation: "control-node hold capacitance bounded by the switching period, requested transition time, and gate-network Thevenin resistance",
				SourceKind: "candidate_topology",
				SourceID:   decision.ID,
				Priority:   1,
			}}
		}
		if value := values[instance.ID]; value > 0 && finite(value) {
			return []AnalyticScale{{
				ID:         "topology:controlled_decision_network:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    value,
				Unit:       "ohm",
				Derivation: derivation,
				SourceKind: "candidate_topology",
				SourceID:   decision.ID,
				Priority:   1,
			}}
		}
		if first != gateNode && second != gateNode &&
			first != thresholdNode && second != thresholdNode &&
			first != signalNode && second != signalNode {
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

func deriveLevelShiftedHighSideSwitchScale(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) (AnalyticScale, bool) {
	const (
		inputImpedanceHeadroom     = 1.5
		loadedDecisionHighFraction = 0.85
		biasCorrectionCornerMargin = 1.35
		decisionNetworkTargetOhm   = 10_000.0
		levelShifterBaseTargetOhm  = 47_000.0
	)
	var decision, levelShifter, controlledDevice, stableReference GraphInstance
	for _, candidate := range graph.Instances {
		switch candidate.Kind {
		case "comparator":
			if decision.ID == "" || candidate.ID < decision.ID {
				decision = candidate
			}
		case "npn_bjt":
			if levelShifter.ID == "" || candidate.ID < levelShifter.ID {
				levelShifter = candidate
			}
		case "p_channel_mosfet":
			if controlledDevice.ID == "" || candidate.ID < controlledDevice.ID {
				controlledDevice = candidate
			}
		}
		primitive := inventory[candidate.PrimitiveKey]
		for _, model := range primitive.Models {
			if model.ModelID == simmodel.PrimitiveShuntVoltageReferenceV1 &&
				(stableReference.ID == "" || candidate.ID < stableReference.ID) {
				stableReference = candidate
			}
		}
	}
	if decision.ID == "" || levelShifter.ID == "" || controlledDevice.ID == "" || stableReference.ID == "" {
		return AnalyticScale{}, false
	}
	decisionTerminals := topologyTerminalNodes(decision)
	shifterTerminals := topologyTerminalNodes(levelShifter)
	switchTerminals := topologyTerminalNodes(controlledDevice)
	referenceTerminals := topologyTerminalNodes(stableReference)
	decisionNode := decisionTerminals["OUT"]
	signalNode := decisionTerminals["IN_PLUS"]
	thresholdNode := decisionTerminals["IN_MINUS"]
	baseNode := shifterTerminals["BASE"]
	gateNode := shifterTerminals["COLLECTOR"]
	referenceNode := shifterTerminals["EMITTER"]
	stableNode := referenceTerminals["CATHODE"]
	if decisionNode == "" || signalNode == "" || thresholdNode == "" || baseNode == "" ||
		gateNode == "" || referenceNode == "" || stableNode == "" ||
		referenceTerminals["ANODE"] != referenceNode || switchTerminals["GATE"] != gateNode ||
		switchTerminals["SOURCE"] == "" {
		return AnalyticScale{}, false
	}
	if len(topologyResistorPath(graph, decisionNode, baseNode)) != 1 ||
		len(topologyResistorPath(graph, decisionNode, signalNode)) != 2 ||
		len(topologyResistorPath(graph, stableNode, signalNode)) != 2 {
		return AnalyticScale{}, false
	}
	referenceVoltage, referenceKnown := topologyPrimitiveReferenceVoltage(inventory[stableReference.PrimitiveKey])
	if !referenceKnown || referenceVoltage <= 0 {
		return AnalyticScale{}, false
	}
	lowerThreshold, upperThreshold := 0.0, 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		target := assertionTarget(assertion)
		switch assertion.Metric {
		case "falling_threshold":
			if target > 0 && (lowerThreshold == 0 || target < lowerThreshold) {
				lowerThreshold = target
			}
		case "rising_threshold":
			if target > upperThreshold {
				upperThreshold = target
			}
		}
	}
	if lowerThreshold <= 0 || upperThreshold <= lowerThreshold {
		return AnalyticScale{}, false
	}
	inputNode := ""
	for _, node := range graph.Nodes {
		if node.Scope != "external" ||
			(!requirementPortIsControl(requirement, node.SemanticID) &&
				!requirementPortDrivesDecision(requirement, node.SemanticID)) {
			continue
		}
		if len(topologyResistorPath(graph, node.ID, signalNode)) == 1 {
			inputNode = node.ID
			break
		}
	}
	if inputNode == "" {
		return AnalyticScale{}, false
	}
	inputMinimum := 10_000.0
	for _, port := range requirement.Requirements.Ports {
		if "port_"+port.ID == inputNode && port.Electrical.InputImpedanceMinOhm != nil {
			inputMinimum = math.Max(inputMinimum, *port.Electrical.InputImpedanceMinOhm)
		}
	}
	inputResistance, inputOK := topologyCatalogResistanceClosest(
		requirement, inventory, inputImpedanceHeadroom*inputMinimum,
	)
	// Reserve output swing for pull-up loading, level-shifter base drive, and
	// threshold-corner separation instead of sizing hysteresis from an ideal,
	// unloaded decision high.
	decisionHigh := loadedDecisionHighFraction * referenceVoltage
	feedbackTarget := decisionHigh * inputResistance / (upperThreshold - lowerThreshold)
	feedbackSeries, feedbackParallelA, feedbackParallelB, feedbackOK := catalogSeriesParallelResistanceTriplet(
		requirement, inventory, feedbackTarget,
	)
	thresholdResistance, thresholdOK := topologyCatalogResistanceClosest(requirement, inventory, decisionNetworkTargetOhm)
	if !inputOK || !feedbackOK || !thresholdOK || inputResistance <= 0 ||
		feedbackSeries <= 0 || feedbackParallelA <= 0 || feedbackParallelB <= 0 || thresholdResistance <= 0 {
		return AnalyticScale{}, false
	}
	feedbackResistance := feedbackSeries + 1/(1/feedbackParallelA+1/feedbackParallelB)
	thresholdVoltage := referenceVoltage / 2
	bottomConductance := upperThreshold/inputResistance/thresholdVoltage -
		1/inputResistance - 1/feedbackResistance
	if bottomConductance <= 0 || !finite(bottomConductance) {
		return AnalyticScale{}, false
	}
	inputBottom, bottomOK := topologyCatalogResistanceClosest(requirement, inventory, 1/bottomConductance)
	if !bottomOK || inputBottom <= 0 {
		return AnalyticScale{}, false
	}
	realizedUpperThreshold := thresholdVoltage * (1 +
		inputResistance/inputBottom + inputResistance/feedbackResistance)
	quantizationError := realizedUpperThreshold - upperThreshold
	biasResistance := 0.0
	if quantizationError > 0 {
		// Keep correction current below the ideal nominal cancellation so
		// reference, comparator-offset, and resistor corners retain margin on
		// both sides of the requested threshold windows.
		biasResistance = biasCorrectionCornerMargin * (referenceVoltage - thresholdVoltage) * inputResistance / quantizationError
	}
	biasFirst, biasSecond, biasOK := catalogSeriesResistancePair(
		requirement, inventory, biasResistance,
	)
	baseResistance, baseOK := topologyCatalogResistanceClosest(requirement, inventory, levelShifterBaseTargetOhm)
	pullResistance, pullOK := topologyCatalogResistanceClosest(requirement, inventory, decisionNetworkTargetOhm)
	if !biasOK || !baseOK || !pullOK {
		return AnalyticScale{}, false
	}
	values := map[string]float64{}
	setPath := func(start, end string, pathValues ...float64) bool {
		path := topologyResistorPath(graph, start, end)
		if len(path) != len(pathValues) {
			return false
		}
		for index, id := range path {
			values[id] = pathValues[index]
		}
		return true
	}
	setSeriesParallel := func(start, end string, series, parallelA, parallelB float64) bool {
		between := func(left, right string) []string {
			ids := []string{}
			for _, instance := range graph.Instances {
				if instance.Kind != "resistor" {
					continue
				}
				terminals := topologyTerminalNodes(instance)
				first, second := terminals["A"], terminals["B"]
				if (first == left && second == right) || (first == right && second == left) {
					ids = append(ids, instance.ID)
				}
			}
			slices.Sort(ids)
			return ids
		}
		for _, node := range graph.Nodes {
			if node.ID == start || node.ID == end {
				continue
			}
			seriesIDs := between(start, node.ID)
			parallelIDs := between(node.ID, end)
			if len(seriesIDs) != 1 || len(parallelIDs) != 2 {
				continue
			}
			values[seriesIDs[0]] = series
			values[parallelIDs[0]] = parallelA
			values[parallelIDs[1]] = parallelB
			return true
		}
		return false
	}
	supplyNode := switchTerminals["SOURCE"]
	if !setPath(supplyNode, stableNode, pullResistance) ||
		!setPath(stableNode, thresholdNode, thresholdResistance) ||
		!setPath(thresholdNode, referenceNode, thresholdResistance) ||
		!setPath(inputNode, signalNode, inputResistance) ||
		!setPath(signalNode, referenceNode, inputBottom) ||
		!setPath(stableNode, decisionNode, pullResistance) ||
		!setPath(decisionNode, baseNode, baseResistance) ||
		!setSeriesParallel(decisionNode, signalNode, feedbackSeries, feedbackParallelA, feedbackParallelB) ||
		!setPath(stableNode, signalNode, biasFirst, biasSecond) ||
		!setPath(supplyNode, gateNode, pullResistance) {
		return AnalyticScale{}, false
	}
	if instance.Kind == "capacitor" {
		first, second := instance.Terminals[0].Node, instance.Terminals[1].Node
		if first != gateNode && second != gateNode || first != supplyNode && second != supplyNode {
			return AnalyticScale{}, false
		}
		return AnalyticScale{
			ID: "topology:fail_safe_high_side_gate_hold:" + instance.ID, Kind: "capacitance",
			ValueSI: 10e-9, Unit: "F",
			Derivation: "gate-source hold capacitance preserves the high-side switch off state while the decision and level-shift rails establish",
			SourceKind: "candidate_topology", SourceID: decision.ID, Priority: 1,
		}, true
	}
	value := values[instance.ID]
	if instance.Kind != "resistor" || value <= 0 || !finite(value) {
		return AnalyticScale{}, false
	}
	return AnalyticScale{
		ID: "topology:absolute_high_side_decision_network:" + instance.ID, Kind: "resistance",
		ValueSI: value, Unit: "ohm",
		Derivation: "catalog-backed reference, input-divider, hysteresis, level-shift, and fail-safe gate ratios derive the requested absolute decision bounds",
		SourceKind: "candidate_topology", SourceID: decision.ID, Priority: 1,
	}, true
}

func deriveTransconductanceTopologyScales(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
	inventory map[string]PrimitiveCandidate,
) []AnalyticScale {
	if scale, found := deriveTransconductanceInputConditioningScale(
		requirement,
		graph,
		instance,
	); found {
		return []AnalyticScale{scale}
	}
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
	if transconductance <= 0 || !finite(transconductance) {
		return nil
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	type nodePair struct{ first, second string }
	resistorsBetween := map[nodePair][]string{}
	betweenKey := func(left, right string) nodePair {
		if right < left {
			left, right = right, left
		}
		return nodePair{first: left, second: right}
	}
	for _, candidate := range graph.Instances {
		if candidate.Kind != "resistor" || len(candidate.Terminals) != 2 {
			continue
		}
		key := betweenKey(candidate.Terminals[0].Node, candidate.Terminals[1].Node)
		resistorsBetween[key] = append(resistorsBetween[key], candidate.ID)
	}
	for key := range resistorsBetween {
		slices.Sort(resistorsBetween[key])
	}
	between := func(left, right string) string {
		instances := resistorsBetween[betweenKey(left, right)]
		if len(instances) == 0 {
			return ""
		}
		return instances[0]
	}
	betweenAll := func(left, right string) []string {
		return resistorsBetween[betweenKey(left, right)]
	}
	protectedPassSource := ""
	for _, passDevice := range graph.Instances {
		if passDevice.Kind != "npn_bjt" {
			continue
		}
		passTerminals := topologyTerminalNodes(passDevice)
		if nodeByID[passTerminals["COLLECTOR"]].Role != "output" ||
			nodeByID[passTerminals["EMITTER"]].Scope != "internal" {
			continue
		}
		controllerFound := false
		for _, active := range graph.Instances {
			if active.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if terminals["OUT"] == passTerminals["BASE"] &&
				terminals["IN_MINUS"] == passTerminals["EMITTER"] {
				controllerFound = true
				break
			}
		}
		if !controllerFound {
			continue
		}
		for _, currentSwitch := range graph.Instances {
			if currentSwitch.Kind != "n_channel_mosfet" {
				continue
			}
			terminals := topologyTerminalNodes(currentSwitch)
			if nodeByID[terminals["SOURCE"]].Role != "reference" {
				continue
			}
			sensePath := topologyResistorPath(
				graph, passTerminals["EMITTER"], terminals["DRAIN"],
			)
			if len(sensePath) == 0 {
				continue
			}
			if slices.Contains(sensePath, instance.ID) {
				return []AnalyticScale{{
					ID:   "topology:low_side_current_sense:" + instance.ID,
					Kind: "resistance", ValueSI: 1 / (transconductance * float64(len(sensePath))), Unit: "ohm",
					Derivation: "equal catalog-backed series segments realize the reciprocal transconductance sense impedance",
					SourceKind: "candidate_topology", SourceID: passDevice.ID, Priority: 1,
				}}
			}
			const controlAnchorResistance = 10_000.0
			return []AnalyticScale{{
				ID:   "topology:protected_current_control:" + instance.ID,
				Kind: "resistance", ValueSI: controlAnchorResistance, Unit: "ohm",
				Derivation: "neutral control-interface resistance limits gate-drive and protection bias current",
				SourceKind: "candidate_topology", SourceID: currentSwitch.ID, Priority: 1,
			}}
		}
	}
	for _, passDevice := range graph.Instances {
		if passDevice.Kind != "pnp_bjt" {
			continue
		}
		passTerminals := topologyTerminalNodes(passDevice)
		emitter := passTerminals["EMITTER"]
		collector := passTerminals["COLLECTOR"]
		supply := ""
		passRail := ""
		protectedSupply := false
		output := ""
		reference := ""
		for _, node := range graph.Nodes {
			if node.Role == "supply" &&
				(node.ID == emitter || between(node.ID, emitter) != "") {
				supply = node.ID
				passRail = node.ID
			}
			if node.Role == "output" && between(collector, node.ID) != "" {
				output = node.ID
			}
			if node.Role == "reference" {
				reference = node.ID
			}
		}
		for _, powerSwitch := range graph.Instances {
			if powerSwitch.Kind != "p_channel_mosfet" {
				continue
			}
			terminals := topologyTerminalNodes(powerSwitch)
			if nodeByID[terminals["SOURCE"]].Role != "supply" ||
				(emitter != terminals["DRAIN"] && between(terminals["DRAIN"], emitter) == "") {
				continue
			}
			supply = terminals["SOURCE"]
			passRail = terminals["DRAIN"]
			protectedSupply = true
			break
		}
		if supply == "" ||
			passRail == "" ||
			nodeByID[collector].Scope != "internal" ||
			output == "" ||
			reference == "" {
			continue
		}
		if protectedSupply {
			protectedPassSource = passDevice.ID
		}
		ballast := between(passRail, emitter)
		if emitter != passRail && instance.ID == ballast {
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
		shunts := betweenAll(collector, output)
		if slices.Contains(shunts, instance.ID) {
			value := 1 / transconductance
			derivation := "sense impedance is the reciprocal of bounded voltage-to-current transfer"
			if protectedSupply && instance.ValueSI != nil && *instance.ValueSI > 0 {
				value = *instance.ValueSI
				derivation = "catalog shunt combines with the differential observation ratio to realize reciprocal transconductance"
			}
			return []AnalyticScale{{
				ID:         "topology:current_sense_impedance:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    value,
				Unit:       "ohm",
				Derivation: derivation,
				SourceKind: "candidate_topology",
				SourceID:   passDevice.ID,
				Priority:   1,
			}}
		}
		bias := between(passTerminals["BASE"], passRail)
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
		for _, driver := range graph.Instances {
			if driver.Kind != "npn_bjt" {
				continue
			}
			driverTerminals := topologyTerminalNodes(driver)
			if driverTerminals["COLLECTOR"] != passTerminals["BASE"] ||
				driverTerminals["EMITTER"] != reference {
				continue
			}
			for _, active := range graph.Instances {
				if active.Kind != "opamp" {
					continue
				}
				activeTerminals := topologyTerminalNodes(active)
				if instance.ID != between(activeTerminals["OUT"], driverTerminals["BASE"]) {
					continue
				}
				value := 10_000.0
				derivation := "buffered pass-device drive limits controller and discrete-driver base current"
				requiredCurrent := requiredTransconductanceOutputCurrent(requirement)
				passBeta := primitiveMinimumForwardBeta(inventory[passDevice.PrimitiveKey])
				driverBeta := primitiveMinimumForwardBeta(inventory[driver.PrimitiveKey])
				controllerLow, controllerHigh, swingOK := topologyDecisionOutputSwing(
					requirement, graph, active, inventory,
				)
				referenceVoltage, referenceOK := topologyNodeNominalVoltage(requirement, graph, reference)
				const conservativeDriverBaseEmitterDropV = 1.0
				const bufferedDriveReserve = 4.0
				availableDrive := controllerHigh - math.Max(controllerLow, referenceVoltage) -
					conservativeDriverBaseEmitterDropV
				if requiredCurrent > 0 && passBeta > 0 && driverBeta > 0 &&
					swingOK && referenceOK && availableDrive > 0 {
					value = availableDrive * passBeta * driverBeta /
						(bufferedDriveReserve * requiredCurrent)
					derivation = "buffered base-drive resistance follows reviewed controller swing, pass and driver minimum beta, required current, and drive reserve"
				}
				return []AnalyticScale{{
					ID:         "topology:buffered_pass_device_drive:" + instance.ID,
					Kind:       "resistance",
					ValueSI:    value,
					Unit:       "ohm",
					Derivation: derivation,
					SourceKind: "candidate_topology",
					SourceID:   driver.ID,
					Priority:   1,
				}}
			}
		}
		for _, active := range graph.Instances {
			if active.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(active)
			if instance.ID != between(passTerminals["BASE"], terminals["OUT"]) {
				continue
			}
			value := 100.0
			derivation := "bounded series resistance isolates the feedback controller from the nonlinear pass-device input"
			if protectedSupply {
				minimumSupply := minimumTransconductanceSupplyVoltage(requirement)
				requiredCurrent := requiredTransconductanceOutputCurrent(requirement)
				minimumBeta := primitiveMinimumForwardBeta(inventory[passDevice.PrimitiveKey])
				const conservativeBaseEmitterDropV = 1.0
				const baseDriveReserve = 2.0
				if minimumSupply <= conservativeBaseEmitterDropV || requiredCurrent <= 0 || minimumBeta <= 0 {
					continue
				}
				value = (minimumSupply - conservativeBaseEmitterDropV) * minimumBeta /
					(baseDriveReserve * requiredCurrent)
				derivation = "base-drive resistance follows available low-line voltage and reviewed minimum beta with current reserve"
			}
			return []AnalyticScale{{
				ID:         "topology:pass_device_drive:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    value,
				Unit:       "ohm",
				Derivation: derivation,
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
			inputNegative := betweenAll(output, terminals["IN_MINUS"])
			feedbackNegative := betweenAll(terminals["OUT"], terminals["IN_MINUS"])
			inputPositive := betweenAll(collector, terminals["IN_PLUS"])
			feedbackPositive := betweenAll(terminals["IN_PLUS"], reference)
			if len(inputNegative) == 0 || len(inputNegative) != len(inputPositive) ||
				len(feedbackNegative) == 0 || len(feedbackNegative) != len(feedbackPositive) {
				continue
			}
			network := map[string]bool{}
			for _, group := range [][]string{inputNegative, feedbackNegative, inputPositive, feedbackPositive} {
				for _, resistor := range group {
					network[resistor] = true
				}
			}
			if !network[instance.ID] {
				continue
			}
			const anchorResistance = 10_000.0
			value := anchorResistance
			derivation := "equal-ratio resistance for bounded differential current observation"
			if protectedSupply {
				if instance.ValueSI == nil || *instance.ValueSI <= 0 || len(shunts) == 0 {
					continue
				}
				value = *instance.ValueSI
				derivation = "preserve the matched catalog differential network selected with the bounded current shunt"
			}
			return []AnalyticScale{{
				ID:         "topology:differential_observation:" + instance.ID,
				Kind:       "resistance",
				ValueSI:    value,
				Unit:       "ohm",
				Derivation: derivation,
				SourceKind: "candidate_topology",
				SourceID:   active.ID,
				Priority:   1,
			}}
		}
	}
	if protectedPassSource != "" {
		return []AnalyticScale{{
			ID:         "topology:protected_current_control:" + instance.ID,
			Kind:       "resistance",
			ValueSI:    10_000,
			Unit:       "ohm",
			Derivation: "neutral control-interface resistance limits switch drive and keeps protection states defined",
			SourceKind: "candidate_topology",
			SourceID:   protectedPassSource,
			Priority:   1,
		}}
	}
	return nil
}

func deriveTransconductanceInputConditioningScale(
	requirement Requirement,
	graph CandidateGraph,
	instance GraphInstance,
) (AnalyticScale, bool) {
	if !slices.Contains([]string{"resistor", "capacitor"}, instance.Kind) ||
		len(instance.Terminals) != 2 ||
		requirementTransconductance(requirement) <= 0 {
		return AnalyticScale{}, false
	}
	nodeByID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	inputNode, conditionedNode, referenceNode := "", "", ""
	resistorID, capacitorID, controllerID := "", "", ""
	for _, capacitor := range graph.Instances {
		if capacitor.Kind != "capacitor" || len(capacitor.Terminals) != 2 {
			continue
		}
		left, right := capacitor.Terminals[0].Node, capacitor.Terminals[1].Node
		switch {
		case nodeByID[left].Role == "reference" && nodeByID[right].Scope == "internal":
			referenceNode, conditionedNode = left, right
		case nodeByID[right].Role == "reference" && nodeByID[left].Scope == "internal":
			referenceNode, conditionedNode = right, left
		default:
			continue
		}
		candidateInput, candidateResistor := "", ""
		for _, resistor := range graph.Instances {
			if resistor.Kind != "resistor" || len(resistor.Terminals) != 2 {
				continue
			}
			first, second := resistor.Terminals[0].Node, resistor.Terminals[1].Node
			switch {
			case first == conditionedNode && nodeByID[second].Scope == "external" && nodeByID[second].Role == "input":
				candidateInput, candidateResistor = second, resistor.ID
			case second == conditionedNode && nodeByID[first].Scope == "external" && nodeByID[first].Role == "input":
				candidateInput, candidateResistor = first, resistor.ID
			}
		}
		if candidateInput == "" || candidateResistor == "" {
			continue
		}
		candidateController := ""
		for _, controller := range graph.Instances {
			if controller.Kind != "opamp" {
				continue
			}
			terminals := topologyTerminalNodes(controller)
			if terminals["IN_PLUS"] == conditionedNode || terminals["IN_MINUS"] == conditionedNode {
				candidateController = controller.ID
				break
			}
		}
		if candidateController == "" {
			continue
		}
		inputNode, resistorID, capacitorID, controllerID =
			candidateInput, candidateResistor, capacitor.ID, candidateController
		break
	}
	if inputNode == "" || conditionedNode == "" || referenceNode == "" ||
		(instance.ID != resistorID && instance.ID != capacitorID) {
		return AnalyticScale{}, false
	}
	const resistance = 10_000.0
	settlingLimit := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "settling_time" || assertion.Excitation == nil ||
			assertion.Excitation.Kind != "port" ||
			assertion.Excitation.ID != nodeByID[inputNode].SemanticID || assertion.Max == nil ||
			*assertion.Max <= 0 {
			continue
		}
		if settlingLimit == 0 || *assertion.Max < settlingLimit {
			settlingLimit = *assertion.Max
		}
	}
	if settlingLimit == 0 {
		for _, operatingCase := range requirement.Requirements.OperatingCases {
			for _, event := range operatingCase.Events {
				if event.Target == nodeByID[inputNode].SemanticID && event.TriggerTimeS > 0 &&
					(settlingLimit == 0 || event.TriggerTimeS < settlingLimit) {
					settlingLimit = event.TriggerTimeS
				}
			}
		}
	}
	if settlingLimit <= 0 || !finite(settlingLimit) {
		settlingLimit = 1e-3
	}
	if instance.ID == resistorID {
		return AnalyticScale{
			ID:   "topology:conditioned_transconductance_input:" + instance.ID,
			Kind: "resistance", ValueSI: resistance, Unit: "ohm",
			Derivation: "neutral series input resistance forms a bounded command ramp without changing steady-state transconductance",
			SourceKind: "candidate_topology", SourceID: controllerID, Priority: 1,
		}, true
	}
	return AnalyticScale{
		ID:   "topology:conditioned_transconductance_input:" + instance.ID,
		Kind: "capacitance", ValueSI: settlingLimit / (5 * resistance), Unit: "F",
		Derivation: "five-time-constant command conditioning fits the tightest requested settling or startup-event interval",
		SourceKind: "candidate_topology", SourceID: controllerID, Priority: 1,
	}, true
}

func minimumTransconductanceSupplyVoltage(requirement Requirement) float64 {
	minimum := math.Inf(1)
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		if domain.MinVoltageV != nil && *domain.MinVoltageV > 0 {
			minimum = math.Min(minimum, *domain.MinVoltageV)
		} else if domain.NominalVoltageV != nil && *domain.NominalVoltageV > 0 {
			minimum = math.Min(minimum, *domain.NominalVoltageV)
		}
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "supply_voltage" && condition.Min > 0 {
				minimum = math.Min(minimum, condition.Min)
			}
		}
	}
	if math.IsInf(minimum, 1) {
		return 0
	}
	return minimum
}

func requiredTransconductanceOutputCurrent(requirement Requirement) float64 {
	maximum := 0.0
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Metric != "output_current" {
			continue
		}
		if assertion.Max != nil {
			maximum = math.Max(maximum, *assertion.Max)
		} else {
			maximum = math.Max(maximum, assertionTarget(assertion))
		}
	}
	if maximum > 0 {
		return maximum
	}
	for _, port := range requirement.Requirements.Ports {
		if port.Kind == "controlled_current" && port.Electrical.MaxCurrentA != nil {
			maximum = math.Max(maximum, *port.Electrical.MaxCurrentA)
		}
	}
	return maximum
}

func primitiveMinimumForwardBeta(primitive PrimitiveCandidate) float64 {
	minimum := math.Inf(1)
	for _, model := range primitive.Models {
		if model.ModelID != simmodel.PrimitiveBJTNPNV1 && model.ModelID != simmodel.PrimitiveBJTPNPV1 {
			continue
		}
		for _, parameter := range model.Parameters {
			if parameter.Name == "forward_beta" && parameter.Value > 0 {
				minimum = math.Min(minimum, parameter.Value)
			}
		}
		for _, uncertainty := range model.Uncertainties {
			if uncertainty.Target == "model_parameters.forward_beta" && uncertainty.Minimum > 0 {
				minimum = math.Min(minimum, uncertainty.Minimum)
			}
		}
	}
	if math.IsInf(minimum, 1) {
		return 0
	}
	return minimum
}

func topologyResistorPath(graph CandidateGraph, start, end string) []string {
	if start == "" || end == "" || start == end {
		return nil
	}
	type step struct {
		node      string
		instances []string
	}
	adjacency := map[string][]step{}
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || len(instance.Terminals) != 2 {
			continue
		}
		left, right := instance.Terminals[0].Node, instance.Terminals[1].Node
		adjacency[left] = append(adjacency[left], step{node: right, instances: []string{instance.ID}})
		adjacency[right] = append(adjacency[right], step{node: left, instances: []string{instance.ID}})
	}
	for node := range adjacency {
		slices.SortFunc(adjacency[node], func(left, right step) int {
			return cmp.Or(cmp.Compare(left.node, right.node), cmp.Compare(left.instances[0], right.instances[0]))
		})
	}
	queue := []step{{node: start}}
	visited := map[string]bool{start: true}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range adjacency[current.node] {
			if visited[edge.node] {
				continue
			}
			path := append(append([]string(nil), current.instances...), edge.instances[0])
			if edge.node == end {
				return path
			}
			visited[edge.node] = true
			queue = append(queue, step{node: edge.node, instances: path})
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
	requiredMaximumDelayS := math.Inf(1)
	requiredFrequencyHz := 0.0
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
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "propagation_delay":
			if assertion.Max != nil && *assertion.Max > 0 {
				requiredMaximumDelayS = math.Min(requiredMaximumDelayS, *assertion.Max)
			}
		case "oscillation_frequency":
			if assertion.Min != nil && *assertion.Min > 0 {
				requiredFrequencyHz = math.Max(requiredFrequencyHz, *assertion.Min)
			}
		}
		if assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0 {
			requiredFrequencyHz = math.Max(requiredFrequencyHz, *assertion.FrequencyHz)
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
		case "propagation_delay":
			delayS, found := timeBoundSeconds(limit, rating.Unit)
			if !found {
				continue
			}
			if delayS > requiredMaximumDelayS ||
				(requiredFrequencyHz > 0 && delayS > 0.5/requiredFrequencyHz) {
				return false
			}
		}
	}
	return true
}

func timeBoundSeconds(value float64, unit string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "s", "sec", "second", "seconds":
		return value, true
	case "ms":
		return value * 1e-3, true
	case "us", "µs", "μs":
		return value * 1e-6, true
	case "ns":
		return value * 1e-9, true
	default:
		return 0, false
	}
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
