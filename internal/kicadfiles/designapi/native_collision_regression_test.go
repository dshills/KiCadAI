package designapi

import (
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestNativeStubDetectsExistingNetLabelGlyphs(t *testing.T) {
	b, err := New(Options{Name: "native_label_crossing", DesignID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", NativeSchematicProfile: "annotation-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AddLabelWithOptions("REFERENCE_NET", kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}, schematic.LabelLocal, LabelOptions{Justify: []string{"left", "bottom"}}); err != nil {
		t.Fatal(err)
	}
	if !b.schematicStubTouchesVisibleText(kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(15)}, kicadfiles.Point{X: kicadfiles.MM(25), Y: kicadfiles.MM(25)}) {
		t.Fatal("native stub may cross an existing net-label glyph rectangle")
	}
}

func TestNativeDesignIsIdempotentAndAuditRejectsWireCrossing(t *testing.T) {
	b := newTestBuilder(t)
	b.nativeSchematicProfile = "annotation-v2"
	b.nativeSchematicNotes = []string{"Explicit circuit reading guide"}
	addTwoPinSymbol(t, b, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)})
	a, z := b.Design(), b.Design()
	if b.nativeAnnotationError != nil {
		t.Fatal(b.nativeAnnotationError)
	}
	if !reflect.DeepEqual(a, z) {
		t.Fatal("native Design changes across repeated calls")
	}
	if len(b.design.Schematic.Texts) != 0 {
		t.Fatal("finalization mutated the stored input")
	}
	if issues := AuditNativeAnnotations(*a.Schematic); len(issues) != 0 {
		t.Fatal(issues)
	}
	field := a.Schematic.Texts[0]
	a.Schematic.Wires = append(a.Schematic.Wires, schematic.NewWire("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", kicadfiles.Point{X: field.Position.X - kicadfiles.MM(10), Y: field.Position.Y}, kicadfiles.Point{X: field.Position.X + kicadfiles.MM(10), Y: field.Position.Y}))
	if len(AuditNativeAnnotations(*a.Schematic)) == 0 {
		t.Fatal("emitted wire through annotation passed audit")
	}
}

func TestNativeLabelCannotJumpToDisconnectedSameNamedIsland(t *testing.T) {
	b := newTestBuilder(t)
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	b.addSchematicWire("N", Endpoint{}, Endpoint{}, p(20, 20), p(30, 20))
	b.addSchematicWire("N", Endpoint{}, Endpoint{}, p(30, 20), p(30, 30))
	b.addSchematicWire("N", Endpoint{}, Endpoint{}, p(50, 50), p(60, 50))
	label := schematic.NewLabel("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "N", schematic.LabelLocal, p(25, 20))
	if got := len(b.nativeLabelIsland(label)); got != 2 {
		t.Fatalf("reachable wires=%d, expected two connected segments", got)
	}
}
