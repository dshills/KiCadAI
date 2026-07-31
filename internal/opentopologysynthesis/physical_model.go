package opentopologysynthesis

import (
	"kicadai/internal/circuitgraph"
	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

const (
	PhysicalLoweringSchema  = "kicadai.open-topology-physical-lowering.v1"
	PhysicalLoweringVersion = 1
)

type PhysicalLoweringStatus string

const (
	PhysicalLoweringReady       PhysicalLoweringStatus = "ready"
	PhysicalLoweringInvalid     PhysicalLoweringStatus = "invalid"
	PhysicalLoweringUnsupported PhysicalLoweringStatus = "unsupported"
)

type PhysicalLoweringResult struct {
	Schema          string                        `json:"schema"`
	Version         int                           `json:"version"`
	PolicyVersion   string                        `json:"policy_version"`
	RequirementHash string                        `json:"requirement_hash"`
	InventoryHash   string                        `json:"inventory_hash"`
	GraphHash       string                        `json:"graph_hash"`
	EvaluationHash  string                        `json:"evaluation_hash"`
	Status          PhysicalLoweringStatus        `json:"status"`
	Document        circuitgraph.Document         `json:"document"`
	Resolved        circuitgraph.ResolvedDocument `json:"resolved"`
	DesignRequest   designworkflow.Request        `json:"design_request"`
	Bindings        []PhysicalSemanticBinding     `json:"bindings"`
	Issues          []reports.Issue               `json:"issues"`
	Hash            string                        `json:"hash"`
}

type PhysicalSemanticBinding struct {
	Kind        string `json:"kind"`
	SemanticID  string `json:"semantic_id"`
	GraphNode   string `json:"graph_node"`
	Component   string `json:"component,omitempty"`
	Function    string `json:"function,omitempty"`
	CatalogID   string `json:"catalog_id,omitempty"`
	VariantID   string `json:"variant_id,omitempty"`
	EvidenceSHA string `json:"evidence_sha256"`
}
