package components

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

var updateGolden = flag.Bool("update", false, "update component golden files")

func TestGoldenCatalogValidationFixtures(t *testing.T) {
	cases := []struct {
		name     string
		wantCode reports.Code
	}{
		{name: "valid"},
		{name: "duplicate_ids", wantCode: CodeDuplicateComponentID},
		{name: "missing_symbol", wantCode: CodeMissingSymbolBinding},
		{name: "missing_footprint", wantCode: CodeMissingFootprint},
		{name: "ambiguous_selection"},
		{name: "unsafe_placeholder"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", tc.name)})
			if err != nil {
				t.Fatalf("load fixture: %v", err)
			}
			if tc.wantCode == "" {
				if reports.HasBlockingIssue(catalog.Diagnostics) {
					t.Fatalf("unexpected diagnostics: %+v", catalog.Diagnostics)
				}
				return
			}
			assertIssueCode(t, catalog.Diagnostics, tc.wantCode)
		})
	}
}

func TestGoldenComponentSelectionOutputs(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "valid")})
	if err != nil {
		t.Fatalf("load valid fixture: %v", err)
	}
	candidates, findResult := Find(context.Background(), catalog, Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"})
	if !findResult.OK {
		t.Fatalf("find failed: %+v", findResult.Issues)
	}
	assertGoldenJSON(t, "find_resistor.json", candidates)

	selection, selectResult := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805", ValueKind: "resistance", Value: "10k"},
		Acceptance: AcceptanceConnectivity,
	})
	if !selectResult.OK {
		t.Fatalf("select failed: %+v", selectResult.Issues)
	}
	assertGoldenJSON(t, "select_resistor.json", selection.Candidate)

	validateResult := ValidateCatalog(catalog)
	assertGoldenJSON(t, "validate_summary.json", validateResult.Data)
}

func TestGoldenCheckedInCoverageOutput(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	report, result := ComponentCoverage(catalog, CoverageOptions{})
	if !result.OK {
		t.Fatalf("coverage failed: %+v", result.Issues)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("coverage has roadmap issues: %+v", report.Issues)
	}
	assertGoldenJSON(t, "coverage_checked_in.json", report)
}

func TestGoldenVerifiedActiveSelections(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	cases := []struct {
		name    string
		request SelectionRequest
	}{
		{
			name: "select_regulator.json",
			request: SelectionRequest{
				Query:             Query{Family: "regulator", Package: "sot223", ValueKind: "output_voltage", Value: "3.3"},
				Acceptance:        AcceptanceConnectivity,
				RequiredRatings:   []RequiredRating{{Kind: "input_voltage", Value: "5", Unit: "V"}},
				RequireConcrete:   true,
				RequireCompanions: true,
			},
		},
		{
			name: "select_opamp.json",
			request: SelectionRequest{
				Query:             Query{Text: "ti", Family: "opamp", Package: "sot23_5"},
				Acceptance:        AcceptanceConnectivity,
				RequiredRatings:   []RequiredRating{{Kind: "supply_voltage", Value: "3.3", Unit: "V"}},
				RequiredFunctions: []string{"IN_PLUS", "IN_MINUS", "OUT"},
				RequireConcrete:   true,
				RequireCompanions: true,
			},
		},
		{
			name: "select_mcu.json",
			request: SelectionRequest{
				Query:             Query{Text: "microchip", Family: "mcu", Package: "tqfp32"},
				Acceptance:        AcceptanceConnectivity,
				RequiredFunctions: []string{"VCC", "GND", "RESET"},
				RequireConcrete:   true,
				RequireCompanions: true,
			},
		},
		{
			name: "select_i2c_sensor.json",
			request: SelectionRequest{
				Query:             Query{Text: "sensor.bosch.bme280.lga8", Family: "sensor", Package: "lga8"},
				Acceptance:        AcceptanceConnectivity,
				RequiredFunctions: []string{"VDD", "GND", "SDA", "SCL"},
				RequireConcrete:   true,
				RequireCompanions: true,
			},
		},
		{
			name: "select_usb_c_power.json",
			request: SelectionRequest{
				Query:             Query{Family: "usb_c", Package: "6p"},
				Acceptance:        AcceptanceConnectivity,
				RequiredFunctions: []string{"VBUS", "GND", "CC1", "CC2"},
				RequireConcrete:   true,
				RequireCompanions: true,
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			selection, result := Select(context.Background(), catalog, tc.request)
			if !result.OK {
				t.Fatalf("selection failed: %+v", result.Issues)
			}
			if selection.Candidate.ComponentID == "" {
				t.Fatalf("no candidate selected")
			}
			assertGoldenJSON(t, tc.name, selection.Candidate)
		})
	}
}

func TestGoldenUnsafePlaceholderSelection(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "unsafe_placeholder")})
	if err != nil {
		t.Fatalf("load unsafe fixture: %v", err)
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "opamp"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected unsafe placeholder selection to block")
	}
	assertGoldenJSON(t, "select_placeholder_blocked.json", result.Issues)
}

func TestGoldenAmbiguousSelection(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: filepath.Join("testdata", "catalog", "ambiguous_selection")})
	if err != nil {
		t.Fatalf("load ambiguous fixture: %v", err)
	}
	_, result := Select(context.Background(), catalog, SelectionRequest{
		Query:      Query{Family: "resistor", Package: "0805"},
		Acceptance: AcceptanceConnectivity,
	})
	if result.OK {
		t.Fatal("expected ambiguous selection to block")
	}
	assertGoldenJSON(t, "select_ambiguous_blocked.json", result.Issues)
}

func assertGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden value: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden %s mismatch\nwant:\n%s\ngot:\n%s", name, string(want), string(got))
	}
}
