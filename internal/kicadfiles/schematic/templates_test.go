package schematic

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestEmbeddedSymbolTemplateRendersSupportedSeedSymbols(t *testing.T) {
	tests := []string{"Device:R", "Device:C", "Device:D", "Device:LED", "power:GND", "power:VCC"}
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
