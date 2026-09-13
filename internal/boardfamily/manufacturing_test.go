package boardfamily

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/sexpr"
)

func testManufacturingBoard(t *testing.T) nativeNode {
	t.Helper()
	b, err := sexpr.Parse([]byte(`(kicad_pcb
  (via (at 1 2) (drill 0.3))
  (footprint "Lib:Part" (layer "F.Cu") (at 10 20 90)
    (property "Reference" "J1") (property "Value" "Header")
    (pad "1" thru_hole circle (at 2 0) (drill 1))
    (pad "" np_thru_hole circle (at 0 3) (drill 2)))
)`))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeManufacturingTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementBindsEveryNativeFootprint(t *testing.T) {
	board := testManufacturingBoard(t)
	header := "Ref,Val,Package,PosX,PosY,Rot,Side\n"
	valid := "J1,Header,Part,10,-20,90,top\n"
	for name, rows := range map[string]string{
		"valid":   valid,
		"missing": "", "duplicate": valid + valid,
		"wrong reference": "J2,Header,Part,10,-20,90,top\n",
		"wrong value":     "J1,Other,Part,10,-20,90,top\n",
		"wrong package":   "J1,Header,Other,10,-20,90,top\n",
		"wrong side":      "J1,Header,Part,10,-20,90,bottom\n",
		"mirrored Y":      "J1,Header,Part,10,20,90,top\n",
		"wrong rotation":  "J1,Header,Part,10,-20,0,top\n",
		"nan":             "J1,Header,Part,NaN,-20,90,top\n",
		"missing column":  "J1,Header,Part,10,-20,90\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "placement.csv")
			writeManufacturingTestFile(t, path, header+rows)
			err := verifyPlacement(path, board)
			if (err == nil) != (name == "valid") {
				t.Fatalf("unexpected placement result: %v", err)
			}
		})
	}
}

func TestDrillsBindTransformedPadsAndVias(t *testing.T) {
	board := testManufacturingBoard(t)
	// At +90 degrees native local +X maps to negative board Y. Then drill Y
	// reverses native Y: pad (2,0) at (10,20) becomes drill (10,-18).
	header := "M48\n; #@! TF.FileFunction,Plated,1,2,PTH\nMETRIC\nT1C0.300\nT2C1.000\n%\nG90\nG05\n"
	valid := "T1\nX1Y-2\nT2\nX10Y-18\nM30\n"
	npth := "M48\n; #@! TF.FileFunction,NonPlated,1,2,NPTH\nMETRIC\nT1C2.000\n%\nG90\nG05\nT1\nX13Y-20\nM30\n"
	for name, content := range map[string]string{
		"valid":          header + valid,
		"wrong position": header + strings.Replace(valid, "X10Y-18", "X10Y-22", 1),
		"missing via":    header + strings.Replace(valid, "X1Y-2\n", "", 1),
		"duplicate via":  header + strings.Replace(valid, "X1Y-2\n", "X1Y-2\nX1Y-2\n", 1),
		"wrong diameter": strings.Replace(header, "0.300", "0.350", 1) + valid,
		"undefined tool": header + strings.Replace(valid, "T1\n", "T9\n", 1),
		"wrong units":    strings.Replace(header, "METRIC", "INCH", 1) + valid,
		"wrong plating":  strings.Replace(header, "Plated,1,2,PTH", "NonPlated,1,2,NPTH", 1) + valid,
		"truncated":      header + strings.Replace(valid, "M30\n", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeManufacturingTestFile(t, filepath.Join(dir, "board-PTH.drl"), content)
			writeManufacturingTestFile(t, filepath.Join(dir, "board-NPTH.drl"), npth)
			err := verifyDrills(dir, board)
			if (err == nil) != (name == "valid") {
				t.Fatalf("unexpected drill result: %v", err)
			}
		})
	}
}

func TestDrillComparisonAllowsOnlySerializationPrecision(t *testing.T) {
	want := []drillHit{{77.0875, -28, .3}}
	for _, tc := range []struct {
		x     float64
		valid bool
	}{{77.087, true}, {77.088, true}, {77.086, false}, {77.089, false}} {
		missing, extra := compareDrillHits(want, []drillHit{{tc.x, -28, .3}})
		if (len(missing) == 0 && len(extra) == 0) != tc.valid {
			t.Fatalf("wrong precision boundary for %f", tc.x)
		}
	}
	if missing, extra := compareDrillHits(want, []drillHit{{77.087, -28, .3}, {77.087, -28, .3}}); len(missing) != 0 || len(extra) != 1 {
		t.Fatal("duplicate drill escaped")
	}
}
