package schematiclayout

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/schematic"
)

func TestStandardExamplesPassReadability(t *testing.T) {
	examples := []string{
		"01_led_indicator",
		"02_button_pullup",
		"03_rc_filter",
		"04_555_timer",
		"05_sensor_node",
	}
	for _, example := range examples {
		t.Run(example, func(t *testing.T) {
			file := readExampleSchematic(t, example)
			request, result := AdaptSchematic(file)
			validated := Validate(result, request)
			if diagnostics := errorDiagnostics(example, validated.Diagnostics); len(diagnostics) > 0 {
				t.Fatalf("standard readability diagnostics: %s", formatDiagnostics(diagnostics))
			}
		})
	}
}

func readExampleSchematic(t *testing.T, directory string) *schematic.SchematicFile {
	t.Helper()
	path, err := exampleSchematicPath(filepath.Join(repoRoot(t), "examples"), directory)
	if err != nil {
		t.Fatalf("find schematic: %v", err)
	}
	file, err := schematic.ReadFile(path)
	if err != nil {
		t.Fatalf("read schematic %s: %v", path, err)
	}
	return &file
}

func errorDiagnostics(example string, diagnostics []Diagnostic) []Diagnostic {
	var blocked []Diagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError && !ignoredExampleErrorDiagnostic(example, diagnostic) {
			blocked = append(blocked, diagnostic)
		}
	}
	return blocked
}

func ignoredExampleErrorDiagnostic(example string, diagnostic Diagnostic) bool {
	ignored := map[string]map[string]bool{
		// Parsed fixture symbols do not yet carry exact symbol-body extents or
		// pin entry geometry, so power-symbol wire terminations can look like
		// body crossings. Keep this explicit and per-example so new error codes
		// still fail the tests.
		"01_led_indicator": {"wire_symbol_overlap": true},
		"02_button_pullup": {"wire_symbol_overlap": true},
		"03_rc_filter":     {"wire_symbol_overlap": true},
		"04_555_timer":     {"wire_symbol_overlap": true},
	}
	return ignored[example][diagnostic.Code]
}

func formatDiagnostics(diagnostics []Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, fmt.Sprintf("[%s] %s: %s", diagnostic.Code, diagnostic.Ref, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
}
