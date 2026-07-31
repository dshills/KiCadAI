package opentopologysynthesis

import "kicadai/internal/reports"

const (
	RepairSearchSchema  = "kicadai.open-topology-repair-search.v1"
	RepairSearchVersion = 1
)

type RepairSearchStatus string

const (
	RepairSearchPassed      RepairSearchStatus = "passed"
	RepairSearchExhausted   RepairSearchStatus = "exhausted"
	RepairSearchUnsupported RepairSearchStatus = "unsupported"
	RepairSearchCanceled    RepairSearchStatus = "canceled"
	RepairSearchFailed      RepairSearchStatus = "failed"
)

type RepairSearchResult struct {
	Schema                string             `json:"schema"`
	Version               int                `json:"version"`
	PolicyVersion         string             `json:"policy_version"`
	RequirementHash       string             `json:"requirement_hash"`
	InventoryHash         string             `json:"inventory_hash"`
	InitialGraphHash      string             `json:"initial_graph_hash"`
	InitialEvaluationHash string             `json:"initial_evaluation_hash"`
	Status                RepairSearchStatus `json:"status"`
	Policy                Policy             `json:"policy"`
	Consumption           Consumption        `json:"consumption"`
	Attempts              []RepairAttempt    `json:"attempts"`
	Selected              *RepairedCandidate `json:"selected,omitempty"`
	Issues                []reports.Issue    `json:"issues"`
	Hash                  string             `json:"hash"`
}

type RepairAttempt struct {
	Number       int                  `json:"number"`
	Repair       Repair               `json:"repair"`
	ValueTrial   *ValueTrial          `json:"value_trial,omitempty"`
	GraphHash    string               `json:"graph_hash"`
	TopologyHash string               `json:"topology_hash"`
	Evaluation   SimulationEvaluation `json:"evaluation"`
	Improved     bool                 `json:"improved"`
	Status       RepairSearchStatus   `json:"status"`
}

type RepairedCandidate struct {
	Graph      CandidateGraph       `json:"graph"`
	Repair     Repair               `json:"repair"`
	Repairs    []Repair             `json:"repairs"`
	ValueTrial *ValueTrial          `json:"value_trial,omitempty"`
	Evaluation SimulationEvaluation `json:"evaluation"`
}
