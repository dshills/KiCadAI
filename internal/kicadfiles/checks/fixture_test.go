package checks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const envKiCadFixtureTarget = "KICADAI_KICAD_FIXTURE_TARGET"

type reportFixtureSummary struct {
	Path         string         `json:"path"`
	Kind         CheckKind      `json:"kind"`
	Units        string         `json:"units"`
	FindingCount int            `json:"finding_count"`
	ParserCount  int            `json:"parser_count"`
	ByCategory   map[string]int `json:"by_category,omitempty"`
	ByCode       map[string]int `json:"by_code,omitempty"`
}

func TestKiCadReportFixtureSummariesStable(t *testing.T) {
	tests := []struct {
		path       string
		kind       CheckKind
		wantCount  int
		wantIssues int
		wantUnits  string
		wantCode   string
	}{
		{path: "erc_clean_kicad10.json", kind: CheckKindERC, wantCount: 0, wantUnits: "mm"},
		{path: "erc_violation_kicad10.json", kind: CheckKindERC, wantCount: 1, wantUnits: "mm", wantCode: "power_pin_not_driven"},
		{path: "drc_clean_kicad10.json", kind: CheckKindDRC, wantCount: 0, wantUnits: "mm"},
		{path: "drc_violation_kicad10.json", kind: CheckKindDRC, wantCount: 1, wantUnits: "mm", wantCode: "clearance"},
		{path: "parser_invalid_kicad10.json", kind: CheckKindDRC, wantIssues: 1},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			summary := summarizeReportFixture(t, tt.kind, tt.path)
			if summary.Path != "$FIXTURE/"+tt.path {
				t.Fatalf("path = %q", summary.Path)
			}
			if tt.wantUnits != "" && summary.Units != tt.wantUnits {
				t.Fatalf("units = %q, want %s", summary.Units, tt.wantUnits)
			}
			if summary.FindingCount != tt.wantCount || summary.ParserCount != tt.wantIssues {
				t.Fatalf("summary = %#v", summary)
			}
			if tt.wantCode != "" && summary.ByCode[tt.wantCode] == 0 {
				t.Fatalf("summary missing code %q: %#v", tt.wantCode, summary.ByCode)
			}
		})
	}
}

func TestMissingReportFixtureReturnsReadError(t *testing.T) {
	_, _, _, err := ParseReportFile(CheckKindDRC, filepath.Join("testdata", "missing_report_kicad10.json"))
	if err == nil {
		t.Fatal("expected missing report read error")
	}
}

func TestOptionalKiCadCLIReportFixtureGeneration(t *testing.T) {
	if os.Getenv(EnvRunKiCadCLI) != "1" {
		t.Skipf("set %s=1 to run local KiCad CLI fixture generation", EnvRunKiCadCLI)
	}
	target := strings.TrimSpace(os.Getenv(envKiCadFixtureTarget))
	if target == "" {
		t.Skipf("set %s to a generated project, schematic, or board", envKiCadFixtureTarget)
	}
	cli, err := DiscoverCLI("")
	if err != nil {
		t.Skipf("KiCad CLI unavailable: %v", err)
	}
	opts := Options{KeepArtifacts: true, ArtifactDir: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var result CheckResult
	switch {
	case strings.EqualFold(filepath.Ext(target), ".kicad_sch"):
		result, err = RunERC(ctx, cli, target, opts)
	case strings.EqualFold(filepath.Ext(target), ".kicad_pcb"):
		result, err = RunDRC(ctx, cli, target, opts)
	default:
		info, statErr := os.Stat(target)
		if statErr != nil {
			t.Fatalf("stat fixture target: %v", statErr)
		}
		if !info.IsDir() {
			t.Fatalf("fixture target must be a project directory, .kicad_sch, or .kicad_pcb: %s", target)
		}
		result, err = RunDRC(ctx, cli, target, opts)
	}
	if err != nil && result.ReportPath == "" {
		t.Fatalf("KiCad CLI failed without a report: %v", err)
	}
	if result.ReportPath == "" {
		t.Fatalf("missing KiCad report path: %#v", result)
	}
}

func summarizeReportFixture(t *testing.T, kind CheckKind, name string) reportFixtureSummary {
	t.Helper()
	path := filepath.Join("testdata", name)
	findings, parserIssues, units, err := ParseReportFile(kind, path)
	if err != nil && len(parserIssues) == 0 {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	summary := reportFixtureSummary{
		Path:         "$FIXTURE/" + filepath.Base(path),
		Kind:         kind,
		Units:        units,
		FindingCount: len(findings),
		ParserCount:  len(parserIssues),
		ByCategory:   map[string]int{},
		ByCode:       map[string]int{},
	}
	for _, finding := range findings {
		summary.ByCategory[string(finding.RepairCategory)]++
		summary.ByCode[finding.Code]++
	}
	if len(summary.ByCategory) == 0 {
		summary.ByCategory = nil
	}
	if len(summary.ByCode) == 0 {
		summary.ByCode = nil
	}
	return summary
}
