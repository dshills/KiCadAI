package designapi

import (
	"fmt"
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestNativeCardinalFieldsBecomeUprightWithoutRotatingSymbol(t *testing.T) {
	for _, symbolAngle := range []kicadfiles.Angle{0, 90, 180, 270} {
		for _, angle := range []kicadfiles.Angle{0, 90, 180, 270} {
			t.Run(fmt.Sprintf("symbol-%v/field-%v", symbolAngle, angle), func(t *testing.T) {
				b := newTestBuilder(t)
				b.nativeSchematicProfile = "annotation-v2"
				at := kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
				_, err := b.AddSymbol(SymbolOptions{Reference: "C1", LibraryID: "Device:C", Value: "22n", Position: at, Rotation: symbolAngle, UsePhysicalConnectionAnchors: true, Pins: []PinSpec{{Number: "1"}, {Number: "2"}}})
				if err != nil {
					t.Fatal(err)
				}
				s := &b.design.Schematic.Symbols[0]
				s.Properties = append(s.Properties, schematic.Property{Name: "Reference", Value: "C1", Position: at, Rotation: angle})
				s.Properties = append(s.Properties, schematic.Property{Name: "Value", Value: "22n", Position: at, Rotation: angle})
				s.Fields = append(s.Fields, schematic.Field{Name: "Purpose", Value: "filter", Visible: true, Position: at, Rotation: angle})
				original := cloneDesign(b.design).Schematic
				got := b.Design()
				if b.nativeAnnotationError != nil {
					t.Fatal(b.nativeAnnotationError)
				}
				if got.Schematic.Symbols[0].Rotation != symbolAngle || !reflect.DeepEqual(got.Schematic.Symbols[0].PinAnchors, original.Symbols[0].PinAnchors) {
					t.Fatal("symbol/pin geometry changed")
				}
				for _, f := range (&Builder{design: got}).nativeAnnotationFields() {
					if !nativeFieldIsUpright(f) {
						t.Fatal("field not upright", f.key)
					}
				}
				if !reflect.DeepEqual(b.design.Schematic, original) {
					t.Fatal("stored source changed")
				}
				if issues := AuditNativeAnnotations(*got.Schematic); len(issues) > 0 {
					t.Fatal(issues)
				}
				if err := b.auditSerializedNativeAnnotations(got.Schematic); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestNativeCardinalAuditAccountsForParentRotation(t *testing.T) {
	b := newTestBuilder(t)
	at := kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
	addTwoPinSymbol(t, b, "C1", "Device:C", "22n", at)
	b.design.Schematic.Symbols[0].Rotation = 90
	b.design.Schematic.Symbols[0].Properties = []schematic.Property{{Name: "Reference", Value: "C1", Position: kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(40)}, Rotation: 0}}
	if len(AuditNativeAnnotations(*b.design.Schematic)) == 0 {
		t.Fatal("serialized zero field angle on rotated parent was certified as horizontal")
	}
}

func TestNativeNonCardinalFieldsFailClosed(t *testing.T) {
	b := newTestBuilder(t)
	b.nativeSchematicProfile = "annotation-v2"
	at := kicadfiles.Point{X: kicadfiles.MM(50), Y: kicadfiles.MM(50)}
	addTwoPinSymbol(t, b, "C1", "Device:C", "22n", at)
	b.design.Schematic.Symbols[0].Properties = []schematic.Property{{Name: "Reference", Value: "C1", Position: at, Rotation: 45}}
	_ = b.Design()
	if b.nativeAnnotationError == nil {
		t.Fatal("non-cardinal field passed generation")
	}
	if len(AuditNativeAnnotations(*b.design.Schematic)) == 0 {
		t.Fatal("non-cardinal emitted field passed audit")
	}
}
