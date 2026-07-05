package amplifiers

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRunSimulationSkipsWhenRunnerUnavailable(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := RunSimulation(context.Background(), artifact, nil)
	if evaluation.Status != SimulationStatusNotSupported {
		t.Fatalf("status = %q, want not_supported", evaluation.Status)
	}
	if len(evaluation.Issues) != 1 || evaluation.Issues[0].Code != "simulation_runner_missing" {
		t.Fatalf("issues = %#v", evaluation.Issues)
	}
}

func TestRunSimulationPassesKnownGoodHeadphoneFixture(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := RunSimulation(context.Background(), artifact, StaticSimulationRunner{
		RunnerName:     "fixture",
		AvailableValue: true,
		Measurements: SimulationMeasurements{
			OutputDCV:          4.5,
			LoadDCV:            0.001,
			IdleCurrentMA:      12,
			ACGain:             2,
			HighPassCutoffHz:   25,
			OutputSwingVPP:     2.5,
			OutputCurrentMA:    20,
			StabilityMarginDeg: 68,
		},
	})
	if evaluation.Status != SimulationStatusPass {
		t.Fatalf("evaluation = %#v, want pass", evaluation)
	}
	if evaluation.Runner != "fixture" {
		t.Fatalf("runner = %q", evaluation.Runner)
	}
}

func TestRunSimulationBlocksBadHeadphoneFixture(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := RunSimulation(context.Background(), artifact, StaticSimulationRunner{
		AvailableValue: true,
		Measurements: SimulationMeasurements{
			OutputDCV:          6.1,
			LoadDCV:            0.2,
			IdleCurrentMA:      60,
			ACGain:             0.5,
			HighPassCutoffHz:   200,
			OutputSwingVPP:     0.2,
			OutputCurrentMA:    200,
			StabilityMarginDeg: 10,
		},
	})
	if evaluation.Status != SimulationStatusBlocked {
		t.Fatalf("evaluation = %#v, want blocked", evaluation)
	}
	for _, want := range []string{"simulation_ac_gain_out_of_range", "simulation_load_dc_offset"} {
		if !simulationIssueCodesContain(evaluation.Issues, want) {
			t.Fatalf("issues = %#v, want %q", evaluation.Issues, want)
		}
	}
}

func TestRunSimulationBlocksRunnerError(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := RunSimulation(context.Background(), artifact, StaticSimulationRunner{
		AvailableValue: true,
		Err:            errors.New("ngspice failed"),
	})
	if evaluation.Status != SimulationStatusBlocked || !simulationIssueCodesContain(evaluation.Issues, "simulation_runner_failed") {
		t.Fatalf("evaluation = %#v, want runner failure", evaluation)
	}
}

func TestWriteSimulationArtifactsWritesNetlistReportAndRawOutput(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := EvaluateSimulationMeasurements(artifact.Expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            0,
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
		RawOutput:          "raw simulator output\n",
	})
	output := t.TempDir()
	artifacts, issues := WriteSimulationArtifacts(output, artifact, &evaluation, false)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(artifacts) != 3 {
		t.Fatalf("artifacts = %#v, want netlist/report/raw", artifacts)
	}
	for _, rel := range []string{SimulationArtifactNetlistPath, SimulationArtifactReportPath, SimulationArtifactRawPath} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
	reportData, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(SimulationArtifactReportPath)))
	if err != nil {
		t.Fatalf("read simulation report: %v", err)
	}
	if !strings.Contains(string(reportData), SimulationArtifactRawPath) {
		t.Fatalf("simulation report did not list raw artifact:\n%s", string(reportData))
	}
	if strings.Contains(string(reportData), "raw simulator output") {
		t.Fatalf("simulation report embedded raw output:\n%s", string(reportData))
	}
	if !slices.Contains(evaluation.Artifacts, SimulationArtifactRawPath) {
		t.Fatalf("in-memory evaluation artifacts = %#v, want raw path", evaluation.Artifacts)
	}
}

func TestWriteSimulationArtifactsRefusesOverwriteWhenDisabled(t *testing.T) {
	artifact := BuildClassABHeadphoneSimulationArtifact("demo")
	evaluation := EvaluateSimulationMeasurements(artifact.Expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            0,
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
	})
	output := t.TempDir()
	if _, issues := WriteSimulationArtifacts(output, artifact, &evaluation, false); len(issues) != 0 {
		t.Fatalf("initial write issues = %#v", issues)
	}
	artifacts, issues := WriteSimulationArtifacts(output, artifact, &evaluation, false)
	if len(issues) == 0 {
		t.Fatalf("second write succeeded without overwrite")
	}
	if len(artifacts) != 0 {
		t.Fatalf("second write artifacts = %#v, want none for failed writes", artifacts)
	}
}

func TestEvaluateSimulationMeasurementsUsesLoadDCTolerance(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.LoadDCMaxAbsV = float64Ptr(0.25)
	evaluation := EvaluateSimulationMeasurements(expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            0.2,
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
	})
	if simulationIssueCodesContain(evaluation.Issues, "simulation_load_dc_offset") {
		t.Fatalf("evaluation = %#v, load DC should pass custom tolerance", evaluation)
	}
}

func TestEvaluateSimulationMeasurementsAllowsStrictZeroLoadDCTolerance(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.LoadDCMaxAbsV = float64Ptr(0)
	evaluation := EvaluateSimulationMeasurements(expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            0.001,
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
	})
	if !simulationIssueCodesContain(evaluation.Issues, "simulation_load_dc_offset") {
		t.Fatalf("evaluation = %#v, want strict load DC offset issue", evaluation)
	}
}

func TestEvaluateSimulationMeasurementsDefaultsOmittedLoadDCTolerance(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	expectation.LoadDCMaxAbsV = nil
	evaluation := EvaluateSimulationMeasurements(expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            0.001,
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
	})
	if simulationIssueCodesContain(evaluation.Issues, "simulation_load_dc_offset") {
		t.Fatalf("evaluation = %#v, omitted tolerance should use default", evaluation)
	}
}

func TestEvaluateSimulationMeasurementsBlocksNaNLoadDC(t *testing.T) {
	expectation := ClassABHeadphoneSimulationExpectation()
	evaluation := EvaluateSimulationMeasurements(expectation, SimulationMeasurements{
		OutputDCV:          4.5,
		LoadDCV:            math.NaN(),
		IdleCurrentMA:      10,
		ACGain:             2,
		HighPassCutoffHz:   25,
		OutputSwingVPP:     2,
		OutputCurrentMA:    20,
		StabilityMarginDeg: 75,
	})
	if !simulationIssueCodesContain(evaluation.Issues, "simulation_load_dc_offset") {
		t.Fatalf("evaluation = %#v, want load DC offset issue", evaluation)
	}
}

func simulationIssueCodesContain(issues []SimulationIssue, want string) bool {
	for _, issue := range issues {
		if issue.Code == want {
			return true
		}
	}
	return false
}
