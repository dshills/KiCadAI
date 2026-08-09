// Package capabilityfeedback turns authoritative synthesis and promotion
// evidence into identity-neutral, corpus-wide capability-expansion evidence.
package capabilityfeedback

import (
	"encoding/json"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/opentopologysynthesis"
)

const (
	CaseEvidenceSchema = "kicadai.closed-loop-capability-case.v1"
	AggregateSchema    = "kicadai.closed-loop-capability-report.v1"
	PolicyVersion      = "closed-loop-capability-policy-v1"
	RankingPolicy      = "case_count,domain_count,analysis_count,safety_score,reuse_score,capability,key"
)

type CorpusRole string

const (
	RoleDiscovery CorpusRole = "discovery"
	RoleHeldOut   CorpusRole = "held_out"
)

type Outcome string

const (
	OutcomePass        Outcome = "pass"
	OutcomeUnsupported Outcome = "unsupported"
	OutcomeUnsafe      Outcome = "unsafe"
	OutcomeExhausted   Outcome = "exhausted"
)

type GapScope string

const (
	ScopeTopology     GapScope = "topology"
	ScopeComponent    GapScope = "component"
	ScopeModel        GapScope = "model"
	ScopeSimulation   GapScope = "simulation"
	ScopePhysical     GapScope = "physical"
	ScopeRouting      GapScope = "routing"
	ScopeVerification GapScope = "verification"
)

type CaseMeta struct {
	ID           string                            `json:"id"`
	Role         CorpusRole                        `json:"role"`
	Domain       capabilityevaluation.Domain       `json:"domain"`
	SafetyImpact capabilityevaluation.SafetyImpact `json:"safety_impact"`
}

type Gap struct {
	Stage              string   `json:"stage"`
	Scope              GapScope `json:"scope"`
	Capability         string   `json:"capability"`
	Code               string   `json:"code"`
	RequirementIDs     []string `json:"requirement_ids,omitempty"`
	OperatingCases     []string `json:"operating_cases,omitempty"`
	AnalysisKinds      []string `json:"analysis_kinds,omitempty"`
	RequiredEvidence   []string `json:"required_evidence"`
	EvidenceHashes     []string `json:"evidence_hashes"`
	DownstreamSymptoms []string `json:"downstream_symptoms,omitempty"`
}

type CaseEvidence struct {
	Schema              string                            `json:"schema"`
	PolicyVersion       string                            `json:"policy_version"`
	Case                CaseMeta                          `json:"case"`
	Outcome             Outcome                           `json:"outcome"`
	StopReason          string                            `json:"stop_reason"`
	RequirementHash     string                            `json:"requirement_hash"`
	InventoryHash       string                            `json:"inventory_hash,omitempty"`
	CatalogHash         string                            `json:"catalog_hash,omitempty"`
	ModelRegistryHash   string                            `json:"model_registry_hash,omitempty"`
	SynthesisPolicyHash string                            `json:"synthesis_policy_hash"`
	SynthesisHash       string                            `json:"synthesis_hash"`
	PromotionHash       string                            `json:"promotion_hash,omitempty"`
	ProjectHash         string                            `json:"project_hash,omitempty"`
	AnalysisKinds       []string                          `json:"analysis_kinds"`
	Consumption         opentopologysynthesis.Consumption `json:"consumption"`
	Gaps                []Gap                             `json:"gaps,omitempty"`
	Hash                string                            `json:"hash"`
}

type Cluster struct {
	Rank             int      `json:"rank"`
	Key              string   `json:"key"`
	Outcome          Outcome  `json:"outcome"`
	Stage            string   `json:"stage"`
	Scope            GapScope `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	CaseCount        int      `json:"case_count"`
	DomainCount      int      `json:"domain_count"`
	AnalysisCount    int      `json:"analysis_count"`
	SafetyScore      int      `json:"safety_score"`
	ReuseScore       int      `json:"reuse_score"`
	Cases            []string `json:"cases"`
	Domains          []string `json:"domains"`
	AnalysisKinds    []string `json:"analysis_kinds"`
	DownstreamReuse  []string `json:"downstream_reuse,omitempty"`
	RequiredEvidence []string `json:"required_evidence"`
	EvidenceHashes   []string `json:"evidence_hashes"`
}

type AggregateReport struct {
	Schema             string         `json:"schema"`
	PolicyVersion      string         `json:"policy_version"`
	RankingPolicy      string         `json:"ranking_policy"`
	CorpusRole         CorpusRole     `json:"corpus_role"`
	ImpactRegistryHash string         `json:"impact_registry_hash"`
	CaseCount          int            `json:"case_count"`
	Cases              []CaseEvidence `json:"cases"`
	Clusters           []Cluster      `json:"clusters,omitempty"`
	Hash               string         `json:"hash"`
}

func (report AggregateReport) MarshalJSONStable() ([]byte, error) {
	return json.Marshal(report)
}
