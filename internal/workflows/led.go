package workflows

import (
	"fmt"
	"regexp"
	"strings"

	"kicadai/internal/schematic"
)

type LEDDemoIntent struct {
	Document  schematic.DocumentRef `json:"document"`
	Origin    schematic.Point       `json:"origin"`
	Prefix    string                `json:"prefix"`
	Libraries LEDDemoLibraries      `json:"libraries"`
}

type LEDDemoLibraries struct {
	VCC      string `json:"vcc"`
	GND      string `json:"gnd"`
	Resistor string `json:"resistor"`
	LED      string `json:"led"`
}

type AutomationPlan struct {
	Operations []schematic.PlannedOperation `json:"operations"`
}

const (
	DefaultLEDDemoPrefix = "LED"

	kiCadIUMil         int64 = 25_400
	ledSymbolSpacing         = 300 * kiCadIUMil
	ledSymbolPinOffset       = 100 * kiCadIUMil

	libraryIDVCC      = "power:VCC"
	libraryIDGND      = "power:GND"
	libraryIDResistor = "Device:R"
	libraryIDLED      = "Device:LED"
)

var unsafeNetLabelChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func PlanLEDDemo(intent LEDDemoIntent) (AutomationPlan, error) {
	if err := schematic.ValidateDocument(intent.Document); err != nil {
		return AutomationPlan{}, err
	}

	prefix := sanitizeNetLabelPart(intent.Prefix)
	if prefix == "" {
		prefix = DefaultLEDDemoPrefix
	}

	origin := intent.Origin
	libraries := intent.Libraries.withDefaults()
	symbols := []schematic.AddSymbolRequest{
		{Document: intent.Document, LibraryID: libraries.VCC, Reference: "#PWR?", Value: "VCC", Position: point(origin, 0, 0)},
		{Document: intent.Document, LibraryID: libraries.Resistor, Reference: "R?", Value: "1k", Position: point(origin, ledSymbolSpacing, 0)},
		{Document: intent.Document, LibraryID: libraries.LED, Reference: "D?", Value: prefix, Position: point(origin, 2*ledSymbolSpacing, 0)},
		{Document: intent.Document, LibraryID: libraries.GND, Reference: "#PWR?", Value: "GND", Position: point(origin, 3*ledSymbolSpacing, 0)},
	}

	operations := make([]schematic.PlannedOperation, 0, 8)
	for _, symbol := range symbols {
		operation, err := schematic.PlanAddSymbol(symbol)
		if err != nil {
			return AutomationPlan{}, err
		}
		operations = append(operations, operation)
	}

	wires := []schematic.AddWireRequest{
		{Document: intent.Document, Points: []schematic.Point{point(origin, 0, 0), point(origin, ledSymbolSpacing-ledSymbolPinOffset, 0)}},
		{Document: intent.Document, Points: []schematic.Point{point(origin, ledSymbolSpacing+ledSymbolPinOffset, 0), point(origin, 2*ledSymbolSpacing-ledSymbolPinOffset, 0)}},
		{Document: intent.Document, Points: []schematic.Point{point(origin, 2*ledSymbolSpacing+ledSymbolPinOffset, 0), point(origin, 3*ledSymbolSpacing, 0)}},
	}
	for _, wire := range wires {
		operation, err := schematic.PlanAddWire(wire)
		if err != nil {
			return AutomationPlan{}, err
		}
		operations = append(operations, operation)
	}

	label, err := schematic.PlanAddLabel(schematic.AddLabelRequest{
		Document:  intent.Document,
		Text:      fmt.Sprintf("%s_OUT", prefix),
		LabelType: schematic.LabelTypeLocal,
		Position:  point(origin, 2*ledSymbolSpacing-ledSymbolPinOffset, 0),
	})
	if err != nil {
		return AutomationPlan{}, err
	}
	operations = append(operations, label)

	return AutomationPlan{Operations: operations}, nil
}

func point(origin schematic.Point, dx int64, dy int64) schematic.Point {
	return schematic.Point{X: origin.X + dx, Y: origin.Y + dy}
}

func (l LEDDemoLibraries) withDefaults() LEDDemoLibraries {
	if strings.TrimSpace(l.VCC) == "" {
		l.VCC = libraryIDVCC
	}
	if strings.TrimSpace(l.GND) == "" {
		l.GND = libraryIDGND
	}
	if strings.TrimSpace(l.Resistor) == "" {
		l.Resistor = libraryIDResistor
	}
	if strings.TrimSpace(l.LED) == "" {
		l.LED = libraryIDLED
	}
	return l
}

func sanitizeNetLabelPart(value string) string {
	trimmed := strings.TrimSpace(value)
	sanitized := unsafeNetLabelChars.ReplaceAllString(trimmed, "_")
	return strings.Trim(sanitized, "_")
}
