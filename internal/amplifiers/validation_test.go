package amplifiers

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestValidationResultRemainsJSONSafeForInvalidTransientInputs(t *testing.T) {
	request := validAmplifierValidationRequest()
	request.LoadImpedanceOhm = 0
	request.MaximumSignalFrequencyHz = math.MaxFloat64
	result := ValidateOperatingEnvelope(request)
	if result.Pass {
		t.Fatal("invalid transient request unexpectedly passed")
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("invalid-input result is not JSON-safe: %v", err)
	}
}

func TestValidateOperatingEnvelopePassesBoundedClassA(t *testing.T) {
	request := validAmplifierValidationRequest()
	result := ValidateOperatingEnvelope(request)
	if !result.Pass {
		t.Fatalf("validation failed: %s issues=%#v", FormatValidationFailure(result), result.Issues)
	}
	if len(result.Analyses) != 7 {
		t.Fatalf("analyses = %#v", result.Analyses)
	}
}

func TestValidateOperatingEnvelopeAcceptsDualSupplyZeroBias(t *testing.T) {
	request := validAmplifierValidationRequest()
	request.NegativeRailVoltageV = -6
	request.PositiveRailVoltageV = 6
	request.OutputBiasV = 0
	request.ToleranceCorners[0].OutputBiasV = -0.2
	request.ToleranceCorners[1].OutputBiasV = 0.2
	result := ValidateOperatingEnvelope(request)
	if !result.Pass {
		t.Fatalf("dual-supply validation failed: %s issues=%#v", FormatValidationFailure(result), result.Issues)
	}
}

func TestValidateOperatingEnvelopeFailsEveryUnsafeDomain(t *testing.T) {
	request := validAmplifierValidationRequest()
	request.OutputBiasV = 0.2
	request.GainBandwidthHz = 1000
	request.SlewRateVPerS = 100
	request.ToleranceCorners[1].PhaseMarginDeg = 20
	request.PhaseMarginDeg = 20
	request.DeviceDissipationW = 1
	request.SOACurrentA = 0.2
	result := ValidateOperatingEnvelope(request)
	if result.Pass {
		t.Fatal("unsafe envelope unexpectedly passed")
	}
	want := []reports.Code{CodeAmplifierDCHeadroom, CodeAmplifierACBandwidth, CodeAmplifierTransientSlew, CodeAmplifierToleranceCorner, CodeAmplifierStabilityMargin, CodeAmplifierThermalLimit, CodeAmplifierSOALimit}
	for _, code := range want {
		if !validationHasCode(result, code) {
			t.Fatalf("issues = %#v, want code %s", result.Issues, code)
		}
	}
}

func TestValidateSOABlocksUnreviewedLinearMOSFET(t *testing.T) {
	request := validAmplifierValidationRequest()
	request.Device.PowerSemiconductor.FabricationProof = false
	request.Device.PowerSemiconductor.SOA = nil
	result := ValidateOperatingEnvelope(request)
	if result.Pass || !validationHasCode(result, CodeAmplifierSOALimit) {
		t.Fatalf("result = %#v, want fail-closed SOA", result)
	}
}

func TestInterpolateDCAllowedCurrentUsesLogBoundary(t *testing.T) {
	points := []components.SOAEnvelopePoint{{VoltageV: 1, CurrentA: 0.2, DC: true, CaseTemperatureC: 25}, {VoltageV: 10, CurrentA: 0.02, DC: true, CaseTemperatureC: 25}}
	current, _, ok := interpolateDCAllowedCurrent(points, 5)
	if !ok || current < 0.0399 || current > 0.0401 {
		t.Fatalf("current = %g ok=%t, want 40mA constant-power interpolation", current, ok)
	}
}

func TestInterpolateDCAllowedCurrentRejectsDegenerateBoundary(t *testing.T) {
	points := []components.SOAEnvelopePoint{
		{VoltageV: 5, CurrentA: 1, CaseTemperatureC: 25, DC: true},
		{VoltageV: 5, CurrentA: 0.5, CaseTemperatureC: 25, DC: true},
	}
	if _, _, ok := interpolateDCAllowedCurrent(points, 5); ok {
		t.Fatal("degenerate SOA boundary unexpectedly interpolated")
	}
}

func TestInterpolateDCAllowedCurrentRejectsZeroCurrentBoundary(t *testing.T) {
	points := []components.SOAEnvelopePoint{
		{VoltageV: 1, CurrentA: 1, CaseTemperatureC: 25, DC: true},
		{VoltageV: 10, CurrentA: 0, CaseTemperatureC: 25, DC: true},
	}
	if _, _, ok := interpolateDCAllowedCurrent(points, 5); ok {
		t.Fatal("zero-current SOA boundary unexpectedly interpolated")
	}
}

func validAmplifierValidationRequest() ValidationRequest {
	maxJunction := 150.0
	thetaJA := 556.0
	return ValidationRequest{
		Topology:                 "class_a_bjt",
		SupplyVoltageV:           12,
		OutputBiasV:              6,
		QuiescentCurrentA:        0.001,
		OutputPeakVoltageV:       1,
		OutputPeakCurrentA:       0.002,
		LoadImpedanceOhm:         10_000,
		MaximumSignalFrequencyHz: 20_000,
		ClosedLoopGain:           10,
		NoiseGain:                10,
		GainBandwidthHz:          3_000_000,
		SlewRateVPerS:            2_000_000,
		PhaseMarginDeg:           60,
		DeviceDissipationW:       0.0055,
		AmbientTemperatureC:      45,
		SOAVoltageV:              5.5,
		SOACurrentA:              0.001,
		SOATemperatureC:          45,
		Device: &components.ComponentRecord{PowerSemiconductor: &components.PowerSemiconductorEvidence{
			DeviceClass: "bjt", Polarity: "npn", FabricationProof: true,
			MaxJunctionTemperatureC: &maxJunction, JunctionToAmbientCPerW: &thetaJA,
			PowerDissipation: &components.EvidenceMeasurement{Value: 0.225, Unit: "W", Conditions: "FR-5 board"},
			SOA:              []components.SOAEnvelopePoint{{VoltageV: 1, CurrentA: 0.2, DC: true, CaseTemperatureC: 25}, {VoltageV: 5, CurrentA: 0.04, DC: true, CaseTemperatureC: 25}, {VoltageV: 10, CurrentA: 0.02, DC: true, CaseTemperatureC: 25}, {VoltageV: 40, CurrentA: 0.005, DC: true, CaseTemperatureC: 25}},
		}},
		ToleranceCorners: []OperatingCorner{
			{Name: "cold_min", OutputBiasV: 5.7, QuiescentCurrentA: 0.0008, DeviceDissipationW: 0.0048, PhaseMarginDeg: 58},
			{Name: "hot_max", OutputBiasV: 6.3, QuiescentCurrentA: 0.0012, DeviceDissipationW: 0.0065, PhaseMarginDeg: 52},
		},
	}
}

func validationHasCode(result ValidationResult, code reports.Code) bool {
	return slices.ContainsFunc(result.Issues, func(issue reports.Issue) bool { return issue.Code == code })
}
