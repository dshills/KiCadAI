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
	"kicadai/internal/kicadfiles/sexpr"
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
	for _, family := range []string{Family, FamilySHT31} {
		for _, p := range ProfilesFor(family) {
			t.Run(family+"/"+p.ID, func(t *testing.T) {
				c := testConfig()
				c.Family = family
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
				count := 17
				if family == FamilySHT31 {
					count = 16
				}
				if len(bom) != count {
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
}

func TestSHT31ElectricalContract(t *testing.T) {
	if ProfilesFor("unknown") != nil || len(ProfilesFor(FamilySHT31)) != 2 {
		t.Fatal("unexpected family profiles")
	}
	for _, p := range ProfilesFor(FamilySHT31) {
		for _, bus := range []float64{50, p.MaxBusPF} {
			c := testConfig()
			c.Family, c.Profile, c.TotalBusCapacitancePF = FamilySHT31, p.ID, bus
			e, err := Check(c)
			if err != nil {
				t.Fatal(err)
			}
			wantSink := 3.4 / (p.ResistanceOhms * 0.9885) * 1000
			if math.Abs(e.PullupSinkMA-wantSink) > 1e-12 {
				t.Fatal("SHT31 inherited Bosch internal pull-up", e)
			}
			if math.Abs(e.AllocatedCurrentMA-(593.5+2*wantSink)) > 1e-10 {
				t.Fatal("incorrect SHT31 current allocation", e)
			}
			if e.RiseTimeNS > 300 || e.Profile.RiseLimitNS != 300 {
				t.Fatal("SHT31 requires fast-mode rise even at 100 kHz", e)
			}
			notes := strings.Join(e.Notes, " ")
			for _, required := range []string{"heater off", "CRC", "20 V/ms", "no firmware"} {
				if !strings.Contains(strings.ToLower(notes), strings.ToLower(required)) {
					t.Fatalf("missing condition: %s", required)
				}
			}
			if strings.Contains(notes, "BMP280") {
				t.Fatal("wrong sensor firmware instructions")
			}
			c.TotalBusCapacitancePF = p.MaxBusPF + 0.01
			if _, err := Check(c); err == nil {
				t.Fatal("accepted excess SHT31 loading")
			}
		}
	}
	c := testConfig()
	c.Family, c.Profile, c.TotalBusCapacitancePF = FamilySHT31, "low_current", 50
	if _, err := Check(c); err == nil {
		t.Fatal("accepted unqualified low-current SHT31 profile")
	}
}

func TestBMP280PublishedOutputCompatibility(t *testing.T) {
	for _, p := range Profiles() {
		t.Run(p.ID, func(t *testing.T) {
			published := filepath.Join("..", "..", "examples", "board-family-v1", p.ID)
			b, err := os.ReadFile(filepath.Join(published, "configuration.json"))
			if err != nil {
				t.Fatal(err)
			}
			c, err := Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			generated := filepath.Join(t.TempDir(), "new")
			if _, err := Generate(c, generated); err != nil {
				t.Fatal(err)
			}
			err = filepath.WalkDir(generated, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(generated, path)
				if err != nil {
					return err
				}
				got, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				want, err := os.ReadFile(filepath.Join(published, rel))
				if err != nil {
					return err
				}
				if !bytes.Equal(got, want) {
					t.Errorf("first-family output changed: %s", rel)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSHT31SensorBindings(t *testing.T) {
	c := testConfig()
	c.Family, c.TotalBusCapacitancePF = FamilySHT31, 70
	dir := filepath.Join(t.TempDir(), "board")
	if _, err := Generate(c, dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "board.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := sexpr.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	var sensor nativeNode
	for _, fp := range root.ChildrenByHead("footprint") {
		if property(fp, "Reference") == "C6" {
			t.Fatal("obsolete sensor capacitor retained")
		}
		if property(fp, "Reference") == "U2" {
			sensor = fp
		}
	}
	if property(sensor, "MPN") != "SHT31-DIS-B2.5kS" || !strings.HasPrefix(sensor.ListValue(1), "Sensor_Humidity:") {
		t.Fatal("wrong sensor identity")
	}
	want := map[string]string{"1": "/SDA", "2": "/GND", "3": "", "4": "/SCL", "5": "/VCC", "6": "", "7": "/GND", "8": "/GND", "9": "/GND"}
	seen := map[string]bool{}
	for _, pad := range sensor.ChildrenByHead("pad") {
		pin := pad.ListValue(1)
		if pin == "" {
			continue
		} // paste-only aperture is not an electrical pin
		net := field(pad, "net")
		expected, ok := want[pin]
		if !ok || net != expected {
			t.Errorf("pin %s = %q, want %q", pin, net, expected)
		}
		seen[pin] = true
	}
	if len(seen) != len(want) {
		t.Fatal("incomplete sensor pads", seen)
	}
}
