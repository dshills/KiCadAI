package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

type denseBoardCorrectionCorpus struct {
	Schema string                           `json:"schema"`
	Cases  []denseBoardCorrectionCorpusCase `json:"cases"`
}

type denseBoardCorrectionCorpusCase struct {
	Name          string                                `json:"name"`
	Issue         string                                `json:"issue"`
	AllowVias     *bool                                 `json:"allow_vias,omitempty"`
	ExpectApplied bool                                  `json:"expect_applied"`
	Components    []denseBoardCorrectionCorpusComponent `json:"components"`
	Operations    []denseBoardCorrectionCorpusOperation `json:"operations"`
}

type denseBoardCorrectionCorpusComponent struct {
	Ref   string  `json:"ref"`
	Net   string  `json:"net"`
	XMM   float64 `json:"x_mm"`
	YMM   float64 `json:"y_mm"`
	Layer string  `json:"layer"`
}

type denseBoardCorrectionCorpusOperation struct {
	Net    string       `json:"net"`
	Layer  string       `json:"layer"`
	Points [][2]float64 `json:"points"`
}

type denseBoardCorrectionCorpusRun struct {
	Initial     RoutingStageResult
	Corrected   RoutingStageResult
	Application AutonomousCorrectionApplication
}

func TestDenseBoardCorrectionHeldOutCorpus(t *testing.T) {
	corpus := loadDenseBoardCorrectionCorpus(t)
	if corpus.Schema != "kicadai.dense-board-correction-corpus.v1" || len(corpus.Cases) < 3 {
		t.Fatalf("dense-board corpus = %#v", corpus)
	}
	for _, corpusCase := range corpus.Cases {
		t.Run(corpusCase.Name, func(t *testing.T) {
			first := runDenseBoardCorrectionCorpusCase(t, corpusCase)
			second := runDenseBoardCorrectionCorpusCase(t, corpusCase)
			if first.Application.Applied != corpusCase.ExpectApplied {
				t.Fatalf("application = %#v, want applied=%t", first.Application, corpusCase.ExpectApplied)
			}
			if !reflect.DeepEqual(first.Corrected.Operations, second.Corrected.Operations) || !reflect.DeepEqual(first.Application, second.Application) {
				t.Fatal("held-out correction replay is not byte-identical")
			}
			if corpusCase.ExpectApplied {
				if first.Corrected.Result.Status != routing.StatusRouted || !first.Application.ProtectedInvariantsPreserved {
					t.Fatalf("corrected result = %#v application=%#v", first.Corrected.Result, first.Application)
				}
				affected := make(map[string]struct{}, len(first.Application.AffectedNets))
				for _, net := range first.Application.AffectedNets {
					affected[net] = struct{}{}
				}
				before := autonomousCorrectionPreservedOperations(first.Initial.Operations, affected)
				after := autonomousCorrectionPreservedOperations(first.Corrected.Operations, affected)
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("unaffected operations changed:\nbefore=%#v\nafter=%#v", before, after)
				}
			} else if !reflect.DeepEqual(first.Initial.Operations, first.Corrected.Operations) {
				t.Fatal("fail-closed corpus case mutated operations")
			}

			renamed := renameDenseBoardCorrectionCorpusCase(corpusCase, "heldout_")
			renamedRun := runDenseBoardCorrectionCorpusCase(t, renamed)
			if renamedRun.Application.Applied != first.Application.Applied ||
				renamedRun.Application.ReplacedOperationCount != first.Application.ReplacedOperationCount ||
				renamedRun.Application.PreservedOperationCount != first.Application.PreservedOperationCount {
				t.Fatalf("identity rename changed behavior:\nbase=%#v\nrenamed=%#v", first.Application, renamedRun.Application)
			}
		})
	}
}

func loadDenseBoardCorrectionCorpus(t *testing.T) denseBoardCorrectionCorpus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "dense_board_correction", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus denseBoardCorrectionCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

func runDenseBoardCorrectionCorpusCase(t *testing.T, corpusCase denseBoardCorrectionCorpusCase) denseBoardCorrectionCorpusRun {
	t.Helper()
	routingRequest, operations := denseBoardCorrectionRoutingInput(t, corpusCase)
	var issues []reports.Issue
	switch corpusCase.Issue {
	case "copper_conflict":
		issues = routing.ValidatePhysicalClearance(routingRequest, routingRoutesFromOperations(operations))
	case "missing_transition":
		issues = []reports.Issue{{
			Code: reports.CodeRouteContactLayerMismatch, Severity: reports.SeverityBlocked,
			Path: "operations[0]", Message: "required route layer transition is absent",
			Nets: []string{corpusCase.Operations[0].Net},
		}}
	default:
		t.Fatalf("unsupported corpus issue %q", corpusCase.Issue)
	}
	if len(issues) == 0 || !reports.HasBlockingIssue(issues) {
		t.Fatalf("corpus case %q is not initially blocked: %#v", corpusCase.Name, issues)
	}
	initial := RoutingStageResult{
		Request: routingRequest,
		Result: routing.Result{Status: routing.StatusBlocked, Metrics: routing.Metrics{
			NetCount: len(routingRequest.Nets), FailedNetCount: 1,
		}},
		Operations: operations,
		Stage:      NewStageResult(StageRouting, issues),
	}
	diagnostics := BuildAutonomousCorrectionDiagnosticsForRouting(nil, initial)
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{
		Attempt: 2, MaxAttempts: 3, RouteOperations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized {
		t.Fatalf("corpus plan rejected: %#v", plan)
	}
	corrected, application, err := ApplyAutonomousRoutingCorrectionPlan(context.Background(), request, initial, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	return denseBoardCorrectionCorpusRun{Initial: initial, Corrected: corrected, Application: application}
}

func denseBoardCorrectionRoutingInput(t *testing.T, corpusCase denseBoardCorrectionCorpusCase) (routing.Request, []transactions.Operation) {
	t.Helper()
	allow := true
	if corpusCase.AllowVias != nil {
		allow = *corpusCase.AllowVias
	}
	request := routing.Request{
		Board: routing.Board{
			WidthMM: 20, HeightMM: 20, MarginMM: 1,
			Layers: []routing.Layer{
				{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true},
				{Name: "B.Cu", Kind: routing.LayerCopper, Routable: true},
			},
		},
		Rules: routing.Rules{
			GridMM: 0.5, TraceWidthMM: 0.25, ClearanceMM: 0.25,
			ViaDiameterMM: 0.7, ViaDrillMM: 0.3, ViaClearanceMM: 0.25,
			MaxSearchNodes: 200000, MaxViasPerNet: 2, AllowVias: &allow, AllowBackLayer: &allow,
		},
		Strategy: routing.Strategy{Mode: routing.ModeTwoLayer, NetOrder: routing.NetOrderConstrainedEndpointAccessV1},
	}
	netIndexes := map[string]int{}
	for _, component := range corpusCase.Components {
		request.Components = append(request.Components, correctionRoutingEndpointComponentOnLayer(component.Ref, component.Net, component.XMM, component.YMM, component.Layer))
		index, ok := netIndexes[component.Net]
		if !ok {
			index = len(request.Nets)
			netIndexes[component.Net] = index
			request.Nets = append(request.Nets, routing.Net{Name: component.Net})
		}
		request.Nets[index].Endpoints = append(request.Nets[index].Endpoints, routing.Endpoint{Ref: component.Ref, Pin: "1"})
	}
	slices.SortFunc(request.Nets, func(left, right routing.Net) int {
		if left.Name < right.Name {
			return -1
		}
		if left.Name > right.Name {
			return 1
		}
		return 0
	})
	operations := make([]transactions.Operation, 0, len(corpusCase.Operations))
	for index, operation := range corpusCase.Operations {
		if len(operation.Points) != 2 {
			t.Fatalf("operation %d points = %#v", index, operation.Points)
		}
		operations = append(operations, correctionLayerRouteOperation(t, operation.Net, index, operation.Layer,
			operation.Points[0][0], operation.Points[0][1], operation.Points[1][0], operation.Points[1][1]))
	}
	return request, operations
}

func renameDenseBoardCorrectionCorpusCase(corpusCase denseBoardCorrectionCorpusCase, prefix string) denseBoardCorrectionCorpusCase {
	renamed := corpusCase
	renamed.Name = prefix + corpusCase.Name
	renamed.Components = slices.Clone(corpusCase.Components)
	renamed.Operations = slices.Clone(corpusCase.Operations)
	for index := range renamed.Components {
		renamed.Components[index].Ref = prefix + renamed.Components[index].Ref
		renamed.Components[index].Net = prefix + renamed.Components[index].Net
	}
	for index := range renamed.Operations {
		renamed.Operations[index].Net = prefix + renamed.Operations[index].Net
	}
	return renamed
}
