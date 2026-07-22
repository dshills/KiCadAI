package reports

const Version = "0.1.0"

type Result struct {
	OK          bool               `json:"ok"`
	Command     string             `json:"command"`
	Version     string             `json:"version"`
	Data        any                `json:"data,omitempty"`
	Issues      []Issue            `json:"issues"`
	Diagnostics *DiagnosticSummary `json:"diagnostics,omitempty"`
	Artifacts   []Artifact         `json:"artifacts"`
}

func OKResult(command string, data any, artifacts []Artifact) Result {
	if artifacts == nil {
		artifacts = []Artifact{}
	}
	return Result{
		OK:        true,
		Command:   command,
		Version:   Version,
		Data:      data,
		Issues:    []Issue{},
		Artifacts: artifacts,
	}
}

func ErrorResult(command string, issue Issue) Result {
	return Result{
		OK:        !issue.Blocking(),
		Command:   command,
		Version:   Version,
		Issues:    []Issue{issue},
		Artifacts: []Artifact{},
	}
}

func ResultWithIssues(command string, data any, issues []Issue, artifacts []Artifact) Result {
	if issues == nil {
		issues = []Issue{}
	}
	if artifacts == nil {
		artifacts = []Artifact{}
	}
	return Result{
		OK:        !HasBlockingIssue(issues),
		Command:   command,
		Version:   Version,
		Data:      data,
		Issues:    issues,
		Artifacts: artifacts,
	}
}

func HasBlockingIssue(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}
