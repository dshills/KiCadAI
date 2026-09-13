package boardfamily

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
)

func testConfig() Config { return Config{"1", Family, "standard", 3.2, 3.4, 1000, 10, 35, 200} }
func TestElectricalProfiles(t *testing.T) {
	for _, p := range Profiles() {
		t.Run(p.ID, func(t *testing.T) {
			c := testConfig()
			c.Profile = p.ID
			c.TotalBusCapacitancePF = p.MaxBusPF
			e, err := Check(c)
			if err != nil {
				t.Fatal(err)
			}
			if e.RiseTimeNS > p.RiseLimitNS || e.PullupSinkMA >= 3 || e.SourceMarginMA < 400 {
				t.Fatalf("insufficient margins: %+v", e)
			}
			// Independently account for the sensor's internal pull-up current.
			// An integer constant division previously erased this contribution.
			externalMA := c.SupplyMaxV / (p.ResistanceOhms * 0.9885) * 1000
			internalMA := c.SupplyMaxV / 70.0
			if math.Abs(e.PullupSinkMA-externalMA-internalMA) > 1e-12 {
				t.Fatalf("missing internal pull-up current: got %.12f, want %.12f mA", e.PullupSinkMA, externalMA+internalMA)
			}
		})
	}
}
func TestRejectUnsupportedConfigurations(t *testing.T) {
	cases := map[string]func(*Config){"family": func(c *Config) { c.Family = "other" }, "profile": func(c *Config) { c.Profile = "invented" }, "overvoltage": func(c *Config) { c.SupplyMaxV = 5 }, "undervoltage": func(c *Config) { c.SupplyMinV = 3.1 }, "source": func(c *Config) { c.SupplyCapacityMA = 500 }, "cold": func(c *Config) { c.AmbientMinC = 0 }, "hot": func(c *Config) { c.AmbientMaxC = 40 }, "inverted": func(c *Config) { c.AmbientMinC = 30; c.AmbientMaxC = 20 }, "capacitance": func(c *Config) { c.TotalBusCapacitancePF = 201 }, "missing": func(c *Config) { c.TotalBusCapacitancePF = 0 }, "nan": func(c *Config) { c.SupplyMinV = math.NaN() }, "infinity": func(c *Config) { c.SupplyMaxV = math.Inf(1) }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			change(&c)
			if _, e := Check(c); e == nil {
				t.Fatal("accepted unsupported condition")
			}
		})
	}
}
func TestDecodeStrict(t *testing.T) {
	b, _ := json.Marshal(testConfig())
	for _, s := range []string{string(b) + " {}", strings.TrimSuffix(string(b), "}") + `,"invented":true}`, `null`, string(b) + strings.Repeat(" ", 65536) + "{}"} {
		c, e := Decode(strings.NewReader(s))
		if e == nil {
			_, e = Check(c)
		}
		if e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestGenerateDeterministicAndNativeWriters(t *testing.T) {
	for _, p := range Profiles() {
		t.Run(p.ID, func(t *testing.T) {
			c := testConfig()
			c.Profile = p.ID
			c.TotalBusCapacitancePF = p.MaxBusPF
			root := t.TempDir()
			a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
			for _, d := range []string{a, b} {
				if _, e := Generate(c, d); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := Generate(c, a); e == nil {
				t.Fatal("overwrote output")
			}
			err := filepath.WalkDir(a, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(a, path)
				x, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				y, e := os.ReadFile(filepath.Join(b, rel))
				if e != nil {
					return e
				}
				if !bytes.Equal(x, y) {
					t.Errorf("nondeterministic %s", rel)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			board, e := pcb.ReadFile(filepath.Join(a, "board.kicad_pcb"))
			if e != nil {
				t.Fatal(e)
			}
			if e = pcb.Validate(board); e != nil {
				t.Fatal(e)
			}
			if e = pcb.ValidateGeneratedConnectivity(board); e != nil {
				t.Fatal(e)
			}
			sch, e := schematic.ReadFile(filepath.Join(a, "board.kicad_sch"))
			if e != nil {
				t.Fatal(e)
			}
			if e = schematic.Validate(sch); e != nil {
				t.Fatal(e)
			}
			var bom []Part
			x, _ := os.ReadFile(filepath.Join(a, "bom.json"))
			if e = json.Unmarshal(x, &bom); e != nil {
				t.Fatal(e)
			}
			if len(bom) != 17 {
				t.Fatal("incomplete BOM")
			}
			for _, x := range bom {
				if x.MPN == "" || x.Footprint == "" {
					t.Fatal("unspecified part", x)
				}
				if x.Reference == "R3" || x.Reference == "R4" {
					if x.MPN != p.MPN || x.Value != p.Value {
						t.Fatal("incorrect bus pullup", x)
					}
				}
			}
		})
	}
}
