package schematic

import (
	"strings"

	"kicadai/internal/kicadfiles"
)

// NewSymbol initializes the required symbol identity fields. Leave Properties
// empty to let Write derive standard Reference and Value properties, or set
// Properties explicitly when custom property flags and positions are needed.
func NewSymbol(uuid kicadfiles.UUID, libraryID, reference, value string, position kicadfiles.Point) SchematicSymbol {
	return SchematicSymbol{
		UUID:      uuid,
		LibraryID: libraryID,
		Reference: reference,
		Value:     value,
		Position:  position,
	}
}

func NewProperty(name, value string, position kicadfiles.Point) Property {
	return Property{
		Name:     name,
		Value:    value,
		Position: position,
	}
}

func CloneProperties(source []Property) []Property {
	if source == nil {
		return nil
	}
	return clonePropertiesWithCapacity(source, len(source))
}

func clonePropertiesWithCapacity(source []Property, capacity int) []Property {
	if capacity < len(source) {
		capacity = len(source)
	}
	clone := make([]Property, 0, capacity)
	for _, property := range source {
		clone = append(clone, cloneProperty(property))
	}
	return clone
}

func MergeProperties(base []Property, incoming []Property) []Property {
	merged := clonePropertiesWithCapacity(base, len(base)+len(incoming))
	if len(incoming) == 0 {
		return merged
	}
	byName := make(map[string]int, len(merged))
	for i := range merged {
		byName[NormalizePropertyName(merged[i].Name)] = i
	}
	for _, property := range incoming {
		clone := cloneProperty(property)
		name := NormalizePropertyName(clone.Name)
		if index, ok := byName[name]; ok {
			clone.Name = merged[index].Name
			merged[index] = clone
			continue
		}
		byName[name] = len(merged)
		merged = append(merged, clone)
	}
	return merged
}

func NormalizePropertyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneProperty(source Property) Property {
	clone := source
	clone.ShowName = CloneBool(source.ShowName)
	clone.DoNotAutoplace = CloneBool(source.DoNotAutoplace)
	return clone
}

func CloneBool(source *bool) *bool {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func NewWire(uuid kicadfiles.UUID, start, end kicadfiles.Point) Wire {
	return Wire{
		UUID:   uuid,
		Points: []kicadfiles.Point{start, end},
	}
}

func NewLabel(uuid kicadfiles.UUID, text string, kind LabelKind, position kicadfiles.Point) Label {
	return Label{
		UUID:     uuid,
		Text:     text,
		Kind:     kind,
		Position: position,
	}
}

func NewNoConnect(uuid kicadfiles.UUID, position kicadfiles.Point) NoConnect {
	return NoConnect{
		UUID:     uuid,
		Position: position,
	}
}

func NewSheet(uuid kicadfiles.UUID, name, filename string, position, size kicadfiles.Point) Sheet {
	return Sheet{
		UUID:     uuid,
		Name:     name,
		Filename: filename,
		Position: position,
		Size:     size,
	}
}

func NewSheetPin(uuid kicadfiles.UUID, text string, kind SheetPinKind, position kicadfiles.Point) SheetPin {
	return SheetPin{
		UUID:     uuid,
		Text:     text,
		Kind:     kind,
		Position: position,
	}
}
