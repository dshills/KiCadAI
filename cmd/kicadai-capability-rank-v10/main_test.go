package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRejectsIncompleteArguments(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), []string{"--baseline-root", "baseline"}, &output)
	if err == nil || !strings.Contains(err.Error(), "--baseline-root, --plans, and --output are required") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), []string{"unexpected"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunReportsMissingGitDependency(t *testing.T) {
	t.Setenv("PATH", "")
	var output bytes.Buffer
	err := run(context.Background(), []string{
		"--repository-root", t.TempDir(), "--baseline-root", "baseline",
		"--plans", "plans.json", "--output", "ranking.json",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "requires the git executable in PATH") {
		t.Fatalf("error = %v", err)
	}
}
