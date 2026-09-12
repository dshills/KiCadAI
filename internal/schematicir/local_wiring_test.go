package schematicir

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
)

func TestFunctionalLocalWiringEligibilityPreservesExplicitIntent(t *testing.T) {
	doc := validLEDDocument()
	doc.Layout.Placements = []Placement{{Target: "r_limit", Orientation: OrientationRotated90}}
	index := &libraryresolver.LibraryIndex{Symbols: map[string]libraryresolver.SymbolRecord{}}
	for _, id := range []string{"Device:R", "Device:LED"} {
		index.Symbols[id] = libraryresolver.SymbolRecord{LibraryID: id, Raw: `(symbol "fixture")`, Pins: []libraryresolver.SymbolPin{
			{Number: "1", Position: kicadfiles.Point{X: kicadfiles.MM(-2.54)}},
			{Number: "2", Position: kicadfiles.Point{X: kicadfiles.MM(2.54)}},
		}}
	}
	net := doc.Circuit.Nets[1]
	components := indexComponentsByID(doc.Circuit.Components)
	if !functionalLocalWiringEligible(doc, net, index, components) {
		t.Fatal("resolver-backed rotated local net rejected")
	}
	for _, explicit := range []bool{false, true} {
		copy := net
		copy.UseLabel = &explicit
		if functionalLocalWiringEligible(doc, copy, index, components) {
			t.Fatal("explicit use_label intent overridden", explicit)
		}
	}
	if functionalLocalWiringEligible(doc, net, nil, components) {
		t.Fatal("unknown library permitted")
	}
	bus := net
	bus.Role = NetRoleBus
	if functionalLocalWiringEligible(doc, bus, index, components) {
		t.Fatal("bus semantics overridden")
	}
	bad := net
	bad.Connect = append([]EndpointRef(nil), net.Connect...)
	bad.Connect[0] = "r_limit.999"
	if functionalLocalWiringEligible(doc, bad, index, components) {
		t.Fatal("missing physical pin permitted")
	}
}
