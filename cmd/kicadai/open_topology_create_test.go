package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenTopologyCreateRequiresStrictRequestAndOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(
		[]string{"--json", "open-topology", "create"},
		&stdout,
		&stderr,
	)
	if err == nil ||
		!strings.Contains(stdout.String(), `"command": "open-topology.create"`) ||
		!strings.Contains(stdout.String(), `"path": "request"`) {
		t.Fatalf(
			"missing request result err=%v stdout=%s stderr=%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}

	root := t.TempDir()
	request := filepath.Join(root, "requirement.json")
	if err := os.WriteFile(request, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	err = run(
		[]string{
			"--json",
			"--request", request,
			"--output", filepath.Join(root, "project"),
			"open-topology", "create",
		},
		&stdout,
		&stderr,
	)
	if err == nil ||
		!strings.Contains(stdout.String(), `"command": "open-topology.create"`) ||
		!strings.Contains(stdout.String(), `"OPEN_TOPOLOGY_REQUIREMENT_INVALID"`) {
		t.Fatalf(
			"invalid request result err=%v stdout=%s stderr=%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
}
