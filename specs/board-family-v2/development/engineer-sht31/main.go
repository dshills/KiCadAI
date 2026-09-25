// Development-only reference engineering. Inputs remain unchanged; outputs must
// be new development directories, never evaluated projects. Not production code.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

type node = sexpr.ParsedNode

const source = "internal/boardfamily/reference"
const installed = "/Applications/KiCad/KiCad.app/Contents/SharedSupport"
const footprint = "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm"
const symbol = "kicadai_sensor_humidity:SHT31-DIS"

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func parse(s string) node           { n, e := sexpr.Parse([]byte(s)); must(e); return n }
func read(p string) node            { b, e := os.ReadFile(p); must(e); return parse(string(b)) }
func child(n node, h string) node   { c, _ := n.Child(h); return c }
func value(n node, h string) string { return child(n, h).ListValue(1) }
func number(n node, i int) float64 {
	v, ok := n.FloatValue(i)
	if !ok {
		panic("invalid coordinate")
	}
	return v
}
func str(s string) node { return node{Quoted: true, String: s} }
func prop(n node, k string) string {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == k {
			return p.ListValue(2)
		}
	}
	return ""
}
func set(n *node, h string, r node) {
	for i := range n.Children {
		if n.Children[i].Head() == h {
			n.Children[i] = r
			return
		}
	}
	n.Children = append(n.Children, r)
}
func property(n *node, k, v string) {
	for i := range n.Children {
		p := &n.Children[i]
		if p.Head() == "property" && p.ListValue(1) == k {
			p.Children[2] = str(v)
			return
		}
	}
}
func walk(n *node, f func(*node)) {
	f(n)
	for i := range n.Children {
		walk(&n.Children[i], f)
	}
}
func uid(key string) string {
	b := sha256.Sum256([]byte("board-family-v2/sht31/" + key))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func write(p string, n node) {
	s, e := sexpr.Format(n.Node())
	must(e)
	must(os.WriteFile(p, []byte(s+"\n"), 0644))
}
func hash(p string) string {
	b, e := os.ReadFile(p)
	must(e)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "inspect" {
		b := read(os.Args[2])
		for _, n := range b.Children {
			switch n.Head() {
			case "segment":
				a, z := child(n, "start"), child(n, "end")
				if (number(a, 1) > 76 && number(a, 2) < 31) || (number(z, 1) > 76 && number(z, 2) < 31) {
					fmt.Printf("%s %s (%.4f,%.4f)->(%.4f,%.4f) %s\n", value(n, "net"), value(n, "layer"), number(a, 1), number(a, 2), number(z, 1), number(z, 2), value(n, "uuid"))
				}
			case "via":
				a := child(n, "at")
				if number(a, 1) > 76 && number(a, 2) < 31 {
					fmt.Printf("via %s %.4f %.4f\n", value(n, "net"), number(a, 1), number(a, 2))
				}
			}
		}
		return
	}
	if len(os.Args) != 2 {
		panic("usage: engineer-sht31 NEW_DEVELOPMENT_DIR | inspect BOARD")
	}
	dst := filepath.Clean(os.Args[1])
	if !strings.HasPrefix(dst, ".cache/board-family-v2/") {
		panic("output must be a new board-family-v2 development directory")
	}
	for f, want := range map[string]string{"board.kicad_pcb": "ce633e4fbe766d3d11db38ccfb00f46fbe5cdf87dd57fecc436dff90fb9d1bae", "board.kicad_sch": "16159b91ed2f2a2d6f1c4ff73740711913f80692d5e8d7255436854b15bba793"} {
		if hash(filepath.Join(source, f)) != want {
			panic("reference source changed: " + f)
		}
	}
	must(os.Mkdir(dst, 0755))
	must(filepath.WalkDir(source, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, e := filepath.Rel(source, p)
		if e != nil {
			return e
		}
		q := filepath.Join(dst, r)
		if d.IsDir() {
			return os.MkdirAll(q, 0755)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(q, b, 0644)
	}))
	sch, b := read(filepath.Join(dst, "board.kicad_sch")), read(filepath.Join(dst, "board.kicad_pcb"))
	lib := read(filepath.Join(installed, "symbols/Sensor_Humidity.kicad_sym"))
	var sensor node
	for _, n := range lib.ChildrenByHead("symbol") {
		if n.ListValue(1) == "SHT31-DIS" {
			sensor = n
			break
		}
	}
	if sensor.Head() != "symbol" {
		panic("installed sensor symbol missing")
	}
	newLib := parse(`(kicad_symbol_lib (version 20241209) (generator "kicad_symbol_editor"))`)
	newLib.Children = append(newLib.Children, sensor)
	write(filepath.Join(dst, "lib/kicadai_sensor_humidity.kicad_sym"), newLib)
	sensor.Children[1] = str(symbol)
	for i := range sch.Children {
		if sch.Children[i].Head() == "lib_symbols" {
			sch.Children[i].Children = append(sch.Children[i].Children, sensor)
		}
	}
	table := read(filepath.Join(dst, "sym-lib-table"))
	table.Children = append(table.Children, parse(`(lib (name "kicadai_sensor_humidity") (type "KiCad") (uri "${KIPRJMOD}/lib/kicadai_sensor_humidity.kicad_sym") (options "") (descr "SHT31 reference"))`))
	write(filepath.Join(dst, "sym-lib-table"), table)
	parts := strings.SplitN(footprint, ":", 2)
	fpPath := filepath.Join(installed, "footprints", parts[0]+".pretty", parts[1]+".kicad_mod")
	newFP := read(fpPath)
	fpDst := filepath.Join(dst, "footprints", parts[0]+".pretty")
	must(os.Mkdir(fpDst, 0755))
	raw, e := os.ReadFile(fpPath)
	must(e)
	must(os.WriteFile(filepath.Join(fpDst, parts[1]+".kicad_mod"), raw, 0644))
	ft := read(filepath.Join(dst, "fp-lib-table"))
	ft.Children = append(ft.Children, parse(`(lib (name "Sensor_Humidity") (type "KiCad") (uri "${KIPRJMOD}/footprints/Sensor_Humidity.pretty") (options "") (descr ""))`))
	write(filepath.Join(dst, "fp-lib-table"), ft)
	engineerSchematic(&sch, sensor)
	engineerPCB(&b, newFP)
	// Align native metadata before parity review, as in the production writer.
	byRef := map[string]node{}
	for _, n := range sch.ChildrenByHead("symbol") {
		byRef[prop(n, "Reference")] = n
	}
	for i := range b.Children {
		n := &b.Children[i]
		if n.Head() != "footprint" {
			continue
		}
		s := byRef[prop(*n, "Reference")]
		for _, p := range s.ChildrenByHead("property") {
			k, v := p.ListValue(1), p.ListValue(2)
			if k == "Footprint" {
				continue
			}
			found := false
			for _, q := range n.ChildrenByHead("property") {
				if q.ListValue(1) == k {
					found = true
				}
			}
			if found {
				property(n, k, v)
			} else {
				n.Children = append(n.Children, parse(fmt.Sprintf("(property %q %q (at 0 0 0) (layer \"F.Fab\") (hide yes) (uuid %q) (effects (font (size 1 1) (thickness 0.15))))", k, v, uid(prop(*n, "Reference")+"/property/"+k))))
			}
		}
		for j := range n.Children {
			c := &n.Children[j]
			if c.Head() == "fp_text" && c.ListValue(1) == "value" {
				c.Children[2] = str(prop(s, "Value"))
			}
		}
	}
	write(filepath.Join(dst, "board.kicad_sch"), sch)
	write(filepath.Join(dst, "board.kicad_pcb"), b)
	receipt := map[string]any{"status": "unqualified_development_reference", "source": source, "manual_reference_engineering": true, "evaluated_output_repair": false, "sensor_symbol_library_sha256": hash(filepath.Join(installed, "symbols/Sensor_Humidity.kicad_sym")), "sensor_footprint_sha256": hash(fpPath), "description": "Replace BMP280 with SHT31-DIS-B; reuse controller/support, explicitly replace sensor-area routes, keep one local sensor bypass and remove redundant C6. Revalidate all electrical/native/manufacturing checks before delivery."}
	raw, e = json.MarshalIndent(receipt, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(dst, "engineering.json"), append(raw, '\n'), 0644))
	fmt.Println(dst)
}

func engineerSchematic(sch *node, sensor node) {
	var old node
	for _, n := range sch.ChildrenByHead("symbol") {
		if prop(n, "Reference") == "U2" {
			old = n
		}
	}
	x, y := number(child(old, "at"), 1), number(child(old, "at"), 2)
	keep := []node{}
	for _, n := range sch.Children {
		if n.Head() == "symbol" && prop(n, "Reference") == "C6" {
			continue
		}
		if n.Head() == "wire" || n.Head() == "label" || n.Head() == "junction" || n.Head() == "no_connect" {
			a := child(n, "at")
			if n.Head() == "wire" {
				pts := child(n, "pts")
				if len(pts.Children) > 1 {
					a = pts.Children[1]
				}
			}
			if a.Head() != "" {
				px, py := number(a, 1), number(a, 2)
				if (math.Abs(px-x) < 35 && math.Abs(py-y) < 30) || (px > 265 && px < 295 && py > 150 && py < 180) {
					continue
				}
			}
		}
		if n.Head() == "symbol" && prop(n, "Reference") == "U2" {
			set(&n, "lib_id", parse(fmt.Sprintf("(lib_id %q)", symbol)))
			property(&n, "Value", "SHT31-DIS-B 0x44")
			property(&n, "Footprint", footprint)
			property(&n, "Manufacturer", "Sensirion")
			property(&n, "MPN", "SHT31-DIS-B2.5kS")
			property(&n, "KiCadAI Component ID", "sensor.sensirion.sht31_dis.dfn8")
			without := []node{}
			for _, c := range n.Children {
				if c.Head() != "pin" {
					without = append(without, c)
				}
			}
			n.Children = without
			for i := 1; i <= 9; i++ {
				n.Children = append(n.Children, parse(fmt.Sprintf("(pin %q (uuid %q))", fmt.Sprint(i), uid(fmt.Sprintf("U2/pin/%d", i)))))
			}
		}
		walk(&n, func(v *node) {
			if !v.IsList && v.Quoted && strings.Contains(v.String, "BMP280") {
				v.String = strings.ReplaceAll(v.String, "BMP280 reference", "SHT31 temperature/humidity reference")
			}
		})
		keep = append(keep, n)
	}
	sch.Children = keep
	nets := map[string]string{"1": "SDA", "2": "GND", "4": "SCL", "5": "VCC", "7": "GND", "8": "GND", "9": "GND"}
	seen := map[string]bool{}
	walk(&sensor, func(p *node) {
		if p.Head() != "pin" || len(p.Children) < 4 {
			return
		}
		a := child(*p, "at")
		px, py := x+number(a, 1), y-number(a, 2)
		net := nets[value(*p, "number")]
		key := fmt.Sprintf("%.4f/%.4f/%s", px, py, net)
		if seen[key] {
			return
		}
		seen[key] = true
		if net == "" {
			sch.Children = append(sch.Children, parse(fmt.Sprintf("(no_connect (at %.4f %.4f) (uuid %q))", px, py, uid(key+"nc"))))
			return
		}
		dx, dy := 0.0, 0.0
		switch number(a, 3) {
		case 0:
			dx = -7.62
		case 180:
			dx = 7.62
		case 90:
			dy = 7.62
		case 270:
			dy = -7.62
		default:
			panic("pin angle")
		}
		ex, ey := px+dx, py+dy
		sch.Children = append(sch.Children, parse(fmt.Sprintf("(wire (pts (xy %.4f %.4f) (xy %.4f %.4f)) (stroke (width 0.1524) (type default)) (uuid %q))", px, py, ex, ey, uid(key+"wire"))))
		just := "left"
		if dx < 0 {
			just = "right"
		}
		sch.Children = append(sch.Children, parse(fmt.Sprintf("(label %q (at %.4f %.4f 0) (effects (font (size 1 1)) (justify %s bottom)) (uuid %q))", net, ex, ey, just, uid(key+"label"))))
	})
}

func engineerPCB(b *node, fp node) {
	keep := []node{}
	for _, n := range b.Children {
		if n.Head() == "footprint" && prop(n, "Reference") == "C6" {
			continue
		}
		if n.Head() == "segment" || n.Head() == "via" {
			if value(n, "uuid") == "a225f650-e428-5078-aec8-0981bd686196" || value(n, "uuid") == "d8598587-bd48-5dea-8566-54bb6aa41d2d" || value(n, "uuid") == "dd107f9a-5447-5bb4-a0f8-86c614c4d4e0" {
				continue
			}
			points := []node{child(n, "at")}
			if n.Head() == "segment" {
				points = []node{child(n, "start"), child(n, "end")}
			}
			remove := false
			for _, a := range points {
				if number(a, 1) > 80 && number(a, 2) < 30 {
					remove = true
				}
			}
			// Keep the unchanged BOOT button's ground return, which passes
			// outside the old sensor group but has one endpoint below y=30.
			if value(n, "uuid") == "812b5897-6887-5f64-ae27-d9c72daf37b5" || value(n, "uuid") == "f68e502c-8260-5446-a9f9-51c1129a7041" {
				remove = false
			}
			if remove {
				continue
			}
		}
		if n.Head() == "footprint" && prop(n, "Reference") == "C5" {
			set(&n, "at", parse("(at 103.875 16.5 0)"))
		}
		if n.Head() == "footprint" && prop(n, "Reference") == "U2" {
			old := n
			n = fp
			n.Children[1] = str(footprint)
			set(&n, "at", parse("(at 100 15)"))
			set(&n, "uuid", child(old, "uuid"))
			for _, h := range []string{"path", "sheetname", "sheetfile"} {
				if c := child(old, h); c.Head() != "" {
					set(&n, h, c)
				}
			}
			property(&n, "Reference", "U2")
			property(&n, "Value", "SHT31-DIS-B 0x44")
			pins := map[string]string{"1": "/SDA", "2": "/GND", "4": "/SCL", "5": "/VCC", "7": "/GND", "8": "/GND", "9": "/GND"}
			for i := range n.Children {
				c := &n.Children[i]
				if c.Head() == "pad" {
					if name := pins[c.ListValue(1)]; name != "" {
						set(c, "net", parse(fmt.Sprintf("(net %q)", name)))
					}
					set(c, "uuid", parse(fmt.Sprintf("(uuid %q)", uid("pad/"+c.ListValue(1)))))
				}
			}
		}
		keep = append(keep, n)
	}
	b.Children = keep
	// Explicit reference-development routing, outside the production path.
	// Anchors preserve the controller/header bus and power partitions after
	// removing the old sensor-area wiring. Native DRC is an independent gate.
	track := func(net, layer string, xy ...float64) {
		for i := 0; i+3 < len(xy); i += 2 {
			key := fmt.Sprintf("%s/%s/%.4f/%.4f/%.4f/%.4f", net, layer, xy[i], xy[i+1], xy[i+2], xy[i+3])
			b.Children = append(b.Children, parse(fmt.Sprintf("(segment (start %.4f %.4f) (end %.4f %.4f) (width 0.25) (layer %q) (net %q) (uuid %q))", xy[i], xy[i+1], xy[i+2], xy[i+3], layer, net, uid(key))))
		}
	}
	via := func(net string, x, y float64) {
		b.Children = append(b.Children, parse(fmt.Sprintf("(via (at %.4f %.4f) (size 0.6) (drill 0.3) (layers \"F.Cu\" \"B.Cu\") (net %q) (uuid %q))", x, y, net, uid(fmt.Sprintf("via/%s/%.4f/%.4f", net, x, y)))))
	}
	track("/SDA", "F.Cu", 78.9125, 19, 80, 19, 80, 12, 97, 12, 97, 14.25, 98.825, 14.25)
	track("/SDA", "B.Cu", 75.5, 20.5, 80, 20.5)
	via("/SDA", 80, 20.5)
	track("/SDA", "F.Cu", 80, 20.5, 80, 19)
	track("/SCL", "B.Cu", 78.9125, 28, 80.5, 28, 80.5, 35.75, 87.75, 35.75)
	via("/SCL", 80.5, 28)
	track("/SCL", "F.Cu", 80.5, 28, 92, 28, 92, 15.75, 98.825, 15.75)
	track("/VCC", "F.Cu", 77.4, 23.8, 79, 23.8)
	via("/VCC", 79, 23.8)
	track("/VCC", "B.Cu", 79, 23.8, 103, 23.8, 103, 18.5)
	via("/VCC", 103, 18.5)
	track("/VCC", "F.Cu", 103, 18.5, 102.925, 18.5, 102.925, 16.5, 102.925, 15.75, 101.175, 15.75)
	track("/GND", "B.Cu", 78.95, 22.365, 82, 22.365, 82, 13.3, 101.8, 13.3)
	via("/GND", 82, 22.365)
	track("/GND", "F.Cu", 82, 22.365, 82, 25, 51, 25)
	track("/GND", "F.Cu", 98.825, 14.75, 98.2, 14.75, 97.7, 15)
	via("/GND", 97.7, 15)
	track("/GND", "B.Cu", 97.7, 15, 97.7, 13.3)
	track("/GND", "F.Cu", 101.175, 14.25, 101.8, 14.25, 101.8, 13.3)
	via("/GND", 101.8, 13.3)
	track("/GND", "F.Cu", 101.175, 14.75, 101.8, 14.75, 101.8, 14.25)
	track("/GND", "F.Cu", 100, 15, 100.65, 14.75, 101.175, 14.75)
	track("/GND", "F.Cu", 104.825, 16.5, 104.825, 13.3, 101.8, 13.3)
}
