package schematic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func ReadFile(path string) (SchematicFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SchematicFile{}, err
	}
	file, err := Read(data)
	if err != nil {
		return SchematicFile{}, fmt.Errorf("%s: %w", path, err)
	}
	file.Filename = filepath.Base(path)
	return file, nil
}

func Read(data []byte) (SchematicFile, error) {
	root, err := sexpr.Parse(data)
	if err != nil {
		return SchematicFile{}, err
	}
	if root.Head() != "kicad_sch" {
		return SchematicFile{}, fmt.Errorf("expected kicad_sch root, got %q", root.Head())
	}
	file := SchematicFile{}
	if node, ok := root.Child("version"); ok {
		file.Version = kicadfiles.KiCadFormatVersion(node.ListValue(1))
	}
	if node, ok := root.Child("generator"); ok {
		file.Generator = node.ListValue(1)
	}
	if node, ok := root.Child("generator_version"); ok {
		file.GeneratorVersion = node.ListValue(1)
	}
	if node, ok := root.Child("uuid"); ok {
		file.UUID = kicadfiles.UUID(node.ListValue(1))
	}
	if node, ok := root.Child("paper"); ok {
		file.Paper = kicadfiles.Paper{Name: node.ListValue(1)}
	}
	for i, child := range root.Children {
		if i == 0 {
			continue
		}
		switch child.Head() {
		case "symbol":
			file.Symbols = append(file.Symbols, readSymbol(child))
		case "wire":
			file.Wires = append(file.Wires, Wire{UUID: readUUID(child), Points: readPoints(child)})
		case "label":
			file.Labels = append(file.Labels, readLabel(child, LabelLocal))
		case "global_label":
			file.Labels = append(file.Labels, readLabel(child, LabelGlobal))
		case "hierarchical_label":
			file.Labels = append(file.Labels, readLabel(child, LabelHierarchical))
		case "junction":
			file.Junctions = append(file.Junctions, Junction{UUID: readUUID(child), Position: readAtPoint(child)})
		case "sheet":
			file.RawItems = append(file.RawItems, rawItem(child, i))
		case "lib_symbols":
			file.LibSymbols = readLibSymbols(child)
		case "version", "generator", "generator_version", "uuid", "paper", "title_block", "sheet_instances":
		default:
			if child.Head() != "" {
				file.RawItems = append(file.RawItems, rawItem(child, i))
			}
		}
	}
	return file, nil
}

func readLibSymbols(node sexpr.ParsedNode) []EmbeddedSymbol {
	var symbols []EmbeddedSymbol
	for _, child := range node.Children[1:] {
		body, _ := child.Node().(sexpr.List)
		symbols = append(symbols, EmbeddedSymbol{LibraryID: child.ListValue(1), Body: body})
	}
	return symbols
}

func readSymbol(node sexpr.ParsedNode) SchematicSymbol {
	symbol := SchematicSymbol{Raw: strings.TrimSpace(node.Raw), UUID: readUUID(node), Position: readAtPoint(node)}
	if at, ok := node.Child("at"); ok {
		if rotation, ok := at.FloatValue(3); ok {
			symbol.Rotation = kicadfiles.Angle(rotation)
		}
	}
	if lib, ok := node.Child("lib_id"); ok {
		symbol.LibraryID = lib.ListValue(1)
	}
	for _, prop := range node.ChildrenByHead("property") {
		property := Property{Name: prop.ListValue(1), Value: prop.ListValue(2), Position: readAtPoint(prop)}
		symbol.Properties = append(symbol.Properties, property)
		switch property.Name {
		case "Reference":
			symbol.Reference = property.Value
		case "Value":
			symbol.Value = property.Value
		}
	}
	for _, pin := range node.ChildrenByHead("pin") {
		symbol.Pins = append(symbol.Pins, SymbolPin{Number: pin.ListValue(1), UUID: readUUID(pin)})
	}
	return symbol
}

func readLabel(node sexpr.ParsedNode, kind LabelKind) Label {
	return Label{Kind: kind, Text: node.ListValue(1), UUID: readUUID(node), Position: readAtPoint(node)}
}

func rawItem(node sexpr.ParsedNode, order int) RawSchematicItem {
	return RawSchematicItem{UUID: readUUID(node), Order: order, Kind: RawSchematicItemKind(node.Head()), Body: sexpr.Raw(strings.TrimSpace(node.Raw))}
}

func readUUID(node sexpr.ParsedNode) kicadfiles.UUID {
	if child, ok := node.Child("uuid"); ok {
		return kicadfiles.UUID(child.ListValue(1))
	}
	return ""
}

func readAtPoint(node sexpr.ParsedNode) kicadfiles.Point {
	at, ok := node.Child("at")
	if !ok {
		return kicadfiles.Point{}
	}
	x, _ := at.FloatValue(1)
	y, _ := at.FloatValue(2)
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
}

func readPoints(node sexpr.ParsedNode) []kicadfiles.Point {
	pts, ok := node.Child("pts")
	if !ok {
		return nil
	}
	var points []kicadfiles.Point
	for _, xy := range pts.ChildrenByHead("xy") {
		x, _ := xy.FloatValue(1)
		y, _ := xy.FloatValue(2)
		points = append(points, kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)})
	}
	return points
}
