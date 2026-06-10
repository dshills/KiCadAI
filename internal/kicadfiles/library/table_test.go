package library

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteSymbolLibraryTable(t *testing.T) {
	var buf bytes.Buffer
	err := WriteSymbolLibraryTable(&buf, []TableEntry{{
		Name:        "local_symbols",
		Type:        "KiCad",
		URI:         "${KIPRJMOD}/lib/local_symbols.kicad_sym",
		Description: "project symbols",
	}})
	if err != nil {
		t.Fatalf("WriteSymbolLibraryTable returned error: %v", err)
	}
	want := strings.Join([]string{
		"(sym_lib_table",
		"  (version 7)",
		"  (lib",
		"    (name \"local_symbols\")",
		"    (type \"KiCad\")",
		"    (uri \"${KIPRJMOD}/lib/local_symbols.kicad_sym\")",
		"    (options \"\")",
		"    (descr \"project symbols\")",
		"  )",
		")",
		"",
	}, "\n")
	if buf.String() != want {
		t.Fatalf("output =\n%s\nwant =\n%s", buf.String(), want)
	}
}

func TestWriteFootprintLibraryTable(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFootprintLibraryTable(&buf, []TableEntry{{
		Name:        "local_footprints",
		Type:        "KiCad",
		URI:         "${KIPRJMOD}/footprints.pretty",
		Description: "project footprints",
	}})
	if err != nil {
		t.Fatalf("WriteFootprintLibraryTable returned error: %v", err)
	}
	for _, want := range []string{"(fp_lib_table", "(name \"local_footprints\")", "(uri \"${KIPRJMOD}/footprints.pretty\")"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %s:\n%s", want, buf.String())
		}
	}
}

func TestValidateTableEntriesRejectsInvalidEntries(t *testing.T) {
	err := ValidateTableEntries("sym_lib_table", []TableEntry{
		{Name: "bad name", Type: "KiCad", URI: "${KIPRJMOD}/bad.kicad_sym"},
		{Name: "dup", Type: "KiCad", URI: "a"},
		{Name: "DUP", Type: "KiCad", URI: "b"},
		{Name: "missing_uri", Type: "KiCad"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"invalid library nickname", "duplicate DUP", "entries[3].uri"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %s: %v", want, err)
		}
	}
}
