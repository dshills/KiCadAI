package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRequirementRejectsMissingCreateSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	err := runRequirement(context.Background(), cliOptions{jsonOutput: true}, &stdout)
	if err == nil {
		t.Fatal("runRequirement error = nil, want missing subcommand error")
	}
	var response struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &response); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if response.OK || response.Command != "requirement.create" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunRequirementCreatesBehaviorOnlyEvidenceOptionalKiCadLibraries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	symbolsRoot := os.Getenv("KICAD_SYMBOLS_ROOT")
	footprintsRoot := os.Getenv("KICAD_FOOTPRINTS_ROOT")
	if symbolsRoot == "" {
		symbolsRoot = "/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols"
	}
	if footprintsRoot == "" {
		footprintsRoot = "/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints"
	}
	if _, err := os.Stat(symbolsRoot); err != nil {
		t.Skipf("KiCad symbol libraries unavailable: %v", err)
	}
	if _, err := os.Stat(footprintsRoot); err != nil {
		t.Skipf("KiCad footprint libraries unavailable: %v", err)
	}
	output := filepath.Join(t.TempDir(), "project")
	opts := cliOptions{
		commandArgs:  []string{"create"},
		jsonOutput:   true,
		outputFormat: "json",
		requestPath: filepath.Join(
			root,
			"internal", "architecturesearch", "testdata", "held_out_capability_expansion_corpus",
			"analog_precision_rectifier.json",
		),
		output:         output,
		catalogDir:     filepath.Join(root, "data", "components"),
		symbolsRoot:    symbolsRoot,
		footprintsRoot: footprintsRoot,
		overwrite:      true,
	}
	var stdout bytes.Buffer
	if err := runRequirement(context.Background(), opts, &stdout); err != nil {
		t.Fatalf("runRequirement: %v\n%s", err, stdout.String())
	}
	var response struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Command != "requirement.create" {
		t.Fatalf("response = %#v", response)
	}
	for _, name := range []string{
		"normalized-requirement.json",
		"architecture-search.json",
		"closed-loop-synthesis.json",
		"simulation.json",
		"design-request.json",
		"transaction.json",
		"workflow-result.json",
		"validation-summary.json",
		"design-promotion.json",
		"manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(output, ".kicadai", name)); err != nil {
			t.Fatalf("missing evidence artifact %s: %v", name, err)
		}
	}
}
