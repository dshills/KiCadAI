package schematiclayout

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

type ExampleAudit struct {
	Name                 string
	Directory            string
	SchematicPath        string
	SymbolCount          int
	WireCount            int
	LabelCount           int
	JunctionCount        int
	DiagonalWireCount    int
	SymbolsAtOrigin      int
	PinAnchorCount       int
	SymbolsWithPinAnchor int
}

var ScopedExampleDirectories = []string{
	"01_led_indicator",
	"02_button_pullup",
	"03_rc_filter",
	"04_555_timer",
	"05_sensor_node",
	"06_class_ab_headphone_amp",
	"09_class_a_headphone_amp",
	"10_opamp_buffer_headphone_amp",
}

func AuditExampleSchematics(repoRoot string) ([]ExampleAudit, error) {
	examplesRoot := filepath.Join(repoRoot, "examples")
	audits := make([]ExampleAudit, 0, len(ScopedExampleDirectories))
	for _, directory := range ScopedExampleDirectories {
		path, err := exampleSchematicPath(examplesRoot, directory)
		if err != nil {
			return nil, fmt.Errorf("find schematic for %s: %w", directory, err)
		}
		file, err := schematic.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schematic for %s: %w", directory, err)
		}
		audits = append(audits, AuditSchematic(directory, path, &file))
	}
	return audits, nil
}

func AuditSchematic(directory string, path string, file *schematic.SchematicFile) ExampleAudit {
	audit := ExampleAudit{
		Name:          strings.TrimSuffix(filepath.Base(path), ".kicad_sch"),
		Directory:     directory,
		SchematicPath: filepath.ToSlash(path),
		SymbolCount:   len(file.Symbols),
		WireCount:     len(file.Wires),
		LabelCount:    len(file.Labels),
		JunctionCount: len(file.Junctions),
	}
	for _, symbol := range file.Symbols {
		if symbol.Position.X == 0 && symbol.Position.Y == 0 {
			audit.SymbolsAtOrigin++
		}
		if len(symbol.PinAnchors) > 0 {
			audit.SymbolsWithPinAnchor++
			audit.PinAnchorCount += len(symbol.PinAnchors)
		}
	}
	for _, wire := range file.Wires {
		for index := 0; index < len(wire.Points)-1; index++ {
			first := wire.Points[index]
			second := wire.Points[index+1]
			if isDiagonalSegment(first, second) {
				audit.DiagonalWireCount++
			}
		}
	}
	return audit
}

func FormatExampleAuditMarkdown(audits []ExampleAudit) string {
	audits = append([]ExampleAudit(nil), audits...)
	sort.SliceStable(audits, func(i, j int) bool {
		return audits[i].Directory < audits[j].Directory
	})
	var builder strings.Builder
	builder.WriteString("# Schematic Example Readability Audit\n\n")
	builder.WriteString("Date: 2026-06-29\n\n")
	builder.WriteString("This audit captures the baseline parsed schematic structure for the scoped checked-in examples before readability fixture rewrites.\n\n")
	builder.WriteString("| Example | Symbols | Wires | Labels | Junctions | Diagonal wires | Symbols at origin | Symbols with pin anchors | Pin anchors |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, audit := range audits {
		builder.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			audit.Directory,
			audit.SymbolCount,
			audit.WireCount,
			audit.LabelCount,
			audit.JunctionCount,
			audit.DiagonalWireCount,
			audit.SymbolsAtOrigin,
			audit.SymbolsWithPinAnchor,
			audit.PinAnchorCount,
		))
	}
	builder.WriteString("\n## Notes\n\n")
	builder.WriteString("- Diagonal wire counts are derived from parsed wire point pairs only.\n")
	builder.WriteString("- Symbol pin anchors are present when the parser can recover explicit anchor coordinates from generated fixture metadata.\n")
	builder.WriteString("- Later phases add geometry and amplifier-specific readability validation on top of this baseline.\n")
	return builder.String()
}

func exampleSchematicPath(examplesRoot string, directory string) (string, error) {
	dir := filepath.Join(examplesRoot, directory)
	matches, err := filepath.Glob(filepath.Join(dir, "*.kicad_sch"))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected one schematic in %s, found %d", dir, len(matches))
	}
	return matches[0], nil
}

func isDiagonalSegment(first, second kicadfiles.Point) bool {
	return first.X != second.X && first.Y != second.Y
}
