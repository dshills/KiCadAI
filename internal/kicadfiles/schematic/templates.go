package schematic

import (
	"strconv"
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
	rawBody  string
}

var embeddedSymbolTemplates = map[string]embeddedTemplate{
	"amplifier_operational:lmv321": {bodyName: "LMV321", pinType: "passive", pins: []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: 0}},
		{Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(2.54)}},
		{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)}},
		{Number: "4", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)}},
		{Number: "5", Offset: kicadfiles.Point{Y: kicadfiles.MM(-2.54)}},
	}},
	"connector_generic:conn_01x02": {bodyName: "Conn_01x02", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08)}}, {Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(-2.54)}}}, rawBody: rawConnectorGenericConn01x02Symbol},
	"connector_generic:conn_01x04": {bodyName: "Conn_01x04", pinType: "passive", pins: connectorTemplatePins(4)},
	"device:c":                     {bodyName: "C", pinType: "passive", pins: twoPinTemplatePins()},
	"device:c_polarized":           {bodyName: "C_Polarized", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(-5.08)}}, {Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}}},
	"device:d":                     {bodyName: "D", pinType: "passive", pins: twoPinTemplatePins()},
	"device:led":                   {bodyName: "LED", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-3.81)}}, {Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(3.81)}}}, rawBody: rawDeviceLEDSymbol},
	"device:q_npn_bec": {bodyName: "Q_NPN_BEC", pinType: "passive", pins: []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
		{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(2.54)}},
		{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(-2.54)}},
	}},
	"device:q_pnp_bec": {bodyName: "Q_PNP_BEC", pinType: "passive", pins: []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
		{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(-2.54)}},
		{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(2.54)}},
	}},
	"device:r":            {bodyName: "R", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(3.81)}}, {Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(-3.81)}}}, rawBody: rawDeviceRSymbol},
	"connector:testpoint": {bodyName: "TestPoint", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{}}}},
	"power:+3.3v":         powerTemplate("+3.3V", 5.08),
	"power:+3v3":          powerTemplate("+3V3", 5.08),
	"power:+5v":           powerTemplate("+5V", 5.08),
	"power:+12v":          powerTemplate("+12V", 5.08),
	"power:-12v":          powerTemplate("-12V", -5.08),
	"power:gnd":           powerTemplate("GND", -5.08),
	"power:pwr_flag": {
		bodyName: "PWR_FLAG",
		pinType:  "power_out",
		pinX:     0,
		power:    true,
		pins:     []TemplatePin{{Number: "1", Offset: kicadfiles.Point{}}},
	},
	"power:vcc": powerTemplate("VCC", 5.08),
	"power:vdd": powerTemplate("VDD", 5.08),
	"power:vee": powerTemplate("VEE", -5.08),
	"power:vss": powerTemplate("VSS", -5.08),
	"sensor:generic_i2c": {
		bodyName: "Generic_I2C",
		pinType:  "bidirectional",
		pins: []TemplatePin{
			{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-3.81)}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(3.81)}},
			{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)}},
			{Number: "4", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)}},
			{Number: "5", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: 0}},
		},
	},
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

func connectorTemplatePins(count int) []TemplatePin {
	pins := make([]TemplatePin, count)
	for index := 0; index < count; index++ {
		pins[index] = TemplatePin{
			Number: strconv.Itoa(index + 1),
			Offset: kicadfiles.Point{
				X: kicadfiles.MM(-5.08),
				Y: -kicadfiles.MM(2.54 * float64(index)),
			},
		}
	}
	return pins
}

func powerTemplate(bodyName string, pinX float64) embeddedTemplate {
	return embeddedTemplate{
		bodyName: bodyName,
		pinType:  "power_in",
		pinX:     pinX,
		power:    true,
		pins: []TemplatePin{{
			Number: "1",
			Offset: kicadfiles.Point{X: kicadfiles.MM(pinX), Y: 0},
		}},
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
	if strings.TrimSpace(template.rawBody) != "" {
		body = rawEmbeddedSymbolBody(template.rawBody)
	} else if template.power {
		body = powerSymbolBody(libraryID, template.bodyName, template.pinType, template.pinX)
	} else {
		body = symbolBodyFromTemplatePins(libraryID, template.bodyName, template.pinType, template.pins)
	}
	return EmbeddedSymbol{LibraryID: libraryID, Body: body}
}

func rawEmbeddedSymbolBody(raw string) sexpr.List {
	node, err := sexpr.Parse([]byte(raw))
	if err != nil {
		return nil
	}
	list, ok := node.Node().(sexpr.List)
	if !ok {
		return nil
	}
	return list
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
	return symbolBodyFromTemplatePins(libraryID, bodyName, pinType, twoPinTemplatePins())
}

func symbolBodyFromTemplatePins(libraryID, bodyName, pinType string, pins []TemplatePin) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	pinNodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(bodyName + "_1_1")}
	if body := embeddedTemplateBody(pins); len(body) > 0 {
		pinNodes = append(pinNodes, body)
	}
	for _, pin := range pins {
		pinNodes = append(pinNodes, embeddedTemplatePin(pinType, pin))
	}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(pinNodes...),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func embeddedTemplateBody(pins []TemplatePin) sexpr.List {
	if len(pins) == 0 {
		return nil
	}
	minX, maxX := pins[0].Offset.X, pins[0].Offset.X
	minY, maxY := pins[0].Offset.Y, pins[0].Offset.Y
	for _, pin := range pins[1:] {
		if pin.Offset.X < minX {
			minX = pin.Offset.X
		}
		if pin.Offset.X > maxX {
			maxX = pin.Offset.X
		}
		if pin.Offset.Y < minY {
			minY = pin.Offset.Y
		}
		if pin.Offset.Y > maxY {
			maxY = pin.Offset.Y
		}
	}
	padding := kicadfiles.MM(1.27)
	bodyMinX, bodyMaxX := embeddedTemplateBodyAxis(minX, maxX, padding)
	bodyMinY, bodyMaxY := embeddedTemplateBodyAxis(minY, maxY, padding)
	return sexpr.L(
		sexpr.A("rectangle"),
		sexpr.L(sexpr.A("start"), sexpr.F(templatePinMM(bodyMinX)), sexpr.F(templatePinMM(bodyMinY))),
		sexpr.L(sexpr.A("end"), sexpr.F(templatePinMM(bodyMaxX)), sexpr.F(templatePinMM(bodyMaxY))),
		sexpr.L(
			sexpr.A("stroke"),
			sexpr.L(sexpr.A("width"), sexpr.X("0.254")),
			sexpr.L(sexpr.A("type"), sexpr.A("default")),
		),
		sexpr.L(sexpr.A("fill"), sexpr.L(sexpr.A("type"), sexpr.A("none"))),
	)
}

func embeddedTemplateBodyAxis(minimum, maximum, padding kicadfiles.IU) (kicadfiles.IU, kicadfiles.IU) {
	if minimum != maximum {
		return minimum, maximum
	}
	switch {
	case minimum < 0:
		return minimum, minimum + 2*padding
	case minimum > 0:
		return minimum - 2*padding, minimum
	default:
		return minimum - padding, minimum + padding
	}
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

func embeddedTemplatePin(pinType string, pin TemplatePin) sexpr.List {
	return sexpr.L(
		sexpr.A("pin"),
		sexpr.A(pinType),
		sexpr.A("line"),
		sexpr.L(sexpr.A("at"), sexpr.F(templatePinMM(pin.Offset.X)), sexpr.F(templatePinMM(pin.Offset.Y)), sexpr.I(templatePinRotation(pin.Offset))),
		sexpr.L(sexpr.A("length"), sexpr.X("2.54")),
		sexpr.L(sexpr.A("name"), sexpr.S("~"), renderEffects(false)),
		sexpr.L(sexpr.A("number"), sexpr.S(pin.Number), renderEffects(false)),
	)
}

func templatePinRotation(offset kicadfiles.Point) int64 {
	switch {
	case offset.X < 0:
		return 0
	case offset.X > 0:
		return 180
	case offset.Y > 0:
		return 270
	case offset.Y < 0:
		return 90
	default:
		return 0
	}
}

func templatePinMM(value kicadfiles.IU) float64 {
	return float64(value) / 1_000_000
}

const rawDeviceRSymbol = `(symbol "Device:R"
  (pin_numbers (hide yes))
  (pin_names (offset 0))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "R" (at 2.032 0 90) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "R" (at 0 0 90) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at -1.778 0 90) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Resistor" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "R res resistor" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "R_*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "R_0_1"
    (rectangle (start -1.016 -2.54) (end 1.016 2.54) (stroke (width 0.254) (type default)) (fill (type none)))
  )
  (symbol "R_1_1"
    (pin passive line (at 0 3.81 270) (length 1.27) (name "" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at 0 -3.81 90) (length 1.27) (name "" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
  )
  (embedded_fonts no)
)`

const rawDeviceLEDSymbol = `(symbol "Device:LED"
  (pin_numbers (hide yes))
  (pin_names (offset 1.016) (hide yes))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "D" (at 0 2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "LED" (at 0 -2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Light emitting diode" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Sim.Pins" "1=K 2=A" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "LED diode" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "LED* LED_SMD:* LED_THT:*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "LED_0_1"
    (polyline (pts (xy -3.048 -0.762) (xy -4.572 -2.286) (xy -3.81 -2.286) (xy -4.572 -2.286) (xy -4.572 -1.524)) (stroke (width 0) (type default)) (fill (type none)))
    (polyline (pts (xy -1.778 -0.762) (xy -3.302 -2.286) (xy -2.54 -2.286) (xy -3.302 -2.286) (xy -3.302 -1.524)) (stroke (width 0) (type default)) (fill (type none)))
    (polyline (pts (xy -1.27 0) (xy 1.27 0)) (stroke (width 0) (type default)) (fill (type none)))
    (polyline (pts (xy -1.27 -1.27) (xy -1.27 1.27)) (stroke (width 0.254) (type default)) (fill (type none)))
    (polyline (pts (xy 1.27 -1.27) (xy 1.27 1.27) (xy -1.27 0) (xy 1.27 -1.27)) (stroke (width 0.254) (type default)) (fill (type none)))
  )
  (symbol "LED_1_1"
    (pin passive line (at -3.81 0 0) (length 2.54) (name "K" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at 3.81 0 180) (length 2.54) (name "A" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
  )
  (embedded_fonts no)
)`

const rawConnectorGenericConn01x02Symbol = `(symbol "Connector_Generic:Conn_01x02"
  (pin_names (offset 1.016) (hide yes))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "J" (at 0 2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "Conn_01x02" (at 0 -5.08 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Generic connector, single row, 01x02, script generated (kicad-library-utils/schlib/autogen/connector/)" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "connector" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "Connector*:*_1x??_*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "Conn_01x02_1_1"
    (rectangle (start -1.27 1.27) (end 1.27 -3.81) (stroke (width 0.254) (type default)) (fill (type background)))
    (rectangle (start -1.27 0.127) (end 0 -0.127) (stroke (width 0.1524) (type default)) (fill (type none)))
    (rectangle (start -1.27 -2.413) (end 0 -2.667) (stroke (width 0.1524) (type default)) (fill (type none)))
    (pin passive line (at -5.08 0 0) (length 3.81) (name "Pin_1" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at -5.08 -2.54 0) (length 3.81) (name "Pin_2" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
  )
  (embedded_fonts no)
)`
