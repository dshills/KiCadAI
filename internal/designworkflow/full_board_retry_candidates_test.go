package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestFullBoardRetryCandidateFixturesDecode(t *testing.T) {
	for _, name := range []string{"spacing_improves", "distance_rules", "generated_led_rejected"} {
		t.Run(name, func(t *testing.T) {
			metadata := loadFullBoardRetryMetadata(t, name)
			if metadata.Request == "" {
				t.Fatalf("metadata missing request: %#v", metadata)
			}
			file, err := os.Open(filepath.Join(fullBoardRetryFixtureRoot, name, metadata.Request))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			if validationIssues := ValidateRequest(request); len(validationIssues) != 0 {
				t.Fatalf("validation issues = %#v", validationIssues)
			}
		})
	}
}

func TestFullBoardRetryCandidateBaselinesAreDeterministic(t *testing.T) {
	for _, name := range []string{"spacing_improves", "distance_rules"} {
		t.Run(name, func(t *testing.T) {
			first := runFullBoardRetrySeedBaseline(t, name)
			second := runFullBoardRetrySeedBaseline(t, name)
			if first.Result.Status != second.Result.Status ||
				first.Result.Metrics.RoutedNetCount != second.Result.Metrics.RoutedNetCount ||
				first.Result.Metrics.FailedNetCount != second.Result.Metrics.FailedNetCount {
				t.Fatalf("non-deterministic baseline: first=%#v second=%#v", first.Result.Metrics, second.Result.Metrics)
			}
		})
	}
}

func TestFullBoardRetryCandidateWorkflowReachesRoutingConnectivity(t *testing.T) {
	for _, name := range []string{"spacing_improves", "distance_rules", "generated_led_rejected"} {
		t.Run(name, func(t *testing.T) {
			result := runFullBoardRetryFixture(t, name)
			stage, ok := stageByName(result, StageRouting)
			if !ok {
				t.Fatalf("routing stage missing: %#v", result.Stages)
			}
			metadata := loadFullBoardRetryMetadata(t, name)
			if name == "generated_led_rejected" && metadata.ExpectedImprovement != "generated_routing_connectivity" {
				t.Fatalf("metadata = %#v", metadata)
			}
			if stage.Status != StageStatusBlocked {
				t.Fatalf("candidate workflow stage = %#v, want current generated routing blocker documented", stage)
			}
			assertIssueCode(t, stage.Issues, reports.CodeDisconnectedPad)
		})
	}
}

func runFullBoardRetryFixture(t *testing.T, name string) WorkflowResult {
	t.Helper()
	metadata := loadFullBoardRetryMetadata(t, name)
	file, err := os.Open(filepath.Join(fullBoardRetryFixtureRoot, name, metadata.Request))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return Create(ctx, request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), name)})
}

func runFullBoardRetrySeedBaseline(t *testing.T, name string) RoutingStageResult {
	t.Helper()
	placed := fullBoardRetrySeedPlacement(t, name)
	request := fullBoardRetrySeedRequest(t, name)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return RoutePlacement(ctx, request, PCBFragmentResult{}, placed, RoutingOptions{Mode: routing.ModeSingleLayer})
}

func fullBoardRetrySeedRequest(t *testing.T, name string) Request {
	t.Helper()
	request := loadFullBoardRetryRequestForTest(t, name)
	return Request{
		Version:    RequestVersion,
		Name:       name,
		Board:      request.Board,
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, StrictUnrouted: true},
	}
}

func fullBoardRetrySeedPlacement(t *testing.T, name string) PlacementStageResult {
	t.Helper()
	req := fullBoardRetrySeedPlacementRequest(t, name)
	result := placement.Place(req)
	stage := NewStageResult(StagePlacement, result.Issues)
	if result.Status != placement.StatusPlaced {
		t.Fatalf("seed placement %s failed: %#v", name, result)
	}
	return PlacementStageResult{Request: req, Result: result, Stage: stage}
}

func fullBoardRetrySeedPlacementRequest(t *testing.T, name string) placement.Request {
	t.Helper()
	requestSpec := loadFullBoardRetryRequestForTest(t, name)
	request := placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: requestSpec.Board.WidthMM, HeightMM: requestSpec.Board.HeightMM, MarginMM: 1},
		Rules: placement.Rules{
			GridMM:                   0.5,
			ComponentSpacingMM:       0.5,
			GroupSpacingMM:           0.5,
			BoardEdgeClearanceMM:     1,
			ConnectorEdgeClearanceMM: 1,
			MaxCandidatesPerPart:     500,
		},
		Components: []placement.Component{
			fullBoardRetryPadComponent("J1", 4, 6, true, placement.EdgeLeft, &placement.Placement{XMM: 3, YMM: 15, Layer: "F.Cu"}),
			fullBoardRetryPadComponent("U1", 5, 5, false, "", nil),
			fullBoardRetryPadComponent("R1", 3, 2, false, "", nil),
			fullBoardRetryPadComponent("D1", 3, 2, false, "", nil),
		},
		Nets: []placement.Net{
			{Name: "VIN", Role: placement.NetPower, WidthClass: "power", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "1"}}},
			{Name: "SIG", Role: placement.NetSignal, WidthClass: "signal", Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "2"}, {Ref: "R1", Pin: "1"}, {Ref: "D1", Pin: "1"}}},
			{Name: "GND", Role: placement.NetPower, WidthClass: "power", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "2"}, {Ref: "U1", Pin: "3"}, {Ref: "D1", Pin: "2"}}},
		},
		Keepouts: []placement.Keepout{{
			ID:     "mounting_keepout",
			Bounds: placement.Rect{Min: placement.Point{XMM: 21, YMM: 12}, Max: placement.Point{XMM: 25, YMM: 18}},
			Layers: []string{"F.Cu"},
			Reason: "mechanical clearance",
		}},
		Seed: "full-board-retry-seed",
	}
	if name == "distance_rules" {
		for index := range request.Nets {
			if request.Nets[index].Name == "SIG" {
				request.Nets[index].Weight = 5
			}
		}
	}
	hydrateFullBoardRetryPadNets(&request)
	return placement.NormalizeRequest(request)
}

func fullBoardRetryPadComponent(ref string, width, height float64, fixed bool, edge placement.EdgeConstraint, position *placement.Placement) placement.Component {
	return placement.Component{
		Ref:         ref,
		FootprintID: "Test:Pad",
		Bounds:      placement.Bounds{WidthMM: width, HeightMM: height, Source: placement.BoundsExplicit},
		Fixed:       fixed,
		Position:    position,
		Edge:        edge,
		Pads: []placement.PadSummary{
			{Name: "1", Net: "", XMM: -0.5, YMM: 0, WidthMM: 0.8, HeightMM: 0.8},
			{Name: "2", Net: "", XMM: 0.5, YMM: 0, WidthMM: 0.8, HeightMM: 0.8},
			{Name: "3", Net: "", XMM: 0, YMM: 0.5, WidthMM: 0.8, HeightMM: 0.8},
		},
	}
}

func loadFullBoardRetryRequestForTest(t *testing.T, name string) Request {
	t.Helper()
	metadata, err := readFullBoardRetryMetadata(name)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fullBoardRetryFixtureRoot, name, metadata.Request)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if len(issues) != 0 {
		t.Fatalf("decode issues = %#v", issues)
	}
	return request
}

func hydrateFullBoardRetryPadNets(request *placement.Request) {
	netByEndpoint := map[string]string{}
	for _, net := range request.Nets {
		for _, endpoint := range net.Endpoints {
			netByEndpoint[endpoint.Ref+"."+endpoint.Pin] = net.Name
		}
	}
	for componentIndex := range request.Components {
		component := &request.Components[componentIndex]
		for padIndex := range component.Pads {
			key := component.Ref + "." + component.Pads[padIndex].Name
			component.Pads[padIndex].Net = netByEndpoint[key]
		}
	}
}
