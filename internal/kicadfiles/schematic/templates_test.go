package schematic

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func TestEmbeddedSymbolTemplateRendersSupportedSeedSymbols(t *testing.T) {
	tests := []string{
		"Connector_Generic:Conn_01x02",
		"Connector_Generic:Conn_01x03",
		"kicadai:USB_C_Receptacle_PowerOnly_6P",
		"Device:R",
		"Device:C",
		"Device:D",
		"Device:D_TVS",
		"Device:Fuse",
		"Device:LED",
		"kicadai:USB_CC_R",
		"power:GND",
		"power:VCC",
		"power:+3.3V",
		"power:+3V3",
		"power:+5V",
		"power:+12V",
		"power:-12V",
		"power:PWR_FLAG",
		"Regulator_Linear:AMS1117-3.3",
		"power:VDD",
		"power:VEE",
		"power:VSS",
	}
	for _, libraryID := range tests {
		t.Run(libraryID, func(t *testing.T) {
			template, ok := EmbeddedSymbolTemplate(libraryID)
			if !ok {
				t.Fatalf("EmbeddedSymbolTemplate(%q) not supported", libraryID)
			}
			schematic := minimalSchematic()
			schematic.LibSymbols = []EmbeddedSymbol{template}
			var buf bytes.Buffer
			if err := Write(&buf, schematic); err != nil {
				t.Fatalf("Write returned error: %v", err)
			}
			output := buf.String()
			if !strings.Contains(output, `"`+libraryID+`"`) || !strings.Contains(output, "(pin") || !strings.Contains(output, "(embedded_fonts no)") {
				t.Fatalf("template output missing expected KiCad symbol body:\n%s", output)
			}
		})
	}
}

func TestEnsureEmbeddedSymbolIsIdempotentAndPreservesUnsupportedLibraries(t *testing.T) {
	schematic := minimalSchematic()
	if !EnsureEmbeddedSymbol(&schematic, "Device:R") {
		t.Fatal("expected Device:R template to be supported")
	}
	if !EnsureEmbeddedSymbol(&schematic, "device:r") {
		t.Fatal("expected case-insensitive duplicate to be supported")
	}
	if EnsureEmbeddedSymbol(&schematic, "Amplifier_Operational:TL072") {
		t.Fatal("expected unsupported library to be left untouched")
	}
	if len(schematic.LibSymbols) != 1 {
		t.Fatalf("lib symbols = %d, want 1", len(schematic.LibSymbols))
	}
	if schematic.LibSymbols[0].LibraryID != "Device:R" || len(schematic.LibSymbols[0].Body) == 0 {
		t.Fatalf("unexpected template: %#v", schematic.LibSymbols[0])
	}
}

func TestEnsureEmbeddedSymbolFromRawQualifiesLibraryID(t *testing.T) {
	var file SchematicFile
	raw := `(symbol "Block" (property "Reference" "J") (symbol "Block_1_1" (pin passive line (at -5.08 0 0) (length 2.54) (name "IN") (number "1"))))`
	if !EnsureEmbeddedSymbolFromRaw(&file, "Custom:Block", raw) {
		t.Fatal("expected raw symbol to be embedded")
	}
	if len(file.LibSymbols) != 1 || len(file.LibSymbols[0].Body) < 2 {
		t.Fatalf("embedded symbols = %#v", file.LibSymbols)
	}
	if got := file.LibSymbols[0].Body[1]; got != sexpr.S("Custom:Block") {
		t.Fatalf("qualified library ID = %#v, want Custom:Block", got)
	}
}

func TestEmbeddedSymbolPinOffsets(t *testing.T) {
	pins, ok := EmbeddedSymbolPinOffsets("Device:R")
	if !ok || len(pins) != 2 {
		t.Fatalf("Device:R pins = %#v ok=%v, want two pins", pins, ok)
	}
	if pins[0].Number != "1" || pins[0].Offset.Y != kicadfiles.MM(3.81) || pins[1].Number != "2" || pins[1].Offset.Y != kicadfiles.MM(-3.81) {
		t.Fatalf("unexpected two-pin offsets: %#v", pins)
	}
	capacitorPins, ok := EmbeddedSymbolPinOffsets("Device:C")
	if !ok || len(capacitorPins) != 2 || capacitorPins[0].Offset.X != 0 || capacitorPins[0].Offset.Y != kicadfiles.MM(3.81) || capacitorPins[1].Offset.X != 0 || capacitorPins[1].Offset.Y != kicadfiles.MM(-3.81) {
		t.Fatalf("unexpected capacitor offsets: %#v ok=%v", capacitorPins, ok)
	}
	connectorPins, ok := EmbeddedSymbolPinOffsets("Connector_Generic:Conn_01x02")
	if !ok || len(connectorPins) != 2 || connectorPins[0].Offset.Y != 0 || connectorPins[1].Offset.Y != kicadfiles.MM(-2.54) {
		t.Fatalf("unexpected connector offsets: %#v ok=%v", connectorPins, ok)
	}
	connector4Pins, ok := EmbeddedSymbolPinOffsets("Connector_Generic:Conn_01x04")
	if !ok || len(connector4Pins) != 4 || connector4Pins[0].Offset.Y != kicadfiles.MM(2.54) || connector4Pins[3].Number != "4" || connector4Pins[3].Offset.Y != kicadfiles.MM(-5.08) {
		t.Fatalf("unexpected 4-pin connector offsets: %#v ok=%v", connector4Pins, ok)
	}
	connector4Pin4Connection, ok := EmbeddedSymbolConnectionPinOffset("Connector_Generic:Conn_01x04", "4")
	if !ok || connector4Pin4Connection.X != kicadfiles.MM(-5.08) || connector4Pin4Connection.Y != kicadfiles.MM(5.08) {
		t.Fatalf("unexpected 4-pin connector connection override: %#v ok=%v", connector4Pin4Connection, ok)
	}
	usbA5Connection, ok := EmbeddedSymbolConnectionPinOffset("kicadai:USB_C_Receptacle_PowerOnly_6P", "A5")
	if !ok || usbA5Connection.X != kicadfiles.MM(15.24) || usbA5Connection.Y != kicadfiles.MM(5.08) {
		t.Fatalf("unexpected USB-C A5 connection override: %#v ok=%v", usbA5Connection, ok)
	}
	usbA9Connection, ok := EmbeddedSymbolConnectionPinOffset("kicadai:USB_C_Receptacle_PowerOnly_6P", "A9")
	if !ok || usbA9Connection.X != kicadfiles.MM(15.24) || usbA9Connection.Y != kicadfiles.MM(-7.62) {
		t.Fatalf("unexpected USB-C A9 connection override: %#v ok=%v", usbA9Connection, ok)
	}
	usbA4Connection, ok := EmbeddedSymbolConnectionPinOffset("kicadai:USB_C_Receptacle_PowerOnly_6P", "A4")
	if ok || usbA4Connection != (kicadfiles.Point{}) {
		t.Fatalf("unexpected USB-C 6P A4 connection override: %#v ok=%v", usbA4Connection, ok)
	}
	connector3Pins, ok := EmbeddedSymbolPinOffsets("Connector_Generic:Conn_01x03")
	if !ok || len(connector3Pins) != 3 || connector3Pins[0].Offset.Y != kicadfiles.MM(2.54) || connector3Pins[2].Number != "3" || connector3Pins[2].Offset.Y != kicadfiles.MM(-2.54) {
		t.Fatalf("unexpected 3-pin connector offsets: %#v ok=%v", connector3Pins, ok)
	}
	usbPins, ok := EmbeddedSymbolPinOffsets("kicadai:USB_C_Receptacle_PowerOnly_6P")
	usbA5Pin, usbA5OK := findTemplatePin(usbPins, "A5")
	usbSHPin, usbSHOK := findTemplatePin(usbPins, "SH")
	if !ok || len(usbPins) != 7 || !usbA5OK || usbA5Pin.Offset.Y != kicadfiles.MM(-5.08) || !usbSHOK || usbSHPin.Offset.X != kicadfiles.MM(-7.62) {
		t.Fatalf("unexpected USB-C power-only offsets: %#v ok=%v", usbPins, ok)
	}
	fullUSBPins, ok := EmbeddedSymbolPinOffsets("kicadai:usb_c_receptacle_poweronly_full")
	fullUSBA5Pin, fullUSBA5OK := findTemplatePin(fullUSBPins, "A5")
	fullUSBSHPin, fullUSBSHOK := findTemplatePin(fullUSBPins, "SH")
	if !ok || len(fullUSBPins) != 11 || !fullUSBA5OK || fullUSBA5Pin.Offset.Y != kicadfiles.MM(-5.08) || !fullUSBSHOK || fullUSBSHPin.Offset.Y != kicadfiles.MM(-20.32) {
		t.Fatalf("unexpected full USB-C power-only offsets: %#v ok=%v", fullUSBPins, ok)
	}
	seenUSBPinOffsets := map[kicadfiles.Point]string{}
	for _, pin := range fullUSBPins {
		if existing, exists := seenUSBPinOffsets[pin.Offset]; exists {
			t.Fatalf("full USB-C power-only pins %s and %s share offset %#v", existing, pin.Number, pin.Offset)
		}
		seenUSBPinOffsets[pin.Offset] = pin.Number
	}
	fusePins, ok := EmbeddedSymbolPinOffsets("Device:Fuse")
	if !ok || len(fusePins) != 2 || fusePins[0].Offset.Y != kicadfiles.MM(3.81) || fusePins[1].Offset.Y != kicadfiles.MM(-3.81) {
		t.Fatalf("unexpected fuse offsets: %#v ok=%v", fusePins, ok)
	}
	tvsPins, ok := EmbeddedSymbolPinOffsets("Device:D_TVS")
	if !ok || len(tvsPins) != 2 || tvsPins[0].Offset.X != kicadfiles.MM(-3.81) || tvsPins[1].Offset.X != kicadfiles.MM(3.81) {
		t.Fatalf("unexpected TVS offsets: %#v ok=%v", tvsPins, ok)
	}
	i2cPins, ok := EmbeddedSymbolPinOffsets("Sensor:Generic_I2C")
	if !ok || len(i2cPins) != 5 || i2cPins[0].Number != "1" || i2cPins[0].Offset.Y != kicadfiles.MM(-3.81) || i2cPins[4].Number != "5" || i2cPins[4].Offset.X != kicadfiles.MM(2.54) {
		t.Fatalf("unexpected generic I2C sensor offsets: %#v ok=%v", i2cPins, ok)
	}
	i2c8Pins, ok := EmbeddedSymbolPinOffsets("Sensor:Generic_I2C_8P")
	if !ok || len(i2c8Pins) != 8 || i2c8Pins[7].Number != "8" || i2c8Pins[7].Offset.Y != kicadfiles.MM(7.62) {
		t.Fatalf("unexpected eight-pin generic I2C offsets: %#v ok=%v", i2c8Pins, ok)
	}
	externalUSBPins, ok := EmbeddedSymbolPinOffsets("Connector:USB_C_Receptacle_USB2.0")
	if !ok || len(externalUSBPins) != 11 || externalUSBPins[0].Number != "A5" {
		t.Fatalf("unexpected external USB-C offsets: %#v ok=%v", externalUSBPins, ok)
	}
	pins[0].Number = "BROKEN"
	freshPins, ok := EmbeddedSymbolPinOffsets("Device:R")
	if !ok || freshPins[0].Number != "1" {
		t.Fatalf("template pins share mutable backing data: %#v ok=%v", freshPins, ok)
	}
	powerPins, ok := EmbeddedSymbolPinOffsets("power:VCC")
	if !ok || len(powerPins) != 1 || powerPins[0].Number != "1" || powerPins[0].Offset.X != kicadfiles.MM(5.08) {
		t.Fatalf("unexpected power offsets: %#v ok=%v", powerPins, ok)
	}
	negativePowerPins, ok := EmbeddedSymbolPinOffsets("power:VSS")
	if !ok || len(negativePowerPins) != 1 || negativePowerPins[0].Number != "1" || negativePowerPins[0].Offset.X != kicadfiles.MM(-5.08) {
		t.Fatalf("unexpected negative power offsets: %#v ok=%v", negativePowerPins, ok)
	}
	powerFlagPins, ok := EmbeddedSymbolPinOffsets("power:PWR_FLAG")
	if !ok || len(powerFlagPins) != 1 || powerFlagPins[0].Number != "1" || powerFlagPins[0].Offset.X != 0 {
		t.Fatalf("unexpected PWR_FLAG offsets: %#v ok=%v", powerFlagPins, ok)
	}
	ams1117Pins, ok := EmbeddedSymbolPinOffsets("Regulator_Linear:AMS1117-3.3")
	if !ok || len(ams1117Pins) != 3 || ams1117Pins[0].Number != "1" || ams1117Pins[0].Offset.X != 0 || ams1117Pins[1].Number != "2" || ams1117Pins[1].Offset.X != kicadfiles.MM(7.62) || ams1117Pins[2].Number != "3" || ams1117Pins[2].Offset.X != kicadfiles.MM(-7.62) {
		t.Fatalf("unexpected AMS1117 offsets: %#v ok=%v", ams1117Pins, ok)
	}
	schematicAMS1117Pins, ok := EmbeddedSymbolPinOffsets("kicadai:ams1117_schematic")
	if !ok || len(schematicAMS1117Pins) != 3 || schematicAMS1117Pins[0].Offset.Y != kicadfiles.MM(-7.62) {
		t.Fatalf("unexpected schematic AMS1117 offsets: %#v ok=%v", schematicAMS1117Pins, ok)
	}
	if _, ok := EmbeddedSymbolPinOffsets("Custom:Block"); ok {
		t.Fatal("unexpected custom block template pins")
	}
}

func TestLocalSymbolLibraryRendersUnqualifiedSyntheticSymbol(t *testing.T) {
	contents, ok := LocalSymbolLibrary("Sensor:Generic_I2C")
	if !ok {
		t.Fatal("expected project-local generic I2C sensor library")
	}
	output := string(contents)
	if !strings.Contains(output, "(kicad_symbol_lib") || !strings.Contains(output, `"Generic_I2C"`) {
		t.Fatalf("local symbol library missing expected symbol body:\n%s", output)
	}
	if strings.Contains(output, `"Sensor:Generic_I2C"`) {
		t.Fatalf("local symbol library should use unqualified symbol name:\n%s", output)
	}
	if _, ok := LocalSymbolLibrary("power:GND"); ok {
		t.Fatal("power symbols should not produce local libraries")
	}
	regulatorContents, ok := LocalSymbolLibrary("Regulator_Linear:AMS1117-3.3")
	if !ok {
		t.Fatal("expected project-local AMS1117 symbol library")
	}
	if !strings.Contains(string(regulatorContents), `"AMS1117-3.3"`) || strings.Contains(string(regulatorContents), `"Regulator_Linear:AMS1117-3.3"`) {
		t.Fatalf("regulator local symbol library should use unqualified symbol name:\n%s", regulatorContents)
	}
	if _, ok := LocalSymbolLibrary("Custom:Missing"); ok {
		t.Fatal("unsupported symbols should not produce local libraries")
	}
	grouped, ok := LocalSymbolLibraryForIDs([]string{"Sensor:Generic_I2C", "Sensor:Generic_I2C", "Device:C"})
	if !ok {
		t.Fatal("expected grouped local symbol library")
	}
	if strings.Count(string(grouped), `"Generic_I2C"`) != 1 || strings.Contains(string(grouped), `"Device:C"`) {
		t.Fatalf("grouped local symbol library should deduplicate eligible symbols only:\n%s", grouped)
	}
}

func findTemplatePin(pins []TemplatePin, number string) (TemplatePin, bool) {
	for _, pin := range pins {
		if pin.Number == number {
			return pin, true
		}
	}
	return TemplatePin{}, false
}

func TestEmbeddedSymbolTemplateRendersTemplatePinOffsets(t *testing.T) {
	resistor, ok := EmbeddedSymbolTemplate("Device:R")
	if !ok {
		t.Fatal("Device:R template missing")
	}
	schematic := minimalSchematic()
	schematic.LibSymbols = []EmbeddedSymbol{resistor}
	var buf bytes.Buffer
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "(at 0 3.81 270)") || !strings.Contains(output, "(at 0 -3.81 90)") {
		t.Fatalf("resistor template did not render vertical KiCad pin anchors:\n%s", output)
	}

	led, ok := EmbeddedSymbolTemplate("Device:LED")
	if !ok {
		t.Fatal("Device:LED template missing")
	}
	schematic.LibSymbols = []EmbeddedSymbol{led}
	buf.Reset()
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output = buf.String()
	if !strings.Contains(output, "(at -3.81 0 0)") || !strings.Contains(output, "(at 3.81 0 180)") {
		t.Fatalf("LED template did not render KiCad pin anchors:\n%s", output)
	}

	connector, ok := EmbeddedSymbolTemplate("Connector_Generic:Conn_01x02")
	if !ok {
		t.Fatal("Connector_Generic:Conn_01x02 template missing")
	}
	schematic.LibSymbols = []EmbeddedSymbol{connector}
	buf.Reset()
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output = buf.String()
	if !strings.Contains(output, "(at -5.08 0 0)") || !strings.Contains(output, "(at -5.08 -2.54 0)") {
		t.Fatalf("connector template did not render KiCad pin anchors:\n%s", output)
	}

	connector4, ok := EmbeddedSymbolTemplate("Connector_Generic:Conn_01x04")
	if !ok {
		t.Fatal("Connector_Generic:Conn_01x04 template missing")
	}
	schematic.LibSymbols = []EmbeddedSymbol{connector4}
	buf.Reset()
	if err := Write(&buf, schematic); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	output = buf.String()
	if !strings.Contains(output, "(rectangle") || !strings.Contains(output, "(at -5.08 -5.08 0)") {
		t.Fatalf("4-pin connector template did not render body and pins:\n%s", output)
	}
}

func TestLEDIndicatorSchematicEmbedsCustomLibraryIDs(t *testing.T) {
	schematic, err := LEDIndicatorSchematic(LEDIndicatorInput{
		Name:            "custom_led",
		DesignID:        kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
		Seed:            "custom-led",
		LibraryLED:      "Custom:Indicator",
		LibraryResistor: "Custom:Sense_R",
		LibraryGND:      "Custom:Ground",
		LibraryVCC:      "Custom:Rail",
	})
	if err != nil {
		t.Fatalf("LEDIndicatorSchematic returned error: %v", err)
	}
	if len(schematic.LibSymbols) != 4 {
		t.Fatalf("lib symbols = %d, want 4", len(schematic.LibSymbols))
	}
	for _, libraryID := range []string{"Custom:Indicator", "Custom:Sense_R", "Custom:Ground", "Custom:Rail"} {
		if !embeddedSymbolPresent(schematic.LibSymbols, libraryID) {
			t.Fatalf("missing embedded custom library %s: %#v", libraryID, schematic.LibSymbols)
		}
	}
}

func embeddedSymbolPresent(symbols []EmbeddedSymbol, libraryID string) bool {
	for _, symbol := range symbols {
		if symbol.LibraryID == libraryID && len(symbol.Body) > 0 {
			return true
		}
	}
	return false
}
