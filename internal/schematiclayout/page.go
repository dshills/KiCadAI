package schematiclayout

import (
	"strings"

	"kicadai/internal/kicadfiles"
)

type standardPaper struct {
	name   string
	width  float64
	height float64
}

var standardPapers = []standardPaper{
	{name: "A5", width: 210, height: 148},
	{name: "A4", width: 297, height: 210},
	{name: "A3", width: 420, height: 297},
	{name: "A2", width: 594, height: 420},
	{name: "A1", width: 841, height: 594},
	{name: "A0", width: 1189, height: 841},
}

// SheetForPaper returns the standard KiCad sheet geometry for a paper name in
// the default landscape orientation.
func SheetForPaper(name string) Sheet {
	return SheetForPaperOrientation(name, false)
}

// SheetForPaperOrientation returns standard KiCad sheet geometry while
// preserving a caller's portrait-orientation choice.
func SheetForPaperOrientation(name string, portrait bool) Sheet {
	trimmed := strings.ToUpper(strings.TrimSpace(name))
	if trimmed == "" {
		trimmed = "A4"
	}
	for _, paper := range standardPapers {
		if paper.name == trimmed {
			width, height := paper.width, paper.height
			if portrait {
				width, height = height, width
			}
			return Sheet{Name: paper.name, Width: kicadfiles.MM(width), Height: kicadfiles.MM(height), Margin: kicadfiles.MM(10.16)}
		}
	}
	width, height := 297.0, 210.0
	if portrait {
		width, height = height, width
	}
	return Sheet{Name: trimmed, Width: kicadfiles.MM(width), Height: kicadfiles.MM(height), Margin: kicadfiles.MM(10.16)}
}

func pageCandidates(requested Sheet) []Sheet {
	name := strings.ToUpper(strings.TrimSpace(requested.Name))
	start := -1
	for index, paper := range standardPapers {
		if paper.name == name {
			start = index
			break
		}
	}
	if start < 0 {
		for index, paper := range standardPapers {
			if (kicadfiles.MM(paper.width) == requested.Width && kicadfiles.MM(paper.height) == requested.Height) ||
				(kicadfiles.MM(paper.height) == requested.Width && kicadfiles.MM(paper.width) == requested.Height) {
				start = index
				name = paper.name
				break
			}
		}
	}
	if start < 0 {
		return []Sheet{requested}
	}

	landscape := requested.Width >= requested.Height
	candidates := make([]Sheet, 0, len(standardPapers)-start)
	for _, paper := range standardPapers[start:] {
		width, height := paper.width, paper.height
		if !landscape {
			width, height = height, width
		}
		candidate := requested
		candidate.Name = paper.name
		candidate.Width = kicadfiles.MM(width)
		candidate.Height = kicadfiles.MM(height)
		if candidate.Margin <= 0 {
			candidate.Margin = kicadfiles.MM(10.16)
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func hasPageOverflow(result Result) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "page_overflow" || diagnostic.Code == "outside_sheet" {
			return true
		}
	}
	return false
}
