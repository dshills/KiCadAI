package roundtrip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func RoundTripPCB(ctx context.Context, cli KiCadCLI, inputPath string, opts Options) (Result, error) {
	if strings.TrimSpace(cli.Path) == "" {
		return Result{}, errors.New("kicad-cli path is empty")
	}
	workspace, cleanup, err := NewArtifactWorkspace("pcb-roundtrip", opts)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	copyPath, err := workspace.CopyInput(inputPath)
	if err != nil {
		return Result{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	versionCtx, cancelVersion := context.WithTimeout(ctx, timeout)
	version, versionErr := cli.Version(versionCtx)
	cancelVersion()
	if versionErr != nil {
		version = ""
	}

	runCtx, cancelRun := context.WithTimeout(ctx, timeout)
	stdout, stderr, exitCode, err := runKiCad(runCtx, cli.Path, "pcb", "upgrade", "--force", copyPath)
	cancelRun()

	result := Result{
		FixtureName:      "pcb-roundtrip",
		FileType:         FileTypePCB,
		KiCadCLIPath:     cli.Path,
		KiCadVersion:     version,
		OriginalPath:     inputPath,
		RoundTrippedPath: copyPath,
		Stdout:           stdout,
		Stderr:           stderr,
		ExitCode:         exitCode,
	}
	if err != nil {
		return result, fmt.Errorf("%s pcb upgrade failed with exit code %d: %w: %s", cli.Path, exitCode, err, strings.TrimSpace(stderr))
	}

	compareOpts := opts
	compareOpts.KeepArtifacts = true
	compareOpts.ArtifactDir = workspace.Root
	comparison, err := CompareFiles(inputPath, copyPath, compareOpts)
	if err != nil {
		return result, err
	}
	comparison.FixtureName = result.FixtureName
	comparison.FileType = result.FileType
	comparison.KiCadCLIPath = result.KiCadCLIPath
	comparison.KiCadVersion = result.KiCadVersion
	comparison.Stdout = result.Stdout
	comparison.Stderr = result.Stderr
	comparison.ExitCode = result.ExitCode
	originalSummary, originalSummaryErr := SummarizePCB(inputPath)
	roundTripSummary, roundTripSummaryErr := SummarizePCB(copyPath)
	if originalSummaryErr != nil {
		comparison.Differences = append(comparison.Differences, Difference{
			Category: "summary-error",
			Message:  originalSummaryErr.Error(),
		})
		comparison.Equal = false
	}
	if roundTripSummaryErr != nil {
		comparison.Differences = append(comparison.Differences, Difference{
			Category: "summary-error",
			Message:  roundTripSummaryErr.Error(),
		})
		comparison.Equal = false
	}
	if originalSummaryErr == nil && roundTripSummaryErr == nil {
		originalSummaryText := originalSummary.String()
		roundTripSummaryText := roundTripSummary.String()
		if originalSummaryText != roundTripSummaryText {
			comparison.Differences = append(comparison.Differences, Difference{
				Category: "summary-diff",
				Message:  "PCB section summary changed",
			})
			comparison.Equal = false
		}
		if opts.KeepArtifacts {
			path, err := workspace.WriteText("summary.txt", "original:\n"+originalSummaryText+"\nround-tripped:\n"+roundTripSummaryText)
			if err == nil {
				comparison.SummaryPath = path
			}
		}
	}
	return comparison, nil
}

func runKiCad(ctx context.Context, path string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return stdout.String(), stderr.String(), exitCode, err
}
