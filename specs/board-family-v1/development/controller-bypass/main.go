// Development-only reference engineering; always creates a NEW project.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/boardfamily"
	"kicadai/internal/kicadfiles/sexpr"
)

type node = sexpr.ParsedNode

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func parse(s string) node { n, e := sexpr.Parse([]byte(s)); must(e); return n }
func set(n *node, h string, v node) {
	for i := range n.Children {
		if n.Children[i].Head() == h {
			n.Children[i] = v
			return
		}
	}
	n.Children = append(n.Children, v)
}
func property(n node, k string) string {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == k {
			return p.ListValue(2)
		}
	}
	return ""
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: controller-bypass CONFIG NEW_OUTPUT")
	}
	f, e := os.Open(os.Args[1])
	must(e)
	c, e := boardfamily.Decode(f)
	must(e)
	must(f.Close())
	_, e = boardfamily.Generate(c, os.Args[2])
	must(e)
	p := filepath.Join(os.Args[2], "board.kicad_pcb")
	b, e := os.ReadFile(p)
	must(e)
	n, e := sexpr.Parse(b)
	must(e)
	setup, ok := n.Child("setup")
	if !ok {
		panic("reference lacks setup")
	}
	set(&setup, "stackup", parse(`(stackup
		(layer "F.SilkS" (type "Top Silk Screen"))
		(layer "F.Paste" (type "Top Solder Paste"))
		(layer "F.Mask" (type "Top Solder Mask") (thickness 0.01))
		(layer "F.Cu" (type "copper") (thickness 0.035))
		(layer "dielectric 1" (type "core") (thickness 1.51) (material "FR4") (epsilon_r 4.5) (loss_tangent 0.02))
		(layer "B.Cu" (type "copper") (thickness 0.035))
		(layer "B.Mask" (type "Bottom Solder Mask") (thickness 0.01))
		(layer "B.Paste" (type "Bottom Solder Paste"))
		(layer "B.SilkS" (type "Bottom Silk Screen"))
		(copper_finish "None") (dielectric_constraints no))`))
	set(&n, "setup", setup)
	for i := range n.Children {
		f := &n.Children[i]
		if f.Head() != "footprint" {
			continue
		}
		ref := property(*f, "Reference")
		x := 0.0
		if ref == "C1" {
			x = 39.5
		}
		if ref == "C2" {
			x = 36.5
		}
		if x == 0 {
			continue
		}
		set(f, "at", parse(fmt.Sprintf("(at %.4f 25.45 90)", x)))
		for j := range f.Children {
			pad := &f.Children[j]
			if pad.Head() == "pad" {
				a, _ := pad.Child("at")
				set(pad, "at", parse(fmt.Sprintf("(at %s %s 90)", a.ListValue(1), a.ListValue(2))))
			}
		}
	}
	// Extend directly to the existing parallel supply/return trunks. Remove
	// only the obsolete dead-end VCC stub that formerly terminated at C2.
	var children []node
	for _, v := range n.Children {
		if v.Head() == "segment" {
			a, _ := v.Child("start")
			z, _ := v.Child("end")
			if (a.ListValue(1) == "25" && a.ListValue(2) == "29") || (z.ListValue(1) == "25" && z.ListValue(2) == "29") {
				continue
			}
		}
		children = append(children, v)
	}
	n.Children = children
	for i, x := range []float64{39.5, 36.5} {
		for j, y := range []float64{26.4, 24.5} {
			end, net := 26.0, 13
			if j == 1 {
				end, net = 24.75, 2
			}
			n.Children = append(n.Children, parse(fmt.Sprintf("(segment (start %.4f %.4f) (end %.4f %.4f) (width 0.4) (layer \"F.Cu\") (net %d) (uuid \"61000000-0000-5000-8000-%012d\"))", x, y, x, end, net, i*2+j+1)))
		}
	}
	s, e := sexpr.Format(n.Node())
	must(e)
	must(os.WriteFile(p, []byte(s), 0644))
}
