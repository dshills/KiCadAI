package environmentdoctor

import (
	"context"
	"kicadai/internal/kicadfiles/checks"
	"os"
	"testing"
)

type versionRunner struct{ version string }

func (r versionRunner) Version(context.Context, string) (string, error) { return r.version, nil }
func (r versionRunner) Run(context.Context, string, string, ...string) checks.CommandResult {
	return checks.CommandResult{}
}
func TestDoctorEmbeddedAndRequiredToolFailures(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	report := Run(context.Background(), Options{KiCadCLI: exe, Runner: versionRunner{"10.0.3"}})
	if report.Status != "offline_ready" || report.APIRequired || report.HardwareValidated {
		t.Fatalf("report %+v", report)
	}
	if len(report.Probes) < 10 {
		t.Fatal("missing environment probes")
	}
	for _, options := range []Options{{KiCadCLI: "/nonexistent/kicad", RequireERC: true}, {KiCadCLI: exe, RequireERC: true, Runner: versionRunner{"9.0.0"}}} {
		if report := Run(context.Background(), options); report.Status != "blocked" {
			t.Fatalf("missing or mismatched required tool passed %+v", report)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if report := Run(ctx, Options{}); report.Status != "blocked" {
		t.Fatal("cancelled probe passed")
	}
}
