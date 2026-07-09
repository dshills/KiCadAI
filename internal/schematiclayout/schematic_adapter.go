package schematiclayout

import (
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func AdaptSchematic(file *schematic.SchematicFile) (Request, Result) {
	request := Request{
		Sheet: sheetFromSchematic(file),
		Rules: DefaultRules(ProfileStandard),
	}
	result := Result{}
	if file == nil {
		return request, result
	}
	request.Components = make([]Component, 0, len(file.Symbols))
	result.Components = make([]PlacedComponent, 0, len(file.Symbols))
	result.Wires = make([]WireSegment, 0, len(file.Wires))
	result.Labels = make([]Label, 0, len(file.Labels))
	result.Junctions = make([]Junction, 0, len(file.Junctions))
	for index, symbol := range file.Symbols {
		component := Component{
			Ref:             symbol.Reference,
			Value:           symbol.Value,
			LibraryID:       symbol.LibraryID,
			Position:        symbol.Position,
			OriginalOrdinal: index,
		}
		component.Role = InferComponentRole(component)
		component.Stage = StageForRole(component.Role)
		component.Lane = LaneForRole(component.Role)
		component.Pins = pinsFromSymbol(symbol)
		placed := PlacedComponent{Component: component, PlacedAt: symbol.Position}
		placed.ReferenceText = textBoxFromProperty(symbol.Properties, "Reference", symbol.Position)
		placed.ValueText = textBoxFromProperty(symbol.Properties, "Value", symbol.Position)
		result.Components = append(result.Components, placed)
		request.Components = append(request.Components, component)
	}
	for _, wire := range file.Wires {
		for index := 0; index < len(wire.Points)-1; index++ {
			result.Wires = append(result.Wires, WireSegment{From: wire.Points[index], To: wire.Points[index+1]})
		}
	}
	for _, label := range file.Labels {
		result.Labels = append(result.Labels, Label{NetName: label.Text, Text: label.Text, Position: label.Position})
	}
	for _, junction := range file.Junctions {
		result.Junctions = append(result.Junctions, Junction{Position: junction.Position})
	}
	return request, NormalizeResult(result, request.Rules)
}

func sheetFromSchematic(file *schematic.SchematicFile) Sheet {
	if file == nil {
		return SheetForPaper("A4")
	}
	return SheetForPaper(file.Paper.Name)
}

func pinsFromSymbol(symbol schematic.SchematicSymbol) []Pin {
	pins := make([]Pin, 0, len(symbol.Pins))
	for index, pin := range symbol.Pins {
		relative := kicadfiles.Point{}
		if index < len(symbol.PinAnchors) {
			relative = kicadfiles.Point{X: symbol.PinAnchors[index].X - symbol.Position.X, Y: symbol.PinAnchors[index].Y - symbol.Position.Y}
		}
		pins = append(pins, Pin{Number: pin.Number, At: relative})
	}
	return pins
}

func textBoxFromProperty(properties []schematic.Property, name string, origin kicadfiles.Point) TextBox {
	for _, property := range properties {
		if property.Name != name || property.Hidden {
			continue
		}
		position := property.Position
		if position.X == 0 && position.Y == 0 {
			position = origin
		}
		box := TextEstimate(property.Value, position, 0, 0)
		return TextBox{Text: property.Value, Box: Rect{MinX: box.MinX - origin.X, MinY: box.MinY - origin.Y, MaxX: box.MaxX - origin.X, MaxY: box.MaxY - origin.Y}}
	}
	return TextBox{}
}
