// Read-only native-board inventory for the integrated engineering review.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/pcb"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: inspect-board BOARD.kicad_pcb")
	}
	b, err := pcb.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	nets := map[int]string{}
	for _, n := range b.Nets {
		nets[n.Code] = n.Name
	}
	type wire struct {
		LengthMM   float64 `json:"total_track_length_mm"`
		Squares    float64 `json:"sum_length_over_width"`
		MinWidthMM float64 `json:"minimum_track_width_mm"`
		Vias       int     `json:"vias"`
	}
	wires := map[string]*wire{}
	type edge struct {
		to      string
		squares float64
		via     bool
	}
	graphs := map[string]map[string][]edge{}
	point := func(x, y float64, layer string) string { return fmt.Sprintf("%.4f,%.4f,%s", x, y, layer) }
	join := func(net, a, z string, squares float64, via bool) {
		if graphs[net] == nil {
			graphs[net] = map[string][]edge{}
		}
		graphs[net][a] = append(graphs[net][a], edge{z, squares, via})
		graphs[net][z] = append(graphs[net][z], edge{a, squares, via})
	}
	for _, t := range b.Tracks {
		n := nets[t.NetCode]
		if wires[n] == nil {
			wires[n] = &wire{MinWidthMM: math.Inf(1)}
		}
		w := wires[n]
		l := math.Hypot(float64(t.End.X-t.Start.X), float64(t.End.Y-t.Start.Y)) / 1e6
		width := float64(t.Width) / 1e6
		w.LengthMM += l
		w.Squares += l / width
		w.MinWidthMM = math.Min(w.MinWidthMM, width)
		join(n, point(float64(t.Start.X)/1e6, float64(t.Start.Y)/1e6, string(t.Layer)), point(float64(t.End.X)/1e6, float64(t.End.Y)/1e6, string(t.Layer)), l/width, false)
	}
	for _, v := range b.Vias {
		if w := wires[nets[v.NetCode]]; w != nil {
			w.Vias++
		}
		join(nets[v.NetCode], point(float64(v.Position.X)/1e6, float64(v.Position.Y)/1e6, "F.Cu"), point(float64(v.Position.X)/1e6, float64(v.Position.Y)/1e6, "B.Cu"), 0, true)
	}
	type pin struct {
		Reference string  `json:"reference"`
		Number    string  `json:"number"`
		Net       string  `json:"net"`
		X         float64 `json:"x_mm"`
		Y         float64 `json:"y_mm"`
	}
	var pins []pin
	for _, f := range b.Footprints {
		for _, p := range f.Pads {
			x, y := kicadfiles.RotateBoardLocalXY(float64(p.Position.X)/1e6, float64(p.Position.Y)/1e6, float64(f.Rotation))
			pins = append(pins, pin{f.Reference, p.Name, nets[p.NetCode], x + float64(f.Position.X)/1e6, y + float64(f.Position.Y)/1e6})
			if strings.Contains(p.Type, "thru_hole") {
				last := pins[len(pins)-1]
				join(last.Net, point(last.X, last.Y, "F.Cu"), point(last.X, last.Y, "B.Cu"), 0, true)
			}
		}
	}
	// One available centerline path, not effective resistance of parallel copper.
	var paths []map[string]any
	for _, target := range []string{"U1:2", "U1:1", "U2:8", "U2:1"} {
		ref, num, _ := strings.Cut(target, ":")
		var dest, src pin
		for _, p := range pins {
			if p.Reference == ref && p.Number == num {
				dest = p
			}
		}
		for _, p := range pins {
			if p.Reference == "J1" && p.Net == dest.Net {
				src = p
			}
		}
		start, end := point(src.X, src.Y, "F.Cu"), point(dest.X, dest.Y, "F.Cu")
		dist := map[string]float64{start: 0}
		prev := map[string]string{}
		viaTo := map[string]bool{}
		done := map[string]bool{}
		for {
			n := ""
			best := math.Inf(1)
			for k, v := range dist {
				if !done[k] && v < best {
					n, best = k, v
				}
			}
			if n == "" || n == end {
				break
			}
			done[n] = true
			for _, e := range graphs[dest.Net][n] {
				d := best + e.squares
				if old, ok := dist[e.to]; !ok || d < old {
					dist[e.to] = d
					prev[e.to] = n
					viaTo[e.to] = e.via
				}
			}
		}
		r := map[string]any{"from": "J1:" + src.Number, "to": target, "net": dest.Net}
		if d, ok := dist[end]; ok {
			n := end
			vias := 0
			steps := []string{n}
			for n != start {
				if viaTo[n] {
					vias++
				}
				n = prev[n]
				steps = append(steps, n)
			}
			r["sum_length_over_width"] = d
			r["layer_transitions"] = vias
			r["reverse_path"] = steps
		} else {
			r["error"] = "no exact centerline path; no resistance claim"
		}
		paths = append(paths, r)
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if err := e.Encode(map[string]any{"source": os.Args[1], "thickness_mm": float64(b.General.Thickness) / 1e6, "tracks": len(b.Tracks), "track_arcs": len(b.TrackArcs), "wires": wires, "pins": pins, "power_paths": paths, "limitation": "Inventory, not a solved power-distribution or thermal model. Net-wide sum(length/width) includes branches; separate power_paths trace one available centerline route. Via plating and contact resistance are not inferred."}); err != nil {
		panic(fmt.Sprint(err))
	}
}
