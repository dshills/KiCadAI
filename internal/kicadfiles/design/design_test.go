package design

import (
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/library"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
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

func TestValidateAcceptsKiCadHierarchicalPCBFootprintPath(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].Path = "/11111111-1111-4111-8111-111111111111/22222222-2222-4222-8222-222222222222"

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
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

func TestValidateRejectsUnresolvedChildSheetSymbolLibrary(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ac1"),
		LibraryID: "Missing:R",
		Reference: "R99",
		Value:     "1k",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected unresolved child symbol library")
	}
	if !strings.Contains(err.Error(), "sheet_files[0].symbols[0].library_id") || !strings.Contains(err.Error(), "unresolved library Missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAllowsChildSheetEmbeddedSymbolLibrary(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.LibSymbols = []schematic.EmbeddedSymbol{{LibraryID: "Local:R"}}
	child.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ac1"),
		LibraryID: "Local:R",
		Reference: "R99",
		Value:     "1k",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateAllowsChildSheetKnownAndTableSymbolLibraries(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{
		{
			UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ac1"),
			LibraryID: "Device:R",
			Reference: "R99",
			Value:     "1k",
		},
		{
			UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ac2"),
			LibraryID: "local_symbols:Thing",
			Reference: "U99",
			Value:     "Thing",
		},
	}
	design.SheetFiles = []*schematic.SchematicFile{&child}
	design.KnownSymbolLibraries = []string{"Device"}
	design.SymbolTables = []library.TableEntry{{
		Name: "local_symbols",
		Type: "KiCad",
		URI:  "${KIPRJMOD}/local_symbols.kicad_sym",
	}}

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
	design.Schematic.Symbols[2].Properties = []schematic.Property{{Name: "Footprint", Value: "local_footprints:R_0603"}}
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

func TestApplyLibraryMappingAssignsSymbolFootprintsAndTables(t *testing.T) {
	design := validLEDDesign(t)
	if err := ApplyLibraryMapping(&design, LibraryMapping{
		SymbolFootprints: []SymbolFootprintAssignment{
			{SymbolLibraryID: "Device:R", ReferencePrefix: "R", FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric"},
			{SymbolLibraryID: "Device:LED", ReferencePrefix: "D", FootprintLibraryID: "LED_SMD:LED_0805_2012Metric"},
		},
		SymbolTables:            []library.TableEntry{{Name: "local_symbols", Type: "KiCad", URI: "${KIPRJMOD}/lib/local_symbols.kicad_sym"}},
		FootprintTables:         []library.TableEntry{{Name: "local_footprints", Type: "KiCad", URI: "${KIPRJMOD}/footprints.pretty"}},
		KnownSymbolLibraries:    []string{"Device"},
		KnownFootprintLibraries: []string{"Resistor_SMD", "LED_SMD"},
	}); err != nil {
		t.Fatalf("ApplyLibraryMapping returned error: %v", err)
	}

	assertSymbolFootprint(t, design, "R1", "Resistor_SMD:R_0805_2012Metric")
	assertSymbolFootprint(t, design, "D1", "LED_SMD:LED_0805_2012Metric")
	if len(design.SymbolTables) != 1 || design.SymbolTables[0].Name != "local_symbols" {
		t.Fatalf("symbol tables = %+v", design.SymbolTables)
	}
	if len(design.FootprintTables) != 1 || design.FootprintTables[0].Name != "local_footprints" {
		t.Fatalf("footprint tables = %+v", design.FootprintTables)
	}
	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestApplyLibraryMappingRejectsConflictingSymbolFootprint(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = []schematic.Property{{Name: "Footprint", Value: "Wrong:Part"}}

	err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{{
		SymbolLibraryID:    "Device:R",
		ReferencePrefix:    "R",
		FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric",
	}}})
	if err == nil {
		t.Fatal("expected conflict")
	}
	if !strings.Contains(err.Error(), "conflicts with mapped footprint") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyLibraryMappingUpdatesFootprintPropertyAndField(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = []schematic.Property{{Name: "Footprint"}}
	design.Schematic.Symbols[1].Fields = []schematic.Field{{Name: "Footprint"}}

	err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{{
		SymbolLibraryID:    "Device:R",
		ReferencePrefix:    "R",
		FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric",
	}}})
	if err != nil {
		t.Fatalf("ApplyLibraryMapping returned error: %v", err)
	}
	if got := design.Schematic.Symbols[1].Properties[0].Value; got != "Resistor_SMD:R_0805_2012Metric" {
		t.Fatalf("property footprint = %q", got)
	}
	if got := design.Schematic.Symbols[1].Fields[0].Value; got != "Resistor_SMD:R_0805_2012Metric" {
		t.Fatalf("field footprint = %q", got)
	}
}

func TestCloneSchematicFileAccountsForEveryField(t *testing.T) {
	accounted := map[string]struct{}{
		"Filename":         {},
		"Version":          {},
		"Generator":        {},
		"GeneratorVersion": {},
		"UUID":             {},
		"Paper":            {},
		"TitleBlock":       {},
		"LibSymbols":       {},
		"Symbols":          {},
		"Wires":            {},
		"NoConnects":       {},
		"Labels":           {},
		"Junctions":        {},
		"Buses":            {},
		"Polylines":        {},
		"BusEntries":       {},
		"Texts":            {},
		"Sheets":           {},
		"RawItems":         {},
		"Instances":        {},
		"SheetInstances":   {},
	}
	schematicType := reflect.TypeOf(schematic.SchematicFile{})
	for i := 0; i < schematicType.NumField(); i++ {
		fieldName := schematicType.Field(i).Name
		if _, ok := accounted[fieldName]; !ok {
			t.Fatalf("cloneSchematicFile needs an explicit clone policy for SchematicFile.%s", fieldName)
		}
		delete(accounted, fieldName)
	}
	for fieldName := range accounted {
		t.Fatalf("cloneSchematicFile accounts for removed SchematicFile.%s", fieldName)
	}
}

func TestClonePCBFootprintAccountsForEveryField(t *testing.T) {
	accounted := map[string]struct{}{
		"Raw":                           {},
		"UUID":                          {},
		"Path":                          {},
		"LibraryID":                     {},
		"Reference":                     {},
		"Value":                         {},
		"Description":                   {},
		"Tags":                          {},
		"SheetName":                     {},
		"SheetFile":                     {},
		"Attributes":                    {},
		"Position":                      {},
		"Rotation":                      {},
		"Layer":                         {},
		"Locked":                        {},
		"Properties":                    {},
		"MetadataProperties":            {},
		"Units":                         {},
		"NetTiePadGroups":               {},
		"Texts":                         {},
		"Pads":                          {},
		"Graphics":                      {},
		"Models":                        {},
		"EmbeddedFonts":                 {},
		"DuplicatePadNumbersAreJumpers": {},
	}
	footprintType := reflect.TypeOf(pcb.Footprint{})
	for i := 0; i < footprintType.NumField(); i++ {
		fieldName := footprintType.Field(i).Name
		if _, ok := accounted[fieldName]; !ok {
			t.Fatalf("clonePCBFootprint needs an explicit clone policy for Footprint.%s", fieldName)
		}
		delete(accounted, fieldName)
	}
	for fieldName := range accounted {
		t.Fatalf("clonePCBFootprint accounts for removed Footprint.%s", fieldName)
	}
}

func TestApplyLibraryMappingLeavesDesignUnchangedOnPCBConflict(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = nil
	design.PCB.Footprints[1].LibraryID = "Other:R_0805"

	err := ApplyLibraryMapping(&design, LibraryMapping{
		SymbolFootprints: []SymbolFootprintAssignment{{
			SymbolLibraryID:    "Device:R",
			ReferencePrefix:    "R",
			FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric",
		}},
		FootprintTables: []library.TableEntry{{Name: "local_footprints", Type: "KiCad", URI: "${KIPRJMOD}/footprints.pretty"}},
	})
	if err == nil {
		t.Fatal("expected PCB conflict")
	}
	if _, ok := schematicFootprintProperty(&design.Schematic.Symbols[1]); ok {
		t.Fatalf("symbol footprint was mutated after failed mapping: %+v", design.Schematic.Symbols[1])
	}
	if len(design.FootprintTables) != 0 {
		t.Fatalf("footprint tables mutated after failed mapping: %+v", design.FootprintTables)
	}
}

func TestApplyLibraryMappingRejectsInconsistentReferenceMapping(t *testing.T) {
	design := validLEDDesign(t)
	secondUnit := design.Schematic.Symbols[1]
	secondUnit.UUID = kicadfiles.UUID("99999999-9999-4999-8999-999999999999")
	secondUnit.LibraryID = "Device:R_Pack"
	secondUnit.Properties = nil
	design.Schematic.Symbols = append(design.Schematic.Symbols, secondUnit)

	err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{
		{SymbolLibraryID: "Device:R", ReferencePrefix: "R", FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric"},
		{SymbolLibraryID: "Device:R_Pack", ReferencePrefix: "R", FootprintLibraryID: "Resistor_SMD:R_Array"},
	}})
	if err == nil {
		t.Fatal("expected inconsistent mapping")
	}
	if !strings.Contains(err.Error(), "maps to both") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyLibraryMappingReferencePrefixRequiresNumericBoundary(t *testing.T) {
	design := validLEDDesign(t)
	varistor := design.Schematic.Symbols[1]
	varistor.Reference = "RV1"
	varistor.LibraryID = "Device:Varistor"
	varistor.Properties = nil
	design.Schematic.Symbols = append(design.Schematic.Symbols, varistor)

	err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{{
		ReferencePrefix:    "R",
		FootprintLibraryID: "Resistor_SMD:R_0805_2012Metric",
	}}})
	if err != nil {
		t.Fatalf("ApplyLibraryMapping returned error: %v", err)
	}
	if _, ok := schematicFootprintProperty(&design.Schematic.Symbols[len(design.Schematic.Symbols)-1]); ok {
		t.Fatalf("RV reference was incorrectly matched by R prefix: %+v", design.Schematic.Symbols[len(design.Schematic.Symbols)-1])
	}
}

func TestValidateRejectsSchematicPCBFootprintMismatch(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = []schematic.Property{{Name: "Footprint", Value: "Wrong:Part"}}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected mismatch")
	}
	if !strings.Contains(err.Error(), "properties.Footprint") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateMatchesSchematicPCBFootprintsCaseInsensitively(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Reference = "r1"
	design.Schematic.Symbols[1].Properties = []schematic.Property{{Name: "Footprint", Value: "resistor_smd:r_0805_2012metric"}}

	err := Validate(design)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRequiresSchematicFootprintForPCBBackedSymbol(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = nil

	err := Validate(design)
	if err == nil {
		t.Fatal("expected missing schematic footprint assignment")
	}
	if !strings.Contains(err.Error(), "properties.Footprint") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAllowsOffBoardSymbolWithoutPCBFootprint(t *testing.T) {
	design := validLEDDesign(t)
	onBoard := false
	design.Schematic.Symbols[1].OnBoard = &onBoard
	design.PCB.Footprints = append(design.PCB.Footprints[:1], design.PCB.Footprints[2:]...)

	err := Validate(design)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateAcceptsPCBFootprintForChildSheetSymbol(t *testing.T) {
	design := validLEDDesign(t)
	childSymbol := design.Schematic.Symbols[1]
	childSymbol.Path = design.PCB.Footprints[1].Path
	design.Schematic.Symbols = append(design.Schematic.Symbols[:1], design.Schematic.Symbols[2:]...)
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{childSymbol}
	design.SheetFiles = []*schematic.SchematicFile{&child}
	design.KnownSymbolLibraries = []string{"Device"}

	err := Validate(design)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsDuplicateSchematicReferenceAcrossSheets(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{design.Schematic.Symbols[1]}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected duplicate schematic reference")
	}
	if !strings.Contains(err.Error(), "duplicate schematic reference R1") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSchematicReferenceAcrossSheetsWithoutPCB(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.KnownSymbolLibraries = []string{"Device"}
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0"),
		LibraryID: "Device:R",
		Reference: " r1 ",
		Value:     "1k",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected duplicate schematic reference")
	}
	if !strings.Contains(err.Error(), "sheet_files[0].symbols[0].reference") ||
		!strings.Contains(err.Error(), "duplicate schematic reference r1") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSchematicReferenceAcrossChildSheets(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.KnownSymbolLibraries = []string{"Device"}
	design.Schematic.Sheets = []schematic.Sheet{
		{
			UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
			Name:     "Input",
			Filename: "input.kicad_sch",
			Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
		{
			UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abe"),
			Name:     "Output",
			Filename: "output.kicad_sch",
			Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}
	childA := minimalChildSheet("input.kicad_sch")
	childA.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0"),
		LibraryID: "Device:R",
		Reference: "U10",
		Value:     "1k",
	}}
	childB := minimalChildSheet("output.kicad_sch")
	childB.UUID = kicadfiles.UUID("12345678-1234-5678-9234-123456789ad1")
	childB.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ad2"),
		LibraryID: "Device:C",
		Reference: "u10",
		Value:     "100n",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&childA, &childB}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected duplicate schematic reference")
	}
	if !strings.Contains(err.Error(), "sheet_files[1].symbols[0].reference") ||
		!strings.Contains(err.Error(), "duplicate schematic reference u10") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAllowsDuplicatePowerSymbolReferencesAcrossSheets(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.KnownSymbolLibraries = []string{"power"}
	design.Schematic.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0"),
		LibraryID: "power:GND",
		Reference: "#PWR0101",
		Value:     "GND",
	}}
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Child",
		Filename: "child.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("child.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{{
		UUID:      kicadfiles.UUID("12345678-1234-5678-9234-123456789ad1"),
		LibraryID: "power:GND",
		Reference: "#PWR0101",
		Value:     "GND",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	if err := Validate(design); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateChecksEveryDuplicatePCBFootprintAssignment(t *testing.T) {
	design := validLEDDesign(t)
	design.Schematic.Symbols[1].Properties = []schematic.Property{{Name: "Footprint", Value: "Resistor_SMD:R_0805_2012Metric"}}
	duplicate := design.PCB.Footprints[1]
	duplicate.LibraryID = "Wrong:Part"
	design.PCB.Footprints = append(design.PCB.Footprints, duplicate)

	err := Validate(design)
	if err == nil {
		t.Fatal("expected duplicate footprint mismatch")
	}
	if !strings.Contains(err.Error(), "must match PCB footprint library Wrong:Part") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyLibraryMappingRejectsInvalidAssignment(t *testing.T) {
	design := validLEDDesign(t)
	err := ApplyLibraryMapping(&design, LibraryMapping{SymbolFootprints: []SymbolFootprintAssignment{{
		ReferencePrefix:    "R",
		FootprintLibraryID: "missing_colon",
	}}})
	if err == nil {
		t.Fatal("expected invalid assignment")
	}
	if !strings.Contains(err.Error(), "footprint_library_id") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsMissingChildSheetFile(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing child schematic power.kicad_sch") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateChildSheetFilename(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{
		{
			UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
			Name:     "Power",
			Filename: "power.kicad_sch",
			Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		},
	}
	child1 := minimalChildSheet("power.kicad_sch")
	child2 := minimalChildSheet("power.kicad_sch")
	child2.UUID = kicadfiles.UUID("12345678-1234-5678-9234-123456789abf")
	design.SheetFiles = []*schematic.SchematicFile{&child1, &child2}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate power.kicad_sch") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateUUIDInsideChildSheet(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	child := minimalChildSheet("power.kicad_sch")
	child.Symbols = []schematic.SchematicSymbol{{
		UUID:      design.Schematic.Symbols[0].UUID,
		LibraryID: "Device:R",
		Reference: "R99",
		Value:     "1k",
	}}
	design.SheetFiles = []*schematic.SchematicFile{&child}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSymbolPinUUID(t *testing.T) {
	design := validLEDDesign(t)
	duplicate := kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0")
	design.Schematic.Symbols[1].Pins = []schematic.SymbolPin{
		{Number: "1", UUID: duplicate},
		{Number: "2", UUID: duplicate},
	}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.symbols[1].pins[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateSheetPinUUID(t *testing.T) {
	design := validLEDDesign(t)
	duplicate := kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0")
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "Power",
		Filename: "power.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
		Pins: []schematic.SheetPin{
			{UUID: duplicate, Text: "VIN", Kind: schematic.SheetPinInput},
			{UUID: duplicate, Text: "VOUT", Kind: schematic.SheetPinOutput},
		},
	}}
	child := minimalChildSheet("power.kicad_sch")
	design.SheetFiles = []*schematic.SchematicFile{&child}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.sheets[0].pins[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateRawSchematicItemUUID(t *testing.T) {
	design := validLEDDesign(t)
	duplicate := kicadfiles.UUID("33333333-3333-4333-8333-333333333333")
	design.Schematic.RawItems = []schematic.RawSchematicItem{
		{
			UUID: duplicate,
			Body: sexpr.Raw(`(rule_area (name "Keepout A") (uuid "33333333-3333-4333-8333-333333333333"))`),
		},
		{
			UUID: duplicate,
			Body: sexpr.Raw(`(rule_area (name "Keepout B") (uuid "33333333-3333-4333-8333-333333333333"))`),
		},
	}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.raw_items[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateNoConnectUUID(t *testing.T) {
	design := validLEDDesign(t)
	duplicate := kicadfiles.UUID("12345678-1234-5678-9234-123456789ad0")
	design.Schematic.NoConnects = []schematic.NoConnect{
		{UUID: duplicate, Position: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(10)}},
		{UUID: duplicate, Position: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}},
	}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "schematic.no_connects[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicatePCBPadUUID(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].Pads[1].UUID = design.PCB.Footprints[0].Pads[0].UUID

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pcb.footprints[0].pads[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsDuplicateFootprintPropertyUUID(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB.Footprints[0].Properties[1].UUID = design.PCB.Footprints[0].Properties[0].UUID

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pcb.footprints[0].properties[1].uuid") || !strings.Contains(err.Error(), "duplicate UUID") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsCircularChildSheetReferences(t *testing.T) {
	design := validLEDDesign(t)
	design.PCB = nil
	design.ExpectedNets = nil
	design.Schematic.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abd"),
		Name:     "A",
		Filename: "a.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	childA := minimalChildSheet("a.kicad_sch")
	childA.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789abf"),
		Name:     "B",
		Filename: "b.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	childB := minimalChildSheet("b.kicad_sch")
	childB.UUID = kicadfiles.UUID("12345678-1234-5678-9234-123456789ac0")
	childB.Sheets = []schematic.Sheet{{
		UUID:     kicadfiles.UUID("12345678-1234-5678-9234-123456789ac1"),
		Name:     "AAgain",
		Filename: "a.kicad_sch",
		Size:     kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)},
	}}
	design.SheetFiles = []*schematic.SchematicFile{&childA, &childB}

	err := Validate(design)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "circular sheet reference") {
		t.Fatalf("error = %v", err)
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

func assertSymbolFootprint(t *testing.T, design Design, reference, want string) {
	t.Helper()
	for i := range design.Schematic.Symbols {
		symbol := &design.Schematic.Symbols[i]
		if symbol.Reference != reference {
			continue
		}
		got, ok := schematicFootprintProperty(symbol)
		if !ok {
			t.Fatalf("symbol %s missing Footprint property", reference)
		}
		if got != want {
			t.Fatalf("symbol %s footprint = %q, want %q", reference, got, want)
		}
		return
	}
	t.Fatalf("symbol %s not found", reference)
}
