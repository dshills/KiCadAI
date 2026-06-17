package verification

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestLoadAndValidateManifest(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	manifest, issues := LoadManifest(filepath.Join("..", "testdata", "verification", "led_indicator_default", "manifest.json"))
	if len(issues) != 0 {
		t.Fatalf("load issues = %#v", issues)
	}
	issues = ValidateManifest(manifest, registry)
	if len(issues) != 0 {
		t.Fatalf("validate issues = %#v", issues)
	}
	if manifest.ID != "led_indicator_default" || manifest.Expected.EvidenceLevel != EvidenceSchematicVerified {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestValidateManifestReportsUnknownBlock(t *testing.T) {
	manifest := validManifest()
	manifest.BlockID = "missing_block"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "unknown block ID missing_block") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicateNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets = append(manifest.Expected.Nets, manifest.Expected.Nets[0])
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate expected net LED_A") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicateComponentRef(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components[0].Ref = "R1"
	manifest.Expected.Components[1].Ref = "R1"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate expected component ref R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsUnknownNetPinRole(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Pins = append(manifest.Expected.Nets[0].Pins, ExpectedPin{Role: "missing", Pin: "1"})
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected net pin references unknown component role missing") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsUnknownPlacementRole(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "missing", XMM: 1, YMM: 2}}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected placement references unknown component role missing") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidPortDirection(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Ports[0].Direction = "sideways"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "unsupported expected port direction sideways") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsPortUnknownNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Ports[0].Net = "MISSING"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected port references unknown net MISSING") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicatePlacement(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Role: "resistor", XMM: 1, YMM: 2},
		{Role: "resistor", XMM: 3, YMM: 4},
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate expected placement for role:resistor") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicatePlacementByRefAndRole(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components[0].Ref = "R1"
	manifest.Expected.PCB.Placements = []ExpectedPlacement{
		{Ref: "R1", XMM: 1, YMM: 2},
		{Role: "resistor", XMM: 3, YMM: 4},
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate expected placement for ref:R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsUnknownPadNetRefs(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Components[0].Ref = "R1"
	manifest.Expected.PCB.PadNets = []ExpectedPadNet{{Ref: "missing", Pad: "1", Net: "MISSING"}}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	for _, want := range []string{
		"expected pad net references unknown component ref missing",
		"expected pad net references unknown net MISSING",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
	}
}

func TestValidateManifestReportsUnmatchedExpectedReference(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.References = append(manifest.Expected.References, "U1")
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected reference has no matching component expectation U1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestAllowsReferencePrefix(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.References = []string{"R1", "D1"}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestDoesNotOvermatchReferencePrefix(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.References = []string{"REG1"}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected reference has no matching component expectation REG1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsSinglePinLocalNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Pins = manifest.Expected.Nets[0].Pins[:1]
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "expected net requires at least two pins unless exported") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestAllowsSinglePinPowerNet(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Nets[0].Visibility = "power"
	manifest.Expected.Nets[0].Pins = manifest.Expected.Nets[0].Pins[:1]
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsWriterRoundTripWithoutWriter(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.Writer = ExpectedWriter{RequireRoundTrip: true}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "writer round-trip evidence requires writer validation") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsERCDRCRequiresRequiredFlag(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{RequireDRC: true}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "ERC or DRC requirements require erc_drc.required") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidCaseID(t *testing.T) {
	manifest := validManifest()
	manifest.ID = "Bad ID"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "verification case ID must start with a lowercase letter") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidAcceptance(t *testing.T) {
	manifest := validManifest()
	manifest.Acceptance = "production"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "unsupported verification acceptance production") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsNilRegistry(t *testing.T) {
	issues := ValidateManifest(validManifest(), nil)
	if !hasIssue(issues, "block registry is required") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidParams(t *testing.T) {
	manifest := validManifest()
	manifest.Request.Params["active_high"] = "yes"
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "parameter active_high must be a bool") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestManifestJSONShapeIsDeterministic(t *testing.T) {
	data, err := json.MarshalIndent(validManifest(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"id": "led_indicator_default"`, `"block_id": "led_indicator"`, `"evidence_level": "schematic_verified"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("json missing %q:\n%s", want, text)
		}
	}
}

func validManifest() Manifest {
	return Manifest{
		ID:         "led_indicator_default",
		BlockID:    "led_indicator",
		Acceptance: "connectivity",
		Request: RequestSpec{
			InstanceID: "status",
			Params: map[string]any{
				"supply_voltage":      "3.3V",
				"led_forward_voltage": "2.0V",
				"led_current":         "5mA",
				"active_high":         true,
			},
		},
		Expected: Expected{
			EvidenceLevel: EvidenceSchematicVerified,
			References:    []string{"R1", "D1"},
			Components: []ExpectedComponent{{
				Role:        "resistor",
				RefPrefix:   "R",
				SymbolID:    "Device:R",
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
			}, {
				Role:        "led",
				RefPrefix:   "D",
				SymbolID:    "Device:LED",
				FootprintID: "LED_SMD:LED_0805_2012Metric",
			}},
			Ports: []ExpectedPort{
				{Name: "IN", Direction: "input"},
				{Name: "GND", Direction: "power"},
			},
			Nets: []ExpectedNet{{
				Name:       "LED_A",
				Visibility: "local",
				Pins: []ExpectedPin{
					{Role: "resistor", Pin: "2"},
					{Role: "led", Pin: "1"},
				},
			}},
		},
	}
}

func hasIssue(issues []reports.Issue, fragment string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	return false
}
