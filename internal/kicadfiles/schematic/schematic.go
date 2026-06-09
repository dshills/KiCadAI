package schematic

import (
	"io"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type SchematicFile struct {
	Filename   string
	Version    kicadfiles.KiCadFormatVersion
	Generator  string
	UUID       kicadfiles.UUID
	Paper      kicadfiles.Paper
	TitleBlock kicadfiles.TitleBlock
	LibSymbols []EmbeddedSymbol
	Symbols    []SchematicSymbol
	Wires      []Wire
	Labels     []Label
	Junctions  []Junction
	Sheets     []Sheet
	Instances  []SymbolInstance
}

type EmbeddedSymbol struct {
	LibraryID string
	Body      sexpr.List
}

type SchematicSymbol struct {
	UUID      kicadfiles.UUID
	Path      string
	LibraryID string
	Reference string
	Value     string
	Position  kicadfiles.Point
	Rotation  kicadfiles.Angle
	Fields    []Field
}

type Field struct {
	Name     string
	Value    string
	Visible  bool
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
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(schematic.UUID))),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(schematic.Paper.Name))),
	}
	if title := renderTitleBlock(schematic.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	if len(schematic.LibSymbols) > 0 {
		nodes = append(nodes, renderLibSymbols(schematic.LibSymbols))
	}
	for _, symbol := range schematic.Symbols {
		nodes = append(nodes, renderSymbol(symbol))
	}
	for _, wire := range schematic.Wires {
		nodes = append(nodes, renderWire(wire))
	}
	for _, label := range schematic.Labels {
		nodes = append(nodes, renderLabel(label))
	}
	for _, junction := range schematic.Junctions {
		nodes = append(nodes, renderJunction(junction))
	}
	if len(schematic.Instances) > 0 {
		nodes = append(nodes, renderInstances(schematic.Instances))
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
	if len(wire.Points) < 2 {
		errs = append(errs, fieldError(indexed("wires", index, "points"), "at least two points required"))
	}
	for i := 1; i < len(wire.Points); i++ {
		if wire.Points[i] == wire.Points[i-1] {
			errs = append(errs, fieldError(indexed("wires", index, "points"), "adjacent points must differ"))
		}
	}
	return errs
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
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(symbol.UUID))),
		sexpr.L(sexpr.A("property"), sexpr.S("Reference"), sexpr.S(symbol.Reference), renderAt(symbol.Position, symbol.Rotation)),
		sexpr.L(sexpr.A("property"), sexpr.S("Value"), sexpr.S(symbol.Value), renderAt(symbol.Position, symbol.Rotation)),
	}
	if symbol.Path != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("path"), sexpr.S(symbol.Path)))
	}
	for _, field := range symbol.Fields {
		nodes = append(nodes, sexpr.L(sexpr.A("property"), sexpr.S(field.Name), sexpr.S(field.Value), renderAt(field.Position, field.Rotation)))
	}
	return sexpr.L(nodes...)
}

func renderWire(wire Wire) sexpr.List {
	return sexpr.L(
		sexpr.A("wire"),
		renderPoints(wire.Points),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(wire.UUID))),
	)
}

func renderLabel(label Label) sexpr.List {
	return sexpr.L(
		sexpr.A(string(label.Kind)),
		sexpr.S(label.Text),
		renderAt(label.Position, label.Rotation),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(label.UUID))),
	)
}

func renderJunction(junction Junction) sexpr.List {
	return sexpr.L(
		sexpr.A("junction"),
		sexpr.L(sexpr.A("at"), sexpr.X(kicadfiles.ToMMString(junction.Position.X)), sexpr.X(kicadfiles.ToMMString(junction.Position.Y))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(junction.UUID))),
	)
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
