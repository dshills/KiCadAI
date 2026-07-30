package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsInvalidInvocationBeforeExternalWork(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing command", contains: "usage:"},
		{name: "unknown command", arguments: []string{"unknown"}, contains: "unknown command"},
		{name: "promote missing output", arguments: []string{"promote"}, contains: "--output"},
		{name: "verify missing bundle", arguments: []string{"verify"}, contains: "--bundle"},
		{name: "resolve invalid timeout", arguments: []string{"resolve", "--timeout=0"}, contains: "timeout must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("run(%q) error = %v, want text %q", test.arguments, err, test.contains)
			}
		})
	}
}

func TestExecuteReturnsFailureExitCodeAndDiagnostic(t *testing.T) {
	var stderr bytes.Buffer
	if code := execute([]string{"unknown"}, &stderr); code != 1 {
		t.Fatalf("failure exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `kicadai-promotion: unknown command "unknown"`) {
		t.Fatalf("failure diagnostic = %q", stderr.String())
	}
}
