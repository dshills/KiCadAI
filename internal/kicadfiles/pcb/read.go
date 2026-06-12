package pcb

import (
	"fmt"
	"os"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func ReadFile(path string) (PCBFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PCBFile{}, err
	}
	file, err := Read(data)
	if err != nil {
		return PCBFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

func Read(data []byte) (PCBFile, error) {
	root, err := sexpr.Parse(data)
	if err != nil {
		return PCBFile{}, err
	}
	if root.Head() != "kicad_pcb" {
		return PCBFile{}, fmt.Errorf("expected kicad_pcb root, got %q", root.Head())
	}
	if err := validatePCBNumberNodes(root); err != nil {
		return PCBFile{}, err
	}
	board := PCBFile{Layers: DefaultTwoLayerStack(), General: DefaultGeneral(), Setup: DefaultSetup()}
	if node, ok := root.Child("version"); ok {
		board.Version = kicadfiles.KiCadFormatVersion(node.ListValue(1))
	}
	if node, ok := root.Child("generator"); ok {
		board.Generator = node.ListValue(1)
	}
	if node, ok := root.Child("generator_version"); ok {
		board.GeneratorVersion = node.ListValue(1)
	}
	if node, ok := root.Child("paper"); ok {
		board.Paper = kicadfiles.Paper{Name: node.ListValue(1)}
	}
	if layers, ok := root.Child("layers"); ok {
		board.Layers = readLayers(layers)
	}
	for i, child := range root.Children {
		if i == 0 {
			continue
		}
		switch child.Head() {
		case "net":
			code, _ := child.FloatValue(1)
			board.Nets = append(board.Nets, Net{Code: int(code), Name: child.ListValue(2)})
		case "footprint":
			board.Footprints = append(board.Footprints, readFootprint(child))
		case "segment":
			board.Tracks = append(board.Tracks, readTrack(child))
		case "via":
			board.Vias = append(board.Vias, readVia(child))
		case "gr_line", "gr_rect", "gr_circle", "gr_arc", "gr_poly", "gr_text":
			board.Drawings = append(board.Drawings, readDrawing(child))
		case "zone":
			board.Zones = append(board.Zones, readZone(child))
		case "version", "generator", "generator_version", "general", "paper", "title_block", "layers", "setup":
		default:
			if child.Head() != "" {
				board.Preserved = append(board.Preserved, PreservedNode{Family: child.Head(), Raw: strings.TrimSpace(child.Raw)})
			}
		}
	}
	return board, nil
}

func readLayers(node sexpr.ParsedNode) []LayerDefinition {
	var layers []LayerDefinition
	for _, child := range node.Children[1:] {
		number, _ := child.FloatValue(0)
		layers = append(layers, LayerDefinition{Number: int(number), Name: kicadfiles.BoardLayer(child.ListValue(1)), Kind: child.ListValue(2), DisplayName: child.ListValue(3)})
	}
	return layers
}

func readFootprint(node sexpr.ParsedNode) Footprint {
	fp := Footprint{Raw: strings.TrimSpace(node.Raw), LibraryID: node.ListValue(1), UUID: readPCBUUID(node), Position: readPCBAtPoint(node)}
	if layer, ok := node.Child("layer"); ok {
		fp.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	for _, property := range node.ChildrenByHead("property") {
		name := property.ListValue(1)
		value := property.ListValue(2)
		fp.Properties = append(fp.Properties, FootprintProperty{Name: name, Value: value, Position: readPCBAtPoint(property), UUID: readPCBUUID(property)})
		switch name {
		case "Reference":
			fp.Reference = value
		case "Value":
			fp.Value = value
		}
	}
	for _, pad := range node.ChildrenByHead("pad") {
		fp.Pads = append(fp.Pads, readPad(pad))
	}
	return fp
}

func readPad(node sexpr.ParsedNode) Pad {
	pad := Pad{Raw: strings.TrimSpace(node.Raw), Name: node.ListValue(1), Type: node.ListValue(2), Shape: node.ListValue(3), UUID: readPCBUUID(node)}
	pad.Position = readPCBAtPoint(node)
	if net, ok := node.Child("net"); ok {
		pad.NetName = net.ListValue(1)
	}
	return pad
}

func readTrack(node sexpr.ParsedNode) Track {
	track := Track{UUID: readPCBUUID(node)}
	track.Start = readNamedPCBPoint(node, "start")
	track.End = readNamedPCBPoint(node, "end")
	if width, ok := node.Child("width"); ok {
		value, _ := width.FloatValue(1)
		track.Width = kicadfiles.MM(value)
	}
	if layer, ok := node.Child("layer"); ok {
		track.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	if net, ok := node.Child("net"); ok {
		track.NetName = net.ListValue(1)
	}
	return track
}

func readVia(node sexpr.ParsedNode) Via {
	via := Via{UUID: readPCBUUID(node), Position: readPCBAtPoint(node)}
	if net, ok := node.Child("net"); ok {
		via.NetName = net.ListValue(1)
	}
	return via
}

func readDrawing(node sexpr.ParsedNode) Drawing {
	drawing := Drawing{UUID: readPCBUUID(node), Kind: strings.TrimPrefix(node.Head(), "gr_")}
	if layer, ok := node.Child("layer"); ok {
		drawing.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	if drawing.Kind == "line" {
		drawing.Line = &LineDrawing{Start: readNamedPCBPoint(node, "start"), End: readNamedPCBPoint(node, "end")}
	}
	return drawing
}

func readZone(node sexpr.ParsedNode) Zone {
	zone := Zone{Raw: strings.TrimSpace(node.Raw), UUID: readPCBUUID(node)}
	if net, ok := node.Child("net_name"); ok {
		zone.NetName = net.ListValue(1)
	}
	if name, ok := node.Child("name"); ok {
		zone.Name = name.ListValue(1)
	}
	return zone
}

func readPCBUUID(node sexpr.ParsedNode) kicadfiles.UUID {
	if child, ok := node.Child("uuid"); ok {
		return kicadfiles.UUID(child.ListValue(1))
	}
	return ""
}

func readPCBAtPoint(node sexpr.ParsedNode) kicadfiles.Point {
	return readNamedPCBPoint(node, "at")
}

func readNamedPCBPoint(node sexpr.ParsedNode, name string) kicadfiles.Point {
	child, ok := node.Child(name)
	if !ok {
		return kicadfiles.Point{}
	}
	x, _ := child.FloatValue(1)
	y, _ := child.FloatValue(2)
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
}

func validatePCBNumberNodes(node sexpr.ParsedNode) error {
	switch node.Head() {
	case "at", "start", "end", "center", "mid", "xy":
		if len(node.Children) < 3 {
			return fmt.Errorf("%s requires x and y coordinates", node.Head())
		}
		if _, ok := node.FloatValue(1); !ok {
			return fmt.Errorf("%s x coordinate must be numeric", node.Head())
		}
		if _, ok := node.FloatValue(2); !ok {
			return fmt.Errorf("%s y coordinate must be numeric", node.Head())
		}
	}
	for _, child := range node.Children {
		if err := validatePCBNumberNodes(child); err != nil {
			return err
		}
	}
	return nil
}
