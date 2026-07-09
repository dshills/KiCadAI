package designapi

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
)

func validHierarchyFilename(filename string) error {
	trimmed := strings.TrimSpace(filename)
	if strings.Contains(trimmed, "\\") || strings.ContainsRune(trimmed, '\x00') || (len(trimmed) > 1 && trimmed[1] == ':') {
		return fmt.Errorf("invalid hierarchy sheet filename %q", filename)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("invalid hierarchy sheet filename %q", filename)
	}
	return nil
}

const hierarchySheetWidth = 100.0
const hierarchySheetHeight = 70.0
const hierarchySheetOrigin = 25.0
const hierarchySheetStepX = 120.0
const hierarchySheetStepY = 90.0

func (builder *Builder) applySchematicHierarchy(design *kicaddesign.Design) error {
	if builder == nil || builder.hierarchy == nil {
		return nil
	}
	if design == nil || design.Schematic == nil {
		return fmt.Errorf("schematic required for hierarchy")
	}
	root := design.Schematic
	if len(root.Sheets) != 0 || len(design.SheetFiles) != 0 {
		return fmt.Errorf("cannot apply generated hierarchy to a project with existing child sheets")
	}
	if len(root.RawItems) != 0 {
		return fmt.Errorf("generated hierarchy cannot preserve unsupported root schematic items")
	}

	refToSheet := make(map[string]string)
	legacyRefToSheet := make(map[string]string)
	for _, sheet := range builder.hierarchy.Sheets {
		for _, symbol := range hierarchySheetSymbols(sheet) {
			refToSheet[symbolStateKey(symbol.Reference, symbol.Unit)] = sheet.ID
		}
		for _, reference := range sheet.References {
			legacyRefToSheet[referenceKey(reference)] = sheet.ID
		}
	}
	for _, symbol := range root.Symbols {
		if _, ok := refToSheet[symbolStateKey(symbol.Reference, symbol.Unit)]; !ok {
			if sheetID, legacy := legacyRefToSheet[referenceKey(symbol.Reference)]; legacy {
				refToSheet[symbolStateKey(symbol.Reference, symbol.Unit)] = sheetID
				continue
			}
			return fmt.Errorf("reference %s is not assigned to a hierarchy sheet", symbol.Reference)
		}
	}

	original := *root
	symbolsBySheet := make(map[string][]schematic.SchematicSymbol, len(builder.hierarchy.Sheets))
	for _, symbol := range original.Symbols {
		sheetID := refToSheet[symbolStateKey(symbol.Reference, symbol.Unit)]
		symbolsBySheet[sheetID] = append(symbolsBySheet[sheetID], symbol)
	}
	children := make([]*schematic.SchematicFile, 0, len(builder.hierarchy.Sheets))
	root.Symbols = nil
	root.Wires = nil
	root.Labels = nil
	root.Junctions = nil
	root.NoConnects = nil
	root.Buses = nil
	root.Polylines = nil
	root.BusEntries = nil
	root.Texts = nil
	root.Sheets = nil
	root.RawItems = nil
	root.Instances = nil
	root.SheetInstances = []schematic.SheetInstance{{Project: design.Project.Name, Path: "/", Page: "1"}}
	usedItems := map[kicadfiles.UUID]struct{}{}
	anchorToSheet := hierarchyAnchorSheets(original.Symbols, refToSheet)
	for index, spec := range builder.hierarchy.Sheets {
		filename := strings.TrimSpace(spec.Filename)
		if filename == "" {
			filename = "sch/" + spec.ID + ".kicad_sch"
		}
		sheetUUID := builder.generator.New("root.schematic.sheet", spec.ID)
		childUUID := builder.generator.New("hierarchy.sheet", spec.ID)
		child := new(schematic.SchematicFile)
		*child = original
		child.Filename = filename
		child.UUID = childUUID
		child.Symbols = append([]schematic.SchematicSymbol(nil), symbolsBySheet[spec.ID]...)
		child.Wires = wiresForSheet(original.Wires, child.Symbols, usedItems, spec.ID, anchorToSheet)
		child.Labels = labelsForSheet(original.Labels, child.Symbols, usedItems)
		child.Junctions = junctionsForSheet(original.Junctions, child.Symbols, usedItems)
		child.NoConnects = noConnectsForSheet(original.NoConnects, child.Symbols, usedItems)
		child.Sheets = nil
		child.SheetInstances = []schematic.SheetInstance{{Project: design.Project.Name, Path: "/" + string(sheetUUID) + "/", Page: strconv.Itoa(index + 2)}}
		appendCrossSheetLabels(builder, child, spec.ID, refToSheet, index)
		children = append(children, child)

		columns := int(math.Ceil(math.Sqrt(float64(len(builder.hierarchy.Sheets)))))
		if columns < 1 {
			columns = 1
		}
		position := kicadfiles.Point{
			X: kicadfiles.MM(hierarchySheetOrigin + float64(index%columns)*hierarchySheetStepX),
			Y: kicadfiles.MM(hierarchySheetOrigin + float64(index/columns)*hierarchySheetStepY),
		}
		root.Sheets = append(root.Sheets, schematic.NewSheet(
			sheetUUID,
			spec.Name,
			filename,
			position,
			kicadfiles.Point{X: kicadfiles.MM(hierarchySheetWidth), Y: kicadfiles.MM(hierarchySheetHeight)},
		))
	}
	design.SheetFiles = children
	return nil
}

func hierarchySheetSymbols(sheet SchematicSheet) []SchematicSymbolRef {
	if len(sheet.Symbols) != 0 {
		symbols := append([]SchematicSymbolRef(nil), sheet.Symbols...)
		for index := range symbols {
			if symbols[index].Unit <= 0 {
				symbols[index].Unit = 1
			}
		}
		return symbols
	}
	refs := make([]SchematicSymbolRef, 0, len(sheet.References))
	for _, reference := range sheet.References {
		refs = append(refs, SchematicSymbolRef{Reference: reference, Unit: 1})
	}
	return refs
}

func sheetSymbolBounds(symbols []schematic.SchematicSymbol) (kicadfiles.Point, kicadfiles.Point, bool) {
	if len(symbols) == 0 {
		return kicadfiles.Point{}, kicadfiles.Point{}, false
	}
	minPoint, maxPoint := symbols[0].Position, symbols[0].Position
	for _, symbol := range symbols[1:] {
		if symbol.Position.X < minPoint.X {
			minPoint.X = symbol.Position.X
		}
		if symbol.Position.Y < minPoint.Y {
			minPoint.Y = symbol.Position.Y
		}
		if symbol.Position.X > maxPoint.X {
			maxPoint.X = symbol.Position.X
		}
		if symbol.Position.Y > maxPoint.Y {
			maxPoint.Y = symbol.Position.Y
		}
	}
	margin := kicadfiles.MM(25)
	return kicadfiles.Point{X: minPoint.X - margin, Y: minPoint.Y - margin}, kicadfiles.Point{X: maxPoint.X + margin, Y: maxPoint.Y + margin}, true
}

func pointInSheetBounds(point kicadfiles.Point, minPoint, maxPoint kicadfiles.Point) bool {
	return point.X >= minPoint.X && point.X <= maxPoint.X && point.Y >= minPoint.Y && point.Y <= maxPoint.Y
}

func hierarchyAnchorSheets(symbols []schematic.SchematicSymbol, refToSheet map[string]string) map[kicadfiles.Point]string {
	anchors := map[kicadfiles.Point]string{}
	for _, symbol := range symbols {
		sheetID := refToSheet[symbolStateKey(symbol.Reference, symbol.Unit)]
		if sheetID == "" {
			continue
		}
		for _, anchor := range symbol.PinAnchors {
			anchors[anchor] = sheetID
		}
	}
	return anchors
}

func wiresForSheet(wires []schematic.Wire, symbols []schematic.SchematicSymbol, used map[kicadfiles.UUID]struct{}, sheetID string, anchorToSheet map[kicadfiles.Point]string) []schematic.Wire {
	minPoint, maxPoint, ok := sheetSymbolBounds(symbols)
	if !ok {
		return nil
	}
	selected := make([]schematic.Wire, 0, len(wires))
	for _, wire := range wires {
		inside := len(wire.Points) > 0
		for _, point := range wire.Points {
			inside = inside && pointInSheetBounds(point, minPoint, maxPoint)
		}
		owners := map[string]struct{}{}
		if len(wire.Points) > 0 {
			if owner := anchorToSheet[wire.Points[0]]; owner != "" {
				owners[owner] = struct{}{}
			}
			if owner := anchorToSheet[wire.Points[len(wire.Points)-1]]; owner != "" {
				owners[owner] = struct{}{}
			}
		}
		if len(owners) > 0 {
			inside = false
			if len(owners) == 1 {
				_, inside = owners[sheetID]
			} else if _, local := owners[sheetID]; local {
				wire.Points = localWireStub(wire.Points, anchorToSheet, sheetID)
				inside = len(wire.Points) >= 2
			}
		}
		if inside {
			if _, exists := used[wire.UUID]; exists {
				continue
			}
			used[wire.UUID] = struct{}{}
			wire.Points = append([]kicadfiles.Point(nil), wire.Points...)
			selected = append(selected, wire)
		}
	}
	return selected
}

func localWireStub(points []kicadfiles.Point, anchors map[kicadfiles.Point]string, sheetID string) []kicadfiles.Point {
	if len(points) < 2 {
		return nil
	}
	localAt := -1
	if anchors[points[0]] == sheetID {
		localAt = 0
	} else if anchors[points[len(points)-1]] == sheetID {
		localAt = len(points) - 1
	}
	if localAt < 0 {
		return nil
	}
	adjacentAt := localAt + 1
	if localAt == len(points)-1 {
		adjacentAt = localAt - 1
	}
	anchor := points[localAt]
	adjacent := points[adjacentAt]
	if adjacent == anchor {
		return nil
	}
	length := kicadfiles.MM(5)
	stub := anchor
	switch {
	case adjacent.X != anchor.X:
		if adjacent.X < anchor.X {
			stub.X -= length
		} else {
			stub.X += length
		}
	case adjacent.Y < anchor.Y:
		stub.Y -= length
	case adjacent.Y > anchor.Y:
		stub.Y += length
	default:
		stub.X += length
	}
	return []kicadfiles.Point{anchor, stub}
}

func labelsForSheet(labels []schematic.Label, symbols []schematic.SchematicSymbol, used map[kicadfiles.UUID]struct{}) []schematic.Label {
	minPoint, maxPoint, ok := sheetSymbolBounds(symbols)
	if !ok {
		return nil
	}
	selected := make([]schematic.Label, 0, len(labels))
	for _, label := range labels {
		if pointInSheetBounds(label.Position, minPoint, maxPoint) {
			if _, exists := used[label.UUID]; exists {
				continue
			}
			used[label.UUID] = struct{}{}
			selected = append(selected, label)
		}
	}
	return selected
}

func junctionsForSheet(junctions []schematic.Junction, symbols []schematic.SchematicSymbol, used map[kicadfiles.UUID]struct{}) []schematic.Junction {
	minPoint, maxPoint, ok := sheetSymbolBounds(symbols)
	if !ok {
		return nil
	}
	selected := make([]schematic.Junction, 0, len(junctions))
	for _, junction := range junctions {
		if pointInSheetBounds(junction.Position, minPoint, maxPoint) {
			if _, exists := used[junction.UUID]; exists {
				continue
			}
			used[junction.UUID] = struct{}{}
			selected = append(selected, junction)
		}
	}
	return selected
}

func noConnectsForSheet(noConnects []schematic.NoConnect, symbols []schematic.SchematicSymbol, used map[kicadfiles.UUID]struct{}) []schematic.NoConnect {
	minPoint, maxPoint, ok := sheetSymbolBounds(symbols)
	if !ok {
		return nil
	}
	selected := make([]schematic.NoConnect, 0, len(noConnects))
	for _, noConnect := range noConnects {
		if pointInSheetBounds(noConnect.Position, minPoint, maxPoint) {
			if _, exists := used[noConnect.UUID]; exists {
				continue
			}
			used[noConnect.UUID] = struct{}{}
			selected = append(selected, noConnect)
		}
	}
	return selected
}

func appendCrossSheetLabels(builder *Builder, child *schematic.SchematicFile, sheetID string, refToSheet map[string]string, sheetIndex int) {
	existing := map[string]struct{}{}
	for _, label := range child.Labels {
		if label.Kind == schematic.LabelGlobal {
			existing[globalLabelKey(label.Text, label.Position)] = struct{}{}
		}
	}
	for _, net := range builder.hierarchy.CrossSheetNets {
		for _, endpoint := range net.Endpoints {
			if refToSheet[symbolStateKey(endpoint.Reference, endpoint.Unit)] != sheetID {
				continue
			}
			state, err := builder.symbolStateForEndpoint(endpoint)
			if err != nil {
				continue
			}
			position, ok := state.pins[endpoint.Pin]
			if !ok {
				continue
			}
			key := globalLabelKey(net.Name, position)
			if _, exists := existing[key]; exists {
				continue
			}
			existing[key] = struct{}{}
			child.Labels = append(child.Labels, schematic.NewLabel(
				builder.generator.New("hierarchy.global_label", net.Name, endpoint.Reference, endpoint.Pin, strconv.Itoa(sheetIndex)),
				net.Name,
				schematic.LabelGlobal,
				position,
			))
		}
	}
}

func globalLabelKey(text string, position kicadfiles.Point) string {
	return text + "\x00" + strconv.FormatInt(int64(position.X), 10) + ":" + strconv.FormatInt(int64(position.Y), 10)
}
