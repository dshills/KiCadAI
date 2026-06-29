package schematic

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestEmbeddedSymbolTemplateRendersSupportedSeedSymbols(t *testing.T) {
	tests := []string{
		"Device:R",
		"Device:C",
		"Device:D",
		"Device:LED",
		"power:GND",
		"power:VCC",
		"power:+3.3V",
		"power:+3V3",
		"power:+5V",
		"power:+12V",
		"power:-12V",
		"power:PWR_FLAG",
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

func TestEmbeddedSymbolPinOffsets(t *testing.T) {
	pins, ok := EmbeddedSymbolPinOffsets("Device:R")
	if !ok || len(pins) != 2 {
		t.Fatalf("Device:R pins = %#v ok=%v, want two pins", pins, ok)
	}
	if pins[0].Number != "1" || pins[0].Offset.X != kicadfiles.MM(-5.08) || pins[1].Number != "2" || pins[1].Offset.X != kicadfiles.MM(5.08) {
		t.Fatalf("unexpected two-pin offsets: %#v", pins)
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
	if _, ok := EmbeddedSymbolPinOffsets("Custom:Block"); ok {
		t.Fatal("unexpected custom block template pins")
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
