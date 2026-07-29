package pcb

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFableH3ReproductionImportedBoardMetadataIsReset(t *testing.T) {
	board := readFableImportedPCB(t)
	if board.General.Thickness != DefaultGeneral().Thickness {
		t.Fatalf("general thickness = %v, want current default %v", board.General.Thickness, DefaultGeneral().Thickness)
	}
	if board.TitleBlock.Title != "" {
		t.Fatalf("title block unexpectedly survived import: %#v", board.TitleBlock)
	}
	if board.Setup.PadToMaskClearance != DefaultSetup().PadToMaskClearance {
		t.Fatalf("setup mask clearance = %v, want current default %v", board.Setup.PadToMaskClearance, DefaultSetup().PadToMaskClearance)
	}
}

func TestFableH4ReproductionImportedFootprintChildrenAreDropped(t *testing.T) {
	board := readFableImportedPCB(t)
	if len(board.Footprints) != 1 {
		t.Fatalf("footprints = %d, want 1", len(board.Footprints))
	}
	footprint := board.Footprints[0]
	if footprint.Description != "" || footprint.Tags != "" || footprint.Locked || len(footprint.Texts) != 0 || len(footprint.Models) != 0 {
		t.Fatalf("unmodeled footprint children unexpectedly survived import: %#v", footprint)
	}
}

func TestFableH5ReproductionImportedKeepoutBecomesCopperZone(t *testing.T) {
	board := readFableImportedPCB(t)
	if len(board.Zones) != 1 {
		t.Fatalf("zones = %d, want 1", len(board.Zones))
	}
	if board.Zones[0].Keepout != nil {
		t.Fatalf("keepout unexpectedly survived import: %#v", board.Zones[0].Keepout)
	}
	var output bytes.Buffer
	if err := Write(&output, board); err != nil {
		t.Fatal(err)
	}
	rewritten, err := Read(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rewritten.Zones) != 1 || rewritten.Zones[0].Keepout != nil || rewritten.Zones[0].NetCode != 0 {
		t.Fatalf("rewritten zone did not reproduce keepout-to-copper conversion: %#v", rewritten.Zones)
	}
}

func readFableImportedPCB(t *testing.T) PCBFile {
	t.Helper()
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.3")`,
		`  (general (thickness 1.0))`,
		`  (paper "A4")`,
		`  (title_block (title "Fable preservation fixture"))`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup (pad_to_mask_clearance 0.123))`,
		`  (net 0 "")`,
		`  (footprint "Test:Imported"`,
		`    (locked yes)`,
		`    (layer "F.Cu")`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10 0)`,
		`    (descr "preserve this description")`,
		`    (tags "preserve these tags")`,
		`    (property "Reference" "U1" (at 10 8 0) (layer "F.SilkS") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`    (property "Value" "Imported" (at 10 12 0) (layer "F.Fab") (uuid "33333333-3333-5333-8333-333333333333"))`,
		`    (path "/11111111-1111-5111-8111-111111111111")`,
		`    (fp_text user "KEEP_ME" (at 0 0) (layer "F.SilkS") (uuid "44444444-4444-5444-8444-444444444444"))`,
		`    (model "${KICAD9_3DMODEL_DIR}/Test.step" (offset (xyz 0 0 0)) (scale (xyz 1 1 1)) (rotate (xyz 0 0 0)))`,
		`  )`,
		`  (zone`,
		`    (layers "F.Cu")`,
		`    (uuid "55555555-5555-5555-8555-555555555555")`,
		`    (hatch edge 0.5)`,
		`    (keepout (tracks not_allowed) (vias not_allowed) (pads not_allowed) (copperpour not_allowed) (footprints not_allowed))`,
		`    (polygon (pts (xy 0 0) (xy 5 0) (xy 5 5) (xy 0 0)))`,
		`  )`,
		`)`,
	}, "\n")
	board, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if board.General.Thickness == kicadfiles.MM(1.0) {
		t.Fatal("H3 reproduction no longer observes metadata loss; replace it with the preservation invariant")
	}
	return board
}
