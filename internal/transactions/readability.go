package transactions

import (
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func validateWrittenSchematicReadability(paths []string) error {
	checked := 0
	for _, path := range paths {
		if !strings.EqualFold(filepath.Ext(path), ".kicad_sch") {
			continue
		}
		checked++
		file, err := schematic.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read generated schematic %s for readability validation: %w", path, err)
		}
		request, result := schematiclayout.AdaptSchematic(&file)
		result = schematiclayout.Validate(result, request)
		report := schematiclayout.BuildReport(result, schematiclayout.ProfileStrict)
		if !report.Passed {
			return fmt.Errorf("generated schematic %s failed readability validation: errors=%d warnings=%d diagnostics=%v", path, report.ErrorCount, report.WarningCount, result.Diagnostics)
		}
	}
	if checked == 0 {
		return fmt.Errorf("readability validation requested but no generated schematic was written")
	}
	return nil
}
