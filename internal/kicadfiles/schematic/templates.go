package schematic

import (
	"bytes"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

const schematicGeneratorVersion = "10.0"

type TemplatePin struct {
	Number string
	Offset kicadfiles.Point
}

type embeddedTemplate struct {
	bodyName                       string
	pinType                        string
	pinX                           float64
	power                          bool
	localLibrary                   bool
	connectionPinOverrideLocalOnly bool
	pins                           []TemplatePin
	// connectionPinOverride stores KiCad's serialized connection anchor when
	// it differs from the raw library pin coordinate used to render the body.
	// These values are empirical KiCad-backed data and must not be normalized
	// to raw pin offsets without rerunning the ERC promotion fixtures.
	connectionPinOverride map[string]kicadfiles.Point
	rawBody               string
}

var embeddedSymbolTemplates = map[string]embeddedTemplate{
	"amplifier_operational:lmv321": {bodyName: "LMV321", pinType: "passive", pins: []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: 0}},
		{Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(2.54)}},
		{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)}},
		{Number: "4", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)}},
		{Number: "5", Offset: kicadfiles.Point{Y: kicadfiles.MM(-2.54)}},
	}},
	"connector_generic:conn_01x02": {bodyName: "Conn_01x02", pinType: "passive", localLibrary: true, pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08)}}, {Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(-2.54)}}}, connectionPinOverride: map[string]kicadfiles.Point{"2": {X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(2.54)}}, rawBody: rawConnectorGenericConn01x02Symbol},
	"connector_generic:conn_01x03": {bodyName: "Conn_01x03", pinType: "passive", localLibrary: true, pins: connectorTemplatePins(3)},
	// Conn_01x04 remains global-library-backed because its generated local
	// library body produces a KiCad lib_symbol_mismatch in the I2C fixture.
	"connector_generic:conn_01x04": {bodyName: "Conn_01x04", pinType: "passive", pins: connectorTemplatePins(4), connectionPinOverride: map[string]kicadfiles.Point{"4": {X: kicadfiles.MM(-5.08), Y: kicadfiles.MM(5.08)}}, rawBody: rawConnectorGenericConn01x04Symbol},
	"kicadai:usb_c_receptacle_poweronly_6p": {
		bodyName:     "USB_C_Receptacle_PowerOnly_6P",
		pinType:      "passive",
		localLibrary: true,
		pins: []TemplatePin{
			{Number: "A5", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-5.08)}},
			{Number: "A9", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(7.62)}},
			{Number: "A12", Offset: kicadfiles.Point{Y: kicadfiles.MM(-17.78)}},
			{Number: "B5", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-7.62)}},
			{Number: "B9", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(7.62)}},
			{Number: "B12", Offset: kicadfiles.Point{Y: kicadfiles.MM(-17.78)}},
			{Number: "SH", Offset: kicadfiles.Point{X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(-17.78)}},
		},
		connectionPinOverride: map[string]kicadfiles.Point{
			"A5":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(5.08)},
			"A9":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-7.62)},
			"A12": {Y: kicadfiles.MM(17.78)},
			"B5":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(7.62)},
			"B9":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-7.62)},
			"B12": {Y: kicadfiles.MM(17.78)},
			"SH":  {X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(17.78)},
		},
	},
	"kicadai:usb_c_receptacle_poweronly_full": {
		bodyName:                       "USB_C_Receptacle_PowerOnly_Full",
		pinType:                        "passive",
		localLibrary:                   true,
		connectionPinOverrideLocalOnly: true,
		// KiCad's saved symbol connection anchors differ from the raw template
		// primitives only for these seven pins. A5, A9, B4, and B5 retain their
		// raw offsets; the KiCad-backed ERC fixture proves those anchors are
		// already correct and must not be inverted as a blanket rule.
		connectionPinOverride: map[string]kicadfiles.Point{
			"A4":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-10.16)},
			"B9":  {X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-2.54)},
			"A1":  {Y: kicadfiles.MM(10.16)},
			"A12": {Y: kicadfiles.MM(12.7)},
			"B1":  {Y: kicadfiles.MM(15.24)},
			"B12": {Y: kicadfiles.MM(17.78)},
			"SH":  {X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(20.32)},
		},
		pins: []TemplatePin{
			{Number: "A1", Offset: kicadfiles.Point{Y: kicadfiles.MM(-10.16)}},
			{Number: "A4", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(10.16)}},
			{Number: "A5", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-5.08)}},
			{Number: "A9", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(7.62)}},
			{Number: "A12", Offset: kicadfiles.Point{Y: kicadfiles.MM(-12.7)}},
			{Number: "B1", Offset: kicadfiles.Point{Y: kicadfiles.MM(-15.24)}},
			{Number: "B4", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(5.08)}},
			{Number: "B5", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(-7.62)}},
			{Number: "B9", Offset: kicadfiles.Point{X: kicadfiles.MM(15.24), Y: kicadfiles.MM(2.54)}},
			{Number: "B12", Offset: kicadfiles.Point{Y: kicadfiles.MM(-17.78)}},
			{Number: "SH", Offset: kicadfiles.Point{X: kicadfiles.MM(-7.62), Y: kicadfiles.MM(-20.32)}},
		},
	},
	"device:c":           {bodyName: "C", pinType: "passive", pins: deviceCTemplatePins(), rawBody: rawDeviceCSymbol},
	"device:c_polarized": {bodyName: "C_Polarized", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(-5.08)}}, {Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(5.08)}}}},
	"device:d":           {bodyName: "D", pinType: "passive", pins: twoPinTemplatePins()},
	"device:d_tvs":       {bodyName: "D_TVS", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-3.81)}}, {Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(3.81)}}}, rawBody: rawDeviceDTVSSymbol},
	"device:fuse": {
		bodyName: "Fuse",
		pinType:  "passive",
		pins:     deviceRTemplatePins(),
		// Keep these anchors aligned with KiCad ERC behavior for the embedded
		// Device:Fuse body. They intentionally differ from the raw library pin
		// offsets used for symbol-body compatibility.
		connectionPinOverride: map[string]kicadfiles.Point{
			"1": {Y: kicadfiles.MM(-3.81)},
			"2": {Y: kicadfiles.MM(3.81)},
		},
		rawBody: rawDeviceFuseSymbol,
	},
	"device:led": {bodyName: "LED", pinType: "passive", pins: []TemplatePin{{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-3.81)}}, {Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(3.81)}}}, rawBody: rawDeviceLEDSymbol},
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
	"kicadai:usb_cc_r": {
		bodyName:     "USB_CC_R",
		pinType:      "passive",
		localLibrary: true,
		pins:         deviceRTemplatePins(),
	},
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
	// Template keys are normalized to lowercase before lookup.
	"regulator_linear:ams1117-3.3": {
		bodyName: "AMS1117-3.3",
		// Keep these pins passive and horizontally aligned with the block-level
		// AMS1117 anchors. KiCad 10 ERC currently regresses this promoted fixture
		// when the generated local symbol uses power_in/power_out pin typing.
		pinType:      "passive",
		localLibrary: true,
		pins: []TemplatePin{
			{Number: "1", Offset: kicadfiles.Point{}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(7.62)}},
			{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-7.62)}},
		},
	},
	"power:vcc": powerTemplate("VCC", 5.08),
	"power:vdd": powerTemplate("VDD", 5.08),
	"power:vee": powerTemplate("VEE", -5.08),
	"power:vss": powerTemplate("VSS", -5.08),
	"sensor:generic_i2c": {
		bodyName:     "Generic_I2C",
		pinType:      "bidirectional",
		localLibrary: true,
		// Local-library connection anchors are Y-inverted relative to the raw
		// body pin coordinates under KiCad's schematic coordinate convention.
		connectionPinOverride: map[string]kicadfiles.Point{
			"1": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(3.81)},
			"2": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-3.81)},
			"3": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)},
			"4": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)},
		},
		pins: []TemplatePin{
			{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-3.81)}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(3.81)}},
			{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)}},
			{Number: "4", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)}},
			{Number: "5", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: 0}},
		},
	},
	"kicadai:ams1117_schematic": {
		bodyName: "AMS1117_Schematic", pinType: "passive", localLibrary: true,
		// KiCad-backed ERC uses a positive Y connection anchor for pin 1 even
		// though the raw body pin is stored at negative Y.
		connectionPinOverride: map[string]kicadfiles.Point{
			"1": {Y: kicadfiles.MM(7.62)},
			"2": {X: kicadfiles.MM(7.62)},
			"3": {X: kicadfiles.MM(-7.62)},
		},
		pins: []TemplatePin{
			{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(-7.62)}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(7.62)}},
			{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-7.62)}},
		},
	},
	"sensor:generic_i2c_8p": {
		bodyName: "Generic_I2C_8P", pinType: "passive", localLibrary: true,
		// Preserve raw body coordinates in pins and KiCad-resolved connection
		// anchors separately; the I2C promotion fixture proves these values.
		connectionPinOverride: map[string]kicadfiles.Point{
			"1": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(3.81)},
			"2": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-3.81)},
			"3": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)},
			"4": {X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)},
			"5": {X: kicadfiles.MM(2.54)},
			"6": {X: kicadfiles.MM(2.54), Y: kicadfiles.MM(-2.54)},
			"7": {X: kicadfiles.MM(2.54), Y: kicadfiles.MM(-5.08)},
			"8": {X: kicadfiles.MM(2.54), Y: kicadfiles.MM(-7.62)},
		},
		pins: []TemplatePin{
			{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-3.81)}},
			{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(3.81)}},
			{Number: "3", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(-1.27)}},
			{Number: "4", Offset: kicadfiles.Point{X: kicadfiles.MM(-2.54), Y: kicadfiles.MM(1.27)}},
			{Number: "5", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
			{Number: "6", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(2.54)}},
			{Number: "7", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(5.08)}},
			{Number: "8", Offset: kicadfiles.Point{X: kicadfiles.MM(2.54), Y: kicadfiles.MM(7.62)}},
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
	template, ok := pinOffsetTemplate(libraryID)
	if !ok {
		return nil, false
	}
	return cloneTemplatePins(template.pins), true
}

// EmbeddedSymbolBodyBounds returns the visible geometry bounds for a known
// seed symbol relative to its symbol origin.
func EmbeddedSymbolBodyBounds(libraryID string) (SymbolBodyBounds, bool) {
	template, ok := EmbeddedSymbolTemplate(libraryID)
	if !ok {
		return SymbolBodyBounds{}, false
	}
	_, bounds, ok := embeddedSymbolGeometry(parsedGeometryNode(template.Body), 1, 1)
	return bounds, ok
}

// pinOffsetTemplate provides connection geometry for known external symbols
// without making the writer emit a synthetic local library body for them.
func pinOffsetTemplate(libraryID string) (embeddedTemplate, bool) {
	normalized := strings.ToLower(strings.TrimSpace(libraryID))
	if template, ok := embeddedSymbolTemplates[normalized]; ok {
		return template, true
	}
	switch normalized {
	case "connector:usb_c_receptacle_usb2.0":
		template, ok := embeddedSymbolTemplates["kicadai:usb_c_receptacle_poweronly_full"]
		return template, ok
	default:
		return embeddedTemplate{}, false
	}
}

// LocalSymbolLibrary renders a project-local KiCad symbol library containing
// the known embedded template for libraryID. The returned library uses the
// unqualified symbol name because KiCad resolves the nickname through
// sym-lib-table.
func LocalSymbolLibrary(libraryID string) ([]byte, bool) {
	return LocalSymbolLibraryForIDs([]string{libraryID})
}

// LocalSymbolLibraryForIDs renders one project-local KiCad symbol library for
// the subset of library IDs whose templates require project-local resolution.
func LocalSymbolLibraryForIDs(libraryIDs []string) ([]byte, bool) {
	bodies := make([]sexpr.Node, 0, len(libraryIDs))
	seen := map[string]struct{}{}
	for _, libraryID := range libraryIDs {
		libraryID = CanonicalEmbeddedSymbolLibraryID(libraryID)
		template, ok := embeddedSymbolTemplates[strings.ToLower(libraryID)]
		if !ok || !template.localLibrary || len(template.pins) == 0 {
			continue
		}
		symbolName := libraryID
		if separator := strings.LastIndex(symbolName, ":"); separator >= 0 {
			symbolName = strings.TrimSpace(symbolName[separator+1:])
		}
		if symbolName == "" {
			symbolName = template.bodyName
		}
		if _, ok := seen[strings.ToLower(symbolName)]; ok {
			continue
		}
		seen[strings.ToLower(symbolName)] = struct{}{}
		bodies = append(bodies, symbolBodyFromTemplatePinsWithNestedName(symbolName, template.bodyName, symbolName, template.pinType, template.pins))
	}
	if len(bodies) == 0 {
		return nil, false
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_symbol_lib"),
		sexpr.L(sexpr.A("version"), sexpr.I(kicadfiles.KiCadSchematicFormatWithGeneratorVersion)),
		sexpr.L(sexpr.A("generator"), sexpr.S("kicadai")),
		sexpr.L(sexpr.A("generator_version"), sexpr.S(schematicGeneratorVersion)),
	}
	nodes = append(nodes, bodies...)
	root := sexpr.L(
		nodes...,
	)
	var buf bytes.Buffer
	if err := sexpr.Render(&buf, root); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// LocalSymbolLibraryForRaw renders resolver-provided symbol bodies into a
// project-local library. The raw bodies are parsed and rendered as KiCad
// symbols without changing their semantic content, so the embedded schematic
// body and the project-local table resolve to the same source.
func LocalSymbolLibraryForRaw(rawSymbols []string) ([]byte, bool) {
	bodies := make([]sexpr.Node, 0, len(rawSymbols))
	seen := map[string]struct{}{}
	for _, raw := range rawSymbols {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		body := sexpr.List(rawEmbeddedSymbolBody(raw))
		if len(body) < 2 {
			return nil, false
		}
		head, ok := body[0].(sexpr.Atom)
		if !ok || string(head) != "symbol" {
			return nil, false
		}
		name, ok := body[1].(sexpr.Atom)
		if !ok {
			if stringValue, stringOK := body[1].(sexpr.String); stringOK {
				name = sexpr.Atom(stringValue)
				ok = true
			}
		}
		if !ok || strings.TrimSpace(string(name)) == "" {
			return nil, false
		}
		body = normalizeRawEmbeddedSymbolBody(body)
		nameKey := strings.ToLower(strings.TrimSpace(string(name)))
		if _, ok := seen[nameKey]; ok {
			continue
		}
		seen[nameKey] = struct{}{}
		bodies = append(bodies, body)
	}
	if len(bodies) == 0 {
		return nil, false
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_symbol_lib"),
		sexpr.L(sexpr.A("version"), sexpr.I(kicadfiles.KiCadSchematicFormatWithGeneratorVersion)),
		sexpr.L(sexpr.A("generator"), sexpr.S("kicadai")),
		sexpr.L(sexpr.A("generator_version"), sexpr.S(schematicGeneratorVersion)),
	}
	nodes = append(nodes, bodies...)
	var buf bytes.Buffer
	if err := sexpr.Render(&buf, sexpr.L(nodes...)); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// CanonicalEmbeddedSymbolLibraryID returns the project-local library ID that
// KiCad resolves for a known generated symbol. Local aliases may use a
// convenient AI-facing spelling, but the on-disk library and schematic must
// share the template's canonical symbol name.
func CanonicalEmbeddedSymbolLibraryID(libraryID string) string {
	libraryID = strings.TrimSpace(libraryID)
	template, ok := embeddedSymbolTemplates[strings.ToLower(libraryID)]
	if !ok || !template.localLibrary {
		return libraryID
	}
	separator := strings.LastIndex(libraryID, ":")
	if separator < 0 {
		return template.bodyName
	}
	nickname := strings.TrimSpace(libraryID[:separator])
	if nickname == "" {
		return template.bodyName
	}
	return nickname + ":" + template.bodyName
}

// EmbeddedSymbolConnectionPinOffset returns a KiCad-validated connection
// anchor override for symbols whose ERC-resolved connection point differs from
// the canonical raw symbol primitive used for library-body compatibility.
func EmbeddedSymbolConnectionPinOffset(libraryID string, pinNumber string) (kicadfiles.Point, bool) {
	normalized := strings.ToLower(strings.TrimSpace(libraryID))
	template, ok := pinOffsetTemplate(libraryID)
	if !ok || len(template.connectionPinOverride) == 0 {
		return kicadfiles.Point{}, false
	}
	if template.connectionPinOverrideLocalOnly {
		resolved, resolvedOK := embeddedSymbolTemplates[normalized]
		if !resolvedOK || !resolved.localLibrary {
			return kicadfiles.Point{}, false
		}
	}
	offset, ok := template.connectionPinOverride[strings.TrimSpace(pinNumber)]
	return offset, ok
}

func twoPinTemplatePins() []TemplatePin {
	return []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{X: kicadfiles.MM(-5.08), Y: 0}},
		{Number: "2", Offset: kicadfiles.Point{X: kicadfiles.MM(5.08), Y: 0}},
	}
}

func deviceCTemplatePins() []TemplatePin {
	return []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(3.81)}},
		{Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(-3.81)}},
	}
}

func deviceRTemplatePins() []TemplatePin {
	return []TemplatePin{
		{Number: "1", Offset: kicadfiles.Point{Y: kicadfiles.MM(3.81)}},
		{Number: "2", Offset: kicadfiles.Point{Y: kicadfiles.MM(-3.81)}},
	}
}

func connectorTemplatePins(count int) []TemplatePin {
	pins := make([]TemplatePin, count)
	for index := 0; index < count; index++ {
		pins[index] = TemplatePin{
			Number: strconv.Itoa(index + 1),
			Offset: kicadfiles.Point{
				X: kicadfiles.MM(-5.08),
				Y: connectorTemplatePinY(count, index),
			},
		}
	}
	return pins
}

func connectorTemplatePinY(count int, zeroBasedPin int) kicadfiles.IU {
	if count <= 2 {
		return -kicadfiles.MM(2.54 * float64(zeroBasedPin))
	}
	return kicadfiles.MM(2.54 - 2.54*float64(zeroBasedPin))
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
	libraryID = CanonicalEmbeddedSymbolLibraryID(libraryID)
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

// EnsureEmbeddedSymbolFromRaw adds a library-resolved KiCad symbol body to the
// schematic. Library files store the top-level symbol under its unqualified
// name; embedded schematic symbols use the fully qualified library ID.
func EnsureEmbeddedSymbolFromRaw(file *SchematicFile, libraryID, raw string) bool {
	if file == nil || strings.TrimSpace(libraryID) == "" || strings.TrimSpace(raw) == "" {
		return false
	}
	body := rawEmbeddedSymbolBody(raw)
	if len(body) < 2 {
		return false
	}
	head, ok := body[0].(sexpr.Atom)
	if !ok || string(head) != "symbol" {
		return false
	}
	switch body[1].(type) {
	case sexpr.Atom, sexpr.String:
	default:
		return false
	}
	body[1] = sexpr.S(strings.TrimSpace(libraryID))
	body = normalizeRawEmbeddedSymbolBody(body)
	return ensureEmbeddedSymbolBody(file, libraryID, body)
}

func normalizeRawEmbeddedSymbolBody(body sexpr.List) sexpr.List {
	if len(body) < 2 {
		return body
	}
	result := make(sexpr.List, 0, len(body)+len(embeddedSymbolDefaults())+1)
	result = append(result, body[:2]...)
	insertedDefaults := false
	for _, item := range body[2:] {
		if embeddedSymbolNodeHead(item) == "property" {
			if property, ok := item.(sexpr.List); ok {
				item = normalizeRawEmbeddedSymbolProperty(property)
			}
		}
		if !insertedDefaults && (embeddedSymbolNodeHead(item) == "property" || embeddedSymbolNodeHead(item) == "symbol") {
			result = appendMissingEmbeddedSymbolDefaults(result, body)
			insertedDefaults = true
		}
		result = append(result, item)
	}
	if !insertedDefaults {
		result = appendMissingEmbeddedSymbolDefaults(result, body)
	}
	result = canonicalizeEmbeddedSymbolProperties(result)
	if !hasEmbeddedSymbolChild(body, "embedded_fonts", "") {
		result = append(result, sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")))
	}
	return result
}

func canonicalizeEmbeddedSymbolProperties(body sexpr.List) sexpr.List {
	properties := map[string]sexpr.List{}
	var extra []sexpr.Node
	for _, item := range body[2:] {
		if embeddedSymbolNodeHead(item) != "property" {
			continue
		}
		property, ok := item.(sexpr.List)
		if !ok {
			continue
		}
		name := embeddedSymbolNodeKey(property)
		if name == "Reference" || name == "Value" || name == "Footprint" || name == "Datasheet" || name == "Description" {
			properties[name] = property
		} else {
			extra = append(extra, property)
		}
	}
	result := append(sexpr.List(nil), body[:2]...)
	inserted := false
	for _, item := range body[2:] {
		if embeddedSymbolNodeHead(item) == "property" {
			continue
		}
		if !inserted && embeddedSymbolNodeHead(item) == "symbol" {
			result = appendCanonicalEmbeddedSymbolProperties(result, properties, extra)
			inserted = true
		}
		result = append(result, item)
	}
	if !inserted {
		result = appendCanonicalEmbeddedSymbolProperties(result, properties, extra)
	}
	return result
}

func appendCanonicalEmbeddedSymbolProperties(result sexpr.List, properties map[string]sexpr.List, extra []sexpr.Node) sexpr.List {
	for _, name := range []string{"Reference", "Value", "Footprint", "Datasheet", "Description"} {
		if property, ok := properties[name]; ok {
			result = append(result, property)
		}
	}
	return append(result, extra...)
}

func normalizeRawEmbeddedSymbolProperty(property sexpr.List) sexpr.List {
	if len(property) < 3 || embeddedSymbolNodeHead(property) != "property" {
		return property
	}
	name := embeddedSymbolNodeKey(property)
	standard := map[string]bool{
		"Reference":   true,
		"Value":       true,
		"Footprint":   true,
		"Datasheet":   true,
		"Description": true,
	}
	if !standard[name] {
		return property
	}
	existing := map[string]sexpr.Node{}
	var remainder []sexpr.Node
	for _, item := range property[3:] {
		head := embeddedSymbolNodeHead(item)
		switch head {
		case "at", "show_name", "do_not_autoplace", "hide", "effects":
			existing[head] = item
		default:
			remainder = append(remainder, item)
		}
	}
	result := append(sexpr.List(nil), property[:3]...)
	if item, ok := existing["at"]; ok {
		result = append(result, item)
	} else {
		result = append(result, sexpr.L(sexpr.A("at"), sexpr.F(0), sexpr.F(0), sexpr.F(0)))
	}
	if item, ok := existing["show_name"]; ok {
		result = append(result, item)
	} else {
		result = append(result, sexpr.L(sexpr.A("show_name"), sexpr.A("no")))
	}
	if item, ok := existing["do_not_autoplace"]; ok {
		result = append(result, item)
	} else {
		result = append(result, sexpr.L(sexpr.A("do_not_autoplace"), sexpr.A("no")))
	}
	if name == "Footprint" || name == "Datasheet" || name == "Description" {
		if item, ok := existing["hide"]; ok {
			result = append(result, item)
		} else {
			result = append(result, sexpr.L(sexpr.A("hide"), sexpr.A("yes")))
		}
	}
	if item, ok := existing["effects"]; ok {
		result = append(result, item)
	} else {
		result = append(result, sexpr.L(
			sexpr.A("effects"),
			sexpr.L(sexpr.A("font"), sexpr.L(sexpr.A("size"), sexpr.F(1.27), sexpr.F(1.27))),
		))
	}
	return append(result, remainder...)
}

func appendMissingEmbeddedSymbolDefaults(result, body sexpr.List) sexpr.List {
	for _, item := range embeddedSymbolDefaults() {
		head := embeddedSymbolNodeHead(item)
		key := embeddedSymbolNodeKey(item)
		if hasEmbeddedSymbolChild(body, head, key) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func hasEmbeddedSymbolChild(body sexpr.List, head, key string) bool {
	for _, item := range body[2:] {
		if embeddedSymbolNodeHead(item) != head {
			continue
		}
		if key == "" || embeddedSymbolNodeKey(item) == key {
			return true
		}
	}
	return false
}

func embeddedSymbolNodeHead(node sexpr.Node) string {
	list, ok := node.(sexpr.List)
	if !ok || len(list) == 0 {
		return ""
	}
	head, ok := list[0].(sexpr.Atom)
	if !ok {
		return ""
	}
	return string(head)
}

func embeddedSymbolNodeKey(node sexpr.Node) string {
	list, ok := node.(sexpr.List)
	if !ok || len(list) < 2 {
		return ""
	}
	switch value := list[1].(type) {
	case sexpr.Atom:
		return string(value)
	case sexpr.String:
		return string(value)
	default:
		return ""
	}
}

// EmbeddedSymbolPresent reports whether the schematic already contains a
// usable embedded body for libraryID.
func EmbeddedSymbolPresent(file *SchematicFile, libraryID string) bool {
	if file == nil {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(libraryID))
	if normalized == "" {
		return false
	}
	for _, symbol := range file.LibSymbols {
		if strings.ToLower(strings.TrimSpace(symbol.LibraryID)) == normalized && len(symbol.Body) > 0 {
			return true
		}
	}
	return false
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
	return symbolBodyFromTemplatePinsWithNestedName(libraryID, bodyName, bodyName, pinType, pins)
}

func symbolBodyFromTemplatePinsWithNestedName(libraryID, bodyName, nestedName, pinType string, pins []TemplatePin) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(libraryID)}
	if isConnectorTemplateBodyName(bodyName) {
		nodes = append(nodes, sexpr.L(
			sexpr.A("pin_names"),
			sexpr.L(sexpr.A("offset"), sexpr.X("1.016")),
			sexpr.L(sexpr.A("hide"), sexpr.A("yes")),
		))
	}
	pinNodes := []sexpr.Node{sexpr.A("symbol"), sexpr.S(nestedName + "_1_1")}
	if body := embeddedTemplateBody(bodyName, pins); len(body) > 0 {
		pinNodes = append(pinNodes, body)
	}
	for _, pin := range pins {
		pinNodes = append(pinNodes, embeddedTemplatePin(bodyName, pinType, pin))
	}
	nodes = append(nodes, embeddedSymbolDefaults()...)
	nodes = append(nodes,
		sexpr.L(pinNodes...),
		// KiCad 10.0.3 writes embedded_fonts inside each embedded lib symbol.
		sexpr.L(sexpr.A("embedded_fonts"), sexpr.A("no")),
	)
	return sexpr.L(nodes...)
}

func embeddedTemplateBody(bodyName string, pins []TemplatePin) sexpr.List {
	if len(pins) == 0 {
		return nil
	}
	if isConnectorTemplateBodyName(bodyName) {
		return connectorTemplateBody(pins)
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

func connectorTemplateBody(pins []TemplatePin) sexpr.List {
	minY, maxY := pins[0].Offset.Y, pins[0].Offset.Y
	for _, pin := range pins[1:] {
		if pin.Offset.Y < minY {
			minY = pin.Offset.Y
		}
		if pin.Offset.Y > maxY {
			maxY = pin.Offset.Y
		}
	}
	padding := kicadfiles.MM(1.27)
	return sexpr.L(
		sexpr.A("rectangle"),
		sexpr.L(sexpr.A("start"), sexpr.F(-1.27), sexpr.F(templatePinMM(maxY+padding))),
		sexpr.L(sexpr.A("end"), sexpr.F(1.27), sexpr.F(templatePinMM(minY-padding))),
		sexpr.L(
			sexpr.A("stroke"),
			sexpr.L(sexpr.A("width"), sexpr.X("0.254")),
			sexpr.L(sexpr.A("type"), sexpr.A("default")),
		),
		sexpr.L(sexpr.A("fill"), sexpr.L(sexpr.A("type"), sexpr.A("background"))),
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

func embeddedTemplatePin(bodyName string, pinType string, pin TemplatePin) sexpr.List {
	pinLength := "2.54"
	pinName := "~"
	if isConnectorTemplateBodyName(bodyName) {
		pinLength = "3.81"
		pinName = "Pin_" + pin.Number
	}
	return sexpr.L(
		sexpr.A("pin"),
		sexpr.A(pinType),
		sexpr.A("line"),
		sexpr.L(sexpr.A("at"), sexpr.F(templatePinMM(pin.Offset.X)), sexpr.F(templatePinMM(pin.Offset.Y)), sexpr.I(templatePinRotation(pin.Offset))),
		sexpr.L(sexpr.A("length"), sexpr.X(pinLength)),
		sexpr.L(sexpr.A("name"), sexpr.S(pinName), renderEffects(false)),
		sexpr.L(sexpr.A("number"), sexpr.S(pin.Number), renderEffects(false)),
	)
}

func isConnectorTemplateBodyName(bodyName string) bool {
	return strings.HasPrefix(bodyName, "Conn_01x")
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

const rawDeviceCSymbol = `(symbol "Device:C"
  (pin_numbers (hide yes))
  (pin_names (offset 0.254))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "C" (at 0.635 2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27)) (justify left)))
  (property "Value" "C" (at 0.635 -2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27)) (justify left)))
  (property "Footprint" "" (at 0.9652 -3.81 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Unpolarized capacitor" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "cap capacitor" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "C_*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "C_0_1"
    (polyline (pts (xy -2.032 0.762) (xy 2.032 0.762)) (stroke (width 0.508) (type default)) (fill (type none)))
    (polyline (pts (xy -2.032 -0.762) (xy 2.032 -0.762)) (stroke (width 0.508) (type default)) (fill (type none)))
  )
  (symbol "C_1_1"
    (pin passive line (at 0 3.81 270) (length 2.794) (name "" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at 0 -3.81 90) (length 2.794) (name "" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
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

const rawDeviceDTVSSymbol = `(symbol "Device:D_TVS"
  (pin_numbers (hide yes))
  (pin_names (offset 1.016) (hide yes))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "D" (at 0 2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "D_TVS" (at 0 -2.54 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Bidirectional transient-voltage-suppression diode" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "diode TVS thyrector" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "TO-???* *_Diode_* *SingleDiode* D_*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "D_TVS_0_1"
    (polyline (pts (xy -2.54 1.27) (xy -2.54 -1.27) (xy 2.54 1.27) (xy 2.54 -1.27) (xy -2.54 1.27)) (stroke (width 0.254) (type default)) (fill (type none)))
    (polyline (pts (xy 0.508 1.27) (xy 0 1.27) (xy 0 -1.27) (xy -0.508 -1.27)) (stroke (width 0.254) (type default)) (fill (type none)))
    (polyline (pts (xy 1.27 0) (xy -1.27 0)) (stroke (width 0) (type default)) (fill (type none)))
  )
  (symbol "D_TVS_1_1"
    (pin passive line (at -3.81 0 0) (length 2.54) (name "A1" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at 3.81 0 180) (length 2.54) (name "A2" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
  )
  (embedded_fonts no)
)`

const rawDeviceFuseSymbol = `(symbol "Device:Fuse"
  (pin_numbers (hide yes))
  (pin_names (offset 0))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "F" (at 2.032 0 90) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "Fuse" (at -1.905 0 90) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at -1.778 0 90) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Fuse" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "fuse" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "*Fuse*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "Fuse_0_1"
    (rectangle (start -0.762 -2.54) (end 0.762 2.54) (stroke (width 0.254) (type default)) (fill (type none)))
    (polyline (pts (xy 0 2.54) (xy 0 -2.54)) (stroke (width 0) (type default)) (fill (type none)))
  )
  (symbol "Fuse_1_1"
    (pin passive line (at 0 3.81 270) (length 1.27) (name "" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at 0 -3.81 90) (length 1.27) (name "" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
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

const rawConnectorGenericConn01x04Symbol = `(symbol "Connector_Generic:Conn_01x04"
  (pin_names (offset 1.016) (hide yes))
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (in_pos_files yes)
  (duplicate_pin_numbers_are_jumpers no)
  (property "Reference" "J" (at 0 5.08 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Value" "Conn_01x04" (at 0 -7.62 0) (show_name no) (do_not_autoplace no) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Description" "Generic connector, single row, 01x04, script generated (kicad-library-utils/schlib/autogen/connector/)" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_keywords" "connector" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (property "ki_fp_filters" "Connector*:*_1x??_*" (at 0 0 0) (show_name no) (do_not_autoplace no) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "Conn_01x04_1_1"
    (rectangle (start -1.27 3.81) (end 1.27 -6.35) (stroke (width 0.254) (type default)) (fill (type background)))
    (rectangle (start -1.27 2.667) (end 0 2.413) (stroke (width 0.1524) (type default)) (fill (type none)))
    (rectangle (start -1.27 0.127) (end 0 -0.127) (stroke (width 0.1524) (type default)) (fill (type none)))
    (rectangle (start -1.27 -2.413) (end 0 -2.667) (stroke (width 0.1524) (type default)) (fill (type none)))
    (rectangle (start -1.27 -4.953) (end 0 -5.207) (stroke (width 0.1524) (type default)) (fill (type none)))
    (pin passive line (at -5.08 2.54 0) (length 3.81) (name "Pin_1" (effects (font (size 1.27 1.27)))) (number "1" (effects (font (size 1.27 1.27)))))
    (pin passive line (at -5.08 0 0) (length 3.81) (name "Pin_2" (effects (font (size 1.27 1.27)))) (number "2" (effects (font (size 1.27 1.27)))))
    (pin passive line (at -5.08 -2.54 0) (length 3.81) (name "Pin_3" (effects (font (size 1.27 1.27)))) (number "3" (effects (font (size 1.27 1.27)))))
    (pin passive line (at -5.08 -5.08 0) (length 3.81) (name "Pin_4" (effects (font (size 1.27 1.27)))) (number "4" (effects (font (size 1.27 1.27)))))
  )
  (embedded_fonts no)
)`
