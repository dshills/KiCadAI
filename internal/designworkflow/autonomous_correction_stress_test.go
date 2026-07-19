package designworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/writercorrectness"
)

const (
	autonomousCorrectionStressFixtureDir = "testdata/generic_autonomous_correction"
	resistor0805PadOffsetNM              = 912_500
	resistor0805PadWidthNM               = 1_025_000
	resistor0805PadHeightNM              = 1_400_000
)

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
	index := autonomousCorrectionStressLibraryIndex(t, request)
	first := runAutonomousCorrectionStress(t, request, index, metadata.OffsetMM)
	second := runAutonomousCorrectionStress(t, request, index, metadata.OffsetMM)

	if first.Initial.Result.Metrics.NetCount != 2 || (first.Initial.Result.Status == routing.StatusRouted && !workflowStageBlocked(first.Initial.Stage)) || !reports.HasBlockingIssue(first.Initial.Stage.Issues) {
		t.Fatalf("initial perturbation was not a real routing failure for the two-net design: status=%s metrics=%#v issues=%#v", first.Initial.Result.Status, first.Initial.Result.Metrics, first.Initial.Stage.Issues)
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
	if plan == nil || !plan.Authorized || len(plan.Diagnostics) == 0 || len(plan.Actions) == 0 || application == nil || !application.Applied {
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

func TestAutonomousCorrectionStressOptionalKiCad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping optional KiCad-backed autonomous correction stress lane in short mode")
	}
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	if cliPath == "" {
		t.Skipf("set %s to run the autonomous correction KiCad-backed stress lane", checks.EnvKiCadCLI)
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if strings.TrimSpace(roots.SymbolsRoot) == "" || strings.TrimSpace(roots.FootprintsRoot) == "" {
		t.Skip("autonomous correction KiCad stress lane requires symbol and footprint roots")
	}
	if reports.HasBlockingIssue(rootIssues) {
		t.Fatalf("library root issues = %#v", rootIssues)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	fullIndex, loadIssues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{})
	if err := ctx.Err(); err != nil {
		t.Fatalf("load stress libraries: %v", err)
	}
	if len(fullIndex.Symbols) == 0 || len(fullIndex.Footprints) == 0 {
		t.Fatalf("stress library index is empty; issues = %#v", loadIssues)
	}
	// The complete upstream libraries may report unrelated diagnostics. Resolve
	// every record required by this fixture explicitly and fail if any is absent.
	request := loadAutonomousCorrectionStressRequest(t)
	symbols := make(map[string]libraryresolver.SymbolRecord)
	for _, component := range request.ExplicitCircuit.Schematic.Circuit.Components {
		record, ok := fullIndex.Symbols[component.Symbol]
		if !ok {
			t.Fatalf("stress symbol library record missing: %s", component.Symbol)
		}
		symbols[component.Symbol] = record
	}
	footprints := make(map[string]libraryresolver.FootprintRecord)
	for _, component := range request.ExplicitCircuit.Components {
		record, ok := fullIndex.Footprints[component.FootprintID]
		if !ok {
			t.Fatalf("stress footprint library record missing: %s", component.FootprintID)
		}
		footprints[component.FootprintID] = record
	}
	index := libraryresolver.LibraryIndex{
		GeneratedAt: fullIndex.GeneratedAt,
		Roots:       fullIndex.Roots,
		Symbols:     symbols,
		Footprints:  footprints,
	}
	metadata := loadAutonomousCorrectionStressMetadata(t)
	stress := runAutonomousCorrectionStress(t, request, &index, metadata.OffsetMM)
	if stress.SelectedRouted.Result.Status != routing.StatusRouted || stress.Report.SelectedAttempt != metadata.ExpectedSelectedAttempt {
		t.Fatalf("stress correction did not select routed attempt: %#v", stress.Report)
	}

	schematicTx, schematicIssues := explicitSchematicTransaction(request, &index)
	if reports.HasBlockingIssue(schematicIssues) {
		t.Fatalf("stress schematic transaction issues = %#v", schematicIssues)
	}
	tx, transactionIssues := explicitCircuitTransaction(request, schematicTx, stress.SelectedPlaced, stress.SelectedRouted, true, nil, &index)
	if reports.HasBlockingIssue(transactionIssues) {
		t.Fatalf("stress project transaction issues = %#v", transactionIssues)
	}
	// KiCad on macOS requires a stable existing cwd; the ignored examples workspace
	// avoids subprocess failures observed when Go removes temporary directories.
	generatedRoot := filepath.Clean(filepath.Join("..", "..", "examples", ".generated"))
	if err := os.MkdirAll(generatedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outputDir, err := os.MkdirTemp(generatedRoot, "autonomous-correction-stress-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outputDir) })
	written := writeExplicitCircuitProject(ctx, request, tx, stress.SelectedPlaced, stress.SelectedRouted, CreateOptions{OutputDir: outputDir, Overwrite: true, LibraryIndex: &index})
	if workflowStageBlocked(written.Stage) {
		t.Fatalf("stress project write = %#v", written.Stage)
	}
	artifactDir := filepath.Join(outputDir, ".kicadai", "checks")
	writer := CheckWriterCorrectnessWithOptions(ctx, &written, writercorrectness.Options{
		RequireKiCadRoundTrip: true,
		KiCadCLI:              cliPath,
		KeepArtifacts:         true,
		ArtifactDir:           artifactDir,
		StrictDiffs:           true,
		LibraryIndex:          index,
		HasLibraryIndex:       true,
		LibraryResolutionUsed: true,
	})
	if workflowStageBlocked(writer.Stage) || !writer.Writer.OK || !writerCheckPassed(writer.Writer, writercorrectness.CheckKiCadRoundTrip) {
		t.Fatalf("stress writer correctness = stage %#v writer %#v", writer.Stage, writer.Writer)
	}
	validated := ValidateProject(ctx, &request, &written, ValidationOptions{})
	if workflowStageBlocked(validated.Stage) {
		t.Fatalf("stress internal validation = %#v", validated.Stage)
	}
	kicad := RunKiCadChecks(ctx, &request, &written, KiCadCheckOptions{
		KiCadCLI: cliPath, RequireERC: true, RequireDRC: true,
		KeepArtifacts: true, ArtifactDir: artifactDir,
	})
	if workflowStageBlocked(kicad.Stage) || kicad.ERC.Status != checks.CheckStatusPass || kicad.DRC.Status != checks.CheckStatusPass {
		t.Fatalf("stress KiCad checks = stage %#v ERC %#v DRC %#v", kicad.Stage, kicad.ERC, kicad.DRC)
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

func autonomousCorrectionStressLibraryIndex(t *testing.T, request Request) *libraryresolver.LibraryIndex {
	t.Helper()
	index := &libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{}}
	for _, component := range request.ExplicitCircuit.Components {
		record := placementTestFootprint(component.FootprintID)
		record.Pads = append([]libraryresolver.FootprintPad(nil), record.Pads...)
		if len(record.Pads) < 2 {
			t.Fatalf("stress footprint %s has %d pads; want at least 2", component.FootprintID, len(record.Pads))
		}
		record.Pads[0].Position.X = -resistor0805PadOffsetNM
		record.Pads[0].Size.X = resistor0805PadWidthNM
		record.Pads[0].Size.Y = resistor0805PadHeightNM
		record.Pads[1].Position.X = resistor0805PadOffsetNM
		record.Pads[1].Size.X = resistor0805PadWidthNM
		record.Pads[1].Size.Y = resistor0805PadHeightNM
		index.Footprints[component.FootprintID] = record
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

func writerCheckPassed(result writercorrectness.Result, name string) bool {
	for _, check := range result.Checks {
		if check.Name == name {
			return check.Status == writercorrectness.CheckPass
		}
	}
	return false
}
