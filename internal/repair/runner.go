package repair

import (
	"context"
	"slices"

	"kicadai/internal/reports"
)

type Validator interface {
	Validate() []reports.Issue
}

type ValidatorFunc func() []reports.Issue

func (fn ValidatorFunc) Validate() []reports.Issue {
	return fn()
}

type AttemptAwareValidator interface {
	ValidateAttempt(Attempt, []reports.Issue) []reports.Issue
}

type Runner struct {
	Options  Options
	Executor *Executor
	Validate Validator
}

func NewRunner(options Options, executor *Executor, validate Validator) Runner {
	defaults := DefaultOptions()
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = defaults.MaxAttempts
	}
	if options.MaxAttemptsPerIssue <= 0 {
		options.MaxAttemptsPerIssue = defaults.MaxAttemptsPerIssue
	}
	return Runner{Options: options, Executor: executor, Validate: validate}
}

func (runner Runner) Run(groups []StageIssues) Result {
	return runner.RunContext(context.Background(), groups)
}

func (runner Runner) RunContext(ctx context.Context, groups []StageIssues) Result {
	if !runner.Options.Enabled {
		return Result{Status: StatusSkipped, Summary: Summary{}}
	}
	if err := ctx.Err(); err != nil {
		issues := []reports.Issue{{
			Code:     reports.CodeOperationCanceled,
			Severity: reports.SeverityBlocked,
			Path:     "context",
			Message:  err.Error(),
		}}
		return Result{Status: StatusBlocked, FinalIssues: issues, Summary: Summary{BlockedCount: 1}}
	}
	initialIssues := flattenIssues(groups)
	if len(initialIssues) == 0 {
		return Result{Status: StatusNotNeeded, Summary: Summary{}}
	}
	if runner.Executor == nil {
		return Result{Status: StatusBlocked, FinalIssues: initialIssues, Summary: Summary{BlockedCount: len(initialIssues)}}
	}
	if runner.Validate == nil {
		return Result{Status: StatusBlocked, FinalIssues: initialIssues, Summary: Summary{BlockedCount: len(initialIssues)}}
	}
	opts := runner.Options
	opts.Apply = true
	opts.Enabled = true
	plan := BuildPlan(groups, opts)
	if len(plan.Attempts) == 0 {
		return Result{Status: plan.Status, FinalIssues: initialIssues, Summary: plan.Summary}
	}
	attempts := []Attempt{}
	latestIssues := initialIssues
	for _, planned := range plan.Attempts {
		if err := ctx.Err(); err != nil {
			latestIssues = append(latestIssues, reports.Issue{
				Code:     reports.CodeOperationCanceled,
				Severity: reports.SeverityBlocked,
				Path:     "context",
				Message:  err.Error(),
			})
			return Result{Status: StatusBlocked, Attempts: attempts, FinalIssues: latestIssues, Summary: summarizeAttempts(attempts)}
		}
		if planned.Status != StatusPlanned {
			attempts = append(attempts, planned)
			if planned.Status == StatusBlocked {
				break
			}
			continue
		}
		planned.BeforeIssues = len(latestIssues)
		planned.DryRun = false
		executed := runner.Executor.Execute(planned)
		if validator, ok := runner.Validate.(AttemptAwareValidator); ok {
			latestIssues = validator.ValidateAttempt(executed, latestIssues)
		} else {
			latestIssues = runner.Validate.Validate()
		}
		executed.AfterIssues = len(latestIssues)
		executed.Issues = append([]reports.Issue(nil), latestIssues...)
		if len(latestIssues) == 0 {
			executed.Status = StatusRepaired
			attempts = append(attempts, executed)
			return Result{Status: StatusRepaired, Attempts: attempts, FinalIssues: nil, Summary: summarizeAttempts(attempts)}
		}
		attempts = append(attempts, executed)
		if executed.Status == StatusBlocked || len(latestIssues) > planned.BeforeIssues {
			return Result{Status: StatusBlocked, Attempts: attempts, FinalIssues: latestIssues, Summary: summarizeAttempts(attempts)}
		}
	}
	status := StatusBlocked
	if len(latestIssues) < len(initialIssues) {
		status = StatusPartial
	}
	return Result{Status: status, Attempts: attempts, FinalIssues: latestIssues, Summary: summarizeAttempts(attempts)}
}

func removeAttemptedIssue(issues []reports.Issue, attempted reports.Issue) []reports.Issue {
	out := make([]reports.Issue, 0, max(0, len(issues)-1))
	removed := false
	for _, issue := range issues {
		if !removed && sameRepairIssue(issue, attempted) {
			removed = true
			continue
		}
		out = append(out, issue)
	}
	return out
}

func sameRepairIssue(a, b reports.Issue) bool {
	if a.Code != b.Code || a.Path != b.Path {
		return false
	}
	if !sameStringSet(a.Refs, b.Refs) {
		return false
	}
	if !sameStringSet(a.Nets, b.Nets) {
		return false
	}
	return true
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if slices.Equal(a, b) {
		return true
	}
	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	slices.Sort(left)
	slices.Sort(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func flattenIssues(groups []StageIssues) []reports.Issue {
	count := 0
	for _, group := range groups {
		count += len(group.Issues)
	}
	out := make([]reports.Issue, 0, count)
	for _, group := range groups {
		out = append(out, group.Issues...)
	}
	return out
}
