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
	Placement       []PhysicalPlacementEvidence   `json:"placement_evidence,omitempty"`
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

type PhysicalPlacementEvidence struct {
	Kind                string               `json:"kind"`
	Component           string               `json:"component,omitempty"`
	Region              string               `json:"region"`
	Role                string               `json:"role"`
	Bounds              *circuitgraph.Bounds `json:"bounds,omitempty"`
	Members             []string             `json:"members,omitempty"`
	Edge                circuitgraph.Side    `json:"edge,omitempty"`
	CatalogID           string               `json:"catalog_id,omitempty"`
	VariantID           string               `json:"variant_id,omitempty"`
	PackageType         string               `json:"package_type,omitempty"`
	ThermalPathID       string               `json:"thermal_path_id,omitempty"`
	ThermalPathCPerW    float64              `json:"thermal_path_c_per_w,omitempty"`
	KeepAwayRole        string               `json:"keep_away_role,omitempty"`
	MinimumClearanceMM  float64              `json:"minimum_clearance_mm,omitempty"`
	BoardEdgeRequired   bool                 `json:"board_edge_required,omitempty"`
	PreferThermalCopper bool                 `json:"prefer_thermal_copper,omitempty"`
	Rationale           string               `json:"rationale"`
	EvidenceSHA         string               `json:"evidence_sha256"`
}
