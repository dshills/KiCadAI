package boardfamily

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/sexpr"
)

const manufacturingTestBoard = `(kicad_pcb
  (via (at 1 2) (drill 0.3))
  (footprint "Lib:Part" (layer "F.Cu") (at 10 20 90)
    (property "Reference" "J1") (property "Value" "Header")
    (pad "1" thru_hole circle (at 2 0) (drill 1))
    (pad "" np_thru_hole circle (at 0 3) (drill 2)))
)`

func testManufacturingBoard(t *testing.T) nativeNode {
	t.Helper()
	b, err := sexpr.Parse([]byte(manufacturingTestBoard))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testGerberHeader(function string) string {
	function = strings.Replace(function, "SolderPaste,", "Paste,", 1)
	function = strings.Replace(function, "SolderMask,", "Soldermask,", 1)
	polarity := "%TF.FilePolarity,Positive*%\n"
	if strings.HasPrefix(function, "Soldermask,") {
		polarity = "%TF.FilePolarity,Negative*%\n"
	}
	if function == "Profile" {
		function, polarity = "Profile,NP", ""
	}
	return "%TF.FileFunction," + function + "*%\n" + polarity + "%FSLAX46Y46*%\n%MOMM*%\nM02*\n"
}

func TestGerberDeclarationsRejectConflicts(t *testing.T) {
	for name, function := range gerberFunctions {
		t.Run(name, func(t *testing.T) {
			valid := testGerberHeader(function)
			for id, content := range map[string]string{
				"valid":              valid,
				"wrong identity":     strings.Replace(valid, "%TF.FileFunction,", "%TF.FileFunction,Wrong,", 1),
				"suffix identity":    strings.Replace(valid, "*%\n", ",Wrong*%\n", 1),
				"duplicate identity": "%TF.FileFunction,Other*%\n" + valid,
				"duplicate units":    "%MOMM*%\n" + valid,
				"conflicting units":  "%MOIN*%\n" + valid,
				"missing units":      strings.Replace(valid, "%MOMM*%\n", "", 1),
				"wrong format":       strings.Replace(valid, "%FSLAX46Y46*%", "%FSLIX46Y46*%", 1),
				"duplicate format":   "%FSLAX46Y46*%\n" + valid,
				"duplicate polarity": "%TF.FilePolarity,Negative*%\n" + valid,
				"early end":          "M02*\n" + valid,
				"truncated":          strings.Replace(valid, "M02*\n", "", 1),
				"data after end":     valid + "X1Y1D03*\n",
				"stop":               strings.Replace(valid, "M02", "M00*\nM02", 1),
				"legacy end":         strings.Replace(valid, "M02", "M2*\nM02", 1),
				"legacy inch":        strings.Replace(valid, "M02", "G70*\nM02", 1),
				"incremental":        strings.Replace(valid, "M02", "G91*\nM02", 1),
			} {
				t.Run(id, func(t *testing.T) {
					if err := verifyGerberFraming(content, function); (err == nil) != (id == "valid") {
						t.Fatalf("unexpected Gerber result: %v", err)
					}
				})
			}
		})
	}
}

func TestManufacturingJobAndArtifactSet(t *testing.T) {
	for _, id := range []string{"valid", "missing layer", "wrong layer header", "wrong layer count", "wrong thickness", "wrong width", "wrong height", "missing job file", "extra job file", "duplicate job file", "wrong job function", "truncated job", "missing placement", "missing drill"} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "manufacturing")
			for _, sub := range []string{"gerbers", "drill"} {
				if err := os.MkdirAll(filepath.Join(out, sub), 0700); err != nil {
					t.Fatal(err)
				}
			}
			files := []map[string]string{}
			for name, function := range gerberFunctions {
				if id != "missing layer" || name != "board-F_Cu.gbr" {
					header := testGerberHeader(function)
					if id == "wrong layer header" && name == "board-F_Cu.gbr" {
						header = testGerberHeader("Copper,L2,Bot")
					}
					writeManufacturingTestFile(t, filepath.Join(out, "gerbers", name), header)
				}
				files = append(files, map[string]string{"Path": name, "FileFunction": function})
			}
			spec := map[string]any{"LayerNumber": 2, "BoardThickness": 1.6, "Size": map[string]float64{"X": 120.1, "Y": 80.1}}
			switch id {
			case "wrong layer count":
				spec["LayerNumber"] = 4
			case "wrong thickness":
				spec["BoardThickness"] = 1.2
			case "wrong width":
				spec["Size"].(map[string]float64)["X"] = 120
			case "wrong height":
				spec["Size"].(map[string]float64)["Y"] = 80
			case "missing job file":
				files = files[1:]
			case "extra job file":
				files = append(files, map[string]string{"Path": "other.gbr", "FileFunction": "Other"})
			case "duplicate job file":
				files = append(files, files[0])
			case "wrong job function":
				files[0]["FileFunction"] = "Other"
			}
			job, err := json.Marshal(map[string]any{"GeneralSpecs": spec, "FilesAttributes": files})
			if err != nil {
				t.Fatal(err)
			}
			if id == "truncated job" {
				job = job[:len(job)-1]
			}
			writeManufacturingTestFile(t, filepath.Join(out, "gerbers", "board-job.gbrjob"), string(job))
			writeManufacturingTestFile(t, filepath.Join(dir, "board.kicad_pcb"), manufacturingTestBoard)
			if id != "missing placement" {
				writeManufacturingTestFile(t, filepath.Join(out, "placement.csv"), "Ref,Val,Package,PosX,PosY,Rot,Side\nJ1,Header,Part,10,-20,90,top\n")
			}
			if id != "missing drill" {
				writeManufacturingTestFile(t, filepath.Join(out, "drill", "board-PTH.drl"), "M48\n; #@! TF.FileFunction,Plated,1,2,PTH\nFMAT,2\nMETRIC\nT1C0.300\nT2C1.000\n%\nG90\nG05\nT1\nX1Y-2\nT2\nX10Y-18\nM30\n")
			}
			writeManufacturingTestFile(t, filepath.Join(out, "drill", "board-NPTH.drl"), "M48\n; #@! TF.FileFunction,NonPlated,1,2,NPTH\nFMAT,2\nMETRIC\nT1C2.000\n%\nG90\nG05\nT1\nX13Y-20\nM30\n")
			if err := verifyManufacturing(dir); (err == nil) != (id == "valid") {
				t.Fatalf("unexpected manufacturing result: %v", err)
			}
		})
	}
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
	header := "M48\n; #@! TF.FileFunction,Plated,1,2,PTH\nFMAT,2\nMETRIC\nT1C0.300\nT2C1.000\n%\nG90\nG05\n"
	valid := "T1\nX1Y-2\nT2\nX10Y-18\nM30\n"
	npth := "M48\n; #@! TF.FileFunction,NonPlated,1,2,NPTH\nFMAT,2\nMETRIC\nT1C2.000\n%\nG90\nG05\nT1\nX13Y-20\nM30\n"
	for name, content := range map[string]string{
		"valid":                     header + valid,
		"wrong position":            header + strings.Replace(valid, "X10Y-18", "X10Y-22", 1),
		"missing via":               header + strings.Replace(valid, "X1Y-2\n", "", 1),
		"duplicate via":             header + strings.Replace(valid, "X1Y-2\n", "X1Y-2\nX1Y-2\n", 1),
		"wrong diameter":            strings.Replace(header, "0.300", "0.350", 1) + valid,
		"undefined tool":            header + strings.Replace(valid, "T1\n", "T9\n", 1),
		"wrong units":               strings.Replace(header, "METRIC", "INCH", 1) + valid,
		"wrong plating":             strings.Replace(header, "Plated,1,2,PTH", "NonPlated,1,2,NPTH", 1) + valid,
		"truncated":                 header + strings.Replace(valid, "M30\n", "", 1),
		"incremental mode":          header + "G91\n" + valid,
		"coordinate offset":         header + "G92X1Y1\n" + valid,
		"routed mode":               header + "G00X1Y-2\n" + valid,
		"repeat holes":              header + "R3X1Y0\n" + valid,
		"early terminator":          header + "M30\n" + valid,
		"early stop":                header + "M00\n" + valid,
		"tool redefinition":         header + "T1C0.300\n" + valid,
		"duplicate tool definition": strings.Replace(header, "T1C0.300\n", "T1C0.300\nT1C0.300\n", 1) + valid,
		"duplicate units":           strings.Replace(header, "METRIC\n", "METRIC\nMETRIC\n", 1) + valid,
		"duplicate plating":         strings.Replace(header, "METRIC\n", "; #@! TF.FileFunction,NonPlated,1,2,NPTH\nMETRIC\n", 1) + valid,
		"missing header end":        strings.Replace(header, "%\n", "", 1) + valid,
		"missing drill mode":        strings.Replace(header, "G05\n", "", 1) + valid,
		"late coordinate mode":      strings.Replace(header, "G90\n", "", 1) + strings.Replace(valid, "M30", "G90\nM30", 1),
		"point in header":           strings.Replace(header, "%\n", "T1\nX1Y-2\n%\n", 1) + strings.Replace(valid, "X1Y-2\n", "", 1),
		"undefined unused tool":     header + valid + "T9\nM30\n",
		"slot":                      header + "G85X1Y-2\n" + valid,
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
