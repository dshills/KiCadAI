package boardfamily

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/pcb"
)

func TestLayoutAdmission(t *testing.T) {
	good := LayoutSpec{"board-layout-1", "120x70", "west", "south", "east"}
	for _, mutate := range []func(*LayoutSpec){func(s *LayoutSpec) { s.Size = "100x60" }, func(s *LayoutSpec) { s.Version = "2" }, func(s *LayoutSpec) { s.Power = "east" }, func(s *LayoutSpec) { s.UART = "west" }, func(s *LayoutSpec) { s.GPIO = "south" }} {
		s := good
		mutate(&s)
		if _, err := ReviewLayout(s); err == nil {
			t.Fatal("invalid layout accepted", s)
		}
	}
	for _, raw := range []string{`{"version":"board-layout-1","size":"120x70","size":"140x80","power_connector":"west","uart_connector":"south","gpio_connector":"east"}`, `{"version":"board-layout-1","size":"120x70","power_connector":"west","uart_connector":"south","gpio_connector":"east","rotation":90}`} {
		if _, err := DecodeLayout(strings.NewReader(raw)); err == nil {
			t.Fatal("ambiguous layout accepted")
		}
	}
}

func TestLayoutGeneration(t *testing.T) {
	c := testConfig()
	s := LayoutSpec{"board-layout-1", "120x70", "west", "south", "east"}
	dir := filepath.Join(t.TempDir(), "board")
	if _, err := GenerateWithLayout(c, &s, dir); err != nil {
		t.Fatal(err)
	}
	b, err := pcb.ReadFile(filepath.Join(dir, "board.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	if err = pcb.ValidateGeneratedConnectivity(b); err != nil {
		t.Fatal(err)
	}
}

func layoutForMask(size string, mask int) LayoutSpec {
	s := LayoutSpec{"board-layout-1", size, "inboard", "inboard", "inboard"}
	if mask&1 != 0 {
		s.Power = "west"
	}
	if mask&2 != 0 {
		s.UART = "south"
	}
	if mask&4 != 0 {
		s.GPIO = "east"
	}
	return s
}

func TestLayoutCombinationsReproduce(t *testing.T) {
	for _, family := range []FamilySpec{Catalog()[0], Catalog()[1]} {
		for _, size := range []string{"120x70", "140x80"} {
			for mask := 0; mask < 8; mask++ {
				t.Run(fmt.Sprintf("%s/%s/%d", family.ID, size, mask), func(t *testing.T) {
					s := layoutForMask(size, mask)
					c := family.DefaultConfiguration
					base := t.TempDir()
					for _, name := range []string{"first", "second"} {
						if _, err := GenerateWithLayout(c, &s, filepath.Join(base, name)); err != nil {
							t.Fatal(err)
						}
					}
					for _, name := range []string{"board.kicad_pcb", "board.kicad_sch", "board.kicad_pro", "bom.json", "layout.json", "layout-report.json"} {
						a, err := os.ReadFile(filepath.Join(base, "first", name))
						if err != nil {
							t.Fatal(err)
						}
						b, err := os.ReadFile(filepath.Join(base, "second", name))
						if err != nil || !bytes.Equal(a, b) {
							t.Fatal("nonreproducible", name, err)
						}
					}
					b, err := pcb.ReadFile(filepath.Join(base, "first", "board.kicad_pcb"))
					if err != nil {
						t.Fatal(err)
					}
					if err = pcb.ValidateGeneratedConnectivity(b); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestLayoutConfirmationBinding(t *testing.T) {
	c := Catalog()[0].DefaultConfiguration
	s := Specification{Version: SpecificationVersion, Origin: "manual", Configuration: &c, Unresolved: []string{}, ReviewNotes: []string{}}
	r, err := ReviewSpecification(s)
	if err != nil {
		t.Fatal(err)
	}
	layout := layoutForMask("120x70", 7)
	s.Layout = &layout
	if _, err := ConfirmSpecification(s, r.SHA256); err == nil {
		t.Fatal("old review accepted changed layout")
	}
	r, err = ReviewSpecification(s)
	if err != nil || !r.Ready || r.Layout == nil {
		t.Fatal(err, r)
	}
	confirmed, err := ConfirmSpecification(s, r.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	confirmed.Specification.Layout.Size = "140x80"
	if _, err := VerifyConfirmedSpecification(confirmed); err == nil {
		t.Fatal("modified layout confirmation accepted")
	}
}

func TestLayoutNative(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if cli == "" {
		t.Skip("requires installed KiCad")
	}
	for _, family := range Catalog() {
		for _, profile := range family.Profiles {
			for _, size := range []string{"120x70", "140x80"} {
				for mask := 0; mask < 8; mask++ {
					label := fmt.Sprintf("%s/%s/%s/%d", family.ID, profile.ID, size, mask)
					t.Run(label, func(t *testing.T) {
						t.Parallel()
						c := family.DefaultConfiguration
						c.Profile = profile.ID
						c.TotalBusCapacitancePF = profile.MaxBusPF
						s := layoutForMask(size, mask)
						dir := filepath.Join(t.TempDir(), "board")
						if base := os.Getenv("KICADAI_LAYOUT_EVIDENCE_ROOT"); base != "" {
							dir = filepath.Join(base, filepath.FromSlash(label))
						}
						if _, err := GenerateWithLayout(c, &s, dir); err != nil {
							t.Fatal(err)
						}
						v, err := Validate(context.Background(), dir, cli)
						if err != nil || !v.Passed {
							t.Fatalf("validation: %v %+v", err, v.Checks)
						}
					})
				}
			}
		}
	}
}
