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
	manifest.Expected.PCB.Placements = []ExpectedPlacement{{Role: "missing", XMM: floatRef(1), YMM: floatRef(2)}}
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
		{Role: "resistor", XMM: floatRef(1), YMM: floatRef(2)},
		{Role: "resistor", XMM: floatRef(3), YMM: floatRef(4)},
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
		{Ref: "R1", XMM: floatRef(1), YMM: floatRef(2)},
		{Role: "resistor", XMM: floatRef(3), YMM: floatRef(4)},
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate expected placement for ref:R1") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicateRequiredLocalRoute(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.PCB.RequiredLocalRoutes = []string{"led_local", "led_local", "", " route ", "bad route"}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	for _, want := range []string{
		"duplicate required local route led_local",
		"required local route ID is required",
		"required local route ID must not contain leading or trailing whitespace",
		"required local route ID must not contain whitespace",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
	}
}

func TestValidateManifestReportsInvalidTimingFixtureExpectations(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.PCB.TimingFixtures = []ExpectedTimingFixture{{
		ID:                "",
		RequiredFindings:  []string{"timing.a", "timing.a", ""},
		ForbiddenFindings: []string{" timing.c "},
	}, {
		ID:                "clock",
		RequiredFindings:  []string{"timing.shared"},
		ForbiddenFindings: []string{"timing.shared"},
	}, {
		ID: "bad clock",
	}, {
		ID:                "clock",
		ForbiddenFindings: []string{"timing.b", "timing.b"},
	}}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	for _, want := range []string{
		"expected timing fixture ID is required",
		"expected timing fixture ID must not contain whitespace",
		"duplicate expected timing fixture clock",
		"duplicate required timing finding timing.a",
		"duplicate forbidden timing finding timing.b",
		"required timing finding ID is required",
		"forbidden timing finding ID must not contain leading or trailing whitespace",
		"timing finding cannot be both required and forbidden timing.shared",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
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

func TestValidateManifestReportsInvalidERCDRCRunnerPolicy(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:   true,
		RequireDRC: true,
		Runner:     ERCDRCRunnerPolicy("best_effort"),
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "unsupported ERC/DRC runner policy best_effort") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsRequiredERCDRCOptionalRunnerConflict(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:   true,
		RequireDRC: true,
		Runner:     ERCDRCRunnerOptional,
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "required ERC/DRC evidence cannot use optional runner policy") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsRequiredERCDRCWithoutSelectedChecks(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{Runner: ERCDRCRunnerRequiredReal}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "required ERC/DRC evidence must select require_erc or require_drc") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicateERCDRCPolicyValues(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:       true,
		RequireDRC:     true,
		AllowedCodes:   []string{"DRC_A", "DRC_A"},
		ExpectedIssues: []string{"ERC_A", "ERC_A"},
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate allowed ERC/DRC code") || !hasIssue(issues, "duplicate expected ERC/DRC issue") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestAcceptsFakeERCDRCRunnerPolicy(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:        true,
		RequireERC:      true,
		Runner:          ERCDRCRunnerFake,
		MinKiCadVersion: "9.0.0",
		MaxKiCadVersion: "10.99.0",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestAcceptsKiCadCorpusMetadata(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           KiCadCorpusTierSmoke,
		Readiness:      KiCadCorpusReadinessCandidate,
		ExpectedStatus: KiCadCorpusStatusPass,
		RequiresDRC:    true,
		AllowedCodes:   []string{"known_warning"},
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidKiCadCorpusMetadata(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           KiCadCorpusTier("daily"),
		Readiness:      KiCadCorpusReadiness("almost"),
		ExpectedStatus: KiCadCorpusExpectedStatus("clean"),
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	for _, want := range []string{
		"unsupported KiCad corpus tier daily",
		"unsupported KiCad corpus readiness almost",
		"unsupported KiCad corpus expected status clean",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("issues missing %q: %#v", want, issues)
		}
	}
}

func TestValidateManifestReportsKiCadCorpusMetadataWithoutInclude(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Tier:           KiCadCorpusTierSmoke,
		Readiness:      KiCadCorpusReadinessCandidate,
		ExpectedStatus: KiCadCorpusStatusSkip,
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "KiCad corpus metadata requires include=true") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsKiCadCorpusCandidateWithoutDRC(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.EvidenceLevel = EvidencePCBVerified
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           KiCadCorpusTierBlock,
		Readiness:      KiCadCorpusReadinessCandidate,
		ExpectedStatus: KiCadCorpusStatusPass,
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "KiCad corpus PCB pass candidates must require DRC evidence") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsKiCadCorpusBlockedWithoutNotes(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           KiCadCorpusTierBlock,
		Readiness:      KiCadCorpusReadinessBlocked,
		ExpectedStatus: KiCadCorpusStatusBlocked,
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "KiCad corpus expected-fail and blocked cases require notes") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDuplicateKiCadCorpusPolicyValues(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.KiCadCorpus = ExpectedKiCadCorpus{
		Include:        true,
		Tier:           KiCadCorpusTierSmoke,
		Readiness:      KiCadCorpusReadinessExpectedFail,
		ExpectedStatus: KiCadCorpusStatusExpectedFail,
		AllowedCodes:   []string{"A", "A"},
		ExpectedIssues: []string{"B", "B"},
		Notes:          "tracks known local routing gap",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "duplicate KiCad corpus allowed code") || !hasIssue(issues, "duplicate KiCad corpus expected issue") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidERCDRCVersionRange(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:        true,
		RequireERC:      true,
		MinKiCadVersion: "10.0.0",
		MaxKiCadVersion: "9.99.0",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "maximum KiCad version must be greater than or equal to minimum KiCad version") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsPrereleaseERCDRCVersionRange(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:        true,
		RequireERC:      true,
		MinKiCadVersion: "10.0.0",
		MaxKiCadVersion: "10.0.0-rc1",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "maximum KiCad version must be greater than or equal to minimum KiCad version") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsInvalidERCDRCVersionFormat(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:        true,
		RequireDRC:      true,
		MinKiCadVersion: "v10",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "minimum KiCad version must use numeric dotted form") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateManifestReportsDelimiterOnlyERCDRCVersionFormat(t *testing.T) {
	manifest := validManifest()
	manifest.Expected.ERCDRC = ExpectedERCDRC{
		Required:        true,
		RequireDRC:      true,
		MaxKiCadVersion: "-",
	}
	issues := ValidateManifest(manifest, blocks.NewBuiltinRegistry())
	if !hasIssue(issues, "maximum KiCad version must use numeric dotted form") {
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
					{Role: "led", Pin: "2"},
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

func floatRef(value float64) *float64 {
	return &value
}
