package pcb

import (
	"bytes"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
)

func TestFableH3ImportedBoardMetadataIsPreserved(t *testing.T) {
	board := readFableImportedPCB(t)
	if board.General.Thickness != kicadfiles.MM(1.0) {
		t.Fatalf("general thickness = %v, want 1.0 mm", board.General.Thickness)
	}
	if board.TitleBlock.Title != "Fable preservation fixture" {
		t.Fatalf("title block lost: %#v", board.TitleBlock)
	}
	if board.Setup.PadToMaskClearance != kicadfiles.MM(0.123) {
		t.Fatalf("setup mask clearance = %v, want 0.123 mm", board.Setup.PadToMaskClearance)
	}
	assertFableRoundTripPreserved(t, board, `(general (thickness 1.0))`, `(paper "User" 210 297)`,
		`(title_block (title "Fable preservation fixture"))`, `(stackup (thickness 1.0))`,
		`(pcbplotparams (layerselection 0x00010fc_ffffffff))`)
}

func TestFableH4ImportedFootprintChildrenArePreserved(t *testing.T) {
	board := readFableImportedPCB(t)
	if len(board.Footprints) != 1 {
		t.Fatalf("footprints = %d, want 1", len(board.Footprints))
	}
	footprint := board.Footprints[0]
	if footprint.Description != "preserve this description" || footprint.Tags != "preserve these tags" ||
		!footprint.Locked || len(footprint.Texts) != 1 || len(footprint.Models) != 1 {
		t.Fatalf("footprint metadata was not modeled: %#v", footprint)
	}
	assertFableRoundTripPreserved(t, board, `(locked yes)`, `(descr "preserve this description")`,
		`(tags "preserve these tags")`, `(fp_text user "KEEP_ME"`, `(effects (font (size 1 1)))`,
		`(model "${KICAD9_3DMODEL_DIR}/Test.step"`, `(solder_mask_margin 0.05)`, `(zone_connect 2)`)
}

func TestFableH5ImportedKeepoutRemainsKeepout(t *testing.T) {
	board := readFableImportedPCB(t)
	if len(board.Zones) != 1 {
		t.Fatalf("zones = %d, want 1", len(board.Zones))
	}
	if board.Zones[0].Keepout == nil || board.Zones[0].Keepout.CopperPour != "not_allowed" {
		t.Fatalf("keepout was not parsed: %#v", board.Zones[0].Keepout)
	}
	var output bytes.Buffer
	if err := Write(&output, board); err != nil {
		t.Fatal(err)
	}
	rewritten, err := Read(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(rewritten.Zones) != 1 || rewritten.Zones[0].Keepout == nil {
		t.Fatalf("rewritten zone changed semantic family: %#v", rewritten.Zones)
	}
}

func TestFablePhase3PreservationComparatorDetectsDefaultSubstitution(t *testing.T) {
	source := readFableImportedPCB(t)
	staged := source
	staged.RawGeneral = `(general (thickness 1.6))`
	if err := ValidatePreservedMutation(source, staged); err == nil || !strings.Contains(err.Error(), "general was substituted") {
		t.Fatalf("expected substituted-default evidence, got %v", err)
	}
}

func assertFableRoundTripPreserved(t *testing.T, board PCBFile, fragments ...string) {
	t.Helper()
	var output bytes.Buffer
	if err := Write(&output, board); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("round-trip output missing %q:\n%s", fragment, output.String())
		}
	}
	rewritten, err := Read(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePreservedMutation(board, rewritten); err != nil {
		t.Fatal(err)
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
		`  (paper "User" 210 297)`,
		`  (title_block (title "Fable preservation fixture"))`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup (stackup (thickness 1.0)) (pad_to_mask_clearance 0.123) (pcbplotparams (layerselection 0x00010fc_ffffffff)))`,
		`  (net 0 "")`,
		`  (footprint "Test:Imported"`,
		`    (locked yes)`,
		`    (layer "F.Cu")`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10 0)`,
		`    (descr "preserve this description")`,
		`    (tags "preserve these tags")`,
		`    (property "Reference" "U1" (at 10 8 45) (layer "F.SilkS") (uuid "22222222-2222-5222-8222-222222222222") (effects (font (size 1 1))))`,
		`    (property "Value" "Imported" (at 10 12 0) (layer "F.Fab") (uuid "33333333-3333-5333-8333-333333333333"))`,
		`    (path "/11111111-1111-5111-8111-111111111111")`,
		`    (fp_text user "KEEP_ME" (at 0 0) (layer "F.SilkS") (uuid "44444444-4444-5444-8444-444444444444"))`,
		`    (solder_mask_margin 0.05)`,
		`    (zone_connect 2)`,
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
	return board
}
