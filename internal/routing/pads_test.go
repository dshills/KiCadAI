package routing

import (
	"testing"

	"kicadai/internal/reports"
)

func TestBuildPadAccessNormalizesEndpointLookup(t *testing.T) {
	request := minimalRequest()

	access := BuildPadAccess(request)
	points, ok := AccessPointsForEndpoint(access, Endpoint{Ref: " j1 ", Pin: " 1 "})
	if !ok {
		t.Fatalf("missing access point")
	}
	if len(points) != 2 {
		t.Fatalf("access points = %d, want through-hole access on both layers", len(points))
	}
}

func TestBuildPadAccessUsesSMDLayerOnly(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Type = PadSMD
	request.Components[0].Pads[0].Layers = []string{"F.Cu"}

	access := BuildPadAccess(request)
	points, ok := AccessPointsForEndpoint(access, Endpoint{Ref: "J1", Pin: "1"})
	if !ok {
		t.Fatal("missing access point")
	}
	if len(points) != 1 || points[0].Layer != "F.CU" {
		t.Fatalf("points = %#v, want one F.Cu point", points)
	}
}

func TestBuildPadAccessExpandsThroughHoleToAllCopperLayers(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Layers = []string{"F.Cu"}

	access := BuildPadAccess(request)
	points, ok := AccessPointsForEndpoint(access, Endpoint{Ref: "J1", Pin: "1"})
	if !ok {
		t.Fatal("missing access point")
	}
	if len(points) != 2 || points[0].Layer != "F.CU" || points[1].Layer != "B.CU" {
		t.Fatalf("points = %#v, want board-order F.Cu/B.Cu access", points)
	}
}

func TestBuildPadAccessWarnsForUnsupportedShape(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Shape = PadShape("trapezoid")

	access := BuildPadAccess(request)
	assertIssueCode(t, access.Issues, reports.CodeUnsupportedOperation)
}

func TestBuildPadAccessNormalizesSupportedShapeCasing(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Shape = PadShape("RECT")

	access := BuildPadAccess(request)
	for _, issue := range access.Issues {
		if issue.Code == reports.CodeUnsupportedOperation {
			t.Fatalf("unexpected unsupported shape issue: %#v", access.Issues)
		}
	}
}

func TestBuildPadAccessBlocksPadWithoutRoutableLayer(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads[0].Type = PadSMD
	request.Components[0].Pads[0].Layers = []string{"F.Mask"}

	access := BuildPadAccess(request)
	assertIssuePath(t, access.Issues, "components[J1].pads[1].layers")
}

func TestBuildPadAccessReportsDuplicateEndpoint(t *testing.T) {
	request := minimalRequest()
	request.Components[0].Pads = append(request.Components[0].Pads, request.Components[0].Pads[0])

	access := BuildPadAccess(request)
	assertIssuePath(t, access.Issues, "components[J1].pads[1]")
}
