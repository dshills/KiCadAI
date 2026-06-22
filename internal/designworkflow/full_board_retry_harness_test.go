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
	Name                    string   `json:"name"`
	Request                 string   `json:"request"`
	Intent                  string   `json:"intent"`
	FixtureClass            string   `json:"fixture_class"`
	ExpectedCategories      []string `json:"expected_categories"`
	ExpectedStopReason      string   `json:"expected_stop_reason"`
	ExpectedImprovement     string   `json:"expected_improvement"`
	ExpectedImprovedMetric  string   `json:"expected_improved_metric"`
	PreserveConstraints     []string `json:"preserve_constraints"`
	PreserveFixedRefs       []string `json:"preserve_fixed_refs"`
	ExpectedRoutingStatus   string   `json:"expected_routing_status"`
	ExpectedBaselineStatus  string   `json:"expected_baseline_status"`
	ExpectedFinalStatus     string   `json:"expected_final_status"`
	ExpectedMinAttempts     int      `json:"expected_min_attempts"`
	ExpectedMinApplied      int      `json:"expected_min_applied"`
	ExpectPadHydration      bool     `json:"expect_pad_hydration"`
	ExpectedMinHydratedPads int      `json:"expected_min_hydrated_pads"`
	ExpectProjectWrite      bool     `json:"expect_project_write"`
	Determinism             string   `json:"determinism"`
}

type fullBoardRetryEvidence struct {
	Fixture               string
	RoutingStatus         routing.Status
	BaselineRoutingStatus routing.Status
	FinalRoutingStatus    routing.Status
	RoutedNets            int
	FailedNets            int
	BaselineRoutedNets    int
	BaselineFailedNets    int
	FinalRoutedNets       int
	FinalFailedNets       int
	BlockingIssues        int
	PadHydration          PadHydrationSummary
	HasPadHydration       bool
	Retry                 placementRoutingRetrySummary
	HasRetry              bool
	Artifacts             int
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
	if metadata.Name == "" || metadata.Intent == "" || metadata.Determinism == "" || metadata.FixtureClass == "" {
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
	if status, ok := fullBoardRetryRoutingStatusFromAny(stage.Summary["status"]); ok {
		evidence.RoutingStatus = status
	}
	evidence.BaselineRoutingStatus = evidence.RoutingStatus
	evidence.BaselineRoutedNets = evidence.RoutedNets
	evidence.BaselineFailedNets = evidence.FailedNets
	evidence.FinalRoutingStatus = evidence.RoutingStatus
	evidence.FinalRoutedNets = evidence.RoutedNets
	evidence.FinalFailedNets = evidence.FailedNets
	if summary, ok := retrySummaryFromStage(t, stage); ok {
		evidence.Retry = summary
		evidence.HasRetry = true
		evidence.BaselineRoutingStatus, evidence.BaselineRoutedNets, evidence.BaselineFailedNets = fullBoardRetryBaselineFromSummary(summary, evidence.RoutingStatus, evidence.RoutedNets, evidence.FailedNets)
	}
	evidence.BlockingIssues = fullBoardRetryBlockingIssueCount(result.Stages)
	if placementStage, ok := stageByName(result, StagePlacement); ok {
		if padSummary, ok := fullBoardRetryPadHydrationSummary(placementStage); ok {
			evidence.PadHydration = padSummary
			evidence.HasPadHydration = true
		}
	}
	return evidence
}

func fullBoardRetryBaselineFromSummary(summary placementRoutingRetrySummary, fallbackStatus routing.Status, fallbackRouted, fallbackFailed int) (routing.Status, int, int) {
	if len(summary.AttemptHistory) == 0 {
		return fallbackStatus, fallbackRouted, fallbackFailed
	}
	first := summary.AttemptHistory[0]
	status, _ := fullBoardRetryRoutingStatusFromAny(first["baseline_routing_status"])
	if status == "" {
		status = fallbackStatus
	}
	return status, intFromStageSummary(first, "baseline_routed_nets"), intFromStageSummary(first, "baseline_failed_nets")
}

func fullBoardRetryRoutingStatusFromAny(value any) (routing.Status, bool) {
	switch status := value.(type) {
	case routing.Status:
		return status, true
	case string:
		return routing.Status(status), true
	default:
		return "", false
	}
}

func fullBoardRetryBlockingIssueCount(stages []StageResult) int {
	seen := map[string]struct{}{}
	for _, stage := range stages {
		for _, issue := range stage.Issues {
			if issue.Blocking() {
				key := string(issue.Code) + "\x00" + string(issue.Severity) + "\x00" + issue.Path + "\x00" + issue.Message
				seen[key] = struct{}{}
			}
		}
	}
	return len(seen)
}

func fullBoardRetryPadHydrationSummary(stage StageResult) (PadHydrationSummary, bool) {
	if stage.Summary == nil {
		return PadHydrationSummary{}, false
	}
	raw, ok := stage.Summary["pad_hydration"]
	if !ok {
		return PadHydrationSummary{}, false
	}
	switch summary := raw.(type) {
	case PadHydrationSummary:
		return summary, true
	case map[string]any:
		return fullBoardRetryPadHydrationSummaryFromMap(summary), true
	default:
		return PadHydrationSummary{}, false
	}
}

func fullBoardRetryPadHydrationSummaryFromMap(raw map[string]any) PadHydrationSummary {
	summary := PadHydrationSummary{
		ComponentCount:     intFromStageSummary(raw, "component_count"),
		HydratedComponents: intFromStageSummary(raw, "hydrated_components"),
		MissingComponents:  intFromStageSummary(raw, "missing_components"),
		PadCount:           intFromStageSummary(raw, "pad_count"),
		BlockingIssues:     intFromStageSummary(raw, "blocking_issues"),
	}
	if values, ok := raw["missing_refs"].([]any); ok {
		for _, value := range values {
			if ref, ok := value.(string); ok {
				summary.MissingRefs = append(summary.MissingRefs, ref)
			}
		}
	}
	if sourceCounts, ok := raw["source_counts"].(map[string]any); ok {
		summary.SourceCounts = map[PadHydrationSource]int{}
		for source, count := range sourceCounts {
			summary.SourceCounts[PadHydrationSource(source)] = intFromAny(count)
		}
	}
	return summary
}

func intFromAny(value any) int {
	switch parsed := value.(type) {
	case nil:
		return 0
	case int:
		return parsed
	case int8:
		return int(parsed)
	case int16:
		return int(parsed)
	case int32:
		return int(parsed)
	case int64:
		return intValueToInt(parsed)
	case uint:
		return intFromUint64(uint64(parsed))
	case uint8:
		return int(parsed)
	case uint16:
		return int(parsed)
	case uint32:
		return intFromUint64(uint64(parsed))
	case uint64:
		return intFromUint64(parsed)
	case float32:
		return intFromFloat64(float64(parsed))
	case float64:
		return intFromFloat64(parsed)
	case json.Number:
		if intValue, err := parsed.Int64(); err == nil {
			return intValueToInt(intValue)
		}
		if floatValue, err := strconv.ParseFloat(string(parsed), 64); err == nil {
			return intFromFloat64(floatValue)
		}
	}
	panic(fmt.Sprintf("unsupported numeric type %T", value))
}

func intFromFloat64(value float64) int {
	rounded := math.Round(value)
	maxInt := float64(int(^uint(0) >> 1))
	minInt := -maxInt - 1
	if rounded > maxInt || rounded < minInt {
		panic(fmt.Sprintf("float %f overflows int", value))
	}
	return int(rounded)
}

func intValueToInt(value int64) int {
	converted := int(value)
	if int64(converted) != value {
		panic(fmt.Sprintf("int64 %d overflows int", value))
	}
	return converted
}

func intFromUint64(value uint64) int {
	maxInt := uint64(^uint(0) >> 1)
	if value > maxInt {
		panic(fmt.Sprintf("uint64 %d overflows int", value))
	}
	return int(value)
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
	if metadata.FixtureClass != "harness" || metadata.ExpectedImprovedMetric != "none" || metadata.ExpectedMinAttempts != 1 {
		t.Fatalf("extended metadata = %#v", metadata)
	}
}

func TestFullBoardRetryFixtureMetadataDeclaresEvidenceContract(t *testing.T) {
	for _, name := range fullBoardRetryFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			metadata := loadFullBoardRetryMetadata(t, name)
			if metadata.Request == "" || metadata.ExpectedRoutingStatus == "" || metadata.ExpectedImprovement == "" || metadata.ExpectedImprovedMetric == "" {
				t.Fatalf("metadata missing evidence contract fields: %#v", metadata)
			}
			if metadata.ExpectedRoutingStatus == "skipped" && metadata.ExpectedMinAttempts != 0 {
				t.Fatalf("expected_min_attempts = %d, want 0 for skipped routing", metadata.ExpectedMinAttempts)
			}
			if metadata.ExpectedRoutingStatus != "skipped" && metadata.ExpectedMinAttempts < 1 {
				t.Fatalf("expected_min_attempts = %d, want >= 1", metadata.ExpectedMinAttempts)
			}
			if metadata.ExpectPadHydration && metadata.ExpectedMinHydratedPads < 1 {
				t.Fatalf("pad hydration expectation missing minimum hydrated pads: %#v", metadata)
			}
		})
	}
}

func fullBoardRetryFixtureNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fullBoardRetryFixtureRoot)
	if err != nil {
		t.Fatalf("read full-board retry fixture root: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names
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

func TestFullBoardRetryEvidenceExtractorReadsRetryAndPadHydration(t *testing.T) {
	result := WorkflowResult{Stages: []StageResult{
		{
			Name: StagePlacement,
			Summary: map[string]any{
				"pad_hydration": PadHydrationSummary{
					ComponentCount:     2,
					HydratedComponents: 2,
					PadCount:           4,
					SourceCounts:       map[PadHydrationSource]int{PadHydrationSourceVerifiedTemplate: 2},
				},
			},
		},
		{
			Name: StageRouting,
			Summary: map[string]any{
				"status":      string(routing.StatusRouted),
				"routed_nets": 2,
				"failed_nets": 0,
				"routing_retry": placementRoutingRetrySummary{
					Enabled:    true,
					Attempts:   2,
					Applied:    1,
					StopReason: "routed",
					AttemptHistory: []map[string]any{{
						"attempt":                 2,
						"baseline_routing_status": string(routing.StatusBlocked),
						"baseline_routed_nets":    0,
						"baseline_failed_nets":    2,
						"routing_status":          string(routing.StatusRouted),
						"routed_nets":             2,
						"failed_nets":             0,
					}},
				},
			},
		},
	}}

	evidence := fullBoardRetryEvidenceFromWorkflow(t, "with_retry", result)
	if !evidence.HasRetry || evidence.Retry.Attempts != 2 || evidence.BaselineRoutingStatus != routing.StatusBlocked || evidence.FinalRoutingStatus != routing.StatusRouted {
		t.Fatalf("retry evidence = %#v", evidence)
	}
	if !evidence.HasPadHydration || evidence.PadHydration.HydratedComponents != 2 || evidence.PadHydration.PadCount != 4 {
		t.Fatalf("pad hydration evidence = %#v", evidence)
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
