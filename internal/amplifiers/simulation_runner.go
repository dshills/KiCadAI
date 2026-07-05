package amplifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/reports"
)

const (
	SimulationArtifactNetlistPath = ".kicadai/amplifier-simulation-netlist.cir"
	SimulationArtifactReportPath  = ".kicadai/amplifier-simulation.json"
	SimulationArtifactRawPath     = ".kicadai/amplifier-simulation-raw.txt"
)

type SimulationRunner interface {
	Name() string
	Available(context.Context) (bool, string)
	Run(context.Context, SimulationArtifact) (SimulationMeasurements, error)
}

type SimulationMeasurements struct {
	OutputDCV          float64 `json:"output_dc_v"`
	LoadDCV            float64 `json:"load_dc_v"`
	IdleCurrentMA      float64 `json:"idle_current_ma"`
	ACGain             float64 `json:"ac_gain"`
	HighPassCutoffHz   float64 `json:"high_pass_cutoff_hz"`
	OutputSwingVPP     float64 `json:"output_swing_vpp"`
	OutputCurrentMA    float64 `json:"output_current_ma"`
	StabilityMarginDeg float64 `json:"stability_margin_deg"`
	RawOutput          string  `json:"-"`
}

type SimulationEvaluation struct {
	Status       SimulationStatus       `json:"simulation_status"`
	Runner       string                 `json:"runner,omitempty"`
	Measurements SimulationMeasurements `json:"measurements,omitempty"`
	Issues       []SimulationIssue      `json:"issues,omitempty"`
	Artifacts    []string               `json:"artifacts,omitempty"`
}

type SimulationIssue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Metric     string `json:"metric,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type StaticSimulationRunner struct {
	RunnerName     string
	AvailableValue bool
	Unavailable    string
	Measurements   SimulationMeasurements
	Err            error
}

func (runner StaticSimulationRunner) Name() string {
	if runner.RunnerName == "" {
		return "static"
	}
	return runner.RunnerName
}

func (runner StaticSimulationRunner) Available(context.Context) (bool, string) {
	if runner.AvailableValue {
		return true, ""
	}
	if runner.Unavailable != "" {
		return false, runner.Unavailable
	}
	return false, "simulation runner is not configured"
}

func (runner StaticSimulationRunner) Run(context.Context, SimulationArtifact) (SimulationMeasurements, error) {
	if runner.Err != nil {
		return SimulationMeasurements{}, runner.Err
	}
	return runner.Measurements, nil
}

func RunSimulation(ctx context.Context, artifact SimulationArtifact, runner SimulationRunner) SimulationEvaluation {
	if runner == nil {
		return SimulationEvaluation{
			Status: SimulationStatusNotSupported,
			Issues: []SimulationIssue{{
				Code:       "simulation_runner_missing",
				Severity:   "warning",
				Message:    "simulation runner is not configured",
				Suggestion: "configure a local simulator runner before requiring simulation-backed promotion",
			}},
		}
	}
	available, reason := runner.Available(ctx)
	if !available {
		if reason == "" {
			reason = "simulation runner is unavailable"
		}
		return SimulationEvaluation{
			Status: SimulationStatusNotSupported,
			Runner: runner.Name(),
			Issues: []SimulationIssue{{
				Code:       "simulation_runner_unavailable",
				Severity:   "warning",
				Message:    reason,
				Suggestion: "install or configure the selected simulator, or leave simulation checks optional",
			}},
		}
	}
	measurements, err := runner.Run(ctx, artifact)
	if err != nil {
		return SimulationEvaluation{
			Status: SimulationStatusBlocked,
			Runner: runner.Name(),
			Issues: []SimulationIssue{{
				Code:       "simulation_runner_failed",
				Severity:   "error",
				Message:    err.Error(),
				Suggestion: "inspect simulator output and repair the generated SPICE artifact",
			}},
		}
	}
	evaluation := EvaluateSimulationMeasurements(artifact.Expectation, measurements)
	evaluation.Runner = runner.Name()
	return evaluation
}

func EvaluateSimulationMeasurements(expectation SimulationExpectation, measurements SimulationMeasurements) SimulationEvaluation {
	var issues []SimulationIssue
	issues = appendRangeMeasurementIssue(issues, "output_dc_v", measurements.OutputDCV, expectation.OperatingPoint.OutputDCMinV, expectation.OperatingPoint.OutputDCMaxV, "adjust bias network so output operating point sits inside the expected range")
	issues = appendRangeMeasurementIssue(issues, "idle_current_ma", measurements.IdleCurrentMA, expectation.OperatingPoint.IdleMinMA, expectation.OperatingPoint.IdleMaxMA, "adjust output bias or emitter degeneration to restore quiescent current")
	issues = appendRangeMeasurementIssue(issues, "ac_gain", measurements.ACGain, expectation.ACGain.Min, expectation.ACGain.Max, "adjust feedback ratio or gain-stage selection")
	issues = appendRangeMeasurementIssue(issues, "high_pass_cutoff_hz", measurements.HighPassCutoffHz, expectation.HighPassCutoffHz.Min, expectation.HighPassCutoffHz.Max, "adjust coupling capacitor or load model")
	issues = appendRangeMeasurementIssue(issues, "output_swing_vpp", measurements.OutputSwingVPP, expectation.OutputSwingVPP.Min, expectation.OutputSwingVPP.Max, "check rail headroom and output-stage bias")
	issues = appendRangeMeasurementIssue(issues, "output_current_ma", measurements.OutputCurrentMA, expectation.OutputCurrentMA.Min, expectation.OutputCurrentMA.Max, "check load impedance and output device limits")
	issues = appendRangeMeasurementIssue(issues, "stability_margin_deg", measurements.StabilityMarginDeg, expectation.StabilityMarginDeg.Min, expectation.StabilityMarginDeg.Max, "add compensation or reduce closed-loop bandwidth")
	loadDCTolerance := defaultLoadDCMaxAbsV
	if expectation.LoadDCMaxAbsV != nil {
		loadDCTolerance = *expectation.LoadDCMaxAbsV
	}
	if loadDCTolerance < 0 || simulationInvalidFloat(loadDCTolerance) {
		loadDCTolerance = defaultLoadDCMaxAbsV
	}
	if simulationInvalidFloat(measurements.LoadDCV) || measurements.LoadDCV < -loadDCTolerance || measurements.LoadDCV > loadDCTolerance {
		issues = append(issues, SimulationIssue{
			Code:       "simulation_load_dc_offset",
			Severity:   "error",
			Metric:     "load_dc_v",
			Message:    fmt.Sprintf("load DC offset %.4g V is outside +/-%.4g V", measurements.LoadDCV, loadDCTolerance),
			Suggestion: "verify the output coupling capacitor and load reference",
		})
	}
	status := SimulationStatusPass
	if len(issues) != 0 {
		status = SimulationStatusBlocked
	}
	return SimulationEvaluation{Status: status, Measurements: measurements, Issues: issues}
}

func WriteSimulationArtifacts(outputRoot string, artifact SimulationArtifact, evaluation *SimulationEvaluation, overwrite bool) ([]reports.Artifact, []reports.Issue) {
	var issues []reports.Issue
	if evaluation == nil {
		return nil, []reports.Issue{{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       SimulationArtifactReportPath,
			Message:    "simulation evaluation is required",
			Suggestion: "run or evaluate simulation before writing simulation artifacts",
		}}
	}
	netlistPath := filepath.Join(outputRoot, filepath.FromSlash(SimulationArtifactNetlistPath))
	reportPath := filepath.Join(outputRoot, filepath.FromSlash(SimulationArtifactReportPath))
	updatedEvaluation := *evaluation
	updatedEvaluation.Artifacts = append([]string(nil), evaluation.Artifacts...)
	var artifacts []reports.Artifact
	if err := writeSimulationFile(netlistPath, []byte(artifact.Netlist), overwrite); err != nil {
		issues = append(issues, simulationWriteIssue(SimulationArtifactNetlistPath, err))
	} else {
		updatedEvaluation.Artifacts = appendUniqueString(updatedEvaluation.Artifacts, SimulationArtifactNetlistPath)
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactSimulationReport, Path: SimulationArtifactNetlistPath, Description: "Amplifier SPICE simulation netlist"})
	}
	if evaluation.Measurements.RawOutput != "" {
		rawPath := filepath.Join(outputRoot, filepath.FromSlash(SimulationArtifactRawPath))
		if err := writeSimulationFile(rawPath, []byte(evaluation.Measurements.RawOutput), overwrite); err != nil {
			issues = append(issues, simulationWriteIssue(SimulationArtifactRawPath, err))
		} else {
			updatedEvaluation.Artifacts = appendUniqueString(updatedEvaluation.Artifacts, SimulationArtifactRawPath)
			artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactSimulationReport, Path: SimulationArtifactRawPath, Description: "Raw amplifier simulator output"})
		}
	}
	reportEvaluation := updatedEvaluation
	reportEvaluation.Artifacts = append([]string(nil), updatedEvaluation.Artifacts...)
	reportEvaluation.Artifacts = appendUniqueString(reportEvaluation.Artifacts, SimulationArtifactReportPath)
	reportData, err := json.MarshalIndent(reportEvaluation, "", "  ")
	if err != nil {
		issues = append(issues, simulationWriteIssue(SimulationArtifactReportPath, err))
	} else if err := writeSimulationFile(reportPath, append(reportData, '\n'), overwrite); err != nil {
		issues = append(issues, simulationWriteIssue(SimulationArtifactReportPath, err))
	} else {
		updatedEvaluation = reportEvaluation
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactSimulationReport, Path: SimulationArtifactReportPath, Description: "Normalized amplifier simulation evaluation"})
	}
	*evaluation = updatedEvaluation
	return artifacts, issues
}

func appendRangeMeasurementIssue(issues []SimulationIssue, metric string, value float64, min float64, max float64, suggestion string) []SimulationIssue {
	if simulationInvalidFloat(value) || value < min || value > max {
		return append(issues, SimulationIssue{
			Code:       "simulation_" + metric + "_out_of_range",
			Severity:   "error",
			Metric:     metric,
			Message:    fmt.Sprintf("%s %.4g is outside %.4g..%.4g", metric, value, min, max),
			Suggestion: suggestion,
		})
	}
	return issues
}

func writeSimulationFile(path string, data []byte, overwrite bool) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if !overwrite {
		var file *os.File
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := file.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}()
		_, err = file.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func simulationWriteIssue(path string, err error) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       path,
		Message:    err.Error(),
		Suggestion: "ensure the output directory is writable or rerun with overwrite enabled",
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
