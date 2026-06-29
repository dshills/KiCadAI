package schematic

import (
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type TemplatePin struct {
	Number string
	Offset kicadfiles.Point
}

type embeddedTemplate struct {
	bodyName string
	pinType  string
	pinX     float64
	power    bool
	pins     []TemplatePin
}

var embeddedSymbolTemplates = map[string]embeddedTemplate{
	"device:c":   {bodyName: "C", pinType: "passive", pins: twoPinTemplatePins()},
	"device:d":   {bodyName: "D", pinType: "passive", pins: twoPinTemplatePins()},
	"device:led": {bodyName: "LED", pinType: "passive", pins: twoPinTemplatePins()},
	"device:r":   {bodyName: "R", pinType: "passive", pins: twoPinTemplatePins()},
	"power:gnd":  {bodyName: "GND", pinType: "power_in", pinX: -5.08, power: true, pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: 0}}}},
	"power:vcc":  {bodyName: "VCC", pinType: "power_in", pinX: 5.08, power: true, pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(5.08), Y: 0}}}},
}

// EmbeddedSymbolTemplate returns a KiCad-native embedded lib symbol body for
// seed symbols whose library IDs are known to KiCadAI.
func EmbeddedSymbolTemplate(libraryID string) (EmbeddedSymbol, bool) {
	libraryID = strings.TrimSpace(libraryID)
	template, ok := embeddedSymbolTemplates[strings.ToLower(libraryID)]
	if !ok {
		return EmbeddedSymbol{}, false
	}
	return embeddedSymbolFromTemplate(libraryID, template), true
}

// EmbeddedSymbolPinOffsets returns template pin anchors relative to the symbol
// origin for supported seed symbols.
func EmbeddedSymbolPinOffsets(libraryID string) ([]TemplatePin, bool) {
	template, ok := embeddedSymbolTemplates[strings.ToLower(strings.TrimSpace(libraryID))]
	if !ok {
		return nil, false
	}
	return cloneTemplatePins(template.pins), true
}

func twoPinTemplatePins() []TemplatePin {
	return []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: 0}},
		{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(5.08), Y: 0}},
	}
}

func cloneTemplatePins(source []TemplatePin) []TemplatePin {
	if source == nil {
		return nil
	}
	clone := make([]TemplatePin, len(source))
	copy(clone, source)
	return clone
}

func embeddedSymbolFromTemplate(libraryID string, template embeddedTemplate) EmbeddedSymbol {
	var body sexpr.List
	if template.power {
		body = powerSymbolBody(libraryID, template.bodyName, template.pinType, template.pinX)
	} else {
		body = twoPinSymbolBody(libraryID, template.bodyName, template.pinType)
	}
	return EmbeddedSymbol{LibraryID: libraryID, Body: body}
}

// EnsureEmbeddedSymbol adds a known embedded lib symbol template to file once.
// Unsupported library IDs are left untouched so external library references can
// continue resolving through KiCad library tables.
func EnsureEmbeddedSymbol(file *SchematicFile, libraryID string) bool {
	if file == nil {
		return false
	}
	libraryID = strings.TrimSpace(libraryID)
	normalizedID := strings.ToLower(libraryID)
	templateDefinition, ok := embeddedSymbolTemplates[normalizedID]
	if !ok {
		return false
	}
	for i := range file.LibSymbols {
		existingID := strings.TrimSpace(file.LibSymbols[i].LibraryID)
		if !strings.EqualFold(existingID, libraryID) {
			continue
		}
		if len(file.LibSymbols[i].Body) == 0 {
			file.LibSymbols[i] = embeddedSymbolFromTemplate(libraryID, templateDefinition)
		}
		return true
	}
	file.LibSymbols = append(file.LibSymbols, embeddedSymbolFromTemplate(libraryID, templateDefinition))
	return true
}

// EnsureEmbeddedTwoPinSymbol force-adds a two-pin embedded body for callers
// that already know the custom library ID should use a seed symbol shape.
func EnsureEmbeddedTwoPinSymbol(file *SchematicFile, libraryID, bodyName, pinType string) bool {
	return ensureEmbeddedSymbolBody(file, libraryID, twoPinSymbolBody(libraryID, bodyName, pinType))
}

// EnsureEmbeddedPowerSymbol force-adds a one-pin power embedded body for callers
// that already know the custom library ID should use a seed power symbol shape.
func EnsureEmbeddedPowerSymbol(file *SchematicFile, libraryID, bodyName, pinType string, pinX float64) bool {
	return ensureEmbeddedSymbolBody(file, libraryID, powerSymbolBody(libraryID, bodyName, pinType, pinX))
}

func ensureEmbeddedSymbolBody(file *SchematicFile, libraryID string, body sexpr.List) bool {
	if file == nil {
		return false
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" || len(body) == 0 {
		return false
	}
	normalizedID := strings.ToLower(libraryID)
	for i := range file.LibSymbols {
		if strings.ToLower(strings.TrimSpace(file.LibSymbols[i].LibraryID)) == normalizedID {
			if len(file.LibSymbols[i].Body) == 0 {
				file.LibSymbols[i] = EmbeddedSymbol{LibraryID: libraryID, Body: body}
			}
			return true
		}
	}
	file.LibSymbols = append(file.LibSymbols, EmbeddedSymbol{LibraryID: libraryID, Body: body})
	return true
}

func twoPinSymbolBody(libraryID, bodyName, pinType string) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(
			sexpr.A("symbol"),
			sexpr.S(bodyName+"_1_1"),
			embeddedPin(pinType, -5.08, 0, "~", "1"),
			embeddedPin(pinType, 5.08, 180, "~", "2"),
		),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func powerSymbolBody(libraryID, bodyName, pinType string, pinX float64) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(
			sexpr.A("symbol"),
			sexpr.S(bodyName+"_1_1"),
			// Match KiCad's save output for these generated power symbols.
			embeddedPin(pinType, pinX, 180, libraryID, "1"),
		),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func embeddedSymbolDefaults() []sexpr.Node {
	return []sexpr.Node{
		sexpr.L(sexpr.A("exclude_from_sim"), sexpr.A("no")),
		sexpr.L(sexpr.A("in_bom"), sexpr.A("yes")),
		sexpr.L(sexpr.A("on_board"), sexpr.A("yes")),
		sexpr.L(sexpr.A("in_pos_files"), sexpr.A("yes")),
		sexpr.L(sexpr.A("duplicate_pin_numbers_are_jumpers"), sexpr.A("no")),
		embeddedLibraryProperty("Reference", "", false),
		embeddedLibraryProperty("Value", "", false),
		embeddedLibraryProperty("Footprint", "", true),
		embeddedLibraryProperty("Datasheet", "", true),
		embeddedLibraryProperty("Description", "", true),
	}
}

func embeddedLibraryProperty(name, value string, hidden bool) sexpr.List {
	return sexpr.L(
		sexpr.A("property"),
		sexpr.S(name),
		sexpr.S(value),
		sexpr.L(sexpr.A("at"), sexpr.I(0), sexpr.I(0), sexpr.I(0)),
		sexpr.L(sexpr.A("show_name"), sexpr.A("no")),
		sexpr.L(sexpr.A("do_not_autoplace"), sexpr.A("no")),
		sexpr.OmitIf(!hidden, sexpr.L(sexpr.A("hide"), sexpr.A("yes"))),
		renderEffects(false),
	)
}

func embeddedPin(pinType string, pinX float64, rotation int64, name string, number string) sexpr.List {
	return sexpr.L(
		sexpr.A("pin"),
		sexpr.A(pinType),
		sexpr.A("line"),
		sexpr.L(sexpr.A("at"), sexpr.F(pinX), sexpr.I(0), sexpr.I(rotation)),
		sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
		sexpr.L(sexpr.A("name"), sexpr.S(name), renderEffects(false)),
		sexpr.L(sexpr.A("number"), sexpr.S(number), renderEffects(false)),
	)
}
