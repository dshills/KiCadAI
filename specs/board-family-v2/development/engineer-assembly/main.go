// Development-only revision of the SHT31 mask/paste reference. Never repairs
// generated evaluation outputs. The previous canonical inputs remain immutable.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

type node = sexpr.ParsedNode

const oldName = "Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm"
const newName = "SHT31-DIS_SensirionV7_MaskPaste"
const oldPath = "footprints/Sensor_Humidity.pretty/" + oldName + ".kicad_mod"
const newPath = "footprints/Sensor_Humidity.pretty/" + newName + ".kicad_mod"

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func parse(b []byte) node         { n, e := sexpr.Parse(b); must(e); return n }
func expr(s string) node          { return parse([]byte(s)) }
func stringNode(s string) node    { return node{Quoted: true, String: s} }
func child(n node, h string) node { c, _ := n.Child(h); return c }
func hash(b []byte) string        { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func format(n node) []byte {
	s, e := sexpr.Format(n.Node())
	must(e)
	return []byte(strings.TrimRight(s, "\n") + "\n")
}
func set(n *node, h string, replacement node) {
	for i := range n.Children {
		if n.Children[i].Head() == h {
			n.Children[i] = replacement
			return
		}
	}
	n.Children = append(n.Children, replacement)
}
func uid(key string) string {
	b := sha256.Sum256([]byte("board-family-v2/sht31-assembly/" + key))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func reference(n node) string {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == "Reference" {
			return p.ListValue(2)
		}
	}
	return ""
}

func reviseFootprint(n node) node {
	// Deep copy: an independent before/after comparison must retain original nodes.
	n = parse(format(n))
	if n.ListValue(1) == oldName {
		n.Children[1] = stringNode(newName)
	} else if n.ListValue(1) == "Sensor_Humidity:"+oldName {
		n.Children[1] = stringNode("Sensor_Humidity:" + newName)
	} else {
		panic("unexpected footprint identity")
	}
	set(&n, "descr", expr(`(descr "KiCad-derived SHT31 reference; Sensirion December 2022 v7 section 5.3 mask/paste revision; copper unchanged; assembly process not qualified")`))
	// This is a local, manufacturer-directed mask grouping, not a global DRC waiver.
	set(&n, "attr", expr(`(attr smd allow_soldermask_bridges)`))
	count := 0
	var extra []node
	for i := range n.Children {
		p := &n.Children[i]
		if p.Head() == "property" && p.ListValue(1) == "Value" && p.ListValue(2) == oldName {
			p.Children[2] = stringNode(newName)
		}
		if p.Head() != "pad" {
			continue
		}
		id := p.ListValue(1)
		if id == "9" {
			var ordered []node
			for _, c := range p.Children {
				if c.Head() == "zone_connect" {
					ordered = append(ordered, expr(`(solder_mask_margin 0.075)`))
				}
				ordered = append(ordered, c)
			}
			p.Children = ordered
			continue
		}
		if id == "" {
			continue
		} // Retain the already reduced, chamfered center paste.
		if len(id) != 1 || id[0] < '1' || id[0] > '8' {
			panic("unexpected electrical pad")
		}
		at := child(*p, "at")
		x, okX := at.FloatValue(1)
		y, okY := at.FloatValue(2)
		if !okX || !okY || (x != -1.175 && x != 1.175) {
			panic("unexpected I/O pad geometry")
		}
		set(p, "layers", expr(`(layers "F.Cu")`))
		if x < 0 {
			x -= 0.1
		} else {
			x += 0.1
		}
		extra = append(extra, expr(fmt.Sprintf(`(pad "" smd rect (at %.3f %.2f) (size 0.55 0.25) (layers "F.Paste") (uuid %q))`, x, y, uid("paste/"+id))))
		count++
	}
	if count != 8 {
		panic("expected exactly eight I/O pads")
	}
	for _, bounds := range [][2]float64{{-1.525, -0.825}, {0.825, 1.525}} {
		extra = append(extra, expr(fmt.Sprintf(`(fp_rect (start %.3f -0.95) (end %.3f 0.95) (stroke (width 0) (type solid)) (fill yes) (layer "F.Mask") (uuid %q))`, bounds[0], bounds[1], uid(fmt.Sprintf("mask/%.3f", bounds[0])))))
	}
	n.Children = append(n.Children, extra...)
	return n
}

func renameSchematic(n *node) {
	for i := range n.Children {
		s := &n.Children[i]
		if s.Head() != "symbol" || reference(*s) != "U2" {
			continue
		}
		for j := range s.Children {
			p := &s.Children[j]
			if p.Head() == "property" && p.ListValue(1) == "Footprint" && p.ListValue(2) == "Sensor_Humidity:"+oldName {
				p.Children[2] = stringNode("Sensor_Humidity:" + newName)
			}
		}
	}
}

// Preserve all electrical-pad information except the intentionally changed
// mask/paste layer membership and exposed-pad mask expansion.
func copperPads(n node) string {
	result := expr(`(copper_pads)`)
	for _, p := range n.ChildrenByHead("pad") {
		if !strings.Contains(string(format(child(p, "layers"))), `"F.Cu"`) {
			continue
		}
		q := parse(format(p))
		var fields []node
		for _, c := range q.Children {
			if c.Head() != "solder_mask_margin" {
				fields = append(fields, c)
			}
		}
		q.Children = fields
		set(&q, "layers", expr(`(layers "F.Cu")`))
		result.Children = append(result.Children, q)
	}
	return string(format(result))
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: engineer-assembly NEW_DEVELOPMENT_DIR")
	}
	dst := filepath.Clean(os.Args[1])
	if !strings.HasPrefix(dst, ".cache/board-family-v2/sht31-assembly-") {
		panic("output must be a new assembly-development directory")
	}
	var original struct {
		Source string            `json:"source"`
		Files  map[string]string `json:"files_sha256"`
	}
	manifest, err := os.ReadFile("specs/board-family-v2/development/reference-import.json")
	must(err)
	must(json.Unmarshal(manifest, &original))
	inputs := map[string][]byte{}
	for file, expected := range original.Files {
		b, e := os.ReadFile(filepath.Join(original.Source, file))
		must(e)
		if hash(b) != expected {
			panic("historical canonical input changed: " + file)
		}
		inputs[file] = b
	}
	board := parse(inputs["board.kicad_pcb"])
	found := 0
	for i := range board.Children {
		if board.Children[i].Head() == "footprint" && reference(board.Children[i]) == "U2" {
			before := board.Children[i]
			board.Children[i] = reviseFootprint(before)
			if copperPads(before) != copperPads(board.Children[i]) {
				panic("U2 electrical copper changed")
			}
			found++
		}
	}
	if found != 1 {
		panic("expected exactly one U2")
	}
	sch := parse(inputs["board.kicad_sch"])
	renameSchematic(&sch)
	outputs := map[string][]byte{}
	for file, b := range inputs {
		outputs[file] = b
	}
	outputs["board.kicad_pcb"] = format(board)
	outputs["board.kicad_sch"] = format(sch)
	outputs[newPath] = format(reviseFootprint(parse(inputs[oldPath])))
	must(os.Mkdir(dst, 0755))
	identities := map[string]string{}
	for file, b := range outputs {
		p := filepath.Join(dst, file)
		must(os.MkdirAll(filepath.Dir(p), 0755))
		must(os.WriteFile(p, b, 0644))
		identities[file] = hash(b)
	}
	receipt := map[string]any{"status": "unqualified-development-reference", "source": original.Source, "source_manifest_sha256": hash(manifest), "source_files_sha256": original.Files, "files_sha256": identities, "evaluated_output_repair": false, "changes": []string{"U2-only custom footprint identity, I/O paste offset 0.1 mm outward, grouped 75 um-clearance row mask openings, 75 um exposed-pad mask clearance", "Unchanged copper pad geometry, electrical nets, placement, routing and board-level design rules", "Existing reduced center paste retained"}}
	b, err := json.MarshalIndent(receipt, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(dst, "engineering.json"), append(b, '\n'), 0644))
	fmt.Println(dst)
}
