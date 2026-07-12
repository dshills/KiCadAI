package pcb

import (
	"fmt"
	"os"
	"sort"
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
		case "arc":
			board.TrackArcs = append(board.TrackArcs, readTrackArc(child))
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
	resolvePCBNetReferences(&board)
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
	if at, ok := node.Child("at"); ok {
		if rotation, ok := at.FloatValue(3); ok {
			fp.Rotation = kicadfiles.Angle(rotation)
		}
	}
	if layer, ok := node.Child("layer"); ok {
		fp.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	if path, ok := node.Child("path"); ok {
		fp.Path = path.ListValue(1)
	}
	for _, property := range node.ChildrenByHead("property") {
		name := property.ListValue(1)
		value := property.ListValue(2)
		fp.Properties = append(fp.Properties, FootprintProperty{Name: name, Value: value, Position: readPCBAtPoint(property), UUID: readPCBUUID(property)})
		if layer, ok := property.Child("layer"); ok {
			fp.Properties[len(fp.Properties)-1].Layer = kicadfiles.BoardLayer(layer.ListValue(1))
		}
		switch name {
		case "Reference":
			fp.Reference = value
		case "Value":
			fp.Value = value
		}
	}
	for _, pad := range node.ChildrenByHead("pad") {
		parsed := readPad(pad)
		parsed.Rotation = normalizedFootprintAngle(parsed.Rotation - fp.Rotation)
		fp.Pads = append(fp.Pads, parsed)
	}
	return fp
}

func readPad(node sexpr.ParsedNode) Pad {
	pad := Pad{Raw: strings.TrimSpace(node.Raw), Name: node.ListValue(1), Type: node.ListValue(2), Shape: node.ListValue(3), UUID: readPCBUUID(node)}
	pad.Position = readPCBAtPoint(node)
	if at, ok := node.Child("at"); ok {
		if rotation, ok := at.FloatValue(3); ok {
			pad.Rotation = kicadfiles.Angle(rotation)
		}
	}
	if size, ok := node.Child("size"); ok {
		x, xOK := size.FloatValue(1)
		y, yOK := size.FloatValue(2)
		if xOK && yOK {
			pad.Size = kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
		}
	}
	if drill, ok := node.Child("drill"); ok {
		pad.Drill, pad.DrillShape, pad.DrillSize = readPadDrill(drill)
	}
	if layers, ok := node.Child("layers"); ok {
		pad.Layers = readPCBLayerList(layers)
	}
	if ratio, ok := node.Child("roundrect_rratio"); ok {
		pad.RoundRectRRatio, _ = ratio.FloatValue(1)
	}
	if net, ok := node.Child("net"); ok {
		pad.NetCode, pad.NetName = readPCBNetRef(net)
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
		track.NetCode, track.NetName = readPCBNetRef(net)
	}
	return track
}

func readTrackArc(node sexpr.ParsedNode) TrackArc {
	arc := TrackArc{UUID: readPCBUUID(node)}
	arc.Start = readNamedPCBPoint(node, "start")
	arc.Mid = readNamedPCBPoint(node, "mid")
	arc.End = readNamedPCBPoint(node, "end")
	if width, ok := node.Child("width"); ok {
		if value, ok := width.FloatValue(1); ok {
			arc.Width = kicadfiles.MM(value)
		}
	}
	if layer, ok := node.Child("layer"); ok {
		arc.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	if net, ok := node.Child("net"); ok {
		arc.NetCode, arc.NetName = readPCBNetRef(net)
	}
	return arc
}

func readVia(node sexpr.ParsedNode) Via {
	via := Via{UUID: readPCBUUID(node), Position: readPCBAtPoint(node)}
	if size, ok := node.Child("size"); ok {
		value, _ := size.FloatValue(1)
		via.Size = kicadfiles.MM(value)
	}
	if drill, ok := node.Child("drill"); ok {
		value, _ := drill.FloatValue(1)
		via.Drill = kicadfiles.MM(value)
	}
	if layers, ok := node.Child("layers"); ok {
		via.Layers = readPCBLayerList(layers)
	}
	if tenting, ok := node.Child("tenting"); ok {
		via.TentingFront, via.TentingBack = readSidePair(tenting)
	}
	if net, ok := node.Child("net"); ok {
		via.NetCode, via.NetName = readPCBNetRef(net)
	}
	return via
}

func readDrawing(node sexpr.ParsedNode) Drawing {
	drawing := Drawing{UUID: readPCBUUID(node), Kind: strings.TrimPrefix(node.Head(), "gr_")}
	if layer, ok := node.Child("layer"); ok {
		drawing.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
	}
	if fill, ok := node.Child("fill"); ok {
		drawing.Fill = fill.ListValue(1)
	}
	if net, ok := node.Child("net"); ok {
		drawing.NetCode, drawing.NetName = readPCBNetRef(net)
	}
	width, strokeType := readStroke(node)
	drawing.StrokeType = strokeType
	switch drawing.Kind {
	case "line":
		drawing.Line = &LineDrawing{Start: readNamedPCBPoint(node, "start"), End: readNamedPCBPoint(node, "end"), Width: width}
	case "rect":
		drawing.Rect = &RectDrawing{Start: readNamedPCBPoint(node, "start"), End: readNamedPCBPoint(node, "end"), Width: width}
	case "circle":
		drawing.Circle = &CircleDrawing{Center: readNamedPCBPoint(node, "center"), End: readNamedPCBPoint(node, "end"), Width: width}
	case "arc":
		drawing.Arc = &ArcDrawing{Start: readNamedPCBPoint(node, "start"), Mid: readNamedPCBPoint(node, "mid"), End: readNamedPCBPoint(node, "end"), Width: width}
	case "poly":
		drawing.Poly = &PolylineDrawing{Points: readPoints(node), Width: width}
	case "text":
		drawing.Text = &TextDrawing{Text: node.ListValue(1), Position: readPCBAtPoint(node)}
		if at, ok := node.Child("at"); ok {
			if rotation, ok := at.FloatValue(3); ok {
				drawing.Text.Rotation = kicadfiles.Angle(rotation)
			}
		}
	}
	return drawing
}

func readZone(node sexpr.ParsedNode) Zone {
	zone := Zone{Raw: strings.TrimSpace(node.Raw), UUID: readPCBUUID(node)}
	if net, ok := node.Child("net"); ok {
		zone.NetCode, zone.NetName = readPCBNetRef(net)
	}
	if net, ok := node.Child("net_name"); ok && strings.TrimSpace(zone.NetName) == "" {
		zone.NetName = net.ListValue(1)
	}
	if name, ok := node.Child("name"); ok {
		zone.Name = name.ListValue(1)
	}
	if layers, ok := node.Child("layers"); ok {
		zone.Layers = readPCBLayerList(layers)
	}
	if priority, ok := node.Child("priority"); ok {
		value, _ := priority.FloatValue(1)
		zone.Priority = int(value)
	}
	if hatch, ok := node.Child("hatch"); ok {
		zone.HatchStyle = hatch.ListValue(1)
		value, _ := hatch.FloatValue(2)
		zone.HatchPitch = kicadfiles.MM(value)
	}
	if connect, ok := node.Child("connect_pads"); ok {
		zone.ConnectPadsMode = connect.ListValue(1)
		if clearance, ok := connect.Child("clearance"); ok {
			value, _ := clearance.FloatValue(1)
			zone.Clearance = kicadfiles.MM(value)
		}
	}
	if minThickness, ok := node.Child("min_thickness"); ok {
		value, _ := minThickness.FloatValue(1)
		zone.MinThickness = kicadfiles.MM(value)
	}
	if fill, ok := node.Child("fill"); ok {
		zone.Fill = readZoneFill(fill)
	}
	for _, polygon := range node.ChildrenByHead("polygon") {
		zone.Polygons = append(zone.Polygons, readPoints(polygon))
	}
	for _, polygon := range node.ChildrenByHead("filled_polygon") {
		filled := ZoneFilledPolygon{Points: readPoints(polygon)}
		if layer, ok := polygon.Child("layer"); ok {
			filled.Layer = kicadfiles.BoardLayer(layer.ListValue(1))
		}
		zone.FilledPolygons = append(zone.FilledPolygons, filled)
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

func readPoints(node sexpr.ParsedNode) []kicadfiles.Point {
	pts, ok := node.Child("pts")
	if !ok {
		return nil
	}
	xys := pts.ChildrenByHead("xy")
	points := make([]kicadfiles.Point, 0, len(xys))
	for _, xy := range xys {
		x, _ := xy.FloatValue(1)
		y, _ := xy.FloatValue(2)
		points = append(points, kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)})
	}
	return points
}

func readPCBLayerList(node sexpr.ParsedNode) []kicadfiles.BoardLayer {
	layers := make([]kicadfiles.BoardLayer, 0, max(0, len(node.Children)-1))
	for _, child := range node.Children[1:] {
		layers = append(layers, kicadfiles.BoardLayer(child.Value()))
	}
	return layers
}

func readPadDrill(node sexpr.ParsedNode) (kicadfiles.IU, string, kicadfiles.Point) {
	if value, ok := node.FloatValue(1); ok {
		drill := kicadfiles.MM(value)
		return drill, "", kicadfiles.Point{X: drill, Y: drill}
	}
	if len(node.Children) > 1 && node.ListValue(1) == "oval" {
		x, xOK := node.FloatValue(2)
		y, yOK := node.FloatValue(3)
		if xOK && yOK && (x <= 0 || y <= 0) {
			return 0, "oval", kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
		}
		if xOK || yOK {
			size := kicadfiles.Point{}
			if xOK {
				size.X = kicadfiles.MM(x)
			}
			if yOK {
				size.Y = kicadfiles.MM(y)
			}
			if size.X == 0 {
				size.X = size.Y
			}
			if size.Y == 0 {
				size.Y = size.X
			}
			return smallestPositiveIU(size.X, size.Y), "oval", size
		}
	}
	return 0, "", kicadfiles.Point{}
}

func smallestPositiveIU(a, b kicadfiles.IU) kicadfiles.IU {
	switch {
	case a <= 0 && b <= 0:
		return 0
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func readPCBNetRef(node sexpr.ParsedNode) (int, string) {
	if len(node.Children) < 2 {
		return 0, ""
	}
	if code, ok := node.FloatValue(1); ok {
		if len(node.Children) > 2 {
			return int(code), node.ListValue(2)
		}
		return int(code), ""
	}
	return 0, node.ListValue(1)
}

func readStroke(node sexpr.ParsedNode) (kicadfiles.IU, string) {
	stroke, ok := node.Child("stroke")
	if !ok {
		return 0, ""
	}
	var width kicadfiles.IU
	if widthNode, ok := stroke.Child("width"); ok {
		value, _ := widthNode.FloatValue(1)
		width = kicadfiles.MM(value)
	}
	strokeType := ""
	if typeNode, ok := stroke.Child("type"); ok {
		strokeType = typeNode.ListValue(1)
	}
	return width, strokeType
}

func readSidePair(node sexpr.ParsedNode) (bool, bool) {
	var front, back bool
	if frontNode, ok := node.Child("front"); ok {
		front = frontNode.ListValue(1) == "yes"
	}
	if backNode, ok := node.Child("back"); ok {
		back = backNode.ListValue(1) == "yes"
	}
	return front, back
}

func readZoneFill(node sexpr.ParsedNode) ZoneFillSettings {
	fill := ZoneFillSettings{Enabled: node.ListValue(1) == "yes"}
	if thermalGap, ok := node.Child("thermal_gap"); ok {
		value, _ := thermalGap.FloatValue(1)
		fill.ThermalGap = kicadfiles.MM(value)
	}
	if bridge, ok := node.Child("thermal_bridge_width"); ok {
		value, _ := bridge.FloatValue(1)
		fill.ThermalBridgeWidth = kicadfiles.MM(value)
	}
	if islandMode, ok := node.Child("island_removal_mode"); ok {
		value, _ := islandMode.FloatValue(1)
		fill.IslandRemovalMode = int(value)
	}
	if islandArea, ok := node.Child("island_area_min"); ok {
		fill.IslandAreaMin, _ = islandArea.FloatValue(1)
	}
	return fill
}

func resolvePCBNetReferences(board *PCBFile) {
	if board == nil {
		return
	}
	addMissingNamedNetReferences(board)
	namesByCode := make(map[int]string, len(board.Nets))
	codesByName := make(map[string]int, len(board.Nets))
	for _, net := range board.Nets {
		name := strings.TrimSpace(net.Name)
		namesByCode[net.Code] = name
		codesByName[name] = net.Code
	}
	resolve := func(code *int, name *string) {
		trimmedName := strings.TrimSpace(*name)
		if trimmedName != *name {
			*name = trimmedName
		}
		if *code == 0 && trimmedName != "" {
			if resolved, ok := codesByName[trimmedName]; ok {
				*code = resolved
				*name = trimmedName
			}
		}
		if trimmedName == "" {
			if resolved, ok := namesByCode[*code]; ok {
				*name = resolved
			}
		}
	}
	for footprintIndex := range board.Footprints {
		for padIndex := range board.Footprints[footprintIndex].Pads {
			pad := &board.Footprints[footprintIndex].Pads[padIndex]
			resolve(&pad.NetCode, &pad.NetName)
		}
	}
	for i := range board.Tracks {
		resolve(&board.Tracks[i].NetCode, &board.Tracks[i].NetName)
	}
	for i := range board.Vias {
		resolve(&board.Vias[i].NetCode, &board.Vias[i].NetName)
	}
	for i := range board.Zones {
		resolve(&board.Zones[i].NetCode, &board.Zones[i].NetName)
	}
	for i := range board.Drawings {
		resolve(&board.Drawings[i].NetCode, &board.Drawings[i].NetName)
	}
}

func addMissingNamedNetReferences(board *PCBFile) {
	if board == nil {
		return
	}
	existingNames := map[string]struct{}{}
	maxCode := 0
	for _, net := range board.Nets {
		name := strings.TrimSpace(net.Name)
		existingNames[name] = struct{}{}
		if net.Code > maxCode {
			maxCode = net.Code
		}
	}
	missing := map[string]struct{}{}
	add := func(code int, name string) {
		if code > maxCode {
			maxCode = code
		}
		if code != 0 || name == "" {
			return
		}
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		if _, ok := existingNames[trimmed]; ok {
			return
		}
		missing[trimmed] = struct{}{}
	}
	for _, footprint := range board.Footprints {
		for _, pad := range footprint.Pads {
			add(pad.NetCode, pad.NetName)
		}
	}
	for _, track := range board.Tracks {
		add(track.NetCode, track.NetName)
	}
	for _, arc := range board.TrackArcs {
		add(arc.NetCode, arc.NetName)
	}
	for _, via := range board.Vias {
		add(via.NetCode, via.NetName)
	}
	for _, zone := range board.Zones {
		add(zone.NetCode, zone.NetName)
	}
	for _, drawing := range board.Drawings {
		add(drawing.NetCode, drawing.NetName)
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		maxCode++
		board.Nets = append(board.Nets, Net{Code: maxCode, Name: name})
	}
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
