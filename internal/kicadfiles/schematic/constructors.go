package schematic

import "kicadai/internal/kicadfiles"

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
