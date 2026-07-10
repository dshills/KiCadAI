package designapi

import (
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
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
	if len(root.Buses) != 0 || len(root.BusEntries) != 0 || len(root.Polylines) != 0 || len(root.Texts) != 0 {
		return fmt.Errorf("generated hierarchy cannot preserve free schematic graphics")
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
	// Root wires and junctions are writer-owned routing artifacts. They are
	// intentionally converted into child-local endpoint labels and cross-sheet
	// global labels below; free graphics are rejected above instead of dropped.
	usedItems := map[kicadfiles.UUID]struct{}{}
	anchorToSheet := hierarchyAnchorSheets(original.Symbols, refToSheet)
	columns := int(math.Ceil(math.Sqrt(float64(len(builder.hierarchy.Sheets)))))
	if columns < 1 {
		columns = 1
	}
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
		// Parent-sheet wires and junctions are writer-owned routing artifacts;
		// child connectivity is rebuilt from the builder's net assignments.
		child.Wires = nil
		child.Junctions = nil
		if err := relayoutHierarchyChild(builder, child, spec.ID); err != nil {
			return err
		}
		appendCrossSheetLabels(builder, child, spec.ID, refToSheet, index)
		if err := fitHierarchyChild(child); err != nil {
			return err
		}
		children = append(children, child)

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

func relayoutHierarchyChild(builder *Builder, child *schematic.SchematicFile, sheetID string) error {
	if builder == nil || child == nil {
		return nil
	}
	if len(child.Symbols) == 0 {
		if len(child.Wires) != 0 || len(child.Labels) != 0 || len(child.Junctions) != 0 || len(child.NoConnects) != 0 {
			return fmt.Errorf("hierarchy child %s has routing or annotations without symbols", sheetID)
		}
		return nil
	}
	if builder.design.Schematic == nil {
		return fmt.Errorf("hierarchy child %s requires a schematic design", sheetID)
	}
	if len(child.Wires) != 0 || len(child.Junctions) != 0 {
		return fmt.Errorf("hierarchy child %s contains unexpected pre-existing routing", sheetID)
	}
	if len(child.Buses) != 0 || len(child.BusEntries) != 0 || len(child.Polylines) != 0 || len(child.Texts) != 0 {
		return fmt.Errorf("hierarchy child %s contains unsupported free schematic graphics", sheetID)
	}
	managedLabels := make(map[string]struct{}, len(builder.design.ExpectedNets))
	for _, netName := range builder.design.ExpectedNets {
		managedLabels[builder.canonicalNet(netName)] = struct{}{}
	}
	preservedLabels := make([]schematic.Label, 0)
	for _, label := range child.Labels {
		if _, managed := managedLabels[builder.canonicalNet(label.Text)]; !managed {
			preservedLabels = append(preservedLabels, label)
		}
	}
	request := schematiclayout.Request{
		Sheet: schematiclayout.SheetForPaper(child.Paper.Name),
		Rules: schematiclayout.DefaultRules(schematiclayout.ProfileStandard),
	}
	for _, symbol := range child.Symbols {
		if len(symbol.Pins) != len(symbol.PinAnchors) {
			return fmt.Errorf("hierarchy child %s symbol %s unit %d has %d pins but %d pin anchors", sheetID, symbol.Reference, symbol.Unit, len(symbol.Pins), len(symbol.PinAnchors))
		}
		key := symbolStateKey(symbol.Reference, symbol.Unit)
		component := schematiclayout.Component{
			Ref:       key,
			Value:     symbol.Value,
			LibraryID: symbol.LibraryID,
			Position:  symbol.Position,
		}
		component.Role = schematiclayout.InferComponentRole(component)
		for index, pin := range symbol.Pins {
			relative := kicadfiles.Point{X: symbol.PinAnchors[index].X - symbol.Position.X, Y: symbol.PinAnchors[index].Y - symbol.Position.Y}
			component.Pins = append(component.Pins, schematiclayout.Pin{Number: pin.Number, At: relative})
		}
		request.Components = append(request.Components, component)
	}
	allNetEndpoints := map[string][]schematiclayout.Endpoint{}
	crossSheetNets := map[string]struct{}{}
	for _, net := range builder.hierarchy.CrossSheetNets {
		crossSheetNets[builder.canonicalNet(net.Name)] = struct{}{}
	}
	stateKeys := make([]string, 0, len(child.Symbols))
	seenStateKeys := make(map[string]struct{}, len(child.Symbols))
	for _, symbol := range child.Symbols {
		key := symbolStateKey(symbol.Reference, symbol.Unit)
		if _, seen := seenStateKeys[key]; seen {
			continue
		}
		seenStateKeys[key] = struct{}{}
		stateKeys = append(stateKeys, key)
	}
	sort.Strings(stateKeys)
	for _, key := range stateKeys {
		state := builder.symbols[key]
		if state == nil {
			continue
		}
		for pin, netName := range state.pinNets {
			netName = builder.canonicalNet(netName)
			if netName == "" {
				continue
			}
			if _, crossSheet := crossSheetNets[netName]; crossSheet {
				continue
			}
			allNetEndpoints[netName] = append(allNetEndpoints[netName], schematiclayout.Endpoint{Ref: key, Pin: pin})
		}
	}
	netNames := make([]string, 0, len(allNetEndpoints))
	for netName := range allNetEndpoints {
		netNames = append(netNames, netName)
	}
	sort.Strings(netNames)
	for _, netName := range netNames {
		endpoints := append([]schematiclayout.Endpoint(nil), allNetEndpoints[netName]...)
		sort.Slice(endpoints, func(i, j int) bool {
			if endpoints[i].Ref != endpoints[j].Ref {
				return endpoints[i].Ref < endpoints[j].Ref
			}
			return endpoints[i].Pin < endpoints[j].Pin
		})
		layoutNet := schematiclayout.Net{Name: netName, PreferredLabels: true, Endpoints: endpoints}
		layoutNet.Role = schematiclayout.InferNetRole(layoutNet)
		request.Nets = append(request.Nets, layoutNet)
	}
	result := schematiclayout.Layout(request)
	placedByKey := make(map[string]schematiclayout.PlacedComponent, len(result.Components))
	for _, placed := range result.Components {
		placedByKey[placed.Ref] = placed
	}
	if len(placedByKey) != len(child.Symbols) {
		return fmt.Errorf("hierarchy child %s layout placed %d of %d symbols", sheetID, len(placedByKey), len(child.Symbols))
	}
	anchorMoves := map[kicadfiles.Point]kicadfiles.Point{}
	for index := range child.Symbols {
		symbol := &child.Symbols[index]
		placed, ok := placedByKey[symbolStateKey(symbol.Reference, symbol.Unit)]
		if !ok {
			continue
		}
		move := kicadfiles.Point{X: placed.PlacedAt.X - symbol.Position.X, Y: placed.PlacedAt.Y - symbol.Position.Y}
		for _, anchor := range symbol.PinAnchors {
			anchorMoves[anchor] = move
		}
		moveSymbol(symbol, move)
	}
	originalNoConnects := append([]schematic.NoConnect(nil), child.NoConnects...)
	anchors := make([]kicadfiles.Point, 0, len(anchorMoves))
	for anchor := range anchorMoves {
		anchors = append(anchors, anchor)
	}
	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].X != anchors[j].X {
			return anchors[i].X < anchors[j].X
		}
		return anchors[i].Y < anchors[j].Y
	})
	movedNoConnects := make([]schematic.NoConnect, 0, len(originalNoConnects))
	for _, noConnect := range originalNoConnects {
		move, ok := nearestAnchorMove(noConnect.Position, anchors, anchorMoves)
		if !ok {
			return fmt.Errorf("hierarchy child %s no-connect at %d,%d does not match a pin anchor", sheetID, noConnect.Position.X, noConnect.Position.Y)
		}
		noConnect.Position.X += move.X
		noConnect.Position.Y += move.Y
		movedNoConnects = append(movedNoConnects, noConnect)
	}
	// Generated hierarchy owns the child routing representation. Free graphics
	// are rejected before hierarchy application, so this normalization cannot
	// silently discard user-authored annotations.
	// Child-local connectivity is emitted as same-net labels at every endpoint.
	// This avoids carrying parent-sheet wire geometry across a partition while
	// preserving KiCad connectivity; cross-sheet nets receive global labels.
	child.Wires = make([]schematic.Wire, 0, len(result.Wires))
	for index, wire := range result.Wires {
		child.Wires = append(child.Wires, schematic.NewWire(
			builder.generator.New("hierarchy.local_wire", sheetID, wire.NetName, strconv.Itoa(index)),
			wire.From,
			wire.To,
		))
	}
	child.Labels = preservedLabels
	child.Junctions = nil
	child.Buses = nil
	child.BusEntries = nil
	child.Polylines = nil
	child.Texts = nil
	child.NoConnects = movedNoConnects
	labels := append([]schematiclayout.Label(nil), result.Labels...)
	sort.Slice(labels, func(i, j int) bool {
		if labels[i].NetName != labels[j].NetName {
			return labels[i].NetName < labels[j].NetName
		}
		if labels[i].Position.X != labels[j].Position.X {
			return labels[i].Position.X < labels[j].Position.X
		}
		return labels[i].Position.Y < labels[j].Position.Y
	})
	for index, label := range labels {
		child.Labels = append(child.Labels, schematic.NewLabel(builder.generator.New("hierarchy.local_label", sheetID, label.NetName, strconv.Itoa(index)), label.Text, schematic.LabelLocal, label.Position))
	}
	return nil
}

func nearestAnchorMove(position kicadfiles.Point, anchors []kicadfiles.Point, moves map[kicadfiles.Point]kicadfiles.Point) (kicadfiles.Point, bool) {
	const tolerance = kicadfiles.IU(200000)
	bestDistance := kicadfiles.IU(0)
	var best kicadfiles.Point
	found := false
	for _, anchor := range anchors {
		move := moves[anchor]
		dx := position.X - anchor.X
		dy := position.Y - anchor.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		distance := dx + dy
		if distance > tolerance || found && distance >= bestDistance {
			continue
		}
		bestDistance = distance
		best = move
		found = true
	}
	return best, found
}

func moveSymbol(symbol *schematic.SchematicSymbol, delta kicadfiles.Point) {
	if symbol == nil {
		return
	}
	symbol.Position.X += delta.X
	symbol.Position.Y += delta.Y
	for index := range symbol.PinAnchors {
		symbol.PinAnchors[index].X += delta.X
		symbol.PinAnchors[index].Y += delta.Y
	}
	for index := range symbol.Properties {
		symbol.Properties[index].Position.X += delta.X
		symbol.Properties[index].Position.Y += delta.Y
	}
	for index := range symbol.Fields {
		symbol.Fields[index].Position.X += delta.X
		symbol.Fields[index].Position.Y += delta.Y
	}
}

func fitHierarchyChild(child *schematic.SchematicFile) error {
	if child == nil {
		return nil
	}
	minPoint, maxPoint, ok := sheetSymbolBounds(child.Symbols)
	if !ok {
		if len(child.Labels) > 0 {
			minPoint = child.Labels[0].Position
			maxPoint = minPoint
			ok = true
		} else if len(child.NoConnects) > 0 {
			minPoint = child.NoConnects[0].Position
			maxPoint = minPoint
			ok = true
		}
		if !ok {
			child.Paper = kicadfiles.Paper{Name: "A4", Width: kicadfiles.MM(297), Height: kicadfiles.MM(210)}
			return nil
		}
		margin := kicadfiles.MM(25)
		minPoint.X -= margin
		minPoint.Y -= margin
		maxPoint.X += margin
		maxPoint.Y += margin
	}
	for _, label := range child.Labels {
		if label.Position.X < minPoint.X {
			minPoint.X = label.Position.X
		}
		if label.Position.Y < minPoint.Y {
			minPoint.Y = label.Position.Y
		}
		if label.Position.X > maxPoint.X {
			maxPoint.X = label.Position.X
		}
		if label.Position.Y > maxPoint.Y {
			maxPoint.Y = label.Position.Y
		}
	}
	for _, noConnect := range child.NoConnects {
		if noConnect.Position.X < minPoint.X {
			minPoint.X = noConnect.Position.X
		}
		if noConnect.Position.Y < minPoint.Y {
			minPoint.Y = noConnect.Position.Y
		}
		if noConnect.Position.X > maxPoint.X {
			maxPoint.X = noConnect.Position.X
		}
		if noConnect.Position.Y > maxPoint.Y {
			maxPoint.Y = noConnect.Position.Y
		}
	}
	textMargin := kicadfiles.MM(10.16)
	minPoint.X -= textMargin
	minPoint.Y -= textMargin
	maxPoint.X += textMargin
	maxPoint.Y += textMargin
	papers := []string{"A5", "A4", "A3", "A2", "A1", "A0"}
	selected := schematiclayout.SheetForPaper(papers[len(papers)-1])
	for _, name := range papers {
		candidate := schematiclayout.SheetForPaper(name)
		usable := schematiclayout.UsableSheet(candidate)
		if maxPoint.X-minPoint.X <= usable.Width() && maxPoint.Y-minPoint.Y <= usable.Height() {
			selected = candidate
			break
		}
	}
	usable := schematiclayout.UsableSheet(selected)
	if maxPoint.X-minPoint.X > usable.Width() || maxPoint.Y-minPoint.Y > usable.Height() {
		return fmt.Errorf("hierarchy child %s exceeds the largest supported schematic page", child.Filename)
	}
	child.Paper = kicadfiles.Paper{Name: selected.Name, Width: selected.Width, Height: selected.Height}
	currentCenter := kicadfiles.Point{
		X: (minPoint.X + maxPoint.X) / 2,
		Y: (minPoint.Y + maxPoint.Y) / 2,
	}
	targetCenter := kicadfiles.Point{
		X: (usable.MinX + usable.MaxX) / 2,
		Y: (usable.MinY + usable.MaxY) / 2,
	}
	translateSchematic(child, kicadfiles.Point{X: targetCenter.X - currentCenter.X, Y: targetCenter.Y - currentCenter.Y})
	return nil
}

func translateSchematic(file *schematic.SchematicFile, delta kicadfiles.Point) {
	movePoints := func(points []kicadfiles.Point) {
		for index := range points {
			points[index].X += delta.X
			points[index].Y += delta.Y
		}
	}
	for index := range file.Symbols {
		moveSymbol(&file.Symbols[index], delta)
	}
	for index := range file.Wires {
		movePoints(file.Wires[index].Points)
	}
	for index := range file.Buses {
		movePoints(file.Buses[index].Points)
	}
	for index := range file.Polylines {
		movePoints(file.Polylines[index].Points)
	}
	for index := range file.BusEntries {
		file.BusEntries[index].Position.X += delta.X
		file.BusEntries[index].Position.Y += delta.Y
	}
	for index := range file.Texts {
		file.Texts[index].Position.X += delta.X
		file.Texts[index].Position.Y += delta.Y
	}
	for index := range file.Labels {
		file.Labels[index].Position.X += delta.X
		file.Labels[index].Position.Y += delta.Y
		for fieldIndex := range file.Labels[index].Fields {
			file.Labels[index].Fields[fieldIndex].Position.X += delta.X
			file.Labels[index].Fields[fieldIndex].Position.Y += delta.Y
		}
	}
	for index := range file.Junctions {
		file.Junctions[index].Position.X += delta.X
		file.Junctions[index].Position.Y += delta.Y
	}
	for index := range file.NoConnects {
		file.NoConnects[index].Position.X += delta.X
		file.NoConnects[index].Position.Y += delta.Y
	}
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
	symbolsByKey := make(map[string]*schematic.SchematicSymbol, len(child.Symbols))
	for index := range child.Symbols {
		symbolsByKey[symbolStateKey(child.Symbols[index].Reference, child.Symbols[index].Unit)] = &child.Symbols[index]
	}
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
			symbol := symbolsByKey[symbolStateKey(endpoint.Reference, endpoint.Unit)]
			if symbol == nil {
				continue
			}
			position, ok := hierarchySymbolPinAnchor(symbol, endpoint.Pin)
			if !ok {
				continue
			}
			key := globalLabelKey(net.Name, position)
			if _, exists := existing[key]; exists {
				continue
			}
			existing[key] = struct{}{}
			child.Labels = append(child.Labels, schematic.NewLabel(
				builder.generator.New("hierarchy.global_label", sheetID, net.Name, endpoint.Reference, strconv.Itoa(endpoint.Unit), endpoint.Pin, strconv.Itoa(sheetIndex)),
				net.Name,
				schematic.LabelGlobal,
				position,
			))
		}
	}
}

func hierarchySymbolPinAnchor(symbol *schematic.SchematicSymbol, pin string) (kicadfiles.Point, bool) {
	if symbol == nil {
		return kicadfiles.Point{}, false
	}
	for index, symbolPin := range symbol.Pins {
		if symbolPin.Number == pin && index < len(symbol.PinAnchors) {
			return symbol.PinAnchors[index], true
		}
	}
	return kicadfiles.Point{}, false
}

func globalLabelKey(text string, position kicadfiles.Point) string {
	return text + "\x00" + strconv.FormatInt(int64(position.X), 10) + ":" + strconv.FormatInt(int64(position.Y), 10)
}
