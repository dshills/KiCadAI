package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
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

func TestOpenTopologyLibraryIssuesAreScopedToSelectedDesign(t *testing.T) {
	selectedPath := filepath.Join(t.TempDir(), "Selected.pretty", "R_0603.kicad_mod")
	unrelatedPath := filepath.Join(t.TempDir(), "Unrelated.pretty", "Broken.kicad_mod")
	index := libraryresolver.LibraryIndex{
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Selected:R_0603": {
				FootprintID: "Selected:R_0603",
				Path:        selectedPath,
			},
		},
	}
	resolved := circuitgraph.ResolvedDocument{
		Components: []circuitgraph.ResolvedComponent{{
			ComponentID: "resistor",
			VariantID:   "0603",
			FootprintID: "Selected:R_0603",
		}},
	}
	libraryIssues := []reports.Issue{
		{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     unrelatedPath,
			Message:  "unrelated stock-library parse failure",
		},
		{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     selectedPath,
			Message:  "selected footprint warning",
		},
	}

	issues := scopeOpenTopologyLibraryIssues(&index, libraryIssues, resolved)
	if len(issues) != 1 || issues[0].Path != selectedPath ||
		issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("scoped issues = %#v", issues)
	}
	if len(index.Diagnostics) != 1 || !reflect.DeepEqual(index.Diagnostics[0], issues[0]) {
		t.Fatalf("index diagnostics = %#v, want %#v", index.Diagnostics, issues)
	}
}
