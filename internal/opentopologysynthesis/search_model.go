package opentopologysynthesis

import "kicadai/internal/reports"

const (
	TopologySearchSchema  = "kicadai.open-topology-search-result.v1"
	TopologySearchVersion = 1
)

type TopologySearchStatus string

const (
	TopologySearchCandidates  TopologySearchStatus = "candidates_found"
	TopologySearchUnsupported TopologySearchStatus = "unsupported"
	TopologySearchExhausted   TopologySearchStatus = "exhausted"
	TopologySearchCanceled    TopologySearchStatus = "canceled"
	TopologySearchFailed      TopologySearchStatus = "failed"
)

type TopologySearchResult struct {
	Schema          string               `json:"schema"`
	Version         int                  `json:"version"`
	PolicyVersion   string               `json:"policy_version"`
	RequirementHash string               `json:"requirement_hash"`
	InventoryHash   string               `json:"inventory_hash"`
	Policy          Policy               `json:"policy"`
	Status          TopologySearchStatus `json:"status"`
	Consumption     Consumption          `json:"consumption"`
	Candidates      []TopologyCandidate  `json:"candidates"`
	Rejections      []SearchRejection    `json:"rejections"`
	Issues          []reports.Issue      `json:"issues"`
}

type TopologyCandidate struct {
	Fingerprint  string           `json:"fingerprint"`
	TopologyHash string           `json:"topology_hash"`
	Repairable   bool             `json:"repairable,omitempty"`
	Score        TopologyScore    `json:"score"`
	Graph        CandidateGraph   `json:"graph"`
	Operations   []GraphOperation `json:"operations"`
}

type TopologyScore struct {
	UnconnectedExternal int     `json:"unconnected_external"`
	UnreachableOutputs  int     `json:"unreachable_outputs"`
	DanglingInternal    int     `json:"dangling_internal"`
	BehaviorGap         int     `json:"behavior_gap"`
	RedundantActive     int     `json:"redundant_active"`
	EndpointAccess      int     `json:"endpoint_access"`
	PrimitiveCount      int     `json:"primitive_count"`
	InternalNodeCount   int     `json:"internal_node_count"`
	EvidencePenalty     int     `json:"evidence_penalty"`
	AreaMM2             float64 `json:"area_mm2"`
	Fingerprint         string  `json:"fingerprint"`
}

type GraphOperation struct {
	Number        int                  `json:"number"`
	Kind          string               `json:"kind"`
	PrimitiveKey  string               `json:"primitive_key,omitempty"`
	PrimitiveKind string               `json:"primitive_kind,omitempty"`
	Node          string               `json:"node,omitempty"`
	Connections   []TerminalConnection `json:"connections,omitempty"`
	ValueSI       *float64             `json:"value_si,omitempty"`
	BeforeHash    string               `json:"before_hash"`
	AfterHash     string               `json:"after_hash"`
}

type SearchRejection struct {
	Code    string   `json:"code"`
	Count   int      `json:"count"`
	Samples []string `json:"samples"`
}
