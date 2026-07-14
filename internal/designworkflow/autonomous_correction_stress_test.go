package designworkflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

const autonomousCorrectionStressFixtureDir = "testdata/generic_autonomous_correction"

type autonomousCorrectionStressMetadata struct {
	Schema                  string  `json:"schema"`
	SourceFixture           string  `json:"source_fixture"`
	Perturbation            string  `json:"perturbation"`
	OffsetMM                float64 `json:"offset_mm"`
	FailureCategory         string  `json:"failure_category"`
	ExpectedAction          string  `json:"expected_action"`
	ExpectedStopReason      string  `json:"expected_stop_reason"`
	ExpectedSelectedAttempt int     `json:"expected_selected_attempt"`
	MaxAttempts             int     `json:"max_attempts"`
	IdentityNeutral         bool    `json:"identity_neutral"`
}

type autonomousCorrectionStressRun struct {
	Initial        RoutingStageResult
	SelectedPlaced PlacementStageResult
	SelectedRouted RoutingStageResult
	Summary        placementRoutingRetrySummary
	Report         AutonomousCorrectionReport
}

func TestAutonomousCorrectionStressFixtureRecoversRealRoutingFailure(t *testing.T) {
	metadata := loadAutonomousCorrectionStressMetadata(t)
	request := loadAutonomousCorrectionStressRequest(t)
	if metadata.SourceFixture != "generic_parallel_resistors" || metadata.Perturbation != "r2_offset_from_r1" || metadata.OffsetMM <= 0 || !metadata.IdentityNeutral {
		t.Fatalf("stress metadata = %#v", metadata)
	}
	if !IsGenericAutonomousCorrectionRequest(request) || request.RoutingRetry.MaxAttempts != metadata.MaxAttempts {
		t.Fatalf("stress request correction scope/policy = %#v", request.RoutingRetry)
	}
	beforeRequest := canonicalStressJSON(t, request)
	beforeInvariant, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	index := autonomousCorrectionStressLibraryIndex(request)
	first := runAutonomousCorrectionStress(t, request, index, metadata.OffsetMM)
	second := runAutonomousCorrectionStress(t, request, index, metadata.OffsetMM)

	if first.Initial.Result.Status != routing.StatusPartial || first.Initial.Result.Metrics.FailedNetCount != 2 {
		t.Fatalf("initial perturbation was not a real two-net routing failure: status=%s metrics=%#v issues=%#v", first.Initial.Result.Status, first.Initial.Result.Metrics, first.Initial.Stage.Issues)
	}
	if first.SelectedRouted.Result.Status != routing.StatusRouted || first.SelectedRouted.Result.Metrics.RoutedNetCount != 2 || first.SelectedRouted.Result.Metrics.FailedNetCount != 0 || workflowStageBlocked(first.SelectedRouted.Stage) {
		t.Fatalf("corrected routing did not complete: status=%s metrics=%#v issues=%#v", first.SelectedRouted.Result.Status, first.SelectedRouted.Result.Metrics, first.SelectedRouted.Stage.Issues)
	}
	if first.Summary.StopReason != metadata.ExpectedStopReason || first.Summary.SelectedAttempt != metadata.ExpectedSelectedAttempt || first.Summary.Attempts != 2 || first.Summary.Applied != 1 {
		t.Fatalf("correction summary = %#v", first.Summary)
	}
	if first.Report.StopReason != metadata.ExpectedStopReason || first.Report.SelectedAttempt != metadata.ExpectedSelectedAttempt || first.Report.PlanEvaluations != 1 || len(first.Report.AttemptHistory) != 2 {
		t.Fatalf("correction report = %#v", first.Report)
	}
	plan := first.Report.AttemptHistory[1].Plan
	application := first.Report.AttemptHistory[1].Application
	if plan == nil || !plan.Authorized || len(plan.Diagnostics) < 4 || len(plan.Actions) != 2 || application == nil || !application.Applied {
		t.Fatalf("correction plan/application evidence = plan %#v application %#v", plan, application)
	}
	for _, action := range plan.Actions {
		if string(action.Kind) != metadata.ExpectedAction || action.PlacementHint != PlacementRetryIncreaseSpacing {
			t.Fatalf("correction action = %#v, want %s", action, metadata.ExpectedAction)
		}
	}
	if first.Report.InitialInvariantFingerprint != beforeInvariant || first.Report.FinalInvariantFingerprint != beforeInvariant || !first.Report.ProtectedInvariantsPreserved || !first.Report.AllAttemptInvariantsPreserved {
		t.Fatalf("invariant evidence = %#v, want %s", first.Report, beforeInvariant)
	}
	if afterRequest := canonicalStressJSON(t, request); !bytes.Equal(beforeRequest, afterRequest) {
		t.Fatal("stress correction mutated the generic design request")
	}
	if !reflect.DeepEqual(first.SelectedPlaced.Result.Placements, second.SelectedPlaced.Result.Placements) || !reflect.DeepEqual(first.SelectedRouted.Operations, second.SelectedRouted.Operations) || !bytes.Equal(canonicalStressJSON(t, first.Report), canonicalStressJSON(t, second.Report)) {
		t.Fatal("repeated stress runs produced different placement, routing, or correction evidence")
	}

	renamed := request
	renamed.Name = "identity_neutral_alias"
	renamedRun := runAutonomousCorrectionStress(t, renamed, index, metadata.OffsetMM)
	if renamedRun.Report.StopReason != metadata.ExpectedStopReason || renamedRun.Report.SelectedAttempt != metadata.ExpectedSelectedAttempt || renamedRun.Report.InitialInvariantFingerprint != beforeInvariant {
		t.Fatalf("project identity changed correction behavior: %#v", renamedRun.Report)
	}
}

func runAutonomousCorrectionStress(t *testing.T, request Request, index *libraryresolver.LibraryIndex, offsetMM float64) autonomousCorrectionStressRun {
	t.Helper()
	ctx := correctionLoopTestContext(t)
	placed := PlaceExplicitCircuit(ctx, request, PlacementOptions{LibraryIndex: index})
	if workflowStageBlocked(placed.Stage) {
		t.Fatalf("stress placement issues = %#v", placed.Stage.Issues)
	}
	perturbed := placed
	perturbed.Result.Placements = append([]placement.PlacementResult(nil), placed.Result.Placements...)
	var origin placement.Placement
	foundOrigin := false
	foundTarget := false
	for _, item := range perturbed.Result.Placements {
		if item.Ref == "R1" {
			origin = item.Position
			foundOrigin = true
			break
		}
	}
	for placementIndex := range perturbed.Result.Placements {
		if perturbed.Result.Placements[placementIndex].Ref == "R2" {
			perturbed.Result.Placements[placementIndex].Position = origin
			perturbed.Result.Placements[placementIndex].Position.XMM += offsetMM
			foundTarget = true
			break
		}
	}
	if !foundOrigin || !foundTarget {
		t.Fatalf("stress fixture placements = %#v", perturbed.Result.Placements)
	}
	initial := RouteExplicitCircuit(ctx, request, perturbed, RoutingOptions{})
	selectedPlaced, selectedRouted, summary := maybeRetryExplicitPlacementRouting(ctx, request, perturbed, initial, RoutingOptions{}, request.RoutingRetry)
	report := correctionReportFromRoutingStage(t, selectedRouted)
	return autonomousCorrectionStressRun{Initial: initial, SelectedPlaced: selectedPlaced, SelectedRouted: selectedRouted, Summary: summary, Report: *report}
}

func loadAutonomousCorrectionStressMetadata(t *testing.T) autonomousCorrectionStressMetadata {
	t.Helper()
	file, err := os.Open(filepath.Join(autonomousCorrectionStressFixtureDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var metadata autonomousCorrectionStressMetadata
	if err := decoder.Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if decoder.Decode(&struct{}{}) == nil || metadata.Schema != "kicadai.autonomous-correction-stress.v1" {
		t.Fatalf("invalid stress metadata = %#v", metadata)
	}
	return metadata
}

func loadAutonomousCorrectionStressRequest(t *testing.T) Request {
	t.Helper()
	file, err := os.Open(filepath.Join(autonomousCorrectionStressFixtureDir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	request, issues := DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("stress request issues = %#v", issues)
	}
	return request
}

func autonomousCorrectionStressLibraryIndex(request Request) *libraryresolver.LibraryIndex {
	index := &libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{}}
	for _, component := range request.ExplicitCircuit.Components {
		index.Footprints[component.FootprintID] = placementTestFootprint(component.FootprintID)
	}
	return index
}

func canonicalStressJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
