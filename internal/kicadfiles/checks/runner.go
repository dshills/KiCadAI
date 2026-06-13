package checks

import (
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
	EnvRunKiCadCLI = "KICADAI_RUN_KICAD_CLI"
	EnvKiCadCLI    = "KICADAI_KICAD_CLI"
	maxCLIOutput   = 1 << 20
)

type Runner interface {
	Run(ctx context.Context, workingDir string, path string, args ...string) CommandResult
	Version(ctx context.Context, path string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, workingDir string, path string, args ...string) CommandResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(workingDir) != "" {
		cmd.Dir = workingDir
	}
	cmd.Env = os.Environ()
	var stdout, stderr limitedBuffer
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
	return CommandResult{
		Path:       path,
		Args:       append([]string(nil), args...),
		WorkingDir: workingDir,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode,
		Duration:   time.Since(start),
		Err:        err,
	}
}

type limitedBuffer struct {
	data      []byte
	truncated bool
}

func (buffer *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxCLIOutput - len(buffer.data)
	if remaining > 0 {
		if len(p) <= remaining {
			buffer.data = append(buffer.data, p...)
		} else {
			buffer.data = append(buffer.data, p[:remaining]...)
			buffer.truncated = true
		}
	} else if len(p) > 0 {
		buffer.truncated = true
	}
	return len(p), nil
}

func (buffer *limitedBuffer) String() string {
	if !buffer.truncated {
		return string(buffer.data)
	}
	return string(buffer.data) + "\n[output truncated]"
}

func (runner ExecRunner) Version(ctx context.Context, path string) (string, error) {
	result := runner.Run(ctx, "", path, "--version")
	if result.Err != nil {
		return "", fmt.Errorf("%s --version failed: %w: %s", path, result.Err, strings.TrimSpace(result.Stderr))
	}
	version := strings.TrimSpace(result.Stdout)
	if version == "" {
		version = strings.TrimSpace(result.Stderr)
	}
	if version == "" {
		return "", fmt.Errorf("%s --version returned no output", path)
	}
	return version, nil
}

type KiCadCLI struct {
	Path string
}

func DiscoverCLI(explicit string) (KiCadCLI, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		return validateCLI(path)
	}
	if path := strings.TrimSpace(os.Getenv(EnvKiCadCLI)); path != "" {
		return validateCLI(path)
	}
	if path, err := exec.LookPath("kicad-cli"); err == nil {
		return KiCadCLI{Path: path}, nil
	}
	for _, candidate := range platformCandidates() {
		if cli, err := validateCLI(candidate); err == nil {
			return cli, nil
		}
	}
	return KiCadCLI{}, errors.New("kicad-cli not found; set --kicad-cli, KICADAI_KICAD_CLI, or add kicad-cli to PATH")
}

func validateCLI(path string) (KiCadCLI, error) {
	info, err := os.Stat(path)
	if err != nil {
		return KiCadCLI{}, err
	}
	if info.IsDir() {
		return KiCadCLI{}, fmt.Errorf("%s is a directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return KiCadCLI{}, fmt.Errorf("%s is not executable", path)
	}
	return KiCadCLI{Path: path}, nil
}

func platformCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli"}
	case "linux":
		return []string{"/usr/local/bin/kicad-cli", "/usr/bin/kicad-cli"}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "10.0", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "9.0", "bin", "kicad-cli.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "KiCad", "8.0", "bin", "kicad-cli.exe"),
		}
	default:
		return nil
	}
}
