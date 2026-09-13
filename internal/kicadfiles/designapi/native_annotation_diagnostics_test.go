package designapi

import (
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func TestNativeJointDiagnosticsAreIsolated(t *testing.T) {
	b := newTestBuilder(t)
	if _, err := b.NativeAnnotationDiagnostics(); err == nil {
		t.Fatal("legacy profile accepted")
	}
	var absent *Builder
	if _, err := absent.NativeAnnotationDiagnostics(); err == nil {
		t.Fatal("absent schematic accepted")
	}
	b.nativeSchematicProfile = schematiclayout.NativeAnnotationV2
	b.nativeSchematicBlocks = []schematiclayout.NativeAnnotationBlock{{ID: "local", References: []string{"R1"}, Lines: []string{"LOCAL"}}}
	addTwoPinSymbol(t, b, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(100), Y: kicadfiles.MM(100)})
	if err := b.AddLabel("unchanged", kicadfiles.Point{X: kicadfiles.MM(150), Y: kicadfiles.MM(100)}, schematic.LabelLocal); err != nil {
		t.Fatal(err)
	}
	before := cloneDesign(b.design)
	ids := b.finalRouteLabelUUIDs
	first, err := b.NativeAnnotationDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.design, before) || !reflect.DeepEqual(b.finalRouteLabelUUIDs, ids) || b.nativeAnnotationError != nil {
		t.Fatal("diagnostic mutated builder")
	}
	second, err := b.NativeAnnotationDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 1 || len(first[0].Candidates) == 0 {
		t.Fatal("unstable or incomplete candidates")
	}
	first[0].Candidates[0].At.X = 0
	third, err := b.NativeAnnotationDiagnostics()
	if err != nil || !reflect.DeepEqual(second, third) {
		t.Fatal("candidate aliases builder state", err)
	}
	b.nativeSchematicBlocks[0].References = []string{"MISSING"}
	_, err = b.NativeAnnotationDiagnostics()
	if err == nil {
		t.Fatal("unsatisfiable panel hidden")
	}
	if b.nativeAnnotationError != nil || len(b.design.Schematic.Texts) != 0 {
		t.Fatal("failed diagnostic mutated builder")
	}
}
