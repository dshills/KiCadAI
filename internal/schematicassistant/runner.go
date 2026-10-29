package schematicassistant

import (
	"context"
	"errors"
	"fmt"
	"kicadai/internal/kicadfiles/checks"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SafeRunner bounds command output and prevents provider credentials entering KiCad.
type SafeRunner struct{}
type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(p []byte) (int, error) {
	count := len(p)
	remaining := (64 << 10) - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, p[:min(remaining, len(p))]...)
	}
	return count, nil
}
func (SafeRunner) Run(ctx context.Context, dir, path string, args ...string) checks.CommandResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.Env = filteredEnvironment(os.Environ())
	var stdout, stderr boundedOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
	}
	return checks.CommandResult{Path: path, Args: args, WorkingDir: dir, Stdout: string(stdout.data), Stderr: string(stderr.data), ExitCode: code, Duration: time.Since(start), Err: err}
}
func filteredEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "API_KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") {
			continue
		}
		result = append(result, entry)
	}
	return result
}
func (r SafeRunner) Version(ctx context.Context, path string) (string, error) {
	result := r.Run(ctx, "", path, "--version")
	if result.Err != nil {
		return "", fmt.Errorf("KiCad version: %w", result.Err)
	}
	version := strings.TrimSpace(result.Stdout)
	if version == "" {
		return "", errors.New("KiCad version returned no text")
	}
	return version, nil
}
