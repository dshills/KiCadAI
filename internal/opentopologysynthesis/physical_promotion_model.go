package opentopologysynthesis

import (
	"time"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

const (
	PhysicalPromotionSchema  = "kicadai.open-topology-physical-promotion.v1"
	PhysicalPromotionVersion = 1
)

type PhysicalPromotionStatus string

const (
	PhysicalPromotionPassed  PhysicalPromotionStatus = "passed"
	PhysicalPromotionFailed  PhysicalPromotionStatus = "failed"
	PhysicalPromotionInvalid PhysicalPromotionStatus = "invalid"
)

// PhysicalPromotionOptions supplies the external production context that is
// intentionally absent from the pure topology and simulation search.
type PhysicalPromotionOptions struct {
	OutputRoot    string
	Overwrite     bool
	KiCadCLI      string
	LibraryIndex  *libraryresolver.LibraryIndex
	Timeout       time.Duration
	KeepArtifacts bool
}

type PhysicalPromotionResult struct {
	Schema          string                  `json:"schema"`
	Version         int                     `json:"version"`
	PolicyVersion   string                  `json:"policy_version"`
	RequirementHash string                  `json:"requirement_hash"`
	InventoryHash   string                  `json:"inventory_hash"`
	SynthesisHash   string                  `json:"synthesis_hash"`
	PhysicalHash    string                  `json:"physical_hash"`
	Status          PhysicalPromotionStatus `json:"status"`
	ReplayIdentical bool                    `json:"replay_identical"`
	ProjectHash     string                  `json:"project_hash,omitempty"`
	Runs            []PhysicalPromotionRun  `json:"runs"`
	Issues          []reports.Issue         `json:"issues"`
	Hash            string                  `json:"hash"`
}

type PhysicalPromotionRun struct {
	Number      int                           `json:"number"`
	ProjectRoot string                        `json:"project_root"`
	ProjectHash string                        `json:"project_hash,omitempty"`
	Workflow    designworkflow.WorkflowResult `json:"workflow"`
	Artifacts   []reports.Artifact            `json:"artifacts"`
}
