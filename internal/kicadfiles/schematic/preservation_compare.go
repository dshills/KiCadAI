package schematic

import (
	"fmt"
	"reflect"

	"kicadai/internal/kicadfiles"
)

// ValidatePreservedMutation verifies that staged imported schematic output
// retains every raw-preserved construct from the source.
func ValidatePreservedMutation(source, staged SchematicFile) error {
	if source.RawPaper != "" && source.RawPaper != staged.RawPaper {
		return fmt.Errorf("imported schematic preservation mismatch: paper was substituted")
	}
	if source.RawTitleBlock != "" && source.RawTitleBlock != staged.RawTitleBlock {
		return fmt.Errorf("imported schematic preservation mismatch: title_block was substituted")
	}
	if !equalRawSchematicItems(source.RawItems, staged.RawItems) {
		return fmt.Errorf("imported schematic preservation mismatch: raw items changed")
	}
	stagedJunctions := make(map[kicadfiles.UUID]Junction, len(staged.Junctions))
	for _, junction := range staged.Junctions {
		stagedJunctions[junction.UUID] = junction
	}
	for _, before := range source.Junctions {
		after, ok := stagedJunctions[before.UUID]
		if !ok || before.Raw != after.Raw {
			return fmt.Errorf("imported schematic preservation mismatch: junction %s changed", before.UUID)
		}
	}
	stagedSheets := make(map[kicadfiles.UUID]Sheet, len(staged.Sheets))
	for _, sheet := range staged.Sheets {
		stagedSheets[sheet.UUID] = sheet
	}
	for _, before := range source.Sheets {
		after, ok := stagedSheets[before.UUID]
		if !ok || before.Raw != after.Raw {
			return fmt.Errorf("imported schematic preservation mismatch: sheet %s changed", before.UUID)
		}
	}
	return nil
}

func equalRawSchematicItems(source, staged []RawSchematicItem) bool {
	type rawIdentity struct {
		UUID kicadfiles.UUID
		Kind RawSchematicItemKind
		Body string
	}
	sourceItems := make([]rawIdentity, 0, len(source))
	for _, item := range source {
		sourceItems = append(sourceItems, rawIdentity{UUID: item.UUID, Kind: item.Kind, Body: string(item.Body)})
	}
	stagedItems := make([]rawIdentity, 0, len(staged))
	for _, item := range staged {
		stagedItems = append(stagedItems, rawIdentity{UUID: item.UUID, Kind: item.Kind, Body: string(item.Body)})
	}
	return reflect.DeepEqual(sourceItems, stagedItems)
}
