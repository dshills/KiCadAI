package writercorrectness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/reports"
)

func CheckKiCadRoundTripEvidence(ctx context.Context, target Target, opts Options) CheckResult {
	cli, issue := resolveRoundTripCLI(opts)
	if issue != nil {
		status := CheckSkipped
		required := false
		if opts.RequireKiCadRoundTrip {
			status = CheckFail
			required = true
		}
		return CheckResult{
			Name:     CheckKiCadRoundTrip,
			Status:   status,
			Required: required,
			Issues:   []reports.Issue{*issue},
			Summary:  issue.Message,
		}
	}
	rtOpts := roundtrip.Options{KeepArtifacts: opts.KeepArtifacts, ArtifactDir: opts.ArtifactDir}
	var issues []reports.Issue
	var artifacts []reports.Artifact
	checked := 0
	for _, path := range target.SchematicFiles {
		if strings.TrimSpace(path) == "" {
			continue
		}
		checked++
		result, err := roundtrip.RoundTripSchematic(ctx, cli, filepath.FromSlash(path), rtOpts)
		issues = append(issues, roundTripIssues(path, err, result)...)
		artifacts = append(artifacts, roundTripArtifacts(result)...)
	}
	if target.PCBPath != "" {
		checked++
		result, err := roundtrip.RoundTripPCB(ctx, cli, filepath.FromSlash(target.PCBPath), rtOpts)
		issues = append(issues, roundTripIssues(target.PCBPath, err, result)...)
		artifacts = append(artifacts, roundTripArtifacts(result)...)
	}
	if checked == 0 {
		return CheckResult{Name: CheckKiCadRoundTrip, Status: CheckSkipped, Required: false, Summary: "no schematic or PCB files resolved for round trip"}
	}
	return CheckResult{
		Name:      CheckKiCadRoundTrip,
		Status:    StatusForIssues(issues),
		Required:  opts.RequireKiCadRoundTrip,
		Issues:    issues,
		Artifacts: artifacts,
		Summary:   "checked " + strconv.Itoa(checked) + " file(s)",
	}
}

func resolveRoundTripCLI(opts Options) (roundtrip.KiCadCLI, *reports.Issue) {
	if strings.TrimSpace(opts.KiCadCLI) != "" {
		resolved, lookErr := exec.LookPath(opts.KiCadCLI)
		if lookErr == nil {
			return roundtrip.KiCadCLI{Path: resolved}, nil
		}
		info, err := os.Stat(opts.KiCadCLI)
		if err != nil {
			severity := reports.SeverityWarning
			if opts.RequireKiCadRoundTrip {
				severity = reports.SeverityError
			}
			issue := reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: severity, Path: "writer.kicad_cli", Message: err.Error()}
			return roundtrip.KiCadCLI{}, &issue
		}
		if info.IsDir() || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
			severity := reports.SeverityWarning
			if opts.RequireKiCadRoundTrip {
				severity = reports.SeverityError
			}
			issue := reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: severity, Path: "writer.kicad_cli", Message: opts.KiCadCLI + " is not executable"}
			return roundtrip.KiCadCLI{}, &issue
		}
		return roundtrip.KiCadCLI{Path: opts.KiCadCLI}, nil
	}
	if !opts.RequireKiCadRoundTrip {
		issue := reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityWarning, Path: "writer.kicad_cli", Message: "KiCad round trip was not run because no KiCad CLI path was configured"}
		return roundtrip.KiCadCLI{}, &issue
	}
	cli, err := roundtrip.DiscoverCLI()
	if err != nil {
		issue := reports.Issue{Code: reports.CodeSkippedExternalTool, Severity: reports.SeverityError, Path: "writer.kicad_cli", Message: err.Error()}
		return roundtrip.KiCadCLI{}, &issue
	}
	return cli, nil
}

func roundTripIssues(path string, err error, result roundtrip.Result) []reports.Issue {
	if err != nil {
		return []reports.Issue{{Code: reports.CodeKiCadCLIFailed, Severity: reports.SeverityError, Path: slashPath(path), Message: err.Error()}}
	}
	var issues []reports.Issue
	for _, diff := range result.Differences {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeRoundTripDiff,
			Severity: reports.SeverityError,
			Path:     slashPath(path),
			Message:  diff.Message,
		})
	}
	return issues
}

func roundTripArtifacts(result roundtrip.Result) []reports.Artifact {
	var artifacts []reports.Artifact
	for _, path := range []string{result.RawDiffPath, result.NormalizedDiffPath, result.SummaryPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactRoundTripReport, Path: slashPath(path), Description: "writer correctness KiCad round-trip artifact"})
	}
	return artifacts
}
