package rationale

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentdraft"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

func intentFromPlan(plan intentplanner.PlanResult) IntentSummary {
	intent := IntentSummary{
		Name:                plan.Intent.Name,
		Kind:                string(plan.Intent.Kind),
		RequestedAcceptance: plan.Intent.Acceptance,
		NormalizedSummary:   plan.Intent.Summary,
	}
	if plan.GeneratedRequest != nil {
		intent.Board = BoardSummary{
			WidthMM:  plan.GeneratedRequest.Board.WidthMM,
			HeightMM: plan.GeneratedRequest.Board.HeightMM,
			Layers:   plan.GeneratedRequest.Board.Layers,
		}
		for _, block := range plan.GeneratedRequest.Blocks {
			intent.Functions = append(intent.Functions, block.ID+":"+block.BlockID)
		}
		for _, connection := range plan.GeneratedRequest.Connections {
			intent.Interfaces = append(intent.Interfaces, connectionSummary(connection.From, connection.To, connection.NetAlias))
		}
	}
	return intent
}

func intentFromRequest(request intentplanner.Request) IntentSummary {
	intent := IntentSummary{
		Name:                request.Name,
		Kind:                string(request.Kind),
		RequestedAcceptance: request.Acceptance,
		NormalizedSummary:   request.Summary,
		Board: BoardSummary{
			WidthMM:  request.Board.WidthMM,
			HeightMM: request.Board.HeightMM,
			Layers:   request.Board.Layers,
		},
	}
	for _, input := range request.Power.Inputs {
		intent.Power = append(intent.Power, strings.TrimSpace(input.Kind+" "+input.Voltage))
	}
	for _, rail := range request.Power.Rails {
		intent.Power = append(intent.Power, strings.TrimSpace(rail.Name+" "+rail.Voltage))
	}
	for _, iface := range request.Interfaces {
		intent.Interfaces = append(intent.Interfaces, strings.TrimSpace(iface.Kind+" "+iface.Voltage))
	}
	for _, function := range request.Functions {
		value := function.Kind
		if function.Family != "" {
			value += ":" + function.Family
		}
		intent.Functions = append(intent.Functions, value)
	}
	if request.Manufacturing.Profile != "" {
		intent.Manufacturing = append(intent.Manufacturing, "profile:"+request.Manufacturing.Profile)
	}
	if request.Manufacturing.FabricationCandidate {
		intent.Manufacturing = append(intent.Manufacturing, "fabrication_candidate")
	}
	if request.Constraints.PreferSMD {
		intent.Constraints = append(intent.Constraints, "prefer_smd")
	}
	if request.Constraints.AllowPlaceholders {
		intent.Constraints = append(intent.Constraints, "allow_placeholders")
	}
	if request.Constraints.SkipRouting {
		intent.Constraints = append(intent.Constraints, "skip_routing")
	}
	return intent
}

func evidenceFromRequirements(requirements []intentplanner.RequirementRecord) []EvidenceRecord {
	out := make([]EvidenceRecord, 0, len(requirements))
	for _, requirement := range requirements {
		summary := requirement.Value
		if summary == "" {
			summary = requirement.Implementation
		}
		if summary == "" {
			summary = requirement.Type
		}
		out = append(out, EvidenceRecord{
			ID:      "req:" + requirement.ID,
			Kind:    "planner_requirement",
			Path:    requirement.Path,
			Summary: summary,
			Notes:   append([]string(nil), requirement.Evidence...),
		})
	}
	return out
}

func decisionsFromPlan(plan intentplanner.PlanResult) []Decision {
	var out []Decision
	for _, block := range plan.SelectedBlocks {
		selected := block.InstanceID
		if block.BlockID != "" {
			selected += ":" + block.BlockID
		}
		out = append(out, Decision{
			ID:             "block:" + firstNonEmpty(block.InstanceID, block.BlockID),
			Type:           "block_selection",
			Path:           "selected_blocks." + firstNonEmpty(block.InstanceID, block.BlockID),
			Selected:       selected,
			Rationale:      firstNonEmpty(block.Rationale, "selected block satisfies planned requirements"),
			RequirementIDs: append([]string(nil), block.RequirementIDs...),
			Confidence:     block.Verification,
			Status:         block.Readiness,
		})
	}
	for index, component := range plan.SelectedComponents {
		selected := component.Family
		if component.PackagePreference != "" {
			selected += ":" + component.PackagePreference
		}
		out = append(out, Decision{
			ID:             fmt.Sprintf("component:%03d", index+1),
			Type:           "component_selection",
			Path:           component.RequirementID,
			Selected:       selected,
			Rationale:      firstNonEmpty(component.Rationale, "selected component family matches planned requirement"),
			RequirementIDs: compactStrings([]string{component.RequirementID}),
			Confidence:     component.MinimumConfidence,
			Status:         component.Acceptance,
		})
	}
	for index, connection := range plan.Connections {
		out = append(out, Decision{
			ID:             fmt.Sprintf("connection:%03d", index+1),
			Type:           "connection",
			Selected:       connectionSummary(connection.From, connection.To, connection.NetAlias),
			Rationale:      firstNonEmpty(connection.Rationale, "connection satisfies planned net requirement"),
			RequirementIDs: append([]string(nil), connection.RequirementIDs...),
		})
	}
	for _, note := range plan.Clarifications {
		out = append(out, Decision{
			ID:        "clarification:" + note.ID,
			Type:      "clarification",
			Path:      note.Path,
			Selected:  "needs user input",
			Rationale: note.Message,
			Status:    string(note.Severity),
		})
	}
	for _, note := range plan.KnownGaps {
		out = append(out, Decision{
			ID:        "gap:" + note.ID,
			Type:      "known_gap",
			Path:      note.Path,
			Selected:  "limited support",
			Rationale: note.Message,
			Status:    string(note.Severity),
		})
	}
	return out
}

func notesFromPlan(notes []intentplanner.PlanNote, evidenceIDs []string) []RationaleNote {
	out := make([]RationaleNote, 0, len(notes))
	for _, note := range notes {
		out = append(out, RationaleNote{
			ID:          note.ID,
			Path:        note.Path,
			Message:     note.Message,
			Severity:    note.Severity,
			Suggestion:  note.Suggestion,
			EvidenceIDs: append([]string(nil), evidenceIDs...),
		})
	}
	return out
}

func limitsFromPlan(plan intentplanner.PlanResult) []KnownLimit {
	var out []KnownLimit
	for _, note := range plan.KnownGaps {
		out = append(out, KnownLimit{
			ID:         "gap:" + note.ID,
			Category:   categoryForPath(note.Path, note.Message),
			Severity:   severityString(note.Severity),
			Path:       note.Path,
			Message:    note.Message,
			Suggestion: note.Suggestion,
		})
	}
	for index, issue := range plan.Issues {
		out = append(out, limitFromIssue(fmt.Sprintf("plan_issue:%03d", index+1), "", issue))
	}
	return out
}

func applyDraft(report *Report, draft intentdraft.Result) {
	if report.Source.Mode == "" {
		report.Source.Mode = draft.Extraction.SourceType
	}
	if report.Source.Path == "" {
		report.Source.Path = draft.Extraction.SourceID
	}
	if report.Source.SourceHash == "" {
		report.Source.SourceHash = draft.Extraction.SourceHash
	}
	if report.Source.Summary == "" {
		report.Source.Summary = draft.Extraction.Summary
	}
	if report.Intent.Name == "" {
		request := intentplanner.NormalizeRequest(draft.Request)
		report.Intent = intentFromRequest(request)
	}
	report.Intent.DraftedSummary = draft.Extraction.Summary
	for index, field := range draft.Extraction.Fields {
		id := fmt.Sprintf("draft:%03d", index+1)
		report.Evidence = append(report.Evidence, EvidenceRecord{
			ID:         id,
			Kind:       "draft_field",
			Path:       field.Path,
			Summary:    fmt.Sprintf("%s=%v", field.Path, field.Value),
			SourceText: field.SourceText,
			Confidence: field.Confidence,
			Notes:      append([]string(nil), field.Notes...),
		})
	}
	for _, assumption := range draft.Extraction.Assumptions {
		report.Assumptions = append(report.Assumptions, RationaleNote{
			ID:       assumption.ID,
			Path:     assumption.Path,
			Message:  assumption.Message,
			Severity: reports.SeverityInfo,
		})
	}
	for _, clarification := range draft.Clarifications {
		severity := reports.SeverityWarning
		if clarification.Severity == intentdraft.ClarificationBlocking {
			severity = reports.SeverityError
		}
		evidenceIDs := evidenceIDsForClarification(clarification)
		report.Clarifications = append(report.Clarifications, RationaleNote{
			ID:          clarification.ID,
			Path:        clarification.Path,
			Message:     clarification.Question,
			Severity:    severity,
			Suggestion:  clarification.Suggestion,
			EvidenceIDs: evidenceIDs,
		})
		report.Decisions = append(report.Decisions, Decision{
			ID:          "clarification:" + clarification.ID,
			Type:        "clarification",
			Path:        clarification.Path,
			Selected:    "needs user input",
			Rationale:   clarification.Question,
			EvidenceIDs: evidenceIDs,
			Status:      string(clarification.Severity),
		})
		report.KnownLimits = append(report.KnownLimits, KnownLimit{
			ID:          "clarification:" + clarification.ID,
			Category:    categoryForPath(clarification.Path, clarification.Question),
			Severity:    severityString(severity),
			Path:        clarification.Path,
			Message:     clarification.Question,
			Suggestion:  clarification.Suggestion,
			EvidenceIDs: evidenceIDs,
		})
	}
	for index, issue := range draft.Issues {
		report.KnownLimits = append(report.KnownLimits, limitFromIssue(fmt.Sprintf("draft_issue:%03d", index+1), "", issue))
	}
}

func applyWorkflow(report *Report, workflow designworkflow.WorkflowResult) {
	report.Validation.RequestedAcceptance = firstNonEmpty(report.Validation.RequestedAcceptance, string(workflow.Acceptance.Requested))
	report.Validation.AchievedAcceptance = string(workflow.Acceptance.Achieved)
	report.Validation.StageCount = len(workflow.Stages)
	report.Validation.BlockingCount += workflow.Feedback.Summary.BlockingCount
	report.Validation.WarningCount += workflow.Feedback.Summary.WarningCount
	for _, stage := range workflow.Stages {
		switch stage.Status {
		case designworkflow.StageStatusOK:
			report.Validation.CompletedStages++
		case designworkflow.StageStatusSkipped:
			report.Validation.SkippedStages++
		case designworkflow.StageStatusWarning:
			report.Validation.CompletedStages++
			report.Validation.WarningCount++
		case designworkflow.StageStatusBlocked:
			report.Validation.BlockingCount++
		}
		evidenceID := "stage:" + string(stage.Name)
		report.Evidence = append(report.Evidence, EvidenceRecord{
			ID:      evidenceID,
			Kind:    "workflow_stage",
			Path:    string(stage.Name),
			Summary: string(stage.Status),
		})
		for index, issue := range stage.Issues {
			report.Evidence = append(report.Evidence, EvidenceRecord{
				ID:      fmt.Sprintf("stage_issue:%s:%03d", stage.Name, index+1),
				Kind:    "validation_issue",
				Path:    issue.Path,
				Summary: issue.Message,
				Notes:   compactStrings([]string{string(issue.Code), issue.Suggestion}),
			})
			report.KnownLimits = append(report.KnownLimits, limitFromIssue(fmt.Sprintf("workflow_issue:%s:%03d", stage.Name, index+1), string(stage.Name), issue))
		}
		report.ArtifactRefs = append(report.ArtifactRefs, stage.Artifacts...)
	}
	for _, repair := range workflow.Feedback.Repairs {
		if repair.SuggestedAction == "" {
			continue
		}
		report.NextActions = append(report.NextActions, NextAction{
			ID:       "workflow:" + string(repair.Stage) + ":" + string(repair.Code),
			Priority: priorityForSeverity(repair.Severity),
			Action:   repair.SuggestedAction,
			Reason:   repair.Message,
		})
	}
}

func LoadFromTarget(target string) TargetLoadResult {
	metadataDir := filepath.Join(target, MetadataDirName)
	var loadIssues []reports.Issue
	var draft *intentdraft.Result
	var request *intentplanner.Request
	var plan *intentplanner.PlanResult
	var workflow *designworkflow.WorkflowResult
	if fileExists(filepath.Join(metadataDir, "intent-draft.json")) {
		var draftRequest intentplanner.Request
		if issue := readJSON(filepath.Join(metadataDir, "intent-draft.json"), &draftRequest); issue != nil {
			loadIssues = append(loadIssues, *issue)
		} else {
			request = &draftRequest
		}
	}
	if fileExists(filepath.Join(metadataDir, "intent-extraction.json")) || fileExists(filepath.Join(metadataDir, "intent-clarifications.json")) {
		d := intentdraft.Result{}
		if request != nil {
			d.Request = *request
		}
		if fileExists(filepath.Join(metadataDir, "intent-extraction.json")) {
			if issue := readJSON(filepath.Join(metadataDir, "intent-extraction.json"), &d.Extraction); issue != nil {
				loadIssues = append(loadIssues, *issue)
			}
		}
		if fileExists(filepath.Join(metadataDir, "intent-clarifications.json")) {
			if issue := readJSON(filepath.Join(metadataDir, "intent-clarifications.json"), &d.Clarifications); issue != nil {
				loadIssues = append(loadIssues, *issue)
			}
		}
		draft = &d
	}
	if fileExists(filepath.Join(metadataDir, "intent-plan.json")) {
		var loaded intentplanner.PlanResult
		if issue := readJSON(filepath.Join(metadataDir, "intent-plan.json"), &loaded); issue != nil {
			loadIssues = append(loadIssues, *issue)
		} else {
			loaded = intentplanner.NormalizePlan(loaded)
			plan = &loaded
		}
	}
	if fileExists(filepath.Join(metadataDir, "workflow-result.json")) {
		var loaded designworkflow.WorkflowResult
		if issue := readJSON(filepath.Join(metadataDir, "workflow-result.json"), &loaded); issue != nil {
			loadIssues = append(loadIssues, *issue)
		} else {
			workflow = &loaded
		}
	}
	source := SourceSummary{Mode: "target", Path: target}
	if draft != nil {
		source.SourceHash = draft.Extraction.SourceHash
		source.Summary = draft.Extraction.Summary
	}
	if request == nil && draft != nil {
		request = &draft.Request
	}
	if request == nil && plan == nil && draft == nil && workflow == nil {
		loadIssues = append(loadIssues, reports.Issue{
			Code:       reports.CodeMissingFile,
			Severity:   reports.SeverityError,
			Path:       metadataDir,
			Message:    "target lacks supported .kicadai rationale artifacts",
			Suggestion: "run intent create or provide --request/--text to build rationale",
		})
	}
	report := Build(BuildOptions{Source: source, Request: request, Draft: draft, Plan: plan, Workflow: workflow})
	for index, issue := range loadIssues {
		report.KnownLimits = append(report.KnownLimits, limitFromIssue(fmt.Sprintf("target_issue:%03d", index+1), "", issue))
	}
	report = Normalize(report)
	return TargetLoadResult{Report: report, Issues: loadIssues}
}

func readJSON(path string, value any) *reports.Issue {
	data, err := os.ReadFile(path)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: path, Message: err.Error()}
		return &issue
	}
	if err := json.Unmarshal(data, value); err != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: err.Error()}
		return &issue
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
