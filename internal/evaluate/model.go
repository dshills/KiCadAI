package evaluate

import (
	"kicadai/internal/preservation"
	"kicadai/internal/reports"
)

type CheckStatus string

const (
	CheckPassed  CheckStatus = "passed"
	CheckFailed  CheckStatus = "failed"
	CheckSkipped CheckStatus = "skipped"
	CheckBlocked CheckStatus = "blocked"
)

type Report struct {
	Target                   string               `json:"target"`
	Checks                   []CheckResult        `json:"checks"`
	Issues                   []reports.Issue      `json:"issues"`
	FabricationReady         bool                 `json:"fabrication_ready"`
	FabricationReadyReason   string               `json:"fabrication_ready_reason,omitempty"`
	InspectionSummaryPresent bool                 `json:"inspection_summary_present"`
	Preservation             *preservation.Report `json:"preservation,omitempty"`
}

type CheckResult struct {
	Name      string             `json:"name"`
	Status    CheckStatus        `json:"status"`
	Required  bool               `json:"required"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
}
