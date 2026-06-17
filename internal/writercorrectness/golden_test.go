package writercorrectness

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestGoldenWriterCorpus(t *testing.T) {
	cases := []struct {
		name              string
		build             func(t *testing.T) string
		wantOK            bool
		wantBlockingCodes []reports.Code
	}{
		{
			name:   "schematic_power_symbol",
			build:  buildGoldenSchematicPowerOnly,
			wantOK: true,
		},
		{
			name:   "pcb_footprint_pad_track_zone",
			build:  buildGoldenPCB,
			wantOK: true,
		},
		{
			name:              "pcb_wrong_pad_net",
			build:             buildGoldenWrongPadNet,
			wantOK:            false,
			wantBlockingCodes: []reports.Code{reports.CodeValidationFailed, reports.CodeInvalidNetAssignment},
		},
		{
			name:              "schematic_missing_footprint",
			build:             buildGoldenMissingFootprint,
			wantOK:            false,
			wantBlockingCodes: []reports.Code{reports.CodeMissingFootprint, reports.CodeMissingFootprint},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.build(t)
			result := Validate(context.Background(), target, Options{})
			if result.OK != tt.wantOK {
				t.Fatalf("OK = %v, want %v; issues = %#v", result.OK, tt.wantOK, result.Issues)
			}
			gotCodes := blockingCodes(result)
			if !sameCodes(gotCodes, tt.wantBlockingCodes) {
				t.Fatalf("blocking codes = %#v, want %#v; issues = %#v", gotCodes, tt.wantBlockingCodes, result.Issues)
			}
		})
	}
}

func buildGoldenSchematicPowerOnly(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "golden_power.kicad_pro"), "{}")
	writeFile(t, filepath.Join(dir, "golden_power.kicad_sch"), schematicWithBody(`
  (symbol (lib_id "power:GND") (at 10 10 0)
    (property "Reference" "#PWR01" (at 10 10 0))
    (property "Value" "GND" (at 10 12 0))
  )
`))
	return filepath.Join(dir, "golden_power.kicad_sch")
}

func buildGoldenPCB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "golden_pcb.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (net 1 "SIG") (uuid "11111111-1111-1111-1111-111111111114"))
  )
  (segment (start 10 10) (end 20 10) (width 0.25) (layer "F.Cu") (net 1 "SIG") (uuid "11111111-1111-1111-1111-111111111115"))
  (zone (net 1) (net_name "SIG") (layers "F.Cu")
    (uuid "11111111-1111-1111-1111-111111111116")
    (polygon (pts (xy 0 0) (xy 10 0) (xy 10 10) (xy 0 0)))
  )
`))
	return path
}

func buildGoldenWrongPadNet(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_pad.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (net 2 "MISSING") (uuid "11111111-1111-1111-1111-111111111114"))
  )
`))
	return path
}

func buildGoldenMissingFootprint(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "missing_fp.kicad_sch"), schematicWithBody(`
  (symbol (lib_id "Device:R") (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "1k" (at 10 12 0))
  )
`))
	return filepath.Join(dir, "missing_fp.kicad_sch")
}

func blockingCodes(result Result) []reports.Code {
	var codes []reports.Code
	for _, issue := range result.Issues {
		if issue.Blocking() {
			codes = append(codes, issue.Code)
		}
	}
	return codes
}

func sameCodes(got []reports.Code, want []reports.Code) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
