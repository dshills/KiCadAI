package designapi

import (
	"bytes"
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/schematiclayout"
)

// AuditNativeAnnotations checks serialized geometry, not planner diagnostics.
// Native SVG inspection remains separate: stroke/font estimates are deliberately
// conservative, not a general KiCad typography or human-readability proof.
func AuditNativeAnnotations(file schematic.SchematicFile) []string {
	b := &Builder{design: kicaddesign.Design{Schematic: &file}}
	var issues []string
	if len(file.Sheets) != 0 {
		issues = append(issues, "native annotation audit supports flat sheets only")
	}
	for _, s := range file.Symbols {
		if _, known := hierarchySymbolBody(b, s); !known {
			issues = append(issues, "unknown emitted symbol geometry: "+s.Reference)
		}
	}
	occupied := b.nativeAnnotationBodies()
	check := func(name string, box schematiclayout.Rect) {
		if !b.nativeTextClear(box, occupied) {
			issues = append(issues, "emitted annotation collision or page overflow: "+name)
		}
		occupied = append(occupied, box)
	}
	for _, label := range file.Labels {
		if label.Rotation != 0 && label.Rotation != 90 && label.Rotation != 180 && label.Rotation != 270 {
			issues = append(issues, "unsupported native label angle: "+label.Text)
		}
		if label.Kind != schematic.LabelLocal {
			issues = append(issues, "unsupported native annotation label kind: "+label.Text)
		}
		if !containsFold(label.Justify, "bottom") {
			issues = append(issues, "native label lacks explicit bottom justification: "+label.Text)
		}
		check("label "+label.Text, schematiclayout.NativeLabelBounds(label.Text, label.Position, label.Rotation, containsFold(label.Justify, "right")))
	}
	for _, field := range b.nativeAnnotationFields() {
		if !nativeFieldIsUpright(field) {
			issues = append(issues, "unsupported native field angle: "+field.key)
		}
		check(field.key, schematiclayout.NativeFieldBounds(field.text, field.preferred))
	}
	for _, note := range file.Texts {
		if note.Rotation != 0 || strings.Contains(note.Value, "\n") {
			issues = append(issues, "unsupported emitted note geometry")
		}
		check("note "+note.Value, schematiclayout.NativeFieldBounds(note.Value, note.Position))
	}
	// The preservation reader intentionally retains free text as raw items.
	// Decode those items here instead of silently omitting the reading guide
	// from the emitted-geometry audit or changing legacy read/write receipts.
	for _, item := range file.RawItems {
		node, err := sexpr.Parse([]byte(item.Body))
		if err != nil {
			issues = append(issues, "invalid raw schematic item")
			continue
		}
		if node.Head() != "text" {
			issues = append(issues, "unsupported raw item in native annotation audit: "+node.Head())
			continue
		}
		at, ok := node.Child("at")
		x, xok := at.FloatValue(1)
		y, yok := at.FloatValue(2)
		rotation, _ := at.FloatValue(3)
		effects, eok := node.Child("effects")
		font, fok := effects.Child("font")
		size, sok := font.Child("size")
		sx, sxok := size.FloatValue(1)
		sy, syok := size.FloatValue(2)
		_, justified := effects.Child("justify")
		if !ok || !xok || !yok || rotation != 0 || !eok || !fok || !sok || !sxok || !syok || sx != 1.27 || sy != 1.27 || justified || strings.Contains(node.ListValue(1), "\n") {
			issues = append(issues, "unsupported raw note geometry")
			continue
		}
		check("note "+node.ListValue(1), schematiclayout.NativeFieldBounds(node.ListValue(1), kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}))
	}
	return issues
}

func (builder *Builder) auditSerializedNativeAnnotations(file *schematic.SchematicFile) error {
	if builder.nativeSchematicProfile != schematiclayout.NativeAnnotationV2 {
		return nil
	}
	if file == nil {
		return fmt.Errorf("native annotation audit requires schematic")
	}
	var out bytes.Buffer
	if err := schematic.Write(&out, *file); err != nil {
		return fmt.Errorf("serialize native annotation audit: %w", err)
	}
	readback, err := schematic.Read(out.Bytes())
	if err != nil {
		return fmt.Errorf("read native annotation audit: %w", err)
	}
	if issues := AuditNativeAnnotations(readback); len(issues) != 0 {
		return fmt.Errorf("native annotation readback audit: %s", strings.Join(issues, "; "))
	}
	return nil
}
