package main

import (
	"bytes"
	"context"
	"encoding/json"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/netformbridge"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductWorkflowCLIAndOfflineBoundary(t *testing.T) {
	root := t.TempDir()
	var output, stderr bytes.Buffer
	for _, args := range [][]string{{"doctor"}, {"family", "profiles"}} {
		output.Reset()
		if err := run(args, &output, &stderr); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join("..", "..", "examples", "install-experience", "board-family.json")
	family := filepath.Join(root, "family")
	if err := run([]string{"--request", config, "--output", family, "family", "generate"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--target", family, "family", "verify"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := run([]string{"--output", source, "schematic", "example", "divider"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(root, "review")
	if err := run([]string{"--target", source, "--output", report, "review", "schematic"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--target", report, "review", "verify"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"schema": netformbridge.RequestSchema, "id": "cli-1", "method": "schematic.inspect", "params": map[string]string{"path": source}})
	output.Reset()
	if err := runAdapter(context.Background(), cliOptions{}, bytes.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"ok": true`) {
		t.Fatal("adapter inspection failed")
	}
	for _, args := range [][]string{{"--provider", "openai", "doctor"}, {"--overwrite", "family", "generate"}, {"--require-erc", "adapter"}, {"review", "repair"}, {"component", "self-test"}, {"--kicad-cli", "/nonexistent/kicad", "--require-erc", "doctor"}} {
		if err := run(args, &output, &stderr); err == nil {
			t.Fatal("accepted unsafe/missing inputs", args)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "repair")); !os.IsNotExist(err) {
		t.Fatal("review wrote repairs")
	}
}

func TestOfflineFamilyVerifyRejectsContradictoryClaims(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "family")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--request", filepath.Join("..", "..", "examples", "install-experience", "board-family.json"), "--output", source, "family", "generate"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"native", "hardware", "configuration", "electrical"} {
		t.Run(kind, func(t *testing.T) {
			files := map[string][]byte{}
			err := filepath.WalkDir(source, func(path string, entry os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if entry.IsDir() || entry.Name() == "manifest.json" {
					return nil
				}
				name, e := filepath.Rel(source, path)
				if e != nil {
					return e
				}
				raw, e := evidencebundle.Read(source, filepath.ToSlash(name))
				if e != nil {
					return e
				}
				files[filepath.ToSlash(name)] = raw
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			if err = json.Unmarshal(files["family-report.json"], &report); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "native":
				report["native_checks_run"] = true
			case "hardware":
				report["hardware_validated"] = true
			case "configuration":
				report["configuration"].(map[string]any)["supply_min_v"] = 0
			case "electrical":
				report["electrical"].(map[string]any)["source_margin_ma"] = 0
			}
			raw, err := evidencebundle.JSON(report)
			if err != nil {
				t.Fatal(err)
			}
			files["family-report.json"] = raw
			out := filepath.Join(t.TempDir(), "tampered")
			if err = evidencebundle.Publish(out, files); err != nil {
				t.Fatal(err)
			}
			if err = run([]string{"--target", out, "family", "verify"}, &stdout, &stderr); err == nil {
				t.Fatal("contradictory rehashed report accepted")
			}
		})
	}
}
