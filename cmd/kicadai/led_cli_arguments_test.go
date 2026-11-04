package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLEDDemoRejectsTrailingArgumentsBeforeWriting(t *testing.T) {
	for _, command := range []string{"generate-led-demo", "generate-project"} {
		for _, form := range []string{"reported_flags", "mixed_flags", "positional_argument", "equals_flag", "overwrite_with_trailing_output"} {
			t.Run(command+"/"+form, func(t *testing.T) {
				root := t.TempDir()
				t.Chdir(root)
				target := filepath.Join(root, "requested")
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
				sentinel := filepath.Join(target, "keep.txt")
				if err := os.WriteFile(sentinel, []byte("existing user data"), 0o600); err != nil {
					t.Fatal(err)
				}
				args := []string{command, "--output", target, "--overwrite"}
				defaultTarget := filepath.Join(root, "led_indicator")
				switch form {
				case "mixed_flags":
					args = []string{"--output", target, "--overwrite", command, "--with-pcb"}
				case "positional_argument":
					args = []string{command, "unexpected"}
				case "equals_flag":
					args = []string{command, "--output=" + target}
				case "overwrite_with_trailing_output":
					if err := os.Mkdir(defaultTarget, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(defaultTarget, "keep.txt"), []byte("default-directory user data"), 0o600); err != nil {
						t.Fatal(err)
					}
					args = []string{"--overwrite", command, "--output", target}
				}
				var stdout, stderr bytes.Buffer
				err := run(args, &stdout, &stderr)
				if err == nil || !strings.Contains(err.Error(), "place global flags before the command") {
					t.Fatalf("err=%v stdout=%s", err, stdout.String())
				}
				var result generationResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.WrittenFiles) != 0 || result.Error == "" {
					t.Fatalf("unexpected result: %+v", result)
				}
				if form == "overwrite_with_trailing_output" {
					data, err := os.ReadFile(filepath.Join(defaultTarget, "keep.txt"))
					if err != nil || string(data) != "default-directory user data" {
						t.Fatalf("default-directory data changed: %q %v", data, err)
					}
					entries, err := os.ReadDir(defaultTarget)
					if err != nil || len(entries) != 1 {
						t.Fatalf("default-directory inventory changed: %v %v", entries, err)
					}
				} else if _, err := os.Stat(defaultTarget); !os.IsNotExist(err) {
					t.Fatalf("default output was touched: %v", err)
				}
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 1 || entries[0].Name() != "keep.txt" {
					t.Fatalf("target changed: %v %v", entries, err)
				}
				data, err := os.ReadFile(sentinel)
				if err != nil || string(data) != "existing user data" {
					t.Fatalf("sentinel changed: %q %v", data, err)
				}
			})
		}
	}
}
