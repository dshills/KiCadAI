package roundtrip

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles/sexpr"
)

func CompareSchematicFiles(originalPath, roundTrippedPath string, opts Options) (Result, error) {
	return compareFilesWithNormalizer(originalPath, roundTrippedPath, opts, NormalizeSchematicBytes)
}

func NormalizeSchematicBytes(input []byte) string {
	normalized, ok := normalizedSchematicSymbolOrderText(input)
	if ok {
		return normalized
	}
	return NormalizeBytes(input)
}

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
	comparison, err := CompareSchematicFiles(inputPath, copyPath, compareOpts)
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

func normalizedSchematicSymbolOrderText(input []byte) (string, bool) {
	root, err := sexpr.Parse(input)
	if err != nil || !root.IsList || root.Head() != "kicad_sch" {
		return "", false
	}
	normalized := normalizeSchematicNode(root)
	text, err := sexpr.Format(normalized)
	if err != nil {
		return "", false
	}
	return NormalizeText(text), true
}

func normalizeSchematicNode(node sexpr.ParsedNode) sexpr.Node {
	if !node.IsList {
		return node.Node()
	}
	parentHead := node.Head()
	children := make([]sexpr.Node, 0, len(node.Children))
	for _, child := range node.Children {
		if skipSchematicKiCadInstanceMetadata(parentHead, child) {
			continue
		}
		children = append(children, normalizeSchematicNode(child))
	}
	if node.Head() == "lib_symbols" {
		children = normalizeSchematicLibSymbolsOrder(children)
	} else if schematicPinInstanceNode(node) {
		children = normalizeSchematicPinInstance(children)
	}
	return sexpr.L(children...)
}

func skipSchematicKiCadInstanceMetadata(parentHead string, child sexpr.ParsedNode) bool {
	if !child.IsList {
		return false
	}
	if parentHead == "path" && child.Head() == "value" {
		return true
	}
	return false
}

func schematicPinInstanceNode(node sexpr.ParsedNode) bool {
	return node.IsList && node.Head() == "pin" && len(node.Children) >= 2 && node.Children[1].Quoted
}

func normalizeSchematicPinInstance(children []sexpr.Node) []sexpr.Node {
	if len(children) <= 2 {
		return children
	}
	result := make([]sexpr.Node, 0, len(children))
	result = append(result, children[0], children[1])
	for _, child := range children[2:] {
		list, ok := child.(sexpr.List)
		if ok && len(list) > 0 {
			if atom, ok := list[0].(sexpr.Atom); ok && string(atom) == "uuid" {
				continue
			}
		}
		result = append(result, child)
	}
	return result
}

func normalizeSchematicLibSymbolsOrder(children []sexpr.Node) []sexpr.Node {
	if len(children) <= 2 {
		return children
	}
	ordered := make([]sortableSchematicNode, 0, len(children)-1)
	for _, child := range children[1:] {
		ordered = append(ordered, sortableSchematicNode{node: child, key: schematicSortKey(child)})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].key < ordered[j].key
	})
	result := make([]sexpr.Node, 0, len(children))
	result = append(result, children[0])
	for _, child := range ordered {
		result = append(result, child.node)
	}
	return result
}

type sortableSchematicNode struct {
	node sexpr.Node
	key  string
}

func schematicSortKey(node sexpr.Node) string {
	if list, ok := node.(sexpr.List); ok && len(list) >= 2 {
		if head, ok := list[0].(sexpr.Atom); ok && string(head) == "symbol" {
			switch name := list[1].(type) {
			case sexpr.String:
				return string(name)
			case sexpr.Atom:
				return string(name)
			}
		}
	}
	text, err := sexpr.Format(node)
	if err != nil {
		return fmt.Sprintf("%T", node)
	}
	return text
}
