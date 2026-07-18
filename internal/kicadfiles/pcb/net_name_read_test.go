package pcb

import (
	"math"
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

func TestReadPCBPreservesFootprintRotationForConnectivity(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (2 "B.Cu" signal))`,
		`  (setup)`,
		`  (footprint "Resistor_SMD:R_0805_2012Metric"`,
		`    (uuid "33333333-3333-5333-8333-333333333331")`,
		`    (at 5 5)`,
		`    (layer "F.Cu")`,
		`    (path "/33333333-3333-5333-8333-333333333331")`,
		`    (property "Reference" "R1" (at 0 0 0) (layer "F.SilkS") (uuid "33333333-3333-5333-8333-333333333332"))`,
		`    (property "Value" "1k" (at 0 1 0) (layer "F.Fab") (uuid "33333333-3333-5333-8333-333333333333"))`,
		`    (pad "2" smd rect (at 0.6 0) (size 0.7 0.8) (layers "F.Cu" "F.Mask") (net "SIG") (uuid "33333333-3333-5333-8333-333333333334"))`,
		`  )`,
		`  (footprint "LED_SMD:LED_0805_2012Metric"`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 5 180)`,
		`    (layer "F.Cu")`,
		`    (path "/11111111-1111-5111-8111-111111111111")`,
		`    (property "Reference" "D1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-5111-8111-111111111112"))`,
		`    (property "Value" "LED" (at 0 1 0) (layer "F.Fab") (uuid "11111111-1111-5111-8111-111111111113"))`,
		`    (pad "1" smd rect (at -0.6 0 180) (size 0.7 0.8) (layers "F.Cu" "F.Mask") (net "SIG") (uuid "11111111-1111-5111-8111-111111111114"))`,
		`  )`,
		`  (segment (start 5.6 5) (end 10.6 5) (width 0.25) (layer "F.Cu") (net "SIG") (uuid "22222222-2222-5222-8222-222222222222"))`,
		`)`,
	}, "\n")

	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if read.Footprints[1].Rotation != 180 {
		t.Fatalf("footprint rotation = %g, want 180", read.Footprints[1].Rotation)
	}
	if math.Abs(float64(read.Footprints[1].Pads[0].Rotation)) > 1e-9 {
		t.Fatalf("relative pad rotation = %g, want 0", read.Footprints[1].Pads[0].Rotation)
	}
	if err := ValidateGeneratedConnectivity(read); err != nil {
		t.Fatalf("ValidateGeneratedConnectivity returned error: %v", err)
	}
}

func TestReadPCBParsesFootprintCourtyardGraphics(t *testing.T) {
	input := strings.Join([]string{
		`(kicad_pcb`,
		`  (version 20260206)`,
		`  (generator "pcbnew")`,
		`  (generator_version "10.0.0")`,
		`  (paper "A4")`,
		`  (layers (0 "F.Cu" signal) (31 "F.CrtYd" user "F.Courtyard"))`,
		`  (setup)`,
		`  (footprint "Device:R"`,
		`    (uuid "11111111-1111-5111-8111-111111111111")`,
		`    (at 10 10)`,
		`    (layer "F.Cu")`,
		`    (property "Reference" "R1" (at 0 0 0) (layer "F.SilkS") (uuid "11111111-1111-5111-8111-111111111112"))`,
		`    (fp_rect (start -1 -1) (end 1 1) (stroke (width 0.05) (type default)) (fill none) (layer "F.CrtYd") (uuid "11111111-1111-5111-8111-111111111113"))`,
		`    (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu" "F.Mask") (uuid "11111111-1111-5111-8111-111111111114"))`,
		`  )`,
		`)`,
	}, "\n")

	read, err := Read([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Footprints) != 1 || len(read.Footprints[0].Graphics) != 1 {
		t.Fatalf("footprint graphics = %#v", read.Footprints)
	}
	drawing := Drawing(read.Footprints[0].Graphics[0])
	if drawing.Kind != "rect" || drawing.Layer != "F.CrtYd" || drawing.Rect == nil {
		t.Fatalf("courtyard graphic = %#v", drawing)
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
