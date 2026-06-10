package schematic

import (
	"io"
	"path"
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
	Labels           []Label
	Junctions        []Junction
	Sheets           []Sheet
	Instances        []SymbolInstance
}

type EmbeddedSymbol struct {
	LibraryID string
	Body      sexpr.List
}

type SchematicSymbol struct {
	UUID           kicadfiles.UUID
	Path           string
	LibraryID      string
	Reference      string
	Value          string
	Position       kicadfiles.Point
	Rotation       kicadfiles.Angle
	Unit           int
	BodyStyle      int
	ExcludeFromSim bool
	InBOM          *bool
	OnBoard        *bool
	InPositionFile *bool
	DoNotPopulate  bool
	Fields         []Field
}

type Field struct {
	Name     string
	Value    string
	Visible  bool
	Hidden   bool
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

type Wire struct {
	UUID   kicadfiles.UUID
	Points []kicadfiles.Point
}

type Label struct {
	UUID     kicadfiles.UUID
	Text     string
	Kind     LabelKind
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
}

type LabelKind string

const (
	LabelLocal        LabelKind = "label"
	LabelGlobal       LabelKind = "global_label"
	LabelHierarchical LabelKind = "hierarchical_label"
)

type Junction struct {
	UUID     kicadfiles.UUID
	Position kicadfiles.Point
}

type Sheet struct {
	UUID     kicadfiles.UUID
	Name     string
	Filename string
	Position kicadfiles.Point
	Size     kicadfiles.Point
}

type SymbolInstance struct {
	Path      string
	Reference string
	Unit      int
	Value     string
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
	if schematic.Version == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if _, err := strconv.ParseInt(string(schematic.Version), 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	}
	if strings.TrimSpace(schematic.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
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
	for i, label := range schematic.Labels {
		errs = append(errs, validateLabel(i, label)...)
	}
	for i, junction := range schematic.Junctions {
		if !junction.UUID.Valid() {
			errs = append(errs, fieldError(indexed("junctions", i, "uuid"), "valid UUID required"))
		}
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
	nodes := []sexpr.Node{
		sexpr.A("kicad_sch"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(schematic.Generator))),
	}
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
	if len(schematic.LibSymbols) > 0 {
		nodes = append(nodes, renderLibSymbols(schematic.LibSymbols))
	}
	for _, label := range schematic.Labels {
		nodes = append(nodes, renderLabel(label))
	}
	for _, wire := range schematic.Wires {
		nodes = append(nodes, renderWire(wire))
	}
	for _, junction := range schematic.Junctions {
		nodes = append(nodes, renderJunction(junction))
	}
	for _, symbol := range schematic.Symbols {
		nodes = append(nodes, renderSymbol(symbol))
	}
	for _, sheet := range schematic.Sheets {
		nodes = append(nodes, renderSheet(sheet))
	}
	return sexpr.L(nodes...), nil
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
	seenFields := map[string]struct{}{}
	for fieldIndex, field := range symbol.Fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "required"))
			continue
		}
		if _, ok := seenFields[name]; ok {
			errs = append(errs, fieldError(indexed(prefix("fields"), fieldIndex, "name"), "duplicate "+name))
		}
		seenFields[name] = struct{}{}
	}
	return errs
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
	if label.Kind != LabelLocal && label.Kind != LabelGlobal && label.Kind != LabelHierarchical {
		errs = append(errs, fieldError(indexed("labels", index, "kind"), "invalid"))
	}
	return errs
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
		sexpr.L(sexpr.A("unit"), sexpr.I(int64(defaultPositive(symbol.Unit, 1)))),
		sexpr.L(sexpr.A("body_style"), sexpr.I(int64(defaultPositive(symbol.BodyStyle, 1)))),
		sexpr.L(sexpr.A("exclude_from_sim"), yesNo(symbol.ExcludeFromSim)),
		sexpr.L(sexpr.A("in_bom"), yesNo(defaultBool(symbol.InBOM, true))),
		sexpr.L(sexpr.A("on_board"), yesNo(defaultBool(symbol.OnBoard, true))),
		sexpr.L(sexpr.A("in_pos_files"), yesNo(defaultBool(symbol.InPositionFile, true))),
		sexpr.L(sexpr.A("dnp"), yesNo(symbol.DoNotPopulate)),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(symbol.UUID))),
		renderSymbolProperty("Reference", symbol.Reference, symbol.Position, symbol.Rotation, false),
		renderSymbolProperty("Value", symbol.Value, symbol.Position, symbol.Rotation, false),
	}
	for _, field := range symbol.Fields {
		nodes = append(nodes, renderSymbolProperty(field.Name, field.Value, field.Position, field.Rotation, field.Hidden || !field.Visible))
	}
	return sexpr.L(nodes...)
}

func renderWire(wire Wire) sexpr.List {
	return sexpr.L(
		sexpr.A("wire"),
		renderPoints(wire.Points),
		renderStroke(0, "default"),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(wire.UUID))),
	)
}

func renderLabel(label Label) sexpr.List {
	return sexpr.L(
		sexpr.A(string(label.Kind)),
		sexpr.S(label.Text),
		renderAt(label.Position, label.Rotation),
		renderEffects(false),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(label.UUID))),
	)
}

func renderJunction(junction Junction) sexpr.List {
	return sexpr.L(
		sexpr.A("junction"),
		sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(junction.Position.X)), sexpr.X(kicadfiles.ToMMString(junction.Position.Y))),
		sexpr.L(sexpr.A("diameter"), sexpr.I(0)),
		sexpr.L(sexpr.A("color"), sexpr.I(0), sexpr.I(0), sexpr.I(0), sexpr.I(0)),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(junction.UUID))),
	)
}

func renderSheet(sheet Sheet) sexpr.List {
	return sexpr.L(
		sexpr.A("sheet"),
		renderAt(sheet.Position, 0),
		sexpr.L(sexpr.A("size"), sexpr.X(kicadfiles.ToMMString(sheet.Size.X)), sexpr.X(kicadfiles.ToMMString(sheet.Size.Y))),
		renderStroke(0.1524, "solid"),
		sexpr.L(sexpr.A("fill"), sexpr.L(sexpr.A("color"), sexpr.I(0), sexpr.I(0), sexpr.I(0), sexpr.X("0.0000"))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(sheet.UUID))),
		renderSheetProperty(0, "Sheetname", strings.TrimSpace(sheet.Name), kicadfiles.Point{X: sheet.Position.X, Y: sheet.Position.Y - kicadfiles.MM(2.54)}),
		renderSheetProperty(1, "Sheetfile", strings.TrimSpace(sheet.Filename), kicadfiles.Point{X: sheet.Position.X, Y: sheet.Position.Y + sheet.Size.Y + kicadfiles.MM(2.54)}),
	)
}

func renderSheetProperty(id int64, name, value string, at kicadfiles.Point) sexpr.List {
	return sexpr.L(
		sexpr.A("property"),
		sexpr.S(name),
		sexpr.S(value),
		sexpr.L(sexpr.A("id"), sexpr.I(id)),
		renderAt(at, 0),
		renderEffects(false),
	)
}

func renderSymbolProperty(name, value string, at kicadfiles.Point, rotation kicadfiles.Angle, hidden bool) sexpr.List {
	return sexpr.L(
		sexpr.A("property"),
		sexpr.S(name),
		sexpr.S(value),
		renderAt(at, rotation),
		sexpr.L(sexpr.A("show_name"), sexpr.A("no")),
		sexpr.L(sexpr.A("do_not_autoplace"), sexpr.A("no")),
		sexpr.OmitIf(!hidden, sexpr.L(sexpr.A("hide"), sexpr.A("yes"))),
		renderEffects(false),
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

func renderInstances(instances []SymbolInstance) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol_instances")}
	for _, instance := range instances {
		nodes = append(nodes, sexpr.L(
			sexpr.A("path"),
			sexpr.S(instance.Path),
			sexpr.L(sexpr.A("reference"), sexpr.S(instance.Reference)),
			sexpr.L(sexpr.A("unit"), sexpr.I(int64(instance.Unit))),
			sexpr.L(sexpr.A("value"), sexpr.S(instance.Value)),
		))
	}
	return sexpr.L(nodes...)
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
