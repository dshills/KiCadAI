package writercorrectness

import (
	"path/filepath"
	"testing"
)

func TestCheckPCBFootprintPadsSkipsWithoutPCB(t *testing.T) {
	_, checks := CheckPCBFootprintPads(Target{})
	if len(checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(checks))
	}
	for _, check := range checks {
		if check.Status != CheckSkipped {
			t.Fatalf("check = %#v, want skipped", check)
		}
	}
}

func TestCheckPCBFootprintPadsPassesValidBoard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (net 1 "SIG") (uuid "11111111-1111-1111-1111-111111111114"))
  )
`))

	snapshot, checks := CheckPCBFootprintPads(Target{PCBPath: path})
	for _, check := range checks {
		if check.Status == CheckFail {
			t.Fatalf("check failed: %#v", checks)
		}
	}
	if snapshot.FootprintCount != 1 || snapshot.PadCount != 1 || snapshot.NetCount != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCheckPCBFootprintPadsReportsWrongPadNet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (net 2 "MISSING") (uuid "11111111-1111-1111-1111-111111111114"))
  )
`))

	_, checks := CheckPCBFootprintPads(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "missing net code")
}

func TestCheckPCBFootprintPadsAllowsNoNetPad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (uuid "11111111-1111-1111-1111-111111111114"))
  )
`))

	_, checks := CheckPCBFootprintPads(Target{PCBPath: path})
	for _, check := range checks {
		if check.Status == CheckFail {
			t.Fatalf("check failed for no-net pad: %#v", checks)
		}
	}
}

func TestCheckPCBFootprintPadsReportsDuplicateFootprintRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	footprint := `
  (footprint "Connector_Test:TH_1x01" (layer "F.Cu") (at 10 10)
    (uuid "11111111-1111-1111-1111-111111111111")
    (path "/11111111-1111-1111-1111-111111111111")
    (property "Reference" "J1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-1111-1111-111111111112"))
    (property "Value" "IN" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-1111-1111-111111111113"))
    (pad "1" thru_hole circle (at 0 0) (size 1.5 1.5) (drill 0.8) (layers "*.Cu" "*.Mask") (net 1 "SIG") (uuid "11111111-1111-1111-1111-111111111114"))
  )
`
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, footprint+footprint))

	_, checks := CheckPCBFootprintPads(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "duplicate PCB footprint reference")
}

func TestCheckPCBFootprintPadsReportsParseFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.kicad_pcb")
	writeFile(t, path, `(not_a_pcb)`)

	_, checks := CheckPCBFootprintPads(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "expected kicad_pcb root")
}

func pcbFixture(extraNets string, body string) string {
	return `(kicad_pcb
  (version 20260206)
  (generator "kicadai-test")
  (generator_version "0.0.0")
  (general (thickness 1.6))
  (paper "A4")
  (layers
    (0 "F.Cu" signal)
    (31 "B.Cu" signal)
    (32 "B.Adhes" user)
    (33 "F.Adhes" user)
    (34 "B.Paste" user)
    (35 "F.Paste" user)
    (36 "B.SilkS" user)
    (37 "F.SilkS" user)
    (38 "B.Mask" user)
    (39 "F.Mask" user)
    (44 "Edge.Cuts" user)
  )
  (net 0 "")
  ` + extraNets + `
` + body + `
)`
}
