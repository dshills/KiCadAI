package schematicir

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/schematiclayout"
)

func TestNativeLayoutReservesPinAnnotations(t *testing.T) {
	body := schematiclayout.Rect{MinX: -100, MinY: -100, MaxX: 100, MaxY: 100}
	pins := []schematiclayout.Pin{{At: kicadfiles.Point{X: 500, Y: 1000}}}
	got := schematicAnnotationEnvelope(body, pins)
	pad := kicadfiles.MM(2.54)
	if got.MinX != -100-pad || got.MaxX != 500+pad || got.MaxY != 1000+pad {
		t.Fatalf("pin envelope: %+v", got)
	}
}

func TestNativeLayoutFieldsUseCenteredKiCadPropertyAnchors(t *testing.T) {
	c := schematiclayout.PlacedComponent{Component: schematiclayout.Component{Ref: "U1", ReferenceText: schematiclayout.TextBox{At: kicadfiles.Point{X: 10, Y: 20}, Box: schematiclayout.Rect{MinX: 10, MinY: 10, MaxX: 30, MaxY: 20}}}, PlacedAt: kicadfiles.Point{X: 100, Y: 200}}
	r := schematiclayout.Result{Components: []schematiclayout.PlacedComponent{c}}
	legacy, native := layoutTextPlacements(r, false)["U1"], layoutTextPlacements(r, true)["U1"]
	if legacy.reference.XMM != .00011 || legacy.reference.YMM != .00022 || native.reference.XMM != .00012 || native.reference.YMM != .000215 {
		t.Fatalf("legacy=%+v native=%+v", legacy.reference, native.reference)
	}
}
