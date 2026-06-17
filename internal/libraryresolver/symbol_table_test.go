package libraryresolver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSymbolLibraryTable(t *testing.T) {
	root := t.TempDir()
	tablePath := filepath.Join(root, "sym-lib-table")
	mustWrite(t, tablePath, `(sym_lib_table
  (lib (name "local_symbols") (type "KiCad") (uri "${KIPRJMOD}/lib/local_symbols.kicad_sym") (options "") (descr "local"))
)`)

	entries, issues := ParseSymbolLibraryTable(tablePath)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(entries) != 1 || entries[0].Name != "local_symbols" || entries[0].URI != "${KIPRJMOD}/lib/local_symbols.kicad_sym" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestProjectSymbolTableShadowsRootSymbol(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	symbolRoot := filepath.Join(root, "symbols")
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "Device") (type "KiCad") (uri "${KIPRJMOD}/project_device.kicad_sym") (options "") (descr "project"))
)`)
	mustWrite(t, filepath.Join(project, "project_device.kicad_sym"), tableSymbolLibrary("Project resistor"))
	mustWrite(t, filepath.Join(symbolRoot, "Device.kicad_sym"), tableSymbolLibrary("Root resistor"))

	index, issues := Load(context.Background(), LibraryRoots{ProjectDir: project, SymbolsRoot: symbolRoot}, LoadOptions{})
	if hasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	record := index.Symbols["Device:R"]
	if record.Description != "Project resistor" || !strings.Contains(record.Path, "project_device.kicad_sym") {
		t.Fatalf("record = %#v", record)
	}
}

func TestProjectSymbolTableDuplicateNicknameBlocks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/a.kicad_sym") (options "") (descr ""))
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/b.kicad_sym") (options "") (descr ""))
)`)
	mustWrite(t, filepath.Join(project, "a.kicad_sym"), resistorSymbolLibrary())
	mustWrite(t, filepath.Join(project, "b.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if !hasSymbolIssue(inventory.Diagnostics, "duplicate symbol library nickname Local") {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
}

func TestProjectSymbolTableUnresolvedVariableBlocks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "Local") (type "KiCad") (uri "${UNKNOWN_SYMBOL_DIR}/a.kicad_sym") (options "") (descr ""))
)`)

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if !hasSymbolIssue(inventory.Diagnostics, "unresolved symbol library URI variable") {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
}

func TestProjectSymbolTableMissingPathBlocks(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "Local") (type "KiCad") (uri "${KIPRJMOD}/missing.kicad_sym") (options "") (descr ""))
)`)

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if !hasSymbolIssue(inventory.Diagnostics, "symbol library path not found") {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
}

func TestSymbolTableExpandsEnvironmentVariables(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	external := filepath.Join(root, "external")
	t.Setenv("MY_CUSTOM_SYMBOL_DIR", external)
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "External") (type "KiCad") (uri "${MY_CUSTOM_SYMBOL_DIR}/external.kicad_sym") (options "") (descr ""))
)`)
	mustWrite(t, filepath.Join(external, "external.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if hasBlockingIssue(inventory.Diagnostics) {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
	if len(inventory.SymbolFiles) != 1 || inventory.SymbolFiles[0].LibraryNickname != "External" {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
}

func TestSymbolTableAllowsEmptyEnvironmentVariable(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	t.Setenv("OPTIONAL_SYMBOL_SUBDIR", "")
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "Optional") (type "KiCad") (uri "${KIPRJMOD}/${OPTIONAL_SYMBOL_SUBDIR}optional.kicad_sym") (options "") (descr ""))
)`)
	mustWrite(t, filepath.Join(project, "optional.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if hasBlockingIssue(inventory.Diagnostics) {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
	if len(inventory.SymbolFiles) != 1 || inventory.SymbolFiles[0].LibraryNickname != "Optional" {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
}

func TestSymbolTableExpandsEnvironmentVariablesCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	external := filepath.Join(root, "external")
	t.Setenv("MY_CASE_SYMBOL_DIR", external)
	mustWrite(t, filepath.Join(project, "sym-lib-table"), `(sym_lib_table
  (lib (name "External") (type "KiCad") (uri "${my_case_symbol_dir}/external.kicad_sym") (options "") (descr ""))
)`)
	mustWrite(t, filepath.Join(external, "external.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{ProjectDir: project})
	if hasBlockingIssue(inventory.Diagnostics) {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
	if len(inventory.SymbolFiles) != 1 || inventory.SymbolFiles[0].LibraryNickname != "External" {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
}

func TestParseSymbolLibraryTableReportsMalformedEntry(t *testing.T) {
	root := t.TempDir()
	tablePath := filepath.Join(root, "sym-lib-table")
	mustWrite(t, tablePath, `(sym_lib_table
  (lib (name "MissingURI") (type "KiCad") (options "") (descr "bad"))
)`)

	entries, issues := ParseSymbolLibraryTable(tablePath)
	if len(entries) != 0 {
		t.Fatalf("entries = %#v", entries)
	}
	if !hasSymbolIssue(issues, "skipping malformed symbol library table entry") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestGlobalSymbolTableRelativePathResolvesFromTableDirectory(t *testing.T) {
	root := t.TempDir()
	global := filepath.Join(root, "global")
	mustWrite(t, filepath.Join(global, "sym-lib-table"), `(sym_lib_table
  (lib (name "Global") (type "KiCad") (uri "global_symbols.kicad_sym") (options "") (descr ""))
)`)
	mustWrite(t, filepath.Join(global, "global_symbols.kicad_sym"), resistorSymbolLibrary())

	inventory := Discover(LibraryRoots{GlobalSymbolTable: filepath.Join(global, "sym-lib-table")})
	if hasBlockingIssue(inventory.Diagnostics) {
		t.Fatalf("diagnostics = %#v", inventory.Diagnostics)
	}
	if len(inventory.SymbolFiles) != 1 || !strings.Contains(inventory.SymbolFiles[0].Path, "global_symbols.kicad_sym") {
		t.Fatalf("symbol files = %#v", inventory.SymbolFiles)
	}
}

func tableSymbolLibrary(description string) string {
	return strings.ReplaceAll(resistorSymbolLibrary(), "(ki_description \"Resistor\")", "(ki_description \""+description+"\")")
}
