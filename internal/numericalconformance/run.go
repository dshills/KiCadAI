package numericalconformance

import (
	"context"
	"errors"
	"fmt"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/schematicassistant"
	"kicadai/internal/simmodel"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Ngspice string
	Runner  checks.Runner
	CaseIDs []string
}
type Comparison struct {
	FixtureID         string  `json:"fixture_id"`
	Status            string  `json:"status"`
	FirstFailingStage string  `json:"first_failing_stage,omitempty"`
	Error             string  `json:"error,omitempty"`
	Expected          Value   `json:"analytical"`
	Go                *Value  `json:"go,omitempty"`
	External          *Value  `json:"ngspice,omitempty"`
	RelativeTolerance float64 `json:"relative_tolerance"`
	AbsoluteTolerance float64 `json:"absolute_tolerance"`
}
type Report struct {
	Schema            string                  `json:"schema"`
	Status            string                  `json:"status"`
	CorpusSHA256      string                  `json:"corpus_sha256"`
	RegistrySHA256    string                  `json:"registry_sha256"`
	Identity          evidencebundle.Identity `json:"identity"`
	ExternalStatus    string                  `json:"external_status"`
	ExternalVersion   string                  `json:"external_version,omitempty"`
	FirstFailingStage string                  `json:"first_failing_stage,omitempty"`
	Error             string                  `json:"error,omitempty"`
	Cases             []Comparison            `json:"cases"`
	Limitations       []string                `json:"limitations"`
}

var versionPattern = regexp.MustCompile(`(?i)\bngspice-([0-9]+(?:\.[0-9]+)*)\b`)

// Run retains discrepancies and operational failures in an atomic evidence bundle.
// Missing tools or a different release never count as external conformance.
func Run(ctx context.Context, output string, options Options) (Report, error) {
	var report Report
	if strings.TrimSpace(output) == "" {
		return report, errors.New("conformance requires a new output directory")
	}
	if err := evidencebundle.CheckDestination(output); err != nil {
		return report, err
	}
	fixtures := Fixtures()
	if len(options.CaseIDs) > 0 {
		fixtures = nil
		seen := map[string]bool{}
		for _, id := range options.CaseIDs {
			f, ok := Lookup(id)
			if !ok || seen[id] {
				return report, errors.New("unknown or duplicate frozen case")
			}
			seen[id] = true
			fixtures = append(fixtures, f)
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	report = Report{Schema: Schema, Status: "analytical_checked", CorpusSHA256: CorpusSHA256(), RegistrySHA256: simmodel.RegistryHash(), Identity: evidencebundle.CurrentIdentity(), ExternalStatus: "not_checked", Cases: []Comparison{}, Limitations: []string{"Only 12 frozen ideal passive-circuit cases; not coverage of nonlinear devices, switching, noise, thermal behavior or arbitrary circuits.", "Agreement is software conformance, not physical validation. Tolerances are frozen: 1e-8 V absolute plus 1e-7 relative for DC/AC, 0.5% relative for fixed-step transient.", "Evidence hashes are integrity checks, not independent attestation. Build revision may be dirty or unknown; executable hash identifies the actual program.", "ngspice source release is pinned to 47; executable content and version output are recorded, but the tool is not a sandbox for arbitrary SPICE."}}
	files := map[string][]byte{}
	put := func(name string, value any) error {
		raw, err := evidencebundle.JSON(value)
		if err == nil {
			files[name] = raw
		}
		return err
	}
	runner := options.Runner
	if runner == nil {
		runner = schematicassistant.SafeRunner{}
	}
	path := ""
	if options.Ngspice != "" {
		report.ExternalStatus = "blocked"
		var err error
		if !strings.ContainsAny(options.Ngspice, `/\`) {
			err = errors.New("explicit simulator path required")
		} else {
			path, err = filepath.Abs(options.Ngspice)
		}
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(path)
			if err == nil && !info.Mode().IsRegular() {
				err = errors.New("simulator is not a regular file")
			}
		}
		if err != nil {
			report.FirstFailingStage = "external_tool"
			report.Error = "ngspice executable unavailable"
		} else {
			versionCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			version := runner.Run(versionCtx, "", path, "-n", "--version")
			stop()
			if err = put("ngspice-version.json", version); err != nil {
				return report, err
			}
			match := versionPattern.FindStringSubmatch(version.Stdout + "\n" + version.Stderr)
			if version.Err != nil || version.ExitCode != 0 || len(match) != 2 || match[1] != NgspiceVersion {
				report.FirstFailingStage = "external_version"
				report.Error = "ngspice must report exactly release 47"
			} else {
				report.ExternalVersion = match[1]
				report.ExternalStatus = "checked"
				// Hash the selected executable, not credentials or the process environment.
				toolHash, readErr := evidencebundle.FileDigest(path)
				if readErr != nil {
					return report, readErr
				}
				if err = put("ngspice-identity.json", map[string]string{"sha256": toolHash, "release": NgspiceVersion, "upstream_source_archive_sha256": NgspiceSourceSHA256}); err != nil {
					return report, err
				}
			}
		}
	}
	for _, fixture := range fixtures {
		if err := ctx.Err(); err != nil {
			report.FirstFailingStage = "runtime_budget"
			report.Error = err.Error()
			break
		}
		prefix := "cases/" + fixture.ID + "/"
		if err := put(prefix+"input.json", fixture); err != nil {
			return report, err
		}
		files[prefix+"circuit.cir"] = []byte(fixture.Netlist())
		comparison := Comparison{FixtureID: fixture.ID, Status: "analytical_checked", Expected: fixture.Expected(), RelativeTolerance: fixture.Tolerance(), AbsoluteTolerance: 1e-8}
		plan, diagnostics := fixture.Plan()
		if err := put(prefix+"plan.json", plan); err != nil {
			return report, err
		}
		if len(diagnostics) > 0 {
			comparison.FirstFailingStage = "go_resolution"
			comparison.Error = fmt.Sprintf("%v", diagnostics)
		} else {
			goReport, issues := simmodel.Evaluate(plan)
			if err := put(prefix+"go-report.json", goReport); err != nil {
				return report, err
			}
			actual, err := observe(fixture, goReport)
			if err != nil || len(issues) > 0 {
				comparison.FirstFailingStage = "go_evaluation"
				comparison.Error = fmt.Sprintf("%v %v", err, issues)
			} else {
				comparison.Go = &actual
				if !Within(actual, comparison.Expected, fixture.Tolerance()) {
					comparison.FirstFailingStage = "go_vs_analytical"
					comparison.Error = "Go result differs from the frozen analytical oracle; expected values were not changed"
				}
			}
		}
		if report.ExternalStatus == "checked" {
			work, err := os.MkdirTemp("", "kicadai-conformance-")
			if err != nil {
				return report, err
			}
			result, externalErr := runExternal(ctx, runner, path, work, fixture)
			_ = os.RemoveAll(work)
			if err = put(prefix+"ngspice-command.json", result); err != nil {
				return report, err
			}
			if externalErr == nil {
				value, parseErr := parseExternal(result.Stdout)
				if parseErr != nil {
					externalErr = parseErr
				} else {
					comparison.External = &value
					if !Within(value, comparison.Expected, fixture.Tolerance()) || (comparison.Go != nil && !Within(*comparison.Go, value, fixture.Tolerance())) {
						externalErr = errors.New("external/Go/analytical comparison differs; retained original observations and expectations")
					}
				}
			}
			if externalErr != nil && comparison.FirstFailingStage == "" {
				comparison.FirstFailingStage = "external_comparison"
				comparison.Error = externalErr.Error()
			}
			if externalErr == nil && comparison.FirstFailingStage == "" {
				comparison.Status = "conformant"
			}
		}
		if comparison.FirstFailingStage != "" {
			comparison.Status = "failed"
			if report.FirstFailingStage == "" {
				report.FirstFailingStage = comparison.FirstFailingStage
				report.Error = fixture.ID + ": " + comparison.Error
			}
		}
		report.Cases = append(report.Cases, comparison)
	}
	if report.FirstFailingStage != "" {
		report.Status = "failed"
	} else if report.ExternalStatus == "checked" {
		report.Status = "conformant"
	}
	if err := put("report.json", report); err != nil {
		return report, err
	}
	files["REPLAY.txt"] = []byte("Run from the parent directory, replacing BUNDLE and NEW_OUTPUT with paths:\nkicadai --target BUNDLE --output NEW_OUTPUT conformance replay\nExternal execution is opt-in: add --ngspice /path/to/ngspice-47 before the command. No retries. No circuit minimization: each fixture is already a 3–4-device circuit.\n")
	if err := evidencebundle.Publish(output, files); err != nil {
		return report, err
	}
	if report.Status == "failed" {
		return report, fmt.Errorf("conformance failed at %s; evidence saved: %s", report.FirstFailingStage, report.Error)
	}
	return report, nil
}

func runExternal(ctx context.Context, runner checks.Runner, path, work string, f Fixture) (checks.CommandResult, error) {
	if err := os.WriteFile(filepath.Join(work, "circuit.cir"), []byte(f.Netlist()), 0600); err != nil {
		return checks.CommandResult{}, err
	}
	child, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := runner.Run(child, work, path, "-n", "-b", "circuit.cir") // -n ignores user/system .spiceinit startup commands.
	if result.Err != nil || result.ExitCode != 0 || child.Err() != nil {
		return result, fmt.Errorf("ngspice process failed or exceeded 15 seconds (exit %d)", result.ExitCode)
	}
	return result, nil
}
func parseExternal(text string) (Value, error) {
	values := map[string]float64{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[1] != "=" || (fields[0] != "observed_real" && fields[0] != "observed_imag") {
			continue
		}
		if _, exists := values[fields[0]]; exists {
			return Value{}, errors.New("duplicate external observation")
		}
		value, err := strconv.ParseFloat(fields[2], 64)
		if err != nil || !Within(Value{Real: value}, Value{Real: value}, 0) {
			return Value{}, errors.New("nonfinite or malformed external observation")
		}
		values[fields[0]] = value
	}
	if len(values) != 2 {
		return Value{}, errors.New("external observations missing")
	}
	return Value{values["observed_real"], values["observed_imag"]}, nil
}

func Inspect(root string) (Report, error) {
	var report Report
	if err := evidencebundle.Verify(root); err != nil {
		return report, err
	}
	raw, err := evidencebundle.Read(root, "report.json")
	if err != nil {
		return report, err
	}
	if err = evidencebundle.Decode(raw, &report); err != nil {
		return report, err
	}
	if report.Schema != Schema {
		return report, errors.New("unsupported conformance report schema")
	}
	return report, nil
}

func ReplayCases(root string) ([]string, error) {
	report, err := Inspect(root)
	if err != nil {
		return nil, err
	}
	if report.CorpusSHA256 != CorpusSHA256() || report.RegistrySHA256 != simmodel.RegistryHash() {
		return nil, errors.New("replay corpus/model identity differs from this executable")
	}
	var failed, all []string
	seen := map[string]bool{}
	for _, c := range report.Cases {
		fixture, ok := Lookup(c.FixtureID)
		if !ok || seen[c.FixtureID] {
			return nil, errors.New("invalid replay case inventory")
		}
		seen[c.FixtureID] = true
		raw, err := evidencebundle.Read(root, "cases/"+c.FixtureID+"/input.json")
		if err != nil {
			return nil, err
		}
		var input Fixture
		if err = evidencebundle.Decode(raw, &input); err != nil || input != fixture {
			return nil, errors.New("replay input differs from frozen fixture")
		}
		all = append(all, c.FixtureID)
		if c.Status == "failed" {
			failed = append(failed, c.FixtureID)
		}
	}
	if len(all) == 0 {
		return nil, errors.New("no replay cases")
	}
	if len(failed) > 0 {
		return failed, nil
	}
	return all, nil
}
