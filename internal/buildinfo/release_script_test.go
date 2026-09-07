package buildinfo

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseScriptAuthenticatesBuildInputs(t *testing.T) {
	for _, scenario := range []string{"wrong commit", "invalid date", "leading zero version", "multiline version", "multiline workers", "oversized workers", "foreign working directory"} {
		t.Run(scenario, func(t *testing.T) {
			root, script, fakeBin, commit := releaseScriptFixture(t)
			output := filepath.Join(t.TempDir(), "artifacts")
			version, date := "1.0.0", "2026-09-06T00:00:00Z"
			workers := "2"
			wantSuccess := false
			switch scenario {
			case "wrong commit":
				commit = strings.Repeat("a", 40)
			case "invalid date":
				date = "bad\"date\n"
			case "leading zero version":
				version = "01.0.0"
			case "multiline version":
				version = "1.0.0\ninvalid"
			case "multiline workers":
				workers = "2\ninvalid"
			case "oversized workers":
				workers = strings.Repeat("9", 40)
				wantSuccess = true
			case "foreign working directory":
				wantSuccess = true
			}
			command := exec.Command("bash", script)
			command.Dir = t.TempDir()
			command.Env = releaseScriptEnvironment(map[string]string{
				"PATH":    fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"VERSION": version, "COMMIT": commit, "BUILD_DATE": date,
				"OUTPUT_DIR": output, "EXPECTED_RELEASE_ROOT": root,
				"RELEASE_MAX_CONCURRENT_BUILDS": workers,
			})
			data, err := command.CombinedOutput()
			if wantSuccess {
				if err != nil {
					t.Fatalf("release from foreign directory: %v\n%s", err, data)
				}
				manifest, err := os.ReadFile(filepath.Join(output, "RELEASE_MANIFEST.json"))
				if err != nil || !json.Valid(manifest) {
					t.Fatalf("invalid release manifest: %v\n%s", err, manifest)
				}
			} else {
				if err == nil {
					t.Fatalf("invalid release identity accepted: %s", data)
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("invalid release inputs must fail before creating artifacts: %v", err)
				}
			}
		})
	}
}

func releaseScriptFixture(t *testing.T) (root, script, fakeBin, commit string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("release shell scripts target macOS and Linux")
	}
	root = t.TempDir()
	script = filepath.Join(root, "scripts", "build-release.sh")
	source, err := os.ReadFile(filepath.Join("..", "..", "scripts", "build-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, source, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "scripts"}, {"-c", "user.name=Release Test", "-c", "user.email=release@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v\n%s", err, output)
		}
	}
	output, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit = strings.TrimSpace(string(output))
	fakeBin = t.TempDir()
	// The stub makes packaging tests cheap while checking the compiler's actual
	// working directory; real cross-platform compilation is a separate gate.
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = version ]; then
  printf 'go version go1.26.8 test/test\n'
  exit 0
fi
if [ "$(pwd -P)" != "$(cd "$EXPECTED_RELEASE_ROOT" && pwd -P)" ]; then
  printf 'compiler invoked from wrong checkout\n' >&2
  exit 9
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    printf 'test artifact\n' > "$2"
    exit 0
  fi
  shift
done
exit 8
`
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, script, fakeBin, commit
}

func releaseScriptEnvironment(values map[string]string) []string {
	result := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; replaced || name == "ALLOW_DIRTY_RELEASE" || strings.HasPrefix(name, "RELEASE_MAX_") {
			continue
		}
		result = append(result, entry)
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}
