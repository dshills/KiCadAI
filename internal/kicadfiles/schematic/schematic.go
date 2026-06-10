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
	Instances        []SymbolInstance
	SheetInstances   []SheetInstance
}

type EmbeddedSymbol struct {
	LibraryID string
	Body      sexpr.List
}

type SchematicSymbol struct {
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
	Fields           []Field
	Pins             []SymbolPin
	Instances        []SymbolInstance
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
)

const schematicHeaderNodeCapacity = 8

type renderItem struct {
	kind schematicItemKind
	uuid kicadfiles.UUID
	node sexpr.List
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
	for i, instance := range schematic.SheetInstances {
		errs = append(errs, validateSheetInstance(indexed("sheet_instances", i, ""), instance)...)
	}
	return errs.Err()
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
	items := renderItems(schematic)
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

func renderItems(schematic SchematicFile) []renderItem {
	items := make([]renderItem, 0, len(schematic.Junctions)+len(schematic.NoConnects)+len(schematic.BusEntries)+len(schematic.Wires)+len(schematic.Buses)+len(schematic.Polylines)+len(schematic.Texts)+len(schematic.Labels)+len(schematic.Symbols)+len(schematic.Sheets))
	for _, junction := range schematic.Junctions {
		items = append(items, renderItem{kind: schematicItemJunction, uuid: junction.UUID, node: renderJunction(junction)})
	}
	for _, noConnect := range schematic.NoConnects {
		items = append(items, renderItem{kind: schematicItemNoConnect, uuid: noConnect.UUID, node: renderNoConnect(noConnect)})
	}
	for _, entry := range schematic.BusEntries {
		items = append(items, renderItem{kind: schematicItemWireToBusEntry, uuid: entry.UUID, node: renderBusEntry(entry)})
	}
	for _, wire := range schematic.Wires {
		items = append(items, renderItem{kind: schematicItemLine, uuid: wire.UUID, node: renderWire(wire)})
	}
	// KiCad saves wires, buses, and graphical polylines as SCH_LINE_T items.
	for _, bus := range schematic.Buses {
		items = append(items, renderItem{kind: schematicItemLine, uuid: bus.UUID, node: renderBus(bus)})
	}
	for _, polyline := range schematic.Polylines {
		items = append(items, renderItem{kind: schematicItemLine, uuid: polyline.UUID, node: renderPolyline(polyline)})
	}
	for _, text := range schematic.Texts {
		items = append(items, renderItem{kind: schematicItemText, uuid: text.UUID, node: renderText(text)})
	}
	for _, label := range schematic.Labels {
		items = append(items, renderItem{kind: labelItemKind(label.Kind), uuid: label.UUID, node: renderLabel(label)})
	}
	for _, symbol := range schematic.Symbols {
		items = append(items, renderItem{kind: schematicItemSymbol, uuid: symbol.UUID, node: renderSymbol(symbol)})
	}
	for _, sheet := range schematic.Sheets {
		items = append(items, renderItem{kind: schematicItemSheet, uuid: sheet.UUID, node: renderSheet(sheet)})
	}
	// Keep the sort stable so invalid duplicate UUID input still renders
	// reproducibly before validation grows strict global UUID checks.
	slices.SortStableFunc(items, func(a, b renderItem) int {
		if a.kind != b.kind {
			return cmp.Compare(a.kind, b.kind)
		}
		return cmp.Compare(a.uuid, b.uuid)
	})
	return items
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
		sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(entry.Position.X)), sexpr.X(kicadfiles.ToMMString(entry.Position.Y))),
		sexpr.L(sexpr.A("size"), sexpr.X(kicadfiles.ToMMString(entry.Size.X)), sexpr.X(kicadfiles.ToMMString(entry.Size.Y))),
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
	diameter := sexpr.Node(sexpr.I(0))
	if junction.Diameter != 0 {
		diameter = sexpr.X(kicadfiles.ToMMString(junction.Diameter))
	}
	color := junction.Color
	if (color.R != 0 || color.G != 0 || color.B != 0) && color.A == 0 {
		color.A = 255
	}
	return sexpr.L(
		sexpr.A("junction"),
		sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(junction.Position.X)), sexpr.X(kicadfiles.ToMMString(junction.Position.Y))),
		sexpr.L(sexpr.A("diameter"), diameter),
		sexpr.L(sexpr.A("color"), sexpr.I(int64(color.R)), sexpr.I(int64(color.G)), sexpr.I(int64(color.B)), sexpr.I(int64(color.A))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(junction.UUID))),
	)
}

func renderNoConnect(noConnect NoConnect) sexpr.List {
	return sexpr.L(
		sexpr.A("no_connect"),
		sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(noConnect.Position.X)), sexpr.X(kicadfiles.ToMMString(noConnect.Position.Y))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(noConnect.UUID))),
	)
}

func renderSheet(sheet Sheet) sexpr.List {
	nodes := []sexpr.Node{
		sexpr.A("sheet"),
		renderAt(sheet.Position, 0),
		sexpr.L(sexpr.A("size"), sexpr.X(kicadfiles.ToMMString(sheet.Size.X)), sexpr.X(kicadfiles.ToMMString(sheet.Size.Y))),
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
	var widthNode sexpr.Node = sexpr.I(0)
	if width != 0 {
		widthNode = sexpr.X(kicadfiles.ToMMString(kicadfiles.MM(width)))
	}
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
		nodes = append(nodes, sexpr.L(sexpr.A("xy"), sexpr.X(kicadfiles.ToMMString(point.X)), sexpr.X(kicadfiles.ToMMString(point.Y))))
	}
	return sexpr.L(nodes...)
}

func renderAt(point kicadfiles.Point, rotation kicadfiles.Angle) sexpr.List {
	return sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(point.X)), sexpr.X(kicadfiles.ToMMString(point.Y)), sexpr.F(float64(rotation)))
}

func indexed(collection string, index int, field string) string {
	return collection + "[" + strconv.Itoa(index) + "]." + field
}
