package designworkflow

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

const fullBoardRetryFixtureRoot = "testdata/full_board_retry"

type fullBoardRetryFixtureMetadata struct {
	Name                  string   `json:"name"`
	Request               string   `json:"request"`
	Intent                string   `json:"intent"`
	ExpectedCategories    []string `json:"expected_categories"`
	ExpectedStopReason    string   `json:"expected_stop_reason"`
	ExpectedImprovement   string   `json:"expected_improvement"`
	PreserveConstraints   []string `json:"preserve_constraints"`
	ExpectedRoutingStatus string   `json:"expected_routing_status"`
	ExpectProjectWrite    bool     `json:"expect_project_write"`
	Determinism           string   `json:"determinism"`
}

type fullBoardRetryEvidence struct {
	Fixture       string
	RoutingStatus routing.Status
	RoutedNets    int
	FailedNets    int
	Retry         placementRoutingRetrySummary
	HasRetry      bool
	Artifacts     int
}

func fullBoardRetryMetadataPath(name string) string {
	return filepath.Join(fullBoardRetryFixtureRoot, name, "metadata.json")
}

func readFullBoardRetryMetadata(name string) (fullBoardRetryFixtureMetadata, error) {
	path := fullBoardRetryMetadataPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return fullBoardRetryFixtureMetadata{}, err
	}
	var metadata fullBoardRetryFixtureMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fullBoardRetryFixtureMetadata{}, err
	}
	return metadata, nil
}

func loadFullBoardRetryMetadata(t *testing.T, name string) fullBoardRetryFixtureMetadata {
	t.Helper()
	metadata, err := readFullBoardRetryMetadata(name)
	if err != nil {
		t.Fatalf("load full-board retry fixture %q from %s: %v", name, fullBoardRetryMetadataPath(name), err)
	}
	if metadata.Name == "" || metadata.Intent == "" || metadata.Determinism == "" {
		t.Fatalf("full-board retry fixture metadata incomplete: %#v", metadata)
	}
	return metadata
}

func fullBoardRetryEvidenceFromWorkflow(t *testing.T, fixture string, result WorkflowResult) fullBoardRetryEvidence {
	t.Helper()
	stage, ok := stageByName(result, StageRouting)
	if !ok {
		t.Fatalf("routing stage missing for fixture %q: %#v", fixture, result.Stages)
	}
	evidence := fullBoardRetryEvidence{
		Fixture:    fixture,
		Artifacts:  len(WorkflowArtifacts(result)),
		HasRetry:   false,
		RoutedNets: intFromStageSummary(stage.Summary, "routed_nets"),
		FailedNets: intFromStageSummary(stage.Summary, "failed_nets"),
	}
	if raw, ok := stage.Summary["status"].(string); ok {
		evidence.RoutingStatus = routing.Status(raw)
	}
	if summary, ok := retrySummaryFromStage(t, stage); ok {
		evidence.Retry = summary
		evidence.HasRetry = true
	}
	return evidence
}

func intFromStageSummary(summary map[string]any, key string) int {
	if summary == nil {
		return 0
	}
	switch value := summary[key].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(math.Round(float64(value)))
	case float64:
		return int(math.Round(value))
	case json.Number:
		parsed, err := strconv.ParseFloat(string(value), 64)
		if err == nil {
			return int(math.Round(parsed))
		}
		return 0
	default:
		return 0
	}
}

func fullBoardRetryConstraintDiff(before, after placement.Request) []string {
	var diffs []string
	beforeFixed := fixedPlacementByRef(before)
	afterFixed := fixedPlacementByRef(after)
	if !reflect.DeepEqual(afterFixed, beforeFixed) {
		diffs = append(diffs, "fixed_refs")
	}
	if !reflect.DeepEqual(keepoutByID(after), keepoutByID(before)) {
		diffs = append(diffs, "keepouts")
	}
	if !reflect.DeepEqual(edgeConstraintByRef(after), edgeConstraintByRef(before)) {
		diffs = append(diffs, "edge_constraints")
	}
	if !reflect.DeepEqual(netEndpointSignatures(after), netEndpointSignatures(before)) {
		diffs = append(diffs, "net_assignments")
	}
	if boardSignature(after.Board) != boardSignature(before.Board) {
		diffs = append(diffs, "board")
	}
	if !reflect.DeepEqual(netClassSignatures(after), netClassSignatures(before)) {
		diffs = append(diffs, "net_classes")
	}
	slices.Sort(diffs)
	return diffs
}

func fixedPlacementByRef(request placement.Request) map[string]placement.Placement {
	fixed := map[string]placement.Placement{}
	for _, component := range request.Components {
		if component.Fixed && component.Position != nil {
			fixed[component.Ref] = *component.Position
		}
	}
	return fixed
}

func edgeConstraintByRef(request placement.Request) map[string]placement.EdgeConstraint {
	edges := map[string]placement.EdgeConstraint{}
	for _, component := range request.Components {
		if component.Edge != "" {
			edges[component.Ref] = component.Edge
		}
	}
	return edges
}

func boardSignature(board placement.BoardPlacementArea) string {
	return fmt.Sprintf("w=%.4f:h=%.4f:ox=%.4f:oy=%.4f:m=%.4f", board.WidthMM, board.HeightMM, board.Origin.XMM, board.Origin.YMM, board.MarginMM)
}

func netEndpointSignatures(request placement.Request) []string {
	var nets []string
	for _, net := range request.Nets {
		endpoints := append([]placement.Endpoint(nil), net.Endpoints...)
		slices.SortFunc(endpoints, func(a, b placement.Endpoint) int {
			if a.Ref != b.Ref {
				return cmp.Compare(a.Ref, b.Ref)
			}
			return cmp.Compare(a.Pin, b.Pin)
		})
		var endpointText []string
		if len(endpoints) > 0 {
			for _, endpoint := range endpoints {
				endpointText = append(endpointText, endpoint.Ref+"."+endpoint.Pin)
			}
		}
		nets = append(nets, net.Name+"="+strings.Join(endpointText, ","))
	}
	slices.Sort(nets)
	return nets
}

func netClassSignatures(request placement.Request) []string {
	var classes []string
	for _, net := range request.Nets {
		classes = append(classes, net.Name+"="+string(net.Role)+"|"+net.WidthClass)
	}
	slices.Sort(classes)
	return classes
}

func keepoutByID(request placement.Request) map[string]placement.Keepout {
	keepouts := map[string]placement.Keepout{}
	for _, keepout := range request.Keepouts {
		keepouts[keepout.ID] = keepout
	}
	return keepouts
}

func fullBoardRetryLocalRouteDiff(before, after PCBFragmentResult) []string {
	if reflect.DeepEqual(localRouteSignatures(before), localRouteSignatures(after)) {
		return nil
	}
	return []string{"local_routes"}
}

func localRouteSignatures(result PCBFragmentResult) []string {
	var signatures []string
	for _, fragment := range result.Fragments {
		for _, route := range fragment.Realization.LocalRoutes {
			signatures = append(signatures, fmt.Sprintf("%s:%s:%s:%s:%s:%.4f",
				fragment.InstanceID,
				route.ID,
				route.NetName,
				route.From.Ref,
				route.To.Ref,
				route.WidthMM,
			))
		}
	}
	slices.Sort(signatures)
	return signatures
}

func TestFullBoardRetryHarnessLoadsMetadata(t *testing.T) {
	metadata := loadFullBoardRetryMetadata(t, "harness")
	if metadata.Name != "harness" || len(metadata.ExpectedCategories) != 1 || metadata.ExpectedCategories[0] != string(PlacementRetryIncreaseSpacing) {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestFullBoardRetryHarnessMissingFixtureReportsPath(t *testing.T) {
	_, err := readFullBoardRetryMetadata("missing")
	if err == nil {
		t.Fatal("missing fixture unexpectedly loaded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing fixture error = %v, want not exist", err)
	}
	if !strings.Contains(filepath.ToSlash(err.Error()), filepath.ToSlash(fullBoardRetryMetadataPath("missing"))) {
		t.Fatalf("missing fixture error %q does not include metadata path", err)
	}
}

func TestFullBoardRetryEvidenceExtractorHandlesMissingRetry(t *testing.T) {
	result := WorkflowResult{Stages: []StageResult{{
		Name: StageRouting,
		Summary: map[string]any{
			"status":      string(routing.StatusBlocked),
			"routed_nets": 1,
			"failed_nets": 2,
		},
	}}}

	evidence := fullBoardRetryEvidenceFromWorkflow(t, "no_retry", result)
	if evidence.HasRetry || evidence.RoutingStatus != routing.StatusBlocked || evidence.RoutedNets != 1 || evidence.FailedNets != 2 {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestFullBoardRetryConstraintDiffReportsTargetedChanges(t *testing.T) {
	before := retryGoldenPlacementRequest()
	after := placement.CloneRequest(before)
	after.Components[0].Position = &placement.Placement{XMM: 8, YMM: 15, Layer: "F.Cu"}
	after.Keepouts = nil
	after.Components[0].Edge = placement.EdgeRight
	after.Nets[0].Endpoints[0].Pin = "9"
	after.Board.WidthMM = 41
	after.Nets[0].WidthClass = "wide"

	diffs := fullBoardRetryConstraintDiff(before, after)
	want := []string{"board", "edge_constraints", "fixed_refs", "keepouts", "net_assignments", "net_classes"}
	if !slices.Equal(diffs, want) {
		t.Fatalf("diffs = %#v, want %#v", diffs, want)
	}
}

func TestFullBoardRetryLocalRouteDiffReportsRouteChanges(t *testing.T) {
	before := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "status",
		Realization: blocks.BlockPCBRealizationResult{LocalRoutes: []blocks.RealizedPCBLocalRoute{{
			ID:      "series",
			NetName: "LED",
			From:    transactions.Endpoint{Ref: "R1"},
			To:      transactions.Endpoint{Ref: "D1"},
			WidthMM: 0.25,
		}}},
	}}}
	after := before
	after.Fragments = append([]BlockFragment(nil), before.Fragments...)
	after.Fragments[0].Realization.LocalRoutes = append([]blocks.RealizedPCBLocalRoute(nil), before.Fragments[0].Realization.LocalRoutes...)
	if diffs := fullBoardRetryLocalRouteDiff(before, after); len(diffs) != 0 {
		t.Fatalf("unchanged local routes diffed: %#v", diffs)
	}
	after.Fragments[0].Realization.LocalRoutes[0].To.Ref = "D2"
	if diffs := fullBoardRetryLocalRouteDiff(before, after); !slices.Equal(diffs, []string{"local_routes"}) {
		t.Fatalf("diffs = %#v", diffs)
	}
}
