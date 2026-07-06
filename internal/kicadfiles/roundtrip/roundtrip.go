package roundtrip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	envRunKiCadCLI   = "KICADAI_RUN_KICAD_CLI"
	envKiCadCLI      = "KICADAI_KICAD_CLI"
	envKeepArtifacts = "KICADAI_KEEP_ROUNDTRIP_ARTIFACTS"
	envArtifactDir   = "KICADAI_ROUNDTRIP_ARTIFACT_DIR"
	envTimeout       = "KICADAI_ROUNDTRIP_TIMEOUT"

	defaultTimeout = 60 * time.Second
)

type KiCadCLI struct {
	Path string
}

type Options struct {
	KeepArtifacts bool
	ArtifactDir   string
	Timeout       time.Duration
	Allowlist     []AllowlistEntry
}

type FileType string

const (
	FileTypePCB       FileType = "pcb"
	FileTypeSchematic FileType = "schematic"
	FileTypeProject   FileType = "project"
)

type Result struct {
	FixtureName        string
	FileType           FileType
	KiCadCLIPath       string
	KiCadVersion       string
	OriginalPath       string
	RoundTrippedPath   string
	RawDiffPath        string
	NormalizedDiffPath string
	SummaryPath        string
	Stdout             string
	Stderr             string
	ExitCode           int
	Equal              bool
	Differences        []Difference
}

type Difference struct {
	Category string
	Section  string
	ObjectID string
	Message  string
	Diff     string
}

func EnabledFromEnv() bool {
	return os.Getenv(envRunKiCadCLI) == "1"
}

func OptionsFromEnv() Options {
	opts := Options{
		KeepArtifacts: os.Getenv(envKeepArtifacts) == "1",
		ArtifactDir:   strings.TrimSpace(os.Getenv(envArtifactDir)),
		Timeout:       defaultTimeout,
	}
	if raw := strings.TrimSpace(os.Getenv(envTimeout)); raw != "" {
		if timeout, err := time.ParseDuration(raw); err == nil && timeout > 0 {
			opts.Timeout = timeout
		}
	}
	return opts
}

func DiscoverCLI() (KiCadCLI, error) {
	if configured := strings.TrimSpace(os.Getenv(envKiCadCLI)); configured != "" {
		if err := validateExecutable(configured); err != nil {
			return KiCadCLI{}, fmt.Errorf("%s=%s: %w", envKiCadCLI, configured, err)
		}
		return KiCadCLI{Path: configured}, nil
	}
	if path, err := exec.LookPath("kicad-cli"); err == nil {
		return KiCadCLI{Path: path}, nil
	}
	for _, candidate := range platformCandidates() {
		if err := validateExecutable(candidate); err == nil {
			return KiCadCLI{Path: candidate}, nil
		}
	}
	return KiCadCLI{}, errors.New("kicad-cli not found; set KICADAI_KICAD_CLI or add kicad-cli to PATH")
}

func (cli KiCadCLI) Version(ctx context.Context) (string, error) {
	if strings.TrimSpace(cli.Path) == "" {
		return "", errors.New("kicad-cli path is empty")
	}
	cmd := exec.CommandContext(ctx, cli.Path, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s --version failed: %w: %s", cli.Path, err, strings.TrimSpace(stderr.String()))
	}
	version := strings.TrimSpace(stdout.String())
	if version == "" {
		version = strings.TrimSpace(stderr.String())
	}
	if version == "" {
		return "", fmt.Errorf("%s --version returned no output", cli.Path)
	}
	return version, nil
}

func contextWithOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("is not executable")
	}
	return nil
}

func platformCandidates() []string {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, "/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli")
	case "linux":
		candidates = append(candidates, "/usr/bin/kicad-cli", "/usr/local/bin/kicad-cli")
	case "windows":
		candidates = append(candidates,
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "8.0", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "9.0", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "10.0", "bin", "kicad-cli.exe"),
		)
	}
	return candidates
}

func CompareFiles(originalPath, roundTrippedPath string, opts Options) (Result, error) {
	return compareFilesWithNormalizer(originalPath, roundTrippedPath, opts, NormalizeBytes)
}

func compareFilesWithNormalizer(originalPath, roundTrippedPath string, opts Options, normalize func([]byte) string) (Result, error) {
	original, err := os.ReadFile(originalPath)
	if err != nil {
		return Result{}, fmt.Errorf("read original %s: %w", originalPath, err)
	}
	roundTripped, err := os.ReadFile(roundTrippedPath)
	if err != nil {
		return Result{}, fmt.Errorf("read round-tripped %s: %w", roundTrippedPath, err)
	}

	normalizedOriginal := normalize(original)
	normalizedRoundTripped := normalize(roundTripped)
	normalizedDiff := unifiedDiff(originalPath+" (normalized)", roundTrippedPath+" (normalized)", normalizedOriginal, normalizedRoundTripped)

	result := Result{
		OriginalPath:       originalPath,
		RoundTrippedPath:   roundTrippedPath,
		Equal:              normalizedOriginal == normalizedRoundTripped,
		RawDiffPath:        "",
		NormalizedDiffPath: "",
	}
	if !result.Equal {
		result.Differences = append(result.Differences, Difference{
			Category: "normalized-diff",
			Message:  firstDiffMessage(normalizedDiff),
			Diff:     normalizedDiff,
		})
	}

	if opts.KeepArtifacts && strings.TrimSpace(opts.ArtifactDir) != "" {
		originalText := string(original)
		roundTrippedText := string(roundTripped)
		rawDiff := unifiedDiff(originalPath, roundTrippedPath, originalText, roundTrippedText)
		if err := os.MkdirAll(opts.ArtifactDir, 0o755); err != nil {
			return Result{}, fmt.Errorf("create artifact dir %s: %w", opts.ArtifactDir, err)
		}
		rawPath := filepath.Join(opts.ArtifactDir, "raw.diff")
		normalizedPath := filepath.Join(opts.ArtifactDir, "normalized.diff")
		if err := os.WriteFile(rawPath, []byte(rawDiff), 0o644); err != nil {
			return Result{}, fmt.Errorf("write raw diff: %w", err)
		}
		if err := os.WriteFile(normalizedPath, []byte(normalizedDiff), 0o644); err != nil {
			return Result{}, fmt.Errorf("write normalized diff: %w", err)
		}
		result.RawDiffPath = rawPath
		result.NormalizedDiffPath = normalizedPath
	}

	return result, nil
}

func NormalizeText(input string) string {
	return NormalizeBytes([]byte(input))
}

func NormalizeBytes(input []byte) string {
	if normalized, ok := normalizedSExprText(input); ok {
		return normalized
	}
	var out strings.Builder
	lineCount := 0
	pendingBlank := 0
	forEachNormalizedLine(input, func(line string) {
		if line == "" {
			pendingBlank++
			return
		}
		for ; pendingBlank > 0; pendingBlank-- {
			writeNormalizedLine(&out, "", &lineCount)
		}
		writeNormalizedLine(&out, line, &lineCount)
	})
	out.WriteString("\n")
	return out.String()
}

func normalizedSExprText(input []byte) (string, bool) {
	var out strings.Builder
	tokenCount := 0
	depth := 0
	for i := 0; i < len(input); {
		switch input[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case ';':
			for i < len(input) && input[i] != '\n' && input[i] != '\r' {
				i++
			}
		case '(', ')':
			if input[i] == '(' {
				depth++
			} else {
				depth--
				if depth < 0 {
					return "", false
				}
			}
			writeNormalizedToken(&out, &tokenCount, string(input[i]))
			i++
		case '"':
			start := i
			i++
			escaped := false
			for i < len(input) {
				switch {
				case escaped:
					escaped = false
				case input[i] == '\\':
					escaped = true
				case input[i] == '"':
					i++
					writeNormalizedToken(&out, &tokenCount, string(input[start:i]))
					goto nextToken
				}
				i++
			}
			return "", false
		default:
			start := i
			for i < len(input) && !isSExprWhitespace(input[i]) && input[i] != '(' && input[i] != ')' && input[i] != ';' {
				i++
			}
			if start == i {
				return "", false
			}
			writeNormalizedToken(&out, &tokenCount, string(input[start:i]))
		}
	nextToken:
	}
	if tokenCount == 0 || depth != 0 {
		return "", false
	}
	out.WriteString("\n")
	return out.String(), true
}

func writeNormalizedToken(out *strings.Builder, tokenCount *int, token string) {
	if *tokenCount > 0 {
		out.WriteString("\n")
	}
	out.WriteString(token)
	*tokenCount = *tokenCount + 1
}

func isSExprWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func forEachNormalizedLine(input []byte, yield func(string)) {
	start := 0
	for i := 0; i < len(input); i++ {
		if input[i] != '\n' && input[i] != '\r' {
			continue
		}
		line := bytes.TrimRight(input[start:i], " \t")
		yield(string(line))
		if input[i] == '\r' && i+1 < len(input) && input[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	line := bytes.TrimRight(input[start:], " \t")
	yield(string(line))
}

func writeNormalizedLine(out *strings.Builder, line string, lineCount *int) {
	if *lineCount > 0 {
		out.WriteString("\n")
	}
	out.WriteString(line)
	*lineCount = *lineCount + 1
}

func unifiedDiff(aName, bName, a, b string) string {
	if a == b {
		return ""
	}
	if diff, err := systemUnifiedDiff(aName, bName, a, b); err == nil {
		return diff
	}
	aLines := splitDiffLines(a)
	bLines := splitDiffLines(b)
	ops, startLine := compactDiffLineOps(aLines, bLines)
	var out strings.Builder
	out.WriteString("--- ")
	out.WriteString(aName)
	out.WriteString("\n+++ ")
	out.WriteString(bName)
	out.WriteString("\n")
	oldCount, newCount := diffOutputCounts(ops)
	fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", startLine, oldCount, startLine, newCount)
	for _, op := range ops {
		out.WriteString(op.prefix)
		out.WriteString(op.line)
		if !strings.HasSuffix(op.line, "\n") {
			out.WriteString("\n")
		}
	}
	return out.String()
}

func diffOutputCounts(ops []diffLineOp) (int, int) {
	var oldCount, newCount int
	for _, op := range ops {
		if op.prefix == " " || op.prefix == "-" {
			oldCount++
		}
		if op.prefix == " " || op.prefix == "+" {
			newCount++
		}
	}
	return oldCount, newCount
}

func systemUnifiedDiff(aName, bName, a, b string) (string, error) {
	if _, err := exec.LookPath("diff"); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "kicadai-diff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	aPath := filepath.Join(dir, "a")
	bPath := filepath.Join(dir, "b")
	if err := os.WriteFile(aPath, []byte(a), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(bPath, []byte(b), 0o600); err != nil {
		return "", err
	}

	cmd := exec.Command("diff", "-u", "--label", aName, "--label", bName, aPath, bPath)
	output, err := cmd.Output()
	if err == nil {
		return "", nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return string(output), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return "", err
}

type diffLineOp struct {
	prefix string
	line   string
}

func compactDiffLineOps(aLines, bLines []string) ([]diffLineOp, int) {
	ops := make([]diffLineOp, 0, 32)
	prefix := 0
	for prefix < len(aLines) && prefix < len(bLines) && aLines[prefix] == bLines[prefix] {
		prefix++
	}
	suffixA := len(aLines)
	suffixB := len(bLines)
	for suffixA > prefix && suffixB > prefix && aLines[suffixA-1] == bLines[suffixB-1] {
		suffixA--
		suffixB--
	}
	contextStart := prefix - 3
	if contextStart < 0 {
		contextStart = 0
	}
	for i := contextStart; i < prefix; i++ {
		ops = append(ops, diffLineOp{prefix: " ", line: aLines[i]})
	}
	changedA := suffixA - prefix
	changedB := suffixB - prefix
	ops = append(ops, diffLineOp{
		prefix: "-",
		line:   fmt.Sprintf("[large diff omitted: %d original lines changed]\n", changedA),
	})
	ops = append(ops, diffLineOp{
		prefix: "+",
		line:   fmt.Sprintf("[large diff omitted: %d round-tripped lines changed]\n", changedB),
	})
	contextEndA := suffixA + 3
	if contextEndA > len(aLines) {
		contextEndA = len(aLines)
	}
	for i := suffixA; i < contextEndA; i++ {
		ops = append(ops, diffLineOp{prefix: " ", line: aLines[i]})
	}
	return ops, contextStart + 1
}

func splitDiffLines(input string) []string {
	lines := strings.SplitAfter(input, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func firstDiffMessage(diff string) string {
	for {
		line, rest, ok := strings.Cut(diff, "\n")
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "+") {
			if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
				if !ok {
					break
				}
				diff = rest
				continue
			}
			return strings.TrimSpace(line)
		}
		if !ok {
			break
		}
		diff = rest
	}
	return "normalized files differ"
}
