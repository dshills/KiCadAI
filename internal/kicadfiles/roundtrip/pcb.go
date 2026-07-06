package roundtrip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

const pcbSummaryNetSection = "net"

func ComparePCBFiles(originalPath, roundTrippedPath string, opts Options) (Result, error) {
	return compareFilesWithNormalizer(originalPath, roundTrippedPath, opts, NormalizePCBBytes)
}

func NormalizePCBBytes(input []byte) string {
	normalized, ok := normalizedPCBKiCad10NetText(input)
	if ok {
		return normalized
	}
	return NormalizeBytes(input)
}

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
	versionCtx, cancelVersion := contextWithOptionalTimeout(ctx, timeout)
	version, versionErr := cli.Version(versionCtx)
	cancelVersion()
	if versionErr != nil {
		version = ""
	}

	runCtx, cancelRun := contextWithOptionalTimeout(ctx, timeout)
	stdout, stderr, exitCode, err := runKiCad(runCtx, filepath.Dir(copyPath), cli.Path, "pcb", "upgrade", "--force", copyPath)
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
	compareOpts.ArtifactDir = workspace.Root
	comparison, err := ComparePCBFiles(inputPath, copyPath, compareOpts)
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
		if originalSummaryText != roundTripSummaryText && !equivalentPCBSummaries(originalSummary, roundTripSummary) {
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
	if len(opts.Allowlist) > 0 {
		filtered, _, err := FilterAllowedDifferences(comparison, opts.Allowlist)
		if err != nil {
			return comparison, err
		}
		comparison = filtered
	}
	return comparison, nil
}

func normalizedPCBKiCad10NetText(input []byte) (string, bool) {
	root, err := sexpr.Parse(input)
	if err != nil || !root.IsList || root.Head() != "kicad_pcb" {
		return "", false
	}
	netNames := topLevelPCBNetNames(root)
	normalized := normalizePCBNetNode(root, netNames, true)
	text, err := sexpr.Format(normalized)
	if err != nil {
		return "", false
	}
	return NormalizeText(text), true
}

func topLevelPCBNetNames(root sexpr.ParsedNode) map[int]string {
	netNames := map[int]string{}
	for _, child := range root.Children {
		code, name, ok := pcbNumericNetDeclaration(child)
		if ok {
			netNames[code] = name
		}
	}
	return netNames
}

func normalizePCBNetNode(node sexpr.ParsedNode, netNames map[int]string, root bool) sexpr.Node {
	if !node.IsList {
		return node.Node()
	}
	children := make([]sexpr.Node, 0, len(node.Children))
	for _, child := range node.Children {
		if root {
			if _, _, ok := pcbNumericNetDeclaration(child); ok {
				continue
			}
		}
		if child.IsList && child.Head() == "net" {
			if normalized, ok := normalizePCBNetReference(child, netNames); ok {
				children = append(children, normalized)
				continue
			}
		}
		children = append(children, normalizePCBNetNode(child, netNames, false))
	}
	return sexpr.L(children...)
}

func pcbNumericNetDeclaration(node sexpr.ParsedNode) (int, string, bool) {
	if !node.IsList || node.Head() != "net" || len(node.Children) < 3 {
		return 0, "", false
	}
	code, err := strconv.Atoi(node.ListValue(1))
	if err != nil {
		return 0, "", false
	}
	return code, node.ListValue(2), true
}

func normalizePCBNetReference(node sexpr.ParsedNode, netNames map[int]string) (sexpr.Node, bool) {
	if len(node.Children) == 3 {
		code, name, ok := pcbNumericNetDeclaration(node)
		if !ok || code == 0 || strings.TrimSpace(name) == "" {
			return nil, false
		}
		return sexpr.L(sexpr.A("net"), sexpr.S(name)), true
	}
	if len(node.Children) != 2 {
		return nil, false
	}
	code, err := strconv.Atoi(node.ListValue(1))
	if err != nil || code == 0 {
		return nil, false
	}
	name := strings.TrimSpace(netNames[code])
	if name == "" {
		return nil, false
	}
	return sexpr.L(sexpr.A("net"), sexpr.S(name)), true
}

func equivalentPCBSummaries(original Summary, roundTripped Summary) bool {
	originalSections := summarySectionsWithout(original, pcbSummaryNetSection)
	roundTrippedSections := summarySectionsWithout(roundTripped, pcbSummaryNetSection)
	if len(originalSections) != len(roundTrippedSections) {
		return false
	}
	for section, originalCount := range originalSections {
		if roundTrippedSections[section] != originalCount {
			return false
		}
	}
	return true
}

func summarySectionsWithout(summary Summary, omitted string) map[string]int {
	sections := make(map[string]int, len(summary.Sections))
	for section, count := range summary.Sections {
		if section == omitted {
			continue
		}
		sections[section] = count
	}
	return sections
}

func runKiCad(ctx context.Context, workingDir, path string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(workingDir) != "" {
		cmd.Dir = workingDir
	}
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
