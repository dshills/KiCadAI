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
}

type FileType string

const (
	FileTypePCB       FileType = "pcb"
	FileTypeSchematic FileType = "schematic"
	FileTypeProject   FileType = "project"
)

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
