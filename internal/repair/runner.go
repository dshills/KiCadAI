package repair

import (
	"context"

	"kicadai/internal/reports"
)

type Validator interface {
	Validate() []reports.Issue
}

type ValidatorFunc func() []reports.Issue

func (fn ValidatorFunc) Validate() []reports.Issue {
	return fn()
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
		latestIssues = runner.Validate.Validate()
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
