package designapi

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/schematiclayout"
)

func (builder *Builder) placeNativeReadingGuide(occupied []schematiclayout.Rect) error {
	var lines []string
	for _, note := range builder.nativeSchematicNotes {
		for _, paragraph := range strings.Split(note, "\n") {
			line := ""
			for _, word := range strings.Fields(paragraph) {
				if utf8.RuneCountInString(word) > 64 {
					return fmt.Errorf("native reading guide contains an overlong token")
				}
				if utf8.RuneCountInString(line)+utf8.RuneCountInString(word)+1 > 64 {
					lines = append(lines, line)
					line = ""
				}
				if line != "" {
					line += " "
				}
				line += word
			}
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	if len(lines) > 512 {
		return fmt.Errorf("native reading guide exceeds 512 lines")
	}
	for offset := 0; offset < len(lines); {
		block := lines[offset:min(offset+18, len(lines))]
		width := kicadfiles.IU(0)
		for _, line := range block {
			width = max(width, schematiclayout.NativeFieldBounds(line, kicadfiles.Point{}).Width())
		}
		row := kicadfiles.MM(2.54)
		height := kicadfiles.IU(len(block)) * row
		paper := nativeAnnotationPaper(builder.design.Schematic.Paper)
		found := false
		for y := kicadfiles.MM(7.62); y+height < paper.Height-kicadfiles.MM(7.62) && !found; y += kicadfiles.MM(5.08) {
			for x := kicadfiles.MM(7.62); x+width < paper.Width-kicadfiles.MM(7.62); x += kicadfiles.MM(5.08) {
				box := schematiclayout.Rect{MinX: x, MinY: y, MaxX: x + width, MaxY: y + height}
				if !builder.nativeTextClear(box, occupied) {
					continue
				}
				for i, line := range block {
					w := schematiclayout.NativeFieldBounds(line, kicadfiles.Point{}).Width()
					at := kicadfiles.Point{X: x + w/2, Y: y + kicadfiles.IU(i)*row + row/2}
					builder.design.Schematic.Texts = append(builder.design.Schematic.Texts, schematic.Text{NativeV10: true, UUID: builder.generator.New("root.schematic.reading-guide", fmt.Sprint(offset+i), line), Value: line, Position: at})
				}
				occupied = append(occupied, box.Inflate(kicadfiles.MM(1.27)))
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("native reading guide cannot place block at line %d", offset)
		}
		offset += len(block)
	}
	return nil
}
