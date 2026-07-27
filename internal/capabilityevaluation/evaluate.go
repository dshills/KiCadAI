package capabilityevaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	semanticIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	codePattern       = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*$`)
)

type clusterAccumulator struct {
	cluster        Cluster
	cases          map[string]bool
	domains        map[Domain]bool
	evidence       map[string]bool
	downstream     map[string]bool
	caseSafetySeen map[string]bool
}

func Evaluate(cases []CaseResult, registry ImpactRegistry, policy RankingPolicy) (Report, error) {
	normalizedRegistry, downstream, registryHash, err := normalizeRegistry(registry)
	if err != nil {
		return Report{}, err
	}
	if policy.Version != DefaultPolicyVersion {
		return Report{}, fmt.Errorf("ranking policy version %q is unsupported", policy.Version)
	}
	normalizedCases, err := normalizeCases(cases)
	if err != nil {
		return Report{}, err
	}

	outcomeCounts := map[Outcome]int{}
	domainOutcomeCounts := map[string]int{}
	clusters := map[string]*clusterAccumulator{}
	for _, currentCase := range normalizedCases {
		outcomeCounts[currentCase.Outcome]++
		domainOutcomeCounts[string(currentCase.Domain)+"\x00"+string(currentCase.Outcome)]++
		for _, observation := range currentCase.Observations {
			key := clusterKey(observation)
			current := clusters[key]
			if current == nil {
				current = &clusterAccumulator{
					cluster: Cluster{
						Key: key, Outcome: observation.Outcome, Stage: observation.Stage,
						Capability: observation.Capability, Code: observation.Code,
					},
					cases:          map[string]bool{},
					domains:        map[Domain]bool{},
					evidence:       map[string]bool{},
					downstream:     map[string]bool{},
					caseSafetySeen: map[string]bool{},
				}
				for _, consumer := range downstream[observation.Capability] {
					current.downstream[consumer] = true
				}
				clusters[key] = current
			}
			if !current.cases[currentCase.ID] {
				current.cases[currentCase.ID] = true
				current.cluster.FrequencyScore++
			}
			if !current.caseSafetySeen[currentCase.ID] {
				current.caseSafetySeen[currentCase.ID] = true
				current.cluster.SafetyScore += safetyWeight(currentCase.SafetyImpact)
			}
			current.domains[currentCase.Domain] = true
			for _, evidence := range observation.RequiredEvidence {
				current.evidence[evidence] = true
			}
		}
	}

	report := Report{
		Schema: ReportSchema, PolicyVersion: policy.Version,
		RegistryVersion: normalizedRegistry.Version, RegistrySHA256: registryHash,
		CaseCount: len(normalizedCases), Cases: normalizedCases,
	}
	for _, outcome := range allOutcomes() {
		report.OutcomeCounts = append(report.OutcomeCounts, Count{Key: string(outcome), Count: outcomeCounts[outcome]})
	}
	for _, domain := range allDomains() {
		for _, outcome := range allOutcomes() {
			key := string(domain) + "\x00" + string(outcome)
			report.DomainOutcomeCounts = append(report.DomainOutcomeCounts, DomainOutcomeCount{
				Domain: domain, Outcome: outcome, Count: domainOutcomeCounts[key],
			})
		}
	}
	for _, current := range clusters {
		current.cluster.Cases = sortedStringKeys(current.cases)
		current.cluster.Domains = sortedDomainKeys(current.domains)
		current.cluster.DomainCount = len(current.cluster.Domains)
		current.cluster.RequiredEvidence = sortedStringKeys(current.evidence)
		current.cluster.DownstreamReuse = sortedStringKeys(current.downstream)
		current.cluster.ReuseScore = len(current.cluster.DownstreamReuse)
		report.RankedClusters = append(report.RankedClusters, current.cluster)
	}
	slices.SortStableFunc(report.RankedClusters, compareClusters)
	for index := range report.RankedClusters {
		report.RankedClusters[index].Rank = index + 1
	}
	return report, nil
}

// EvaluateCorpus verifies that terminal evidence corresponds exactly to a
// frozen corpus before ranking it. The prompt text is used only by the normal
// compiler/provider path that produced cases; clustering never inspects it.
func EvaluateCorpus(corpus Corpus, cases []CaseResult, registry ImpactRegistry, policy RankingPolicy) (Report, error) {
	if err := ValidateCorpus(corpus); err != nil {
		return Report{}, err
	}
	if len(cases) != len(corpus.Cases) {
		return Report{}, fmt.Errorf("terminal evidence case count = %d, corpus case count = %d", len(cases), len(corpus.Cases))
	}
	byID := make(map[string]CaseResult, len(cases))
	for _, current := range cases {
		if _, exists := byID[current.ID]; exists {
			return Report{}, fmt.Errorf("terminal evidence case id %q is duplicated", current.ID)
		}
		byID[current.ID] = current
	}
	for _, expected := range corpus.Cases {
		actual, exists := byID[expected.ID]
		if !exists {
			return Report{}, fmt.Errorf("terminal evidence is missing corpus case %q", expected.ID)
		}
		if actual.Domain != expected.Domain || actual.SafetyImpact != expected.SafetyImpact {
			return Report{}, fmt.Errorf(
				"terminal evidence metadata for %q = %q/%q, want %q/%q",
				expected.ID, actual.Domain, actual.SafetyImpact, expected.Domain, expected.SafetyImpact,
			)
		}
	}
	corpusHash, err := CorpusSHA256(corpus)
	if err != nil {
		return Report{}, err
	}
	report, err := Evaluate(cases, registry, policy)
	if err != nil {
		return Report{}, err
	}
	report.CorpusRole = corpus.Role
	report.CorpusSHA256 = corpusHash
	return report, nil
}

func normalizeCases(values []CaseResult) ([]CaseResult, error) {
	result := slices.Clone(values)
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
	}
	slices.SortFunc(result, func(left, right CaseResult) int { return strings.Compare(left.ID, right.ID) })
	for index := range result {
		current := &result[index]
		if !semanticIDPattern.MatchString(current.ID) {
			return nil, fmt.Errorf("case %d has invalid semantic id %q", index, current.ID)
		}
		if index > 0 && result[index-1].ID == current.ID {
			return nil, fmt.Errorf("case id %q is duplicated", current.ID)
		}
		if !validDomain(current.Domain) {
			return nil, fmt.Errorf("case %q has invalid domain %q", current.ID, current.Domain)
		}
		if !validSafetyImpact(current.SafetyImpact) {
			return nil, fmt.Errorf("case %q has invalid safety impact %q", current.ID, current.SafetyImpact)
		}
		if !validOutcome(current.Outcome) {
			return nil, fmt.Errorf("case %q has invalid terminal outcome %q", current.ID, current.Outcome)
		}
		if current.Outcome == OutcomeReady && len(current.Observations) != 0 {
			return nil, fmt.Errorf("ready case %q cannot contain blocking observations", current.ID)
		}
		if current.Outcome != OutcomeReady && len(current.Observations) == 0 {
			return nil, fmt.Errorf("non-ready case %q requires a blocking observation", current.ID)
		}
		observations, err := normalizeObservations(current.ID, current.Outcome, current.Observations)
		if err != nil {
			return nil, err
		}
		current.Observations = observations
	}
	return result, nil
}

func normalizeObservations(caseID string, outcome Outcome, values []Observation) ([]Observation, error) {
	byKey := map[string]Observation{}
	for index, value := range values {
		value.Capability = strings.TrimSpace(value.Capability)
		value.Stage = strings.TrimSpace(value.Stage)
		value.Code = strings.TrimSpace(value.Code)
		value.Path = strings.TrimSpace(value.Path)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.Outcome != outcome {
			return nil, fmt.Errorf("case %q observation %d outcome %q does not match terminal outcome %q", caseID, index, value.Outcome, outcome)
		}
		if !semanticIDPattern.MatchString(value.Capability) || !semanticIDPattern.MatchString(value.Stage) {
			return nil, fmt.Errorf("case %q observation %d has invalid capability or stage", caseID, index)
		}
		if !codePattern.MatchString(value.Code) {
			return nil, fmt.Errorf("case %q observation %d has invalid issue code %q", caseID, index, value.Code)
		}
		if value.Path == "" || value.Reason == "" {
			return nil, fmt.Errorf("case %q observation %d requires path and reason", caseID, index)
		}
		value.RequiredEvidence = normalizeNonEmptyStrings(value.RequiredEvidence)
		if len(value.RequiredEvidence) == 0 {
			return nil, fmt.Errorf("case %q observation %d requires evidence needed to close the gap", caseID, index)
		}
		key := clusterKey(value)
		if prior, ok := byKey[key]; ok {
			prior.RequiredEvidence = normalizeNonEmptyStrings(append(prior.RequiredEvidence, value.RequiredEvidence...))
			if value.Path < prior.Path {
				prior.Path = value.Path
			}
			if value.Reason < prior.Reason {
				prior.Reason = value.Reason
			}
			byKey[key] = prior
		} else {
			byKey[key] = value
		}
	}
	result := make([]Observation, 0, len(byKey))
	for _, value := range byKey {
		result = append(result, value)
	}
	slices.SortFunc(result, func(left, right Observation) int {
		return strings.Compare(clusterKey(left), clusterKey(right))
	})
	return result, nil
}

func normalizeRegistry(registry ImpactRegistry) (ImpactRegistry, map[string][]string, string, error) {
	if !semanticIDPattern.MatchString(registry.Version) {
		return ImpactRegistry{}, nil, "", fmt.Errorf("impact registry version %q is invalid", registry.Version)
	}
	normalized := ImpactRegistry{Version: registry.Version, Records: slices.Clone(registry.Records)}
	for index := range normalized.Records {
		record := &normalized.Records[index]
		record.Capability = strings.TrimSpace(record.Capability)
		record.Consumers = normalizeNonEmptyStrings(record.Consumers)
	}
	slices.SortFunc(normalized.Records, func(left, right ImpactRecord) int {
		return strings.Compare(left.Capability, right.Capability)
	})
	graph := map[string][]string{}
	for index := range normalized.Records {
		record := &normalized.Records[index]
		if !semanticIDPattern.MatchString(record.Capability) {
			return ImpactRegistry{}, nil, "", fmt.Errorf("impact record %d has invalid capability %q", index, record.Capability)
		}
		if index > 0 && normalized.Records[index-1].Capability == record.Capability {
			return ImpactRegistry{}, nil, "", fmt.Errorf("impact capability %q is duplicated", record.Capability)
		}
		for _, consumer := range record.Consumers {
			if !semanticIDPattern.MatchString(consumer) {
				return ImpactRegistry{}, nil, "", fmt.Errorf("impact capability %q has invalid consumer %q", record.Capability, consumer)
			}
			if consumer == record.Capability {
				return ImpactRegistry{}, nil, "", fmt.Errorf("impact capability %q cannot consume itself", record.Capability)
			}
		}
		graph[record.Capability] = record.Consumers
	}
	if err := validateAcyclic(graph); err != nil {
		return ImpactRegistry{}, nil, "", err
	}
	closure := map[string][]string{}
	for capability := range graph {
		seen := map[string]bool{}
		collectConsumers(capability, graph, seen)
		delete(seen, capability)
		closure[capability] = sortedStringKeys(seen)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return ImpactRegistry{}, nil, "", fmt.Errorf("marshal impact registry: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return normalized, closure, hex.EncodeToString(sum[:]), nil
}

func validateAcyclic(graph map[string][]string) error {
	const (
		unseen = iota
		visiting
		visited
	)
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
	keys := make([]string, 0, len(graph))
	for key := range graph {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}

func collectConsumers(capability string, graph map[string][]string, seen map[string]bool) {
	for _, consumer := range graph[capability] {
		if seen[consumer] {
			continue
		}
		seen[consumer] = true
		collectConsumers(consumer, graph, seen)
	}
}

func compareClusters(left, right Cluster) int {
	if left.FrequencyScore != right.FrequencyScore {
		return right.FrequencyScore - left.FrequencyScore
	}
	if left.SafetyScore != right.SafetyScore {
		return right.SafetyScore - left.SafetyScore
	}
	if left.ReuseScore != right.ReuseScore {
		return right.ReuseScore - left.ReuseScore
	}
	if left.DomainCount != right.DomainCount {
		return right.DomainCount - left.DomainCount
	}
	if order := strings.Compare(left.Capability, right.Capability); order != 0 {
		return order
	}
	return strings.Compare(left.Key, right.Key)
}

func clusterKey(value Observation) string {
	return string(value.Outcome) + ":" + value.Stage + ":" + value.Capability + ":" + value.Code
}

func safetyWeight(value SafetyImpact) int {
	switch value {
	case SafetyNonSafety:
		return 0
	case SafetyReviewRequired:
		return 1
	case SafetyRelevant:
		return 3
	case SafetyCritical:
		return 5
	default:
		return 0
	}
}

func validOutcome(value Outcome) bool {
	return slices.Contains(allOutcomes(), value)
}

func validDomain(value Domain) bool {
	return slices.Contains(allDomains(), value)
}

func validSafetyImpact(value SafetyImpact) bool {
	return value == SafetyNonSafety || value == SafetyReviewRequired || value == SafetyRelevant || value == SafetyCritical
}

func allOutcomes() []Outcome {
	return []Outcome{OutcomeReady, OutcomeNeedsClarification, OutcomeUnsupported, OutcomeAmbiguous, OutcomeBudgetExhausted}
}

func allDomains() []Domain {
	return []Domain{DomainAnalog, DomainPower, DomainMCU, DomainSensor, DomainDigital, DomainMixedSignal}
}

func normalizeNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func sortedStringKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func sortedDomainKeys(values map[Domain]bool) []Domain {
	result := make([]Domain, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
