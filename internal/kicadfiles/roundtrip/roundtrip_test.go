package roundtrip

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEnabledFromEnv(t *testing.T) {
	t.Setenv(envRunKiCadCLI, "")
	if EnabledFromEnv() {
		t.Fatal("EnabledFromEnv enabled without opt-in")
	}

	t.Setenv(envRunKiCadCLI, "true")
	if EnabledFromEnv() {
		t.Fatal("EnabledFromEnv enabled for non-1 value")
	}

	t.Setenv(envRunKiCadCLI, "1")
	if !EnabledFromEnv() {
		t.Fatal("EnabledFromEnv disabled for 1")
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv(envKeepArtifacts, "1")
	t.Setenv(envArtifactDir, "/tmp/kicadai-roundtrip")
	t.Setenv(envTimeout, "2m")

	opts := OptionsFromEnv()
	if !opts.KeepArtifacts {
		t.Fatal("KeepArtifacts = false, want true")
	}
	if opts.ArtifactDir != "/tmp/kicadai-roundtrip" {
		t.Fatalf("ArtifactDir = %q", opts.ArtifactDir)
	}
	if opts.Timeout != 2*time.Minute {
		t.Fatalf("Timeout = %s", opts.Timeout)
	}
}

func TestOptionsFromEnvDefaultsTimeout(t *testing.T) {
	t.Setenv(envTimeout, "not-a-duration")

	opts := OptionsFromEnv()
	if opts.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", opts.Timeout, defaultTimeout)
	}
}

func TestDiscoverCLIHonerConfiguredPath(t *testing.T) {
	path := fakeExecutable(t, "kicad-cli")
	t.Setenv(envKiCadCLI, path)

	cli, err := DiscoverCLI()
	if err != nil {
		t.Fatalf("DiscoverCLI returned error: %v", err)
	}
	if cli.Path != path {
		t.Fatalf("Path = %q, want %q", cli.Path, path)
	}
}

func TestDiscoverCLIRejectsMissingConfiguredPath(t *testing.T) {
	t.Setenv(envKiCadCLI, filepath.Join(t.TempDir(), "missing-kicad-cli"))

	_, err := DiscoverCLI()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKiCadCLIVersion(t *testing.T) {
	path := fakeCommand(t, "kicad-cli", "10.0.0", "", 0)
	cli := KiCadCLI{Path: path}

	version, err := cli.Version(context.Background())
	if err != nil {
		t.Fatalf("Version returned error: %v", err)
	}
	if version != "10.0.0" {
		t.Fatalf("Version = %q", version)
	}
}

func TestKiCadCLIVersionErrorIncludesPath(t *testing.T) {
	path := fakeCommand(t, "kicad-cli", "", "nope", 3)
	cli := KiCadCLI{Path: path}

	_, err := cli.Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !containsAll(got, path, "nope") {
		t.Fatalf("error = %q", got)
	}
}

func fakeExecutable(t *testing.T, name string) string {
	t.Helper()
	return fakeCommand(t, name, "", "", 0)
}

func fakeCommand(t *testing.T, name, stdout, stderr string, exitCode int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	var body string
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n"
		if stdout != "" {
			body += "echo " + stdout + "\r\n"
		}
		if stderr != "" {
			body += "echo " + stderr + " 1>&2\r\n"
		}
		body += "exit /b " + strconv.Itoa(exitCode) + "\r\n"
	} else {
		body = "#!/bin/sh\n"
		if stdout != "" {
			body += "printf '%s\\n' '" + stdout + "'\n"
		}
		if stderr != "" {
			body += "printf '%s\\n' '" + stderr + "' >&2\n"
		}
		body += "exit " + strconv.Itoa(exitCode) + "\n"
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	return path
}

func containsAll(s string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(s, value) {
			return false
		}
	}
	return true
}
