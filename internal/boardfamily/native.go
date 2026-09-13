package boardfamily

import (
	"crypto/sha256"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

//go:embed reference
var reference embed.FS

type Part struct {
	Reference    string `json:"reference"`
	Value        string `json:"value"`
	Manufacturer string `json:"manufacturer"`
	MPN          string `json:"mpn"`
	Footprint    string `json:"footprint"`
}
type nativeNode = sexpr.ParsedNode

func field(n nativeNode, h string) string { c, _ := n.Child(h); return c.ListValue(1) }
func property(n nativeNode, k string) string {
	for _, p := range n.ChildrenByHead("property") {
		if p.ListValue(1) == k {
			return p.ListValue(2)
		}
	}
	return ""
}
func stringNode(s string) nativeNode { return nativeNode{String: s, Quoted: true} }
func walkNative(n *nativeNode, f func(*nativeNode)) {
	f(n)
	for i := range n.Children {
		walkNative(&n.Children[i], f)
	}
}
func nativeUUID(key string) string {
	b := sha256.Sum256([]byte("board-family-v1/" + key))
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func setProperty(n *nativeNode, k, v string, pcb bool) error {
	for i := range n.Children {
		p := &n.Children[i]
		if p.Head() == "property" && p.ListValue(1) == k {
			p.Children[2] = stringNode(v)
			return nil
		}
	}
	extra := ""
	if pcb {
		extra = fmt.Sprintf("(layer \"F.Fab\") (uuid %q)", nativeUUID(property(*n, "Reference")+"/property/"+k))
	}
	p, e := sexpr.Parse([]byte(fmt.Sprintf("(property %q %q (at 0 0 0) %s (hide yes) (effects (font (size 1 1))))", k, v, extra)))
	if e != nil {
		return e
	}
	n.Children = append(n.Children, p)
	return nil
}

func parts(p Profile) []Part {
	return []Part{
		{"C1", "100nF", "Murata", "GRM21BR71H104KA01L", ""}, {"C2", "10uF", "Murata", "GRM21BR61A106KE19L", ""}, {"C3", "1uF", "Murata", "GCM21BR71H105KA03L", ""},
		{"C4", "100nF", "Murata", "GRM21BR71H104KA01L", ""}, {"C5", "100nF", "Murata", "GRM21BR71H104KA01L", ""}, {"C6", "100nF", "Murata", "GRM21BR71H104KA01L", ""},
		{"J1", "3V3 INPUT", "Samtec", "TSW-102-07-L-S", ""}, {"J2", "3V3 UART", "Samtec", "TSW-104-07-L-S", ""}, {"J3", "GPIO I2C SPI", "Samtec", "TSW-106-07-L-S", ""},
		{"R1", "10k", "Yageo", "RC0805FR-0710KL", ""}, {"R2", "10k", "Yageo", "RC0805FR-0710KL", ""}, {"R3", p.Value, "Yageo", p.MPN, ""}, {"R4", p.Value, "Yageo", p.MPN, ""},
		{"SW1", "RESET", "Omron", "B3U-1000P", ""}, {"SW2", "BOOT", "Omron", "B3U-1000P", ""},
		{"U1", "ESP32-WROOM-32E-N4", "Espressif", "ESP32-WROOM-32E-N4", ""}, {"U2", "BMP280 0x76", "Bosch Sensortec", "BMP280", ""},
	}
}

// Generate writes an exclusively-created output. Native geometry comes only from
// the engineered reference. All native/round-trip validation is a separate gate.
func Generate(c Config, dir string) (Electrical, error) {
	e, err := Check(c)
	if err != nil {
		return e, err
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return e, err
	}
	if err = os.Mkdir(dir, 0755); err != nil {
		return e, fmt.Errorf("output must be new: %w", err)
	}
	err = fs.WalkDir(reference, "reference", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		r := strings.TrimPrefix(p, "reference")
		r = strings.TrimPrefix(r, "/")
		dest := filepath.Join(dir, filepath.FromSlash(r))
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		b, err := reference.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, b, 0644)
	})
	if err != nil {
		return e, err
	}
	ps := parts(e.Profile)
	byRef := map[string]*Part{}
	for i := range ps {
		byRef[ps[i].Reference] = &ps[i]
	}
	for _, ext := range []string{"kicad_sch", "kicad_pcb"} {
		path := filepath.Join(dir, "board."+ext)
		b, err := os.ReadFile(path)
		if err != nil {
			return e, err
		}
		root, err := sexpr.Parse(b)
		if err != nil {
			return e, err
		}
		seen := map[string]bool{}
		for i := range root.Children {
			n := &root.Children[i]
			if n.Head() != "symbol" && n.Head() != "footprint" {
				continue
			}
			r := property(*n, "Reference")
			p := byRef[r]
			if p == nil {
				continue
			}
			seen[r] = true
			for _, kv := range [][2]string{{"Value", p.Value}, {"Manufacturer", p.Manufacturer}, {"MPN", p.MPN}, {"KiCadAI Component ID", "boardfamily.v1." + p.MPN}, {"Component Source", "board-family-v1 reviewed BOM"}, {"Component Confidence", "application_reviewed_not_bench_tested"}} {
				if err = setProperty(n, kv[0], kv[1], ext == "kicad_pcb"); err != nil {
					return e, err
				}
			}
			for j := range n.Children {
				t := &n.Children[j]
				if t.Head() == "fp_text" && t.ListValue(1) == "value" {
					t.Children[2] = stringNode(p.Value)
				}
				if t.Head() == "instances" {
					walkNative(t, func(v *nativeNode) {
						if v.Head() == "value" && len(v.Children) == 2 {
							v.Children[1] = stringNode(p.Value)
						}
					})
				}
			}
			if ext == "kicad_sch" {
				p.Footprint = property(*n, "Footprint")
			}
		}
		for r := range byRef {
			if !seen[r] {
				return e, fmt.Errorf("reference missing %s in %s", r, ext)
			}
		}
		s, err := sexpr.Format(root.Node())
		if err != nil {
			return e, err
		}
		if err = os.WriteFile(path, []byte(s+"\n"), 0644); err != nil {
			return e, err
		}
	}
	for name, v := range map[string]any{"configuration.json": c, "electrical.json": e, "bom.json": ps} {
		if err = writeJSON(filepath.Join(dir, name), v); err != nil {
			return e, err
		}
	}
	f, err := os.Create(filepath.Join(dir, "bom.csv"))
	if err != nil {
		return e, err
	}
	w := csv.NewWriter(f)
	err = w.Write([]string{"Reference", "Value", "Manufacturer", "MPN", "Footprint"})
	for _, p := range ps {
		if err != nil {
			break
		}
		err = w.Write([]string{p.Reference, p.Value, p.Manufacturer, p.MPN, p.Footprint})
	}
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	return e, err
}

func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
