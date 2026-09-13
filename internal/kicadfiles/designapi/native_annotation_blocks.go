package designapi

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func validateNativeAnnotationBlocks(options Options) error {
	blocks := options.NativeSchematicBlocks
	if len(blocks) == 0 {
		return nil
	}
	if options.NativeSchematicProfile != schematiclayout.NativeAnnotationV2 || len(options.NativeSchematicNotes) != 0 || len(blocks) > 64 {
		return fmt.Errorf("local annotation blocks require annotation-v2, no global guide and at most 64 blocks")
	}
	ids, refs := map[string]bool{}, map[string]bool{}
	for _, b := range blocks {
		if strings.TrimSpace(b.ID) == "" || ids[b.ID] || len(b.References) == 0 || len(b.Lines) == 0 || len(b.Lines) > 24 {
			return fmt.Errorf("invalid local annotation block")
		}
		ids[b.ID] = true
		for _, ref := range b.References {
			if strings.TrimSpace(ref) == "" || refs[ref] {
				return fmt.Errorf("ambiguous local annotation reference")
			}
			refs[ref] = true
		}
		if _, err := nativeBlockLines(b.Lines); err != nil {
			return err
		}
	}
	return nil
}

func nativeBlockLines(paragraphs []string) ([]string, error) {
	var lines []string
	for _, paragraph := range paragraphs {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if utf8.RuneCountInString(word) > 64 {
				return nil, fmt.Errorf("local annotation token exceeds 64 characters")
			}
			if line != "" && utf8.RuneCountInString(line)+utf8.RuneCountInString(word)+1 > 64 {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		if line == "" {
			return nil, fmt.Errorf("empty local annotation paragraph")
		}
		lines = append(lines, line)
	}
	if len(lines) > 32 {
		return nil, fmt.Errorf("local annotation block exceeds 32 rendered lines")
	}
	return lines, nil
}

func (builder *Builder) placeNativeAnnotationBlocks(occupied []schematiclayout.Rect) error {
	blocks := schematiclayout.CloneNativeAnnotationBlocks(builder.nativeSchematicBlocks)
	slices.SortFunc(blocks, func(a, b schematiclayout.NativeAnnotationBlock) int { return strings.Compare(a.ID, b.ID) })
	boundsByID := map[string]schematiclayout.Rect{}
	for _, block := range blocks {
		var bounds schematiclayout.Rect
		first := true
		for _, ref := range block.References {
			found := false
			for _, symbol := range builder.design.Schematic.Symbols {
				if symbol.Reference != ref {
					continue
				}
				found = true
				body := schematicSymbolCollisionBody(builder, symbol)
				if first {
					bounds, first = body, false
				} else {
					bounds = schematiclayout.Rect{MinX: min(bounds.MinX, body.MinX), MinY: min(bounds.MinY, body.MinY), MaxX: max(bounds.MaxX, body.MaxX), MaxY: max(bounds.MaxY, body.MaxY)}
				}
			}
			if !found {
				return fmt.Errorf("local annotation block %s references missing symbol %s", block.ID, ref)
			}
		}
		boundsByID[block.ID] = bounds
		// A block's text must not occupy the whitespace inside another block.
		occupied = append(occupied, bounds.Inflate(kicadfiles.MM(2.54)))
	}
	for _, block := range blocks {
		lines, err := nativeBlockLines(block.Lines)
		if err != nil {
			return err
		}
		width := kicadfiles.IU(0)
		for _, line := range lines {
			width = max(width, schematiclayout.NativeFieldBounds(line, kicadfiles.Point{}).Width())
		}
		row := kicadfiles.MM(2.54)
		height := row * kicadfiles.IU(len(lines))
		anchor := boundsByID[block.ID]
		preferred := kicadfiles.Point{X: anchor.MinX, Y: anchor.MinY - height - kicadfiles.MM(10.16)}
		found := false
		for radius := 0; radius <= 15 && !found; radius++ {
			for dy := -radius; dy <= radius && !found; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					if dx != -radius && dx != radius && dy != -radius && dy != radius {
						continue
					}
					at := kicadfiles.Point{X: preferred.X + kicadfiles.IU(dx)*kicadfiles.MM(5.08), Y: preferred.Y + kicadfiles.IU(dy)*kicadfiles.MM(5.08)}
					box := schematiclayout.Rect{MinX: at.X, MinY: at.Y, MaxX: at.X + width, MaxY: at.Y + height}
					if !builder.nativeTextClear(box, occupied) {
						continue
					}
					for i, line := range lines {
						w := schematiclayout.NativeFieldBounds(line, kicadfiles.Point{}).Width()
						position := kicadfiles.Point{X: at.X + w/2, Y: at.Y + kicadfiles.IU(i)*row + row/2}
						builder.design.Schematic.Texts = append(builder.design.Schematic.Texts, schematic.Text{NativeV10: true, UUID: builder.generator.New("root.schematic.local-block", block.ID, fmt.Sprint(i), line), Value: line, Position: position})
					}
					occupied = append(occupied, box.Inflate(kicadfiles.MM(1.27)))
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("cannot place intact local annotation block %s within its bounded neighborhood", block.ID)
		}
	}
	return nil
}
