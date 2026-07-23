package schematicpcb

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	defaultOriginXMM  = 20.0
	defaultOriginYMM  = 20.0
	defaultSpacingXMM = 12.5
	defaultSpacingYMM = 10.0
	defaultColumns    = 5
)

type Options struct {
	LibraryIndex *libraryresolver.LibraryIndex
	OriginXMM    float64
	OriginYMM    float64
	SpacingXMM   float64
	SpacingYMM   float64
	Columns      int
}

type Result struct {
	ProjectName       string                   `json:"project_name,omitempty"`
	SymbolCount       int                      `json:"symbol_count"`
	AssignedCount     int                      `json:"assigned_count"`
	PlacedCount       int                      `json:"placed_count"`
	NetHintCount      int                      `json:"net_hint_count"`
	Transaction       transactions.Transaction `json:"transaction"`
	Issues            []reports.Issue          `json:"issues,omitempty"`
	RequiresLibraries bool                     `json:"requires_libraries"`
}

type component struct {
	Ref         string
	Value       string
	LibraryID   string
	FootprintID string
	Symbols     []componentSymbol
}

type componentSymbol struct {
	FileIndex int
	Symbol    schematic.SchematicSymbol
}

type schematicFileEntry struct {
	Path      string
	NetPrefix string
	File      schematic.SchematicFile
}

type pinAnchor struct {
	Ref    string
	Pin    string
	Point  kicadfiles.Point
	Hidden bool
}

func FromDesign(design kicaddesign.Design, opts Options) Result {
	result := Result{ProjectName: design.Name, Transaction: transactions.Transaction{Name: design.Name, Project: design.Name}}
	if design.Schematic == nil {
		result.Issues = append(result.Issues, issue(reports.CodeMissingFile, reports.SeverityError, "schematic", "root schematic is required"))
		return result
	}
	files := schematicFiles(design)
	if !validateHierarchy(files, &result) {
		return result
	}
	components := schematicComponents(files, &result)
	result.SymbolCount = symbolCount(files)
	result.AssignedCount = len(components)
	result.RequiresLibraries = opts.LibraryIndex == nil
	netHints := map[string]map[string]string{}
	if opts.LibraryIndex != nil {
		netHints = inferPinNetHints(files, components, *opts.LibraryIndex, &result)
	} else {
		netHints = inferPinNetHints(files, components, libraryresolver.LibraryIndex{}, &result)
	}
	ops := make([]transactions.Operation, 0, len(components)+1)
	layout := normalizeLayout(opts)
	placedCount := 0
	for i, component := range components {
		payload := transactions.PlaceFootprintOperation{
			Op:          transactions.OpPlaceFootprint,
			Ref:         component.Ref,
			FootprintID: component.FootprintID,
			Value:       component.Value,
			At: transactions.Point{
				XMM: layout.originX + float64(i%layout.columns)*layout.spacingX,
				YMM: layout.originY + float64(i/layout.columns)*layout.spacingY,
			},
		}
		if opts.LibraryIndex != nil {
			if footprint, ok := libraryresolver.ResolveFootprint(*opts.LibraryIndex, component.FootprintID); ok {
				payload.Pads = padSpecsWithNetHints(footprint, netHints[component.Ref])
			} else if pads, templateOK := verifiedTransferPadSpecs(component.FootprintID, netHints[component.Ref]); templateOK {
				payload.Pads = pads
			} else {
				result.Issues = append(result.Issues, issue(reports.CodeUnknownFootprintLibrary, reports.SeverityWarning, "footprint."+component.Ref, "footprint record not found for "+component.FootprintID))
			}
		} else if pads, ok := verifiedTransferPadSpecs(component.FootprintID, netHints[component.Ref]); ok {
			payload.Pads = pads
		} else {
			result.Issues = append(result.Issues, issue(reports.CodeUnknownFootprintLibrary, reports.SeverityWarning, "footprint."+component.Ref, "no library index provided and no verified footprint template for "+component.FootprintID))
		}
		result.NetHintCount += countPadNetHints(payload.Pads)
		op, err := newOperation(transactions.OpPlaceFootprint, payload, component.Ref)
		if err != nil {
			result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, "transaction.place_footprint."+component.Ref, "failed to serialize placement operation: "+err.Error()))
			continue
		}
		ops = append(ops, op)
		placedCount++
	}
	writeOp, err := newOperation(transactions.OpWriteProject, transactions.WriteProjectOperation{Op: transactions.OpWriteProject}, "")
	if err != nil {
		result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, "transaction.write_project", "failed to serialize write operation: "+err.Error()))
	} else {
		ops = append(ops, writeOp)
	}
	result.Transaction.Operations = ops
	result.PlacedCount = placedCount
	return result
}

func schematicFiles(design kicaddesign.Design) []schematicFileEntry {
	rootPath := strings.TrimSpace(design.Schematic.Filename)
	if rootPath == "" {
		rootPath = "schematic"
	}
	files := []schematicFileEntry{{Path: rootPath, File: *design.Schematic}}
	for i, sheetFile := range design.SheetFiles {
		if sheetFile == nil {
			continue
		}
		path := strings.TrimSpace(sheetFile.Filename)
		if path == "" {
			path = fmt.Sprintf("sheet_files[%d]", i)
		}
		files = append(files, schematicFileEntry{Path: path, File: *sheetFile})
	}
	applySheetNetPrefixes(files)
	return files
}

func applySheetNetPrefixes(files []schematicFileEntry) {
	byPath := map[string]int{}
	for i, file := range files {
		byPath[file.Path] = i
	}
	visited := map[string]struct{}{}
	var visit func(parent schematicFileEntry)
	visit = func(parent schematicFileEntry) {
		if _, ok := visited[parent.Path]; ok {
			return
		}
		visited[parent.Path] = struct{}{}
		for _, sheet := range parent.File.Sheets {
			childPath, err := kicaddesign.ResolveSheetPath(parent.Path, sheet.Filename)
			if err != nil {
				continue
			}
			childIndex, ok := byPath[childPath]
			if !ok {
				continue
			}
			prefix := sheetNetName(sheet)
			if parent.NetPrefix != "" {
				prefix = parent.NetPrefix + "/" + prefix
			}
			files[childIndex].NetPrefix = prefix
			visit(files[childIndex])
		}
	}
	if len(files) > 0 {
		visit(files[0])
	}
}

func sheetNetName(sheet schematic.Sheet) string {
	name := strings.TrimSpace(sheet.Name)
	if name != "" {
		return name
	}
	filename := strings.TrimSpace(sheet.Filename)
	if filename == "" {
		return "sheet"
	}
	base := filepath.Base(filepath.FromSlash(filename))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func symbolCount(files []schematicFileEntry) int {
	count := 0
	for _, file := range files {
		count += len(file.File.Symbols)
	}
	return count
}

func validateHierarchy(files []schematicFileEntry, result *Result) bool {
	counts := map[string]int{}
	for _, file := range files {
		for _, sheet := range file.File.Sheets {
			name := strings.TrimSpace(sheet.Filename)
			if name == "" {
				continue
			}
			key, err := kicaddesign.ResolveSheetPath(file.Path, name)
			if err != nil {
				result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, "schematic.sheets."+name, fmt.Sprintf("invalid sheet file path %q: %v", name, err)))
				continue
			}
			counts[key]++
		}
	}
	ok := true
	for name, count := range counts {
		if count > 1 {
			ok = false
			result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, "schematic.sheets."+name, "multi-instantiated sheet files are not supported yet"))
		}
	}
	return ok
}

type layoutOptions struct {
	originX  float64
	originY  float64
	spacingX float64
	spacingY float64
	columns  int
}

func normalizeLayout(opts Options) layoutOptions {
	layout := layoutOptions{originX: opts.OriginXMM, originY: opts.OriginYMM, spacingX: opts.SpacingXMM, spacingY: opts.SpacingYMM, columns: opts.Columns}
	if layout.originX == 0 {
		layout.originX = defaultOriginXMM
	}
	if layout.originY == 0 {
		layout.originY = defaultOriginYMM
	}
	if layout.spacingX == 0 {
		layout.spacingX = defaultSpacingXMM
	}
	if layout.spacingY == 0 {
		layout.spacingY = defaultSpacingYMM
	}
	if layout.columns <= 0 {
		layout.columns = defaultColumns
	}
	return layout
}

func schematicComponents(files []schematicFileEntry, result *Result) []component {
	byRef := map[string]*component{}
	for fileIndex, file := range files {
		for i, symbol := range file.File.Symbols {
			ref := strings.TrimSpace(symbol.Reference)
			if ref == "" || strings.HasPrefix(ref, "#") {
				continue
			}
			if symbol.OnBoard != nil && !*symbol.OnBoard {
				continue
			}
			footprintID := symbolProperty(symbol, "Footprint")
			symbolRef := componentSymbol{FileIndex: fileIndex, Symbol: symbol}
			if existing, ok := byRef[ref]; ok {
				existing.Symbols = append(existing.Symbols, symbolRef)
				if symbol.LibraryID != "" && existing.LibraryID != "" && symbol.LibraryID != existing.LibraryID {
					result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("%s.symbols[%d].lib_id", file.Path, i), "multi-unit schematic reference "+ref+" has conflicting library id "+symbol.LibraryID))
				} else if existing.LibraryID == "" {
					existing.LibraryID = symbol.LibraryID
				}
				if symbol.Value != "" && existing.Value != "" && symbol.Value != existing.Value {
					result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("%s.symbols[%d].Value", file.Path, i), "multi-unit schematic reference "+ref+" has conflicting value "+symbol.Value))
				} else if existing.Value == "" {
					existing.Value = symbol.Value
				}
				if footprintID != "" && existing.FootprintID != "" && footprintID != existing.FootprintID {
					result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("%s.symbols[%d].Footprint", file.Path, i), "multi-unit schematic reference "+ref+" has conflicting footprint "+footprintID))
				} else if existing.FootprintID == "" {
					existing.FootprintID = footprintID
				}
				continue
			}
			byRef[ref] = &component{Ref: ref, Value: symbol.Value, LibraryID: symbol.LibraryID, FootprintID: footprintID, Symbols: []componentSymbol{symbolRef}}
		}
	}
	components := make([]component, 0, len(byRef))
	for _, component := range byRef {
		if component.FootprintID == "" {
			result.Issues = append(result.Issues, issue(reports.CodeMissingFootprint, reports.SeverityWarning, "symbols."+component.Ref+".Footprint", "schematic symbol "+component.Ref+" has no assigned footprint"))
			continue
		}
		components = append(components, *component)
	}
	// Map collection is intentionally followed by a natural reference sort so
	// generated placement operations are deterministic.
	sort.SliceStable(components, func(i, j int) bool { return naturalRefLess(components[i].Ref, components[j].Ref) })
	return components
}

func inferPinNetHints(files []schematicFileEntry, components []component, index libraryresolver.LibraryIndex, result *Result) map[string]map[string]string {
	hints := map[string]map[string]string{}
	byFile := componentsByFile(len(files), components)
	for fileIndex, file := range files {
		fileHints := inferFilePinNetHints(fileIndex == 0, file.NetPrefix, file.File, byFile[fileIndex], index, result)
		for ref, pinHints := range fileHints {
			if hints[ref] == nil {
				hints[ref] = map[string]string{}
			}
			for pin, net := range pinHints {
				hints[ref][pin] = net
			}
		}
	}
	return hints
}

func componentsByFile(fileCount int, components []component) [][]component {
	byFile := make([][]component, fileCount)
	for _, component := range components {
		groupedSymbols := map[int][]componentSymbol{}
		for _, symbol := range component.Symbols {
			groupedSymbols[symbol.FileIndex] = append(groupedSymbols[symbol.FileIndex], symbol)
		}
		for fileIndex, symbols := range groupedSymbols {
			if fileIndex < 0 || fileIndex >= fileCount {
				continue
			}
			fileComponent := component
			fileComponent.Symbols = symbols
			byFile[fileIndex] = append(byFile[fileIndex], fileComponent)
		}
	}
	return byFile
}

func inferFilePinNetHints(isRoot bool, netPrefix string, file schematic.SchematicFile, components []component, index libraryresolver.LibraryIndex, result *Result) map[string]map[string]string {
	graph := newNetGraph()
	var terminals []kicadfiles.Point
	terminalSeen := map[graphPoint]struct{}{}
	addTerminal := func(point kicadfiles.Point) {
		key := pointKey(point)
		if _, ok := terminalSeen[key]; ok {
			return
		}
		terminalSeen[key] = struct{}{}
		terminals = append(terminals, point)
	}
	for _, junction := range file.Junctions {
		graph.ensure(pointKey(junction.Position))
		addTerminal(junction.Position)
	}
	// The schematic reader normalizes local, global, and hierarchical labels
	// into file.Labels while preserving each label kind.
	for _, label := range file.Labels {
		key := pointKey(label.Position)
		graph.ensure(key)
		graph.label(key, labelNetName(label, isRoot, netPrefix))
		addTerminal(label.Position)
	}
	anchors := collectPinAnchors(components, index, result)
	for _, anchor := range anchors {
		graph.ensure(pointKey(anchor.Point))
		addTerminal(anchor.Point)
	}
	terminalIndex := newTerminalIndex(terminals)
	for _, wire := range file.Wires {
		for i := 0; i < len(wire.Points)-1; i++ {
			start := wire.Points[i]
			end := wire.Points[i+1]
			graph.union(pointKey(start), pointKey(end))
			for _, terminal := range terminalIndex.candidates(start, end) {
				if pointInSegmentBounds(terminal, start, end) && pointOnSegment(terminal, start, end) {
					graph.union(pointKey(start), pointKey(terminal))
				}
			}
		}
	}
	netByRoot := graph.netNames()
	hints := map[string]map[string]string{}
	for _, anchor := range anchors {
		root := graph.find(pointKey(anchor.Point))
		net := netByRoot[root]
		if strings.TrimSpace(net) == "" {
			net = fallbackNetName(root, anchors, graph)
		}
		if strings.TrimSpace(net) == "" {
			continue
		}
		if hints[anchor.Ref] == nil {
			hints[anchor.Ref] = map[string]string{}
		}
		hints[anchor.Ref][anchor.Pin] = net
	}
	return hints
}

func labelNetName(label schematic.Label, isRoot bool, netPrefix string) string {
	text := strings.TrimSpace(label.Text)
	if text == "" {
		return ""
	}
	if label.Kind == schematic.LabelGlobal {
		return text
	}
	if isRoot {
		return text
	}
	if strings.TrimSpace(netPrefix) == "" {
		return text
	}
	return netPrefix + "/" + text
}

func collectPinAnchors(components []component, index libraryresolver.LibraryIndex, result *Result) []pinAnchor {
	var anchors []pinAnchor
	for _, component := range components {
		for _, componentSymbol := range component.Symbols {
			symbol := componentSymbol.Symbol
			record, ok := libraryresolver.ResolveSymbol(index, symbol.LibraryID)
			if !ok {
				if len(symbol.Pins) != 0 && len(symbol.Pins) == len(symbol.PinAnchors) {
					for pinIndex, pin := range symbol.Pins {
						if strings.TrimSpace(pin.Number) == "" {
							continue
						}
						anchors = append(anchors, pinAnchor{
							Ref:   component.Ref,
							Pin:   pin.Number,
							Point: symbol.PinAnchors[pinIndex],
						})
					}
					continue
				}
				if templatePins, templateOK := schematic.EmbeddedSymbolPinOffsets(symbol.LibraryID); templateOK {
					for _, pin := range templatePins {
						if strings.TrimSpace(pin.Number) == "" {
							continue
						}
						anchors = append(anchors, pinAnchor{
							Ref:   component.Ref,
							Pin:   pin.Number,
							Point: addPoint(symbol.Position, transformPinPoint(pin.Offset, symbol.Mirror, symbol.Rotation)),
						})
					}
					continue
				}
				result.Issues = append(result.Issues, issue(reports.CodeUnknownSymbolLibrary, reports.SeverityWarning, "symbol."+component.Ref, "symbol record not found for "+symbol.LibraryID))
				continue
			}
			unit := symbol.Unit
			if unit == 0 {
				unit = 1
			}
			bodyStyle := symbol.BodyStyle
			if bodyStyle == 0 {
				bodyStyle = 1
			}
			for _, pin := range record.Pins {
				if strings.TrimSpace(pin.Number) == "" {
					continue
				}
				if pin.Unit != 0 && pin.Unit != unit {
					continue
				}
				if pin.BodyStyle != 0 && pin.BodyStyle != bodyStyle {
					continue
				}
				anchors = append(anchors, pinAnchor{
					Ref:    component.Ref,
					Pin:    pin.Number,
					Point:  addPoint(symbol.Position, transformPinPoint(pin.Position, symbol.Mirror, symbol.Rotation)),
					Hidden: pin.Hidden,
				})
			}
		}
	}
	return anchors
}

func fallbackNetName(root graphPoint, anchors []pinAnchor, graph *netGraph) string {
	var connected []pinAnchor
	for _, anchor := range anchors {
		if graph.find(pointKey(anchor.Point)) == root {
			connected = append(connected, anchor)
		}
	}
	if len(connected) < 2 {
		return ""
	}
	sort.SliceStable(connected, func(i, j int) bool {
		if connected[i].Ref != connected[j].Ref {
			return naturalRefLess(connected[i].Ref, connected[j].Ref)
		}
		return connected[i].Pin < connected[j].Pin
	})
	return fmt.Sprintf("Net-(%s-Pad%s)", connected[0].Ref, connected[0].Pin)
}

func padSpecsWithNetHints(footprint libraryresolver.FootprintRecord, hints map[string]string) []transactions.PadSpec {
	pads := make([]transactions.PadSpec, 0, len(footprint.Pads))
	for _, pad := range footprint.Pads {
		spec := transactions.PadSpec{
			Name:     pad.Name,
			Type:     pad.Type,
			Shape:    pad.Shape,
			XMM:      iuToMM(pad.Position.X),
			YMM:      iuToMM(pad.Position.Y),
			WidthMM:  iuToMM(pad.Size.X),
			HeightMM: iuToMM(pad.Size.Y),
			DrillMM:  iuToMM(pad.Drill),
		}
		for _, layer := range pad.Layers {
			spec.Layers = append(spec.Layers, string(layer))
		}
		if net := strings.TrimSpace(hints[pad.Name]); net != "" {
			value := net
			spec.Net = &value
		}
		pads = append(pads, spec)
	}
	return pads
}

func verifiedTransferPadSpecs(footprintID string, hints map[string]string) ([]transactions.PadSpec, bool) {
	switch strings.TrimSpace(footprintID) {
	case "Resistor_SMD:R_0603_1608Metric", "Capacitor_SMD:C_0603_1608Metric":
		return twoTransferPads(0.85, 0.95, 1.7, hints), true
	case "Resistor_SMD:R_0805_2012Metric",
		"Capacitor_SMD:C_0805_2012Metric",
		"LED_SMD:LED_0805_2012Metric",
		"Diode_SMD:D_SOD-323":
		return twoTransferPads(1.15, 1.45, 2.0, hints), true
	case "Diode_SMD:D_SOD-123":
		return twoTransferPads(1.35, 1.55, 3.0, hints), true
	case "Fuse:Fuse_1206_3216Metric":
		return twoTransferPads(1.75, 1.9, 3.6, hints), true
	case "Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal":
		return usbCGCTUSB4125TransferPads(hints), true
	case "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm":
		return soic8NarrowTransferTemplate.pads(hints), true
	case "Package_QFP:TQFP-32_7x7mm_P0.8mm":
		return tqfp32TransferPads(hints), true
	case "Package_TO_SOT_SMD:SOT-23-5":
		return sot23_5TransferPads(hints), true
	case "Package_TO_SOT_SMD:SOT-223-3_TabPin2":
		return sot223_3TransferPads(hints), true
	case "Package_TO_SOT_SMD:SOT-223":
		return sot223_4TransferPads(hints), true
	case "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering":
		return boschLGA8TransferPads(hints), true
	case "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering":
		return boschLGA8_2_5TransferPads(hints), true
	case "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm":
		return sensirionDFN8TransferPads(hints), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x01_P2.54mm_Vertical":
		return pinHeaderTransferPads(1, hints), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical":
		return pinHeaderTransferPads(2, hints), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical":
		return pinHeaderTransferPads(3, hints), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical":
		return pinHeaderTransferPads(4, hints), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x05_P2.54mm_Vertical":
		return pinHeaderTransferPads(5, hints), true
	default:
		return nil, false
	}
}

func sot23_5TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -1.1375, -0.95, 1.325, 0.6, hints),
		transferPadSpec("2", -1.1375, 0, 1.325, 0.6, hints),
		transferPadSpec("3", -1.1375, 0.95, 1.325, 0.6, hints),
		transferPadSpec("4", 1.1375, 0.95, 1.325, 0.6, hints),
		transferPadSpec("5", 1.1375, -0.95, 1.325, 0.6, hints),
	}
}

func sot223_3TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -2.3, 2.4, 1.2, 1.5, hints),
		transferPadSpec("2", 0, 2.4, 1.2, 1.5, hints),
		transferPadSpec("3", 2.3, 2.4, 1.2, 1.5, hints),
		transferPadSpec("2", 0, -2.1, 3.8, 2.4, hints),
	}
}

func sot223_4TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -3.15, -2.3, 2, 1.5, hints),
		transferPadSpec("2", -3.15, 0, 2, 1.5, hints),
		transferPadSpec("3", -3.15, 2.3, 2, 1.5, hints),
		transferPadSpec("4", 3.15, 0, 2, 3.8, hints),
	}
}

func boschLGA8TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -0.975, -0.8, 0.35, 0.5, hints),
		transferPadSpec("2", -0.325, -0.8, 0.35, 0.5, hints),
		transferPadSpec("3", 0.325, -0.8, 0.35, 0.5, hints),
		transferPadSpec("4", 0.975, -0.8, 0.35, 0.5, hints),
		transferPadSpec("5", 0.975, 0.8, 0.35, 0.5, hints),
		transferPadSpec("6", 0.325, 0.8, 0.35, 0.5, hints),
		transferPadSpec("7", -0.325, 0.8, 0.35, 0.5, hints),
		transferPadSpec("8", -0.975, 0.8, 0.35, 0.5, hints),
	}
}

func boschLGA8_2_5TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -0.975, -1.025, 0.35, 0.5, hints),
		transferPadSpec("2", -0.325, -1.025, 0.35, 0.5, hints),
		transferPadSpec("3", 0.325, -1.025, 0.35, 0.5, hints),
		transferPadSpec("4", 0.975, -1.025, 0.35, 0.5, hints),
		transferPadSpec("5", 0.975, 1.025, 0.35, 0.5, hints),
		transferPadSpec("6", 0.325, 1.025, 0.35, 0.5, hints),
		transferPadSpec("7", -0.325, 1.025, 0.35, 0.5, hints),
		transferPadSpec("8", -0.975, 1.025, 0.35, 0.5, hints),
	}
}

func sensirionDFN8TransferPads(hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -1.175, -0.75, 0.55, 0.25, hints),
		transferPadSpec("2", -1.175, -0.25, 0.55, 0.25, hints),
		transferPadSpec("3", -1.175, 0.25, 0.55, 0.25, hints),
		transferPadSpec("4", -1.175, 0.75, 0.55, 0.25, hints),
		transferPadSpec("5", 1.175, 0.75, 0.55, 0.25, hints),
		transferPadSpec("6", 1.175, 0.25, 0.55, 0.25, hints),
		transferPadSpec("7", 1.175, -0.25, 0.55, 0.25, hints),
		transferPadSpec("8", 1.175, -0.75, 0.55, 0.25, hints),
		transferPadSpec("9", 0, 0, 1.0, 1.7, hints),
	}
}

func tqfp32TransferPads(hints map[string]string) []transactions.PadSpec {
	pads := make([]transactions.PadSpec, 0, 32)
	for number := 1; number <= 8; number++ {
		pads = append(pads, transferPadSpec(strconv.Itoa(number), -4.1625, -2.8+float64(number-1)*0.8, 1.475, 0.55, hints))
	}
	for number := 9; number <= 16; number++ {
		pads = append(pads, transferPadSpec(strconv.Itoa(number), -2.8+float64(number-9)*0.8, 4.1625, 0.55, 1.475, hints))
	}
	for number := 17; number <= 24; number++ {
		pads = append(pads, transferPadSpec(strconv.Itoa(number), 4.1625, 2.8-float64(number-17)*0.8, 1.475, 0.55, hints))
	}
	for number := 25; number <= 32; number++ {
		pads = append(pads, transferPadSpec(strconv.Itoa(number), 2.8-float64(number-25)*0.8, -4.1625, 0.55, 1.475, hints))
	}
	return pads
}

func usbCGCTUSB4125TransferPads(hints map[string]string) []transactions.PadSpec {
	pads := []transactions.PadSpec{
		transferPadSpec("A5", -0.5, -3.08, 0.7, 1.2, hints),
		transferPadSpec("A9", 1.52, -3.08, 0.76, 1.2, hints),
		transferPadSpec("A12", 2.75, -3.08, 0.8, 1.2, hints),
		transferPadSpec("B5", 0.5, -3.08, 0.7, 1.2, hints),
		transferPadSpec("B9", -1.52, -3.08, 0.76, 1.2, hints),
		transferPadSpec("B12", -2.75, -3.08, 0.8, 1.2, hints),
		transferPadSpec("SH", -4.32, -3.0, 1.1, 1.7, hints),
		transferPadSpec("SH", -4.32, 0.8, 1.1, 1.7, hints),
		transferPadSpec("SH", 4.32, -3.0, 1.1, 1.7, hints),
		transferPadSpec("SH", 4.32, 0.8, 1.1, 1.7, hints),
	}
	if hints == nil {
		return pads
	}
	if net := strings.TrimSpace(hints["SH"]); net != "" {
		for i := range pads {
			if strings.HasPrefix(pads[i].Name, "SH") {
				pads[i].Net = &net
			}
		}
	}
	return pads
}

type rowPadTemplate struct {
	rowSpacingMM float64
	pitchMM      float64
	padSizeXMM   float64
	padSizeYMM   float64
	leftNames    []string
	rightNames   []string
}

var soic8NarrowTransferTemplate = rowPadTemplate{
	rowSpacingMM: 5.9,
	pitchMM:      1.27,
	padSizeXMM:   1.55,
	padSizeYMM:   0.6,
	leftNames:    []string{"1", "2", "3", "4"},
	rightNames:   []string{"8", "7", "6", "5"},
}

func (template rowPadTemplate) pads(hints map[string]string) []transactions.PadSpec {
	return rowTransferPads(template.rowSpacingMM, template.pitchMM, template.padSizeXMM, template.padSizeYMM, template.leftNames, template.rightNames, hints)
}

func twoTransferPads(widthMM float64, heightMM float64, pitchMM float64, hints map[string]string) []transactions.PadSpec {
	return []transactions.PadSpec{
		transferPadSpec("1", -pitchMM/2, 0, widthMM, heightMM, hints),
		transferPadSpec("2", pitchMM/2, 0, widthMM, heightMM, hints),
	}
}

// rowTransferPads emits centered coordinates for verified KiCad library footprints.
func rowTransferPads(rowSpacingMM float64, pitchMM float64, padSizeXMM float64, padSizeYMM float64, leftNames []string, rightNames []string, hints map[string]string) []transactions.PadSpec {
	pads := make([]transactions.PadSpec, 0, len(leftNames)+len(rightNames))
	rowX := rowSpacingMM / 2
	leftStart := -float64(len(leftNames)-1) * pitchMM / 2
	for index, name := range leftNames {
		pads = append(pads, transferPadSpec(name, -rowX, leftStart+float64(index)*pitchMM, padSizeXMM, padSizeYMM, hints))
	}
	rightStart := -float64(len(rightNames)-1) * pitchMM / 2
	for index, name := range rightNames {
		pads = append(pads, transferPadSpec(name, rowX, rightStart+float64(index)*pitchMM, padSizeXMM, padSizeYMM, hints))
	}
	return pads
}

func pinHeaderTransferPads(count int, hints map[string]string) []transactions.PadSpec {
	if count <= 0 {
		return nil
	}
	pads := make([]transactions.PadSpec, 0, count)
	offset := float64(count-1) * 2.54 / 2.0
	for i := 1; i <= count; i++ {
		pads = append(pads, transferPadSpec(strconv.Itoa(i), 0, float64(i-1)*2.54-offset, 1.7, 1.7, hints))
	}
	return pads
}

func transferPadSpec(name string, xMM float64, yMM float64, widthMM float64, heightMM float64, hints map[string]string) transactions.PadSpec {
	spec := transactions.PadSpec{Name: name, XMM: xMM, YMM: yMM, WidthMM: widthMM, HeightMM: heightMM}
	if hints == nil {
		return spec
	}
	if net := strings.TrimSpace(hints[name]); net != "" {
		spec.Net = &net
	}
	return spec
}

func countPadNetHints(pads []transactions.PadSpec) int {
	count := 0
	for _, pad := range pads {
		if pad.Net != nil && strings.TrimSpace(*pad.Net) != "" {
			count++
		}
	}
	return count
}

func symbolProperty(symbol schematic.SchematicSymbol, name string) string {
	for _, property := range symbol.Properties {
		if strings.EqualFold(strings.TrimSpace(property.Name), name) {
			return strings.TrimSpace(property.Value)
		}
	}
	return ""
}

func newOperation(kind transactions.OperationKind, payload any, ref string) (transactions.Operation, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithRef(kind, raw, ref), nil
}

func issue(code reports.Code, severity reports.Severity, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: severity, Path: path, Message: message}
}

func addPoint(a kicadfiles.Point, b kicadfiles.Point) kicadfiles.Point {
	return kicadfiles.Point{X: a.X + b.X, Y: a.Y + b.Y}
}

func transformPinPoint(point kicadfiles.Point, mirror schematic.SymbolMirror, angle kicadfiles.Angle) kicadfiles.Point {
	point = mirrorPoint(point, mirror)
	return rotatePoint(point, angle)
}

func mirrorPoint(point kicadfiles.Point, mirror schematic.SymbolMirror) kicadfiles.Point {
	switch mirror {
	case schematic.SymbolMirrorX:
		point.X = -point.X
	case schematic.SymbolMirrorY:
		point.Y = -point.Y
	}
	return point
}

func rotatePoint(point kicadfiles.Point, angle kicadfiles.Angle) kicadfiles.Point {
	if angle == 0 {
		return point
	}
	radians := float64(angle) * math.Pi / 180
	sin, cos := math.Sin(radians), math.Cos(radians)
	x := float64(point.X)
	y := float64(point.Y)
	return kicadfiles.Point{X: kicadfiles.IU(math.Round(x*cos + y*sin)), Y: kicadfiles.IU(math.Round(-x*sin + y*cos))}
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / 1_000_000.0
}

type graphPoint = kicadfiles.Point

func pointKey(point kicadfiles.Point) graphPoint {
	return point
}

func pointOnSegment(point kicadfiles.Point, start kicadfiles.Point, end kicadfiles.Point) bool {
	// Coordinates are already normalized to integer KiCad internal units, so
	// exact collinearity is intentional here.
	cross := (int64(point.Y)-int64(start.Y))*(int64(end.X)-int64(start.X)) -
		(int64(point.X)-int64(start.X))*(int64(end.Y)-int64(start.Y))
	return cross == 0
}

func pointInSegmentBounds(point kicadfiles.Point, start kicadfiles.Point, end kicadfiles.Point) bool {
	minX, maxX := orderedIU(start.X, end.X)
	minY, maxY := orderedIU(start.Y, end.Y)
	return point.X >= minX && point.X <= maxX && point.Y >= minY && point.Y <= maxY
}

func orderedIU(a kicadfiles.IU, b kicadfiles.IU) (kicadfiles.IU, kicadfiles.IU) {
	if a <= b {
		return a, b
	}
	return b, a
}

type terminalIndex struct {
	all []kicadfiles.Point
	byX map[kicadfiles.IU][]kicadfiles.Point
	byY map[kicadfiles.IU][]kicadfiles.Point
}

func newTerminalIndex(terminals []kicadfiles.Point) terminalIndex {
	index := terminalIndex{
		all: terminals,
		byX: map[kicadfiles.IU][]kicadfiles.Point{},
		byY: map[kicadfiles.IU][]kicadfiles.Point{},
	}
	for _, terminal := range terminals {
		index.byX[terminal.X] = append(index.byX[terminal.X], terminal)
		index.byY[terminal.Y] = append(index.byY[terminal.Y], terminal)
	}
	return index
}

func (index terminalIndex) candidates(start kicadfiles.Point, end kicadfiles.Point) []kicadfiles.Point {
	if start.Y == end.Y {
		return index.byY[start.Y]
	}
	if start.X == end.X {
		return index.byX[start.X]
	}
	return index.all
}

type netGraph struct {
	parent map[graphPoint]graphPoint
	labels map[graphPoint][]string
}

func newNetGraph() *netGraph {
	return &netGraph{parent: map[graphPoint]graphPoint{}, labels: map[graphPoint][]string{}}
}

func (graph *netGraph) ensure(key graphPoint) {
	if _, ok := graph.parent[key]; !ok {
		graph.parent[key] = key
	}
}

func (graph *netGraph) find(key graphPoint) graphPoint {
	graph.ensure(key)
	parent := graph.parent[key]
	if parent != key {
		graph.parent[key] = graph.find(parent)
	}
	return graph.parent[key]
}

func (graph *netGraph) union(a graphPoint, b graphPoint) {
	rootA := graph.find(a)
	rootB := graph.find(b)
	if rootA == rootB {
		return
	}
	if graphPointLess(rootB, rootA) {
		rootA, rootB = rootB, rootA
	}
	graph.parent[rootB] = rootA
}

func (graph *netGraph) label(key graphPoint, label string) {
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	graph.labels[graph.find(key)] = append(graph.labels[graph.find(key)], label)
}

func (graph *netGraph) netNames() map[graphPoint]string {
	grouped := map[graphPoint][]string{}
	for key, labels := range graph.labels {
		root := graph.find(key)
		grouped[root] = append(grouped[root], labels...)
	}
	result := map[graphPoint]string{}
	for root, labels := range grouped {
		sort.Strings(labels)
		result[root] = labels[0]
	}
	return result
}

func graphPointLess(a graphPoint, b graphPoint) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

func naturalRefLess(a string, b string) bool {
	prefixA, numberA, okA := splitRef(a)
	prefixB, numberB, okB := splitRef(b)
	if okA && okB && prefixA == prefixB && numberA != numberB {
		return numberA < numberB
	}
	return a < b
}

func splitRef(ref string) (string, int, bool) {
	i := len(ref)
	for i > 0 && ref[i-1] >= '0' && ref[i-1] <= '9' {
		i--
	}
	if i == len(ref) {
		return ref, 0, false
	}
	var number int
	for _, r := range ref[i:] {
		number = number*10 + int(r-'0')
	}
	return ref[:i], number, true
}
