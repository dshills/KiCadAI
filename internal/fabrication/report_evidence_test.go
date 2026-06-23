package fabrication

import "testing"

func TestApplyReportEvidenceSetsIdentityConsistencyAndAssemblyGates(t *testing.T) {
	result := Result{}
	applyReportEvidence(&result, ReportData{
		BOM: []BOMRow{{
			References:     []string{"U1"},
			Manufacturer:   "Microchip",
			MPN:            "ATMEGA328P-AU",
			IdentityStatus: IdentityPass,
		}},
		CPL: []CPLRow{{Reference: "U1", NormalizedSide: "top"}},
		Consistency: ConsistencySummary{
			CheckedReferences: 1,
			MatchedReferences: 1,
		},
	}, Options{ManufacturerProfile: GenericAssemblyProfileID})
	if result.Summary.ComponentIdentity != EvidencePass || result.Summary.BOMCPLConsistency != EvidencePass || result.Summary.ManufacturerProfile != EvidencePass || result.Summary.AssemblyReadiness != EvidencePass {
		t.Fatalf("summary = %#v, want pass gates", result.Summary)
	}
}

func TestApplyReportEvidenceBlocksUnknownManufacturerProfile(t *testing.T) {
	result := Result{}
	applyReportEvidence(&result, ReportData{BOM: []BOMRow{{References: []string{"U1"}}}, CPL: []CPLRow{{Reference: "U1"}}}, Options{ManufacturerProfile: "missing"})
	if result.Summary.ManufacturerProfile != EvidenceFail || result.Summary.AssemblyReadiness != EvidenceFail {
		t.Fatalf("summary = %#v, want failed profile gate", result.Summary)
	}
	if !hasIssuePath(result.Issues, "manufacturer_profile") {
		t.Fatalf("issues = %#v, want unknown profile issue", result.Issues)
	}
}

func TestComponentIdentityEvidenceSkipsMechanicalRows(t *testing.T) {
	status := componentIdentityEvidence([]BOMRow{{
		References:     []string{"MH1"},
		ComponentClass: "mechanical",
	}})
	if status != EvidencePass {
		t.Fatalf("status = %s, want pass for mechanical row without MPN", status)
	}
}

func TestSummaryEvidenceIncludesAssemblyGates(t *testing.T) {
	evidence := summaryEvidence(Summary{
		ComponentIdentity:   EvidencePass,
		BOMCPLConsistency:   EvidenceWarning,
		ManufacturerProfile: EvidenceSkipped,
		AssemblyReadiness:   EvidenceWarning,
	})
	for _, key := range []string{"component_identity", "bom_cpl_consistency", "manufacturer_profile", "assembly_readiness"} {
		if _, ok := evidence[key]; !ok {
			t.Fatalf("evidence missing %s: %#v", key, evidence)
		}
	}
}
