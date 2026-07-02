package schematic

import (
	"fmt"
	"math"
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
		case "no_connect":
			file.NoConnects = append(file.NoConnects, NoConnect{UUID: readUUID(child), Position: readAtPoint(child)})
		case "sheet":
			file.Sheets = append(file.Sheets, readSheet(child))
		case "lib_symbols":
			file.LibSymbols = readLibSymbols(child)
		case "sheet_instances":
			file.SheetInstances = readRootSheetInstances(child)
		case "version", "generator", "generator_version", "uuid", "paper", "title_block":
		default:
			if child.Head() != "" {
				file.RawItems = append(file.RawItems, rawItem(child, i))
			}
		}
	}
	return file, nil
}

func readSheet(node sexpr.ParsedNode) Sheet {
	sheet := Sheet{UUID: readUUID(node), Position: readAtPoint(node)}
	if size, ok := node.Child("size"); ok {
		x, xOK := size.FloatValue(1)
		y, yOK := size.FloatValue(2)
		if xOK && yOK {
			sheet.Size = kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
		}
	}
	for _, property := range node.ChildrenByHead("property") {
		if len(property.Children) < 3 {
			continue
		}
		prop := readProperty(property)
		if prop.Name == "" {
			continue
		}
		sheet.Properties = append(sheet.Properties, prop)
		switch prop.Name {
		case "Sheetname":
			sheet.Name = prop.Value
		case "Sheetfile":
			sheet.Filename = prop.Value
		}
	}
	for _, pin := range node.ChildrenByHead("pin") {
		if sheetPin, ok := readSheetPin(pin); ok {
			sheet.Pins = append(sheet.Pins, sheetPin)
		}
	}
	if instances, ok := node.Child("instances"); ok {
		sheet.Instances = readNestedSheetInstances(instances)
	}
	return sheet
}

func readSheetPin(node sexpr.ParsedNode) (SheetPin, bool) {
	text := node.ListValue(1)
	kind := SheetPinKind(node.ListValue(2))
	if text == "" || kind == "" {
		return SheetPin{}, false
	}
	pin := SheetPin{
		Text: text,
		Kind: kind,
		UUID: readUUID(node),
	}
	if at, ok := node.Child("at"); ok {
		x, xOK := at.FloatValue(1)
		y, yOK := at.FloatValue(2)
		if xOK && yOK {
			pin.Position = kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
		}
		if rotation, ok := at.FloatValue(3); ok {
			pin.Rotation = kicadfiles.Angle(rotation)
		}
	}
	return pin, true
}

func readRootSheetInstances(node sexpr.ParsedNode) []SheetInstance {
	var instances []SheetInstance
	for _, pathNode := range node.ChildrenByHead("path") {
		instances = append(instances, readSheetInstancePath("", pathNode))
	}
	return instances
}

func readNestedSheetInstances(node sexpr.ParsedNode) []SheetInstance {
	var instances []SheetInstance
	for _, project := range node.ChildrenByHead("project") {
		projectName := project.ListValue(1)
		for _, pathNode := range project.ChildrenByHead("path") {
			instances = append(instances, readSheetInstancePath(projectName, pathNode))
		}
	}
	return instances
}

func readSheetInstancePath(project string, node sexpr.ParsedNode) SheetInstance {
	path := node.ListValue(1)
	if path == "" {
		return SheetInstance{Project: project}
	}
	instance := SheetInstance{Project: project, Path: path}
	if page, ok := node.Child("page"); ok {
		instance.Page = page.ListValue(1)
	}
	return instance
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
	if mirror, ok := node.Child("mirror"); ok {
		symbol.Mirror = SymbolMirror(mirror.ListValue(1))
	}
	if unit, ok := node.Child("unit"); ok {
		if value, ok := unit.FloatValue(1); ok {
			symbol.Unit = int(value)
		}
	}
	if bodyStyle, ok := node.Child("body_style"); ok {
		if value, ok := bodyStyle.FloatValue(1); ok {
			symbol.BodyStyle = int(value)
		}
	}
	if excludeFromSim, ok := readYesNoChild(node, "exclude_from_sim"); ok {
		symbol.ExcludeFromSim = excludeFromSim
	}
	if inBOM, ok := readYesNoChild(node, "in_bom"); ok {
		symbol.InBOM = &inBOM
	}
	if onBoard, ok := readYesNoChild(node, "on_board"); ok {
		symbol.OnBoard = &onBoard
	}
	if inPositionFile, ok := readYesNoChild(node, "in_pos_files"); ok {
		symbol.InPositionFile = &inPositionFile
	}
	if doNotPopulate, ok := readYesNoChild(node, "dnp"); ok {
		symbol.DoNotPopulate = doNotPopulate
	}
	if passthrough, ok := node.Child("passthrough"); ok {
		symbol.Passthrough = SymbolPassthrough(passthrough.ListValue(1))
	}
	if locked, ok := readYesNoChild(node, "locked"); ok {
		symbol.Locked = locked
	}
	if fieldsAutoplaced, ok := readYesNoChild(node, "fields_autoplaced"); ok {
		symbol.FieldsAutoplaced = fieldsAutoplaced
	}
	for _, prop := range node.ChildrenByHead("property") {
		property := readProperty(prop)
		if property.Name == "" {
			continue
		}
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
	if instances, ok := node.Child("instances"); ok {
		symbol.Instances = readSymbolInstances(instances)
	}
	if len(symbol.PinAnchors) == 0 {
		for _, pin := range templatePinsForReadSymbol(symbol) {
			offset := transformedReadPinOffset(pin.Offset, symbol.Rotation, symbol.Mirror)
			symbol.PinAnchors = append(symbol.PinAnchors, kicadfiles.Point{
				X: symbol.Position.X + offset.X,
				Y: symbol.Position.Y + offset.Y,
			})
		}
	}
	return symbol
}

func readSymbolInstances(node sexpr.ParsedNode) []SymbolInstance {
	var instances []SymbolInstance
	for _, project := range node.ChildrenByHead("project") {
		projectName := project.ListValue(1)
		for _, pathNode := range project.ChildrenByHead("path") {
			instance := SymbolInstance{Project: projectName, Path: pathNode.ListValue(1)}
			if reference, ok := pathNode.Child("reference"); ok {
				instance.Reference = reference.ListValue(1)
			}
			if unit, ok := pathNode.Child("unit"); ok {
				if value, ok := unit.FloatValue(1); ok {
					instance.Unit = int(value)
				}
			}
			if value, ok := pathNode.Child("value"); ok {
				instance.Value = value.ListValue(1)
			}
			instances = append(instances, instance)
		}
	}
	return instances
}

func transformedReadPinOffset(offset kicadfiles.Point, rotation kicadfiles.Angle, mirror SymbolMirror) kicadfiles.Point {
	x := float64(offset.X)
	y := float64(offset.Y)
	switch mirror {
	case SymbolMirrorX:
		y = -y
	case SymbolMirrorY:
		x = -x
	}
	if rotation != 0 {
		radians := float64(rotation) * math.Pi / 180
		cosine := math.Cos(radians)
		sine := math.Sin(radians)
		x, y = x*cosine-y*sine, x*sine+y*cosine
	}
	return kicadfiles.Point{X: kicadfiles.IU(math.Round(x)), Y: kicadfiles.IU(math.Round(y))}
}

func templatePinsForReadSymbol(symbol SchematicSymbol) []TemplatePin {
	templatePins, ok := EmbeddedSymbolPinOffsets(symbol.LibraryID)
	if !ok {
		return nil
	}
	if len(symbol.Pins) == 0 {
		return templatePins
	}
	allowed := map[string]struct{}{}
	for _, pin := range symbol.Pins {
		number := strings.TrimSpace(pin.Number)
		if number != "" {
			allowed[number] = struct{}{}
		}
	}
	filtered := make([]TemplatePin, 0, len(templatePins))
	for _, pin := range templatePins {
		if _, ok := allowed[strings.TrimSpace(pin.Number)]; ok {
			filtered = append(filtered, pin)
		}
	}
	return filtered
}

func readYesNoChild(node sexpr.ParsedNode, head string) (bool, bool) {
	child, ok := node.Child(head)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(child.ListValue(1))) {
	case "yes":
		return true, true
	case "no":
		return false, true
	default:
		return false, false
	}
}

func readLabel(node sexpr.ParsedNode, kind LabelKind) Label {
	label := Label{Kind: kind, Text: node.ListValue(1), UUID: readUUID(node), Position: readAtPoint(node)}
	if _, rotation, ok := readAt(node); ok {
		label.Rotation = rotation
	}
	if shape, ok := node.Child("shape"); ok {
		label.Shape = LabelShape(shape.ListValue(1))
	}
	if locked, ok := readYesNoChild(node, "locked"); ok {
		label.Locked = locked
	}
	if fieldsAutoplaced, ok := readYesNoChild(node, "fields_autoplaced"); ok {
		label.FieldsAutoplaced = fieldsAutoplaced
	}
	return label
}

func rawItem(node sexpr.ParsedNode, order int) RawSchematicItem {
	kindName := RawSchematicItemKind(node.Head())
	kind, ok := rawSchematicItemKind(kindName)
	sortOrder := int64(order)
	if ok {
		sortOrder = int64(kind)*schematicItemOrderStride + int64(order)
	}
	return RawSchematicItem{UUID: readUUID(node), Order: sortOrder, Kind: kindName, Body: sexpr.Raw(strings.TrimSpace(node.Raw))}
}

func readUUID(node sexpr.ParsedNode) kicadfiles.UUID {
	if child, ok := node.Child("uuid"); ok {
		return kicadfiles.UUID(child.ListValue(1))
	}
	return ""
}

func readAtPoint(node sexpr.ParsedNode) kicadfiles.Point {
	point, _, ok := readAt(node)
	if !ok {
		return kicadfiles.Point{}
	}
	return point
}

func readAt(node sexpr.ParsedNode) (kicadfiles.Point, kicadfiles.Angle, bool) {
	at, ok := node.Child("at")
	if !ok {
		return kicadfiles.Point{}, 0, false
	}
	x, _ := at.FloatValue(1)
	y, _ := at.FloatValue(2)
	var rotation float64
	if len(at.Children) > 3 {
		rotation, _ = at.FloatValue(3)
	}
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}, kicadfiles.Angle(rotation), true
}

func readProperty(node sexpr.ParsedNode) Property {
	if len(node.Children) < 2 {
		return Property{}
	}
	nameIndex := 1
	private := false
	if strings.EqualFold(node.ListValue(1), "private") {
		if len(node.Children) < 4 {
			return Property{}
		}
		nameIndex = 2
		private = true
	} else if len(node.Children) < 3 {
		return Property{}
	}
	valueIndex := nameIndex + 1
	if node.Children[nameIndex].IsList || node.Children[valueIndex].IsList {
		return Property{}
	}
	position, rotation, _ := readAt(node)
	property := Property{Private: private, Name: node.ListValue(nameIndex), Value: node.ListValue(valueIndex), Position: position, Rotation: rotation}
	readPropertyFlags(node, &property)
	return property
}

func readPropertyFlags(node sexpr.ParsedNode, property *Property) {
	if readPropertyHidden(node) {
		property.Hidden = true
	}
	if showName, ok := readOptionalBoolProperty(node, "show_name"); ok {
		property.ShowName = &showName
	}
	if doNotAutoplace, ok := readOptionalBoolProperty(node, "do_not_autoplace"); ok {
		property.DoNotAutoplace = &doNotAutoplace
	}
}

func readPropertyHidden(node sexpr.ParsedNode) bool {
	if hide, ok := node.Child("hide"); ok {
		return readKiCadFlag(hide)
	}
	if effects, ok := node.Child("effects"); ok {
		for _, child := range effects.Children {
			if !child.IsList && strings.EqualFold(child.Value(), "hide") {
				return true
			}
			if strings.EqualFold(child.Head(), "hide") {
				return readKiCadFlag(child)
			}
		}
	}
	return false
}

func readKiCadFlag(node sexpr.ParsedNode) bool {
	// ParsedNode.Children includes a list node's head at index 0.
	if !node.IsList {
		return node.Value() != "" && !strings.EqualFold(node.Value(), "no")
	}
	if len(node.Children) == 1 {
		return true
	}
	if len(node.Children) > 1 {
		return !strings.EqualFold(node.Children[1].Value(), "no")
	}
	return false
}

func readOptionalBoolProperty(node sexpr.ParsedNode, head string) (bool, bool) {
	child, ok := node.Child(head)
	if !ok {
		return false, false
	}
	return readKiCadFlag(child), true
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
