package schematiclayout

import (
	"fmt"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestLayoutEscalatesPaperForWideGraph(t *testing.T) {
	request := Request{
		Sheet: SheetForPaper("A4"),
		Rules: DefaultRules(ProfileStandard),
	}
	for index := 0; index < 18; index++ {
		ref := fmt.Sprintf("U%d", index+1)
		request.Components = append(request.Components, Component{
			Ref:  ref,
			Role: "ic",
			Pins: []Pin{{Number: "1", Role: "output"}, {Number: "2", Role: "input"}},
		})
		if index == 0 {
			continue
		}
		request.Nets = append(request.Nets, Net{
			Name: fmt.Sprintf("N%d", index),
			Endpoints: []Endpoint{
				{Ref: fmt.Sprintf("U%d", index), Pin: "1"},
				{Ref: ref, Pin: "2"},
			},
		})
	}

	result := Layout(request)
	if result.Report.PageEscalationCount == 0 {
		t.Fatalf("page did not escalate: %#v", result.Report)
	}
	if result.Report.SelectedPaper == "" || result.Report.SelectedPaper == "A4" {
		t.Fatalf("selected paper = %q, want larger than A4", result.Report.SelectedPaper)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "page_overflow" || diagnostic.Code == "outside_sheet" || diagnostic.Code == "page_fit_exhausted" {
			t.Fatalf("escalated layout still has page diagnostic: %#v", diagnostic)
		}
	}
	usable := UsableSheet(result.Sheet)
	if !usable.ContainsRect(result.Report.OccupiedBounds) {
		t.Fatalf("occupied bounds %#v outside selected sheet %#v", result.Report.OccupiedBounds, usable)
	}
}

func TestSheetForPaperPreservesPortraitOrientationWhenEscalating(t *testing.T) {
	request := Request{
		Sheet:      Sheet{Name: "A4", Width: kicadfiles.MM(210), Height: kicadfiles.MM(297), Margin: kicadfiles.MM(10.16)},
		Components: []Component{{Ref: "R1", Role: "resistor"}},
	}
	result := Layout(request)
	if result.Sheet.Width >= result.Sheet.Height {
		t.Fatalf("selected sheet lost portrait orientation: %#v", result.Sheet)
	}
}

func TestSheetForPaperOrientationReturnsPortraitSheet(t *testing.T) {
	sheet := SheetForPaperOrientation("A3", true)
	if sheet.Width >= sheet.Height {
		t.Fatalf("portrait sheet = %#v", sheet)
	}
	if sheet.Width != kicadfiles.MM(297) || sheet.Height != kicadfiles.MM(420) {
		t.Fatalf("portrait A3 dimensions = %#v", sheet)
	}
}
