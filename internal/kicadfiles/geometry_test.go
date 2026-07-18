package kicadfiles

import (
	"math"
	"testing"
)

func TestRotateBoardLocalXYUsesKiCadCanvasConvention(t *testing.T) {
	tests := []struct {
		degrees      float64
		wantX, wantY float64
	}{
		{degrees: 90 + 1e-12, wantX: 0, wantY: -2},
		{degrees: 180, wantX: -2, wantY: 0},
		{degrees: 270, wantX: 0, wantY: 2},
		{degrees: 45, wantX: math.Sqrt2, wantY: -math.Sqrt2},
	}
	for _, test := range tests {
		gotX, gotY := RotateBoardLocalXY(2, 0, test.degrees)
		if math.Abs(gotX-test.wantX) > 1e-9 || math.Abs(gotY-test.wantY) > 1e-9 {
			t.Fatalf("RotateBoardLocalXY(2, 0, %g) = (%g, %g), want (%g, %g)", test.degrees, gotX, gotY, test.wantX, test.wantY)
		}
	}
}
