package placement

import (
	"testing"

	"kicadai/internal/reports"
)

func TestDiagnosticsForQualityReportsRoutingReadiness(t *testing.T) {
	req := minimalRequest()
	result := Place(req)
	quality := BuildQualityReport(req, result)

	if len(quality.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one readiness diagnostic", quality.Diagnostics)
	}
	got := quality.Diagnostics[0]
	if got.Category != PlacementDiagnosticRoutingReadiness || got.Action != PlacementActionProceedToRouting || got.Severity != reports.SeverityInfo {
		t.Fatalf("diagnostic = %#v, want routing readiness info", got)
	}
}

func TestDiagnosticsForQualityReportsEstimatedBounds(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds.Source = BoundsEstimated
	result := Place(req)
	quality := BuildQualityReport(req, result)

	got := findPlacementDiagnostic(quality.Diagnostics, PlacementDiagnosticLibraryGeometry)
	if got == nil {
		t.Fatalf("missing library geometry diagnostic: %#v", quality.Diagnostics)
	}
	if got.Action != PlacementActionAssignCourtyardFootprint || len(got.Refs) != 1 || got.Refs[0] != "R1" {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func TestDiagnosticsForQualityReportsUnplacedComponents(t *testing.T) {
	req := minimalRequest()
	req.Board.WidthMM = 2
	req.Board.HeightMM = 2
	result := Place(req)
	quality := BuildQualityReport(req, result)

	got := findPlacementDiagnostic(quality.Diagnostics, PlacementDiagnosticMissingPlacement)
	if got == nil {
		t.Fatalf("missing unplaced diagnostic: %#v", quality.Diagnostics)
	}
	if got.Severity != reports.SeverityBlocked || got.Action != PlacementActionPlaceMissingComponents {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func TestDiagnosticsForQualityReportsGroupSpread(t *testing.T) {
	req := twoComponentRequest()
	req.Groups = []Group{{ID: "analog", Components: []string{"R1", "R2"}, MaxSpreadMM: 1}}
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[1].Fixed = true
	req.Components[1].Position = &Placement{XMM: 30, YMM: 5, Layer: "F.Cu"}

	result := Place(req)
	quality := BuildQualityReport(req, result)
	got := findPlacementDiagnostic(quality.Diagnostics, PlacementDiagnosticGrouping)
	if got == nil {
		t.Fatalf("missing grouping diagnostic: %#v", quality.Diagnostics)
	}
	if got.Action != PlacementActionMoveGroupTogether {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func TestDiagnosticsForQualityReportsLongWeightedNet(t *testing.T) {
	req := twoComponentRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}
	req.Components[1].Fixed = true
	req.Components[1].Position = &Placement{XMM: 90, YMM: 5, Layer: "F.Cu"}
	req.Board.WidthMM = 100
	req.Board.HeightMM = 20
	req.Nets = []Net{{Name: "VCC", Weight: 5, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}}}

	result := Place(req)
	quality := BuildQualityReport(req, result)
	got := findPlacementDiagnostic(quality.Diagnostics, PlacementDiagnosticNetProximity)
	if got == nil {
		t.Fatalf("missing net proximity diagnostic: %#v", quality.Diagnostics)
	}
	if len(got.Nets) != 1 || got.Nets[0] != "VCC" || len(got.Refs) != 2 {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func findPlacementDiagnostic(diagnostics []PlacementDiagnostic, category PlacementDiagnosticCategory) *PlacementDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Category == category {
			return &diagnostics[i]
		}
	}
	return nil
}
