package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestChooseTextPositionUsesClearCornerWhenCardinalSidesAreWired(t *testing.T) {
	body := Rect{
		MinX: -kicadfiles.MM(2.54),
		MinY: -kicadfiles.MM(2.54),
		MaxX: kicadfiles.MM(2.54),
		MaxY: kicadfiles.MM(2.54),
	}
	occupied := []Rect{
		body,
		{MinX: -kicadfiles.MM(30), MinY: -kicadfiles.MM(0.635), MaxX: kicadfiles.MM(30), MaxY: kicadfiles.MM(0.635)},
		{MinX: -kicadfiles.MM(0.635), MinY: -kicadfiles.MM(30), MaxX: kicadfiles.MM(0.635), MaxY: kicadfiles.MM(30)},
	}

	field, clean := chooseTextPosition("VALUE", kicadfiles.Point{}, body, occupied, DefaultRules(ProfileStandard), false)
	if !clean {
		t.Fatalf("corner placement reported a crowded fallback: %#v", field)
	}
	if rectIntersectsAny(field.Box, occupied) {
		t.Fatalf("corner placement intersects occupied geometry: %#v", field.Box)
	}
}
