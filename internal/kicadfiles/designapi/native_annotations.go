package designapi

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

type nativeAnnotationField struct {
	key            string
	text           string
	preferred      kicadfiles.Point
	rotation       kicadfiles.Angle
	symbolRotation kicadfiles.Angle
	set            func(kicadfiles.Point)
}

// finalizeNativeAnnotations runs after routing, when all wire obstacles exist.
// It changes annotation positions only: symbol/pin geometry, wire connectivity,
// electrical net names and the PCB remain untouched. Failure is explicit.
func (builder *Builder) finalizeNativeAnnotations() error {
	if builder.design.Schematic == nil {
		return fmt.Errorf("native annotation profile requires a schematic")
	}
	if builder.hierarchy != nil && len(builder.hierarchy.Sheets) != 0 {
		return fmt.Errorf("native annotation-v2 requires flat-sheet validation; hierarchy is not certified")
	}
	fields := builder.nativeAnnotationFields()
	bodies := builder.nativeAnnotationBodies()
	occupied := append([]schematiclayout.Rect(nil), bodies...)
	labels := append([]schematic.Label(nil), builder.design.Schematic.Labels...)
	candidates := make(map[kicadfiles.UUID][]nativeLabelCandidate, len(labels))
	for _, label := range labels {
		candidates[label.UUID] = builder.nativeLabelCandidates(label, occupied)
		if len(candidates[label.UUID]) == 0 {
			return fmt.Errorf("native annotation placement exhausted for label %s at %s: no wire/body-clear candidate", label.Text, formatPoint(label.Position))
		}
	}
	// Most-constrained-first search allows a long label to yield its preferred
	// position when that would strand another electrical anchor. UUIDs keep
	// ties deterministic. Search is explicitly bounded and fails closed.
	sort.SliceStable(labels, func(i, j int) bool {
		if len(candidates[labels[i].UUID]) != len(candidates[labels[j].UUID]) {
			return len(candidates[labels[i].UUID]) < len(candidates[labels[j].UUID])
		}
		return string(labels[i].UUID) < string(labels[j].UUID)
	})
	visits := 0
	var place func(int) bool
	place = func(i int) bool {
		if i == len(labels) {
			return true
		}
		for _, candidate := range candidates[labels[i].UUID] {
			visits++
			if visits > 50000 {
				return false
			}
			if !nativeRectClear(candidate.box, occupied) {
				continue
			}
			labels[i].Position, labels[i].Rotation, labels[i].Justify = candidate.at, candidate.options.Rotation, candidate.options.Justify
			occupied = append(occupied, candidate.box.Inflate(kicadfiles.MM(0.254)))
			if place(i + 1) {
				return true
			}
			occupied = occupied[:len(occupied)-1]
		}
		return false
	}
	if !place(0) {
		return fmt.Errorf("native annotation placement exhausted after %d bounded candidate visits", visits)
	}
	// Preserve document ordering: relocation is not a net-order normalization.
	byID := make(map[kicadfiles.UUID]schematic.Label, len(labels))
	for _, label := range labels {
		byID[label.UUID] = label
	}
	for i, label := range builder.design.Schematic.Labels {
		builder.design.Schematic.Labels[i] = byID[label.UUID]
	}
	// Electrical labels are constrained to their original wire segment, while
	// fields may move freely. Reserve the constrained annotations first.
	for _, field := range fields {
		if !nativeCardinalAngle(field.rotation) || !nativeCardinalAngle(field.symbolRotation) {
			return fmt.Errorf("native annotation profile does not support rotated field %s", field.key)
		}
		at, ok := builder.nativeFieldPosition(field.text, field.preferred, occupied)
		if !ok {
			return fmt.Errorf("native annotation placement exhausted for field %s", field.key)
		}
		// KiCad field angles are interpreted with the parent symbol transform.
		// Cancel that cardinal orientation before using horizontal glyph bounds;
		// symbol and pin rotations remain untouched.
		field.set(at)
		occupied = append(occupied, schematiclayout.NativeFieldBounds(field.text, at).Inflate(kicadfiles.MM(0.635)))
	}
	return builder.placeNativeReadingGuide(occupied)
}

func (builder *Builder) nativeAnnotationFields() []nativeAnnotationField {
	var fields []nativeAnnotationField
	for si := range builder.design.Schematic.Symbols {
		s := &builder.design.Schematic.Symbols[si]
		for pi := range s.Properties {
			p := &s.Properties[pi]
			if p.Hidden || strings.TrimSpace(p.Value) == "" {
				continue
			}
			fields = append(fields, nativeAnnotationField{key: fmt.Sprintf("%s.property.%d", s.Reference, pi), text: p.Value, preferred: p.Position, rotation: p.Rotation, symbolRotation: s.Rotation, set: func(at kicadfiles.Point) { p.Position, p.Rotation = at, nativeUprightFieldAngle(s.Rotation) }})
		}
		for fi := range s.Fields {
			f := &s.Fields[fi]
			if f.Hidden || (!f.Visible && f.Name != "Reference" && f.Name != "Value") || strings.TrimSpace(f.Value) == "" {
				continue
			}
			fields = append(fields, nativeAnnotationField{key: fmt.Sprintf("%s.field.%d", s.Reference, fi), text: f.Value, preferred: f.Position, rotation: f.Rotation, symbolRotation: s.Rotation, set: func(at kicadfiles.Point) { f.Position, f.Rotation = at, nativeUprightFieldAngle(s.Rotation) }})
		}
	}
	return fields
}

func nativeCardinalAngle(angle kicadfiles.Angle) bool {
	return angle == 0 || angle == 90 || angle == 180 || angle == 270
}

func nativeUprightFieldAngle(symbolRotation kicadfiles.Angle) kicadfiles.Angle {
	if symbolRotation == 0 {
		return 0
	}
	return 360 - symbolRotation
}

func nativeFieldIsUpright(field nativeAnnotationField) bool {
	if !nativeCardinalAngle(field.rotation) || !nativeCardinalAngle(field.symbolRotation) {
		return false
	}
	// The native renderer auto-flips text for readability; 0/180 share a
	// horizontal glyph rectangle, while 90/270 share a vertical rectangle.
	return (int(field.rotation)+int(field.symbolRotation))%180 == 0
}

func (builder *Builder) nativeAnnotationBodies() []schematiclayout.Rect {
	bodies := make([]schematiclayout.Rect, 0, len(builder.design.Schematic.Symbols))
	for _, s := range builder.design.Schematic.Symbols {
		body, known := hierarchySymbolBody(builder, s)
		if known {
			body = schematiclayout.TransformRect(body, s.Rotation, schematiclayout.Mirror(s.Mirror)).Translate(s.Position)
		} else {
			body = schematicSymbolCollisionBody(builder, s)
		}
		bodies = append(bodies, body.Inflate(kicadfiles.MM(0.254)))
		for _, p := range s.PinAnchors {
			// Reserve each external pin corridor independently. A union of all
			// pin anchors with the body wrongly fills clear corner space beside
			// capacitors and other narrow symbols.
			q := kicadfiles.Point{X: max(body.MinX, min(body.MaxX, p.X)), Y: max(body.MinY, min(body.MaxY, p.Y))}
			bodies = append(bodies, (schematiclayout.Rect{MinX: min(p.X, q.X), MaxX: max(p.X, q.X), MinY: min(p.Y, q.Y), MaxY: max(p.Y, q.Y)}).Inflate(kicadfiles.MM(0.635)))
		}
	}
	return bodies
}

func (builder *Builder) nativeTextClear(box schematiclayout.Rect, occupied []schematiclayout.Rect) bool {
	margin := kicadfiles.MM(5)
	paper := nativeAnnotationPaper(builder.design.Schematic.Paper)
	if box.Empty() || box.MinX < margin || box.MinY < margin || box.MaxX > paper.Width-margin || box.MaxY > paper.Height-margin {
		return false
	}
	if !nativeRectClear(box, occupied) {
		return false
	}
	// A label may sit just above its own wire baseline, but glyph interiors
	// cannot be crossed by any wire, including another wire on the same net.
	interior := box.Inflate(-kicadfiles.MM(0.05))
	for _, wire := range builder.design.Schematic.Wires {
		for i := 1; i < len(wire.Points); i++ {
			if schematiclayout.SegmentIntersectsRect(schematiclayout.WireSegment{From: wire.Points[i-1], To: wire.Points[i]}, interior) {
				return false
			}
		}
	}
	return true
}

func nativeAnnotationPaper(paper kicadfiles.Paper) kicadfiles.Paper {
	if paper.Width > 0 && paper.Height > 0 {
		return paper
	}
	switch paper.Name {
	case "A0", "A1", "A2", "A3", "A4", "A5", "USLetter", "USLegal", "USLedger":
		sheet := schematiclayout.SheetForPaperOrientation(paper.Name, paper.Portrait)
		paper.Width, paper.Height = sheet.Width, sheet.Height
	}
	return paper
}

func (builder *Builder) nativeFieldPosition(text string, preferred kicadfiles.Point, occupied []schematiclayout.Rect) (kicadfiles.Point, bool) {
	grid := kicadfiles.MM(1.27)
	for radius := 0; radius <= 64; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if max(absIU(kicadfiles.IU(dx)), absIU(kicadfiles.IU(dy))) != kicadfiles.IU(radius) {
					continue
				}
				at := kicadfiles.Point{X: preferred.X + kicadfiles.IU(dx)*grid, Y: preferred.Y + kicadfiles.IU(dy)*grid}
				if builder.nativeTextClear(schematiclayout.NativeFieldBounds(text, at), occupied) {
					return at, true
				}
			}
		}
	}
	return kicadfiles.Point{}, false
}

func nativeRectClear(box schematiclayout.Rect, occupied []schematiclayout.Rect) bool {
	for _, obstacle := range occupied {
		if box.Intersects(obstacle) {
			return false
		}
	}
	return true
}

type nativeLabelCandidate struct {
	at      kicadfiles.Point
	options LabelOptions
	box     schematiclayout.Rect
}

func (builder *Builder) nativeLabelCandidates(label schematic.Label, occupied []schematiclayout.Rect) []nativeLabelCandidate {
	positions := []kicadfiles.Point{label.Position}
	// Stay in the original geometrically connected wire island. A bend is not
	// an electrical boundary, but a disconnected same-named island is.
	for _, wire := range builder.nativeLabelIsland(label) {
		for i := 1; i < len(wire.Points); i++ {
			a, b := wire.Points[i-1], wire.Points[i]
			length := max(absIU(b.X-a.X), absIU(b.Y-a.Y))
			if length == 0 {
				continue
			}
			// Generated routes are orthogonal. Keep candidates on exact grid
			// steps instead of interpolating a nonintegral number of steps.
			if a.X != b.X && a.Y != b.Y {
				continue
			}
			positions = append(positions, a, b)
			for distance := kicadfiles.MM(1.27); distance < length; distance += kicadfiles.MM(1.27) {
				positions = append(positions, kicadfiles.Point{X: a.X + (b.X-a.X)/length*distance, Y: a.Y + (b.Y-a.Y)/length*distance})
			}
		}
	}
	sort.SliceStable(positions, func(i, j int) bool {
		return absIU(positions[i].X-label.Position.X)+absIU(positions[i].Y-label.Position.Y) < absIU(positions[j].X-label.Position.X)+absIU(positions[j].Y-label.Position.Y)
	})
	options := []LabelOptions{{Rotation: label.Rotation, Justify: []string{"left", "bottom"}}, {Justify: []string{"left", "bottom"}}, {Rotation: 180, Justify: []string{"right", "bottom"}}, {Rotation: 90, Justify: []string{"left", "bottom"}}, {Rotation: 270, Justify: []string{"right", "bottom"}}}
	if containsFold(label.Justify, "right") {
		options[0].Justify[0] = "right"
	}
	var candidates []nativeLabelCandidate
	seen := map[string]bool{}
	for _, at := range positions {
		if builder.schematicPointTouchesForeignWire(label.Text, at) || builder.schematicPinAnchorCount(at) > 0 {
			continue
		}
		for _, option := range options {
			if option.Rotation != 0 && option.Rotation != 90 && option.Rotation != 180 && option.Rotation != 270 {
				continue
			}
			box := schematiclayout.NativeLabelBounds(label.Text, at, option.Rotation, containsFold(option.Justify, "right"))
			key := fmt.Sprintf("%v/%v", at, box)
			if !seen[key] && builder.nativeTextClear(box, occupied) {
				candidates = append(candidates, nativeLabelCandidate{at: at, options: option, box: box})
				seen[key] = true
			}
		}
	}
	return candidates
}

func (builder *Builder) nativeLabelIsland(label schematic.Label) []schematic.Wire {
	var pool []schematic.Wire
	for _, wire := range builder.design.Schematic.Wires {
		if builder.canonicalNet(builder.schematicWireNets[wire.UUID]) == builder.canonicalNet(label.Text) {
			pool = append(pool, wire)
		}
	}
	points := []kicadfiles.Point{label.Position}
	seen := make(map[kicadfiles.UUID]bool)
	var result []schematic.Wire
	for head := 0; head < len(points); head++ {
		for _, wire := range pool {
			if seen[wire.UUID] {
				continue
			}
			for i := 1; i < len(wire.Points); i++ {
				if !pointOnSchematicSegment(points[head], wire.Points[i-1], wire.Points[i]) {
					continue
				}
				seen[wire.UUID] = true
				result = append(result, wire)
				points = append(points, wire.Points...)
				break
			}
		}
	}
	return result
}
