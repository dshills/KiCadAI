package writercorrectness

import (
	"path/filepath"
	"testing"
)

func TestCheckPCBCopperZonesSkipsWithoutPCB(t *testing.T) {
	_, checks := CheckPCBCopperZones(Target{})
	if len(checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(checks))
	}
	for _, check := range checks {
		if check.Status != CheckSkipped {
			t.Fatalf("check = %#v, want skipped", check)
		}
	}
}

func TestCheckPCBCopperZonesPassesValidCopper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (segment (start 0 0) (end 10 0) (width 0.25) (layer "F.Cu") (net 1))
  (via (at 10 0) (size 0.8) (drill 0.4) (layers "F.Cu" "B.Cu") (net 1))
  (zone (net 1) (net_name "SIG") (layers "F.Cu")
    (polygon (pts (xy 0 0) (xy 10 0) (xy 10 10) (xy 0 0)))
  )
`))

	snapshot, checks := CheckPCBCopperZones(Target{PCBPath: path})
	for _, check := range checks {
		if check.Status == CheckFail {
			t.Fatalf("check failed: %#v", checks)
		}
	}
	if snapshot.TrackCount != 1 || snapshot.ViaCount != 1 || snapshot.ZoneCount != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCheckPCBCopperZonesReportsWrongTrackNet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (segment (start 0 0) (end 10 0) (width 0.25) (layer "F.Cu") (net 2))
`))

	_, checks := CheckPCBCopperZones(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "missing net code")
}

func TestCheckPCBCopperZonesReportsViaAnnularRing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (via (at 10 0) (size 0.5) (drill 0.45) (layers "F.Cu" "B.Cu") (net 1))
`))

	_, checks := CheckPCBCopperZones(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "annular ring")
}

func TestCheckPCBCopperZonesReportsUnclosedZone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_pcb")
	writeFile(t, path, pcbFixture(`(net 1 "SIG")`, `
  (zone (net 1) (net_name "SIG") (layers "F.Cu")
    (polygon (pts (xy 0 0) (xy 10 0) (xy 10 10)))
  )
`))

	_, checks := CheckPCBCopperZones(Target{PCBPath: path})
	assertCheckIssueContains(t, checks, "zone polygon must be closed")
}
