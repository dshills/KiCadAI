package libraryresolver

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestBundledClockSymbolResolvesWithoutExternalSymbolRoot(t *testing.T) {
	index, issues := Load(context.Background(), LibraryRoots{}, LoadOptions{})
	if slices.ContainsFunc(issues, func(issue reports.Issue) bool { return issue.Blocking() }) {
		t.Fatalf("bundled symbol load issues = %#v", issues)
	}
	record, ok := ResolveSymbol(index, "KiCadAI_Clock:LTC6906")
	if !ok {
		t.Fatal("bundled LTC6906 symbol did not resolve")
	}
	if record.Path != "embedded://bundled/KiCadAI_Clock.kicad_sym" || len(record.Raw) == 0 {
		t.Fatalf("bundled symbol provenance = %#v", record)
	}
	want := map[string]string{"1": "OUT", "2": "GND", "3": "DIV", "4": "SET", "5": "GRD", "6": "V+"}
	for pin, name := range want {
		if !slices.ContainsFunc(record.Pins, func(candidate SymbolPin) bool {
			return candidate.Number == pin && candidate.Name == name
		}) {
			t.Fatalf("bundled symbol missing pin %s %s: %#v", pin, name, record.Pins)
		}
	}
}
