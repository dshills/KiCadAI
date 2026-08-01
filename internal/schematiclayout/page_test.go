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

func TestLayoutChoosesPortraitBeforeEscalatingTallDrawing(t *testing.T) {
	request := Request{
		Sheet: SheetForPaper("A4"),
		Rules: DefaultRules(ProfileStandard),
	}
	for index := 0; index < 9; index++ {
		request.Components = append(request.Components, Component{
			Ref: "R" + fmt.Sprint(index+1), Role: "resistor", Fixed: true,
			Position: kicadfiles.Point{X: kicadfiles.MM(100), Y: kicadfiles.MM(30 + float64(index)*28)},
		})
	}
	result := Layout(request)
	if result.Sheet.Name != "A4" || result.Sheet.Width >= result.Sheet.Height {
		t.Fatalf("tall drawing selected %#v, want A4 portrait before escalation", result.Sheet)
	}
}

func TestStandardTitleBlockReservationShrinksUsableHeight(t *testing.T) {
	plain := SheetForPaper("A4")
	reserved := SheetWithStandardTitleBlock(plain)
	plainUsable := UsableSheet(plain)
	reservedUsable := UsableSheet(reserved)
	if reserved.TitleBlock.Empty() {
		t.Fatal("standard title block was not populated")
	}
	if reservedUsable.MaxY >= reserved.TitleBlock.MinY {
		t.Fatalf("usable sheet overlaps title block: usable=%#v title=%#v", reservedUsable, reserved.TitleBlock)
	}
	if reservedUsable.Height() >= plainUsable.Height() {
		t.Fatalf("reserved usable height = %v, want less than plain height %v", reservedUsable.Height(), plainUsable.Height())
	}
}

func TestPageCandidatesReanchorReservedTitleBlock(t *testing.T) {
	requested := SheetWithStandardTitleBlock(SheetForPaper("A4"))
	for _, candidate := range pageCandidates(requested) {
		if candidate.TitleBlock.Empty() {
			t.Fatalf("candidate %s omitted reserved title block", candidate.Name)
		}
		if candidate.TitleBlock.MaxX != candidate.Width-candidate.Margin || candidate.TitleBlock.MaxY != candidate.Height-candidate.Margin {
			t.Fatalf("candidate %s title block is not anchored to lower right: sheet=%#v title=%#v", candidate.Name, candidate, candidate.TitleBlock)
		}
	}
}
