package libraryresolver

import (
	"testing"

	"kicadai/internal/reports"
)

func TestPinmapCandidateNumericTwoPinPassive(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:R", "Resistor_SMD:R_0805_2012Metric")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.PinmapCandidate[0].SymbolPin != "1" || result.PinmapCandidate[0].FootprintPad != "1" {
		t.Fatalf("candidate = %#v", result.PinmapCandidate)
	}
	assertPinmapUnverified(t, result)
}

func TestPinmapCandidateConnectorMatchingPins(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Connector:Conn_01x02", "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.PinmapCandidate[1].SymbolPin != "2" || result.PinmapCandidate[1].FootprintPad != "2" {
		t.Fatalf("candidate = %#v", result.PinmapCandidate)
	}
	assertPinmapUnverified(t, result)
}

func TestPinmapCandidateTransistorNeedsVerification(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:Q_NPN", "Package_TO_SOT_THT:TO-92")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 3 {
		t.Fatalf("result = %#v", result)
	}
	assertPinmapUnverified(t, result)
}

func TestPinmapCandidateAllowsStackedSymbolPins(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:STACKED", "Resistor_SMD:R_0805_2012Metric")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestPinmapCandidateUsesFunctionHintsWhenDesignatorsDiffer(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:FUNC", "Function:FuncPads")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.PinmapCandidate[0].FootprintPad != "P1" || result.PinmapCandidate[1].FootprintPad != "P2" {
		t.Fatalf("candidate = %#v", result.PinmapCandidate)
	}
	assertPinmapUnverified(t, result)
}

func TestPinmapCandidateDuplicatePadGroup(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:R", "Duplicate:DupPads")
	if result.Status != CompatibilityCandidate || len(result.PinmapCandidate) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.PinmapCandidate[0].Confidence >= 0.9 {
		t.Fatalf("duplicate pad candidate should have reduced confidence: %#v", result.PinmapCandidate[0])
	}
}

func TestPinmapCandidateMismatchedPadsBlocks(t *testing.T) {
	index := compatibilityFixture(t)
	result := GeneratePinmapCandidate(index, "Device:R", "Bad:MissingPin")
	if result.Status != CompatibilityIncompatible || len(result.Issues) != 1 || !result.Issues[0].Blocking() {
		t.Fatalf("result = %#v", result)
	}
}

func assertPinmapUnverified(t *testing.T, result CompatibilityResult) {
	t.Helper()
	for _, issue := range result.Issues {
		if issue.Code == reports.CodePinmapUnverified {
			return
		}
	}
	t.Fatalf("missing PINMAP_UNVERIFIED issue: %#v", result.Issues)
}
