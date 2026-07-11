package writercorrectness

import (
	"path/filepath"
	"testing"

	"kicadai/internal/libraryresolver"
)

func TestCheckSchematicPCBTransferSkipsWithoutSchematic(t *testing.T) {
	snapshot, check := CheckSchematicToPCBTransfer(Target{})
	if snapshot.Confidence != TransferConfidenceUnknown {
		t.Fatalf("confidence = %q", snapshot.Confidence)
	}
	if check.Status != CheckSkipped {
		t.Fatalf("status = %q, want skipped", check.Status)
	}
}

func TestCheckSchematicPCBTransferUsesLibraryIndex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeFile(t, filepath.Join(dir, "demo.kicad_sch"), schematicWithBody(`
  (symbol (lib_id "Connector:Conn_01x02") (at 10 10 0)
    (property "Reference" "J1" (at 10 10 0))
    (property "Value" "IN" (at 10 12 0))
    (property "Footprint" "Connector_Test:Exact" (at 10 14 0) hide)
  )
`))
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Connector_Test:Exact": {FootprintID: "Connector_Test:Exact"},
	}}
	_, check := CheckSchematicToPCBTransferWithOptions(Target{
		ProjectDir:    dir,
		ProjectPath:   filepath.Join(dir, "demo.kicad_pro"),
		SchematicPath: filepath.Join(dir, "demo.kicad_sch"),
	}, Options{LibraryIndex: index, HasLibraryIndex: true})
	for _, issue := range check.Issues {
		if issue.Code == "UNKNOWN_FOOTPRINT_LIBRARY" {
			t.Fatalf("library index was not used: %#v", check.Issues)
		}
	}
}

func TestCheckSchematicPCBTransferPlacesAssignedFootprint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_pro"), "{}")
	writeFile(t, filepath.Join(dir, "demo.kicad_sch"), schematicWithBody(`
  (symbol (lib_id "Connector:Conn_01x02") (at 10 10 0)
    (property "Reference" "J1" (at 10 10 0))
    (property "Value" "IN" (at 10 12 0))
    (property "Footprint" "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical" (at 10 14 0) hide)
  )
`))

	snapshot, check := CheckSchematicToPCBTransfer(Target{
		ProjectDir:    dir,
		ProjectPath:   filepath.Join(dir, "demo.kicad_pro"),
		SchematicPath: filepath.Join(dir, "demo.kicad_sch"),
	})
	if check.Status == CheckFail {
		t.Fatalf("check failed: %#v", check.Issues)
	}
	if snapshot.AssignedCount != 1 || snapshot.PlacedCount != 1 || len(snapshot.Placements) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Placements[0].Reference != "J1" {
		t.Fatalf("placement = %#v", snapshot.Placements[0])
	}
}

func TestCheckSchematicPCBTransferFailsWithoutFootprints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "demo.kicad_sch"), schematicWithBody(`
  (symbol (lib_id "Device:R") (at 10 10 0)
    (property "Reference" "R1" (at 10 10 0))
    (property "Value" "1k" (at 10 12 0))
  )
`))

	_, check := CheckSchematicToPCBTransfer(Target{SchematicPath: filepath.Join(dir, "demo.kicad_sch")})
	if check.Status != CheckFail {
		t.Fatalf("status = %q issues = %#v", check.Status, check.Issues)
	}
	assertCheckIssueContains(t, []CheckResult{check}, "has no assigned footprint")
}
