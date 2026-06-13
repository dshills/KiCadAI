package blocks

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

func TestValidateOutputLibrariesWarnsWithoutIndex(t *testing.T) {
	issues := ValidateOutputLibraries(BlockOutput{}, nil)
	if len(issues) != 1 || issues[0].Code != reports.CodeUnknownSymbolLibrary {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateOutputLibrariesReportsMissingRecords(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, instantiateIssues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if len(instantiateIssues) != 0 {
		t.Fatalf("instantiate issues = %#v", instantiateIssues)
	}
	index := libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{}, Footprints: map[string]libraryresolver.FootprintRecord{}}
	issues := ValidateOutputLibraries(output, &index)
	if len(issues) == 0 {
		t.Fatalf("expected missing record issues")
	}
	found := false
	for _, issue := range issues {
		if issue.Code == reports.CodeMissingFile && strings.Contains(issue.Message, "Device:R") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing Device:R issue not found: %#v", issues)
	}
}

func TestValidateOutputLibrariesChecksAssignmentsAndPinmaps(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, instantiateIssues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if len(instantiateIssues) != 0 {
		t.Fatalf("instantiate issues = %#v", instantiateIssues)
	}
	index := blockResolverFixture()
	issues := ValidateOutputLibraries(output, &index)
	if len(issues) == 0 {
		t.Fatalf("expected pinmap readiness warnings")
	}
	for _, issue := range issues {
		if issue.Code == reports.CodeMissingFile {
			t.Fatalf("unexpected missing library issue: %#v", issues)
		}
	}
	foundPinmapWarning := false
	for _, issue := range issues {
		if issue.Code == reports.CodePinmapUnverified {
			foundPinmapWarning = true
		}
	}
	if !foundPinmapWarning {
		t.Fatalf("pinmap warning missing: %#v", issues)
	}
}

func blockResolverFixture() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:R": {
				LibraryID:       "Device:R",
				Name:            "R",
				FootprintFilter: []string{"R_0805*"},
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive"},
					{Number: "2", Electrical: "passive"},
				},
			},
			"Device:LED": {
				LibraryID:       "Device:LED",
				Name:            "LED",
				FootprintFilter: []string{"LED_0805*"},
				Pins: []libraryresolver.SymbolPin{
					{Number: "1", Electrical: "passive"},
					{Number: "2", Electrical: "passive"},
				},
			},
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": {
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				Name:        "R_0805_2012Metric",
				Pads: []libraryresolver.FootprintPad{
					{Name: "1"},
					{Name: "2"},
				},
			},
			"LED_SMD:LED_0805_2012Metric": {
				FootprintID: "LED_SMD:LED_0805_2012Metric",
				Name:        "LED_0805_2012Metric",
				Pads: []libraryresolver.FootprintPad{
					{Name: "1"},
					{Name: "2"},
				},
			},
		},
	}
}
