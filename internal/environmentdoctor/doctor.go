// Package environmentdoctor performs local, credential-free readiness probes.
package environmentdoctor

import (
	"context"
	"fmt"
	"kicadai/internal/boardfamily"
	"kicadai/internal/buildinfo"
	"kicadai/internal/components"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/schematicassistant"
	"kicadai/internal/simmodel"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Schema = "kicadai.environment-doctor.v1"

type Probe struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"`
}
type Report struct {
	Schema            string                       `json:"schema"`
	Status            string                       `json:"status"`
	Build             buildinfo.Info               `json:"build"`
	Probes            []Probe                      `json:"probes"`
	Roots             libraryresolver.LibraryRoots `json:"library_roots"`
	RegistrySHA256    string                       `json:"registry_sha256"`
	APIRequired       bool                         `json:"api_required"`
	HardwareValidated bool                         `json:"hardware_validated"`
}
type Options struct {
	Roots      libraryresolver.LibraryRoots
	KiCadCLI   string
	RequireERC bool
	Runner     checks.Runner
}

func DetectRoots(roots libraryresolver.LibraryRoots) libraryresolver.LibraryRoots {
	var bases []string
	switch runtime.GOOS {
	case "darwin":
		bases = []string{"/Applications/KiCad/KiCad.app/Contents/SharedSupport"}
	case "linux":
		bases = []string{"/usr/share/kicad", "/usr/local/share/kicad"}
	case "windows":
		if base := os.Getenv("ProgramFiles"); base != "" {
			bases = []string{filepath.Join(base, "KiCad", "10.0", "share", "kicad")}
		}
	}
	find := func(current, sub string) string {
		if current != "" {
			return current
		}
		for _, base := range bases {
			p := filepath.Join(base, sub)
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return p
			}
		}
		return ""
	}
	roots.SymbolsRoot = find(roots.SymbolsRoot, "symbols")
	roots.FootprintsRoot = find(roots.FootprintsRoot, "footprints")
	roots.TemplatesRoot = find(roots.TemplatesRoot, "template")
	return roots
}

func Run(ctx context.Context, options Options) Report {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	report := Report{Schema: Schema, Status: "offline_ready", Build: buildinfo.Current(), Roots: DetectRoots(options.Roots), RegistrySHA256: simmodel.RegistryHash(), Probes: []Probe{}}
	add := func(name, status, detail, action string) {
		report.Probes = append(report.Probes, Probe{name, status, detail, action})
		if status == "failed" {
			report.Status = "blocked"
		}
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		add("embedded_catalog", "failed", err.Error(), "Reinstall a complete released binary.")
	} else if validation := components.ValidateCatalog(catalog); !validation.OK {
		add("embedded_catalog", "failed", "Embedded catalog validation failed.", "Inspect component validate diagnostics.")
	} else {
		add("embedded_catalog", "checked", fmt.Sprintf("%d embedded records; no repository-relative catalog needed.", len(catalog.Records)), "")
	}
	_, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) > 0 {
		add("model_provenance", "failed", diagnostics[0].Message, "Reinstall or inspect model registry evidence.")
	} else {
		add("model_provenance", "checked", "Embedded provenance registry validated; not a new numerical or hardware test.", "")
	}
	for _, family := range schematicassistant.Families() {
		_, err := schematicassistant.Example(family)
		if err != nil {
			add("specification/"+family, "failed", err.Error(), "Inspect the confirmed-specification lane.")
		} else {
			add("specification/"+family, "checked", "Explicit example confirms and passes analytical bounds; generation/ERC not run by doctor.", "")
		}
	}
	for _, profile := range boardfamily.Profiles() {
		config := boardfamily.Config{Version: "1", Family: boardfamily.Family, Profile: profile.ID, SupplyMinV: 3.2, SupplyMaxV: 3.4, SupplyCapacityMA: 1000, AmbientMinC: 10, AmbientMaxC: 35, TotalBusCapacitancePF: profile.MaxBusPF}
		if _, err := boardfamily.Check(config); err != nil {
			add("board_family/"+profile.ID, "failed", err.Error(), "Inspect the board family contract.")
		} else {
			add("board_family/"+profile.ID, "checked", "Embedded profile electrical bounds accepted; no board or manufacturing validation.", "")
		}
	}
	for _, probe := range []struct{ name, path string }{{"symbol_root", report.Roots.SymbolsRoot}, {"footprint_root", report.Roots.FootprintsRoot}} {
		if info, err := os.Stat(probe.path); probe.path == "" || err != nil || !info.IsDir() {
			add(probe.name, "missing_optional", "External library directory unavailable; offline schematic examples still work.", "Install KiCad libraries or set --symbols-root / --footprints-root.")
		} else {
			add(probe.name, "discovered", "Directory exists: "+probe.path+". This is discovery, not full library validation.", "Use library index and component validation for content checks.")
		}
	}
	cli, err := checks.DiscoverCLI(options.KiCadCLI)
	if err != nil {
		status := "missing_optional"
		if options.RequireERC || options.KiCadCLI != "" {
			status = "failed"
		}
		add("kicad_cli", status, "KiCad CLI unavailable; no native validation possible.", "Install supported KiCad 10.0.3 or supply --kicad-cli.")
	} else {
		runner := options.Runner
		if runner == nil {
			runner = schematicassistant.SafeRunner{}
		}
		child, stop := context.WithTimeout(ctx, 10*time.Second)
		version, err := runner.Version(child, cli.Path)
		stop()
		if err != nil || strings.TrimSpace(version) == "" {
			status := "missing_optional"
			if options.RequireERC || options.KiCadCLI != "" {
				status = "failed"
			}
			add("kicad_cli", status, "Version probe failed; native checks not established.", "Run the selected KiCad CLI --version manually.")
		} else if strings.TrimSpace(version) != "10.0.3" {
			status := "different_reference"
			if options.RequireERC {
				status = "failed"
			}
			add("kicad_cli", status, "Detected "+strings.TrimSpace(version)+"; supported reference is 10.0.3. No ERC/DRC was run.", "Use the reference release for qualified native evidence.")
		} else {
			add("kicad_cli", "version_checked", "KiCad 10.0.3 detected; version probe only, no ERC/DRC.", "")
		}
	}
	if err := ctx.Err(); err != nil {
		add("runtime_budget", "failed", err.Error(), "Rerun after resolving the stalled local probe.")
	}
	return report
}
