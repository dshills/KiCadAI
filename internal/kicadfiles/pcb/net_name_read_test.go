package pcb

import (
	"strings"
	"testing"
)

func TestReadPCBResolvesNameOnlyPadAndCopperNets(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (footprint "Device:R"`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10)`,
		`    (property "Reference" "R1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-5111-8111-111111111112"))`,
		`    (property "Value" "10k" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-5111-8111-111111111113"))`,
		`    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu" "F.Mask") (net "SIG") (uuid "11111111-1111-5111-8111-111111111114"))`,
		`  )`,
		`  (segment (start 10 10) (end 20 10) (width 0.25) (layer "F.Cu") (net "SIG") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")

	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Nets) != 1 || read.Nets[0].Code != 1 || read.Nets[0].Name != "SIG" {
		t.Fatalf("nets = %#v, want deterministic SIG net", read.Nets)
	}
	if read.Footprints[0].Pads[0].NetCode != 1 || read.Footprints[0].Pads[0].NetName != "SIG" {
		t.Fatalf("pad net = %d %q", read.Footprints[0].Pads[0].NetCode, read.Footprints[0].Pads[0].NetName)
	}
	if read.Tracks[0].NetCode != 1 || read.Tracks[0].NetName != "SIG" {
		t.Fatalf("track net = %d %q", read.Tracks[0].NetCode, read.Tracks[0].NetName)
	}
}

func TestReadPCBNameOnlyNetAvoidsUndeclaredNumericCollision(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (footprint "Device:R"`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10)`,
		`    (property "Reference" "R1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-5111-8111-111111111112"))`,
		`    (property "Value" "10k" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-5111-8111-111111111113"))`,
		`    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu" "F.Mask") (net 5 "LEGACY") (uuid "11111111-1111-5111-8111-111111111114"))`,
		`    (pad "2" smd rect (at 2 0) (size 1 1) (layers "F.Cu" "F.Mask") (net "SIG") (uuid "11111111-1111-5111-8111-111111111115"))`,
		`  )`,
		`)`,
	}, "\n")

	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if read.Footprints[0].Pads[1].NetCode <= read.Footprints[0].Pads[0].NetCode {
		t.Fatalf("name-only net code collided with numeric reference: pads=%#v nets=%#v", read.Footprints[0].Pads, read.Nets)
	}
}

func TestReadPCBTrimsNameOnlyNetReferences(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (segment (start 10 10) (end 20 10) (width 0.25) (layer "F.Cu") (net " SIG ") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")

	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if read.Tracks[0].NetCode != 1 || read.Tracks[0].NetName != "SIG" {
		t.Fatalf("track net = %d %q, want trimmed SIG", read.Tracks[0].NetCode, read.Tracks[0].NetName)
	}
}
