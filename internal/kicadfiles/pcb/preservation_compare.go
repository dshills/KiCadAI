package pcb

import (
	"fmt"
	"reflect"

	"kicadai/internal/kicadfiles"
)

// ValidatePreservedMutation compares reader-owned preservation evidence from an
// imported source board with the staged board that would replace it.
func ValidatePreservedMutation(source, staged PCBFile) error {
	for _, section := range []struct {
		name   string
		source string
		staged string
	}{
		{"general", source.RawGeneral, staged.RawGeneral},
		{"paper", source.RawPaper, staged.RawPaper},
		{"title_block", source.RawTitleBlock, staged.RawTitleBlock},
		{"setup", source.RawSetup, staged.RawSetup},
	} {
		if section.source != "" && section.source != section.staged {
			return fmt.Errorf("imported PCB preservation mismatch: %s was substituted", section.name)
		}
	}
	if !reflect.DeepEqual(source.Preserved, staged.Preserved) {
		return fmt.Errorf("imported PCB preservation mismatch: top-level raw nodes changed")
	}
	stagedFootprints := make(map[kicadfiles.UUID]Footprint, len(staged.Footprints))
	for _, footprint := range staged.Footprints {
		stagedFootprints[footprint.UUID] = footprint
	}
	for _, before := range source.Footprints {
		after, ok := stagedFootprints[before.UUID]
		if !ok {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s disappeared", before.UUID)
		}
		if before.Description != after.Description || before.Tags != after.Tags || before.Locked != after.Locked {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s metadata changed", before.UUID)
		}
		if !reflect.DeepEqual(before.Texts, after.Texts) {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s text changed", before.UUID)
		}
		if !reflect.DeepEqual(before.Models, after.Models) {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s models changed", before.UUID)
		}
		if !reflect.DeepEqual(before.Preserved, after.Preserved) {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s raw children changed: before=%#v after=%#v", before.UUID, before.Preserved, after.Preserved)
		}
		if err := compareFootprintPropertyPresentation(before, after); err != nil {
			return fmt.Errorf("imported PCB preservation mismatch: footprint %s: %w", before.UUID, err)
		}
	}
	stagedZones := make(map[kicadfiles.UUID]Zone, len(staged.Zones))
	for _, zone := range staged.Zones {
		stagedZones[zone.UUID] = zone
	}
	for _, before := range source.Zones {
		after, ok := stagedZones[before.UUID]
		if !ok {
			return fmt.Errorf("imported PCB preservation mismatch: zone %s disappeared", before.UUID)
		}
		if !reflect.DeepEqual(before.Keepout, after.Keepout) ||
			!reflect.DeepEqual(before.Attributes, after.Attributes) ||
			!reflect.DeepEqual(before.Preserved, after.Preserved) {
			return fmt.Errorf("imported PCB preservation mismatch: zone %s changed semantic family", before.UUID)
		}
	}
	return nil
}

func compareFootprintPropertyPresentation(source, staged Footprint) error {
	afterByName := make(map[string]FootprintProperty, len(staged.Properties))
	for _, property := range staged.Properties {
		afterByName[property.Name] = property
	}
	for _, before := range source.Properties {
		after, ok := afterByName[before.Name]
		if !ok {
			return fmt.Errorf("property %q disappeared", before.Name)
		}
		if before.UUID != after.UUID || before.Position != after.Position || before.Rotation != after.Rotation ||
			before.Layer != after.Layer || before.Hide != after.Hide || before.Unlocked != after.Unlocked ||
			!reflect.DeepEqual(before.Effects, after.Effects) {
			return fmt.Errorf("property %q presentation changed", before.Name)
		}
	}
	return nil
}
