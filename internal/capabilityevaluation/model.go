// Package capabilityevaluation aggregates terminal generation evidence into
// fixture-neutral, deterministically ranked reusable capability gaps.
package capabilityevaluation

import "encoding/json"

const (
	ReportSchema         = "kicadai.open-world-capability-evaluation.v1"
	DefaultPolicyVersion = "open-world-ranking-policy-v1"
)

type Outcome string

const (
	OutcomeReady              Outcome = "ready"
	OutcomeNeedsClarification Outcome = "needs_clarification"
	OutcomeUnsupported        Outcome = "unsupported"
	OutcomeAmbiguous          Outcome = "ambiguous"
	OutcomeBudgetExhausted    Outcome = "budget_exhausted"
)

type SafetyImpact string

const (
	SafetyNonSafety      SafetyImpact = "non_safety"
	SafetyReviewRequired SafetyImpact = "review_required"
	SafetyRelevant       SafetyImpact = "safety_relevant"
	SafetyCritical       SafetyImpact = "safety_critical"
)

type Domain string

const (
	DomainAnalog      Domain = "analog"
	DomainPower       Domain = "power"
	DomainMCU         Domain = "mcu"
	DomainSensor      Domain = "sensor"
	DomainDigital     Domain = "digital"
	DomainMixedSignal Domain = "mixed_signal"
)

type Observation struct {
	Capability       string   `json:"capability"`
	Outcome          Outcome  `json:"outcome"`
	Stage            string   `json:"stage"`
	Code             string   `json:"code"`
	Path             string   `json:"path"`
	Reason           string   `json:"reason"`
	RequiredEvidence []string `json:"required_evidence"`
}

type CaseResult struct {
	ID           string        `json:"id"`
	Domain       Domain        `json:"domain"`
	SafetyImpact SafetyImpact  `json:"safety_impact"`
	Outcome      Outcome       `json:"outcome"`
	Observations []Observation `json:"observations"`
}

// ImpactRecord is reviewed product evidence. Consumers are semantic
// capabilities that become easier to support when Capability is implemented.
// Corpus cases and providers never supply this relationship.
type ImpactRecord struct {
	Capability string   `json:"capability"`
	Consumers  []string `json:"consumers"`
}

type ImpactRegistry struct {
	Version string         `json:"version"`
	Records []ImpactRecord `json:"records"`
}

type RankingPolicy struct {
	Version string `json:"version"`
}

func DefaultRankingPolicy() RankingPolicy {
	return RankingPolicy{Version: DefaultPolicyVersion}
}

type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type DomainOutcomeCount struct {
	Domain  Domain  `json:"domain"`
	Outcome Outcome `json:"outcome"`
	Count   int     `json:"count"`
}

type Cluster struct {
	Rank             int      `json:"rank"`
	Key              string   `json:"key"`
	Outcome          Outcome  `json:"outcome"`
	Stage            string   `json:"stage"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	FrequencyScore   int      `json:"frequency_score"`
	SafetyScore      int      `json:"safety_score"`
	ReuseScore       int      `json:"reuse_score"`
	DomainCount      int      `json:"domain_count"`
	Cases            []string `json:"cases"`
	Domains          []Domain `json:"domains"`
	DownstreamReuse  []string `json:"downstream_reuse"`
	RequiredEvidence []string `json:"required_evidence"`
}

type Report struct {
	Schema              string               `json:"schema"`
	PolicyVersion       string               `json:"policy_version"`
	CorpusRole          CorpusRole           `json:"corpus_role,omitempty"`
	CorpusSHA256        string               `json:"corpus_sha256,omitempty"`
	RegistryVersion     string               `json:"registry_version"`
	RegistrySHA256      string               `json:"registry_sha256"`
	CaseCount           int                  `json:"case_count"`
	OutcomeCounts       []Count              `json:"outcome_counts"`
	DomainOutcomeCounts []DomainOutcomeCount `json:"domain_outcome_counts"`
	Cases               []CaseResult         `json:"cases"`
	RankedClusters      []Cluster            `json:"ranked_failure_clusters"`
}

func (report Report) MarshalJSONStable() ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
