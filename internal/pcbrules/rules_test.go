package pcbrules

import "testing"

func TestResolveMergesPerAttributeByPrecedence(t *testing.T) {
	set := RuleSet{
		TraceWidthMM:  floatPtr(0.20),
		ClearanceMM:   floatPtr(0.15),
		MaxViasPerNet: intPtr(4),
		Classes: map[string]Class{
			"POWER": {
				TraceWidthMM: floatPtr(0.60),
			},
		},
		NetOverrides: map[string]Rule{
			"VBUS": {
				ClassName:     "POWER",
				MaxViasPerNet: intPtr(1),
			},
		},
	}

	got, issues := Resolve(set, NetDescriptor{Name: "VBUS", Class: "SIGNAL", Role: RolePower})
	if len(issues) != 0 {
		t.Fatalf("Resolve issues = %#v", issues)
	}
	if got.ClassName != "POWER" {
		t.Fatalf("class = %q, want override class", got.ClassName)
	}
	if got.TraceWidthMM != 0.60 {
		t.Fatalf("trace width = %v, want class width", got.TraceWidthMM)
	}
	if got.ClearanceMM != 0.15 {
		t.Fatalf("clearance = %v, want global fallback", got.ClearanceMM)
	}
	if got.MaxViasPerNet != 1 {
		t.Fatalf("max vias = %v, want override", got.MaxViasPerNet)
	}
}

func TestResolverCachesResolvedRules(t *testing.T) {
	resolver := NewResolver(RuleSet{
		TraceWidthMM: floatPtr(0.25),
		NetOverrides: map[string]Rule{
			"SIG": {TraceWidthMM: floatPtr(0.40)},
		},
	})
	first, firstIssues := resolver.Resolve(NetDescriptor{Name: "SIG"})
	second, secondIssues := resolver.Resolve(NetDescriptor{Name: "SIG"})
	if len(firstIssues) != 0 || len(secondIssues) != 0 {
		t.Fatalf("issues = %#v / %#v", firstIssues, secondIssues)
	}
	if first.TraceWidthMM != second.TraceWidthMM || second.TraceWidthMM != 0.40 {
		t.Fatalf("cached rule mismatch: %#v / %#v", first, second)
	}
}

func TestValidateRejectsUnnormalizedClearanceMatrixKey(t *testing.T) {
	issues := Validate(RuleSet{
		ClearanceMatrix: ClearanceMatrix{
			"LOGIC|HV": 1.0,
		},
	})
	if !hasIssuePath(issues, "clearance_matrix[LOGIC|HV]") {
		t.Fatalf("expected clearance matrix issue, got %#v", issues)
	}
}

func TestValidateRejectsDifferentialPairMemberCount(t *testing.T) {
	issues := Validate(RuleSet{
		DifferentialPairs: []CoupledNetGroup{{
			ID:      "USB",
			Mode:    DifferentialPairMode,
			Members: []string{"USB_P", "USB_N", "USB_SHIELD"},
		}},
	})
	if !hasIssuePath(issues, "differential_pairs[0].members") {
		t.Fatalf("expected differential pair member issue, got %#v", issues)
	}
}

func TestResolveRejectsNeckdownBelowManufacturingMinimum(t *testing.T) {
	got, issues := Resolve(RuleSet{
		TraceWidthMM:       floatPtr(0.25),
		NeckdownWidthMM:    floatPtr(0.08),
		MinNeckdownWidthMM: floatPtr(0.10),
	}, NetDescriptor{Name: "SIG"})
	if got.NeckdownWidthMM != 0.08 {
		t.Fatalf("neckdown = %v, want resolved value", got.NeckdownWidthMM)
	}
	if !hasIssuePath(issues, "neckdown_width_mm") {
		t.Fatalf("expected neckdown minimum issue, got %#v", issues)
	}
}

func hasIssuePath(issues []Issue, path string) bool {
	for _, issue := range issues {
		if issue.Path == path {
			return true
		}
	}
	return false
}
