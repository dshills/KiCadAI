package repair

import (
	"testing"

	"kicadai/internal/boardvalidation"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

func TestNormalizeWriterCorrectnessCheckMapsParseAndPadNet(t *testing.T) {
	parse := NormalizeWriterCorrectnessCheck(writercorrectness.CheckResult{
		Name: writercorrectness.CheckSchematicParse,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "demo.kicad_sch",
			Message:  "expected kicad_sch root",
		}},
	}, "")
	if len(parse) != 1 || parse[0].Source != FindingSourceWriter || parse[0].Category != FindingCategoryParse {
		t.Fatalf("parse findings = %+v", parse)
	}

	pad := NormalizeWriterCorrectnessCheck(writercorrectness.CheckResult{
		Name: writercorrectness.CheckFootprintPadNets,
		Issues: []reports.Issue{{
			Code:     reports.CodeInvalidNetAssignment,
			Severity: reports.SeverityError,
			Path:     "demo.kicad_pcb.footprints.J1.pads.1",
			Message:  "PCB pad references missing net code",
			Refs:     []string{"J1"},
			Nets:     []string{"VBUS"},
		}},
	}, "")
	if len(pad) != 1 || pad[0].Category != FindingCategoryPadNet || pad[0].Subject.Ref != "J1" || pad[0].Subject.Net != "VBUS" {
		t.Fatalf("pad findings = %+v", pad)
	}
}

func TestNormalizeWriterCorrectnessCheckMapsRoundTripSource(t *testing.T) {
	findings := NormalizeWriterCorrectnessCheck(writercorrectness.CheckResult{
		Name: writercorrectness.CheckKiCadRoundTrip,
		Issues: []reports.Issue{{
			Code:     reports.CodeRoundTripDiff,
			Severity: reports.SeverityError,
			Path:     "demo.kicad_pcb",
			Message:  "round-trip diff",
		}},
	}, "")
	if len(findings) != 1 || findings[0].Source != FindingSourceRoundTrip || findings[0].Category != FindingCategoryRoundTrip {
		t.Fatalf("roundtrip findings = %+v", findings)
	}
}

func TestNormalizeBoardValidationCheckMapsRouteZoneAndDRC(t *testing.T) {
	route := NormalizeBoardValidationCheck(boardvalidation.Check{
		Name: boardvalidation.CheckRouteCompletion,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "nets.GND",
			Message:  "net GND is unrouted",
			Nets:     []string{"GND"},
		}},
	}, "")
	if len(route) != 1 || route[0].Source != FindingSourceBoard || route[0].Category != FindingCategoryRoute || route[0].Subject.Net != "GND" {
		t.Fatalf("route findings = %+v", route)
	}

	zone := NormalizeBoardValidationCheck(boardvalidation.Check{
		Name: boardvalidation.CheckZoneValidation,
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     "zones.GND",
			Message:  "zone has no fill evidence",
			Nets:     []string{"GND"},
		}},
	}, "")
	if len(zone) != 1 || zone[0].Category != FindingCategoryZone || zone[0].Repairability != RepairabilityInformational {
		t.Fatalf("zone findings = %+v", zone)
	}

	drc := NormalizeBoardValidationCheck(boardvalidation.Check{
		Name:     boardvalidation.CheckKiCadDRC,
		Evidence: "artifacts/drc.json",
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "drc",
			Message:  "clearance failure",
		}},
	}, "")
	if len(drc) != 1 || drc[0].Source != FindingSourceKiCadDRC || drc[0].Category != FindingCategoryBoardDRC || drc[0].EvidencePath != "artifacts/drc.json" {
		t.Fatalf("drc findings = %+v", drc)
	}
}

func TestNormalizeBuiltInValidationPreservesMultipleRefsAndNets(t *testing.T) {
	findings := NormalizeBoardValidationCheck(boardvalidation.Check{
		Name: boardvalidation.CheckNetToPadValidation,
		Issues: []reports.Issue{{
			Code:     reports.CodeInvalidNetAssignment,
			Severity: reports.SeverityError,
			Path:     "short",
			Message:  "short between nets",
			Refs:     []string{"U2", "U1"},
			Nets:     []string{"GND", "VCC"},
		}},
	}, "")
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	if findings[0].Subject.Ref != "U1,U2" || findings[0].Subject.Net != "GND,VCC" {
		t.Fatalf("multi-entity subject was not preserved: %+v", findings[0].Subject)
	}
}

func TestNormalizeValidationResultsSortFindings(t *testing.T) {
	result := writercorrectness.Result{Checks: []writercorrectness.CheckResult{
		{
			Name: writercorrectness.CheckPCBNetTable,
			Issues: []reports.Issue{{
				Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Message: "b", Nets: []string{"B"},
			}},
		},
		{
			Name: writercorrectness.CheckPCBNetTable,
			Issues: []reports.Issue{{
				Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Message: "a", Nets: []string{"A"},
			}},
		},
	}}
	findings := NormalizeWriterCorrectnessResult(result, "")
	if len(findings) != 2 {
		t.Fatalf("findings not sorted: %+v", findings)
	}
	for index := 1; index < len(findings); index++ {
		if findings[index-1].Key > findings[index].Key {
			t.Fatalf("findings not sorted by key: %+v", findings)
		}
	}
}
