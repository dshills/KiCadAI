package main

import (
	"math"
	"os"
	"strings"
	"testing"
)

func sourceFootprint(t *testing.T) node {
	t.Helper()
	b, err := os.ReadFile("../../../../internal/boardfamily/reference-sht31/" + oldPath)
	if err != nil {
		t.Fatal(err)
	}
	return parse(b)
}

func values(n node, want ...string) bool {
	if len(n.Children) != len(want) {
		return false
	}
	for i, s := range want {
		if n.ListValue(i) != s {
			return false
		}
	}
	return true
}

func TestAssemblyGeometryAndInputImmutability(t *testing.T) {
	original := sourceFootprint(t)
	before := string(format(original))
	revised := reviseFootprint(original)
	if string(format(original)) != before {
		t.Fatal("revision mutated input")
	}
	if copperPads(original) != copperPads(revised) {
		t.Fatal("electrical pads changed")
	}
	if revised.ListValue(1) != newName || !values(child(revised, "attr"), "attr", "smd", "allow_soldermask_bridges") {
		t.Fatal("missing scoped footprint identity/mask attribute")
	}
	var centerOriginal, centerRevised string
	for _, p := range original.ChildrenByHead("pad") {
		if p.ListValue(1) == "" {
			centerOriginal = string(format(p))
		}
	}
	pasteCount := 0
	seen := map[string]bool{}
	for _, p := range revised.ChildrenByHead("pad") {
		layers := child(p, "layers")
		id := p.ListValue(1)
		if id == "" {
			pasteCount++
			if p.ListValue(3) == "custom" {
				centerRevised = string(format(p))
				continue
			}
			x, _ := child(p, "at").FloatValue(1)
			y, _ := child(p, "at").FloatValue(2)
			if math.Abs(math.Abs(x)-1.275) > 1e-9 || math.Abs(y) != 0.25 && math.Abs(y) != 0.75 {
				t.Fatal("incorrect shifted paste center", x, y)
			}
			if !values(child(p, "size"), "size", "0.55", "0.25") || !values(layers, "layers", "F.Paste") {
				t.Fatal("incorrect paste dimensions/layer")
			}
			key := string(format(child(p, "at")))
			if seen[key] {
				t.Fatal("duplicate paste location")
			}
			seen[key] = true
		} else if id == "9" {
			v, ok := child(p, "solder_mask_margin").FloatValue(1)
			if !ok || v != 0.075 {
				t.Fatal("incorrect EP mask clearance")
			}
		} else if !values(layers, "layers", "F.Cu") {
			t.Fatal("I/O mask/paste still follows copper")
		}
	}
	if pasteCount != 9 || len(seen) != 8 || centerOriginal == "" || centerOriginal != centerRevised {
		t.Fatal("paste inventory or existing EP aperture changed")
	}
	masks := revised.ChildrenByHead("fp_rect")
	if len(masks) != 2 {
		t.Fatal("need exactly two mask rows")
	}
	for i, xs := range [][2]float64{{-1.525, -0.825}, {0.825, 1.525}} {
		m := masks[i]
		sx, _ := child(m, "start").FloatValue(1)
		sy, _ := child(m, "start").FloatValue(2)
		ex, _ := child(m, "end").FloatValue(1)
		ey, _ := child(m, "end").FloatValue(2)
		if sx != xs[0] || ex != xs[1] || sy != -0.95 || ey != 0.95 || child(m, "layer").ListValue(1) != "F.Mask" || child(m, "fill").ListValue(1) != "yes" {
			t.Fatal("wrong grouped mask geometry")
		}
	}
}

func TestAssemblyRejectsUnexpectedReference(t *testing.T) {
	for _, mutation := range []string{"identity", "missing-pad", "coordinate"} {
		t.Run(mutation, func(t *testing.T) {
			n := sourceFootprint(t)
			switch mutation {
			case "identity":
				n.Children[1] = stringNode("unknown")
			case "missing-pad":
				for i, p := range n.Children {
					if p.Head() == "pad" && p.ListValue(1) == "1" {
						n.Children = append(n.Children[:i], n.Children[i+1:]...)
						break
					}
				}
			case "coordinate":
				for i, p := range n.Children {
					if p.Head() == "pad" && p.ListValue(1) == "1" {
						set(&n.Children[i], "at", expr(`(at -1.2 -0.75)`))
						break
					}
				}
			}
			defer func() {
				if recover() == nil {
					t.Error("unexpected reference was admitted")
				}
			}()
			reviseFootprint(n)
		})
	}
}

func TestSchematicLibraryRemainsUnchanged(t *testing.T) {
	n := expr(`(kicad_sch (lib_symbols (symbol "sensor" (property "Footprint" "Sensor_Humidity:` + oldName + `"))) (symbol (property "Reference" "U2") (property "Footprint" "Sensor_Humidity:` + oldName + `")) (symbol (property "Reference" "U1") (property "Footprint" "controller")))`)
	beforeLib := string(format(child(n, "lib_symbols")))
	renameSchematic(&n)
	if string(format(child(n, "lib_symbols"))) != beforeLib || strings.Count(string(format(n)), newName) != 1 {
		t.Fatal("instance update changed the library or another component")
	}
}

func TestCapturedFootprintMatchesDevelopmentRecipe(t *testing.T) {
	want := format(reviseFootprint(sourceFootprint(t)))
	got, err := os.ReadFile("../../../../internal/boardfamily/reference-sht31/" + newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("captured footprint differs from reproducible development recipe")
	}
}
