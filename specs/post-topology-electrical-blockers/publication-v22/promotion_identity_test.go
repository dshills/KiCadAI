// This read-only projection mirrors the sealed physical promotion identity.
// Machine-local paths and messages are deliberately excluded, just as they are
// in internal/opentopologysynthesis/physical_promotion.go at the freeze commit.
package v22publication

import (
	"slices"

	"kicadai/internal/designworkflow"
	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

func promotionIdentity(result ot.PhysicalPromotionResult) any {
	type issueEvidence struct {
		Code     reports.Code     `json:"code"`
		Severity reports.Severity `json:"severity"`
		Stage    string           `json:"stage,omitempty"`
	}
	type stageEvidence struct {
		Name      designworkflow.StageName   `json:"name"`
		Status    designworkflow.StageStatus `json:"status"`
		Issues    []issueEvidence            `json:"issues"`
		Artifacts []reports.ArtifactKind     `json:"artifacts"`
	}
	type runEvidence struct {
		Number      int                             `json:"number"`
		ProjectHash string                          `json:"project_hash,omitempty"`
		Acceptance  designworkflow.AcceptanceResult `json:"acceptance"`
		Stages      []stageEvidence                 `json:"stages"`
		Artifacts   []reports.ArtifactKind          `json:"artifacts"`
	}
	type hashEvidence struct {
		Schema          string                     `json:"schema"`
		Version         int                        `json:"version"`
		PolicyVersion   string                     `json:"policy_version"`
		RequirementHash string                     `json:"requirement_hash"`
		InventoryHash   string                     `json:"inventory_hash"`
		SynthesisHash   string                     `json:"synthesis_hash"`
		PhysicalHash    string                     `json:"physical_hash"`
		Status          ot.PhysicalPromotionStatus `json:"status"`
		ReplayIdentical bool                       `json:"replay_identical"`
		ProjectHash     string                     `json:"project_hash,omitempty"`
		Runs            []runEvidence              `json:"runs"`
		Issues          []issueEvidence            `json:"issues"`
	}
	issueView := func(issue reports.Issue) issueEvidence {
		return issueEvidence{
			Code:     issue.Code,
			Severity: issue.Severity,
			Stage:    issue.Stage,
		}
	}
	view := hashEvidence{
		Schema:          result.Schema,
		Version:         result.Version,
		PolicyVersion:   result.PolicyVersion,
		RequirementHash: result.RequirementHash,
		InventoryHash:   result.InventoryHash,
		SynthesisHash:   result.SynthesisHash,
		PhysicalHash:    result.PhysicalHash,
		Status:          result.Status,
		ReplayIdentical: result.ReplayIdentical,
		ProjectHash:     result.ProjectHash,
		Runs:            []runEvidence{},
		Issues:          []issueEvidence{},
	}
	for _, issue := range result.Issues {
		view.Issues = append(view.Issues, issueView(issue))
	}
	for _, run := range result.Runs {
		runView := runEvidence{
			Number:      run.Number,
			ProjectHash: run.ProjectHash,
			Acceptance:  run.Workflow.Acceptance,
			Stages:      []stageEvidence{},
			Artifacts:   []reports.ArtifactKind{},
		}
		for _, stage := range run.Workflow.Stages {
			stageView := stageEvidence{
				Name:      stage.Name,
				Status:    stage.Status,
				Issues:    []issueEvidence{},
				Artifacts: []reports.ArtifactKind{},
			}
			for _, issue := range stage.Issues {
				stageView.Issues = append(stageView.Issues, issueView(issue))
			}
			for _, artifact := range stage.Artifacts {
				stageView.Artifacts = append(stageView.Artifacts, artifact.Kind)
			}
			slices.Sort(stageView.Artifacts)
			runView.Stages = append(runView.Stages, stageView)
		}
		for _, artifact := range run.Artifacts {
			runView.Artifacts = append(runView.Artifacts, artifact.Kind)
		}
		slices.Sort(runView.Artifacts)
		view.Runs = append(view.Runs, runView)
	}
	return view
}
