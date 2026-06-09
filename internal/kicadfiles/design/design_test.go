package design

import (
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/library"
)

func TestLEDIndicatorDesignValidates(t *testing.T) {
	design, err := LEDIndicatorDesign(LEDIndicatorInput{
		Name:       "led_indicator",
		DesignID:   kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:       "phase-9",
		IncludePCB: true,
	})
	if err != nil {
		t.Fatalf("LEDIndicatorDesign returned error: %v", err)
	}
	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if design.Schematic == nil || design.PCB == nil {
		t.Fatal("LED design missing schematic or PCB")
	}
}

func TestValidateRejectsMissingPCBNet(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Nets = design.PCB.Nets[:2]

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing expected net GND") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateFootprintReference(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[1].Reference = design.PCB.Footprints[0].Reference

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSchematicReference(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[2].Reference = design.Schematic.Symbols[1].Reference

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.symbols[2].reference") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSchematicReferenceWithoutPCB(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.Schematic.Symbols[2].Reference = design.Schematic.Symbols[1].Reference

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.symbols[2].reference") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsOrphanPCBFootprint(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].Reference = "U99"

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing schematic symbol") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsFootprintSymbolPathMismatch(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].Path = ""

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must match schematic symbol path") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsProjectSchematicNameMismatch(t *testing.T) {
	design := validLEDDesign(t)
	design.Project.Name = "other"

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project.name") {
		t.Fatalf("error = %v", err)
	}

	design = validLEDDesign(t)
	design.Schematic.Filename = "other.kicad_sch"
	err = Validate(design)
	if err == nil {
		t.Fatal("expected schematic filename error")
	}
	if !strings.Contains(err.Error(), "schematic.filename") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsUnresolvedSymbolLibrary(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.LibSymbols = nil
	design.Schematic.Symbols[1].LibraryID = "Missing:R"

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unresolved library Missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAllowsSymbolLibraryTableReference(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.LibSymbols = nil
	design.Schematic.Symbols = design.Schematic.Symbols[:1]
	design.Schematic.Symbols[0].LibraryID = "local_symbols:Thing"
	design.SymbolTables = []library.TableEntry{{
		Name: "local_symbols",
		Type: "KiCad",
		URI:  "${KIPRJMOD}/local_symbols.kicad_sym",
	}}
	design.PCB = nil
	design.ExpectedNets = nil

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateAllowsKnownExternalSymbolLibrary(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.LibSymbols = nil
	design.Schematic.Symbols = design.Schematic.Symbols[:1]
	design.Schematic.Symbols[0].LibraryID = "Device:R"
	design.KnownSymbolLibraries = []string{"Device"}
	design.PCB = nil
	design.ExpectedNets = nil

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateTrimsLibraryIDNickname(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.LibSymbols = nil
	design.Schematic.Symbols = design.Schematic.Symbols[:1]
	design.Schematic.Symbols[0].LibraryID = " Device : R "
	design.KnownSymbolLibraries = []string{"Device"}
	design.PCB = nil
	design.ExpectedNets = nil

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsUnresolvedNonInlineFootprintLibrary(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].LibraryID = "Missing:R_0603"
	design.PCB.Footprints[0].Pads = nil
	design.PCB.Footprints[0].Graphics = nil

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unresolved library Missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAllowsFootprintLibraryTableReference(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].LibraryID = "local_footprints:R_0603"
	design.PCB.Footprints[0].Pads = nil
	design.PCB.Footprints[0].Graphics = nil
	design.FootprintTables = []library.TableEntry{{
		Name: "local_footprints",
		Type: "KiCad",
		URI:  "${KIPRJMOD}/footprints.pretty",
	}}

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func validLEDDesign(t *testing.T) Design {
	t.Helper()
	design, err := LEDIndicatorDesign(LEDIndicatorInput{
		Name:       "led_indicator",
		DesignID:   kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed:       "phase-9",
		IncludePCB: true,
	})
	if err != nil {
		t.Fatalf("LEDIndicatorDesign returned error: %v", err)
	}
	return design
}
