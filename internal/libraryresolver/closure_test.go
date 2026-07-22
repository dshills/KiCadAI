package libraryresolver

import (
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestResolveDesignClosureIncludesInheritedBasesAndSelectionIdentity(t *testing.T) {
	index := LibraryIndex{
		Roots: LibraryRoots{SymbolsRoot: "/symbols", FootprintsRoot: "/footprints"},
		Symbols: map[string]SymbolRecord{
			"Amplifier:Base":  {LibraryID: "Amplifier:Base", LibraryNickname: "Amplifier", Path: "/symbols/Amplifier.kicad_sym"},
			"Amplifier:Child": {LibraryID: "Amplifier:Child", LibraryNickname: "Amplifier", Path: "/symbols/Amplifier.kicad_sym", Extends: "Base", Inherited: true},
		},
		Footprints: map[string]FootprintRecord{
			"Package:SOT": {FootprintID: "Package:SOT", Path: "/footprints/Package.pretty/SOT.kicad_mod"},
		},
	}
	request := ClosureRequest{
		Symbols:    []SymbolReference{{LibraryID: "Amplifier:Child", Units: []int{2, 1, 2}, Pins: []string{"3", "1", "3"}}},
		Footprints: []FootprintReference{{LibraryID: "Package:SOT", Pads: []string{"3", "1"}}},
		Variants:   []VariantReference{{ComponentID: "opamp", VariantID: "sot", FootprintID: "Package:SOT"}},
	}
	closure, issues := ResolveDesignClosure(index, request)
	if len(issues) != 0 || closure.Identity == "" {
		t.Fatalf("closure=%#v issues=%#v", closure, issues)
	}
	if got := []string{closure.Symbols[0].LibraryID, closure.Symbols[1].LibraryID}; !reflect.DeepEqual(got, []string{"Amplifier:Base", "Amplifier:Child"}) {
		t.Fatalf("symbols=%#v", closure.Symbols)
	}
	if !reflect.DeepEqual(closure.Symbols[1].Units, []int{1, 2}) || !reflect.DeepEqual(closure.Symbols[1].Pins, []string{"1", "3"}) {
		t.Fatalf("selected symbol detail=%#v", closure.Symbols[1])
	}
	reordered, reorderedIssues := ResolveDesignClosure(index, ClosureRequest{
		Symbols:    []SymbolReference{{LibraryID: "Amplifier:Child", Pins: []string{"1", "3"}, Units: []int{1, 2}}},
		Footprints: []FootprintReference{{LibraryID: "Package:SOT", Pads: []string{"1", "3"}}},
		Variants:   request.Variants,
	})
	if len(reorderedIssues) != 0 || reordered.Identity != closure.Identity || !reflect.DeepEqual(reordered, closure) {
		t.Fatalf("closure is not stable: first=%#v second=%#v issues=%#v", closure, reordered, reorderedIssues)
	}
	changedSelection := request
	changedSelection.Symbols = []SymbolReference{{LibraryID: "Amplifier:Child", Units: []int{1}, Pins: []string{"1"}}}
	changed, _ := ResolveDesignClosure(index, changedSelection)
	if changed.Identity == closure.Identity {
		t.Fatal("closure identity did not include selected units and pins")
	}
	changedRoots := index
	changedRoots.Roots.SymbolsRoot = "/other-symbols"
	otherRoot, _ := ResolveDesignClosure(changedRoots, request)
	if otherRoot.Identity == closure.Identity {
		t.Fatal("closure identity did not include library roots")
	}
}

func TestResolveDesignClosureFailsForMissingObjectOrInheritedBase(t *testing.T) {
	index := LibraryIndex{Symbols: map[string]SymbolRecord{
		"Amplifier:Child": {LibraryID: "Amplifier:Child", LibraryNickname: "Amplifier", Path: "/symbols/Amplifier.kicad_sym", Extends: "Missing"},
	}, Footprints: map[string]FootprintRecord{}}
	_, issues := ResolveDesignClosure(index, ClosureRequest{
		Symbols:    []SymbolReference{{LibraryID: "Amplifier:Child"}},
		Footprints: []FootprintReference{{LibraryID: "Package:Missing"}},
	})
	if len(issues) != 2 || !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues=%#v", issues)
	}
}

func TestDesignClosureIssuesPromotesOnlyAssociatedDiagnostics(t *testing.T) {
	selectedSource := "/symbols/Device.kicad_sym"
	index := LibraryIndex{Diagnostics: []reports.Issue{
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "library.symbol.Device:R", Message: "selected object defect"},
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: selectedSource, Message: "selected file syntax defect"},
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "library.symbol.Unrelated:Bad", Message: "unrelated defect"},
		{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "library.symbol.Device", Message: "multiple symbol library files use nickname Device"},
	}}
	closure := DesignClosure{Symbols: []ClosureSymbol{{LibraryID: "Device:R", Sources: []string{selectedSource}}}}
	issues := DesignClosureIssues(index, closure)
	if len(issues) != 3 {
		t.Fatalf("issues=%#v", issues)
	}
	for _, issue := range issues {
		if !issue.Blocking() || strings.Contains(issue.Message, "unrelated") {
			t.Fatalf("issue=%#v", issue)
		}
	}
}

func TestMissingClosureObjectRetainsCandidateSourceDiagnostics(t *testing.T) {
	path := "/symbols/Device.kicad_sym"
	index := LibraryIndex{
		Inventory: LibraryInventory{SymbolFiles: []LibraryFile{{Kind: LibraryFileSymbol, LibraryNickname: "Device", Path: path}}},
		Symbols:   map[string]SymbolRecord{},
		Diagnostics: []reports.Issue{{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: path, Message: "unexpected end of file",
		}},
	}
	closure, missing := ResolveDesignClosure(index, ClosureRequest{Symbols: []SymbolReference{{LibraryID: "Device:R"}}})
	if len(missing) != 1 || len(closure.Symbols) != 1 || !reflect.DeepEqual(closure.Symbols[0].Sources, []string{path}) {
		t.Fatalf("closure=%#v missing=%#v", closure, missing)
	}
	issues := DesignClosureIssues(index, closure)
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Message != "unexpected end of file" {
		t.Fatalf("issues=%#v", issues)
	}
}
