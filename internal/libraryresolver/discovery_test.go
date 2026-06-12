package libraryresolver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSymbolAndFootprintFiles(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "MCU_Test.kicad_symdir", "PartA.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "README.md"), "ignore")
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), "(footprint)")
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.pretty", "notes.txt"), "ignore")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints})
	if len(inventory.Diagnostics) != 2 {
		t.Fatalf("expected missing KLC/templates warnings only, got %#v", inventory.Diagnostics)
	}
	if len(inventory.SymbolFiles) != 2 {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
	if len(inventory.FootprintFiles) != 1 {
		t.Fatalf("footprint files = %#v", inventory.FootprintFiles)
	}
	assertLibraryFile(t, inventory.SymbolFiles[0], LibraryFileSymbol, "Device", "Device", "Device:")
	assertLibraryFile(t, inventory.SymbolFiles[1], LibraryFileSymbol, "MCU_Test", "PartA", "MCU_Test:")
	assertLibraryFile(t, inventory.FootprintFiles[0], LibraryFileFootprint, "Resistor_SMD", "R_0805_2012Metric", "Resistor_SMD:")
	if inventory.SymbolLibraryCount != 2 || inventory.FootprintLibraryCount != 1 {
		t.Fatalf("library counts = %d/%d", inventory.SymbolLibraryCount, inventory.FootprintLibraryCount)
	}
}

func TestDiscoverSortsDeterministically(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Zed.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "Alpha.kicad_symdir", "B.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "Alpha.kicad_symdir", "A.kicad_sym"), "(kicad_symbol_lib)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	got := []string{}
	for _, file := range inventory.SymbolFiles {
		got = append(got, file.LibraryNickname+":"+file.Name)
	}
	want := []string{"Alpha:A", "Alpha:B", "Zed:Zed"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %#v, want %#v", got, want)
		}
	}
}

func TestDiscoverInvalidFootprintLayoutDiagnostic(t *testing.T) {
	root := t.TempDir()
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(footprints, "Loose.kicad_mod"), "(footprint)")

	inventory := Discover(LibraryRoots{FootprintsRoot: footprints})
	found := false
	for _, issue := range inventory.Diagnostics {
		if issue.Path == filepath.ToSlash(filepath.Join(footprints, "Loose.kicad_mod")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected loose footprint diagnostic: %#v", inventory.Diagnostics)
	}
}

func TestDiscoverHandlesCaseInsensitiveExtensions(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(symbols, "Device.KICAD_SYM"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.PRETTY", "R_0805_2012Metric.KICAD_MOD"), "(footprint)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints})
	if len(inventory.SymbolFiles) != 1 || inventory.SymbolFiles[0].LibraryNickname != "Device" {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
	if len(inventory.FootprintFiles) != 1 || inventory.FootprintFiles[0].LibraryNickname != "Resistor_SMD" {
		t.Fatalf("footprint files = %#v", inventory.FootprintFiles)
	}
}

func TestDiscoverHandlesNestedLibraryContainers(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(symbols, "Manufacturer", "Device.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "Manufacturer", "MCU_Test.kicad_symdir", "PartA.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(footprints, "Manufacturer", "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), "(footprint)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints})
	assertLibraryFile(t, inventory.SymbolFiles[0], LibraryFileSymbol, "Device", "Device", "Device:")
	assertLibraryFile(t, inventory.SymbolFiles[1], LibraryFileSymbol, "MCU_Test", "PartA", "MCU_Test:")
	assertLibraryFile(t, inventory.FootprintFiles[0], LibraryFileFootprint, "Resistor_SMD", "R_0805_2012Metric", "Resistor_SMD:")
}

func TestDiscoverHandlesLibraryContainerRoots(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "MCU_Test.kicad_symdir")
	footprints := filepath.Join(root, "Resistor_SMD.pretty")
	mustWrite(t, filepath.Join(symbols, "PartA.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(footprints, "R_0805_2012Metric.kicad_mod"), "(footprint)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints})
	assertLibraryFile(t, inventory.SymbolFiles[0], LibraryFileSymbol, "MCU_Test", "PartA", "MCU_Test:")
	assertLibraryFile(t, inventory.FootprintFiles[0], LibraryFileFootprint, "Resistor_SMD", "R_0805_2012Metric", "Resistor_SMD:")
}

func TestDiscoverReportsNicknameCollisionAcrossContainers(t *testing.T) {
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), "(kicad_symbol_lib)")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_symdir", "PartA.kicad_sym"), "(kicad_symbol_lib)")

	inventory := Discover(LibraryRoots{SymbolsRoot: symbols})
	found := false
	for _, issue := range inventory.Diagnostics {
		if issue.Path == "library.symbol.Device" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate nickname diagnostic: %#v", inventory.Diagnostics)
	}
}

func assertLibraryFile(t *testing.T, file LibraryFile, kind LibraryFileKind, nickname, name, prefix string) {
	t.Helper()
	if file.Kind != kind || file.LibraryNickname != nickname || file.Name != name || file.IDPrefix != prefix {
		t.Fatalf("file = %#v, want kind=%s nickname=%s name=%s prefix=%s", file, kind, nickname, name, prefix)
	}
}

func mustWrite(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
