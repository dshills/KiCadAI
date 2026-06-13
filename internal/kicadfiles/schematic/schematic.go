package schematic

import (
	"cmp"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type SchematicFile struct {
	Filename         string
	Version          kicadfiles.KiCadFormatVersion
	Generator        string
	GeneratorVersion string
	UUID             kicadfiles.UUID
	Paper            kicadfiles.Paper
	TitleBlock       kicadfiles.TitleBlock
	LibSymbols       []EmbeddedSymbol
	Symbols          []SchematicSymbol
	Wires            []Wire
	NoConnects       []NoConnect
	Labels           []Label
	Junctions        []Junction
	Buses            []Bus
	Polylines        []Polyline
	BusEntries       []BusEntry
	Texts            []Text
	Sheets           []Sheet
	RawItems         []RawSchematicItem
	// Instances is retained for compatibility with early generators. KiCad 10
	// output renders per-symbol instances from SchematicSymbol.Instances.
	Instances []SymbolInstance
	// SheetInstances renders the root sheet_instances block.
	SheetInstances []SheetInstance
}

type EmbeddedSymbol struct {
	LibraryID string
	Body      sexpr.List
}

type SchematicSymbol struct {
	Raw              string
	UUID             kicadfiles.UUID
	Path             string
	LibraryID        string
	Reference        string
	Value            string
	Position         kicadfiles.Point
	Rotation         kicadfiles.Angle
	Mirror           SymbolMirror
	Unit             int
	BodyStyle        int
	ExcludeFromSim   bool
	InBOM            *bool
	OnBoard          *bool
	InPositionFile   *bool
	DoNotPopulate    bool
	Passthrough      SymbolPassthrough
	Locked           bool
	FieldsAutoplaced bool
	Properties       []Property
	// Fields is retained for compatibility with early generators. New code
	// should prefer Properties so KiCad property flags can be represented.
	Fields []Field
	Pins   []SymbolPin
	// PinAnchors are absolute schematic coordinates supplied by generators for
	// connectivity validation. The writer does not derive them from libraries.
	PinAnchors []kicadfiles.Point
	Instances  []SymbolInstance
}

type Field struct {
	Name  string
	Value string
	// Visible is the legacy field visibility flag. Hidden takes precedence
	// when both are set during compatibility conversion.
	Visible  bool
	Hidden   bool
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

type Property struct {
	Name           string
	Value          string
	Private        bool
	Hidden         bool
	ShowName       *bool
	DoNotAutoplace *bool
	Position       kicadfiles.Point
	Rotation       kicadfiles.Angle
}

type SymbolPin struct {
	Number    string
	UUID      kicadfiles.UUID
	Alternate string
}

type SymbolMirror string

const (
	SymbolMirrorNone SymbolMirror = ""
	SymbolMirrorX    SymbolMirror = "x"
	SymbolMirrorY    SymbolMirror = "y"
)

var schematicZero = sexpr.X("0")

type SymbolPassthrough string

const (
	SymbolPassthroughDefault SymbolPassthrough = ""
	SymbolPassthroughYes     SymbolPassthrough = "yes"
	SymbolPassthroughNo      SymbolPassthrough = "no"
)

type Wire struct {
	UUID   kicadfiles.UUID
	Points []kicadfiles.Point
}

type Bus struct {
	UUID   kicadfiles.UUID
	Points []kicadfiles.Point
}

type Polyline struct {
	UUID   kicadfiles.UUID
	Points []kicadfiles.Point
}

type BusEntry struct {
	UUID     kicadfiles.UUID
	Position kicadfiles.Point
	Size     kicadfiles.Point
}

type Text struct {
	UUID     kicadfiles.UUID
	Value    string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Locked   bool
}

type Label struct {
	UUID             kicadfiles.UUID
	Text             string
	Kind             LabelKind
	Shape            LabelShape
	Position         kicadfiles.Point
	Rotation         kicadfiles.Angle
	Locked           bool
	FieldsAutoplaced bool
	Fields           []Field
}

type LabelKind string

const (
	LabelLocal        LabelKind = "label"
	LabelGlobal       LabelKind = "global_label"
	LabelHierarchical LabelKind = "hierarchical_label"
	LabelDirective    LabelKind = "directive_label"
)

type LabelShape string

const (
	LabelShapeInput         LabelShape = "input"
	LabelShapeOutput        LabelShape = "output"
	LabelShapeBidirectional LabelShape = "bidirectional"
	LabelShapeTriState      LabelShape = "tri_state"
	LabelShapePassive       LabelShape = "passive"
)

type Junction struct {
	UUID     kicadfiles.UUID
	Position kicadfiles.Point
	Diameter kicadfiles.IU
	Color    Color
}

type NoConnect struct {
	UUID     kicadfiles.UUID
	Position kicadfiles.Point
}

type Color struct {
	R int
	G int
	B int
	A int
}

// 10 mils in KiCad internal units; close enough to catch visual near-misses
// without masking intentional grid-separated endpoints.
const connectivityNearMissDistance = kicadfiles.IU(254000)

type Sheet struct {
	UUID             kicadfiles.UUID
	Name             string
	Filename         string
	Position         kicadfiles.Point
	Size             kicadfiles.Point
	ExcludeFromSim   bool
	InBOM            *bool
	OnBoard          *bool
	DoNotPopulate    bool
	Locked           bool
	FieldsAutoplaced bool
	Properties       []Property
	Pins             []SheetPin
	Instances        []SheetInstance
}

type SymbolInstance struct {
	Project   string
	Path      string
	Reference string
	Unit      int
	Value     string
}

type SheetPin struct {
	UUID     kicadfiles.UUID
	Text     string
	Kind     SheetPinKind
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

type SheetPinKind string

const (
	SheetPinInput         SheetPinKind = "input"
	SheetPinOutput        SheetPinKind = "output"
	SheetPinBidirectional SheetPinKind = "bidirectional"
	SheetPinTriState      SheetPinKind = "tri_state"
	SheetPinPassive       SheetPinKind = "passive"
)

type RawSchematicItem struct {
	UUID kicadfiles.UUID
	// Order optionally pins this raw item into the KiCad top-level item order.
	// Zero keeps the writer's normal KiCad type/UUID ordering.
	Order int
	// Kind is the KiCad schematic item node name, such as "bitmap" or
	// "rule_area". When omitted, the writer infers it from Body. Body must be
	// a comment-free S-expression fragment whose UUID matches UUID.
	Kind RawSchematicItemKind
	Body sexpr.Raw
}

type RawSchematicItemKind string

const (
	RawItemJunction          RawSchematicItemKind = "junction"
	RawItemNoConnect         RawSchematicItemKind = "no_connect"
	RawItemBusEntry          RawSchematicItemKind = "bus_entry"
	RawItemWire              RawSchematicItemKind = "wire"
	RawItemBus               RawSchematicItemKind = "bus"
	RawItemPolyline          RawSchematicItemKind = "polyline"
	RawItemBitmap            RawSchematicItemKind = "bitmap"
	RawItemTable             RawSchematicItemKind = "table"
	RawItemText              RawSchematicItemKind = "text"
	RawItemLabel             RawSchematicItemKind = "label"
	RawItemGlobalLabel       RawSchematicItemKind = "global_label"
	RawItemHierarchicalLabel RawSchematicItemKind = "hierarchical_label"
	RawItemRuleArea          RawSchematicItemKind = "rule_area"
	RawItemDirectiveLabel    RawSchematicItemKind = "directive_label"
	RawItemSymbol            RawSchematicItemKind = "symbol"
	RawItemGroup             RawSchematicItemKind = "group"
	RawItemSheet             RawSchematicItemKind = "sheet"
)

type SheetInstance struct {
	Project string
	Path    string
	Page    string
}

type schematicItemKind int

const (
	// Values intentionally follow KiCad schematic item save order from typeinfo.h.
	// Leave gaps so new supported item types can be inserted without renumbering.
	schematicItemJunction          schematicItemKind = 10
	schematicItemNoConnect         schematicItemKind = 20
	schematicItemWireToBusEntry    schematicItemKind = 30
	schematicItemBusToBusEntry     schematicItemKind = 40
	schematicItemLine              schematicItemKind = 50
	schematicItemBitmap            schematicItemKind = 60
	schematicItemTable             schematicItemKind = 70
	schematicItemTableCell         schematicItemKind = 80
	schematicItemText              schematicItemKind = 85
	schematicItemLabel             schematicItemKind = 90
	schematicItemGlobalLabel       schematicItemKind = 100
	schematicItemHierarchicalLabel schematicItemKind = 110
	schematicItemRuleArea          schematicItemKind = 120
	schematicItemDirectiveLabel    schematicItemKind = 130
	schematicItemSymbol            schematicItemKind = 140
	schematicItemGroup             schematicItemKind = 150
	schematicItemSheetPin          schematicItemKind = 160
	schematicItemSheet             schematicItemKind = 170
	schematicItemUnknownRaw        schematicItemKind = 10000
)

const schematicHeaderNodeCapacity = 8

type renderItem struct {
	kind  schematicItemKind
	order int
	uuid  kicadfiles.UUID
	node  sexpr.Node
}

type LEDIndicatorInput struct {
	Name            string
	DesignID        kicadfiles.UUID
	Seed            string
	IncludePCB      bool
	LibraryVCC      string
	LibraryGND      string
	LibraryResistor string
	LibraryLED      string
}

func Validate(schematic SchematicFile) error {
	var errs kicadfiles.ValidationErrors
	var versionNumber int64
	versionText := string(schematic.Version)
	if versionText == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if parsed, err := strconv.ParseInt(versionText, 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	} else {
		versionNumber = parsed
	}
	if strings.TrimSpace(schematic.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if versionNumber >= kicadfiles.KiCadSchematicFormatWithGeneratorVersion && strings.TrimSpace(schematic.GeneratorVersion) == "" {
		errs = append(errs, fieldError("generator_version", fmt.Sprintf("required for schematic versions %d and newer", kicadfiles.KiCadSchematicFormatWithGeneratorVersion)))
	}
	if !schematic.UUID.Valid() {
		errs = append(errs, fieldError("uuid", "valid UUID required"))
	}
	if strings.TrimSpace(schematic.Paper.Name) == "" {
		errs = append(errs, fieldError("paper", "required"))
	}
	if len(schematic.TitleBlock.Comments) > 9 {
		errs = append(errs, fieldError("title_block.comments", "at most 9 comments allowed"))
	}
	for i, symbol := range schematic.Symbols {
		errs = append(errs, validateSymbol(i, symbol)...)
	}
	for i, wire := range schematic.Wires {
		errs = append(errs, validateWire(i, wire)...)
	}
	for i, bus := range schematic.Buses {
		errs = append(errs, validateLineLike(indexed("buses", i, ""), bus.UUID, bus.Points)...)
	}
	for i, polyline := range schematic.Polylines {
		errs = append(errs, validateLineLike(indexed("polylines", i, ""), polyline.UUID, polyline.Points)...)
	}
	for i, entry := range schematic.BusEntries {
		errs = append(errs, validateBusEntry(i, entry)...)
	}
	for i, text := range schematic.Texts {
		errs = append(errs, validateText(i, text)...)
	}
	for i, noConnect := range schematic.NoConnects {
		if !noConnect.UUID.Valid() {
			errs = append(errs, fieldError(indexed("no_connects", i, "uuid"), "valid UUID required"))
		}
	}
	for i, label := range schematic.Labels {
		errs = append(errs, validateLabel(i, label)...)
	}
	for i, junction := range schematic.Junctions {
		errs = append(errs, validateJunction(i, junction)...)
	}
	seenSheets := map[string]struct{}{}
	for i, sheet := range schematic.Sheets {
		errs = append(errs, validateSheet(i, sheet)...)
		name := strings.TrimSpace(sheet.Name)
		if name != "" {
			if _, ok := seenSheets[name]; ok {
				errs = append(errs, fieldError(indexed("sheets", i, "name"), "duplicate "+name))
			}
			seenSheets[name] = struct{}{}
		}
	}
	rawMetadata := make([]rawSchematicItemValidationMetadata, len(schematic.RawItems))
	for i, raw := range schematic.RawItems {
		rawErrs, metadata := validateRawSchematicItem(i, raw)
		errs = append(errs, rawErrs...)
		rawMetadata[i] = metadata
	}
	for i, instance := range schematic.SheetInstances {
		errs = append(errs, validateSheetInstance(indexed("sheet_instances", i, ""), instance)...)
	}
	errs = append(errs, validateUniqueSchematicItemUUIDs(schematic, rawMetadata)...)
	return errs.Err()
}

func ValidateGeneratedConnectivity(schematic SchematicFile) error {
	if err := Validate(schematic); err != nil {
		return err
	}
	anchors := schematicConnectivityAnchors(schematic)
	anchorIndex := newAnchorIndex(anchors)
	symbolAnchors := schematicSymbolPinAnchorSet(schematic)
	var errs kicadfiles.ValidationErrors
	for wireIndex, wire := range schematic.Wires {
		for _, endpoint := range wireEndpoints(wire) {
			pointIndex := endpoint.index
			point := endpoint.point
			if anchors[point] > 1 {
				continue
			}
			if near, ok := anchorIndex.nearest(point); ok {
				errs = append(errs, fieldError(indexed("wires", wireIndex, "points")+"["+strconv.Itoa(pointIndex)+"]", "endpoint is near but not on anchor at "+formatPoint(near)))
				continue
			}
			errs = append(errs, fieldError(indexed("wires", wireIndex, "points")+"["+strconv.Itoa(pointIndex)+"]", "endpoint is not connected to a known anchor"))
		}
	}
	for labelIndex, label := range schematic.Labels {
		if anchors[label.Position] <= 1 {
			errs = append(errs, fieldError(indexed("labels", labelIndex, "position"), "label is not connected to a known anchor"))
		}
	}
	for junctionIndex, junction := range schematic.Junctions {
		if anchors[junction.Position] <= 1 {
			errs = append(errs, fieldError(indexed("junctions", junctionIndex, "position"), "junction is not connected to a known anchor"))
		}
	}
	for noConnectIndex, noConnect := range schematic.NoConnects {
		if anchors[noConnect.Position] <= 1 {
			errs = append(errs, fieldError(indexed("no_connects", noConnectIndex, "position"), "no-connect is not connected to a known anchor"))
		} else if _, ok := symbolAnchors[noConnect.Position]; !ok {
			errs = append(errs, fieldError(indexed("no_connects", noConnectIndex, "position"), "no-connect must be placed on a symbol pin anchor"))
		}
	}
	for symbolIndex, symbol := range schematic.Symbols {
		for pinIndex, point := range symbol.PinAnchors {
			if anchors[point] <= 1 {
				errs = append(errs, fieldError(indexed("symbols", symbolIndex, "pin_anchors")+"["+strconv.Itoa(pinIndex)+"]", "symbol pin anchor is not connected"))
			}
		}
	}
	for sheetIndex, sheet := range schematic.Sheets {
		for pinIndex, pin := range sheet.Pins {
			if anchors[pin.Position] <= 1 {
				errs = append(errs, fieldError(indexed(indexed("sheets", sheetIndex, "pins"), pinIndex, "position"), "sheet pin is not connected to a known anchor"))
			}
		}
	}
	return errs.Err()
}

func schematicSymbolPinAnchorSet(schematic SchematicFile) map[kicadfiles.Point]struct{} {
	anchors := map[kicadfiles.Point]struct{}{}
	for _, symbol := range schematic.Symbols {
		for _, point := range symbol.PinAnchors {
			anchors[point] = struct{}{}
		}
	}
	return anchors
}

func schematicConnectivityAnchors(schematic SchematicFile) map[kicadfiles.Point]int {
	anchors := make(map[kicadfiles.Point]int, len(schematic.Wires)*2+len(schematic.Labels)+len(schematic.Junctions)+len(schematic.NoConnects))
	seen := map[kicadfiles.Point]struct{}{}
	for _, wire := range schematic.Wires {
		clear(seen)
		for _, point := range wire.Points {
			seen[point] = struct{}{}
		}
		addAnchorPoints(anchors, seen)
	}
	for _, label := range schematic.Labels {
		// Separate schematic items at the same coordinate are distinct anchors.
		anchors[label.Position]++
	}
	for _, junction := range schematic.Junctions {
		anchors[junction.Position]++
	}
	for _, noConnect := range schematic.NoConnects {
		anchors[noConnect.Position]++
	}
	for _, symbol := range schematic.Symbols {
		clear(seen)
		for _, point := range symbol.PinAnchors {
			seen[point] = struct{}{}
		}
		addAnchorPoints(anchors, seen)
	}
	for _, sheet := range schematic.Sheets {
		clear(seen)
		for _, pin := range sheet.Pins {
			seen[pin.Position] = struct{}{}
		}
		addAnchorPoints(anchors, seen)
	}
	return anchors
}

func addAnchorPoints(anchors map[kicadfiles.Point]int, points map[kicadfiles.Point]struct{}) {
	for point := range points {
		anchors[point]++
	}
}

type wireEndpoint struct {
	index int
	point kicadfiles.Point
}

func wireEndpoints(wire Wire) []wireEndpoint {
	if len(wire.Points) == 0 {
		return nil
	}
	if len(wire.Points) == 1 {
		return []wireEndpoint{{index: 0, point: wire.Points[0]}}
	}
	return []wireEndpoint{
		{index: 0, point: wire.Points[0]},
		{index: len(wire.Points) - 1, point: wire.Points[len(wire.Points)-1]},
	}
}

type anchorBucket struct {
	x int64
	y int64
}

type anchorIndex map[anchorBucket][]kicadfiles.Point

func newAnchorIndex(anchors map[kicadfiles.Point]int) anchorIndex {
	index := make(anchorIndex, len(anchors))
	for point := range anchors {
		bucket := anchorBucketFor(point)
		index[bucket] = append(index[bucket], point)
	}
	return index
}

func (index anchorIndex) nearest(point kicadfiles.Point) (kicadfiles.Point, bool) {
	var nearest kicadfiles.Point
	var nearestDistance kicadfiles.IU
	found := false
	center := anchorBucketFor(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, anchor := range index[anchorBucket{x: center.x + dx, y: center.y + dy}] {
				if anchor == point {
					continue
				}
				distance := manhattanDistance(point, anchor)
				if distance > connectivityNearMissDistance {
					continue
				}
				if !found || distance < nearestDistance || (distance == nearestDistance && pointLess(anchor, nearest)) {
					found = true
					nearest = anchor
					nearestDistance = distance
				}
			}
		}
	}
	return nearest, found
}

func anchorBucketFor(point kicadfiles.Point) anchorBucket {
	return anchorBucket{
		x: floorDiv(int64(point.X), int64(connectivityNearMissDistance)),
		y: floorDiv(int64(point.Y), int64(connectivityNearMissDistance)),
	}
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor
	remainder := value % divisor
	if remainder != 0 && ((remainder < 0) != (divisor < 0)) {
		quotient--
	}
	return quotient
}

func pointLess(a, b kicadfiles.Point) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

func manhattanDistance(a, b kicadfiles.Point) kicadfiles.IU {
	return absIU(a.X-b.X) + absIU(a.Y-b.Y)
}

func absIU(value kicadfiles.IU) kicadfiles.IU {
	if value < 0 {
		return -value
	}
	return value
}

func formatPoint(point kicadfiles.Point) string {
	return "(" + kicadfiles.ToMMString(point.X) + "," + kicadfiles.ToMMString(point.Y) + ")"
}

func Write(w io.Writer, schematic SchematicFile) error {
	if err := Validate(schematic); err != nil {
		return err
	}
	node, err := render(schematic)
	if err != nil {
		return err
	}
	return sexpr.Render(w, node)
}

func render(schematic SchematicFile) (sexpr.List, error) {
	version, err := versionInt(schematic.Version)
	if err != nil {
		return nil, err
	}
	items, err := renderItems(schematic)
	if err != nil {
		return nil, err
	}
	nodes := make([]sexpr.Node, 0, len(items)+schematicHeaderNodeCapacity)
	nodes = append(nodes,
		sexpr.A("kicad_sch"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(schematic.Generator))),
	)
	if generatorVersion := strings.TrimSpace(schematic.GeneratorVersion); generatorVersion != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("generator_version"), sexpr.S(generatorVersion)))
	}
	nodes = append(nodes,
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(schematic.UUID))),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(schematic.Paper.Name))),
	)
	if title := renderTitleBlock(schematic.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	// KiCad's schematic writer emits lib_symbols even when the cache is empty.
	nodes = append(nodes, renderLibSymbols(schematic.LibSymbols))
	for _, item := range items {
		nodes = append(nodes, item.node)
	}
	nodes = append(nodes, renderRootSheetInstances(schematic.SheetInstances))
	return sexpr.L(nodes...), nil
}

func renderItems(schematic SchematicFile) ([]renderItem, error) {
	itemCount := len(schematic.Junctions) + len(schematic.NoConnects) + len(schematic.BusEntries) +
		len(schematic.Wires) + len(schematic.Buses) + len(schematic.Polylines) +
		len(schematic.Texts) + len(schematic.Labels) + len(schematic.Symbols) +
		len(schematic.Sheets) + len(schematic.RawItems)
	items := make([]renderItem, 0, itemCount)
	for _, junction := range schematic.Junctions {
		items = append(items, newRenderItem(schematicItemJunction, junction.UUID, renderJunction(junction)))
	}
	for _, noConnect := range schematic.NoConnects {
		items = append(items, newRenderItem(schematicItemNoConnect, noConnect.UUID, renderNoConnect(noConnect)))
	}
	for _, entry := range schematic.BusEntries {
		items = append(items, newRenderItem(schematicItemWireToBusEntry, entry.UUID, renderBusEntry(entry)))
	}
	for _, wire := range schematic.Wires {
		items = append(items, newRenderItem(schematicItemLine, wire.UUID, renderWire(wire)))
	}
	// KiCad saves wires, buses, and graphical polylines as SCH_LINE_T items.
	for _, bus := range schematic.Buses {
		items = append(items, newRenderItem(schematicItemLine, bus.UUID, renderBus(bus)))
	}
	for _, polyline := range schematic.Polylines {
		items = append(items, newRenderItem(schematicItemLine, polyline.UUID, renderPolyline(polyline)))
	}
	for _, text := range schematic.Texts {
		items = append(items, newRenderItem(schematicItemText, text.UUID, renderText(text)))
	}
	for _, label := range schematic.Labels {
		items = append(items, newRenderItem(labelItemKind(label.Kind), label.UUID, renderLabel(label)))
	}
	for _, symbol := range schematic.Symbols {
		items = append(items, newRenderItem(schematicItemSymbol, symbol.UUID, renderSymbol(symbol)))
	}
	for _, sheet := range schematic.Sheets {
		items = append(items, newRenderItem(schematicItemSheet, sheet.UUID, renderSheet(sheet)))
	}
	for i, raw := range schematic.RawItems {
		effectiveKind := raw.effectiveKind()
		kind, ok := rawSchematicItemKind(effectiveKind)
		if !ok {
			return nil, fieldError(indexed("raw_items", i, "kind"), "unsupported schematic item kind "+string(effectiveKind))
		}
		items = append(items, newRawRenderItem(kind, raw.Order, raw.UUID, sexpr.R(string(raw.Body))))
	}
	// Keep the sort stable so invalid duplicate UUID input still renders
	// reproducibly before validation grows strict global UUID checks.
	slices.SortStableFunc(items, func(a, b renderItem) int {
		if a.order != b.order {
			return cmp.Compare(a.order, b.order)
		}
		return cmp.Compare(a.uuid, b.uuid)
	})
	return items, nil
}

func newRenderItem(kind schematicItemKind, uuid kicadfiles.UUID, node sexpr.Node) renderItem {
	return renderItem{kind: kind, order: int(kind) * 1000, uuid: uuid, node: node}
}

func newRawRenderItem(kind schematicItemKind, order int, uuid kicadfiles.UUID, node sexpr.Node) renderItem {
	if order <= 0 {
		order = int(kind) * 1000
	}
	return renderItem{kind: kind, order: order, uuid: uuid, node: node}
}

func labelItemKind(kind LabelKind) schematicItemKind {
	switch kind {
	case LabelLocal:
		return schematicItemLabel
	case LabelGlobal:
		return schematicItemGlobalLabel
	case LabelHierarchical:
		return schematicItemHierarchicalLabel
	case LabelDirective:
		return schematicItemDirectiveLabel
	default:
		return schematicItemLabel
	}
}

func (raw RawSchematicItem) effectiveKind() RawSchematicItemKind {
	if raw.Kind != "" {
		return raw.Kind
	}
	return RawSchematicItemKind(rawSchematicItemTopLevelAtom(strings.TrimSpace(string(raw.Body))))
}

func rawSchematicItemKind(kind RawSchematicItemKind) (schematicItemKind, bool) {
	switch kind {
	case RawItemJunction:
		return schematicItemJunction, true
	case RawItemNoConnect:
		return schematicItemNoConnect, true
	case RawItemBusEntry:
		// The structured BusEntry type currently represents KiCad's wire-to-bus
		// entry item and is sorted with the same internal kind.
		return schematicItemWireToBusEntry, true
	case RawItemWire, RawItemBus, RawItemPolyline:
		return schematicItemLine, true
	case RawItemBitmap:
		return schematicItemBitmap, true
	case RawItemTable:
		return schematicItemTable, true
	case RawItemText:
		return schematicItemText, true
	case RawItemLabel:
		return schematicItemLabel, true
	case RawItemGlobalLabel:
		return schematicItemGlobalLabel, true
	case RawItemHierarchicalLabel:
		return schematicItemHierarchicalLabel, true
	case RawItemRuleArea:
		return schematicItemRuleArea, true
	case RawItemDirectiveLabel:
		return schematicItemDirectiveLabel, true
	case RawItemSymbol:
		return schematicItemSymbol, true
	case RawItemGroup:
		return schematicItemGroup, true
	case RawItemSheet:
		return schematicItemSheet, true
	default:
		if strings.TrimSpace(string(kind)) == "" {
			return 0, false
		}
		return schematicItemUnknownRaw, true
	}
}

func renderTitleBlock(title kicadfiles.TitleBlock) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("title_block")}
	if title.Title != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("title"), sexpr.S(title.Title)))
	}
	if title.Date != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("date"), sexpr.S(title.Date)))
	}
	if title.Revision != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("rev"), sexpr.S(title.Revision)))
	}
	if title.Company != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("company"), sexpr.S(title.Company)))
	}
	for i, comment := range title.Comments {
		nodes = append(nodes, sexpr.L(sexpr.A("comment"), sexpr.I(int64(i+1)), sexpr.S(comment)))
	}
	return sexpr.L(nodes...)
}

func versionInt(version kicadfiles.KiCadFormatVersion) (int64, error) {
	return strconv.ParseInt(string(version), 10, 64)
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "schematic", Field: field, Message: message}
}

func validateSymbol(index int, symbol SchematicSymbol) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("symbols", index, field) }
	if !symbol.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(symbol.LibraryID) == "" {
		errs = append(errs, fieldError(prefix("library_id"), "required"))
	}
	if strings.TrimSpace(symbol.Reference) == "" {
		errs = append(errs, fieldError(prefix("reference"), "required"))
	}
	if strings.TrimSpace(symbol.Value) == "" {
		errs = append(errs, fieldError(prefix("value"), "required"))
	}
	if !validSymbolMirror(symbol.Mirror) {
		errs = append(errs, fieldError(prefix("mirror"), "invalid"))
	}
	if !validSymbolPassthrough(symbol.Passthrough) {
		errs = append(errs, fieldError(prefix("passthrough"), "invalid"))
	}
	seenProperties := map[string]struct{}{}
	for propertyIndex, property := range symbol.Properties {
		name := strings.TrimSpace(property.Name)
		key := strings.ToLower(name)
		if name == "" {
			errs = append(errs, fieldError(indexed(prefix("properties"), propertyIndex, "name"), "required"))
		}
		if _, ok := seenProperties[key]; ok && name != "" {
			errs = append(errs, fieldError(indexed(prefix("properties"), propertyIndex, "name"), "duplicate "+name))
		}
		seenProperties[key] = struct{}{}
	}
	seenFields := map[string]struct{}{}
	for fieldIndex, field := range symbol.Fields {
		name := strings.TrimSpace(field.Name)
		key := strings.ToLower(name)
		if name == "" {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "required"))
			continue
		}
		if strings.EqualFold(name, "Reference") || strings.EqualFold(name, "Value") {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "reserved; use symbol."+strings.ToLower(name)))
		}
		if _, ok := seenProperties[key]; ok {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "duplicate "+name))
		}
		if _, ok := seenFields[key]; ok {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "duplicate "+name))
		}
		seenFields[key] = struct{}{}
	}
	seenPinUUIDs := map[kicadfiles.UUID]struct{}{}
	seenPinNumbers := map[string]struct{}{}
	for pinIndex, pin := range symbol.Pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			errs = append(errs, fieldError(indexed(prefix("pins"), pinIndex, "number"), "required"))
		} else {
			if _, ok := seenPinNumbers[number]; ok {
				errs = append(errs, fieldError(indexed(prefix("pins"), pinIndex, "number"), "duplicate "+number))
			}
			seenPinNumbers[number] = struct{}{}
		}
		if !pin.UUID.Valid() {
			errs = append(errs, fieldError(indexed(prefix("pins"), pinIndex, "uuid"), "valid UUID required"))
		} else if _, ok := seenPinUUIDs[pin.UUID]; ok {
			errs = append(errs, fieldError(indexed(prefix("pins"), pinIndex, "uuid"), "duplicate "+string(pin.UUID)))
		} else {
			seenPinUUIDs[pin.UUID] = struct{}{}
		}
	}
	seenInstancePaths := map[string]struct{}{}
	for instanceIndex, instance := range symbol.Instances {
		path := strings.TrimSpace(instance.Path)
		if path == "" {
			errs = append(errs, fieldError(indexed(prefix("instances"), instanceIndex, "path"), "required"))
		} else if !strings.HasPrefix(path, "/") {
			errs = append(errs, fieldError(indexed(prefix("instances"), instanceIndex, "path"), "must start with /"))
		} else if _, ok := seenInstancePaths[path]; ok {
			errs = append(errs, fieldError(indexed(prefix("instances"), instanceIndex, "path"), "duplicate "+path))
		} else {
			seenInstancePaths[path] = struct{}{}
		}
		if strings.TrimSpace(instance.Reference) == "" {
			errs = append(errs, fieldError(indexed(prefix("instances"), instanceIndex, "reference"), "required"))
		}
		if instance.Unit < 0 {
			errs = append(errs, fieldError(indexed(prefix("instances"), instanceIndex, "unit"), "must be non-negative"))
		}
	}
	return errs
}

func validSymbolMirror(mirror SymbolMirror) bool {
	switch mirror {
	case SymbolMirrorNone, SymbolMirrorX, SymbolMirrorY:
		return true
	default:
		return false
	}
}

func validSymbolPassthrough(passthrough SymbolPassthrough) bool {
	switch passthrough {
	case SymbolPassthroughDefault, SymbolPassthroughYes, SymbolPassthroughNo:
		return true
	default:
		return false
	}
}

func validateWire(index int, wire Wire) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !wire.UUID.Valid() {
		errs = append(errs, fieldError(indexed("wires", index, "uuid"), "valid UUID required"))
	}
	if len(wire.Points) != 2 {
		errs = append(errs, fieldError(indexed("wires", index, "points"), "exactly two points required"))
	}
	for i := 1; i < len(wire.Points); i++ {
		if wire.Points[i] == wire.Points[i-1] {
			errs = append(errs, fieldError(indexed("wires", index, "points"), "adjacent points must differ"))
		}
	}
	return errs
}

func validateLineLike(prefix string, uuid kicadfiles.UUID, points []kicadfiles.Point) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix = strings.TrimSuffix(prefix, ".")
	if !uuid.Valid() {
		errs = append(errs, fieldError(prefix+".uuid", "valid UUID required"))
	}
	if len(points) < 2 {
		errs = append(errs, fieldError(prefix+".points", "at least two points required"))
	}
	for i := 1; i < len(points); i++ {
		if points[i] == points[i-1] {
			errs = append(errs, fieldError(prefix+".points", "adjacent points must differ"))
		}
	}
	return errs
}

func validateBusEntry(index int, entry BusEntry) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !entry.UUID.Valid() {
		errs = append(errs, fieldError(indexed("bus_entries", index, "uuid"), "valid UUID required"))
	}
	if entry.Size.X == 0 && entry.Size.Y == 0 {
		errs = append(errs, fieldError(indexed("bus_entries", index, "size"), "non-zero size required"))
	}
	return errs
}

func validateText(index int, text Text) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !text.UUID.Valid() {
		errs = append(errs, fieldError(indexed("texts", index, "uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(text.Value) == "" {
		errs = append(errs, fieldError(indexed("texts", index, "value"), "required"))
	}
	return errs
}

func validateSheet(index int, sheet Sheet) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := func(field string) string { return indexed("sheets", index, field) }
	if !sheet.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(sheet.Name) == "" {
		errs = append(errs, fieldError(prefix("name"), "required"))
	}
	filename := strings.TrimSpace(sheet.Filename)
	if filename == "" {
		errs = append(errs, fieldError(prefix("filename"), "required"))
	} else if !validSheetFilename(filename) {
		errs = append(errs, fieldError(prefix("filename"), "must be a relative KiCad path"))
	}
	if sheet.Size.X <= 0 || sheet.Size.Y <= 0 {
		errs = append(errs, fieldError(prefix("size"), "positive size required"))
	}
	for pinIndex, pin := range sheet.Pins {
		errs = append(errs, validateSheetPin(prefix, pinIndex, sheet, pin)...)
	}
	for instanceIndex, instance := range sheet.Instances {
		errs = append(errs, validateSheetInstance(indexed(prefix("instances"), instanceIndex, ""), instance)...)
	}
	return errs
}

func validateSheetPin(prefix func(string) string, index int, sheet Sheet, pin SheetPin) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	field := func(name string) string { return indexed(prefix("pins"), index, name) }
	if !pin.UUID.Valid() {
		errs = append(errs, fieldError(field("uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(pin.Text) == "" {
		errs = append(errs, fieldError(field("text"), "required"))
	}
	if !validSheetPinKind(pin.Kind) {
		errs = append(errs, fieldError(field("kind"), "invalid"))
	}
	if sheet.Size.X > 0 && sheet.Size.Y > 0 && !sheetPinOnBorder(sheet, pin.Position) {
		errs = append(errs, fieldError(field("position"), "must be on sheet border"))
	}
	return errs
}

func validSheetPinKind(kind SheetPinKind) bool {
	switch kind {
	case SheetPinInput, SheetPinOutput, SheetPinBidirectional, SheetPinTriState, SheetPinPassive:
		return true
	default:
		return false
	}
}

func sheetPinOnBorder(sheet Sheet, point kicadfiles.Point) bool {
	// Points use integer internal units, so exact border comparisons are stable.
	left := sheet.Position.X
	right := sheet.Position.X + sheet.Size.X
	top := sheet.Position.Y
	bottom := sheet.Position.Y + sheet.Size.Y
	onVertical := (point.X == left || point.X == right) && point.Y >= top && point.Y <= bottom
	onHorizontal := (point.Y == top || point.Y == bottom) && point.X >= left && point.X <= right
	return onVertical || onHorizontal
}

func validateSheetInstance(fieldPrefix string, instance SheetInstance) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	prefix := strings.TrimSuffix(fieldPrefix, ".")
	pathField := strings.TrimSuffix(prefix+".path", ".")
	pageField := strings.TrimSuffix(prefix+".page", ".")
	path := strings.TrimSpace(instance.Path)
	if path == "" {
		errs = append(errs, fieldError(pathField, "required"))
	} else if !strings.HasPrefix(path, "/") {
		errs = append(errs, fieldError(pathField, "must start with /"))
	}
	if strings.TrimSpace(instance.Page) == "" {
		errs = append(errs, fieldError(pageField, "required"))
	}
	return errs
}

func validSheetFilename(filename string) bool {
	if strings.Contains(filename, "\\") || strings.ContainsRune(filename, '\x00') {
		return false
	}
	if path.IsAbs(filename) || (len(filename) > 1 && filename[1] == ':') {
		return false
	}
	cleaned := path.Clean(filename)
	return cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func validateLabel(index int, label Label) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !label.UUID.Valid() {
		errs = append(errs, fieldError(indexed("labels", index, "uuid"), "valid UUID required"))
	}
	if strings.TrimSpace(label.Text) == "" {
		errs = append(errs, fieldError(indexed("labels", index, "text"), "required"))
	}
	if label.Kind != LabelLocal && label.Kind != LabelGlobal && label.Kind != LabelHierarchical && label.Kind != LabelDirective {
		errs = append(errs, fieldError(indexed("labels", index, "kind"), "invalid"))
	}
	if label.Kind != LabelLocal && !validLabelShape(labelShape(label)) {
		errs = append(errs, fieldError(indexed("labels", index, "shape"), "invalid"))
	}
	seenFields := map[string]struct{}{}
	for fieldIndex, field := range label.Fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			errs = append(errs, fieldError(indexed("labels", index, "fields")+"["+strconv.Itoa(fieldIndex)+"].name", "required"))
		}
		key := strings.ToLower(name)
		if _, ok := seenFields[key]; ok && key != "" {
			errs = append(errs, fieldError(indexed("labels", index, "fields")+"["+strconv.Itoa(fieldIndex)+"].name", "duplicate "+name))
		}
		seenFields[key] = struct{}{}
	}
	return errs
}

func validateJunction(index int, junction Junction) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	if !junction.UUID.Valid() {
		errs = append(errs, fieldError(indexed("junctions", index, "uuid"), "valid UUID required"))
	}
	if junction.Diameter < 0 {
		errs = append(errs, fieldError(indexed("junctions", index, "diameter"), "must be non-negative"))
	}
	if !validColor(junction.Color) {
		errs = append(errs, fieldError(indexed("junctions", index, "color"), "components must be between 0 and 255"))
	}
	return errs
}

type rawSchematicItemValidationMetadata struct {
	uuids []kicadfiles.UUID
}

func validateRawSchematicItem(index int, raw RawSchematicItem) (kicadfiles.ValidationErrors, rawSchematicItemValidationMetadata) {
	var errs kicadfiles.ValidationErrors
	var metadata rawSchematicItemValidationMetadata
	prefix := func(field string) string { return indexed("raw_items", index, field) }
	if !raw.UUID.Valid() {
		errs = append(errs, fieldError(prefix("uuid"), "valid UUID required"))
	}
	if raw.Order < 0 {
		errs = append(errs, fieldError(prefix("order"), "must be non-negative"))
	}
	body := strings.TrimSpace(string(raw.Body))
	if !sexpr.ValidRaw(body) {
		errs = append(errs, fieldError(prefix("body"), "valid S-expression fragment required"))
		return errs, metadata
	}
	bodyKind := RawSchematicItemKind(rawSchematicItemTopLevelAtom(body))
	if bodyKind == "" {
		errs = append(errs, fieldError(prefix("body"), "top-level item kind required"))
		return errs, metadata
	}
	if raw.Kind != "" && raw.Kind != bodyKind {
		errs = append(errs, fieldError(prefix("kind"), "does not match body kind "+string(bodyKind)))
	}
	metadata.uuids = rawSchematicItemUUIDs(body)
	if raw.UUID.Valid() && !rawSchematicItemUUIDListContains(metadata.uuids, raw.UUID) {
		errs = append(errs, fieldError(prefix("body"), "must contain matching uuid "+string(raw.UUID)))
	}
	effectiveKind := raw.Kind
	if effectiveKind == "" {
		effectiveKind = bodyKind
	}
	if _, ok := rawSchematicItemKind(effectiveKind); !ok {
		errs = append(errs, fieldError(prefix("kind"), "unsupported schematic item kind "+string(effectiveKind)))
	}
	return errs, metadata
}

func validateUniqueSchematicItemUUIDs(schematic SchematicFile, rawMetadata []rawSchematicItemValidationMetadata) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	seen := map[kicadfiles.UUID]string{}
	add := func(field string, uuid kicadfiles.UUID) {
		if !uuid.Valid() {
			return
		}
		if previous, ok := seen[uuid]; ok {
			errs = append(errs, fieldError(field, "duplicate "+string(uuid)+" also used by "+previous))
			return
		}
		seen[uuid] = field
	}
	for i, symbol := range schematic.Symbols {
		add(indexed("symbols", i, "uuid"), symbol.UUID)
	}
	for i, wire := range schematic.Wires {
		add(indexed("wires", i, "uuid"), wire.UUID)
	}
	for i, bus := range schematic.Buses {
		add(indexed("buses", i, "uuid"), bus.UUID)
	}
	for i, polyline := range schematic.Polylines {
		add(indexed("polylines", i, "uuid"), polyline.UUID)
	}
	for i, entry := range schematic.BusEntries {
		add(indexed("bus_entries", i, "uuid"), entry.UUID)
	}
	for i, text := range schematic.Texts {
		add(indexed("texts", i, "uuid"), text.UUID)
	}
	for i, label := range schematic.Labels {
		add(indexed("labels", i, "uuid"), label.UUID)
	}
	for i, junction := range schematic.Junctions {
		add(indexed("junctions", i, "uuid"), junction.UUID)
	}
	for i, noConnect := range schematic.NoConnects {
		add(indexed("no_connects", i, "uuid"), noConnect.UUID)
	}
	for i, sheet := range schematic.Sheets {
		add(indexed("sheets", i, "uuid"), sheet.UUID)
		for pinIndex, pin := range sheet.Pins {
			add(indexed(indexed("sheets", i, "pins"), pinIndex, "uuid"), pin.UUID)
		}
	}
	for i, raw := range schematic.RawItems {
		add(indexed("raw_items", i, "uuid"), raw.UUID)
		ownUUIDSeen := false
		for uuidIndex, uuid := range rawMetadata[i].uuids {
			if uuid == raw.UUID && !ownUUIDSeen {
				ownUUIDSeen = true
				continue
			}
			add(indexed("raw_items", i, "body")+"[uuid "+strconv.Itoa(uuidIndex)+"]", uuid)
		}
	}
	return errs
}

func rawSchematicItemTopLevelAtom(value string) string {
	// Raw fragments preserve KiCad S-expressions, but this helper only infers
	// the first item atom from already-trimmed, comment-free fragments.
	if !strings.HasPrefix(value, "(") {
		return ""
	}
	value = strings.TrimLeft(value[1:], " \t\r\n")
	if value == "" {
		return ""
	}
	end := 0
	for end < len(value) {
		switch value[end] {
		case ' ', '\t', '\r', '\n', '(', ')':
			return value[:end]
		default:
			end++
		}
	}
	return value
}

func rawSchematicItemUUIDListContains(uuids []kicadfiles.UUID, uuid kicadfiles.UUID) bool {
	for _, candidate := range uuids {
		if candidate == uuid {
			return true
		}
	}
	return false
}

func rawSchematicItemUUIDs(body string) []kicadfiles.UUID {
	var uuids []kicadfiles.UUID
	inString := false
	escaped := false
	for i := 0; i < len(body); i++ {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch body[i] {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if body[i] == '"' {
			inString = true
			continue
		}
		if body[i] != '(' || i+5 > len(body) || body[i+1:i+5] != "uuid" {
			continue
		}
		start := i + len("(uuid")
		if start >= len(body) || !isSExprSpace(body[start]) {
			continue
		}
		pos := skipSExprSpace(body, start)
		if pos >= len(body) || body[pos] != '"' {
			continue
		}
		pos++
		valueEnd, ok := rawSchematicStringEnd(body, pos)
		if !ok {
			return uuids
		}
		candidate := kicadfiles.UUID(body[pos:valueEnd])
		afterValue := skipSExprSpace(body, valueEnd+1)
		if afterValue < len(body) && body[afterValue] == ')' && candidate.Valid() {
			uuids = append(uuids, candidate)
		}
		i = afterValue
	}
	return uuids
}

func rawSchematicStringEnd(value string, start int) (int, bool) {
	escaped := false
	for i := start; i < len(value); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch value[i] {
		case '\\':
			escaped = true
		case '"':
			return i, true
		}
	}
	return 0, false
}

func skipSExprSpace(value string, pos int) int {
	for pos < len(value) && isSExprSpace(value[pos]) {
		pos++
	}
	return pos
}

func isSExprSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func validColor(color Color) bool {
	return validColorComponent(color.R) && validColorComponent(color.G) && validColorComponent(color.B) && validColorComponent(color.A)
}

func validColorComponent(value int) bool {
	return value >= 0 && value <= 255
}

func validLabelShape(shape LabelShape) bool {
	switch shape {
	case LabelShapeInput, LabelShapeOutput, LabelShapeBidirectional, LabelShapeTriState, LabelShapePassive:
		return true
	default:
		return false
	}
}

func labelShape(label Label) LabelShape {
	if label.Shape != "" {
		return label.Shape
	}
	return LabelShapeInput
}

func renderLibSymbols(symbols []EmbeddedSymbol) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("lib_symbols")}
	for _, symbol := range symbols {
		if len(symbol.Body) > 0 {
			nodes = append(nodes, symbol.Body)
			continue
		}
		nodes = append(nodes, sexpr.L(sexpr.A("symbol"), sexpr.S(symbol.LibraryID)))
	}
	return sexpr.L(nodes...)
}

func renderSymbol(symbol SchematicSymbol) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("symbol"),
		sexpr.L(sexpr.A("lib_id"), sexpr.S(symbol.LibraryID)),
		renderAt(symbol.Position, symbol.Rotation),
		renderSymbolMirror(symbol.Mirror),
		sexpr.L(sexpr.A("unit"), sexpr.I(int64(defaultPositive(symbol.Unit, 1)))),
		sexpr.L(sexpr.A("body_style"), sexpr.I(int64(defaultPositive(symbol.BodyStyle, 1)))),
		sexpr.L(sexpr.A("exclude_from_sim"), yesNo(symbol.ExcludeFromSim)),
		sexpr.L(sexpr.A("in_bom"), yesNo(defaultBool(symbol.InBOM, true))),
		sexpr.L(sexpr.A("on_board"), yesNo(defaultBool(symbol.OnBoard, true))),
		sexpr.L(sexpr.A("in_pos_files"), yesNo(defaultBool(symbol.InPositionFile, true))),
		sexpr.L(sexpr.A("dnp"), yesNo(symbol.DoNotPopulate)),
		renderSymbolPassthrough(symbol.Passthrough),
		sexpr.OmitIf(!symbol.Locked, sexpr.L(sexpr.A("locked"), sexpr.A("yes"))),
		sexpr.OmitIf(!symbol.FieldsAutoplaced, sexpr.L(sexpr.A("fields_autoplaced"), sexpr.A("yes"))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(symbol.UUID))),
	}
	for _, property := range symbolProperties(symbol) {
		nodes = append(nodes, renderProperty(property))
	}
	for _, pin := range symbol.Pins {
		nodes = append(nodes, renderSymbolPin(pin))
	}
	if len(symbol.Instances) > 0 {
		nodes = append(nodes, renderSymbolInstances(symbol.Instances))
	}
	return sexpr.L(nodes...)
}

func renderSymbolMirror(mirror SymbolMirror) sexpr.Node {
	switch mirror {
	case SymbolMirrorX:
		return sexpr.L(sexpr.A("mirror"), sexpr.A("x"))
	case SymbolMirrorY:
		return sexpr.L(sexpr.A("mirror"), sexpr.A("y"))
	default:
		return sexpr.Omit{}
	}
}

func renderSymbolPassthrough(passthrough SymbolPassthrough) sexpr.Node {
	if passthrough == SymbolPassthroughDefault {
		return sexpr.Omit{}
	}
	return sexpr.L(sexpr.A("passthrough"), sexpr.A(string(passthrough)))
}

func symbolProperties(symbol SchematicSymbol) []Property {
	if len(symbol.Properties) > 0 {
		reference := Property{Name: "Reference", Value: symbol.Reference, Position: symbol.Position, Rotation: symbol.Rotation}
		value := Property{Name: "Value", Value: symbol.Value, Position: symbol.Position, Rotation: symbol.Rotation}
		properties := make([]Property, 0, len(symbol.Properties)+len(symbol.Fields)+2)
		extras := make([]Property, 0, len(symbol.Properties)+len(symbol.Fields))
		seen := map[string]struct{}{}
		for _, property := range symbol.Properties {
			name := strings.TrimSpace(property.Name)
			seen[strings.ToLower(name)] = struct{}{}
			switch {
			case strings.EqualFold(name, "Reference"):
				property.Name = "Reference"
				reference = property
			case strings.EqualFold(name, "Value"):
				property.Name = "Value"
				value = property
			default:
				extras = append(extras, property)
			}
		}
		for _, field := range symbol.Fields {
			name := strings.TrimSpace(field.Name)
			if strings.EqualFold(name, "Reference") || strings.EqualFold(name, "Value") {
				continue
			}
			if _, ok := seen[strings.ToLower(name)]; ok {
				continue
			}
			extras = append(extras, propertyFromField(field))
		}
		properties = append(properties, reference, value)
		properties = append(properties, extras...)
		return properties
	}
	properties := make([]Property, 0, len(symbol.Fields)+2)
	properties = append(properties,
		Property{Name: "Reference", Value: symbol.Reference, Position: symbol.Position, Rotation: symbol.Rotation},
		Property{Name: "Value", Value: symbol.Value, Position: symbol.Position, Rotation: symbol.Rotation},
	)
	for _, field := range symbol.Fields {
		name := strings.TrimSpace(field.Name)
		if name == "Reference" || name == "Value" {
			continue
		}
		properties = append(properties, propertyFromField(field))
	}
	return properties
}

func propertyFromField(field Field) Property {
	return Property{
		Name:     field.Name,
		Value:    field.Value,
		Hidden:   legacyFieldHidden(field),
		Position: field.Position,
		Rotation: field.Rotation,
	}
}

func legacyFieldHidden(field Field) bool {
	return field.Hidden || !field.Visible
}

func renderProperty(property Property) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("property")}
	if property.Private {
		// KiCad saveField writes the private marker before the canonical name.
		nodes = append(nodes, sexpr.A("private"))
	}
	nodes = append(nodes,
		sexpr.S(property.Name),
		sexpr.S(property.Value),
		renderAt(property.Position, property.Rotation),
		// KiCad saveField writes property hide directly, before show_name.
		sexpr.OmitIf(!property.Hidden, sexpr.L(sexpr.A("hide"), sexpr.A("yes"))),
		sexpr.L(sexpr.A("show_name"), yesNo(defaultBool(property.ShowName, false))),
		sexpr.L(sexpr.A("do_not_autoplace"), yesNo(defaultBool(property.DoNotAutoplace, false))),
		renderEffects(false),
	)
	return sexpr.L(nodes...)
}

func renderSymbolPin(pin SymbolPin) sexpr.List {
	number := strings.TrimSpace(pin.Number)
	nodes := []sexpr.Node{
		sexpr.A("pin"),
		sexpr.S(number),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(pin.UUID))),
	}
	if alternate := strings.TrimSpace(pin.Alternate); alternate != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("alternate"), sexpr.S(alternate)))
	}
	return sexpr.L(nodes...)
}

func renderSymbolInstances(instances []SymbolInstance) sexpr.List {
	grouped := map[string][]SymbolInstance{}
	projects := make([]string, 0)
	for _, instance := range instances {
		project := strings.TrimSpace(instance.Project)
		if project == "" {
			project = "project"
		}
		if _, ok := grouped[project]; !ok {
			projects = append(projects, project)
		}
		grouped[project] = append(grouped[project], instance)
	}
	slices.Sort(projects)
	nodes := []sexpr.Node{sexpr.A("instances")}
	for _, project := range projects {
		projectInstances := grouped[project]
		slices.SortFunc(projectInstances, func(a, b SymbolInstance) int {
			return cmp.Compare(a.Path, b.Path)
		})
		projectNodes := []sexpr.Node{sexpr.A("project"), sexpr.S(project)}
		for _, instance := range projectInstances {
			path := strings.TrimSpace(instance.Path)
			reference := strings.TrimSpace(instance.Reference)
			projectNodes = append(projectNodes, sexpr.L(
				sexpr.A("path"),
				sexpr.S(path),
				sexpr.L(sexpr.A("reference"), sexpr.S(reference)),
				sexpr.L(sexpr.A("unit"), sexpr.I(int64(defaultPositive(instance.Unit, 1)))),
			))
		}
		nodes = append(nodes, sexpr.L(projectNodes...))
	}
	return sexpr.L(nodes...)
}

func renderWire(wire Wire) sexpr.List {
	return renderLineLike("wire", wire.Points, wire.UUID)
}

func renderBus(bus Bus) sexpr.List {
	return renderLineLike("bus", bus.Points, bus.UUID)
}

func renderPolyline(polyline Polyline) sexpr.List {
	return renderLineLike("polyline", polyline.Points, polyline.UUID)
}

func renderLineLike(token string, points []kicadfiles.Point, uuid kicadfiles.UUID) sexpr.List {
	return sexpr.L(
		sexpr.A(token),
		renderPoints(points),
		renderStroke(0, "default"),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(uuid))),
	)
}

func renderBusEntry(entry BusEntry) sexpr.List {
	return sexpr.L(
		sexpr.A("bus_entry"),
		sexpr.L(sexpr.A("at"), schematicFixed(entry.Position.X), schematicFixed(entry.Position.Y)),
		sexpr.L(sexpr.A("size"), schematicFixed(entry.Size.X), schematicFixed(entry.Size.Y)),
		renderStroke(0, "default"),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(entry.UUID))),
	)
}

func renderText(text Text) sexpr.List {
	return sexpr.L(
		sexpr.A("text"),
		sexpr.S(text.Value),
		renderAt(text.Position, text.Rotation),
		renderEffects(false),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(text.UUID))),
		sexpr.OmitIf(!text.Locked, sexpr.L(sexpr.A("locked"), sexpr.A("yes"))),
	)
}

func renderLabel(label Label) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A(labelToken(label.Kind)),
		sexpr.S(label.Text),
		sexpr.OmitIf(label.Kind == LabelLocal, sexpr.L(sexpr.A("shape"), sexpr.A(string(labelShape(label))))),
		renderAt(label.Position, label.Rotation),
		renderEffects(false),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(label.UUID))),
		sexpr.OmitIf(!label.Locked, sexpr.L(sexpr.A("locked"), sexpr.A("yes"))),
		sexpr.OmitIf(!label.FieldsAutoplaced, sexpr.L(sexpr.A("fields_autoplaced"), sexpr.A("yes"))),
	}
	for _, field := range label.Fields {
		nodes = append(nodes, renderProperty(propertyFromField(field)))
	}
	return sexpr.L(nodes...)
}

func labelToken(kind LabelKind) string {
	// Keep this as an explicit mapping so future label kinds cannot
	// accidentally render arbitrary unvalidated atoms.
	switch kind {
	case LabelLocal:
		return "label"
	case LabelGlobal:
		return "global_label"
	case LabelHierarchical:
		return "hierarchical_label"
	case LabelDirective:
		return "directive_label"
	default:
		return "label"
	}
}

func renderJunction(junction Junction) sexpr.List {
	diameter := schematicFixed(junction.Diameter)
	color := junction.Color
	if (color.R != 0 || color.G != 0 || color.B != 0) && color.A == 0 {
		color.A = 255
	}
	return sexpr.L(
		sexpr.A("junction"),
		sexpr.L(sexpr.A("at"), schematicFixed(junction.Position.X), schematicFixed(junction.Position.Y)),
		sexpr.L(sexpr.A("diameter"), diameter),
		sexpr.L(sexpr.A("color"), sexpr.I(int64(color.R)), sexpr.I(int64(color.G)), sexpr.I(int64(color.B)), sexpr.I(int64(color.A))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(junction.UUID))),
	)
}

func renderNoConnect(noConnect NoConnect) sexpr.List {
	return sexpr.L(
		sexpr.A("no_connect"),
		sexpr.L(sexpr.A("at"), schematicFixed(noConnect.Position.X), schematicFixed(noConnect.Position.Y)),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(noConnect.UUID))),
	)
}

func renderSheet(sheet Sheet) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("sheet"),
		renderAt(sheet.Position, 0),
		sexpr.L(sexpr.A("size"), schematicFixed(sheet.Size.X), schematicFixed(sheet.Size.Y)),
		sexpr.L(sexpr.A("exclude_from_sim"), yesNo(sheet.ExcludeFromSim)),
		sexpr.L(sexpr.A("in_bom"), yesNo(defaultBool(sheet.InBOM, true))),
		sexpr.L(sexpr.A("on_board"), yesNo(defaultBool(sheet.OnBoard, true))),
		sexpr.L(sexpr.A("dnp"), yesNo(sheet.DoNotPopulate)),
		sexpr.OmitIf(!sheet.Locked, sexpr.L(sexpr.A("locked"), sexpr.A("yes"))),
		sexpr.OmitIf(!sheet.FieldsAutoplaced, sexpr.L(sexpr.A("fields_autoplaced"), sexpr.A("yes"))),
		renderStroke(0.1524, "solid"),
		sexpr.L(sexpr.A("fill"), sexpr.L(sexpr.A("color"), sexpr.I(0), sexpr.I(0), sexpr.I(0), sexpr.X("0.0000"))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(sheet.UUID))),
	}
	for _, property := range sheetProperties(sheet) {
		nodes = append(nodes, renderProperty(property))
	}
	for _, pin := range sheet.Pins {
		nodes = append(nodes, renderSheetPin(pin))
	}
	if len(sheet.Instances) > 0 {
		nodes = append(nodes, renderSheetInstances(sheet.Instances))
	}
	return sexpr.L(nodes...)
}

func sheetProperties(sheet Sheet) []Property {
	// Modern KiCad schematic properties no longer write legacy numeric IDs.
	name := Property{Name: "Sheetname", Value: strings.TrimSpace(sheet.Name), Position: kicadfiles.Point{X: sheet.Position.X, Y: sheet.Position.Y - kicadfiles.MM(2.54)}}
	file := Property{Name: "Sheetfile", Value: strings.TrimSpace(sheet.Filename), Position: kicadfiles.Point{X: sheet.Position.X, Y: sheet.Position.Y + sheet.Size.Y + kicadfiles.MM(2.54)}}
	properties := make([]Property, 0, len(sheet.Properties)+2)
	extras := make([]Property, 0, len(sheet.Properties))
	for _, property := range sheet.Properties {
		switch {
		case strings.EqualFold(strings.TrimSpace(property.Name), "Sheetname"):
			property.Name = "Sheetname"
			name = property
		case strings.EqualFold(strings.TrimSpace(property.Name), "Sheetfile"):
			property.Name = "Sheetfile"
			file = property
		default:
			extras = append(extras, property)
		}
	}
	properties = append(properties, name, file)
	properties = append(properties, extras...)
	return properties
}

func renderSheetPin(pin SheetPin) sexpr.List {
	return sexpr.L(
		sexpr.A("pin"),
		sexpr.S(strings.TrimSpace(pin.Text)),
		sexpr.A(string(pin.Kind)),
		renderAt(pin.Position, pin.Rotation),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(pin.UUID))),
		renderEffects(false),
	)
}

func renderRootSheetInstances(instances []SheetInstance) sexpr.List {
	if len(instances) == 0 {
		instances = []SheetInstance{{Path: "/", Page: "1"}}
	}
	nodes := []sexpr.Node{sexpr.A("sheet_instances")}
	for _, instance := range sortedSheetInstances(instances) {
		nodes = append(nodes, renderSheetInstancePath(instance))
	}
	return sexpr.L(nodes...)
}

func renderSheetInstances(instances []SheetInstance) sexpr.List {
	grouped := map[string][]SheetInstance{}
	projects := make([]string, 0)
	for _, instance := range instances {
		project := strings.TrimSpace(instance.Project)
		if project == "" {
			project = "project"
		}
		if _, ok := grouped[project]; !ok {
			projects = append(projects, project)
		}
		grouped[project] = append(grouped[project], instance)
	}
	slices.Sort(projects)
	nodes := []sexpr.Node{sexpr.A("instances")}
	for _, project := range projects {
		projectNodes := []sexpr.Node{sexpr.A("project"), sexpr.S(project)}
		for _, instance := range sortedSheetInstances(grouped[project]) {
			projectNodes = append(projectNodes, renderSheetInstancePath(instance))
		}
		nodes = append(nodes, sexpr.L(projectNodes...))
	}
	return sexpr.L(nodes...)
}

func sortedSheetInstances(instances []SheetInstance) []SheetInstance {
	out := append([]SheetInstance(nil), instances...)
	slices.SortFunc(out, func(a, b SheetInstance) int {
		return cmp.Compare(strings.TrimSpace(a.Path), strings.TrimSpace(b.Path))
	})
	return out
}

func renderSheetInstancePath(instance SheetInstance) sexpr.List {
	return sexpr.L(
		sexpr.A("path"),
		sexpr.S(strings.TrimSpace(instance.Path)),
		sexpr.L(sexpr.A("page"), sexpr.S(strings.TrimSpace(instance.Page))),
	)
}

func renderStroke(width float64, strokeType string) sexpr.List {
	widthNode := schematicFixed(kicadfiles.MM(width))
	return sexpr.L(
		sexpr.A("stroke"),
		sexpr.L(sexpr.A("width"), widthNode),
		sexpr.L(sexpr.A("type"), sexpr.A(strokeType)),
	)
}

func renderEffects(hidden bool) sexpr.List {
	return sexpr.L(
		sexpr.A("effects"),
		sexpr.L(sexpr.A("font"), sexpr.L(sexpr.A("size"), sexpr.X("1.27"), sexpr.X("1.27"))),
		sexpr.OmitIf(!hidden, sexpr.L(sexpr.A("hide"), sexpr.A("yes"))),
	)
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func yesNo(value bool) sexpr.Atom {
	if value {
		return sexpr.A("yes")
	}
	return sexpr.A("no")
}

func renderPoints(points []kicadfiles.Point) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("pts")}
	for _, point := range points {
		nodes = append(nodes, sexpr.L(sexpr.A("xy"), schematicFixed(point.X), schematicFixed(point.Y)))
	}
	return sexpr.L(nodes...)
}

func renderAt(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	return sexpr.L(sexpr.A("at"), schematicFixed(point.X), schematicFixed(point.Y), sexpr.F(float64(rotation)))
}

func schematicFixed(value kicadfiles.IU) sexpr.Fixed {
	if value == 0 {
		return schematicZero
	}
	return sexpr.X(kicadfiles.ToMMString(value))
}

func indexed(collection string, index int, field string) string {
	return collection + "[" + strconv.Itoa(index) + "]." + field
}
