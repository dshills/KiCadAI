package boardfamily

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/kicadfiles/sexpr"
)

type CheckResult struct {
	Name    string  `json:"name"`
	Passed  bool    `json:"passed"`
	Seconds float64 `json:"seconds"`
	Error   string  `json:"error,omitempty"`
}
type Validation struct {
	Passed       bool              `json:"passed"`
	KiCadVersion string            `json:"kicad_version"`
	Seconds      float64           `json:"seconds"`
	Checks       []CheckResult     `json:"checks"`
	NativeSHA256 map[string]string `json:"native_sha256"`
}

func offlineEnv() []string {
	var env []string
	for _, s := range os.Environ() {
		k, _, _ := strings.Cut(s, "=")
		switch k {
		case "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_LIVE_PROVIDER_TESTS":
			continue
		}
		env = append(env, s)
	}
	return env
}
func command(ctx context.Context, cli, dir, name string, args ...string) error {
	c := exec.CommandContext(ctx, cli, args...)
	c.Env = offlineEnv()
	b, e := c.CombinedOutput()
	if w := os.WriteFile(filepath.Join(dir, name+".log"), b, 0644); w != nil {
		return w
	}
	if e != nil {
		return fmt.Errorf("%s failed (%v); see %s.log", name, e, name)
	}
	return nil
}

// Validate never changes a circuit to make it pass. Native round-trip writes are
// confined to temporary copies; a file digest gate verifies delivered inputs.
func Validate(ctx context.Context, dir, cli string) (Validation, error) {
	start := time.Now()
	v := Validation{Passed: true, NativeSHA256: map[string]string{}}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return v, err
	}
	check := func(name string, f func() error) {
		s := time.Now()
		e := f()
		r := CheckResult{Name: name, Passed: e == nil, Seconds: time.Since(s).Seconds()}
		if e != nil {
			r.Error = e.Error()
			v.Passed = false
		}
		v.Checks = append(v.Checks, r)
	}
	path := func(ext string) string { return filepath.Join(dir, "board."+ext) }
	for _, ext := range []string{"kicad_sch", "kicad_pcb", "kicad_pro"} {
		b, e := os.ReadFile(path(ext))
		if e != nil {
			return v, e
		}
		h := sha256.Sum256(b)
		v.NativeSHA256["board."+ext] = hex.EncodeToString(h[:])
	}
	check("electrical_contract", func() error {
		b, e := os.Open(filepath.Join(dir, "configuration.json"))
		if e != nil {
			return e
		}
		c, e := Decode(b)
		if e = errors.Join(e, b.Close()); e != nil {
			return e
		}
		_, e = Check(c)
		return e
	})
	check("reference_and_bom_integrity", func() (err error) {
		f, e := os.Open(filepath.Join(dir, "configuration.json"))
		if e != nil {
			return e
		}
		c, e := Decode(f)
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		tmp, e := os.MkdirTemp("", "board-family-integrity-")
		if e != nil {
			return e
		}
		defer func() { err = errors.Join(err, os.RemoveAll(tmp)) }()
		expected := filepath.Join(tmp, "project")
		if _, e = Generate(c, expected); e != nil {
			return e
		}
		return filepath.WalkDir(expected, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			rel, e := filepath.Rel(expected, p)
			if e != nil {
				return e
			}
			want, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			got, e := os.ReadFile(filepath.Join(dir, rel))
			if e != nil {
				return e
			}
			if !bytes.Equal(got, want) {
				return fmt.Errorf("%s differs from approved configuration/reference", rel)
			}
			return nil
		})
	})
	check("pcb_writer_and_connectivity", func() error {
		b, e := pcb.ReadFile(path("kicad_pcb"))
		if e != nil {
			return e
		}
		if e = pcb.Validate(b); e != nil {
			return e
		}
		return pcb.ValidateGeneratedConnectivity(b)
	})
	check("schematic_writer", func() error {
		s, e := schematic.ReadFile(path("kicad_sch"))
		if e != nil {
			return e
		}
		return schematic.Validate(s)
	})
	check("native_parse_render", func() error {
		for _, ext := range []string{"kicad_pcb", "kicad_sch"} {
			b, e := os.ReadFile(path(ext))
			if e != nil {
				return e
			}
			n, e := sexpr.Parse(b)
			if e != nil {
				return e
			}
			s, e := sexpr.Format(n.Node())
			if e != nil {
				return e
			}
			if roundtrip.NormalizeText(string(b)) != roundtrip.NormalizeText(s) {
				return errors.New("native parse/render changed tokens")
			}
		}
		return nil
	})
	check("kicad_version", func() error {
		c := exec.CommandContext(ctx, cli, "version")
		c.Env = offlineEnv()
		b, e := c.Output()
		if e != nil {
			return e
		}
		v.KiCadVersion = strings.TrimSpace(string(b))
		if v.KiCadVersion != "10.0.3" {
			return fmt.Errorf("reference qualified with KiCad 10.0.3, found %s; requalification required", v.KiCadVersion)
		}
		return nil
	})
	check("kicad_erc", func() error {
		r := filepath.Join(dir, "erc.json")
		if e := command(ctx, cli, dir, "erc", "sch", "erc", "--format", "json", "--severity-all", "--exit-code-violations", "--units", "mm", "--output", r, path("kicad_sch")); e != nil {
			return e
		}
		var x struct {
			Sheets []struct {
				Violations []json.RawMessage `json:"violations"`
			} `json:"sheets"`
		}
		b, e := os.ReadFile(r)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &x); e != nil {
			return e
		}
		if len(x.Sheets) == 0 {
			return errors.New("ERC report has no sheets")
		}
		for _, s := range x.Sheets {
			if s.Violations == nil || len(s.Violations) != 0 {
				return errors.New("ERC violations or incomplete report")
			}
		}
		return nil
	})
	check("kicad_strict_drc_and_parity", func() error {
		r := filepath.Join(dir, "drc.json")
		if e := command(ctx, cli, dir, "drc", "pcb", "drc", "--format", "json", "--severity-all", "--all-track-errors", "--schematic-parity", "--exit-code-violations", "--units", "mm", "--output", r, path("kicad_pcb")); e != nil {
			return e
		}
		var x map[string]json.RawMessage
		b, e := os.ReadFile(r)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &x); e != nil {
			return e
		}
		for _, k := range []string{"violations", "unconnected_items", "schematic_parity"} {
			var a []json.RawMessage
			if e = json.Unmarshal(x[k], &a); e != nil || a == nil || len(a) > 0 {
				return fmt.Errorf("DRC %s is nonempty or missing", k)
			}
		}
		return nil
	})
	for _, ext := range []string{"kicad_sch", "kicad_pcb"} {
		ext := ext
		check("kicad_roundtrip_"+ext, func() error {
			cl := roundtrip.KiCadCLI{Path: cli}
			opts := roundtrip.Options{Timeout: 90 * time.Second}
			var r roundtrip.Result
			var e error
			if ext == "kicad_pcb" {
				r, e = roundtrip.RoundTripPCB(ctx, cl, path(ext), opts)
			} else {
				r, e = roundtrip.RoundTripSchematic(ctx, cl, path(ext), opts)
			}
			if e != nil {
				return e
			}
			if !r.Equal {
				if e = writeJSON(filepath.Join(dir, "roundtrip-"+ext+"-differences.json"), r.Differences); e != nil {
					return e
				}
				return fmt.Errorf("round-trip changed native content (%d difference categories); see roundtrip-%s-differences.json", len(r.Differences), ext)
			}
			return nil
		})
	}
	check("schematic_preview", func() error {
		return command(ctx, cli, dir, "schematic-preview", "sch", "export", "svg", "--output", filepath.Join(dir, "preview")+string(os.PathSeparator), path("kicad_sch"))
	})
	check("pcb_preview", func() error {
		return command(ctx, cli, dir, "pcb-preview", "pcb", "export", "svg", "--layers", "F.Cu,B.Cu,F.SilkS,Edge.Cuts", "--mode-single", "--fit-page-to-board", "--exclude-drawing-sheet", "--output", filepath.Join(dir, "preview", "pcb.svg"), path("kicad_pcb"))
	})
	check("manufacturing_exports", func() error {
		if !v.Passed {
			return errors.New("manufacturing exports withheld because native validation failed")
		}
		return exportManufacturing(ctx, dir, cli)
	})
	check("input_immutability", func() error {
		for file, want := range v.NativeSHA256 {
			b, e := os.ReadFile(filepath.Join(dir, file))
			if e != nil {
				return e
			}
			h := sha256.Sum256(b)
			if hex.EncodeToString(h[:]) != want {
				return fmt.Errorf("validation changed %s", file)
			}
		}
		return nil
	})
	v.Seconds = time.Since(start).Seconds()
	if err = writeJSON(filepath.Join(dir, "validation.json"), v); err != nil {
		return v, err
	}
	if !v.Passed {
		return v, errors.New("board validation failed; see validation.json")
	}
	return v, nil
}
