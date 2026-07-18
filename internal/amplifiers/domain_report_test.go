package amplifiers_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/amplifiers"
	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

var updateAudioDomainReport = flag.Bool("update-audio-domain-report", false, "update the verified audio amplifier capability report")

const audioMeasurementSignificantDigits = 12

type audioDomainReport struct {
	Schema          string              `json:"schema"`
	GeneratedAt     string              `json:"generated_at"`
	PolicyVersion   string              `json:"policy_version"`
	EvidenceRecords map[string]string   `json:"evidence_records"`
	PositiveCases   []audioPositiveCase `json:"positive_cases"`
	UnsafeCases     []audioUnsafeCase   `json:"unsafe_cases"`
	KiCadFixtures   []audioKiCadFixture `json:"kicad_fixtures"`
}

type audioPositiveCase struct {
	ID           string                          `json:"id"`
	Status       string                          `json:"status"`
	Analyses     []amplifiers.AnalysisKind       `json:"analyses"`
	Layout       amplifiers.LayoutPolicyEvidence `json:"layout"`
	Measurements map[string]map[string]float64   `json:"measurements"`
}

type audioUnsafeCase struct {
	ID             string   `json:"id"`
	ExpectedStatus string   `json:"expected_status"`
	IssueCodes     []string `json:"issue_codes"`
	IssuePaths     []string `json:"issue_paths"`
}

type audioKiCadFixture struct {
	ID             string   `json:"id"`
	Declared       string   `json:"declared_readiness"`
	Acceptance     string   `json:"acceptance"`
	RequireERC     bool     `json:"require_erc"`
	RequireDRC     bool     `json:"require_drc"`
	ExpectedStages []string `json:"expected_stages"`
	RequestSHA256  string   `json:"request_sha256"`
	MetadataSHA256 string   `json:"metadata_sha256"`
}

func TestVerifiedAudioAmplifierCapabilityReport(t *testing.T) {
	report := buildAudioDomainReport(t)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	reportPath := filepath.Join(audioRepoRoot(t), "specs", "verified-audio-amplifier-domain", "CAPABILITY_REPORT.json")
	checksum := sha256Hex(encoded) + "  CAPABILITY_REPORT.json\n"
	checksumPath := filepath.Join(filepath.Dir(reportPath), "CAPABILITY_REPORT.sha256")
	if *updateAudioDomainReport {
		if err := os.WriteFile(reportPath, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(checksumPath, []byte(checksum), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(encoded) {
		t.Fatalf("audio capability report is stale; run go test ./internal/amplifiers -run TestVerifiedAudioAmplifierCapabilityReport -update-audio-domain-report\n%s", encoded)
	}
	wantChecksum, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(wantChecksum) != checksum {
		t.Fatalf("audio capability report checksum is stale: got %q want %q", wantChecksum, checksum)
	}
}

func buildAudioDomainReport(t *testing.T) audioDomainReport {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	npn := audioRecord(t, catalog, "bjt.onsemi.mmbt3904.sot23")
	opamp := audioRecord(t, catalog, "opamp.ti.lmv321.sot23_5")
	capacitor := audioRecord(t, catalog, "capacitor.panasonic.eeufr1c221.radial")
	mosfet := audioRecord(t, catalog, "mosfet.vishay.irfp240.to247")
	registry := blocks.NewBuiltinRegistry()
	classADefinition, _ := registry.GetBlock("class_a_voltage_stage")
	classABDefinition, _ := registry.GetBlock("class_ab_output_stage")
	classA := audioValidationRequest(npn, "class_a_bjt", 12, 6, 0.001, 1, 0.002, 10_000, 0.0055, 5.5, 0.001)
	classAB := audioValidationRequest(npn, "class_ab_bjt_headphone", 9, 4.5, 0.005, 3, 0.12, 32, 0.02, 4.5, 0.03)
	positive := []audioPositiveCase{
		audioPositive(t, "class_a_bjt_line_preamplifier", classA, amplifiers.ValidateLayoutPolicy(amplifiers.LayoutProfileClassALine, classADefinition.PCBRealization)),
		audioPositive(t, "protected_bjt_class_ab_headphone_amplifier", classAB, amplifiers.ValidateLayoutPolicy(amplifiers.LayoutProfileClassABHeadphone, classABDefinition.PCBRealization)),
	}
	unsafe := []audioUnsafeCase{}
	unsafe = append(unsafe,
		audioUnsafeFromIssues("opamp_common_mode_and_output_drive", amplifiers.ValidateOpAmpApplication(opamp, amplifiers.OpAmpApplication{SupplyVoltageV: 9, InputMinimumV: -0.1, InputMaximumV: 8.8, OutputMinimumV: 0, OutputMaximumV: 9, OutputCurrentA: 0.2, RequiredGainBandwidthHz: 2e6, RequiredSlewRateVPerS: 2e6, RequireFabricationProof: true})),
		audioUnsafeFromIssues("capacitor_voltage_esr_ripple_and_polarity", unsafeCapacitorIssues(capacitor)),
	)
	soa := classAB
	soa.SOACurrentA = 0.2
	unsafe = append(unsafe, audioUnsafeFromValidation("bjt_secondary_breakdown_soa", amplifiers.ValidateOperatingEnvelope(soa)))
	mosfetSOA := classAB
	mosfetSOA.Device = mosfet
	unsafe = append(unsafe, audioUnsafeFromValidation("mosfet_linear_mode_soa", amplifiers.ValidateOperatingEnvelope(mosfetSOA)))
	thermal := classA
	thermal.DeviceDissipationW = 1
	unsafe = append(unsafe, audioUnsafeFromValidation("class_a_excessive_dissipation", amplifiers.ValidateOperatingEnvelope(thermal)))
	runaway := classAB
	runaway.ToleranceCorners[1].QuiescentCurrentA = 0
	unsafe = append(unsafe, audioUnsafeFromValidation("class_ab_thermal_runaway_bias_corner", amplifiers.ValidateOperatingEnvelope(runaway)))
	unstable := classAB
	unstable.PhaseMarginDeg = 20
	unsafe = append(unsafe, audioUnsafeFromValidation("inadequate_phase_margin", amplifiers.ValidateOperatingEnvelope(unstable)))
	unsafeLayout := amplifiers.ValidateLayoutPolicy(amplifiers.LayoutProfileClassABHeadphone, &blocks.PCBRealization{Constraints: []blocks.PCBConstraint{{ID: "current", Kind: "route_width", Category: blocks.PCBConstraintCurrentPath, NetTemplate: "out"}}})
	unsafe = append(unsafe, audioUnsafeCase{ID: "high_current_return_and_thermal_layout", ExpectedStatus: "blocked", IssueCodes: []string{"AMPLIFIER_LAYOUT_POLICY"}, IssuePaths: append(append([]string{}, categoryStrings(unsafeLayout.MissingCategories)...), categoryStrings(unsafeLayout.InvalidCategories)...)})
	sort.Slice(unsafe, func(i, j int) bool { return unsafe[i].ID < unsafe[j].ID })
	return audioDomainReport{
		Schema: "kicadai-verified-audio-amplifier-capability-v1", GeneratedAt: "2026-07-17", PolicyVersion: "verified-audio-amplifier-policy-v1",
		EvidenceRecords: map[string]string{npn.ID: recordSHA256(npn), opamp.ID: recordSHA256(opamp), capacitor.ID: recordSHA256(capacitor), mosfet.ID: recordSHA256(mosfet)},
		PositiveCases:   positive, UnsafeCases: unsafe,
		KiCadFixtures: []audioKiCadFixture{audioFixture(t, "class_a_bjt_line_preamplifier"), audioFixture(t, "class_ab_headphone_protected")},
	}
}

func audioValidationRequest(device *components.ComponentRecord, topology string, supply, bias, idle, peakV, peakI, load, dissipation, soaV, soaI float64) amplifiers.ValidationRequest {
	return amplifiers.ValidationRequest{
		Topology: topology, SupplyVoltageV: supply, OutputBiasV: bias, QuiescentCurrentA: idle, OutputPeakVoltageV: peakV, OutputPeakCurrentA: peakI,
		LoadImpedanceOhm: load, MaximumSignalFrequencyHz: 20_000, ClosedLoopGain: 2, NoiseGain: 2, GainBandwidthHz: 1_000_000,
		SlewRateVPerS: 1_000_000, PhaseMarginDeg: 60, DeviceDissipationW: dissipation, AmbientTemperatureC: 45,
		SOAVoltageV: soaV, SOACurrentA: soaI, SOATemperatureC: 45, Device: device,
		ToleranceCorners: []amplifiers.OperatingCorner{
			{Name: "cold_min", OutputBiasV: bias * 0.95, QuiescentCurrentA: idle * 0.8, DeviceDissipationW: dissipation * 0.85, PhaseMarginDeg: 58},
			{Name: "hot_max", OutputBiasV: bias * 1.05, QuiescentCurrentA: idle * 1.2, DeviceDissipationW: dissipation * 1.2, PhaseMarginDeg: 52},
		},
	}
}

func audioPositive(t *testing.T, id string, request amplifiers.ValidationRequest, layout amplifiers.LayoutPolicyEvidence) audioPositiveCase {
	t.Helper()
	result := amplifiers.ValidateOperatingEnvelope(request)
	if !result.Pass || !layout.OK() {
		t.Fatalf("positive audio case %s failed: validation=%#v layout=%#v", id, result, layout)
	}
	entry := audioPositiveCase{ID: id, Status: "pass", Layout: layout, Measurements: map[string]map[string]float64{}}
	for _, analysis := range result.Analyses {
		entry.Analyses = append(entry.Analyses, analysis.Kind)
		entry.Measurements[string(analysis.Kind)] = canonicalAudioMeasurements(t, analysis.Measurements)
	}
	return entry
}

func canonicalAudioMeasurements(t testing.TB, measurements map[string]float64) map[string]float64 {
	t.Helper()
	canonical := make(map[string]float64, len(measurements))
	for name, value := range measurements {
		formatted := strconv.FormatFloat(value, 'g', audioMeasurementSignificantDigits, 64)
		parsed, err := strconv.ParseFloat(formatted, 64)
		if err != nil {
			t.Fatalf("canonical measurement %s=%q cannot be parsed: %v", name, formatted, err)
		}
		canonical[name] = parsed
	}
	return canonical
}

func unsafeCapacitorIssues(record *components.ComponentRecord) []reports.Issue {
	copyRecord := *record
	copyEvidence := *record.Capacitor
	copyEvidence.FabricationProof = false
	copyRecord.Capacitor = &copyEvidence
	return amplifiers.ValidateCapacitorApplication(&copyRecord, amplifiers.CapacitorApplication{AppliedVoltageV: 25, RippleCurrentA: 2, MaximumESROhm: 0.01, RequiredCapacitanceF: 300e-6, ExpectedPolarity: "non_polarized", RequireFabricationProof: true})
}

func audioUnsafeFromValidation(id string, result amplifiers.ValidationResult) audioUnsafeCase {
	return audioUnsafeFromIssues(id, result.Issues)
}

func audioUnsafeFromIssues(id string, issues []reports.Issue) audioUnsafeCase {
	entry := audioUnsafeCase{ID: id, ExpectedStatus: "blocked"}
	for _, issue := range issues {
		entry.IssueCodes = append(entry.IssueCodes, string(issue.Code))
		entry.IssuePaths = append(entry.IssuePaths, issue.Path)
	}
	sort.Strings(entry.IssueCodes)
	entry.IssueCodes = slices.Compact(entry.IssueCodes)
	sort.Strings(entry.IssuePaths)
	entry.IssuePaths = slices.Compact(entry.IssuePaths)
	return entry
}

func audioRecord(t *testing.T, catalog *components.Catalog, id string) *components.ComponentRecord {
	t.Helper()
	for index := range catalog.Records {
		if catalog.Records[index].ID == id {
			return &catalog.Records[index]
		}
	}
	t.Fatalf("missing catalog record %s", id)
	return nil
}

func audioFixture(t *testing.T, id string) audioKiCadFixture {
	t.Helper()
	root := audioRepoRoot(t)
	metadataPath := filepath.Join(root, "examples", "design", "kicad-backed", id+".metadata.json")
	requestPath := filepath.Join(root, "examples", "design", "kicad-backed", id+".json")
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	requestBytes, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Readiness      string   `json:"readiness"`
		Acceptance     string   `json:"acceptance"`
		RequireERC     bool     `json:"require_erc"`
		RequireDRC     bool     `json:"require_drc"`
		ExpectedStages []string `json:"expected_stages"`
	}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Readiness != "pass" || !metadata.RequireERC || !metadata.RequireDRC {
		t.Fatalf("fixture %s does not declare complete KiCad-backed gates", id)
	}
	return audioKiCadFixture{ID: id, Declared: metadata.Readiness, Acceptance: metadata.Acceptance, RequireERC: metadata.RequireERC, RequireDRC: metadata.RequireDRC, ExpectedStages: metadata.ExpectedStages, RequestSHA256: sha256Hex(requestBytes), MetadataSHA256: sha256Hex(metadataBytes)}
}

func categoryStrings(values []blocks.PCBConstraintCategory) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = "amplifier.layout." + string(value)
	}
	sort.Strings(out)
	return out
}

func recordSHA256(record *components.ComponentRecord) string {
	data, _ := json.Marshal(record)
	return sha256Hex(data)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func audioRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve audio report source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestVerifiedAudioUnsafeCorpusIsComplete(t *testing.T) {
	report := buildAudioDomainReport(t)
	want := []string{"bjt_secondary_breakdown_soa", "capacitor_voltage_esr_ripple_and_polarity", "class_a_excessive_dissipation", "class_ab_thermal_runaway_bias_corner", "high_current_return_and_thermal_layout", "inadequate_phase_margin", "mosfet_linear_mode_soa", "opamp_common_mode_and_output_drive"}
	for _, id := range want {
		if !slices.ContainsFunc(report.UnsafeCases, func(entry audioUnsafeCase) bool {
			return entry.ID == id && len(entry.IssueCodes) != 0 && len(entry.IssuePaths) != 0
		}) {
			t.Fatalf("unsafe corpus missing actionable blocked case %s: %#v", id, report.UnsafeCases)
		}
	}
	if strings.TrimSpace(report.PolicyVersion) == "" {
		t.Fatal("audio policy version missing")
	}
}
