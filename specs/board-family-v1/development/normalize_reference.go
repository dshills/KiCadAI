// Development-only reference engineering. Never run on acceptance outputs.
// Reuses KiCadAI's native parser, checks net partitions before aligning names,
// makes references readable, and applies an explicit reviewed copper bridge.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

type node = sexpr.ParsedNode

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) node {
	b, e := os.ReadFile(path)
	must(e)
	n, e := sexpr.Parse(b)
	must(e)
	return n
}
func child(n node, head string) node   { c, _ := n.Child(head); return c }
func value(n node, head string) string { return child(n, head).ListValue(1) }
func str(s string) node                { return node{Quoted: true, String: s} }
func parse(s string) node              { n, e := sexpr.Parse([]byte(s)); must(e); return n }
func prop(n node, name string) string {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == name {
			return p.ListValue(2)
		}
	}
	return ""
}
func hasProp(n node, name string) bool {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == name {
			return true
		}
	}
	return false
}
func walk(n *node, f func(*node)) {
	f(n)
	for i := range n.Children {
		walk(&n.Children[i], f)
	}
}
func write(path string, n node) {
	s, e := sexpr.Format(n.Node())
	must(e)
	must(os.WriteFile(path, []byte(s+"\n"), 0644))
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "inspect" {
		p := read(os.Args[2])
		for _, f := range p.ChildrenByHead("footprint") {
			if r := prop(f, "Reference"); r == "U2" || r == "C4" {
				fmt.Println(r, child(f, "at").Raw)
				for _, pad := range f.ChildrenByHead("pad") {
					fmt.Println("pad", pad.ListValue(1), child(pad, "at").Raw, child(pad, "net").Raw)
				}
			}
		}
		for _, n := range p.ChildrenByHead("segment") {
			a := child(n, "start")
			if number(a, 1) > 80 && number(a, 2) < 35 {
				fmt.Println(value(n, "net"), value(n, "layer"), a.Raw, child(n, "end").Raw)
			}
		}
		return
	}
	if len(os.Args) != 5 {
		panic("usage: go run normalize_reference.go INPUT_DIR INPUT_NAME NETLIST OUTPUT_DIR")
	}
	src, base, netpath, dst := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	if _, e := os.Stat(dst); !os.IsNotExist(e) {
		panic("output must not exist")
	}
	must(os.MkdirAll(dst, 0755))
	must(filepath.WalkDir(src, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		out := filepath.Join(dst, strings.ReplaceAll(r, base, "board"))
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(out, []byte(strings.ReplaceAll(string(b), base, "board")), 0644)
	}))
	sch := read(filepath.Join(dst, "board.kicad_sch"))
	pcb := read(filepath.Join(dst, "board.kicad_pcb"))
	net := read(netpath)
	pinNets := map[string]string{}
	for _, n := range child(net, "nets").ChildrenByHead("net") {
		for _, p := range n.ChildrenByHead("node") {
			pinNets[value(p, "ref")+"."+value(p, "pin")] = value(n, "name")
		}
	}
	names := map[string]string{}
	for _, fp := range pcb.ChildrenByHead("footprint") {
		for _, pad := range fp.ChildrenByHead("pad") {
			pn := child(pad, "net")
			code := pn.ListValue(1)
			if code == "" || code == "0" {
				continue
			}
			key := prop(fp, "Reference") + "." + pad.ListValue(1)
			name := pinNets[key]
			if name == "" {
				panic("missing schematic pin " + key)
			}
			if old := names[code]; old != "" && old != name {
				panic("PCB net merges distinct schematic nets: " + old + " / " + name)
			}
			names[code] = name
		}
	}
	reverse := map[string]string{}
	for c, n := range names {
		if old := reverse[n]; old != "" && old != c {
			panic("PCB splits schematic net " + n)
		}
		reverse[n] = c
	}
	walk(&pcb, func(n *node) {
		if n.Head() == "net" && len(n.Children) > 2 {
			if name := names[n.ListValue(1)]; name != "" {
				n.Children[2] = str(name)
			}
		}
	})
	refs := []string{}
	symbols := map[string]node{}
	for _, s := range sch.ChildrenByHead("symbol") {
		r := prop(s, "Reference")
		if r != "" && !strings.HasPrefix(r, "#") {
			refs = append(refs, r)
			symbols[r] = s
		}
	}
	sort.Strings(refs)
	counts := map[string]int{}
	renames := map[string]string{}
	for _, r := range refs {
		prefix := r[:1]
		if strings.HasPrefix(r, "SW") {
			prefix = "SW"
		}
		counts[prefix]++
		renames[r] = fmt.Sprintf("%s%d", prefix, counts[prefix])
	}
	for i := range pcb.Children {
		fp := &pcb.Children[i]
		if fp.Head() != "footprint" {
			continue
		}
		s := symbols[prop(*fp, "Reference")]
		for _, p := range s.ChildrenByHead("property") {
			name := p.ListValue(1)
			if name == "Reference" || name == "Value" || name == "Footprint" {
				continue
			}
			if hasProp(*fp, name) {
				continue
			}
			fp.Children = append(fp.Children, parse(fmt.Sprintf("(property %q %q (at 0 0 0) (layer \"F.Fab\") (hide yes) (uuid %q) (effects (font (size 1 1) (thickness 0.15))))", name, p.ListValue(2), uid(prop(*fp, "Reference")+"/property/"+name))))
		}
	}
	for _, root := range []*node{&sch, &pcb} {
		walk(root, func(n *node) {
			if !n.IsList && n.Quoted {
				if r := renames[n.String]; r != "" {
					n.String = r
				}
			}
		})
	}
	addSensorBypass(&sch, &pcb)
	for _, r := range []string{"C5", "C6"} {
		renames[r] = r
		pinNets[r+".1"] = "/VCC"
		pinNets[r+".2"] = "/GND"
	}
	relayout(&sch, pinNets, renames)
	// Bridge only the identified reference-development crossing; fail closed if
	// a different input is accidentally supplied. Geometry remains native copper.
	patched := false
	for i, n := range pcb.Children {
		if n.Head() == "segment" && value(n, "uuid") == "e0b16914-94a1-57ed-85b8-729b83cdf9df" {
			if value(n, "layer") != "B.Cu" || child(n, "start").ListValue(1) != "87" || child(n, "end").ListValue(1) != "81.5" {
				panic("unexpected bridge geometry")
			}
			pcb.Children = append(pcb.Children[:i], pcb.Children[i+1:]...)
			patched = true
			break
		}
	}
	if !patched {
		panic("expected BMP280 reference crossing absent")
	}
	code := reverse["/VCC"]
	if code == "" {
		panic("expected /VCC net")
	}
	for i, s := range []string{
		fmt.Sprintf("(segment (start 87 26.5) (end 84.8 26.5) (width 0.2) (layer \"B.Cu\") (net %s) (uuid \"be370000-0000-5000-8000-000000000001\"))", code),
		fmt.Sprintf("(segment (start 84.8 26.5) (end 82.8 26.5) (width 0.2) (layer \"F.Cu\") (net %s) (uuid \"be370000-0000-5000-8000-000000000002\"))", code),
		fmt.Sprintf("(segment (start 82.8 26.5) (end 81.5 26.5) (width 0.2) (layer \"B.Cu\") (net %s) (uuid \"be370000-0000-5000-8000-000000000003\"))", code),
		fmt.Sprintf("(via (at 84.8 26.5) (size 0.6) (drill 0.3) (layers \"F.Cu\" \"B.Cu\") (net %s) (uuid \"be370000-0000-5000-8000-000000000004\"))", code),
		fmt.Sprintf("(via (at 82.8 26.5) (size 0.6) (drill 0.3) (layers \"F.Cu\" \"B.Cu\") (net %s) (uuid \"be370000-0000-5000-8000-000000000005\"))", code),
	} {
		_ = i
		pcb.Children = append(pcb.Children, parse(s))
	}
	write(filepath.Join(dst, "board.kicad_sch"), sch)
	write(filepath.Join(dst, "board.kicad_pcb"), pcb)
	fmt.Printf("Reference engineering: aligned %d nets, renamed %d components, inserted explicit two-via supply bridge\n", len(names), len(refs))
}
