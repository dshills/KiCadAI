package pcb

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadPCBWrittenByWriter(t *testing.T) {
	board, err := LEDIndicatorPCB(LEDIndicatorInput{
		Name:     "reader_demo",
		DesignID: "11111111-1111-5111-8111-111111111111",
		Seed:     "reader_demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Write(&buf, board); err != nil {
		t.Fatal(err)
	}
	read, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if read.Version == "" || read.Generator == "" || len(read.Layers) == 0 || len(read.Footprints) == 0 || len(read.Tracks) == 0 {
		t.Fatalf("unexpected read PCB: %#v", read)
	}
	if read.Footprints[0].LibraryID == "" || read.Footprints[0].Reference == "" {
		t.Fatalf("footprint metadata not read: %#v", read.Footprints[0])
	}
	if read.Footprints[0].Raw == "" || len(read.Footprints[0].Pads) == 0 || read.Footprints[0].Pads[0].Raw == "" {
		t.Fatalf("footprint raw nodes not preserved: %#v", read.Footprints[0])
	}
}

func TestReadPCBPreservesUnsupportedNode(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (future_constraint (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Preserved) != 1 {
		t.Fatalf("preserved = %d, want 1", len(read.Preserved))
	}
	if read.Preserved[0].Family != "future_constraint" || !strings.Contains(read.Preserved[0].Raw, "future_constraint") {
		t.Fatalf("unexpected preserved node: %#v", read.Preserved[0])
	}
}

func TestReadPCBRejectsInvalidCoordinates(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (segment (start nope 1) (end 2 1) (width 0.25) (layer "F.Cu") (net "A") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	_, err := Read([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "coordinate") {
		t.Fatalf("expected coordinate error, got %v", err)
	}
}

func TestReadPCBRejectsMissingCoordinates(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (segment (start 1) (end 2 1) (width 0.25) (layer "F.Cu") (net "A") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")
	_, err := Read([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "requires x and y") {
		t.Fatalf("expected missing coordinate error, got %v", err)
	}
}
