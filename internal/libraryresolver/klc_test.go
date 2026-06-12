package libraryresolver

import (
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
)

func TestValidateSymbolKLCMissingMetadataWarning(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{"Device:R": {LibraryID: "Device:R", Pins: []SymbolPin{{Number: "1"}}}}}
	report := ValidateSymbolKLC(index, "Device:R")
	if len(report.Issues) == 0 || report.Issues[0].Severity != reports.SeverityWarning {
		t.Fatalf("expected metadata warning: %#v", report)
	}
}

func TestValidateFootprintKLCMissingPadNameError(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:Bad": {FootprintID: "Test:Bad", Pads: []FootprintPad{{Type: "smd", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}}}}}}
	report := ValidateFootprintKLC(index, "Test:Bad")
	if !hasKLCIssue(report, reports.SeverityError, "electrical pad without a name") {
		t.Fatalf("expected missing pad name error: %#v", report)
	}
}

func TestValidateSymbolKLCDuplicatePinNumberError(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{"Device:Stacked": {LibraryID: "Device:Stacked", Description: "Stacked", Keywords: []string{"stacked"}, Pins: []SymbolPin{{Number: "1", Name: "A"}, {Number: "1", Name: "B"}}}}}
	report := ValidateSymbolKLC(index, "Device:Stacked")
	if !hasKLCIssue(report, reports.SeverityError, "duplicate pin number") {
		t.Fatalf("expected duplicate pin error: %#v", report)
	}
}

func TestValidateSymbolKLCRejectsHiddenFlattenedDuplicatePins(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{"Device:HiddenStack": {LibraryID: "Device:HiddenStack", Description: "Stacked", Keywords: []string{"stacked"}, Pins: []SymbolPin{{Number: "1", Name: "A"}, {Number: "1", Name: "ALT", Hidden: true}}}}}
	report := ValidateSymbolKLC(index, "Device:HiddenStack")
	if !hasKLCIssue(report, reports.SeverityError, "duplicate pin number") {
		t.Fatalf("expected hidden flattened duplicate pin conflict: %#v", report)
	}
}

func TestValidateSymbolKLCRejectsMultipleVisibleStackedPinNames(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{"Device:HiddenFirst": {LibraryID: "Device:HiddenFirst", Description: "Stacked", Keywords: []string{"stacked"}, Pins: []SymbolPin{{Number: "1", Name: "ALT", Hidden: true}, {Number: "1", Name: "A"}, {Number: "1", Name: "B"}}}}}
	report := ValidateSymbolKLC(index, "Device:HiddenFirst")
	if !hasKLCIssue(report, reports.SeverityError, "duplicate pin number") {
		t.Fatalf("expected visible stacked pin conflict: %#v", report)
	}
}

func TestValidateSymbolKLCRejectsMultipleVisibleSameNameStackedPins(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{"Device:VisibleStack": {LibraryID: "Device:VisibleStack", Description: "Stacked", Keywords: []string{"stacked"}, Pins: []SymbolPin{{Number: "1", Name: "VCC"}, {Number: "1", Name: "VCC"}}}}}
	report := ValidateSymbolKLC(index, "Device:VisibleStack")
	if !hasKLCIssue(report, reports.SeverityError, "duplicate pin number") {
		t.Fatalf("expected visible stacked same-name conflict: %#v", report)
	}
}

func TestValidateFootprintKLCAllowsDuplicateElectricalPadNames(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:Dup": {FootprintID: "Test:Dup", Description: "Duplicate pads", Tags: []string{"dup"}, Attributes: []string{"smd"}, GraphicsSummary: GraphicsSummary{HasCourtyard: true, HasFabOutline: true, HasSilk: true}, Pads: []FootprintPad{
		{Name: "1", Type: "smd", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
		{Name: "1", Type: "smd", Position: kicadfiles.Point{X: kicadfiles.MM(1)}, Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:Dup")
	for _, issue := range report.Issues {
		if issue.Blocking() {
			t.Fatalf("duplicate electrical pad names should be allowed: %#v", report)
		}
	}
}

func TestValidateFootprintKLCAllowsEmptyNPTHPadName(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:Mount": {FootprintID: "Test:Mount", Description: "Mount", Tags: []string{"mount"}, Attributes: []string{"through_hole"}, GraphicsSummary: GraphicsSummary{HasCourtyard: true, HasFabOutline: true, HasSilk: true}, Pads: []FootprintPad{
		{Type: "np_thru_hole", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:Mount")
	for _, issue := range report.Issues {
		if issue.Blocking() {
			t.Fatalf("empty NPTH pad names should be allowed: %#v", report)
		}
	}
}

func TestValidateFootprintKLCAllowsNonElectricalPadWithoutCopper(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:Mechanical": {FootprintID: "Test:Mechanical", Description: "Mechanical", Tags: []string{"mount"}, Attributes: []string{"exclude_from_pos_files"}, GraphicsSummary: GraphicsSummary{HasCourtyard: true, HasFabOutline: true, HasSilk: true}, Pads: []FootprintPad{
		{Type: "mechanical", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:Mechanical")
	for _, issue := range report.Issues {
		if issue.Blocking() {
			t.Fatalf("non-electrical pad without copper should be allowed: %#v", report)
		}
	}
}

func TestValidateFootprintKLCThroughHolePadRequiresCopper(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:NoCu": {FootprintID: "Test:NoCu", Pads: []FootprintPad{
		{Name: "1", Type: "thru_hole", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:NoCu")
	if !hasKLCIssue(report, reports.SeverityError, "does not include a copper layer") {
		t.Fatalf("expected copper layer error: %#v", report)
	}
}

func TestValidateFootprintKLCUnnamedElectricalPadStillChecksCopper(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:NoNameNoCu": {FootprintID: "Test:NoNameNoCu", Pads: []FootprintPad{
		{Type: "smd", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:NoNameNoCu")
	if !hasKLCIssue(report, reports.SeverityError, "electrical pad without a name") || !hasKLCIssue(report, reports.SeverityError, "electrical pad does not include a copper layer") {
		t.Fatalf("expected missing name and copper errors: %#v", report)
	}
}

func TestValidateFootprintKLCNPTHPadRejectsCopper(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:BadMount": {FootprintID: "Test:BadMount", Pads: []FootprintPad{
		{Type: "NP_Thru_Hole", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:BadMount")
	if !hasKLCIssue(report, reports.SeverityError, "non-electrical pad must not include a copper layer") {
		t.Fatalf("expected NPTH copper layer error: %#v", report)
	}
}

func TestValidateFootprintKLCMechanicalPadRejectsCopper(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:BadMechanical": {FootprintID: "Test:BadMechanical", Pads: []FootprintPad{
		{Type: "mechanical", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerAllMask}},
	}}}}
	report := ValidateFootprintKLC(index, "Test:BadMechanical")
	if !hasKLCIssue(report, reports.SeverityError, "must not include a copper layer") {
		t.Fatalf("expected mechanical copper layer error: %#v", report)
	}
}

func TestValidateFootprintKLCMissingCourtyardWarning(t *testing.T) {
	index := LibraryIndex{Footprints: map[string]FootprintRecord{"Test:NoCourt": validKLCFootprint("Test:NoCourt")}}
	report := ValidateFootprintKLC(index, "Test:NoCourt")
	if !hasKLCIssue(report, reports.SeverityWarning, "no courtyard") {
		t.Fatalf("expected missing courtyard warning: %#v", report)
	}
}

func TestValidateFootprintKLCValidHasNoBlockingIssues(t *testing.T) {
	record := validKLCFootprint("Test:Good")
	record.GraphicsSummary.HasCourtyard = true
	index := LibraryIndex{Footprints: map[string]FootprintRecord{record.FootprintID: record}}
	report := ValidateFootprintKLC(index, record.FootprintID)
	for _, issue := range report.Issues {
		if issue.Blocking() {
			t.Fatalf("unexpected blocking issue: %#v", report)
		}
	}
}

func validKLCFootprint(id string) FootprintRecord {
	return FootprintRecord{
		FootprintID: id,
		Description: "Valid test footprint",
		Tags:        []string{"test"},
		Attributes:  []string{"smd"},
		Pads:        []FootprintPad{{Name: "1", Type: "smd", Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask}}},
		GraphicsSummary: GraphicsSummary{
			HasFabOutline: true,
			HasSilk:       true,
		},
	}
}

func hasKLCIssue(report KLCReport, severity reports.Severity, contains string) bool {
	for _, issue := range report.Issues {
		if issue.Severity == severity && strings.Contains(issue.Message, contains) {
			return true
		}
	}
	return false
}
