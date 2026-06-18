package repair

import (
	"strings"

	"kicadai/internal/reports"
)

type Planner struct {
	Options Options
}

func NewPlanner(opts Options) Planner {
	defaults := DefaultOptions()
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaults.MaxAttempts
	}
	if opts.MaxAttemptsPerIssue <= 0 {
		opts.MaxAttemptsPerIssue = defaults.MaxAttemptsPerIssue
	}
	return Planner{Options: opts}
}

func BuildPlan(groups []StageIssues, opts Options) Plan {
	return NewPlanner(opts).Build(groups)
}

func (planner Planner) Build(groups []StageIssues) Plan {
	opts := planner.Options
	if !opts.Enabled {
		return Plan{
			Status:  StatusSkipped,
			Options: opts,
			Summary: Summary{},
		}
	}
	attempts := make([]Attempt, 0)
	perIssue := map[string]int{}
	for _, group := range groups {
		for _, issue := range group.Issues {
			if len(attempts) >= opts.MaxAttempts {
				return finalizePlan(opts, attempts)
			}
			key := issueKey(group.Stage, issue)
			if perIssue[key] >= opts.MaxAttemptsPerIssue {
				continue
			}
			classification := Classify(issue)
			action, status, message := planner.actionFor(classification)
			attempt := Attempt{
				Number:   len(attempts) + 1,
				Stage:    group.Stage,
				Issue:    issue,
				Category: classification.Category,
				Action:   action,
				Status:   status,
				DryRun:   !opts.Apply,
				Message:  firstNonEmpty(message, classification.Reason),
			}
			attempts = append(attempts, attempt)
			perIssue[key]++
		}
	}
	return finalizePlan(opts, attempts)
}

func (planner Planner) actionFor(classification Classification) (Action, Status, string) {
	if !classification.Repairable {
		return ActionUnsupported, StatusBlocked, classification.Reason
	}
	switch classification.Category {
	case CategoryMissingFootprint:
		if !planner.Options.AllowFootprintAssignment {
			return ActionAssignFootprint, StatusSkipped, "footprint assignment repair is disabled"
		}
		return ActionAssignFootprint, StatusPlanned, "planned footprint assignment repair"
	case CategoryInvalidNetAssignment, CategoryDisconnectedPad:
		if !planner.Options.AllowPadNetRegeneration {
			return ActionRegeneratePadNets, StatusSkipped, "pad net hint regeneration is disabled"
		}
		return ActionRegeneratePadNets, StatusPlanned, "planned pad net hint regeneration"
	case CategoryUnroutedNet, CategoryRouteClearance:
		if !planner.Options.AllowRoutingRetry {
			return ActionRerouteNet, StatusSkipped, "routing retry is disabled"
		}
		return ActionRerouteNet, StatusPlanned, "planned routing retry"
	case CategoryPlacementCollision, CategoryPlacementOutside:
		if !planner.Options.AllowPlacementRetry {
			return ActionRetryPlacement, StatusSkipped, "placement retry is disabled"
		}
		return ActionRetryPlacement, StatusPlanned, "planned placement retry"
	case CategoryMissingBoardOutline:
		if !planner.Options.AllowOutlineGeneration {
			return ActionGenerateOutline, StatusSkipped, "outline generation repair is disabled"
		}
		return ActionGenerateOutline, StatusPlanned, "planned board outline generation"
	case CategoryZoneUnfilled:
		if !planner.Options.AllowKiCadCLI {
			return ActionRequireKiCadRefill, StatusSkipped, "zone refill requires KiCad CLI"
		}
		return ActionRequireKiCadRefill, StatusPlanned, "planned KiCad zone refill"
	case CategoryZoneWrongNet:
		if !planner.Options.AllowZoneNetRepair {
			return ActionRepairZoneNet, StatusSkipped, "zone net repair is disabled"
		}
		return ActionRepairZoneNet, StatusPlanned, "planned zone net repair"
	default:
		return ActionUnsupported, StatusBlocked, "no deterministic repair action is registered"
	}
}

func finalizePlan(opts Options, attempts []Attempt) Plan {
	summary := summarizeAttempts(attempts)
	status := StatusNotNeeded
	if len(attempts) == 0 {
		if opts.Enabled {
			status = StatusNotNeeded
		} else {
			status = StatusSkipped
		}
	} else if summary.PlannedCount > 0 && (summary.BlockedCount > 0 || summary.SkippedCount > 0) {
		status = StatusPartial
	} else if summary.PlannedCount > 0 {
		status = StatusPlanned
	} else if summary.BlockedCount > 0 {
		status = StatusBlocked
	} else {
		status = StatusSkipped
	}
	return Plan{Status: status, Options: opts, Attempts: attempts, Summary: summary}
}

func summarizeAttempts(attempts []Attempt) Summary {
	var summary Summary
	summary.AttemptCount = len(attempts)
	for _, attempt := range attempts {
		switch attempt.Status {
		case StatusBlocked:
			summary.BlockedCount++
		case StatusPlanned:
			summary.PlannedCount++
		case StatusSkipped:
			summary.SkippedCount++
		case StatusRepaired:
			summary.RepairedCount++
		}
		if !attempt.DryRun && attempt.Status == StatusRepaired {
			summary.AppliedCount++
		}
	}
	return summary
}

func issueKey(stage string, issue reports.Issue) string {
	var builder strings.Builder
	builder.Grow(len(stage) + len(issue.Code) + len(issue.Path) + 2)
	builder.WriteString(stage)
	builder.WriteByte(0)
	builder.WriteString(string(issue.Code))
	builder.WriteByte(0)
	builder.WriteString(issue.Path)
	return builder.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
