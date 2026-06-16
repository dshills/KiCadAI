package schematicpcb

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
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
	Symbols     []schematic.SchematicSymbol
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
	if len(design.Schematic.Sheets) > 0 {
		result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, "schematic.sheets", "hierarchical schematic transfer is not implemented yet"))
		return result
	}
	components := schematicComponents(*design.Schematic, &result)
	result.SymbolCount = len(design.Schematic.Symbols)
	result.AssignedCount = len(components)
	if opts.LibraryIndex == nil {
		result.RequiresLibraries = true
		result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityWarning, "library_index", "library index not provided; generated placement transaction will omit pad net hints"))
	}
	netHints := map[string]map[string]string{}
	if opts.LibraryIndex != nil {
		netHints = inferPinNetHints(*design.Schematic, components, *opts.LibraryIndex, &result)
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
			} else {
				result.Issues = append(result.Issues, issue(reports.CodeUnknownFootprintLibrary, reports.SeverityWarning, "footprint."+component.Ref, "footprint record not found for "+component.FootprintID))
			}
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

func schematicComponents(file schematic.SchematicFile, result *Result) []component {
	byRef := map[string]*component{}
	for i, symbol := range file.Symbols {
		ref := strings.TrimSpace(symbol.Reference)
		if ref == "" || strings.HasPrefix(ref, "#") {
			continue
		}
		footprintID := symbolProperty(symbol, "Footprint")
		if existing, ok := byRef[ref]; ok {
			existing.Symbols = append(existing.Symbols, symbol)
			if symbol.Value != "" && existing.Value != "" && symbol.Value != existing.Value {
				result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("symbols[%d].Value", i), "multi-unit schematic reference "+ref+" has conflicting value "+symbol.Value))
			} else if existing.Value == "" {
				existing.Value = symbol.Value
			}
			if footprintID != "" && existing.FootprintID != "" && footprintID != existing.FootprintID {
				result.Issues = append(result.Issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("symbols[%d].Footprint", i), "multi-unit schematic reference "+ref+" has conflicting footprint "+footprintID))
			} else if existing.FootprintID == "" {
				existing.FootprintID = footprintID
			}
			continue
		}
		byRef[ref] = &component{Ref: ref, Value: symbol.Value, LibraryID: symbol.LibraryID, FootprintID: footprintID, Symbols: []schematic.SchematicSymbol{symbol}}
	}
	components := make([]component, 0, len(byRef))
	for _, component := range byRef {
		if component.FootprintID == "" {
			result.Issues = append(result.Issues, issue(reports.CodeMissingFootprint, reports.SeverityWarning, "symbols."+component.Ref+".Footprint", "schematic symbol "+component.Ref+" has no assigned footprint"))
			continue
		}
		components = append(components, *component)
	}
	sort.SliceStable(components, func(i, j int) bool { return naturalRefLess(components[i].Ref, components[j].Ref) })
	return components
}

func inferPinNetHints(file schematic.SchematicFile, components []component, index libraryresolver.LibraryIndex, result *Result) map[string]map[string]string {
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
		graph.label(key, label.Text)
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

func collectPinAnchors(components []component, index libraryresolver.LibraryIndex, result *Result) []pinAnchor {
	var anchors []pinAnchor
	for _, component := range components {
		for _, symbol := range component.Symbols {
			record, ok := libraryresolver.ResolveSymbol(index, symbol.LibraryID)
			if !ok {
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
		if net := strings.TrimSpace(hints[pad.Name]); net != "" {
			value := net
			spec.Net = &value
		}
		pads = append(pads, spec)
	}
	return pads
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
