package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseERCJSON(t *testing.T) {
	findings, issues, units := ParseReport(CheckKindERC, []byte(sampleERCReport))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if units != "mm" {
		t.Fatalf("units = %q, want mm", units)
	}
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}
	first := findings[0]
	if first.Code != "power_pin_not_driven" || first.Sheet != "/" {
		t.Fatalf("unexpected first finding: %#v", first)
	}
	if !containsString(first.References, "#PWR01") || !containsString(first.Pins, "1") {
		t.Fatalf("expected reference and pin extraction: %#v", first)
	}
	if first.Location == nil || first.Location.Units != "mm" {
		t.Fatalf("expected location with units: %#v", first.Location)
	}
}

func TestParseDRCJSON(t *testing.T) {
	findings, issues, units := ParseReport(CheckKindDRC, []byte(sampleDRCReport))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if units != "mm" {
		t.Fatalf("units = %q, want mm", units)
	}
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}
	if findings[0].RepairCategory != RepairClearance {
		t.Fatalf("first category = %s, want clearance", findings[0].RepairCategory)
	}
	if findings[0].Layer != "F.Cu" || findings[0].Net != "GND" {
		t.Fatalf("unexpected DRC finding: %#v", findings[0])
	}
}

func TestParseMalformedReport(t *testing.T) {
	findings, issues, _ := ParseReport(CheckKindERC, []byte(`{"sheets":`))
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unexpected end") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestParseCheckedInReportSamples(t *testing.T) {
	tests := []struct {
		path string
		kind CheckKind
		want int
	}{
		{path: "erc_fail_kicad10.json", kind: CheckKindERC, want: 4},
		{path: "drc_fail_kicad10.json", kind: CheckKindDRC, want: 1},
		{path: "pass_kicad10.json", kind: CheckKindDRC, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.path))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}
			findings, issues, _ := ParseReport(tt.kind, data)
			if len(issues) != 0 {
				t.Fatalf("issues = %#v", issues)
			}
			if len(findings) != tt.want {
				t.Fatalf("findings = %d, want %d", len(findings), tt.want)
			}
		})
	}
}

func TestFilterAllowedFindings(t *testing.T) {
	findings, _, _ := ParseReport(CheckKindERC, []byte(sampleERCReport))
	remaining, allowed := FilterAllowedFindings(findings, []AllowlistEntry{{
		Kind:      CheckKindERC,
		Code:      "power_pin_not_driven",
		Reference: "#PWR01",
		Message:   "Input Power",
		Reason:    "known fixture issue",
	}})
	if len(allowed) != 1 {
		t.Fatalf("allowed = %d, want 1", len(allowed))
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
	if remaining[0].Code != "wire_dangling" {
		t.Fatalf("remaining = %#v", remaining)
	}
}

func TestReferencesFromTextIgnoresGenericWords(t *testing.T) {
	refs := referencesFromText("Symbol U1 Pin 1 and Track on F.Cu near #PWR01")
	if len(refs) != 2 || refs[0] != "U1" || refs[1] != "#PWR01" {
		t.Fatalf("refs = %#v, want U1 and #PWR01", refs)
	}
}

func TestEmptyAllowlistEntryDoesNotMatchEverything(t *testing.T) {
	findings, _, _ := ParseReport(CheckKindERC, []byte(sampleERCReport))
	remaining, allowed := FilterAllowedFindings(findings, []AllowlistEntry{{Reason: "too broad"}})
	if len(allowed) != 0 {
		t.Fatalf("allowed = %d, want 0", len(allowed))
	}
	if len(remaining) != len(findings) {
		t.Fatalf("remaining = %d, want %d", len(remaining), len(findings))
	}
}

const sampleERCReport = `{
  "$schema": "https://schemas.kicad.org/erc.v1.json",
  "coordinate_units": "mm",
  "kicad_version": "10.0.3",
  "sheets": [
    {
      "path": "/",
      "violations": [
        {
          "description": "Input Power pin not driven by any Output Power pins",
          "items": [
            {
              "description": "Symbol #PWR01 Pin 1 [Power input, Line]",
              "pos": {"x": 0.254, "y": 0.254},
              "uuid": "e23cbe03-44d3-4c01-a28d-634607b10096"
            }
          ],
          "severity": "error",
          "type": "power_pin_not_driven"
        },
        {
          "description": "Wires not connected to anything",
          "items": [
            {
              "description": "Horizontal Wire, length 0.0762 mm",
              "pos": {"x": 0.4953, "y": 0.254},
              "uuid": "11111111-1111-4111-8111-111111111202"
            }
          ],
          "severity": "error",
          "type": "wire_dangling"
        }
      ]
    }
  ]
}`

const sampleDRCReport = `{
  "$schema": "https://schemas.kicad.org/drc.v1.json",
  "coordinate_units": "mm",
  "violations": [
    {
      "description": "Clearance violation between tracks",
      "severity": "error",
      "type": "clearance",
      "layer": "F.Cu",
      "net": "GND",
      "items": [
        {"description": "Track on F.Cu", "pos": {"x": 10.125001, "y": 12.5}, "uuid": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "net": "GND", "layer": "F.Cu"}
      ]
    },
    {
      "description": "Board has malformed outline",
      "severity": "warning",
      "type": "board_outline",
      "items": []
    }
  ]
}`
