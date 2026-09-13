package main

import (
	"fmt"
	"kicadai/internal/kicadfiles/sexpr"
	"strings"
)

func sexprText(n node) (string, error) { return sexpr.Format(n.Node()) }

// Bosch Figure 17 recommends separate 100 nF VDD and VDDIO bypass capacitors.
// Preserve the inherited supply capacitor and add these explicit local paths.
func addSensorBypass(sch, pcb *node) {
	var s0, f0 node
	for _, s := range sch.ChildrenByHead("symbol") {
		if prop(s, "Reference") == "C4" {
			s0 = s
		}
	}
	for _, f := range pcb.ChildrenByHead("footprint") {
		if prop(f, "Reference") == "C4" {
			f0 = f
		}
	}
	oldSymbol := value(s0, "uuid")
	for _, spec := range []struct {
		ref  string
		x, y float64
	}{{"C5", 85.5, 28}, {"C6", 81.5, 24.4}} {
		clone := func(n node) node {
			s, e := sexprText(n)
			must(e)
			s = strings.ReplaceAll(s, oldSymbol, uid(spec.ref+"symbol"))
			s = strings.ReplaceAll(s, "\"C4\"", fmt.Sprintf("%q", spec.ref))
			out := parse(s)
			walk(&out, func(n *node) {
				if n.Head() == "uuid" {
					n.Children[1] = str(uid(spec.ref + n.ListValue(1)))
				}
			})
			return out
		}
		s := clone(s0)
		set(&s, "uuid", parse(fmt.Sprintf("(uuid %q)", uid(spec.ref+"symbol"))))
		sch.Children = append(sch.Children, s)
		f := clone(f0)
		set(&f, "at", parse(fmt.Sprintf("(at %.4f %.4f)", spec.x, spec.y)))
		if spec.ref == "C6" {
			set(&f, "at", parse(fmt.Sprintf("(at %.4f %.4f 270)", spec.x, spec.y)))
			for j := range f.Children {
				p := &f.Children[j]
				if p.Head() == "pad" {
					a := child(*p, "at")
					set(p, "at", parse(fmt.Sprintf("(at %s %s 270)", a.ListValue(1), a.ListValue(2))))
				}
			}
		}
		pcb.Children = append(pcb.Children, f)
	}
	for i, s := range []string{
		"(segment (start 84.55 28) (end 84.8 26.5) (width 0.25) (layer \"F.Cu\") (net 13))",
		"(segment (start 86.45 28) (end 86.45 29.25) (width 0.25) (layer \"F.Cu\") (net 2))",
		"(segment (start 81.5 23.45) (end 81.5 23.8) (width 0.25) (layer \"F.Cu\") (net 13))",
		"(segment (start 81.5 25.35) (end 81.5 25) (width 0.25) (layer \"F.Cu\") (net 2))",
	} {
		n := parse(s)
		set(&n, "uuid", parse(fmt.Sprintf("(uuid %q)", uid(fmt.Sprintf("sensor-bypass-%d", i)))))
		pcb.Children = append(pcb.Children, n)
	}
}
