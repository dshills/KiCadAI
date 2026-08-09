package capabilityfeedback

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityexpansion"
	"kicadai/internal/capabilitygate"
)

type clusterAccumulator struct {
	cluster    Cluster
	cases      map[string]bool
	domains    map[string]bool
	analyses   map[string]bool
	evidence   map[string]bool
	required   map[string]bool
	downstream map[string]bool
}

func Evaluate(
	role CorpusRole,
	cases []CaseEvidence,
	registry capabilityevaluation.ImpactRegistry,
) (AggregateReport, error) {
	if role != RoleDiscovery && role != RoleHeldOut {
		return AggregateReport{}, fmt.Errorf("unsupported corpus role %q", role)
	}
	normalizedRegistry, downstream, registryHash, err := normalizeImpactRegistry(registry)
	if err != nil {
		return AggregateReport{}, err
	}
	_ = normalizedRegistry
	normalizedCases := slices.Clone(cases)
	slices.SortFunc(normalizedCases, func(left, right CaseEvidence) int {
		return cmp.Compare(left.Case.ID, right.Case.ID)
	})
	for index := range normalizedCases {
		current := normalizedCases[index]
		if current.Case.Role != role {
			return AggregateReport{}, fmt.Errorf("case %q role %q does not match report role %q", current.Case.ID, current.Case.Role, role)
		}
		if index > 0 && normalizedCases[index-1].Case.ID == current.Case.ID {
			return AggregateReport{}, fmt.Errorf("duplicate case %q", current.Case.ID)
		}
		// Evaluate is also a public trust boundary for in-memory callers. Do not
		// assume a codec validated these cases: recomputing 24 small case hashes
		// prevents forged or drifted evidence from influencing rank selection.
		if err := ValidateCaseEvidence(current); err != nil {
			return AggregateReport{}, fmt.Errorf("case %q evidence is invalid or has drifted: %w", current.Case.ID, err)
		}
	}
	report := AggregateReport{
		Schema: AggregateSchema, PolicyVersion: PolicyVersion, RankingPolicy: RankingPolicy,
		CorpusRole: role, ImpactRegistryHash: registryHash, CaseCount: len(normalizedCases), Cases: normalizedCases,
	}
	if role == RoleHeldOut {
		hash, err := aggregateHash(report)
		if err != nil {
			return AggregateReport{}, err
		}
		report.Hash = hash
		return report, nil
	}

	clusters := map[string]*clusterAccumulator{}
	for _, currentCase := range normalizedCases {
		caseKeys := map[string]bool{}
		for _, gap := range currentCase.Gaps {
			key := clusterKey(gap, currentCase.Outcome)
			current := clusters[key]
			if current == nil {
				current = &clusterAccumulator{
					cluster: Cluster{Key: key, Outcome: currentCase.Outcome, Stage: gap.Stage, Scope: gap.Scope, Capability: gap.Capability, Code: gap.Code},
					cases:   map[string]bool{}, domains: map[string]bool{}, analyses: map[string]bool{},
					evidence: map[string]bool{}, required: map[string]bool{}, downstream: map[string]bool{},
				}
				for _, consumer := range downstream[gap.Capability] {
					current.downstream[consumer] = true
				}
				clusters[key] = current
			}
			if !caseKeys[key] {
				caseKeys[key] = true
				current.cases[currentCase.Case.ID] = true
				current.domains[string(currentCase.Case.Domain)] = true
				current.cluster.SafetyScore += safetyWeight(currentCase.Case.SafetyImpact)
				for _, analysis := range currentCase.AnalysisKinds {
					current.analyses[analysis] = true
				}
			}
			for _, analysis := range gap.AnalysisKinds {
				current.analyses[analysis] = true
			}
			for _, evidence := range gap.EvidenceHashes {
				current.evidence[evidence] = true
			}
			for _, required := range gap.RequiredEvidence {
				current.required[required] = true
			}
		}
	}
	for _, current := range clusters {
		current.cluster.Cases = sortedKeys(current.cases)
		current.cluster.Domains = sortedKeys(current.domains)
		current.cluster.AnalysisKinds = sortedKeys(current.analyses)
		current.cluster.EvidenceHashes = sortedKeys(current.evidence)
		current.cluster.RequiredEvidence = sortedKeys(current.required)
		current.cluster.DownstreamReuse = sortedKeys(current.downstream)
		current.cluster.CaseCount = len(current.cluster.Cases)
		current.cluster.DomainCount = len(current.cluster.Domains)
		current.cluster.AnalysisCount = len(current.cluster.AnalysisKinds)
		current.cluster.ReuseScore = len(current.cluster.DownstreamReuse)
		report.Clusters = append(report.Clusters, current.cluster)
	}
	slices.SortFunc(report.Clusters, compareClusters)
	for index := range report.Clusters {
		report.Clusters[index].Rank = index + 1
	}
	hash, err := aggregateHash(report)
	if err != nil {
		return AggregateReport{}, err
	}
	report.Hash = hash
	return report, nil
}

func BuildRankOneExpansionPlan(report AggregateReport) (capabilityexpansion.ExpansionPlan, error) {
	if report.Schema != AggregateSchema || report.PolicyVersion != PolicyVersion ||
		report.CorpusRole != RoleDiscovery || len(report.Clusters) == 0 || report.Clusters[0].Rank != 1 {
		return capabilityexpansion.ExpansionPlan{}, fmt.Errorf("rank-1 expansion requires a valid discovery report")
	}
	expected, err := aggregateHash(report)
	if err != nil || report.Hash != expected {
		return capabilityexpansion.ExpansionPlan{}, fmt.Errorf("discovery report hash is invalid")
	}
	selected := report.Clusters[0]
	evidenceID := "closed_loop_rank_one_gap"
	requirements := []capabilitygate.Requirement{{
		Kind: scopeRequirementKind(selected.Scope), ID: selected.Capability,
		Description: "rank-1 frozen open-set capability gap", EvidenceIDs: []string{evidenceID},
	}}
	for _, domain := range selected.Domains {
		requirements = append(requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementDomain, ID: domain,
			Description: "electrical domain affected by rank-1 gap", EvidenceIDs: []string{evidenceID},
		})
	}
	assessment, err := capabilitygate.Assess(capabilitygate.Input{
		Stage: "closed_loop_open_set_baseline", Requirements: requirements,
		Evidence: []capabilitygate.Evidence{{
			ID: evidenceID, Kind: "closed_loop_open_set_baseline", Status: capabilitygate.EvidenceMissing,
			Stage: selected.Stage, Description: "content-addressed rank-1 discovery cluster requires expansion evidence",
		}},
		Gaps: []capabilitygate.Gap{{
			Code: selected.Code, Kind: scopeRequirementKind(selected.Scope), ID: selected.Capability,
			Stage: selected.Stage, Reason: "rank-1 frozen discovery cluster blocks multiple behavior-only cases",
			Action: strings.Join(selected.RequiredEvidence, "; "),
		}},
		Risks: []capabilitygate.Risk{{
			Code: "CLOSED_LOOP_EXPANSION_QUARANTINE", Stage: selected.Stage,
			Summary:    "candidate capability must remain experimental until full reviewed promotion",
			Mitigation: "use the existing source-backed capability-expansion and exact-hash approval path",
		}},
	})
	if err != nil {
		return capabilityexpansion.ExpansionPlan{}, fmt.Errorf("build rank-1 capability assessment: %w", err)
	}
	return capabilityexpansion.Plan(assessment)
}

func compareClusters(left, right Cluster) int {
	return cmp.Or(
		cmp.Compare(right.CaseCount, left.CaseCount),
		cmp.Compare(right.DomainCount, left.DomainCount),
		cmp.Compare(right.AnalysisCount, left.AnalysisCount),
		cmp.Compare(right.SafetyScore, left.SafetyScore),
		cmp.Compare(right.ReuseScore, left.ReuseScore),
		cmp.Compare(left.Capability, right.Capability),
		cmp.Compare(left.Key, right.Key),
	)
}

func normalizeImpactRegistry(registry capabilityevaluation.ImpactRegistry) (capabilityevaluation.ImpactRegistry, map[string][]string, string, error) {
	if strings.TrimSpace(registry.Version) == "" {
		return capabilityevaluation.ImpactRegistry{}, nil, "", fmt.Errorf("impact registry version is required")
	}
	normalized := capabilityevaluation.ImpactRegistry{Version: strings.TrimSpace(registry.Version)}
	graph := map[string][]string{}
	for _, record := range registry.Records {
		capability := canonicalID(record.Capability)
		if capability == "" || capability != record.Capability {
			return capabilityevaluation.ImpactRegistry{}, nil, "", fmt.Errorf("impact capability %q is not canonical", record.Capability)
		}
		if _, duplicate := graph[capability]; duplicate {
			return capabilityevaluation.ImpactRegistry{}, nil, "", fmt.Errorf("duplicate impact capability %q", capability)
		}
		consumers := normalizedStrings(record.Consumers)
		for _, consumer := range consumers {
			if canonicalID(consumer) != consumer || consumer == capability {
				return capabilityevaluation.ImpactRegistry{}, nil, "", fmt.Errorf("invalid downstream consumer %q for %q", consumer, capability)
			}
		}
		graph[capability] = consumers
		normalized.Records = append(normalized.Records, capabilityevaluation.ImpactRecord{Capability: capability, Consumers: consumers})
	}
	slices.SortFunc(normalized.Records, func(left, right capabilityevaluation.ImpactRecord) int {
		return cmp.Compare(left.Capability, right.Capability)
	})
	if err := validateAcyclicImpact(graph); err != nil {
		return capabilityevaluation.ImpactRegistry{}, nil, "", err
	}
	downstream := map[string][]string{}
	for capability := range graph {
		seen := map[string]bool{}
		collectDownstream(capability, graph, seen)
		downstream[capability] = sortedKeys(seen)
	}
	hash, err := digest(normalized)
	return normalized, downstream, hash, err
}

func validateAcyclicImpact(graph map[string][]string) error {
	const visiting, visited = 1, 2
	state := map[string]int{}
	var visit func(string) error
	visit = func(node string) error {
		if state[node] == visiting {
			return fmt.Errorf("impact registry contains a cycle at %q", node)
		}
		if state[node] == visited {
			return nil
		}
		state[node] = visiting
		for _, consumer := range graph[node] {
			if err := visit(consumer); err != nil {
				return err
			}
		}
		state[node] = visited
		return nil
	}
	for _, node := range sortedKeysFromSlices(graph) {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}

func collectDownstream(capability string, graph map[string][]string, seen map[string]bool) {
	for _, consumer := range graph[capability] {
		if seen[consumer] {
			continue
		}
		seen[consumer] = true
		collectDownstream(consumer, graph, seen)
	}
}

func safetyWeight(value capabilityevaluation.SafetyImpact) int {
	switch value {
	case capabilityevaluation.SafetyReviewRequired:
		return 1
	case capabilityevaluation.SafetyRelevant:
		return 3
	case capabilityevaluation.SafetyCritical:
		return 5
	default:
		return 0
	}
}

func scopeRequirementKind(scope GapScope) capabilitygate.RequirementKind {
	switch scope {
	case ScopeTopology:
		return capabilitygate.RequirementArchitecture
	case ScopeComponent:
		return capabilitygate.RequirementComponent
	case ScopeModel:
		return capabilitygate.RequirementModel
	case ScopePhysical, ScopeRouting:
		return capabilitygate.RequirementPhysical
	case ScopeSimulation, ScopeVerification:
		return capabilitygate.RequirementVerification
	default:
		return capabilitygate.RequirementVerification
	}
}

func clusterKey(gap Gap, outcome Outcome) string {
	return strings.Join([]string{string(outcome), gap.Stage, string(gap.Scope), gap.Capability, gap.Code}, ":")
}

func aggregateHash(report AggregateReport) (string, error) {
	hashless := report
	hashless.Hash = ""
	return digest(hashless)
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func sortedKeysFromSlices(values map[string][]string) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
