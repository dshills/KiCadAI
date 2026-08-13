package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRequiresCompleteArguments(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), []string{"--baseline-root", "baseline"}, &output)
	if err == nil || !strings.Contains(err.Error(), "--baseline-root, --next-report") {
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
