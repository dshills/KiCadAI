package amplifiers

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestValidateSpeakerAmplifierPassesProtectedTenWattCandidate(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	result := ValidateSpeakerAmplifier(request)
	if !result.Pass {
		t.Fatalf("speaker validation failed: %s issues=%#v", FormatValidationFailure(result), result.Issues)
	}
	if len(result.Analyses) != 15 {
		t.Fatalf("analysis count = %d, want 15: %#v", len(result.Analyses), result.Analyses)
	}
	power := requireSpeakerAnalysis(t, result, AnalysisOutputPower)
	if power.Measurements["available_output_power_w"] < 10 {
		t.Fatalf("output power measurements = %#v", power.Measurements)
	}
	thermal := requireSpeakerAnalysis(t, result, AnalysisElectrothermal)
	if thermal.Measurements["blocked_heatsink_temperature_c"] <= thermal.Measurements["hot_heatsink_temperature_c"] {
		t.Fatalf("thermal corners = %#v", thermal.Measurements)
	}
}

func TestValidateSpeakerAmplifierFailsClosedAcrossEverySpeakerDomain(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.Components.Relay = nil
	request.TargetPowerW = 100
	request.Loads = request.Loads[:2]
	request.OpAmpOutputCurrentA = 1e-6
	request.Distortion.MaximumTHDPct = 0.001
	request.Thermal.BlockedAirflowFactor = 0
	request.Protection.RelayClampPresent = false
	request.Layout.StarGround = false
	result := ValidateSpeakerAmplifier(request)
	if result.Pass {
		t.Fatal("unsafe speaker candidate unexpectedly passed")
	}
	for _, code := range []reports.Code{
		CodeSpeakerComponentEvidence,
		CodeSpeakerOutputPower,
		CodeSpeakerReactiveLoad,
		CodeSpeakerDriverLimit,
		CodeSpeakerDistortion,
		CodeSpeakerElectrothermal,
		CodeSpeakerProtection,
		CodeSpeakerLayout,
	} {
		if !validationHasCode(result, code) {
			t.Fatalf("issues = %#v, want %s", result.Issues, code)
		}
	}
}

func TestValidateSpeakerAmplifierRejectsShortOutsideTemperatureDeratedSOA(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.Protection.CurrentLimitA = 6
	request.Protection.SenseThresholdV = request.Protection.CurrentLimitA * request.EmitterResistanceOhm
	request.Envelope.OutputPeakCurrentA = 6
	result := ValidateSpeakerAmplifier(request)
	if result.Pass || !validationHasIssuePath(result, CodeSpeakerElectrothermal, "upper_output.soa") || !validationHasIssuePath(result, CodeSpeakerElectrothermal, "lower_output.soa") {
		t.Fatalf("result = %#v, want complementary short-SOA failures", result)
	}
}

func TestValidateSpeakerAmplifierRejectsNormalizedLoadNameCollision(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.Loads[0].Name = "Load A"
	request.Loads[1].Name = "Load-A"
	result := ValidateSpeakerAmplifier(request)
	if result.Pass || !validationHasIssuePath(result, CodeSpeakerReactiveLoad, "loads.Load-A") {
		t.Fatalf("issues = %#v, want normalized load-name collision", result.Issues)
	}
}

func TestValidateSpeakerAmplifierAllowsExplicitCustomLoadSet(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.RequireStandardLoadCoverage = false
	request.Loads = []SpeakerLoadCase{{Name: "custom_6_ohm", ResistanceOhm: 6, PhaseMarginDeg: 50}}
	result := ValidateSpeakerAmplifier(request)
	if !result.Pass {
		t.Fatalf("custom bounded load set issues = %#v", result.Issues)
	}
}

func TestValidateSpeakerAmplifierSupportsConfiguredTwoOhmLoad(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.RequireStandardLoadCoverage = false
	request.MinimumLoadOhm = 2
	request.MaximumLoadOhm = 8
	request.Loads = []SpeakerLoadCase{{Name: "automotive_2_ohm", ResistanceOhm: 2, PhaseMarginDeg: 50}}
	result := validateSpeakerLoads(request)
	if !result.Pass {
		t.Fatalf("configured 2 ohm load issues = %#v", result.Issues)
	}
}

func TestValidateSpeakerAmplifierRejectsLoadOutsideConfiguredEnvelope(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.RequireStandardLoadCoverage = false
	request.MinimumLoadOhm = 6
	request.MaximumLoadOhm = 16
	request.Loads = []SpeakerLoadCase{{Name: "outside", ResistanceOhm: 4, PhaseMarginDeg: 50}}
	result := validateSpeakerLoads(request)
	if result.Pass || !validationHasIssuePath(ValidationResult{Pass: result.Pass, Analyses: []AnalysisResult{result}, Issues: result.Issues}, CodeSpeakerReactiveLoad, "loads.outside") {
		t.Fatalf("configured envelope issues = %#v, want rejected 4 ohm load", result.Issues)
	}
}

func TestMeasurementNamePreservesUnicodeIdentity(t *testing.T) {
	word := measurementName("8 Ohm")
	symbol := measurementName("8ω")
	if word == "" || symbol == "" || word == symbol || !strings.Contains(symbol, "u3c9") {
		t.Fatalf("measurement keys word=%q symbol=%q, want distinct deterministic Unicode identity", word, symbol)
	}
}

func TestMeasurementNameBoundsLongKeysWithoutCollisions(t *testing.T) {
	left := measurementName(strings.Repeat("speaker", 20) + "left")
	right := measurementName(strings.Repeat("speaker", 20) + "right")
	if len(left) > measurementNameMaxLength || len(right) > measurementNameMaxLength || left == right {
		t.Fatalf("bounded measurement keys left=%q right=%q", left, right)
	}
}

func TestValidateSpeakerAmplifierAcceptsProcurementUsableLifecycle(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	relay := *request.Components.Relay
	relay.Lifecycle = "nrnd"
	request.Components.Relay = &relay
	result := ValidateSpeakerAmplifier(request)
	if !result.Pass {
		t.Fatalf("NRND but procurement-usable relay rejected: %#v", result.Issues)
	}
	relay.Lifecycle = "obsolete"
	result = ValidateSpeakerAmplifier(request)
	if result.Pass || !validationHasIssuePath(result, CodeSpeakerComponentEvidence, "relay") {
		t.Fatalf("obsolete relay result = %#v, want component-evidence failure", result)
	}
}

func TestValidateSpeakerAmplifierUsesRelayCatalogContactRating(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	relay := *request.Components.Relay
	relay.Ratings = append([]components.RatingConstraint(nil), relay.Ratings...)
	for index := range relay.Ratings {
		if relay.Ratings[index].Kind == "contact_current_dc" {
			relay.Ratings[index].Max = "2"
		}
	}
	request.Components.Relay = &relay
	result := ValidateSpeakerAmplifier(request)
	if result.Pass || !validationHasIssuePath(result, CodeSpeakerProtection, "relay") {
		t.Fatalf("undersized relay evidence result = %#v, want protection failure", result)
	}
}

func TestSpeakerShortTransientUsesJunctionToCaseResistance(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	analysis := validateSpeakerElectrothermal(request)
	upper := powerEvidence(request.Components.UpperOutput)
	lower := powerEvidence(request.Components.LowerOutput)
	rail := speakerRailMagnitude(request.Envelope)
	minLoad := minimumLoadResistance(request.Loads)
	peak := math.Min(2*rail/(math.Pi*minLoad), request.Protection.CurrentLimitA)
	perDeviceW := rail*peak/math.Pi - peak*peak*minLoad/4 + rail*request.Envelope.QuiescentCurrentA
	blockedSinkC := request.Thermal.HotAmbientC + 2*perDeviceW*request.Thermal.SinkToAmbientCPerW*request.Thermal.BlockedAirflowFactor
	blockedCaseC := blockedSinkC + perDeviceW*request.Thermal.CaseToSinkCPerW
	if got := analysis.Measurements["upper_blocked_case_temperature_c"]; math.Abs(got-blockedCaseC) > 1e-9 || got <= blockedSinkC {
		t.Fatalf("upper blocked case = %g, want case-to-sink rise above %g", got, blockedSinkC)
	}
	expected := blockedSinkC + rail*request.Protection.CurrentLimitA*(request.Thermal.CaseToSinkCPerW+math.Max(*upper.JunctionToCaseCPerW, *lower.JunctionToCaseCPerW))*request.Thermal.TransientRJCFactor
	if got := analysis.Measurements["short_transient_junction_temperature_c"]; math.Abs(got-expected) > 1e-9 {
		t.Fatalf("short transient junction = %g, want thermal-resistance result %g", got, expected)
	}
}

func TestValidateSpeakerAmplifierRejectsProtectionFailuresByPath(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SpeakerAmplifierRequest)
		path   string
	}{
		{name: "current_limit", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.SenseThresholdV = 0.1 }, path: "current_limit"},
		{name: "zero_sense_resistance", mutate: func(r *SpeakerAmplifierRequest) { r.EmitterResistanceOhm = 0 }, path: "current_limit"},
		{name: "dc_detection", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.PositiveDCTripV = 3 }, path: "dc_detection"},
		{name: "turn_on", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.TurnOnDelayS = 0.1 }, path: "turn_on_mute"},
		{name: "release", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.ReleaseTimeS = 1 }, path: "release"},
		{name: "relay", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.RelayNormallyOpen = false }, path: "relay"},
		{name: "zobel", mutate: func(r *SpeakerAmplifierRequest) { r.Protection.ZobelCapacitanceF = 0 }, path: "zobel"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validSpeakerAmplifierRequest(t)
			test.mutate(&request)
			result := ValidateSpeakerAmplifier(request)
			if result.Pass || !validationHasIssuePath(result, CodeSpeakerProtection, test.path) {
				t.Fatalf("issues = %#v, want protection path %q", result.Issues, test.path)
			}
		})
	}
}

func TestSpeakerValidationInvalidInputsRemainJSONSafe(t *testing.T) {
	request := validSpeakerAmplifierRequest(t)
	request.Loads[0].ResistanceOhm = math.NaN()
	result := ValidateSpeakerAmplifier(request)
	if result.Pass {
		t.Fatal("non-finite speaker request unexpectedly passed")
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("invalid speaker result is not JSON-safe: %v", err)
	}
}

func TestSanitizeAnalysisMeasurementsDropsNonFiniteValues(t *testing.T) {
	analyses := []AnalysisResult{{Measurements: map[string]float64{
		"finite": 1,
		"nan":    math.NaN(),
		"inf":    math.Inf(1),
	}}}
	sanitizeAnalysisMeasurements(analyses)
	if len(analyses[0].Measurements) != 1 || analyses[0].Measurements["finite"] != 1 {
		t.Fatalf("sanitized measurements = %#v, want only finite value", analyses[0].Measurements)
	}
}

func validSpeakerAmplifierRequest(t *testing.T) SpeakerAmplifierRequest {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatalf("load component catalog: %v", err)
	}
	upperOutput := speakerCatalogRecord(t, catalog, "bjt.onsemi.njw0281g.to3p")
	return SpeakerAmplifierRequest{
		RequireStandardLoadCoverage: true,
		Envelope: ValidationRequest{
			Topology:                 "class_ab_speaker",
			SupplyVoltageV:           36,
			NegativeRailVoltageV:     -18,
			PositiveRailVoltageV:     18,
			OutputBiasV:              0,
			QuiescentCurrentA:        0.06,
			OutputPeakVoltageV:       13,
			OutputPeakCurrentA:       1.7,
			LoadImpedanceOhm:         8,
			MaximumSignalFrequencyHz: 20_000,
			ClosedLoopGain:           20,
			NoiseGain:                20,
			GainBandwidthHz:          8_000_000,
			SlewRateVPerS:            20_000_000,
			PhaseMarginDeg:           58,
			DeviceDissipationW:       8,
			AmbientTemperatureC:      50,
			CaseToSinkCPerW:          0.4,
			SinkToAmbientCPerW:       1,
			SOAVoltageV:              9,
			SOACurrentA:              1,
			SOATemperatureC:          70,
			Device:                   upperOutput,
			ToleranceCorners: []OperatingCorner{
				{Name: "cold_min_bias", OutputBiasV: -0.2, QuiescentCurrentA: 0.04, DeviceDissipationW: 7.4, PhaseMarginDeg: 55},
				{Name: "hot_max_bias", OutputBiasV: 0.2, QuiescentCurrentA: 0.08, DeviceDissipationW: 8.8, PhaseMarginDeg: 48},
			},
		},
		TargetPowerW:            10,
		TargetLoadOhm:           8,
		OutputStageLossV:        4,
		DriverGainMinimum:       15,
		OutputGainMinimum:       75,
		OpAmpOutputCurrentA:     0.04,
		EmitterResistanceOhm:    0.22,
		EmitterResistorRatingW:  3,
		EmitterResistorDerating: 0.6,
		BiasSpreadV:             2.6,
		RequiredBiasSpreadV:     2.6,
		BiasToleranceV:          0.2,
		Loads: []SpeakerLoadCase{
			{Name: "8_ohm_resistive", ResistanceOhm: 8, PhaseMarginDeg: 60},
			{Name: "4_ohm_resistive", ResistanceOhm: 4, PhaseMarginDeg: 52},
			{Name: "6_ohm_reactive", ResistanceOhm: 6, InductanceH: 500e-6, CapacitanceF: 1e-6, PhaseMarginDeg: 48},
		},
		Distortion: SpeakerDistortionBudget{MaximumTHDPct: 0.1, OpAmpTHDPct: 0.001, CrossoverTHDPct: 0.03, FeedbackTHDPct: 0.01, LoadTHDPct: 0.005},
		Thermal: SpeakerThermalModel{
			HeatsinkManufacturer: "Wakefield-Vette",
			HeatsinkMPN:          "641K",
			SinkToAmbientCPerW:   2.4,
			CaseToSinkCPerW:      0.4,
			HotAmbientC:          50,
			BlockedAirflowFactor: 1.4,
			JunctionMarginC:      20,
			TransientRJCFactor:   0.2,
			TransientBasis:       "conservative bounded one-second assembly approximation",
		},
		Protection: SpeakerProtectionContract{
			CurrentLimitA:         1.7,
			SenseThresholdV:       0.374,
			ShortCircuitDurationS: 1,
			PositiveDCTripV:       1.2,
			NegativeDCTripV:       1.2,
			MaximumDCTripV:        1.5,
			TurnOnDelayS:          2.5,
			MinimumTurnOnDelayS:   1,
			MaximumTurnOnDelayS:   5,
			ReleaseTimeS:          0.1,
			MaximumReleaseTimeS:   0.25,
			RelayContactRatingA:   5,
			RelayNormallyOpen:     true,
			RelayClampPresent:     true,
			SupplyFaultRelease:    true,
			TurnOffMute:           true,
			ZobelResistanceOhm:    10,
			ZobelCapacitanceF:     100e-9,
		},
		Layout: SpeakerLayoutContract{
			StarGround:                  true,
			KelvinFeedback:              true,
			KelvinEmitterSense:          true,
			LocalRailDecoupling:         true,
			ThermalBiasCoupling:         true,
			ComplementarySymmetry:       true,
			HeatsinkKeepout:             true,
			MountingAccess:              true,
			CopperWidthMM:               2,
			MinimumCopperWidthMM:        1.5,
			CopperClearanceMM:           0.5,
			MinimumClearanceMM:          0.4,
			HighCurrentLoopLengthMM:     45,
			MaximumLoopLengthMM:         60,
			FeedbackSeparationMM:        10,
			MinimumFeedbackSeparationMM: 8,
		},
		Components: SpeakerComponentSet{
			OpAmp:           speakerCatalogRecord(t, catalog, "opamp.ti.opa134ua.soic8"),
			UpperDriver:     speakerCatalogRecord(t, catalog, "bjt.onsemi.mje243g.to225"),
			LowerDriver:     speakerCatalogRecord(t, catalog, "bjt.onsemi.mje253g.to225"),
			UpperOutput:     upperOutput,
			LowerOutput:     speakerCatalogRecord(t, catalog, "bjt.onsemi.njw0302g.to3p"),
			EmitterResistor: speakerCatalogRecord(t, catalog, "resistor.vishay.ac03.0r22.axial"),
			ZobelResistor:   speakerCatalogRecord(t, catalog, "resistor.vishay.ac03.10r.axial"),
			Relay:           speakerCatalogRecord(t, catalog, "relay.omron.g5q_1a.dc12"),
		},
	}
}

func speakerCatalogRecord(t *testing.T, catalog *components.Catalog, id string) *components.ComponentRecord {
	t.Helper()
	for index := range catalog.Records {
		if catalog.Records[index].ID == id {
			return &catalog.Records[index]
		}
	}
	t.Fatalf("component %q not found", id)
	return nil
}

func requireSpeakerAnalysis(t *testing.T, result ValidationResult, kind AnalysisKind) AnalysisResult {
	t.Helper()
	for _, analysis := range result.Analyses {
		if analysis.Kind == kind {
			return analysis
		}
	}
	t.Fatalf("analysis %q not found: %#v", kind, result.Analyses)
	return AnalysisResult{}
}

func validationHasIssuePath(result ValidationResult, code reports.Code, path string) bool {
	return slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool {
		return issue.Code == code && strings.HasSuffix(issue.Path, "."+path)
	})
}
