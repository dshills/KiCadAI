package amplifiers

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var spiceNumericTokenPattern = regexp.MustCompile(`^([+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)\s*([A-Za-z]*)`)

type SimulationStatus string

const (
	SimulationStatusNotSupported SimulationStatus = "not_supported"
	SimulationStatusNotRun       SimulationStatus = "not_run"
	SimulationStatusBlocked      SimulationStatus = "blocked"
	SimulationStatusCandidate    SimulationStatus = "candidate"
	SimulationStatusPass         SimulationStatus = "pass"

	defaultHeadphoneLoadOhms      = 32
	defaultHeadphoneGain          = 2
	defaultHighPassCutoffHz       = 22.6
	defaultOutputCouplingFarads   = 220e-6
	defaultLoadDCMaxAbsV          = 0.05
	maxOutputCouplingFarads       = 0.1
	defaultFeedbackReferenceOhms  = 10000
	defaultAttenuationTopOhms     = 1e6
	minAttenuationTopOhms         = 100e3
	minFeedbackOhms               = 1e-6
	maxAttenuationResistorOhms    = 10e6
	unityGainEpsilon              = 1e-4
	defaultHeadphoneSupplyVoltage = "9"
)

type SimulationExpectation struct {
	OperatingPoint       OperatingPointExpectation `json:"operating_point"`
	ACGain               GainExpectation           `json:"ac_gain"`
	HighPassCutoffHz     RangeExpectation          `json:"high_pass_cutoff_hz"`
	OutputSwingVPP       RangeExpectation          `json:"output_swing_vpp"`
	OutputCurrentMA      RangeExpectation          `json:"output_current_ma"`
	StabilityMarginDeg   RangeExpectation          `json:"stability_margin_deg"`
	LoadImpedanceOhms    float64                   `json:"load_impedance_ohms,omitempty"`
	LoadDCMaxAbsV        *float64                  `json:"load_dc_max_abs_v,omitempty"`
	SupplyVoltage        string                    `json:"supply_voltage,omitempty"`
	RequiredMeasurements []string                  `json:"required_measurements,omitempty"`
}

type OperatingPointExpectation struct {
	OutputDCMinV float64 `json:"output_dc_min_v"`
	OutputDCMaxV float64 `json:"output_dc_max_v"`
	IdleMinMA    float64 `json:"idle_min_ma"`
	IdleMaxMA    float64 `json:"idle_max_ma"`
}

type GainExpectation struct {
	Nominal float64 `json:"nominal"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

type RangeExpectation struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type SimulationArtifact struct {
	Status      SimulationStatus      `json:"simulation_status"`
	Format      string                `json:"format"`
	Netlist     string                `json:"netlist"`
	Expectation SimulationExpectation `json:"expectation"`
	Notes       []string              `json:"notes,omitempty"`
}

func ClassABHeadphoneSimulationExpectation() SimulationExpectation {
	return SimulationExpectation{
		OperatingPoint: OperatingPointExpectation{OutputDCMinV: 3.8, OutputDCMaxV: 5.2, IdleMinMA: 0, IdleMaxMA: 30},
		ACGain:         GainExpectation{Nominal: 2, Min: 1.8, Max: 2.2},
		HighPassCutoffHz: RangeExpectation{
			Min: 10,
			Max: 60,
		},
		OutputSwingVPP:       RangeExpectation{Min: 1, Max: 6},
		OutputCurrentMA:      RangeExpectation{Min: 1, Max: 100},
		StabilityMarginDeg:   RangeExpectation{Min: 45, Max: 180},
		LoadImpedanceOhms:    32,
		LoadDCMaxAbsV:        float64Ptr(defaultLoadDCMaxAbsV),
		SupplyVoltage:        defaultHeadphoneSupplyVoltage + "V",
		RequiredMeasurements: []string{"operating_point", "ac_gain", "high_pass_cutoff", "output_swing", "load_current", "stability_margin"},
	}
}

func BuildClassABHeadphoneSimulationArtifact(reference string) SimulationArtifact {
	expectation := ClassABHeadphoneSimulationExpectation()
	notes := []string{
		"Simulation execution is opt-in and not run by default.",
		"Netlist is a deterministic structural artifact for future simulator runners.",
	}
	if issues := expectation.Validate(); len(issues) > 0 {
		notes = append(notes, issues...)
	}
	return SimulationArtifact{
		Status:      SimulationStatusNotRun,
		Format:      "spice-like-netlist.v0",
		Netlist:     ClassABHeadphoneSPICENetlist(reference, expectation),
		Expectation: expectation,
		Notes:       notes,
	}
}

func ClassABHeadphoneSPICENetlist(reference string, expectation SimulationExpectation) string {
	reference = sanitizeSPICEText(reference)
	if reference == "" {
		reference = "class_ab_headphone"
	}
	supplyVoltage := spiceVoltageValue(expectation.SupplyVoltage, defaultHeadphoneSupplyVoltage)
	loadOhms := expectation.LoadImpedanceOhms
	if loadOhms <= 0 || simulationInvalidFloat(loadOhms) {
		loadOhms = defaultHeadphoneLoadOhms
	}
	requestedGain := simulationRequestedGain(expectation)
	feedbackGain := requestedGain
	if feedbackGain < 1 {
		feedbackGain = 1
	}
	rgOhms := float64(defaultFeedbackReferenceOhms)
	rfOhms := (feedbackGain - 1) * rgOhms
	if rfOhms < minFeedbackOhms {
		rfOhms = minFeedbackOhms
	}
	outputCapFarads := outputCouplingCapFarads(expectation, loadOhms)
	inputNode, attenuationLines := simulationInputAttenuation(requestedGain)
	header := []string{
		"* KiCadAI amplifier simulation artifact: " + reference,
		"* status: " + string(SimulationStatusNotRun),
		fmt.Sprintf("* expected_gain: %.3g %.3g %.3g", expectation.ACGain.Min, expectation.ACGain.Nominal, expectation.ACGain.Max),
		fmt.Sprintf("* expected_high_pass_hz: %.3g %.3g", expectation.HighPassCutoffHz.Min, expectation.HighPassCutoffHz.Max),
		fmt.Sprintf("* expected_load_ohms: %.3g", loadOhms),
		"* rails: 0V/" + supplyVoltage + "V single-supply, signal reference: VCC/2",
	}
	for _, issue := range expectation.Validate() {
		header = append(header, "* expectation_issue: "+sanitizeSPICEText(issue))
	}
	body := []string{
		"VCC vcc 0 DC " + supplyVoltage,
		"VIN vin 0 AC 1 SIN(0 0.1 1000)",
		"RBIAS1 vcc vbias 100k",
		"RBIAS2 vbias 0 100k",
		"RIN vin bias_in 100",
		"CIN bias_in amp_in 1u",
		"RBIAS_IN amp_in vbias 100k",
	}
	body = append(body, attenuationLines...)
	body = append(body, []string{
		"XU1 " + inputNode + " feedback driver_out vcc 0 OPAMP",
		fmt.Sprintf("RF hp_drive feedback %.6g", rfOhms),
		fmt.Sprintf("RG feedback vbias %.6g", rgOhms),
		"RBIASP vcc bias_p 10k",
		"RBIASN bias_n 0 10k",
		"D1 bias_p driver_out D4148",
		"D2 driver_out bias_n D4148",
		"Q1 vcc bias_p e1 NPN3904",
		"Q2 0 bias_n e2 PNP3906",
		"RE1 e1 hp_drive 0.47",
		"RE2 e2 hp_drive 0.47",
		fmt.Sprintf("COUT hp_drive hp_out %.6g", outputCapFarads),
		fmt.Sprintf("RLOAD hp_out 0 %.6g", loadOhms),
	}...)
	footer := []string{
		".subckt OPAMP noninv inv out vcc vee",
		"EGAIN internal vee value={1e5*(V(noninv)-V(inv))}",
		"RPOLE internal out_buf 1k",
		"CPOLE out_buf vee 15.9n",
		"ECLAMP out vee value={limit(V(out_buf), V(vee)+0.2, V(vcc)-0.2)}",
		".ends OPAMP",
		".model D4148 D",
		".model NPN3904 NPN(Is=6.734f Xti=3 Eg=1.11 Vaf=74.03 Bf=416.4 Ne=1.259 Ise=6.734f Ikf=66.78m Xtb=1.5 Br=0.7371 Rc=1 Cjc=3.638p Mjc=0.3085 Vjc=0.75 Fc=0.5 Cje=4.493p Mje=0.2593 Vje=0.75 Tr=239.5n Tf=301.2p Itf=0.4 Vtf=4 Xtf=2 Rb=10)",
		".model PNP3906 PNP(Is=1.41f Xti=3 Eg=1.11 Vaf=18.7 Bf=180.7 Ne=1.5 Ikf=80m Xtb=1.5 Br=4.977 Rc=2.5 Cjc=9.728p Mjc=0.5776 Vjc=0.75 Fc=0.5 Cje=8.063p Mje=0.3677 Vje=0.75 Tr=33.42n Tf=179.3p Itf=0.4 Vtf=4 Xtf=6 Rb=10)",
		".op",
		".ac dec 20 10 100k",
		".tran 0.1m 50m",
		".end",
	}
	lines := append(header, body...)
	lines = append(lines, footer...)
	return strings.Join(lines, "\n") + "\n"
}

func (expectation SimulationExpectation) Validate() []string {
	var issues []string
	if simulationInvalidFloat(expectation.OperatingPoint.OutputDCMinV) || simulationInvalidFloat(expectation.OperatingPoint.OutputDCMaxV) {
		issues = append(issues, "operating_point output DC bounds must be finite")
	}
	if expectation.OperatingPoint.OutputDCMinV > expectation.OperatingPoint.OutputDCMaxV {
		issues = append(issues, "operating_point output DC min exceeds max")
	}
	if simulationInvalidFloat(expectation.OperatingPoint.IdleMinMA) || simulationInvalidFloat(expectation.OperatingPoint.IdleMaxMA) {
		issues = append(issues, "operating_point idle current bounds must be finite")
	}
	if expectation.OperatingPoint.IdleMinMA > expectation.OperatingPoint.IdleMaxMA {
		issues = append(issues, "operating_point idle current min exceeds max")
	}
	if simulationInvalidFloat(expectation.ACGain.Min) || simulationInvalidFloat(expectation.ACGain.Nominal) || simulationInvalidFloat(expectation.ACGain.Max) {
		issues = append(issues, "ac_gain values must be finite")
	}
	if expectation.ACGain.Nominal <= 0 {
		issues = append(issues, "ac_gain nominal must be positive")
	}
	if expectation.ACGain.Min > expectation.ACGain.Nominal || expectation.ACGain.Nominal > expectation.ACGain.Max {
		issues = append(issues, "ac_gain must satisfy min <= nominal <= max")
	}
	issues = appendRangeIssue(issues, "high_pass_cutoff_hz", expectation.HighPassCutoffHz)
	issues = appendRangeIssue(issues, "output_swing_vpp", expectation.OutputSwingVPP)
	issues = appendRangeIssue(issues, "output_current_ma", expectation.OutputCurrentMA)
	issues = appendRangeIssue(issues, "stability_margin_deg", expectation.StabilityMarginDeg)
	if simulationInvalidFloat(expectation.LoadImpedanceOhms) {
		issues = append(issues, "load_impedance_ohms must be finite")
	}
	if expectation.LoadImpedanceOhms <= 0 {
		issues = append(issues, "load_impedance_ohms must be positive")
	}
	if expectation.LoadDCMaxAbsV != nil {
		if simulationInvalidFloat(*expectation.LoadDCMaxAbsV) {
			issues = append(issues, "load_dc_max_abs_v must be finite")
		}
		if *expectation.LoadDCMaxAbsV < 0 {
			issues = append(issues, "load_dc_max_abs_v must be non-negative")
		}
	}
	return issues
}

func appendRangeIssue(issues []string, name string, value RangeExpectation) []string {
	if simulationInvalidFloat(value.Min) || simulationInvalidFloat(value.Max) {
		return append(issues, name+" bounds must be finite")
	}
	if value.Min > value.Max {
		return append(issues, name+" min exceeds max")
	}
	return issues
}

func simulationInvalidFloat(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

func float64Ptr(value float64) *float64 {
	return &value
}

func simulationRequestedGain(expectation SimulationExpectation) float64 {
	if simulationInvalidFloat(expectation.ACGain.Nominal) || expectation.ACGain.Nominal <= 0 {
		return defaultHeadphoneGain
	}
	return expectation.ACGain.Nominal
}

func simulationInputAttenuation(requestedGain float64) (string, []string) {
	if requestedGain <= 0 || requestedGain >= 1-unityGainEpsilon {
		return "amp_in", nil
	}
	topOhms := float64(defaultAttenuationTopOhms)
	bottomOhms := requestedGain * topOhms / (1 - requestedGain)
	if simulationInvalidFloat(bottomOhms) || bottomOhms <= 0 {
		return "amp_in", nil
	}
	if bottomOhms > maxAttenuationResistorOhms {
		ratio := bottomOhms / topOhms
		bottomOhms = maxAttenuationResistorOhms
		topOhms = bottomOhms / ratio
		if topOhms < minAttenuationTopOhms {
			return "amp_in", nil
		}
	}
	return "atten_in", []string{
		fmt.Sprintf("RATTEN_TOP amp_in atten_in %.6g", topOhms),
		fmt.Sprintf("RATTEN_BOTTOM atten_in vbias %.6g", bottomOhms),
	}
}

func outputCouplingCapFarads(expectation SimulationExpectation, loadOhms float64) float64 {
	cutoffHz := simulationHighPassCutoffHz(expectation)
	if loadOhms <= 0 || simulationInvalidFloat(loadOhms) || cutoffHz <= 0 {
		return defaultOutputCouplingFarads
	}
	capFarads := 1 / (2 * math.Pi * loadOhms * cutoffHz)
	if simulationInvalidFloat(capFarads) || capFarads <= 0 || capFarads > maxOutputCouplingFarads {
		return defaultOutputCouplingFarads
	}
	return capFarads
}

func simulationHighPassCutoffHz(expectation SimulationExpectation) float64 {
	minHz := expectation.HighPassCutoffHz.Min
	maxHz := expectation.HighPassCutoffHz.Max
	if simulationInvalidFloat(minHz) || simulationInvalidFloat(maxHz) || minHz <= 0 || maxHz <= 0 {
		return defaultHighPassCutoffHz
	}
	cutoffHz := math.Exp((math.Log(minHz) + math.Log(maxHz)) / 2)
	if simulationInvalidFloat(cutoffHz) || cutoffHz <= 0 {
		return defaultHighPassCutoffHz
	}
	return cutoffHz
}

func spiceVoltageValue(voltage string, fallback string) string {
	voltage = sanitizeSPICEText(voltage)
	voltage = strings.TrimRight(strings.TrimSpace(voltage), "Vv")
	match := spiceNumericTokenPattern.FindStringSubmatch(voltage)
	if match == nil {
		return fallback
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return fallback
	}
	multiplier, ok := spiceSuffixMultiplier(match[2])
	if !ok {
		return fallback
	}
	return strconv.FormatFloat(value*multiplier, 'g', -1, 64)
}

func spiceSuffixMultiplier(suffix string) (float64, bool) {
	switch strings.ToLower(suffix) {
	case "":
		return 1, true
	case "v", "volt", "volts":
		return 1, true
	case "t":
		return 1e12, true
	case "g":
		return 1e9, true
	case "meg":
		return 1e6, true
	case "k":
		return 1e3, true
	case "m":
		return 1e-3, true
	case "u":
		return 1e-6, true
	case "n":
		return 1e-9, true
	case "p":
		return 1e-12, true
	case "f":
		return 1e-15, true
	default:
		return 0, false
	}
}

func sanitizeSPICEText(value string) string {
	value = strings.TrimSpace(value)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, value)
}
