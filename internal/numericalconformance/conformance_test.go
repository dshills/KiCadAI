package numericalconformance

import (
	"context"
	"errors"
	"fmt"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/simmodel"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConformanceFrozenAnalyticalAndGo(t *testing.T) {
	if len(Fixtures()) != 12 || CorpusSHA256() != "34d0a6be649cbf4c76a9411fecdb94f959ff38df2d9bb6e29838907605268261" {
		t.Fatal("frozen corpus changed; investigate, do not silently reset oracle")
	}
	known := map[string]Value{"divider-equal": {2.5, 0}, "divider-loaded": {5.0 / 3, 0}, "divider-asymmetric": {1.0521739130434782, 0}, "divider-high-resistance": {.0000999000000999, 0}, "rc-ac-load-0-ratio-1": {.5, -.5}, "rc-ac-load-10000-ratio-1": {.25, -.25}, "rc-step-load-0": {3.1606027941427883, 0}, "rc-step-load-10000": {1.5803013970713942, 0}}
	for _, f := range Fixtures() {
		t.Run(f.ID, func(t *testing.T) {
			if expected, ok := known[f.ID]; ok && !Within(f.Expected(), expected, 1e-12) {
				t.Fatalf("analytical drift: %+v", f.Expected())
			}
			plan, issues := f.Plan()
			if len(issues) > 0 {
				t.Fatalf("resolve: %v", issues)
			}
			report, issues := simmodel.Evaluate(plan)
			if len(issues) > 0 {
				t.Fatalf("evaluate: %v", issues)
			}
			actual, err := observe(f, report)
			if err != nil {
				t.Fatal(err)
			}
			if !Within(actual, f.Expected(), f.Tolerance()) {
				t.Fatalf("Go=%+v analytical=%+v", actual, f.Expected())
			}
		})
	}
	if Within(Value{Real: math.NaN()}, Value{}, 1) || Within(Value{Real: math.Inf(1)}, Value{}, 1) || Within(Value{Real: 1.01}, Value{Real: 1}, 1e-7) {
		t.Fatal("invalid comparison passed")
	}
}

type fakeRunner struct {
	mode  string
	calls int
}

func (r *fakeRunner) Version(context.Context, string) (string, error) { return "47", nil }

func (r *fakeRunner) Run(_ context.Context, dir, path string, args ...string) checks.CommandResult {
	r.calls++
	result := checks.CommandResult{Path: path, Args: args, WorkingDir: dir}
	if len(args) > 0 && args[len(args)-1] == "--version" {
		result.Stdout = "** ngspice-47 : test runner\n"
		if r.mode == "version" {
			result.Stdout = "ngspice-46"
		}
		return result
	}
	raw, err := os.ReadFile(filepath.Join(dir, "circuit.cir"))
	if err != nil {
		result.Err = err
		return result
	}
	id := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(raw), "\n", 2)[0], "KiCadAI frozen conformance "))
	f, ok := Lookup(id)
	if !ok {
		result.Err = errors.New("unknown fixture")
		return result
	}
	value := f.Expected()
	if r.mode == "mismatch" {
		value.Real += 1
	}
	result.Stdout = fmt.Sprintf("observed_real = %.17g\nobserved_imag = %.17g\n", value.Real, value.Imaginary)
	if r.mode == "malformed" {
		result.Stdout = "observed_real = NaN\n"
	}
	if r.mode == "crash" {
		result.ExitCode = 1
		result.Err = errors.New("crashed")
	}
	return result
}

func TestConformanceEvidenceReplayAndNegativeControls(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"pass", "version", "mismatch", "malformed", "crash"} {
		t.Run(mode, func(t *testing.T) {
			runner := &fakeRunner{mode: mode}
			root := filepath.Join(t.TempDir(), "run")
			report, err := Run(context.Background(), root, Options{Ngspice: exe, Runner: runner, CaseIDs: []string{"divider-equal"}})
			if (mode == "pass") != (err == nil) {
				t.Fatalf("report=%+v err=%v", report, err)
			}
			if err := evidencebundle.Verify(root); err != nil {
				t.Fatal(err)
			}
			ids, err := ReplayCases(root)
			if err != nil || len(ids) != 1 || ids[0] != "divider-equal" {
				t.Fatalf("replay=%v %v", ids, err)
			}
			if mode == "version" && runner.calls != 1 {
				t.Fatal("wrong-version tool was executed")
			}
			if mode != "pass" && report.FirstFailingStage == "" {
				t.Fatal("missing first failure")
			}
		})
	}
	root := filepath.Join(t.TempDir(), "offline")
	report, err := Run(context.Background(), root, Options{CaseIDs: []string{"divider-equal"}})
	if err != nil || report.Status != "analytical_checked" || report.ExternalStatus != "not_checked" {
		t.Fatalf("offline report %+v %v", report, err)
	}
	if _, err = Run(context.Background(), root, Options{}); err == nil {
		t.Fatal("overwrote bundle")
	}
	if _, err = Run(context.Background(), filepath.Join(t.TempDir(), "bad"), Options{CaseIDs: []string{"../bad"}}); err == nil {
		t.Fatal("arbitrary fixture accepted")
	}
	if err = os.WriteFile(filepath.Join(root, "report.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReplayCases(root); err == nil {
		t.Fatal("tampered replay accepted")
	}
}
func TestConformanceExternalParserFailsClosed(t *testing.T) {
	for _, text := range []string{"", "observed_real = NaN\nobserved_imag = 0", "observed_real = 1\nobserved_real = 2\nobserved_imag = 0", "observed_real = 1e999\nobserved_imag = 0"} {
		if _, err := parseExternal(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, filepath.Join(t.TempDir(), "cancelled"), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConformanceIntegrityIsIndependentOfReplayCompatibility(t *testing.T) {
	report := Report{Schema: Schema, CorpusSHA256: CorpusSHA256(), RegistrySHA256: "historical-registry", Cases: []Comparison{{FixtureID: "divider-equal"}}}
	raw, err := evidencebundle.JSON(report)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "historical")
	if err = evidencebundle.Publish(root, map[string][]byte{"report.json": raw}); err != nil {
		t.Fatal(err)
	}
	if _, err = Inspect(root); err != nil {
		t.Fatalf("intact historical evidence rejected: %v", err)
	}
	if _, err = ReplayCases(root); err == nil {
		t.Fatal("incompatible model replay accepted")
	}
	runner := &fakeRunner{mode: "pass"}
	report, err = Run(context.Background(), filepath.Join(t.TempDir(), "bare"), Options{Ngspice: "ngspice", Runner: runner, CaseIDs: []string{"divider-equal"}})
	if err == nil || runner.calls != 0 || report.FirstFailingStage != "external_tool" {
		t.Fatal("bare executable discovered from PATH")
	}
}
func TestConformanceNativeNgspice47(t *testing.T) {
	path := os.Getenv("KICADAI_NGSPICE_47")
	if path == "" {
		t.Skip("set KICADAI_NGSPICE_47 for pinned external validation")
	}
	root := filepath.Join(t.TempDir(), "native")
	report, err := Run(context.Background(), root, Options{Ngspice: path})
	if err != nil || report.Status != "conformant" || len(report.Cases) != 12 {
		t.Fatalf("native report=%+v err=%v", report, err)
	}
	if err = evidencebundle.Verify(root); err != nil {
		t.Fatal(err)
	}
}
