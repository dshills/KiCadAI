package main

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

func uid(key string) string {
	b := sha256.Sum256([]byte("board-family-v1/" + key))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func set(n *node, head string, replacement node) {
	for i := range n.Children {
		if n.Children[i].Head() == head {
			n.Children[i] = replacement
			return
		}
	}
	n.Children = append(n.Children, replacement)
}
func number(n node, i int) float64 {
	v, ok := n.FloatValue(i)
	if !ok {
		panic("invalid coordinate")
	}
	return v
}

// Rebuild drawing-only geometry from the exported electrical pin partition.
// Symbols and pin definitions remain the installed/resolved KiCad symbols.
func relayout(sch *node, pinNets map[string]string, renames map[string]string) {
	newNets := map[string]string{}
	for k, n := range pinNets {
		r, p, _ := strings.Cut(k, ".")
		newNets[renames[r]+"."+p] = strings.TrimPrefix(n, "/")
	}
	newNets["F1.1"] = "VCC"
	newNets["F2.1"] = "GND"
	libs := map[string]node{}
	for _, l := range child(*sch, "lib_symbols").ChildrenByHead("symbol") {
		libs[l.ListValue(1)] = l
	}
	positions := map[string][2]float64{
		"J1": {45, 45}, "J2": {170, 115}, "J3": {170, 165},
		"U1": {100, 100}, "U2": {280, 65},
		"C1": {30, 90}, "C2": {55, 90}, "C3": {30, 135}, "C4": {245, 130},
		"C5": {245, 165}, "C6": {280, 165},
		"R1": {55, 135}, "R2": {55, 190}, "R3": {280, 130}, "R4": {320, 130},
		"S1": {55, 165}, "S2": {90, 190}, "SW1": {55, 165}, "SW2": {90, 190},
		"F1": {30, 225}, "F2": {65, 225},
	}
	keep := []node{}
	for _, n := range sch.Children {
		switch n.Head() {
		case "wire", "junction", "label", "global_label", "no_connect", "text", "polyline", "title_block":
			continue
		case "paper":
			n = parse("(paper \"A3\")")
		}
		keep = append(keep, n)
	}
	sch.Children = keep
	appendix := []node{}
	add := func(s string) {
		appendix = append(appendix, parse(strings.ReplaceAll(s, "(width 0)", "(width 0.1524)")))
	}
	for i := range sch.Children {
		s := &sch.Children[i]
		if s.Head() != "symbol" {
			continue
		}
		ref := prop(*s, "Reference")
		xy, ok := positions[ref]
		if !ok {
			panic("unplaced reference " + ref)
		}
		x, y := math.Round(xy[0]/1.27)*1.27, math.Round(xy[1]/1.27)*1.27
		set(s, "at", parse(fmt.Sprintf("(at %.4f %.4f 0)", x, y)))
		lib := libs[value(*s, "lib_id")]
		pins := []node{}
		walk(&lib, func(n *node) {
			if n.Head() == "pin" && len(n.Children) > 3 {
				pins = append(pins, *n)
			}
		})
		minY := y
		for _, p := range pins {
			a := child(p, "at")
			py := y - number(a, 2)
			if py < minY {
				minY = py
			}
		}
		for j := range s.Children {
			p := &s.Children[j]
			if p.Head() != "property" {
				continue
			}
			name := p.ListValue(1)
			if name != "Reference" && name != "Value" {
				set(p, "hide", parse("(hide yes)"))
				continue
			}
			px, py := x+6, y-1.5
			if name == "Value" {
				py = y + 1.5
			}
			if ref[0] == 'U' || ref[0] == 'J' {
				px = x + 8
				py = minY - 9
				if name == "Value" {
					py += 3
				}
			}
			set(p, "at", parse(fmt.Sprintf("(at %.4f %.4f 0)", px, py)))
			set(p, "effects", parse("(effects (font (size 1.27 1.27)) (justify left))"))
		}
		seen := map[string]bool{}
		for _, p := range pins {
			a := child(p, "at")
			px, py := x+number(a, 1), y-number(a, 2)
			pin := value(p, "number")
			nums := strings.Split(strings.Trim(pin, "[]"), ",")
			net := newNets[ref+"."+nums[0]]
			for _, n := range nums[1:] {
				if newNets[ref+"."+n] != net {
					panic("grouped pin net mismatch")
				}
			}
			key := fmt.Sprintf("%s/%.4f/%.4f/%s", ref, px, py, net)
			if seen[key] {
				continue
			}
			seen[key] = true
			if net == "" {
				add(fmt.Sprintf("(no_connect (at %.4f %.4f) (uuid %q))", px, py, uid(key+"nc")))
				continue
			}
			dx, dy := 0.0, 0.0
			angle := number(a, 3)
			switch angle {
			case 0:
				dx = -7.62
			case 180:
				dx = 7.62
			case 90:
				dy = 7.62
			case 270:
				dy = -7.62
			default:
				panic("unsupported pin orientation")
			}
			// Merge the ESP32's bottom ground comb into one readable label.
			if ref == "U1" && net == "GND" && dy > 0 {
				ex := x - 5.08
				ey := py + 7.62
				add(fmt.Sprintf("(wire (pts (xy %.4f %.4f) (xy %.4f %.4f)) (stroke (width 0) (type default)) (uuid %q))", px, py, px, ey, uid(key+"v")))
				if px != ex {
					add(fmt.Sprintf("(wire (pts (xy %.4f %.4f) (xy %.4f %.4f)) (stroke (width 0) (type default)) (uuid %q))", px, ey, ex, ey, uid(key+"h")))
				}
				if !seen["gndlabel"] {
					seen["gndlabel"] = true
					add(fmt.Sprintf("(label %q (at %.4f %.4f 0) (effects (font (size 1 1)) (justify right bottom)) (uuid %q))", net, ex, ey, uid(ref+"gndlabel")))
				}
				continue
			}
			ex, ey := px+dx, py+dy
			add(fmt.Sprintf("(wire (pts (xy %.4f %.4f) (xy %.4f %.4f)) (stroke (width 0) (type default)) (uuid %q))", px, py, ex, ey, uid(key+"wire")))
			justify := "left"
			if dx < 0 {
				justify = "right"
			}
			add(fmt.Sprintf("(label %q (at %.4f %.4f 0) (effects (font (size 1 1)) (justify %s bottom)) (uuid %q))", net, ex, ey, justify, uid(key+"label")))
		}
	}
	for i, t := range []struct {
		x, y float64
		s    string
	}{{20, 18, "3.3 V SENSOR CONTROLLER — BMP280 reference"}, {20, 25, "External regulated 3.3 V input only. No USB power, batteries, or 5 V logic."}, {20, 65, "POWER DECOUPLING"}, {20, 115, "RESET / ENABLE"}, {20, 180, "BOOT SELECT"}, {85, 35, "CONTROLLER"}, {230, 25, "ON-BOARD I2C SENSOR"}, {230, 110, "SENSOR DECOUPLING / BUS PULLUPS"}, {155, 90, "3.3 V UART PROGRAMMING"}, {155, 140, "PERIPHERAL HEADER"}, {20, 245, "Development reference: software checks only; not manufactured or bench-tested."}} {
		add(fmt.Sprintf("(text %q (at %.4f %.4f 0) (effects (font (size 1.5 1.5)) (justify left)) (uuid %q))", t.s, t.x, t.y, uid(fmt.Sprintf("section%d", i))))
	}
	sch.Children = append(sch.Children, appendix...)
}
