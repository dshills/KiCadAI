package roundtrip

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func RoundTripSchematic(ctx context.Context, cli KiCadCLI, inputPath string, opts Options) (Result, error) {
	if strings.TrimSpace(cli.Path) == "" {
		return Result{}, errors.New("kicad-cli path is empty")
	}
	fixtureName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if fixtureName == "" {
		fixtureName = "schematic-roundtrip"
	}
	workspace, cleanup, err := NewArtifactWorkspace(fixtureName, opts)
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
	versionCtx, cancelVersion := contextWithOptionalTimeout(ctx, timeout)
	version, versionErr := cli.Version(versionCtx)
	cancelVersion()
	if versionErr != nil {
		version = ""
	}

	runCtx, cancelRun := contextWithOptionalTimeout(ctx, timeout)
	stdout, stderr, exitCode, err := runKiCad(runCtx, filepath.Dir(copyPath), cli.Path, "sch", "upgrade", "--force", copyPath)
	cancelRun()

	result := Result{
		FixtureName:      fixtureName,
		FileType:         FileTypeSchematic,
		KiCadCLIPath:     cli.Path,
		KiCadVersion:     version,
		OriginalPath:     inputPath,
		RoundTrippedPath: copyPath,
		Stdout:           stdout,
		Stderr:           stderr,
		ExitCode:         exitCode,
	}
	if err != nil {
		return result, fmt.Errorf("%s sch upgrade failed with exit code %d: %w: %s", cli.Path, exitCode, err, strings.TrimSpace(stderr))
	}

	compareOpts := opts
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
	if len(opts.Allowlist) > 0 {
		filtered, _, err := FilterAllowedDifferences(comparison, opts.Allowlist)
		if err != nil {
			return comparison, err
		}
		comparison = filtered
	}
	return comparison, nil
}
