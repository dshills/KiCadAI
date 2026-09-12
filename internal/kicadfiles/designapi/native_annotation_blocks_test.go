package designapi

import (
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func TestNativeBlockValidationAndClone(t *testing.T) {
	block := schematiclayout.NativeAnnotationBlock{ID: "group", References: []string{"R1"}, Lines: []string{"LOCAL GROUP", "Parts: R1"}}
	valid := Options{NativeSchematicProfile: schematiclayout.NativeAnnotationV2, NativeSchematicBlocks: []schematiclayout.NativeAnnotationBlock{block}}
	if err := validateNativeAnnotationBlocks(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Options){
		"legacy":    func(o *Options) { o.NativeSchematicProfile = "" },
		"global":    func(o *Options) { o.NativeSchematicNotes = []string{"guide"} },
		"duplicate": func(o *Options) { o.NativeSchematicBlocks = append(o.NativeSchematicBlocks, block) },
		"empty":     func(o *Options) { o.NativeSchematicBlocks[0].Lines = []string{" "} },
		"long":      func(o *Options) { o.NativeSchematicBlocks[0].Lines = []string{strings.Repeat("X", 65)} },
		"shared": func(o *Options) {
			b := block
			b.ID = "other"
			o.NativeSchematicBlocks = append(o.NativeSchematicBlocks, b)
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := valid
			o.NativeSchematicBlocks = schematiclayout.CloneNativeAnnotationBlocks(valid.NativeSchematicBlocks)
			mutate(&o)
			if validateNativeAnnotationBlocks(o) == nil {
				t.Fatal("invalid blocks accepted")
			}
		})
	}
	clone := schematiclayout.CloneNativeAnnotationBlocks(valid.NativeSchematicBlocks)
	clone[0].References[0] = "changed"
	clone[0].Lines[0] = "changed"
	if valid.NativeSchematicBlocks[0].References[0] != "R1" || valid.NativeSchematicBlocks[0].Lines[0] != "LOCAL GROUP" {
		t.Fatal("shallow clone")
	}
}

func TestNativeBlockExistingWireNeedsIslandLabel(t *testing.T) {
	b := newTestBuilder(t)
	b.nativeSchematicProfile = schematiclayout.NativeAnnotationV2
	b.nativeSchematicBlocks = []schematiclayout.NativeAnnotationBlock{{ID: "g", References: []string{"R1", "R2", "R3"}, Lines: []string{"GROUP"}}}
	p := func(x, y float64) kicadfiles.Point { return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)} }
	for i, ref := range []string{"R1", "R2", "R3"} {
		addTwoPinSymbol(t, b, ref, "Device:R", "1k", p(60+float64(i)*50, 100))
	}
	a, z := p(65, 100), p(105, 100)
	b.addSchematicWire("opaque", Endpoint{Reference: "R1", Pin: "2"}, Endpoint{Reference: "R2", Pin: "1"}, a, z)
	// A same-name label on a disconnected island cannot suppress this one.
	b.addSchematicWire("opaque", Endpoint{}, Endpoint{}, p(165, 100), p(180, 100))
	if err := b.AddLabel("opaque", p(180, 100), schematic.LabelLocal); err != nil {
		t.Fatal(err)
	}
	wires := len(b.design.Schematic.Wires)
	b.addSchematicLabelStub("opaque", Endpoint{Reference: "R1", Pin: "2"}, a, p(0, -5))
	if len(b.design.Schematic.Labels) != 2 || len(b.design.Schematic.Wires) != wires {
		t.Fatal("unlabeled island or extra stub")
	}
	b.addSchematicLabelStub("opaque", Endpoint{Reference: "R1", Pin: "2"}, a, p(0, -5))
	if len(b.design.Schematic.Labels) != 2 {
		t.Fatal("duplicate island label")
	}
	result := b.Design()
	if b.nativeAnnotationError != nil {
		t.Fatal(b.nativeAnnotationError)
	}
	if len(result.Schematic.Labels) != 2 || len(AuditNativeAnnotations(*result.Schematic)) != 0 {
		t.Fatal("invalid emitted labels")
	}
	for _, label := range result.Schematic.Labels {
		if label.Position.X < kicadfiles.MM(130) && !pointOnSchematicSegment(label.Position, a, z) {
			t.Fatal("label left its island")
		}
	}
}

func TestNativeBlockLocalIntactIdempotentAndMissingAnchor(t *testing.T) {
	b := newTestBuilder(t)
	b.nativeSchematicProfile = schematiclayout.NativeAnnotationV2
	b.nativeSchematicBlocks = []schematiclayout.NativeAnnotationBlock{{ID: "group", References: []string{"R1"}, Lines: []string{"LOCAL GROUP", strings.Repeat("pin intent ", 8)}}}
	addTwoPinSymbol(t, b, "R1", "Device:R", "1k", kicadfiles.Point{X: kicadfiles.MM(100), Y: kicadfiles.MM(100)})
	a, z := b.Design(), b.Design()
	if b.nativeAnnotationError != nil {
		t.Fatal(b.nativeAnnotationError)
	}
	if !reflect.DeepEqual(a, z) || len(b.design.Schematic.Texts) != 0 {
		t.Fatal("non-idempotent or mutated source")
	}
	if issues := AuditNativeAnnotations(*a.Schematic); len(issues) != 0 {
		t.Fatal(issues)
	}
	texts := a.Schematic.Texts
	if len(texts) != 3 {
		t.Fatalf("wrapped lines=%d", len(texts))
	}
	for i := 1; i < len(texts); i++ {
		if texts[i].Position.Y-texts[i-1].Position.Y != kicadfiles.MM(2.54) {
			t.Fatal("block split")
		}
	}
	b.nativeSchematicBlocks[0].References = []string{"MISSING"}
	_ = b.Design()
	if b.nativeAnnotationError == nil {
		t.Fatal("missing anchor accepted")
	}
}
